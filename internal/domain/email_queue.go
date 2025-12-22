package domain

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

//go:generate mockgen -destination mocks/mock_email_queue_repository.go -package mocks github.com/Notifuse/notifuse/internal/domain EmailQueueRepository

// EmailQueueStatus represents the status of a queued email
type EmailQueueStatus string

const (
	EmailQueueStatusPending    EmailQueueStatus = "pending"
	EmailQueueStatusProcessing EmailQueueStatus = "processing"
	EmailQueueStatusFailed     EmailQueueStatus = "failed"
	// Note: There is no "sent" status - entries are deleted immediately after successful send
)

// EmailQueueSourceType identifies the origin of the queued email
type EmailQueueSourceType string

const (
	EmailQueueSourceBroadcast  EmailQueueSourceType = "broadcast"
	EmailQueueSourceAutomation EmailQueueSourceType = "automation"
)

// Default priority for marketing emails (broadcasts and automations)
const EmailQueuePriorityMarketing = 5

// EmailQueueEntry represents a single email in the queue
type EmailQueueEntry struct {
	ID            string               `json:"id"`
	Status        EmailQueueStatus     `json:"status"`
	Priority      int                  `json:"priority"`
	SourceType    EmailQueueSourceType `json:"source_type"`
	SourceID      string               `json:"source_id"` // BroadcastID or AutomationID
	IntegrationID string               `json:"integration_id"`
	ProviderKind  EmailProviderKind    `json:"provider_kind"`

	// Email identification
	ContactEmail string `json:"contact_email"`
	MessageID    string `json:"message_id"`
	TemplateID   string `json:"template_id"`

	// Serialized payload for sending (contains all data needed to send)
	Payload EmailQueuePayload `json:"payload"`

	// Retry tracking
	Attempts    int        `json:"attempts"`
	MaxAttempts int        `json:"max_attempts"`
	LastError   *string    `json:"last_error,omitempty"`
	NextRetryAt *time.Time `json:"next_retry_at,omitempty"`

	// Timestamps
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	ProcessedAt *time.Time `json:"processed_at,omitempty"`
}

// EmailQueuePayload contains all data needed to send the email
// This is stored as JSONB in the database
type EmailQueuePayload struct {
	// Email content (compiled and ready to send)
	FromAddress string `json:"from_address"`
	FromName    string `json:"from_name"`
	Subject     string `json:"subject"`
	HTMLContent string `json:"html_content"`

	// Options
	EmailOptions EmailOptions `json:"email_options"`

	// Provider configuration (rate limit needed for worker)
	RateLimitPerMinute int `json:"rate_limit_per_minute"`

	// Provider settings (encrypted, will be decrypted by worker)
	ProviderSettings map[string]interface{} `json:"provider_settings"`
}

// ToSendEmailProviderRequest converts the payload to a SendEmailProviderRequest
// The provider must be passed in separately as it's not stored in the payload
func (p *EmailQueuePayload) ToSendEmailProviderRequest(workspaceID, integrationID, messageID, toEmail string, provider *EmailProvider) *SendEmailProviderRequest {
	return &SendEmailProviderRequest{
		WorkspaceID:   workspaceID,
		IntegrationID: integrationID,
		MessageID:     messageID,
		FromAddress:   p.FromAddress,
		FromName:      p.FromName,
		To:            toEmail,
		Subject:       p.Subject,
		Content:       p.HTMLContent,
		Provider:      provider,
		EmailOptions:  p.EmailOptions,
	}
}

// EmailQueueDeadLetter stores permanently failed emails for investigation
type EmailQueueDeadLetter struct {
	ID              string               `json:"id"`
	OriginalEntryID string               `json:"original_entry_id"`
	SourceType      EmailQueueSourceType `json:"source_type"`
	SourceID        string               `json:"source_id"`
	ContactEmail    string               `json:"contact_email"`
	MessageID       string               `json:"message_id"`
	Payload         EmailQueuePayload    `json:"payload"`
	FinalError      string               `json:"final_error"`
	Attempts        int                  `json:"attempts"`
	CreatedAt       time.Time            `json:"created_at"` // Original creation time
	FailedAt        time.Time            `json:"failed_at"`
}

// EmailQueueStats provides queue statistics for a workspace
type EmailQueueStats struct {
	Pending    int64 `json:"pending"`
	Processing int64 `json:"processing"`
	Failed     int64 `json:"failed"`
	DeadLetter int64 `json:"dead_letter"`
	// Note: Sent entries are deleted immediately, not tracked in stats
}

// EmailQueueRepository defines data access for the email queue
type EmailQueueRepository interface {
	// Enqueue adds emails to the queue
	Enqueue(ctx context.Context, workspaceID string, entries []*EmailQueueEntry) error

	// EnqueueTx adds emails to the queue within an existing transaction
	EnqueueTx(ctx context.Context, tx *sql.Tx, entries []*EmailQueueEntry) error

	// FetchPending retrieves pending emails for processing
	// Uses FOR UPDATE SKIP LOCKED to allow concurrent workers
	// Orders by priority ASC (lower = higher priority), then created_at ASC
	FetchPending(ctx context.Context, workspaceID string, limit int) ([]*EmailQueueEntry, error)

	// MarkAsProcessing atomically marks an entry as processing
	MarkAsProcessing(ctx context.Context, workspaceID string, id string) error

	// MarkAsSent deletes the entry after successful send
	// (entries are removed immediately rather than marked with a "sent" status)
	MarkAsSent(ctx context.Context, workspaceID string, id string) error

	// MarkAsFailed marks an entry as failed and schedules retry
	MarkAsFailed(ctx context.Context, workspaceID string, id string, errorMsg string, nextRetryAt *time.Time) error

	// MoveToDeadLetter moves a permanently failed entry to the dead letter queue
	MoveToDeadLetter(ctx context.Context, workspaceID string, entry *EmailQueueEntry, finalError string) error

	// GetStats returns queue statistics for a workspace
	GetStats(ctx context.Context, workspaceID string) (*EmailQueueStats, error)

	// GetBySourceID retrieves queue entries by source type and ID
	// Useful for tracking broadcast/automation progress
	GetBySourceID(ctx context.Context, workspaceID string, sourceType EmailQueueSourceType, sourceID string) ([]*EmailQueueEntry, error)

	// CountBySourceAndStatus counts entries by source and status
	CountBySourceAndStatus(ctx context.Context, workspaceID string, sourceType EmailQueueSourceType, sourceID string, status EmailQueueStatus) (int64, error)

	// CleanupDeadLetter removes old dead letter entries
	CleanupDeadLetter(ctx context.Context, workspaceID string, olderThan time.Duration) (int64, error)

	// GetDeadLetterEntries retrieves dead letter entries for investigation
	GetDeadLetterEntries(ctx context.Context, workspaceID string, limit, offset int) ([]*EmailQueueDeadLetter, int64, error)

	// RetryDeadLetter moves a dead letter entry back to the queue for retry
	RetryDeadLetter(ctx context.Context, workspaceID string, deadLetterID string) error
}

// CalculateNextRetryTime calculates the next retry time using exponential backoff
// Backoff: 1min, 2min, 4min for attempts 1, 2, 3
func CalculateNextRetryTime(attempts int) time.Time {
	if attempts <= 0 {
		attempts = 1
	}
	// 2^(attempts-1) minutes
	backoffMinutes := 1 << uint(attempts-1)
	return time.Now().UTC().Add(time.Duration(backoffMinutes) * time.Minute)
}

// CleanupDeadLetterRequest represents a request to clean up old dead letter entries
type CleanupDeadLetterRequest struct {
	WorkspaceID    string `json:"workspace_id"`
	RetentionHours int    `json:"retention_hours"` // Delete entries older than this many hours (default: 720 = 30 days)
}

// Validate validates the cleanup request
func (r *CleanupDeadLetterRequest) Validate() error {
	if r.WorkspaceID == "" {
		return fmt.Errorf("workspace_id is required")
	}
	if r.RetentionHours < 0 {
		r.RetentionHours = 0
	}
	// Default to 30 days if not specified
	if r.RetentionHours == 0 {
		r.RetentionHours = 720 // 30 days
	}
	return nil
}

// GetEmailQueueStatsRequest represents a request to get queue statistics
type GetEmailQueueStatsRequest struct {
	WorkspaceID string `json:"workspace_id"`
}

// FromURLParams populates the request from URL parameters
func (r *GetEmailQueueStatsRequest) FromURLParams(params map[string][]string) error {
	if ids, ok := params["workspace_id"]; ok && len(ids) > 0 {
		r.WorkspaceID = ids[0]
	}
	return r.Validate()
}

// Validate validates the request
func (r *GetEmailQueueStatsRequest) Validate() error {
	if r.WorkspaceID == "" {
		return fmt.Errorf("workspace_id is required")
	}
	return nil
}

// GetDeadLetterEntriesRequest represents a request to get dead letter entries
type GetDeadLetterEntriesRequest struct {
	WorkspaceID string `json:"workspace_id"`
	Limit       int    `json:"limit"`
	Offset      int    `json:"offset"`
}

// FromURLParams populates the request from URL parameters
func (r *GetDeadLetterEntriesRequest) FromURLParams(params map[string][]string) error {
	if ids, ok := params["workspace_id"]; ok && len(ids) > 0 {
		r.WorkspaceID = ids[0]
	}
	if limits, ok := params["limit"]; ok && len(limits) > 0 {
		val, err := ParseIntParam(limits[0])
		if err == nil {
			r.Limit = val
		} else {
			r.Limit = 50
		}
	} else {
		r.Limit = 50
	}
	if offsets, ok := params["offset"]; ok && len(offsets) > 0 {
		val, err := ParseIntParam(offsets[0])
		if err == nil {
			r.Offset = val
		}
	}
	return r.Validate()
}

// Validate validates the request
func (r *GetDeadLetterEntriesRequest) Validate() error {
	if r.WorkspaceID == "" {
		return fmt.Errorf("workspace_id is required")
	}
	if r.Limit <= 0 {
		r.Limit = 50
	}
	if r.Limit > 100 {
		r.Limit = 100
	}
	if r.Offset < 0 {
		r.Offset = 0
	}
	return nil
}

// RetryDeadLetterRequest represents a request to retry a dead letter entry
type RetryDeadLetterRequest struct {
	WorkspaceID  string `json:"workspace_id"`
	DeadLetterID string `json:"dead_letter_id"`
}

// Validate validates the request
func (r *RetryDeadLetterRequest) Validate() error {
	if r.WorkspaceID == "" {
		return fmt.Errorf("workspace_id is required")
	}
	if r.DeadLetterID == "" {
		return fmt.Errorf("dead_letter_id is required")
	}
	return nil
}
