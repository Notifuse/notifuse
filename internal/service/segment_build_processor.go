package service

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/lib/pq"
)

// SegmentBuildProcessor handles the execution of segment building tasks
type SegmentBuildProcessor struct {
	segmentRepo   domain.SegmentRepository
	contactRepo   domain.ContactRepository
	taskRepo      domain.TaskRepository
	workspaceRepo domain.WorkspaceRepository
	queryBuilder  *QueryBuilder
	logger        logger.Logger
	batchSize     int // Number of contacts to process per batch
}

// NewSegmentBuildProcessor creates a new segment build processor
func NewSegmentBuildProcessor(
	segmentRepo domain.SegmentRepository,
	contactRepo domain.ContactRepository,
	taskRepo domain.TaskRepository,
	workspaceRepo domain.WorkspaceRepository,
	logger logger.Logger,
) *SegmentBuildProcessor {
	return &SegmentBuildProcessor{
		segmentRepo:   segmentRepo,
		contactRepo:   contactRepo,
		taskRepo:      taskRepo,
		workspaceRepo: workspaceRepo,
		queryBuilder:  NewQueryBuilder(),
		logger:        logger,
		batchSize:     100, // Process 100 contacts at a time, allows frequent version checks
	}
}

// CanProcess returns whether this processor can handle the given task type
func (p *SegmentBuildProcessor) CanProcess(taskType string) bool {
	return taskType == "build_segment"
}

// Process executes or continues a segment building task
func (p *SegmentBuildProcessor) Process(ctx context.Context, task *domain.Task, timeoutAt time.Time) (completed bool, err error) {
	p.logger.WithFields(map[string]interface{}{
		"task_id":      task.ID,
		"workspace_id": task.WorkspaceID,
		"type":         task.Type,
	}).Info("Processing segment build task")

	// Get or initialize the build state
	state := task.State.BuildSegment
	if state == nil {
		return false, fmt.Errorf("task state missing BuildSegment data - task may not have been properly initialized")
	}

	// Ensure state has required fields
	if state.SegmentID == "" {
		return false, fmt.Errorf("build state missing segment_id")
	}

	if state.BatchSize == 0 {
		state.BatchSize = p.batchSize
	}

	if state.StartedAt == "" {
		state.StartedAt = time.Now().UTC().Format(time.RFC3339)
	}

	// Fetch the segment
	segment, err := p.segmentRepo.GetSegmentByID(ctx, task.WorkspaceID, state.SegmentID)
	if err != nil {
		return false, fmt.Errorf("failed to fetch segment: %w", err)
	}

	// Update segment status to "building" if not already
	if segment.Status != string(domain.SegmentStatusBuilding) {
		segment.Status = string(domain.SegmentStatusBuilding)
		if err := p.segmentRepo.UpdateSegment(ctx, task.WorkspaceID, segment); err != nil {
			return false, fmt.Errorf("failed to update segment status: %w", err)
		}
	}

	// Store the version we're building
	if state.Version == 0 {
		state.Version = segment.Version
	}

	// Use the pre-generated SQL and args stored in the segment
	// These are generated when the segment is created/updated
	if segment.GeneratedSQL == nil || *segment.GeneratedSQL == "" {
		return false, fmt.Errorf("segment has no generated SQL - segment may not have been properly initialized")
	}

	sqlQuery := *segment.GeneratedSQL
	args := []interface{}(segment.GeneratedArgs)

	// Get total contact count if not already set
	if state.TotalContacts == 0 {
		totalCount, err := p.contactRepo.Count(ctx, task.WorkspaceID)
		if err != nil {
			return false, fmt.Errorf("failed to count contacts: %w", err)
		}
		state.TotalContacts = totalCount
	}

	// Process contacts in batches
	for {
		// Check if we're approaching timeout
		if time.Now().Add(5 * time.Second).After(timeoutAt) {
			// Save progress and pause for next execution
			p.logger.Info("Approaching timeout, pausing segment build")
			if err := p.saveProgress(ctx, task, state); err != nil {
				return false, fmt.Errorf("failed to save progress: %w", err)
			}
			return false, nil
		}

		// Refetch segment to check if version has changed (segment was updated)
		currentSegment, err := p.segmentRepo.GetSegmentByID(ctx, task.WorkspaceID, state.SegmentID)
		if err != nil {
			return false, fmt.Errorf("failed to refetch segment: %w", err)
		}

		// If segment version has changed, this task is outdated
		if currentSegment.Version > state.Version {
			p.logger.WithFields(map[string]interface{}{
				"task_version":     state.Version,
				"current_version":  currentSegment.Version,
				"segment_id":       state.SegmentID,
				"processed_so_far": state.ProcessedCount,
			}).Info("Segment was updated, aborting outdated task")

			// Clean up partial work from this outdated task version
			// Remove all memberships with version < currentVersion to clean up stale data
			if err := p.segmentRepo.RemoveOldMemberships(ctx, task.WorkspaceID, state.SegmentID, currentSegment.Version); err != nil {
				p.logger.WithFields(map[string]interface{}{
					"error":      err.Error(),
					"segment_id": state.SegmentID,
					"version":    state.Version,
				}).Warn("Failed to clean up old memberships on abort (non-fatal)")
				// Don't fail the task - the next successful build will clean it up
			} else {
				p.logger.WithFields(map[string]interface{}{
					"segment_id":      state.SegmentID,
					"cleaned_version": state.Version,
				}).Info("Cleaned up partial memberships from aborted task")
			}

			// Mark task as completed (it's superseded, not a failure)
			return true, nil
		}

		// Get next batch of emails (optimized - only fetches email addresses)
		emails, err := p.contactRepo.GetBatchForSegment(ctx, task.WorkspaceID, state.ContactOffset, p.batchSize)
		if err != nil {
			return false, fmt.Errorf("failed to fetch email batch: %w", err)
		}

		// If no more emails, we're done
		if len(emails) == 0 {
			break
		}

		// Process this batch
		if err := p.processBatch(ctx, task.WorkspaceID, sqlQuery, args, emails, state); err != nil {
			return false, fmt.Errorf("failed to process batch: %w", err)
		}

		// Update offset for next batch
		state.ContactOffset += int64(len(emails))
		state.ProcessedCount += len(emails)

		// Update progress
		if state.TotalContacts > 0 {
			task.Progress = float64(state.ProcessedCount) / float64(state.TotalContacts)
		}

		// Save progress periodically
		if err := p.saveProgress(ctx, task, state); err != nil {
			p.logger.WithField("error", err.Error()).Warn("Failed to save progress (non-fatal)")
		}
	}

	// If no contacts matched, skip the segment building
	if state.MatchedCount == 0 {
		p.logger.WithFields(map[string]interface{}{
			"segment_id":     state.SegmentID,
			"version":        state.Version,
			"total_contacts": state.TotalContacts,
		}).Info("Segment build skipped: no contacts matched the criteria")

		// Update segment status to "active" but with no members
		segment.Status = string(domain.SegmentStatusActive)
		if err := p.segmentRepo.UpdateSegment(ctx, task.WorkspaceID, segment); err != nil {
			return false, fmt.Errorf("failed to update segment status to active: %w", err)
		}

		return true, nil
	}

	// Remove old memberships from previous versions
	if err := p.segmentRepo.RemoveOldMemberships(ctx, task.WorkspaceID, state.SegmentID, state.Version); err != nil {
		return false, fmt.Errorf("failed to remove old memberships: %w", err)
	}

	// Update segment status to "active"
	segment.Status = string(domain.SegmentStatusActive)
	if err := p.segmentRepo.UpdateSegment(ctx, task.WorkspaceID, segment); err != nil {
		return false, fmt.Errorf("failed to update segment status to active: %w", err)
	}

	// If segment has recompute_after set, reschedule it for next 5AM
	if segment.RecomputeAfter != nil {
		next5AM, err := calculateNext5AMInTimezone(segment.Timezone)
		if err != nil {
			p.logger.WithFields(map[string]interface{}{
				"error":      err.Error(),
				"segment_id": state.SegmentID,
				"timezone":   segment.Timezone,
			}).Warn("Failed to calculate next 5AM for recompute rescheduling (non-fatal)")
		} else {
			if err := p.segmentRepo.UpdateRecomputeAfter(ctx, task.WorkspaceID, state.SegmentID, &next5AM); err != nil {
				p.logger.WithFields(map[string]interface{}{
					"error":      err.Error(),
					"segment_id": state.SegmentID,
				}).Warn("Failed to update recompute_after for segment (non-fatal)")
			} else {
				p.logger.WithFields(map[string]interface{}{
					"segment_id":      state.SegmentID,
					"recompute_after": next5AM.Format(time.RFC3339),
				}).Info("Rescheduled segment for next daily recomputation")
			}
		}
	}

	p.logger.WithFields(map[string]interface{}{
		"segment_id":     state.SegmentID,
		"version":        state.Version,
		"total_contacts": state.TotalContacts,
		"matched_count":  state.MatchedCount,
	}).Info("Segment build completed")

	return true, nil
}

// processBatch processes a batch of emails against the segment criteria
func (p *SegmentBuildProcessor) processBatch(
	ctx context.Context,
	workspaceID string,
	sqlQuery string,
	args []interface{},
	emails []string,
	state *domain.BuildSegmentState,
) error {
	// Execute the segment query with email filter
	// We need to modify the query to only check contacts in this batch
	batchQuery := sqlQuery + " AND email = ANY($" + fmt.Sprintf("%d", len(args)+1) + ")"
	batchArgs := append(args, pq.Array(emails))

	// Execute query to find matching contacts
	rows, err := p.executeSegmentQuery(ctx, workspaceID, batchQuery, batchArgs)
	if err != nil {
		return fmt.Errorf("failed to execute segment query: %w", err)
	}
	defer func() { _ = rows.Close() }()

	// Track matched emails
	matchedEmails := make(map[string]bool)
	for rows.Next() {
		var email string
		if err := rows.Scan(&email); err != nil {
			return fmt.Errorf("failed to scan email: %w", err)
		}
		matchedEmails[email] = true
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("error iterating rows: %w", err)
	}

	// Add matched contacts to segment
	for email := range matchedEmails {
		if err := p.segmentRepo.AddContactToSegment(ctx, workspaceID, email, state.SegmentID, state.Version); err != nil {
			p.logger.WithFields(map[string]interface{}{
				"error": err.Error(),
				"email": email,
			}).Warn("Failed to add contact to segment")
			continue
		}
		state.MatchedCount++
	}

	return nil
}

// executeSegmentQuery executes the segment query against the contacts table
func (p *SegmentBuildProcessor) executeSegmentQuery(ctx context.Context, workspaceID string, query string, args []interface{}) (*sql.Rows, error) {
	// Get the workspace database connection
	workspaceDB, err := p.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace database connection: %w", err)
	}

	// Execute the query
	rows, err := workspaceDB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to execute segment query: %w", err)
	}

	return rows, nil
}

// saveProgress saves the current progress of the segment build
func (p *SegmentBuildProcessor) saveProgress(ctx context.Context, task *domain.Task, state *domain.BuildSegmentState) error {
	// Update state message
	task.State.Message = fmt.Sprintf("Processing contacts: %d/%d matched", state.MatchedCount, state.ProcessedCount)

	// Save state using transaction
	err := p.taskRepo.SaveState(ctx, task.WorkspaceID, task.ID, task.Progress, task.State)
	if err != nil {
		return fmt.Errorf("failed to save task state: %w", err)
	}

	return nil
}
