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

func TestV17Migration_HasSystemUpdate(t *testing.T) {
	// Test V17Migration.HasSystemUpdate - this was at 0% coverage
	migration := &V17Migration{}
	assert.True(t, migration.HasSystemUpdate(), "V17Migration should have system updates")
}

func TestV17Migration_HasWorkspaceUpdate(t *testing.T) {
	// Test V17Migration.HasWorkspaceUpdate - this was at 0% coverage
	migration := &V17Migration{}
	assert.True(t, migration.HasWorkspaceUpdate(), "V17Migration should have workspace updates")
}

func TestV17Migration_ShouldRestartServer(t *testing.T) {
	// Test V17Migration.ShouldRestartServer - this was at 0% coverage
	migration := &V17Migration{}
	assert.False(t, migration.ShouldRestartServer(), "V17Migration should not require server restart")
}

func TestV17Migration_UpdateSystem(t *testing.T) {
	// Test V17Migration.UpdateSystem - this was at 0% coverage
	migration := &V17Migration{}
	ctx := context.Background()
	cfg := &config.Config{}

	t.Run("Success - Basic execution", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		// Mock UPDATE statement
		mock.ExpectExec("UPDATE user_workspaces SET permissions = permissions").
			WillReturnResult(sqlmock.NewResult(0, 0))

		err = migration.UpdateSystem(ctx, cfg, db)
		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestV17Migration_UpdateWorkspace(t *testing.T) {
	// Test V17Migration.UpdateWorkspace - this was at 0% coverage
	migration := &V17Migration{}
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

		// Mock broadcasts table changes
		mock.ExpectExec("ALTER TABLE broadcasts ADD COLUMN IF NOT EXISTS pause_reason").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("UPDATE broadcasts SET audience").
			WillReturnResult(sqlmock.NewResult(0, 0))

		// Mock message_history table changes
		mock.ExpectExec("ALTER TABLE message_history ADD COLUMN IF NOT EXISTS list_id").
			WillReturnResult(sqlmock.NewResult(0, 0))
		// The DO $$ block includes UPDATE and ALTER TABLE DROP COLUMN
		mock.ExpectExec("DO \\$\\$").
			WillReturnResult(sqlmock.NewResult(0, 0))

		// Mock trigger function update
		mock.ExpectExec("CREATE OR REPLACE FUNCTION update_contact_lists_on_status_change").
			WillReturnResult(sqlmock.NewResult(0, 0))

		// Mock blog_categories table
		mock.ExpectExec("CREATE TABLE IF NOT EXISTS blog_categories").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE UNIQUE INDEX IF NOT EXISTS idx_blog_categories_slug").
			WillReturnResult(sqlmock.NewResult(0, 0))

		// Mock blog_posts table
		mock.ExpectExec("CREATE TABLE IF NOT EXISTS blog_posts").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_blog_posts_published").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_blog_posts_category").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE UNIQUE INDEX IF NOT EXISTS idx_blog_posts_slug").
			WillReturnResult(sqlmock.NewResult(0, 0))

		// Mock blog_themes table
		mock.ExpectExec("CREATE TABLE IF NOT EXISTS blog_themes").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE UNIQUE INDEX IF NOT EXISTS idx_blog_themes_published").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_blog_themes_version").
			WillReturnResult(sqlmock.NewResult(0, 0))

		// Mock templates table changes
		mock.ExpectExec("ALTER TABLE templates ALTER COLUMN email DROP NOT NULL").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("ALTER TABLE templates ADD COLUMN IF NOT EXISTS web").
			WillReturnResult(sqlmock.NewResult(0, 0))

		err = migration.UpdateWorkspace(ctx, cfg, workspace, db)
		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}
