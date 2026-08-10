package service

import (
	"context"
	"fmt"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/logger"
)

// webBackfillTimeoutMargin: stop picking up new partitions when the runtime
// budget is nearly spent, so the state save happens comfortably inside it.
const webBackfillTimeoutMargin = 5 * time.Second

// WebAnalyticsBackfillProcessor rewrites the attribution dimensions of
// historical web_sessions and web_goals rows after the workspace's rules
// changed, one monthly partition per step, resumable across runs.
type WebAnalyticsBackfillProcessor struct {
	workspaceRepo    domain.WorkspaceRepository
	webAnalyticsRepo domain.WebAnalyticsRepository
	taskRepo         domain.TaskRepository
	logger           logger.Logger
}

// NewWebAnalyticsBackfillProcessor creates the processor.
func NewWebAnalyticsBackfillProcessor(
	workspaceRepo domain.WorkspaceRepository,
	webAnalyticsRepo domain.WebAnalyticsRepository,
	taskRepo domain.TaskRepository,
	log logger.Logger,
) *WebAnalyticsBackfillProcessor {
	return &WebAnalyticsBackfillProcessor{
		workspaceRepo:    workspaceRepo,
		webAnalyticsRepo: webAnalyticsRepo,
		taskRepo:         taskRepo,
		logger:           log,
	}
}

// CanProcess implements domain.TaskProcessor.
func (p *WebAnalyticsBackfillProcessor) CanProcess(taskType string) bool {
	return taskType == domain.WebAnalyticsBackfillTaskType
}

// Process implements domain.TaskProcessor. Cancellation contract: internal
// work is bounded by timeoutAt via the loop margin, never by a self-created
// cancellable context, so a surfaced context.Canceled always means "cut short
// from outside" and preserves the retry budget.
func (p *WebAnalyticsBackfillProcessor) Process(ctx context.Context, task *domain.Task, timeoutAt time.Time) (bool, error) {
	workspace, err := p.workspaceRepo.GetByID(ctx, task.WorkspaceID)
	if err != nil {
		return false, fmt.Errorf("failed to load workspace: %w", err)
	}
	if workspace == nil || workspace.Settings.WebAnalytics == nil {
		// Nothing to backfill; the feature was removed since the task was queued.
		return true, nil
	}
	filters := workspace.Settings.WebAnalytics.Filters
	currentVersion := domain.ComputeWebFiltersVersion(filters)

	if task.State == nil {
		task.State = &domain.TaskState{}
	}
	state := task.State.WebAnalyticsBackfill

	// First run — or the rules changed under a resumable run: restart against
	// the current rule set so every partition ends up consistent.
	if state == nil || state.FiltersVersion != currentVersion {
		sessionPartitions, err := p.webAnalyticsRepo.ListPartitions(ctx, task.WorkspaceID, "web_sessions")
		if err != nil {
			return false, fmt.Errorf("failed to list web_sessions partitions: %w", err)
		}
		goalPartitions, err := p.webAnalyticsRepo.ListPartitions(ctx, task.WorkspaceID, "web_goals")
		if err != nil {
			return false, fmt.Errorf("failed to list web_goals partitions: %w", err)
		}
		state = &domain.WebAnalyticsBackfillState{
			FiltersVersion: currentVersion,
			Partitions:     append(sessionPartitions, goalPartitions...),
		}
		task.State.WebAnalyticsBackfill = state
	}

	total := len(state.Partitions)
	for state.PartitionIndex < total {
		if time.Until(timeoutAt) < webBackfillTimeoutMargin {
			p.saveProgress(ctx, task, state, total)
			return false, nil // re-queued; resumes at the saved index
		}

		partition := state.Partitions[state.PartitionIndex]
		rows, err := p.webAnalyticsRepo.BackfillPartition(ctx, task.WorkspaceID, partition, filters)
		if err != nil {
			p.saveProgress(ctx, task, state, total)
			return false, fmt.Errorf("failed to backfill partition %s: %w", partition, err)
		}
		state.RowsUpdated += rows
		state.PartitionIndex++
		p.saveProgress(ctx, task, state, total)
	}

	task.Progress = 100
	task.State.Message = fmt.Sprintf("Attribution backfill complete: %d rows across %d partitions", state.RowsUpdated, total)
	return true, nil
}

func (p *WebAnalyticsBackfillProcessor) saveProgress(ctx context.Context, task *domain.Task, state *domain.WebAnalyticsBackfillState, total int) {
	if total > 0 {
		task.Progress = float64(state.PartitionIndex) * 100 / float64(total)
	}
	task.State.Message = fmt.Sprintf("Backfilling partition %d/%d (%d rows rewritten)", state.PartitionIndex, total, state.RowsUpdated)
	if err := p.taskRepo.SaveState(ctx, task.WorkspaceID, task.ID, task.Progress, task.State); err != nil {
		p.logger.WithField("workspace_id", task.WorkspaceID).WithField("task_id", task.ID).
			WithField("error", err.Error()).Error("Failed to save backfill state")
	}
}
