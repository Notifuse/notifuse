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

func TestV9Migration_HasSystemUpdate(t *testing.T) {
	// Test V9Migration.HasSystemUpdate - this was at 0% coverage
	migration := &V9Migration{}
	assert.False(t, migration.HasSystemUpdate(), "V9Migration should not have system updates")
}

func TestV9Migration_HasWorkspaceUpdate(t *testing.T) {
	// Test V9Migration.HasWorkspaceUpdate - this was at 0% coverage
	migration := &V9Migration{}
	assert.True(t, migration.HasWorkspaceUpdate(), "V9Migration should have workspace updates")
}

func TestV9Migration_ShouldRestartServer(t *testing.T) {
	// Test V9Migration.ShouldRestartServer - this was at 0% coverage
	migration := &V9Migration{}
	assert.False(t, migration.ShouldRestartServer(), "V9Migration should not require server restart")
}

func TestV9Migration_UpdateSystem(t *testing.T) {
	// Test V9Migration.UpdateSystem - this was at 0% coverage
	migration := &V9Migration{}
	ctx := context.Background()
	cfg := &config.Config{}

	t.Run("Success - No system updates", func(t *testing.T) {
		db, _, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		err = migration.UpdateSystem(ctx, cfg, db)
		assert.NoError(t, err, "UpdateSystem should succeed with no operations")
	})
}

func TestV9Migration_UpdateWorkspace(t *testing.T) {
	// Test V9Migration.UpdateWorkspace - this was at 0% coverage
	migration := &V9Migration{}
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

		// Mock CREATE TABLE
		mock.ExpectExec("CREATE TABLE IF NOT EXISTS message_attachments").
			WillReturnResult(sqlmock.NewResult(0, 0))

		// Mock ALTER TABLE
		mock.ExpectExec("ALTER TABLE message_history ADD COLUMN IF NOT EXISTS attachments JSONB").
			WillReturnResult(sqlmock.NewResult(0, 0))

		err = migration.UpdateWorkspace(ctx, cfg, workspace, db)
		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}
