package domain

import (
	"context"
	"fmt"

	"github.com/Notifuse/notifuse/pkg/crypto"
)

// PostmarkWebhookPayload represents the base webhook payload from Postmark
type PostmarkWebhookPayload struct {
	RecordType    string            `json:"RecordType"`
	MessageStream string            `json:"MessageStream"`
	ID            string            `json:"ID"`
	MessageID     string            `json:"MessageID"`
	ServerID      int               `json:"ServerID"`
	Metadata      map[string]string `json:"Metadata,omitempty"`
	Tag           string            `json:"Tag,omitempty"`

	// Delivered event specific fields
	DeliveredFields *PostmarkDeliveredFields `json:"-"`

	// Bounce event specific fields
	BounceFields *PostmarkBounceFields `json:"-"`

	// Complaint event specific fields
	ComplaintFields *PostmarkComplaintFields `json:"-"`
}

// PostmarkDeliveredFields contains fields specific to delivery events
type PostmarkDeliveredFields struct {
	RecipientEmail string `json:"Recipient"`
	DeliveredAt    string `json:"DeliveredAt"`
	Details        string `json:"Details"`
}

// PostmarkBounceFields contains fields specific to bounce events
type PostmarkBounceFields struct {
	RecipientEmail string `json:"Email"`
	BouncedAt      string `json:"BouncedAt"`
	Type           string `json:"Type"`
	TypeCode       int    `json:"TypeCode"`
	Name           string `json:"Name"`
	Description    string `json:"Description,omitempty"`
	Details        string `json:"Details,omitempty"`
	DumpAvailable  bool   `json:"DumpAvailable"`
	CanActivate    bool   `json:"CanActivate"`
	Subject        string `json:"Subject"`
	Content        string `json:"Content,omitempty"`
}

// PostmarkComplaintFields contains fields specific to complaint events
type PostmarkComplaintFields struct {
	RecipientEmail string `json:"Email"`
	ComplainedAt   string `json:"ComplainedAt"`
	Type           string `json:"Type"`
	UserAgent      string `json:"UserAgent,omitempty"`
	Subject        string `json:"Subject"`
}

// PostmarkWebhookConfig represents a webhook configuration in Postmark
type PostmarkWebhookConfig struct {
	ID            int               `json:"ID,omitempty"`
	URL           string            `json:"Url"`
	MessageStream string            `json:"MessageStream"`
	HttpAuth      *HttpAuth         `json:"HttpAuth,omitempty"`
	HttpHeaders   []HttpHeader      `json:"HttpHeaders,omitempty"`
	Triggers      *PostmarkTriggers `json:"Triggers"`
}

// HttpAuth represents HTTP authentication for webhooks
type HttpAuth struct {
	Username string `json:"Username"`
	Password string `json:"Password"`
}

// HttpHeader represents a custom HTTP header
type HttpHeader struct {
	Name  string `json:"Name"`
	Value string `json:"Value"`
}

// PostmarkTriggers represents the webhook triggers configuration
type PostmarkTriggers struct {
	Open               *PostmarkOpenTrigger               `json:"Open,omitempty"`
	Click              *PostmarkClickTrigger              `json:"Click,omitempty"`
	Delivery           *PostmarkDeliveryTrigger           `json:"Delivery,omitempty"`
	Bounce             *PostmarkBounceTrigger             `json:"Bounce,omitempty"`
	SpamComplaint      *PostmarkSpamComplaintTrigger      `json:"SpamComplaint,omitempty"`
	SubscriptionChange *PostmarkSubscriptionChangeTrigger `json:"SubscriptionChange,omitempty"`
}

// PostmarkOpenTrigger represents the open trigger configuration
type PostmarkOpenTrigger struct {
	Enabled           bool `json:"Enabled"`
	PostFirstOpenOnly bool `json:"PostFirstOpenOnly,omitempty"`
}

// PostmarkClickTrigger represents the click trigger configuration
type PostmarkClickTrigger struct {
	Enabled bool `json:"Enabled"`
}

// PostmarkDeliveryTrigger represents the delivery trigger configuration
type PostmarkDeliveryTrigger struct {
	Enabled bool `json:"Enabled"`
}

// PostmarkBounceTrigger represents the bounce trigger configuration
type PostmarkBounceTrigger struct {
	Enabled        bool `json:"Enabled"`
	IncludeContent bool `json:"IncludeContent,omitempty"`
}

// PostmarkSpamComplaintTrigger represents the spam complaint trigger configuration
type PostmarkSpamComplaintTrigger struct {
	Enabled        bool `json:"Enabled"`
	IncludeContent bool `json:"IncludeContent,omitempty"`
}

// PostmarkSubscriptionChangeTrigger represents the subscription change trigger configuration
type PostmarkSubscriptionChangeTrigger struct {
	Enabled bool `json:"Enabled"`
}

// PostmarkTriggerRule represents a trigger for webhooks
// Note: This is kept for compatibility with existing code
type PostmarkTriggerRule struct {
	Key   string `json:"Key"`
	Match string `json:"Match"`
	Value string `json:"Value"`
}

// PostmarkWebhookResponse represents the response from Postmark API for webhook operations
type PostmarkWebhookResponse struct {
	ID            int               `json:"ID"`
	URL           string            `json:"Url"`
	MessageStream string            `json:"MessageStream"`
	HttpAuth      *HttpAuth         `json:"HttpAuth,omitempty"`
	HttpHeaders   []HttpHeader      `json:"HttpHeaders,omitempty"`
	Triggers      *PostmarkTriggers `json:"Triggers"`
}

// PostmarkListWebhooksResponse represents the response for listing webhooks
type PostmarkListWebhooksResponse struct {
	TotalCount int                       `json:"TotalCount"`
	Webhooks   []PostmarkWebhookResponse `json:"Webhooks"`
}

type PostmarkSettings struct {
	EncryptedServerToken string `json:"encrypted_server_token,omitempty"`
	ServerToken          string `json:"server_token,omitempty"`
	MessageStream        string `json:"message_stream,omitempty"`
}

// GetMessageStream returns the configured message stream, defaulting to "outbound"
func (p *PostmarkSettings) GetMessageStream() string {
	if p.MessageStream == "" {
		return "outbound"
	}
	return p.MessageStream
}

func (p *PostmarkSettings) DecryptServerToken(passphrase string) error {
	serverToken, err := crypto.DecryptFromHexString(p.EncryptedServerToken, passphrase)
	if err != nil {
		return fmt.Errorf("failed to decrypt Postmark server token: %w", err)
	}
	p.ServerToken = serverToken
	return nil
}

func (p *PostmarkSettings) EncryptServerToken(passphrase string) error {
	encryptedServerToken, err := crypto.EncryptString(p.ServerToken, passphrase)
	if err != nil {
		return fmt.Errorf("failed to encrypt Postmark server token: %w", err)
	}
	p.EncryptedServerToken = encryptedServerToken
	return nil
}

func (p *PostmarkSettings) Validate(passphrase string) error {
	// Encrypt server token if it's not empty
	if p.ServerToken != "" {
		if err := p.EncryptServerToken(passphrase); err != nil {
			return fmt.Errorf("failed to encrypt Postmark server token: %w", err)
		}
	}

	// Default empty message stream to "outbound"
	if p.MessageStream == "" {
		p.MessageStream = "outbound"
	}

	return nil
}

//go:generate mockgen -destination mocks/mock_postmark_service.go -package mocks github.com/Notifuse/notifuse/internal/domain PostmarkServiceInterface

// PostmarkServiceInterface defines operations for managing Postmark webhooks
type PostmarkServiceInterface interface {
	// ListWebhooks retrieves all registered webhooks
	ListWebhooks(ctx context.Context, config PostmarkSettings) (*PostmarkListWebhooksResponse, error)

	// RegisterWebhook registers a new webhook
	RegisterWebhook(ctx context.Context, config PostmarkSettings, webhook PostmarkWebhookConfig) (*PostmarkWebhookResponse, error)

	// UnregisterWebhook removes a webhook by ID
	UnregisterWebhook(ctx context.Context, config PostmarkSettings, webhookID int) error

	// GetWebhook retrieves a specific webhook by ID
	GetWebhook(ctx context.Context, config PostmarkSettings, webhookID int) (*PostmarkWebhookResponse, error)

	// UpdateWebhook updates an existing webhook
	UpdateWebhook(ctx context.Context, config PostmarkSettings, webhookID int, webhook PostmarkWebhookConfig) (*PostmarkWebhookResponse, error)

	// TestWebhook sends a test event to the webhook
	TestWebhook(ctx context.Context, config PostmarkSettings, webhookID int, eventType EmailEventType) error
}
