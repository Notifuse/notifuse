package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/crypto"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/Notifuse/notifuse/pkg/mailer"
	"github.com/Notifuse/notifuse/pkg/totp"
)

// TwoFactorMailer sends the security notices of two-factor authentication.
type TwoFactorMailer interface {
	SendSecurityNotice(email string, notice mailer.SecurityNotice, language string) error
}

// TwoFactorProof is the second factor a request presents: a code from the authenticator app
// or a recovery code. One of them is set.
type TwoFactorProof struct {
	Code         string
	RecoveryCode string
}

// TwoFactorRequestMeta describes the request for the security notices. IP is the address
// the request gave, which a client can forge through X-Forwarded-For.
type TwoFactorRequestMeta struct {
	IP        string
	UserAgent string
}

// TwoFactorStatus is what the account security page shows.
type TwoFactorStatus struct {
	Enabled                bool       `json:"enabled"`
	EnabledAt              *time.Time `json:"enabled_at,omitempty"`
	RecoveryCodesRemaining int        `json:"recovery_codes_remaining"`
}

// TwoFactorSetup is the secret of a setup, shown once: as a QR code and as a key to type.
type TwoFactorSetup struct {
	Secret     string `json:"secret"`
	OtpauthURI string `json:"otpauth_uri"`
}

// Invitation outcomes a sign-in challenge reports when it carried an invitation.
const (
	InvitationAccepted        = "accepted"
	InvitationTeamMemberLimit = "team_member_limit"
	InvitationUnavailable     = "invitation_unavailable"
)

// ErrCannotResetOwnTwoFactor: the root user resets other people's two-factor
// authentication, never their own.
var ErrCannotResetOwnTwoFactor = errors.New("you cannot reset your own two-factor authentication")

// TwoFactorServiceConfig holds the dependencies of TwoFactorService.
type TwoFactorServiceConfig struct {
	Tx               domain.SystemTxRunner
	Repo             domain.TwoFactorRepository
	AuthTx           domain.AuthTxRepository
	InvitationTx     domain.InvitationTxRepository
	Users            domain.UserRepository
	AuthService      domain.AuthService
	Mailer           TwoFactorMailer
	Logger           logger.Logger
	SecretKey        string
	PreviousKeys     []string
	RequireTwoFactor bool
	IsRootEmail      func(email string) bool
	// MaxUsers is MAX_USERS, checked when a sign-in challenge accepts an invitation.
	MaxUsers int
	// Issuer returns the name authenticator apps show for a new enrolment: the instance brand
	// while a licence covers it. Read at each enrolment, so a brand or licence change needs no
	// restart. nil, or an empty answer, means "Notifuse". Accounts already enrolled keep the
	// label their app stored.
	Issuer func() string
}

// TwoFactorService runs two-factor authentication: setup, sign-in challenges, recovery
// codes and resets. Each operation is one system transaction that takes, in this order,
// the user's row, user_totp, the recovery codes, the sessions, then invitations and
// memberships. The users lock serializes every transaction about one person, so the rest
// of the order cannot deadlock between them; a reset takes two users' rows by ID order.
//
// Deadlines are compared with ClockTimestamp, read after the transaction's last lock.
// Notices are sent after the commit, never after a rollback.
type TwoFactorService struct {
	tx               domain.SystemTxRunner
	repo             domain.TwoFactorRepository
	authTx           domain.AuthTxRepository
	invitationTx     domain.InvitationTxRepository
	users            domain.UserRepository
	authService      domain.AuthService
	mailer           TwoFactorMailer
	logger           logger.Logger
	keys             []string
	requireTwoFactor bool
	isRootEmail      func(email string) bool
	maxUsers         int
	issuer           func() string
}

// NewTwoFactorService builds a TwoFactorService.
func NewTwoFactorService(cfg TwoFactorServiceConfig) *TwoFactorService {
	return &TwoFactorService{
		tx:               cfg.Tx,
		repo:             cfg.Repo,
		authTx:           cfg.AuthTx,
		invitationTx:     cfg.InvitationTx,
		users:            cfg.Users,
		authService:      cfg.AuthService,
		mailer:           cfg.Mailer,
		logger:           cfg.Logger,
		keys:             append([]string{cfg.SecretKey}, cfg.PreviousKeys...),
		requireTwoFactor: cfg.RequireTwoFactor,
		isRootEmail:      cfg.IsRootEmail,
		maxUsers:         cfg.MaxUsers,
		issuer:           cfg.Issuer,
	}
}

// RequireTwoFactor reports whether the instance requires two-factor authentication.
func (s *TwoFactorService) RequireTwoFactor() bool {
	return s.requireTwoFactor
}

var secretEncoding = base32.StdEncoding.WithPadding(base32.NoPadding)

// NewChallenge returns a sign-in challenge token, to give the client once, and the hash to
// store: 32 random bytes, of which only the SHA-256 is kept.
func NewChallenge() (token, hash string, err error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", "", fmt.Errorf("failed to generate a challenge: %w", err)
	}
	token = base64.RawURLEncoding.EncodeToString(raw)
	return token, HashChallenge(token), nil
}

// HashChallenge is the stored form of a challenge token.
func HashChallenge(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// run runs fn in a system transaction and reports whether it committed: fn returned nil
// or CommitAndFail, and the commit itself succeeded. Notices are sent only for a commit.
func (s *TwoFactorService) run(ctx context.Context, fn func(tx domain.DBExecutor) error) (bool, error) {
	var fnErr error
	err := s.tx.WithSystemTx(ctx, func(tx domain.DBExecutor) error {
		fnErr = fn(tx)
		return fnErr
	})
	if fnErr == nil {
		return err == nil, err
	}
	business, commit := domain.AsCommitAndFail(fnErr)
	return commit && err == business, err
}

// Status returns the user's two-factor authentication state.
func (s *TwoFactorService) Status(ctx context.Context, userID string) (*TwoFactorStatus, error) {
	status := &TwoFactorStatus{}
	err := s.tx.WithSystemTx(ctx, func(tx domain.DBExecutor) error {
		t, err := s.repo.GetTOTP(ctx, tx, userID)
		if err != nil || !t.Enabled() {
			return err
		}
		status.Enabled, status.EnabledAt = true, t.EnabledAt
		status.RecoveryCodesRemaining, err = s.repo.CountRemainingRecoveryCodes(ctx, tx, userID)
		return err
	})
	return status, err
}

// UsersWithoutTwoFactor lists the emails of the people who have not turned it on.
func (s *TwoFactorService) UsersWithoutTwoFactor(ctx context.Context) ([]string, error) {
	var emails []string
	err := s.tx.WithSystemTx(ctx, func(tx domain.DBExecutor) error {
		var err error
		emails, err = s.repo.ListUsersWithoutTwoFactor(ctx, tx)
		return err
	})
	return emails, err
}

// Setup starts a setup: a new secret, kept apart from any active one until Enable confirms
// it. Turning two-factor authentication on for the first time needs a session opened less
// than FreshSessionWindow ago; replacing the app needs a proof.
func (s *TwoFactorService) Setup(ctx context.Context, userID, sessionID string, proof TwoFactorProof, meta TwoFactorRequestMeta) (*TwoFactorSetup, error) {
	user, err := s.users.GetUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	aad, err := domain.TOTPAAD(userID)
	if err != nil {
		return nil, err
	}
	var setup *TwoFactorSetup
	var outcome proofOutcome
	committed, err := s.run(ctx, func(tx domain.DBExecutor) error {
		if err := s.authTx.LockUser(ctx, tx, userID); err != nil {
			return err
		}
		t, err := s.repo.GetTOTPForUpdate(ctx, tx, userID)
		if err != nil {
			return err
		}
		var now time.Time
		if t.Enabled() {
			if now, err = s.repo.ClockTimestamp(ctx, tx); err != nil {
				return err
			}
			if outcome, err = s.checkProof(ctx, tx, userID, t, proof, now, false); err != nil {
				return err
			}
		} else {
			session, err := s.authTx.GetSessionForUpdate(ctx, tx, userID, sessionID)
			if err != nil {
				return sessionGone(err)
			}
			if now, err = s.repo.ClockTimestamp(ctx, tx); err != nil {
				return err
			}
			if !sessionIsFresh(session.AuthMethod, session.AuthenticatedAt, now) {
				domain.AuditFromContext(ctx).Fail("not_fresh")
				return domain.ErrReauthenticationRequired
			}
		}
		secret, err := totp.GenerateSecret()
		if err != nil {
			return err
		}
		encoded := totp.EncodeSecret(secret)
		sealed, err := crypto.EncryptStringAAD(encoded, s.keys[0], aad)
		if err != nil {
			return err
		}
		if err := s.repo.SavePending(ctx, tx, userID, sealed, now.Add(domain.PendingSetupTTL), now); err != nil {
			return err
		}
		setup = &TwoFactorSetup{Secret: encoded, OtpauthURI: totp.KeyURI(s.issuerName(), user.Email, secret)}
		return nil
	})
	if committed {
		s.afterProof(ctx, user, outcome, meta)
	}
	if err != nil {
		return nil, err
	}
	return setup, nil
}

// sessionGone turns a session that disappeared during the request (signed out from another
// tab, its user reset) into an authentication failure: 401, as for a session already gone.
func sessionGone(err error) error {
	var notFound *domain.ErrSessionNotFound
	if errors.As(err, &notFound) {
		return fmt.Errorf("%w: %w", domain.ErrSessionAuthFailed, err)
	}
	return err
}

// sessionIsFresh reports whether a session may turn on two-factor authentication without
// another proof: opened less than FreshSessionWindow ago, or by the root signature.
func sessionIsFresh(method domain.AuthMethod, authenticatedAt *time.Time, now time.Time) bool {
	if method == domain.AuthMethodRootSignature {
		return true
	}
	return authenticatedAt != nil && authenticatedAt.After(now.Add(-domain.FreshSessionWindow))
}

// Enable confirms a setup with a code from the app. On a first setup it returns the
// recovery codes, shown once; replacing the app keeps the codes and returns none. The
// current session is marked as having passed the second factor, and the user's other
// sessions are ended when signOutOthers.
func (s *TwoFactorService) Enable(ctx context.Context, userID, sessionID, code string, signOutOthers bool, meta TwoFactorRequestMeta) ([]string, error) {
	user, err := s.users.GetUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	aad, err := domain.TOTPAAD(userID)
	if err != nil {
		return nil, err
	}
	var codes []string
	firstTime := false
	err = s.tx.WithSystemTx(ctx, func(tx domain.DBExecutor) error {
		if err := s.authTx.LockUser(ctx, tx, userID); err != nil {
			return err
		}
		t, err := s.repo.GetTOTPForUpdate(ctx, tx, userID)
		if err != nil {
			return err
		}
		now, err := s.repo.ClockTimestamp(ctx, tx)
		if err != nil {
			return err
		}
		if t == nil || t.PendingSecretEncrypted == nil || t.PendingExpiresAt == nil || !t.PendingExpiresAt.After(now) {
			domain.AuditFromContext(ctx).Fail("setup_expired")
			return domain.ErrTwoFactorSetupExpired
		}
		encoded, keyIndex, err := crypto.DecryptWithKeysAAD(*t.PendingSecretEncrypted, s.keys, aad)
		if err != nil {
			return fmt.Errorf("failed to open the pending secret: %w", err)
		}
		secret, err := secretEncoding.DecodeString(encoded)
		if err != nil {
			return fmt.Errorf("failed to decode the pending secret: %w", err)
		}
		step, result := totp.Match(secret, strings.ReplaceAll(code, " ", ""), now, 0)
		if result != totp.MatchOK {
			dropped, err := s.repo.RecordPendingFailure(ctx, tx, userID, domain.MaxPendingSetupAttempts, now)
			if err != nil {
				return err
			}
			if dropped {
				// The last attempt this setup allowed: its secret is gone, a new one is needed.
				domain.AuditFromContext(ctx).Fail("setup_expired")
				return domain.CommitAndFail(domain.ErrTwoFactorSetupExpired)
			}
			domain.AuditFromContext(ctx).Fail("invalid_code")
			return domain.CommitAndFail(domain.ErrTwoFactorInvalidCode)
		}
		// The secret stays sealed as it is (same user, same associated data), unless a
		// previous key sealed it.
		sealed := *t.PendingSecretEncrypted
		if keyIndex != 0 {
			if sealed, err = crypto.EncryptStringAAD(encoded, s.keys[0], aad); err != nil {
				return err
			}
		}
		firstTime = !t.Enabled()
		if err := s.repo.Enable(ctx, tx, userID, sealed, step, now); err != nil {
			return err
		}
		if firstTime {
			if codes, err = s.replaceRecoveryCodes(ctx, tx, userID, now); err != nil {
				return err
			}
		}
		if err := s.authTx.SetSessionSecondFactor(ctx, tx, userID, sessionID, domain.SecondFactorTOTP, now); err != nil {
			return sessionGone(err)
		}
		if signOutOthers {
			return s.authTx.DeleteSessionsExcept(ctx, tx, userID, sessionID)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if firstTime {
		s.notify(ctx, user, mailer.SecurityNoticeTwoFactorEnabled, meta, 0)
	} else {
		// A new app now holds the second factor: the owner hears of it, as of any change.
		s.notify(ctx, user, mailer.SecurityNoticeTwoFactorReplaced, meta, 0)
	}
	return codes, nil
}

// Disable turns two-factor authentication off, with a proof. It is refused while the
// instance requires it. Every session of the user forgets the second factor it passed, so
// that none of them meets a requirement switched on later.
func (s *TwoFactorService) Disable(ctx context.Context, userID string, proof TwoFactorProof, meta TwoFactorRequestMeta) error {
	if s.requireTwoFactor {
		return domain.ErrTwoFactorRequiredByInstance
	}
	user, err := s.users.GetUserByID(ctx, userID)
	if err != nil {
		return err
	}
	var outcome proofOutcome
	committed, err := s.run(ctx, func(tx domain.DBExecutor) error {
		if err := s.authTx.LockUser(ctx, tx, userID); err != nil {
			return err
		}
		t, err := s.repo.GetTOTPForUpdate(ctx, tx, userID)
		if err != nil {
			return err
		}
		if !t.Enabled() {
			return nil
		}
		now, err := s.repo.ClockTimestamp(ctx, tx)
		if err != nil {
			return err
		}
		if outcome, err = s.checkProof(ctx, tx, userID, t, proof, now, false); err != nil {
			return err
		}
		if err := s.repo.DeleteTOTP(ctx, tx, userID); err != nil {
			return err
		}
		if err := s.repo.DeleteRecoveryCodes(ctx, tx, userID); err != nil {
			return err
		}
		outcome.disabled = true
		return s.authTx.ClearSecondFactorOnSessions(ctx, tx, userID)
	})
	if committed {
		s.afterProof(ctx, user, outcome, meta)
	}
	if err != nil {
		return err
	}
	if outcome.disabled {
		s.notify(ctx, user, mailer.SecurityNoticeTwoFactorDisabled, meta, 0)
	}
	return nil
}

// RegenerateRecoveryCodes replaces the recovery codes, with a proof, and returns the new
// ones, shown once.
func (s *TwoFactorService) RegenerateRecoveryCodes(ctx context.Context, userID string, proof TwoFactorProof, meta TwoFactorRequestMeta) ([]string, error) {
	user, err := s.users.GetUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	var codes []string
	var outcome proofOutcome
	committed, err := s.run(ctx, func(tx domain.DBExecutor) error {
		if err := s.authTx.LockUser(ctx, tx, userID); err != nil {
			return err
		}
		t, err := s.repo.GetTOTPForUpdate(ctx, tx, userID)
		if err != nil {
			return err
		}
		if !t.Enabled() {
			return domain.ErrTwoFactorSetupExpired
		}
		now, err := s.repo.ClockTimestamp(ctx, tx)
		if err != nil {
			return err
		}
		if outcome, err = s.checkProof(ctx, tx, userID, t, proof, now, false); err != nil {
			return err
		}
		codes, err = s.replaceRecoveryCodes(ctx, tx, userID, now)
		return err
	})
	// The new codes replace the one used: only the notice of new codes is sent.
	outcome.recoveryCodeUsed = false
	if committed {
		s.afterProof(ctx, user, outcome, meta)
	}
	if err != nil {
		return nil, err
	}
	s.notify(ctx, user, mailer.SecurityNoticeRecoveryCodesNew, meta, 0)
	return codes, nil
}

// VerifyChallenge completes a sign-in that waits for its second factor, and returns the
// token of the session. When the challenge carried an invitation, the invitation is
// accepted in the same transaction, and the outcome is in InvitationStatus.
func (s *TwoFactorService) VerifyChallenge(ctx context.Context, challengeToken string, proof TwoFactorProof, meta TwoFactorRequestMeta) (*domain.AuthResponse, error) {
	if challengeToken == "" || len(challengeToken) > 64 {
		// Nobody to name: a scanner sending random tokens must not fill the audit log.
		domain.AuditFromContext(ctx).Skip()
		return nil, domain.ErrTwoFactorChallengeInvalid
	}
	hash := HashChallenge(challengeToken)
	audit := domain.AuditFromContext(ctx)

	var user *domain.User
	var session *domain.Session
	var outcome proofOutcome
	invitationStatus := ""
	committed, err := s.run(ctx, func(tx domain.DBExecutor) error {
		userID, err := s.authTx.FindChallengeUser(ctx, tx, hash)
		if errors.Is(err, domain.ErrTwoFactorChallengeInvalid) {
			// No challenge ever had this token: nobody to name, and a scanner sending
			// random tokens must not fill the audit log.
			audit.Skip()
			return err
		}
		if err != nil {
			return err
		}
		if user, err = s.users.GetUserByID(ctx, userID); err != nil {
			return err
		}
		audit.SetActor(user, nil, false)
		if err := s.authTx.LockUser(ctx, tx, userID); err != nil {
			return err
		}
		t, err := s.repo.GetTOTPForUpdate(ctx, tx, userID)
		if err != nil {
			return err
		}
		// The session before the recovery codes: the challenge must be checked (expiry,
		// attempts) before a code is consumed. Every transaction about this person holds the
		// users lock first, so this order cannot deadlock against the documented one.
		if session, err = s.authTx.GetChallengeSessionForUpdate(ctx, tx, userID, hash); err != nil {
			audit.Fail("challenge_invalid")
			return err
		}
		now, err := s.repo.ClockTimestamp(ctx, tx)
		if err != nil {
			return err
		}
		if session.ChallengeExpiresAt == nil || !session.ChallengeExpiresAt.After(now) ||
			session.ChallengeAttempts >= domain.MaxChallengeAttempts || !t.Enabled() {
			audit.Fail("challenge_invalid")
			return domain.ErrTwoFactorChallengeInvalid
		}
		outcome, err = s.checkProof(ctx, tx, userID, t, proof, now, true)
		if _, counted := domain.AsCommitAndFail(err); counted {
			if failErr := s.authTx.FailChallenge(ctx, tx, session.ID); failErr != nil {
				return failErr
			}
			return err
		}
		if err != nil {
			return err
		}
		if err := s.authTx.CompleteChallenge(ctx, tx, session.ID, outcome.factor, now); err != nil {
			return err
		}
		audit.AddMetadata("second_factor", string(outcome.factor))
		if session.PendingInvitationID != nil {
			invitationStatus, err = s.acceptPendingInvitation(ctx, tx, user, *session.PendingInvitationID)
			return err
		}
		return nil
	})
	if committed {
		s.afterProof(ctx, user, outcome, meta)
	}
	if err != nil {
		return nil, err
	}
	token := s.authService.GenerateUserAuthToken(user, session.ID, session.ExpiresAt)
	return &domain.AuthResponse{
		Token:            token,
		User:             *user,
		ExpiresAt:        session.ExpiresAt,
		InvitationStatus: invitationStatus,
	}, nil
}

// acceptPendingInvitation accepts, at the end of a sign-in challenge, the invitation that
// started it: still there, not expired, for this person, and with a seat left. An
// existing membership is never rewritten. It returns the outcome; the sign-in succeeds
// whatever it is, since the second factor was valid.
func (s *TwoFactorService) acceptPendingInvitation(ctx context.Context, tx domain.DBExecutor, user *domain.User, invitationID string) (string, error) {
	userID := user.ID
	invitation, expired, err := s.invitationTx.LockInvitation(ctx, tx, invitationID)
	if errors.Is(err, domain.ErrInvitationNotFound) {
		return InvitationUnavailable, nil
	}
	if err != nil {
		return "", err
	}
	if expired || !strings.EqualFold(invitation.Email, user.Email) {
		return InvitationUnavailable, nil
	}
	member, err := s.invitationTx.IsUserWorkspaceMember(ctx, tx, userID, invitation.WorkspaceID)
	if err != nil {
		return "", err
	}
	if member {
		// Already in the workspace, as when the link is accepted twice: the invitation has
		// nothing left to open, as for an invitee without two-factor authentication.
		if err := s.invitationTx.DeleteInvitationsByEmail(ctx, tx, invitation.WorkspaceID, invitation.Email); err != nil {
			return "", err
		}
		return InvitationUnavailable, nil
	}
	if s.maxUsers > 0 {
		count, err := s.invitationTx.CountWorkspaceMembersAndInvitations(ctx, tx, invitation.WorkspaceID, invitation.Email)
		if err != nil {
			return "", err
		}
		if count >= s.maxUsers {
			return InvitationTeamMemberLimit, nil
		}
	}
	// The invitations go before the membership is written: workspace_invitations comes
	// before user_workspaces in the lock order.
	if err := s.invitationTx.DeleteInvitationsByEmail(ctx, tx, invitation.WorkspaceID, invitation.Email); err != nil {
		return "", err
	}
	now := time.Now().UTC()
	if _, err := s.invitationTx.AddUserToWorkspaceIfAbsent(ctx, tx, &domain.UserWorkspace{
		UserID:      userID,
		WorkspaceID: invitation.WorkspaceID,
		Role:        "member",
		Permissions: invitation.Permissions,
		CreatedAt:   now,
		UpdatedAt:   now,
	}); err != nil {
		return "", err
	}
	return InvitationAccepted, nil
}

// ResetForUser removes the two-factor authentication of another person and ends their
// sessions: the root user's answer to someone who lost both their phone and their recovery
// codes. A root user who has two-factor authentication proves it with a code; one who has
// not needs a fresh session, unless it was opened by the root signature.
func (s *TwoFactorService) ResetForUser(ctx context.Context, rootUserID, rootSessionID string, rootProof TwoFactorProof, targetUserID, targetEmail string, meta TwoFactorRequestMeta) error {
	root, err := s.users.GetUserByID(ctx, rootUserID)
	if err != nil {
		return err
	}
	if s.isRootEmail == nil || root.Type != domain.UserTypeUser || !s.isRootEmail(root.Email) {
		return &domain.ErrUnauthorized{Message: "root user access required"}
	}
	var target *domain.User
	if targetUserID != "" {
		target, err = s.users.GetUserByID(ctx, targetUserID)
	} else {
		target, err = s.users.GetUserByEmailInsensitive(ctx, targetEmail)
	}
	if err != nil {
		return err
	}
	if target.Type != domain.UserTypeUser {
		return &domain.ErrUserNotFound{Message: "user not found"}
	}
	if target.ID == root.ID {
		return ErrCannotResetOwnTwoFactor
	}
	domain.AuditFromContext(ctx).SetTarget(domain.AuditTargetUser, target.ID, target.Email)

	var outcome proofOutcome
	committed, err := s.run(ctx, func(tx domain.DBExecutor) error {
		ids := []string{root.ID, target.ID}
		sort.Strings(ids)
		for _, id := range ids {
			if err := s.authTx.LockUser(ctx, tx, id); err != nil {
				return err
			}
		}
		totps := map[string]*domain.UserTOTP{}
		for _, id := range ids {
			t, err := s.repo.GetTOTPForUpdate(ctx, tx, id)
			if err != nil {
				return err
			}
			totps[id] = t
		}
		var rootSession *domain.Session
		if !totps[root.ID].Enabled() {
			var err error
			if rootSession, err = s.authTx.GetSessionForUpdate(ctx, tx, root.ID, rootSessionID); err != nil {
				return sessionGone(err)
			}
		}
		now, err := s.repo.ClockTimestamp(ctx, tx)
		if err != nil {
			return err
		}
		if totps[root.ID].Enabled() {
			if outcome, err = s.checkProof(ctx, tx, root.ID, totps[root.ID], rootProof, now, false); err != nil {
				return err
			}
		} else if !sessionIsFresh(rootSession.AuthMethod, rootSession.AuthenticatedAt, now) {
			domain.AuditFromContext(ctx).Fail("not_fresh")
			return domain.ErrReauthenticationRequired
		}
		if err := s.repo.DeleteTOTP(ctx, tx, target.ID); err != nil {
			return err
		}
		if err := s.repo.DeleteRecoveryCodes(ctx, tx, target.ID); err != nil {
			return err
		}
		return s.authTx.DeleteAllSessions(ctx, tx, target.ID)
	})
	if committed {
		s.afterProof(ctx, root, outcome, meta)
	}
	if err != nil {
		return err
	}
	s.notify(ctx, target, mailer.SecurityNoticeTwoFactorReset, meta, 0)
	return nil
}

// proofOutcome is what checkProof found, read after the transaction to send notices.
type proofOutcome struct {
	factor           domain.SecondFactor
	recoveryCodeUsed bool
	recoveryLeft     int
	// alert is set by the failure that reached the start of the waits: the owner is told.
	alert    bool
	disabled bool
}

// checkProof checks a code from the app or a recovery code against the user's locked
// user_totp row, and records the success or the failure. A wrong code returns
// CommitAndFail(ErrTwoFactorInvalidCode): the failure counters must persist. A refusal
// before any check (the account is waiting, or authenticator codes are locked) changes
// nothing and is returned as it is.
//
// At sign-in, a recovery code is accepted whatever the wait and the hard lock: it is the
// way out for the legitimate person, and 120 bits are not guessed. Elsewhere it waits like
// a code from the app, so a session cannot try codes in bursts.
func (s *TwoFactorService) checkProof(ctx context.Context, tx domain.DBExecutor, userID string, t *domain.UserTOTP, proof TwoFactorProof, now time.Time, atSignIn bool) (proofOutcome, error) {
	var outcome proofOutcome
	audit := domain.AuditFromContext(ctx)
	useRecovery := proof.RecoveryCode != ""
	if !useRecovery && proof.Code == "" {
		audit.Fail("invalid_code")
		return outcome, domain.ErrTwoFactorInvalidCode
	}
	if !useRecovery || !atSignIn {
		if !useRecovery && t.FailedAttempts >= domain.TOTPHardLockThreshold {
			audit.Fail("totp_locked")
			return outcome, domain.ErrTwoFactorTOTPLocked
		}
		if t.LockedUntil != nil && t.LockedUntil.After(now) {
			audit.Fail("locked")
			return outcome, &domain.TwoFactorLockedError{RetryAfter: t.LockedUntil.Sub(now)}
		}
	}

	reason := "invalid_code"
	if useRecovery {
		if normalized, err := domain.NormalizeRecoveryCode(proof.RecoveryCode); err == nil {
			used, err := s.repo.ConsumeRecoveryCode(ctx, tx, userID, domain.HashRecoveryCode(userID, normalized), now)
			if err != nil {
				return outcome, err
			}
			if used {
				if err := s.repo.RecordSuccess(ctx, tx, userID, nil, now); err != nil {
					return outcome, err
				}
				left, err := s.repo.CountRemainingRecoveryCodes(ctx, tx, userID)
				if err != nil {
					return outcome, err
				}
				outcome.factor, outcome.recoveryCodeUsed, outcome.recoveryLeft = domain.SecondFactorRecoveryCode, true, left
				return outcome, nil
			}
		}
	} else {
		aad, err := domain.TOTPAAD(userID)
		if err != nil {
			return outcome, err
		}
		encoded, _, err := crypto.DecryptWithKeysAAD(*t.SecretEncrypted, s.keys, aad)
		if err != nil {
			return outcome, fmt.Errorf("failed to open the authenticator secret: %w", err)
		}
		secret, err := secretEncoding.DecodeString(encoded)
		if err != nil {
			return outcome, fmt.Errorf("failed to decode the authenticator secret: %w", err)
		}
		step, result := totp.Match(secret, strings.ReplaceAll(proof.Code, " ", ""), now, t.LastUsedStep)
		if result == totp.MatchOK {
			if err := s.repo.RecordSuccess(ctx, tx, userID, &step, now); err != nil {
				return outcome, err
			}
			outcome.factor = domain.SecondFactorTOTP
			return outcome, nil
		}
		if result == totp.MatchReplay {
			reason = "replayed_code"
		}
	}

	failures := t.FailedAttempts + 1
	if err := s.repo.RecordFailure(ctx, tx, userID, failures, domain.NextLockout(failures, now), now); err != nil {
		return outcome, err
	}
	outcome.alert = failures == 5
	audit.Fail(reason)
	return outcome, domain.CommitAndFail(domain.ErrTwoFactorInvalidCode)
}

// afterProof sends the notices a proof calls for. Callers run it only once the transaction
// committed: a wrong code commits its counters, a success commits with the operation, and
// a rollback sends nothing.
func (s *TwoFactorService) afterProof(ctx context.Context, user *domain.User, outcome proofOutcome, meta TwoFactorRequestMeta) {
	if user == nil {
		return
	}
	if outcome.recoveryCodeUsed {
		s.notify(ctx, user, mailer.SecurityNoticeRecoveryCodeUsed, meta, outcome.recoveryLeft)
	}
	if outcome.alert {
		s.notify(ctx, user, mailer.SecurityNoticeTwoFactorCodeFailures, meta, 0)
	}
}

// notify sends a security notice; a failure is logged and never undoes the operation.
func (s *TwoFactorService) notify(ctx context.Context, user *domain.User, kind mailer.SecurityNoticeKind, meta TwoFactorRequestMeta, recoveryLeft int) {
	if s.mailer == nil || user == nil {
		return
	}
	err := s.mailer.SendSecurityNotice(user.Email, mailer.SecurityNotice{
		Kind: kind, At: time.Now(), IP: meta.IP, UserAgent: meta.UserAgent, RecoveryCodesLeft: recoveryLeft,
	}, user.Language)
	if err != nil && s.logger != nil {
		s.logger.WithField("user_id", user.ID).WithField("notice", string(kind)).WithField("error", err.Error()).
			Error("Failed to send a two-factor security notice")
	}
}

// replaceRecoveryCodes generates a new set of recovery codes, stores their hashes in place
// of the user's codes, and returns them to show once.
func (s *TwoFactorService) replaceRecoveryCodes(ctx context.Context, tx domain.DBExecutor, userID string, now time.Time) ([]string, error) {
	codes, err := domain.GenerateRecoveryCodes(rand.Reader)
	if err != nil {
		return nil, err
	}
	hashes := make([]string, 0, len(codes))
	for _, code := range codes {
		normalized, err := domain.NormalizeRecoveryCode(code)
		if err != nil {
			return nil, err
		}
		hashes = append(hashes, domain.HashRecoveryCode(userID, normalized))
	}
	if err := s.repo.ReplaceRecoveryCodes(ctx, tx, userID, hashes, now); err != nil {
		return nil, err
	}
	return codes, nil
}

// issuerName is the name a new enrolment shows in the authenticator app.
func (s *TwoFactorService) issuerName() string {
	if s.issuer != nil {
		if name := s.issuer(); name != "" {
			return name
		}
	}
	return "Notifuse"
}
