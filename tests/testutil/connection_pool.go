package testutil

import (
	"database/sql"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/Notifuse/notifuse/config"
	_ "github.com/lib/pq"
)

// TestConnectionPool manages a pool of database connections for integration tests
type TestConnectionPool struct {
	config          *config.DatabaseConfig
	systemPool      *sql.DB
	workspacePools  map[string]*sql.DB
	poolMutex       sync.RWMutex
	maxConnections  int
	maxIdleTime     time.Duration
	connectionCount int
}

// NewTestConnectionPool creates a new connection pool for tests
func NewTestConnectionPool(config *config.DatabaseConfig) *TestConnectionPool {
	return &TestConnectionPool{
		config:         config,
		workspacePools: make(map[string]*sql.DB),
		maxConnections: 10, // Conservative limit for tests
		maxIdleTime:    2 * time.Minute,
	}
}

// GetSystemConnection returns a connection to the system database
func (pool *TestConnectionPool) GetSystemConnection() (*sql.DB, error) {
	pool.poolMutex.Lock()
	defer pool.poolMutex.Unlock()

	if pool.systemPool == nil {
		systemDSN := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=postgres sslmode=%s",
			pool.config.Host, pool.config.Port, pool.config.User, pool.config.Password, pool.config.SSLMode)

		db, err := sql.Open("postgres", systemDSN)
		if err != nil {
			return nil, fmt.Errorf("failed to create system connection: %w", err)
		}

		// Configure connection pool for tests
		db.SetMaxOpenConns(5) // Conservative for system operations
		db.SetMaxIdleConns(2)
		db.SetConnMaxLifetime(pool.maxIdleTime)
		db.SetConnMaxIdleTime(pool.maxIdleTime / 2)

		if err := db.Ping(); err != nil {
			db.Close()
			return nil, fmt.Errorf("failed to ping system database: %w", err)
		}

		pool.systemPool = db
	}

	return pool.systemPool, nil
}

// GetWorkspaceConnection returns a pooled connection to a workspace database
func (pool *TestConnectionPool) GetWorkspaceConnection(workspaceID string) (*sql.DB, error) {
	pool.poolMutex.Lock()
	defer pool.poolMutex.Unlock()

	// Check if we already have a connection for this workspace
	if db, exists := pool.workspacePools[workspaceID]; exists {
		// Test if connection is still alive
		if err := db.Ping(); err == nil {
			return db, nil
		}
		// Connection is dead, remove it
		db.Close()
		delete(pool.workspacePools, workspaceID)
		pool.connectionCount--
	}

	// Create new connection
	workspaceDBName := fmt.Sprintf("%s_ws_%s", pool.config.Prefix, workspaceID)
	workspaceDSN := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		pool.config.Host, pool.config.Port, pool.config.User, pool.config.Password, workspaceDBName, pool.config.SSLMode)

	db, err := sql.Open("postgres", workspaceDSN)
	if err != nil {
		return nil, fmt.Errorf("failed to create workspace connection: %w", err)
	}

	// Configure connection pool for tests - use smaller pools
	db.SetMaxOpenConns(3) // Very conservative for individual workspaces
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(pool.maxIdleTime)
	db.SetConnMaxIdleTime(pool.maxIdleTime / 2)

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping workspace database: %w", err)
	}

	pool.workspacePools[workspaceID] = db
	pool.connectionCount++

	return db, nil
}

// EnsureWorkspaceDatabase creates the workspace database if it doesn't exist
func (pool *TestConnectionPool) EnsureWorkspaceDatabase(workspaceID string) error {
	systemDB, err := pool.GetSystemConnection()
	if err != nil {
		return fmt.Errorf("failed to get system connection: %w", err)
	}

	workspaceDBName := fmt.Sprintf("%s_ws_%s", pool.config.Prefix, workspaceID)

	// Check if database exists
	var exists bool
	query := "SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname = $1)"
	err = systemDB.QueryRow(query, workspaceDBName).Scan(&exists)
	if err != nil {
		return fmt.Errorf("failed to check if workspace database exists: %w", err)
	}

	// Create database if it doesn't exist
	if !exists {
		createDBQuery := fmt.Sprintf("CREATE DATABASE %s", workspaceDBName)
		_, err = systemDB.Exec(createDBQuery)
		if err != nil {
			return fmt.Errorf("failed to create workspace database: %w", err)
		}
	}

	return nil
}

// CleanupWorkspace removes a workspace connection from the pool
func (pool *TestConnectionPool) CleanupWorkspace(workspaceID string) error {
	pool.poolMutex.Lock()
	defer pool.poolMutex.Unlock()

	if db, exists := pool.workspacePools[workspaceID]; exists {
		db.Close()
		delete(pool.workspacePools, workspaceID)
		pool.connectionCount--
	}

	// Also drop the workspace database
	if pool.systemPool != nil {
		workspaceDBName := fmt.Sprintf("%s_ws_%s", pool.config.Prefix, workspaceID)

		// Terminate connections to the workspace database
		terminateQuery := fmt.Sprintf(`
			SELECT pg_terminate_backend(pid) 
			FROM pg_stat_activity 
			WHERE datname = '%s' 
			AND pid <> pg_backend_pid()`, workspaceDBName)

		pool.systemPool.Exec(terminateQuery)

		// Small delay for connections to close
		time.Sleep(100 * time.Millisecond)

		// Drop the database
		dropQuery := fmt.Sprintf("DROP DATABASE IF EXISTS %s", workspaceDBName)
		_, err := pool.systemPool.Exec(dropQuery)
		if err != nil {
			return fmt.Errorf("failed to drop workspace database: %w", err)
		}
	}

	return nil
}

// GetConnectionCount returns the current number of active connections
func (pool *TestConnectionPool) GetConnectionCount() int {
	pool.poolMutex.RLock()
	defer pool.poolMutex.RUnlock()
	return pool.connectionCount
}

// Cleanup closes all connections in the pool
func (pool *TestConnectionPool) Cleanup() error {
	pool.poolMutex.Lock()
	defer pool.poolMutex.Unlock()

	// Close all workspace connections
	for workspaceID, db := range pool.workspacePools {
		db.Close()
		delete(pool.workspacePools, workspaceID)
	}

	// Close system connection
	if pool.systemPool != nil {
		pool.systemPool.Close()
		pool.systemPool = nil
	}

	pool.connectionCount = 0
	return nil
}

// Global connection pool instance for tests
var globalTestPool *TestConnectionPool
var poolOnce sync.Once

// GetGlobalTestPool returns a singleton connection pool for all tests
func GetGlobalTestPool() *TestConnectionPool {
	poolOnce.Do(func() {
		// Default to localhost for normal environments
		// In containerized environments (like Cursor), set TEST_DB_HOST env var
		// to the actual container IP or accessible hostname
		defaultHost := "localhost"
		defaultPort := 5433
		
		// Check if we're likely in a containerized environment
		// If TEST_DB_HOST is explicitly set, use it with its port
		testHost := getEnvOrDefault("TEST_DB_HOST", defaultHost)
		testPort := defaultPort
		if testHost != defaultHost {
			// If custom host is set, likely need internal port
			if os.Getenv("TEST_DB_PORT") != "" {
				fmt.Sscanf(os.Getenv("TEST_DB_PORT"), "%d", &testPort)
			} else {
				testPort = 5432 // Default to internal port when using custom host
			}
		}
		
		config := &config.DatabaseConfig{
			Host:     testHost,
			Port:     testPort,
			User:     getEnvOrDefault("TEST_DB_USER", "notifuse_test"),
			Password: getEnvOrDefault("TEST_DB_PASSWORD", "test_password"),
			Prefix:   "notifuse_test",
			SSLMode:  "disable",
		}
		globalTestPool = NewTestConnectionPool(config)
	})
	return globalTestPool
}

// CleanupGlobalTestPool cleans up the global test pool
func CleanupGlobalTestPool() error {
	if globalTestPool != nil {
		err := globalTestPool.Cleanup()
		globalTestPool = nil
		// Reset the sync.Once so the pool can be re-initialized in the next test
		poolOnce = sync.Once{}
		// Give PostgreSQL time to release connections
		time.Sleep(500 * time.Millisecond)
		return err
	}
	return nil
}
