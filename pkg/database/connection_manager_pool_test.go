package database

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/config"
)

// Pool creation against an unreachable Postgres: creation must fail with an
// error that names the workspace but never leaks the DSN (it contains the
// password), and the capacity reservation taken before the network I/O must
// be released — a leaked reservation permanently shrinks the pool budget.

func unreachableManager(t *testing.T) *connectionManager {
	t.Helper()
	systemDB, err := sql.Open("postgres", "postgres://postgres:pw@127.0.0.1:1/postgres?sslmode=disable")
	require.NoError(t, err)
	t.Cleanup(func() { _ = systemDB.Close() })
	return &connectionManager{
		config: &config.Config{Database: config.DatabaseConfig{
			Host: "127.0.0.1", Port: 1, User: "postgres", Password: "pw", SSLMode: "disable",
		}},
		systemDB:            systemDB,
		workspacePools:      map[string]*sql.DB{},
		poolAccessTimes:     map[string]time.Time{},
		maxConnections:      100,
		maxConnectionsPerDB: 5,
	}
}

func TestOpenAndVerifyPool_Unreachable(t *testing.T) {
	cm := unreachableManager(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := cm.openAndVerifyPool(ctx, "postgres://postgres:topsecret@127.0.0.1:1/wsdb?sslmode=disable", "ws1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to connect to workspace ws1 database:")
	assert.NotContains(t, err.Error(), "topsecret", "the DSN (with password) must never reach an error")
	assert.NotContains(t, err.Error(), "127.0.0.1:1/wsdb")
}

func TestCreateWorkspacePool_Unreachable(t *testing.T) {
	cm := unreachableManager(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := cm.createWorkspacePool(ctx, "ws1")
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "pw", "no credential material in the error")
}

func TestGetWorkspaceConnection_PoolFailureReleasesReservation(t *testing.T) {
	cm := unreachableManager(t)
	// The workspace is registered, so creation gets past the registration check and
	// reserves capacity before the dial to the unreachable workspace database fails.
	sysDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sysDB.Close() })
	for range 2 {
		mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM workspaces WHERE id = \$1\)`).WithArgs("ws1").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	}
	cm.systemDB = sysDB
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err = cm.GetWorkspaceConnection(ctx, "ws1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create workspace pool:")

	assert.Equal(t, 0, cm.inflightPools, "the capacity reservation must be released on failure")
	_, cached := cm.workspacePools["ws1"]
	assert.False(t, cached, "a failed pool must not be cached")

	// A second attempt fails the same way rather than consuming budget.
	_, err = cm.GetWorkspaceConnection(ctx, "ws1")
	require.Error(t, err)
	assert.Equal(t, 0, cm.inflightPools)
}

func TestGetWorkspaceConnection_CancelledContext(t *testing.T) {
	cm := unreachableManager(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := cm.GetWorkspaceConnection(ctx, "ws1")
	require.ErrorIs(t, err, context.Canceled)
}
