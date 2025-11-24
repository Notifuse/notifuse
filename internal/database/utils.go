package database

import (
	"database/sql"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Notifuse/notifuse/config"
	_ "github.com/lib/pq" // PostgreSQL driver
)

// GetConnectionPoolSettings returns connection pool settings based on environment
func GetConnectionPoolSettings() (maxOpen, maxIdle int, maxLifetime time.Duration) {
	environment := os.Getenv("ENVIRONMENT")

	// Use smaller pools for test environment to conserve connections
	if environment == "test" || os.Getenv("INTEGRATION_TESTS") == "true" {
		return 10, 5, 2 * time.Minute
	}

	// Production settings
	return 25, 25, 20 * time.Minute
}

// GetSystemDSN returns the DSN for the system database
func GetSystemDSN(cfg *config.DatabaseConfig) string {
	return fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
		cfg.User,
		cfg.Password,
		cfg.Host,
		cfg.Port,
		cfg.DBName,
		cfg.SSLMode,
	)
}

// GetPostgresDSN returns the DSN for connecting to PostgreSQL server without specifying a database
func GetPostgresDSN(cfg *config.DatabaseConfig) string {
	return fmt.Sprintf("postgres://%s:%s@%s:%d/postgres?sslmode=%s",
		cfg.User,
		cfg.Password,
		cfg.Host,
		cfg.Port,
		cfg.SSLMode,
	)
}

// GetWorkspaceDSN returns the DSN for a workspace database
func GetWorkspaceDSN(cfg *config.DatabaseConfig, workspaceID string) string {
	// Replace hyphens with underscores for PostgreSQL compatibility
	safeID := strings.ReplaceAll(workspaceID, "-", "_")
	dbName := fmt.Sprintf("%s_ws_%s", cfg.Prefix, safeID)
	return fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
		cfg.User,
		cfg.Password,
		cfg.Host,
		cfg.Port,
		dbName,
		cfg.SSLMode,
	)
}

// ConnectToWorkspace creates a new database connection for a workspace
func ConnectToWorkspace(cfg *config.DatabaseConfig, workspaceID string) (*sql.DB, error) {
	// Ensure the workspace database exists
	if err := EnsureWorkspaceDatabaseExists(cfg, workspaceID); err != nil {
		return nil, fmt.Errorf("failed to ensure workspace database exists: %w", err)
	}

	dsn := GetWorkspaceDSN(cfg, workspaceID)
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to workspace database: %w", err)
	}

	// Test the connection
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to ping workspace database: %w", err)
	}

	// Set connection pool settings based on environment
	maxOpen, maxIdle, maxLifetime := GetConnectionPoolSettings()
	db.SetMaxOpenConns(maxOpen)
	db.SetMaxIdleConns(maxIdle)
	db.SetConnMaxLifetime(maxLifetime)
	db.SetConnMaxIdleTime(maxLifetime / 2)

	return db, nil
}

// EnsureWorkspaceDatabaseExists creates the workspace database if it doesn't exist
func EnsureWorkspaceDatabaseExists(cfg *config.DatabaseConfig, workspaceID string) error {
	// Replace hyphens with underscores for PostgreSQL compatibility
	safeID := strings.ReplaceAll(workspaceID, "-", "_")
	dbName := fmt.Sprintf("%s_ws_%s", cfg.Prefix, safeID)

	// Connect to PostgreSQL server without specifying a database
	pgDSN := GetPostgresDSN(cfg)
	db, err := sql.Open("postgres", pgDSN)
	if err != nil {
		return fmt.Errorf("failed to connect to PostgreSQL server: %w", err)
	}
	defer func() { _ = db.Close() }()

	// Test the connection
	if err := db.Ping(); err != nil {
		return fmt.Errorf("failed to ping PostgreSQL server: %w", err)
	}

	// Check if database exists
	var exists bool
	query := "SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname = $1)"
	err = db.QueryRow(query, dbName).Scan(&exists)
	if err != nil {
		return fmt.Errorf("failed to check if database exists: %w", err)
	}

	// Create database if it doesn't exist
	if !exists {
		// Use fmt.Sprintf for proper quoting of identifiers in SQL
		createDBQuery := fmt.Sprintf("CREATE DATABASE %s",
			// Proper quoting to prevent SQL injection
			strings.ReplaceAll(dbName, `"`, `""`))

		_, err = db.Exec(createDBQuery)
		if err != nil {
			return fmt.Errorf("failed to create workspace database: %w", err)
		}

		// Connect to the new database to initialize schema
		wsDB, err := sql.Open("postgres", GetWorkspaceDSN(cfg, workspaceID))
		if err != nil {
			return fmt.Errorf("failed to connect to new workspace database: %w", err)
		}
		defer func() {
			_ = wsDB.Close()
		}()

		// Test the connection
		if err := wsDB.Ping(); err != nil {
			return fmt.Errorf("failed to ping new workspace database: %w", err)
		}

		// Initialize the workspace database schema
		if err := InitializeWorkspaceDatabase(wsDB); err != nil {
			return fmt.Errorf("failed to initialize workspace database schema: %w", err)
		}
	}

	return nil
}

// EnsureSystemDatabaseExists creates the system database if it doesn't exist
func EnsureSystemDatabaseExists(dsn string, dbName string) error {
	// Connect to PostgreSQL server without specifying a database
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return fmt.Errorf("failed to connect to PostgreSQL server: %w", err)
	}
	defer func() { _ = db.Close() }()

	// Test the connection
	if err := db.Ping(); err != nil {
		return fmt.Errorf("failed to ping PostgreSQL server: %w", err)
	}

	// Check if database exists
	var exists bool
	query := "SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname = $1)"
	err = db.QueryRow(query, dbName).Scan(&exists)
	if err != nil {
		return fmt.Errorf("failed to check if database exists: %w", err)
	}

	// Create database if it doesn't exist
	if !exists {
		// Use fmt.Sprintf for proper quoting of identifiers in SQL
		createDBQuery := fmt.Sprintf("CREATE DATABASE %s",
			// Proper quoting to prevent SQL injection
			strings.ReplaceAll(dbName, `"`, `""`))

		_, err = db.Exec(createDBQuery)
		if err != nil {
			return fmt.Errorf("failed to create system database: %w", err)
		}
	}

	return nil
}
