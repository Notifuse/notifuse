package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/Notifuse/notifuse/pkg/logger"
)

// newDemoAnnotationService builds a DemoService with only what the annotation
// seeding touches.
func newDemoAnnotationService(t *testing.T, timezone string) (
	*DemoService,
	*mocks.MockAnnotationRepository,
	*mocks.MockBroadcastRepository,
) {
	t.Helper()
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	annotationRepo := mocks.NewMockAnnotationRepository(ctrl)
	broadcastRepo := mocks.NewMockBroadcastRepository(ctrl)
	workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)

	workspaceRepo.EXPECT().GetByID(gomock.Any(), "demo").
		Return(&domain.Workspace{
			ID:       "demo",
			Settings: domain.WorkspaceSettings{Timezone: timezone},
		}, nil).AnyTimes()

	return &DemoService{
		logger:         logger.NewLoggerWithLevel("disabled"),
		annotationRepo: annotationRepo,
		broadcastRepo:  broadcastRepo,
		workspaceRepo:  workspaceRepo,
	}, annotationRepo, broadcastRepo
}

func demoBroadcast(id, name string, startedAt *time.Time) *domain.Broadcast {
	return &domain.Broadcast{ID: id, Name: name, StartedAt: startedAt}
}

func TestDemoLaunchAnnotation(t *testing.T) {
	now := time.Date(2026, 8, 15, 13, 45, 12, 0, time.UTC)

	t.Run("lands on the day the traffic generator spikes", func(t *testing.T) {
		// The annotation is only worth anything if it sits on the spike. Both sides
		// read demoLaunchDaysAgo, and this is what keeps them reading it the same
		// way: change the constant, or the generator's window arithmetic, and this
		// fails instead of quietly sliding the marker onto a flat day.
		generator := newDemoWebAnalyticsGenerator(demoWebAnalyticsOptions{
			Sessions: 1000,
			Days:     demoWebAnalyticsDays,
			Now:      now,
			Seed:     demoWebAnalyticsSeed,
			SiteURL:  demoWebAnalyticsSite,
		})

		assert.Equal(t, "launch", generator.launchPeriod(generator.launchIndex),
			"the pinned index is not the generator's launch day")
		assert.True(t, generator.DayStart(generator.launchIndex).Equal(demoLaunchDay(now)),
			"annotation at %s, spike at %s",
			demoLaunchDay(now), generator.DayStart(generator.launchIndex))
	})

	t.Run("is exactly demoLaunchDaysAgo days before the run's own midnight", func(t *testing.T) {
		launch := demoLaunchDay(now)
		elapsed := now.Truncate(24 * time.Hour).Sub(launch)

		assert.Equal(t, demoLaunchDaysAgo, int(elapsed.Hours()/24))
		assert.Equal(t, time.UTC, launch.Location())
		assert.Equal(t, 0, launch.Hour())
		assert.Equal(t, 0, launch.Minute())
	})

	t.Run("normalises a non-UTC clock to the same instant", func(t *testing.T) {
		tokyo, err := time.LoadLocation("Asia/Tokyo")
		require.NoError(t, err)

		assert.True(t, demoLaunchDay(now.In(tokyo)).Equal(demoLaunchDay(now)))
	})

	t.Run("writes one manual annotation with no source id", func(t *testing.T) {
		service, annotationRepo, broadcastRepo := newDemoAnnotationService(t, "Europe/Paris")

		var written *domain.Annotation
		annotationRepo.EXPECT().Create(gomock.Any(), "demo", gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, annotation *domain.Annotation) error {
				written = annotation
				return nil
			})
		broadcastRepo.EXPECT().ListBroadcasts(gomock.Any(), gomock.Any()).
			Return(&domain.BroadcastListResponse{}, nil)

		require.NoError(t, service.seedAnnotations(context.Background(), "demo", now))
		require.NotNil(t, written)

		assert.Equal(t, domain.AnnotationSourceManual, written.Source)
		assert.Nil(t, written.SourceID)
		assert.Equal(t, demoLaunchAnnotationTitle, written.Title)
		assert.Equal(t, demoLaunchAnnotationColor, written.Color)
		assert.Equal(t, "Europe/Paris", written.Timezone)
		assert.True(t, written.AnnotatedAt.Equal(demoLaunchDay(now)))
		assert.NotEmpty(t, written.ID)
		assert.NoError(t, written.Validate())

		// The description has to describe what the reader will actually see on that
		// day, so it is built from the generator's own constants.
		assert.Contains(t, written.Description, "iphone-launch-2024")
		assert.Contains(t, written.Description, "/iphone-17")
	})

	t.Run("falls back to UTC when the workspace cannot be read", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		t.Cleanup(ctrl.Finish)

		workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
		workspaceRepo.EXPECT().GetByID(gomock.Any(), "demo").Return(nil, errors.New("gone"))

		service := &DemoService{
			logger:        logger.NewLoggerWithLevel("disabled"),
			workspaceRepo: workspaceRepo,
		}

		assert.Equal(t, "UTC", service.demoAnnotationTimezone(context.Background(), "demo"))
	})
}

func TestDemoBroadcastAnnotations(t *testing.T) {
	startedAt := time.Date(2026, 8, 10, 9, 0, 0, 0, time.UTC)
	otherStartedAt := startedAt.AddDate(0, 0, 2)

	t.Run("annotates every broadcast that started", func(t *testing.T) {
		testCases := []struct {
			name       string
			broadcasts []*domain.Broadcast
			// expected is keyed by source_id, which is what the console uses to tie an
			// annotation back to its broadcast.
			expected map[string]string
		}{
			{
				name: "one annotation per broadcast",
				broadcasts: []*domain.Broadcast{
					demoBroadcast("bc1", "Weekly Newsletter #1", &startedAt),
					demoBroadcast("bc2", "Weekly Newsletter #2", &otherStartedAt),
				},
				expected: map[string]string{
					"bc1": "Weekly Newsletter #1",
					"bc2": "Weekly Newsletter #2",
				},
			},
			{
				name: "a broadcast that never started is skipped",
				broadcasts: []*domain.Broadcast{
					demoBroadcast("bc1", "Weekly Newsletter #1", &startedAt),
					demoBroadcast("bc2", "Draft never sent", nil),
				},
				expected: map[string]string{"bc1": "Weekly Newsletter #1"},
			},
			{
				name: "an unnamed broadcast still gets its marker",
				broadcasts: []*domain.Broadcast{
					demoBroadcast("bc1", "", &startedAt),
				},
				expected: map[string]string{"bc1": "Broadcast"},
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				service, annotationRepo, broadcastRepo := newDemoAnnotationService(t, "UTC")

				broadcastRepo.EXPECT().ListBroadcasts(gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, params domain.ListBroadcastsParams) (*domain.BroadcastListResponse, error) {
						assert.Equal(t, "demo", params.WorkspaceID)
						return &domain.BroadcastListResponse{Broadcasts: tc.broadcasts}, nil
					})

				written := map[string]*domain.Annotation{}
				annotationRepo.EXPECT().CreateFromSource(gomock.Any(), "demo", gomock.Any()).
					DoAndReturn(func(_ context.Context, _ string, annotation *domain.Annotation) (bool, error) {
						require.NotNil(t, annotation.SourceID)
						written[*annotation.SourceID] = annotation
						return true, nil
					}).AnyTimes()

				require.NoError(t, service.seedBroadcastAnnotations(context.Background(), "demo", "UTC"))
				require.Len(t, written, len(tc.expected))

				for sourceID, title := range tc.expected {
					annotation, ok := written[sourceID]
					require.True(t, ok, "no annotation for broadcast %s", sourceID)
					assert.Equal(t, title, annotation.Title)
					assert.Equal(t, domain.AnnotationSourceBroadcast, annotation.Source)
					assert.Equal(t, domain.AnnotationBroadcastColor, annotation.Color)
					assert.NoError(t, annotation.Validate())
				}
			})
		}
	})

	t.Run("marks the broadcast at the instant it started", func(t *testing.T) {
		service, annotationRepo, broadcastRepo := newDemoAnnotationService(t, "UTC")

		broadcastRepo.EXPECT().ListBroadcasts(gomock.Any(), gomock.Any()).
			Return(&domain.BroadcastListResponse{
				Broadcasts: []*domain.Broadcast{demoBroadcast("bc1", "Weekly Newsletter #1", &startedAt)},
			}, nil)

		var written *domain.Annotation
		annotationRepo.EXPECT().CreateFromSource(gomock.Any(), "demo", gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, annotation *domain.Annotation) (bool, error) {
				written = annotation
				return true, nil
			})

		require.NoError(t, service.seedBroadcastAnnotations(context.Background(), "demo", "Asia/Tokyo"))
		require.NotNil(t, written)
		assert.True(t, written.AnnotatedAt.Equal(startedAt))
		assert.Equal(t, "Asia/Tokyo", written.Timezone)
	})

	t.Run("truncates a long name by runes, not bytes", func(t *testing.T) {
		// Every rune here is three bytes, so a byte-based cut would keep a third of
		// the characters the column accepts and could split a rune in half.
		longName := strings.Repeat("é", domain.AnnotationMaxTitleLength+40)
		require.Greater(t, len(longName), domain.AnnotationMaxTitleLength)

		title := demoAnnotationTitle(longName)

		assert.Equal(t, domain.AnnotationMaxTitleLength, utf8.RuneCountInString(title))
		assert.True(t, utf8.ValidString(title), "truncation cut a rune in half")
		assert.NoError(t, (&domain.Annotation{
			AnnotatedAt: startedAt,
			Timezone:    "UTC",
			Title:       title,
			Color:       domain.AnnotationBroadcastColor,
			Source:      domain.AnnotationSourceBroadcast,
		}).Validate())
	})

	t.Run("a short name is left alone", func(t *testing.T) {
		assert.Equal(t, "Weekly Newsletter #1", demoAnnotationTitle("Weekly Newsletter #1"))
	})
}

func TestDemoAnnotationsSeedingIsNonFatal(t *testing.T) {
	now := time.Date(2026, 8, 15, 13, 45, 12, 0, time.UTC)
	startedAt := time.Date(2026, 8, 10, 9, 0, 0, 0, time.UTC)

	t.Run("a failed broadcast annotation does not cost the others", func(t *testing.T) {
		service, annotationRepo, broadcastRepo := newDemoAnnotationService(t, "UTC")

		broadcastRepo.EXPECT().ListBroadcasts(gomock.Any(), gomock.Any()).
			Return(&domain.BroadcastListResponse{
				Broadcasts: []*domain.Broadcast{
					demoBroadcast("bc1", "Weekly Newsletter #1", &startedAt),
					demoBroadcast("bc2", "Weekly Newsletter #2", &startedAt),
				},
			}, nil)

		attempted := []string{}
		annotationRepo.EXPECT().CreateFromSource(gomock.Any(), "demo", gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, annotation *domain.Annotation) (bool, error) {
				attempted = append(attempted, *annotation.SourceID)
				if *annotation.SourceID == "bc1" {
					return false, errors.New("write failed")
				}
				return true, nil
			}).Times(2)

		require.NoError(t, service.seedBroadcastAnnotations(context.Background(), "demo", "UTC"))
		assert.Equal(t, []string{"bc1", "bc2"}, attempted)
	})

	t.Run("a failed launch annotation still lets the broadcasts be annotated", func(t *testing.T) {
		service, annotationRepo, broadcastRepo := newDemoAnnotationService(t, "UTC")

		annotationRepo.EXPECT().Create(gomock.Any(), "demo", gomock.Any()).
			Return(errors.New("write failed"))
		broadcastRepo.EXPECT().ListBroadcasts(gomock.Any(), gomock.Any()).
			Return(&domain.BroadcastListResponse{
				Broadcasts: []*domain.Broadcast{demoBroadcast("bc1", "Weekly Newsletter #1", &startedAt)},
			}, nil)
		annotationRepo.EXPECT().CreateFromSource(gomock.Any(), "demo", gomock.Any()).Return(true, nil)

		// The error is reported for the caller to log; addSampleData warns and keeps
		// going, so a failing annotation write can never fail a demo reset.
		err := service.seedAnnotations(context.Background(), "demo", now)
		assert.Error(t, err)
	})

	t.Run("a listing failure is reported without touching the launch annotation", func(t *testing.T) {
		service, annotationRepo, broadcastRepo := newDemoAnnotationService(t, "UTC")

		annotationRepo.EXPECT().Create(gomock.Any(), "demo", gomock.Any()).Return(nil)
		broadcastRepo.EXPECT().ListBroadcasts(gomock.Any(), gomock.Any()).
			Return(nil, errors.New("connection lost"))

		err := service.seedAnnotations(context.Background(), "demo", now)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "list demo broadcasts")
	})

	t.Run("an unconfigured repository is reported, not panicked on", func(t *testing.T) {
		service := &DemoService{logger: logger.NewLoggerWithLevel("disabled")}

		assert.Error(t, service.seedAnnotations(context.Background(), "demo", now))
	})
}
