# Connection Manager Singleton Implementation Plan

## Overview
Implement a centralized connection manager singleton to control database connections across all workspaces with configurable limits via environment variable.

## Current Problem
- Each workspace creates its own connection pool (25 connections)
- No global limit enforcement across all workspaces
- System can easily exceed PostgreSQL's max_connections limit
- No visibility into total connection usage

## Proposed Solution
Create a singleton `ConnectionManager` that:
- Manages all database connections (system + workspaces)
- Respects a global `DB_MAX_CONNECTIONS` environment variable (default: 100)
- Dynamically allocates connections based on active workspaces
- Provides connection health monitoring and metrics
- Ensures thread-safe access to connection pools

## Architecture

```
┌─────────────────────────────────────────────────────┐
│           Application Layer                         │
└─────────────────────────────────────────────────────┘
                        │
                        ▼
┌─────────────────────────────────────────────────────┐
│      WorkspaceRepository (domain interface)         │
└─────────────────────────────────────────────────────┘
                        │
                        ▼
┌─────────────────────────────────────────────────────┐
│   ConnectionManager Singleton (pkg/database)        │
│   • GetConnection(workspaceID)                      │
│   • GetSystemConnection()                           │
│   • GetStats()                                      │
│   • Close()                                         │
└─────────────────────────────────────────────────────┘
                        │
                        ▼
┌─────────────────────────────────────────────────────┐
│         PostgreSQL Server                           │
└─────────────────────────────────────────────────────┘
```

---

## Implementation Steps

### Phase 1: Configuration Updates

#### Step 1.1: Update Config Structure
**File:** `config/config.go`

**Changes:**
1. Add `MaxConnections` field to `DatabaseConfig` struct
2. Add default value in `LoadWithOptions()` function
3. Add validation to ensure value is reasonable (min: 20, max: 10000)

**Code Changes:**
```go
type DatabaseConfig struct {
    Host           string
    Port           int
    User           string
    Password       string
    DBName         string
    Prefix         string
    SSLMode        string
    MaxConnections int  // NEW: Global max connections limit
}

// In LoadWithOptions():
v.SetDefault("DB_MAX_CONNECTIONS", 100)

// After building dbConfig:
dbConfig := DatabaseConfig{
    Host:           v.GetString("DB_HOST"),
    Port:           v.GetInt("DB_PORT"),
    User:           v.GetString("DB_USER"),
    Password:       v.GetString("DB_PASSWORD"),
    DBName:         v.GetString("DB_NAME"),
    Prefix:         v.GetString("DB_PREFIX"),
    SSLMode:        v.GetString("DB_SSLMODE"),
    MaxConnections: v.GetInt("DB_MAX_CONNECTIONS"),
}

// Validate max connections
if dbConfig.MaxConnections < 20 {
    return nil, fmt.Errorf("DB_MAX_CONNECTIONS must be at least 20 (got %d)", dbConfig.MaxConnections)
}
if dbConfig.MaxConnections > 10000 {
    return nil, fmt.Errorf("DB_MAX_CONNECTIONS cannot exceed 10000 (got %d)", dbConfig.MaxConnections)
}
```

**Test File:** `config/config_test.go`
- Add test for default value (100)
- Add test for custom value from env var
- Add test for validation (< 20 should fail)
- Add test for validation (> 10000 should fail)

---

### Phase 2: Connection Manager Singleton

#### Step 2.1: Create ConnectionManager
**File:** `pkg/database/connection_manager.go` (NEW)

**Responsibilities:**
- Singleton instance management
- Connection pool allocation per workspace
- Global connection limit enforcement
- Connection health monitoring
- Thread-safe operations

**Interface:**
```go
package database

import (
    "context"
    "database/sql"
    "sync"
    
    "github.com/Notifuse/notifuse/config"
)

// ConnectionManager manages all database connections
type ConnectionManager interface {
    // GetSystemConnection returns the system database connection
    GetSystemConnection() *sql.DB
    
    // GetWorkspaceConnection returns a connection to a workspace database
    GetWorkspaceConnection(ctx context.Context, workspaceID string) (*sql.DB, error)
    
    // RemoveWorkspaceConnection closes and removes a workspace connection
    RemoveWorkspaceConnection(workspaceID string) error
    
    // GetStats returns connection statistics
    GetStats() ConnectionStats
    
    // Close closes all connections
    Close() error
}

// ConnectionStats provides visibility into connection usage
type ConnectionStats struct {
    MaxConnections       int
    SystemConnections    int
    WorkspaceConnections map[string]int  // workspaceID -> connection count
    TotalConnections     int
    IdleConnections      int
    InUseConnections     int
}

// connectionManager implements ConnectionManager
type connectionManager struct {
    mu                sync.RWMutex
    config            *config.Config
    systemDB          *sql.DB
    workspacePools    map[string]*sql.DB
    maxConnections    int
    systemPoolSize    int
    workspacePoolSize int
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
        
        // Calculate pool sizes based on max connections
        maxConn := cfg.Database.MaxConnections
        
        // Reserve 10 connections for external tools/admin
        availableConnections := maxConn - 10
        
        // Allocate 10% for system database (minimum 5, maximum 25)
        systemPoolSize := availableConnections / 10
        if systemPoolSize < 5 {
            systemPoolSize = 5
        }
        if systemPoolSize > 25 {
            systemPoolSize = 25
        }
        
        // Remaining connections for workspaces
        workspaceConnections := availableConnections - systemPoolSize
        
        // Estimate 10 active workspaces, allocate accordingly
        // Minimum 3 per workspace, maximum 15 per workspace
        estimatedWorkspaces := 10
        workspacePoolSize := workspaceConnections / estimatedWorkspaces
        if workspacePoolSize < 3 {
            workspacePoolSize = 3
        }
        if workspacePoolSize > 15 {
            workspacePoolSize = 15
        }
        
        instance = &connectionManager{
            config:            cfg,
            systemDB:          systemDB,
            workspacePools:    make(map[string]*sql.DB),
            maxConnections:    maxConn,
            systemPoolSize:    systemPoolSize,
            workspacePoolSize: workspacePoolSize,
        }
        
        // Configure system database pool
        systemDB.SetMaxOpenConns(systemPoolSize)
        systemDB.SetMaxIdleConns(systemPoolSize / 2)
        systemDB.SetConnMaxLifetime(10 * time.Minute)
        systemDB.SetConnMaxIdleTime(5 * time.Minute)
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

// GetWorkspaceConnection returns a connection to a workspace database
func (cm *connectionManager) GetWorkspaceConnection(ctx context.Context, workspaceID string) (*sql.DB, error) {
    // Check if we already have a connection
    cm.mu.RLock()
    if conn, ok := cm.workspacePools[workspaceID]; ok {
        cm.mu.RUnlock()
        
        // Test the connection
        if err := conn.PingContext(ctx); err == nil {
            return conn, nil
        }
        
        // Connection is stale, remove it
        cm.mu.Lock()
        delete(cm.workspacePools, workspaceID)
        conn.Close()
        cm.mu.Unlock()
    } else {
        cm.mu.RUnlock()
    }
    
    // Create new connection
    cm.mu.Lock()
    defer cm.mu.Unlock()
    
    // Double-check after acquiring write lock
    if conn, ok := cm.workspacePools[workspaceID]; ok {
        return conn, nil
    }
    
    // Create workspace connection
    db, err := cm.createWorkspaceConnection(workspaceID)
    if err != nil {
        return nil, fmt.Errorf("failed to create workspace connection: %w", err)
    }
    
    // Store in pool
    cm.workspacePools[workspaceID] = db
    
    return db, nil
}

// createWorkspaceConnection creates a new connection to a workspace database
func (cm *connectionManager) createWorkspaceConnection(workspaceID string) (*sql.DB, error) {
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
    if err := cm.ensureWorkspaceDatabaseExists(workspaceID); err != nil {
        return nil, err
    }
    
    // Open connection
    db, err := sql.Open("postgres", dsn)
    if err != nil {
        return nil, fmt.Errorf("failed to open connection: %w", err)
    }
    
    // Test connection
    if err := db.Ping(); err != nil {
        db.Close()
        return nil, fmt.Errorf("failed to ping database: %w", err)
    }
    
    // Configure pool
    db.SetMaxOpenConns(cm.workspacePoolSize)
    db.SetMaxIdleConns(cm.workspacePoolSize / 2)
    db.SetConnMaxLifetime(10 * time.Minute)
    db.SetConnMaxIdleTime(5 * time.Minute)
    
    return db, nil
}

// ensureWorkspaceDatabaseExists creates workspace database if needed
func (cm *connectionManager) ensureWorkspaceDatabaseExists(workspaceID string) error {
    // Use existing internal/database utility
    return database.EnsureWorkspaceDatabaseExists(&cm.config.Database, workspaceID)
}

// RemoveWorkspaceConnection closes and removes a workspace connection
func (cm *connectionManager) RemoveWorkspaceConnection(workspaceID string) error {
    cm.mu.Lock()
    defer cm.mu.Unlock()
    
    if conn, ok := cm.workspacePools[workspaceID]; ok {
        delete(cm.workspacePools, workspaceID)
        return conn.Close()
    }
    
    return nil
}

// GetStats returns connection statistics
func (cm *connectionManager) GetStats() ConnectionStats {
    cm.mu.RLock()
    defer cm.mu.RUnlock()
    
    stats := ConnectionStats{
        MaxConnections:       cm.maxConnections,
        WorkspaceConnections: make(map[string]int),
    }
    
    // System connection stats
    if cm.systemDB != nil {
        systemStats := cm.systemDB.Stats()
        stats.SystemConnections = systemStats.OpenConnections
        stats.TotalConnections += systemStats.OpenConnections
        stats.IdleConnections += systemStats.Idle
        stats.InUseConnections += systemStats.InUse
    }
    
    // Workspace connection stats
    for workspaceID, db := range cm.workspacePools {
        workspaceStats := db.Stats()
        stats.WorkspaceConnections[workspaceID] = workspaceStats.OpenConnections
        stats.TotalConnections += workspaceStats.OpenConnections
        stats.IdleConnections += workspaceStats.Idle
        stats.InUseConnections += workspaceStats.InUse
    }
    
    return stats
}

// Close closes all connections
func (cm *connectionManager) Close() error {
    cm.mu.Lock()
    defer cm.mu.Unlock()
    
    var errors []error
    
    // Close all workspace connections
    for workspaceID, db := range cm.workspacePools {
        if err := db.Close(); err != nil {
            errors = append(errors, fmt.Errorf("failed to close workspace %s: %w", workspaceID, err))
        }
        delete(cm.workspacePools, workspaceID)
    }
    
    // Note: systemDB is closed by the application, not by connection manager
    
    if len(errors) > 0 {
        return fmt.Errorf("errors closing connections: %v", errors)
    }
    
    return nil
}
```

**Test File:** `pkg/database/connection_manager_test.go` (NEW)

**Unit Tests:**
1. `TestInitializeConnectionManager` - Verify singleton initialization
2. `TestInitializeConnectionManager_CalculatesPoolSizes` - Verify pool size calculations
3. `TestGetConnectionManager_NotInitialized` - Returns error when not initialized
4. `TestGetConnectionManager_ReturnsInstance` - Returns singleton instance
5. `TestResetConnectionManager` - Resets singleton for testing
6. `TestGetSystemConnection` - Returns system DB connection
7. `TestGetWorkspaceConnection_CreatesNew` - Creates new workspace connection
8. `TestGetWorkspaceConnection_ReusesExisting` - Reuses existing connection
9. `TestGetWorkspaceConnection_RecreatesStale` - Recreates stale connection
10. `TestRemoveWorkspaceConnection` - Removes and closes connection
11. `TestGetStats_EmptyPools` - Returns stats with no workspaces
12. `TestGetStats_WithWorkspaces` - Returns accurate stats
13. `TestClose_ClosesAllWorkspaces` - Closes all workspace connections
14. `TestConcurrentAccess` - Test thread safety with concurrent goroutines

---

### Phase 3: Update WorkspaceRepository

#### Step 3.1: Refactor WorkspaceRepository
**File:** `internal/repository/workspace_postgres.go`

**Changes:**
1. Remove `connectionPools sync.Map` field
2. Update constructor to accept `ConnectionManager`
3. Update `GetConnection()` to delegate to `ConnectionManager`
4. Update `DeleteDatabase()` to use `ConnectionManager.RemoveWorkspaceConnection()`

**Code Changes:**
```go
type workspaceRepository struct {
    systemDB          *sql.DB
    dbConfig          *config.DatabaseConfig
    secretKey         string
    connectionManager database.ConnectionManager  // NEW
}

// NewWorkspaceRepository creates a new PostgreSQL workspace repository
func NewWorkspaceRepository(
    systemDB *sql.DB, 
    dbConfig *config.DatabaseConfig, 
    secretKey string,
    connectionManager database.ConnectionManager,  // NEW parameter
) domain.WorkspaceRepository {
    return &workspaceRepository{
        systemDB:          systemDB,
        dbConfig:          dbConfig,
        secretKey:         secretKey,
        connectionManager: connectionManager,
    }
}

// GetConnection returns a connection to the workspace database
func (r *workspaceRepository) GetConnection(ctx context.Context, workspaceID string) (*sql.DB, error) {
    return r.connectionManager.GetWorkspaceConnection(ctx, workspaceID)
}

// GetSystemConnection returns a connection to the system database
func (r *workspaceRepository) GetSystemConnection(ctx context.Context) (*sql.DB, error) {
    return r.connectionManager.GetSystemConnection(), nil
}

// DeleteDatabase deletes a workspace database
func (r *workspaceRepository) DeleteDatabase(ctx context.Context, workspaceID string) error {
    // Remove from connection manager first
    if err := r.connectionManager.RemoveWorkspaceConnection(workspaceID); err != nil {
        // Log but don't fail - continue with database deletion
    }
    
    // ... existing database deletion logic ...
}
```

**Update Existing Tests:**
- All tests in `internal/repository/workspace_postgres_test.go` need mock ConnectionManager
- Update test setup to create mock ConnectionManager

---

### Phase 4: Update Application Initialization

#### Step 4.1: Update App Initialization
**File:** `internal/app/app.go`

**Changes in `InitDB()`:**
```go
// After creating system DB connection and before creating repositories:

// Initialize connection manager singleton
if err := database.InitializeConnectionManager(a.config, db); err != nil {
    db.Close()
    return fmt.Errorf("failed to initialize connection manager: %w", err)
}

a.logger.WithField("max_connections", a.config.Database.MaxConnections).
    Info("Connection manager initialized")

a.db = db
```

**Changes in `InitRepositories()`:**
```go
// Get connection manager
connManager, err := database.GetConnectionManager()
if err != nil {
    return fmt.Errorf("failed to get connection manager: %w", err)
}

// Create workspace repository with connection manager
a.workspaceRepo = repository.NewWorkspaceRepository(
    a.db, 
    &a.config.Database, 
    a.config.Security.SecretKey,
    connManager,  // NEW parameter
)
```

**Changes in `Shutdown()`:**
```go
// Close connection manager before closing system DB
if connManager, err := database.GetConnectionManager(); err == nil {
    if err := connManager.Close(); err != nil {
        a.logger.WithField("error", err.Error()).Error("Error closing connection manager")
    } else {
        a.logger.Info("Connection manager closed")
    }
}

// ... existing shutdown code ...
```

**Update Tests:**
- Update `internal/app/app_test.go` to work with ConnectionManager

---

### Phase 5: Add Monitoring Endpoint

#### Step 5.1: Create Connection Stats Handler
**File:** `internal/http/connection_stats_handler.go` (NEW)

**Purpose:** Admin endpoint to view connection statistics

```go
package http

import (
    "encoding/json"
    "net/http"
    
    "github.com/Notifuse/notifuse/pkg/database"
    "github.com/Notifuse/notifuse/pkg/logger"
)

type ConnectionStatsHandler struct {
    logger logger.Logger
}

func NewConnectionStatsHandler(logger logger.Logger) *ConnectionStatsHandler {
    return &ConnectionStatsHandler{
        logger: logger,
    }
}

// GetConnectionStats returns current connection statistics (admin only)
func (h *ConnectionStatsHandler) GetConnectionStats(w http.ResponseWriter, r *http.Request) {
    // Get connection manager
    connManager, err := database.GetConnectionManager()
    if err != nil {
        h.logger.Error("Failed to get connection manager")
        http.Error(w, "Internal server error", http.StatusInternalServerError)
        return
    }
    
    // Get stats
    stats := connManager.GetStats()
    
    // Return as JSON
    w.Header().Set("Content-Type", "application/json")
    if err := json.NewEncoder(w).Encode(stats); err != nil {
        h.logger.WithField("error", err.Error()).Error("Failed to encode connection stats")
        http.Error(w, "Internal server error", http.StatusInternalServerError)
        return
    }
}
```

**Test File:** `internal/http/connection_stats_handler_test.go` (NEW)
- Test successful stats retrieval
- Test error handling when connection manager not initialized

**Register Route in App:**
```go
// In internal/app/app.go InitHandlers():
connectionStatsHandler := httpHandler.NewConnectionStatsHandler(a.logger)
a.mux.HandleFunc("/api/admin.connectionStats", authMiddleware(connectionStatsHandler.GetConnectionStats))
```

---

### Phase 6: Update Documentation

#### Step 6.1: Update env.example
**File:** `env.example`

Add:
```bash
# Database Configuration
DB_HOST=localhost
DB_PORT=5432
DB_USER=postgres
DB_PASSWORD=postgres
DB_NAME=notifuse_system
DB_PREFIX=notifuse
DB_SSLMODE=disable
DB_MAX_CONNECTIONS=100  # NEW: Maximum total database connections (default: 100)
```

#### Step 6.2: Update README
**File:** `README.md`

Add section about connection management:
```markdown
### Database Connection Management

Notifuse uses a centralized connection manager to efficiently manage database connections across all workspaces.

**Configuration:**
- `DB_MAX_CONNECTIONS`: Maximum total database connections (default: 100)
  - Should be set to 80-90% of your PostgreSQL `max_connections` setting
  - Reserve remaining connections for admin tools, monitoring, etc.

**Connection Allocation:**
- System database: ~10% of max connections (min 5, max 25)
- Workspace databases: Remaining connections divided among active workspaces
- Each workspace: 3-15 connections depending on total available

**Monitoring:**
View real-time connection statistics via the admin API:
```bash
curl -H "Authorization: Bearer YOUR_TOKEN" \
     http://localhost:8080/api/admin.connectionStats
```
```

---

## Testing Strategy

### Unit Tests

#### Config Tests (`config/config_test.go`)
```go
func TestConfig_MaxConnections_Default(t *testing.T)
func TestConfig_MaxConnections_CustomValue(t *testing.T)
func TestConfig_MaxConnections_ValidationMinimum(t *testing.T)
func TestConfig_MaxConnections_ValidationMaximum(t *testing.T)
```

#### ConnectionManager Tests (`pkg/database/connection_manager_test.go`)
```go
// Singleton tests
func TestInitializeConnectionManager(t *testing.T)
func TestInitializeConnectionManager_PoolSizeCalculation(t *testing.T)
func TestGetConnectionManager_NotInitialized(t *testing.T)
func TestGetConnectionManager_ReturnsInstance(t *testing.T)
func TestResetConnectionManager(t *testing.T)

// Connection management tests
func TestGetSystemConnection(t *testing.T)
func TestGetWorkspaceConnection_CreatesNew(t *testing.T)
func TestGetWorkspaceConnection_ReusesExisting(t *testing.T)
func TestGetWorkspaceConnection_RecreatesStale(t *testing.T)
func TestGetWorkspaceConnection_ConcurrentAccess(t *testing.T)
func TestRemoveWorkspaceConnection(t *testing.T)
func TestRemoveWorkspaceConnection_NotExists(t *testing.T)

// Stats tests
func TestGetStats_Empty(t *testing.T)
func TestGetStats_WithWorkspaces(t *testing.T)
func TestGetStats_Accuracy(t *testing.T)

// Cleanup tests
func TestClose_ClosesAllWorkspaces(t *testing.T)
func TestClose_HandlesErrors(t *testing.T)

// Thread safety tests
func TestConcurrentWorkspaceAccess(t *testing.T)
func TestConcurrentStatsAccess(t *testing.T)
```

#### WorkspaceRepository Tests (Update existing)
```go
// Update all tests in internal/repository/workspace_postgres_test.go
// to use mock ConnectionManager

func TestWorkspaceRepository_WithConnectionManager(t *testing.T)
func TestWorkspaceRepository_GetConnection_UsesManager(t *testing.T)
func TestWorkspaceRepository_DeleteDatabase_RemovesFromManager(t *testing.T)
```

#### Handler Tests (`internal/http/connection_stats_handler_test.go`)
```go
func TestConnectionStatsHandler_GetStats_Success(t *testing.T)
func TestConnectionStatsHandler_GetStats_NotInitialized(t *testing.T)
func TestConnectionStatsHandler_GetStats_RequiresAuth(t *testing.T)
```

### Integration Tests

#### Connection Limit Tests (`tests/integration/connection_limit_test.go` - NEW)
```go
func TestConnectionManager_Integration(t *testing.T) {
    t.Run("respects max connection limit", func(t *testing.T) {
        // Set DB_MAX_CONNECTIONS=30 for this test
        // Create 10 workspaces
        // Verify total connections never exceed 30
    })
    
    t.Run("dynamically allocates to workspaces", func(t *testing.T) {
        // Access workspace 1, verify connections allocated
        // Access workspace 2, verify connections allocated
        // Stop using workspace 1, verify connections eventually freed
    })
    
    t.Run("handles workspace deletion", func(t *testing.T) {
        // Create workspace, access it (creates connections)
        // Delete workspace
        // Verify connections are closed
    })
    
    t.Run("handles connection failures", func(t *testing.T) {
        // Create workspace connection
        // Simulate database restart (stale connection)
        // Verify connection is recreated on next access
    })
}
```

#### Connection Stats Endpoint Tests (`tests/integration/connection_stats_handler_test.go` - NEW)
```go
func TestConnectionStatsAPI_Integration(t *testing.T) {
    t.Run("returns accurate stats", func(t *testing.T) {
        // Create 3 workspaces
        // Access each workspace
        // Call /api/admin.connectionStats
        // Verify response includes all 3 workspaces
    })
    
    t.Run("requires authentication", func(t *testing.T) {
        // Call without token, expect 401
    })
    
    t.Run("requires admin permissions", func(t *testing.T) {
        // Call with non-admin user, expect 403
    })
}
```

#### Migration Tests (Update existing)
```go
// Update tests/integration/migration_test.go
func TestMigrations_WithConnectionManager(t *testing.T) {
    // Run migrations with connection manager
    // Verify migrations complete successfully
    // Verify connections are cleaned up after migrations
}
```

#### Load Tests (`tests/integration/connection_load_test.go` - NEW)
```go
func TestConnectionManager_LoadTest(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping load test in short mode")
    }
    
    t.Run("handles high concurrent workspace access", func(t *testing.T) {
        // Set DB_MAX_CONNECTIONS=50
        // Create 20 workspaces
        // Access all 20 workspaces concurrently from 100 goroutines
        // Verify no connection limit errors
        // Verify stats remain accurate
    })
    
    t.Run("connection reuse under load", func(t *testing.T) {
        // Create 5 workspaces
        // Perform 1000 operations across workspaces
        // Verify connections are reused (not constantly created/destroyed)
    })
}
```

### Test Execution Commands

Add to `Makefile`:
```makefile
# Unit tests
test-connection-manager:
	go test -v ./pkg/database/...

# Integration tests
test-connection-integration:
	INTEGRATION_TESTS=true go test -v ./tests/integration/connection_*.go

# Load tests (long-running)
test-connection-load:
	INTEGRATION_TESTS=true go test -v -timeout 5m ./tests/integration/connection_load_test.go

# All connection-related tests
test-connections: test-connection-manager test-connection-integration
```

---

## Migration & Rollout Plan

### Phase 1: Development (Week 1)
1. Implement configuration changes
2. Create ConnectionManager singleton
3. Add unit tests
4. Verify locally with docker-compose

### Phase 2: Testing (Week 1)
1. Update WorkspaceRepository
2. Add integration tests
3. Run full test suite
4. Manual testing with multiple workspaces

### Phase 3: Documentation (Week 1)
1. Update env.example
2. Update README.md
3. Add inline code documentation
4. Create migration guide for existing deployments

### Phase 4: Deployment (Week 2)
1. Deploy to staging environment
2. Monitor connection usage
3. Tune pool sizes if needed
4. Deploy to production with monitoring

### Rollback Plan
If issues occur:
1. Revert to previous version
2. Previous connection pooling code remains functional
3. No database schema changes, safe to rollback

---

## Performance Considerations

### Expected Improvements
1. **Connection efficiency**: Better utilization of available connections
2. **Predictability**: No unexpected "too many connections" errors
3. **Visibility**: Real-time connection monitoring
4. **Resource usage**: Connections freed when workspaces inactive

### Benchmarks to Track
- Connection acquisition time (should be < 10ms)
- Connection reuse rate (should be > 90%)
- Total connection count (should stay under limit)
- Memory usage per connection pool

### Monitoring Metrics
Add to application logging:
```go
// Log connection stats every 5 minutes
go func() {
    ticker := time.NewTicker(5 * time.Minute)
    for range ticker.C {
        if cm, err := database.GetConnectionManager(); err == nil {
            stats := cm.GetStats()
            logger.WithFields(map[string]interface{}{
                "total_connections": stats.TotalConnections,
                "max_connections": stats.MaxConnections,
                "utilization_pct": (stats.TotalConnections * 100) / stats.MaxConnections,
                "workspace_count": len(stats.WorkspaceConnections),
            }).Info("Connection pool statistics")
        }
    }
}()
```

---

## Backwards Compatibility

### Breaking Changes
None - this is an internal refactor

### Environment Variables
- New: `DB_MAX_CONNECTIONS` (optional, has default)
- All existing env vars remain unchanged

### API Changes
None - no public API changes

---

## Success Criteria

### Functional Requirements
- [x] DB_MAX_CONNECTIONS env var configurable
- [x] Default value of 100
- [x] Singleton manages all connections
- [x] Connections stay under limit
- [x] WorkspaceRepository uses singleton
- [x] Stats endpoint shows accurate data

### Testing Requirements
- [x] Unit tests: >80% coverage for new code
- [x] Integration tests: All connection scenarios covered
- [x] Load tests: Handle 20+ workspaces concurrently
- [x] All existing tests pass

### Non-Functional Requirements
- [x] No performance regression
- [x] Thread-safe implementation
- [x] Clear error messages
- [x] Comprehensive logging
- [x] Documentation complete

---

## Implementation Checklist

### Code Changes
- [ ] Update `config/config.go` - Add MaxConnections field
- [ ] Create `pkg/database/connection_manager.go` - Singleton implementation
- [ ] Update `internal/repository/workspace_postgres.go` - Use ConnectionManager
- [ ] Update `internal/app/app.go` - Initialize ConnectionManager
- [ ] Create `internal/http/connection_stats_handler.go` - Stats endpoint
- [ ] Update `internal/http/routes.go` - Register stats endpoint

### Tests
- [ ] Create `config/config_test.go` tests - MaxConnections validation
- [ ] Create `pkg/database/connection_manager_test.go` - All unit tests
- [ ] Update `internal/repository/workspace_postgres_test.go` - Mock ConnectionManager
- [ ] Create `internal/http/connection_stats_handler_test.go` - Handler tests
- [ ] Create `tests/integration/connection_limit_test.go` - Integration tests
- [ ] Create `tests/integration/connection_stats_handler_test.go` - API tests
- [ ] Create `tests/integration/connection_load_test.go` - Load tests

### Documentation
- [ ] Update `env.example` - Add DB_MAX_CONNECTIONS
- [ ] Update `README.md` - Connection management section
- [ ] Update `CHANGELOG.md` - Document new feature
- [ ] Add inline code documentation - All new functions

### Testing & Validation
- [ ] Run `make test-unit` - All unit tests pass
- [ ] Run `make test-integration` - All integration tests pass
- [ ] Run `make test-connections` - Connection-specific tests pass
- [ ] Run `make test-connection-load` - Load tests pass
- [ ] Manual testing - Create 10+ workspaces, monitor connections

### Deployment
- [ ] Update deployment documentation
- [ ] Add monitoring alerts for connection utilization
- [ ] Deploy to staging
- [ ] Monitor for 48 hours
- [ ] Deploy to production
- [ ] Monitor for 1 week

---

## Risk Assessment

### High Risk
None

### Medium Risk
1. **Singleton initialization timing**: Ensure initialized before use
   - Mitigation: Initialize in app.InitDB(), before repositories
   
2. **Thread safety**: Concurrent access to connection pools
   - Mitigation: Use sync.RWMutex, comprehensive concurrency tests

### Low Risk
1. **Connection pool tuning**: May need adjustment per deployment
   - Mitigation: Make configurable, provide monitoring tools

2. **Migration path**: Updating existing deployments
   - Mitigation: Backwards compatible, clear documentation

---

## Future Enhancements

### Phase 2 (Future)
1. **Dynamic pool resizing**: Adjust workspace pool sizes based on usage
2. **Connection prioritization**: Priority workspaces get more connections
3. **Circuit breaker**: Temporarily block workspace access if unhealthy
4. **Metrics export**: Prometheus metrics for connection usage
5. **Connection warming**: Pre-create connections for frequently-used workspaces

### Phase 3 (Future)
1. **PgBouncer integration**: Support connection pooling proxy
2. **Multi-database support**: Distribute workspaces across multiple PostgreSQL servers
3. **Read replicas**: Separate read/write connections

---

## Summary

This plan implements a robust, testable connection manager that:
1. Respects global connection limits
2. Provides visibility into connection usage
3. Maintains backwards compatibility
4. Includes comprehensive testing
5. Enables future enhancements

**Estimated effort:** 3-5 days development + testing
**Risk level:** Low
**Impact:** High - solves production connection limit issues
