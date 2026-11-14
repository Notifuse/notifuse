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
	"github.com/stretchr/testify/require"
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
				Version: 3,
				Tree:    &domain.TreeNode{},
			},
		}

		mockSegmentRepo.EXPECT().
			GetSegmentsDueForRecompute(ctx, "workspace1", 100).
			Return(segments, nil)

		// Each daily rebuild writes at a version of its own: the task is created at the next
		// version, then the segment is moved to it.
		for _, seg := range segments {
			seg := seg
			gomock.InOrder(
				mockTaskService.EXPECT().
					CreateTask(ctx, "workspace1", buildTaskFor(seg.ID)).
					Do(func(ctx context.Context, workspace string, task *domain.Task) {
						assert.Equal(t, "build_segment", task.Type)
						assert.Equal(t, domain.TaskStatusPending, task.Status)
						require.NotNil(t, task.State)
						require.NotNil(t, task.State.BuildSegment)
						assert.Equal(t, seg.Version+1, task.State.BuildSegment.Version)
					}).
					Return(nil),
				mockSegmentRepo.EXPECT().
					StartRecompute(ctx, "workspace1", seg.ID, seg.Version).
					Return(true, nil),
			)
		}

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
		// No task, no bump: the segment stays active and due, and the next check retries.

		completed, err := processor.Process(ctx, task, timeoutAt)
		assert.NoError(t, err) // Task creation errors don't fail the recurring task
		assert.False(t, completed)
	})

	t.Run("a failed task creation does not stop the next segment", func(t *testing.T) {
		ctx := context.Background()
		task := &domain.Task{ID: "task1", WorkspaceID: "workspace1", Type: "check_segment_recompute"}
		segments := []*domain.Segment{
			{ID: "segment1", Version: 1, Tree: &domain.TreeNode{}},
			{ID: "segment2", Version: 2, Tree: &domain.TreeNode{}},
		}
		mockSegmentRepo.EXPECT().GetSegmentsDueForRecompute(ctx, "workspace1", 100).Return(segments, nil)
		mockTaskService.EXPECT().CreateTask(ctx, "workspace1", buildTaskFor("segment1")).Return(assert.AnError)
		mockTaskService.EXPECT().CreateTask(ctx, "workspace1", buildTaskFor("segment2")).Return(nil)
		mockSegmentRepo.EXPECT().StartRecompute(ctx, "workspace1", "segment2", int64(2)).Return(true, nil)

		completed, err := processor.Process(ctx, task, time.Now().Add(time.Minute))
		assert.NoError(t, err)
		assert.False(t, completed)
	})

	t.Run("a bump that does not land is logged and the check goes on", func(t *testing.T) {
		ctx := context.Background()
		task := &domain.Task{ID: "task1", WorkspaceID: "workspace1", Type: "check_segment_recompute"}
		segments := []*domain.Segment{
			{ID: "segment1", Version: 1, Tree: &domain.TreeNode{}},
			{ID: "segment2", Version: 5, Tree: &domain.TreeNode{}},
		}
		mockSegmentRepo.EXPECT().GetSegmentsDueForRecompute(ctx, "workspace1", 100).Return(segments, nil)
		mockTaskService.EXPECT().CreateTask(ctx, "workspace1", gomock.Any()).Return(nil).Times(2)
		// Edited meanwhile, then a database error: the tasks stay, and their builds only write
		// once the segment is at their version.
		mockSegmentRepo.EXPECT().StartRecompute(ctx, "workspace1", "segment1", int64(1)).Return(false, nil)
		mockSegmentRepo.EXPECT().StartRecompute(ctx, "workspace1", "segment2", int64(5)).Return(false, assert.AnError)

		completed, err := processor.Process(ctx, task, time.Now().Add(time.Minute))
		assert.NoError(t, err)
		assert.False(t, completed)
	})
}

// buildTaskFor matches a build_segment task for segmentID.
func buildTaskFor(segmentID string) gomock.Matcher {
	return buildTaskMatcher{segmentID: segmentID}
}

type buildTaskMatcher struct{ segmentID string }

func (m buildTaskMatcher) Matches(x interface{}) bool {
	task, ok := x.(*domain.Task)
	return ok && task.State != nil && task.State.BuildSegment != nil && task.State.BuildSegment.SegmentID == m.segmentID
}

func (m buildTaskMatcher) String() string { return "build_segment task for " + m.segmentID }

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
