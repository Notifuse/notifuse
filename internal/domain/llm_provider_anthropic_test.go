package domain_test

import (
	"testing"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAnthropicSettings_EncryptAPIKey(t *testing.T) {
	passphrase := "test-passphrase"
	apiKey := "sk-ant-api03-test-key"

	settings := domain.AnthropicSettings{
		APIKey: apiKey,
		Model:  "claude-opus-4-5-20251101",
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

func TestAnthropicSettings_DecryptAPIKey(t *testing.T) {
	passphrase := "test-passphrase"
	apiKey := "sk-ant-api03-test-key"

	// Create encrypted key
	encryptedKey, err := crypto.EncryptString(apiKey, passphrase)
	require.NoError(t, err)

	settings := domain.AnthropicSettings{
		EncryptedAPIKey: encryptedKey,
		Model:           "claude-opus-4-5-20251101",
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

func TestAnthropicSettings_Validate(t *testing.T) {
	passphrase := "test-passphrase"

	t.Run("valid settings", func(t *testing.T) {
		settings := domain.AnthropicSettings{
			APIKey: "sk-ant-test-key",
			Model:  "claude-opus-4-5-20251101",
		}

		err := settings.Validate(passphrase)
		require.NoError(t, err)
		// API key should be encrypted after validation
		assert.NotEmpty(t, settings.EncryptedAPIKey)
	})

	t.Run("missing model", func(t *testing.T) {
		settings := domain.AnthropicSettings{
			APIKey: "sk-ant-test-key",
		}

		err := settings.Validate(passphrase)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "model is required")
	})

	t.Run("empty API key is allowed", func(t *testing.T) {
		// This allows updating settings without providing a new API key
		settings := domain.AnthropicSettings{
			Model: "claude-opus-4-5-20251101",
		}

		err := settings.Validate(passphrase)
		require.NoError(t, err)
		assert.Empty(t, settings.EncryptedAPIKey)
	})

	t.Run("any model name is allowed", func(t *testing.T) {
		// Model is free text - no validation on specific model names
		settings := domain.AnthropicSettings{
			APIKey: "sk-ant-test-key",
			Model:  "some-new-model-2025",
		}

		err := settings.Validate(passphrase)
		require.NoError(t, err)
	})
}
