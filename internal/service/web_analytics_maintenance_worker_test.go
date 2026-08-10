package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/golang/mock/gomock"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/Notifuse/notifuse/pkg/logger"
)

func newMaintenanceWorkerForTest(t *testing.T, now time.Time) (*WebAnalyticsMaintenanceWorker, *mocks.MockWorkspaceRepository, *mocks.MockWebAnalyticsRepository) {
	t.Helper()
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)
	workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	webRepo := mocks.NewMockWebAnalyticsRepository(ctrl)
	worker := NewWebAnalyticsMaintenanceWorker(workspaceRepo, webRepo, logger.NewLogger())
	worker.nowFn = func() time.Time { return now }
	return worker, workspaceRepo, webRepo
}

func maintenanceWorkspace(id string) *domain.Workspace {
	return &domain.Workspace{
		ID: id,
		Settings: domain.WorkspaceSettings{
			WebAnalytics: &domain.WebAnalyticsSettings{Enabled: true},
		},
	}
}

func TestWebAnalyticsMaintenanceWorkerRunOnce(t *testing.T) {
	now := time.Date(2026, 8, 8, 3, 0, 0, 0, time.UTC)
	currentMonth := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	nextMonth := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)

	t.Run("ensures partitions, resets last month's autovacuum, analyzes new ones", func(t *testing.T) {
		worker, workspaceRepo, webRepo := newMaintenanceWorkerForTest(t, now)
		workspaceRepo.EXPECT().List(gomock.Any()).Return([]*domain.Workspace{maintenanceWorkspace("ws1")}, nil)

		// Existing: current month present, next month absent, July present.
		webRepo.EXPECT().ListPartitions(gomock.Any(), "ws1", "web_sessions").
			Return([]string{"web_sessions_y2026m07", "web_sessions_y2026m08"}, nil)
		webRepo.EXPECT().ListPartitions(gomock.Any(), "ws1", "web_pages").
			Return([]string{"web_pages_y2026m07", "web_pages_y2026m08"}, nil)
		webRepo.EXPECT().ListPartitions(gomock.Any(), "ws1", "web_goals").
			Return([]string{"web_goals_y2026m07", "web_goals_y2026m08"}, nil)

		webRepo.EXPECT().EnsureMonthlyPartitions(gomock.Any(), "ws1", []time.Time{currentMonth, nextMonth}).Return(nil)

		// July (previous month) partitions get their autovacuum profile reset.
		for _, table := range []string{"web_sessions", "web_pages", "web_goals"} {
			webRepo.EXPECT().SetPartitionAutovacuum(gomock.Any(), "ws1", table+"_y2026m07", false).Return(nil)
		}

		// Newly created (September) partitions are analyzed.
		webRepo.EXPECT().AnalyzePartitions(gomock.Any(), "ws1",
			[]string{"web_sessions_y2026m09", "web_pages_y2026m09", "web_goals_y2026m09"}).Return(nil)

		worker.RunOnce(context.Background())
	})

	t.Run("workspaces without web analytics are skipped", func(t *testing.T) {
		worker, workspaceRepo, _ := newMaintenanceWorkerForTest(t, now)
		workspaceRepo.EXPECT().List(gomock.Any()).Return([]*domain.Workspace{{ID: "plain"}}, nil)
		worker.RunOnce(context.Background())
	})

	t.Run("a broken workspace does not stall the others", func(t *testing.T) {
		worker, workspaceRepo, webRepo := newMaintenanceWorkerForTest(t, now)
		workspaceRepo.EXPECT().List(gomock.Any()).Return([]*domain.Workspace{
			maintenanceWorkspace("broken"),
			maintenanceWorkspace("healthy"),
		}, nil)

		webRepo.EXPECT().ListPartitions(gomock.Any(), "broken", "web_sessions").
			Return(nil, errors.New("database is on fire"))

		webRepo.EXPECT().ListPartitions(gomock.Any(), "healthy", gomock.Any()).Return(nil, nil).Times(3)
		webRepo.EXPECT().EnsureMonthlyPartitions(gomock.Any(), "healthy", gomock.Any()).Return(nil)
		webRepo.EXPECT().AnalyzePartitions(gomock.Any(), "healthy", gomock.Any()).Return(nil)

		worker.RunOnce(context.Background())
	})

	t.Run("context cancellation stops the sweep", func(t *testing.T) {
		worker, workspaceRepo, _ := newMaintenanceWorkerForTest(t, now)
		ctx, cancel := context.WithCancel(context.Background())
		workspaceRepo.EXPECT().List(gomock.Any()).DoAndReturn(func(context.Context) ([]*domain.Workspace, error) {
			cancel()
			return []*domain.Workspace{maintenanceWorkspace("ws1")}, nil
		})
		// No per-workspace expectations: the cancelled context short-circuits.
		worker.RunOnce(ctx)
	})

	t.Run("Start honors the initial delay and shuts down cleanly", func(t *testing.T) {
		worker, workspaceRepo, webRepo := newMaintenanceWorkerForTest(t, now)
		worker.initialDelay = 10 * time.Millisecond
		worker.interval = time.Hour

		ran := make(chan struct{})
		workspaceRepo.EXPECT().List(gomock.Any()).DoAndReturn(func(context.Context) ([]*domain.Workspace, error) {
			close(ran)
			return nil, nil
		})
		_ = webRepo

		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		go func() { worker.Start(ctx); close(done) }()

		select {
		case <-ran:
		case <-time.After(2 * time.Second):
			t.Fatal("initial run never happened")
		}
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("worker did not stop")
		}
	})
}
