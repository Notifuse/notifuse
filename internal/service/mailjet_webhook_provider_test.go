package service

import (
	"bytes"
	"context"
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

// mockMailjetResponse creates an HTTP response for mailjet tests
func mockMailjetResponse(t *testing.T, statusCode int, body interface{}) *http.Response {
	var responseBody io.ReadCloser
	if body != nil {
		jsonData, err := json.Marshal(body)
		require.NoError(t, err)
		responseBody = io.NopCloser(strings.NewReader(string(jsonData)))
	} else {
		responseBody = io.NopCloser(strings.NewReader(""))
	}

	return &http.Response{
		StatusCode: statusCode,
		Body:       responseBody,
	}
}

// TestMailjetService_TestWebhook tests the TestWebhook method
func TestMailjetService_TestWebhook(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mocks
	mockHTTPClient := mocks.NewMockHTTPClient(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	// Set up logger expectations
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

	// Create service with mocks
	service := NewMailjetService(mockHTTPClient, mockAuthService, mockLogger)

	// Test data
	ctx := context.Background()
	config := domain.MailjetSettings{
		APIKey:    "test-api-key",
		SecretKey: "test-secret-key",
	}
	webhookID := "123"
	eventType := "sent"

	// Call the method - need to implement TestWebhook in mailjet_service.go if it doesn't exist
	err := service.TestWebhook(ctx, config, webhookID, eventType)

	// Assertion - this should return an error since Mailjet doesn't support testing webhooks
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not supported")
}

func TestMailjetService_RegisterWebhooksProvider(t *testing.T) {
	// Test data
	ctx := context.Background()
	workspaceID := "workspace-123"
	integrationID := "integration-456"
	baseURL := "https://api.notifuse.com"
	eventTypes := []domain.EmailEventType{
		domain.EmailEventDelivered,
		domain.EmailEventBounce,
		domain.EmailEventComplaint,
	}

	t.Run("successful registration", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		// Create mocks
		mockHTTPClient := mocks.NewMockHTTPClient(ctrl)
		mockAuthService := mocks.NewMockAuthService(ctrl)
		mockLogger := pkgmocks.NewMockLogger(ctrl)

		// Set up logger expectations
		mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
		mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
		mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()

		// Create service with mocks
		service := NewMailjetService(mockHTTPClient, mockAuthService, mockLogger)
		// Create email provider config
		providerConfig := &domain.EmailProvider{
			Kind: domain.EmailProviderKindMailjet,
			Mailjet: &domain.MailjetSettings{
				APIKey:    "test-api-key",
				SecretKey: "test-secret-key",
			},
		}

		// Expected webhook URL
		expectedWebhookURL := domain.GenerateWebhookCallbackURL(baseURL, domain.EmailProviderKindMailjet, workspaceID, integrationID)

		// Response for ListWebhooks - empty list
		emptyResponse := domain.MailjetWebhookResponse{
			Count: 0,
			Data:  []domain.MailjetWebhook{},
			Total: 0,
		}

		// Setup mock to handle all HTTP requests
		mockHTTPClient.EXPECT().
			Do(gomock.Any()).
			DoAndReturn(func(req *http.Request) (*http.Response, error) {
				// Handle ListWebhooks (GET)
				if req.Method == "GET" && strings.Contains(req.URL.String(), "eventcallbackurl") {
					return mockMailjetResponse(t, http.StatusOK, emptyResponse), nil
				}
				// Handle CreateWebhook (POST) - return different webhook based on request body
				if req.Method == "POST" && strings.Contains(req.URL.String(), "eventcallbackurl") {
					// Parse the request body to determine event type
					body, _ := io.ReadAll(req.Body)
					req.Body = io.NopCloser(bytes.NewReader(body)) // Reset body for potential re-reading

					var webhookReq domain.MailjetWebhook
					_ = json.Unmarshal(body, &webhookReq)

					// Return a webhook with the same event type as requested
					responseWebhook := domain.MailjetWebhook{
						ID:        1001 + int64(len(webhookReq.EventType)), // Different ID for each webhook
						EventType: webhookReq.EventType,
						Endpoint:  expectedWebhookURL,
						Status:    "alive",
					}
					return mockMailjetResponse(t, http.StatusCreated, responseWebhook), nil
				}
				// Handle DeleteWebhook (DELETE) - just return success
				if req.Method == "DELETE" && strings.Contains(req.URL.String(), "eventcallbackurl") {
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(strings.NewReader("{}")),
						Header:     make(http.Header),
					}, nil
				}
				return nil, errors.New("unexpected request")
			}).AnyTimes()

		// Call the service method
		status, err := service.RegisterWebhooks(ctx, workspaceID, integrationID, baseURL, eventTypes, providerConfig)

		// Assertions
		require.NoError(t, err)
		require.NotNil(t, status)
		assert.Equal(t, domain.EmailProviderKindMailjet, status.EmailProviderKind)
		assert.True(t, status.IsRegistered)
		assert.NotEmpty(t, status.Endpoints)
		assert.Equal(t, workspaceID, status.ProviderDetails["workspace_id"])
		assert.Equal(t, integrationID, status.ProviderDetails["integration_id"])

		// Check for registered event types
		var eventTypesCovered = make(map[domain.EmailEventType]bool)
		for _, endpoint := range status.Endpoints {
			eventTypesCovered[endpoint.EventType] = true
		}

		assert.True(t, eventTypesCovered[domain.EmailEventDelivered], "Should have an endpoint for delivered events")
	})

	t.Run("missing configuration", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		// Create mocks
		mockHTTPClient := mocks.NewMockHTTPClient(ctrl)
		mockAuthService := mocks.NewMockAuthService(ctrl)
		mockLogger := pkgmocks.NewMockLogger(ctrl)

		// Set up logger expectations
		mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
		mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
		mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()

		// Create service with mocks
		service := NewMailjetService(mockHTTPClient, mockAuthService, mockLogger)

		// Call with nil provider config
		status, err := service.RegisterWebhooks(ctx, workspaceID, integrationID, baseURL, eventTypes, nil)

		// Assertions
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "mailjet configuration is missing or invalid")
		assert.Nil(t, status)

		// Call with empty Mailjet config
		emptyConfig := &domain.EmailProvider{
			Kind:    domain.EmailProviderKindMailjet,
			Mailjet: &domain.MailjetSettings{},
		}

		status, err = service.RegisterWebhooks(ctx, workspaceID, integrationID, baseURL, eventTypes, emptyConfig)

		// Assertions
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "mailjet configuration is missing or invalid")
		assert.Nil(t, status)
	})

	t.Run("list webhooks error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		// Create mocks
		mockHTTPClient := mocks.NewMockHTTPClient(ctrl)
		mockAuthService := mocks.NewMockAuthService(ctrl)
		mockLogger := pkgmocks.NewMockLogger(ctrl)

		// Set up logger expectations
		mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
		mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
		mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()

		// Create service with mocks
		service := NewMailjetService(mockHTTPClient, mockAuthService, mockLogger)

		providerConfig := &domain.EmailProvider{
			Kind: domain.EmailProviderKindMailjet,
			Mailjet: &domain.MailjetSettings{
				APIKey:    "test-api-key",
				SecretKey: "test-secret-key",
			},
		}

		// Setup mock for ListWebhooks to return error
		mockHTTPClient.EXPECT().
			Do(gomock.Any()).
			Return(nil, errors.New("network error")).AnyTimes()

		// Call the service method
		status, err := service.RegisterWebhooks(ctx, workspaceID, integrationID, baseURL, eventTypes, providerConfig)

		// Assertions
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to list Mailjet webhooks")
		assert.Nil(t, status)
	})

	t.Run("create webhook error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		// Create mocks
		mockHTTPClient := mocks.NewMockHTTPClient(ctrl)
		mockAuthService := mocks.NewMockAuthService(ctrl)
		mockLogger := pkgmocks.NewMockLogger(ctrl)

		// Set up logger expectations
		mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
		mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
		mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()

		// Create service with mocks
		service := NewMailjetService(mockHTTPClient, mockAuthService, mockLogger)

		providerConfig := &domain.EmailProvider{
			Kind: domain.EmailProviderKindMailjet,
			Mailjet: &domain.MailjetSettings{
				APIKey:    "test-api-key",
				SecretKey: "test-secret-key",
			},
		}

		// Response for ListWebhooks - empty list
		emptyResponse := domain.MailjetWebhookResponse{
			Count: 0,
			Data:  []domain.MailjetWebhook{},
			Total: 0,
		}

		// Setup mock for ListWebhooks
		mockHTTPClient.EXPECT().
			Do(gomock.Any()).
			DoAndReturn(func(req *http.Request) (*http.Response, error) {
				if req.Method == "GET" && strings.Contains(req.URL.String(), "eventcallbackurl") {
					return mockMailjetResponse(t, http.StatusOK, emptyResponse), nil
				}
				return nil, errors.New("unexpected request")
			}).AnyTimes()

		// Setup mock for CreateWebhook to return error
		mockHTTPClient.EXPECT().
			Do(gomock.Any()).
			DoAndReturn(func(req *http.Request) (*http.Response, error) {
				if req.Method == "POST" && strings.Contains(req.URL.String(), "eventcallbackurl") {
					return mockMailjetResponse(t, http.StatusBadRequest, nil), nil
				}
				return nil, errors.New("unexpected request")
			}).AnyTimes()

		// Call the service method
		status, err := service.RegisterWebhooks(ctx, workspaceID, integrationID, baseURL, eventTypes, providerConfig)

		// Assertions
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create Mailjet webhook")
		assert.Nil(t, status)
	})
}

func TestMailjetService_GetWebhookStatusProvider(t *testing.T) {
	// Test data
	ctx := context.Background()
	workspaceID := "workspace-123"
	integrationID := "integration-456"

	t.Run("successful status check with webhooks", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		// Create mocks
		mockHTTPClient := mocks.NewMockHTTPClient(ctrl)
		mockAuthService := mocks.NewMockAuthService(ctrl)
		mockLogger := pkgmocks.NewMockLogger(ctrl)

		// Set up logger expectations
		mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
		mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

		// Create service with mocks
		service := NewMailjetService(mockHTTPClient, mockAuthService, mockLogger)
		// Create email provider config
		providerConfig := &domain.EmailProvider{
			Kind: domain.EmailProviderKindMailjet,
			Mailjet: &domain.MailjetSettings{
				APIKey:    "test-api-key",
				SecretKey: "test-secret-key",
			},
		}

		// Generate webhook URL
		webhookURL := domain.GenerateWebhookCallbackURL("https://api.notifuse.com", domain.EmailProviderKindMailjet, workspaceID, integrationID)

		// Response for ListWebhooks with registered webhooks
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
					EventType: string(domain.MailjetEventSpam),
					Endpoint:  webhookURL,
					Status:    "alive",
				},
			},
			Total: 3,
		}

		// Setup mock for ListWebhooks
		mockHTTPClient.EXPECT().
			Do(gomock.Any()).
			DoAndReturn(func(req *http.Request) (*http.Response, error) {
				if req.Method == "GET" && strings.Contains(req.URL.String(), "eventcallbackurl") {
					return mockMailjetResponse(t, http.StatusOK, webhooksResponse), nil
				}
				return nil, errors.New("unexpected request")
			}).AnyTimes()

		// Call the service method
		status, err := service.GetWebhookStatus(ctx, workspaceID, integrationID, providerConfig)

		// Assertions
		require.NoError(t, err)
		require.NotNil(t, status)
		assert.Equal(t, domain.EmailProviderKindMailjet, status.EmailProviderKind)
		assert.True(t, status.IsRegistered)
		assert.NotEmpty(t, status.Endpoints)

		// Verify webhooks are properly mapped to event types
		hasEventTypes := make(map[domain.EmailEventType]bool)
		for _, endpoint := range status.Endpoints {
			hasEventTypes[endpoint.EventType] = true
		}

		assert.True(t, hasEventTypes[domain.EmailEventDelivered])
	})

	t.Run("no webhooks registered", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		// Create mocks
		mockHTTPClient := mocks.NewMockHTTPClient(ctrl)
		mockAuthService := mocks.NewMockAuthService(ctrl)
		mockLogger := pkgmocks.NewMockLogger(ctrl)

		// Set up logger expectations
		mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
		mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

		// Create service with mocks
		service := NewMailjetService(mockHTTPClient, mockAuthService, mockLogger)

		// Create email provider config
		providerConfig := &domain.EmailProvider{
			Kind: domain.EmailProviderKindMailjet,
			Mailjet: &domain.MailjetSettings{
				APIKey:    "test-api-key",
				SecretKey: "test-secret-key",
			},
		}

		// Response for ListWebhooks with no registered webhooks
		emptyResponse := domain.MailjetWebhookResponse{
			Count: 0,
			Data:  []domain.MailjetWebhook{},
			Total: 0,
		}

		// Setup mock for ListWebhooks
		mockHTTPClient.EXPECT().
			Do(gomock.Any()).
			DoAndReturn(func(req *http.Request) (*http.Response, error) {
				if req.Method == "GET" && strings.Contains(req.URL.String(), "eventcallbackurl") {
					return mockMailjetResponse(t, http.StatusOK, emptyResponse), nil
				}
				return nil, errors.New("unexpected request")
			}).AnyTimes()

		// Call the service method
		status, err := service.GetWebhookStatus(ctx, workspaceID, integrationID, providerConfig)

		// Assertions
		require.NoError(t, err)
		require.NotNil(t, status)
		assert.Equal(t, domain.EmailProviderKindMailjet, status.EmailProviderKind)
		assert.False(t, status.IsRegistered)
		assert.Empty(t, status.Endpoints)
	})

	t.Run("missing configuration", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		// Create mocks
		mockHTTPClient := mocks.NewMockHTTPClient(ctrl)
		mockAuthService := mocks.NewMockAuthService(ctrl)
		mockLogger := pkgmocks.NewMockLogger(ctrl)

		// Set up logger expectations
		mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
		mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

		// Create service with mocks
		service := NewMailjetService(mockHTTPClient, mockAuthService, mockLogger)

		// Call with nil provider config
		status, err := service.GetWebhookStatus(ctx, workspaceID, integrationID, nil)

		// Assertions
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "mailjet configuration is missing or invalid")
		assert.Nil(t, status)

		// Call with empty Mailjet config
		emptyConfig := &domain.EmailProvider{
			Kind:    domain.EmailProviderKindMailjet,
			Mailjet: &domain.MailjetSettings{},
		}

		status, err = service.GetWebhookStatus(ctx, workspaceID, integrationID, emptyConfig)

		// Assertions
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "mailjet configuration is missing or invalid")
		assert.Nil(t, status)
	})

	t.Run("list webhooks error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		// Create mocks
		mockHTTPClient := mocks.NewMockHTTPClient(ctrl)
		mockAuthService := mocks.NewMockAuthService(ctrl)
		mockLogger := pkgmocks.NewMockLogger(ctrl)

		// Set up logger expectations
		mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
		mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

		// Create service with mocks
		service := NewMailjetService(mockHTTPClient, mockAuthService, mockLogger)

		providerConfig := &domain.EmailProvider{
			Kind: domain.EmailProviderKindMailjet,
			Mailjet: &domain.MailjetSettings{
				APIKey:    "test-api-key",
				SecretKey: "test-secret-key",
			},
		}

		// Setup mock for ListWebhooks to return error
		mockHTTPClient.EXPECT().
			Do(gomock.Any()).
			Return(nil, errors.New("network error")).AnyTimes()

		// Call the service method
		status, err := service.GetWebhookStatus(ctx, workspaceID, integrationID, providerConfig)

		// Assertions
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to list Mailjet webhooks")
		assert.Nil(t, status)
	})
}

func TestMailjetService_UnregisterWebhooksProvider(t *testing.T) {
	// Test data
	ctx := context.Background()
	workspaceID := "workspace-123"
	integrationID := "integration-456"

	t.Run("successful unregistration", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		// Create mocks
		mockHTTPClient := mocks.NewMockHTTPClient(ctrl)
		mockAuthService := mocks.NewMockAuthService(ctrl)
		mockLogger := pkgmocks.NewMockLogger(ctrl)

		// Set up logger expectations
		mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
		mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
		mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()

		// Create service with mocks
		service := NewMailjetService(mockHTTPClient, mockAuthService, mockLogger)
		// Create email provider config
		providerConfig := &domain.EmailProvider{
			Kind: domain.EmailProviderKindMailjet,
			Mailjet: &domain.MailjetSettings{
				APIKey:    "test-api-key",
				SecretKey: "test-secret-key",
			},
		}

		// Generate webhook URL
		webhookURL := domain.GenerateWebhookCallbackURL("https://api.notifuse.com", domain.EmailProviderKindMailjet, workspaceID, integrationID)

		// Response for ListWebhooks with registered webhooks
		webhooksResponse := domain.MailjetWebhookResponse{
			Count: 2,
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
			},
			Total: 2,
		}

		// Setup mock to handle all HTTP requests
		mockHTTPClient.EXPECT().
			Do(gomock.Any()).
			DoAndReturn(func(req *http.Request) (*http.Response, error) {
				// Handle ListWebhooks (GET)
				if req.Method == "GET" && strings.Contains(req.URL.String(), "eventcallbackurl") {
					return mockMailjetResponse(t, http.StatusOK, webhooksResponse), nil
				}
				// Handle DeleteWebhook (DELETE) - accept any webhook ID
				if req.Method == "DELETE" && strings.Contains(req.URL.String(), "eventcallbackurl") {
					return &http.Response{
						StatusCode: http.StatusNoContent,
						Body:       io.NopCloser(strings.NewReader("")),
						Header:     make(http.Header),
					}, nil
				}
				return nil, errors.New("unexpected request")
			}).AnyTimes()

		// Call the service method
		err := service.UnregisterWebhooks(ctx, workspaceID, integrationID, providerConfig)

		// Assertions
		require.NoError(t, err)
	})

	t.Run("no webhooks to unregister", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		// Create mocks
		mockHTTPClient := mocks.NewMockHTTPClient(ctrl)
		mockAuthService := mocks.NewMockAuthService(ctrl)
		mockLogger := pkgmocks.NewMockLogger(ctrl)

		// Set up logger expectations
		mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
		mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
		mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()

		// Create service with mocks
		service := NewMailjetService(mockHTTPClient, mockAuthService, mockLogger)

		// Create email provider config
		providerConfig := &domain.EmailProvider{
			Kind: domain.EmailProviderKindMailjet,
			Mailjet: &domain.MailjetSettings{
				APIKey:    "test-api-key",
				SecretKey: "test-secret-key",
			},
		}

		// Response for ListWebhooks with no registered webhooks
		emptyResponse := domain.MailjetWebhookResponse{
			Count: 0,
			Data:  []domain.MailjetWebhook{},
			Total: 0,
		}

		// Setup mock for ListWebhooks
		mockHTTPClient.EXPECT().
			Do(gomock.Any()).
			DoAndReturn(func(req *http.Request) (*http.Response, error) {
				if req.Method == "GET" && strings.Contains(req.URL.String(), "eventcallbackurl") {
					return mockMailjetResponse(t, http.StatusOK, emptyResponse), nil
				}
				return nil, errors.New("unexpected request")
			}).AnyTimes()

		// Call the service method
		err := service.UnregisterWebhooks(ctx, workspaceID, integrationID, providerConfig)

		// Assertions - no error when there are no webhooks to delete
		require.NoError(t, err)
	})

	t.Run("missing configuration", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		// Create mocks
		mockHTTPClient := mocks.NewMockHTTPClient(ctrl)
		mockAuthService := mocks.NewMockAuthService(ctrl)
		mockLogger := pkgmocks.NewMockLogger(ctrl)

		// Set up logger expectations
		mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
		mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
		mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()

		// Create service with mocks
		service := NewMailjetService(mockHTTPClient, mockAuthService, mockLogger)

		// Call with nil provider config
		err := service.UnregisterWebhooks(ctx, workspaceID, integrationID, nil)

		// Assertions
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "mailjet configuration is missing or invalid")

		// Call with empty Mailjet config
		emptyConfig := &domain.EmailProvider{
			Kind:    domain.EmailProviderKindMailjet,
			Mailjet: &domain.MailjetSettings{},
		}

		err = service.UnregisterWebhooks(ctx, workspaceID, integrationID, emptyConfig)

		// Assertions
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "mailjet configuration is missing or invalid")
	})

	t.Run("list webhooks error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		// Create mocks
		mockHTTPClient := mocks.NewMockHTTPClient(ctrl)
		mockAuthService := mocks.NewMockAuthService(ctrl)
		mockLogger := pkgmocks.NewMockLogger(ctrl)

		// Set up logger expectations
		mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
		mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
		mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()

		// Create service with mocks
		service := NewMailjetService(mockHTTPClient, mockAuthService, mockLogger)

		providerConfig := &domain.EmailProvider{
			Kind: domain.EmailProviderKindMailjet,
			Mailjet: &domain.MailjetSettings{
				APIKey:    "test-api-key",
				SecretKey: "test-secret-key",
			},
		}

		// Setup mock for ListWebhooks to return error
		mockHTTPClient.EXPECT().
			Do(gomock.Any()).
			Return(nil, errors.New("network error")).AnyTimes()

		// Call the service method
		err := service.UnregisterWebhooks(ctx, workspaceID, integrationID, providerConfig)

		// Assertions
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to list Mailjet webhooks")
	})

	t.Run("delete webhook error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		// Create mocks
		mockHTTPClient := mocks.NewMockHTTPClient(ctrl)
		mockAuthService := mocks.NewMockAuthService(ctrl)
		mockLogger := pkgmocks.NewMockLogger(ctrl)

		// Set up logger expectations
		mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
		mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
		mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()

		// Create service with mocks
		service := NewMailjetService(mockHTTPClient, mockAuthService, mockLogger)

		// Create email provider config
		providerConfig := &domain.EmailProvider{
			Kind: domain.EmailProviderKindMailjet,
			Mailjet: &domain.MailjetSettings{
				APIKey:    "test-api-key",
				SecretKey: "test-secret-key",
			},
		}

		// Generate webhook URL
		webhookURL := domain.GenerateWebhookCallbackURL("https://api.notifuse.com", domain.EmailProviderKindMailjet, workspaceID, integrationID)

		// Response for ListWebhooks with registered webhooks
		webhooksResponse := domain.MailjetWebhookResponse{
			Count: 1,
			Data: []domain.MailjetWebhook{
				{
					ID:        101,
					EventType: string(domain.MailjetEventSent),
					Endpoint:  webhookURL,
					Status:    "alive",
				},
			},
			Total: 1,
		}

		// Setup mock to handle all HTTP requests
		mockHTTPClient.EXPECT().
			Do(gomock.Any()).
			DoAndReturn(func(req *http.Request) (*http.Response, error) {
				// Handle ListWebhooks (GET)
				if req.Method == "GET" && strings.Contains(req.URL.String(), "eventcallbackurl") {
					return mockMailjetResponse(t, http.StatusOK, webhooksResponse), nil
				}
				// Handle DeleteWebhook (DELETE) - return error
				if req.Method == "DELETE" && strings.Contains(req.URL.String(), "eventcallbackurl") {
					return nil, errors.New("network error")
				}
				return nil, errors.New("unexpected request")
			}).AnyTimes()

		// Call the service method
		err := service.UnregisterWebhooks(ctx, workspaceID, integrationID, providerConfig)

		// Assertions
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to delete one or more Mailjet webhooks")
	})
}
