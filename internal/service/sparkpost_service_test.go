package service_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/Notifuse/notifuse/internal/service"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
)

// mockHTTPResponse creates a mock HTTP response
func mockHTTPResponse(statusCode int, body string) *http.Response {
	return &http.Response{
		StatusCode: statusCode,
		Body:       io.NopCloser(bytes.NewReader([]byte(body))),
	}
}

func TestSparkPostService_ListWebhooks(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mocks
	mockHTTPClient := mocks.NewMockHTTPClient(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	// Allow any log calls
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()

	// Initialize service
	sparkPostService := service.NewSparkPostService(mockHTTPClient, mockAuthService, mockLogger)

	// Test configuration
	config := domain.SparkPostSettings{
		Endpoint: "https://api.sparkpost.test",
		APIKey:   "test-api-key",
	}

	t.Run("Success", func(t *testing.T) {
		ctx := context.Background()

		// Mock successful response
		webhookListResponse := domain.SparkPostWebhookListResponse{
			Results: []domain.SparkPostWebhook{
				{
					ID:     "webhook-1",
					Name:   "Test Webhook",
					Target: "https://example.com/webhook",
					Events: []string{"delivery", "bounce"},
					Active: true,
				},
			},
		}
		responseJSON, _ := json.Marshal(webhookListResponse)

		// Expect HTTP request and return mocked response
		mockHTTPClient.EXPECT().
			Do(gomock.Any()).
			DoAndReturn(func(req *http.Request) (*http.Response, error) {
				// Verify request
				assert.Equal(t, "GET", req.Method)
				assert.Equal(t, "https://api.sparkpost.test/api/v1/webhooks", req.URL.String())
				assert.Equal(t, "Bearer test-api-key", req.Header.Get("Authorization"))

				return mockHTTPResponse(http.StatusOK, string(responseJSON)), nil
			})

		// Call the service method
		result, err := sparkPostService.ListWebhooks(ctx, config)

		// Verify results
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Len(t, result.Results, 1)
		assert.Equal(t, "webhook-1", result.Results[0].ID)
		assert.Equal(t, "Test Webhook", result.Results[0].Name)
	})

	t.Run("HTTP request error", func(t *testing.T) {
		ctx := context.Background()
		expectedErr := errors.New("connection error")

		// Mock HTTP client to return error
		mockHTTPClient.EXPECT().
			Do(gomock.Any()).
			Return(nil, expectedErr)

		// Call the service method
		result, err := sparkPostService.ListWebhooks(ctx, config)

		// Verify results
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "failed to execute request")
	})

	t.Run("Non-OK status code", func(t *testing.T) {
		ctx := context.Background()

		// Mock error response
		mockHTTPClient.EXPECT().
			Do(gomock.Any()).
			Return(mockHTTPResponse(http.StatusUnauthorized, `{"errors":[{"message":"Unauthorized"}]}`), nil)

		// Call the service method
		result, err := sparkPostService.ListWebhooks(ctx, config)

		// Verify results
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "API returned non-OK status code 401")
	})

	t.Run("Invalid response body", func(t *testing.T) {
		ctx := context.Background()

		// Mock invalid JSON response
		mockHTTPClient.EXPECT().
			Do(gomock.Any()).
			Return(mockHTTPResponse(http.StatusOK, `invalid json`), nil)

		// Call the service method
		result, err := sparkPostService.ListWebhooks(ctx, config)

		// Verify results
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "failed to decode response")
	})
}

func TestSparkPostService_CreateWebhook(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mocks
	mockHTTPClient := mocks.NewMockHTTPClient(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	// Allow any log calls
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()

	// Initialize service
	sparkPostService := service.NewSparkPostService(mockHTTPClient, mockAuthService, mockLogger)

	// Test configuration
	config := domain.SparkPostSettings{
		Endpoint: "https://api.sparkpost.test",
		APIKey:   "test-api-key",
	}

	webhook := domain.SparkPostWebhook{
		Name:     "Test Webhook",
		Target:   "https://example.com/webhook",
		Events:   []string{"delivery", "bounce"},
		Active:   true,
		AuthType: "none",
	}

	t.Run("Success", func(t *testing.T) {
		ctx := context.Background()

		// Mock successful response
		webhookResponse := domain.SparkPostWebhookResponse{
			Results: domain.SparkPostWebhook{
				ID:     "webhook-123",
				Name:   webhook.Name,
				Target: webhook.Target,
				Events: webhook.Events,
				Active: webhook.Active,
			},
		}
		responseJSON, _ := json.Marshal(webhookResponse)

		// Expect HTTP request and return mocked response
		mockHTTPClient.EXPECT().
			Do(gomock.Any()).
			DoAndReturn(func(req *http.Request) (*http.Response, error) {
				// Verify request
				assert.Equal(t, "POST", req.Method)
				assert.Equal(t, "https://api.sparkpost.test/api/v1/webhooks", req.URL.String())
				assert.Equal(t, "Bearer test-api-key", req.Header.Get("Authorization"))

				// Verify request body
				var requestBody domain.SparkPostWebhook
				body, _ := io.ReadAll(req.Body)
				_ = json.Unmarshal(body, &requestBody)
				assert.Equal(t, webhook.Name, requestBody.Name)
				assert.Equal(t, webhook.Target, requestBody.Target)

				return mockHTTPResponse(http.StatusOK, string(responseJSON)), nil
			})

		// Call the service method
		result, err := sparkPostService.CreateWebhook(ctx, config, webhook)

		// Verify results
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, "webhook-123", result.Results.ID)
		assert.Equal(t, webhook.Name, result.Results.Name)
	})
}

func TestSparkPostService_GetWebhookStatus(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mocks
	mockHTTPClient := mocks.NewMockHTTPClient(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	// Allow any log calls
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()

	// Initialize service
	sparkPostService := service.NewSparkPostService(mockHTTPClient, mockAuthService, mockLogger)

	// Test configuration
	providerConfig := &domain.EmailProvider{
		SparkPost: &domain.SparkPostSettings{
			Endpoint: "https://api.sparkpost.test",
			APIKey:   "test-api-key",
		},
	}

	workspaceID := "workspace-123"
	integrationID := "integration-123"

	t.Run("Webhook found", func(t *testing.T) {
		ctx := context.Background()

		// Mock list webhooks response with a matching webhook
		webhookTarget := "https://api.notifuse.com/webhook?provider=sparkpost&workspace_id=workspace-123&integration_id=integration-123"
		listResponse := domain.SparkPostWebhookListResponse{
			Results: []domain.SparkPostWebhook{
				{
					ID:     "webhook-123",
					Name:   "Notifuse Webhook",
					Target: webhookTarget,
					Events: []string{"delivery", "bounce"},
					Active: true,
				},
			},
		}
		listResponseJSON, _ := json.Marshal(listResponse)

		// Expect list webhooks request
		mockHTTPClient.EXPECT().
			Do(gomock.Any()).
			DoAndReturn(func(req *http.Request) (*http.Response, error) {
				assert.Equal(t, "GET", req.Method)
				assert.Equal(t, "https://api.sparkpost.test/api/v1/webhooks", req.URL.String())
				return mockHTTPResponse(http.StatusOK, string(listResponseJSON)), nil
			})

		// Call the service method
		result, err := sparkPostService.GetWebhookStatus(ctx, workspaceID, integrationID, providerConfig)

		// Verify results
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, domain.EmailProviderKindSparkPost, result.EmailProviderKind)
		assert.True(t, result.IsRegistered)
		assert.NotEmpty(t, result.Endpoints)
	})

	t.Run("Webhook not found", func(t *testing.T) {
		ctx := context.Background()

		// Mock list webhooks response with no matching webhook
		listResponse := domain.SparkPostWebhookListResponse{
			Results: []domain.SparkPostWebhook{
				{
					ID:     "webhook-456",
					Name:   "Other Webhook",
					Target: "https://other-service.com/webhook",
					Events: []string{"delivery"},
					Active: true,
				},
			},
		}
		listResponseJSON, _ := json.Marshal(listResponse)

		// Expect list webhooks request
		mockHTTPClient.EXPECT().
			Do(gomock.Any()).
			DoAndReturn(func(req *http.Request) (*http.Response, error) {
				assert.Equal(t, "GET", req.Method)
				assert.Equal(t, "https://api.sparkpost.test/api/v1/webhooks", req.URL.String())
				return mockHTTPResponse(http.StatusOK, string(listResponseJSON)), nil
			})

		// Call the service method
		result, err := sparkPostService.GetWebhookStatus(ctx, workspaceID, integrationID, providerConfig)

		// Verify results
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, domain.EmailProviderKindSparkPost, result.EmailProviderKind)
		assert.False(t, result.IsRegistered)
		assert.Empty(t, result.Endpoints)
	})

	t.Run("Sandbox mode", func(t *testing.T) {
		ctx := context.Background()

		// Create a provider config with sandbox mode enabled
		sandboxConfig := &domain.EmailProvider{
			SparkPost: &domain.SparkPostSettings{
				Endpoint:    "https://api.sparkpost.test",
				APIKey:      "test-api-key",
				SandboxMode: true,
			},
		}

		// Call the service method
		result, err := sparkPostService.GetWebhookStatus(ctx, workspaceID, integrationID, sandboxConfig)

		// Verify results
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, domain.EmailProviderKindSparkPost, result.EmailProviderKind)
		assert.True(t, result.IsRegistered)
		assert.NotEmpty(t, result.Endpoints)
		assert.Equal(t, true, result.ProviderDetails["sandbox_mode"])
	})
}

func TestSparkPostService_RegisterWebhooks(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mocks
	mockHTTPClient := mocks.NewMockHTTPClient(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	// Allow any log calls
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()

	// Initialize service
	sparkPostService := service.NewSparkPostService(mockHTTPClient, mockAuthService, mockLogger)

	// Test configuration
	providerConfig := &domain.EmailProvider{
		SparkPost: &domain.SparkPostSettings{
			Endpoint: "https://api.sparkpost.test",
			APIKey:   "test-api-key",
		},
	}

	workspaceID := "workspace-123"
	integrationID := "integration-123"
	baseURL := "https://api.notifuse.com/webhook"
	eventTypes := []domain.EmailEventType{domain.EmailEventDelivered, domain.EmailEventBounce}

	t.Run("Success - Create new webhook", func(t *testing.T) {
		ctx := context.Background()

		// Mock list webhooks response (empty list)
		listResponse := domain.SparkPostWebhookListResponse{
			Results: []domain.SparkPostWebhook{},
		}
		listResponseJSON, _ := json.Marshal(listResponse)

		// Mock create webhook response
		createResponse := domain.SparkPostWebhookResponse{
			Results: domain.SparkPostWebhook{
				ID:     "webhook-123",
				Name:   "Notifuse-integration-123",
				Target: domain.GenerateWebhookCallbackURL(baseURL, domain.EmailProviderKindSparkPost, workspaceID, integrationID),
				Events: []string{"delivery", "bounce"},
				Active: true,
			},
		}
		createResponseJSON, _ := json.Marshal(createResponse)

		// Expect list webhooks request
		mockHTTPClient.EXPECT().
			Do(gomock.Any()).
			DoAndReturn(func(req *http.Request) (*http.Response, error) {
				assert.Equal(t, "GET", req.Method)
				assert.Equal(t, "https://api.sparkpost.test/api/v1/webhooks", req.URL.String())
				return mockHTTPResponse(http.StatusOK, string(listResponseJSON)), nil
			})

		// Expect create webhook request
		mockHTTPClient.EXPECT().
			Do(gomock.Any()).
			DoAndReturn(func(req *http.Request) (*http.Response, error) {
				assert.Equal(t, "POST", req.Method)
				assert.Equal(t, "https://api.sparkpost.test/api/v1/webhooks", req.URL.String())
				return mockHTTPResponse(http.StatusOK, string(createResponseJSON)), nil
			})

		// Call the service method
		result, err := sparkPostService.RegisterWebhooks(ctx, workspaceID, integrationID, baseURL, eventTypes, providerConfig)

		// Verify results
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, domain.EmailProviderKindSparkPost, result.EmailProviderKind)
		assert.True(t, result.IsRegistered)
		assert.Len(t, result.Endpoints, 2) // One for each event type
	})

	t.Run("Success - Sandbox mode", func(t *testing.T) {
		ctx := context.Background()

		// Create a provider config with sandbox mode enabled
		sandboxConfig := &domain.EmailProvider{
			SparkPost: &domain.SparkPostSettings{
				Endpoint:    "https://api.sparkpost.test",
				APIKey:      "test-api-key",
				SandboxMode: true,
			},
		}

		// Call the service method
		result, err := sparkPostService.RegisterWebhooks(ctx, workspaceID, integrationID, baseURL, eventTypes, sandboxConfig)

		// Verify results
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, domain.EmailProviderKindSparkPost, result.EmailProviderKind)
		assert.True(t, result.IsRegistered)
		assert.Len(t, result.Endpoints, 2) // One for each event type
		assert.Equal(t, true, result.ProviderDetails["sandbox_mode"])
	})

	t.Run("Invalid provider config", func(t *testing.T) {
		ctx := context.Background()

		// Call with nil provider config
		result, err := sparkPostService.RegisterWebhooks(ctx, workspaceID, integrationID, baseURL, eventTypes, nil)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "configuration is missing or invalid")
	})
}

func TestSparkPostService_UnregisterWebhooks(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mocks
	mockHTTPClient := mocks.NewMockHTTPClient(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	// Allow any log calls
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()

	// Initialize service
	sparkPostService := service.NewSparkPostService(mockHTTPClient, mockAuthService, mockLogger)

	// Test configuration
	providerConfig := &domain.EmailProvider{
		SparkPost: &domain.SparkPostSettings{
			Endpoint: "https://api.sparkpost.test",
			APIKey:   "test-api-key",
		},
	}

	workspaceID := "workspace-123"
	integrationID := "integration-123"

	t.Run("Success - Delete existing webhook", func(t *testing.T) {
		ctx := context.Background()

		// Mock list webhooks response with a matching webhook
		webhookTarget := "https://api.notifuse.com/webhook?provider=sparkpost&workspace_id=workspace-123&integration_id=integration-123"
		listResponse := domain.SparkPostWebhookListResponse{
			Results: []domain.SparkPostWebhook{
				{
					ID:     "webhook-123",
					Name:   "Notifuse Webhook",
					Target: webhookTarget,
					Events: []string{"delivery", "bounce"},
					Active: true,
				},
			},
		}
		listResponseJSON, _ := json.Marshal(listResponse)

		// Expect list webhooks request
		mockHTTPClient.EXPECT().
			Do(gomock.Any()).
			DoAndReturn(func(req *http.Request) (*http.Response, error) {
				assert.Equal(t, "GET", req.Method)
				assert.Equal(t, "https://api.sparkpost.test/api/v1/webhooks", req.URL.String())
				return mockHTTPResponse(http.StatusOK, string(listResponseJSON)), nil
			})

		// Expect delete webhook request
		mockHTTPClient.EXPECT().
			Do(gomock.Any()).
			DoAndReturn(func(req *http.Request) (*http.Response, error) {
				assert.Equal(t, "DELETE", req.Method)
				assert.Equal(t, "https://api.sparkpost.test/api/v1/webhooks/webhook-123", req.URL.String())
				return mockHTTPResponse(http.StatusOK, "{}"), nil
			})

		// Call the service method
		err := sparkPostService.UnregisterWebhooks(ctx, workspaceID, integrationID, providerConfig)

		// Verify results
		assert.NoError(t, err)
	})

	t.Run("Success - Sandbox mode", func(t *testing.T) {
		ctx := context.Background()

		// Create a provider config with sandbox mode enabled
		sandboxConfig := &domain.EmailProvider{
			SparkPost: &domain.SparkPostSettings{
				Endpoint:    "https://api.sparkpost.test",
				APIKey:      "test-api-key",
				SandboxMode: true,
			},
		}

		// Call the service method - should succeed without making any HTTP calls
		err := sparkPostService.UnregisterWebhooks(ctx, workspaceID, integrationID, sandboxConfig)

		// Verify results
		assert.NoError(t, err)
	})

	t.Run("No matching webhooks", func(t *testing.T) {
		ctx := context.Background()

		// Mock list webhooks response with no matching webhooks
		listResponse := domain.SparkPostWebhookListResponse{
			Results: []domain.SparkPostWebhook{
				{
					ID:     "webhook-abc",
					Name:   "Other Webhook",
					Target: "https://other-service.com/webhook",
					Events: []string{"delivery"},
					Active: true,
				},
			},
		}
		listResponseJSON, _ := json.Marshal(listResponse)

		// Expect list webhooks request
		mockHTTPClient.EXPECT().
			Do(gomock.Any()).
			DoAndReturn(func(req *http.Request) (*http.Response, error) {
				assert.Equal(t, "GET", req.Method)
				return mockHTTPResponse(http.StatusOK, string(listResponseJSON)), nil
			})

		// Call the service method
		err := sparkPostService.UnregisterWebhooks(ctx, workspaceID, integrationID, providerConfig)

		// Verify results - should succeed as there's nothing to delete
		assert.NoError(t, err)
	})
}

func TestSparkPostService_GetWebhook(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHTTPClient := mocks.NewMockHTTPClient(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()

	sparkPostService := service.NewSparkPostService(mockHTTPClient, mockAuthService, mockLogger)

	config := domain.SparkPostSettings{
		Endpoint: "https://api.sparkpost.test",
		APIKey:   "test-api-key",
	}
	webhookID := "webhook-123"

	t.Run("Success", func(t *testing.T) {
		ctx := context.Background()

		webhookResponse := domain.SparkPostWebhookResponse{
			Results: domain.SparkPostWebhook{
				ID:     webhookID,
				Name:   "Test Webhook",
				Target: "https://example.com/webhook",
				Events: []string{"delivery", "bounce"},
				Active: true,
			},
		}
		responseJSON, _ := json.Marshal(webhookResponse)

		mockHTTPClient.EXPECT().
			Do(gomock.Any()).
			DoAndReturn(func(req *http.Request) (*http.Response, error) {
				assert.Equal(t, "GET", req.Method)
				assert.Equal(t, "https://api.sparkpost.test/api/v1/webhooks/webhook-123", req.URL.String())
				assert.Equal(t, "Bearer test-api-key", req.Header.Get("Authorization"))

				return mockHTTPResponse(http.StatusOK, string(responseJSON)), nil
			})

		result, err := sparkPostService.GetWebhook(ctx, config, webhookID)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, webhookID, result.Results.ID)
		assert.Equal(t, "Test Webhook", result.Results.Name)
	})

	t.Run("Error response", func(t *testing.T) {
		ctx := context.Background()

		mockHTTPClient.EXPECT().
			Do(gomock.Any()).
			Return(mockHTTPResponse(http.StatusNotFound, `{"errors":[{"message":"Webhook not found"}]}`), nil)

		result, err := sparkPostService.GetWebhook(ctx, config, webhookID)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "API returned non-OK status code 404")
	})
}

func TestSparkPostService_UpdateWebhook(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHTTPClient := mocks.NewMockHTTPClient(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()

	sparkPostService := service.NewSparkPostService(mockHTTPClient, mockAuthService, mockLogger)

	config := domain.SparkPostSettings{
		Endpoint: "https://api.sparkpost.test",
		APIKey:   "test-api-key",
	}
	webhookID := "webhook-123"
	webhook := domain.SparkPostWebhook{
		Name:     "Updated Webhook",
		Target:   "https://example.com/webhook",
		Events:   []string{"delivery", "bounce", "spam_complaint"},
		Active:   true,
		AuthType: "none",
	}

	t.Run("Success", func(t *testing.T) {
		ctx := context.Background()

		webhookResponse := domain.SparkPostWebhookResponse{
			Results: domain.SparkPostWebhook{
				ID:     webhookID,
				Name:   webhook.Name,
				Target: webhook.Target,
				Events: webhook.Events,
				Active: webhook.Active,
			},
		}
		responseJSON, _ := json.Marshal(webhookResponse)

		mockHTTPClient.EXPECT().
			Do(gomock.Any()).
			DoAndReturn(func(req *http.Request) (*http.Response, error) {
				assert.Equal(t, "PUT", req.Method)
				assert.Equal(t, "https://api.sparkpost.test/api/v1/webhooks/webhook-123", req.URL.String())

				var requestBody domain.SparkPostWebhook
				body, _ := io.ReadAll(req.Body)
				_ = json.Unmarshal(body, &requestBody)
				assert.Equal(t, webhook.Name, requestBody.Name)
				assert.Equal(t, webhook.Events, requestBody.Events)

				return mockHTTPResponse(http.StatusOK, string(responseJSON)), nil
			})

		result, err := sparkPostService.UpdateWebhook(ctx, config, webhookID, webhook)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, webhookID, result.Results.ID)
		assert.Equal(t, webhook.Name, result.Results.Name)
		assert.Equal(t, webhook.Events, result.Results.Events)
	})

	t.Run("Empty Endpoint - Request Creation Error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		// Create mocks
		mockHTTPClient := mocks.NewMockHTTPClient(ctrl)
		mockAuthService := mocks.NewMockAuthService(ctrl)
		mockLogger := pkgmocks.NewMockLogger(ctrl)

		// Allow any log calls
		mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
		mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
		mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()

		// Initialize service
		sparkPostService := service.NewSparkPostService(mockHTTPClient, mockAuthService, mockLogger)

		ctx := context.Background()

		// Test with empty endpoint to trigger request creation issues
		emptySettings := domain.SparkPostSettings{
			Endpoint: "", // This will create a malformed URL
			APIKey:   "test-api-key",
		}

		// Even with empty endpoint, the HTTP request will be created with a malformed URL
		// So we need to mock the HTTP client call and return an error
		mockHTTPClient.EXPECT().
			Do(gomock.Any()).
			DoAndReturn(func(req *http.Request) (*http.Response, error) {
				// Verify the URL is malformed as expected
				assert.Equal(t, "PUT", req.Method)
				assert.Equal(t, "/api/v1/webhooks/webhook-123", req.URL.String())
				// Return a connection error to simulate what would happen with an invalid URL
				return nil, errors.New("dial tcp: no such host")
			})

		// Call the method
		result, err := sparkPostService.UpdateWebhook(ctx, emptySettings, webhookID, webhook)

		// Verify results - should fail due to HTTP error
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "failed to execute request")
	})

	t.Run("HTTP Request Error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		// Create mocks
		mockHTTPClient := mocks.NewMockHTTPClient(ctrl)
		mockAuthService := mocks.NewMockAuthService(ctrl)
		mockLogger := pkgmocks.NewMockLogger(ctrl)

		// Allow any log calls
		mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
		mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
		mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()

		// Initialize service
		sparkPostService := service.NewSparkPostService(mockHTTPClient, mockAuthService, mockLogger)

		ctx := context.Background()

		// Test with empty endpoint to trigger request creation issues
		emptySettings := domain.SparkPostSettings{
			Endpoint: "", // This will create a malformed URL
			APIKey:   "test-api-key",
		}

		// Even with empty endpoint, the HTTP request will be created with a malformed URL
		// So we need to mock the HTTP client call and return an error
		mockHTTPClient.EXPECT().
			Do(gomock.Any()).
			DoAndReturn(func(req *http.Request) (*http.Response, error) {
				// Verify the URL is malformed as expected
				assert.Equal(t, "PUT", req.Method)
				assert.Equal(t, "/api/v1/webhooks/webhook-123", req.URL.String())
				// Return a connection error to simulate what would happen with an invalid URL
				return nil, errors.New("dial tcp: no such host")
			})

		// Call the method
		result, err := sparkPostService.UpdateWebhook(ctx, emptySettings, webhookID, webhook)

		// Verify results - should fail due to HTTP error
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "failed to execute request")
	})
}

func TestSparkPostService_DeleteWebhook(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHTTPClient := mocks.NewMockHTTPClient(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()

	sparkPostService := service.NewSparkPostService(mockHTTPClient, mockAuthService, mockLogger)

	config := domain.SparkPostSettings{
		Endpoint: "https://api.sparkpost.test",
		APIKey:   "test-api-key",
	}
	webhookID := "webhook-123"

	t.Run("Success", func(t *testing.T) {
		ctx := context.Background()

		mockHTTPClient.EXPECT().
			Do(gomock.Any()).
			DoAndReturn(func(req *http.Request) (*http.Response, error) {
				assert.Equal(t, "DELETE", req.Method)
				assert.Equal(t, "https://api.sparkpost.test/api/v1/webhooks/webhook-123", req.URL.String())

				return mockHTTPResponse(http.StatusOK, "{}"), nil
			})

		err := sparkPostService.DeleteWebhook(ctx, config, webhookID)

		assert.NoError(t, err)
	})

	t.Run("Error response", func(t *testing.T) {
		ctx := context.Background()

		mockHTTPClient.EXPECT().
			Do(gomock.Any()).
			Return(mockHTTPResponse(http.StatusNotFound, `{"errors":[{"message":"Webhook not found"}]}`), nil)

		err := sparkPostService.DeleteWebhook(ctx, config, webhookID)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "API returned non-OK status code 404")
	})
}

func TestSparkPostService_TestWebhook(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHTTPClient := mocks.NewMockHTTPClient(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()

	sparkPostService := service.NewSparkPostService(mockHTTPClient, mockAuthService, mockLogger)

	config := domain.SparkPostSettings{
		Endpoint: "https://api.sparkpost.test",
		APIKey:   "test-api-key",
	}
	webhookID := "webhook-123"

	t.Run("Success", func(t *testing.T) {
		ctx := context.Background()

		mockHTTPClient.EXPECT().
			Do(gomock.Any()).
			DoAndReturn(func(req *http.Request) (*http.Response, error) {
				assert.Equal(t, "POST", req.Method)
				assert.Equal(t, "https://api.sparkpost.test/api/v1/webhooks/webhook-123/validate", req.URL.String())

				return mockHTTPResponse(http.StatusOK, `{"results":{"message":"Test event sent successfully"}}`), nil
			})

		err := sparkPostService.TestWebhook(ctx, config, webhookID)

		assert.NoError(t, err)
	})
}

func TestSparkPostService_ValidateWebhook(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHTTPClient := mocks.NewMockHTTPClient(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()

	sparkPostService := service.NewSparkPostService(mockHTTPClient, mockAuthService, mockLogger)

	config := domain.SparkPostSettings{
		Endpoint: "https://api.sparkpost.test",
		APIKey:   "test-api-key",
	}
	webhook := domain.SparkPostWebhook{
		Target: "https://example.com/webhook",
	}

	t.Run("Valid webhook", func(t *testing.T) {
		ctx := context.Background()

		mockHTTPClient.EXPECT().
			Do(gomock.Any()).
			DoAndReturn(func(req *http.Request) (*http.Response, error) {
				assert.Equal(t, "POST", req.Method)
				assert.Equal(t, "https://api.sparkpost.test/api/v1/webhooks/validate", req.URL.String())

				// Verify request body
				var requestBody map[string]string
				body, _ := io.ReadAll(req.Body)
				_ = json.Unmarshal(body, &requestBody)
				assert.Equal(t, webhook.Target, requestBody["target"])

				return mockHTTPResponse(http.StatusOK, `{"results":{"valid":true}}`), nil
			})

		isValid, err := sparkPostService.ValidateWebhook(ctx, config, webhook)

		assert.NoError(t, err)
		assert.True(t, isValid)
	})

	t.Run("Invalid webhook", func(t *testing.T) {
		ctx := context.Background()

		mockHTTPClient.EXPECT().
			Do(gomock.Any()).
			Return(mockHTTPResponse(http.StatusOK, `{"results":{"valid":false}}`), nil)

		isValid, err := sparkPostService.ValidateWebhook(ctx, config, webhook)

		assert.NoError(t, err)
		assert.False(t, isValid)
	})

	t.Run("Error decoding response", func(t *testing.T) {
		ctx := context.Background()

		mockHTTPClient.EXPECT().
			Do(gomock.Any()).
			Return(mockHTTPResponse(http.StatusOK, `invalid json`), nil)

		isValid, err := sparkPostService.ValidateWebhook(ctx, config, webhook)

		assert.Error(t, err)
		assert.False(t, isValid)
		assert.Contains(t, err.Error(), "failed to decode validation response")
	})
}

func TestSparkPostService_SendEmail(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mocks
	mockHTTPClient := mocks.NewMockHTTPClient(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	// Allow any log calls
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()

	// Initialize service
	sparkPostService := service.NewSparkPostService(mockHTTPClient, mockAuthService, mockLogger)

	// Test data
	workspaceID := "workspace-123"
	fromAddress := "sender@example.com"
	fromName := "Test Sender"
	to := "recipient@example.com"
	subject := "Test Subject"
	content := "<p>Test Email Content</p>"

	t.Run("Success", func(t *testing.T) {
		ctx := context.Background()

		// Create provider config
		provider := &domain.EmailProvider{
			SparkPost: &domain.SparkPostSettings{
				Endpoint: "https://api.sparkpost.test",
				APIKey:   "test-api-key",
			},
		}

		// Expect HTTP request and return success response
		mockHTTPClient.EXPECT().
			Do(gomock.Any()).
			DoAndReturn(func(req *http.Request) (*http.Response, error) {
				// Verify request
				assert.Equal(t, "POST", req.Method)
				assert.Equal(t, "https://api.sparkpost.test/api/v1/transmissions", req.URL.String())
				assert.Equal(t, "Bearer test-api-key", req.Header.Get("Authorization"))
				assert.Equal(t, "application/json", req.Header.Get("Content-Type"))

				// Verify request body
				body, _ := io.ReadAll(req.Body)
				var emailReq map[string]interface{}
				err := json.Unmarshal(body, &emailReq)
				assert.NoError(t, err)

				// Check essential fields
				recipients, ok := emailReq["recipients"].([]interface{})
				assert.True(t, ok)
				assert.Len(t, recipients, 1)

				// Check the address structure matches what the API is sending
				recipientObj, ok := recipients[0].(map[string]interface{})
				assert.True(t, ok)
				address, ok := recipientObj["address"].(map[string]interface{})
				assert.True(t, ok)
				assert.Equal(t, to, address["email"])

				// Check content
				contentMap, ok := emailReq["content"].(map[string]interface{})
				assert.True(t, ok)
				assert.Equal(t, content, contentMap["html"])

				// Check subject and from are inside content map
				fromMap, ok := contentMap["from"].(map[string]interface{})
				assert.True(t, ok)
				assert.Equal(t, fromAddress, fromMap["email"])
				assert.Equal(t, fromName, fromMap["name"])
				assert.Equal(t, subject, contentMap["subject"])

				// Check that metadata contains our message ID
				metadata, ok := emailReq["metadata"].(map[string]interface{})
				assert.True(t, ok)
				assert.Equal(t, "test-message-id", metadata["notifuse_message_id"])

				return mockHTTPResponse(http.StatusOK, `{"results":{"id":"test-transmission-id"}}`), nil
			})

		// Call the service method
		request := domain.SendEmailProviderRequest{
			WorkspaceID:   workspaceID,
			IntegrationID: "test-integration-id",
			MessageID:     "test-message-id",
			FromAddress:   fromAddress,
			FromName:      fromName,
			To:            to,
			Subject:       subject,
			Content:       content,
			Provider:      provider,
			EmailOptions:  domain.EmailOptions{},
		}
		err := sparkPostService.SendEmail(ctx, request)

		// Verify results
		assert.NoError(t, err)
	})

	t.Run("Missing SparkPost configuration", func(t *testing.T) {
		ctx := context.Background()

		// Create provider without SparkPost config
		provider := &domain.EmailProvider{}

		// Call the service method
		request := domain.SendEmailProviderRequest{
			WorkspaceID:   workspaceID,
			IntegrationID: "test-integration-id",
			MessageID:     "test-message-id",
			FromAddress:   fromAddress,
			FromName:      fromName,
			To:            to,
			Subject:       subject,
			Content:       content,
			Provider:      provider,
			EmailOptions:  domain.EmailOptions{},
		}
		err := sparkPostService.SendEmail(ctx, request)

		// Verify results
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "SparkPost provider is not configured")
	})

	t.Run("HTTP request error", func(t *testing.T) {
		ctx := context.Background()
		expectedErr := errors.New("connection error")

		// Create provider config
		provider := &domain.EmailProvider{
			SparkPost: &domain.SparkPostSettings{
				Endpoint: "https://api.sparkpost.test",
				APIKey:   "test-api-key",
			},
		}

		// Mock HTTP client to return error
		mockHTTPClient.EXPECT().
			Do(gomock.Any()).
			Return(nil, expectedErr)

		// Call the service method
		request := domain.SendEmailProviderRequest{
			WorkspaceID:   workspaceID,
			IntegrationID: "test-integration-id",
			MessageID:     "test-message-id",
			FromAddress:   fromAddress,
			FromName:      fromName,
			To:            to,
			Subject:       subject,
			Content:       content,
			Provider:      provider,
			EmailOptions:  domain.EmailOptions{},
		}
		err := sparkPostService.SendEmail(ctx, request)

		// Verify results
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to execute request")
	})

	t.Run("API error response", func(t *testing.T) {
		ctx := context.Background()

		// Create provider config
		provider := &domain.EmailProvider{
			SparkPost: &domain.SparkPostSettings{
				Endpoint: "https://api.sparkpost.test",
				APIKey:   "test-api-key",
			},
		}

		// Mock error response
		mockHTTPClient.EXPECT().
			Do(gomock.Any()).
			Return(mockHTTPResponse(http.StatusBadRequest, `{"errors":[{"message":"Invalid recipient address"}]}`), nil)

		// Call the service method
		request := domain.SendEmailProviderRequest{
			WorkspaceID:   workspaceID,
			IntegrationID: "test-integration-id",
			MessageID:     "test-message-id",
			FromAddress:   fromAddress,
			FromName:      fromName,
			To:            to,
			Subject:       subject,
			Content:       content,
			Provider:      provider,
			EmailOptions:  domain.EmailOptions{},
		}
		err := sparkPostService.SendEmail(ctx, request)

		// Verify results
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "API returned non-OK status code 400")
	})

	t.Run("Sandbox mode", func(t *testing.T) {
		ctx := context.Background()

		// Create provider with sandbox mode enabled
		provider := &domain.EmailProvider{
			SparkPost: &domain.SparkPostSettings{
				Endpoint:    "https://api.sparkpost.test",
				APIKey:      "test-api-key",
				SandboxMode: true,
			},
		}

		// Expect HTTP request and return success response
		mockHTTPClient.EXPECT().
			Do(gomock.Any()).
			Return(mockHTTPResponse(http.StatusOK, `{"results":{"id":"test-transmission-id"}}`), nil)

		// Call the service method
		request := domain.SendEmailProviderRequest{
			WorkspaceID:   workspaceID,
			IntegrationID: "test-integration-id",
			MessageID:     "test-message-id",
			FromAddress:   fromAddress,
			FromName:      fromName,
			To:            to,
			Subject:       subject,
			Content:       content,
			Provider:      provider,
			EmailOptions:  domain.EmailOptions{},
		}
		err := sparkPostService.SendEmail(ctx, request)

		// Verify results - should succeed in sandbox mode
		assert.NoError(t, err)
	})
}

func TestSparkPostService_directUpdateWebhook(t *testing.T) {
	// Test configuration
	settings := &domain.SparkPostSettings{
		Endpoint: "https://api.sparkpost.test",
		APIKey:   "test-api-key",
	}

	webhookID := "webhook-123"
	webhook := domain.SparkPostWebhook{
		Name:     "Updated Webhook",
		Target:   "https://example.com/updated-webhook",
		Events:   []string{"delivery", "bounce", "spam_complaint"},
		Active:   true,
		AuthType: "none",
	}

	t.Run("Success", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		// Create mocks
		mockHTTPClient := mocks.NewMockHTTPClient(ctrl)
		mockAuthService := mocks.NewMockAuthService(ctrl)
		mockLogger := pkgmocks.NewMockLogger(ctrl)

		// Allow any log calls
		mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
		mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
		mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()

		// Initialize service
		sparkPostService := service.NewSparkPostService(mockHTTPClient, mockAuthService, mockLogger)

		ctx := context.Background()

		// Mock successful response
		webhookResponse := domain.SparkPostWebhookResponse{
			Results: domain.SparkPostWebhook{
				ID:       webhookID,
				Name:     webhook.Name,
				Target:   webhook.Target,
				Events:   webhook.Events,
				Active:   webhook.Active,
				AuthType: webhook.AuthType,
			},
		}
		responseJSON, _ := json.Marshal(webhookResponse)

		// Expect HTTP request and return mocked response
		mockHTTPClient.EXPECT().
			Do(gomock.Any()).
			DoAndReturn(func(req *http.Request) (*http.Response, error) {
				// Verify request method and URL
				assert.Equal(t, "PUT", req.Method)
				assert.Equal(t, "https://api.sparkpost.test/api/v1/webhooks/webhook-123", req.URL.String())

				// Verify headers
				assert.Equal(t, "Bearer test-api-key", req.Header.Get("Authorization"))
				assert.Equal(t, "application/json", req.Header.Get("Content-Type"))
				assert.Equal(t, "application/json", req.Header.Get("Accept"))

				// Verify request body
				body, err := io.ReadAll(req.Body)
				assert.NoError(t, err)

				var requestWebhook domain.SparkPostWebhook
				err = json.Unmarshal(body, &requestWebhook)
				assert.NoError(t, err)
				assert.Equal(t, webhook.Name, requestWebhook.Name)
				assert.Equal(t, webhook.Target, requestWebhook.Target)
				assert.Equal(t, webhook.Events, requestWebhook.Events)

				return mockHTTPResponse(http.StatusOK, string(responseJSON)), nil
			})

		// Call the method
		result, err := sparkPostService.UpdateWebhook(ctx, *settings, webhookID, webhook)

		// Verify results
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, webhookID, result.Results.ID)
		assert.Equal(t, webhook.Name, result.Results.Name)
		assert.Equal(t, webhook.Target, result.Results.Target)
	})

	t.Run("HTTP Request Error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		// Create mocks
		mockHTTPClient := mocks.NewMockHTTPClient(ctrl)
		mockAuthService := mocks.NewMockAuthService(ctrl)
		mockLogger := pkgmocks.NewMockLogger(ctrl)

		// Allow any log calls
		mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
		mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
		mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()

		// Initialize service
		sparkPostService := service.NewSparkPostService(mockHTTPClient, mockAuthService, mockLogger)

		ctx := context.Background()

		// Test with empty endpoint to trigger request creation issues
		emptySettings := domain.SparkPostSettings{
			Endpoint: "", // This will create a malformed URL
			APIKey:   "test-api-key",
		}

		// Even with empty endpoint, the HTTP request will be created with a malformed URL
		// So we need to mock the HTTP client call and return an error
		mockHTTPClient.EXPECT().
			Do(gomock.Any()).
			DoAndReturn(func(req *http.Request) (*http.Response, error) {
				// Verify the URL is malformed as expected
				assert.Equal(t, "PUT", req.Method)
				assert.Equal(t, "/api/v1/webhooks/webhook-123", req.URL.String())
				// Return a connection error to simulate what would happen with an invalid URL
				return nil, errors.New("dial tcp: no such host")
			})

		// Call the method
		result, err := sparkPostService.UpdateWebhook(ctx, emptySettings, webhookID, webhook)

		// Verify results - should fail due to HTTP error
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "failed to execute request")
	})

	t.Run("Non-OK Status Code", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		// Create mocks
		mockHTTPClient := mocks.NewMockHTTPClient(ctrl)
		mockAuthService := mocks.NewMockAuthService(ctrl)
		mockLogger := pkgmocks.NewMockLogger(ctrl)

		// Allow any log calls
		mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
		mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
		mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()

		// Initialize service
		sparkPostService := service.NewSparkPostService(mockHTTPClient, mockAuthService, mockLogger)

		ctx := context.Background()

		// Mock error response
		errorResponse := `{"errors":[{"message":"Webhook not found"}]}`
		mockHTTPClient.EXPECT().
			Do(gomock.Any()).
			Return(mockHTTPResponse(http.StatusNotFound, errorResponse), nil)

		// Call the method
		result, err := sparkPostService.UpdateWebhook(ctx, *settings, webhookID, webhook)

		// Verify results
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "API returned non-OK status code 404")
	})

	t.Run("Invalid Response JSON", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		// Create mocks
		mockHTTPClient := mocks.NewMockHTTPClient(ctrl)
		mockAuthService := mocks.NewMockAuthService(ctrl)
		mockLogger := pkgmocks.NewMockLogger(ctrl)

		// Allow any log calls
		mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
		mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
		mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()

		// Initialize service
		sparkPostService := service.NewSparkPostService(mockHTTPClient, mockAuthService, mockLogger)

		ctx := context.Background()

		// Mock invalid JSON response
		mockHTTPClient.EXPECT().
			Do(gomock.Any()).
			Return(mockHTTPResponse(http.StatusOK, `invalid json response`), nil)

		// Call the method
		result, err := sparkPostService.UpdateWebhook(ctx, *settings, webhookID, webhook)

		// Verify results
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "failed to decode response")
	})

	t.Run("Empty Webhook ID", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		// Create mocks
		mockHTTPClient := mocks.NewMockHTTPClient(ctrl)
		mockAuthService := mocks.NewMockAuthService(ctrl)
		mockLogger := pkgmocks.NewMockLogger(ctrl)

		// Allow any log calls
		mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
		mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
		mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()

		// Initialize service
		sparkPostService := service.NewSparkPostService(mockHTTPClient, mockAuthService, mockLogger)

		ctx := context.Background()

		// Mock successful response (even though the URL will be malformed)
		webhookResponse := domain.SparkPostWebhookResponse{
			Results: webhook,
		}
		responseJSON, _ := json.Marshal(webhookResponse)

		mockHTTPClient.EXPECT().
			Do(gomock.Any()).
			DoAndReturn(func(req *http.Request) (*http.Response, error) {
				// Verify the URL construction with empty webhook ID
				assert.Equal(t, "PUT", req.Method)
				assert.Equal(t, "https://api.sparkpost.test/api/v1/webhooks/", req.URL.String())
				return mockHTTPResponse(http.StatusOK, string(responseJSON)), nil
			})

		// Call the method with empty webhook ID
		result, err := sparkPostService.UpdateWebhook(ctx, *settings, "", webhook)

		// Should succeed (the API might handle empty ID differently)
		assert.NoError(t, err)
		assert.NotNil(t, result)
	})

	t.Run("Different Status Codes", func(t *testing.T) {
		testCases := []struct {
			name       string
			statusCode int
			shouldFail bool
		}{
			{"Status OK", http.StatusOK, false},
			{"Status Bad Request", http.StatusBadRequest, true},
			{"Status Unauthorized", http.StatusUnauthorized, true},
			{"Status Forbidden", http.StatusForbidden, true},
			{"Status Internal Server Error", http.StatusInternalServerError, true},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				ctrl := gomock.NewController(t)
				defer ctrl.Finish()

				// Create mocks
				mockHTTPClient := mocks.NewMockHTTPClient(ctrl)
				mockAuthService := mocks.NewMockAuthService(ctrl)
				mockLogger := pkgmocks.NewMockLogger(ctrl)

				// Allow any log calls
				mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
				mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
				mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()

				// Initialize service
				sparkPostService := service.NewSparkPostService(mockHTTPClient, mockAuthService, mockLogger)

				ctx := context.Background()

				webhookResponse := domain.SparkPostWebhookResponse{
					Results: webhook,
				}
				responseJSON, _ := json.Marshal(webhookResponse)

				var responseBody string
				if tc.shouldFail {
					responseBody = `{"errors":[{"message":"API Error"}]}`
				} else {
					responseBody = string(responseJSON)
				}

				mockHTTPClient.EXPECT().
					Do(gomock.Any()).
					Return(mockHTTPResponse(tc.statusCode, responseBody), nil)

				result, err := sparkPostService.UpdateWebhook(ctx, *settings, webhookID, webhook)

				if tc.shouldFail {
					assert.Error(t, err)
					assert.Nil(t, result)
					assert.Contains(t, err.Error(), fmt.Sprintf("API returned non-OK status code %d", tc.statusCode))
				} else {
					assert.NoError(t, err)
					assert.NotNil(t, result)
				}
			})
		}
	})

	t.Run("Large Webhook Data", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		// Create mocks
		mockHTTPClient := mocks.NewMockHTTPClient(ctrl)
		mockAuthService := mocks.NewMockAuthService(ctrl)
		mockLogger := pkgmocks.NewMockLogger(ctrl)

		// Allow any log calls
		mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
		mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
		mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()

		// Initialize service
		sparkPostService := service.NewSparkPostService(mockHTTPClient, mockAuthService, mockLogger)

		ctx := context.Background()

		// Create webhook with large data
		largeWebhook := domain.SparkPostWebhook{
			Name:   "Test Webhook with Very Long Name " + string(make([]byte, 1000)),
			Target: "https://example.com/webhook",
			Events: make([]string, 100), // Large events array
			Active: true,
		}

		// Fill events array
		for i := 0; i < 100; i++ {
			largeWebhook.Events[i] = fmt.Sprintf("event_%d", i)
		}

		webhookResponse := domain.SparkPostWebhookResponse{
			Results: largeWebhook,
		}
		responseJSON, _ := json.Marshal(webhookResponse)

		mockHTTPClient.EXPECT().
			Do(gomock.Any()).
			DoAndReturn(func(req *http.Request) (*http.Response, error) {
				// Verify that large data is handled correctly
				body, err := io.ReadAll(req.Body)
				assert.NoError(t, err)
				assert.True(t, len(body) > 1000) // Should be quite large

				return mockHTTPResponse(http.StatusOK, string(responseJSON)), nil
			})

		result, err := sparkPostService.UpdateWebhook(ctx, *settings, webhookID, largeWebhook)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, largeWebhook.Name, result.Results.Name)
		assert.Len(t, result.Results.Events, 100)
	})
}

// TestSparkPostService_directUpdateWebhook_ThroughRegisterWebhooks tests the directUpdateWebhook method
// indirectly through RegisterWebhooks which calls it when updating existing webhooks
func TestSparkPostService_directUpdateWebhook_ThroughRegisterWebhooks(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mocks
	mockHTTPClient := mocks.NewMockHTTPClient(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	// Allow any log calls
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()

	// Initialize service
	sparkPostService := service.NewSparkPostService(mockHTTPClient, mockAuthService, mockLogger)

	// Test configuration
	providerConfig := &domain.EmailProvider{
		SparkPost: &domain.SparkPostSettings{
			Endpoint: "https://api.sparkpost.test",
			APIKey:   "test-api-key",
		},
	}

	workspaceID := "workspace-123"
	integrationID := "integration-456"
	baseURL := "https://example.com"
	eventTypes := []domain.EmailEventType{domain.EmailEventDelivered, domain.EmailEventBounce}

	t.Run("Update Existing Webhook Success", func(t *testing.T) {
		ctx := context.Background()

		// Mock list webhooks response - webhook already exists
		existingWebhook := domain.SparkPostWebhook{
			ID:     "existing-webhook-123",
			Name:   "Existing Webhook",
			Target: fmt.Sprintf("%s/webhooks/email/sparkpost?workspace_id=%s&integration_id=%s", baseURL, workspaceID, integrationID),
			Events: []string{"delivery"},
			Active: true,
		}

		listResponse := domain.SparkPostWebhookListResponse{
			Results: []domain.SparkPostWebhook{existingWebhook},
		}
		listResponseJSON, _ := json.Marshal(listResponse)

		// Mock update webhook response
		updatedWebhook := existingWebhook
		updatedWebhook.Events = []string{"delivery", "bounce"}
		updateResponse := domain.SparkPostWebhookResponse{
			Results: updatedWebhook,
		}
		updateResponseJSON, _ := json.Marshal(updateResponse)

		// Expect both list and update calls
		gomock.InOrder(
			mockHTTPClient.EXPECT().
				Do(gomock.Any()).
				DoAndReturn(func(req *http.Request) (*http.Response, error) {
					// First call should be list webhooks
					assert.Equal(t, "GET", req.Method)
					assert.Contains(t, req.URL.String(), "/api/v1/webhooks")
					assert.NotContains(t, req.URL.String(), "existing-webhook-123") // Should not contain webhook ID
					return mockHTTPResponse(http.StatusOK, string(listResponseJSON)), nil
				}),
			mockHTTPClient.EXPECT().
				Do(gomock.Any()).
				DoAndReturn(func(req *http.Request) (*http.Response, error) {
					// Second call should be update webhook (this tests directUpdateWebhook)
					assert.Equal(t, "PUT", req.Method)
					assert.Contains(t, req.URL.String(), "/api/v1/webhooks/existing-webhook-123")

					// Verify headers for update request
					assert.Equal(t, "Bearer test-api-key", req.Header.Get("Authorization"))
					assert.Equal(t, "application/json", req.Header.Get("Content-Type"))
					assert.Equal(t, "application/json", req.Header.Get("Accept"))

					// Verify request body contains updated events
					body, err := io.ReadAll(req.Body)
					assert.NoError(t, err)

					var requestWebhook domain.SparkPostWebhook
					err = json.Unmarshal(body, &requestWebhook)
					assert.NoError(t, err)
					assert.Contains(t, requestWebhook.Events, "delivery")
					assert.Contains(t, requestWebhook.Events, "bounce")

					return mockHTTPResponse(http.StatusOK, string(updateResponseJSON)), nil
				}),
		)

		// Call RegisterWebhooks which should trigger directUpdateWebhook
		result, err := sparkPostService.RegisterWebhooks(ctx, workspaceID, integrationID, baseURL, eventTypes, providerConfig)

		// Verify results
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.True(t, result.IsRegistered)
		assert.Equal(t, domain.EmailProviderKindSparkPost, result.EmailProviderKind)
		assert.Len(t, result.Endpoints, 2) // Should have endpoints for both event types
	})

	t.Run("Update Webhook HTTP Error", func(t *testing.T) {
		ctx := context.Background()

		// Mock list webhooks response - webhook already exists
		existingWebhook := domain.SparkPostWebhook{
			ID:     "existing-webhook-123",
			Name:   "Existing Webhook",
			Target: fmt.Sprintf("%s/webhooks/email/sparkpost?workspace_id=%s&integration_id=%s", baseURL, workspaceID, integrationID),
			Events: []string{"delivery"},
			Active: true,
		}

		listResponse := domain.SparkPostWebhookListResponse{
			Results: []domain.SparkPostWebhook{existingWebhook},
		}
		listResponseJSON, _ := json.Marshal(listResponse)

		// Expect list call to succeed, but update call to fail
		gomock.InOrder(
			mockHTTPClient.EXPECT().
				Do(gomock.Any()).
				Return(mockHTTPResponse(http.StatusOK, string(listResponseJSON)), nil),
			mockHTTPClient.EXPECT().
				Do(gomock.Any()).
				Return(nil, errors.New("network error")),
		)

		// Call RegisterWebhooks which should trigger directUpdateWebhook and fail
		result, err := sparkPostService.RegisterWebhooks(ctx, workspaceID, integrationID, baseURL, eventTypes, providerConfig)

		// Verify error
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "failed to update SparkPost webhook")
	})

	t.Run("Update Webhook Non-OK Status", func(t *testing.T) {
		ctx := context.Background()

		// Mock list webhooks response
		existingWebhook := domain.SparkPostWebhook{
			ID:     "existing-webhook-123",
			Name:   "Existing Webhook",
			Target: fmt.Sprintf("%s/webhooks/email/sparkpost?workspace_id=%s&integration_id=%s", baseURL, workspaceID, integrationID),
			Events: []string{"delivery"},
			Active: true,
		}

		listResponse := domain.SparkPostWebhookListResponse{
			Results: []domain.SparkPostWebhook{existingWebhook},
		}
		listResponseJSON, _ := json.Marshal(listResponse)

		// Expect list call to succeed, but update call to return error status
		gomock.InOrder(
			mockHTTPClient.EXPECT().
				Do(gomock.Any()).
				Return(mockHTTPResponse(http.StatusOK, string(listResponseJSON)), nil),
			mockHTTPClient.EXPECT().
				Do(gomock.Any()).
				Return(mockHTTPResponse(http.StatusBadRequest, `{"errors":[{"message":"Invalid webhook"}]}`), nil),
		)

		// Call RegisterWebhooks
		result, err := sparkPostService.RegisterWebhooks(ctx, workspaceID, integrationID, baseURL, eventTypes, providerConfig)

		// Verify error
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "failed to update SparkPost webhook")
	})
}

// Additional tests for better coverage
func TestSparkPostService_CreateWebhook_ErrorCases(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHTTPClient := mocks.NewMockHTTPClient(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()

	sparkPostService := service.NewSparkPostService(mockHTTPClient, mockAuthService, mockLogger)

	config := domain.SparkPostSettings{
		Endpoint: "https://api.sparkpost.test",
		APIKey:   "test-api-key",
	}

	webhook := domain.SparkPostWebhook{
		Name:     "Test Webhook",
		Target:   "https://example.com/webhook",
		Events:   []string{"delivery", "bounce"},
		Active:   true,
		AuthType: "none",
	}

	t.Run("Marshal Error", func(t *testing.T) {
		// This test is tricky since json.Marshal rarely fails with normal structs
		// We'll test with HTTP error instead
		ctx := context.Background()

		mockHTTPClient.EXPECT().
			Do(gomock.Any()).
			Return(nil, errors.New("connection timeout"))

		result, err := sparkPostService.CreateWebhook(ctx, config, webhook)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "failed to execute request")
	})

	t.Run("Status Created Success", func(t *testing.T) {
		ctx := context.Background()

		webhookResponse := domain.SparkPostWebhookResponse{
			Results: domain.SparkPostWebhook{
				ID:     "webhook-123",
				Name:   webhook.Name,
				Target: webhook.Target,
				Events: webhook.Events,
				Active: webhook.Active,
			},
		}
		responseJSON, _ := json.Marshal(webhookResponse)

		mockHTTPClient.EXPECT().
			Do(gomock.Any()).
			Return(mockHTTPResponse(http.StatusCreated, string(responseJSON)), nil)

		result, err := sparkPostService.CreateWebhook(ctx, config, webhook)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, "webhook-123", result.Results.ID)
	})
}

func TestSparkPostService_directListWebhooks_ErrorCases(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHTTPClient := mocks.NewMockHTTPClient(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()

	sparkPostService := service.NewSparkPostService(mockHTTPClient, mockAuthService, mockLogger)

	settings := &domain.SparkPostSettings{
		Endpoint: "https://api.sparkpost.test",
		APIKey:   "test-api-key",
	}

	t.Run("HTTP Error", func(t *testing.T) {
		ctx := context.Background()

		mockHTTPClient.EXPECT().
			Do(gomock.Any()).
			Return(nil, errors.New("network error"))

		// Use RegisterWebhooks to trigger directListWebhooks
		providerConfig := &domain.EmailProvider{SparkPost: settings}
		result, err := sparkPostService.RegisterWebhooks(ctx, "workspace", "integration", "https://example.com", []domain.EmailEventType{domain.EmailEventDelivered}, providerConfig)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "failed to list SparkPost webhooks")
	})

	t.Run("Non-OK Status", func(t *testing.T) {
		ctx := context.Background()

		mockHTTPClient.EXPECT().
			Do(gomock.Any()).
			Return(mockHTTPResponse(http.StatusUnauthorized, `{"errors":[{"message":"Unauthorized"}]}`), nil)

		providerConfig := &domain.EmailProvider{SparkPost: settings}
		result, err := sparkPostService.RegisterWebhooks(ctx, "workspace", "integration", "https://example.com", []domain.EmailEventType{domain.EmailEventDelivered}, providerConfig)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "failed to list SparkPost webhooks")
	})

	t.Run("Invalid JSON Response", func(t *testing.T) {
		ctx := context.Background()

		mockHTTPClient.EXPECT().
			Do(gomock.Any()).
			Return(mockHTTPResponse(http.StatusOK, `invalid json`), nil)

		providerConfig := &domain.EmailProvider{SparkPost: settings}
		result, err := sparkPostService.RegisterWebhooks(ctx, "workspace", "integration", "https://example.com", []domain.EmailEventType{domain.EmailEventDelivered}, providerConfig)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "failed to list SparkPost webhooks")
	})
}

func TestSparkPostService_directCreateWebhook_ErrorCases(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHTTPClient := mocks.NewMockHTTPClient(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()

	sparkPostService := service.NewSparkPostService(mockHTTPClient, mockAuthService, mockLogger)

	settings := &domain.SparkPostSettings{
		Endpoint: "https://api.sparkpost.test",
		APIKey:   "test-api-key",
	}

	t.Run("HTTP Error", func(t *testing.T) {
		ctx := context.Background()

		// Mock list webhooks to return empty result (to trigger create)
		listResponse := domain.SparkPostWebhookListResponse{Results: []domain.SparkPostWebhook{}}
		listResponseJSON, _ := json.Marshal(listResponse)

		// Then mock create to fail
		gomock.InOrder(
			mockHTTPClient.EXPECT().
				Do(gomock.Any()).
				Return(mockHTTPResponse(http.StatusOK, string(listResponseJSON)), nil),
			mockHTTPClient.EXPECT().
				Do(gomock.Any()).
				Return(nil, errors.New("connection error")),
		)

		providerConfig := &domain.EmailProvider{SparkPost: settings}
		result, err := sparkPostService.RegisterWebhooks(ctx, "workspace", "integration", "https://example.com", []domain.EmailEventType{domain.EmailEventDelivered}, providerConfig)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "failed to create SparkPost webhook")
	})

	t.Run("Non-OK Status", func(t *testing.T) {
		ctx := context.Background()

		// Mock list webhooks to return empty result
		listResponse := domain.SparkPostWebhookListResponse{Results: []domain.SparkPostWebhook{}}
		listResponseJSON, _ := json.Marshal(listResponse)

		// Then mock create to return error status
		gomock.InOrder(
			mockHTTPClient.EXPECT().
				Do(gomock.Any()).
				Return(mockHTTPResponse(http.StatusOK, string(listResponseJSON)), nil),
			mockHTTPClient.EXPECT().
				Do(gomock.Any()).
				Return(mockHTTPResponse(http.StatusBadRequest, `{"errors":[{"message":"Invalid webhook"}]}`), nil),
		)

		providerConfig := &domain.EmailProvider{SparkPost: settings}
		result, err := sparkPostService.RegisterWebhooks(ctx, "workspace", "integration", "https://example.com", []domain.EmailEventType{domain.EmailEventDelivered}, providerConfig)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "failed to create SparkPost webhook")
	})

	t.Run("Invalid JSON Response", func(t *testing.T) {
		ctx := context.Background()

		// Mock list webhooks to return empty result
		listResponse := domain.SparkPostWebhookListResponse{Results: []domain.SparkPostWebhook{}}
		listResponseJSON, _ := json.Marshal(listResponse)

		// Then mock create to return invalid JSON
		gomock.InOrder(
			mockHTTPClient.EXPECT().
				Do(gomock.Any()).
				Return(mockHTTPResponse(http.StatusOK, string(listResponseJSON)), nil),
			mockHTTPClient.EXPECT().
				Do(gomock.Any()).
				Return(mockHTTPResponse(http.StatusOK, `invalid json`), nil),
		)

		providerConfig := &domain.EmailProvider{SparkPost: settings}
		result, err := sparkPostService.RegisterWebhooks(ctx, "workspace", "integration", "https://example.com", []domain.EmailEventType{domain.EmailEventDelivered}, providerConfig)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "failed to create SparkPost webhook")
	})
}

func TestSparkPostService_TestWebhook_ErrorCases(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHTTPClient := mocks.NewMockHTTPClient(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()

	sparkPostService := service.NewSparkPostService(mockHTTPClient, mockAuthService, mockLogger)

	config := domain.SparkPostSettings{
		Endpoint: "https://api.sparkpost.test",
		APIKey:   "test-api-key",
	}
	webhookID := "webhook-123"

	t.Run("HTTP Error", func(t *testing.T) {
		ctx := context.Background()

		mockHTTPClient.EXPECT().
			Do(gomock.Any()).
			Return(nil, errors.New("network error"))

		err := sparkPostService.TestWebhook(ctx, config, webhookID)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to execute request")
	})

	t.Run("Non-OK Status", func(t *testing.T) {
		ctx := context.Background()

		mockHTTPClient.EXPECT().
			Do(gomock.Any()).
			Return(mockHTTPResponse(http.StatusBadRequest, `{"errors":[{"message":"Invalid webhook"}]}`), nil)

		err := sparkPostService.TestWebhook(ctx, config, webhookID)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "API returned non-OK status code 400")
	})
}

func TestSparkPostService_directDeleteWebhook_ErrorCases(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHTTPClient := mocks.NewMockHTTPClient(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()

	sparkPostService := service.NewSparkPostService(mockHTTPClient, mockAuthService, mockLogger)

	settings := &domain.SparkPostSettings{
		Endpoint: "https://api.sparkpost.test",
		APIKey:   "test-api-key",
	}

	t.Run("HTTP Error", func(t *testing.T) {
		ctx := context.Background()

		// Mock list webhooks to return a webhook that matches our criteria
		webhook := domain.SparkPostWebhook{
			ID:     "webhook-123",
			Target: "https://example.com/webhook?workspace_id=workspace&integration_id=integration",
		}
		listResponse := domain.SparkPostWebhookListResponse{Results: []domain.SparkPostWebhook{webhook}}
		listResponseJSON, _ := json.Marshal(listResponse)

		// Then mock delete to fail
		gomock.InOrder(
			mockHTTPClient.EXPECT().
				Do(gomock.Any()).
				Return(mockHTTPResponse(http.StatusOK, string(listResponseJSON)), nil),
			mockHTTPClient.EXPECT().
				Do(gomock.Any()).
				Return(nil, errors.New("connection error")),
		)

		providerConfig := &domain.EmailProvider{SparkPost: settings}
		err := sparkPostService.UnregisterWebhooks(ctx, "workspace", "integration", providerConfig)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to delete one or more SparkPost webhooks")
	})

	t.Run("Non-OK Status", func(t *testing.T) {
		ctx := context.Background()

		// Mock list webhooks to return a webhook that matches our criteria
		webhook := domain.SparkPostWebhook{
			ID:     "webhook-123",
			Target: "https://example.com/webhook?workspace_id=workspace&integration_id=integration",
		}
		listResponse := domain.SparkPostWebhookListResponse{Results: []domain.SparkPostWebhook{webhook}}
		listResponseJSON, _ := json.Marshal(listResponse)

		// Then mock delete to return error status
		gomock.InOrder(
			mockHTTPClient.EXPECT().
				Do(gomock.Any()).
				Return(mockHTTPResponse(http.StatusOK, string(listResponseJSON)), nil),
			mockHTTPClient.EXPECT().
				Do(gomock.Any()).
				Return(mockHTTPResponse(http.StatusBadRequest, `{"errors":[{"message":"Cannot delete webhook"}]}`), nil),
		)

		providerConfig := &domain.EmailProvider{SparkPost: settings}
		err := sparkPostService.UnregisterWebhooks(ctx, "workspace", "integration", providerConfig)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to delete one or more SparkPost webhooks")
	})

	t.Run("Success with NoContent Status", func(t *testing.T) {
		ctx := context.Background()

		// Mock list webhooks to return a webhook that matches our criteria
		webhook := domain.SparkPostWebhook{
			ID:     "webhook-123",
			Target: "https://example.com/webhook?workspace_id=workspace&integration_id=integration",
		}
		listResponse := domain.SparkPostWebhookListResponse{Results: []domain.SparkPostWebhook{webhook}}
		listResponseJSON, _ := json.Marshal(listResponse)

		// Then mock delete to return NoContent status (which should succeed)
		gomock.InOrder(
			mockHTTPClient.EXPECT().
				Do(gomock.Any()).
				Return(mockHTTPResponse(http.StatusOK, string(listResponseJSON)), nil),
			mockHTTPClient.EXPECT().
				Do(gomock.Any()).
				Return(mockHTTPResponse(http.StatusNoContent, ""), nil),
		)

		providerConfig := &domain.EmailProvider{SparkPost: settings}
		err := sparkPostService.UnregisterWebhooks(ctx, "workspace", "integration", providerConfig)

		assert.NoError(t, err)
	})
}

func TestSparkPostService_ValidateWebhook_ErrorCases(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHTTPClient := mocks.NewMockHTTPClient(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()

	sparkPostService := service.NewSparkPostService(mockHTTPClient, mockAuthService, mockLogger)

	config := domain.SparkPostSettings{
		Endpoint: "https://api.sparkpost.test",
		APIKey:   "test-api-key",
	}
	webhook := domain.SparkPostWebhook{
		Target: "https://example.com/webhook",
	}

	t.Run("HTTP Error", func(t *testing.T) {
		ctx := context.Background()

		mockHTTPClient.EXPECT().
			Do(gomock.Any()).
			Return(nil, errors.New("network error"))

		isValid, err := sparkPostService.ValidateWebhook(ctx, config, webhook)

		assert.Error(t, err)
		assert.False(t, isValid)
		assert.Contains(t, err.Error(), "failed to execute request")
	})
}

func TestSparkPostService_SendEmail_AdditionalCases(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHTTPClient := mocks.NewMockHTTPClient(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()

	sparkPostService := service.NewSparkPostService(mockHTTPClient, mockAuthService, mockLogger)

	t.Run("With CC and BCC", func(t *testing.T) {
		ctx := context.Background()

		provider := &domain.EmailProvider{
			SparkPost: &domain.SparkPostSettings{
				Endpoint: "https://api.sparkpost.test",
				APIKey:   "test-api-key",
			},
		}

		mockHTTPClient.EXPECT().
			Do(gomock.Any()).
			DoAndReturn(func(req *http.Request) (*http.Response, error) {
				// Verify CC and BCC recipients are included
				body, _ := io.ReadAll(req.Body)
				var emailReq map[string]interface{}
				_ = json.Unmarshal(body, &emailReq)

				recipients := emailReq["recipients"].([]interface{})
				assert.Len(t, recipients, 3) // to + cc + bcc

				return mockHTTPResponse(http.StatusOK, `{"results":{"id":"test-id"}}`), nil
			})

		request := domain.SendEmailProviderRequest{
			WorkspaceID:   "workspace-123",
			IntegrationID: "integration-123",
			MessageID:     "message-123",
			FromAddress:   "sender@example.com",
			FromName:      "Test Sender",
			To:            "recipient@example.com",
			Subject:       "Test Subject",
			Content:       "<p>Test Content</p>",
			Provider:      provider,
			EmailOptions: domain.EmailOptions{
				CC:      []string{"cc@example.com"},
				BCC:     []string{"bcc@example.com"},
				ReplyTo: "reply@example.com",
			},
		}
		err := sparkPostService.SendEmail(ctx, request)

		assert.NoError(t, err)
	})

	t.Run("Default Endpoint", func(t *testing.T) {
		ctx := context.Background()

		provider := &domain.EmailProvider{
			SparkPost: &domain.SparkPostSettings{
				// No endpoint specified - should use default
				APIKey: "test-api-key",
			},
		}

		mockHTTPClient.EXPECT().
			Do(gomock.Any()).
			DoAndReturn(func(req *http.Request) (*http.Response, error) {
				// Verify default endpoint is used
				assert.Equal(t, "https://api.sparkpost.com/api/v1/transmissions", req.URL.String())
				return mockHTTPResponse(http.StatusOK, `{"results":{"id":"test-id"}}`), nil
			})

		request := domain.SendEmailProviderRequest{
			WorkspaceID:   "workspace-123",
			IntegrationID: "integration-123",
			MessageID:     "message-123",
			FromAddress:   "sender@example.com",
			FromName:      "Test Sender",
			To:            "recipient@example.com",
			Subject:       "Test Subject",
			Content:       "<p>Test Content</p>",
			Provider:      provider,
			EmailOptions:  domain.EmailOptions{},
		}
		err := sparkPostService.SendEmail(ctx, request)

		assert.NoError(t, err)
	})

	t.Run("Request Validation Error", func(t *testing.T) {
		ctx := context.Background()

		provider := &domain.EmailProvider{
			SparkPost: &domain.SparkPostSettings{
				Endpoint: "https://api.sparkpost.test",
				APIKey:   "test-api-key",
			},
		}

		// Create invalid request (missing required fields)
		request := domain.SendEmailProviderRequest{
			// Missing required fields to trigger validation error
			Provider: provider,
		}
		err := sparkPostService.SendEmail(ctx, request)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid request")
	})

	t.Run("Empty CC and BCC", func(t *testing.T) {
		ctx := context.Background()

		provider := &domain.EmailProvider{
			SparkPost: &domain.SparkPostSettings{
				Endpoint: "https://api.sparkpost.test",
				APIKey:   "test-api-key",
			},
		}

		mockHTTPClient.EXPECT().
			Do(gomock.Any()).
			DoAndReturn(func(req *http.Request) (*http.Response, error) {
				// Verify empty CC/BCC are handled correctly
				body, _ := io.ReadAll(req.Body)
				var emailReq map[string]interface{}
				_ = json.Unmarshal(body, &emailReq)

				recipients := emailReq["recipients"].([]interface{})
				assert.Len(t, recipients, 1) // Only the main recipient

				return mockHTTPResponse(http.StatusOK, `{"results":{"id":"test-id"}}`), nil
			})

		request := domain.SendEmailProviderRequest{
			WorkspaceID:   "workspace-123",
			IntegrationID: "integration-123",
			MessageID:     "message-123",
			FromAddress:   "sender@example.com",
			FromName:      "Test Sender",
			To:            "recipient@example.com",
			Subject:       "Test Subject",
			Content:       "<p>Test Content</p>",
			Provider:      provider,
			EmailOptions: domain.EmailOptions{
				CC:  []string{""}, // Empty CC
				BCC: []string{""}, // Empty BCC
			},
		}
		err := sparkPostService.SendEmail(ctx, request)

		assert.NoError(t, err)
	})

	t.Run("With Single Attachment", func(t *testing.T) {
		ctx := context.Background()

		provider := &domain.EmailProvider{
			SparkPost: &domain.SparkPostSettings{
				Endpoint: "https://api.sparkpost.test",
				APIKey:   "test-api-key",
			},
		}

		// Create a simple PDF attachment (base64 encoded "test content")
		attachmentContent := "dGVzdCBjb250ZW50" // "test content" in base64

		mockHTTPClient.EXPECT().
			Do(gomock.Any()).
			DoAndReturn(func(req *http.Request) (*http.Response, error) {
				// Verify attachment is included
				body, _ := io.ReadAll(req.Body)
				var emailReq map[string]interface{}
				_ = json.Unmarshal(body, &emailReq)

				content := emailReq["content"].(map[string]interface{})
				attachments := content["attachments"].([]interface{})
				assert.Len(t, attachments, 1)

				att := attachments[0].(map[string]interface{})
				assert.Equal(t, "invoice.pdf", att["name"])
				assert.Equal(t, "application/pdf", att["type"])
				assert.Equal(t, attachmentContent, att["data"])

				return mockHTTPResponse(http.StatusOK, `{"results":{"id":"test-id"}}`), nil
			})

		request := domain.SendEmailProviderRequest{
			WorkspaceID:   "workspace-123",
			IntegrationID: "integration-123",
			MessageID:     "message-123",
			FromAddress:   "sender@example.com",
			FromName:      "Test Sender",
			To:            "recipient@example.com",
			Subject:       "Test Subject",
			Content:       "<p>Test Content</p>",
			Provider:      provider,
			EmailOptions: domain.EmailOptions{
				Attachments: []domain.Attachment{
					{
						Filename:    "invoice.pdf",
						Content:     attachmentContent,
						ContentType: "application/pdf",
						Disposition: "attachment",
					},
				},
			},
		}
		err := sparkPostService.SendEmail(ctx, request)

		assert.NoError(t, err)
	})

	t.Run("With Multiple Attachments", func(t *testing.T) {
		ctx := context.Background()

		provider := &domain.EmailProvider{
			SparkPost: &domain.SparkPostSettings{
				Endpoint: "https://api.sparkpost.test",
				APIKey:   "test-api-key",
			},
		}

		mockHTTPClient.EXPECT().
			Do(gomock.Any()).
			DoAndReturn(func(req *http.Request) (*http.Response, error) {
				// Verify multiple attachments are included
				body, _ := io.ReadAll(req.Body)
				var emailReq map[string]interface{}
				_ = json.Unmarshal(body, &emailReq)

				content := emailReq["content"].(map[string]interface{})
				attachments := content["attachments"].([]interface{})
				assert.Len(t, attachments, 3)

				att1 := attachments[0].(map[string]interface{})
				assert.Equal(t, "invoice.pdf", att1["name"])
				assert.Equal(t, "application/pdf", att1["type"])

				att2 := attachments[1].(map[string]interface{})
				assert.Equal(t, "report.docx", att2["name"])
				assert.Equal(t, "application/vnd.openxmlformats-officedocument.wordprocessingml.document", att2["type"])

				att3 := attachments[2].(map[string]interface{})
				assert.Equal(t, "data.csv", att3["name"])
				assert.Equal(t, "text/csv", att3["type"])

				return mockHTTPResponse(http.StatusOK, `{"results":{"id":"test-id"}}`), nil
			})

		request := domain.SendEmailProviderRequest{
			WorkspaceID:   "workspace-123",
			IntegrationID: "integration-123",
			MessageID:     "message-123",
			FromAddress:   "sender@example.com",
			FromName:      "Test Sender",
			To:            "recipient@example.com",
			Subject:       "Test Subject",
			Content:       "<p>Test Content</p>",
			Provider:      provider,
			EmailOptions: domain.EmailOptions{
				Attachments: []domain.Attachment{
					{
						Filename:    "invoice.pdf",
						Content:     "dGVzdCBjb250ZW50", // "test content" in base64
						ContentType: "application/pdf",
						Disposition: "attachment",
					},
					{
						Filename:    "report.docx",
						Content:     "ZG9jdW1lbnQgY29udGVudA==", // "document content" in base64
						ContentType: "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
						Disposition: "attachment",
					},
					{
						Filename:    "data.csv",
						Content:     "Y3N2IGRhdGE=", // "csv data" in base64
						ContentType: "text/csv",
						Disposition: "attachment",
					},
				},
			},
		}
		err := sparkPostService.SendEmail(ctx, request)

		assert.NoError(t, err)
	})

	t.Run("With Inline Image", func(t *testing.T) {
		ctx := context.Background()

		provider := &domain.EmailProvider{
			SparkPost: &domain.SparkPostSettings{
				Endpoint: "https://api.sparkpost.test",
				APIKey:   "test-api-key",
			},
		}

		imageContent := "aW1hZ2UgZGF0YQ==" // "image data" in base64

		mockHTTPClient.EXPECT().
			Do(gomock.Any()).
			DoAndReturn(func(req *http.Request) (*http.Response, error) {
				// Verify inline image is in inline_images array
				body, _ := io.ReadAll(req.Body)
				var emailReq map[string]interface{}
				_ = json.Unmarshal(body, &emailReq)

				content := emailReq["content"].(map[string]interface{})

				// Should have no regular attachments
				attachments, hasAttachments := content["attachments"]
				if hasAttachments {
					assert.Len(t, attachments, 0)
				}

				// Should have inline image
				inlineImages := content["inline_images"].([]interface{})
				assert.Len(t, inlineImages, 1)

				img := inlineImages[0].(map[string]interface{})
				assert.Equal(t, "logo.png", img["name"])
				assert.Equal(t, "image/png", img["type"])
				assert.Equal(t, imageContent, img["data"])

				return mockHTTPResponse(http.StatusOK, `{"results":{"id":"test-id"}}`), nil
			})

		request := domain.SendEmailProviderRequest{
			WorkspaceID:   "workspace-123",
			IntegrationID: "integration-123",
			MessageID:     "message-123",
			FromAddress:   "sender@example.com",
			FromName:      "Test Sender",
			To:            "recipient@example.com",
			Subject:       "Test Subject",
			Content:       "<p>Test Content</p>",
			Provider:      provider,
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
		err := sparkPostService.SendEmail(ctx, request)

		assert.NoError(t, err)
	})

	t.Run("With Mixed Attachments and Inline Images", func(t *testing.T) {
		ctx := context.Background()

		provider := &domain.EmailProvider{
			SparkPost: &domain.SparkPostSettings{
				Endpoint: "https://api.sparkpost.test",
				APIKey:   "test-api-key",
			},
		}

		mockHTTPClient.EXPECT().
			Do(gomock.Any()).
			DoAndReturn(func(req *http.Request) (*http.Response, error) {
				// Verify both attachments and inline images
				body, _ := io.ReadAll(req.Body)
				var emailReq map[string]interface{}
				_ = json.Unmarshal(body, &emailReq)

				content := emailReq["content"].(map[string]interface{})

				// Should have 2 regular attachments
				attachments := content["attachments"].([]interface{})
				assert.Len(t, attachments, 2)

				// Should have 1 inline image
				inlineImages := content["inline_images"].([]interface{})
				assert.Len(t, inlineImages, 1)

				return mockHTTPResponse(http.StatusOK, `{"results":{"id":"test-id"}}`), nil
			})

		request := domain.SendEmailProviderRequest{
			WorkspaceID:   "workspace-123",
			IntegrationID: "integration-123",
			MessageID:     "message-123",
			FromAddress:   "sender@example.com",
			FromName:      "Test Sender",
			To:            "recipient@example.com",
			Subject:       "Test Subject",
			Content:       "<p>Test Content</p>",
			Provider:      provider,
			EmailOptions: domain.EmailOptions{
				Attachments: []domain.Attachment{
					{
						Filename:    "invoice.pdf",
						Content:     "dGVzdCBjb250ZW50",
						ContentType: "application/pdf",
						Disposition: "attachment",
					},
					{
						Filename:    "logo.png",
						Content:     "aW1hZ2UgZGF0YQ==",
						ContentType: "image/png",
						Disposition: "inline",
					},
					{
						Filename:    "report.docx",
						Content:     "ZG9jdW1lbnQ=",
						ContentType: "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
						Disposition: "attachment",
					},
				},
			},
		}
		err := sparkPostService.SendEmail(ctx, request)

		assert.NoError(t, err)
	})

	t.Run("Attachment with Missing ContentType", func(t *testing.T) {
		ctx := context.Background()

		provider := &domain.EmailProvider{
			SparkPost: &domain.SparkPostSettings{
				Endpoint: "https://api.sparkpost.test",
				APIKey:   "test-api-key",
			},
		}

		mockHTTPClient.EXPECT().
			Do(gomock.Any()).
			DoAndReturn(func(req *http.Request) (*http.Response, error) {
				// Verify default content type is applied
				body, _ := io.ReadAll(req.Body)
				var emailReq map[string]interface{}
				_ = json.Unmarshal(body, &emailReq)

				content := emailReq["content"].(map[string]interface{})
				attachments := content["attachments"].([]interface{})
				assert.Len(t, attachments, 1)

				att := attachments[0].(map[string]interface{})
				assert.Equal(t, "application/octet-stream", att["type"])

				return mockHTTPResponse(http.StatusOK, `{"results":{"id":"test-id"}}`), nil
			})

		request := domain.SendEmailProviderRequest{
			WorkspaceID:   "workspace-123",
			IntegrationID: "integration-123",
			MessageID:     "message-123",
			FromAddress:   "sender@example.com",
			FromName:      "Test Sender",
			To:            "recipient@example.com",
			Subject:       "Test Subject",
			Content:       "<p>Test Content</p>",
			Provider:      provider,
			EmailOptions: domain.EmailOptions{
				Attachments: []domain.Attachment{
					{
						Filename:    "unknown.bin",
						Content:     "dGVzdCBjb250ZW50",
						ContentType: "", // Empty content type
						Disposition: "attachment",
					},
				},
			},
		}
		err := sparkPostService.SendEmail(ctx, request)

		assert.NoError(t, err)
	})

	t.Run("Invalid Base64 Attachment Content", func(t *testing.T) {
		ctx := context.Background()

		provider := &domain.EmailProvider{
			SparkPost: &domain.SparkPostSettings{
				Endpoint: "https://api.sparkpost.test",
				APIKey:   "test-api-key",
			},
		}

		request := domain.SendEmailProviderRequest{
			WorkspaceID:   "workspace-123",
			IntegrationID: "integration-123",
			MessageID:     "message-123",
			FromAddress:   "sender@example.com",
			FromName:      "Test Sender",
			To:            "recipient@example.com",
			Subject:       "Test Subject",
			Content:       "<p>Test Content</p>",
			Provider:      provider,
			EmailOptions: domain.EmailOptions{
				Attachments: []domain.Attachment{
					{
						Filename:    "invalid.pdf",
						Content:     "not-valid-base64!!!",
						ContentType: "application/pdf",
						Disposition: "attachment",
					},
				},
			},
		}
		err := sparkPostService.SendEmail(ctx, request)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to decode content")
	})

	t.Run("Empty Attachments Array", func(t *testing.T) {
		ctx := context.Background()

		provider := &domain.EmailProvider{
			SparkPost: &domain.SparkPostSettings{
				Endpoint: "https://api.sparkpost.test",
				APIKey:   "test-api-key",
			},
		}

		mockHTTPClient.EXPECT().
			Do(gomock.Any()).
			DoAndReturn(func(req *http.Request) (*http.Response, error) {
				// Verify no attachments or inline images in request
				body, _ := io.ReadAll(req.Body)
				var emailReq map[string]interface{}
				_ = json.Unmarshal(body, &emailReq)

				content := emailReq["content"].(map[string]interface{})

				// Attachments and inline_images should not be present or empty
				attachments, hasAttachments := content["attachments"]
				if hasAttachments {
					assert.Empty(t, attachments)
				}

				inlineImages, hasInlineImages := content["inline_images"]
				if hasInlineImages {
					assert.Empty(t, inlineImages)
				}

				return mockHTTPResponse(http.StatusOK, `{"results":{"id":"test-id"}}`), nil
			})

		request := domain.SendEmailProviderRequest{
			WorkspaceID:   "workspace-123",
			IntegrationID: "integration-123",
			MessageID:     "message-123",
			FromAddress:   "sender@example.com",
			FromName:      "Test Sender",
			To:            "recipient@example.com",
			Subject:       "Test Subject",
			Content:       "<p>Test Content</p>",
			Provider:      provider,
			EmailOptions: domain.EmailOptions{
				Attachments: []domain.Attachment{}, // Empty array
			},
		}
		err := sparkPostService.SendEmail(ctx, request)

		assert.NoError(t, err)
	})

	t.Run("with RFC-8058 List-Unsubscribe headers", func(t *testing.T) {
		ctx := context.Background()

		provider := &domain.EmailProvider{
			SparkPost: &domain.SparkPostSettings{
				Endpoint: "https://api.sparkpost.test",
				APIKey:   "test-api-key",
			},
		}

		mockHTTPClient.EXPECT().
			Do(gomock.Any()).
			DoAndReturn(func(req *http.Request) (*http.Response, error) {
				// Verify RFC-8058 List-Unsubscribe headers in request
				body, _ := io.ReadAll(req.Body)
				var emailReq map[string]interface{}
				_ = json.Unmarshal(body, &emailReq)

				content := emailReq["content"].(map[string]interface{})
				headers, ok := content["headers"].(map[string]interface{})
				assert.True(t, ok, "headers should be present")

				listUnsubscribe, ok := headers["List-Unsubscribe"].(string)
				assert.True(t, ok, "List-Unsubscribe header should be present")
				assert.Equal(t, "<https://example.com/unsubscribe/abc123>", listUnsubscribe)

				listUnsubscribePost, ok := headers["List-Unsubscribe-Post"].(string)
				assert.True(t, ok, "List-Unsubscribe-Post header should be present")
				assert.Equal(t, "List-Unsubscribe=One-Click", listUnsubscribePost)

				return mockHTTPResponse(http.StatusOK, `{"results":{"id":"test-id"}}`), nil
			})

		request := domain.SendEmailProviderRequest{
			WorkspaceID:   "workspace-123",
			IntegrationID: "integration-123",
			MessageID:     "message-123",
			FromAddress:   "sender@example.com",
			FromName:      "Test Sender",
			To:            "recipient@example.com",
			Subject:       "Test Subject",
			Content:       "<p>Test Content</p>",
			Provider:      provider,
			EmailOptions: domain.EmailOptions{
				ListUnsubscribeURL: "https://example.com/unsubscribe/abc123",
			},
		}
		err := sparkPostService.SendEmail(ctx, request)

		assert.NoError(t, err)
	})

	t.Run("with RFC-8058 List-Unsubscribe headers and attachments", func(t *testing.T) {
		ctx := context.Background()

		provider := &domain.EmailProvider{
			SparkPost: &domain.SparkPostSettings{
				Endpoint: "https://api.sparkpost.test",
				APIKey:   "test-api-key",
			},
		}

		mockHTTPClient.EXPECT().
			Do(gomock.Any()).
			DoAndReturn(func(req *http.Request) (*http.Response, error) {
				// Verify RFC-8058 List-Unsubscribe headers in request
				body, _ := io.ReadAll(req.Body)
				var emailReq map[string]interface{}
				_ = json.Unmarshal(body, &emailReq)

				content := emailReq["content"].(map[string]interface{})

				// Verify headers
				headers, ok := content["headers"].(map[string]interface{})
				assert.True(t, ok, "headers should be present")
				assert.Contains(t, headers, "List-Unsubscribe")
				assert.Contains(t, headers, "List-Unsubscribe-Post")

				// Verify attachment
				attachments, ok := content["attachments"].([]interface{})
				assert.True(t, ok, "attachments should be present")
				assert.Len(t, attachments, 1)

				return mockHTTPResponse(http.StatusOK, `{"results":{"id":"test-id"}}`), nil
			})

		request := domain.SendEmailProviderRequest{
			WorkspaceID:   "workspace-123",
			IntegrationID: "integration-123",
			MessageID:     "message-123",
			FromAddress:   "sender@example.com",
			FromName:      "Test Sender",
			To:            "recipient@example.com",
			Subject:       "Test Subject",
			Content:       "<p>Test Content</p>",
			Provider:      provider,
			EmailOptions: domain.EmailOptions{
				Attachments: []domain.Attachment{
					{
						Filename:    "test.txt",
						Content:     "SGVsbG8gV29ybGQ=", // base64 of "Hello World"
						ContentType: "text/plain",
						Disposition: "attachment",
					},
				},
				ListUnsubscribeURL: "https://example.com/unsubscribe/xyz789",
			},
		}
		err := sparkPostService.SendEmail(ctx, request)

		assert.NoError(t, err)
	})
}

// Test additional edge cases for better coverage
func TestSparkPostService_EdgeCases(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHTTPClient := mocks.NewMockHTTPClient(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()

	sparkPostService := service.NewSparkPostService(mockHTTPClient, mockAuthService, mockLogger)

	t.Run("RegisterWebhooks with EmailEventComplaint", func(t *testing.T) {
		ctx := context.Background()

		providerConfig := &domain.EmailProvider{
			SparkPost: &domain.SparkPostSettings{
				Endpoint: "https://api.sparkpost.test",
				APIKey:   "test-api-key",
			},
		}

		// Mock empty list to trigger create
		listResponse := domain.SparkPostWebhookListResponse{Results: []domain.SparkPostWebhook{}}
		listResponseJSON, _ := json.Marshal(listResponse)

		// Mock create response with spam_complaint event
		createResponse := domain.SparkPostWebhookResponse{
			Results: domain.SparkPostWebhook{
				ID:     "webhook-123",
				Name:   "Test Webhook",
				Events: []string{"spam_complaint"},
				Active: true,
			},
		}
		createResponseJSON, _ := json.Marshal(createResponse)

		gomock.InOrder(
			mockHTTPClient.EXPECT().
				Do(gomock.Any()).
				Return(mockHTTPResponse(http.StatusOK, string(listResponseJSON)), nil),
			mockHTTPClient.EXPECT().
				Do(gomock.Any()).
				DoAndReturn(func(req *http.Request) (*http.Response, error) {
					// Verify spam_complaint event is mapped correctly
					body, _ := io.ReadAll(req.Body)
					var webhook domain.SparkPostWebhook
					_ = json.Unmarshal(body, &webhook)
					assert.Contains(t, webhook.Events, "spam_complaint")
					return mockHTTPResponse(http.StatusOK, string(createResponseJSON)), nil
				}),
		)

		result, err := sparkPostService.RegisterWebhooks(ctx, "workspace", "integration", "https://example.com", []domain.EmailEventType{domain.EmailEventComplaint}, providerConfig)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.True(t, result.IsRegistered)
	})

	t.Run("RegisterWebhooks with long integration ID", func(t *testing.T) {
		ctx := context.Background()

		providerConfig := &domain.EmailProvider{
			SparkPost: &domain.SparkPostSettings{
				Endpoint: "https://api.sparkpost.test",
				APIKey:   "test-api-key",
			},
		}

		// Mock empty list to trigger create
		listResponse := domain.SparkPostWebhookListResponse{Results: []domain.SparkPostWebhook{}}
		listResponseJSON, _ := json.Marshal(listResponse)

		// Mock create response
		createResponse := domain.SparkPostWebhookResponse{
			Results: domain.SparkPostWebhook{
				ID:     "webhook-123",
				Name:   "Test Webhook",
				Events: []string{"delivery"},
				Active: true,
			},
		}
		createResponseJSON, _ := json.Marshal(createResponse)

		gomock.InOrder(
			mockHTTPClient.EXPECT().
				Do(gomock.Any()).
				Return(mockHTTPResponse(http.StatusOK, string(listResponseJSON)), nil),
			mockHTTPClient.EXPECT().
				Do(gomock.Any()).
				DoAndReturn(func(req *http.Request) (*http.Response, error) {
					// Verify webhook name uses full integration ID (no truncation)
					body, _ := io.ReadAll(req.Body)
					var webhook domain.SparkPostWebhook
					_ = json.Unmarshal(body, &webhook)
					assert.Equal(t, "Notifuse-very-long-integration-id-that-exceeds-limit", webhook.Name)
					return mockHTTPResponse(http.StatusOK, string(createResponseJSON)), nil
				}),
		)

		longIntegrationID := "very-long-integration-id-that-exceeds-limit"
		result, err := sparkPostService.RegisterWebhooks(ctx, "workspace", longIntegrationID, "https://example.com", []domain.EmailEventType{domain.EmailEventDelivered}, providerConfig)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.True(t, result.IsRegistered)
	})

	t.Run("RegisterWebhooks with short integration ID", func(t *testing.T) {
		ctx := context.Background()

		providerConfig := &domain.EmailProvider{
			SparkPost: &domain.SparkPostSettings{
				Endpoint: "https://api.sparkpost.test",
				APIKey:   "test-api-key",
			},
		}

		// Mock empty list to trigger create
		listResponse := domain.SparkPostWebhookListResponse{Results: []domain.SparkPostWebhook{}}
		listResponseJSON, _ := json.Marshal(listResponse)

		// Mock create response
		createResponse := domain.SparkPostWebhookResponse{
			Results: domain.SparkPostWebhook{
				ID:     "webhook-123",
				Name:   "Test Webhook",
				Events: []string{"delivery"},
				Active: true,
			},
		}
		createResponseJSON, _ := json.Marshal(createResponse)

		gomock.InOrder(
			mockHTTPClient.EXPECT().
				Do(gomock.Any()).
				Return(mockHTTPResponse(http.StatusOK, string(listResponseJSON)), nil),
			mockHTTPClient.EXPECT().
				Do(gomock.Any()).
				DoAndReturn(func(req *http.Request) (*http.Response, error) {
					// Verify webhook name uses full short integration ID
					body, _ := io.ReadAll(req.Body)
					var webhook domain.SparkPostWebhook
					_ = json.Unmarshal(body, &webhook)
					assert.Equal(t, "Notifuse-integration", webhook.Name)
					return mockHTTPResponse(http.StatusOK, string(createResponseJSON)), nil
				}),
		)

		shortIntegrationID := "integration"
		result, err := sparkPostService.RegisterWebhooks(ctx, "workspace", shortIntegrationID, "https://example.com", []domain.EmailEventType{domain.EmailEventDelivered}, providerConfig)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.True(t, result.IsRegistered)
	})

	t.Run("GetWebhookStatus list error", func(t *testing.T) {
		ctx := context.Background()

		providerConfig := &domain.EmailProvider{
			SparkPost: &domain.SparkPostSettings{
				Endpoint: "https://api.sparkpost.test",
				APIKey:   "test-api-key",
			},
		}

		mockHTTPClient.EXPECT().
			Do(gomock.Any()).
			Return(nil, errors.New("network error"))

		result, err := sparkPostService.GetWebhookStatus(ctx, "workspace", "integration", providerConfig)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "failed to list SparkPost webhooks")
	})

	t.Run("UnregisterWebhooks list error", func(t *testing.T) {
		ctx := context.Background()

		providerConfig := &domain.EmailProvider{
			SparkPost: &domain.SparkPostSettings{
				Endpoint: "https://api.sparkpost.test",
				APIKey:   "test-api-key",
			},
		}

		mockHTTPClient.EXPECT().
			Do(gomock.Any()).
			Return(nil, errors.New("network error"))

		err := sparkPostService.UnregisterWebhooks(ctx, "workspace", "integration", providerConfig)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to list SparkPost webhooks")
	})
}

// Test missing configuration scenarios
func TestSparkPostService_MissingConfig(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHTTPClient := mocks.NewMockHTTPClient(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()

	sparkPostService := service.NewSparkPostService(mockHTTPClient, mockAuthService, mockLogger)

	ctx := context.Background()

	t.Run("GetWebhookStatus missing config", func(t *testing.T) {
		result, err := sparkPostService.GetWebhookStatus(ctx, "workspace", "integration", nil)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "configuration is missing or invalid")
	})

	t.Run("GetWebhookStatus missing SparkPost config", func(t *testing.T) {
		providerConfig := &domain.EmailProvider{}

		result, err := sparkPostService.GetWebhookStatus(ctx, "workspace", "integration", providerConfig)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "configuration is missing or invalid")
	})

	t.Run("GetWebhookStatus missing API key", func(t *testing.T) {
		providerConfig := &domain.EmailProvider{
			SparkPost: &domain.SparkPostSettings{
				Endpoint: "https://api.sparkpost.test",
				// Missing API key
			},
		}

		result, err := sparkPostService.GetWebhookStatus(ctx, "workspace", "integration", providerConfig)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "configuration is missing or invalid")
	})

	t.Run("UnregisterWebhooks missing config", func(t *testing.T) {
		err := sparkPostService.UnregisterWebhooks(ctx, "workspace", "integration", nil)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "configuration is missing or invalid")
	})

	t.Run("UnregisterWebhooks missing SparkPost config", func(t *testing.T) {
		providerConfig := &domain.EmailProvider{}

		err := sparkPostService.UnregisterWebhooks(ctx, "workspace", "integration", providerConfig)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "configuration is missing or invalid")
	})

	t.Run("UnregisterWebhooks missing API key", func(t *testing.T) {
		providerConfig := &domain.EmailProvider{
			SparkPost: &domain.SparkPostSettings{
				Endpoint: "https://api.sparkpost.test",
				// Missing API key
			},
		}

		err := sparkPostService.UnregisterWebhooks(ctx, "workspace", "integration", providerConfig)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "configuration is missing or invalid")
	})
}
