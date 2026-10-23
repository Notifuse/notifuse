package database

import (
	"database/sql"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/Notifuse/notifuse/config"
	"github.com/lib/pq"
)

// workspaceIDInDatabaseName matches the characters allowed in the workspace ID part of a
// database name, capitals included (refused next, with their own message). The ID is
// written into DSNs and SQL as is. Workspace IDs have at most 20 characters; the bound only
// keeps an arbitrary input from going further.
var workspaceIDInDatabaseName = regexp.MustCompile(`^[A-Za-z0-9_]{1,63}$`)

// WorkspaceDatabaseName returns the name of a workspace's database, or an error when the
// workspace ID cannot be part of one. Every place that names a workspace database goes
// through it, so the name a database is created under is the one it is opened and
// dropped under.
func WorkspaceDatabaseName(prefix, workspaceID string) (string, error) {
	// Replace hyphens with underscores for PostgreSQL compatibility
	safeID := strings.ReplaceAll(workspaceID, "-", "_")
	// Characters first, case aside, so that an ID refused for its characters is reported
	// quoted whatever its case.
	if !workspaceIDInDatabaseName.MatchString(safeID) {
		return "", fmt.Errorf("invalid workspace ID for a database name: %q", workspaceID)
	}
	dbName := fmt.Sprintf("%s_ws_%s", prefix, safeID)

	// PostgreSQL folds the ASCII capitals of an unquoted name to lowercase, so a name with
	// one would be created and opened under different names by tools that do not quote it.
	if strings.ContainsAny(dbName, "ABCDEFGHIJKLMNOPQRSTUVWXYZ") {
		return "", fmt.Errorf("workspace database name must be lowercase: %s", dbName)
	}
	return dbName, nil
}

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
func GetWorkspaceDSN(cfg *config.DatabaseConfig, workspaceID string) (string, error) {
	dbName, err := WorkspaceDatabaseName(cfg.Prefix, workspaceID)
	if err != nil {
		return "", err
	}
	return workspaceDSNForName(cfg, dbName), nil
}

// workspaceDSNForName returns the DSN for a database name WorkspaceDatabaseName returned.
func workspaceDSNForName(cfg *config.DatabaseConfig, dbName string) string {
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

	dsn, err := GetWorkspaceDSN(cfg, workspaceID)
	if err != nil {
		return nil, err
	}
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
	dbName, err := WorkspaceDatabaseName(cfg.Prefix, workspaceID)
	if err != nil {
		return err
	}

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
		_, err = db.Exec("CREATE DATABASE " + pq.QuoteIdentifier(dbName))
		if err != nil {
			return fmt.Errorf("failed to create workspace database: %w", err)
		}

		// Connect to the new database to initialize schema
		wsDB, err := sql.Open("postgres", workspaceDSNForName(cfg, dbName))
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
