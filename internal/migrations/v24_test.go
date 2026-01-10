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

func TestV24Migration_GetMajorVersion(t *testing.T) {
	migration := &V24Migration{}
	assert.Equal(t, 24.0, migration.GetMajorVersion())
}

func TestV24Migration_HasSystemUpdate(t *testing.T) {
	migration := &V24Migration{}
	assert.False(t, migration.HasSystemUpdate(), "V24Migration should not have system updates")
}

func TestV24Migration_HasWorkspaceUpdate(t *testing.T) {
	migration := &V24Migration{}
	assert.True(t, migration.HasWorkspaceUpdate(), "V24Migration should have workspace updates")
}

func TestV24Migration_ShouldRestartServer(t *testing.T) {
	migration := &V24Migration{}
	assert.False(t, migration.ShouldRestartServer())
}

func TestV24Migration_UpdateSystem(t *testing.T) {
	migration := &V24Migration{}
	ctx := context.Background()
	cfg := &config.Config{}

	t.Run("Success - no-op for system", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		err = migration.UpdateSystem(ctx, cfg, db)
		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestV24Migration_UpdateWorkspace(t *testing.T) {
	migration := &V24Migration{}
	ctx := context.Background()
	cfg := &config.Config{}
	workspace := &domain.Workspace{
		ID:   "test-workspace",
		Name: "Test Workspace",
	}

	t.Run("Success - fixes automation stats", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		// Expect UPDATE to fix automation stats
		mock.ExpectExec("UPDATE automations").
			WillReturnResult(sqlmock.NewResult(0, 5)) // 5 rows updated

		err = migration.UpdateWorkspace(ctx, cfg, workspace, db)
		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Success - no automations to fix", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		// Expect UPDATE to fix automation stats (0 rows affected is ok)
		mock.ExpectExec("UPDATE automations").
			WillReturnResult(sqlmock.NewResult(0, 0))

		err = migration.UpdateWorkspace(ctx, cfg, workspace, db)
		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Error - update fails", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mock.ExpectExec("UPDATE automations").
			WillReturnError(assert.AnError)

		err = migration.UpdateWorkspace(ctx, cfg, workspace, db)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to fix automation stats")
	})
}

func TestV24Migration_Registered(t *testing.T) {
	found := false
	for _, m := range GetRegisteredMigrations() {
		if m.GetMajorVersion() == 24.0 {
			found = true
			break
		}
	}
	assert.True(t, found, "V24Migration should be registered")
}
