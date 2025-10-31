package migrations

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strconv"
	"testing"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestV14Migration_GetMajorVersion(t *testing.T) {
	migration := &V14Migration{}
	assert.Equal(t, 14.0, migration.GetMajorVersion())
}

func TestV14Migration_HasSystemUpdate(t *testing.T) {
	migration := &V14Migration{}
	assert.True(t, migration.HasSystemUpdate())
}

func TestV14Migration_HasWorkspaceUpdate(t *testing.T) {
	migration := &V14Migration{}
	assert.True(t, migration.HasWorkspaceUpdate())
}

func TestV14Migration_UpdateSystem(t *testing.T) {
	t.Skip("Skipping integration test - requires test database")
	
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	migration := &V14Migration{}
	ctx := context.Background()
	cfg := createTestConfig()
	db := setupTestDB(t, cfg)
	defer db.Close()

	// Create settings table if it doesn't exist
	_, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS settings (
			key VARCHAR(255) PRIMARY KEY,
			value TEXT NOT NULL,
			created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
		)
	`)
	require.NoError(t, err)

	// Set is_installed to true to simulate an existing installation
	_, err = db.ExecContext(ctx,
		"INSERT INTO settings (key, value) VALUES ($1, $2) ON CONFLICT (key) DO UPDATE SET value = $2",
		"is_installed", "true")
	require.NoError(t, err)

	// Run migration
	err = migration.UpdateSystem(ctx, cfg, db)
	assert.NoError(t, err)

	// Verify telemetry_enabled was set
	var telemetryValue string
	err = db.QueryRowContext(ctx, "SELECT value FROM settings WHERE key = 'telemetry_enabled'").Scan(&telemetryValue)
	assert.NoError(t, err)
	assert.NotEmpty(t, telemetryValue)

	// Verify check_for_updates was set
	var checkUpdatesValue string
	err = db.QueryRowContext(ctx, "SELECT value FROM settings WHERE key = 'check_for_updates'").Scan(&checkUpdatesValue)
	assert.NoError(t, err)
	assert.NotEmpty(t, checkUpdatesValue)

	// Test idempotency - running migration again should not error
	err = migration.UpdateSystem(ctx, cfg, db)
	assert.NoError(t, err, "Migration should be idempotent")
}

func TestV14Migration_UpdateWorkspace(t *testing.T) {
	t.Skip("Skipping integration test - requires test database")
	
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Setup test database
	ctx := context.Background()
	cfg := createTestConfig()
	db := setupTestDB(t, cfg)
	defer db.Close()

	workspace := &domain.Workspace{
		ID:   "test-workspace",
		Name: "Test Workspace",
	}

	// Create test workspace database
	createTestWorkspaceDB(t, cfg, workspace.ID)
	defer dropTestWorkspaceDB(t, cfg, workspace.ID)

	// Connect to workspace database
	workspaceDB := connectToWorkspaceDB(t, cfg, workspace.ID)
	defer workspaceDB.Close()

	// Create message_history table without channel_options column (simulating old schema)
	_, err := workspaceDB.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS message_history (
			id VARCHAR(255) NOT NULL PRIMARY KEY,
			contact_email VARCHAR(255) NOT NULL,
			external_id VARCHAR(255),
			broadcast_id VARCHAR(255),
			list_ids TEXT[],
			template_id VARCHAR(32) NOT NULL,
			template_version INTEGER NOT NULL,
			channel VARCHAR(20) NOT NULL,
			status_info VARCHAR(255),
			message_data JSONB NOT NULL,
			attachments JSONB,
			sent_at TIMESTAMP WITH TIME ZONE NOT NULL,
			delivered_at TIMESTAMP WITH TIME ZONE,
			failed_at TIMESTAMP WITH TIME ZONE,
			opened_at TIMESTAMP WITH TIME ZONE,
			clicked_at TIMESTAMP WITH TIME ZONE,
			bounced_at TIMESTAMP WITH TIME ZONE,
			complained_at TIMESTAMP WITH TIME ZONE,
			unsubscribed_at TIMESTAMP WITH TIME ZONE,
			created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
		)
	`)
	require.NoError(t, err)

	// Run the migration
	migration := &V14Migration{}
	err = migration.UpdateWorkspace(ctx, cfg, workspace, workspaceDB)
	require.NoError(t, err)

	// Verify the column was added
	var columnExists bool
	err = workspaceDB.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM information_schema.columns
			WHERE table_name = 'message_history'
			AND column_name = 'channel_options'
		)
	`).Scan(&columnExists)
	require.NoError(t, err)
	assert.True(t, columnExists, "channel_options column should exist after migration")

	// Test idempotency - running migration again should not error
	err = migration.UpdateWorkspace(ctx, cfg, workspace, workspaceDB)
	assert.NoError(t, err, "Migration should be idempotent")
}

// Helper functions for test database setup

func setupTestDB(t *testing.T, cfg *config.Config) *sql.DB {
	dsn := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=postgres sslmode=disable",
		cfg.Database.Host,
		cfg.Database.Port,
		cfg.Database.User,
		cfg.Database.Password,
	)

	db, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	require.NoError(t, db.Ping())

	return db
}

func createTestWorkspaceDB(t *testing.T, cfg *config.Config, workspaceID string) {
	db := setupTestDB(t, cfg)
	defer db.Close()

	dbName := cfg.Database.Prefix + "_" + workspaceID

	// Drop if exists (cleanup from previous failed tests)
	_, _ = db.Exec(fmt.Sprintf("DROP DATABASE IF EXISTS %s", dbName))

	// Create database
	_, err := db.Exec(fmt.Sprintf("CREATE DATABASE %s", dbName))
	require.NoError(t, err)
}

func dropTestWorkspaceDB(t *testing.T, cfg *config.Config, workspaceID string) {
	db := setupTestDB(t, cfg)
	defer db.Close()

	dbName := cfg.Database.Prefix + "_" + workspaceID
	_, _ = db.Exec(fmt.Sprintf("DROP DATABASE IF EXISTS %s", dbName))
}

func connectToWorkspaceDB(t *testing.T, cfg *config.Config, workspaceID string) *sql.DB {
	dbName := cfg.Database.Prefix + "_" + workspaceID
	dsn := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		cfg.Database.Host,
		cfg.Database.Port,
		cfg.Database.User,
		cfg.Database.Password,
		dbName,
	)

	db, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	require.NoError(t, db.Ping())

	return db
}

func createTestConfig() *config.Config {
	return &config.Config{
		Database: config.DatabaseConfig{
			Host:     getEnv("TEST_DB_HOST", "localhost"),
			Port:     getEnvInt("TEST_DB_PORT", 5433),
			User:     getEnv("TEST_DB_USER", "notifuse_test"),
			Password: getEnv("TEST_DB_PASSWORD", "test_password"),
			Prefix:   "notifuse_test",
		},
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}
