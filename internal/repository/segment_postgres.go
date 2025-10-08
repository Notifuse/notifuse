package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
)

// segmentRepository implements domain.SegmentRepository for PostgreSQL
type segmentRepository struct {
	workspaceRepo domain.WorkspaceRepository
}

// NewSegmentRepository creates a new PostgreSQL segment repository
func NewSegmentRepository(workspaceRepo domain.WorkspaceRepository) domain.SegmentRepository {
	return &segmentRepository{
		workspaceRepo: workspaceRepo,
	}
}

// WithTransaction executes a function within a transaction
func (r *segmentRepository) WithTransaction(ctx context.Context, workspaceID string, fn func(*sql.Tx) error) error {
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

// CreateSegment persists a new segment
func (r *segmentRepository) CreateSegment(ctx context.Context, workspaceID string, segment *domain.Segment) error {
	// Get the workspace database connection
	workspaceDB, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get workspace connection: %w", err)
	}

	// Set timestamps
	now := time.Now().UTC()
	segment.DBCreatedAt = now
	segment.DBUpdatedAt = now

	query := `
		INSERT INTO segments (
			id, name, color, tree, timezone, version, status,
			generated_sql, generated_args, recompute_after, db_created_at, db_updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12
		)
	`

	// Convert Tree to MapOfAny for database storage
	treeMap, err := segment.Tree.ToMapOfAny()
	if err != nil {
		return fmt.Errorf("failed to convert tree to map: %w", err)
	}

	_, err = workspaceDB.ExecContext(ctx, query,
		segment.ID,
		segment.Name,
		segment.Color,
		treeMap,
		segment.Timezone,
		segment.Version,
		segment.Status,
		segment.GeneratedSQL,
		segment.GeneratedArgs,
		segment.RecomputeAfter,
		segment.DBCreatedAt,
		segment.DBUpdatedAt,
	)

	if err != nil {
		return fmt.Errorf("failed to create segment: %w", err)
	}

	return nil
}

// GetSegmentByID retrieves a segment by its ID
func (r *segmentRepository) GetSegmentByID(ctx context.Context, workspaceID string, id string) (*domain.Segment, error) {
	// Get the workspace database connection
	workspaceDB, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace connection: %w", err)
	}

	query := `
		SELECT 
			s.id, s.name, s.color, s.tree, s.timezone, s.version, s.status,
			s.generated_sql, s.generated_args, s.recompute_after, s.db_created_at, s.db_updated_at,
			COALESCE(COUNT(cs.email), 0) as users_count
		FROM segments s
		LEFT JOIN contact_segments cs ON s.id = cs.segment_id AND s.version = cs.version
		WHERE s.id = $1
		GROUP BY s.id, s.name, s.color, s.tree, s.timezone, s.version, s.status,
			s.generated_sql, s.generated_args, s.recompute_after, s.db_created_at, s.db_updated_at
	`

	row := workspaceDB.QueryRowContext(ctx, query, id)

	segment, err := domain.ScanSegment(row)
	if err == sql.ErrNoRows {
		return nil, &domain.ErrSegmentNotFound{Message: fmt.Sprintf("segment not found: %s", id)}
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get segment: %w", err)
	}

	return segment, nil
}

// GetSegments retrieves all segments for a workspace
func (r *segmentRepository) GetSegments(ctx context.Context, workspaceID string, withCount bool) ([]*domain.Segment, error) {
	// Get the workspace database connection
	workspaceDB, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace connection: %w", err)
	}

	var query string
	if withCount {
		// Include contact counts with JOIN (more expensive)
		query = `
			SELECT 
				s.id, s.name, s.color, s.tree, s.timezone, s.version, s.status,
				s.generated_sql, s.generated_args, s.recompute_after, s.db_created_at, s.db_updated_at,
				COALESCE(COUNT(cs.email), 0) as users_count
			FROM segments s
			LEFT JOIN contact_segments cs ON s.id = cs.segment_id AND s.version = cs.version
			WHERE s.status != 'deleted'
			GROUP BY s.id, s.name, s.color, s.tree, s.timezone, s.version, s.status,
				s.generated_sql, s.generated_args, s.recompute_after, s.db_created_at, s.db_updated_at
			ORDER BY s.db_created_at DESC
		`
	} else {
		// Skip contact counts for better performance
		query = `
			SELECT 
				s.id, s.name, s.color, s.tree, s.timezone, s.version, s.status,
				s.generated_sql, s.generated_args, s.recompute_after, s.db_created_at, s.db_updated_at,
				0 as users_count
			FROM segments s
			WHERE s.status != 'deleted'
			ORDER BY s.db_created_at DESC
		`
	}

	rows, err := workspaceDB.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query segments: %w", err)
	}
	defer rows.Close()

	segments := make([]*domain.Segment, 0)
	for rows.Next() {
		segment, err := domain.ScanSegment(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan segment: %w", err)
		}
		segments = append(segments, segment)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating segments: %w", err)
	}

	return segments, nil
}

// UpdateSegment updates an existing segment
func (r *segmentRepository) UpdateSegment(ctx context.Context, workspaceID string, segment *domain.Segment) error {
	// Get the workspace database connection
	workspaceDB, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get workspace connection: %w", err)
	}

	// Update timestamp
	segment.DBUpdatedAt = time.Now().UTC()

	query := `
		UPDATE segments
		SET 
			name = $2,
			color = $3,
			tree = $4,
			timezone = $5,
			version = $6,
			status = $7,
			generated_sql = $8,
			generated_args = $9,
			recompute_after = $10,
			db_updated_at = $11
		WHERE id = $1
	`

	// Convert Tree to MapOfAny for database storage
	treeMap, err := segment.Tree.ToMapOfAny()
	if err != nil {
		return fmt.Errorf("failed to convert tree to map: %w", err)
	}

	result, err := workspaceDB.ExecContext(ctx, query,
		segment.ID,
		segment.Name,
		segment.Color,
		treeMap,
		segment.Timezone,
		segment.Version,
		segment.Status,
		segment.GeneratedSQL,
		segment.GeneratedArgs,
		segment.RecomputeAfter,
		segment.DBUpdatedAt,
	)

	if err != nil {
		return fmt.Errorf("failed to update segment: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return &domain.ErrSegmentNotFound{Message: fmt.Sprintf("segment not found: %s", segment.ID)}
	}

	return nil
}

// DeleteSegment soft deletes a segment by setting status to 'deleted'
func (r *segmentRepository) DeleteSegment(ctx context.Context, workspaceID string, id string) error {
	// Get the workspace database connection
	workspaceDB, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get workspace connection: %w", err)
	}

	query := `
		UPDATE segments
		SET status = 'deleted', db_updated_at = $2
		WHERE id = $1
	`

	result, err := workspaceDB.ExecContext(ctx, query, id, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("failed to delete segment: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return &domain.ErrSegmentNotFound{Message: fmt.Sprintf("segment not found: %s", id)}
	}

	// Delete all contact_segments entries for this segment
	deleteContactSegmentsQuery := `DELETE FROM contact_segments WHERE segment_id = $1`
	_, err = workspaceDB.ExecContext(ctx, deleteContactSegmentsQuery, id)
	if err != nil {
		return fmt.Errorf("failed to delete contact_segments for segment: %w", err)
	}

	return nil
}

// AddContactToSegment adds a contact to a segment
func (r *segmentRepository) AddContactToSegment(ctx context.Context, workspaceID string, email string, segmentID string, version int64) error {
	// Get the workspace database connection
	workspaceDB, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get workspace connection: %w", err)
	}

	query := `
		INSERT INTO contact_segments (email, segment_id, version, matched_at, computed_at)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (email, segment_id)
		DO UPDATE SET version = $3, computed_at = $5
	`

	now := time.Now().UTC()
	_, err = workspaceDB.ExecContext(ctx, query, email, segmentID, version, now, now)
	if err != nil {
		return fmt.Errorf("failed to add contact to segment: %w", err)
	}

	return nil
}

// RemoveContactFromSegment removes a contact from a segment
func (r *segmentRepository) RemoveContactFromSegment(ctx context.Context, workspaceID string, email string, segmentID string) error {
	// Get the workspace database connection
	workspaceDB, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get workspace connection: %w", err)
	}

	query := `DELETE FROM contact_segments WHERE email = $1 AND segment_id = $2`

	_, err = workspaceDB.ExecContext(ctx, query, email, segmentID)
	if err != nil {
		return fmt.Errorf("failed to remove contact from segment: %w", err)
	}

	return nil
}

// RemoveOldMemberships removes contact_segment records with old versions
func (r *segmentRepository) RemoveOldMemberships(ctx context.Context, workspaceID string, segmentID string, currentVersion int64) error {
	// Get the workspace database connection
	workspaceDB, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get workspace connection: %w", err)
	}

	query := `DELETE FROM contact_segments WHERE segment_id = $1 AND version < $2`

	_, err = workspaceDB.ExecContext(ctx, query, segmentID, currentVersion)
	if err != nil {
		return fmt.Errorf("failed to remove old memberships: %w", err)
	}

	return nil
}

// GetContactSegments retrieves all segments a contact belongs to
func (r *segmentRepository) GetContactSegments(ctx context.Context, workspaceID string, email string) ([]*domain.Segment, error) {
	// Get the workspace database connection
	workspaceDB, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace connection: %w", err)
	}

	query := `
		SELECT 
			s.id, s.name, s.color, s.tree, s.timezone, s.version, s.status,
			s.generated_sql, s.generated_args, s.recompute_after, s.db_created_at, s.db_updated_at,
			0 as users_count
		FROM segments s
		INNER JOIN contact_segments cs ON s.id = cs.segment_id AND s.version = cs.version
		WHERE cs.email = $1 AND s.status = 'active'
		ORDER BY s.name ASC
	`

	rows, err := workspaceDB.QueryContext(ctx, query, email)
	if err != nil {
		return nil, fmt.Errorf("failed to query contact segments: %w", err)
	}
	defer rows.Close()

	segments := make([]*domain.Segment, 0)
	for rows.Next() {
		segment, err := domain.ScanSegment(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan segment: %w", err)
		}
		segments = append(segments, segment)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating contact segments: %w", err)
	}

	return segments, nil
}

// GetSegmentContactCount gets the count of contacts in a segment
func (r *segmentRepository) GetSegmentContactCount(ctx context.Context, workspaceID string, segmentID string) (int, error) {
	// Get the workspace database connection
	workspaceDB, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return 0, fmt.Errorf("failed to get workspace connection: %w", err)
	}

	query := `SELECT COUNT(*) FROM contact_segments WHERE segment_id = $1`

	var count int
	err = workspaceDB.QueryRowContext(ctx, query, segmentID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to get segment contact count: %w", err)
	}

	return count, nil
}

// PreviewSegment executes a segment query and returns the count of matching contacts
func (r *segmentRepository) PreviewSegment(ctx context.Context, workspaceID string, sqlQuery string, args []interface{}, limit int) (int, error) {
	// Get the workspace database connection
	workspaceDB, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return 0, fmt.Errorf("failed to get workspace connection: %w", err)
	}

	// Get the total count (emails not fetched for privacy/performance)
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM (%s) AS segment_results", sqlQuery)
	var totalCount int
	err = workspaceDB.QueryRowContext(ctx, countQuery, args...).Scan(&totalCount)
	if err != nil {
		return 0, fmt.Errorf("failed to execute preview count query: %w", err)
	}

	return totalCount, nil
}

// GetSegmentsDueForRecompute retrieves segments that need recomputation (recompute_after <= now)
func (r *segmentRepository) GetSegmentsDueForRecompute(ctx context.Context, workspaceID string, limit int) ([]*domain.Segment, error) {
	// Get the workspace database connection
	workspaceDB, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace connection: %w", err)
	}

	query := `
		SELECT 
			s.id, s.name, s.color, s.tree, s.timezone, s.version, s.status,
			s.generated_sql, s.generated_args, s.recompute_after, s.db_created_at, s.db_updated_at,
			0 as users_count
		FROM segments s
		WHERE s.status = 'active'
			AND s.recompute_after IS NOT NULL
			AND s.recompute_after <= NOW()
		ORDER BY s.recompute_after ASC
		LIMIT $1
	`

	rows, err := workspaceDB.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query segments due for recompute: %w", err)
	}
	defer rows.Close()

	segments := make([]*domain.Segment, 0)
	for rows.Next() {
		segment, err := domain.ScanSegment(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan segment: %w", err)
		}
		segments = append(segments, segment)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating segments: %w", err)
	}

	return segments, nil
}

// UpdateRecomputeAfter updates only the recompute_after field for a segment
func (r *segmentRepository) UpdateRecomputeAfter(ctx context.Context, workspaceID string, segmentID string, recomputeAfter *time.Time) error {
	// Get the workspace database connection
	workspaceDB, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get workspace connection: %w", err)
	}

	query := `
		UPDATE segments
		SET recompute_after = $2, db_updated_at = $3
		WHERE id = $1
	`

	result, err := workspaceDB.ExecContext(ctx, query, segmentID, recomputeAfter, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("failed to update recompute_after: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return &domain.ErrSegmentNotFound{Message: fmt.Sprintf("segment not found: %s", segmentID)}
	}

	return nil
}
