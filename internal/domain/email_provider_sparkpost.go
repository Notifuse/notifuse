package domain

import (
	"context"
	"fmt"

	"github.com/Notifuse/notifuse/pkg/crypto"
)

//go:generate mockgen -destination mocks/mock_sparkpost_service.go -package mocks github.com/Notifuse/notifuse/internal/domain SparkPostServiceInterface

// SparkPostWebhookPayload represents the webhook payload from SparkPost
type SparkPostWebhookPayload struct {
	MSys SparkPostMSys `json:"msys"`
}

// SparkPostMSys contains the event data from SparkPost
type SparkPostMSys struct {
	MessageEvent *SparkPostMessageEvent `json:"message_event,omitempty"`
}

type SparkPostMessageEvent struct {
	Type          string                 `json:"type"`
	EventID       string                 `json:"event_id"` // unique to the event, kept on a redelivery
	CampaignID    string                 `json:"campaign_id"`
	MessageID     string                 `json:"message_id"`
	Timestamp     string                 `json:"timestamp"`
	RecipientTo   string                 `json:"rcpt_to"`
	RecipientMeta map[string]interface{} `json:"rcpt_meta,omitempty"`
	Tags          []string               `json:"tags,omitempty"`
	Transmission  string                 `json:"transmission_id,omitempty"`
	IPAddress     string                 `json:"ip_address,omitempty"`
	MessageFrom   string                 `json:"msg_from,omitempty"`
	QueueTime     string                 `json:"queue_time,omitempty"`
	// bounce fields
	BounceClass string `json:"bounce_class,omitempty"`
	Reason      string `json:"reason,omitempty"`
	RawReason   string `json:"raw_reason,omitempty"`
	Error       string `json:"error_code,omitempty"`
	// delivery fields
	DeliveryMethod        string `json:"delv_method,omitempty"`
	MailboxProvider       string `json:"mailbox_provider,omitempty"`
	MailboxProviderRegion string `json:"mailbox_provider_region,omitempty"`
	// spam fields
	FeedbackType string `json:"fbtype,omitempty"`
}

// SparkPostWebhook represents a webhook configuration in SparkPost
type SparkPostWebhook struct {
	ID            string                 `json:"id,omitempty"`
	Name          string                 `json:"name"`
	Target        string                 `json:"target"`
	Events        []string               `json:"events"`
	Active        bool                   `json:"active"`
	AuthType      string                 `json:"auth_type,omitempty"` // "none", "basic", "oauth2"
	AuthToken     string                 `json:"auth_token,omitempty"`
	AuthRequest   map[string]interface{} `json:"auth_request,omitempty"`
	CustomHeaders map[string]string      `json:"custom_headers,omitempty"`
}

// SparkPostWebhookListResponse represents the response for listing webhooks
type SparkPostWebhookListResponse struct {
	Results []SparkPostWebhook `json:"results"`
}

// SparkPostWebhookResponse represents a response for webhook operations
type SparkPostWebhookResponse struct {
	Results SparkPostWebhook `json:"results"`
}

// SparkPostSettings contains configuration for SparkPost email provider
type SparkPostSettings struct {
	EncryptedAPIKey string `json:"encrypted_api_key,omitempty"`
	SandboxMode     bool   `json:"sandbox_mode"`
	Endpoint        string `json:"endpoint"`

	// decoded API key, not stored in the database
	APIKey string `json:"api_key,omitempty"`
}

func (s *SparkPostSettings) DecryptAPIKey(passphrase string) error {
	apiKey, err := crypto.DecryptFromHexString(s.EncryptedAPIKey, passphrase)
	if err != nil {
		return fmt.Errorf("failed to decrypt SparkPost API key: %w", err)
	}
	s.APIKey = apiKey
	return nil
}

func (s *SparkPostSettings) EncryptAPIKey(passphrase string) error {
	encryptedAPIKey, err := crypto.EncryptString(s.APIKey, passphrase)
	if err != nil {
		return fmt.Errorf("failed to encrypt SparkPost API key: %w", err)
	}
	s.EncryptedAPIKey = encryptedAPIKey
	return nil
}

func (s *SparkPostSettings) Validate(passphrase string) error {
	if s.Endpoint == "" {
		return fmt.Errorf("endpoint is required for SparkPost configuration")
	}

	// Encrypt API key if it's not empty
	if s.APIKey != "" {
		if err := s.EncryptAPIKey(passphrase); err != nil {
			return fmt.Errorf("failed to encrypt SparkPost API key: %w", err)
		}
	}

	return nil
}

//go:generate mockgen -destination mocks/mock_sparkpost_service.go -package mocks github.com/Notifuse/notifuse/internal/domain SparkPostServiceInterface

// SparkPostServiceInterface defines operations for managing SparkPost webhooks
type SparkPostServiceInterface interface {
	// ListWebhooks retrieves all registered webhooks
	ListWebhooks(ctx context.Context, config SparkPostSettings) (*SparkPostWebhookListResponse, error)

	// CreateWebhook creates a new webhook
	CreateWebhook(ctx context.Context, config SparkPostSettings, webhook SparkPostWebhook) (*SparkPostWebhookResponse, error)

	// GetWebhook retrieves a webhook by ID
	GetWebhook(ctx context.Context, config SparkPostSettings, webhookID string) (*SparkPostWebhookResponse, error)

	// UpdateWebhook updates an existing webhook
	UpdateWebhook(ctx context.Context, config SparkPostSettings, webhookID string, webhook SparkPostWebhook) (*SparkPostWebhookResponse, error)

	// DeleteWebhook deletes a webhook by ID
	DeleteWebhook(ctx context.Context, config SparkPostSettings, webhookID string) error

	// TestWebhook sends a test event to a webhook
	TestWebhook(ctx context.Context, config SparkPostSettings, webhookID string) error

	// ValidateWebhook validates a webhook's configuration
	ValidateWebhook(ctx context.Context, config SparkPostSettings, webhook SparkPostWebhook) (bool, error)
}
