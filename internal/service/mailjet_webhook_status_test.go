package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMailjetService_GetWebhookStatus(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mocks
	mockHTTPClient := mocks.NewMockHTTPClient(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)
	testLogger := logger.NewLogger()

	// Create service with mocks
	service := NewMailjetService(mockHTTPClient, mockAuthService, testLogger)

	// Test data
	workspaceID := "workspace-123"
	integrationID := "integration-456"

	providerConfig := &domain.EmailProvider{
		Kind: domain.EmailProviderKindMailjet,
		Mailjet: &domain.MailjetSettings{
			APIKey:    "test-api-key",
			SecretKey: "test-secret-key",
		},
	}

	ctx := context.Background()

	t.Run("No webhooks registered", func(t *testing.T) {
		// Create a specific controller and mock for this test
		ctrlEmpty := gomock.NewController(t)
		defer ctrlEmpty.Finish()

		mockEmptyClient := mocks.NewMockHTTPClient(ctrlEmpty)

		// Create service with the test-specific mock
		emptyService := NewMailjetService(mockEmptyClient, mockAuthService, testLogger)

		// Setup mock for ListWebhooks - return empty list
		mockEmptyClient.EXPECT().
			Do(gomock.Any()).
			DoAndReturn(func(req *http.Request) (*http.Response, error) {
				if strings.Contains(req.URL.String(), "eventcallbackurl") && req.Method == "GET" {
					emptyResponse := domain.MailjetWebhookResponse{
						Count: 0,
						Data:  []domain.MailjetWebhook{},
						Total: 0,
					}
					return mockHTTPResponse(t, http.StatusOK, emptyResponse), nil
				}
				t.Fatalf("Unexpected request: %s %s", req.Method, req.URL.String())
				return nil, errors.New("unexpected request")
			})

		// Call the service method
		status, err := emptyService.GetWebhookStatus(ctx, workspaceID, integrationID, providerConfig)

		// Assertions
		require.NoError(t, err)
		require.NotNil(t, status)
		assert.Equal(t, domain.EmailProviderKindMailjet, status.EmailProviderKind)
		assert.False(t, status.IsRegistered)
		assert.Empty(t, status.Endpoints)
	})

	t.Run("Webhooks registered for integration", func(t *testing.T) {
		// Create a specific controller and mock for this test
		ctrlRegistered := gomock.NewController(t)
		defer ctrlRegistered.Finish()

		mockRegisteredClient := mocks.NewMockHTTPClient(ctrlRegistered)

		// Create service with the test-specific mock
		registeredService := NewMailjetService(mockRegisteredClient, mockAuthService, testLogger)

		// Create webhook URL that includes workspace_id and integration_id
		webhookURL := domain.GenerateWebhookCallbackURL("https://api.notifuse.com", domain.EmailProviderKindMailjet, workspaceID, integrationID)

		// Setup mock for ListWebhooks - return list with webhooks for this integration
		mockRegisteredClient.EXPECT().
			Do(gomock.Any()).
			DoAndReturn(func(req *http.Request) (*http.Response, error) {
				if strings.Contains(req.URL.String(), "eventcallbackurl") && req.Method == "GET" {
					webhooksResponse := domain.MailjetWebhookResponse{
						Count: 3,
						Data: []domain.MailjetWebhook{
							{
								ID:        101,
								EventType: string(domain.MailjetEventSent),
								Endpoint:  webhookURL,
								Status:    "alive",
							},
							{
								ID:        102,
								EventType: string(domain.MailjetEventBounce),
								Endpoint:  webhookURL,
								Status:    "alive",
							},
							{
								ID:        103,
								EventType: string(domain.MailjetEventClick),
								Endpoint:  "https://other-endpoint.com/webhook", // Different integration
								Status:    "alive",
							},
						},
						Total: 3,
					}
					return mockHTTPResponse(t, http.StatusOK, webhooksResponse), nil
				}
				t.Fatalf("Unexpected request: %s %s", req.Method, req.URL.String())
				return nil, errors.New("unexpected request")
			})

		// Call the service method
		status, err := registeredService.GetWebhookStatus(ctx, workspaceID, integrationID, providerConfig)

		// Debug
		fmt.Printf("Status endpoints: %+v\n", status.Endpoints)

		// Assertions
		require.NoError(t, err)
		require.NotNil(t, status)
		assert.Equal(t, domain.EmailProviderKindMailjet, status.EmailProviderKind)
		assert.True(t, status.IsRegistered)

		// The service implementation processes all matching webhooks
		// First webhook (ID 101) is MailjetEventSent -> EmailEventDelivered
		// Second webhook (ID 102) is MailjetEventBounce -> EmailEventBounce
		assert.Len(t, status.Endpoints, 2, "Should have two endpoints corresponding to the matching webhooks")

		// Check first endpoint (sent event)
		assert.Equal(t, domain.EmailEventDelivered, status.Endpoints[0].EventType)
		assert.Equal(t, webhookURL, status.Endpoints[0].URL)
		assert.Equal(t, "101", status.Endpoints[0].WebhookID)

		// Check second endpoint (bounce event)
		assert.Equal(t, domain.EmailEventBounce, status.Endpoints[1].EventType)
		assert.Equal(t, webhookURL, status.Endpoints[1].URL)
		assert.Equal(t, "102", status.Endpoints[1].WebhookID)
	})

	t.Run("Missing provider configuration", func(t *testing.T) {
		// Call with nil provider config
		status, err := service.GetWebhookStatus(ctx, workspaceID, integrationID, nil)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "mailjet configuration is missing or invalid")
		assert.Nil(t, status)

		// Call with empty Mailjet config
		emptyConfig := &domain.EmailProvider{
			Kind:    domain.EmailProviderKindMailjet,
			Mailjet: &domain.MailjetSettings{},
		}

		status, err = service.GetWebhookStatus(ctx, workspaceID, integrationID, emptyConfig)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "mailjet configuration is missing or invalid")
		assert.Nil(t, status)
	})

	t.Run("Error listing webhooks", func(t *testing.T) {
		// Create a specific controller and mock for this test
		ctrlError := gomock.NewController(t)
		defer ctrlError.Finish()

		mockErrorClient := mocks.NewMockHTTPClient(ctrlError)

		// Create service with the test-specific mock
		errorService := NewMailjetService(mockErrorClient, mockAuthService, testLogger)

		// Setup mock for ListWebhooks - return error
		mockErrorClient.EXPECT().
			Do(gomock.Any()).
			Return(nil, errors.New("network error"))

		// Call the service method
		status, err := errorService.GetWebhookStatus(ctx, workspaceID, integrationID, providerConfig)

		// Assertions
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to list Mailjet webhooks")
		assert.Nil(t, status)
	})
}
