package integration

import (
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/tests/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestConnectionPoolPerformance validates performance characteristics
func TestConnectionPoolPerformance(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance tests in short mode")
	}

	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	t.Run("connection reuse performance", func(t *testing.T) {
		config := testutil.GetTestDatabaseConfig()
		pool := testutil.NewTestConnectionPool(config)
		defer pool.Cleanup()

		workspaceID := "test_perf_reuse"

		// Ensure database
		err := pool.EnsureWorkspaceDatabase(workspaceID)
		require.NoError(t, err)

		// Get initial connection
		_, err = pool.GetWorkspaceConnection(workspaceID)
		require.NoError(t, err)

		// Measure time for 1000 operations with connection reuse
		numOperations := 1000
		start := time.Now()

		for i := 0; i < numOperations; i++ {
			db, err := pool.GetWorkspaceConnection(workspaceID)
			require.NoError(t, err)

			var result int
			err = db.QueryRow("SELECT 1").Scan(&result)
			require.NoError(t, err)
		}

		duration := time.Since(start)
		avgTime := duration / time.Duration(numOperations)

		t.Logf("Connection reuse: %d operations in %v (avg: %v per operation)",
			numOperations, duration, avgTime)

		// Should complete reasonably fast (less than 10 seconds for 1000 ops)
		assert.Less(t, duration, 10*time.Second,
			"Connection reuse should be efficient")

		// Average operation should be very fast (< 10ms)
		assert.Less(t, avgTime, 10*time.Millisecond,
			"Individual operations should be fast with connection reuse")
	})

	t.Run("high workspace count", func(t *testing.T) {
		config := testutil.GetTestDatabaseConfig()
		pool := testutil.NewTestConnectionPool(config)
		defer pool.Cleanup()

		// Create 25 workspace pools (reduced from 100 to avoid "too many clients")
		// Test environment has connection limits
		numWorkspaces := 25
		workspaceIDs := make([]string, numWorkspaces)

		start := time.Now()

		// Record memory at start
		var memStatsBefore runtime.MemStats
		runtime.ReadMemStats(&memStatsBefore)

		for i := 0; i < numWorkspaces; i++ {
			workspaceID := fmt.Sprintf("test_perf_high_count_%d", i)
			workspaceIDs[i] = workspaceID

			err := pool.EnsureWorkspaceDatabase(workspaceID)
			require.NoError(t, err)

			db, err := pool.GetWorkspaceConnection(workspaceID)
			require.NoError(t, err)

			// Perform operation on each
			var result int
			err = db.QueryRow("SELECT 1").Scan(&result)
			require.NoError(t, err)
		}

		duration := time.Since(start)

		// Record memory at end
		runtime.GC() // Force GC to get accurate measurement
		var memStatsAfter runtime.MemStats
		runtime.ReadMemStats(&memStatsAfter)

		// Calculate memory growth (handle potential underflow from GC)
		var memoryGrowth int64
		if memStatsAfter.Alloc > memStatsBefore.Alloc {
			memoryGrowth = int64(memStatsAfter.Alloc - memStatsBefore.Alloc)
		} else {
			memoryGrowth = 0
		}

		t.Logf("Created %d workspaces in %v", numWorkspaces, duration)
		t.Logf("Memory growth: %d MB", memoryGrowth/(1024*1024))

		// Verify all succeeded
		assert.LessOrEqual(t, pool.GetConnectionCount(), numWorkspaces)

		// Total time should be reasonable (< 30s for 25 workspaces)
		assert.Less(t, duration, 30*time.Second,
			"Should handle high workspace count efficiently")

		// Memory usage should be reasonable (< 50 MB for 25 workspaces)
		if memoryGrowth > 0 {
			assert.Less(t, memoryGrowth, int64(50*1024*1024),
				"Memory usage should be reasonable")
		}
	})

	t.Run("rapid create destroy cycles", func(t *testing.T) {
		config := testutil.GetTestDatabaseConfig()
		pool := testutil.NewTestConnectionPool(config)
		defer pool.Cleanup()

		// Rapidly create and destroy 50 workspaces, repeat 10 times
		numCycles := 10
		workspacesPerCycle := 5

		var totalDuration time.Duration
		var memStatsBefore, memStatsAfter runtime.MemStats

		runtime.ReadMemStats(&memStatsBefore)

		for cycle := 0; cycle < numCycles; cycle++ {
			cycleStart := time.Now()

			// Create workspaces
			workspaceIDs := make([]string, workspacesPerCycle)
			for i := 0; i < workspacesPerCycle; i++ {
				workspaceID := fmt.Sprintf("test_perf_rapid_%d_%d", cycle, i)
				workspaceIDs[i] = workspaceID

				err := pool.EnsureWorkspaceDatabase(workspaceID)
				require.NoError(t, err)

				_, err = pool.GetWorkspaceConnection(workspaceID)
				require.NoError(t, err)
			}

			// Destroy workspaces
			for _, workspaceID := range workspaceIDs {
				err := pool.CleanupWorkspace(workspaceID)
				require.NoError(t, err)
			}

			cycleDuration := time.Since(cycleStart)
			totalDuration += cycleDuration

			t.Logf("Cycle %d/%d: %v", cycle+1, numCycles, cycleDuration)
		}

		runtime.GC()
		runtime.ReadMemStats(&memStatsAfter)

		avgCycleDuration := totalDuration / time.Duration(numCycles)
		memoryGrowth := int64(memStatsAfter.Alloc) - int64(memStatsBefore.Alloc)

		t.Logf("Average cycle duration: %v", avgCycleDuration)
		t.Logf("Memory growth: %d KB", memoryGrowth/1024)

		// No memory leaks: memory growth should be minimal
		// Allow some growth due to runtime overhead, but not excessive
		assert.Less(t, memoryGrowth, int64(10*1024*1024),
			"Should not leak significant memory across cycles")

		// Performance should be stable across iterations
		assert.Less(t, avgCycleDuration, 5*time.Second,
			"Create/destroy cycles should be reasonably fast")
	})

	t.Run("idle connection cleanup overhead", func(t *testing.T) {
		config := testutil.GetTestDatabaseConfig()
		pool := testutil.NewTestConnectionPool(config)
		defer pool.Cleanup()

		// Create 10 workspace pools (reduced from 20 to avoid exhaustion)
		numWorkspaces := 10
		workspaceIDs := make([]string, numWorkspaces)

		for i := 0; i < numWorkspaces; i++ {
			workspaceID := fmt.Sprintf("test_perf_idle_%d", i)
			workspaceIDs[i] = workspaceID

			err := pool.EnsureWorkspaceDatabase(workspaceID)
			require.NoError(t, err)

			_, err = pool.GetWorkspaceConnection(workspaceID)
			require.NoError(t, err)
		}

		initialCount := pool.GetConnectionCount()
		assert.Equal(t, numWorkspaces, initialCount)

		// Record memory before idle period
		var memStatsBefore runtime.MemStats
		runtime.ReadMemStats(&memStatsBefore)

		// Let connections idle
		t.Log("Letting connections idle for 3 seconds...")
		time.Sleep(3 * time.Second)

		// Record memory after idle period
		runtime.GC()
		var memStatsAfter runtime.MemStats
		runtime.ReadMemStats(&memStatsAfter)

		finalCount := pool.GetConnectionCount()
		memoryFreed := int64(memStatsBefore.Alloc) - int64(memStatsAfter.Alloc)

		t.Logf("Connection count: before=%d, after=%d", initialCount, finalCount)
		t.Logf("Memory change: %d KB", memoryFreed/1024)

		// Connection pool count should remain stable
		// (sql.DB handles its own connection recycling internally)
		assert.Equal(t, initialCount, finalCount,
			"Pool count should remain stable during idle period")
	})

	t.Run("concurrent query performance", func(t *testing.T) {
		config := testutil.GetTestDatabaseConfig()
		pool := testutil.NewTestConnectionPool(config)
		defer pool.Cleanup()

		// Create 5 workspaces
		numWorkspaces := 5
		workspaceIDs := make([]string, numWorkspaces)

		for i := 0; i < numWorkspaces; i++ {
			workspaceID := fmt.Sprintf("test_perf_concurrent_%d", i)
			workspaceIDs[i] = workspaceID

			err := pool.EnsureWorkspaceDatabase(workspaceID)
			require.NoError(t, err)

			_, err = pool.GetWorkspaceConnection(workspaceID)
			require.NoError(t, err)
		}

		// Run 1000 concurrent queries across all workspaces
		numQueries := 1000
		var wg sync.WaitGroup
		var successCount int32

		start := time.Now()

		for i := 0; i < numQueries; i++ {
			wg.Add(1)

			go func(queryID int) {
				defer wg.Done()

				// Round-robin across workspaces
				workspaceID := workspaceIDs[queryID%numWorkspaces]

				db, err := pool.GetWorkspaceConnection(workspaceID)
				if err != nil {
					return
				}

				var result int
				err = db.QueryRow("SELECT $1", queryID).Scan(&result)
				if err == nil {
					atomic.AddInt32(&successCount, 1)
				}
			}(i)
		}

		wg.Wait()
		duration := time.Since(start)

		qps := float64(successCount) / duration.Seconds()

		t.Logf("Concurrent queries: %d/%d successful in %v (%.0f QPS)",
			successCount, numQueries, duration, qps)

		// All queries should succeed
		assert.Equal(t, int32(numQueries), successCount,
			"All concurrent queries should succeed")

		// Should handle reasonable throughput (> 100 QPS)
		assert.Greater(t, qps, 100.0,
			"Should handle at least 100 queries per second")
	})

	t.Run("memory efficiency with large result sets", func(t *testing.T) {
		config := testutil.GetTestDatabaseConfig()
		pool := testutil.NewTestConnectionPool(config)
		defer pool.Cleanup()

		workspaceID := "test_perf_memory"

		// Create workspace and table
		err := pool.EnsureWorkspaceDatabase(workspaceID)
		require.NoError(t, err)

		db, err := pool.GetWorkspaceConnection(workspaceID)
		require.NoError(t, err)

		// Create test table with some data
		_, err = db.Exec(`
			CREATE TABLE IF NOT EXISTS test_large (
				id SERIAL PRIMARY KEY,
				data TEXT
			)
		`)
		require.NoError(t, err)

		// Insert test data
		for i := 0; i < 1000; i++ {
			_, err = db.Exec("INSERT INTO test_large (data) VALUES ($1)",
				fmt.Sprintf("test_data_%d_with_some_content_to_make_it_larger", i))
			require.NoError(t, err)
		}

		// Measure memory before queries
		runtime.GC()
		var memStatsBefore runtime.MemStats
		runtime.ReadMemStats(&memStatsBefore)

		// Run multiple large queries
		for i := 0; i < 10; i++ {
			rows, err := db.Query("SELECT id, data FROM test_large")
			require.NoError(t, err)

			count := 0
			for rows.Next() {
				var id int
				var data string
				err = rows.Scan(&id, &data)
				require.NoError(t, err)
				count++
			}
			rows.Close()

			assert.Equal(t, 1000, count, "Should retrieve all rows")
		}

		// Measure memory after queries
		runtime.GC()
		var memStatsAfter runtime.MemStats
		runtime.ReadMemStats(&memStatsAfter)

		// Calculate memory growth (handle potential underflow from GC)
		var memoryGrowth int64
		if memStatsAfter.Alloc > memStatsBefore.Alloc {
			memoryGrowth = int64(memStatsAfter.Alloc - memStatsBefore.Alloc)
		} else {
			// GC may have reduced memory - this is actually good
			memoryGrowth = 0
		}

		t.Logf("Memory growth for large queries: %d KB", memoryGrowth/1024)

		// Memory usage should be reasonable (< 50 MB growth)
		// Note: GC may actually reduce memory, which is fine
		if memoryGrowth > 0 {
			assert.Less(t, memoryGrowth, int64(50*1024*1024),
				"Should not use excessive memory for large result sets")
		}
	})

	t.Run("connection pool warmup time", func(t *testing.T) {
		config := testutil.GetTestDatabaseConfig()

		// Measure time to create and initialize pool
		start := time.Now()

		pool := testutil.NewTestConnectionPool(config)
		defer pool.Cleanup()

		// Get system connection
		_, err := pool.GetSystemConnection()
		require.NoError(t, err)

		// Create first workspace
		workspaceID := "test_perf_warmup"
		err = pool.EnsureWorkspaceDatabase(workspaceID)
		require.NoError(t, err)

		_, err = pool.GetWorkspaceConnection(workspaceID)
		require.NoError(t, err)

		warmupTime := time.Since(start)

		t.Logf("Pool warmup time: %v", warmupTime)

		// Warmup should be reasonably fast (< 5 seconds)
		assert.Less(t, warmupTime, 5*time.Second,
			"Pool warmup should be fast")
	})
}

// TestConnectionPoolScalability tests behavior under scaling scenarios
func TestConnectionPoolScalability(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping scalability tests in short mode")
	}

	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	t.Run("linear scaling with workspace count", func(t *testing.T) {
		config := testutil.GetTestDatabaseConfig()
		pool := testutil.NewTestConnectionPool(config)
		defer pool.Cleanup()

		// Test different workspace counts and measure time
		workspaceCounts := []int{5, 10, 20}
		timings := make([]time.Duration, len(workspaceCounts))

		for idx, count := range workspaceCounts {
			start := time.Now()

			for i := 0; i < count; i++ {
				workspaceID := fmt.Sprintf("test_scale_linear_%d_%d", idx, i)

				err := pool.EnsureWorkspaceDatabase(workspaceID)
				require.NoError(t, err)

				db, err := pool.GetWorkspaceConnection(workspaceID)
				require.NoError(t, err)

				var result int
				err = db.QueryRow("SELECT 1").Scan(&result)
				require.NoError(t, err)
			}

			timings[idx] = time.Since(start)

			t.Logf("Created %d workspaces in %v", count, timings[idx])

			// Cleanup for next iteration
			for i := 0; i < count; i++ {
				workspaceID := fmt.Sprintf("test_scale_linear_%d_%d", idx, i)
				pool.CleanupWorkspace(workspaceID)
			}
		}

		// Verify scaling is reasonable (not exponential)
		// Doubling workspaces shouldn't more than triple the time
		for i := 1; i < len(timings); i++ {
			ratio := float64(timings[i]) / float64(timings[i-1])
			countRatio := float64(workspaceCounts[i]) / float64(workspaceCounts[i-1])

			t.Logf("Time ratio: %.2f for count ratio: %.2f", ratio, countRatio)

			// Time should scale roughly linearly (allow 1.5x factor)
			assert.Less(t, ratio, countRatio*1.5,
				"Time should scale roughly linearly with workspace count")
		}
	})

	t.Run("throughput under sustained load", func(t *testing.T) {
		config := testutil.GetTestDatabaseConfig()
		pool := testutil.NewTestConnectionPool(config)
		defer pool.Cleanup()

		// Create 10 workspaces
		numWorkspaces := 10
		workspaceIDs := make([]string, numWorkspaces)

		for i := 0; i < numWorkspaces; i++ {
			workspaceID := fmt.Sprintf("test_scale_sustained_%d", i)
			workspaceIDs[i] = workspaceID

			err := pool.EnsureWorkspaceDatabase(workspaceID)
			require.NoError(t, err)

			_, err = pool.GetWorkspaceConnection(workspaceID)
			require.NoError(t, err)
		}

		// Sustained load for 10 seconds
		duration := 10 * time.Second
		stopChan := make(chan struct{})
		var operationCount int64

		// Start workers
		numWorkers := 20
		var wg sync.WaitGroup

		for i := 0; i < numWorkers; i++ {
			wg.Add(1)

			go func(workerID int) {
				defer wg.Done()

				for {
					select {
					case <-stopChan:
						return
					default:
						// Pick random workspace
						wsID := workspaceIDs[workerID%numWorkspaces]
						db, err := pool.GetWorkspaceConnection(wsID)
						if err != nil {
							continue
						}

						var result int
						err = db.QueryRow("SELECT 1").Scan(&result)
						if err == nil {
							atomic.AddInt64(&operationCount, 1)
						}
					}
				}
			}(i)
		}

		// Let it run
		time.Sleep(duration)
		close(stopChan)
		wg.Wait()

		totalOps := atomic.LoadInt64(&operationCount)
		opsPerSecond := float64(totalOps) / duration.Seconds()

		t.Logf("Sustained load: %d operations in %v (%.0f ops/sec)",
			totalOps, duration, opsPerSecond)

		// Should handle reasonable sustained throughput
		assert.Greater(t, opsPerSecond, 500.0,
			"Should handle at least 500 ops/sec under sustained load")
	})
}
