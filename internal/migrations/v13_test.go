package migrations

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestV13Migration_HasSystemUpdate(t *testing.T) {
	// Test V13Migration.HasSystemUpdate - this was at 0% coverage
	migration := &V13Migration{}
	assert.True(t, migration.HasSystemUpdate(), "V13Migration should have system updates")
}

func TestV13Migration_HasWorkspaceUpdate(t *testing.T) {
	// Test V13Migration.HasWorkspaceUpdate - this was at 0% coverage
	migration := &V13Migration{}
	assert.True(t, migration.HasWorkspaceUpdate(), "V13Migration should have workspace updates")
}

func TestV13Migration_ShouldRestartServer(t *testing.T) {
	// Test V13Migration.ShouldRestartServer - this was at 0% coverage
	migration := &V13Migration{}
	assert.False(t, migration.ShouldRestartServer(), "V13Migration should not require server restart")
}

func TestV13Migration_UpdateSystem(t *testing.T) {
	// Test V13Migration.UpdateSystem - this was at 0% coverage
	migration := &V13Migration{}
	ctx := context.Background()
	cfg := &config.Config{}

	t.Run("Success - No workspaces", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		// Mock empty workspace query
		mock.ExpectQuery("SELECT id FROM workspaces").
			WillReturnRows(sqlmock.NewRows([]string{"id"}))

		err = migration.UpdateSystem(ctx, cfg, db)
		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Success - With workspaces", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		// Mock workspace query
		mock.ExpectQuery("SELECT id FROM workspaces").
			WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("workspace1"))

		// Mock task existence check
		mock.ExpectQuery("SELECT EXISTS").
			WithArgs("workspace1").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))

		// Mock task creation
		mock.ExpectExec("INSERT INTO tasks").
			WithArgs("workspace1").
			WillReturnResult(sqlmock.NewResult(1, 1))

		err = migration.UpdateSystem(ctx, cfg, db)
		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestV13Migration_UpdateWorkspace(t *testing.T) {
	// Test V13Migration.UpdateWorkspace - this was at 0% coverage
	migration := &V13Migration{}
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
		mock.ExpectExec("ALTER TABLE segments ADD COLUMN IF NOT EXISTS recompute_after").
			WillReturnResult(sqlmock.NewResult(0, 0))

		err = migration.UpdateWorkspace(ctx, cfg, workspace, db)
		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}
