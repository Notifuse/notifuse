//go:build integration
// +build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/database/schema"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/repository"
	"github.com/Notifuse/notifuse/internal/service"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/Notifuse/notifuse/tests/testutil"
)

func TestWebAnalyticsMaintenanceEndToEnd(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer func() { suite.Cleanup() }()

	workspace, err := suite.DataFactory.CreateWorkspace(func(w *domain.Workspace) {
		w.Settings.WebAnalytics = &domain.WebAnalyticsSettings{Enabled: true}
	})
	require.NoError(t, err)

	wsDB, err := suite.DBManager.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)

	now := time.Now().UTC()
	currentMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)

	// A partition far outside every window the worker touches. Expiring history
	// is the operator's call, so maintenance must leave it alone.
	oldMonth := currentMonth.AddDate(0, -3, 0)
	_, err = wsDB.Exec(schema.WebAnalyticsPartitionDDL("web_sessions", oldMonth))
	require.NoError(t, err)
	oldPartition := schema.WebAnalyticsPartitionName("web_sessions", oldMonth)

	partitionExists := func(name string) bool {
		var count int
		require.NoError(t, wsDB.QueryRow(
			`SELECT COUNT(*) FROM pg_class WHERE relname = $1`, name).Scan(&count))
		return count > 0
	}
	require.True(t, partitionExists(oldPartition))

	workspaceRepo := suite.ServerManager.GetApp().GetWorkspaceRepository()
	webRepo := repository.NewWebAnalyticsRepository(workspaceRepo, logger.NewLogger())
	worker := service.NewWebAnalyticsMaintenanceWorker(workspaceRepo, webRepo, logger.NewLogger())

	worker.RunOnce(context.Background())

	assert.True(t, partitionExists(oldPartition), "maintenance must never drop history")
	for _, table := range schema.WebAnalyticsTableNames {
		assert.True(t, partitionExists(schema.WebAnalyticsPartitionName(table, currentMonth)), "current month partition")
		assert.True(t, partitionExists(schema.WebAnalyticsPartitionName(table, currentMonth.AddDate(0, 1, 0))), "next month partition")
	}

	// A second pass is a no-op and must not error (idempotency).
	worker.RunOnce(context.Background())
	assert.True(t, partitionExists(oldPartition))
}

// TestWebAnalyticsIngestCreatesMissingPartition pins the self-healing half of
// the partition story: workspace init and the maintenance worker only ever
// bootstrap the current and next month, so a batch dated anywhere else — a
// session that began before midnight on the first of a month, a replay, a
// backdated import — reaches an insert with no partition to land in.
//
// FlushBatch is supposed to catch that, create the months the batch actually
// spans, and retry. Until now that was asserted only against sqlmock, with a
// hand-written pq.Error: the test proved the retry fires for the SQLSTATE the
// code expects, not that PostgreSQL emits that SQLSTATE. This runs it against a
// real database with a real missing partition.
func TestWebAnalyticsIngestCreatesMissingPartition(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer func() { suite.Cleanup() }()

	workspace, err := suite.DataFactory.CreateWorkspace(func(w *domain.Workspace) {
		w.Settings.WebAnalytics = &domain.WebAnalyticsSettings{Enabled: true, Filters: domain.DefaultWebFilters()}
	})
	require.NoError(t, err)

	wsDB, err := suite.DBManager.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)
	repo := suite.ServerManager.GetApp().GetWebAnalyticsRepository()
	ctx := context.Background()

	// Two months back: outside every window anything bootstraps.
	past := time.Now().UTC().AddDate(0, -2, 0)
	pastMonth := time.Date(past.Year(), past.Month(), 1, 0, 0, 0, 0, time.UTC)
	sessionDate := pastMonth.AddDate(0, 0, 14)
	visitedAt := sessionDate.Add(12 * time.Hour)

	for _, table := range schema.WebAnalyticsTableNames {
		var exists bool
		require.NoError(t, wsDB.QueryRow(`SELECT EXISTS (SELECT 1 FROM pg_class WHERE relname = $1)`,
			schema.WebAnalyticsPartitionName(table, pastMonth)).Scan(&exists))
		require.False(t, exists, "precondition: no %s partition for the target month", table)
	}

	email := "boundary@example.com"
	sessionID := waUUIDv7At(visitedAt, 0xE1)
	require.NoError(t, repo.FlushBatch(ctx, workspace.ID,
		[]*domain.WebSession{{
			SessionDate: sessionDate, ID: sessionID, ContactEmail: &email,
			PageviewCount: 1, DurationMs: 1000, CreatedAt: visitedAt, UpdatedAt: visitedAt,
		}},
		[]*domain.WebPage{{
			SessionDate: sessionDate, SessionID: sessionID, TabID: 1, PageNumber: 1,
			Path: "/boundary", EnteredAt: visitedAt, ExitedAt: visitedAt.Add(time.Minute),
			DurationMs: 1000, ContactEmail: &email,
		}},
		nil,
	), "ingest must create the missing partition and retry, not drop the batch")

	var sessionCount, pageCount int
	require.NoError(t, wsDB.QueryRow(
		`SELECT count(*) FROM web_sessions WHERE session_date = $1`, sessionDate).Scan(&sessionCount))
	require.NoError(t, wsDB.QueryRow(
		`SELECT count(*) FROM web_pages WHERE session_date = $1`, sessionDate).Scan(&pageCount))
	assert.Equal(t, 1, sessionCount, "the session must have landed")
	assert.Equal(t, 1, pageCount, "and so must its pageview")
}

// TestWebAnalyticsFlushTellsAMissingPartitionFromRefusedRows pins what
// PostgreSQL reports for a row no partition takes and for a row a CHECK
// refuses. Both raise SQLSTATE 23514, and the message is in the server's
// language (lc_messages), so the repository tells them apart by the routine
// that raised the error, which is never translated: a missing partition is
// created and the batch retried, while refused rows are left to the buffer,
// which splits the batch around them.
func TestWebAnalyticsFlushTellsAMissingPartitionFromRefusedRows(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer func() { suite.Cleanup() }()

	workspace, err := suite.DataFactory.CreateWorkspace(func(w *domain.Workspace) {
		w.Settings.WebAnalytics = &domain.WebAnalyticsSettings{Enabled: true, Filters: domain.DefaultWebFilters()}
	})
	require.NoError(t, err)

	wsDB, err := suite.DBManager.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)
	repo := suite.ServerManager.GetApp().GetWebAnalyticsRepository()

	t.Run("a row no partition takes", func(t *testing.T) {
		// Two months back: outside every window anything bootstraps.
		past := time.Now().UTC().AddDate(0, -2, 0)
		sessionDate := time.Date(past.Year(), past.Month(), 15, 0, 0, 0, 0, time.UTC)
		visitedAt := sessionDate.Add(12 * time.Hour)

		_, err := wsDB.Exec(`INSERT INTO web_sessions (session_date, id, created_at, updated_at) VALUES ($1, $2, $3, $3)`,
			sessionDate, waUUIDv7At(visitedAt, 0xE2), visitedAt)
		var pqErr *pq.Error
		require.ErrorAs(t, err, &pqErr)
		assert.Equal(t, pq.ErrorCode("23514"), pqErr.Code)
		assert.Equal(t, "ExecFindPartition", pqErr.Routine)
		assert.Empty(t, pqErr.Constraint)
	})

	t.Run("a row a CHECK refuses", func(t *testing.T) {
		const refused = "refused-by-the-database"
		_, err := wsDB.Exec(`ALTER TABLE web_sessions ADD CONSTRAINT test_refused_utm_source CHECK (utm_source <> '` + refused + `')`)
		require.NoError(t, err)
		defer func() {
			_, _ = wsDB.Exec(`ALTER TABLE web_sessions DROP CONSTRAINT IF EXISTS test_refused_utm_source`)
		}()

		now := time.Now().UTC()
		err = repo.FlushBatch(context.Background(), workspace.ID, []*domain.WebSession{{
			SessionDate: time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC),
			ID:          waUUIDv7At(now, 0xE3), UTMSource: refused,
			PageviewCount: 1, CreatedAt: now, UpdatedAt: now,
		}}, nil, nil)
		assert.ErrorIs(t, err, domain.ErrWebAnalyticsRowsRefused)
		var pqErr *pq.Error
		require.ErrorAs(t, err, &pqErr)
		assert.Equal(t, pq.ErrorCode("23514"), pqErr.Code)
		assert.Equal(t, "ExecConstraints", pqErr.Routine)
		assert.Equal(t, "test_refused_utm_source", pqErr.Constraint)
	})
}
