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

func TestV14Migration_GetMajorVersion(t *testing.T) {
	migration := &V14Migration{}
	assert.Equal(t, 14.0, migration.GetMajorVersion())
}

func TestV14Migration_HasSystemUpdate(t *testing.T) {
	migration := &V14Migration{}
	assert.False(t, migration.HasSystemUpdate())
}

func TestV14Migration_HasWorkspaceUpdate(t *testing.T) {
	migration := &V14Migration{}
	assert.True(t, migration.HasWorkspaceUpdate())
}

func TestV14Migration_UpdateSystem(t *testing.T) {
	migration := &V14Migration{}
	cfg := &config.Config{}

	// Should return nil since no system updates
	err := migration.UpdateSystem(context.Background(), cfg, nil)
	assert.NoError(t, err)
}

func TestV14Migration_UpdateWorkspace(t *testing.T) {
	migration := &V14Migration{}
	cfg := &config.Config{}
	workspace := &domain.Workspace{ID: "test-workspace-123"}

	t.Run("successful migration", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		// Expect ALTER TABLE for channel_options column
		mock.ExpectExec("ALTER TABLE message_history").
			WillReturnResult(sqlmock.NewResult(0, 0))

		// Expect CREATE INDEX for channel_options
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_message_history_channel_options").
			WillReturnResult(sqlmock.NewResult(0, 0))

		err = migration.UpdateWorkspace(context.Background(), cfg, workspace, db)
		assert.NoError(t, err)

		// Verify all expectations were met
		err = mock.ExpectationsWereMet()
		assert.NoError(t, err)
	})

	t.Run("error adding column", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		mock.ExpectExec("ALTER TABLE message_history").
			WillReturnError(assert.AnError)

		err = migration.UpdateWorkspace(context.Background(), cfg, workspace, db)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to add channel_options column")
	})

	t.Run("error creating index", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		mock.ExpectExec("ALTER TABLE message_history").
			WillReturnResult(sqlmock.NewResult(0, 0))

		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_message_history_channel_options").
			WillReturnError(assert.AnError)

		err = migration.UpdateWorkspace(context.Background(), cfg, workspace, db)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create index on channel_options")
	})
}
