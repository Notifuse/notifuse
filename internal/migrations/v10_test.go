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

func TestV10Migration_GetMajorVersion(t *testing.T) {
	migration := &V10Migration{}
	assert.Equal(t, 10.0, migration.GetMajorVersion(), "V10Migration should return version 10.0")
}

func TestV10Migration_HasSystemUpdate(t *testing.T) {
	migration := &V10Migration{}
	assert.False(t, migration.HasSystemUpdate(), "V10Migration should not have system updates")
}

func TestV10Migration_HasWorkspaceUpdate(t *testing.T) {
	migration := &V10Migration{}
	assert.True(t, migration.HasWorkspaceUpdate(), "V10Migration should have workspace updates")
}

func TestV10Migration_UpdateSystem(t *testing.T) {
	migration := &V10Migration{}
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

func TestV10Migration_UpdateWorkspace(t *testing.T) {
	migration := &V10Migration{}
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

		// Mock ALTER TABLE to add list_ids column
		mock.ExpectExec("ALTER TABLE message_history").
			WillReturnResult(sqlmock.NewResult(0, 0))

		// Mock UPDATE to backfill list_ids from broadcasts
		mock.ExpectExec("UPDATE message_history mh").
			WillReturnResult(sqlmock.NewResult(0, 100))

		// Mock UPDATE to set historical complaints
		mock.ExpectExec("UPDATE contact_lists cl").
			WillReturnResult(sqlmock.NewResult(0, 5))

		// Mock UPDATE to set historical bounces
		mock.ExpectExec("UPDATE contact_lists cl").
			WillReturnResult(sqlmock.NewResult(0, 10))

		// Mock CREATE FUNCTION for update_contact_lists_on_status_change
		mock.ExpectExec("CREATE OR REPLACE FUNCTION update_contact_lists_on_status_change").
			WillReturnResult(sqlmock.NewResult(0, 0))

		// Mock DROP and CREATE TRIGGER
		mock.ExpectExec("DROP TRIGGER IF EXISTS message_history_status_trigger").
			WillReturnResult(sqlmock.NewResult(0, 0))

		err = migration.UpdateWorkspace(ctx, cfg, workspace, db)
		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Error - ALTER TABLE fails", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		mock.ExpectExec("ALTER TABLE message_history").
			WillReturnError(sql.ErrConnDone)

		err = migration.UpdateWorkspace(ctx, cfg, workspace, db)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to add list_ids column")
	})

	t.Run("Error - Backfill fails", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		mock.ExpectExec("ALTER TABLE message_history").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("UPDATE message_history mh").
			WillReturnError(sql.ErrConnDone)

		err = migration.UpdateWorkspace(ctx, cfg, workspace, db)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to backfill list_ids")
	})

	t.Run("Error - Update historical complaints fails", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		mock.ExpectExec("ALTER TABLE message_history").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("UPDATE message_history mh").
			WillReturnResult(sqlmock.NewResult(0, 100))
		mock.ExpectExec("UPDATE contact_lists cl").
			WillReturnError(sql.ErrConnDone)

		err = migration.UpdateWorkspace(ctx, cfg, workspace, db)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to update contact_lists for historical complaints")
	})

	t.Run("Error - Update historical bounces fails", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		mock.ExpectExec("ALTER TABLE message_history").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("UPDATE message_history mh").
			WillReturnResult(sqlmock.NewResult(0, 100))
		mock.ExpectExec("UPDATE contact_lists cl").
			WillReturnResult(sqlmock.NewResult(0, 5))
		mock.ExpectExec("UPDATE contact_lists cl").
			WillReturnError(sql.ErrConnDone)

		err = migration.UpdateWorkspace(ctx, cfg, workspace, db)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to update contact_lists for historical bounces")
	})

	t.Run("Error - CREATE FUNCTION fails", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		mock.ExpectExec("ALTER TABLE message_history").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("UPDATE message_history mh").
			WillReturnResult(sqlmock.NewResult(0, 100))
		mock.ExpectExec("UPDATE contact_lists cl").
			WillReturnResult(sqlmock.NewResult(0, 5))
		mock.ExpectExec("UPDATE contact_lists cl").
			WillReturnResult(sqlmock.NewResult(0, 10))
		mock.ExpectExec("CREATE OR REPLACE FUNCTION update_contact_lists_on_status_change").
			WillReturnError(sql.ErrConnDone)

		err = migration.UpdateWorkspace(ctx, cfg, workspace, db)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create trigger function")
	})

	t.Run("Error - CREATE TRIGGER fails", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		mock.ExpectExec("ALTER TABLE message_history").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("UPDATE message_history mh").
			WillReturnResult(sqlmock.NewResult(0, 100))
		mock.ExpectExec("UPDATE contact_lists cl").
			WillReturnResult(sqlmock.NewResult(0, 5))
		mock.ExpectExec("UPDATE contact_lists cl").
			WillReturnResult(sqlmock.NewResult(0, 10))
		mock.ExpectExec("CREATE OR REPLACE FUNCTION update_contact_lists_on_status_change").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("DROP TRIGGER IF EXISTS message_history_status_trigger").
			WillReturnError(sql.ErrConnDone)

		err = migration.UpdateWorkspace(ctx, cfg, workspace, db)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create trigger")
	})
}

func TestV10Migration_Registration(t *testing.T) {
	// Test that the migration is registered in the default registry
	migration, exists := GetRegisteredMigration(10.0)
	assert.True(t, exists, "V10Migration should be registered")
	assert.NotNil(t, migration, "V10Migration should not be nil")
	assert.Equal(t, 10.0, migration.GetMajorVersion(), "Registered migration should be version 10.0")
}
