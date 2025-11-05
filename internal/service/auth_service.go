package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/golang-jwt/jwt/v5"
)

var (
	ErrSessionExpired = errors.New("session expired")
	ErrUserNotFound   = errors.New("user not found")
)

// UserClaims for user session tokens
type UserClaims struct {
	UserID    string `json:"user_id"`
	Type      string `json:"type"`
	SessionID string `json:"session_id,omitempty"`
	Email     string `json:"email,omitempty"`
	jwt.RegisteredClaims
}

// InvitationClaims for workspace invitation tokens
type InvitationClaims struct {
	InvitationID string `json:"invitation_id"`
	WorkspaceID  string `json:"workspace_id"`
	Email        string `json:"email"`
	jwt.RegisteredClaims
}

type AuthService struct {
	repo          domain.AuthRepository
	workspaceRepo domain.WorkspaceRepository
	logger        logger.Logger
	getSecret     func() ([]byte, error) // Changed from getKeys

	// Cached secret
	cachedSecret []byte
	secretLoaded bool
}

type AuthServiceConfig struct {
	Repository          domain.AuthRepository
	WorkspaceRepository domain.WorkspaceRepository
	GetSecret           func() ([]byte, error) // Changed from GetKeys
	Logger              logger.Logger
}

func NewAuthService(cfg AuthServiceConfig) *AuthService {
	return &AuthService{
		repo:          cfg.Repository,
		workspaceRepo: cfg.WorkspaceRepository,
		logger:        cfg.Logger,
		getSecret:     cfg.GetSecret,
		secretLoaded:  false,
	}
}

// ensureSecret loads and caches JWT secret if not already loaded
func (s *AuthService) ensureSecret() error {
	if s.secretLoaded {
		return nil
	}

	secret, err := s.getSecret()
	if err != nil {
		return fmt.Errorf("JWT secret not available: %w", err)
	}

	if len(secret) == 0 {
		return fmt.Errorf("JWT secret cannot be empty")
	}

	// Warn if secret is less than recommended length
	if len(secret) < 32 && s.logger != nil {
		s.logger.WithField("length", len(secret)).Warn("JWT secret is less than 32 bytes - consider using a stronger secret for production")
	}

	s.cachedSecret = secret
	s.secretLoaded = true
	return nil
}

// InvalidateSecretCache clears the cached secret, forcing it to be reloaded on next use
func (s *AuthService) InvalidateSecretCache() {
	s.secretLoaded = false
}
func (s *AuthService) AuthenticateUserFromContext(ctx context.Context) (*domain.User, error) {

	userID, ok := ctx.Value(domain.UserIDKey).(string)
	if !ok || userID == "" {
		return nil, ErrUserNotFound
	}
	userType, ok := ctx.Value(domain.UserTypeKey).(string)
	if !ok || userType == "" {
		return nil, ErrUserNotFound
	}
	if userType == string(domain.UserTypeUser) {
		sessionID, ok := ctx.Value(domain.SessionIDKey).(string)
		if !ok || sessionID == "" {
			return nil, ErrUserNotFound
		}
		return s.VerifyUserSession(ctx, userID, sessionID)
	} else if userType == string(domain.UserTypeAPIKey) {
		return s.GetUserByID(ctx, userID)
	}
	return nil, ErrUserNotFound
}

// AuthenticateUserForWorkspace checks if the user exists and the session is valid for a specific workspace
func (s *AuthService) AuthenticateUserForWorkspace(ctx context.Context, workspaceID string) (context.Context, *domain.User, *domain.UserWorkspace, error) {
	// Check if user is already set in context for this workspace
	if workspaceUser, ok := ctx.Value(domain.WorkspaceUserKey(workspaceID)).(*domain.User); ok && workspaceUser != nil {
		// Also check if we have the userWorkspace in context
		if userWorkspace, ok := ctx.Value(domain.UserWorkspaceKey).(*domain.UserWorkspace); ok && userWorkspace != nil {
			return ctx, workspaceUser, userWorkspace, nil
		}
	}

	user, err := s.AuthenticateUserFromContext(ctx)
	if err != nil {
		return ctx, nil, nil, err
	}

	// First check if the workspace exists - this will return ErrWorkspaceNotFound if it doesn't exist
	_, err = s.workspaceRepo.GetByID(ctx, workspaceID)
	if err != nil {
		return ctx, nil, nil, err
	}

	// Then check if the user is a member of the workspace
	userWorkspace, err := s.workspaceRepo.GetUserWorkspace(ctx, user.ID, workspaceID)
	if err != nil {
		return ctx, nil, nil, err
	}

	// Store user and user workspace in context for future calls - return the new context to the caller
	newCtx := context.WithValue(ctx, domain.WorkspaceUserKey(workspaceID), user)
	newCtx = context.WithValue(newCtx, domain.UserWorkspaceKey, userWorkspace)
	return newCtx, user, userWorkspace, nil
}

// VerifyUserSession checks if the user exists and the session is valid
func (s *AuthService) VerifyUserSession(ctx context.Context, userID, sessionID string) (*domain.User, error) {
	// First check if the session is valid and not expired
	expiresAt, err := s.repo.GetSessionByID(ctx, sessionID, userID)

	if err == sql.ErrNoRows {
		if s.logger != nil {
			s.logger.WithField("user_id", userID).WithField("session_id", sessionID).Error("Session not found")
		}
		return nil, ErrSessionExpired
	}
	if err != nil {
		if s.logger != nil {
			s.logger.WithField("user_id", userID).WithField("session_id", sessionID).WithField("error", err.Error()).Error("Failed to query session")
		}
		return nil, err
	}

	// Check if session is expired
	if time.Now().After(*expiresAt) {
		if s.logger != nil {
			s.logger.WithField("user_id", userID).WithField("session_id", sessionID).WithField("expires_at", expiresAt).Error("Session expired")
		}
		return nil, ErrSessionExpired
	}

	// Get user details
	user, err := s.repo.GetUserByID(ctx, userID)

	if err == sql.ErrNoRows {
		if s.logger != nil {
			s.logger.WithField("user_id", userID).Error("User not found")
		}
		return nil, ErrUserNotFound
	}
	if err != nil {
		if s.logger != nil {
			s.logger.WithField("user_id", userID).WithField("error", err.Error()).Error("Failed to query user")
		}
		return nil, err
	}

	return user, nil
}

// GenerateUserAuthToken generates an authentication token for a user
func (s *AuthService) GenerateUserAuthToken(user *domain.User, sessionID string, expiresAt time.Time) string {
	if err := s.ensureSecret(); err != nil {
		if s.logger != nil {
			s.logger.WithField("error", err.Error()).Error("Cannot generate auth token")
		}
		return ""
	}

	claims := UserClaims{
		UserID:    user.ID,
		Type:      string(domain.UserTypeUser),
		SessionID: sessionID,
		Email:     user.Email,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(s.cachedSecret)
	if err != nil && s.logger != nil {
		s.logger.WithField("error", err.Error()).Error("Failed to sign token")
		return ""
	}

	return signed
}

// GenerateAPIAuthToken generates an authentication token for an API key
func (s *AuthService) GenerateAPIAuthToken(user *domain.User) string {
	if err := s.ensureSecret(); err != nil {
		if s.logger != nil {
			s.logger.WithField("error", err.Error()).Error("Cannot generate API token")
		}
		return ""
	}

	claims := UserClaims{
		UserID: user.ID,
		Email:  user.Email, // Include email for SMTP Relay authentication
		Type:   string(domain.UserTypeAPIKey),
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour * 24 * 365 * 10)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(s.cachedSecret)
	if err != nil && s.logger != nil {
		s.logger.WithField("error", err.Error()).Error("Failed to sign API token")
		return ""
	}

	return signed
}

// GenerateInvitationToken generates a JWT token for a workspace invitation
func (s *AuthService) GenerateInvitationToken(invitation *domain.WorkspaceInvitation) string {
	if err := s.ensureSecret(); err != nil {
		if s.logger != nil {
			s.logger.WithField("error", err.Error()).Error("Cannot generate invitation token")
		}
		return ""
	}

	claims := InvitationClaims{
		InvitationID: invitation.ID,
		WorkspaceID:  invitation.WorkspaceID,
		Email:        invitation.Email,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(invitation.ExpiresAt),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(s.cachedSecret)
	if err != nil && s.logger != nil {
		s.logger.WithField("error", err.Error()).Error("Failed to sign invitation token")
		return ""
	}

	return signed
}

// ValidateInvitationToken validates a JWT invitation token and returns the invitation details
func (s *AuthService) ValidateInvitationToken(tokenString string) (invitationID, workspaceID, email string, err error) {
	if err := s.ensureSecret(); err != nil {
		return "", "", "", fmt.Errorf("secret not available: %w", err)
	}

	claims := &InvitationClaims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		// CRITICAL: Verify signing method to prevent algorithm confusion attacks
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return s.cachedSecret, nil
	})

	// CRITICAL: Check both error AND token.Valid
	if err != nil {
		return "", "", "", fmt.Errorf("invalid invitation token: %w", err)
	}
	if !token.Valid {
		return "", "", "", fmt.Errorf("invalid invitation token: token not valid")
	}

	return claims.InvitationID, claims.WorkspaceID, claims.Email, nil
}

// GetUserByID retrieves a user by their ID
func (s *AuthService) GetUserByID(ctx context.Context, userID string) (*domain.User, error) {
	// Delegate to the repository
	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		if s.logger != nil {
			s.logger.WithField("error", err.Error()).WithField("user_id", userID).Error("Failed to get user by ID")
		}
		return nil, err
	}
	return user, nil
}
