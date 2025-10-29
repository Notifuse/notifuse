package database

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
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
	MaxConnections           int
	MaxConnectionsPerDB      int
	SystemConnections        ConnectionPoolStats
	WorkspacePools           map[string]ConnectionPoolStats
	TotalOpenConnections     int
	TotalInUseConnections    int
	TotalIdleConnections     int
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
	workspacePools      map[string]*sql.DB    // workspaceID -> connection pool
	poolAccessTimes     map[string]time.Time  // workspaceID -> last access time
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
			poolAccessTimes:     make(map[string]time.Time),
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
	// Check if context is already cancelled before doing any work
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Check if we already have a connection pool for this workspace
	cm.mu.RLock()
	pool, ok := cm.workspacePools[workspaceID]
	cm.mu.RUnlock()

	if ok {
		// Test the connection pool is still valid
		if err := pool.PingContext(ctx); err == nil {
			// Double-check it's still in the map (not closed by another goroutine)
			cm.mu.RLock()
			stillExists := cm.workspacePools[workspaceID] == pool
			cm.mu.RUnlock()

			if stillExists {
				// Update access time for LRU tracking
				cm.mu.Lock()
				cm.poolAccessTimes[workspaceID] = time.Now()
				cm.mu.Unlock()
				return pool, nil
			}
		}

		// Pool is stale or was closed, try to clean it up safely
		cm.mu.Lock()
		// Only delete if it's still the same pool instance
		if cm.workspacePools[workspaceID] == pool {
			delete(cm.workspacePools, workspaceID)
			delete(cm.poolAccessTimes, workspaceID)
			pool.Close()
		}
		cm.mu.Unlock()
	}

	// Check context again before expensive pool creation
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	// Need to create a new pool
	cm.mu.Lock()
	defer cm.mu.Unlock()

	// Double-check after acquiring write lock (another goroutine may have created it)
	if pool, ok := cm.workspacePools[workspaceID]; ok {
		cm.poolAccessTimes[workspaceID] = time.Now()
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
	pool, err := cm.createWorkspacePool(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to create workspace pool: %w", err)
	}

	// Store in map with current access time
	cm.workspacePools[workspaceID] = pool
	cm.poolAccessTimes[workspaceID] = time.Now()

	return pool, nil
}

// createWorkspacePool creates a new connection pool for a workspace database
func (cm *connectionManager) createWorkspacePool(ctx context.Context, workspaceID string) (*sql.DB, error) {
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
		// Don't include dsn in error (contains password)
		return nil, fmt.Errorf("failed to open connection to workspace %s: %w", workspaceID, err)
	}

	// Test connection with context
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		// Don't include dsn in error (contains password)
		return nil, fmt.Errorf("failed to connect to workspace %s database: %w", workspaceID, err)
	}

	// Verify pool actually works with a test query
	var result int
	if err := db.QueryRowContext(ctx, "SELECT 1").Scan(&result); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to verify database access for workspace %s: %w", workspaceID, err)
	}

	// Configure small pool for this workspace database
	// Each workspace DB gets only a few connections since queries are short-lived
	db.SetMaxOpenConns(cm.maxConnectionsPerDB)
	db.SetMaxIdleConns(1) // Keep 1 idle connection warm
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
	type candidate struct {
		workspaceID string
		lastAccess  time.Time
	}

	var candidates []candidate

	// Find all idle pools with their access times
	for workspaceID, pool := range cm.workspacePools {
		stats := pool.Stats()

		// If no connections are in use, this pool can be closed
		if stats.InUse == 0 && stats.OpenConnections > 0 {
			accessTime := cm.poolAccessTimes[workspaceID]
			candidates = append(candidates, candidate{
				workspaceID: workspaceID,
				lastAccess:  accessTime,
			})
		}
	}

	// If no candidates, return early
	if len(candidates) == 0 {
		return 0
	}

	// Sort by access time (oldest first) - this is true LRU
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].lastAccess.Before(candidates[j].lastAccess)
	})

	// Close up to 'count' oldest idle pools
	closed := 0
	for i := 0; i < len(candidates) && i < count; i++ {
		workspaceID := candidates[i].workspaceID
		if pool, ok := cm.workspacePools[workspaceID]; ok {
			pool.Close()
			delete(cm.workspacePools, workspaceID)
			delete(cm.poolAccessTimes, workspaceID)
			closed++
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
		delete(cm.poolAccessTimes, workspaceID)
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
		delete(cm.poolAccessTimes, workspaceID)
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
