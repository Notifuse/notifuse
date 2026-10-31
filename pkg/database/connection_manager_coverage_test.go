package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCoverage_IsDatabaseDoesNotExistErr(t *testing.T) {
	pqDoesNotExist := &pq.Error{Code: "3D000"}
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"plain error mentioning code", errors.New("3D000 database does not exist"), false},
		{"pq 3D000", pqDoesNotExist, true},
		{"pq undefined_table 42P01", &pq.Error{Code: "42P01"}, false},
		{"pq 3D001 is not 3D000", &pq.Error{Code: "3D001"}, false},
		{"pq empty code", &pq.Error{}, false},
		{"wrapped pq 3D000", fmt.Errorf("failed to connect to workspace ws1 database: %w", pqDoesNotExist), true},
		{"double wrapped pq 3D000", fmt.Errorf("outer: %w", fmt.Errorf("inner: %w", pqDoesNotExist)), true},
		{"wrapped pq other code", fmt.Errorf("connect: %w", &pq.Error{Code: "28P01"}), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isDatabaseDoesNotExistErr(tt.err))
		})
	}
}

func TestCoverage_ReserveAndReleaseAccounting(t *testing.T) {
	newMgr := func(maxConns, perDB int) *connectionManager {
		cfg := &config.Config{Database: config.DatabaseConfig{
			Prefix: "test", MaxConnections: maxConns, MaxConnectionsPerDB: perDB,
		}}
		sysDB, _, err := sqlmock.New()
		require.NoError(t, err)
		t.Cleanup(func() { _ = sysDB.Close() })
		return &connectionManager{
			config: cfg, systemDB: sysDB,
			workspacePools:      make(map[string]*sql.DB),
			poolAccessTimes:     make(map[string]time.Time),
			maxConnections:      maxConns,
			maxConnectionsPerDB: perDB,
		}
	}

	t.Run("exact fit then limit error with exact fields", func(t *testing.T) {
		cm := newMgr(100, 3)
		cm.mu.Lock()
		base := cm.getTotalConnectionCount()
		cm.mu.Unlock()
		// Cap admits exactly two more pools: base + 2*perDB.
		cm.maxConnections = base + 2*3
		require.NoError(t, cm.reserveCapacityForNewPool("ws-a"))
		assert.Equal(t, 1, cm.inflightPools)
		// Second pool lands exactly on the cap and still fits.
		require.NoError(t, cm.reserveCapacityForNewPool("ws-b"))
		assert.Equal(t, 2, cm.inflightPools)

		err := cm.reserveCapacityForNewPool("ws-c")
		require.Error(t, err)
		var limitErr *ConnectionLimitError
		require.ErrorAs(t, err, &limitErr)
		assert.Equal(t, base+6, limitErr.MaxConnections)
		assert.Equal(t, base, limitErr.CurrentConnections)
		assert.Equal(t, "ws-c", limitErr.WorkspaceID)
		assert.Equal(t, fmt.Sprintf("connection limit reached: %d/%d connections in use, cannot create pool for workspace ws-c", base, base+6), err.Error())
		assert.True(t, IsConnectionLimitError(err))
		// A failed reservation must not consume capacity.
		assert.Equal(t, 2, cm.inflightPools)
	})

	t.Run("release decrements and floors at zero", func(t *testing.T) {
		cm := newMgr(9, 3)
		cm.releasePoolReservation()
		assert.Equal(t, 0, cm.inflightPools, "release without reserve must not go negative")

		require.NoError(t, cm.reserveCapacityForNewPool("ws-a"))
		require.NoError(t, cm.reserveCapacityForNewPool("ws-b"))
		assert.Equal(t, 2, cm.inflightPools)
		cm.releasePoolReservation()
		assert.Equal(t, 1, cm.inflightPools)
		cm.releasePoolReservation()
		assert.Equal(t, 0, cm.inflightPools)
		cm.releasePoolReservation()
		assert.Equal(t, 0, cm.inflightPools)
	})

	t.Run("released capacity can be reserved again", func(t *testing.T) {
		cm := newMgr(100, 3)
		cm.mu.Lock()
		base := cm.getTotalConnectionCount()
		cm.mu.Unlock()
		cm.maxConnections = base + 3 // exactly one pool fits
		require.NoError(t, cm.reserveCapacityForNewPool("ws-a"))
		require.Error(t, cm.reserveCapacityForNewPool("ws-b"))
		cm.releasePoolReservation()
		require.NoError(t, cm.reserveCapacityForNewPool("ws-b"))
		assert.Equal(t, 1, cm.inflightPools)
	})

	t.Run("single pool exactly at cap", func(t *testing.T) {
		cm := newMgr(100, 3)
		cm.mu.Lock()
		base := cm.getTotalConnectionCount()
		cm.mu.Unlock()
		cm.maxConnections = base + 3
		require.NoError(t, cm.reserveCapacityForNewPool("only"))
		err := cm.reserveCapacityForNewPool("second")
		require.Error(t, err)
		assert.True(t, IsConnectionLimitError(err))
	})
}

func TestCoverage_GetWorkspaceConnectionErrorPaths(t *testing.T) {
	newMgr := func(maxConns, perDB int) *connectionManager {
		cfg := createTestConfig()
		cfg.Database.MaxConnections = maxConns
		cfg.Database.MaxConnectionsPerDB = perDB
		sysDB, _, err := sqlmock.New()
		require.NoError(t, err)
		t.Cleanup(func() { _ = sysDB.Close() })
		return &connectionManager{
			config: cfg, systemDB: sysDB,
			workspacePools:      make(map[string]*sql.DB),
			poolAccessTimes:     make(map[string]time.Time),
			maxConnections:      maxConns,
			maxConnectionsPerDB: perDB,
		}
	}

	t.Run("invalid workspace IDs are rejected before any capacity use", func(t *testing.T) {
		cm := newMgr(100, 3)
		cases := []struct{ id, wantErr string }{
			{"", `invalid workspace ID for a database name: ""`},
			{"bad id!", `invalid workspace ID for a database name: "bad id!"`},
			{"a/b", `invalid workspace ID for a database name: "a/b"`},
			{"ABC", "workspace database name must be lowercase: test_ws_ABC"},
		}
		for _, tc := range cases {
			pool, err := cm.GetWorkspaceConnection(context.Background(), tc.id)
			assert.Nil(t, pool, tc.id)
			require.Error(t, err, tc.id)
			assert.Equal(t, tc.wantErr, err.Error(), tc.id)
		}
		assert.Equal(t, 0, cm.inflightPools)
		assert.Empty(t, cm.workspacePools)
	})

	t.Run("cached pool is returned even for an ID that could not be created", func(t *testing.T) {
		cm := newMgr(100, 3)
		cached, _, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = cached.Close() }()
		old := time.Now().Add(-time.Hour)
		cm.workspacePools["BAD ID!"] = cached
		cm.poolAccessTimes["BAD ID!"] = old

		pool, err := cm.GetWorkspaceConnection(context.Background(), "BAD ID!")
		require.NoError(t, err)
		assert.True(t, pool == cached)
		assert.True(t, cm.poolAccessTimes["BAD ID!"].After(old), "fast path must refresh access time")
	})

	t.Run("no capacity surfaces ConnectionLimitError and keeps reservation count", func(t *testing.T) {
		cm := newMgr(3, 3)
		// Registration is checked before capacity: the workspace exists.
		sysDB, mock, err := sqlmock.New()
		require.NoError(t, err)
		t.Cleanup(func() { _ = sysDB.Close() })
		mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM workspaces WHERE id = \$1\)`).WithArgs("ws_new").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
		cm.systemDB = sysDB
		cm.inflightPools = 1 // one creation already in flight fills the cap
		pool, err := cm.GetWorkspaceConnection(context.Background(), "ws_new")
		assert.Nil(t, pool)
		require.Error(t, err)
		assert.True(t, IsConnectionLimitError(err))
		var limitErr *ConnectionLimitError
		require.ErrorAs(t, err, &limitErr)
		assert.Equal(t, "ws_new", limitErr.WorkspaceID)
		assert.Equal(t, 3, limitErr.MaxConnections)
		assert.Equal(t, 1, cm.inflightPools, "failed reservation must not change inflight count")
		assert.Empty(t, cm.workspacePools)
	})
}

func TestCoverage_CheckWorkspaceRegistered(t *testing.T) {
	const q = `SELECT EXISTS(SELECT 1 FROM workspaces WHERE id = $1)`
	newMgr := func() (*connectionManager, sqlmock.Sqlmock) {
		cfg := createTestConfig()
		sysDB, mock, err := sqlmock.New()
		require.NoError(t, err)
		t.Cleanup(func() { _ = sysDB.Close() })
		return &connectionManager{
			config: cfg, systemDB: sysDB,
			workspacePools:  make(map[string]*sql.DB),
			poolAccessTimes: make(map[string]time.Time),
			maxConnections:  100, maxConnectionsPerDB: 3,
		}, mock
	}

	t.Run("registered workspace passes", func(t *testing.T) {
		cm, mock := newMgr()
		mock.ExpectQuery(regexp.QuoteMeta(q)).WithArgs("ws1").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
		assert.NoError(t, cm.checkWorkspaceRegistered(context.Background(), "ws1"))
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("unregistered workspace yields ErrWorkspaceNotFound with exact ID", func(t *testing.T) {
		cm, mock := newMgr()
		mock.ExpectQuery(regexp.QuoteMeta(q)).WithArgs("ghost").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
		err := cm.checkWorkspaceRegistered(context.Background(), "ghost")
		require.Error(t, err)
		var notFound *domain.ErrWorkspaceNotFound
		require.ErrorAs(t, err, &notFound)
		assert.Equal(t, "ghost", notFound.WorkspaceID)
		assert.Equal(t, "workspace not found: ghost", err.Error())
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("query failure is wrapped with workspace context", func(t *testing.T) {
		cm, mock := newMgr()
		mock.ExpectQuery(regexp.QuoteMeta(q)).WithArgs("ws9").
			WillReturnError(errors.New("connection reset"))
		err := cm.checkWorkspaceRegistered(context.Background(), "ws9")
		require.Error(t, err)
		assert.Equal(t, "failed to check workspace ws9: connection reset", err.Error())
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}
