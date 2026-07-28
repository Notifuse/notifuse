package repository

import (
	"context"
	"fmt"

	"github.com/Notifuse/notifuse/internal/domain"
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
