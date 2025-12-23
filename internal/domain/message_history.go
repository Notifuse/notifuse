package domain

import (
	"bytes"
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"net/url"
	"time"

	"github.com/asaskevich/govalidator"
)

//go:generate mockgen -destination mocks/mock_message_history_service.go -package mocks github.com/Notifuse/notifuse/internal/domain MessageHistoryService
//go:generate mockgen -destination mocks/mock_message_history_repository.go -package mocks github.com/Notifuse/notifuse/internal/domain MessageHistoryRepository

// MessageStatus represents the current status of a message
type MessageEvent string

const (
	// Message status constants
	MessageEventSent         MessageEvent = "sent"
	MessageEventDelivered    MessageEvent = "delivered"
	MessageEventFailed       MessageEvent = "failed"
	MessageEventOpened       MessageEvent = "opened"
	MessageEventClicked      MessageEvent = "clicked"
	MessageEventBounced      MessageEvent = "bounced"
	MessageEventComplained   MessageEvent = "complained"
	MessageEventUnsubscribed MessageEvent = "unsubscribed"
)

// MessageEventUpdate represents a status update for a message
type MessageEventUpdate struct {
	ID         string       `json:"id"`
	Event      MessageEvent `json:"event"`
	Timestamp  time.Time    `json:"timestamp"`
	StatusInfo *string      `json:"status_info,omitempty"`
}

// ChannelOptions represents channel-specific delivery options
// This structure allows future extension for SMS/push without breaking changes
type ChannelOptions struct {
	// Email-specific options
	FromName *string  `json:"from_name,omitempty"`
	CC       []string `json:"cc,omitempty"`
	BCC      []string `json:"bcc,omitempty"`
	ReplyTo  string   `json:"reply_to,omitempty"`

	// Future: SMS options would go here
	// Future: Push notification options would go here
}

// Value implements the driver.Valuer interface for database storage
func (co ChannelOptions) Value() (driver.Value, error) {
	return json.Marshal(co)
}

// Scan implements the sql.Scanner interface for database retrieval
func (co *ChannelOptions) Scan(value interface{}) error {
	if value == nil {
		return nil
	}

	b, ok := value.([]byte)
	if !ok {
		return sql.ErrNoRows
	}

	cloned := bytes.Clone(b)
	return json.Unmarshal(cloned, co)
}

// MessageData represents the JSON data used to compile a template
type MessageData struct {
	// Custom fields used in template compilation
	Data map[string]interface{} `json:"data"`
	// Optional metadata for tracking
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// Value implements the driver.Valuer interface for database storage
func (d MessageData) Value() (driver.Value, error) {
	return json.Marshal(d)
}

// Scan implements the sql.Scanner interface for database retrieval
func (d *MessageData) Scan(value interface{}) error {
	if value == nil {
		return nil
	}

	b, ok := value.([]byte)
	if !ok {
		return sql.ErrNoRows
	}

	cloned := bytes.Clone(b)
	return json.Unmarshal(cloned, &d)
}

// MessageHistory represents a record of a message sent to a contact
type MessageHistory struct {
	ID              string               `json:"id"`
	ExternalID      *string              `json:"external_id,omitempty"` // For idempotency checks
	ContactEmail    string               `json:"contact_email"`
	BroadcastID     *string              `json:"broadcast_id,omitempty"`
	AutomationID    *string              `json:"automation_id,omitempty"` // Automation this message was sent from (nullable for broadcasts/transactional)
	ListID          *string              `json:"list_id,omitempty"`       // List this message was sent to (nullable for transactional emails)
	TemplateID      string               `json:"template_id"`
	TemplateVersion int64                `json:"template_version"`
	Channel         string               `json:"channel"` // email, sms, push, etc.
	StatusInfo      *string              `json:"status_info,omitempty"`
	MessageData     MessageData          `json:"message_data"`
	ChannelOptions  *ChannelOptions      `json:"channel_options,omitempty"` // Channel-specific delivery options
	Attachments     []AttachmentMetadata `json:"attachments,omitempty"`

	// Event timestamps
	SentAt         time.Time  `json:"sent_at"`
	DeliveredAt    *time.Time `json:"delivered_at,omitempty"`
	FailedAt       *time.Time `json:"failed_at,omitempty"`
	OpenedAt       *time.Time `json:"opened_at,omitempty"`
	ClickedAt      *time.Time `json:"clicked_at,omitempty"`
	BouncedAt      *time.Time `json:"bounced_at,omitempty"`
	ComplainedAt   *time.Time `json:"complained_at,omitempty"`
	UnsubscribedAt *time.Time `json:"unsubscribed_at,omitempty"`

	// System timestamps
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type MessageHistoryStatusSum struct {
	TotalSent         int `json:"total_sent"`
	TotalDelivered    int `json:"total_delivered"`
	TotalBounced      int `json:"total_bounced"`
	TotalComplained   int `json:"total_complained"`
	TotalFailed       int `json:"total_failed"`
	TotalOpened       int `json:"total_opened"`
	TotalClicked      int `json:"total_clicked"`
	TotalUnsubscribed int `json:"total_unsubscribed"`
}

// MessageHistoryRepository defines methods for message history persistence
type MessageHistoryRepository interface {
	// Create adds a new message history record
	Create(ctx context.Context, workspaceID string, secretKey string, message *MessageHistory) error

	// Upsert creates or updates a message history record (for retry handling)
	// On conflict, updates failed_at, status_info, and updated_at fields
	Upsert(ctx context.Context, workspaceID string, secretKey string, message *MessageHistory) error

	// Update updates an existing message history record
	Update(ctx context.Context, workspaceID string, message *MessageHistory) error

	// Get retrieves a message history by ID
	Get(ctx context.Context, workspaceID string, secretKey string, id string) (*MessageHistory, error)

	// GetByExternalID retrieves a message history by external ID for idempotency checks
	GetByExternalID(ctx context.Context, workspaceID string, secretKey string, externalID string) (*MessageHistory, error)

	// GetByContact retrieves message history for a specific contact
	GetByContact(ctx context.Context, workspaceID string, secretKey string, contactEmail string, limit, offset int) ([]*MessageHistory, int, error)

	// GetByBroadcast retrieves message history for a specific broadcast
	GetByBroadcast(ctx context.Context, workspaceID string, secretKey string, broadcastID string, limit, offset int) ([]*MessageHistory, int, error)

	// ListMessages retrieves message history with cursor-based pagination and filtering
	ListMessages(ctx context.Context, workspaceID string, secretKey string, params MessageListParams) ([]*MessageHistory, string, error)

	// SetStatusesIfNotSet updates multiple message statuses in a batch if they haven't been set before
	SetStatusesIfNotSet(ctx context.Context, workspaceID string, updates []MessageEventUpdate) error

	// SetClicked sets the clicked_at timestamp and ensures opened_at is also set
	SetClicked(ctx context.Context, workspaceID, id string, timestamp time.Time) error

	// SetOpened sets the opened_at timestamp if not already set
	SetOpened(ctx context.Context, workspaceID, id string, timestamp time.Time) error

	// GetBroadcastStats retrieves statistics for a broadcast
	GetBroadcastStats(ctx context.Context, workspaceID, broadcastID string) (*MessageHistoryStatusSum, error)

	// GetBroadcastVariationStats retrieves statistics for a specific variation of a broadcast
	GetBroadcastVariationStats(ctx context.Context, workspaceID, broadcastID, templateID string) (*MessageHistoryStatusSum, error)

	// DeleteForEmail deletes all message history records for a specific email
	DeleteForEmail(ctx context.Context, workspaceID, email string) error
}

// MessageHistoryService defines methods for interacting with message history
type MessageHistoryService interface {
	// ListMessages retrieves messages for a workspace with cursor-based pagination and filters
	ListMessages(ctx context.Context, workspaceID string, params MessageListParams) (*MessageListResult, error)

	// GetBroadcastStats retrieves statistics for a broadcast
	GetBroadcastStats(ctx context.Context, workspaceID, broadcastID string) (*MessageHistoryStatusSum, error)

	// GetBroadcastVariationStats retrieves statistics for a specific variation of a broadcast
	GetBroadcastVariationStats(ctx context.Context, workspaceID, broadcastID, templateID string) (*MessageHistoryStatusSum, error)
}

// MessageListParams contains parameters for listing messages with pagination and filtering
type MessageListParams struct {
	// Cursor-based pagination
	Cursor string `json:"cursor,omitempty"`
	Limit  int    `json:"limit,omitempty"`

	// Filters
	ID             string `json:"id,omitempty"`              // filter by message ID
	ExternalID     string `json:"external_id,omitempty"`     // filter by external ID
	ListID         string `json:"list_id,omitempty"`         // filter by list ID (array contains)
	Channel        string `json:"channel,omitempty"`         // email, sms, push, etc.
	ContactEmail   string `json:"contact_email,omitempty"`   // filter by contact
	BroadcastID    string `json:"broadcast_id,omitempty"`    // filter by broadcast
	TemplateID     string `json:"template_id,omitempty"`     // filter by template
	IsSent         *bool  `json:"is_sent,omitempty"`         // filter messages that are sent
	IsDelivered    *bool  `json:"is_delivered,omitempty"`    // filter messages that are delivered
	IsFailed       *bool  `json:"is_failed,omitempty"`       // filter messages that are failed
	IsOpened       *bool  `json:"is_opened,omitempty"`       // filter messages that are opened
	IsClicked      *bool  `json:"is_clicked,omitempty"`      // filter messages that are clicked
	IsBounced      *bool  `json:"is_bounced,omitempty"`      // filter messages that are bounced
	IsComplained   *bool  `json:"is_complained,omitempty"`   // filter messages that are complained
	IsUnsubscribed *bool  `json:"is_unsubscribed,omitempty"` // filter messages that are unsubscribed
	// Time range filters
	SentAfter     *time.Time `json:"sent_after,omitempty"`
	SentBefore    *time.Time `json:"sent_before,omitempty"`
	UpdatedAfter  *time.Time `json:"updated_after,omitempty"`
	UpdatedBefore *time.Time `json:"updated_before,omitempty"`
}

// FromQuery creates MessageListParams from HTTP query parameters
func (p *MessageListParams) FromQuery(query url.Values) error {
	// Parse cursor and basic string filters
	p.Cursor = query.Get("cursor")
	p.ID = query.Get("id")
	p.ExternalID = query.Get("external_id")
	p.ListID = query.Get("list_id")
	p.Channel = query.Get("channel")
	p.ContactEmail = query.Get("contact_email")
	p.BroadcastID = query.Get("broadcast_id")
	p.TemplateID = query.Get("template_id")

	// Parse limit
	if limitStr := query.Get("limit"); limitStr != "" {
		var limit int
		if err := json.Unmarshal([]byte(limitStr), &limit); err != nil {
			return fmt.Errorf("invalid limit value: %s", limitStr)
		}
		p.Limit = limit
	}

	// Parse isSent if provided
	if isSentStr := query.Get("is_sent"); isSentStr != "" {
		var isSent bool
		if err := json.Unmarshal([]byte(isSentStr), &isSent); err != nil {
			return fmt.Errorf("invalid is_sent value: %s", isSentStr)
		}
		p.IsSent = &isSent
	}

	// Parse isDelivered if provided
	if isDeliveredStr := query.Get("is_delivered"); isDeliveredStr != "" {
		var isDelivered bool
		if err := json.Unmarshal([]byte(isDeliveredStr), &isDelivered); err != nil {
			return fmt.Errorf("invalid is_delivered value: %s", isDeliveredStr)
		}
		p.IsDelivered = &isDelivered
	}
	// Parse isFailed if provided
	if isFailedStr := query.Get("is_failed"); isFailedStr != "" {
		var isFailed bool
		if err := json.Unmarshal([]byte(isFailedStr), &isFailed); err != nil {
			return fmt.Errorf("invalid is_failed value: %s", isFailedStr)
		}
		p.IsFailed = &isFailed
	}
	// Parse isOpened if provided
	if isOpenedStr := query.Get("is_opened"); isOpenedStr != "" {
		var isOpened bool
		if err := json.Unmarshal([]byte(isOpenedStr), &isOpened); err != nil {
			return fmt.Errorf("invalid is_opened value: %s", isOpenedStr)
		}
		p.IsOpened = &isOpened
	}

	// Parse isClicked if provided
	if isClickedStr := query.Get("is_clicked"); isClickedStr != "" {
		var isClicked bool
		if err := json.Unmarshal([]byte(isClickedStr), &isClicked); err != nil {
			return fmt.Errorf("invalid is_clicked value: %s", isClickedStr)
		}
		p.IsClicked = &isClicked
	}

	// Parse isBounced if provided
	if isBouncedStr := query.Get("is_bounced"); isBouncedStr != "" {
		var isBounced bool
		if err := json.Unmarshal([]byte(isBouncedStr), &isBounced); err != nil {
			return fmt.Errorf("invalid is_bounced value: %s", isBouncedStr)
		}
		p.IsBounced = &isBounced
	}
	// Parse isComplained if provided
	if isComplainedStr := query.Get("is_complained"); isComplainedStr != "" {
		var isComplained bool
		if err := json.Unmarshal([]byte(isComplainedStr), &isComplained); err != nil {
			return fmt.Errorf("invalid is_complained value: %s", isComplainedStr)
		}
		p.IsComplained = &isComplained
	}
	// Parse isUnsubscribed if provided
	if isUnsubscribedStr := query.Get("is_unsubscribed"); isUnsubscribedStr != "" {
		var isUnsubscribed bool
		if err := json.Unmarshal([]byte(isUnsubscribedStr), &isUnsubscribed); err != nil {
			return fmt.Errorf("invalid is_unsubscribed value: %s", isUnsubscribedStr)
		}
		p.IsUnsubscribed = &isUnsubscribed
	}
	// Parse time filters if provided
	if err := parseTimeParam(query, "sent_after", &p.SentAfter); err != nil {
		return err
	}
	if err := parseTimeParam(query, "sent_before", &p.SentBefore); err != nil {
		return err
	}
	if err := parseTimeParam(query, "updated_after", &p.UpdatedAfter); err != nil {
		return err
	}
	if err := parseTimeParam(query, "updated_before", &p.UpdatedBefore); err != nil {
		return err
	}

	// Validate all parameters
	return p.Validate()
}

// Helper function to parse time parameters
func parseTimeParam(query url.Values, paramName string, target **time.Time) error {
	if paramStr := query.Get(paramName); paramStr != "" {
		parsedTime, err := time.Parse(time.RFC3339, paramStr)
		if err != nil {
			return fmt.Errorf("invalid %s time format, expected RFC3339: %v", paramName, err)
		}
		*target = &parsedTime
	}
	return nil
}

func (p *MessageListParams) Validate() error {
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

	// Validate channel
	if p.Channel != "" {
		// Use govalidator to check if channel is valid
		if !govalidator.IsIn(p.Channel, "email", "sms", "push") {
			return fmt.Errorf("invalid channel type: %s", p.Channel)
		}
	}

	// Validate contact email if provided
	if p.ContactEmail != "" && !govalidator.IsEmail(p.ContactEmail) {
		return fmt.Errorf("invalid contact email format")
	}

	// Note: BroadcastID and TemplateID are not validated as UUIDs
	// They can be any non-empty string format

	// Validate time ranges
	if p.SentAfter != nil && p.SentBefore != nil {
		if p.SentAfter.After(*p.SentBefore) {
			return fmt.Errorf("sent_after must be before sent_before")
		}
	}

	if p.UpdatedAfter != nil && p.UpdatedBefore != nil {
		if p.UpdatedAfter.After(*p.UpdatedBefore) {
			return fmt.Errorf("updated_after must be before updated_before")
		}
	}

	return nil
}

// MessageListResult contains the result of a ListMessages operation
type MessageListResult struct {
	Messages   []*MessageHistory `json:"messages"`
	NextCursor string            `json:"next_cursor,omitempty"`
	HasMore    bool              `json:"has_more"`
}
