package repository

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/Notifuse/notifuse/internal/domain"
)

// AttachmentRepository implements domain.AttachmentRepository
type AttachmentRepository struct {
	workspaceRepo domain.WorkspaceRepository
}

// NewAttachmentRepository creates a new attachment repository
func NewAttachmentRepository(workspaceRepo domain.WorkspaceRepository) *AttachmentRepository {
	return &AttachmentRepository{
		workspaceRepo: workspaceRepo,
	}
}

// Store saves an attachment and returns its checksum
func (r *AttachmentRepository) Store(ctx context.Context, workspaceID string, record *domain.AttachmentRecord) error {
	// Get the workspace database connection
	workspaceDB, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get workspace connection: %w", err)
	}

	query := `
		INSERT INTO message_attachments (checksum, content, content_type, size_bytes, created_at)
		VALUES ($1, $2, $3, $4, CURRENT_TIMESTAMP)
		ON CONFLICT (checksum) DO NOTHING
	`

	_, err = workspaceDB.ExecContext(
		ctx,
		query,
		record.Checksum,
		record.Content,
		record.ContentType,
		record.SizeBytes,
	)

	if err != nil {
		return fmt.Errorf("failed to store attachment: %w", err)
	}

	return nil
}

// Get retrieves an attachment by checksum
func (r *AttachmentRepository) Get(ctx context.Context, workspaceID string, checksum string) (*domain.AttachmentRecord, error) {
	// Get the workspace database connection
	workspaceDB, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace connection: %w", err)
	}

	query := `
		SELECT checksum, content, content_type, size_bytes
		FROM message_attachments
		WHERE checksum = $1
	`

	record := &domain.AttachmentRecord{}
	err = workspaceDB.QueryRowContext(ctx, query, checksum).Scan(
		&record.Checksum,
		&record.Content,
		&record.ContentType,
		&record.SizeBytes,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("attachment not found: %s", checksum)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to get attachment: %w", err)
	}

	return record, nil
}

// Exists checks if an attachment exists by checksum
func (r *AttachmentRepository) Exists(ctx context.Context, workspaceID string, checksum string) (bool, error) {
	// Get the workspace database connection
	workspaceDB, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return false, fmt.Errorf("failed to get workspace connection: %w", err)
	}

	query := `
		SELECT EXISTS(SELECT 1 FROM message_attachments WHERE checksum = $1)
	`

	var exists bool
	err = workspaceDB.QueryRowContext(ctx, query, checksum).Scan(&exists)

	if err != nil {
		return false, fmt.Errorf("failed to check attachment existence: %w", err)
	}

	return exists, nil
}
