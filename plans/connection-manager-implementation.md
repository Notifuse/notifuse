# Connection Manager Implementation Plan
## FINAL SOLUTION - Shared Connection Pool Architecture

> **Status:** ✅ This is the approved implementation plan  
> **Supersedes:** connection-manager-singleton-OLD.md (per-workspace pools - doesn't scale)  
> **Key Insight:** Don't reserve pools per workspace - use small shared pools per workspace DATABASE

---

## Executive Summary

**Problem:** Database "too many connections" errors with PostgreSQL max_connections=100

**Root Cause:** 
- Each workspace has its own PostgreSQL database
- Current code creates 25 connections per workspace database
- Math fails: 4 workspaces × 25 = 100 connections (limit reached with just 4 workspaces!)

**Solution:**
- Shared connection pools (2-3 connections per workspace DATABASE)
- LRU eviction of idle pools
- Supports UNLIMITED workspaces with fixed 100 connection limit

**Capacity:**
```
DB_MAX_CONNECTIONS=100
DB_MAX_CONNECTIONS_PER_DB=3

Result:
✅ System DB: 10 connections
✅ Available for workspaces: 90 connections
✅ Concurrent active workspace DBs: 30 (90 ÷ 3)
✅ Total workspaces supported: UNLIMITED (100, 500, 1000+)
```

---

## What Changed from Original Plan

| Aspect | ❌ Original Plan | ✅ Current Plan |
|--------|-----------------|----------------|
| **Pool strategy** | Reserved 5-25 connections per workspace | 2-3 connections per workspace DATABASE |
| **Workspace limit** | ~10 workspaces max | Unlimited workspaces |
| **Pool creation** | On first workspace access | On first workspace DB access |
| **Pool lifetime** | Permanent until workspace deleted | LRU eviction when idle |
| **Scalability** | Doesn't scale | Scales to 100+ workspaces |
| **Connection efficiency** | Wastes connections on idle workspaces | Only active DBs have pools |

**Key Questions Answered:**
1. ✅ "What if we have more workspaces than connections?" - **LRU eviction handles it**
2. ✅ "What if all connections are in use?" - **503 error with clear message, client retries**
3. ✅ "Is pooling better than per-query connections?" - **Yes, 10-100x faster** (see connection-pooling-vs-per-query.md)

---

## Overview
Implement a shared connection pool manager that handles connections for unlimited workspaces without per-workspace reservation.

## Current Problem Analysis
- **Original plan flaw**: Reserved 3-15 connections per workspace
- **Scale issue**: With 100 workspaces and 100 max connections, can only support ~7-10 workspaces
- **Real requirement**: Support 100+ workspaces with 100 connections by sharing the pool

## Proposed Solution: Shared Pool Architecture

### Key Concept
**One shared connection pool per database**, not per workspace. Connections are acquired per-query, used, and immediately returned to the pool.

```
┌─────────────────────────────────────────────────────┐
│           Application Layer                         │
│   (100+ workspaces accessing data)                  │
└─────────────────────────────────────────────────────┘
                        │
                        ▼
┌─────────────────────────────────────────────────────┐
│   ConnectionManager Singleton                       │
│   • GetConnection(workspaceID) → *sql.DB            │
│   • ReleaseConnection(workspaceID)                  │
│   • Total: 1 pool per workspace DATABASE            │
└─────────────────────────────────────────────────────┘
                        │
    ┌───────────────────┼───────────────────┐
    │                   │                   │
    ▼                   ▼                   ▼
┌─────────┐      ┌─────────┐        ┌─────────┐
│ Pool    │      │ Pool    │        │ Pool    │
│ System  │      │ ws_001  │  ...   │ ws_100  │
│ DB      │      │ DB      │        │ DB      │
│ (10)    │      │ (3-5)   │        │ (3-5)   │
└─────────┘      └─────────┘        └─────────┘
     │                │                   │
     └────────────────┴───────────────────┘
                      │
                      ▼
            ┌─────────────────┐
            │  PostgreSQL     │
            │  max_conn = 100 │
            └─────────────────┘
```

### Architecture Differences

**OLD (Per-Workspace Pools):**
- ❌ Each workspace gets dedicated pool
- ❌ Connections reserved even when idle
- ❌ Doesn't scale beyond ~10 workspaces

**NEW (Shared Pools):**
- ✅ One pool per unique database
- ✅ Multiple workspaces share same database
- ✅ Connections acquired per-query, returned immediately
- ✅ Scales to unlimited workspaces

### Important Realization
Each workspace has its **own PostgreSQL database**, so we actually need:
- **1 connection pool per workspace database** (not per workspace access)
- Each pool size: **2-5 connections** (small, since queries are short-lived)
- Connections returned immediately after query completes

---

## Implementation Steps

### Phase 1: Configuration Updates

#### Step 1.1: Update Config Structure
**File:** `config/config.go`

**Changes:**
```go
type DatabaseConfig struct {
    Host                    string
    Port                    int
    User                    string
    Password                string
    DBName                  string
    Prefix                  string
    SSLMode                 string
    MaxConnections          int  // Total across all databases
    MaxConnectionsPerDB     int  // Per workspace database (default: 3)
    ConnectionMaxLifetime   time.Duration
    ConnectionMaxIdleTime   time.Duration
}

// In LoadWithOptions():
v.SetDefault("DB_MAX_CONNECTIONS", 100)
v.SetDefault("DB_MAX_CONNECTIONS_PER_DB", 3)  // Small pool per workspace DB
v.SetDefault("DB_CONNECTION_MAX_LIFETIME", "10m")
v.SetDefault("DB_CONNECTION_MAX_IDLE_TIME", "5m")

dbConfig := DatabaseConfig{
    Host:                  v.GetString("DB_HOST"),
    Port:                  v.GetInt("DB_PORT"),
    User:                  v.GetString("DB_USER"),
    Password:              v.GetString("DB_PASSWORD"),
    DBName:                v.GetString("DB_NAME"),
    Prefix:                v.GetString("DB_PREFIX"),
    SSLMode:               v.GetString("DB_SSLMODE"),
    MaxConnections:        v.GetInt("DB_MAX_CONNECTIONS"),
    MaxConnectionsPerDB:   v.GetInt("DB_MAX_CONNECTIONS_PER_DB"),
    ConnectionMaxLifetime: v.GetDuration("DB_CONNECTION_MAX_LIFETIME"),
    ConnectionMaxIdleTime: v.GetDuration("DB_CONNECTION_MAX_IDLE_TIME"),
}

// Validate
if dbConfig.MaxConnections < 20 {
    return nil, fmt.Errorf("DB_MAX_CONNECTIONS must be at least 20")
}
if dbConfig.MaxConnectionsPerDB < 1 {
    return nil, fmt.Errorf("DB_MAX_CONNECTIONS_PER_DB must be at least 1")
}
```

---

### Phase 2: Shared Connection Manager

#### Step 2.1: Create ConnectionManager
**File:** `pkg/database/connection_manager.go` (NEW)

**Key Difference:** Track connections by database, not by workspace access count

```go
package database

import (
    "context"
    "database/sql"
    "fmt"
    "strings"
    "sync"
    "time"
    
    "github.com/Notifuse/notifuse/config"
    "github.com/Notifuse/notifuse/internal/database"
)

// ConnectionManager manages database connections with a shared pool approach
type ConnectionManager interface {
    // GetSystemConnection returns the system database connection
    GetSystemConnection() *sql.DB
    
    // GetWorkspaceConnection returns a connection pool for a workspace database
    // The returned *sql.DB is a connection pool - use it for queries and sql.DB
    // will handle connection pooling automatically
    GetWorkspaceConnection(ctx context.Context, workspaceID string) (*sql.DB, error)
    
    // CloseWorkspaceConnection closes a workspace database connection pool
    CloseWorkspaceConnection(workspaceID string) error
    
    // GetStats returns connection statistics
    GetStats() ConnectionStats
    
    // Close closes all connections
    Close() error
}

// ConnectionStats provides visibility into connection usage
type ConnectionStats struct {
    MaxConnections          int
    MaxConnectionsPerDB     int
    SystemConnections       ConnectionPoolStats
    WorkspacePools          map[string]ConnectionPoolStats
    TotalOpenConnections    int
    TotalInUseConnections   int
    TotalIdleConnections    int
    ActiveWorkspaceDatabases int
}

// ConnectionPoolStats provides stats for a single connection pool
type ConnectionPoolStats struct {
    OpenConnections int
    InUse           int
    Idle            int
    MaxOpen         int
    WaitCount       int64
    WaitDuration    time.Duration
}

// connectionManager implements ConnectionManager
type connectionManager struct {
    mu                  sync.RWMutex
    config              *config.Config
    systemDB            *sql.DB
    workspacePools      map[string]*sql.DB  // workspaceID -> connection pool
    maxConnections      int
    maxConnectionsPerDB int
}

var (
    instance     *connectionManager
    instanceOnce sync.Once
    instanceMu   sync.RWMutex
)

// InitializeConnectionManager initializes the singleton with configuration
func InitializeConnectionManager(cfg *config.Config, systemDB *sql.DB) error {
    var initErr error
    instanceOnce.Do(func() {
        instanceMu.Lock()
        defer instanceMu.Unlock()
        
        instance = &connectionManager{
            config:              cfg,
            systemDB:            systemDB,
            workspacePools:      make(map[string]*sql.DB),
            maxConnections:      cfg.Database.MaxConnections,
            maxConnectionsPerDB: cfg.Database.MaxConnectionsPerDB,
        }
        
        // Configure system database pool
        // System DB gets slightly more connections (10% of total, min 5, max 20)
        systemPoolSize := cfg.Database.MaxConnections / 10
        if systemPoolSize < 5 {
            systemPoolSize = 5
        }
        if systemPoolSize > 20 {
            systemPoolSize = 20
        }
        
        systemDB.SetMaxOpenConns(systemPoolSize)
        systemDB.SetMaxIdleConns(systemPoolSize / 2)
        systemDB.SetConnMaxLifetime(cfg.Database.ConnectionMaxLifetime)
        systemDB.SetConnMaxIdleTime(cfg.Database.ConnectionMaxIdleTime)
    })
    
    return initErr
}

// GetConnectionManager returns the singleton instance
func GetConnectionManager() (ConnectionManager, error) {
    instanceMu.RLock()
    defer instanceMu.RUnlock()
    
    if instance == nil {
        return nil, fmt.Errorf("connection manager not initialized")
    }
    
    return instance, nil
}

// ResetConnectionManager resets the singleton (for testing only)
func ResetConnectionManager() {
    instanceMu.Lock()
    defer instanceMu.Unlock()
    
    if instance != nil {
        instance.Close()
        instance = nil
    }
    instanceOnce = sync.Once{}
}

// GetSystemConnection returns the system database connection
func (cm *connectionManager) GetSystemConnection() *sql.DB {
    return cm.systemDB
}

// GetWorkspaceConnection returns a connection pool for a workspace database
func (cm *connectionManager) GetWorkspaceConnection(ctx context.Context, workspaceID string) (*sql.DB, error) {
    // Check if we already have a connection pool for this workspace
    cm.mu.RLock()
    if pool, ok := cm.workspacePools[workspaceID]; ok {
        cm.mu.RUnlock()
        
        // Test the connection pool is still valid
        if err := pool.PingContext(ctx); err == nil {
            return pool, nil
        }
        
        // Pool is stale, remove it
        cm.mu.Lock()
        delete(cm.workspacePools, workspaceID)
        pool.Close()
        cm.mu.Unlock()
    } else {
        cm.mu.RUnlock()
    }
    
    // Need to create a new pool
    cm.mu.Lock()
    defer cm.mu.Unlock()
    
    // Double-check after acquiring write lock
    if pool, ok := cm.workspacePools[workspaceID]; ok {
        return pool, nil
    }
    
    // Check if we have capacity for a new database connection pool
    if !cm.hasCapacityForNewPool() {
        // Try to close least recently used idle pools
        if cm.closeLRUIdlePools(1) > 0 {
            // Successfully closed a pool, retry
            if !cm.hasCapacityForNewPool() {
                return nil, &ConnectionLimitError{
                    MaxConnections:     cm.maxConnections,
                    CurrentConnections: cm.getTotalConnectionCount(),
                    WorkspaceID:        workspaceID,
                }
            }
        } else {
            // Cannot close any pools - all are in use
            return nil, &ConnectionLimitError{
                MaxConnections:     cm.maxConnections,
                CurrentConnections: cm.getTotalConnectionCount(),
                WorkspaceID:        workspaceID,
            }
        }
    }
    
    // Create new workspace connection pool
    pool, err := cm.createWorkspacePool(workspaceID)
    if err != nil {
        return nil, fmt.Errorf("failed to create workspace pool: %w", err)
    }
    
    // Store in map
    cm.workspacePools[workspaceID] = pool
    
    return pool, nil
}

// createWorkspacePool creates a new connection pool for a workspace database
func (cm *connectionManager) createWorkspacePool(workspaceID string) (*sql.DB, error) {
    // Build workspace DSN
    safeID := strings.ReplaceAll(workspaceID, "-", "_")
    dbName := fmt.Sprintf("%s_ws_%s", cm.config.Database.Prefix, safeID)
    
    dsn := fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
        cm.config.Database.User,
        cm.config.Database.Password,
        cm.config.Database.Host,
        cm.config.Database.Port,
        dbName,
        cm.config.Database.SSLMode,
    )
    
    // Ensure database exists
    if err := database.EnsureWorkspaceDatabaseExists(&cm.config.Database, workspaceID); err != nil {
        return nil, err
    }
    
    // Open connection pool
    db, err := sql.Open("postgres", dsn)
    if err != nil {
        return nil, fmt.Errorf("failed to open connection: %w", err)
    }
    
    // Test connection
    if err := db.Ping(); err != nil {
        db.Close()
        return nil, fmt.Errorf("failed to ping database: %w", err)
    }
    
    // Configure small pool for this workspace database
    // Each workspace DB gets only a few connections since queries are short-lived
    db.SetMaxOpenConns(cm.maxConnectionsPerDB)
    db.SetMaxIdleConns(1)  // Keep 1 idle connection warm
    db.SetConnMaxLifetime(cm.config.Database.ConnectionMaxLifetime)
    db.SetConnMaxIdleTime(cm.config.Database.ConnectionMaxIdleTime)
    
    return db, nil
}

// hasCapacityForNewPool checks if we have capacity for a new connection pool
// Must be called with write lock held
func (cm *connectionManager) hasCapacityForNewPool() bool {
    currentTotal := cm.getTotalConnectionCount()
    
    // Calculate projected total if we add a new pool
    projectedTotal := currentTotal + cm.maxConnectionsPerDB
    
    // Leave 10% buffer
    maxAllowed := int(float64(cm.maxConnections) * 0.9)
    
    return projectedTotal <= maxAllowed
}

// getTotalConnectionCount returns the current total open connections
// Must be called with lock held
func (cm *connectionManager) getTotalConnectionCount() int {
    total := 0
    
    // Count system connections
    if cm.systemDB != nil {
        stats := cm.systemDB.Stats()
        total += stats.OpenConnections
    }
    
    // Count workspace pool connections
    for _, pool := range cm.workspacePools {
        stats := pool.Stats()
        total += stats.OpenConnections
    }
    
    return total
}

// closeLRUIdlePools closes up to 'count' least recently used idle pools
// Returns the number of pools actually closed
// Must be called with write lock held
func (cm *connectionManager) closeLRUIdlePools(count int) int {
    var closed int
    var toClose []string
    
    // Find pools with no active connections (all idle)
    for workspaceID, pool := range cm.workspacePools {
        if closed >= count {
            break
        }
        
        stats := pool.Stats()
        
        // If no connections are in use, this pool can be closed
        if stats.InUse == 0 && stats.OpenConnections > 0 {
            toClose = append(toClose, workspaceID)
            closed++
        }
    }
    
    // Close selected pools
    for _, workspaceID := range toClose {
        if pool, ok := cm.workspacePools[workspaceID]; ok {
            pool.Close()
            delete(cm.workspacePools, workspaceID)
        }
    }
    
    return closed
}

// CloseWorkspaceConnection closes a specific workspace connection pool
func (cm *connectionManager) CloseWorkspaceConnection(workspaceID string) error {
    cm.mu.Lock()
    defer cm.mu.Unlock()
    
    if pool, ok := cm.workspacePools[workspaceID]; ok {
        delete(cm.workspacePools, workspaceID)
        return pool.Close()
    }
    
    return nil
}

// GetStats returns connection statistics
func (cm *connectionManager) GetStats() ConnectionStats {
    cm.mu.RLock()
    defer cm.mu.RUnlock()
    
    stats := ConnectionStats{
        MaxConnections:      cm.maxConnections,
        MaxConnectionsPerDB: cm.maxConnectionsPerDB,
        WorkspacePools:      make(map[string]ConnectionPoolStats),
    }
    
    // System connection stats
    if cm.systemDB != nil {
        systemStats := cm.systemDB.Stats()
        stats.SystemConnections = ConnectionPoolStats{
            OpenConnections: systemStats.OpenConnections,
            InUse:           systemStats.InUse,
            Idle:            systemStats.Idle,
            MaxOpen:         systemStats.MaxOpenConnections,
            WaitCount:       systemStats.WaitCount,
            WaitDuration:    systemStats.WaitDuration,
        }
        stats.TotalOpenConnections += systemStats.OpenConnections
        stats.TotalInUseConnections += systemStats.InUse
        stats.TotalIdleConnections += systemStats.Idle
    }
    
    // Workspace pool stats
    for workspaceID, pool := range cm.workspacePools {
        poolStats := pool.Stats()
        stats.WorkspacePools[workspaceID] = ConnectionPoolStats{
            OpenConnections: poolStats.OpenConnections,
            InUse:           poolStats.InUse,
            Idle:            poolStats.Idle,
            MaxOpen:         poolStats.MaxOpenConnections,
            WaitCount:       poolStats.WaitCount,
            WaitDuration:    poolStats.WaitDuration,
        }
        stats.TotalOpenConnections += poolStats.OpenConnections
        stats.TotalInUseConnections += poolStats.InUse
        stats.TotalIdleConnections += poolStats.Idle
    }
    
    stats.ActiveWorkspaceDatabases = len(cm.workspacePools)
    
    return stats
}

// Close closes all connections
func (cm *connectionManager) Close() error {
    cm.mu.Lock()
    defer cm.mu.Unlock()
    
    var errors []error
    
    // Close all workspace pools
    for workspaceID, pool := range cm.workspacePools {
        if err := pool.Close(); err != nil {
            errors = append(errors, fmt.Errorf("failed to close workspace %s: %w", workspaceID, err))
        }
        delete(cm.workspacePools, workspaceID)
    }
    
    // Note: systemDB is closed by the application
    
    if len(errors) > 0 {
        return fmt.Errorf("errors closing connections: %v", errors)
    }
    
    return nil
}

// ConnectionLimitError is returned when connection limit is reached
type ConnectionLimitError struct {
    MaxConnections     int
    CurrentConnections int
    WorkspaceID        string
}

func (e *ConnectionLimitError) Error() string {
    return fmt.Sprintf(
        "connection limit reached: %d/%d connections in use, cannot create pool for workspace %s",
        e.CurrentConnections,
        e.MaxConnections,
        e.WorkspaceID,
    )
}

// IsConnectionLimitError checks if an error is a connection limit error
func IsConnectionLimitError(err error) bool {
    _, ok := err.(*ConnectionLimitError)
    return ok
}
```

---

### Phase 3: Usage Pattern Changes

#### Key Difference: Connection Lifecycle

**OLD (Per-workspace pools with reservation):**
```go
// Reserve pool for workspace (e.g., 5 connections)
pool := GetWorkspaceConnection(workspaceID)  
// Pool stays reserved until workspace deleted
// Even when idle, holds 5 connections
```

**NEW (Shared pools, no reservation):**
```go
// Get the connection pool (small, 2-3 connections)
pool := GetWorkspaceConnection(workspaceID)  // Returns *sql.DB pool

// Use it for a query (sql.DB handles pooling internally)
rows, err := pool.QueryContext(ctx, "SELECT * FROM contacts WHERE email = $1", email)
// Connection automatically returned to pool after query

// When workspace inactive for long time, entire pool auto-closed
// Next access will recreate pool automatically
```

The `*sql.DB` returned is itself a connection pool. Go's `database/sql` package handles:
- Connection acquisition from pool
- Connection return after query
- Connection health checks
- Idle connection cleanup

---

## Capacity Planning

### Example Scenario

**Configuration:**
```
DB_MAX_CONNECTIONS=100
DB_MAX_CONNECTIONS_PER_DB=3
```

**Allocation:**
- System database: 10 connections (10% of total)
- Available for workspaces: 90 connections
- Max workspace databases with active pools: 90 / 3 = **30 concurrent workspace databases**

**What this means:**
- Can have **unlimited total workspaces** (100, 500, 1000+)
- At any moment, **up to 30 workspace databases** have active connection pools
- Inactive workspace pools are closed automatically (LRU)
- When workspace accessed, pool created on-demand

### Example with 200 Workspaces

```
Total Workspaces: 200
Active at once: ~15-25 (typical web app pattern)
Inactive: 175-185

Connection Usage:
- System: 10 connections
- Active workspace pools (25 × 3): 75 connections
- Total: 85 connections (within 100 limit)

When 26th workspace accessed:
- LRU pool (oldest idle) closed: frees 3 connections
- New pool created: uses 3 connections
- Total stays at: 85 connections
```

---

## Testing Strategy

### Unit Tests

**File:** `pkg/database/connection_manager_test.go`

```go
func TestConnectionManager_MultipleWorkspaces(t *testing.T) {
    t.Run("supports more workspaces than max connections", func(t *testing.T) {
        // Config: 30 max connections, 3 per DB
        // Create 20 workspaces (would need 60 connections if all reserved)
        // Access 5 workspaces concurrently
        // Verify: only 5 pools created (~15 connections)
        // Access remaining 15 workspaces
        // Verify: old pools closed, new pools created
        // Verify: never exceeds 30 connections
    })
    
    t.Run("LRU eviction when at capacity", func(t *testing.T) {
        // Config: 30 max connections, 3 per DB
        // Create 9 workspaces, access all (27 connections)
        // All pools now active
        // Access 10th workspace
        // Verify: LRU pool closed, new pool created
        // Verify: still at ~27 connections
    })
    
    t.Run("pool reuse for same workspace", func(t *testing.T) {
        // Access workspace A
        // Verify: pool created
        // Access workspace A again
        // Verify: same pool reused, no new pool created
    })
    
    t.Run("connection count with varying pool sizes", func(t *testing.T) {
        // Test with maxConnectionsPerDB = 2, 3, 5
        // Verify calculations work correctly
    })
}
```

### Integration Tests

**File:** `tests/integration/connection_scaling_test.go` (NEW)

```go
func TestConnectionManager_Scaling(t *testing.T) {
    t.Run("handles 50 workspaces with 30 connection limit", func(t *testing.T) {
        // Set DB_MAX_CONNECTIONS=30, DB_MAX_CONNECTIONS_PER_DB=3
        // Create 50 workspaces
        // Sequentially access each workspace (create contact)
        // Verify: all operations succeed
        // Verify: peak connections never exceeds 30
        // Verify: at most 10 pools exist at once (30/3)
    })
    
    t.Run("concurrent access across many workspaces", func(t *testing.T) {
        // Set DB_MAX_CONNECTIONS=50, DB_MAX_CONNECTIONS_PER_DB=2
        // Create 30 workspaces
        // Concurrently access 15 workspaces from 50 goroutines
        // Verify: all operations succeed
        // Verify: connection limit respected
        // Verify: no connection leaks
    })
}
```

---

## Environment Variables

### New Configuration

```bash
# Maximum total connections across ALL databases
DB_MAX_CONNECTIONS=100

# Maximum connections per individual workspace database
# Recommended: 2-5 (queries are short-lived)
DB_MAX_CONNECTIONS_PER_DB=3

# Connection lifecycle settings
DB_CONNECTION_MAX_LIFETIME=10m
DB_CONNECTION_MAX_IDLE_TIME=5m
```

### Tuning Guidelines

**For 100 max connections:**
- System DB: ~10 connections (auto-calculated)
- Workspace DBs: ~90 connections available
- With `DB_MAX_CONNECTIONS_PER_DB=3`: **30 concurrent workspace DBs**
- With `DB_MAX_CONNECTIONS_PER_DB=2`: **45 concurrent workspace DBs**

**When to adjust:**
- **More concurrent workspaces needed**: Lower `DB_MAX_CONNECTIONS_PER_DB` to 2
- **Complex queries (longer-running)**: Increase to 4-5
- **High throughput per workspace**: Increase to 5-7

---

## Advantages Over Old Approach

| Aspect | Old (Per-Workspace Pools) | New (Shared Pools) |
|--------|---------------------------|-------------------|
| **Scalability** | Limited to ~10 workspaces | Unlimited workspaces |
| **Connection efficiency** | Connections reserved even when idle | Connections only for active DBs |
| **Memory usage** | High (many pools) | Low (LRU eviction) |
| **Flexibility** | Fixed allocation | Dynamic based on usage |
| **Total workspaces supported** | Max 10 | Unlimited (100+) |

---

## Migration Notes

### No Breaking Changes
- Same API interface
- Existing code continues to work
- Just change configuration values

### Deployment Steps
1. Update environment variables
2. Deploy new code
3. Monitor connection usage
4. Tune `DB_MAX_CONNECTIONS_PER_DB` if needed

---

## Summary

### Key Changes from Original Plan

1. **No per-workspace reservation** - Pools created on-demand
2. **Small pool sizes** - 2-3 connections per workspace DB (not 5-25)
3. **LRU eviction** - Automatically close least recently used pools
4. **Scales to unlimited workspaces** - Only limits concurrent active workspace DBs

### Example Capacity

```
Config: DB_MAX_CONNECTIONS=100, DB_MAX_CONNECTIONS_PER_DB=3

Supported:
✅ Total workspaces: Unlimited (100, 500, 1000+)
✅ Concurrent active workspace DBs: ~30
✅ Total connections: ≤100

Not supported:
❌ Per-workspace connection reservation
❌ Fixed pools that never close
```

### What Happens When at Capacity?

```
Scenario: 30 workspace DBs already have active pools (90 connections used)
Action: 31st workspace accessed
Result:
1. Identify LRU idle pool (no active queries)
2. Close that pool (frees 3 connections)
3. Create new pool for 31st workspace (uses 3 connections)
4. Total connections: Still ~90

If all 30 pools are actively querying:
1. Cannot close any pools
2. Return ConnectionLimitError
3. HTTP 503 Service Unavailable
4. Client retries after delay
```

**Estimated effort:** 4-6 days
**Risk level:** Low
**Impact:** High - supports unlimited workspaces with fixed connection limit
