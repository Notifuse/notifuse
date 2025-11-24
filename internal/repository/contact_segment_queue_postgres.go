package repository

import (
	"context"
	"fmt"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/lib/pq"
)

type contactSegmentQueueRepository struct {
	workspaceRepo domain.WorkspaceRepository
}

// NewContactSegmentQueueRepository creates a new contact segment queue repository
func NewContactSegmentQueueRepository(workspaceRepo domain.WorkspaceRepository) domain.ContactSegmentQueueRepository {
	return &contactSegmentQueueRepository{
		workspaceRepo: workspaceRepo,
	}
}

// GetPendingEmails retrieves and locks emails that need segment recomputation
// Uses FOR UPDATE SKIP LOCKED to prevent concurrent processing
// Applies a 15-second debounce to avoid processing contacts being updated rapidly
func (r *contactSegmentQueueRepository) GetPendingEmails(ctx context.Context, workspaceID string, limit int) ([]string, error) {
	workspaceDB, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace connection: %w", err)
	}

	// Use FOR UPDATE SKIP LOCKED to:
	// 1. Lock the rows being selected (prevent other workers from getting them)
	// 2. Skip any rows already locked by another transaction
	// This ensures each worker processes different contacts
	//
	// WHERE queued_at < NOW() - INTERVAL '15 seconds':
	// Debounce logic - only process contacts that were queued at least 15 seconds ago
	// If a contact is updated multiple times rapidly, we wait until updates stop
	query := `
		SELECT email
		FROM contact_segment_queue
		WHERE queued_at < NOW() - INTERVAL '15 seconds'
		ORDER BY queued_at ASC
		LIMIT $1
		FOR UPDATE SKIP LOCKED
	`

	rows, err := workspaceDB.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query pending emails: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var emails []string
	for rows.Next() {
		var email string
		if err := rows.Scan(&email); err != nil {
			return nil, fmt.Errorf("failed to scan email: %w", err)
		}
		emails = append(emails, email)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating emails: %w", err)
	}

	return emails, nil
}

// RemoveFromQueue removes an email from the queue after processing
func (r *contactSegmentQueueRepository) RemoveFromQueue(ctx context.Context, workspaceID string, email string) error {
	workspaceDB, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get workspace connection: %w", err)
	}

	query := `DELETE FROM contact_segment_queue WHERE email = $1`

	_, err = workspaceDB.ExecContext(ctx, query, email)
	if err != nil {
		return fmt.Errorf("failed to remove email from queue: %w", err)
	}

	return nil
}

// RemoveBatchFromQueue removes multiple emails from the queue
func (r *contactSegmentQueueRepository) RemoveBatchFromQueue(ctx context.Context, workspaceID string, emails []string) error {
	if len(emails) == 0 {
		return nil
	}

	workspaceDB, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get workspace connection: %w", err)
	}

	query := `DELETE FROM contact_segment_queue WHERE email = ANY($1)`

	_, err = workspaceDB.ExecContext(ctx, query, pq.Array(emails))
	if err != nil {
		return fmt.Errorf("failed to remove emails from queue: %w", err)
	}

	return nil
}

// GetQueueSize returns the number of contacts in the queue
func (r *contactSegmentQueueRepository) GetQueueSize(ctx context.Context, workspaceID string) (int, error) {
	workspaceDB, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return 0, fmt.Errorf("failed to get workspace connection: %w", err)
	}

	query := `SELECT COUNT(*) FROM contact_segment_queue`

	var count int
	err = workspaceDB.QueryRowContext(ctx, query).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to get queue size: %w", err)
	}

	return count, nil
}

// ClearQueue removes all items from the queue
func (r *contactSegmentQueueRepository) ClearQueue(ctx context.Context, workspaceID string) error {
	workspaceDB, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get workspace connection: %w", err)
	}

	query := `DELETE FROM contact_segment_queue`

	_, err = workspaceDB.ExecContext(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to clear queue: %w", err)
	}

	return nil
}
