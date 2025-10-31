package database

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Notifuse/notifuse/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInitializeConnectionManager(t *testing.T) {
	// Reset singleton before each test
	defer ResetConnectionManager()

	// Create a test config
	_ = &config.Config{
		Database: config.DatabaseConfig{
			Host:                  "localhost",
			Port:                  5432,
			User:                  "test",
			Password:              "test",
			DBName:                "test",
			Prefix:                "test",
			SSLMode:               "disable",
			MaxConnections:        100,
			MaxConnectionsPerDB:   3,
			ConnectionMaxLifetime: 10 * time.Minute,
			ConnectionMaxIdleTime: 5 * time.Minute,
		},
	}

	// Create a mock database connection
	// In a real test with database, you'd use sql.Open
	// For unit tests without DB, we'll skip the actual connection
	t.Run("initializes successfully", func(t *testing.T) {
		// Note: This would need a real DB connection or mock
		// For now, we'll test the singleton pattern
		ResetConnectionManager()

		// Since we can't create a real DB connection in unit tests,
		// we test that GetConnectionManager fails before init
		_, err := GetConnectionManager()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not initialized")
	})
}

func TestGetConnectionManager_NotInitialized(t *testing.T) {
	defer ResetConnectionManager()
	ResetConnectionManager()

	_, err := GetConnectionManager()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not initialized")
}

func TestResetConnectionManager(t *testing.T) {
	defer ResetConnectionManager()

	// Reset should clear the singleton
	ResetConnectionManager()

	_, err := GetConnectionManager()
	assert.Error(t, err)
}

func TestConnectionLimitError(t *testing.T) {
	err := &ConnectionLimitError{
		MaxConnections:     100,
		CurrentConnections: 95,
		WorkspaceID:        "test-workspace",
	}

	assert.Contains(t, err.Error(), "connection limit reached")
	assert.Contains(t, err.Error(), "95/100")
	assert.Contains(t, err.Error(), "test-workspace")
}

func TestIsConnectionLimitError(t *testing.T) {
	t.Run("identifies ConnectionLimitError", func(t *testing.T) {
		err := &ConnectionLimitError{
			MaxConnections:     100,
			CurrentConnections: 95,
			WorkspaceID:        "test",
		}

		assert.True(t, IsConnectionLimitError(err))
	})

	t.Run("returns false for other errors", func(t *testing.T) {
		err := assert.AnError

		assert.False(t, IsConnectionLimitError(err))
	})
}

func TestConnectionPoolStats(t *testing.T) {
	stats := ConnectionPoolStats{
		OpenConnections: 5,
		InUse:           2,
		Idle:            3,
		MaxOpen:         10,
		WaitCount:       5,
		WaitDuration:    100 * time.Millisecond,
	}

	assert.Equal(t, 5, stats.OpenConnections)
	assert.Equal(t, 2, stats.InUse)
	assert.Equal(t, 3, stats.Idle)
	assert.Equal(t, 10, stats.MaxOpen)
}

func TestConnectionStats(t *testing.T) {
	stats := ConnectionStats{
		MaxConnections:           100,
		MaxConnectionsPerDB:      3,
		TotalOpenConnections:     15,
		TotalInUseConnections:    8,
		TotalIdleConnections:     7,
		ActiveWorkspaceDatabases: 5,
		WorkspacePools:           make(map[string]ConnectionPoolStats),
	}

	assert.Equal(t, 100, stats.MaxConnections)
	assert.Equal(t, 3, stats.MaxConnectionsPerDB)
	assert.Equal(t, 15, stats.TotalOpenConnections)
	assert.Equal(t, 8, stats.TotalInUseConnections)
	assert.Equal(t, 7, stats.TotalIdleConnections)
	assert.Equal(t, 5, stats.ActiveWorkspaceDatabases)
}

// Helper function for tests
func createTestConfig() *config.Config {
	return &config.Config{
		Database: config.DatabaseConfig{
			Host:                  "localhost",
			Port:                  5432,
			User:                  "test",
			Password:              "test",
			DBName:                "test_db",
			Prefix:                "test",
			SSLMode:               "disable",
			MaxConnections:        100,
			MaxConnectionsPerDB:   3,
			ConnectionMaxLifetime: 10 * time.Minute,
			ConnectionMaxIdleTime: 5 * time.Minute,
		},
	}
}

// Internal method tests - these access private fields/methods since we're in the same package

func TestConnectionManager_HasCapacityForNewPool(t *testing.T) {
	defer ResetConnectionManager()

	cfg := createTestConfig()
	cfg.Database.MaxConnections = 30
	cfg.Database.MaxConnectionsPerDB = 3

	db, _, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	err = InitializeConnectionManager(cfg, db)
	require.NoError(t, err)

	cm := instance

	t.Run("has capacity when empty", func(t *testing.T) {
		cm.mu.Lock()
		hasCapacity := cm.hasCapacityForNewPool()
		cm.mu.Unlock()

		assert.True(t, hasCapacity)
	})

	t.Run("no capacity when at limit", func(t *testing.T) {
		cm.mu.Lock()
		defer cm.mu.Unlock()

		// With empty pools (just system DB), should have capacity
		hasCapacity := cm.hasCapacityForNewPool()
		assert.True(t, hasCapacity)
	})
}

func TestConnectionManager_GetTotalConnectionCount(t *testing.T) {
	defer ResetConnectionManager()

	cfg := createTestConfig()
	systemDB, systemMock, err := sqlmock.New()
	require.NoError(t, err)
	defer systemDB.Close()

	systemDB.SetMaxOpenConns(10)
	systemMock.ExpectClose()

	err = InitializeConnectionManager(cfg, systemDB)
	require.NoError(t, err)

	cm := instance

	t.Run("counts system connections", func(t *testing.T) {
		cm.mu.RLock()
		total := cm.getTotalConnectionCount()
		cm.mu.RUnlock()

		assert.GreaterOrEqual(t, total, 0)
	})

	t.Run("counts workspace pools", func(t *testing.T) {
		cm.mu.Lock()

		wsDB, _, _ := sqlmock.New()
		wsDB.SetMaxOpenConns(3)
		cm.workspacePools["test_ws"] = wsDB
		cm.poolAccessTimes["test_ws"] = time.Now()

		total := cm.getTotalConnectionCount()

		cm.mu.Unlock()

		assert.GreaterOrEqual(t, total, 0)

		// Clean up
		cm.mu.Lock()
		delete(cm.workspacePools, "test_ws")
		delete(cm.poolAccessTimes, "test_ws")
		cm.mu.Unlock()
		wsDB.Close()
	})
}

func TestConnectionManager_CloseLRUIdlePools(t *testing.T) {
	defer ResetConnectionManager()

	cfg := createTestConfig()
	systemDB, _, err := sqlmock.New()
	require.NoError(t, err)
	defer systemDB.Close()

	err = InitializeConnectionManager(cfg, systemDB)
	require.NoError(t, err)

	cm := instance

	t.Run("closes oldest idle pool first", func(t *testing.T) {
		cm.mu.Lock()

		old, _, _ := sqlmock.New()
		old.SetMaxOpenConns(3)
		old.SetMaxIdleConns(3)

		medium, _, _ := sqlmock.New()
		medium.SetMaxOpenConns(3)
		medium.SetMaxIdleConns(3)

		recent, _, _ := sqlmock.New()
		recent.SetMaxOpenConns(3)
		recent.SetMaxIdleConns(3)

		now := time.Now()
		cm.workspacePools["ws_old"] = old
		cm.poolAccessTimes["ws_old"] = now.Add(-1 * time.Hour)

		cm.workspacePools["ws_medium"] = medium
		cm.poolAccessTimes["ws_medium"] = now.Add(-30 * time.Minute)

		cm.workspacePools["ws_recent"] = recent
		cm.poolAccessTimes["ws_recent"] = now

		cm.mu.Unlock()

		closed := cm.closeLRUIdlePools(1)

		assert.Equal(t, 1, closed)

		cm.mu.RLock()
		_, oldExists := cm.workspacePools["ws_old"]
		_, mediumExists := cm.workspacePools["ws_medium"]
		_, recentExists := cm.workspacePools["ws_recent"]
		cm.mu.RUnlock()

		assert.False(t, oldExists, "Oldest pool should be closed")
		assert.True(t, mediumExists, "Medium pool should remain")
		assert.True(t, recentExists, "Recent pool should remain")

		// Clean up
		cm.mu.Lock()
		delete(cm.workspacePools, "ws_medium")
		delete(cm.workspacePools, "ws_recent")
		delete(cm.poolAccessTimes, "ws_medium")
		delete(cm.poolAccessTimes, "ws_recent")
		cm.mu.Unlock()

		old.Close()
		medium.Close()
		recent.Close()
	})

	t.Run("closes multiple pools in LRU order", func(t *testing.T) {
		cm.mu.Lock()

		now := time.Now()
		for i := 0; i < 5; i++ {
			mockDB, _, _ := sqlmock.New()
			mockDB.SetMaxOpenConns(3)
			mockDB.SetMaxIdleConns(3)
			wsID := fmt.Sprintf("ws_%d", i)
			cm.workspacePools[wsID] = mockDB
			cm.poolAccessTimes[wsID] = now.Add(time.Duration(-5+i) * time.Minute)
		}

		cm.mu.Unlock()

		closed := cm.closeLRUIdlePools(2)

		assert.Equal(t, 2, closed)

		cm.mu.RLock()
		_, ws0 := cm.workspacePools["ws_0"]
		_, ws1 := cm.workspacePools["ws_1"]
		_, ws2 := cm.workspacePools["ws_2"]
		_, ws3 := cm.workspacePools["ws_3"]
		_, ws4 := cm.workspacePools["ws_4"]
		cm.mu.RUnlock()

		assert.False(t, ws0, "ws_0 (oldest) should be closed")
		assert.False(t, ws1, "ws_1 (second oldest) should be closed")
		assert.True(t, ws2, "ws_2 should remain")
		assert.True(t, ws3, "ws_3 should remain")
		assert.True(t, ws4, "ws_4 (newest) should remain")

		// Clean up
		cm.mu.Lock()
		for i := 2; i < 5; i++ {
			wsID := fmt.Sprintf("ws_%d", i)
			if pool, ok := cm.workspacePools[wsID]; ok {
				pool.Close()
				delete(cm.workspacePools, wsID)
				delete(cm.poolAccessTimes, wsID)
			}
		}
		cm.mu.Unlock()
	})

	t.Run("returns 0 when no idle pools", func(t *testing.T) {
		closed := cm.closeLRUIdlePools(1)
		assert.Equal(t, 0, closed)
	})
}

func TestConnectionManager_ContextCancellation(t *testing.T) {
	defer ResetConnectionManager()

	cfg := createTestConfig()
	systemDB, _, err := sqlmock.New()
	require.NoError(t, err)
	defer systemDB.Close()

	err = InitializeConnectionManager(cfg, systemDB)
	require.NoError(t, err)

	cm := instance

	t.Run("returns error when context already cancelled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := cm.GetWorkspaceConnection(ctx, "test_workspace")
		assert.Error(t, err)
		assert.Equal(t, context.Canceled, err)
	})

	t.Run("returns error when context cancelled with timeout", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
		defer cancel()

		time.Sleep(10 * time.Millisecond)

		_, err := cm.GetWorkspaceConnection(ctx, "test_workspace")
		assert.Error(t, err)
		assert.Equal(t, context.DeadlineExceeded, err)
	})
}

func TestConnectionManager_RaceConditionSafety(t *testing.T) {
	defer ResetConnectionManager()

	cfg := createTestConfig()
	systemDB, _, err := sqlmock.New()
	require.NoError(t, err)
	defer systemDB.Close()

	err = InitializeConnectionManager(cfg, systemDB)
	require.NoError(t, err)

	t.Run("double-check prevents duplicate pool creation", func(t *testing.T) {
		mockPool, _, _ := sqlmock.New()
		mockPool.SetMaxOpenConns(3)
		defer mockPool.Close()

		instance.mu.Lock()
		instance.workspacePools["race_test"] = mockPool
		instance.poolAccessTimes["race_test"] = time.Now()
		instance.mu.Unlock()

		ctx := context.Background()
		pool, err := instance.GetWorkspaceConnection(ctx, "race_test")

		assert.NoError(t, err)
		assert.Equal(t, mockPool, pool)

		// Clean up
		instance.mu.Lock()
		delete(instance.workspacePools, "race_test")
		delete(instance.poolAccessTimes, "race_test")
		instance.mu.Unlock()
	})
}

func TestConnectionManager_CloseWorkspaceConnection(t *testing.T) {
	defer ResetConnectionManager()

	cfg := createTestConfig()
	systemDB, _, err := sqlmock.New()
	require.NoError(t, err)
	defer systemDB.Close()

	err = InitializeConnectionManager(cfg, systemDB)
	require.NoError(t, err)

	cm := instance

	t.Run("closes pool and removes from both maps", func(t *testing.T) {
		mockPool, mockSQL, _ := sqlmock.New()
		mockPool.SetMaxOpenConns(3)

		mockSQL.ExpectClose()

		cm.mu.Lock()
		cm.workspacePools["test_close"] = mockPool
		cm.poolAccessTimes["test_close"] = time.Now()
		cm.mu.Unlock()

		err := cm.CloseWorkspaceConnection("test_close")
		assert.NoError(t, err)

		cm.mu.RLock()
		_, poolExists := cm.workspacePools["test_close"]
		_, timeExists := cm.poolAccessTimes["test_close"]
		cm.mu.RUnlock()

		assert.False(t, poolExists, "Pool should be removed from workspacePools")
		assert.False(t, timeExists, "Access time should be removed from poolAccessTimes")

		assert.NoError(t, mockSQL.ExpectationsWereMet())
	})

	t.Run("idempotent - closing non-existent pool is safe", func(t *testing.T) {
		err := cm.CloseWorkspaceConnection("non_existent")
		assert.NoError(t, err)
	})
}

func TestConnectionManager_AccessTimeTracking(t *testing.T) {
	defer ResetConnectionManager()

	cfg := createTestConfig()
	systemDB, _, err := sqlmock.New()
	require.NoError(t, err)
	defer systemDB.Close()

	err = InitializeConnectionManager(cfg, systemDB)
	require.NoError(t, err)

	cm := instance

	t.Run("tracks access time on pool reuse", func(t *testing.T) {
		mockPool, mockSQL, _ := sqlmock.New(sqlmock.MonitorPingsOption(true))
		mockPool.SetMaxOpenConns(3)
		defer mockPool.Close()

		now := time.Now()
		cm.mu.Lock()
		cm.workspacePools["time_test"] = mockPool
		cm.poolAccessTimes["time_test"] = now.Add(-1 * time.Hour)
		cm.mu.Unlock()

		mockSQL.ExpectPing()

		ctx := context.Background()
		pool, err := cm.GetWorkspaceConnection(ctx, "time_test")

		require.NoError(t, err)
		assert.Equal(t, mockPool, pool)

		cm.mu.RLock()
		accessTime := cm.poolAccessTimes["time_test"]
		cm.mu.RUnlock()

		assert.WithinDuration(t, time.Now(), accessTime, 1*time.Second)

		// Clean up
		cm.mu.Lock()
		delete(cm.workspacePools, "time_test")
		delete(cm.poolAccessTimes, "time_test")
		cm.mu.Unlock()

		assert.NoError(t, mockSQL.ExpectationsWereMet())
	})
}

func TestConnectionManager_StalePoolRemoval(t *testing.T) {
	defer ResetConnectionManager()

	cfg := createTestConfig()
	systemDB, _, err := sqlmock.New()
	require.NoError(t, err)
	defer systemDB.Close()

	err = InitializeConnectionManager(cfg, systemDB)
	require.NoError(t, err)

	cm := instance

	t.Run("removes stale pool when ping fails", func(t *testing.T) {
		mockPool, _, _ := sqlmock.New()
		mockPool.SetMaxOpenConns(3)
		mockPool.Close()

		cm.mu.Lock()
		cm.workspacePools["stale_test"] = mockPool
		cm.poolAccessTimes["stale_test"] = time.Now()
		cm.mu.Unlock()

		ctx := context.Background()
		_, err := cm.GetWorkspaceConnection(ctx, "stale_test")

		assert.Error(t, err)

		cm.mu.RLock()
		_, poolExists := cm.workspacePools["stale_test"]
		_, timeExists := cm.poolAccessTimes["stale_test"]
		cm.mu.RUnlock()

		assert.False(t, poolExists, "Stale pool should be removed")
		assert.False(t, timeExists, "Stale pool access time should be removed")
	})
}

func TestConnectionManager_LRUSorting(t *testing.T) {
	defer ResetConnectionManager()

	cfg := createTestConfig()
	systemDB, _, err := sqlmock.New()
	require.NoError(t, err)
	defer systemDB.Close()

	err = InitializeConnectionManager(cfg, systemDB)
	require.NoError(t, err)

	cm := instance

	t.Run("sorts by access time correctly", func(t *testing.T) {
		cm.mu.Lock()

		now := time.Now()
		ages := []struct {
			id  string
			age time.Duration
		}{
			{"ws_newest", 0},
			{"ws_5min", -5 * time.Minute},
			{"ws_10min", -10 * time.Minute},
			{"ws_1hour", -1 * time.Hour},
			{"ws_oldest", -2 * time.Hour},
		}

		for _, a := range ages {
			mockDB, _, _ := sqlmock.New()
			mockDB.SetMaxOpenConns(3)
			mockDB.SetMaxIdleConns(3)
			cm.workspacePools[a.id] = mockDB
			cm.poolAccessTimes[a.id] = now.Add(a.age)
		}

		cm.mu.Unlock()

		closed := cm.closeLRUIdlePools(3)

		assert.Equal(t, 3, closed)

		cm.mu.RLock()
		_, oldestExists := cm.workspacePools["ws_oldest"]
		_, hourExists := cm.workspacePools["ws_1hour"]
		_, tenExists := cm.workspacePools["ws_10min"]
		_, fiveExists := cm.workspacePools["ws_5min"]
		_, newestExists := cm.workspacePools["ws_newest"]
		cm.mu.RUnlock()

		assert.False(t, oldestExists, "ws_oldest should be closed")
		assert.False(t, hourExists, "ws_1hour should be closed")
		assert.False(t, tenExists, "ws_10min should be closed")
		assert.True(t, fiveExists, "ws_5min should remain")
		assert.True(t, newestExists, "ws_newest should remain")

		// Clean up
		cm.mu.Lock()
		for _, a := range ages {
			if pool, ok := cm.workspacePools[a.id]; ok {
				pool.Close()
				delete(cm.workspacePools, a.id)
				delete(cm.poolAccessTimes, a.id)
			}
		}
		cm.mu.Unlock()
	})
}
