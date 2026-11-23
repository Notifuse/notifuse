package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
)

// Accepting an invitation opens the invitee's session in one transaction under their lock.
// An invitee with two-factor authentication gets a challenge instead of a token, and the
// invitation waits for the second factor.

type acceptInvitationFixture struct {
	service  *WorkspaceService
	repo     *mocks.MockWorkspaceRepository
	userRepo *mocks.MockUserRepository
	auth     *mocks.MockAuthService
	tx       *fakeTx
	authTx   *mocks.MockAuthTxRepository
	totp     *mocks.MockTwoFactorRepository
	inv      *mocks.MockInvitationTxRepository
	now      time.Time
}

const (
	acceptInvitationID = "invitation-123"
	acceptWorkspaceID  = "workspace-123"
	acceptEmail        = "invitee@example.com"
)

func newAcceptInvitationFixture(t *testing.T, maxUsers int) *acceptInvitationFixture {
	ctrl := gomock.NewController(t)
	f := &acceptInvitationFixture{
		repo:     mocks.NewMockWorkspaceRepository(ctrl),
		userRepo: mocks.NewMockUserRepository(ctrl),
		auth:     mocks.NewMockAuthService(ctrl),
		tx:       &fakeTx{},
		authTx:   mocks.NewMockAuthTxRepository(ctrl),
		totp:     mocks.NewMockTwoFactorRepository(ctrl),
		inv:      mocks.NewMockInvitationTxRepository(ctrl),
		now:      time.Now().UTC(),
	}
	log := pkgmocks.NewMockLogger(ctrl)
	log.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(log).AnyTimes()
	log.EXPECT().Error(gomock.Any()).AnyTimes()
	log.EXPECT().Info(gomock.Any()).AnyTimes()
	log.EXPECT().Warn(gomock.Any()).AnyTimes()

	f.service = NewWorkspaceService(
		f.repo, f.userRepo, mocks.NewMockTaskRepository(ctrl),
		log, mocks.NewMockUserServiceInterface(ctrl), f.auth, pkgmocks.NewMockMailer(ctrl),
		&config.Config{RootEmail: "root@example.com", MaxUsers: maxUsers},
		mocks.NewMockContactService(ctrl), mocks.NewMockListService(ctrl),
		mocks.NewMockContactListService(ctrl), mocks.NewMockTemplateService(ctrl),
		mocks.NewMockWebhookRegistrationService(ctrl), "secret_key",
		&SupabaseService{}, &DNSVerificationService{}, &BlogService{},
		fullyLicensedProvider(ctrl),
		WorkspaceInvitationAuth{Tx: f.tx, AuthTx: f.authTx, TwoFactor: f.totp, InvitationTx: f.inv},
	)
	return f
}

func (f *acceptInvitationFixture) invitation() *domain.WorkspaceInvitation {
	return &domain.WorkspaceInvitation{
		ID:          acceptInvitationID,
		WorkspaceID: acceptWorkspaceID,
		Email:       acceptEmail,
		ExpiresAt:   time.Now().Add(24 * time.Hour),
		Permissions: domain.UserPermissions{domain.PermissionResourceContacts: {Read: true, Write: true}},
	}
}

// expectUpToClock: the stored invitation, the invitee (nil: a new account), the lock, the
// authenticator row and the clock.
func (f *acceptInvitationFixture) expectUpToClock(existing *domain.User, totpRow *domain.UserTOTP) {
	f.repo.EXPECT().GetInvitationByID(gomock.Any(), acceptInvitationID).Return(f.invitation(), nil)
	userID := "existing-user"
	if existing == nil {
		f.userRepo.EXPECT().GetUserByEmailInsensitive(gomock.Any(), acceptEmail).Return(nil, &domain.ErrUserNotFound{Message: "not found"})
		f.authTx.EXPECT().CreateUserIfAbsent(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, _ domain.DBExecutor, u *domain.User) (bool, error) {
				userID = u.ID
				return true, nil
			})
		f.authTx.EXPECT().LockUser(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, _ domain.DBExecutor, id string) error {
				if id != userID {
					return errors.New("locked another user")
				}
				return nil
			})
	} else {
		f.userRepo.EXPECT().GetUserByEmailInsensitive(gomock.Any(), acceptEmail).Return(existing, nil)
		f.authTx.EXPECT().LockUser(gomock.Any(), gomock.Any(), existing.ID).Return(nil)
	}
	f.totp.EXPECT().GetTOTPForUpdate(gomock.Any(), gomock.Any(), gomock.Any()).Return(totpRow, nil)
	f.totp.EXPECT().ClockTimestamp(gomock.Any(), gomock.Any()).Return(f.now, nil)
}

func enabledTOTP() *domain.UserTOTP {
	at := time.Now().Add(-time.Hour)
	secret := "sealed"
	return &domain.UserTOTP{UserID: "existing-user", EnabledAt: &at, SecretEncrypted: &secret}
}

func TestWorkspaceService_AcceptInvitation(t *testing.T) {
	existing := &domain.User{ID: "existing-user", Email: acceptEmail, Type: domain.UserTypeUser}

	t.Run("a new account joins as a member with a session opened by the invitation", func(t *testing.T) {
		f := newAcceptInvitationFixture(t, 0)
		f.expectUpToClock(nil, nil)
		f.inv.EXPECT().LockInvitation(gomock.Any(), gomock.Any(), acceptInvitationID).Return(f.invitation(), false, nil)
		f.inv.EXPECT().IsUserWorkspaceMember(gomock.Any(), gomock.Any(), gomock.Any(), acceptWorkspaceID).Return(false, nil)
		f.authTx.EXPECT().CreateSession(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, _ domain.DBExecutor, s *domain.Session) error {
				assert.Equal(t, domain.AuthMethodInvitation, s.AuthMethod)
				require.NotNil(t, s.AuthenticatedAt)
				assert.Equal(t, f.now, *s.AuthenticatedAt)
				assert.Nil(t, s.MagicCode)
				return nil
			})
		f.inv.EXPECT().AddUserToWorkspaceIfAbsent(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, _ domain.DBExecutor, uw *domain.UserWorkspace) (bool, error) {
				assert.Equal(t, acceptWorkspaceID, uw.WorkspaceID)
				assert.Equal(t, "member", uw.Role)
				assert.True(t, uw.Permissions[domain.PermissionResourceContacts].Write)
				return true, nil
			})
		f.inv.EXPECT().DeleteInvitationsByEmail(gomock.Any(), gomock.Any(), acceptWorkspaceID, acceptEmail).Return(nil)
		f.auth.EXPECT().GenerateUserAuthToken(gomock.Any(), gomock.Any(), gomock.Any()).Return("auth-token")

		resp, err := f.service.AcceptInvitation(context.Background(), acceptInvitationID, acceptWorkspaceID, acceptEmail)
		require.NoError(t, err)
		assert.Equal(t, "auth-token", resp.Token)
		assert.Equal(t, acceptEmail, resp.User.Email)
		assert.Nil(t, resp.Challenge)
		assert.Equal(t, 1, f.tx.commits)
	})

	t.Run("an invitee with two-factor authentication gets a challenge and joins nothing yet", func(t *testing.T) {
		f := newAcceptInvitationFixture(t, 0)
		f.expectUpToClock(existing, enabledTOTP())
		var sessionID string
		f.authTx.EXPECT().CreateSession(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, _ domain.DBExecutor, s *domain.Session) error {
				assert.Empty(t, s.AuthMethod, "set with the challenge")
				assert.Nil(t, s.AuthenticatedAt, "not usable before the second factor")
				sessionID = s.ID
				return nil
			})
		var storedHash string
		f.authTx.EXPECT().SetSessionChallenge(gomock.Any(), gomock.Any(), gomock.Any(), domain.AuthMethodInvitation, gomock.Any(), f.now.Add(domain.ChallengeTTL), gomock.Any()).
			DoAndReturn(func(_ context.Context, _ domain.DBExecutor, id string, _ domain.AuthMethod, hash string, _ time.Time, invitationID *string) error {
				assert.Equal(t, sessionID, id)
				require.NotNil(t, invitationID)
				assert.Equal(t, acceptInvitationID, *invitationID)
				storedHash = hash
				return nil
			})
		// No membership, no deleted invitation, no token: strict mocks fail on any of them.

		record := domain.NewAuditRecord("203.0.113.1", "test", "req-1")
		resp, err := f.service.AcceptInvitation(domain.WithAuditRecord(context.Background(), record), acceptInvitationID, acceptWorkspaceID, acceptEmail)
		require.NoError(t, err)
		assert.Empty(t, resp.Token)
		require.NotNil(t, resp.Challenge)
		assert.Equal(t, HashChallenge(resp.Challenge.Token), storedHash)
		event, _ := record.Finalize(200)
		assert.Equal(t, "second_factor_required", event.Metadata["stage"])
	})

	// Typically the same link accepted twice at once. The deleted invitation is kept; no
	// session is opened.
	t.Run("a member already in the workspace is refused and the invitation goes", func(t *testing.T) {
		f := newAcceptInvitationFixture(t, 0)
		f.expectUpToClock(existing, nil)
		f.inv.EXPECT().LockInvitation(gomock.Any(), gomock.Any(), acceptInvitationID).Return(f.invitation(), false, nil)
		f.inv.EXPECT().IsUserWorkspaceMember(gomock.Any(), gomock.Any(), existing.ID, acceptWorkspaceID).Return(true, nil)
		f.inv.EXPECT().DeleteInvitationsByEmail(gomock.Any(), gomock.Any(), acceptWorkspaceID, acceptEmail).Return(nil)

		record := domain.NewAuditRecord("203.0.113.1", "test", "req-1")
		resp, err := f.service.AcceptInvitation(domain.WithAuditRecord(context.Background(), record), acceptInvitationID, acceptWorkspaceID, acceptEmail)
		require.ErrorIs(t, err, domain.ErrInvitationUnavailable)
		assert.Nil(t, resp)
		assert.Equal(t, 1, f.tx.commits, "the deleted invitation is committed")
		event, _ := record.Finalize(404)
		assert.Equal(t, "already_member", event.Metadata["reason"])
	})

	t.Run("an invitation gone or expired under the lock opens nothing", func(t *testing.T) {
		for name, lockResult := range map[string]struct {
			expired bool
			err     error
		}{
			"gone":    {err: domain.ErrInvitationNotFound},
			"expired": {expired: true},
		} {
			t.Run(name, func(t *testing.T) {
				f := newAcceptInvitationFixture(t, 0)
				f.expectUpToClock(nil, nil)
				var inv *domain.WorkspaceInvitation
				if lockResult.err == nil {
					inv = f.invitation()
				}
				f.inv.EXPECT().LockInvitation(gomock.Any(), gomock.Any(), acceptInvitationID).Return(inv, lockResult.expired, lockResult.err)

				_, err := f.service.AcceptInvitation(context.Background(), acceptInvitationID, acceptWorkspaceID, acceptEmail)
				require.ErrorIs(t, err, domain.ErrInvitationUnavailable)
				assert.Equal(t, 1, f.tx.rollbacks, "the account created for it is rolled back too")
			})
		}
	})

	t.Run("a write failure rolls everything back", func(t *testing.T) {
		f := newAcceptInvitationFixture(t, 0)
		f.expectUpToClock(nil, nil)
		f.inv.EXPECT().LockInvitation(gomock.Any(), gomock.Any(), acceptInvitationID).Return(f.invitation(), false, nil)
		f.inv.EXPECT().IsUserWorkspaceMember(gomock.Any(), gomock.Any(), gomock.Any(), acceptWorkspaceID).Return(false, nil)
		f.authTx.EXPECT().CreateSession(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
		sentinel := errors.New("write failed")
		f.inv.EXPECT().AddUserToWorkspaceIfAbsent(gomock.Any(), gomock.Any(), gomock.Any()).Return(false, sentinel)

		_, err := f.service.AcceptInvitation(context.Background(), acceptInvitationID, acceptWorkspaceID, acceptEmail)
		require.ErrorIs(t, err, sentinel)
		assert.Equal(t, 1, f.tx.rollbacks)
	})
}

func TestWorkspaceService_AcceptInvitation_TeamMemberLimit(t *testing.T) {
	t.Run("limit reached on accept", func(t *testing.T) {
		// MaxUsers=3, and the other members and invitations already hold 3 seats; the count
		// leaves out the invitation being accepted, so accepting would exceed the limit.
		f := newAcceptInvitationFixture(t, 3)
		f.expectUpToClock(nil, nil)
		f.inv.EXPECT().LockInvitation(gomock.Any(), gomock.Any(), acceptInvitationID).Return(f.invitation(), false, nil)
		f.inv.EXPECT().IsUserWorkspaceMember(gomock.Any(), gomock.Any(), gomock.Any(), acceptWorkspaceID).Return(false, nil)
		f.inv.EXPECT().CountWorkspaceMembersAndInvitations(gomock.Any(), gomock.Any(), acceptWorkspaceID, acceptEmail).Return(3, nil)

		_, err := f.service.AcceptInvitation(context.Background(), acceptInvitationID, acceptWorkspaceID, acceptEmail)
		var limitErr *domain.ErrTeamMemberLimitReached
		require.True(t, errors.As(err, &limitErr))
		assert.Equal(t, 3, limitErr.Limit)
		assert.Equal(t, 3, limitErr.Current)
		assert.Equal(t, 1, f.tx.rollbacks)
	})

	t.Run("accepts under the limit: its own invitation is left out of the count", func(t *testing.T) {
		f := newAcceptInvitationFixture(t, 3)
		f.expectUpToClock(nil, nil)
		f.inv.EXPECT().LockInvitation(gomock.Any(), gomock.Any(), acceptInvitationID).Return(f.invitation(), false, nil)
		f.inv.EXPECT().IsUserWorkspaceMember(gomock.Any(), gomock.Any(), gomock.Any(), acceptWorkspaceID).Return(false, nil)
		f.inv.EXPECT().CountWorkspaceMembersAndInvitations(gomock.Any(), gomock.Any(), acceptWorkspaceID, acceptEmail).Return(2, nil)
		f.authTx.EXPECT().CreateSession(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
		f.inv.EXPECT().AddUserToWorkspaceIfAbsent(gomock.Any(), gomock.Any(), gomock.Any()).Return(true, nil)
		f.inv.EXPECT().DeleteInvitationsByEmail(gomock.Any(), gomock.Any(), acceptWorkspaceID, acceptEmail).Return(nil)
		f.auth.EXPECT().GenerateUserAuthToken(gomock.Any(), gomock.Any(), gomock.Any()).Return("auth-token")

		resp, err := f.service.AcceptInvitation(context.Background(), acceptInvitationID, acceptWorkspaceID, acceptEmail)
		require.NoError(t, err)
		assert.Equal(t, "auth-token", resp.Token)
	})
}
