package service

import (
	"context"
	"fmt"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/logger"
)

// dataRetentionBackfillBatch is the number of contacts read back per statement.
const dataRetentionBackfillBatch = 1000

// dataRetentionBackfillTimeoutMargin: stop taking a new batch when the runtime budget is
// nearly spent. It is wide, so that a slow batch on a large message history still ends, and
// its state is saved, inside the budget: a run cut off is marked failed and spends a retry.
const dataRetentionBackfillTimeoutMargin = 30 * time.Second

// DataRetentionBackfillProcessor reads the engagement of a workspace's existing contacts back
// from their history, once, after the upgrade to 45 (one task per workspace, queued by the v45
// system migration), then marks the workspace ready. It walks the contacts in email order,
// a batch per step, and resumes from the last email after a restart or a timeout.
type DataRetentionBackfillProcessor struct {
	engagement domain.ContactEngagementRepository
	taskRepo   domain.TaskRepository
	logger     logger.Logger
}

// NewDataRetentionBackfillProcessor creates the processor.
func NewDataRetentionBackfillProcessor(engagement domain.ContactEngagementRepository, taskRepo domain.TaskRepository, log logger.Logger) *DataRetentionBackfillProcessor {
	return &DataRetentionBackfillProcessor{engagement: engagement, taskRepo: taskRepo, logger: log}
}

// CanProcess implements domain.TaskProcessor.
func (p *DataRetentionBackfillProcessor) CanProcess(taskType string) bool {
	return taskType == domain.DataRetentionBackfillTaskType
}

// Process implements domain.TaskProcessor. Internal work is bounded by timeoutAt through the
// loop margin, never by a context of its own, so a context.Canceled always means "cut short
// from outside".
func (p *DataRetentionBackfillProcessor) Process(ctx context.Context, task *domain.Task, timeoutAt time.Time) (bool, error) {
	if task.State == nil {
		task.State = &domain.TaskState{}
	}
	if task.State.DataRetentionBackfill == nil {
		task.State.DataRetentionBackfill = &domain.DataRetentionBackfillState{}
	}
	state := task.State.DataRetentionBackfill

	for {
		if time.Until(timeoutAt) < dataRetentionBackfillTimeoutMargin {
			p.saveProgress(ctx, task, state)
			return false, nil // re-queued; resumes after the saved email
		}
		last, contacts, engaged, err := p.engagement.BackfillBatch(ctx, task.WorkspaceID, state.AfterEmail, dataRetentionBackfillBatch)
		if err != nil {
			p.saveProgress(ctx, task, state)
			return false, fmt.Errorf("failed to backfill contact engagement: %w", err)
		}
		if contacts == 0 {
			break
		}
		state.AfterEmail = last
		state.Contacts += contacts
		state.Engaged += engaged
		p.saveProgress(ctx, task, state)
	}

	if err := p.engagement.CompleteBackfill(ctx, task.WorkspaceID); err != nil {
		return false, err
	}
	task.Progress = 100
	task.State.Message = fmt.Sprintf("Engagement read back for %d contacts (%d with an engagement date)", state.Contacts, state.Engaged)
	return true, nil
}

func (p *DataRetentionBackfillProcessor) saveProgress(ctx context.Context, task *domain.Task, state *domain.DataRetentionBackfillState) {
	task.State.Message = fmt.Sprintf("Reading engagement back: %d contacts so far", state.Contacts)
	if err := p.taskRepo.SaveState(ctx, task.WorkspaceID, task.ID, task.Progress, task.State); err != nil {
		p.logger.WithField("workspace_id", task.WorkspaceID).WithField("task_id", task.ID).
			WithField("error", err.Error()).Error("Failed to save the engagement backfill state")
	}
}
