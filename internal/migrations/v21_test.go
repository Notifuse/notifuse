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

func TestV21Migration_GetMajorVersion(t *testing.T) {
	migration := &V21Migration{}
	assert.Equal(t, 21.0, migration.GetMajorVersion())
}

func TestV21Migration_HasSystemUpdate(t *testing.T) {
	migration := &V21Migration{}
	assert.False(t, migration.HasSystemUpdate(), "V21Migration should not have system updates")
}

func TestV21Migration_HasWorkspaceUpdate(t *testing.T) {
	migration := &V21Migration{}
	assert.True(t, migration.HasWorkspaceUpdate(), "V21Migration should have workspace updates for email_queue tables")
}

func TestV21Migration_ShouldRestartServer(t *testing.T) {
	migration := &V21Migration{}
	assert.False(t, migration.ShouldRestartServer(), "V21Migration should not require server restart")
}

func TestV21Migration_UpdateSystem(t *testing.T) {
	migration := &V21Migration{}
	ctx := context.Background()
	cfg := &config.Config{}

	db, _, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	// UpdateSystem should be a no-op for V21
	err = migration.UpdateSystem(ctx, cfg, db)
	assert.NoError(t, err)
}

func TestV21Migration_UpdateWorkspace(t *testing.T) {
	migration := &V21Migration{}
	ctx := context.Background()
	cfg := &config.Config{}
	workspace := &domain.Workspace{
		ID:   "test-workspace",
		Name: "Test Workspace",
	}

	t.Run("Success - Creates email_queue table and indexes", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		// PART 1: Create email_queue table
		mock.ExpectExec("CREATE TABLE IF NOT EXISTS email_queue").
			WillReturnResult(sqlmock.NewResult(0, 0))

		// Index for fetching pending emails by priority and creation time
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_email_queue_pending").
			WillReturnResult(sqlmock.NewResult(0, 0))

		// Index for next_retry_at to support retry filtering
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_email_queue_next_retry").
			WillReturnResult(sqlmock.NewResult(0, 0))

		// Index for fetching failed emails ready for retry
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_email_queue_retry").
			WillReturnResult(sqlmock.NewResult(0, 0))

		// Index for tracking broadcast/automation progress
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_email_queue_source").
			WillReturnResult(sqlmock.NewResult(0, 0))

		// Index for integration-based queries
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_email_queue_integration").
			WillReturnResult(sqlmock.NewResult(0, 0))

		// PART 2: Add broadcast count columns
		mock.ExpectExec("ALTER TABLE broadcasts").
			WillReturnResult(sqlmock.NewResult(0, 0))

		// PART 3: Migrate broadcast statuses
		mock.ExpectExec("UPDATE broadcasts SET status = 'processing' WHERE status = 'sending'").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("UPDATE broadcasts SET status = 'processed' WHERE status = 'sent'").
			WillReturnResult(sqlmock.NewResult(0, 0))

		// PART 4: Add missing broadcast_id index
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_message_history_broadcast_id").
			WillReturnResult(sqlmock.NewResult(0, 0))

		// PART 5: Update automation_enroll_contact function with timeline events
		mock.ExpectExec("CREATE OR REPLACE FUNCTION automation_enroll_contact").
			WillReturnResult(sqlmock.NewResult(0, 0))

		err = migration.UpdateWorkspace(ctx, cfg, workspace, db)
		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Error - Create email_queue table fails", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mock.ExpectExec("CREATE TABLE IF NOT EXISTS email_queue").
			WillReturnError(assert.AnError)

		err = migration.UpdateWorkspace(ctx, cfg, workspace, db)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create email_queue table")
	})

	t.Run("Error - Create pending index fails", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mock.ExpectExec("CREATE TABLE IF NOT EXISTS email_queue").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_email_queue_pending").
			WillReturnError(assert.AnError)

		err = migration.UpdateWorkspace(ctx, cfg, workspace, db)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create email_queue pending index")
	})

	t.Run("Error - Create next_retry index fails", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mock.ExpectExec("CREATE TABLE IF NOT EXISTS email_queue").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_email_queue_pending").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_email_queue_next_retry").
			WillReturnError(assert.AnError)

		err = migration.UpdateWorkspace(ctx, cfg, workspace, db)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create email_queue next_retry index")
	})

	t.Run("Error - Create retry index fails", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mock.ExpectExec("CREATE TABLE IF NOT EXISTS email_queue").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_email_queue_pending").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_email_queue_next_retry").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_email_queue_retry").
			WillReturnError(assert.AnError)

		err = migration.UpdateWorkspace(ctx, cfg, workspace, db)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create email_queue retry index")
	})

	t.Run("Error - Create source index fails", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mock.ExpectExec("CREATE TABLE IF NOT EXISTS email_queue").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_email_queue_pending").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_email_queue_next_retry").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_email_queue_retry").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_email_queue_source").
			WillReturnError(assert.AnError)

		err = migration.UpdateWorkspace(ctx, cfg, workspace, db)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create email_queue source index")
	})

	t.Run("Error - Create integration index fails", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mock.ExpectExec("CREATE TABLE IF NOT EXISTS email_queue").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_email_queue_pending").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_email_queue_next_retry").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_email_queue_retry").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_email_queue_source").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_email_queue_integration").
			WillReturnError(assert.AnError)

		err = migration.UpdateWorkspace(ctx, cfg, workspace, db)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create email_queue integration index")
	})

	t.Run("Error - Add broadcast enqueued_count column fails", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mock.ExpectExec("CREATE TABLE IF NOT EXISTS email_queue").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_email_queue_pending").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_email_queue_next_retry").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_email_queue_retry").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_email_queue_source").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_email_queue_integration").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("ALTER TABLE broadcasts").
			WillReturnError(assert.AnError)

		err = migration.UpdateWorkspace(ctx, cfg, workspace, db)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to add broadcast enqueued_count column")
	})

	t.Run("Error - Migrate sending status fails", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mock.ExpectExec("CREATE TABLE IF NOT EXISTS email_queue").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_email_queue_pending").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_email_queue_next_retry").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_email_queue_retry").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_email_queue_source").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_email_queue_integration").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("ALTER TABLE broadcasts").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("UPDATE broadcasts SET status = 'processing' WHERE status = 'sending'").
			WillReturnError(assert.AnError)

		err = migration.UpdateWorkspace(ctx, cfg, workspace, db)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to migrate sending status")
	})

	t.Run("Error - Migrate sent status fails", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mock.ExpectExec("CREATE TABLE IF NOT EXISTS email_queue").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_email_queue_pending").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_email_queue_next_retry").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_email_queue_retry").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_email_queue_source").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_email_queue_integration").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("ALTER TABLE broadcasts").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("UPDATE broadcasts SET status = 'processing' WHERE status = 'sending'").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("UPDATE broadcasts SET status = 'processed' WHERE status = 'sent'").
			WillReturnError(assert.AnError)

		err = migration.UpdateWorkspace(ctx, cfg, workspace, db)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to migrate sent status")
	})

	t.Run("Error - Create message_history broadcast_id index fails", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mock.ExpectExec("CREATE TABLE IF NOT EXISTS email_queue").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_email_queue_pending").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_email_queue_next_retry").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_email_queue_retry").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_email_queue_source").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_email_queue_integration").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("ALTER TABLE broadcasts").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("UPDATE broadcasts SET status = 'processing' WHERE status = 'sending'").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("UPDATE broadcasts SET status = 'processed' WHERE status = 'sent'").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_message_history_broadcast_id").
			WillReturnError(assert.AnError)

		err = migration.UpdateWorkspace(ctx, cfg, workspace, db)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create message_history broadcast_id index")
	})

	t.Run("Error - Update automation_enroll_contact function fails", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mock.ExpectExec("CREATE TABLE IF NOT EXISTS email_queue").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_email_queue_pending").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_email_queue_next_retry").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_email_queue_retry").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_email_queue_source").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_email_queue_integration").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("ALTER TABLE broadcasts").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("UPDATE broadcasts SET status = 'processing' WHERE status = 'sending'").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("UPDATE broadcasts SET status = 'processed' WHERE status = 'sent'").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_message_history_broadcast_id").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE OR REPLACE FUNCTION automation_enroll_contact").
			WillReturnError(assert.AnError)

		err = migration.UpdateWorkspace(ctx, cfg, workspace, db)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to update automation_enroll_contact function")
	})
}

func TestV21Migration_Registration(t *testing.T) {
	// Verify the migration is registered
	migrations := GetRegisteredMigrations()

	var found bool
	for _, m := range migrations {
		if m.GetMajorVersion() == 21.0 {
			found = true
			break
		}
	}

	assert.True(t, found, "V21Migration should be registered")
}
