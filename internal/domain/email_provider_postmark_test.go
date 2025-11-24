package domain_test

import (
	"context"
	"testing"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/Notifuse/notifuse/pkg/crypto"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPostmarkSettings_DecryptServerToken(t *testing.T) {
	// Setup
	passphrase := "test-passphrase"
	serverToken := "test-server-token"

	// Create encrypted token
	encryptedToken, err := crypto.EncryptString(serverToken, passphrase)
	require.NoError(t, err)

	settings := domain.PostmarkSettings{
		EncryptedServerToken: encryptedToken,
	}

	// Test
	err = settings.DecryptServerToken(passphrase)
	require.NoError(t, err)
	assert.Equal(t, serverToken, settings.ServerToken)

	// Test with invalid passphrase
	settings.ServerToken = ""
	err = settings.DecryptServerToken("wrong-passphrase")
	assert.Error(t, err)
}

func TestPostmarkSettings_EncryptServerToken(t *testing.T) {
	// Setup
	passphrase := "test-passphrase"
	serverToken := "test-server-token"

	settings := domain.PostmarkSettings{
		ServerToken: serverToken,
	}

	// Test
	err := settings.EncryptServerToken(passphrase)
	require.NoError(t, err)
	assert.NotEmpty(t, settings.EncryptedServerToken)

	// Verify by decrypting
	decrypted, err := crypto.DecryptFromHexString(settings.EncryptedServerToken, passphrase)
	require.NoError(t, err)
	assert.Equal(t, serverToken, decrypted)
}

func TestPostmarkSettings_ValidateSettings(t *testing.T) {
	// Setup
	passphrase := "test-passphrase"
	serverToken := "test-server-token"

	settings := domain.PostmarkSettings{
		ServerToken: serverToken,
	}

	// Test
	err := settings.Validate(passphrase)
	require.NoError(t, err)
	assert.NotEmpty(t, settings.EncryptedServerToken)

	// Verify encryption happened
	decrypted, err := crypto.DecryptFromHexString(settings.EncryptedServerToken, passphrase)
	require.NoError(t, err)
	assert.Equal(t, serverToken, decrypted)
}

// TestPostmarkServiceInterface tests the PostmarkServiceInterface using generated mocks
func TestPostmarkServiceInterface(t *testing.T) {
	// Setup
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockService := mocks.NewMockPostmarkServiceInterface(ctrl)
	ctx := context.Background()
	config := domain.PostmarkSettings{
		ServerToken: "test-token",
	}
	webhookID := 123

	// Create test webhook config
	webhookConfig := domain.PostmarkWebhookConfig{
		URL:           "https://example.com/webhook",
		MessageStream: "outbound",
		Triggers: &domain.PostmarkTriggers{
			Delivery: &domain.PostmarkDeliveryTrigger{
				Enabled: true,
			},
		},
	}

	// Create expected responses
	expectedWebhookResponse := &domain.PostmarkWebhookResponse{
		ID:            webhookID,
		URL:           webhookConfig.URL,
		MessageStream: webhookConfig.MessageStream,
		Triggers:      webhookConfig.Triggers,
	}

	expectedListResponse := &domain.PostmarkListWebhooksResponse{
		TotalCount: 1,
		Webhooks: []domain.PostmarkWebhookResponse{
			*expectedWebhookResponse,
		},
	}

	t.Run("ListWebhooks", func(t *testing.T) {
		// Set expectations
		mockService.EXPECT().ListWebhooks(gomock.Any(), gomock.Eq(config)).Return(expectedListResponse, nil)

		// Call the method
		response, err := mockService.ListWebhooks(ctx, config)

		// Assert
		require.NoError(t, err)
		assert.Equal(t, expectedListResponse, response)
	})

	t.Run("RegisterWebhook", func(t *testing.T) {
		// Set expectations
		mockService.EXPECT().RegisterWebhook(gomock.Any(), gomock.Eq(config), gomock.Eq(webhookConfig)).Return(expectedWebhookResponse, nil)

		// Call the method
		response, err := mockService.RegisterWebhook(ctx, config, webhookConfig)

		// Assert
		require.NoError(t, err)
		assert.Equal(t, expectedWebhookResponse, response)
	})

	t.Run("UnregisterWebhook", func(t *testing.T) {
		// Set expectations
		mockService.EXPECT().UnregisterWebhook(gomock.Any(), gomock.Eq(config), gomock.Eq(webhookID)).Return(nil)

		// Call the method
		err := mockService.UnregisterWebhook(ctx, config, webhookID)

		// Assert
		require.NoError(t, err)
	})

	t.Run("GetWebhook", func(t *testing.T) {
		// Set expectations
		mockService.EXPECT().GetWebhook(gomock.Any(), gomock.Eq(config), gomock.Eq(webhookID)).Return(expectedWebhookResponse, nil)

		// Call the method
		response, err := mockService.GetWebhook(ctx, config, webhookID)

		// Assert
		require.NoError(t, err)
		assert.Equal(t, expectedWebhookResponse, response)
	})

	t.Run("UpdateWebhook", func(t *testing.T) {
		// Set expectations
		mockService.EXPECT().UpdateWebhook(gomock.Any(), gomock.Eq(config), gomock.Eq(webhookID), gomock.Eq(webhookConfig)).Return(expectedWebhookResponse, nil)

		// Call the method
		response, err := mockService.UpdateWebhook(ctx, config, webhookID, webhookConfig)

		// Assert
		require.NoError(t, err)
		assert.Equal(t, expectedWebhookResponse, response)
	})

	t.Run("TestWebhook", func(t *testing.T) {
		// Set expectations
		mockService.EXPECT().TestWebhook(gomock.Any(), gomock.Eq(config), gomock.Eq(webhookID), gomock.Eq(domain.EmailEventDelivered)).Return(nil)

		// Call the method
		err := mockService.TestWebhook(ctx, config, webhookID, domain.EmailEventDelivered)

		// Assert
		require.NoError(t, err)
	})
}

// Test email provider with Postmark settings
func TestEmailProvider_WithPostmarkSettings(t *testing.T) {
	passphrase := "test-passphrase"

	t.Run("Postmark provider encryption/decryption", func(t *testing.T) {
		provider := domain.EmailProvider{
			Kind:               domain.EmailProviderKindPostmark,
			RateLimitPerMinute: 25,
			Senders: []domain.EmailSender{
				domain.NewEmailSender("default@example.com", "Default Sender"),
			},
			Postmark: &domain.PostmarkSettings{
				ServerToken: "test-server-token",
			},
		}

		// Test encryption
		err := provider.EncryptSecretKeys(passphrase)
		assert.NoError(t, err)
		assert.NotEmpty(t, provider.Postmark.EncryptedServerToken)
		assert.Empty(t, provider.Postmark.ServerToken)

		// Test decryption
		err = provider.DecryptSecretKeys(passphrase)
		assert.NoError(t, err)
		assert.Equal(t, "test-server-token", provider.Postmark.ServerToken)
	})

	t.Run("Postmark provider validation", func(t *testing.T) {
		// Valid provider
		provider := domain.EmailProvider{
			Kind:               domain.EmailProviderKindPostmark,
			RateLimitPerMinute: 25,
			Senders: []domain.EmailSender{
				domain.NewEmailSender("default@example.com", "Default Sender"),
			},
			Postmark: &domain.PostmarkSettings{
				ServerToken: "test-server-token",
			},
		}

		// Should validate without error
		err := provider.Validate(passphrase)
		assert.NoError(t, err)
		assert.NotEmpty(t, provider.Postmark.EncryptedServerToken)

		// Provider with missing Postmark settings
		invalidProvider := domain.EmailProvider{
			Kind:               domain.EmailProviderKindPostmark,
			RateLimitPerMinute: 25,
			Senders: []domain.EmailSender{
				domain.NewEmailSender("default@example.com", "Default Sender"),
			},
		}

		// Should fail validation
		err = invalidProvider.Validate(passphrase)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "postmark settings required")
	})
}
