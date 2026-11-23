package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
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
	isRootEmail   func(string) bool      // reports whether an email is a configured platform admin (ROOT_EMAIL)
	// requireTwoFactor: the instance requires every person to pass a second factor.
	requireTwoFactor bool

	// The cached secret. Every request that checks a token reads it, and
	// InvalidateSecretCache clears it, so both go through secretMu.
	secretMu     sync.RWMutex
	cachedSecret []byte
	secretLoaded bool
}

type AuthServiceConfig struct {
	Repository          domain.AuthRepository
	WorkspaceRepository domain.WorkspaceRepository
	GetSecret           func() ([]byte, error) // Changed from GetKeys
	Logger              logger.Logger
	// IsRootEmail reports whether an email is a configured platform admin (ROOT_EMAIL).
	// When set, root emails get synthesized owner access to every workspace. Optional in tests.
	IsRootEmail func(string) bool
	// RequireTwoFactor is config.RequireTwoFactor: a session that did not pass a second
	// factor then reaches only the routes that let it turn two-factor authentication on.
	RequireTwoFactor bool
}

func NewAuthService(cfg AuthServiceConfig) *AuthService {
	return &AuthService{
		repo:             cfg.Repository,
		workspaceRepo:    cfg.WorkspaceRepository,
		logger:           cfg.Logger,
		getSecret:        cfg.GetSecret,
		isRootEmail:      cfg.IsRootEmail,
		requireTwoFactor: cfg.RequireTwoFactor,
		secretLoaded:     false,
	}
}

// jwtSecret returns the JWT secret, loading and caching it on first use.
func (s *AuthService) jwtSecret() ([]byte, error) {
	s.secretMu.RLock()
	if s.secretLoaded {
		secret := s.cachedSecret
		s.secretMu.RUnlock()
		return secret, nil
	}
	s.secretMu.RUnlock()

	s.secretMu.Lock()
	defer s.secretMu.Unlock()
	if s.secretLoaded {
		return s.cachedSecret, nil
	}

	secret, err := s.getSecret()
	if err != nil {
		return nil, fmt.Errorf("JWT secret not available: %w", err)
	}

	if len(secret) == 0 {
		return nil, fmt.Errorf("JWT secret cannot be empty")
	}

	// Warn if secret is less than recommended length
	if len(secret) < 32 && s.logger != nil {
		s.logger.WithField("length", len(secret)).Warn("JWT secret is less than 32 bytes - consider using a stronger secret for production")
	}

	s.cachedSecret = secret
	s.secretLoaded = true
	return secret, nil
}

// InvalidateSecretCache clears the cached secret, forcing it to be reloaded on next use
func (s *AuthService) InvalidateSecretCache() {
	s.secretMu.Lock()
	defer s.secretMu.Unlock()
	s.secretLoaded = false
	s.cachedSecret = nil
}
func (s *AuthService) AuthenticateUserFromContext(ctx context.Context) (*domain.User, error) {
	user, _, err := s.authenticateContext(ctx)
	return user, err
}

// authenticateContext is AuthenticateUserFromContext, also returning what is known about the
// session of a console token (nil for an API key).
func (s *AuthService) authenticateContext(ctx context.Context) (*domain.User, *domain.SessionAuth, error) {

	// A request without the claims of a token is not authenticated: ErrSessionAuthFailed,
	// which answers 401, like an expired session.
	userID, ok := ctx.Value(domain.UserIDKey).(string)
	if !ok || userID == "" {
		return nil, nil, fmt.Errorf("%w: %w", domain.ErrSessionAuthFailed, ErrUserNotFound)
	}
	userType, ok := ctx.Value(domain.UserTypeKey).(string)
	if !ok || userType == "" {
		return nil, nil, fmt.Errorf("%w: %w", domain.ErrSessionAuthFailed, ErrUserNotFound)
	}
	if userType == string(domain.UserTypeUser) {
		user, session, err := s.AuthenticateSession(ctx)
		if err != nil {
			return nil, nil, err
		}
		if s.requireTwoFactor && !domain.SessionSatisfiesTwoFactor(*session) {
			return nil, nil, domain.ErrTwoFactorRequired
		}
		return user, session, nil
	} else if userType == string(domain.UserTypeAPIKey) {
		user, err := s.GetUserByID(ctx, userID)
		if err == nil && user.Type != domain.UserTypeAPIKey {
			// An API key token stands for an API key account and nothing else: it
			// has no session behind it, so naming a person's account must not make
			// it that person's session. Answered like a key whose account is gone.
			err = ErrUserNotFound
		}
		if errors.Is(err, ErrUserNotFound) {
			// Revoking an API key deletes its user row, and that delete is the
			// whole of the revocation — the token itself stays signed and valid
			// for ten years. So "the user this key names is gone" means "this key
			// was revoked", and it has to reach the client as an authentication
			// failure. Untyped it collapsed to 500 at every handler, which tells
			// an API client its server is broken rather than that its credential
			// is dead.
			return nil, nil, fmt.Errorf("%w: %w", domain.ErrAPIKeyRevoked, err)
		}
		return user, nil, err
	}
	return nil, nil, fmt.Errorf("%w: %w", domain.ErrSessionAuthFailed, ErrUserNotFound)
}

// AuthenticateAPIKeyToken resolves a bearer string as /mcp resolves its header: a JWT of
// type api_key, signed by this installation, whose account still exists and is an API key.
// Anything that is not such a token (malformed, signed elsewhere, a console session, an
// invitation) answers domain.ErrNotAnAPIKey; a key whose account was deleted answers
// domain.ErrAPIKeyRevoked. The token is never logged.
func (s *AuthService) AuthenticateAPIKeyToken(ctx context.Context, token string) (*domain.User, error) {
	secret, err := s.jwtSecret()
	if err != nil {
		return nil, err
	}
	claims := &UserClaims{}
	parsed, err := jwt.ParseWithClaims(strings.TrimSpace(token), claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return secret, nil
	})
	if err != nil || !parsed.Valid || claims.UserID == "" || claims.Type != string(domain.UserTypeAPIKey) {
		return nil, domain.ErrNotAnAPIKey
	}
	keyCtx := context.WithValue(ctx, domain.UserIDKey, claims.UserID)
	keyCtx = context.WithValue(keyCtx, domain.UserTypeKey, claims.Type)
	return s.AuthenticateUserFromContext(keyCtx)
}

// AuthenticateUserForWorkspace checks if the user exists and the session is valid for a specific workspace.
//
// When the context carries an MCPCaller, the workspace must also be reachable
// over MCP: a membership row and the workspace's MCP switch on. A nonexistent
// workspace, a missing membership and a switched-off workspace then all answer
// ErrUserNotInWorkspace, so an MCP client cannot tell them apart.
func (s *AuthService) AuthenticateUserForWorkspace(ctx context.Context, workspaceID string) (context.Context, *domain.User, *domain.UserWorkspace, error) {
	// Check if user is already set in context for this workspace. The membership
	// is cached per workspace and must name this one: a context authenticated
	// against several workspaces holds one membership for each.
	if workspaceUser, ok := ctx.Value(domain.WorkspaceUserKey(workspaceID)).(*domain.User); ok && workspaceUser != nil {
		if userWorkspace, ok := ctx.Value(domain.WorkspaceMembershipKey(workspaceID)).(*domain.UserWorkspace); ok && userWorkspace != nil && userWorkspace.WorkspaceID == workspaceID {
			// An audit record created after the first authentication (an MCP
			// tool call opens its own) still has to name its actor. The cache lives
			// in the request's context, so it never outlives one HTTP request.
			s.recordActor(ctx, workspaceUser, userWorkspace, workspaceID)
			return ctx, workspaceUser, userWorkspace, nil
		}
	}

	user, session, err := s.authenticateContext(ctx)
	if err != nil {
		return ctx, nil, nil, err
	}
	// A root user who turned two-factor authentication on reaches workspaces as their owner only
	// from a session that passed it: an older session, kept when they turned it on, would
	// otherwise keep that access to every workspace.
	if session != nil && s.isRootUser(user) {
		if err := s.checkRootSession(ctx, user, session); err != nil {
			return ctx, nil, nil, err
		}
	}

	mcpCaller := domain.MCPCallerFromContext(ctx)
	// An MCP call is always the API key its credential names: any other account,
	// such as a console session token that reached /mcp, gets the not-found answer.
	if mcpCaller != nil && (user.Type != domain.UserTypeAPIKey || user.ID != mcpCaller.APIKeyUserID) {
		return ctx, nil, nil, domain.ErrUserNotInWorkspace
	}

	// First check if the workspace exists - this will return ErrWorkspaceNotFound if it doesn't exist.
	// The row is kept: the MCP reachability check reads its switch.
	workspace, err := s.workspaceRepo.GetByID(ctx, workspaceID)
	if err != nil {
		var notFound *domain.ErrWorkspaceNotFound
		if mcpCaller != nil && errors.As(err, &notFound) {
			return ctx, nil, nil, domain.ErrUserNotInWorkspace
		}
		return ctx, nil, nil, err
	}

	// Then check if the user is a member of the workspace
	userWorkspace, err := s.workspaceRepo.GetUserWorkspace(ctx, user.ID, workspaceID)

	if mcpCaller != nil {
		if errors.Is(err, domain.ErrUserNotInWorkspace) || (err == nil && !workspace.MCPEnabled()) {
			return ctx, nil, nil, domain.ErrUserNotInWorkspace
		}
	}

	// Platform admins (ROOT_EMAIL) are full owners of every workspace. An API key
	// account is never one, whatever its email: a key must not become an owner
	// because ROOT_EMAIL was later set to the address it was created with.
	isRoot := s.isRootUser(user)
	switch {
	case err == nil:
		// Elevate a root user who happens to hold a non-owner membership row so
		// their access is identical to a root user who holds no row at all
		// (handled below).
		if isRoot && userWorkspace.Role != "owner" {
			userWorkspace.Role = "owner"
			userWorkspace.Permissions = domain.NewFullPermissions()
		}
	case errors.Is(err, domain.ErrUserNotInWorkspace) && isRoot:
		// Platform-admin override: a configured ROOT_EMAIL gets synthesized owner access to
		// every existing workspace, even without a membership row. Keyed off the typed
		// not-found error so genuine DB failures still propagate. The GetByID check above
		// already guaranteed the workspace exists.
		userWorkspace = &domain.UserWorkspace{
			UserID:      user.ID,
			WorkspaceID: workspaceID,
			Role:        "owner",
			Permissions: domain.NewFullPermissions(),
		}
	default:
		return ctx, nil, nil, err
	}

	s.recordActor(ctx, user, userWorkspace, workspaceID)

	// Store user and user workspace in context for future calls - return the new context to the caller
	newCtx := context.WithValue(ctx, domain.WorkspaceUserKey(workspaceID), user)
	newCtx = context.WithValue(newCtx, domain.WorkspaceMembershipKey(workspaceID), userWorkspace)
	return newCtx, user, userWorkspace, nil
}

// isRootUser reports whether the user is a platform admin (ROOT_EMAIL). Only a
// person's account can be one; an API key account never is.
func (s *AuthService) isRootUser(user *domain.User) bool {
	return user.Type != domain.UserTypeAPIKey && s.isRootEmail != nil && s.isRootEmail(user.Email)
}

// recordActor names the actor and the workspace for the audit log; a no-op
// outside an audited request. Root is recorded as such whatever the membership
// row says.
func (s *AuthService) recordActor(ctx context.Context, user *domain.User, userWorkspace *domain.UserWorkspace, workspaceID string) {
	if record := domain.AuditFromContext(ctx); record != nil {
		record.SetActor(user, userWorkspace, s.isRootUser(user))
		record.SetWorkspace(workspaceID)
	}
}

// AuthenticateSession checks the session of a console token and returns its user and what
// is known about the session, without the instance's two-factor requirement: for the few
// routes a session that has not passed it may still use (its account, signing out, turning
// two-factor authentication on). Every other route goes through AuthenticateUserFromContext.
func (s *AuthService) AuthenticateSession(ctx context.Context) (*domain.User, *domain.SessionAuth, error) {
	userID, ok := ctx.Value(domain.UserIDKey).(string)
	if !ok || userID == "" {
		return nil, nil, fmt.Errorf("%w: %w", domain.ErrSessionAuthFailed, ErrUserNotFound)
	}
	if userType, _ := ctx.Value(domain.UserTypeKey).(string); userType != string(domain.UserTypeUser) {
		return nil, nil, fmt.Errorf("%w: %w", domain.ErrSessionAuthFailed, ErrUserNotFound)
	}
	sessionID, ok := ctx.Value(domain.SessionIDKey).(string)
	if !ok || sessionID == "" {
		return nil, nil, fmt.Errorf("%w: %w", domain.ErrSessionAuthFailed, ErrUserNotFound)
	}
	return s.verifySession(ctx, userID, sessionID)
}

// RequireCompliantSession refuses, when the instance requires two-factor authentication, a
// console session that did not pass it. For the handlers that check the session by
// themselves rather than through AuthenticateUserFromContext. A request that is not a
// console session (an API key) is not concerned.
func (s *AuthService) RequireCompliantSession(ctx context.Context) error {
	if !s.requireTwoFactor {
		return nil
	}
	if userType, _ := ctx.Value(domain.UserTypeKey).(string); userType != string(domain.UserTypeUser) {
		return nil
	}
	_, session, err := s.AuthenticateSession(ctx)
	if err != nil {
		return err
	}
	if !domain.SessionSatisfiesTwoFactor(*session) {
		return domain.ErrTwoFactorRequired
	}
	return nil
}

// RequireCompliantRootSession refuses, on a root route, a session that did not pass a
// second factor when the root user has two-factor authentication, even when the instance
// does not require it: otherwise an older session, kept when the root user turned it on,
// would still hold the instance's settings.
func (s *AuthService) RequireCompliantRootSession(ctx context.Context) error {
	if userType, _ := ctx.Value(domain.UserTypeKey).(string); userType != string(domain.UserTypeUser) {
		return nil
	}
	user, session, err := s.AuthenticateSession(ctx)
	if err != nil {
		return err
	}
	return s.checkRootSession(ctx, user, session)
}

// checkRootSession refuses a root user's session that did not pass a second factor when the
// instance requires it, or when the root user has two-factor authentication.
func (s *AuthService) checkRootSession(ctx context.Context, user *domain.User, session *domain.SessionAuth) error {
	if domain.SessionSatisfiesTwoFactor(*session) {
		return nil
	}
	if s.requireTwoFactor {
		return domain.ErrTwoFactorRequired
	}
	enabled, err := s.repo.IsTwoFactorEnabled(ctx, user.ID)
	if err != nil {
		return err
	}
	if enabled {
		return domain.ErrTwoFactorRequired
	}
	return nil
}

// VerifyUserSession checks if the user exists and the session is valid
func (s *AuthService) VerifyUserSession(ctx context.Context, userID, sessionID string) (*domain.User, error) {
	user, _, err := s.verifySession(ctx, userID, sessionID)
	return user, err
}

// verifySession checks the session and its user, and returns what the request needs to
// know about the session. Every failure that means "not authenticated" (no usable session,
// an expired one, a user that is gone or that is not a person) wraps
// domain.ErrSessionAuthFailed, which the HTTP layer answers with 401; a lookup that could
// not be answered stays a plain error, so a database failure is not taken for a sign-out.
func (s *AuthService) verifySession(ctx context.Context, userID, sessionID string) (*domain.User, *domain.SessionAuth, error) {
	session, err := s.repo.GetSessionByID(ctx, sessionID, userID)
	if err == sql.ErrNoRows {
		if s.logger != nil {
			s.logger.WithField("user_id", userID).WithField("session_id", sessionID).Error("Session not found")
		}
		return nil, nil, fmt.Errorf("%w: %w", domain.ErrSessionAuthFailed, ErrSessionExpired)
	}
	if err != nil {
		if s.logger != nil {
			s.logger.WithField("user_id", userID).WithField("session_id", sessionID).WithField("error", err.Error()).Error("Failed to query session")
		}
		return nil, nil, err
	}

	if time.Now().After(session.ExpiresAt) {
		if s.logger != nil {
			s.logger.WithField("user_id", userID).WithField("session_id", sessionID).WithField("expires_at", session.ExpiresAt).Error("Session expired")
		}
		return nil, nil, fmt.Errorf("%w: %w", domain.ErrSessionAuthFailed, ErrSessionExpired)
	}

	user, err := s.repo.GetUserByID(ctx, userID)
	if err == sql.ErrNoRows {
		if s.logger != nil {
			s.logger.WithField("user_id", userID).Error("User not found")
		}
		return nil, nil, fmt.Errorf("%w: %w", domain.ErrSessionAuthFailed, ErrUserNotFound)
	}
	if err != nil {
		if s.logger != nil {
			s.logger.WithField("user_id", userID).WithField("error", err.Error()).Error("Failed to query user")
		}
		return nil, nil, err
	}

	// Sessions belong to people. An API key account never signs in, so a session
	// token naming one was not issued by this installation.
	if user.Type != domain.UserTypeUser {
		if s.logger != nil {
			s.logger.WithField("user_id", userID).Error("Session token names an account that is not a user")
		}
		return nil, nil, fmt.Errorf("%w: %w", domain.ErrSessionAuthFailed, ErrUserNotFound)
	}

	return user, session, nil
}

// GenerateUserAuthToken generates an authentication token for a user
func (s *AuthService) GenerateUserAuthToken(user *domain.User, sessionID string, expiresAt time.Time) string {
	secret, err := s.jwtSecret()
	if err != nil {
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
	signed, err := token.SignedString(secret)
	if err != nil && s.logger != nil {
		s.logger.WithField("error", err.Error()).Error("Failed to sign token")
		return ""
	}

	return signed
}

// GenerateAPIAuthToken generates an authentication token for an API key
func (s *AuthService) GenerateAPIAuthToken(user *domain.User) string {
	secret, err := s.jwtSecret()
	if err != nil {
		if s.logger != nil {
			s.logger.WithField("error", err.Error()).Error("Cannot generate API token")
		}
		return ""
	}

	claims := UserClaims{
		UserID: user.ID,
		Email:  user.Email, // Include email for SMTP Bridge authentication
		Type:   string(domain.UserTypeAPIKey),
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour * 24 * 365 * 10)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(secret)
	if err != nil && s.logger != nil {
		s.logger.WithField("error", err.Error()).Error("Failed to sign API token")
		return ""
	}

	return signed
}

// GenerateInvitationToken generates a JWT token for a workspace invitation
func (s *AuthService) GenerateInvitationToken(invitation *domain.WorkspaceInvitation) string {
	secret, err := s.jwtSecret()
	if err != nil {
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
	signed, err := token.SignedString(secret)
	if err != nil && s.logger != nil {
		s.logger.WithField("error", err.Error()).Error("Failed to sign invitation token")
		return ""
	}

	return signed
}

// ValidateInvitationToken validates a JWT invitation token and returns the invitation details
func (s *AuthService) ValidateInvitationToken(tokenString string) (invitationID, workspaceID, email string, err error) {
	secret, err := s.jwtSecret()
	if err != nil {
		return "", "", "", fmt.Errorf("secret not available: %w", err)
	}

	// Every invitation token carries its invitation's expiry, so one without was
	// not issued here: WithExpirationRequired refuses it.
	claims := &InvitationClaims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		// CRITICAL: Verify signing method to prevent algorithm confusion attacks
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return secret, nil
	}, jwt.WithExpirationRequired())

	// CRITICAL: Check both error AND token.Valid
	if err != nil {
		return "", "", "", fmt.Errorf("invalid invitation token: %w", err)
	}
	if !token.Valid {
		return "", "", "", fmt.Errorf("invalid invitation token: token not valid")
	}
	// Every token this installation issues is signed with the same key, and a session
	// token parses here too, naming no invitation.
	if claims.InvitationID == "" || claims.WorkspaceID == "" || claims.Email == "" {
		return "", "", "", fmt.Errorf("invalid invitation token: no invitation named")
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
