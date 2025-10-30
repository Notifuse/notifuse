# Connection Pool Integration Tests Improvement Plan

**Version:** 1.0  
**Date:** 2025-10-30  
**Status:** Planning  
**Priority:** High (Production stability)

---

## Executive Summary

The production connection manager (`pkg/database/`) has excellent unit test coverage with 18 comprehensive tests. However, the test infrastructure (`tests/testutil/connection_pool.go`) and integration tests have significant gaps that led to test hangs, connection leaks, and unreliable test execution. This plan addresses these issues with a phased approach to improve integration test coverage and reliability.

---

## Current State Analysis

### ✅ **What's Working Well**

**Production Connection Manager** (`pkg/database/`):
- 720 lines of comprehensive unit tests
- Tests cover capacity, LRU eviction, race conditions, stale pool removal
- All unit tests pass consistently
- Good isolation with mocks

### ⚠️ **Critical Issues**

**Test Infrastructure** (`tests/testutil/`):
1. **Connection Leaks**: Global connection pool not cleaned between tests
2. **Test Hangs**: `TestAPIServerShutdown` times out after 2 minutes
3. **Resource Management**: No proper lifecycle for test connections
4. **Port Configuration**: Hardcoded ports cause conflicts
5. **Minimal Coverage**: Only 4 basic integration tests

**Integration Tests** (`tests/integration/`):
1. **No Connection Pool Tests**: Connection pooling not explicitly tested in integration scenarios
2. **No Stress Tests**: No tests for high connection counts or concurrent operations
3. **No Failure Recovery**: Missing tests for database disconnection/reconnection
4. **No Performance Tests**: No validation of connection pool efficiency

---

## Problem Breakdown

### Issue 1: Global Pool Cleanup
**Current Problem:**
```go
// Global singleton pool is never fully reset between tests
var globalTestPool *TestConnectionPool
var poolOnce sync.Once

// Tests create pool but cleanup doesn't reset sync.Once properly
```

**Impact:**
- Second test hangs trying to connect to already-closed connections
- Database connections leak between test runs
- Unpredictable test failures

### Issue 2: Test Isolation
**Current Problem:**
- Tests share global state (connection pool)
- No clean slate between test functions
- Cleanup happens in `defer` which doesn't guarantee order

**Impact:**
- Cannot run multiple integration tests in sequence
- `TestAPIServerShutdown` always hangs as second test
- Intermittent failures depending on test order

### Issue 3: Missing Test Scenarios
**Current Gap:**
- No tests for connection pool under load
- No tests for workspace database lifecycle
- No tests for connection limit enforcement
- No tests for error recovery

**Impact:**
- Production issues may not be caught before deployment
- No confidence in connection pool behavior at scale
- Unknown performance characteristics

---

## Improvement Plan

### Phase 1: Fix Critical Test Infrastructure (Week 1)

#### Task 1.1: Isolate Connection Pool Per Test
**Goal:** Each test gets its own connection pool instance

**Changes:**
```go
// Remove global singleton pattern for tests
// Instead, create per-test pools with proper cleanup

type TestConnectionPoolManager struct {
    pools map[string]*TestConnectionPool
    mutex sync.RWMutex
}

func NewTestConnectionPoolManager() *TestConnectionPoolManager {
    return &TestConnectionPoolManager{
        pools: make(map[string]*TestConnectionPool),
    }
}

func (m *TestConnectionPoolManager) GetOrCreatePool(testID string, config *config.DatabaseConfig) *TestConnectionPool {
    // Create isolated pool per test
    // Return existing pool if already created for this test
}

func (m *TestConnectionPoolManager) CleanupPool(testID string) error {
    // Properly close all connections
    // Remove from registry
    // Wait for connections to close
}

func (m *TestConnectionPoolManager) CleanupAll() error {
    // Close all test pools
    // Reset all state
}
```

**Files to Modify:**
- `tests/testutil/connection_pool.go`
- `tests/testutil/helpers.go`

**Success Criteria:**
- ✅ Each test function gets isolated connection pool
- ✅ Cleanup removes all connections without leaks
- ✅ Can run all integration tests in sequence without hangs
- ✅ Test execution time < 30s for all tests

#### Task 1.2: Proper Connection Lifecycle Management
**Goal:** Ensure connections are properly closed and resources released

**Implementation:**
```go
func (pool *TestConnectionPool) Cleanup() error {
    pool.poolMutex.Lock()
    defer pool.poolMutex.Unlock()
    
    // 1. Close all workspace connections first
    for workspaceID, db := range pool.workspacePools {
        if err := db.Close(); err != nil {
            log.Printf("Warning: error closing workspace pool %s: %v", workspaceID, err)
        }
        delete(pool.workspacePools, workspaceID)
    }
    
    // 2. Wait for connections to actually close
    time.Sleep(100 * time.Millisecond)
    
    // 3. Drop workspace databases using system connection
    if pool.systemPool != nil {
        for workspaceID := range pool.workspacePools {
            pool.dropWorkspaceDatabase(workspaceID)
        }
    }
    
    // 4. Close system connection last
    if pool.systemPool != nil {
        if err := pool.systemPool.Close(); err != nil {
            log.Printf("Warning: error closing system pool: %v", err)
        }
        pool.systemPool = nil
    }
    
    pool.connectionCount = 0
    
    // 5. Verify no leaked connections
    return pool.verifyNoLeakedConnections()
}

func (pool *TestConnectionPool) verifyNoLeakedConnections() error {
    // Query PostgreSQL for active connections from our test user
    // Fail if any remain
}
```

**Success Criteria:**
- ✅ No database connections remain after cleanup
- ✅ PostgreSQL shows 0 active connections from test user
- ✅ No goroutine leaks
- ✅ Cleanup completes in < 1 second

#### Task 1.3: Fix `TestAPIServerShutdown`
**Goal:** Enable test to run without hanging

**Changes:**
- Use isolated connection pool for test
- Add explicit connection verification before assertions
- Add timeout context for all operations
- Log connection state at each step

**Success Criteria:**
- ✅ Test completes in < 5 seconds
- ✅ Can run after any other test without hanging
- ✅ Remove skip statement

---

### Phase 2: Add Comprehensive Integration Tests (Week 2)

#### Task 2.1: Connection Pool Lifecycle Tests
**Goal:** Test complete lifecycle of connection pools

**New Test File:** `tests/integration/connection_pool_lifecycle_test.go`

**Test Cases:**
```go
func TestConnectionPoolLifecycle(t *testing.T) {
    t.Run("pool initialization", func(t *testing.T) {
        // Test pool starts correctly
        // System connection established
        // Stats show correct initial state
    })
    
    t.Run("workspace pool creation", func(t *testing.T) {
        // Create workspace database
        // Get connection from pool
        // Verify connection works
        // Stats show increased count
    })
    
    t.Run("workspace pool reuse", func(t *testing.T) {
        // Request same workspace twice
        // Verify same connection returned
        // No duplicate pools created
    })
    
    t.Run("workspace pool cleanup", func(t *testing.T) {
        // Close workspace pool
        // Verify database dropped
        // Stats show decreased count
        // Connection no longer in pool
    })
    
    t.Run("full cleanup", func(t *testing.T) {
        // Create multiple workspace pools
        // Clean up all
        // Verify no connections remain
        // Pool is empty
    })
}
```

#### Task 2.2: Concurrent Access Tests
**Goal:** Verify thread-safety and concurrent performance

**New Test File:** `tests/integration/connection_pool_concurrency_test.go`

**Test Cases:**
```go
func TestConnectionPoolConcurrency(t *testing.T) {
    t.Run("concurrent workspace creation", func(t *testing.T) {
        // 50 goroutines request different workspaces simultaneously
        // All succeed
        // No duplicate pools
        // No panics
    })
    
    t.Run("concurrent same workspace access", func(t *testing.T) {
        // 100 goroutines request same workspace
        // All get same connection
        // No race conditions
        // Connection count = 1
    })
    
    t.Run("concurrent read/write operations", func(t *testing.T) {
        // Multiple goroutines read/write to different workspaces
        // All operations succeed
        // Data integrity maintained
        // No deadlocks
    })
    
    t.Run("concurrent cleanup", func(t *testing.T) {
        // Multiple goroutines close different workspaces
        // All cleanups succeed
        // No panics
        // Final state is clean
    })
}
```

#### Task 2.3: Connection Limit Tests
**Goal:** Verify connection limits are enforced

**New Test File:** `tests/integration/connection_pool_limits_test.go`

**Test Cases:**
```go
func TestConnectionPoolLimits(t *testing.T) {
    t.Run("max connections enforced", func(t *testing.T) {
        // Set max connections to 10
        // Create 10 workspace pools
        // 11th request should trigger LRU eviction or error
        // Verify limit never exceeded
    })
    
    t.Run("LRU eviction works correctly", func(t *testing.T) {
        // Fill pool to capacity
        // Request new workspace
        // Verify oldest idle pool is closed
        // New pool is created
    })
    
    t.Run("per-workspace connection limits", func(t *testing.T) {
        // Set MaxOpenConns to 5 per workspace
        // Create 10 concurrent queries to same workspace
        // Verify only 5 connections open at once
        // All queries complete successfully
    })
    
    t.Run("connection timeout handling", func(t *testing.T) {
        // Set connection timeout to 1s
        // Simulate slow database
        // Verify timeout error returned
        // Pool remains stable
    })
}
```

#### Task 2.4: Error Recovery Tests
**Goal:** Test resilience to failures

**New Test File:** `tests/integration/connection_pool_failure_test.go`

**Test Cases:**
```go
func TestConnectionPoolFailureRecovery(t *testing.T) {
    t.Run("database connection lost", func(t *testing.T) {
        // Get workspace connection
        // Stop database container
        // Verify error on next operation
        // Start database
        // Verify reconnection works
    })
    
    t.Run("stale connection detection", func(t *testing.T) {
        // Create connection
        // Let it idle past MaxLifetime
        // Verify stale connection removed
        // New connection created automatically
    })
    
    t.Run("workspace database deleted externally", func(t *testing.T) {
        // Create workspace pool
        // Delete database externally
        // Verify error on next operation
        // Pool cleans up gracefully
    })
    
    t.Run("system database connection lost", func(t *testing.T) {
        // Lose system DB connection
        // Verify workspace operations fail gracefully
        // Reconnect
        // Verify operations resume
    })
}
```

#### Task 2.5: Performance & Load Tests
**Goal:** Validate performance characteristics

**New Test File:** `tests/integration/connection_pool_performance_test.go`

**Test Cases:**
```go
func TestConnectionPoolPerformance(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping performance tests in short mode")
    }
    
    t.Run("connection reuse performance", func(t *testing.T) {
        // Measure time for 1000 operations with connection reuse
        // Compare to 1000 operations creating new connections
        // Verify reuse is significantly faster (>10x)
    })
    
    t.Run("high workspace count", func(t *testing.T) {
        // Create 100 workspace pools
        // Perform operation on each
        // Verify all succeed
        // Total time < 30s
        // Memory usage reasonable
    })
    
    t.Run("rapid create/destroy", func(t *testing.T) {
        // Rapidly create and destroy 50 workspaces
        // Repeat 10 times
        // Verify no memory leaks
        // Verify no connection leaks
        // Performance stable across iterations
    })
    
    t.Run("idle connection cleanup", func(t *testing.T) {
        // Create 20 workspace pools
        // Let them idle for ConnMaxIdleTime + 1s
        // Verify idle connections cleaned up
        // Memory reclaimed
    })
}
```

---

### Phase 3: Documentation & Monitoring (Week 3)

#### Task 3.1: Test Documentation
**Goal:** Document test infrastructure for maintainers

**New File:** `tests/README_CONNECTION_POOLS.md`

**Contents:**
- Connection pool architecture overview
- How to run connection pool tests
- How to write new connection pool tests
- Common pitfalls and solutions
- Debugging connection leaks
- Performance benchmarks and targets

#### Task 3.2: Test Utilities Enhancement
**Goal:** Add helper functions for common test scenarios

**New File:** `tests/testutil/connection_pool_helpers.go`

**Functions:**
```go
// VerifyNoLeakedConnections queries PostgreSQL for leaked connections
func VerifyNoLeakedConnections(t *testing.T, db *sql.DB, testUser string)

// WaitForConnectionClose waits for a connection to be closed
func WaitForConnectionClose(t *testing.T, db *sql.DB, timeout time.Duration)

// GetActiveConnectionCount returns current connection count for a user
func GetActiveConnectionCount(t *testing.T, systemDB *sql.DB, user string) int

// CreateTestWorkspaces creates N test workspaces and returns their IDs
func CreateTestWorkspaces(t *testing.T, pool *TestConnectionPool, count int) []string

// CleanupTestWorkspaces removes test workspaces
func CleanupTestWorkspaces(t *testing.T, pool *TestConnectionPool, workspaceIDs []string)

// SimulateDatabaseFailure temporarily stops database access
func SimulateDatabaseFailure(t *testing.T, duration time.Duration)

// MeasureOperationTime measures and logs operation duration
func MeasureOperationTime(t *testing.T, operation string, fn func()) time.Duration
```

#### Task 3.3: Monitoring & Observability
**Goal:** Add metrics for connection pool health in tests

**Implementation:**
- Log connection pool stats at test start/end
- Track connection count changes during tests
- Alert on leaked connections
- Report performance metrics

```go
type ConnectionPoolMetrics struct {
    TestName           string
    InitialConnections int
    PeakConnections    int
    FinalConnections   int
    LeakedConnections  int
    PoolCreations      int
    PoolDestructions   int
    Duration           time.Duration
}

func (m *ConnectionPoolMetrics) Report(t *testing.T) {
    t.Logf("Connection Pool Metrics for %s:", m.TestName)
    t.Logf("  Initial: %d, Peak: %d, Final: %d", 
        m.InitialConnections, m.PeakConnections, m.FinalConnections)
    t.Logf("  Leaked: %d, Created: %d, Destroyed: %d",
        m.LeakedConnections, m.PoolCreations, m.PoolDestructions)
    t.Logf("  Duration: %v", m.Duration)
    
    if m.LeakedConnections > 0 {
        t.Errorf("LEAK DETECTED: %d connections leaked", m.LeakedConnections)
    }
}
```

---

## Test Execution Strategy

### Local Development
```bash
# Run all connection pool tests
make test-connection-pools

# Run specific test suite
go test -v ./tests/integration -run TestConnectionPool

# Run with race detector
go test -race -v ./tests/integration -run TestConnectionPool

# Run performance tests
go test -v ./tests/integration -run TestConnectionPoolPerformance

# Check for connection leaks
make test-connection-pools-leak-check
```

### CI/CD Integration
```yaml
# .github/workflows/connection-pool-tests.yml
name: Connection Pool Tests

on: [push, pull_request]

jobs:
  connection-pool-tests:
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
      
      - name: Run Connection Pool Unit Tests
        run: go test -v -race ./pkg/database
      
      - name: Run Connection Pool Integration Tests
        env:
          INTEGRATION_TESTS: true
          TEST_DB_HOST: localhost
          TEST_DB_PORT: 5432
          TEST_DB_USER: notifuse_test
          TEST_DB_PASSWORD: test_password
        run: |
          go test -v -race ./tests/integration -run TestConnectionPool
          go test -v ./tests/testutil -run TestConnectionPool
      
      - name: Check for Connection Leaks
        if: always()
        run: |
          # Query PostgreSQL for remaining connections
          PGPASSWORD=test_password psql -h localhost -U notifuse_test -d postgres -c \
            "SELECT count(*) FROM pg_stat_activity WHERE usename = 'notifuse_test';"
```

---

## Success Metrics

### Reliability Metrics
- ✅ **0% flaky tests**: No intermittent failures
- ✅ **100% pass rate**: All tests pass consistently
- ✅ **0 connection leaks**: No leaked connections after any test
- ✅ **< 60s total execution**: All integration tests complete quickly

### Coverage Metrics
- ✅ **25+ integration test cases**: Comprehensive scenarios covered
- ✅ **Concurrent operations tested**: Thread-safety verified
- ✅ **Error scenarios tested**: Failure recovery validated
- ✅ **Performance validated**: Benchmarks established

### Quality Metrics
- ✅ **Code coverage > 80%**: Connection pool code well-tested
- ✅ **Race detector clean**: No race conditions detected
- ✅ **Memory leak free**: No memory growth over time
- ✅ **Documentation complete**: Clear guide for maintainers

---

## Risks & Mitigation

### Risk 1: Test Duration Too Long
**Mitigation:**
- Use `testing.Short()` to skip performance tests in CI
- Run full suite nightly only
- Optimize test database operations
- Use test categories: fast/slow/stress

### Risk 2: Flaky Tests Due to Timing
**Mitigation:**
- Use explicit waits instead of sleeps
- Poll for conditions with timeout
- Increase timeouts in CI environment
- Add retry logic for transient failures

### Risk 3: Breaking Production Code
**Mitigation:**
- Make changes incrementally
- Run full test suite after each change
- Review with team before merging
- Feature flag new test infrastructure

### Risk 4: Test Maintenance Burden
**Mitigation:**
- Create reusable test helpers
- Document test patterns
- Keep tests DRY (Don't Repeat Yourself)
- Regular refactoring

---

## Timeline & Milestones

### Week 1: Critical Fixes (Nov 4-8, 2025)
- ✅ Fix global pool cleanup
- ✅ Isolate test connection pools
- ✅ Fix `TestAPIServerShutdown`
- ✅ Verify no connection leaks

**Deliverable:** All existing tests pass reliably

### Week 2: New Test Coverage (Nov 11-15, 2025)
- ✅ Lifecycle tests
- ✅ Concurrency tests  
- ✅ Limit enforcement tests
- ✅ Error recovery tests
- ✅ Performance tests

**Deliverable:** 25+ new integration test cases

### Week 3: Documentation & Polish (Nov 18-22, 2025)
- ✅ Test documentation
- ✅ Helper utilities
- ✅ CI/CD integration
- ✅ Monitoring & metrics

**Deliverable:** Complete test infrastructure

---

## Implementation Checklist

### Phase 1: Critical Fixes
- [ ] Create `TestConnectionPoolManager` for test isolation
- [ ] Implement proper `Cleanup()` with verification
- [ ] Add `verifyNoLeakedConnections()` helper
- [ ] Fix global pool initialization pattern
- [ ] Update `CleanupTestEnvironment()` to use new manager
- [ ] Remove skip from `TestAPIServerShutdown`
- [ ] Verify all existing integration tests pass
- [ ] Add cleanup verification to all tests

### Phase 2: New Tests
- [ ] Create `connection_pool_lifecycle_test.go`
- [ ] Create `connection_pool_concurrency_test.go`
- [ ] Create `connection_pool_limits_test.go`
- [ ] Create `connection_pool_failure_test.go`
- [ ] Create `connection_pool_performance_test.go`
- [ ] Run with race detector
- [ ] Verify no memory leaks with pprof
- [ ] Establish performance baselines

### Phase 3: Documentation
- [ ] Create `tests/README_CONNECTION_POOLS.md`
- [ ] Create `connection_pool_helpers.go`
- [ ] Add connection pool metrics collection
- [ ] Create CI/CD workflow
- [ ] Add monitoring dashboard
- [ ] Write troubleshooting guide
- [ ] Create team presentation

---

## Appendix A: Example Test Implementation

### Example: Proper Test with Isolated Pool

```go
func TestWorkspaceCreationWithIsolatedPool(t *testing.T) {
    // Setup: Create isolated test environment
    testID := fmt.Sprintf("test_%d", time.Now().UnixNano())
    poolManager := testutil.NewTestConnectionPoolManager()
    defer poolManager.CleanupAll()
    
    config := &config.DatabaseConfig{
        Host:     getEnvOrDefault("TEST_DB_HOST", "localhost"),
        Port:     5432,
        User:     "notifuse_test",
        Password: "test_password",
        Prefix:   "notifuse_test",
        SSLMode:  "disable",
    }
    
    pool := poolManager.GetOrCreatePool(testID, config)
    
    // Track initial state
    metrics := &ConnectionPoolMetrics{
        TestName:           t.Name(),
        InitialConnections: pool.GetConnectionCount(),
    }
    defer metrics.Report(t)
    
    // Test: Create workspace
    workspaceID := "test_workspace_" + testID
    err := pool.EnsureWorkspaceDatabase(workspaceID)
    require.NoError(t, err)
    
    db, err := pool.GetWorkspaceConnection(workspaceID)
    require.NoError(t, err)
    
    // Verify connection works
    err = db.Ping()
    require.NoError(t, err)
    
    metrics.PeakConnections = pool.GetConnectionCount()
    
    // Cleanup
    err = pool.CleanupWorkspace(workspaceID)
    require.NoError(t, err)
    
    metrics.FinalConnections = pool.GetConnectionCount()
    
    // Verify no leaks
    testutil.VerifyNoLeakedConnections(t, pool.GetSystemConnection(), "notifuse_test")
}
```

---

## Appendix B: Connection Leak Detection

### PostgreSQL Query to Find Leaks
```sql
-- Find active connections from test user
SELECT 
    pid,
    usename,
    application_name,
    client_addr,
    backend_start,
    state,
    query,
    wait_event_type,
    wait_event
FROM pg_stat_activity 
WHERE usename = 'notifuse_test'
  AND state != 'idle'
  AND pid != pg_backend_pid();
```

### Helper Function
```go
func VerifyNoLeakedConnections(t *testing.T, systemDB *sql.DB, user string) {
    t.Helper()
    
    var count int
    query := `
        SELECT COUNT(*) 
        FROM pg_stat_activity 
        WHERE usename = $1 
          AND state != 'idle'
          AND pid != pg_backend_pid()
    `
    
    err := systemDB.QueryRow(query, user).Scan(&count)
    require.NoError(t, err, "Failed to query connection count")
    
    if count > 0 {
        // Get details about leaked connections
        rows, err := systemDB.Query(`
            SELECT pid, application_name, state, query 
            FROM pg_stat_activity 
            WHERE usename = $1 
              AND state != 'idle'
              AND pid != pg_backend_pid()
        `, user)
        require.NoError(t, err)
        defer rows.Close()
        
        t.Errorf("LEAK DETECTED: %d connections remain active", count)
        for rows.Next() {
            var pid int
            var app, state, query string
            rows.Scan(&pid, &app, &state, &query)
            t.Logf("  - PID %d: %s (%s) - %s", pid, app, state, query)
        }
        
        t.FailNow()
    }
}
```

---

## Questions & Answers

**Q: Why not just increase test timeouts?**  
A: Timeouts hide the real problem (connection leaks). We need to fix the root cause, not mask it.

**Q: Can we skip these tests in CI to save time?**  
A: No. Connection pool issues can cause production outages. These tests are critical.

**Q: Why not use a library for connection pooling?**  
A: We have custom requirements (workspace isolation, LRU eviction). But we should test like a library would be tested.

**Q: How do we prevent these issues in the future?**  
A: Mandatory connection pool tests for any database code. Pre-commit hooks. Code review checklist.

---

## References

- [PostgreSQL Connection Pooling Best Practices](https://www.postgresql.org/docs/current/runtime-config-connection.html)
- [Go database/sql Package Documentation](https://pkg.go.dev/database/sql)
- [Testing Concurrent Code in Go](https://go.dev/blog/race-detector)
- [Connection Pool Pattern](https://en.wikipedia.org/wiki/Connection_pool)

---

**Plan Owner:** Engineering Team  
**Reviewers:** Backend Team, QA Team  
**Approval Required:** Tech Lead, Engineering Manager
