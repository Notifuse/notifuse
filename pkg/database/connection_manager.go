package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/database"
	"github.com/lib/pq"
	"golang.org/x/sync/singleflight"
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
	MaxConnections           int                            `json:"max_connections"`
	MaxConnectionsPerDB      int                            `json:"max_connections_per_db"`
	SystemConnections        ConnectionPoolStats            `json:"system_connections"`
	WorkspacePools           map[string]ConnectionPoolStats `json:"-"`
	TotalOpenConnections     int                            `json:"total_open_connections"`
	TotalInUseConnections    int                            `json:"total_in_use_connections"`
	TotalIdleConnections     int                            `json:"total_idle_connections"`
	ActiveWorkspaceDatabases int                            `json:"-"`
}

// ConnectionPoolStats provides stats for a single connection pool
type ConnectionPoolStats struct {
	OpenConnections int           `json:"open_connections"`
	InUse           int           `json:"in_use"`
	Idle            int           `json:"idle"`
	MaxOpen         int           `json:"max_open"`
	WaitCount       int64         `json:"wait_count"`
	WaitDuration    time.Duration `json:"wait_duration"`
}

// workspacePoolCreateTimeout bounds a single pool creation (connect + verify,
// plus a one-time lazy database create). It exists so that pool creation, which
// is detached from any individual caller's context (see GetWorkspaceConnection),
// can never hang indefinitely on an unreachable server.
const workspacePoolCreateTimeout = 30 * time.Second

// connectionManager implements ConnectionManager
type connectionManager struct {
	mu                  sync.RWMutex
	config              *config.Config
	systemDB            *sql.DB
	workspacePools      map[string]*sql.DB   // workspaceID -> connection pool
	poolAccessTimes     map[string]time.Time // workspaceID -> last access time
	maxConnections      int
	maxConnectionsPerDB int
	inflightPools       int                // pools reserved but not yet inserted, counted against capacity
	createGroup         singleflight.Group // coalesces concurrent pool creation per workspace
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
		_ = instance.Close()
		instance = nil
	}
	instanceOnce = sync.Once{}
}

// GetSystemConnection returns the system database connection
func (cm *connectionManager) GetSystemConnection() *sql.DB {
	return cm.systemDB
}

// GetWorkspaceConnection returns a connection pool for a workspace database.
//
// The returned *sql.DB is a long-lived, self-healing connection pool: it
// validates connections and transparently retries on driver.ErrBadConn. We
// therefore do NOT ping it on every call. A per-call ping added a full
// round-trip to every query and, worse, a transient blip (or a ping merely slow
// under broadcast WAL pressure) used to evict and Close an otherwise-healthy
// pool out from under every goroutine still holding it, surfacing as spurious
// "failed to get workspace connection" errors that vanish on retry.
func (cm *connectionManager) GetWorkspaceConnection(ctx context.Context, workspaceID string) (*sql.DB, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Fast path: return the cached pool directly. Note: a pool whose workspace
	// database was dropped out-of-band (e.g. by another instance in a
	// multi-instance deployment) will linger here and error on use until
	// CloseWorkspaceConnection is called — the single-instance self-hosted target
	// deletes the pool on the same instance, so this is not reachable there.
	cm.mu.RLock()
	pool, ok := cm.workspacePools[workspaceID]
	cm.mu.RUnlock()
	if ok {
		cm.mu.Lock()
		cm.poolAccessTimes[workspaceID] = time.Now()
		cm.mu.Unlock()
		return pool, nil
	}

	// Slow path: create the pool. singleflight coalesces concurrent creators for
	// the SAME workspace into a single creation, and — crucially — no global
	// lock is held across the network I/O of pool creation, so creating one
	// workspace's pool never blocks access to a different workspace.
	v, err, _ := cm.createGroup.Do(workspaceID, func() (any, error) {
		// Another caller may have finished creating it while we waited.
		cm.mu.RLock()
		existing, ok := cm.workspacePools[workspaceID]
		cm.mu.RUnlock()
		if ok {
			return existing, nil
		}

		// Reserve capacity (fast, in-memory; may evict LRU idle pools).
		if err := cm.reserveCapacityForNewPool(workspaceID); err != nil {
			return nil, err
		}
		defer cm.releasePoolReservation()

		// Detach creation from the (arbitrary) first coalesced caller's context:
		// singleflight serves every waiter from this one creation, so one
		// caller's cancellation must not fail the others. A bounded timeout keeps
		// a hung connect from wedging all waiters.
		createCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), workspacePoolCreateTimeout)
		defer cancel()

		// Network I/O happens WITHOUT cm.mu held.
		newPool, err := cm.createWorkspacePool(createCtx, workspaceID)
		if err != nil {
			return nil, fmt.Errorf("failed to create workspace pool: %w", err)
		}

		cm.mu.Lock()
		cm.workspacePools[workspaceID] = newPool
		cm.poolAccessTimes[workspaceID] = time.Now()
		cm.mu.Unlock()
		return newPool, nil
	})
	if err != nil {
		return nil, err
	}
	return v.(*sql.DB), nil
}

// reserveCapacityForNewPool verifies there is room for one more workspace pool
// and, on success, records an in-flight reservation so that concurrent creations
// of *distinct* workspaces cannot collectively overshoot maxConnections. The
// caller must pair a successful reservation with releasePoolReservation. It
// briefly takes cm.mu but never holds it across network I/O.
func (cm *connectionManager) reserveCapacityForNewPool(workspaceID string) error {
	cm.mu.Lock()
	if cm.hasCapacityForNewPool() {
		cm.inflightPools++
		cm.mu.Unlock()
		return nil
	}
	cm.mu.Unlock()

	// Try to free capacity by closing least-recently-used idle pools.
	cm.closeLRUIdlePools(1)

	cm.mu.Lock()
	defer cm.mu.Unlock()
	if cm.hasCapacityForNewPool() {
		cm.inflightPools++
		return nil
	}
	return &ConnectionLimitError{
		MaxConnections:     cm.maxConnections,
		CurrentConnections: cm.getTotalConnectionCount(),
		WorkspaceID:        workspaceID,
	}
}

// releasePoolReservation releases an in-flight reservation taken by
// reserveCapacityForNewPool, once the pool is either inserted into the map (and
// thus counted via its own Stats) or creation has failed.
func (cm *connectionManager) releasePoolReservation() {
	cm.mu.Lock()
	if cm.inflightPools > 0 {
		cm.inflightPools--
	}
	cm.mu.Unlock()
}

// createWorkspacePool creates a new connection pool for a workspace database.
//
// The workspace database is normally provisioned up-front at workspace creation
// (see workspaceRepository.CreateDatabase). We therefore connect directly and
// only fall back to EnsureWorkspaceDatabaseExists — which connects to the
// `postgres` admin database — when the workspace DB is genuinely missing. Doing
// that admin-DB round-trip on every pool creation caused a storm of admin
// connections whenever pools were recreated.
func (cm *connectionManager) createWorkspacePool(ctx context.Context, workspaceID string) (*sql.DB, error) {
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

	db, err := cm.openAndVerifyPool(ctx, dsn, workspaceID)
	if err != nil {
		// If the workspace database does not exist yet (e.g. first touch by a
		// fresh process), create it once and retry — instead of paying an
		// admin-DB round-trip on every pool creation.
		if isDatabaseDoesNotExistErr(err) {
			if ensureErr := database.EnsureWorkspaceDatabaseExists(&cm.config.Database, workspaceID); ensureErr != nil {
				return nil, ensureErr
			}
			db, err = cm.openAndVerifyPool(ctx, dsn, workspaceID)
		}
		if err != nil {
			return nil, err
		}
	}

	// Configure small pool for this workspace database.
	// Each workspace DB gets only a few connections since queries are short-lived.
	db.SetMaxOpenConns(cm.maxConnectionsPerDB)
	db.SetMaxIdleConns(1) // Keep 1 idle connection warm
	db.SetConnMaxLifetime(cm.config.Database.ConnectionMaxLifetime)
	db.SetConnMaxIdleTime(cm.config.Database.ConnectionMaxIdleTime)

	return db, nil
}

// openAndVerifyPool opens a connection pool for the given DSN and verifies it is
// usable with a ping and a trivial query. Errors never include the DSN (which
// contains the password).
func (cm *connectionManager) openAndVerifyPool(ctx context.Context, dsn, workspaceID string) (*sql.DB, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open connection to workspace %s: %w", workspaceID, err)
	}

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to connect to workspace %s database: %w", workspaceID, err)
	}

	var result int
	if err := db.QueryRowContext(ctx, "SELECT 1").Scan(&result); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to verify database access for workspace %s: %w", workspaceID, err)
	}

	return db, nil
}

// isDatabaseDoesNotExistErr reports whether err is PostgreSQL's
// invalid_catalog_name (3D000), i.e. the target database does not exist.
func isDatabaseDoesNotExistErr(err error) bool {
	var pqErr *pq.Error
	if errors.As(err, &pqErr) {
		return pqErr.Code == "3D000"
	}
	return false
}

// hasCapacityForNewPool checks if we have capacity for a new connection pool.
// It reserves maxConnectionsPerDB for the pool being created plus for every
// other pool currently in flight (reserved but not yet inserted), so concurrent
// creations of distinct workspaces cannot collectively exceed maxConnections.
// Must be called with write lock held.
func (cm *connectionManager) hasCapacityForNewPool() bool {
	currentTotal := cm.getTotalConnectionCount()

	// Calculate projected total if we add this pool plus any already in flight.
	projectedTotal := currentTotal + (cm.inflightPools+1)*cm.maxConnectionsPerDB

	return projectedTotal <= cm.maxConnections
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

// identifyLRUCandidates identifies idle workspace pools for eviction using LRU policy
// Returns workspace IDs sorted by least recently used (oldest first)
// This method acquires a read lock internally
func (cm *connectionManager) identifyLRUCandidates(count int) []string {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

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

	// If no candidates, return empty slice
	if len(candidates) == 0 {
		return nil
	}

	// Sort by access time (oldest first) - this is true LRU
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].lastAccess.Before(candidates[j].lastAccess)
	})

	// Return up to 'count' workspace IDs
	result := make([]string, 0, count)
	for i := 0; i < len(candidates) && i < count; i++ {
		result = append(result, candidates[i].workspaceID)
	}

	return result
}

// closeLRUIdlePools closes up to 'count' least recently used idle pools
// Returns the number of pools actually closed
// This method uses two-phase eviction: identify candidates with read lock,
// then close with write lock. Must be called WITHOUT lock held.
func (cm *connectionManager) closeLRUIdlePools(count int) int {
	// Phase 1: Identify candidates (with read lock inside identifyLRUCandidates)
	candidates := cm.identifyLRUCandidates(count)

	// If no candidates, return early
	if len(candidates) == 0 {
		return 0
	}

	// Phase 2: Close pools (acquire write lock only for closing)
	cm.mu.Lock()
	defer cm.mu.Unlock()

	closed := 0
	for _, workspaceID := range candidates {
		if pool, ok := cm.workspacePools[workspaceID]; ok {
			// Re-check pool is still idle (state may have changed between phases)
			stats := pool.Stats()
			if stats.InUse == 0 {
				_ = pool.Close()
				delete(cm.workspacePools, workspaceID)
				delete(cm.poolAccessTimes, workspaceID)
				closed++
			}
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

// IsConnectionLimitError checks if an error is a connection limit error,
// including when it has been wrapped with fmt.Errorf("%w").
func IsConnectionLimitError(err error) bool {
	var e *ConnectionLimitError
	return errors.As(err, &e)
}
