package http

import (
	"context"
	"errors"
	"fmt"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/service"
)

// sessionViaUserService answers AuthenticateSession through UserServiceInterface's
// VerifyUserSession, which the handler tests written before 45.0 mock: their expectations
// keep describing the session check. The session it returns is a magic-code one; session,
// when set, replaces it.
type sessionViaUserService struct {
	domain.AuthService
	users   UserServiceInterface
	session *domain.SessionAuth
	rootErr error
}

// RequireCompliantRootSession answers rootErr: nil unless a test sets it.
func (s *sessionViaUserService) RequireCompliantRootSession(context.Context) error { return s.rootErr }

func (s *sessionViaUserService) AuthenticateSession(ctx context.Context) (*domain.User, *domain.SessionAuth, error) {
	userID, _ := ctx.Value(domain.UserIDKey).(string)
	sessionID, _ := ctx.Value(domain.SessionIDKey).(string)
	if userID == "" || sessionID == "" {
		return nil, nil, fmt.Errorf("%w: missing claims", domain.ErrSessionAuthFailed)
	}
	user, err := s.users.VerifyUserSession(ctx, userID, sessionID)
	if err != nil {
		return nil, nil, err
	}
	if s.session != nil {
		return user, s.session, nil
	}
	return user, &domain.SessionAuth{AuthMethod: domain.AuthMethodMagicCode}, nil
}

// stubTwoFactor is a two-factor service whose answers a test sets. Calls nobody set fail.
type stubTwoFactor struct {
	status       *service.TwoFactorStatus
	required     bool
	usersWithout []string

	setup   func(userID, sessionID string, proof service.TwoFactorProof) (*service.TwoFactorSetup, error)
	enable  func(userID, sessionID, code string, signOut bool) ([]string, error)
	disable func(userID string, proof service.TwoFactorProof) error
	regen   func(userID string, proof service.TwoFactorProof) ([]string, error)
	verify  func(token string, proof service.TwoFactorProof, meta service.TwoFactorRequestMeta) (*domain.AuthResponse, error)
	reset   func(rootID, rootSession string, proof service.TwoFactorProof, userID, email string) error
}

var errNotStubbed = errors.New("not stubbed")

func (s *stubTwoFactor) Status(context.Context, string) (*service.TwoFactorStatus, error) {
	if s.status == nil {
		return &service.TwoFactorStatus{}, nil
	}
	return s.status, nil
}
func (s *stubTwoFactor) RequireTwoFactor() bool { return s.required }
func (s *stubTwoFactor) UsersWithoutTwoFactor(context.Context) ([]string, error) {
	return s.usersWithout, nil
}
func (s *stubTwoFactor) Setup(_ context.Context, userID, sessionID string, proof service.TwoFactorProof, _ service.TwoFactorRequestMeta) (*service.TwoFactorSetup, error) {
	if s.setup == nil {
		return nil, errNotStubbed
	}
	return s.setup(userID, sessionID, proof)
}
func (s *stubTwoFactor) Enable(_ context.Context, userID, sessionID, code string, signOut bool, _ service.TwoFactorRequestMeta) ([]string, error) {
	if s.enable == nil {
		return nil, errNotStubbed
	}
	return s.enable(userID, sessionID, code, signOut)
}
func (s *stubTwoFactor) Disable(_ context.Context, userID string, proof service.TwoFactorProof, _ service.TwoFactorRequestMeta) error {
	if s.disable == nil {
		return errNotStubbed
	}
	return s.disable(userID, proof)
}
func (s *stubTwoFactor) RegenerateRecoveryCodes(_ context.Context, userID string, proof service.TwoFactorProof, _ service.TwoFactorRequestMeta) ([]string, error) {
	if s.regen == nil {
		return nil, errNotStubbed
	}
	return s.regen(userID, proof)
}
func (s *stubTwoFactor) VerifyChallenge(_ context.Context, token string, proof service.TwoFactorProof, meta service.TwoFactorRequestMeta) (*domain.AuthResponse, error) {
	if s.verify == nil {
		return nil, errNotStubbed
	}
	return s.verify(token, proof, meta)
}
func (s *stubTwoFactor) ResetForUser(_ context.Context, rootID, rootSession string, proof service.TwoFactorProof, userID, email string, _ service.TwoFactorRequestMeta) error {
	if s.reset == nil {
		return errNotStubbed
	}
	return s.reset(rootID, rootSession, proof, userID, email)
}
