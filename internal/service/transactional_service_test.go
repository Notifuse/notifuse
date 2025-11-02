package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/Notifuse/notifuse/pkg/logger"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
	"github.com/Notifuse/notifuse/pkg/notifuse_mjml"
	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockEmailService is a simplified mock for testing
type mockEmailService struct {
	logger logger.Logger
}

func (m *mockEmailService) SendEmail(ctx context.Context, request domain.SendEmailProviderRequest, isMarketing bool) error {
	return nil
}

func (m *mockEmailService) TestEmailProvider(ctx context.Context, workspaceID string, provider domain.EmailProvider, to string) error {
	return nil
}

func (m *mockEmailService) TestTemplate(ctx context.Context, workspaceID string, templateID string, integrationID string, recipientEmail string, cc []string, bcc []string, replyTo string) error {
	return nil
}

func TestTransactionalNotificationService_CreateNotification(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockTransactionalNotificationRepository(ctrl)
	mockMsgHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
	mockTemplateService := mocks.NewMockTemplateService(ctrl)
	mockContactService := mocks.NewMockContactService(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)

	// Create a stub logger that simply returns itself for chaining calls
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

	type testCase struct {
		name           string
		input          domain.TransactionalNotificationCreateParams
		mockSetup      func()
		expectedError  bool
		expectedResult *domain.TransactionalNotification
	}

	ctx := context.Background()
	workspace := "test-workspace"
	templateID := uuid.New().String()

	tests := []testCase{
		{
			name: "Success_CreateNotification",
			input: domain.TransactionalNotificationCreateParams{
				ID:          uuid.New().String(),
				Name:        "Test Notification",
				Description: "This is a test notification",
				Channels: map[domain.TransactionalChannel]domain.ChannelTemplate{
					domain.TransactionalChannelEmail: {
						TemplateID: templateID,
					},
				},
				Metadata: map[string]interface{}{
					"key": "value",
				},
			},
			mockSetup: func() {
				// Expect auth service to authenticate the user
				mockAuthService.EXPECT().
					AuthenticateUserForWorkspace(gomock.Any(), workspace).
					Return(ctx, &domain.User{ID: "user-123"}, &domain.UserWorkspace{
						UserID:      "user-123",
						WorkspaceID: workspace,
						Role:        "member",
						Permissions: domain.UserPermissions{
							domain.PermissionResourceTransactional: {Read: true, Write: true},
						},
					}, nil)

				// Expect template service to validate the template exists
				mockTemplateService.EXPECT().
					GetTemplateByID(gomock.Any(), workspace, templateID, int64(0)).
					Return(&domain.Template{ID: templateID}, nil)

				// Expect repo to create notification
				mockRepo.EXPECT().
					Create(gomock.Any(), workspace, gomock.Any()).
					DoAndReturn(func(_ context.Context, _ string, notif *domain.TransactionalNotification) error {
						assert.Equal(t, "Test Notification", notif.Name)
						return nil
					})
			},
			expectedError: false,
			expectedResult: &domain.TransactionalNotification{
				ID:          gomock.Any().String(),
				Name:        "Test Notification",
				Description: "This is a test notification",
				Channels: map[domain.TransactionalChannel]domain.ChannelTemplate{
					domain.TransactionalChannelEmail: {
						TemplateID: templateID,
					},
				},
				Metadata: map[string]interface{}{
					"key": "value",
				},
			},
		},
		{
			name: "Error_TemplateNotFound",
			input: domain.TransactionalNotificationCreateParams{
				ID:   uuid.New().String(),
				Name: "Test Notification",
				Channels: map[domain.TransactionalChannel]domain.ChannelTemplate{
					domain.TransactionalChannelEmail: {
						TemplateID: templateID,
					},
				},
			},
			mockSetup: func() {
				// Expect auth service to authenticate the user
				mockAuthService.EXPECT().
					AuthenticateUserForWorkspace(gomock.Any(), workspace).
					Return(ctx, &domain.User{ID: "user-123"}, &domain.UserWorkspace{
						UserID:      "user-123",
						WorkspaceID: workspace,
						Role:        "member",
						Permissions: domain.UserPermissions{
							domain.PermissionResourceTransactional: {Read: true, Write: true},
						},
					}, nil)

				// Expect template service to fail finding the template
				mockTemplateService.EXPECT().
					GetTemplateByID(gomock.Any(), workspace, templateID, int64(0)).
					Return(nil, errors.New("template not found"))
			},
			expectedError:  true,
			expectedResult: nil,
		},
		{
			name: "Error_RepositoryCreateFailed",
			input: domain.TransactionalNotificationCreateParams{
				ID:   uuid.New().String(),
				Name: "Test Notification",
				Channels: map[domain.TransactionalChannel]domain.ChannelTemplate{
					domain.TransactionalChannelEmail: {
						TemplateID: templateID,
					},
				},
			},
			mockSetup: func() {
				// Expect auth service to authenticate the user
				mockAuthService.EXPECT().
					AuthenticateUserForWorkspace(gomock.Any(), workspace).
					Return(ctx, &domain.User{ID: "user-123"}, &domain.UserWorkspace{
						UserID:      "user-123",
						WorkspaceID: workspace,
						Role:        "member",
						Permissions: domain.UserPermissions{
							domain.PermissionResourceTransactional: {Read: true, Write: true},
						},
					}, nil)

				// Template exists
				mockTemplateService.EXPECT().
					GetTemplateByID(gomock.Any(), workspace, templateID, int64(0)).
					Return(&domain.Template{ID: templateID}, nil)

				// But repo create fails
				mockRepo.EXPECT().
					Create(gomock.Any(), workspace, gomock.Any()).
					Return(errors.New("repository error"))
			},
			expectedError:  true,
			expectedResult: nil,
		},
		{
			name: "Error_AuthenticationFailed",
			input: domain.TransactionalNotificationCreateParams{
				ID:   uuid.New().String(),
				Name: "Test Notification",
			},
			mockSetup: func() {
				// Auth service fails
				mockAuthService.EXPECT().
					AuthenticateUserForWorkspace(gomock.Any(), workspace).
					Return(ctx, nil, nil, errors.New("authentication failed"))
			},
			expectedError:  true,
			expectedResult: nil,
		},
		{
			name: "Error_InsufficientPermissions",
			input: domain.TransactionalNotificationCreateParams{
				ID:   uuid.New().String(),
				Name: "Test Notification",
			},
			mockSetup: func() {
				// Auth succeeds but user has no write permissions
				mockAuthService.EXPECT().
					AuthenticateUserForWorkspace(gomock.Any(), workspace).
					Return(ctx, &domain.User{ID: "user-123"}, &domain.UserWorkspace{
						UserID:      "user-123",
						WorkspaceID: workspace,
						Role:        "viewer",
						Permissions: domain.UserPermissions{
							domain.PermissionResourceTransactional: {Read: true, Write: false},
						},
					}, nil)
			},
			expectedError:  true,
			expectedResult: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Set up mocks for this test case
			tc.mockSetup()

			// Create service with mocked dependencies
			service := &TransactionalNotificationService{
				transactionalRepo:  mockRepo,
				messageHistoryRepo: mockMsgHistoryRepo,
				templateService:    mockTemplateService,
				contactService:     mockContactService,
				emailService:       nil, // Not used in this test
				logger:             mockLogger,
				workspaceRepo:      mockWorkspaceRepo,
				apiEndpoint:        "https://api.example.com",
				authService:        mockAuthService,
			}

			// Call the method being tested
			result, err := service.CreateNotification(ctx, workspace, tc.input)

			// Check results
			if tc.expectedError {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, tc.input.Name, result.Name)
				assert.Equal(t, tc.input.Description, result.Description)
				assert.Equal(t, tc.input.Channels, result.Channels)
				assert.Equal(t, tc.input.Metadata, result.Metadata)
			}
		})
	}
}

func TestTransactionalNotificationService_UpdateNotification(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockTransactionalNotificationRepository(ctrl)
	mockMsgHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
	mockTemplateService := mocks.NewMockTemplateService(ctrl)
	mockContactService := mocks.NewMockContactService(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)

	// Create a stub logger that simply returns itself for chaining calls
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

	type testCase struct {
		name           string
		id             string
		input          domain.TransactionalNotificationUpdateParams
		mockSetup      func()
		expectedError  bool
		expectedResult *domain.TransactionalNotification
	}

	ctx := context.Background()
	workspace := "test-workspace"
	notificationID := uuid.New().String()
	templateID := uuid.New().String()
	newTemplateID := uuid.New().String()

	existingNotification := &domain.TransactionalNotification{
		ID:          notificationID,
		Name:        "Original Name",
		Description: "Original Description",
		Channels: map[domain.TransactionalChannel]domain.ChannelTemplate{
			domain.TransactionalChannelEmail: {
				TemplateID: templateID,
			},
		},
		Metadata: map[string]interface{}{
			"original": "value",
		},
	}

	tests := []testCase{
		{
			name: "Success_UpdateName",
			id:   notificationID,
			input: domain.TransactionalNotificationUpdateParams{
				Name: "Updated Name",
			},
			mockSetup: func() {
				// Expect auth service to authenticate the user
				mockAuthService.EXPECT().
					AuthenticateUserForWorkspace(gomock.Any(), workspace).
					Return(ctx, &domain.User{ID: "user-123"}, &domain.UserWorkspace{
						UserID:      "user-123",
						WorkspaceID: workspace,
						Role:        "member",
						Permissions: domain.UserPermissions{
							domain.PermissionResourceTransactional: {Read: true, Write: true},
						},
					}, nil)

				// Get existing notification
				mockRepo.EXPECT().
					Get(gomock.Any(), workspace, notificationID).
					Return(existingNotification, nil)

				// Update notification
				mockRepo.EXPECT().
					Update(gomock.Any(), workspace, gomock.Any()).
					DoAndReturn(func(_ context.Context, _ string, notif *domain.TransactionalNotification) error {
						assert.Equal(t, "Updated Name", notif.Name)
						assert.Equal(t, existingNotification.Description, notif.Description)
						assert.Equal(t, existingNotification.Channels, notif.Channels)
						assert.Equal(t, existingNotification.Metadata, notif.Metadata)
						return nil
					})
			},
			expectedError: false,
			expectedResult: &domain.TransactionalNotification{
				ID:          notificationID,
				Name:        "Updated Name",
				Description: "Original Description",
				Channels: map[domain.TransactionalChannel]domain.ChannelTemplate{
					domain.TransactionalChannelEmail: {
						TemplateID: templateID,
					},
				},
				Metadata: map[string]interface{}{
					"original": "value",
				},
			},
		},
		{
			name: "Success_UpdateAllFields",
			id:   notificationID,
			input: domain.TransactionalNotificationUpdateParams{
				Name:        "Completely Updated",
				Description: "New Description",
				Channels: map[domain.TransactionalChannel]domain.ChannelTemplate{
					domain.TransactionalChannelEmail: {
						TemplateID: newTemplateID,
					},
				},
				Metadata: map[string]interface{}{
					"new": "metadata",
				},
			},
			mockSetup: func() {
				// Expect auth service to authenticate the user
				mockAuthService.EXPECT().
					AuthenticateUserForWorkspace(gomock.Any(), workspace).
					Return(ctx, &domain.User{ID: "user-123"}, &domain.UserWorkspace{
						UserID:      "user-123",
						WorkspaceID: workspace,
						Role:        "member",
						Permissions: domain.UserPermissions{
							domain.PermissionResourceTransactional: {Read: true, Write: true},
						},
					}, nil)

				// Get existing notification
				mockRepo.EXPECT().
					Get(gomock.Any(), workspace, notificationID).
					Return(existingNotification, nil)

				// Expect template service to validate the template exists
				mockTemplateService.EXPECT().
					GetTemplateByID(gomock.Any(), workspace, newTemplateID, int64(0)).
					Return(&domain.Template{ID: newTemplateID}, nil)

				// Update notification
				mockRepo.EXPECT().
					Update(gomock.Any(), workspace, gomock.Any()).
					DoAndReturn(func(_ context.Context, _ string, notif *domain.TransactionalNotification) error {
						assert.Equal(t, "Completely Updated", notif.Name)
						assert.Equal(t, "New Description", notif.Description)
						assert.Equal(t, newTemplateID, notif.Channels[domain.TransactionalChannelEmail].TemplateID)
						assert.Equal(t, "metadata", notif.Metadata["new"])
						return nil
					})
			},
			expectedError: false,
			expectedResult: &domain.TransactionalNotification{
				ID:          notificationID,
				Name:        "Completely Updated",
				Description: "New Description",
				Channels: map[domain.TransactionalChannel]domain.ChannelTemplate{
					domain.TransactionalChannelEmail: {
						TemplateID: newTemplateID,
					},
				},
				Metadata: map[string]interface{}{
					"new": "metadata",
				},
			},
		},
		{
			name: "Error_NotificationNotFound",
			id:   notificationID,
			input: domain.TransactionalNotificationUpdateParams{
				Name: "Updated Name",
			},
			mockSetup: func() {
				// Expect auth service to authenticate the user
				mockAuthService.EXPECT().
					AuthenticateUserForWorkspace(gomock.Any(), workspace).
					Return(ctx, &domain.User{ID: "user-123"}, &domain.UserWorkspace{
						UserID:      "user-123",
						WorkspaceID: workspace,
						Role:        "member",
						Permissions: domain.UserPermissions{
							domain.PermissionResourceTransactional: {Read: true, Write: true},
						},
					}, nil)

				// Get existing notification fails
				mockRepo.EXPECT().
					Get(gomock.Any(), workspace, notificationID).
					Return(nil, errors.New("notification not found"))
			},
			expectedError:  true,
			expectedResult: nil,
		},
		{
			name: "Error_TemplateNotFound",
			id:   notificationID,
			input: domain.TransactionalNotificationUpdateParams{
				Channels: map[domain.TransactionalChannel]domain.ChannelTemplate{
					domain.TransactionalChannelEmail: {
						TemplateID: newTemplateID,
					},
				},
			},
			mockSetup: func() {
				// Expect auth service to authenticate the user
				mockAuthService.EXPECT().
					AuthenticateUserForWorkspace(gomock.Any(), workspace).
					Return(ctx, &domain.User{ID: "user-123"}, &domain.UserWorkspace{
						UserID:      "user-123",
						WorkspaceID: workspace,
						Role:        "member",
						Permissions: domain.UserPermissions{
							domain.PermissionResourceTransactional: {Read: true, Write: true},
						},
					}, nil)

				// Get existing notification
				mockRepo.EXPECT().
					Get(gomock.Any(), workspace, notificationID).
					Return(existingNotification, nil)

				// Template validation fails
				mockTemplateService.EXPECT().
					GetTemplateByID(gomock.Any(), workspace, newTemplateID, int64(0)).
					Return(nil, errors.New("template not found"))
			},
			expectedError:  true,
			expectedResult: nil,
		},
		{
			name: "Error_UpdateFailed",
			id:   notificationID,
			input: domain.TransactionalNotificationUpdateParams{
				Name: "Updated Name",
			},
			mockSetup: func() {
				// Expect auth service to authenticate the user
				mockAuthService.EXPECT().
					AuthenticateUserForWorkspace(gomock.Any(), workspace).
					Return(ctx, &domain.User{ID: "user-123"}, &domain.UserWorkspace{
						UserID:      "user-123",
						WorkspaceID: workspace,
						Role:        "member",
						Permissions: domain.UserPermissions{
							domain.PermissionResourceTransactional: {Read: true, Write: true},
						},
					}, nil)

				// Get existing notification
				mockRepo.EXPECT().
					Get(gomock.Any(), workspace, notificationID).
					Return(existingNotification, nil)

				// Update notification fails
				mockRepo.EXPECT().
					Update(gomock.Any(), workspace, gomock.Any()).
					Return(errors.New("update failed"))
			},
			expectedError:  true,
			expectedResult: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Set up mocks for this test case
			tc.mockSetup()

			// Create service with mocked dependencies
			service := &TransactionalNotificationService{
				transactionalRepo:  mockRepo,
				messageHistoryRepo: mockMsgHistoryRepo,
				templateService:    mockTemplateService,
				contactService:     mockContactService,
				emailService:       nil, // Not used in this test
				logger:             mockLogger,
				workspaceRepo:      mockWorkspaceRepo,
				apiEndpoint:        "https://api.example.com",
				authService:        mockAuthService,
			}

			// Call the method being tested
			result, err := service.UpdateNotification(ctx, workspace, tc.id, tc.input)

			// Check results
			if tc.expectedError {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				if tc.input.Name != "" {
					assert.Equal(t, tc.input.Name, result.Name)
				}
				if tc.input.Description != "" {
					assert.Equal(t, tc.input.Description, result.Description)
				}
				if tc.input.Channels != nil {
					assert.Equal(t, tc.input.Channels, result.Channels)
				}
				if tc.input.Metadata != nil {
					assert.Equal(t, tc.input.Metadata, result.Metadata)
				}
			}
		})
	}
}

func TestTransactionalNotificationService_GetNotification(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockTransactionalNotificationRepository(ctrl)
	mockMsgHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
	mockTemplateService := mocks.NewMockTemplateService(ctrl)
	mockContactService := mocks.NewMockContactService(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)

	// Create a stub logger that simply returns itself for chaining calls
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

	type testCase struct {
		name           string
		id             string
		mockSetup      func()
		expectedError  bool
		expectedResult *domain.TransactionalNotification
	}

	ctx := context.Background()
	workspace := "test-workspace"
	notificationID := uuid.New().String()

	existingNotification := &domain.TransactionalNotification{
		ID:          notificationID,
		Name:        "Test Notification",
		Description: "Test Description",
	}

	tests := []testCase{
		{
			name: "Success_GetNotification",
			id:   notificationID,
			mockSetup: func() {
				// Expect auth service to authenticate the user
				mockAuthService.EXPECT().
					AuthenticateUserForWorkspace(gomock.Any(), workspace).
					Return(ctx, &domain.User{ID: "user-123"}, &domain.UserWorkspace{
						UserID:      "user-123",
						WorkspaceID: workspace,
						Role:        "member",
						Permissions: domain.UserPermissions{
							domain.PermissionResourceTransactional: {Read: true, Write: true},
						},
					}, nil)

				mockRepo.EXPECT().
					Get(gomock.Any(), workspace, notificationID).
					Return(existingNotification, nil)
			},
			expectedError:  false,
			expectedResult: existingNotification,
		},
		{
			name: "Error_NotificationNotFound",
			id:   notificationID,
			mockSetup: func() {
				// Expect auth service to authenticate the user
				mockAuthService.EXPECT().
					AuthenticateUserForWorkspace(gomock.Any(), workspace).
					Return(ctx, &domain.User{ID: "user-123"}, &domain.UserWorkspace{
						UserID:      "user-123",
						WorkspaceID: workspace,
						Role:        "member",
						Permissions: domain.UserPermissions{
							domain.PermissionResourceTransactional: {Read: true, Write: true},
						},
					}, nil)

				mockRepo.EXPECT().
					Get(gomock.Any(), workspace, notificationID).
					Return(nil, errors.New("notification not found"))
			},
			expectedError:  true,
			expectedResult: nil,
		},
		{
			name: "Error_AuthenticationFailed",
			id:   notificationID,
			mockSetup: func() {
				// Auth service fails
				mockAuthService.EXPECT().
					AuthenticateUserForWorkspace(gomock.Any(), workspace).
					Return(ctx, nil, nil, errors.New("authentication failed"))
			},
			expectedError:  true,
			expectedResult: nil,
		},
		{
			name: "Error_InsufficientPermissions",
			id:   notificationID,
			mockSetup: func() {
				// Auth succeeds but user has no read permissions
				mockAuthService.EXPECT().
					AuthenticateUserForWorkspace(gomock.Any(), workspace).
					Return(ctx, &domain.User{ID: "user-123"}, &domain.UserWorkspace{
						UserID:      "user-123",
						WorkspaceID: workspace,
						Role:        "viewer",
						Permissions: domain.UserPermissions{
							domain.PermissionResourceTransactional: {Read: false, Write: false},
						},
					}, nil)
			},
			expectedError:  true,
			expectedResult: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Set up mocks for this test case
			tc.mockSetup()

			// Create service with mocked dependencies
			service := &TransactionalNotificationService{
				transactionalRepo:  mockRepo,
				messageHistoryRepo: mockMsgHistoryRepo,
				templateService:    mockTemplateService,
				contactService:     mockContactService,
				emailService:       nil, // Not used in this test
				logger:             mockLogger,
				workspaceRepo:      mockWorkspaceRepo,
				apiEndpoint:        "https://api.example.com",
				authService:        mockAuthService,
			}

			// Call the method being tested
			result, err := service.GetNotification(ctx, workspace, tc.id)

			// Check results
			if tc.expectedError {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expectedResult, result)
			}
		})
	}
}

func TestTransactionalNotificationService_ListNotifications(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockTransactionalNotificationRepository(ctrl)
	mockMsgHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
	mockTemplateService := mocks.NewMockTemplateService(ctrl)
	mockContactService := mocks.NewMockContactService(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)

	// Create a stub logger that simply returns itself for chaining calls
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

	type testCase struct {
		name              string
		filter            map[string]interface{}
		limit             int
		offset            int
		mockSetup         func()
		expectedError     bool
		expectedResults   []*domain.TransactionalNotification
		expectedTotalRows int
	}

	ctx := context.Background()
	workspace := "test-workspace"

	notifications := []*domain.TransactionalNotification{
		{
			ID:   uuid.New().String(),
			Name: "Notification 1",
		},
		{
			ID:   uuid.New().String(),
			Name: "Notification 2",
		},
	}

	tests := []testCase{
		{
			name:   "Success_ListNotifications",
			filter: map[string]interface{}{"name": "Test"},
			limit:  10,
			offset: 0,
			mockSetup: func() {
				// Expect auth service to authenticate the user
				mockAuthService.EXPECT().
					AuthenticateUserForWorkspace(gomock.Any(), workspace).
					Return(ctx, &domain.User{ID: "user-123"}, &domain.UserWorkspace{
						UserID:      "user-123",
						WorkspaceID: workspace,
						Role:        "member",
						Permissions: domain.UserPermissions{
							domain.PermissionResourceTransactional: {Read: true, Write: true},
						},
					}, nil)

				mockRepo.EXPECT().
					List(gomock.Any(), workspace, gomock.Any(), gomock.Any(), gomock.Any()).
					Return(notifications, 2, nil)
			},
			expectedError:     false,
			expectedResults:   notifications,
			expectedTotalRows: 2,
		},
		{
			name:   "Success_EmptyResults",
			filter: map[string]interface{}{"name": "NonExistent"},
			limit:  10,
			offset: 0,
			mockSetup: func() {
				// Expect auth service to authenticate the user
				mockAuthService.EXPECT().
					AuthenticateUserForWorkspace(gomock.Any(), workspace).
					Return(ctx, &domain.User{ID: "user-123"}, &domain.UserWorkspace{
						UserID:      "user-123",
						WorkspaceID: workspace,
						Role:        "member",
						Permissions: domain.UserPermissions{
							domain.PermissionResourceTransactional: {Read: true, Write: true},
						},
					}, nil)

				mockRepo.EXPECT().
					List(gomock.Any(), workspace, gomock.Any(), gomock.Any(), gomock.Any()).
					Return([]*domain.TransactionalNotification{}, 0, nil)
			},
			expectedError:     false,
			expectedResults:   []*domain.TransactionalNotification{},
			expectedTotalRows: 0,
		},
		{
			name:   "Error_RepositoryListFailed",
			filter: map[string]interface{}{},
			limit:  10,
			offset: 0,
			mockSetup: func() {
				// Expect auth service to authenticate the user
				mockAuthService.EXPECT().
					AuthenticateUserForWorkspace(gomock.Any(), workspace).
					Return(ctx, &domain.User{ID: "user-123"}, &domain.UserWorkspace{
						UserID:      "user-123",
						WorkspaceID: workspace,
						Role:        "member",
						Permissions: domain.UserPermissions{
							domain.PermissionResourceTransactional: {Read: true, Write: true},
						},
					}, nil)

				mockRepo.EXPECT().
					List(gomock.Any(), workspace, gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, 0, errors.New("repository error"))
			},
			expectedError:     true,
			expectedResults:   nil,
			expectedTotalRows: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Set up mocks for this test case
			tc.mockSetup()

			// Create service with mocked dependencies
			service := &TransactionalNotificationService{
				transactionalRepo:  mockRepo,
				messageHistoryRepo: mockMsgHistoryRepo,
				templateService:    mockTemplateService,
				contactService:     mockContactService,
				emailService:       nil, // Not used in this test
				logger:             mockLogger,
				workspaceRepo:      mockWorkspaceRepo,
				apiEndpoint:        "https://api.example.com",
				authService:        mockAuthService,
			}

			// Call the method being tested
			results, total, err := service.ListNotifications(ctx, workspace, tc.filter, tc.limit, tc.offset)

			// Check results
			if tc.expectedError {
				assert.Error(t, err)
				assert.Nil(t, results)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expectedResults, results)
				assert.Equal(t, tc.expectedTotalRows, total)
			}
		})
	}
}

func TestTransactionalNotificationService_DeleteNotification(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockTransactionalNotificationRepository(ctrl)
	mockMsgHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
	mockTemplateService := mocks.NewMockTemplateService(ctrl)
	mockContactService := mocks.NewMockContactService(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)

	// Create a stub logger that simply returns itself for chaining calls
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

	type testCase struct {
		name          string
		id            string
		mockSetup     func()
		expectedError bool
	}

	ctx := context.Background()
	workspace := "test-workspace"
	notificationID := uuid.New().String()

	existingNotification := &domain.TransactionalNotification{
		ID:          notificationID,
		Name:        "Test Notification",
		Description: "Test Description",
	}

	tests := []testCase{
		{
			name: "Success_DeleteNotification",
			id:   notificationID,
			mockSetup: func() {
				// Expect auth service to authenticate the user
				mockAuthService.EXPECT().
					AuthenticateUserForWorkspace(gomock.Any(), workspace).
					Return(ctx, &domain.User{ID: "user-123"}, &domain.UserWorkspace{
						UserID:      "user-123",
						WorkspaceID: workspace,
						Role:        "member",
						Permissions: domain.UserPermissions{
							domain.PermissionResourceTransactional: {Read: true, Write: true},
						},
					}, nil)

				// Get the notification first to check if it's integration-managed
				mockRepo.EXPECT().
					Get(gomock.Any(), workspace, notificationID).
					Return(existingNotification, nil)

				mockRepo.EXPECT().
					Delete(gomock.Any(), workspace, notificationID).
					Return(nil)
			},
			expectedError: false,
		},
		{
			name: "Error_DeleteFailed",
			id:   notificationID,
			mockSetup: func() {
				mockAuthService.EXPECT().
					AuthenticateUserForWorkspace(gomock.Any(), workspace).
					Return(ctx, &domain.User{ID: "user-123"}, &domain.UserWorkspace{
						UserID:      "user-123",
						WorkspaceID: workspace,
						Role:        "member",
						Permissions: domain.UserPermissions{
							domain.PermissionResourceTransactional: {Read: true, Write: true},
						},
					}, nil)

				// Get the notification first to check if it's integration-managed
				mockRepo.EXPECT().
					Get(gomock.Any(), workspace, notificationID).
					Return(existingNotification, nil)

				mockRepo.EXPECT().
					Delete(gomock.Any(), workspace, notificationID).
					Return(errors.New("delete failed"))
			},
			expectedError: true,
		},
		{
			name: "Error_NotificationNotFound",
			id:   notificationID,
			mockSetup: func() {
				mockAuthService.EXPECT().
					AuthenticateUserForWorkspace(gomock.Any(), workspace).
					Return(ctx, &domain.User{ID: "user-123"}, &domain.UserWorkspace{
						UserID:      "user-123",
						WorkspaceID: workspace,
						Role:        "member",
						Permissions: domain.UserPermissions{
							domain.PermissionResourceTransactional: {Read: true, Write: true},
						},
					}, nil)

				// Get fails - notification not found
				mockRepo.EXPECT().
					Get(gomock.Any(), workspace, notificationID).
					Return(nil, errors.New("notification not found"))
			},
			expectedError: true,
		},
		{
			name: "Error_IntegrationManagedNotification",
			id:   notificationID,
			mockSetup: func() {
				mockAuthService.EXPECT().
					AuthenticateUserForWorkspace(gomock.Any(), workspace).
					Return(ctx, &domain.User{ID: "user-123"}, &domain.UserWorkspace{
						UserID:      "user-123",
						WorkspaceID: workspace,
						Role:        "member",
						Permissions: domain.UserPermissions{
							domain.PermissionResourceTransactional: {Read: true, Write: true},
						},
					}, nil)

				// Notification is integration-managed
				integrationID := "integration-123"
				integrationManagedNotification := &domain.TransactionalNotification{
					ID:            notificationID,
					Name:          "Integration Managed Notification",
					IntegrationID: &integrationID,
				}
				mockRepo.EXPECT().
					Get(gomock.Any(), workspace, notificationID).
					Return(integrationManagedNotification, nil)
				// Delete should NOT be called for integration-managed notifications
			},
			expectedError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Set up mocks for this test case
			tc.mockSetup()

			// Create service with mocked dependencies
			service := &TransactionalNotificationService{
				transactionalRepo:  mockRepo,
				messageHistoryRepo: mockMsgHistoryRepo,
				templateService:    mockTemplateService,
				contactService:     mockContactService,
				emailService:       nil, // Not used in this test
				logger:             mockLogger,
				workspaceRepo:      mockWorkspaceRepo,
				apiEndpoint:        "https://api.example.com",
				authService:        mockAuthService,
			}

			// Call the method being tested
			err := service.DeleteNotification(ctx, workspace, tc.id)

			// Check results
			if tc.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestNewTransactionalNotificationService(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockTransactionalNotificationRepository(ctrl)
	mockMsgHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
	mockTemplateService := mocks.NewMockTemplateService(ctrl)
	mockContactService := mocks.NewMockContactService(ctrl)
	mockEmailService := mocks.NewMockEmailServiceInterface(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	apiEndpoint := "https://api.example.com"

	service := NewTransactionalNotificationService(
		mockRepo,
		mockMsgHistoryRepo,
		mockTemplateService,
		mockContactService,
		mockEmailService,
		mockAuthService,
		mockLogger,
		mockWorkspaceRepo,
		apiEndpoint,
	)

	assert.NotNil(t, service)
	assert.Equal(t, mockRepo, service.transactionalRepo)
	assert.Equal(t, mockMsgHistoryRepo, service.messageHistoryRepo)
	assert.Equal(t, mockTemplateService, service.templateService)
	assert.Equal(t, mockContactService, service.contactService)
	assert.Equal(t, mockEmailService, service.emailService)
	assert.Equal(t, mockAuthService, service.authService)
	assert.Equal(t, mockLogger, service.logger)
	assert.Equal(t, mockWorkspaceRepo, service.workspaceRepo)
	assert.Equal(t, apiEndpoint, service.apiEndpoint)
}

func TestTransactionalNotificationService_SendNotification(t *testing.T) {
	// Common test data (not controller-dependent)
	ctx := context.Background()
	workspace := "test-workspace"
	notificationID := uuid.New().String()
	templateID := uuid.New().String()

	// Create a sample notification and contact for tests
	notification := &domain.TransactionalNotification{
		ID:          notificationID,
		Name:        "Test Notification",
		Description: "Test Description",
		Channels: map[domain.TransactionalChannel]domain.ChannelTemplate{
			domain.TransactionalChannelEmail: {
				TemplateID: templateID,
			},
		},
	}

	workspaceObj := &domain.Workspace{
		ID:   workspace,
		Name: "Test Workspace",
		Settings: domain.WorkspaceSettings{
			TransactionalEmailProviderID: "integration-1",
			SecretKey:                    "test-secret-key",
		},
		Integrations: []domain.Integration{
			{
				ID:   "integration-1",
				Name: "Test Integration",
				Type: "email",
				EmailProvider: domain.EmailProvider{
					Kind: domain.EmailProviderKindSparkPost,
					Senders: []domain.EmailSender{
						domain.NewEmailSender("test@example.com", "Test Sender"),
					},
					SparkPost: &domain.SparkPostSettings{
						EncryptedAPIKey: "encrypted-api-key",
					},
				},
			},
		},
	}

	contact := &domain.Contact{
		Email: "test@example.com",
		FirstName: &domain.NullableString{
			String: "John",
			IsNull: false,
		},
		LastName: &domain.NullableString{
			String: "Doe",
			IsNull: false,
		},
	}

	t.Run("Success_SendNotification", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRepo := mocks.NewMockTransactionalNotificationRepository(ctrl)
		mockMsgHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
		mockTemplateService := mocks.NewMockTemplateService(ctrl)
		mockContactService := mocks.NewMockContactService(ctrl)
		mockEmailService := mocks.NewMockEmailServiceInterface(ctrl)
		mockLogger := pkgmocks.NewMockLogger(ctrl)
		mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
		mockAuthService := mocks.NewMockAuthService(ctrl)

		// Create a stub logger that simply returns itself for chaining calls
		mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
		mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
		mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
		mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
		mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

		service := &TransactionalNotificationService{
			transactionalRepo:  mockRepo,
			messageHistoryRepo: mockMsgHistoryRepo,
			templateService:    mockTemplateService,
			contactService:     mockContactService,
			emailService:       mockEmailService,
			logger:             mockLogger,
			workspaceRepo:      mockWorkspaceRepo,
			apiEndpoint:        "https://api.example.com",
			authService:        mockAuthService,
		}

		params := domain.TransactionalNotificationSendParams{
			ID:      notificationID,
			Contact: contact,
			Data: map[string]interface{}{
				"product_name": "Test Product",
				"order_id":     "12345",
			},
			Metadata: map[string]interface{}{
				"source": "api",
			},
			EmailOptions: domain.EmailOptions{
				CC:  []string{"cc@example.com"},
				BCC: []string{"bcc@example.com"},
			},
		}

		// Expect auth service to authenticate the user
		mockAuthService.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), workspace).
			Return(ctx, &domain.User{ID: "user-123"}, &domain.UserWorkspace{
				UserID:      "user-123",
				WorkspaceID: workspace,
				Role:        "member",
				Permissions: domain.UserPermissions{
					domain.PermissionResourceTransactional: {Read: true, Write: true},
				},
			}, nil)

		// Get the workspace
		mockWorkspaceRepo.EXPECT().
			GetByID(gomock.Any(), workspace).
			Return(workspaceObj, nil)

		// Get the notification
		mockRepo.EXPECT().
			Get(gomock.Any(), workspace, notificationID).
			Return(notification, nil)

		// Upsert the contact
		mockContactService.EXPECT().
			UpsertContact(gomock.Any(), workspace, contact).
			Return(domain.UpsertContactOperation{
				Email:  contact.Email,
				Action: domain.UpsertContactOperationUpdate,
			})

		// Get the contact after upsert
		mockContactService.EXPECT().
			GetContactByEmail(gomock.Any(), workspace, contact.Email).
			Return(contact, nil)

		// Expect call to SendEmailForTemplate with the correct parameters
		mockEmailService.EXPECT().
			SendEmailForTemplate(
				gomock.Any(),
				gomock.Any(), // SendEmailRequest
			).Do(func(_ context.Context, request domain.SendEmailRequest) {
			assert.Equal(t, workspace, request.WorkspaceID)
			assert.Equal(t, contact, request.Contact)
			assert.Equal(t, notification.Channels[domain.TransactionalChannelEmail], request.TemplateConfig)
			assert.NotNil(t, request.EmailProvider)
			assert.Equal(t, workspaceObj.Settings.EmailTrackingEnabled, request.TrackingSettings.EnableTracking)
			assert.Equal(t, "https://api.example.com", request.TrackingSettings.Endpoint)
		}).Return(nil)

		// Message history creation happens inside SendEmailForTemplate

		// Call the method
		messageID, err := service.SendNotification(ctx, workspace, params)

		// Assertions
		require.NoError(t, err)
		require.NotEmpty(t, messageID)
	})

	t.Run("Error_NotificationNotFound", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRepo := mocks.NewMockTransactionalNotificationRepository(ctrl)
		mockMsgHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
		mockTemplateService := mocks.NewMockTemplateService(ctrl)
		mockContactService := mocks.NewMockContactService(ctrl)
		mockEmailService := mocks.NewMockEmailServiceInterface(ctrl)
		mockLogger := pkgmocks.NewMockLogger(ctrl)
		mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
		mockAuthService := mocks.NewMockAuthService(ctrl)

		// Create a stub logger that simply returns itself for chaining calls
		mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
		mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
		mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
		mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
		mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

		service := &TransactionalNotificationService{
			transactionalRepo:  mockRepo,
			messageHistoryRepo: mockMsgHistoryRepo,
			templateService:    mockTemplateService,
			contactService:     mockContactService,
			emailService:       mockEmailService,
			logger:             mockLogger,
			workspaceRepo:      mockWorkspaceRepo,
			apiEndpoint:        "https://api.example.com",
			authService:        mockAuthService,
		}

		params := domain.TransactionalNotificationSendParams{
			ID:      notificationID,
			Contact: contact,
		}

		// Expect auth service to authenticate the user
		mockAuthService.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), workspace).
			Return(ctx, &domain.User{ID: "user-123"}, &domain.UserWorkspace{
				UserID:      "user-123",
				WorkspaceID: workspace,
				Role:        "member",
				Permissions: domain.UserPermissions{
					domain.PermissionResourceTransactional: {Read: true, Write: true},
				},
			}, nil)

		// Get the workspace
		mockWorkspaceRepo.EXPECT().
			GetByID(gomock.Any(), workspace).
			Return(workspaceObj, nil)

		// Notification not found
		mockRepo.EXPECT().
			Get(gomock.Any(), workspace, notificationID).
			Return(nil, errors.New("notification not found"))

		// Call the method
		messageID, err := service.SendNotification(ctx, workspace, params)

		// Assertions
		require.Error(t, err)
		require.Empty(t, messageID)
		assert.Contains(t, err.Error(), "notification not found")
	})

	t.Run("Error_ContactRequired", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRepo := mocks.NewMockTransactionalNotificationRepository(ctrl)
		mockMsgHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
		mockTemplateService := mocks.NewMockTemplateService(ctrl)
		mockContactService := mocks.NewMockContactService(ctrl)
		mockEmailService := mocks.NewMockEmailServiceInterface(ctrl)
		mockLogger := pkgmocks.NewMockLogger(ctrl)
		mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
		mockAuthService := mocks.NewMockAuthService(ctrl)

		// Create a stub logger that simply returns itself for chaining calls
		mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
		mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
		mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
		mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
		mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

		service := &TransactionalNotificationService{
			transactionalRepo:  mockRepo,
			messageHistoryRepo: mockMsgHistoryRepo,
			templateService:    mockTemplateService,
			contactService:     mockContactService,
			emailService:       mockEmailService,
			logger:             mockLogger,
			workspaceRepo:      mockWorkspaceRepo,
			apiEndpoint:        "https://api.example.com",
			authService:        mockAuthService,
		}

		params := domain.TransactionalNotificationSendParams{
			ID:      notificationID,
			Contact: nil, // No contact provided
		}

		// Expect auth service to authenticate the user
		mockAuthService.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), workspace).
			Return(ctx, &domain.User{ID: "user-123"}, &domain.UserWorkspace{
				UserID:      "user-123",
				WorkspaceID: workspace,
				Role:        "member",
				Permissions: domain.UserPermissions{
					domain.PermissionResourceTransactional: {Read: true, Write: true},
				},
			}, nil)

		// Get the workspace
		mockWorkspaceRepo.EXPECT().
			GetByID(gomock.Any(), workspace).
			Return(workspaceObj, nil)

		// Get the notification
		mockRepo.EXPECT().
			Get(gomock.Any(), workspace, notificationID).
			Return(notification, nil)

		// Call the method
		messageID, err := service.SendNotification(ctx, workspace, params)

		// Assertions
		require.Error(t, err)
		require.Empty(t, messageID)
		assert.Contains(t, err.Error(), "contact is required")
	})

	t.Run("Error_WorkspaceNotFound", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRepo := mocks.NewMockTransactionalNotificationRepository(ctrl)
		mockMsgHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
		mockTemplateService := mocks.NewMockTemplateService(ctrl)
		mockContactService := mocks.NewMockContactService(ctrl)
		mockEmailService := mocks.NewMockEmailServiceInterface(ctrl)
		mockLogger := pkgmocks.NewMockLogger(ctrl)
		mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
		mockAuthService := mocks.NewMockAuthService(ctrl)

		// Create a stub logger that simply returns itself for chaining calls
		mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
		mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
		mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
		mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
		mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

		service := &TransactionalNotificationService{
			transactionalRepo:  mockRepo,
			messageHistoryRepo: mockMsgHistoryRepo,
			templateService:    mockTemplateService,
			contactService:     mockContactService,
			emailService:       mockEmailService,
			logger:             mockLogger,
			workspaceRepo:      mockWorkspaceRepo,
			apiEndpoint:        "https://api.example.com",
			authService:        mockAuthService,
		}

		params := domain.TransactionalNotificationSendParams{
			ID:      notificationID,
			Contact: contact,
		}

		// Expect auth service to authenticate the user
		mockAuthService.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), workspace).
			Return(ctx, &domain.User{ID: "user-123"}, &domain.UserWorkspace{
				UserID:      "user-123",
				WorkspaceID: workspace,
				Role:        "member",
				Permissions: domain.UserPermissions{
					domain.PermissionResourceTransactional: {Read: true, Write: true},
				},
			}, nil)

		// Workspace not found
		mockWorkspaceRepo.EXPECT().
			GetByID(gomock.Any(), workspace).
			Return(nil, errors.New("workspace not found"))

		// Call the method
		messageID, err := service.SendNotification(ctx, workspace, params)

		// Assertions
		require.Error(t, err)
		require.Empty(t, messageID)
		assert.Contains(t, err.Error(), "failed to get workspace")
	})

	t.Run("Success_IdempotentRequest", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRepo := mocks.NewMockTransactionalNotificationRepository(ctrl)
		mockMsgHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
		mockTemplateService := mocks.NewMockTemplateService(ctrl)
		mockContactService := mocks.NewMockContactService(ctrl)
		mockEmailService := mocks.NewMockEmailServiceInterface(ctrl)
		mockLogger := pkgmocks.NewMockLogger(ctrl)
		mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
		mockAuthService := mocks.NewMockAuthService(ctrl)

		// Create a stub logger that simply returns itself for chaining calls
		mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
		mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
		mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
		mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
		mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

		service := &TransactionalNotificationService{
			transactionalRepo:  mockRepo,
			messageHistoryRepo: mockMsgHistoryRepo,
			templateService:    mockTemplateService,
			contactService:     mockContactService,
			emailService:       mockEmailService,
			logger:             mockLogger,
			workspaceRepo:      mockWorkspaceRepo,
			apiEndpoint:        "https://api.example.com",
			authService:        mockAuthService,
		}

		externalID := "ext-123"
		existingMessageID := "existing-msg-123"
		params := domain.TransactionalNotificationSendParams{
			ID:         notificationID,
			Contact:    contact,
			ExternalID: &externalID,
		}

		// Expect auth service to authenticate the user
		mockAuthService.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), workspace).
			Return(ctx, &domain.User{ID: "user-123"}, &domain.UserWorkspace{
				UserID:      "user-123",
				WorkspaceID: workspace,
				Role:        "member",
				Permissions: domain.UserPermissions{
					domain.PermissionResourceTransactional: {Read: true, Write: true},
				},
			}, nil)

		// Get the workspace
		mockWorkspaceRepo.EXPECT().
			GetByID(gomock.Any(), workspace).
			Return(workspaceObj, nil)

		// Get the notification
		mockRepo.EXPECT().
			Get(gomock.Any(), workspace, notificationID).
			Return(notification, nil)

		// Contact upsert succeeds
		mockContactService.EXPECT().
			UpsertContact(gomock.Any(), workspace, contact).
			Return(domain.UpsertContactOperation{
				Email:  contact.Email,
				Action: domain.UpsertContactOperationUpdate,
			})

		// Get contact succeeds
		mockContactService.EXPECT().
			GetContactByEmail(gomock.Any(), workspace, contact.Email).
			Return(contact, nil)

		// Message with external ID already exists
		existingMessage := &domain.MessageHistory{
			ID:         existingMessageID,
			ExternalID: &externalID,
		}
		mockMsgHistoryRepo.EXPECT().
			GetByExternalID(gomock.Any(), workspace, externalID).
			Return(existingMessage, nil)

		// Call the method
		messageID, err := service.SendNotification(ctx, workspace, params)

		// Assertions
		require.NoError(t, err)
		require.Equal(t, existingMessageID, messageID)
	})

	t.Run("Error_ContactUpsertFailed", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRepo := mocks.NewMockTransactionalNotificationRepository(ctrl)
		mockMsgHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
		mockTemplateService := mocks.NewMockTemplateService(ctrl)
		mockContactService := mocks.NewMockContactService(ctrl)
		mockEmailService := mocks.NewMockEmailServiceInterface(ctrl)
		mockLogger := pkgmocks.NewMockLogger(ctrl)
		mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
		mockAuthService := mocks.NewMockAuthService(ctrl)

		// Create a stub logger that simply returns itself for chaining calls
		mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
		mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
		mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
		mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
		mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

		service := &TransactionalNotificationService{
			transactionalRepo:  mockRepo,
			messageHistoryRepo: mockMsgHistoryRepo,
			templateService:    mockTemplateService,
			contactService:     mockContactService,
			emailService:       mockEmailService,
			logger:             mockLogger,
			workspaceRepo:      mockWorkspaceRepo,
			apiEndpoint:        "https://api.example.com",
			authService:        mockAuthService,
		}

		params := domain.TransactionalNotificationSendParams{
			ID:      notificationID,
			Contact: contact,
		}

		// Expect auth service to authenticate the user
		mockAuthService.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), workspace).
			Return(ctx, &domain.User{ID: "user-123"}, &domain.UserWorkspace{
				UserID:      "user-123",
				WorkspaceID: workspace,
				Role:        "member",
				Permissions: domain.UserPermissions{
					domain.PermissionResourceTransactional: {Read: true, Write: true},
				},
			}, nil)

		// Get the workspace
		mockWorkspaceRepo.EXPECT().
			GetByID(gomock.Any(), workspace).
			Return(workspaceObj, nil)

		// Get the notification
		mockRepo.EXPECT().
			Get(gomock.Any(), workspace, notificationID).
			Return(notification, nil)

		// Contact upsert fails
		mockContactService.EXPECT().
			UpsertContact(gomock.Any(), workspace, contact).
			Return(domain.UpsertContactOperation{
				Email:  contact.Email,
				Action: domain.UpsertContactOperationError,
				Error:  "database error",
			})

		// Call the method
		messageID, err := service.SendNotification(ctx, workspace, params)

		// Assertions
		require.Error(t, err)
		require.Empty(t, messageID)
		assert.Contains(t, err.Error(), "failed to upsert contact")
	})

	t.Run("Error_ContactNotFoundAfterUpsert", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRepo := mocks.NewMockTransactionalNotificationRepository(ctrl)
		mockMsgHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
		mockTemplateService := mocks.NewMockTemplateService(ctrl)
		mockContactService := mocks.NewMockContactService(ctrl)
		mockEmailService := mocks.NewMockEmailServiceInterface(ctrl)
		mockLogger := pkgmocks.NewMockLogger(ctrl)
		mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
		mockAuthService := mocks.NewMockAuthService(ctrl)

		// Create a stub logger that simply returns itself for chaining calls
		mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
		mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
		mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
		mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
		mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

		service := &TransactionalNotificationService{
			transactionalRepo:  mockRepo,
			messageHistoryRepo: mockMsgHistoryRepo,
			templateService:    mockTemplateService,
			contactService:     mockContactService,
			emailService:       mockEmailService,
			logger:             mockLogger,
			workspaceRepo:      mockWorkspaceRepo,
			apiEndpoint:        "https://api.example.com",
			authService:        mockAuthService,
		}

		params := domain.TransactionalNotificationSendParams{
			ID:      notificationID,
			Contact: contact,
		}

		// Expect auth service to authenticate the user
		mockAuthService.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), workspace).
			Return(ctx, &domain.User{ID: "user-123"}, &domain.UserWorkspace{
				UserID:      "user-123",
				WorkspaceID: workspace,
				Role:        "member",
				Permissions: domain.UserPermissions{
					domain.PermissionResourceTransactional: {Read: true, Write: true},
				},
			}, nil)

		// Get the workspace
		mockWorkspaceRepo.EXPECT().
			GetByID(gomock.Any(), workspace).
			Return(workspaceObj, nil)

		// Get the notification
		mockRepo.EXPECT().
			Get(gomock.Any(), workspace, notificationID).
			Return(notification, nil)

		// Contact upsert succeeds
		mockContactService.EXPECT().
			UpsertContact(gomock.Any(), workspace, contact).
			Return(domain.UpsertContactOperation{
				Email:  contact.Email,
				Action: domain.UpsertContactOperationUpdate,
			})

		// But getting contact fails
		mockContactService.EXPECT().
			GetContactByEmail(gomock.Any(), workspace, contact.Email).
			Return(nil, errors.New("contact not found"))

		// Call the method
		messageID, err := service.SendNotification(ctx, workspace, params)

		// Assertions
		require.Error(t, err)
		require.Empty(t, messageID)
		assert.Contains(t, err.Error(), "contact not found after upsert")
	})

	t.Run("Error_AuthenticationFailed", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRepo := mocks.NewMockTransactionalNotificationRepository(ctrl)
		mockMsgHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
		mockTemplateService := mocks.NewMockTemplateService(ctrl)
		mockContactService := mocks.NewMockContactService(ctrl)
		mockEmailService := mocks.NewMockEmailServiceInterface(ctrl)
		mockLogger := pkgmocks.NewMockLogger(ctrl)
		mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
		mockAuthService := mocks.NewMockAuthService(ctrl)

		// Create a stub logger that simply returns itself for chaining calls
		mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
		mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
		mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
		mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
		mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

		service := &TransactionalNotificationService{
			transactionalRepo:  mockRepo,
			messageHistoryRepo: mockMsgHistoryRepo,
			templateService:    mockTemplateService,
			contactService:     mockContactService,
			emailService:       mockEmailService,
			logger:             mockLogger,
			workspaceRepo:      mockWorkspaceRepo,
			apiEndpoint:        "https://api.example.com",
			authService:        mockAuthService,
		}

		params := domain.TransactionalNotificationSendParams{
			ID:      notificationID,
			Contact: contact,
		}

		// Auth fails
		mockAuthService.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), workspace).
			Return(ctx, nil, nil, errors.New("authentication failed"))

		// Call the method
		messageID, err := service.SendNotification(ctx, workspace, params)

		// Assertions
		require.Error(t, err)
		require.Empty(t, messageID)
		assert.Contains(t, err.Error(), "failed to authenticate user for workspace")
	})

	t.Run("Error_ExternalIDCheckFailed", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRepo := mocks.NewMockTransactionalNotificationRepository(ctrl)
		mockMsgHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
		mockTemplateService := mocks.NewMockTemplateService(ctrl)
		mockContactService := mocks.NewMockContactService(ctrl)
		mockEmailService := mocks.NewMockEmailServiceInterface(ctrl)
		mockLogger := pkgmocks.NewMockLogger(ctrl)
		mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
		mockAuthService := mocks.NewMockAuthService(ctrl)

		// Create a stub logger that simply returns itself for chaining calls
		mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
		mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
		mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
		mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
		mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

		service := &TransactionalNotificationService{
			transactionalRepo:  mockRepo,
			messageHistoryRepo: mockMsgHistoryRepo,
			templateService:    mockTemplateService,
			contactService:     mockContactService,
			emailService:       mockEmailService,
			logger:             mockLogger,
			workspaceRepo:      mockWorkspaceRepo,
			apiEndpoint:        "https://api.example.com",
			authService:        mockAuthService,
		}

		externalID := "ext-123"
		params := domain.TransactionalNotificationSendParams{
			ID:         notificationID,
			Contact:    contact,
			ExternalID: &externalID,
		}

		// Expect auth service to authenticate the user
		mockAuthService.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), workspace).
			Return(ctx, &domain.User{ID: "user-123"}, &domain.UserWorkspace{
				UserID:      "user-123",
				WorkspaceID: workspace,
				Role:        "member",
				Permissions: domain.UserPermissions{
					domain.PermissionResourceTransactional: {Read: true, Write: true},
				},
			}, nil)

		// Get the workspace
		mockWorkspaceRepo.EXPECT().
			GetByID(gomock.Any(), workspace).
			Return(workspaceObj, nil)

		// Get the notification
		mockRepo.EXPECT().
			Get(gomock.Any(), workspace, notificationID).
			Return(notification, nil)

		// Contact upsert succeeds
		mockContactService.EXPECT().
			UpsertContact(gomock.Any(), workspace, contact).
			Return(domain.UpsertContactOperation{
				Email:  contact.Email,
				Action: domain.UpsertContactOperationUpdate,
			})

		// Get contact succeeds
		mockContactService.EXPECT().
			GetContactByEmail(gomock.Any(), workspace, contact.Email).
			Return(contact, nil)

		// External ID check fails with a real database error (not "not found")
		mockMsgHistoryRepo.EXPECT().
			GetByExternalID(gomock.Any(), workspace, externalID).
			Return(nil, errors.New("database connection failed"))

		// Call the method
		messageID, err := service.SendNotification(ctx, workspace, params)

		// Assertions
		require.Error(t, err)
		require.Empty(t, messageID)
		assert.Contains(t, err.Error(), "failed to check for existing message")
	})
}

func TestTransactionalNotificationService_TestTemplate(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockTransactionalNotificationRepository(ctrl)
	mockMsgHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
	mockTemplateService := mocks.NewMockTemplateService(ctrl)
	mockContactService := mocks.NewMockContactService(ctrl)
	mockEmailService := mocks.NewMockEmailServiceInterface(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)

	// Create a stub logger that simply returns itself for chaining calls
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

	ctx := context.Background()
	workspaceID := "test-workspace"
	templateID := uuid.New().String()
	integrationID := "integration-1"
	senderID := "sender-1"
	recipientEmail := "test@example.com"

	// Setup workspace
	workspace := &domain.Workspace{
		ID:   workspaceID,
		Name: "Test Workspace",
		Settings: domain.WorkspaceSettings{
			SecretKey: "test-secret-key",
		},
		Integrations: []domain.Integration{
			{
				ID:   integrationID,
				Name: "Test Integration",
				Type: "email",
				EmailProvider: domain.EmailProvider{
					Kind: domain.EmailProviderKindSparkPost,
					Senders: []domain.EmailSender{
						{
							ID:    senderID,
							Email: "sender@example.com",
							Name:  "Test Sender",
						},
					},
				},
			},
		},
	}

	// Setup template
	template := &domain.Template{
		ID:   templateID,
		Name: "Test Template",
		Email: &domain.EmailTemplate{
			Subject: "Test Subject",
			VisualEditorTree: &notifuse_mjml.MJMLBlock{
				BaseBlock: notifuse_mjml.NewBaseBlock("root", notifuse_mjml.MJMLComponentMjml),
			},
			ReplyTo: "",
		},
	}

	// Setup HTML result
	htmlResult := "<html><body>Test content</body></html>"
	compilationResult := &domain.CompileTemplateResponse{
		Success: true,
		HTML:    &htmlResult,
		Error:   nil,
	}

	service := &TransactionalNotificationService{
		transactionalRepo:  mockRepo,
		messageHistoryRepo: mockMsgHistoryRepo,
		templateService:    mockTemplateService,
		contactService:     mockContactService,
		emailService:       mockEmailService,
		logger:             mockLogger,
		workspaceRepo:      mockWorkspaceRepo,
		apiEndpoint:        "https://api.example.com",
		authService:        mockAuthService,
	}

	// Expect authentication
	mockAuthService.EXPECT().
		AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
		Return(ctx, &domain.User{ID: "user-123"}, &domain.UserWorkspace{
			UserID:      "user-123",
			WorkspaceID: workspaceID,
			Role:        "member",
			Permissions: domain.UserPermissions{
				domain.PermissionResourceTransactional: {Read: true, Write: true},
			},
		}, nil)

	// Expect get template
	mockTemplateService.EXPECT().
		GetTemplateByID(gomock.Any(), workspaceID, templateID, int64(0)).
		Return(template, nil)

	// Expect get workspace
	mockWorkspaceRepo.EXPECT().
		GetByID(gomock.Any(), workspaceID).
		Return(workspace, nil)

	// Expect upsert contact call
	mockContactService.EXPECT().
		UpsertContact(gomock.Any(), workspaceID, gomock.Any()).
		Return(domain.UpsertContactOperation{
			Email:  recipientEmail,
			Action: domain.UpsertContactOperationUpdate,
		})

	// Expect compile template
	mockTemplateService.EXPECT().
		CompileTemplate(gomock.Any(), gomock.Any()).
		Return(compilationResult, nil)

	// Expect send email
	mockEmailService.EXPECT().
		SendEmail(
			gomock.Any(),
			gomock.Any(), // SendEmailProviderRequest
			gomock.Any(), // isMarketing
		).Return(nil)

	// Expect message history creation
	mockMsgHistoryRepo.EXPECT().
		Create(gomock.Any(), workspaceID, gomock.Any()).
		Return(nil)

	// Call the method
	err := service.TestTemplate(ctx, workspaceID, templateID, integrationID, senderID, recipientEmail, domain.EmailOptions{})

	// Assertions
	require.NoError(t, err)
}

func TestTransactionalNotificationService_TestTemplate_WithChannelOptions(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockTransactionalNotificationRepository(ctrl)
	mockMsgHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
	mockTemplateService := mocks.NewMockTemplateService(ctrl)
	mockContactService := mocks.NewMockContactService(ctrl)
	mockEmailService := mocks.NewMockEmailServiceInterface(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)

	// Create a stub logger that simply returns itself for chaining calls
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

	ctx := context.Background()
	workspaceID := "test-workspace"
	templateID := uuid.New().String()
	integrationID := "integration-1"
	senderID := "sender-1"
	recipientEmail := "test@example.com"

	// Setup workspace
	workspace := &domain.Workspace{
		ID:   workspaceID,
		Name: "Test Workspace",
		Settings: domain.WorkspaceSettings{
			SecretKey: "test-secret-key",
		},
		Integrations: []domain.Integration{
			{
				ID:   integrationID,
				Name: "Test Integration",
				Type: "email",
				EmailProvider: domain.EmailProvider{
					Kind: domain.EmailProviderKindSparkPost,
					Senders: []domain.EmailSender{
						{
							ID:    senderID,
							Email: "sender@example.com",
							Name:  "Test Sender",
						},
					},
				},
			},
		},
	}

	// Setup template
	template := &domain.Template{
		ID:   templateID,
		Name: "Test Template",
		Email: &domain.EmailTemplate{
			Subject: "Test Subject",
			VisualEditorTree: &notifuse_mjml.MJMLBlock{
				BaseBlock: notifuse_mjml.NewBaseBlock("root", notifuse_mjml.MJMLComponentMjml),
			},
			ReplyTo: "",
		},
	}

	// Setup HTML result
	htmlResult := "<html><body>Test content</body></html>"
	compilationResult := &domain.CompileTemplateResponse{
		Success: true,
		HTML:    &htmlResult,
		Error:   nil,
	}

	service := &TransactionalNotificationService{
		transactionalRepo:  mockRepo,
		messageHistoryRepo: mockMsgHistoryRepo,
		templateService:    mockTemplateService,
		contactService:     mockContactService,
		emailService:       mockEmailService,
		logger:             mockLogger,
		workspaceRepo:      mockWorkspaceRepo,
		apiEndpoint:        "https://api.example.com",
		authService:        mockAuthService,
	}

	// Expect authentication
	mockAuthService.EXPECT().
		AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
		Return(ctx, &domain.User{ID: "user-123"}, &domain.UserWorkspace{
			UserID:      "user-123",
			WorkspaceID: workspaceID,
			Role:        "member",
			Permissions: domain.UserPermissions{
				domain.PermissionResourceTransactional: {Read: true, Write: true},
			},
		}, nil)

	// Expect get template
	mockTemplateService.EXPECT().
		GetTemplateByID(gomock.Any(), workspaceID, templateID, int64(0)).
		Return(template, nil)

	// Expect get workspace
	mockWorkspaceRepo.EXPECT().
		GetByID(gomock.Any(), workspaceID).
		Return(workspace, nil)

	// Expect upsert contact call
	mockContactService.EXPECT().
		UpsertContact(gomock.Any(), workspaceID, gomock.Any()).
		Return(domain.UpsertContactOperation{
			Email:  recipientEmail,
			Action: domain.UpsertContactOperationUpdate,
		})

	// Expect compile template
	mockTemplateService.EXPECT().
		CompileTemplate(gomock.Any(), gomock.Any()).
		Return(compilationResult, nil)

	// Expect send email with options
	mockEmailService.EXPECT().
		SendEmail(
			gomock.Any(),
			gomock.Any(), // SendEmailProviderRequest
			gomock.Any(), // isMarketing
		).Return(nil)

	// Expect message history creation with ChannelOptions
	fromName := "Custom Sender"
	emailOptions := domain.EmailOptions{
		FromName: &fromName,
		CC:       []string{"cc@example.com"},
		BCC:      []string{"bcc@example.com"},
		ReplyTo:  "reply@example.com",
	}

	mockMsgHistoryRepo.EXPECT().
		Create(gomock.Any(), workspaceID, gomock.Any()).
		DoAndReturn(func(ctx context.Context, workspaceID string, message *domain.MessageHistory) error {
			// Verify ChannelOptions are set
			require.NotNil(t, message.ChannelOptions)
			require.NotNil(t, message.ChannelOptions.FromName)
			assert.Equal(t, "Custom Sender", *message.ChannelOptions.FromName)
			assert.Equal(t, []string{"cc@example.com"}, message.ChannelOptions.CC)
			assert.Equal(t, []string{"bcc@example.com"}, message.ChannelOptions.BCC)
			assert.Equal(t, "reply@example.com", message.ChannelOptions.ReplyTo)
			return nil
		})

	// Call the method with email options
	err := service.TestTemplate(ctx, workspaceID, templateID, integrationID, senderID, recipientEmail, emailOptions)

	// Assertions
	require.NoError(t, err)
}

func TestTransactionalNotificationService_TestTemplate_ErrorCases(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockTransactionalNotificationRepository(ctrl)
	mockMsgHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
	mockTemplateService := mocks.NewMockTemplateService(ctrl)
	mockContactService := mocks.NewMockContactService(ctrl)
	mockEmailService := mocks.NewMockEmailServiceInterface(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)

	// Create a stub logger that simply returns itself for chaining calls
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

	ctx := context.Background()
	workspaceID := "test-workspace"
	templateID := uuid.New().String()
	integrationID := "integration-1"
	senderID := "sender-1"
	recipientEmail := "test@example.com"

	service := &TransactionalNotificationService{
		transactionalRepo:  mockRepo,
		messageHistoryRepo: mockMsgHistoryRepo,
		templateService:    mockTemplateService,
		contactService:     mockContactService,
		emailService:       mockEmailService,
		logger:             mockLogger,
		workspaceRepo:      mockWorkspaceRepo,
		apiEndpoint:        "https://api.example.com",
		authService:        mockAuthService,
	}

	t.Run("Error_AuthenticationFailed", func(t *testing.T) {
		// Auth fails
		mockAuthService.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
			Return(ctx, nil, nil, errors.New("authentication failed"))

		err := service.TestTemplate(ctx, workspaceID, templateID, integrationID, senderID, recipientEmail, domain.EmailOptions{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to authenticate user for workspace")
	})

	t.Run("Error_TemplateNotFound", func(t *testing.T) {
		// Auth succeeds
		mockAuthService.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
			Return(ctx, &domain.User{ID: "user-123"}, &domain.UserWorkspace{
				UserID:      "user-123",
				WorkspaceID: workspaceID,
				Role:        "member",
			}, nil)

		// Template not found
		mockTemplateService.EXPECT().
			GetTemplateByID(gomock.Any(), workspaceID, templateID, int64(0)).
			Return(nil, errors.New("template not found"))

		err := service.TestTemplate(ctx, workspaceID, templateID, integrationID, senderID, recipientEmail, domain.EmailOptions{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to retrieve template")
	})

	t.Run("Error_TemplateHasNoEmailContent", func(t *testing.T) {
		// Auth succeeds
		mockAuthService.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
			Return(ctx, &domain.User{ID: "user-123"}, &domain.UserWorkspace{
				UserID:      "user-123",
				WorkspaceID: workspaceID,
				Role:        "member",
			}, nil)

		// Template exists but has no email content
		template := &domain.Template{
			ID:    templateID,
			Name:  "Test Template",
			Email: nil, // No email content
		}
		mockTemplateService.EXPECT().
			GetTemplateByID(gomock.Any(), workspaceID, templateID, int64(0)).
			Return(template, nil)

		err := service.TestTemplate(ctx, workspaceID, templateID, integrationID, senderID, recipientEmail, domain.EmailOptions{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "template does not contain email content")
	})

	t.Run("Error_WorkspaceNotFound", func(t *testing.T) {
		// Auth succeeds
		mockAuthService.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
			Return(ctx, &domain.User{ID: "user-123"}, &domain.UserWorkspace{
				UserID:      "user-123",
				WorkspaceID: workspaceID,
				Role:        "member",
			}, nil)

		// Template exists with email content
		template := &domain.Template{
			ID:   templateID,
			Name: "Test Template",
			Email: &domain.EmailTemplate{
				Subject: "Test Subject",
			},
		}
		mockTemplateService.EXPECT().
			GetTemplateByID(gomock.Any(), workspaceID, templateID, int64(0)).
			Return(template, nil)

		// Workspace not found
		mockWorkspaceRepo.EXPECT().
			GetByID(gomock.Any(), workspaceID).
			Return(nil, errors.New("workspace not found"))

		err := service.TestTemplate(ctx, workspaceID, templateID, integrationID, senderID, recipientEmail, domain.EmailOptions{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get workspace")
	})

	t.Run("Error_IntegrationNotFound", func(t *testing.T) {
		// Auth succeeds
		mockAuthService.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
			Return(ctx, &domain.User{ID: "user-123"}, &domain.UserWorkspace{
				UserID:      "user-123",
				WorkspaceID: workspaceID,
				Role:        "member",
			}, nil)

		// Template exists with email content
		template := &domain.Template{
			ID:   templateID,
			Name: "Test Template",
			Email: &domain.EmailTemplate{
				Subject: "Test Subject",
			},
		}
		mockTemplateService.EXPECT().
			GetTemplateByID(gomock.Any(), workspaceID, templateID, int64(0)).
			Return(template, nil)

		// Workspace exists but integration not found
		workspace := &domain.Workspace{
			ID:           workspaceID,
			Name:         "Test Workspace",
			Integrations: []domain.Integration{}, // No integrations
		}
		mockWorkspaceRepo.EXPECT().
			GetByID(gomock.Any(), workspaceID).
			Return(workspace, nil)

		err := service.TestTemplate(ctx, workspaceID, templateID, "nonexistent-integration", senderID, recipientEmail, domain.EmailOptions{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "integration not found")
	})

	t.Run("Error_SenderNotFound", func(t *testing.T) {
		// Auth succeeds
		mockAuthService.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
			Return(ctx, &domain.User{ID: "user-123"}, &domain.UserWorkspace{
				UserID:      "user-123",
				WorkspaceID: workspaceID,
				Role:        "member",
			}, nil)

		// Template exists with email content
		template := &domain.Template{
			ID:   templateID,
			Name: "Test Template",
			Email: &domain.EmailTemplate{
				Subject: "Test Subject",
			},
		}
		mockTemplateService.EXPECT().
			GetTemplateByID(gomock.Any(), workspaceID, templateID, int64(0)).
			Return(template, nil)

		// Workspace exists with integration but sender not found
		workspace := &domain.Workspace{
			ID:   workspaceID,
			Name: "Test Workspace",
			Integrations: []domain.Integration{
				{
					ID:   integrationID,
					Name: "Test Integration",
					Type: "email",
					EmailProvider: domain.EmailProvider{
						Kind:    domain.EmailProviderKindSparkPost,
						Senders: []domain.EmailSender{}, // No senders
					},
				},
			},
		}
		mockWorkspaceRepo.EXPECT().
			GetByID(gomock.Any(), workspaceID).
			Return(workspace, nil)

		err := service.TestTemplate(ctx, workspaceID, templateID, integrationID, "nonexistent-sender", recipientEmail, domain.EmailOptions{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "sender not found")
	})

	t.Run("Error_ContactUpsertFailed", func(t *testing.T) {
		// Auth succeeds
		mockAuthService.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
			Return(ctx, &domain.User{ID: "user-123"}, &domain.UserWorkspace{
				UserID:      "user-123",
				WorkspaceID: workspaceID,
				Role:        "member",
			}, nil)

		// Template exists with email content
		template := &domain.Template{
			ID:   templateID,
			Name: "Test Template",
			Email: &domain.EmailTemplate{
				Subject: "Test Subject",
			},
		}
		mockTemplateService.EXPECT().
			GetTemplateByID(gomock.Any(), workspaceID, templateID, int64(0)).
			Return(template, nil)

		// Workspace exists with integration and sender
		workspace := &domain.Workspace{
			ID:   workspaceID,
			Name: "Test Workspace",
			Integrations: []domain.Integration{
				{
					ID:   integrationID,
					Name: "Test Integration",
					Type: "email",
					EmailProvider: domain.EmailProvider{
						Kind: domain.EmailProviderKindSparkPost,
						Senders: []domain.EmailSender{
							{
								ID:    senderID,
								Email: "sender@example.com",
								Name:  "Test Sender",
							},
						},
					},
				},
			},
		}
		mockWorkspaceRepo.EXPECT().
			GetByID(gomock.Any(), workspaceID).
			Return(workspace, nil)

		// Contact upsert fails
		mockContactService.EXPECT().
			UpsertContact(gomock.Any(), workspaceID, gomock.Any()).
			Return(domain.UpsertContactOperation{
				Email:  recipientEmail,
				Action: domain.UpsertContactOperationError,
				Error:  "database error",
			})

		err := service.TestTemplate(ctx, workspaceID, templateID, integrationID, senderID, recipientEmail, domain.EmailOptions{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to upsert contact")
	})
}
