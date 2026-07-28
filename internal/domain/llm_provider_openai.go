package domain

import (
	"fmt"

	"github.com/Notifuse/notifuse/pkg/crypto"
)

// OpenAISettings contains configuration for OpenAI or OpenAI-compatible providers
type OpenAISettings struct {
	EncryptedAPIKey string `json:"encrypted_api_key,omitempty"`
	Model           string `json:"model"`              // free text - e.g. gpt-4o, gpt-4.1, or custom model name
	BaseURL         string `json:"base_url,omitempty"` // optional custom endpoint for OpenAI-compatible providers
	// ReasoningEffort controls how much a reasoning model thinks. Empty = provider
	// default (param omitted); one of none/minimal/low/medium/high/xhigh otherwise.
	// Non-reasoning models and many OpenAI-compatible endpoints ignore it.
	ReasoningEffort string `json:"reasoning_effort,omitempty"`

	// Decoded API key, not stored in the database
	APIKey string `json:"api_key,omitempty"`
}

// IsValidReasoningEffort reports whether effort is empty (= provider default) or one
// of the supported OpenAI reasoning-effort levels.
func IsValidReasoningEffort(effort string) bool {
	switch effort {
	case "", "none", "minimal", "low", "medium", "high", "xhigh":
		return true
	default:
		return false
	}
}

// DecryptAPIKey decrypts the encrypted API key
func (o *OpenAISettings) DecryptAPIKey(passphrase string) error {
	apiKey, err := crypto.DecryptFromHexString(o.EncryptedAPIKey, passphrase)
	if err != nil {
		return fmt.Errorf("failed to decrypt OpenAI API key: %w", err)
	}
	o.APIKey = apiKey
	return nil
}

// EncryptAPIKey encrypts the API key
func (o *OpenAISettings) EncryptAPIKey(passphrase string) error {
	encryptedAPIKey, err := crypto.EncryptString(o.APIKey, passphrase)
	if err != nil {
		return fmt.Errorf("failed to encrypt OpenAI API key: %w", err)
	}
	o.EncryptedAPIKey = encryptedAPIKey
	return nil
}

// Validate validates the OpenAI settings
func (o *OpenAISettings) Validate(passphrase string) error {
	if o.Model == "" {
		return fmt.Errorf("model is required for OpenAI configuration")
	}

	if !IsValidReasoningEffort(o.ReasoningEffort) {
		return fmt.Errorf("invalid reasoning_effort %q for OpenAI configuration", o.ReasoningEffort)
	}

	// Encrypt API key if it's not empty
	if o.APIKey != "" {
		if err := o.EncryptAPIKey(passphrase); err != nil {
			return fmt.Errorf("failed to encrypt OpenAI API key: %w", err)
		}
	}

	return nil
}
