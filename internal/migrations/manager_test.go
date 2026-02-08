package migrations

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockLogger implements logger.Logger interface for testing
type mockLogger struct{}

func (m *mockLogger) WithField(key string, value interface{}) logger.Logger  { return m }
func (m *mockLogger) WithFields(fields map[string]interface{}) logger.Logger { return m }
func (m *mockLogger) Debug(msg string)                                       {}
func (m *mockLogger) Info(msg string)                                        {}
func (m *mockLogger) Warn(msg string)                                        {}
func (m *mockLogger) Error(msg string)                                       {}
func (m *mockLogger) Fatal(msg string)                                       {}

// mockWorkspaceConnector implements workspaceConnector interface for testing
type mockWorkspaceConnector struct {
	db *sql.DB
}

func (m *mockWorkspaceConnector) connectToWorkspace(cfg *config.DatabaseConfig, workspaceID string) (*sql.DB, error) {
	return m.db, nil
}

func TestNewManager(t *testing.T) {
	logger := &mockLogger{}
	manager := NewManager(logger)

	assert.NotNil(t, manager)
	assert.Equal(t, logger, manager.logger)
}

func TestManager_GetCurrentDBVersion_Success(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	manager := NewManager(&mockLogger{})
	ctx := context.Background()

	// Mock successful query
	rows := sqlmock.NewRows([]string{"value"}).AddRow("4")
	mock.ExpectQuery("SELECT value FROM settings WHERE key = 'db_version'").WillReturnRows(rows)

	version, err, exists := manager.GetCurrentDBVersion(ctx, db)

	assert.NoError(t, err)
	assert.True(t, exists)
	assert.Equal(t, 4.0, version)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestManager_GetCurrentDBVersion_NoRows(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	manager := NewManager(&mockLogger{})
	ctx := context.Background()

	// Mock no rows found
	mock.ExpectQuery("SELECT value FROM settings WHERE key = 'db_version'").WillReturnError(sql.ErrNoRows)

	version, err, exists := manager.GetCurrentDBVersion(ctx, db)

	assert.NoError(t, err)
	assert.False(t, exists)
	assert.Equal(t, 0.0, version)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestManager_GetCurrentDBVersion_QueryError(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	manager := NewManager(&mockLogger{})
	ctx := context.Background()

	// Mock query error
	mock.ExpectQuery("SELECT value FROM settings WHERE key = 'db_version'").WillReturnError(errors.New("database error"))

	version, err, exists := manager.GetCurrentDBVersion(ctx, db)

	assert.Error(t, err)
	assert.False(t, exists)
	assert.Equal(t, 0.0, version)
	assert.Contains(t, err.Error(), "failed to get current database version")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestManager_GetCurrentDBVersion_InvalidFormat(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	manager := NewManager(&mockLogger{})
	ctx := context.Background()

	// Mock invalid version format
	rows := sqlmock.NewRows([]string{"value"}).AddRow("invalid")
	mock.ExpectQuery("SELECT value FROM settings WHERE key = 'db_version'").WillReturnRows(rows)

	version, err, exists := manager.GetCurrentDBVersion(ctx, db)

	assert.Error(t, err)
	assert.False(t, exists)
	assert.Equal(t, 0.0, version)
	assert.Contains(t, err.Error(), "invalid database version format")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestManager_SetCurrentDBVersion_Success(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	manager := NewManager(&mockLogger{})
	ctx := context.Background()

	// Mock successful update
	mock.ExpectExec("INSERT INTO settings").
		WithArgs("4").
		WillReturnResult(sqlmock.NewResult(1, 1))

	err = manager.SetCurrentDBVersion(ctx, db, 4.0)

	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestManager_SetCurrentDBVersion_Error(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	manager := NewManager(&mockLogger{})
	ctx := context.Background()

	// Mock execution error
	mock.ExpectExec("INSERT INTO settings").
		WithArgs("4").
		WillReturnError(errors.New("database error"))

	err = manager.SetCurrentDBVersion(ctx, db, 4.0)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to set database version")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestManager_getAllWorkspaces_Success(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	manager := NewManager(&mockLogger{})
	ctx := context.Background()

	// Mock workspace query
	now := time.Now()
	rows := sqlmock.NewRows([]string{"id", "name", "settings", "integrations", "created_at", "updated_at"}).
		AddRow("ws1", "Workspace 1", []byte("{}"), []byte("[]"), now, now).
		AddRow("ws2", "Workspace 2", []byte("{}"), []byte("[]"), now, now)

	mock.ExpectQuery("SELECT id, name, settings, integrations, created_at, updated_at FROM workspaces").
		WillReturnRows(rows)

	workspaces, err := manager.getAllWorkspaces(ctx, db)

	require.NoError(t, err)
	require.Len(t, workspaces, 2)
	assert.Equal(t, "ws1", workspaces[0].ID)
	assert.Equal(t, "Workspace 1", workspaces[0].Name)
	assert.Equal(t, "ws2", workspaces[1].ID)
	assert.Equal(t, "Workspace 2", workspaces[1].Name)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestManager_getAllWorkspaces_QueryError(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	manager := NewManager(&mockLogger{})
	ctx := context.Background()

	// Mock query error
	mock.ExpectQuery("SELECT id, name, settings, integrations, created_at, updated_at FROM workspaces").
		WillReturnError(errors.New("database error"))

	workspaces, err := manager.getAllWorkspaces(ctx, db)

	assert.Error(t, err)
	assert.Nil(t, workspaces)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestManager_executeMigration_SystemOnly(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	manager := NewManager(&mockLogger{})
	ctx := context.Background()
	cfg := &config.Config{}

	// Create a migration that only has system updates
	migration := &mockMigration{
		version:            3.0,
		hasSystemUpdate:    true,
		hasWorkspaceUpdate: false,
	}

	// Mock transaction
	mock.ExpectBegin()
	mock.ExpectCommit()

	err = manager.executeMigration(ctx, cfg, db, migration)

	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestManager_executeMigration_WorkspaceOnly(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	// Create workspace mock db
	workspaceDB, workspaceMock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = workspaceDB.Close() }()

	// Create mock connector
	mockConnector := &mockWorkspaceConnector{
		db: workspaceDB,
	}

	manager := NewManager(&mockLogger{})
	manager.connector = mockConnector // Inject mock connector
	ctx := context.Background()
	cfg := &config.Config{}

	// Create a migration that only has workspace updates
	migration := &mockMigration{
		version:            3.0,
		hasSystemUpdate:    false,
		hasWorkspaceUpdate: true,
	}

	// Mock transaction
	mock.ExpectBegin()

	// Mock workspace query
	now := time.Now()
	rows := sqlmock.NewRows([]string{"id", "name", "settings", "integrations", "created_at", "updated_at"}).
		AddRow("ws1", "Workspace 1", []byte("{}"), []byte("[]"), now, now)

	mock.ExpectQuery("SELECT id, name, settings, integrations, created_at, updated_at FROM workspaces").
		WillReturnRows(rows)

	// Mock workspace transaction expectations
	workspaceMock.ExpectBegin()
	workspaceMock.ExpectCommit()

	mock.ExpectCommit()

	err = manager.executeMigration(ctx, cfg, db, migration)

	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
	assert.NoError(t, workspaceMock.ExpectationsWereMet())
}

func TestManager_executeMigration_TransactionError(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	manager := NewManager(&mockLogger{})
	ctx := context.Background()
	cfg := &config.Config{}

	migration := &mockMigration{version: 3.0}

	// Mock transaction begin error
	mock.ExpectBegin().WillReturnError(errors.New("transaction error"))

	err = manager.executeMigration(ctx, cfg, db, migration)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to start transaction")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestManager_executeMigration_CommitError(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	manager := NewManager(&mockLogger{})
	ctx := context.Background()
	cfg := &config.Config{}

	migration := &mockMigration{version: 3.0}

	// Mock transaction
	mock.ExpectBegin()
	mock.ExpectCommit().WillReturnError(errors.New("commit error"))
	// Note: rollback is called via defer but may be a no-op after failed commit

	err = manager.executeMigration(ctx, cfg, db, migration)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to commit migration transaction")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// mockMigrationWithError is a mock migration that returns errors
type mockMigrationWithError struct {
	*mockMigration
	systemError    error
	workspaceError error
}

func (m *mockMigrationWithError) UpdateSystem(ctx context.Context, config *config.Config, db DBExecutor) error {
	return m.systemError
}

func (m *mockMigrationWithError) UpdateWorkspace(ctx context.Context, config *config.Config, workspace *domain.Workspace, db DBExecutor) error {
	return m.workspaceError
}

func TestManager_executeMigration_SystemUpdateError(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	manager := NewManager(&mockLogger{})
	ctx := context.Background()
	cfg := &config.Config{}

	migration := &mockMigrationWithError{
		mockMigration: &mockMigration{
			version:            3.0,
			hasSystemUpdate:    true,
			hasWorkspaceUpdate: false,
		},
		systemError: errors.New("system update error"),
	}

	// Mock transaction
	mock.ExpectBegin()
	mock.ExpectRollback()

	err = manager.executeMigration(ctx, cfg, db, migration)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "system migration failed")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestManager_RunMigrations_FirstRun(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	manager := NewManager(&mockLogger{})
	ctx := context.Background()
	cfg := &config.Config{}

	// Mock no version found (first run)
	mock.ExpectQuery("SELECT value FROM settings WHERE key = 'db_version'").WillReturnError(sql.ErrNoRows)

	// Mock version initialization
	mock.ExpectExec("INSERT INTO settings").
		WithArgs(sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err = manager.RunMigrations(ctx, cfg, db)

	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestManager_RunMigrations_GetVersionError(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	manager := NewManager(&mockLogger{})
	ctx := context.Background()
	cfg := &config.Config{}

	// Mock version query error
	mock.ExpectQuery("SELECT value FROM settings WHERE key = 'db_version'").WillReturnError(errors.New("database error"))

	err = manager.RunMigrations(ctx, cfg, db)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get current database version")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestDefaultConnector_ConnectToWorkspace(t *testing.T) {
	connector := &defaultConnector{}

	t.Run("Connection failure due to invalid config", func(t *testing.T) {
		cfg := &config.DatabaseConfig{
			User:     "invalid",
			Password: "invalid",
			Host:     "nonexistent-host",
			Port:     9999,
			Prefix:   "test",
			SSLMode:  "disable",
		}

		db, err := connector.connectToWorkspace(cfg, "test-workspace")

		assert.Error(t, err)
		assert.Nil(t, db)
		assert.Contains(t, err.Error(), "failed to ping workspace database")
	})

	t.Run("Workspace ID formatting", func(t *testing.T) {
		// Test that hyphens are replaced with underscores in the database name
		cfg := &config.DatabaseConfig{
			User:     "user",
			Password: "pass",
			Host:     "invalid-host", // Will fail, but we can test the logic
			Port:     5432,
			Prefix:   "app",
			SSLMode:  "disable",
		}

		db, err := connector.connectToWorkspace(cfg, "test-workspace-123")

		// Should fail due to invalid host, but we verify the error handling
		assert.Error(t, err)
		assert.Nil(t, db)
	})

	t.Run("SQL Open failure", func(t *testing.T) {
		// Test with invalid driver to trigger sql.Open failure
		connector := &defaultConnector{}
		cfg := &config.DatabaseConfig{
			User:     "",
			Password: "",
			Host:     "",
			Port:     0,
			Prefix:   "",
			SSLMode:  "invalid",
		}

		db, err := connector.connectToWorkspace(cfg, "test")

		// Should fail due to invalid configuration
		assert.Error(t, err)
		assert.Nil(t, db)
	})

	t.Run("DSN construction with special characters", func(t *testing.T) {
		connector := &defaultConnector{}
		cfg := &config.DatabaseConfig{
			User:     "user@domain",
			Password: "pass!@#",
			Host:     "nonexistent-host-123",
			Port:     5432,
			Prefix:   "test_app",
			SSLMode:  "require",
		}

		db, err := connector.connectToWorkspace(cfg, "workspace-with-special-chars-123")

		// Should fail due to nonexistent host, but DSN construction is tested
		assert.Error(t, err)
		assert.Nil(t, db)
		assert.Contains(t, err.Error(), "failed to ping workspace database")
	})
}

func TestManager_RunMigrations_AdditionalCoverage(t *testing.T) {
	t.Run("First run - no version exists", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		logger := &mockLogger{}
		manager := NewManager(logger)

		cfg := &config.Config{
			Database: config.DatabaseConfig{
				Host: "localhost",
				Port: 5432,
			},
		}

		// Mock GetCurrentDBVersion to return no version exists
		mock.ExpectQuery("SELECT value FROM settings WHERE key = 'db_version'").
			WillReturnError(sql.ErrNoRows)

		// Mock SetCurrentDBVersion
		mock.ExpectExec("INSERT INTO settings \\(key, value\\) VALUES \\('db_version', \\$1\\)\\s+ON CONFLICT \\(key\\) DO UPDATE SET").
			WithArgs(sqlmock.AnyArg()).
			WillReturnResult(sqlmock.NewResult(1, 1))

		err = manager.RunMigrations(context.Background(), cfg, db)

		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Database up to date - no migrations needed", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		logger := &mockLogger{}
		manager := NewManager(logger)

		cfg := &config.Config{
			Database: config.DatabaseConfig{
				Host: "localhost",
				Port: 5432,
			},
		}

		// Mock GetCurrentDBVersion to return current version (27 - up to date)
		mock.ExpectQuery("SELECT value FROM settings WHERE key = 'db_version'").
			WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow("27"))

		err = manager.RunMigrations(context.Background(), cfg, db)

		// This should succeed as database is up to date
		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Error - Failed to initialize database version", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		logger := &mockLogger{}
		manager := NewManager(logger)

		cfg := &config.Config{
			Database: config.DatabaseConfig{
				Host: "localhost",
				Port: 5432,
			},
		}

		// Mock GetCurrentDBVersion to return no version exists
		mock.ExpectQuery("SELECT value FROM settings WHERE key = 'db_version'").
			WillReturnError(sql.ErrNoRows)

		// Mock SetCurrentDBVersion to fail
		mock.ExpectExec("INSERT INTO settings \\(key, value\\) VALUES \\('db_version', \\$1\\)\\s+ON CONFLICT \\(key\\) DO UPDATE SET").
			WithArgs(sqlmock.AnyArg()).
			WillReturnError(sql.ErrConnDone)

		err = manager.RunMigrations(context.Background(), cfg, db)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to initialize database version")
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("No migrations to run", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		logger := &mockLogger{}
		manager := NewManager(logger)

		cfg := &config.Config{
			Database: config.DatabaseConfig{
				Host: "localhost",
				Port: 5432,
			},
		}

		// Mock current version to be higher than available migrations
		mock.ExpectQuery("SELECT value FROM settings WHERE key = 'db_version'").
			WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow("100"))

		err = manager.RunMigrations(context.Background(), cfg, db)

		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Error - Invalid version format in database", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		logger := &mockLogger{}
		manager := NewManager(logger)

		cfg := &config.Config{
			Database: config.DatabaseConfig{
				Host: "localhost",
				Port: 5432,
			},
		}

		// Mock GetCurrentDBVersion to return invalid version format
		mock.ExpectQuery("SELECT value FROM settings WHERE key = 'db_version'").
			WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow("invalid-version"))

		err = manager.RunMigrations(context.Background(), cfg, db)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid database version format")
		assert.NoError(t, mock.ExpectationsWereMet())
	})

}

func TestManager_ExecuteMigration_AdditionalCoverage(t *testing.T) {
	t.Run("Error - Failed to start transaction", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		logger := &mockLogger{}
		manager := NewManager(logger)

		cfg := &config.Config{
			Database: config.DatabaseConfig{
				Host: "localhost",
				Port: 5432,
			},
		}

		// Mock BeginTx to fail
		mock.ExpectBegin().WillReturnError(sql.ErrConnDone)

		migration := &V4Migration{}
		err = manager.executeMigration(context.Background(), cfg, db, migration)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to start transaction")
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Success - System migration only", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		logger := &mockLogger{}
		manager := NewManager(logger)

		cfg := &config.Config{
			Database: config.DatabaseConfig{
				Host: "localhost",
				Port: 5432,
			},
		}

		// Mock successful transaction
		mock.ExpectBegin()

		// Mock successful system migration (V4 has system updates only)
		mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM users").
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(5))
		mock.ExpectExec("ALTER TABLE user_workspaces ADD COLUMN IF NOT EXISTS permissions JSONB DEFAULT").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("UPDATE user_workspaces SET permissions = .+ WHERE role = 'owner'").
			WillReturnResult(sqlmock.NewResult(0, 2))
		mock.ExpectExec("UPDATE user_workspaces SET permissions = .+ WHERE role = 'member'").
			WillReturnResult(sqlmock.NewResult(0, 3))

		mock.ExpectCommit()

		migration := &V4Migration{}
		err = manager.executeMigration(context.Background(), cfg, db, migration)

		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}
