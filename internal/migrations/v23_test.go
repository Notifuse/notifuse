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

func TestV23Migration_GetMajorVersion(t *testing.T) {
	migration := &V23Migration{}
	assert.Equal(t, 23.0, migration.GetMajorVersion())
}

func TestV23Migration_HasSystemUpdate(t *testing.T) {
	migration := &V23Migration{}
	assert.False(t, migration.HasSystemUpdate(), "V23Migration should not have system updates")
}

func TestV23Migration_HasWorkspaceUpdate(t *testing.T) {
	migration := &V23Migration{}
	assert.True(t, migration.HasWorkspaceUpdate(), "V23Migration should have workspace updates")
}

func TestV23Migration_ShouldRestartServer(t *testing.T) {
	migration := &V23Migration{}
	assert.False(t, migration.ShouldRestartServer())
}

func TestV23Migration_UpdateSystem(t *testing.T) {
	migration := &V23Migration{}
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

func TestV23Migration_UpdateWorkspace(t *testing.T) {
	migration := &V23Migration{}
	ctx := context.Background()
	cfg := &config.Config{}
	workspace := &domain.Workspace{
		ID:   "test-workspace",
		Name: "Test Workspace",
	}

	t.Run("Success - reinstalls all trigger functions", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		// Expect track_contact_list_changes update
		mock.ExpectExec("CREATE OR REPLACE FUNCTION track_contact_list_changes").
			WillReturnResult(sqlmock.NewResult(0, 0))

		// Expect track_contact_segment_changes update
		mock.ExpectExec("CREATE OR REPLACE FUNCTION track_contact_segment_changes").
			WillReturnResult(sqlmock.NewResult(0, 0))

		// Expect track_contact_changes update
		mock.ExpectExec("CREATE OR REPLACE FUNCTION track_contact_changes").
			WillReturnResult(sqlmock.NewResult(0, 0))

		err = migration.UpdateWorkspace(ctx, cfg, workspace, db)
		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Error - track_contact_list_changes fails", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mock.ExpectExec("CREATE OR REPLACE FUNCTION track_contact_list_changes").
			WillReturnError(assert.AnError)

		err = migration.UpdateWorkspace(ctx, cfg, workspace, db)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to update track_contact_list_changes")
	})

	t.Run("Error - track_contact_segment_changes fails", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mock.ExpectExec("CREATE OR REPLACE FUNCTION track_contact_list_changes").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE OR REPLACE FUNCTION track_contact_segment_changes").
			WillReturnError(assert.AnError)

		err = migration.UpdateWorkspace(ctx, cfg, workspace, db)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to update track_contact_segment_changes")
	})

	t.Run("Error - track_contact_changes fails", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mock.ExpectExec("CREATE OR REPLACE FUNCTION track_contact_list_changes").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE OR REPLACE FUNCTION track_contact_segment_changes").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE OR REPLACE FUNCTION track_contact_changes").
			WillReturnError(assert.AnError)

		err = migration.UpdateWorkspace(ctx, cfg, workspace, db)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to update track_contact_changes")
	})
}

func TestV23Migration_Registered(t *testing.T) {
	found := false
	for _, m := range GetRegisteredMigrations() {
		if m.GetMajorVersion() == 23.0 {
			found = true
			break
		}
	}
	assert.True(t, found, "V23Migration should be registered")
}
