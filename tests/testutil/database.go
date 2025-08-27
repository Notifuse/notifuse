package testutil

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/database"
	"github.com/Notifuse/notifuse/pkg/crypto"
	_ "github.com/lib/pq"
)

// DatabaseManager manages test database lifecycle
type DatabaseManager struct {
	config         *config.DatabaseConfig
	db             *sql.DB
	dbName         string
	systemDB       *sql.DB
	isSetup        bool
	connectionPool *TestConnectionPool
}

// NewDatabaseManager creates a new database manager for testing
func NewDatabaseManager() *DatabaseManager {
	config := &config.DatabaseConfig{
		Host:     getEnvOrDefault("TEST_DB_HOST", "localhost"),
		Port:     5433, // Different port for test DB
		User:     getEnvOrDefault("TEST_DB_USER", "notifuse_test"),
		Password: getEnvOrDefault("TEST_DB_PASSWORD", "test_password"),
		DBName:   fmt.Sprintf("notifuse_test_%d", time.Now().UnixNano()),
		Prefix:   "notifuse_test",
		SSLMode:  "disable",
	}

	return &DatabaseManager{
		config:         config,
		connectionPool: GetGlobalTestPool(),
	}
}

// Setup creates the test database and initializes it
func (dm *DatabaseManager) Setup() error {
	if dm.isSetup {
		return nil
	}

	// Get system connection from pool
	var err error
	dm.systemDB, err = dm.connectionPool.GetSystemConnection()
	if err != nil {
		return fmt.Errorf("failed to get system connection from pool: %w", err)
	}

	// Create test database
	_, err = dm.systemDB.Exec(fmt.Sprintf("CREATE DATABASE %s", dm.config.DBName))
	if err != nil {
		return fmt.Errorf("failed to create test database: %w", err)
	}

	dm.dbName = dm.config.DBName

	// Connect to the test database - use direct connection for system database
	testDSN := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		dm.config.Host, dm.config.Port, dm.config.User, dm.config.Password, dm.config.DBName, dm.config.SSLMode)

	dm.db, err = sql.Open("postgres", testDSN)
	if err != nil {
		return fmt.Errorf("failed to connect to test database: %w", err)
	}

	// Configure connection pool for system test database
	dm.db.SetMaxOpenConns(5)
	dm.db.SetMaxIdleConns(2)
	dm.db.SetConnMaxLifetime(2 * time.Minute)
	dm.db.SetConnMaxIdleTime(1 * time.Minute)

	// Test connection
	if err := dm.db.Ping(); err != nil {
		return fmt.Errorf("failed to ping test database: %w", err)
	}

	// Initialize the database schema
	if err := dm.runMigrations(); err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	dm.isSetup = true
	return nil
}

// GetDB returns the test database connection
func (dm *DatabaseManager) GetDB() *sql.DB {
	return dm.db
}

// GetConfig returns the test database configuration
func (dm *DatabaseManager) GetConfig() *config.DatabaseConfig {
	return dm.config
}

// GetWorkspaceDB returns a connection to the workspace database
func (dm *DatabaseManager) GetWorkspaceDB(workspaceID string) (*sql.DB, error) {
	// Ensure workspace database exists
	if err := dm.connectionPool.EnsureWorkspaceDatabase(workspaceID); err != nil {
		return nil, fmt.Errorf("failed to ensure workspace database exists: %w", err)
	}

	// Get connection from pool
	workspaceDB, err := dm.connectionPool.GetWorkspaceConnection(workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace connection from pool: %w", err)
	}

	return workspaceDB, nil
}

// SeedTestData seeds the database with test data
func (dm *DatabaseManager) SeedTestData() error {
	if !dm.isSetup {
		return fmt.Errorf("database not setup")
	}

	// Create a test user with valid UUID (using different email to avoid conflict with root user)
	testUserID := "550e8400-e29b-41d4-a716-446655440000"
	testUserQuery := `
		INSERT INTO users (id, email, name, type, created_at, updated_at)
		VALUES ($1, 'testuser@example.com', 'Test User', 'user', NOW(), NOW())
		ON CONFLICT (email) DO NOTHING
	`
	_, err := dm.db.Exec(testUserQuery, testUserID)
	if err != nil {
		return fmt.Errorf("failed to create test user: %w", err)
	}

	// Create a test workspace with valid UUID and proper encrypted secret key
	testWorkspaceID := "testws01"

	// Create workspace settings with encrypted secret key
	// For testing, we'll use a simple secret key and encrypt it with the same global key used in server.go
	testSecretKey := "test-workspace-secret-key-for-integration-tests"
	testGlobalKey := "test-secret-key-for-integration-tests-only" // Must match server.go SecurityConfig.SecretKey

	// Import crypto package functions
	encryptedSecretKey, err := crypto.EncryptString(testSecretKey, testGlobalKey)
	if err != nil {
		return fmt.Errorf("failed to encrypt test workspace secret key: %w", err)
	}

	workspaceSettings := fmt.Sprintf(`{
		"timezone": "UTC",
		"encrypted_secret_key": "%s"
	}`, encryptedSecretKey)

	testWorkspaceQuery := `
		INSERT INTO workspaces (id, name, settings, integrations, created_at, updated_at)
		VALUES ($1, 'Test Workspace', $2, '[]', NOW(), NOW())
		ON CONFLICT (id) DO NOTHING
	`
	_, err = dm.db.Exec(testWorkspaceQuery, testWorkspaceID, workspaceSettings)
	if err != nil {
		return fmt.Errorf("failed to create test workspace: %w", err)
	}

	// Create workspace user association
	workspaceUserQuery := `
		INSERT INTO user_workspaces (user_id, workspace_id, role, created_at, updated_at)
		VALUES ($1, $2, 'owner', NOW(), NOW())
		ON CONFLICT (user_id, workspace_id) DO NOTHING
	`
	_, err = dm.db.Exec(workspaceUserQuery, testUserID, testWorkspaceID)
	if err != nil {
		return fmt.Errorf("failed to create workspace user association: %w", err)
	}

	return nil
}

// CleanupTestData removes all test data but keeps schema
func (dm *DatabaseManager) CleanupTestData() error {
	if !dm.isSetup {
		return nil
	}

	// List of tables to clean in dependency order
	tables := []string{
		"user_workspaces",
		"message_history",
		"broadcasts",
		"templates",
		"contact_lists",
		"lists",
		"contacts",
		"transactional_notifications",
		"webhook_events",
		"tasks",
		"workspaces",
		"users",
	}

	// Clean each table
	for _, table := range tables {
		_, err := dm.db.Exec(fmt.Sprintf("DELETE FROM %s", table))
		if err != nil {
			log.Printf("Warning: failed to clean table %s: %v", table, err)
		}
	}

	return nil
}

// Cleanup drops the test database and closes connections
func (dm *DatabaseManager) Cleanup() error {
	if dm.db != nil {
		dm.db.Close()
	}

	if dm.systemDB != nil && dm.dbName != "" {
		// Drop the test database
		_, err := dm.systemDB.Exec(fmt.Sprintf("DROP DATABASE IF EXISTS %s", dm.dbName))
		if err != nil {
			log.Printf("Warning: failed to drop test database: %v", err)
		}
		// Note: Don't close systemDB as it's managed by the connection pool
	}

	dm.isSetup = false
	return nil
}

// runMigrations runs the database migrations
func (dm *DatabaseManager) runMigrations() error {
	// Initialize system tables
	if err := database.InitializeDatabase(dm.db, "test@example.com"); err != nil {
		return fmt.Errorf("failed to initialize system database: %w", err)
	}

	// Initialize workspace tables
	if err := database.InitializeWorkspaceDatabase(dm.db); err != nil {
		return fmt.Errorf("failed to initialize workspace database: %w", err)
	}

	return nil
}

// getEnvOrDefault gets environment variable or returns default value
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// WaitForDatabase waits for the database to be ready
func (dm *DatabaseManager) WaitForDatabase(maxRetries int) error {
	for i := 0; i < maxRetries; i++ {
		if dm.systemDB != nil {
			if err := dm.systemDB.Ping(); err == nil {
				return nil
			}
		}
		time.Sleep(time.Second)
	}
	return fmt.Errorf("database not ready after %d retries", maxRetries)
}
