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
	FromName       *string  `json:"from_name,omitempty"`
	Subject        *string  `json:"subject,omitempty"`
	SubjectPreview *string  `json:"subject_preview,omitempty"`
	CC             []string `json:"cc,omitempty"`
	BCC            []string `json:"bcc,omitempty"`
	ReplyTo        string   `json:"reply_to,omitempty"`

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
	ID                          string               `json:"id"`
	ExternalID                  *string              `json:"external_id,omitempty"`     // For idempotency checks
	SMTPMessageID               *string              `json:"smtp_message_id,omitempty"` // Recipient-visible RFC Message-ID for reply matching; set only for exit_on_reply sends
	ContactEmail                string               `json:"contact_email"`
	BroadcastID                 *string              `json:"broadcast_id,omitempty"`
	AutomationID                *string              `json:"automation_id,omitempty"`                 // Automation this message was sent from (nullable for broadcasts/transactional)
	TransactionalNotificationID *string              `json:"transactional_notification_id,omitempty"` // Transactional notification this message was sent from
	ListID                      *string              `json:"list_id,omitempty"`                       // List this message was sent to (nullable for transactional emails)
	TemplateID                  string               `json:"template_id"`
	TemplateVersion             int64                `json:"template_version"`
	Channel                     string               `json:"channel"` // email, sms, push, etc.
	StatusInfo                  *string              `json:"status_info,omitempty"`
	MessageData                 MessageData          `json:"message_data"`
	ChannelOptions              *ChannelOptions      `json:"channel_options,omitempty"` // Channel-specific delivery options
	Attachments                 []AttachmentMetadata `json:"attachments,omitempty"`

	// Event timestamps
	SentAt         time.Time  `json:"sent_at"`
	DeliveredAt    *time.Time `json:"delivered_at,omitempty"`
	FailedAt       *time.Time `json:"failed_at,omitempty"`
	OpenedAt       *time.Time `json:"opened_at,omitempty"`
	ClickedAt      *time.Time `json:"clicked_at,omitempty"`
	BouncedAt      *time.Time `json:"bounced_at,omitempty"`
	ComplainedAt   *time.Time `json:"complained_at,omitempty"`
	UnsubscribedAt *time.Time `json:"unsubscribed_at,omitempty"`

	// OpensTracked and ClicksTracked say whether the email carried the open
	// pixel and the click redirect. nil on a message sent before they existed,
	// which counts as tracked.
	OpensTracked  *bool `json:"opens_tracked,omitempty"`
	ClicksTracked *bool `json:"clicks_tracked,omitempty"`

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

	// Sends (not failed) whose email was tracked for opens or clicks: the
	// denominators of the open and click rates.
	TotalOpensTracked  int `json:"total_opens_tracked"`
	TotalClicksTracked int `json:"total_clicks_tracked"`
}

// LinkClickStats aggregates clicks for a single destination URL within a broadcast
type LinkClickStats struct {
	URL          string `json:"url"`
	TotalClicks  int64  `json:"total_clicks"`
	UniqueClicks int64  `json:"unique_clicks"` // messages that clicked this URL at least once
}

// MessageHistoryRepository defines methods for message history persistence
type MessageHistoryRepository interface {
	// Create adds a new message history record
	Create(ctx context.Context, workspaceID string, secretKey string, message *MessageHistory) error

	// Upsert creates or updates a message history record (for retry handling)
	// On conflict, updates failed_at, status_info, and updated_at fields
	Upsert(ctx context.Context, workspaceID string, secretKey string, message *MessageHistory) error

	// GetByExternalID retrieves a message history by external ID for idempotency checks
	GetByExternalID(ctx context.Context, workspaceID string, secretKey string, externalID string) (*MessageHistory, error)

	// GetBySMTPMessageID looks up a send by its recipient-visible RFC Message-ID
	// (for matching an inbound reply's In-Reply-To). Returns (nil, nil) when there
	// is no match. Only id, contact_email and automation_id are populated.
	GetBySMTPMessageID(ctx context.Context, workspaceID string, smtpMessageID string) (*MessageHistory, error)

	// ListMessages retrieves message history with cursor-based pagination and filtering
	ListMessages(ctx context.Context, workspaceID string, secretKey string, params MessageListParams) ([]*MessageHistory, string, error)

	// CountMessages counts the messages matching the filter, in total and by status
	CountMessages(ctx context.Context, workspaceID string, filter MessageFilter) (*MessageCounts, error)

	// SetStatusesIfNotSet updates multiple message statuses in a batch if they haven't been set before.
	// An unsubscribe is not recorded on a message whose address an erasure redacted.
	SetStatusesIfNotSet(ctx context.Context, workspaceID string, updates []MessageEventUpdate) error

	// SetClicked sets the clicked_at timestamp and ensures opened_at is also set.
	// When clickedURL is non-empty, the per-URL click counters in clicked_links are updated too.
	SetClicked(ctx context.Context, workspaceID, id string, timestamp time.Time, clickedURL string, verified bool) error

	// SetOpened sets the opened_at timestamp if not already set
	SetOpened(ctx context.Context, workspaceID, id string, timestamp time.Time) error

	// GetBroadcastStats retrieves statistics for a broadcast
	// (message history only; the queue is not this repository's concern)
	GetBroadcastStats(ctx context.Context, workspaceID, broadcastID string) (*MessageHistoryStatusSum, error)

	// GetBroadcastVariationStats retrieves statistics for a specific variation of a broadcast
	GetBroadcastVariationStats(ctx context.Context, workspaceID, broadcastID, templateID string) (*MessageHistoryStatusSum, error)

	// GetBroadcastLinkStats retrieves per-URL click statistics for a broadcast.
	// When templateID is non-empty, only clicks on messages sent with that template
	// (i.e. that A/B variation) are counted; empty templateID covers the whole broadcast.
	GetBroadcastLinkStats(ctx context.Context, workspaceID, broadcastID, templateID string) ([]LinkClickStats, error)

	// WasSentForList reports whether message messageID was sent to email for listID: by
	// the list stored on the message, or, for an older automation message stored without
	// one, by the automation's current list
	WasSentForList(ctx context.Context, workspaceID, messageID, email, listID string) (bool, error)

	// GetRecipientAndList returns who message id was sent to and the list it was sent for,
	// read as WasSentForList reads it, so the automation's current list for an older
	// automation message stored without one (nil for a transactional email). Only id,
	// contact_email and list_id are populated. Returns (nil, nil) when there is no such
	// message.
	GetRecipientAndList(ctx context.Context, workspaceID, id string) (*MessageHistory, error)

	// GetForWebVersion returns what the web version of message id is rendered from: id,
	// broadcast_id, automation_id, template_id, template_version and message_data,
	// decrypted with secretKey. An erased message comes back with empty data. Returns
	// (nil, nil) when there is no such message.
	GetForWebVersion(ctx context.Context, workspaceID, secretKey, id string) (*MessageHistory, error)

	// GetRecipients returns, for each of ids that names a message, the address it was sent
	// to. Ids that name no message are absent from the map, as are the messages whose
	// address an erasure redacted: they name no one.
	GetRecipients(ctx context.Context, workspaceID string, ids []string) (map[string]string, error)

	// DeleteForEmail deletes all message history records for a specific email
	DeleteForEmail(ctx context.Context, workspaceID, email string) error
}

// BroadcastDeliveryStats pairs what message history knows about a broadcast with
// what its queue is still holding. The two answer different questions and only
// together say whether a campaign is finished: history records the outcome of each
// attempt, while the queue is the only thing that knows whether anything is still
// on its way out, or has been given up on.
type BroadcastDeliveryStats struct {
	Stats *MessageHistoryStatusSum `json:"stats"`
	// Queue is nil when the counts could not be read. The stats still stand on
	// their own, so a queue that cannot be reached degrades the page rather than
	// emptying it.
	Queue *EmailQueueSourceCounts `json:"queue,omitempty"`
}

// MessageHistoryService defines methods for interacting with message history
type MessageHistoryService interface {
	// ListMessages retrieves messages for a workspace with cursor-based pagination and filters
	ListMessages(ctx context.Context, workspaceID string, params MessageListParams) (*MessageListResult, error)

	// CountMessages counts a workspace's messages matching the filter, in total and by status
	CountMessages(ctx context.Context, workspaceID string, filter MessageFilter) (*MessageCounts, error)

	// GetBroadcastStats retrieves statistics for a broadcast, together with what its queue is still holding
	GetBroadcastStats(ctx context.Context, workspaceID, broadcastID string) (*BroadcastDeliveryStats, error)

	// GetBroadcastVariationStats retrieves statistics for a specific variation of a broadcast
	GetBroadcastVariationStats(ctx context.Context, workspaceID, broadcastID, templateID string) (*MessageHistoryStatusSum, error)

	// GetBroadcastLinkStats retrieves per-URL click statistics for a broadcast.
	// When templateID is non-empty, only clicks on messages sent with that template
	// (i.e. that A/B variation) are counted; empty templateID covers the whole broadcast.
	GetBroadcastLinkStats(ctx context.Context, workspaceID, broadcastID, templateID string) ([]LinkClickStats, error)
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
	AutomationID   string `json:"automation_id,omitempty"`   // filter by automation
	TemplateID     string `json:"template_id,omitempty"`     // filter by template
	ClickedURL     string `json:"clicked_url,omitempty"`     // filter by a link clicked, exact URL as clicked_links holds it (requires broadcast_id)
	IsSent         *bool  `json:"is_sent,omitempty"`         // filter messages that are sent
	IsDelivered    *bool  `json:"is_delivered,omitempty"`    // filter messages that are delivered
	IsFailed       *bool  `json:"is_failed,omitempty"`       // filter messages that are failed
	IsOpened       *bool  `json:"is_opened,omitempty"`       // filter messages that are opened
	IsClicked      *bool  `json:"is_clicked,omitempty"`      // filter messages that are clicked
	IsBounced      *bool  `json:"is_bounced,omitempty"`      // filter messages that are bounced
	IsComplained   *bool  `json:"is_complained,omitempty"`   // filter messages that are complained
	IsUnsubscribed *bool  `json:"is_unsubscribed,omitempty"` // filter messages that are unsubscribed

	TransactionalNotificationID string `json:"transactional_notification_id,omitempty"` // filter by transactional notification
	Origin                      string `json:"origin,omitempty"`                        // broadcast, automation, transactional or other (MessageOrigins)

	// Time range filters
	SentAfter     *time.Time `json:"sent_after,omitempty"`
	SentBefore    *time.Time `json:"sent_before,omitempty"`
	UpdatedAfter  *time.Time `json:"updated_after,omitempty"`
	UpdatedBefore *time.Time `json:"updated_before,omitempty"`
	CreatedAfter  *time.Time `json:"created_after,omitempty"`
	CreatedBefore *time.Time `json:"created_before,omitempty"`
}

// FromQuery creates MessageListParams from HTTP query parameters
func (p *MessageListParams) FromQuery(query url.Values) error {
	p.Cursor = query.Get("cursor")

	if limitStr := query.Get("limit"); limitStr != "" {
		var limit int
		if err := json.Unmarshal([]byte(limitStr), &limit); err != nil {
			return fmt.Errorf("invalid limit value: %s", limitStr)
		}
		p.Limit = limit
	}

	var f MessageFilter
	if err := f.FromQuery(query); err != nil {
		return err
	}
	p.setFilter(f)

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

	return p.Filter().Validate()
}

// MessageListResult contains the result of a ListMessages operation
type MessageListResult struct {
	Messages   []*MessageHistory `json:"messages"`
	NextCursor string            `json:"next_cursor,omitempty"`
	HasMore    bool              `json:"has_more"`
}
