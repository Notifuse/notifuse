package service

import (
	"context"
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
			Return(ctx, &domain.User{ID: requesterID}, nil, nil)

		mockRepo.EXPECT().
			GetUserWorkspace(ctx, requesterID, workspaceID).
			Return(&domain.UserWorkspace{
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
		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(ctx, workspaceID).
			Return(ctx, &domain.User{ID: requesterID}, nil, nil)

		mockRepo.EXPECT().
			GetUserWorkspace(ctx, requesterID, workspaceID).
			Return(nil, fmt.Errorf("user workspace not found"))

		err := service.AddUserToWorkspace(ctx, workspaceID, userID, "member", domain.UserPermissions{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "user workspace not found")
	})

	t.Run("requester_not_an_owner", func(t *testing.T) {
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

		err := service.AddUserToWorkspace(ctx, workspaceID, userID, "member", domain.UserPermissions{})
		require.Error(t, err)
		assert.Equal(t, "user is not an owner of the workspace", err.Error())
	})

	t.Run("invalid_role", func(t *testing.T) {
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
			Return(ctx, &domain.User{ID: requesterID}, nil, nil)

		mockRepo.EXPECT().
			GetUserWorkspace(ctx, requesterID, workspaceID).
			Return(&domain.UserWorkspace{
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
		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(ctx, workspaceID).
			Return(ctx, &domain.User{ID: requesterID}, nil, nil)

		mockRepo.EXPECT().
			GetUserWorkspace(ctx, requesterID, workspaceID).
			Return(nil, fmt.Errorf("user is not a member of the workspace"))

		err := service.RemoveUserFromWorkspace(ctx, workspaceID, userID)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "user is not a member of the workspace")
	})

	t.Run("requester_not_an_owner", func(t *testing.T) {
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

		err := service.RemoveUserFromWorkspace(ctx, workspaceID, userID)
		require.Error(t, err)
		assert.Equal(t, "user is not an owner of the workspace", err.Error())
	})

	t.Run("target_user_not_found", func(t *testing.T) {
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
			RemoveUserFromWorkspace(ctx, userID, workspaceID).
			Return(fmt.Errorf("user is not a member of the workspace"))

		err := service.RemoveUserFromWorkspace(ctx, workspaceID, userID)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "user is not a member of the workspace")
	})

	t.Run("cannot_remove_owner", func(t *testing.T) {
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

		err := service.RemoveUserFromWorkspace(ctx, workspaceID, requesterID)
		require.Error(t, err)
		assert.Equal(t, "cannot remove yourself from the workspace", err.Error())
	})

	t.Run("cannot remove self", func(t *testing.T) {
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
			Return(ctx, expectedUser, nil, nil)

		mockRepo.EXPECT().
			GetUserWorkspace(ctx, userID, workspaceID).
			Return(&domain.UserWorkspace{
				UserID:      userID,
				WorkspaceID: workspaceID,
				Role:        "owner",
			}, nil)

		mockRepo.EXPECT().
			GetWorkspaceUsersWithEmail(ctx, workspaceID).
			Return(expectedMembers, nil)

		members, err := service.GetWorkspaceMembersWithEmail(ctx, workspaceID)
		require.NoError(t, err)
		assert.Equal(t, expectedMembers, members)
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
			Return(ctx, expectedUser, nil, nil)

		mockRepo.EXPECT().
			GetUserWorkspace(ctx, userID, workspaceID).
			Return(&domain.UserWorkspace{
				UserID:      userID,
				WorkspaceID: workspaceID,
				Role:        "owner",
			}, nil)

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
			Return(ctx, &domain.User{ID: inviterID}, nil, nil)

		mockRepo.EXPECT().
			GetByID(ctx, workspaceID).
			Return(&domain.Workspace{
				ID:   workspaceID,
				Name: "Test Workspace",
			}, nil)

		mockRepo.EXPECT().
			IsUserWorkspaceMember(ctx, inviterID, workspaceID).
			Return(true, nil)

		mockUserSvc.EXPECT().
			GetUserByID(ctx, inviterID).
			Return(&domain.User{
				ID:    inviterID,
				Name:  "Test Inviter",
				Email: "inviter@example.com",
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
			Return(ctx, &domain.User{ID: inviterID}, nil, nil)

		mockRepo.EXPECT().
			GetByID(ctx, workspaceID).
			Return(&domain.Workspace{
				ID:   workspaceID,
				Name: "Test Workspace",
			}, nil)

		mockRepo.EXPECT().
			IsUserWorkspaceMember(ctx, inviterID, workspaceID).
			Return(true, nil)

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
			Return(ctx, &domain.User{ID: inviterID}, nil, nil)

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
			Return(ctx, &domain.User{ID: inviterID}, nil, nil)

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
		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(ctx, workspaceID).
			Return(ctx, &domain.User{ID: inviterID}, nil, nil)

		mockRepo.EXPECT().
			GetByID(ctx, workspaceID).
			Return(&domain.Workspace{
				ID:   workspaceID,
				Name: "Test Workspace",
			}, nil)

		mockRepo.EXPECT().
			IsUserWorkspaceMember(ctx, inviterID, workspaceID).
			Return(false, nil)

		invitation, token, err := service.InviteMember(ctx, workspaceID, email, domain.UserPermissions{})
		require.Error(t, err)
		assert.Nil(t, invitation)
		assert.Empty(t, token)
		assert.Contains(t, err.Error(), "inviter is not a member of the workspace")
	})

	t.Run("user already a member", func(t *testing.T) {
		existingUser := &domain.User{
			ID:    "existing-user",
			Email: email,
		}

		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(ctx, workspaceID).
			Return(ctx, &domain.User{ID: inviterID}, nil, nil)

		mockRepo.EXPECT().
			GetByID(ctx, workspaceID).
			Return(&domain.Workspace{
				ID:   workspaceID,
				Name: "Test Workspace",
			}, nil)

		mockRepo.EXPECT().
			IsUserWorkspaceMember(ctx, inviterID, workspaceID).
			Return(true, nil)

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
			Return(ctx, &domain.User{ID: userID}, nil, nil)

		mockRepo.EXPECT().
			GetUserWorkspace(ctx, userID, workspaceID).
			Return(&domain.UserWorkspace{
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
				return nil
			})

		// Expect generating a token
		mockAuthSvc.EXPECT().
			GenerateAPIAuthToken(gomock.Any()).
			Return(expectedToken)

		token, email, err := subService.CreateAPIKey(ctx, workspaceID, emailPrefix)
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

		token, email, err := subService.CreateAPIKey(ctx, workspaceID, emailPrefix)
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
			Return(ctx, &domain.User{ID: userID}, nil, nil)

		mockRepo.EXPECT().
			GetUserWorkspace(ctx, userID, workspaceID).
			Return(&domain.UserWorkspace{
				UserID:      userID,
				WorkspaceID: workspaceID,
				Role:        "member", // Not an owner
			}, nil)

		token, email, err := subService.CreateAPIKey(ctx, workspaceID, emailPrefix)
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
			Return(ctx, &domain.User{ID: userID}, nil, nil)

		mockRepo.EXPECT().
			GetUserWorkspace(ctx, userID, workspaceID).
			Return(&domain.UserWorkspace{
				UserID:      userID,
				WorkspaceID: workspaceID,
				Role:        "owner",
			}, nil)

		// The API auth token should not be generated if user creation fails
		mockUserRepo.EXPECT().
			CreateUser(ctx, gomock.Any()).
			Return(fmt.Errorf("user creation failed"))

		token, email, err := subService.CreateAPIKey(ctx, workspaceID, emailPrefix)
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
			Return(ctx, &domain.User{ID: userID}, nil, nil)

		mockRepo.EXPECT().
			GetUserWorkspace(ctx, userID, workspaceID).
			Return(&domain.UserWorkspace{
				UserID:      userID,
				WorkspaceID: workspaceID,
				Role:        "owner",
			}, nil)

		mockUserRepo.EXPECT().
			CreateUser(ctx, gomock.Any()).
			Return(nil)

		mockRepo.EXPECT().
			AddUserToWorkspace(ctx, gomock.Any()).
			Return(fmt.Errorf("add to workspace failed"))

		token, email, err := subService.CreateAPIKey(ctx, workspaceID, emailPrefix)
		require.Error(t, err)
		assert.Equal(t, "", token)
		assert.Equal(t, "", email)
		assert.Equal(t, "add to workspace failed", err.Error())
	})
}
