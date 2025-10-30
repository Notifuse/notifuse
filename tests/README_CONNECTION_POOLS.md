# Connection Pool Testing Infrastructure

## Overview

This document describes the connection pool testing infrastructure for Notifuse. The infrastructure provides comprehensive testing of database connection pooling with proper isolation, cleanup, and leak detection.

## Architecture

### Production Connection Manager

Location: `pkg/database/connection_manager.go`

The production connection manager handles:
- System database connection (singleton)
- Workspace database connection pools (one per workspace)
- LRU eviction when capacity is reached
- Connection limits and statistics
- Thread-safe operations

### Test Connection Pool

Location: `tests/testutil/connection_pool.go`

The test connection pool provides:
- Isolated connection pools for integration tests
- System and workspace database connections
- Proper cleanup with leak verification
- Connection statistics tracking

### Test Pool Manager

Location: `tests/testutil/connection_pool_manager.go`

The test pool manager provides:
- Per-test connection pool isolation
- Multiple isolated pools for concurrent tests
- Centralized cleanup
- Connection metrics tracking

### Helper Utilities

Location: `tests/testutil/connection_pool_helpers.go`

Helper functions include:
- `VerifyNoLeakedConnections()` - Checks for leaked connections
- `GetActiveConnectionCount()` - Queries PostgreSQL for connection count
- `CreateTestWorkspaces()` - Bulk workspace creation
- `CleanupTestWorkspaces()` - Bulk workspace cleanup
- `MeasureOperationTime()` - Performance measurement
- `WaitForDatabaseReady()` - Connection readiness verification

## Test Suites

### Lifecycle Tests (`connection_pool_lifecycle_test.go`)

Tests the complete lifecycle of connection pools:

1. **Pool Initialization** - Verify pool starts correctly
2. **Workspace Pool Creation** - Create and verify workspace databases
3. **Workspace Pool Reuse** - Ensure same workspace returns same connection
4. **Workspace Pool Cleanup** - Verify proper cleanup of individual workspaces
5. **Full Cleanup** - Verify all connections are released
6. **Cleanup Idempotency** - Multiple cleanups don't error
7. **Multiple Pools Isolated** - Different pools don't interfere
8. **Pool Manager Isolation** - Test manager properly isolates tests
9. **Metrics Tracking** - Verify metrics collection works

### Concurrency Tests (`connection_pool_concurrency_test.go`)

Tests thread-safety and concurrent access:

1. **Concurrent Workspace Creation** - 50 goroutines create different workspaces
2. **Concurrent Same Workspace Access** - 100 goroutines access same workspace
3. **Concurrent Read/Write Operations** - Multiple goroutines perform operations
4. **Concurrent Cleanup** - Multiple goroutines cleanup different workspaces
5. **Race Detector Stress Test** - Stress test with race detector
6. **High Contention** - 200 goroutines accessing single workspace
7. **Rapid Create/Destroy** - Quick creation and destruction cycles

### Limits Tests (`connection_pool_limits_test.go`)

Tests connection limit enforcement:

1. **Max Connections Respected** - Verify connection limits work
2. **Connection Reuse** - Same workspace returns same connection pool
3. **Connection Timeout Handling** - Verify timeout behavior
4. **Idle Connection Cleanup** - Test idle connection handling
5. **Connection Stats Accuracy** - Verify statistics are correct
6. **Max Open Connections Per Database** - Per-workspace limits work
7. **Connection Limit Protects System** - Limits prevent resource exhaustion
8. **No Connection Leaks on Error** - Errors don't leak connections
9. **Cleanup Releases All Resources** - Full cleanup works properly

### Failure Tests (`connection_pool_failure_test.go`)

Tests error handling and recovery:

1. **Stale Connection Detection** - Detect and handle stale connections
2. **Workspace Database Deleted Externally** - Handle external deletion
3. **Invalid Database Name** - Handle non-existent databases
4. **Recover from Connection Errors** - Continue working after errors
5. **Concurrent Failures** - Multiple goroutines causing errors
6. **Cleanup Handles Partially Failed State** - Partial cleanup works
7. **System Connection Retry** - System connection can be re-acquired
8. **Edge Cases** - Empty IDs, long IDs, special characters
9. **Double Cleanup Idempotency** - Multiple cleanups are safe

### Performance Tests (`connection_pool_performance_test.go`)

Tests performance characteristics:

1. **Connection Reuse Performance** - Verify reuse is fast
2. **High Workspace Count** - Handle 100+ workspaces
3. **Rapid Create/Destroy Cycles** - No memory leaks over time
4. **Idle Connection Cleanup Overhead** - Minimal overhead
5. **Concurrent Query Performance** - High throughput (>100 QPS)
6. **Memory Efficiency** - Reasonable memory usage with large datasets
7. **Connection Pool Warmup Time** - Fast initialization
8. **Linear Scaling** - Time scales linearly with workspace count
9. **Throughput Under Sustained Load** - Stable performance over time

## Running Tests

### Run All Connection Pool Tests

```bash
# Run all connection pool integration tests
make test-connection-pools

# Run with race detector (recommended)
make test-connection-pools-race

# Run specific test file
go test -v ./tests/integration -run TestConnectionPoolLifecycle

# Run specific test case
go test -v ./tests/integration -run TestConnectionPoolConcurrency/concurrent_workspace_creation
```

### Run Performance Tests

```bash
# Performance tests are skipped in short mode
go test -v ./tests/integration -run TestConnectionPoolPerformance

# Run scalability tests
go test -v ./tests/integration -run TestConnectionPoolScalability
```

### Check for Connection Leaks

```bash
# Run tests with leak detection
make test-connection-pools-leak-check

# Manual leak check using PostgreSQL
psql -h localhost -U notifuse_test -d postgres -c \
  "SELECT count(*) FROM pg_stat_activity WHERE usename = 'notifuse_test';"
```

## Writing New Connection Pool Tests

### Basic Test Structure

```go
func TestMyConnectionPoolFeature(t *testing.T) {
    // Skip if running in short mode (for performance tests)
    if testing.Short() {
        t.Skip("Skipping in short mode")
    }

    // Setup environment
    testutil.SetupTestEnvironment()
    defer testutil.CleanupTestEnvironment()

    t.Run("my test case", func(t *testing.T) {
        // Get test configuration
        config := testutil.GetTestDatabaseConfig()
        
        // Create isolated pool for this test
        pool := testutil.NewTestConnectionPool(config)
        defer pool.Cleanup()

        // Create workspace
        workspaceID := "test_my_feature"
        err := pool.EnsureWorkspaceDatabase(workspaceID)
        require.NoError(t, err)

        // Get connection
        db, err := pool.GetWorkspaceConnection(workspaceID)
        require.NoError(t, err)

        // Perform test operations
        var result int
        err = db.QueryRow("SELECT 1").Scan(&result)
        require.NoError(t, err)
        assert.Equal(t, 1, result)

        // Cleanup is handled by defer
    })
}
```

### Using Test Pool Manager for Isolation

```go
func TestWithPoolManager(t *testing.T) {
    testutil.SetupTestEnvironment()
    defer testutil.CleanupTestEnvironment()

    t.Run("isolated test 1", func(t *testing.T) {
        config := testutil.GetTestDatabaseConfig()
        manager := testutil.NewTestConnectionPoolManager()
        defer manager.CleanupAll()

        // Each sub-test gets its own pool
        pool1 := manager.GetOrCreatePool("test1", config)
        pool2 := manager.GetOrCreatePool("test2", config)

        // Pools are isolated
        // ... perform tests ...
    })
}
```

### Measuring Performance

```go
func TestPerformance(t *testing.T) {
    config := testutil.GetTestDatabaseConfig()
    pool := testutil.NewTestConnectionPool(config)
    defer pool.Cleanup()

    // Use helper to measure operation time
    duration := testutil.MeasureOperationTime(t, "my operation", func() {
        // Perform operation
        workspaceID := "test_perf"
        pool.EnsureWorkspaceDatabase(workspaceID)
        pool.GetWorkspaceConnection(workspaceID)
    })

    // Assert performance requirements
    assert.Less(t, duration, 1*time.Second, "Should be fast")
}
```

### Verifying No Connection Leaks

```go
func TestNoLeaks(t *testing.T) {
    config := testutil.GetTestDatabaseConfig()
    pool := testutil.NewTestConnectionPool(config)
    defer pool.Cleanup()

    systemDB, err := pool.GetSystemConnection()
    require.NoError(t, err)

    // Perform operations
    // ...

    // Cleanup
    err = pool.Cleanup()
    require.NoError(t, err)

    // Wait for connections to close
    time.Sleep(500 * time.Millisecond)

    // Verify no leaked connections
    testutil.VerifyNoLeakedConnections(t, systemDB, config.User)
}
```

## Common Pitfalls and Solutions

### Problem: Tests Hang

**Cause**: Connection pool not properly cleaned up between tests

**Solution**: Always use `defer pool.Cleanup()` immediately after creating a pool

```go
pool := testutil.NewTestConnectionPool(config)
defer pool.Cleanup() // Always defer cleanup
```

### Problem: Connection Leaks

**Cause**: Connections not closed, databases not dropped

**Solution**: Use the improved `Cleanup()` method which:
1. Closes all workspace connections
2. Waits for connections to close
3. Drops workspace databases
4. Closes system connection

### Problem: Tests Fail Intermittently

**Cause**: Race conditions in concurrent tests

**Solution**: Always run tests with race detector:

```bash
go test -race -v ./tests/integration
```

### Problem: Tests Too Slow

**Cause**: Creating too many workspaces or not reusing connections

**Solution**:
- Reuse connections where possible
- Use smaller workspace counts for tests
- Skip performance tests in short mode
- Run expensive tests selectively

### Problem: "Database Already Exists" Error

**Cause**: Previous test didn't clean up properly

**Solution**: Ensure cleanup is always called, even if test fails:

```go
pool := testutil.NewTestConnectionPool(config)
defer pool.Cleanup() // Will run even if test panics
```

## Debugging Connection Issues

### Check Active Connections

```sql
-- View all active connections
SELECT 
    pid, usename, application_name, client_addr, 
    state, query_start, state_change, query
FROM pg_stat_activity 
WHERE usename = 'notifuse_test'
ORDER BY state_change DESC;

-- Count connections by state
SELECT state, count(*) 
FROM pg_stat_activity 
WHERE usename = 'notifuse_test'
GROUP BY state;
```

### Terminate Stuck Connections

```sql
-- Terminate all test user connections
SELECT pg_terminate_backend(pid) 
FROM pg_stat_activity 
WHERE usename = 'notifuse_test' 
  AND pid != pg_backend_pid();
```

### View Connection Pool Stats

```go
// In your test
pool := testutil.NewTestConnectionPool(config)
defer pool.Cleanup()

db, _ := pool.GetWorkspaceConnection(workspaceID)
stats := db.Stats()

fmt.Printf("Open: %d, InUse: %d, Idle: %d, MaxOpen: %d\n",
    stats.OpenConnections, stats.InUse, stats.Idle, stats.MaxOpenConnections)
```

## CI/CD Integration

### GitHub Actions Example

```yaml
test-connection-pools:
  runs-on: ubuntu-latest
  services:
    postgres:
      image: postgres:17-alpine
      env:
        POSTGRES_USER: notifuse_test
        POSTGRES_PASSWORD: test_password
      options: >-
        --health-cmd pg_isready
        --health-interval 10s
        --health-timeout 5s
        --health-retries 5

  steps:
    - uses: actions/checkout@v4
    
    - name: Set up Go
      uses: actions/setup-go@v4
      with:
        go-version: '1.23'
    
    - name: Run Connection Pool Tests
      env:
        INTEGRATION_TESTS: true
        TEST_DB_HOST: localhost
        TEST_DB_PORT: 5432
        TEST_DB_USER: notifuse_test
        TEST_DB_PASSWORD: test_password
      run: make test-connection-pools-race
```

## Performance Benchmarks

### Target Metrics

- **Connection Reuse**: < 10ms per operation
- **Workspace Creation**: < 500ms per workspace
- **High Workspace Count**: 100 workspaces in < 60s
- **Concurrent Query Throughput**: > 100 QPS
- **Sustained Load**: > 500 ops/sec
- **Memory Usage**: < 100 MB for 100 workspaces
- **No Memory Leaks**: Stable memory across 10 cycles

### Measuring Performance

Run performance tests and check the output:

```bash
go test -v ./tests/integration -run TestConnectionPoolPerformance 2>&1 | grep -E "(operations|duration|QPS|ops/sec|Memory)"
```

## Maintenance

### Regular Checks

1. Run full test suite weekly with race detector
2. Monitor test execution time (should be < 5 minutes for all tests)
3. Check for flaky tests (intermittent failures)
4. Review connection pool statistics in production

### When to Update Tests

- Adding new connection pooling features
- Changing connection pool configuration
- Modifying database schema
- Upgrading PostgreSQL version
- Changing connection limits

## Resources

- [PostgreSQL Connection Management](https://www.postgresql.org/docs/current/runtime-config-connection.html)
- [Go database/sql Package](https://pkg.go.dev/database/sql)
- [Go Race Detector](https://go.dev/blog/race-detector)
- [Notifuse Tech Stack](../CLAUDE.md)

## Support

For issues or questions about connection pool testing:
1. Check this documentation
2. Review existing test examples
3. Check PostgreSQL logs for connection errors
4. Run tests with `-v` flag for verbose output
5. Use race detector to catch concurrency issues

---

**Last Updated**: 2025-10-30  
**Version**: 1.0  
**Status**: Production Ready
