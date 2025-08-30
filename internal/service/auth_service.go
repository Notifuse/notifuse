package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/logger"

	"aidanwoods.dev/go-paseto"
)

var (
	ErrSessionExpired = errors.New("session expired")
	ErrUserNotFound   = errors.New("user not found")
)

type AuthService struct {
	repo          domain.AuthRepository
	workspaceRepo domain.WorkspaceRepository
	logger        logger.Logger
	privateKey    paseto.V4AsymmetricSecretKey
	publicKey     paseto.V4AsymmetricPublicKey
}

type AuthServiceConfig struct {
	Repository          domain.AuthRepository
	WorkspaceRepository domain.WorkspaceRepository
	PrivateKey          []byte
	PublicKey           []byte
	Logger              logger.Logger
}

func NewAuthService(cfg AuthServiceConfig) (*AuthService, error) {
	privateKey, err := paseto.NewV4AsymmetricSecretKeyFromBytes(cfg.PrivateKey)
	if err != nil {
		if cfg.Logger != nil {
			cfg.Logger.WithField("error", err.Error()).Error("Error creating PASETO private key")
		}
		return nil, err
	}

	publicKey, err := paseto.NewV4AsymmetricPublicKeyFromBytes(cfg.PublicKey)
	if err != nil {
		if cfg.Logger != nil {
			cfg.Logger.WithField("error", err.Error()).Error("Error creating PASETO public key")
		}
		return nil, err
	}

	return &AuthService{
		repo:          cfg.Repository,
		workspaceRepo: cfg.WorkspaceRepository,
		logger:        cfg.Logger,
		privateKey:    privateKey,
		publicKey:     publicKey,
	}, nil
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
func (s *AuthService) AuthenticateUserForWorkspace(ctx context.Context, workspaceID string) (context.Context, *domain.User, error) {
	// Check if user is already set in context for this workspace
	if workspaceUser, ok := ctx.Value(domain.WorkspaceUserKey(workspaceID)).(*domain.User); ok && workspaceUser != nil {
		return ctx, workspaceUser, nil
	}

	user, err := s.AuthenticateUserFromContext(ctx)
	if err != nil {
		return ctx, nil, err
	}

	// First check if the workspace exists - this will return ErrWorkspaceNotFound if it doesn't exist
	_, err = s.workspaceRepo.GetByID(ctx, workspaceID)
	if err != nil {
		return ctx, nil, err
	}

	// Then check if the user is a member of the workspace
	_, err = s.workspaceRepo.GetUserWorkspace(ctx, user.ID, workspaceID)
	if err != nil {
		return ctx, nil, err
	}

	// Store user in context for future calls - return the new context to the caller
	newCtx := context.WithValue(ctx, domain.WorkspaceUserKey(workspaceID), user)
	return newCtx, user, nil
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

// GenerateAuthToken generates an authentication token for a user
func (s *AuthService) GenerateUserAuthToken(user *domain.User, sessionID string, expiresAt time.Time) string {
	token := paseto.NewToken()
	token.SetIssuedAt(time.Now())
	token.SetNotBefore(time.Now())
	token.SetExpiration(expiresAt)
	token.SetString("user_id", user.ID)
	token.SetString("type", string(domain.UserTypeUser))
	token.SetString("session_id", sessionID)
	token.SetString("email", user.Email)

	encrypted := token.V4Sign(s.privateKey, nil)
	if encrypted == "" && s.logger != nil {
		s.logger.WithField("user_id", user.ID).WithField("session_id", sessionID).Error("Failed to sign authentication token")
	}

	return encrypted
}

// GenerateAPIAuthToken generates an authentication token for an API key
func (s *AuthService) GenerateAPIAuthToken(user *domain.User) string {
	token := paseto.NewToken()
	token.SetIssuedAt(time.Now())
	token.SetNotBefore(time.Now())
	token.SetExpiration(time.Now().Add(time.Hour * 24 * 365 * 10))
	token.SetString("user_id", user.ID)
	token.SetString("type", string(domain.UserTypeAPIKey))

	encrypted := token.V4Sign(s.privateKey, nil)
	if encrypted == "" && s.logger != nil {
		s.logger.WithField("user_id", user.ID).Error("Failed to sign API authentication token")
	}

	return encrypted
}

// GetPrivateKey returns the private key
func (s *AuthService) GetPrivateKey() paseto.V4AsymmetricSecretKey {
	return s.privateKey
}

// GenerateInvitationToken generates a PASETO token for a workspace invitation
func (s *AuthService) GenerateInvitationToken(invitation *domain.WorkspaceInvitation) string {
	token := paseto.NewToken()
	token.SetIssuedAt(time.Now())
	token.SetNotBefore(time.Now())
	token.SetExpiration(invitation.ExpiresAt)
	token.SetString("invitation_id", invitation.ID)
	token.SetString("workspace_id", invitation.WorkspaceID)
	token.SetString("email", invitation.Email)

	encrypted := token.V4Sign(s.privateKey, nil)
	if encrypted == "" && s.logger != nil {
		s.logger.WithField("invitation_id", invitation.ID).Error("Failed to sign invitation token")
	}

	return encrypted
}

// ValidateInvitationToken validates a PASETO invitation token and returns the invitation details
func (s *AuthService) ValidateInvitationToken(token string) (invitationID, workspaceID, email string, err error) {
	parser := paseto.NewParser()
	parser.AddRule(paseto.NotExpired())

	// Verify token and get claims
	verified, err := parser.ParseV4Public(s.publicKey, token, nil)
	if err != nil {
		if s.logger != nil {
			s.logger.WithField("error", err.Error()).Error("Failed to parse invitation token")
		}
		return "", "", "", fmt.Errorf("invalid invitation token: %w", err)
	}

	// Extract invitation details from claims
	invitationID, err = verified.GetString("invitation_id")
	if err != nil {
		if s.logger != nil {
			s.logger.WithField("error", err.Error()).Error("Invitation ID not found in token")
		}
		return "", "", "", fmt.Errorf("invitation ID not found in token")
	}

	workspaceID, err = verified.GetString("workspace_id")
	if err != nil {
		if s.logger != nil {
			s.logger.WithField("error", err.Error()).Error("Workspace ID not found in token")
		}
		return "", "", "", fmt.Errorf("workspace ID not found in token")
	}

	email, err = verified.GetString("email")
	if err != nil {
		if s.logger != nil {
			s.logger.WithField("error", err.Error()).Error("Email not found in token")
		}
		return "", "", "", fmt.Errorf("email not found in token")
	}

	return invitationID, workspaceID, email, nil
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
