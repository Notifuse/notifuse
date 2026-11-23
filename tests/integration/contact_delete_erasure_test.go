//go:build integration
// +build integration

package integration

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/app"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/repository"
	"github.com/Notifuse/notifuse/internal/service"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/Notifuse/notifuse/tests/testutil"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestContactDeleteErasure deletes a contact who left a trace in every place a contact can,
// through /api/contacts.delete as an API client sends it, then scans every text and JSON
// column of every workspace table for their address instead of listing the tables: a table
// added later is covered without anyone remembering it. The one row allowed to keep the
// address are the contact.deleted and segment.left webhook deliveries the deletion queues,
// which is how subscribers learn of it.
//
// It then checks the opt-out hashes: one per list the contact opted out of, their statuses
// put back when the address returns through any path, and none with forget_opt_outs.
func TestContactDeleteErasure(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	ctx := context.Background()
	factory := suite.DataFactory
	client := suite.APIClient

	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	require.NoError(t, factory.AddUserToWorkspace(user.ID, workspace.ID, "owner"))
	require.NoError(t, client.Login(user.Email, "password"))
	client.SetWorkspaceID(workspace.ID)
	db, err := factory.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)

	template, err := factory.CreateTemplate(workspace.ID)
	require.NoError(t, err)

	workspaceRepo := suite.ServerManager.GetApp().GetWorkspaceRepository()
	runRetentionWorker := func(ctx context.Context) {
		service.NewDataRetentionWorker(workspaceRepo, repository.NewContactErasureRepository(workspaceRepo), logger.NewLogger()).RunOnce(ctx)
	}

	post := func(t *testing.T, path string, body interface{}) (int, string) {
		t.Helper()
		resp, err := client.Post(path, body)
		require.NoError(t, err)
		raw, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		return resp.StatusCode, string(raw)
	}
	deleteContact := func(t *testing.T, email string, forget bool) {
		t.Helper()
		body := fmt.Sprintf(`{"workspace_id":%q,"email":%q}`, workspace.ID, email)
		if forget {
			body = fmt.Sprintf(`{"workspace_id":%q,"email":%q,"forget_opt_outs":true}`, workspace.ID, email)
		}
		code, raw := post(t, "/api/contacts.delete", json.RawMessage(body))
		require.Equal(t, http.StatusOK, code, raw)
	}
	importContact := func(t *testing.T, email string, listIDs ...string) {
		t.Helper()
		body := map[string]interface{}{
			"workspace_id": workspace.ID,
			"contacts":     []map[string]interface{}{{"email": email}},
		}
		if len(listIDs) > 0 {
			body["subscribe_to_lists"] = listIDs
		}
		code, raw := post(t, "/api/contacts.import", body)
		require.Equal(t, http.StatusOK, code, raw)
	}
	listStatus := func(t *testing.T, email, listID string) string {
		t.Helper()
		var status string
		err := db.QueryRowContext(ctx, `SELECT status FROM contact_lists WHERE email = $1 AND list_id = $2`, email, listID).Scan(&status)
		if err == sql.ErrNoRows {
			return "none"
		}
		require.NoError(t, err)
		return status
	}
	hashCount := func(t *testing.T) int {
		t.Helper()
		var n int
		require.NoError(t, db.QueryRowContext(ctx, `SELECT COUNT(*) FROM contact_opt_out_hashes`).Scan(&n))
		return n
	}
	timelineKinds := func(t *testing.T, email string) []string {
		t.Helper()
		rows, err := db.QueryContext(ctx, `SELECT kind FROM contact_timeline WHERE email = $1 ORDER BY created_at, kind`, email)
		require.NoError(t, err)
		defer func() { _ = rows.Close() }()
		var kinds []string
		for rows.Next() {
			var kind string
			require.NoError(t, rows.Scan(&kind))
			kinds = append(kinds, kind)
		}
		return kinds
	}
	newOptedOutContact := func(t *testing.T, prefix string, unsubscribedFrom, activeOn *domain.List) string {
		t.Helper()
		email := fmt.Sprintf("%s-%s@example.com", prefix, strings.ToLower(uuid.New().String()[:8]))
		_, err := factory.CreateContact(workspace.ID, testutil.WithContactEmail(email))
		require.NoError(t, err)
		for _, list := range []*domain.List{unsubscribedFrom, activeOn} {
			_, err = factory.CreateContactList(workspace.ID, testutil.WithContactListEmail(email),
				testutil.WithContactListListID(list.ID), testutil.WithContactListStatus(domain.ContactListStatusActive))
			require.NoError(t, err)
		}
		require.NoError(t, factory.UpdateContactListStatus(workspace.ID, email, unsubscribedFrom.ID, domain.ContactListStatusUnsubscribed))
		return email
	}

	t.Run("no workspace table keeps the address", func(t *testing.T) {
		left, err := factory.CreateList(workspace.ID)
		require.NoError(t, err)
		kept, err := factory.CreateList(workspace.ID)
		require.NoError(t, err)
		email := newOptedOutContact(t, "erase", left, kept)
		cc := fmt.Sprintf("cc-%s", email)

		// Webhook deliveries carrying the address, queued and never sent (the harness runs
		// no delivery worker).
		createZapierSubscription(t, suite, workspace.ID, "Erasure", "https://hooks.example.com/erasure",
			[]string{"contact.updated", "contact.deleted", "list.unsubscribed", "custom_event.created"}, nil, nil)
		code, raw := post(t, "/api/contacts.upsert", map[string]interface{}{
			"workspace_id": workspace.ID, "contact": map[string]interface{}{"email": email, "first_name": "Erin"},
		})
		require.Equal(t, http.StatusOK, code, raw)
		require.NoError(t, factory.UpdateContactListStatus(workspace.ID, email, kept.ID, domain.ContactListStatusActive))

		// A message with cc, bcc and a subject quoting them, a bounce text quoting the
		// recipient, and two attachments: one only this contact received, one another
		// contact received too.
		message, err := factory.CreateMessageHistory(workspace.ID, testutil.WithMessageContact(email),
			testutil.WithMessageTemplate(template.ID), testutil.WithMessageListID(kept.ID))
		require.NoError(t, err)
		own := sha256.Sum256([]byte("own file of " + email))
		shared := sha256.Sum256([]byte("shared file " + email))
		ownChecksum, sharedChecksum := hex.EncodeToString(own[:]), hex.EncodeToString(shared[:])
		for _, checksum := range []string{ownChecksum, sharedChecksum} {
			_, err = db.ExecContext(ctx, `INSERT INTO message_attachments (checksum, content, content_type, size_bytes) VALUES ($1, $2, 'text/plain', 10)`,
				checksum, []byte("Dear "+email))
			require.NoError(t, err)
		}
		attachments := func(checksums ...string) string {
			var items []string
			for _, c := range checksums {
				items = append(items, fmt.Sprintf(`{"checksum":%q,"filename":"note.txt","content_type":"text/plain","disposition":"attachment"}`, c))
			}
			return "[" + strings.Join(items, ",") + "]"
		}
		_, err = db.ExecContext(ctx, `UPDATE message_history SET channel_options = $2::jsonb, attachments = $3::jsonb, status_info = $4 WHERE id = $1`,
			message.ID,
			fmt.Sprintf(`{"cc":[%q],"bcc":[%q],"reply_to":%q,"subject":"For %s"}`, cc, cc, cc, email),
			attachments(ownChecksum, sharedChecksum),
			fmt.Sprintf("550 5.1.1 <%s>: user unknown", email))
		require.NoError(t, err)
		other := fmt.Sprintf("other-%s@example.com", strings.ToLower(uuid.New().String()[:8]))
		_, err = factory.CreateContact(workspace.ID, testutil.WithContactEmail(other))
		require.NoError(t, err)
		otherMessage, err := factory.CreateMessageHistory(workspace.ID, testutil.WithMessageContact(other), testutil.WithMessageTemplate(template.ID))
		require.NoError(t, err)
		_, err = db.ExecContext(ctx, `UPDATE message_history SET attachments = $2::jsonb WHERE id = $1`, otherMessage.ID, attachments(sharedChecksum))
		require.NoError(t, err)

		_, err = db.ExecContext(ctx, `INSERT INTO inbound_webhook_events (id, type, source, integration_id, recipient_email, message_id, timestamp, raw_payload, bounce_type, bounce_diagnostic, created_at)
			VALUES ($1, 'bounce', 'ses', 'integration', $2, $3, NOW(), $4, 'Permanent', $5, NOW())`,
			uuid.New().String(), email, message.ID, fmt.Sprintf(`{"to":%q}`, email), fmt.Sprintf("550 <%s> unknown", email))
		require.NoError(t, err)

		// An automation journey with a step output naming the address, and the "once" log.
		automation, err := factory.CreateAutomation(workspace.ID)
		require.NoError(t, err)
		journeyID := uuid.New().String()
		_, err = db.ExecContext(ctx, `INSERT INTO contact_automations (id, automation_id, contact_email, status, context) VALUES ($1, $2, $3, 'completed', $4::jsonb)`,
			journeyID, automation.ID, email, fmt.Sprintf(`{"trigger_event":{"email":%q}}`, email))
		require.NoError(t, err)
		_, err = db.ExecContext(ctx, `INSERT INTO automation_node_executions (id, contact_automation_id, automation_id, node_id, node_type, action, output) VALUES ($1, $2, $3, 'n1', 'email', 'completed', $4::jsonb)`,
			uuid.New().String(), journeyID, automation.ID, fmt.Sprintf(`{"to":%q}`, email))
		require.NoError(t, err)
		_, err = db.ExecContext(ctx, `INSERT INTO automation_trigger_log (id, automation_id, contact_email) VALUES ($1, $2, $3)`,
			uuid.New().String(), automation.ID, email)
		require.NoError(t, err)

		require.NoError(t, factory.CreateCustomEvent(workspace.ID, email, "purchase", map[string]interface{}{"buyer": email}))

		deleteContact(t, email, false)

		for _, needle := range []string{email, cc} {
			found := scanWorkspaceForText(t, db, needle)
			assert.Empty(t, found, "rows still holding %s", needle)
		}
		var deletedDeliveries int
		require.NoError(t, db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM webhook_deliveries WHERE event_type = 'contact.deleted' AND payload::text LIKE '%' || $1 || '%'`, email).Scan(&deletedDeliveries))
		assert.Equal(t, 1, deletedDeliveries, "the contact.deleted delivery is kept")

		var stillThere bool
		require.NoError(t, db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM message_attachments WHERE checksum = $1)`, ownChecksum).Scan(&stillThere))
		assert.False(t, stillThere, "a file only this contact received is deleted")
		require.NoError(t, db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM message_attachments WHERE checksum = $1)`, sharedChecksum).Scan(&stillThere))
		assert.True(t, stillThere, "a file another contact received is kept")

		var messages int
		require.NoError(t, db.QueryRowContext(ctx, `SELECT COUNT(*) FROM message_history WHERE id = $1 AND contact_email = 'DELETED_EMAIL'`, message.ID).Scan(&messages))
		assert.Equal(t, 1, messages, "the message is redacted, not deleted: newsletter stats read it")

		var hashedList, hashedStatus string
		require.NoError(t, db.QueryRowContext(ctx, `SELECT list_id, status FROM contact_opt_out_hashes WHERE email_hash = contact_opt_out_hash($1)`, email).
			Scan(&hashedList, &hashedStatus))
		assert.Equal(t, left.ID, hashedList, "only the list they opted out of")
		assert.Equal(t, "unsubscribed", hashedStatus)

		// Back through an import, the address padded and in capitals: the opt-out returns on
		// its list, quietly, and the hash goes.
		importContact(t, "  "+strings.ToUpper(email)+" ", left.ID, kept.ID)
		assert.Equal(t, "unsubscribed", listStatus(t, email, left.ID), "the import cannot subscribe them again")
		assert.Equal(t, "active", listStatus(t, email, kept.ID))
		var remaining int
		require.NoError(t, db.QueryRowContext(ctx, `SELECT COUNT(*) FROM contact_opt_out_hashes WHERE email_hash = contact_opt_out_hash($1)`, email).Scan(&remaining))
		assert.Zero(t, remaining, "the hash is deleted once restored")
		kinds := timelineKinds(t, email)
		assert.Contains(t, kinds, "list.opt_out_restored")
		assert.NotContains(t, kinds, "list.unsubscribed", "nothing listening for list.unsubscribed runs again")
		var unsubscribedDeliveries int
		require.NoError(t, db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM webhook_deliveries WHERE event_type = 'list.unsubscribed' AND payload->>'email' = $1`, email).Scan(&unsubscribedDeliveries))
		assert.Zero(t, unsubscribedDeliveries, "subscribers are not told again")
	})

	// Deleting the segment memberships writes segment.left, which starts the automations
	// listening for it: the journeys must go after that step, or the erased contact is
	// enrolled again under their address.
	t.Run("a segment.left automation does not enrol the contact being deleted", func(t *testing.T) {
		segment, err := factory.CreateSegment(workspace.ID)
		require.NoError(t, err)
		email := fmt.Sprintf("left-%s@example.com", strings.ToLower(uuid.New().String()[:8]))
		_, err = factory.CreateContact(workspace.ID, testutil.WithContactEmail(email))
		require.NoError(t, err)

		automationID, triggerID := uuid.New().String()[:20], uuid.New().String()[:20]
		resp, err := client.CreateAutomation(map[string]interface{}{
			"workspace_id": workspace.ID,
			"automation": map[string]interface{}{
				"id": automationID, "workspace_id": workspace.ID, "name": "On segment left", "status": "draft",
				"trigger":      map[string]interface{}{"event_kind": "segment.left", "segment_id": segment.ID, "frequency": "once"},
				"root_node_id": triggerID,
				"nodes": []map[string]interface{}{{"id": triggerID, "automation_id": automationID, "type": "trigger",
					"config": map[string]interface{}{}, "position": map[string]interface{}{"x": 0, "y": 0}}},
				"stats": map[string]interface{}{"enrolled": 0, "completed": 0, "exited": 0, "failed": 0},
			},
		})
		require.NoError(t, err)
		require.Equal(t, http.StatusCreated, resp.StatusCode)
		_ = resp.Body.Close()
		resp, err = client.ActivateAutomation(map[string]interface{}{"workspace_id": workspace.ID, "automation_id": automationID})
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		_ = resp.Body.Close()

		_, err = db.ExecContext(ctx, `INSERT INTO contact_segments (email, segment_id, version, matched_at) VALUES ($1, $2, 1, NOW())`, email, segment.ID)
		require.NoError(t, err)

		deleteContact(t, email, false)

		var journeys, logs int
		require.NoError(t, db.QueryRowContext(ctx, `SELECT COUNT(*) FROM contact_automations WHERE contact_email = $1`, email).Scan(&journeys))
		require.NoError(t, db.QueryRowContext(ctx, `SELECT COUNT(*) FROM automation_trigger_log WHERE contact_email = $1`, email).Scan(&logs))
		assert.Zero(t, journeys)
		assert.Zero(t, logs)
		assert.Empty(t, scanWorkspaceForText(t, db, email))
	})

	t.Run("every way an address returns restores its opt-outs", func(t *testing.T) {
		for _, path := range []string{"contacts.upsert", "lists.subscribe", "custom event"} {
			t.Run(path, func(t *testing.T) {
				left, err := factory.CreateList(workspace.ID)
				require.NoError(t, err)
				kept, err := factory.CreateList(workspace.ID)
				require.NoError(t, err)
				email := newOptedOutContact(t, "return", left, kept)
				deleteContact(t, email, false)
				require.Equal(t, 1, countHashesFor(t, db, email))

				mixed := "  " + strings.ToUpper(email[:1]) + email[1:] + " "
				switch path {
				case "contacts.upsert":
					code, raw := post(t, "/api/contacts.upsert", map[string]interface{}{
						"workspace_id": workspace.ID, "contact": map[string]interface{}{"email": strings.TrimSpace(mixed)},
					})
					require.Equal(t, http.StatusOK, code, raw)
				case "lists.subscribe":
					code, raw := post(t, "/api/lists.subscribe", map[string]interface{}{
						"workspace_id": workspace.ID, "contact": map[string]interface{}{"email": strings.TrimSpace(mixed)},
						"list_ids": []string{left.ID, kept.ID},
					})
					require.Equal(t, http.StatusOK, code, raw)
					assert.Equal(t, "active", listStatus(t, email, kept.ID))
				case "custom event":
					code, raw := post(t, "/api/customEvents.upsert", map[string]interface{}{
						"workspace_id": workspace.ID, "email": strings.TrimSpace(mixed), "event_name": "came_back",
						"external_id": uuid.New().String(),
					})
					require.Equal(t, http.StatusOK, code, raw)
				}
				assert.Equal(t, "unsubscribed", listStatus(t, email, left.ID))
				assert.Zero(t, countHashesFor(t, db, email))
			})
		}
	})

	t.Run("forget_opt_outs keeps no hash", func(t *testing.T) {
		left, err := factory.CreateList(workspace.ID)
		require.NoError(t, err)
		kept, err := factory.CreateList(workspace.ID)
		require.NoError(t, err)
		email := newOptedOutContact(t, "forget", left, kept)
		before := hashCount(t)
		deleteContact(t, email, true)
		assert.Equal(t, before, hashCount(t))

		importContact(t, email, left.ID)
		assert.Equal(t, "active", listStatus(t, email, left.ID), "nothing was remembered")
	})

	t.Run("an opt-out on a list deleted since is not restored, and the purge drops it", func(t *testing.T) {
		left, err := factory.CreateList(workspace.ID)
		require.NoError(t, err)
		kept, err := factory.CreateList(workspace.ID)
		require.NoError(t, err)
		email := newOptedOutContact(t, "gone", left, kept)
		deleteContact(t, email, false)
		require.Equal(t, 1, countHashesFor(t, db, email))

		code, raw := post(t, "/api/lists.delete", map[string]interface{}{"workspace_id": workspace.ID, "id": left.ID})
		require.Equal(t, http.StatusOK, code, raw)

		runRetentionWorker(ctx)
		assert.Zero(t, countHashesFor(t, db, email), "the hash of a deleted list is purged")

		importContact(t, email)
		assert.Equal(t, "none", listStatus(t, email, left.ID))
	})

	t.Run("the purge keeps a hash younger than three years and drops an older one", func(t *testing.T) {
		left, err := factory.CreateList(workspace.ID)
		require.NoError(t, err)
		kept, err := factory.CreateList(workspace.ID)
		require.NoError(t, err)
		young := newOptedOutContact(t, "young", left, kept)
		old := newOptedOutContact(t, "old", left, kept)
		deleteContact(t, young, false)
		deleteContact(t, old, false)
		_, err = db.ExecContext(ctx, `UPDATE contact_opt_out_hashes SET recorded_at = NOW() - INTERVAL '3 years 1 day' WHERE email_hash = contact_opt_out_hash($1)`, old)
		require.NoError(t, err)
		_, err = db.ExecContext(ctx, `UPDATE contact_opt_out_hashes SET recorded_at = NOW() - INTERVAL '3 years' + INTERVAL '1 day' WHERE email_hash = contact_opt_out_hash($1)`, young)
		require.NoError(t, err)

		runRetentionWorker(ctx)

		assert.Equal(t, 1, countHashesFor(t, db, young))
		assert.Zero(t, countHashesFor(t, db, old))
	})
}

func countHashesFor(t *testing.T, db *sql.DB, email string) int {
	t.Helper()
	var n int
	require.NoError(t, db.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM contact_opt_out_hashes WHERE email_hash = contact_opt_out_hash($1)`, email).Scan(&n))
	return n
}

// scanWorkspaceForText returns "table.column" for every text, varchar or JSON column of
// the workspace database that holds needle, case-insensitively. The contact.deleted and
// segment.left webhook deliveries are left out: the deletion queues them on purpose.
func scanWorkspaceForText(t *testing.T, db *sql.DB, needle string) []string {
	t.Helper()
	ctx := context.Background()
	rows, err := db.QueryContext(ctx, `SELECT c.table_name, c.column_name
		FROM information_schema.columns c
		JOIN information_schema.tables tb ON tb.table_schema = c.table_schema AND tb.table_name = c.table_name
		WHERE c.table_schema = 'public' AND tb.table_type = 'BASE TABLE'
		AND c.data_type IN ('text', 'character varying', 'jsonb', 'json')
		ORDER BY c.table_name, c.column_name`)
	require.NoError(t, err)
	type column struct{ table, name string }
	var columns []column
	for rows.Next() {
		var c column
		require.NoError(t, rows.Scan(&c.table, &c.name))
		columns = append(columns, c)
	}
	require.NoError(t, rows.Close())
	require.NotEmpty(t, columns)

	var found []string
	for _, c := range columns {
		query := fmt.Sprintf(`SELECT COUNT(*) FROM %q WHERE %q::text ILIKE '%%' || $1 || '%%'`, c.table, c.name)
		if c.table == "webhook_deliveries" {
			query += ` AND event_type NOT IN ('contact.deleted', 'segment.left')`
		}
		var n int
		require.NoError(t, db.QueryRowContext(ctx, query, needle).Scan(&n), c.table+"."+c.name)
		if n > 0 {
			found = append(found, fmt.Sprintf("%s.%s (%d)", c.table, c.name, n))
		}
	}
	return found
}
