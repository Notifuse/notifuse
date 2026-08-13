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
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
)

func TestDemoEnableWebAnalytics(t *testing.T) {
	newService := func(t *testing.T) (*DemoService, *mocks.MockWorkspaceRepository) {
		t.Helper()
		ctrl := gomock.NewController(t)
		t.Cleanup(ctrl.Finish)

		workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
		return &DemoService{
			logger:        pkgmocks.NewMockLogger(ctrl),
			workspaceRepo: workspaceRepo,
		}, workspaceRepo
	}

	demoWorkspace := func() *domain.Workspace {
		defaults := domain.DefaultWebFilters()
		return &domain.Workspace{
			ID: "demo",
			Settings: domain.WorkspaceSettings{
				WebAnalytics: &domain.WebAnalyticsSettings{
					Enabled:                false,
					BounceThresholdSeconds: 10,
					Filters:                defaults,
					FiltersVersion:         domain.ComputeWebFiltersVersion(defaults),
				},
			},
		}
	}

	t.Run("turns the feature on and keeps the workspace defaults", func(t *testing.T) {
		// The default rules a real workspace starts from are what an operator will
		// recognise; replacing them would hide the actual starting point.
		service, workspaceRepo := newService(t)
		workspace := demoWorkspace()
		defaultCount := len(workspace.Settings.WebAnalytics.Filters)

		workspaceRepo.EXPECT().GetByID(gomock.Any(), "demo").Return(workspace, nil)
		workspaceRepo.EXPECT().Update(gomock.Any(), workspace).Return(nil)

		filters, err := service.enableDemoWebAnalytics(context.Background(), "demo")
		require.NoError(t, err)

		settings := workspace.Settings.WebAnalytics
		assert.True(t, settings.Enabled)
		assert.Equal(t, []string{"*.apple.com"}, settings.AllowedDomains)
		assert.Equal(t, "Product line", settings.CustomDimensionLabels["custom_1"])
		assert.Greater(t, len(filters), defaultCount, "demo rules are appended, not substituted")
		assert.Len(t, filters,
			defaultCount+len(demoChannelFilters())+len(demoProductCategoryFilters()))
	})

	t.Run("recomputes the filters version so a backfill is not falsely stale", func(t *testing.T) {
		service, workspaceRepo := newService(t)
		workspace := demoWorkspace()
		before := workspace.Settings.WebAnalytics.FiltersVersion

		workspaceRepo.EXPECT().GetByID(gomock.Any(), "demo").Return(workspace, nil)
		workspaceRepo.EXPECT().Update(gomock.Any(), gomock.Any()).Return(nil)

		_, err := service.enableDemoWebAnalytics(context.Background(), "demo")
		require.NoError(t, err)

		settings := workspace.Settings.WebAnalytics
		assert.NotEqual(t, before, settings.FiltersVersion)
		assert.Equal(t, domain.ComputeWebFiltersVersion(settings.Filters), settings.FiltersVersion)
	})

	t.Run("reports a failure to persist rather than generating against stale rules", func(t *testing.T) {
		service, workspaceRepo := newService(t)
		workspaceRepo.EXPECT().GetByID(gomock.Any(), "demo").Return(demoWorkspace(), nil)
		workspaceRepo.EXPECT().Update(gomock.Any(), gomock.Any()).Return(errors.New("write failed"))

		_, err := service.enableDemoWebAnalytics(context.Background(), "demo")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "enable demo web analytics")
	})
}

func TestDemoWebAnalyticsMonthPlanning(t *testing.T) {
	generator := demoTestGenerator(t, 1000, 400)

	t.Run("every month the data touches gets a partition", func(t *testing.T) {
		months := demoMonthsCovering(generator)

		covered := map[time.Time]bool{}
		for _, month := range months {
			covered[month] = true
		}
		for day := 0; day < generator.Days(); day++ {
			assert.True(t, covered[demoMonthOf(generator.DayStart(day))],
				"day %d has no partition", day)
		}
		// A 400-day window spans fourteen calendar months at most, plus the
		// next one so a session ingested seconds after the reset still lands.
		assert.GreaterOrEqual(t, len(months), 14)
	})

	t.Run("the next month is provisioned ahead of ingestion", func(t *testing.T) {
		next := demoMonthOf(time.Now().UTC()).AddDate(0, 1, 0)
		assert.Contains(t, demoMonthsCovering(generator), next)
	})
}

func TestDemoMonthOrdering(t *testing.T) {
	// Newest first: the ranges a visitor is most likely to open are populated
	// before the older history lands.
	months := []time.Time{
		time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC),
	}
	sortTimesDescending(months)

	assert.Equal(t, 2026, months[0].Year())
	assert.Equal(t, time.August, months[0].Month())
	assert.Equal(t, time.December, months[2].Month())
}
