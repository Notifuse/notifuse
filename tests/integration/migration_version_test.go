package integration

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/migrations"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/Notifuse/notifuse/tests/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// skipIfNotIntegrationTest skips the test if INTEGRATION_TESTS is not set
func skipIfNotIntegrationTest(t *testing.T) {
	if os.Getenv("INTEGRATION_TESTS") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TESTS=true to run.")
	}
}

func TestMigrationVersionUpgrade(t *testing.T) {
	skipIfNotIntegrationTest(t)

	t.Run("should_run_migrations_when_database_version_is_lower_than_code_version", func(t *testing.T) {
		// Set SECRET_KEY for v15 migration
		os.Setenv("SECRET_KEY", "test-secret-key-for-migration")
		defer os.Unsetenv("SECRET_KEY")

		// Create a fresh test database
		dbManager := testutil.NewDatabaseManager()
		defer dbManager.Cleanup()

		err := dbManager.Setup()
		require.NoError(t, err)

		db := dbManager.GetDB()
		testConfig := &config.Config{
			Database: *dbManager.GetConfig(),
			Version:  "5.0", // Current code version
			LogLevel: "info",
		}
		testLogger := logger.NewLoggerWithLevel("info")

		// Step 1: Get the current database version (should be 5 after setup)
		var initialVersion string
		err = db.QueryRow("SELECT value FROM settings WHERE key = 'db_version'").Scan(&initialVersion)
		require.NoError(t, err)

		currentCodeVersion, err := migrations.GetCurrentCodeVersion()
		require.NoError(t, err)
		expectedVersion := fmt.Sprintf("%.0f", currentCodeVersion)
		assert.Equal(t, expectedVersion, initialVersion, "Database should be at current code version after setup")

		// Step 2: Manually set database version to a lower version (simulate old database)
		lowerVersion := "3" // Simulate database that's at version 3
		_, err = db.Exec(`
			UPDATE settings SET value = $1, updated_at = CURRENT_TIMESTAMP 
			WHERE key = 'db_version'
		`, lowerVersion)
		require.NoError(t, err)

		// Step 3: Verify the database version is set to the lower version
		var dbVersion string
		err = db.QueryRow("SELECT value FROM settings WHERE key = 'db_version'").Scan(&dbVersion)
		require.NoError(t, err)
		assert.Equal(t, lowerVersion, dbVersion, "Database should be at lower version before migration")

		// Step 4: Run migrations - this should upgrade from version 3 to 5
		migrationManager := migrations.NewManager(testLogger)
		ctx := context.Background()

		// Capture the current code version to ensure it's higher than database version
		assert.Greater(t, currentCodeVersion, 3.0, "Code version should be higher than database version")

		// Run migrations
		err = migrationManager.RunMigrations(ctx, testConfig, db)
		// Migration may return ErrRestartRequired if any migration requires a restart, which is not an error
		if err != nil && err != migrations.ErrRestartRequired {
			require.NoError(t, err, "Migration should succeed when database version is lower than code version")
		}

		// Step 5: Verify the database version was updated to current code version
		var updatedDbVersion string
		err = db.QueryRow("SELECT value FROM settings WHERE key = 'db_version'").Scan(&updatedDbVersion)
		require.NoError(t, err)

		assert.Equal(t, expectedVersion, updatedDbVersion, "Database version should be updated to current code version")

		// Step 6: Verify that the migration system detected the need to run migrations
		// This test primarily verifies the version comparison and upgrade logic works correctly
		// The actual schema changes are tested elsewhere since workspace databases already have current schema
	})

	t.Run("should_not_run_migrations_when_database_version_equals_code_version", func(t *testing.T) {
		// Set SECRET_KEY for v15 migration (in case it runs)
		os.Setenv("SECRET_KEY", "test-secret-key-for-migration")
		defer os.Unsetenv("SECRET_KEY")

		// Create a fresh test database
		dbManager := testutil.NewDatabaseManager()
		defer dbManager.Cleanup()

		err := dbManager.Setup()
		require.NoError(t, err)

		db := dbManager.GetDB()
		testConfig := &config.Config{
			Database: *dbManager.GetConfig(),
			Version:  "5.0",
			LogLevel: "info",
		}
		testLogger := logger.NewLoggerWithLevel("info")

		// Step 1: Database is already initialized by dbManager.Setup()

		// Step 2: Get current code version and set database to same version
		currentCodeVersion, err := migrations.GetCurrentCodeVersion()
		require.NoError(t, err)

		currentVersionStr := fmt.Sprintf("%.0f", currentCodeVersion)
		_, err = db.Exec(`
			INSERT INTO settings (key, value, created_at, updated_at) 
			VALUES ('db_version', $1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
			ON CONFLICT (key) DO UPDATE SET 
				value = $1, 
				updated_at = CURRENT_TIMESTAMP
		`, currentVersionStr)
		require.NoError(t, err)

		// Step 3: Run migrations - should be no-op
		migrationManager := migrations.NewManager(testLogger)
		ctx := context.Background()

		// Run migrations
		err = migrationManager.RunMigrations(ctx, testConfig, db)
		// Migration may return ErrRestartRequired if any migration requires a restart, which is not an error
		if err != nil && err != migrations.ErrRestartRequired {
			require.NoError(t, err, "Migration should succeed even when no migrations need to run")
		}

		// Step 4: Verify the database version remains the same
		var afterVersion string
		err = db.QueryRow("SELECT value FROM settings WHERE key = 'db_version'").Scan(&afterVersion)
		require.NoError(t, err)

		assert.Equal(t, currentVersionStr, afterVersion, "Database version should remain unchanged")
	})

	t.Run("should_handle_missing_database_version_as_first_run", func(t *testing.T) {
		// Set SECRET_KEY for v15 migration (in case it runs)
		os.Setenv("SECRET_KEY", "test-secret-key-for-migration")
		defer os.Unsetenv("SECRET_KEY")

		// Create a fresh test database
		dbManager := testutil.NewDatabaseManager()
		defer dbManager.Cleanup()

		err := dbManager.Setup()
		require.NoError(t, err)

		db := dbManager.GetDB()
		testConfig := &config.Config{
			Database: *dbManager.GetConfig(),
			Version:  "5.0",
			LogLevel: "info",
		}
		testLogger := logger.NewLoggerWithLevel("info")

		// Step 1: Database is already initialized by dbManager.Setup()

		// Step 2: Remove the db_version setting to simulate first run
		_, err = db.Exec("DELETE FROM settings WHERE key = 'db_version'")
		require.NoError(t, err)

		// Step 3: Verify no version exists
		var count int
		err = db.QueryRow("SELECT COUNT(*) FROM settings WHERE key = 'db_version'").Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 0, count, "No db_version should exist before migration")

		// Step 4: Run migrations - should initialize version to current code version
		migrationManager := migrations.NewManager(testLogger)
		ctx := context.Background()

		err = migrationManager.RunMigrations(ctx, testConfig, db)
		// Migration may return ErrRestartRequired if any migration requires a restart, which is not an error
		if err != nil && err != migrations.ErrRestartRequired {
			require.NoError(t, err, "Migration should succeed on first run")
		}

		// Step 5: Verify the database version was initialized to current code version
		currentCodeVersion, err := migrations.GetCurrentCodeVersion()
		require.NoError(t, err)

		var dbVersion string
		err = db.QueryRow("SELECT value FROM settings WHERE key = 'db_version'").Scan(&dbVersion)
		require.NoError(t, err)

		expectedVersion := fmt.Sprintf("%.0f", currentCodeVersion)
		assert.Equal(t, expectedVersion, dbVersion, "Database version should be initialized to current code version")
	})

}
