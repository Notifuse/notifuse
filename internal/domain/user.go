package domain

import (
	"context"
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

type AuthResponse struct {
	Token     string    `json:"token"`
	User      User      `json:"user"`
	ExpiresAt time.Time `json:"expires_at"`
}

// UserServiceInterface defines the interface for user operations
type UserServiceInterface interface {
	SignIn(ctx context.Context, input SignInInput) (string, error)
	VerifyCode(ctx context.Context, input VerifyCodeInput) (*AuthResponse, error)
	VerifyUserSession(ctx context.Context, userID string, sessionID string) (*User, error)
	GetUserByID(ctx context.Context, userID string) (*User, error)
	GetUserByEmail(ctx context.Context, email string) (*User, error)
	Logout(ctx context.Context, userID string) error
}

type UserRepository interface {
	// CreateUser creates a new user in the database
	CreateUser(ctx context.Context, user *User) error

	// GetUserByEmail retrieves a user by their email address
	GetUserByEmail(ctx context.Context, email string) (*User, error)

	// GetUserByID retrieves a user by their ID
	GetUserByID(ctx context.Context, id string) (*User, error)

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
