package service

import (
	"context"
	"crypto/rand"
	"fmt"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/Notifuse/notifuse/pkg/tracing"
	"github.com/google/uuid"
	"go.opencensus.io/trace"
)

type UserService struct {
	repo              domain.UserRepository
	authService       domain.AuthService
	emailSender       EmailSender
	sessionExpiry     time.Duration
	logger            logger.Logger
	isProduction      bool
	tracer            tracing.Tracer
	signInLimiter     *RateLimiter
	verifyCodeLimiter *RateLimiter
}

type EmailSender interface {
	SendMagicCode(email, code string) error
}

type UserServiceConfig struct {
	Repository        domain.UserRepository
	AuthService       domain.AuthService
	EmailSender       EmailSender
	SessionExpiry     time.Duration
	Logger            logger.Logger
	IsProduction      bool
	Tracer            tracing.Tracer
	SignInLimiter     *RateLimiter
	VerifyCodeLimiter *RateLimiter
}

func NewUserService(cfg UserServiceConfig) (*UserService, error) {
	// Default to global tracer if none provided
	tracer := cfg.Tracer
	if tracer == nil {
		tracer = tracing.GetTracer()
	}

	return &UserService{
		repo:              cfg.Repository,
		authService:       cfg.AuthService,
		emailSender:       cfg.EmailSender,
		sessionExpiry:     cfg.SessionExpiry,
		logger:            cfg.Logger,
		isProduction:      cfg.IsProduction,
		tracer:            tracer,
		signInLimiter:     cfg.SignInLimiter,
		verifyCodeLimiter: cfg.VerifyCodeLimiter,
	}, nil
}

// Ensure UserService implements UserServiceInterface
var _ domain.UserServiceInterface = (*UserService)(nil)

func (s *UserService) SignIn(ctx context.Context, input domain.SignInInput) (string, error) {
	ctx, span := s.tracer.StartServiceSpan(ctx, "UserService", "SignIn")
	defer span.End()

	s.tracer.AddAttribute(ctx, "user.email", input.Email)

	// Check rate limit to prevent email bombing and session creation spam
	if s.signInLimiter != nil && !s.signInLimiter.Allow(input.Email) {
		s.logger.WithField("email", input.Email).Warn("Sign-in rate limit exceeded")
		s.tracer.AddAttribute(ctx, "error", "rate_limit_exceeded")
		s.tracer.MarkSpanError(ctx, fmt.Errorf("rate limit exceeded"))
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
			return "", &domain.ErrUserNotFound{Message: "user does not exist"}
		}

		s.logger.WithField("email", input.Email).WithField("error", err.Error()).Error("Failed to get user by email")
		s.tracer.MarkSpanError(ctx, err)
		return "", err
	}

	s.tracer.AddAttribute(ctx, "user.id", user.ID)
	s.tracer.AddAttribute(ctx, "action", "use_existing_user")

	// Generate magic code
	code := s.generateMagicCode()
	expiresAt := time.Now().Add(s.sessionExpiry)
	codeExpiresAt := time.Now().Add(15 * time.Minute)

	// Create new session
	session := &domain.Session{
		ID:               generateID(),
		UserID:           user.ID,
		ExpiresAt:        expiresAt,
		CreatedAt:        time.Now(),
		MagicCode:        code,
		MagicCodeExpires: codeExpiresAt,
	}

	s.tracer.AddAttribute(ctx, "session.id", session.ID)
	s.tracer.AddAttribute(ctx, "session.expires_at", expiresAt.String())

	if err := s.repo.CreateSession(ctx, session); err != nil {
		s.logger.WithField("user_id", user.ID).WithField("error", err.Error()).Error("Failed to create session")
		s.tracer.MarkSpanError(ctx, err)
		return "", err
	}

	// In development/demo mode, return the code directly
	// In production, send the code via email
	if !s.isProduction {
		return code, nil
	}

	// Send magic code via email in production
	if err := s.emailSender.SendMagicCode(user.Email, code); err != nil {
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
	if s.verifyCodeLimiter != nil && !s.verifyCodeLimiter.Allow(input.Email) {
		s.logger.WithField("email", input.Email).Warn("Verify code rate limit exceeded")
		s.tracer.AddAttribute(ctx, "error", "rate_limit_exceeded")
		s.tracer.MarkSpanError(ctx, fmt.Errorf("rate limit exceeded"))
		return nil, fmt.Errorf("too many verification attempts, please try again in a few minutes")
	}

	// Find user by email
	user, err := s.repo.GetUserByEmail(ctx, input.Email)
	if err != nil {
		s.logger.WithField("email", input.Email).WithField("error", err.Error()).Error("Failed to get user by email for code verification")
		s.tracer.MarkSpanError(ctx, err)
		return nil, err
	}

	s.tracer.AddAttribute(ctx, "user.id", user.ID)

	// Find all sessions for this user
	sessions, err := s.repo.GetSessionsByUserID(ctx, user.ID)
	if err != nil {
		s.logger.WithField("user_id", user.ID).WithField("error", err.Error()).Error("Failed to get sessions for user")
		s.tracer.MarkSpanError(ctx, err)
		return nil, err
	}

	s.tracer.AddAttribute(ctx, "sessions.count", len(sessions))

	// Find the session with the matching code
	var matchingSession *domain.Session
	for _, session := range sessions {
		if session.MagicCode == input.Code {
			matchingSession = session
			break
		}
	}

	if matchingSession == nil {
		s.logger.WithField("user_id", user.ID).WithField("email", input.Email).Error("Invalid magic code")
		err := fmt.Errorf("invalid magic code")
		s.tracer.MarkSpanError(ctx, err)
		return nil, err
	}

	s.tracer.AddAttribute(ctx, "session.id", matchingSession.ID)

	// Check if magic code is expired
	if time.Now().After(matchingSession.MagicCodeExpires) {
		s.logger.WithField("user_id", user.ID).WithField("email", input.Email).WithField("session_id", matchingSession.ID).Error("Magic code expired")
		err := fmt.Errorf("magic code expired")
		s.tracer.MarkSpanError(ctx, err)
		return nil, err
	}

	// Clear the magic code from the session
	matchingSession.MagicCode = ""
	matchingSession.MagicCodeExpires = time.Time{}

	if err := s.repo.UpdateSession(ctx, matchingSession); err != nil {
		s.logger.WithField("user_id", user.ID).WithField("session_id", matchingSession.ID).WithField("error", err.Error()).Error("Failed to update session")
		s.tracer.MarkSpanError(ctx, err)
		return nil, err
	}

	// Generate authentication token
	token := s.authService.GenerateUserAuthToken(user, matchingSession.ID, matchingSession.ExpiresAt)
	s.tracer.AddAttribute(ctx, "token.generated", true)
	s.tracer.AddAttribute(ctx, "token.expires_at", matchingSession.ExpiresAt.String())

	// Reset rate limiter on successful verification
	if s.verifyCodeLimiter != nil {
		s.verifyCodeLimiter.Reset(input.Email)
	}

	return &domain.AuthResponse{
		Token:     token,
		User:      *user,
		ExpiresAt: matchingSession.ExpiresAt,
	}, nil
}

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

// VerifyUserSession verifies a user session and returns the associated user
func (s *UserService) VerifyUserSession(ctx context.Context, userID string, sessionID string) (*domain.User, error) {
	result, err := s.tracer.TraceMethodWithResultAny(ctx, "UserService", "VerifyUserSession", func(ctx context.Context) (interface{}, error) {
		// Add attributes to the current span
		s.tracer.AddAttribute(ctx, "user.id", userID)
		s.tracer.AddAttribute(ctx, "session.id", sessionID)

		// First check if the session is valid and not expired
		session, err := s.repo.GetSessionByID(ctx, sessionID)
		if err != nil {
			s.logger.WithField("user_id", userID).WithField("session_id", sessionID).WithField("error", err.Error()).Error("Failed to get session by ID")
			return nil, err
		}

		// Verify that the session belongs to the user
		if session.UserID != userID {
			s.logger.WithField("user_id", userID).WithField("session_id", sessionID).WithField("session_user_id", session.UserID).Error("Session does not belong to user")
			return nil, fmt.Errorf("session does not belong to user")
		}

		// Check if session is expired
		if time.Now().After(session.ExpiresAt) {
			s.logger.WithField("user_id", userID).WithField("session_id", sessionID).WithField("expires_at", session.ExpiresAt).Error("Session expired")
			return nil, ErrSessionExpired
		}

		// Get user details
		user, err := s.repo.GetUserByID(ctx, userID)
		if err != nil {
			s.logger.WithField("user_id", userID).WithField("error", err.Error()).Error("Failed to get user by ID")
			return nil, err
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
		s.logger.WithField("email", email).WithField("error", err.Error()).Error("Failed to get user by email")
		span.SetStatus(trace.Status{
			Code:    trace.StatusCodeNotFound,
			Message: err.Error(),
		})
		return nil, err
	}

	s.tracer.AddAttribute(ctx, "user.id", user.ID)
	return user, nil
}
