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
		Return(nil, errors.New("connection error")).
		AnyTimes()

	// Expect logging for the initial info, error in ProcessQueue, and final info
	mockLogger.EXPECT().
		WithFields(gomock.Any()).
		Return(mockLogger).
		AnyTimes()

	mockLogger.EXPECT().
		Info(gomock.Any()).
		AnyTimes()

	mockLogger.EXPECT().
		Error(gomock.Any()).
		AnyTimes()

	mockLogger.EXPECT().
		Warn(gomock.Any()).
		AnyTimes()

	// GetQueueSize to be called - return error on second call to stop the loop after wait
	gomock.InOrder(
		mockQueueRepo.EXPECT().
			GetQueueSize(gomock.Any(), workspaceID).
			Return(0, nil), // First call: queue is empty
		mockQueueRepo.EXPECT().
			GetQueueSize(gomock.Any(), workspaceID).
			Return(0, errors.New("stop processing")), // Second call after wait: error to stop
	)

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

	// Expect logging - use AnyTimes for flexibility
	mockLogger.EXPECT().
		WithFields(gomock.Any()).
		Return(mockLogger).
		AnyTimes()

	mockLogger.EXPECT().
		Info(gomock.Any()).
		AnyTimes()

	mockLogger.EXPECT().
		Error(gomock.Any())

	// GetQueueSize also fails - this causes the loop to exit
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
		AnyTimes()

	mockLogger.EXPECT().
		Info(gomock.Any()).
		AnyTimes()

	mockLogger.EXPECT().
		Error(gomock.Any())

	// GetQueueSize fails - this causes the loop to exit
	mockQueueRepo.EXPECT().
		GetQueueSize(gomock.Any(), workspaceID).
		Return(0, errors.New("failed to get queue size"))

	mockLogger.EXPECT().
		Warn(gomock.Any())

	// Execute
	completed, err := processor.Process(ctx, task, timeoutAt)

	// Verify - should handle error gracefully
	assert.NoError(t, err)
	assert.False(t, completed)
	assert.Equal(t, float64(0), task.Progress, "Progress should be reset")
}
