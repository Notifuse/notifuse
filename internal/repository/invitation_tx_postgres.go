package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/Notifuse/notifuse/internal/domain"
)

// invitationTxRepository implements domain.InvitationTxRepository. It holds no connection:
// every method runs on the executor it is given. Its SQL mirrors the workspace repository's
// methods of the same names.
//
// workspace_invitations.expires_at is TIMESTAMP written in UTC, so it is compared with
// clock_timestamp() AT TIME ZONE 'UTC': clock_timestamp() so that a wait for the lock counts,
// and at UTC so that the connection's time zone does not shift it.
type invitationTxRepository struct{}

// NewInvitationTxRepository returns the domain.InvitationTxRepository of the system database.
func NewInvitationTxRepository() domain.InvitationTxRepository {
	return &invitationTxRepository{}
}

func (r *invitationTxRepository) LockInvitation(ctx context.Context, db domain.DBExecutor, invitationID string) (*domain.WorkspaceInvitation, bool, error) {
	var invitation domain.WorkspaceInvitation
	var expired bool
	err := db.QueryRowContext(ctx, `
		SELECT id, workspace_id, inviter_id, email, permissions, expires_at, created_at, updated_at,
			expires_at <= (clock_timestamp() AT TIME ZONE 'UTC')
		FROM workspace_invitations
		WHERE id = $1
		FOR UPDATE`, invitationID).Scan(
		&invitation.ID, &invitation.WorkspaceID, &invitation.InviterID, &invitation.Email,
		&invitation.Permissions, &invitation.ExpiresAt, &invitation.CreatedAt, &invitation.UpdatedAt,
		&expired,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, domain.ErrInvitationNotFound
	}
	if err != nil {
		return nil, false, fmt.Errorf("failed to lock the invitation: %w", err)
	}
	return &invitation, expired, nil
}

func (r *invitationTxRepository) DeleteInvitationsByEmail(ctx context.Context, db domain.DBExecutor, workspaceID, email string) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM workspace_invitations WHERE workspace_id = $1 AND LOWER(email) = LOWER($2)`,
		workspaceID, email); err != nil {
		return fmt.Errorf("failed to delete invitation by email: %w", err)
	}
	return nil
}

func (r *invitationTxRepository) IsUserWorkspaceMember(ctx context.Context, db domain.DBExecutor, userID, workspaceID string) (bool, error) {
	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM user_workspaces WHERE user_id = $1 AND workspace_id = $2`,
		userID, workspaceID).Scan(&count); err != nil {
		return false, fmt.Errorf("failed to check membership: %w", err)
	}
	return count > 0, nil
}

func (r *invitationTxRepository) CountWorkspaceMembersAndInvitations(ctx context.Context, db domain.DBExecutor, workspaceID, excludeEmail string) (int, error) {
	var count int
	err := db.QueryRowContext(ctx, `
		SELECT
			(SELECT COUNT(*) FROM user_workspaces uw JOIN users u ON uw.user_id = u.id WHERE uw.workspace_id = $1 AND u.type != 'api_key') +
			(SELECT COUNT(*) FROM workspace_invitations WHERE workspace_id = $1 AND LOWER(email) <> LOWER($2) AND expires_at > (clock_timestamp() AT TIME ZONE 'UTC'))`,
		workspaceID, excludeEmail).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count workspace members and invitations: %w", err)
	}
	return count, nil
}

func (r *invitationTxRepository) AddUserToWorkspaceIfAbsent(ctx context.Context, db domain.DBExecutor, membership *domain.UserWorkspace) (bool, error) {
	result, err := db.ExecContext(ctx, `
		INSERT INTO user_workspaces (user_id, workspace_id, role, permissions, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (user_id, workspace_id) DO NOTHING`,
		membership.UserID, membership.WorkspaceID, membership.Role, membership.Permissions,
		membership.CreatedAt, membership.UpdatedAt)
	if err != nil {
		return false, fmt.Errorf("failed to add user to workspace: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("failed to read affected rows: %w", err)
	}
	return rows == 1, nil
}
