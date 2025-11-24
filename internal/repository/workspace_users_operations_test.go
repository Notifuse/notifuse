package repository

import (
	"context"
	"fmt"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/repository/testutil"
)

func TestWorkspaceRepository_AddUserToWorkspace(t *testing.T) {
	db, mock, cleanup := testutil.SetupMockDB(t)
	defer cleanup()

	dbConfig := &config.DatabaseConfig{
		Prefix: "notifuse",
	}

	connMgr := newMockConnectionManager(db)
	repo := NewWorkspaceRepository(db, dbConfig, "secret-key", connMgr)

	userWorkspace := &domain.UserWorkspace{
		UserID:      "user123",
		WorkspaceID: "workspace123",
		Role:        "member",
		Permissions: domain.FullPermissions,
	}

	// Test success case
	mock.ExpectExec(`INSERT INTO user_workspaces.*VALUES.*`).
		WithArgs(
			userWorkspace.UserID,
			userWorkspace.WorkspaceID,
			userWorkspace.Role,
			userWorkspace.Permissions,
			sqlmock.AnyArg(), // created_at
			sqlmock.AnyArg(), // updated_at
		).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err := repo.AddUserToWorkspace(context.Background(), userWorkspace)
	require.NoError(t, err)

	// Test database error
	mock.ExpectExec(`INSERT INTO user_workspaces.*VALUES.*`).
		WithArgs(
			userWorkspace.UserID,
			userWorkspace.WorkspaceID,
			userWorkspace.Role,
			userWorkspace.Permissions,
			sqlmock.AnyArg(), // created_at
			sqlmock.AnyArg(), // updated_at
		).
		WillReturnError(fmt.Errorf("database error"))

	err = repo.AddUserToWorkspace(context.Background(), userWorkspace)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to add user to workspace")
}

func TestWorkspaceRepository_RemoveUserFromWorkspace(t *testing.T) {
	db, mock, cleanup := testutil.SetupMockDB(t)
	defer cleanup()

	dbConfig := &config.DatabaseConfig{
		Prefix: "notifuse",
	}

	connMgr := newMockConnectionManager(db)
	repo := NewWorkspaceRepository(db, dbConfig, "secret-key", connMgr)
	userID := "user123"
	workspaceID := "workspace123"

	// Test success case
	mock.ExpectExec(`DELETE FROM user_workspaces WHERE user_id = \$1 AND workspace_id = \$2`).
		WithArgs(userID, workspaceID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := repo.RemoveUserFromWorkspace(context.Background(), userID, workspaceID)
	require.NoError(t, err)

	// Test not found case
	mock.ExpectExec(`DELETE FROM user_workspaces WHERE user_id = \$1 AND workspace_id = \$2`).
		WithArgs("nonexistent", workspaceID).
		WillReturnResult(sqlmock.NewResult(0, 0))

	err = repo.RemoveUserFromWorkspace(context.Background(), "nonexistent", workspaceID)
	require.Error(t, err)

	// Test database error
	mock.ExpectExec(`DELETE FROM user_workspaces WHERE user_id = \$1 AND workspace_id = \$2`).
		WithArgs(userID, workspaceID).
		WillReturnError(fmt.Errorf("database error"))

	err = repo.RemoveUserFromWorkspace(context.Background(), userID, workspaceID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to remove user from workspace")

	// Test error getting affected rows
	mock.ExpectExec(`DELETE FROM user_workspaces WHERE user_id = \$1 AND workspace_id = \$2`).
		WithArgs(userID, workspaceID).
		WillReturnResult(sqlmock.NewErrorResult(fmt.Errorf("rows affected error")))

	err = repo.RemoveUserFromWorkspace(context.Background(), userID, workspaceID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get affected rows")
}

func TestWorkspaceRepository_UpdateUserWorkspacePermissions(t *testing.T) {
	// Test workspaceRepository.UpdateUserWorkspacePermissions - this was at 0% coverage
	db, mock, cleanup := testutil.SetupMockDB(t)
	defer cleanup()

	dbConfig := &config.DatabaseConfig{
		Prefix: "notifuse",
	}

	connMgr := newMockConnectionManager(db)
	repo := NewWorkspaceRepository(db, dbConfig, "secret-key", connMgr)

	userWorkspace := &domain.UserWorkspace{
		UserID:      "user123",
		WorkspaceID: "workspace123",
		Role:        "member",
		Permissions: domain.UserPermissions{
			domain.PermissionResourceContacts: domain.ResourcePermissions{Read: true, Write: true},
		},
	}

	t.Run("Success - Updates permissions", func(t *testing.T) {
		mock.ExpectExec(`UPDATE user_workspaces SET permissions = \$1, updated_at = \$2 WHERE user_id = \$3 AND workspace_id = \$4`).
			WithArgs(
				userWorkspace.Permissions,
				sqlmock.AnyArg(), // updated_at
				userWorkspace.UserID,
				userWorkspace.WorkspaceID,
			).
			WillReturnResult(sqlmock.NewResult(0, 1))

		err := repo.UpdateUserWorkspacePermissions(context.Background(), userWorkspace)
		require.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Error - Database error", func(t *testing.T) {
		mock.ExpectExec(`UPDATE user_workspaces SET permissions = \$1, updated_at = \$2 WHERE user_id = \$3 AND workspace_id = \$4`).
			WithArgs(
				userWorkspace.Permissions,
				sqlmock.AnyArg(),
				userWorkspace.UserID,
				userWorkspace.WorkspaceID,
			).
			WillReturnError(fmt.Errorf("database error"))

		err := repo.UpdateUserWorkspacePermissions(context.Background(), userWorkspace)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to update user workspace permissions")
	})
}
