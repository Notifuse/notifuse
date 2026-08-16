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

func newUsageServiceForTest(t *testing.T) (*UsageService, *mocks.MockWorkspaceRepository, *mocks.MockWebAnalyticsRepository) {
	t.Helper()
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)
	workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	webRepo := mocks.NewMockWebAnalyticsRepository(ctrl)
	return NewUsageService(workspaceRepo, webRepo, logger.NewLogger()), workspaceRepo, webRepo
}

func TestUsageService_GetUsage(t *testing.T) {
	july := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	august := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	months := []time.Time{july, august}

	t.Run("sums across workspaces", func(t *testing.T) {
		svc, workspaceRepo, webRepo := newUsageServiceForTest(t)
		workspaceRepo.EXPECT().List(gomock.Any()).Return([]*domain.Workspace{{ID: "ws1"}, {ID: "ws2"}}, nil)

		at := time.Date(2026, 8, 16, 2, 0, 0, 0, time.UTC)
		webRepo.EXPECT().GetUsage(gomock.Any(), "ws1", months).Return([]*domain.MonthlyUsage{
			{PeriodMonth: july, Pageviews: 100, TimelineEntries: 10, ComputedAt: at},
			{PeriodMonth: august, Pageviews: 1000, TimelineEntries: 50, ComputedAt: at},
		}, nil)
		webRepo.EXPECT().GetUsage(gomock.Any(), "ws2", months).Return([]*domain.MonthlyUsage{
			{PeriodMonth: august, Pageviews: 200, TimelineEntries: 5, ComputedAt: at},
		}, nil)

		report, err := svc.GetUsage(context.Background(), months)
		require.NoError(t, err)
		require.Len(t, report.Months, 2)

		// Order follows the request, so months[0] is the month the caller put first.
		assert.Equal(t, july, report.Months[0].PeriodMonth)
		assert.Equal(t, int64(100), report.Months[0].Pageviews)
		assert.Equal(t, 1, report.Months[0].Workspaces)

		assert.Equal(t, august, report.Months[1].PeriodMonth)
		assert.Equal(t, int64(1200), report.Months[1].Pageviews)
		assert.Equal(t, int64(55), report.Months[1].TimelineEntries)
		assert.Equal(t, 2, report.Months[1].Workspaces)

		// The denominator: two workspaces exist, both contributed to August, only
		// one to July.
		assert.Equal(t, 2, report.WorkspaceCount)
	})

	t.Run("a workspace that cannot be read fails the whole report", func(t *testing.T) {
		svc, workspaceRepo, webRepo := newUsageServiceForTest(t)
		workspaceRepo.EXPECT().List(gomock.Any()).Return([]*domain.Workspace{{ID: "ws-broken"}}, nil)
		webRepo.EXPECT().GetUsage(gomock.Any(), "ws-broken", months).Return(nil, errors.New("database is gone"))

		// Never a partial total: a sum missing one workspace's traffic is a wrong
		// number the control plane would act on, whereas an error means no usage
		// was reported, which it already knows to skip.
		report, err := svc.GetUsage(context.Background(), months)
		require.Error(t, err)
		assert.Nil(t, report)
		assert.Contains(t, err.Error(), "ws-broken")
	})

	t.Run("a workspace with no snapshot contributes nothing but is still counted", func(t *testing.T) {
		svc, workspaceRepo, webRepo := newUsageServiceForTest(t)
		workspaceRepo.EXPECT().List(gomock.Any()).Return([]*domain.Workspace{{ID: "ws1"}, {ID: "ws-quiet"}}, nil)
		webRepo.EXPECT().GetUsage(gomock.Any(), "ws1", months).Return([]*domain.MonthlyUsage{
			{PeriodMonth: august, Pageviews: 7},
		}, nil)
		webRepo.EXPECT().GetUsage(gomock.Any(), "ws-quiet", months).Return(nil, nil)

		report, err := svc.GetUsage(context.Background(), months)
		require.NoError(t, err)
		require.Len(t, report.Months, 2)
		assert.Equal(t, int64(0), report.Months[0].Pageviews)
		assert.Equal(t, 0, report.Months[0].Workspaces)
		assert.Equal(t, int64(7), report.Months[1].Pageviews)
		assert.Equal(t, 1, report.Months[1].Workspaces)
		assert.Equal(t, 2, report.WorkspaceCount)
	})

	t.Run("reports the oldest snapshot time, not the freshest", func(t *testing.T) {
		svc, workspaceRepo, webRepo := newUsageServiceForTest(t)
		workspaceRepo.EXPECT().List(gomock.Any()).Return([]*domain.Workspace{{ID: "ws1"}, {ID: "ws2"}}, nil)

		fresh := time.Date(2026, 8, 16, 2, 0, 0, 0, time.UTC)
		stale := time.Date(2026, 8, 11, 2, 0, 0, 0, time.UTC)
		webRepo.EXPECT().GetUsage(gomock.Any(), "ws1", months).Return([]*domain.MonthlyUsage{
			{PeriodMonth: august, Pageviews: 1, ComputedAt: fresh},
		}, nil)
		webRepo.EXPECT().GetUsage(gomock.Any(), "ws2", months).Return([]*domain.MonthlyUsage{
			{PeriodMonth: august, Pageviews: 1, ComputedAt: stale},
		}, nil)

		report, err := svc.GetUsage(context.Background(), months)
		require.NoError(t, err)
		// A total is only as fresh as its stalest part; reporting the freshest
		// would hide a workspace whose meter stopped days ago.
		assert.Equal(t, stale, report.Months[1].ComputedAt)
	})

	t.Run("a mid-month instant still lands on the UTC month", func(t *testing.T) {
		svc, workspaceRepo, webRepo := newUsageServiceForTest(t)
		workspaceRepo.EXPECT().List(gomock.Any()).Return([]*domain.Workspace{{ID: "ws1"}}, nil)

		requested := []time.Time{time.Date(2026, 8, 17, 15, 4, 5, 0, time.UTC)}
		webRepo.EXPECT().GetUsage(gomock.Any(), "ws1", requested).Return([]*domain.MonthlyUsage{
			{PeriodMonth: august, Pageviews: 42},
		}, nil)

		report, err := svc.GetUsage(context.Background(), requested)
		require.NoError(t, err)
		require.Len(t, report.Months, 1)
		assert.Equal(t, august, report.Months[0].PeriodMonth)
		assert.Equal(t, int64(42), report.Months[0].Pageviews)
	})

	t.Run("listing failure propagates", func(t *testing.T) {
		svc, workspaceRepo, _ := newUsageServiceForTest(t)
		workspaceRepo.EXPECT().List(gomock.Any()).Return(nil, errors.New("no system database"))

		_, err := svc.GetUsage(context.Background(), months)
		require.Error(t, err)
	})

	t.Run("no months requested is an error, not an empty success", func(t *testing.T) {
		svc, _, _ := newUsageServiceForTest(t)
		_, err := svc.GetUsage(context.Background(), nil)
		require.Error(t, err)
	})
}
