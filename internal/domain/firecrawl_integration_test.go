package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFirecrawlSettings_Validate(t *testing.T) {
	passphrase := "test-passphrase-32-chars-long!!"

	t.Run("empty API key returns error", func(t *testing.T) {
		settings := &FirecrawlSettings{}
		err := settings.Validate(passphrase)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "API key is required")
	})

	t.Run("valid API key is encrypted", func(t *testing.T) {
		settings := &FirecrawlSettings{
			APIKey: "fc-test-api-key-12345",
		}
		err := settings.Validate(passphrase)
		require.NoError(t, err)
		assert.NotEmpty(t, settings.EncryptedAPIKey)
	})

	t.Run("already encrypted key passes validation", func(t *testing.T) {
		settings := &FirecrawlSettings{
			APIKey: "fc-test-api-key-12345",
		}
		err := settings.Validate(passphrase)
		require.NoError(t, err)

		// Create new settings with only encrypted key
		newSettings := &FirecrawlSettings{
			EncryptedAPIKey: settings.EncryptedAPIKey,
		}
		err = newSettings.Validate(passphrase)
		assert.NoError(t, err)
	})
}

func TestFirecrawlSettings_EncryptDecryptAPIKey(t *testing.T) {
	passphrase := "test-passphrase-32-chars-long!!"

	t.Run("encrypt and decrypt round trip", func(t *testing.T) {
		originalKey := "fc-test-api-key-12345"
		settings := &FirecrawlSettings{
			APIKey: originalKey,
		}

		// Encrypt
		err := settings.EncryptAPIKey(passphrase)
		require.NoError(t, err)
		assert.NotEmpty(t, settings.EncryptedAPIKey)

		// Clear the plaintext key
		settings.APIKey = ""

		// Decrypt
		err = settings.DecryptAPIKey(passphrase)
		require.NoError(t, err)
		assert.Equal(t, originalKey, settings.APIKey)
	})

	t.Run("empty API key does not encrypt", func(t *testing.T) {
		settings := &FirecrawlSettings{}
		err := settings.EncryptAPIKey(passphrase)
		assert.NoError(t, err)
		assert.Empty(t, settings.EncryptedAPIKey)
	})

	t.Run("empty encrypted key does not decrypt", func(t *testing.T) {
		settings := &FirecrawlSettings{}
		err := settings.DecryptAPIKey(passphrase)
		assert.NoError(t, err)
		assert.Empty(t, settings.APIKey)
	})
}

func TestFirecrawlSettings_EncryptDecryptSecretKeys(t *testing.T) {
	passphrase := "test-passphrase-32-chars-long!!"

	t.Run("encrypt and decrypt via wrapper methods", func(t *testing.T) {
		originalKey := "fc-test-api-key-12345"
		settings := &FirecrawlSettings{
			APIKey: originalKey,
		}

		// Encrypt using wrapper
		err := settings.EncryptSecretKeys(passphrase)
		require.NoError(t, err)
		assert.NotEmpty(t, settings.EncryptedAPIKey)
		assert.Empty(t, settings.APIKey) // Should be cleared after encryption

		// Decrypt using wrapper
		err = settings.DecryptSecretKeys(passphrase)
		require.NoError(t, err)
		assert.Equal(t, originalKey, settings.APIKey)
	})
}

func TestFirecrawlSettings_GetBaseURL(t *testing.T) {
	t.Run("returns default URL when not set", func(t *testing.T) {
		settings := &FirecrawlSettings{}
		assert.Equal(t, "https://api.firecrawl.dev", settings.GetBaseURL())
	})

	t.Run("returns custom URL when set", func(t *testing.T) {
		customURL := "https://my-firecrawl.example.com"
		settings := &FirecrawlSettings{
			BaseURL: customURL,
		}
		assert.Equal(t, customURL, settings.GetBaseURL())
	})
}

func TestIntegration_Firecrawl_Validate(t *testing.T) {
	passphrase := "test-passphrase-32-chars-long!!"

	t.Run("valid firecrawl integration", func(t *testing.T) {
		integration := &Integration{
			ID:   "int_firecrawl_1",
			Name: "My Firecrawl",
			Type: IntegrationTypeFirecrawl,
			FirecrawlSettings: &FirecrawlSettings{
				APIKey: "fc-test-api-key",
			},
		}
		err := integration.Validate(passphrase)
		assert.NoError(t, err)
	})

	t.Run("missing firecrawl settings returns error", func(t *testing.T) {
		integration := &Integration{
			ID:   "int_firecrawl_1",
			Name: "My Firecrawl",
			Type: IntegrationTypeFirecrawl,
		}
		err := integration.Validate(passphrase)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "firecrawl settings are required")
	})

	t.Run("invalid firecrawl settings returns error", func(t *testing.T) {
		integration := &Integration{
			ID:                "int_firecrawl_1",
			Name:              "My Firecrawl",
			Type:              IntegrationTypeFirecrawl,
			FirecrawlSettings: &FirecrawlSettings{}, // No API key
		}
		err := integration.Validate(passphrase)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid firecrawl settings")
	})
}

func TestIntegration_Firecrawl_BeforeSave(t *testing.T) {
	secretKey := "test-secret-key-32-chars-long!!"

	t.Run("encrypts API key before save", func(t *testing.T) {
		integration := &Integration{
			ID:   "int_firecrawl_1",
			Name: "My Firecrawl",
			Type: IntegrationTypeFirecrawl,
			FirecrawlSettings: &FirecrawlSettings{
				APIKey: "fc-test-api-key",
			},
		}

		err := integration.BeforeSave(secretKey)
		require.NoError(t, err)
		assert.NotEmpty(t, integration.FirecrawlSettings.EncryptedAPIKey)
		assert.Empty(t, integration.FirecrawlSettings.APIKey)
	})
}

func TestIntegration_Firecrawl_AfterLoad(t *testing.T) {
	secretKey := "test-secret-key-32-chars-long!!"

	t.Run("decrypts API key after load", func(t *testing.T) {
		originalKey := "fc-test-api-key"
		integration := &Integration{
			ID:   "int_firecrawl_1",
			Name: "My Firecrawl",
			Type: IntegrationTypeFirecrawl,
			FirecrawlSettings: &FirecrawlSettings{
				APIKey: originalKey,
			},
		}

		// Encrypt first
		err := integration.BeforeSave(secretKey)
		require.NoError(t, err)

		// Decrypt
		err = integration.AfterLoad(secretKey)
		require.NoError(t, err)
		assert.Equal(t, originalKey, integration.FirecrawlSettings.APIKey)
	})
}

func TestCreateIntegrationRequest_Firecrawl_Validate(t *testing.T) {
	passphrase := "test-passphrase-32-chars-long!!"

	t.Run("valid firecrawl request", func(t *testing.T) {
		req := &CreateIntegrationRequest{
			WorkspaceID: "workspace1",
			Name:        "My Firecrawl",
			Type:        IntegrationTypeFirecrawl,
			FirecrawlSettings: &FirecrawlSettings{
				APIKey: "fc-test-api-key",
			},
		}
		err := req.Validate(passphrase)
		assert.NoError(t, err)
	})

	t.Run("missing firecrawl settings returns error", func(t *testing.T) {
		req := &CreateIntegrationRequest{
			WorkspaceID: "workspace1",
			Name:        "My Firecrawl",
			Type:        IntegrationTypeFirecrawl,
		}
		err := req.Validate(passphrase)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "firecrawl settings are required")
	})
}

func TestUpdateIntegrationRequest_Firecrawl_Validate(t *testing.T) {
	passphrase := "test-passphrase-32-chars-long!!"

	t.Run("valid firecrawl update request", func(t *testing.T) {
		req := &UpdateIntegrationRequest{
			WorkspaceID:   "workspace1",
			IntegrationID: "int_firecrawl_1",
			Name:          "My Firecrawl Updated",
			FirecrawlSettings: &FirecrawlSettings{
				APIKey: "fc-new-api-key",
			},
		}
		err := req.Validate(passphrase)
		assert.NoError(t, err)
	})

	t.Run("invalid firecrawl settings returns error", func(t *testing.T) {
		req := &UpdateIntegrationRequest{
			WorkspaceID:       "workspace1",
			IntegrationID:     "int_firecrawl_1",
			Name:              "My Firecrawl Updated",
			FirecrawlSettings: &FirecrawlSettings{}, // No API key
		}
		err := req.Validate(passphrase)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid firecrawl settings")
	})
}
