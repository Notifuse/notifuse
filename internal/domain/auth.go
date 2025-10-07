package domain

import (
	"context"
	"time"

	"aidanwoods.dev/go-paseto"
)

//go:generate mockgen -destination mocks/mock_auth_repository.go -package mocks github.com/Notifuse/notifuse/internal/domain AuthRepository
//go:generate mockgen -destination mocks/mock_auth_service.go -package mocks github.com/Notifuse/notifuse/internal/domain AuthService

type ContextKey string

const SystemCallKey ContextKey = "system_call"

// AuthRepository defines the interface for auth-related database operations
type AuthRepository interface {
	GetSessionByID(ctx context.Context, sessionID string, userID string) (*time.Time, error)
	GetUserByID(ctx context.Context, userID string) (*User, error)
}

type AuthService interface {
	AuthenticateUserFromContext(ctx context.Context) (*User, error)
	AuthenticateUserForWorkspace(ctx context.Context, workspaceID string) (context.Context, *User, *UserWorkspace, error)
	VerifyUserSession(ctx context.Context, userID, sessionID string) (*User, error)
	GenerateUserAuthToken(user *User, sessionID string, expiresAt time.Time) string
	GenerateAPIAuthToken(user *User) string
	GetPrivateKey() (paseto.V4AsymmetricSecretKey, error)
	GetPublicKey() (paseto.V4AsymmetricPublicKey, error)
	GenerateInvitationToken(invitation *WorkspaceInvitation) string
	ValidateInvitationToken(token string) (invitationID, workspaceID, email string, err error)
	InvalidateKeyCache()
}
