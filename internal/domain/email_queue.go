package domain

import (
	"context"
	"database/sql"
	"os"
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

	// Message history tracking fields
	TemplateVersion int                    `json:"template_version"`        // Needed for message_history
	ListID          string                 `json:"list_id,omitempty"`       // For broadcasts
	TemplateData    map[string]interface{} `json:"template_data,omitempty"` // For message history logging
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

// EmailQueueStats provides queue statistics for a workspace
type EmailQueueStats struct {
	Pending    int64 `json:"pending"`
	Processing int64 `json:"processing"`
	Failed     int64 `json:"failed"`
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

	// Delete removes a queue entry (used when max retries exhausted)
	Delete(ctx context.Context, workspaceID string, entryID string) error

	// SetNextRetry updates next_retry_at WITHOUT incrementing attempts
	// Used by circuit breaker to schedule retry without burning retry attempts
	SetNextRetry(ctx context.Context, workspaceID string, entryID string, nextRetry time.Time) error

	// GetStats returns queue statistics for a workspace
	GetStats(ctx context.Context, workspaceID string) (*EmailQueueStats, error)

	// GetBySourceID retrieves queue entries by source type and ID
	// Useful for tracking broadcast/automation progress
	GetBySourceID(ctx context.Context, workspaceID string, sourceType EmailQueueSourceType, sourceID string) ([]*EmailQueueEntry, error)

	// CountBySourceAndStatus counts entries by source and status
	CountBySourceAndStatus(ctx context.Context, workspaceID string, sourceType EmailQueueSourceType, sourceID string, status EmailQueueStatus) (int64, error)
}

// getEmailQueueRetryBase returns the base retry interval for exponential backoff.
// Can be overridden via EMAIL_QUEUE_RETRY_BASE environment variable for testing.
// Default is 1 minute.
func getEmailQueueRetryBase() time.Duration {
	if base := os.Getenv("EMAIL_QUEUE_RETRY_BASE"); base != "" {
		if d, err := time.ParseDuration(base); err == nil {
			return d
		}
	}
	return 1 * time.Minute
}

// CalculateNextRetryTime calculates the next retry time using exponential backoff
// Backoff: base, 2*base, 4*base for attempts 1, 2, 3 (default base = 1min)
func CalculateNextRetryTime(attempts int) time.Time {
	if attempts <= 0 {
		attempts = 1
	}
	// 2^(attempts-1) * base
	base := getEmailQueueRetryBase()
	multiplier := 1 << uint(attempts-1)
	return time.Now().UTC().Add(time.Duration(multiplier) * base)
}
