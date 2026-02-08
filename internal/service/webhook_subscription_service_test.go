package service

import (
	"context"
	"errors"
	"fmt"
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

	// AuthService is not used by WebhookSubscriptionService methods, so pass nil
	service := NewWebhookSubscriptionService(mockRepo, mockDeliveryRepo, nil, mockLogger)

	return mockRepo, mockDeliveryRepo, mockLogger, service, ctrl
}

func TestNewWebhookSubscriptionService(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockWebhookSubscriptionRepository(ctrl)
	mockDeliveryRepo := mocks.NewMockWebhookDeliveryRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	service := NewWebhookSubscriptionService(mockRepo, mockDeliveryRepo, nil, mockLogger)

	require.NotNil(t, service)
	require.Equal(t, mockRepo, service.repo)
	require.Equal(t, mockDeliveryRepo, service.deliveryRepo)
	require.Nil(t, service.authService)
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
		enabled            bool
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
			enabled:     true,
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
		setupMocks  func(*mocks.MockWebhookSubscriptionRepository)
		expectError bool
	}{
		{
			name:        "successful deletion",
			workspaceID: "workspace123",
			subID:       "sub123",
			setupMocks: func(mockRepo *mocks.MockWebhookSubscriptionRepository) {
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
			setupMocks: func(mockRepo *mocks.MockWebhookSubscriptionRepository) {
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
			setupMocks: func(mockRepo *mocks.MockWebhookSubscriptionRepository) {
				mockRepo.EXPECT().
					Delete(gomock.Any(), "workspace123", "sub123").
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
	// Test that generateSecret produces valid secrets
	secrets := make(map[string]bool)

	for i := 0; i < 100; i++ {
		secret, err := generateSecret()
		require.NoError(t, err)
		require.NotEmpty(t, secret)

		// Secret should be base64 encoded 32 bytes (~44 chars)
		require.Greater(t, len(secret), 40)

		// Each secret should be unique
		require.False(t, secrets[secret], "Secret should be unique")
		secrets[secret] = true
	}
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
		true,
	)
	require.NoError(t, err)
}
