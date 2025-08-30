package repository

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
)

func TestWorkspaceRepository_CreateInvitation(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	repo := &workspaceRepository{
		systemDB: db,
		dbConfig: &config.DatabaseConfig{
			Host:     "localhost",
			Port:     5432,
			User:     "postgres",
			Password: "postgres",
			DBName:   "notifuse",
			Prefix:   "nf",
		},
	}

	// Create a sample invitation
	now := time.Now().Truncate(time.Second)
	expiresAt := now.Add(24 * time.Hour).Truncate(time.Second)
	invitation := &domain.WorkspaceInvitation{
		ID:          "inv-123",
		WorkspaceID: "ws-123",
		InviterID:   "user-123",
		Email:       "test@example.com",
		ExpiresAt:   expiresAt,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	t.Run("successful creation", func(t *testing.T) {
		// Set up expectations
		mock.ExpectExec(`INSERT INTO workspace_invitations`).
			WithArgs(invitation.ID, invitation.WorkspaceID, invitation.InviterID,
				invitation.Email, invitation.ExpiresAt, invitation.CreatedAt, invitation.UpdatedAt).
			WillReturnResult(sqlmock.NewResult(1, 1))

		// Call the method
		err := repo.CreateInvitation(context.Background(), invitation)
		require.NoError(t, err)

		// Verify expectations
		err = mock.ExpectationsWereMet()
		require.NoError(t, err)
	})

	t.Run("database error", func(t *testing.T) {
		// Set up expectations for error
		mock.ExpectExec(`INSERT INTO workspace_invitations`).
			WithArgs(invitation.ID, invitation.WorkspaceID, invitation.InviterID,
				invitation.Email, invitation.ExpiresAt, invitation.CreatedAt, invitation.UpdatedAt).
			WillReturnError(fmt.Errorf("database error"))

		// Call the method
		err := repo.CreateInvitation(context.Background(), invitation)
		require.Error(t, err)

		// Verify expectations
		err = mock.ExpectationsWereMet()
		require.NoError(t, err)
	})
}

func TestWorkspaceRepository_GetInvitationByID(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	repo := &workspaceRepository{
		systemDB: db,
		dbConfig: &config.DatabaseConfig{},
	}

	// Sample invitation data
	invitationID := "inv-123"
	workspaceID := "ws-123"
	inviterID := "user-123"
	email := "test@example.com"
	now := time.Now().Truncate(time.Second)
	expiresAt := now.Add(24 * time.Hour).Truncate(time.Second)

	t.Run("invitation found", func(t *testing.T) {
		rows := sqlmock.NewRows([]string{"id", "workspace_id", "inviter_id", "email", "expires_at", "created_at", "updated_at"}).
			AddRow(invitationID, workspaceID, inviterID, email, expiresAt, now, now)

		mock.ExpectQuery(`SELECT id, workspace_id, inviter_id, email, expires_at, created_at, updated_at FROM workspace_invitations WHERE id = \$1`).
			WithArgs(invitationID).
			WillReturnRows(rows)

		invitation, err := repo.GetInvitationByID(context.Background(), invitationID)
		require.NoError(t, err)
		require.NotNil(t, invitation)
		assert.Equal(t, invitationID, invitation.ID)
		assert.Equal(t, workspaceID, invitation.WorkspaceID)
		assert.Equal(t, inviterID, invitation.InviterID)
		assert.Equal(t, email, invitation.Email)
		assert.Equal(t, expiresAt.UTC(), invitation.ExpiresAt.UTC())

		err = mock.ExpectationsWereMet()
		require.NoError(t, err)
	})

	t.Run("invitation not found", func(t *testing.T) {
		mock.ExpectQuery(`SELECT .+ FROM workspace_invitations WHERE id = \$1`).
			WithArgs("non-existent-id").
			WillReturnError(sql.ErrNoRows)

		invitation, err := repo.GetInvitationByID(context.Background(), "non-existent-id")
		require.Error(t, err)
		assert.Nil(t, invitation)
		assert.Contains(t, err.Error(), "invitation not found")

		err = mock.ExpectationsWereMet()
		require.NoError(t, err)
	})

	t.Run("database error", func(t *testing.T) {
		mock.ExpectQuery(`SELECT .+ FROM workspace_invitations WHERE id = \$1`).
			WithArgs(invitationID).
			WillReturnError(fmt.Errorf("database error"))

		invitation, err := repo.GetInvitationByID(context.Background(), invitationID)
		require.Error(t, err)
		assert.Nil(t, invitation)

		err = mock.ExpectationsWereMet()
		require.NoError(t, err)
	})
}

func TestWorkspaceRepository_GetInvitationByEmail(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	repo := &workspaceRepository{
		systemDB: db,
		dbConfig: &config.DatabaseConfig{},
	}

	// Sample invitation data
	invitationID := "inv-123"
	workspaceID := "ws-123"
	inviterID := "user-123"
	email := "test@example.com"
	now := time.Now().Truncate(time.Second)
	expiresAt := now.Add(24 * time.Hour).Truncate(time.Second)

	t.Run("invitation found", func(t *testing.T) {
		rows := sqlmock.NewRows([]string{"id", "workspace_id", "inviter_id", "email", "expires_at", "created_at", "updated_at"}).
			AddRow(invitationID, workspaceID, inviterID, email, expiresAt, now, now)

		mock.ExpectQuery(`SELECT .+ FROM workspace_invitations WHERE workspace_id = \$1 AND email = \$2 ORDER BY created_at DESC LIMIT 1`).
			WithArgs(workspaceID, email).
			WillReturnRows(rows)

		invitation, err := repo.GetInvitationByEmail(context.Background(), workspaceID, email)
		require.NoError(t, err)
		require.NotNil(t, invitation)
		assert.Equal(t, invitationID, invitation.ID)
		assert.Equal(t, workspaceID, invitation.WorkspaceID)
		assert.Equal(t, inviterID, invitation.InviterID)
		assert.Equal(t, email, invitation.Email)
		assert.Equal(t, expiresAt.UTC(), invitation.ExpiresAt.UTC())

		err = mock.ExpectationsWereMet()
		require.NoError(t, err)
	})

	t.Run("invitation not found", func(t *testing.T) {
		mock.ExpectQuery(`SELECT .+ FROM workspace_invitations WHERE workspace_id = \$1 AND email = \$2 ORDER BY created_at DESC LIMIT 1`).
			WithArgs(workspaceID, "nonexistent@example.com").
			WillReturnError(sql.ErrNoRows)

		invitation, err := repo.GetInvitationByEmail(context.Background(), workspaceID, "nonexistent@example.com")
		require.Error(t, err)
		assert.Nil(t, invitation)
		assert.Contains(t, err.Error(), "invitation not found")

		err = mock.ExpectationsWereMet()
		require.NoError(t, err)
	})

	t.Run("database error", func(t *testing.T) {
		mock.ExpectQuery(`SELECT .+ FROM workspace_invitations WHERE workspace_id = \$1 AND email = \$2 ORDER BY created_at DESC LIMIT 1`).
			WithArgs(workspaceID, email).
			WillReturnError(fmt.Errorf("database error"))

		invitation, err := repo.GetInvitationByEmail(context.Background(), workspaceID, email)
		require.Error(t, err)
		assert.Nil(t, invitation)

		err = mock.ExpectationsWereMet()
		require.NoError(t, err)
	})
}

func TestWorkspaceRepository_DeleteInvitation(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	repo := &workspaceRepository{
		systemDB: db,
		dbConfig: &config.DatabaseConfig{},
	}

	invitationID := "inv-123"

	t.Run("successful deletion", func(t *testing.T) {
		mock.ExpectExec(`DELETE FROM workspace_invitations WHERE id = \$1`).
			WithArgs(invitationID).
			WillReturnResult(sqlmock.NewResult(0, 1)) // 1 row affected

		err := repo.DeleteInvitation(context.Background(), invitationID)
		require.NoError(t, err)

		err = mock.ExpectationsWereMet()
		require.NoError(t, err)
	})

	t.Run("invitation not found", func(t *testing.T) {
		mock.ExpectExec(`DELETE FROM workspace_invitations WHERE id = \$1`).
			WithArgs("non-existent-id").
			WillReturnResult(sqlmock.NewResult(0, 0)) // 0 rows affected

		err := repo.DeleteInvitation(context.Background(), "non-existent-id")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invitation not found")

		err = mock.ExpectationsWereMet()
		require.NoError(t, err)
	})

	t.Run("database error on exec", func(t *testing.T) {
		mock.ExpectExec(`DELETE FROM workspace_invitations WHERE id = \$1`).
			WithArgs(invitationID).
			WillReturnError(fmt.Errorf("database error"))

		err := repo.DeleteInvitation(context.Background(), invitationID)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to delete invitation")

		err = mock.ExpectationsWereMet()
		require.NoError(t, err)
	})

	t.Run("error getting rows affected", func(t *testing.T) {
		// Create a result that will return an error when RowsAffected is called
		mock.ExpectExec(`DELETE FROM workspace_invitations WHERE id = \$1`).
			WithArgs(invitationID).
			WillReturnResult(sqlmock.NewErrorResult(fmt.Errorf("rows affected error")))

		err := repo.DeleteInvitation(context.Background(), invitationID)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get rows affected")

		err = mock.ExpectationsWereMet()
		require.NoError(t, err)
	})
}
