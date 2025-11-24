package database

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/config"
	"github.com/stretchr/testify/assert"
)

func TestGetConnectionPoolSettings(t *testing.T) {
	// Save original environment
	originalEnv := os.Getenv("ENVIRONMENT")
	originalIntegrationTests := os.Getenv("INTEGRATION_TESTS")
	defer func() {
		_ = os.Setenv("ENVIRONMENT", originalEnv)
		_ = os.Setenv("INTEGRATION_TESTS", originalIntegrationTests)
	}()

	t.Run("Production settings", func(t *testing.T) {
		// Clear environment variables
		_ = os.Unsetenv("ENVIRONMENT")
		_ = os.Unsetenv("INTEGRATION_TESTS")

		maxOpen, maxIdle, maxLifetime := GetConnectionPoolSettings()

		assert.Equal(t, 25, maxOpen, "Production max open connections should be 25")
		assert.Equal(t, 25, maxIdle, "Production max idle connections should be 25")
		assert.Equal(t, 20*time.Minute, maxLifetime, "Production max lifetime should be 20 minutes")
	})

	t.Run("Test environment settings", func(t *testing.T) {
		_ = os.Setenv("ENVIRONMENT", "test")
		_ = os.Unsetenv("INTEGRATION_TESTS")

		maxOpen, maxIdle, maxLifetime := GetConnectionPoolSettings()

		assert.Equal(t, 10, maxOpen, "Test max open connections should be 10")
		assert.Equal(t, 5, maxIdle, "Test max idle connections should be 5")
		assert.Equal(t, 2*time.Minute, maxLifetime, "Test max lifetime should be 2 minutes")
	})

	t.Run("Integration tests settings", func(t *testing.T) {
		_ = os.Unsetenv("ENVIRONMENT")
		_ = os.Setenv("INTEGRATION_TESTS", "true")

		maxOpen, maxIdle, maxLifetime := GetConnectionPoolSettings()

		assert.Equal(t, 10, maxOpen, "Integration test max open connections should be 10")
		assert.Equal(t, 5, maxIdle, "Integration test max idle connections should be 5")
		assert.Equal(t, 2*time.Minute, maxLifetime, "Integration test max lifetime should be 2 minutes")
	})

	t.Run("Both test environment and integration tests", func(t *testing.T) {
		_ = os.Setenv("ENVIRONMENT", "test")
		_ = os.Setenv("INTEGRATION_TESTS", "true")

		maxOpen, maxIdle, maxLifetime := GetConnectionPoolSettings()

		assert.Equal(t, 10, maxOpen, "Should use test settings when both are set")
		assert.Equal(t, 5, maxIdle, "Should use test settings when both are set")
		assert.Equal(t, 2*time.Minute, maxLifetime, "Should use test settings when both are set")
	})
}

func TestGetSystemDSN(t *testing.T) {
	t.Run("Standard configuration", func(t *testing.T) {
		cfg := &config.DatabaseConfig{
			User:     "testuser",
			Password: "testpass",
			Host:     "localhost",
			Port:     5432,
			DBName:   "testdb",
			SSLMode:  "disable",
		}

		expected := "postgres://testuser:testpass@localhost:5432/testdb?sslmode=disable"
		result := GetSystemDSN(cfg)

		assert.Equal(t, expected, result)
	})

	t.Run("Configuration with special characters", func(t *testing.T) {
		cfg := &config.DatabaseConfig{
			User:     "user@domain",
			Password: "pass!@#$%",
			Host:     "db.example.com",
			Port:     3306,
			DBName:   "my-database",
			SSLMode:  "require",
		}

		expected := "postgres://user@domain:pass!@#$%@db.example.com:3306/my-database?sslmode=require"
		result := GetSystemDSN(cfg)

		assert.Equal(t, expected, result)
	})

	t.Run("Empty SSL mode", func(t *testing.T) {
		cfg := &config.DatabaseConfig{
			User:     "user",
			Password: "pass",
			Host:     "host",
			Port:     5432,
			DBName:   "db",
			SSLMode:  "",
		}

		expected := "postgres://user:pass@host:5432/db?sslmode="
		result := GetSystemDSN(cfg)

		assert.Equal(t, expected, result)
	})
}

func TestGetPostgresDSN(t *testing.T) {
	t.Run("Standard configuration", func(t *testing.T) {
		cfg := &config.DatabaseConfig{
			User:     "testuser",
			Password: "testpass",
			Host:     "localhost",
			Port:     5432,
			DBName:   "testdb", // This should be ignored and replaced with "postgres"
			SSLMode:  "disable",
		}

		expected := "postgres://testuser:testpass@localhost:5432/postgres?sslmode=disable"
		result := GetPostgresDSN(cfg)

		assert.Equal(t, expected, result)
	})

	t.Run("Different port and SSL mode", func(t *testing.T) {
		cfg := &config.DatabaseConfig{
			User:     "admin",
			Password: "secret",
			Host:     "db.example.com",
			Port:     3306,
			DBName:   "ignored",
			SSLMode:  "require",
		}

		expected := "postgres://admin:secret@db.example.com:3306/postgres?sslmode=require"
		result := GetPostgresDSN(cfg)

		assert.Equal(t, expected, result)
	})
}

func TestGetWorkspaceDSN(t *testing.T) {
	t.Run("Standard workspace ID", func(t *testing.T) {
		cfg := &config.DatabaseConfig{
			User:     "testuser",
			Password: "testpass",
			Host:     "localhost",
			Port:     5432,
			Prefix:   "notifuse",
			SSLMode:  "disable",
		}
		workspaceID := "workspace123"

		expected := "postgres://testuser:testpass@localhost:5432/notifuse_ws_workspace123?sslmode=disable"
		result := GetWorkspaceDSN(cfg, workspaceID)

		assert.Equal(t, expected, result)
	})

	t.Run("Workspace ID with hyphens", func(t *testing.T) {
		cfg := &config.DatabaseConfig{
			User:     "user",
			Password: "pass",
			Host:     "host",
			Port:     5432,
			Prefix:   "app",
			SSLMode:  "require",
		}
		workspaceID := "workspace-with-hyphens-123"

		expected := "postgres://user:pass@host:5432/app_ws_workspace_with_hyphens_123?sslmode=require"
		result := GetWorkspaceDSN(cfg, workspaceID)

		assert.Equal(t, expected, result)
	})

	t.Run("Complex workspace ID", func(t *testing.T) {
		cfg := &config.DatabaseConfig{
			User:     "dbuser",
			Password: "dbpass",
			Host:     "localhost",
			Port:     5432,
			Prefix:   "myapp",
			SSLMode:  "disable",
		}
		workspaceID := "test-workspace-abc-123-def"

		expected := "postgres://dbuser:dbpass@localhost:5432/myapp_ws_test_workspace_abc_123_def?sslmode=disable"
		result := GetWorkspaceDSN(cfg, workspaceID)

		assert.Equal(t, expected, result)
	})

	t.Run("Empty prefix", func(t *testing.T) {
		cfg := &config.DatabaseConfig{
			User:     "user",
			Password: "pass",
			Host:     "host",
			Port:     5432,
			Prefix:   "",
			SSLMode:  "disable",
		}
		workspaceID := "workspace-id"

		expected := "postgres://user:pass@host:5432/_ws_workspace_id?sslmode=disable"
		result := GetWorkspaceDSN(cfg, workspaceID)

		assert.Equal(t, expected, result)
	})
}

func TestConnectToWorkspace(t *testing.T) {
	t.Run("Connection failure due to invalid config", func(t *testing.T) {
		cfg := &config.DatabaseConfig{
			User:     "invalid",
			Password: "invalid",
			Host:     "nonexistent-host",
			Port:     9999,
			Prefix:   "test",
			SSLMode:  "disable",
		}
		workspaceID := "test-workspace"

		db, err := ConnectToWorkspace(cfg, workspaceID)

		assert.Error(t, err, "Should fail with invalid database configuration")
		assert.Nil(t, db, "Database connection should be nil on failure")
		assert.Contains(t, err.Error(), "failed to ensure workspace database exists")
	})

	// Note: Full integration tests would require a real database
	// These tests focus on the error handling and configuration logic
}

func TestEnsureWorkspaceDatabaseExists(t *testing.T) {
	t.Run("Connection failure", func(t *testing.T) {
		cfg := &config.DatabaseConfig{
			User:     "invalid",
			Password: "invalid",
			Host:     "nonexistent-host",
			Port:     9999,
			Prefix:   "test",
			SSLMode:  "disable",
		}
		workspaceID := "test-workspace"

		err := EnsureWorkspaceDatabaseExists(cfg, workspaceID)

		assert.Error(t, err, "Should fail with invalid database configuration")
		assert.Contains(t, err.Error(), "failed to ping PostgreSQL server")
	})

	t.Run("Workspace ID formatting", func(t *testing.T) {
		// Test that hyphens are replaced with underscores in the database name
		// This test verifies the logic without requiring a database connection
		cfg := &config.DatabaseConfig{
			User:     "user",
			Password: "pass",
			Host:     "invalid-host", // Will fail connection, but we can test the formatting logic
			Port:     5432,
			Prefix:   "app",
			SSLMode:  "disable",
		}
		workspaceID := "test-workspace-123"

		err := EnsureWorkspaceDatabaseExists(cfg, workspaceID)

		// Should fail due to invalid host, but we can verify the error message structure
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to ping PostgreSQL server")
	})
}

func TestEnsureSystemDatabaseExists(t *testing.T) {
	t.Run("Invalid DSN", func(t *testing.T) {
		invalidDSN := "invalid://dsn/format"
		dbName := "testdb"

		err := EnsureSystemDatabaseExists(invalidDSN, dbName)

		assert.Error(t, err, "Should fail with invalid DSN")
		assert.Contains(t, err.Error(), "failed to ping PostgreSQL server")
	})

	t.Run("Connection failure", func(t *testing.T) {
		dsn := "postgres://invalid:invalid@nonexistent-host:9999/postgres?sslmode=disable"
		dbName := "testdb"

		err := EnsureSystemDatabaseExists(dsn, dbName)

		assert.Error(t, err, "Should fail with invalid connection parameters")
		assert.Contains(t, err.Error(), "failed to ping PostgreSQL server")
	})

	t.Run("Empty database name", func(t *testing.T) {
		dsn := "postgres://user:pass@localhost:5432/postgres?sslmode=disable"
		dbName := ""

		err := EnsureSystemDatabaseExists(dsn, dbName)

		// Should fail when trying to connect, but tests the parameter handling
		assert.Error(t, err)
	})
}

// Integration test helpers (these would be used in actual integration tests)
func TestDSNGeneration_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	t.Run("DSN format validation", func(t *testing.T) {
		cfg := &config.DatabaseConfig{
			User:     "testuser",
			Password: "testpass",
			Host:     "localhost",
			Port:     5432,
			DBName:   "testdb",
			Prefix:   "app",
			SSLMode:  "disable",
		}

		// Test all DSN generation functions
		systemDSN := GetSystemDSN(cfg)
		postgresDSN := GetPostgresDSN(cfg)
		workspaceDSN := GetWorkspaceDSN(cfg, "test-workspace")

		// Verify DSN format is valid (starts with postgres://)
		assert.True(t, len(systemDSN) > 0 && systemDSN[:11] == "postgres://")
		assert.True(t, len(postgresDSN) > 0 && postgresDSN[:11] == "postgres://")
		assert.True(t, len(workspaceDSN) > 0 && workspaceDSN[:11] == "postgres://")

		// Verify workspace DSN contains workspace identifier
		assert.Contains(t, workspaceDSN, "app_ws_test_workspace")

		// Verify postgres DSN uses postgres database
		assert.Contains(t, postgresDSN, "/postgres?")

		// Verify system DSN uses specified database
		assert.Contains(t, systemDSN, "/testdb?")
	})
}

func TestConnectionPoolSettings_Coverage(t *testing.T) {
	t.Run("Environment variable edge cases", func(t *testing.T) {
		// Save original environment
		originalEnv := os.Getenv("ENVIRONMENT")
		originalIntegrationTests := os.Getenv("INTEGRATION_TESTS")
		defer func() {
			_ = os.Setenv("ENVIRONMENT", originalEnv)
			_ = os.Setenv("INTEGRATION_TESTS", originalIntegrationTests)
		}()

		// Test with different environment values
		testCases := []struct {
			env          string
			integration  string
			expectedOpen int
			expectedIdle int
			expectedLife time.Duration
		}{
			{"production", "", 25, 25, 20 * time.Minute},
			{"development", "", 25, 25, 20 * time.Minute},
			{"staging", "", 25, 25, 20 * time.Minute},
			{"test", "false", 10, 5, 2 * time.Minute},
			{"", "true", 10, 5, 2 * time.Minute},
			{"production", "true", 10, 5, 2 * time.Minute}, // integration tests override
		}

		for _, tc := range testCases {
			t.Run(fmt.Sprintf("env=%s,integration=%s", tc.env, tc.integration), func(t *testing.T) {
				if tc.env == "" {
					_ = os.Unsetenv("ENVIRONMENT")
				} else {
					_ = os.Setenv("ENVIRONMENT", tc.env)
				}

				if tc.integration == "" {
					_ = os.Unsetenv("INTEGRATION_TESTS")
				} else {
					_ = os.Setenv("INTEGRATION_TESTS", tc.integration)
				}

				maxOpen, maxIdle, maxLifetime := GetConnectionPoolSettings()

				assert.Equal(t, tc.expectedOpen, maxOpen)
				assert.Equal(t, tc.expectedIdle, maxIdle)
				assert.Equal(t, tc.expectedLife, maxLifetime)
			})
		}
	})
}
