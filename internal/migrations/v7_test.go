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

func TestV7Migration_GetMajorVersion(t *testing.T) {
	migration := &V7Migration{}
	assert.Equal(t, 7.0, migration.GetMajorVersion(), "V7Migration should return version 7.0")
}

func TestV7Migration_HasSystemUpdate(t *testing.T) {
	migration := &V7Migration{}
	assert.False(t, migration.HasSystemUpdate(), "V7Migration should not have system updates")
}

func TestV7Migration_HasWorkspaceUpdate(t *testing.T) {
	migration := &V7Migration{}
	assert.True(t, migration.HasWorkspaceUpdate(), "V7Migration should have workspace updates")
}

func TestV7Migration_UpdateSystem(t *testing.T) {
	migration := &V7Migration{}
	ctx := context.Background()
	cfg := &config.Config{}

	t.Run("Success - No system updates", func(t *testing.T) {
		db, _, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		err = migration.UpdateSystem(ctx, cfg, db)
		assert.NoError(t, err, "UpdateSystem should succeed with no operations")
	})
}

func TestV7Migration_UpdateWorkspace(t *testing.T) {
	migration := &V7Migration{}
	ctx := context.Background()
	cfg := &config.Config{}
	workspace := &domain.Workspace{
		ID:   "test-workspace",
		Name: "Test Workspace",
	}

	t.Run("Success - All operations complete", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		// Mock CREATE TABLE
		mock.ExpectExec("CREATE TABLE IF NOT EXISTS contact_timeline").
			WillReturnResult(sqlmock.NewResult(0, 0))

		// Mock CREATE INDEX for composite email_created_at index
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_contact_timeline_email_created_at ON contact_timeline").
			WillReturnResult(sqlmock.NewResult(0, 0))

		// Mock CREATE INDEX for entity_id
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_contact_timeline_entity_id ON contact_timeline").
			WillReturnResult(sqlmock.NewResult(0, 0))

		// Mock CREATE FUNCTION for track_contact_changes
		mock.ExpectExec("CREATE OR REPLACE FUNCTION track_contact_changes").
			WillReturnResult(sqlmock.NewResult(0, 0))

		// Mock DROP and CREATE TRIGGER for contacts
		mock.ExpectExec("DROP TRIGGER IF EXISTS contact_changes_trigger ON contacts").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE TRIGGER contact_changes_trigger").
			WillReturnResult(sqlmock.NewResult(0, 0))

		// Mock CREATE FUNCTION for track_contact_list_changes
		mock.ExpectExec("CREATE OR REPLACE FUNCTION track_contact_list_changes").
			WillReturnResult(sqlmock.NewResult(0, 0))

		// Mock DROP and CREATE TRIGGER for contact_lists
		mock.ExpectExec("DROP TRIGGER IF EXISTS contact_list_changes_trigger ON contact_lists").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE TRIGGER contact_list_changes_trigger").
			WillReturnResult(sqlmock.NewResult(0, 0))

		// Mock CREATE FUNCTION for track_message_history_changes
		mock.ExpectExec("CREATE OR REPLACE FUNCTION track_message_history_changes").
			WillReturnResult(sqlmock.NewResult(0, 0))

		// Mock DROP and CREATE TRIGGER for message_history
		mock.ExpectExec("DROP TRIGGER IF EXISTS message_history_changes_trigger ON message_history").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE TRIGGER message_history_changes_trigger").
			WillReturnResult(sqlmock.NewResult(0, 0))

		err = migration.UpdateWorkspace(ctx, cfg, workspace, db)
		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Error - CREATE TABLE fails", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		mock.ExpectExec("CREATE TABLE IF NOT EXISTS contact_timeline").
			WillReturnError(sql.ErrConnDone)

		err = migration.UpdateWorkspace(ctx, cfg, workspace, db)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create contact_timeline table")
	})

	t.Run("Error - CREATE INDEX for email_created_at fails", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		mock.ExpectExec("CREATE TABLE IF NOT EXISTS contact_timeline").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_contact_timeline_email_created_at ON contact_timeline").
			WillReturnError(sql.ErrConnDone)

		err = migration.UpdateWorkspace(ctx, cfg, workspace, db)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create email_created_at composite index")
	})

	t.Run("Error - CREATE INDEX for entity_id fails", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		mock.ExpectExec("CREATE TABLE IF NOT EXISTS contact_timeline").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_contact_timeline_email_created_at ON contact_timeline").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_contact_timeline_entity_id ON contact_timeline").
			WillReturnError(sql.ErrConnDone)

		err = migration.UpdateWorkspace(ctx, cfg, workspace, db)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create entity_id index")
	})

	t.Run("Error - CREATE FUNCTION for track_contact_changes fails", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		mock.ExpectExec("CREATE TABLE IF NOT EXISTS contact_timeline").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_contact_timeline_email_created_at ON contact_timeline").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_contact_timeline_entity_id ON contact_timeline").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE OR REPLACE FUNCTION track_contact_changes").
			WillReturnError(sql.ErrConnDone)

		err = migration.UpdateWorkspace(ctx, cfg, workspace, db)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create track_contact_changes function")
	})

	t.Run("Error - CREATE TRIGGER for contacts fails", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		mock.ExpectExec("CREATE TABLE IF NOT EXISTS contact_timeline").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_contact_timeline_email_created_at ON contact_timeline").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_contact_timeline_entity_id ON contact_timeline").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE OR REPLACE FUNCTION track_contact_changes").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("DROP TRIGGER IF EXISTS contact_changes_trigger ON contacts").
			WillReturnError(sql.ErrConnDone)

		err = migration.UpdateWorkspace(ctx, cfg, workspace, db)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create contact_changes_trigger")
	})

	t.Run("Error - CREATE FUNCTION for track_contact_list_changes fails", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		mock.ExpectExec("CREATE TABLE IF NOT EXISTS contact_timeline").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_contact_timeline_email_created_at ON contact_timeline").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_contact_timeline_entity_id ON contact_timeline").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE OR REPLACE FUNCTION track_contact_changes").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("DROP TRIGGER IF EXISTS contact_changes_trigger ON contacts").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE TRIGGER contact_changes_trigger").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE OR REPLACE FUNCTION track_contact_list_changes").
			WillReturnError(sql.ErrConnDone)

		err = migration.UpdateWorkspace(ctx, cfg, workspace, db)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create track_contact_list_changes function")
	})

	t.Run("Error - CREATE TRIGGER for contact_lists fails", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		mock.ExpectExec("CREATE TABLE IF NOT EXISTS contact_timeline").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_contact_timeline_email_created_at ON contact_timeline").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_contact_timeline_entity_id ON contact_timeline").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE OR REPLACE FUNCTION track_contact_changes").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("DROP TRIGGER IF EXISTS contact_changes_trigger ON contacts").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE TRIGGER contact_changes_trigger").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE OR REPLACE FUNCTION track_contact_list_changes").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("DROP TRIGGER IF EXISTS contact_list_changes_trigger ON contact_lists").
			WillReturnError(sql.ErrConnDone)

		err = migration.UpdateWorkspace(ctx, cfg, workspace, db)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create contact_list_changes_trigger")
	})

	t.Run("Error - CREATE FUNCTION for track_message_history_changes fails", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		mock.ExpectExec("CREATE TABLE IF NOT EXISTS contact_timeline").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_contact_timeline_email_created_at ON contact_timeline").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_contact_timeline_entity_id ON contact_timeline").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE OR REPLACE FUNCTION track_contact_changes").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("DROP TRIGGER IF EXISTS contact_changes_trigger ON contacts").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE TRIGGER contact_changes_trigger").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE OR REPLACE FUNCTION track_contact_list_changes").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("DROP TRIGGER IF EXISTS contact_list_changes_trigger ON contact_lists").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE TRIGGER contact_list_changes_trigger").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE OR REPLACE FUNCTION track_message_history_changes").
			WillReturnError(sql.ErrConnDone)

		err = migration.UpdateWorkspace(ctx, cfg, workspace, db)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create track_message_history_changes function")
	})

	t.Run("Error - CREATE TRIGGER for message_history fails", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		mock.ExpectExec("CREATE TABLE IF NOT EXISTS contact_timeline").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_contact_timeline_email_created_at ON contact_timeline").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_contact_timeline_entity_id ON contact_timeline").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE OR REPLACE FUNCTION track_contact_changes").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("DROP TRIGGER IF EXISTS contact_changes_trigger ON contacts").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE TRIGGER contact_changes_trigger").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE OR REPLACE FUNCTION track_contact_list_changes").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("DROP TRIGGER IF EXISTS contact_list_changes_trigger ON contact_lists").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE TRIGGER contact_list_changes_trigger").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE OR REPLACE FUNCTION track_message_history_changes").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("DROP TRIGGER IF EXISTS message_history_changes_trigger ON message_history").
			WillReturnError(sql.ErrConnDone)

		err = migration.UpdateWorkspace(ctx, cfg, workspace, db)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create message_history_changes_trigger")
	})
}

func TestV7Migration_Registration(t *testing.T) {
	// Test that the migration is registered in the default registry
	migration, exists := GetRegisteredMigration(7.0)
	assert.True(t, exists, "V7Migration should be registered")
	assert.NotNil(t, migration, "V7Migration should not be nil")
	assert.Equal(t, 7.0, migration.GetMajorVersion(), "Registered migration should be version 7.0")
}
