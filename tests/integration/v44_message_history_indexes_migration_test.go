//go:build integration
// +build integration

package integration

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/migrations"
	"github.com/Notifuse/notifuse/tests/testutil"
)

// v44 gives a workspace that ran 43 the partial indexes the dashboard's "Recent failures"
// card reads, and messages.list?is_<flag>=true&limit=5 walks them instead of the whole table.
func TestV44MessageHistoryIndexesMigration(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer func() { suite.Cleanup() }()
	factory := suite.DataFactory
	ctx := context.Background()

	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	workspaceDB, err := factory.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)

	flags := map[string]string{
		"idx_message_history_failed_recent":     "failed_at",
		"idx_message_history_bounced_recent":    "bounced_at",
		"idx_message_history_complained_recent": "complained_at",
	}

	indexPresent := func(t *testing.T, name string) bool {
		t.Helper()
		var n int
		require.NoError(t, workspaceDB.QueryRowContext(ctx,
			`SELECT count(*) FROM pg_indexes WHERE tablename = 'message_history' AND indexname = $1`, name).Scan(&n))
		return n == 1
	}

	t.Run("a new workspace has them", func(t *testing.T) {
		for name := range flags {
			assert.True(t, indexPresent(t, name), name)
		}
	})

	// Rows of every kind, a few of each flag, so the planner has a table to choose on.
	_, err = workspaceDB.ExecContext(ctx, `
		INSERT INTO message_history (id, contact_email, template_id, template_version, channel, message_data,
		                             sent_at, created_at, updated_at, failed_at, bounced_at, complained_at)
		SELECT 'v44-' || g, 'v44-' || g || '@example.test', 'tpl', 1, 'email', '{}'::jsonb,
		       NOW() - g * INTERVAL '1 minute', NOW() - g * INTERVAL '1 minute', NOW(),
		       CASE WHEN g % 97 = 0 THEN NOW() END,
		       CASE WHEN g % 89 = 0 THEN NOW() END,
		       CASE WHEN g % 83 = 0 THEN NOW() END
		FROM generate_series(1, 2000) AS g`)
	require.NoError(t, err)

	t.Run("v44 creates them on a workspace that ran 43, and runs twice", func(t *testing.T) {
		for name := range flags {
			_, err := workspaceDB.ExecContext(ctx, "DROP INDEX IF EXISTS "+name)
			require.NoError(t, err)
			require.False(t, indexPresent(t, name))
		}

		for run := 1; run <= 2; run++ {
			tx, err := workspaceDB.BeginTx(ctx, nil)
			require.NoError(t, err)
			require.NoError(t, (&migrations.V44Migration{}).UpdateWorkspace(ctx, &config.Config{}, workspace, tx), "run %d", run)
			require.NoError(t, tx.Commit())
		}

		for name := range flags {
			assert.True(t, indexPresent(t, name), name)
		}
	})

	t.Run("the recent failures query of each flag reads its partial index", func(t *testing.T) {
		_, err := workspaceDB.ExecContext(ctx, "ANALYZE message_history")
		require.NoError(t, err)

		for name, column := range flags {
			// What ListMessages builds for is_<flag>=true&limit=5 (it fetches limit+1 rows).
			query := fmt.Sprintf(`EXPLAIN SELECT id, external_id, contact_email, broadcast_id, automation_id,
				transactional_notification_id, list_id, template_id, template_version, channel, status_info,
				message_data, channel_options, attachments, sent_at, delivered_at, failed_at, opened_at,
				clicked_at, bounced_at, complained_at, unsubscribed_at, created_at, updated_at, opens_tracked,
				clicks_tracked
				FROM message_history WHERE %s IS NOT NULL ORDER BY created_at DESC, id DESC LIMIT 6`, column)

			tx, err := workspaceDB.BeginTx(ctx, nil)
			require.NoError(t, err)
			_, err = tx.ExecContext(ctx, "SET LOCAL enable_seqscan = off")
			require.NoError(t, err)
			rows, err := tx.QueryContext(ctx, query)
			require.NoError(t, err)
			var plan []string
			for rows.Next() {
				var line string
				require.NoError(t, rows.Scan(&line))
				plan = append(plan, line)
			}
			require.NoError(t, rows.Err())
			_ = rows.Close()
			require.NoError(t, tx.Rollback())

			assert.Contains(t, strings.Join(plan, "\n"), name, "%s plan:\n%s", column, strings.Join(plan, "\n"))
		}
	})
}
