package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
)

// broadcastRepository implements domain.BroadcastRepository for PostgreSQL
type broadcastRepository struct {
	workspaceRepo domain.WorkspaceRepository
}

// NewBroadcastRepository creates a new PostgreSQL broadcast repository
func NewBroadcastRepository(workspaceRepo domain.WorkspaceRepository) domain.BroadcastRepository {
	return &broadcastRepository{
		workspaceRepo: workspaceRepo,
	}
}

// WithTransaction executes a function within a transaction
func (r *broadcastRepository) WithTransaction(ctx context.Context, workspaceID string, fn func(*sql.Tx) error) error {
	// Get the workspace database connection
	workspaceDB, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get workspace connection: %w", err)
	}

	// Begin a transaction
	tx, err := workspaceDB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	// Defer rollback - this will be a no-op if we successfully commit
	defer func() { _ = tx.Rollback() }()

	// Execute the provided function with the transaction
	if err := fn(tx); err != nil {
		return err
	}

	// Commit the transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// CreateBroadcast persists a new broadcast
func (r *broadcastRepository) CreateBroadcast(ctx context.Context, broadcast *domain.Broadcast) error {
	return r.WithTransaction(ctx, broadcast.WorkspaceID, func(tx *sql.Tx) error {
		return r.CreateBroadcastTx(ctx, tx, broadcast)
	})
}

// CreateBroadcastTx persists a new broadcast within a transaction
func (r *broadcastRepository) CreateBroadcastTx(ctx context.Context, tx *sql.Tx, broadcast *domain.Broadcast) error {
	// Set created and updated timestamps
	now := time.Now().UTC()
	broadcast.CreatedAt = now
	broadcast.UpdatedAt = now

	// Insert the broadcast
	query := `
		INSERT INTO broadcasts (
			id,
			workspace_id,
			name,
			status,
			audience,
			schedule,
			test_settings,
			utm_parameters,
			metadata,
			winning_template,
			test_sent_at,
			winner_sent_at,
			enqueued_count,
			created_at,
			updated_at,
			started_at,
			completed_at,
			cancelled_at,
			paused_at,
			pause_reason,
			data_feed
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21
		)
	`

	_, err := tx.ExecContext(ctx, query,
		broadcast.ID,
		broadcast.WorkspaceID,
		broadcast.Name,
		broadcast.Status,
		broadcast.Audience,
		broadcast.Schedule,
		broadcast.TestSettings,
		broadcast.UTMParameters,
		broadcast.Metadata,
		broadcast.WinningTemplate,
		broadcast.TestSentAt,
		broadcast.WinnerSentAt,
		broadcast.EnqueuedCount,
		broadcast.CreatedAt,
		broadcast.UpdatedAt,
		broadcast.StartedAt,
		broadcast.CompletedAt,
		broadcast.CancelledAt,
		broadcast.PausedAt,
		broadcast.PauseReason,
		broadcast.DataFeed,
	)

	if err != nil {
		return fmt.Errorf("failed to create broadcast: %w", err)
	}

	return nil
}

// GetBroadcast retrieves a broadcast by ID
func (r *broadcastRepository) GetBroadcast(ctx context.Context, workspaceID, id string) (*domain.Broadcast, error) {
	// Get the workspace database connection
	workspaceDB, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace connection: %w", err)
	}

	query := `
		SELECT
			id,
			workspace_id,
			name,
			status,
			audience,
			schedule,
			test_settings,
			utm_parameters,
			metadata,
			winning_template,
			test_sent_at,
			winner_sent_at,
			enqueued_count,
			created_at,
			updated_at,
			started_at,
			completed_at,
			cancelled_at,
			paused_at,
			pause_reason,
			data_feed
		FROM broadcasts
		WHERE id = $1 AND workspace_id = $2
	`

	row := workspaceDB.QueryRowContext(ctx, query, id, workspaceID)

	broadcast, err := scanBroadcast(row)
	if err == sql.ErrNoRows {
		return nil, &domain.ErrBroadcastNotFound{ID: id}
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get broadcast: %w", err)
	}

	return broadcast, nil
}

// GetBroadcastTx retrieves a broadcast by ID within a transaction
func (r *broadcastRepository) GetBroadcastTx(ctx context.Context, tx *sql.Tx, workspaceID, id string) (*domain.Broadcast, error) {
	query := `
		SELECT
			id,
			workspace_id,
			name,
			status,
			audience,
			schedule,
			test_settings,
			utm_parameters,
			metadata,
			winning_template,
			test_sent_at,
			winner_sent_at,
			enqueued_count,
			created_at,
			updated_at,
			started_at,
			completed_at,
			cancelled_at,
			paused_at,
			pause_reason,
			data_feed
		FROM broadcasts
		WHERE id = $1 AND workspace_id = $2
	`

	row := tx.QueryRowContext(ctx, query, id, workspaceID)

	broadcast, err := scanBroadcast(row)
	if err == sql.ErrNoRows {
		return nil, &domain.ErrBroadcastNotFound{ID: id}
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get broadcast: %w", err)
	}

	return broadcast, nil
}

// UpdateBroadcast updates an existing broadcast
func (r *broadcastRepository) UpdateBroadcast(ctx context.Context, broadcast *domain.Broadcast) error {
	return r.WithTransaction(ctx, broadcast.WorkspaceID, func(tx *sql.Tx) error {
		return r.UpdateBroadcastTx(ctx, tx, broadcast)
	})
}

// UpdateBroadcastTx updates an existing broadcast within a transaction
func (r *broadcastRepository) UpdateBroadcastTx(ctx context.Context, tx *sql.Tx, broadcast *domain.Broadcast) error {
	// Update the timestamp
	broadcast.UpdatedAt = time.Now().UTC()

	query := `
		UPDATE broadcasts SET
			name = $3,
			status = $4,
			audience = $5,
			schedule = $6,
			test_settings = $7,
			utm_parameters = $8,
			metadata = $9,
			winning_template = $10,
			test_sent_at = $11,
			winner_sent_at = $12,
			updated_at = $13,
			started_at = $14,
			completed_at = $15,
			cancelled_at = $16,
			paused_at = $17,
			pause_reason = $18,
			enqueued_count = $19,
			data_feed = $20
		WHERE id = $1 AND workspace_id = $2
			AND status != 'cancelled'
			AND status != 'processed'
	`

	result, err := tx.ExecContext(ctx, query,
		broadcast.ID,
		broadcast.WorkspaceID,
		broadcast.Name,
		broadcast.Status,
		broadcast.Audience,
		broadcast.Schedule,
		broadcast.TestSettings,
		broadcast.UTMParameters,
		broadcast.Metadata,
		broadcast.WinningTemplate,
		broadcast.TestSentAt,
		broadcast.WinnerSentAt,
		broadcast.UpdatedAt,
		broadcast.StartedAt,
		broadcast.CompletedAt,
		broadcast.CancelledAt,
		broadcast.PausedAt,
		broadcast.PauseReason,
		broadcast.EnqueuedCount,
		broadcast.DataFeed,
	)

	if err != nil {
		return fmt.Errorf("failed to update broadcast: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return &domain.ErrBroadcastNotFound{ID: broadcast.ID}
	}

	return nil
}

// ListBroadcastsTx retrieves a list of broadcasts within a transaction
func (r *broadcastRepository) ListBroadcastsTx(ctx context.Context, tx *sql.Tx, params domain.ListBroadcastsParams) (*domain.BroadcastListResponse, error) {
	// First count total records that match the criteria
	var countQuery string
	var countArgs []interface{}

	if params.Status != "" {
		countQuery = `
			SELECT COUNT(*)
			FROM broadcasts
			WHERE workspace_id = $1 AND status = $2
		`
		countArgs = []interface{}{params.WorkspaceID, params.Status}
	} else {
		countQuery = `
			SELECT COUNT(*)
			FROM broadcasts
			WHERE workspace_id = $1
		`
		countArgs = []interface{}{params.WorkspaceID}
	}

	var totalCount int
	err := tx.QueryRowContext(ctx, countQuery, countArgs...).Scan(&totalCount)
	if err != nil {
		return nil, fmt.Errorf("failed to count broadcasts: %w", err)
	}

	// Then query paginated data
	var dataQuery string
	var dataArgs []interface{}

	if params.Status != "" {
		dataQuery = `
			SELECT
				id,
				workspace_id,
				name,
				status,
				audience,
				schedule,
				test_settings,
				utm_parameters,
				metadata,
				winning_template,
				test_sent_at,
				winner_sent_at,
				enqueued_count,
				created_at,
				updated_at,
				started_at,
				completed_at,
				cancelled_at,
				paused_at,
				pause_reason,
				data_feed
			FROM broadcasts
			WHERE workspace_id = $1 AND status = $2
			ORDER BY created_at DESC
			LIMIT $3 OFFSET $4
		`
		dataArgs = []interface{}{params.WorkspaceID, params.Status, params.Limit, params.Offset}
	} else {
		dataQuery = `
			SELECT
				id,
				workspace_id,
				name,
				status,
				audience,
				schedule,
				test_settings,
				utm_parameters,
				metadata,
				winning_template,
				test_sent_at,
				winner_sent_at,
				enqueued_count,
				created_at,
				updated_at,
				started_at,
				completed_at,
				cancelled_at,
				paused_at,
				pause_reason,
				data_feed
			FROM broadcasts
			WHERE workspace_id = $1
			ORDER BY created_at DESC
			LIMIT $2 OFFSET $3
		`
		dataArgs = []interface{}{params.WorkspaceID, params.Limit, params.Offset}
	}

	rows, err := tx.QueryContext(ctx, dataQuery, dataArgs...)
	if err != nil {
		return nil, fmt.Errorf("failed to list broadcasts: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var broadcasts []*domain.Broadcast
	for rows.Next() {
		broadcast, err := scanBroadcast(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan broadcast: %w", err)
		}
		broadcasts = append(broadcasts, broadcast)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating broadcast rows: %w", err)
	}

	return &domain.BroadcastListResponse{
		Broadcasts: broadcasts,
		TotalCount: totalCount,
	}, nil
}

// ListBroadcasts retrieves a list of broadcasts
func (r *broadcastRepository) ListBroadcasts(ctx context.Context, params domain.ListBroadcastsParams) (*domain.BroadcastListResponse, error) {
	// Get the workspace database connection
	workspaceDB, err := r.workspaceRepo.GetConnection(ctx, params.WorkspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace connection: %w", err)
	}

	// Begin a transaction
	tx, err := workspaceDB.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Use the transaction-aware method
	result, err := r.ListBroadcastsTx(ctx, tx, params)
	if err != nil {
		return nil, err
	}

	// Commit the transaction
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return result, nil
}

// DeleteBroadcast deletes a broadcast from the database
func (r *broadcastRepository) DeleteBroadcast(ctx context.Context, workspaceID, id string) error {
	return r.WithTransaction(ctx, workspaceID, func(tx *sql.Tx) error {
		return r.DeleteBroadcastTx(ctx, tx, workspaceID, id)
	})
}

// DeleteBroadcastTx deletes a broadcast from the database within a transaction
func (r *broadcastRepository) DeleteBroadcastTx(ctx context.Context, tx *sql.Tx, workspaceID, id string) error {
	query := `
		DELETE FROM broadcasts
		WHERE id = $1 AND workspace_id = $2
	`

	result, err := tx.ExecContext(ctx, query, id, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to delete broadcast: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return &domain.ErrBroadcastNotFound{ID: id}
	}

	return nil
}

// scanBroadcast scans a row into a Broadcast struct
func scanBroadcast(scanner interface {
	Scan(dest ...interface{}) error
}) (*domain.Broadcast, error) {
	broadcast := &domain.Broadcast{}
	var winningTemplate sql.NullString
	var pauseReason sql.NullString
	var dataFeed domain.DataFeedSettings

	err := scanner.Scan(
		&broadcast.ID,
		&broadcast.WorkspaceID,
		&broadcast.Name,
		&broadcast.Status,
		&broadcast.Audience,
		&broadcast.Schedule,
		&broadcast.TestSettings,
		&broadcast.UTMParameters,
		&broadcast.Metadata,
		&winningTemplate,
		&broadcast.TestSentAt,
		&broadcast.WinnerSentAt,
		&broadcast.EnqueuedCount,
		&broadcast.CreatedAt,
		&broadcast.UpdatedAt,
		&broadcast.StartedAt,
		&broadcast.CompletedAt,
		&broadcast.CancelledAt,
		&broadcast.PausedAt,
		&pauseReason,
		&dataFeed,
	)

	if err != nil {
		return nil, err
	}

	// Convert sql.NullString to *string
	if winningTemplate.Valid {
		broadcast.WinningTemplate = &winningTemplate.String
	}
	if pauseReason.Valid {
		broadcast.PauseReason = &pauseReason.String
	}

	// Set DataFeed pointer if it has any data
	if dataFeed.GlobalFeed != nil || dataFeed.RecipientFeed != nil || len(dataFeed.GlobalFeedData) > 0 || dataFeed.GlobalFeedFetchedAt != nil {
		broadcast.DataFeed = &dataFeed
	}

	return broadcast, nil
}
