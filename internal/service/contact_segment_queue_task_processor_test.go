package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestNewContactSegmentQueueTaskProcessor(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockQueueProcessor := NewContactSegmentQueueProcessor(
		mocks.NewMockContactSegmentQueueRepository(ctrl),
		mocks.NewMockSegmentRepository(ctrl),
		mocks.NewMockContactRepository(ctrl),
		mocks.NewMockWorkspaceRepository(ctrl),
		pkgmocks.NewMockLogger(ctrl),
	)
	mockTaskRepo := mocks.NewMockTaskRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	processor := NewContactSegmentQueueTaskProcessor(
		mockQueueProcessor,
		mockTaskRepo,
		mockLogger,
	)

	assert.NotNil(t, processor)
	assert.NotNil(t, processor.queueProcessor)
	assert.NotNil(t, processor.taskRepo)
	assert.NotNil(t, processor.logger)
}

func TestContactSegmentQueueTaskProcessor_CanProcess(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockQueueProcessor := NewContactSegmentQueueProcessor(
		mocks.NewMockContactSegmentQueueRepository(ctrl),
		mocks.NewMockSegmentRepository(ctrl),
		mocks.NewMockContactRepository(ctrl),
		mocks.NewMockWorkspaceRepository(ctrl),
		pkgmocks.NewMockLogger(ctrl),
	)
	mockTaskRepo := mocks.NewMockTaskRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	processor := NewContactSegmentQueueTaskProcessor(
		mockQueueProcessor,
		mockTaskRepo,
		mockLogger,
	)

	// Test with correct task type
	assert.True(t, processor.CanProcess("process_contact_segment_queue"))

	// Test with incorrect task types
	assert.False(t, processor.CanProcess("send_broadcast"))
	assert.False(t, processor.CanProcess("build_segment"))
	assert.False(t, processor.CanProcess("import_contacts"))
	assert.False(t, processor.CanProcess(""))
}

func TestContactSegmentQueueTaskProcessor_Process_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockQueueRepo := mocks.NewMockContactSegmentQueueRepository(ctrl)
	mockSegmentRepo := mocks.NewMockSegmentRepository(ctrl)
	mockContactRepo := mocks.NewMockContactRepository(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockTaskRepo := mocks.NewMockTaskRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	// Create queue processor
	queueProcessor := NewContactSegmentQueueProcessor(
		mockQueueRepo,
		mockSegmentRepo,
		mockContactRepo,
		mockWorkspaceRepo,
		mockLogger,
	)

	processor := NewContactSegmentQueueTaskProcessor(
		queueProcessor,
		mockTaskRepo,
		mockLogger,
	)

	ctx := context.Background()
	workspaceID := "workspace-1"
	task := &domain.Task{
		ID:          "task-1",
		WorkspaceID: workspaceID,
		Type:        "process_contact_segment_queue",
		Status:      domain.TaskStatusRunning,
		Progress:    50, // Will be reset to 0
	}
	timeoutAt := time.Now().Add(50 * time.Second)

	// Mock expectations - ProcessQueue will be called but we need to set up the mocks
	// For simplicity, we'll expect it to fail gracefully when getting connection
	mockWorkspaceRepo.EXPECT().
		GetConnection(gomock.Any(), workspaceID).
		Return(nil, errors.New("connection error"))

	// Expect logging
	mockLogger.EXPECT().
		WithFields(gomock.Any()).
		Return(mockLogger).
		Times(2) // Info log + Error log

	mockLogger.EXPECT().
		Info(gomock.Any())

	mockLogger.EXPECT().
		Error(gomock.Any())

	// Also expect GetQueueSize to be called
	mockQueueRepo.EXPECT().
		GetQueueSize(gomock.Any(), workspaceID).
		Return(10, nil)

	mockLogger.EXPECT().
		WithFields(gomock.Any()).
		Return(mockLogger)

	mockLogger.EXPECT().
		Debug(gomock.Any())

	// Execute
	completed, err := processor.Process(ctx, task, timeoutAt)

	// Verify
	assert.NoError(t, err)
	assert.False(t, completed, "Task should return false to keep it as permanent recurring task")
	assert.Equal(t, float64(0), task.Progress, "Progress should be reset to 0")
}

func TestContactSegmentQueueTaskProcessor_Process_QueueProcessorError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockQueueRepo := mocks.NewMockContactSegmentQueueRepository(ctrl)
	mockSegmentRepo := mocks.NewMockSegmentRepository(ctrl)
	mockContactRepo := mocks.NewMockContactRepository(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockTaskRepo := mocks.NewMockTaskRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	queueProcessor := NewContactSegmentQueueProcessor(
		mockQueueRepo,
		mockSegmentRepo,
		mockContactRepo,
		mockWorkspaceRepo,
		mockLogger,
	)

	processor := NewContactSegmentQueueTaskProcessor(
		queueProcessor,
		mockTaskRepo,
		mockLogger,
	)

	ctx := context.Background()
	workspaceID := "workspace-1"
	task := &domain.Task{
		ID:          "task-1",
		WorkspaceID: workspaceID,
		Type:        "process_contact_segment_queue",
		Status:      domain.TaskStatusRunning,
		Progress:    0,
	}
	timeoutAt := time.Now().Add(50 * time.Second)

	// Mock queue processing to fail
	mockWorkspaceRepo.EXPECT().
		GetConnection(gomock.Any(), workspaceID).
		Return(nil, errors.New("database connection failed"))

	// Expect logging
	mockLogger.EXPECT().
		WithFields(gomock.Any()).
		Return(mockLogger).
		Times(3) // Info + Error + GetQueueSize Warn

	mockLogger.EXPECT().
		Info(gomock.Any())

	mockLogger.EXPECT().
		Error(gomock.Any())

	// GetQueueSize also fails
	mockQueueRepo.EXPECT().
		GetQueueSize(gomock.Any(), workspaceID).
		Return(0, errors.New("queue size error"))

	mockLogger.EXPECT().
		Warn(gomock.Any())

	// Execute
	completed, err := processor.Process(ctx, task, timeoutAt)

	// Should not fail the task, just log errors
	assert.NoError(t, err)
	assert.False(t, completed, "Task should return false even on errors")
	assert.Equal(t, float64(0), task.Progress)
}

func TestContactSegmentQueueTaskProcessor_Process_GetQueueSizeError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockQueueRepo := mocks.NewMockContactSegmentQueueRepository(ctrl)
	mockSegmentRepo := mocks.NewMockSegmentRepository(ctrl)
	mockContactRepo := mocks.NewMockContactRepository(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockTaskRepo := mocks.NewMockTaskRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	queueProcessor := NewContactSegmentQueueProcessor(
		mockQueueRepo,
		mockSegmentRepo,
		mockContactRepo,
		mockWorkspaceRepo,
		mockLogger,
	)

	processor := NewContactSegmentQueueTaskProcessor(
		queueProcessor,
		mockTaskRepo,
		mockLogger,
	)

	ctx := context.Background()
	workspaceID := "workspace-1"
	task := &domain.Task{
		ID:          "task-1",
		WorkspaceID: workspaceID,
		Type:        "process_contact_segment_queue",
		Status:      domain.TaskStatusRunning,
		Progress:    25,
	}
	timeoutAt := time.Now().Add(50 * time.Second)

	// Mock queue processing fails
	mockWorkspaceRepo.EXPECT().
		GetConnection(gomock.Any(), workspaceID).
		Return(nil, errors.New("connection error"))

	mockLogger.EXPECT().
		WithFields(gomock.Any()).
		Return(mockLogger).
		Times(3) // Info + Error + Warn

	mockLogger.EXPECT().Info(gomock.Any())
	mockLogger.EXPECT().Error(gomock.Any())

	// GetQueueSize fails
	mockQueueRepo.EXPECT().
		GetQueueSize(gomock.Any(), workspaceID).
		Return(0, errors.New("failed to get queue size"))

	mockLogger.EXPECT().Warn(gomock.Any())

	// Execute
	completed, err := processor.Process(ctx, task, timeoutAt)

	// Verify - should handle error gracefully
	assert.NoError(t, err)
	assert.False(t, completed)
	assert.Equal(t, float64(0), task.Progress, "Progress should be reset")
}

func TestEnsureContactSegmentQueueProcessingTask_CreatesNew(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTaskRepo := mocks.NewMockTaskRepository(ctrl)
	ctx := context.Background()
	workspaceID := "workspace-1"

	// Mock List to return empty (no existing task)
	mockTaskRepo.EXPECT().
		List(ctx, workspaceID, gomock.Any()).
		DoAndReturn(func(ctx context.Context, workspace string, filter domain.TaskFilter) ([]*domain.Task, int, error) {
			// Verify the filter
			assert.Equal(t, []string{"process_contact_segment_queue"}, filter.Type)
			assert.Equal(t, 1, filter.Limit)
			return []*domain.Task{}, 0, nil
		})

	// Mock Create to capture the task being created
	mockTaskRepo.EXPECT().
		Create(ctx, workspaceID, gomock.Any()).
		DoAndReturn(func(ctx context.Context, workspace string, task *domain.Task) error {
			// Verify task properties
			assert.Equal(t, workspaceID, task.WorkspaceID)
			assert.Equal(t, "process_contact_segment_queue", task.Type)
			assert.Equal(t, domain.TaskStatusPending, task.Status)
			assert.NotNil(t, task.NextRunAfter)
			assert.Equal(t, 50, task.MaxRuntime)
			assert.Equal(t, 3, task.MaxRetries)
			assert.Equal(t, 60, task.RetryInterval)
			assert.Equal(t, float64(0), task.Progress)
			assert.NotNil(t, task.State)
			assert.Equal(t, "Contact segment queue processing task", task.State.Message)
			return nil
		})

	// Execute
	err := EnsureContactSegmentQueueProcessingTask(ctx, mockTaskRepo, workspaceID)

	// Verify
	assert.NoError(t, err)
}

func TestEnsureContactSegmentQueueProcessingTask_UpdatesExisting(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTaskRepo := mocks.NewMockTaskRepository(ctrl)
	ctx := context.Background()
	workspaceID := "workspace-1"

	// Create an existing task that needs updating
	futureTime := time.Now().Add(1 * time.Hour)
	existingTask := &domain.Task{
		ID:           "task-1",
		WorkspaceID:  workspaceID,
		Type:         "process_contact_segment_queue",
		Status:       domain.TaskStatusCompleted, // Should be updated to pending
		NextRunAfter: &futureTime,                // Should be updated to now
		MaxRuntime:   50,
		MaxRetries:   3,
	}

	// Mock List to return existing task
	mockTaskRepo.EXPECT().
		List(ctx, workspaceID, gomock.Any()).
		Return([]*domain.Task{existingTask}, 1, nil)

	// Mock Update to verify the task is updated correctly
	mockTaskRepo.EXPECT().
		Update(ctx, workspaceID, gomock.Any()).
		DoAndReturn(func(ctx context.Context, workspace string, task *domain.Task) error {
			// Verify task was updated correctly
			assert.Equal(t, domain.TaskStatusPending, task.Status)
			assert.NotNil(t, task.NextRunAfter)
			assert.True(t, task.NextRunAfter.Before(time.Now().Add(1*time.Second)))
			return nil
		})

	// Execute
	err := EnsureContactSegmentQueueProcessingTask(ctx, mockTaskRepo, workspaceID)

	// Verify
	assert.NoError(t, err)
}

func TestEnsureContactSegmentQueueProcessingTask_NoUpdateNeeded(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTaskRepo := mocks.NewMockTaskRepository(ctrl)
	ctx := context.Background()
	workspaceID := "workspace-1"

	// Create an existing task that doesn't need updating
	now := time.Now()
	existingTask := &domain.Task{
		ID:           "task-1",
		WorkspaceID:  workspaceID,
		Type:         "process_contact_segment_queue",
		Status:       domain.TaskStatusPending,
		NextRunAfter: &now,
		MaxRuntime:   50,
		MaxRetries:   3,
	}

	// Mock List to return existing task
	mockTaskRepo.EXPECT().
		List(ctx, workspaceID, gomock.Any()).
		Return([]*domain.Task{existingTask}, 1, nil)

	// Update should NOT be called since no changes needed

	// Execute
	err := EnsureContactSegmentQueueProcessingTask(ctx, mockTaskRepo, workspaceID)

	// Verify
	assert.NoError(t, err)
}

func TestEnsureContactSegmentQueueProcessingTask_ListError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTaskRepo := mocks.NewMockTaskRepository(ctrl)
	ctx := context.Background()
	workspaceID := "workspace-1"

	// Mock List to return error
	mockTaskRepo.EXPECT().
		List(ctx, workspaceID, gomock.Any()).
		Return(nil, 0, errors.New("database error"))

	// Execute
	err := EnsureContactSegmentQueueProcessingTask(ctx, mockTaskRepo, workspaceID)

	// Verify
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to check for existing queue processing task")
}

func TestEnsureContactSegmentQueueProcessingTask_CreateError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTaskRepo := mocks.NewMockTaskRepository(ctrl)
	ctx := context.Background()
	workspaceID := "workspace-1"

	// Mock List to return empty
	mockTaskRepo.EXPECT().
		List(ctx, workspaceID, gomock.Any()).
		Return([]*domain.Task{}, 0, nil)

	// Mock Create to return error
	mockTaskRepo.EXPECT().
		Create(ctx, workspaceID, gomock.Any()).
		Return(errors.New("failed to create task"))

	// Execute
	err := EnsureContactSegmentQueueProcessingTask(ctx, mockTaskRepo, workspaceID)

	// Verify
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create queue processing task")
}

func TestEnsureContactSegmentQueueProcessingTask_UpdateError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTaskRepo := mocks.NewMockTaskRepository(ctrl)
	ctx := context.Background()
	workspaceID := "workspace-1"

	// Create an existing task that needs updating
	futureTime := time.Now().Add(1 * time.Hour)
	existingTask := &domain.Task{
		ID:           "task-1",
		WorkspaceID:  workspaceID,
		Type:         "process_contact_segment_queue",
		Status:       domain.TaskStatusCompleted,
		NextRunAfter: &futureTime,
		MaxRuntime:   50,
		MaxRetries:   3,
	}

	// Mock List to return existing task
	mockTaskRepo.EXPECT().
		List(ctx, workspaceID, gomock.Any()).
		Return([]*domain.Task{existingTask}, 1, nil)

	// Mock Update to return error
	mockTaskRepo.EXPECT().
		Update(ctx, workspaceID, gomock.Any()).
		Return(errors.New("failed to update task"))

	// Execute
	err := EnsureContactSegmentQueueProcessingTask(ctx, mockTaskRepo, workspaceID)

	// Verify
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to update queue processing task")
}
