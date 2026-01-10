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

func TestV25Migration_GetMajorVersion(t *testing.T) {
	migration := &V25Migration{}
	assert.Equal(t, 25.0, migration.GetMajorVersion())
}

func TestV25Migration_HasSystemUpdate(t *testing.T) {
	migration := &V25Migration{}
	assert.False(t, migration.HasSystemUpdate(), "V25Migration should not have system updates")
}

func TestV25Migration_HasWorkspaceUpdate(t *testing.T) {
	migration := &V25Migration{}
	assert.True(t, migration.HasWorkspaceUpdate(), "V25Migration should have workspace updates")
}

func TestV25Migration_ShouldRestartServer(t *testing.T) {
	migration := &V25Migration{}
	assert.False(t, migration.ShouldRestartServer())
}

func TestV25Migration_UpdateSystem(t *testing.T) {
	migration := &V25Migration{}
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

func TestV25Migration_UpdateWorkspace(t *testing.T) {
	migration := &V25Migration{}
	ctx := context.Background()
	cfg := &config.Config{}
	workspace := &domain.Workspace{
		ID:   "test-workspace",
		Name: "Test Workspace",
	}

	t.Run("Success - updates function and regenerates triggers", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		// Expect CREATE OR REPLACE FUNCTION for automation_enroll_contact (4 params, no list_id)
		mock.ExpectExec("CREATE OR REPLACE FUNCTION automation_enroll_contact").
			WillReturnResult(sqlmock.NewResult(0, 0))

		// Expect DO $$ block for regenerating automation trigger functions
		mock.ExpectExec("DO").
			WillReturnResult(sqlmock.NewResult(0, 0))

		err = migration.UpdateWorkspace(ctx, cfg, workspace, db)
		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Error - function update fails", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mock.ExpectExec("CREATE OR REPLACE FUNCTION automation_enroll_contact").
			WillReturnError(assert.AnError)

		err = migration.UpdateWorkspace(ctx, cfg, workspace, db)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to update automation_enroll_contact function")
	})

	t.Run("Error - trigger regeneration fails", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		// Function update succeeds
		mock.ExpectExec("CREATE OR REPLACE FUNCTION automation_enroll_contact").
			WillReturnResult(sqlmock.NewResult(0, 0))

		// Trigger regeneration fails
		mock.ExpectExec("DO").
			WillReturnError(assert.AnError)

		err = migration.UpdateWorkspace(ctx, cfg, workspace, db)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to regenerate automation trigger functions")
	})
}

func TestV25Migration_Registered(t *testing.T) {
	found := false
	for _, m := range GetRegisteredMigrations() {
		if m.GetMajorVersion() == 25.0 {
			found = true
			break
		}
	}
	assert.True(t, found, "V25Migration should be registered")
}
