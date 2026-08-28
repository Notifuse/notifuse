package service

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
	"github.com/Notifuse/notifuse/pkg/notifuse_mjml"
)

func TestWorkspaceService_ListWorkspaces(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockUserRepo := mocks.NewMockUserRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockUserService := mocks.NewMockUserServiceInterface(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)
	mockMailer := pkgmocks.NewMockMailer(ctrl)
	mockConfig := &config.Config{RootEmail: "test@example.com"}
	mockContactService := mocks.NewMockContactService(ctrl)
	mockListService := mocks.NewMockListService(ctrl)
	mockContactListService := mocks.NewMockContactListService(ctrl)
	mockTemplateService := mocks.NewMockTemplateService(ctrl)
	mockWebhookRegService := mocks.NewMockWebhookRegistrationService(ctrl)

	service := NewWorkspaceService(
		mockRepo,
		mockUserRepo,
		mocks.NewMockTaskRepository(ctrl),
		mockLogger,
		mockUserService,
		mockAuthService,
		mockMailer,
		mockConfig,
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
	user := &domain.User{ID: "test-user"}

	t.Run("successful list with workspaces", func(t *testing.T) {
		mockAuthService.EXPECT().AuthenticateUserFromContext(ctx).Return(user, nil)
		mockRepo.EXPECT().GetUserWorkspaces(ctx, user.ID).Return([]*domain.UserWorkspace{
			{WorkspaceID: "1"},
			{WorkspaceID: "2"},
		}, nil)
		mockRepo.EXPECT().GetByID(ctx, "1").Return(&domain.Workspace{ID: "1"}, nil)
		mockRepo.EXPECT().GetByID(ctx, "2").Return(&domain.Workspace{ID: "2"}, nil)

		workspaces, err := service.ListWorkspaces(ctx)
		assert.NoError(t, err)
		assert.Len(t, workspaces, 2)
	})

	t.Run("authentication error", func(t *testing.T) {
		mockAuthService.EXPECT().AuthenticateUserFromContext(ctx).Return(nil, errors.New("auth error"))

		workspaces, err := service.ListWorkspaces(ctx)
		assert.Error(t, err)
		assert.Nil(t, workspaces)
	})

	t.Run("get user workspaces error", func(t *testing.T) {
		mockAuthService.EXPECT().AuthenticateUserFromContext(ctx).Return(user, nil)
		mockRepo.EXPECT().GetUserWorkspaces(ctx, user.ID).Return(nil, errors.New("repo error"))
		mockLogger.EXPECT().WithField("user_id", user.ID).Return(mockLogger)
		mockLogger.EXPECT().WithField("error", "repo error").Return(mockLogger)
		mockLogger.EXPECT().Error("Failed to get user workspaces")

		workspaces, err := service.ListWorkspaces(ctx)
		assert.Error(t, err)
		assert.Nil(t, workspaces)
	})

	t.Run("get workspace by ID error", func(t *testing.T) {
		mockAuthService.EXPECT().AuthenticateUserFromContext(ctx).Return(user, nil)
		mockRepo.EXPECT().GetUserWorkspaces(ctx, user.ID).Return([]*domain.UserWorkspace{
			{WorkspaceID: "1"},
		}, nil)
		mockRepo.EXPECT().GetByID(ctx, "1").Return(nil, errors.New("repo error"))
		mockLogger.EXPECT().WithField("workspace_id", "1").Return(mockLogger)
		mockLogger.EXPECT().WithField("user_id", user.ID).Return(mockLogger)
		mockLogger.EXPECT().WithField("error", "repo error").Return(mockLogger)
		mockLogger.EXPECT().Error("Failed to get workspace by ID")

		workspaces, err := service.ListWorkspaces(ctx)
		assert.Error(t, err)
		assert.Nil(t, workspaces)
	})

	t.Run("no workspaces", func(t *testing.T) {
		mockAuthService.EXPECT().AuthenticateUserFromContext(ctx).Return(user, nil)
		mockRepo.EXPECT().GetUserWorkspaces(ctx, user.ID).Return([]*domain.UserWorkspace{}, nil)

		workspaces, err := service.ListWorkspaces(ctx)
		assert.NoError(t, err)
		assert.Empty(t, workspaces)
	})

	t.Run("platform admin lists every workspace via repo.List", func(t *testing.T) {
		// A ROOT_EMAIL user bypasses the membership scan and gets all workspaces.
		rootUser := &domain.User{ID: "root-user", Email: "test@example.com"}
		mockAuthService.EXPECT().AuthenticateUserFromContext(ctx).Return(rootUser, nil)
		mockRepo.EXPECT().List(ctx).Return([]*domain.Workspace{{ID: "1"}, {ID: "2"}, {ID: "3"}}, nil)

		workspaces, err := service.ListWorkspaces(ctx)
		assert.NoError(t, err)
		assert.Len(t, workspaces, 3)
	})
}

func TestWorkspaceService_GetWorkspace(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockUserRepo := mocks.NewMockUserRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockUserService := mocks.NewMockUserServiceInterface(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)
	mockMailer := pkgmocks.NewMockMailer(ctrl)
	mockConfig := &config.Config{Environment: "development"}
	mockContactService := mocks.NewMockContactService(ctrl)
	mockListService := mocks.NewMockListService(ctrl)
	mockContactListService := mocks.NewMockContactListService(ctrl)
	mockTemplateService := mocks.NewMockTemplateService(ctrl)
	mockWebhookRegService := mocks.NewMockWebhookRegistrationService(ctrl)

	service := NewWorkspaceService(
		mockRepo,
		mockUserRepo,
		mocks.NewMockTaskRepository(ctrl),
		mockLogger,
		mockUserService,
		mockAuthService,
		mockMailer,
		mockConfig,
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
	workspaceID := "testworkspace"
	userID := "testuser"

	t.Run("successful get", func(t *testing.T) {
		expectedUser := &domain.User{
			ID: userID,
		}

		expectedWorkspace := &domain.Workspace{
			ID:   workspaceID,
			Name: "Test Workspace",
		}

		// GetWorkspace relies on AuthenticateUserForWorkspace for access; it no longer
		// re-reads GetUserWorkspace (platform admins pass via the override).
		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).
			Return(ctx, expectedUser, &domain.UserWorkspace{UserID: userID, WorkspaceID: workspaceID, Role: "owner"}, nil)
		mockRepo.EXPECT().GetByID(ctx, workspaceID).Return(expectedWorkspace, nil)

		workspace, err := service.GetWorkspace(ctx, workspaceID)
		require.NoError(t, err)
		assert.Equal(t, expectedWorkspace, workspace)
	})

	t.Run("member with workspace read permission", func(t *testing.T) {
		expectedWorkspace := &domain.Workspace{ID: workspaceID, Name: "Test Workspace"}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).
			Return(ctx, &domain.User{ID: userID}, &domain.UserWorkspace{
				UserID:      userID,
				WorkspaceID: workspaceID,
				Role:        "member",
				Permissions: domain.UserPermissions{
					domain.PermissionResourceWorkspace: {Read: true},
				},
			}, nil)
		mockRepo.EXPECT().GetByID(ctx, workspaceID).Return(expectedWorkspace, nil)

		workspace, err := service.GetWorkspace(ctx, workspaceID)
		require.NoError(t, err)
		assert.Equal(t, expectedWorkspace, workspace)
	})

	t.Run("member with workspace write permission only", func(t *testing.T) {
		// The console treats access as read || write, so a write-only member reaches
		// Settings and must be able to load the workspace it is allowed to edit.
		expectedWorkspace := &domain.Workspace{ID: workspaceID, Name: "Test Workspace"}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).
			Return(ctx, &domain.User{ID: userID}, &domain.UserWorkspace{
				UserID:      userID,
				WorkspaceID: workspaceID,
				Role:        "member",
				Permissions: domain.UserPermissions{
					domain.PermissionResourceWorkspace: {Write: true},
				},
			}, nil)
		mockRepo.EXPECT().GetByID(ctx, workspaceID).Return(expectedWorkspace, nil)

		workspace, err := service.GetWorkspace(ctx, workspaceID)
		require.NoError(t, err)
		assert.Equal(t, expectedWorkspace, workspace)
	})

	t.Run("member without workspace permission is denied", func(t *testing.T) {
		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).
			Return(ctx, &domain.User{ID: userID}, &domain.UserWorkspace{
				UserID:      userID,
				WorkspaceID: workspaceID,
				Role:        "member",
				Permissions: domain.UserPermissions{
					domain.PermissionResourceContacts: {Read: true, Write: true},
				},
			}, nil)

		workspace, err := service.GetWorkspace(ctx, workspaceID)
		require.Error(t, err)
		assert.Nil(t, workspace)
		var permErr *domain.PermissionError
		require.True(t, errors.As(err, &permErr))
		assert.Equal(t, domain.PermissionResourceWorkspace, permErr.Resource)
	})

	t.Run("error authenticating user", func(t *testing.T) {
		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, nil, nil, assert.AnError)

		workspace, err := service.GetWorkspace(ctx, workspaceID)
		require.Error(t, err)
		assert.Nil(t, workspace)
		assert.ErrorIs(t, err, assert.AnError)
	})

	t.Run("error getting workspace by ID", func(t *testing.T) {
		expectedUser := &domain.User{
			ID: userID,
		}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).
			Return(ctx, expectedUser, &domain.UserWorkspace{UserID: userID, WorkspaceID: workspaceID, Role: "owner"}, nil)
		mockRepo.EXPECT().GetByID(ctx, workspaceID).Return(nil, assert.AnError)

		workspace, err := service.GetWorkspace(ctx, workspaceID)
		require.Error(t, err)
		assert.Nil(t, workspace)
		assert.Equal(t, assert.AnError, err)
	})

	t.Run("system call bypasses authentication", func(t *testing.T) {
		expectedWorkspace := &domain.Workspace{
			ID:   workspaceID,
			Name: "Test Workspace",
		}

		// Create a system context that should bypass authentication
		systemCtx := context.WithValue(ctx, domain.SystemCallKey, true)

		// No auth service call expected since this is a system call
		mockRepo.EXPECT().GetByID(systemCtx, workspaceID).Return(expectedWorkspace, nil)

		workspace, err := service.GetWorkspace(systemCtx, workspaceID)
		require.NoError(t, err)
		assert.Equal(t, expectedWorkspace, workspace)
	})
}

func TestWorkspaceService_CreateWorkspace(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockUserRepo := mocks.NewMockUserRepository(ctrl)
	mockTaskRepo := mocks.NewMockTaskRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockUserService := mocks.NewMockUserServiceInterface(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)
	mockMailer := pkgmocks.NewMockMailer(ctrl)
	mockConfig := &config.Config{RootEmail: "test@example.com"}
	mockContactService := mocks.NewMockContactService(ctrl)
	mockListService := mocks.NewMockListService(ctrl)
	mockContactListService := mocks.NewMockContactListService(ctrl)
	mockTemplateService := mocks.NewMockTemplateService(ctrl)
	mockWebhookRegService := mocks.NewMockWebhookRegistrationService(ctrl)

	service := NewWorkspaceService(
		mockRepo,
		mockUserRepo,
		mockTaskRepo,
		mockLogger,
		mockUserService,
		mockAuthService,
		mockMailer,
		mockConfig,
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
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()

	ctx := context.Background()
	workspaceID := "testworkspace"

	t.Run("successful creation", func(t *testing.T) {
		expectedUser := &domain.User{
			ID:    "testowner",
			Email: "test@example.com",
			Name:  "Test User",
		}

		mockAuthService.EXPECT().AuthenticateUserFromContext(ctx).Return(expectedUser, nil)
		mockRepo.EXPECT().GetByID(ctx, workspaceID).Return(nil, nil)
		mockRepo.EXPECT().Create(ctx, gomock.Any()).DoAndReturn(func(ctx context.Context, workspace *domain.Workspace) error {
			// Instead of expecting an exact value, verify it's not empty and has expected format
			assert.NotEmpty(t, workspace.Settings.SecretKey, "Secret key should not be empty")
			assert.Equal(t, 64, len(workspace.Settings.SecretKey), "Secret key should be 64 hex characters (32 bytes)")
			// Verify hex encoding
			_, err := hex.DecodeString(workspace.Settings.SecretKey)
			assert.NoError(t, err, "Secret key should be valid hex")
			return nil
		})
		mockRepo.EXPECT().AddUserToWorkspace(ctx, gomock.Any()).Return(nil)
		mockUserService.EXPECT().GetUserByID(ctx, expectedUser.ID).Return(expectedUser, nil)
		mockContactService.EXPECT().UpsertContact(ctx, workspaceID, gomock.Any()).Return(domain.UpsertContactOperation{Action: domain.UpsertContactOperationCreate})

		mockListService.EXPECT().CreateList(ctx, workspaceID, gomock.Any()).Return(nil)
		mockListService.EXPECT().SubscribeToLists(ctx, &domain.SubscribeToListsRequest{
			WorkspaceID: workspaceID,
			Contact: domain.Contact{
				Email: expectedUser.Email,
			},
			ListIDs: []string{"test"},
		}, true).Return(nil)

		// Expect EnsureContactSegmentQueueProcessingTask to be called
		mockTaskRepo.EXPECT().List(ctx, workspaceID, gomock.Any()).Return([]*domain.Task{}, 0, nil).AnyTimes()
		mockTaskRepo.EXPECT().Create(ctx, workspaceID, gomock.Any()).Return(nil).AnyTimes()

		workspace, err := service.CreateWorkspace(ctx, workspaceID, "Test Workspace", "https://example.com", "https://example.com/logo.png", "https://example.com/cover.png", "UTC", domain.FileManagerSettings{
			Endpoint:  "https://s3.amazonaws.com",
			Bucket:    "my-bucket",
			AccessKey: "AKIAIOSFODNN7EXAMPLE",
		}, "en", []string{"en"})
		require.NoError(t, err)
		assert.Equal(t, workspaceID, workspace.ID)
		assert.Equal(t, "Test Workspace", workspace.Name)

		// Verify the structure of settings but don't check the exact SecretKey value
		assert.Equal(t, "https://example.com", workspace.Settings.WebsiteURL)
		assert.Equal(t, "https://example.com/logo.png", workspace.Settings.LogoURL)
		assert.Equal(t, "https://example.com/cover.png", workspace.Settings.CoverURL)
		assert.Equal(t, "UTC", workspace.Settings.Timezone)

		// Verify language defaults
		assert.Equal(t, "en", workspace.Settings.DefaultLanguage)
		assert.Equal(t, []string{"en"}, workspace.Settings.Languages)

		// Verify SecretKey format but not exact value
		assert.NotEmpty(t, workspace.Settings.SecretKey)
		assert.Equal(t, 64, len(workspace.Settings.SecretKey))
		_, err = hex.DecodeString(workspace.Settings.SecretKey)
		assert.NoError(t, err, "Secret key should be valid hex")
	})

	t.Run("validation error", func(t *testing.T) {
		expectedUser := &domain.User{
			ID:    "testowner",
			Email: "test@example.com",
			Name:  "Test User",
		}

		mockAuthService.EXPECT().AuthenticateUserFromContext(ctx).Return(expectedUser, nil)
		// No need to mock GetByID here as the validation fails before that check

		// Invalid timezone
		workspace, err := service.CreateWorkspace(ctx, workspaceID, "Test Workspace", "https://example.com", "https://example.com/logo.png", "https://example.com/cover.png", "INVALID_TIMEZONE", domain.FileManagerSettings{
			Endpoint:  "https://s3.amazonaws.com",
			Bucket:    "my-bucket",
			AccessKey: "AKIAIOSFODNN7EXAMPLE",
		}, "en", []string{"en"})
		require.Error(t, err)
		assert.Nil(t, workspace)
		assert.Contains(t, err.Error(), "invalid timezone: INVALID_TIMEZONE")
	})

	t.Run("repository error", func(t *testing.T) {
		expectedUser := &domain.User{
			ID:    "testowner",
			Email: "test@example.com",
			Name:  "Test User",
		}

		mockAuthService.EXPECT().AuthenticateUserFromContext(ctx).Return(expectedUser, nil)
		mockRepo.EXPECT().GetByID(ctx, workspaceID).Return(nil, nil)
		mockRepo.EXPECT().Create(ctx, gomock.Any()).Return(assert.AnError)

		workspace, err := service.CreateWorkspace(ctx, workspaceID, "Test Workspace", "https://example.com", "https://example.com/logo.png", "https://example.com/cover.png", "UTC", domain.FileManagerSettings{
			Endpoint:  "https://s3.amazonaws.com",
			Bucket:    "my-bucket",
			AccessKey: "AKIAIOSFODNN7EXAMPLE",
		}, "en", []string{"en"})
		require.Error(t, err)
		assert.Nil(t, workspace)
		assert.Equal(t, assert.AnError, err)
	})

	t.Run("add user error", func(t *testing.T) {
		expectedUser := &domain.User{
			ID:    "testowner",
			Email: "test@example.com",
			Name:  "Test User",
		}

		mockAuthService.EXPECT().AuthenticateUserFromContext(ctx).Return(expectedUser, nil)
		mockRepo.EXPECT().GetByID(ctx, workspaceID).Return(nil, nil)
		mockRepo.EXPECT().Create(ctx, gomock.Any()).Return(nil)
		mockRepo.EXPECT().AddUserToWorkspace(ctx, gomock.Any()).Return(assert.AnError)

		workspace, err := service.CreateWorkspace(ctx, workspaceID, "Test Workspace", "https://example.com", "https://example.com/logo.png", "https://example.com/cover.png", "UTC", domain.FileManagerSettings{
			Endpoint:  "https://s3.amazonaws.com",
			Bucket:    "my-bucket",
			AccessKey: "AKIAIOSFODNN7EXAMPLE",
		}, "en", []string{"en"})
		require.Error(t, err)
		assert.Nil(t, workspace)
		assert.Equal(t, assert.AnError, err)
	})

	t.Run("get user error", func(t *testing.T) {
		expectedUser := &domain.User{
			ID:    "testowner",
			Email: "test@example.com",
			Name:  "Test User",
		}

		mockAuthService.EXPECT().AuthenticateUserFromContext(ctx).Return(expectedUser, nil)
		mockRepo.EXPECT().GetByID(ctx, workspaceID).Return(nil, nil)
		mockRepo.EXPECT().Create(ctx, gomock.Any()).Return(nil)
		mockRepo.EXPECT().AddUserToWorkspace(ctx, gomock.Any()).Return(nil)
		mockUserService.EXPECT().GetUserByID(ctx, expectedUser.ID).Return(nil, assert.AnError)

		workspace, err := service.CreateWorkspace(ctx, workspaceID, "Test Workspace", "https://example.com", "https://example.com/logo.png", "https://example.com/cover.png", "UTC", domain.FileManagerSettings{
			Endpoint:  "https://s3.amazonaws.com",
			Bucket:    "my-bucket",
			AccessKey: "AKIAIOSFODNN7EXAMPLE",
		}, "en", []string{"en"})
		require.Error(t, err)
		assert.Nil(t, workspace)
		assert.Equal(t, assert.AnError, err)
	})

	t.Run("upsert contact error", func(t *testing.T) {
		expectedUser := &domain.User{
			ID:    "testowner",
			Email: "test@example.com",
			Name:  "Test User",
		}

		mockAuthService.EXPECT().AuthenticateUserFromContext(ctx).Return(expectedUser, nil)
		mockRepo.EXPECT().GetByID(ctx, workspaceID).Return(nil, nil)
		mockRepo.EXPECT().Create(ctx, gomock.Any()).Return(nil)
		mockRepo.EXPECT().AddUserToWorkspace(ctx, gomock.Any()).Return(nil)
		mockUserService.EXPECT().GetUserByID(ctx, expectedUser.ID).Return(expectedUser, nil)
		mockContactService.EXPECT().UpsertContact(ctx, workspaceID, gomock.Any()).Return(domain.UpsertContactOperation{
			Action: domain.UpsertContactOperationError,
			Error:  "failed to upsert contact",
		})

		workspace, err := service.CreateWorkspace(ctx, workspaceID, "Test Workspace", "https://example.com", "https://example.com/logo.png", "https://example.com/cover.png", "UTC", domain.FileManagerSettings{
			Endpoint:  "https://s3.amazonaws.com",
			Bucket:    "my-bucket",
			AccessKey: "AKIAIOSFODNN7EXAMPLE",
		}, "en", []string{"en"})
		require.Error(t, err)
		assert.Nil(t, workspace)
		assert.Contains(t, err.Error(), "failed to upsert contact")
	})

	t.Run("template creation error still allows workspace creation", func(t *testing.T) {
		expectedUser := &domain.User{
			ID:    "testowner",
			Email: "test@example.com",
			Name:  "Test User",
		}

		mockAuthService.EXPECT().AuthenticateUserFromContext(ctx).Return(expectedUser, nil)
		mockRepo.EXPECT().GetByID(ctx, workspaceID).Return(nil, nil)
		mockRepo.EXPECT().Create(ctx, gomock.Any()).Return(nil)
		mockRepo.EXPECT().AddUserToWorkspace(ctx, gomock.Any()).Return(nil)
		mockUserService.EXPECT().GetUserByID(ctx, expectedUser.ID).Return(expectedUser, nil)
		mockContactService.EXPECT().UpsertContact(ctx, workspaceID, gomock.Any()).Return(domain.UpsertContactOperation{Action: domain.UpsertContactOperationCreate})

		// Simulate template creation error for all four templates
		mockTemplateService.EXPECT().CreateTemplate(ctx, workspaceID, gomock.Any()).Return(errors.New("template creation failed")).AnyTimes()

		mockListService.EXPECT().CreateList(ctx, workspaceID, gomock.Any()).Return(nil)
		mockListService.EXPECT().SubscribeToLists(ctx, &domain.SubscribeToListsRequest{
			WorkspaceID: workspaceID,
			Contact: domain.Contact{
				Email: expectedUser.Email,
			},
			ListIDs: []string{"test"},
		}, true).Return(nil)

		workspace, err := service.CreateWorkspace(ctx, workspaceID, "Test Workspace", "https://example.com", "https://example.com/logo.png", "https://example.com/cover.png", "UTC", domain.FileManagerSettings{
			Endpoint:  "https://s3.amazonaws.com",
			Bucket:    "my-bucket",
			AccessKey: "AKIAIOSFODNN7EXAMPLE",
		}, "en", []string{"en"})

		// Should still succeed despite template error
		require.NoError(t, err)
		assert.Equal(t, workspaceID, workspace.ID)
	})

	t.Run("workspace already exists", func(t *testing.T) {
		expectedUser := &domain.User{
			ID:    "testowner",
			Email: "test@example.com",
			Name:  "Test User",
		}

		existingWorkspace := &domain.Workspace{
			ID:   workspaceID,
			Name: "Test Workspace",
		}

		mockAuthService.EXPECT().AuthenticateUserFromContext(ctx).Return(expectedUser, nil)
		mockRepo.EXPECT().GetByID(ctx, workspaceID).Return(existingWorkspace, nil)

		workspace, err := service.CreateWorkspace(ctx, workspaceID, "Test Workspace", "https://example.com", "https://example.com/logo.png", "https://example.com/cover.png", "UTC", domain.FileManagerSettings{
			Endpoint:  "https://s3.amazonaws.com",
			Bucket:    "my-bucket",
			AccessKey: "AKIAIOSFODNN7EXAMPLE",
		}, "en", []string{"en"})
		require.Error(t, err)
		assert.Nil(t, workspace)
		assert.Contains(t, err.Error(), "workspace already exists")
	})
}

func TestWorkspaceService_CreateWorkspace_MultipleRootEmails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockUserRepo := mocks.NewMockUserRepository(ctrl)
	mockTaskRepo := mocks.NewMockTaskRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockUserService := mocks.NewMockUserServiceInterface(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)
	mockMailer := pkgmocks.NewMockMailer(ctrl)
	// Two configured root users (comma-separated).
	mockConfig := &config.Config{RootEmail: "root1@example.com,root2@example.com"}
	mockContactService := mocks.NewMockContactService(ctrl)
	mockListService := mocks.NewMockListService(ctrl)
	mockContactListService := mocks.NewMockContactListService(ctrl)
	mockTemplateService := mocks.NewMockTemplateService(ctrl)
	mockWebhookRegService := mocks.NewMockWebhookRegistrationService(ctrl)

	service := NewWorkspaceService(
		mockRepo,
		mockUserRepo,
		mockTaskRepo,
		mockLogger,
		mockUserService,
		mockAuthService,
		mockMailer,
		mockConfig,
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

	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()

	ctx := context.Background()

	t.Run("second listed root can create a workspace", func(t *testing.T) {
		workspaceID := "secondroot"
		secondRoot := &domain.User{ID: "root2", Email: "root2@example.com", Name: "Second Root"}

		mockAuthService.EXPECT().AuthenticateUserFromContext(ctx).Return(secondRoot, nil)
		mockRepo.EXPECT().GetByID(ctx, workspaceID).Return(nil, nil)
		mockRepo.EXPECT().Create(ctx, gomock.Any()).Return(nil)
		mockRepo.EXPECT().AddUserToWorkspace(ctx, gomock.Any()).Return(nil)
		mockUserService.EXPECT().GetUserByID(ctx, secondRoot.ID).Return(secondRoot, nil)
		mockContactService.EXPECT().UpsertContact(ctx, workspaceID, gomock.Any()).Return(domain.UpsertContactOperation{Action: domain.UpsertContactOperationCreate})
		mockListService.EXPECT().CreateList(ctx, workspaceID, gomock.Any()).Return(nil)
		mockListService.EXPECT().SubscribeToLists(ctx, gomock.Any(), true).Return(nil)
		mockTaskRepo.EXPECT().List(ctx, workspaceID, gomock.Any()).Return([]*domain.Task{}, 0, nil).AnyTimes()
		mockTaskRepo.EXPECT().Create(ctx, workspaceID, gomock.Any()).Return(nil).AnyTimes()

		workspace, err := service.CreateWorkspace(ctx, workspaceID, "Second Root Workspace", "https://example.com", "https://example.com/logo.png", "https://example.com/cover.png", "UTC", domain.FileManagerSettings{
			Endpoint:  "https://s3.amazonaws.com",
			Bucket:    "my-bucket",
			AccessKey: "AKIAIOSFODNN7EXAMPLE",
		}, "en", []string{"en"})
		require.NoError(t, err)
		assert.Equal(t, workspaceID, workspace.ID)
	})

	t.Run("non-listed user is rejected", func(t *testing.T) {
		outsider := &domain.User{ID: "outsider", Email: "outsider@example.com", Name: "Outsider"}
		mockAuthService.EXPECT().AuthenticateUserFromContext(ctx).Return(outsider, nil)

		workspace, err := service.CreateWorkspace(ctx, "outsiderws", "Outsider Workspace", "https://example.com", "https://example.com/logo.png", "https://example.com/cover.png", "UTC", domain.FileManagerSettings{
			Endpoint:  "https://s3.amazonaws.com",
			Bucket:    "my-bucket",
			AccessKey: "AKIAIOSFODNN7EXAMPLE",
		}, "en", []string{"en"})
		require.Error(t, err)
		assert.Nil(t, workspace)
		var unauthorized *domain.ErrUnauthorized
		assert.ErrorAs(t, err, &unauthorized)
	})

	t.Run("case-sensitive mismatch is rejected", func(t *testing.T) {
		// "Root1@example.com" differs in case from the configured "root1@example.com".
		mixedCase := &domain.User{ID: "mixed", Email: "Root1@example.com", Name: "Mixed Case"}
		mockAuthService.EXPECT().AuthenticateUserFromContext(ctx).Return(mixedCase, nil)

		workspace, err := service.CreateWorkspace(ctx, "mixedws", "Mixed Workspace", "https://example.com", "https://example.com/logo.png", "https://example.com/cover.png", "UTC", domain.FileManagerSettings{
			Endpoint:  "https://s3.amazonaws.com",
			Bucket:    "my-bucket",
			AccessKey: "AKIAIOSFODNN7EXAMPLE",
		}, "en", []string{"en"})
		require.Error(t, err)
		assert.Nil(t, workspace)
		var unauthorized *domain.ErrUnauthorized
		assert.ErrorAs(t, err, &unauthorized)
	})
}

func TestWorkspaceService_CreateWorkspace_CustomLanguageSettings(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockUserRepo := mocks.NewMockUserRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockUserService := mocks.NewMockUserServiceInterface(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)
	mockMailer := pkgmocks.NewMockMailer(ctrl)
	mockConfig := &config.Config{RootEmail: "test@example.com"}
	mockContactService := mocks.NewMockContactService(ctrl)
	mockListService := mocks.NewMockListService(ctrl)
	mockContactListService := mocks.NewMockContactListService(ctrl)
	mockTemplateService := mocks.NewMockTemplateService(ctrl)
	mockWebhookRegService := mocks.NewMockWebhookRegistrationService(ctrl)
	mockTaskRepo := mocks.NewMockTaskRepository(ctrl)

	service := NewWorkspaceService(
		mockRepo,
		mockUserRepo,
		mockTaskRepo,
		mockLogger,
		mockUserService,
		mockAuthService,
		mockMailer,
		mockConfig,
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

	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()

	ctx := context.Background()
	workspaceID := "testworkspace"

	expectedUser := &domain.User{
		ID:    "testowner",
		Email: "test@example.com",
		Name:  "Test User",
	}

	mockAuthService.EXPECT().AuthenticateUserFromContext(ctx).Return(expectedUser, nil)
	mockRepo.EXPECT().GetByID(ctx, workspaceID).Return(nil, nil)
	mockRepo.EXPECT().Create(ctx, gomock.Any()).DoAndReturn(func(ctx context.Context, workspace *domain.Workspace) error {
		assert.Equal(t, "fr", workspace.Settings.DefaultLanguage)
		assert.Equal(t, []string{"fr", "en"}, workspace.Settings.Languages)
		return nil
	})
	mockRepo.EXPECT().AddUserToWorkspace(ctx, gomock.Any()).Return(nil)
	mockUserService.EXPECT().GetUserByID(ctx, expectedUser.ID).Return(expectedUser, nil)
	mockContactService.EXPECT().UpsertContact(ctx, workspaceID, gomock.Any()).Return(domain.UpsertContactOperation{Action: domain.UpsertContactOperationCreate})
	mockListService.EXPECT().CreateList(ctx, workspaceID, gomock.Any()).Return(nil)
	mockListService.EXPECT().SubscribeToLists(ctx, gomock.Any(), true).Return(nil)
	mockTaskRepo.EXPECT().List(ctx, workspaceID, gomock.Any()).Return([]*domain.Task{}, 0, nil).AnyTimes()
	mockTaskRepo.EXPECT().Create(ctx, workspaceID, gomock.Any()).Return(nil).AnyTimes()

	workspace, err := service.CreateWorkspace(ctx, workspaceID, "Test Workspace", "https://example.com", "https://example.com/logo.png", "https://example.com/cover.png", "UTC", domain.FileManagerSettings{
		Endpoint:  "https://s3.amazonaws.com",
		Bucket:    "my-bucket",
		AccessKey: "AKIAIOSFODNN7EXAMPLE",
	}, "fr", []string{"fr", "en"})
	require.NoError(t, err)
	assert.Equal(t, "fr", workspace.Settings.DefaultLanguage)
	assert.Equal(t, []string{"fr", "en"}, workspace.Settings.Languages)
}

func TestWorkspaceService_CreateWorkspace_DefaultLanguageNotInList(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockUserRepo := mocks.NewMockUserRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockUserService := mocks.NewMockUserServiceInterface(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)
	mockMailer := pkgmocks.NewMockMailer(ctrl)
	mockConfig := &config.Config{RootEmail: "test@example.com"}
	mockContactService := mocks.NewMockContactService(ctrl)
	mockListService := mocks.NewMockListService(ctrl)
	mockContactListService := mocks.NewMockContactListService(ctrl)
	mockTemplateService := mocks.NewMockTemplateService(ctrl)
	mockWebhookRegService := mocks.NewMockWebhookRegistrationService(ctrl)
	mockTaskRepo := mocks.NewMockTaskRepository(ctrl)

	service := NewWorkspaceService(
		mockRepo,
		mockUserRepo,
		mockTaskRepo,
		mockLogger,
		mockUserService,
		mockAuthService,
		mockMailer,
		mockConfig,
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

	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()

	ctx := context.Background()
	workspaceID := "testworkspace"

	expectedUser := &domain.User{
		ID:    "testowner",
		Email: "test@example.com",
		Name:  "Test User",
	}

	mockAuthService.EXPECT().AuthenticateUserFromContext(ctx).Return(expectedUser, nil)

	// Pass defaultLanguage="fr" but languages=["en"] — validation should reject this
	_, err := service.CreateWorkspace(ctx, workspaceID, "Test Workspace", "https://example.com", "https://example.com/logo.png", "https://example.com/cover.png", "UTC", domain.FileManagerSettings{
		Endpoint:  "https://s3.amazonaws.com",
		Bucket:    "my-bucket",
		AccessKey: "AKIAIOSFODNN7EXAMPLE",
	}, "fr", []string{"en"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "default language fr must be in the languages list")
}

func TestWorkspaceService_SetCustomFieldLabels(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockUserRepo := mocks.NewMockUserRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockUserService := mocks.NewMockUserServiceInterface(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)
	mockMailer := pkgmocks.NewMockMailer(ctrl)
	mockConfig := &config.Config{RootEmail: "test@example.com"}
	mockContactService := mocks.NewMockContactService(ctrl)
	mockListService := mocks.NewMockListService(ctrl)
	mockContactListService := mocks.NewMockContactListService(ctrl)
	mockTemplateService := mocks.NewMockTemplateService(ctrl)
	mockWebhookRegService := mocks.NewMockWebhookRegistrationService(ctrl)

	service := NewWorkspaceService(
		mockRepo,
		mockUserRepo,
		mocks.NewMockTaskRepository(ctrl),
		mockLogger,
		mockUserService,
		mockAuthService,
		mockMailer,
		mockConfig,
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

	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

	ctx := context.Background()
	workspaceID := "testworkspace"
	userID := "testuser"
	labels := map[string]string{
		"custom_string_1": "Company Name",
		"custom_number_1": "Revenue",
	}

	t.Run("owner can set labels", func(t *testing.T) {
		ownerWorkspace := &domain.UserWorkspace{UserID: userID, WorkspaceID: workspaceID, Role: "owner"}
		existing := &domain.Workspace{ID: workspaceID, Name: "WS", Settings: domain.WorkspaceSettings{Timezone: "UTC"}}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{ID: userID}, ownerWorkspace, nil)
		mockRepo.EXPECT().GetByID(ctx, workspaceID).Return(existing, nil)
		mockRepo.EXPECT().Update(ctx, gomock.Any()).DoAndReturn(func(_ context.Context, ws *domain.Workspace) error {
			assert.Equal(t, labels, ws.Settings.CustomFieldLabels)
			return nil
		})

		err := service.SetCustomFieldLabels(ctx, workspaceID, labels)
		require.NoError(t, err)
	})

	t.Run("member with workspace write can set labels", func(t *testing.T) {
		memberWorkspace := &domain.UserWorkspace{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Role:        "member",
			Permissions: domain.UserPermissions{
				domain.PermissionResourceWorkspace: {Read: true, Write: true},
			},
		}
		existing := &domain.Workspace{ID: workspaceID, Name: "WS", Settings: domain.WorkspaceSettings{Timezone: "UTC"}}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{ID: userID}, memberWorkspace, nil)
		mockRepo.EXPECT().GetByID(ctx, workspaceID).Return(existing, nil)
		mockRepo.EXPECT().Update(ctx, gomock.Any()).DoAndReturn(func(_ context.Context, ws *domain.Workspace) error {
			assert.Equal(t, labels, ws.Settings.CustomFieldLabels)
			return nil
		})

		err := service.SetCustomFieldLabels(ctx, workspaceID, labels)
		require.NoError(t, err)
	})

	t.Run("member with full permissions can set labels", func(t *testing.T) {
		memberWorkspace := &domain.UserWorkspace{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Role:        "member",
			Permissions: domain.FullPermissions,
		}
		existing := &domain.Workspace{ID: workspaceID, Name: "WS", Settings: domain.WorkspaceSettings{Timezone: "UTC"}}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{ID: userID}, memberWorkspace, nil)
		mockRepo.EXPECT().GetByID(ctx, workspaceID).Return(existing, nil)
		mockRepo.EXPECT().Update(ctx, gomock.Any()).Return(nil)

		err := service.SetCustomFieldLabels(ctx, workspaceID, labels)
		require.NoError(t, err)
	})

	t.Run("member with workspace read only is denied", func(t *testing.T) {
		memberWorkspace := &domain.UserWorkspace{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Role:        "member",
			Permissions: domain.UserPermissions{
				domain.PermissionResourceWorkspace: {Read: true, Write: false},
			},
		}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{ID: userID}, memberWorkspace, nil)
		// No GetByID / Update expected.

		err := service.SetCustomFieldLabels(ctx, workspaceID, labels)
		require.Error(t, err)
		var permErr *domain.PermissionError
		assert.ErrorAs(t, err, &permErr)
	})

	t.Run("member with nil permissions is denied", func(t *testing.T) {
		memberWorkspace := &domain.UserWorkspace{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Role:        "member",
			Permissions: nil,
		}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{ID: userID}, memberWorkspace, nil)

		err := service.SetCustomFieldLabels(ctx, workspaceID, labels)
		require.Error(t, err)
		var permErr *domain.PermissionError
		assert.ErrorAs(t, err, &permErr)
	})

	t.Run("authentication failure returns error", func(t *testing.T) {
		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, nil, nil, assert.AnError)

		err := service.SetCustomFieldLabels(ctx, workspaceID, labels)
		require.Error(t, err)
	})

	t.Run("get workspace error is propagated", func(t *testing.T) {
		ownerWorkspace := &domain.UserWorkspace{UserID: userID, WorkspaceID: workspaceID, Role: "owner"}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{ID: userID}, ownerWorkspace, nil)
		mockRepo.EXPECT().GetByID(ctx, workspaceID).Return(nil, assert.AnError)

		err := service.SetCustomFieldLabels(ctx, workspaceID, labels)
		require.Error(t, err)
	})

	t.Run("update error is propagated", func(t *testing.T) {
		ownerWorkspace := &domain.UserWorkspace{UserID: userID, WorkspaceID: workspaceID, Role: "owner"}
		existing := &domain.Workspace{ID: workspaceID, Name: "WS", Settings: domain.WorkspaceSettings{Timezone: "UTC"}}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{ID: userID}, ownerWorkspace, nil)
		mockRepo.EXPECT().GetByID(ctx, workspaceID).Return(existing, nil)
		mockRepo.EXPECT().Update(ctx, gomock.Any()).Return(assert.AnError)

		err := service.SetCustomFieldLabels(ctx, workspaceID, labels)
		require.Error(t, err)
	})

	t.Run("invalid label is rejected before update", func(t *testing.T) {
		ownerWorkspace := &domain.UserWorkspace{UserID: userID, WorkspaceID: workspaceID, Role: "owner"}
		existing := &domain.Workspace{ID: workspaceID, Name: "WS", Settings: domain.WorkspaceSettings{Timezone: "UTC"}}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{ID: userID}, ownerWorkspace, nil)
		mockRepo.EXPECT().GetByID(ctx, workspaceID).Return(existing, nil)
		// No Update expected — validation fails first.

		err := service.SetCustomFieldLabels(ctx, workspaceID, map[string]string{"custom_string_99": "Bad"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid custom field key")
	})

	t.Run("empty labels clears all and preserves other settings", func(t *testing.T) {
		ownerWorkspace := &domain.UserWorkspace{UserID: userID, WorkspaceID: workspaceID, Role: "owner"}
		existing := &domain.Workspace{
			ID:   workspaceID,
			Name: "WS",
			Settings: domain.WorkspaceSettings{
				WebsiteURL:        "https://example.com",
				Timezone:          "Europe/Paris",
				DefaultLanguage:   "en",
				FileManager:       domain.FileManagerSettings{Bucket: "my-bucket", AccessKey: "AKIA"},
				CustomFieldLabels: map[string]string{"custom_string_1": "Old"},
			},
		}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{ID: userID}, ownerWorkspace, nil)
		mockRepo.EXPECT().GetByID(ctx, workspaceID).Return(existing, nil)
		mockRepo.EXPECT().Update(ctx, gomock.Any()).DoAndReturn(func(_ context.Context, ws *domain.Workspace) error {
			// Labels cleared
			assert.Empty(t, ws.Settings.CustomFieldLabels)
			// Other settings preserved untouched
			assert.Equal(t, "https://example.com", ws.Settings.WebsiteURL)
			assert.Equal(t, "Europe/Paris", ws.Settings.Timezone)
			assert.Equal(t, "my-bucket", ws.Settings.FileManager.Bucket)
			assert.Equal(t, "AKIA", ws.Settings.FileManager.AccessKey)
			return nil
		})

		err := service.SetCustomFieldLabels(ctx, workspaceID, map[string]string{})
		require.NoError(t, err)
	})
}

func TestWorkspaceService_SetBlogSettings(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockUserRepo := mocks.NewMockUserRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockUserService := mocks.NewMockUserServiceInterface(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)
	mockMailer := pkgmocks.NewMockMailer(ctrl)
	mockConfig := &config.Config{RootEmail: "test@example.com"}
	mockContactService := mocks.NewMockContactService(ctrl)
	mockListService := mocks.NewMockListService(ctrl)
	mockContactListService := mocks.NewMockContactListService(ctrl)
	mockTemplateService := mocks.NewMockTemplateService(ctrl)
	mockWebhookRegService := mocks.NewMockWebhookRegistrationService(ctrl)

	service := NewWorkspaceService(
		mockRepo,
		mockUserRepo,
		mocks.NewMockTaskRepository(ctrl),
		mockLogger,
		mockUserService,
		mockAuthService,
		mockMailer,
		mockConfig,
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

	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

	ctx := context.Background()
	workspaceID := "testworkspace"
	userID := "testuser"
	settings := &domain.BlogSettings{Title: "My Blog", HomePageSize: 10}

	t.Run("owner can set blog settings", func(t *testing.T) {
		ownerWorkspace := &domain.UserWorkspace{UserID: userID, WorkspaceID: workspaceID, Role: "owner"}
		existing := &domain.Workspace{ID: workspaceID, Name: "WS", Settings: domain.WorkspaceSettings{Timezone: "UTC"}}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{ID: userID}, ownerWorkspace, nil)
		mockRepo.EXPECT().GetByID(ctx, workspaceID).Return(existing, nil)
		mockRepo.EXPECT().Update(ctx, gomock.Any()).DoAndReturn(func(_ context.Context, ws *domain.Workspace) error {
			assert.True(t, ws.Settings.BlogEnabled)
			assert.Equal(t, settings, ws.Settings.BlogSettings)
			return nil
		})

		err := service.SetBlogSettings(ctx, workspaceID, boolPtr(true), settings, true)
		require.NoError(t, err)
	})

	t.Run("member with blog write can set blog settings (the fix)", func(t *testing.T) {
		memberWorkspace := &domain.UserWorkspace{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Role:        "member",
			Permissions: domain.UserPermissions{
				domain.PermissionResourceBlog: {Read: true, Write: true},
			},
		}
		existing := &domain.Workspace{ID: workspaceID, Name: "WS", Settings: domain.WorkspaceSettings{Timezone: "UTC"}}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{ID: userID}, memberWorkspace, nil)
		mockRepo.EXPECT().GetByID(ctx, workspaceID).Return(existing, nil)
		mockRepo.EXPECT().Update(ctx, gomock.Any()).DoAndReturn(func(_ context.Context, ws *domain.Workspace) error {
			assert.True(t, ws.Settings.BlogEnabled)
			assert.Equal(t, settings, ws.Settings.BlogSettings)
			return nil
		})

		err := service.SetBlogSettings(ctx, workspaceID, boolPtr(true), settings, true)
		require.NoError(t, err)
	})

	t.Run("member with full permissions can set blog settings", func(t *testing.T) {
		memberWorkspace := &domain.UserWorkspace{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Role:        "member",
			Permissions: domain.FullPermissions,
		}
		existing := &domain.Workspace{ID: workspaceID, Name: "WS", Settings: domain.WorkspaceSettings{Timezone: "UTC"}}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{ID: userID}, memberWorkspace, nil)
		mockRepo.EXPECT().GetByID(ctx, workspaceID).Return(existing, nil)
		mockRepo.EXPECT().Update(ctx, gomock.Any()).Return(nil)

		err := service.SetBlogSettings(ctx, workspaceID, boolPtr(true), settings, true)
		require.NoError(t, err)
	})

	t.Run("member with blog read only is denied", func(t *testing.T) {
		memberWorkspace := &domain.UserWorkspace{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Role:        "member",
			Permissions: domain.UserPermissions{
				domain.PermissionResourceBlog: {Read: true, Write: false},
			},
		}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{ID: userID}, memberWorkspace, nil)
		// No GetByID / Update expected.

		err := service.SetBlogSettings(ctx, workspaceID, boolPtr(true), settings, true)
		require.Error(t, err)
		var permErr *domain.PermissionError
		assert.ErrorAs(t, err, &permErr)
	})

	t.Run("member with only contacts permission is denied", func(t *testing.T) {
		memberWorkspace := &domain.UserWorkspace{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Role:        "member",
			Permissions: domain.UserPermissions{
				domain.PermissionResourceContacts: {Read: true, Write: true},
			},
		}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{ID: userID}, memberWorkspace, nil)

		err := service.SetBlogSettings(ctx, workspaceID, boolPtr(true), settings, true)
		require.Error(t, err)
		var permErr *domain.PermissionError
		assert.ErrorAs(t, err, &permErr)
	})

	t.Run("member with nil permissions is denied", func(t *testing.T) {
		memberWorkspace := &domain.UserWorkspace{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Role:        "member",
			Permissions: nil,
		}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{ID: userID}, memberWorkspace, nil)

		err := service.SetBlogSettings(ctx, workspaceID, boolPtr(true), settings, true)
		require.Error(t, err)
		var permErr *domain.PermissionError
		assert.ErrorAs(t, err, &permErr)
	})

	t.Run("authentication failure returns error", func(t *testing.T) {
		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, nil, nil, assert.AnError)

		err := service.SetBlogSettings(ctx, workspaceID, boolPtr(true), settings, true)
		require.Error(t, err)
	})

	t.Run("get workspace error is propagated", func(t *testing.T) {
		ownerWorkspace := &domain.UserWorkspace{UserID: userID, WorkspaceID: workspaceID, Role: "owner"}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{ID: userID}, ownerWorkspace, nil)
		mockRepo.EXPECT().GetByID(ctx, workspaceID).Return(nil, assert.AnError)

		err := service.SetBlogSettings(ctx, workspaceID, boolPtr(true), settings, true)
		require.Error(t, err)
	})

	t.Run("update error is propagated", func(t *testing.T) {
		ownerWorkspace := &domain.UserWorkspace{UserID: userID, WorkspaceID: workspaceID, Role: "owner"}
		existing := &domain.Workspace{ID: workspaceID, Name: "WS", Settings: domain.WorkspaceSettings{Timezone: "UTC"}}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{ID: userID}, ownerWorkspace, nil)
		mockRepo.EXPECT().GetByID(ctx, workspaceID).Return(existing, nil)
		mockRepo.EXPECT().Update(ctx, gomock.Any()).Return(assert.AnError)

		err := service.SetBlogSettings(ctx, workspaceID, boolPtr(true), settings, true)
		require.Error(t, err)
	})

	t.Run("invalid settings rejected before update", func(t *testing.T) {
		ownerWorkspace := &domain.UserWorkspace{UserID: userID, WorkspaceID: workspaceID, Role: "owner"}
		existing := &domain.Workspace{ID: workspaceID, Name: "WS", Settings: domain.WorkspaceSettings{Timezone: "UTC"}}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{ID: userID}, ownerWorkspace, nil)
		mockRepo.EXPECT().GetByID(ctx, workspaceID).Return(existing, nil)
		// No Update expected — validation fails first.

		err := service.SetBlogSettings(ctx, workspaceID, boolPtr(true), &domain.BlogSettings{HomePageSize: 999}, true)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "home_page_size must be between 1 and 100")
	})

	t.Run("disable clears blog settings and preserves other settings", func(t *testing.T) {
		ownerWorkspace := &domain.UserWorkspace{UserID: userID, WorkspaceID: workspaceID, Role: "owner"}
		existing := &domain.Workspace{
			ID:   workspaceID,
			Name: "WS",
			Settings: domain.WorkspaceSettings{
				WebsiteURL:        "https://example.com",
				Timezone:          "Europe/Paris",
				DefaultLanguage:   "en",
				FileManager:       domain.FileManagerSettings{Bucket: "my-bucket", AccessKey: "AKIA"},
				CustomFieldLabels: map[string]string{"custom_string_1": "Keep Me"},
				BlogEnabled:       true,
				BlogSettings:      &domain.BlogSettings{Title: "Old Blog"},
			},
		}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{ID: userID}, ownerWorkspace, nil)
		mockRepo.EXPECT().GetByID(ctx, workspaceID).Return(existing, nil)
		mockRepo.EXPECT().Update(ctx, gomock.Any()).DoAndReturn(func(_ context.Context, ws *domain.Workspace) error {
			// Blog disabled and cleared.
			assert.False(t, ws.Settings.BlogEnabled)
			assert.Nil(t, ws.Settings.BlogSettings)
			// Other settings preserved untouched.
			assert.Equal(t, "https://example.com", ws.Settings.WebsiteURL)
			assert.Equal(t, "Europe/Paris", ws.Settings.Timezone)
			assert.Equal(t, "my-bucket", ws.Settings.FileManager.Bucket)
			assert.Equal(t, map[string]string{"custom_string_1": "Keep Me"}, ws.Settings.CustomFieldLabels)
			return nil
		})

		err := service.SetBlogSettings(ctx, workspaceID, boolPtr(false), nil, true)
		require.NoError(t, err)
	})
}

func TestWorkspaceService_UpdateWorkspace(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockUserRepo := mocks.NewMockUserRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockUserService := mocks.NewMockUserServiceInterface(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)
	mockMailer := pkgmocks.NewMockMailer(ctrl)
	mockConfig := &config.Config{RootEmail: "test@example.com"}
	mockContactService := mocks.NewMockContactService(ctrl)
	mockListService := mocks.NewMockListService(ctrl)
	mockContactListService := mocks.NewMockContactListService(ctrl)
	mockTemplateService := mocks.NewMockTemplateService(ctrl)
	mockWebhookRegService := mocks.NewMockWebhookRegistrationService(ctrl)

	service := NewWorkspaceService(
		mockRepo,
		mockUserRepo,
		mocks.NewMockTaskRepository(ctrl),
		mockLogger,
		mockUserService,
		mockAuthService,
		mockMailer,
		mockConfig,
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
	workspaceID := "testworkspace"
	userID := "testuser"

	t.Run("successful update", func(t *testing.T) {
		expectedUser := &domain.User{
			ID: userID,
		}

		expectedUserWorkspace := &domain.UserWorkspace{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Role:        "owner",
		}

		settings := domain.WorkspaceSettings{
			WebsiteURL:      "https://example.com",
			LogoURL:         "https://example.com/logo.png",
			CoverURL:        "https://example.com/cover.png",
			Timezone:        "UTC",
			DefaultLanguage: "en",
			Languages:       []string{"en"},
			FileManager: domain.FileManagerSettings{
				Endpoint:  "https://s3.amazonaws.com",
				Bucket:    "my-bucket",
				AccessKey: "AKIAIOSFODNN7EXAMPLE",
			},
		}

		existingWorkspace := &domain.Workspace{
			ID:   workspaceID,
			Name: "Original Workspace Name",
			Settings: domain.WorkspaceSettings{
				WebsiteURL: "https://old-example.com",
			},
			CreatedAt: time.Now().Add(-24 * time.Hour), // Created a day ago
			UpdatedAt: time.Now().Add(-24 * time.Hour),
		}

		expectedWorkspace := &domain.Workspace{
			ID:        workspaceID,
			Name:      "Updated Workspace",
			Settings:  settings,
			CreatedAt: existingWorkspace.CreatedAt,
			UpdatedAt: time.Now(),
		}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, expectedUser, expectedUserWorkspace, nil)
		mockRepo.EXPECT().GetByID(ctx, workspaceID).Return(existingWorkspace, nil)
		mockRepo.EXPECT().Update(ctx, gomock.Any()).Return(nil)

		workspace, err := service.UpdateWorkspace(ctx, workspaceID, "Updated Workspace", settings)
		require.NoError(t, err)
		assert.Equal(t, expectedWorkspace.ID, workspace.ID)
		assert.Equal(t, expectedWorkspace.Name, workspace.Name)
		assert.Equal(t, expectedWorkspace.Settings, workspace.Settings)
	})

	t.Run("reject transactional-only provider as marketing provider", func(t *testing.T) {
		expectedUser := &domain.User{
			ID: userID,
		}

		expectedUserWorkspace := &domain.UserWorkspace{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Role:        "owner",
		}

		mailjetIntegrationID := "mailjet-integration"

		settings := domain.WorkspaceSettings{
			Timezone:                 "UTC",
			DefaultLanguage:          "en",
			Languages:                []string{"en"},
			MarketingEmailProviderID: mailjetIntegrationID,
		}

		existingWorkspace := &domain.Workspace{
			ID:   workspaceID,
			Name: "Original Workspace Name",
			Integrations: []domain.Integration{
				{
					ID:   mailjetIntegrationID,
					Name: "Mailjet",
					Type: domain.IntegrationTypeEmail,
					EmailProvider: domain.EmailProvider{
						Kind: domain.EmailProviderKindMailjet,
					},
				},
			},
			CreatedAt: time.Now().Add(-24 * time.Hour),
			UpdatedAt: time.Now().Add(-24 * time.Hour),
		}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, expectedUser, expectedUserWorkspace, nil)
		mockRepo.EXPECT().GetByID(ctx, workspaceID).Return(existingWorkspace, nil)

		workspace, err := service.UpdateWorkspace(ctx, workspaceID, "Updated Workspace", settings)
		require.Error(t, err)
		assert.Nil(t, workspace)
		assert.Contains(t, err.Error(), "transactional")
	})

	t.Run("allow unchanged grandfathered transactional-only marketing provider", func(t *testing.T) {
		// A workspace whose marketing provider was set to a transactional-only
		// kind BEFORE the restriction existed must still be able to save
		// unrelated settings changes: the console resubmits the full settings
		// object, so the guard only fires when the assignment CHANGES.
		// (Send-time resolution blocks the grandfathered provider instead.)
		expectedUser := &domain.User{
			ID: userID,
		}

		expectedUserWorkspace := &domain.UserWorkspace{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Role:        "owner",
		}

		mailjetIntegrationID := "mailjet-integration"

		settings := domain.WorkspaceSettings{
			Timezone:                 "Europe/Paris", // the unrelated change being saved
			DefaultLanguage:          "en",
			Languages:                []string{"en"},
			MarketingEmailProviderID: mailjetIntegrationID, // unchanged, rides along
		}

		existingWorkspace := &domain.Workspace{
			ID:   workspaceID,
			Name: "Original Workspace Name",
			Settings: domain.WorkspaceSettings{
				Timezone:                 "UTC",
				MarketingEmailProviderID: mailjetIntegrationID, // grandfathered
			},
			Integrations: []domain.Integration{
				{
					ID:   mailjetIntegrationID,
					Name: "Mailjet",
					Type: domain.IntegrationTypeEmail,
					// A fully valid integration: unlike the reject case above,
					// this save passes the guard and reaches workspace
					// validation before the repo Update.
					EmailProvider: domain.EmailProvider{
						Kind:               domain.EmailProviderKindMailjet,
						RateLimitPerMinute: 25,
						Senders: []domain.EmailSender{
							{
								ID:    "123e4567-e89b-12d3-a456-426614174000",
								Email: "sender@example.com",
								Name:  "Sender",
							},
						},
						Mailjet: &domain.MailjetSettings{},
					},
				},
			},
			CreatedAt: time.Now().Add(-24 * time.Hour),
			UpdatedAt: time.Now().Add(-24 * time.Hour),
		}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, expectedUser, expectedUserWorkspace, nil)
		mockRepo.EXPECT().GetByID(ctx, workspaceID).Return(existingWorkspace, nil)
		mockRepo.EXPECT().Update(ctx, gomock.Any()).Return(nil)

		workspace, err := service.UpdateWorkspace(ctx, workspaceID, "Updated Workspace", settings)
		require.NoError(t, err)
		require.NotNil(t, workspace)
		assert.Equal(t, "Europe/Paris", workspace.Settings.Timezone)
		assert.Equal(t, mailjetIntegrationID, workspace.Settings.MarketingEmailProviderID)
	})

	t.Run("authentication error", func(t *testing.T) {
		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, nil, nil, assert.AnError)

		settings := domain.WorkspaceSettings{
			WebsiteURL:      "https://example.com",
			LogoURL:         "https://example.com/logo.png",
			CoverURL:        "https://example.com/cover.png",
			Timezone:        "UTC",
			DefaultLanguage: "en",
			Languages:       []string{"en"},
			FileManager: domain.FileManagerSettings{
				Endpoint:  "https://s3.amazonaws.com",
				Bucket:    "my-bucket",
				AccessKey: "AKIAIOSFODNN7EXAMPLE",
			},
		}

		workspace, err := service.UpdateWorkspace(ctx, workspaceID, "Updated Workspace", settings)
		require.Error(t, err)
		assert.Nil(t, workspace)
		assert.Contains(t, err.Error(), assert.AnError.Error())
	})

	t.Run("user not workspace owner", func(t *testing.T) {
		expectedUser := &domain.User{
			ID: userID,
		}

		expectedUserWorkspace := &domain.UserWorkspace{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Role:        "member", // Not an owner
		}

		settings := domain.WorkspaceSettings{
			WebsiteURL:      "https://example.com",
			LogoURL:         "https://example.com/logo.png",
			CoverURL:        "https://example.com/cover.png",
			Timezone:        "UTC",
			DefaultLanguage: "en",
			Languages:       []string{"en"},
			FileManager: domain.FileManagerSettings{
				Endpoint:  "https://s3.amazonaws.com",
				Bucket:    "my-bucket",
				AccessKey: "AKIAIOSFODNN7EXAMPLE",
			},
		}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, expectedUser, expectedUserWorkspace, nil)

		workspace, err := service.UpdateWorkspace(ctx, workspaceID, "Updated Workspace", settings)
		require.Error(t, err)
		assert.Nil(t, workspace)
		assert.IsType(t, &domain.ErrUnauthorized{}, err)
	})

	t.Run("preserves custom field labels and ignores request labels", func(t *testing.T) {
		// Custom field labels are managed exclusively via SetCustomFieldLabels. UpdateWorkspace
		// must NOT modify them: it preserves the labels already stored on the workspace and
		// ignores any labels passed in the update request (prevents a stale owner save from
		// clobbering labels set by a member with workspace:write).
		expectedUser := &domain.User{
			ID: userID,
		}

		expectedUserWorkspace := &domain.UserWorkspace{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Role:        "owner",
		}

		// Labels already stored on the workspace (set via the dedicated endpoint).
		existingLabels := map[string]string{
			"custom_string_1": "Existing Label",
		}
		// Different labels in the update request — these must be IGNORED.
		requestLabels := map[string]string{
			"custom_number_1": "From Request",
		}

		settings := domain.WorkspaceSettings{
			WebsiteURL:      "https://example.com",
			Timezone:        "UTC",
			DefaultLanguage: "en",
			Languages:       []string{"en"},
			FileManager: domain.FileManagerSettings{
				Endpoint:  "https://s3.amazonaws.com",
				Bucket:    "my-bucket",
				AccessKey: "AKIAIOSFODNN7EXAMPLE",
			},
			CustomFieldLabels: requestLabels,
		}

		existingWorkspace := &domain.Workspace{
			ID:   workspaceID,
			Name: "Original Workspace Name",
			Settings: domain.WorkspaceSettings{
				WebsiteURL:        "https://old-example.com",
				CustomFieldLabels: existingLabels,
			},
			CreatedAt: time.Now().Add(-24 * time.Hour),
			UpdatedAt: time.Now().Add(-24 * time.Hour),
		}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, expectedUser, expectedUserWorkspace, nil)
		mockRepo.EXPECT().GetByID(ctx, workspaceID).Return(existingWorkspace, nil)
		mockRepo.EXPECT().Update(ctx, gomock.Any()).DoAndReturn(func(ctx context.Context, workspace *domain.Workspace) error {
			// Existing labels preserved, request labels ignored.
			assert.Equal(t, existingLabels, workspace.Settings.CustomFieldLabels)
			assert.NotContains(t, workspace.Settings.CustomFieldLabels, "custom_number_1")
			return nil
		})

		workspace, err := service.UpdateWorkspace(ctx, workspaceID, "Updated Workspace", settings)
		require.NoError(t, err)
		assert.NotNil(t, workspace)
		assert.Equal(t, existingLabels, workspace.Settings.CustomFieldLabels)
	})

	t.Run("preserves blog settings and ignores request blog fields", func(t *testing.T) {
		// Blog settings are managed exclusively via SetBlogSettings. UpdateWorkspace
		// must NOT modify them: it preserves the blog config already stored on the
		// workspace and ignores any blog fields passed in the update request (prevents
		// a stale owner save from clobbering blog config set by a member with blog:write).
		expectedUser := &domain.User{ID: userID}
		expectedUserWorkspace := &domain.UserWorkspace{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Role:        "owner",
		}

		// Blog config already stored on the workspace (set via the dedicated endpoint).
		existingBlog := &domain.BlogSettings{Title: "Member's Blog"}

		// The update request asks to disable the blog and change its title — both IGNORED.
		settings := domain.WorkspaceSettings{
			WebsiteURL:      "https://example.com",
			Timezone:        "UTC",
			DefaultLanguage: "en",
			Languages:       []string{"en"},
			BlogEnabled:     false,
			BlogSettings:    &domain.BlogSettings{Title: "From Request"},
		}

		existingWorkspace := &domain.Workspace{
			ID:   workspaceID,
			Name: "Original Workspace Name",
			Settings: domain.WorkspaceSettings{
				WebsiteURL:   "https://old-example.com",
				BlogEnabled:  true,
				BlogSettings: existingBlog,
			},
			CreatedAt: time.Now().Add(-24 * time.Hour),
			UpdatedAt: time.Now().Add(-24 * time.Hour),
		}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, expectedUser, expectedUserWorkspace, nil)
		mockRepo.EXPECT().GetByID(ctx, workspaceID).Return(existingWorkspace, nil)
		mockRepo.EXPECT().Update(ctx, gomock.Any()).DoAndReturn(func(ctx context.Context, workspace *domain.Workspace) error {
			// Existing blog config preserved, request blog fields ignored.
			assert.True(t, workspace.Settings.BlogEnabled)
			assert.Equal(t, existingBlog, workspace.Settings.BlogSettings)
			return nil
		})

		workspace, err := service.UpdateWorkspace(ctx, workspaceID, "Updated Workspace", settings)
		require.NoError(t, err)
		assert.NotNil(t, workspace)
		assert.True(t, workspace.Settings.BlogEnabled)
		assert.Equal(t, existingBlog, workspace.Settings.BlogSettings)
	})

	t.Run("preserves template blocks when not provided", func(t *testing.T) {
		expectedUser := &domain.User{ID: userID}
		expectedUserWorkspace := &domain.UserWorkspace{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Role:        "owner",
		}

		// Create a test email block
		blockJSON := []byte(`{"id":"b1","type":"mj-text","content":"Hello","attributes":{"fontSize":"16px"}}`)
		testBlock, _ := notifuse_mjml.UnmarshalEmailBlock(blockJSON)

		// Existing workspace with template blocks
		existingTemplateBlocks := []domain.TemplateBlock{
			{
				ID:      "block-1",
				Name:    "Existing Block",
				Block:   testBlock,
				Created: time.Now().Add(-24 * time.Hour),
				Updated: time.Now().Add(-24 * time.Hour),
			},
		}

		existingWorkspace := &domain.Workspace{
			ID:   workspaceID,
			Name: "Original Workspace",
			Settings: domain.WorkspaceSettings{
				Timezone:        "UTC",
				DefaultLanguage: "en",
				Languages:       []string{"en"},
				TemplateBlocks:  existingTemplateBlocks,
			},
		}

		// Update settings without template blocks
		settings := domain.WorkspaceSettings{
			Timezone:        "America/New_York",
			DefaultLanguage: "en",
			Languages:       []string{"en"},
		}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, expectedUser, expectedUserWorkspace, nil)
		mockRepo.EXPECT().GetByID(ctx, workspaceID).Return(existingWorkspace, nil)
		mockRepo.EXPECT().Update(ctx, gomock.Any()).DoAndReturn(func(ctx context.Context, workspace *domain.Workspace) error {
			// Verify template blocks are preserved
			assert.Len(t, workspace.Settings.TemplateBlocks, 1)
			assert.Equal(t, "block-1", workspace.Settings.TemplateBlocks[0].ID)
			assert.Equal(t, "Existing Block", workspace.Settings.TemplateBlocks[0].Name)
			// Verify timezone was updated
			assert.Equal(t, "America/New_York", workspace.Settings.Timezone)
			return nil
		})

		workspace, err := service.UpdateWorkspace(ctx, workspaceID, "Updated Workspace", settings)
		require.NoError(t, err)
		assert.NotNil(t, workspace)
		assert.Len(t, workspace.Settings.TemplateBlocks, 1)
		assert.Equal(t, existingTemplateBlocks[0].ID, workspace.Settings.TemplateBlocks[0].ID)
	})

	t.Run("updates template blocks when explicitly provided", func(t *testing.T) {
		expectedUser := &domain.User{ID: userID}
		expectedUserWorkspace := &domain.UserWorkspace{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Role:        "owner",
		}

		// Create a test email block
		blockJSON := []byte(`{"id":"b1","type":"mj-text","content":"Hello","attributes":{"fontSize":"16px"}}`)
		testBlock, _ := notifuse_mjml.UnmarshalEmailBlock(blockJSON)

		existingWorkspace := &domain.Workspace{
			ID:   workspaceID,
			Name: "Original Workspace",
			Settings: domain.WorkspaceSettings{
				Timezone:        "UTC",
				DefaultLanguage: "en",
				Languages:       []string{"en"},
				TemplateBlocks: []domain.TemplateBlock{
					{
						ID:      "old-block",
						Name:    "Old Block",
						Block:   testBlock,
						Created: time.Now().Add(-24 * time.Hour),
						Updated: time.Now().Add(-24 * time.Hour),
					},
				},
			},
		}

		// Update settings with new template blocks
		newTemplateBlocks := []domain.TemplateBlock{
			{
				ID:    "", // New block without ID
				Name:  "New Block",
				Block: testBlock,
			},
		}

		settings := domain.WorkspaceSettings{
			Timezone:        "UTC",
			DefaultLanguage: "en",
			Languages:       []string{"en"},
			TemplateBlocks:  newTemplateBlocks,
		}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, expectedUser, expectedUserWorkspace, nil)
		mockRepo.EXPECT().GetByID(ctx, workspaceID).Return(existingWorkspace, nil)
		mockRepo.EXPECT().Update(ctx, gomock.Any()).DoAndReturn(func(ctx context.Context, workspace *domain.Workspace) error {
			// Verify template blocks were updated
			assert.Len(t, workspace.Settings.TemplateBlocks, 1)
			assert.NotEmpty(t, workspace.Settings.TemplateBlocks[0].ID) // ID should be generated
			assert.Equal(t, "New Block", workspace.Settings.TemplateBlocks[0].Name)
			assert.NotZero(t, workspace.Settings.TemplateBlocks[0].Created)
			assert.NotZero(t, workspace.Settings.TemplateBlocks[0].Updated)
			return nil
		})

		workspace, err := service.UpdateWorkspace(ctx, workspaceID, "Updated Workspace", settings)
		require.NoError(t, err)
		assert.NotNil(t, workspace)
		assert.Len(t, workspace.Settings.TemplateBlocks, 1)
		assert.Equal(t, "New Block", workspace.Settings.TemplateBlocks[0].Name)
	})
}

func TestWorkspaceService_DeleteWorkspace(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockUserRepo := mocks.NewMockUserRepository(ctrl)
	mockTaskRepo := mocks.NewMockTaskRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockUserService := mocks.NewMockUserServiceInterface(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)
	mockMailer := pkgmocks.NewMockMailer(ctrl)
	mockConfig := &config.Config{Environment: "development"}
	mockContactService := mocks.NewMockContactService(ctrl)
	mockListService := mocks.NewMockListService(ctrl)
	mockContactListService := mocks.NewMockContactListService(ctrl)
	mockTemplateService := mocks.NewMockTemplateService(ctrl)
	mockWebhookRegService := mocks.NewMockWebhookRegistrationService(ctrl)

	service := NewWorkspaceService(
		mockRepo,
		mockUserRepo,
		mockTaskRepo,
		mockLogger,
		mockUserService,
		mockAuthService,
		mockMailer,
		mockConfig,
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
	mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()

	ctx := context.Background()
	userID := "testuser"
	workspaceID := "testworkspace"

	t.Run("successful delete as owner with no integrations", func(t *testing.T) {
		expectedUser := &domain.User{
			ID: userID,
		}

		userWorkspace := &domain.UserWorkspace{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Role:        "owner",
		}

		// Workspace with no integrations
		workspace := &domain.Workspace{
			ID:           workspaceID,
			Name:         "Test Workspace",
			Integrations: []domain.Integration{},
		}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, expectedUser, userWorkspace, nil)
		mockRepo.EXPECT().GetByID(ctx, workspaceID).Return(workspace, nil)
		mockTaskRepo.EXPECT().DeleteAll(ctx, workspaceID).Return(nil)
		mockRepo.EXPECT().Delete(ctx, workspaceID).Return(nil)

		err := service.DeleteWorkspace(ctx, workspaceID)
		require.NoError(t, err)
	})

	t.Run("successful delete as owner with integrations", func(t *testing.T) {
		expectedUser := &domain.User{
			ID: userID,
		}

		userWorkspace := &domain.UserWorkspace{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Role:        "owner",
		}

		// Workspace with two integrations
		integrations := []domain.Integration{
			{
				ID:   "integration-1",
				Name: "Integration 1",
				Type: domain.IntegrationTypeEmail,
				EmailProvider: domain.EmailProvider{
					Kind: domain.EmailProviderKindSMTP,
				},
			},
			{
				ID:   "integration-2",
				Name: "Integration 2",
				Type: domain.IntegrationTypeEmail,
				EmailProvider: domain.EmailProvider{
					Kind: domain.EmailProviderKindSMTP,
				},
			},
		}

		workspace := &domain.Workspace{
			ID:           workspaceID,
			Name:         "Test Workspace",
			Integrations: integrations,
		}

		// Initial authentication for the DeleteWorkspace itself
		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, expectedUser, userWorkspace, nil)
		mockRepo.EXPECT().GetByID(ctx, workspaceID).Return(workspace, nil)

		// For each DeleteIntegration call inside DeleteWorkspace, expect these mocks
		// The DeleteIntegration method will call AuthenticateUserForWorkspace again for each integration
		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, expectedUser, userWorkspace, nil).Times(2)
		mockRepo.EXPECT().GetByID(ctx, workspaceID).Return(workspace, nil).Times(2)

		// No webhook operations expected for SMTP integrations

		// Once for each integration deletion
		mockRepo.EXPECT().Update(ctx, gomock.Any()).Return(nil).Times(2)

		// Task cleanup
		mockTaskRepo.EXPECT().DeleteAll(ctx, workspaceID).Return(nil)

		// Final workspace deletion
		mockRepo.EXPECT().Delete(ctx, workspaceID).Return(nil)

		err := service.DeleteWorkspace(ctx, workspaceID)
		require.NoError(t, err)
	})

	t.Run("continues deletion despite integration deletion failure", func(t *testing.T) {
		expectedUser := &domain.User{
			ID: userID,
		}

		userWorkspace := &domain.UserWorkspace{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Role:        "owner",
		}

		// Workspace with one integration
		integrations := []domain.Integration{
			{
				ID:   "integration-1",
				Name: "Integration 1",
				Type: domain.IntegrationTypeEmail,
				EmailProvider: domain.EmailProvider{
					Kind: domain.EmailProviderKindSMTP,
				},
			},
		}

		workspace := &domain.Workspace{
			ID:           workspaceID,
			Name:         "Test Workspace",
			Integrations: integrations,
		}

		// Initial authentication for DeleteWorkspace
		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, expectedUser, userWorkspace, nil)
		mockRepo.EXPECT().GetByID(ctx, workspaceID).Return(workspace, nil)

		// Authentication for DeleteIntegration
		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, expectedUser, userWorkspace, nil)
		mockRepo.EXPECT().GetByID(ctx, workspaceID).Return(workspace, nil)

		// No webhook operations expected for SMTP integrations
		// The update fails
		mockRepo.EXPECT().Update(ctx, gomock.Any()).Return(errors.New("integration delete error"))

		// Should still proceed with task cleanup and workspace deletion
		mockTaskRepo.EXPECT().DeleteAll(ctx, workspaceID).Return(nil)
		mockRepo.EXPECT().Delete(ctx, workspaceID).Return(nil)

		err := service.DeleteWorkspace(ctx, workspaceID)
		require.NoError(t, err)
	})

	t.Run("unauthorized user", func(t *testing.T) {
		expectedUser := &domain.User{
			ID: userID,
		}

		userWorkspace := &domain.UserWorkspace{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Role:        "member", // Not an owner
		}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, expectedUser, userWorkspace, nil)

		err := service.DeleteWorkspace(ctx, workspaceID)
		require.Error(t, err)
		assert.IsType(t, &domain.ErrUnauthorized{}, err)
	})

	t.Run("error getting workspace details", func(t *testing.T) {
		expectedUser := &domain.User{
			ID: userID,
		}

		userWorkspace := &domain.UserWorkspace{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Role:        "owner",
		}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, expectedUser, userWorkspace, nil)
		mockRepo.EXPECT().GetByID(ctx, workspaceID).Return(nil, errors.New("error getting workspace"))

		err := service.DeleteWorkspace(ctx, workspaceID)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "error getting workspace")
	})
}

func TestWorkspaceService_CreateIntegration(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockUserRepo := mocks.NewMockUserRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockUserService := mocks.NewMockUserServiceInterface(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)
	mockMailer := pkgmocks.NewMockMailer(ctrl)
	mockConfig := &config.Config{RootEmail: "test@example.com"}
	mockContactService := mocks.NewMockContactService(ctrl)
	mockListService := mocks.NewMockListService(ctrl)
	mockContactListService := mocks.NewMockContactListService(ctrl)
	mockTemplateService := mocks.NewMockTemplateService(ctrl)
	mockWebhookRegService := mocks.NewMockWebhookRegistrationService(ctrl)

	service := NewWorkspaceService(
		mockRepo,
		mockUserRepo,
		mocks.NewMockTaskRepository(ctrl),
		mockLogger,
		mockUserService,
		mockAuthService,
		mockMailer,
		mockConfig,
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
	workspaceID := "testworkspace"
	userID := "testuser"
	integrationName := "Test SMTP Integration"

	provider := domain.EmailProvider{
		Kind:               domain.EmailProviderKindSMTP,
		RateLimitPerMinute: 25,
		SMTP: &domain.SMTPSettings{
			Host:     "smtp.example.com",
			Port:     587,
			Username: "smtp_user",
			Password: "smtp_password",
			UseTLS:   true,
		},
		Senders: []domain.EmailSender{
			domain.NewEmailSender("test@example.com", "Test Sender"),
		},
	}

	t.Run("successful create integration", func(t *testing.T) {
		expectedUser := &domain.User{
			ID: userID,
		}

		expectedUserWorkspace := &domain.UserWorkspace{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Role:        "owner",
		}

		expectedWorkspace := &domain.Workspace{
			ID:   workspaceID,
			Name: "Test Workspace",
		}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, expectedUser, expectedUserWorkspace, nil)
		mockRepo.EXPECT().GetByID(ctx, workspaceID).Return(expectedWorkspace, nil)
		mockRepo.EXPECT().Update(ctx, gomock.Any()).DoAndReturn(func(ctx context.Context, workspace *domain.Workspace) error {
			// Verify the integration was added to the workspace
			require.Equal(t, 1, len(workspace.Integrations))
			require.Equal(t, integrationName, workspace.Integrations[0].Name)
			require.Equal(t, domain.IntegrationTypeEmail, workspace.Integrations[0].Type)
			require.Equal(t, domain.EmailProviderKindSMTP, workspace.Integrations[0].EmailProvider.Kind)
			return nil
		})

		// No webhook registration expected for SMTP provider
		mockConfig.APIEndpoint = "https://api.example.com"

		integrationID, err := service.CreateIntegration(ctx, domain.CreateIntegrationRequest{
			WorkspaceID: workspaceID,
			Name:        integrationName,
			Type:        domain.IntegrationTypeEmail,
			Provider:    provider,
		})
		require.NoError(t, err)
		require.NotEmpty(t, integrationID)
	})

	t.Run("unauthorized user", func(t *testing.T) {
		expectedUser := &domain.User{
			ID: userID,
		}

		// User is a member, not an owner
		expectedUserWorkspace := &domain.UserWorkspace{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Role:        "member",
		}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, expectedUser, expectedUserWorkspace, nil)

		integrationID, err := service.CreateIntegration(ctx, domain.CreateIntegrationRequest{
			WorkspaceID: workspaceID,
			Name:        integrationName,
			Type:        domain.IntegrationTypeEmail,
			Provider:    provider,
		})
		require.Error(t, err)
		require.Empty(t, integrationID)
		require.IsType(t, &domain.ErrUnauthorized{}, err)
	})

	t.Run("workspace not found", func(t *testing.T) {
		expectedUser := &domain.User{
			ID: userID,
		}

		expectedUserWorkspace := &domain.UserWorkspace{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Role:        "owner",
		}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, expectedUser, expectedUserWorkspace, nil)
		mockRepo.EXPECT().GetByID(ctx, workspaceID).Return(nil, errors.New("workspace not found"))

		integrationID, err := service.CreateIntegration(ctx, domain.CreateIntegrationRequest{
			WorkspaceID: workspaceID,
			Name:        integrationName,
			Type:        domain.IntegrationTypeEmail,
			Provider:    provider,
		})
		require.Error(t, err)
		require.Empty(t, integrationID)
		require.Contains(t, err.Error(), "workspace not found")
	})

	t.Run("update error", func(t *testing.T) {
		expectedUser := &domain.User{
			ID: userID,
		}

		expectedUserWorkspace := &domain.UserWorkspace{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Role:        "owner",
		}

		expectedWorkspace := &domain.Workspace{
			ID:   workspaceID,
			Name: "Test Workspace",
		}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, expectedUser, expectedUserWorkspace, nil)
		mockRepo.EXPECT().GetByID(ctx, workspaceID).Return(expectedWorkspace, nil)
		mockRepo.EXPECT().Update(ctx, gomock.Any()).Return(errors.New("update error"))

		integrationID, err := service.CreateIntegration(ctx, domain.CreateIntegrationRequest{
			WorkspaceID: workspaceID,
			Name:        integrationName,
			Type:        domain.IntegrationTypeEmail,
			Provider:    provider,
		})
		require.Error(t, err)
		require.Empty(t, integrationID)
		require.Contains(t, err.Error(), "update error")
	})

	t.Run("successful create firecrawl integration", func(t *testing.T) {
		expectedUser := &domain.User{
			ID: userID,
		}

		expectedUserWorkspace := &domain.UserWorkspace{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Role:        "owner",
		}

		expectedWorkspace := &domain.Workspace{
			ID:   workspaceID,
			Name: "Test Workspace",
		}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, expectedUser, expectedUserWorkspace, nil)
		mockRepo.EXPECT().GetByID(ctx, workspaceID).Return(expectedWorkspace, nil)
		mockRepo.EXPECT().Update(ctx, gomock.Any()).DoAndReturn(func(ctx context.Context, workspace *domain.Workspace) error {
			// Verify the integration was added to the workspace
			require.Equal(t, 1, len(workspace.Integrations))
			require.Equal(t, "My Firecrawl", workspace.Integrations[0].Name)
			require.Equal(t, domain.IntegrationTypeFirecrawl, workspace.Integrations[0].Type)
			require.NotNil(t, workspace.Integrations[0].FirecrawlSettings)
			// API key should be encrypted
			require.NotEmpty(t, workspace.Integrations[0].FirecrawlSettings.EncryptedAPIKey)
			require.Empty(t, workspace.Integrations[0].FirecrawlSettings.APIKey) // Plain key should be cleared
			return nil
		})

		integrationID, err := service.CreateIntegration(ctx, domain.CreateIntegrationRequest{
			WorkspaceID: workspaceID,
			Name:        "My Firecrawl",
			Type:        domain.IntegrationTypeFirecrawl,
			FirecrawlSettings: &domain.FirecrawlSettings{
				APIKey: "fc-test-key",
			},
		})
		require.NoError(t, err)
		require.NotEmpty(t, integrationID)
	})

	t.Run("service level create rejects a zapier integration", func(t *testing.T) {
		// The HTTP boundary rejects the type in CreateIntegrationRequest.Validate, which this
		// path never calls: the demo seeder and anything else in the service layer go straight
		// through here. What closes it is the create switch having no zapier case, so the
		// settings stay nil and Integration.Validate refuses the record.
		expectedWorkspace := &domain.Workspace{ID: workspaceID, Name: "Test Workspace"}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{ID: userID}, &domain.UserWorkspace{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Role:        "owner",
		}, nil)
		mockRepo.EXPECT().GetByID(ctx, workspaceID).Return(expectedWorkspace, nil)
		// No Update expectation: writing the record would be the failure.

		integrationID, err := service.CreateIntegration(ctx, domain.CreateIntegrationRequest{
			WorkspaceID: workspaceID,
			Name:        "Zapier",
			Type:        domain.IntegrationTypeZapier,
		})
		require.Error(t, err)
		assert.Empty(t, integrationID)
		assert.Contains(t, err.Error(), "zapier settings are required")
	})
}

func TestWorkspaceService_UpdateIntegration(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockUserRepo := mocks.NewMockUserRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockUserService := mocks.NewMockUserServiceInterface(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)
	mockMailer := pkgmocks.NewMockMailer(ctrl)
	mockConfig := &config.Config{RootEmail: "test@example.com"}
	mockContactService := mocks.NewMockContactService(ctrl)
	mockListService := mocks.NewMockListService(ctrl)
	mockContactListService := mocks.NewMockContactListService(ctrl)
	mockTemplateService := mocks.NewMockTemplateService(ctrl)
	mockWebhookRegService := mocks.NewMockWebhookRegistrationService(ctrl)

	service := NewWorkspaceService(
		mockRepo,
		mockUserRepo,
		mocks.NewMockTaskRepository(ctrl),
		mockLogger,
		mockUserService,
		mockAuthService,
		mockMailer,
		mockConfig,
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
	workspaceID := "testworkspace"
	userID := "testuser"
	integrationID := "integration123"
	integrationName := "Updated SMTP Integration"

	provider := domain.EmailProvider{
		Kind:               domain.EmailProviderKindSMTP,
		RateLimitPerMinute: 25,
		SMTP: &domain.SMTPSettings{
			Host:     "smtp.updated.com",
			Port:     587,
			Username: "updated_user",
			Password: "updated_password",
			UseTLS:   true,
		},
		Senders: []domain.EmailSender{
			domain.NewEmailSender("updated@example.com", "Updated Sender"),
		},
	}

	t.Run("successful update integration", func(t *testing.T) {
		expectedUser := &domain.User{
			ID: userID,
		}

		expectedUserWorkspace := &domain.UserWorkspace{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Role:        "owner",
		}

		// Create a workspace with an existing integration
		existingIntegration := domain.Integration{
			ID:   integrationID,
			Name: "Original SMTP Integration",
			Type: domain.IntegrationTypeEmail,
			EmailProvider: domain.EmailProvider{
				Kind:               domain.EmailProviderKindSMTP,
				RateLimitPerMinute: 25,
				SMTP: &domain.SMTPSettings{
					Host:     "smtp.example.com",
					Port:     587,
					Username: "smtp_user",
					Password: "smtp_password",
					UseTLS:   true,
				},
				Senders: []domain.EmailSender{
					domain.NewEmailSender("test@example.com", "Test Sender"),
				},
			},
			CreatedAt: time.Now().Add(-24 * time.Hour), // Created 24 hours ago
			UpdatedAt: time.Now().Add(-24 * time.Hour),
		}

		expectedWorkspace := &domain.Workspace{
			ID:           workspaceID,
			Name:         "Test Workspace",
			Integrations: []domain.Integration{existingIntegration},
		}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, expectedUser, expectedUserWorkspace, nil)
		mockRepo.EXPECT().GetByID(ctx, workspaceID).Return(expectedWorkspace, nil)
		mockRepo.EXPECT().Update(ctx, gomock.Any()).DoAndReturn(func(ctx context.Context, workspace *domain.Workspace) error {
			// Verify the integration was updated in the workspace
			require.Equal(t, 1, len(workspace.Integrations))
			require.Equal(t, integrationID, workspace.Integrations[0].ID)
			require.Equal(t, integrationName, workspace.Integrations[0].Name)
			require.Equal(t, domain.IntegrationTypeEmail, workspace.Integrations[0].Type)
			require.Equal(t, domain.EmailProviderKindSMTP, workspace.Integrations[0].EmailProvider.Kind)
			require.Equal(t, "smtp.updated.com", workspace.Integrations[0].EmailProvider.SMTP.Host)
			require.Equal(t, "updated_user", workspace.Integrations[0].EmailProvider.SMTP.Username)
			require.Equal(t, existingIntegration.CreatedAt, workspace.Integrations[0].CreatedAt)      // CreatedAt should remain the same
			require.True(t, workspace.Integrations[0].UpdatedAt.After(existingIntegration.UpdatedAt)) // UpdatedAt should be updated
			return nil
		})

		err := service.UpdateIntegration(ctx, domain.UpdateIntegrationRequest{
			WorkspaceID:   workspaceID,
			IntegrationID: integrationID,
			Name:          integrationName,
			Provider:      provider,
		})
		require.NoError(t, err)
	})

	t.Run("unauthorized user", func(t *testing.T) {
		expectedUser := &domain.User{
			ID: userID,
		}

		// User is a member, not an owner
		expectedUserWorkspace := &domain.UserWorkspace{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Role:        "member",
		}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, expectedUser, expectedUserWorkspace, nil)

		err := service.UpdateIntegration(ctx, domain.UpdateIntegrationRequest{
			WorkspaceID:   workspaceID,
			IntegrationID: integrationID,
			Name:          integrationName,
			Provider:      provider,
		})
		require.Error(t, err)
		require.IsType(t, &domain.ErrUnauthorized{}, err)
	})

	t.Run("integration not found", func(t *testing.T) {
		expectedUser := &domain.User{
			ID: userID,
		}

		expectedUserWorkspace := &domain.UserWorkspace{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Role:        "owner",
		}

		// Create a workspace with no integrations
		expectedWorkspace := &domain.Workspace{
			ID:   workspaceID,
			Name: "Test Workspace",
		}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, expectedUser, expectedUserWorkspace, nil)
		mockRepo.EXPECT().GetByID(ctx, workspaceID).Return(expectedWorkspace, nil)

		err := service.UpdateIntegration(ctx, domain.UpdateIntegrationRequest{
			WorkspaceID:   workspaceID,
			IntegrationID: integrationID,
			Name:          integrationName,
			Provider:      provider,
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "integration not found")
	})

	t.Run("successful update firecrawl integration preserves API key", func(t *testing.T) {
		firecrawlIntegrationID := "firecrawl123"
		expectedUser := &domain.User{
			ID: userID,
		}

		expectedUserWorkspace := &domain.UserWorkspace{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Role:        "owner",
		}

		// Create a workspace with an existing Firecrawl integration
		existingIntegration := domain.Integration{
			ID:   firecrawlIntegrationID,
			Name: "Original Firecrawl",
			Type: domain.IntegrationTypeFirecrawl,
			FirecrawlSettings: &domain.FirecrawlSettings{
				EncryptedAPIKey: "encrypted-existing-key",
				BaseURL:         "https://custom.firecrawl.dev",
			},
			CreatedAt: time.Now().Add(-24 * time.Hour),
			UpdatedAt: time.Now().Add(-24 * time.Hour),
		}

		expectedWorkspace := &domain.Workspace{
			ID:           workspaceID,
			Name:         "Test Workspace",
			Integrations: []domain.Integration{existingIntegration},
		}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, expectedUser, expectedUserWorkspace, nil)
		mockRepo.EXPECT().GetByID(ctx, workspaceID).Return(expectedWorkspace, nil)
		mockRepo.EXPECT().Update(ctx, gomock.Any()).DoAndReturn(func(ctx context.Context, workspace *domain.Workspace) error {
			// Verify the integration was updated
			require.Equal(t, 1, len(workspace.Integrations))
			require.Equal(t, firecrawlIntegrationID, workspace.Integrations[0].ID)
			require.Equal(t, "Updated Firecrawl", workspace.Integrations[0].Name)
			require.Equal(t, domain.IntegrationTypeFirecrawl, workspace.Integrations[0].Type)
			// API key should be preserved since no new key was provided
			require.Equal(t, "encrypted-existing-key", workspace.Integrations[0].FirecrawlSettings.EncryptedAPIKey)
			// BaseURL should be updated
			require.Equal(t, "https://new.firecrawl.dev", workspace.Integrations[0].FirecrawlSettings.BaseURL)
			return nil
		})

		err := service.UpdateIntegration(ctx, domain.UpdateIntegrationRequest{
			WorkspaceID:   workspaceID,
			IntegrationID: firecrawlIntegrationID,
			Name:          "Updated Firecrawl",
			FirecrawlSettings: &domain.FirecrawlSettings{
				APIKey:  "", // Empty - should preserve existing encrypted key
				BaseURL: "https://new.firecrawl.dev",
			},
		})
		require.NoError(t, err)
	})

	t.Run("successful update gemini LLM integration preserves API key", func(t *testing.T) {
		geminiIntegrationID := "gemini123"
		expectedUser := &domain.User{ID: userID}
		expectedUserWorkspace := &domain.UserWorkspace{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Role:        "owner",
		}

		// Existing Gemini LLM integration with an encrypted key
		existingIntegration := domain.Integration{
			ID:   geminiIntegrationID,
			Name: "Original Gemini",
			Type: domain.IntegrationTypeLLM,
			LLMProvider: &domain.LLMProvider{
				Kind: domain.LLMProviderKindGemini,
				Gemini: &domain.GeminiSettings{
					EncryptedAPIKey: "encrypted-existing-gemini-key",
					Model:           "gemini-2.5-flash",
				},
			},
			CreatedAt: time.Now().Add(-24 * time.Hour),
			UpdatedAt: time.Now().Add(-24 * time.Hour),
		}

		expectedWorkspace := &domain.Workspace{
			ID:           workspaceID,
			Name:         "Test Workspace",
			Integrations: []domain.Integration{existingIntegration},
		}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, expectedUser, expectedUserWorkspace, nil)
		mockRepo.EXPECT().GetByID(ctx, workspaceID).Return(expectedWorkspace, nil)
		mockRepo.EXPECT().Update(ctx, gomock.Any()).DoAndReturn(func(ctx context.Context, workspace *domain.Workspace) error {
			require.Equal(t, 1, len(workspace.Integrations))
			require.NotNil(t, workspace.Integrations[0].LLMProvider)
			require.NotNil(t, workspace.Integrations[0].LLMProvider.Gemini)
			// Model should be updated
			require.Equal(t, "gemini-3.1-pro-preview", workspace.Integrations[0].LLMProvider.Gemini.Model)
			// API key should be preserved since no new key was provided
			require.Equal(t, "encrypted-existing-gemini-key", workspace.Integrations[0].LLMProvider.Gemini.EncryptedAPIKey)
			return nil
		})

		err := service.UpdateIntegration(ctx, domain.UpdateIntegrationRequest{
			WorkspaceID:   workspaceID,
			IntegrationID: geminiIntegrationID,
			Name:          "Updated Gemini",
			LLMProvider: &domain.LLMProvider{
				Kind: domain.LLMProviderKindGemini,
				Gemini: &domain.GeminiSettings{
					APIKey: "", // Empty - should preserve existing encrypted key
					Model:  "gemini-3.1-pro-preview",
				},
			},
		})
		require.NoError(t, err)
	})

	t.Run("update openai LLM integration preserves key and carries reasoning_effort", func(t *testing.T) {
		openaiIntegrationID := "openai123"
		expectedUser := &domain.User{ID: userID}
		expectedUserWorkspace := &domain.UserWorkspace{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Role:        "owner",
		}

		// Existing OpenAI integration with an encrypted key and no effort set.
		existingIntegration := domain.Integration{
			ID:   openaiIntegrationID,
			Name: "Original OpenAI",
			Type: domain.IntegrationTypeLLM,
			LLMProvider: &domain.LLMProvider{
				Kind: domain.LLMProviderKindOpenAI,
				OpenAI: &domain.OpenAISettings{
					EncryptedAPIKey: "encrypted-existing-openai-key",
					Model:           "gpt-4.1",
				},
			},
			CreatedAt: time.Now().Add(-24 * time.Hour),
			UpdatedAt: time.Now().Add(-24 * time.Hour),
		}

		expectedWorkspace := &domain.Workspace{
			ID:           workspaceID,
			Name:         "Test Workspace",
			Integrations: []domain.Integration{existingIntegration},
		}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, expectedUser, expectedUserWorkspace, nil)
		mockRepo.EXPECT().GetByID(ctx, workspaceID).Return(expectedWorkspace, nil)
		mockRepo.EXPECT().Update(ctx, gomock.Any()).DoAndReturn(func(ctx context.Context, workspace *domain.Workspace) error {
			require.Equal(t, 1, len(workspace.Integrations))
			openai := workspace.Integrations[0].LLMProvider.OpenAI
			require.NotNil(t, openai)
			// API key preserved (none provided), and reasoning_effort persisted.
			require.Equal(t, "encrypted-existing-openai-key", openai.EncryptedAPIKey)
			require.Equal(t, "high", openai.ReasoningEffort)
			return nil
		})

		err := service.UpdateIntegration(ctx, domain.UpdateIntegrationRequest{
			WorkspaceID:   workspaceID,
			IntegrationID: openaiIntegrationID,
			Name:          "Updated OpenAI",
			LLMProvider: &domain.LLMProvider{
				Kind: domain.LLMProviderKindOpenAI,
				OpenAI: &domain.OpenAISettings{
					APIKey:          "", // Empty - should preserve existing encrypted key
					Model:           "gpt-4.1",
					ReasoningEffort: "high",
				},
			},
		})
		require.NoError(t, err)
	})

	t.Run("successful update firecrawl integration replaces API key", func(t *testing.T) {
		firecrawlIntegrationID := "firecrawl456"
		expectedUser := &domain.User{
			ID: userID,
		}

		expectedUserWorkspace := &domain.UserWorkspace{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Role:        "owner",
		}

		// Create a workspace with an existing Firecrawl integration
		existingIntegration := domain.Integration{
			ID:   firecrawlIntegrationID,
			Name: "Original Firecrawl",
			Type: domain.IntegrationTypeFirecrawl,
			FirecrawlSettings: &domain.FirecrawlSettings{
				EncryptedAPIKey: "encrypted-old-key",
			},
			CreatedAt: time.Now().Add(-24 * time.Hour),
			UpdatedAt: time.Now().Add(-24 * time.Hour),
		}

		expectedWorkspace := &domain.Workspace{
			ID:           workspaceID,
			Name:         "Test Workspace",
			Integrations: []domain.Integration{existingIntegration},
		}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, expectedUser, expectedUserWorkspace, nil)
		mockRepo.EXPECT().GetByID(ctx, workspaceID).Return(expectedWorkspace, nil)
		mockRepo.EXPECT().Update(ctx, gomock.Any()).DoAndReturn(func(ctx context.Context, workspace *domain.Workspace) error {
			// Verify the integration was updated
			require.Equal(t, 1, len(workspace.Integrations))
			require.Equal(t, firecrawlIntegrationID, workspace.Integrations[0].ID)
			// New API key should be encrypted (different from old encrypted key)
			require.NotEmpty(t, workspace.Integrations[0].FirecrawlSettings.EncryptedAPIKey)
			require.NotEqual(t, "encrypted-old-key", workspace.Integrations[0].FirecrawlSettings.EncryptedAPIKey)
			// Plain key should be cleared
			require.Empty(t, workspace.Integrations[0].FirecrawlSettings.APIKey)
			return nil
		})

		err := service.UpdateIntegration(ctx, domain.UpdateIntegrationRequest{
			WorkspaceID:   workspaceID,
			IntegrationID: firecrawlIntegrationID,
			Name:          "Updated Firecrawl",
			FirecrawlSettings: &domain.FirecrawlSettings{
				APIKey: "fc-new-api-key", // New key provided - should be encrypted
			},
		})
		require.NoError(t, err)
	})

	t.Run("renaming a zapier integration keeps its minted address", func(t *testing.T) {
		zapierIntegrationID := "zapier789"
		expectedUser := &domain.User{ID: userID}
		expectedUserWorkspace := &domain.UserWorkspace{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Role:        "owner",
		}

		existingIntegration := domain.Integration{
			ID:   zapierIntegrationID,
			Name: "Support Zaps",
			Type: domain.IntegrationTypeZapier,
			ZapierSettings: &domain.ZapierSettings{
				APIKeyEmail: "zapier-support-3f9a1c02@api.example.com",
			},
			CreatedAt: time.Now().Add(-24 * time.Hour),
			UpdatedAt: time.Now().Add(-24 * time.Hour),
		}

		expectedWorkspace := &domain.Workspace{
			ID:           workspaceID,
			Name:         "Test Workspace",
			Integrations: []domain.Integration{existingIntegration},
		}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, expectedUser, expectedUserWorkspace, nil)
		mockRepo.EXPECT().GetByID(ctx, workspaceID).Return(expectedWorkspace, nil)
		mockRepo.EXPECT().Update(ctx, gomock.Any()).DoAndReturn(func(ctx context.Context, workspace *domain.Workspace) error {
			require.Equal(t, 1, len(workspace.Integrations))
			updated := workspace.Integrations[0]
			require.Equal(t, "Marketing Zaps", updated.Name)
			require.NotNil(t, updated.ZapierSettings)
			// The address is server-owned and immutable, so a renamed card keeps the address
			// it was minted with and the two diverge by design.
			require.Equal(t, "zapier-support-3f9a1c02@api.example.com", updated.ZapierSettings.APIKeyEmail)
			return nil
		})

		// No settings on the request, because UpdateIntegrationRequest has no field to put
		// them in. This is what every zapier update looks like.
		err := service.UpdateIntegration(ctx, domain.UpdateIntegrationRequest{
			WorkspaceID:   workspaceID,
			IntegrationID: zapierIntegrationID,
			Name:          "Marketing Zaps",
		})
		require.NoError(t, err)
	})

	t.Run("update firecrawl integration with no settings preserves existing", func(t *testing.T) {
		// The else branch of the firecrawl case, which the preserve-on-blank sibling above
		// never reaches: an omitted settings object rather than a blank key.
		firecrawlIntegrationID := "firecrawl-nil-settings"
		existingIntegration := domain.Integration{
			ID:                firecrawlIntegrationID,
			Name:              "Original Firecrawl",
			Type:              domain.IntegrationTypeFirecrawl,
			FirecrawlSettings: &domain.FirecrawlSettings{EncryptedAPIKey: "encrypted-kept-key"},
			CreatedAt:         time.Now().Add(-24 * time.Hour),
			UpdatedAt:         time.Now().Add(-24 * time.Hour),
		}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{ID: userID}, &domain.UserWorkspace{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Role:        "owner",
		}, nil)
		mockRepo.EXPECT().GetByID(ctx, workspaceID).Return(&domain.Workspace{
			ID:           workspaceID,
			Name:         "Test Workspace",
			Integrations: []domain.Integration{existingIntegration},
		}, nil)
		mockRepo.EXPECT().Update(ctx, gomock.Any()).DoAndReturn(func(ctx context.Context, workspace *domain.Workspace) error {
			require.NotNil(t, workspace.Integrations[0].FirecrawlSettings)
			require.Equal(t, "encrypted-kept-key", workspace.Integrations[0].FirecrawlSettings.EncryptedAPIKey)
			return nil
		})

		err := service.UpdateIntegration(ctx, domain.UpdateIntegrationRequest{
			WorkspaceID:   workspaceID,
			IntegrationID: firecrawlIntegrationID,
			Name:          "Renamed Firecrawl",
		})
		require.NoError(t, err)
	})

	t.Run("update llm integration with no settings preserves existing", func(t *testing.T) {
		llmIntegrationID := "llm-nil-settings"
		existingIntegration := domain.Integration{
			ID:   llmIntegrationID,
			Name: "Original Gemini",
			Type: domain.IntegrationTypeLLM,
			LLMProvider: &domain.LLMProvider{
				Kind: domain.LLMProviderKindGemini,
				Gemini: &domain.GeminiSettings{
					EncryptedAPIKey: "encrypted-kept-gemini-key",
					Model:           "gemini-2.5-flash",
				},
			},
			CreatedAt: time.Now().Add(-24 * time.Hour),
			UpdatedAt: time.Now().Add(-24 * time.Hour),
		}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{ID: userID}, &domain.UserWorkspace{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Role:        "owner",
		}, nil)
		mockRepo.EXPECT().GetByID(ctx, workspaceID).Return(&domain.Workspace{
			ID:           workspaceID,
			Name:         "Test Workspace",
			Integrations: []domain.Integration{existingIntegration},
		}, nil)
		mockRepo.EXPECT().Update(ctx, gomock.Any()).DoAndReturn(func(ctx context.Context, workspace *domain.Workspace) error {
			require.NotNil(t, workspace.Integrations[0].LLMProvider)
			require.NotNil(t, workspace.Integrations[0].LLMProvider.Gemini)
			require.Equal(t, "encrypted-kept-gemini-key", workspace.Integrations[0].LLMProvider.Gemini.EncryptedAPIKey)
			return nil
		})

		err := service.UpdateIntegration(ctx, domain.UpdateIntegrationRequest{
			WorkspaceID:   workspaceID,
			IntegrationID: llmIntegrationID,
			Name:          "Renamed Gemini",
		})
		require.NoError(t, err)
	})
}

func TestWorkspaceService_DeleteIntegration(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockUserRepo := mocks.NewMockUserRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockUserService := mocks.NewMockUserServiceInterface(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)
	mockMailer := pkgmocks.NewMockMailer(ctrl)
	mockConfig := &config.Config{RootEmail: "test@example.com"}
	mockContactService := mocks.NewMockContactService(ctrl)
	mockListService := mocks.NewMockListService(ctrl)
	mockContactListService := mocks.NewMockContactListService(ctrl)
	mockTemplateService := mocks.NewMockTemplateService(ctrl)
	mockWebhookRegService := mocks.NewMockWebhookRegistrationService(ctrl)

	service := NewWorkspaceService(
		mockRepo,
		mockUserRepo,
		mocks.NewMockTaskRepository(ctrl),
		mockLogger,
		mockUserService,
		mockAuthService,
		mockMailer,
		mockConfig,
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
	workspaceID := "testworkspace"
	userID := "testuser"
	integrationID := "integration123"

	t.Run("successful delete integration", func(t *testing.T) {
		expectedUser := &domain.User{
			ID: userID,
		}

		expectedUserWorkspace := &domain.UserWorkspace{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Role:        "owner",
		}

		// Create a workspace with an existing integration
		existingIntegration := domain.Integration{
			ID:   integrationID,
			Name: "SMTP Integration",
			Type: domain.IntegrationTypeEmail,
			EmailProvider: domain.EmailProvider{
				Kind: domain.EmailProviderKindSMTP,
				SMTP: &domain.SMTPSettings{
					Host:     "smtp.example.com",
					Port:     587,
					Username: "smtp_user",
					Password: "smtp_password",
					UseTLS:   true,
				},
			},
		}

		expectedWorkspace := &domain.Workspace{
			ID:   workspaceID,
			Name: "Test Workspace",
			Settings: domain.WorkspaceSettings{
				DefaultLanguage:              "en",
				Languages:                    []string{"en"},
				TransactionalEmailProviderID: integrationID, // Reference the integration
			},
			Integrations: []domain.Integration{existingIntegration},
		}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, expectedUser, expectedUserWorkspace, nil)
		mockRepo.EXPECT().GetByID(ctx, workspaceID).Return(expectedWorkspace, nil)

		// No webhook operations expected for SMTP provider

		mockRepo.EXPECT().Update(ctx, gomock.Any()).DoAndReturn(func(ctx context.Context, workspace *domain.Workspace) error {
			// Verify the integration was removed from the workspace
			require.Empty(t, workspace.Integrations)
			// Verify the reference was removed from settings
			require.Empty(t, workspace.Settings.TransactionalEmailProviderID)
			return nil
		})

		err := service.DeleteIntegration(ctx, workspaceID, integrationID)
		require.NoError(t, err)
	})

	t.Run("unauthorized user", func(t *testing.T) {
		expectedUser := &domain.User{
			ID: userID,
		}

		// User is a member, not an owner
		expectedUserWorkspace := &domain.UserWorkspace{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Role:        "member",
		}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, expectedUser, expectedUserWorkspace, nil)

		err := service.DeleteIntegration(ctx, workspaceID, integrationID)
		require.Error(t, err)
		require.IsType(t, &domain.ErrUnauthorized{}, err)
	})

	t.Run("integration not found", func(t *testing.T) {
		expectedUser := &domain.User{
			ID: userID,
		}

		expectedUserWorkspace := &domain.UserWorkspace{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Role:        "owner",
		}

		// Create a workspace with no integrations
		expectedWorkspace := &domain.Workspace{
			ID:   workspaceID,
			Name: "Test Workspace",
		}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, expectedUser, expectedUserWorkspace, nil)
		mockRepo.EXPECT().GetByID(ctx, workspaceID).Return(expectedWorkspace, nil)

		err := service.DeleteIntegration(ctx, workspaceID, integrationID)
		require.Error(t, err)
		require.Contains(t, err.Error(), "integration not found")
	})

	t.Run("webhook unregistration error", func(t *testing.T) {
		expectedUser := &domain.User{
			ID: userID,
		}

		expectedUserWorkspace := &domain.UserWorkspace{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Role:        "owner",
		}

		// Create a workspace with an existing integration
		existingIntegration := domain.Integration{
			ID:   integrationID,
			Name: "SMTP Integration",
			Type: domain.IntegrationTypeEmail,
			EmailProvider: domain.EmailProvider{
				Kind: domain.EmailProviderKindSMTP,
				SMTP: &domain.SMTPSettings{
					Host:     "smtp.example.com",
					Port:     587,
					Username: "smtp_user",
					Password: "smtp_password",
					UseTLS:   true,
				},
			},
		}

		expectedWorkspace := &domain.Workspace{
			ID:           workspaceID,
			Name:         "Test Workspace",
			Integrations: []domain.Integration{existingIntegration},
		}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, expectedUser, expectedUserWorkspace, nil)
		mockRepo.EXPECT().GetByID(ctx, workspaceID).Return(expectedWorkspace, nil)

		// No webhook operations expected for SMTP provider

		mockRepo.EXPECT().Update(ctx, gomock.Any()).Return(nil)

		err := service.DeleteIntegration(ctx, workspaceID, integrationID)
		require.NoError(t, err) // Should still succeed despite webhook unregistration error
	})

	t.Run("removes marketing reference", func(t *testing.T) {
		expectedUser := &domain.User{
			ID: userID,
		}

		expectedUserWorkspace := &domain.UserWorkspace{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Role:        "owner",
		}

		// Create a workspace with an existing integration
		existingIntegration := domain.Integration{
			ID:   integrationID,
			Name: "SMTP Integration",
			Type: domain.IntegrationTypeEmail,
			EmailProvider: domain.EmailProvider{
				Kind: domain.EmailProviderKindSMTP,
			},
		}

		expectedWorkspace := &domain.Workspace{
			ID:   workspaceID,
			Name: "Test Workspace",
			Settings: domain.WorkspaceSettings{
				DefaultLanguage:          "en",
				Languages:                []string{"en"},
				MarketingEmailProviderID: integrationID, // Reference the integration as marketing provider
			},
			Integrations: []domain.Integration{existingIntegration},
		}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, expectedUser, expectedUserWorkspace, nil)
		mockRepo.EXPECT().GetByID(ctx, workspaceID).Return(expectedWorkspace, nil)

		// No webhook operations expected for SMTP provider

		mockRepo.EXPECT().Update(ctx, gomock.Any()).DoAndReturn(func(ctx context.Context, workspace *domain.Workspace) error {
			// Verify the reference was removed from settings
			require.Empty(t, workspace.Settings.MarketingEmailProviderID)
			return nil
		})

		err := service.DeleteIntegration(ctx, workspaceID, integrationID)
		require.NoError(t, err)
	})

	t.Run("deleting a zapier integration revokes its API key", func(t *testing.T) {
		zapierIntegrationID := "zapier-delete"
		apiKeyEmail := "zapier-support-3f9a1c02@api.example.com"
		apiKeyUserID := "api-key-user-1"

		existingIntegration := domain.Integration{
			ID:             zapierIntegrationID,
			Name:           "Support Zaps",
			Type:           domain.IntegrationTypeZapier,
			ZapierSettings: &domain.ZapierSettings{APIKeyEmail: apiKeyEmail},
		}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{ID: userID}, &domain.UserWorkspace{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Role:        "owner",
		}, nil)
		mockRepo.EXPECT().GetByID(ctx, workspaceID).Return(&domain.Workspace{
			ID:           workspaceID,
			Name:         "Test Workspace",
			Integrations: []domain.Integration{existingIntegration},
		}, nil)

		// The address is the only handle the record keeps on the key.
		mockUserRepo.EXPECT().GetUserByEmail(ctx, apiKeyEmail).Return(&domain.User{
			ID:    apiKeyUserID,
			Email: apiKeyEmail,
			Type:  domain.UserTypeAPIKey,
		}, nil)

		removed := false
		mockRepo.EXPECT().RemoveUserFromWorkspace(ctx, apiKeyUserID, workspaceID).DoAndReturn(func(_ context.Context, _ string, _ string) error {
			removed = true
			return nil
		})
		deleted := false
		mockUserRepo.EXPECT().Delete(ctx, apiKeyUserID).DoAndReturn(func(_ context.Context, _ string) error {
			// Both halves, in this order: deleting the user is the whole of the revocation,
			// and a membership row left behind would be unremovable afterwards.
			assert.True(t, removed, "the membership must be removed before the user")
			deleted = true
			return nil
		})

		mockRepo.EXPECT().Update(ctx, gomock.Any()).DoAndReturn(func(ctx context.Context, workspace *domain.Workspace) error {
			require.Empty(t, workspace.Integrations)
			return nil
		})

		err := service.DeleteIntegration(ctx, workspaceID, zapierIntegrationID)
		require.NoError(t, err)
		assert.True(t, removed)
		assert.True(t, deleted)
	})

	t.Run("a zapier integration survives a failed revocation", func(t *testing.T) {
		// Unlike its neighbours in the same switch, this one refuses to remove the card: the
		// confirmation the user answered promises the key is revoked, and deleting the card
		// anyway would report a revocation that never happened with nothing left to retry from.
		zapierIntegrationID := "zapier-revoke-fails"
		apiKeyEmail := "zapier-ops-11223344@api.example.com"

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{ID: userID}, &domain.UserWorkspace{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Role:        "owner",
		}, nil)
		mockRepo.EXPECT().GetByID(ctx, workspaceID).Return(&domain.Workspace{
			ID:   workspaceID,
			Name: "Test Workspace",
			Integrations: []domain.Integration{{
				ID:             zapierIntegrationID,
				Name:           "Ops Zaps",
				Type:           domain.IntegrationTypeZapier,
				ZapierSettings: &domain.ZapierSettings{APIKeyEmail: apiKeyEmail},
			}},
		}, nil)
		mockUserRepo.EXPECT().GetUserByEmail(ctx, apiKeyEmail).Return(&domain.User{ID: "api-key-user-2", Email: apiKeyEmail}, nil)
		mockRepo.EXPECT().RemoveUserFromWorkspace(ctx, "api-key-user-2", workspaceID).Return(fmt.Errorf("membership removal failed"))
		// The row is still there, so the failure was a real one and not the half-finished
		// revocation the tolerance below it exists for.
		mockRepo.EXPECT().IsUserWorkspaceMember(ctx, "api-key-user-2", workspaceID).Return(true, nil).AnyTimes()
		// No Update expectation: writing the workspace back would be the defect.

		err := service.DeleteIntegration(ctx, workspaceID, zapierIntegrationID)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "membership removal failed")
	})

	t.Run("a zapier integration whose key is already gone still deletes", func(t *testing.T) {
		// An owner may revoke the key from Settings → Team, which leaves the card pointing at
		// an address no user row answers to. That card has to stay deletable.
		zapierIntegrationID := "zapier-orphan"
		apiKeyEmail := "zapier-stale-99887766@api.example.com"

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{ID: userID}, &domain.UserWorkspace{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Role:        "owner",
		}, nil)
		mockRepo.EXPECT().GetByID(ctx, workspaceID).Return(&domain.Workspace{
			ID:   workspaceID,
			Name: "Test Workspace",
			Integrations: []domain.Integration{{
				ID:             zapierIntegrationID,
				Name:           "Stale Zaps",
				Type:           domain.IntegrationTypeZapier,
				ZapierSettings: &domain.ZapierSettings{APIKeyEmail: apiKeyEmail},
			}},
		}, nil)
		mockUserRepo.EXPECT().GetUserByEmail(ctx, apiKeyEmail).Return(nil, &domain.ErrUserNotFound{Message: "user not found"})
		mockRepo.EXPECT().Update(ctx, gomock.Any()).DoAndReturn(func(ctx context.Context, workspace *domain.Workspace) error {
			require.Empty(t, workspace.Integrations)
			return nil
		})

		err := service.DeleteIntegration(ctx, workspaceID, zapierIntegrationID)
		require.NoError(t, err)
	})

	// The revocation is two writes with no transaction around them, so it can stop in the
	// middle: the membership goes first, and a failure on the delete that follows leaves the
	// user row behind with its membership already gone. Every later attempt re-enters the
	// revocation, and the user it now finds has no membership left to remove — so refusing
	// there would wedge the card permanently, together with the live key it points at, which
	// the roster cannot show either because it inner-joins user_workspaces.
	t.Run("a revocation interrupted between its two writes can still be finished", func(t *testing.T) {
		zapierIntegrationID := "zapier-half-revoked"
		apiKeyEmail := "zapier-half-55667788@api.example.com"
		apiKeyUserID := "api-key-user-3"

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{ID: userID}, &domain.UserWorkspace{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Role:        "owner",
		}, nil)
		mockRepo.EXPECT().GetByID(ctx, workspaceID).Return(&domain.Workspace{
			ID:   workspaceID,
			Name: "Test Workspace",
			Integrations: []domain.Integration{{
				ID:             zapierIntegrationID,
				Name:           "Half Zaps",
				Type:           domain.IntegrationTypeZapier,
				ZapierSettings: &domain.ZapierSettings{APIKeyEmail: apiKeyEmail},
			}},
		}, nil)

		// The user row outlived the interrupted attempt, so the ErrUserNotFound tolerance
		// that covers the opposite drift never fires.
		mockUserRepo.EXPECT().GetUserByEmail(ctx, apiKeyEmail).Return(&domain.User{
			ID:    apiKeyUserID,
			Email: apiKeyEmail,
			Type:  domain.UserTypeAPIKey,
		}, nil).AnyTimes()
		// What the DELETE reports when it matches no row.
		mockRepo.EXPECT().RemoveUserFromWorkspace(ctx, apiKeyUserID, workspaceID).
			Return(fmt.Errorf("user is not a member of the workspace")).AnyTimes()
		mockRepo.EXPECT().IsUserWorkspaceMember(ctx, apiKeyUserID, workspaceID).Return(false, nil).AnyTimes()

		deleted := false
		mockUserRepo.EXPECT().Delete(ctx, apiKeyUserID).DoAndReturn(func(_ context.Context, _ string) error {
			deleted = true
			return nil
		}).AnyTimes()

		var written *domain.Workspace
		mockRepo.EXPECT().Update(ctx, gomock.Any()).DoAndReturn(func(_ context.Context, workspace *domain.Workspace) error {
			written = workspace
			return nil
		}).AnyTimes()

		err := service.DeleteIntegration(ctx, workspaceID, zapierIntegrationID)
		require.NoError(t, err)
		assert.True(t, deleted, "the key the card points at is still live, and deleting the user is the whole of the revocation")
		require.NotNil(t, written, "the card has to become removable again")
		assert.Empty(t, written.Integrations)
	})
}

func TestWorkspaceService_RemoveMember(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockUserRepo := mocks.NewMockUserRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockUserService := mocks.NewMockUserServiceInterface(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)
	mockMailer := pkgmocks.NewMockMailer(ctrl)
	mockConfig := &config.Config{RootEmail: "test@example.com"}
	mockContactService := mocks.NewMockContactService(ctrl)
	mockListService := mocks.NewMockListService(ctrl)
	mockContactListService := mocks.NewMockContactListService(ctrl)
	mockTemplateService := mocks.NewMockTemplateService(ctrl)
	mockWebhookRegService := mocks.NewMockWebhookRegistrationService(ctrl)

	service := NewWorkspaceService(
		mockRepo,
		mockUserRepo,
		mocks.NewMockTaskRepository(ctrl),
		mockLogger,
		mockUserService,
		mockAuthService,
		mockMailer,
		mockConfig,
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
	workspaceID := "test-workspace"
	ownerID := "owner-user"
	memberID := "member-user"
	apiKeyID := "api-key-user"

	t.Run("successful removal of regular member", func(t *testing.T) {
		owner := &domain.User{ID: ownerID, Type: domain.UserTypeUser}
		member := &domain.User{ID: memberID, Type: domain.UserTypeUser}
		ownerWorkspace := &domain.UserWorkspace{
			UserID:      ownerID,
			WorkspaceID: workspaceID,
			Role:        "owner",
		}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, owner, ownerWorkspace, nil)
		mockUserService.EXPECT().GetUserByID(ctx, memberID).Return(member, nil)
		mockRepo.EXPECT().RemoveUserFromWorkspace(ctx, memberID, workspaceID).Return(nil)

		err := service.RemoveMember(ctx, workspaceID, memberID)
		assert.NoError(t, err)
	})

	t.Run("successful removal of API key member", func(t *testing.T) {
		owner := &domain.User{ID: ownerID, Type: domain.UserTypeUser}
		apiKeyUser := &domain.User{ID: apiKeyID, Type: domain.UserTypeAPIKey}
		ownerWorkspace := &domain.UserWorkspace{
			UserID:      ownerID,
			WorkspaceID: workspaceID,
			Role:        "owner",
		}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, owner, ownerWorkspace, nil)
		mockUserService.EXPECT().GetUserByID(ctx, apiKeyID).Return(apiKeyUser, nil)
		mockRepo.EXPECT().RemoveUserFromWorkspace(ctx, apiKeyID, workspaceID).Return(nil)
		mockUserRepo.EXPECT().Delete(ctx, apiKeyID).Return(nil)
		mockLogger.EXPECT().WithField("user_id", apiKeyID).Return(mockLogger)
		mockLogger.EXPECT().Info("API key user deleted successfully")

		err := service.RemoveMember(ctx, workspaceID, apiKeyID)
		assert.NoError(t, err)
	})

	t.Run("authentication failure", func(t *testing.T) {
		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, nil, nil, errors.New("auth error"))

		err := service.RemoveMember(ctx, workspaceID, memberID)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to authenticate user")
	})

	t.Run("requester is not owner", func(t *testing.T) {
		member := &domain.User{ID: memberID, Type: domain.UserTypeUser}
		memberWorkspace := &domain.UserWorkspace{
			UserID:      memberID,
			WorkspaceID: workspaceID,
			Role:        "member",
		}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, member, memberWorkspace, nil)
		mockLogger.EXPECT().WithField("workspace_id", workspaceID).Return(mockLogger)
		mockLogger.EXPECT().WithField("user_id", memberID).Return(mockLogger)
		mockLogger.EXPECT().WithField("requester_id", memberID).Return(mockLogger)
		mockLogger.EXPECT().WithField("role", "member").Return(mockLogger)
		mockLogger.EXPECT().Error("Requester is not an owner of the workspace")

		err := service.RemoveMember(ctx, workspaceID, memberID)
		assert.Error(t, err)
		assert.IsType(t, &domain.ErrUnauthorized{}, err)
	})

	t.Run("cannot remove self", func(t *testing.T) {
		owner := &domain.User{ID: ownerID, Type: domain.UserTypeUser}
		ownerWorkspace := &domain.UserWorkspace{
			UserID:      ownerID,
			WorkspaceID: workspaceID,
			Role:        "owner",
		}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, owner, ownerWorkspace, nil)
		mockLogger.EXPECT().WithField("workspace_id", workspaceID).Return(mockLogger)
		mockLogger.EXPECT().WithField("user_id", ownerID).Return(mockLogger)
		mockLogger.EXPECT().Error("Cannot remove self from workspace")

		err := service.RemoveMember(ctx, workspaceID, ownerID)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "cannot remove yourself from the workspace")
	})

	t.Run("error getting user details", func(t *testing.T) {
		owner := &domain.User{ID: ownerID, Type: domain.UserTypeUser}
		ownerWorkspace := &domain.UserWorkspace{
			UserID:      ownerID,
			WorkspaceID: workspaceID,
			Role:        "owner",
		}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, owner, ownerWorkspace, nil)
		mockUserService.EXPECT().GetUserByID(ctx, memberID).Return(nil, errors.New("user not found"))
		mockLogger.EXPECT().WithField("user_id", memberID).Return(mockLogger)
		mockLogger.EXPECT().WithField("error", "user not found").Return(mockLogger)
		mockLogger.EXPECT().Error("Failed to get user details")

		err := service.RemoveMember(ctx, workspaceID, memberID)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "user not found")
	})

	t.Run("error removing user from workspace", func(t *testing.T) {
		owner := &domain.User{ID: ownerID, Type: domain.UserTypeUser}
		member := &domain.User{ID: memberID, Type: domain.UserTypeUser}
		ownerWorkspace := &domain.UserWorkspace{
			UserID:      ownerID,
			WorkspaceID: workspaceID,
			Role:        "owner",
		}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, owner, ownerWorkspace, nil)
		mockUserService.EXPECT().GetUserByID(ctx, memberID).Return(member, nil)
		mockRepo.EXPECT().RemoveUserFromWorkspace(ctx, memberID, workspaceID).Return(errors.New("remove error"))
		mockLogger.EXPECT().WithField("workspace_id", workspaceID).Return(mockLogger)
		mockLogger.EXPECT().WithField("user_id", memberID).Return(mockLogger)
		mockLogger.EXPECT().WithField("error", "remove error").Return(mockLogger)
		mockLogger.EXPECT().Error("Failed to remove user from workspace")

		err := service.RemoveMember(ctx, workspaceID, memberID)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "remove error")
	})

	t.Run("failed API key deletion is reported", func(t *testing.T) {
		// Deleting the user row is the only thing that revokes the key's ten-year token,
		// so a failure here must not be reported as a successful revocation.
		owner := &domain.User{ID: ownerID, Type: domain.UserTypeUser}
		apiKeyUser := &domain.User{ID: apiKeyID, Type: domain.UserTypeAPIKey}
		ownerWorkspace := &domain.UserWorkspace{
			UserID:      ownerID,
			WorkspaceID: workspaceID,
			Role:        "owner",
		}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, owner, ownerWorkspace, nil)
		mockUserService.EXPECT().GetUserByID(ctx, apiKeyID).Return(apiKeyUser, nil)
		mockRepo.EXPECT().RemoveUserFromWorkspace(ctx, apiKeyID, workspaceID).Return(nil)
		mockUserRepo.EXPECT().Delete(ctx, apiKeyID).Return(errors.New("delete error"))
		mockLogger.EXPECT().WithField("user_id", apiKeyID).Return(mockLogger)
		mockLogger.EXPECT().WithField("error", "delete error").Return(mockLogger)
		mockLogger.EXPECT().Error("Failed to delete API key user")

		err := service.RemoveMember(ctx, workspaceID, apiKeyID)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "delete error")
	})
}

func TestGenerateSecureKey(t *testing.T) {
	t.Run("generates key of expected length", func(t *testing.T) {
		// Test with different byte lengths
		byteLengths := []int{16, 32, 64}

		for _, byteLen := range byteLengths {
			// Each byte becomes 2 hex chars
			expectedHexLen := byteLen * 2

			// Generate the key
			key, err := GenerateSecureKey(byteLen)

			// Verify results
			require.NoError(t, err)
			assert.Len(t, key, expectedHexLen)

			// Verify it's valid hex
			_, err = hex.DecodeString(key)
			require.NoError(t, err, "Generated key is not valid hex")
		}
	})

	t.Run("generates unique keys", func(t *testing.T) {
		// Generate multiple keys to ensure uniqueness
		iterations := 10
		keys := make([]string, iterations)

		for i := 0; i < iterations; i++ {
			key, err := GenerateSecureKey(32)
			require.NoError(t, err)
			keys[i] = key
		}

		// Check for duplicates
		seen := make(map[string]bool)
		for _, key := range keys {
			assert.False(t, seen[key], "Duplicate key generated")
			seen[key] = true
		}
	})
}

func TestWorkspaceService_GetInvitationByID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockUserRepo := mocks.NewMockUserRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockUserService := mocks.NewMockUserServiceInterface(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)
	mockMailer := pkgmocks.NewMockMailer(ctrl)
	mockConfig := &config.Config{RootEmail: "test@example.com"}
	mockContactService := mocks.NewMockContactService(ctrl)
	mockListService := mocks.NewMockListService(ctrl)
	mockContactListService := mocks.NewMockContactListService(ctrl)
	mockTemplateService := mocks.NewMockTemplateService(ctrl)
	mockWebhookRegService := mocks.NewMockWebhookRegistrationService(ctrl)

	service := NewWorkspaceService(
		mockRepo,
		mockUserRepo,
		mocks.NewMockTaskRepository(ctrl),
		mockLogger,
		mockUserService,
		mockAuthService,
		mockMailer,
		mockConfig,
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

	invitationID := "invitation-123"
	invitation := &domain.WorkspaceInvitation{
		ID:          invitationID,
		WorkspaceID: "workspace-123",
		InviterID:   "inviter-123",
		Email:       "test@example.com",
		ExpiresAt:   time.Now().Add(24 * time.Hour),
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	t.Run("successful retrieval", func(t *testing.T) {
		mockRepo.EXPECT().
			GetInvitationByID(context.Background(), invitationID).
			Return(invitation, nil)

		result, err := service.GetInvitationByID(context.Background(), invitationID)

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, invitationID, result.ID)
		assert.Equal(t, invitation.WorkspaceID, result.WorkspaceID)
		assert.Equal(t, invitation.Email, result.Email)
	})

	t.Run("invitation not found", func(t *testing.T) {
		mockRepo.EXPECT().
			GetInvitationByID(context.Background(), "non-existent").
			Return(nil, errors.New("invitation not found"))

		result, err := service.GetInvitationByID(context.Background(), "non-existent")

		require.Error(t, err)
		require.Nil(t, result)
		assert.Contains(t, err.Error(), "invitation not found")
	})
}

func TestWorkspaceService_AcceptInvitation(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockUserRepo := mocks.NewMockUserRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockUserService := mocks.NewMockUserServiceInterface(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)
	mockMailer := pkgmocks.NewMockMailer(ctrl)
	mockConfig := &config.Config{RootEmail: "test@example.com"}
	mockContactService := mocks.NewMockContactService(ctrl)
	mockListService := mocks.NewMockListService(ctrl)
	mockContactListService := mocks.NewMockContactListService(ctrl)
	mockTemplateService := mocks.NewMockTemplateService(ctrl)
	mockWebhookRegService := mocks.NewMockWebhookRegistrationService(ctrl)

	service := NewWorkspaceService(
		mockRepo,
		mockUserRepo,
		mocks.NewMockTaskRepository(ctrl),
		mockLogger,
		mockUserService,
		mockAuthService,
		mockMailer,
		mockConfig,
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

	invitationID := "invitation-123"
	workspaceID := "workspace-123"
	email := "test@example.com"

	t.Run("successful acceptance with new user", func(t *testing.T) {
		// Mock invitation retrieval
		mockRepo.EXPECT().
			GetInvitationByID(context.Background(), invitationID).
			Return(&domain.WorkspaceInvitation{
				ID:          invitationID,
				Email:       email,
				WorkspaceID: workspaceID,
				Permissions: domain.UserPermissions{
					domain.PermissionResourceContacts: {Read: true, Write: true},
					domain.PermissionResourceLists:    {Read: true, Write: true},
				},
			}, nil)

		// User doesn't exist, should create new user
		mockUserService.EXPECT().
			GetUserByEmail(context.Background(), email).
			Return(nil, &domain.ErrUserNotFound{Message: "user not found"})

		// Mock user creation
		mockUserRepo.EXPECT().
			CreateUser(context.Background(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, user *domain.User) error {
				user.ID = "new-user-123" // Simulate ID assignment
				return nil
			})

		// Mock adding user to workspace
		mockRepo.EXPECT().
			AddUserToWorkspace(context.Background(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, userWorkspace *domain.UserWorkspace) error {
				assert.Equal(t, "new-user-123", userWorkspace.UserID)
				assert.Equal(t, workspaceID, userWorkspace.WorkspaceID)
				assert.Equal(t, "member", userWorkspace.Role)
				return nil
			})

		// Mock session creation
		mockUserRepo.EXPECT().
			CreateSession(context.Background(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, session *domain.Session) error {
				assert.Equal(t, "new-user-123", session.UserID)
				assert.NotEmpty(t, session.ID)
				return nil
			})

		// Mock auth token generation
		mockAuthService.EXPECT().
			GenerateUserAuthToken(gomock.Any(), gomock.Any(), gomock.Any()).
			Return("auth-token-123")

		// Mock invitation deletion
		mockRepo.EXPECT().
			DeleteInvitation(context.Background(), invitationID).
			Return(nil)

		// Mock logger calls
		mockLogger.EXPECT().
			WithField("user_id", "new-user-123").
			Return(mockLogger)
		mockLogger.EXPECT().
			WithField("email", email).
			Return(mockLogger)
		mockLogger.EXPECT().
			Info("Created new user from invitation acceptance")

		mockLogger.EXPECT().
			WithField("user_id", "new-user-123").
			Return(mockLogger)
		mockLogger.EXPECT().
			WithField("workspace_id", workspaceID).
			Return(mockLogger)
		mockLogger.EXPECT().
			WithField("invitation_id", invitationID).
			Return(mockLogger)
		mockLogger.EXPECT().
			Info("Successfully accepted invitation and created session")

		authResponse, err := service.AcceptInvitation(context.Background(), invitationID, workspaceID, email)

		require.NoError(t, err)
		require.NotNil(t, authResponse)
		assert.Equal(t, "auth-token-123", authResponse.Token)
		assert.Equal(t, "new-user-123", authResponse.User.ID)
		assert.Equal(t, email, authResponse.User.Email)
		assert.NotZero(t, authResponse.ExpiresAt)
	})

	t.Run("successful acceptance with existing user", func(t *testing.T) {
		existingUser := &domain.User{
			ID:    "existing-user-123",
			Email: email,
		}

		// Mock invitation retrieval
		mockRepo.EXPECT().
			GetInvitationByID(context.Background(), invitationID).
			Return(&domain.WorkspaceInvitation{
				ID:          invitationID,
				Email:       email,
				WorkspaceID: workspaceID,
				Permissions: domain.UserPermissions{
					domain.PermissionResourceContacts: {Read: true, Write: true},
					domain.PermissionResourceLists:    {Read: true, Write: true},
				},
			}, nil)

		// User exists
		mockUserService.EXPECT().
			GetUserByEmail(context.Background(), email).
			Return(existingUser, nil)

		// Check if user is already a member (not a member)
		mockRepo.EXPECT().
			IsUserWorkspaceMember(context.Background(), existingUser.ID, workspaceID).
			Return(false, nil)

		// Mock adding user to workspace
		mockRepo.EXPECT().
			AddUserToWorkspace(context.Background(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, userWorkspace *domain.UserWorkspace) error {
				assert.Equal(t, existingUser.ID, userWorkspace.UserID)
				assert.Equal(t, workspaceID, userWorkspace.WorkspaceID)
				assert.Equal(t, "member", userWorkspace.Role)
				return nil
			})

		// Mock session creation
		mockUserRepo.EXPECT().
			CreateSession(context.Background(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, session *domain.Session) error {
				assert.Equal(t, existingUser.ID, session.UserID)
				assert.NotEmpty(t, session.ID)
				return nil
			})

		// Mock auth token generation
		mockAuthService.EXPECT().
			GenerateUserAuthToken(gomock.Any(), gomock.Any(), gomock.Any()).
			Return("auth-token-456")

		// Mock invitation deletion
		mockRepo.EXPECT().
			DeleteInvitation(context.Background(), invitationID).
			Return(nil)

		// Mock logger call
		mockLogger.EXPECT().
			WithField("user_id", existingUser.ID).
			Return(mockLogger)
		mockLogger.EXPECT().
			WithField("workspace_id", workspaceID).
			Return(mockLogger)
		mockLogger.EXPECT().
			WithField("invitation_id", invitationID).
			Return(mockLogger)
		mockLogger.EXPECT().
			Info("Successfully accepted invitation and created session")

		authResponse, err := service.AcceptInvitation(context.Background(), invitationID, workspaceID, email)

		require.NoError(t, err)
		require.NotNil(t, authResponse)
		assert.Equal(t, "auth-token-456", authResponse.Token)
		assert.Equal(t, existingUser.ID, authResponse.User.ID)
		assert.Equal(t, email, authResponse.User.Email)
		assert.NotZero(t, authResponse.ExpiresAt)
	})

	t.Run("user already member", func(t *testing.T) {
		existingUser := &domain.User{
			ID:    "existing-user-123",
			Email: email,
		}

		// Mock invitation retrieval
		mockRepo.EXPECT().
			GetInvitationByID(context.Background(), invitationID).
			Return(&domain.WorkspaceInvitation{
				ID:          invitationID,
				Email:       email,
				WorkspaceID: workspaceID,
				Permissions: domain.UserPermissions{
					domain.PermissionResourceContacts: {Read: true, Write: true},
					domain.PermissionResourceLists:    {Read: true, Write: true},
				},
			}, nil)

		// User exists
		mockUserService.EXPECT().
			GetUserByEmail(context.Background(), email).
			Return(existingUser, nil)

		// Check if user is already a member (is a member)
		mockRepo.EXPECT().
			IsUserWorkspaceMember(context.Background(), existingUser.ID, workspaceID).
			Return(true, nil)

		// Mock invitation deletion (cleanup)
		mockRepo.EXPECT().
			DeleteInvitation(context.Background(), invitationID).
			Return(nil)

		// Mock logger calls
		mockLogger.EXPECT().
			WithField("user_id", existingUser.ID).
			Return(mockLogger)
		mockLogger.EXPECT().
			WithField("workspace_id", workspaceID).
			Return(mockLogger)
		mockLogger.EXPECT().
			Info("User is already a member of the workspace")

		authResponse, err := service.AcceptInvitation(context.Background(), invitationID, workspaceID, email)

		require.Error(t, err)
		assert.Nil(t, authResponse)
		assert.Contains(t, err.Error(), "user is already a member of the workspace")
	})

	t.Run("failed user creation", func(t *testing.T) {
		// Mock invitation retrieval
		mockRepo.EXPECT().
			GetInvitationByID(context.Background(), invitationID).
			Return(&domain.WorkspaceInvitation{
				ID:          invitationID,
				Email:       email,
				WorkspaceID: workspaceID,
				Permissions: domain.UserPermissions{
					domain.PermissionResourceContacts: {Read: true, Write: true},
					domain.PermissionResourceLists:    {Read: true, Write: true},
				},
			}, nil)

		// User doesn't exist, should create new user but fails
		mockUserService.EXPECT().
			GetUserByEmail(context.Background(), email).
			Return(nil, &domain.ErrUserNotFound{Message: "user not found"})

		// Mock user creation failure
		mockUserRepo.EXPECT().
			CreateUser(context.Background(), gomock.Any()).
			Return(errors.New("database error"))

		// Mock logger calls
		mockLogger.EXPECT().
			WithField("email", email).
			Return(mockLogger)
		mockLogger.EXPECT().
			WithField("error", "database error").
			Return(mockLogger)
		mockLogger.EXPECT().
			Error("Failed to create user for invitation acceptance")

		authResponse, err := service.AcceptInvitation(context.Background(), invitationID, workspaceID, email)

		require.Error(t, err)
		assert.Nil(t, authResponse)
		assert.Contains(t, err.Error(), "failed to create user")
	})

	t.Run("failed to add user to workspace", func(t *testing.T) {
		existingUser := &domain.User{
			ID:    "existing-user-123",
			Email: email,
		}

		// Mock invitation retrieval
		mockRepo.EXPECT().
			GetInvitationByID(context.Background(), invitationID).
			Return(&domain.WorkspaceInvitation{
				ID:          invitationID,
				Email:       email,
				WorkspaceID: workspaceID,
				Permissions: domain.UserPermissions{
					domain.PermissionResourceContacts: {Read: true, Write: true},
					domain.PermissionResourceLists:    {Read: true, Write: true},
				},
			}, nil)

		// User exists
		mockUserService.EXPECT().
			GetUserByEmail(context.Background(), email).
			Return(existingUser, nil)

		// Check if user is already a member (not a member)
		mockRepo.EXPECT().
			IsUserWorkspaceMember(context.Background(), existingUser.ID, workspaceID).
			Return(false, nil)

		// Mock adding user to workspace failure
		mockRepo.EXPECT().
			AddUserToWorkspace(context.Background(), gomock.Any()).
			Return(errors.New("database error"))

		// Mock logger calls
		mockLogger.EXPECT().
			WithField("user_id", existingUser.ID).
			Return(mockLogger)
		mockLogger.EXPECT().
			WithField("workspace_id", workspaceID).
			Return(mockLogger)
		mockLogger.EXPECT().
			WithField("error", "database error").
			Return(mockLogger)
		mockLogger.EXPECT().
			Error("Failed to add user to workspace")

		authResponse, err := service.AcceptInvitation(context.Background(), invitationID, workspaceID, email)

		require.Error(t, err)
		assert.Nil(t, authResponse)
		assert.Contains(t, err.Error(), "failed to add user to workspace")
	})

	t.Run("invitation deletion fails but main operation succeeds", func(t *testing.T) {
		existingUser := &domain.User{
			ID:    "existing-user-123",
			Email: email,
		}

		// Mock invitation retrieval
		mockRepo.EXPECT().
			GetInvitationByID(context.Background(), invitationID).
			Return(&domain.WorkspaceInvitation{
				ID:          invitationID,
				Email:       email,
				WorkspaceID: workspaceID,
				Permissions: domain.UserPermissions{
					domain.PermissionResourceContacts: {Read: true, Write: true},
					domain.PermissionResourceLists:    {Read: true, Write: true},
				},
			}, nil)

		// User exists
		mockUserService.EXPECT().
			GetUserByEmail(context.Background(), email).
			Return(existingUser, nil)

		// Check if user is already a member (not a member)
		mockRepo.EXPECT().
			IsUserWorkspaceMember(context.Background(), existingUser.ID, workspaceID).
			Return(false, nil)

		// Mock adding user to workspace
		mockRepo.EXPECT().
			AddUserToWorkspace(context.Background(), gomock.Any()).
			Return(nil)

		// Mock session creation
		mockUserRepo.EXPECT().
			CreateSession(context.Background(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, session *domain.Session) error {
				assert.Equal(t, existingUser.ID, session.UserID)
				assert.NotEmpty(t, session.ID)
				return nil
			})

		// Mock auth token generation
		mockAuthService.EXPECT().
			GenerateUserAuthToken(gomock.Any(), gomock.Any(), gomock.Any()).
			Return("auth-token-789")

		// Mock invitation deletion failure (should not fail the main operation)
		mockRepo.EXPECT().
			DeleteInvitation(context.Background(), invitationID).
			Return(errors.New("deletion failed"))

		// Mock logger calls for deletion failure
		mockLogger.EXPECT().
			WithField("invitation_id", invitationID).
			Return(mockLogger)
		mockLogger.EXPECT().
			WithField("error", "deletion failed").
			Return(mockLogger)
		mockLogger.EXPECT().
			Warn("Failed to delete invitation after successful acceptance")

		// Mock logger calls for success
		mockLogger.EXPECT().
			WithField("user_id", existingUser.ID).
			Return(mockLogger)
		mockLogger.EXPECT().
			WithField("workspace_id", workspaceID).
			Return(mockLogger)
		mockLogger.EXPECT().
			WithField("invitation_id", invitationID).
			Return(mockLogger)
		mockLogger.EXPECT().
			Info("Successfully accepted invitation and created session")

		authResponse, err := service.AcceptInvitation(context.Background(), invitationID, workspaceID, email)

		require.NoError(t, err) // Should still succeed despite deletion failure
		require.NotNil(t, authResponse)
		assert.Equal(t, "auth-token-789", authResponse.Token)
		assert.Equal(t, existingUser.ID, authResponse.User.ID)
		assert.Equal(t, email, authResponse.User.Email)
		assert.NotZero(t, authResponse.ExpiresAt)
	})
}

func TestWorkspaceService_DeleteInvitation(t *testing.T) {
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
	invitationID := "invitation1"
	userID := "user1"

	// Setup common logger expectations
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()

	t.Run("successful deletion", func(t *testing.T) {
		// Setup invitation for testing
		invitation := &domain.WorkspaceInvitation{
			ID:          invitationID,
			WorkspaceID: workspaceID,
			InviterID:   "inviter1",
			Email:       "test@example.com",
			ExpiresAt:   time.Now().Add(24 * time.Hour),
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}

		mockAuthSvc.EXPECT().
			AuthenticateUserFromContext(ctx).
			Return(&domain.User{ID: userID}, nil)

		mockRepo.EXPECT().
			GetInvitationByID(ctx, invitationID).
			Return(invitation, nil)

		mockRepo.EXPECT().
			GetUserWorkspace(ctx, userID, workspaceID).
			Return(&domain.UserWorkspace{
				UserID:      userID,
				WorkspaceID: workspaceID,
				Role:        "member",
			}, nil)

		mockRepo.EXPECT().
			DeleteInvitation(ctx, invitationID).
			Return(nil)

		err := service.DeleteInvitation(ctx, invitationID)
		require.NoError(t, err)
	})

	t.Run("authentication error", func(t *testing.T) {
		mockAuthSvc.EXPECT().
			AuthenticateUserFromContext(ctx).
			Return(nil, fmt.Errorf("authentication failed"))

		err := service.DeleteInvitation(ctx, invitationID)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to authenticate user")
	})

	t.Run("invitation not found", func(t *testing.T) {
		mockAuthSvc.EXPECT().
			AuthenticateUserFromContext(ctx).
			Return(&domain.User{ID: userID}, nil)

		mockRepo.EXPECT().
			GetInvitationByID(ctx, invitationID).
			Return(nil, fmt.Errorf("invitation not found"))

		err := service.DeleteInvitation(ctx, invitationID)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invitation not found")
	})

	t.Run("user not member of workspace", func(t *testing.T) {
		invitation := &domain.WorkspaceInvitation{
			ID:          invitationID,
			WorkspaceID: workspaceID,
			InviterID:   "inviter1",
			Email:       "test@example.com",
			ExpiresAt:   time.Now().Add(24 * time.Hour),
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}

		mockAuthSvc.EXPECT().
			AuthenticateUserFromContext(ctx).
			Return(&domain.User{ID: userID}, nil)

		mockRepo.EXPECT().
			GetInvitationByID(ctx, invitationID).
			Return(invitation, nil)

		mockRepo.EXPECT().
			GetUserWorkspace(ctx, userID, workspaceID).
			Return(nil, fmt.Errorf("user is not a member of the workspace"))

		err := service.DeleteInvitation(ctx, invitationID)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "You do not have access to this workspace")
	})

	t.Run("repository deletion error", func(t *testing.T) {
		invitation := &domain.WorkspaceInvitation{
			ID:          invitationID,
			WorkspaceID: workspaceID,
			InviterID:   "inviter1",
			Email:       "test@example.com",
			ExpiresAt:   time.Now().Add(24 * time.Hour),
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}

		mockAuthSvc.EXPECT().
			AuthenticateUserFromContext(ctx).
			Return(&domain.User{ID: userID}, nil)

		mockRepo.EXPECT().
			GetInvitationByID(ctx, invitationID).
			Return(invitation, nil)

		mockRepo.EXPECT().
			GetUserWorkspace(ctx, userID, workspaceID).
			Return(&domain.UserWorkspace{
				UserID:      userID,
				WorkspaceID: workspaceID,
				Role:        "member",
			}, nil)

		mockRepo.EXPECT().
			DeleteInvitation(ctx, invitationID).
			Return(fmt.Errorf("database error"))

		err := service.DeleteInvitation(ctx, invitationID)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to delete invitation")
	})
}

func TestWorkspaceService_SetUserPermissions(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockUserRepo := mocks.NewMockUserRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockUserService := mocks.NewMockUserServiceInterface(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)
	mockMailer := pkgmocks.NewMockMailer(ctrl)
	mockConfig := &config.Config{RootEmail: "test@example.com"}
	mockContactService := mocks.NewMockContactService(ctrl)
	mockListService := mocks.NewMockListService(ctrl)
	mockContactListService := mocks.NewMockContactListService(ctrl)
	mockTemplateService := mocks.NewMockTemplateService(ctrl)
	mockWebhookRegService := mocks.NewMockWebhookRegistrationService(ctrl)

	service := NewWorkspaceService(
		mockRepo,
		mockUserRepo,
		mocks.NewMockTaskRepository(ctrl),
		mockLogger,
		mockUserService,
		mockAuthService,
		mockMailer,
		mockConfig,
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
	mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()

	ctx := context.Background()
	workspaceID := "testworkspace"
	ownerID := "owner123"
	targetUserID := "user123"
	permissions := domain.UserPermissions{
		domain.PermissionResourceContacts: {Read: true, Write: true},
	}

	t.Run("successful set permissions", func(t *testing.T) {
		owner := &domain.User{ID: ownerID}
		ownerWorkspace := &domain.UserWorkspace{
			UserID:      ownerID,
			WorkspaceID: workspaceID,
			Role:        "owner",
		}
		targetUserWorkspace := &domain.UserWorkspace{
			UserID:      targetUserID,
			WorkspaceID: workspaceID,
			Role:        "member",
		}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, owner, ownerWorkspace, nil)
		mockRepo.EXPECT().GetUserWorkspace(ctx, targetUserID, workspaceID).Return(targetUserWorkspace, nil)
		mockRepo.EXPECT().UpdateUserWorkspacePermissions(ctx, gomock.Any()).DoAndReturn(func(ctx context.Context, userWorkspace *domain.UserWorkspace) error {
			assert.Equal(t, targetUserID, userWorkspace.UserID)
			assert.Equal(t, workspaceID, userWorkspace.WorkspaceID)
			return nil
		})
		mockUserRepo.EXPECT().GetSessionsByUserID(ctx, targetUserID).Return([]*domain.Session{
			{ID: "session1", UserID: targetUserID},
			{ID: "session2", UserID: targetUserID},
		}, nil)
		mockUserRepo.EXPECT().DeleteSession(ctx, "session1").Return(nil)
		mockUserRepo.EXPECT().DeleteSession(ctx, "session2").Return(nil)

		err := service.SetUserPermissions(ctx, workspaceID, targetUserID, permissions)
		require.NoError(t, err)
	})

	t.Run("authentication error", func(t *testing.T) {
		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, nil, nil, errors.New("auth error"))

		err := service.SetUserPermissions(ctx, workspaceID, targetUserID, permissions)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to authenticate user")
	})

	t.Run("current user not owner", func(t *testing.T) {
		member := &domain.User{ID: "member123"}
		memberWorkspace := &domain.UserWorkspace{
			UserID:      "member123",
			WorkspaceID: workspaceID,
			Role:        "member",
		}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, member, memberWorkspace, nil)

		err := service.SetUserPermissions(ctx, workspaceID, targetUserID, permissions)
		require.Error(t, err)
		// Typed, so the handler answers 403 instead of a generic 500.
		var unauthorized *domain.ErrUnauthorized
		require.ErrorAs(t, err, &unauthorized)
		assert.Contains(t, err.Error(), "only workspace owners can manage user permissions")
	})

	t.Run("unknown permission resource is rejected", func(t *testing.T) {
		owner := &domain.User{ID: ownerID}
		ownerWorkspace := &domain.UserWorkspace{
			UserID:      ownerID,
			WorkspaceID: workspaceID,
			Role:        "owner",
		}

		// Rejected before the target row is even read.
		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, owner, ownerWorkspace, nil)

		err := service.SetUserPermissions(ctx, workspaceID, targetUserID, domain.UserPermissions{
			"not_a_resource": {Read: true},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown permission resource")
	})

	t.Run("stored permissions do not alias the caller's map", func(t *testing.T) {
		owner := &domain.User{ID: ownerID}
		ownerWorkspace := &domain.UserWorkspace{
			UserID:      ownerID,
			WorkspaceID: workspaceID,
			Role:        "owner",
		}
		targetUserWorkspace := &domain.UserWorkspace{
			UserID:      targetUserID,
			WorkspaceID: workspaceID,
			Role:        "member",
		}
		callerPermissions := domain.UserPermissions{
			domain.PermissionResourceContacts: {Read: true},
		}

		var stored domain.UserPermissions
		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, owner, ownerWorkspace, nil)
		mockRepo.EXPECT().GetUserWorkspace(ctx, targetUserID, workspaceID).Return(targetUserWorkspace, nil)
		mockRepo.EXPECT().UpdateUserWorkspacePermissions(ctx, gomock.Any()).DoAndReturn(func(ctx context.Context, userWorkspace *domain.UserWorkspace) error {
			stored = userWorkspace.Permissions
			return nil
		})
		mockUserRepo.EXPECT().GetSessionsByUserID(ctx, targetUserID).Return(nil, nil)

		err := service.SetUserPermissions(ctx, workspaceID, targetUserID, callerPermissions)
		require.NoError(t, err)

		callerPermissions[domain.PermissionResourceBroadcasts] = domain.ResourcePermissions{Read: true, Write: true}
		assert.NotContains(t, stored, domain.PermissionResourceBroadcasts)
	})

	t.Run("target user not member", func(t *testing.T) {
		owner := &domain.User{ID: ownerID}
		ownerWorkspace := &domain.UserWorkspace{
			UserID:      ownerID,
			WorkspaceID: workspaceID,
			Role:        "owner",
		}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, owner, ownerWorkspace, nil)
		mockRepo.EXPECT().GetUserWorkspace(ctx, targetUserID, workspaceID).Return(nil, errors.New("user not found"))

		err := service.SetUserPermissions(ctx, workspaceID, targetUserID, permissions)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "user is not a member of the workspace")
	})

	t.Run("cannot modify owner permissions", func(t *testing.T) {
		owner := &domain.User{ID: ownerID}
		ownerWorkspace := &domain.UserWorkspace{
			UserID:      ownerID,
			WorkspaceID: workspaceID,
			Role:        "owner",
		}
		targetOwnerWorkspace := &domain.UserWorkspace{
			UserID:      targetUserID,
			WorkspaceID: workspaceID,
			Role:        "owner",
		}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, owner, ownerWorkspace, nil)
		mockRepo.EXPECT().GetUserWorkspace(ctx, targetUserID, workspaceID).Return(targetOwnerWorkspace, nil)

		err := service.SetUserPermissions(ctx, workspaceID, targetUserID, permissions)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot modify permissions for workspace owners")
	})

	t.Run("session invalidation fails but operation succeeds", func(t *testing.T) {
		owner := &domain.User{ID: ownerID}
		ownerWorkspace := &domain.UserWorkspace{
			UserID:      ownerID,
			WorkspaceID: workspaceID,
			Role:        "owner",
		}
		targetUserWorkspace := &domain.UserWorkspace{
			UserID:      targetUserID,
			WorkspaceID: workspaceID,
			Role:        "member",
		}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, owner, ownerWorkspace, nil)
		mockRepo.EXPECT().GetUserWorkspace(ctx, targetUserID, workspaceID).Return(targetUserWorkspace, nil)
		mockRepo.EXPECT().UpdateUserWorkspacePermissions(ctx, gomock.Any()).Return(nil)
		mockUserRepo.EXPECT().GetSessionsByUserID(ctx, targetUserID).Return(nil, errors.New("session error"))

		err := service.SetUserPermissions(ctx, workspaceID, targetUserID, permissions)
		require.NoError(t, err) // Should still succeed despite session error
	})
}

func TestWorkspaceService_deleteSupabaseIntegrationResources(t *testing.T) {
	// Test WorkspaceService.deleteSupabaseIntegrationResources - this was at 0% coverage
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockUserRepo := mocks.NewMockUserRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockUserService := mocks.NewMockUserServiceInterface(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)
	mockMailer := pkgmocks.NewMockMailer(ctrl)
	mockConfig := &config.Config{RootEmail: "test@example.com"}
	mockContactService := mocks.NewMockContactService(ctrl)
	mockListService := mocks.NewMockListService(ctrl)
	mockContactListService := mocks.NewMockContactListService(ctrl)
	mockTemplateService := mocks.NewMockTemplateService(ctrl)
	mockWebhookRegService := mocks.NewMockWebhookRegistrationService(ctrl)

	// Create a real SupabaseService with mocked dependencies
	mockTemplateRepo := mocks.NewMockTemplateRepository(ctrl)
	mockTransactionalRepo := mocks.NewMockTransactionalNotificationRepository(ctrl)
	mockInboundWebhookEventRepo := mocks.NewMockInboundWebhookEventRepository(ctrl)
	mockContactListRepo := mocks.NewMockContactListRepository(ctrl)
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()

	supabaseService := NewSupabaseService(
		mockRepo,
		nil, // emailService
		mockContactService,
		mockListService,
		mockContactListRepo,
		mockTemplateRepo,
		mockTemplateService,
		mockTransactionalRepo,
		nil, // transactionalService
		mockInboundWebhookEventRepo,
		mockLogger,
	)

	service := NewWorkspaceService(
		mockRepo,
		mockUserRepo,
		mocks.NewMockTaskRepository(ctrl),
		mockLogger,
		mockUserService,
		mockAuthService,
		mockMailer,
		mockConfig,
		mockContactService,
		mockListService,
		mockContactListService,
		mockTemplateService,
		mockWebhookRegService,
		"secret_key",
		supabaseService,
		&DNSVerificationService{},
		&BlogService{},
	)

	ctx := context.Background()
	workspaceID := "workspace-123"
	integrationID := "integration-456"

	t.Run("Success - Deletes resources", func(t *testing.T) {
		// Mock template repo to return empty list (no templates to delete)
		mockTemplateRepo.EXPECT().
			GetTemplates(gomock.Any(), workspaceID, "", "").
			Return([]*domain.Template{}, nil)

		// Mock transactional repo to return empty list (no notifications to delete)
		mockTransactionalRepo.EXPECT().
			List(gomock.Any(), workspaceID, gomock.Any(), gomock.Any(), gomock.Any()).
			Return([]*domain.TransactionalNotification{}, 0, nil)

		err := service.deleteSupabaseIntegrationResources(ctx, workspaceID, integrationID)
		assert.NoError(t, err)
	})

	t.Run("Error - Template repo error", func(t *testing.T) {
		mockTemplateRepo.EXPECT().
			GetTemplates(gomock.Any(), workspaceID, "", "").
			Return(nil, errors.New("template repo error"))

		err := service.deleteSupabaseIntegrationResources(ctx, workspaceID, integrationID)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to list templates")
	})
}

func TestWorkspaceService_InviteMember_TeamMemberLimit(t *testing.T) {
	workspaceID := "ws-123"
	inviterID := "inviter-123"
	email := "newuser@example.com"

	setupService := func(t *testing.T, maxUsers int) (
		*WorkspaceService,
		*mocks.MockWorkspaceRepository,
		*mocks.MockUserServiceInterface,
		*mocks.MockAuthService,
		*pkgmocks.MockLogger,
	) {
		ctrl := gomock.NewController(t)
		mockRepo := mocks.NewMockWorkspaceRepository(ctrl)
		mockUserRepo := mocks.NewMockUserRepository(ctrl)
		mockLogger := pkgmocks.NewMockLogger(ctrl)
		mockUserService := mocks.NewMockUserServiceInterface(ctrl)
		mockAuthService := mocks.NewMockAuthService(ctrl)
		mockMailer := pkgmocks.NewMockMailer(ctrl)
		mockConfig := &config.Config{RootEmail: "test@example.com", MaxUsers: maxUsers, Environment: "development"}
		mockContactService := mocks.NewMockContactService(ctrl)
		mockListService := mocks.NewMockListService(ctrl)
		mockContactListService := mocks.NewMockContactListService(ctrl)
		mockTemplateService := mocks.NewMockTemplateService(ctrl)
		mockWebhookRegService := mocks.NewMockWebhookRegistrationService(ctrl)

		mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
		mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
		mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()

		service := NewWorkspaceService(
			mockRepo, mockUserRepo, mocks.NewMockTaskRepository(ctrl),
			mockLogger, mockUserService, mockAuthService, mockMailer,
			mockConfig, mockContactService, mockListService,
			mockContactListService, mockTemplateService, mockWebhookRegService,
			"secret_key", &SupabaseService{}, &DNSVerificationService{}, &BlogService{},
		)
		return service, mockRepo, mockUserService, mockAuthService, mockLogger
	}

	t.Run("limit reached", func(t *testing.T) {
		service, mockRepo, _, mockAuthService, _ := setupService(t, 3)

		inviter := &domain.User{ID: inviterID}
		mockAuthService.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
			Return(context.Background(), inviter, &domain.UserWorkspace{UserID: inviterID, WorkspaceID: workspaceID, Role: "owner"}, nil)
		mockRepo.EXPECT().
			GetByID(gomock.Any(), workspaceID).
			Return(&domain.Workspace{ID: workspaceID}, nil)
		mockRepo.EXPECT().
			CountWorkspaceMembersAndInvitations(gomock.Any(), workspaceID).
			Return(3, nil)

		_, _, err := service.InviteMember(context.Background(), workspaceID, email, domain.UserPermissions{})
		require.Error(t, err)

		var limitErr *domain.ErrTeamMemberLimitReached
		assert.True(t, errors.As(err, &limitErr))
		assert.Equal(t, 3, limitErr.Limit)
		assert.Equal(t, 3, limitErr.Current)
	})

	t.Run("under limit", func(t *testing.T) {
		service, mockRepo, mockUserService, mockAuthService, _ := setupService(t, 3)

		inviter := &domain.User{ID: inviterID}
		mockAuthService.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
			Return(context.Background(), inviter, &domain.UserWorkspace{UserID: inviterID, WorkspaceID: workspaceID, Role: "owner"}, nil)
		mockRepo.EXPECT().
			GetByID(gomock.Any(), workspaceID).
			Return(&domain.Workspace{ID: workspaceID}, nil)
		mockRepo.EXPECT().
			CountWorkspaceMembersAndInvitations(gomock.Any(), workspaceID).
			Return(2, nil)

		// After limit check passes, the flow continues — mock the user lookup
		mockUserService.EXPECT().
			GetUserByID(gomock.Any(), inviterID).
			Return(&domain.User{ID: inviterID, Name: "Inviter"}, nil)
		mockUserService.EXPECT().
			GetUserByEmail(gomock.Any(), email).
			Return(nil, fmt.Errorf("user not found"))

		// User doesn't exist → invitation created
		mockRepo.EXPECT().
			CreateInvitation(gomock.Any(), gomock.Any()).
			Return(nil)
		mockAuthService.EXPECT().
			GenerateInvitationToken(gomock.Any()).
			Return("token-123")

		_, _, err := service.InviteMember(context.Background(), workspaceID, email, domain.UserPermissions{})
		require.NoError(t, err)
	})

	t.Run("unlimited when MaxUsers is zero", func(t *testing.T) {
		service, mockRepo, mockUserService, mockAuthService, _ := setupService(t, 0)

		inviter := &domain.User{ID: inviterID}
		mockAuthService.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
			Return(context.Background(), inviter, &domain.UserWorkspace{UserID: inviterID, WorkspaceID: workspaceID, Role: "owner"}, nil)
		mockRepo.EXPECT().
			GetByID(gomock.Any(), workspaceID).
			Return(&domain.Workspace{ID: workspaceID}, nil)

		// CountWorkspaceMembersAndInvitations should NOT be called when MaxUsers=0
		// (gomock will fail if it's called since no expectation is set)

		mockUserService.EXPECT().
			GetUserByID(gomock.Any(), inviterID).
			Return(&domain.User{ID: inviterID, Name: "Inviter"}, nil)
		mockUserService.EXPECT().
			GetUserByEmail(gomock.Any(), email).
			Return(nil, fmt.Errorf("user not found"))
		mockRepo.EXPECT().
			CreateInvitation(gomock.Any(), gomock.Any()).
			Return(nil)
		mockAuthService.EXPECT().
			GenerateInvitationToken(gomock.Any()).
			Return("token-123")

		_, _, err := service.InviteMember(context.Background(), workspaceID, email, domain.UserPermissions{})
		require.NoError(t, err)
	})
}

func TestWorkspaceService_AcceptInvitation_TeamMemberLimit(t *testing.T) {
	invitationID := "invitation-123"
	workspaceID := "ws-123"
	email := "test@example.com"

	setupService := func(t *testing.T, maxUsers int) (
		*WorkspaceService,
		*mocks.MockWorkspaceRepository,
		*mocks.MockUserServiceInterface,
		*mocks.MockUserRepository,
		*mocks.MockAuthService,
	) {
		ctrl := gomock.NewController(t)
		mockRepo := mocks.NewMockWorkspaceRepository(ctrl)
		mockUserRepo := mocks.NewMockUserRepository(ctrl)
		mockLogger := pkgmocks.NewMockLogger(ctrl)
		mockUserService := mocks.NewMockUserServiceInterface(ctrl)
		mockAuthService := mocks.NewMockAuthService(ctrl)
		mockMailer := pkgmocks.NewMockMailer(ctrl)
		mockConfig := &config.Config{RootEmail: "test@example.com", MaxUsers: maxUsers}
		mockContactService := mocks.NewMockContactService(ctrl)
		mockListService := mocks.NewMockListService(ctrl)
		mockContactListService := mocks.NewMockContactListService(ctrl)
		mockTemplateService := mocks.NewMockTemplateService(ctrl)
		mockWebhookRegService := mocks.NewMockWebhookRegistrationService(ctrl)

		mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
		mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
		mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
		mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()

		service := NewWorkspaceService(
			mockRepo, mockUserRepo, mocks.NewMockTaskRepository(ctrl),
			mockLogger, mockUserService, mockAuthService, mockMailer,
			mockConfig, mockContactService, mockListService,
			mockContactListService, mockTemplateService, mockWebhookRegService,
			"secret_key", &SupabaseService{}, &DNSVerificationService{}, &BlogService{},
		)
		return service, mockRepo, mockUserService, mockUserRepo, mockAuthService
	}

	t.Run("limit reached on accept", func(t *testing.T) {
		// MaxUsers=3, count returns 4 (3 members + 1 invitation including the one being accepted).
		// The code subtracts 1 for the current invitation, so effective count = 3 >= limit 3 → blocked.
		service, mockRepo, mockUserService, _, _ := setupService(t, 3)

		mockRepo.EXPECT().
			GetInvitationByID(gomock.Any(), invitationID).
			Return(&domain.WorkspaceInvitation{
				ID:          invitationID,
				Email:       email,
				WorkspaceID: workspaceID,
				Permissions: domain.UserPermissions{},
			}, nil)

		// Existing user
		mockUserService.EXPECT().
			GetUserByEmail(gomock.Any(), email).
			Return(&domain.User{ID: "user-123", Email: email}, nil)
		mockRepo.EXPECT().
			IsUserWorkspaceMember(gomock.Any(), "user-123", workspaceID).
			Return(false, nil)
		mockRepo.EXPECT().
			CountWorkspaceMembersAndInvitations(gomock.Any(), workspaceID).
			Return(4, nil)

		_, err := service.AcceptInvitation(context.Background(), invitationID, workspaceID, email)
		require.Error(t, err)

		var limitErr *domain.ErrTeamMemberLimitReached
		assert.True(t, errors.As(err, &limitErr))
		assert.Equal(t, 3, limitErr.Limit)
	})

	t.Run("accepts when count equals limit (current invitation counted)", func(t *testing.T) {
		// MaxUsers=3, count returns 3 (2 members + 1 invitation being accepted).
		// The code subtracts 1 → effective count = 2 < 3 → allowed.
		service, mockRepo, mockUserService, mockUserRepo, mockAuthService := setupService(t, 3)

		mockRepo.EXPECT().
			GetInvitationByID(gomock.Any(), invitationID).
			Return(&domain.WorkspaceInvitation{
				ID:          invitationID,
				Email:       email,
				WorkspaceID: workspaceID,
				Permissions: domain.UserPermissions{},
			}, nil)

		mockUserService.EXPECT().
			GetUserByEmail(gomock.Any(), email).
			Return(nil, &domain.ErrUserNotFound{Message: "not found"})
		mockUserRepo.EXPECT().
			CreateUser(gomock.Any(), gomock.Any()).
			Return(nil)
		mockRepo.EXPECT().
			CountWorkspaceMembersAndInvitations(gomock.Any(), workspaceID).
			Return(3, nil)
		mockRepo.EXPECT().
			AddUserToWorkspace(gomock.Any(), gomock.Any()).
			Return(nil)
		mockUserRepo.EXPECT().
			CreateSession(gomock.Any(), gomock.Any()).
			Return(nil)
		mockAuthService.EXPECT().
			GenerateUserAuthToken(gomock.Any(), gomock.Any(), gomock.Any()).
			Return("auth-token")
		mockRepo.EXPECT().
			DeleteInvitation(gomock.Any(), invitationID).
			Return(nil)

		resp, err := service.AcceptInvitation(context.Background(), invitationID, workspaceID, email)
		require.NoError(t, err)
		assert.NotNil(t, resp)
	})

	t.Run("under limit on accept", func(t *testing.T) {
		service, mockRepo, mockUserService, mockUserRepo, mockAuthService := setupService(t, 3)

		mockRepo.EXPECT().
			GetInvitationByID(gomock.Any(), invitationID).
			Return(&domain.WorkspaceInvitation{
				ID:          invitationID,
				Email:       email,
				WorkspaceID: workspaceID,
				Permissions: domain.UserPermissions{},
			}, nil)

		mockUserService.EXPECT().
			GetUserByEmail(gomock.Any(), email).
			Return(nil, &domain.ErrUserNotFound{Message: "not found"})
		mockUserRepo.EXPECT().
			CreateUser(gomock.Any(), gomock.Any()).
			Return(nil)
		mockRepo.EXPECT().
			CountWorkspaceMembersAndInvitations(gomock.Any(), workspaceID).
			Return(2, nil)
		mockRepo.EXPECT().
			AddUserToWorkspace(gomock.Any(), gomock.Any()).
			Return(nil)
		mockUserRepo.EXPECT().
			CreateSession(gomock.Any(), gomock.Any()).
			Return(nil)
		mockAuthService.EXPECT().
			GenerateUserAuthToken(gomock.Any(), gomock.Any(), gomock.Any()).
			Return("auth-token")
		mockRepo.EXPECT().
			DeleteInvitation(gomock.Any(), invitationID).
			Return(nil)

		resp, err := service.AcceptInvitation(context.Background(), invitationID, workspaceID, email)
		require.NoError(t, err)
		assert.NotNil(t, resp)
		assert.Equal(t, "auth-token", resp.Token)
	})
}

func TestWorkspaceService_CreateWorkspace_WorkspaceLimit(t *testing.T) {
	setupService := func(t *testing.T, maxWorkspaces int) (
		*WorkspaceService,
		*mocks.MockWorkspaceRepository,
		*mocks.MockAuthService,
		*pkgmocks.MockLogger,
	) {
		ctrl := gomock.NewController(t)
		mockRepo := mocks.NewMockWorkspaceRepository(ctrl)
		mockUserRepo := mocks.NewMockUserRepository(ctrl)
		mockLogger := pkgmocks.NewMockLogger(ctrl)
		mockUserService := mocks.NewMockUserServiceInterface(ctrl)
		mockAuthService := mocks.NewMockAuthService(ctrl)
		mockMailer := pkgmocks.NewMockMailer(ctrl)
		mockConfig := &config.Config{RootEmail: "test@example.com", MaxWorkspaces: maxWorkspaces, Environment: "development"}
		mockContactService := mocks.NewMockContactService(ctrl)
		mockListService := mocks.NewMockListService(ctrl)
		mockContactListService := mocks.NewMockContactListService(ctrl)
		mockTemplateService := mocks.NewMockTemplateService(ctrl)
		mockWebhookRegService := mocks.NewMockWebhookRegistrationService(ctrl)

		mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
		mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
		mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()

		service := NewWorkspaceService(
			mockRepo, mockUserRepo, mocks.NewMockTaskRepository(ctrl),
			mockLogger, mockUserService, mockAuthService, mockMailer,
			mockConfig, mockContactService, mockListService,
			mockContactListService, mockTemplateService, mockWebhookRegService,
			"secret_key", &SupabaseService{}, &DNSVerificationService{}, &BlogService{},
		)
		return service, mockRepo, mockAuthService, mockLogger
	}

	t.Run("limit reached", func(t *testing.T) {
		service, mockRepo, mockAuthService, _ := setupService(t, 3)

		rootUser := &domain.User{ID: "root-user", Email: "test@example.com"}
		mockAuthService.EXPECT().
			AuthenticateUserFromContext(gomock.Any()).
			Return(rootUser, nil)
		mockRepo.EXPECT().
			CountWorkspaces(gomock.Any()).
			Return(3, nil)

		_, err := service.CreateWorkspace(context.Background(), "newws", "New Workspace", "", "", "", "UTC", domain.FileManagerSettings{}, "en", []string{"en"})
		require.Error(t, err)

		var limitErr *domain.ErrWorkspaceLimitReached
		assert.True(t, errors.As(err, &limitErr))
		assert.Equal(t, 3, limitErr.Limit)
		assert.Equal(t, 3, limitErr.Current)
	})

	t.Run("under limit", func(t *testing.T) {
		service, mockRepo, mockAuthService, _ := setupService(t, 3)

		rootUser := &domain.User{ID: "root-user", Email: "test@example.com"}
		mockAuthService.EXPECT().
			AuthenticateUserFromContext(gomock.Any()).
			Return(rootUser, nil)
		mockRepo.EXPECT().
			CountWorkspaces(gomock.Any()).
			Return(2, nil)
		// After limit check passes, mock GetByID to return existing workspace so we get
		// a clean "workspace already exists" error — proving the limit check passed
		mockRepo.EXPECT().
			GetByID(gomock.Any(), "newws").
			Return(&domain.Workspace{ID: "newws"}, nil)

		_, err := service.CreateWorkspace(context.Background(), "newws", "New Workspace", "", "", "", "UTC", domain.FileManagerSettings{}, "en", []string{"en"})
		require.Error(t, err)
		assert.Equal(t, "workspace already exists", err.Error())

		var limitErr *domain.ErrWorkspaceLimitReached
		assert.False(t, errors.As(err, &limitErr), "error should NOT be a workspace limit error")
	})

	t.Run("unlimited when MaxWorkspaces is zero", func(t *testing.T) {
		service, mockRepo, mockAuthService, _ := setupService(t, 0)

		rootUser := &domain.User{ID: "root-user", Email: "test@example.com"}
		mockAuthService.EXPECT().
			AuthenticateUserFromContext(gomock.Any()).
			Return(rootUser, nil)

		// CountWorkspaces should NOT be called when MaxWorkspaces=0
		// (gomock will fail if it's called since no expectation is set)

		// Mock GetByID to return existing workspace so flow exits cleanly
		mockRepo.EXPECT().
			GetByID(gomock.Any(), "newws").
			Return(&domain.Workspace{ID: "newws"}, nil)

		_, err := service.CreateWorkspace(context.Background(), "newws", "New Workspace", "", "", "", "UTC", domain.FileManagerSettings{}, "en", []string{"en"})
		require.Error(t, err)
		assert.Equal(t, "workspace already exists", err.Error())

		var limitErr *domain.ErrWorkspaceLimitReached
		assert.False(t, errors.As(err, &limitErr), "error should NOT be a workspace limit error")
	})
}

// TestWorkspaceService_DeleteInvitation_PlatformAdmin verifies the explicit root bypass
// in DeleteInvitation: a platform admin with no membership row may delete invitations,
// but the bypass only fires for the typed not-a-member error — real DB errors still deny.
func TestWorkspaceService_DeleteInvitation_PlatformAdmin(t *testing.T) {
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
	invitationID := "inv1"
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	invitation := &domain.WorkspaceInvitation{ID: invitationID, WorkspaceID: workspaceID, Email: "x@example.com", ExpiresAt: time.Now().Add(time.Hour)}

	t.Run("root admin without a membership row may delete the invitation", func(t *testing.T) {
		mockAuthSvc.EXPECT().AuthenticateUserFromContext(ctx).Return(&domain.User{ID: "root-id", Email: "root@example.com"}, nil)
		mockRepo.EXPECT().GetInvitationByID(ctx, invitationID).Return(invitation, nil)
		mockRepo.EXPECT().GetUserWorkspace(ctx, "root-id", workspaceID).Return(nil, domain.ErrUserNotInWorkspace)
		mockRepo.EXPECT().DeleteInvitation(ctx, invitationID).Return(nil)

		err := service.DeleteInvitation(ctx, invitationID)
		require.NoError(t, err)
	})

	t.Run("root bypass does not swallow a real DB error", func(t *testing.T) {
		mockAuthSvc.EXPECT().AuthenticateUserFromContext(ctx).Return(&domain.User{ID: "root-id", Email: "root@example.com"}, nil)
		mockRepo.EXPECT().GetInvitationByID(ctx, invitationID).Return(invitation, nil)
		mockRepo.EXPECT().GetUserWorkspace(ctx, "root-id", workspaceID).Return(nil, errors.New("db connection lost"))
		// No DeleteInvitation call expected — a non-not-found error must NOT be bypassed.

		err := service.DeleteInvitation(ctx, invitationID)
		require.Error(t, err)
	})
}

// zapierTestCtxKey marks the context AuthenticateUserForWorkspace hands back, so a test can tell
// the enriched context from the one the caller came in with.
type zapierTestCtxKey struct{}

// newZapierTestService wires a WorkspaceService whose repositories and auth service are the
// mocks it returns, with an API endpoint the minted address can be matched against.
func newZapierTestService(t *testing.T, ctrl *gomock.Controller) (*WorkspaceService, *mocks.MockWorkspaceRepository, *mocks.MockUserRepository, *mocks.MockAuthService) {
	t.Helper()

	mockRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockUserRepo := mocks.NewMockUserRepository(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()

	service := NewWorkspaceService(
		mockRepo,
		mockUserRepo,
		mocks.NewMockTaskRepository(ctrl),
		mockLogger,
		mocks.NewMockUserServiceInterface(ctrl),
		mockAuthService,
		pkgmocks.NewMockMailer(ctrl),
		&config.Config{RootEmail: "root@example.com", APIEndpoint: "https://api.example.com/v1"},
		mocks.NewMockContactService(ctrl),
		mocks.NewMockListService(ctrl),
		mocks.NewMockContactListService(ctrl),
		mocks.NewMockTemplateService(ctrl),
		mocks.NewMockWebhookRegistrationService(ctrl),
		"secret_key",
		&SupabaseService{},
		&DNSVerificationService{},
		&BlogService{},
	)

	return service, mockRepo, mockUserRepo, mockAuthService
}

func TestWorkspaceService_ConnectZapier(t *testing.T) {
	const (
		workspaceID = "testworkspace"
		userID      = "testuser"
		label       = "Marketing Ops"
	)

	// The shape ZapierKeyPrefix mints, anchored to the API endpoint the test service carries.
	mintedAddress := regexp.MustCompile(`^zapier-[a-z0-9_-]*-?[0-9a-f]{8}@api\.example\.com$`)

	ownerAuth := func(mockAuthService *mocks.MockAuthService, times int) {
		mockAuthService.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
			DoAndReturn(func(c context.Context, _ string) (context.Context, *domain.User, *domain.UserWorkspace, error) {
				return c, &domain.User{ID: userID}, &domain.UserWorkspace{
					UserID:      userID,
					WorkspaceID: workspaceID,
					Role:        "owner",
				}, nil
			}).
			Times(times)
	}

	t.Run("rejects a non-member with an authorization error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		service, _, _, mockAuthService := newZapierTestService(t, ctrl)

		// Modelled on CreateAPIKey rather than CreateIntegration: that one wraps this sentinel
		// in a bare fmt.Errorf and the caller gets a 500 for what is a 403.
		mockAuthService.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
			Return(context.Background(), nil, nil, domain.ErrUserNotInWorkspace)

		token, email, integrationID, err := service.ConnectZapier(context.Background(), workspaceID, label)
		require.Error(t, err)
		assert.IsType(t, &domain.ErrUnauthorized{}, err)
		assert.Empty(t, token)
		assert.Empty(t, email)
		assert.Empty(t, integrationID)
	})

	t.Run("rejects a non-owner before any write", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		// No repository expectations at all: any user row, membership or workspace write
		// before the gate fails the subtest.
		service, _, _, mockAuthService := newZapierTestService(t, ctrl)

		mockAuthService.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
			Return(context.Background(), &domain.User{ID: userID}, &domain.UserWorkspace{
				UserID:      userID,
				WorkspaceID: workspaceID,
				Role:        "member",
			}, nil)

		token, email, integrationID, err := service.ConnectZapier(context.Background(), workspaceID, label)
		require.Error(t, err)
		assert.IsType(t, &domain.ErrUnauthorized{}, err)
		assert.Equal(t, "user is not an owner of the workspace", err.Error())
		assert.Empty(t, token)
		assert.Empty(t, email)
		assert.Empty(t, integrationID)
	})

	t.Run("mints a key and records the integration", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		service, mockRepo, mockUserRepo, mockAuthService := newZapierTestService(t, ctrl)

		ownerAuth(mockAuthService, 2)

		var createdEmail string
		mockUserRepo.EXPECT().CreateUser(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, user *domain.User) error {
			createdEmail = user.Email
			assert.Equal(t, domain.UserTypeAPIKey, user.Type)
			return nil
		})
		mockRepo.EXPECT().AddUserToWorkspace(gomock.Any(), gomock.Any()).Return(nil)
		mockAuthService.EXPECT().GenerateAPIAuthToken(gomock.Any()).Return("zapier-token")
		mockRepo.EXPECT().GetByID(gomock.Any(), workspaceID).Return(&domain.Workspace{ID: workspaceID, Name: "Test Workspace"}, nil)

		var written domain.Integration
		mockRepo.EXPECT().Update(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, workspace *domain.Workspace) error {
			require.Equal(t, 1, len(workspace.Integrations))
			written = workspace.Integrations[0]
			return nil
		})

		token, email, integrationID, err := service.ConnectZapier(context.Background(), workspaceID, label)
		require.NoError(t, err)
		assert.Equal(t, "zapier-token", token)
		assert.Regexp(t, mintedAddress, email)
		assert.Equal(t, createdEmail, email)
		assert.NotEmpty(t, integrationID)

		assert.Equal(t, integrationID, written.ID)
		assert.Equal(t, domain.IntegrationTypeZapier, written.Type)
		// The label names the card; the address is what Settings → Team shows.
		assert.Equal(t, label, written.Name)
		require.NotNil(t, written.ZapierSettings)
		assert.Equal(t, email, written.ZapierSettings.APIKeyEmail)
	})

	t.Run("threads the authenticated context into CreateAPIKey", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		service, mockRepo, mockUserRepo, mockAuthService := newZapierTestService(t, ctrl)

		// Dropping the returned context would defeat the auth cache and make CreateAPIKey
		// re-run the queries behind AuthenticateUserForWorkspace on every attempt.
		var seen []context.Context
		mockAuthService.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
			DoAndReturn(func(c context.Context, _ string) (context.Context, *domain.User, *domain.UserWorkspace, error) {
				seen = append(seen, c)
				return context.WithValue(c, zapierTestCtxKey{}, "authenticated"), &domain.User{ID: userID}, &domain.UserWorkspace{
					UserID:      userID,
					WorkspaceID: workspaceID,
					Role:        "owner",
				}, nil
			}).
			Times(2)

		mockUserRepo.EXPECT().CreateUser(gomock.Any(), gomock.Any()).Return(nil)
		mockRepo.EXPECT().AddUserToWorkspace(gomock.Any(), gomock.Any()).Return(nil)
		mockAuthService.EXPECT().GenerateAPIAuthToken(gomock.Any()).Return("zapier-token")
		mockRepo.EXPECT().GetByID(gomock.Any(), workspaceID).Return(&domain.Workspace{ID: workspaceID}, nil)
		mockRepo.EXPECT().Update(gomock.Any(), gomock.Any()).Return(nil)

		_, _, _, err := service.ConnectZapier(context.Background(), workspaceID, label)
		require.NoError(t, err)

		require.Equal(t, 2, len(seen))
		assert.Nil(t, seen[0].Value(zapierTestCtxKey{}), "ConnectZapier authenticates with the caller context")
		assert.Equal(t, "authenticated", seen[1].Value(zapierTestCtxKey{}), "CreateAPIKey must receive the enriched context")
	})

	t.Run("mints a scoped key rather than a full-access one", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		service, mockRepo, mockUserRepo, mockAuthService := newZapierTestService(t, ctrl)

		ownerAuth(mockAuthService, 2)
		mockUserRepo.EXPECT().CreateUser(gomock.Any(), gomock.Any()).Return(nil)

		var stored domain.UserPermissions
		mockRepo.EXPECT().AddUserToWorkspace(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, userWorkspace *domain.UserWorkspace) error {
			stored = userWorkspace.Permissions
			return nil
		})
		mockAuthService.EXPECT().GenerateAPIAuthToken(gomock.Any()).Return("zapier-token")
		mockRepo.EXPECT().GetByID(gomock.Any(), workspaceID).Return(&domain.Workspace{ID: workspaceID}, nil)
		mockRepo.EXPECT().Update(gomock.Any(), gomock.Any()).Return(nil)

		_, _, _, err := service.ConnectZapier(context.Background(), workspaceID, label)
		require.NoError(t, err)

		// Deliberately not compared against ZapierKeyPermissions(), which would assert the
		// function against itself. What matters here is that a grant was passed at all:
		// CreateAPIKey reads nil as full workspace access, so an omitted argument would mint
		// a key that can do everything.
		require.NotNil(t, stored)
		assert.NotEmpty(t, stored)
		assert.NotEqual(t, domain.NewFullPermissions(), stored)
	})

	t.Run("retries with fresh randomness when the address is taken", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		service, mockRepo, mockUserRepo, mockAuthService := newZapierTestService(t, ctrl)

		ownerAuth(mockAuthService, 4)

		// Driven through CreateUser so the error ConnectZapier sees is the one CreateAPIKey
		// really returns: *ErrUserExists wrapped in a fmt.Errorf. A retry written with
		// errors.Is against a sentinel would never fire on it.
		var attempted []string
		gomock.InOrder(
			mockUserRepo.EXPECT().CreateUser(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, user *domain.User) error {
				attempted = append(attempted, user.Email)
				return &domain.ErrUserExists{Message: "user already exists"}
			}),
			mockUserRepo.EXPECT().CreateUser(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, user *domain.User) error {
				attempted = append(attempted, user.Email)
				return &domain.ErrUserExists{Message: "user already exists"}
			}),
			mockUserRepo.EXPECT().CreateUser(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, user *domain.User) error {
				attempted = append(attempted, user.Email)
				return nil
			}),
		)

		mockRepo.EXPECT().AddUserToWorkspace(gomock.Any(), gomock.Any()).Return(nil)
		mockAuthService.EXPECT().GenerateAPIAuthToken(gomock.Any()).Return("zapier-token")
		mockRepo.EXPECT().GetByID(gomock.Any(), workspaceID).Return(&domain.Workspace{ID: workspaceID}, nil)
		mockRepo.EXPECT().Update(gomock.Any(), gomock.Any()).Return(nil)

		token, email, integrationID, err := service.ConnectZapier(context.Background(), workspaceID, label)
		require.NoError(t, err)
		assert.Equal(t, "zapier-token", token)
		assert.NotEmpty(t, integrationID)

		require.Equal(t, 3, len(attempted))
		assert.Equal(t, attempted[2], email)
		distinct := map[string]struct{}{}
		for _, address := range attempted {
			assert.Regexp(t, mintedAddress, address)
			distinct[address] = struct{}{}
		}
		assert.Equal(t, len(attempted), len(distinct), "each attempt must draw fresh randomness")
	})

	t.Run("gives up on a permanent collision without writing an integration", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		// mockRepo is left unnamed on purpose: with no expectations on it, a GetByID or an
		// Update from an exhausted retry fails the subtest.
		service, _, mockUserRepo, mockAuthService := newZapierTestService(t, ctrl)

		mockAuthService.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
			DoAndReturn(func(c context.Context, _ string) (context.Context, *domain.User, *domain.UserWorkspace, error) {
				return c, &domain.User{ID: userID}, &domain.UserWorkspace{
					UserID:      userID,
					WorkspaceID: workspaceID,
					Role:        "owner",
				}, nil
			}).
			AnyTimes()

		attempts := 0
		mockUserRepo.EXPECT().CreateUser(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, _ *domain.User) error {
			attempts++
			return &domain.ErrUserExists{Message: "user already exists"}
		}).AnyTimes()

		token, email, integrationID, err := service.ConnectZapier(context.Background(), workspaceID, label)
		require.Error(t, err)
		assert.Empty(t, token)
		assert.Empty(t, email)
		assert.Empty(t, integrationID)
		// A literal rather than the constant it protects: comparing against
		// zapierConnectAttempts would pass however the loop is bounded, including at one.
		assert.Equal(t, 5, attempts)

		// Still wrapping the struct error, so the handler maps an exhausted retry to 409 the
		// same way it maps the first collision.
		var userExistsErr *domain.ErrUserExists
		assert.True(t, errors.As(err, &userExistsErr), "the exhausted error must still carry *domain.ErrUserExists")
	})

	t.Run("revokes the key when the integration cannot be saved", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		service, mockRepo, mockUserRepo, mockAuthService := newZapierTestService(t, ctrl)

		ownerAuth(mockAuthService, 2)

		apiKeyUserID := "api-key-user"
		var mintedEmail string
		mockUserRepo.EXPECT().CreateUser(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, user *domain.User) error {
			mintedEmail = user.Email
			return nil
		})
		mockRepo.EXPECT().AddUserToWorkspace(gomock.Any(), gomock.Any()).Return(nil)
		mockAuthService.EXPECT().GenerateAPIAuthToken(gomock.Any()).Return("zapier-token")
		mockRepo.EXPECT().GetByID(gomock.Any(), workspaceID).Return(&domain.Workspace{ID: workspaceID}, nil)
		mockRepo.EXPECT().Update(gomock.Any(), gomock.Any()).Return(fmt.Errorf("workspace update failed"))

		// The key is live and no card points at it, so both halves of the revocation have to
		// run: deleting the user is the only revocation there is, and a membership row left
		// pointing at a deleted user can never be removed afterwards.
		mockUserRepo.EXPECT().GetUserByEmail(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, email string) (*domain.User, error) {
			assert.Equal(t, mintedEmail, email)
			return &domain.User{ID: apiKeyUserID, Email: email, Type: domain.UserTypeAPIKey}, nil
		})
		removed := false
		mockRepo.EXPECT().RemoveUserFromWorkspace(gomock.Any(), apiKeyUserID, workspaceID).DoAndReturn(func(_ context.Context, _ string, _ string) error {
			removed = true
			return nil
		})
		deleted := false
		mockUserRepo.EXPECT().Delete(gomock.Any(), apiKeyUserID).DoAndReturn(func(_ context.Context, _ string) error {
			assert.True(t, removed, "the membership must be removed before the user")
			deleted = true
			return nil
		})

		token, email, integrationID, err := service.ConnectZapier(context.Background(), workspaceID, label)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "workspace update failed")
		assert.Empty(t, token)
		assert.Empty(t, email)
		assert.Empty(t, integrationID)
		assert.True(t, removed)
		assert.True(t, deleted)
	})

	t.Run("revokes the key even when the request context is already cancelled", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		service, mockRepo, mockUserRepo, mockAuthService := newZapierTestService(t, ctrl)

		ownerAuth(mockAuthService, 2)

		// A client disconnect is one of the ways the workspace write fails, so the
		// compensation cannot run on the context that just died.
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		mockUserRepo.EXPECT().CreateUser(gomock.Any(), gomock.Any()).Return(nil)
		mockRepo.EXPECT().AddUserToWorkspace(gomock.Any(), gomock.Any()).Return(nil)
		mockAuthService.EXPECT().GenerateAPIAuthToken(gomock.Any()).Return("zapier-token")
		mockRepo.EXPECT().GetByID(gomock.Any(), workspaceID).Return(&domain.Workspace{ID: workspaceID}, nil)
		mockRepo.EXPECT().Update(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, _ *domain.Workspace) error {
			cancel()
			return context.Canceled
		})

		mockUserRepo.EXPECT().GetUserByEmail(gomock.Any(), gomock.Any()).DoAndReturn(func(c context.Context, email string) (*domain.User, error) {
			require.NoError(t, c.Err(), "the compensation must not inherit the cancelled context")
			return &domain.User{ID: "api-key-user", Email: email, Type: domain.UserTypeAPIKey}, nil
		})
		mockRepo.EXPECT().RemoveUserFromWorkspace(gomock.Any(), "api-key-user", workspaceID).DoAndReturn(func(c context.Context, _ string, _ string) error {
			require.NoError(t, c.Err())
			return nil
		})
		deleted := false
		mockUserRepo.EXPECT().Delete(gomock.Any(), "api-key-user").DoAndReturn(func(c context.Context, _ string) error {
			require.NoError(t, c.Err())
			deleted = true
			return nil
		})

		_, _, _, err := service.ConnectZapier(ctx, workspaceID, label)
		require.Error(t, err)
		assert.True(t, deleted, "the key must still be revoked after the request context is cancelled")
	})
}

// Updating an integration with a body that says nothing about the provider.
//
// The wholesale assignment in UpdateIntegration used to overwrite the stored
// provider with the zero EmailProvider, which validates clean (an empty Kind
// reads as "not configured") and takes kind, senders, rate limit and the
// encrypted credential with it. Neither rescue helper could see it: both key
// off the incoming provider's own pointers, and with no provider sent they are
// all nil. A workspace never serves its credentials back, so nothing could put
// them back and the workspace stopped sending.
//
// Decoded from a raw body on purpose: a struct literal cannot express a key
// that is not there, which is exactly how this shipped.
func TestUpdateIntegration_OmittedProviderKeepsStoredEmailProvider(t *testing.T) {
	const workspaceID = "testworkspace"
	const integrationID = "integration123"

	// Rebuilt per call so the expectation never aliases the pointers the
	// service mutates — comparing a field against itself always passes.
	storedEmailProvider := func() domain.EmailProvider {
		return domain.EmailProvider{
			Kind:               domain.EmailProviderKindSES,
			RateLimitPerMinute: 42,
			Senders: []domain.EmailSender{
				// A real UUID: EmailProvider.Validate regenerates any sender id that
				// is not one, which would show up here as a spurious difference.
				{ID: "11111111-1111-4111-8111-111111111111", Email: "stored@example.com", Name: "Stored", IsDefault: true},
			},
			SES: &domain.AmazonSESSettings{
				Region:                  "eu-west-3",
				AccessKey:               "AKIASTORED",
				EncryptedSecretKey:      "STORED-CIPHERTEXT",
				ManagedTenantName:       "tenant-1",
				ManagedConfigurationSet: "config-set-1",
				InboundTopicARN:         "arn:aws:sns:eu-west-3:1:inbound",
			},
		}
	}

	run := func(t *testing.T, body string) *domain.Integration {
		t.Helper()

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRepo := mocks.NewMockWorkspaceRepository(ctrl)
		mockAuthService := mocks.NewMockAuthService(ctrl)
		mockLogger := pkgmocks.NewMockLogger(ctrl)
		mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
		mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
		mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()

		service := NewWorkspaceService(
			mockRepo,
			mocks.NewMockUserRepository(ctrl),
			mocks.NewMockTaskRepository(ctrl),
			mockLogger,
			mocks.NewMockUserServiceInterface(ctrl),
			mockAuthService,
			pkgmocks.NewMockMailer(ctrl),
			&config.Config{RootEmail: "root@example.com"},
			mocks.NewMockContactService(ctrl),
			mocks.NewMockListService(ctrl),
			mocks.NewMockContactListService(ctrl),
			mocks.NewMockTemplateService(ctrl),
			mocks.NewMockWebhookRegistrationService(ctrl),
			"secret_key",
			&SupabaseService{},
			&DNSVerificationService{},
			&BlogService{},
		)

		ctx := context.Background()
		mockAuthService.EXPECT().
			AuthenticateUserForWorkspace(ctx, workspaceID).
			Return(ctx, &domain.User{ID: "u1"}, &domain.UserWorkspace{
				UserID: "u1", WorkspaceID: workspaceID, Role: "owner",
			}, nil)

		mockRepo.EXPECT().GetByID(ctx, workspaceID).Return(&domain.Workspace{
			ID:   workspaceID,
			Name: "Test",
			Integrations: domain.Integrations{{
				ID:            integrationID,
				Name:          "Original",
				Type:          domain.IntegrationTypeEmail,
				EmailProvider: storedEmailProvider(),
			}},
		}, nil)

		var saved *domain.Workspace
		mockRepo.EXPECT().Update(ctx, gomock.Any()).
			DoAndReturn(func(_ context.Context, w *domain.Workspace) error {
				saved = w
				return nil
			})

		var req domain.UpdateIntegrationRequest
		require.NoError(t, json.Unmarshal([]byte(body), &req))

		require.NoError(t, service.UpdateIntegration(ctx, req))
		require.NotNil(t, saved)
		got := saved.GetIntegrationByID(integrationID)
		require.NotNil(t, got)
		return got
	}

	t.Run("rename only - stored provider survives", func(t *testing.T) {
		got := run(t, `{
			"workspace_id": "testworkspace",
			"integration_id": "integration123",
			"name": "Renamed"
		}`)

		assert.Equal(t, "Renamed", got.Name, "the edit itself must still apply")
		assert.Equal(t, storedEmailProvider(), got.EmailProvider,
			"an absent provider must leave the stored one untouched, credential included: "+
				"workspaces never serve credentials back, so a wipe is unrecoverable")
	})

	t.Run("provider sent - replaces the stored one", func(t *testing.T) {
		got := run(t, `{
			"workspace_id": "testworkspace",
			"integration_id": "integration123",
			"name": "Renamed",
			"provider": {
				"kind": "smtp",
				"rate_limit_per_minute": 10,
				"senders": [{"id": "s1", "email": "new@example.com", "name": "New", "is_default": true}],
				"smtp": {"host": "smtp.example.com", "port": 587, "username": "u", "password": "p", "use_tls": true}
			}
		}`)

		assert.Equal(t, domain.EmailProviderKindSMTP, got.EmailProvider.Kind)
		assert.Equal(t, 10, got.EmailProvider.RateLimitPerMinute)
		require.Len(t, got.EmailProvider.Senders, 1)
		assert.Equal(t, "new@example.com", got.EmailProvider.Senders[0].Email)
	})
}

// Settings an update body never names must keep the value the workspace already
// has. The assignment block used to copy the whole request over the stored
// settings, so a body that mentioned only the timezone turned email tracking off,
// unassigned both sending providers and emptied the file manager — the S3 secret
// with it, which no client can restore because a workspace does not serve it back
// to anything but a console session.
//
// The settings come from a decoded body, not a struct literal: a literal cannot
// express a key that is not there, which is the whole defect.
func TestUpdateWorkspace_OmittedSettingsKeepStoredValues(t *testing.T) {
	const workspaceID = "testworkspace"

	storedEndpoint := "https://track.example.com"

	storedSettings := func() domain.WorkspaceSettings {
		return domain.WorkspaceSettings{
			WebsiteURL:      "https://stored.example.com",
			LogoURL:         "https://stored.example.com/logo.png",
			CoverURL:        "https://stored.example.com/cover.png",
			Timezone:        "UTC",
			DefaultLanguage: "en",
			Languages:       []string{"en"},
			FileManager: domain.FileManagerSettings{
				Endpoint:           "https://s3.example.com",
				Bucket:             "stored-bucket",
				AccessKey:          "AKIASTORED",
				EncryptedSecretKey: "STORED-CIPHERTEXT",
			},
			TransactionalEmailProviderID: "provider-transactional",
			MarketingEmailProviderID:     "provider-marketing",
			EmailTrackingEnabled:         true,
			CustomEndpointURL:            &storedEndpoint,
		}
	}

	run := func(t *testing.T, body string) *domain.Workspace {
		t.Helper()

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRepo := mocks.NewMockWorkspaceRepository(ctrl)
		mockAuthService := mocks.NewMockAuthService(ctrl)
		mockLogger := pkgmocks.NewMockLogger(ctrl)
		mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
		mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
		mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()

		service := NewWorkspaceService(
			mockRepo,
			mocks.NewMockUserRepository(ctrl),
			mocks.NewMockTaskRepository(ctrl),
			mockLogger,
			mocks.NewMockUserServiceInterface(ctrl),
			mockAuthService,
			pkgmocks.NewMockMailer(ctrl),
			&config.Config{RootEmail: "root@example.com"},
			mocks.NewMockContactService(ctrl),
			mocks.NewMockListService(ctrl),
			mocks.NewMockContactListService(ctrl),
			mocks.NewMockTemplateService(ctrl),
			mocks.NewMockWebhookRegistrationService(ctrl),
			"secret_key",
			&SupabaseService{},
			&DNSVerificationService{},
			&BlogService{},
		)

		ctx := context.Background()
		mockAuthService.EXPECT().
			AuthenticateUserForWorkspace(ctx, workspaceID).
			Return(ctx, &domain.User{ID: "u1"}, &domain.UserWorkspace{
				UserID: "u1", WorkspaceID: workspaceID, Role: "owner",
			}, nil)

		mockRepo.EXPECT().GetByID(ctx, workspaceID).Return(&domain.Workspace{
			ID:       workspaceID,
			Name:     "Original",
			Settings: storedSettings(),
		}, nil)

		var saved *domain.Workspace
		mockRepo.EXPECT().Update(ctx, gomock.Any()).
			DoAndReturn(func(_ context.Context, w *domain.Workspace) error {
				saved = w
				return nil
			})

		var req domain.UpdateWorkspaceRequest
		require.NoError(t, json.Unmarshal([]byte(body), &req))

		// Exactly what the handler does with the decoded request.
		_, err := service.UpdateWorkspace(ctx, req.ID, req.Name, req.Settings)
		require.NoError(t, err)
		require.NotNil(t, saved)
		return saved
	}

	t.Run("a minimal body changes only the name", func(t *testing.T) {
		saved := run(t, `{
			"id": "testworkspace",
			"name": "Renamed",
			"settings": {"timezone": "UTC", "default_language": "en", "languages": ["en"]}
		}`)

		assert.Equal(t, "Renamed", saved.Name, "the edit itself must still apply")
		assert.Equal(t, storedSettings(), saved.Settings,
			"every setting the body did not name must survive the save")
	})

	t.Run("file manager sent without its secret keeps the stored one", func(t *testing.T) {
		// The shape any API client sends back: reads never carry the S3 secret to a
		// machine caller, so an echo of the settings object has nothing to put there.
		saved := run(t, `{
			"id": "testworkspace",
			"name": "Renamed",
			"settings": {
				"timezone": "UTC", "default_language": "en", "languages": ["en"],
				"file_manager": {
					"endpoint": "https://s3.example.com",
					"bucket": "new-bucket",
					"access_key": "AKIASTORED"
				}
			}
		}`)

		assert.Equal(t, "new-bucket", saved.Settings.FileManager.Bucket, "the edit itself must still apply")
		assert.Equal(t, "STORED-CIPHERTEXT", saved.Settings.FileManager.EncryptedSecretKey,
			"a caller with no secret to send must not blank the stored one")
	})

	t.Run("values the body does name still apply", func(t *testing.T) {
		// The counterpart: preserving on absence must not cost the ability to change
		// or clear anything. null clears a URL, which is how the console clears one.
		saved := run(t, `{
			"id": "testworkspace",
			"name": "Renamed",
			"settings": {
				"timezone": "UTC", "default_language": "en", "languages": ["en"],
				"email_tracking_enabled": false,
				"logo_url": null,
				"transactional_email_provider_id": "",
				"file_manager": {}
			}
		}`)

		assert.False(t, saved.Settings.EmailTrackingEnabled, "an explicit false must still turn tracking off")
		assert.Empty(t, saved.Settings.LogoURL, "a null must still clear the logo")
		assert.Empty(t, saved.Settings.TransactionalEmailProviderID, "an explicit empty string must still unassign")
		assert.Empty(t, saved.Settings.FileManager.EncryptedSecretKey,
			"clearing the file manager outright must take its credential with it")
		assert.Equal(t, "provider-marketing", saved.Settings.MarketingEmailProviderID,
			"the sibling the body did not name is untouched")
	})
}

// setBlogSettings writes both fields on every call, so a body that says nothing
// about blog_enabled used to take the blog offline as a side effect of editing its
// title. The console papers over it by recomputing the flag from the workspace it
// holds; nothing else can, and the service is the only place that knows the stored
// value.
func TestSetBlogSettings_OmittedEnabledFlagKeepsStoredValue(t *testing.T) {
	const workspaceID = "testworkspace"

	run := func(t *testing.T, enabled *bool) *domain.Workspace {
		t.Helper()

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRepo := mocks.NewMockWorkspaceRepository(ctrl)
		mockAuthService := mocks.NewMockAuthService(ctrl)
		mockLogger := pkgmocks.NewMockLogger(ctrl)
		mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
		mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
		mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()

		service := NewWorkspaceService(
			mockRepo,
			mocks.NewMockUserRepository(ctrl),
			mocks.NewMockTaskRepository(ctrl),
			mockLogger,
			mocks.NewMockUserServiceInterface(ctrl),
			mockAuthService,
			pkgmocks.NewMockMailer(ctrl),
			&config.Config{RootEmail: "root@example.com"},
			mocks.NewMockContactService(ctrl),
			mocks.NewMockListService(ctrl),
			mocks.NewMockContactListService(ctrl),
			mocks.NewMockTemplateService(ctrl),
			mocks.NewMockWebhookRegistrationService(ctrl),
			"secret_key",
			&SupabaseService{},
			&DNSVerificationService{},
			&BlogService{},
		)

		ctx := context.Background()
		mockAuthService.EXPECT().
			AuthenticateUserForWorkspace(ctx, workspaceID).
			Return(ctx, &domain.User{ID: "u1"}, &domain.UserWorkspace{
				UserID: "u1", WorkspaceID: workspaceID, Role: "owner",
			}, nil)

		mockRepo.EXPECT().GetByID(ctx, workspaceID).Return(&domain.Workspace{
			ID:   workspaceID,
			Name: "Test",
			Settings: domain.WorkspaceSettings{
				Timezone:     "UTC",
				BlogEnabled:  true,
				BlogSettings: &domain.BlogSettings{Title: "Stored Blog"},
			},
		}, nil)

		var saved *domain.Workspace
		mockRepo.EXPECT().Update(ctx, gomock.Any()).
			DoAndReturn(func(_ context.Context, w *domain.Workspace) error {
				saved = w
				return nil
			})

		require.NoError(t, service.SetBlogSettings(ctx, workspaceID, enabled, &domain.BlogSettings{Title: "Edited"}, true))
		require.NotNil(t, saved)
		return saved
	}

	t.Run("no flag in the body leaves a live blog live", func(t *testing.T) {
		saved := run(t, nil)

		assert.True(t, saved.Settings.BlogEnabled,
			"editing the blog's settings must not take it offline")
		require.NotNil(t, saved.Settings.BlogSettings)
		assert.Equal(t, "Edited", saved.Settings.BlogSettings.Title, "the edit itself must still apply")
	})

	t.Run("an explicit false still disables the blog", func(t *testing.T) {
		saved := run(t, boolPtr(false))

		assert.False(t, saved.Settings.BlogEnabled)
	})
}

// The settings block needs its own presence answer, because unlike the flag above it cannot
// be read off the pointer: nil is already how a caller asks for the configuration to be
// cleared. Merging it here rather than in the handler is what lets the write use the very
// workspace it has just loaded and is about to save, instead of a second read taken through
// a different lookup with a different answer to who may see the workspace.
func TestSetBlogSettings_UnspecifiedSettingsKeepTheStoredConfiguration(t *testing.T) {
	const workspaceID = "testworkspace"

	// Richly seeded on purpose: against an empty stored value a wipe and a correct merge
	// look the same.
	storedSettings := func() *domain.BlogSettings {
		return &domain.BlogSettings{
			Title:            "Stored Blog",
			HomePageSize:     12,
			CategoryPageSize: 7,
			FeedSummaryOnly:  true,
			FeedMaxItems:     5,
			SEO: &domain.SEOSettings{
				MetaTitle:       "Stored meta title",
				MetaDescription: "Stored meta description",
				Keywords:        []string{"stored", "keywords"},
			},
		}
	}

	run := func(t *testing.T, settings *domain.BlogSettings, settingsSpecified bool) *domain.Workspace {
		t.Helper()

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRepo := mocks.NewMockWorkspaceRepository(ctrl)
		mockAuthService := mocks.NewMockAuthService(ctrl)
		mockLogger := pkgmocks.NewMockLogger(ctrl)
		mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
		mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
		mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()

		service := NewWorkspaceService(
			mockRepo,
			mocks.NewMockUserRepository(ctrl),
			mocks.NewMockTaskRepository(ctrl),
			mockLogger,
			mocks.NewMockUserServiceInterface(ctrl),
			mockAuthService,
			pkgmocks.NewMockMailer(ctrl),
			&config.Config{RootEmail: "root@example.com"},
			mocks.NewMockContactService(ctrl),
			mocks.NewMockListService(ctrl),
			mocks.NewMockContactListService(ctrl),
			mocks.NewMockTemplateService(ctrl),
			mocks.NewMockWebhookRegistrationService(ctrl),
			"secret_key",
			&SupabaseService{},
			&DNSVerificationService{},
			&BlogService{},
		)

		ctx := context.Background()
		mockAuthService.EXPECT().
			AuthenticateUserForWorkspace(ctx, workspaceID).
			Return(ctx, &domain.User{ID: "u1"}, &domain.UserWorkspace{
				UserID: "u1", WorkspaceID: workspaceID, Role: "owner",
			}, nil)

		mockRepo.EXPECT().GetByID(ctx, workspaceID).Return(&domain.Workspace{
			ID:   workspaceID,
			Name: "Test",
			Settings: domain.WorkspaceSettings{
				Timezone:     "UTC",
				WebsiteURL:   "https://example.com",
				BlogEnabled:  true,
				BlogSettings: storedSettings(),
			},
		}, nil)

		var saved *domain.Workspace
		mockRepo.EXPECT().Update(ctx, gomock.Any()).
			DoAndReturn(func(_ context.Context, w *domain.Workspace) error {
				saved = w
				return nil
			}).AnyTimes()

		require.NoError(t, service.SetBlogSettings(ctx, workspaceID, boolPtr(false), settings, settingsSpecified))
		require.NotNil(t, saved)
		return saved
	}

	t.Run("a body that named no blog_settings leaves the whole configuration standing", func(t *testing.T) {
		saved := run(t, nil, false)

		// Against literals rather than the fixture: the settings the write keeps are the very
		// pointer the read handed it, so comparing them to the fixture would risk comparing
		// them to themselves.
		require.NotNil(t, saved.Settings.BlogSettings,
			"switching the blog off must not erase how it is configured")
		assert.Equal(t, "Stored Blog", saved.Settings.BlogSettings.Title)
		assert.Equal(t, 12, saved.Settings.BlogSettings.HomePageSize)
		assert.Equal(t, 7, saved.Settings.BlogSettings.CategoryPageSize)
		assert.True(t, saved.Settings.BlogSettings.FeedSummaryOnly)
		assert.Equal(t, 5, saved.Settings.BlogSettings.FeedMaxItems)
		require.NotNil(t, saved.Settings.BlogSettings.SEO)
		assert.Equal(t, "Stored meta title", saved.Settings.BlogSettings.SEO.MetaTitle)
		assert.Equal(t, []string{"stored", "keywords"}, saved.Settings.BlogSettings.SEO.Keywords)

		// The switch the caller did flip still lands, and nothing else moves.
		assert.False(t, saved.Settings.BlogEnabled)
		assert.Equal(t, "https://example.com", saved.Settings.WebsiteURL)
	})

	t.Run("an explicit null still clears the configuration", func(t *testing.T) {
		saved := run(t, nil, true)

		assert.Nil(t, saved.Settings.BlogSettings,
			"an object has a null that means something: wiping the configuration stays expressible")
	})

	t.Run("a settings block that was sent replaces the stored one wholesale", func(t *testing.T) {
		saved := run(t, &domain.BlogSettings{Title: "Replaced"}, true)

		require.NotNil(t, saved.Settings.BlogSettings)
		assert.Equal(t, "Replaced", saved.Settings.BlogSettings.Title)
		assert.Zero(t, saved.Settings.BlogSettings.HomePageSize,
			"the block is a replacement, not a per-field merge")
		assert.Nil(t, saved.Settings.BlogSettings.SEO)
	})
}
