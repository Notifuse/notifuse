package service

import (
	"context"
	"fmt"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/logger"
)

// ContactQueueTaskProcessor handles the execution of contact segment queue processing tasks
// This is a permanent, recurring task that runs for each workspace
type ContactQueueTaskProcessor struct {
	queueProcessor *ContactSegmentQueueProcessor
	taskRepo       domain.TaskRepository
	logger         logger.Logger
}

// NewContactQueueTaskProcessor creates a new contact queue task processor
func NewContactQueueTaskProcessor(
	queueProcessor *ContactSegmentQueueProcessor,
	taskRepo domain.TaskRepository,
	logger logger.Logger,
) *ContactQueueTaskProcessor {
	return &ContactQueueTaskProcessor{
		queueProcessor: queueProcessor,
		taskRepo:       taskRepo,
		logger:         logger,
	}
}

// CanProcess returns whether this processor can handle the given task type
func (p *ContactQueueTaskProcessor) CanProcess(taskType string) bool {
	return taskType == "process_contact_queue"
}

// Process executes the contact queue processing task
// This task is permanent and recurring - it always reschedules itself for the next run
func (p *ContactQueueTaskProcessor) Process(ctx context.Context, task *domain.Task, timeoutAt time.Time) (bool, error) {
	p.logger.WithFields(map[string]interface{}{
		"task_id":      task.ID,
		"workspace_id": task.WorkspaceID,
	}).Info("Processing contact segment queue")

	// Process the queue for this workspace
	if err := p.queueProcessor.ProcessQueue(ctx, task.WorkspaceID); err != nil {
		p.logger.WithFields(map[string]interface{}{
			"task_id":      task.ID,
			"workspace_id": task.WorkspaceID,
			"error":        err.Error(),
		}).Error("Failed to process contact segment queue")
		// Don't fail the task - just log the error and continue
		// The task will retry on the next cron run
	}

	// Get queue size for monitoring
	queueSize, err := p.queueProcessor.GetQueueSize(ctx, task.WorkspaceID)
	if err != nil {
		p.logger.WithFields(map[string]interface{}{
			"task_id":      task.ID,
			"workspace_id": task.WorkspaceID,
			"error":        err.Error(),
		}).Warn("Failed to get queue size")
	} else {
		p.logger.WithFields(map[string]interface{}{
			"task_id":      task.ID,
			"workspace_id": task.WorkspaceID,
			"queue_size":   queueSize,
		}).Debug("Queue processing completed")
	}

	// This is a permanent task - mark as completed so it can be rescheduled
	// The task will be picked up again on the next cron run because NextRunAfter is always set to "now"
	return true, nil
}

// EnsureQueueProcessingTask creates or updates the permanent queue processing task for a workspace
// This should be called when a workspace is created or during migration
func EnsureQueueProcessingTask(ctx context.Context, taskRepo domain.TaskRepository, workspaceID string) error {
	// Try to find existing task
	filter := domain.TaskFilter{
		Type:   []string{"process_contact_queue"},
		Limit:  1,
		Offset: 0,
	}

	tasks, _, err := taskRepo.List(ctx, workspaceID, filter)
	if err != nil {
		return fmt.Errorf("failed to check for existing queue processing task: %w", err)
	}

	// If task already exists, ensure it's configured correctly
	if len(tasks) > 0 {
		existingTask := tasks[0]
		needsUpdate := false

		// Ensure task is pending and scheduled to run now
		if existingTask.Status != domain.TaskStatusPending {
			existingTask.Status = domain.TaskStatusPending
			needsUpdate = true
		}

		now := time.Now()
		if existingTask.NextRunAfter == nil || existingTask.NextRunAfter.After(now) {
			existingTask.NextRunAfter = &now
			needsUpdate = true
		}

		if needsUpdate {
			if err := taskRepo.Update(ctx, workspaceID, existingTask); err != nil {
				return fmt.Errorf("failed to update queue processing task: %w", err)
			}
		}

		return nil
	}

	// Create new task
	now := time.Now()
	task := &domain.Task{
		WorkspaceID:   workspaceID,
		Type:          "process_contact_queue",
		Status:        domain.TaskStatusPending,
		NextRunAfter:  &now,
		MaxRuntime:    50, // 50 seconds (same as other tasks)
		MaxRetries:    3,
		RetryInterval: 60, // 1 minute
		Progress:      0,
		State: &domain.TaskState{
			Message: "Queue processing task",
		},
	}

	if err := taskRepo.Create(ctx, workspaceID, task); err != nil {
		return fmt.Errorf("failed to create queue processing task: %w", err)
	}

	return nil
}
