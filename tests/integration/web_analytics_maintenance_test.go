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
