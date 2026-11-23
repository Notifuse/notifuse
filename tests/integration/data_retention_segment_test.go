//go:build integration
// +build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/repository"
	"github.com/Notifuse/notifuse/tests/testutil"
)

// TestDataRetentionExpiringSoonSegment checks the "expires within N days" condition (D13)
// against the purge itself: on the same contacts, the function contact_retention_expires_at
// says "expired" for exactly the contacts the purge lists, and the condition, compiled as
// segments compile it, follows the policy in force, the change that waits and its
// cancellation, while the segments that use it are queued for a rebuild.
func TestDataRetentionExpiringSoonSegment(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer func() { suite.Cleanup() }()

	app := suite.ServerManager.GetApp()
	factory := suite.DataFactory
	client := suite.APIClient
	bg := context.Background()

	owner, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	require.NoError(t, factory.AddUserToWorkspace(owner.ID, workspace.ID, "owner"))
	require.NoError(t, client.Login(owner.Email, "password"))
	client.SetWorkspaceID(workspace.ID)
	db, err := factory.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)
	repo := repository.NewDataRetentionRepository(app.GetWorkspaceRepository())

	ago := func(days int) time.Time { return time.Now().UTC().AddDate(0, 0, -days) }
	email := func(name string) string {
		return fmt.Sprintf("expiring-%s-%s@example.com", name, strings.ToLower(uuid.New().String()[:8]))
	}
	var (
		due        = email("due")        // collected 350 days ago: 15 days left of a 365-day period
		far        = email("far")        // collected 100 days ago
		expired    = email("expired")    // collected 400 days ago
		exempt     = email("exempt")     // 400 days ago, on an exempt list
		designated = email("designated") // 400 days ago, but a designated event 10 days ago
		floor      = email("floor")      // stored today with a 2019 created_at: 7 days of floor
	)
	collected := map[string]int{due: 350, far: 100, expired: 400, exempt: 400, designated: 400}
	for address, days := range collected {
		_, err := factory.CreateContact(workspace.ID, testutil.WithContactEmail(address))
		require.NoError(t, err)
		_, err = db.ExecContext(bg, `UPDATE contacts SET created_at = $2, db_created_at = $2 WHERE email = $1`, address, ago(days))
		require.NoError(t, err)
	}
	_, err = factory.CreateContact(workspace.ID, testutil.WithContactEmail(floor))
	require.NoError(t, err)
	_, err = db.ExecContext(bg, `UPDATE contacts SET created_at = '2019-01-01' WHERE email = $1`, floor)
	require.NoError(t, err)
	vip, err := factory.CreateList(workspace.ID)
	require.NoError(t, err)
	_, err = factory.CreateContactList(workspace.ID, testutil.WithContactListEmail(exempt), testutil.WithContactListListID(vip.ID))
	require.NoError(t, err)
	require.NoError(t, factory.CreateCustomEvent(workspace.ID, designated, "purchase", map[string]interface{}{}))
	_, err = db.ExecContext(bg, `UPDATE custom_events SET occurred_at = $2 WHERE email = $1`, designated, ago(10))
	require.NoError(t, err)

	// A segment that uses the condition, to see it queued for a rebuild.
	_, err = db.ExecContext(bg, `INSERT INTO segments (id, name, color, tree, timezone, version, status, generated_sql, generated_args, recompute_after)
		VALUES ('seg-expiring', 'Expiring soon', 'red', $1::jsonb, 'UTC', 1, 'active', 'SELECT 1', '[]'::jsonb, NOW() + INTERVAL '1 day')`,
		`{"kind":"leaf","leaf":{"source":"retention","retention":{"operator":"expires_within_days","days":30}}}`)
	require.NoError(t, err)
	queued := func(t *testing.T) bool {
		t.Helper()
		var due bool
		require.NoError(t, db.QueryRowContext(bg, `SELECT recompute_after <= NOW() FROM segments WHERE id = 'seg-expiring'`).Scan(&due))
		return due
	}

	mine := map[string]bool{due: true, far: true, expired: true, exempt: true, designated: true, floor: true}
	expiringWithin := func(t *testing.T, days int) []string {
		t.Helper()
		resp, err := client.Post("/api/segments.preview", map[string]interface{}{
			"workspace_id": workspace.ID, "limit": 100,
			"tree": map[string]interface{}{"kind": "leaf", "leaf": map[string]interface{}{
				"source": "retention", "retention": map[string]interface{}{"operator": "expires_within_days", "days": days}}},
		})
		require.NoError(t, err)
		raw, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode, "%s", raw)
		var preview domain.PreviewSegmentResponse
		require.NoError(t, json.Unmarshal(raw, &preview))
		// The preview counts; the addresses come from running the SQL it compiled, unchanged.
		rows, err := db.QueryContext(bg, preview.GeneratedSQL, preview.SQLArgs...)
		require.NoError(t, err)
		var out []string
		for rows.Next() {
			var address string
			require.NoError(t, rows.Scan(&address))
			if mine[address] {
				out = append(out, address)
			}
		}
		_ = rows.Close()
		assert.Equal(t, len(out), preview.TotalCount, "the preview counts the same contacts")
		sort.Strings(out)
		return out
	}
	sorted := func(values ...string) []string { sort.Strings(values); return values }

	t.Run("no contact policy: the condition matches nobody", func(t *testing.T) {
		assert.Empty(t, expiringWithin(t, 365))
	})

	policy := domain.ContactPolicyMirror{Days: 365, ExemptListIDs: []string{vip.ID}, EngagementEventNames: []string{"purchase"}}

	t.Run("the function agrees with the purge on who has expired", func(t *testing.T) {
		require.NoError(t, repo.WriteContactPolicyMirror(bg, workspace.ID, policy))
		assert.True(t, queued(t), "writing the policy queues the segments that use the condition")

		listed, err := repo.ListExpiredContacts(bg, workspace.ID, domain.ExpiredContactsQuery{
			EngagedBefore: time.Now().UTC().AddDate(0, 0, -365), ExemptListIDs: policy.ExemptListIDs,
			EngagementEventNames: policy.EngagementEventNames, Limit: 1000,
		})
		require.NoError(t, err)
		rows, err := db.QueryContext(bg, `SELECT email FROM contacts WHERE contact_retention_expires_at(email) <= NOW() ORDER BY email`)
		require.NoError(t, err)
		var byFunction []string
		for rows.Next() {
			var address string
			require.NoError(t, rows.Scan(&address))
			byFunction = append(byFunction, address)
		}
		_ = rows.Close()
		assert.Equal(t, listed, byFunction)
		assert.Equal(t, []string{expired}, listed)
	})

	t.Run("expiring within 30 days: the due contact, the expired one, and the floor's", func(t *testing.T) {
		assert.Equal(t, sorted(due, expired, floor), expiringWithin(t, 30))
		assert.Equal(t, sorted(due, expired, floor, far), expiringWithin(t, 300), "the designated event keeps its contact longer")
	})

	t.Run("a shorter period that waits counts from its date", func(t *testing.T) {
		// Through the API: a policy saved now with 180 days waits 7 days.
		_, err := db.ExecContext(bg, `UPDATE segments SET recompute_after = NOW() + INTERVAL '1 day' WHERE id = 'seg-expiring'`)
		require.NoError(t, err)
		resp, err := client.Post("/api/dataRetention.set", map[string]interface{}{"workspace_id": workspace.ID, "policy": map[string]interface{}{
			"categories": map[string]int{"inactive_contacts": 180}, "exempt_list_ids": []string{vip.ID}, "engagement_event_names": []string{"purchase"},
		}})
		require.NoError(t, err)
		raw, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode, "%s", raw)
		assert.True(t, queued(t))

		var days, pendingDays int
		require.NoError(t, db.QueryRowContext(bg, `SELECT contacts_days, contacts_pending_days FROM data_retention_state WHERE id = 1`).Scan(&days, &pendingDays))
		assert.Equal(t, 0, days, "nothing in force: switching on waits")
		assert.Equal(t, 180, pendingDays)
		// Due, expired and the floor's expire on the date the change applies, 7 days from now.
		assert.Equal(t, sorted(due, expired, floor), expiringWithin(t, 8))
		assert.Empty(t, expiringWithin(t, 6), "nothing before the change applies")
	})

	t.Run("cancelling the change: nobody expires again", func(t *testing.T) {
		resp, err := client.Post("/api/dataRetention.cancelPending", map[string]interface{}{"workspace_id": workspace.ID})
		require.NoError(t, err)
		_ = resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Empty(t, expiringWithin(t, 365))
	})

	setPolicy := func(t *testing.T, policy map[string]interface{}) (int, string) {
		t.Helper()
		resp, err := client.Post("/api/dataRetention.set", map[string]interface{}{"workspace_id": workspace.ID, "policy": policy})
		require.NoError(t, err)
		raw, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		return resp.StatusCode, string(raw)
	}

	t.Run("a waiting change that only removes an exemption reaches the exempt contact from its date", func(t *testing.T) {
		// In force: 365 days with vip exempt (written as the run would), then a save that
		// keeps 365 days and drops the exemption: it waits 7 days.
		_, err := app.GetWorkspaceRepository().SetDataRetentionSettings(bg, workspace.ID, currentRevision(t, app, workspace.ID), &domain.DataRetentionSettings{
			Policy: &domain.DataRetentionPolicy{Categories: map[domain.RetentionCategory]int{domain.RetentionInactiveContacts: 365},
				ExemptListIDs: []string{vip.ID}, EngagementEventNames: []string{"purchase"}},
		})
		require.NoError(t, err)
		status, body := setPolicy(t, map[string]interface{}{"categories": map[string]int{"inactive_contacts": 365}, "engagement_event_names": []string{"purchase"}})
		require.Equal(t, http.StatusOK, status, body)
		assert.NotContains(t, expiringWithin(t, 6), exempt, "still exempt before the date")
		assert.Contains(t, expiringWithin(t, 8), exempt, "no longer exempt once the change applies")
		resp, err := client.Post("/api/dataRetention.cancelPending", map[string]interface{}{"workspace_id": workspace.ID})
		require.NoError(t, err)
		_ = resp.Body.Close()
	})

	t.Run("an exempt list that is gone: nobody expires, as the purge skips the category", func(t *testing.T) {
		gone, err := factory.CreateList(workspace.ID)
		require.NoError(t, err)
		require.NoError(t, repo.WriteContactPolicyMirror(bg, workspace.ID, domain.ContactPolicyMirror{Days: 365, ExemptListIDs: []string{gone.ID}}))
		require.NotEmpty(t, expiringWithin(t, 30))
		_, err = db.ExecContext(bg, `UPDATE lists SET deleted_at = NOW() WHERE id = $1`, gone.ID)
		require.NoError(t, err)
		assert.Empty(t, expiringWithin(t, 30))
	})

	t.Run("a segment that uses the condition cannot be exempt", func(t *testing.T) {
		status, body := setPolicy(t, map[string]interface{}{"categories": map[string]int{"inactive_contacts": 365}, "exempt_segment_ids": []string{"seg-expiring"}})
		assert.Equal(t, http.StatusBadRequest, status, body)
		assert.Contains(t, body, "seg-expiring")
	})
}

func currentRevision(t *testing.T, app testutil.AppInterface, workspaceID string) int {
	t.Helper()
	ws, err := app.GetWorkspaceRepository().GetByID(context.Background(), workspaceID)
	require.NoError(t, err)
	if ws.Settings.DataRetention == nil {
		return 0
	}
	return ws.Settings.DataRetention.Revision
}
