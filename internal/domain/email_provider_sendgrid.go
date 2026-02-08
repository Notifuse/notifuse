package domain

import (
	"context"
	"fmt"

	"github.com/Notifuse/notifuse/pkg/crypto"
)

//go:generate mockgen -destination mocks/mock_sendgrid_service.go -package mocks github.com/Notifuse/notifuse/internal/domain SendGridServiceInterface

// SendGridSettings contains configuration for SendGrid email provider
type SendGridSettings struct {
	EncryptedAPIKey string `json:"encrypted_api_key,omitempty"`

	// decoded API key, not stored in the database
	APIKey string `json:"api_key,omitempty"`
}

func (s *SendGridSettings) DecryptAPIKey(passphrase string) error {
	apiKey, err := crypto.DecryptFromHexString(s.EncryptedAPIKey, passphrase)
	if err != nil {
		return fmt.Errorf("failed to decrypt SendGrid API key: %w", err)
	}
	s.APIKey = apiKey
	return nil
}

func (s *SendGridSettings) EncryptAPIKey(passphrase string) error {
	encryptedAPIKey, err := crypto.EncryptString(s.APIKey, passphrase)
	if err != nil {
		return fmt.Errorf("failed to encrypt SendGrid API key: %w", err)
	}
	s.EncryptedAPIKey = encryptedAPIKey
	return nil
}

func (s *SendGridSettings) Validate(passphrase string) error {
	// Encrypt API key if it's not empty
	if s.APIKey != "" {
		if err := s.EncryptAPIKey(passphrase); err != nil {
			return fmt.Errorf("failed to encrypt SendGrid API key: %w", err)
		}
	}

	return nil
}

// SendGridWebhookEvent represents a single event from SendGrid webhook payload
// Note: custom_args are flattened into top-level fields by SendGrid
type SendGridWebhookEvent struct {
	Email     string `json:"email"`
	Timestamp int64  `json:"timestamp"`
	Event     string `json:"event"` // processed, deferred, delivered, bounce, blocked, dropped, spamreport, open, click, unsubscribe
	SGEventID string `json:"sg_event_id"`
	SMTPId    string `json:"smtp-id,omitempty"`
	// SendGrid message ID - can have different field names
	SGMessageID string `json:"sg_message_id,omitempty"`

	// Bounce-specific fields
	Reason               string `json:"reason,omitempty"`
	Status               string `json:"status,omitempty"` // SMTP status code e.g., "5.1.1"
	Type                 string `json:"type,omitempty"`   // "bounce" or "blocked"
	BounceClassification string `json:"bounce_classification,omitempty"`

	// Delivery fields
	Response string `json:"response,omitempty"` // SMTP response

	// Open/Click fields
	IP        string `json:"ip,omitempty"`
	UserAgent string `json:"useragent,omitempty"`
	URL       string `json:"url,omitempty"`

	// Category for email organization
	Category []string `json:"category,omitempty"`

	// Our custom_arg - flattened at top level by SendGrid (not nested in an object)
	NotifuseMessageID string `json:"notifuse_message_id,omitempty"`
}

// SendGridWebhookSettings represents the webhook configuration in SendGrid
type SendGridWebhookSettings struct {
	Enabled          bool   `json:"enabled"`
	URL              string `json:"url"`
	FriendlyName     string `json:"friendly_name,omitempty"`
	Processed        bool   `json:"processed,omitempty"`
	Deferred         bool   `json:"deferred,omitempty"`
	Delivered        bool   `json:"delivered,omitempty"`
	Bounce           bool   `json:"bounce,omitempty"`
	Dropped          bool   `json:"dropped,omitempty"`
	SpamReport       bool   `json:"spam_report,omitempty"`
	Unsubscribe      bool   `json:"unsubscribe,omitempty"`
	GroupUnsubscribe bool   `json:"group_unsubscribe,omitempty"`
	GroupResubscribe bool   `json:"group_resubscribe,omitempty"`
	Open             bool   `json:"open,omitempty"`
	Click            bool   `json:"click,omitempty"`
}

// SendGridServiceInterface defines operations for managing SendGrid webhooks
type SendGridServiceInterface interface {
	// GetWebhookSettings retrieves the current webhook configuration
	GetWebhookSettings(ctx context.Context, config SendGridSettings) (*SendGridWebhookSettings, error)

	// UpdateWebhookSettings updates the webhook configuration
	UpdateWebhookSettings(ctx context.Context, config SendGridSettings, settings SendGridWebhookSettings) error
}
