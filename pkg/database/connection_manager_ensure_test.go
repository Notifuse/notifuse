package database

import (
	"context"
	"database/sql"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
)

// The 3D000 fallback creates the database through EnsureWorkspaceDatabaseExists on the
// pool creation's own bounded context, with the registration check run again inside
// the name lock.
func TestCreateWorkspacePool_MissingDatabaseFallback(t *testing.T) {
	newManager := func(t *testing.T) (*connectionManager, sqlmock.Sqlmock, *atomic.Int32) {
		cm, mock, _, _ := guardManager(t, 100, 10)
		var opens atomic.Int32
		cm.openPool = func(ctx context.Context, dsn, workspaceID string) (*sql.DB, error) {
			if opens.Add(1) == 1 {
				return nil, &pq.Error{Code: "3D000", Message: "database does not exist"}
			}
			pool, _, err := sqlmock.New()
			require.NoError(t, err)
			t.Cleanup(func() { _ = pool.Close() })
			return pool, nil
		}
		return cm, mock, &opens
	}

	t.Run("passes the creation context and a registration check", func(t *testing.T) {
		cm, mock, opens := newManager(t)
		expectRegistered(mock, "acme", true) // GetWorkspaceConnection's own check
		expectRegistered(mock, "acme", true) // stillRegistered, inside the lock

		var gotDeadline time.Time
		cm.ensureWorkspaceDatabase = func(ctx context.Context, cfg *config.DatabaseConfig, workspaceID string, stillRegistered func(ctx context.Context) error) error {
			assert.Equal(t, "acme", workspaceID)
			gotDeadline, _ = ctx.Deadline()
			require.NotNil(t, stillRegistered)
			return stillRegistered(ctx)
		}

		ctx, cancel := context.WithCancel(context.Background())
		start := time.Now()
		pool, err := cm.GetWorkspaceConnection(ctx, "acme")
		cancel()
		require.NoError(t, err)
		require.NotNil(t, pool)
		assert.Equal(t, int32(2), opens.Load())
		assert.WithinDuration(t, start.Add(workspacePoolCreateTimeout), gotDeadline, 5*time.Second)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("a workspace deleted meanwhile gets no database", func(t *testing.T) {
		cm, mock, opens := newManager(t)
		expectRegistered(mock, "acme", true)
		expectRegistered(mock, "acme", false)

		cm.ensureWorkspaceDatabase = func(ctx context.Context, cfg *config.DatabaseConfig, workspaceID string, stillRegistered func(ctx context.Context) error) error {
			return stillRegistered(ctx)
		}

		_, err := cm.GetWorkspaceConnection(context.Background(), "acme")
		var notFound *domain.ErrWorkspaceNotFound
		require.True(t, errors.As(err, &notFound), "got %v", err)
		assert.Equal(t, int32(1), opens.Load(), "no second open after the refusal")
	})
}
