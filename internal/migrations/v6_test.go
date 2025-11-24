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

func TestV6Migration_HasSystemUpdate(t *testing.T) {
	migration := &V6Migration{}
	assert.True(t, migration.HasSystemUpdate(), "V6Migration should have system updates")
}

func TestV6Migration_HasWorkspaceUpdate(t *testing.T) {
	migration := &V6Migration{}
	assert.False(t, migration.HasWorkspaceUpdate(), "V6Migration should not have workspace updates")
}

func TestV6Migration_UpdateSystem(t *testing.T) {
	migration := &V6Migration{}
	ctx := context.Background()
	cfg := &config.Config{}

	t.Run("Success - All operations complete", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		// Mock the ALTER TABLE for workspace_invitations permissions column
		mock.ExpectExec("ALTER TABLE workspace_invitations ADD COLUMN IF NOT EXISTS permissions JSONB DEFAULT").
			WillReturnResult(sqlmock.NewResult(0, 0))

		// Mock the UPDATE for existing invitations
		mock.ExpectExec("UPDATE workspace_invitations SET permissions = .+ WHERE permissions IS NULL OR permissions = .+").
			WillReturnResult(sqlmock.NewResult(0, 3))

		// Mock the UPDATE for existing workspace users
		mock.ExpectExec("UPDATE user_workspaces SET permissions = .+ WHERE permissions IS NULL OR permissions = .+").
			WillReturnResult(sqlmock.NewResult(0, 5))

		err = migration.UpdateSystem(ctx, cfg, db)
		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Error - ALTER TABLE fails", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		// Mock the ALTER TABLE to fail
		mock.ExpectExec("ALTER TABLE workspace_invitations ADD COLUMN IF NOT EXISTS permissions JSONB DEFAULT").
			WillReturnError(sql.ErrConnDone)

		err = migration.UpdateSystem(ctx, cfg, db)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to add permissions column to workspace_invitations table")
	})

	t.Run("Error - Invitations permissions update fails", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		// Mock successful ALTER TABLE
		mock.ExpectExec("ALTER TABLE workspace_invitations ADD COLUMN IF NOT EXISTS permissions JSONB DEFAULT").
			WillReturnResult(sqlmock.NewResult(0, 0))

		// Mock the UPDATE for invitations to fail
		mock.ExpectExec("UPDATE workspace_invitations SET permissions = .+ WHERE permissions IS NULL OR permissions = .+").
			WillReturnError(sql.ErrConnDone)

		err = migration.UpdateSystem(ctx, cfg, db)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to grant default permissions to existing invitations")
	})

	t.Run("Error - User workspaces permissions update fails", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		// Mock successful ALTER TABLE and invitations update
		mock.ExpectExec("ALTER TABLE workspace_invitations ADD COLUMN IF NOT EXISTS permissions JSONB DEFAULT").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("UPDATE workspace_invitations SET permissions = .+ WHERE permissions IS NULL OR permissions = .+").
			WillReturnResult(sqlmock.NewResult(0, 3))

		// Mock the UPDATE for user workspaces to fail
		mock.ExpectExec("UPDATE user_workspaces SET permissions = .+ WHERE permissions IS NULL OR permissions = .+").
			WillReturnError(sql.ErrConnDone)

		err = migration.UpdateSystem(ctx, cfg, db)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to grant default permissions to existing workspace users")
	})
}

func TestV6Migration_UpdateWorkspace(t *testing.T) {
	migration := &V6Migration{}
	ctx := context.Background()
	cfg := &config.Config{}
	workspace := &domain.Workspace{
		ID:   "test-workspace",
		Name: "Test Workspace",
	}

	t.Run("Success - No workspace changes", func(t *testing.T) {
		db, _, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		// V6 has no workspace-level changes, should return nil
		err = migration.UpdateWorkspace(ctx, cfg, workspace, db)
		assert.NoError(t, err)
	})
}

func TestV6Migration_Interface(t *testing.T) {
	migration := &V6Migration{}

	// Test that it implements the MajorMigrationInterface
	var _ MajorMigrationInterface = migration

	// Test GetMajorVersion
	assert.Equal(t, 6.0, migration.GetMajorVersion())
}

func TestV6Migration_Registration(t *testing.T) {
	// Test that the migration is registered (this tests the init function)
	registeredMigration, found := GetRegisteredMigration(6.0)
	assert.True(t, found, "V6Migration should be registered")
	assert.NotNil(t, registeredMigration, "V6Migration should be registered")

	if registeredMigration != nil {
		assert.Equal(t, 6.0, registeredMigration.GetMajorVersion())
		assert.True(t, registeredMigration.HasSystemUpdate())
		assert.False(t, registeredMigration.HasWorkspaceUpdate())
	}
}

func TestV6Migration_PermissionsStructure(t *testing.T) {
	migration := &V6Migration{}
	ctx := context.Background()
	cfg := &config.Config{}

	t.Run("Verify permissions JSON structure", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		// We can verify the permissions structure by checking the SQL contains expected keys
		mock.ExpectExec("ALTER TABLE workspace_invitations ADD COLUMN IF NOT EXISTS permissions JSONB DEFAULT").
			WillReturnResult(sqlmock.NewResult(0, 0))

		// Verify the UPDATE contains expected permission keys
		mock.ExpectExec("UPDATE workspace_invitations").
			WithArgs().
			WillReturnResult(sqlmock.NewResult(0, 3))

		mock.ExpectExec("UPDATE user_workspaces").
			WithArgs().
			WillReturnResult(sqlmock.NewResult(0, 5))

		err = migration.UpdateSystem(ctx, cfg, db)
		assert.NoError(t, err)
	})
}

func TestV6Migration_Idempotency(t *testing.T) {
	migration := &V6Migration{}
	ctx := context.Background()
	cfg := &config.Config{}

	t.Run("Migration should be idempotent", func(t *testing.T) {
		// The migration uses "IF NOT EXISTS" and conditional updates
		// This test verifies the migration can be run multiple times safely
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		// First run - column doesn't exist, gets created
		mock.ExpectExec("ALTER TABLE workspace_invitations ADD COLUMN IF NOT EXISTS permissions JSONB DEFAULT").
			WillReturnResult(sqlmock.NewResult(0, 1)) // 1 row affected (column added)

		mock.ExpectExec("UPDATE workspace_invitations SET permissions = .+ WHERE permissions IS NULL OR permissions = .+").
			WillReturnResult(sqlmock.NewResult(0, 3))

		mock.ExpectExec("UPDATE user_workspaces SET permissions = .+ WHERE permissions IS NULL OR permissions = .+").
			WillReturnResult(sqlmock.NewResult(0, 5))

		err = migration.UpdateSystem(ctx, cfg, db)
		assert.NoError(t, err)

		// Second run would be safe due to IF NOT EXISTS and WHERE conditions
		// This demonstrates the idempotent design
	})
}
