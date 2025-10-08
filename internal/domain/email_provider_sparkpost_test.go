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

func TestSparkPostSettings_DecryptAPIKey(t *testing.T) {
	// Setup
	passphrase := "test-passphrase"
	apiKey := "test-api-key"

	// Create encrypted key
	encryptedKey, err := crypto.EncryptString(apiKey, passphrase)
	require.NoError(t, err)

	settings := domain.SparkPostSettings{
		EncryptedAPIKey: encryptedKey,
	}

	// Test successful decryption
	err = settings.DecryptAPIKey(passphrase)
	require.NoError(t, err)
	assert.Equal(t, apiKey, settings.APIKey)

	// Test with invalid passphrase
	settings.APIKey = ""
	err = settings.DecryptAPIKey("wrong-passphrase")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to decrypt SparkPost API key")
}

func TestSparkPostSettings_EncryptAPIKey(t *testing.T) {
	// Setup
	passphrase := "test-passphrase"
	apiKey := "test-api-key"

	settings := domain.SparkPostSettings{
		APIKey: apiKey,
	}

	// Test
	err := settings.EncryptAPIKey(passphrase)
	require.NoError(t, err)
	assert.NotEmpty(t, settings.EncryptedAPIKey)
	assert.NotEqual(t, apiKey, settings.EncryptedAPIKey)

	// Verify by decrypting
	decrypted, err := crypto.DecryptFromHexString(settings.EncryptedAPIKey, passphrase)
	require.NoError(t, err)
	assert.Equal(t, apiKey, decrypted)
}

func TestSparkPostSettings_Validate(t *testing.T) {
	passphrase := "test-passphrase"

	t.Run("Valid settings", func(t *testing.T) {
		settings := domain.SparkPostSettings{
			APIKey:      "test-api-key",
			Endpoint:    "https://api.sparkpost.com/api/v1",
			SandboxMode: false,
		}

		err := settings.Validate(passphrase)
		require.NoError(t, err)
		assert.NotEmpty(t, settings.EncryptedAPIKey)
		assert.Equal(t, "test-api-key", settings.APIKey)

		// Verify encryption happened correctly
		decrypted, err := crypto.DecryptFromHexString(settings.EncryptedAPIKey, passphrase)
		require.NoError(t, err)
		assert.Equal(t, "test-api-key", decrypted)
	})

	t.Run("Missing endpoint", func(t *testing.T) {
		settings := domain.SparkPostSettings{
			APIKey:      "test-api-key",
			SandboxMode: false,
		}

		err := settings.Validate(passphrase)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "endpoint is required")
	})

	t.Run("No API key to encrypt", func(t *testing.T) {
		settings := domain.SparkPostSettings{
			Endpoint:    "https://api.sparkpost.com/api/v1",
			SandboxMode: false,
		}

		err := settings.Validate(passphrase)
		assert.NoError(t, err)
		assert.Empty(t, settings.EncryptedAPIKey)
	})
}

func TestSparkPostServiceInterface(t *testing.T) {
	// Setup
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockService := mocks.NewMockSparkPostServiceInterface(ctrl)
	ctx := context.Background()

	config := domain.SparkPostSettings{
		APIKey:      "test-api-key",
		Endpoint:    "https://api.sparkpost.com/api/v1",
		SandboxMode: false,
	}

	webhookID := "webhook-123"

	webhook := domain.SparkPostWebhook{
		Name:     "Test Webhook",
		Target:   "https://example.com/webhook",
		Events:   []string{"delivery", "bounce", "spam_complaint"},
		Active:   true,
		AuthType: "none",
	}

	// Expected responses
	expectedWebhook := domain.SparkPostWebhook{
		ID:       webhookID,
		Name:     webhook.Name,
		Target:   webhook.Target,
		Events:   webhook.Events,
		Active:   webhook.Active,
		AuthType: webhook.AuthType,
	}

	expectedWebhookResponse := &domain.SparkPostWebhookResponse{
		Results: expectedWebhook,
	}

	expectedWebhookListResponse := &domain.SparkPostWebhookListResponse{
		Results: []domain.SparkPostWebhook{expectedWebhook},
	}

	t.Run("ListWebhooks", func(t *testing.T) {
		// Set expectations
		mockService.EXPECT().ListWebhooks(gomock.Any(), gomock.Eq(config)).Return(expectedWebhookListResponse, nil)

		// Call the method
		response, err := mockService.ListWebhooks(ctx, config)

		// Assert
		require.NoError(t, err)
		assert.Equal(t, expectedWebhookListResponse, response)
	})

	t.Run("CreateWebhook", func(t *testing.T) {
		// Set expectations
		mockService.EXPECT().CreateWebhook(gomock.Any(), gomock.Eq(config), gomock.Eq(webhook)).Return(expectedWebhookResponse, nil)

		// Call the method
		response, err := mockService.CreateWebhook(ctx, config, webhook)

		// Assert
		require.NoError(t, err)
		assert.Equal(t, expectedWebhookResponse, response)
		assert.Equal(t, webhookID, expectedWebhook.ID)
	})

	t.Run("GetWebhook", func(t *testing.T) {
		// Set expectations
		mockService.EXPECT().GetWebhook(gomock.Any(), gomock.Eq(config), gomock.Eq(webhookID)).Return(expectedWebhookResponse, nil)

		// Call the method
		response, err := mockService.GetWebhook(ctx, config, webhookID)

		// Assert
		require.NoError(t, err)
		assert.Equal(t, expectedWebhookResponse, response)
		assert.Equal(t, webhookID, expectedWebhook.ID)
	})

	t.Run("UpdateWebhook", func(t *testing.T) {
		// Set expectations
		mockService.EXPECT().UpdateWebhook(gomock.Any(), gomock.Eq(config), gomock.Eq(webhookID), gomock.Eq(webhook)).Return(expectedWebhookResponse, nil)

		// Call the method
		response, err := mockService.UpdateWebhook(ctx, config, webhookID, webhook)

		// Assert
		require.NoError(t, err)
		assert.Equal(t, expectedWebhookResponse, response)
		assert.Equal(t, webhookID, expectedWebhook.ID)
	})

	t.Run("DeleteWebhook", func(t *testing.T) {
		// Set expectations
		mockService.EXPECT().DeleteWebhook(gomock.Any(), gomock.Eq(config), gomock.Eq(webhookID)).Return(nil)

		// Call the method
		err := mockService.DeleteWebhook(ctx, config, webhookID)

		// Assert
		require.NoError(t, err)
	})

	t.Run("TestWebhook", func(t *testing.T) {
		// Set expectations
		mockService.EXPECT().TestWebhook(gomock.Any(), gomock.Eq(config), gomock.Eq(webhookID)).Return(nil)

		// Call the method
		err := mockService.TestWebhook(ctx, config, webhookID)

		// Assert
		require.NoError(t, err)
	})

	t.Run("ValidateWebhook", func(t *testing.T) {
		// Set expectations
		mockService.EXPECT().ValidateWebhook(gomock.Any(), gomock.Eq(config), gomock.Eq(webhook)).Return(true, nil)

		// Call the method
		isValid, err := mockService.ValidateWebhook(ctx, config, webhook)

		// Assert
		require.NoError(t, err)
		assert.True(t, isValid)
	})
}

// Test email provider with SparkPost settings
func TestEmailProvider_WithSparkPostSettings(t *testing.T) {
	passphrase := "test-passphrase"

	t.Run("SparkPost provider encryption/decryption", func(t *testing.T) {
		provider := domain.EmailProvider{
			Kind:               domain.EmailProviderKindSparkPost,
			RateLimitPerMinute: 25,
			Senders: []domain.EmailSender{
				domain.NewEmailSender("default@example.com", "Default Sender"),
			},
			SparkPost: &domain.SparkPostSettings{
				APIKey:      "test-api-key",
				Endpoint:    "https://api.sparkpost.com/api/v1",
				SandboxMode: false,
			},
		}

		// Test encryption
		err := provider.EncryptSecretKeys(passphrase)
		assert.NoError(t, err)
		assert.NotEmpty(t, provider.SparkPost.EncryptedAPIKey)
		assert.Empty(t, provider.SparkPost.APIKey)

		// Test decryption
		err = provider.DecryptSecretKeys(passphrase)
		assert.NoError(t, err)
		assert.Equal(t, "test-api-key", provider.SparkPost.APIKey)
	})

	t.Run("SparkPost provider validation", func(t *testing.T) {
		// Valid provider
		provider := domain.EmailProvider{
			Kind:               domain.EmailProviderKindSparkPost,
			RateLimitPerMinute: 25,
			Senders: []domain.EmailSender{
				domain.NewEmailSender("default@example.com", "Default Sender"),
			},
			SparkPost: &domain.SparkPostSettings{
				APIKey:      "test-api-key",
				Endpoint:    "https://api.sparkpost.com/api/v1",
				SandboxMode: false,
			},
		}

		// Should validate without error
		err := provider.Validate(passphrase)
		assert.NoError(t, err)
		assert.NotEmpty(t, provider.SparkPost.EncryptedAPIKey)

		// Provider with missing SparkPost settings
		invalidProvider := domain.EmailProvider{
			Kind:               domain.EmailProviderKindSparkPost,
			RateLimitPerMinute: 25,
			Senders: []domain.EmailSender{
				domain.NewEmailSender("default@example.com", "Default Sender"),
			},
		}

		// Should fail validation
		err = invalidProvider.Validate(passphrase)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "SparkPost settings required")
	})

	t.Run("SparkPost provider with missing endpoint", func(t *testing.T) {
		provider := domain.EmailProvider{
			Kind:               domain.EmailProviderKindSparkPost,
			RateLimitPerMinute: 25,
			Senders: []domain.EmailSender{
				domain.NewEmailSender("default@example.com", "Default Sender"),
			},
			SparkPost: &domain.SparkPostSettings{
				APIKey:      "test-api-key",
				SandboxMode: false,
			},
		}

		// Should fail validation due to missing endpoint
		err := provider.Validate(passphrase)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "endpoint is required")
	})
}
