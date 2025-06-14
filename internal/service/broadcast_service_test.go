package service

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
	"github.com/Notifuse/notifuse/pkg/notifuse_mjml"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBroadcastService_CreateBroadcast(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockBroadcastRepository(ctrl)
	mockEmailSvc := mocks.NewMockEmailServiceInterface(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockContactRepo := mocks.NewMockContactRepository(ctrl)
	mockTemplateSvc := mocks.NewMockTemplateService(ctrl)
	mockAuthSvc := mocks.NewMockAuthService(ctrl)
	mockTaskService := mocks.NewMockTaskService(ctrl)
	mockEventBus := mocks.NewMockEventBus(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)

	// Set up logger mock to return itself for chaining
	mockLoggerWithFields := pkgmocks.NewMockLogger(ctrl)
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLoggerWithFields).AnyTimes()
	mockLoggerWithFields.EXPECT().Error(gomock.Any()).AnyTimes()
	mockLoggerWithFields.EXPECT().Info(gomock.Any()).AnyTimes()

	// Add direct logger method expectations
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()

	service := NewBroadcastService(mockLogger, mockRepo, mockWorkspaceRepo, mockEmailSvc, mockContactRepo, mockTemplateSvc, mockTaskService, mockAuthSvc, mockEventBus, "https://api.example.com")

	// Then manually set the repositories needed for testing
	service.repo = mockRepo
	service.emailSvc = mockEmailSvc
	service.contactRepo = mockContactRepo
	service.templateSvc = mockTemplateSvc

	t.Run("Success", func(t *testing.T) {
		ctx := context.Background()
		now := time.Now().UTC()
		request := &domain.CreateBroadcastRequest{
			WorkspaceID: "ws123",
			Name:        "Test Broadcast",
			Audience: domain.AudienceSettings{
				Lists:               []string{"list123"},
				ExcludeUnsubscribed: true,
				SkipDuplicateEmails: true,
			},
			Schedule: domain.ScheduleSettings{
				IsScheduled: false,
			},
			TestSettings: domain.BroadcastTestSettings{
				Enabled: false,
			},
		}

		// Mock auth service to authenticate the user
		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), request.WorkspaceID).
			Return(ctx, &domain.User{ID: "user123"}, nil)

		// Mock repository to expect a broadcast to be created
		mockRepo.EXPECT().
			CreateBroadcast(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, broadcast *domain.Broadcast) error {
				// Verify important properties
				assert.Equal(t, request.WorkspaceID, broadcast.WorkspaceID)
				assert.Equal(t, request.Name, broadcast.Name)
				assert.Equal(t, domain.BroadcastStatusDraft, broadcast.Status)
				assert.Equal(t, request.Audience, broadcast.Audience)
				assert.Equal(t, request.Schedule, broadcast.Schedule)
				assert.Equal(t, request.TestSettings, broadcast.TestSettings)
				assert.NotEmpty(t, broadcast.ID)
				assert.WithinDuration(t, now, broadcast.CreatedAt, 2*time.Second)
				return nil
			})

		// Call the service
		result, err := service.CreateBroadcast(ctx, request)

		// Verify results
		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, request.WorkspaceID, result.WorkspaceID)
		assert.Equal(t, request.Name, result.Name)
		assert.Equal(t, domain.BroadcastStatusDraft, result.Status)
	})

	t.Run("AuthenticationError", func(t *testing.T) {
		ctx := context.Background()
		request := &domain.CreateBroadcastRequest{
			WorkspaceID: "ws123",
			Name:        "Test Broadcast",
			Audience: domain.AudienceSettings{
				Lists:               []string{"list123"},
				ExcludeUnsubscribed: true,
				SkipDuplicateEmails: true,
			},
		}

		// Mock auth service to return authentication error
		authErr := errors.New("authentication failed")
		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), request.WorkspaceID).
			Return(nil, nil, authErr)

		// We expect no repository calls due to authentication failure
		mockRepo.EXPECT().CreateBroadcast(gomock.Any(), gomock.Any()).Times(0)

		// Call the service
		result, err := service.CreateBroadcast(ctx, request)

		// Verify results
		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "failed to authenticate user")
	})

	t.Run("ValidationError", func(t *testing.T) {
		ctx := context.Background()
		// Create an invalid request (missing required fields)
		request := &domain.CreateBroadcastRequest{
			WorkspaceID: "ws123",
			// Missing Name and other required fields
		}

		// Mock auth service to authenticate the user
		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), request.WorkspaceID).
			Return(ctx, &domain.User{ID: "user123"}, nil)

		// We expect validation to fail, no repository calls
		mockRepo.EXPECT().CreateBroadcast(gomock.Any(), gomock.Any()).Times(0)

		// Call the service
		result, err := service.CreateBroadcast(ctx, request)

		// Verify results
		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "required")
	})

	t.Run("RepositoryError", func(t *testing.T) {
		ctx := context.Background()
		request := &domain.CreateBroadcastRequest{
			WorkspaceID: "ws123",
			Name:        "Test Broadcast",
			Audience: domain.AudienceSettings{
				Lists:               []string{"list123"},
				ExcludeUnsubscribed: true,
				SkipDuplicateEmails: true,
			},
			Schedule: domain.ScheduleSettings{
				IsScheduled: false,
			},
			TestSettings: domain.BroadcastTestSettings{
				Enabled: false,
			},
		}

		// Mock auth service to authenticate the user
		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), request.WorkspaceID).
			Return(ctx, &domain.User{ID: "user123"}, nil)

		// Mock repository to return an error
		expectedErr := errors.New("database error")
		mockRepo.EXPECT().
			CreateBroadcast(gomock.Any(), gomock.Any()).
			Return(expectedErr)

		// Call the service
		result, err := service.CreateBroadcast(ctx, request)

		// Verify results
		require.Error(t, err)
		assert.Same(t, expectedErr, err)
		assert.Nil(t, result)
	})
}

func TestBroadcastService_GetBroadcast(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockBroadcastRepository(ctrl)
	mockEmailSvc := mocks.NewMockEmailServiceInterface(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockContactRepo := mocks.NewMockContactRepository(ctrl)
	mockTemplateSvc := mocks.NewMockTemplateService(ctrl)
	mockAuthSvc := mocks.NewMockAuthService(ctrl)
	mockTaskService := mocks.NewMockTaskService(ctrl)
	mockEventBus := mocks.NewMockEventBus(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	// Set up logger mock to return itself for chaining
	mockLoggerWithFields := pkgmocks.NewMockLogger(ctrl)
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLoggerWithFields).AnyTimes()
	mockLoggerWithFields.EXPECT().Error(gomock.Any()).AnyTimes()
	mockLoggerWithFields.EXPECT().Info(gomock.Any()).AnyTimes()

	// Add direct logger method expectations
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()

	service := NewBroadcastService(mockLogger, mockRepo, mockWorkspaceRepo, mockEmailSvc, mockContactRepo, mockTemplateSvc, mockTaskService, mockAuthSvc, mockEventBus, "https://api.example.com")

	// Then manually set the repositories needed for testing
	service.repo = mockRepo
	service.emailSvc = mockEmailSvc
	service.contactRepo = mockContactRepo
	service.templateSvc = mockTemplateSvc

	t.Run("Success", func(t *testing.T) {
		ctx := context.Background()
		workspaceID := "ws123"
		broadcastID := "bcast123"

		expectedBroadcast := &domain.Broadcast{
			ID:          broadcastID,
			WorkspaceID: workspaceID,
			Name:        "Test Broadcast",
			Status:      domain.BroadcastStatusDraft,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}

		// Mock authentication
		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
			Return(ctx, &domain.User{ID: "user123"}, nil)

		mockRepo.EXPECT().
			GetBroadcast(gomock.Any(), workspaceID, broadcastID).
			Return(expectedBroadcast, nil)

		// Call the service
		broadcast, err := service.GetBroadcast(ctx, workspaceID, broadcastID)

		// Verify results
		require.NoError(t, err)
		assert.Equal(t, expectedBroadcast, broadcast)
	})

	t.Run("NotFound", func(t *testing.T) {
		ctx := context.Background()
		workspaceID := "ws123"
		broadcastID := "nonexistent"

		// Mock authentication
		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
			Return(ctx, &domain.User{ID: "user123"}, nil)

		notFoundErr := &domain.ErrBroadcastNotFound{ID: broadcastID}
		mockRepo.EXPECT().
			GetBroadcast(gomock.Any(), workspaceID, broadcastID).
			Return(nil, notFoundErr)

		// Call the service
		broadcast, err := service.GetBroadcast(ctx, workspaceID, broadcastID)

		// Verify results
		require.Error(t, err)
		assert.Nil(t, broadcast)
		assert.Equal(t, notFoundErr, err)
	})
}

func TestBroadcastService_UpdateBroadcast(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockBroadcastRepository(ctrl)
	mockEmailSvc := mocks.NewMockEmailServiceInterface(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockContactRepo := mocks.NewMockContactRepository(ctrl)
	mockTemplateSvc := mocks.NewMockTemplateService(ctrl)
	mockAuthSvc := mocks.NewMockAuthService(ctrl)
	mockTaskService := mocks.NewMockTaskService(ctrl)
	mockEventBus := mocks.NewMockEventBus(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	// Set up logger mock to return itself for chaining
	mockLoggerWithFields := pkgmocks.NewMockLogger(ctrl)
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLoggerWithFields).AnyTimes()
	mockLoggerWithFields.EXPECT().Error(gomock.Any()).AnyTimes()
	mockLoggerWithFields.EXPECT().Info(gomock.Any()).AnyTimes()

	// Add direct logger method expectations
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()

	service := NewBroadcastService(mockLogger, mockRepo, mockWorkspaceRepo, mockEmailSvc, mockContactRepo, mockTemplateSvc, mockTaskService, mockAuthSvc, mockEventBus, "https://api.example.com")

	// Then manually set the repositories needed for testing
	service.repo = mockRepo
	service.emailSvc = mockEmailSvc
	service.contactRepo = mockContactRepo
	service.templateSvc = mockTemplateSvc

	t.Run("Success", func(t *testing.T) {
		ctx := context.Background()
		workspaceID := "ws123"
		broadcastID := "bcast123"

		// Create an existing broadcast with fixed timestamps
		createdTime := time.Now().Add(-24 * time.Hour).UTC()
		existingBroadcast := &domain.Broadcast{
			ID:          broadcastID,
			WorkspaceID: workspaceID,
			Name:        "Original Broadcast",
			Status:      domain.BroadcastStatusDraft,
			Audience: domain.AudienceSettings{
				Lists:               []string{"list123"},
				ExcludeUnsubscribed: true,
				SkipDuplicateEmails: true,
			},
			Schedule: domain.ScheduleSettings{
				IsScheduled: false,
			},
			TestSettings: domain.BroadcastTestSettings{
				Enabled: false,
			},
			CreatedAt: createdTime,
			UpdatedAt: createdTime,
		}

		// Create update request
		updateRequest := &domain.UpdateBroadcastRequest{
			WorkspaceID: workspaceID,
			ID:          broadcastID,
			Name:        "Updated Broadcast",
			Audience: domain.AudienceSettings{
				Lists:               []string{"list123"},
				ExcludeUnsubscribed: true,
				SkipDuplicateEmails: true,
			},
			Schedule: domain.ScheduleSettings{
				IsScheduled: false,
			},
			TestSettings: domain.BroadcastTestSettings{
				Enabled: false,
			},
		}

		// Mock authentication
		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
			Return(ctx, &domain.User{ID: "user123"}, nil)

		// Mock repository to return the existing broadcast
		mockRepo.EXPECT().
			GetBroadcast(gomock.Any(), workspaceID, broadcastID).
			Return(existingBroadcast, nil)

		// Mock repository update
		mockRepo.EXPECT().
			UpdateBroadcast(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, broadcast *domain.Broadcast) error {
				// Verify important properties
				assert.Equal(t, broadcastID, broadcast.ID)
				assert.Equal(t, workspaceID, broadcast.WorkspaceID)
				assert.Equal(t, updateRequest.Name, broadcast.Name)
				assert.Equal(t, domain.BroadcastStatusDraft, broadcast.Status)

				// Just verify that updated time isn't zero
				assert.False(t, broadcast.UpdatedAt.IsZero())
				return nil
			})

		// Call the service
		result, err := service.UpdateBroadcast(ctx, updateRequest)

		// Verify results
		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, updateRequest.Name, result.Name)

		// Verify that updated time is later than creation time
		assert.True(t, result.UpdatedAt.After(result.CreatedAt))
	})

	t.Run("BroadcastNotFound", func(t *testing.T) {
		ctx := context.Background()
		workspaceID := "ws123"
		broadcastID := "nonexistent"

		// Mock authentication
		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
			Return(ctx, &domain.User{ID: "user123"}, nil)

		updateRequest := &domain.UpdateBroadcastRequest{
			WorkspaceID: workspaceID,
			ID:          broadcastID,
			Name:        "Updated Broadcast",
			Audience: domain.AudienceSettings{
				Lists:               []string{"list123"},
				ExcludeUnsubscribed: true,
				SkipDuplicateEmails: true,
			},
			Schedule: domain.ScheduleSettings{
				IsScheduled: false,
			},
		}

		// Mock repository to return not found error
		notFoundErr := errors.New("broadcast not found")
		mockRepo.EXPECT().
			GetBroadcast(gomock.Any(), workspaceID, broadcastID).
			Return(nil, notFoundErr)

		// Call the service
		result, err := service.UpdateBroadcast(ctx, updateRequest)

		// Verify results
		require.Error(t, err)
		assert.Nil(t, result)
		assert.Equal(t, notFoundErr, err)
	})

	t.Run("InvalidStatus", func(t *testing.T) {
		ctx := context.Background()
		workspaceID := "ws123"
		broadcastID := "bcast123"

		// Create an existing broadcast with invalid status
		existingBroadcast := &domain.Broadcast{
			ID:          broadcastID,
			WorkspaceID: workspaceID,
			Name:        "Original Broadcast",
			Status:      domain.BroadcastStatusSent,
			CreatedAt:   time.Now().Add(-24 * time.Hour),
			UpdatedAt:   time.Now().Add(-24 * time.Hour),
		}

		// Create update request
		updateRequest := &domain.UpdateBroadcastRequest{
			WorkspaceID: workspaceID,
			ID:          broadcastID,
			Name:        "Updated Broadcast",
			Audience: domain.AudienceSettings{
				Lists:               []string{"list123"},
				ExcludeUnsubscribed: true,
				SkipDuplicateEmails: true,
			},
			Schedule: domain.ScheduleSettings{
				IsScheduled: false,
			},
			TestSettings: domain.BroadcastTestSettings{
				Enabled: false,
			},
		}

		// Mock authentication
		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
			Return(ctx, &domain.User{ID: "user123"}, nil)

		// Mock repository to return the existing broadcast
		mockRepo.EXPECT().
			GetBroadcast(gomock.Any(), workspaceID, broadcastID).
			Return(existingBroadcast, nil)

		// No update expected due to validation failure
		mockRepo.EXPECT().UpdateBroadcast(gomock.Any(), gomock.Any()).Times(0)

		// Call the service
		result, err := service.UpdateBroadcast(ctx, updateRequest)

		// Verify results
		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "cannot update broadcast with status")
	})
}

func TestBroadcastService_ListBroadcasts(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockBroadcastRepository(ctrl)
	mockEmailSvc := mocks.NewMockEmailServiceInterface(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockContactRepo := mocks.NewMockContactRepository(ctrl)
	mockTemplateSvc := mocks.NewMockTemplateService(ctrl)
	mockAuthSvc := mocks.NewMockAuthService(ctrl)
	mockTaskService := mocks.NewMockTaskService(ctrl)
	mockEventBus := mocks.NewMockEventBus(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)

	// Set up logger mock to return itself for chaining
	mockLoggerWithFields := pkgmocks.NewMockLogger(ctrl)
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLoggerWithFields).AnyTimes()
	mockLoggerWithFields.EXPECT().Error(gomock.Any()).AnyTimes()
	mockLoggerWithFields.EXPECT().Info(gomock.Any()).AnyTimes()

	// Add direct logger method expectations
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()

	service := NewBroadcastService(mockLogger, mockRepo, mockWorkspaceRepo, mockEmailSvc, mockContactRepo, mockTemplateSvc, mockTaskService, mockAuthSvc, mockEventBus, "https://api.example.com")

	t.Run("Success_WithoutTemplates", func(t *testing.T) {
		ctx := context.Background()
		workspaceID := "ws123"

		params := domain.ListBroadcastsParams{
			WorkspaceID:   workspaceID,
			Status:        domain.BroadcastStatusDraft,
			Limit:         10,
			Offset:        0,
			WithTemplates: false,
		}

		// Create sample broadcasts
		broadcasts := []*domain.Broadcast{
			{
				ID:          "bcast1",
				WorkspaceID: workspaceID,
				Name:        "Broadcast 1",
				Status:      domain.BroadcastStatusDraft,
				CreatedAt:   time.Now().Add(-2 * time.Hour),
				UpdatedAt:   time.Now().Add(-2 * time.Hour),
			},
			{
				ID:          "bcast2",
				WorkspaceID: workspaceID,
				Name:        "Broadcast 2",
				Status:      domain.BroadcastStatusDraft,
				CreatedAt:   time.Now().Add(-1 * time.Hour),
				UpdatedAt:   time.Now().Add(-1 * time.Hour),
			},
		}

		expectedResponse := &domain.BroadcastListResponse{
			Broadcasts: broadcasts,
			TotalCount: 2,
		}

		// Mock authentication
		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
			Return(ctx, &domain.User{ID: "user123"}, nil)

		// Mock repository to return broadcasts
		mockRepo.EXPECT().
			ListBroadcasts(gomock.Any(), params).
			Return(expectedResponse, nil)

		// Call the service
		result, err := service.ListBroadcasts(ctx, params)

		// Verify results
		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, 2, result.TotalCount)
		assert.Len(t, result.Broadcasts, 2)
		assert.Equal(t, "bcast1", result.Broadcasts[0].ID)
		assert.Equal(t, "bcast2", result.Broadcasts[1].ID)
	})

	t.Run("Success_WithTemplates", func(t *testing.T) {
		ctx := context.Background()
		workspaceID := "ws123"

		params := domain.ListBroadcastsParams{
			WorkspaceID:   workspaceID,
			Status:        "",
			Limit:         5,
			Offset:        0,
			WithTemplates: true,
		}

		// Create sample broadcasts with variations
		broadcasts := []*domain.Broadcast{
			{
				ID:          "bcast1",
				WorkspaceID: workspaceID,
				Name:        "Broadcast 1",
				Status:      domain.BroadcastStatusDraft,
				TestSettings: domain.BroadcastTestSettings{
					Enabled: true,
					Variations: []domain.BroadcastVariation{
						{
							ID:         "var1",
							TemplateID: "template1",
						},
						{
							ID:         "var2",
							TemplateID: "template2",
						},
					},
				},
				CreatedAt: time.Now().Add(-2 * time.Hour),
				UpdatedAt: time.Now().Add(-2 * time.Hour),
			},
		}

		expectedResponse := &domain.BroadcastListResponse{
			Broadcasts: broadcasts,
			TotalCount: 1,
		}

		// Create sample templates
		template1 := &domain.Template{
			ID:   "template1",
			Name: "Template 1",
		}
		template2 := &domain.Template{
			ID:   "template2",
			Name: "Template 2",
		}

		// Mock authentication
		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
			Return(ctx, &domain.User{ID: "user123"}, nil)

		// Mock repository to return broadcasts
		mockRepo.EXPECT().
			ListBroadcasts(gomock.Any(), params).
			Return(expectedResponse, nil)

		// Mock template service to return templates for each variation
		mockTemplateSvc.EXPECT().
			GetTemplateByID(gomock.Any(), workspaceID, "template1", int64(0)).
			Return(template1, nil)

		mockTemplateSvc.EXPECT().
			GetTemplateByID(gomock.Any(), workspaceID, "template2", int64(0)).
			Return(template2, nil)

		// Call the service
		result, err := service.ListBroadcasts(ctx, params)

		// Verify results
		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, 1, result.TotalCount)
		assert.Len(t, result.Broadcasts, 1)

		// Verify that templates were fetched and attached
		broadcast := result.Broadcasts[0]
		assert.Len(t, broadcast.TestSettings.Variations, 2)
		assert.Equal(t, template1, broadcast.TestSettings.Variations[0].Template)
		assert.Equal(t, template2, broadcast.TestSettings.Variations[1].Template)
	})

	t.Run("Success_WithTemplates_TemplateError", func(t *testing.T) {
		ctx := context.Background()
		workspaceID := "ws123"

		params := domain.ListBroadcastsParams{
			WorkspaceID:   workspaceID,
			Status:        "",
			Limit:         5,
			Offset:        0,
			WithTemplates: true,
		}

		// Create sample broadcasts with variations
		broadcasts := []*domain.Broadcast{
			{
				ID:          "bcast1",
				WorkspaceID: workspaceID,
				Name:        "Broadcast 1",
				Status:      domain.BroadcastStatusDraft,
				TestSettings: domain.BroadcastTestSettings{
					Enabled: true,
					Variations: []domain.BroadcastVariation{
						{
							ID:         "var1",
							TemplateID: "template1",
						},
					},
				},
				CreatedAt: time.Now().Add(-2 * time.Hour),
				UpdatedAt: time.Now().Add(-2 * time.Hour),
			},
		}

		expectedResponse := &domain.BroadcastListResponse{
			Broadcasts: broadcasts,
			TotalCount: 1,
		}

		// Mock authentication
		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
			Return(ctx, &domain.User{ID: "user123"}, nil)

		// Mock repository to return broadcasts
		mockRepo.EXPECT().
			ListBroadcasts(gomock.Any(), params).
			Return(expectedResponse, nil)

		// Mock template service to return error for template
		templateErr := errors.New("template not found")
		mockTemplateSvc.EXPECT().
			GetTemplateByID(gomock.Any(), workspaceID, "template1", int64(0)).
			Return(nil, templateErr)

		// Call the service - should not fail even if template fetch fails
		result, err := service.ListBroadcasts(ctx, params)

		// Verify results - should succeed despite template error
		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, 1, result.TotalCount)
		assert.Len(t, result.Broadcasts, 1)

		// Verify that template was not attached due to error
		broadcast := result.Broadcasts[0]
		assert.Len(t, broadcast.TestSettings.Variations, 1)
		assert.Nil(t, broadcast.TestSettings.Variations[0].Template)
	})

	t.Run("Success_DefaultPagination", func(t *testing.T) {
		ctx := context.Background()
		workspaceID := "ws123"

		// Test with invalid/zero pagination values
		params := domain.ListBroadcastsParams{
			WorkspaceID:   workspaceID,
			Status:        "",
			Limit:         0,  // Should default to 50
			Offset:        -5, // Should default to 0
			WithTemplates: false,
		}

		expectedResponse := &domain.BroadcastListResponse{
			Broadcasts: []*domain.Broadcast{},
			TotalCount: 0,
		}

		// Mock authentication
		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
			Return(ctx, &domain.User{ID: "user123"}, nil)

		// Mock repository - verify that defaults are applied
		mockRepo.EXPECT().
			ListBroadcasts(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, actualParams domain.ListBroadcastsParams) (*domain.BroadcastListResponse, error) {
				// Verify that defaults were applied
				assert.Equal(t, 50, actualParams.Limit) // Default limit
				assert.Equal(t, 0, actualParams.Offset) // Default offset
				assert.Equal(t, workspaceID, actualParams.WorkspaceID)
				return expectedResponse, nil
			})

		// Call the service
		result, err := service.ListBroadcasts(ctx, params)

		// Verify results
		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, 0, result.TotalCount)
	})

	t.Run("Success_MaxLimitEnforced", func(t *testing.T) {
		ctx := context.Background()
		workspaceID := "ws123"

		// Test with limit exceeding maximum
		params := domain.ListBroadcastsParams{
			WorkspaceID:   workspaceID,
			Status:        "",
			Limit:         150, // Should be capped to 100
			Offset:        0,
			WithTemplates: false,
		}

		expectedResponse := &domain.BroadcastListResponse{
			Broadcasts: []*domain.Broadcast{},
			TotalCount: 0,
		}

		// Mock authentication
		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
			Return(ctx, &domain.User{ID: "user123"}, nil)

		// Mock repository - verify that limit is capped
		mockRepo.EXPECT().
			ListBroadcasts(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, actualParams domain.ListBroadcastsParams) (*domain.BroadcastListResponse, error) {
				// Verify that limit was capped to maximum
				assert.Equal(t, 100, actualParams.Limit) // Maximum limit
				assert.Equal(t, 0, actualParams.Offset)
				assert.Equal(t, workspaceID, actualParams.WorkspaceID)
				return expectedResponse, nil
			})

		// Call the service
		result, err := service.ListBroadcasts(ctx, params)

		// Verify results
		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, 0, result.TotalCount)
	})

	t.Run("AuthenticationError", func(t *testing.T) {
		ctx := context.Background()
		workspaceID := "ws123"

		params := domain.ListBroadcastsParams{
			WorkspaceID:   workspaceID,
			Status:        "",
			Limit:         10,
			Offset:        0,
			WithTemplates: false,
		}

		// Mock authentication to return error
		authErr := errors.New("authentication failed")
		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
			Return(nil, nil, authErr)

		// Repository should not be called due to authentication failure
		mockRepo.EXPECT().ListBroadcasts(gomock.Any(), gomock.Any()).Times(0)

		// Call the service
		result, err := service.ListBroadcasts(ctx, params)

		// Verify results
		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "failed to authenticate user")
	})

	t.Run("RepositoryError", func(t *testing.T) {
		ctx := context.Background()
		workspaceID := "ws123"

		params := domain.ListBroadcastsParams{
			WorkspaceID:   workspaceID,
			Status:        "",
			Limit:         10,
			Offset:        0,
			WithTemplates: false,
		}

		// Mock authentication
		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
			Return(ctx, &domain.User{ID: "user123"}, nil)

		// Mock repository to return error
		repoErr := errors.New("database error")
		mockRepo.EXPECT().
			ListBroadcasts(gomock.Any(), params).
			Return(nil, repoErr)

		// Call the service
		result, err := service.ListBroadcasts(ctx, params)

		// Verify results
		require.Error(t, err)
		assert.Nil(t, result)
		assert.Equal(t, repoErr, err)
	})

	t.Run("Success_EmptyVariations", func(t *testing.T) {
		ctx := context.Background()
		workspaceID := "ws123"

		params := domain.ListBroadcastsParams{
			WorkspaceID:   workspaceID,
			Status:        "",
			Limit:         5,
			Offset:        0,
			WithTemplates: true,
		}

		// Create broadcast with no variations
		broadcasts := []*domain.Broadcast{
			{
				ID:          "bcast1",
				WorkspaceID: workspaceID,
				Name:        "Broadcast 1",
				Status:      domain.BroadcastStatusDraft,
				TestSettings: domain.BroadcastTestSettings{
					Enabled:    false,
					Variations: []domain.BroadcastVariation{}, // Empty variations
				},
				CreatedAt: time.Now().Add(-2 * time.Hour),
				UpdatedAt: time.Now().Add(-2 * time.Hour),
			},
		}

		expectedResponse := &domain.BroadcastListResponse{
			Broadcasts: broadcasts,
			TotalCount: 1,
		}

		// Mock authentication
		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
			Return(ctx, &domain.User{ID: "user123"}, nil)

		// Mock repository to return broadcasts
		mockRepo.EXPECT().
			ListBroadcasts(gomock.Any(), params).
			Return(expectedResponse, nil)

		// No template service calls expected since there are no variations

		// Call the service
		result, err := service.ListBroadcasts(ctx, params)

		// Verify results
		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, 1, result.TotalCount)
		assert.Len(t, result.Broadcasts, 1)

		// Verify that no templates were attached
		broadcast := result.Broadcasts[0]
		assert.Len(t, broadcast.TestSettings.Variations, 0)
	})

	t.Run("Success_VariationWithoutTemplateID", func(t *testing.T) {
		ctx := context.Background()
		workspaceID := "ws123"

		params := domain.ListBroadcastsParams{
			WorkspaceID:   workspaceID,
			Status:        "",
			Limit:         5,
			Offset:        0,
			WithTemplates: true,
		}

		// Create broadcast with variation that has no template ID
		broadcasts := []*domain.Broadcast{
			{
				ID:          "bcast1",
				WorkspaceID: workspaceID,
				Name:        "Broadcast 1",
				Status:      domain.BroadcastStatusDraft,
				TestSettings: domain.BroadcastTestSettings{
					Enabled: true,
					Variations: []domain.BroadcastVariation{
						{
							ID:         "var1",
							TemplateID: "", // Empty template ID
						},
					},
				},
				CreatedAt: time.Now().Add(-2 * time.Hour),
				UpdatedAt: time.Now().Add(-2 * time.Hour),
			},
		}

		expectedResponse := &domain.BroadcastListResponse{
			Broadcasts: broadcasts,
			TotalCount: 1,
		}

		// Mock authentication
		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
			Return(ctx, &domain.User{ID: "user123"}, nil)

		// Mock repository to return broadcasts
		mockRepo.EXPECT().
			ListBroadcasts(gomock.Any(), params).
			Return(expectedResponse, nil)

		// No template service calls expected since template ID is empty

		// Call the service
		result, err := service.ListBroadcasts(ctx, params)

		// Verify results
		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, 1, result.TotalCount)
		assert.Len(t, result.Broadcasts, 1)

		// Verify that no template was attached
		broadcast := result.Broadcasts[0]
		assert.Len(t, broadcast.TestSettings.Variations, 1)
		assert.Nil(t, broadcast.TestSettings.Variations[0].Template)
	})
}

func TestBroadcastService_ScheduleBroadcast(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockBroadcastRepository(ctrl)
	mockEmailSvc := mocks.NewMockEmailServiceInterface(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockContactRepo := mocks.NewMockContactRepository(ctrl)
	mockTemplateSvc := mocks.NewMockTemplateService(ctrl)
	mockAuthSvc := mocks.NewMockAuthService(ctrl)
	mockTaskService := mocks.NewMockTaskService(ctrl)
	mockEventBus := mocks.NewMockEventBus(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	// Set up logger mock to return itself for chaining
	mockLoggerWithField := pkgmocks.NewMockLogger(ctrl)
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLoggerWithField).AnyTimes()
	mockLoggerWithField.EXPECT().Error(gomock.Any()).AnyTimes()
	mockLoggerWithField.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLoggerWithField.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLoggerWithField.EXPECT().Warn(gomock.Any()).AnyTimes()

	// Add direct logger method expectations
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()

	service := NewBroadcastService(mockLogger, mockRepo, mockWorkspaceRepo, mockEmailSvc, mockContactRepo, mockTemplateSvc, mockTaskService, mockAuthSvc, mockEventBus, "https://api.example.com")

	// Then manually set the repositories needed for testing
	service.repo = mockRepo
	service.emailSvc = mockEmailSvc
	service.contactRepo = mockContactRepo
	service.templateSvc = mockTemplateSvc

	t.Run("ScheduleForLater", func(t *testing.T) {
		ctx := context.Background()
		workspaceID := "ws123"
		broadcastID := "bcast123"

		request := &domain.ScheduleBroadcastRequest{
			WorkspaceID:          workspaceID,
			ID:                   broadcastID,
			SendNow:              false,
			ScheduledDate:        time.Now().Add(time.Hour).Format("2006-01-02"),
			ScheduledTime:        time.Now().Add(time.Hour).Format("15:04"),
			Timezone:             "UTC",
			UseRecipientTimezone: false,
		}

		// Create a draft broadcast
		existingBroadcast := &domain.Broadcast{
			ID:          broadcastID,
			WorkspaceID: workspaceID,
			Name:        "Test Broadcast",
			Status:      domain.BroadcastStatusDraft,
			CreatedAt:   time.Now().Add(-1 * time.Hour),
			UpdatedAt:   time.Now().Add(-1 * time.Hour),
		}

		// Mock authentication
		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
			Return(ctx, &domain.User{ID: "user123"}, nil)

		// Create a workspace with a marketing email provider
		workspace := &domain.Workspace{
			ID:   workspaceID,
			Name: "Test Workspace",
			Settings: domain.WorkspaceSettings{
				MarketingEmailProviderID: "email-provider-123",
			},
			Integrations: []domain.Integration{
				{
					ID:   "email-provider-123",
					Type: domain.IntegrationTypeEmail,
					EmailProvider: domain.EmailProvider{
						Kind: domain.EmailProviderKindSMTP,
						Senders: []domain.EmailSender{
							domain.NewEmailSender("test@example.com", "Test Sender"),
						},
					},
				},
			},
		}

		// Mock workspace repository to return workspace with email provider
		mockWorkspaceRepo.EXPECT().
			GetByID(gomock.Any(), workspaceID).
			Return(workspace, nil)

		// Mock the WithTransaction call and execute the provided function with nil
		mockRepo.EXPECT().
			WithTransaction(gomock.Any(), workspaceID, gomock.Any()).
			DoAndReturn(func(ctx context.Context, workspaceID string, fn func(*sql.Tx) error) error {
				return fn(nil) // Pass nil as the transaction, it won't be used in the test
			})

		// Mock repository to return the existing broadcast
		mockRepo.EXPECT().
			GetBroadcastTx(gomock.Any(), gomock.Any(), workspaceID, broadcastID).
			Return(existingBroadcast, nil)

		// Mock repository update with verification
		mockRepo.EXPECT().
			UpdateBroadcastTx(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, _ *sql.Tx, broadcast *domain.Broadcast) error {
				// Verify important properties
				assert.Equal(t, broadcastID, broadcast.ID)
				assert.Equal(t, workspaceID, broadcast.WorkspaceID)
				assert.Equal(t, domain.BroadcastStatusScheduled, broadcast.Status)

				// Verify scheduled time using Schedule struct
				assert.True(t, broadcast.Schedule.IsScheduled)
				assert.NotEmpty(t, broadcast.Schedule.ScheduledDate)
				assert.NotEmpty(t, broadcast.Schedule.ScheduledTime)

				assert.Nil(t, broadcast.StartedAt) // Should not be set when scheduling for later
				return nil
			})

		// In the TestBroadcastService_ScheduleBroadcast test, find all test cases and add mockEventBus expectation before the service call
		mockEventBus.EXPECT().
			PublishWithAck(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, payload domain.EventPayload, callback domain.EventAckCallback) {
				// Call the callback with success
				callback(nil)
			}).
			AnyTimes()

		// Call the service
		err := service.ScheduleBroadcast(ctx, request)

		// Verify results
		require.NoError(t, err)
	})

	t.Run("SendNow", func(t *testing.T) {
		ctx := context.Background()
		workspaceID := "ws123"
		broadcastID := "bcast123"

		request := &domain.ScheduleBroadcastRequest{
			WorkspaceID: workspaceID,
			ID:          broadcastID,
			SendNow:     true,
		}

		// Create a draft broadcast
		existingBroadcast := &domain.Broadcast{
			ID:          broadcastID,
			WorkspaceID: workspaceID,
			Name:        "Test Broadcast",
			Status:      domain.BroadcastStatusDraft,
			CreatedAt:   time.Now().Add(-1 * time.Hour),
			UpdatedAt:   time.Now().Add(-1 * time.Hour),
		}

		// Mock authentication
		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
			Return(ctx, &domain.User{ID: "user123"}, nil)

		// Create a workspace with a marketing email provider
		workspace := &domain.Workspace{
			ID:   workspaceID,
			Name: "Test Workspace",
			Settings: domain.WorkspaceSettings{
				MarketingEmailProviderID: "email-provider-123",
			},
			Integrations: []domain.Integration{
				{
					ID:   "email-provider-123",
					Type: domain.IntegrationTypeEmail,
					EmailProvider: domain.EmailProvider{
						Kind: domain.EmailProviderKindSMTP,
						Senders: []domain.EmailSender{
							domain.NewEmailSender("test@example.com", "Test Sender"),
						},
					},
				},
			},
		}

		// Mock workspace repository to return workspace with email provider
		mockWorkspaceRepo.EXPECT().
			GetByID(gomock.Any(), workspaceID).
			Return(workspace, nil)

		// Mock the WithTransaction call and execute the provided function with nil
		mockRepo.EXPECT().
			WithTransaction(gomock.Any(), workspaceID, gomock.Any()).
			DoAndReturn(func(ctx context.Context, workspaceID string, fn func(*sql.Tx) error) error {
				return fn(nil) // Pass nil as the transaction, it won't be used in the test
			})

		// Mock repository to return the existing broadcast
		mockRepo.EXPECT().
			GetBroadcastTx(gomock.Any(), gomock.Any(), workspaceID, broadcastID).
			Return(existingBroadcast, nil)

		// Mock repository update with verification
		mockRepo.EXPECT().
			UpdateBroadcastTx(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, _ *sql.Tx, broadcast *domain.Broadcast) error {
				// Verify important properties
				assert.Equal(t, broadcastID, broadcast.ID)
				assert.Equal(t, workspaceID, broadcast.WorkspaceID)
				assert.Equal(t, domain.BroadcastStatusSending, broadcast.Status)

				// No scheduled time should be set in Schedule
				assert.False(t, broadcast.Schedule.IsScheduled)

				assert.NotNil(t, broadcast.StartedAt) // Should be set when sending now
				return nil
			})

		// In the TestBroadcastService_ScheduleBroadcast test, find all test cases and add mockEventBus expectation before the service call
		mockEventBus.EXPECT().
			PublishWithAck(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, payload domain.EventPayload, callback domain.EventAckCallback) {
				// Call the callback with success
				callback(nil)
			}).
			AnyTimes()

		// Call the service
		err := service.ScheduleBroadcast(ctx, request)

		// Verify results
		require.NoError(t, err)
	})

	t.Run("NonDraftStatus", func(t *testing.T) {
		ctx := context.Background()
		workspaceID := "ws123"
		broadcastID := "bcast123"

		request := &domain.ScheduleBroadcastRequest{
			WorkspaceID:          workspaceID,
			ID:                   broadcastID,
			SendNow:              false,
			ScheduledDate:        time.Now().Add(time.Hour).Format("2006-01-02"),
			ScheduledTime:        time.Now().Add(time.Hour).Format("15:04"),
			Timezone:             "UTC",
			UseRecipientTimezone: false,
		}

		// Create a broadcast with non-draft status
		existingBroadcast := &domain.Broadcast{
			ID:          broadcastID,
			WorkspaceID: workspaceID,
			Name:        "Test Broadcast",
			Status:      domain.BroadcastStatusSent, // Already sent
			CreatedAt:   time.Now().Add(-1 * time.Hour),
			UpdatedAt:   time.Now().Add(-1 * time.Hour),
		}

		// Mock authentication
		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
			Return(ctx, &domain.User{ID: "user123"}, nil)

		// Create a workspace with a marketing email provider
		workspace := &domain.Workspace{
			ID:   workspaceID,
			Name: "Test Workspace",
			Settings: domain.WorkspaceSettings{
				MarketingEmailProviderID: "email-provider-123",
			},
			Integrations: []domain.Integration{
				{
					ID:   "email-provider-123",
					Type: domain.IntegrationTypeEmail,
					EmailProvider: domain.EmailProvider{
						Kind: domain.EmailProviderKindSMTP,
						Senders: []domain.EmailSender{
							domain.NewEmailSender("test@example.com", "Test Sender"),
						},
					},
				},
			},
		}

		// Mock workspace repository to return workspace with email provider
		mockWorkspaceRepo.EXPECT().
			GetByID(gomock.Any(), workspaceID).
			Return(workspace, nil)

		// Mock the WithTransaction call and execute the provided function with nil
		mockRepo.EXPECT().
			WithTransaction(gomock.Any(), workspaceID, gomock.Any()).
			DoAndReturn(func(ctx context.Context, workspaceID string, fn func(*sql.Tx) error) error {
				return fn(nil) // Pass nil as the transaction, it won't be used in the test
			})

		// Mock repository to return the existing broadcast
		mockRepo.EXPECT().
			GetBroadcastTx(gomock.Any(), gomock.Any(), workspaceID, broadcastID).
			Return(existingBroadcast, nil)

		// No update calls should be made
		mockRepo.EXPECT().UpdateBroadcastTx(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

		// Call the service
		err := service.ScheduleBroadcast(ctx, request)

		// Verify results
		require.Error(t, err)
		assert.Contains(t, err.Error(), "only broadcasts with draft status can be scheduled")
	})

	t.Run("NoMarketingEmailProvider", func(t *testing.T) {
		ctx := context.Background()
		workspaceID := "ws123"
		broadcastID := "bcast123"

		request := &domain.ScheduleBroadcastRequest{
			WorkspaceID: workspaceID,
			ID:          broadcastID,
			SendNow:     true,
		}

		// Create a workspace with no marketing email provider set
		workspace := &domain.Workspace{
			ID:        workspaceID,
			Name:      "Test Workspace",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
			Settings: domain.WorkspaceSettings{
				// No marketing email provider ID set
				MarketingEmailProviderID: "",
			},
			Integrations: []domain.Integration{}, // Empty integrations
		}

		// Mock authentication
		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
			Return(ctx, &domain.User{ID: "user123"}, nil)

		// Mock workspace repository to return workspace with no email provider
		mockWorkspaceRepo.EXPECT().
			GetByID(gomock.Any(), workspaceID).
			Return(workspace, nil)

		// No transaction should be started
		mockRepo.EXPECT().WithTransaction(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

		// Call the service
		err := service.ScheduleBroadcast(ctx, request)

		// Verify results
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no marketing email provider configured")
	})
}

func TestBroadcastService_CancelBroadcast(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockBroadcastRepository(ctrl)
	mockEmailSvc := mocks.NewMockEmailServiceInterface(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockContactRepo := mocks.NewMockContactRepository(ctrl)
	mockTemplateSvc := mocks.NewMockTemplateService(ctrl)
	mockAuthSvc := mocks.NewMockAuthService(ctrl)
	mockTaskService := mocks.NewMockTaskService(ctrl)
	mockEventBus := mocks.NewMockEventBus(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	// Set up logger mock to return itself for chaining
	mockLoggerWithField := pkgmocks.NewMockLogger(ctrl)
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLoggerWithField).AnyTimes()
	mockLoggerWithField.EXPECT().Error(gomock.Any()).AnyTimes()
	mockLoggerWithField.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLoggerWithField.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLoggerWithField.EXPECT().Warn(gomock.Any()).AnyTimes()

	// Add direct logger method expectations
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()

	service := NewBroadcastService(mockLogger, mockRepo, mockWorkspaceRepo, mockEmailSvc, mockContactRepo, mockTemplateSvc, mockTaskService, mockAuthSvc, mockEventBus, "https://api.example.com")

	// Then manually set the repositories needed for testing
	service.repo = mockRepo
	service.emailSvc = mockEmailSvc
	service.contactRepo = mockContactRepo
	service.templateSvc = mockTemplateSvc

	t.Run("CancelScheduledBroadcast", func(t *testing.T) {
		ctx := context.Background()
		workspaceID := "ws123"
		broadcastID := "bcast123"

		request := &domain.CancelBroadcastRequest{
			WorkspaceID: workspaceID,
			ID:          broadcastID,
		}

		// Create a scheduled broadcast with future time
		futureTime := time.Now().Add(24 * time.Hour).UTC()
		scheduledDate := futureTime.Format("2006-01-02")
		scheduledTimeStr := futureTime.Format("15:04")

		existingBroadcast := &domain.Broadcast{
			ID:          broadcastID,
			WorkspaceID: workspaceID,
			Name:        "Test Broadcast",
			Status:      domain.BroadcastStatusScheduled,
			CreatedAt:   time.Now().Add(-1 * time.Hour),
			UpdatedAt:   time.Now().Add(-1 * time.Hour),
			Schedule: domain.ScheduleSettings{
				IsScheduled:   true,
				ScheduledDate: scheduledDate,
				ScheduledTime: scheduledTimeStr,
				Timezone:      "UTC",
			},
		}

		// Mock authentication
		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
			Return(ctx, &domain.User{ID: "user123"}, nil)

		// Mock the WithTransaction call and execute the provided function with nil
		mockRepo.EXPECT().
			WithTransaction(gomock.Any(), workspaceID, gomock.Any()).
			DoAndReturn(func(ctx context.Context, workspaceID string, fn func(*sql.Tx) error) error {
				return fn(nil) // Pass nil as the transaction, it won't be used in the test
			})

		// Mock repository to return the existing broadcast for the transaction
		mockRepo.EXPECT().
			GetBroadcastTx(gomock.Any(), gomock.Any(), workspaceID, broadcastID).
			Return(existingBroadcast, nil)

		// Mock repository update with verification
		mockRepo.EXPECT().
			UpdateBroadcastTx(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, _ *sql.Tx, broadcast *domain.Broadcast) error {
				// Verify important properties
				assert.Equal(t, broadcastID, broadcast.ID)
				assert.Equal(t, workspaceID, broadcast.WorkspaceID)
				assert.Equal(t, domain.BroadcastStatusCancelled, broadcast.Status)
				assert.NotNil(t, broadcast.CancelledAt)

				// Schedule settings should remain the same
				assert.Equal(t, existingBroadcast.Schedule.ScheduledDate, broadcast.Schedule.ScheduledDate)
				assert.Equal(t, existingBroadcast.Schedule.ScheduledTime, broadcast.Schedule.ScheduledTime)
				assert.Equal(t, existingBroadcast.Schedule.Timezone, broadcast.Schedule.Timezone)

				return nil
			})

		// In the TestBroadcastService_CancelBroadcast test, find all test cases and add mockEventBus expectation before the service call
		mockEventBus.EXPECT().
			PublishWithAck(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, payload domain.EventPayload, callback domain.EventAckCallback) {
				// Call the callback with success
				callback(nil)
			}).
			AnyTimes()

		// Call the service
		err := service.CancelBroadcast(ctx, request)

		// Verify results
		require.NoError(t, err)
	})

	t.Run("InvalidStatus", func(t *testing.T) {
		ctx := context.Background()
		workspaceID := "ws123"
		broadcastID := "bcast123"

		request := &domain.CancelBroadcastRequest{
			WorkspaceID: workspaceID,
			ID:          broadcastID,
		}

		// Create a draft broadcast - can't cancel a draft
		existingBroadcast := &domain.Broadcast{
			ID:          broadcastID,
			WorkspaceID: workspaceID,
			Name:        "Test Broadcast",
			Status:      domain.BroadcastStatusDraft,
			CreatedAt:   time.Now().Add(-1 * time.Hour),
			UpdatedAt:   time.Now().Add(-1 * time.Hour),
		}

		// Mock authentication
		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
			Return(ctx, &domain.User{ID: "user123"}, nil)

		// Mock the WithTransaction call and execute the provided function with nil
		mockRepo.EXPECT().
			WithTransaction(gomock.Any(), workspaceID, gomock.Any()).
			DoAndReturn(func(ctx context.Context, workspaceID string, fn func(*sql.Tx) error) error {
				return fn(nil) // Pass nil as the transaction, it won't be used in the test
			})

		// Mock repository to return the existing broadcast
		mockRepo.EXPECT().
			GetBroadcastTx(gomock.Any(), gomock.Any(), workspaceID, broadcastID).
			Return(existingBroadcast, nil)

		// No update calls expected
		mockRepo.EXPECT().UpdateBroadcastTx(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

		// Call the service
		err := service.CancelBroadcast(ctx, request)

		// Verify results
		require.Error(t, err)
		assert.Contains(t, err.Error(), "only broadcasts with scheduled or paused status can be cancelled")
	})
}

func TestBroadcastService_PauseBroadcast(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockBroadcastRepository(ctrl)
	mockEmailSvc := mocks.NewMockEmailServiceInterface(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockContactRepo := mocks.NewMockContactRepository(ctrl)
	mockTemplateSvc := mocks.NewMockTemplateService(ctrl)
	mockAuthSvc := mocks.NewMockAuthService(ctrl)
	mockTaskService := mocks.NewMockTaskService(ctrl)
	mockEventBus := mocks.NewMockEventBus(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)

	// Set up logger mock to return itself for chaining
	mockLoggerWithField := pkgmocks.NewMockLogger(ctrl)
	mockLoggerWithFields := pkgmocks.NewMockLogger(ctrl)
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLoggerWithField).AnyTimes()
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLoggerWithFields).AnyTimes()
	mockLoggerWithField.EXPECT().Error(gomock.Any()).AnyTimes()
	mockLoggerWithField.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLoggerWithField.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLoggerWithField.EXPECT().Warn(gomock.Any()).AnyTimes()
	mockLoggerWithFields.EXPECT().Error(gomock.Any()).AnyTimes()
	mockLoggerWithFields.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLoggerWithFields.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLoggerWithFields.EXPECT().Warn(gomock.Any()).AnyTimes()

	// Add direct logger method expectations
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()

	service := NewBroadcastService(mockLogger, mockRepo, mockWorkspaceRepo, mockEmailSvc, mockContactRepo, mockTemplateSvc, mockTaskService, mockAuthSvc, mockEventBus, "https://api.example.com")

	t.Run("Success_PauseSendingBroadcast", func(t *testing.T) {
		ctx := context.Background()
		workspaceID := "ws123"
		broadcastID := "bcast123"

		request := &domain.PauseBroadcastRequest{
			WorkspaceID: workspaceID,
			ID:          broadcastID,
		}

		// Create a sending broadcast that can be paused
		startedTime := time.Now().Add(-10 * time.Minute)
		existingBroadcast := &domain.Broadcast{
			ID:          broadcastID,
			WorkspaceID: workspaceID,
			Name:        "Test Broadcast",
			Status:      domain.BroadcastStatusSending,
			CreatedAt:   time.Now().Add(-1 * time.Hour),
			UpdatedAt:   time.Now().Add(-1 * time.Hour),
			StartedAt:   &startedTime,
		}

		// Mock authentication
		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
			Return(ctx, &domain.User{ID: "user123"}, nil)

		// Mock the WithTransaction call and execute the provided function with nil
		mockRepo.EXPECT().
			WithTransaction(gomock.Any(), workspaceID, gomock.Any()).
			DoAndReturn(func(ctx context.Context, workspaceID string, fn func(*sql.Tx) error) error {
				return fn(nil) // Pass nil as the transaction, it won't be used in the test
			})

		// Mock repository to return the existing broadcast
		mockRepo.EXPECT().
			GetBroadcastTx(gomock.Any(), gomock.Any(), workspaceID, broadcastID).
			Return(existingBroadcast, nil)

		// Mock repository update with verification
		mockRepo.EXPECT().
			UpdateBroadcastTx(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, _ *sql.Tx, broadcast *domain.Broadcast) error {
				// Verify important properties
				assert.Equal(t, broadcastID, broadcast.ID)
				assert.Equal(t, workspaceID, broadcast.WorkspaceID)
				assert.Equal(t, domain.BroadcastStatusPaused, broadcast.Status)
				assert.NotNil(t, broadcast.PausedAt)
				assert.Equal(t, startedTime, *broadcast.StartedAt) // StartedAt should remain unchanged
				return nil
			})

		// Mock event bus to simulate successful event processing
		mockEventBus.EXPECT().
			PublishWithAck(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, payload domain.EventPayload, callback domain.EventAckCallback) {
				// Verify event payload
				assert.Equal(t, domain.EventBroadcastPaused, payload.Type)
				assert.Equal(t, workspaceID, payload.WorkspaceID)
				assert.Equal(t, broadcastID, payload.EntityID)
				assert.Equal(t, broadcastID, payload.Data["broadcast_id"])

				// Call the callback with success
				callback(nil)
			})

		// Call the service
		err := service.PauseBroadcast(ctx, request)

		// Verify results
		require.NoError(t, err)
	})

	t.Run("Error_InvalidStatus_Draft", func(t *testing.T) {
		ctx := context.Background()
		workspaceID := "ws123"
		broadcastID := "bcast123"

		request := &domain.PauseBroadcastRequest{
			WorkspaceID: workspaceID,
			ID:          broadcastID,
		}

		// Create a draft broadcast that cannot be paused
		existingBroadcast := &domain.Broadcast{
			ID:          broadcastID,
			WorkspaceID: workspaceID,
			Name:        "Test Broadcast",
			Status:      domain.BroadcastStatusDraft,
			CreatedAt:   time.Now().Add(-1 * time.Hour),
			UpdatedAt:   time.Now().Add(-1 * time.Hour),
		}

		// Mock authentication
		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
			Return(ctx, &domain.User{ID: "user123"}, nil)

		// Mock the WithTransaction call and execute the provided function with nil
		mockRepo.EXPECT().
			WithTransaction(gomock.Any(), workspaceID, gomock.Any()).
			DoAndReturn(func(ctx context.Context, workspaceID string, fn func(*sql.Tx) error) error {
				return fn(nil) // Pass nil as the transaction, it won't be used in the test
			})

		// Mock repository to return the existing broadcast
		mockRepo.EXPECT().
			GetBroadcastTx(gomock.Any(), gomock.Any(), workspaceID, broadcastID).
			Return(existingBroadcast, nil)

		// No update or event calls should be made
		mockRepo.EXPECT().UpdateBroadcastTx(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
		mockEventBus.EXPECT().PublishWithAck(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

		// Call the service
		err := service.PauseBroadcast(ctx, request)

		// Verify results
		require.Error(t, err)
		assert.Contains(t, err.Error(), "only broadcasts with sending status can be paused")
		assert.Contains(t, err.Error(), "current status: draft")
	})

	t.Run("Error_InvalidStatus_Scheduled", func(t *testing.T) {
		ctx := context.Background()
		workspaceID := "ws123"
		broadcastID := "bcast123"

		request := &domain.PauseBroadcastRequest{
			WorkspaceID: workspaceID,
			ID:          broadcastID,
		}

		// Create a scheduled broadcast that cannot be paused
		existingBroadcast := &domain.Broadcast{
			ID:          broadcastID,
			WorkspaceID: workspaceID,
			Name:        "Test Broadcast",
			Status:      domain.BroadcastStatusScheduled,
			CreatedAt:   time.Now().Add(-1 * time.Hour),
			UpdatedAt:   time.Now().Add(-1 * time.Hour),
		}

		// Mock authentication
		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
			Return(ctx, &domain.User{ID: "user123"}, nil)

		// Mock the WithTransaction call and execute the provided function with nil
		mockRepo.EXPECT().
			WithTransaction(gomock.Any(), workspaceID, gomock.Any()).
			DoAndReturn(func(ctx context.Context, workspaceID string, fn func(*sql.Tx) error) error {
				return fn(nil) // Pass nil as the transaction, it won't be used in the test
			})

		// Mock repository to return the existing broadcast
		mockRepo.EXPECT().
			GetBroadcastTx(gomock.Any(), gomock.Any(), workspaceID, broadcastID).
			Return(existingBroadcast, nil)

		// No update or event calls should be made
		mockRepo.EXPECT().UpdateBroadcastTx(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
		mockEventBus.EXPECT().PublishWithAck(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

		// Call the service
		err := service.PauseBroadcast(ctx, request)

		// Verify results
		require.Error(t, err)
		assert.Contains(t, err.Error(), "only broadcasts with sending status can be paused")
		assert.Contains(t, err.Error(), "current status: scheduled")
	})

	t.Run("Error_InvalidStatus_AlreadyPaused", func(t *testing.T) {
		ctx := context.Background()
		workspaceID := "ws123"
		broadcastID := "bcast123"

		request := &domain.PauseBroadcastRequest{
			WorkspaceID: workspaceID,
			ID:          broadcastID,
		}

		// Create a paused broadcast that cannot be paused again
		pausedTime := time.Now().Add(-5 * time.Minute)
		existingBroadcast := &domain.Broadcast{
			ID:          broadcastID,
			WorkspaceID: workspaceID,
			Name:        "Test Broadcast",
			Status:      domain.BroadcastStatusPaused,
			CreatedAt:   time.Now().Add(-1 * time.Hour),
			UpdatedAt:   time.Now().Add(-1 * time.Hour),
			PausedAt:    &pausedTime,
		}

		// Mock authentication
		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
			Return(ctx, &domain.User{ID: "user123"}, nil)

		// Mock the WithTransaction call and execute the provided function with nil
		mockRepo.EXPECT().
			WithTransaction(gomock.Any(), workspaceID, gomock.Any()).
			DoAndReturn(func(ctx context.Context, workspaceID string, fn func(*sql.Tx) error) error {
				return fn(nil) // Pass nil as the transaction, it won't be used in the test
			})

		// Mock repository to return the existing broadcast
		mockRepo.EXPECT().
			GetBroadcastTx(gomock.Any(), gomock.Any(), workspaceID, broadcastID).
			Return(existingBroadcast, nil)

		// No update or event calls should be made
		mockRepo.EXPECT().UpdateBroadcastTx(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
		mockEventBus.EXPECT().PublishWithAck(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

		// Call the service
		err := service.PauseBroadcast(ctx, request)

		// Verify results
		require.Error(t, err)
		assert.Contains(t, err.Error(), "only broadcasts with sending status can be paused")
		assert.Contains(t, err.Error(), "current status: paused")
	})

	t.Run("Error_AuthenticationFailed", func(t *testing.T) {
		ctx := context.Background()
		workspaceID := "ws123"
		broadcastID := "bcast123"

		request := &domain.PauseBroadcastRequest{
			WorkspaceID: workspaceID,
			ID:          broadcastID,
		}

		// Mock authentication to return error
		authErr := errors.New("authentication failed")
		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
			Return(nil, nil, authErr)

		// No repository or event calls should be made due to authentication failure
		mockRepo.EXPECT().WithTransaction(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
		mockEventBus.EXPECT().PublishWithAck(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

		// Call the service
		err := service.PauseBroadcast(ctx, request)

		// Verify results
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to authenticate user")
	})

	t.Run("Error_ValidationFailed", func(t *testing.T) {
		ctx := context.Background()
		workspaceID := "ws123"

		// Create invalid request (missing ID)
		request := &domain.PauseBroadcastRequest{
			WorkspaceID: workspaceID,
			ID:          "", // Missing ID
		}

		// Mock authentication
		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
			Return(ctx, &domain.User{ID: "user123"}, nil)

		// No repository or event calls should be made due to validation failure
		mockRepo.EXPECT().WithTransaction(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
		mockEventBus.EXPECT().PublishWithAck(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

		// Call the service
		err := service.PauseBroadcast(ctx, request)

		// Verify results
		require.Error(t, err)
		assert.Contains(t, err.Error(), "required")
	})

	t.Run("Error_BroadcastNotFound", func(t *testing.T) {
		ctx := context.Background()
		workspaceID := "ws123"
		broadcastID := "nonexistent"

		request := &domain.PauseBroadcastRequest{
			WorkspaceID: workspaceID,
			ID:          broadcastID,
		}

		// Mock authentication
		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
			Return(ctx, &domain.User{ID: "user123"}, nil)

		// Mock the WithTransaction call and execute the provided function with nil
		mockRepo.EXPECT().
			WithTransaction(gomock.Any(), workspaceID, gomock.Any()).
			DoAndReturn(func(ctx context.Context, workspaceID string, fn func(*sql.Tx) error) error {
				return fn(nil) // Pass nil as the transaction, it won't be used in the test
			})

		// Mock repository to return not found error
		notFoundErr := &domain.ErrBroadcastNotFound{ID: broadcastID}
		mockRepo.EXPECT().
			GetBroadcastTx(gomock.Any(), gomock.Any(), workspaceID, broadcastID).
			Return(nil, notFoundErr)

		// No update or event calls should be made
		mockRepo.EXPECT().UpdateBroadcastTx(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
		mockEventBus.EXPECT().PublishWithAck(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

		// Call the service
		err := service.PauseBroadcast(ctx, request)

		// Verify results
		require.Error(t, err)
		assert.Equal(t, notFoundErr, err)
	})

	t.Run("Error_RepositoryUpdateFailed", func(t *testing.T) {
		ctx := context.Background()
		workspaceID := "ws123"
		broadcastID := "bcast123"

		request := &domain.PauseBroadcastRequest{
			WorkspaceID: workspaceID,
			ID:          broadcastID,
		}

		// Create a sending broadcast that can be paused
		startedTime := time.Now().Add(-10 * time.Minute)
		existingBroadcast := &domain.Broadcast{
			ID:          broadcastID,
			WorkspaceID: workspaceID,
			Name:        "Test Broadcast",
			Status:      domain.BroadcastStatusSending,
			CreatedAt:   time.Now().Add(-1 * time.Hour),
			UpdatedAt:   time.Now().Add(-1 * time.Hour),
			StartedAt:   &startedTime,
		}

		// Mock authentication
		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
			Return(ctx, &domain.User{ID: "user123"}, nil)

		// Mock the WithTransaction call and execute the provided function with nil
		mockRepo.EXPECT().
			WithTransaction(gomock.Any(), workspaceID, gomock.Any()).
			DoAndReturn(func(ctx context.Context, workspaceID string, fn func(*sql.Tx) error) error {
				return fn(nil) // Pass nil as the transaction, it won't be used in the test
			})

		// Mock repository to return the existing broadcast
		mockRepo.EXPECT().
			GetBroadcastTx(gomock.Any(), gomock.Any(), workspaceID, broadcastID).
			Return(existingBroadcast, nil)

		// Mock repository update to return error
		updateErr := errors.New("failed to update broadcast in database")
		mockRepo.EXPECT().
			UpdateBroadcastTx(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(updateErr)

		// No event calls should be made due to update failure
		mockEventBus.EXPECT().PublishWithAck(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

		// Call the service
		err := service.PauseBroadcast(ctx, request)

		// Verify results
		require.Error(t, err)
		assert.Equal(t, updateErr, err)
	})

	t.Run("Error_EventProcessingFailed", func(t *testing.T) {
		ctx := context.Background()
		workspaceID := "ws123"
		broadcastID := "bcast123"

		request := &domain.PauseBroadcastRequest{
			WorkspaceID: workspaceID,
			ID:          broadcastID,
		}

		// Create a sending broadcast that can be paused
		startedTime := time.Now().Add(-10 * time.Minute)
		existingBroadcast := &domain.Broadcast{
			ID:          broadcastID,
			WorkspaceID: workspaceID,
			Name:        "Test Broadcast",
			Status:      domain.BroadcastStatusSending,
			CreatedAt:   time.Now().Add(-1 * time.Hour),
			UpdatedAt:   time.Now().Add(-1 * time.Hour),
			StartedAt:   &startedTime,
		}

		// Mock authentication
		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
			Return(ctx, &domain.User{ID: "user123"}, nil)

		// Mock the WithTransaction call and execute the provided function with nil
		mockRepo.EXPECT().
			WithTransaction(gomock.Any(), workspaceID, gomock.Any()).
			DoAndReturn(func(ctx context.Context, workspaceID string, fn func(*sql.Tx) error) error {
				return fn(nil) // Pass nil as the transaction, it won't be used in the test
			})

		// Mock repository to return the existing broadcast
		mockRepo.EXPECT().
			GetBroadcastTx(gomock.Any(), gomock.Any(), workspaceID, broadcastID).
			Return(existingBroadcast, nil)

		// Mock repository update to succeed
		mockRepo.EXPECT().
			UpdateBroadcastTx(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil)

		// Mock event bus to simulate failed event processing
		eventErr := errors.New("event processing failed")
		mockEventBus.EXPECT().
			PublishWithAck(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, payload domain.EventPayload, callback domain.EventAckCallback) {
				// Call the callback with error
				callback(eventErr)
			})

		// Call the service
		err := service.PauseBroadcast(ctx, request)

		// Verify results
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to process pause event")
	})

	t.Run("Error_ContextCancelled", func(t *testing.T) {
		// Create a cancelled context
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		workspaceID := "ws123"
		broadcastID := "bcast123"

		request := &domain.PauseBroadcastRequest{
			WorkspaceID: workspaceID,
			ID:          broadcastID,
		}

		// Create a sending broadcast that can be paused
		startedTime := time.Now().Add(-10 * time.Minute)
		existingBroadcast := &domain.Broadcast{
			ID:          broadcastID,
			WorkspaceID: workspaceID,
			Name:        "Test Broadcast",
			Status:      domain.BroadcastStatusSending,
			CreatedAt:   time.Now().Add(-1 * time.Hour),
			UpdatedAt:   time.Now().Add(-1 * time.Hour),
			StartedAt:   &startedTime,
		}

		// Mock authentication
		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
			Return(ctx, &domain.User{ID: "user123"}, nil)

		// Mock the WithTransaction call and execute the provided function with nil
		mockRepo.EXPECT().
			WithTransaction(gomock.Any(), workspaceID, gomock.Any()).
			DoAndReturn(func(ctx context.Context, workspaceID string, fn func(*sql.Tx) error) error {
				return fn(nil) // Pass nil as the transaction, it won't be used in the test
			})

		// Mock repository to return the existing broadcast
		mockRepo.EXPECT().
			GetBroadcastTx(gomock.Any(), gomock.Any(), workspaceID, broadcastID).
			Return(existingBroadcast, nil)

		// Mock repository update to succeed
		mockRepo.EXPECT().
			UpdateBroadcastTx(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil)

		// Mock event bus but don't call the callback (simulate hanging event processing)
		mockEventBus.EXPECT().
			PublishWithAck(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, payload domain.EventPayload, callback domain.EventAckCallback) {
				// Don't call the callback, let context cancellation handle it
			})

		// Call the service
		err := service.PauseBroadcast(ctx, request)

		// Verify results
		require.Error(t, err)
		assert.Equal(t, context.Canceled, err)
	})
}

func TestBroadcastService_DeleteBroadcast(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockBroadcastRepository(ctrl)
	mockEmailSvc := mocks.NewMockEmailServiceInterface(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockContactRepo := mocks.NewMockContactRepository(ctrl)
	mockTemplateSvc := mocks.NewMockTemplateService(ctrl)
	mockAuthSvc := mocks.NewMockAuthService(ctrl)
	mockTaskService := mocks.NewMockTaskService(ctrl)
	mockEventBus := mocks.NewMockEventBus(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)

	// Set up logger mock to return itself for chaining
	mockLoggerWithField := pkgmocks.NewMockLogger(ctrl)
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLoggerWithField).AnyTimes()
	mockLoggerWithField.EXPECT().Error(gomock.Any()).AnyTimes()
	mockLoggerWithField.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLoggerWithField.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLoggerWithField.EXPECT().Warn(gomock.Any()).AnyTimes()

	// Add direct logger method expectations
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()

	service := NewBroadcastService(mockLogger, mockRepo, mockWorkspaceRepo, mockEmailSvc, mockContactRepo, mockTemplateSvc, mockTaskService, mockAuthSvc, mockEventBus, "https://api.example.com")

	t.Run("Success_DraftBroadcast", func(t *testing.T) {
		ctx := context.Background()
		workspaceID := "ws123"
		broadcastID := "bcast123"

		request := &domain.DeleteBroadcastRequest{
			WorkspaceID: workspaceID,
			ID:          broadcastID,
		}

		// Create a draft broadcast that can be deleted
		existingBroadcast := &domain.Broadcast{
			ID:          broadcastID,
			WorkspaceID: workspaceID,
			Name:        "Test Broadcast",
			Status:      domain.BroadcastStatusDraft,
			CreatedAt:   time.Now().Add(-1 * time.Hour),
			UpdatedAt:   time.Now().Add(-1 * time.Hour),
		}

		// Mock authentication
		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
			Return(ctx, &domain.User{ID: "user123"}, nil)

		// Mock repository to return the existing broadcast
		mockRepo.EXPECT().
			GetBroadcast(gomock.Any(), workspaceID, broadcastID).
			Return(existingBroadcast, nil)

		// Mock repository delete
		mockRepo.EXPECT().
			DeleteBroadcast(gomock.Any(), workspaceID, broadcastID).
			Return(nil)

		// Call the service
		err := service.DeleteBroadcast(ctx, request)

		// Verify results
		require.NoError(t, err)
	})

	t.Run("Success_ScheduledBroadcast", func(t *testing.T) {
		ctx := context.Background()
		workspaceID := "ws123"
		broadcastID := "bcast123"

		request := &domain.DeleteBroadcastRequest{
			WorkspaceID: workspaceID,
			ID:          broadcastID,
		}

		// Create a scheduled broadcast that can be deleted
		existingBroadcast := &domain.Broadcast{
			ID:          broadcastID,
			WorkspaceID: workspaceID,
			Name:        "Test Broadcast",
			Status:      domain.BroadcastStatusScheduled,
			CreatedAt:   time.Now().Add(-1 * time.Hour),
			UpdatedAt:   time.Now().Add(-1 * time.Hour),
		}

		// Mock authentication
		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
			Return(ctx, &domain.User{ID: "user123"}, nil)

		// Mock repository to return the existing broadcast
		mockRepo.EXPECT().
			GetBroadcast(gomock.Any(), workspaceID, broadcastID).
			Return(existingBroadcast, nil)

		// Mock repository delete
		mockRepo.EXPECT().
			DeleteBroadcast(gomock.Any(), workspaceID, broadcastID).
			Return(nil)

		// Call the service
		err := service.DeleteBroadcast(ctx, request)

		// Verify results
		require.NoError(t, err)
	})

	t.Run("Success_CancelledBroadcast", func(t *testing.T) {
		ctx := context.Background()
		workspaceID := "ws123"
		broadcastID := "bcast123"

		request := &domain.DeleteBroadcastRequest{
			WorkspaceID: workspaceID,
			ID:          broadcastID,
		}

		// Create a cancelled broadcast that can be deleted
		cancelledTime := time.Now().Add(-30 * time.Minute)
		existingBroadcast := &domain.Broadcast{
			ID:          broadcastID,
			WorkspaceID: workspaceID,
			Name:        "Test Broadcast",
			Status:      domain.BroadcastStatusCancelled,
			CreatedAt:   time.Now().Add(-1 * time.Hour),
			UpdatedAt:   time.Now().Add(-1 * time.Hour),
			CancelledAt: &cancelledTime,
		}

		// Mock authentication
		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
			Return(ctx, &domain.User{ID: "user123"}, nil)

		// Mock repository to return the existing broadcast
		mockRepo.EXPECT().
			GetBroadcast(gomock.Any(), workspaceID, broadcastID).
			Return(existingBroadcast, nil)

		// Mock repository delete
		mockRepo.EXPECT().
			DeleteBroadcast(gomock.Any(), workspaceID, broadcastID).
			Return(nil)

		// Call the service
		err := service.DeleteBroadcast(ctx, request)

		// Verify results
		require.NoError(t, err)
	})

	t.Run("Success_SentBroadcast", func(t *testing.T) {
		ctx := context.Background()
		workspaceID := "ws123"
		broadcastID := "bcast123"

		request := &domain.DeleteBroadcastRequest{
			WorkspaceID: workspaceID,
			ID:          broadcastID,
		}

		// Create a sent broadcast that can be deleted
		sentTime := time.Now().Add(-30 * time.Minute)
		existingBroadcast := &domain.Broadcast{
			ID:          broadcastID,
			WorkspaceID: workspaceID,
			Name:        "Test Broadcast",
			Status:      domain.BroadcastStatusSent,
			CreatedAt:   time.Now().Add(-1 * time.Hour),
			UpdatedAt:   time.Now().Add(-1 * time.Hour),
			SentAt:      &sentTime,
		}

		// Mock authentication
		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
			Return(ctx, &domain.User{ID: "user123"}, nil)

		// Mock repository to return the existing broadcast
		mockRepo.EXPECT().
			GetBroadcast(gomock.Any(), workspaceID, broadcastID).
			Return(existingBroadcast, nil)

		// Mock repository delete
		mockRepo.EXPECT().
			DeleteBroadcast(gomock.Any(), workspaceID, broadcastID).
			Return(nil)

		// Call the service
		err := service.DeleteBroadcast(ctx, request)

		// Verify results
		require.NoError(t, err)
	})

	t.Run("Error_SendingStatus", func(t *testing.T) {
		ctx := context.Background()
		workspaceID := "ws123"
		broadcastID := "bcast123"

		request := &domain.DeleteBroadcastRequest{
			WorkspaceID: workspaceID,
			ID:          broadcastID,
		}

		// Create a broadcast with sending status that cannot be deleted
		startedTime := time.Now().Add(-10 * time.Minute)
		existingBroadcast := &domain.Broadcast{
			ID:          broadcastID,
			WorkspaceID: workspaceID,
			Name:        "Test Broadcast",
			Status:      domain.BroadcastStatusSending,
			CreatedAt:   time.Now().Add(-1 * time.Hour),
			UpdatedAt:   time.Now().Add(-1 * time.Hour),
			StartedAt:   &startedTime,
		}

		// Mock authentication
		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
			Return(ctx, &domain.User{ID: "user123"}, nil)

		// Mock repository to return the existing broadcast
		mockRepo.EXPECT().
			GetBroadcast(gomock.Any(), workspaceID, broadcastID).
			Return(existingBroadcast, nil)

		// No delete call should be made
		mockRepo.EXPECT().DeleteBroadcast(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

		// Call the service
		err := service.DeleteBroadcast(ctx, request)

		// Verify results
		require.Error(t, err)
		assert.Contains(t, err.Error(), "broadcasts in 'sending' status cannot be deleted")
	})

	t.Run("Error_AuthenticationFailed", func(t *testing.T) {
		ctx := context.Background()
		workspaceID := "ws123"
		broadcastID := "bcast123"

		request := &domain.DeleteBroadcastRequest{
			WorkspaceID: workspaceID,
			ID:          broadcastID,
		}

		// Mock authentication to return error
		authErr := errors.New("authentication failed")
		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
			Return(nil, nil, authErr)

		// No repository calls should be made due to authentication failure
		mockRepo.EXPECT().GetBroadcast(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
		mockRepo.EXPECT().DeleteBroadcast(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

		// Call the service
		err := service.DeleteBroadcast(ctx, request)

		// Verify results
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to authenticate user")
	})

	t.Run("Error_ValidationFailed", func(t *testing.T) {
		ctx := context.Background()
		workspaceID := "ws123"

		// Create invalid request (missing ID)
		request := &domain.DeleteBroadcastRequest{
			WorkspaceID: workspaceID,
			ID:          "", // Missing ID
		}

		// Mock authentication
		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
			Return(ctx, &domain.User{ID: "user123"}, nil)

		// No repository calls should be made due to validation failure
		mockRepo.EXPECT().GetBroadcast(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
		mockRepo.EXPECT().DeleteBroadcast(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

		// Call the service
		err := service.DeleteBroadcast(ctx, request)

		// Verify results
		require.Error(t, err)
		assert.Contains(t, err.Error(), "required")
	})

	t.Run("Error_BroadcastNotFound", func(t *testing.T) {
		ctx := context.Background()
		workspaceID := "ws123"
		broadcastID := "nonexistent"

		request := &domain.DeleteBroadcastRequest{
			WorkspaceID: workspaceID,
			ID:          broadcastID,
		}

		// Mock authentication
		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
			Return(ctx, &domain.User{ID: "user123"}, nil)

		// Mock repository to return not found error
		notFoundErr := &domain.ErrBroadcastNotFound{ID: broadcastID}
		mockRepo.EXPECT().
			GetBroadcast(gomock.Any(), workspaceID, broadcastID).
			Return(nil, notFoundErr)

		// No delete call should be made
		mockRepo.EXPECT().DeleteBroadcast(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

		// Call the service
		err := service.DeleteBroadcast(ctx, request)

		// Verify results
		require.Error(t, err)
		assert.Equal(t, notFoundErr, err)
	})

	t.Run("Error_RepositoryGetFailed", func(t *testing.T) {
		ctx := context.Background()
		workspaceID := "ws123"
		broadcastID := "bcast123"

		request := &domain.DeleteBroadcastRequest{
			WorkspaceID: workspaceID,
			ID:          broadcastID,
		}

		// Mock authentication
		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
			Return(ctx, &domain.User{ID: "user123"}, nil)

		// Mock repository to return database error
		dbErr := errors.New("database connection failed")
		mockRepo.EXPECT().
			GetBroadcast(gomock.Any(), workspaceID, broadcastID).
			Return(nil, dbErr)

		// No delete call should be made
		mockRepo.EXPECT().DeleteBroadcast(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

		// Call the service
		err := service.DeleteBroadcast(ctx, request)

		// Verify results
		require.Error(t, err)
		assert.Equal(t, dbErr, err)
	})

	t.Run("Error_RepositoryDeleteFailed", func(t *testing.T) {
		ctx := context.Background()
		workspaceID := "ws123"
		broadcastID := "bcast123"

		request := &domain.DeleteBroadcastRequest{
			WorkspaceID: workspaceID,
			ID:          broadcastID,
		}

		// Create a draft broadcast that can be deleted
		existingBroadcast := &domain.Broadcast{
			ID:          broadcastID,
			WorkspaceID: workspaceID,
			Name:        "Test Broadcast",
			Status:      domain.BroadcastStatusDraft,
			CreatedAt:   time.Now().Add(-1 * time.Hour),
			UpdatedAt:   time.Now().Add(-1 * time.Hour),
		}

		// Mock authentication
		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
			Return(ctx, &domain.User{ID: "user123"}, nil)

		// Mock repository to return the existing broadcast
		mockRepo.EXPECT().
			GetBroadcast(gomock.Any(), workspaceID, broadcastID).
			Return(existingBroadcast, nil)

		// Mock repository delete to return error
		deleteErr := errors.New("failed to delete from database")
		mockRepo.EXPECT().
			DeleteBroadcast(gomock.Any(), workspaceID, broadcastID).
			Return(deleteErr)

		// Call the service
		err := service.DeleteBroadcast(ctx, request)

		// Verify results
		require.Error(t, err)
		assert.Equal(t, deleteErr, err)
	})

	t.Run("Success_PausedBroadcast", func(t *testing.T) {
		ctx := context.Background()
		workspaceID := "ws123"
		broadcastID := "bcast123"

		request := &domain.DeleteBroadcastRequest{
			WorkspaceID: workspaceID,
			ID:          broadcastID,
		}

		// Create a paused broadcast that can be deleted
		pausedTime := time.Now().Add(-15 * time.Minute)
		existingBroadcast := &domain.Broadcast{
			ID:          broadcastID,
			WorkspaceID: workspaceID,
			Name:        "Test Broadcast",
			Status:      domain.BroadcastStatusPaused,
			CreatedAt:   time.Now().Add(-1 * time.Hour),
			UpdatedAt:   time.Now().Add(-1 * time.Hour),
			PausedAt:    &pausedTime,
		}

		// Mock authentication
		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
			Return(ctx, &domain.User{ID: "user123"}, nil)

		// Mock repository to return the existing broadcast
		mockRepo.EXPECT().
			GetBroadcast(gomock.Any(), workspaceID, broadcastID).
			Return(existingBroadcast, nil)

		// Mock repository delete
		mockRepo.EXPECT().
			DeleteBroadcast(gomock.Any(), workspaceID, broadcastID).
			Return(nil)

		// Call the service
		err := service.DeleteBroadcast(ctx, request)

		// Verify results
		require.NoError(t, err)
	})
}

func TestBroadcastService_SendToIndividual(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRepo := mocks.NewMockBroadcastRepository(ctrl)
		mockEmailSvc := mocks.NewMockEmailServiceInterface(ctrl)
		mockLogger := pkgmocks.NewMockLogger(ctrl)
		mockContactRepo := mocks.NewMockContactRepository(ctrl)
		mockTemplateSvc := mocks.NewMockTemplateService(ctrl)
		mockAuthSvc := mocks.NewMockAuthService(ctrl)
		mockTaskService := mocks.NewMockTaskService(ctrl)
		mockEventBus := mocks.NewMockEventBus(ctrl)
		mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)

		// Set up logger mock to return itself for chaining
		mockLoggerWithFields := pkgmocks.NewMockLogger(ctrl)
		mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLoggerWithFields).AnyTimes()
		mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLoggerWithFields).AnyTimes()
		mockLoggerWithFields.EXPECT().Error(gomock.Any()).AnyTimes()
		mockLoggerWithFields.EXPECT().Info(gomock.Any()).AnyTimes()
		mockLoggerWithFields.EXPECT().Debug(gomock.Any()).AnyTimes()

		// Add direct logger method expectations
		mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
		mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
		mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
		mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()

		apiEndpoint := "https://api.example.com"
		service := NewBroadcastService(mockLogger, mockRepo, mockWorkspaceRepo, mockEmailSvc, mockContactRepo, mockTemplateSvc, mockTaskService, mockAuthSvc, mockEventBus, apiEndpoint)

		ctx := context.Background()
		workspaceID := "ws123"
		broadcastID := "bcast123"
		recipientEmail := "test@example.com"
		variationID := "variation123"
		emailSender := domain.NewEmailSender("test@example.com", "Test Sender")

		// Create the request
		request := &domain.SendToIndividualRequest{
			WorkspaceID:    workspaceID,
			BroadcastID:    broadcastID,
			RecipientEmail: recipientEmail,
			VariationID:    variationID,
		}

		// Mock auth service to authenticate the user
		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
			Return(ctx, &domain.User{ID: "user123"}, nil)

		// Create a broadcast with the test variation
		broadcast := &domain.Broadcast{
			ID:          broadcastID,
			WorkspaceID: workspaceID,
			Name:        "Test Broadcast",
			Status:      domain.BroadcastStatusDraft,
			TestSettings: domain.BroadcastTestSettings{
				Variations: []domain.BroadcastVariation{
					{
						ID:         variationID,
						TemplateID: "template123",
					},
				},
			},
			UTMParameters: &domain.UTMParameters{
				Source:   "test_source",
				Medium:   "test_medium",
				Campaign: "test_campaign",
				Content:  "test_content",
			},
		}

		// Mock repository to return the broadcast
		mockRepo.EXPECT().
			GetBroadcast(gomock.Any(), workspaceID, broadcastID).
			Return(broadcast, nil)

		// Mock workspace repository to return a workspace with marketing email provider
		workspace := &domain.Workspace{
			ID:   workspaceID,
			Name: "Test Workspace",
			Settings: domain.WorkspaceSettings{
				TransactionalEmailProviderID: "email-provider-123",
				MarketingEmailProviderID:     "email-provider-123",
				EmailTrackingEnabled:         true,
				SecretKey:                    "test-secret-key",
			},
			Integrations: []domain.Integration{
				{
					ID:   "email-provider-123",
					Type: domain.IntegrationTypeEmail,
					EmailProvider: domain.EmailProvider{
						Kind: domain.EmailProviderKindSMTP,
						Senders: []domain.EmailSender{
							emailSender,
						},
					},
				},
			},
		}
		mockWorkspaceRepo.EXPECT().
			GetByID(gomock.Any(), workspaceID).
			Return(workspace, nil)

		// Mock contact repository to return a contact
		contact := &domain.Contact{
			Email: recipientEmail,
			FirstName: &domain.NullableString{
				String: "Test",
				IsNull: false,
			},
			LastName: &domain.NullableString{
				String: "User",
				IsNull: false,
			},
		}
		mockContactRepo.EXPECT().
			GetContactByEmail(gomock.Any(), workspaceID, recipientEmail).
			Return(contact, nil)

		// Mock template service to return a template
		emailBlock := getTestEmailBlock()
		template := &domain.Template{
			ID:   "template123",
			Name: "Test Template",
			Email: &domain.EmailTemplate{
				Subject:          "Test Subject",
				SenderID:         emailSender.ID,
				VisualEditorTree: emailBlock,
			},
		}
		mockTemplateSvc.EXPECT().
			GetTemplateByID(gomock.Any(), workspaceID, "template123", int64(0)).
			Return(template, nil)

		// Mock template service to compile template
		compiledHTML := "<html><body>Test Content</body></html>"
		compiledResult := &domain.CompileTemplateResponse{
			Success: true,
			HTML:    &compiledHTML,
		}
		mockTemplateSvc.EXPECT().
			CompileTemplate(gomock.Any(), gomock.Any()).
			Return(compiledResult, nil)

		// Mock email service to send email
		mockEmailSvc.EXPECT().
			SendEmail(
				gomock.Any(),
				workspaceID,
				gomock.Any(), // messageID
				true,         // isMarketing
				emailSender.Email,
				emailSender.Name,
				recipientEmail,
				template.Email.Subject,
				*compiledResult.HTML,
				nil,
				domain.EmailOptions{},
			).
			Return(nil)

		// Call the service
		err := service.SendToIndividual(ctx, request)

		// Verify results
		require.NoError(t, err)
	})

	t.Run("NoMarketingEmailProvider", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRepo := mocks.NewMockBroadcastRepository(ctrl)
		mockEmailSvc := mocks.NewMockEmailServiceInterface(ctrl)
		mockLogger := pkgmocks.NewMockLogger(ctrl)
		mockContactRepo := mocks.NewMockContactRepository(ctrl)
		mockTemplateSvc := mocks.NewMockTemplateService(ctrl)
		mockAuthSvc := mocks.NewMockAuthService(ctrl)
		mockTaskService := mocks.NewMockTaskService(ctrl)
		mockEventBus := mocks.NewMockEventBus(ctrl)
		mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)

		// Set up logger mock to return itself for chaining
		mockLoggerWithFields := pkgmocks.NewMockLogger(ctrl)
		mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLoggerWithFields).AnyTimes()
		mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLoggerWithFields).AnyTimes()
		mockLoggerWithFields.EXPECT().Error(gomock.Any()).AnyTimes()
		mockLoggerWithFields.EXPECT().Info(gomock.Any()).AnyTimes()
		mockLoggerWithFields.EXPECT().Debug(gomock.Any()).AnyTimes()

		// Add direct logger method expectations
		mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
		mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
		mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
		mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()

		apiEndpoint := "https://api.example.com"
		service := NewBroadcastService(mockLogger, mockRepo, mockWorkspaceRepo, mockEmailSvc, mockContactRepo, mockTemplateSvc, mockTaskService, mockAuthSvc, mockEventBus, apiEndpoint)

		ctx := context.Background()
		workspaceID := "ws123"
		broadcastID := "bcast123"
		recipientEmail := "test@example.com"
		variationID := "variation123"
		emailSender := domain.NewEmailSender("test@example.com", "Test Sender")
		// Create the request
		request := &domain.SendToIndividualRequest{
			WorkspaceID:    workspaceID,
			BroadcastID:    broadcastID,
			RecipientEmail: recipientEmail,
			VariationID:    variationID,
		}

		// Mock auth service to authenticate the user
		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
			Return(ctx, &domain.User{ID: "user123"}, nil)

		// Create a workspace with no marketing email provider
		workspace := &domain.Workspace{
			ID:   workspaceID,
			Name: "Test Workspace",
			Settings: domain.WorkspaceSettings{
				// No marketing email provider ID set
				TransactionalEmailProviderID: "email-provider-123",
				MarketingEmailProviderID:     "",
			},
			Integrations: []domain.Integration{
				{
					ID:   "email-provider-123",
					Type: domain.IntegrationTypeEmail,
					EmailProvider: domain.EmailProvider{
						Kind: domain.EmailProviderKindSMTP,
						Senders: []domain.EmailSender{
							emailSender,
						},
					},
				},
			},
		}

		// Mock workspace repository to return a workspace without a marketing email provider
		mockWorkspaceRepo.EXPECT().
			GetByID(gomock.Any(), workspaceID).
			Return(workspace, nil)

		// No calls to repository expected after workspace check
		mockRepo.EXPECT().GetBroadcast(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

		// Call the service
		err := service.SendToIndividual(ctx, request)

		// Verify results
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no marketing email provider configured")
	})
}

func getTestEmailBlock() notifuse_mjml.EmailBlock {
	return &notifuse_mjml.MJMLBlock{
		BaseBlock: notifuse_mjml.BaseBlock{
			ID:   "root",
			Type: notifuse_mjml.MJMLComponentMjml,
			Attributes: map[string]interface{}{
				"version":         "4.0.0",
				"backgroundColor": "#ffffff",
			},
		},
	}
}

func TestBroadcastService_ResumeBroadcast(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockBroadcastRepository(ctrl)
	mockEmailSvc := mocks.NewMockEmailServiceInterface(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockContactRepo := mocks.NewMockContactRepository(ctrl)
	mockTemplateSvc := mocks.NewMockTemplateService(ctrl)
	mockAuthSvc := mocks.NewMockAuthService(ctrl)
	mockTaskService := mocks.NewMockTaskService(ctrl)
	mockEventBus := mocks.NewMockEventBus(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)

	// Set up logger mock to return itself for chaining
	mockLoggerWithField := pkgmocks.NewMockLogger(ctrl)
	mockLoggerWithFields := pkgmocks.NewMockLogger(ctrl)
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLoggerWithField).AnyTimes()
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLoggerWithFields).AnyTimes()
	mockLoggerWithField.EXPECT().Error(gomock.Any()).AnyTimes()
	mockLoggerWithField.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLoggerWithField.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLoggerWithField.EXPECT().Warn(gomock.Any()).AnyTimes()
	mockLoggerWithFields.EXPECT().Error(gomock.Any()).AnyTimes()
	mockLoggerWithFields.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLoggerWithFields.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLoggerWithFields.EXPECT().Warn(gomock.Any()).AnyTimes()

	// Add direct logger method expectations
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()

	service := NewBroadcastService(mockLogger, mockRepo, mockWorkspaceRepo, mockEmailSvc, mockContactRepo, mockTemplateSvc, mockTaskService, mockAuthSvc, mockEventBus, "https://api.example.com")

	t.Run("Success_ResumeToSending_NotScheduled", func(t *testing.T) {
		ctx := context.Background()
		workspaceID := "ws123"
		broadcastID := "bcast123"

		request := &domain.ResumeBroadcastRequest{
			WorkspaceID: workspaceID,
			ID:          broadcastID,
		}

		// Create a paused broadcast that was not originally scheduled
		pausedTime := time.Now().Add(-10 * time.Minute)
		existingBroadcast := &domain.Broadcast{
			ID:          broadcastID,
			WorkspaceID: workspaceID,
			Name:        "Test Broadcast",
			Status:      domain.BroadcastStatusPaused,
			CreatedAt:   time.Now().Add(-1 * time.Hour),
			UpdatedAt:   time.Now().Add(-1 * time.Hour),
			PausedAt:    &pausedTime,
			Schedule: domain.ScheduleSettings{
				IsScheduled: false, // Not scheduled
			},
		}

		// Mock authentication
		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
			Return(ctx, &domain.User{ID: "user123"}, nil)

		// Mock the WithTransaction call and execute the provided function with nil
		mockRepo.EXPECT().
			WithTransaction(gomock.Any(), workspaceID, gomock.Any()).
			DoAndReturn(func(ctx context.Context, workspaceID string, fn func(*sql.Tx) error) error {
				return fn(nil) // Pass nil as the transaction, it won't be used in the test
			})

		// Mock repository to return the existing broadcast
		mockRepo.EXPECT().
			GetBroadcastTx(gomock.Any(), gomock.Any(), workspaceID, broadcastID).
			Return(existingBroadcast, nil)

		// Mock repository update with verification
		mockRepo.EXPECT().
			UpdateBroadcastTx(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, _ *sql.Tx, broadcast *domain.Broadcast) error {
				// Verify important properties
				assert.Equal(t, broadcastID, broadcast.ID)
				assert.Equal(t, workspaceID, broadcast.WorkspaceID)
				assert.Equal(t, domain.BroadcastStatusSending, broadcast.Status)
				assert.Nil(t, broadcast.PausedAt)     // Should be cleared
				assert.NotNil(t, broadcast.StartedAt) // Should be set
				return nil
			})

		// Mock event bus to simulate successful event processing
		mockEventBus.EXPECT().
			PublishWithAck(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, payload domain.EventPayload, callback domain.EventAckCallback) {
				// Verify event payload
				assert.Equal(t, domain.EventBroadcastResumed, payload.Type)
				assert.Equal(t, workspaceID, payload.WorkspaceID)
				assert.Equal(t, broadcastID, payload.EntityID)
				assert.Equal(t, broadcastID, payload.Data["broadcast_id"])
				assert.Equal(t, true, payload.Data["start_now"])

				// Call the callback with success
				callback(nil)
			})

		// Call the service
		err := service.ResumeBroadcast(ctx, request)

		// Verify results
		require.NoError(t, err)
	})

	t.Run("Success_ResumeToScheduled_FutureSchedule", func(t *testing.T) {
		ctx := context.Background()
		workspaceID := "ws123"
		broadcastID := "bcast123"

		request := &domain.ResumeBroadcastRequest{
			WorkspaceID: workspaceID,
			ID:          broadcastID,
		}

		// Create a paused broadcast that was originally scheduled for the future
		pausedTime := time.Now().Add(-10 * time.Minute)
		futureTime := time.Now().Add(2 * time.Hour)
		existingBroadcast := &domain.Broadcast{
			ID:          broadcastID,
			WorkspaceID: workspaceID,
			Name:        "Test Broadcast",
			Status:      domain.BroadcastStatusPaused,
			CreatedAt:   time.Now().Add(-1 * time.Hour),
			UpdatedAt:   time.Now().Add(-1 * time.Hour),
			PausedAt:    &pausedTime,
			Schedule: domain.ScheduleSettings{
				IsScheduled:   true,
				ScheduledDate: futureTime.Format("2006-01-02"),
				ScheduledTime: futureTime.Format("15:04"),
				Timezone:      "UTC",
			},
		}

		// Mock authentication
		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
			Return(ctx, &domain.User{ID: "user123"}, nil)

		// Mock the WithTransaction call and execute the provided function with nil
		mockRepo.EXPECT().
			WithTransaction(gomock.Any(), workspaceID, gomock.Any()).
			DoAndReturn(func(ctx context.Context, workspaceID string, fn func(*sql.Tx) error) error {
				return fn(nil) // Pass nil as the transaction, it won't be used in the test
			})

		// Mock repository to return the existing broadcast
		mockRepo.EXPECT().
			GetBroadcastTx(gomock.Any(), gomock.Any(), workspaceID, broadcastID).
			Return(existingBroadcast, nil)

		// Mock repository update with verification
		mockRepo.EXPECT().
			UpdateBroadcastTx(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, _ *sql.Tx, broadcast *domain.Broadcast) error {
				// Verify important properties
				assert.Equal(t, broadcastID, broadcast.ID)
				assert.Equal(t, workspaceID, broadcast.WorkspaceID)
				assert.Equal(t, domain.BroadcastStatusScheduled, broadcast.Status)
				assert.Nil(t, broadcast.PausedAt)  // Should be cleared
				assert.Nil(t, broadcast.StartedAt) // Should remain nil for scheduled
				return nil
			})

		// Mock event bus to simulate successful event processing
		mockEventBus.EXPECT().
			PublishWithAck(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, payload domain.EventPayload, callback domain.EventAckCallback) {
				// Verify event payload
				assert.Equal(t, domain.EventBroadcastResumed, payload.Type)
				assert.Equal(t, workspaceID, payload.WorkspaceID)
				assert.Equal(t, broadcastID, payload.EntityID)
				assert.Equal(t, broadcastID, payload.Data["broadcast_id"])
				assert.Equal(t, false, payload.Data["start_now"])

				// Call the callback with success
				callback(nil)
			})

		// Call the service
		err := service.ResumeBroadcast(ctx, request)

		// Verify results
		require.NoError(t, err)
	})

	t.Run("Success_ResumeToSending_PastSchedule", func(t *testing.T) {
		ctx := context.Background()
		workspaceID := "ws123"
		broadcastID := "bcast123"

		request := &domain.ResumeBroadcastRequest{
			WorkspaceID: workspaceID,
			ID:          broadcastID,
		}

		// Create a paused broadcast that was originally scheduled for the past
		pausedTime := time.Now().Add(-10 * time.Minute)
		pastTime := time.Now().Add(-2 * time.Hour)
		existingBroadcast := &domain.Broadcast{
			ID:          broadcastID,
			WorkspaceID: workspaceID,
			Name:        "Test Broadcast",
			Status:      domain.BroadcastStatusPaused,
			CreatedAt:   time.Now().Add(-3 * time.Hour),
			UpdatedAt:   time.Now().Add(-1 * time.Hour),
			PausedAt:    &pausedTime,
			Schedule: domain.ScheduleSettings{
				IsScheduled:   true,
				ScheduledDate: pastTime.Format("2006-01-02"),
				ScheduledTime: pastTime.Format("15:04"),
				Timezone:      "UTC",
			},
		}

		// Mock authentication
		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
			Return(ctx, &domain.User{ID: "user123"}, nil)

		// Mock the WithTransaction call and execute the provided function with nil
		mockRepo.EXPECT().
			WithTransaction(gomock.Any(), workspaceID, gomock.Any()).
			DoAndReturn(func(ctx context.Context, workspaceID string, fn func(*sql.Tx) error) error {
				return fn(nil) // Pass nil as the transaction, it won't be used in the test
			})

		// Mock repository to return the existing broadcast
		mockRepo.EXPECT().
			GetBroadcastTx(gomock.Any(), gomock.Any(), workspaceID, broadcastID).
			Return(existingBroadcast, nil)

		// Mock repository update with verification
		mockRepo.EXPECT().
			UpdateBroadcastTx(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, _ *sql.Tx, broadcast *domain.Broadcast) error {
				// Verify important properties
				assert.Equal(t, broadcastID, broadcast.ID)
				assert.Equal(t, workspaceID, broadcast.WorkspaceID)
				assert.Equal(t, domain.BroadcastStatusSending, broadcast.Status)
				assert.Nil(t, broadcast.PausedAt)     // Should be cleared
				assert.NotNil(t, broadcast.StartedAt) // Should be set
				return nil
			})

		// Mock event bus to simulate successful event processing
		mockEventBus.EXPECT().
			PublishWithAck(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, payload domain.EventPayload, callback domain.EventAckCallback) {
				// Verify event payload
				assert.Equal(t, domain.EventBroadcastResumed, payload.Type)
				assert.Equal(t, workspaceID, payload.WorkspaceID)
				assert.Equal(t, broadcastID, payload.EntityID)
				assert.Equal(t, broadcastID, payload.Data["broadcast_id"])
				assert.Equal(t, true, payload.Data["start_now"])

				// Call the callback with success
				callback(nil)
			})

		// Call the service
		err := service.ResumeBroadcast(ctx, request)

		// Verify results
		require.NoError(t, err)
	})

	t.Run("Success_ResumeToSending_AlreadyStarted", func(t *testing.T) {
		ctx := context.Background()
		workspaceID := "ws123"
		broadcastID := "bcast123"

		request := &domain.ResumeBroadcastRequest{
			WorkspaceID: workspaceID,
			ID:          broadcastID,
		}

		// Create a paused broadcast that was already started
		pausedTime := time.Now().Add(-10 * time.Minute)
		startedTime := time.Now().Add(-30 * time.Minute)
		existingBroadcast := &domain.Broadcast{
			ID:          broadcastID,
			WorkspaceID: workspaceID,
			Name:        "Test Broadcast",
			Status:      domain.BroadcastStatusPaused,
			CreatedAt:   time.Now().Add(-1 * time.Hour),
			UpdatedAt:   time.Now().Add(-1 * time.Hour),
			PausedAt:    &pausedTime,
			StartedAt:   &startedTime, // Already started
			Schedule: domain.ScheduleSettings{
				IsScheduled: false,
			},
		}

		// Mock authentication
		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
			Return(ctx, &domain.User{ID: "user123"}, nil)

		// Mock the WithTransaction call and execute the provided function with nil
		mockRepo.EXPECT().
			WithTransaction(gomock.Any(), workspaceID, gomock.Any()).
			DoAndReturn(func(ctx context.Context, workspaceID string, fn func(*sql.Tx) error) error {
				return fn(nil) // Pass nil as the transaction, it won't be used in the test
			})

		// Mock repository to return the existing broadcast
		mockRepo.EXPECT().
			GetBroadcastTx(gomock.Any(), gomock.Any(), workspaceID, broadcastID).
			Return(existingBroadcast, nil)

		// Mock repository update with verification
		mockRepo.EXPECT().
			UpdateBroadcastTx(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, _ *sql.Tx, broadcast *domain.Broadcast) error {
				// Verify important properties
				assert.Equal(t, broadcastID, broadcast.ID)
				assert.Equal(t, workspaceID, broadcast.WorkspaceID)
				assert.Equal(t, domain.BroadcastStatusSending, broadcast.Status)
				assert.Nil(t, broadcast.PausedAt)                  // Should be cleared
				assert.Equal(t, startedTime, *broadcast.StartedAt) // Should remain the same
				return nil
			})

		// Mock event bus to simulate successful event processing
		mockEventBus.EXPECT().
			PublishWithAck(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, payload domain.EventPayload, callback domain.EventAckCallback) {
				// Call the callback with success
				callback(nil)
			})

		// Call the service
		err := service.ResumeBroadcast(ctx, request)

		// Verify results
		require.NoError(t, err)
	})

	t.Run("Error_InvalidStatus_Draft", func(t *testing.T) {
		ctx := context.Background()
		workspaceID := "ws123"
		broadcastID := "bcast123"

		request := &domain.ResumeBroadcastRequest{
			WorkspaceID: workspaceID,
			ID:          broadcastID,
		}

		// Create a draft broadcast that cannot be resumed
		existingBroadcast := &domain.Broadcast{
			ID:          broadcastID,
			WorkspaceID: workspaceID,
			Name:        "Test Broadcast",
			Status:      domain.BroadcastStatusDraft,
			CreatedAt:   time.Now().Add(-1 * time.Hour),
			UpdatedAt:   time.Now().Add(-1 * time.Hour),
		}

		// Mock authentication
		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
			Return(ctx, &domain.User{ID: "user123"}, nil)

		// Mock the WithTransaction call and execute the provided function with nil
		mockRepo.EXPECT().
			WithTransaction(gomock.Any(), workspaceID, gomock.Any()).
			DoAndReturn(func(ctx context.Context, workspaceID string, fn func(*sql.Tx) error) error {
				return fn(nil) // Pass nil as the transaction, it won't be used in the test
			})

		// Mock repository to return the existing broadcast
		mockRepo.EXPECT().
			GetBroadcastTx(gomock.Any(), gomock.Any(), workspaceID, broadcastID).
			Return(existingBroadcast, nil)

		// No update or event calls should be made
		mockRepo.EXPECT().UpdateBroadcastTx(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
		mockEventBus.EXPECT().PublishWithAck(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

		// Call the service
		err := service.ResumeBroadcast(ctx, request)

		// Verify results
		require.Error(t, err)
		assert.Contains(t, err.Error(), "only broadcasts with paused status can be resumed")
		assert.Contains(t, err.Error(), "current status: draft")
	})

	t.Run("Error_InvalidStatus_Sending", func(t *testing.T) {
		ctx := context.Background()
		workspaceID := "ws123"
		broadcastID := "bcast123"

		request := &domain.ResumeBroadcastRequest{
			WorkspaceID: workspaceID,
			ID:          broadcastID,
		}

		// Create a sending broadcast that cannot be resumed
		startedTime := time.Now().Add(-10 * time.Minute)
		existingBroadcast := &domain.Broadcast{
			ID:          broadcastID,
			WorkspaceID: workspaceID,
			Name:        "Test Broadcast",
			Status:      domain.BroadcastStatusSending,
			CreatedAt:   time.Now().Add(-1 * time.Hour),
			UpdatedAt:   time.Now().Add(-1 * time.Hour),
			StartedAt:   &startedTime,
		}

		// Mock authentication
		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
			Return(ctx, &domain.User{ID: "user123"}, nil)

		// Mock the WithTransaction call and execute the provided function with nil
		mockRepo.EXPECT().
			WithTransaction(gomock.Any(), workspaceID, gomock.Any()).
			DoAndReturn(func(ctx context.Context, workspaceID string, fn func(*sql.Tx) error) error {
				return fn(nil) // Pass nil as the transaction, it won't be used in the test
			})

		// Mock repository to return the existing broadcast
		mockRepo.EXPECT().
			GetBroadcastTx(gomock.Any(), gomock.Any(), workspaceID, broadcastID).
			Return(existingBroadcast, nil)

		// No update or event calls should be made
		mockRepo.EXPECT().UpdateBroadcastTx(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
		mockEventBus.EXPECT().PublishWithAck(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

		// Call the service
		err := service.ResumeBroadcast(ctx, request)

		// Verify results
		require.Error(t, err)
		assert.Contains(t, err.Error(), "only broadcasts with paused status can be resumed")
		assert.Contains(t, err.Error(), "current status: sending")
	})

	t.Run("Error_InvalidStatus_Sent", func(t *testing.T) {
		ctx := context.Background()
		workspaceID := "ws123"
		broadcastID := "bcast123"

		request := &domain.ResumeBroadcastRequest{
			WorkspaceID: workspaceID,
			ID:          broadcastID,
		}

		// Create a sent broadcast that cannot be resumed
		sentTime := time.Now().Add(-30 * time.Minute)
		existingBroadcast := &domain.Broadcast{
			ID:          broadcastID,
			WorkspaceID: workspaceID,
			Name:        "Test Broadcast",
			Status:      domain.BroadcastStatusSent,
			CreatedAt:   time.Now().Add(-1 * time.Hour),
			UpdatedAt:   time.Now().Add(-1 * time.Hour),
			SentAt:      &sentTime,
		}

		// Mock authentication
		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
			Return(ctx, &domain.User{ID: "user123"}, nil)

		// Mock the WithTransaction call and execute the provided function with nil
		mockRepo.EXPECT().
			WithTransaction(gomock.Any(), workspaceID, gomock.Any()).
			DoAndReturn(func(ctx context.Context, workspaceID string, fn func(*sql.Tx) error) error {
				return fn(nil) // Pass nil as the transaction, it won't be used in the test
			})

		// Mock repository to return the existing broadcast
		mockRepo.EXPECT().
			GetBroadcastTx(gomock.Any(), gomock.Any(), workspaceID, broadcastID).
			Return(existingBroadcast, nil)

		// No update or event calls should be made
		mockRepo.EXPECT().UpdateBroadcastTx(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
		mockEventBus.EXPECT().PublishWithAck(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

		// Call the service
		err := service.ResumeBroadcast(ctx, request)

		// Verify results
		require.Error(t, err)
		assert.Contains(t, err.Error(), "only broadcasts with paused status can be resumed")
		assert.Contains(t, err.Error(), "current status: sent")
	})

	t.Run("Error_AuthenticationFailed", func(t *testing.T) {
		ctx := context.Background()
		workspaceID := "ws123"
		broadcastID := "bcast123"

		request := &domain.ResumeBroadcastRequest{
			WorkspaceID: workspaceID,
			ID:          broadcastID,
		}

		// Mock authentication to return error
		authErr := errors.New("authentication failed")
		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
			Return(nil, nil, authErr)

		// No repository or event calls should be made due to authentication failure
		mockRepo.EXPECT().WithTransaction(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
		mockEventBus.EXPECT().PublishWithAck(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

		// Call the service
		err := service.ResumeBroadcast(ctx, request)

		// Verify results
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to authenticate user")
	})

	t.Run("Error_ValidationFailed", func(t *testing.T) {
		ctx := context.Background()
		workspaceID := "ws123"

		// Create invalid request (missing ID)
		request := &domain.ResumeBroadcastRequest{
			WorkspaceID: workspaceID,
			ID:          "", // Missing ID
		}

		// Mock authentication
		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
			Return(ctx, &domain.User{ID: "user123"}, nil)

		// No repository or event calls should be made due to validation failure
		mockRepo.EXPECT().WithTransaction(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
		mockEventBus.EXPECT().PublishWithAck(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

		// Call the service
		err := service.ResumeBroadcast(ctx, request)

		// Verify results
		require.Error(t, err)
		assert.Contains(t, err.Error(), "required")
	})

	t.Run("Error_BroadcastNotFound", func(t *testing.T) {
		ctx := context.Background()
		workspaceID := "ws123"
		broadcastID := "nonexistent"

		request := &domain.ResumeBroadcastRequest{
			WorkspaceID: workspaceID,
			ID:          broadcastID,
		}

		// Mock authentication
		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
			Return(ctx, &domain.User{ID: "user123"}, nil)

		// Mock the WithTransaction call and execute the provided function with nil
		mockRepo.EXPECT().
			WithTransaction(gomock.Any(), workspaceID, gomock.Any()).
			DoAndReturn(func(ctx context.Context, workspaceID string, fn func(*sql.Tx) error) error {
				return fn(nil) // Pass nil as the transaction, it won't be used in the test
			})

		// Mock repository to return not found error
		notFoundErr := &domain.ErrBroadcastNotFound{ID: broadcastID}
		mockRepo.EXPECT().
			GetBroadcastTx(gomock.Any(), gomock.Any(), workspaceID, broadcastID).
			Return(nil, notFoundErr)

		// No update or event calls should be made
		mockRepo.EXPECT().UpdateBroadcastTx(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
		mockEventBus.EXPECT().PublishWithAck(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

		// Call the service
		err := service.ResumeBroadcast(ctx, request)

		// Verify results
		require.Error(t, err)
		assert.Equal(t, notFoundErr, err)
	})

	t.Run("Error_RepositoryUpdateFailed", func(t *testing.T) {
		ctx := context.Background()
		workspaceID := "ws123"
		broadcastID := "bcast123"

		request := &domain.ResumeBroadcastRequest{
			WorkspaceID: workspaceID,
			ID:          broadcastID,
		}

		// Create a paused broadcast that can be resumed
		pausedTime := time.Now().Add(-10 * time.Minute)
		existingBroadcast := &domain.Broadcast{
			ID:          broadcastID,
			WorkspaceID: workspaceID,
			Name:        "Test Broadcast",
			Status:      domain.BroadcastStatusPaused,
			CreatedAt:   time.Now().Add(-1 * time.Hour),
			UpdatedAt:   time.Now().Add(-1 * time.Hour),
			PausedAt:    &pausedTime,
			Schedule: domain.ScheduleSettings{
				IsScheduled: false,
			},
		}

		// Mock authentication
		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
			Return(ctx, &domain.User{ID: "user123"}, nil)

		// Mock the WithTransaction call and execute the provided function with nil
		mockRepo.EXPECT().
			WithTransaction(gomock.Any(), workspaceID, gomock.Any()).
			DoAndReturn(func(ctx context.Context, workspaceID string, fn func(*sql.Tx) error) error {
				return fn(nil) // Pass nil as the transaction, it won't be used in the test
			})

		// Mock repository to return the existing broadcast
		mockRepo.EXPECT().
			GetBroadcastTx(gomock.Any(), gomock.Any(), workspaceID, broadcastID).
			Return(existingBroadcast, nil)

		// Mock repository update to return error
		updateErr := errors.New("failed to update broadcast in database")
		mockRepo.EXPECT().
			UpdateBroadcastTx(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(updateErr)

		// No event calls should be made due to update failure
		mockEventBus.EXPECT().PublishWithAck(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

		// Call the service
		err := service.ResumeBroadcast(ctx, request)

		// Verify results
		require.Error(t, err)
		assert.Equal(t, updateErr, err)
	})

	t.Run("Error_EventProcessingFailed", func(t *testing.T) {
		ctx := context.Background()
		workspaceID := "ws123"
		broadcastID := "bcast123"

		request := &domain.ResumeBroadcastRequest{
			WorkspaceID: workspaceID,
			ID:          broadcastID,
		}

		// Create a paused broadcast that can be resumed
		pausedTime := time.Now().Add(-10 * time.Minute)
		existingBroadcast := &domain.Broadcast{
			ID:          broadcastID,
			WorkspaceID: workspaceID,
			Name:        "Test Broadcast",
			Status:      domain.BroadcastStatusPaused,
			CreatedAt:   time.Now().Add(-1 * time.Hour),
			UpdatedAt:   time.Now().Add(-1 * time.Hour),
			PausedAt:    &pausedTime,
			Schedule: domain.ScheduleSettings{
				IsScheduled: false,
			},
		}

		// Mock authentication
		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
			Return(ctx, &domain.User{ID: "user123"}, nil)

		// Mock the WithTransaction call and execute the provided function with nil
		mockRepo.EXPECT().
			WithTransaction(gomock.Any(), workspaceID, gomock.Any()).
			DoAndReturn(func(ctx context.Context, workspaceID string, fn func(*sql.Tx) error) error {
				return fn(nil) // Pass nil as the transaction, it won't be used in the test
			})

		// Mock repository to return the existing broadcast
		mockRepo.EXPECT().
			GetBroadcastTx(gomock.Any(), gomock.Any(), workspaceID, broadcastID).
			Return(existingBroadcast, nil)

		// Mock repository update to succeed
		mockRepo.EXPECT().
			UpdateBroadcastTx(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil)

		// Mock event bus to simulate failed event processing
		eventErr := errors.New("event processing failed")
		mockEventBus.EXPECT().
			PublishWithAck(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, payload domain.EventPayload, callback domain.EventAckCallback) {
				// Call the callback with error
				callback(eventErr)
			})

		// Call the service
		err := service.ResumeBroadcast(ctx, request)

		// Verify results
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to process resume event")
	})

	t.Run("Error_ContextCancelled", func(t *testing.T) {
		// Create a cancelled context
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		workspaceID := "ws123"
		broadcastID := "bcast123"

		request := &domain.ResumeBroadcastRequest{
			WorkspaceID: workspaceID,
			ID:          broadcastID,
		}

		// Create a paused broadcast that can be resumed
		pausedTime := time.Now().Add(-10 * time.Minute)
		existingBroadcast := &domain.Broadcast{
			ID:          broadcastID,
			WorkspaceID: workspaceID,
			Name:        "Test Broadcast",
			Status:      domain.BroadcastStatusPaused,
			CreatedAt:   time.Now().Add(-1 * time.Hour),
			UpdatedAt:   time.Now().Add(-1 * time.Hour),
			PausedAt:    &pausedTime,
			Schedule: domain.ScheduleSettings{
				IsScheduled: false,
			},
		}

		// Mock authentication
		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
			Return(ctx, &domain.User{ID: "user123"}, nil)

		// Mock the WithTransaction call and execute the provided function with nil
		mockRepo.EXPECT().
			WithTransaction(gomock.Any(), workspaceID, gomock.Any()).
			DoAndReturn(func(ctx context.Context, workspaceID string, fn func(*sql.Tx) error) error {
				return fn(nil) // Pass nil as the transaction, it won't be used in the test
			})

		// Mock repository to return the existing broadcast
		mockRepo.EXPECT().
			GetBroadcastTx(gomock.Any(), gomock.Any(), workspaceID, broadcastID).
			Return(existingBroadcast, nil)

		// Mock repository update to succeed
		mockRepo.EXPECT().
			UpdateBroadcastTx(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil)

		// Mock event bus but don't call the callback (simulate hanging event processing)
		mockEventBus.EXPECT().
			PublishWithAck(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, payload domain.EventPayload, callback domain.EventAckCallback) {
				// Don't call the callback, let context cancellation handle it
			})

		// Call the service
		err := service.ResumeBroadcast(ctx, request)

		// Verify results
		require.Error(t, err)
		assert.Equal(t, context.Canceled, err)
	})
}
