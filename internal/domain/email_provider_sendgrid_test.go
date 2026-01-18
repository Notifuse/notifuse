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

func TestSendGridSettings_DecryptAPIKey(t *testing.T) {
	// Setup
	passphrase := "test-passphrase"
	apiKey := "SG.test-api-key-12345"

	// Create encrypted key
	encryptedKey, err := crypto.EncryptString(apiKey, passphrase)
	require.NoError(t, err)

	settings := domain.SendGridSettings{
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
	assert.Contains(t, err.Error(), "failed to decrypt SendGrid API key")
}

func TestSendGridSettings_EncryptAPIKey(t *testing.T) {
	// Setup
	passphrase := "test-passphrase"
	apiKey := "SG.test-api-key-12345"

	settings := domain.SendGridSettings{
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

func TestSendGridSettings_Validate(t *testing.T) {
	passphrase := "test-passphrase"

	t.Run("Valid settings with API key", func(t *testing.T) {
		settings := domain.SendGridSettings{
			APIKey: "SG.test-api-key-12345",
		}

		err := settings.Validate(passphrase)
		require.NoError(t, err)
		assert.NotEmpty(t, settings.EncryptedAPIKey)

		// Verify encryption happened correctly
		decrypted, err := crypto.DecryptFromHexString(settings.EncryptedAPIKey, passphrase)
		require.NoError(t, err)
		assert.Equal(t, "SG.test-api-key-12345", decrypted)
	})

	t.Run("No API key to encrypt", func(t *testing.T) {
		settings := domain.SendGridSettings{}

		err := settings.Validate(passphrase)
		assert.NoError(t, err)
		assert.Empty(t, settings.EncryptedAPIKey)
	})
}

func TestSendGridServiceInterface(t *testing.T) {
	// Setup
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockService := mocks.NewMockSendGridServiceInterface(ctrl)
	ctx := context.Background()

	config := domain.SendGridSettings{
		APIKey: "SG.test-api-key-12345",
	}

	webhookSettings := domain.SendGridWebhookSettings{
		Enabled:    true,
		URL:        "https://example.com/webhooks/email?provider=sendgrid",
		Delivered:  true,
		Bounce:     true,
		SpamReport: true,
		Dropped:    true,
	}

	t.Run("GetWebhookSettings", func(t *testing.T) {
		// Set expectations
		mockService.EXPECT().GetWebhookSettings(gomock.Any(), gomock.Eq(config)).Return(&webhookSettings, nil)

		// Call the method
		response, err := mockService.GetWebhookSettings(ctx, config)

		// Assert
		require.NoError(t, err)
		assert.Equal(t, &webhookSettings, response)
		assert.True(t, response.Enabled)
		assert.True(t, response.Bounce)
	})

	t.Run("UpdateWebhookSettings", func(t *testing.T) {
		// Set expectations
		mockService.EXPECT().UpdateWebhookSettings(gomock.Any(), gomock.Eq(config), gomock.Eq(webhookSettings)).Return(nil)

		// Call the method
		err := mockService.UpdateWebhookSettings(ctx, config, webhookSettings)

		// Assert
		require.NoError(t, err)
	})
}

// Test email provider with SendGrid settings
func TestEmailProvider_WithSendGridSettings(t *testing.T) {
	passphrase := "test-passphrase"

	t.Run("SendGrid provider encryption/decryption", func(t *testing.T) {
		provider := domain.EmailProvider{
			Kind:               domain.EmailProviderKindSendGrid,
			RateLimitPerMinute: 100,
			Senders: []domain.EmailSender{
				domain.NewEmailSender("default@example.com", "Default Sender"),
			},
			SendGrid: &domain.SendGridSettings{
				APIKey: "SG.test-api-key-12345",
			},
		}

		// Test encryption
		err := provider.EncryptSecretKeys(passphrase)
		assert.NoError(t, err)
		assert.NotEmpty(t, provider.SendGrid.EncryptedAPIKey)
		assert.Empty(t, provider.SendGrid.APIKey)

		// Test decryption
		err = provider.DecryptSecretKeys(passphrase)
		assert.NoError(t, err)
		assert.Equal(t, "SG.test-api-key-12345", provider.SendGrid.APIKey)
	})

	t.Run("SendGrid provider validation", func(t *testing.T) {
		// Valid provider
		provider := domain.EmailProvider{
			Kind:               domain.EmailProviderKindSendGrid,
			RateLimitPerMinute: 100,
			Senders: []domain.EmailSender{
				domain.NewEmailSender("default@example.com", "Default Sender"),
			},
			SendGrid: &domain.SendGridSettings{
				APIKey: "SG.test-api-key-12345",
			},
		}

		// Should validate without error
		err := provider.Validate(passphrase)
		assert.NoError(t, err)
		assert.NotEmpty(t, provider.SendGrid.EncryptedAPIKey)
	})

	t.Run("Provider with missing SendGrid settings", func(t *testing.T) {
		invalidProvider := domain.EmailProvider{
			Kind:               domain.EmailProviderKindSendGrid,
			RateLimitPerMinute: 100,
			Senders: []domain.EmailSender{
				domain.NewEmailSender("default@example.com", "Default Sender"),
			},
		}

		// Should fail validation
		err := invalidProvider.Validate(passphrase)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "sendgrid settings required")
	})
}

func TestSendGridWebhookEvent(t *testing.T) {
	t.Run("Parse delivered event", func(t *testing.T) {
		event := domain.SendGridWebhookEvent{
			Email:             "recipient@example.com",
			Timestamp:         1706097600,
			Event:             "delivered",
			SGEventID:         "abc123",
			SGMessageID:       "14c5d75ce93.dfd.64b469",
			Response:          "250 2.0.0 OK",
			NotifuseMessageID: "msg_123",
		}

		assert.Equal(t, "delivered", event.Event)
		assert.Equal(t, "recipient@example.com", event.Email)
		assert.Equal(t, "msg_123", event.NotifuseMessageID)
	})

	t.Run("Parse bounce event", func(t *testing.T) {
		event := domain.SendGridWebhookEvent{
			Email:                "recipient@example.com",
			Timestamp:            1706097700,
			Event:                "bounce",
			SGEventID:            "def456",
			SGMessageID:          "14c5d75ce93.dfd.64b469",
			Reason:               "550 5.1.1 The email account does not exist.",
			Status:               "5.1.1",
			Type:                 "bounce",
			BounceClassification: "Invalid Address",
			NotifuseMessageID:    "msg_123",
		}

		assert.Equal(t, "bounce", event.Event)
		assert.Equal(t, "bounce", event.Type)
		assert.Equal(t, "Invalid Address", event.BounceClassification)
		assert.Equal(t, "5.1.1", event.Status)
	})

	t.Run("Parse spam report event", func(t *testing.T) {
		event := domain.SendGridWebhookEvent{
			Email:             "recipient@example.com",
			Timestamp:         1706099000,
			Event:             "spamreport",
			SGEventID:         "ghi789",
			SGMessageID:       "14c5d75ce93.dfd.64b469",
			NotifuseMessageID: "msg_123",
		}

		assert.Equal(t, "spamreport", event.Event)
		assert.Equal(t, "msg_123", event.NotifuseMessageID)
	})

	t.Run("Parse blocked event (soft bounce)", func(t *testing.T) {
		event := domain.SendGridWebhookEvent{
			Email:                "recipient@example.com",
			Timestamp:            1706097800,
			Event:                "blocked",
			SGEventID:            "jkl012",
			SGMessageID:          "14c5d75ce93.dfd.64b469",
			Reason:               "Mailbox full",
			Type:                 "blocked",
			BounceClassification: "Mailbox Unavailable",
		}

		assert.Equal(t, "blocked", event.Event)
		assert.Equal(t, "blocked", event.Type)
		assert.Equal(t, "Mailbox Unavailable", event.BounceClassification)
	})
}

func TestSendGridWebhookSettings(t *testing.T) {
	settings := domain.SendGridWebhookSettings{
		Enabled:     true,
		URL:         "https://example.com/webhooks/email?provider=sendgrid",
		Delivered:   true,
		Bounce:      true,
		SpamReport:  true,
		Dropped:     true,
		Deferred:    true,
		Open:        false,
		Click:       false,
		Unsubscribe: false,
	}

	assert.True(t, settings.Enabled)
	assert.True(t, settings.Delivered)
	assert.True(t, settings.Bounce)
	assert.True(t, settings.SpamReport)
	assert.False(t, settings.Open)
	assert.False(t, settings.Click)
}
