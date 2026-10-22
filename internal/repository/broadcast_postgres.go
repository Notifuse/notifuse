package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
)

// broadcastListColumns is the ordered column list selected when listing
// broadcasts. The order must stay in sync with scanBroadcast.
const broadcastListColumns = `id,
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
			data_feed`

// escapeLikePattern escapes the characters that carry special meaning in a SQL
// LIKE/ILIKE pattern so a user-provided search term is matched literally.
func escapeLikePattern(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

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

// UpdateBroadcastStatusTx updates only the status-lifecycle fields. Unlike
// UpdateBroadcastTx it does not enforce the "not already terminal" guard,
// so pause/resume/cancel flows can transition a Processed broadcast.
// Only updates: status, started_at, completed_at, cancelled_at, paused_at,
// pause_reason, updated_at.
func (r *broadcastRepository) UpdateBroadcastStatusTx(ctx context.Context, tx *sql.Tx, broadcast *domain.Broadcast) error {
	broadcast.UpdatedAt = time.Now().UTC()

	query := `
		UPDATE broadcasts SET
			status = $3,
			started_at = $4,
			completed_at = $5,
			cancelled_at = $6,
			paused_at = $7,
			pause_reason = $8,
			updated_at = $9
		WHERE id = $1 AND workspace_id = $2
	`

	result, err := tx.ExecContext(ctx, query,
		broadcast.ID,
		broadcast.WorkspaceID,
		broadcast.Status,
		broadcast.StartedAt,
		broadcast.CompletedAt,
		broadcast.CancelledAt,
		broadcast.PausedAt,
		broadcast.PauseReason,
		broadcast.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to update broadcast status: %w", err)
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

// UpdateBroadcastStatusUnless writes the status, pause time and pause reason,
// unless the stored status is one of unless. The condition sits in the UPDATE
// itself: a write waiting on the row lock re-checks it once the holder
// commits, so a status committed after the caller's read is kept.
func (r *broadcastRepository) UpdateBroadcastStatusUnless(ctx context.Context, broadcast *domain.Broadcast, unless []domain.BroadcastStatus) (bool, error) {
	db, err := r.workspaceRepo.GetConnection(ctx, broadcast.WorkspaceID)
	if err != nil {
		return false, fmt.Errorf("failed to get workspace connection: %w", err)
	}

	broadcast.UpdatedAt = time.Now().UTC()

	args := []interface{}{
		broadcast.ID,
		broadcast.WorkspaceID,
		string(broadcast.Status),
		broadcast.PausedAt,
		broadcast.PauseReason,
		broadcast.UpdatedAt,
	}
	query := `
		UPDATE broadcasts SET status = $3, paused_at = $4, pause_reason = $5, updated_at = $6
		WHERE id = $1 AND workspace_id = $2`
	if len(unless) > 0 {
		placeholders := make([]string, len(unless))
		for i, status := range unless {
			args = append(args, string(status))
			placeholders[i] = fmt.Sprintf("$%d", len(args))
		}
		query += " AND status NOT IN (" + strings.Join(placeholders, ", ") + ")"
	}

	result, err := db.ExecContext(ctx, query, args...)
	if err != nil {
		return false, fmt.Errorf("failed to update broadcast status: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("failed to get rows affected: %w", err)
	}
	return rowsAffected > 0, nil
}

// ListBroadcastsTx retrieves a list of broadcasts within a transaction
func (r *broadcastRepository) ListBroadcastsTx(ctx context.Context, tx *sql.Tx, params domain.ListBroadcastsParams) (*domain.BroadcastListResponse, error) {
	// Build the WHERE clause dynamically from the provided filters so status
	// and name search can be combined in any arrangement.
	conditions := []string{"workspace_id = $1"}
	args := []interface{}{params.WorkspaceID}
	argIdx := 2

	// Status filter: prefer the multi-status list, falling back to the single
	// Status field for backward compatibility.
	if len(params.Statuses) > 0 {
		placeholders := make([]string, len(params.Statuses))
		for i, status := range params.Statuses {
			placeholders[i] = fmt.Sprintf("$%d", argIdx)
			args = append(args, status)
			argIdx++
		}
		conditions = append(conditions, fmt.Sprintf("status IN (%s)", strings.Join(placeholders, ", ")))
	} else if params.Status != "" {
		conditions = append(conditions, fmt.Sprintf("status = $%d", argIdx))
		args = append(args, params.Status)
		argIdx++
	}

	// Search filter: case-insensitive substring match on the broadcast name.
	if params.Search != "" {
		conditions = append(conditions, fmt.Sprintf("name ILIKE $%d", argIdx))
		args = append(args, "%"+escapeLikePattern(params.Search)+"%")
		argIdx++
	}

	whereClause := strings.Join(conditions, " AND ")

	// First count total records that match the criteria
	countQuery := "SELECT COUNT(*) FROM broadcasts WHERE " + whereClause

	var totalCount int
	err := tx.QueryRowContext(ctx, countQuery, args...).Scan(&totalCount)
	if err != nil {
		return nil, fmt.Errorf("failed to count broadcasts: %w", err)
	}

	// Then query paginated data using the same filters plus pagination.
	dataQuery := fmt.Sprintf(
		"SELECT %s FROM broadcasts WHERE %s ORDER BY created_at DESC LIMIT $%d OFFSET $%d",
		broadcastListColumns, whereClause, argIdx, argIdx+1,
	)
	dataArgs := append(append([]interface{}{}, args...), params.Limit, params.Offset)

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
