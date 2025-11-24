package integration

import (
	"fmt"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/tests/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestConnectionPoolFailureRecovery tests resilience to failures
func TestConnectionPoolFailureRecovery(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping connection pool failure recovery tests in short mode")
	}

	testutil.SetupTestEnvironment()
	defer func() {
		testutil.CleanupTestEnvironment()
		// Extra delay to ensure PostgreSQL releases all connections before next test suite
		time.Sleep(2 * time.Second)
	}()

	t.Run("stale connection detection", func(t *testing.T) {
		config := testutil.GetTestDatabaseConfig()
		pool := testutil.NewTestConnectionPool(config)
		defer _ = pool.Cleanup()

		workspaceID := "test_failure_stale"

		// Create workspace and get connection
		err := pool.EnsureWorkspaceDatabase(workspaceID)
		require.NoError(t, err)

		db, err := pool.GetWorkspaceConnection(workspaceID)
		require.NoError(t, err)

		// Connection should work initially
		err = db.Ping()
		require.NoError(t, err)

		// Let connection idle for a bit
		t.Log("Waiting for connection to idle...")
		time.Sleep(3 * time.Second)

		// Connection should still work (database/sql handles reconnection)
		err = db.Ping()
		require.NoError(t, err, "Connection should still work after idling")

		// Query should also work
		var result int
		err = db.QueryRow("SELECT 1").Scan(&result)
		require.NoError(t, err)
		assert.Equal(t, 1, result)
	})

	t.Run("workspace database deleted externally", func(t *testing.T) {
		config := testutil.GetTestDatabaseConfig()
		pool := testutil.NewTestConnectionPool(config)
		defer _ = pool.Cleanup()

		workspaceID := "test_failure_deleted"

		// Create workspace and get connection
		err := pool.EnsureWorkspaceDatabase(workspaceID)
		require.NoError(t, err)

		db, err := pool.GetWorkspaceConnection(workspaceID)
		require.NoError(t, err)

		// Initial query should work
		var result int
		err = db.QueryRow("SELECT 1").Scan(&result)
		require.NoError(t, err)

		// Delete database externally (simulate external deletion)
		systemDB, err := pool.GetSystemConnection()
		require.NoError(t, err)

		dbName := fmt.Sprintf("%s_ws_%s", config.Prefix, workspaceID)

		// Terminate connections to the database
		err = testutil.TerminateAllConnections(t, systemDB, dbName)
		require.NoError(t, err)

		// Drop the database
		_, err = systemDB.Exec(fmt.Sprintf("DROP DATABASE IF EXISTS %s", dbName))
		require.NoError(t, err)

		// Next operation should fail gracefully
		err = db.QueryRow("SELECT 1").Scan(&result)
		assert.Error(t, err, "Query should fail when database is deleted")

		// Pool should handle the error gracefully (not panic)
		// Cleanup should still work
		err = pool.CleanupWorkspace(workspaceID)
		// Error is expected since database is already gone
		// The important thing is we don't panic
		t.Logf("Cleanup after external deletion: %v", err)
	})

	t.Run("connection pool handles invalid database name", func(t *testing.T) {
		config := testutil.GetTestDatabaseConfig()
		pool := testutil.NewTestConnectionPool(config)
		defer _ = pool.Cleanup()

		// Try to get connection to non-existent workspace
		// (without calling EnsureWorkspaceDatabase first)
		invalidWorkspaceID := "nonexistent_workspace_12345"

		_, err := pool.GetWorkspaceConnection(invalidWorkspaceID)
		// This should fail because database doesn't exist
		assert.Error(t, err, "Should error when database doesn't exist")

		// Pool should still be in valid state
		count := pool.GetConnectionCount()
		assert.Equal(t, 0, count, "Failed connection attempt should not create pool entry")
	})

	t.Run("recover from connection errors", func(t *testing.T) {
		config := testutil.GetTestDatabaseConfig()
		pool := testutil.NewTestConnectionPool(config)
		defer _ = pool.Cleanup()

		workspaceID := "test_failure_recover"

		// Create workspace
		err := pool.EnsureWorkspaceDatabase(workspaceID)
		require.NoError(t, err)

		db, err := pool.GetWorkspaceConnection(workspaceID)
		require.NoError(t, err)

		// Perform multiple operations with some failing
		successCount := 0
		errorCount := 0

		for i := 0; i < 10; i++ {
			// Alternate between valid and invalid queries
			var result int
			var err error

			if i%2 == 0 {
				// Valid query
				err = db.QueryRow("SELECT 1").Scan(&result)
			} else {
				// Invalid query
				err = db.QueryRow("SELECT * FROM nonexistent_table").Scan(&result)
			}

			if err != nil {
				errorCount++
			} else {
				successCount++
			}
		}

		assert.Equal(t, 5, successCount, "Valid queries should succeed")
		assert.Equal(t, 5, errorCount, "Invalid queries should error")

		// Pool should still be usable after errors
		var finalResult int
		err = db.QueryRow("SELECT 1").Scan(&finalResult)
		require.NoError(t, err, "Pool should be usable after errors")
		assert.Equal(t, 1, finalResult)
	})

	t.Run("concurrent failures don't crash pool", func(t *testing.T) {
		config := testutil.GetTestDatabaseConfig()
		pool := testutil.NewTestConnectionPool(config)
		defer _ = pool.Cleanup()

		workspaceID := "test_failure_concurrent"

		// Create workspace
		err := pool.EnsureWorkspaceDatabase(workspaceID)
		require.NoError(t, err)

		db, err := pool.GetWorkspaceConnection(workspaceID)
		require.NoError(t, err)

		// Multiple goroutines causing errors
		numGoroutines := 20
		done := make(chan struct{}, numGoroutines)

		for i := 0; i < numGoroutines; i++ {
			go func(id int) {
				defer func() {
					if r := recover(); r != nil {
						t.Errorf("Goroutine %d panicked: %v", id, r)
					}
					done <- struct{}{}
				}()

				// Mix of valid and invalid operations
				for j := 0; j < 5; j++ {
					var result int
					if j%2 == 0 {
						db.QueryRow("SELECT 1").Scan(&result)
					} else {
						db.QueryRow("SELECT * FROM nonexistent").Scan(&result)
					}

					time.Sleep(10 * time.Millisecond)
				}
			}(i)
		}

		// Wait for all goroutines
		for i := 0; i < numGoroutines; i++ {
			<-done
		}

		// Pool should still be functional
		var result int
		err = db.QueryRow("SELECT 1").Scan(&result)
		require.NoError(t, err, "Pool should be functional after concurrent errors")
	})

	t.Run("cleanup handles partially failed state", func(t *testing.T) {
		config := testutil.GetTestDatabaseConfig()
		pool := testutil.NewTestConnectionPool(config)
		defer _ = pool.Cleanup()

		// Create multiple workspaces
		workspaceIDs := []string{
			"test_failure_partial_1",
			"test_failure_partial_2",
			"test_failure_partial_3",
		}

		for _, wsID := range workspaceIDs {
			err := pool.EnsureWorkspaceDatabase(wsID)
			require.NoError(t, err)

			_, err = pool.GetWorkspaceConnection(wsID)
			require.NoError(t, err)
		}

		// Externally delete one database to create partial failure
		systemDB, err := pool.GetSystemConnection()
		require.NoError(t, err)

		dbName := fmt.Sprintf("%s_ws_%s", config.Prefix, workspaceIDs[1])
		err = testutil.TerminateAllConnections(t, systemDB, dbName)
		require.NoError(t, err)

		_, err = systemDB.Exec(fmt.Sprintf("DROP DATABASE IF EXISTS %s", dbName))
		require.NoError(t, err)

		// Cleanup should handle the partial failure gracefully
		err = _ = pool.Cleanup()
		// May return error, but shouldn't panic
		t.Logf("Cleanup with partial failure: %v", err)

		// Connection count should be reset
		assert.Equal(t, 0, pool.GetConnectionCount())
	})
}

// TestConnectionPoolSystemConnectionFailure tests system connection failures
func TestConnectionPoolSystemConnectionFailure(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping system connection failure tests in short mode")
	}

	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	t.Run("system connection retry", func(t *testing.T) {
		config := testutil.GetTestDatabaseConfig()
		pool := testutil.NewTestConnectionPool(config)
		defer _ = pool.Cleanup()

		// Get system connection multiple times
		for i := 0; i < 5; i++ {
			db, err := pool.GetSystemConnection()
			require.NoError(t, err, "Should be able to get system connection")
			require.NotNil(t, db)

			err = db.Ping()
			require.NoError(t, err, "System connection should be pingable")
		}
	})

	t.Run("workspace operations fail gracefully without system connection", func(t *testing.T) {
		cfg := testutil.GetTestDatabaseConfig()

		// Create pool with invalid connection details
		invalidConfig := &config.DatabaseConfig{
			Host:     cfg.Host,
			Port:     cfg.Port,
			User:     "invalid_user_xyz",
			Password: "invalid_password",
			Prefix:   "notifuse_test",
			SSLMode:  "disable",
		}

		pool := testutil.NewTestConnectionPool(invalidConfig)
		defer _ = pool.Cleanup()

		// System connection should fail
		_, err := pool.GetSystemConnection()
		assert.Error(t, err, "Should fail with invalid credentials")
	})
}

// TestConnectionPoolEdgeCases tests edge cases and boundary conditions
func TestConnectionPoolEdgeCases(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping connection pool edge case tests in short mode")
	}

	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	t.Run("empty workspace ID", func(t *testing.T) {
		config := testutil.GetTestDatabaseConfig()
		pool := testutil.NewTestConnectionPool(config)
		defer _ = pool.Cleanup()

		// Empty workspace ID should be handled gracefully
		err := pool.EnsureWorkspaceDatabase("")
		// May succeed or fail, but shouldn't panic
		t.Logf("EnsureWorkspaceDatabase with empty ID: %v", err)
	})

	t.Run("very long workspace ID", func(t *testing.T) {
		config := testutil.GetTestDatabaseConfig()
		pool := testutil.NewTestConnectionPool(config)
		defer _ = pool.Cleanup()

		// PostgreSQL has limits on identifier length
		// Use a reasonable but long ID
		longID := "test_failure_very_long_workspace_identifier_that_exceeds_normal_length"

		err := pool.EnsureWorkspaceDatabase(longID)
		// May succeed or fail depending on PostgreSQL limits
		t.Logf("EnsureWorkspaceDatabase with long ID: %v", err)

		if err == nil {
			// If it succeeded, cleanup should also work
			err = pool.CleanupWorkspace(longID)
			t.Logf("CleanupWorkspace with long ID: %v", err)
		}
	})

	t.Run("special characters in workspace ID", func(t *testing.T) {
		config := testutil.GetTestDatabaseConfig()
		pool := testutil.NewTestConnectionPool(config)
		defer _ = pool.Cleanup()

		// Workspace IDs with special characters (should be sanitized)
		specialIDs := []string{
			"test-with-dashes",
			"test_with_underscores",
			"test123numbers",
		}

		for _, wsID := range specialIDs {
			err := pool.EnsureWorkspaceDatabase(wsID)
			// Should handle these gracefully
			t.Logf("EnsureWorkspaceDatabase with ID '%s': %v", wsID, err)

			if err == nil {
				_, err = pool.GetWorkspaceConnection(wsID)
				t.Logf("GetWorkspaceConnection with ID '%s': %v", wsID, err)
			}
		}
	})

	t.Run("double cleanup idempotency", func(t *testing.T) {
		config := testutil.GetTestDatabaseConfig()
		pool := testutil.NewTestConnectionPool(config)

		workspaceID := "test_failure_double_cleanup"

		// Create and cleanup once
		err := pool.EnsureWorkspaceDatabase(workspaceID)
		require.NoError(t, err)

		_, err = pool.GetWorkspaceConnection(workspaceID)
		require.NoError(t, err)

		// First cleanup
		err = pool.CleanupWorkspace(workspaceID)
		require.NoError(t, err)

		// Second cleanup should not error
		err = pool.CleanupWorkspace(workspaceID)
		assert.NoError(t, err, "Double cleanup should be idempotent")

		// Full pool cleanup
		err = _ = pool.Cleanup()
		require.NoError(t, err)

		// Another cleanup should not error
		err = _ = pool.Cleanup()
		assert.NoError(t, err, "Double pool cleanup should be idempotent")
	})

	t.Run("nil database connection handling", func(t *testing.T) {
		config := testutil.GetTestDatabaseConfig()
		pool := testutil.NewTestConnectionPool(config)
		defer _ = pool.Cleanup()

		// Get system connection
		db, err := pool.GetSystemConnection()
		require.NoError(t, err)
		require.NotNil(t, db)

		// Verify connection stats can be retrieved
		stats := db.Stats()
		t.Logf("Connection stats: Open=%d, InUse=%d, Idle=%d",
			stats.OpenConnections, stats.InUse, stats.Idle)

		// Should not panic
		assert.GreaterOrEqual(t, stats.MaxOpenConnections, 1)
	})
}

// TestConnectionPoolConcurrentFailures tests handling of concurrent failures
func TestConnectionPoolConcurrentFailures(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping concurrent failure tests in short mode")
	}

	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	t.Run("concurrent creation with failures", func(t *testing.T) {
		config := testutil.GetTestDatabaseConfig()
		pool := testutil.NewTestConnectionPool(config)
		defer _ = pool.Cleanup()

		numGoroutines := 20
		done := make(chan error, numGoroutines)

		// Half create valid workspaces, half try invalid operations
		for i := 0; i < numGoroutines; i++ {
			go func(id int) {
				defer func() {
					if r := recover(); r != nil {
						done <- fmt.Errorf("panic: %v", r)
						return
					}
					done <- nil
				}()

				if id%2 == 0 {
					// Valid workspace
					wsID := fmt.Sprintf("test_concurrent_fail_valid_%d", id)
					err := pool.EnsureWorkspaceDatabase(wsID)
					if err != nil {
						done <- err
						return
					}
					_, err = pool.GetWorkspaceConnection(wsID)
					done <- err
				} else {
					// Try to get connection without ensuring database
					wsID := fmt.Sprintf("test_concurrent_fail_invalid_%d", id)
					_, err := pool.GetWorkspaceConnection(wsID)
					// Expected to fail, but shouldn't panic
					_ = err
					done <- nil
				}
			}(i)
		}

		// Collect results
		panicCount := 0
		for i := 0; i < numGoroutines; i++ {
			err := <-done
			if err != nil && err.Error() == "panic" {
				panicCount++
			}
		}

		assert.Equal(t, 0, panicCount, "Should not panic on concurrent failures")
	})
}
