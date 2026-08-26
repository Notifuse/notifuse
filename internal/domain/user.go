package domain

import (
	"context"
	"errors"
	"time"
)

//go:generate mockgen -destination mocks/mock_user_repository.go -package mocks github.com/Notifuse/notifuse/internal/domain UserRepository
//go:generate mockgen -destination mocks/mock_user_service.go -package mocks github.com/Notifuse/notifuse/internal/domain UserServiceInterface

// Key for storing user ID and session ID in context
type contextKey string

const (
	UserIDKey        contextKey = "user_id"
	SessionIDKey     contextKey = "session_id"
	UserTypeKey      contextKey = "type"
	UserWorkspaceKey contextKey = "user_workspace"
)

type UserType string

const (
	UserTypeUser   UserType = "user"
	UserTypeAPIKey UserType = "api_key"
)

// WorkspaceUserKey creates a context key for storing a workspace-specific user
func WorkspaceUserKey(workspaceID string) contextKey {
	return contextKey("workspace_user_" + workspaceID)
}

// User represents a user in the system
type User struct {
	ID        string    `json:"id" db:"id"`
	Type      UserType  `json:"type" db:"type"`
	Email     string    `json:"email" db:"email"`
	Name      string    `json:"name,omitempty" db:"name"`
	Language  string    `json:"language" db:"language"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

// Session represents a user session
type Session struct {
	ID               string     `json:"id" db:"id"`
	UserID           string     `json:"user_id" db:"user_id"`
	ExpiresAt        time.Time  `json:"expires_at" db:"expires_at"`
	CreatedAt        time.Time  `json:"created_at" db:"created_at"`
	MagicCode        *string    `json:"magic_code,omitempty" db:"magic_code"`
	MagicCodeExpires *time.Time `json:"magic_code_expires,omitempty" db:"magic_code_expires_at"`
}

type SignInInput struct {
	Email string `json:"email"`
}

type VerifyCodeInput struct {
	Email string `json:"email"`
	Code  string `json:"code"`
}

// RootSigninInput represents the request for root user programmatic signin
type RootSigninInput struct {
	Email     string `json:"email"`
	Timestamp int64  `json:"timestamp"`
	Signature string `json:"signature"`
}

type AuthResponse struct {
	Token     string    `json:"token"`
	User      User      `json:"user"`
	ExpiresAt time.Time `json:"expires_at"`
}

// UserServiceInterface defines the interface for user operations
type UserServiceInterface interface {
	SignIn(ctx context.Context, input SignInInput) (string, error)
	VerifyCode(ctx context.Context, input VerifyCodeInput) (*AuthResponse, error)
	RootSignin(ctx context.Context, input RootSigninInput) (*AuthResponse, error)
	VerifyUserSession(ctx context.Context, userID string, sessionID string) (*User, error)
	GetUserByID(ctx context.Context, userID string) (*User, error)
	GetUserByEmail(ctx context.Context, email string) (*User, error)
	UpdateUserLanguage(ctx context.Context, userID string, language string) error
	Logout(ctx context.Context, userID string) error
}

type UserRepository interface {
	// CreateUser creates a new user in the database
	CreateUser(ctx context.Context, user *User) error

	// GetUserByEmail retrieves a user by their email address (exact match)
	GetUserByEmail(ctx context.Context, email string) (*User, error)

	// GetUserByEmailInsensitive retrieves a user by case-insensitive email match
	// (lower(email)=lower($1)). Used by the OIDC bridge so a mixed-case invited
	// user is matched regardless of the casing asserted by the IdP.
	GetUserByEmailInsensitive(ctx context.Context, email string) (*User, error)

	// GetUserByID retrieves a user by their ID
	GetUserByID(ctx context.Context, id string) (*User, error)

	// UpdateUserLanguage updates a user's preferred language
	UpdateUserLanguage(ctx context.Context, userID string, language string) error

	// CreateSession creates a new session for a user
	CreateSession(ctx context.Context, session *Session) error

	// GetSessionByID retrieves a session by its ID
	GetSessionByID(ctx context.Context, id string) (*Session, error)

	// GetSessionsByUserID retrieves all sessions for a user
	GetSessionsByUserID(ctx context.Context, userID string) ([]*Session, error)

	// UpdateSession updates an existing session
	UpdateSession(ctx context.Context, session *Session) error

	// DeleteSession deletes a session by its ID
	DeleteSession(ctx context.Context, id string) error

	// DeleteAllSessionsByUserID deletes all sessions for a user
	DeleteAllSessionsByUserID(ctx context.Context, userID string) error

	// Delete removes a user by their ID
	Delete(ctx context.Context, id string) error
}

// ErrUserNotFound is returned when a user is not found
type ErrUserNotFound struct {
	Message string
}

func (e *ErrUserNotFound) Error() string {
	return e.Message
}

// ErrUserExists is returned when trying to create a user that already exists
type ErrUserExists struct {
	Message string
}

func (e *ErrUserExists) Error() string {
	return e.Message
}

// ErrSessionNotFound is returned when a session is not found
type ErrSessionNotFound struct {
	Message string
}

func (e *ErrSessionNotFound) Error() string {
	return e.Message
}

// ErrSessionAuthFailed wraps every session-verification failure that means the
// caller is not authenticated: an unknown, expired, or mismatched session. It
// deliberately does NOT cover a lookup that could not be answered, such as a
// database error.
//
// Handlers must answer the two cases differently. The console discards the stored
// token and leaves the current page on any 401, so reporting a transient database
// failure that way signs the user out and throws away the URL they were on.
var ErrSessionAuthFailed = errors.New("session authentication failed")

// ErrAPIKeyRevoked reports that a bearer token authenticated, but the API-key
// user it names no longer exists.
//
// It is the only shape a revoked key can take. An API key's token lives ten
// years, carries no jti and has no denylist, so revoking one deletes its user
// row and nothing else — the signature keeps verifying forever. Without a typed
// error for it, every endpoint answered a revoked key with 500, which reads as
// "the server is broken" to any client and specifically stops Zapier raising the
// reconnect prompt that a 401 triggers.
//
// Distinct from ErrSessionAuthFailed for the same reason that one exists: only a
// proven authentication failure may answer 401, never a database that could not
// answer the lookup.
var ErrAPIKeyRevoked = errors.New("api key has been revoked")
