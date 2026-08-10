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
	"github.com/Notifuse/notifuse/pkg/geoip"
	"github.com/Notifuse/notifuse/pkg/logger"
)

type fakeGeoLookup struct {
	result geoip.Result
	err    error
	calls  int
}

func (f *fakeGeoLookup) Lookup(string) (geoip.Result, error) {
	f.calls++
	return f.result, f.err
}

func newWebAnalyticsServiceForTest(t *testing.T, settings *domain.WebAnalyticsSettings) (*WebAnalyticsService, *mocks.MockWorkspaceRepository, *mocks.MockWebAnalyticsRepository, *fakeGeoLookup) {
	t.Helper()
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	webRepo := mocks.NewMockWebAnalyticsRepository(ctrl)
	geo := &fakeGeoLookup{}

	if settings != nil {
		workspaceRepo.EXPECT().GetByID(gomock.Any(), "ws1").
			Return(&domain.Workspace{ID: "ws1", Settings: domain.WorkspaceSettings{WebAnalytics: settings}}, nil).
			Times(1) // the 60s cache must absorb repeat lookups
	}

	buffer := NewWebAnalyticsBuffer(webRepo, logger.NewLogger(), WebAnalyticsBufferConfig{})
	svc := NewWebAnalyticsService(workspaceRepo, nil, buffer, geo, mocks.NewMockAuthService(ctrl), mocks.NewMockTaskRepository(ctrl), nil, logger.NewLogger())
	return svc, workspaceRepo, webRepo, geo
}

func webTrackTestPayload(t *testing.T, receivedAt time.Time) *domain.WebTrackPayload {
	t.Helper()
	sentAt := receivedAt.UnixMilli()
	return &domain.WebTrackPayload{
		WorkspaceID: "ws1",
		SessionID:   testUUIDv7At(receivedAt.Add(-time.Minute)),
		Seq:         1,
		CreatedAt:   receivedAt.Add(-time.Minute).UnixMilli(),
		UpdatedAt:   receivedAt.UnixMilli(),
		SentAt:      &sentAt,
		Attributes:  &domain.WebSessionAttributes{LandingPage: "https://shop.example.com/"},
		Actions: []domain.WebTrackAction{
			{Type: "pageview", Path: "/", PageNumber: 1, Duration: 500,
				EnteredAt: receivedAt.Add(-time.Minute).UnixMilli(), ExitedAt: receivedAt.UnixMilli()},
		},
	}
}

func TestWebAnalyticsServiceTrack(t *testing.T) {
	receivedAt := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	meta := domain.WebRequestMeta{
		Origin:     "https://shop.example.com",
		UserAgent:  "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) Chrome/126.0",
		ClientIP:   "203.0.113.10",
		ReceivedAt: receivedAt,
	}

	t.Run("happy path buffers the beat and caches workspace settings", func(t *testing.T) {
		svc, _, _, geo := newWebAnalyticsServiceForTest(t, &domain.WebAnalyticsSettings{Enabled: true, GeoEnabled: true})

		require.NoError(t, svc.Track(context.Background(), webTrackTestPayload(t, receivedAt), meta))
		assert.Equal(t, 1, svc.buffer.PendingSessions("ws1"))
		assert.Equal(t, 1, geo.calls)

		// Second beat: GetByID must NOT be called again (Times(1) enforces it).
		payload := webTrackTestPayload(t, receivedAt)
		payload.Seq = 2
		require.NoError(t, svc.Track(context.Background(), payload, meta))
	})

	t.Run("disabled feature drops silently", func(t *testing.T) {
		svc, _, _, _ := newWebAnalyticsServiceForTest(t, &domain.WebAnalyticsSettings{Enabled: false})
		require.NoError(t, svc.Track(context.Background(), webTrackTestPayload(t, receivedAt), meta))
		assert.Zero(t, svc.buffer.PendingSessions("ws1"))
	})

	t.Run("unknown workspace drops silently and caches the miss", func(t *testing.T) {
		svc, workspaceRepo, _, _ := newWebAnalyticsServiceForTest(t, nil)
		workspaceRepo.EXPECT().GetByID(gomock.Any(), "ws1").Return(nil, errors.New("not found")).Times(1)

		require.NoError(t, svc.Track(context.Background(), webTrackTestPayload(t, receivedAt), meta))
		require.NoError(t, svc.Track(context.Background(), webTrackTestPayload(t, receivedAt), meta))
		assert.Zero(t, svc.buffer.PendingSessions("ws1"))
	})

	t.Run("allowed domains: origin wildcard matrix", func(t *testing.T) {
		settings := &domain.WebAnalyticsSettings{Enabled: true, AllowedDomains: []string{"*.example.com"}}

		cases := []struct {
			name     string
			meta     domain.WebRequestMeta
			buffered int
		}{
			{"subdomain origin allowed", domain.WebRequestMeta{Origin: "https://shop.example.com", ReceivedAt: receivedAt}, 1},
			{"apex origin allowed", domain.WebRequestMeta{Origin: "https://example.com", ReceivedAt: receivedAt}, 1},
			{"foreign origin rejected silently", domain.WebRequestMeta{Origin: "https://evil.io", ReceivedAt: receivedAt}, 0},
			{"referer fallback when origin missing", domain.WebRequestMeta{Referer: "https://app.example.com/page", ReceivedAt: receivedAt}, 1},
			{"no origin nor referer rejected", domain.WebRequestMeta{ReceivedAt: receivedAt}, 0},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				svc, _, _, _ := newWebAnalyticsServiceForTest(t, settings)
				require.NoError(t, svc.Track(context.Background(), webTrackTestPayload(t, receivedAt), tc.meta))
				assert.Equal(t, tc.buffered, svc.buffer.PendingSessions("ws1"))
			})
		}
	})

	t.Run("invalid payload returns the typed error", func(t *testing.T) {
		svc, _, _, _ := newWebAnalyticsServiceForTest(t, &domain.WebAnalyticsSettings{Enabled: true})
		payload := webTrackTestPayload(t, receivedAt)
		payload.SessionID = "not-a-uuid"

		err := svc.Track(context.Background(), payload, meta)
		var invalid *ErrWebTrackInvalidPayload
		require.ErrorAs(t, err, &invalid)
		assert.Zero(t, svc.buffer.PendingSessions("ws1"))
	})

	t.Run("empty actions accepted without buffering", func(t *testing.T) {
		svc, _, _, _ := newWebAnalyticsServiceForTest(t, &domain.WebAnalyticsSettings{Enabled: true})
		payload := webTrackTestPayload(t, receivedAt)
		payload.Actions = nil
		require.NoError(t, svc.Track(context.Background(), payload, meta))
		assert.Zero(t, svc.buffer.PendingSessions("ws1"))
	})

	t.Run("request user agent fills missing attribute", func(t *testing.T) {
		svc, _, webRepo, _ := newWebAnalyticsServiceForTest(t, &domain.WebAnalyticsSettings{Enabled: true})
		payload := webTrackTestPayload(t, receivedAt)
		payload.Attributes.UserAgent = ""

		require.NoError(t, svc.Track(context.Background(), payload, meta))

		webRepo.EXPECT().FlushBatch(gomock.Any(), "ws1", gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, sessions []*domain.WebSession, _ []*domain.WebPage, _ []*domain.WebGoal) error {
				require.Len(t, sessions, 1)
				assert.Equal(t, meta.UserAgent, sessions[0].UserAgent)
				// The SDK parses device/browser/OS in the browser (Client
				// Hints); the server no longer re-parses the UA string, so a
				// payload without those fields yields the defaults.
				assert.Equal(t, "Unknown", sessions[0].OS)
				assert.Equal(t, "desktop", sessions[0].Device)
				return nil
			})
		svc.buffer.FlushAll(context.Background())
	})

	t.Run("geo lookup errors degrade to empty geo", func(t *testing.T) {
		svc, _, webRepo, geo := newWebAnalyticsServiceForTest(t, &domain.WebAnalyticsSettings{Enabled: true, GeoEnabled: true})
		geo.err = errors.New("mmdb corrupted")

		require.NoError(t, svc.Track(context.Background(), webTrackTestPayload(t, receivedAt), meta))

		webRepo.EXPECT().FlushBatch(gomock.Any(), "ws1", gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, sessions []*domain.WebSession, _ []*domain.WebPage, _ []*domain.WebGoal) error {
				assert.Empty(t, sessions[0].Country)
				return nil
			})
		svc.buffer.FlushAll(context.Background())
	})

	t.Run("cache invalidation forces a fresh workspace read", func(t *testing.T) {
		svc, workspaceRepo, _, _ := newWebAnalyticsServiceForTest(t, &domain.WebAnalyticsSettings{Enabled: true})
		require.NoError(t, svc.Track(context.Background(), webTrackTestPayload(t, receivedAt), meta))

		svc.InvalidateWorkspaceCache("ws1")
		workspaceRepo.EXPECT().GetByID(gomock.Any(), "ws1").
			Return(&domain.Workspace{ID: "ws1", Settings: domain.WorkspaceSettings{WebAnalytics: &domain.WebAnalyticsSettings{Enabled: false}}}, nil).
			Times(1)
		require.NoError(t, svc.Track(context.Background(), webTrackTestPayload(t, receivedAt), meta))
	})
}
