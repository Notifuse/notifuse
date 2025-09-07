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

func TestWorkspaceRepository_GetWorkspaceUsersWithEmail(t *testing.T) {
	db, mock, cleanup := testutil.SetupMockDB(t)
	defer cleanup()

	repo := &workspaceRepository{
		systemDB: db,
		dbConfig: &config.DatabaseConfig{},
	}

	// Sample data
	workspaceID := "ws-123"
	now := time.Now().Truncate(time.Second)
	users := []struct {
		userID    string
		email     string
		role      string
		userType  domain.UserType
		createdAt time.Time
		updatedAt time.Time
	}{
		{
			userID:    "user-1",
			email:     "user1@example.com",
			role:      "admin",
			userType:  domain.UserTypeUser,
			createdAt: now,
			updatedAt: now,
		},
		{
			userID:    "user-2",
			email:     "user2@example.com",
			role:      "member",
			userType:  domain.UserTypeUser,
			createdAt: now,
			updatedAt: now,
		},
	}

	t.Run("successful retrieval", func(t *testing.T) {
		// Set up permissions
		permissions := domain.UserPermissions{
			domain.PermissionResourceContacts: domain.ResourcePermissions{Read: true, Write: true},
		}

		// Set up expectations
		rows := sqlmock.NewRows([]string{"user_id", "workspace_id", "role", "permissions", "created_at", "updated_at", "email", "type"}).
			AddRow(users[0].userID, workspaceID, users[0].role, permissions, users[0].createdAt, users[0].updatedAt, users[0].email, users[0].userType).
			AddRow(users[1].userID, workspaceID, users[1].role, permissions, users[1].createdAt, users[1].updatedAt, users[1].email, users[1].userType)

		mock.ExpectQuery(`SELECT uw.user_id, uw.workspace_id, uw.role, uw.permissions, uw.created_at, uw.updated_at, u.email, u.type FROM user_workspaces uw JOIN users u ON uw.user_id = u.id WHERE uw.workspace_id = \$1`).
			WithArgs(workspaceID).
			WillReturnRows(rows)

		// Call the method
		result, err := repo.GetWorkspaceUsersWithEmail(context.Background(), workspaceID)
		require.NoError(t, err)
		require.Len(t, result, 2)

		// Verify results
		assert.Equal(t, users[0].userID, result[0].UserID)
		assert.Equal(t, users[0].email, result[0].Email)
		assert.Equal(t, users[0].role, result[0].Role)
		assert.Equal(t, users[0].userType, result[0].Type)
		assert.Equal(t, users[1].userID, result[1].UserID)
		assert.Equal(t, users[1].email, result[1].Email)
		assert.Equal(t, users[1].role, result[1].Role)
		assert.Equal(t, users[1].userType, result[1].Type)

		// Verify expectations
		err = mock.ExpectationsWereMet()
		require.NoError(t, err)
	})

	t.Run("empty result", func(t *testing.T) {
		// Set up expectations for empty result
		rows := sqlmock.NewRows([]string{"user_id", "workspace_id", "role", "permissions", "created_at", "updated_at", "email", "type"})
		mock.ExpectQuery(`SELECT uw.user_id, uw.workspace_id, uw.role, uw.permissions, uw.created_at, uw.updated_at, u.email, u.type FROM user_workspaces uw JOIN users u ON uw.user_id = u.id WHERE uw.workspace_id = \$1`).
			WithArgs("empty-workspace").
			WillReturnRows(rows)

		// Call the method
		result, err := repo.GetWorkspaceUsersWithEmail(context.Background(), "empty-workspace")
		require.NoError(t, err)
		require.Empty(t, result)

		// Verify expectations
		err = mock.ExpectationsWereMet()
		require.NoError(t, err)
	})

	t.Run("database error", func(t *testing.T) {
		// Set up expectations for database error
		mock.ExpectQuery(`SELECT uw.user_id, uw.workspace_id, uw.role, uw.permissions, uw.created_at, uw.updated_at, u.email, u.type FROM user_workspaces uw JOIN users u ON uw.user_id = u.id WHERE uw.workspace_id = \$1`).
			WithArgs("error-workspace").
			WillReturnError(fmt.Errorf("database error"))

		// Call the method
		result, err := repo.GetWorkspaceUsersWithEmail(context.Background(), "error-workspace")
		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "failed to get workspace users with email")

		// Verify expectations
		err = mock.ExpectationsWereMet()
		require.NoError(t, err)
	})

	t.Run("scan error", func(t *testing.T) {
		// Set up expectations for scan error
		permissions := domain.UserPermissions{}
		rows := sqlmock.NewRows([]string{"user_id", "workspace_id", "role", "permissions", "created_at", "updated_at", "email", "type"}).
			AddRow(nil, workspaceID, "admin", permissions, now, now, "user@example.com", domain.UserTypeUser) // Invalid user_id (nil)

		mock.ExpectQuery(`SELECT uw.user_id, uw.workspace_id, uw.role, uw.permissions, uw.created_at, uw.updated_at, u.email, u.type FROM user_workspaces uw JOIN users u ON uw.user_id = u.id WHERE uw.workspace_id = \$1`).
			WithArgs(workspaceID).
			WillReturnRows(rows)

		// Call the method
		result, err := repo.GetWorkspaceUsersWithEmail(context.Background(), workspaceID)
		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "failed to scan user workspace with email")

		// Verify expectations
		err = mock.ExpectationsWereMet()
		require.NoError(t, err)
	})

	t.Run("row iteration error", func(t *testing.T) {
		// Set up expectations for row iteration error
		permissions := domain.UserPermissions{}
		rows := sqlmock.NewRows([]string{"user_id", "workspace_id", "role", "permissions", "created_at", "updated_at", "email", "type"}).
			AddRow(users[0].userID, workspaceID, users[0].role, permissions, users[0].createdAt, users[0].updatedAt, users[0].email, users[0].userType).
			AddRow(users[1].userID, workspaceID, users[1].role, permissions, users[1].createdAt, users[1].updatedAt, users[1].email, users[1].userType)

		mock.ExpectQuery(`SELECT uw.user_id, uw.workspace_id, uw.role, uw.permissions, uw.created_at, uw.updated_at, u.email, u.type FROM user_workspaces uw JOIN users u ON uw.user_id = u.id WHERE uw.workspace_id = \$1`).
			WithArgs(workspaceID).
			WillReturnRows(rows).
			WillReturnError(errors.New("row iteration error"))

		// Call the method
		result, err := repo.GetWorkspaceUsersWithEmail(context.Background(), workspaceID)
		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "failed to get workspace users with email")

		// Verify expectations
		err = mock.ExpectationsWereMet()
		require.NoError(t, err)
	})
}
