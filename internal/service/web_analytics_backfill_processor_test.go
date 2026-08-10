package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/Notifuse/notifuse/pkg/logger"
)

func newBackfillProcessorForTest(t *testing.T) (*WebAnalyticsBackfillProcessor, *mocks.MockWorkspaceRepository, *mocks.MockWebAnalyticsRepository, *mocks.MockTaskRepository) {
	t.Helper()
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)
	workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	webRepo := mocks.NewMockWebAnalyticsRepository(ctrl)
	taskRepo := mocks.NewMockTaskRepository(ctrl)
	processor := NewWebAnalyticsBackfillProcessor(workspaceRepo, webRepo, taskRepo, logger.NewLogger())
	return processor, workspaceRepo, webRepo, taskRepo
}

func backfillWorkspace(filters []domain.WebFilter) *domain.Workspace {
	return &domain.Workspace{
		ID: "ws1",
		Settings: domain.WorkspaceSettings{
			WebAnalytics: &domain.WebAnalyticsSettings{Enabled: true, Filters: filters},
		},
	}
}

func backfillTask() *domain.Task {
	return &domain.Task{
		ID:          "task1",
		WorkspaceID: "ws1",
		Type:        domain.WebAnalyticsBackfillTaskType,
		Status:      domain.TaskStatusRunning,
		State:       &domain.TaskState{},
	}
}

func TestWebAnalyticsBackfillProcessor(t *testing.T) {
	filters := []domain.WebFilter{{
		ID: "r1", Name: "rule", Priority: 500, Enabled: true,
		Operations: []domain.WebFilterOperation{{Dimension: "channel", Action: domain.WebFilterActionSetValue, Value: "x"}},
	}}
	farDeadline := time.Now().Add(time.Hour)

	t.Run("CanProcess", func(t *testing.T) {
		processor, _, _, _ := newBackfillProcessorForTest(t)
		assert.True(t, processor.CanProcess(domain.WebAnalyticsBackfillTaskType))
		assert.False(t, processor.CanProcess("send_broadcast"))
	})

	t.Run("full run: lists partitions, rewrites each, completes", func(t *testing.T) {
		processor, workspaceRepo, webRepo, taskRepo := newBackfillProcessorForTest(t)

		workspaceRepo.EXPECT().GetByID(gomock.Any(), "ws1").Return(backfillWorkspace(filters), nil)
		webRepo.EXPECT().ListPartitions(gomock.Any(), "ws1", "web_sessions").
			Return([]string{"web_sessions_y2026m07", "web_sessions_y2026m08"}, nil)
		webRepo.EXPECT().ListPartitions(gomock.Any(), "ws1", "web_goals").
			Return([]string{"web_goals_y2026m08"}, nil)
		webRepo.EXPECT().BackfillPartition(gomock.Any(), "ws1", "web_sessions_y2026m07", gomock.Any()).Return(int64(100), nil)
		webRepo.EXPECT().BackfillPartition(gomock.Any(), "ws1", "web_sessions_y2026m08", gomock.Any()).Return(int64(200), nil)
		webRepo.EXPECT().BackfillPartition(gomock.Any(), "ws1", "web_goals_y2026m08", gomock.Any()).Return(int64(7), nil)
		taskRepo.EXPECT().SaveState(gomock.Any(), "ws1", "task1", gomock.Any(), gomock.Any()).Return(nil).Times(3)

		task := backfillTask()
		completed, err := processor.Process(context.Background(), task, farDeadline)
		require.NoError(t, err)
		assert.True(t, completed)
		assert.Equal(t, float64(100), task.Progress)
		assert.Equal(t, int64(307), task.State.WebAnalyticsBackfill.RowsUpdated)
		assert.Contains(t, task.State.Message, "complete")
	})

	t.Run("timeout pauses with resumable state and no error", func(t *testing.T) {
		processor, workspaceRepo, webRepo, taskRepo := newBackfillProcessorForTest(t)

		workspaceRepo.EXPECT().GetByID(gomock.Any(), "ws1").Return(backfillWorkspace(filters), nil)
		webRepo.EXPECT().ListPartitions(gomock.Any(), "ws1", "web_sessions").Return([]string{"web_sessions_y2026m08"}, nil)
		webRepo.EXPECT().ListPartitions(gomock.Any(), "ws1", "web_goals").Return(nil, nil)
		taskRepo.EXPECT().SaveState(gomock.Any(), "ws1", "task1", gomock.Any(), gomock.Any()).Return(nil)

		task := backfillTask()
		completed, err := processor.Process(context.Background(), task, time.Now().Add(time.Second))
		require.NoError(t, err, "a timeout pause must not consume the retry budget")
		assert.False(t, completed)
		assert.Equal(t, 0, task.State.WebAnalyticsBackfill.PartitionIndex)
	})

	t.Run("resumes from the saved index without relisting", func(t *testing.T) {
		processor, workspaceRepo, webRepo, taskRepo := newBackfillProcessorForTest(t)

		workspaceRepo.EXPECT().GetByID(gomock.Any(), "ws1").Return(backfillWorkspace(filters), nil)
		// No ListPartitions expectations: state already carries the plan.
		webRepo.EXPECT().BackfillPartition(gomock.Any(), "ws1", "web_goals_y2026m08", gomock.Any()).Return(int64(5), nil)
		taskRepo.EXPECT().SaveState(gomock.Any(), "ws1", "task1", gomock.Any(), gomock.Any()).Return(nil)

		task := backfillTask()
		task.State.WebAnalyticsBackfill = &domain.WebAnalyticsBackfillState{
			FiltersVersion: domain.ComputeWebFiltersVersion(filters),
			Partitions:     []string{"web_sessions_y2026m08", "web_goals_y2026m08"},
			PartitionIndex: 1,
			RowsUpdated:    100,
		}
		completed, err := processor.Process(context.Background(), task, farDeadline)
		require.NoError(t, err)
		assert.True(t, completed)
		assert.Equal(t, int64(105), task.State.WebAnalyticsBackfill.RowsUpdated)
	})

	t.Run("rule change mid-run restarts against the current version", func(t *testing.T) {
		processor, workspaceRepo, webRepo, taskRepo := newBackfillProcessorForTest(t)

		workspaceRepo.EXPECT().GetByID(gomock.Any(), "ws1").Return(backfillWorkspace(filters), nil)
		webRepo.EXPECT().ListPartitions(gomock.Any(), "ws1", "web_sessions").Return([]string{"web_sessions_y2026m08"}, nil)
		webRepo.EXPECT().ListPartitions(gomock.Any(), "ws1", "web_goals").Return(nil, nil)
		webRepo.EXPECT().BackfillPartition(gomock.Any(), "ws1", "web_sessions_y2026m08", gomock.Any()).Return(int64(9), nil)
		taskRepo.EXPECT().SaveState(gomock.Any(), "ws1", "task1", gomock.Any(), gomock.Any()).Return(nil)

		task := backfillTask()
		task.State.WebAnalyticsBackfill = &domain.WebAnalyticsBackfillState{
			FiltersVersion: "stale000",
			Partitions:     []string{"web_sessions_y2026m01"},
			PartitionIndex: 1,
			RowsUpdated:    999,
		}
		completed, err := processor.Process(context.Background(), task, farDeadline)
		require.NoError(t, err)
		assert.True(t, completed)
		assert.Equal(t, int64(9), task.State.WebAnalyticsBackfill.RowsUpdated, "restart resets progress")
	})

	t.Run("partition failure surfaces the error after saving state", func(t *testing.T) {
		processor, workspaceRepo, webRepo, taskRepo := newBackfillProcessorForTest(t)

		workspaceRepo.EXPECT().GetByID(gomock.Any(), "ws1").Return(backfillWorkspace(filters), nil)
		webRepo.EXPECT().ListPartitions(gomock.Any(), "ws1", "web_sessions").Return([]string{"web_sessions_y2026m08"}, nil)
		webRepo.EXPECT().ListPartitions(gomock.Any(), "ws1", "web_goals").Return(nil, nil)
		webRepo.EXPECT().BackfillPartition(gomock.Any(), "ws1", "web_sessions_y2026m08", gomock.Any()).
			Return(int64(0), errors.New("lock timeout"))
		taskRepo.EXPECT().SaveState(gomock.Any(), "ws1", "task1", gomock.Any(), gomock.Any()).Return(nil)

		task := backfillTask()
		completed, err := processor.Process(context.Background(), task, farDeadline)
		assert.False(t, completed)
		assert.ErrorContains(t, err, "lock timeout")
	})

	t.Run("workspace without web analytics completes as a no-op", func(t *testing.T) {
		processor, workspaceRepo, _, _ := newBackfillProcessorForTest(t)
		workspaceRepo.EXPECT().GetByID(gomock.Any(), "ws1").Return(&domain.Workspace{ID: "ws1"}, nil)

		completed, err := processor.Process(context.Background(), backfillTask(), farDeadline)
		require.NoError(t, err)
		assert.True(t, completed)
	})
}

func TestWebAnalyticsServiceBackfillRPCs(t *testing.T) {
	ctx := context.Background()

	authorize := func(auth *mocks.MockAuthService, permissions domain.UserPermissions) {
		auth.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), "ws1").
			Return(ctx, &domain.User{ID: "u1"}, &domain.UserWorkspace{
				UserID: "u1", WorkspaceID: "ws1", Role: "member", Permissions: permissions,
			}, nil)
	}
	writePerms := domain.UserPermissions{domain.PermissionResourceWebAnalytics: {Read: true, Write: true}}
	readPerms := domain.UserPermissions{domain.PermissionResourceWebAnalytics: {Read: true, Write: false}}

	newService := func(t *testing.T) (*WebAnalyticsService, *mocks.MockAuthService, *mocks.MockTaskRepository) {
		ctrl := gomock.NewController(t)
		t.Cleanup(ctrl.Finish)
		auth := mocks.NewMockAuthService(ctrl)
		taskRepo := mocks.NewMockTaskRepository(ctrl)
		webRepo := mocks.NewMockWebAnalyticsRepository(ctrl)
		buffer := NewWebAnalyticsBuffer(webRepo, logger.NewLogger(), WebAnalyticsBufferConfig{})
		svc := NewWebAnalyticsService(mocks.NewMockWorkspaceRepository(ctrl), nil, buffer, nil, auth, taskRepo, nil, logger.NewLogger())
		return svc, auth, taskRepo
	}

	t.Run("start creates a pending task", func(t *testing.T) {
		svc, auth, taskRepo := newService(t)
		authorize(auth, writePerms)
		taskRepo.EXPECT().List(gomock.Any(), "ws1", gomock.Any()).Return(nil, 0, nil)
		taskRepo.EXPECT().Create(gomock.Any(), "ws1", gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, task *domain.Task) error {
				assert.Equal(t, domain.WebAnalyticsBackfillTaskType, task.Type)
				assert.Equal(t, domain.TaskStatusPending, task.Status)
				task.ID = "created"
				return nil
			})

		status, err := svc.BackfillStart(ctx, "ws1")
		require.NoError(t, err)
		assert.Equal(t, "created", status.TaskID)
		assert.Equal(t, "pending", status.Status)
	})

	t.Run("start rejected while a run is in flight", func(t *testing.T) {
		svc, auth, taskRepo := newService(t)
		authorize(auth, writePerms)
		taskRepo.EXPECT().List(gomock.Any(), "ws1", gomock.Any()).
			Return([]*domain.Task{{ID: "t0", Status: domain.TaskStatusRunning, CreatedAt: time.Now()}}, 1, nil)

		_, err := svc.BackfillStart(ctx, "ws1")
		assert.ErrorContains(t, err, "already in progress")
	})

	t.Run("start requires write permission", func(t *testing.T) {
		svc, auth, _ := newService(t)
		authorize(auth, readPerms)
		_, err := svc.BackfillStart(ctx, "ws1")
		var permErr *domain.PermissionError
		assert.ErrorAs(t, err, &permErr)
	})

	t.Run("status returns the newest run and needs only read", func(t *testing.T) {
		svc, auth, taskRepo := newService(t)
		authorize(auth, readPerms)
		old := &domain.Task{ID: "old", Status: domain.TaskStatusCompleted, CreatedAt: time.Now().Add(-time.Hour)}
		recent := &domain.Task{ID: "recent", Status: domain.TaskStatusRunning, Progress: 40, CreatedAt: time.Now()}
		taskRepo.EXPECT().List(gomock.Any(), "ws1", gomock.Any()).Return([]*domain.Task{old, recent}, 2, nil)

		status, err := svc.BackfillStatus(ctx, "ws1")
		require.NoError(t, err)
		assert.Equal(t, "recent", status.TaskID)
		assert.Equal(t, float64(40), status.Progress)
	})

	t.Run("cancel marks the run failed", func(t *testing.T) {
		svc, auth, taskRepo := newService(t)
		authorize(auth, writePerms)
		running := &domain.Task{ID: "t1", Status: domain.TaskStatusRunning, CreatedAt: time.Now()}
		taskRepo.EXPECT().List(gomock.Any(), "ws1", gomock.Any()).Return([]*domain.Task{running}, 1, nil)
		taskRepo.EXPECT().Update(gomock.Any(), "ws1", gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, task *domain.Task) error {
				assert.Equal(t, domain.TaskStatusFailed, task.Status)
				require.NotNil(t, task.ErrorMessage)
				assert.Contains(t, *task.ErrorMessage, "cancelled")
				return nil
			})

		require.NoError(t, svc.BackfillCancel(ctx, "ws1"))
	})

	t.Run("cancel with nothing running errors", func(t *testing.T) {
		svc, auth, taskRepo := newService(t)
		authorize(auth, writePerms)
		taskRepo.EXPECT().List(gomock.Any(), "ws1", gomock.Any()).Return(nil, 0, nil)
		assert.ErrorContains(t, svc.BackfillCancel(ctx, "ws1"), "no backfill in progress")
	})
}
