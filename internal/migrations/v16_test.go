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

func TestV16Migration_GetMajorVersion(t *testing.T) {
	migration := &V16Migration{}
	assert.Equal(t, 16.0, migration.GetMajorVersion())
}

func TestV16Migration_HasSystemUpdate(t *testing.T) {
	migration := &V16Migration{}
	assert.False(t, migration.HasSystemUpdate())
}

func TestV16Migration_HasWorkspaceUpdate(t *testing.T) {
	migration := &V16Migration{}
	assert.True(t, migration.HasWorkspaceUpdate())
}

func TestV16Migration_ShouldRestartServer(t *testing.T) {
	migration := &V16Migration{}
	assert.False(t, migration.ShouldRestartServer())
}

func TestV16Migration_UpdateSystem(t *testing.T) {
	migration := &V16Migration{}
	ctx := context.Background()

	// Should be a no-op since HasSystemUpdate returns false
	err := migration.UpdateSystem(ctx, nil, nil)
	assert.NoError(t, err)
}

func TestV16Migration_UpdateWorkspace(t *testing.T) {
	// Test V16Migration.UpdateWorkspace - this was at 0% coverage
	migration := &V16Migration{}
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

		// Mock ALTER TABLE statements
		mock.ExpectExec("ALTER TABLE templates ADD COLUMN IF NOT EXISTS integration_id").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("ALTER TABLE transactional_notifications ADD COLUMN IF NOT EXISTS integration_id").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("ALTER TABLE webhook_events RENAME COLUMN email_provider_kind TO source").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("ALTER TABLE webhook_events ALTER COLUMN message_id DROP NOT NULL").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE OR REPLACE FUNCTION track_webhook_event_changes").
			WillReturnResult(sqlmock.NewResult(0, 0))

		err = migration.UpdateWorkspace(ctx, cfg, workspace, db)
		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}
