package domain

import (
	"context"
	"time"
)

//go:generate mockgen -destination mocks/mock_auth_repository.go -package mocks github.com/Notifuse/notifuse/internal/domain AuthRepository
//go:generate mockgen -destination mocks/mock_auth_service.go -package mocks github.com/Notifuse/notifuse/internal/domain AuthService

type ContextKey string

const SystemCallKey ContextKey = "system_call"

// WorkspaceIDKey is the context key for workspace ID
const WorkspaceIDKey ContextKey = "workspace_id"

// AuthRepository defines the interface for auth-related database operations
type AuthRepository interface {
	// GetSessionByID returns a usable session (sql.ErrNoRows when there is none).
	GetSessionByID(ctx context.Context, sessionID string, userID string) (*SessionAuth, error)
	// IsTwoFactorEnabled reports whether the user turned on two-factor authentication.
	IsTwoFactorEnabled(ctx context.Context, userID string) (bool, error)
	GetUserByID(ctx context.Context, userID string) (*User, error)
}

type AuthService interface {
	AuthenticateUserFromContext(ctx context.Context) (*User, error)
	// AuthenticateSession checks a console session without the instance's two-factor
	// requirement, for the routes that let a session turn it on.
	AuthenticateSession(ctx context.Context) (*User, *SessionAuth, error)
	// RequireCompliantSession refuses a console session that did not pass the instance's
	// two-factor requirement (ErrTwoFactorRequired).
	RequireCompliantSession(ctx context.Context) error
	// RequireCompliantRootSession also refuses, on root routes, a session without a second
	// factor when the root user has two-factor authentication.
	RequireCompliantRootSession(ctx context.Context) error
	AuthenticateUserForWorkspace(ctx context.Context, workspaceID string) (context.Context, *User, *UserWorkspace, error)
	VerifyUserSession(ctx context.Context, userID, sessionID string) (*User, error)
	GenerateUserAuthToken(user *User, sessionID string, expiresAt time.Time) string
	GenerateAPIAuthToken(user *User) string
	GenerateInvitationToken(invitation *WorkspaceInvitation) string
	ValidateInvitationToken(token string) (invitationID, workspaceID, email string, err error)
	InvalidateSecretCache()
}
