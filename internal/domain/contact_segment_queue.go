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

// ContactSegmentQueueRepository defines the interface for contact segment queue operations.
// Claiming and re-enqueueing pending contacts is done by the queue processor
// (ContactSegmentQueueProcessor.claimBatch/requeueBatch), which needs statement-level
// control of transaction and cancellation semantics.
type ContactSegmentQueueRepository interface {
	// RemoveFromQueue removes an email from the queue after processing
	RemoveFromQueue(ctx context.Context, workspaceID string, email string) error

	// GetQueueSize returns the number of contacts in the queue
	GetQueueSize(ctx context.Context, workspaceID string) (int, error)

	// ClearQueue removes all items from the queue
	ClearQueue(ctx context.Context, workspaceID string) error
}
