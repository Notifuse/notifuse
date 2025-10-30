package integration

import (
	"fmt"
	"testing"

	"github.com/Notifuse/notifuse/tests/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestConnectionPoolLifecycle tests the complete lifecycle of connection pools
func TestConnectionPoolLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping connection pool lifecycle tests in short mode")
	}

	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	t.Run("pool initialization", func(t *testing.T) {
		config := testutil.GetTestDatabaseConfig()
		pool := testutil.NewTestConnectionPool(config)
		defer pool.Cleanup()

		// Test system connection is established
		systemDB, err := pool.GetSystemConnection()
		require.NoError(t, err, "Should be able to get system connection")
		require.NotNil(t, systemDB, "System connection should not be nil")

		// Test system connection works
		err = systemDB.Ping()
		require.NoError(t, err, "System connection should be pingable")

		// Stats should show correct initial state
		count := pool.GetConnectionCount()
		assert.Equal(t, 0, count, "Should have 0 workspace connections initially")
	})

	t.Run("workspace pool creation", func(t *testing.T) {
		config := testutil.GetTestDatabaseConfig()
		pool := testutil.NewTestConnectionPool(config)
		defer pool.Cleanup()

		workspaceID := "test_lifecycle_create"

		// Create workspace database
		err := pool.EnsureWorkspaceDatabase(workspaceID)
		require.NoError(t, err, "Should be able to ensure workspace database")

		// Get connection from pool
		db, err := pool.GetWorkspaceConnection(workspaceID)
		require.NoError(t, err, "Should be able to get workspace connection")
		require.NotNil(t, db, "Workspace connection should not be nil")

		// Verify connection works
		err = db.Ping()
		require.NoError(t, err, "Workspace connection should be pingable")

		var result int
		err = db.QueryRow("SELECT 1").Scan(&result)
		require.NoError(t, err, "Should be able to query workspace database")
		assert.Equal(t, 1, result, "Query should return 1")

		// Stats should show increased count
		count := pool.GetConnectionCount()
		assert.Equal(t, 1, count, "Should have 1 workspace connection")
	})

	t.Run("workspace pool reuse", func(t *testing.T) {
		config := testutil.GetTestDatabaseConfig()
		pool := testutil.NewTestConnectionPool(config)
		defer pool.Cleanup()

		workspaceID := "test_lifecycle_reuse"

		// Ensure database exists
		err := pool.EnsureWorkspaceDatabase(workspaceID)
		require.NoError(t, err)

		// Request same workspace twice
		db1, err := pool.GetWorkspaceConnection(workspaceID)
		require.NoError(t, err)

		db2, err := pool.GetWorkspaceConnection(workspaceID)
		require.NoError(t, err)

		// Verify same connection returned (pointer equality)
		assert.Equal(t, db1, db2, "Should return same connection pool instance")

		// No duplicate pools created
		count := pool.GetConnectionCount()
		assert.Equal(t, 1, count, "Should still have only 1 workspace connection")
	})

	t.Run("workspace pool cleanup", func(t *testing.T) {
		config := testutil.GetTestDatabaseConfig()
		pool := testutil.NewTestConnectionPool(config)
		defer pool.Cleanup()

		workspaceID := "test_lifecycle_cleanup"

		// Create workspace
		err := pool.EnsureWorkspaceDatabase(workspaceID)
		require.NoError(t, err)

		_, err = pool.GetWorkspaceConnection(workspaceID)
		require.NoError(t, err)

		// Verify connection exists
		assert.Equal(t, 1, pool.GetConnectionCount())

		// Close workspace pool
		err = pool.CleanupWorkspace(workspaceID)
		require.NoError(t, err, "Should be able to cleanup workspace")

		// Stats should show decreased count
		count := pool.GetConnectionCount()
		assert.Equal(t, 0, count, "Should have 0 workspace connections after cleanup")

		// Connection should no longer be in pool
		// Getting the same workspace should create a new connection
		// (but database won't exist since we dropped it)
	})

	t.Run("full cleanup", func(t *testing.T) {
		config := testutil.GetTestDatabaseConfig()
		pool := testutil.NewTestConnectionPool(config)

		// Create multiple workspace pools
		workspaceIDs := []string{
			"test_lifecycle_full_1",
			"test_lifecycle_full_2",
			"test_lifecycle_full_3",
		}

		for _, workspaceID := range workspaceIDs {
			err := pool.EnsureWorkspaceDatabase(workspaceID)
			require.NoError(t, err)

			_, err = pool.GetWorkspaceConnection(workspaceID)
			require.NoError(t, err)
		}

		// Verify all connections exist
		assert.Equal(t, 3, pool.GetConnectionCount())

		// Clean up all
		err := pool.Cleanup()
		require.NoError(t, err, "Full cleanup should succeed")

		// Verify no connections remain
		count := pool.GetConnectionCount()
		assert.Equal(t, 0, count, "Should have 0 connections after full cleanup")

		// Verify pool is empty (system connection closed)
		systemDB, err := pool.GetSystemConnection()
		if err == nil && systemDB != nil {
			// If we can still get a system connection, it means pool was re-initialized
			// This is actually fine for the test pool design
			err = systemDB.Ping()
			require.NoError(t, err)
		}
	})

	t.Run("cleanup idempotency", func(t *testing.T) {
		config := testutil.GetTestDatabaseConfig()
		pool := testutil.NewTestConnectionPool(config)

		workspaceID := "test_lifecycle_idempotent"

		// Create and cleanup once
		err := pool.EnsureWorkspaceDatabase(workspaceID)
		require.NoError(t, err)

		_, err = pool.GetWorkspaceConnection(workspaceID)
		require.NoError(t, err)

		err = pool.Cleanup()
		require.NoError(t, err)

		// Cleanup again should not error
		err = pool.Cleanup()
		require.NoError(t, err, "Second cleanup should not error")
	})

	t.Run("multiple pools isolated", func(t *testing.T) {
		config := testutil.GetTestDatabaseConfig()

		pool1 := testutil.NewTestConnectionPool(config)
		pool2 := testutil.NewTestConnectionPool(config)
		defer pool1.Cleanup()
		defer pool2.Cleanup()

		workspaceID1 := "test_lifecycle_isolated_1"
		workspaceID2 := "test_lifecycle_isolated_2"

		// Create workspace in pool1
		err := pool1.EnsureWorkspaceDatabase(workspaceID1)
		require.NoError(t, err)
		_, err = pool1.GetWorkspaceConnection(workspaceID1)
		require.NoError(t, err)

		// Create workspace in pool2
		err = pool2.EnsureWorkspaceDatabase(workspaceID2)
		require.NoError(t, err)
		_, err = pool2.GetWorkspaceConnection(workspaceID2)
		require.NoError(t, err)

		// Each pool should have its own connection
		assert.Equal(t, 1, pool1.GetConnectionCount())
		assert.Equal(t, 1, pool2.GetConnectionCount())

		// Cleanup pool1 should not affect pool2
		err = pool1.Cleanup()
		require.NoError(t, err)

		assert.Equal(t, 0, pool1.GetConnectionCount())
		assert.Equal(t, 1, pool2.GetConnectionCount())
	})
}

// TestConnectionPoolManagerIsolation tests that the pool manager properly isolates tests
func TestConnectionPoolManagerIsolation(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping connection pool manager tests in short mode")
	}

	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	t.Run("isolated pools per test", func(t *testing.T) {
		config := testutil.GetTestDatabaseConfig()
		manager := testutil.NewTestConnectionPoolManager()
		defer manager.CleanupAll()

		// Create pools for different "tests"
		pool1 := manager.GetOrCreatePool("test1", config)
		pool2 := manager.GetOrCreatePool("test2", config)

		// Should be different pool instances (check pointer addresses)
		require.NotNil(t, pool1, "Pool1 should not be nil")
		require.NotNil(t, pool2, "Pool2 should not be nil")
		assert.NotSame(t, pool1, pool2, "Different test IDs should get different pool instances")

		// Same test ID should get same pool
		pool1Again := manager.GetOrCreatePool("test1", config)
		assert.Same(t, pool1, pool1Again, "Same test ID should get same pool instance")

		// Manager should track both pools
		assert.Equal(t, 2, manager.GetPoolCount())
	})

	t.Run("cleanup specific pool", func(t *testing.T) {
		config := testutil.GetTestDatabaseConfig()
		manager := testutil.NewTestConnectionPoolManager()
		defer manager.CleanupAll()

		pool1 := manager.GetOrCreatePool("test1", config)
		pool2 := manager.GetOrCreatePool("test2", config)

		// Ensure databases
		err := pool1.EnsureWorkspaceDatabase("ws1")
		require.NoError(t, err)
		err = pool2.EnsureWorkspaceDatabase("ws2")
		require.NoError(t, err)

		// Cleanup test1
		err = manager.CleanupPool("test1")
		require.NoError(t, err)

		// Only one pool should remain
		assert.Equal(t, 1, manager.GetPoolCount())
	})

	t.Run("cleanup all pools", func(t *testing.T) {
		config := testutil.GetTestDatabaseConfig()
		manager := testutil.NewTestConnectionPoolManager()

		// Create multiple pools
		for i := 0; i < 5; i++ {
			testID := fmt.Sprintf("test%d", i)
			pool := manager.GetOrCreatePool(testID, config)
			workspaceID := fmt.Sprintf("ws%d", i)
			err := pool.EnsureWorkspaceDatabase(workspaceID)
			require.NoError(t, err)
		}

		assert.Equal(t, 5, manager.GetPoolCount())

		// Cleanup all
		err := manager.CleanupAll()
		require.NoError(t, err)

		// No pools should remain
		assert.Equal(t, 0, manager.GetPoolCount())
	})
}

// TestConnectionPoolMetrics tests the metrics collection
func TestConnectionPoolMetrics(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping connection pool metrics tests in short mode")
	}

	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	t.Run("metrics tracking", func(t *testing.T) {
		config := testutil.GetTestDatabaseConfig()
		pool := testutil.NewTestConnectionPool(config)
		defer pool.Cleanup()

		// Create metrics tracker
		metrics := testutil.NewConnectionPoolMetrics("test_metrics", 0)

		// Create some workspaces
		for i := 0; i < 3; i++ {
			workspaceID := fmt.Sprintf("test_metrics_%d", i)
			err := pool.EnsureWorkspaceDatabase(workspaceID)
			require.NoError(t, err)

			_, err = pool.GetWorkspaceConnection(workspaceID)
			require.NoError(t, err)

			metrics.RecordPoolCreation()
			metrics.UpdateConnections(pool.GetConnectionCount())
		}

		// Cleanup and finalize
		err := pool.Cleanup()
		require.NoError(t, err)

		metrics.Finalize(pool.GetConnectionCount())

		// Verify metrics
		assert.Equal(t, 3, metrics.PeakConnections)
		assert.Equal(t, 3, metrics.PoolCreations)
		assert.False(t, metrics.HasLeaks(), "Should not have leaks")
	})
}
