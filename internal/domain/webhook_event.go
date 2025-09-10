package domain

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"time"

	"github.com/asaskevich/govalidator"
)

//go:generate mockgen -destination mocks/mock_webhook_event_repository.go -package mocks github.com/Notifuse/notifuse/internal/domain WebhookEventRepository
//go:generate mockgen -destination mocks/mock_webhook_event_service.go -package mocks github.com/Notifuse/notifuse/internal/domain WebhookEventServiceInterface

// EmailEventType defines the type of email webhook event
type EmailEventType string

const (
	// EmailEventDelivered indicates a successful email delivery
	EmailEventDelivered EmailEventType = "delivered"

	// EmailEventBounce indicates a bounced email
	EmailEventBounce EmailEventType = "bounce"

	// EmailEventComplaint indicates a complaint was filed for the email
	EmailEventComplaint EmailEventType = "complaint"
)

// WebhookEvent represents an event received from an email provider webhook
type WebhookEvent struct {
	ID                string            `json:"id"`
	Type              EmailEventType    `json:"type"`
	EmailProviderKind EmailProviderKind `json:"email_provider_kind"`
	IntegrationID     string            `json:"integration_id"`
	RecipientEmail    string            `json:"recipient_email"`
	MessageID         string            `json:"message_id"`
	Timestamp         time.Time         `json:"timestamp"`
	RawPayload        string            `json:"raw_payload"`

	// Bounce specific fields
	BounceType       string `json:"bounce_type,omitempty"`
	BounceCategory   string `json:"bounce_category,omitempty"`
	BounceDiagnostic string `json:"bounce_diagnostic,omitempty"`

	// Complaint specific fields
	ComplaintFeedbackType string `json:"complaint_feedback_type,omitempty"`

	CreatedAt time.Time `json:"created_at"` // Creation timestamp in the database
}

// NewWebhookEvent creates a new webhook event
func NewWebhookEvent(
	id string,
	eventType EmailEventType,
	providerKind EmailProviderKind,
	integrationID string,
	recipientEmail string,
	messageID string,
	timestamp time.Time,
	rawPayload string,
) *WebhookEvent {
	return &WebhookEvent{
		ID:                id,
		Type:              eventType,
		EmailProviderKind: providerKind,
		IntegrationID:     integrationID,
		RecipientEmail:    recipientEmail,
		MessageID:         messageID,
		Timestamp:         timestamp,
		RawPayload:        rawPayload,
	}
}

// ErrWebhookEventNotFound is returned when a webhook event is not found
type ErrWebhookEventNotFound struct {
	ID string
}

// Error returns the error message
func (e *ErrWebhookEventNotFound) Error() string {
	return fmt.Sprintf("webhook event with ID %s not found", e.ID)
}

// GetEventByIDRequest defines the parameters for retrieving a webhook event by ID
type GetEventByIDRequest struct {
	ID string `json:"id"`
}

// GetEventsByMessageIDRequest defines the parameters for retrieving webhook events by message ID
type GetEventsByMessageIDRequest struct {
	MessageID string `json:"message_id"`
	Limit     int    `json:"limit,omitempty"`
	Offset    int    `json:"offset,omitempty"`
}
type WebhookEventListParams struct {
	// Cursor-based pagination
	Cursor string `json:"cursor,omitempty"`
	Limit  int    `json:"limit,omitempty"`

	// Workspace identification
	WorkspaceID string `json:"workspace_id"`

	// Filters
	EventType      EmailEventType `json:"event_type,omitempty"`
	RecipientEmail string         `json:"recipient_email,omitempty"`
	MessageID      string         `json:"message_id,omitempty"`

	// Time range filters
	TimestampAfter  *time.Time `json:"timestamp_after,omitempty"`
	TimestampBefore *time.Time `json:"timestamp_before,omitempty"`
}

// FromQuery creates WebhookEventListParams from HTTP query parameters
func (p *WebhookEventListParams) FromQuery(query url.Values) error {
	// Parse cursor and basic string filters
	p.Cursor = query.Get("cursor")
	p.WorkspaceID = query.Get("workspace_id")
	p.EventType = EmailEventType(query.Get("event_type"))
	p.RecipientEmail = query.Get("recipient_email")
	p.MessageID = query.Get("message_id")

	// Parse limit
	if limitStr := query.Get("limit"); limitStr != "" {
		var limit int
		if err := json.Unmarshal([]byte(limitStr), &limit); err != nil {
			return fmt.Errorf("invalid limit value: %s", limitStr)
		}
		p.Limit = limit
	}

	// Parse time filters if provided
	if err := parseTimeParam(query, "timestamp_after", &p.TimestampAfter); err != nil {
		return err
	}
	if err := parseTimeParam(query, "timestamp_before", &p.TimestampBefore); err != nil {
		return err
	}

	// Validate all parameters
	return p.Validate()
}

func (p *WebhookEventListParams) Validate() error {
	// Validate workspace ID
	if p.WorkspaceID == "" {
		return fmt.Errorf("workspace_id is required")
	}

	// Validate limit
	if p.Limit < 0 {
		return fmt.Errorf("limit cannot be negative")
	}
	if p.Limit > 100 {
		p.Limit = 100 // Cap at maximum 100 items
	}
	if p.Limit == 0 {
		p.Limit = 20 // Default limit
	}

	// Validate event type
	if p.EventType != "" {
		validEventTypes := []string{
			string(EmailEventDelivered),
			string(EmailEventBounce),
			string(EmailEventComplaint),
		}
		if !govalidator.IsIn(string(p.EventType), validEventTypes...) {
			return fmt.Errorf("invalid event type: %s", p.EventType)
		}
	}

	// Validate contact email if provided
	if p.RecipientEmail != "" && !govalidator.IsEmail(p.RecipientEmail) {
		return fmt.Errorf("invalid contact email format")
	}

	// Validate time ranges
	if p.TimestampAfter != nil && p.TimestampBefore != nil {
		if p.TimestampAfter.After(*p.TimestampBefore) {
			return fmt.Errorf("timestamp_after must be before timestamp_before")
		}
	}

	return nil
}

// WebhookEventListResult contains the result of a ListWebhookEvents operation
type WebhookEventListResult struct {
	Events     []*WebhookEvent `json:"events"`
	NextCursor string          `json:"next_cursor,omitempty"`
	HasMore    bool            `json:"has_more"`
}

// WebhookEventServiceInterface defines the interface for webhook event service
type WebhookEventServiceInterface interface {
	// ProcessWebhook processes a webhook event from an email provider
	ProcessWebhook(ctx context.Context, workspaceID, integrationID string, rawPayload []byte) error

	// ListEvents retrieves all webhook events for a workspace
	ListEvents(ctx context.Context, workspaceID string, params WebhookEventListParams) (*WebhookEventListResult, error)
}

// WebhookEventRepository is the interface for webhook event operations
type WebhookEventRepository interface {
	// StoreEvents stores a webhook event in the database
	StoreEvents(ctx context.Context, workspaceID string, events []*WebhookEvent) error

	// ListEvents retrieves all webhook events for a workspace
	ListEvents(ctx context.Context, workspaceID string, params WebhookEventListParams) (*WebhookEventListResult, error)

	// DeleteForEmail deletes all webhook events for a specific email
	DeleteForEmail(ctx context.Context, workspaceID, email string) error
}
