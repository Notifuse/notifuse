package migrations

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
)

func TestV28Migration_GetMajorVersion(t *testing.T) {
	m := &V28Migration{}
	assert.Equal(t, 28.0, m.GetMajorVersion())
}

func TestV28Migration_HasSystemUpdate(t *testing.T) {
	m := &V28Migration{}
	assert.True(t, m.HasSystemUpdate())
}

func TestV28Migration_HasWorkspaceUpdate(t *testing.T) {
	m := &V28Migration{}
	assert.True(t, m.HasWorkspaceUpdate())
}

func TestV28Migration_ShouldRestartServer(t *testing.T) {
	m := &V28Migration{}
	assert.False(t, m.ShouldRestartServer())
}

func TestV28Migration_UpdateSystem_Success(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	m := &V28Migration{}
	cfg := &config.Config{}

	// Expect UPDATE to backfill workspace language settings
	mock.ExpectExec(`UPDATE workspaces SET settings`).
		WillReturnResult(sqlmock.NewResult(0, 3))

	err = m.UpdateSystem(context.Background(), cfg, db)
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestV28Migration_UpdateSystem_Error(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	m := &V28Migration{}
	cfg := &config.Config{}

	// Query fails
	mock.ExpectExec(`UPDATE workspaces SET settings`).
		WillReturnError(assert.AnError)

	err = m.UpdateSystem(context.Background(), cfg, db)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to backfill workspace language settings")
}

func TestV28Migration_UpdateWorkspace_Success(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	m := &V28Migration{}
	cfg := &config.Config{}
	workspace := &domain.Workspace{ID: "test-workspace"}

	// Expect ALTER TABLE for translations column
	mock.ExpectExec(`ALTER TABLE templates ADD COLUMN IF NOT EXISTS translations`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	// Expect UPDATE to normalize NULL translations
	mock.ExpectExec(`UPDATE templates SET translations`).
		WillReturnResult(sqlmock.NewResult(0, 5))
	// Expect ALTER TABLE for transactional_notification_id column
	mock.ExpectExec(`ALTER TABLE message_history ADD COLUMN IF NOT EXISTS transactional_notification_id`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	// Expect CREATE INDEX for transactional_notification_id
	mock.ExpectExec(`CREATE INDEX IF NOT EXISTS idx_message_history_transactional_notification_id`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	// Expect UPDATE to backfill transactional_notification_id
	mock.ExpectExec(`UPDATE message_history mh`).
		WillReturnResult(sqlmock.NewResult(0, 10))

	err = m.UpdateWorkspace(context.Background(), cfg, workspace, db)
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestV28Migration_UpdateWorkspace_AlterError(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	m := &V28Migration{}
	cfg := &config.Config{}
	workspace := &domain.Workspace{ID: "test-workspace"}

	// ALTER TABLE fails
	mock.ExpectExec(`ALTER TABLE templates ADD COLUMN IF NOT EXISTS translations`).
		WillReturnError(assert.AnError)

	err = m.UpdateWorkspace(context.Background(), cfg, workspace, db)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to add translations column to templates")
}

func TestV28Migration_UpdateWorkspace_NormalizeError(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	m := &V28Migration{}
	cfg := &config.Config{}
	workspace := &domain.Workspace{ID: "test-workspace"}

	// ALTER TABLE succeeds
	mock.ExpectExec(`ALTER TABLE templates ADD COLUMN IF NOT EXISTS translations`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	// UPDATE to normalize NULLs fails
	mock.ExpectExec(`UPDATE templates SET translations`).
		WillReturnError(assert.AnError)

	err = m.UpdateWorkspace(context.Background(), cfg, workspace, db)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to normalize NULL translations")
}

func TestV28Migration_UpdateWorkspace_ColumnAlreadyExists(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	m := &V28Migration{}
	cfg := &config.Config{}
	workspace := &domain.Workspace{ID: "test-workspace"}

	// Query succeeds (column already exists due to IF NOT EXISTS)
	mock.ExpectExec(`ALTER TABLE templates ADD COLUMN IF NOT EXISTS translations`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	// Normalize NULLs (idempotent - may update 0 rows)
	mock.ExpectExec(`UPDATE templates SET translations`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	// transactional_notification_id column (idempotent)
	mock.ExpectExec(`ALTER TABLE message_history ADD COLUMN IF NOT EXISTS transactional_notification_id`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`CREATE INDEX IF NOT EXISTS idx_message_history_transactional_notification_id`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`UPDATE message_history mh`).
		WillReturnResult(sqlmock.NewResult(0, 0))

	err = m.UpdateWorkspace(context.Background(), cfg, workspace, db)
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestV28Migration_UpdateWorkspace_AddColumnError(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	m := &V28Migration{}
	cfg := &config.Config{}
	workspace := &domain.Workspace{ID: "test-workspace"}

	mock.ExpectExec(`ALTER TABLE templates ADD COLUMN IF NOT EXISTS translations`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`UPDATE templates SET translations`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`ALTER TABLE message_history ADD COLUMN IF NOT EXISTS transactional_notification_id`).
		WillReturnError(assert.AnError)

	err = m.UpdateWorkspace(context.Background(), cfg, workspace, db)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to add transactional_notification_id column")
}

func TestV28Migration_UpdateWorkspace_CreateIndexError(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	m := &V28Migration{}
	cfg := &config.Config{}
	workspace := &domain.Workspace{ID: "test-workspace"}

	mock.ExpectExec(`ALTER TABLE templates ADD COLUMN IF NOT EXISTS translations`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`UPDATE templates SET translations`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`ALTER TABLE message_history ADD COLUMN IF NOT EXISTS transactional_notification_id`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`CREATE INDEX IF NOT EXISTS idx_message_history_transactional_notification_id`).
		WillReturnError(assert.AnError)

	err = m.UpdateWorkspace(context.Background(), cfg, workspace, db)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create transactional_notification_id index")
}

func TestV28Migration_UpdateWorkspace_BackfillError(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	m := &V28Migration{}
	cfg := &config.Config{}
	workspace := &domain.Workspace{ID: "test-workspace"}

	mock.ExpectExec(`ALTER TABLE templates ADD COLUMN IF NOT EXISTS translations`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`UPDATE templates SET translations`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`ALTER TABLE message_history ADD COLUMN IF NOT EXISTS transactional_notification_id`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`CREATE INDEX IF NOT EXISTS idx_message_history_transactional_notification_id`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`UPDATE message_history mh`).
		WillReturnError(assert.AnError)

	err = m.UpdateWorkspace(context.Background(), cfg, workspace, db)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to backfill transactional_notification_id")
}

func TestV28Migration_Registration(t *testing.T) {
	// Verify the migration is registered
	found := false
	for _, m := range GetRegisteredMigrations() {
		if m.GetMajorVersion() == 28.0 {
			found = true
			break
		}
	}
	assert.True(t, found, "V28Migration should be registered")
}
