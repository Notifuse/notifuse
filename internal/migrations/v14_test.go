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

func TestV14Migration_ShouldRestartServer(t *testing.T) {
	// Test V14Migration.ShouldRestartServer - this was at 0% coverage
	migration := &V14Migration{}
	assert.True(t, migration.ShouldRestartServer(), "V14Migration should require server restart")
}

func TestV14Migration_UpdateSystem(t *testing.T) {
	// Test V14Migration.UpdateSystem - this was at 0% coverage
	migration := &V14Migration{}
	ctx := context.Background()
	cfg := &config.Config{}

	t.Run("Success - System not installed", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		// Mock is_installed check returning no rows
		mock.ExpectQuery("SELECT value FROM settings WHERE key = 'is_installed'").
			WillReturnError(sql.ErrNoRows)

		err = migration.UpdateSystem(ctx, cfg, db)
		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Success - System installed, settings exist", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		// Mock is_installed check returning true
		mock.ExpectQuery("SELECT value FROM settings WHERE key = 'is_installed'").
			WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow("true"))

		// Mock telemetry_enabled check returning existing value
		mock.ExpectQuery("SELECT value FROM settings WHERE key = 'telemetry_enabled'").
			WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow("true"))

		// Mock check_for_updates check returning existing value
		mock.ExpectQuery("SELECT value FROM settings WHERE key = 'check_for_updates'").
			WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow("true"))

		err = migration.UpdateSystem(ctx, cfg, db)
		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestV14Migration_UpdateWorkspace(t *testing.T) {
	// Test V14Migration.UpdateWorkspace - this was at 0% coverage
	migration := &V14Migration{}
	ctx := context.Background()
	cfg := &config.Config{}
	workspace := &domain.Workspace{
		ID:   "test-workspace",
		Name: "Test Workspace",
	}

	t.Run("Success - Basic execution", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		// Mock ALTER TABLE
		mock.ExpectExec("ALTER TABLE message_history ADD COLUMN IF NOT EXISTS channel_options JSONB").
			WillReturnResult(sqlmock.NewResult(0, 0))

		err = migration.UpdateWorkspace(ctx, cfg, workspace, db)
		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}
