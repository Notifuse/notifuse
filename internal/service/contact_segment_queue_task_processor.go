package service

import (
	"context"
	"fmt"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/logger"
)

// ContactSegmentQueueTaskProcessor handles the execution of contact segment queue processing tasks
// This is a permanent, recurring task that runs for each workspace
type ContactSegmentQueueTaskProcessor struct {
	queueProcessor *ContactSegmentQueueProcessor
	taskRepo       domain.TaskRepository
	logger         logger.Logger
}

// NewContactSegmentQueueTaskProcessor creates a new contact segment queue task processor
func NewContactSegmentQueueTaskProcessor(
	queueProcessor *ContactSegmentQueueProcessor,
	taskRepo domain.TaskRepository,
	logger logger.Logger,
) *ContactSegmentQueueTaskProcessor {
	return &ContactSegmentQueueTaskProcessor{
		queueProcessor: queueProcessor,
		taskRepo:       taskRepo,
		logger:         logger,
	}
}

// CanProcess returns whether this processor can handle the given task type
func (p *ContactSegmentQueueTaskProcessor) CanProcess(taskType string) bool {
	return taskType == "process_contact_segment_queue"
}

// Process executes the contact segment queue processing task
// This task is permanent and recurring - it always reschedules itself for the next run
// It processes batches continuously until the timeout is reached
func (p *ContactSegmentQueueTaskProcessor) Process(ctx context.Context, task *domain.Task, timeoutAt time.Time) (bool, error) {
	p.logger.WithFields(map[string]interface{}{
		"task_id":      task.ID,
		"workspace_id": task.WorkspaceID,
	}).Info("Processing contact segment queue")

	// Keep track of statistics
	totalProcessed := 0
	batchCount := 0

	// Process batches until we're close to the timeout
	// Leave 5 seconds buffer before timeout to ensure cleanup
	bufferDuration := 5 * time.Second

	for {
		// Check if we're approaching the timeout
		if time.Now().Add(bufferDuration).After(timeoutAt) {
			p.logger.WithFields(map[string]interface{}{
				"task_id":         task.ID,
				"workspace_id":    task.WorkspaceID,
				"batch_count":     batchCount,
				"total_processed": totalProcessed,
			}).Info("Approaching timeout, stopping queue processing")
			break
		}

		// Check if context is done
		if ctx.Err() != nil {
			p.logger.WithFields(map[string]interface{}{
				"task_id":      task.ID,
				"workspace_id": task.WorkspaceID,
				"error":        ctx.Err().Error(),
			}).Info("Context cancelled, stopping queue processing")
			break
		}

		// Process the queue for this workspace (one batch)
		processedCount, err := p.queueProcessor.ProcessQueue(ctx, task.WorkspaceID)
		if err != nil {
			p.logger.WithFields(map[string]interface{}{
				"task_id":      task.ID,
				"workspace_id": task.WorkspaceID,
				"error":        err.Error(),
			}).Error("Failed to process contact segment queue batch")
			// Don't fail the task - just log the error and continue to next batch
		}

		// Get queue size to check if we should continue
		queueSize, err := p.queueProcessor.GetQueueSize(ctx, task.WorkspaceID)
		if err != nil {
			p.logger.WithFields(map[string]interface{}{
				"task_id":      task.ID,
				"workspace_id": task.WorkspaceID,
				"error":        err.Error(),
			}).Warn("Failed to get queue size")
			// If we can't get queue size, stop processing to be safe
			break
		}

		batchCount++
		totalProcessed += processedCount

		// If queue is empty, check if we have time to wait and check again
		if queueSize == 0 {
			// Check if we have enough time to wait 10 seconds + buffer
			waitDuration := 10 * time.Second
			if time.Now().Add(waitDuration + bufferDuration).After(timeoutAt) {
				// Not enough time to wait, stop processing
				break
			}

			// We have time, wait 10 seconds and check again
			time.Sleep(waitDuration)
			continue
		}

		// Small delay between batches to avoid hammering the database
		time.Sleep(100 * time.Millisecond)
	}

	// This is a permanent recurring task - return false with progress to keep it as "pending"
	// This allows it to be picked up again on the next cron run
	// The task's NextRunAfter is set to NOW in the EnsureTask function, so it's immediately available
	task.Progress = 0 // Reset progress for next run
	return false, nil // false = task is not complete, will be marked as "pending" and re-run
}

// EnsureContactSegmentQueueProcessingTask creates or updates the permanent queue processing task for a workspace
// This should be called when a workspace is created or during migration
func EnsureContactSegmentQueueProcessingTask(ctx context.Context, taskRepo domain.TaskRepository, workspaceID string) error {
	// Try to find existing task
	filter := domain.TaskFilter{
		Type:   []string{"process_contact_segment_queue"},
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

		now := time.Now().UTC()
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
	now := time.Now().UTC()
	task := &domain.Task{
		WorkspaceID:   workspaceID,
		Type:          "process_contact_segment_queue",
		Status:        domain.TaskStatusPending,
		NextRunAfter:  &now,
		MaxRuntime:    50, // 50 seconds (same as other tasks)
		MaxRetries:    3,
		RetryInterval: 60, // 1 minute
		Progress:      0,
		State: &domain.TaskState{
			Message: "Contact segment queue processing task",
		},
	}

	if err := taskRepo.Create(ctx, workspaceID, task); err != nil {
		return fmt.Errorf("failed to create queue processing task: %w", err)
	}

	return nil
}
