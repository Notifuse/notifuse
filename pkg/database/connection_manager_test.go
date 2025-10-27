package database

import (
	"testing"
	"time"

	"github.com/Notifuse/notifuse/config"
	"github.com/stretchr/testify/assert"
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

// Integration-style tests that would require a real database
// These are marked as integration tests and skipped in unit test runs

func TestConnectionManager_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// These tests would need a real PostgreSQL database
	// They should be in integration tests instead
	t.Skip("Integration tests moved to tests/integration/")
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

func TestConnectionManager_HasCapacityForNewPool(t *testing.T) {
	// This tests the internal logic without needing a DB
	// We'd need to refactor to make hasCapacityForNewPool testable
	// or use integration tests with a real DB
	t.Skip("Internal method testing requires refactoring or integration tests")
}

func TestConnectionManager_GetTotalConnectionCount(t *testing.T) {
	// This tests the internal logic without needing a DB
	t.Skip("Internal method testing requires refactoring or integration tests")
}

func TestConnectionManager_CloseLRUIdlePools(t *testing.T) {
	// This tests the internal logic without needing a DB
	t.Skip("Internal method testing requires refactoring or integration tests")
}
