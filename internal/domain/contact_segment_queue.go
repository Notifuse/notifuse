package domain

import (
	"context"
	"time"
)

//go:generate mockgen -destination mocks/mock_contact_segment_queue_repository.go -package mocks github.com/Notifuse/notifuse/internal/domain ContactSegmentQueueRepository

// ContactSegmentQueueItem represents a contact that needs segment recomputation
type ContactSegmentQueueItem struct {
	Email    string    `json:"email"`
	QueuedAt time.Time `json:"queued_at"`
}

// ContactSegmentQueueRepository defines the interface for contact segment queue operations
type ContactSegmentQueueRepository interface {
	// GetPendingEmails retrieves emails that need segment recomputation
	GetPendingEmails(ctx context.Context, workspaceID string, limit int) ([]string, error)

	// RemoveFromQueue removes an email from the queue after processing
	RemoveFromQueue(ctx context.Context, workspaceID string, email string) error

	// RemoveBatchFromQueue removes multiple emails from the queue
	RemoveBatchFromQueue(ctx context.Context, workspaceID string, emails []string) error

	// GetQueueSize returns the number of contacts in the queue
	GetQueueSize(ctx context.Context, workspaceID string) (int, error)

	// ClearQueue removes all items from the queue
	ClearQueue(ctx context.Context, workspaceID string) error
}

