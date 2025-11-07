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
	defer tx.Rollback()

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
			channels,
			web_publication_settings,
			web_published_at,
			winning_template, 
			test_sent_at, 
			winner_sent_at, 
			created_at, 
			updated_at, 
			started_at, 
			completed_at, 
			cancelled_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19
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
		broadcast.Channels,
		broadcast.WebPublicationSettings,
		broadcast.WebPublishedAt,
		broadcast.WinningTemplate,
		broadcast.TestSentAt,
		broadcast.WinnerSentAt,
		broadcast.CreatedAt,
		broadcast.UpdatedAt,
		broadcast.StartedAt,
		broadcast.CompletedAt,
		broadcast.CancelledAt,
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
			channels,
			web_publication_settings,
			web_published_at,
			winning_template, 
			test_sent_at, 
			winner_sent_at, 
			created_at, 
			updated_at, 
			started_at, 
			completed_at, 
			cancelled_at
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
			channels,
			web_publication_settings,
			web_published_at,
			winning_template, 
			test_sent_at, 
			winner_sent_at, 
			created_at, 
			updated_at, 
			started_at, 
			completed_at, 
			cancelled_at
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
			channels = $10,
			web_publication_settings = $11,
			web_published_at = $12,
			winning_template = $13,
			test_sent_at = $14,
			winner_sent_at = $15,
			updated_at = $16,
			started_at = $17,
			completed_at = $18,
			cancelled_at = $19
		WHERE id = $1 AND workspace_id = $2
			AND status != 'cancelled'
			AND status != 'sent'
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
		broadcast.Channels,
		broadcast.WebPublicationSettings,
		broadcast.WebPublishedAt,
		broadcast.WinningTemplate,
		broadcast.TestSentAt,
		broadcast.WinnerSentAt,
		broadcast.UpdatedAt,
		broadcast.StartedAt,
		broadcast.CompletedAt,
		broadcast.CancelledAt,
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
				channels,
				web_settings,
				winning_template, 
				test_sent_at, 
				winner_sent_at, 
				created_at, 
				updated_at, 
				started_at, 
				completed_at, 
				cancelled_at
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
				channels,
				web_settings,
				winning_template, 
				test_sent_at, 
				winner_sent_at, 
				created_at, 
				updated_at, 
				started_at, 
				completed_at, 
				cancelled_at
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
	defer rows.Close()

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
	defer tx.Rollback()

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
		&broadcast.Channels,
		&broadcast.WebPublicationSettings,
		&broadcast.WebPublishedAt,
		&broadcast.WinningTemplate,
		&broadcast.TestSentAt,
		&broadcast.WinnerSentAt,
		&broadcast.CreatedAt,
		&broadcast.UpdatedAt,
		&broadcast.StartedAt,
		&broadcast.CompletedAt,
		&broadcast.CancelledAt,
	)

	if err != nil {
		return nil, err
	}

	return broadcast, nil
}

// GetBySlug retrieves a broadcast by its slug within a workspace
func (r *broadcastRepository) GetBySlug(ctx context.Context, workspaceID, slug string) (*domain.Broadcast, error) {
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
			channels,
			web_publication_settings,
			web_published_at,
			winning_template, 
			test_sent_at, 
			winner_sent_at, 
			created_at, 
			updated_at, 
			started_at, 
			completed_at, 
			cancelled_at
		FROM broadcasts
		WHERE workspace_id = $1 AND web_settings->>'slug' = $2
	`

	row := workspaceDB.QueryRowContext(ctx, query, workspaceID, slug)

	broadcast, err := scanBroadcast(row)
	if err == sql.ErrNoRows {
		return nil, &domain.ErrBroadcastNotFound{ID: slug}
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get broadcast by slug: %w", err)
	}

	return broadcast, nil
}

// GetPublishedWebBroadcasts retrieves published web broadcasts for a workspace
func (r *broadcastRepository) GetPublishedWebBroadcasts(ctx context.Context, workspaceID string, limit, offset int) ([]*domain.Broadcast, int, error) {
	// Get the workspace database connection
	workspaceDB, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get workspace connection: %w", err)
	}

	// Count total published web broadcasts
	countQuery := `
		SELECT COUNT(*)
		FROM broadcasts
		WHERE workspace_id = $1
		  AND channels->>'web' = 'true'
		  AND web_published_at IS NOT NULL
		  AND web_published_at <= NOW()
	`

	var totalCount int
	err = workspaceDB.QueryRowContext(ctx, countQuery, workspaceID).Scan(&totalCount)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count published web broadcasts: %w", err)
	}

	// Query paginated broadcasts
	dataQuery := `
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
			channels,
			web_publication_settings,
			web_published_at,
			winning_template, 
			test_sent_at, 
			winner_sent_at, 
			created_at, 
			updated_at, 
			started_at, 
			completed_at, 
			cancelled_at
		FROM broadcasts
		WHERE workspace_id = $1
		  AND channels->>'web' = 'true'
		  AND web_published_at IS NOT NULL
		  AND web_published_at <= NOW()
		ORDER BY web_published_at DESC
		LIMIT $2 OFFSET $3
	`

	rows, err := workspaceDB.QueryContext(ctx, dataQuery, workspaceID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query published web broadcasts: %w", err)
	}
	defer rows.Close()

	var broadcasts []*domain.Broadcast
	for rows.Next() {
		broadcast, err := scanBroadcast(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan broadcast: %w", err)
		}
		broadcasts = append(broadcasts, broadcast)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("error iterating broadcast rows: %w", err)
	}

	return broadcasts, totalCount, nil
}

// HasWebPublications checks if a workspace has any published web broadcasts
func (r *broadcastRepository) HasWebPublications(ctx context.Context, workspaceID string) (bool, error) {
	// Get the workspace database connection
	workspaceDB, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return false, fmt.Errorf("failed to get workspace connection: %w", err)
	}

	query := `
		SELECT EXISTS(
			SELECT 1
			FROM broadcasts
			WHERE workspace_id = $1
			  AND channels->>'web' = 'true'
			  AND web_settings->>'published_at' IS NOT NULL
			  AND (web_settings->>'published_at')::timestamp <= NOW()
		)
	`

	var hasPublications bool
	err = workspaceDB.QueryRowContext(ctx, query, workspaceID).Scan(&hasPublications)
	if err != nil {
		return false, fmt.Errorf("failed to check web publications: %w", err)
	}

	return hasPublications, nil
}

// GetPublishedCountsByList returns post counts for each list in a single query
func (r *broadcastRepository) GetPublishedCountsByList(ctx context.Context, workspaceID string) (map[string]int, error) {
	workspaceDB, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace connection: %w", err)
	}

	query := `
		SELECT 
			audience->'lists'->>0 as list_id,
			COUNT(*) as count
		FROM broadcasts
		WHERE workspace_id = $1
		  AND channels->>'web' = 'true'
		  AND web_published_at IS NOT NULL
		  AND web_published_at <= NOW()
		GROUP BY audience->'lists'->>0
	`

	rows, err := workspaceDB.QueryContext(ctx, query, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get published counts by list: %w", err)
	}
	defer rows.Close()

	counts := make(map[string]int)
	for rows.Next() {
		var listID string
		var count int
		if err := rows.Scan(&listID, &count); err != nil {
			return nil, fmt.Errorf("failed to scan count: %w", err)
		}
		counts[listID] = count
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating count rows: %w", err)
	}

	return counts, nil
}

// GetByListAndSlug retrieves a broadcast by list ID and slug
func (r *broadcastRepository) GetByListAndSlug(ctx context.Context, workspaceID, listID, slug string) (*domain.Broadcast, error) {
	workspaceDB, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace connection: %w", err)
	}

	query := `
		SELECT 
			id, workspace_id, name, status, audience, schedule, test_settings,
			utm_parameters, metadata, channels, web_publication_settings, web_published_at,
			winning_template, test_sent_at, winner_sent_at,
			created_at, updated_at, started_at, completed_at, cancelled_at
		FROM broadcasts
		WHERE workspace_id = $1
		  AND audience->'lists' @> $2::jsonb
		  AND web_publication_settings->>'slug' = $3
		  AND channels->>'web' = 'true'
		LIMIT 1
	`

	listJSON := fmt.Sprintf(`["%s"]`, listID)
	row := workspaceDB.QueryRowContext(ctx, query, workspaceID, listJSON, slug)

	broadcast, err := scanBroadcast(row)
	if err == sql.ErrNoRows {
		return nil, &domain.ErrBroadcastNotFound{ID: slug}
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get broadcast by list and slug: %w", err)
	}

	return broadcast, nil
}

// GetPublishedWebBroadcastsByList returns published web broadcasts for a specific list
func (r *broadcastRepository) GetPublishedWebBroadcastsByList(ctx context.Context, workspaceID, listID string, limit, offset int) ([]*domain.Broadcast, int, error) {
	workspaceDB, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get workspace connection: %w", err)
	}

	// Count total
	countQuery := `
		SELECT COUNT(*)
		FROM broadcasts
		WHERE workspace_id = $1
		  AND audience->'lists' @> $2::jsonb
		  AND channels->>'web' = 'true'
		  AND web_published_at IS NOT NULL
		  AND web_published_at <= NOW()
	`

	listJSON := fmt.Sprintf(`["%s"]`, listID)
	var totalCount int
	err = workspaceDB.QueryRowContext(ctx, countQuery, workspaceID, listJSON).Scan(&totalCount)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count published web broadcasts: %w", err)
	}

	// Get data
	dataQuery := `
		SELECT 
			id, workspace_id, name, status, audience, schedule, test_settings,
			utm_parameters, metadata, channels, web_publication_settings, web_published_at,
			winning_template, test_sent_at, winner_sent_at,
			created_at, updated_at, started_at, completed_at, cancelled_at
		FROM broadcasts
		WHERE workspace_id = $1
		  AND audience->'lists' @> $2::jsonb
		  AND channels->>'web' = 'true'
		  AND web_published_at IS NOT NULL
		  AND web_published_at <= NOW()
		ORDER BY web_published_at DESC
		LIMIT $3 OFFSET $4
	`

	rows, err := workspaceDB.QueryContext(ctx, dataQuery, workspaceID, listJSON, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get published web broadcasts: %w", err)
	}
	defer rows.Close()

	broadcasts := []*domain.Broadcast{}
	for rows.Next() {
		broadcast, err := scanBroadcast(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan broadcast: %w", err)
		}
		broadcasts = append(broadcasts, broadcast)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("error iterating broadcast rows: %w", err)
	}

	return broadcasts, totalCount, nil
}
