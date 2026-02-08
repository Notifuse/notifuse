package integration

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/tests/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestConnectionPoolConcurrency tests thread-safety and concurrent performance
func TestConnectionPoolConcurrency(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping connection pool concurrency tests in short mode")
	}

	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	t.Run("concurrent workspace creation", func(t *testing.T) {
		config := testutil.GetTestDatabaseConfig()
		pool := testutil.NewTestConnectionPoolWithTiming(config, testutil.FastTimingConfig())
		defer func() { _ = pool.Cleanup() }()

		numGoroutines := 25 // Reduced from 50 to avoid connection exhaustion
		var wg sync.WaitGroup
		errors := make(chan error, numGoroutines)
		workspaceIDs := make([]string, numGoroutines)

		// 25 goroutines request different workspaces simultaneously
		for i := 0; i < numGoroutines; i++ {
			workspaceIDs[i] = fmt.Sprintf("test_concurrent_create_%d", i)
			wg.Add(1)

			go func(id int) {
				defer wg.Done()

				workspaceID := workspaceIDs[id]

				// Ensure database
				if err := pool.EnsureWorkspaceDatabase(workspaceID); err != nil {
					errors <- fmt.Errorf("failed to ensure database %s: %w", workspaceID, err)
					return
				}

				// Get connection
				db, err := pool.GetWorkspaceConnection(workspaceID)
				if err != nil {
					errors <- fmt.Errorf("failed to get connection %s: %w", workspaceID, err)
					return
				}

				// Test connection
				if err := db.Ping(); err != nil {
					errors <- fmt.Errorf("failed to ping %s: %w", workspaceID, err)
					return
				}

				errors <- nil
			}(i)
		}

		// Wait for all goroutines
		wg.Wait()
		close(errors)

		// Check for errors
		errorCount := 0
		for err := range errors {
			if err != nil {
				t.Logf("Error: %v", err)
				errorCount++
			}
		}

		assert.Equal(t, 0, errorCount, "All concurrent creations should succeed")
		assert.Equal(t, numGoroutines, pool.GetConnectionCount(), "Should have all workspace connections")

		// Explicit cleanup to release connections faster
		err := pool.Cleanup()
		require.NoError(t, err)
	})

	t.Run("concurrent same workspace access", func(t *testing.T) {
		config := testutil.GetTestDatabaseConfig()
		pool := testutil.NewTestConnectionPoolWithTiming(config, testutil.FastTimingConfig())
		defer func() { _ = pool.Cleanup() }()

		workspaceID := "test_concurrent_same"

		// Ensure database exists first
		err := pool.EnsureWorkspaceDatabase(workspaceID)
		require.NoError(t, err)

		numGoroutines := 50 // Reduced from 100 to be less aggressive
		var wg sync.WaitGroup
		errors := make(chan error, numGoroutines)
		connections := make(chan interface{}, numGoroutines)

		// 50 goroutines request same workspace
		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)

			go func() {
				defer wg.Done()

				db, err := pool.GetWorkspaceConnection(workspaceID)
				if err != nil {
					errors <- err
					return
				}

				// Test connection
				if err := db.Ping(); err != nil {
					errors <- err
					return
				}

				connections <- db
				errors <- nil
			}()
		}

		wg.Wait()
		close(errors)
		close(connections)

		// Check for errors
		errorCount := 0
		for err := range errors {
			if err != nil {
				errorCount++
			}
		}

		assert.Equal(t, 0, errorCount, "All concurrent accesses should succeed")

		// All goroutines should get the same connection pool instance
		var firstConn interface{}
		sameConnection := true
		for conn := range connections {
			if firstConn == nil {
				firstConn = conn
			} else if conn != firstConn {
				sameConnection = false
				break
			}
		}

		assert.True(t, sameConnection, "All goroutines should get same connection pool")

		// Connection count should be 1 (same workspace)
		count := pool.GetConnectionCount()
		assert.Equal(t, 1, count, "Should have only 1 workspace connection")
	})

	t.Run("concurrent read write operations", func(t *testing.T) {
		config := testutil.GetTestDatabaseConfig()
		pool := testutil.NewTestConnectionPoolWithTiming(config, testutil.FastTimingConfig())
		defer func() { _ = pool.Cleanup() }()

		// Create 5 workspaces (reduced to avoid connection exhaustion)
		numWorkspaces := 5
		workspaceIDs := make([]string, numWorkspaces)
		for i := 0; i < numWorkspaces; i++ {
			workspaceID := fmt.Sprintf("test_concurrent_rw_%d", i)
			workspaceIDs[i] = workspaceID

			err := pool.EnsureWorkspaceDatabase(workspaceID)
			require.NoError(t, err)

			db, err := pool.GetWorkspaceConnection(workspaceID)
			require.NoError(t, err)

			// Create a simple test table
			_, err = db.Exec(`
				CREATE TABLE IF NOT EXISTS test_data (
					id SERIAL PRIMARY KEY,
					value INTEGER NOT NULL
				)
			`)
			require.NoError(t, err)
		}

		// Multiple goroutines read/write to different workspaces
		numOperations := 100
		var wg sync.WaitGroup
		var successCount int32
		errors := make(chan error, numOperations)

		for i := 0; i < numOperations; i++ {
			wg.Add(1)

			go func(opID int) {
				defer wg.Done()

				// Pick a random workspace
				workspaceID := workspaceIDs[opID%numWorkspaces]
				db, err := pool.GetWorkspaceConnection(workspaceID)
				if err != nil {
					errors <- err
					return
				}

				// Alternate between read and write
				if opID%2 == 0 {
					// Write operation
					_, err = db.Exec("INSERT INTO test_data (value) VALUES ($1)", opID)
				} else {
					// Read operation
					var count int
					err = db.QueryRow("SELECT COUNT(*) FROM test_data").Scan(&count)
				}

				if err != nil {
					errors <- err
				} else {
					atomic.AddInt32(&successCount, 1)
					errors <- nil
				}
			}(i)
		}

		wg.Wait()
		close(errors)

		// Check for errors
		errorCount := 0
		for err := range errors {
			if err != nil {
				t.Logf("Operation error: %v", err)
				errorCount++
			}
		}

		assert.Equal(t, 0, errorCount, "All concurrent operations should succeed")
		assert.Equal(t, int32(numOperations), successCount, "All operations should be successful")
	})

	t.Run("concurrent cleanup", func(t *testing.T) {
		config := testutil.GetTestDatabaseConfig()
		pool := testutil.NewTestConnectionPoolWithTiming(config, testutil.FastTimingConfig())
		defer func() { _ = pool.Cleanup() }()

		// Create multiple workspaces (reduced to avoid connection exhaustion)
		numWorkspaces := 10
		workspaceIDs := make([]string, numWorkspaces)
		for i := 0; i < numWorkspaces; i++ {
			workspaceID := fmt.Sprintf("test_concurrent_cleanup_%d", i)
			workspaceIDs[i] = workspaceID

			err := pool.EnsureWorkspaceDatabase(workspaceID)
			require.NoError(t, err)

			_, err = pool.GetWorkspaceConnection(workspaceID)
			require.NoError(t, err)
		}

		assert.Equal(t, numWorkspaces, pool.GetConnectionCount())

		// Multiple goroutines close different workspaces concurrently
		var wg sync.WaitGroup
		errors := make(chan error, numWorkspaces)

		for i := 0; i < numWorkspaces; i++ {
			wg.Add(1)

			go func(id int) {
				defer wg.Done()

				workspaceID := workspaceIDs[id]
				err := pool.CleanupWorkspace(workspaceID)
				errors <- err
			}(i)
		}

		wg.Wait()
		close(errors)

		// Check for errors
		errorCount := 0
		for err := range errors {
			if err != nil {
				t.Logf("Cleanup error: %v", err)
				errorCount++
			}
		}

		assert.Equal(t, 0, errorCount, "All concurrent cleanups should succeed")

		// Final state should be clean
		count := pool.GetConnectionCount()
		assert.Equal(t, 0, count, "All connections should be cleaned up")
	})

	t.Run("race detector stress test", func(t *testing.T) {
		// This test is specifically designed to trigger race conditions
		// Run with: go test -race
		config := testutil.GetTestDatabaseConfig()
		pool := testutil.NewTestConnectionPoolWithTiming(config, testutil.FastTimingConfig())
		defer func() { _ = pool.Cleanup() }()

		workspaceID := "test_race_detector"
		err := pool.EnsureWorkspaceDatabase(workspaceID)
		require.NoError(t, err)

		// Stress test: many goroutines doing various operations
		numGoroutines := 30                // Reduced from 50
		duration := 300 * time.Millisecond // Reduced for faster tests
		stopChan := make(chan struct{})
		var wg sync.WaitGroup

		// Start goroutines
		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)

			go func(id int) {
				defer wg.Done()

				for {
					select {
					case <-stopChan:
						return
					default:
						// Randomly perform different operations
						switch id % 4 {
						case 0:
							// Get connection
							db, err := pool.GetWorkspaceConnection(workspaceID)
							if err == nil && db != nil {
								db.Ping()
							}
						case 1:
							// Check connection count
							pool.GetConnectionCount()
						case 2:
							// Get system connection
							sysDB, err := pool.GetSystemConnection()
							if err == nil && sysDB != nil {
								sysDB.Ping()
							}
						case 3:
							// Ensure database (idempotent)
							pool.EnsureWorkspaceDatabase(workspaceID)
						}

						time.Sleep(10 * time.Millisecond)
					}
				}
			}(i)
		}

		// Let it run for specified duration
		time.Sleep(duration)
		close(stopChan)
		wg.Wait()

		// If we got here without panics, race detector is happy
		t.Log("Race detector stress test completed successfully")
	})

	t.Run("high contention on single workspace", func(t *testing.T) {
		config := testutil.GetTestDatabaseConfig()
		pool := testutil.NewTestConnectionPoolWithTiming(config, testutil.FastTimingConfig())
		defer func() { _ = pool.Cleanup() }()

		workspaceID := "test_high_contention"
		err := pool.EnsureWorkspaceDatabase(workspaceID)
		require.NoError(t, err)

		// High contention: 100 goroutines all trying to access same workspace (reduced from 200)
		numGoroutines := 100
		var wg sync.WaitGroup
		var successCount int32

		startTime := time.Now()

		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)

			go func() {
				defer wg.Done()

				db, err := pool.GetWorkspaceConnection(workspaceID)
				if err != nil {
					return
				}

				// Perform a simple query
				var result int
				err = db.QueryRow("SELECT 1").Scan(&result)
				if err == nil && result == 1 {
					atomic.AddInt32(&successCount, 1)
				}
			}()
		}

		wg.Wait()
		duration := time.Since(startTime)

		t.Logf("High contention test completed in %v", duration)
		t.Logf("Success rate: %d/%d", successCount, numGoroutines)

		assert.Equal(t, int32(numGoroutines), successCount, "All operations should succeed under high contention")
		assert.Equal(t, 1, pool.GetConnectionCount(), "Should still have only 1 connection pool")

		// Performance check: should complete reasonably fast
		assert.Less(t, duration, 10*time.Second, "High contention should be handled efficiently")
	})
}

// Note: Rapid create/destroy test removed to avoid connection exhaustion
// This scenario is already covered in connection_pool_performance_test.go
// which runs in isolation and is better suited for this type of testing
