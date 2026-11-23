package service

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"errors"
	"fmt"
	"time"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/crypto"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/Notifuse/notifuse/pkg/ratelimiter"
	"github.com/Notifuse/notifuse/pkg/tracing"
	"github.com/google/uuid"
	"go.opencensus.io/trace"
)

type UserService struct {
	repo          domain.UserRepository
	authService   domain.AuthService
	emailSender   EmailSender
	sessionExpiry time.Duration
	logger        logger.Logger
	isProduction  bool
	tracer        tracing.Tracer
	rateLimiter   *ratelimiter.RateLimiter // Global rate limiter with namespace support
	secretKey     string
	rootEmail     string
	// Two-factor authentication: the sign-in transaction and its repositories.
	tx            domain.SystemTxRunner
	authTx        domain.AuthTxRepository
	twoFactorRepo domain.TwoFactorRepository
}

type EmailSender interface {
	SendMagicCode(email, code, language string) error
}

type UserServiceConfig struct {
	Repository    domain.UserRepository
	AuthService   domain.AuthService
	EmailSender   EmailSender
	SessionExpiry time.Duration
	Logger        logger.Logger
	IsProduction  bool
	Tracer        tracing.Tracer
	RateLimiter   *ratelimiter.RateLimiter // Global rate limiter
	SecretKey     string
	RootEmail     string
	Tx            domain.SystemTxRunner
	AuthTx        domain.AuthTxRepository
	TwoFactorRepo domain.TwoFactorRepository
}

func NewUserService(cfg UserServiceConfig) (*UserService, error) {
	// Default to global tracer if none provided
	tracer := cfg.Tracer
	if tracer == nil {
		tracer = tracing.GetTracer()
	}

	return &UserService{
		repo:          cfg.Repository,
		authService:   cfg.AuthService,
		emailSender:   cfg.EmailSender,
		sessionExpiry: cfg.SessionExpiry,
		logger:        cfg.Logger,
		isProduction:  cfg.IsProduction,
		tracer:        tracer,
		rateLimiter:   cfg.RateLimiter, // Global rate limiter
		secretKey:     cfg.SecretKey,
		rootEmail:     cfg.RootEmail,
		tx:            cfg.Tx,
		authTx:        cfg.AuthTx,
		twoFactorRepo: cfg.TwoFactorRepo,
	}, nil
}

// Ensure UserService implements UserServiceInterface
var _ domain.UserServiceInterface = (*UserService)(nil)

func (s *UserService) SignIn(ctx context.Context, input domain.SignInInput) (string, error) {
	ctx, span := s.tracer.StartServiceSpan(ctx, "UserService", "SignIn")
	defer span.End()

	s.tracer.AddAttribute(ctx, "user.email", input.Email)

	// Check rate limit to prevent email bombing and session creation spam
	if s.rateLimiter != nil && !s.rateLimiter.Allow("signin", input.Email) {
		s.logger.WithField("email", input.Email).Warn("Sign-in rate limit exceeded")
		s.tracer.AddAttribute(ctx, "error", "rate_limit_exceeded")
		s.tracer.MarkSpanError(ctx, fmt.Errorf("rate limit exceeded"))
		domain.AuditFromContext(ctx).Fail("rate_limited")
		return "", fmt.Errorf("too many sign-in attempts, please try again in a few minutes")
	}

	// Check if user exists - return error if user not found
	user, err := s.repo.GetUserByEmail(ctx, input.Email)
	if err != nil {
		if _, ok := err.(*domain.ErrUserNotFound); ok {
			// User not found, return error instead of creating new user
			s.logger.WithField("email", input.Email).Error("User does not exist")
			s.tracer.AddAttribute(ctx, "error", "user_not_found")
			s.tracer.MarkSpanError(ctx, err)
			// The response says nothing (no enumeration); the audit log says why.
			domain.AuditFromContext(ctx).Fail("unknown_email")
			return "", &domain.ErrUserNotFound{Message: "user does not exist"}
		}

		s.logger.WithField("email", input.Email).WithField("error", err.Error()).Error("Failed to get user by email")
		s.tracer.MarkSpanError(ctx, err)
		return "", err
	}

	// An API key is a users row with a real, unique email, so without this check
	// /api/user.signin mints a human session for it — and on a non-production
	// instance hands the magic code straight back in the response body. The
	// refusal reuses the not-found error verbatim so the endpoint cannot be used
	// to tell an API-key address apart from an unknown one.
	if user.Type == domain.UserTypeAPIKey {
		notFound := &domain.ErrUserNotFound{Message: "user does not exist"}
		s.logger.WithField("email", input.Email).Warn("Sign-in refused for an API key identity")
		s.tracer.AddAttribute(ctx, "error", "api_key_signin_refused")
		s.tracer.MarkSpanError(ctx, notFound)
		domain.AuditFromContext(ctx).Fail("api_key_identity")
		return "", notFound
	}

	s.tracer.AddAttribute(ctx, "user.id", user.ID)
	s.tracer.AddAttribute(ctx, "action", "use_existing_user")

	// user.signin is the "code requested" half of a magic-code login.
	if audit := domain.AuditFromContext(ctx); audit != nil {
		audit.SetActor(user, nil, false)
		audit.SetAuthMethod(domain.AuditAuthMagicCode)
		audit.AddMetadata("stage", "code_sent")
	}

	// Generate magic code
	plainCode := s.generateMagicCode()
	expiresAt := time.Now().Add(s.sessionExpiry)
	codeExpiresAt := time.Now().Add(15 * time.Minute)

	// Hash the magic code before storing (security: prevent plain-text exposure in database)
	hashedCode := crypto.HashMagicCode(plainCode, s.secretKey)

	// Create new session
	session := &domain.Session{
		ID:               generateID(),
		UserID:           user.ID,
		ExpiresAt:        expiresAt,
		CreatedAt:        time.Now(),
		MagicCode:        &hashedCode,
		MagicCodeExpires: &codeExpiresAt,
	}

	s.tracer.AddAttribute(ctx, "session.id", session.ID)
	s.tracer.AddAttribute(ctx, "session.expires_at", expiresAt.String())

	// Under the user's lock, the sessions still waiting for a code or a second factor past
	// their deadline go: otherwise each sign-in left one behind.
	err = s.tx.WithSystemTx(ctx, func(tx domain.DBExecutor) error {
		if err := s.authTx.LockUser(ctx, tx, user.ID); err != nil {
			return err
		}
		if err := s.authTx.PurgeExpiredPendingSessions(ctx, tx, user.ID); err != nil {
			return err
		}
		return s.authTx.CreateSession(ctx, tx, session)
	})
	if err != nil {
		s.logger.WithField("user_id", user.ID).WithField("error", err.Error()).Error("Failed to create session")
		s.tracer.MarkSpanError(ctx, err)
		return "", err
	}

	// In development/demo mode, return the plain code directly
	// In production, send the plain code via email
	// Note: We return/send the plain code, but store the hashed version in DB
	if !s.isProduction {
		return plainCode, nil
	}

	// Send magic code via email in production
	if err := s.emailSender.SendMagicCode(user.Email, plainCode, user.Language); err != nil {
		s.logger.WithField("user_id", user.ID).WithField("email", user.Email).WithField("error", err.Error()).Error("Failed to send magic code")
		s.tracer.MarkSpanError(ctx, err)
		return "", err
	}

	return "", nil
}

func (s *UserService) VerifyCode(ctx context.Context, input domain.VerifyCodeInput) (*domain.AuthResponse, error) {
	ctx, span := s.tracer.StartServiceSpan(ctx, "UserService", "VerifyCode")
	defer span.End()

	s.tracer.AddAttribute(ctx, "user.email", input.Email)

	// Check rate limit to prevent brute force attacks on magic codes
	if s.rateLimiter != nil && !s.rateLimiter.Allow("verify", input.Email) {
		s.logger.WithField("email", input.Email).Warn("Verify code rate limit exceeded")
		s.tracer.AddAttribute(ctx, "error", "rate_limit_exceeded")
		s.tracer.MarkSpanError(ctx, fmt.Errorf("rate limit exceeded"))
		domain.AuditFromContext(ctx).Fail("rate_limited")
		return nil, fmt.Errorf("too many verification attempts, please try again in a few minutes")
	}

	// Find user by email
	user, err := s.repo.GetUserByEmail(ctx, input.Email)
	if err != nil {
		s.logger.WithField("email", input.Email).WithField("error", err.Error()).Error("Failed to get user by email for code verification")
		s.tracer.MarkSpanError(ctx, err)
		domain.AuditFromContext(ctx).Fail("unknown_email")
		return nil, err
	}

	s.tracer.AddAttribute(ctx, "user.id", user.ID)

	// One transaction, under the user's lock: the code is consumed once even when two
	// requests send it at the same time, and the session either becomes usable or waits for
	// the second factor, never both.
	codeHash := crypto.HashMagicCode(input.Code, s.secretKey)
	var matchingSession *domain.Session
	var challenge *domain.TwoFactorChallenge
	err = s.tx.WithSystemTx(ctx, func(tx domain.DBExecutor) error {
		if err := s.authTx.LockUser(ctx, tx, user.ID); err != nil {
			return err
		}
		totpRow, err := s.twoFactorRepo.GetTOTPForUpdate(ctx, tx, user.ID)
		if err != nil {
			return err
		}
		matchingSession, err = s.authTx.ConsumeMagicCode(ctx, tx, user.ID, codeHash)
		var notFound *domain.ErrSessionNotFound
		if errors.As(err, &notFound) {
			expired, checkErr := s.authTx.HasExpiredMagicCode(ctx, tx, user.ID, codeHash)
			if checkErr != nil {
				return checkErr
			}
			if expired {
				domain.AuditFromContext(ctx).Fail("expired_code")
				return errMagicCodeExpired
			}
			domain.AuditFromContext(ctx).Fail("invalid_code")
			return errInvalidMagicCode
		}
		if err != nil {
			return err
		}
		if err := s.authTx.PurgeExpiredPendingSessions(ctx, tx, user.ID); err != nil {
			return err
		}
		now, err := s.twoFactorRepo.ClockTimestamp(ctx, tx)
		if err != nil {
			return err
		}
		if !totpRow.Enabled() {
			return s.authTx.ActivateSession(ctx, tx, matchingSession.ID, domain.AuthMethodMagicCode, now)
		}
		token, hash, err := NewChallenge()
		if err != nil {
			return err
		}
		expiresAt := now.Add(domain.ChallengeTTL)
		if err := s.authTx.SetSessionChallenge(ctx, tx, matchingSession.ID, domain.AuthMethodMagicCode, hash, expiresAt, nil); err != nil {
			return err
		}
		challenge = &domain.TwoFactorChallenge{Token: token, ExpiresAt: expiresAt}
		return nil
	})
	if err != nil {
		s.logger.WithField("user_id", user.ID).WithField("email", input.Email).WithField("error", err.Error()).Error("Magic code verification failed")
		s.tracer.MarkSpanError(ctx, err)
		return nil, err
	}

	s.tracer.AddAttribute(ctx, "session.id", matchingSession.ID)

	// Reset rate limiter on successful verification
	if s.rateLimiter != nil {
		s.rateLimiter.Reset("verify", input.Email)
	}

	if challenge != nil {
		// The email code was right; the session waits for the second factor.
		if audit := domain.AuditFromContext(ctx); audit != nil {
			audit.SetActor(user, nil, false)
			audit.SetAuthMethod(domain.AuditAuthMagicCode)
			audit.AddMetadata("stage", "second_factor_required")
		}
		return &domain.AuthResponse{User: *user, Challenge: challenge}, nil
	}

	// Generate authentication token
	token := s.authService.GenerateUserAuthToken(user, matchingSession.ID, matchingSession.ExpiresAt)
	s.tracer.AddAttribute(ctx, "token.generated", true)
	s.tracer.AddAttribute(ctx, "token.expires_at", matchingSession.ExpiresAt.String())

	if audit := domain.AuditFromContext(ctx); audit != nil {
		audit.SetActor(user, nil, false)
		audit.SetAuthMethod(domain.AuditAuthMagicCode)
		audit.AddMetadata("session_id", matchingSession.ID)
	}

	return &domain.AuthResponse{
		Token:     token,
		User:      *user,
		ExpiresAt: matchingSession.ExpiresAt,
	}, nil
}

// RootSignin authenticates the root user using HMAC signature.
// This allows programmatic authentication without magic link for automation scenarios.
func (s *UserService) RootSignin(ctx context.Context, input domain.RootSigninInput) (*domain.AuthResponse, error) {
	ctx, span := s.tracer.StartServiceSpan(ctx, "UserService", "RootSignin")
	defer span.End()

	s.tracer.AddAttribute(ctx, "user.email", input.Email)

	// Check rate limit (reuse "signin" namespace)
	if s.rateLimiter != nil && !s.rateLimiter.Allow("signin", input.Email) {
		s.logger.WithField("email", input.Email).Warn("Root sign-in rate limit exceeded")
		s.tracer.AddAttribute(ctx, "error", "rate_limit_exceeded")
		s.tracer.MarkSpanError(ctx, fmt.Errorf("rate limit exceeded"))
		domain.AuditFromContext(ctx).Fail("rate_limited")
		return nil, fmt.Errorf("too many sign-in attempts, please try again in a few minutes")
	}

	// Verify email matches a configured root user
	if !config.IsRootEmail(s.rootEmail, input.Email) {
		s.logger.WithField("email", input.Email).Warn("Root signin attempted with non-root email")
		s.tracer.AddAttribute(ctx, "error", "invalid_credentials")
		domain.AuditFromContext(ctx).Fail("not_root")
		return nil, fmt.Errorf("unauthorized: invalid credentials")
	}

	// Validate timestamp (60-second window to prevent replay attacks)
	now := time.Now().Unix()
	if input.Timestamp < now-60 || input.Timestamp > now+60 {
		s.logger.WithField("email", input.Email).WithField("timestamp", input.Timestamp).Warn("Root signin timestamp out of range")
		s.tracer.AddAttribute(ctx, "error", "invalid_timestamp")
		domain.AuditFromContext(ctx).Fail("stale_timestamp")
		return nil, fmt.Errorf("unauthorized: invalid credentials")
	}

	// Verify HMAC signature using constant-time comparison
	message := fmt.Sprintf("%s:%d", input.Email, input.Timestamp)
	expectedSig := crypto.ComputeHMAC256([]byte(message), s.secretKey)
	if !hmac.Equal([]byte(input.Signature), []byte(expectedSig)) {
		s.logger.WithField("email", input.Email).Warn("Root signin invalid signature")
		s.tracer.AddAttribute(ctx, "error", "invalid_signature")
		domain.AuditFromContext(ctx).Fail("bad_signature")
		return nil, fmt.Errorf("unauthorized: invalid credentials")
	}

	// Get root user
	user, err := s.repo.GetUserByEmail(ctx, input.Email)
	if err != nil {
		s.logger.WithField("email", input.Email).WithField("error", err.Error()).Error("Root signin user not found")
		s.tracer.MarkSpanError(ctx, err)
		domain.AuditFromContext(ctx).Fail("unknown_email")
		return nil, fmt.Errorf("unauthorized: invalid credentials")
	}

	s.tracer.AddAttribute(ctx, "user.id", user.ID)

	// Create session (no magic code needed for root signin)
	session, err := s.startSession(ctx, user.ID, s.sessionExpiry, domain.AuthMethodRootSignature)
	if err != nil {
		s.logger.WithField("user_id", user.ID).WithField("error", err.Error()).Error("Failed to create session for root signin")
		s.tracer.MarkSpanError(ctx, err)
		return nil, err
	}
	expiresAt := session.ExpiresAt

	s.tracer.AddAttribute(ctx, "session.id", session.ID)
	s.tracer.AddAttribute(ctx, "session.expires_at", expiresAt.String())

	// Generate JWT token
	token := s.authService.GenerateUserAuthToken(user, session.ID, expiresAt)
	s.tracer.AddAttribute(ctx, "token.generated", true)

	// Reset rate limiter on success
	if s.rateLimiter != nil {
		s.rateLimiter.Reset("signin", input.Email)
	}

	s.logger.WithField("user_id", user.ID).WithField("email", user.Email).Info("Root user signed in via HMAC")

	if audit := domain.AuditFromContext(ctx); audit != nil {
		audit.SetActor(user, nil, true)
		audit.SetAuthMethod(domain.AuditAuthRootPassword)
		audit.AddMetadata("session_id", session.ID)
	}

	return &domain.AuthResponse{
		Token:     token,
		User:      *user,
		ExpiresAt: expiresAt,
	}, nil
}

// The answers of a magic code that does not open a session. Their text is what the
// console and API clients have always read.
var (
	errInvalidMagicCode = errors.New("invalid magic code")
	errMagicCodeExpired = errors.New("magic code expired")
)

func (s *UserService) generateMagicCode() string {
	// Generate a 6-digit code
	code := make([]byte, 3)
	_, err := rand.Read(code)
	if err != nil {
		s.logger.WithField("error", err.Error()).Error("Failed to generate random bytes for magic code")
		return "123456" // Fallback code in case of error
	}

	// Convert to 6 digits
	codeNum := int(code[0])<<16 | int(code[1])<<8 | int(code[2])
	codeNum = codeNum % 1000000 // Ensure it's 6 digits
	return fmt.Sprintf("%06d", codeNum)
}

// generateID generates a proper UUID
func generateID() string {
	// Use the github.com/google/uuid package to generate a standard UUID
	return uuid.New().String()
}

// startSession opens a usable session for userID that lasts expiry, opened by method,
// without issuing a token for it: the root signature mints its token from it, and the demo
// seed uses it for server-side work that acts as the root user.
func (s *UserService) startSession(ctx context.Context, userID string, expiry time.Duration, method domain.AuthMethod) (*domain.Session, error) {
	now := time.Now()
	session := &domain.Session{
		ID:              generateID(),
		UserID:          userID,
		ExpiresAt:       now.Add(expiry),
		CreatedAt:       now,
		AuthMethod:      method,
		AuthenticatedAt: &now,
	}
	if err := s.repo.CreateSession(ctx, session); err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}
	return session, nil
}

// endSession closes a session startSession opened, once the work it served is done. A
// session already gone, signed out meanwhile, is closed all the same.
func (s *UserService) endSession(ctx context.Context, sessionID string) error {
	err := s.repo.DeleteSession(ctx, sessionID)
	var notFound *domain.ErrSessionNotFound
	if err != nil && !errors.As(err, &notFound) {
		return fmt.Errorf("failed to delete session: %w", err)
	}
	return nil
}

// VerifyUserSession verifies a user session and returns the associated user
func (s *UserService) VerifyUserSession(ctx context.Context, userID string, sessionID string) (*domain.User, error) {
	result, err := s.tracer.TraceMethodWithResultAny(ctx, "UserService", "VerifyUserSession", func(ctx context.Context) (interface{}, error) {
		// Add attributes to the current span
		s.tracer.AddAttribute(ctx, "user.id", userID)
		s.tracer.AddAttribute(ctx, "session.id", sessionID)

		// Failures that mean "not authenticated" are wrapped in
		// domain.ErrSessionAuthFailed; everything else is a lookup that could not be
		// answered and must stay distinguishable, so callers do not sign a user out
		// over a transient database error.

		// First check if the session is valid and not expired
		session, err := s.repo.GetSessionByID(ctx, sessionID)
		if err != nil {
			s.logger.WithField("user_id", userID).WithField("session_id", sessionID).WithField("error", err.Error()).Error("Failed to get session by ID")
			var notFound *domain.ErrSessionNotFound
			if errors.As(err, &notFound) {
				return nil, fmt.Errorf("%w: %w", domain.ErrSessionAuthFailed, err)
			}
			return nil, err
		}

		// Verify that the session belongs to the user
		if session.UserID != userID {
			s.logger.WithField("user_id", userID).WithField("session_id", sessionID).WithField("session_user_id", session.UserID).Error("Session does not belong to user")
			return nil, fmt.Errorf("%w: session does not belong to user", domain.ErrSessionAuthFailed)
		}

		// Check if session is expired
		if time.Now().After(session.ExpiresAt) {
			s.logger.WithField("user_id", userID).WithField("session_id", sessionID).WithField("expires_at", session.ExpiresAt).Error("Session expired")
			return nil, fmt.Errorf("%w: %w", domain.ErrSessionAuthFailed, ErrSessionExpired)
		}

		// Get user details
		user, err := s.repo.GetUserByID(ctx, userID)
		if err != nil {
			s.logger.WithField("user_id", userID).WithField("error", err.Error()).Error("Failed to get user by ID")
			var notFound *domain.ErrUserNotFound
			if errors.As(err, &notFound) {
				return nil, fmt.Errorf("%w: %w", domain.ErrSessionAuthFailed, err)
			}
			return nil, err
		}

		// Sessions belong to people. An API key account never signs in, so a
		// session token naming one was not issued by this installation.
		if user.Type != domain.UserTypeUser {
			s.logger.WithField("user_id", userID).WithField("session_id", sessionID).Error("Session token names an account that is not a user")
			return nil, fmt.Errorf("%w: session token names an account that is not a user", domain.ErrSessionAuthFailed)
		}

		// Add user email to span
		s.tracer.AddAttribute(ctx, "user.email", user.Email)

		return user, nil
	})

	if err != nil {
		return nil, err
	}

	return result.(*domain.User), nil
}

// GetUserByID retrieves a user by their ID
func (s *UserService) GetUserByID(ctx context.Context, userID string) (*domain.User, error) {
	ctx, span := s.tracer.StartServiceSpan(ctx, "UserService", "GetUserByID")
	defer span.End()

	s.tracer.AddAttribute(ctx, "user.id", userID)

	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		s.logger.WithField("user_id", userID).WithField("error", err.Error()).Error("Failed to get user by ID")
		span.SetStatus(trace.Status{
			Code:    trace.StatusCodeNotFound,
			Message: err.Error(),
		})
		return nil, err
	}

	s.tracer.AddAttribute(ctx, "user.email", user.Email)
	return user, nil
}

// GetUserByEmail retrieves a user by their email address
func (s *UserService) GetUserByEmail(ctx context.Context, email string) (*domain.User, error) {
	ctx, span := s.tracer.StartServiceSpan(ctx, "UserService", "GetUserByEmail")
	defer span.End()

	s.tracer.AddAttribute(ctx, "user.email", email)

	user, err := s.repo.GetUserByEmail(ctx, email)
	if err != nil {
		// Check if it's an expected "not found" error vs unexpected error
		if _, ok := err.(*domain.ErrUserNotFound); ok {
			// User not found is expected in some contexts (e.g., invitation acceptance)
			// Log at Info level instead of Error
			s.logger.WithField("email", email).Info("User not found by email")
		} else {
			// Real errors (DB connection, etc.) should be logged as Error
			s.logger.WithField("email", email).WithField("error", err.Error()).Error("Failed to get user by email")
		}
		span.SetStatus(trace.Status{
			Code:    trace.StatusCodeNotFound,
			Message: err.Error(),
		})
		return nil, err
	}

	s.tracer.AddAttribute(ctx, "user.id", user.ID)
	return user, nil
}

// Logout logs out a user by deleting all their sessions
func (s *UserService) Logout(ctx context.Context, userID string) error {
	ctx, span := s.tracer.StartServiceSpan(ctx, "UserService", "Logout")
	defer span.End()

	s.tracer.AddAttribute(ctx, "user.id", userID)

	// Under the user's lock, so that a code or a second factor completing at the same
	// time either ends before and is signed out too, or starts after. Sessions opened by
	// single sign-on or the root signature are inserted without that lock.
	err := s.tx.WithSystemTx(ctx, func(tx domain.DBExecutor) error {
		err := s.authTx.LockUser(ctx, tx, userID)
		var gone *domain.ErrUserNotFound
		if errors.As(err, &gone) {
			// Deleted meanwhile: their sessions went with the account.
			return nil
		}
		if err != nil {
			return err
		}
		return s.authTx.DeleteAllSessions(ctx, tx, userID)
	})
	if err != nil {
		s.logger.WithField("user_id", userID).WithField("error", err.Error()).Error("Failed to delete user sessions")
		s.tracer.MarkSpanError(ctx, err)
		return fmt.Errorf("failed to logout: %w", err)
	}

	s.logger.WithField("user_id", userID).Info("User logged out - all sessions deleted")
	return nil
}

// UpdateUserLanguage updates a user's preferred language for the console UI and system emails
func (s *UserService) UpdateUserLanguage(ctx context.Context, userID string, language string) error {
	ctx, span := s.tracer.StartServiceSpan(ctx, "UserService", "UpdateUserLanguage")
	defer span.End()

	s.tracer.AddAttribute(ctx, "user.id", userID)
	s.tracer.AddAttribute(ctx, "user.language", language)

	if !domain.IsSupportedUILanguage(language) {
		err := &domain.ErrUnsupportedLanguage{Language: language}
		s.tracer.MarkSpanError(ctx, err)
		return err
	}

	if err := s.repo.UpdateUserLanguage(ctx, userID, language); err != nil {
		s.logger.WithField("user_id", userID).WithField("error", err.Error()).Error("Failed to update user language")
		s.tracer.MarkSpanError(ctx, err)
		return err
	}

	return nil
}
