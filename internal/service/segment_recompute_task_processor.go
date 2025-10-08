package service

import (
	"context"
	"fmt"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/google/uuid"
)

// SegmentRecomputeTaskProcessor handles the execution of segment recompute checking tasks
// This is a permanent, recurring task that runs for each workspace
type SegmentRecomputeTaskProcessor struct {
	segmentRepo domain.SegmentRepository
	taskRepo    domain.TaskRepository
	taskService domain.TaskService
	logger      logger.Logger
}

// NewSegmentRecomputeTaskProcessor creates a new segment recompute task processor
func NewSegmentRecomputeTaskProcessor(
	segmentRepo domain.SegmentRepository,
	taskRepo domain.TaskRepository,
	taskService domain.TaskService,
	logger logger.Logger,
) *SegmentRecomputeTaskProcessor {
	return &SegmentRecomputeTaskProcessor{
		segmentRepo: segmentRepo,
		taskRepo:    taskRepo,
		taskService: taskService,
		logger:      logger,
	}
}

// CanProcess returns whether this processor can handle the given task type
func (p *SegmentRecomputeTaskProcessor) CanProcess(taskType string) bool {
	return taskType == "check_segment_recompute"
}

// Process executes the segment recompute checking task
// This task is permanent and recurring - it always reschedules itself for the next run
// It checks for segments due for recomputation and creates build tasks for them
func (p *SegmentRecomputeTaskProcessor) Process(ctx context.Context, task *domain.Task, timeoutAt time.Time) (bool, error) {
	p.logger.WithFields(map[string]interface{}{
		"task_id":      task.ID,
		"workspace_id": task.WorkspaceID,
	}).Info("Checking for segments due for recomputation")

	// Get segments that need recomputation
	segments, err := p.segmentRepo.GetSegmentsDueForRecompute(ctx, task.WorkspaceID, 100)
	if err != nil {
		p.logger.WithFields(map[string]interface{}{
			"task_id":      task.ID,
			"workspace_id": task.WorkspaceID,
			"error":        err.Error(),
		}).Error("Failed to get segments due for recompute")
		// Don't fail the task - just log the error and continue to next run
		return false, nil
	}

	// Create build tasks for each segment
	tasksCreated := 0
	for _, segment := range segments {
		// Create a build_segment task
		buildTask := &domain.Task{
			ID:          uuid.New().String(),
			WorkspaceID: task.WorkspaceID,
			Type:        "build_segment",
			Status:      domain.TaskStatusPending,
			Progress:    0,
			State: &domain.TaskState{
				BuildSegment: &domain.BuildSegmentState{
					SegmentID: segment.ID,
					Version:   segment.Version,
					BatchSize: 100,
					StartedAt: time.Now().Format(time.RFC3339),
				},
			},
			MaxRuntime: 300, // 5 minutes
			MaxRetries: 3,
		}

		if err := p.taskService.CreateTask(ctx, task.WorkspaceID, buildTask); err != nil {
			p.logger.WithFields(map[string]interface{}{
				"error":      err.Error(),
				"segment_id": segment.ID,
			}).Warn("Failed to create build task for segment recompute (continuing)")
			continue
		}

		tasksCreated++
		p.logger.WithFields(map[string]interface{}{
			"segment_id": segment.ID,
			"task_id":    buildTask.ID,
		}).Info("Created build task for segment recompute")
	}

	p.logger.WithFields(map[string]interface{}{
		"task_id":       task.ID,
		"workspace_id":  task.WorkspaceID,
		"segments_due":  len(segments),
		"tasks_created": tasksCreated,
	}).Info("Completed segment recompute check")

	// This is a permanent recurring task - return false to keep it as "pending"
	// This allows it to be picked up again on the next cron run
	task.Progress = 0 // Reset progress for next run
	return false, nil // false = task is not complete, will be marked as "pending" and re-run
}

// EnsureSegmentRecomputeTask creates or updates the permanent recompute checking task for a workspace
// This should be called when a workspace is created or during migration
func EnsureSegmentRecomputeTask(ctx context.Context, taskRepo domain.TaskRepository, workspaceID string) error {
	// Try to find existing task
	filter := domain.TaskFilter{
		Type:   []string{"check_segment_recompute"},
		Limit:  1,
		Offset: 0,
	}

	tasks, _, err := taskRepo.List(ctx, workspaceID, filter)
	if err != nil {
		return fmt.Errorf("failed to check for existing segment recompute task: %w", err)
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
				return fmt.Errorf("failed to update segment recompute task: %w", err)
			}
		}

		return nil
	}

	// Create new task
	now := time.Now().UTC()
	task := &domain.Task{
		WorkspaceID:   workspaceID,
		Type:          "check_segment_recompute",
		Status:        domain.TaskStatusPending,
		NextRunAfter:  &now,
		MaxRuntime:    50, // 50 seconds (same as other tasks)
		MaxRetries:    3,
		RetryInterval: 60, // 1 minute
		Progress:      0,
		State: &domain.TaskState{
			Message: "Check segments for daily recompute",
		},
	}

	if err := taskRepo.Create(ctx, workspaceID, task); err != nil {
		return fmt.Errorf("failed to create segment recompute task: %w", err)
	}

	return nil
}
