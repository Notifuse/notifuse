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

func TestMailjetSettings_DecryptAPIKey(t *testing.T) {
	// Setup
	passphrase := "test-passphrase"
	apiKey := "test-api-key"

	// Create encrypted key
	encryptedKey, err := crypto.EncryptString(apiKey, passphrase)
	require.NoError(t, err)

	settings := domain.MailjetSettings{
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
	assert.Contains(t, err.Error(), "failed to decrypt Mailjet API key")
}

func TestMailjetSettings_EncryptAPIKey(t *testing.T) {
	// Setup
	passphrase := "test-passphrase"
	apiKey := "test-api-key"

	settings := domain.MailjetSettings{
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

func TestMailjetSettings_DecryptSecretKey(t *testing.T) {
	// Setup
	passphrase := "test-passphrase"
	secretKey := "test-secret-key"

	// Create encrypted key
	encryptedKey, err := crypto.EncryptString(secretKey, passphrase)
	require.NoError(t, err)

	settings := domain.MailjetSettings{
		EncryptedSecretKey: encryptedKey,
	}

	// Test successful decryption
	err = settings.DecryptSecretKey(passphrase)
	require.NoError(t, err)
	assert.Equal(t, secretKey, settings.SecretKey)

	// Test with invalid passphrase
	settings.SecretKey = ""
	err = settings.DecryptSecretKey("wrong-passphrase")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to decrypt Mailjet Secret key")
}

func TestMailjetSettings_EncryptSecretKey(t *testing.T) {
	// Setup
	passphrase := "test-passphrase"
	secretKey := "test-secret-key"

	settings := domain.MailjetSettings{
		SecretKey: secretKey,
	}

	// Test
	err := settings.EncryptSecretKey(passphrase)
	require.NoError(t, err)
	assert.NotEmpty(t, settings.EncryptedSecretKey)
	assert.NotEqual(t, secretKey, settings.EncryptedSecretKey)

	// Verify by decrypting
	decrypted, err := crypto.DecryptFromHexString(settings.EncryptedSecretKey, passphrase)
	require.NoError(t, err)
	assert.Equal(t, secretKey, decrypted)
}

func TestMailjetSettings_Validate(t *testing.T) {
	passphrase := "test-passphrase"

	t.Run("Valid settings", func(t *testing.T) {
		settings := domain.MailjetSettings{
			APIKey:      "test-api-key",
			SecretKey:   "test-secret-key",
			SandboxMode: false,
		}

		err := settings.Validate(passphrase)
		require.NoError(t, err)
		assert.NotEmpty(t, settings.EncryptedAPIKey)
		assert.NotEmpty(t, settings.EncryptedSecretKey)
		assert.Empty(t, settings.APIKey)
		assert.Empty(t, settings.SecretKey)

		// Verify encryption happened correctly
		decryptedAPIKey, err := crypto.DecryptFromHexString(settings.EncryptedAPIKey, passphrase)
		require.NoError(t, err)
		assert.Equal(t, "test-api-key", decryptedAPIKey)

		decryptedSecretKey, err := crypto.DecryptFromHexString(settings.EncryptedSecretKey, passphrase)
		require.NoError(t, err)
		assert.Equal(t, "test-secret-key", decryptedSecretKey)
	})

	t.Run("No keys to encrypt", func(t *testing.T) {
		settings := domain.MailjetSettings{
			SandboxMode: true,
		}

		err := settings.Validate(passphrase)
		assert.NoError(t, err)
		assert.Empty(t, settings.EncryptedAPIKey)
		assert.Empty(t, settings.EncryptedSecretKey)
	})
}

func TestMailjetServiceInterface(t *testing.T) {
	// Setup
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockService := mocks.NewMockMailjetServiceInterface(ctrl)
	ctx := context.Background()

	config := domain.MailjetSettings{
		APIKey:      "test-api-key",
		SecretKey:   "test-secret-key",
		SandboxMode: false,
	}

	webhookID := int64(123)

	webhook := domain.MailjetWebhook{
		Endpoint:  "https://example.com/webhook",
		EventType: string(domain.MailjetEventBounce),
		Status:    "alive",
		Version:   1,
	}

	// Expected responses
	expectedWebhook := domain.MailjetWebhook{
		ID:        webhookID,
		APIKey:    config.APIKey,
		Endpoint:  webhook.Endpoint,
		EventType: webhook.EventType,
		Status:    webhook.Status,
		Version:   webhook.Version,
	}

	expectedWebhookResponse := &domain.MailjetWebhookResponse{
		Count: 1,
		Data:  []domain.MailjetWebhook{expectedWebhook},
		Total: 1,
	}

	t.Run("ListWebhooks", func(t *testing.T) {
		// Set expectations
		mockService.EXPECT().ListWebhooks(gomock.Any(), gomock.Eq(config)).Return(expectedWebhookResponse, nil)

		// Call the method
		response, err := mockService.ListWebhooks(ctx, config)

		// Assert
		require.NoError(t, err)
		assert.Equal(t, expectedWebhookResponse, response)
	})

	t.Run("CreateWebhook", func(t *testing.T) {
		// Set expectations
		mockService.EXPECT().CreateWebhook(gomock.Any(), gomock.Eq(config), gomock.Eq(webhook)).Return(&expectedWebhook, nil)

		// Call the method
		response, err := mockService.CreateWebhook(ctx, config, webhook)

		// Assert
		require.NoError(t, err)
		assert.Equal(t, &expectedWebhook, response)
	})

	t.Run("GetWebhook", func(t *testing.T) {
		// Set expectations
		mockService.EXPECT().GetWebhook(gomock.Any(), gomock.Eq(config), gomock.Eq(webhookID)).Return(&expectedWebhook, nil)

		// Call the method
		response, err := mockService.GetWebhook(ctx, config, webhookID)

		// Assert
		require.NoError(t, err)
		assert.Equal(t, &expectedWebhook, response)
	})

	t.Run("UpdateWebhook", func(t *testing.T) {
		// Set expectations
		mockService.EXPECT().UpdateWebhook(gomock.Any(), gomock.Eq(config), gomock.Eq(webhookID), gomock.Eq(webhook)).Return(&expectedWebhook, nil)

		// Call the method
		response, err := mockService.UpdateWebhook(ctx, config, webhookID, webhook)

		// Assert
		require.NoError(t, err)
		assert.Equal(t, &expectedWebhook, response)
	})

	t.Run("DeleteWebhook", func(t *testing.T) {
		// Set expectations
		mockService.EXPECT().DeleteWebhook(gomock.Any(), gomock.Eq(config), gomock.Eq(webhookID)).Return(nil)

		// Call the method
		err := mockService.DeleteWebhook(ctx, config, webhookID)

		// Assert
		require.NoError(t, err)
	})
}

// Test email provider with Mailjet settings
func TestEmailProvider_WithMailjetSettings(t *testing.T) {
	passphrase := "test-passphrase"

	t.Run("Mailjet provider encryption/decryption", func(t *testing.T) {
		provider := domain.EmailProvider{
			Kind:               domain.EmailProviderKindMailjet,
			RateLimitPerMinute: 25,
			Senders: []domain.EmailSender{
				domain.NewEmailSender("default@example.com", "Default Sender"),
			},
			Mailjet: &domain.MailjetSettings{
				APIKey:      "test-api-key",
				SecretKey:   "test-secret-key",
				SandboxMode: false,
			},
		}

		// Test encryption
		err := provider.EncryptSecretKeys(passphrase)
		assert.NoError(t, err)
		assert.NotEmpty(t, provider.Mailjet.EncryptedAPIKey)
		assert.NotEmpty(t, provider.Mailjet.EncryptedSecretKey)
		assert.Empty(t, provider.Mailjet.APIKey)
		assert.Empty(t, provider.Mailjet.SecretKey)

		// Test decryption
		err = provider.DecryptSecretKeys(passphrase)
		assert.NoError(t, err)
		assert.Equal(t, "test-api-key", provider.Mailjet.APIKey)
		assert.Equal(t, "test-secret-key", provider.Mailjet.SecretKey)
	})

	t.Run("Mailjet provider validation", func(t *testing.T) {
		// Valid provider
		provider := domain.EmailProvider{
			Kind:               domain.EmailProviderKindMailjet,
			RateLimitPerMinute: 25,
			Senders: []domain.EmailSender{
				domain.NewEmailSender("default@example.com", "Default Sender"),
			},
			Mailjet: &domain.MailjetSettings{
				APIKey:      "test-api-key",
				SecretKey:   "test-secret-key",
				SandboxMode: false,
			},
		}

		// Should validate without error
		err := provider.Validate(passphrase)
		assert.NoError(t, err)
		assert.NotEmpty(t, provider.Mailjet.EncryptedAPIKey)
		assert.NotEmpty(t, provider.Mailjet.EncryptedSecretKey)

		// Provider with missing Mailjet settings
		invalidProvider := domain.EmailProvider{
			Kind:               domain.EmailProviderKindMailjet,
			RateLimitPerMinute: 25,
			Senders: []domain.EmailSender{
				domain.NewEmailSender("default@example.com", "Default Sender"),
			},
		}

		// Should fail validation
		err = invalidProvider.Validate(passphrase)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "mailjet settings required")
	})
}
