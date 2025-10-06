package service

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/logger"
)

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

// ProcessQueue processes pending contacts in the queue for segment recomputation
// Uses a transaction to ensure row locks are held during processing
// Returns the number of contacts successfully processed
func (p *ContactSegmentQueueProcessor) ProcessQueue(ctx context.Context, workspaceID string) (int, error) {
	// Get workspace DB connection
	workspaceDB, err := p.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return 0, fmt.Errorf("failed to get workspace connection: %w", err)
	}

	// Start a transaction to hold locks during processing
	tx, err := workspaceDB.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback() // Rollback if not committed

	// Get pending emails (locks them with FOR UPDATE SKIP LOCKED)
	emails, err := p.getPendingEmailsInTx(ctx, tx, p.batchSize)
	if err != nil {
		return 0, fmt.Errorf("failed to get pending emails: %w", err)
	}

	if len(emails) == 0 {
		p.logger.WithField("workspace_id", workspaceID).Debug("No pending contacts to process")
		return 0, nil // Nothing to commit, just return
	}

	p.logger.WithFields(map[string]interface{}{
		"workspace_id": workspaceID,
		"count":        len(emails),
	}).Info("Processing contact segment queue")

	// Get all active segments for this workspace
	segments, err := p.segmentRepo.GetSegments(ctx, workspaceID, false)
	if err != nil {
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
		p.logger.WithField("workspace_id", workspaceID).Debug("No active segments found, clearing queue")
		// Remove from queue since there are no segments to update
		if err := p.removeBatchFromQueueInTx(ctx, tx, emails); err != nil {
			p.logger.WithField("error", err.Error()).Warn("Failed to clear queue")
			return 0, err
		}
		if err := tx.Commit(); err != nil {
			return 0, err
		}
		return len(emails), nil
	}

	processedEmails := make([]string, 0, len(emails))

	// Process each queued contact
	for _, email := range emails {
		if err := p.processContact(ctx, workspaceID, workspaceDB, email, activeSegments); err != nil {
			p.logger.WithFields(map[string]interface{}{
				"email": email,
				"error": err.Error(),
			}).Error("Failed to process contact")
			// Continue processing other contacts even if one fails
			continue
		}
		processedEmails = append(processedEmails, email)
	}

	// Remove processed emails from queue within the transaction
	if len(processedEmails) > 0 {
		if err := p.removeBatchFromQueueInTx(ctx, tx, processedEmails); err != nil {
			p.logger.WithField("error", err.Error()).Error("Failed to remove processed emails from queue")
			return 0, err
		}
	}

	// Commit the transaction (releases locks)
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("failed to commit transaction: %w", err)
	}

	p.logger.WithFields(map[string]interface{}{
		"workspace_id":    workspaceID,
		"processed_count": len(processedEmails),
		"failed_count":    len(emails) - len(processedEmails),
	}).Info("Completed processing contact segment queue")

	return len(processedEmails), nil
}

// getPendingEmailsInTx gets pending emails within a transaction (with row locks)
// Applies a 15-second debounce to avoid processing contacts that are being updated rapidly
func (p *ContactSegmentQueueProcessor) getPendingEmailsInTx(ctx context.Context, tx *sql.Tx, limit int) ([]string, error) {
	query := `
		SELECT email
		FROM contact_segment_queue
		WHERE queued_at < NOW() - INTERVAL '15 seconds'
		ORDER BY queued_at ASC
		LIMIT $1
		FOR UPDATE SKIP LOCKED
	`

	rows, err := tx.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query pending emails: %w", err)
	}
	defer rows.Close()

	var emails []string
	for rows.Next() {
		var email string
		if err := rows.Scan(&email); err != nil {
			return nil, fmt.Errorf("failed to scan email: %w", err)
		}
		emails = append(emails, email)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating emails: %w", err)
	}

	return emails, nil
}

// removeBatchFromQueueInTx removes multiple emails from the queue within a transaction
func (p *ContactSegmentQueueProcessor) removeBatchFromQueueInTx(ctx context.Context, tx *sql.Tx, emails []string) error {
	if len(emails) == 0 {
		return nil
	}

	// Import pq for array support
	query := `DELETE FROM contact_segment_queue WHERE email = ANY($1)`

	// Convert emails to a format compatible with pq.Array
	emailsArray := make([]interface{}, len(emails))
	for i, email := range emails {
		emailsArray[i] = email
	}

	// Use string concatenation to build a proper array literal
	placeholders := ""
	args := make([]interface{}, len(emails))
	for i, email := range emails {
		if i > 0 {
			placeholders += ","
		}
		placeholders += fmt.Sprintf("$%d", i+1)
		args[i] = email
	}

	query = fmt.Sprintf("DELETE FROM contact_segment_queue WHERE email IN (%s)", placeholders)

	_, err := tx.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("failed to remove emails from queue: %w", err)
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
	defer rows.Close()

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
