package service

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupWebhookSubscriptionTest(t *testing.T) (
	*mocks.MockWebhookSubscriptionRepository,
	*mocks.MockWebhookDeliveryRepository,
	*pkgmocks.MockLogger,
	*WebhookSubscriptionService,
	*gomock.Controller,
) {
	mockRepo, mockDeliveryRepo, mockLogger, _, service, ctrl := setupWebhookSubscriptionTestWithAuth(t)
	return mockRepo, mockDeliveryRepo, mockLogger, service, ctrl
}

// setupWebhookSubscriptionTestWithAuth also exposes the auth mock, for the tests
// that care who is asking. The plain harness above arms it permissively, so the
// existing behaviour tests stay about behaviour.
func setupWebhookSubscriptionTestWithAuth(t *testing.T) (
	*mocks.MockWebhookSubscriptionRepository,
	*mocks.MockWebhookDeliveryRepository,
	*pkgmocks.MockLogger,
	*mocks.MockAuthService,
	*WebhookSubscriptionService,
	*gomock.Controller,
) {
	ctrl := gomock.NewController(t)
	mockRepo := mocks.NewMockWebhookSubscriptionRepository(ctrl)
	mockDeliveryRepo := mocks.NewMockWebhookDeliveryRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	// Set up logger expectations - these can be called any number of times
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

	mockAuthService := mocks.NewMockAuthService(ctrl)
	mockAuthService.EXPECT().
		AuthenticateUserForWorkspace(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, workspaceID string) (context.Context, *domain.User, *domain.UserWorkspace, error) {
			return ctx, &domain.User{ID: "user-1"}, &domain.UserWorkspace{
				UserID:      "user-1",
				WorkspaceID: workspaceID,
				Role:        "owner",
			}, nil
		}).AnyTimes()

	service := NewWebhookSubscriptionService(mockRepo, mockDeliveryRepo, mockAuthService, mockLogger)

	return mockRepo, mockDeliveryRepo, mockLogger, mockAuthService, service, ctrl
}

func TestNewWebhookSubscriptionService(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockWebhookSubscriptionRepository(ctrl)
	mockDeliveryRepo := mocks.NewMockWebhookDeliveryRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	mockAuthService := mocks.NewMockAuthService(ctrl)
	service := NewWebhookSubscriptionService(mockRepo, mockDeliveryRepo, mockAuthService, mockLogger)

	require.NotNil(t, service)
	require.Equal(t, mockRepo, service.repo)
	require.Equal(t, mockDeliveryRepo, service.deliveryRepo)
	require.Equal(t, mockAuthService, service.authService)
	require.Equal(t, mockLogger, service.logger)
}

func TestWebhookSubscriptionService_Create(t *testing.T) {
	testCases := []struct {
		name               string
		workspaceID        string
		webhookName        string
		webhookURL         string
		eventTypes         []string
		customEventFilters *domain.CustomEventFilters
		setupMocks         func(*mocks.MockWebhookSubscriptionRepository)
		expectError        bool
		validateResult     func(*testing.T, *domain.WebhookSubscription)
	}{
		{
			name:        "successful creation",
			workspaceID: "workspace123",
			webhookName: "Test Webhook",
			webhookURL:  "https://example.com/webhook",
			eventTypes:  []string{"contact.created", "contact.updated"},
			setupMocks: func(mockRepo *mocks.MockWebhookSubscriptionRepository) {
				mockRepo.EXPECT().
					Create(gomock.Any(), "workspace123", gomock.Any()).
					DoAndReturn(func(ctx context.Context, workspaceID string, sub *domain.WebhookSubscription) error {
						// Verify that subscription has all required fields
						require.NotEmpty(t, sub.ID)
						require.Len(t, sub.ID, 32)
						require.Equal(t, "Test Webhook", sub.Name)
						require.Equal(t, "https://example.com/webhook", sub.URL)
						require.NotEmpty(t, sub.Secret)
						require.True(t, sub.Enabled)
						require.Equal(t, []string{"contact.created", "contact.updated"}, sub.Settings.EventTypes)
						return nil
					})
			},
			expectError: false,
			validateResult: func(t *testing.T, sub *domain.WebhookSubscription) {
				require.NotNil(t, sub)
				require.NotEmpty(t, sub.ID)
				require.NotEmpty(t, sub.Secret)
				require.True(t, sub.Enabled)
			},
		},
		{
			name:        "with custom event filters",
			workspaceID: "workspace123",
			webhookName: "Custom Event Webhook",
			webhookURL:  "https://example.com/webhook",
			eventTypes:  []string{"custom_event.created"},
			customEventFilters: &domain.CustomEventFilters{
				GoalTypes:  []string{"conversion"},
				EventNames: []string{"purchase", "signup"},
			},
			setupMocks: func(mockRepo *mocks.MockWebhookSubscriptionRepository) {
				mockRepo.EXPECT().
					Create(gomock.Any(), "workspace123", gomock.Any()).
					DoAndReturn(func(ctx context.Context, workspaceID string, sub *domain.WebhookSubscription) error {
						require.NotNil(t, sub.Settings.CustomEventFilters)
						require.Equal(t, []string{"conversion"}, sub.Settings.CustomEventFilters.GoalTypes)
						require.Equal(t, []string{"purchase", "signup"}, sub.Settings.CustomEventFilters.EventNames)
						return nil
					})
			},
			expectError: false,
			validateResult: func(t *testing.T, sub *domain.WebhookSubscription) {
				require.NotNil(t, sub)
				require.NotNil(t, sub.Settings.CustomEventFilters)
			},
		},
		{
			name:        "empty name error",
			workspaceID: "workspace123",
			webhookName: "",
			webhookURL:  "https://example.com/webhook",
			eventTypes:  []string{"contact.created"},
			setupMocks:  func(mockRepo *mocks.MockWebhookSubscriptionRepository) {},
			expectError: true,
		},
		{
			name:        "empty URL error",
			workspaceID: "workspace123",
			webhookName: "Test Webhook",
			webhookURL:  "",
			eventTypes:  []string{"contact.created"},
			setupMocks:  func(mockRepo *mocks.MockWebhookSubscriptionRepository) {},
			expectError: true,
		},
		{
			name:        "invalid URL scheme",
			workspaceID: "workspace123",
			webhookName: "Test Webhook",
			webhookURL:  "ftp://example.com/webhook",
			eventTypes:  []string{"contact.created"},
			setupMocks:  func(mockRepo *mocks.MockWebhookSubscriptionRepository) {},
			expectError: true,
		},
		{
			name:        "URL without host",
			workspaceID: "workspace123",
			webhookName: "Test Webhook",
			webhookURL:  "https://",
			eventTypes:  []string{"contact.created"},
			setupMocks:  func(mockRepo *mocks.MockWebhookSubscriptionRepository) {},
			expectError: true,
		},
		{
			name:        "malformed URL",
			workspaceID: "workspace123",
			webhookName: "Test Webhook",
			webhookURL:  "not a url",
			eventTypes:  []string{"contact.created"},
			setupMocks:  func(mockRepo *mocks.MockWebhookSubscriptionRepository) {},
			expectError: true,
		},
		{
			name:        "empty event types",
			workspaceID: "workspace123",
			webhookName: "Test Webhook",
			webhookURL:  "https://example.com/webhook",
			eventTypes:  []string{},
			setupMocks:  func(mockRepo *mocks.MockWebhookSubscriptionRepository) {},
			expectError: true,
		},
		{
			name:        "invalid event type",
			workspaceID: "workspace123",
			webhookName: "Test Webhook",
			webhookURL:  "https://example.com/webhook",
			eventTypes:  []string{"contact.created", "invalid.event"},
			setupMocks:  func(mockRepo *mocks.MockWebhookSubscriptionRepository) {},
			expectError: true,
		},
		{
			name:        "repository error",
			workspaceID: "workspace123",
			webhookName: "Test Webhook",
			webhookURL:  "https://example.com/webhook",
			eventTypes:  []string{"contact.created"},
			setupMocks: func(mockRepo *mocks.MockWebhookSubscriptionRepository) {
				mockRepo.EXPECT().
					Create(gomock.Any(), "workspace123", gomock.Any()).
					Return(errors.New("database error"))
			},
			expectError: true,
		},
		{
			name:        "http URL is allowed",
			workspaceID: "workspace123",
			webhookName: "Test Webhook",
			webhookURL:  "http://example.com/webhook",
			eventTypes:  []string{"contact.created"},
			setupMocks: func(mockRepo *mocks.MockWebhookSubscriptionRepository) {
				mockRepo.EXPECT().
					Create(gomock.Any(), "workspace123", gomock.Any()).
					Return(nil)
			},
			expectError: false,
			validateResult: func(t *testing.T, sub *domain.WebhookSubscription) {
				require.NotNil(t, sub)
				require.Equal(t, "http://example.com/webhook", sub.URL)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockRepo, _, _, service, ctrl := setupWebhookSubscriptionTest(t)
			defer ctrl.Finish()

			tc.setupMocks(mockRepo)

			result, err := service.Create(
				context.Background(),
				tc.workspaceID,
				tc.webhookName,
				tc.webhookURL,
				tc.eventTypes,
				tc.customEventFilters,
				domain.WebhookSubscriptionSourceUser,
				nil,
				nil,
			)

			if tc.expectError {
				require.Error(t, err)
				require.Nil(t, result)
			} else {
				require.NoError(t, err)
				if tc.validateResult != nil {
					tc.validateResult(t, result)
				}
			}
		})
	}
}

func TestWebhookSubscriptionService_GetByID(t *testing.T) {
	testCases := []struct {
		name           string
		workspaceID    string
		subID          string
		setupMocks     func(*mocks.MockWebhookSubscriptionRepository)
		expectError    bool
		validateResult func(*testing.T, *domain.WebhookSubscription)
	}{
		{
			name:        "successful retrieval",
			workspaceID: "workspace123",
			subID:       "sub123",
			setupMocks: func(mockRepo *mocks.MockWebhookSubscriptionRepository) {
				mockRepo.EXPECT().
					GetByID(gomock.Any(), "workspace123", "sub123").
					Return(&domain.WebhookSubscription{
						ID:      "sub123",
						Name:    "Test Webhook",
						URL:     "https://example.com/webhook",
						Enabled: true,
					}, nil)
			},
			expectError: false,
			validateResult: func(t *testing.T, sub *domain.WebhookSubscription) {
				require.NotNil(t, sub)
				require.Equal(t, "sub123", sub.ID)
				require.Equal(t, "Test Webhook", sub.Name)
			},
		},
		{
			name:        "not found error",
			workspaceID: "workspace123",
			subID:       "nonexistent",
			setupMocks: func(mockRepo *mocks.MockWebhookSubscriptionRepository) {
				mockRepo.EXPECT().
					GetByID(gomock.Any(), "workspace123", "nonexistent").
					Return(nil, errors.New("not found"))
			},
			expectError: true,
		},
		{
			name:        "repository error",
			workspaceID: "workspace123",
			subID:       "sub123",
			setupMocks: func(mockRepo *mocks.MockWebhookSubscriptionRepository) {
				mockRepo.EXPECT().
					GetByID(gomock.Any(), "workspace123", "sub123").
					Return(nil, errors.New("database error"))
			},
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockRepo, _, _, service, ctrl := setupWebhookSubscriptionTest(t)
			defer ctrl.Finish()

			tc.setupMocks(mockRepo)

			result, err := service.GetByID(context.Background(), tc.workspaceID, tc.subID)

			if tc.expectError {
				require.Error(t, err)
				require.Nil(t, result)
			} else {
				require.NoError(t, err)
				if tc.validateResult != nil {
					tc.validateResult(t, result)
				}
			}
		})
	}
}

func TestWebhookSubscriptionService_List(t *testing.T) {
	testCases := []struct {
		name        string
		workspaceID string
		setupMocks  func(*mocks.MockWebhookSubscriptionRepository)
		expectError bool
		expectedLen int
	}{
		{
			name:        "successful list with multiple subscriptions",
			workspaceID: "workspace123",
			setupMocks: func(mockRepo *mocks.MockWebhookSubscriptionRepository) {
				mockRepo.EXPECT().
					List(gomock.Any(), "workspace123").
					Return([]*domain.WebhookSubscription{
						{ID: "sub1", Name: "Webhook 1"},
						{ID: "sub2", Name: "Webhook 2"},
						{ID: "sub3", Name: "Webhook 3"},
					}, nil)
			},
			expectError: false,
			expectedLen: 3,
		},
		{
			name:        "empty list",
			workspaceID: "workspace123",
			setupMocks: func(mockRepo *mocks.MockWebhookSubscriptionRepository) {
				mockRepo.EXPECT().
					List(gomock.Any(), "workspace123").
					Return([]*domain.WebhookSubscription{}, nil)
			},
			expectError: false,
			expectedLen: 0,
		},
		{
			name:        "repository error",
			workspaceID: "workspace123",
			setupMocks: func(mockRepo *mocks.MockWebhookSubscriptionRepository) {
				mockRepo.EXPECT().
					List(gomock.Any(), "workspace123").
					Return(nil, errors.New("database error"))
			},
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockRepo, _, _, service, ctrl := setupWebhookSubscriptionTest(t)
			defer ctrl.Finish()

			tc.setupMocks(mockRepo)

			result, err := service.List(context.Background(), tc.workspaceID)

			if tc.expectError {
				require.Error(t, err)
				require.Nil(t, result)
			} else {
				require.NoError(t, err)
				require.Len(t, result, tc.expectedLen)
			}
		})
	}
}

func TestWebhookSubscriptionService_Update(t *testing.T) {
	testCases := []struct {
		name               string
		workspaceID        string
		subID              string
		webhookName        string
		webhookURL         string
		eventTypes         []string
		customEventFilters *domain.CustomEventFilters
		enabled            *bool
		setupMocks         func(*mocks.MockWebhookSubscriptionRepository)
		expectError        bool
		validateResult     func(*testing.T, *domain.WebhookSubscription)
	}{
		{
			name:        "successful update",
			workspaceID: "workspace123",
			subID:       "sub123",
			webhookName: "Updated Webhook",
			webhookURL:  "https://updated.example.com/webhook",
			eventTypes:  []string{"contact.updated", "contact.deleted"},
			enabled:     boolPtr(true),
			setupMocks: func(mockRepo *mocks.MockWebhookSubscriptionRepository) {
				mockRepo.EXPECT().
					GetByID(gomock.Any(), "workspace123", "sub123").
					Return(&domain.WebhookSubscription{
						ID:      "sub123",
						Name:    "Old Name",
						URL:     "https://old.example.com",
						Enabled: false,
					}, nil)

				mockRepo.EXPECT().
					Update(gomock.Any(), "workspace123", gomock.Any()).
					DoAndReturn(func(ctx context.Context, workspaceID string, sub *domain.WebhookSubscription) error {
						require.Equal(t, "Updated Webhook", sub.Name)
						require.Equal(t, "https://updated.example.com/webhook", sub.URL)
						require.True(t, sub.Enabled)
						require.Equal(t, []string{"contact.updated", "contact.deleted"}, sub.Settings.EventTypes)
						return nil
					})
			},
			expectError: false,
			validateResult: func(t *testing.T, sub *domain.WebhookSubscription) {
				require.NotNil(t, sub)
				require.Equal(t, "Updated Webhook", sub.Name)
				require.True(t, sub.Enabled)
			},
		},
		{
			name:        "disable webhook",
			workspaceID: "workspace123",
			subID:       "sub123",
			webhookName: "Webhook",
			webhookURL:  "https://example.com/webhook",
			eventTypes:  []string{"contact.created"},
			enabled:     boolPtr(false),
			setupMocks: func(mockRepo *mocks.MockWebhookSubscriptionRepository) {
				mockRepo.EXPECT().
					GetByID(gomock.Any(), "workspace123", "sub123").
					Return(&domain.WebhookSubscription{
						ID:      "sub123",
						Enabled: true,
					}, nil)

				mockRepo.EXPECT().
					Update(gomock.Any(), "workspace123", gomock.Any()).
					DoAndReturn(func(ctx context.Context, workspaceID string, sub *domain.WebhookSubscription) error {
						require.False(t, sub.Enabled)
						return nil
					})
			},
			expectError: false,
			validateResult: func(t *testing.T, sub *domain.WebhookSubscription) {
				require.False(t, sub.Enabled)
			},
		},
		{
			name:        "subscription not found",
			workspaceID: "workspace123",
			subID:       "nonexistent",
			webhookName: "Webhook",
			webhookURL:  "https://example.com/webhook",
			eventTypes:  []string{"contact.created"},
			setupMocks: func(mockRepo *mocks.MockWebhookSubscriptionRepository) {
				mockRepo.EXPECT().
					GetByID(gomock.Any(), "workspace123", "nonexistent").
					Return(nil, errors.New("not found"))
			},
			expectError: true,
		},
		{
			name:        "empty name validation error",
			workspaceID: "workspace123",
			subID:       "sub123",
			webhookName: "",
			webhookURL:  "https://example.com/webhook",
			eventTypes:  []string{"contact.created"},
			setupMocks: func(mockRepo *mocks.MockWebhookSubscriptionRepository) {
				mockRepo.EXPECT().
					GetByID(gomock.Any(), "workspace123", "sub123").
					Return(&domain.WebhookSubscription{ID: "sub123"}, nil)
			},
			expectError: true,
		},
		{
			name:        "invalid URL validation error",
			workspaceID: "workspace123",
			subID:       "sub123",
			webhookName: "Webhook",
			webhookURL:  "invalid-url",
			eventTypes:  []string{"contact.created"},
			setupMocks: func(mockRepo *mocks.MockWebhookSubscriptionRepository) {
				mockRepo.EXPECT().
					GetByID(gomock.Any(), "workspace123", "sub123").
					Return(&domain.WebhookSubscription{ID: "sub123"}, nil)
			},
			expectError: true,
		},
		{
			name:        "invalid event types validation error",
			workspaceID: "workspace123",
			subID:       "sub123",
			webhookName: "Webhook",
			webhookURL:  "https://example.com/webhook",
			eventTypes:  []string{"invalid.event"},
			setupMocks: func(mockRepo *mocks.MockWebhookSubscriptionRepository) {
				mockRepo.EXPECT().
					GetByID(gomock.Any(), "workspace123", "sub123").
					Return(&domain.WebhookSubscription{ID: "sub123"}, nil)
			},
			expectError: true,
		},
		{
			name:        "update repository error",
			workspaceID: "workspace123",
			subID:       "sub123",
			webhookName: "Webhook",
			webhookURL:  "https://example.com/webhook",
			eventTypes:  []string{"contact.created"},
			setupMocks: func(mockRepo *mocks.MockWebhookSubscriptionRepository) {
				mockRepo.EXPECT().
					GetByID(gomock.Any(), "workspace123", "sub123").
					Return(&domain.WebhookSubscription{ID: "sub123"}, nil)

				mockRepo.EXPECT().
					Update(gomock.Any(), "workspace123", gomock.Any()).
					Return(errors.New("database error"))
			},
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockRepo, _, _, service, ctrl := setupWebhookSubscriptionTest(t)
			defer ctrl.Finish()

			tc.setupMocks(mockRepo)

			result, err := service.Update(
				context.Background(),
				tc.workspaceID,
				tc.subID,
				tc.webhookName,
				tc.webhookURL,
				tc.eventTypes,
				tc.customEventFilters,
				tc.enabled,
				nil,
				nil,
			)

			if tc.expectError {
				require.Error(t, err)
				require.Nil(t, result)
			} else {
				require.NoError(t, err)
				if tc.validateResult != nil {
					tc.validateResult(t, result)
				}
			}
		})
	}
}

func TestWebhookSubscriptionService_Delete(t *testing.T) {
	testCases := []struct {
		name        string
		workspaceID string
		subID       string
		setupMocks  func(*mocks.MockWebhookSubscriptionRepository, *mocks.MockWebhookDeliveryRepository)
		expectError bool
	}{
		{
			name:        "successful deletion",
			workspaceID: "workspace123",
			subID:       "sub123",
			setupMocks: func(mockRepo *mocks.MockWebhookSubscriptionRepository, mockDeliveryRepo *mocks.MockWebhookDeliveryRepository) {
				mockDeliveryRepo.EXPECT().
					DeleteBySubscriptionID(gomock.Any(), "workspace123", "sub123").
					Return(nil)
				mockRepo.EXPECT().
					Delete(gomock.Any(), "workspace123", "sub123").
					Return(nil)
			},
			expectError: false,
		},
		{
			name:        "subscription not found",
			workspaceID: "workspace123",
			subID:       "nonexistent",
			setupMocks: func(mockRepo *mocks.MockWebhookSubscriptionRepository, mockDeliveryRepo *mocks.MockWebhookDeliveryRepository) {
				mockDeliveryRepo.EXPECT().
					DeleteBySubscriptionID(gomock.Any(), "workspace123", "nonexistent").
					Return(nil)
				mockRepo.EXPECT().
					Delete(gomock.Any(), "workspace123", "nonexistent").
					Return(errors.New("not found"))
			},
			expectError: true,
		},
		{
			name:        "repository error",
			workspaceID: "workspace123",
			subID:       "sub123",
			setupMocks: func(mockRepo *mocks.MockWebhookSubscriptionRepository, mockDeliveryRepo *mocks.MockWebhookDeliveryRepository) {
				mockDeliveryRepo.EXPECT().
					DeleteBySubscriptionID(gomock.Any(), "workspace123", "sub123").
					Return(nil)
				mockRepo.EXPECT().
					Delete(gomock.Any(), "workspace123", "sub123").
					Return(errors.New("database error"))
			},
			expectError: true,
		},
		{
			// The subscription row has to survive a failed sweep, or the caller
			// is left with a live subscription whose queue is half gone and no
			// error to act on.
			name:        "delivery sweep failure aborts the delete",
			workspaceID: "workspace123",
			subID:       "sub123",
			setupMocks: func(mockRepo *mocks.MockWebhookSubscriptionRepository, mockDeliveryRepo *mocks.MockWebhookDeliveryRepository) {
				mockDeliveryRepo.EXPECT().
					DeleteBySubscriptionID(gomock.Any(), "workspace123", "sub123").
					Return(errors.New("database error"))
				// No EXPECT on the subscription repository: it must not be reached.
			},
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockRepo, mockDeliveryRepo, _, service, ctrl := setupWebhookSubscriptionTest(t)
			defer ctrl.Finish()

			tc.setupMocks(mockRepo, mockDeliveryRepo)

			err := service.Delete(context.Background(), tc.workspaceID, tc.subID)

			if tc.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestWebhookSubscriptionService_Toggle(t *testing.T) {
	testCases := []struct {
		name           string
		workspaceID    string
		subID          string
		enabled        bool
		setupMocks     func(*mocks.MockWebhookSubscriptionRepository)
		expectError    bool
		validateResult func(*testing.T, *domain.WebhookSubscription)
	}{
		{
			name:        "enable webhook",
			workspaceID: "workspace123",
			subID:       "sub123",
			enabled:     true,
			setupMocks: func(mockRepo *mocks.MockWebhookSubscriptionRepository) {
				mockRepo.EXPECT().
					GetByID(gomock.Any(), "workspace123", "sub123").
					Return(&domain.WebhookSubscription{
						ID:      "sub123",
						Enabled: false,
					}, nil)

				mockRepo.EXPECT().
					Update(gomock.Any(), "workspace123", gomock.Any()).
					DoAndReturn(func(ctx context.Context, workspaceID string, sub *domain.WebhookSubscription) error {
						require.True(t, sub.Enabled)
						return nil
					})
			},
			expectError: false,
			validateResult: func(t *testing.T, sub *domain.WebhookSubscription) {
				require.True(t, sub.Enabled)
			},
		},
		{
			name:        "disable webhook",
			workspaceID: "workspace123",
			subID:       "sub123",
			enabled:     false,
			setupMocks: func(mockRepo *mocks.MockWebhookSubscriptionRepository) {
				mockRepo.EXPECT().
					GetByID(gomock.Any(), "workspace123", "sub123").
					Return(&domain.WebhookSubscription{
						ID:      "sub123",
						Enabled: true,
					}, nil)

				mockRepo.EXPECT().
					Update(gomock.Any(), "workspace123", gomock.Any()).
					DoAndReturn(func(ctx context.Context, workspaceID string, sub *domain.WebhookSubscription) error {
						require.False(t, sub.Enabled)
						return nil
					})
			},
			expectError: false,
			validateResult: func(t *testing.T, sub *domain.WebhookSubscription) {
				require.False(t, sub.Enabled)
			},
		},
		{
			name:        "subscription not found",
			workspaceID: "workspace123",
			subID:       "nonexistent",
			enabled:     true,
			setupMocks: func(mockRepo *mocks.MockWebhookSubscriptionRepository) {
				mockRepo.EXPECT().
					GetByID(gomock.Any(), "workspace123", "nonexistent").
					Return(nil, errors.New("not found"))
			},
			expectError: true,
		},
		{
			name:        "update error",
			workspaceID: "workspace123",
			subID:       "sub123",
			enabled:     true,
			setupMocks: func(mockRepo *mocks.MockWebhookSubscriptionRepository) {
				mockRepo.EXPECT().
					GetByID(gomock.Any(), "workspace123", "sub123").
					Return(&domain.WebhookSubscription{ID: "sub123"}, nil)

				mockRepo.EXPECT().
					Update(gomock.Any(), "workspace123", gomock.Any()).
					Return(errors.New("database error"))
			},
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockRepo, _, _, service, ctrl := setupWebhookSubscriptionTest(t)
			defer ctrl.Finish()

			tc.setupMocks(mockRepo)

			result, err := service.Toggle(context.Background(), tc.workspaceID, tc.subID, tc.enabled)

			if tc.expectError {
				require.Error(t, err)
				require.Nil(t, result)
			} else {
				require.NoError(t, err)
				if tc.validateResult != nil {
					tc.validateResult(t, result)
				}
			}
		})
	}
}

func TestWebhookSubscriptionService_RegenerateSecret(t *testing.T) {
	testCases := []struct {
		name           string
		workspaceID    string
		subID          string
		setupMocks     func(*mocks.MockWebhookSubscriptionRepository)
		expectError    bool
		validateResult func(*testing.T, *domain.WebhookSubscription, string)
	}{
		{
			name:        "successful secret regeneration",
			workspaceID: "workspace123",
			subID:       "sub123",
			setupMocks: func(mockRepo *mocks.MockWebhookSubscriptionRepository) {
				mockRepo.EXPECT().
					GetByID(gomock.Any(), "workspace123", "sub123").
					Return(&domain.WebhookSubscription{
						ID:     "sub123",
						Secret: "old-secret",
					}, nil)

				mockRepo.EXPECT().
					Update(gomock.Any(), "workspace123", gomock.Any()).
					DoAndReturn(func(ctx context.Context, workspaceID string, sub *domain.WebhookSubscription) error {
						// Verify that secret was changed
						require.NotEqual(t, "old-secret", sub.Secret)
						require.NotEmpty(t, sub.Secret)
						return nil
					})
			},
			expectError: false,
			validateResult: func(t *testing.T, sub *domain.WebhookSubscription, oldSecret string) {
				require.NotNil(t, sub)
				require.NotEqual(t, oldSecret, sub.Secret)
				require.NotEmpty(t, sub.Secret)
				// Secret should be base64 encoded
				require.Greater(t, len(sub.Secret), 40) // 32 bytes base64 encoded is ~44 chars
			},
		},
		{
			name:        "subscription not found",
			workspaceID: "workspace123",
			subID:       "nonexistent",
			setupMocks: func(mockRepo *mocks.MockWebhookSubscriptionRepository) {
				mockRepo.EXPECT().
					GetByID(gomock.Any(), "workspace123", "nonexistent").
					Return(nil, errors.New("not found"))
			},
			expectError: true,
		},
		{
			name:        "update error",
			workspaceID: "workspace123",
			subID:       "sub123",
			setupMocks: func(mockRepo *mocks.MockWebhookSubscriptionRepository) {
				mockRepo.EXPECT().
					GetByID(gomock.Any(), "workspace123", "sub123").
					Return(&domain.WebhookSubscription{
						ID:     "sub123",
						Secret: "old-secret",
					}, nil)

				mockRepo.EXPECT().
					Update(gomock.Any(), "workspace123", gomock.Any()).
					Return(errors.New("database error"))
			},
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockRepo, _, _, service, ctrl := setupWebhookSubscriptionTest(t)
			defer ctrl.Finish()

			tc.setupMocks(mockRepo)

			result, err := service.RegenerateSecret(context.Background(), tc.workspaceID, tc.subID)

			if tc.expectError {
				require.Error(t, err)
				require.Nil(t, result)
			} else {
				require.NoError(t, err)
				if tc.validateResult != nil {
					tc.validateResult(t, result, "old-secret")
				}
			}
		})
	}
}

func TestWebhookSubscriptionService_GetDeliveries(t *testing.T) {
	now := time.Now()
	subID := "sub123"

	testCases := []struct {
		name           string
		workspaceID    string
		subscriptionID *string
		limit          int
		offset         int
		setupMocks     func(*mocks.MockWebhookDeliveryRepository)
		expectError    bool
		expectedTotal  int
		expectedCount  int
	}{
		{
			name:           "successful retrieval with deliveries",
			workspaceID:    "workspace123",
			subscriptionID: &subID,
			limit:          10,
			offset:         0,
			setupMocks: func(mockDeliveryRepo *mocks.MockWebhookDeliveryRepository) {
				mockDeliveryRepo.EXPECT().
					ListAll(gomock.Any(), "workspace123", gomock.Any(), 10, 0).
					Return([]*domain.WebhookDelivery{
						{
							ID:             "delivery1",
							SubscriptionID: "sub123",
							EventType:      "contact.created",
							Status:         domain.WebhookDeliveryStatusDelivered,
							CreatedAt:      now,
						},
						{
							ID:             "delivery2",
							SubscriptionID: "sub123",
							EventType:      "contact.updated",
							Status:         domain.WebhookDeliveryStatusFailed,
							CreatedAt:      now.Add(-1 * time.Hour),
						},
					}, 2, nil)
			},
			expectError:   false,
			expectedTotal: 2,
			expectedCount: 2,
		},
		{
			name:           "empty deliveries list",
			workspaceID:    "workspace123",
			subscriptionID: &subID,
			limit:          10,
			offset:         0,
			setupMocks: func(mockDeliveryRepo *mocks.MockWebhookDeliveryRepository) {
				mockDeliveryRepo.EXPECT().
					ListAll(gomock.Any(), "workspace123", gomock.Any(), 10, 0).
					Return([]*domain.WebhookDelivery{}, 0, nil)
			},
			expectError:   false,
			expectedTotal: 0,
			expectedCount: 0,
		},
		{
			name:           "pagination with offset",
			workspaceID:    "workspace123",
			subscriptionID: &subID,
			limit:          5,
			offset:         10,
			setupMocks: func(mockDeliveryRepo *mocks.MockWebhookDeliveryRepository) {
				mockDeliveryRepo.EXPECT().
					ListAll(gomock.Any(), "workspace123", gomock.Any(), 5, 10).
					Return([]*domain.WebhookDelivery{
						{ID: "delivery11", SubscriptionID: "sub123"},
						{ID: "delivery12", SubscriptionID: "sub123"},
					}, 25, nil)
			},
			expectError:   false,
			expectedTotal: 25,
			expectedCount: 2,
		},
		{
			name:           "all deliveries without subscription filter",
			workspaceID:    "workspace123",
			subscriptionID: nil,
			limit:          10,
			offset:         0,
			setupMocks: func(mockDeliveryRepo *mocks.MockWebhookDeliveryRepository) {
				mockDeliveryRepo.EXPECT().
					ListAll(gomock.Any(), "workspace123", nil, 10, 0).
					Return([]*domain.WebhookDelivery{
						{ID: "delivery1", SubscriptionID: "sub123"},
						{ID: "delivery2", SubscriptionID: "sub456"},
					}, 2, nil)
			},
			expectError:   false,
			expectedTotal: 2,
			expectedCount: 2,
		},
		{
			name:           "repository error",
			workspaceID:    "workspace123",
			subscriptionID: &subID,
			limit:          10,
			offset:         0,
			setupMocks: func(mockDeliveryRepo *mocks.MockWebhookDeliveryRepository) {
				mockDeliveryRepo.EXPECT().
					ListAll(gomock.Any(), "workspace123", gomock.Any(), 10, 0).
					Return(nil, 0, errors.New("database error"))
			},
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, mockDeliveryRepo, _, service, ctrl := setupWebhookSubscriptionTest(t)
			defer ctrl.Finish()

			tc.setupMocks(mockDeliveryRepo)

			deliveries, total, err := service.GetDeliveries(
				context.Background(),
				tc.workspaceID,
				tc.subscriptionID,
				tc.limit,
				tc.offset,
			)

			if tc.expectError {
				require.Error(t, err)
				require.Nil(t, deliveries)
				require.Equal(t, 0, total)
			} else {
				require.NoError(t, err)
				require.Len(t, deliveries, tc.expectedCount)
				require.Equal(t, tc.expectedTotal, total)
			}
		})
	}
}

func TestWebhookSubscriptionService_GetEventTypes(t *testing.T) {
	_, _, _, service, ctrl := setupWebhookSubscriptionTest(t)
	defer ctrl.Finish()

	eventTypes := service.GetEventTypes()

	// Verify it returns the expected event types
	require.NotEmpty(t, eventTypes)
	require.Contains(t, eventTypes, "contact.created")
	require.Contains(t, eventTypes, "contact.updated")
	require.Contains(t, eventTypes, "contact.deleted")
	require.Contains(t, eventTypes, "email.sent")
	require.Contains(t, eventTypes, "email.delivered")
	require.Contains(t, eventTypes, "custom_event.created")

	// Verify the list matches domain.WebhookEventTypes
	require.Equal(t, domain.WebhookEventTypes, eventTypes)
}

func TestValidateURL(t *testing.T) {
	testCases := []struct {
		name        string
		url         string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "valid https URL",
			url:         "https://example.com/webhook",
			expectError: false,
		},
		{
			name:        "valid http URL",
			url:         "http://example.com/webhook",
			expectError: false,
		},
		{
			name:        "valid URL with port",
			url:         "https://example.com:8080/webhook",
			expectError: false,
		},
		{
			name:        "valid URL with path and query",
			url:         "https://example.com/webhook?token=abc123",
			expectError: false,
		},
		{
			name:        "empty URL",
			url:         "",
			expectError: true,
			errorMsg:    "URL is required",
		},
		{
			name:        "invalid scheme - ftp",
			url:         "ftp://example.com/webhook",
			expectError: true,
			errorMsg:    "URL must use http or https scheme",
		},
		{
			name:        "invalid scheme - ws",
			url:         "ws://example.com/webhook",
			expectError: true,
			errorMsg:    "URL must use http or https scheme",
		},
		{
			name:        "URL without scheme",
			url:         "example.com/webhook",
			expectError: true,
		},
		{
			name:        "URL without host",
			url:         "https://",
			expectError: true,
			errorMsg:    "URL must have a host",
		},
		{
			name:        "malformed URL",
			url:         "not a url at all",
			expectError: true,
		},
		{
			name:        "URL with only scheme",
			url:         "https://",
			expectError: true,
			errorMsg:    "URL must have a host",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateURL(tc.url)

			if tc.expectError {
				require.Error(t, err)
				if tc.errorMsg != "" {
					require.Contains(t, err.Error(), tc.errorMsg)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidateEventTypes(t *testing.T) {
	testCases := []struct {
		name        string
		eventTypes  []string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "valid single event type",
			eventTypes:  []string{"contact.created"},
			expectError: false,
		},
		{
			name:        "valid multiple event types",
			eventTypes:  []string{"contact.created", "contact.updated", "email.sent"},
			expectError: false,
		},
		{
			name:        "all event types",
			eventTypes:  domain.WebhookEventTypes,
			expectError: false,
		},
		{
			name:        "empty event types",
			eventTypes:  []string{},
			expectError: true,
			errorMsg:    "at least one event type is required",
		},
		{
			name:        "nil event types",
			eventTypes:  nil,
			expectError: true,
			errorMsg:    "at least one event type is required",
		},
		{
			name:        "invalid event type",
			eventTypes:  []string{"invalid.event"},
			expectError: true,
			errorMsg:    "invalid event type: invalid.event",
		},
		{
			name:        "mix of valid and invalid",
			eventTypes:  []string{"contact.created", "invalid.event"},
			expectError: true,
			errorMsg:    "invalid event type: invalid.event",
		},
		{
			name:        "case sensitive validation",
			eventTypes:  []string{"Contact.Created"}, // wrong case
			expectError: true,
			errorMsg:    "invalid event type",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateEventTypes(tc.eventTypes)

			if tc.expectError {
				require.Error(t, err)
				if tc.errorMsg != "" {
					require.Contains(t, err.Error(), tc.errorMsg)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestGenerateSecret(t *testing.T) {
	// Test that generateSecret produces Standard Webhooks compliant secrets
	secrets := make(map[string]bool)

	for i := 0; i < 100; i++ {
		secret, err := generateSecret()
		require.NoError(t, err)
		require.NotEmpty(t, secret)

		// Must carry the whsec_ prefix required by the Standard Webhooks spec
		require.True(t, strings.HasPrefix(secret, "whsec_"), "Secret must start with whsec_")

		// The part after the prefix must decode to exactly 32 bytes (256 bits)
		key, decodeErr := decodeSecret(secret)
		require.NoError(t, decodeErr)
		require.Len(t, key, 32)

		// Each secret should be unique
		require.False(t, secrets[secret], "Secret should be unique")
		secrets[secret] = true
	}
}

func TestDecodeSecret(t *testing.T) {
	t.Run("valid whsec_ prefixed secret", func(t *testing.T) {
		secret, err := generateSecret()
		require.NoError(t, err)

		key, err := decodeSecret(secret)
		require.NoError(t, err)
		require.Len(t, key, 32)
	})

	t.Run("missing prefix is rejected", func(t *testing.T) {
		// Generate a secret then strip the prefix to simulate a pre-v30 row.
		secret, err := generateSecret()
		require.NoError(t, err)
		bare := strings.TrimPrefix(secret, "whsec_")

		_, err = decodeSecret(bare)
		require.Error(t, err)
		require.Contains(t, err.Error(), "whsec_")
	})

	t.Run("malformed base64 after prefix is rejected", func(t *testing.T) {
		_, err := decodeSecret("whsec_!!!not-base64!!!")
		require.Error(t, err)
		require.Contains(t, err.Error(), "base64")
	})

	t.Run("empty string is rejected", func(t *testing.T) {
		_, err := decodeSecret("")
		require.Error(t, err)
	})
}

func TestGenerateWebhookID(t *testing.T) {
	// Test that generateWebhookID produces valid IDs
	ids := make(map[string]bool)

	for i := 0; i < 100; i++ {
		id := generateWebhookID()
		require.NotEmpty(t, id)

		// ID should be exactly 32 characters
		require.Len(t, id, 32)

		// ID should not contain dashes
		require.NotContains(t, id, "-")

		// Each ID should be unique
		require.False(t, ids[id], "ID should be unique")
		ids[id] = true
	}
}

func TestWebhookSubscriptionService_Create_SecretGeneration(t *testing.T) {
	// Test that Create generates unique secrets
	mockRepo, _, _, service, ctrl := setupWebhookSubscriptionTest(t)
	defer ctrl.Finish()

	secrets := make([]string, 0, 10)

	for i := 0; i < 10; i++ {
		mockRepo.EXPECT().
			Create(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, workspaceID string, sub *domain.WebhookSubscription) error {
				secrets = append(secrets, sub.Secret)
				return nil
			})

		_, err := service.Create(
			context.Background(),
			"workspace123",
			fmt.Sprintf("Webhook %d", i),
			"https://example.com/webhook",
			[]string{"contact.created"},
			nil,
			domain.WebhookSubscriptionSourceUser,
			nil,
			nil,
		)
		require.NoError(t, err)
	}

	// Verify all secrets are unique
	secretMap := make(map[string]bool)
	for _, secret := range secrets {
		require.False(t, secretMap[secret], "Each webhook should have a unique secret")
		secretMap[secret] = true
	}
}

func TestWebhookSubscriptionService_Create_IDGeneration(t *testing.T) {
	// Test that Create generates unique IDs
	mockRepo, _, _, service, ctrl := setupWebhookSubscriptionTest(t)
	defer ctrl.Finish()

	ids := make([]string, 0, 10)

	for i := 0; i < 10; i++ {
		mockRepo.EXPECT().
			Create(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, workspaceID string, sub *domain.WebhookSubscription) error {
				ids = append(ids, sub.ID)
				return nil
			})

		_, err := service.Create(
			context.Background(),
			"workspace123",
			fmt.Sprintf("Webhook %d", i),
			"https://example.com/webhook",
			[]string{"contact.created"},
			nil,
			domain.WebhookSubscriptionSourceUser,
			nil,
			nil,
		)
		require.NoError(t, err)
	}

	// Verify all IDs are unique and properly formatted
	idMap := make(map[string]bool)
	for _, id := range ids {
		require.Len(t, id, 32, "ID should be 32 characters")
		require.False(t, strings.Contains(id, "-"), "ID should not contain dashes")
		require.False(t, idMap[id], "Each webhook should have a unique ID")
		idMap[id] = true
	}
}

func TestWebhookSubscriptionService_Create_DefaultValues(t *testing.T) {
	// Test that Create sets correct default values
	mockRepo, _, _, service, ctrl := setupWebhookSubscriptionTest(t)
	defer ctrl.Finish()

	mockRepo.EXPECT().
		Create(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, workspaceID string, sub *domain.WebhookSubscription) error {
			// Verify default values
			assert.True(t, sub.Enabled, "Webhook should be enabled by default")
			assert.Nil(t, sub.LastDeliveryAt, "LastDeliveryAt should be nil")
			return nil
		})

	_, err := service.Create(
		context.Background(),
		"workspace123",
		"Test Webhook",
		"https://example.com/webhook",
		[]string{"contact.created"},
		nil,
		domain.WebhookSubscriptionSourceUser,
		nil,
		nil,
	)
	require.NoError(t, err)
}

func TestWebhookSubscriptionService_Update_PreservesSecret(t *testing.T) {
	// Test that Update does not change the secret
	mockRepo, _, _, service, ctrl := setupWebhookSubscriptionTest(t)
	defer ctrl.Finish()

	originalSecret := "original-secret-value"

	mockRepo.EXPECT().
		GetByID(gomock.Any(), "workspace123", "sub123").
		Return(&domain.WebhookSubscription{
			ID:     "sub123",
			Secret: originalSecret,
		}, nil)

	mockRepo.EXPECT().
		Update(gomock.Any(), "workspace123", gomock.Any()).
		DoAndReturn(func(ctx context.Context, workspaceID string, sub *domain.WebhookSubscription) error {
			// Verify secret was not changed
			assert.Equal(t, originalSecret, sub.Secret, "Update should not modify the secret")
			return nil
		})

	_, err := service.Update(
		context.Background(),
		"workspace123",
		"sub123",
		"Updated Name",
		"https://new.example.com/webhook",
		[]string{"contact.updated"},
		nil,
		boolPtr(true),
		nil,
		nil,
	)
	require.NoError(t, err)
}

// TestWebhookSubscriptionService_RejectsNonMembers pins the workspace boundary.
//
// Isolation is per-database, but workspace_id — which every one of these methods
// takes straight from the caller — is only a database selector. The membership
// check is what establishes the right to the data behind it.
//
// The assertion that matters is not the returned error: it is that NO repository
// method is reached. The mocks below have no EXPECT() calls, so gomock fails the
// test if any of them is touched.
func TestWebhookSubscriptionService_RejectsNonMembers(t *testing.T) {
	const victimWorkspace = "victim-workspace"

	authFailure := errors.New("user is not a member of the workspace")

	cases := []struct {
		name string
		call func(context.Context, *WebhookSubscriptionService) error
	}{
		{"Create", func(ctx context.Context, s *WebhookSubscriptionService) error {
			_, err := s.Create(ctx, victimWorkspace, "n", "https://example.com/h", []string{"contact.created"}, nil, domain.WebhookSubscriptionSourceUser, nil, nil)
			return err
		}},
		{"GetByID", func(ctx context.Context, s *WebhookSubscriptionService) error {
			_, err := s.GetByID(ctx, victimWorkspace, "sub-1")
			return err
		}},
		{"List", func(ctx context.Context, s *WebhookSubscriptionService) error {
			_, err := s.List(ctx, victimWorkspace)
			return err
		}},
		{"Update", func(ctx context.Context, s *WebhookSubscriptionService) error {
			_, err := s.Update(ctx, victimWorkspace, "sub-1", "n", "https://attacker.example.com/h", []string{"contact.created"}, nil, boolPtr(true), nil, nil)
			return err
		}},
		{"Delete", func(ctx context.Context, s *WebhookSubscriptionService) error {
			return s.Delete(ctx, victimWorkspace, "sub-1")
		}},
		{"Toggle", func(ctx context.Context, s *WebhookSubscriptionService) error {
			_, err := s.Toggle(ctx, victimWorkspace, "sub-1", false)
			return err
		}},
		{"RegenerateSecret", func(ctx context.Context, s *WebhookSubscriptionService) error {
			_, err := s.RegenerateSecret(ctx, victimWorkspace, "sub-1")
			return err
		}},
		{"GetDeliveries", func(ctx context.Context, s *WebhookSubscriptionService) error {
			_, _, err := s.GetDeliveries(ctx, victimWorkspace, nil, 10, 0)
			return err
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockRepo := mocks.NewMockWebhookSubscriptionRepository(ctrl)
			mockDeliveryRepo := mocks.NewMockWebhookDeliveryRepository(ctrl)
			mockLogger := pkgmocks.NewMockLogger(ctrl)
			mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
			mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
			mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
			mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
			mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()
			mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

			mockAuthService := mocks.NewMockAuthService(ctrl)
			mockAuthService.EXPECT().
				AuthenticateUserForWorkspace(gomock.Any(), victimWorkspace).
				Return(nil, nil, nil, authFailure)

			service := NewWebhookSubscriptionService(mockRepo, mockDeliveryRepo, mockAuthService, mockLogger)

			err := tc.call(context.Background(), service)
			require.Error(t, err, "a non-member must not be served")
			assert.Contains(t, err.Error(), "failed to authenticate user")
		})
	}
}

// A webhook secret is a signing key, not ordinary workspace data.
//
// Each subscription carries its own, and whoever holds one can forge payloads
// that the customer's downstream consumer will accept as genuine. Reading it is
// therefore owner-only, matching how integrations and API keys are already gated
// (workspace_service.go:324, 457, 778) rather than the read/write permission
// model used for contacts or lists.
//
// The creator of a subscription still receives its secret once — they have to,
// to configure the receiver — and that grants nothing about anyone else's.
func TestWebhookSubscriptionSecretIsOwnerOnly(t *testing.T) {
	const workspaceID = "ws-1"
	const secret = "whsec_SENTINEL"

	newService := func(t *testing.T, role string) (*WebhookSubscriptionService, *mocks.MockWebhookSubscriptionRepository) {
		t.Helper()
		ctrl := gomock.NewController(t)
		t.Cleanup(ctrl.Finish)

		repo := mocks.NewMockWebhookSubscriptionRepository(ctrl)
		logger := pkgmocks.NewMockLogger(ctrl)
		logger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(logger).AnyTimes()
		logger.EXPECT().WithFields(gomock.Any()).Return(logger).AnyTimes()
		logger.EXPECT().Info(gomock.Any()).AnyTimes()
		logger.EXPECT().Debug(gomock.Any()).AnyTimes()
		logger.EXPECT().Warn(gomock.Any()).AnyTimes()
		logger.EXPECT().Error(gomock.Any()).AnyTimes()

		auth := mocks.NewMockAuthService(ctrl)
		auth.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
			DoAndReturn(func(ctx context.Context, id string) (context.Context, *domain.User, *domain.UserWorkspace, error) {
				return ctx, &domain.User{ID: "u1"}, &domain.UserWorkspace{
					UserID: "u1", WorkspaceID: id, Role: role,
					// Both verbs granted: this test is about the signing secret, which
					// no permission confers. Redaction is what withholds it, and it
					// answers to the role alone.
					Permissions: domain.UserPermissions{
						domain.PermissionResourceWebhookSubscriptions: {Read: true, Write: true},
					},
				}, nil
			}).AnyTimes()

		return NewWebhookSubscriptionService(repo, mocks.NewMockWebhookDeliveryRepository(ctrl), auth, logger), repo
	}

	stored := func() *domain.WebhookSubscription {
		return &domain.WebhookSubscription{ID: "sub-1", Name: "crm", URL: "https://x/h", Secret: secret}
	}

	t.Run("an owner reading a list still gets the secret", func(t *testing.T) {
		svc, repo := newService(t, "owner")
		repo.EXPECT().List(gomock.Any(), workspaceID).Return([]*domain.WebhookSubscription{stored()}, nil)

		subs, err := svc.List(context.Background(), workspaceID)
		require.NoError(t, err)
		require.Len(t, subs, 1)
		assert.Equal(t, secret, subs[0].Secret, "the console reveals it from this payload")
	})

	t.Run("a member reading a list does not", func(t *testing.T) {
		svc, repo := newService(t, "member")
		repo.EXPECT().List(gomock.Any(), workspaceID).Return([]*domain.WebhookSubscription{stored()}, nil)

		subs, err := svc.List(context.Background(), workspaceID)
		require.NoError(t, err)
		require.Len(t, subs, 1)
		assert.Empty(t, subs[0].Secret)
		assert.Equal(t, "crm", subs[0].Name, "everything else is still readable")
		assert.Equal(t, "https://x/h", subs[0].URL)
	})

	t.Run("a member reading one subscription does not", func(t *testing.T) {
		svc, repo := newService(t, "member")
		repo.EXPECT().GetByID(gomock.Any(), workspaceID, "sub-1").Return(stored(), nil)

		sub, err := svc.GetByID(context.Background(), workspaceID, "sub-1")
		require.NoError(t, err)
		assert.Empty(t, sub.Secret)
	})

	t.Run("an owner reading one subscription does", func(t *testing.T) {
		svc, repo := newService(t, "owner")
		repo.EXPECT().GetByID(gomock.Any(), workspaceID, "sub-1").Return(stored(), nil)

		sub, err := svc.GetByID(context.Background(), workspaceID, "sub-1")
		require.NoError(t, err)
		assert.Equal(t, secret, sub.Secret)
	})

	// Without this, gating the read is theatre: a member could rotate a secret to
	// learn it — and in doing so break the customer's live integration.
	t.Run("a member cannot regenerate a secret", func(t *testing.T) {
		svc, repo := newService(t, "member")
		// No EXPECT on the repo: it must not be reached at all.
		_ = repo

		_, err := svc.RegenerateSecret(context.Background(), workspaceID, "sub-1")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "owner")
	})

	t.Run("an owner can regenerate and receives the new secret", func(t *testing.T) {
		svc, repo := newService(t, "owner")
		repo.EXPECT().GetByID(gomock.Any(), workspaceID, "sub-1").Return(stored(), nil)
		repo.EXPECT().Update(gomock.Any(), workspaceID, gomock.Any()).Return(nil)

		sub, err := svc.RegenerateSecret(context.Background(), workspaceID, "sub-1")
		require.NoError(t, err)
		assert.NotEmpty(t, sub.Secret, "the owner needs it once, to update the receiver")
		assert.NotEqual(t, secret, sub.Secret)
	})

	// Creating your own subscription hands you its own secret, which grants
	// nothing about anyone else's — every subscription has a distinct one.
	t.Run("a member creating a subscription still receives its secret", func(t *testing.T) {
		svc, repo := newService(t, "member")
		repo.EXPECT().Create(gomock.Any(), workspaceID, gomock.Any()).Return(nil)

		sub, err := svc.Create(context.Background(), workspaceID, "mine", "https://mine/h",
			[]string{"contact.created"}, nil, domain.WebhookSubscriptionSourceUser, nil, nil)
		require.NoError(t, err)
		assert.NotEmpty(t, sub.Secret)
	})
}

// newPermissionScopedSubscriptionService builds the service around a member row
// carrying exactly the grants a case wants to prove something about. Role is
// "member", not "owner", so HasPermission actually consults the map.
func newPermissionScopedSubscriptionService(t *testing.T, workspaceID string, permissions domain.UserPermissions) (
	*WebhookSubscriptionService,
	*mocks.MockWebhookSubscriptionRepository,
	*mocks.MockWebhookDeliveryRepository,
) {
	t.Helper()
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	repo := mocks.NewMockWebhookSubscriptionRepository(ctrl)
	deliveryRepo := mocks.NewMockWebhookDeliveryRepository(ctrl)

	logger := pkgmocks.NewMockLogger(ctrl)
	logger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(logger).AnyTimes()
	logger.EXPECT().WithFields(gomock.Any()).Return(logger).AnyTimes()
	logger.EXPECT().Info(gomock.Any()).AnyTimes()
	logger.EXPECT().Debug(gomock.Any()).AnyTimes()
	logger.EXPECT().Warn(gomock.Any()).AnyTimes()
	logger.EXPECT().Error(gomock.Any()).AnyTimes()

	auth := mocks.NewMockAuthService(ctrl)
	auth.EXPECT().
		AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
		DoAndReturn(func(ctx context.Context, id string) (context.Context, *domain.User, *domain.UserWorkspace, error) {
			return ctx, &domain.User{ID: "u1"}, &domain.UserWorkspace{
				UserID:      "u1",
				WorkspaceID: id,
				Role:        "member",
				Permissions: permissions,
			}, nil
		}).AnyTimes()

	return NewWebhookSubscriptionService(repo, deliveryRepo, auth, logger), repo, deliveryRepo
}

// TestWebhookSubscriptionService_PermissionEnforcement verifies that every
// subscription operation enforces the correct webhook_subscriptions permission.
// Each method is exercised by a workspace member who has been granted the OPPOSITE
// permission (a write operation is tested with a read-only member), so the test
// fails both if a check is missing AND if a method is gated on the wrong permission
// type. No repository expectations are set, so gomock also fails if anything beyond
// the permission gate runs.
func TestWebhookSubscriptionService_PermissionEnforcement(t *testing.T) {
	const workspaceID = "ws-1"

	grant := func(read, write bool) domain.UserPermissions {
		return domain.UserPermissions{
			domain.PermissionResourceWebhookSubscriptions: {Read: read, Write: write},
		}
	}

	cases := []struct {
		name string
		perm domain.PermissionType
		call func(context.Context, *WebhookSubscriptionService) error
	}{
		{"Create", domain.PermissionTypeWrite, func(ctx context.Context, s *WebhookSubscriptionService) error {
			_, err := s.Create(ctx, workspaceID, "crm", "https://x/h", []string{"contact.created"}, nil, domain.WebhookSubscriptionSourceUser, nil, nil)
			return err
		}},
		{"GetByID", domain.PermissionTypeRead, func(ctx context.Context, s *WebhookSubscriptionService) error {
			_, err := s.GetByID(ctx, workspaceID, "sub-1")
			return err
		}},
		{"List", domain.PermissionTypeRead, func(ctx context.Context, s *WebhookSubscriptionService) error {
			_, err := s.List(ctx, workspaceID)
			return err
		}},
		{"Update", domain.PermissionTypeWrite, func(ctx context.Context, s *WebhookSubscriptionService) error {
			_, err := s.Update(ctx, workspaceID, "sub-1", "crm", "https://x/h", []string{"contact.created"}, nil, boolPtr(true), nil, nil)
			return err
		}},
		{"Delete", domain.PermissionTypeWrite, func(ctx context.Context, s *WebhookSubscriptionService) error {
			return s.Delete(ctx, workspaceID, "sub-1")
		}},
		{"Toggle", domain.PermissionTypeWrite, func(ctx context.Context, s *WebhookSubscriptionService) error {
			_, err := s.Toggle(ctx, workspaceID, "sub-1", false)
			return err
		}},
		{"GetDeliveries", domain.PermissionTypeRead, func(ctx context.Context, s *WebhookSubscriptionService) error {
			_, _, err := s.GetDeliveries(ctx, workspaceID, nil, 10, 0)
			return err
		}},
		{"GetForTestDelivery", domain.PermissionTypeWrite, func(ctx context.Context, s *WebhookSubscriptionService) error {
			_, err := s.GetForTestDelivery(ctx, workspaceID, "sub-1")
			return err
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Grant only the OPPOSITE permission, so the case proves the exact
			// permission type is required and catches a read/write swap.
			permissions := grant(true, false)
			if tc.perm == domain.PermissionTypeRead {
				permissions = grant(false, true)
			}

			svc, _, _ := newPermissionScopedSubscriptionService(t, workspaceID, permissions)

			err := tc.call(context.Background(), svc)
			require.Error(t, err)
			assert.IsType(t, &domain.PermissionError{}, err)

			var permErr *domain.PermissionError
			require.True(t, errors.As(err, &permErr))
			assert.Equal(t, domain.PermissionResourceWebhookSubscriptions, permErr.Resource)
			assert.Equal(t, tc.perm, permErr.Permission)
		})
	}
}

// A read grant reads subscriptions; it does not hand out signing secrets. The
// permission and the owner-only secret rules are orthogonal, and this pins that
// the first does not quietly relax the second.
func TestWebhookSubscriptionService_ReadGrantDoesNotRevealSecret(t *testing.T) {
	const workspaceID = "ws-1"
	const secret = "whsec_SENTINEL"

	readOnly := domain.UserPermissions{
		domain.PermissionResourceWebhookSubscriptions: {Read: true},
	}
	stored := func() *domain.WebhookSubscription {
		return &domain.WebhookSubscription{ID: "sub-1", Name: "crm", URL: "https://x/h", Secret: secret}
	}

	t.Run("GetByID", func(t *testing.T) {
		svc, repo, _ := newPermissionScopedSubscriptionService(t, workspaceID, readOnly)
		repo.EXPECT().GetByID(gomock.Any(), workspaceID, "sub-1").Return(stored(), nil)

		sub, err := svc.GetByID(context.Background(), workspaceID, "sub-1")
		require.NoError(t, err)
		assert.Empty(t, sub.Secret)
		assert.Equal(t, "crm", sub.Name, "everything else is still readable")
	})

	t.Run("List", func(t *testing.T) {
		svc, repo, _ := newPermissionScopedSubscriptionService(t, workspaceID, readOnly)
		repo.EXPECT().List(gomock.Any(), workspaceID).Return([]*domain.WebhookSubscription{stored()}, nil)

		subs, err := svc.List(context.Background(), workspaceID)
		require.NoError(t, err)
		require.Len(t, subs, 1)
		assert.Empty(t, subs[0].Secret)
	})
}

// Rotating a secret is owner-only and stays that way: the new resource is not a
// second route to it. A member holding both verbs is rejected, and the repository
// is never reached.
func TestWebhookSubscriptionService_RegenerateSecretStaysOwnerOnly(t *testing.T) {
	const workspaceID = "ws-1"

	svc, _, _ := newPermissionScopedSubscriptionService(t, workspaceID, domain.UserPermissions{
		domain.PermissionResourceWebhookSubscriptions: {Read: true, Write: true},
	})

	_, err := svc.RegenerateSecret(context.Background(), workspaceID, "sub-1")
	require.Error(t, err)
	assert.IsType(t, &domain.ErrUnauthorized{}, err)
	assert.Contains(t, err.Error(), "owner")
}

// A test delivery fires a real outbound request, so it answers to the write grant.
// The handler used to authorize it by calling GetByID, which would have let a
// read-only key trigger arbitrary deliveries.
func TestWebhookSubscriptionService_TestDeliveryRequiresWrite(t *testing.T) {
	const workspaceID = "ws-1"
	const secret = "whsec_SENTINEL"

	t.Run("a read-only member is denied and the repository is untouched", func(t *testing.T) {
		svc, _, _ := newPermissionScopedSubscriptionService(t, workspaceID, domain.UserPermissions{
			domain.PermissionResourceWebhookSubscriptions: {Read: true},
		})

		_, err := svc.GetForTestDelivery(context.Background(), workspaceID, "sub-1")
		require.Error(t, err)
		assert.IsType(t, &domain.PermissionError{}, err)
	})

	// The secret comes back un-redacted for a non-owner: it signs the test payload
	// on its way to the subscription's own URL and is never written to the client.
	t.Run("a member holding write receives the signing secret", func(t *testing.T) {
		svc, repo, _ := newPermissionScopedSubscriptionService(t, workspaceID, domain.UserPermissions{
			domain.PermissionResourceWebhookSubscriptions: {Write: true},
		})
		repo.EXPECT().GetByID(gomock.Any(), workspaceID, "sub-1").
			Return(&domain.WebhookSubscription{ID: "sub-1", URL: "https://x/h", Secret: secret}, nil)

		sub, err := svc.GetForTestDelivery(context.Background(), workspaceID, "sub-1")
		require.NoError(t, err)
		assert.Equal(t, secret, sub.Secret)
	})
}

// TestWebhookSubscriptionService_SecretRedactionPerMethod records, one row per
// method, whether the signing secret survives the trip back to the caller.
//
// Every method here that returns a subscription needs such a row, because the
// handlers serialise the whole object and Secret carries no `omitempty`. That is
// how `.toggle` came to be a no-op route to any subscription's plaintext key for
// anyone holding webhook_subscriptions:write, while the two read methods were
// carefully redacted — the decision was made twice and then never again. The
// reflection guard below fails when a method is added without a row, so the
// third time cannot be silent.
func TestWebhookSubscriptionService_SecretRedactionPerMethod(t *testing.T) {
	const workspaceID = "ws-1"
	const storedSecret = "whsec_SENTINEL"

	stored := func() *domain.WebhookSubscription {
		return &domain.WebhookSubscription{ID: "sub-1", Name: "crm", URL: "https://x/h", Secret: storedSecret}
	}

	newService := func(t *testing.T, role string) (*WebhookSubscriptionService, *mocks.MockWebhookSubscriptionRepository) {
		t.Helper()
		ctrl := gomock.NewController(t)
		t.Cleanup(ctrl.Finish)

		repo := mocks.NewMockWebhookSubscriptionRepository(ctrl)
		logger := pkgmocks.NewMockLogger(ctrl)
		logger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(logger).AnyTimes()
		logger.EXPECT().WithFields(gomock.Any()).Return(logger).AnyTimes()
		logger.EXPECT().Info(gomock.Any()).AnyTimes()
		logger.EXPECT().Debug(gomock.Any()).AnyTimes()
		logger.EXPECT().Warn(gomock.Any()).AnyTimes()
		logger.EXPECT().Error(gomock.Any()).AnyTimes()

		auth := mocks.NewMockAuthService(ctrl)
		auth.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
			DoAndReturn(func(ctx context.Context, id string) (context.Context, *domain.User, *domain.UserWorkspace, error) {
				return ctx, &domain.User{ID: "u1"}, &domain.UserWorkspace{
					UserID: "u1", WorkspaceID: id, Role: role,
					// Both verbs, deliberately: no permission confers the signing
					// secret, so redaction has to answer to the role alone.
					Permissions: domain.UserPermissions{
						domain.PermissionResourceWebhookSubscriptions: {Read: true, Write: true},
					},
				}, nil
			}).AnyTimes()

		return NewWebhookSubscriptionService(repo, mocks.NewMockWebhookDeliveryRepository(ctrl), auth, logger), repo
	}

	type decision struct {
		// memberSeesSecret is the recorded decision for a workspace member.
		memberSeesSecret bool
		// memberIsRejected marks a method that refuses a non-owner outright, so
		// there is no subscription for it to redact.
		memberIsRejected bool
		why              string
		arrange          func(repo *mocks.MockWebhookSubscriptionRepository, isOwner bool)
		call             func(context.Context, *WebhookSubscriptionService) (*domain.WebhookSubscription, error)
	}

	decisions := map[string]decision{
		"Create": {
			memberSeesSecret: true,
			why:              "the secret was minted for a row that did not exist a moment ago, and this response is the only place its creator can read it",
			arrange: func(repo *mocks.MockWebhookSubscriptionRepository, _ bool) {
				repo.EXPECT().Create(gomock.Any(), workspaceID, gomock.Any()).Return(nil)
			},
			call: func(ctx context.Context, s *WebhookSubscriptionService) (*domain.WebhookSubscription, error) {
				return s.Create(ctx, workspaceID, "crm", "https://x/h", []string{"contact.created"}, nil, domain.WebhookSubscriptionSourceUser, nil, nil)
			},
		},
		"GetByID": {
			why: "reading a subscription is not being handed its key",
			arrange: func(repo *mocks.MockWebhookSubscriptionRepository, _ bool) {
				repo.EXPECT().GetByID(gomock.Any(), workspaceID, "sub-1").Return(stored(), nil)
			},
			call: func(ctx context.Context, s *WebhookSubscriptionService) (*domain.WebhookSubscription, error) {
				return s.GetByID(ctx, workspaceID, "sub-1")
			},
		},
		"List": {
			why: "same as GetByID, once per row",
			arrange: func(repo *mocks.MockWebhookSubscriptionRepository, _ bool) {
				repo.EXPECT().List(gomock.Any(), workspaceID).Return([]*domain.WebhookSubscription{stored()}, nil)
			},
			call: func(ctx context.Context, s *WebhookSubscriptionService) (*domain.WebhookSubscription, error) {
				subs, err := s.List(ctx, workspaceID)
				if err != nil {
					return nil, err
				}
				if len(subs) == 0 {
					return nil, fmt.Errorf("expected one subscription")
				}
				return subs[0], nil
			},
		},
		"Update": {
			why: "editing a name must not be a way to read a key",
			arrange: func(repo *mocks.MockWebhookSubscriptionRepository, _ bool) {
				repo.EXPECT().GetByID(gomock.Any(), workspaceID, "sub-1").Return(stored(), nil)
				repo.EXPECT().Update(gomock.Any(), workspaceID, gomock.Any()).Return(nil)
			},
			call: func(ctx context.Context, s *WebhookSubscriptionService) (*domain.WebhookSubscription, error) {
				return s.Update(ctx, workspaceID, "sub-1", "crm", "https://x/h", []string{"contact.created"}, nil, boolPtr(true), nil, nil)
			},
		},
		"Toggle": {
			why: "the cheapest write there is, and so the easiest route to somebody else's key",
			arrange: func(repo *mocks.MockWebhookSubscriptionRepository, _ bool) {
				repo.EXPECT().GetByID(gomock.Any(), workspaceID, "sub-1").Return(stored(), nil)
				repo.EXPECT().Update(gomock.Any(), workspaceID, gomock.Any()).Return(nil)
			},
			call: func(ctx context.Context, s *WebhookSubscriptionService) (*domain.WebhookSubscription, error) {
				return s.Toggle(ctx, workspaceID, "sub-1", false)
			},
		},
		"RegenerateSecret": {
			memberSeesSecret: true,
			memberIsRejected: true,
			why:              "owner-only at the gate, and rotating a secret is pointless unless the owner receives the new one",
			arrange: func(repo *mocks.MockWebhookSubscriptionRepository, isOwner bool) {
				if !isOwner {
					// Nothing armed: the guard must reject before the repository.
					return
				}
				repo.EXPECT().GetByID(gomock.Any(), workspaceID, "sub-1").Return(stored(), nil)
				repo.EXPECT().Update(gomock.Any(), workspaceID, gomock.Any()).Return(nil)
			},
			call: func(ctx context.Context, s *WebhookSubscriptionService) (*domain.WebhookSubscription, error) {
				return s.RegenerateSecret(ctx, workspaceID, "sub-1")
			},
		},
		"GetForTestDelivery": {
			memberSeesSecret: true,
			why:              "an internal accessor: the caller signs the outbound test request with this and answers the client with the receiver's status code, never with the subscription",
			arrange: func(repo *mocks.MockWebhookSubscriptionRepository, _ bool) {
				repo.EXPECT().GetByID(gomock.Any(), workspaceID, "sub-1").Return(stored(), nil)
			},
			call: func(ctx context.Context, s *WebhookSubscriptionService) (*domain.WebhookSubscription, error) {
				return s.GetForTestDelivery(ctx, workspaceID, "sub-1")
			},
		},
	}

	t.Run("every method returning a subscription has a recorded decision", func(t *testing.T) {
		subscriptionType := reflect.TypeOf(&domain.WebhookSubscription{})
		serviceType := reflect.TypeOf(&WebhookSubscriptionService{})

		var covered int
		for i := 0; i < serviceType.NumMethod(); i++ {
			method := serviceType.Method(i)

			returnsSubscription := false
			for out := 0; out < method.Type.NumOut(); out++ {
				result := method.Type.Out(out)
				if result == subscriptionType ||
					(result.Kind() == reflect.Slice && result.Elem() == subscriptionType) {
					returnsSubscription = true
					break
				}
			}
			if !returnsSubscription {
				continue
			}

			covered++
			assert.Containsf(t, decisions, method.Name,
				"%s hands a subscription back to a handler but this table says nothing about its signing secret", method.Name)
		}

		assert.Equal(t, len(decisions), covered,
			"the table lists a method that no longer returns a subscription")
	})

	for name, tc := range decisions {
		t.Run("member/"+name, func(t *testing.T) {
			svc, repo := newService(t, "member")
			tc.arrange(repo, false)

			sub, err := tc.call(context.Background(), svc)

			if tc.memberIsRejected {
				require.Error(t, err, tc.why)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, sub)
			assert.Equal(t, "crm", sub.Name, "everything but the secret stays readable")

			if tc.memberSeesSecret {
				assert.NotEmpty(t, sub.Secret, tc.why)
				return
			}
			assert.Empty(t, sub.Secret, tc.why)
		})

		t.Run("owner/"+name, func(t *testing.T) {
			svc, repo := newService(t, "owner")
			tc.arrange(repo, true)

			sub, err := tc.call(context.Background(), svc)
			require.NoError(t, err)
			require.NotNil(t, sub)

			// An owner is never redacted anywhere: the console reveals the
			// secret from these payloads.
			assert.NotEmpty(t, sub.Secret)
		})
	}
}

// Deleting a subscription without taking its queue is what turns a normal
// integration turn-off into a permanent head-of-line block: the orphaned rows go
// on matching the worker's pending predicate for the whole retention window
// while they can never be delivered.
func TestWebhookSubscriptionService_DeleteTakesTheQueueWithIt(t *testing.T) {
	mockRepo, mockDeliveryRepo, _, service, ctrl := setupWebhookSubscriptionTest(t)
	defer ctrl.Finish()

	gomock.InOrder(
		// Deliveries first, so the subscription row can never disappear while
		// its queue survives.
		mockDeliveryRepo.EXPECT().DeleteBySubscriptionID(gomock.Any(), "ws-1", "sub-1").Return(nil),
		mockRepo.EXPECT().Delete(gomock.Any(), "ws-1", "sub-1").Return(nil),
	)

	require.NoError(t, service.Delete(context.Background(), "ws-1", "sub-1"))
}

// Source is write-once, and these are the two halves of that claim: Create is
// the only place it can be set, and Update — which takes no source at all — must
// leave it exactly as it was found.
func TestWebhookSubscriptionService_SourceIsWriteOnce(t *testing.T) {
	t.Run("Create stores the source it was given", func(t *testing.T) {
		mockRepo, _, _, service, ctrl := setupWebhookSubscriptionTest(t)
		defer ctrl.Finish()

		var stored *domain.WebhookSubscription
		mockRepo.EXPECT().
			Create(gomock.Any(), "ws-1", gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, sub *domain.WebhookSubscription) error {
				stored = sub
				return nil
			})

		_, err := service.Create(context.Background(), "ws-1", "Zap", "https://example.com/h",
			[]string{"contact.created"}, nil, domain.WebhookSubscriptionSourceZapier, nil, nil)
		require.NoError(t, err)
		require.NotNil(t, stored)
		assert.Equal(t, domain.WebhookSubscriptionSourceZapier, stored.Source)
	})

	t.Run("Update cannot re-attribute an existing subscription", func(t *testing.T) {
		mockRepo, _, _, service, ctrl := setupWebhookSubscriptionTest(t)
		defer ctrl.Finish()

		existing := &domain.WebhookSubscription{
			ID:     "sub-1",
			Name:   "Zap",
			URL:    "https://example.com/h",
			Secret: "whsec_" + strings.Repeat("a", 8),
			Source: domain.WebhookSubscriptionSourceZapier,
		}

		var written *domain.WebhookSubscription
		mockRepo.EXPECT().GetByID(gomock.Any(), "ws-1", "sub-1").Return(existing, nil)
		mockRepo.EXPECT().
			Update(gomock.Any(), "ws-1", gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, sub *domain.WebhookSubscription) error {
				written = sub
				return nil
			})

		_, err := service.Update(context.Background(), "ws-1", "sub-1", "Renamed", "https://example.com/h",
			[]string{"contact.created"}, nil, boolPtr(true), nil, nil)
		require.NoError(t, err)
		require.NotNil(t, written)
		assert.Equal(t, "Renamed", written.Name, "the rename should have gone through")
		assert.Equal(t, domain.WebhookSubscriptionSourceZapier, written.Source)
	})

	// An unrecognised value reads as "not user-created" everywhere while matching
	// none of the integration branches, and the column cannot be corrected after
	// the fact, so it has to be refused before the row is written.
	t.Run("Create refuses an unrecognised source without writing", func(t *testing.T) {
		mockRepo, _, _, service, ctrl := setupWebhookSubscriptionTest(t)
		defer ctrl.Finish()

		mockRepo.EXPECT().Create(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

		_, err := service.Create(context.Background(), "ws-1", "Evil", "https://example.com/h",
			[]string{"contact.created"}, nil, "evil", nil, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid webhook subscription source")
	})
}

// The id filters have to survive the trip into stored settings on both write
// paths, because a filter that is accepted and silently dropped is worse than
// one that was refused: the subscription reads as narrowed and is not.
func TestWebhookSubscriptionService_IDFiltersReachTheStoredSettings(t *testing.T) {
	t.Run("Create", func(t *testing.T) {
		mockRepo, _, _, service, ctrl := setupWebhookSubscriptionTest(t)
		defer ctrl.Finish()

		var stored *domain.WebhookSubscription
		mockRepo.EXPECT().
			Create(gomock.Any(), "ws-1", gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, sub *domain.WebhookSubscription) error {
				stored = sub
				return nil
			})

		_, err := service.Create(context.Background(), "ws-1", "Filtered", "https://example.com/h",
			[]string{"list.subscribed"}, nil, domain.WebhookSubscriptionSourceUser,
			[]string{"list-a"}, []string{"seg-a"})
		require.NoError(t, err)
		require.NotNil(t, stored)
		assert.Equal(t, []string{"list-a"}, stored.Settings.ListIDs)
		assert.Equal(t, []string{"seg-a"}, stored.Settings.SegmentIDs)
	})

	// Update patches the filters, so a caller that names them empty has to actually
	// clear them rather than have the omission-preserving path keep the previous
	// narrowing in place. This is the half that makes a filter removable at all.
	t.Run("Update clears the stored filters when the caller names them empty", func(t *testing.T) {
		mockRepo, _, _, service, ctrl := setupWebhookSubscriptionTest(t)
		defer ctrl.Finish()

		existing := &domain.WebhookSubscription{
			ID: "sub-1",
			Settings: domain.WebhookSubscriptionSettings{
				EventTypes: []string{"list.subscribed"},
				ListIDs:    []string{"list-a"},
				SegmentIDs: []string{"seg-a"},
			},
		}

		var written *domain.WebhookSubscription
		mockRepo.EXPECT().GetByID(gomock.Any(), "ws-1", "sub-1").Return(existing, nil)
		mockRepo.EXPECT().
			Update(gomock.Any(), "ws-1", gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, sub *domain.WebhookSubscription) error {
				written = sub
				return nil
			})

		cleared := []string{}
		_, err := service.Update(context.Background(), "ws-1", "sub-1", "Unfiltered", "https://example.com/h",
			[]string{"list.subscribed"}, nil, boolPtr(true), &cleared, &cleared)
		require.NoError(t, err)
		require.NotNil(t, written)
		// Nil, not empty: an empty filter is normalized away so that nothing reading
		// the stored settings can mistake a stored [] for "match nothing".
		assert.Nil(t, written.Settings.ListIDs)
		assert.Nil(t, written.Settings.SegmentIDs)
	})
}

// TestWebhookSubscriptionService_SwitchingOnClearsTheFailureHistory covers the
// half of the auto-disable feature the user gets to act on.
//
// Without it, re-enabling a subscription the sweep retired is a no-op with extra
// steps: the counter that retired it is still at the threshold and the window it
// was failing in still points at the old outage, so the very next failed
// delivery — a single 500 from a healthy endpoint — switches it straight back
// off. Turning a webhook back on is a statement that the endpoint has been
// fixed; the failure history is exactly what has to be forgotten for that to
// mean anything.
func TestWebhookSubscriptionService_SwitchingOnClearsTheFailureHistory(t *testing.T) {
	retired := func() *domain.WebhookSubscription {
		startedAt := time.Now().UTC().Add(-30 * time.Hour)
		reason := "automatically disabled after 20 consecutive delivery failures over 12h0m0s"
		return &domain.WebhookSubscription{
			ID:                  "sub123",
			Name:                "Zapier — New Contact",
			URL:                 "https://hooks.zapier.com/hooks/standard/1/2/",
			Secret:              "secret",
			Enabled:             false,
			ConsecutiveFailures: 20,
			FailingSince:        &startedAt,
			DisabledReason:      &reason,
		}
	}

	t.Run("toggling it on", func(t *testing.T) {
		mockRepo, _, _, service, ctrl := setupWebhookSubscriptionTest(t)
		defer ctrl.Finish()

		mockRepo.EXPECT().GetByID(gomock.Any(), "workspace123", "sub123").Return(retired(), nil)

		var written *domain.WebhookSubscription
		mockRepo.EXPECT().Update(gomock.Any(), "workspace123", gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, sub *domain.WebhookSubscription) error {
				written = sub
				return nil
			})

		_, err := service.Toggle(context.Background(), "workspace123", "sub123", true)
		require.NoError(t, err)

		require.NotNil(t, written)
		assert.True(t, written.Enabled)
		assert.Equal(t, 0, written.ConsecutiveFailures)
		assert.Nil(t, written.FailingSince, "a fresh start needs a fresh window")
		assert.Nil(t, written.DisabledReason)
	})

	t.Run("switching it off leaves the history to read", func(t *testing.T) {
		mockRepo, _, _, service, ctrl := setupWebhookSubscriptionTest(t)
		defer ctrl.Finish()

		enabled := retired()
		enabled.Enabled = true

		mockRepo.EXPECT().GetByID(gomock.Any(), "workspace123", "sub123").Return(enabled, nil)

		var written *domain.WebhookSubscription
		mockRepo.EXPECT().Update(gomock.Any(), "workspace123", gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, sub *domain.WebhookSubscription) error {
				written = sub
				return nil
			})

		_, err := service.Toggle(context.Background(), "workspace123", "sub123", false)
		require.NoError(t, err)

		require.NotNil(t, written)
		assert.False(t, written.Enabled)
		assert.Equal(t, 20, written.ConsecutiveFailures)
		assert.NotNil(t, written.FailingSince)
		assert.NotNil(t, written.DisabledReason)
	})
}

// TestWebhookSubscriptionService_Create_SecretGoesOnlyToAPerson pins who the freshly
// minted signing secret is returned to.
//
// It is returned to whoever created a subscription by hand, because that response is the
// one place they can read it and they need it to configure their receiver. It is withheld
// from an integration, because there is nobody on the other end of that call to copy it,
// a REST Hook target URL verifies no signature so the integration has no use for it — and
// the response body travels wherever that platform logs bodies. Zapier's core middleware
// logs every response body it receives, so returning the secret writes a live signing key
// into a third party's log store on every Zap turn-on.
func TestWebhookSubscriptionService_Create_SecretGoesOnlyToAPerson(t *testing.T) {
	create := func(t *testing.T, source string) *domain.WebhookSubscription {
		t.Helper()
		mockRepo, _, _, service, ctrl := setupWebhookSubscriptionTest(t)
		defer ctrl.Finish()

		// Copied by value at the moment of the write: the service hands the
		// repository the same pointer it returns, so reading Secret afterwards
		// would read whatever the response was left holding rather than what was
		// stored.
		var storedSecret string
		mockRepo.EXPECT().Create(gomock.Any(), "workspace123", gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, sub *domain.WebhookSubscription) error {
				storedSecret = sub.Secret
				return nil
			})

		created, err := service.Create(context.Background(), "workspace123", "Hook",
			"https://example.com/webhook", []string{"contact.created"}, nil, source, nil, nil)
		require.NoError(t, err)

		// Whatever the response shows, the row always keeps a real secret: it is
		// what every delivery is signed with.
		require.NotEmpty(t, storedSecret)
		return created
	}

	t.Run("a subscription a person created answers with its secret", func(t *testing.T) {
		created := create(t, domain.WebhookSubscriptionSourceUser)
		assert.NotEmpty(t, created.Secret)
	})

	t.Run("a subscription an integration created does not", func(t *testing.T) {
		created := create(t, domain.WebhookSubscriptionSourceZapier)
		assert.Empty(t, created.Secret,
			"an integration has no use for the key and its platform logs the response body")
	})
}

// An edit must not undo a retirement Notifuse decided on.
//
// A console form loaded while the subscription was healthy still carries an
// explicit true after the delivery worker has retired the endpoint. Saving a
// renamed webhook then switched it back on, cleared the reason that explained
// why it was off, and pointed the whole queue at a dead URL again — none of
// which the person renaming it asked for or was told about. Turning it back on
// is a claim that the endpoint has been fixed, so it goes through the toggle
// endpoint, deliberately.
func TestWebhookSubscriptionService_Update_WillNotResurrectAnAutoDisabledSubscription(t *testing.T) {
	mockRepo, _, _, service, ctrl := setupWebhookSubscriptionTest(t)
	defer ctrl.Finish()

	reason := "automatically disabled after 20 consecutive delivery failures over more than 2 hours (most recent response: HTTP 500)"
	mockRepo.EXPECT().
		GetByID(gomock.Any(), "workspace123", "sub123").
		Return(&domain.WebhookSubscription{
			ID:                  "sub123",
			Name:                "Old Name",
			URL:                 "https://example.com/webhook",
			Enabled:             false,
			ConsecutiveFailures: 20,
			DisabledReason:      &reason,
		}, nil)
	// Nothing arms Update: reaching the write at all is the bug.

	sub, err := service.Update(context.Background(), "workspace123", "sub123",
		"Renamed", "https://example.com/webhook", []string{"contact.created"}, nil,
		boolPtr(true), nil, nil)

	require.Error(t, err)
	assert.Nil(t, sub)
	// The message has to carry the reason, or the user is told no and not why.
	assert.Contains(t, err.Error(), "disabled automatically")
	assert.Contains(t, err.Error(), "HTTP 500")
}

// A subscription a person switched off carries no reason, and editing it back on
// is exactly what it looks like. The guard above must not turn every disabled
// webhook into one that can only be re-enabled from a second screen.
func TestWebhookSubscriptionService_Update_StillEnablesAUserDisabledSubscription(t *testing.T) {
	mockRepo, _, _, service, ctrl := setupWebhookSubscriptionTest(t)
	defer ctrl.Finish()

	mockRepo.EXPECT().
		GetByID(gomock.Any(), "workspace123", "sub123").
		Return(&domain.WebhookSubscription{
			ID:      "sub123",
			Name:    "Old Name",
			URL:     "https://example.com/webhook",
			Enabled: false,
		}, nil)
	mockRepo.EXPECT().
		Update(gomock.Any(), "workspace123", gomock.Any()).
		DoAndReturn(func(_ context.Context, _ string, sub *domain.WebhookSubscription) error {
			require.True(t, sub.Enabled)
			return nil
		})

	sub, err := service.Update(context.Background(), "workspace123", "sub123",
		"Renamed", "https://example.com/webhook", []string{"contact.created"}, nil,
		boolPtr(true), nil, nil)

	require.NoError(t, err)
	require.NotNil(t, sub)
	assert.True(t, sub.Enabled)
}

// And the guard is about re-enabling, not about editing: an auto-disabled
// subscription can still be renamed or repointed while it stays off, which is
// how a user fixes the URL that killed it before turning it back on.
func TestWebhookSubscriptionService_Update_StillEditsAnAutoDisabledSubscription(t *testing.T) {
	mockRepo, _, _, service, ctrl := setupWebhookSubscriptionTest(t)
	defer ctrl.Finish()

	reason := "automatically disabled after repeated delivery failures"
	mockRepo.EXPECT().
		GetByID(gomock.Any(), "workspace123", "sub123").
		Return(&domain.WebhookSubscription{
			ID:             "sub123",
			Name:           "Old Name",
			URL:            "https://old.example.com/webhook",
			Enabled:        false,
			DisabledReason: &reason,
		}, nil)
	mockRepo.EXPECT().
		Update(gomock.Any(), "workspace123", gomock.Any()).
		DoAndReturn(func(_ context.Context, _ string, sub *domain.WebhookSubscription) error {
			require.Equal(t, "https://new.example.com/webhook", sub.URL)
			require.False(t, sub.Enabled)
			return nil
		})

	sub, err := service.Update(context.Background(), "workspace123", "sub123",
		"Renamed", "https://new.example.com/webhook", []string{"contact.created"}, nil,
		boolPtr(false), nil, nil)

	require.NoError(t, err)
	require.NotNil(t, sub)
}

// An update that says nothing about the switch must not throw it.
//
// The console drawer renders no enabled control, so the value it sends is only
// ever an echo of what it last read — and when it sent nothing at all, the
// field decoded as false and every save disabled the subscription. That is not
// a cosmetic mistake: switching a subscription off drains its queued
// deliveries, and a drained delivery is pinned at max_attempts, outside the
// worker's claim predicate for good, so re-enabling never brings it back.
func TestWebhookSubscriptionService_Update_OmittedEnabledLeavesTheSwitchAlone(t *testing.T) {
	t.Run("an enabled subscription stays enabled", func(t *testing.T) {
		mockRepo, _, _, service, ctrl := setupWebhookSubscriptionTest(t)
		defer ctrl.Finish()

		mockRepo.EXPECT().
			GetByID(gomock.Any(), "workspace123", "sub123").
			Return(&domain.WebhookSubscription{
				ID:      "sub123",
				Name:    "Old Name",
				URL:     "https://example.com/webhook",
				Enabled: true,
			}, nil)

		var written *domain.WebhookSubscription
		mockRepo.EXPECT().
			Update(gomock.Any(), "workspace123", gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, sub *domain.WebhookSubscription) error {
				written = sub
				return nil
			})

		sub, err := service.Update(context.Background(), "workspace123", "sub123",
			"Renamed", "https://example.com/webhook", []string{"contact.created"}, nil,
			nil, nil, nil)

		require.NoError(t, err)
		require.NotNil(t, written)
		assert.True(t, written.Enabled, "a rename must not switch the subscription off")
		assert.Equal(t, "Renamed", written.Name, "the rest of the edit still has to land")
		require.NotNil(t, sub)
		assert.True(t, sub.Enabled)
	})

	// The mirror case: nil is "leave it alone", not "switch it on". It must
	// neither enable the subscription nor trip the guard that refuses to
	// resurrect one the delivery worker retired.
	t.Run("an auto-disabled subscription stays disabled and is still editable", func(t *testing.T) {
		mockRepo, _, _, service, ctrl := setupWebhookSubscriptionTest(t)
		defer ctrl.Finish()

		failingSince := time.Now().UTC().Add(-30 * time.Hour)
		reason := "automatically disabled after repeated delivery failures"
		mockRepo.EXPECT().
			GetByID(gomock.Any(), "workspace123", "sub123").
			Return(&domain.WebhookSubscription{
				ID:                  "sub123",
				Name:                "Old Name",
				URL:                 "https://old.example.com/webhook",
				Enabled:             false,
				ConsecutiveFailures: 20,
				FailingSince:        &failingSince,
				DisabledReason:      &reason,
			}, nil)

		var written *domain.WebhookSubscription
		mockRepo.EXPECT().
			Update(gomock.Any(), "workspace123", gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, sub *domain.WebhookSubscription) error {
				written = sub
				return nil
			})

		sub, err := service.Update(context.Background(), "workspace123", "sub123",
			"Renamed", "https://new.example.com/webhook", []string{"contact.created"}, nil,
			nil, nil, nil)

		require.NoError(t, err, "an edit that never mentions the switch is not a re-enable")
		require.NotNil(t, written)
		assert.False(t, written.Enabled)
		assert.Equal(t, "https://new.example.com/webhook", written.URL)
		// The failure history explains the retirement, and nothing here has
		// claimed the endpoint is fixed.
		assert.Equal(t, 20, written.ConsecutiveFailures)
		assert.NotNil(t, written.FailingSince)
		assert.NotNil(t, written.DisabledReason)
		require.NotNil(t, sub)
	})
}

// TestWebhookSubscriptionService_Update_OmittedFiltersLeaveTheStoredOnesAlone is the
// three narrowing filters' version of the omitted-enabled test above.
//
// Their zero value is not "unset", it is "no filter at all", so replacing them from a
// body that never mentioned them widens the subscription instead of leaving it alone:
// a Zap registered against one list starts receiving a delivery for every list and
// every segment in the workspace, and its custom-event narrowing disappears too.
// Nothing reports the change — the deliveries simply arrive.
func TestWebhookSubscriptionService_Update_OmittedFiltersLeaveTheStoredOnesAlone(t *testing.T) {
	mockRepo, _, _, svc, ctrl := setupWebhookSubscriptionTest(t)
	defer ctrl.Finish()

	// Seeded with every filter populated: a stored blank would make a wipe
	// indistinguishable from a correct merge.
	mockRepo.EXPECT().
		GetByID(gomock.Any(), "ws-1", "sub-1").
		Return(&domain.WebhookSubscription{
			ID:      "sub-1",
			Name:    "Zap: new contact to Slack",
			URL:     "https://hooks.zapier.com/hook",
			Enabled: true,
			Source:  domain.WebhookSubscriptionSourceZapier,
			Settings: domain.WebhookSubscriptionSettings{
				EventTypes:         []string{"list.subscribed"},
				ListIDs:            []string{"list-a"},
				SegmentIDs:         []string{"seg-a"},
				CustomEventFilters: &domain.CustomEventFilters{GoalTypes: []string{"purchase"}},
			},
		}, nil)

	var written *domain.WebhookSubscription
	mockRepo.EXPECT().
		Update(gomock.Any(), "ws-1", gomock.Any()).
		DoAndReturn(func(_ context.Context, _ string, sub *domain.WebhookSubscription) error {
			written = sub
			return nil
		})

	_, err := svc.Update(context.Background(), "ws-1", "sub-1",
		"Renamed", "https://hooks.zapier.com/hook",
		[]string{"list.subscribed"}, nil, nil, nil, nil)

	require.NoError(t, err)
	require.NotNil(t, written)
	// Literals, not fields read off the fixture: the repository hands the service the
	// very object it mutates, so comparing against the fixture would compare a field
	// with itself and pass whatever happened.
	assert.Equal(t, []string{"list-a"}, written.Settings.ListIDs,
		"a rename that named no list filter must not widen the subscription to every list")
	assert.Equal(t, []string{"seg-a"}, written.Settings.SegmentIDs,
		"a rename that named no segment filter must not widen the subscription to every segment")
	assert.Equal(t, &domain.CustomEventFilters{GoalTypes: []string{"purchase"}}, written.Settings.CustomEventFilters,
		"a rename that named no custom event filter must not widen the subscription to every custom event")
	// The edit itself still has to land, so a dropped update cannot pass for a merge.
	assert.Equal(t, "Renamed", written.Name)
	assert.Equal(t, []string{"list.subscribed"}, written.Settings.EventTypes)
}
