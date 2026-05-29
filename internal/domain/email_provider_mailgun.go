package domain

import (
	"context"
	"fmt"

	"github.com/Notifuse/notifuse/pkg/crypto"
)

//go:generate mockgen -destination mocks/mock_mailgun_service.go -package mocks github.com/Notifuse/notifuse/internal/domain MailgunServiceInterface

// MailgunWebhookPayload represents a Mailgun webhook payload
type MailgunWebhookPayload struct {
	Signature MailgunSignature `json:"signature"`
	EventData MailgunEventData `json:"event-data"`
}

// MailgunSignature contains signature information for webhook authentication
type MailgunSignature struct {
	Timestamp string `json:"timestamp"`
	Token     string `json:"token"`
	Signature string `json:"signature"`
}

// MailgunEventData contains the main event data from Mailgun
type MailgunEventData struct {
	Event         string                 `json:"event"`
	Timestamp     float64                `json:"timestamp"`
	ID            string                 `json:"id"`
	Recipient     string                 `json:"recipient"`
	Tags          []string               `json:"tags"`
	Message       MailgunMessage         `json:"message"`
	Delivery      MailgunDelivery        `json:"delivery,omitempty"`
	Reason        string                 `json:"reason,omitempty"`
	Severity      string                 `json:"severity,omitempty"`
	Storage       map[string]interface{} `json:"storage,omitempty"`
	UserVariables map[string]interface{} `json:"user-variables,omitempty"`
	Flags         map[string]interface{} `json:"flags,omitempty"`
}

// MailgunMessage contains information about the email message
type MailgunMessage struct {
	Headers     MailgunHeaders `json:"headers"`
	Attachments []interface{}  `json:"attachments"`
	Size        int            `json:"size"`
}

// MailgunHeaders contains email headers
type MailgunHeaders struct {
	To        string `json:"to"`
	MessageID string `json:"message-id"`
	From      string `json:"from"`
	Subject   string `json:"subject"`
}

// MailgunDelivery contains delivery information
type MailgunDelivery struct {
	Status           string                 `json:"status,omitempty"`
	Code             int                    `json:"code,omitempty"`
	Message          string                 `json:"message,omitempty"`
	AttemptNo        int                    `json:"attempt-no,omitempty"`
	Description      string                 `json:"description,omitempty"`
	SessionSeconds   float64                `json:"session-seconds,omitempty"`
	Certificate      bool                   `json:"certificate,omitempty"`
	TLS              bool                   `json:"tls,omitempty"`
	MXHost           string                 `json:"mx-host,omitempty"`
	DelvDataFeedback []interface{}          `json:"delivery-status,omitempty"`
	SMTP             map[string]interface{} `json:"smtp,omitempty"`
}

// MailgunWebhook represents a webhook configuration in Mailgun
type MailgunWebhook struct {
	ID     string   `json:"id,omitempty"`
	URL    string   `json:"url"`
	URLs   []string `json:"urls,omitempty"`
	Events []string `json:"events"`
	Active bool     `json:"active"`
}

// MailgunWebhookListResponse represents the response from listing webhooks
type MailgunWebhooks struct {
	Delivered     MailgunUrls `json:"delivered"`
	PermanentFail MailgunUrls `json:"permanent_fail"`
	TemporaryFail MailgunUrls `json:"temporary_fail"`
	Complained    MailgunUrls `json:"complained"`
}

// MailgunUrls mirrors a Mailgun webhook entry. The API returns the configured
// callbacks either as a single "url" string or as a "urls" array (the official
// mailgun-go SDK models this as its UrlOrUrls type), so both must be decoded.
type MailgunUrls struct {
	URL  string   `json:"url,omitempty"`
	URLs []string `json:"urls"`
}

// All folds the singular "url" and the "urls" array into a single, deduplicated
// list of callback URLs, preserving order (url first, then urls).
func (u MailgunUrls) All() []string {
	seen := make(map[string]bool)
	result := []string{}
	for _, candidate := range append([]string{u.URL}, u.URLs...) {
		if candidate != "" && !seen[candidate] {
			seen[candidate] = true
			result = append(result, candidate)
		}
	}
	return result
}

type MailgunWebhookListResponse struct {
	Webhooks MailgunWebhooks `json:"webhooks"`
}

// MailgunSettings contains configuration for Mailgun
type MailgunSettings struct {
	EncryptedAPIKey string `json:"encrypted_api_key,omitempty"`
	Domain          string `json:"domain"`
	Region          string `json:"region,omitempty"` // "US" or "EU"

	// decoded API key, not stored in the database
	APIKey string `json:"api_key,omitempty"`
}

func (m *MailgunSettings) DecryptAPIKey(passphrase string) error {
	apiKey, err := crypto.DecryptFromHexString(m.EncryptedAPIKey, passphrase)
	if err != nil {
		return fmt.Errorf("failed to decrypt Mailgun API key: %w", err)
	}
	m.APIKey = apiKey
	return nil
}

func (m *MailgunSettings) EncryptAPIKey(passphrase string) error {
	encryptedAPIKey, err := crypto.EncryptString(m.APIKey, passphrase)
	if err != nil {
		return fmt.Errorf("failed to encrypt Mailgun API key: %w", err)
	}
	m.EncryptedAPIKey = encryptedAPIKey
	return nil
}

func (m *MailgunSettings) Validate(passphrase string) error {
	if m.Domain == "" {
		return fmt.Errorf("domain is required for Mailgun configuration")
	}

	// Encrypt API key if it's not empty
	if m.APIKey != "" {
		if err := m.EncryptAPIKey(passphrase); err != nil {
			return fmt.Errorf("failed to encrypt Mailgun API key: %w", err)
		}
		m.APIKey = "" // Clear the API key after encryption
	}

	return nil
}

//go:generate mockgen -destination mocks/mock_mailgun_service.go -package mocks github.com/Notifuse/notifuse/internal/domain MailgunServiceInterface

// MailgunServiceInterface defines operations for managing Mailgun webhooks
type MailgunServiceInterface interface {
	// ListWebhooks retrieves all registered webhooks for a domain
	ListWebhooks(ctx context.Context, config MailgunSettings) (*MailgunWebhookListResponse, error)

	// CreateWebhook creates a new webhook
	CreateWebhook(ctx context.Context, config MailgunSettings, webhook MailgunWebhook) (*MailgunWebhook, error)

	// GetWebhook retrieves a webhook by ID
	GetWebhook(ctx context.Context, config MailgunSettings, webhookID string) (*MailgunWebhook, error)

	// UpdateWebhook updates an existing webhook
	UpdateWebhook(ctx context.Context, config MailgunSettings, webhookID string, webhook MailgunWebhook) (*MailgunWebhook, error)

	// DeleteWebhook deletes a webhook by ID
	DeleteWebhook(ctx context.Context, config MailgunSettings, webhookID string) error

	// TestWebhook sends a test event to a webhook
	TestWebhook(ctx context.Context, config MailgunSettings, webhookID string, eventType string) error
}
