package migrations

import (
	"context"
	"database/sql"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestV4Migration_HasSystemUpdate(t *testing.T) {
	migration := &V4Migration{}
	assert.True(t, migration.HasSystemUpdate(), "V4Migration should have system updates")
}

func TestV4Migration_HasWorkspaceUpdate(t *testing.T) {
	migration := &V4Migration{}
	assert.False(t, migration.HasWorkspaceUpdate(), "V4Migration should not have workspace updates")
}

func TestV4Migration_UpdateSystem(t *testing.T) {
	migration := &V4Migration{}
	ctx := context.Background()
	cfg := &config.Config{}

	t.Run("Success - All operations complete", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		// Mock the verification query
		mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM users").
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(5))

		// Mock the ALTER TABLE for permissions column
		mock.ExpectExec("ALTER TABLE user_workspaces ADD COLUMN IF NOT EXISTS permissions JSONB DEFAULT").
			WillReturnResult(sqlmock.NewResult(0, 0))

		// Mock the UPDATE for owners
		mock.ExpectExec("UPDATE user_workspaces SET permissions = .+ WHERE role = 'owner'").
			WillReturnResult(sqlmock.NewResult(0, 2))

		// Mock the UPDATE for members
		mock.ExpectExec("UPDATE user_workspaces SET permissions = .+ WHERE role = 'member'").
			WillReturnResult(sqlmock.NewResult(0, 3))

		err = migration.UpdateSystem(ctx, cfg, db)
		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Error - Database verification fails", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		// Mock the verification query to fail
		mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM users").
			WillReturnError(sql.ErrConnDone)

		err = migration.UpdateSystem(ctx, cfg, db)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to verify system database connection")
	})

	t.Run("Error - ALTER TABLE fails", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		// Mock successful verification
		mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM users").
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(5))

		// Mock the ALTER TABLE to fail
		mock.ExpectExec("ALTER TABLE user_workspaces ADD COLUMN IF NOT EXISTS permissions JSONB DEFAULT").
			WillReturnError(sql.ErrConnDone)

		err = migration.UpdateSystem(ctx, cfg, db)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to add permissions column to user_workspaces")
	})

	t.Run("Error - Owner permissions update fails", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		// Mock successful verification and ALTER TABLE
		mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM users").
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(5))
		mock.ExpectExec("ALTER TABLE user_workspaces ADD COLUMN IF NOT EXISTS permissions JSONB DEFAULT").
			WillReturnResult(sqlmock.NewResult(0, 0))

		// Mock the UPDATE for owners to fail
		mock.ExpectExec("UPDATE user_workspaces SET permissions = .+ WHERE role = 'owner'").
			WillReturnError(sql.ErrConnDone)

		err = migration.UpdateSystem(ctx, cfg, db)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to grant permissions to existing owners")
	})

	t.Run("Error - Member permissions update fails", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		// Mock successful verification, ALTER TABLE, and owner update
		mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM users").
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(5))
		mock.ExpectExec("ALTER TABLE user_workspaces ADD COLUMN IF NOT EXISTS permissions JSONB DEFAULT").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("UPDATE user_workspaces SET permissions = .+ WHERE role = 'owner'").
			WillReturnResult(sqlmock.NewResult(0, 2))

		// Mock the UPDATE for members to fail
		mock.ExpectExec("UPDATE user_workspaces SET permissions = .+ WHERE role = 'member'").
			WillReturnError(sql.ErrConnDone)

		err = migration.UpdateSystem(ctx, cfg, db)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to grant read permissions to existing members")
	})
}

func TestV4Migration_UpdateWorkspace(t *testing.T) {
	migration := &V4Migration{}
	ctx := context.Background()
	cfg := &config.Config{}
	workspace := &domain.Workspace{
		ID:   "test-workspace",
		Name: "Test Workspace",
	}

	t.Run("Success - Workspace verification passes", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		// Mock the verification query
		mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM contacts").
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(10))

		err = migration.UpdateWorkspace(ctx, cfg, workspace, db)
		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Error - Workspace verification fails", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		// Mock the verification query to fail
		mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM contacts").
			WillReturnError(sql.ErrConnDone)

		err = migration.UpdateWorkspace(ctx, cfg, workspace, db)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to verify workspace database connection")
	})
}

func TestV4Migration_Interface(t *testing.T) {
	migration := &V4Migration{}

	// Test that it implements the MajorMigrationInterface
	var _ MajorMigrationInterface = migration

	// Test GetMajorVersion
	assert.Equal(t, 4.0, migration.GetMajorVersion())
}

func TestV4Migration_Registration(t *testing.T) {
	// Test that the migration is registered (this tests the init function)
	registeredMigration, found := GetRegisteredMigration(4.0)
	assert.True(t, found, "V4Migration should be registered")
	assert.NotNil(t, registeredMigration, "V4Migration should be registered")

	if registeredMigration != nil {
		assert.Equal(t, 4.0, registeredMigration.GetMajorVersion())
		assert.True(t, registeredMigration.HasSystemUpdate())
		assert.False(t, registeredMigration.HasWorkspaceUpdate())
	}
}
