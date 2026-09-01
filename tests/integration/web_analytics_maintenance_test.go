//go:build integration
// +build integration

package integration

import (
	"context"
	"testing"
	"time"

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
