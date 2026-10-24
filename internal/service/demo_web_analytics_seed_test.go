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

// The demo web-analytics seed must turn the feature on with the demo's own
// classification rules, keep anonymous/old visits off contact timelines, and
// never let an analytics side effect fail the reset.

func demoWebAnalyticsSvc(t *testing.T) (*DemoService, *mocks.MockWebAnalyticsRepository, *mocks.MockWorkspaceRepository, *mocks.MockCustomEventRepository) {
	t.Helper()
	ctrl := gomock.NewController(t)
	webRepo := mocks.NewMockWebAnalyticsRepository(ctrl)
	wsRepo := mocks.NewMockWorkspaceRepository(ctrl)
	customRepo := mocks.NewMockCustomEventRepository(ctrl)
	svc := &DemoService{
		logger:           logger.NewLoggerWithLevel("disabled"),
		webAnalyticsRepo: webRepo,
		workspaceRepo:    wsRepo,
		customEventRepo:  customRepo,
	}
	return svc, webRepo, wsRepo, customRepo
}

func TestSeedWebAnalytics_RequiresRepository(t *testing.T) {
	svc := &DemoService{logger: logger.NewLoggerWithLevel("disabled")}
	err := svc.seedWebAnalytics(context.Background(), "ws-1", time.Now())
	require.EqualError(t, err, "web analytics repository is not configured")
}

func TestEnableDemoWebAnalytics(t *testing.T) {
	ctx := context.Background()

	t.Run("workspace load failure is wrapped", func(t *testing.T) {
		svc, _, wsRepo, _ := demoWebAnalyticsSvc(t)
		sentinel := errors.New("workspace gone")
		wsRepo.EXPECT().GetByID(gomock.Any(), "ws-1").Return(nil, sentinel).Times(1)
		_, err := svc.enableDemoWebAnalytics(ctx, "ws-1")
		require.ErrorContains(t, err, "failed to load demo workspace:")
		assert.ErrorIs(t, err, sentinel)
	})

	t.Run("enables the feature with demo domains, geo and appended filters", func(t *testing.T) {
		svc, _, wsRepo, _ := demoWebAnalyticsSvc(t)
		workspace := &domain.Workspace{ID: "ws-1"}
		wsRepo.EXPECT().GetByID(gomock.Any(), "ws-1").Return(workspace, nil).Times(1)
		wsRepo.EXPECT().Update(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, ws *domain.Workspace) error {
				s := ws.Settings.WebAnalytics
				require.NotNil(t, s)
				assert.True(t, s.Enabled)
				assert.Equal(t, []string{"*.apple.com"}, s.AllowedDomains)
				assert.True(t, s.GeoEnabled)
				assert.True(t, s.GeoStoreCity)
				assert.True(t, s.GeoStoreRegion)
				assert.Equal(t, 2, s.GeoCoordsPrecision)
				assert.Equal(t, "Product line", s.CustomDimensionLabels["custom_1"])
				assert.NotEmpty(t, s.Filters)
				assert.Equal(t, domain.ComputeWebFiltersVersion(s.Filters), s.FiltersVersion,
					"the stored version must match the stored filters or backfills misdetect changes")
				return nil
			}).Times(1)

		filters, err := svc.enableDemoWebAnalytics(ctx, "ws-1")
		require.NoError(t, err)
		assert.NotEmpty(t, filters)
		assert.True(t, workspace.Settings.WebAnalytics.Enabled)
	})

	t.Run("existing settings are kept and extended, not replaced", func(t *testing.T) {
		svc, _, wsRepo, _ := demoWebAnalyticsSvc(t)
		existing := &domain.WebAnalyticsSettings{BounceThresholdSeconds: 42}
		workspace := &domain.Workspace{ID: "ws-1"}
		workspace.Settings.WebAnalytics = existing
		wsRepo.EXPECT().GetByID(gomock.Any(), "ws-1").Return(workspace, nil).Times(1)
		wsRepo.EXPECT().Update(gomock.Any(), gomock.Any()).Return(nil).Times(1)

		_, err := svc.enableDemoWebAnalytics(ctx, "ws-1")
		require.NoError(t, err)
		assert.Equal(t, 42, existing.BounceThresholdSeconds,
			"a workspace's own bounce threshold survives the demo enablement")
	})

	t.Run("update failure is wrapped", func(t *testing.T) {
		svc, _, wsRepo, _ := demoWebAnalyticsSvc(t)
		wsRepo.EXPECT().GetByID(gomock.Any(), "ws-1").Return(&domain.Workspace{ID: "ws-1"}, nil).Times(1)
		sentinel := errors.New("write failed")
		wsRepo.EXPECT().Update(gomock.Any(), gomock.Any()).Return(sentinel).Times(1)
		_, err := svc.enableDemoWebAnalytics(ctx, "ws-1")
		require.ErrorContains(t, err, "failed to enable demo web analytics:")
		assert.ErrorIs(t, err, sentinel)
	})
}

func TestAnalyzeDemoWebAnalytics_SweepsAllTablesDespiteFailures(t *testing.T) {
	svc, webRepo, _, _ := demoWebAnalyticsSvc(t)
	// Failing the first table must not skip the rest: a partial ANALYZE would
	// leave some dashboards planning against empty statistics.
	for _, table := range []string{"web_sessions", "web_pages", "web_goals"} {
		webRepo.EXPECT().ListPartitions(gomock.Any(), "ws-1", table).
			Return(nil, errors.New("boom")).Times(1)
	}
	svc.analyzeDemoWebAnalytics(context.Background(), "ws-1") // must not panic
}

func TestAnalyzeDemoWebAnalytics_AnalyzesListedPartitions(t *testing.T) {
	svc, webRepo, _, _ := demoWebAnalyticsSvc(t)
	parts := []string{"web_sessions_2026_05"}
	webRepo.EXPECT().ListPartitions(gomock.Any(), "ws-1", "web_sessions").Return(parts, nil).Times(1)
	webRepo.EXPECT().AnalyzePartitions(gomock.Any(), "ws-1", parts).Return(nil).Times(1)
	webRepo.EXPECT().ListPartitions(gomock.Any(), "ws-1", "web_pages").Return(nil, nil).Times(1)
	webRepo.EXPECT().AnalyzePartitions(gomock.Any(), "ws-1", gomock.Any()).Return(errors.New("analyze failed")).Times(1)
	webRepo.EXPECT().ListPartitions(gomock.Any(), "ws-1", "web_goals").Return(nil, nil).Times(1)
	webRepo.EXPECT().AnalyzePartitions(gomock.Any(), "ws-1", gomock.Any()).Return(nil).Times(1)
	svc.analyzeDemoWebAnalytics(context.Background(), "ws-1")
}

func TestProjectDemoNavigation_Filtering(t *testing.T) {
	ctx := context.Background()
	from := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	email := "person@example.com"
	recent := from.Add(24 * time.Hour)

	session := func(contactEmail *string, created time.Time) *domain.WebSession {
		return &domain.WebSession{ContactEmail: contactEmail, CreatedAt: created}
	}

	t.Run("only identified, in-window sessions are projected", func(t *testing.T) {
		svc, webRepo, _, _ := demoWebAnalyticsSvc(t)
		kept := session(&email, recent)
		webRepo.EXPECT().ProjectContactNavigation(gomock.Any(), "ws-1", []*domain.WebSession{kept}).
			Return(nil).Times(1)
		svc.projectDemoNavigation(ctx, "ws-1", []*domain.WebSession{
			session(nil, recent),                  // anonymous
			session(&[]string{""}[0], recent),     // empty identity
			session(&email, from.Add(-time.Hour)), // before the window
			kept,
		}, from)
	})

	t.Run("nothing identified means no repository call", func(t *testing.T) {
		svc, _, _, _ := demoWebAnalyticsSvc(t)
		svc.projectDemoNavigation(ctx, "ws-1", []*domain.WebSession{
			session(nil, recent),
		}, from)
	})

	t.Run("projection failure is swallowed", func(t *testing.T) {
		svc, webRepo, _, _ := demoWebAnalyticsSvc(t)
		webRepo.EXPECT().ProjectContactNavigation(gomock.Any(), "ws-1", gomock.Any()).
			Return(errors.New("timeline down")).Times(1)
		assert.NotPanics(t, func() {
			svc.projectDemoNavigation(ctx, "ws-1", []*domain.WebSession{session(&email, recent)}, from)
		})
	})
}

func TestSeedDemoWebGoalEvents(t *testing.T) {
	ctx := context.Background()
	email := "buyer@example.com"

	t.Run("identified goals become BatchInsertNew custom events", func(t *testing.T) {
		svc, _, _, customRepo := demoWebAnalyticsSvc(t)
		goal := &domain.WebGoal{
			ContactEmail: &email, GoalName: "Purchase", ClientTsMs: 1_780_000_000_000,
		}
		customRepo.EXPECT().BatchInsertNew(gomock.Any(), "ws-1", gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, events []*domain.CustomEvent) error {
				require.Len(t, events, 1)
				assert.Equal(t, "buyer@example.com", events[0].Email)
				assert.NotEmpty(t, events[0].EventName)
				return nil
			}).Times(1)
		svc.seedDemoWebGoalEvents(ctx, "ws-1", []*domain.WebGoal{goal})
	})

	t.Run("anonymous goals and a missing repo are no-ops", func(t *testing.T) {
		svc, _, _, _ := demoWebAnalyticsSvc(t)
		svc.seedDemoWebGoalEvents(ctx, "ws-1", []*domain.WebGoal{{GoalName: "Purchase"}})

		svcNoRepo := &DemoService{logger: logger.NewLoggerWithLevel("disabled")}
		svcNoRepo.seedDemoWebGoalEvents(ctx, "ws-1", []*domain.WebGoal{
			{ContactEmail: &email, GoalName: "Purchase", ClientTsMs: 1},
		})
	})
}

func TestDemoMonthsCovering(t *testing.T) {
	now := time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC)
	generator := newDemoWebAnalyticsGenerator(demoWebAnalyticsOptions{
		Sessions: 10, Days: 40, Now: now, Seed: 1, SiteURL: "https://example.com",
	})
	months := demoMonthsCovering(generator)
	require.NotEmpty(t, months)
	for _, m := range months {
		assert.Equal(t, 1, m.Day(), "months are truncated to their first day")
		assert.Equal(t, time.UTC, m.Location())
	}
	// The successor of the current (real-clock) month is always present so a
	// session generated right after the reset has a partition to land in.
	assert.Contains(t, months, demoMonthOf(time.Now().UTC()).AddDate(0, 1, 0))
}

func TestFlushDemoWebAnalytics_TinyWindow(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC)
	newGen := func() *demoWebAnalyticsGenerator {
		return newDemoWebAnalyticsGenerator(demoWebAnalyticsOptions{
			Sessions: 12, Days: 3, Now: now, Seed: 7, SiteURL: "https://www.apple.com",
		})
	}

	t.Run("days are grouped by month into one flush with summed counts", func(t *testing.T) {
		svc, webRepo, _, _ := demoWebAnalyticsSvc(t)
		var flushedSessions int
		webRepo.EXPECT().FlushBatch(gomock.Any(), "ws-1", gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, sessions []*domain.WebSession, _ []*domain.WebPage, _ []*domain.WebGoal) error {
				flushedSessions += len(sessions)
				return nil
			}).Times(1) // three May days share one month bucket

		sessions, pages, goals, err := svc.flushDemoWebAnalytics(ctx, "ws-1", newGen(), now)
		require.NoError(t, err)
		assert.Equal(t, flushedSessions, sessions, "returned count must match what was written")
		assert.Greater(t, sessions, 0)
		assert.Greater(t, pages, 0)
		assert.GreaterOrEqual(t, goals, 0)
	})

	t.Run("a write failure names its month and returns counts so far", func(t *testing.T) {
		svc, webRepo, _, _ := demoWebAnalyticsSvc(t)
		sentinel := errors.New("insert failed")
		webRepo.EXPECT().FlushBatch(gomock.Any(), "ws-1", gomock.Any(), gomock.Any(), gomock.Any()).
			Return(sentinel).Times(1)

		sessions, _, _, err := svc.flushDemoWebAnalytics(ctx, "ws-1", newGen(), now)
		require.ErrorContains(t, err, "failed to write demo web analytics for 2026-05:")
		assert.ErrorIs(t, err, sentinel)
		assert.Equal(t, 0, sessions, "nothing was committed for the failed month")
	})
}

func TestDemoContactIdentities(t *testing.T) {
	ctx := context.Background()

	newSvc := func(t *testing.T) (*DemoService, *mocks.MockAuthService, *mocks.MockContactRepository) {
		ctrl := gomock.NewController(t)
		authService := mocks.NewMockAuthService(ctrl)
		contactRepo := mocks.NewMockContactRepository(ctrl)
		contactService := NewContactService(
			contactRepo,
			mocks.NewMockWorkspaceRepository(ctrl),
			authService,
			mocks.NewMockMessageHistoryRepository(ctrl),
			mocks.NewMockInboundWebhookEventRepository(ctrl),
			mocks.NewMockContactListRepository(ctrl),
			mocks.NewMockContactTimelineRepository(ctrl),
			nil,
			mocks.NewMockEmailQueueRepository(ctrl),
			mocks.NewMockCustomEventRepository(ctrl),
			mocks.NewMockSegmentRepository(ctrl),
			mocks.NewMockContactSegmentQueueRepository(ctrl),
			logger.NewLoggerWithLevel("disabled"),
		)
		return &DemoService{
			logger:         logger.NewLoggerWithLevel("disabled"),
			contactService: contactService,
		}, authService, contactRepo
	}

	t.Run("contacts become identities with UTC creation dates, blanks skipped", func(t *testing.T) {
		svc, authService, contactRepo := newSvc(t)
		zone := time.FixedZone("UTC+2", 2*3600)
		created := time.Date(2026, 1, 10, 12, 0, 0, 0, zone)
		authService.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), "ws-1").
			Return(ctx, &domain.User{ID: "u1"}, &domain.UserWorkspace{Role: "owner"}, nil).Times(1)
		contactRepo.EXPECT().GetContacts(gomock.Any(), gomock.Any()).
			Return(&domain.GetContactsResponse{Contacts: []*domain.Contact{
				{Email: "a@example.com", CreatedAt: created},
				{Email: "", CreatedAt: created},
				{Email: "b@example.com", CreatedAt: created},
			}}, nil).Times(1)

		identities, err := svc.demoContactIdentities(ctx, "ws-1")
		require.NoError(t, err)
		require.Len(t, identities, 2, "the blank address is not a visitor")
		assert.Equal(t, "a@example.com", identities[0].Email)
		assert.Equal(t, time.UTC, identities[0].KnownSince.Location())
		assert.Equal(t, 10, identities[0].KnownSince.Hour(), "12:00 at UTC+2 is 10:00 UTC")
	})

	t.Run("contact listing failure propagates", func(t *testing.T) {
		svc, authService, _ := newSvc(t)
		sentinel := errors.New("auth backend down")
		authService.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), "ws-1").
			Return(nil, nil, nil, sentinel).Times(1)

		_, err := svc.demoContactIdentities(ctx, "ws-1")
		require.Error(t, err)
	})
}
