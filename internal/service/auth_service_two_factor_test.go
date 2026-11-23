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
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
)

// When the instance requires two-factor authentication, a console session that did not
// pass it reaches only the routes that let it turn it on.

func newGatedAuthService(t *testing.T, require2FA bool) (*AuthService, *mocks.MockAuthRepository, *mocks.MockWorkspaceRepository) {
	ctrl := gomock.NewController(t)
	repo := mocks.NewMockAuthRepository(ctrl)
	workspaces := mocks.NewMockWorkspaceRepository(ctrl)
	log := pkgmocks.NewMockLogger(ctrl)
	log.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(log).AnyTimes()
	log.EXPECT().Error(gomock.Any()).AnyTimes()
	log.EXPECT().Warn(gomock.Any()).AnyTimes()
	svc := NewAuthService(AuthServiceConfig{
		Repository:          repo,
		WorkspaceRepository: workspaces,
		GetSecret:           func() ([]byte, error) { return []byte("test-jwt-secret-key-1234567890123456"), nil },
		Logger:              log,
		RequireTwoFactor:    require2FA,
	})
	return svc, repo, workspaces
}

func gatedSessionCtx(userType domain.UserType) context.Context {
	ctx := context.WithValue(context.Background(), domain.UserIDKey, "user-1")
	ctx = context.WithValue(ctx, domain.SessionIDKey, "session-1")
	return context.WithValue(ctx, domain.UserTypeKey, string(userType))
}

func expectSession(repo *mocks.MockAuthRepository, session domain.SessionAuth) {
	session.ExpiresAt = time.Now().Add(time.Hour)
	repo.EXPECT().GetSessionByID(gomock.Any(), "session-1", "user-1").Return(&session, nil).AnyTimes()
	repo.EXPECT().GetUserByID(gomock.Any(), "user-1").Return(&domain.User{ID: "user-1", Email: "ann@example.com", Type: domain.UserTypeUser}, nil).AnyTimes()
}

func TestAuthService_InstanceTwoFactorRequirement(t *testing.T) {
	cases := []struct {
		name    string
		session domain.SessionAuth
		allowed bool
	}{
		{"magic code without a second factor", domain.SessionAuth{AuthMethod: domain.AuthMethodMagicCode}, false},
		{"a session opened before 45.0", domain.SessionAuth{}, false},
		{"magic code with an authenticator code", domain.SessionAuth{AuthMethod: domain.AuthMethodMagicCode, SecondFactor: domain.SecondFactorTOTP}, true},
		{"magic code with a recovery code", domain.SessionAuth{AuthMethod: domain.AuthMethodMagicCode, SecondFactor: domain.SecondFactorRecoveryCode}, true},
		{"single sign-on", domain.SessionAuth{AuthMethod: domain.AuthMethodSSO}, true},
		{"root signature", domain.SessionAuth{AuthMethod: domain.AuthMethodRootSignature}, true},
		{"demo seed", domain.SessionAuth{AuthMethod: domain.AuthMethodDemo}, true},
		{"invitation without a second factor", domain.SessionAuth{AuthMethod: domain.AuthMethodInvitation}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, repo, _ := newGatedAuthService(t, true)
			expectSession(repo, tc.session)

			_, err := svc.AuthenticateUserFromContext(gatedSessionCtx(domain.UserTypeUser))
			if tc.allowed {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, domain.ErrTwoFactorRequired)
				assert.False(t, errors.Is(err, domain.ErrSessionAuthFailed), "never a 401: the console would drop its token")
			}

			// The routes that let the session turn it on still see it.
			_, auth, err := svc.AuthenticateSession(gatedSessionCtx(domain.UserTypeUser))
			require.NoError(t, err)
			assert.Equal(t, tc.session.AuthMethod, auth.AuthMethod)
		})
	}

	t.Run("not required: every valid session passes", func(t *testing.T) {
		svc, repo, _ := newGatedAuthService(t, false)
		expectSession(repo, domain.SessionAuth{AuthMethod: domain.AuthMethodMagicCode})
		_, err := svc.AuthenticateUserFromContext(gatedSessionCtx(domain.UserTypeUser))
		require.NoError(t, err)
		require.NoError(t, svc.RequireCompliantSession(gatedSessionCtx(domain.UserTypeUser)))
	})

	t.Run("API keys are not concerned", func(t *testing.T) {
		svc, repo, _ := newGatedAuthService(t, true)
		repo.EXPECT().GetUserByID(gomock.Any(), "user-1").Return(&domain.User{ID: "user-1", Type: domain.UserTypeAPIKey}, nil)
		_, err := svc.AuthenticateUserFromContext(gatedSessionCtx(domain.UserTypeAPIKey))
		require.NoError(t, err)
		require.NoError(t, svc.RequireCompliantSession(gatedSessionCtx(domain.UserTypeAPIKey)))
	})

	// A refused session must not leave a membership cached in the context for the next call.
	t.Run("a refused workspace call caches nothing", func(t *testing.T) {
		svc, repo, _ := newGatedAuthService(t, true)
		expectSession(repo, domain.SessionAuth{AuthMethod: domain.AuthMethodMagicCode})
		ctx, _, _, err := svc.AuthenticateUserForWorkspace(gatedSessionCtx(domain.UserTypeUser), "ws1")
		require.ErrorIs(t, err, domain.ErrTwoFactorRequired)
		assert.Nil(t, ctx.Value(domain.WorkspaceUserKey("ws1")))
		assert.Nil(t, ctx.Value(domain.WorkspaceMembershipKey("ws1")))
	})

	t.Run("RequireCompliantSession refuses a session without a second factor", func(t *testing.T) {
		svc, repo, _ := newGatedAuthService(t, true)
		expectSession(repo, domain.SessionAuth{AuthMethod: domain.AuthMethodMagicCode})
		require.ErrorIs(t, svc.RequireCompliantSession(gatedSessionCtx(domain.UserTypeUser)), domain.ErrTwoFactorRequired)
	})
}

// Even where the instance does not require it, a root user who turned two-factor
// authentication on holds root routes only from a session that passed it.
func TestAuthService_RequireCompliantRootSession(t *testing.T) {
	t.Run("an older session of an enrolled root is refused", func(t *testing.T) {
		svc, repo, _ := newGatedAuthService(t, false)
		expectSession(repo, domain.SessionAuth{AuthMethod: domain.AuthMethodMagicCode})
		repo.EXPECT().IsTwoFactorEnabled(gomock.Any(), "user-1").Return(true, nil)
		require.ErrorIs(t, svc.RequireCompliantRootSession(gatedSessionCtx(domain.UserTypeUser)), domain.ErrTwoFactorRequired)
	})

	t.Run("a root without two-factor authentication passes", func(t *testing.T) {
		svc, repo, _ := newGatedAuthService(t, false)
		expectSession(repo, domain.SessionAuth{AuthMethod: domain.AuthMethodMagicCode})
		repo.EXPECT().IsTwoFactorEnabled(gomock.Any(), "user-1").Return(false, nil)
		require.NoError(t, svc.RequireCompliantRootSession(gatedSessionCtx(domain.UserTypeUser)))
	})

	t.Run("a session that passed it needs no lookup", func(t *testing.T) {
		svc, repo, _ := newGatedAuthService(t, false)
		expectSession(repo, domain.SessionAuth{AuthMethod: domain.AuthMethodMagicCode, SecondFactor: domain.SecondFactorTOTP})
		require.NoError(t, svc.RequireCompliantRootSession(gatedSessionCtx(domain.UserTypeUser)))
	})

	t.Run("a lookup failure is not a refusal", func(t *testing.T) {
		svc, repo, _ := newGatedAuthService(t, false)
		expectSession(repo, domain.SessionAuth{AuthMethod: domain.AuthMethodMagicCode})
		sentinel := errors.New("db down")
		repo.EXPECT().IsTwoFactorEnabled(gomock.Any(), "user-1").Return(false, sentinel)
		require.ErrorIs(t, svc.RequireCompliantRootSession(gatedSessionCtx(domain.UserTypeUser)), sentinel)
	})
}

func TestCheckRequireTwoFactorChange(t *testing.T) {
	on, off := true, false
	magic := domain.SessionAuth{AuthMethod: domain.AuthMethodMagicCode}
	verified := domain.SessionAuth{AuthMethod: domain.AuthMethodMagicCode, SecondFactor: domain.SecondFactorTOTP}

	assert.NoError(t, CheckRequireTwoFactorChange(false, nil, magic, false), "left as it is")
	assert.NoError(t, CheckRequireTwoFactorChange(true, &off, magic, false), "turning it off")
	assert.NoError(t, CheckRequireTwoFactorChange(true, &on, magic, false), "already on")
	assert.ErrorIs(t, CheckRequireTwoFactorChange(false, &on, magic, false), ErrTwoFactorRequiredForRoot, "a root without it would lock themselves out")
	assert.ErrorIs(t, CheckRequireTwoFactorChange(false, &on, magic, true), ErrTwoFactorRequiredForRoot, "enrolled, but this session did not pass it")
	assert.NoError(t, CheckRequireTwoFactorChange(false, &on, verified, true))
	assert.NoError(t, CheckRequireTwoFactorChange(false, &on, domain.SessionAuth{AuthMethod: domain.AuthMethodSSO}, false))
	assert.NoError(t, CheckRequireTwoFactorChange(false, &on, domain.SessionAuth{AuthMethod: domain.AuthMethodRootSignature}, false))
}

// A root route refuses a root session that the root check refuses.
func TestWorkspaceService_CreateWorkspace_RefusesNonCompliantRootSession(t *testing.T) {
	f := newAcceptInvitationFixture(t, 0)
	f.auth.EXPECT().AuthenticateUserFromContext(gomock.Any()).Return(&domain.User{ID: "root", Email: "root@example.com", Type: domain.UserTypeUser}, nil)
	f.auth.EXPECT().RequireCompliantRootSession(gomock.Any()).Return(domain.ErrTwoFactorRequired)

	_, err := f.service.CreateWorkspace(context.Background(), "newws", "New", "", "", "", "UTC", domain.FileManagerSettings{}, "en", []string{"en"})
	require.ErrorIs(t, err, domain.ErrTwoFactorRequired)
}
