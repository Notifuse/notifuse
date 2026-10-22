package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/google/uuid"
	"github.com/lib/pq"
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

// The three ways a row is due for the worker. Each matches one index
// (idx_email_queue_pending, idx_email_queue_retry, idx_email_queue_integration), so a
// branch can be read in order and stop at its limit; ORed in one WHERE, they made
// PostgreSQL sort the whole backlog on every fetch.
//
// The attempt guard on the pending branch is a backstop. An entry that has spent
// its budget should never be 'pending', but anything that put it there would
// otherwise hand it straight back to the worker and mail someone the campaign has
// already reported as failed.
//
// Stuck processing rows (over 2 minutes) are recovered after a worker crash.
const (
	emailQueueDuePending      = `status = 'pending' AND attempts < max_attempts AND (next_retry_at IS NULL OR next_retry_at <= NOW())`
	emailQueueDueRetry        = `status = 'failed' AND attempts < max_attempts AND next_retry_at <= NOW()`
	emailQueueStuckProcessing = `status = 'processing' AND updated_at < NOW() - INTERVAL '2 minutes'`
)

var (
	// emailQueueDueKinds are the ways a row can be due: one fetch branch each.
	emailQueueDueKinds = []string{emailQueueDuePending, emailQueueDueRetry, emailQueueStuckProcessing}

	// emailQueueDue is any of them: what the fetches recheck under the lock and what
	// a claim requires, so the branches, the recheck and the claim cannot drift apart.
	emailQueueDue = `((` + strings.Join(emailQueueDueKinds, `) OR (`) + `))`
)

const emailQueueEntryColumns = `id, status, priority, source_type, source_id, integration_id, provider_kind,
		       contact_email, message_id, template_id, payload, attempts, max_attempts,
		       last_error, next_retry_at, created_at, updated_at, processed_at`

// dueRowsSQL selects up to $2 due rows that match filter, oldest first within
// priority. Each branch reads its own index in order and stops at its limit.
//
// The outer WHERE repeats the conditions. Depending on the plan, PostgreSQL can lock
// a row another instance changed after the statement began in its new version and
// recheck only the outer conditions against it, returning a row that is no longer
// due. MarkAsProcessing would refuse it; the recheck saves the wasted claim.
//
// The inner limits pick candidates before the outer SKIP LOCKED, so rows another
// instance's statement has locked at that instant shorten the round. Fetches and
// claims lock for one statement only, so that costs one short round at most.
func dueRowsSQL(filter string) string {
	branches := make([]string, len(emailQueueDueKinds))
	for i, due := range emailQueueDueKinds {
		branches[i] = `(SELECT id FROM email_queue
		 WHERE (` + filter + `) AND (` + due + `)
		 ORDER BY priority ASC, created_at ASC
		 LIMIT $2)`
	}
	return `
	SELECT ` + emailQueueEntryColumns + `
	FROM email_queue
	WHERE id IN (
		` + strings.Join(branches, `
		UNION ALL
		`) + `
	)
	AND (` + filter + `) AND ` + emailQueueDue + `
	ORDER BY priority ASC, created_at ASC
	LIMIT $2
	FOR UPDATE SKIP LOCKED
`
}

var (
	fetchPendingForIntegrationSQL = dueRowsSQL(`integration_id = $1`)
	// Without rows of unknown integrations, each branch reads its whole status, which
	// is why the worker runs it about once a minute per workspace rather than on every
	// round.
	fetchPendingOrphansSQL = dueRowsSQL(`integration_id <> ALL($1)`)
)

// FetchPendingForIntegration returns up to limit due rows of one integration, ordered
// by priority (lower = higher priority), then creation time
func (r *EmailQueueRepository) FetchPendingForIntegration(ctx context.Context, workspaceID string, integrationID string, limit int) ([]*domain.EmailQueueEntry, error) {
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get database connection: %w", err)
	}

	rows, err := db.QueryContext(ctx, fetchPendingForIntegrationSQL, integrationID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query pending emails: %w", err)
	}
	return scanEmailQueueEntries(rows)
}

// FetchPendingOrphans returns up to limit due rows whose integration is not in
// integrationIDs
func (r *EmailQueueRepository) FetchPendingOrphans(ctx context.Context, workspaceID string, integrationIDs []string, limit int) ([]*domain.EmailQueueEntry, error) {
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get database connection: %w", err)
	}

	// pq.Array(nil) is NULL, and integration_id <> ALL(NULL) matches nothing: a
	// workspace whose email integrations were all deleted would keep its rows forever.
	if integrationIDs == nil {
		integrationIDs = []string{}
	}

	rows, err := db.QueryContext(ctx, fetchPendingOrphansSQL, pq.Array(integrationIDs), limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query orphaned emails: %w", err)
	}
	return scanEmailQueueEntries(rows)
}

// scanEmailQueueEntries reads every row and closes rows
func scanEmailQueueEntries(rows *sql.Rows) ([]*domain.EmailQueueEntry, error) {
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

	// A row is claimed only while the fetch would return it. Another instance can fail
	// a row between this instance's fetch and its claim; without the conditions, the
	// row would be sent again at once, before its retry date or after it was given up.
	query := `
		UPDATE email_queue
		SET status = 'processing', updated_at = NOW(), attempts = attempts + 1
		WHERE id = $1 AND ` + emailQueueDue + `
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
		return fmt.Errorf("email not found, already processing, or not due: %s", id)
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

// Delete removes a queue entry (used when max retries exhausted)
func (r *EmailQueueRepository) Delete(ctx context.Context, workspaceID string, entryID string) error {
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get database connection: %w", err)
	}

	_, err = db.ExecContext(ctx, `DELETE FROM email_queue WHERE id = $1`, entryID)
	if err != nil {
		return fmt.Errorf("failed to delete queue entry: %w", err)
	}

	return nil
}

// ListActiveSourcesByIntegration returns the sources still riding on one integration.
// Covered by idx_email_queue_integration (integration_id, status).
func (r *EmailQueueRepository) ListActiveSourcesByIntegration(ctx context.Context, workspaceID string, sourceType domain.EmailQueueSourceType, integrationID string) ([]string, error) {
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get database connection: %w", err)
	}

	// 'paused' is excluded on purpose: a source already stopped needs nothing done
	// to it, and including it would make a flapping breaker rewrite its pause reason.
	query := `
		SELECT DISTINCT source_id
		FROM email_queue
		WHERE source_type = $1 AND integration_id = $2
		  AND status IN ('pending', 'processing', 'failed')
	`

	rows, err := db.QueryContext(ctx, query, sourceType, integrationID)
	if err != nil {
		return nil, fmt.Errorf("failed to list active sources by integration: %w", err)
	}
	defer rows.Close()

	var sourceIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("failed to scan source id: %w", err)
		}
		sourceIDs = append(sourceIDs, id)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating source ids: %w", err)
	}

	return sourceIDs, nil
}

// MarkAsPermanentlyFailed records that an entry has spent every attempt.
// The row stays: deleting it was what made the recipient unrecoverable and left the
// broadcast reading as complete. processed_at is stamped here — this is its first
// writer — because the retention sweep needs to know when the entry was given up on.
func (r *EmailQueueRepository) MarkAsPermanentlyFailed(ctx context.Context, workspaceID string, entryID string, errorMsg string) error {
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get database connection: %w", err)
	}

	query := `
		UPDATE email_queue
		SET status = 'failed',
		    last_error = $2,
		    next_retry_at = NULL,
		    processed_at = NOW(),
		    updated_at = NOW()
		WHERE id = $1
	`

	if _, err := db.ExecContext(ctx, query, entryID, errorMsg); err != nil {
		return fmt.Errorf("failed to mark entry as permanently failed: %w", err)
	}

	return nil
}

// ResetTerminallyFailedBySource requeues the entries a source gave up on.
func (r *EmailQueueRepository) ResetTerminallyFailedBySource(ctx context.Context, workspaceID string, sourceType domain.EmailQueueSourceType, sourceID string, integrationID string) (int64, error) {
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return 0, fmt.Errorf("failed to get database connection: %w", err)
	}

	query := `
		UPDATE email_queue
		SET status = 'pending',
		    attempts = 0,
		    next_retry_at = NULL,
		    last_error = NULL,
		    processed_at = NULL,
		    integration_id = $3,
		    updated_at = NOW()
		WHERE source_type = $1 AND source_id = $2
		  AND status = 'failed' AND next_retry_at IS NULL
	`

	result, err := db.ExecContext(ctx, query, sourceType, sourceID, integrationID)
	if err != nil {
		return 0, fmt.Errorf("failed to requeue terminally failed entries: %w", err)
	}

	return result.RowsAffected()
}

// DeleteTerminallyFailedOlderThan sweeps abandoned entries past the retention window.
func (r *EmailQueueRepository) DeleteTerminallyFailedOlderThan(ctx context.Context, workspaceID string, days int) (int64, error) {
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return 0, fmt.Errorf("failed to get database connection: %w", err)
	}

	query := `
		DELETE FROM email_queue
		WHERE status = 'failed'
		  AND next_retry_at IS NULL
		  AND processed_at IS NOT NULL
		  AND processed_at < NOW() - make_interval(days => $1)
	`

	result, err := db.ExecContext(ctx, query, days)
	if err != nil {
		return 0, fmt.Errorf("failed to delete expired terminally failed entries: %w", err)
	}

	return result.RowsAffected()
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

// GetSourceCounts breaks a source's rows down by what each one is waiting on.
// Covered by idx_email_queue_source (source_type, source_id, status).
func (r *EmailQueueRepository) GetSourceCounts(ctx context.Context, workspaceID string, sourceType domain.EmailQueueSourceType, sourceID string) (*domain.EmailQueueSourceCounts, error) {
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get database connection: %w", err)
	}

	// 'failed' splits on next_retry_at, not on the attempt count. A row that has been
	// given up on carries no retry date, and that is true whether it exhausted three
	// attempts or was refused once by a rejection there is no point repeating — a
	// mailbox that does not exist gets one attempt, and counting attempts would file
	// it under "still coming back".
	query := `
		SELECT
			COALESCE(SUM(CASE WHEN status = 'pending' THEN 1 ELSE 0 END), 0) AS pending,
			COALESCE(SUM(CASE WHEN status = 'processing' THEN 1 ELSE 0 END), 0) AS processing,
			COALESCE(SUM(CASE WHEN status = 'paused' THEN 1 ELSE 0 END), 0) AS paused,
			COALESCE(SUM(CASE WHEN status = 'failed' AND next_retry_at IS NOT NULL THEN 1 ELSE 0 END), 0) AS failed_retrying,
			COALESCE(SUM(CASE WHEN status = 'failed' AND next_retry_at IS NULL THEN 1 ELSE 0 END), 0) AS failed_terminal
		FROM email_queue
		WHERE source_type = $1 AND source_id = $2
	`

	counts := &domain.EmailQueueSourceCounts{}
	err = db.QueryRowContext(ctx, query, sourceType, sourceID).Scan(
		&counts.Pending, &counts.Processing, &counts.Paused,
		&counts.FailedRetrying, &counts.FailedTerminal,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get source counts: %w", err)
	}

	return counts, nil
}

// emailQueueExecutor is satisfied by both *sql.DB and *sql.Tx.
type emailQueueExecutor interface {
	ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
}

// Pausing must not touch an entry that has already been given up on. Sweeping it
// into 'paused' would make it come back as 'pending' on resume, so a pause and a
// resume would silently retry recipients nobody asked to retry — and the count
// behind "Completed — N failed" would fall to zero on its own.
const pauseBySourceSQL = `
	UPDATE email_queue
	SET status = 'paused', updated_at = NOW()
	WHERE source_type = $1 AND source_id = $2
	  AND status IN ('pending', 'failed')
	  AND NOT (status = 'failed' AND next_retry_at IS NULL)
`

const resumeBySourceSQL = `
	UPDATE email_queue
	SET status = 'pending', next_retry_at = NULL, updated_at = NOW()
	WHERE source_type = $1 AND source_id = $2
	  AND status = 'paused'
`

const deleteBySourceSQL = `
	DELETE FROM email_queue
	WHERE source_type = $1 AND source_id = $2
	  AND status IN ('pending', 'failed', 'paused')
`

func execBySource(ctx context.Context, exec emailQueueExecutor, query string, sourceType domain.EmailQueueSourceType, sourceID string) (int64, error) {
	result, err := exec.ExecContext(ctx, query, sourceType, sourceID)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// PauseBySource marks pending/failed entries for a source as paused.
func (r *EmailQueueRepository) PauseBySource(ctx context.Context, workspaceID string, sourceType domain.EmailQueueSourceType, sourceID string) (int64, error) {
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return 0, fmt.Errorf("failed to get database connection: %w", err)
	}
	n, err := execBySource(ctx, db, pauseBySourceSQL, sourceType, sourceID)
	if err != nil {
		return 0, fmt.Errorf("failed to pause queue entries by source: %w", err)
	}
	return n, nil
}

// PauseBySourceTx is the transactional variant of PauseBySource.
func (r *EmailQueueRepository) PauseBySourceTx(ctx context.Context, tx *sql.Tx, sourceType domain.EmailQueueSourceType, sourceID string) (int64, error) {
	n, err := execBySource(ctx, tx, pauseBySourceSQL, sourceType, sourceID)
	if err != nil {
		return 0, fmt.Errorf("failed to pause queue entries by source: %w", err)
	}
	return n, nil
}

// ResumeBySource flips paused entries back to pending and clears next_retry_at.
func (r *EmailQueueRepository) ResumeBySource(ctx context.Context, workspaceID string, sourceType domain.EmailQueueSourceType, sourceID string) (int64, error) {
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return 0, fmt.Errorf("failed to get database connection: %w", err)
	}
	n, err := execBySource(ctx, db, resumeBySourceSQL, sourceType, sourceID)
	if err != nil {
		return 0, fmt.Errorf("failed to resume queue entries by source: %w", err)
	}
	return n, nil
}

// ResumeBySourceTx is the transactional variant of ResumeBySource.
func (r *EmailQueueRepository) ResumeBySourceTx(ctx context.Context, tx *sql.Tx, sourceType domain.EmailQueueSourceType, sourceID string) (int64, error) {
	n, err := execBySource(ctx, tx, resumeBySourceSQL, sourceType, sourceID)
	if err != nil {
		return 0, fmt.Errorf("failed to resume queue entries by source: %w", err)
	}
	return n, nil
}

// DeleteBySource deletes pending/failed/paused entries for a source.
func (r *EmailQueueRepository) DeleteBySource(ctx context.Context, workspaceID string, sourceType domain.EmailQueueSourceType, sourceID string) (int64, error) {
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return 0, fmt.Errorf("failed to get database connection: %w", err)
	}
	n, err := execBySource(ctx, db, deleteBySourceSQL, sourceType, sourceID)
	if err != nil {
		return 0, fmt.Errorf("failed to delete queue entries by source: %w", err)
	}
	return n, nil
}

// DeleteBySourceTx is the transactional variant of DeleteBySource.
func (r *EmailQueueRepository) DeleteBySourceTx(ctx context.Context, tx *sql.Tx, sourceType domain.EmailQueueSourceType, sourceID string) (int64, error) {
	n, err := execBySource(ctx, tx, deleteBySourceSQL, sourceType, sourceID)
	if err != nil {
		return 0, fmt.Errorf("failed to delete queue entries by source: %w", err)
	}
	return n, nil
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

const deleteForEmailSQL = `
	DELETE FROM email_queue
	WHERE contact_email = $1
`

// DeleteForEmail removes every queued email addressed to a contact.
//
// No status filter: see the interface doc for why 'processing' is included here
// but excluded from DeleteBySource.
func (r *EmailQueueRepository) DeleteForEmail(ctx context.Context, workspaceID string, email string) (int64, error) {
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return 0, fmt.Errorf("failed to get database connection: %w", err)
	}

	result, err := db.ExecContext(ctx, deleteForEmailSQL, email)
	if err != nil {
		return 0, fmt.Errorf("failed to delete queue entries for contact: %w", err)
	}
	return result.RowsAffected()
}
