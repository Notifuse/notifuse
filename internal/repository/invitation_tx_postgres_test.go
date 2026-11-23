package repository

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
)

// The expiry is decided in SQL, on the database clock read after the lock, at UTC.
func TestInvitationTxRepository_LockInvitation(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := NewInvitationTxRepository()
	now := time.Now().UTC()

	mock.ExpectQuery(`expires_at <= \(clock_timestamp\(\) AT TIME ZONE 'UTC'\)\s+FROM workspace_invitations\s+WHERE id = \$1\s+FOR UPDATE`).
		WithArgs("inv-1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "workspace_id", "inviter_id", "email", "permissions", "expires_at", "created_at", "updated_at", "expired"}).
			AddRow("inv-1", "ws1", "u0", "a@x.io", []byte("{}"), now, now, now, true))
	invitation, expired, err := repo.LockInvitation(context.Background(), db, "inv-1")
	require.NoError(t, err)
	assert.Equal(t, "ws1", invitation.WorkspaceID)
	assert.True(t, expired)

	mock.ExpectQuery(`FROM workspace_invitations`).WithArgs("gone").WillReturnError(sql.ErrNoRows)
	_, _, err = repo.LockInvitation(context.Background(), db, "gone")
	assert.ErrorIs(t, err, domain.ErrInvitationNotFound)
}

// An existing membership is never rewritten: a member who became owner keeps the role.
func TestInvitationTxRepository_AddUserToWorkspaceIfAbsent(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := NewInvitationTxRepository()
	membership := &domain.UserWorkspace{UserID: "u1", WorkspaceID: "ws1", Role: "member"}

	mock.ExpectExec(`INSERT INTO user_workspaces .* ON CONFLICT \(user_id, workspace_id\) DO NOTHING`).WillReturnResult(sqlmock.NewResult(0, 1))
	added, err := repo.AddUserToWorkspaceIfAbsent(context.Background(), db, membership)
	require.NoError(t, err)
	assert.True(t, added)

	mock.ExpectExec(`ON CONFLICT \(user_id, workspace_id\) DO NOTHING`).WillReturnResult(sqlmock.NewResult(0, 0))
	added, err = repo.AddUserToWorkspaceIfAbsent(context.Background(), db, membership)
	require.NoError(t, err)
	assert.False(t, added)
}

func TestInvitationTxRepository_MembershipAndSeats(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := NewInvitationTxRepository()

	mock.ExpectExec(`DELETE FROM workspace_invitations WHERE workspace_id = \$1 AND LOWER\(email\) = LOWER\(\$2\)`).
		WithArgs("ws1", "A@x.io").WillReturnResult(sqlmock.NewResult(0, 2))
	require.NoError(t, repo.DeleteInvitationsByEmail(context.Background(), db, "ws1", "A@x.io"))

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM user_workspaces WHERE user_id = \$1 AND workspace_id = \$2`).WithArgs("u1", "ws1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	member, err := repo.IsUserWorkspaceMember(context.Background(), db, "u1", "ws1")
	require.NoError(t, err)
	assert.True(t, member)

	mock.ExpectQuery(`u.type != 'api_key'.*expires_at > \(clock_timestamp\(\) AT TIME ZONE 'UTC'\)`).WithArgs("ws1", "a@x.io").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(4))
	seats, err := repo.CountWorkspaceMembersAndInvitations(context.Background(), db, "ws1", "a@x.io")
	require.NoError(t, err)
	assert.Equal(t, 4, seats)
}
