//go:build integration
// +build integration

package integration

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/database/schema"
	"github.com/Notifuse/notifuse/internal/migrations"
	"github.com/Notifuse/notifuse/tests/testutil"
)

// TestV45ExternalIDMigration puts a workspace back in its pre-45 state (no unique index on
// message_history.external_id, and duplicate keys), runs v45's workspace part the way the
// manager does, twice, and checks that empty keys become NULL, the oldest message of each key
// keeps it, the others give it up without being deleted, and the index is there, unique.
func TestV45ExternalIDMigration(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer func() { suite.Cleanup() }()

	factory := suite.DataFactory
	ctx := context.Background()

	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	db, err := factory.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)

	indexDefinition := func(t *testing.T) string {
		t.Helper()
		var definition sql.NullString
		require.NoError(t, db.QueryRowContext(ctx,
			`SELECT indexdef FROM pg_indexes WHERE tablename = 'message_history' AND indexname = $1`,
			schema.MessageHistoryExternalIDIndexName).Scan(&definition))
		return definition.String
	}
	runV45 := func(t *testing.T) {
		t.Helper()
		tx, err := db.BeginTx(ctx, nil)
		require.NoError(t, err)
		if err := (&migrations.V45Migration{}).UpdateWorkspace(ctx, &config.Config{}, workspace, tx); err != nil {
			_ = tx.Rollback()
			require.NoError(t, err)
		}
		require.NoError(t, tx.Commit())
	}

	t.Run("a new workspace has the index", func(t *testing.T) {
		assert.Equal(t,
			"CREATE UNIQUE INDEX idx_message_history_external_id ON public.message_history USING btree (external_id) WHERE (external_id IS NOT NULL)",
			indexDefinition(t))
	})

	// --- pre-45 state ------------------------------------------------------------------
	_, err = db.ExecContext(ctx, `DROP INDEX IF EXISTS `+schema.MessageHistoryExternalIDIndexName)
	require.NoError(t, err)

	// order-1 is held three times and order-2 twice; the oldest of each is the one created
	// first, not the lowest id. order-3 and the messages without a key are left alone. An
	// empty key is no key: every message stored with one gets NULL, the oldest included.
	_, err = db.ExecContext(ctx, `
		INSERT INTO message_history (id, external_id, contact_email, template_id, template_version, channel,
		                             message_data, sent_at, created_at, updated_at)
		VALUES
			('m-a', 'order-1', 'a@example.test', 'tpl', 1, 'email', '{}'::jsonb, NOW(), NOW() - INTERVAL '1 minute', NOW()),
			('m-b', 'order-1', 'a@example.test', 'tpl', 1, 'email', '{}'::jsonb, NOW(), NOW() - INTERVAL '3 minutes', NOW()),
			('m-c', 'order-1', 'a@example.test', 'tpl', 1, 'email', '{}'::jsonb, NOW(), NOW() - INTERVAL '2 minutes', NOW()),
			('m-d', 'order-2', 'b@example.test', 'tpl', 1, 'email', '{}'::jsonb, NOW(), NOW() - INTERVAL '5 minutes', NOW()),
			('m-e', 'order-2', 'b@example.test', 'tpl', 1, 'email', '{}'::jsonb, NOW(), NOW() - INTERVAL '5 minutes', NOW()),
			('m-f', 'order-3', 'c@example.test', 'tpl', 1, 'email', '{}'::jsonb, NOW(), NOW(), NOW()),
			('m-g', NULL,      'c@example.test', 'tpl', 1, 'email', '{}'::jsonb, NOW(), NOW(), NOW()),
			('m-h', NULL,      'c@example.test', 'tpl', 1, 'email', '{}'::jsonb, NOW(), NOW(), NOW()),
			('m-i', '',        'd@example.test', 'tpl', 1, 'email', '{}'::jsonb, NOW(), NOW() - INTERVAL '4 minutes', NOW()),
			('m-j', '',        'd@example.test', 'tpl', 1, 'email', '{}'::jsonb, NOW(), NOW(), NOW())`)
	require.NoError(t, err)

	type row struct {
		externalID sql.NullString
		updatedAt  time.Time
	}
	snapshot := func(t *testing.T) map[string]row {
		t.Helper()
		rows, err := db.QueryContext(ctx, `SELECT id, external_id, updated_at FROM message_history WHERE id LIKE 'm-%'`)
		require.NoError(t, err)
		defer func() { _ = rows.Close() }()
		out := map[string]row{}
		for rows.Next() {
			var id string
			var r row
			require.NoError(t, rows.Scan(&id, &r.externalID, &r.updatedAt))
			out[id] = r
		}
		require.NoError(t, rows.Err())
		return out
	}
	before := snapshot(t)

	t.Run("v45 resolves the duplicates, then builds the index", func(t *testing.T) {
		runV45(t)
		after := snapshot(t)
		require.Len(t, after, len(before), "no message is deleted")

		want := map[string]sql.NullString{
			"m-a": {},
			"m-b": {String: "order-1", Valid: true}, // created first
			"m-c": {},
			"m-d": {String: "order-2", Valid: true}, // same created_at as m-e, lower id
			"m-e": {},
			"m-f": {String: "order-3", Valid: true},
			"m-g": {},
			"m-h": {},
			"m-i": {}, // '' is no key, even on the oldest
			"m-j": {},
		}
		for id, externalID := range want {
			assert.Equal(t, externalID, after[id].externalID, id)
			assert.Equal(t, before[id].updatedAt, after[id].updatedAt, "%s: updated_at is left as it was", id)
		}
		assert.NotEmpty(t, indexDefinition(t))

		_, err := db.ExecContext(ctx, `UPDATE message_history SET external_id = 'order-1' WHERE id = 'm-a'`)
		require.Error(t, err, "a second message cannot take a held key")
		_, err = db.ExecContext(ctx, `UPDATE message_history SET external_id = NULL WHERE id = 'm-f'`)
		require.NoError(t, err, "messages without a key do not conflict")
		_, err = db.ExecContext(ctx, `UPDATE message_history SET external_id = 'order-3' WHERE id = 'm-f'`)
		require.NoError(t, err)
	})

	// The part lifts the timeouts for its build, then gives the transaction back the values it
	// had, so a part that runs after it keeps the settings an earlier one set.
	t.Run("the transaction's own timeouts are restored", func(t *testing.T) {
		tx, err := db.BeginTx(ctx, nil)
		require.NoError(t, err)
		defer func() { _ = tx.Rollback() }()
		_, err = tx.ExecContext(ctx, `SET LOCAL statement_timeout = '7s'`)
		require.NoError(t, err)
		_, err = tx.ExecContext(ctx, `SET LOCAL lock_timeout = '3s'`)
		require.NoError(t, err)

		require.NoError(t, (&migrations.V45Migration{}).UpdateWorkspace(ctx, &config.Config{}, workspace, tx))

		var statementTimeout, lockTimeout string
		require.NoError(t, tx.QueryRowContext(ctx, `SELECT current_setting('statement_timeout'), current_setting('lock_timeout')`).Scan(&statementTimeout, &lockTimeout))
		assert.Equal(t, "7s", statementTimeout)
		assert.Equal(t, "3s", lockTimeout)
	})

	t.Run("a second run changes nothing and waits for no lock", func(t *testing.T) {
		settled := snapshot(t)

		// A send in progress holds a write lock on message_history. The first run would wait
		// for it; with the index there the part takes no lock at all.
		writer, err := db.BeginTx(ctx, nil)
		require.NoError(t, err)
		defer func() { _ = writer.Rollback() }()
		_, err = writer.ExecContext(ctx, `
			INSERT INTO message_history (id, contact_email, template_id, template_version, channel, message_data,
			                             sent_at, created_at, updated_at)
			VALUES ('m-inflight', 'd@example.test', 'tpl', 1, 'email', '{}'::jsonb, NOW(), NOW(), NOW())`)
		require.NoError(t, err)

		started := time.Now()
		runV45(t)
		assert.Less(t, time.Since(started), 4*time.Second, "the rerun did not wait for the lock_timeout")
		require.NoError(t, writer.Rollback())

		assert.Equal(t, settled, snapshot(t))
		assert.NotEmpty(t, indexDefinition(t))
	})
}
