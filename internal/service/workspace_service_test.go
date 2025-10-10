package service

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
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

		expectedUserWorkspace := &domain.UserWorkspace{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Role:        "owner",
		}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, expectedUser, nil, nil)
		mockRepo.EXPECT().GetUserWorkspace(ctx, userID, workspaceID).Return(expectedUserWorkspace, nil)
		mockRepo.EXPECT().GetByID(ctx, workspaceID).Return(expectedWorkspace, nil)

		workspace, err := service.GetWorkspace(ctx, workspaceID)
		require.NoError(t, err)
		assert.Equal(t, expectedWorkspace, workspace)
	})

	t.Run("workspace not found", func(t *testing.T) {
		expectedUser := &domain.User{
			ID: userID,
		}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, expectedUser, nil, nil)
		mockRepo.EXPECT().GetUserWorkspace(ctx, userID, workspaceID).Return(nil, assert.AnError)

		workspace, err := service.GetWorkspace(ctx, workspaceID)
		require.Error(t, err)
		assert.Nil(t, workspace)
		assert.Equal(t, assert.AnError, err)
	})

	t.Run("error getting workspace by ID", func(t *testing.T) {
		expectedUser := &domain.User{
			ID: userID,
		}

		expectedUserWorkspace := &domain.UserWorkspace{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Role:        "owner",
		}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, expectedUser, nil, nil)
		mockRepo.EXPECT().GetUserWorkspace(ctx, userID, workspaceID).Return(expectedUserWorkspace, nil)
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
		})
		require.NoError(t, err)
		assert.Equal(t, workspaceID, workspace.ID)
		assert.Equal(t, "Test Workspace", workspace.Name)

		// Verify the structure of settings but don't check the exact SecretKey value
		assert.Equal(t, "https://example.com", workspace.Settings.WebsiteURL)
		assert.Equal(t, "https://example.com/logo.png", workspace.Settings.LogoURL)
		assert.Equal(t, "https://example.com/cover.png", workspace.Settings.CoverURL)
		assert.Equal(t, "UTC", workspace.Settings.Timezone)

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
		})
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
		})
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
		})
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
		})
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
		})
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
		})

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
		})
		require.Error(t, err)
		assert.Nil(t, workspace)
		assert.Contains(t, err.Error(), "workspace already exists")
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
			WebsiteURL: "https://example.com",
			LogoURL:    "https://example.com/logo.png",
			CoverURL:   "https://example.com/cover.png",
			Timezone:   "UTC",
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

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, expectedUser, nil, nil)
		mockRepo.EXPECT().GetUserWorkspace(ctx, userID, workspaceID).Return(expectedUserWorkspace, nil)
		mockRepo.EXPECT().GetByID(ctx, workspaceID).Return(existingWorkspace, nil)
		mockRepo.EXPECT().Update(ctx, gomock.Any()).Return(nil)

		workspace, err := service.UpdateWorkspace(ctx, workspaceID, "Updated Workspace", settings)
		require.NoError(t, err)
		assert.Equal(t, expectedWorkspace.ID, workspace.ID)
		assert.Equal(t, expectedWorkspace.Name, workspace.Name)
		assert.Equal(t, expectedWorkspace.Settings, workspace.Settings)
	})

	t.Run("authentication error", func(t *testing.T) {
		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, nil, nil, assert.AnError)

		settings := domain.WorkspaceSettings{
			WebsiteURL: "https://example.com",
			LogoURL:    "https://example.com/logo.png",
			CoverURL:   "https://example.com/cover.png",
			Timezone:   "UTC",
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
			WebsiteURL: "https://example.com",
			LogoURL:    "https://example.com/logo.png",
			CoverURL:   "https://example.com/cover.png",
			Timezone:   "UTC",
			FileManager: domain.FileManagerSettings{
				Endpoint:  "https://s3.amazonaws.com",
				Bucket:    "my-bucket",
				AccessKey: "AKIAIOSFODNN7EXAMPLE",
			},
		}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, expectedUser, nil, nil)
		mockRepo.EXPECT().GetUserWorkspace(ctx, userID, workspaceID).Return(expectedUserWorkspace, nil)

		workspace, err := service.UpdateWorkspace(ctx, workspaceID, "Updated Workspace", settings)
		require.Error(t, err)
		assert.Nil(t, workspace)
		assert.IsType(t, &domain.ErrUnauthorized{}, err)
	})

	t.Run("error getting user workspace", func(t *testing.T) {
		expectedUser := &domain.User{
			ID: userID,
		}

		settings := domain.WorkspaceSettings{
			WebsiteURL: "https://example.com",
			LogoURL:    "https://example.com/logo.png",
			CoverURL:   "https://example.com/cover.png",
			Timezone:   "UTC",
			FileManager: domain.FileManagerSettings{
				Endpoint:  "https://s3.amazonaws.com",
				Bucket:    "my-bucket",
				AccessKey: "AKIAIOSFODNN7EXAMPLE",
			},
		}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, expectedUser, nil, nil)
		mockRepo.EXPECT().GetUserWorkspace(ctx, userID, workspaceID).Return(nil, assert.AnError)

		workspace, err := service.UpdateWorkspace(ctx, workspaceID, "Updated Workspace", settings)
		require.Error(t, err)
		assert.Nil(t, workspace)
		assert.Equal(t, assert.AnError, err)
	})

	t.Run("successful update with custom field labels", func(t *testing.T) {
		expectedUser := &domain.User{
			ID: userID,
		}

		expectedUserWorkspace := &domain.UserWorkspace{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Role:        "owner",
		}

		customFieldLabels := map[string]string{
			"custom_string_1":   "Company Name",
			"custom_number_1":   "Revenue",
			"custom_datetime_1": "Contract Start",
			"custom_json_1":     "Metadata",
		}

		settings := domain.WorkspaceSettings{
			WebsiteURL: "https://example.com",
			LogoURL:    "https://example.com/logo.png",
			CoverURL:   "https://example.com/cover.png",
			Timezone:   "UTC",
			FileManager: domain.FileManagerSettings{
				Endpoint:  "https://s3.amazonaws.com",
				Bucket:    "my-bucket",
				AccessKey: "AKIAIOSFODNN7EXAMPLE",
			},
			CustomFieldLabels: customFieldLabels,
		}

		existingWorkspace := &domain.Workspace{
			ID:   workspaceID,
			Name: "Original Workspace Name",
			Settings: domain.WorkspaceSettings{
				WebsiteURL: "https://old-example.com",
			},
			CreatedAt: time.Now().Add(-24 * time.Hour),
			UpdatedAt: time.Now().Add(-24 * time.Hour),
		}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, expectedUser, nil, nil)
		mockRepo.EXPECT().GetUserWorkspace(ctx, userID, workspaceID).Return(expectedUserWorkspace, nil)
		mockRepo.EXPECT().GetByID(ctx, workspaceID).Return(existingWorkspace, nil)
		mockRepo.EXPECT().Update(ctx, gomock.Any()).DoAndReturn(func(ctx context.Context, workspace *domain.Workspace) error {
			// Verify custom field labels are correctly set
			assert.NotNil(t, workspace.Settings.CustomFieldLabels)
			assert.Equal(t, customFieldLabels, workspace.Settings.CustomFieldLabels)
			assert.Equal(t, "Company Name", workspace.Settings.CustomFieldLabels["custom_string_1"])
			assert.Equal(t, "Revenue", workspace.Settings.CustomFieldLabels["custom_number_1"])
			assert.Equal(t, "Contract Start", workspace.Settings.CustomFieldLabels["custom_datetime_1"])
			assert.Equal(t, "Metadata", workspace.Settings.CustomFieldLabels["custom_json_1"])
			return nil
		})

		workspace, err := service.UpdateWorkspace(ctx, workspaceID, "Updated Workspace", settings)
		require.NoError(t, err)
		assert.NotNil(t, workspace)
		assert.Equal(t, customFieldLabels, workspace.Settings.CustomFieldLabels)
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

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, expectedUser, nil, nil)
		mockRepo.EXPECT().GetUserWorkspace(ctx, userID, workspaceID).Return(userWorkspace, nil)
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
		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, expectedUser, nil, nil)
		mockRepo.EXPECT().GetUserWorkspace(ctx, userID, workspaceID).Return(userWorkspace, nil)
		mockRepo.EXPECT().GetByID(ctx, workspaceID).Return(workspace, nil)

		// For each DeleteIntegration call inside DeleteWorkspace, expect these mocks
		// The DeleteIntegration method will call AuthenticateUserForWorkspace again for each integration
		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, expectedUser, nil, nil).Times(2)
		mockRepo.EXPECT().GetUserWorkspace(ctx, userID, workspaceID).Return(userWorkspace, nil).Times(2)
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
		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, expectedUser, nil, nil)
		mockRepo.EXPECT().GetUserWorkspace(ctx, userID, workspaceID).Return(userWorkspace, nil)
		mockRepo.EXPECT().GetByID(ctx, workspaceID).Return(workspace, nil)

		// Authentication for DeleteIntegration
		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, expectedUser, nil, nil)
		mockRepo.EXPECT().GetUserWorkspace(ctx, userID, workspaceID).Return(userWorkspace, nil)
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

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, expectedUser, nil, nil)
		mockRepo.EXPECT().GetUserWorkspace(ctx, userID, workspaceID).Return(userWorkspace, nil)

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

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, expectedUser, nil, nil)
		mockRepo.EXPECT().GetUserWorkspace(ctx, userID, workspaceID).Return(userWorkspace, nil)
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

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, expectedUser, nil, nil)
		mockRepo.EXPECT().GetUserWorkspace(ctx, userID, workspaceID).Return(expectedUserWorkspace, nil)
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

		integrationID, err := service.CreateIntegration(ctx, workspaceID, integrationName, domain.IntegrationTypeEmail, provider)
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

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, expectedUser, nil, nil)
		mockRepo.EXPECT().GetUserWorkspace(ctx, userID, workspaceID).Return(expectedUserWorkspace, nil)

		integrationID, err := service.CreateIntegration(ctx, workspaceID, integrationName, domain.IntegrationTypeEmail, provider)
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

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, expectedUser, nil, nil)
		mockRepo.EXPECT().GetUserWorkspace(ctx, userID, workspaceID).Return(expectedUserWorkspace, nil)
		mockRepo.EXPECT().GetByID(ctx, workspaceID).Return(nil, errors.New("workspace not found"))

		integrationID, err := service.CreateIntegration(ctx, workspaceID, integrationName, domain.IntegrationTypeEmail, provider)
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

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, expectedUser, nil, nil)
		mockRepo.EXPECT().GetUserWorkspace(ctx, userID, workspaceID).Return(expectedUserWorkspace, nil)
		mockRepo.EXPECT().GetByID(ctx, workspaceID).Return(expectedWorkspace, nil)
		mockRepo.EXPECT().Update(ctx, gomock.Any()).Return(errors.New("update error"))

		integrationID, err := service.CreateIntegration(ctx, workspaceID, integrationName, domain.IntegrationTypeEmail, provider)
		require.Error(t, err)
		require.Empty(t, integrationID)
		require.Contains(t, err.Error(), "update error")
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

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, expectedUser, nil, nil)
		mockRepo.EXPECT().GetUserWorkspace(ctx, userID, workspaceID).Return(expectedUserWorkspace, nil)
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

		err := service.UpdateIntegration(ctx, workspaceID, integrationID, integrationName, provider)
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

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, expectedUser, nil, nil)
		mockRepo.EXPECT().GetUserWorkspace(ctx, userID, workspaceID).Return(expectedUserWorkspace, nil)

		err := service.UpdateIntegration(ctx, workspaceID, integrationID, integrationName, provider)
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

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, expectedUser, nil, nil)
		mockRepo.EXPECT().GetUserWorkspace(ctx, userID, workspaceID).Return(expectedUserWorkspace, nil)
		mockRepo.EXPECT().GetByID(ctx, workspaceID).Return(expectedWorkspace, nil)

		err := service.UpdateIntegration(ctx, workspaceID, integrationID, integrationName, provider)
		require.Error(t, err)
		require.Contains(t, err.Error(), "integration not found")
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
				TransactionalEmailProviderID: integrationID, // Reference the integration
			},
			Integrations: []domain.Integration{existingIntegration},
		}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, expectedUser, nil, nil)
		mockRepo.EXPECT().GetUserWorkspace(ctx, userID, workspaceID).Return(expectedUserWorkspace, nil)
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

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, expectedUser, nil, nil)
		mockRepo.EXPECT().GetUserWorkspace(ctx, userID, workspaceID).Return(expectedUserWorkspace, nil)

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

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, expectedUser, nil, nil)
		mockRepo.EXPECT().GetUserWorkspace(ctx, userID, workspaceID).Return(expectedUserWorkspace, nil)
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

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, expectedUser, nil, nil)
		mockRepo.EXPECT().GetUserWorkspace(ctx, userID, workspaceID).Return(expectedUserWorkspace, nil)
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
				MarketingEmailProviderID: integrationID, // Reference the integration as marketing provider
			},
			Integrations: []domain.Integration{existingIntegration},
		}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, expectedUser, nil, nil)
		mockRepo.EXPECT().GetUserWorkspace(ctx, userID, workspaceID).Return(expectedUserWorkspace, nil)
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

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, owner, nil, nil)
		mockRepo.EXPECT().GetUserWorkspace(ctx, ownerID, workspaceID).Return(ownerWorkspace, nil)
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

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, owner, nil, nil)
		mockRepo.EXPECT().GetUserWorkspace(ctx, ownerID, workspaceID).Return(ownerWorkspace, nil)
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

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, member, nil, nil)
		mockRepo.EXPECT().GetUserWorkspace(ctx, memberID, workspaceID).Return(memberWorkspace, nil)
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

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, owner, nil, nil)
		mockRepo.EXPECT().GetUserWorkspace(ctx, ownerID, workspaceID).Return(ownerWorkspace, nil)
		mockLogger.EXPECT().WithField("workspace_id", workspaceID).Return(mockLogger)
		mockLogger.EXPECT().WithField("user_id", ownerID).Return(mockLogger)
		mockLogger.EXPECT().Error("Cannot remove self from workspace")

		err := service.RemoveMember(ctx, workspaceID, ownerID)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "cannot remove yourself from the workspace")
	})

	t.Run("error getting requester workspace", func(t *testing.T) {
		owner := &domain.User{ID: ownerID, Type: domain.UserTypeUser}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, owner, nil, nil)
		mockRepo.EXPECT().GetUserWorkspace(ctx, ownerID, workspaceID).Return(nil, errors.New("repo error"))
		mockLogger.EXPECT().WithField("workspace_id", workspaceID).Return(mockLogger)
		mockLogger.EXPECT().WithField("user_id", memberID).Return(mockLogger)
		mockLogger.EXPECT().WithField("requester_id", ownerID).Return(mockLogger)
		mockLogger.EXPECT().WithField("error", "repo error").Return(mockLogger)
		mockLogger.EXPECT().Error("Failed to get requester workspace")

		err := service.RemoveMember(ctx, workspaceID, memberID)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "repo error")
	})

	t.Run("error getting user details", func(t *testing.T) {
		owner := &domain.User{ID: ownerID, Type: domain.UserTypeUser}
		ownerWorkspace := &domain.UserWorkspace{
			UserID:      ownerID,
			WorkspaceID: workspaceID,
			Role:        "owner",
		}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, owner, nil, nil)
		mockRepo.EXPECT().GetUserWorkspace(ctx, ownerID, workspaceID).Return(ownerWorkspace, nil)
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

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, owner, nil, nil)
		mockRepo.EXPECT().GetUserWorkspace(ctx, ownerID, workspaceID).Return(ownerWorkspace, nil)
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

	t.Run("API key deletion fails but removal succeeds", func(t *testing.T) {
		owner := &domain.User{ID: ownerID, Type: domain.UserTypeUser}
		apiKeyUser := &domain.User{ID: apiKeyID, Type: domain.UserTypeAPIKey}
		ownerWorkspace := &domain.UserWorkspace{
			UserID:      ownerID,
			WorkspaceID: workspaceID,
			Role:        "owner",
		}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, owner, nil, nil)
		mockRepo.EXPECT().GetUserWorkspace(ctx, ownerID, workspaceID).Return(ownerWorkspace, nil)
		mockUserService.EXPECT().GetUserByID(ctx, apiKeyID).Return(apiKeyUser, nil)
		mockRepo.EXPECT().RemoveUserFromWorkspace(ctx, apiKeyID, workspaceID).Return(nil)
		mockUserRepo.EXPECT().Delete(ctx, apiKeyID).Return(errors.New("delete error"))
		mockLogger.EXPECT().WithField("user_id", apiKeyID).Return(mockLogger)
		mockLogger.EXPECT().WithField("error", "delete error").Return(mockLogger)
		mockLogger.EXPECT().Error("Failed to delete API key user")

		err := service.RemoveMember(ctx, workspaceID, apiKeyID)
		assert.NoError(t, err) // Should not return error even if API key deletion fails
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

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, owner, nil, nil)
		mockRepo.EXPECT().GetUserWorkspace(ctx, ownerID, workspaceID).Return(ownerWorkspace, nil)
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

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, member, nil, nil)
		mockRepo.EXPECT().GetUserWorkspace(ctx, "member123", workspaceID).Return(memberWorkspace, nil)

		err := service.SetUserPermissions(ctx, workspaceID, targetUserID, permissions)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "only workspace owners can manage user permissions")
	})

	t.Run("target user not member", func(t *testing.T) {
		owner := &domain.User{ID: ownerID}
		ownerWorkspace := &domain.UserWorkspace{
			UserID:      ownerID,
			WorkspaceID: workspaceID,
			Role:        "owner",
		}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, owner, nil, nil)
		mockRepo.EXPECT().GetUserWorkspace(ctx, ownerID, workspaceID).Return(ownerWorkspace, nil)
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

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, owner, nil, nil)
		mockRepo.EXPECT().GetUserWorkspace(ctx, ownerID, workspaceID).Return(ownerWorkspace, nil)
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

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, owner, nil, nil)
		mockRepo.EXPECT().GetUserWorkspace(ctx, ownerID, workspaceID).Return(ownerWorkspace, nil)
		mockRepo.EXPECT().GetUserWorkspace(ctx, targetUserID, workspaceID).Return(targetUserWorkspace, nil)
		mockRepo.EXPECT().UpdateUserWorkspacePermissions(ctx, gomock.Any()).Return(nil)
		mockUserRepo.EXPECT().GetSessionsByUserID(ctx, targetUserID).Return(nil, errors.New("session error"))

		err := service.SetUserPermissions(ctx, workspaceID, targetUserID, permissions)
		require.NoError(t, err) // Should still succeed despite session error
	})
}
