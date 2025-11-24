package migrations

import (
	"context"
	"database/sql"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestV11Migration_HasSystemUpdate(t *testing.T) {
	// Test V11Migration.HasSystemUpdate - this was at 0% coverage
	migration := &V11Migration{}
	assert.True(t, migration.HasSystemUpdate(), "V11Migration should have system updates")
}

func TestV11Migration_HasWorkspaceUpdate(t *testing.T) {
	// Test V11Migration.HasWorkspaceUpdate - this was at 0% coverage
	migration := &V11Migration{}
	assert.False(t, migration.HasWorkspaceUpdate(), "V11Migration should not have workspace updates")
}

func TestV11Migration_ShouldRestartServer(t *testing.T) {
	// Test V11Migration.ShouldRestartServer - this was at 0% coverage
	migration := &V11Migration{}
	assert.False(t, migration.ShouldRestartServer(), "V11Migration should not require server restart")
}

func TestV11Migration_UpdateSystem(t *testing.T) {
	// Test V11Migration.UpdateSystem - this was at 0% coverage
	migration := &V11Migration{}
	ctx := context.Background()
	cfg := &config.Config{
		APIEndpoint: "https://api.example.com",
		Security: config.SecurityConfig{
			JWTSecret: []byte("test-jwt-secret"),
			SecretKey: "test-secret-key",
		},
		SMTP: config.SMTPConfig{
			Host:      "smtp.example.com",
			FromEmail: "test@example.com",
		},
	}

	t.Run("Success - Already installed", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		// Mock is_installed check returning true
		mock.ExpectQuery("SELECT value FROM settings WHERE key = 'is_installed'").
			WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow("true"))

		err = migration.UpdateSystem(ctx, cfg, db)
		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Success - No users (fresh install)", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		// Mock is_installed check returning no rows
		mock.ExpectQuery("SELECT value FROM settings WHERE key = 'is_installed'").
			WillReturnError(sql.ErrNoRows)

		// Mock user count query returning 0
		mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM users").
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

		err = migration.UpdateSystem(ctx, cfg, db)
		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Success - Existing installation with required settings", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		// Mock is_installed check returning no rows
		mock.ExpectQuery("SELECT value FROM settings WHERE key = 'is_installed'").
			WillReturnError(sql.ErrNoRows)

		// Mock user count query returning > 0
		mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM users").
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(5))

		// Mock INSERT/UPDATE for is_installed
		mock.ExpectExec("INSERT INTO settings").
			WillReturnResult(sqlmock.NewResult(1, 1))

		err = migration.UpdateSystem(ctx, cfg, db)
		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestV11Migration_UpdateWorkspace(t *testing.T) {
	// Test V11Migration.UpdateWorkspace - this was at 0% coverage
	migration := &V11Migration{}
	ctx := context.Background()
	cfg := &config.Config{}
	workspace := &domain.Workspace{
		ID:   "test-workspace",
		Name: "Test Workspace",
	}

	t.Run("Success - No workspace updates", func(t *testing.T) {
		db, _, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		err = migration.UpdateWorkspace(ctx, cfg, workspace, db)
		assert.NoError(t, err, "UpdateWorkspace should succeed with no operations")
	})
}
