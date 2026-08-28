package service

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
	mocks "github.com/Notifuse/notifuse/internal/domain/mocks"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
)

func TestWorkspaceService_AddUserToWorkspace(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockUserRepo := mocks.NewMockUserRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockUserSvc := mocks.NewMockUserServiceInterface(ctrl)
	mockAuthSvc := mocks.NewMockAuthService(ctrl)
	mockMailer := pkgmocks.NewMockMailer(ctrl)
	mockContactService := mocks.NewMockContactService(ctrl)
	mockListService := mocks.NewMockListService(ctrl)
	mockContactListService := mocks.NewMockContactListService(ctrl)
	mockTemplateService := mocks.NewMockTemplateService(ctrl)
	mockWebhookRegService := mocks.NewMockWebhookRegistrationService(ctrl)
	cfg := &config.Config{}

	service := NewWorkspaceService(
		mockRepo,
		mockUserRepo,
		mocks.NewMockTaskRepository(ctrl),
		mockLogger,
		mockUserSvc,
		mockAuthSvc,
		mockMailer,
		cfg,
		mockContactService,
		mockListService,
		mockContactListService,
		mockTemplateService,
		mockWebhookRegService,
		"secret_key",
		&SupabaseService{},
		&DNSVerificationService{},
		&BlogService{},
	)

	ctx := context.Background()
	workspaceID := "workspace1"
	userID := "user1"
	requesterID := "requester1"

	// Setup common logger expectations
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

	t.Run("successful_add_user_to_workspace", func(t *testing.T) {
		// Set up mock expectations
		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(ctx, workspaceID).
			Return(ctx, &domain.User{ID: requesterID}, &domain.UserWorkspace{
				UserID:      requesterID,
				WorkspaceID: workspaceID,
				Role:        "owner",
			}, nil)

		mockRepo.EXPECT().
			AddUserToWorkspace(gomock.Any(), gomock.Any()).
			Return(nil)

		err := service.AddUserToWorkspace(ctx, workspaceID, userID, "member", domain.UserPermissions{})
		require.NoError(t, err)
	})

	t.Run("authentication_error", func(t *testing.T) {
		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(ctx, workspaceID).
			Return(ctx, nil, nil, fmt.Errorf("authentication failed"))

		err := service.AddUserToWorkspace(ctx, workspaceID, userID, "member", domain.UserPermissions{})
		require.Error(t, err)
		assert.Equal(t, "failed to authenticate user: authentication failed", err.Error())
	})

	t.Run("requester_not_found_in_workspace", func(t *testing.T) {
		// Access (and the requester's membership) is now proven entirely by
		// AuthenticateUserForWorkspace; a requester that is not a member of the
		// workspace surfaces as an authentication error here.
		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(ctx, workspaceID).
			Return(ctx, nil, nil, fmt.Errorf("user workspace not found"))

		err := service.AddUserToWorkspace(ctx, workspaceID, userID, "member", domain.UserPermissions{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "user workspace not found")
	})

	t.Run("requester_not_an_owner", func(t *testing.T) {
		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(ctx, workspaceID).
			Return(ctx, &domain.User{ID: requesterID}, &domain.UserWorkspace{
				UserID:      requesterID,
				WorkspaceID: workspaceID,
				Role:        "member",
			}, nil)

		err := service.AddUserToWorkspace(ctx, workspaceID, userID, "member", domain.UserPermissions{})
		require.Error(t, err)
		assert.Equal(t, "user is not an owner of the workspace", err.Error())
	})

	t.Run("unknown_permission_resource", func(t *testing.T) {
		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(ctx, workspaceID).
			Return(ctx, &domain.User{ID: requesterID}, &domain.UserWorkspace{
				UserID:      requesterID,
				WorkspaceID: workspaceID,
				Role:        "owner",
			}, nil)

		err := service.AddUserToWorkspace(ctx, workspaceID, userID, "member", domain.UserPermissions{
			"not_a_resource": {Read: true},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown permission resource")
	})

	t.Run("invalid_role", func(t *testing.T) {
		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(ctx, workspaceID).
			Return(ctx, &domain.User{ID: requesterID}, &domain.UserWorkspace{
				UserID:      requesterID,
				WorkspaceID: workspaceID,
				Role:        "owner",
			}, nil)

		err := service.AddUserToWorkspace(ctx, workspaceID, userID, "invalid_role", domain.UserPermissions{})
		require.Error(t, err)
		assert.Equal(t, "invalid user workspace: role must be either 'owner' or 'member'", err.Error())
	})
}

func TestWorkspaceService_RemoveUserFromWorkspace(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockUserRepo := mocks.NewMockUserRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockUserSvc := mocks.NewMockUserServiceInterface(ctrl)
	mockAuthSvc := mocks.NewMockAuthService(ctrl)
	mockMailer := pkgmocks.NewMockMailer(ctrl)
	mockContactService := mocks.NewMockContactService(ctrl)
	mockListService := mocks.NewMockListService(ctrl)
	mockContactListService := mocks.NewMockContactListService(ctrl)
	mockTemplateService := mocks.NewMockTemplateService(ctrl)
	mockWebhookRegService := mocks.NewMockWebhookRegistrationService(ctrl)
	cfg := &config.Config{}

	service := NewWorkspaceService(
		mockRepo,
		mockUserRepo,
		mocks.NewMockTaskRepository(ctrl),
		mockLogger,
		mockUserSvc,
		mockAuthSvc,
		mockMailer,
		cfg,
		mockContactService,
		mockListService,
		mockContactListService,
		mockTemplateService,
		mockWebhookRegService,
		"secret_key",
		&SupabaseService{},
		&DNSVerificationService{},
		&BlogService{},
	)

	// Setup common logger expectations
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

	ctx := context.Background()
	workspaceID := "workspace1"
	userID := "user1"
	requesterID := "requester1"

	t.Run("successful_remove_user_from_workspace", func(t *testing.T) {
		// Set up mock expectations
		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(ctx, workspaceID).
			Return(ctx, &domain.User{ID: requesterID}, &domain.UserWorkspace{
				UserID:      requesterID,
				WorkspaceID: workspaceID,
				Role:        "owner",
			}, nil)

		mockRepo.EXPECT().
			RemoveUserFromWorkspace(ctx, userID, workspaceID).
			Return(nil)

		err := service.RemoveUserFromWorkspace(ctx, workspaceID, userID)
		require.NoError(t, err)
	})

	t.Run("authentication_error", func(t *testing.T) {
		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(ctx, workspaceID).
			Return(ctx, nil, nil, fmt.Errorf("authentication failed"))

		err := service.RemoveUserFromWorkspace(ctx, workspaceID, userID)
		require.Error(t, err)
		assert.Equal(t, "failed to authenticate user: authentication failed", err.Error())
	})

	t.Run("requester_not_found_in_workspace", func(t *testing.T) {
		// The requester's membership is now proven entirely by
		// AuthenticateUserForWorkspace; a non-member surfaces as an auth error here.
		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(ctx, workspaceID).
			Return(ctx, nil, nil, fmt.Errorf("user is not a member of the workspace"))

		err := service.RemoveUserFromWorkspace(ctx, workspaceID, userID)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "user is not a member of the workspace")
	})

	t.Run("requester_not_an_owner", func(t *testing.T) {
		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(ctx, workspaceID).
			Return(ctx, &domain.User{ID: requesterID}, &domain.UserWorkspace{
				UserID:      requesterID,
				WorkspaceID: workspaceID,
				Role:        "member",
			}, nil)

		err := service.RemoveUserFromWorkspace(ctx, workspaceID, userID)
		require.Error(t, err)
		assert.Equal(t, "user is not an owner of the workspace", err.Error())
	})

	t.Run("target_user_not_found", func(t *testing.T) {
		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(ctx, workspaceID).
			Return(ctx, &domain.User{ID: requesterID}, &domain.UserWorkspace{
				UserID:      requesterID,
				WorkspaceID: workspaceID,
				Role:        "owner",
			}, nil)

		mockRepo.EXPECT().
			RemoveUserFromWorkspace(ctx, userID, workspaceID).
			Return(fmt.Errorf("user is not a member of the workspace"))

		err := service.RemoveUserFromWorkspace(ctx, workspaceID, userID)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "user is not a member of the workspace")
	})

	t.Run("cannot_remove_owner", func(t *testing.T) {
		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(ctx, workspaceID).
			Return(ctx, &domain.User{ID: requesterID}, &domain.UserWorkspace{
				UserID:      requesterID,
				WorkspaceID: workspaceID,
				Role:        "owner",
			}, nil)

		err := service.RemoveUserFromWorkspace(ctx, workspaceID, requesterID)
		require.Error(t, err)
		assert.Equal(t, "cannot remove yourself from the workspace", err.Error())
	})

	t.Run("cannot remove self", func(t *testing.T) {
		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(ctx, workspaceID).
			Return(ctx, &domain.User{ID: requesterID}, &domain.UserWorkspace{
				UserID:      requesterID,
				WorkspaceID: workspaceID,
				Role:        "owner",
			}, nil)

		err := service.RemoveUserFromWorkspace(ctx, workspaceID, requesterID)
		require.Error(t, err)
		assert.Equal(t, "cannot remove yourself from the workspace", err.Error())
	})
}

func TestWorkspaceService_TransferOwnership(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockUserRepo := mocks.NewMockUserRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockUserSvc := mocks.NewMockUserServiceInterface(ctrl)
	mockAuthSvc := mocks.NewMockAuthService(ctrl)
	mockMailer := pkgmocks.NewMockMailer(ctrl)
	mockContactService := mocks.NewMockContactService(ctrl)
	mockListService := mocks.NewMockListService(ctrl)
	mockContactListService := mocks.NewMockContactListService(ctrl)
	mockTemplateService := mocks.NewMockTemplateService(ctrl)
	mockWebhookRegService := mocks.NewMockWebhookRegistrationService(ctrl)
	cfg := &config.Config{}

	service := NewWorkspaceService(
		mockRepo,
		mockUserRepo,
		mocks.NewMockTaskRepository(ctrl),
		mockLogger,
		mockUserSvc,
		mockAuthSvc,
		mockMailer,
		cfg,
		mockContactService,
		mockListService,
		mockContactListService,
		mockTemplateService,
		mockWebhookRegService,
		"secret_key",
		&SupabaseService{},
		&DNSVerificationService{},
		&BlogService{},
	)

	ctx := context.Background()
	workspaceID := "workspace1"
	userID := "user1"
	requesterID := "requester1"

	// Setup common logger expectations
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

	t.Run("successful transfer ownership", func(t *testing.T) {
		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(ctx, workspaceID).
			Return(ctx, &domain.User{ID: requesterID}, nil, nil)

		mockRepo.EXPECT().
			GetUserWorkspace(ctx, requesterID, workspaceID).
			Return(&domain.UserWorkspace{
				UserID:      requesterID,
				WorkspaceID: workspaceID,
				Role:        "owner",
			}, nil)

		mockRepo.EXPECT().
			GetUserWorkspace(ctx, userID, workspaceID).
			Return(&domain.UserWorkspace{
				UserID:      userID,
				WorkspaceID: workspaceID,
				Role:        "member",
			}, nil)

		mockRepo.EXPECT().
			AddUserToWorkspace(ctx, gomock.Any()).
			DoAndReturn(func(_ context.Context, uw *domain.UserWorkspace) error {
				assert.Equal(t, userID, uw.UserID)
				assert.Equal(t, workspaceID, uw.WorkspaceID)
				assert.Equal(t, "owner", uw.Role)
				return nil
			})

		mockRepo.EXPECT().
			AddUserToWorkspace(ctx, gomock.Any()).
			DoAndReturn(func(_ context.Context, uw *domain.UserWorkspace) error {
				assert.Equal(t, requesterID, uw.UserID)
				assert.Equal(t, workspaceID, uw.WorkspaceID)
				assert.Equal(t, "member", uw.Role)
				return nil
			})

		err := service.TransferOwnership(ctx, workspaceID, userID, requesterID)
		require.NoError(t, err)
	})

	t.Run("authentication error", func(t *testing.T) {
		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(ctx, workspaceID).
			Return(ctx, nil, nil, fmt.Errorf("authentication failed"))

		err := service.TransferOwnership(ctx, workspaceID, userID, requesterID)
		require.Error(t, err)
		assert.Equal(t, "failed to authenticate user: authentication failed", err.Error())
	})

	t.Run("requester not found in workspace", func(t *testing.T) {
		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(ctx, workspaceID).
			Return(ctx, &domain.User{ID: requesterID}, nil, nil)

		mockRepo.EXPECT().
			GetUserWorkspace(ctx, requesterID, workspaceID).
			Return(nil, fmt.Errorf("user workspace not found"))

		err := service.TransferOwnership(ctx, workspaceID, userID, requesterID)
		require.Error(t, err)
		assert.Equal(t, "user workspace not found", err.Error())
	})

	t.Run("requester not an owner", func(t *testing.T) {
		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(ctx, workspaceID).
			Return(ctx, &domain.User{ID: requesterID}, nil, nil)

		mockRepo.EXPECT().
			GetUserWorkspace(ctx, requesterID, workspaceID).
			Return(&domain.UserWorkspace{
				UserID:      requesterID,
				WorkspaceID: workspaceID,
				Role:        "member",
			}, nil)

		err := service.TransferOwnership(ctx, workspaceID, userID, requesterID)
		require.Error(t, err)
		assert.IsType(t, &domain.ErrUnauthorized{}, err)
		assert.Equal(t, "user is not an owner of the workspace", err.Error())
	})

	t.Run("target user not found in workspace", func(t *testing.T) {
		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(ctx, workspaceID).
			Return(ctx, &domain.User{ID: requesterID}, nil, nil)

		mockRepo.EXPECT().
			GetUserWorkspace(ctx, requesterID, workspaceID).
			Return(&domain.UserWorkspace{
				UserID:      requesterID,
				WorkspaceID: workspaceID,
				Role:        "owner",
			}, nil)

		mockRepo.EXPECT().
			GetUserWorkspace(ctx, userID, workspaceID).
			Return(nil, fmt.Errorf("user workspace not found"))

		err := service.TransferOwnership(ctx, workspaceID, userID, requesterID)
		require.Error(t, err)
		assert.Equal(t, "user workspace not found", err.Error())
	})

	t.Run("target user is already an owner", func(t *testing.T) {
		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(ctx, workspaceID).
			Return(ctx, &domain.User{ID: requesterID}, nil, nil)

		mockRepo.EXPECT().
			GetUserWorkspace(ctx, requesterID, workspaceID).
			Return(&domain.UserWorkspace{
				UserID:      requesterID,
				WorkspaceID: workspaceID,
				Role:        "owner",
			}, nil)

		mockRepo.EXPECT().
			GetUserWorkspace(ctx, userID, workspaceID).
			Return(&domain.UserWorkspace{
				UserID:      userID,
				WorkspaceID: workspaceID,
				Role:        "owner",
			}, nil)

		err := service.TransferOwnership(ctx, workspaceID, userID, requesterID)
		require.Error(t, err)
		assert.Equal(t, "new owner must be a current member of the workspace", err.Error())
	})
}

func TestWorkspaceService_GetWorkspaceMembersWithEmail(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockUserRepo := mocks.NewMockUserRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockUserSvc := mocks.NewMockUserServiceInterface(ctrl)
	mockAuthSvc := mocks.NewMockAuthService(ctrl)
	mockMailer := pkgmocks.NewMockMailer(ctrl)
	mockContactService := mocks.NewMockContactService(ctrl)
	mockListService := mocks.NewMockListService(ctrl)
	mockContactListService := mocks.NewMockContactListService(ctrl)
	mockTemplateService := mocks.NewMockTemplateService(ctrl)
	mockWebhookRegService := mocks.NewMockWebhookRegistrationService(ctrl)
	cfg := &config.Config{}

	service := NewWorkspaceService(
		mockRepo,
		mockUserRepo,
		mocks.NewMockTaskRepository(ctrl),
		mockLogger,
		mockUserSvc,
		mockAuthSvc,
		mockMailer,
		cfg,
		mockContactService,
		mockListService,
		mockContactListService,
		mockTemplateService,
		mockWebhookRegService,
		"secret_key",
		&SupabaseService{},
		&DNSVerificationService{},
		&BlogService{},
	)

	ctx := context.Background()
	workspaceID := "workspace1"
	userID := "user1"
	now := time.Now()

	// Setup common logger expectations
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

	// Mock workspace invitations for all test cases (empty by default)
	mockRepo.EXPECT().
		GetWorkspaceInvitations(ctx, workspaceID).
		Return([]*domain.WorkspaceInvitation{}, nil).AnyTimes()

	ownerWorkspace := &domain.UserWorkspace{UserID: userID, WorkspaceID: workspaceID, Role: "owner"}

	t.Run("successful get members with email", func(t *testing.T) {
		expectedUser := &domain.User{
			ID: userID,
		}

		expectedMembers := []*domain.UserWorkspaceWithEmail{
			{
				UserWorkspace: domain.UserWorkspace{
					UserID:      "user1",
					WorkspaceID: workspaceID,
					Role:        "owner",
					CreatedAt:   now,
					UpdatedAt:   now,
				},
				Email: "user1@example.com",
			},
			{
				UserWorkspace: domain.UserWorkspace{
					UserID:      "user2",
					WorkspaceID: workspaceID,
					Role:        "member",
					CreatedAt:   now,
					UpdatedAt:   now,
				},
				Email: "user2@example.com",
			},
		}

		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(ctx, workspaceID).
			Return(ctx, expectedUser, ownerWorkspace, nil)

		mockRepo.EXPECT().
			GetWorkspaceUsersWithEmail(ctx, workspaceID).
			Return(expectedMembers, nil)

		members, err := service.GetWorkspaceMembersWithEmail(ctx, workspaceID)
		require.NoError(t, err)
		assert.Equal(t, expectedMembers, members)
	})

	t.Run("member with workspace read gets the full roster", func(t *testing.T) {
		allMembers := []*domain.UserWorkspaceWithEmail{
			{
				UserWorkspace: domain.UserWorkspace{UserID: "owner1", WorkspaceID: workspaceID, Role: "owner"},
				Email:         "owner1@example.com",
			},
			{
				UserWorkspace: domain.UserWorkspace{UserID: userID, WorkspaceID: workspaceID, Role: "member"},
				Email:         "user1@example.com",
			},
		}

		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(ctx, workspaceID).
			Return(ctx, &domain.User{ID: userID}, &domain.UserWorkspace{
				UserID:      userID,
				WorkspaceID: workspaceID,
				Role:        "member",
				Permissions: domain.UserPermissions{
					domain.PermissionResourceWorkspace: {Read: true},
				},
			}, nil)

		mockRepo.EXPECT().
			GetWorkspaceUsersWithEmail(ctx, workspaceID).
			Return(allMembers, nil)

		members, err := service.GetWorkspaceMembersWithEmail(ctx, workspaceID)
		require.NoError(t, err)
		assert.Equal(t, allMembers, members)
	})

	t.Run("member without workspace read is degraded to its own row", func(t *testing.T) {
		// A denial here would blank the console (this is its only source of the signed-in
		// user's permission map), so the roster is narrowed instead of refused.
		allMembers := []*domain.UserWorkspaceWithEmail{
			{
				UserWorkspace: domain.UserWorkspace{UserID: "owner1", WorkspaceID: workspaceID, Role: "owner"},
				Email:         "owner1@example.com",
			},
			{
				UserWorkspace: domain.UserWorkspace{UserID: userID, WorkspaceID: workspaceID, Role: "member"},
				Email:         "user1@example.com",
			},
			{
				UserWorkspace: domain.UserWorkspace{UserID: "user2", WorkspaceID: workspaceID, Role: "member"},
				Email:         "user2@example.com",
			},
		}

		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(ctx, workspaceID).
			Return(ctx, &domain.User{ID: userID}, &domain.UserWorkspace{
				UserID:      userID,
				WorkspaceID: workspaceID,
				Role:        "member",
				Permissions: domain.UserPermissions{
					domain.PermissionResourceContacts: {Read: true},
				},
			}, nil)

		mockRepo.EXPECT().
			GetWorkspaceUsersWithEmail(ctx, workspaceID).
			Return(allMembers, nil)

		members, err := service.GetWorkspaceMembersWithEmail(ctx, workspaceID)
		require.NoError(t, err)
		assert.Equal(t, []*domain.UserWorkspaceWithEmail{allMembers[1]}, members)
	})

	t.Run("nil requester workspace is degraded, not bypassed", func(t *testing.T) {
		allMembers := []*domain.UserWorkspaceWithEmail{
			{
				UserWorkspace: domain.UserWorkspace{UserID: "owner1", WorkspaceID: workspaceID, Role: "owner"},
				Email:         "owner1@example.com",
			},
			{
				UserWorkspace: domain.UserWorkspace{UserID: userID, WorkspaceID: workspaceID, Role: "member"},
				Email:         "user1@example.com",
			},
		}

		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(ctx, workspaceID).
			Return(ctx, &domain.User{ID: userID}, nil, nil)

		mockRepo.EXPECT().
			GetWorkspaceUsersWithEmail(ctx, workspaceID).
			Return(allMembers, nil)

		members, err := service.GetWorkspaceMembersWithEmail(ctx, workspaceID)
		require.NoError(t, err)
		assert.Equal(t, []*domain.UserWorkspaceWithEmail{allMembers[1]}, members)
	})

	t.Run("authentication error", func(t *testing.T) {
		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(ctx, workspaceID).
			Return(ctx, nil, nil, fmt.Errorf("authentication failed"))

		members, err := service.GetWorkspaceMembersWithEmail(ctx, workspaceID)
		require.Error(t, err)
		assert.Nil(t, members)
		assert.Equal(t, "failed to authenticate user: authentication failed", err.Error())
	})

	t.Run("repository error", func(t *testing.T) {
		expectedUser := &domain.User{
			ID: userID,
		}

		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(ctx, workspaceID).
			Return(ctx, expectedUser, ownerWorkspace, nil)

		mockRepo.EXPECT().
			GetWorkspaceUsersWithEmail(ctx, workspaceID).
			Return(nil, fmt.Errorf("database error"))

		members, err := service.GetWorkspaceMembersWithEmail(ctx, workspaceID)
		require.Error(t, err)
		assert.Nil(t, members)
		assert.Equal(t, "database error", err.Error())
	})
}

func TestWorkspaceService_InviteMember(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockUserRepo := mocks.NewMockUserRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockUserSvc := mocks.NewMockUserServiceInterface(ctrl)
	mockAuthSvc := mocks.NewMockAuthService(ctrl)
	mockMailer := pkgmocks.NewMockMailer(ctrl)
	mockContactService := mocks.NewMockContactService(ctrl)
	mockListService := mocks.NewMockListService(ctrl)
	mockContactListService := mocks.NewMockContactListService(ctrl)
	mockTemplateService := mocks.NewMockTemplateService(ctrl)
	mockWebhookRegService := mocks.NewMockWebhookRegistrationService(ctrl)
	cfg := &config.Config{
		Environment: "development",
	}

	service := NewWorkspaceService(
		mockRepo,
		mockUserRepo,
		mocks.NewMockTaskRepository(ctrl),
		mockLogger,
		mockUserSvc,
		mockAuthSvc,
		mockMailer,
		cfg,
		mockContactService,
		mockListService,
		mockContactListService,
		mockTemplateService,
		mockWebhookRegService,
		"secret_key",
		&SupabaseService{},
		&DNSVerificationService{},
		&BlogService{},
	)

	// Set up mockLogger to allow any calls
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()

	ctx := context.Background()
	workspaceID := "workspace1"
	inviterID := "inviter1"
	email := "test@example.com"

	t.Run("successful invitation for new user in production", func(t *testing.T) {
		// Setup common logger expectations
		mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
		mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()

		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(ctx, workspaceID).
			Return(ctx, &domain.User{ID: inviterID}, &domain.UserWorkspace{UserID: inviterID, WorkspaceID: workspaceID, Role: "owner"}, nil)

		mockRepo.EXPECT().
			GetByID(ctx, workspaceID).
			Return(&domain.Workspace{
				ID:   workspaceID,
				Name: "Test Workspace",
			}, nil)

		mockUserSvc.EXPECT().
			GetUserByID(ctx, inviterID).
			Return(&domain.User{
				ID:       inviterID,
				Name:     "Test Inviter",
				Email:    "inviter@example.com",
				Language: "de",
			}, nil)

		mockUserSvc.EXPECT().
			GetUserByEmail(ctx, email).
			Return(nil, &domain.ErrUserNotFound{})

		// Mock invitation creation
		mockRepo.EXPECT().
			CreateInvitation(ctx, gomock.Any()).
			DoAndReturn(func(_ context.Context, invitation *domain.WorkspaceInvitation) error {
				assert.Equal(t, workspaceID, invitation.WorkspaceID)
				assert.Equal(t, inviterID, invitation.InviterID)
				assert.Equal(t, email, invitation.Email)
				assert.NotEmpty(t, invitation.ID)
				return nil
			})

		// Set config to production to test email sending
		cfg.Environment = "production"

		// Mock token generation
		mockAuthSvc.EXPECT().
			GenerateInvitationToken(gomock.Any()).
			Return("test-token")

		// We expect the invitation email to be sent
		mockMailer.EXPECT().
			SendWorkspaceInvitation(
				email,
				"Test Workspace",
				"Test Inviter",
				"test-token",
				"de",
			).Return(nil)

		invitation, token, err := service.InviteMember(ctx, workspaceID, email, domain.UserPermissions{})
		require.NoError(t, err)
		assert.NotNil(t, invitation)
		assert.Empty(t, token) // In production mode, token is not returned
		assert.Equal(t, workspaceID, invitation.WorkspaceID)
		assert.Equal(t, inviterID, invitation.InviterID)
		assert.Equal(t, email, invitation.Email)

		// Reset config for other tests
		cfg.Environment = "development"
	})

	t.Run("successful invitation for existing user", func(t *testing.T) {
		existingUser := &domain.User{
			ID:    "existing-user",
			Email: email,
		}

		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(ctx, workspaceID).
			Return(ctx, &domain.User{ID: inviterID}, &domain.UserWorkspace{UserID: inviterID, WorkspaceID: workspaceID, Role: "owner"}, nil)

		mockRepo.EXPECT().
			GetByID(ctx, workspaceID).
			Return(&domain.Workspace{
				ID:   workspaceID,
				Name: "Test Workspace",
			}, nil)

		mockUserSvc.EXPECT().
			GetUserByID(ctx, inviterID).
			Return(&domain.User{
				ID:    inviterID,
				Name:  "Test Inviter",
				Email: "inviter@example.com",
			}, nil)

		mockUserSvc.EXPECT().
			GetUserByEmail(ctx, email).
			Return(existingUser, nil)

		mockRepo.EXPECT().
			IsUserWorkspaceMember(ctx, existingUser.ID, workspaceID).
			Return(false, nil)

		mockRepo.EXPECT().
			AddUserToWorkspace(ctx, gomock.Any()).
			DoAndReturn(func(_ context.Context, uw *domain.UserWorkspace) error {
				assert.Equal(t, existingUser.ID, uw.UserID)
				assert.Equal(t, workspaceID, uw.WorkspaceID)
				assert.Equal(t, "member", uw.Role)
				return nil
			})

		invitation, token, err := service.InviteMember(ctx, workspaceID, email, domain.UserPermissions{})
		require.NoError(t, err)
		assert.Nil(t, invitation)
		assert.Empty(t, token)
	})

	t.Run("invalid_email_format", func(t *testing.T) {
		// Mock authentication - this should be called before email validation
		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(ctx, workspaceID).
			Return(ctx, &domain.User{ID: inviterID}, &domain.UserWorkspace{UserID: inviterID, WorkspaceID: workspaceID, Role: "owner"}, nil)

		invitation, token, err := service.InviteMember(ctx, workspaceID, "invalid-email", domain.UserPermissions{})
		require.Error(t, err)
		assert.Nil(t, invitation)
		assert.Empty(t, token)
		assert.Contains(t, err.Error(), "invalid email format")
	})

	t.Run("authentication error", func(t *testing.T) {
		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(ctx, workspaceID).
			Return(ctx, nil, nil, fmt.Errorf("authentication failed"))

		invitation, token, err := service.InviteMember(ctx, workspaceID, email, domain.UserPermissions{})
		require.Error(t, err)
		assert.Nil(t, invitation)
		assert.Empty(t, token)
		assert.Contains(t, err.Error(), "authentication failed")
	})

	t.Run("workspace not found", func(t *testing.T) {
		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(ctx, workspaceID).
			Return(ctx, &domain.User{ID: inviterID}, &domain.UserWorkspace{UserID: inviterID, WorkspaceID: workspaceID, Role: "owner"}, nil)

		mockRepo.EXPECT().
			GetByID(ctx, workspaceID).
			Return(nil, fmt.Errorf("workspace not found"))

		invitation, token, err := service.InviteMember(ctx, workspaceID, email, domain.UserPermissions{})
		require.Error(t, err)
		assert.Nil(t, invitation)
		assert.Empty(t, token)
		assert.Contains(t, err.Error(), "workspace not found")
	})

	t.Run("inviter not a member", func(t *testing.T) {
		// The inviter's access (and membership) is now proven entirely by
		// AuthenticateUserForWorkspace; a non-member inviter is rejected there with the
		// real sentinel error, which InviteMember wraps as "failed to authenticate user".
		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(ctx, workspaceID).
			Return(ctx, nil, nil, domain.ErrUserNotInWorkspace)

		invitation, token, err := service.InviteMember(ctx, workspaceID, email, domain.UserPermissions{})
		require.Error(t, err)
		assert.Nil(t, invitation)
		assert.Empty(t, token)
		assert.Contains(t, err.Error(), "failed to authenticate user")
	})

	t.Run("inviter is not an owner", func(t *testing.T) {
		// An invitation carries an arbitrary permission map, so a scoped member — or a
		// scoped API key, which is a member row — must not be able to issue one.
		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(ctx, workspaceID).
			Return(ctx, &domain.User{ID: inviterID}, &domain.UserWorkspace{
				UserID:      inviterID,
				WorkspaceID: workspaceID,
				Role:        "member",
				Permissions: domain.NewFullPermissions(),
			}, nil)

		invitation, token, err := service.InviteMember(ctx, workspaceID, email, domain.NewFullPermissions())
		require.Error(t, err)
		assert.Nil(t, invitation)
		assert.Empty(t, token)
		assert.IsType(t, &domain.ErrUnauthorized{}, err)
		assert.Equal(t, "user is not an owner of the workspace", err.Error())
	})

	t.Run("unknown permission resource is rejected", func(t *testing.T) {
		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(ctx, workspaceID).
			Return(ctx, &domain.User{ID: inviterID}, &domain.UserWorkspace{
				UserID:      inviterID,
				WorkspaceID: workspaceID,
				Role:        "owner",
			}, nil)

		invitation, token, err := service.InviteMember(ctx, workspaceID, email, domain.UserPermissions{
			"not_a_resource": {Read: true},
		})
		require.Error(t, err)
		assert.Nil(t, invitation)
		assert.Empty(t, token)
		assert.Contains(t, err.Error(), "unknown permission resource")
	})

	t.Run("user already a member", func(t *testing.T) {
		existingUser := &domain.User{
			ID:    "existing-user",
			Email: email,
		}

		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(ctx, workspaceID).
			Return(ctx, &domain.User{ID: inviterID}, &domain.UserWorkspace{UserID: inviterID, WorkspaceID: workspaceID, Role: "owner"}, nil)

		mockRepo.EXPECT().
			GetByID(ctx, workspaceID).
			Return(&domain.Workspace{
				ID:   workspaceID,
				Name: "Test Workspace",
			}, nil)

		mockUserSvc.EXPECT().
			GetUserByID(ctx, inviterID).
			Return(&domain.User{
				ID:    inviterID,
				Name:  "Test Inviter",
				Email: "inviter@example.com",
			}, nil)

		mockUserSvc.EXPECT().
			GetUserByEmail(ctx, email).
			Return(existingUser, nil)

		mockRepo.EXPECT().
			IsUserWorkspaceMember(ctx, existingUser.ID, workspaceID).
			Return(true, nil)

		invitation, token, err := service.InviteMember(ctx, workspaceID, email, domain.UserPermissions{})
		require.Error(t, err)
		assert.Nil(t, invitation)
		assert.Empty(t, token)
		assert.Contains(t, err.Error(), "user is already a member of the workspace")
	})
}

func TestWorkspaceService_CreateAPIKey(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// These variables are only used to share common mock configs across subtests
	// Each subtest creates its own service instance with freshly created mocks
	// nolint
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockUserSvc := mocks.NewMockUserServiceInterface(ctrl)
	mockMailer := pkgmocks.NewMockMailer(ctrl)
	mockContactService := mocks.NewMockContactService(ctrl)
	mockListService := mocks.NewMockListService(ctrl)
	mockContactListService := mocks.NewMockContactListService(ctrl)
	mockTemplateService := mocks.NewMockTemplateService(ctrl)
	mockWebhookRegService := mocks.NewMockWebhookRegistrationService(ctrl)
	cfg := &config.Config{APIEndpoint: "https://api.example.com/v1"}

	ctx := context.Background()
	workspaceID := "workspace1"
	userID := "user1"
	emailPrefix := "test-api"
	expectedDomain := "api.example.com"
	expectedEmail := emailPrefix + "@" + expectedDomain
	expectedToken := "test-token"

	// Setup common logger expectations
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

	t.Run("successful_create_api_key", func(t *testing.T) {
		// Set up fresh controller for each test to ensure independent mocks
		subCtrl := gomock.NewController(t)
		defer subCtrl.Finish()

		mockRepo := mocks.NewMockWorkspaceRepository(subCtrl)
		mockUserRepo := mocks.NewMockUserRepository(subCtrl)
		mockAuthSvc := mocks.NewMockAuthService(subCtrl)

		subService := NewWorkspaceService(
			mockRepo,
			mockUserRepo,
			mocks.NewMockTaskRepository(ctrl),
			mockLogger,
			mockUserSvc,
			mockAuthSvc,
			mockMailer,
			cfg,
			mockContactService,
			mockListService,
			mockContactListService,
			mockTemplateService,
			mockWebhookRegService,
			"secret_key",
			&SupabaseService{},
			&DNSVerificationService{},
			&BlogService{},
		)

		// Set up mock expectations
		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(ctx, workspaceID).
			Return(ctx, &domain.User{ID: userID}, &domain.UserWorkspace{
				UserID:      userID,
				WorkspaceID: workspaceID,
				Role:        "owner",
			}, nil)

		// Expect creating a user with API type
		mockUserRepo.EXPECT().
			CreateUser(ctx, gomock.Any()).
			DoAndReturn(func(_ context.Context, user *domain.User) error {
				assert.Equal(t, expectedEmail, user.Email)
				assert.Equal(t, domain.UserTypeAPIKey, user.Type)
				return nil
			})

		// Expect adding the user to the workspace
		mockRepo.EXPECT().
			AddUserToWorkspace(ctx, gomock.Any()).
			DoAndReturn(func(_ context.Context, userWorkspace *domain.UserWorkspace) error {
				assert.Equal(t, workspaceID, userWorkspace.WorkspaceID)
				assert.Equal(t, "member", userWorkspace.Role)
				assert.Equal(t, domain.NewFullPermissions(), userWorkspace.Permissions)
				return nil
			})

		// Expect generating a token
		mockAuthSvc.EXPECT().
			GenerateAPIAuthToken(gomock.Any()).
			Return(expectedToken)

		token, email, err := subService.CreateAPIKey(ctx, workspaceID, emailPrefix, nil)
		require.NoError(t, err)
		assert.Equal(t, expectedToken, token)
		assert.Equal(t, expectedEmail, email)
	})

	t.Run("authentication_error", func(t *testing.T) {
		subCtrl := gomock.NewController(t)
		defer subCtrl.Finish()

		mockRepo := mocks.NewMockWorkspaceRepository(subCtrl)
		mockUserRepo := mocks.NewMockUserRepository(subCtrl)
		mockAuthSvc := mocks.NewMockAuthService(subCtrl)

		subService := NewWorkspaceService(
			mockRepo,
			mockUserRepo,
			mocks.NewMockTaskRepository(ctrl),
			mockLogger,
			mockUserSvc,
			mockAuthSvc,
			mockMailer,
			cfg,
			mockContactService,
			mockListService,
			mockContactListService,
			mockTemplateService,
			mockWebhookRegService,
			"secret_key",
			&SupabaseService{},
			&DNSVerificationService{},
			&BlogService{},
		)

		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(ctx, workspaceID).
			Return(ctx, nil, nil, fmt.Errorf("authentication failed"))

		token, email, err := subService.CreateAPIKey(ctx, workspaceID, emailPrefix, nil)
		require.Error(t, err)
		assert.Equal(t, "", token)
		assert.Equal(t, "", email)
		assert.Equal(t, "failed to authenticate user: authentication failed", err.Error())
	})

	t.Run("not_workspace_owner", func(t *testing.T) {
		subCtrl := gomock.NewController(t)
		defer subCtrl.Finish()

		mockRepo := mocks.NewMockWorkspaceRepository(subCtrl)
		mockUserRepo := mocks.NewMockUserRepository(subCtrl)
		mockAuthSvc := mocks.NewMockAuthService(subCtrl)

		subService := NewWorkspaceService(
			mockRepo,
			mockUserRepo,
			mocks.NewMockTaskRepository(ctrl),
			mockLogger,
			mockUserSvc,
			mockAuthSvc,
			mockMailer,
			cfg,
			mockContactService,
			mockListService,
			mockContactListService,
			mockTemplateService,
			mockWebhookRegService,
			"secret_key",
			&SupabaseService{},
			&DNSVerificationService{},
			&BlogService{},
		)

		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(ctx, workspaceID).
			Return(ctx, &domain.User{ID: userID}, &domain.UserWorkspace{
				UserID:      userID,
				WorkspaceID: workspaceID,
				Role:        "member", // Not an owner
			}, nil)

		token, email, err := subService.CreateAPIKey(ctx, workspaceID, emailPrefix, nil)
		require.Error(t, err)
		assert.Equal(t, "", token)
		assert.Equal(t, "", email)
		assert.IsType(t, &domain.ErrUnauthorized{}, err)
		assert.Equal(t, "user is not an owner of the workspace", err.Error())
	})

	t.Run("user_creation_error", func(t *testing.T) {
		subCtrl := gomock.NewController(t)
		defer subCtrl.Finish()

		mockRepo := mocks.NewMockWorkspaceRepository(subCtrl)
		mockUserRepo := mocks.NewMockUserRepository(subCtrl)
		mockAuthSvc := mocks.NewMockAuthService(subCtrl)

		subService := NewWorkspaceService(
			mockRepo,
			mockUserRepo,
			mocks.NewMockTaskRepository(ctrl),
			mockLogger,
			mockUserSvc,
			mockAuthSvc,
			mockMailer,
			cfg,
			mockContactService,
			mockListService,
			mockContactListService,
			mockTemplateService,
			mockWebhookRegService,
			"secret_key",
			&SupabaseService{},
			&DNSVerificationService{},
			&BlogService{},
		)

		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(ctx, workspaceID).
			Return(ctx, &domain.User{ID: userID}, &domain.UserWorkspace{
				UserID:      userID,
				WorkspaceID: workspaceID,
				Role:        "owner",
			}, nil)

		// The API auth token should not be generated if user creation fails
		mockUserRepo.EXPECT().
			CreateUser(ctx, gomock.Any()).
			Return(fmt.Errorf("user creation failed"))

		token, email, err := subService.CreateAPIKey(ctx, workspaceID, emailPrefix, nil)
		require.Error(t, err)
		assert.Equal(t, "", token)
		assert.Equal(t, "", email)
		assert.Equal(t, "user creation failed", err.Error())
	})

	t.Run("workspace_member_creation_error", func(t *testing.T) {
		subCtrl := gomock.NewController(t)
		defer subCtrl.Finish()

		mockRepo := mocks.NewMockWorkspaceRepository(subCtrl)
		mockUserRepo := mocks.NewMockUserRepository(subCtrl)
		mockAuthSvc := mocks.NewMockAuthService(subCtrl)

		subService := NewWorkspaceService(
			mockRepo,
			mockUserRepo,
			mocks.NewMockTaskRepository(ctrl),
			mockLogger,
			mockUserSvc,
			mockAuthSvc,
			mockMailer,
			cfg,
			mockContactService,
			mockListService,
			mockContactListService,
			mockTemplateService,
			mockWebhookRegService,
			"secret_key",
			&SupabaseService{},
			&DNSVerificationService{},
			&BlogService{},
		)

		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(ctx, workspaceID).
			Return(ctx, &domain.User{ID: userID}, &domain.UserWorkspace{
				UserID:      userID,
				WorkspaceID: workspaceID,
				Role:        "owner",
			}, nil)

		var createdUserID string
		mockUserRepo.EXPECT().
			CreateUser(ctx, gomock.Any()).
			DoAndReturn(func(_ context.Context, user *domain.User) error {
				createdUserID = user.ID
				return nil
			})

		mockRepo.EXPECT().
			AddUserToWorkspace(ctx, gomock.Any()).
			Return(fmt.Errorf("add to workspace failed"))

		// The half-written users row has to go: users.email is unique installation-wide, so a
		// row with no membership behind it burns the address for every workspace on the
		// deployment while showing up on no Team screen. It runs on a detached context, hence
		// gomock.Any() rather than ctx. Failing it must not change what the caller is told.
		deleted := false
		mockUserRepo.EXPECT().
			Delete(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, id string) error {
				deleted = true
				assert.Equal(t, createdUserID, id)
				return fmt.Errorf("delete failed too")
			})

		token, email, err := subService.CreateAPIKey(ctx, workspaceID, emailPrefix, nil)
		require.Error(t, err)
		assert.Equal(t, "", token)
		assert.Equal(t, "", email)
		assert.Equal(t, "add to workspace failed", err.Error())
		assert.True(t, deleted, "the API user must be deleted when it could not be added to the workspace")
	})

	t.Run("stored_permissions", func(t *testing.T) {
		scoped := domain.UserPermissions{
			domain.PermissionResourceTransactional: {Write: true},
		}

		testCases := []struct {
			name     string
			given    domain.UserPermissions
			expected domain.UserPermissions
		}{
			{"nil grants full access", nil, domain.NewFullPermissions()},
			{"scoped map is stored verbatim", scoped, scoped},
			{"empty map is stored empty", domain.UserPermissions{}, domain.UserPermissions{}},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				subCtrl := gomock.NewController(t)
				defer subCtrl.Finish()

				mockRepo := mocks.NewMockWorkspaceRepository(subCtrl)
				mockUserRepo := mocks.NewMockUserRepository(subCtrl)
				mockAuthSvc := mocks.NewMockAuthService(subCtrl)

				subService := NewWorkspaceService(
					mockRepo,
					mockUserRepo,
					mocks.NewMockTaskRepository(ctrl),
					mockLogger,
					mockUserSvc,
					mockAuthSvc,
					mockMailer,
					cfg,
					mockContactService,
					mockListService,
					mockContactListService,
					mockTemplateService,
					mockWebhookRegService,
					"secret_key",
					&SupabaseService{},
					&DNSVerificationService{},
					&BlogService{},
				)

				mockAuthSvc.EXPECT().
					AuthenticateUserForWorkspace(ctx, workspaceID).
					Return(ctx, &domain.User{ID: userID}, &domain.UserWorkspace{
						UserID:      userID,
						WorkspaceID: workspaceID,
						Role:        "owner",
					}, nil)

				mockUserRepo.EXPECT().CreateUser(ctx, gomock.Any()).Return(nil)

				var stored domain.UserPermissions
				mockRepo.EXPECT().
					AddUserToWorkspace(ctx, gomock.Any()).
					DoAndReturn(func(_ context.Context, userWorkspace *domain.UserWorkspace) error {
						stored = userWorkspace.Permissions
						return nil
					})

				mockAuthSvc.EXPECT().GenerateAPIAuthToken(gomock.Any()).Return(expectedToken)

				_, _, err := subService.CreateAPIKey(ctx, workspaceID, emailPrefix, tc.given)
				require.NoError(t, err)
				assert.Equal(t, tc.expected, stored)
			})
		}
	})

	t.Run("stored_permissions_are_copies", func(t *testing.T) {
		// The membership row must own its map: sharing domain.FullPermissions, or the
		// caller's map, would let one mutation rewrite the permissions of other rows.
		newSubService := func(subCtrl *gomock.Controller) (*WorkspaceService, *mocks.MockWorkspaceRepository, *mocks.MockUserRepository, *mocks.MockAuthService) {
			mockRepo := mocks.NewMockWorkspaceRepository(subCtrl)
			mockUserRepo := mocks.NewMockUserRepository(subCtrl)
			mockAuthSvc := mocks.NewMockAuthService(subCtrl)

			return NewWorkspaceService(
				mockRepo,
				mockUserRepo,
				mocks.NewMockTaskRepository(ctrl),
				mockLogger,
				mockUserSvc,
				mockAuthSvc,
				mockMailer,
				cfg,
				mockContactService,
				mockListService,
				mockContactListService,
				mockTemplateService,
				mockWebhookRegService,
				"secret_key",
				&SupabaseService{},
				&DNSVerificationService{},
				&BlogService{},
			), mockRepo, mockUserRepo, mockAuthSvc
		}

		t.Run("not the global full permissions map", func(t *testing.T) {
			subCtrl := gomock.NewController(t)
			defer subCtrl.Finish()

			subService, mockRepo, mockUserRepo, mockAuthSvc := newSubService(subCtrl)

			mockAuthSvc.EXPECT().
				AuthenticateUserForWorkspace(ctx, workspaceID).
				Return(ctx, &domain.User{ID: userID}, &domain.UserWorkspace{
					UserID:      userID,
					WorkspaceID: workspaceID,
					Role:        "owner",
				}, nil)
			mockUserRepo.EXPECT().CreateUser(ctx, gomock.Any()).Return(nil)

			var stored domain.UserPermissions
			mockRepo.EXPECT().
				AddUserToWorkspace(ctx, gomock.Any()).
				DoAndReturn(func(_ context.Context, userWorkspace *domain.UserWorkspace) error {
					stored = userWorkspace.Permissions
					return nil
				})
			mockAuthSvc.EXPECT().GenerateAPIAuthToken(gomock.Any()).Return(expectedToken)

			_, _, err := subService.CreateAPIKey(ctx, workspaceID, emailPrefix, nil)
			require.NoError(t, err)

			stored[domain.PermissionResourceContacts] = domain.ResourcePermissions{}
			assert.Equal(t,
				domain.ResourcePermissions{Read: true, Write: true},
				domain.FullPermissions[domain.PermissionResourceContacts])
		})

		t.Run("not the caller's map", func(t *testing.T) {
			subCtrl := gomock.NewController(t)
			defer subCtrl.Finish()

			subService, mockRepo, mockUserRepo, mockAuthSvc := newSubService(subCtrl)

			mockAuthSvc.EXPECT().
				AuthenticateUserForWorkspace(ctx, workspaceID).
				Return(ctx, &domain.User{ID: userID}, &domain.UserWorkspace{
					UserID:      userID,
					WorkspaceID: workspaceID,
					Role:        "owner",
				}, nil)
			mockUserRepo.EXPECT().CreateUser(ctx, gomock.Any()).Return(nil)

			var stored domain.UserPermissions
			mockRepo.EXPECT().
				AddUserToWorkspace(ctx, gomock.Any()).
				DoAndReturn(func(_ context.Context, userWorkspace *domain.UserWorkspace) error {
					stored = userWorkspace.Permissions
					return nil
				})
			mockAuthSvc.EXPECT().GenerateAPIAuthToken(gomock.Any()).Return(expectedToken)

			callerPermissions := domain.UserPermissions{
				domain.PermissionResourceTransactional: {Write: true},
			}
			_, _, err := subService.CreateAPIKey(ctx, workspaceID, emailPrefix, callerPermissions)
			require.NoError(t, err)

			callerPermissions[domain.PermissionResourceContacts] = domain.ResourcePermissions{Read: true, Write: true}
			assert.NotContains(t, stored, domain.PermissionResourceContacts)
		})
	})

	t.Run("root_email_prefix_is_rejected", func(t *testing.T) {
		subCtrl := gomock.NewController(t)
		defer subCtrl.Finish()

		mockRepo := mocks.NewMockWorkspaceRepository(subCtrl)
		mockUserRepo := mocks.NewMockUserRepository(subCtrl)
		mockAuthSvc := mocks.NewMockAuthService(subCtrl)

		// A prefix that lands on a configured ROOT_EMAIL would mint a platform-admin key.
		rootCfg := &config.Config{APIEndpoint: "https://api.example.com/v1", RootEmail: "root@api.example.com"}

		subService := NewWorkspaceService(
			mockRepo,
			mockUserRepo,
			mocks.NewMockTaskRepository(ctrl),
			mockLogger,
			mockUserSvc,
			mockAuthSvc,
			mockMailer,
			rootCfg,
			mockContactService,
			mockListService,
			mockContactListService,
			mockTemplateService,
			mockWebhookRegService,
			"secret_key",
			&SupabaseService{},
			&DNSVerificationService{},
			&BlogService{},
		)

		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(ctx, workspaceID).
			Return(ctx, &domain.User{ID: userID}, &domain.UserWorkspace{
				UserID:      userID,
				WorkspaceID: workspaceID,
				Role:        "owner",
			}, nil)

		token, email, err := subService.CreateAPIKey(ctx, workspaceID, "root", nil)
		require.Error(t, err)
		assert.Equal(t, "", token)
		assert.Equal(t, "", email)
		assert.IsType(t, &domain.ErrUnauthorized{}, err)
		assert.Contains(t, err.Error(), "platform admin")
	})

	t.Run("non_member_requester", func(t *testing.T) {
		subCtrl := gomock.NewController(t)
		defer subCtrl.Finish()

		mockRepo := mocks.NewMockWorkspaceRepository(subCtrl)
		mockUserRepo := mocks.NewMockUserRepository(subCtrl)
		mockAuthSvc := mocks.NewMockAuthService(subCtrl)

		subService := NewWorkspaceService(
			mockRepo,
			mockUserRepo,
			mocks.NewMockTaskRepository(ctrl),
			mockLogger,
			mockUserSvc,
			mockAuthSvc,
			mockMailer,
			cfg,
			mockContactService,
			mockListService,
			mockContactListService,
			mockTemplateService,
			mockWebhookRegService,
			"secret_key",
			&SupabaseService{},
			&DNSVerificationService{},
			&BlogService{},
		)

		// The sentinel is a plain error, so it has to be translated for the handler to
		// answer 403 instead of 500.
		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(ctx, workspaceID).
			Return(ctx, nil, nil, domain.ErrUserNotInWorkspace)

		token, email, err := subService.CreateAPIKey(ctx, workspaceID, emailPrefix, nil)
		require.Error(t, err)
		assert.Equal(t, "", token)
		assert.Equal(t, "", email)
		var unauthorizedErr *domain.ErrUnauthorized
		assert.True(t, errors.As(err, &unauthorizedErr))
	})

	t.Run("duplicate_api_key_email", func(t *testing.T) {
		subCtrl := gomock.NewController(t)
		defer subCtrl.Finish()

		mockRepo := mocks.NewMockWorkspaceRepository(subCtrl)
		mockUserRepo := mocks.NewMockUserRepository(subCtrl)
		mockAuthSvc := mocks.NewMockAuthService(subCtrl)

		subService := NewWorkspaceService(
			mockRepo,
			mockUserRepo,
			mocks.NewMockTaskRepository(ctrl),
			mockLogger,
			mockUserSvc,
			mockAuthSvc,
			mockMailer,
			cfg,
			mockContactService,
			mockListService,
			mockContactListService,
			mockTemplateService,
			mockWebhookRegService,
			"secret_key",
			&SupabaseService{},
			&DNSVerificationService{},
			&BlogService{},
		)

		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(ctx, workspaceID).
			Return(ctx, &domain.User{ID: userID}, &domain.UserWorkspace{
				UserID:      userID,
				WorkspaceID: workspaceID,
				Role:        "owner",
			}, nil)

		mockUserRepo.EXPECT().
			CreateUser(ctx, gomock.Any()).
			Return(&domain.ErrUserExists{Message: "user already exists"})

		// The typed error must survive the wrap: the handler maps it to 409.
		token, email, err := subService.CreateAPIKey(ctx, workspaceID, emailPrefix, nil)
		require.Error(t, err)
		assert.Equal(t, "", token)
		assert.Equal(t, "", email)
		var userExistsErr *domain.ErrUserExists
		assert.True(t, errors.As(err, &userExistsErr))
	})

	t.Run("unknown_permission_resource", func(t *testing.T) {
		subCtrl := gomock.NewController(t)
		defer subCtrl.Finish()

		mockRepo := mocks.NewMockWorkspaceRepository(subCtrl)
		mockUserRepo := mocks.NewMockUserRepository(subCtrl)
		mockAuthSvc := mocks.NewMockAuthService(subCtrl)

		subService := NewWorkspaceService(
			mockRepo,
			mockUserRepo,
			mocks.NewMockTaskRepository(ctrl),
			mockLogger,
			mockUserSvc,
			mockAuthSvc,
			mockMailer,
			cfg,
			mockContactService,
			mockListService,
			mockContactListService,
			mockTemplateService,
			mockWebhookRegService,
			"secret_key",
			&SupabaseService{},
			&DNSVerificationService{},
			&BlogService{},
		)

		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(ctx, workspaceID).
			Return(ctx, &domain.User{ID: userID}, &domain.UserWorkspace{
				UserID:      userID,
				WorkspaceID: workspaceID,
				Role:        "owner",
			}, nil)

		// No repo expectations: an unknown resource is rejected before anything is created.
		token, email, err := subService.CreateAPIKey(ctx, workspaceID, emailPrefix, domain.UserPermissions{
			"not_a_resource": {Read: true},
		})
		require.Error(t, err)
		assert.Equal(t, "", token)
		assert.Equal(t, "", email)
		assert.Contains(t, err.Error(), "unknown permission resource")
	})
}

// TestWorkspaceService_GetWorkspaceMembersWithEmail_PlatformAdmins verifies that
// ROOT_EMAIL platform admins are surfaced as virtual owner entries in the members
// list when they are not already real members, are skipped when already present,
// and are skipped when their user account is not provisioned yet.
func TestWorkspaceService_GetWorkspaceMembersWithEmail_PlatformAdmins(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockUserRepo := mocks.NewMockUserRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockUserSvc := mocks.NewMockUserServiceInterface(ctrl)
	mockAuthSvc := mocks.NewMockAuthService(ctrl)
	mockMailer := pkgmocks.NewMockMailer(ctrl)
	cfg := &config.Config{RootEmail: "root@example.com"}

	service := NewWorkspaceService(
		mockRepo, mockUserRepo, mocks.NewMockTaskRepository(ctrl), mockLogger,
		mockUserSvc, mockAuthSvc, mockMailer, cfg,
		mocks.NewMockContactService(ctrl), mocks.NewMockListService(ctrl),
		mocks.NewMockContactListService(ctrl), mocks.NewMockTemplateService(ctrl),
		mocks.NewMockWebhookRegistrationService(ctrl), "secret_key",
		&SupabaseService{}, &DNSVerificationService{}, &BlogService{},
	)

	ctx := context.Background()
	workspaceID := "ws1"
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
	mockRepo.EXPECT().GetWorkspaceInvitations(ctx, workspaceID).Return([]*domain.WorkspaceInvitation{}, nil).AnyTimes()
	mockAuthSvc.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).
		Return(ctx, &domain.User{ID: "req"}, &domain.UserWorkspace{Role: "owner"}, nil).AnyTimes()

	t.Run("root not already a member is appended as a virtual owner", func(t *testing.T) {
		realMembers := []*domain.UserWorkspaceWithEmail{
			{UserWorkspace: domain.UserWorkspace{UserID: "u1", WorkspaceID: workspaceID, Role: "member"}, Email: "member@example.com"},
		}
		mockRepo.EXPECT().GetWorkspaceUsersWithEmail(ctx, workspaceID).Return(realMembers, nil)
		mockUserSvc.EXPECT().GetUserByEmail(ctx, "root@example.com").
			Return(&domain.User{ID: "root-id", Email: "root@example.com"}, nil)

		members, err := service.GetWorkspaceMembersWithEmail(ctx, workspaceID)
		require.NoError(t, err)
		require.Len(t, members, 2)
		virtual := members[len(members)-1]
		assert.Equal(t, "root@example.com", virtual.Email)
		assert.Equal(t, "owner", virtual.Role)
		assert.Equal(t, "root-id", virtual.UserID)
		assert.Equal(t, domain.FullPermissions, virtual.Permissions)
	})

	t.Run("root already a member is not duplicated", func(t *testing.T) {
		realMembers := []*domain.UserWorkspaceWithEmail{
			{UserWorkspace: domain.UserWorkspace{UserID: "root-id", WorkspaceID: workspaceID, Role: "owner"}, Email: "root@example.com"},
		}
		mockRepo.EXPECT().GetWorkspaceUsersWithEmail(ctx, workspaceID).Return(realMembers, nil)
		// No GetUserByEmail expected — root is already present.

		members, err := service.GetWorkspaceMembersWithEmail(ctx, workspaceID)
		require.NoError(t, err)
		require.Len(t, members, 1)
	})

	t.Run("root with no provisioned account is skipped", func(t *testing.T) {
		realMembers := []*domain.UserWorkspaceWithEmail{
			{UserWorkspace: domain.UserWorkspace{UserID: "u1", WorkspaceID: workspaceID, Role: "member"}, Email: "member@example.com"},
		}
		mockRepo.EXPECT().GetWorkspaceUsersWithEmail(ctx, workspaceID).Return(realMembers, nil)
		mockUserSvc.EXPECT().GetUserByEmail(ctx, "root@example.com").
			Return(nil, &domain.ErrUserNotFound{Message: "not found"})

		members, err := service.GetWorkspaceMembersWithEmail(ctx, workspaceID)
		require.NoError(t, err)
		require.Len(t, members, 1)
	})
}

// TestWorkspaceService_InviteMember_RejectsRootEmail verifies the defense-in-depth guard:
// a configured ROOT_EMAIL cannot be invited (it already has god-mode access, and rejecting it
// closes a theoretical path to provisioning a root identity via invitation acceptance).
func TestWorkspaceService_InviteMember_RejectsRootEmail(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockAuthSvc := mocks.NewMockAuthService(ctrl)
	cfg := &config.Config{RootEmail: "root@example.com"}

	service := NewWorkspaceService(
		mockRepo, mocks.NewMockUserRepository(ctrl), mocks.NewMockTaskRepository(ctrl), mockLogger,
		mocks.NewMockUserServiceInterface(ctrl), mockAuthSvc, pkgmocks.NewMockMailer(ctrl), cfg,
		mocks.NewMockContactService(ctrl), mocks.NewMockListService(ctrl),
		mocks.NewMockContactListService(ctrl), mocks.NewMockTemplateService(ctrl),
		mocks.NewMockWebhookRegistrationService(ctrl), "secret_key",
		&SupabaseService{}, &DNSVerificationService{}, &BlogService{},
	)

	ctx := context.Background()
	workspaceID := "ws1"
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

	// Requester passes the workspace auth gate as an owner.
	mockAuthSvc.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).
		Return(ctx, &domain.User{ID: "req"}, &domain.UserWorkspace{Role: "owner"}, nil)

	invitation, token, err := service.InviteMember(ctx, workspaceID, "root@example.com", domain.UserPermissions{})
	require.Error(t, err)
	assert.Nil(t, invitation)
	assert.Empty(t, token)
	assert.Contains(t, err.Error(), "platform admin")
}
