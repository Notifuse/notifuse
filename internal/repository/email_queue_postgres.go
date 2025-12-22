package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/google/uuid"
)

// EmailQueueRepository implements domain.EmailQueueRepository
type EmailQueueRepository struct {
	workspaceRepo domain.WorkspaceRepository
	db            *sql.DB // Used for testing with sqlmock
}

// NewEmailQueueRepository creates a new EmailQueueRepository using workspace repository
func NewEmailQueueRepository(workspaceRepo domain.WorkspaceRepository) domain.EmailQueueRepository {
	return &EmailQueueRepository{
		workspaceRepo: workspaceRepo,
	}
}

// NewEmailQueueRepositoryWithDB creates a new EmailQueueRepository with a direct DB connection (for testing)
func NewEmailQueueRepositoryWithDB(db *sql.DB) domain.EmailQueueRepository {
	return &EmailQueueRepository{
		db: db,
	}
}

// getDB returns the database connection for a workspace
func (r *EmailQueueRepository) getDB(ctx context.Context, workspaceID string) (*sql.DB, error) {
	if r.db != nil {
		return r.db, nil
	}
	return r.workspaceRepo.GetConnection(ctx, workspaceID)
}

// psql is a Squirrel StatementBuilder configured for PostgreSQL
var emailQueuePsql = sq.StatementBuilder.PlaceholderFormat(sq.Dollar)

// Enqueue adds emails to the queue
func (r *EmailQueueRepository) Enqueue(ctx context.Context, workspaceID string, entries []*domain.EmailQueueEntry) error {
	if len(entries) == 0 {
		return nil
	}

	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get database connection: %w", err)
	}

	// Use a transaction for batch insert
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	if err := r.EnqueueTx(ctx, tx, entries); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// EnqueueTx adds emails to the queue within an existing transaction
func (r *EmailQueueRepository) EnqueueTx(ctx context.Context, tx *sql.Tx, entries []*domain.EmailQueueEntry) error {
	if len(entries) == 0 {
		return nil
	}

	now := time.Now().UTC()

	insertBuilder := emailQueuePsql.
		Insert("email_queue").
		Columns(
			"id", "status", "priority", "source_type", "source_id",
			"integration_id", "provider_kind", "contact_email", "message_id",
			"template_id", "payload", "attempts", "max_attempts",
			"created_at", "updated_at",
		)

	for _, entry := range entries {
		// Generate ID if not set
		if entry.ID == "" {
			entry.ID = uuid.New().String()
		}

		// Set defaults
		if entry.Status == "" {
			entry.Status = domain.EmailQueueStatusPending
		}
		if entry.Priority == 0 {
			entry.Priority = domain.EmailQueuePriorityMarketing
		}
		if entry.MaxAttempts == 0 {
			entry.MaxAttempts = 3
		}

		entry.CreatedAt = now
		entry.UpdatedAt = now

		payloadJSON, err := json.Marshal(entry.Payload)
		if err != nil {
			return fmt.Errorf("failed to marshal payload: %w", err)
		}

		insertBuilder = insertBuilder.Values(
			entry.ID, entry.Status, entry.Priority, entry.SourceType, entry.SourceID,
			entry.IntegrationID, entry.ProviderKind, entry.ContactEmail, entry.MessageID,
			entry.TemplateID, payloadJSON, entry.Attempts, entry.MaxAttempts,
			entry.CreatedAt, entry.UpdatedAt,
		)
	}

	query, args, err := insertBuilder.ToSql()
	if err != nil {
		return fmt.Errorf("failed to build query: %w", err)
	}

	_, err = tx.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("failed to insert queue entries: %w", err)
	}

	return nil
}

// FetchPending retrieves pending emails for processing
// Uses FOR UPDATE SKIP LOCKED for safe concurrent worker access
func (r *EmailQueueRepository) FetchPending(ctx context.Context, workspaceID string, limit int) ([]*domain.EmailQueueEntry, error) {
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get database connection: %w", err)
	}

	// Fetch pending emails ordered by priority (lower = higher priority), then by creation time
	// Include failed emails that are ready for retry
	query := `
		SELECT id, status, priority, source_type, source_id, integration_id, provider_kind,
		       contact_email, message_id, template_id, payload, attempts, max_attempts,
		       last_error, next_retry_at, created_at, updated_at, processed_at
		FROM email_queue
		WHERE (status = 'pending' AND (next_retry_at IS NULL OR next_retry_at <= NOW()))
		   OR (status = 'failed' AND attempts < max_attempts AND next_retry_at <= NOW())
		ORDER BY priority ASC, created_at ASC
		LIMIT $1
		FOR UPDATE SKIP LOCKED
	`

	rows, err := db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query pending emails: %w", err)
	}
	defer rows.Close()

	var entries []*domain.EmailQueueEntry
	for rows.Next() {
		entry, err := scanEmailQueueEntry(rows)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return entries, nil
}

// MarkAsProcessing atomically marks an entry as processing
func (r *EmailQueueRepository) MarkAsProcessing(ctx context.Context, workspaceID string, id string) error {
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get database connection: %w", err)
	}

	query := `
		UPDATE email_queue
		SET status = 'processing', updated_at = NOW(), attempts = attempts + 1
		WHERE id = $1 AND status IN ('pending', 'failed')
	`

	result, err := db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to mark email as processing: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("email not found or already processing: %s", id)
	}

	return nil
}

// MarkAsSent deletes the entry after successful send
// (entries are removed immediately rather than kept with a "sent" status)
func (r *EmailQueueRepository) MarkAsSent(ctx context.Context, workspaceID string, id string) error {
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get database connection: %w", err)
	}

	query := `DELETE FROM email_queue WHERE id = $1`

	_, err = db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete sent email: %w", err)
	}

	return nil
}

// MarkAsFailed marks an entry as failed and schedules retry
func (r *EmailQueueRepository) MarkAsFailed(ctx context.Context, workspaceID string, id string, errorMsg string, nextRetryAt *time.Time) error {
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get database connection: %w", err)
	}

	now := time.Now().UTC()
	query := `
		UPDATE email_queue
		SET status = 'failed', updated_at = $2, last_error = $3, next_retry_at = $4
		WHERE id = $1
	`

	_, err = db.ExecContext(ctx, query, id, now, errorMsg, nextRetryAt)
	if err != nil {
		return fmt.Errorf("failed to mark email as failed: %w", err)
	}

	return nil
}

// MoveToDeadLetter moves a permanently failed entry to the dead letter queue
func (r *EmailQueueRepository) MoveToDeadLetter(ctx context.Context, workspaceID string, entry *domain.EmailQueueEntry, finalError string) error {
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get database connection: %w", err)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Insert into dead letter queue
	payloadJSON, err := json.Marshal(entry.Payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	deadLetterID := uuid.New().String()
	now := time.Now().UTC()

	insertQuery := `
		INSERT INTO email_queue_dead_letter (
			id, original_entry_id, source_type, source_id, contact_email,
			message_id, payload, final_error, attempts, created_at, failed_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`

	_, err = tx.ExecContext(ctx, insertQuery,
		deadLetterID, entry.ID, entry.SourceType, entry.SourceID, entry.ContactEmail,
		entry.MessageID, payloadJSON, finalError, entry.Attempts, entry.CreatedAt, now,
	)
	if err != nil {
		return fmt.Errorf("failed to insert into dead letter queue: %w", err)
	}

	// Delete from main queue
	deleteQuery := `DELETE FROM email_queue WHERE id = $1`
	_, err = tx.ExecContext(ctx, deleteQuery, entry.ID)
	if err != nil {
		return fmt.Errorf("failed to delete from email queue: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// GetStats returns queue statistics for a workspace
func (r *EmailQueueRepository) GetStats(ctx context.Context, workspaceID string) (*domain.EmailQueueStats, error) {
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get database connection: %w", err)
	}

	// Note: sent entries are deleted immediately, so we don't track them in stats
	query := `
		SELECT
			COALESCE(SUM(CASE WHEN status = 'pending' THEN 1 ELSE 0 END), 0) as pending,
			COALESCE(SUM(CASE WHEN status = 'processing' THEN 1 ELSE 0 END), 0) as processing,
			COALESCE(SUM(CASE WHEN status = 'failed' THEN 1 ELSE 0 END), 0) as failed
		FROM email_queue
	`

	var stats domain.EmailQueueStats
	err = db.QueryRowContext(ctx, query).Scan(
		&stats.Pending, &stats.Processing, &stats.Failed,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get queue stats: %w", err)
	}

	// Get dead letter count separately
	deadLetterQuery := `SELECT COUNT(*) FROM email_queue_dead_letter`
	err = db.QueryRowContext(ctx, deadLetterQuery).Scan(&stats.DeadLetter)
	if err != nil {
		return nil, fmt.Errorf("failed to get dead letter count: %w", err)
	}

	return &stats, nil
}

// GetBySourceID retrieves queue entries by source type and ID
func (r *EmailQueueRepository) GetBySourceID(ctx context.Context, workspaceID string, sourceType domain.EmailQueueSourceType, sourceID string) ([]*domain.EmailQueueEntry, error) {
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get database connection: %w", err)
	}

	query := `
		SELECT id, status, priority, source_type, source_id, integration_id, provider_kind,
		       contact_email, message_id, template_id, payload, attempts, max_attempts,
		       last_error, next_retry_at, created_at, updated_at, processed_at
		FROM email_queue
		WHERE source_type = $1 AND source_id = $2
		ORDER BY created_at ASC
	`

	rows, err := db.QueryContext(ctx, query, sourceType, sourceID)
	if err != nil {
		return nil, fmt.Errorf("failed to query by source: %w", err)
	}
	defer rows.Close()

	var entries []*domain.EmailQueueEntry
	for rows.Next() {
		entry, err := scanEmailQueueEntry(rows)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}

	return entries, rows.Err()
}

// CountBySourceAndStatus counts entries by source and status
func (r *EmailQueueRepository) CountBySourceAndStatus(ctx context.Context, workspaceID string, sourceType domain.EmailQueueSourceType, sourceID string, status domain.EmailQueueStatus) (int64, error) {
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return 0, fmt.Errorf("failed to get database connection: %w", err)
	}

	query := `
		SELECT COUNT(*)
		FROM email_queue
		WHERE source_type = $1 AND source_id = $2 AND status = $3
	`

	var count int64
	err = db.QueryRowContext(ctx, query, sourceType, sourceID, status).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count by source and status: %w", err)
	}

	return count, nil
}

// CleanupDeadLetter removes old dead letter entries
func (r *EmailQueueRepository) CleanupDeadLetter(ctx context.Context, workspaceID string, olderThan time.Duration) (int64, error) {
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return 0, fmt.Errorf("failed to get database connection: %w", err)
	}

	cutoff := time.Now().UTC().Add(-olderThan)
	query := `DELETE FROM email_queue_dead_letter WHERE failed_at < $1`

	result, err := db.ExecContext(ctx, query, cutoff)
	if err != nil {
		return 0, fmt.Errorf("failed to cleanup dead letter entries: %w", err)
	}

	return result.RowsAffected()
}

// GetDeadLetterEntries retrieves dead letter entries for investigation
func (r *EmailQueueRepository) GetDeadLetterEntries(ctx context.Context, workspaceID string, limit, offset int) ([]*domain.EmailQueueDeadLetter, int64, error) {
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get database connection: %w", err)
	}

	// Get total count
	var total int64
	countQuery := `SELECT COUNT(*) FROM email_queue_dead_letter`
	err = db.QueryRowContext(ctx, countQuery).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count dead letter entries: %w", err)
	}

	// Get entries
	query := `
		SELECT id, original_entry_id, source_type, source_id, contact_email,
		       message_id, payload, final_error, attempts, created_at, failed_at
		FROM email_queue_dead_letter
		ORDER BY failed_at DESC
		LIMIT $1 OFFSET $2
	`

	rows, err := db.QueryContext(ctx, query, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query dead letter entries: %w", err)
	}
	defer rows.Close()

	var entries []*domain.EmailQueueDeadLetter
	for rows.Next() {
		var entry domain.EmailQueueDeadLetter
		var payloadJSON []byte

		err := rows.Scan(
			&entry.ID, &entry.OriginalEntryID, &entry.SourceType, &entry.SourceID,
			&entry.ContactEmail, &entry.MessageID, &payloadJSON, &entry.FinalError,
			&entry.Attempts, &entry.CreatedAt, &entry.FailedAt,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan dead letter entry: %w", err)
		}

		if err := json.Unmarshal(payloadJSON, &entry.Payload); err != nil {
			return nil, 0, fmt.Errorf("failed to unmarshal payload: %w", err)
		}

		entries = append(entries, &entry)
	}

	return entries, total, rows.Err()
}

// RetryDeadLetter moves a dead letter entry back to the queue for retry
func (r *EmailQueueRepository) RetryDeadLetter(ctx context.Context, workspaceID string, deadLetterID string) error {
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get database connection: %w", err)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Get dead letter entry
	var entry domain.EmailQueueDeadLetter
	var payloadJSON []byte

	query := `
		SELECT id, original_entry_id, source_type, source_id, contact_email,
		       message_id, payload, final_error, attempts, created_at, failed_at
		FROM email_queue_dead_letter
		WHERE id = $1
	`

	err = tx.QueryRowContext(ctx, query, deadLetterID).Scan(
		&entry.ID, &entry.OriginalEntryID, &entry.SourceType, &entry.SourceID,
		&entry.ContactEmail, &entry.MessageID, &payloadJSON, &entry.FinalError,
		&entry.Attempts, &entry.CreatedAt, &entry.FailedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("dead letter entry not found: %s", deadLetterID)
		}
		return fmt.Errorf("failed to get dead letter entry: %w", err)
	}

	if err := json.Unmarshal(payloadJSON, &entry.Payload); err != nil {
		return fmt.Errorf("failed to unmarshal payload: %w", err)
	}

	// Create new queue entry
	now := time.Now().UTC()
	newID := uuid.New().String()

	// Extract integration_id, provider_kind, template_id from original context
	// These should be stored in the payload or we need to add them to dead letter
	insertQuery := `
		INSERT INTO email_queue (
			id, status, priority, source_type, source_id, integration_id, provider_kind,
			contact_email, message_id, template_id, payload, attempts, max_attempts,
			created_at, updated_at
		) VALUES ($1, 'pending', 5, $2, $3, '', '', $4, $5, '', $6, 0, 3, $7, $7)
	`

	_, err = tx.ExecContext(ctx, insertQuery,
		newID, entry.SourceType, entry.SourceID, entry.ContactEmail,
		entry.MessageID, payloadJSON, now,
	)
	if err != nil {
		return fmt.Errorf("failed to re-enqueue entry: %w", err)
	}

	// Delete from dead letter
	deleteQuery := `DELETE FROM email_queue_dead_letter WHERE id = $1`
	_, err = tx.ExecContext(ctx, deleteQuery, deadLetterID)
	if err != nil {
		return fmt.Errorf("failed to delete from dead letter: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// scanEmailQueueEntry scans a row into an EmailQueueEntry
func scanEmailQueueEntry(rows *sql.Rows) (*domain.EmailQueueEntry, error) {
	var entry domain.EmailQueueEntry
	var payloadJSON []byte
	var lastError sql.NullString
	var nextRetryAt sql.NullTime
	var processedAt sql.NullTime

	err := rows.Scan(
		&entry.ID, &entry.Status, &entry.Priority, &entry.SourceType, &entry.SourceID,
		&entry.IntegrationID, &entry.ProviderKind, &entry.ContactEmail, &entry.MessageID,
		&entry.TemplateID, &payloadJSON, &entry.Attempts, &entry.MaxAttempts,
		&lastError, &nextRetryAt, &entry.CreatedAt, &entry.UpdatedAt, &processedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to scan email queue entry: %w", err)
	}

	if lastError.Valid {
		entry.LastError = &lastError.String
	}
	if nextRetryAt.Valid {
		entry.NextRetryAt = &nextRetryAt.Time
	}
	if processedAt.Valid {
		entry.ProcessedAt = &processedAt.Time
	}

	if err := json.Unmarshal(payloadJSON, &entry.Payload); err != nil {
		return nil, fmt.Errorf("failed to unmarshal payload: %w", err)
	}

	return &entry, nil
}
