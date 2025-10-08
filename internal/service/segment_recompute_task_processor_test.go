package service

import (
	"context"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestSegmentRecomputeTaskProcessor_CanProcess(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockSegmentRepo := mocks.NewMockSegmentRepository(ctrl)
	mockTaskRepo := mocks.NewMockTaskRepository(ctrl)
	mockTaskService := mocks.NewMockTaskService(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	processor := NewSegmentRecomputeTaskProcessor(
		mockSegmentRepo,
		mockTaskRepo,
		mockTaskService,
		mockLogger,
	)

	assert.True(t, processor.CanProcess("check_segment_recompute"))
	assert.False(t, processor.CanProcess("build_segment"))
	assert.False(t, processor.CanProcess("other_task"))
}

func TestSegmentRecomputeTaskProcessor_Process(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockSegmentRepo := mocks.NewMockSegmentRepository(ctrl)
	mockTaskRepo := mocks.NewMockTaskRepository(ctrl)
	mockTaskService := mocks.NewMockTaskService(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	// Setup logger expectations for all tests
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()

	processor := NewSegmentRecomputeTaskProcessor(
		mockSegmentRepo,
		mockTaskRepo,
		mockTaskService,
		mockLogger,
	)

	t.Run("with segments due for recompute", func(t *testing.T) {
		ctx := context.Background()
		now := time.Now().UTC()
		timeoutAt := now.Add(1 * time.Minute)

		task := &domain.Task{
			ID:          "task1",
			WorkspaceID: "workspace1",
			Type:        "check_segment_recompute",
			Status:      domain.TaskStatusPending,
		}

		// Mock segments due for recompute
		segments := []*domain.Segment{
			{
				ID:      "segment1",
				Name:    "Test Segment 1",
				Version: 1,
				Tree:    &domain.TreeNode{},
			},
			{
				ID:      "segment2",
				Name:    "Test Segment 2",
				Version: 1,
				Tree:    &domain.TreeNode{},
			},
		}

		mockSegmentRepo.EXPECT().
			GetSegmentsDueForRecompute(ctx, "workspace1", 100).
			Return(segments, nil)

		// Expect build tasks to be created for each segment
		mockTaskService.EXPECT().
			CreateTask(ctx, "workspace1", gomock.Any()).
			Do(func(ctx context.Context, workspace string, task *domain.Task) {
				assert.Equal(t, "build_segment", task.Type)
				assert.Equal(t, domain.TaskStatusPending, task.Status)
				assert.NotNil(t, task.State)
				assert.NotNil(t, task.State.BuildSegment)
			}).
			Return(nil).
			Times(2) // Two segments

		completed, err := processor.Process(ctx, task, timeoutAt)
		assert.NoError(t, err)
		assert.False(t, completed) // Task should remain recurring
		assert.Equal(t, float64(0), task.Progress)
	})

	t.Run("with no segments due", func(t *testing.T) {
		ctx := context.Background()
		now := time.Now().UTC()
		timeoutAt := now.Add(1 * time.Minute)

		task := &domain.Task{
			ID:          "task1",
			WorkspaceID: "workspace1",
			Type:        "check_segment_recompute",
			Status:      domain.TaskStatusPending,
		}

		mockSegmentRepo.EXPECT().
			GetSegmentsDueForRecompute(ctx, "workspace1", 100).
			Return([]*domain.Segment{}, nil)

		completed, err := processor.Process(ctx, task, timeoutAt)
		assert.NoError(t, err)
		assert.False(t, completed) // Task should remain recurring
		assert.Equal(t, float64(0), task.Progress)
	})

	t.Run("with repository error", func(t *testing.T) {
		ctx := context.Background()
		now := time.Now().UTC()
		timeoutAt := now.Add(1 * time.Minute)

		task := &domain.Task{
			ID:          "task1",
			WorkspaceID: "workspace1",
			Type:        "check_segment_recompute",
			Status:      domain.TaskStatusPending,
		}

		mockSegmentRepo.EXPECT().
			GetSegmentsDueForRecompute(ctx, "workspace1", 100).
			Return(nil, assert.AnError)

		completed, err := processor.Process(ctx, task, timeoutAt)
		assert.NoError(t, err) // Errors don't fail the task
		assert.False(t, completed)
	})

	t.Run("with task creation error", func(t *testing.T) {
		ctx := context.Background()
		now := time.Now().UTC()
		timeoutAt := now.Add(1 * time.Minute)

		task := &domain.Task{
			ID:          "task1",
			WorkspaceID: "workspace1",
			Type:        "check_segment_recompute",
			Status:      domain.TaskStatusPending,
		}

		segments := []*domain.Segment{
			{
				ID:      "segment1",
				Name:    "Test Segment 1",
				Version: 1,
				Tree:    &domain.TreeNode{},
			},
		}

		mockSegmentRepo.EXPECT().
			GetSegmentsDueForRecompute(ctx, "workspace1", 100).
			Return(segments, nil)

		mockTaskService.EXPECT().
			CreateTask(ctx, "workspace1", gomock.Any()).
			Return(assert.AnError)

		completed, err := processor.Process(ctx, task, timeoutAt)
		assert.NoError(t, err) // Task creation errors don't fail the recurring task
		assert.False(t, completed)
	})
}

func TestEnsureSegmentRecomputeTask(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTaskRepo := mocks.NewMockTaskRepository(ctrl)
	ctx := context.Background()

	t.Run("creates new task when none exists", func(t *testing.T) {
		mockTaskRepo.EXPECT().
			List(ctx, "workspace1", gomock.Any()).
			Do(func(ctx context.Context, workspace string, filter domain.TaskFilter) {
				assert.Contains(t, filter.Type, "check_segment_recompute")
				assert.Equal(t, 1, filter.Limit)
			}).
			Return([]*domain.Task{}, 0, nil)

		mockTaskRepo.EXPECT().
			Create(ctx, "workspace1", gomock.Any()).
			Do(func(ctx context.Context, workspace string, task *domain.Task) {
				assert.Equal(t, "check_segment_recompute", task.Type)
				assert.Equal(t, domain.TaskStatusPending, task.Status)
				assert.NotNil(t, task.NextRunAfter)
			}).
			Return(nil)

		err := EnsureSegmentRecomputeTask(ctx, mockTaskRepo, "workspace1")
		assert.NoError(t, err)
	})

	t.Run("updates existing pending task", func(t *testing.T) {
		existingTask := &domain.Task{
			ID:           "existing-task",
			WorkspaceID:  "workspace1",
			Type:         "check_segment_recompute",
			Status:       domain.TaskStatusPending,
			NextRunAfter: nil,
		}

		mockTaskRepo.EXPECT().
			List(ctx, "workspace1", gomock.Any()).
			Return([]*domain.Task{existingTask}, 1, nil)

		mockTaskRepo.EXPECT().
			Update(ctx, "workspace1", gomock.Any()).
			Do(func(ctx context.Context, workspace string, task *domain.Task) {
				assert.NotNil(t, task.NextRunAfter)
			}).
			Return(nil)

		err := EnsureSegmentRecomputeTask(ctx, mockTaskRepo, "workspace1")
		assert.NoError(t, err)
	})

	t.Run("doesn't update if task already configured", func(t *testing.T) {
		now := time.Now().UTC()
		existingTask := &domain.Task{
			ID:           "existing-task",
			WorkspaceID:  "workspace1",
			Type:         "check_segment_recompute",
			Status:       domain.TaskStatusPending,
			NextRunAfter: &now,
		}

		mockTaskRepo.EXPECT().
			List(ctx, "workspace1", gomock.Any()).
			Return([]*domain.Task{existingTask}, 1, nil)

		// No Update call expected

		err := EnsureSegmentRecomputeTask(ctx, mockTaskRepo, "workspace1")
		assert.NoError(t, err)
	})
}
