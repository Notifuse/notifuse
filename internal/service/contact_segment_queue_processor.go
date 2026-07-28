package service

import (
	"context"
	"database/sql"
	"fmt"
	"runtime/debug"
	"strings"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/logger"
)

// detachedStatementTimeout bounds the claim and requeue statements, which run on
// contexts detached from the caller's cancellation (see claimBatch/requeueBatch).
// Both are fast, indexed, non-lock-blocking statements — normal execution is
// milliseconds — so this only trips if the DB is unresponsive, where failing
// fast is correct. It also caps how long a detached call can delay shutdown.
const detachedStatementTimeout = 5 * time.Second

// ContactSegmentQueueProcessor processes queued contacts for segment recomputation
type ContactSegmentQueueProcessor struct {
	queueRepo     domain.ContactSegmentQueueRepository
	segmentRepo   domain.SegmentRepository
	contactRepo   domain.ContactRepository
	workspaceRepo domain.WorkspaceRepository
	queryBuilder  *QueryBuilder
	logger        logger.Logger
	batchSize     int
}

// NewContactSegmentQueueProcessor creates a new contact segment queue processor
func NewContactSegmentQueueProcessor(
	queueRepo domain.ContactSegmentQueueRepository,
	segmentRepo domain.SegmentRepository,
	contactRepo domain.ContactRepository,
	workspaceRepo domain.WorkspaceRepository,
	logger logger.Logger,
) *ContactSegmentQueueProcessor {
	return &ContactSegmentQueueProcessor{
		queueRepo:     queueRepo,
		segmentRepo:   segmentRepo,
		contactRepo:   contactRepo,
		workspaceRepo: workspaceRepo,
		queryBuilder:  NewQueryBuilder(),
		logger:        logger,
		batchSize:     100, // Process up to 100 contacts at a time
	}
}

// ProcessQueue claims a batch of pending contacts and recomputes their segment
// memberships. Returns the number of contacts successfully processed.
//
// Rows are claimed by DELETING them in a single atomic statement (a DELETE ...
// RETURNING backed by a FOR UPDATE SKIP LOCKED sub-select) that commits on its
// own, so NO transaction is held open across the per-contact work. Holding the
// claim inside a transaction for the whole per-contact loop — while doing the
// actual work on a separate pooled connection — left that transaction idle in
// transaction with the queue rows still locked. Any concurrent write that
// re-enqueued a claimed contact (every broadcast's message_history insert reaches
// contact_segment_queue through the AFTER INSERT trigger cascade) then blocked
// behind the idle claimer, a wait invisible to Postgres's deadlock detector,
// freezing broadcasts and the task scheduler until the backend was killed.
//
// A contact whose recomputation fails (or is cut short by context cancellation)
// is re-enqueued so it is retried on a later pass; because the claim already
// removed it, nothing is silently dropped. A hard crash (or shutdown racing the
// connection-pool close) between the claim commit and processing drops that
// batch: those contacts recompute on their next event (or, for relative-date
// segments, the daily rebuild).
func (p *ContactSegmentQueueProcessor) ProcessQueue(ctx context.Context, workspaceID string) (int, error) {
	// A cancelled caller (task timeout, shutdown) must not claim new work: the
	// claim deliberately ignores cancellation once started (see claimBatch), so
	// gate entry here instead.
	if err := ctx.Err(); err != nil {
		return 0, err
	}

	// Get workspace DB connection
	workspaceDB, err := p.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return 0, fmt.Errorf("failed to get workspace connection: %w", err)
	}

	// Claim a batch: atomically delete the rows and return their emails. The
	// FOR UPDATE SKIP LOCKED makes concurrent claimers take disjoint sets; the
	// surrounding DELETE removes exactly the claimed set. No open transaction is
	// held across processing, so nothing stays locked beyond this one statement.
	emails, err := p.claimBatch(ctx, workspaceDB, p.batchSize)
	if err != nil {
		return 0, err // claimBatch already wraps with context
	}

	if len(emails) == 0 {
		p.logger.WithField("workspace_id", workspaceID).Debug("No pending contacts to process")
		return 0, nil
	}

	p.logger.WithFields(map[string]interface{}{
		"workspace_id": workspaceID,
		"count":        len(emails),
	}).Info("Processing contact segment queue")

	// Get all active segments for this workspace
	segments, err := p.segmentRepo.GetSegments(ctx, workspaceID, false)
	if err != nil {
		// The claimed rows are already removed; re-enqueue them so this batch is
		// retried rather than lost, then surface the error.
		p.requeueOrLog(ctx, workspaceDB, emails)
		return 0, fmt.Errorf("failed to get segments: %w", err)
	}

	// Filter for active segments only
	activeSegments := make([]*domain.Segment, 0)
	for _, segment := range segments {
		if segment.Status == string(domain.SegmentStatusActive) {
			activeSegments = append(activeSegments, segment)
		}
	}

	if len(activeSegments) == 0 {
		// No segments to evaluate; the claimed rows are already removed.
		p.logger.WithField("workspace_id", workspaceID).Debug("No active segments found, queue cleared")
		return len(emails), nil
	}

	// Process each claimed contact on the pooled connection. No locks are held,
	// so a concurrent broadcast send (or any other re-enqueueing write) is never
	// blocked by this worker.
	failedEmails := make([]string, 0)
	for i, email := range emails {
		// Stop early if the caller's context is cancelled (e.g. the task hit its
		// runtime budget) and re-enqueue whatever remains so it is retried.
		if ctx.Err() != nil {
			failedEmails = append(failedEmails, emails[i:]...)
			break
		}

		err := func() (procErr error) {
			// A panic evaluating one contact must not kill the process: this is
			// a permanent recurring task, so crashing here would re-claim the
			// same contact after the debounce and crash again, forever. Convert
			// to an error so the contact takes the normal failed-contact retry
			// path below.
			defer func() {
				if r := recover(); r != nil {
					procErr = fmt.Errorf("panic processing contact: %v\n%s", r, debug.Stack())
				}
			}()
			return p.processContact(ctx, workspaceID, workspaceDB, email, activeSegments)
		}()
		if err != nil {
			p.logger.WithFields(map[string]interface{}{
				"email": email,
				"error": err.Error(),
			}).Error("Failed to process contact")
			// Re-enqueue this contact so its recomputation is retried later.
			failedEmails = append(failedEmails, email)
			continue
		}
	}

	if len(failedEmails) > 0 {
		p.requeueOrLog(ctx, workspaceDB, failedEmails)
	}

	processedCount := len(emails) - len(failedEmails)
	p.logger.WithFields(map[string]interface{}{
		"workspace_id":    workspaceID,
		"processed_count": processedCount,
		"failed_count":    len(failedEmails),
	}).Info("Completed processing contact segment queue")

	return processedCount, nil
}

// claimBatch atomically claims up to limit pending contacts: it deletes their
// contact_segment_queue rows and returns the emails. The FOR UPDATE SKIP LOCKED
// sub-select makes concurrent claimers take disjoint sets; the DELETE then
// removes exactly what was claimed. A 15-second debounce (queued_at older than
// 15s) avoids processing contacts that are being updated in rapid bursts.
//
// The claim runs on a context detached from the caller's cancellation, inside a
// short transaction of its own. Detachment: once the DELETE reaches the server,
// a caller-side cancel could commit the delete yet abort the row scan, losing
// the claimed contacts; the statement cannot lock-block (SKIP LOCKED), so the
// short detached timeout is a sufficient bound. The transaction makes the delete
// and the row scan succeed or fail as a unit: any scan failure rolls back and
// the rows survive for the next pass. Unlike the idle-in-transaction bug this
// file fixes, nothing runs inside this transaction but the claim itself —
// begin/query/scan/commit are consecutive statements on one connection, and the
// commit happens BEFORE any per-contact work starts, so no lock is held while
// contacts are processed.
func (p *ContactSegmentQueueProcessor) claimBatch(ctx context.Context, db *sql.DB, limit int) ([]string, error) {
	const query = `
		WITH claimed AS (
			SELECT email
			FROM contact_segment_queue
			WHERE queued_at < NOW() - INTERVAL '15 seconds'
			ORDER BY queued_at ASC
			LIMIT $1
			FOR UPDATE SKIP LOCKED
		)
		DELETE FROM contact_segment_queue q
		USING claimed
		WHERE q.email = claimed.email
		RETURNING q.email
	`

	claimCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), detachedStatementTimeout)
	defer cancel()

	tx, err := db.BeginTx(claimCtx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to claim pending emails: %w", err)
	}
	defer func() {
		_ = tx.Rollback() // no-op after a successful Commit
	}()

	rows, err := tx.QueryContext(claimCtx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to claim pending emails: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var emails []string
	for rows.Next() {
		var email string
		if err := rows.Scan(&email); err != nil {
			return nil, fmt.Errorf("failed to claim pending emails: %w", err)
		}
		emails = append(emails, email)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to claim pending emails: %w", err)
	}

	// The result set must be closed before the transaction can commit.
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("failed to claim pending emails: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to claim pending emails: %w", err)
	}

	return emails, nil
}

// requeueOrLog re-enqueues contacts whose processing did not complete, logging
// (but not failing) if the re-enqueue itself errors — a failed re-enqueue only
// means the contact waits for its next event, never a lost claim.
func (p *ContactSegmentQueueProcessor) requeueOrLog(ctx context.Context, db *sql.DB, emails []string) {
	if err := p.requeueBatch(ctx, db, emails); err != nil {
		p.logger.WithFields(map[string]interface{}{
			"count": len(emails),
			"error": err.Error(),
		}).Error("Failed to re-enqueue contacts for segment recomputation")
	}
}

// requeueBatch re-inserts emails into contact_segment_queue so they are retried.
// It runs on a context detached from the caller's cancellation/deadline: if the
// batch is being cancelled (e.g. task timeout), we still want the un-processed
// contacts back in the queue rather than silently dropped. ON CONFLICT refreshes
// queued_at, keeping the row a newer event may have re-created while it was in
// flight.
func (p *ContactSegmentQueueProcessor) requeueBatch(ctx context.Context, db *sql.DB, emails []string) error {
	if len(emails) == 0 {
		return nil
	}

	// Dedup: a multi-row ON CONFLICT (email) DO UPDATE errors if the same key
	// appears twice. Callers pass unique emails today; this keeps it robust.
	emails = dedupeStrings(emails)

	reqCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), detachedStatementTimeout)
	defer cancel()

	placeholders := make([]string, len(emails))
	args := make([]interface{}, len(emails))
	for i, email := range emails {
		placeholders[i] = fmt.Sprintf("($%d, NOW())", i+1)
		args[i] = email
	}

	query := fmt.Sprintf(`
		INSERT INTO contact_segment_queue (email, queued_at)
		VALUES %s
		ON CONFLICT (email) DO UPDATE SET queued_at = EXCLUDED.queued_at
	`, strings.Join(placeholders, ", "))

	if _, err := db.ExecContext(reqCtx, query, args...); err != nil {
		return fmt.Errorf("failed to re-enqueue emails: %w", err)
	}

	return nil
}

// processContact processes a single contact against all segments
// Uses a single query to evaluate all segments at once for better performance
func (p *ContactSegmentQueueProcessor) processContact(ctx context.Context, workspaceID string, workspaceDB *sql.DB, email string, segments []*domain.Segment) error {
	if len(segments) == 0 {
		return nil
	}

	// Build a single query that checks all segments at once using UNION ALL
	// Each segment's stored SQL is used to check if the contact matches
	var queryParts []string
	var allArgs []interface{}
	argOffset := 1

	for _, segment := range segments {
		// Use the pre-generated SQL stored in the segment
		segmentSQL := segment.GeneratedSQL
		if segmentSQL == nil || *segmentSQL == "" {
			p.logger.WithField("segment_id", segment.ID).Warn("Segment has no generated SQL, skipping")
			continue
		}

		// Use the stored args directly (already in correct order)
		segmentArgs := segment.GeneratedArgs
		if segmentArgs == nil {
			segmentArgs = make([]interface{}, 0)
		}

		// Add email filter to the segment's SQL and rebind placeholders
		emailFilteredSQL := *segmentSQL + " AND email = $" + fmt.Sprintf("%d", len(segmentArgs)+1)

		// Rebind placeholders to account for all previous args
		reboundSQL := p.rebindPlaceholders(emailFilteredSQL, argOffset)

		// Add to union query with segment ID
		queryParts = append(queryParts, fmt.Sprintf("SELECT '%s' as segment_id WHERE EXISTS (%s)", segment.ID, reboundSQL))

		// Add segment args and email to all args
		allArgs = append(allArgs, segmentArgs...)
		allArgs = append(allArgs, email)
		argOffset += len(segmentArgs) + 1
	}

	if len(queryParts) == 0 {
		return nil
	}

	// Combine all parts with UNION ALL
	fullQuery := "(" + queryParts[0] + ")"
	for i := 1; i < len(queryParts); i++ {
		fullQuery += " UNION ALL (" + queryParts[i] + ")"
	}

	// Execute the combined query to get all matching segment IDs
	rows, err := workspaceDB.QueryContext(ctx, fullQuery, allArgs...)
	if err != nil {
		return fmt.Errorf("failed to evaluate segments: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	matchingSegments := make(map[string]bool)
	for rows.Next() {
		var segmentID string
		if err := rows.Scan(&segmentID); err != nil {
			return fmt.Errorf("failed to scan segment ID: %w", err)
		}
		matchingSegments[segmentID] = true
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("error iterating segment matches: %w", err)
	}

	// Now update segment memberships based on matches
	for _, segment := range segments {
		if matchingSegments[segment.ID] {
			// Contact matches - add to segment
			if err := p.segmentRepo.AddContactToSegment(ctx, workspaceID, email, segment.ID, segment.Version); err != nil {
				p.logger.WithFields(map[string]interface{}{
					"segment_id": segment.ID,
					"email":      email,
					"error":      err.Error(),
				}).Warn("Failed to add contact to segment")
			}
		} else {
			// Contact doesn't match - remove from segment if exists
			if err := p.segmentRepo.RemoveContactFromSegment(ctx, workspaceID, email, segment.ID); err != nil {
				// It's OK if the contact wasn't in the segment, ignore the error
				p.logger.WithFields(map[string]interface{}{
					"segment_id": segment.ID,
					"email":      email,
				}).Debug("Contact not in segment or already removed")
			}
		}
	}

	return nil
}

// rebindPlaceholders rebinds SQL placeholders starting from the given offset
// e.g., $1, $2, $3 becomes $5, $6, $7 if offset is 5
func (p *ContactSegmentQueueProcessor) rebindPlaceholders(sql string, offset int) string {
	result := ""
	placeholderNum := 1
	i := 0

	for i < len(sql) {
		if sql[i] == '$' && i+1 < len(sql) {
			// Found a placeholder, extract the number
			j := i + 1
			for j < len(sql) && sql[j] >= '0' && sql[j] <= '9' {
				j++
			}

			// Replace with new placeholder number
			result += fmt.Sprintf("$%d", offset+placeholderNum-1)
			placeholderNum++
			i = j
		} else {
			result += string(sql[i])
			i++
		}
	}

	return result
}

// GetQueueSize returns the number of contacts waiting to be processed
func (p *ContactSegmentQueueProcessor) GetQueueSize(ctx context.Context, workspaceID string) (int, error) {
	return p.queueRepo.GetQueueSize(ctx, workspaceID)
}
