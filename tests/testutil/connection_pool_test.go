package testutil

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConnectionPoolFunctionality(t *testing.T) {
	// Skip if not running integration tests
	if os.Getenv("INTEGRATION_TESTS") != "true" {
		t.Skip("Skipping connection pool test. Set INTEGRATION_TESTS=true to run.")
	}

	// Setup test configuration
	config := &config.DatabaseConfig{
		Host:     getEnvOrDefault("TEST_DB_HOST", "localhost"),
		Port:     5433,
		User:     getEnvOrDefault("TEST_DB_USER", "notifuse_test"),
		Password: getEnvOrDefault("TEST_DB_PASSWORD", "test_password"),
		Prefix:   "notifuse_test",
		SSLMode:  "disable",
	}

	pool := NewTestConnectionPool(config)
	defer func() { _ = pool.Cleanup() }()

	t.Run("system connection", func(t *testing.T) {
		// Test system connection
		db, err := pool.GetSystemConnection()
		require.NoError(t, err)
		require.NotNil(t, db)

		// Test that we can ping the database
		err = db.Ping()
		require.NoError(t, err)

		// Test that getting the same connection returns the same instance
		db2, err := pool.GetSystemConnection()
		require.NoError(t, err)
		assert.Equal(t, db, db2, "Should return the same system connection instance")
	})

	t.Run("workspace connection pooling", func(t *testing.T) {
		workspaceID := fmt.Sprintf("testws_%d", time.Now().UnixNano())

		// Ensure workspace database exists
		err := pool.EnsureWorkspaceDatabase(workspaceID)
		require.NoError(t, err)

		// Get workspace connection
		db1, err := pool.GetWorkspaceConnection(workspaceID)
		require.NoError(t, err)
		require.NotNil(t, db1)

		// Test that we can ping the workspace database
		err = db1.Ping()
		require.NoError(t, err)

		// Get the same workspace connection again
		db2, err := pool.GetWorkspaceConnection(workspaceID)
		require.NoError(t, err)
		assert.Equal(t, db1, db2, "Should return the same workspace connection instance")

		// Cleanup workspace
		err = pool.CleanupWorkspace(workspaceID)
		require.NoError(t, err)
	})

	t.Run("multiple workspace connections", func(t *testing.T) {
		var workspaceIDs []string
		var connections []interface{}

		// Create multiple workspace connections
		for i := 0; i < 5; i++ {
			workspaceID := fmt.Sprintf("testws_%d_%d", time.Now().UnixNano(), i)
			workspaceIDs = append(workspaceIDs, workspaceID)

			err := pool.EnsureWorkspaceDatabase(workspaceID)
			require.NoError(t, err)

			db, err := pool.GetWorkspaceConnection(workspaceID)
			require.NoError(t, err)
			require.NotNil(t, db)

			err = db.Ping()
			require.NoError(t, err)

			connections = append(connections, db)
		}

		// Verify all connections are different instances
		for i := 0; i < len(connections); i++ {
			for j := i + 1; j < len(connections); j++ {
				assert.NotEqual(t, connections[i], connections[j],
					"Workspace connections should be different instances")
			}
		}

		// Cleanup all workspaces
		for _, workspaceID := range workspaceIDs {
			err := pool.CleanupWorkspace(workspaceID)
			assert.NoError(t, err)
		}
	})

	t.Run("connection count tracking", func(t *testing.T) {
		initialCount := pool.GetConnectionCount()

		workspaceID := fmt.Sprintf("testws_count_%d", time.Now().UnixNano())

		// Create workspace connection
		err := pool.EnsureWorkspaceDatabase(workspaceID)
		require.NoError(t, err)

		_, err = pool.GetWorkspaceConnection(workspaceID)
		require.NoError(t, err)

		// Count should increase
		newCount := pool.GetConnectionCount()
		assert.Equal(t, initialCount+1, newCount, "Connection count should increase")

		// Cleanup workspace
		err = pool.CleanupWorkspace(workspaceID)
		require.NoError(t, err)

		// Count should return to initial
		finalCount := pool.GetConnectionCount()
		assert.Equal(t, initialCount, finalCount, "Connection count should return to initial after cleanup")
	})
}

func TestGlobalConnectionPool(t *testing.T) {
	// Skip if not running integration tests
	if os.Getenv("INTEGRATION_TESTS") != "true" {
		t.Skip("Skipping global connection pool test. Set INTEGRATION_TESTS=true to run.")
	}

	t.Run("singleton behavior", func(t *testing.T) {
		pool1 := GetGlobalTestPool()
		pool2 := GetGlobalTestPool()

		assert.Equal(t, pool1, pool2, "GetGlobalTestPool should return the same instance")
	})

	t.Run("connection limit enforcement", func(t *testing.T) {
		pool := GetGlobalTestPool()
		initialCount := pool.GetConnectionCount()

		// Create several workspace connections
		var workspaceIDs []string
		maxConnections := 3 // Keep it small for testing

		for i := 0; i < maxConnections; i++ {
			workspaceID := fmt.Sprintf("testws_limit_%d_%d", time.Now().UnixNano(), i)
			workspaceIDs = append(workspaceIDs, workspaceID)

			err := pool.EnsureWorkspaceDatabase(workspaceID)
			require.NoError(t, err)

			_, err = pool.GetWorkspaceConnection(workspaceID)
			require.NoError(t, err)
		}

		// Verify connection count
		currentCount := pool.GetConnectionCount()
		expectedCount := initialCount + maxConnections
		assert.Equal(t, expectedCount, currentCount,
			"Connection count should match expected value")

		// Cleanup all workspaces
		for _, workspaceID := range workspaceIDs {
			err := pool.CleanupWorkspace(workspaceID)
			assert.NoError(t, err)
		}

		// Verify count returns to initial
		finalCount := pool.GetConnectionCount()
		assert.Equal(t, initialCount, finalCount,
			"Connection count should return to initial after cleanup")
	})
}
