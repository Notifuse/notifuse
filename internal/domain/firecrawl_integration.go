package domain

import (
	"fmt"

	"github.com/Notifuse/notifuse/pkg/crypto"
)

// FirecrawlSettings contains configuration for Firecrawl integration
type FirecrawlSettings struct {
	EncryptedAPIKey string `json:"encrypted_api_key,omitempty"`
	BaseURL         string `json:"base_url,omitempty"` // Optional, for self-hosted

	// Decoded API key, not stored in the database
	APIKey string `json:"api_key,omitempty"`
}

// DecryptAPIKey decrypts the encrypted API key
func (f *FirecrawlSettings) DecryptAPIKey(passphrase string) error {
	if f.EncryptedAPIKey == "" {
		return nil
	}
	apiKey, err := crypto.DecryptFromHexString(f.EncryptedAPIKey, passphrase)
	if err != nil {
		return fmt.Errorf("failed to decrypt Firecrawl API key: %w", err)
	}
	f.APIKey = apiKey
	return nil
}

// EncryptAPIKey encrypts the API key
func (f *FirecrawlSettings) EncryptAPIKey(passphrase string) error {
	if f.APIKey == "" {
		return nil
	}
	encryptedAPIKey, err := crypto.EncryptString(f.APIKey, passphrase)
	if err != nil {
		return fmt.Errorf("failed to encrypt Firecrawl API key: %w", err)
	}
	f.EncryptedAPIKey = encryptedAPIKey
	return nil
}

// Validate validates the Firecrawl settings and encrypts API key
func (f *FirecrawlSettings) Validate(passphrase string) error {
	// API key is required (either plaintext or already encrypted)
	if f.APIKey == "" && f.EncryptedAPIKey == "" {
		return fmt.Errorf("API key is required for Firecrawl configuration")
	}

	// Encrypt API key if it's not empty
	if f.APIKey != "" {
		if err := f.EncryptAPIKey(passphrase); err != nil {
			return err
		}
		f.APIKey = "" // Clear plaintext after encryption
	}

	return nil
}

// EncryptSecretKeys encrypts all secret keys (wrapper for integration compatibility)
func (f *FirecrawlSettings) EncryptSecretKeys(passphrase string) error {
	if f.APIKey != "" {
		if err := f.EncryptAPIKey(passphrase); err != nil {
			return err
		}
		f.APIKey = "" // Clear plaintext after encryption
	}
	return nil
}

// DecryptSecretKeys decrypts all secret keys (wrapper for integration compatibility)
func (f *FirecrawlSettings) DecryptSecretKeys(passphrase string) error {
	return f.DecryptAPIKey(passphrase)
}

// GetBaseURL returns the API base URL (default or custom)
func (f *FirecrawlSettings) GetBaseURL() string {
	if f.BaseURL != "" {
		return f.BaseURL
	}
	return "https://api.firecrawl.dev"
}
