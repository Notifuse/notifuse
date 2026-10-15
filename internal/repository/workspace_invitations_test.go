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
	defer func() { _ = db.Close() }()

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
		Permissions: domain.UserPermissions{
			domain.PermissionResourceContacts: domain.ResourcePermissions{Read: true, Write: true},
		},
		ExpiresAt: expiresAt,
		CreatedAt: now,
		UpdatedAt: now,
	}

	t.Run("successful creation", func(t *testing.T) {
		// Set up expectations for upsert query
		mock.ExpectQuery(`INSERT INTO workspace_invitations .* ON CONFLICT .* RETURNING id, created_at`).
			WithArgs(invitation.ID, invitation.WorkspaceID, invitation.InviterID,
				invitation.Email, invitation.Permissions, invitation.ExpiresAt, invitation.CreatedAt, invitation.UpdatedAt).
			WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow(invitation.ID, invitation.CreatedAt))

		// Call the method
		err := repo.CreateInvitation(context.Background(), invitation)
		require.NoError(t, err)

		// Verify expectations
		err = mock.ExpectationsWereMet()
		require.NoError(t, err)
	})

	t.Run("successful update of existing invitation", func(t *testing.T) {
		// Create a new invitation with updated details
		updatedInvitation := &domain.WorkspaceInvitation{
			ID:          "new-inv-456", // Different ID for new invitation
			WorkspaceID: invitation.WorkspaceID,
			InviterID:   "new-inviter-456", // Different inviter
			Email:       invitation.Email,  // Same email
			Permissions: domain.UserPermissions{
				domain.PermissionResourceContacts: domain.ResourcePermissions{Read: true, Write: false},
			},
			ExpiresAt: invitation.ExpiresAt.Add(24 * time.Hour), // Extended expiry
			CreatedAt: now,
			UpdatedAt: now.Add(time.Hour), // Updated timestamp
		}

		// The conflict keeps the existing row, so RETURNING hands back its id and
		// created_at rather than the ones passed in.
		originalCreatedAt := now.Add(-48 * time.Hour)
		mock.ExpectQuery(`INSERT INTO workspace_invitations .* ON CONFLICT .* RETURNING id, created_at`).
			WithArgs(updatedInvitation.ID, updatedInvitation.WorkspaceID, updatedInvitation.InviterID,
				updatedInvitation.Email, updatedInvitation.Permissions, updatedInvitation.ExpiresAt, updatedInvitation.CreatedAt, updatedInvitation.UpdatedAt).
			WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow(invitation.ID, originalCreatedAt))

		// Call the method
		err := repo.CreateInvitation(context.Background(), updatedInvitation)
		require.NoError(t, err)

		// The invitation link is signed with this id, so it must name the surviving row.
		assert.Equal(t, "inv-123", updatedInvitation.ID)
		assert.Equal(t, originalCreatedAt, updatedInvitation.CreatedAt)

		// Verify expectations
		err = mock.ExpectationsWereMet()
		require.NoError(t, err)
	})

	t.Run("database error", func(t *testing.T) {
		// Set up expectations for error
		mock.ExpectQuery(`INSERT INTO workspace_invitations .* ON CONFLICT .* RETURNING id, created_at`).
			WithArgs(invitation.ID, invitation.WorkspaceID, invitation.InviterID,
				invitation.Email, invitation.Permissions, invitation.ExpiresAt, invitation.CreatedAt, invitation.UpdatedAt).
			WillReturnError(fmt.Errorf("database error"))

		// Call the method
		err := repo.CreateInvitation(context.Background(), invitation)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create or update invitation")

		// Verify expectations
		err = mock.ExpectationsWereMet()
		require.NoError(t, err)
	})
}

func TestWorkspaceRepository_GetInvitationByID(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

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
		permissions := domain.UserPermissions{
			domain.PermissionResourceContacts: domain.ResourcePermissions{Read: true, Write: true},
		}
		rows := sqlmock.NewRows([]string{"id", "workspace_id", "inviter_id", "email", "permissions", "expires_at", "created_at", "updated_at"}).
			AddRow(invitationID, workspaceID, inviterID, email, permissions, expiresAt, now, now)

		mock.ExpectQuery(`SELECT id, workspace_id, inviter_id, email, permissions, expires_at, created_at, updated_at FROM workspace_invitations WHERE id = \$1`).
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
		// Typed, so a caller can tell a missing invitation from a database failure.
		assert.ErrorIs(t, err, domain.ErrInvitationNotFound)

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
	defer func() { _ = db.Close() }()

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
		permissions := domain.UserPermissions{
			domain.PermissionResourceContacts: domain.ResourcePermissions{Read: true, Write: true},
		}
		rows := sqlmock.NewRows([]string{"id", "workspace_id", "inviter_id", "email", "permissions", "expires_at", "created_at", "updated_at"}).
			AddRow(invitationID, workspaceID, inviterID, email, permissions, expiresAt, now, now)

		mock.ExpectQuery(`SELECT id, workspace_id, inviter_id, email, permissions, expires_at, created_at, updated_at FROM workspace_invitations WHERE workspace_id = \$1 AND LOWER\(email\) = LOWER\(\$2\) ORDER BY updated_at DESC LIMIT 1`).
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
		mock.ExpectQuery(`SELECT .+ FROM workspace_invitations WHERE workspace_id = \$1 AND LOWER\(email\) = LOWER\(\$2\) ORDER BY updated_at DESC LIMIT 1`).
			WithArgs(workspaceID, "nonexistent@example.com").
			WillReturnError(sql.ErrNoRows)

		invitation, err := repo.GetInvitationByEmail(context.Background(), workspaceID, "nonexistent@example.com")
		require.Error(t, err)
		assert.Nil(t, invitation)
		assert.Contains(t, err.Error(), "invitation not found")
		assert.ErrorIs(t, err, domain.ErrInvitationNotFound)

		err = mock.ExpectationsWereMet()
		require.NoError(t, err)
	})

	t.Run("database error", func(t *testing.T) {
		mock.ExpectQuery(`SELECT .+ FROM workspace_invitations WHERE workspace_id = \$1 AND LOWER\(email\) = LOWER\(\$2\) ORDER BY updated_at DESC LIMIT 1`).
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
	defer func() { _ = db.Close() }()

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

func TestWorkspaceRepository_DeleteInvitationByEmail(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	repo := &workspaceRepository{systemDB: db}

	// Case-insensitive: invitations keep the address as the owner typed it.
	t.Run("deletes the address's invitations whatever their case", func(t *testing.T) {
		mock.ExpectExec(`DELETE FROM workspace_invitations WHERE workspace_id = \$1 AND LOWER\(email\) = LOWER\(\$2\)`).
			WithArgs("ws-123", "test@example.com").
			WillReturnResult(sqlmock.NewResult(0, 1))

		require.NoError(t, repo.DeleteInvitationByEmail(context.Background(), "ws-123", "test@example.com"))
		require.NoError(t, mock.ExpectationsWereMet())
	})

	// Callers clear the invitation of an address they have just seated another way; most
	// such addresses never had one.
	t.Run("no invitation for the address is not an error", func(t *testing.T) {
		mock.ExpectExec(`DELETE FROM workspace_invitations`).
			WithArgs("ws-123", "nobody@example.com").
			WillReturnResult(sqlmock.NewResult(0, 0))

		require.NoError(t, repo.DeleteInvitationByEmail(context.Background(), "ws-123", "nobody@example.com"))
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("database error", func(t *testing.T) {
		mock.ExpectExec(`DELETE FROM workspace_invitations`).
			WithArgs("ws-123", "test@example.com").
			WillReturnError(fmt.Errorf("database error"))

		err := repo.DeleteInvitationByEmail(context.Background(), "ws-123", "test@example.com")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to delete invitation by email")
		require.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestWorkspaceRepository_ListPendingInvitationsForEmail(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	repo := &workspaceRepository{systemDB: db}
	columns := []string{"id", "workspace_id", "workspace_name", "inviter_name", "expires_at"}
	expiresAt := time.Now().Add(24 * time.Hour).Truncate(time.Second)

	// One invitation per workspace (the newest), live ones only, matched case-insensitively
	// against the address as typed, expiry read at UTC like the seat count.
	t.Run("lists live invitations with workspace and inviter names", func(t *testing.T) {
		mock.ExpectQuery(`SELECT DISTINCT ON \(i\.workspace_id\)(.|\n)*`+
			`JOIN workspaces w ON w\.id = i\.workspace_id(.|\n)*`+
			`LEFT JOIN users u ON u\.id = i\.inviter_id(.|\n)*`+
			`WHERE LOWER\(i\.email\) = LOWER\(\$1\) AND i\.expires_at > NOW\(\) AT TIME ZONE 'UTC'(.|\n)*`+
			`ORDER BY i\.workspace_id, i\.updated_at DESC`).
			WithArgs("Invitee@Example.com").
			WillReturnRows(sqlmock.NewRows(columns).
				AddRow("inv-1", "wsacme", "Acme", "Jane Owner", expiresAt).
				AddRow("inv-2", "wsbeta", "Beta", "", expiresAt))

		invitations, err := repo.ListPendingInvitationsForEmail(context.Background(), "Invitee@Example.com")
		require.NoError(t, err)
		require.Len(t, invitations, 2)
		assert.Equal(t, domain.PendingInvitation{
			ID: "inv-1", WorkspaceID: "wsacme", WorkspaceName: "Acme", InviterName: "Jane Owner", ExpiresAt: expiresAt,
		}, *invitations[0])
		assert.Equal(t, "wsbeta", invitations[1].WorkspaceID)
		assert.Empty(t, invitations[1].InviterName)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("no invitations is an empty list, not nil", func(t *testing.T) {
		mock.ExpectQuery(`SELECT DISTINCT ON`).
			WithArgs("nobody@example.com").
			WillReturnRows(sqlmock.NewRows(columns))

		invitations, err := repo.ListPendingInvitationsForEmail(context.Background(), "nobody@example.com")
		require.NoError(t, err)
		assert.NotNil(t, invitations)
		assert.Empty(t, invitations)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("query error", func(t *testing.T) {
		mock.ExpectQuery(`SELECT DISTINCT ON`).
			WithArgs("invitee@example.com").
			WillReturnError(fmt.Errorf("database error"))

		_, err := repo.ListPendingInvitationsForEmail(context.Background(), "invitee@example.com")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to list pending invitations")
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("scan error", func(t *testing.T) {
		mock.ExpectQuery(`SELECT DISTINCT ON`).
			WithArgs("invitee@example.com").
			WillReturnRows(sqlmock.NewRows(columns).AddRow("inv-1", "wsacme", "Acme", "Jane", "not-a-time"))

		_, err := repo.ListPendingInvitationsForEmail(context.Background(), "invitee@example.com")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to scan pending invitation")
		require.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestWorkspaceRepository_GetWorkspaceInvitations(t *testing.T) {
	// Test workspaceRepository.GetWorkspaceInvitations - this was at 0% coverage
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

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

	workspaceID := "ws-123"
	now := time.Now().Truncate(time.Second)

	t.Run("Success - Returns invitations", func(t *testing.T) {
		rows := sqlmock.NewRows([]string{
			"id", "workspace_id", "inviter_id", "email", "permissions", "expires_at", "created_at", "updated_at",
		}).
			AddRow("inv-1", workspaceID, "user-1", "test1@example.com", []byte(`{"contacts":{"read":true}}`), now.Add(24*time.Hour), now, now).
			AddRow("inv-2", workspaceID, "user-2", "test2@example.com", []byte(`{"contacts":{"read":true,"write":true}}`), now.Add(48*time.Hour), now, now)

		mock.ExpectQuery(`SELECT id, workspace_id, inviter_id, email, permissions, expires_at, created_at, updated_at FROM workspace_invitations WHERE workspace_id = \$1 ORDER BY created_at DESC`).
			WithArgs(workspaceID).
			WillReturnRows(rows)

		invitations, err := repo.GetWorkspaceInvitations(context.Background(), workspaceID)
		require.NoError(t, err)
		require.Len(t, invitations, 2)
		assert.Equal(t, "inv-1", invitations[0].ID)
		assert.Equal(t, "inv-2", invitations[1].ID)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Success - Empty result", func(t *testing.T) {
		rows := sqlmock.NewRows([]string{
			"id", "workspace_id", "inviter_id", "email", "permissions", "expires_at", "created_at", "updated_at",
		})

		mock.ExpectQuery(`SELECT id, workspace_id, inviter_id, email, permissions, expires_at, created_at, updated_at FROM workspace_invitations WHERE workspace_id = \$1 ORDER BY created_at DESC`).
			WithArgs(workspaceID).
			WillReturnRows(rows)

		invitations, err := repo.GetWorkspaceInvitations(context.Background(), workspaceID)
		require.NoError(t, err)
		require.Empty(t, invitations)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Error - Query error", func(t *testing.T) {
		mock.ExpectQuery(`SELECT id, workspace_id, inviter_id, email, permissions, expires_at, created_at, updated_at FROM workspace_invitations WHERE workspace_id = \$1 ORDER BY created_at DESC`).
			WithArgs(workspaceID).
			WillReturnError(fmt.Errorf("query error"))

		invitations, err := repo.GetWorkspaceInvitations(context.Background(), workspaceID)
		require.Error(t, err)
		assert.Nil(t, invitations)
		assert.Contains(t, err.Error(), "failed to get workspace invitations")
	})
}
