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

// finalStepsBudget is the time a slice must have left to run the end-of-build steps after it
// processed batches. RemoveOldMemberships can delete many rows at once, each firing the membership
// triggers; with less time left the steps are put off to the next slice, which starts with a full
// budget.
const finalStepsBudget = 60 * time.Second

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

	// Store the version we're building
	if state.Version == 0 {
		state.Version = segment.Version
	}

	// A segment deleted after this task was queued stays deleted.
	if segment.Status == string(domain.SegmentStatusDeleted) {
		p.logger.WithField("segment_id", state.SegmentID).Info("Segment was deleted, dropping its build task")
		return true, nil
	}

	// Check if this task is already outdated before touching the segment status
	if segment.Version > state.Version {
		p.logger.WithFields(map[string]interface{}{
			"task_version":    state.Version,
			"current_version": segment.Version,
			"segment_id":      state.SegmentID,
		}).Info("Segment was updated before task started, aborting outdated task")

		return true, nil
	}

	// The daily rebuild creates its task before it moves the segment to the version the task
	// writes. Until that bump lands, writing would leave every member at a version the segment
	// does not have, so write nothing and let the retry find it landed.
	if segment.Version < state.Version {
		return false, errSegmentBehindBuild(segment.Version, state.Version)
	}

	// Update segment status to "building" if not already. Only the status is written, and only
	// while the segment is still at this version and not deleted, so an edit made since the read
	// above is never overwritten.
	if segment.Status != string(domain.SegmentStatusBuilding) {
		updated, err := p.segmentRepo.UpdateSegmentStatus(ctx, task.WorkspaceID, state.SegmentID, state.Version, string(domain.SegmentStatusBuilding))
		if err != nil {
			return false, fmt.Errorf("failed to update segment status: %w", err)
		}
		if !updated {
			p.logger.WithField("segment_id", state.SegmentID).Info("Segment changed before its build started, dropping the build task")
			return true, nil
		}
	}

	// Use the pre-generated SQL and args stored in the segment
	// These are generated when the segment is created/updated
	if segment.GeneratedSQL == nil || *segment.GeneratedSQL == "" {
		return false, fmt.Errorf("segment has no generated SQL - segment may not have been properly initialized")
	}

	completed, err = p.build(ctx, task, state, segment, timeoutAt)
	if err != nil && !task.HasRetriesLeft() {
		p.releaseAfterFinalFailure(ctx, task, state, segment)
	}
	return completed, err
}

// errSegmentBehindBuild is returned when the segment is at a lower version than the build.
func errSegmentBehindBuild(segmentVersion, buildVersion int64) error {
	return fmt.Errorf("segment is at version %d, behind the version %d this build writes", segmentVersion, buildVersion)
}

// build scans the contacts, writes the matching ones and runs the end-of-build steps. It runs
// once the segment is marked building at state.Version.
func (p *SegmentBuildProcessor) build(ctx context.Context, task *domain.Task, state *domain.BuildSegmentState, segment *domain.Segment, timeoutAt time.Time) (bool, error) {
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

	processedBatch := false

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

		if currentSegment.Status == string(domain.SegmentStatusDeleted) {
			p.logger.WithField("segment_id", state.SegmentID).Info("Segment was deleted during its build, stopping")
			return true, nil
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

		// A whole-row write from a snapshot read before this build's version bump put the
		// segment back a version: stop writing at a version the segment does not have.
		if currentSegment.Version < state.Version {
			return false, errSegmentBehindBuild(currentSegment.Version, state.Version)
		}

		emails, err := p.nextBatch(ctx, task.WorkspaceID, state)
		if err != nil {
			return false, fmt.Errorf("failed to fetch email batch: %w", err)
		}

		// If no more emails, we're done
		if len(emails) == 0 {
			break
		}

		// Process this batch. A failed write fails the task: its retry resumes after the last
		// batch saved, so the contact is written instead of being skipped.
		if err := p.processBatch(ctx, task.WorkspaceID, sqlQuery, args, emails, state); err != nil {
			return false, fmt.Errorf("failed to process batch: %w", err)
		}
		processedBatch = true

		// Move the cursor past this batch
		state.ContactOffset += int64(len(emails))
		state.ProcessedCount += len(emails)
		state.LastEmail = emails[len(emails)-1]

		// Update progress
		if state.TotalContacts > 0 {
			task.Progress = float64(state.ProcessedCount) / float64(state.TotalContacts)
		}

		// Save progress periodically
		if err := p.saveProgress(ctx, task, state); err != nil {
			p.logger.WithField("error", err.Error()).Warn("Failed to save progress (non-fatal)")
		}
	}

	// The end-of-build steps need room: after a slice spent on batches, leave them to the next
	// slice when little time is left. A slice that only has them left runs them whatever its
	// budget, so they are never put off forever.
	if processedBatch && time.Now().Add(finalStepsBudget).After(timeoutAt) {
		p.logger.Info("Little time left after the last batch, finishing segment build in the next run")
		if err := p.saveProgress(ctx, task, state); err != nil {
			return false, fmt.Errorf("failed to save progress: %w", err)
		}
		return false, nil
	}

	// Remove the memberships this build did not rewrite, whatever the number of matches: a build
	// that matches nobody must still drop the members of the previous version.
	if err := p.segmentRepo.RemoveOldMemberships(ctx, task.WorkspaceID, state.SegmentID, state.Version); err != nil {
		return false, fmt.Errorf("failed to remove old memberships: %w", err)
	}

	// Update segment status to "active"
	updated, err := p.segmentRepo.UpdateSegmentStatus(ctx, task.WorkspaceID, state.SegmentID, state.Version, string(domain.SegmentStatusActive))
	if err != nil {
		return false, fmt.Errorf("failed to update segment status to active: %w", err)
	}
	if !updated {
		p.logger.WithField("segment_id", state.SegmentID).Info("Segment changed during its build, leaving it to the newer build")
		return true, nil
	}

	// If segment has recompute_after set, reschedule it for next 5AM
	p.rescheduleRecompute(ctx, task.WorkspaceID, segment)

	p.logger.WithFields(map[string]interface{}{
		"segment_id":     state.SegmentID,
		"version":        state.Version,
		"total_contacts": state.TotalContacts,
		"matched_count":  state.MatchedCount,
	}).Info("Segment build completed")

	return true, nil
}

// nextBatch returns the next contacts to evaluate, after the last email of the last batch. A
// state saved before the last email was recorded resumes once by offset.
func (p *SegmentBuildProcessor) nextBatch(ctx context.Context, workspaceID string, state *domain.BuildSegmentState) ([]string, error) {
	if state.LastEmail == "" && state.ContactOffset > 0 {
		return p.contactRepo.GetBatchForSegment(ctx, workspaceID, state.ContactOffset, p.batchSize)
	}
	return p.contactRepo.GetBatchForSegmentAfter(ctx, workspaceID, state.LastEmail, p.batchSize)
}

// rescheduleRecompute moves the next daily rebuild of a segment with relative dates to the next
// 5 AM in its timezone. Failures are logged: the segment is then rebuilt at the next check.
func (p *SegmentBuildProcessor) rescheduleRecompute(ctx context.Context, workspaceID string, segment *domain.Segment) {
	if segment.RecomputeAfter == nil {
		return
	}

	next5AM, err := calculateNext5AMInTimezone(segment.Timezone)
	if err != nil {
		p.logger.WithFields(map[string]interface{}{
			"error":      err.Error(),
			"segment_id": segment.ID,
			"timezone":   segment.Timezone,
		}).Warn("Failed to calculate next 5AM for recompute rescheduling (non-fatal)")
		return
	}

	if err := p.segmentRepo.UpdateRecomputeAfter(ctx, workspaceID, segment.ID, &next5AM); err != nil {
		p.logger.WithFields(map[string]interface{}{
			"error":      err.Error(),
			"segment_id": segment.ID,
		}).Warn("Failed to update recompute_after for segment (non-fatal)")
		return
	}

	p.logger.WithFields(map[string]interface{}{
		"segment_id":      segment.ID,
		"recompute_after": next5AM.Format(time.RFC3339),
	}).Info("Rescheduled segment for next daily recomputation")
}

// releaseAfterFinalFailure runs when a build fails with no retry left. Left building, the segment
// would be skipped by the contact queue and by the daily rebuild for good, so it goes back to
// active (with the members written so far) and, for a segment with relative dates, its next
// rebuild moves to the next day rather than to every scheduler tick.
func (p *SegmentBuildProcessor) releaseAfterFinalFailure(ctx context.Context, task *domain.Task, state *domain.BuildSegmentState, segment *domain.Segment) {
	updated, err := p.segmentRepo.UpdateSegmentStatus(ctx, task.WorkspaceID, state.SegmentID, state.Version, string(domain.SegmentStatusActive))
	if err != nil {
		p.logger.WithFields(map[string]interface{}{
			"error":      err.Error(),
			"segment_id": state.SegmentID,
		}).Error("Failed to reactivate segment after its build failed for good")
		return
	}
	if !updated {
		return
	}

	p.logger.WithField("segment_id", state.SegmentID).Warn("Segment build failed with no retry left, segment set back to active")
	p.rescheduleRecompute(ctx, task.WorkspaceID, segment)
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

	// Matched emails, once each, in the order the query returned them
	seen := make(map[string]bool)
	matchedEmails := make([]string, 0)
	for rows.Next() {
		var email string
		if err := rows.Scan(&email); err != nil {
			return fmt.Errorf("failed to scan email: %w", err)
		}
		if !seen[email] {
			seen[email] = true
			matchedEmails = append(matchedEmails, email)
		}
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("error iterating rows: %w", err)
	}

	// Add matched contacts to segment. A failed write fails the batch: skipping the contact
	// would leave it out of the segment, and RemoveOldMemberships would then also drop the row
	// it had at the previous version.
	for _, email := range matchedEmails {
		if err := p.segmentRepo.AddContactToSegment(ctx, workspaceID, email, state.SegmentID, state.Version); err != nil {
			return fmt.Errorf("failed to add contact to segment: %w", err)
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
