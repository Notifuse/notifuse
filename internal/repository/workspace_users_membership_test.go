package repository

import (
	"context"
	"fmt"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/repository/testutil"
)

func TestWorkspaceRepository_IsUserWorkspaceMember(t *testing.T) {
	db, mock, cleanup := testutil.SetupMockDB(t)
	defer cleanup()

	dbConfig := &config.DatabaseConfig{
		Prefix: "notifuse",
	}

	connMgr := newMockConnectionManager(db)
	repo := NewWorkspaceRepository(db, dbConfig, "secret-key", connMgr)
	userID := "user-123"
	workspaceID := "ws-123"

	t.Run("user is a member", func(t *testing.T) {
		rows := sqlmock.NewRows([]string{"count"}).AddRow(1)

		mock.ExpectQuery(`SELECT COUNT\(\*\) FROM user_workspaces WHERE user_id = \$1 AND workspace_id = \$2`).
			WithArgs(userID, workspaceID).
			WillReturnRows(rows)

		isMember, err := repo.IsUserWorkspaceMember(context.Background(), userID, workspaceID)
		require.NoError(t, err)
		assert.True(t, isMember)

		err = mock.ExpectationsWereMet()
		require.NoError(t, err)
	})

	t.Run("user is not a member", func(t *testing.T) {
		rows := sqlmock.NewRows([]string{"count"}).AddRow(0)

		mock.ExpectQuery(`SELECT COUNT\(\*\) FROM user_workspaces WHERE user_id = \$1 AND workspace_id = \$2`).
			WithArgs(userID, workspaceID).
			WillReturnRows(rows)

		isMember, err := repo.IsUserWorkspaceMember(context.Background(), userID, workspaceID)
		require.NoError(t, err)
		assert.False(t, isMember)

		err = mock.ExpectationsWereMet()
		require.NoError(t, err)
	})

	t.Run("database error", func(t *testing.T) {
		mock.ExpectQuery(`SELECT COUNT\(\*\) FROM user_workspaces WHERE user_id = \$1 AND workspace_id = \$2`).
			WithArgs(userID, workspaceID).
			WillReturnError(fmt.Errorf("database error"))

		isMember, err := repo.IsUserWorkspaceMember(context.Background(), userID, workspaceID)
		require.Error(t, err)
		assert.False(t, isMember)

		err = mock.ExpectationsWereMet()
		require.NoError(t, err)
	})
}
