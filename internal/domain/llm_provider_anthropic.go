package domain

import (
	"fmt"

	"github.com/Notifuse/notifuse/pkg/crypto"
)

// AnthropicSettings contains configuration for Anthropic Claude
type AnthropicSettings struct {
	EncryptedAPIKey string `json:"encrypted_api_key,omitempty"`
	Model           string `json:"model"` // free text - e.g. claude-sonnet-4-20250514

	// Decoded API key, not stored in the database
	APIKey string `json:"api_key,omitempty"`
}

// DecryptAPIKey decrypts the encrypted API key
func (a *AnthropicSettings) DecryptAPIKey(passphrase string) error {
	apiKey, err := crypto.DecryptFromHexString(a.EncryptedAPIKey, passphrase)
	if err != nil {
		return fmt.Errorf("failed to decrypt Anthropic API key: %w", err)
	}
	a.APIKey = apiKey
	return nil
}

// EncryptAPIKey encrypts the API key
func (a *AnthropicSettings) EncryptAPIKey(passphrase string) error {
	encryptedAPIKey, err := crypto.EncryptString(a.APIKey, passphrase)
	if err != nil {
		return fmt.Errorf("failed to encrypt Anthropic API key: %w", err)
	}
	a.EncryptedAPIKey = encryptedAPIKey
	return nil
}

// Validate validates the Anthropic settings
func (a *AnthropicSettings) Validate(passphrase string) error {
	if a.Model == "" {
		return fmt.Errorf("model is required for Anthropic configuration")
	}

	// Encrypt API key if it's not empty
	if a.APIKey != "" {
		if err := a.EncryptAPIKey(passphrase); err != nil {
			return fmt.Errorf("failed to encrypt Anthropic API key: %w", err)
		}
	}

	return nil
}
