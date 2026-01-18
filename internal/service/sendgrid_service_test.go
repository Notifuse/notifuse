package service_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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

// mockSendGridHTTPResponse creates a mock HTTP response for SendGrid tests
func mockSendGridHTTPResponse(statusCode int, body string) *http.Response {
	return &http.Response{
		StatusCode: statusCode,
		Body:       io.NopCloser(bytes.NewReader([]byte(body))),
	}
}

func TestSendGridService_GetWebhookSettings(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHTTPClient := mocks.NewMockHTTPClient(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()

	sendGridService := service.NewSendGridService(mockHTTPClient, mockAuthService, mockLogger)

	config := domain.SendGridSettings{
		APIKey: "SG.test-api-key",
	}

	t.Run("Success", func(t *testing.T) {
		ctx := context.Background()

		webhookSettings := domain.SendGridWebhookSettings{
			Enabled:    true,
			URL:        "https://example.com/webhooks/email?provider=sendgrid",
			Delivered:  true,
			Bounce:     true,
			SpamReport: true,
		}
		responseJSON, _ := json.Marshal(webhookSettings)

		mockHTTPClient.EXPECT().
			Do(gomock.Any()).
			DoAndReturn(func(req *http.Request) (*http.Response, error) {
				assert.Equal(t, "GET", req.Method)
				assert.Equal(t, "https://api.sendgrid.com/v3/user/webhooks/event/settings", req.URL.String())
				assert.Equal(t, "Bearer SG.test-api-key", req.Header.Get("Authorization"))

				return mockSendGridHTTPResponse(http.StatusOK, string(responseJSON)), nil
			})

		result, err := sendGridService.GetWebhookSettings(ctx, config)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.True(t, result.Enabled)
		assert.True(t, result.Delivered)
		assert.True(t, result.Bounce)
		assert.True(t, result.SpamReport)
	})

	t.Run("HTTP request error", func(t *testing.T) {
		ctx := context.Background()
		expectedErr := errors.New("connection error")

		mockHTTPClient.EXPECT().
			Do(gomock.Any()).
			Return(nil, expectedErr)

		result, err := sendGridService.GetWebhookSettings(ctx, config)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "failed to execute request")
	})

	t.Run("Non-OK status code", func(t *testing.T) {
		ctx := context.Background()

		mockHTTPClient.EXPECT().
			Do(gomock.Any()).
			Return(mockSendGridHTTPResponse(http.StatusUnauthorized, `{"errors":[{"message":"Unauthorized"}]}`), nil)

		result, err := sendGridService.GetWebhookSettings(ctx, config)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "API returned non-OK status code 401")
	})

	t.Run("Invalid response body", func(t *testing.T) {
		ctx := context.Background()

		mockHTTPClient.EXPECT().
			Do(gomock.Any()).
			Return(mockSendGridHTTPResponse(http.StatusOK, `invalid json`), nil)

		result, err := sendGridService.GetWebhookSettings(ctx, config)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "failed to decode response")
	})
}

func TestSendGridService_UpdateWebhookSettings(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHTTPClient := mocks.NewMockHTTPClient(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()

	sendGridService := service.NewSendGridService(mockHTTPClient, mockAuthService, mockLogger)

	config := domain.SendGridSettings{
		APIKey: "SG.test-api-key",
	}

	settings := domain.SendGridWebhookSettings{
		Enabled:    true,
		URL:        "https://example.com/webhooks/email?provider=sendgrid",
		Delivered:  true,
		Bounce:     true,
		SpamReport: true,
	}

	t.Run("Success", func(t *testing.T) {
		ctx := context.Background()

		mockHTTPClient.EXPECT().
			Do(gomock.Any()).
			DoAndReturn(func(req *http.Request) (*http.Response, error) {
				assert.Equal(t, "PATCH", req.Method)
				assert.Equal(t, "https://api.sendgrid.com/v3/user/webhooks/event/settings", req.URL.String())
				assert.Equal(t, "Bearer SG.test-api-key", req.Header.Get("Authorization"))
				assert.Equal(t, "application/json", req.Header.Get("Content-Type"))

				return mockSendGridHTTPResponse(http.StatusOK, `{}`), nil
			})

		err := sendGridService.UpdateWebhookSettings(ctx, config, settings)

		assert.NoError(t, err)
	})

	t.Run("HTTP request error", func(t *testing.T) {
		ctx := context.Background()
		expectedErr := errors.New("connection error")

		mockHTTPClient.EXPECT().
			Do(gomock.Any()).
			Return(nil, expectedErr)

		err := sendGridService.UpdateWebhookSettings(ctx, config, settings)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to execute request")
	})

	t.Run("Non-OK status code", func(t *testing.T) {
		ctx := context.Background()

		mockHTTPClient.EXPECT().
			Do(gomock.Any()).
			Return(mockSendGridHTTPResponse(http.StatusUnauthorized, `{"errors":[{"message":"Unauthorized"}]}`), nil)

		err := sendGridService.UpdateWebhookSettings(ctx, config, settings)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "API returned non-OK status code 401")
	})
}

func TestSendGridService_RegisterWebhooks(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHTTPClient := mocks.NewMockHTTPClient(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()

	sendGridService := service.NewSendGridService(mockHTTPClient, mockAuthService, mockLogger)

	t.Run("Success", func(t *testing.T) {
		ctx := context.Background()

		providerConfig := &domain.EmailProvider{
			Kind: domain.EmailProviderKindSendGrid,
			SendGrid: &domain.SendGridSettings{
				APIKey: "SG.test-api-key",
			},
		}

		mockHTTPClient.EXPECT().
			Do(gomock.Any()).
			DoAndReturn(func(req *http.Request) (*http.Response, error) {
				assert.Equal(t, "PATCH", req.Method)
				return mockSendGridHTTPResponse(http.StatusOK, `{}`), nil
			})

		result, err := sendGridService.RegisterWebhooks(
			ctx,
			"workspace-123",
			"integration-456",
			"https://api.example.com",
			[]domain.EmailEventType{domain.EmailEventDelivered, domain.EmailEventBounce},
			providerConfig,
		)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.True(t, result.IsRegistered)
		assert.Equal(t, domain.EmailProviderKindSendGrid, result.EmailProviderKind)
	})

	t.Run("Missing configuration", func(t *testing.T) {
		ctx := context.Background()

		result, err := sendGridService.RegisterWebhooks(
			ctx,
			"workspace-123",
			"integration-456",
			"https://api.example.com",
			[]domain.EmailEventType{domain.EmailEventDelivered},
			nil,
		)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "SendGrid configuration is missing or invalid")
	})

	t.Run("Missing API key", func(t *testing.T) {
		ctx := context.Background()

		providerConfig := &domain.EmailProvider{
			Kind:     domain.EmailProviderKindSendGrid,
			SendGrid: &domain.SendGridSettings{},
		}

		result, err := sendGridService.RegisterWebhooks(
			ctx,
			"workspace-123",
			"integration-456",
			"https://api.example.com",
			[]domain.EmailEventType{domain.EmailEventDelivered},
			providerConfig,
		)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "SendGrid configuration is missing or invalid")
	})
}

func TestSendGridService_GetWebhookStatus(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHTTPClient := mocks.NewMockHTTPClient(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()

	sendGridService := service.NewSendGridService(mockHTTPClient, mockAuthService, mockLogger)

	t.Run("Success", func(t *testing.T) {
		ctx := context.Background()

		providerConfig := &domain.EmailProvider{
			Kind: domain.EmailProviderKindSendGrid,
			SendGrid: &domain.SendGridSettings{
				APIKey: "SG.test-api-key",
			},
		}

		webhookSettings := domain.SendGridWebhookSettings{
			Enabled:    true,
			URL:        "https://example.com/webhooks/email?provider=sendgrid",
			Delivered:  true,
			Bounce:     true,
			SpamReport: true,
		}
		responseJSON, _ := json.Marshal(webhookSettings)

		mockHTTPClient.EXPECT().
			Do(gomock.Any()).
			DoAndReturn(func(req *http.Request) (*http.Response, error) {
				assert.Equal(t, "GET", req.Method)
				return mockSendGridHTTPResponse(http.StatusOK, string(responseJSON)), nil
			})

		result, err := sendGridService.GetWebhookStatus(ctx, "workspace-123", "integration-456", providerConfig)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.True(t, result.IsRegistered)
		assert.Len(t, result.Endpoints, 3) // Delivered, Bounce, SpamReport
	})

	t.Run("Missing configuration", func(t *testing.T) {
		ctx := context.Background()

		result, err := sendGridService.GetWebhookStatus(ctx, "workspace-123", "integration-456", nil)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "SendGrid configuration is missing or invalid")
	})
}

func TestSendGridService_UnregisterWebhooks(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHTTPClient := mocks.NewMockHTTPClient(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()

	sendGridService := service.NewSendGridService(mockHTTPClient, mockAuthService, mockLogger)

	t.Run("Success", func(t *testing.T) {
		ctx := context.Background()

		providerConfig := &domain.EmailProvider{
			Kind: domain.EmailProviderKindSendGrid,
			SendGrid: &domain.SendGridSettings{
				APIKey: "SG.test-api-key",
			},
		}

		mockHTTPClient.EXPECT().
			Do(gomock.Any()).
			DoAndReturn(func(req *http.Request) (*http.Response, error) {
				assert.Equal(t, "PATCH", req.Method)
				return mockSendGridHTTPResponse(http.StatusOK, `{}`), nil
			})

		err := sendGridService.UnregisterWebhooks(ctx, "workspace-123", "integration-456", providerConfig)

		assert.NoError(t, err)
	})

	t.Run("Missing configuration", func(t *testing.T) {
		ctx := context.Background()

		err := sendGridService.UnregisterWebhooks(ctx, "workspace-123", "integration-456", nil)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "SendGrid configuration is missing or invalid")
	})
}

func TestSendGridService_SendEmail(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHTTPClient := mocks.NewMockHTTPClient(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()

	sendGridService := service.NewSendGridService(mockHTTPClient, mockAuthService, mockLogger)

	t.Run("Success", func(t *testing.T) {
		ctx := context.Background()

		request := domain.SendEmailProviderRequest{
			WorkspaceID:   "workspace-123",
			IntegrationID: "integration-456",
			MessageID:     "msg-789",
			FromAddress:   "sender@example.com",
			FromName:      "Sender Name",
			To:            "recipient@example.com",
			Subject:       "Test Subject",
			Content:       "<p>Test content</p>",
			Provider: &domain.EmailProvider{
				Kind: domain.EmailProviderKindSendGrid,
				SendGrid: &domain.SendGridSettings{
					APIKey: "SG.test-api-key",
				},
			},
		}

		mockHTTPClient.EXPECT().
			Do(gomock.Any()).
			DoAndReturn(func(req *http.Request) (*http.Response, error) {
				assert.Equal(t, "POST", req.Method)
				assert.Equal(t, "https://api.sendgrid.com/v3/mail/send", req.URL.String())
				assert.Equal(t, "Bearer SG.test-api-key", req.Header.Get("Authorization"))
				assert.Equal(t, "application/json", req.Header.Get("Content-Type"))

				// Verify request body contains custom_args
				body, _ := io.ReadAll(req.Body)
				assert.Contains(t, string(body), `"notifuse_message_id":"msg-789"`)

				return mockSendGridHTTPResponse(http.StatusAccepted, `{}`), nil
			})

		err := sendGridService.SendEmail(ctx, request)

		assert.NoError(t, err)
	})

	t.Run("Success with email options", func(t *testing.T) {
		ctx := context.Background()

		request := domain.SendEmailProviderRequest{
			WorkspaceID:   "workspace-123",
			IntegrationID: "integration-456",
			MessageID:     "msg-789",
			FromAddress:   "sender@example.com",
			FromName:      "Sender Name",
			To:            "recipient@example.com",
			Subject:       "Test Subject",
			Content:       "<p>Test content</p>",
			Provider: &domain.EmailProvider{
				Kind: domain.EmailProviderKindSendGrid,
				SendGrid: &domain.SendGridSettings{
					APIKey: "SG.test-api-key",
				},
			},
			EmailOptions: domain.EmailOptions{
				CC:                 []string{"cc@example.com"},
				BCC:                []string{"bcc@example.com"},
				ReplyTo:            "reply@example.com",
				ListUnsubscribeURL: "https://example.com/unsubscribe",
			},
		}

		mockHTTPClient.EXPECT().
			Do(gomock.Any()).
			DoAndReturn(func(req *http.Request) (*http.Response, error) {
				body, _ := io.ReadAll(req.Body)
				bodyStr := string(body)

				// Verify CC, BCC, ReplyTo are included
				assert.Contains(t, bodyStr, `"cc":[{"email":"cc@example.com"}]`)
				assert.Contains(t, bodyStr, `"bcc":[{"email":"bcc@example.com"}]`)
				assert.Contains(t, bodyStr, `"reply_to":{"email":"reply@example.com"}`)
				assert.Contains(t, bodyStr, `"List-Unsubscribe"`)

				return mockSendGridHTTPResponse(http.StatusAccepted, `{}`), nil
			})

		err := sendGridService.SendEmail(ctx, request)

		assert.NoError(t, err)
	})

	t.Run("HTTP request error", func(t *testing.T) {
		ctx := context.Background()
		expectedErr := errors.New("connection error")

		request := domain.SendEmailProviderRequest{
			WorkspaceID:   "workspace-123",
			IntegrationID: "integration-456",
			MessageID:     "msg-789",
			FromAddress:   "sender@example.com",
			FromName:      "Sender Name",
			To:            "recipient@example.com",
			Subject:       "Test Subject",
			Content:       "<p>Test content</p>",
			Provider: &domain.EmailProvider{
				Kind: domain.EmailProviderKindSendGrid,
				SendGrid: &domain.SendGridSettings{
					APIKey: "SG.test-api-key",
				},
			},
		}

		mockHTTPClient.EXPECT().
			Do(gomock.Any()).
			Return(nil, expectedErr)

		err := sendGridService.SendEmail(ctx, request)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to execute request")
	})

	t.Run("Non-OK status code", func(t *testing.T) {
		ctx := context.Background()

		request := domain.SendEmailProviderRequest{
			WorkspaceID:   "workspace-123",
			IntegrationID: "integration-456",
			MessageID:     "msg-789",
			FromAddress:   "sender@example.com",
			FromName:      "Sender Name",
			To:            "recipient@example.com",
			Subject:       "Test Subject",
			Content:       "<p>Test content</p>",
			Provider: &domain.EmailProvider{
				Kind: domain.EmailProviderKindSendGrid,
				SendGrid: &domain.SendGridSettings{
					APIKey: "SG.test-api-key",
				},
			},
		}

		mockHTTPClient.EXPECT().
			Do(gomock.Any()).
			Return(mockSendGridHTTPResponse(http.StatusBadRequest, `{"errors":[{"message":"Invalid email"}]}`), nil)

		err := sendGridService.SendEmail(ctx, request)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "API returned non-OK status code 400")
	})

	t.Run("Missing provider", func(t *testing.T) {
		ctx := context.Background()

		request := domain.SendEmailProviderRequest{
			WorkspaceID:   "workspace-123",
			IntegrationID: "integration-456",
			MessageID:     "msg-789",
			FromAddress:   "sender@example.com",
			FromName:      "Sender Name",
			To:            "recipient@example.com",
			Subject:       "Test Subject",
			Content:       "<p>Test content</p>",
			Provider: &domain.EmailProvider{
				Kind: domain.EmailProviderKindSendGrid,
			},
		}

		err := sendGridService.SendEmail(ctx, request)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "SendGrid provider is not configured")
	})

	t.Run("Invalid request - missing required fields", func(t *testing.T) {
		ctx := context.Background()

		request := domain.SendEmailProviderRequest{
			Provider: &domain.EmailProvider{
				Kind: domain.EmailProviderKindSendGrid,
				SendGrid: &domain.SendGridSettings{
					APIKey: "SG.test-api-key",
				},
			},
		}

		err := sendGridService.SendEmail(ctx, request)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid request")
	})
}
