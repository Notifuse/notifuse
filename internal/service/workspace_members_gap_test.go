package service

import (
	"context"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
)

// gapMemberDeps exposes every collaborator a guard test needs to leave
// expectation-free, so gomock fails the test if a failed guard still reaches
// the repository or user service.
type gapMemberDeps struct {
	repo        *mocks.MockWorkspaceRepository
	userRepo    *mocks.MockUserRepository
	taskRepo    *mocks.MockTaskRepository
	userService *mocks.MockUserServiceInterface
	auth        *mocks.MockAuthService
}

func newGapMemberService(ctrl *gomock.Controller, cfg *config.Config, provider domain.EntitlementProvider) (*WorkspaceService, gapMemberDeps) {
	deps := gapMemberDeps{
		repo:        mocks.NewMockWorkspaceRepository(ctrl),
		userRepo:    mocks.NewMockUserRepository(ctrl),
		taskRepo:    mocks.NewMockTaskRepository(ctrl),
		userService: mocks.NewMockUserServiceInterface(ctrl),
		auth:        mocks.NewMockAuthService(ctrl),
	}
	if cfg == nil {
		cfg = &config.Config{RootEmail: "root@example.com"}
	}
	if provider == nil {
		provider = fullyLicensedProvider(ctrl)
	}
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()

	svc := NewWorkspaceService(
		deps.repo,
		deps.userRepo,
		deps.taskRepo,
		mockLogger,
		deps.userService,
		deps.auth,
		pkgmocks.NewMockMailer(ctrl),
		cfg,
		mocks.NewMockContactService(ctrl),
		mocks.NewMockListService(ctrl),
		mocks.NewMockContactListService(ctrl),
		mocks.NewMockTemplateService(ctrl),
		mocks.NewMockWebhookRegistrationService(ctrl),
		"secret_key",
		&SupabaseService{},
		&DNSVerificationService{},
		&BlogService{},
		provider,
	)
	return svc, deps
}

func (d gapMemberDeps) gapAuthAs(workspaceID, userID, role string) {
	d.auth.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
		DoAndReturn(func(ctx context.Context, _ string) (context.Context, *domain.User, *domain.UserWorkspace, error) {
			return ctx, &domain.User{ID: userID}, &domain.UserWorkspace{UserID: userID, WorkspaceID: workspaceID, Role: role}, nil
		}).Times(1)
}

func (d gapMemberDeps) gapAuthErr(workspaceID string, err error) {
	d.auth.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
		Return(context.Background(), nil, nil, err).Times(1)
}

func TestWorkspaceMembersGap_AddUserToWorkspaceGuards(t *testing.T) {
	ctx := context.Background()

	t.Run("auth failure is wrapped and repo is never called", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		svc, deps := newGapMemberService(ctrl, nil, nil)
		sentinel := errors.New("session revoked")
		deps.gapAuthErr("ws1", sentinel)

		err := svc.AddUserToWorkspace(ctx, "ws1", "user2", "member", domain.NewFullPermissions())
		require.Error(t, err)
		assert.ErrorIs(t, err, sentinel)
		assert.EqualError(t, err, "failed to authenticate user: session revoked")
		// No repo expectation was set: gomock fails if AddUserToWorkspace or the seat count runs.
	})

	t.Run("non-owner is refused before permission validation and repo", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		svc, deps := newGapMemberService(ctrl, nil, nil)
		deps.gapAuthAs("ws1", "requester1", "member")

		err := svc.AddUserToWorkspace(ctx, "ws1", "user2", "member", domain.NewFullPermissions())
		require.Error(t, err)
		var unauth *domain.ErrUnauthorized
		require.ErrorAs(t, err, &unauth)
		assert.Equal(t, "user is not an owner of the workspace", unauth.Message)
		assert.EqualError(t, err, "user is not an owner of the workspace")
	})

	t.Run("unknown permission resource is refused before seat check and repo write", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		svc, deps := newGapMemberService(ctrl, nil, nil)
		deps.gapAuthAs("ws1", "owner1", "owner")
		bad := domain.UserPermissions{domain.PermissionResource("bogus"): {Read: true}}

		err := svc.AddUserToWorkspace(ctx, "ws1", "user2", "member", bad)
		require.Error(t, err)
		assert.EqualError(t, err, "unknown permission resource: bogus")
	})

	t.Run("restricted grant without RBAC licence is refused before seat check and repo write", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		provider := stubEntitlements(ctrl, licensedEntitlements(5))
		svc, deps := newGapMemberService(ctrl, nil, provider)
		deps.gapAuthAs("ws1", "owner1", "owner")

		err := svc.AddUserToWorkspace(ctx, "ws1", "user2", "member", restrictedPermissions())
		require.Error(t, err)
		var notLicensed *domain.ErrFeatureNotLicensed
		require.ErrorAs(t, err, &notLicensed)
		assert.Equal(t, domain.FeatureRBAC, notLicensed.Feature)
		assert.EqualError(t, err, "Custom permissions require a Notifuse licence (Studio or above).")
	})

	t.Run("seat limit at boundary blocks the repo write", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		cfg := &config.Config{RootEmail: "root@example.com", MaxUsers: 2}
		svc, deps := newGapMemberService(ctrl, cfg, nil)
		deps.gapAuthAs("ws1", "owner1", "owner")
		deps.repo.EXPECT().CountWorkspaceMembersAndInvitations(gomock.Any(), "ws1", "").Return(2, nil).Times(1)
		// AddUserToWorkspace on repo deliberately has no expectation.

		err := svc.AddUserToWorkspace(ctx, "ws1", "user2", "member", domain.NewFullPermissions())
		require.Error(t, err)
		var limitErr *domain.ErrTeamMemberLimitReached
		require.ErrorAs(t, err, &limitErr)
		assert.Equal(t, 2, limitErr.Limit)
		assert.Equal(t, 2, limitErr.Current)
		assert.EqualError(t, err, "team member limit reached: workspace has 2 members and invitations (limit: 2)")
	})

	t.Run("invalid role is refused after seat check but before repo write", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		svc, deps := newGapMemberService(ctrl, nil, nil)
		deps.gapAuthAs("ws1", "owner1", "owner")

		err := svc.AddUserToWorkspace(ctx, "ws1", "user2", "superadmin", domain.NewFullPermissions())
		require.Error(t, err)
		assert.EqualError(t, err, "invalid user workspace: role must be either 'owner' or 'member'")
	})
}

func TestWorkspaceMembersGap_InviteMemberGuards(t *testing.T) {
	ctx := context.Background()

	t.Run("auth failure is wrapped and repo is never called", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		svc, deps := newGapMemberService(ctrl, nil, nil)
		sentinel := errors.New("token expired")
		deps.gapAuthErr("ws1", sentinel)

		inv, token, err := svc.InviteMember(ctx, "ws1", "jane@example.com", domain.NewFullPermissions())
		require.Error(t, err)
		assert.Nil(t, inv)
		assert.Empty(t, token)
		assert.ErrorIs(t, err, sentinel)
		assert.EqualError(t, err, "failed to authenticate user: token expired")
	})

	t.Run("non-owner is refused before email validation and repo", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		svc, deps := newGapMemberService(ctrl, nil, nil)
		deps.gapAuthAs("ws1", "member1", "member")

		inv, token, err := svc.InviteMember(ctx, "ws1", "not-an-email", domain.NewFullPermissions())
		require.Error(t, err)
		assert.Nil(t, inv)
		assert.Empty(t, token)
		var unauth *domain.ErrUnauthorized
		require.ErrorAs(t, err, &unauth)
		assert.EqualError(t, err, "user is not an owner of the workspace")
	})

	t.Run("invalid email is refused before permission validation and repo", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		svc, deps := newGapMemberService(ctrl, nil, nil)
		deps.gapAuthAs("ws1", "owner1", "owner")

		inv, token, err := svc.InviteMember(ctx, "ws1", "not-an-email", domain.NewFullPermissions())
		require.Error(t, err)
		assert.Nil(t, inv)
		assert.Empty(t, token)
		assert.EqualError(t, err, "invalid email format")
	})

	t.Run("unknown permission resource is refused before workspace lookup", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		svc, deps := newGapMemberService(ctrl, nil, nil)
		deps.gapAuthAs("ws1", "owner1", "owner")
		bad := domain.UserPermissions{domain.PermissionResource("nope"): {Write: true}}

		inv, token, err := svc.InviteMember(ctx, "ws1", "jane@example.com", bad)
		require.Error(t, err)
		assert.Nil(t, inv)
		assert.Empty(t, token)
		assert.EqualError(t, err, "unknown permission resource: nope")
	})

	t.Run("restricted grant without RBAC licence is refused before workspace lookup", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		provider := stubEntitlements(ctrl, licensedEntitlements(5))
		svc, deps := newGapMemberService(ctrl, nil, provider)
		deps.gapAuthAs("ws1", "owner1", "owner")

		inv, token, err := svc.InviteMember(ctx, "ws1", "jane@example.com", restrictedPermissions())
		require.Error(t, err)
		assert.Nil(t, inv)
		assert.Empty(t, token)
		var notLicensed *domain.ErrFeatureNotLicensed
		require.ErrorAs(t, err, &notLicensed)
		assert.Equal(t, domain.FeatureRBAC, notLicensed.Feature)
		assert.EqualError(t, err, "Custom permissions require a Notifuse licence (Studio or above).")
	})

	t.Run("platform admin email is refused before workspace lookup", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		svc, deps := newGapMemberService(ctrl, nil, nil)
		deps.gapAuthAs("ws1", "owner1", "owner")

		inv, token, err := svc.InviteMember(ctx, "ws1", "root@example.com", domain.NewFullPermissions())
		require.Error(t, err)
		assert.Nil(t, inv)
		assert.Empty(t, token)
		assert.EqualError(t, err, "cannot invite a platform admin email")
	})

	t.Run("missing workspace is refused before seat check and invitation write", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		svc, deps := newGapMemberService(ctrl, nil, nil)
		deps.gapAuthAs("ws1", "owner1", "owner")
		deps.repo.EXPECT().GetByID(gomock.Any(), "ws1").Return(nil, nil).Times(1)

		inv, token, err := svc.InviteMember(ctx, "ws1", "jane@example.com", domain.NewFullPermissions())
		require.Error(t, err)
		assert.Nil(t, inv)
		assert.Empty(t, token)
		assert.EqualError(t, err, "workspace not found")
	})

	t.Run("workspace lookup error propagates exactly and blocks seat check", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		svc, deps := newGapMemberService(ctrl, nil, nil)
		deps.gapAuthAs("ws1", "owner1", "owner")
		sentinel := errors.New("db down")
		deps.repo.EXPECT().GetByID(gomock.Any(), "ws1").Return(nil, sentinel).Times(1)

		inv, token, err := svc.InviteMember(ctx, "ws1", "jane@example.com", domain.NewFullPermissions())
		require.Error(t, err)
		assert.Nil(t, inv)
		assert.Empty(t, token)
		assert.ErrorIs(t, err, sentinel)
		assert.EqualError(t, err, "db down")
	})
}

func TestWorkspaceMembersGap_RemoveMemberGuards(t *testing.T) {
	ctx := context.Background()

	t.Run("non-owner is refused before user lookup and repo removal", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		svc, deps := newGapMemberService(ctrl, nil, nil)
		deps.gapAuthAs("ws1", "member1", "member")

		err := svc.RemoveMember(ctx, "ws1", "user2")
		require.Error(t, err)
		var unauth *domain.ErrUnauthorized
		require.ErrorAs(t, err, &unauth)
		assert.EqualError(t, err, "user is not an owner of the workspace")
		// userService.GetUserByID and repo.RemoveUserFromWorkspace have no expectations.
	})

	t.Run("owner cannot remove itself and repo is never called", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		svc, deps := newGapMemberService(ctrl, nil, nil)
		deps.gapAuthAs("ws1", "owner1", "owner")

		err := svc.RemoveMember(ctx, "ws1", "owner1")
		require.Error(t, err)
		assert.EqualError(t, err, "cannot remove yourself from the workspace")
	})

	t.Run("user lookup error propagates and repo removal never happens", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		svc, deps := newGapMemberService(ctrl, nil, nil)
		deps.gapAuthAs("ws1", "owner1", "owner")
		sentinel := errors.New("user store down")
		deps.userService.EXPECT().GetUserByID(gomock.Any(), "user2").Return(nil, sentinel).Times(1)

		err := svc.RemoveMember(ctx, "ws1", "user2")
		require.Error(t, err)
		assert.ErrorIs(t, err, sentinel)
		assert.EqualError(t, err, "user store down")
	})
}

func TestWorkspaceMembersGap_RemoveUserFromWorkspaceGuards(t *testing.T) {
	ctx := context.Background()

	t.Run("non-owner is refused before repo removal", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		svc, deps := newGapMemberService(ctrl, nil, nil)
		deps.gapAuthAs("ws1", "member1", "member")

		err := svc.RemoveUserFromWorkspace(ctx, "ws1", "user2")
		require.Error(t, err)
		var unauth *domain.ErrUnauthorized
		require.ErrorAs(t, err, &unauth)
		assert.EqualError(t, err, "user is not an owner of the workspace")
	})

	t.Run("owner cannot remove itself and repo is never called", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		svc, deps := newGapMemberService(ctrl, nil, nil)
		deps.gapAuthAs("ws1", "owner1", "owner")

		err := svc.RemoveUserFromWorkspace(ctx, "ws1", "owner1")
		require.Error(t, err)
		assert.EqualError(t, err, "cannot remove yourself from the workspace")
	})
}

func TestWorkspaceMembersGap_TransferOwnershipGuards(t *testing.T) {
	ctx := context.Background()

	t.Run("auth failure is wrapped and repo is never called", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		svc, deps := newGapMemberService(ctrl, nil, nil)
		sentinel := errors.New("bad token")
		deps.gapAuthErr("ws1", sentinel)

		err := svc.TransferOwnership(ctx, "ws1", "user2", "owner1")
		require.Error(t, err)
		assert.ErrorIs(t, err, sentinel)
		assert.EqualError(t, err, "failed to authenticate user: bad token")
	})

	t.Run("non-owner current owner blocks new-owner lookup and all writes", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		svc, deps := newGapMemberService(ctrl, nil, nil)
		deps.gapAuthAs("ws1", "owner1", "owner")
		deps.repo.EXPECT().GetUserWorkspace(gomock.Any(), "owner1", "ws1").
			Return(&domain.UserWorkspace{UserID: "owner1", WorkspaceID: "ws1", Role: "member"}, nil).Times(1)
		// Second GetUserWorkspace and AddUserToWorkspace deliberately have no expectations.

		err := svc.TransferOwnership(ctx, "ws1", "user2", "owner1")
		require.Error(t, err)
		var unauth *domain.ErrUnauthorized
		require.ErrorAs(t, err, &unauth)
		assert.EqualError(t, err, "user is not an owner of the workspace")
	})

	t.Run("new owner who is not a plain member blocks all writes", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		svc, deps := newGapMemberService(ctrl, nil, nil)
		deps.gapAuthAs("ws1", "owner1", "owner")
		gomock.InOrder(
			deps.repo.EXPECT().GetUserWorkspace(gomock.Any(), "owner1", "ws1").
				Return(&domain.UserWorkspace{UserID: "owner1", WorkspaceID: "ws1", Role: "owner"}, nil).Times(1),
			deps.repo.EXPECT().GetUserWorkspace(gomock.Any(), "user2", "ws1").
				Return(&domain.UserWorkspace{UserID: "user2", WorkspaceID: "ws1", Role: "owner"}, nil).Times(1),
		)

		err := svc.TransferOwnership(ctx, "ws1", "user2", "owner1")
		require.Error(t, err)
		assert.EqualError(t, err, "new owner must be a current member of the workspace")
	})

	t.Run("current owner lookup error propagates and blocks new-owner lookup", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		svc, deps := newGapMemberService(ctrl, nil, nil)
		deps.gapAuthAs("ws1", "owner1", "owner")
		sentinel := errors.New("membership store down")
		deps.repo.EXPECT().GetUserWorkspace(gomock.Any(), "owner1", "ws1").Return(nil, sentinel).Times(1)

		err := svc.TransferOwnership(ctx, "ws1", "user2", "owner1")
		require.Error(t, err)
		assert.ErrorIs(t, err, sentinel)
		assert.EqualError(t, err, "membership store down")
	})
}

func TestWorkspaceMembersGap_DeleteAndAuditGuards(t *testing.T) {
	ctx := context.Background()

	t.Run("DeleteWorkspace non-owner blocks workspace fetch delete and task purge", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		svc, deps := newGapMemberService(ctrl, nil, nil)
		deps.gapAuthAs("ws1", "member1", "member")

		err := svc.DeleteWorkspace(ctx, "ws1")
		require.Error(t, err)
		var unauth *domain.ErrUnauthorized
		require.ErrorAs(t, err, &unauth)
		assert.EqualError(t, err, "user is not an owner of the workspace")
		// repo.GetByID, repo.Delete and taskRepo.DeleteAll have no expectations.
	})

	t.Run("DeleteWorkspace auth failure is wrapped and repo is never called", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		svc, deps := newGapMemberService(ctrl, nil, nil)
		sentinel := errors.New("expired")
		deps.gapAuthErr("ws1", sentinel)

		err := svc.DeleteWorkspace(ctx, "ws1")
		require.Error(t, err)
		assert.ErrorIs(t, err, sentinel)
		assert.EqualError(t, err, "failed to authenticate user: expired")
	})

	t.Run("SetAuditLogSettings non-owner blocks validation fetch and update", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		svc, deps := newGapMemberService(ctrl, nil, nil)
		deps.gapAuthAs("ws1", "member1", "member")
		days := 29

		err := svc.SetAuditLogSettings(ctx, "ws1", &domain.AuditLogSettings{RetentionDays: &days})
		require.Error(t, err)
		var unauth *domain.ErrUnauthorized
		require.ErrorAs(t, err, &unauth)
		assert.Equal(t, "Only workspace owners can change the audit log retention", unauth.Message)
		assert.EqualError(t, err, "Only workspace owners can change the audit log retention")
	})

	t.Run("SetAuditLogSettings invalid retention blocks workspace fetch and update", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		svc, deps := newGapMemberService(ctrl, nil, nil)
		deps.gapAuthAs("ws1", "owner1", "owner")
		days := 29

		err := svc.SetAuditLogSettings(ctx, "ws1", &domain.AuditLogSettings{RetentionDays: &days})
		require.Error(t, err)
		assert.EqualError(t, err, "retention_days must be 0 (keep forever) or between 30 and 3650")
		// repo.GetByID and repo.Update have no expectations.
	})

	t.Run("UpdateWorkspace non-owner blocks workspace fetch and update", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		svc, deps := newGapMemberService(ctrl, nil, nil)
		deps.gapAuthAs("ws1", "member1", "member")

		ws, err := svc.UpdateWorkspace(ctx, "ws1", "New Name", domain.WorkspaceSettings{})
		require.Error(t, err)
		assert.Nil(t, ws)
		var unauth *domain.ErrUnauthorized
		require.ErrorAs(t, err, &unauth)
		assert.EqualError(t, err, "user is not an owner of the workspace")
	})
}

func TestWorkspaceMembersGap_EnsureSeatBoundaries(t *testing.T) {
	ctx := context.Background()

	t.Run("unlimited config never counts seats", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		cfg := &config.Config{RootEmail: "root@example.com", MaxUsers: 0}
		svc, _ := newGapMemberService(ctrl, cfg, nil)

		require.NoError(t, svc.ensureSeat(ctx, "ws1", ""))
		// CountWorkspaceMembersAndInvitations has no expectation.
	})

	t.Run("one below the limit is admitted", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		cfg := &config.Config{RootEmail: "root@example.com", MaxUsers: 3}
		svc, deps := newGapMemberService(ctrl, cfg, nil)
		deps.repo.EXPECT().CountWorkspaceMembersAndInvitations(gomock.Any(), "ws1", "jane@example.com").Return(2, nil).Times(1)

		require.NoError(t, svc.ensureSeat(ctx, "ws1", "jane@example.com"))
	})

	t.Run("exactly at the limit is refused with exact counts", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		cfg := &config.Config{RootEmail: "root@example.com", MaxUsers: 3}
		svc, deps := newGapMemberService(ctrl, cfg, nil)
		deps.repo.EXPECT().CountWorkspaceMembersAndInvitations(gomock.Any(), "ws1", "").Return(3, nil).Times(1)

		err := svc.ensureSeat(ctx, "ws1", "")
		require.Error(t, err)
		var limitErr *domain.ErrTeamMemberLimitReached
		require.ErrorAs(t, err, &limitErr)
		assert.Equal(t, 3, limitErr.Limit)
		assert.Equal(t, 3, limitErr.Current)
		assert.EqualError(t, err, "team member limit reached: workspace has 3 members and invitations (limit: 3)")
	})

	t.Run("count error propagates exactly", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		cfg := &config.Config{RootEmail: "root@example.com", MaxUsers: 3}
		svc, deps := newGapMemberService(ctrl, cfg, nil)
		sentinel := errors.New("count failed")
		deps.repo.EXPECT().CountWorkspaceMembersAndInvitations(gomock.Any(), "ws1", "").Return(0, sentinel).Times(1)

		err := svc.ensureSeat(ctx, "ws1", "")
		require.Error(t, err)
		assert.ErrorIs(t, err, sentinel)
		assert.EqualError(t, err, "count failed")
	})
}

func TestWorkspaceMembersGap_CreateWorkspaceGuards(t *testing.T) {
	ctx := context.Background()

	t.Run("non-root user is refused before workspace count and create", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		svc, deps := newGapMemberService(ctrl, nil, nil)
		deps.auth.EXPECT().AuthenticateUserFromContext(gomock.Any()).
			Return(&domain.User{ID: "u1", Email: "member@example.com"}, nil).Times(1)

		ws, err := svc.CreateWorkspace(ctx, "wsnew1", "Acme", "", "", "", "UTC", domain.FileManagerSettings{}, "", nil)
		require.Error(t, err)
		assert.Nil(t, ws)
		var unauth *domain.ErrUnauthorized
		require.ErrorAs(t, err, &unauth)
		assert.EqualError(t, err, "only root user can create workspaces")
		// repo.CountWorkspaces and repo.Create have no expectations.
	})

	t.Run("uppercase workspace id is refused before workspace count", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		svc, deps := newGapMemberService(ctrl, nil, nil)
		deps.auth.EXPECT().AuthenticateUserFromContext(gomock.Any()).
			Return(&domain.User{ID: "root1", Email: "root@example.com"}, nil).Times(1)

		ws, err := svc.CreateWorkspace(ctx, "ABC123", "Acme", "", "", "", "UTC", domain.FileManagerSettings{}, "", nil)
		require.Error(t, err)
		assert.Nil(t, ws)
		assert.EqualError(t, err, "invalid workspace: id must contain only lowercase letters and digits")
	})

	t.Run("operator workspace limit at boundary blocks create", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		cfg := &config.Config{RootEmail: "root@example.com", MaxWorkspaces: 3}
		svc, deps := newGapMemberService(ctrl, cfg, nil)
		deps.auth.EXPECT().AuthenticateUserFromContext(gomock.Any()).
			Return(&domain.User{ID: "root1", Email: "root@example.com"}, nil).Times(1)
		deps.repo.EXPECT().CountWorkspaces(gomock.Any()).Return(3, nil).Times(1)
		// repo.Create has no expectation.

		ws, err := svc.CreateWorkspace(ctx, "wsnew1", "Acme", "", "", "", "UTC", domain.FileManagerSettings{}, "", nil)
		require.Error(t, err)
		assert.Nil(t, ws)
		var limitErr *domain.ErrWorkspaceLimitReached
		require.ErrorAs(t, err, &limitErr)
		assert.Equal(t, 3, limitErr.Limit)
		assert.Equal(t, 3, limitErr.Current)
		assert.EqualError(t, err, "workspace limit reached: 3 workspaces exist (limit: 3)")
	})
}

func TestWorkspaceMembersGap_ConnectZapierGuards(t *testing.T) {
	ctx := context.Background()

	t.Run("non-member maps to member unauthorized and mints nothing", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		svc, deps := newGapMemberService(ctrl, nil, nil)
		deps.auth.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), "ws1").
			Return(context.Background(), nil, nil, domain.ErrUserNotInWorkspace).Times(1)

		token, email, id, err := svc.ConnectZapier(ctx, "ws1", "Zapier")
		require.Error(t, err)
		assert.Empty(t, token)
		assert.Empty(t, email)
		assert.Empty(t, id)
		var unauth *domain.ErrUnauthorized
		require.ErrorAs(t, err, &unauth)
		assert.EqualError(t, err, "user is not a member of the workspace")
		// userRepo and repo have no expectations: no key was minted or stored.
	})

	t.Run("non-owner is refused before minting any key", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		svc, deps := newGapMemberService(ctrl, nil, nil)
		deps.gapAuthAs("ws1", "member1", "member")

		token, email, id, err := svc.ConnectZapier(ctx, "ws1", "Zapier")
		require.Error(t, err)
		assert.Empty(t, token)
		assert.Empty(t, email)
		assert.Empty(t, id)
		var unauth *domain.ErrUnauthorized
		require.ErrorAs(t, err, &unauth)
		assert.EqualError(t, err, "user is not an owner of the workspace")
	})

	t.Run("other auth errors are wrapped and mint nothing", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		svc, deps := newGapMemberService(ctrl, nil, nil)
		sentinel := errors.New("identity provider down")
		deps.gapAuthErr("ws1", sentinel)

		token, email, id, err := svc.ConnectZapier(ctx, "ws1", "Zapier")
		require.Error(t, err)
		assert.Empty(t, token)
		assert.Empty(t, email)
		assert.Empty(t, id)
		assert.ErrorIs(t, err, sentinel)
		assert.EqualError(t, err, "failed to authenticate user: identity provider down")
	})
}
