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

func TestAmazonSESSettings_DecryptSecretKey(t *testing.T) {
	// Setup
	passphrase := "test-passphrase"
	secretKey := "test-secret-key"

	// Create encrypted key
	encryptedKey, err := crypto.EncryptString(secretKey, passphrase)
	require.NoError(t, err)

	settings := domain.AmazonSESSettings{
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
	assert.Contains(t, err.Error(), "failed to decrypt SES secret key")
}

func TestAmazonSESSettings_EncryptSecretKey(t *testing.T) {
	// Setup
	passphrase := "test-passphrase"
	secretKey := "test-secret-key"

	settings := domain.AmazonSESSettings{
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

func TestAmazonSESSettings_Validate(t *testing.T) {
	passphrase := "test-passphrase"

	t.Run("Valid settings", func(t *testing.T) {
		settings := domain.AmazonSESSettings{
			Region:    "us-east-1",
			AccessKey: "test-access-key",
			SecretKey: "test-secret-key",
		}

		err := settings.Validate(passphrase)
		require.NoError(t, err)
		assert.NotEmpty(t, settings.EncryptedSecretKey)
		assert.Equal(t, "test-secret-key", settings.SecretKey)

		// Verify encryption happened correctly
		decrypted, err := crypto.DecryptFromHexString(settings.EncryptedSecretKey, passphrase)
		require.NoError(t, err)
		assert.Equal(t, "test-secret-key", decrypted)
	})

	t.Run("Missing region", func(t *testing.T) {
		settings := domain.AmazonSESSettings{
			AccessKey: "test-access-key",
			SecretKey: "test-secret-key",
		}

		err := settings.Validate(passphrase)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "region is required")
	})

	t.Run("Missing access key", func(t *testing.T) {
		settings := domain.AmazonSESSettings{
			Region:    "us-east-1",
			SecretKey: "test-secret-key",
		}

		err := settings.Validate(passphrase)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "access key is required")
	})

	t.Run("No secret key to encrypt", func(t *testing.T) {
		settings := domain.AmazonSESSettings{
			Region:    "us-east-1",
			AccessKey: "test-access-key",
		}

		err := settings.Validate(passphrase)
		assert.NoError(t, err)
		assert.Empty(t, settings.EncryptedSecretKey)
	})

	t.Run("Empty settings", func(t *testing.T) {
		// When no fields are set, validation should pass (optional config)
		settings := domain.AmazonSESSettings{}

		err := settings.Validate(passphrase)
		assert.NoError(t, err)
	})
}

func TestSESServiceInterface(t *testing.T) {
	// Setup
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockService := mocks.NewMockSESServiceInterface(ctrl)
	ctx := context.Background()

	config := domain.AmazonSESSettings{
		Region:    "us-east-1",
		AccessKey: "test-access-key",
		SecretKey: "test-secret-key",
	}

	configSetName := "test-config-set"
	destinationName := "test-destination"

	topicConfig := domain.SESTopicConfig{
		TopicName:            "test-topic",
		NotificationEndpoint: "https://example.com/webhook",
		Protocol:             "https",
	}

	// Expected responses
	topicARN := "arn:aws:sns:us-east-1:123456789012:test-topic"

	destinations := []domain.SESConfigurationSetEventDestination{
		{
			Name:                 destinationName,
			ConfigurationSetName: configSetName,
			Enabled:              true,
			MatchingEventTypes:   []string{"send", "bounce", "complaint"},
			SNSDestination:       &topicConfig,
		},
	}

	configurationSets := []string{configSetName, "default"}

	t.Run("ListConfigurationSets", func(t *testing.T) {
		// Set expectations
		mockService.EXPECT().ListConfigurationSets(gomock.Any(), gomock.Eq(config)).Return(configurationSets, nil)

		// Call the method
		response, err := mockService.ListConfigurationSets(ctx, config)

		// Assert
		require.NoError(t, err)
		assert.Equal(t, configurationSets, response)
		assert.Contains(t, response, configSetName)
	})

	t.Run("CreateConfigurationSet", func(t *testing.T) {
		// Set expectations
		mockService.EXPECT().CreateConfigurationSet(gomock.Any(), gomock.Eq(config), gomock.Eq(configSetName)).Return(nil)

		// Call the method
		err := mockService.CreateConfigurationSet(ctx, config, configSetName)

		// Assert
		require.NoError(t, err)
	})

	t.Run("DeleteConfigurationSet", func(t *testing.T) {
		// Set expectations
		mockService.EXPECT().DeleteConfigurationSet(gomock.Any(), gomock.Eq(config), gomock.Eq(configSetName)).Return(nil)

		// Call the method
		err := mockService.DeleteConfigurationSet(ctx, config, configSetName)

		// Assert
		require.NoError(t, err)
	})

	t.Run("CreateSNSTopic", func(t *testing.T) {
		// Set expectations
		mockService.EXPECT().CreateSNSTopic(gomock.Any(), gomock.Eq(config), gomock.Eq(topicConfig)).Return(topicARN, nil)

		// Call the method
		response, err := mockService.CreateSNSTopic(ctx, config, topicConfig)

		// Assert
		require.NoError(t, err)
		assert.Equal(t, topicARN, response)
	})

	t.Run("DeleteSNSTopic", func(t *testing.T) {
		// Set expectations
		mockService.EXPECT().DeleteSNSTopic(gomock.Any(), gomock.Eq(config), gomock.Eq(topicARN)).Return(nil)

		// Call the method
		err := mockService.DeleteSNSTopic(ctx, config, topicARN)

		// Assert
		require.NoError(t, err)
	})

	t.Run("CreateEventDestination", func(t *testing.T) {
		// Set up the event destination
		destination := domain.SESConfigurationSetEventDestination{
			Name:                 destinationName,
			ConfigurationSetName: configSetName,
			Enabled:              true,
			MatchingEventTypes:   []string{"send", "bounce", "complaint"},
			SNSDestination:       &topicConfig,
		}

		// Set expectations
		mockService.EXPECT().CreateEventDestination(gomock.Any(), gomock.Eq(config), gomock.Eq(destination)).Return(nil)

		// Call the method
		err := mockService.CreateEventDestination(ctx, config, destination)

		// Assert
		require.NoError(t, err)
	})

	t.Run("UpdateEventDestination", func(t *testing.T) {
		// Set up the event destination
		destination := domain.SESConfigurationSetEventDestination{
			Name:                 destinationName,
			ConfigurationSetName: configSetName,
			Enabled:              true,
			MatchingEventTypes:   []string{"send", "bounce", "complaint", "reject"},
			SNSDestination:       &topicConfig,
		}

		// Set expectations
		mockService.EXPECT().UpdateEventDestination(gomock.Any(), gomock.Eq(config), gomock.Eq(destination)).Return(nil)

		// Call the method
		err := mockService.UpdateEventDestination(ctx, config, destination)

		// Assert
		require.NoError(t, err)
	})

	t.Run("DeleteEventDestination", func(t *testing.T) {
		// Set expectations
		mockService.EXPECT().DeleteEventDestination(gomock.Any(), gomock.Eq(config), gomock.Eq(configSetName), gomock.Eq(destinationName)).Return(nil)

		// Call the method
		err := mockService.DeleteEventDestination(ctx, config, configSetName, destinationName)

		// Assert
		require.NoError(t, err)
	})

	t.Run("ListEventDestinations", func(t *testing.T) {
		// Set expectations
		mockService.EXPECT().ListEventDestinations(gomock.Any(), gomock.Eq(config), gomock.Eq(configSetName)).Return(destinations, nil)

		// Call the method
		response, err := mockService.ListEventDestinations(ctx, config, configSetName)

		// Assert
		require.NoError(t, err)
		assert.Equal(t, destinations, response)
		if len(response) > 0 {
			assert.Equal(t, destinationName, response[0].Name)
		}
	})
}

// Test email provider with SES settings
func TestEmailProvider_WithSESSettings(t *testing.T) {
	passphrase := "test-passphrase"

	t.Run("SES provider encryption/decryption", func(t *testing.T) {
		provider := domain.EmailProvider{
			Kind:               domain.EmailProviderKindSES,
			RateLimitPerMinute: 25,
			Senders: []domain.EmailSender{
				domain.NewEmailSender("default@example.com", "Default Sender"),
			},
			SES: &domain.AmazonSESSettings{
				Region:    "us-east-1",
				AccessKey: "test-access-key",
				SecretKey: "test-secret-key",
			},
		}

		// Test encryption
		err := provider.EncryptSecretKeys(passphrase)
		assert.NoError(t, err)
		assert.NotEmpty(t, provider.SES.EncryptedSecretKey)
		assert.Empty(t, provider.SES.SecretKey)

		// Test decryption
		err = provider.DecryptSecretKeys(passphrase)
		assert.NoError(t, err)
		assert.Equal(t, "test-secret-key", provider.SES.SecretKey)
	})

	t.Run("SES provider validation", func(t *testing.T) {
		// Valid provider
		provider := domain.EmailProvider{
			Kind:               domain.EmailProviderKindSES,
			RateLimitPerMinute: 25,
			Senders: []domain.EmailSender{
				domain.NewEmailSender("default@example.com", "Default Sender"),
			},
			SES: &domain.AmazonSESSettings{
				Region:    "us-east-1",
				AccessKey: "test-access-key",
				SecretKey: "test-secret-key",
			},
		}

		// Should validate without error
		err := provider.Validate(passphrase)
		assert.NoError(t, err)
		assert.NotEmpty(t, provider.SES.EncryptedSecretKey)

		// Provider with missing SES settings
		invalidProvider := domain.EmailProvider{
			Kind:               domain.EmailProviderKindSES,
			RateLimitPerMinute: 25,
			Senders: []domain.EmailSender{
				domain.NewEmailSender("default@example.com", "Default Sender"),
			},
		}

		// Should fail validation
		err = invalidProvider.Validate(passphrase)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "SES settings required")
	})

	t.Run("SES provider with missing region", func(t *testing.T) {
		provider := domain.EmailProvider{
			Kind:               domain.EmailProviderKindSES,
			RateLimitPerMinute: 25,
			Senders: []domain.EmailSender{
				{
					ID:    "123e4567-e89b-12d3-a456-426614174000",
					Email: "default@example.com",
					Name:  "Default Sender",
				},
			},
			SES: &domain.AmazonSESSettings{
				AccessKey: "test-access-key",
				SecretKey: "test-secret-key",
			},
		}

		// Should fail validation due to missing region
		err := provider.Validate(passphrase)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "region is required")
	})

	t.Run("SES provider with missing access key", func(t *testing.T) {
		provider := domain.EmailProvider{
			Kind:               domain.EmailProviderKindSES,
			RateLimitPerMinute: 25,
			Senders: []domain.EmailSender{
				domain.NewEmailSender("default@example.com", "Default Sender"),
			},
			SES: &domain.AmazonSESSettings{
				Region:    "us-east-1",
				SecretKey: "test-secret-key",
			},
		}

		// Should fail validation due to missing access key
		err := provider.Validate(passphrase)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "access key is required")
	})
}
