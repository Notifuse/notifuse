package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockResponse creates a mock HTTP response
func createMockResponse(statusCode int, body string) *http.Response {
	return &http.Response{
		StatusCode: statusCode,
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

// setupPostmarkTest creates all the necessary mocks for testing the PostmarkService
func setupPostmarkTest(t *testing.T) (*PostmarkService, *mocks.MockHTTPClient, *mocks.MockAuthService, *pkgmocks.MockLogger) {
	ctrl := gomock.NewController(t)
	httpClient := mocks.NewMockHTTPClient(ctrl)
	authService := mocks.NewMockAuthService(ctrl)
	logger := pkgmocks.NewMockLogger(ctrl)

	// Set up logger expectations
	logger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(logger).AnyTimes()
	logger.EXPECT().WithFields(gomock.Any()).Return(logger).AnyTimes()
	logger.EXPECT().Error(gomock.Any()).AnyTimes()
	logger.EXPECT().Info(gomock.Any()).AnyTimes()

	service := NewPostmarkService(httpClient, authService, logger)
	return service, httpClient, authService, logger
}

// Standard test configuration
var (
	testConfig = domain.PostmarkSettings{
		ServerToken: "test-server-token",
	}
)

func TestPostmarkService_ListWebhooks(t *testing.T) {
	t.Run("Successfully list webhooks", func(t *testing.T) {
		// Setup
		service, httpClient, _, _ := setupPostmarkTest(t)

		// Mock response data
		responseData := domain.PostmarkListWebhooksResponse{
			Webhooks: []domain.PostmarkWebhookResponse{
				{
					ID:            123,
					URL:           "https://api.notifuse.com/webhooks/email?provider=postmark&workspace_id=ws-123&integration_id=int-456",
					MessageStream: "outbound",
					Triggers: &domain.PostmarkTriggers{
						Delivery: &domain.PostmarkDeliveryTrigger{Enabled: true},
						Bounce:   &domain.PostmarkBounceTrigger{Enabled: true},
					},
				},
			},
		}
		responseBody, _ := json.Marshal(responseData)

		// Expect HTTP request
		httpClient.EXPECT().
			Do(gomock.Any()).
			DoAndReturn(func(req *http.Request) (*http.Response, error) {
				// Verify request
				assert.Equal(t, "GET", req.Method)
				assert.Equal(t, "https://api.postmarkapp.com/webhooks", req.URL.String())
				assert.Equal(t, "application/json", req.Header.Get("Content-Type"))
				assert.Equal(t, testConfig.ServerToken, req.Header.Get("X-Postmark-Server-Token"))

				return createMockResponse(http.StatusOK, string(responseBody)), nil
			})

		// Call the method
		result, err := service.ListWebhooks(context.Background(), testConfig)

		// Verify results
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Len(t, result.Webhooks, 1)
		assert.Equal(t, 123, result.Webhooks[0].ID)
	})

	t.Run("HTTP client error", func(t *testing.T) {
		// Setup
		service, httpClient, _, _ := setupPostmarkTest(t)

		// Simulate HTTP error
		httpClient.EXPECT().
			Do(gomock.Any()).
			Return(nil, errors.New("network error"))

		// Call the method
		result, err := service.ListWebhooks(context.Background(), testConfig)

		// Verify results
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "failed to execute request")
	})

	t.Run("API returns error status code", func(t *testing.T) {
		// Setup
		service, httpClient, _, _ := setupPostmarkTest(t)

		// Simulate 401 response
		httpClient.EXPECT().
			Do(gomock.Any()).
			Return(createMockResponse(http.StatusUnauthorized, ""), nil)

		// Call the method
		result, err := service.ListWebhooks(context.Background(), testConfig)

		// Verify results
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "API returned non-OK status code 401")
	})

	t.Run("Malformed response", func(t *testing.T) {
		// Setup
		service, httpClient, _, _ := setupPostmarkTest(t)

		// Simulate invalid JSON
		httpClient.EXPECT().
			Do(gomock.Any()).
			Return(createMockResponse(http.StatusOK, "invalid json"), nil)

		// Call the method
		result, err := service.ListWebhooks(context.Background(), testConfig)

		// Verify results
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "failed to decode response")
	})
}

func TestPostmarkService_RegisterWebhook(t *testing.T) {
	t.Run("Successfully register webhook", func(t *testing.T) {
		// Setup
		service, httpClient, _, _ := setupPostmarkTest(t)

		// Test webhook configuration
		webhookConfig := domain.PostmarkWebhookConfig{
			URL:           "https://api.notifuse.com/webhooks/email",
			MessageStream: "outbound",
			Triggers: &domain.PostmarkTriggers{
				Delivery: &domain.PostmarkDeliveryTrigger{Enabled: true},
				Bounce:   &domain.PostmarkBounceTrigger{Enabled: true},
			},
		}

		// Mock response data
		responseData := domain.PostmarkWebhookResponse{
			ID:            123,
			URL:           webhookConfig.URL,
			MessageStream: webhookConfig.MessageStream,
			Triggers:      webhookConfig.Triggers,
		}
		responseBody, _ := json.Marshal(responseData)

		// Expect HTTP request
		httpClient.EXPECT().
			Do(gomock.Any()).
			DoAndReturn(func(req *http.Request) (*http.Response, error) {
				// Verify request
				assert.Equal(t, "POST", req.Method)
				assert.Equal(t, "https://api.postmarkapp.com/webhooks", req.URL.String())
				assert.Equal(t, "application/json", req.Header.Get("Content-Type"))
				assert.Equal(t, testConfig.ServerToken, req.Header.Get("X-Postmark-Server-Token"))

				// Verify request body
				body, _ := io.ReadAll(req.Body)
				var requestedConfig domain.PostmarkWebhookConfig
				err := json.Unmarshal(body, &requestedConfig)
				assert.NoError(t, err)
				assert.Equal(t, webhookConfig.URL, requestedConfig.URL)

				return createMockResponse(http.StatusCreated, string(responseBody)), nil
			})

		// Call the method
		result, err := service.RegisterWebhook(context.Background(), testConfig, webhookConfig)

		// Verify results
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, 123, result.ID)
		assert.Equal(t, webhookConfig.URL, result.URL)
	})

	t.Run("HTTP client error", func(t *testing.T) {
		// Setup
		service, httpClient, _, _ := setupPostmarkTest(t)

		// Test webhook configuration
		webhookConfig := domain.PostmarkWebhookConfig{
			URL:           "https://api.notifuse.com/webhooks/email",
			MessageStream: "outbound",
		}

		// Simulate HTTP error
		httpClient.EXPECT().
			Do(gomock.Any()).
			Return(nil, errors.New("network error"))

		// Call the method
		result, err := service.RegisterWebhook(context.Background(), testConfig, webhookConfig)

		// Verify results
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "failed to execute request")
	})

	t.Run("API returns error status code", func(t *testing.T) {
		// Setup
		service, httpClient, _, _ := setupPostmarkTest(t)

		// Test webhook configuration
		webhookConfig := domain.PostmarkWebhookConfig{
			URL:           "https://api.notifuse.com/webhooks/email",
			MessageStream: "outbound",
		}

		// Simulate 400 response
		httpClient.EXPECT().
			Do(gomock.Any()).
			Return(createMockResponse(http.StatusBadRequest, ""), nil)

		// Call the method
		result, err := service.RegisterWebhook(context.Background(), testConfig, webhookConfig)

		// Verify results
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "API returned non-OK status code 400")
	})

	// Skipping the marshal error test as it requires modifying the json.Marshal function which is complex in Go.
	// In a real-world scenario, we could use a mock/stub implementation of the service with dependency injection.
}

func TestPostmarkService_UnregisterWebhook(t *testing.T) {
	t.Run("Successfully unregister webhook", func(t *testing.T) {
		// Setup
		service, httpClient, _, _ := setupPostmarkTest(t)
		webhookID := 123

		// Expect HTTP request
		httpClient.EXPECT().
			Do(gomock.Any()).
			DoAndReturn(func(req *http.Request) (*http.Response, error) {
				// Verify request
				assert.Equal(t, "DELETE", req.Method)
				assert.Equal(t, "https://api.postmarkapp.com/webhooks/123", req.URL.String())
				assert.Equal(t, testConfig.ServerToken, req.Header.Get("X-Postmark-Server-Token"))

				return createMockResponse(http.StatusOK, "{}"), nil
			})

		// Call the method
		err := service.UnregisterWebhook(context.Background(), testConfig, webhookID)

		// Verify results
		assert.NoError(t, err)
	})

	t.Run("HTTP client error", func(t *testing.T) {
		// Setup
		service, httpClient, _, _ := setupPostmarkTest(t)
		webhookID := 123

		// Simulate HTTP error
		httpClient.EXPECT().
			Do(gomock.Any()).
			Return(nil, errors.New("network error"))

		// Call the method
		err := service.UnregisterWebhook(context.Background(), testConfig, webhookID)

		// Verify results
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to execute request")
	})

	t.Run("API returns error status code", func(t *testing.T) {
		// Setup
		service, httpClient, _, _ := setupPostmarkTest(t)
		webhookID := 123

		// Simulate 404 response
		httpClient.EXPECT().
			Do(gomock.Any()).
			Return(createMockResponse(http.StatusNotFound, ""), nil)

		// Call the method
		err := service.UnregisterWebhook(context.Background(), testConfig, webhookID)

		// Verify results
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "API returned non-OK status code 404")
	})
}

func TestPostmarkService_GetWebhook(t *testing.T) {
	t.Run("Successfully get webhook", func(t *testing.T) {
		// Setup
		service, httpClient, _, _ := setupPostmarkTest(t)
		webhookID := 123

		// Mock response data
		responseData := domain.PostmarkWebhookResponse{
			ID:            webhookID,
			URL:           "https://api.notifuse.com/webhooks/email",
			MessageStream: "outbound",
			Triggers: &domain.PostmarkTriggers{
				Delivery: &domain.PostmarkDeliveryTrigger{Enabled: true},
				Bounce:   &domain.PostmarkBounceTrigger{Enabled: true},
			},
		}
		responseBody, _ := json.Marshal(responseData)

		// Expect HTTP request
		httpClient.EXPECT().
			Do(gomock.Any()).
			DoAndReturn(func(req *http.Request) (*http.Response, error) {
				// Verify request
				assert.Equal(t, "GET", req.Method)
				assert.Equal(t, "https://api.postmarkapp.com/webhooks/123", req.URL.String())
				assert.Equal(t, testConfig.ServerToken, req.Header.Get("X-Postmark-Server-Token"))

				return createMockResponse(http.StatusOK, string(responseBody)), nil
			})

		// Call the method
		result, err := service.GetWebhook(context.Background(), testConfig, webhookID)

		// Verify results
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, webhookID, result.ID)
		assert.Equal(t, "https://api.notifuse.com/webhooks/email", result.URL)
		assert.Equal(t, "outbound", result.MessageStream)
		assert.True(t, result.Triggers.Delivery.Enabled)
		assert.True(t, result.Triggers.Bounce.Enabled)
	})

	t.Run("HTTP client error", func(t *testing.T) {
		// Setup
		service, httpClient, _, _ := setupPostmarkTest(t)
		webhookID := 123

		// Simulate HTTP error
		httpClient.EXPECT().
			Do(gomock.Any()).
			Return(nil, errors.New("network error"))

		// Call the method
		result, err := service.GetWebhook(context.Background(), testConfig, webhookID)

		// Verify results
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "failed to execute request")
	})

	t.Run("API returns error status code", func(t *testing.T) {
		// Setup
		service, httpClient, _, _ := setupPostmarkTest(t)
		webhookID := 123

		// Simulate 404 response
		httpClient.EXPECT().
			Do(gomock.Any()).
			Return(createMockResponse(http.StatusNotFound, ""), nil)

		// Call the method
		result, err := service.GetWebhook(context.Background(), testConfig, webhookID)

		// Verify results
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "API returned non-OK status code 404")
	})

	t.Run("Malformed response", func(t *testing.T) {
		// Setup
		service, httpClient, _, _ := setupPostmarkTest(t)
		webhookID := 123

		// Simulate invalid JSON
		httpClient.EXPECT().
			Do(gomock.Any()).
			Return(createMockResponse(http.StatusOK, "invalid json"), nil)

		// Call the method
		result, err := service.GetWebhook(context.Background(), testConfig, webhookID)

		// Verify results
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "failed to decode response")
	})
}

func TestPostmarkService_UpdateWebhook(t *testing.T) {
	t.Run("Successfully update webhook", func(t *testing.T) {
		// Setup
		service, httpClient, _, _ := setupPostmarkTest(t)
		webhookID := 123

		// Test webhook configuration
		webhookConfig := domain.PostmarkWebhookConfig{
			URL:           "https://api.notifuse.com/webhooks/email/updated",
			MessageStream: "outbound",
			Triggers: &domain.PostmarkTriggers{
				Delivery:      &domain.PostmarkDeliveryTrigger{Enabled: true},
				Bounce:        &domain.PostmarkBounceTrigger{Enabled: true},
				SpamComplaint: &domain.PostmarkSpamComplaintTrigger{Enabled: true},
			},
		}

		// Mock response data
		responseData := domain.PostmarkWebhookResponse{
			ID:            webhookID,
			URL:           webhookConfig.URL,
			MessageStream: webhookConfig.MessageStream,
			Triggers:      webhookConfig.Triggers,
		}
		responseBody, _ := json.Marshal(responseData)

		// Expect HTTP request
		httpClient.EXPECT().
			Do(gomock.Any()).
			DoAndReturn(func(req *http.Request) (*http.Response, error) {
				// Verify request
				assert.Equal(t, "PUT", req.Method)
				assert.Equal(t, "https://api.postmarkapp.com/webhooks/123", req.URL.String())
				assert.Equal(t, "application/json", req.Header.Get("Content-Type"))
				assert.Equal(t, testConfig.ServerToken, req.Header.Get("X-Postmark-Server-Token"))

				// Verify request body
				body, _ := io.ReadAll(req.Body)
				var requestedConfig domain.PostmarkWebhookConfig
				err := json.Unmarshal(body, &requestedConfig)
				assert.NoError(t, err)
				assert.Equal(t, webhookConfig.URL, requestedConfig.URL)
				assert.True(t, requestedConfig.Triggers.SpamComplaint.Enabled)

				return createMockResponse(http.StatusOK, string(responseBody)), nil
			})

		// Call the method
		result, err := service.UpdateWebhook(context.Background(), testConfig, webhookID, webhookConfig)

		// Verify results
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, webhookID, result.ID)
		assert.Equal(t, webhookConfig.URL, result.URL)
		assert.True(t, result.Triggers.SpamComplaint.Enabled)
	})

	t.Run("HTTP client error", func(t *testing.T) {
		// Setup
		service, httpClient, _, _ := setupPostmarkTest(t)
		webhookID := 123

		// Test webhook configuration
		webhookConfig := domain.PostmarkWebhookConfig{
			URL:           "https://api.notifuse.com/webhooks/email",
			MessageStream: "outbound",
		}

		// Simulate HTTP error
		httpClient.EXPECT().
			Do(gomock.Any()).
			Return(nil, errors.New("network error"))

		// Call the method
		result, err := service.UpdateWebhook(context.Background(), testConfig, webhookID, webhookConfig)

		// Verify results
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "failed to execute request")
	})

	t.Run("API returns error status code", func(t *testing.T) {
		// Setup
		service, httpClient, _, _ := setupPostmarkTest(t)
		webhookID := 123

		// Test webhook configuration
		webhookConfig := domain.PostmarkWebhookConfig{
			URL:           "https://api.notifuse.com/webhooks/email",
			MessageStream: "outbound",
		}

		// Simulate 400 response
		httpClient.EXPECT().
			Do(gomock.Any()).
			Return(createMockResponse(http.StatusBadRequest, ""), nil)

		// Call the method
		result, err := service.UpdateWebhook(context.Background(), testConfig, webhookID, webhookConfig)

		// Verify results
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "API returned non-OK status code 400")
	})

	// Skipping the marshal error test as it requires modifying the json.Marshal function which is complex in Go.
	// In a real-world scenario, we could use a mock/stub implementation of the service with dependency injection.

	t.Run("Malformed response", func(t *testing.T) {
		// Setup
		service, httpClient, _, _ := setupPostmarkTest(t)
		webhookID := 123

		// Test webhook configuration
		webhookConfig := domain.PostmarkWebhookConfig{
			URL:           "https://api.notifuse.com/webhooks/email",
			MessageStream: "outbound",
		}

		// Simulate invalid JSON
		httpClient.EXPECT().
			Do(gomock.Any()).
			Return(createMockResponse(http.StatusOK, "invalid json"), nil)

		// Call the method
		result, err := service.UpdateWebhook(context.Background(), testConfig, webhookID, webhookConfig)

		// Verify results
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "failed to decode response")
	})
}

func TestPostmarkService_TestWebhook(t *testing.T) {
	t.Run("Successfully test webhook with Delivery event", func(t *testing.T) {
		// Setup
		service, httpClient, _, _ := setupPostmarkTest(t)
		webhookID := 123
		eventType := domain.EmailEventDelivered

		// Expect HTTP request
		httpClient.EXPECT().
			Do(gomock.Any()).
			DoAndReturn(func(req *http.Request) (*http.Response, error) {
				// Verify request
				assert.Equal(t, "POST", req.Method)
				assert.Equal(t, "https://api.postmarkapp.com/webhooks/123/trigger", req.URL.String())
				assert.Equal(t, "application/json", req.Header.Get("Content-Type"))
				assert.Equal(t, testConfig.ServerToken, req.Header.Get("X-Postmark-Server-Token"))

				// Verify request body
				body, _ := io.ReadAll(req.Body)
				var triggerConfig map[string]string
				err := json.Unmarshal(body, &triggerConfig)
				assert.NoError(t, err)
				assert.Equal(t, "Delivery", triggerConfig["Trigger"])

				return createMockResponse(http.StatusOK, "{}"), nil
			})

		// Call the method
		err := service.TestWebhook(context.Background(), testConfig, webhookID, eventType)

		// Verify results
		assert.NoError(t, err)
	})

	t.Run("Successfully test webhook with Bounce event", func(t *testing.T) {
		// Setup
		service, httpClient, _, _ := setupPostmarkTest(t)
		webhookID := 123
		eventType := domain.EmailEventBounce

		// Expect HTTP request
		httpClient.EXPECT().
			Do(gomock.Any()).
			DoAndReturn(func(req *http.Request) (*http.Response, error) {
				// Verify request body
				body, _ := io.ReadAll(req.Body)
				var triggerConfig map[string]string
				err := json.Unmarshal(body, &triggerConfig)
				assert.NoError(t, err)
				assert.Equal(t, "Bounce", triggerConfig["Trigger"])

				return createMockResponse(http.StatusOK, "{}"), nil
			})

		// Call the method
		err := service.TestWebhook(context.Background(), testConfig, webhookID, eventType)

		// Verify results
		assert.NoError(t, err)
	})

	t.Run("Successfully test webhook with Complaint event", func(t *testing.T) {
		// Setup
		service, httpClient, _, _ := setupPostmarkTest(t)
		webhookID := 123
		eventType := domain.EmailEventComplaint

		// Expect HTTP request
		httpClient.EXPECT().
			Do(gomock.Any()).
			DoAndReturn(func(req *http.Request) (*http.Response, error) {
				// Verify request body
				body, _ := io.ReadAll(req.Body)
				var triggerConfig map[string]string
				err := json.Unmarshal(body, &triggerConfig)
				assert.NoError(t, err)
				assert.Equal(t, "SpamComplaint", triggerConfig["Trigger"])

				return createMockResponse(http.StatusOK, "{}"), nil
			})

		// Call the method
		err := service.TestWebhook(context.Background(), testConfig, webhookID, eventType)

		// Verify results
		assert.NoError(t, err)
	})

	t.Run("Unsupported event type", func(t *testing.T) {
		// Setup
		service, _, _, _ := setupPostmarkTest(t)
		webhookID := 123
		// Create an unsupported event type
		eventType := domain.EmailEventType("unsupported")

		// Call the method
		err := service.TestWebhook(context.Background(), testConfig, webhookID, eventType)

		// Verify results
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported event type")
	})

	t.Run("HTTP client error", func(t *testing.T) {
		// Setup
		service, httpClient, _, _ := setupPostmarkTest(t)
		webhookID := 123
		eventType := domain.EmailEventDelivered

		// Simulate HTTP error
		httpClient.EXPECT().
			Do(gomock.Any()).
			Return(nil, errors.New("network error"))

		// Call the method
		err := service.TestWebhook(context.Background(), testConfig, webhookID, eventType)

		// Verify results
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to execute request")
	})

	t.Run("API returns error status code", func(t *testing.T) {
		// Setup
		service, httpClient, _, _ := setupPostmarkTest(t)
		webhookID := 123
		eventType := domain.EmailEventDelivered

		// Simulate 404 response
		httpClient.EXPECT().
			Do(gomock.Any()).
			Return(createMockResponse(http.StatusNotFound, ""), nil)

		// Call the method
		err := service.TestWebhook(context.Background(), testConfig, webhookID, eventType)

		// Verify results
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "API returned non-OK status code 404")
	})
}

func TestPostmarkService_RegisterWebhooks(t *testing.T) {
	t.Run("Successfully register webhooks", func(t *testing.T) {
		// Setup
		service, httpClient, _, _ := setupPostmarkTest(t)
		workspaceID := "workspace-123"
		integrationID := "integration-456"
		baseURL := "https://api.notifuse.com"
		eventTypes := []domain.EmailEventType{
			domain.EmailEventDelivered,
			domain.EmailEventBounce,
		}

		// Provider config
		providerConfig := &domain.EmailProvider{
			Kind: domain.EmailProviderKindPostmark,
			Postmark: &domain.PostmarkSettings{
				ServerToken: "test-server-token",
			},
		}

		// Expected callback URL
		expectedURL := "https://api.notifuse.com/webhooks/email?provider=postmark&workspace_id=workspace-123&integration_id=integration-456"

		// Mock list webhooks response
		listResponse := &domain.PostmarkListWebhooksResponse{
			Webhooks: []domain.PostmarkWebhookResponse{},
		}
		listResponseBody, _ := json.Marshal(listResponse)

		// Mock register webhook response
		registerResponse := domain.PostmarkWebhookResponse{
			ID:            123,
			URL:           expectedURL,
			MessageStream: "outbound",
			Triggers: &domain.PostmarkTriggers{
				Delivery: &domain.PostmarkDeliveryTrigger{Enabled: true},
				Bounce:   &domain.PostmarkBounceTrigger{Enabled: true},
			},
		}
		registerResponseBody, _ := json.Marshal(registerResponse)

		// Expect list webhooks request
		listCall := httpClient.EXPECT().
			Do(gomock.Any()).
			DoAndReturn(func(req *http.Request) (*http.Response, error) {
				assert.Equal(t, "GET", req.Method)
				assert.Equal(t, "https://api.postmarkapp.com/webhooks", req.URL.String())
				return createMockResponse(http.StatusOK, string(listResponseBody)), nil
			})

		// Expect register webhook request
		httpClient.EXPECT().
			Do(gomock.Any()).
			DoAndReturn(func(req *http.Request) (*http.Response, error) {
				assert.Equal(t, "POST", req.Method)
				assert.Equal(t, "https://api.postmarkapp.com/webhooks", req.URL.String())

				// Verify request body
				body, _ := io.ReadAll(req.Body)
				var webhookConfig domain.PostmarkWebhookConfig
				err := json.Unmarshal(body, &webhookConfig)
				assert.NoError(t, err)
				assert.Equal(t, expectedURL, webhookConfig.URL)
				assert.Equal(t, "outbound", webhookConfig.MessageStream)
				assert.True(t, webhookConfig.Triggers.Delivery.Enabled)
				assert.True(t, webhookConfig.Triggers.Bounce.Enabled)
				assert.False(t, webhookConfig.Triggers.SpamComplaint.Enabled)

				return createMockResponse(http.StatusCreated, string(registerResponseBody)), nil
			}).After(listCall)

		// Call the method
		status, err := service.RegisterWebhooks(
			context.Background(),
			workspaceID,
			integrationID,
			baseURL,
			eventTypes,
			providerConfig,
		)

		// Verify results
		require.NoError(t, err)
		require.NotNil(t, status)
		assert.Equal(t, domain.EmailProviderKindPostmark, status.EmailProviderKind)
		assert.True(t, status.IsRegistered)
		assert.Len(t, status.Endpoints, 2)

		// Verify the webhook URL is correct
		assert.Equal(t, expectedURL, status.Endpoints[0].URL)
		assert.Equal(t, "123", status.Endpoints[0].WebhookID)
	})

	t.Run("Unregister existing webhooks and register new ones", func(t *testing.T) {
		// Setup
		service, httpClient, _, _ := setupPostmarkTest(t)
		workspaceID := "workspace-123"
		integrationID := "integration-456"
		baseURL := "https://api.notifuse.com"
		eventTypes := []domain.EmailEventType{
			domain.EmailEventDelivered,
			domain.EmailEventComplaint,
		}

		// Provider config
		providerConfig := &domain.EmailProvider{
			Kind: domain.EmailProviderKindPostmark,
			Postmark: &domain.PostmarkSettings{
				ServerToken: "test-server-token",
			},
		}

		// Expected callback URL
		expectedURL := "https://api.notifuse.com/webhooks/email?provider=postmark&workspace_id=workspace-123&integration_id=integration-456"

		// Mock an existing webhook in the list response
		existingWebhook := domain.PostmarkWebhookResponse{
			ID:            100,
			URL:           expectedURL,
			MessageStream: "outbound",
			Triggers: &domain.PostmarkTriggers{
				Delivery: &domain.PostmarkDeliveryTrigger{Enabled: true},
				Bounce:   &domain.PostmarkBounceTrigger{Enabled: true},
			},
		}

		listResponse := &domain.PostmarkListWebhooksResponse{
			Webhooks: []domain.PostmarkWebhookResponse{existingWebhook},
		}
		listResponseBody, _ := json.Marshal(listResponse)

		// Mock register webhook response
		registerResponse := domain.PostmarkWebhookResponse{
			ID:            123,
			URL:           expectedURL,
			MessageStream: "outbound",
			Triggers: &domain.PostmarkTriggers{
				Delivery:      &domain.PostmarkDeliveryTrigger{Enabled: true},
				SpamComplaint: &domain.PostmarkSpamComplaintTrigger{Enabled: true},
			},
		}
		registerResponseBody, _ := json.Marshal(registerResponse)

		// Expect list webhooks request
		listCall := httpClient.EXPECT().
			Do(gomock.Any()).
			DoAndReturn(func(req *http.Request) (*http.Response, error) {
				assert.Equal(t, "GET", req.Method)
				return createMockResponse(http.StatusOK, string(listResponseBody)), nil
			})

		// Expect delete existing webhook request
		deleteCall := httpClient.EXPECT().
			Do(gomock.Any()).
			DoAndReturn(func(req *http.Request) (*http.Response, error) {
				assert.Equal(t, "DELETE", req.Method)
				assert.Equal(t, "https://api.postmarkapp.com/webhooks/100", req.URL.String())
				return createMockResponse(http.StatusOK, "{}"), nil
			}).After(listCall)

		// Expect register new webhook request
		httpClient.EXPECT().
			Do(gomock.Any()).
			DoAndReturn(func(req *http.Request) (*http.Response, error) {
				assert.Equal(t, "POST", req.Method)

				// Verify request body
				body, _ := io.ReadAll(req.Body)
				var webhookConfig domain.PostmarkWebhookConfig
				err := json.Unmarshal(body, &webhookConfig)
				assert.NoError(t, err)
				assert.Equal(t, expectedURL, webhookConfig.URL)
				assert.True(t, webhookConfig.Triggers.Delivery.Enabled)
				assert.False(t, webhookConfig.Triggers.Bounce.Enabled)
				assert.True(t, webhookConfig.Triggers.SpamComplaint.Enabled)

				return createMockResponse(http.StatusCreated, string(registerResponseBody)), nil
			}).After(deleteCall)

		// Call the method
		status, err := service.RegisterWebhooks(
			context.Background(),
			workspaceID,
			integrationID,
			baseURL,
			eventTypes,
			providerConfig,
		)

		// Verify results
		require.NoError(t, err)
		require.NotNil(t, status)
		assert.Equal(t, domain.EmailProviderKindPostmark, status.EmailProviderKind)
		assert.True(t, status.IsRegistered)
		assert.Len(t, status.Endpoints, 2)
	})

	t.Run("Missing provider configuration", func(t *testing.T) {
		// Setup
		service, _, _, _ := setupPostmarkTest(t)
		workspaceID := "workspace-123"
		integrationID := "integration-456"
		baseURL := "https://api.notifuse.com"
		eventTypes := []domain.EmailEventType{domain.EmailEventDelivered}

		// Call with nil provider config
		status, err := service.RegisterWebhooks(
			context.Background(),
			workspaceID,
			integrationID,
			baseURL,
			eventTypes,
			nil,
		)

		// Verify results
		assert.Error(t, err)
		assert.Nil(t, status)
		assert.Contains(t, err.Error(), "configuration is missing or invalid")

		// Call with missing Postmark config
		status, err = service.RegisterWebhooks(
			context.Background(),
			workspaceID,
			integrationID,
			baseURL,
			eventTypes,
			&domain.EmailProvider{Kind: domain.EmailProviderKindPostmark},
		)

		// Verify results
		assert.Error(t, err)
		assert.Nil(t, status)
		assert.Contains(t, err.Error(), "configuration is missing or invalid")
	})

	t.Run("Failed to list webhooks", func(t *testing.T) {
		// Setup
		service, httpClient, _, _ := setupPostmarkTest(t)
		workspaceID := "workspace-123"
		integrationID := "integration-456"
		baseURL := "https://api.notifuse.com"
		eventTypes := []domain.EmailEventType{domain.EmailEventDelivered}

		// Provider config
		providerConfig := &domain.EmailProvider{
			Kind: domain.EmailProviderKindPostmark,
			Postmark: &domain.PostmarkSettings{
				ServerToken: "test-server-token",
			},
		}

		// Simulate error on list webhooks
		httpClient.EXPECT().
			Do(gomock.Any()).
			Return(nil, errors.New("network error"))

		// Call the method
		status, err := service.RegisterWebhooks(
			context.Background(),
			workspaceID,
			integrationID,
			baseURL,
			eventTypes,
			providerConfig,
		)

		// Verify results
		assert.Error(t, err)
		assert.Nil(t, status)
		assert.Contains(t, err.Error(), "failed to list Postmark webhooks")
	})

	t.Run("Failed to register webhook", func(t *testing.T) {
		// Setup
		service, httpClient, _, _ := setupPostmarkTest(t)
		workspaceID := "workspace-123"
		integrationID := "integration-456"
		baseURL := "https://api.notifuse.com"
		eventTypes := []domain.EmailEventType{domain.EmailEventDelivered}

		// Provider config
		providerConfig := &domain.EmailProvider{
			Kind: domain.EmailProviderKindPostmark,
			Postmark: &domain.PostmarkSettings{
				ServerToken: "test-server-token",
			},
		}

		// Mock empty list response
		listResponse := &domain.PostmarkListWebhooksResponse{
			Webhooks: []domain.PostmarkWebhookResponse{},
		}
		listResponseBody, _ := json.Marshal(listResponse)

		// Expect list webhooks request to succeed
		listCall := httpClient.EXPECT().
			Do(gomock.Any()).
			DoAndReturn(func(req *http.Request) (*http.Response, error) {
				assert.Equal(t, "GET", req.Method)
				return createMockResponse(http.StatusOK, string(listResponseBody)), nil
			})

		// But register webhook request to fail
		httpClient.EXPECT().
			Do(gomock.Any()).
			Return(createMockResponse(http.StatusBadRequest, ""), nil).
			After(listCall)

		// Call the method
		status, err := service.RegisterWebhooks(
			context.Background(),
			workspaceID,
			integrationID,
			baseURL,
			eventTypes,
			providerConfig,
		)

		// Verify results
		assert.Error(t, err)
		assert.Nil(t, status)
		assert.Contains(t, err.Error(), "failed to register Postmark webhook")
	})
}

func TestPostmarkService_GetWebhookStatus(t *testing.T) {
	t.Run("Successfully get webhook status with registered webhooks", func(t *testing.T) {
		// Setup
		service, httpClient, _, _ := setupPostmarkTest(t)
		workspaceID := "workspace-123"
		integrationID := "integration-456"

		// Provider config
		providerConfig := &domain.EmailProvider{
			Kind: domain.EmailProviderKindPostmark,
			Postmark: &domain.PostmarkSettings{
				ServerToken: "test-server-token",
			},
		}

		// Mock a webhook in the list response
		webhook := domain.PostmarkWebhookResponse{
			ID:            123,
			URL:           "https://api.notifuse.com/webhooks/email?provider=postmark&workspace_id=workspace-123&integration_id=integration-456",
			MessageStream: "outbound",
			Triggers: &domain.PostmarkTriggers{
				Delivery:      &domain.PostmarkDeliveryTrigger{Enabled: true},
				Bounce:        &domain.PostmarkBounceTrigger{Enabled: true},
				SpamComplaint: &domain.PostmarkSpamComplaintTrigger{Enabled: true},
			},
		}

		listResponse := &domain.PostmarkListWebhooksResponse{
			Webhooks: []domain.PostmarkWebhookResponse{webhook},
		}
		listResponseBody, _ := json.Marshal(listResponse)

		// Expect list webhooks request
		httpClient.EXPECT().
			Do(gomock.Any()).
			DoAndReturn(func(req *http.Request) (*http.Response, error) {
				assert.Equal(t, "GET", req.Method)
				assert.Equal(t, "https://api.postmarkapp.com/webhooks", req.URL.String())
				return createMockResponse(http.StatusOK, string(listResponseBody)), nil
			})

		// Call the method
		status, err := service.GetWebhookStatus(
			context.Background(),
			workspaceID,
			integrationID,
			providerConfig,
		)

		// Verify results
		require.NoError(t, err)
		require.NotNil(t, status)
		assert.Equal(t, domain.EmailProviderKindPostmark, status.EmailProviderKind)
		assert.True(t, status.IsRegistered)
		assert.Len(t, status.Endpoints, 3) // One for each trigger type that's enabled

		// Check if all event types are properly represented
		foundEventTypes := map[domain.EmailEventType]bool{}
		for _, endpoint := range status.Endpoints {
			foundEventTypes[endpoint.EventType] = true
			assert.Equal(t, "123", endpoint.WebhookID)
			assert.True(t, endpoint.Active)
		}
		assert.True(t, foundEventTypes[domain.EmailEventDelivered])
		assert.True(t, foundEventTypes[domain.EmailEventBounce])
		assert.True(t, foundEventTypes[domain.EmailEventComplaint])
	})

	t.Run("No webhooks registered", func(t *testing.T) {
		// Setup
		service, httpClient, _, _ := setupPostmarkTest(t)
		workspaceID := "workspace-123"
		integrationID := "integration-456"

		// Provider config
		providerConfig := &domain.EmailProvider{
			Kind: domain.EmailProviderKindPostmark,
			Postmark: &domain.PostmarkSettings{
				ServerToken: "test-server-token",
			},
		}

		// Mock empty list response
		listResponse := &domain.PostmarkListWebhooksResponse{
			Webhooks: []domain.PostmarkWebhookResponse{},
		}
		listResponseBody, _ := json.Marshal(listResponse)

		// Expect list webhooks request
		httpClient.EXPECT().
			Do(gomock.Any()).
			DoAndReturn(func(req *http.Request) (*http.Response, error) {
				return createMockResponse(http.StatusOK, string(listResponseBody)), nil
			})

		// Call the method
		status, err := service.GetWebhookStatus(
			context.Background(),
			workspaceID,
			integrationID,
			providerConfig,
		)

		// Verify results
		require.NoError(t, err)
		require.NotNil(t, status)
		assert.Equal(t, domain.EmailProviderKindPostmark, status.EmailProviderKind)
		assert.False(t, status.IsRegistered)
		assert.Empty(t, status.Endpoints)
	})

	t.Run("Missing provider configuration", func(t *testing.T) {
		// Setup
		service, _, _, _ := setupPostmarkTest(t)
		workspaceID := "workspace-123"
		integrationID := "integration-456"

		// Call with nil provider config
		status, err := service.GetWebhookStatus(
			context.Background(),
			workspaceID,
			integrationID,
			nil,
		)

		// Verify results
		assert.Error(t, err)
		assert.Nil(t, status)
		assert.Contains(t, err.Error(), "configuration is missing or invalid")
	})

	t.Run("Failed to list webhooks", func(t *testing.T) {
		// Setup
		service, httpClient, _, _ := setupPostmarkTest(t)
		workspaceID := "workspace-123"
		integrationID := "integration-456"

		// Provider config
		providerConfig := &domain.EmailProvider{
			Kind: domain.EmailProviderKindPostmark,
			Postmark: &domain.PostmarkSettings{
				ServerToken: "test-server-token",
			},
		}

		// Simulate error on list webhooks
		httpClient.EXPECT().
			Do(gomock.Any()).
			Return(nil, errors.New("network error"))

		// Call the method
		status, err := service.GetWebhookStatus(
			context.Background(),
			workspaceID,
			integrationID,
			providerConfig,
		)

		// Verify results
		assert.Error(t, err)
		assert.Nil(t, status)
		assert.Contains(t, err.Error(), "failed to list Postmark webhooks")
	})
}

func TestPostmarkService_UnregisterWebhooks(t *testing.T) {
	t.Run("Successfully unregister webhooks", func(t *testing.T) {
		// Setup
		service, httpClient, _, _ := setupPostmarkTest(t)
		workspaceID := "workspace-123"
		integrationID := "integration-456"

		// Provider config
		providerConfig := &domain.EmailProvider{
			Kind: domain.EmailProviderKindPostmark,
			Postmark: &domain.PostmarkSettings{
				ServerToken: "test-server-token",
			},
		}

		// Mock webhooks in the list response
		webhook1 := domain.PostmarkWebhookResponse{
			ID:  123,
			URL: "https://api.notifuse.com/webhooks/email?provider=postmark&workspace_id=workspace-123&integration_id=integration-456",
		}
		webhook2 := domain.PostmarkWebhookResponse{
			ID:  456,
			URL: "https://api.notifuse.com/webhooks/email?provider=postmark&workspace_id=workspace-123&integration_id=integration-456&different=true",
		}

		listResponse := &domain.PostmarkListWebhooksResponse{
			Webhooks: []domain.PostmarkWebhookResponse{webhook1, webhook2},
		}
		listResponseBody, _ := json.Marshal(listResponse)

		// Expect list webhooks request
		listCall := httpClient.EXPECT().
			Do(gomock.Any()).
			DoAndReturn(func(req *http.Request) (*http.Response, error) {
				assert.Equal(t, "GET", req.Method)
				return createMockResponse(http.StatusOK, string(listResponseBody)), nil
			})

		// Expect unregister requests for both webhooks
		delete1Call := httpClient.EXPECT().
			Do(gomock.Any()).
			DoAndReturn(func(req *http.Request) (*http.Response, error) {
				assert.Equal(t, "DELETE", req.Method)
				assert.Equal(t, "https://api.postmarkapp.com/webhooks/123", req.URL.String())
				return createMockResponse(http.StatusOK, "{}"), nil
			}).After(listCall)

		httpClient.EXPECT().
			Do(gomock.Any()).
			DoAndReturn(func(req *http.Request) (*http.Response, error) {
				assert.Equal(t, "DELETE", req.Method)
				assert.Equal(t, "https://api.postmarkapp.com/webhooks/456", req.URL.String())
				return createMockResponse(http.StatusOK, "{}"), nil
			}).After(delete1Call)

		// Call the method
		err := service.UnregisterWebhooks(
			context.Background(),
			workspaceID,
			integrationID,
			providerConfig,
		)

		// Verify results
		assert.NoError(t, err)
	})

	t.Run("No webhooks to unregister", func(t *testing.T) {
		// Setup
		service, httpClient, _, _ := setupPostmarkTest(t)
		workspaceID := "workspace-123"
		integrationID := "integration-456"

		// Provider config
		providerConfig := &domain.EmailProvider{
			Kind: domain.EmailProviderKindPostmark,
			Postmark: &domain.PostmarkSettings{
				ServerToken: "test-server-token",
			},
		}

		// Mock empty list response
		listResponse := &domain.PostmarkListWebhooksResponse{
			Webhooks: []domain.PostmarkWebhookResponse{},
		}
		listResponseBody, _ := json.Marshal(listResponse)

		// Expect list webhooks request
		httpClient.EXPECT().
			Do(gomock.Any()).
			DoAndReturn(func(req *http.Request) (*http.Response, error) {
				return createMockResponse(http.StatusOK, string(listResponseBody)), nil
			})

		// Call the method - no delete calls expected
		err := service.UnregisterWebhooks(
			context.Background(),
			workspaceID,
			integrationID,
			providerConfig,
		)

		// Verify results
		assert.NoError(t, err)
	})

	t.Run("Missing provider configuration", func(t *testing.T) {
		// Setup
		service, _, _, _ := setupPostmarkTest(t)
		workspaceID := "workspace-123"
		integrationID := "integration-456"

		// Call with nil provider config
		err := service.UnregisterWebhooks(
			context.Background(),
			workspaceID,
			integrationID,
			nil,
		)

		// Verify results
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "configuration is missing or invalid")
	})

	t.Run("Failed to list webhooks", func(t *testing.T) {
		// Setup
		service, httpClient, _, _ := setupPostmarkTest(t)
		workspaceID := "workspace-123"
		integrationID := "integration-456"

		// Provider config
		providerConfig := &domain.EmailProvider{
			Kind: domain.EmailProviderKindPostmark,
			Postmark: &domain.PostmarkSettings{
				ServerToken: "test-server-token",
			},
		}

		// Simulate error on list webhooks
		httpClient.EXPECT().
			Do(gomock.Any()).
			Return(nil, errors.New("network error"))

		// Call the method
		err := service.UnregisterWebhooks(
			context.Background(),
			workspaceID,
			integrationID,
			providerConfig,
		)

		// Verify results
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to list Postmark webhooks")
	})

	t.Run("Partial failure during unregistration", func(t *testing.T) {
		// Setup
		service, httpClient, _, _ := setupPostmarkTest(t)
		workspaceID := "workspace-123"
		integrationID := "integration-456"

		// Provider config
		providerConfig := &domain.EmailProvider{
			Kind: domain.EmailProviderKindPostmark,
			Postmark: &domain.PostmarkSettings{
				ServerToken: "test-server-token",
			},
		}

		// Mock webhooks in the list response
		webhook1 := domain.PostmarkWebhookResponse{
			ID:  123,
			URL: "https://api.notifuse.com/webhooks/email?provider=postmark&workspace_id=workspace-123&integration_id=integration-456",
		}
		webhook2 := domain.PostmarkWebhookResponse{
			ID:  456,
			URL: "https://api.notifuse.com/webhooks/email?provider=postmark&workspace_id=workspace-123&integration_id=integration-456",
		}

		listResponse := &domain.PostmarkListWebhooksResponse{
			Webhooks: []domain.PostmarkWebhookResponse{webhook1, webhook2},
		}
		listResponseBody, _ := json.Marshal(listResponse)

		// Expect list webhooks request
		listCall := httpClient.EXPECT().
			Do(gomock.Any()).
			DoAndReturn(func(req *http.Request) (*http.Response, error) {
				assert.Equal(t, "GET", req.Method)
				return createMockResponse(http.StatusOK, string(listResponseBody)), nil
			})

		// First webhook unregisters successfully
		delete1Call := httpClient.EXPECT().
			Do(gomock.Any()).
			DoAndReturn(func(req *http.Request) (*http.Response, error) {
				assert.Equal(t, "DELETE", req.Method)
				assert.Equal(t, "https://api.postmarkapp.com/webhooks/123", req.URL.String())
				return createMockResponse(http.StatusOK, "{}"), nil
			}).After(listCall)

		// Second webhook fails to unregister
		httpClient.EXPECT().
			Do(gomock.Any()).
			DoAndReturn(func(req *http.Request) (*http.Response, error) {
				assert.Equal(t, "DELETE", req.Method)
				assert.Equal(t, "https://api.postmarkapp.com/webhooks/456", req.URL.String())
				return createMockResponse(http.StatusInternalServerError, "{}"), nil
			}).After(delete1Call)

		// Call the method
		err := service.UnregisterWebhooks(
			context.Background(),
			workspaceID,
			integrationID,
			providerConfig,
		)

		// Verify results
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to unregister one or more Postmark webhooks")
	})
}

func TestPostmarkService_FilterPostmarkWebhooks(t *testing.T) {
	t.Run("Filter by URL, workspace, and integration", func(t *testing.T) {
		// Setup
		service, _, _, _ := setupPostmarkTest(t)
		baseURL := "https://api.notifuse.com"
		workspaceID := "workspace-123"
		integrationID := "integration-456"

		// Create a mix of webhooks
		webhooks := []domain.PostmarkWebhookResponse{
			{
				ID:  1,
				URL: "https://api.notifuse.com/webhooks/email?provider=postmark&workspace_id=workspace-123&integration_id=integration-456",
			},
			{
				ID:  2,
				URL: "https://api.notifuse.com/webhooks/email?provider=postmark&workspace_id=workspace-123&integration_id=other-integration",
			},
			{
				ID:  3,
				URL: "https://api.notifuse.com/webhooks/email?provider=postmark&workspace_id=other-workspace&integration_id=integration-456",
			},
			{
				ID:  4,
				URL: "https://other-domain.com/webhooks/email?provider=postmark&workspace_id=workspace-123&integration_id=integration-456",
			},
		}

		// Filter webhooks
		filtered := service.filterPostmarkWebhooks(webhooks, baseURL, workspaceID, integrationID)

		// Verify results
		assert.Len(t, filtered, 1)
		assert.Equal(t, 1, filtered[0].ID)
	})

	t.Run("Filter without baseURL", func(t *testing.T) {
		// Setup
		service, _, _, _ := setupPostmarkTest(t)
		workspaceID := "workspace-123"
		integrationID := "integration-456"

		// Create a mix of webhooks
		webhooks := []domain.PostmarkWebhookResponse{
			{
				ID:  1,
				URL: "https://api.notifuse.com/webhooks/email?provider=postmark&workspace_id=workspace-123&integration_id=integration-456",
			},
			{
				ID:  2,
				URL: "https://other-domain.com/webhooks/email?provider=postmark&workspace_id=workspace-123&integration_id=integration-456",
			},
		}

		// Filter webhooks without specifying baseURL
		filtered := service.filterPostmarkWebhooks(webhooks, "", workspaceID, integrationID)

		// Verify results - both should match since we didn't filter by baseURL
		assert.Len(t, filtered, 2)
	})

	t.Run("Empty webhooks list", func(t *testing.T) {
		// Setup
		service, _, _, _ := setupPostmarkTest(t)

		// Filter an empty webhooks list
		filtered := service.filterPostmarkWebhooks([]domain.PostmarkWebhookResponse{}, "", "any", "any")

		// Verify results
		assert.Empty(t, filtered)
	})
}

func TestPostmarkService_SendEmail(t *testing.T) {
	t.Run("Successfully send email", func(t *testing.T) {
		// Setup
		service, httpClient, _, _ := setupPostmarkTest(t)
		workspaceID := "workspace-123"
		fromAddress := "sender@example.com"
		fromName := "Sender Name"
		to := "recipient@example.com"
		subject := "Test Email"
		content := "<p>This is a test email</p>"

		// Provider config
		providerConfig := &domain.EmailProvider{
			Kind: domain.EmailProviderKindPostmark,
			Postmark: &domain.PostmarkSettings{
				ServerToken: "test-server-token",
			},
		}

		// Expect HTTP request
		httpClient.EXPECT().
			Do(gomock.Any()).
			DoAndReturn(func(req *http.Request) (*http.Response, error) {
				// Verify request details
				assert.Equal(t, "POST", req.Method)
				assert.Equal(t, "https://api.postmarkapp.com/email", req.URL.String())
				assert.Equal(t, "application/json", req.Header.Get("Content-Type"))
				assert.Equal(t, "application/json", req.Header.Get("Accept"))
				assert.Equal(t, "test-server-token", req.Header.Get("X-Postmark-Server-Token"))

				// Verify request body
				body, _ := io.ReadAll(req.Body)
				var requestBody map[string]interface{}
				err := json.Unmarshal(body, &requestBody)
				require.NoError(t, err)
				assert.Equal(t, "Sender Name <sender@example.com>", requestBody["From"])
				assert.Equal(t, "recipient@example.com", requestBody["To"])
				assert.Equal(t, "Test Email", requestBody["Subject"])
				assert.Equal(t, "<p>This is a test email</p>", requestBody["HtmlBody"])

				return createMockResponse(http.StatusOK, `{"MessageID":"12345"}`), nil
			})

		// Call the method
		request := domain.SendEmailProviderRequest{
			WorkspaceID:   workspaceID,
			IntegrationID: "test-integration-id",
			MessageID:     "test-message-id",
			FromAddress:   fromAddress,
			FromName:      fromName,
			To:            to,
			Subject:       subject,
			Content:       content,
			Provider:      providerConfig,
			EmailOptions:  domain.EmailOptions{},
		}
		err := service.SendEmail(context.Background(), request)

		// Verify results
		assert.NoError(t, err)
	})

	t.Run("Missing Postmark configuration", func(t *testing.T) {
		// Setup
		service, _, _, _ := setupPostmarkTest(t)
		workspaceID := "workspace-123"

		// Call with nil Postmark config
		request := domain.SendEmailProviderRequest{
			WorkspaceID:   workspaceID,
			IntegrationID: "test-integration-id",
			MessageID:     "test-message-id",
			FromAddress:   "sender@example.com",
			FromName:      "Sender",
			To:            "recipient@example.com",
			Subject:       "Subject",
			Content:       "Content",
			Provider: &domain.EmailProvider{
				Kind: domain.EmailProviderKindPostmark,
			},
			EmailOptions: domain.EmailOptions{},
		}
		err := service.SendEmail(context.Background(), request)

		// Verify results
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "postmark provider is not configured")
	})

	t.Run("Empty server token", func(t *testing.T) {
		// Setup
		service, _, _, _ := setupPostmarkTest(t)
		workspaceID := "workspace-123"

		// Provider config with empty token
		providerConfig := &domain.EmailProvider{
			Kind: domain.EmailProviderKindPostmark,
			Postmark: &domain.PostmarkSettings{
				ServerToken: "",
			},
		}

		// Call with empty server token
		request := domain.SendEmailProviderRequest{
			WorkspaceID:   workspaceID,
			IntegrationID: "test-integration-id",
			MessageID:     "test-message-id",
			FromAddress:   "sender@example.com",
			FromName:      "Sender",
			To:            "recipient@example.com",
			Subject:       "Subject",
			Content:       "Content",
			Provider:      providerConfig,
			EmailOptions:  domain.EmailOptions{},
		}
		err := service.SendEmail(context.Background(), request)

		// Verify results
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "postmark server token is required")
	})

	t.Run("HTTP client error", func(t *testing.T) {
		// Setup
		service, httpClient, _, _ := setupPostmarkTest(t)
		workspaceID := "workspace-123"

		// Provider config
		providerConfig := &domain.EmailProvider{
			Kind: domain.EmailProviderKindPostmark,
			Postmark: &domain.PostmarkSettings{
				ServerToken: "test-server-token",
			},
		}

		// Simulate HTTP error
		httpClient.EXPECT().
			Do(gomock.Any()).
			Return(nil, errors.New("network error"))

		// Call the method
		request := domain.SendEmailProviderRequest{
			WorkspaceID:   workspaceID,
			IntegrationID: "test-integration-id",
			MessageID:     "test-message-id",
			FromAddress:   "sender@example.com",
			FromName:      "Sender",
			To:            "recipient@example.com",
			Subject:       "Subject",
			Content:       "Content",
			Provider:      providerConfig,
			EmailOptions:  domain.EmailOptions{},
		}
		err := service.SendEmail(context.Background(), request)

		// Verify results
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to send request to Postmark API")
	})

	t.Run("API returns error status code", func(t *testing.T) {
		// Setup
		service, httpClient, _, _ := setupPostmarkTest(t)
		workspaceID := "workspace-123"

		// Provider config
		providerConfig := &domain.EmailProvider{
			Kind: domain.EmailProviderKindPostmark,
			Postmark: &domain.PostmarkSettings{
				ServerToken: "test-server-token",
			},
		}

		// Simulate 400 response
		httpClient.EXPECT().
			Do(gomock.Any()).
			Return(createMockResponse(http.StatusBadRequest, `{"ErrorCode":400,"Message":"Invalid email"}`), nil)

		// Call the method
		request := domain.SendEmailProviderRequest{
			WorkspaceID:   workspaceID,
			IntegrationID: "test-integration-id",
			MessageID:     "test-message-id",
			FromAddress:   "sender@example.com",
			FromName:      "Sender",
			To:            "recipient@example.com",
			Subject:       "Subject",
			Content:       "Content",
			Provider:      providerConfig,
			EmailOptions:  domain.EmailOptions{},
		}
		err := service.SendEmail(context.Background(), request)

		// Verify results
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "postmark API error")
	})

	t.Run("Error reading response body", func(t *testing.T) {
		// Setup
		service, httpClient, _, _ := setupPostmarkTest(t)
		workspaceID := "workspace-123"

		// Provider config
		providerConfig := &domain.EmailProvider{
			Kind: domain.EmailProviderKindPostmark,
			Postmark: &domain.PostmarkSettings{
				ServerToken: "test-server-token",
			},
		}

		// Create a mock response with a body that will error when read
		erroringBody := io.NopCloser(&errorReader{})
		resp := &http.Response{
			StatusCode: http.StatusBadRequest,
			Body:       erroringBody,
		}

		httpClient.EXPECT().
			Do(gomock.Any()).
			Return(resp, nil)

		// Call the method
		request := domain.SendEmailProviderRequest{
			WorkspaceID:   workspaceID,
			IntegrationID: "test-integration-id",
			MessageID:     "test-message-id",
			FromAddress:   "sender@example.com",
			FromName:      "Sender",
			To:            "recipient@example.com",
			Subject:       "Subject",
			Content:       "Content",
			Provider:      providerConfig,
			EmailOptions:  domain.EmailOptions{},
		}
		err := service.SendEmail(context.Background(), request)

		// Verify results
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to read Postmark API response")
	})

	t.Run("Email with single attachment", func(t *testing.T) {
		// Setup
		service, httpClient, _, logger := setupPostmarkTest(t)
		workspaceID := "workspace-123"
		fromAddress := "sender@example.com"
		fromName := "Sender Name"
		to := "recipient@example.com"
		subject := "Test Email"
		content := "<p>This is a test email</p>"

		// Create a small PDF attachment (base64 encoded)
		base64Content := "c2FtcGxlIHBkZiBjb250ZW50" // base64 of "sample pdf content"

		// Provider config
		providerConfig := &domain.EmailProvider{
			Kind: domain.EmailProviderKindPostmark,
			Postmark: &domain.PostmarkSettings{
				ServerToken: "test-server-token",
			},
		}

		// Allow logger calls for attachment debugging
		logger.EXPECT().Debug(gomock.Any()).AnyTimes()

		// Expect HTTP request
		httpClient.EXPECT().
			Do(gomock.Any()).
			DoAndReturn(func(req *http.Request) (*http.Response, error) {
				// Verify request details
				assert.Equal(t, "POST", req.Method)
				assert.Equal(t, "https://api.postmarkapp.com/email", req.URL.String())
				assert.Equal(t, "application/json", req.Header.Get("Content-Type"))

				// Verify request body
				body, _ := io.ReadAll(req.Body)
				var requestBody map[string]interface{}
				err := json.Unmarshal(body, &requestBody)
				require.NoError(t, err)

				// Check for attachments
				attachments, ok := requestBody["Attachments"].([]interface{})
				assert.True(t, ok, "Attachments should be present")
				assert.Len(t, attachments, 1)

				attachment := attachments[0].(map[string]interface{})
				assert.Equal(t, "invoice.pdf", attachment["Name"])
				assert.Equal(t, base64Content, attachment["Content"])
				assert.Equal(t, "application/pdf", attachment["ContentType"])

				return createMockResponse(http.StatusOK, `{"MessageID":"12345"}`), nil
			})

		// Call the method
		request := domain.SendEmailProviderRequest{
			WorkspaceID:   workspaceID,
			IntegrationID: "test-integration-id",
			MessageID:     "test-message-id",
			FromAddress:   fromAddress,
			FromName:      fromName,
			To:            to,
			Subject:       subject,
			Content:       content,
			Provider:      providerConfig,
			EmailOptions: domain.EmailOptions{
				Attachments: []domain.Attachment{
					{
						Filename:    "invoice.pdf",
						Content:     base64Content,
						ContentType: "application/pdf",
						Disposition: "attachment",
					},
				},
			},
		}
		err := service.SendEmail(context.Background(), request)

		// Verify results
		assert.NoError(t, err)
	})

	t.Run("Email with multiple attachments", func(t *testing.T) {
		// Setup
		service, httpClient, _, logger := setupPostmarkTest(t)
		workspaceID := "workspace-123"

		// Create attachments (base64 encoded)
		pdfContent := "c2FtcGxlIHBkZiBjb250ZW50"       // base64 of "sample pdf content"
		imageContent := "c2FtcGxlIGltYWdlIGNvbnRlbnQ=" // base64 of "sample image content"

		// Provider config
		providerConfig := &domain.EmailProvider{
			Kind: domain.EmailProviderKindPostmark,
			Postmark: &domain.PostmarkSettings{
				ServerToken: "test-server-token",
			},
		}

		// Allow logger calls for attachment debugging
		logger.EXPECT().Debug(gomock.Any()).AnyTimes()

		// Expect HTTP request
		httpClient.EXPECT().
			Do(gomock.Any()).
			DoAndReturn(func(req *http.Request) (*http.Response, error) {
				// Verify request body
				body, _ := io.ReadAll(req.Body)
				var requestBody map[string]interface{}
				err := json.Unmarshal(body, &requestBody)
				require.NoError(t, err)

				// Check for attachments
				attachments, ok := requestBody["Attachments"].([]interface{})
				assert.True(t, ok, "Attachments should be present")
				assert.Len(t, attachments, 2)

				// Verify first attachment
				attachment1 := attachments[0].(map[string]interface{})
				assert.Equal(t, "invoice.pdf", attachment1["Name"])
				assert.Equal(t, pdfContent, attachment1["Content"])
				assert.Equal(t, "application/pdf", attachment1["ContentType"])

				// Verify second attachment
				attachment2 := attachments[1].(map[string]interface{})
				assert.Equal(t, "logo.png", attachment2["Name"])
				assert.Equal(t, imageContent, attachment2["Content"])
				assert.Equal(t, "image/png", attachment2["ContentType"])

				return createMockResponse(http.StatusOK, `{"MessageID":"12345"}`), nil
			})

		// Call the method
		request := domain.SendEmailProviderRequest{
			WorkspaceID:   workspaceID,
			IntegrationID: "test-integration-id",
			MessageID:     "test-message-id",
			FromAddress:   "sender@example.com",
			FromName:      "Sender",
			To:            "recipient@example.com",
			Subject:       "Subject",
			Content:       "Content",
			Provider:      providerConfig,
			EmailOptions: domain.EmailOptions{
				Attachments: []domain.Attachment{
					{
						Filename:    "invoice.pdf",
						Content:     pdfContent,
						ContentType: "application/pdf",
						Disposition: "attachment",
					},
					{
						Filename:    "logo.png",
						Content:     imageContent,
						ContentType: "image/png",
						Disposition: "attachment",
					},
				},
			},
		}
		err := service.SendEmail(context.Background(), request)

		// Verify results
		assert.NoError(t, err)
	})

	t.Run("Email with inline attachment", func(t *testing.T) {
		// Setup
		service, httpClient, _, logger := setupPostmarkTest(t)
		workspaceID := "workspace-123"

		// Create an inline image attachment
		imageContent := "c2FtcGxlIGltYWdlIGNvbnRlbnQ=" // base64 of "sample image content"

		// Provider config
		providerConfig := &domain.EmailProvider{
			Kind: domain.EmailProviderKindPostmark,
			Postmark: &domain.PostmarkSettings{
				ServerToken: "test-server-token",
			},
		}

		// Allow logger calls for attachment debugging
		logger.EXPECT().Debug(gomock.Any()).AnyTimes()

		// Expect HTTP request
		httpClient.EXPECT().
			Do(gomock.Any()).
			DoAndReturn(func(req *http.Request) (*http.Response, error) {
				// Verify request body
				body, _ := io.ReadAll(req.Body)
				var requestBody map[string]interface{}
				err := json.Unmarshal(body, &requestBody)
				require.NoError(t, err)

				// Check for attachments
				attachments, ok := requestBody["Attachments"].([]interface{})
				assert.True(t, ok, "Attachments should be present")
				assert.Len(t, attachments, 1)

				// Verify inline attachment has ContentID
				attachment := attachments[0].(map[string]interface{})
				assert.Equal(t, "logo.png", attachment["Name"])
				assert.Equal(t, imageContent, attachment["Content"])
				assert.Equal(t, "image/png", attachment["ContentType"])
				assert.Equal(t, "cid:logo.png", attachment["ContentID"], "Inline attachment should have ContentID")

				return createMockResponse(http.StatusOK, `{"MessageID":"12345"}`), nil
			})

		// Call the method
		request := domain.SendEmailProviderRequest{
			WorkspaceID:   workspaceID,
			IntegrationID: "test-integration-id",
			MessageID:     "test-message-id",
			FromAddress:   "sender@example.com",
			FromName:      "Sender",
			To:            "recipient@example.com",
			Subject:       "Subject",
			Content:       "Content",
			Provider:      providerConfig,
			EmailOptions: domain.EmailOptions{
				Attachments: []domain.Attachment{
					{
						Filename:    "logo.png",
						Content:     imageContent,
						ContentType: "image/png",
						Disposition: "inline",
					},
				},
			},
		}
		err := service.SendEmail(context.Background(), request)

		// Verify results
		assert.NoError(t, err)
	})

	t.Run("Email with attachments and CC/BCC/ReplyTo", func(t *testing.T) {
		// Setup
		service, httpClient, _, logger := setupPostmarkTest(t)
		workspaceID := "workspace-123"

		// Create attachment
		pdfContent := "c2FtcGxlIHBkZiBjb250ZW50" // base64 of "sample pdf content"

		// Provider config
		providerConfig := &domain.EmailProvider{
			Kind: domain.EmailProviderKindPostmark,
			Postmark: &domain.PostmarkSettings{
				ServerToken: "test-server-token",
			},
		}

		// Allow logger calls for attachment debugging
		logger.EXPECT().Debug(gomock.Any()).AnyTimes()

		// Expect HTTP request
		httpClient.EXPECT().
			Do(gomock.Any()).
			DoAndReturn(func(req *http.Request) (*http.Response, error) {
				// Verify request body
				body, _ := io.ReadAll(req.Body)
				var requestBody map[string]interface{}
				err := json.Unmarshal(body, &requestBody)
				require.NoError(t, err)

				// Check for attachments
				attachments, ok := requestBody["Attachments"].([]interface{})
				assert.True(t, ok, "Attachments should be present")
				assert.Len(t, attachments, 1)

				// Check for CC, BCC, and ReplyTo
				assert.Equal(t, "cc1@example.com", requestBody["Cc"])
				assert.Equal(t, "bcc1@example.com", requestBody["Bcc"])
				assert.Equal(t, "reply@example.com", requestBody["ReplyTo"])

				return createMockResponse(http.StatusOK, `{"MessageID":"12345"}`), nil
			})

		// Call the method
		request := domain.SendEmailProviderRequest{
			WorkspaceID:   workspaceID,
			IntegrationID: "test-integration-id",
			MessageID:     "test-message-id",
			FromAddress:   "sender@example.com",
			FromName:      "Sender",
			To:            "recipient@example.com",
			Subject:       "Subject",
			Content:       "Content",
			Provider:      providerConfig,
			EmailOptions: domain.EmailOptions{
				CC:      []string{"cc1@example.com"},
				BCC:     []string{"bcc1@example.com"},
				ReplyTo: "reply@example.com",
				Attachments: []domain.Attachment{
					{
						Filename:    "invoice.pdf",
						Content:     pdfContent,
						ContentType: "application/pdf",
						Disposition: "attachment",
					},
				},
			},
		}
		err := service.SendEmail(context.Background(), request)

		// Verify results
		assert.NoError(t, err)
	})

	t.Run("Email with attachment decode error", func(t *testing.T) {
		// Setup
		service, _, _, _ := setupPostmarkTest(t)
		workspaceID := "workspace-123"

		// Provider config
		providerConfig := &domain.EmailProvider{
			Kind: domain.EmailProviderKindPostmark,
			Postmark: &domain.PostmarkSettings{
				ServerToken: "test-server-token",
			},
		}

		// Call the method with invalid base64 content
		request := domain.SendEmailProviderRequest{
			WorkspaceID:   workspaceID,
			IntegrationID: "test-integration-id",
			MessageID:     "test-message-id",
			FromAddress:   "sender@example.com",
			FromName:      "Sender",
			To:            "recipient@example.com",
			Subject:       "Subject",
			Content:       "Content",
			Provider:      providerConfig,
			EmailOptions: domain.EmailOptions{
				Attachments: []domain.Attachment{
					{
						Filename:    "invoice.pdf",
						Content:     "invalid-base64-content!!!",
						ContentType: "application/pdf",
						Disposition: "attachment",
					},
				},
			},
		}
		err := service.SendEmail(context.Background(), request)

		// Verify error handling
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to decode content")
	})

	t.Run("Email with attachment API error", func(t *testing.T) {
		// Setup
		service, httpClient, _, logger := setupPostmarkTest(t)
		workspaceID := "workspace-123"

		// Create attachment
		pdfContent := "c2FtcGxlIHBkZiBjb250ZW50" // base64 of "sample pdf content"

		// Provider config
		providerConfig := &domain.EmailProvider{
			Kind: domain.EmailProviderKindPostmark,
			Postmark: &domain.PostmarkSettings{
				ServerToken: "test-server-token",
			},
		}

		// Allow logger calls for attachment debugging
		logger.EXPECT().Debug(gomock.Any()).AnyTimes()

		// Simulate error response (payload too large)
		httpClient.EXPECT().
			Do(gomock.Any()).
			Return(createMockResponse(http.StatusRequestEntityTooLarge, `{"ErrorCode":413,"Message":"Attachment too large"}`), nil)

		// Call the method
		request := domain.SendEmailProviderRequest{
			WorkspaceID:   workspaceID,
			IntegrationID: "test-integration-id",
			MessageID:     "test-message-id",
			FromAddress:   "sender@example.com",
			FromName:      "Sender",
			To:            "recipient@example.com",
			Subject:       "Subject",
			Content:       "Content",
			Provider:      providerConfig,
			EmailOptions: domain.EmailOptions{
				Attachments: []domain.Attachment{
					{
						Filename:    "invoice.pdf",
						Content:     pdfContent,
						ContentType: "application/pdf",
						Disposition: "attachment",
					},
				},
			},
		}
		err := service.SendEmail(context.Background(), request)

		// Verify error handling
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "postmark API error")
	})

	t.Run("Email with attachment without ContentType", func(t *testing.T) {
		// Setup
		service, httpClient, _, logger := setupPostmarkTest(t)
		workspaceID := "workspace-123"

		// Create attachment without content type
		base64Content := "c2FtcGxlIGNvbnRlbnQ=" // base64 of "sample content"

		// Provider config
		providerConfig := &domain.EmailProvider{
			Kind: domain.EmailProviderKindPostmark,
			Postmark: &domain.PostmarkSettings{
				ServerToken: "test-server-token",
			},
		}

		// Allow logger calls for attachment debugging
		logger.EXPECT().Debug(gomock.Any()).AnyTimes()

		// Expect HTTP request
		httpClient.EXPECT().
			Do(gomock.Any()).
			DoAndReturn(func(req *http.Request) (*http.Response, error) {
				// Verify request body
				body, _ := io.ReadAll(req.Body)
				var requestBody map[string]interface{}
				err := json.Unmarshal(body, &requestBody)
				require.NoError(t, err)

				// Check for attachments
				attachments, ok := requestBody["Attachments"].([]interface{})
				assert.True(t, ok, "Attachments should be present")
				assert.Len(t, attachments, 1)

				// Verify default content type is set
				attachment := attachments[0].(map[string]interface{})
				assert.Equal(t, "document.bin", attachment["Name"])
				assert.Equal(t, "application/octet-stream", attachment["ContentType"], "Should default to application/octet-stream")

				return createMockResponse(http.StatusOK, `{"MessageID":"12345"}`), nil
			})

		// Call the method
		request := domain.SendEmailProviderRequest{
			WorkspaceID:   workspaceID,
			IntegrationID: "test-integration-id",
			MessageID:     "test-message-id",
			FromAddress:   "sender@example.com",
			FromName:      "Sender",
			To:            "recipient@example.com",
			Subject:       "Subject",
			Content:       "Content",
			Provider:      providerConfig,
			EmailOptions: domain.EmailOptions{
				Attachments: []domain.Attachment{
					{
						Filename:    "document.bin",
						Content:     base64Content,
						ContentType: "", // Empty content type
						Disposition: "attachment",
					},
				},
			},
		}
		err := service.SendEmail(context.Background(), request)

		// Verify results
		assert.NoError(t, err)
	})

	t.Run("with RFC-8058 List-Unsubscribe headers", func(t *testing.T) {
		// Setup
		service, httpClient, _, _ := setupPostmarkTest(t)
		workspaceID := "workspace-123"

		// Provider config
		providerConfig := &domain.EmailProvider{
			Kind: domain.EmailProviderKindPostmark,
			Postmark: &domain.PostmarkSettings{
				ServerToken: "test-server-token",
			},
		}

		// Expect HTTP request
		httpClient.EXPECT().
			Do(gomock.Any()).
			DoAndReturn(func(req *http.Request) (*http.Response, error) {
				// Verify request body
				body, _ := io.ReadAll(req.Body)
				var requestBody map[string]interface{}
				err := json.Unmarshal(body, &requestBody)
				require.NoError(t, err)

				// Check for RFC-8058 List-Unsubscribe headers
				headers, ok := requestBody["Headers"].([]interface{})
				assert.True(t, ok, "Headers should be present")
				assert.Len(t, headers, 2)

				// Verify header values
				var foundListUnsubscribe, foundListUnsubscribePost bool
				for _, h := range headers {
					header := h.(map[string]interface{})
					name := header["Name"].(string)
					value := header["Value"].(string)
					if name == "List-Unsubscribe" {
						assert.Equal(t, "<https://example.com/unsubscribe/abc123>", value)
						foundListUnsubscribe = true
					}
					if name == "List-Unsubscribe-Post" {
						assert.Equal(t, "List-Unsubscribe=One-Click", value)
						foundListUnsubscribePost = true
					}
				}
				assert.True(t, foundListUnsubscribe, "List-Unsubscribe header should be present")
				assert.True(t, foundListUnsubscribePost, "List-Unsubscribe-Post header should be present")

				return createMockResponse(http.StatusOK, `{"MessageID":"12345"}`), nil
			})

		// Call the method
		request := domain.SendEmailProviderRequest{
			WorkspaceID:   workspaceID,
			IntegrationID: "test-integration-id",
			MessageID:     "test-message-id",
			FromAddress:   "sender@example.com",
			FromName:      "Sender",
			To:            "recipient@example.com",
			Subject:       "Subject",
			Content:       "Content",
			Provider:      providerConfig,
			EmailOptions: domain.EmailOptions{
				ListUnsubscribeURL: "https://example.com/unsubscribe/abc123",
			},
		}
		err := service.SendEmail(context.Background(), request)

		// Verify results
		assert.NoError(t, err)
	})

	t.Run("with RFC-8058 List-Unsubscribe headers and attachments", func(t *testing.T) {
		// Setup
		service, httpClient, _, mockLogger := setupPostmarkTest(t)
		workspaceID := "workspace-123"
		base64Content := base64.StdEncoding.EncodeToString([]byte("Hello World"))

		// Provider config
		providerConfig := &domain.EmailProvider{
			Kind: domain.EmailProviderKindPostmark,
			Postmark: &domain.PostmarkSettings{
				ServerToken: "test-server-token",
			},
		}

		// Allow logger calls for attachment debugging
		mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()

		// Expect HTTP request
		httpClient.EXPECT().
			Do(gomock.Any()).
			DoAndReturn(func(req *http.Request) (*http.Response, error) {
				// Verify request body
				body, _ := io.ReadAll(req.Body)
				var requestBody map[string]interface{}
				err := json.Unmarshal(body, &requestBody)
				require.NoError(t, err)

				// Check for RFC-8058 List-Unsubscribe headers
				headers, ok := requestBody["Headers"].([]interface{})
				assert.True(t, ok, "Headers should be present")
				assert.Len(t, headers, 2)

				// Check for attachments
				attachments, ok := requestBody["Attachments"].([]interface{})
				assert.True(t, ok, "Attachments should be present")
				assert.Len(t, attachments, 1)

				return createMockResponse(http.StatusOK, `{"MessageID":"12345"}`), nil
			})

		// Call the method
		request := domain.SendEmailProviderRequest{
			WorkspaceID:   workspaceID,
			IntegrationID: "test-integration-id",
			MessageID:     "test-message-id",
			FromAddress:   "sender@example.com",
			FromName:      "Sender",
			To:            "recipient@example.com",
			Subject:       "Subject",
			Content:       "Content",
			Provider:      providerConfig,
			EmailOptions: domain.EmailOptions{
				Attachments: []domain.Attachment{
					{
						Filename:    "test.txt",
						Content:     base64Content,
						ContentType: "text/plain",
						Disposition: "attachment",
					},
				},
				ListUnsubscribeURL: "https://example.com/unsubscribe/xyz789",
			},
		}
		err := service.SendEmail(context.Background(), request)

		// Verify results
		assert.NoError(t, err)
	})
}

// errorReader is a helper type that always returns an error when Read is called
type errorReader struct{}

func (e *errorReader) Read(p []byte) (n int, err error) {
	return 0, errors.New("read error")
}
