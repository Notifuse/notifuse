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

func TestV8Migration_HasSystemUpdate(t *testing.T) {
	// Test V8Migration.HasSystemUpdate - this was at 0% coverage
	migration := &V8Migration{}
	assert.True(t, migration.HasSystemUpdate(), "V8Migration should have system updates")
}

func TestV8Migration_HasWorkspaceUpdate(t *testing.T) {
	// Test V8Migration.HasWorkspaceUpdate - this was at 0% coverage
	migration := &V8Migration{}
	assert.True(t, migration.HasWorkspaceUpdate(), "V8Migration should have workspace updates")
}

func TestV8Migration_ShouldRestartServer(t *testing.T) {
	// Test V8Migration.ShouldRestartServer - this was at 0% coverage
	migration := &V8Migration{}
	assert.False(t, migration.ShouldRestartServer(), "V8Migration should not require server restart")
}

func TestV8Migration_UpdateSystem(t *testing.T) {
	// Test V8Migration.UpdateSystem - this was at 0% coverage
	migration := &V8Migration{}
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

func TestV8Migration_UpdateWorkspace(t *testing.T) {
	// Test V8Migration.UpdateWorkspace - this was at 0% coverage
	migration := &V8Migration{}
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

		// Mock all the ALTER TABLE and CREATE statements
		mock.ExpectExec("ALTER TABLE contact_timeline ADD COLUMN IF NOT EXISTS kind").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("ALTER TABLE contact_timeline ADD COLUMN IF NOT EXISTS db_created_at").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("UPDATE contact_timeline").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("UPDATE contact_timeline SET db_created_at").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("ALTER TABLE contact_timeline ALTER COLUMN created_at DROP DEFAULT").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("ALTER TABLE contacts ADD COLUMN IF NOT EXISTS db_created_at").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_contact_timeline_kind").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE TABLE IF NOT EXISTS contact_segment_queue").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_contact_segment_queue_queued_at").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE OR REPLACE FUNCTION track_contact_changes").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE OR REPLACE FUNCTION track_contact_list_changes").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE OR REPLACE FUNCTION track_message_history_changes").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE OR REPLACE FUNCTION track_webhook_event_changes").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE OR REPLACE FUNCTION queue_contact_for_segment_recomputation").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("DROP TRIGGER IF EXISTS contact_timeline_queue_trigger").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE TRIGGER contact_timeline_queue_trigger").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE TABLE IF NOT EXISTS segments").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_segments_status").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE TABLE IF NOT EXISTS contact_segments").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_contact_segments_segment_id").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_contact_segments_version").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE OR REPLACE FUNCTION track_contact_segment_changes").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("DROP TRIGGER IF EXISTS contact_segment_changes_trigger").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE TRIGGER contact_segment_changes_trigger").
			WillReturnResult(sqlmock.NewResult(0, 0))

		err = migration.UpdateWorkspace(ctx, cfg, workspace, db)
		assert.NoError(t, err)
	})
}
