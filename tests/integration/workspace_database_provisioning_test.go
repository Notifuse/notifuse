package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	intdb "github.com/Notifuse/notifuse/internal/database"
	"github.com/Notifuse/notifuse/internal/domain"
	pgdb "github.com/Notifuse/notifuse/pkg/database"
	"github.com/Notifuse/notifuse/tests/testutil"
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWorkspaceDatabaseProvisioning drives the app's connection manager, whose system
// database holds the workspaces table: a missing workspace database is created again for a registered
// workspace, and never for an ID no workspace has.
func TestWorkspaceDatabaseProvisioning(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	// A first app, shut down before the one under test: the manager the second app runs
	// with has to read the second app's system database, not the first's, now closed.
	first := testutil.NewIntegrationTestSuite(t, appFactory)
	first.Cleanup()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer func() { suite.Cleanup() }()

	// The manager the suite's app runs with.
	cm, err := pgdb.GetConnectionManager()
	require.NoError(t, err)
	dbConfig := suite.DBManager.GetConfig()
	systemDB := suite.DBManager.GetDB()
	prefix := dbConfig.Prefix

	// workspaces.id is VARCHAR(20).
	newID := func(kind string) string {
		return fmt.Sprintf("%s%d", kind, time.Now().UnixNano()%1_000_000_000_000)
	}
	dbName := func(t *testing.T, workspaceID string) string {
		t.Helper()
		name, err := intdb.WorkspaceDatabaseName(prefix, workspaceID)
		require.NoError(t, err)
		return name
	}
	databaseExists := func(t *testing.T, workspaceID string) bool {
		t.Helper()
		var exists bool
		require.NoError(t, systemDB.QueryRow(
			`SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname = $1)`,
			dbName(t, workspaceID)).Scan(&exists))
		return exists
	}
	dropDatabase := func(t *testing.T, workspaceID string) {
		t.Helper()
		name := dbName(t, workspaceID)
		t.Cleanup(func() {
			_ = cm.CloseWorkspaceConnection(workspaceID)
			if _, err := systemDB.Exec("DROP DATABASE IF EXISTS " + pq.QuoteIdentifier(name) + " WITH (FORCE)"); err != nil {
				t.Logf("dropping the database of %s: %v", workspaceID, err)
			}
		})
	}

	t.Run("no database for an ID no workspace has", func(t *testing.T) {
		workspaceID := newID("unk")
		dropDatabase(t, workspaceID)

		pool, err := cm.GetWorkspaceConnection(context.Background(), workspaceID)

		assert.Nil(t, pool)
		var notFound *domain.ErrWorkspaceNotFound
		require.ErrorAs(t, err, &notFound)
		assert.Equal(t, workspaceID, notFound.WorkspaceID)
		assert.False(t, databaseExists(t, workspaceID))
	})

	t.Run("missing database of a registered workspace is created", func(t *testing.T) {
		workspaceID := newID("reg")
		dropDatabase(t, workspaceID)
		_, err := systemDB.Exec(
			`INSERT INTO workspaces (id, name, settings, created_at, updated_at) VALUES ($1, $2, '{"timezone": "UTC"}', NOW(), NOW())`,
			workspaceID, "Provisioning test")
		require.NoError(t, err)
		t.Cleanup(func() { _, _ = systemDB.Exec(`DELETE FROM workspaces WHERE id = $1`, workspaceID) })

		pool, err := cm.GetWorkspaceConnection(context.Background(), workspaceID)
		require.NoError(t, err)

		assert.True(t, databaseExists(t, workspaceID))
		var tables int
		require.NoError(t, pool.QueryRow(
			`SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = 'public' AND table_name = 'message_history'`).Scan(&tables))
		assert.Equal(t, 1, tables, "the database is initialised")
	})

	// That a refused ID takes no capacity and evicts no pool is pinned by the connection
	// manager's unit test.
	t.Run("ID that cannot name a database", func(t *testing.T) {
		workspaceID := "a?b=c&d="

		pool, err := cm.GetWorkspaceConnection(context.Background(), workspaceID)

		assert.Nil(t, pool)
		assert.EqualError(t, err, `invalid workspace ID for a database name: "a?b=c&d="`)
	})
}
