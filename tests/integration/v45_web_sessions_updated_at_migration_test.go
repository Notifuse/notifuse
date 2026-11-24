//go:build integration
// +build integration

package integration

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/database/schema"
	"github.com/Notifuse/notifuse/internal/migrations"
	"github.com/Notifuse/notifuse/tests/testutil"
)

// TestV45WebSessionsUpdatedAtIndexMigration puts a workspace back in its pre-45 state (no
// index on web_sessions.updated_at), runs v45's workspace part the way the manager does, and
// checks that it only creates the parent index, invalid, without touching a partition; that
// the maintenance worker's build then attaches an index to every partition, without holding
// writes back, until the parent index is valid; that the live views' predicate uses it; that
// a heartbeat update stays HOT; and that reruns change nothing.
func TestV45WebSessionsUpdatedAtIndexMigration(t *testing.T) {
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
	repo := suite.ServerManager.GetApp().GetWebAnalyticsRepository()

	now := time.Now().UTC()
	partition := schema.WebAnalyticsPartitionName("web_sessions", now)

	// parentIndex returns how many partitions carry an index attached to the parent index, and
	// whether the parent index is valid; -1 when it does not exist.
	parentIndex := func(t *testing.T) (int, bool) {
		t.Helper()
		var exists bool
		require.NoError(t, db.QueryRowContext(ctx, `SELECT to_regclass($1) IS NOT NULL`, schema.WebSessionsUpdatedAtIndexName).Scan(&exists))
		if !exists {
			return -1, false
		}
		var children int
		var valid bool
		require.NoError(t, db.QueryRowContext(ctx, `
			SELECT (SELECT COUNT(*) FROM pg_inherits WHERE inhparent = $1::regclass), indisvalid
			FROM pg_index WHERE indexrelid = $1::regclass`, schema.WebSessionsUpdatedAtIndexName).Scan(&children, &valid))
		return children, valid
	}
	partitionCount := func(t *testing.T) int {
		t.Helper()
		var count int
		require.NoError(t, db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM pg_inherits WHERE inhparent = 'web_sessions'::regclass`).Scan(&count))
		return count
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

	t.Run("a new workspace has the whole index at once", func(t *testing.T) {
		require.Positive(t, partitionCount(t))
		children, valid := parentIndex(t)
		assert.Equal(t, partitionCount(t), children)
		assert.True(t, valid)
	})

	// --- pre-45 state ------------------------------------------------------------------
	_, err = db.ExecContext(ctx, `DROP INDEX IF EXISTS `+schema.WebSessionsUpdatedAtIndexName)
	require.NoError(t, err)
	children, _ := parentIndex(t)
	require.Equal(t, -1, children)

	sessionID := waUUIDv7At(now, 0x41)
	_, err = db.ExecContext(ctx, `
		INSERT INTO web_sessions (session_date, id, beat_seq, created_at, updated_at, duration_ms, pageview_count)
		VALUES ($1, $2, 1, $3, $3, 1000, 1)`, now.Format("2006-01-02"), sessionID, now)
	require.NoError(t, err)

	t.Run("v45 creates the parent index alone, invalid, and locks no partition", func(t *testing.T) {
		tx, err := db.BeginTx(ctx, nil)
		require.NoError(t, err)
		require.NoError(t, (&migrations.V45Migration{}).UpdateWorkspace(ctx, &config.Config{}, workspace, tx))
		// Before the commit, the transaction holds no lock on any partition of web_sessions:
		// it read none of them.
		var partitionLocks int
		require.NoError(t, tx.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM pg_locks l
			JOIN pg_inherits i ON i.inhrelid = l.relation AND i.inhparent = 'web_sessions'::regclass
			WHERE l.pid = pg_backend_pid()`).Scan(&partitionLocks))
		assert.Zero(t, partitionLocks)
		require.NoError(t, tx.Commit())

		children, valid := parentIndex(t)
		assert.Equal(t, 0, children)
		assert.False(t, valid, "the planner ignores it until every partition has its index")
	})

	t.Run("a partition created afterwards gets its index at creation", func(t *testing.T) {
		later := now.AddDate(2, 0, 0)
		_, err := db.ExecContext(ctx, schema.WebAnalyticsPartitionDDL("web_sessions", later))
		require.NoError(t, err)
		var attached bool
		require.NoError(t, db.QueryRowContext(ctx, `
			SELECT EXISTS (SELECT 1 FROM pg_inherits x JOIN pg_index i ON i.indexrelid = x.inhrelid
			WHERE x.inhparent = $1::regclass AND i.indrelid = $2::regclass)`,
			schema.WebSessionsUpdatedAtIndexName, schema.WebAnalyticsPartitionName("web_sessions", later)).Scan(&attached))
		assert.True(t, attached)
	})

	t.Run("the maintenance build attaches every partition without holding writes back", func(t *testing.T) {
		// A heartbeat transaction in progress: CREATE INDEX CONCURRENTLY waits for it to end,
		// and meanwhile another session's write goes through.
		writer, err := db.BeginTx(ctx, nil)
		require.NoError(t, err)
		defer func() { _ = writer.Rollback() }()
		_, err = writer.ExecContext(ctx, `UPDATE web_sessions SET beat_seq = beat_seq + 1 WHERE session_date = $1 AND id = $2`,
			now.Format("2006-01-02"), sessionID)
		require.NoError(t, err)

		type result struct {
			valid bool
			err   error
		}
		done := make(chan result, 1)
		go func() {
			valid, err := repo.BuildWebSessionsUpdatedAtIndex(ctx, workspace.ID)
			done <- result{valid, err}
		}()
		require.Eventually(t, func() bool {
			var building bool
			_ = db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM pg_stat_activity
				WHERE query LIKE 'CREATE INDEX CONCURRENTLY%' AND wait_event_type = 'Lock')`).Scan(&building)
			return building
		}, 10*time.Second, 50*time.Millisecond, "the build waits for the heartbeat in progress")

		insertCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		other := waUUIDv7At(now.Add(time.Second), 0x42)
		_, err = db.ExecContext(insertCtx, `
			INSERT INTO web_sessions (session_date, id, beat_seq, created_at, updated_at, duration_ms, pageview_count)
			VALUES ($1, $2, 1, $3, $3, 1000, 1)`, now.Format("2006-01-02"), other, now)
		require.NoError(t, err, "a new session is written during the build")

		require.NoError(t, writer.Commit())
		select {
		case r := <-done:
			require.NoError(t, r.err)
			assert.True(t, r.valid)
		case <-time.After(30 * time.Second):
			t.Fatal("the build did not finish")
		}
		children, valid := parentIndex(t)
		assert.Equal(t, partitionCount(t), children)
		assert.True(t, valid)
	})

	t.Run("the live views' predicate uses it", func(t *testing.T) {
		tx, err := db.BeginTx(ctx, nil)
		require.NoError(t, err)
		defer func() { _ = tx.Rollback() }()
		_, err = tx.ExecContext(ctx, `SET LOCAL enable_seqscan = off`)
		require.NoError(t, err)
		rows, err := tx.QueryContext(ctx, `EXPLAIN SELECT COUNT(*) FROM web_sessions WHERE updated_at >= $1 AND updated_at <= $2`,
			now.Add(-5*time.Minute), now.Add(time.Minute))
		require.NoError(t, err)
		var plan []string
		for rows.Next() {
			var line string
			require.NoError(t, rows.Scan(&line))
			plan = append(plan, line)
		}
		require.NoError(t, rows.Err())
		_ = rows.Close()
		assert.Contains(t, strings.Join(plan, "\n"), "Bitmap Index Scan on "+schema.WebSessionsUpdatedAtPartitionIndexName(partition))
	})

	t.Run("a heartbeat update stays HOT", func(t *testing.T) {
		tx, err := db.BeginTx(ctx, nil)
		require.NoError(t, err)
		defer func() { _ = tx.Rollback() }()
		_, err = tx.ExecContext(ctx, `
			UPDATE web_sessions SET beat_seq = beat_seq + 1, updated_at = updated_at + INTERVAL '10 seconds', duration_ms = 11000
			WHERE session_date = $1 AND id = $2`, now.Format("2006-01-02"), sessionID)
		require.NoError(t, err)
		var updates, hot int64
		require.NoError(t, tx.QueryRowContext(ctx,
			`SELECT n_tup_upd, n_tup_hot_upd FROM pg_stat_xact_user_tables WHERE relname = $1`, partition).Scan(&updates, &hot))
		assert.Equal(t, int64(1), updates)
		assert.Equal(t, int64(1), hot, "a BRIN index leaves the update HOT")
	})

	t.Run("reruns change nothing and wait for no lock", func(t *testing.T) {
		// A heartbeat in progress holds a write lock on web_sessions. With the index there,
		// neither v45 nor the build waits for it.
		writer, err := db.BeginTx(ctx, nil)
		require.NoError(t, err)
		defer func() { _ = writer.Rollback() }()
		_, err = writer.ExecContext(ctx, `UPDATE web_sessions SET beat_seq = beat_seq + 1 WHERE session_date = $1 AND id = $2`,
			now.Format("2006-01-02"), sessionID)
		require.NoError(t, err)

		started := time.Now()
		runV45(t)
		valid, err := repo.BuildWebSessionsUpdatedAtIndex(ctx, workspace.ID)
		require.NoError(t, err)
		assert.True(t, valid)
		assert.Less(t, time.Since(started), 4*time.Second, "neither waited for the lock")
		require.NoError(t, writer.Rollback())

		children, valid := parentIndex(t)
		assert.Equal(t, partitionCount(t), children)
		assert.True(t, valid)
	})
}
