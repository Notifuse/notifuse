package domain

import "fmt"

// LLMProviderKind defines the type of LLM provider
type LLMProviderKind string

const (
	LLMProviderKindAnthropic LLMProviderKind = "anthropic"
)

// LLMProvider contains configuration for an LLM service provider
type LLMProvider struct {
	Kind      LLMProviderKind    `json:"kind"`
	Anthropic *AnthropicSettings `json:"anthropic,omitempty"`
}

// Validate validates the LLM provider settings
func (l *LLMProvider) Validate(passphrase string) error {
	if l.Kind == "" {
		return fmt.Errorf("LLM provider kind is required")
	}

	switch l.Kind {
	case LLMProviderKindAnthropic:
		if l.Anthropic == nil {
			return fmt.Errorf("Anthropic settings required when LLM provider kind is anthropic")
		}
		return l.Anthropic.Validate(passphrase)
	default:
		return fmt.Errorf("invalid LLM provider kind: %s", l.Kind)
	}
}

// EncryptSecretKeys encrypts all secret keys in the LLM provider
func (l *LLMProvider) EncryptSecretKeys(passphrase string) error {
	if l.Kind == LLMProviderKindAnthropic && l.Anthropic != nil && l.Anthropic.APIKey != "" {
		if err := l.Anthropic.EncryptAPIKey(passphrase); err != nil {
			return err
		}
		l.Anthropic.APIKey = ""
	}

	return nil
}

// DecryptSecretKeys decrypts all encrypted secret keys in the LLM provider
func (l *LLMProvider) DecryptSecretKeys(passphrase string) error {
	if l.Kind == LLMProviderKindAnthropic && l.Anthropic != nil && l.Anthropic.EncryptedAPIKey != "" {
		if err := l.Anthropic.DecryptAPIKey(passphrase); err != nil {
			return err
		}
	}

	return nil
}
