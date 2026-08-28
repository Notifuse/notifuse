package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
	"github.com/Notifuse/notifuse/pkg/notifuse_mjml"
	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
		name string
		id   string
		// input builds the parameters directly. Parameters assembled in Go mean
		// exactly what their fields say, so a case whose subject is a key the body
		// never carried has to set body instead and go through the decoder.
		input          domain.TransactionalNotificationUpdateParams
		body           string
		mockSetup      func()
		expectedError  bool
		expectedResult *domain.TransactionalNotification
	}

	ctx := context.Background()
	workspace := "test-workspace"
	notificationID := uuid.New().String()
	templateID := uuid.New().String()
	newTemplateID := uuid.New().String()
	integrationID := "supabase-integration"

	// UpdateNotification mutates the notification the repository hands back, so
	// every Get must return its own copy. Sharing one pointer with the assertions
	// below turns assert.Equal(existingNotification.X, notif.X) into a comparison
	// of a field with itself, which holds no matter what the service did to it.
	newExistingNotification := func() *domain.TransactionalNotification {
		return &domain.TransactionalNotification{
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
	}
	existingNotification := newExistingNotification()

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
					Return(newExistingNotification(), nil)

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
					Return(newExistingNotification(), nil)

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
			// The shape that took down password resets: the console submits only
			// the field it edited, and every block the client did not mention has
			// to survive untouched.
			name: "Success_AbsentBlocksKeepStoredChannelsMetadataAndTracking",
			id:   notificationID,
			// Raw JSON: a struct literal cannot express a tracking_settings key that
			// was never sent, which is exactly the shape under test.
			body: `{"name":"Password Reset"}`,
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

				mockRepo.EXPECT().
					Get(gomock.Any(), workspace, notificationID).
					Return(&domain.TransactionalNotification{
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
						TrackingSettings: notifuse_mjml.TrackingSettings{
							TrackingMode: notifuse_mjml.TrackingModeDisabled,
							UTMSource:    "stored-source",
						},
					}, nil)

				// Assert against literals rather than the stored notification: the
				// service mutates that same struct in place.
				mockRepo.EXPECT().
					Update(gomock.Any(), workspace, gomock.Any()).
					DoAndReturn(func(_ context.Context, _ string, notif *domain.TransactionalNotification) error {
						assert.Equal(t, "Password Reset", notif.Name)
						assert.Equal(t, "Original Description", notif.Description)
						assert.Equal(t, domain.ChannelTemplates{
							domain.TransactionalChannelEmail: {TemplateID: templateID},
						}, notif.Channels)
						assert.Equal(t, domain.MapOfAny{"original": "value"}, notif.Metadata)
						assert.Equal(t, notifuse_mjml.TrackingSettings{
							TrackingMode: notifuse_mjml.TrackingModeDisabled,
							UTMSource:    "stored-source",
						}, notif.TrackingSettings)
						return nil
					})
			},
			expectedError: false,
			expectedResult: &domain.TransactionalNotification{
				ID:          notificationID,
				Name:        "Password Reset",
				Description: "Original Description",
				Channels: map[domain.TransactionalChannel]domain.ChannelTemplate{
					domain.TransactionalChannelEmail: {
						TemplateID: templateID,
					},
				},
				Metadata: map[string]interface{}{
					"original": "value",
				},
				TrackingSettings: notifuse_mjml.TrackingSettings{
					TrackingMode: notifuse_mjml.TrackingModeDisabled,
					UTMSource:    "stored-source",
				},
			},
		},
		{
			name: "Success_KeepsStoredTrackingModeWhenAbsentOnRegularUpdate",
			id:   notificationID,
			input: domain.TransactionalNotificationUpdateParams{
				// Mirrors the console drawer: only UTM fields are submitted
				TrackingSettings: notifuse_mjml.TrackingSettings{
					UTMSource: "newsletter",
				},
			},
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

				// Stored notification opted out of tracking
				mockRepo.EXPECT().
					Get(gomock.Any(), workspace, notificationID).
					Return(&domain.TransactionalNotification{
						ID:   notificationID,
						Name: "Opted Out",
						TrackingSettings: notifuse_mjml.TrackingSettings{
							TrackingMode: notifuse_mjml.TrackingModeDisabled,
						},
					}, nil)

				// The stored opt-out survives the tracking settings replacement
				mockRepo.EXPECT().
					Update(gomock.Any(), workspace, gomock.Any()).
					DoAndReturn(func(_ context.Context, _ string, notif *domain.TransactionalNotification) error {
						assert.Equal(t, notifuse_mjml.TrackingModeDisabled, notif.TrackingSettings.TrackingMode)
						assert.Equal(t, "newsletter", notif.TrackingSettings.UTMSource)
						return nil
					})
			},
			expectedError: false,
			expectedResult: &domain.TransactionalNotification{
				ID:   notificationID,
				Name: "Opted Out",
				TrackingSettings: notifuse_mjml.TrackingSettings{
					TrackingMode: notifuse_mjml.TrackingModeDisabled,
					UTMSource:    "newsletter",
				},
			},
		},
		{
			name: "Success_KeepsStoredTrackingModeWhenAbsentOnIntegrationManagedUpdate",
			id:   notificationID,
			input: domain.TransactionalNotificationUpdateParams{
				Description: "Updated Description",
				TrackingSettings: notifuse_mjml.TrackingSettings{
					UTMSource: "newsletter",
				},
			},
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

				// Integration-managed notification (e.g. Supabase auth email) with the opt-out set
				mockRepo.EXPECT().
					Get(gomock.Any(), workspace, notificationID).
					Return(&domain.TransactionalNotification{
						ID:            notificationID,
						Name:          "Magic Link",
						IntegrationID: &integrationID,
						TrackingSettings: notifuse_mjml.TrackingSettings{
							TrackingMode: notifuse_mjml.TrackingModeDisabled,
						},
					}, nil)

				mockRepo.EXPECT().
					Update(gomock.Any(), workspace, gomock.Any()).
					DoAndReturn(func(_ context.Context, _ string, notif *domain.TransactionalNotification) error {
						assert.Equal(t, notifuse_mjml.TrackingModeDisabled, notif.TrackingSettings.TrackingMode)
						assert.Equal(t, "newsletter", notif.TrackingSettings.UTMSource)
						assert.Equal(t, "Updated Description", notif.Description)
						return nil
					})
			},
			expectedError: false,
			expectedResult: &domain.TransactionalNotification{
				ID:            notificationID,
				Name:          "Magic Link",
				Description:   "Updated Description",
				IntegrationID: &integrationID,
				TrackingSettings: notifuse_mjml.TrackingSettings{
					TrackingMode: notifuse_mjml.TrackingModeDisabled,
					UTMSource:    "newsletter",
				},
			},
		},
		{
			name: "Success_ExplicitInheritResetsStoredOptOut",
			id:   notificationID,
			input: domain.TransactionalNotificationUpdateParams{
				TrackingSettings: notifuse_mjml.TrackingSettings{
					TrackingMode: notifuse_mjml.TrackingModeInherit,
					UTMSource:    "newsletter",
				},
			},
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

				mockRepo.EXPECT().
					Get(gomock.Any(), workspace, notificationID).
					Return(&domain.TransactionalNotification{
						ID:   notificationID,
						Name: "Opted Out",
						TrackingSettings: notifuse_mjml.TrackingSettings{
							TrackingMode: notifuse_mjml.TrackingModeDisabled,
						},
					}, nil)

				// The explicit reset clears the stored opt-out (canonicalized to "")
				mockRepo.EXPECT().
					Update(gomock.Any(), workspace, gomock.Any()).
					DoAndReturn(func(_ context.Context, _ string, notif *domain.TransactionalNotification) error {
						assert.Empty(t, notif.TrackingSettings.TrackingMode)
						assert.Equal(t, "newsletter", notif.TrackingSettings.UTMSource)
						return nil
					})
			},
			expectedError: false,
			expectedResult: &domain.TransactionalNotification{
				ID:   notificationID,
				Name: "Opted Out",
				TrackingSettings: notifuse_mjml.TrackingSettings{
					UTMSource: "newsletter",
				},
			},
		},
		{
			name: "Success_ExplicitDisabledSetViaUpdate",
			id:   notificationID,
			input: domain.TransactionalNotificationUpdateParams{
				TrackingSettings: notifuse_mjml.TrackingSettings{
					TrackingMode: notifuse_mjml.TrackingModeDisabled,
				},
			},
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

				mockRepo.EXPECT().
					Get(gomock.Any(), workspace, notificationID).
					Return(&domain.TransactionalNotification{
						ID:   notificationID,
						Name: "Regular",
					}, nil)

				mockRepo.EXPECT().
					Update(gomock.Any(), workspace, gomock.Any()).
					DoAndReturn(func(_ context.Context, _ string, notif *domain.TransactionalNotification) error {
						assert.Equal(t, notifuse_mjml.TrackingModeDisabled, notif.TrackingSettings.TrackingMode)
						return nil
					})
			},
			expectedError: false,
			expectedResult: &domain.TransactionalNotification{
				ID:   notificationID,
				Name: "Regular",
				TrackingSettings: notifuse_mjml.TrackingSettings{
					TrackingMode: notifuse_mjml.TrackingModeDisabled,
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
					Return(newExistingNotification(), nil)

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
					Return(newExistingNotification(), nil)

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

			params := tc.input
			if tc.body != "" {
				require.NoError(t, json.Unmarshal([]byte(tc.body), &params))
			}

			// Call the method being tested
			result, err := service.UpdateNotification(ctx, workspace, tc.id, params)

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
					domain.PermissionResourceContacts:      {Read: true, Write: true},
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
			// Verify transactional notification ID is passed through
			require.NotNil(t, request.TransactionalNotificationID)
			assert.Equal(t, notificationID, *request.TransactionalNotificationID)
		}).Return(nil)

		// Message history creation happens inside SendEmailForTemplate

		// Call the method
		messageID, err := service.SendNotification(ctx, workspace, params)

		// Assertions
		require.NoError(t, err)
		require.NotEmpty(t, messageID)
	})

	t.Run("Success_DisabledModeSuppressesWorkspaceTracking", func(t *testing.T) {
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

		// Workspace-level tracking is ON
		trackingWorkspace := &domain.Workspace{
			ID:   workspace,
			Name: "Test Workspace",
			Settings: domain.WorkspaceSettings{
				TransactionalEmailProviderID: "integration-1",
				SecretKey:                    "test-secret-key",
				EmailTrackingEnabled:         true,
			},
			Integrations: workspaceObj.Integrations,
		}

		// Notification opted out of tracking (e.g. Supabase auth email)
		optedOutNotification := &domain.TransactionalNotification{
			ID:          notificationID,
			Name:        "Magic Link",
			Description: "Auth email",
			Channels: map[domain.TransactionalChannel]domain.ChannelTemplate{
				domain.TransactionalChannelEmail: {
					TemplateID: templateID,
				},
			},
			TrackingSettings: notifuse_mjml.TrackingSettings{
				EnableTracking: false,
				TrackingMode:   notifuse_mjml.TrackingModeDisabled,
			},
		}

		params := domain.TransactionalNotificationSendParams{
			ID:      notificationID,
			Contact: contact,
		}

		mockAuthService.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), workspace).
			Return(ctx, &domain.User{ID: "user-123"}, &domain.UserWorkspace{
				UserID:      "user-123",
				WorkspaceID: workspace,
				Role:        "member",
				Permissions: domain.UserPermissions{
					domain.PermissionResourceTransactional: {Read: true, Write: true},
					domain.PermissionResourceContacts:      {Read: true, Write: true},
				},
			}, nil)

		mockWorkspaceRepo.EXPECT().
			GetByID(gomock.Any(), workspace).
			Return(trackingWorkspace, nil)

		mockRepo.EXPECT().
			Get(gomock.Any(), workspace, notificationID).
			Return(optedOutNotification, nil)

		mockContactService.EXPECT().
			UpsertContact(gomock.Any(), workspace, contact).
			Return(domain.UpsertContactOperation{
				Email:  contact.Email,
				Action: domain.UpsertContactOperationUpdate,
			})

		mockContactService.EXPECT().
			GetContactByEmail(gomock.Any(), workspace, contact.Email).
			Return(contact, nil)

		// The opt-out wins over the workspace flag
		mockEmailService.EXPECT().
			SendEmailForTemplate(gomock.Any(), gomock.Any()).
			Do(func(_ context.Context, request domain.SendEmailRequest) {
				assert.False(t, request.TrackingSettings.EnableTracking)
				assert.Equal(t, notifuse_mjml.TrackingModeDisabled, request.TrackingSettings.TrackingMode)
			}).Return(nil)

		messageID, err := service.SendNotification(ctx, workspace, params)

		require.NoError(t, err)
		require.NotEmpty(t, messageID)
	})

	t.Run("Success_UnsetModeKeepsTracking", func(t *testing.T) {
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

		// Workspace-level tracking is ON
		trackingWorkspace := &domain.Workspace{
			ID:   workspace,
			Name: "Test Workspace",
			Settings: domain.WorkspaceSettings{
				TransactionalEmailProviderID: "integration-1",
				SecretKey:                    "test-secret-key",
				EmailTrackingEnabled:         true,
			},
			Integrations: workspaceObj.Integrations,
		}

		// Regular notification: zero-value TrackingSettings (console-created)
		regularNotification := &domain.TransactionalNotification{
			ID:          notificationID,
			Name:        "Order Confirmation",
			Description: "Regular notification",
			Channels: map[domain.TransactionalChannel]domain.ChannelTemplate{
				domain.TransactionalChannelEmail: {
					TemplateID: templateID,
				},
			},
		}

		params := domain.TransactionalNotificationSendParams{
			ID:      notificationID,
			Contact: contact,
		}

		mockAuthService.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), workspace).
			Return(ctx, &domain.User{ID: "user-123"}, &domain.UserWorkspace{
				UserID:      "user-123",
				WorkspaceID: workspace,
				Role:        "member",
				Permissions: domain.UserPermissions{
					domain.PermissionResourceTransactional: {Read: true, Write: true},
					domain.PermissionResourceContacts:      {Read: true, Write: true},
				},
			}, nil)

		mockWorkspaceRepo.EXPECT().
			GetByID(gomock.Any(), workspace).
			Return(trackingWorkspace, nil)

		mockRepo.EXPECT().
			Get(gomock.Any(), workspace, notificationID).
			Return(regularNotification, nil)

		mockContactService.EXPECT().
			UpsertContact(gomock.Any(), workspace, contact).
			Return(domain.UpsertContactOperation{
				Email:  contact.Email,
				Action: domain.UpsertContactOperationUpdate,
			})

		mockContactService.EXPECT().
			GetContactByEmail(gomock.Any(), workspace, contact.Email).
			Return(contact, nil)

		// Without the opt-out, the workspace flag applies as before
		mockEmailService.EXPECT().
			SendEmailForTemplate(gomock.Any(), gomock.Any()).
			Do(func(_ context.Context, request domain.SendEmailRequest) {
				assert.True(t, request.TrackingSettings.EnableTracking)
				assert.Empty(t, request.TrackingSettings.TrackingMode)
			}).Return(nil)

		messageID, err := service.SendNotification(ctx, workspace, params)

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
					domain.PermissionResourceContacts:      {Read: true, Write: true},
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
					domain.PermissionResourceContacts:      {Read: true, Write: true},
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
					domain.PermissionResourceContacts:      {Read: true, Write: true},
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
					domain.PermissionResourceContacts:      {Read: true, Write: true},
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
			GetByExternalID(gomock.Any(), workspace, gomock.Any(), externalID).
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
					domain.PermissionResourceContacts:      {Read: true, Write: true},
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
					domain.PermissionResourceContacts:      {Read: true, Write: true},
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
					domain.PermissionResourceContacts:      {Read: true, Write: true},
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
			GetByExternalID(gomock.Any(), workspace, gomock.Any(), externalID).
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

	// Expect get contact by email after upsert (returns full contact)
	mockContactService.EXPECT().
		GetContactByEmail(gomock.Any(), workspaceID, recipientEmail).
		Return(&domain.Contact{
			Email:     recipientEmail,
			FirstName: &domain.NullableString{String: "Test", IsNull: false},
			LastName:  &domain.NullableString{String: "User", IsNull: false},
		}, nil)

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
		Create(gomock.Any(), workspaceID, gomock.Any(), gomock.Any()).
		Return(nil)

	// Call the method
	err := service.TestTemplate(ctx, workspaceID, templateID, integrationID, senderID, recipientEmail, "", domain.EmailOptions{})

	// Assertions
	require.NoError(t, err)
}

func TestTransactionalNotificationService_TestTemplate_TrackingFollowsWorkspaceFlag(t *testing.T) {
	for _, trackingEnabled := range []bool{true, false} {
		name := "WorkspaceTrackingEnabled"
		if !trackingEnabled {
			name = "WorkspaceTrackingDisabled"
		}
		t.Run(name, func(t *testing.T) {
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

			workspace := &domain.Workspace{
				ID:   workspaceID,
				Name: "Test Workspace",
				Settings: domain.WorkspaceSettings{
					SecretKey:            "test-secret-key",
					EmailTrackingEnabled: trackingEnabled,
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

			template := &domain.Template{
				ID:   templateID,
				Name: "Test Template",
				Email: &domain.EmailTemplate{
					Subject: "Test Subject",
					VisualEditorTree: &notifuse_mjml.MJMLBlock{
						BaseBlock: notifuse_mjml.NewBaseBlock("root", notifuse_mjml.MJMLComponentMjml),
					},
				},
			}

			htmlResult := "<html><body>Test content</body></html>"

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

			mockTemplateService.EXPECT().
				GetTemplateByID(gomock.Any(), workspaceID, templateID, int64(0)).
				Return(template, nil)

			mockWorkspaceRepo.EXPECT().
				GetByID(gomock.Any(), workspaceID).
				Return(workspace, nil)

			mockContactService.EXPECT().
				UpsertContact(gomock.Any(), workspaceID, gomock.Any()).
				Return(domain.UpsertContactOperation{
					Email:  recipientEmail,
					Action: domain.UpsertContactOperationUpdate,
				})

			mockContactService.EXPECT().
				GetContactByEmail(gomock.Any(), workspaceID, recipientEmail).
				Return(&domain.Contact{Email: recipientEmail}, nil)

			// Test sends follow the workspace tracking flag instead of always tracking
			mockTemplateService.EXPECT().
				CompileTemplate(gomock.Any(), gomock.Any()).
				DoAndReturn(func(_ context.Context, req domain.CompileTemplateRequest) (*domain.CompileTemplateResponse, error) {
					assert.Equal(t, trackingEnabled, req.TrackingSettings.EnableTracking)
					return &domain.CompileTemplateResponse{
						Success: true,
						HTML:    &htmlResult,
					}, nil
				})

			mockEmailService.EXPECT().
				SendEmail(gomock.Any(), gomock.Any(), gomock.Any()).
				Return(nil)

			mockMsgHistoryRepo.EXPECT().
				Create(gomock.Any(), workspaceID, gomock.Any(), gomock.Any()).
				Return(nil)

			err := service.TestTemplate(ctx, workspaceID, templateID, integrationID, senderID, recipientEmail, "", domain.EmailOptions{})

			require.NoError(t, err)
		})
	}
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
				domain.PermissionResourceContacts:      {Read: true, Write: true},
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

	// Expect get contact by email after upsert
	mockContactService.EXPECT().
		GetContactByEmail(gomock.Any(), workspaceID, recipientEmail).
		Return(&domain.Contact{
			Email:     recipientEmail,
			FirstName: &domain.NullableString{String: "Test", IsNull: false},
			LastName:  &domain.NullableString{String: "User", IsNull: false},
		}, nil)

	// Expect compile template - verify SubjectPreviewOverride is passed
	mockTemplateService.EXPECT().
		CompileTemplate(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, req domain.CompileTemplateRequest) (*domain.CompileTemplateResponse, error) {
			require.NotNil(t, req.SubjectPreviewOverride)
			assert.Equal(t, "Override Preview", *req.SubjectPreviewOverride)
			return compilationResult, nil
		})

	// Expect send email with options - verify subject and from name overrides
	mockEmailService.EXPECT().
		SendEmail(
			gomock.Any(),
			gomock.Any(), // SendEmailProviderRequest
			gomock.Any(), // isMarketing
		).DoAndReturn(func(ctx context.Context, req domain.SendEmailProviderRequest, isMarketing bool) error {
		// Verify the subject was overridden
		assert.Equal(t, "Override Subject", req.Subject)
		// Verify the from name was overridden
		assert.Equal(t, "Custom Sender", req.FromName)
		return nil
	})

	// Expect message history creation with ChannelOptions
	fromName := "Custom Sender"
	overrideSubject := "Override Subject"
	overridePreview := "Override Preview"
	emailOptions := domain.EmailOptions{
		FromName:       &fromName,
		Subject:        &overrideSubject,
		SubjectPreview: &overridePreview,
		CC:             []string{"cc@example.com"},
		BCC:            []string{"bcc@example.com"},
		ReplyTo:        "reply@example.com",
	}

	mockMsgHistoryRepo.EXPECT().
		Create(gomock.Any(), workspaceID, gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, workspaceID string, secretKey string, message *domain.MessageHistory) error {
			// Verify ChannelOptions are set
			require.NotNil(t, message.ChannelOptions)
			require.NotNil(t, message.ChannelOptions.FromName)
			assert.Equal(t, "Custom Sender", *message.ChannelOptions.FromName)
			require.NotNil(t, message.ChannelOptions.Subject)
			assert.Equal(t, "Override Subject", *message.ChannelOptions.Subject)
			require.NotNil(t, message.ChannelOptions.SubjectPreview)
			assert.Equal(t, "Override Preview", *message.ChannelOptions.SubjectPreview)
			assert.Equal(t, []string{"cc@example.com"}, message.ChannelOptions.CC)
			assert.Equal(t, []string{"bcc@example.com"}, message.ChannelOptions.BCC)
			assert.Equal(t, "reply@example.com", message.ChannelOptions.ReplyTo)
			return nil
		})

	// Call the method with email options
	err := service.TestTemplate(ctx, workspaceID, templateID, integrationID, senderID, recipientEmail, "", emailOptions)

	// Assertions
	require.NoError(t, err)
}

func TestTransactionalNotificationService_TestTemplate_TemplateReplyToFallback(t *testing.T) {
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

	// Setup template with a reply-to address
	template := &domain.Template{
		ID:   templateID,
		Name: "Test Template",
		Email: &domain.EmailTemplate{
			Subject: "Test Subject",
			VisualEditorTree: &notifuse_mjml.MJMLBlock{
				BaseBlock: notifuse_mjml.NewBaseBlock("root", notifuse_mjml.MJMLComponentMjml),
			},
			ReplyTo: "template-reply@example.com",
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
		}, nil).
		Times(2)

	// Expect get template
	mockTemplateService.EXPECT().
		GetTemplateByID(gomock.Any(), workspaceID, templateID, int64(0)).
		Return(template, nil).
		Times(2)

	// Expect get workspace
	mockWorkspaceRepo.EXPECT().
		GetByID(gomock.Any(), workspaceID).
		Return(workspace, nil).
		Times(2)

	// Expect upsert contact call
	mockContactService.EXPECT().
		UpsertContact(gomock.Any(), workspaceID, gomock.Any()).
		Return(domain.UpsertContactOperation{
			Email:  recipientEmail,
			Action: domain.UpsertContactOperationUpdate,
		}).
		Times(2)

	// Expect get contact by email after upsert
	mockContactService.EXPECT().
		GetContactByEmail(gomock.Any(), workspaceID, recipientEmail).
		Return(&domain.Contact{
			Email: recipientEmail,
		}, nil).
		Times(2)

	// Expect compile template
	mockTemplateService.EXPECT().
		CompileTemplate(gomock.Any(), gomock.Any()).
		Return(compilationResult, nil).
		Times(2)

	// Expect message history creation
	mockMsgHistoryRepo.EXPECT().
		Create(gomock.Any(), workspaceID, gomock.Any(), gomock.Any()).
		Return(nil).
		Times(2)

	// 1) No reply-to in the options: the template's reply-to must be used
	mockEmailService.EXPECT().
		SendEmail(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, req domain.SendEmailProviderRequest, isMarketing bool) error {
			assert.Equal(t, "template-reply@example.com", req.EmailOptions.ReplyTo)
			return nil
		})

	err := service.TestTemplate(ctx, workspaceID, templateID, integrationID, senderID, recipientEmail, "", domain.EmailOptions{})
	require.NoError(t, err)

	// 2) Explicit reply-to in the options takes precedence over the template's
	mockEmailService.EXPECT().
		SendEmail(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, req domain.SendEmailProviderRequest, isMarketing bool) error {
			assert.Equal(t, "explicit-reply@example.com", req.EmailOptions.ReplyTo)
			return nil
		})

	err = service.TestTemplate(ctx, workspaceID, templateID, integrationID, senderID, recipientEmail, "", domain.EmailOptions{
		ReplyTo: "explicit-reply@example.com",
	})
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

		err := service.TestTemplate(ctx, workspaceID, templateID, integrationID, senderID, recipientEmail, "", domain.EmailOptions{})
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
				Permissions: domain.UserPermissions{
					domain.PermissionResourceTransactional: {Read: true, Write: true},
				},
			}, nil)

		// Template not found
		mockTemplateService.EXPECT().
			GetTemplateByID(gomock.Any(), workspaceID, templateID, int64(0)).
			Return(nil, errors.New("template not found"))

		err := service.TestTemplate(ctx, workspaceID, templateID, integrationID, senderID, recipientEmail, "", domain.EmailOptions{})
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
				Permissions: domain.UserPermissions{
					domain.PermissionResourceTransactional: {Read: true, Write: true},
				},
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

		err := service.TestTemplate(ctx, workspaceID, templateID, integrationID, senderID, recipientEmail, "", domain.EmailOptions{})
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
				Permissions: domain.UserPermissions{
					domain.PermissionResourceTransactional: {Read: true, Write: true},
				},
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

		err := service.TestTemplate(ctx, workspaceID, templateID, integrationID, senderID, recipientEmail, "", domain.EmailOptions{})
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
				Permissions: domain.UserPermissions{
					domain.PermissionResourceTransactional: {Read: true, Write: true},
				},
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

		err := service.TestTemplate(ctx, workspaceID, templateID, "nonexistent-integration", senderID, recipientEmail, "", domain.EmailOptions{})
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
				Permissions: domain.UserPermissions{
					domain.PermissionResourceTransactional: {Read: true, Write: true},
				},
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

		err := service.TestTemplate(ctx, workspaceID, templateID, integrationID, "nonexistent-sender", recipientEmail, "", domain.EmailOptions{})
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
				Permissions: domain.UserPermissions{
					domain.PermissionResourceTransactional: {Read: true, Write: true},
				},
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

		err := service.TestTemplate(ctx, workspaceID, templateID, integrationID, senderID, recipientEmail, "", domain.EmailOptions{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to upsert contact")
	})

	t.Run("Fallback_GetContactByEmailFailed", func(t *testing.T) {
		// Auth succeeds
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

		// Template with email content
		template := &domain.Template{
			ID:   templateID,
			Name: "Test Template",
			Email: &domain.EmailTemplate{
				Subject: "Test Subject",
				VisualEditorTree: &notifuse_mjml.MJMLBlock{
					BaseBlock: notifuse_mjml.NewBaseBlock("root", notifuse_mjml.MJMLComponentMjml),
				},
			},
		}
		mockTemplateService.EXPECT().
			GetTemplateByID(gomock.Any(), workspaceID, templateID, int64(0)).
			Return(template, nil)

		// Workspace with valid integration and sender
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
		mockWorkspaceRepo.EXPECT().
			GetByID(gomock.Any(), workspaceID).
			Return(workspace, nil)

		// Upsert succeeds
		mockContactService.EXPECT().
			UpsertContact(gomock.Any(), workspaceID, gomock.Any()).
			Return(domain.UpsertContactOperation{
				Email:  recipientEmail,
				Action: domain.UpsertContactOperationCreate,
			})

		// GetContactByEmail fails - should fallback to minimal contact
		mockContactService.EXPECT().
			GetContactByEmail(gomock.Any(), workspaceID, recipientEmail).
			Return(nil, errors.New("contact not found"))

		// Compilation succeeds (proving fallback worked)
		htmlResult := "<html><body>Test</body></html>"
		mockTemplateService.EXPECT().
			CompileTemplate(gomock.Any(), gomock.Any()).
			Return(&domain.CompileTemplateResponse{
				Success: true,
				HTML:    &htmlResult,
			}, nil)

		// Email sends successfully
		mockEmailService.EXPECT().
			SendEmail(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil)

		// Message history created
		mockMsgHistoryRepo.EXPECT().
			Create(gomock.Any(), workspaceID, gomock.Any(), gomock.Any()).
			Return(nil)

		err := service.TestTemplate(ctx, workspaceID, templateID, integrationID, senderID, recipientEmail, "", domain.EmailOptions{})
		require.NoError(t, err)
	})
}

func TestTransactionalNotificationService_TestTemplate_WithLanguage(t *testing.T) {
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

	// Setup workspace with default language "en"
	workspace := &domain.Workspace{
		ID:   workspaceID,
		Name: "Test Workspace",
		Settings: domain.WorkspaceSettings{
			SecretKey:       "test-secret-key",
			DefaultLanguage: "en",
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

	// Setup template with French translation
	template := &domain.Template{
		ID:   templateID,
		Name: "Test Template",
		Email: &domain.EmailTemplate{
			Subject: "English Subject",
			VisualEditorTree: &notifuse_mjml.MJMLBlock{
				BaseBlock: notifuse_mjml.NewBaseBlock("root", notifuse_mjml.MJMLComponentMjml),
			},
		},
		Translations: map[string]domain.TemplateTranslation{
			"fr": {
				Email: &domain.EmailTemplate{
					Subject: "Sujet Français",
					VisualEditorTree: &notifuse_mjml.MJMLBlock{
						BaseBlock: notifuse_mjml.NewBaseBlock("root-fr", notifuse_mjml.MJMLComponentMjml),
					},
				},
			},
		},
	}

	htmlResult := "<html><body>Contenu français</body></html>"
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

	mockTemplateService.EXPECT().
		GetTemplateByID(gomock.Any(), workspaceID, templateID, int64(0)).
		Return(template, nil)

	mockWorkspaceRepo.EXPECT().
		GetByID(gomock.Any(), workspaceID).
		Return(workspace, nil)

	mockContactService.EXPECT().
		UpsertContact(gomock.Any(), workspaceID, gomock.Any()).
		Return(domain.UpsertContactOperation{
			Email:  recipientEmail,
			Action: domain.UpsertContactOperationUpdate,
		})

	// Expect get contact by email after upsert
	mockContactService.EXPECT().
		GetContactByEmail(gomock.Any(), workspaceID, recipientEmail).
		Return(&domain.Contact{
			Email:     recipientEmail,
			FirstName: &domain.NullableString{String: "Test", IsNull: false},
			LastName:  &domain.NullableString{String: "User", IsNull: false},
		}, nil)

	// Verify the compile is called with the French translation's visual editor tree
	mockTemplateService.EXPECT().
		CompileTemplate(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, req domain.CompileTemplateRequest) (*domain.CompileTemplateResponse, error) {
			// The visual editor tree should be from the French translation
			assert.Equal(t, "root-fr", req.VisualEditorTree.GetID())
			return compilationResult, nil
		})

	// Verify the email is sent with the French subject
	mockEmailService.EXPECT().
		SendEmail(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, req domain.SendEmailProviderRequest, isMarketing bool) error {
			assert.Equal(t, "Sujet Français", req.Subject)
			return nil
		})

	mockMsgHistoryRepo.EXPECT().
		Create(gomock.Any(), workspaceID, gomock.Any(), gomock.Any()).
		Return(nil)

	err := service.TestTemplate(ctx, workspaceID, templateID, integrationID, senderID, recipientEmail, "fr", domain.EmailOptions{})
	require.NoError(t, err)
}

func TestEffectiveTracking(t *testing.T) {
	tests := []struct {
		name             string
		workspaceEnabled bool
		mode             string
		expected         bool
	}{
		{"workspace on, unset mode inherits", true, "", true},
		{"workspace on, explicit inherit", true, notifuse_mjml.TrackingModeInherit, true},
		{"workspace on, disabled wins", true, notifuse_mjml.TrackingModeDisabled, false},
		{"workspace off is a kill-switch for unset", false, "", false},
		{"workspace off is a kill-switch for inherit", false, notifuse_mjml.TrackingModeInherit, false},
		{"workspace off, disabled", false, notifuse_mjml.TrackingModeDisabled, false},
		{"unknown future mode degrades to inherit", true, "force_on", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, effectiveTracking(tc.workspaceEnabled, tc.mode))
		})
	}
}

func TestCanonicalTrackingMode(t *testing.T) {
	assert.Equal(t, "", canonicalTrackingMode(""))
	assert.Equal(t, "", canonicalTrackingMode(notifuse_mjml.TrackingModeInherit))
	assert.Equal(t, notifuse_mjml.TrackingModeDisabled, canonicalTrackingMode(notifuse_mjml.TrackingModeDisabled))
}

// TestTransactionalNotificationService_PermissionGates pins list, update and delete to
// the transactional permission. They authenticated workspace membership and stopped
// there, so a member whose transactional access had been revoked could still read,
// edit and delete notifications — the console offers the permission and the API
// ignored it. Create already checks write and get already checks read; these three
// follow the same split.
func TestTransactionalNotificationService_PermissionGates(t *testing.T) {
	const workspace = "test-workspace"
	notificationID := uuid.New().String()

	// A member holding every permission except transactional: enough to be in the
	// workspace, not enough to touch transactional notifications.
	noTransactional := &domain.UserWorkspace{
		UserID:      "user-123",
		WorkspaceID: workspace,
		Role:        "member",
		Permissions: domain.UserPermissions{
			domain.PermissionResourceTemplates: {Read: true, Write: true},
			domain.PermissionResourceContacts:  {Read: true, Write: true},
		},
	}
	// Read granted, write withheld — the case that separates list from update/delete.
	readOnly := &domain.UserWorkspace{
		UserID:      "user-123",
		WorkspaceID: workspace,
		Role:        "member",
		Permissions: domain.UserPermissions{
			domain.PermissionResourceTransactional: {Read: true, Write: false},
		},
	}

	testCases := []struct {
		name               string
		userWorkspace      *domain.UserWorkspace
		call               func(context.Context, *TransactionalNotificationService) error
		expectedPermission domain.PermissionType
	}{
		{
			name:          "list without transactional read",
			userWorkspace: noTransactional,
			call: func(ctx context.Context, s *TransactionalNotificationService) error {
				_, _, err := s.ListNotifications(ctx, workspace, nil, 10, 0)
				return err
			},
			expectedPermission: domain.PermissionTypeRead,
		},
		{
			name:          "update without transactional write",
			userWorkspace: readOnly,
			call: func(ctx context.Context, s *TransactionalNotificationService) error {
				_, err := s.UpdateNotification(ctx, workspace, notificationID, domain.TransactionalNotificationUpdateParams{
					Name: "Renamed",
				})
				return err
			},
			expectedPermission: domain.PermissionTypeWrite,
		},
		{
			name:          "delete without transactional write",
			userWorkspace: readOnly,
			call: func(ctx context.Context, s *TransactionalNotificationService) error {
				return s.DeleteNotification(ctx, workspace, notificationID)
			},
			expectedPermission: domain.PermissionTypeWrite,
		},
		{
			// contacts:write is granted here, so this fails if sending is left to be
			// gated by the nested contact upsert rather than by transactional itself.
			name:          "send without transactional write",
			userWorkspace: noTransactional,
			call: func(ctx context.Context, s *TransactionalNotificationService) error {
				_, err := s.SendNotification(ctx, workspace, domain.TransactionalNotificationSendParams{
					ID:      notificationID,
					Contact: &domain.Contact{Email: "recipient@example.com"},
				})
				return err
			},
			expectedPermission: domain.PermissionTypeWrite,
		},
		{
			// testTemplate sends a real email through the workspace's provider.
			name:          "test template without transactional write",
			userWorkspace: readOnly,
			call: func(ctx context.Context, s *TransactionalNotificationService) error {
				return s.TestTemplate(ctx, workspace, "template-1", "integration-1", "sender-1",
					"recipient@example.com", "", domain.EmailOptions{})
			},
			expectedPermission: domain.PermissionTypeWrite,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockRepo := mocks.NewMockTransactionalNotificationRepository(ctrl)
			mockAuthService := mocks.NewMockAuthService(ctrl)
			mockLogger := pkgmocks.NewMockLogger(ctrl)
			mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
			mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
			mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
			mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
			mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

			ctx := context.Background()
			mockAuthService.EXPECT().
				AuthenticateUserForWorkspace(gomock.Any(), workspace).
				Return(ctx, &domain.User{ID: "user-123"}, tc.userWorkspace, nil)

			// No repository call may happen: the denial has to land before any work.
			service := NewTransactionalNotificationService(
				mockRepo,
				mocks.NewMockMessageHistoryRepository(ctrl),
				mocks.NewMockTemplateService(ctrl),
				mocks.NewMockContactService(ctrl),
				mocks.NewMockEmailServiceInterface(ctrl),
				mockAuthService,
				mockLogger,
				mocks.NewMockWorkspaceRepository(ctrl),
				"https://api.example.com",
			)

			err := tc.call(ctx, service)

			require.Error(t, err)
			var permErr *domain.PermissionError
			require.ErrorAs(t, err, &permErr, "denial must be a *domain.PermissionError so the handler answers 403")
			assert.Equal(t, domain.PermissionResourceTransactional, permErr.Resource)
			assert.Equal(t, tc.expectedPermission, permErr.Permission)
		})
	}
}

// An owner holds every permission implicitly, with no permissions map at all — the
// gates must not lock out the role that is meant to bypass them.
func TestTransactionalNotificationService_PermissionGates_OwnerBypasses(t *testing.T) {
	const workspace = "test-workspace"
	notificationID := uuid.New().String()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockTransactionalNotificationRepository(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

	ctx := context.Background()
	owner := &domain.UserWorkspace{UserID: "user-123", WorkspaceID: workspace, Role: "owner"}
	mockAuthService.EXPECT().
		AuthenticateUserForWorkspace(gomock.Any(), workspace).
		Return(ctx, &domain.User{ID: "user-123"}, owner, nil).
		Times(2)

	service := NewTransactionalNotificationService(
		mockRepo,
		mocks.NewMockMessageHistoryRepository(ctrl),
		mocks.NewMockTemplateService(ctrl),
		mocks.NewMockContactService(ctrl),
		mocks.NewMockEmailServiceInterface(ctrl),
		mockAuthService,
		mockLogger,
		mocks.NewMockWorkspaceRepository(ctrl),
		"https://api.example.com",
	)

	mockRepo.EXPECT().
		List(gomock.Any(), workspace, gomock.Any(), gomock.Any(), gomock.Any()).
		Return([]*domain.TransactionalNotification{}, 0, nil)
	_, _, err := service.ListNotifications(ctx, workspace, nil, 10, 0)
	require.NoError(t, err)

	mockRepo.EXPECT().
		Get(gomock.Any(), workspace, notificationID).
		Return(&domain.TransactionalNotification{ID: notificationID}, nil)
	mockRepo.EXPECT().
		Delete(gomock.Any(), workspace, notificationID).
		Return(nil)
	require.NoError(t, service.DeleteNotification(ctx, workspace, notificationID))
}

// TestTransactionalNotificationService_SendNotification_SystemCallSkipsAuth pins the
// path the SMTP bridge and the Supabase webhook use. Both build a context carrying
// SystemCallKey because they have already authenticated the caller themselves, and
// SendNotification then skips authentication entirely — which also means it never
// obtains a UserWorkspace. Any permission check added to this method has to live
// inside that same branch: outside it, HasPermission dereferences a nil pointer and
// every bridge send panics.
//
// The auth service mock carries NO expectation on purpose. gomock fails the test if
// AuthenticateUserForWorkspace is called at all, so this asserts the bypass itself
// rather than merely asserting that the send happened to succeed.
func TestTransactionalNotificationService_SendNotification_SystemCallSkipsAuth(t *testing.T) {
	const workspace = "test-workspace"
	notificationID := uuid.New().String()
	templateID := uuid.New().String()

	notification := &domain.TransactionalNotification{
		ID:   notificationID,
		Name: "Test Notification",
		Channels: map[domain.TransactionalChannel]domain.ChannelTemplate{
			domain.TransactionalChannelEmail: {TemplateID: templateID},
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
					Kind:    domain.EmailProviderKindSparkPost,
					Senders: []domain.EmailSender{domain.NewEmailSender("test@example.com", "Test Sender")},
					SparkPost: &domain.SparkPostSettings{
						EncryptedAPIKey: "encrypted-api-key",
					},
				},
			},
		},
	}

	contact := &domain.Contact{Email: "recipient@example.com"}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockTransactionalNotificationRepository(ctrl)
	mockMsgHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
	mockTemplateService := mocks.NewMockTemplateService(ctrl)
	mockContactService := mocks.NewMockContactService(ctrl)
	mockEmailService := mocks.NewMockEmailServiceInterface(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
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

	mockWorkspaceRepo.EXPECT().GetByID(gomock.Any(), workspace).Return(workspaceObj, nil)
	mockRepo.EXPECT().Get(gomock.Any(), workspace, notificationID).Return(notification, nil)
	mockContactService.EXPECT().
		UpsertContact(gomock.Any(), workspace, contact).
		Return(domain.UpsertContactOperation{Email: contact.Email, Action: domain.UpsertContactOperationUpdate})
	mockContactService.EXPECT().
		GetContactByEmail(gomock.Any(), workspace, contact.Email).
		Return(contact, nil)
	mockEmailService.EXPECT().
		SendEmailForTemplate(gomock.Any(), gomock.Any()).
		Return(nil)

	// The context the SMTP bridge and the Supabase webhook build.
	systemCtx := context.WithValue(context.Background(), domain.SystemCallKey, true)

	messageID, err := service.SendNotification(systemCtx, workspace, domain.TransactionalNotificationSendParams{
		ID:      notificationID,
		Contact: contact,
	})

	require.NoError(t, err)
	require.NotEmpty(t, messageID)
}

// TestTransactionalNotificationService_SendOnlyKey pins the send-only key: an API key
// granted transactional:write and nothing else can send, and can send a test template.
// The recipient upsert and lookup are run as system calls so they do not silently make
// contacts:write and contacts:read prerequisites for sending — the ContactService mock
// asserts the context it receives carries SystemCallKey, which is the flag the real
// ContactService checks before its own contacts gate.
func TestTransactionalNotificationService_SendOnlyKey(t *testing.T) {
	const workspaceID = "test-workspace"
	const recipientEmail = "recipient@example.com"

	// No contacts grant at all — the point of the test.
	sendOnly := &domain.UserWorkspace{
		UserID:      "api-key-user",
		WorkspaceID: workspaceID,
		Role:        "member",
		Permissions: domain.UserPermissions{
			domain.PermissionResourceTransactional: {Read: false, Write: true},
		},
	}

	assertSystemScoped := func(t *testing.T, ctx context.Context) {
		t.Helper()
		assert.NotNil(t, ctx.Value(domain.SystemCallKey),
			"nested contact call must be system-scoped, otherwise sending requires contacts permissions")
	}

	t.Run("SendNotification", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		service, m := newSendOnlyTestService(t, ctrl)

		notificationID := uuid.New().String()
		templateID := uuid.New().String()
		contact := &domain.Contact{Email: recipientEmail}

		ctx := context.Background()
		m.authService.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
			Return(ctx, &domain.User{ID: "api-key-user"}, sendOnly, nil)

		m.workspaceRepo.EXPECT().GetByID(gomock.Any(), workspaceID).Return(&domain.Workspace{
			ID: workspaceID,
			Settings: domain.WorkspaceSettings{
				TransactionalEmailProviderID: "integration-1",
				SecretKey:                    "test-secret-key",
			},
			Integrations: []domain.Integration{{
				ID:   "integration-1",
				Type: "email",
				EmailProvider: domain.EmailProvider{
					Kind:      domain.EmailProviderKindSparkPost,
					Senders:   []domain.EmailSender{domain.NewEmailSender("sender@example.com", "Test Sender")},
					SparkPost: &domain.SparkPostSettings{EncryptedAPIKey: "encrypted-api-key"},
				},
			}},
		}, nil)

		m.repo.EXPECT().Get(gomock.Any(), workspaceID, notificationID).Return(&domain.TransactionalNotification{
			ID:   notificationID,
			Name: "Test Notification",
			Channels: map[domain.TransactionalChannel]domain.ChannelTemplate{
				domain.TransactionalChannelEmail: {TemplateID: templateID},
			},
		}, nil)

		m.contactService.EXPECT().
			UpsertContact(gomock.Any(), workspaceID, contact).
			DoAndReturn(func(ctx context.Context, _ string, _ *domain.Contact) domain.UpsertContactOperation {
				assertSystemScoped(t, ctx)
				return domain.UpsertContactOperation{Email: recipientEmail, Action: domain.UpsertContactOperationUpdate}
			})

		m.contactService.EXPECT().
			GetContactByEmail(gomock.Any(), workspaceID, recipientEmail).
			DoAndReturn(func(ctx context.Context, _ string, _ string) (*domain.Contact, error) {
				assertSystemScoped(t, ctx)
				return contact, nil
			})

		m.emailService.EXPECT().SendEmailForTemplate(gomock.Any(), gomock.Any()).Return(nil)

		messageID, err := service.SendNotification(ctx, workspaceID, domain.TransactionalNotificationSendParams{
			ID:      notificationID,
			Contact: contact,
		})

		require.NoError(t, err)
		require.NotEmpty(t, messageID)
	})

	t.Run("TestTemplate", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		service, m := newSendOnlyTestService(t, ctrl)

		templateID := uuid.New().String()
		const integrationID = "integration-1"
		const senderID = "sender-1"

		ctx := context.Background()
		m.authService.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
			Return(ctx, &domain.User{ID: "api-key-user"}, sendOnly, nil)

		m.templateService.EXPECT().
			GetTemplateByID(gomock.Any(), workspaceID, templateID, int64(0)).
			Return(&domain.Template{
				ID:   templateID,
				Name: "Test Template",
				Email: &domain.EmailTemplate{
					Subject: "Test Subject",
					VisualEditorTree: &notifuse_mjml.MJMLBlock{
						BaseBlock: notifuse_mjml.NewBaseBlock("root", notifuse_mjml.MJMLComponentMjml),
					},
				},
			}, nil)

		m.workspaceRepo.EXPECT().GetByID(gomock.Any(), workspaceID).Return(&domain.Workspace{
			ID:       workspaceID,
			Settings: domain.WorkspaceSettings{SecretKey: "test-secret-key"},
			Integrations: []domain.Integration{{
				ID:   integrationID,
				Type: "email",
				EmailProvider: domain.EmailProvider{
					Kind: domain.EmailProviderKindSparkPost,
					Senders: []domain.EmailSender{
						{ID: senderID, Email: "sender@example.com", Name: "Test Sender"},
					},
				},
			}},
		}, nil)

		m.contactService.EXPECT().
			UpsertContact(gomock.Any(), workspaceID, gomock.Any()).
			DoAndReturn(func(ctx context.Context, _ string, _ *domain.Contact) domain.UpsertContactOperation {
				assertSystemScoped(t, ctx)
				return domain.UpsertContactOperation{Email: recipientEmail, Action: domain.UpsertContactOperationUpdate}
			})

		m.contactService.EXPECT().
			GetContactByEmail(gomock.Any(), workspaceID, recipientEmail).
			DoAndReturn(func(ctx context.Context, _ string, _ string) (*domain.Contact, error) {
				assertSystemScoped(t, ctx)
				return &domain.Contact{Email: recipientEmail}, nil
			})

		htmlResult := "<html><body>Test content</body></html>"
		m.templateService.EXPECT().
			CompileTemplate(gomock.Any(), gomock.Any()).
			Return(&domain.CompileTemplateResponse{Success: true, HTML: &htmlResult}, nil)

		m.emailService.EXPECT().SendEmail(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
		m.msgHistoryRepo.EXPECT().Create(gomock.Any(), workspaceID, gomock.Any(), gomock.Any()).Return(nil)

		err := service.TestTemplate(ctx, workspaceID, templateID, integrationID, senderID,
			recipientEmail, "", domain.EmailOptions{})

		require.NoError(t, err)
	})
}

type serviceMocksForSendOnly struct {
	repo            *mocks.MockTransactionalNotificationRepository
	msgHistoryRepo  *mocks.MockMessageHistoryRepository
	templateService *mocks.MockTemplateService
	contactService  *mocks.MockContactService
	emailService    *mocks.MockEmailServiceInterface
	workspaceRepo   *mocks.MockWorkspaceRepository
	authService     *mocks.MockAuthService
}

// newSendOnlyTestService builds a transactional service on fully mocked collaborators
// and a permissive logger. It sets no expectations of its own, so each caller's
// expectations describe exactly the calls its path is allowed to make.
func newSendOnlyTestService(t *testing.T, ctrl *gomock.Controller) (*TransactionalNotificationService, *serviceMocksForSendOnly) {
	t.Helper()

	m := &serviceMocksForSendOnly{
		repo:            mocks.NewMockTransactionalNotificationRepository(ctrl),
		msgHistoryRepo:  mocks.NewMockMessageHistoryRepository(ctrl),
		templateService: mocks.NewMockTemplateService(ctrl),
		contactService:  mocks.NewMockContactService(ctrl),
		emailService:    mocks.NewMockEmailServiceInterface(ctrl),
		workspaceRepo:   mocks.NewMockWorkspaceRepository(ctrl),
		authService:     mocks.NewMockAuthService(ctrl),
	}

	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

	return &TransactionalNotificationService{
		transactionalRepo:  m.repo,
		messageHistoryRepo: m.msgHistoryRepo,
		templateService:    m.templateService,
		contactService:     m.contactService,
		emailService:       m.emailService,
		logger:             mockLogger,
		workspaceRepo:      m.workspaceRepo,
		apiEndpoint:        "https://api.example.com",
		authService:        m.authService,
	}, m
}

// TestTransactionalNotificationService_ContactScopeContainment pins the blast radius of
// the system subcontext the send path runs its recipient upsert and lookup under. That
// subcontext skips the contacts gates by design — it is what lets transactional:write
// alone send — so the two nested calls otherwise reach every field of an arbitrary
// contact record:
//
//   - write: the repository merges every non-nil pointer of the posted body onto an
//     existing record, so an unrelated stored contact could be rewritten wholesale;
//   - read: the full record is put on templateData and the subject is Liquid-evaluated
//     against it, so naming an extra recipient with cc or bcc exfiltrates any field,
//     the notification_center_url HMAC included.
//
// SendNotification therefore carries the caller's contacts grants across the context
// switch. A genuine system caller keeps both, so nothing on that path changes.
func TestTransactionalNotificationService_ContactScopeContainment(t *testing.T) {
	const workspaceID = "test-workspace"
	const recipientEmail = "victim@customer.com"

	sendOnly := &domain.UserWorkspace{
		UserID:      "api-key-user",
		WorkspaceID: workspaceID,
		Role:        "member",
		Permissions: domain.UserPermissions{
			domain.PermissionResourceTransactional: {Write: true},
		},
	}

	sendAndContacts := &domain.UserWorkspace{
		UserID:      "api-key-user",
		WorkspaceID: workspaceID,
		Role:        "member",
		Permissions: domain.UserPermissions{
			domain.PermissionResourceTransactional: {Write: true},
			domain.PermissionResourceContacts:      {Read: true, Write: true},
		},
	}

	// The body a caller posts. Every field beyond the email would be merged onto the
	// stored record for that address.
	newPostedContact := func() *domain.Contact {
		return &domain.Contact{
			Email:         recipientEmail,
			FirstName:     &domain.NullableString{String: "Overwritten"},
			Phone:         &domain.NullableString{String: "+15550000000"},
			CustomString1: &domain.NullableString{String: "injected"},
		}
	}

	notificationID := uuid.New().String()
	templateID := uuid.New().String()

	// expectSendPath sets up everything the send needs either side of the contact calls,
	// so a subtest only has to describe the contact calls it expects.
	expectSendPath := func(m *serviceMocksForSendOnly) {
		m.workspaceRepo.EXPECT().GetByID(gomock.Any(), workspaceID).Return(&domain.Workspace{
			ID: workspaceID,
			Settings: domain.WorkspaceSettings{
				TransactionalEmailProviderID: "integration-1",
				SecretKey:                    "test-secret-key",
			},
			Integrations: []domain.Integration{{
				ID:   "integration-1",
				Type: "email",
				EmailProvider: domain.EmailProvider{
					Kind:      domain.EmailProviderKindSparkPost,
					Senders:   []domain.EmailSender{domain.NewEmailSender("sender@example.com", "Test Sender")},
					SparkPost: &domain.SparkPostSettings{EncryptedAPIKey: "encrypted-api-key"},
				},
			}},
		}, nil)

		m.repo.EXPECT().Get(gomock.Any(), workspaceID, notificationID).Return(&domain.TransactionalNotification{
			ID:   notificationID,
			Name: "Password reset",
			Channels: map[domain.TransactionalChannel]domain.ChannelTemplate{
				domain.TransactionalChannelEmail: {TemplateID: templateID},
			},
		}, nil)

		m.emailService.EXPECT().SendEmailForTemplate(gomock.Any(), gomock.Any()).Return(nil)
	}

	// expectContactCalls records the contact the upsert actually received.
	expectContactCalls := func(m *serviceMocksForSendOnly, upserted **domain.Contact) {
		m.contactService.EXPECT().
			UpsertContact(gomock.Any(), workspaceID, gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, contact *domain.Contact) domain.UpsertContactOperation {
				*upserted = contact
				return domain.UpsertContactOperation{Email: recipientEmail, Action: domain.UpsertContactOperationUpdate}
			})

		m.contactService.EXPECT().
			GetContactByEmail(gomock.Any(), workspaceID, recipientEmail).
			Return(&domain.Contact{Email: recipientEmail}, nil)
	}

	t.Run("send-only key upserts the email and nothing else", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		service, m := newSendOnlyTestService(t, ctrl)

		ctx := context.Background()
		m.authService.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
			Return(ctx, &domain.User{ID: "api-key-user"}, sendOnly, nil)

		expectSendPath(m)

		var upserted *domain.Contact
		expectContactCalls(m, &upserted)

		messageID, err := service.SendNotification(ctx, workspaceID, domain.TransactionalNotificationSendParams{
			ID:      notificationID,
			Contact: newPostedContact(),
		})
		require.NoError(t, err)
		require.NotEmpty(t, messageID)

		// Merge copies only the non-nil pointers, so leaving them nil is what stops the
		// send from rewriting an existing contact's fields.
		require.NotNil(t, upserted)
		assert.Equal(t, recipientEmail, upserted.Email)
		assert.Nil(t, upserted.FirstName, "send-only key must not carry first_name into the stored contact")
		assert.Nil(t, upserted.Phone, "send-only key must not carry phone into the stored contact")
		assert.Nil(t, upserted.CustomString1, "send-only key must not carry custom fields into the stored contact")
	})

	t.Run("send-only key cannot name an extra recipient", func(t *testing.T) {
		extraRecipients := map[string]domain.EmailOptions{
			"bcc": {BCC: []string{"attacker@evil.example"}},
			"cc":  {CC: []string{"attacker@evil.example"}},
		}

		for name, emailOptions := range extraRecipients {
			t.Run(name, func(t *testing.T) {
				ctrl := gomock.NewController(t)
				defer ctrl.Finish()

				service, m := newSendOnlyTestService(t, ctrl)

				ctx := context.Background()
				m.authService.EXPECT().
					AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
					Return(ctx, &domain.User{ID: "api-key-user"}, sendOnly, nil)

				// No other expectation: the refusal must land before the workspace is
				// even fetched, so gomock fails the test if anything else is touched.
				_, err := service.SendNotification(ctx, workspaceID, domain.TransactionalNotificationSendParams{
					ID:           notificationID,
					Contact:      &domain.Contact{Email: recipientEmail},
					EmailOptions: emailOptions,
				})

				require.Error(t, err)
				var permErr *domain.PermissionError
				require.ErrorAs(t, err, &permErr)
				assert.Equal(t, domain.PermissionResourceContacts, permErr.Resource)
				assert.Equal(t, domain.PermissionTypeRead, permErr.Permission)
			})
		}
	})

	t.Run("key holding contacts read and write keeps both", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		service, m := newSendOnlyTestService(t, ctrl)

		ctx := context.Background()
		m.authService.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
			Return(ctx, &domain.User{ID: "api-key-user"}, sendAndContacts, nil)

		expectSendPath(m)

		var upserted *domain.Contact
		expectContactCalls(m, &upserted)

		posted := newPostedContact()
		messageID, err := service.SendNotification(ctx, workspaceID, domain.TransactionalNotificationSendParams{
			ID:           notificationID,
			Contact:      posted,
			EmailOptions: domain.EmailOptions{BCC: []string{"archive@customer.com"}},
		})
		require.NoError(t, err)
		require.NotEmpty(t, messageID)

		assert.Equal(t, posted, upserted, "a key granted contacts:write posts the body verbatim")
	})

	t.Run("system call is unaffected", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		service, m := newSendOnlyTestService(t, ctrl)

		expectSendPath(m)

		var upserted *domain.Contact
		expectContactCalls(m, &upserted)

		// The context Supabase and any other system caller build. The auth service mock
		// carries no expectation, so gomock fails if the bypass is lost.
		systemCtx := context.WithValue(context.Background(), domain.SystemCallKey, true)

		posted := newPostedContact()
		messageID, err := service.SendNotification(systemCtx, workspaceID, domain.TransactionalNotificationSendParams{
			ID:           notificationID,
			Contact:      posted,
			EmailOptions: domain.EmailOptions{BCC: []string{"archive@customer.com"}},
		})
		require.NoError(t, err)
		require.NotEmpty(t, messageID)

		assert.Equal(t, posted, upserted, "a system call still posts the body verbatim")
	})

	// TestTemplate makes the same nested, system-scoped calls. Its upsert already carries
	// nothing but the email, so only the read side needs closing.
	t.Run("TestTemplate refuses an extra recipient for a send-only key", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		service, m := newSendOnlyTestService(t, ctrl)

		ctx := context.Background()
		m.authService.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
			Return(ctx, &domain.User{ID: "api-key-user"}, sendOnly, nil)

		// No other expectation: the refusal lands before the template is fetched.
		err := service.TestTemplate(ctx, workspaceID, templateID, "integration-1", "sender-1",
			recipientEmail, "", domain.EmailOptions{BCC: []string{"attacker@evil.example"}})

		require.Error(t, err)
		var permErr *domain.PermissionError
		require.ErrorAs(t, err, &permErr)
		assert.Equal(t, domain.PermissionResourceContacts, permErr.Resource)
		assert.Equal(t, domain.PermissionTypeRead, permErr.Permission)
	})

	t.Run("TestTemplate allows an extra recipient with contacts read", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		service, m := newSendOnlyTestService(t, ctrl)

		const integrationID = "integration-1"
		const senderID = "sender-1"

		ctx := context.Background()
		m.authService.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
			Return(ctx, &domain.User{ID: "api-key-user"}, sendAndContacts, nil)

		m.templateService.EXPECT().
			GetTemplateByID(gomock.Any(), workspaceID, templateID, int64(0)).
			Return(&domain.Template{
				ID:   templateID,
				Name: "Test Template",
				Email: &domain.EmailTemplate{
					Subject: "Test Subject",
					VisualEditorTree: &notifuse_mjml.MJMLBlock{
						BaseBlock: notifuse_mjml.NewBaseBlock("root", notifuse_mjml.MJMLComponentMjml),
					},
				},
			}, nil)

		m.workspaceRepo.EXPECT().GetByID(gomock.Any(), workspaceID).Return(&domain.Workspace{
			ID:       workspaceID,
			Settings: domain.WorkspaceSettings{SecretKey: "test-secret-key"},
			Integrations: []domain.Integration{{
				ID:   integrationID,
				Type: "email",
				EmailProvider: domain.EmailProvider{
					Kind: domain.EmailProviderKindSparkPost,
					Senders: []domain.EmailSender{
						{ID: senderID, Email: "sender@example.com", Name: "Test Sender"},
					},
				},
			}},
		}, nil)

		m.contactService.EXPECT().
			UpsertContact(gomock.Any(), workspaceID, gomock.Any()).
			Return(domain.UpsertContactOperation{Email: recipientEmail, Action: domain.UpsertContactOperationUpdate})
		m.contactService.EXPECT().
			GetContactByEmail(gomock.Any(), workspaceID, recipientEmail).
			Return(&domain.Contact{Email: recipientEmail}, nil)

		htmlResult := "<html><body>Test content</body></html>"
		m.templateService.EXPECT().
			CompileTemplate(gomock.Any(), gomock.Any()).
			Return(&domain.CompileTemplateResponse{Success: true, HTML: &htmlResult}, nil)
		m.emailService.EXPECT().SendEmail(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
		m.msgHistoryRepo.EXPECT().Create(gomock.Any(), workspaceID, gomock.Any(), gomock.Any()).Return(nil)

		err := service.TestTemplate(ctx, workspaceID, templateID, integrationID, senderID,
			recipientEmail, "", domain.EmailOptions{BCC: []string{"archive@customer.com"}})

		require.NoError(t, err)
	})
}

// An update body is a patch, so tracking_settings has to tell a body that never
// mentioned it from one that explicitly emptied it. Both decode to a zero-valued
// struct, and reading the zero as "absent" turns switching tracking off — or
// stripping the UTMs a finished campaign left behind — into a no-op that still
// answers 200, with nothing in the response to say so. These notifications carry
// password resets and magic links, whose links keep being rewritten with the old
// campaign until someone notices.
//
// Raw JSON bodies throughout: a Go struct literal cannot express a key that was
// never sent, which is the whole distinction under test.
func TestTransactionalNotificationService_UpdateNotification_TrackingSettingsPresence(t *testing.T) {
	ctx := context.Background()
	workspace := "test-workspace"
	notificationID := uuid.New().String()

	// Seeded richly: against an empty stored value a wipe and a preserve look the
	// same.
	newStoredNotification := func() *domain.TransactionalNotification {
		return &domain.TransactionalNotification{
			ID:          notificationID,
			Name:        "Password Reset",
			Description: "Original Description",
			TrackingSettings: notifuse_mjml.TrackingSettings{
				EnableTracking: true,
				TrackingMode:   notifuse_mjml.TrackingModeDisabled,
				UTMSource:      "newsletter",
				UTMCampaign:    "spring-sale",
			},
		}
	}

	tests := []struct {
		name     string
		body     string
		expected notifuse_mjml.TrackingSettings
	}{
		{
			// tracking_mode is absent from the object, so the tri-state rule still
			// keeps the stored opt-out; "inherit" is how that one is reset.
			name: "an explicitly disabled object switches tracking off",
			body: `{"workspace_id":"test-workspace","id":"%s","updates":{"name":"Renamed","tracking_settings":{"enable_tracking":false}}}`,
			expected: notifuse_mjml.TrackingSettings{
				TrackingMode: notifuse_mjml.TrackingModeDisabled,
			},
		},
		{
			name: "an empty object strips the stored utms",
			body: `{"workspace_id":"test-workspace","id":"%s","updates":{"name":"Renamed","tracking_settings":{}}}`,
			expected: notifuse_mjml.TrackingSettings{
				TrackingMode: notifuse_mjml.TrackingModeDisabled,
			},
		},
		{
			name: "an absent key leaves the stored settings alone",
			body: `{"workspace_id":"test-workspace","id":"%s","updates":{"name":"Renamed"}}`,
			expected: notifuse_mjml.TrackingSettings{
				EnableTracking: true,
				TrackingMode:   notifuse_mjml.TrackingModeDisabled,
				UTMSource:      "newsletter",
				UTMCampaign:    "spring-sale",
			},
		},
		{
			// Nothing else in the body: the request has to count as an update on the
			// strength of the tracking settings alone.
			name: "a tracking-settings-only body is a valid update",
			body: `{"workspace_id":"test-workspace","id":"%s","updates":{"tracking_settings":{"enable_tracking":false}}}`,
			expected: notifuse_mjml.TrackingSettings{
				TrackingMode: notifuse_mjml.TrackingModeDisabled,
			},
		},
		{
			name: "an explicit inherit still clears the stored opt-out",
			body: `{"workspace_id":"test-workspace","id":"%s","updates":{"tracking_settings":{"enable_tracking":true,"tracking_mode":"inherit","utm_source":"welcome"}}}`,
			expected: notifuse_mjml.TrackingSettings{
				EnableTracking: true,
				UTMSource:      "welcome",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockRepo := mocks.NewMockTransactionalNotificationRepository(ctrl)
			mockAuthService := mocks.NewMockAuthService(ctrl)
			mockLogger := pkgmocks.NewMockLogger(ctrl)
			mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
			mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
			mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
			mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
			mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

			var req domain.UpdateTransactionalRequest
			require.NoError(t, json.Unmarshal([]byte(fmt.Sprintf(tc.body, notificationID)), &req))
			require.NoError(t, req.Validate())

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
				Return(newStoredNotification(), nil)

			// Asserted against the case's own literal rather than the fixture: the
			// service mutates the very notification the repository handed it.
			mockRepo.EXPECT().
				Update(gomock.Any(), workspace, gomock.Any()).
				DoAndReturn(func(_ context.Context, _ string, notif *domain.TransactionalNotification) error {
					assert.Equal(t, tc.expected, notif.TrackingSettings)
					assert.Equal(t, "Original Description", notif.Description,
						"a field the body never mentioned still stands")
					return nil
				})

			service := &TransactionalNotificationService{
				transactionalRepo: mockRepo,
				logger:            mockLogger,
				apiEndpoint:       "https://api.example.com",
				authService:       mockAuthService,
			}

			result, err := service.UpdateNotification(ctx, workspace, req.ID, req.Updates)
			require.NoError(t, err)
			require.NotNil(t, result)
			assert.Equal(t, tc.expected, result.TrackingSettings)
		})
	}
}
