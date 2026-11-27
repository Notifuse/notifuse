//go:build integration
// +build integration

package integration

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/database/schema"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/migrations"
	"github.com/Notifuse/notifuse/tests/testutil"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// pre45Bodies are the three function bodies as 44 shipped them: today's without the
// erased-contact early return and without the opt-out restore branches.
func pre45Bodies(t *testing.T) []string {
	t.Helper()
	timeline := regexp.MustCompile(`(?s)\n\s*-- An opt-out put back on a returning contact \(restore_contact_opt_outs\).*?END IF;\n`)
	webhook := regexp.MustCompile(`(?s)\n\s*-- An opt-out put back on a returning contact is not news.*?END IF;\n`)
	trackLists := schema.TrackContactListChangesFunction()
	webhookLists := schema.WebhookContactListsTriggerFunction()
	trackMessages := schema.TrackMessageHistoryChangesFunction()
	require.Regexp(t, timeline, trackLists)
	require.Regexp(t, webhook, webhookLists)
	require.Contains(t, trackMessages, schema.ErasedContactStep)
	// The retention engine's purge branches (no segment.left, an address-only contact.deleted).
	segmentTimeline := regexp.MustCompile(`(?s)\n\s*-- A contact erased by a retention purge does not leave a segment\.\n\s*IF current_setting\('notifuse\.retention_purge', true\) = 'on' THEN\n\s*RETURN OLD;\n\s*END IF;`)
	trackSegments := schema.TrackContactSegmentChangesFunction()
	webhookSegments := schema.WebhookContactSegmentsTriggerFunction()
	require.Regexp(t, segmentTimeline, trackSegments)
	require.Regexp(t, segmentTimeline, webhookSegments)
	webhookContacts := schema.WebhookContactsTriggerFunction()
	purgePayload := regexp.MustCompile(`(?s)IF TG_OP = 'DELETE' AND current_setting\('notifuse\.retention_purge', true\) = 'on' THEN\n.*?ELSE\n(\s*payload := jsonb_build_object\(\n\s*'contact', to_jsonb\(contact_record\)\n\s*\);)\n\s*END IF;`)
	require.Regexp(t, purgePayload, webhookContacts)
	return []string{
		timeline.ReplaceAllString(trackLists, "\n"),
		webhook.ReplaceAllString(webhookLists, "\n"),
		strings.Replace(trackMessages, schema.ErasedContactStep, "", 1),
		segmentTimeline.ReplaceAllString(trackSegments, ""),
		segmentTimeline.ReplaceAllString(webhookSegments, ""),
		purgePayload.ReplaceAllString(webhookContacts, "$1"),
	}
}

// TestV45DataRetentionMigration puts a workspace back in its pre-45 state (no opt-out
// hashes, key, hash function or restore trigger, and the 44 bodies of the three functions),
// runs v45's workspace part the way the manager does, twice, and checks that the workspace
// then has the objects and function bodies of a workspace created at 45, and behaves as one.
func TestV45DataRetentionMigration(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer func() { suite.Cleanup() }()

	factory := suite.DataFactory
	client := suite.APIClient
	bg := context.Background()

	user, err := factory.CreateUser()
	require.NoError(t, err)
	migrated, err := factory.CreateWorkspace()
	require.NoError(t, err)
	fresh, err := factory.CreateWorkspace()
	require.NoError(t, err)
	require.NoError(t, factory.AddUserToWorkspace(user.ID, migrated.ID, "owner"))
	require.NoError(t, client.Login(user.Email, "password"))
	client.SetWorkspaceID(migrated.ID)
	db, err := factory.GetWorkspaceDB(migrated.ID)
	require.NoError(t, err)
	freshDB, err := factory.GetWorkspaceDB(fresh.ID)
	require.NoError(t, err)

	// --- pre-45 state ----------------------------------------------------------------
	for _, statement := range append([]string{
		`DROP TRIGGER IF EXISTS ` + schema.ContactOptOutRestoreTrigger + ` ON contacts`,
		`DROP FUNCTION IF EXISTS restore_contact_opt_outs()`,
		`DROP FUNCTION IF EXISTS contact_opt_out_hash(TEXT)`,
		`DROP TABLE IF EXISTS contact_opt_out_hashes`,
		`DROP TABLE IF EXISTS contact_opt_out_hash_key`,
		`DROP TABLE IF EXISTS contact_engagement`,
		`DROP TABLE IF EXISTS data_retention_state`,
		`DROP FUNCTION IF EXISTS contact_retention_expires_at(TEXT)`,
		`DROP TABLE IF EXISTS data_retention_runs`,
		`DROP TABLE IF EXISTS data_retention_erasure_queue`,
	}, pre45Bodies(t)...) {
		_, err := db.ExecContext(bg, statement)
		require.NoError(t, err, statement)
	}
	require.False(t, tableExists(t, db, "contact_opt_out_hashes"))

	// --- v45, twice, each run in a transaction as the manager runs it ----------------
	migration, ok := migrations.GetRegisteredMigration(45.0)
	require.True(t, ok)
	for run := 1; run <= 2; run++ {
		tx, err := db.BeginTx(bg, nil)
		require.NoError(t, err)
		require.NoError(t, migration.UpdateWorkspace(bg, &config.Config{}, &domain.Workspace{ID: migrated.ID}, tx), "run %d", run)
		require.NoError(t, tx.Commit())
	}

	// --- the same objects as a workspace created at 45 --------------------------------
	for _, table := range []string{"contact_opt_out_hashes", "contact_opt_out_hash_key", "contact_engagement", "data_retention_state",
		"data_retention_runs", "data_retention_erasure_queue"} {
		assert.True(t, tableExists(t, db, table), table)
	}

	// The engagement clock: a migrated workspace waits for its backfill, a new one is ready.
	assert.Equal(t, "not ready", retentionMarker(t, db))
	assert.Equal(t, "ready", retentionMarker(t, freshDB))

	// --- v45's system part, twice: one backfill task per workspace, never two ---------
	// The first run happens on a session 14 hours ahead of UTC: the task's dates must still
	// be UTC, which the scheduler compares them with, or the task would wait 14 hours.
	systemDB := suite.DBManager.GetDB()
	for run := 1; run <= 2; run++ {
		tx, err := systemDB.BeginTx(bg, nil)
		require.NoError(t, err)
		if run == 1 {
			_, err = tx.ExecContext(bg, `SET LOCAL TimeZone = 'Pacific/Kiritimati'`)
			require.NoError(t, err)
		}
		require.NoError(t, migration.UpdateSystem(bg, &config.Config{}, tx), "run %d", run)
		require.NoError(t, tx.Commit())
	}
	for _, ws := range []string{migrated.ID, fresh.ID} {
		var tasks int
		require.NoError(t, systemDB.QueryRowContext(bg, `SELECT COUNT(*) FROM tasks WHERE workspace_id = $1 AND type = $2`,
			ws, domain.DataRetentionBackfillTaskType).Scan(&tasks))
		assert.Equal(t, 1, tasks, ws)
		var due, createdUTC bool
		require.NoError(t, systemDB.QueryRowContext(bg, `SELECT
				next_run_after <= (NOW() AT TIME ZONE 'UTC') + INTERVAL '1 minute',
				created_at BETWEEN (NOW() AT TIME ZONE 'UTC') - INTERVAL '10 minutes' AND (NOW() AT TIME ZONE 'UTC') + INTERVAL '1 minute'
			FROM tasks WHERE workspace_id = $1 AND type = $2`, ws, domain.DataRetentionBackfillTaskType).Scan(&due, &createdUTC))
		assert.True(t, due, "%s: the backfill task is due now in UTC", ws)
		assert.True(t, createdUTC, "%s: the backfill task's dates are UTC", ws)
	}
	var keys int
	require.NoError(t, db.QueryRowContext(bg, `SELECT COUNT(*) FROM contact_opt_out_hash_key WHERE octet_length(key) = 32`).Scan(&keys))
	assert.Equal(t, 1, keys, "one 32-byte key, generated once")
	for _, function := range []string{"contact_opt_out_hash", "restore_contact_opt_outs", "track_contact_list_changes",
		"webhook_contact_lists_trigger", "track_message_history_changes",
		"track_contact_segment_changes", "webhook_contact_segments_trigger", "webhook_contacts_trigger",
		"contact_retention_expires_at"} {
		assert.Equal(t, functionDefinition(t, freshDB, function), functionDefinition(t, db, function), function)
	}
	var triggers int
	require.NoError(t, db.QueryRowContext(bg,
		`SELECT COUNT(*) FROM pg_trigger WHERE tgname = $1 AND NOT tgisinternal`, schema.ContactOptOutRestoreTrigger).Scan(&triggers))
	assert.Equal(t, 1, triggers)

	// The two workspaces hash with different keys.
	var same bool
	var freshKey []byte
	require.NoError(t, freshDB.QueryRowContext(bg, `SELECT key FROM contact_opt_out_hash_key`).Scan(&freshKey))
	require.NoError(t, db.QueryRowContext(bg, `SELECT key = $1 FROM contact_opt_out_hash_key`, freshKey).Scan(&same))
	assert.False(t, same, "each workspace has its own key")

	// --- and it behaves as one: a deleted contact's unsubscribe comes back -----------
	list, err := factory.CreateList(migrated.ID)
	require.NoError(t, err)
	email := fmt.Sprintf("v45-%s@example.com", strings.ToLower(uuid.New().String()[:8]))
	_, err = factory.CreateContact(migrated.ID, testutil.WithContactEmail(email))
	require.NoError(t, err)
	_, err = factory.CreateContactList(migrated.ID, testutil.WithContactListEmail(email),
		testutil.WithContactListListID(list.ID), testutil.WithContactListStatus(domain.ContactListStatusUnsubscribed))
	require.NoError(t, err)

	resp, err := client.Post("/api/contacts.delete", map[string]interface{}{"workspace_id": migrated.ID, "email": email})
	require.NoError(t, err)
	_ = resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp, err = client.Post("/api/contacts.upsert", map[string]interface{}{
		"workspace_id": migrated.ID, "contact": map[string]interface{}{"email": email},
	})
	require.NoError(t, err)
	_ = resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var status string
	require.NoError(t, db.QueryRowContext(bg, `SELECT status FROM contact_lists WHERE email = $1 AND list_id = $2`, email, list.ID).Scan(&status))
	assert.Equal(t, "unsubscribed", status)
}

// retentionMarker reads data_retention_state: "missing", "not ready" or "ready".
func retentionMarker(t *testing.T, db *sql.DB) string {
	t.Helper()
	var done sql.NullBool
	err := db.QueryRowContext(context.Background(), `SELECT backfill_completed_at IS NOT NULL FROM data_retention_state WHERE id = 1`).Scan(&done)
	if err == sql.ErrNoRows {
		return "missing"
	}
	require.NoError(t, err)
	if done.Bool {
		return "ready"
	}
	return "not ready"
}

func tableExists(t *testing.T, db *sql.DB, name string) bool {
	t.Helper()
	var exists bool
	require.NoError(t, db.QueryRowContext(context.Background(), `SELECT to_regclass($1) IS NOT NULL`, name).Scan(&exists))
	return exists
}

func functionDefinition(t *testing.T, db *sql.DB, name string) string {
	t.Helper()
	var def string
	require.NoError(t, db.QueryRowContext(context.Background(),
		`SELECT pg_get_functiondef(p.oid) FROM pg_proc p JOIN pg_namespace n ON n.oid = p.pronamespace
		WHERE p.proname = $1 AND n.nspname = 'public'`, name).Scan(&def), name)
	return def
}
