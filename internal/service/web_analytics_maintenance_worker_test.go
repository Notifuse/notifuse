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

// newMaintenanceWorkerForTest builds the worker with usage metering stubbed out.
// RunOnce meters every workspace before it maintains it, and the partition tests
// are not about metering. Tests that are use newMaintenanceWorkerForUsageTest,
// which leaves RecomputeUsage unexpected so they can pin the exact calls.
func newMaintenanceWorkerForTest(t *testing.T, now time.Time) (*WebAnalyticsMaintenanceWorker, *mocks.MockWorkspaceRepository, *mocks.MockWebAnalyticsRepository) {
	t.Helper()
	worker, workspaceRepo, webRepo := newMaintenanceWorkerForUsageTest(t, now)
	webRepo.EXPECT().
		RecomputeUsage(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil).
		AnyTimes()
	return worker, workspaceRepo, webRepo
}

func newMaintenanceWorkerForUsageTest(t *testing.T, now time.Time) (*WebAnalyticsMaintenanceWorker, *mocks.MockWorkspaceRepository, *mocks.MockWebAnalyticsRepository) {
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

func TestWebAnalyticsMaintenanceWorkerUsageMetering(t *testing.T) {
	now := time.Date(2026, 8, 8, 3, 0, 0, 0, time.UTC)
	currentMonth := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	previousMonth := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)

	// Every maintenance call a workspace with analytics installed makes, stubbed
	// permissively: these tests are about metering, not partitions.
	allowMaintenance := func(webRepo *mocks.MockWebAnalyticsRepository) {
		webRepo.EXPECT().ListPartitions(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
		webRepo.EXPECT().EnsureMonthlyPartitions(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
		webRepo.EXPECT().SetPartitionAutovacuum(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
		webRepo.EXPECT().AnalyzePartitions(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	}

	t.Run("meters the open month live and the previous month closed", func(t *testing.T) {
		worker, workspaceRepo, webRepo := newMaintenanceWorkerForUsageTest(t, now)
		workspaceRepo.EXPECT().List(gomock.Any()).Return([]*domain.Workspace{maintenanceWorkspace("ws1")}, nil)
		allowMaintenance(webRepo)

		// live=false for July is the whole retention guard: a closed month's
		// stored counts must never be lowered by a later recount.
		webRepo.EXPECT().RecomputeUsage(gomock.Any(), "ws1", previousMonth, false).Return(nil)
		webRepo.EXPECT().RecomputeUsage(gomock.Any(), "ws1", currentMonth, true).Return(nil)

		worker.RunOnce(context.Background())
	})

	t.Run("meters a workspace whose web analytics settings are absent", func(t *testing.T) {
		worker, workspaceRepo, webRepo := newMaintenanceWorkerForUsageTest(t, now)
		noAnalytics := &domain.Workspace{ID: "ws-quiet"}
		workspaceRepo.EXPECT().List(gomock.Any()).Return([]*domain.Workspace{noAnalytics}, nil)

		// The timeline meter counts events that exist whether or not the tracking
		// snippet is installed, so clearing the analytics settings must not freeze
		// a workspace's usage history. No partition maintenance is expected: that
		// part is still correctly skipped.
		webRepo.EXPECT().RecomputeUsage(gomock.Any(), "ws-quiet", previousMonth, false).Return(nil)
		webRepo.EXPECT().RecomputeUsage(gomock.Any(), "ws-quiet", currentMonth, true).Return(nil)

		worker.RunOnce(context.Background())
	})

	t.Run("a failing recompute still lets partition maintenance run", func(t *testing.T) {
		worker, workspaceRepo, webRepo := newMaintenanceWorkerForUsageTest(t, now)
		workspaceRepo.EXPECT().List(gomock.Any()).Return([]*domain.Workspace{maintenanceWorkspace("ws1")}, nil)

		webRepo.EXPECT().RecomputeUsage(gomock.Any(), "ws1", previousMonth, false).Return(errors.New("count failed"))
		webRepo.EXPECT().RecomputeUsage(gomock.Any(), "ws1", currentMonth, true).Return(errors.New("count failed"))

		// Metering must never be able to stop the work that actually keeps
		// ingestion alive, so these are still expected exactly as usual.
		webRepo.EXPECT().ListPartitions(gomock.Any(), "ws1", gomock.Any()).Return(nil, nil).Times(3)
		webRepo.EXPECT().EnsureMonthlyPartitions(gomock.Any(), "ws1", gomock.Any()).Return(nil)
		webRepo.EXPECT().AnalyzePartitions(gomock.Any(), "ws1", gomock.Any()).Return(nil).AnyTimes()
		webRepo.EXPECT().SetPartitionAutovacuum(gomock.Any(), "ws1", gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

		worker.RunOnce(context.Background())
	})

	t.Run("a workspace whose metering fails does not stall the fleet", func(t *testing.T) {
		worker, workspaceRepo, webRepo := newMaintenanceWorkerForUsageTest(t, now)
		workspaceRepo.EXPECT().List(gomock.Any()).Return([]*domain.Workspace{
			maintenanceWorkspace("ws-broken"),
			maintenanceWorkspace("ws-fine"),
		}, nil)
		allowMaintenance(webRepo)

		webRepo.EXPECT().RecomputeUsage(gomock.Any(), "ws-broken", gomock.Any(), gomock.Any()).
			Return(errors.New("database is gone")).Times(2)
		webRepo.EXPECT().RecomputeUsage(gomock.Any(), "ws-fine", previousMonth, false).Return(nil)
		webRepo.EXPECT().RecomputeUsage(gomock.Any(), "ws-fine", currentMonth, true).Return(nil)

		worker.RunOnce(context.Background())
	})
}
