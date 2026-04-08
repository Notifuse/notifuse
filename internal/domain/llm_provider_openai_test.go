package domain_test

import (
	"testing"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenAISettings_EncryptAPIKey(t *testing.T) {
	passphrase := "test-passphrase"
	apiKey := "sk-proj-test-key"

	settings := domain.OpenAISettings{
		APIKey: apiKey,
		Model:  "gpt-4o",
	}

	// Test encryption
	err := settings.EncryptAPIKey(passphrase)
	require.NoError(t, err)
	assert.NotEmpty(t, settings.EncryptedAPIKey)

	// Verify by decrypting directly
	decrypted, err := crypto.DecryptFromHexString(settings.EncryptedAPIKey, passphrase)
	require.NoError(t, err)
	assert.Equal(t, apiKey, decrypted)
}

func TestOpenAISettings_DecryptAPIKey(t *testing.T) {
	passphrase := "test-passphrase"
	apiKey := "sk-proj-test-key"

	// Create encrypted key
	encryptedKey, err := crypto.EncryptString(apiKey, passphrase)
	require.NoError(t, err)

	settings := domain.OpenAISettings{
		EncryptedAPIKey: encryptedKey,
		Model:           "gpt-4o",
	}

	// Test decryption
	err = settings.DecryptAPIKey(passphrase)
	require.NoError(t, err)
	assert.Equal(t, apiKey, settings.APIKey)

	// Test with wrong passphrase
	settings.APIKey = ""
	err = settings.DecryptAPIKey("wrong-passphrase")
	assert.Error(t, err)
}

func TestOpenAISettings_Validate(t *testing.T) {
	passphrase := "test-passphrase"

	t.Run("valid settings", func(t *testing.T) {
		settings := domain.OpenAISettings{
			APIKey: "sk-proj-test-key",
			Model:  "gpt-4o",
		}

		err := settings.Validate(passphrase)
		require.NoError(t, err)
		// API key should be encrypted after validation
		assert.NotEmpty(t, settings.EncryptedAPIKey)
	})

	t.Run("valid settings with base URL", func(t *testing.T) {
		settings := domain.OpenAISettings{
			APIKey:  "sk-proj-test-key",
			Model:   "custom-model",
			BaseURL: "http://localhost:11434/v1",
		}

		err := settings.Validate(passphrase)
		require.NoError(t, err)
		assert.NotEmpty(t, settings.EncryptedAPIKey)
		assert.Equal(t, "http://localhost:11434/v1", settings.BaseURL)
	})

	t.Run("missing model", func(t *testing.T) {
		settings := domain.OpenAISettings{
			APIKey: "sk-proj-test-key",
		}

		err := settings.Validate(passphrase)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "model is required")
	})

	t.Run("empty API key is allowed", func(t *testing.T) {
		// This allows updating settings without providing a new API key
		settings := domain.OpenAISettings{
			Model: "gpt-4o",
		}

		err := settings.Validate(passphrase)
		require.NoError(t, err)
		assert.Empty(t, settings.EncryptedAPIKey)
	})

	t.Run("any model name is allowed", func(t *testing.T) {
		// Model is free text - no validation on specific model names
		settings := domain.OpenAISettings{
			APIKey: "sk-proj-test-key",
			Model:  "my-custom-llama-model",
		}

		err := settings.Validate(passphrase)
		require.NoError(t, err)
	})
}
