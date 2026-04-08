package domain_test

import (
	"testing"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLLMProvider_Validate(t *testing.T) {
	passphrase := "test-passphrase"

	t.Run("valid Anthropic provider", func(t *testing.T) {
		provider := domain.LLMProvider{
			Kind: domain.LLMProviderKindAnthropic,
			Anthropic: &domain.AnthropicSettings{
				APIKey: "sk-ant-test-key",
				Model:  "claude-sonnet-4-20250514",
			},
		}

		err := provider.Validate(passphrase)
		require.NoError(t, err)
		// API key should be encrypted after validation
		assert.NotEmpty(t, provider.Anthropic.EncryptedAPIKey)
	})

	t.Run("missing kind", func(t *testing.T) {
		provider := domain.LLMProvider{
			Anthropic: &domain.AnthropicSettings{
				APIKey: "sk-ant-test-key",
				Model:  "claude-sonnet-4-20250514",
			},
		}

		err := provider.Validate(passphrase)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "LLM provider kind is required")
	})

	t.Run("invalid kind", func(t *testing.T) {
		provider := domain.LLMProvider{
			Kind: "invalid",
		}

		err := provider.Validate(passphrase)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid LLM provider kind")
	})

	t.Run("Anthropic kind without settings", func(t *testing.T) {
		provider := domain.LLMProvider{
			Kind: domain.LLMProviderKindAnthropic,
		}

		err := provider.Validate(passphrase)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "Anthropic settings required")
	})

	t.Run("valid OpenAI provider", func(t *testing.T) {
		provider := domain.LLMProvider{
			Kind: domain.LLMProviderKindOpenAI,
			OpenAI: &domain.OpenAISettings{
				APIKey: "sk-proj-test-key",
				Model:  "gpt-4o",
			},
		}

		err := provider.Validate(passphrase)
		require.NoError(t, err)
		assert.NotEmpty(t, provider.OpenAI.EncryptedAPIKey)
	})

	t.Run("valid OpenAI provider with base URL", func(t *testing.T) {
		provider := domain.LLMProvider{
			Kind: domain.LLMProviderKindOpenAI,
			OpenAI: &domain.OpenAISettings{
				APIKey:  "sk-proj-test-key",
				Model:   "custom-model",
				BaseURL: "http://localhost:11434/v1",
			},
		}

		err := provider.Validate(passphrase)
		require.NoError(t, err)
	})

	t.Run("OpenAI kind without settings", func(t *testing.T) {
		provider := domain.LLMProvider{
			Kind: domain.LLMProviderKindOpenAI,
		}

		err := provider.Validate(passphrase)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "OpenAI settings required")
	})
}

func TestLLMProvider_EncryptDecryptSecretKeys(t *testing.T) {
	passphrase := "test-passphrase"

	t.Run("encrypt and decrypt Anthropic API key", func(t *testing.T) {
		originalAPIKey := "sk-ant-test-key-12345"

		provider := domain.LLMProvider{
			Kind: domain.LLMProviderKindAnthropic,
			Anthropic: &domain.AnthropicSettings{
				APIKey: originalAPIKey,
				Model:  "claude-sonnet-4-20250514",
			},
		}

		// Encrypt
		err := provider.EncryptSecretKeys(passphrase)
		require.NoError(t, err)
		assert.NotEmpty(t, provider.Anthropic.EncryptedAPIKey)
		assert.Empty(t, provider.Anthropic.APIKey) // Should be cleared after encryption

		// Decrypt
		err = provider.DecryptSecretKeys(passphrase)
		require.NoError(t, err)
		assert.Equal(t, originalAPIKey, provider.Anthropic.APIKey)
	})

	t.Run("encrypt with empty API key does nothing", func(t *testing.T) {
		provider := domain.LLMProvider{
			Kind: domain.LLMProviderKindAnthropic,
			Anthropic: &domain.AnthropicSettings{
				Model: "claude-sonnet-4-20250514",
			},
		}

		err := provider.EncryptSecretKeys(passphrase)
		require.NoError(t, err)
		assert.Empty(t, provider.Anthropic.EncryptedAPIKey)
	})

	t.Run("decrypt with empty encrypted key does nothing", func(t *testing.T) {
		provider := domain.LLMProvider{
			Kind: domain.LLMProviderKindAnthropic,
			Anthropic: &domain.AnthropicSettings{
				Model: "claude-sonnet-4-20250514",
			},
		}

		err := provider.DecryptSecretKeys(passphrase)
		require.NoError(t, err)
		assert.Empty(t, provider.Anthropic.APIKey)
	})

	t.Run("encrypt and decrypt OpenAI API key", func(t *testing.T) {
		originalAPIKey := "sk-proj-test-key-12345"

		provider := domain.LLMProvider{
			Kind: domain.LLMProviderKindOpenAI,
			OpenAI: &domain.OpenAISettings{
				APIKey: originalAPIKey,
				Model:  "gpt-4o",
			},
		}

		// Encrypt
		err := provider.EncryptSecretKeys(passphrase)
		require.NoError(t, err)
		assert.NotEmpty(t, provider.OpenAI.EncryptedAPIKey)
		assert.Empty(t, provider.OpenAI.APIKey) // Should be cleared after encryption

		// Decrypt
		err = provider.DecryptSecretKeys(passphrase)
		require.NoError(t, err)
		assert.Equal(t, originalAPIKey, provider.OpenAI.APIKey)
	})

	t.Run("encrypt OpenAI with empty API key does nothing", func(t *testing.T) {
		provider := domain.LLMProvider{
			Kind: domain.LLMProviderKindOpenAI,
			OpenAI: &domain.OpenAISettings{
				Model: "gpt-4o",
			},
		}

		err := provider.EncryptSecretKeys(passphrase)
		require.NoError(t, err)
		assert.Empty(t, provider.OpenAI.EncryptedAPIKey)
	})

	t.Run("decrypt OpenAI with empty encrypted key does nothing", func(t *testing.T) {
		provider := domain.LLMProvider{
			Kind: domain.LLMProviderKindOpenAI,
			OpenAI: &domain.OpenAISettings{
				Model: "gpt-4o",
			},
		}

		err := provider.DecryptSecretKeys(passphrase)
		require.NoError(t, err)
		assert.Empty(t, provider.OpenAI.APIKey)
	})
}
