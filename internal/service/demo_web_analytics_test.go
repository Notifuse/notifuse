package service

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/Notifuse/notifuse/pkg/logger"
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

// demoWebSession builds a session the way the generator would, with only the
// fields the seeding paths read.
func demoWebSession(t *testing.T, createdAt time.Time, email string) *domain.WebSession {
	t.Helper()
	session := &domain.WebSession{
		ID:          createdAt.Format("20060102150405.000"),
		SessionDate: time.Date(createdAt.Year(), createdAt.Month(), createdAt.Day(), 0, 0, 0, 0, time.UTC),
		CreatedAt:   createdAt,
	}
	if email != "" {
		session.ContactEmail = &email
	}
	return session
}

func TestDemoProjectNavigation(t *testing.T) {
	now := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	from := now.AddDate(0, 0, -demoWebAnalyticsTimelineDays)

	newService := func(t *testing.T) (*DemoService, *mocks.MockWebAnalyticsRepository) {
		t.Helper()
		ctrl := gomock.NewController(t)
		t.Cleanup(ctrl.Finish)

		repo := mocks.NewMockWebAnalyticsRepository(ctrl)
		return &DemoService{
			logger:           logger.NewLoggerWithLevel("disabled"),
			webAnalyticsRepo: repo,
		}, repo
	}

	t.Run("projects only the identified sessions", func(t *testing.T) {
		// An anonymous visit has no contact to attach itself to, and the
		// projection's own SQL would discard it — passing it only widens the
		// session-id array the statement scans.
		service, repo := newService(t)
		known := demoWebSession(t, now.AddDate(0, 0, -3), "ada@example.com")
		sessions := []*domain.WebSession{
			known,
			demoWebSession(t, now.AddDate(0, 0, -3), ""),
		}

		var projected []*domain.WebSession
		repo.EXPECT().ProjectContactNavigation(gomock.Any(), "demo", gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, s []*domain.WebSession) error {
				projected = s
				return nil
			})

		service.projectDemoNavigation(context.Background(), "demo", sessions, from)
		require.Len(t, projected, 1)
		assert.Equal(t, known.ID, projected[0].ID)
	})

	t.Run("projects only the sessions inside the timeline window", func(t *testing.T) {
		// The analytics tables keep the full history; the drawer paginates ten
		// entries at a time, so the older visits stay out of it.
		service, repo := newService(t)
		recent := demoWebSession(t, now.AddDate(0, 0, -10), "ada@example.com")
		sessions := []*domain.WebSession{
			recent,
			demoWebSession(t, from.Add(-time.Second), "ada@example.com"),
			demoWebSession(t, now.AddDate(0, 0, -300), "grace@example.com"),
		}

		var projected []*domain.WebSession
		repo.EXPECT().ProjectContactNavigation(gomock.Any(), "demo", gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, s []*domain.WebSession) error {
				projected = s
				return nil
			})

		service.projectDemoNavigation(context.Background(), "demo", sessions, from)
		require.Len(t, projected, 1)
		assert.Equal(t, recent.ID, projected[0].ID)
	})

	t.Run("a session exactly on the boundary is kept", func(t *testing.T) {
		service, repo := newService(t)
		repo.EXPECT().ProjectContactNavigation(gomock.Any(), "demo", gomock.Len(1)).Return(nil)

		service.projectDemoNavigation(context.Background(), "demo",
			[]*domain.WebSession{demoWebSession(t, from, "ada@example.com")}, from)
	})

	t.Run("does not call the projection when nothing qualifies", func(t *testing.T) {
		// gomock fails the test on an unexpected call, which is the assertion.
		service, _ := newService(t)
		service.projectDemoNavigation(context.Background(), "demo",
			[]*domain.WebSession{demoWebSession(t, now.AddDate(0, 0, -300), "ada@example.com")}, from)
	})

	t.Run("a projection failure does not stop the seed", func(t *testing.T) {
		service, repo := newService(t)
		repo.EXPECT().ProjectContactNavigation(gomock.Any(), "demo", gomock.Any()).
			Return(errors.New("deadlock detected"))

		service.projectDemoNavigation(context.Background(), "demo",
			[]*domain.WebSession{demoWebSession(t, now, "ada@example.com")}, from)
	})
}

func TestDemoSeedWebGoalEvents(t *testing.T) {
	newService := func(t *testing.T) (*DemoService, *mocks.MockCustomEventRepository) {
		t.Helper()
		ctrl := gomock.NewController(t)
		t.Cleanup(ctrl.Finish)

		repo := mocks.NewMockCustomEventRepository(ctrl)
		return &DemoService{
			logger:          logger.NewLoggerWithLevel("disabled"),
			customEventRepo: repo,
		}, repo
	}

	goal := func(name, goalType, email string, value float64) *domain.WebGoal {
		at := time.Date(2026, 5, 2, 9, 30, 0, 0, time.UTC)
		g := &domain.WebGoal{
			SessionID: "session-1", TabID: 3, GoalName: name, GoalType: goalType,
			ClientTsMs: at.UnixMilli(), GoalAt: at.Add(time.Minute), GoalValue: value,
			Path: "/iphone-17-pro/", LandingPath: "/iphone-17-pro/",
			UTMSource: "google", UTMMedium: "cpc", UTMCampaign: "holiday-sale",
			Device: "desktop", Country: "US",
			Properties: map[string]string{"product": "iPhone 17 Pro"},
		}
		if email != "" {
			g.ContactEmail = &email
		}
		return g
	}

	t.Run("writes one event per identified goal, in one batch", func(t *testing.T) {
		service, repo := newService(t)
		goals := []*domain.WebGoal{
			goal("add_to_cart", domain.GoalTypeOther, "ada@example.com", 1199),
			goal("purchase", domain.GoalTypePurchase, "ada@example.com", 1199),
			goal("add_to_cart", domain.GoalTypeOther, "", 999), // anonymous
		}

		var written []*domain.CustomEvent
		repo.EXPECT().BatchInsertNew(gomock.Any(), "demo", gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, events []*domain.CustomEvent) error {
				written = events
				return nil
			})

		service.seedDemoWebGoalEvents(context.Background(), "demo", goals)

		require.Len(t, written, 2, "the anonymous goal has no contact to attach to")
		assert.Equal(t, "add_to_cart", written[0].EventName)
		assert.Equal(t, "ada@example.com", written[0].Email)
		assert.Equal(t, "web_analytics", written[0].Source)
		require.NotNil(t, written[1].GoalType)
		assert.Equal(t, domain.GoalTypePurchase, *written[1].GoalType)
		require.NotNil(t, written[1].GoalValue)
		assert.Equal(t, 1199.0, *written[1].GoalValue)
	})

	// The demo cannot go through WebAnalyticsContactBridge — it hangs off the ingest
	// buffer, and its staleness guard rejects everything the demo generates. Sharing the
	// payload builder is what stops the two representations drifting, and a drift would
	// only show up as a segment that matches on live traffic but not on the demo.
	t.Run("a demo goal and a live one are the same row", func(t *testing.T) {
		service, repo := newService(t)
		g := goal("purchase", domain.GoalTypePurchase, "ada@example.com", 1199)

		var written []*domain.CustomEvent
		repo.EXPECT().BatchInsertNew(gomock.Any(), "demo", gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, events []*domain.CustomEvent) error {
				written = events
				return nil
			})

		service.seedDemoWebGoalEvents(context.Background(), "demo", []*domain.WebGoal{g})
		require.Len(t, written, 1)

		live := buildWebGoalCustomEvent(g, normalizeWebGoalEventName(g.GoalName),
			time.UnixMilli(g.ClientTsMs).UTC())
		assert.Equal(t, live, written[0])
	})

	t.Run("the external id is the goal's own key, so a re-seed is idempotent", func(t *testing.T) {
		service, repo := newService(t)

		var written []*domain.CustomEvent
		repo.EXPECT().BatchInsertNew(gomock.Any(), "demo", gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, events []*domain.CustomEvent) error {
				written = events
				return nil
			})

		g := goal("purchase", domain.GoalTypePurchase, "ada@example.com", 1199)
		service.seedDemoWebGoalEvents(context.Background(), "demo", []*domain.WebGoal{g})

		require.Len(t, written, 1)
		assert.Equal(t,
			fmt.Sprintf("%s:%d:%s:%d", g.SessionID, g.TabID, g.GoalName, g.ClientTsMs),
			written[0].ExternalID)
	})

	t.Run("writes nothing when every goal is anonymous", func(t *testing.T) {
		// gomock fails on an unexpected call, which is the assertion.
		service, _ := newService(t)
		service.seedDemoWebGoalEvents(context.Background(), "demo",
			[]*domain.WebGoal{goal("add_to_cart", domain.GoalTypeOther, "", 999)})
	})

	t.Run("a write failure does not stop the seed", func(t *testing.T) {
		service, repo := newService(t)
		repo.EXPECT().BatchInsertNew(gomock.Any(), "demo", gomock.Any()).
			Return(errors.New("insert failed"))

		service.seedDemoWebGoalEvents(context.Background(), "demo",
			[]*domain.WebGoal{goal("purchase", domain.GoalTypePurchase, "ada@example.com", 999)})
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
