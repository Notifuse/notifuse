package repository

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/repository/testutil"
)

func TestWorkspaceRepository_GetUserWorkspaces(t *testing.T) {
	db, mock, cleanup := testutil.SetupMockDB(t)
	defer cleanup()

	dbConfig := &config.DatabaseConfig{
		Prefix: "notifuse",
	}

	connMgr := newMockConnectionManager(db)
	repo := NewWorkspaceRepository(db, dbConfig, "secret-key", connMgr)
	userID := "user123"

	// Test success case
	now := time.Now().Truncate(time.Second)

	permissions := domain.UserPermissions{
		domain.PermissionResourceContacts: domain.ResourcePermissions{Read: true, Write: true},
	}
	rows := sqlmock.NewRows([]string{"user_id", "workspace_id", "role", "permissions", "created_at", "updated_at"}).
		AddRow(userID, "workspace1", "owner", permissions, now, now).
		AddRow(userID, "workspace2", "member", permissions, now, now)

	mock.ExpectQuery(`SELECT user_id, workspace_id, role, permissions, created_at, updated_at FROM user_workspaces WHERE user_id = \$1`).
		WithArgs(userID).
		WillReturnRows(rows)

	userWorkspaces, err := repo.GetUserWorkspaces(context.Background(), userID)
	require.NoError(t, err)
	assert.Len(t, userWorkspaces, 2)
	assert.Equal(t, "workspace1", userWorkspaces[0].WorkspaceID)
	assert.Equal(t, "owner", userWorkspaces[0].Role)
	assert.Equal(t, "workspace2", userWorkspaces[1].WorkspaceID)
	assert.Equal(t, "member", userWorkspaces[1].Role)

	// Test database query error
	mock.ExpectQuery(`SELECT user_id, workspace_id, role, permissions, created_at, updated_at FROM user_workspaces WHERE user_id = \$1`).
		WithArgs(userID).
		WillReturnError(fmt.Errorf("database error"))

	_, err = repo.GetUserWorkspaces(context.Background(), userID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get user workspaces")

	// Test empty result
	emptyRows := sqlmock.NewRows([]string{"user_id", "workspace_id", "role", "permissions", "created_at", "updated_at"})
	mock.ExpectQuery(`SELECT user_id, workspace_id, role, permissions, created_at, updated_at FROM user_workspaces WHERE user_id = \$1`).
		WithArgs(userID).
		WillReturnRows(emptyRows)

	emptyWorkspaces, err := repo.GetUserWorkspaces(context.Background(), userID)
	require.NoError(t, err)
	assert.Empty(t, emptyWorkspaces)
}

func TestWorkspaceRepository_GetUserWorkspace(t *testing.T) {
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
	now := time.Now().Truncate(time.Second)

	permissions := domain.UserPermissions{
		domain.PermissionResourceLists: domain.ResourcePermissions{Read: true, Write: true},
	}
	rows := sqlmock.NewRows([]string{"user_id", "workspace_id", "role", "permissions", "created_at", "updated_at"}).
		AddRow(userID, workspaceID, "owner", permissions, now, now)

	mock.ExpectQuery(`SELECT user_id, workspace_id, role, permissions, created_at, updated_at FROM user_workspaces WHERE user_id = \$1 AND workspace_id = \$2`).
		WithArgs(userID, workspaceID).
		WillReturnRows(rows)

	userWorkspace, err := repo.GetUserWorkspace(context.Background(), userID, workspaceID)
	require.NoError(t, err)
	assert.Equal(t, userID, userWorkspace.UserID)
	assert.Equal(t, workspaceID, userWorkspace.WorkspaceID)
	assert.Equal(t, "owner", userWorkspace.Role)

	// Test not found case
	mock.ExpectQuery(`SELECT user_id, workspace_id, role, created_at, updated_at FROM user_workspaces WHERE user_id = \$1 AND workspace_id = \$2`).
		WithArgs("nonexistent", workspaceID).
		WillReturnError(errors.New("no rows"))

	_, err = repo.GetUserWorkspace(context.Background(), "nonexistent", workspaceID)
	require.Error(t, err)

	// Test database query error
	mock.ExpectQuery(`SELECT user_id, workspace_id, role, created_at, updated_at FROM user_workspaces WHERE user_id = \$1 AND workspace_id = \$2`).
		WithArgs(userID, workspaceID).
		WillReturnError(fmt.Errorf("database error"))

	_, err = repo.GetUserWorkspace(context.Background(), userID, workspaceID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get user workspace")
}
