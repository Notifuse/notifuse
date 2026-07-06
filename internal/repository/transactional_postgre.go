package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
)

// TransactionalNotificationRepository implements domain.TransactionalNotificationRepository
type TransactionalNotificationRepository struct {
	workspaceRepo domain.WorkspaceRepository
}

// NewTransactionalNotificationRepository creates a new instance of the repository
func NewTransactionalNotificationRepository(workspaceRepo domain.WorkspaceRepository) *TransactionalNotificationRepository {
	return &TransactionalNotificationRepository{
		workspaceRepo: workspaceRepo,
	}
}

// Create adds a new transactional notification
func (r *TransactionalNotificationRepository) Create(ctx context.Context, workspaceID string, notification *domain.TransactionalNotification) error {
	// Get workspace database connection
	db, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get workspace db: %w", err)
	}

	// Set creation timestamps
	now := time.Now().UTC()
	notification.CreatedAt = now
	notification.UpdatedAt = now

	// Insert, or resurrect a previously soft-deleted notification sharing the same id.
	// The id is the primary key, so a plain INSERT would collide with a soft-deleted
	// row (which Get/List/Delete all hide via "deleted_at IS NULL"), making a deleted
	// id impossible to recreate. ON CONFLICT overwrites only when the existing row is
	// soft-deleted; an active row is left untouched so the affected-rows guard below
	// still reports a conflict.
	query := `
		INSERT INTO transactional_notifications (
			id, name, description, channels, tracking_settings, metadata, integration_id, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9
		)
		ON CONFLICT (id) DO UPDATE SET
			name = EXCLUDED.name,
			description = EXCLUDED.description,
			channels = EXCLUDED.channels,
			tracking_settings = EXCLUDED.tracking_settings,
			metadata = EXCLUDED.metadata,
			integration_id = EXCLUDED.integration_id,
			created_at = EXCLUDED.created_at,
			updated_at = EXCLUDED.updated_at,
			deleted_at = NULL
		WHERE transactional_notifications.deleted_at IS NOT NULL
	`

	result, err := db.ExecContext(
		ctx,
		query,
		notification.ID,
		notification.Name,
		notification.Description,
		notification.Channels,
		notification.TrackingSettings,
		notification.Metadata,
		notification.IntegrationID,
		notification.CreatedAt,
		notification.UpdatedAt,
	)

	if err != nil {
		return fmt.Errorf("failed to create transactional notification: %w", err)
	}

	// Zero affected rows means an active (non-deleted) notification already owns this id.
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("transactional notification with id %s already exists", notification.ID)
	}

	return nil
}

// Update updates an existing transactional notification
func (r *TransactionalNotificationRepository) Update(ctx context.Context, workspaceID string, notification *domain.TransactionalNotification) error {
	// Get workspace database connection
	db, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get workspace db: %w", err)
	}

	// Update the timestamp
	notification.UpdatedAt = time.Now().UTC()

	query := `
		UPDATE transactional_notifications
		SET name = $1,
			description = $2,
			channels = $3,
			tracking_settings = $4,
			metadata = $5,
			integration_id = $6,
			updated_at = $7
		WHERE id = $8 AND deleted_at IS NULL
	`

	result, err := db.ExecContext(
		ctx,
		query,
		notification.Name,
		notification.Description,
		notification.Channels,
		notification.TrackingSettings,
		notification.Metadata,
		notification.IntegrationID,
		notification.UpdatedAt,
		notification.ID,
	)

	if err != nil {
		return fmt.Errorf("failed to update transactional notification: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("transactional notification not found: %s", notification.ID)
	}

	return nil
}

// Get retrieves a transactional notification by ID
func (r *TransactionalNotificationRepository) Get(ctx context.Context, workspaceID, id string) (*domain.TransactionalNotification, error) {
	// Get workspace database connection
	db, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace db: %w", err)
	}

	query := `
		SELECT id, name, description, channels, tracking_settings, metadata, integration_id, created_at, updated_at, deleted_at
		FROM transactional_notifications
		WHERE id = $1 AND deleted_at IS NULL
	`

	var notification domain.TransactionalNotification
	err = db.QueryRowContext(ctx, query, id).Scan(
		&notification.ID,
		&notification.Name,
		&notification.Description,
		&notification.Channels,
		&notification.TrackingSettings,
		&notification.Metadata,
		&notification.IntegrationID,
		&notification.CreatedAt,
		&notification.UpdatedAt,
		&notification.DeletedAt,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("transactional notification not found: %s", id)
		}
		return nil, fmt.Errorf("failed to get transactional notification: %w", err)
	}

	return &notification, nil
}

// List retrieves all transactional notifications with optional filtering
func (r *TransactionalNotificationRepository) List(ctx context.Context, workspaceID string, filter map[string]interface{}, limit, offset int) ([]*domain.TransactionalNotification, int, error) {
	// Get workspace database connection
	db, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get workspace db: %w", err)
	}

	// Build the query based on filters
	whereClause := "WHERE deleted_at IS NULL"
	args := []interface{}{}
	argIndex := 1

	// Search by name if provided
	if search, ok := filter["search"]; ok {
		whereClause += fmt.Sprintf(" AND (name ILIKE $%d OR id ILIKE $%d)", argIndex, argIndex)
		searchPattern := "%" + search.(string) + "%"
		args = append(args, searchPattern)
	}

	// Count total matching records
	countQuery := "SELECT COUNT(*) FROM transactional_notifications " + whereClause
	var total int
	err = db.QueryRowContext(ctx, countQuery, args...).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count transactional notifications: %w", err)
	}

	// Apply limit and offset
	limitClause := ""
	if limit > 0 {
		limitClause = fmt.Sprintf(" LIMIT %d OFFSET %d", limit, offset)
	}

	// Get the actual records
	query := `
		SELECT id, name, description, channels, tracking_settings, metadata, integration_id, created_at, updated_at, deleted_at
		FROM transactional_notifications
		` + whereClause + `
		ORDER BY created_at DESC
		` + limitClause

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list transactional notifications: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var notifications []*domain.TransactionalNotification
	for rows.Next() {
		var notification domain.TransactionalNotification
		err := rows.Scan(
			&notification.ID,
			&notification.Name,
			&notification.Description,
			&notification.Channels,
			&notification.TrackingSettings,
			&notification.Metadata,
			&notification.IntegrationID,
			&notification.CreatedAt,
			&notification.UpdatedAt,
			&notification.DeletedAt,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan transactional notification: %w", err)
		}
		notifications = append(notifications, &notification)
	}

	if err = rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("error iterating transactional notification rows: %w", err)
	}

	return notifications, total, nil
}

// Delete soft-deletes a transactional notification
func (r *TransactionalNotificationRepository) Delete(ctx context.Context, workspaceID, id string) error {
	// Get workspace database connection
	db, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get workspace db: %w", err)
	}

	now := time.Now().UTC()
	query := `
		UPDATE transactional_notifications
		SET deleted_at = $1, updated_at = $1
		WHERE id = $2 AND deleted_at IS NULL
	`

	result, err := db.ExecContext(ctx, query, now, id)
	if err != nil {
		return fmt.Errorf("failed to delete transactional notification: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("transactional notification not found: %s", id)
	}

	return nil
}
