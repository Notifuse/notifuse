package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"testing"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMailjetService_UpdateWebhook(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mocks
	mockHTTPClient := mocks.NewMockHTTPClient(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)
	testLogger := logger.NewLogger()

	// Create service with mocks
	service := NewMailjetService(mockHTTPClient, mockAuthService, testLogger)

	// Test data
	config := domain.MailjetSettings{
		APIKey:    "test-api-key",
		SecretKey: "test-secret-key",
	}

	webhookID := int64(123)
	webhookToUpdate := domain.MailjetWebhook{
		EventType: string(domain.MailjetEventClick),
		Endpoint:  "https://example.com/webhook-updated",
		Status:    "active",
	}

	ctx := context.Background()

	t.Run("Successfully update webhook", func(t *testing.T) {
		// Expected response from Mailjet API
		expectedResponse := domain.MailjetWebhook{
			ID:        webhookID,
			EventType: webhookToUpdate.EventType,
			Endpoint:  webhookToUpdate.Endpoint,
			Status:    webhookToUpdate.Status,
		}

		// Setup mock expectations
		mockHTTPClient.EXPECT().
			Do(gomock.Any()).
			DoAndReturn(func(req *http.Request) (*http.Response, error) {
				// Verify request
				assert.Equal(t, "PUT", req.Method)
				assert.Equal(t, "https://api.mailjet.com/v3/REST/eventcallbackurl/123", req.URL.String())

				// Verify auth header
				username, password, ok := req.BasicAuth()
				assert.True(t, ok)
				assert.Equal(t, config.APIKey, username)
				assert.Equal(t, config.SecretKey, password)

				// Verify request body
				body, err := io.ReadAll(req.Body)
				require.NoError(t, err)

				var sentWebhook domain.MailjetWebhook
				err = json.Unmarshal(body, &sentWebhook)
				require.NoError(t, err)

				assert.Equal(t, webhookToUpdate.EventType, sentWebhook.EventType)
				assert.Equal(t, webhookToUpdate.Endpoint, sentWebhook.Endpoint)
				assert.Equal(t, webhookToUpdate.Status, sentWebhook.Status)

				return mockHTTPResponse(t, http.StatusOK, expectedResponse), nil
			})

		// Call the service method
		response, err := service.UpdateWebhook(ctx, config, webhookID, webhookToUpdate)

		// Assertions
		require.NoError(t, err)
		require.NotNil(t, response)
		assert.Equal(t, webhookID, response.ID)
		assert.Equal(t, webhookToUpdate.EventType, response.EventType)
		assert.Equal(t, webhookToUpdate.Endpoint, response.Endpoint)
	})

	t.Run("HTTP client error", func(t *testing.T) {
		// Setup mock expectations
		mockHTTPClient.EXPECT().
			Do(gomock.Any()).
			Return(nil, errors.New("network error"))

		// Call the service method
		response, err := service.UpdateWebhook(ctx, config, webhookID, webhookToUpdate)

		// Assertions
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to execute request")
		assert.Nil(t, response)
	})

	t.Run("API returns error status code", func(t *testing.T) {
		// Setup mock expectations
		mockHTTPClient.EXPECT().
			Do(gomock.Any()).
			Return(mockHTTPResponse(t, http.StatusBadRequest, nil), nil)

		// Call the service method
		response, err := service.UpdateWebhook(ctx, config, webhookID, webhookToUpdate)

		// Assertions
		require.Error(t, err)
		assert.Contains(t, err.Error(), "API returned non-OK status code")
		assert.Nil(t, response)
	})
}

// Test error cases for RegisterWebhooks
func TestMailjetService_RegisterWebhooks_Errors(t *testing.T) {
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
	baseURL := "https://api.notifuse.com"

	providerConfig := &domain.EmailProvider{
		Kind: domain.EmailProviderKindMailjet,
		Mailjet: &domain.MailjetSettings{
			APIKey:    "test-api-key",
			SecretKey: "test-secret-key",
		},
	}

	ctx := context.Background()

	// Test error cases
	t.Run("Error listing webhooks", func(t *testing.T) {
		eventTypes := []domain.EmailEventType{domain.EmailEventDelivered}

		// Create a specific controller and mock for this test
		ctrlError := gomock.NewController(t)
		defer ctrlError.Finish()

		mockHTTPClientError := mocks.NewMockHTTPClient(ctrlError)

		// Create a service with the error mock
		serviceError := NewMailjetService(mockHTTPClientError, mockAuthService, testLogger)

		// Setup mock for ListWebhooks - return error
		mockHTTPClientError.EXPECT().
			Do(gomock.Any()).
			Return(nil, errors.New("network error"))

		// Call the service method
		status, err := serviceError.RegisterWebhooks(ctx, workspaceID, integrationID, baseURL, eventTypes, providerConfig)

		// Assertions
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to list Mailjet webhooks")
		assert.Nil(t, status)
	})

	t.Run("Missing provider configuration", func(t *testing.T) {
		eventTypes := []domain.EmailEventType{domain.EmailEventDelivered}

		// Call with nil provider config
		status, err := service.RegisterWebhooks(ctx, workspaceID, integrationID, baseURL, eventTypes, nil)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "mailjet configuration is missing or invalid")
		assert.Nil(t, status)

		// Call with empty Mailjet config
		emptyConfig := &domain.EmailProvider{
			Kind:    domain.EmailProviderKindMailjet,
			Mailjet: &domain.MailjetSettings{},
		}

		status, err = service.RegisterWebhooks(ctx, workspaceID, integrationID, baseURL, eventTypes, emptyConfig)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "mailjet configuration is missing or invalid")
		assert.Nil(t, status)
	})
}
