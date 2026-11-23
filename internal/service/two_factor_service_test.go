package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/Notifuse/notifuse/pkg/crypto"
	"github.com/Notifuse/notifuse/pkg/mailer"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
	"github.com/Notifuse/notifuse/pkg/totp"
)

const (
	tfKey     = "two-factor-service-test-key"
	tfUserID  = "3b2f6a1c-8d4e-4f5a-9b7c-1e2d3f4a5b6c"
	tfRootID  = "0c1d2e3f-4a5b-4c6d-8e7f-9a0b1c2d3e4f"
	tfSession = "session-1"
)

type sentNotice struct {
	email  string
	notice mailer.SecurityNotice
}

type recordingMailer struct{ sent []sentNotice }

func (m *recordingMailer) SendSecurityNotice(email string, notice mailer.SecurityNotice, _ string) error {
	m.sent = append(m.sent, sentNotice{email, notice})
	return nil
}

func (m *recordingMailer) kinds() []mailer.SecurityNoticeKind {
	var kinds []mailer.SecurityNoticeKind
	for _, s := range m.sent {
		kinds = append(kinds, s.notice.Kind)
	}
	return kinds
}

type twoFactorFixture struct {
	svc    *TwoFactorService
	tx     *fakeTx
	repo   *mocks.MockTwoFactorRepository
	authTx *mocks.MockAuthTxRepository
	inv    *mocks.MockInvitationTxRepository
	users  *mocks.MockUserRepository
	auth   *mocks.MockAuthService
	mail   *recordingMailer
	now    time.Time
	secret []byte
	user   *domain.User
	root   *domain.User
}

func newTwoFactorFixture(t *testing.T, mutate ...func(*TwoFactorServiceConfig)) *twoFactorFixture {
	ctrl := gomock.NewController(t)
	secret, err := totp.GenerateSecret()
	require.NoError(t, err)
	f := &twoFactorFixture{
		tx:     &fakeTx{},
		repo:   mocks.NewMockTwoFactorRepository(ctrl),
		authTx: mocks.NewMockAuthTxRepository(ctrl),
		inv:    mocks.NewMockInvitationTxRepository(ctrl),
		users:  mocks.NewMockUserRepository(ctrl),
		auth:   mocks.NewMockAuthService(ctrl),
		mail:   &recordingMailer{},
		// Mid-step, so that the previous and next steps are distinct codes.
		now:    time.Unix(1_800_000_015, 0).UTC(),
		secret: secret,
		user:   &domain.User{ID: tfUserID, Email: "ann@example.com", Type: domain.UserTypeUser, Language: "fr"},
		root:   &domain.User{ID: tfRootID, Email: "root@example.com", Type: domain.UserTypeUser},
	}
	log := pkgmocks.NewMockLogger(ctrl)
	log.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(log).AnyTimes()
	log.EXPECT().Error(gomock.Any()).AnyTimes()
	cfg := TwoFactorServiceConfig{
		Tx: f.tx, Repo: f.repo, AuthTx: f.authTx, InvitationTx: f.inv, Users: f.users,
		AuthService: f.auth, Mailer: f.mail, Logger: log, SecretKey: tfKey,
		IsRootEmail: func(email string) bool { return email == "root@example.com" },
	}
	for _, m := range mutate {
		m(&cfg)
	}
	f.svc = NewTwoFactorService(cfg)
	f.users.EXPECT().GetUserByID(gomock.Any(), tfUserID).Return(f.user, nil).AnyTimes()
	f.users.EXPECT().GetUserByID(gomock.Any(), tfRootID).Return(f.root, nil).AnyTimes()
	f.repo.EXPECT().ClockTimestamp(gomock.Any(), gomock.Any()).Return(f.now, nil).AnyTimes()
	return f
}

func (f *twoFactorFixture) seal(t *testing.T, userID string, secret []byte, key string) *string {
	t.Helper()
	aad, err := domain.TOTPAAD(userID)
	require.NoError(t, err)
	sealed, err := crypto.EncryptStringAAD(totp.EncodeSecret(secret), key, aad)
	require.NoError(t, err)
	return &sealed
}

// enabled returns an active authenticator row of userID sealed with the test key.
func (f *twoFactorFixture) enabled(t *testing.T, userID string) *domain.UserTOTP {
	at := f.now.Add(-24 * time.Hour)
	return &domain.UserTOTP{UserID: userID, SecretEncrypted: f.seal(t, userID, f.secret, tfKey), EnabledAt: &at}
}

func (f *twoFactorFixture) code(offset int64) string {
	return totp.Code(f.secret, totp.Step(f.now)+offset)
}

func (f *twoFactorFixture) lock(userID string, row *domain.UserTOTP) {
	f.authTx.EXPECT().LockUser(gomock.Any(), gomock.Any(), userID).Return(nil)
	f.repo.EXPECT().GetTOTPForUpdate(gomock.Any(), gomock.Any(), userID).Return(row, nil)
}

// --- Sign-in challenge ---------------------------------------------------------------

func (f *twoFactorFixture) expectChallenge(t *testing.T, row *domain.UserTOTP, session *domain.Session) string {
	token, hash, err := NewChallenge()
	require.NoError(t, err)
	f.authTx.EXPECT().FindChallengeUser(gomock.Any(), gomock.Any(), hash).Return(tfUserID, nil)
	f.lock(tfUserID, row)
	f.authTx.EXPECT().GetChallengeSessionForUpdate(gomock.Any(), gomock.Any(), tfUserID, hash).Return(session, nil)
	return token
}

func (f *twoFactorFixture) challengeSession() *domain.Session {
	expires := f.now.Add(5 * time.Minute)
	return &domain.Session{ID: tfSession, UserID: tfUserID, ExpiresAt: f.now.Add(30 * 24 * time.Hour), ChallengeExpiresAt: &expires}
}

func auditCtx() (context.Context, *domain.AuditRecord) {
	record := domain.NewAuditRecord("203.0.113.1", "test", "req-1")
	return domain.WithAuditRecord(context.Background(), record), record
}

func TestTwoFactorService_VerifyChallenge(t *testing.T) {
	t.Run("a right code completes the sign-in and records the step", func(t *testing.T) {
		f := newTwoFactorFixture(t)
		token := f.expectChallenge(t, f.enabled(t, tfUserID), f.challengeSession())
		step := totp.Step(f.now)
		f.repo.EXPECT().RecordSuccess(gomock.Any(), gomock.Any(), tfUserID, &step, f.now).Return(nil)
		f.authTx.EXPECT().CompleteChallenge(gomock.Any(), gomock.Any(), tfSession, domain.SecondFactorTOTP, f.now).Return(nil)
		f.auth.EXPECT().GenerateUserAuthToken(f.user, tfSession, gomock.Any()).Return("jwt")

		ctx, record := auditCtx()
		resp, err := f.svc.VerifyChallenge(ctx, token, TwoFactorProof{Code: f.code(0)}, TwoFactorRequestMeta{})
		require.NoError(t, err)
		assert.Equal(t, "jwt", resp.Token)
		assert.Empty(t, resp.InvitationStatus)
		assert.Empty(t, f.mail.sent)
		event, _ := record.Finalize(200)
		assert.Equal(t, "totp", event.Metadata["second_factor"])
		assert.Equal(t, tfUserID, event.ActorID)
	})

	// A wrong code answers an error, and its counters must persist: the transaction commits.
	t.Run("a wrong code commits the failure on the account and on the challenge", func(t *testing.T) {
		f := newTwoFactorFixture(t)
		token := f.expectChallenge(t, f.enabled(t, tfUserID), f.challengeSession())
		f.repo.EXPECT().RecordFailure(gomock.Any(), gomock.Any(), tfUserID, 1, nil, f.now).Return(nil)
		f.authTx.EXPECT().FailChallenge(gomock.Any(), gomock.Any(), tfSession).Return(nil)

		ctx, record := auditCtx()
		_, err := f.svc.VerifyChallenge(ctx, token, TwoFactorProof{Code: "000000"}, TwoFactorRequestMeta{})
		require.ErrorIs(t, err, domain.ErrTwoFactorInvalidCode)
		assert.Equal(t, 1, f.tx.commits)
		assert.Zero(t, f.tx.rollbacks)
		assert.Empty(t, f.mail.sent)
		event, _ := record.Finalize(422)
		assert.Equal(t, "invalid_code", event.Metadata["reason"])
	})

	t.Run("the fifth failure starts the waits and tells the owner", func(t *testing.T) {
		f := newTwoFactorFixture(t)
		row := f.enabled(t, tfUserID)
		row.FailedAttempts = 4
		token := f.expectChallenge(t, row, f.challengeSession())
		f.repo.EXPECT().RecordFailure(gomock.Any(), gomock.Any(), tfUserID, 5, domain.NextLockout(5, f.now), f.now).Return(nil)
		f.authTx.EXPECT().FailChallenge(gomock.Any(), gomock.Any(), tfSession).Return(nil)

		_, err := f.svc.VerifyChallenge(context.Background(), token, TwoFactorProof{Code: "000000"}, TwoFactorRequestMeta{IP: "198.51.100.7"})
		require.ErrorIs(t, err, domain.ErrTwoFactorInvalidCode)
		require.Equal(t, []mailer.SecurityNoticeKind{mailer.SecurityNoticeTwoFactorCodeFailures}, f.mail.kinds())
		assert.Equal(t, "198.51.100.7", f.mail.sent[0].notice.IP)
	})

	t.Run("a code already used is refused as a replay", func(t *testing.T) {
		f := newTwoFactorFixture(t)
		row := f.enabled(t, tfUserID)
		row.LastUsedStep = totp.Step(f.now)
		token := f.expectChallenge(t, row, f.challengeSession())
		f.repo.EXPECT().RecordFailure(gomock.Any(), gomock.Any(), tfUserID, 1, nil, f.now).Return(nil)
		f.authTx.EXPECT().FailChallenge(gomock.Any(), gomock.Any(), tfSession).Return(nil)

		ctx, record := auditCtx()
		_, err := f.svc.VerifyChallenge(ctx, token, TwoFactorProof{Code: f.code(0)}, TwoFactorRequestMeta{})
		require.ErrorIs(t, err, domain.ErrTwoFactorInvalidCode)
		event, _ := record.Finalize(422)
		assert.Equal(t, "replayed_code", event.Metadata["reason"])
	})

	t.Run("during a wait, a code is refused without touching anything", func(t *testing.T) {
		f := newTwoFactorFixture(t)
		row := f.enabled(t, tfUserID)
		until := f.now.Add(90 * time.Second)
		row.FailedAttempts, row.LockedUntil = 6, &until
		token := f.expectChallenge(t, row, f.challengeSession())

		_, err := f.svc.VerifyChallenge(context.Background(), token, TwoFactorProof{Code: f.code(0)}, TwoFactorRequestMeta{})
		var locked *domain.TwoFactorLockedError
		require.ErrorAs(t, err, &locked)
		assert.Equal(t, 90*time.Second, locked.RetryAfter)
		assert.Equal(t, 1, f.tx.rollbacks)
	})

	// At sign-in a recovery code is the legitimate person's way out of a wait or a lock.
	t.Run("past the hard lock, a recovery code still signs in and resets the counter", func(t *testing.T) {
		f := newTwoFactorFixture(t)
		row := f.enabled(t, tfUserID)
		until := f.now.Add(time.Hour)
		row.FailedAttempts, row.LockedUntil = domain.TOTPHardLockThreshold, &until
		token := f.expectChallenge(t, row, f.challengeSession())
		code := "abcdef-ghjkmn-pqrstv-wxyz01"
		normalized, err := domain.NormalizeRecoveryCode(code)
		require.NoError(t, err)
		f.repo.EXPECT().ConsumeRecoveryCode(gomock.Any(), gomock.Any(), tfUserID, domain.HashRecoveryCode(tfUserID, normalized), f.now).Return(true, nil)
		f.repo.EXPECT().RecordSuccess(gomock.Any(), gomock.Any(), tfUserID, nil, f.now).Return(nil)
		f.repo.EXPECT().CountRemainingRecoveryCodes(gomock.Any(), gomock.Any(), tfUserID).Return(9, nil)
		f.authTx.EXPECT().CompleteChallenge(gomock.Any(), gomock.Any(), tfSession, domain.SecondFactorRecoveryCode, f.now).Return(nil)
		f.auth.EXPECT().GenerateUserAuthToken(f.user, tfSession, gomock.Any()).Return("jwt")

		resp, err := f.svc.VerifyChallenge(context.Background(), token, TwoFactorProof{RecoveryCode: code}, TwoFactorRequestMeta{})
		require.NoError(t, err)
		assert.Equal(t, "jwt", resp.Token)
		require.Equal(t, []mailer.SecurityNoticeKind{mailer.SecurityNoticeRecoveryCodeUsed}, f.mail.kinds())
		assert.Equal(t, 9, f.mail.sent[0].notice.RecoveryCodesLeft)
	})

	t.Run("past the hard lock, an authenticator code is refused", func(t *testing.T) {
		f := newTwoFactorFixture(t)
		row := f.enabled(t, tfUserID)
		row.FailedAttempts = domain.TOTPHardLockThreshold
		token := f.expectChallenge(t, row, f.challengeSession())

		_, err := f.svc.VerifyChallenge(context.Background(), token, TwoFactorProof{Code: f.code(0)}, TwoFactorRequestMeta{})
		require.ErrorIs(t, err, domain.ErrTwoFactorTOTPLocked)
	})

	t.Run("a used-up or expired challenge is refused", func(t *testing.T) {
		for name, mutate := range map[string]func(*domain.Session, time.Time){
			"used up": func(s *domain.Session, _ time.Time) { s.ChallengeAttempts = domain.MaxChallengeAttempts },
			"expired": func(s *domain.Session, now time.Time) { past := now.Add(-time.Second); s.ChallengeExpiresAt = &past },
		} {
			t.Run(name, func(t *testing.T) {
				f := newTwoFactorFixture(t)
				session := f.challengeSession()
				mutate(session, f.now)
				token := f.expectChallenge(t, f.enabled(t, tfUserID), session)

				ctx, record := auditCtx()
				_, err := f.svc.VerifyChallenge(ctx, token, TwoFactorProof{Code: f.code(0)}, TwoFactorRequestMeta{})
				require.ErrorIs(t, err, domain.ErrTwoFactorChallengeInvalid)
				event, _ := record.Finalize(401)
				assert.Equal(t, "challenge_invalid", event.Metadata["reason"])
			})
		}
	})

	t.Run("an unknown token is not audited", func(t *testing.T) {
		f := newTwoFactorFixture(t)
		f.authTx.EXPECT().FindChallengeUser(gomock.Any(), gomock.Any(), gomock.Any()).Return("", domain.ErrTwoFactorChallengeInvalid)

		ctx, record := auditCtx()
		_, err := f.svc.VerifyChallenge(ctx, "some-token", TwoFactorProof{Code: "123456"}, TwoFactorRequestMeta{})
		require.ErrorIs(t, err, domain.ErrTwoFactorChallengeInvalid)
		_, skipped := record.Finalize(401)
		assert.True(t, skipped)

		ctx, record = auditCtx()
		_, err = f.svc.VerifyChallenge(ctx, "", TwoFactorProof{Code: "123456"}, TwoFactorRequestMeta{})
		require.ErrorIs(t, err, domain.ErrTwoFactorChallengeInvalid)
		_, skipped = record.Finalize(401)
		assert.True(t, skipped)
	})

	// The recovery code was consumed in a transaction that rolled back: it is still there,
	// and its owner is not told it was used.
	t.Run("no notice after a rollback", func(t *testing.T) {
		f := newTwoFactorFixture(t)
		token := f.expectChallenge(t, f.enabled(t, tfUserID), f.challengeSession())
		f.repo.EXPECT().ConsumeRecoveryCode(gomock.Any(), gomock.Any(), tfUserID, gomock.Any(), f.now).Return(true, nil)
		f.repo.EXPECT().RecordSuccess(gomock.Any(), gomock.Any(), tfUserID, nil, f.now).Return(nil)
		f.repo.EXPECT().CountRemainingRecoveryCodes(gomock.Any(), gomock.Any(), tfUserID).Return(9, nil)
		sentinel := errors.New("write failed")
		f.authTx.EXPECT().CompleteChallenge(gomock.Any(), gomock.Any(), tfSession, domain.SecondFactorRecoveryCode, f.now).Return(sentinel)

		_, err := f.svc.VerifyChallenge(context.Background(), token, TwoFactorProof{RecoveryCode: "abcdef-ghjkmn-pqrstv-wxyz01"}, TwoFactorRequestMeta{})
		require.ErrorIs(t, err, sentinel)
		assert.Empty(t, f.mail.sent)
	})

	t.Run("no notice when the commit fails", func(t *testing.T) {
		f := newTwoFactorFixture(t)
		f.tx.commitErr = errors.New("commit failed")
		row := f.enabled(t, tfUserID)
		row.FailedAttempts = 4
		token := f.expectChallenge(t, row, f.challengeSession())
		f.repo.EXPECT().RecordFailure(gomock.Any(), gomock.Any(), tfUserID, 5, gomock.Any(), f.now).Return(nil)
		f.authTx.EXPECT().FailChallenge(gomock.Any(), gomock.Any(), tfSession).Return(nil)

		_, err := f.svc.VerifyChallenge(context.Background(), token, TwoFactorProof{Code: "000000"}, TwoFactorRequestMeta{})
		require.EqualError(t, err, "commit failed")
		assert.Empty(t, f.mail.sent)
	})
}

func TestTwoFactorService_VerifyChallenge_PendingInvitation(t *testing.T) {
	invitationID := "inv-1"
	invitation := &domain.WorkspaceInvitation{ID: invitationID, WorkspaceID: "ws1", Email: "Ann@Example.com", Permissions: domain.UserPermissions{}}

	setup := func(t *testing.T, maxUsers int) (*twoFactorFixture, string) {
		f := newTwoFactorFixture(t, func(c *TwoFactorServiceConfig) { c.MaxUsers = maxUsers })
		session := f.challengeSession()
		session.PendingInvitationID = &invitationID
		token := f.expectChallenge(t, f.enabled(t, tfUserID), session)
		f.repo.EXPECT().RecordSuccess(gomock.Any(), gomock.Any(), tfUserID, gomock.Any(), f.now).Return(nil)
		f.authTx.EXPECT().CompleteChallenge(gomock.Any(), gomock.Any(), tfSession, domain.SecondFactorTOTP, f.now).Return(nil)
		f.auth.EXPECT().GenerateUserAuthToken(f.user, tfSession, gomock.Any()).Return("jwt")
		return f, token
	}

	t.Run("accepted once the second factor passes", func(t *testing.T) {
		f, token := setup(t, 0)
		f.inv.EXPECT().LockInvitation(gomock.Any(), gomock.Any(), invitationID).Return(invitation, false, nil)
		f.inv.EXPECT().IsUserWorkspaceMember(gomock.Any(), gomock.Any(), tfUserID, "ws1").Return(false, nil)
		f.inv.EXPECT().AddUserToWorkspaceIfAbsent(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, _ domain.DBExecutor, uw *domain.UserWorkspace) (bool, error) {
				assert.Equal(t, "member", uw.Role)
				return true, nil
			})
		f.inv.EXPECT().DeleteInvitationsByEmail(gomock.Any(), gomock.Any(), "ws1", invitation.Email).Return(nil)

		resp, err := f.svc.VerifyChallenge(context.Background(), token, TwoFactorProof{Code: f.code(0)}, TwoFactorRequestMeta{})
		require.NoError(t, err)
		assert.Equal(t, InvitationAccepted, resp.InvitationStatus)
	})

	// The second factor was valid: the sign-in succeeds, the invitation stays.
	t.Run("the member limit reached meanwhile keeps the invitation", func(t *testing.T) {
		f, token := setup(t, 3)
		f.inv.EXPECT().LockInvitation(gomock.Any(), gomock.Any(), invitationID).Return(invitation, false, nil)
		f.inv.EXPECT().IsUserWorkspaceMember(gomock.Any(), gomock.Any(), tfUserID, "ws1").Return(false, nil)
		f.inv.EXPECT().CountWorkspaceMembersAndInvitations(gomock.Any(), gomock.Any(), "ws1", invitation.Email).Return(3, nil)

		resp, err := f.svc.VerifyChallenge(context.Background(), token, TwoFactorProof{Code: f.code(0)}, TwoFactorRequestMeta{})
		require.NoError(t, err)
		assert.Equal(t, InvitationTeamMemberLimit, resp.InvitationStatus)
		assert.Equal(t, "jwt", resp.Token)
	})

	// The link accepted twice, or by someone already in the workspace: nothing to open,
	// as for an invitee without two-factor authentication. The membership is not rewritten.
	t.Run("an invitee already a member gets invitation_unavailable", func(t *testing.T) {
		f, token := setup(t, 0)
		f.inv.EXPECT().LockInvitation(gomock.Any(), gomock.Any(), invitationID).Return(invitation, false, nil)
		f.inv.EXPECT().IsUserWorkspaceMember(gomock.Any(), gomock.Any(), tfUserID, "ws1").Return(true, nil)
		f.inv.EXPECT().DeleteInvitationsByEmail(gomock.Any(), gomock.Any(), "ws1", invitation.Email).Return(nil)

		resp, err := f.svc.VerifyChallenge(context.Background(), token, TwoFactorProof{Code: f.code(0)}, TwoFactorRequestMeta{})
		require.NoError(t, err)
		assert.Equal(t, InvitationUnavailable, resp.InvitationStatus)
	})

	t.Run("an invitation gone or expired meanwhile is reported", func(t *testing.T) {
		f, token := setup(t, 0)
		f.inv.EXPECT().LockInvitation(gomock.Any(), gomock.Any(), invitationID).Return(invitation, true, nil)

		resp, err := f.svc.VerifyChallenge(context.Background(), token, TwoFactorProof{Code: f.code(0)}, TwoFactorRequestMeta{})
		require.NoError(t, err)
		assert.Equal(t, InvitationUnavailable, resp.InvitationStatus)
	})
}

// --- Setup and Enable ----------------------------------------------------------------

func TestTwoFactorService_Setup(t *testing.T) {
	t.Run("a first setup needs a fresh session", func(t *testing.T) {
		f := newTwoFactorFixture(t)
		f.lock(tfUserID, nil)
		old := f.now.Add(-domain.FreshSessionWindow - time.Minute)
		f.authTx.EXPECT().GetSessionForUpdate(gomock.Any(), gomock.Any(), tfUserID, tfSession).
			Return(&domain.Session{ID: tfSession, AuthMethod: domain.AuthMethodMagicCode, AuthenticatedAt: &old}, nil)

		ctx, record := auditCtx()
		_, err := f.svc.Setup(ctx, tfUserID, tfSession, TwoFactorProof{}, TwoFactorRequestMeta{})
		require.ErrorIs(t, err, domain.ErrReauthenticationRequired)
		event, _ := record.Finalize(403)
		assert.Equal(t, "not_fresh", event.Metadata["reason"])
	})

	t.Run("a fresh session gets a secret sealed for this user", func(t *testing.T) {
		f := newTwoFactorFixture(t)
		f.lock(tfUserID, nil)
		recent := f.now.Add(-time.Minute)
		f.authTx.EXPECT().GetSessionForUpdate(gomock.Any(), gomock.Any(), tfUserID, tfSession).
			Return(&domain.Session{ID: tfSession, AuthMethod: domain.AuthMethodMagicCode, AuthenticatedAt: &recent}, nil)
		var sealed string
		f.repo.EXPECT().SavePending(gomock.Any(), gomock.Any(), tfUserID, gomock.Any(), f.now.Add(domain.PendingSetupTTL), f.now).
			DoAndReturn(func(_ context.Context, _ domain.DBExecutor, _, s string, _, _ time.Time) error {
				sealed = s
				return nil
			})

		setup, err := f.svc.Setup(context.Background(), tfUserID, tfSession, TwoFactorProof{}, TwoFactorRequestMeta{})
		require.NoError(t, err)
		aad, _ := domain.TOTPAAD(tfUserID)
		opened, err := crypto.DecryptFromHexStringAAD(sealed, tfKey, aad)
		require.NoError(t, err)
		assert.Equal(t, setup.Secret, opened)
		assert.Contains(t, setup.OtpauthURI, "otpauth://totp/")
		otherAAD, _ := domain.TOTPAAD(tfRootID)
		_, err = crypto.DecryptFromHexStringAAD(sealed, tfKey, otherAAD)
		assert.Error(t, err, "bound to its user")
	})

	t.Run("a session opened by the root signature counts as fresh", func(t *testing.T) {
		f := newTwoFactorFixture(t)
		f.lock(tfUserID, nil)
		f.authTx.EXPECT().GetSessionForUpdate(gomock.Any(), gomock.Any(), tfUserID, tfSession).
			Return(&domain.Session{ID: tfSession, AuthMethod: domain.AuthMethodRootSignature}, nil)
		f.repo.EXPECT().SavePending(gomock.Any(), gomock.Any(), tfUserID, gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

		_, err := f.svc.Setup(context.Background(), tfUserID, tfSession, TwoFactorProof{}, TwoFactorRequestMeta{})
		require.NoError(t, err)
	})

	t.Run("replacing the app needs a code from the current one", func(t *testing.T) {
		f := newTwoFactorFixture(t)
		f.lock(tfUserID, f.enabled(t, tfUserID))
		f.repo.EXPECT().RecordFailure(gomock.Any(), gomock.Any(), tfUserID, 1, nil, f.now).Return(nil)

		_, err := f.svc.Setup(context.Background(), tfUserID, tfSession, TwoFactorProof{Code: "000000"}, TwoFactorRequestMeta{})
		require.ErrorIs(t, err, domain.ErrTwoFactorInvalidCode)
		assert.Equal(t, 1, f.tx.commits, "the failure is counted")
	})
}

func TestTwoFactorService_Enable(t *testing.T) {
	pending := func(f *twoFactorFixture, t *testing.T, enabled bool) *domain.UserTOTP {
		row := &domain.UserTOTP{UserID: tfUserID}
		if enabled {
			row = f.enabled(t, tfUserID)
		}
		expires := f.now.Add(5 * time.Minute)
		row.PendingSecretEncrypted, row.PendingExpiresAt = f.seal(t, tfUserID, f.secret, tfKey), &expires
		return row
	}

	t.Run("a first setup returns recovery codes, marks the session and tells the owner", func(t *testing.T) {
		f := newTwoFactorFixture(t)
		row := pending(f, t, false)
		f.lock(tfUserID, row)
		f.repo.EXPECT().Enable(gomock.Any(), gomock.Any(), tfUserID, *row.PendingSecretEncrypted, totp.Step(f.now), f.now).Return(nil)
		var hashes []string
		f.repo.EXPECT().ReplaceRecoveryCodes(gomock.Any(), gomock.Any(), tfUserID, gomock.Any(), f.now).
			DoAndReturn(func(_ context.Context, _ domain.DBExecutor, _ string, h []string, _ time.Time) error {
				hashes = h
				return nil
			})
		f.authTx.EXPECT().SetSessionSecondFactor(gomock.Any(), gomock.Any(), tfUserID, tfSession, domain.SecondFactorTOTP, f.now).Return(nil)
		f.authTx.EXPECT().DeleteSessionsExcept(gomock.Any(), gomock.Any(), tfUserID, tfSession).Return(nil)

		codes, err := f.svc.Enable(context.Background(), tfUserID, tfSession, f.code(0), true, TwoFactorRequestMeta{})
		require.NoError(t, err)
		require.Len(t, codes, domain.RecoveryCodeCount)
		require.Len(t, hashes, domain.RecoveryCodeCount)
		normalized, err := domain.NormalizeRecoveryCode(codes[0])
		require.NoError(t, err)
		assert.Equal(t, domain.HashRecoveryCode(tfUserID, normalized), hashes[0], "only hashes are stored")
		assert.Equal(t, []mailer.SecurityNoticeKind{mailer.SecurityNoticeTwoFactorEnabled}, f.mail.kinds())
	})

	t.Run("replacing the app keeps the recovery codes", func(t *testing.T) {
		f := newTwoFactorFixture(t)
		f.lock(tfUserID, pending(f, t, true))
		f.repo.EXPECT().Enable(gomock.Any(), gomock.Any(), tfUserID, gomock.Any(), gomock.Any(), f.now).Return(nil)
		f.authTx.EXPECT().SetSessionSecondFactor(gomock.Any(), gomock.Any(), tfUserID, tfSession, domain.SecondFactorTOTP, f.now).Return(nil)

		codes, err := f.svc.Enable(context.Background(), tfUserID, tfSession, f.code(0), false, TwoFactorRequestMeta{})
		require.NoError(t, err)
		assert.Empty(t, codes)
		assert.Equal(t, []mailer.SecurityNoticeKind{mailer.SecurityNoticeTwoFactorReplaced}, f.mail.kinds(), "a new app holds the second factor: the owner hears of it")
	})

	t.Run("a wrong code counts against the setup and commits", func(t *testing.T) {
		f := newTwoFactorFixture(t)
		f.lock(tfUserID, pending(f, t, false))
		f.repo.EXPECT().RecordPendingFailure(gomock.Any(), gomock.Any(), tfUserID, domain.MaxPendingSetupAttempts, f.now).Return(false, nil)

		_, err := f.svc.Enable(context.Background(), tfUserID, tfSession, "000000", false, TwoFactorRequestMeta{})
		require.ErrorIs(t, err, domain.ErrTwoFactorInvalidCode)
		assert.Equal(t, 1, f.tx.commits)
		assert.Empty(t, f.mail.sent)
	})

	// The last attempt the setup allowed drops its secret: the answer says so, and the wizard
	// shows a new QR code rather than asking for another code from a dead secret.
	t.Run("the attempt that drops the setup answers setup_expired", func(t *testing.T) {
		f := newTwoFactorFixture(t)
		f.lock(tfUserID, pending(f, t, false))
		f.repo.EXPECT().RecordPendingFailure(gomock.Any(), gomock.Any(), tfUserID, domain.MaxPendingSetupAttempts, f.now).Return(true, nil)

		ctx, record := auditCtx()
		_, err := f.svc.Enable(ctx, tfUserID, tfSession, "000000", false, TwoFactorRequestMeta{})
		require.ErrorIs(t, err, domain.ErrTwoFactorSetupExpired)
		assert.Equal(t, 1, f.tx.commits)
		event, _ := record.Finalize(410)
		assert.Equal(t, "setup_expired", event.Metadata["reason"])
	})

	// A session signed out from another tab meanwhile: 401, as for a session already gone.
	t.Run("a session gone meanwhile is an authentication failure", func(t *testing.T) {
		f := newTwoFactorFixture(t)
		f.lock(tfUserID, pending(f, t, false))
		f.repo.EXPECT().Enable(gomock.Any(), gomock.Any(), tfUserID, gomock.Any(), gomock.Any(), f.now).Return(nil)
		f.repo.EXPECT().ReplaceRecoveryCodes(gomock.Any(), gomock.Any(), tfUserID, gomock.Any(), f.now).Return(nil)
		f.authTx.EXPECT().SetSessionSecondFactor(gomock.Any(), gomock.Any(), tfUserID, tfSession, domain.SecondFactorTOTP, f.now).
			Return(&domain.ErrSessionNotFound{Message: "session not found"})

		_, err := f.svc.Enable(context.Background(), tfUserID, tfSession, f.code(0), false, TwoFactorRequestMeta{})
		require.ErrorIs(t, err, domain.ErrSessionAuthFailed)
		assert.Equal(t, 1, f.tx.rollbacks)
	})

	t.Run("an expired setup is refused", func(t *testing.T) {
		f := newTwoFactorFixture(t)
		row := pending(f, t, false)
		past := f.now.Add(-time.Second)
		row.PendingExpiresAt = &past
		f.lock(tfUserID, row)

		_, err := f.svc.Enable(context.Background(), tfUserID, tfSession, f.code(0), false, TwoFactorRequestMeta{})
		require.ErrorIs(t, err, domain.ErrTwoFactorSetupExpired)
	})
}

// --- Disable and recovery codes -------------------------------------------------------

func TestTwoFactorService_Disable(t *testing.T) {
	t.Run("refused while the instance requires it", func(t *testing.T) {
		f := newTwoFactorFixture(t, func(c *TwoFactorServiceConfig) { c.RequireTwoFactor = true })
		err := f.svc.Disable(context.Background(), tfUserID, TwoFactorProof{Code: "123456"}, TwoFactorRequestMeta{})
		require.ErrorIs(t, err, domain.ErrTwoFactorRequiredByInstance)
	})

	// Otherwise a session verified before would still meet a requirement switched on later.
	t.Run("with a code, removes everything and the second factor of every session", func(t *testing.T) {
		f := newTwoFactorFixture(t)
		f.lock(tfUserID, f.enabled(t, tfUserID))
		f.repo.EXPECT().RecordSuccess(gomock.Any(), gomock.Any(), tfUserID, gomock.Any(), f.now).Return(nil)
		f.repo.EXPECT().DeleteTOTP(gomock.Any(), gomock.Any(), tfUserID).Return(nil)
		f.repo.EXPECT().DeleteRecoveryCodes(gomock.Any(), gomock.Any(), tfUserID).Return(nil)
		f.authTx.EXPECT().ClearSecondFactorOnSessions(gomock.Any(), gomock.Any(), tfUserID).Return(nil)

		err := f.svc.Disable(context.Background(), tfUserID, TwoFactorProof{Code: f.code(0)}, TwoFactorRequestMeta{})
		require.NoError(t, err)
		assert.Equal(t, []mailer.SecurityNoticeKind{mailer.SecurityNoticeTwoFactorDisabled}, f.mail.kinds())
	})

	t.Run("a wrong code removes nothing", func(t *testing.T) {
		f := newTwoFactorFixture(t)
		f.lock(tfUserID, f.enabled(t, tfUserID))
		f.repo.EXPECT().RecordFailure(gomock.Any(), gomock.Any(), tfUserID, 1, nil, f.now).Return(nil)

		err := f.svc.Disable(context.Background(), tfUserID, TwoFactorProof{Code: "000000"}, TwoFactorRequestMeta{})
		require.ErrorIs(t, err, domain.ErrTwoFactorInvalidCode)
		assert.Empty(t, f.mail.sent)
	})

	// Outside sign-in, a recovery code waits like an authenticator code.
	t.Run("during a wait, a recovery code is refused too", func(t *testing.T) {
		f := newTwoFactorFixture(t)
		row := f.enabled(t, tfUserID)
		until := f.now.Add(time.Minute)
		row.LockedUntil = &until
		f.lock(tfUserID, row)

		err := f.svc.Disable(context.Background(), tfUserID, TwoFactorProof{RecoveryCode: "abcdef-ghjkmn-pqrstv-wxyz01"}, TwoFactorRequestMeta{})
		var locked *domain.TwoFactorLockedError
		require.ErrorAs(t, err, &locked)
	})
}

func TestTwoFactorService_RegenerateRecoveryCodes(t *testing.T) {
	t.Run("proved by a recovery code, only the notice of new codes is sent", func(t *testing.T) {
		f := newTwoFactorFixture(t)
		f.lock(tfUserID, f.enabled(t, tfUserID))
		f.repo.EXPECT().ConsumeRecoveryCode(gomock.Any(), gomock.Any(), tfUserID, gomock.Any(), f.now).Return(true, nil)
		f.repo.EXPECT().RecordSuccess(gomock.Any(), gomock.Any(), tfUserID, nil, f.now).Return(nil)
		f.repo.EXPECT().CountRemainingRecoveryCodes(gomock.Any(), gomock.Any(), tfUserID).Return(4, nil)
		f.repo.EXPECT().ReplaceRecoveryCodes(gomock.Any(), gomock.Any(), tfUserID, gomock.Any(), f.now).Return(nil)

		codes, err := f.svc.RegenerateRecoveryCodes(context.Background(), tfUserID, TwoFactorProof{RecoveryCode: "abcdef-ghjkmn-pqrstv-wxyz01"}, TwoFactorRequestMeta{})
		require.NoError(t, err)
		assert.Len(t, codes, domain.RecoveryCodeCount)
		assert.Equal(t, []mailer.SecurityNoticeKind{mailer.SecurityNoticeRecoveryCodesNew}, f.mail.kinds())
	})

	t.Run("a malformed recovery code is refused without a lookup", func(t *testing.T) {
		f := newTwoFactorFixture(t)
		f.lock(tfUserID, f.enabled(t, tfUserID))
		f.repo.EXPECT().RecordFailure(gomock.Any(), gomock.Any(), tfUserID, 1, nil, f.now).Return(nil)

		_, err := f.svc.RegenerateRecoveryCodes(context.Background(), tfUserID, TwoFactorProof{RecoveryCode: "nope"}, TwoFactorRequestMeta{})
		require.ErrorIs(t, err, domain.ErrTwoFactorInvalidCode)
	})
}

// --- Reset by the root user -----------------------------------------------------------

func TestTwoFactorService_ResetForUser(t *testing.T) {
	expectResetLocks := func(f *twoFactorFixture, rootRow, targetRow *domain.UserTOTP) {
		// By ID order: the root's ID sorts first here.
		first := f.authTx.EXPECT().LockUser(gomock.Any(), gomock.Any(), tfRootID).Return(nil)
		f.authTx.EXPECT().LockUser(gomock.Any(), gomock.Any(), tfUserID).Return(nil).After(first)
		f.repo.EXPECT().GetTOTPForUpdate(gomock.Any(), gomock.Any(), tfRootID).Return(rootRow, nil)
		f.repo.EXPECT().GetTOTPForUpdate(gomock.Any(), gomock.Any(), tfUserID).Return(targetRow, nil)
	}
	expectRemoved := func(f *twoFactorFixture) {
		f.repo.EXPECT().DeleteTOTP(gomock.Any(), gomock.Any(), tfUserID).Return(nil)
		f.repo.EXPECT().DeleteRecoveryCodes(gomock.Any(), gomock.Any(), tfUserID).Return(nil)
		f.authTx.EXPECT().DeleteAllSessions(gomock.Any(), gomock.Any(), tfUserID).Return(nil)
	}

	t.Run("only the root user resets", func(t *testing.T) {
		f := newTwoFactorFixture(t)
		err := f.svc.ResetForUser(context.Background(), tfUserID, tfSession, TwoFactorProof{}, tfRootID, "", TwoFactorRequestMeta{})
		var unauthorized *domain.ErrUnauthorized
		require.ErrorAs(t, err, &unauthorized)
	})

	t.Run("never their own", func(t *testing.T) {
		f := newTwoFactorFixture(t)
		err := f.svc.ResetForUser(context.Background(), tfRootID, tfSession, TwoFactorProof{}, tfRootID, "", TwoFactorRequestMeta{})
		require.ErrorIs(t, err, ErrCannotResetOwnTwoFactor)
	})

	t.Run("an enrolled root proves it with a code, whatever opened the session", func(t *testing.T) {
		f := newTwoFactorFixture(t)
		expectResetLocks(f, f.enabled(t, tfRootID), f.enabled(t, tfUserID))
		f.repo.EXPECT().RecordFailure(gomock.Any(), gomock.Any(), tfRootID, 1, nil, f.now).Return(nil)

		err := f.svc.ResetForUser(context.Background(), tfRootID, tfSession, TwoFactorProof{Code: "000000"}, tfUserID, "", TwoFactorRequestMeta{})
		require.ErrorIs(t, err, domain.ErrTwoFactorInvalidCode)
		assert.Empty(t, f.mail.sent)
	})

	t.Run("an enrolled root with a right code resets and the person is told", func(t *testing.T) {
		f := newTwoFactorFixture(t)
		f.users.EXPECT().GetUserByEmailInsensitive(gomock.Any(), "Ann@Example.com").Return(f.user, nil)
		expectResetLocks(f, f.enabled(t, tfRootID), f.enabled(t, tfUserID))
		f.repo.EXPECT().RecordSuccess(gomock.Any(), gomock.Any(), tfRootID, gomock.Any(), f.now).Return(nil)
		expectRemoved(f)
		ctx, record := auditCtx()
		err := f.svc.ResetForUser(ctx, tfRootID, tfSession, TwoFactorProof{Code: f.code(0)}, "", "Ann@Example.com", TwoFactorRequestMeta{})
		require.NoError(t, err)
		require.Len(t, f.mail.sent, 1)
		assert.Equal(t, "ann@example.com", f.mail.sent[0].email)
		assert.Equal(t, mailer.SecurityNoticeTwoFactorReset, f.mail.sent[0].notice.Kind)
		event, _ := record.Finalize(200)
		assert.Equal(t, domain.AuditTargetUser, event.TargetType)
		assert.Equal(t, tfUserID, event.TargetID)
	})

	t.Run("a root without two-factor authentication needs a fresh session", func(t *testing.T) {
		f := newTwoFactorFixture(t)
		expectResetLocks(f, nil, f.enabled(t, tfUserID))
		old := f.now.Add(-time.Hour)
		f.authTx.EXPECT().GetSessionForUpdate(gomock.Any(), gomock.Any(), tfRootID, tfSession).
			Return(&domain.Session{ID: tfSession, AuthMethod: domain.AuthMethodMagicCode, AuthenticatedAt: &old}, nil)

		err := f.svc.ResetForUser(context.Background(), tfRootID, tfSession, TwoFactorProof{}, tfUserID, "", TwoFactorRequestMeta{})
		require.ErrorIs(t, err, domain.ErrReauthenticationRequired)
		assert.Equal(t, 1, f.tx.rollbacks)
	})
}

// --- Rotation of SECRET_KEY -----------------------------------------------------------

// A secret sealed under a previous key still opens, and is sealed again under the first key
// when the setup is confirmed.
func TestTwoFactorService_EnableReencryptsUnderTheFirstKey(t *testing.T) {
	f := newTwoFactorFixture(t, func(c *TwoFactorServiceConfig) { c.PreviousKeys = []string{"old-key"} })
	expires := f.now.Add(5 * time.Minute)
	row := &domain.UserTOTP{UserID: tfUserID, PendingSecretEncrypted: f.seal(t, tfUserID, f.secret, "old-key"), PendingExpiresAt: &expires}
	f.lock(tfUserID, row)
	f.repo.EXPECT().Enable(gomock.Any(), gomock.Any(), tfUserID, gomock.Any(), gomock.Any(), f.now).
		DoAndReturn(func(_ context.Context, _ domain.DBExecutor, _, sealed string, _ int64, _ time.Time) error {
			aad, _ := domain.TOTPAAD(tfUserID)
			_, err := crypto.DecryptFromHexStringAAD(sealed, tfKey, aad)
			assert.NoError(t, err)
			return nil
		})
	f.repo.EXPECT().ReplaceRecoveryCodes(gomock.Any(), gomock.Any(), tfUserID, gomock.Any(), f.now).Return(nil)
	f.authTx.EXPECT().SetSessionSecondFactor(gomock.Any(), gomock.Any(), tfUserID, tfSession, domain.SecondFactorTOTP, f.now).Return(nil)

	_, err := f.svc.Enable(context.Background(), tfUserID, tfSession, f.code(0), false, TwoFactorRequestMeta{})
	require.NoError(t, err)
}
