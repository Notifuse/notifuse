//go:build integration
// +build integration

package integration

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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

// TestDataRetentionEngine drives the retention engine through its API and its real run:
// the policy, the preview, the 7-day delay and its cancellation, who may change it, then a
// run past the delay on seeded contacts (who is deleted, who is kept and why, what the
// deletion leaves and does not leave), and the advisory lock.
func TestDataRetentionEngine(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer func() { suite.Cleanup() }()

	app := suite.ServerManager.GetApp()
	retention := app.GetDataRetentionService()
	defer retention.SetClock(time.Now)
	factory := suite.DataFactory
	client := suite.APIClient
	bg := context.Background()

	owner, err := factory.CreateUser()
	require.NoError(t, err)
	member, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	require.NoError(t, factory.AddUserToWorkspace(owner.ID, workspace.ID, "owner"))
	require.NoError(t, factory.AddUserToWorkspace(member.ID, workspace.ID, "member"))
	require.NoError(t, client.Login(owner.Email, "password"))
	client.SetWorkspaceID(workspace.ID)
	memberClient := testutil.NewAPIClient(suite.ServerManager.GetURL())
	require.NoError(t, memberClient.Login(member.Email, "password"))
	memberClient.SetWorkspaceID(workspace.ID)
	db, err := factory.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)

	call := func(t *testing.T, c *testutil.APIClient, method, path string, body interface{}) (int, map[string]interface{}) {
		t.Helper()
		var resp *http.Response
		var err error
		if method == http.MethodGet {
			resp, err = c.Get(path, map[string]string{"workspace_id": workspace.ID})
		} else {
			resp, err = c.Post(path, body)
		}
		require.NoError(t, err)
		raw, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		out := map[string]interface{}{}
		_ = json.Unmarshal(raw, &out)
		return resp.StatusCode, out
	}

	// --- seeds: contacts collected two years ago, unless said otherwise ------------------
	twoYearsAgo := time.Now().UTC().AddDate(-2, 0, 0)
	email := func(name string) string {
		return fmt.Sprintf("retention-%s-%s@example.com", name, strings.ToLower(uuid.New().String()[:8]))
	}
	old := func(t *testing.T, address string) {
		t.Helper()
		_, err := factory.CreateContact(workspace.ID, testutil.WithContactEmail(address))
		require.NoError(t, err)
		_, err = db.ExecContext(bg, `UPDATE contacts SET created_at = $2, db_created_at = $2 WHERE email = $1`, address, twoYearsAgo)
		require.NoError(t, err)
	}
	var (
		opener       = email("opener")       // only opened lately: deleted
		clicker      = email("clicker")      // engaged lately: kept
		exempt       = email("exempt")       // on an exempt list: kept
		unsubscribed = email("unsubscribed") // deleted, opt-out kept as a hash
		designated   = email("designated")   // a designated custom event lately: kept
		undesignated = email("undesignated") // a custom event not designated: deleted
		reimported   = email("reimported")   // re-imported without created_at: still deleted
		imported     = email("imported")     // stored yesterday with a 2019 created_at: kept (floor)
	)
	for _, address := range []string{opener, clicker, exempt, unsubscribed, designated, undesignated, reimported} {
		old(t, address)
	}
	_, err = factory.CreateContact(workspace.ID, testutil.WithContactEmail(imported))
	require.NoError(t, err)
	_, err = db.ExecContext(bg, `UPDATE contacts SET created_at = '2019-01-01' WHERE email = $1`, imported)
	require.NoError(t, err)

	message, err := factory.CreateMessageHistory(workspace.ID, testutil.WithMessageHistoryContactEmail(opener),
		testutil.WithMessageHistoryTemplateID("tpl-retention"))
	require.NoError(t, err)
	_, err = db.ExecContext(bg, `UPDATE message_history SET opened_at = NOW() - INTERVAL '2 days' WHERE id = $1`, message.ID)
	require.NoError(t, err)
	_, err = db.ExecContext(bg, `INSERT INTO contact_engagement (email, last_engaged_at) VALUES ($1, NOW() - INTERVAL '10 days')`, clicker)
	require.NoError(t, err)

	vip, err := factory.CreateList(workspace.ID)
	require.NoError(t, err)
	newsletter, err := factory.CreateList(workspace.ID)
	require.NoError(t, err)
	_, err = factory.CreateContactList(workspace.ID, testutil.WithContactListEmail(exempt), testutil.WithContactListListID(vip.ID))
	require.NoError(t, err)
	_, err = factory.CreateContactList(workspace.ID, testutil.WithContactListEmail(unsubscribed),
		testutil.WithContactListListID(newsletter.ID), testutil.WithContactListStatus(domain.ContactListStatusUnsubscribed))
	require.NoError(t, err)

	require.NoError(t, factory.CreateCustomEvent(workspace.ID, designated, "purchase", map[string]interface{}{}))
	require.NoError(t, factory.CreateCustomEvent(workspace.ID, undesignated, "crm_sync", map[string]interface{}{}))

	// A segment membership and a webhook subscriber, to see what the erasure announces.
	_, err = db.ExecContext(bg, `INSERT INTO segments (id, name, color, tree, timezone, version, status, generated_sql, generated_args)
		VALUES ('seg-retention', 'All', 'blue', '{}'::jsonb, 'UTC', 1, 'active', 'SELECT 1', '[]'::jsonb)`)
	require.NoError(t, err)
	_, err = db.ExecContext(bg, `INSERT INTO contact_segments (email, segment_id, version, matched_at) VALUES ($1, 'seg-retention', 1, NOW())`, unsubscribed)
	require.NoError(t, err)
	_, err = db.ExecContext(bg, `INSERT INTO webhook_subscriptions (id, name, url, secret, settings, enabled)
		VALUES ('sub-retention', 'crm', 'https://crm.example.com/hook', 'whsec_x', $1::jsonb, true)`,
		`{"event_types": ["segment.left", "contact.deleted"]}`)
	require.NoError(t, err)

	// Re-imported through the API without created_at: the import cannot make it young.
	status, body := call(t, client, http.MethodPost, "/api/contacts.import", map[string]interface{}{
		"workspace_id": workspace.ID, "contacts": []map[string]interface{}{{"email": reimported, "first_name": "Again"}},
	})
	require.Equal(t, http.StatusOK, status, "%v", body)

	policy := map[string]interface{}{
		// Webhook deliveries are kept, so the deliveries the erasure writes can be read
		// after a run whose clock is 7 days ahead.
		"categories":             map[string]int{"inactive_contacts": 365, "webhook_deliveries": 0},
		"exempt_list_ids":        []string{vip.ID},
		"engagement_event_names": []string{"purchase"},
	}

	t.Run("reading: any member; a new workspace is ready, the technical logs keep 7 days", func(t *testing.T) {
		status, view := call(t, memberClient, http.MethodGet, "/api/dataRetention.get", nil)
		require.Equal(t, http.StatusOK, status, "%v", view)
		inForce := view["in_force"].(map[string]interface{})
		assert.EqualValues(t, 7, inForce["webhook_deliveries"])
		assert.EqualValues(t, 0, inForce["inactive_contacts"])
		assert.Equal(t, true, view["backfill_ready"])
	})

	t.Run("changing the policy: owners only, and validated", func(t *testing.T) {
		status, _ := call(t, memberClient, http.MethodPost, "/api/dataRetention.set", map[string]interface{}{"workspace_id": workspace.ID, "policy": policy})
		assert.Equal(t, http.StatusForbidden, status)
		for name, bad := range map[string]interface{}{
			"a period below the minimum": map[string]interface{}{"categories": map[string]int{"inactive_contacts": 30}},
			"an unknown category":        map[string]interface{}{"categories": map[string]int{"everything": 365}},
			"an empty exemption":         map[string]interface{}{"exempt_list_ids": []string{""}},
		} {
			status, body := call(t, client, http.MethodPost, "/api/dataRetention.set", map[string]interface{}{"workspace_id": workspace.ID, "policy": bad})
			assert.Equal(t, http.StatusBadRequest, status, "%s: %v", name, body)
		}
		status, _ = call(t, client, http.MethodPost, "/api/dataRetention.set", map[string]interface{}{"workspace_id": workspace.ID})
		assert.Equal(t, http.StatusBadRequest, status, "policy is required")
	})

	t.Run("the preview counts exactly the contacts the run will delete", func(t *testing.T) {
		status, preview := call(t, client, http.MethodPost, "/api/dataRetention.preview", map[string]interface{}{"workspace_id": workspace.ID, "policy": policy})
		require.Equal(t, http.StatusOK, status, "%v", preview)
		assert.Equal(t, true, preview["pending"], "switching a category on waits")
		contacts := preview["categories"].(map[string]interface{})["inactive_contacts"].(map[string]interface{})
		assert.EqualValues(t, 4, contacts["affected"], "opener, unsubscribed, undesignated, reimported")
		assert.EqualValues(t, 1, contacts["opt_outs_kept"])
	})

	var effectiveAt time.Time
	t.Run("a policy that deletes more waits 7 days; saving it again keeps its date; it can be cancelled", func(t *testing.T) {
		status, body := call(t, client, http.MethodPost, "/api/dataRetention.set", map[string]interface{}{"workspace_id": workspace.ID, "policy": policy})
		require.Equal(t, http.StatusOK, status, "%v", body)
		pending := body["data_retention"].(map[string]interface{})["pending"].(map[string]interface{})
		effectiveAt, err = time.Parse(time.RFC3339Nano, pending["effective_at"].(string))
		require.NoError(t, err)
		assert.WithinDuration(t, time.Now().Add(7*24*time.Hour), effectiveAt, time.Minute)

		status, body = call(t, client, http.MethodPost, "/api/dataRetention.set", map[string]interface{}{"workspace_id": workspace.ID, "policy": policy})
		require.Equal(t, http.StatusOK, status, "%v", body)
		again := body["data_retention"].(map[string]interface{})["pending"].(map[string]interface{})
		assert.Equal(t, pending["effective_at"], again["effective_at"])

		status, body = call(t, client, http.MethodPost, "/api/dataRetention.cancelPending", map[string]interface{}{"workspace_id": workspace.ID})
		require.Equal(t, http.StatusOK, status, "%v", body)
		assert.Nil(t, body["data_retention"].(map[string]interface{})["pending"])

		// Saved again for the run below.
		status, body = call(t, client, http.MethodPost, "/api/dataRetention.set", map[string]interface{}{"workspace_id": workspace.ID, "policy": policy})
		require.Equal(t, http.StatusOK, status, "%v", body)
		pending = body["data_retention"].(map[string]interface{})["pending"].(map[string]interface{})
		effectiveAt, err = time.Parse(time.RFC3339Nano, pending["effective_at"].(string))
		require.NoError(t, err)
	})

	contactsLeft := func(t *testing.T) map[string]bool {
		t.Helper()
		left := map[string]bool{}
		for _, address := range []string{opener, clicker, exempt, unsubscribed, designated, undesignated, reimported, imported} {
			var n int
			require.NoError(t, db.QueryRowContext(bg, `SELECT COUNT(*) FROM contacts WHERE email = $1`, address).Scan(&n))
			left[address] = n == 1
		}
		return left
	}
	runOnce := func(t *testing.T) {
		t.Helper()
		ws, err := app.GetWorkspaceRepository().GetByID(bg, workspace.ID)
		require.NoError(t, err)
		defaults, err := retention.Defaults(bg)
		require.NoError(t, err)
		require.NoError(t, retention.RunWorkspace(bg, ws, defaults))
	}

	t.Run("before its date, the run deletes nobody", func(t *testing.T) {
		runOnce(t)
		for address, kept := range contactsLeft(t) {
			assert.True(t, kept, address)
		}
	})

	t.Run("after its date, the run deletes the inactive contacts and nothing else", func(t *testing.T) {
		later := effectiveAt.Add(time.Hour)
		retention.SetClock(func() time.Time { return later })
		runOnce(t)
		retention.SetClock(time.Now)

		left := contactsLeft(t)
		for _, address := range []string{opener, unsubscribed, undesignated, reimported} {
			assert.False(t, left[address], "%s must be deleted", address)
		}
		for _, address := range []string{clicker, exempt, designated, imported} {
			assert.True(t, left[address], "%s must be kept", address)
		}

		var queued, timeline, mine int
		require.NoError(t, db.QueryRowContext(bg, `SELECT COUNT(*) FROM data_retention_erasure_queue`).Scan(&queued))
		assert.Zero(t, queued, "the history of the deleted contacts is erased in the same run")
		require.NoError(t, db.QueryRowContext(bg, `SELECT COUNT(*) FROM contact_timeline WHERE email = ANY($1)`,
			fmt.Sprintf("{%s,%s,%s,%s}", opener, unsubscribed, undesignated, reimported)).Scan(&timeline))
		assert.Zero(t, timeline)
		require.NoError(t, db.QueryRowContext(bg, `SELECT COUNT(*) FROM message_history WHERE id = $1 AND contact_email = 'DELETED_EMAIL'`, message.ID).Scan(&mine))
		assert.Equal(t, 1, mine, "the message is redacted, not deleted")

		var hashes int
		require.NoError(t, db.QueryRowContext(bg, `SELECT COUNT(*) FROM contact_opt_out_hashes WHERE email_hash = contact_opt_out_hash($1) AND source = 'retention'`, unsubscribed).Scan(&hashes))
		assert.Equal(t, 1, hashes, "the unsubscribe survives as a hash")

		var left_, deleted int
		require.NoError(t, db.QueryRowContext(bg, `SELECT COUNT(*) FROM webhook_deliveries WHERE event_type = 'segment.left' AND payload->>'email' = $1`, unsubscribed).Scan(&left_))
		assert.Zero(t, left_, "an erased contact does not leave a segment")
		var payload string
		require.NoError(t, db.QueryRowContext(bg, `SELECT COUNT(*), MAX(payload::text) FROM webhook_deliveries
			WHERE event_type = 'contact.deleted' AND payload->'contact'->>'email' = $1`, unsubscribed).Scan(&deleted, &payload))
		assert.Equal(t, 1, deleted)
		assert.JSONEq(t, fmt.Sprintf(`{"contact": {"email": %q}}`, unsubscribed), payload, "the address only (D8)")

		var runs int
		var results string
		require.NoError(t, db.QueryRowContext(bg, `SELECT COUNT(*), MAX(results::text) FILTER (WHERE started_at = (SELECT MAX(started_at) FROM data_retention_runs))
			FROM data_retention_runs`).Scan(&runs, &results))
		assert.Equal(t, 2, runs)
		var parsed map[domain.RetentionCategory]domain.RetentionCategoryResult
		require.NoError(t, json.Unmarshal([]byte(results), &parsed))
		assert.EqualValues(t, 4, parsed[domain.RetentionInactiveContacts].Affected)
		assert.EqualValues(t, 1, parsed[domain.RetentionInactiveContacts].OptOutsKept)

		var audit int
		require.NoError(t, suite.DBManager.GetDB().QueryRowContext(bg, `SELECT COUNT(*) FROM audit_logs WHERE workspace_id = $1 AND action = 'dataRetention.purged'`,
			workspace.ID).Scan(&audit))
		assert.Equal(t, 1, audit, "one audit entry, for the run that deleted something")
	})

	t.Run("the unsubscribed contact comes back unsubscribed, and the API does not re-subscribe them", func(t *testing.T) {
		status, body := call(t, client, http.MethodPost, "/api/contacts.upsert", map[string]interface{}{
			"workspace_id": workspace.ID, "contact": map[string]interface{}{"email": unsubscribed},
		})
		require.Equal(t, http.StatusOK, status, "%v", body)
		status, body = call(t, client, http.MethodPost, "/api/lists.subscribe", map[string]interface{}{
			"workspace_id": workspace.ID, "contact": map[string]interface{}{"email": unsubscribed}, "list_ids": []string{newsletter.ID},
		})
		require.Equal(t, http.StatusOK, status, "%v", body)
		var st string
		require.NoError(t, db.QueryRowContext(bg, `SELECT status FROM contact_lists WHERE email = $1 AND list_id = $2`, unsubscribed, newsletter.ID).Scan(&st))
		assert.Equal(t, "unsubscribed", st)
	})

	t.Run("another server holding the lock: the run does nothing", func(t *testing.T) {
		release, ok, err := repository.NewDataRetentionRepository(app.GetWorkspaceRepository()).TryLock(bg, workspace.ID)
		require.NoError(t, err)
		require.True(t, ok)
		defer release()
		var before, after int
		require.NoError(t, db.QueryRowContext(bg, `SELECT COUNT(*) FROM data_retention_runs`).Scan(&before))
		runOnce(t)
		require.NoError(t, db.QueryRowContext(bg, `SELECT COUNT(*) FROM data_retention_runs`).Scan(&after))
		assert.Equal(t, before, after)
	})

	t.Run("the runs and their export", func(t *testing.T) {
		status, body := call(t, memberClient, http.MethodGet, "/api/dataRetention.runs", nil)
		require.Equal(t, http.StatusOK, status, "%v", body)
		assert.Len(t, body["runs"], 2)
		resp, err := client.Get("/api/dataRetention.exportRuns", map[string]string{"workspace_id": workspace.ID})
		require.NoError(t, err)
		raw, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Contains(t, resp.Header.Get("Content-Type"), "text/csv")
		assert.Contains(t, string(raw), "inactive_contacts")
		assert.NotContains(t, string(raw), "@example.com", "counts only")
	})

	_ = sql.ErrNoRows
}

// TestDataRetentionCategories runs every other category past its date on seeded rows: each
// removes what is older than its period and keeps the rest, a message is redacted without a
// copy in the timeline, a designated custom event is folded into the contact's engagement
// date before it goes (so the contact outlives it), and a past web analytics month is dropped.
func TestDataRetentionCategories(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer func() { suite.Cleanup() }()

	app := suite.ServerManager.GetApp()
	retention := app.GetDataRetentionService()
	defer retention.SetClock(time.Now)
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
	exec := func(t *testing.T, query string, args ...interface{}) {
		t.Helper()
		_, err := db.ExecContext(bg, query, args...)
		require.NoError(t, err, query)
	}
	count := func(t *testing.T, query string, args ...interface{}) int {
		t.Helper()
		var n int
		require.NoError(t, db.QueryRowContext(bg, query, args...).Scan(&n), query)
		return n
	}
	days := func(n int) time.Time { return time.Now().UTC().AddDate(0, 0, -n) }

	live := fmt.Sprintf("live-%s@example.com", strings.ToLower(uuid.New().String()[:8]))
	folded := fmt.Sprintf("folded-%s@example.com", strings.ToLower(uuid.New().String()[:8]))
	for _, address := range []string{live, folded} {
		_, err := factory.CreateContact(workspace.ID, testutil.WithContactEmail(address))
		require.NoError(t, err)
	}
	exec(t, `UPDATE contacts SET created_at = $2, db_created_at = $2 WHERE email = $1`, folded, days(730))
	exec(t, `UPDATE contacts SET created_at = $2, db_created_at = $2 WHERE email = $1`, live, days(730))
	exec(t, `INSERT INTO contact_engagement (email, last_engaged_at) VALUES ($1, NOW())`, live)

	// Message history: an old bounced message quoting the address, and a recent one.
	oldMessage, err := factory.CreateMessageHistory(workspace.ID, testutil.WithMessageHistoryContactEmail(live), testutil.WithMessageHistoryTemplateID("tpl-cat"))
	require.NoError(t, err)
	recentMessage, err := factory.CreateMessageHistory(workspace.ID, testutil.WithMessageHistoryContactEmail(live), testutil.WithMessageHistoryTemplateID("tpl-cat"))
	require.NoError(t, err)
	exec(t, `UPDATE message_history SET created_at = $2, status_info = $3 WHERE id = $1`, oldMessage.ID, days(200), "550 "+live+" does not exist")
	exec(t, `UPDATE message_history SET created_at = $2 WHERE id = $1`, recentMessage.ID, days(10))

	// Timeline, inbound events, custom events, automation journeys, deliveries, failed emails.
	for _, age := range []int{200, 10} {
		exec(t, `INSERT INTO contact_timeline (email, operation, entity_type, kind, entity_id, changes, created_at)
			VALUES ($1, 'insert', 'contact', 'contact.updated', $2, '{}'::jsonb, $3)`, live, fmt.Sprintf("age-%d", age), days(age))
		exec(t, `INSERT INTO inbound_webhook_events (id, type, source, integration_id, recipient_email, timestamp, raw_payload, created_at)
			VALUES (gen_random_uuid(), 'delivered', 'ses', 'int-1', $1, $2, '{}', $2)`, live, days(age))
	}
	require.NoError(t, factory.CreateCustomEvent(workspace.ID, folded, "purchase", map[string]interface{}{}))
	require.NoError(t, factory.CreateCustomEvent(workspace.ID, live, "crm_sync", map[string]interface{}{}))
	exec(t, `UPDATE custom_events SET occurred_at = $2 WHERE email = $1`, folded, days(400))
	exec(t, `INSERT INTO automations (id, workspace_id, name, status, trigger_config) VALUES ('auto-cat', $1, 'Welcome', 'live', '{}')`, workspace.ID)
	for i, row := range []struct {
		status string
		age    int
	}{{"completed", 200}, {"active", 200}, {"completed", 10}} {
		exec(t, `INSERT INTO contact_automations (id, automation_id, contact_email, status, entered_at) VALUES ($1, 'auto-cat', $2, $3, $4)`,
			fmt.Sprintf("journey-%d", i), live, row.status, days(row.age))
	}
	exec(t, `INSERT INTO webhook_subscriptions (id, name, url, secret, settings, enabled)
		VALUES ('sub-x', 'crm', 'https://crm.example.com/hook', 'whsec_x', '{"event_types": []}'::jsonb, false)`)
	// The run below is 7 days ahead, so "recent" here means a date in the future of today.
	for i, at := range []time.Time{days(30), time.Now().UTC().AddDate(0, 0, 3)} {
		exec(t, `INSERT INTO webhook_deliveries (id, subscription_id, event_type, payload, status, attempts, max_attempts, next_attempt_at, created_at)
			VALUES ($1, 'sub-x', 'contact.updated', '{}'::jsonb, 'delivered', 1, 10, NOW(), $2)`, fmt.Sprintf("delivery-%d", i), at)
		exec(t, `INSERT INTO email_queue (id, status, priority, source_type, source_id, integration_id, provider_kind, contact_email, message_id, template_id, payload, processed_at)
			VALUES (gen_random_uuid(), 'failed', 5, 'broadcast', 'b-1', 'i-1', 'smtp', $1, $2, 'tpl-1', '{}'::jsonb, $3)`, live, fmt.Sprintf("queued-%d", i), at)
	}

	// A web analytics month four months ago.
	past := time.Date(time.Now().UTC().Year(), time.Now().UTC().Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, -4, 0)
	partition := func(table string) string { return fmt.Sprintf("%s_y%dm%02d", table, past.Year(), int(past.Month())) }
	for _, table := range []string{"web_sessions", "web_pages", "web_goals"} {
		exec(t, fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s PARTITION OF %s FOR VALUES FROM ('%s') TO ('%s')`,
			partition(table), table, past.Format("2006-01-02"), past.AddDate(0, 1, 0).Format("2006-01-02")))
	}

	resp, err := client.Post("/api/dataRetention.set", map[string]interface{}{"workspace_id": workspace.ID, "policy": map[string]interface{}{
		"categories": map[string]int{
			"inactive_contacts": 720, "message_history": 90, "contact_timeline": 90, "custom_events": 30,
			"automation_history": 90, "web_analytics": 31, "inbound_webhook_events": 30, "webhook_deliveries": 7, "failed_emails": 7,
		},
		"engagement_event_names": []string{"purchase"},
	}})
	require.NoError(t, err)
	raw, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode, "%s", raw)

	later := time.Now().Add(domain.RetentionPendingDelay + time.Hour)
	retention.SetClock(func() time.Time { return later })
	runOnce := func(t *testing.T) {
		t.Helper()
		ws, err := app.GetWorkspaceRepository().GetByID(bg, workspace.ID)
		require.NoError(t, err)
		defaults, err := retention.Defaults(bg)
		require.NoError(t, err)
		require.NoError(t, retention.RunWorkspace(bg, ws, defaults))
	}
	// Seeding status_info above wrote a timeline row quoting the address; the redaction must
	// add none.
	quotingBefore := count(t, `SELECT COUNT(*) FROM contact_timeline WHERE changes::text LIKE '%' || $1 || '%'`, live)
	runOnce(t)

	t.Run("message history: the old message is redacted, with no copy in the timeline; the recent one kept", func(t *testing.T) {
		assert.Equal(t, 1, count(t, `SELECT COUNT(*) FROM message_history WHERE id = $1 AND contact_email = 'DELETED_EMAIL' AND status_info IS NULL`, oldMessage.ID))
		assert.Equal(t, 1, count(t, `SELECT COUNT(*) FROM message_history WHERE id = $1 AND contact_email = $2`, recentMessage.ID, live))
		assert.Equal(t, quotingBefore, count(t, `SELECT COUNT(*) FROM contact_timeline WHERE changes::text LIKE '%' || $1 || '%'`, live))
	})
	t.Run("timeline: old rows go, recent ones stay, and the index was built", func(t *testing.T) {
		assert.Zero(t, count(t, `SELECT COUNT(*) FROM contact_timeline WHERE entity_id = 'age-200'`))
		assert.Equal(t, 1, count(t, `SELECT COUNT(*) FROM contact_timeline WHERE entity_id = 'age-10'`))
		assert.Equal(t, 1, count(t, `SELECT COUNT(*) FROM pg_index i JOIN pg_class c ON c.oid = i.indexrelid
			WHERE c.relname = 'idx_contact_timeline_created_at' AND i.indisvalid`))
	})
	t.Run("inbound events, deliveries and failed emails: the old ones go", func(t *testing.T) {
		assert.Equal(t, 1, count(t, `SELECT COUNT(*) FROM inbound_webhook_events WHERE recipient_email = $1`, live))
		assert.Equal(t, 1, count(t, `SELECT COUNT(*) FROM webhook_deliveries WHERE id LIKE 'delivery-%'`))
		assert.Equal(t, 1, count(t, `SELECT COUNT(*) FROM email_queue WHERE contact_email = $1`, live))
	})
	t.Run("automation history: finished old journeys go, a running one stays", func(t *testing.T) {
		assert.Zero(t, count(t, `SELECT COUNT(*) FROM contact_automations WHERE id = 'journey-0'`))
		assert.Equal(t, 1, count(t, `SELECT COUNT(*) FROM contact_automations WHERE id = 'journey-1'`), "still active")
		assert.Equal(t, 1, count(t, `SELECT COUNT(*) FROM contact_automations WHERE id = 'journey-2'`), "recent")
	})
	t.Run("custom events: old ones go; a designated one is folded into the engagement date first", func(t *testing.T) {
		assert.Zero(t, count(t, `SELECT COUNT(*) FROM custom_events WHERE email = $1`, folded))
		assert.Equal(t, 1, count(t, `SELECT COUNT(*) FROM custom_events WHERE email = $1`, live), "recent")
		var engaged time.Time
		require.NoError(t, db.QueryRowContext(bg, `SELECT last_engaged_at FROM contact_engagement WHERE email = $1`, folded).Scan(&engaged))
		assert.WithinDuration(t, days(400), engaged, time.Minute)

		// The next run: without the fold, the contact's clock would be its collection, two
		// years ago, and it would go.
		runOnce(t)
		assert.Equal(t, 1, count(t, `SELECT COUNT(*) FROM contacts WHERE email = $1`, folded))
	})
	t.Run("web analytics: the past month's partitions are dropped", func(t *testing.T) {
		for _, table := range []string{"web_sessions", "web_pages", "web_goals"} {
			assert.Zero(t, count(t, `SELECT COUNT(*) FROM pg_class WHERE relname = $1`, partition(table)), table)
		}
	})
}
