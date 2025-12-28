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

func TestV22Migration_GetMajorVersion(t *testing.T) {
	migration := &V22Migration{}
	assert.Equal(t, 22.0, migration.GetMajorVersion())
}

func TestV22Migration_HasSystemUpdate(t *testing.T) {
	migration := &V22Migration{}
	assert.True(t, migration.HasSystemUpdate(), "V22Migration should have system updates for LLM permissions")
}

func TestV22Migration_HasWorkspaceUpdate(t *testing.T) {
	migration := &V22Migration{}
	assert.False(t, migration.HasWorkspaceUpdate(), "V22Migration should not have workspace updates")
}

func TestV22Migration_ShouldRestartServer(t *testing.T) {
	migration := &V22Migration{}
	assert.False(t, migration.ShouldRestartServer(), "V22Migration should not require server restart")
}

func TestV22Migration_UpdateSystem(t *testing.T) {
	migration := &V22Migration{}
	ctx := context.Background()
	cfg := &config.Config{}

	t.Run("Success - adds LLM permissions", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		// UpdateSystem adds llm permissions to user_workspaces and workspace_invitations
		mock.ExpectExec("UPDATE user_workspaces").
			WillReturnResult(sqlmock.NewResult(0, 5))
		mock.ExpectExec("UPDATE workspace_invitations").
			WillReturnResult(sqlmock.NewResult(0, 2))

		err = migration.UpdateSystem(ctx, cfg, db)
		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Error - user_workspaces update fails", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mock.ExpectExec("UPDATE user_workspaces").
			WillReturnError(assert.AnError)

		err = migration.UpdateSystem(ctx, cfg, db)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to add llm permissions to user workspaces")
	})

	t.Run("Error - workspace_invitations update fails", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mock.ExpectExec("UPDATE user_workspaces").
			WillReturnResult(sqlmock.NewResult(0, 5))
		mock.ExpectExec("UPDATE workspace_invitations").
			WillReturnError(assert.AnError)

		err = migration.UpdateSystem(ctx, cfg, db)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to add llm permissions to workspace invitations")
	})
}

func TestV22Migration_UpdateWorkspace(t *testing.T) {
	migration := &V22Migration{}
	ctx := context.Background()
	cfg := &config.Config{}
	workspace := &domain.Workspace{
		ID:   "test-workspace",
		Name: "Test Workspace",
	}

	t.Run("Success - no-op for workspace", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		err = migration.UpdateWorkspace(ctx, cfg, workspace, db)
		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestV22Migration_Registered(t *testing.T) {
	// Verify that V22Migration is properly registered
	found := false
	for _, m := range GetRegisteredMigrations() {
		if m.GetMajorVersion() == 22.0 {
			found = true
			break
		}
	}
	assert.True(t, found, "V22Migration should be registered")
}
