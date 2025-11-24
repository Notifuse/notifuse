package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
)

func TestMailgunService_ListWebhooks(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHTTPClient := mocks.NewMockHTTPClient(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	webhookEndpoint := "https://webhook.example.com"
	service := NewMailgunService(mockHTTPClient, mockAuthService, mockLogger, webhookEndpoint)

	ctx := context.Background()
	config := domain.MailgunSettings{
		Domain: "example.com",
		APIKey: "test-api-key",
		Region: "US",
	}

	t.Run("successful response", func(t *testing.T) {
		// Mock HTTP response
		responseBody := `{
			"webhooks": {
				"delivered": {
					"urls": ["https://webhook.example.com/mailgun/delivered"]
				},
				"permanent_fail": {
					"urls": ["https://webhook.example.com/mailgun/failed", "https://other-domain.com/webhook"]
				},
				"temporary_fail": {
					"urls": []
				},
				"complained": {
					"urls": []
				}
			}
		}`

		resp := &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(responseBody)),
		}

		// Set expectation for HTTP request
		mockHTTPClient.EXPECT().
			Do(gomock.Any()).
			DoAndReturn(func(req *http.Request) (*http.Response, error) {
				// Verify request
				assert.Equal(t, "GET", req.Method)
				assert.Equal(t, "https://api.mailgun.net/v3/domains/example.com/webhooks", req.URL.String())
				// Check for Basic auth header instead of raw header
				username, password, ok := req.BasicAuth()
				assert.True(t, ok, "Basic auth header should be set")
				assert.Equal(t, "api", username)
				assert.Equal(t, "test-api-key", password)

				return resp, nil
			})

		// Call the service
		result, err := service.ListWebhooks(ctx, config)

		// Verify results
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Len(t, result.Webhooks.Delivered.URLs, 1)
		assert.Equal(t, "https://webhook.example.com/mailgun/delivered", result.Webhooks.Delivered.URLs[0])
		assert.Len(t, result.Webhooks.PermanentFail.URLs, 1) // Filtered out the non-matching URL
		assert.Equal(t, "https://webhook.example.com/mailgun/failed", result.Webhooks.PermanentFail.URLs[0])
		assert.Empty(t, result.Webhooks.TemporaryFail.URLs)
		assert.Empty(t, result.Webhooks.Complained.URLs)
	})

	t.Run("EU region", func(t *testing.T) {
		// Use EU region config
		euConfig := domain.MailgunSettings{
			Domain: "example.com",
			APIKey: "test-api-key",
			Region: "EU",
		}

		// Mock HTTP response
		resp := &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"webhooks": {"delivered": {"urls": []}, "permanent_fail": {"urls": []}, "temporary_fail": {"urls": []}, "complained": {"urls": []}}}`)),
		}

		// Set expectation for HTTP request with EU endpoint
		mockHTTPClient.EXPECT().
			Do(gomock.Any()).
			DoAndReturn(func(req *http.Request) (*http.Response, error) {
				// Verify request uses EU endpoint
				assert.Equal(t, "https://api.eu.mailgun.net/v3/domains/example.com/webhooks", req.URL.String())
				return resp, nil
			})

		// Call the service
		result, err := service.ListWebhooks(ctx, euConfig)

		// Verify results
		require.NoError(t, err)
		require.NotNil(t, result)
	})

	t.Run("HTTP request error", func(t *testing.T) {
		// Set expectation for HTTP error
		mockHTTPClient.EXPECT().
			Do(gomock.Any()).
			Return(nil, errors.New("connection error"))

		// Allow any logger calls since we don't test logging
		mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

		// Call the service
		result, err := service.ListWebhooks(ctx, config)

		// Verify error handling
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "failed to execute request")
	})

	t.Run("non-200 response", func(t *testing.T) {
		// Mock error response
		resp := &http.Response{
			StatusCode: http.StatusUnauthorized,
			Body:       io.NopCloser(strings.NewReader(`{"error": "Unauthorized"}`)),
		}

		// Set expectation
		mockHTTPClient.EXPECT().
			Do(gomock.Any()).
			Return(resp, nil)

		// Allow any logger calls since we don't test logging
		mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

		// Call the service
		result, err := service.ListWebhooks(ctx, config)

		// Verify error handling
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "API returned non-OK status code 401")
	})

	t.Run("invalid JSON response", func(t *testing.T) {
		// Mock invalid JSON
		resp := &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{invalid json}`)),
		}

		// Set expectation
		mockHTTPClient.EXPECT().
			Do(gomock.Any()).
			Return(resp, nil)

		// Allow any logger calls since we don't test logging
		mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

		// Call the service
		result, err := service.ListWebhooks(ctx, config)

		// Verify error handling
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "failed to decode response")
	})
}

func TestMailgunService_CreateWebhook(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHTTPClient := mocks.NewMockHTTPClient(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	webhookEndpoint := "https://webhook.example.com"
	service := NewMailgunService(mockHTTPClient, mockAuthService, mockLogger, webhookEndpoint)

	ctx := context.Background()
	config := domain.MailgunSettings{
		Domain: "example.com",
		APIKey: "test-api-key",
		Region: "US",
	}

	webhook := domain.MailgunWebhook{
		URL:    "https://webhook.example.com/mailgun/delivered",
		Events: []string{"delivered"},
		Active: true,
	}

	t.Run("successful webhook creation", func(t *testing.T) {
		// Mock successful response
		responseBody := `{
			"message": "Webhook has been created",
			"webhook": {
				"id": "delivered",
				"url": "https://webhook.example.com/mailgun/delivered"
			}
		}`

		resp := &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(responseBody)),
		}

		// Set expectation for HTTP request
		mockHTTPClient.EXPECT().
			Do(gomock.Any()).
			DoAndReturn(func(req *http.Request) (*http.Response, error) {
				// Verify request
				assert.Equal(t, "POST", req.Method)
				assert.Equal(t, "https://api.mailgun.net/v3/domains/example.com/webhooks", req.URL.String())

				// Ensure body contains form data
				body, _ := io.ReadAll(req.Body)
				assert.Contains(t, string(body), "id=delivered")
				assert.Contains(t, string(body), "url=https%3A%2F%2Fwebhook.example.com%2Fmailgun%2Fdelivered")

				return resp, nil
			})

		// Call the service
		result, err := service.CreateWebhook(ctx, config, webhook)

		// Verify results
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, "delivered", result.ID)
		assert.Equal(t, "https://webhook.example.com/mailgun/delivered", result.URL)
		assert.Equal(t, []string{"delivered"}, result.Events)
		assert.True(t, result.Active)
	})

	t.Run("empty events list", func(t *testing.T) {
		// Try to create webhook with no events
		emptyWebhook := domain.MailgunWebhook{
			URL:    "https://webhook.example.com/mailgun/delivered",
			Events: []string{},
			Active: true,
		}

		// Call the service
		result, err := service.CreateWebhook(ctx, config, emptyWebhook)

		// Verify error is returned
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "at least one event type is required")
	})

	t.Run("HTTP request error", func(t *testing.T) {
		// Set expectation for HTTP error
		mockHTTPClient.EXPECT().
			Do(gomock.Any()).
			Return(nil, errors.New("connection error"))

		// Allow any logger calls since we don't test logging
		mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

		// Call the service
		result, err := service.CreateWebhook(ctx, config, webhook)

		// Verify error handling
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "failed to execute request")
	})

	t.Run("non-200 response", func(t *testing.T) {
		// Mock error response
		resp := &http.Response{
			StatusCode: http.StatusBadRequest,
			Body:       io.NopCloser(strings.NewReader(`{"error": "Bad Request"}`)),
		}

		// Set expectation
		mockHTTPClient.EXPECT().
			Do(gomock.Any()).
			Return(resp, nil)

		// Allow any logger calls since we don't test logging
		mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

		// Call the service
		result, err := service.CreateWebhook(ctx, config, webhook)

		// Verify error handling
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "API returned non-OK status code 400")
	})
}

func TestMailgunService_DeleteWebhook(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHTTPClient := mocks.NewMockHTTPClient(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	webhookEndpoint := "https://webhook.example.com"
	service := NewMailgunService(mockHTTPClient, mockAuthService, mockLogger, webhookEndpoint)

	ctx := context.Background()
	config := domain.MailgunSettings{
		Domain: "example.com",
		APIKey: "test-api-key",
		Region: "US",
	}
	webhookID := "delivered"

	t.Run("successful webhook deletion", func(t *testing.T) {
		// Mock successful response
		responseBody := `{
			"message": "Webhook has been deleted",
			"id": "delivered"
		}`

		resp := &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(responseBody)),
		}

		// Set expectation for HTTP request
		mockHTTPClient.EXPECT().
			Do(gomock.Any()).
			DoAndReturn(func(req *http.Request) (*http.Response, error) {
				// Verify request
				assert.Equal(t, "DELETE", req.Method)
				assert.Equal(t, "https://api.mailgun.net/v3/domains/example.com/webhooks/delivered", req.URL.String())

				return resp, nil
			})

		// Call the service
		err := service.DeleteWebhook(ctx, config, webhookID)

		// Verify results
		require.NoError(t, err)
	})

	t.Run("HTTP request error", func(t *testing.T) {
		// Set expectation for HTTP error
		mockHTTPClient.EXPECT().
			Do(gomock.Any()).
			Return(nil, errors.New("connection error"))

		// Allow any logger calls since we don't test logging
		mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

		// Call the service
		err := service.DeleteWebhook(ctx, config, webhookID)

		// Verify error handling
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to execute request")
	})

	t.Run("non-200 response", func(t *testing.T) {
		// Mock error response
		resp := &http.Response{
			StatusCode: http.StatusNotFound,
			Body:       io.NopCloser(strings.NewReader(`{"error": "Webhook not found"}`)),
		}

		// Set expectation
		mockHTTPClient.EXPECT().
			Do(gomock.Any()).
			Return(resp, nil)

		// Allow any logger calls since we don't test logging
		mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

		// Call the service
		err := service.DeleteWebhook(ctx, config, webhookID)

		// Verify error handling
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "API returned non-OK status code 404")
	})
}

func TestMailgunService_GetWebhook(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHTTPClient := mocks.NewMockHTTPClient(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	webhookEndpoint := "https://webhook.example.com"
	service := NewMailgunService(mockHTTPClient, mockAuthService, mockLogger, webhookEndpoint)

	ctx := context.Background()
	config := domain.MailgunSettings{
		Domain: "example.com",
		APIKey: "test-api-key",
		Region: "US",
	}
	webhookID := "delivered"

	t.Run("successful webhook retrieval", func(t *testing.T) {
		// Mock successful response
		responseBody := `{
			"webhook": {
				"id": "delivered",
				"url": "https://webhook.example.com/mailgun/delivered",
				"active": true
			}
		}`

		resp := &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(responseBody)),
		}

		// Set expectation for HTTP request
		mockHTTPClient.EXPECT().
			Do(gomock.Any()).
			DoAndReturn(func(req *http.Request) (*http.Response, error) {
				// Verify request
				assert.Equal(t, "GET", req.Method)
				assert.Equal(t, "https://api.mailgun.net/v3/domains/example.com/webhooks/delivered", req.URL.String())

				return resp, nil
			})

		// Call the service
		result, err := service.GetWebhook(ctx, config, webhookID)

		// Verify results
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, "delivered", result.ID)
		assert.Equal(t, "https://webhook.example.com/mailgun/delivered", result.URL)
		assert.Equal(t, []string{"delivered"}, result.Events)
		assert.True(t, result.Active)
	})

	t.Run("HTTP request error", func(t *testing.T) {
		// Set expectation for HTTP error
		mockHTTPClient.EXPECT().
			Do(gomock.Any()).
			Return(nil, errors.New("connection error"))

		// Allow any logger calls since we don't test logging
		mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

		// Call the service
		result, err := service.GetWebhook(ctx, config, webhookID)

		// Verify error handling
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "failed to execute request")
	})

	t.Run("non-200 response", func(t *testing.T) {
		// Mock error response
		resp := &http.Response{
			StatusCode: http.StatusNotFound,
			Body:       io.NopCloser(strings.NewReader(`{"error": "Webhook not found"}`)),
		}

		// Set expectation
		mockHTTPClient.EXPECT().
			Do(gomock.Any()).
			Return(resp, nil)

		// Allow any logger calls since we don't test logging
		mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

		// Call the service
		result, err := service.GetWebhook(ctx, config, webhookID)

		// Verify error handling
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "API returned non-OK status code 404")
	})
}

func TestMailgunService_UpdateWebhook(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHTTPClient := mocks.NewMockHTTPClient(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	webhookEndpoint := "https://webhook.example.com"
	service := NewMailgunService(mockHTTPClient, mockAuthService, mockLogger, webhookEndpoint)

	ctx := context.Background()
	config := domain.MailgunSettings{
		Domain: "example.com",
		APIKey: "test-api-key",
		Region: "US",
	}
	webhookID := "delivered"
	webhook := domain.MailgunWebhook{
		URL:    "https://webhook.example.com/mailgun/delivered-updated",
		Events: []string{"delivered"},
		Active: true,
	}

	t.Run("successful webhook update", func(t *testing.T) {
		// Mock successful response
		responseBody := `{
			"message": "Webhook has been updated",
			"webhook": {
				"id": "delivered",
				"url": "https://webhook.example.com/mailgun/delivered-updated"
			}
		}`

		resp := &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(responseBody)),
		}

		// Set expectation for HTTP request
		mockHTTPClient.EXPECT().
			Do(gomock.Any()).
			DoAndReturn(func(req *http.Request) (*http.Response, error) {
				// Verify request
				assert.Equal(t, "PUT", req.Method)
				assert.Equal(t, "https://api.mailgun.net/v3/domains/example.com/webhooks/delivered", req.URL.String())

				// Ensure body contains form data
				body, _ := io.ReadAll(req.Body)
				// Use a more generic assertion since the exact form parameter names might differ
				assert.Contains(t, string(body), "urls=https%3A%2F%2Fwebhook.example.com%2Fmailgun%2Fdelivered-updated")

				return resp, nil
			})

		// Call the service
		result, err := service.UpdateWebhook(ctx, config, webhookID, webhook)

		// Verify results
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, "delivered", result.ID)
		assert.Equal(t, "https://webhook.example.com/mailgun/delivered-updated", result.URL)
		assert.Equal(t, []string{"delivered"}, result.Events)
		assert.True(t, result.Active)
	})

	t.Run("HTTP request error", func(t *testing.T) {
		// Set expectation for HTTP error
		mockHTTPClient.EXPECT().
			Do(gomock.Any()).
			Return(nil, errors.New("connection error"))

		// Allow any logger calls since we don't test logging
		mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

		// Call the service
		result, err := service.UpdateWebhook(ctx, config, webhookID, webhook)

		// Verify error handling
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "failed to execute request")
	})

	t.Run("non-200 response", func(t *testing.T) {
		// Mock error response
		resp := &http.Response{
			StatusCode: http.StatusBadRequest,
			Body:       io.NopCloser(strings.NewReader(`{"error": "Bad Request"}`)),
		}

		// Set expectation
		mockHTTPClient.EXPECT().
			Do(gomock.Any()).
			Return(resp, nil)

		// Allow any logger calls since we don't test logging
		mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

		// Call the service
		result, err := service.UpdateWebhook(ctx, config, webhookID, webhook)

		// Verify error handling
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "API returned non-OK status code 400")
	})
}

func TestMailgunService_SendEmail(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHTTPClient := mocks.NewMockHTTPClient(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	webhookEndpoint := "https://webhook.example.com"
	service := NewMailgunService(mockHTTPClient, mockAuthService, mockLogger, webhookEndpoint)

	// Test data
	workspaceID := "workspace-123"
	fromAddress := "sender@example.com"
	fromName := "Test Sender"
	to := "recipient@example.com"
	subject := "Test Subject"
	content := "<p>Test Email Content</p>"

	t.Run("successful email sending without attachments", func(t *testing.T) {
		ctx := context.Background()

		// Create provider config
		provider := &domain.EmailProvider{
			Mailgun: &domain.MailgunSettings{
				Domain: "example.com",
				APIKey: "test-api-key",
				Region: "US",
			},
		}

		// Mock successful response
		responseBody := `{
			"id": "<message-id>",
			"message": "Queued. Thank you."
		}`

		resp := &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(responseBody)),
		}

		// Set expectation for HTTP request
		mockHTTPClient.EXPECT().
			Do(gomock.Any()).
			DoAndReturn(func(req *http.Request) (*http.Response, error) {
				// Verify request
				assert.Equal(t, "POST", req.Method)
				assert.Equal(t, "https://api.mailgun.net/v3/example.com/messages", req.URL.String())

				// Verify auth header
				username, password, ok := req.BasicAuth()
				assert.True(t, ok)
				assert.Equal(t, "api", username)
				assert.Equal(t, provider.Mailgun.APIKey, password)

				// Verify Content-Type header is form-urlencoded (not multipart) when no attachments
				assert.Equal(t, "application/x-www-form-urlencoded", req.Header.Get("Content-Type"))

				// Read and verify form data
				body, err := io.ReadAll(req.Body)
				require.NoError(t, err)
				formData := string(body)

				// Check for required fields in the form data
				assert.Contains(t, formData, "from="+url.QueryEscape(fmt.Sprintf("%s <%s>", fromName, fromAddress)))
				assert.Contains(t, formData, "to="+url.QueryEscape(to))
				assert.Contains(t, formData, "subject="+url.QueryEscape(subject))
				assert.Contains(t, formData, "html="+url.QueryEscape(content))

				return resp, nil
			})

		// Call the service
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
		err := service.SendEmail(ctx, request)

		// Verify results
		require.NoError(t, err)
	})

	t.Run("EU region", func(t *testing.T) {
		ctx := context.Background()

		// Create provider config with EU region
		provider := &domain.EmailProvider{
			Mailgun: &domain.MailgunSettings{
				Domain: "example.com",
				APIKey: "test-api-key",
				Region: "EU",
			},
		}

		// Mock successful response
		resp := &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"id": "<message-id>", "message": "Queued. Thank you."}`)),
		}

		// Set expectation for HTTP request with EU endpoint
		mockHTTPClient.EXPECT().
			Do(gomock.Any()).
			DoAndReturn(func(req *http.Request) (*http.Response, error) {
				// Verify EU endpoint is used
				assert.Equal(t, "https://api.eu.mailgun.net/v3/example.com/messages", req.URL.String())
				return resp, nil
			})

		// Call the service
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
		err := service.SendEmail(ctx, request)

		// Verify results
		require.NoError(t, err)
	})

	t.Run("missing Mailgun configuration", func(t *testing.T) {
		ctx := context.Background()

		// Create provider without Mailgun config
		provider := &domain.EmailProvider{}

		// Call the service
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
		err := service.SendEmail(ctx, request)

		// Verify error
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "mailgun provider is not configured")
	})

	t.Run("HTTP request error", func(t *testing.T) {
		ctx := context.Background()

		// Create provider config
		provider := &domain.EmailProvider{
			Mailgun: &domain.MailgunSettings{
				Domain: "example.com",
				APIKey: "test-api-key",
				Region: "US",
			},
		}

		// Set expectation for HTTP error
		mockHTTPClient.EXPECT().
			Do(gomock.Any()).
			Return(nil, errors.New("connection error"))

		// Allow any logger calls since we don't test logging
		mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

		// Call the service
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
		err := service.SendEmail(ctx, request)

		// Verify error handling
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to execute request")
	})

	t.Run("API error response", func(t *testing.T) {
		ctx := context.Background()

		// Create provider config
		provider := &domain.EmailProvider{
			Mailgun: &domain.MailgunSettings{
				Domain: "example.com",
				APIKey: "test-api-key",
				Region: "US",
			},
		}

		// Mock error response
		resp := &http.Response{
			StatusCode: http.StatusBadRequest,
			Body:       io.NopCloser(strings.NewReader(`{"message": "Invalid recipient address"}`)),
		}

		// Set expectation
		mockHTTPClient.EXPECT().
			Do(gomock.Any()).
			Return(resp, nil)

		// Allow any logger calls since we don't test logging
		mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

		// Call the service
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
		err := service.SendEmail(ctx, request)

		// Verify error handling
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "API returned non-OK status code 400")
	})

	t.Run("email with single attachment", func(t *testing.T) {
		ctx := context.Background()

		// Create provider config
		provider := &domain.EmailProvider{
			Mailgun: &domain.MailgunSettings{
				Domain: "example.com",
				APIKey: "test-api-key",
				Region: "US",
			},
		}

		// Create a small PDF attachment (base64 encoded)
		pdfContent := []byte("sample pdf content")
		base64Content := "c2FtcGxlIHBkZiBjb250ZW50" // base64 of "sample pdf content"

		// Mock successful response
		resp := &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"id": "<message-id>", "message": "Queued. Thank you."}`)),
		}

		// Allow logger calls for attachment debugging
		mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
		mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()

		// Set expectation for HTTP request
		mockHTTPClient.EXPECT().
			Do(gomock.Any()).
			DoAndReturn(func(req *http.Request) (*http.Response, error) {
				// Verify request
				assert.Equal(t, "POST", req.Method)
				assert.Equal(t, "https://api.mailgun.net/v3/example.com/messages", req.URL.String())

				// Verify Content-Type is multipart/form-data
				contentType := req.Header.Get("Content-Type")
				assert.Contains(t, contentType, "multipart/form-data")

				// Verify auth header
				username, password, ok := req.BasicAuth()
				assert.True(t, ok)
				assert.Equal(t, "api", username)
				assert.Equal(t, provider.Mailgun.APIKey, password)

				// Read and verify form data contains attachment
				body, err := io.ReadAll(req.Body)
				require.NoError(t, err)
				bodyStr := string(body)

				// Check for basic fields
				assert.Contains(t, bodyStr, "from")
				assert.Contains(t, bodyStr, "to")
				assert.Contains(t, bodyStr, "subject")
				assert.Contains(t, bodyStr, "html")

				// Check for attachment
				assert.Contains(t, bodyStr, "filename=\"invoice.pdf\"")
				assert.Contains(t, bodyStr, string(pdfContent))

				return resp, nil
			})

		// Call the service with attachment
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
		err := service.SendEmail(ctx, request)

		// Verify results
		require.NoError(t, err)
	})

	t.Run("email with multiple attachments", func(t *testing.T) {
		ctx := context.Background()

		// Create provider config
		provider := &domain.EmailProvider{
			Mailgun: &domain.MailgunSettings{
				Domain: "example.com",
				APIKey: "test-api-key",
				Region: "US",
			},
		}

		// Create attachments (base64 encoded)
		pdfContent := "c2FtcGxlIHBkZiBjb250ZW50"       // base64 of "sample pdf content"
		imageContent := "c2FtcGxlIGltYWdlIGNvbnRlbnQ=" // base64 of "sample image content"

		// Mock successful response
		resp := &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"id": "<message-id>", "message": "Queued. Thank you."}`)),
		}

		// Allow logger calls for attachment debugging
		mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
		mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()

		// Set expectation for HTTP request
		mockHTTPClient.EXPECT().
			Do(gomock.Any()).
			DoAndReturn(func(req *http.Request) (*http.Response, error) {
				// Verify Content-Type is multipart/form-data
				contentType := req.Header.Get("Content-Type")
				assert.Contains(t, contentType, "multipart/form-data")

				// Read and verify form data contains both attachments
				body, err := io.ReadAll(req.Body)
				require.NoError(t, err)
				bodyStr := string(body)

				// Check for both attachments
				assert.Contains(t, bodyStr, "filename=\"invoice.pdf\"")
				assert.Contains(t, bodyStr, "filename=\"logo.png\"")

				return resp, nil
			})

		// Call the service with multiple attachments
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
		err := service.SendEmail(ctx, request)

		// Verify results
		require.NoError(t, err)
	})

	t.Run("email with inline attachment", func(t *testing.T) {
		ctx := context.Background()

		// Create provider config
		provider := &domain.EmailProvider{
			Mailgun: &domain.MailgunSettings{
				Domain: "example.com",
				APIKey: "test-api-key",
				Region: "US",
			},
		}

		// Create an inline image attachment
		imageContent := "c2FtcGxlIGltYWdlIGNvbnRlbnQ=" // base64 of "sample image content"

		// Mock successful response
		resp := &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"id": "<message-id>", "message": "Queued. Thank you."}`)),
		}

		// Allow logger calls for attachment debugging
		mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
		mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()

		// Set expectation for HTTP request
		mockHTTPClient.EXPECT().
			Do(gomock.Any()).
			DoAndReturn(func(req *http.Request) (*http.Response, error) {
				// Verify Content-Type is multipart/form-data
				contentType := req.Header.Get("Content-Type")
				assert.Contains(t, contentType, "multipart/form-data")

				// Read and verify form data
				body, err := io.ReadAll(req.Body)
				require.NoError(t, err)
				bodyStr := string(body)

				// Check for inline attachment
				assert.Contains(t, bodyStr, "filename=\"logo.png\"")
				// Verify it's marked as inline (Mailgun uses "inline" field name for inline attachments)
				assert.Contains(t, bodyStr, "name=\"inline\"")

				return resp, nil
			})

		// Call the service with inline attachment
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
		err := service.SendEmail(ctx, request)

		// Verify results
		require.NoError(t, err)
	})

	t.Run("email with attachments and CC/BCC", func(t *testing.T) {
		ctx := context.Background()

		// Create provider config
		provider := &domain.EmailProvider{
			Mailgun: &domain.MailgunSettings{
				Domain: "example.com",
				APIKey: "test-api-key",
				Region: "US",
			},
		}

		// Create attachment
		pdfContent := "c2FtcGxlIHBkZiBjb250ZW50" // base64 of "sample pdf content"

		// Mock successful response
		resp := &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"id": "<message-id>", "message": "Queued. Thank you."}`)),
		}

		// Allow logger calls for attachment debugging
		mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
		mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()

		// Set expectation for HTTP request
		mockHTTPClient.EXPECT().
			Do(gomock.Any()).
			DoAndReturn(func(req *http.Request) (*http.Response, error) {
				// Read and verify form data
				body, err := io.ReadAll(req.Body)
				require.NoError(t, err)
				bodyStr := string(body)

				// Check for attachment
				assert.Contains(t, bodyStr, "filename=\"invoice.pdf\"")

				// Check for CC and BCC
				assert.Contains(t, bodyStr, "cc1@example.com")
				assert.Contains(t, bodyStr, "bcc1@example.com")

				// Check for reply-to
				assert.Contains(t, bodyStr, "reply@example.com")

				return resp, nil
			})

		// Call the service with attachment and CC/BCC
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
		err := service.SendEmail(ctx, request)

		// Verify results
		require.NoError(t, err)
	})

	t.Run("email with attachment decode error", func(t *testing.T) {
		ctx := context.Background()

		// Create provider config
		provider := &domain.EmailProvider{
			Mailgun: &domain.MailgunSettings{
				Domain: "example.com",
				APIKey: "test-api-key",
				Region: "US",
			},
		}

		// Call the service with invalid base64 content
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
		err := service.SendEmail(ctx, request)

		// Verify error handling
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to decode content")
	})

	t.Run("email with attachment API error", func(t *testing.T) {
		ctx := context.Background()

		// Create provider config
		provider := &domain.EmailProvider{
			Mailgun: &domain.MailgunSettings{
				Domain: "example.com",
				APIKey: "test-api-key",
				Region: "US",
			},
		}

		// Create attachment
		pdfContent := "c2FtcGxlIHBkZiBjb250ZW50" // base64 of "sample pdf content"

		// Mock error response
		resp := &http.Response{
			StatusCode: http.StatusRequestEntityTooLarge,
			Body:       io.NopCloser(strings.NewReader(`{"message": "Attachment too large"}`)),
		}

		// Set expectation
		mockHTTPClient.EXPECT().
			Do(gomock.Any()).
			Return(resp, nil)

		// Allow any logger calls since we don't test logging
		mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

		// Call the service
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
		err := service.SendEmail(ctx, request)

		// Verify error handling
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "API returned non-OK status code 413")
	})
}
