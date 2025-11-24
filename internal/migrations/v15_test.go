package migrations

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Notifuse/notifuse/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestV15Migration_GetMajorVersion(t *testing.T) {
	migration := &V15Migration{}
	assert.Equal(t, 15.0, migration.GetMajorVersion())
}

func TestV15Migration_HasSystemUpdate(t *testing.T) {
	migration := &V15Migration{}
	assert.True(t, migration.HasSystemUpdate())
}

func TestV15Migration_HasWorkspaceUpdate(t *testing.T) {
	migration := &V15Migration{}
	assert.False(t, migration.HasWorkspaceUpdate())
}

func TestV15Migration_ShouldRestartServer(t *testing.T) {
	migration := &V15Migration{}
	assert.False(t, migration.ShouldRestartServer())
}

func TestV15Migration_UpdateSystem(t *testing.T) {
	// Test V15Migration.UpdateSystem - this was at 0% coverage
	migration := &V15Migration{}
	ctx := context.Background()
	cfg := &config.Config{
		Security: config.SecurityConfig{
			SecretKey: "test-secret-key-for-migration",
		},
	}

	t.Run("Success - Basic execution", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		// Mock COUNT queries (these happen before the DELETE statements)
		mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM users WHERE type = 'api_key'").
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
		mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM workspace_invitations WHERE expires_at > NOW\\(\\)").
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
		mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM user_sessions").
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

		// Mock DELETE statements in the order they appear in the code
		mock.ExpectExec("DELETE FROM settings WHERE key = 'encrypted_paseto_private_key'").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("DELETE FROM settings WHERE key = 'encrypted_paseto_public_key'").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("DELETE FROM users WHERE type = 'api_key'").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("DELETE FROM workspace_invitations WHERE expires_at > NOW\\(\\)").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("DELETE FROM user_sessions").
			WillReturnResult(sqlmock.NewResult(0, 0))

		err = migration.UpdateSystem(ctx, cfg, db)
		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Error - Missing SECRET_KEY", func(t *testing.T) {
		migration := &V15Migration{}
		ctx := context.Background()
		cfg := &config.Config{
			Security: config.SecurityConfig{
				SecretKey: "", // Missing secret key
			},
		}

		db, _, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		err = migration.UpdateSystem(ctx, cfg, db)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "SECRET_KEY is required")
	})
}

func TestV15Migration_UpdateWorkspace(t *testing.T) {
	migration := &V15Migration{}
	ctx := context.Background()

	// Should be a no-op since HasWorkspaceUpdate returns false
	err := migration.UpdateWorkspace(ctx, nil, nil, nil)
	assert.NoError(t, err)
}
