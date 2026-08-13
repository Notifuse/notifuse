package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/Notifuse/notifuse/pkg/geoip"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/Notifuse/notifuse/pkg/ratelimiter"
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

	// Every successful flush projects now, and these tests set their own
	// FlushBatch expectations; this is a different method, so it cannot shadow
	// them.
	webRepo.EXPECT().ProjectContactNavigation(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil).AnyTimes()
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

// newWebAnalyticsIdentityService builds the service with the collaborators the
// identify() path actually uses: a contact repository and a rate limiter.
// newWebAnalyticsServiceForTest passes nil for both, which is why the identity
// path had no unit coverage at all before contact creation was added to it.
func newWebAnalyticsIdentityService(t *testing.T, settings *domain.WebAnalyticsSettings, secretKey string) (*WebAnalyticsService, *mocks.MockContactRepository, *ratelimiter.RateLimiter, *WebAnalyticsBuffer, *stampedIdentity) {
	t.Helper()
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	workspaceRepo.EXPECT().GetByID(gomock.Any(), "ws1").
		Return(&domain.Workspace{ID: "ws1", Settings: domain.WorkspaceSettings{
			WebAnalytics: settings,
			SecretKey:    secretKey,
		}}, nil).AnyTimes()

	contactRepo := mocks.NewMockContactRepository(ctrl)
	limiter := ratelimiter.NewRateLimiter()
	limiter.SetPolicy(webIdentifyEmailLimit, 1000, time.Minute)
	limiter.SetPolicy(webIdentifyIPLimit, 1000, time.Minute)
	limiter.SetPolicy(webIdentifyCreateLimit, 1000, time.Minute)

	// Capture what actually reached the buffer. Without this the "costs the
	// identity, never the beat" subtests can only assert the second half — and a
	// resolveContactIdentity that returned the email even when creation failed
	// would pass every one of them.
	stamped := &stampedIdentity{}
	webRepo := mocks.NewMockWebAnalyticsRepository(ctrl)
	webRepo.EXPECT().FlushBatch(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _ string, sessions []*domain.WebSession, _ []*domain.WebPage, _ []*domain.WebGoal) error {
			for _, session := range sessions {
				stamped.sessions = append(stamped.sessions, session)
			}
			return nil
		}).AnyTimes()
	// The projection now runs on every flush — there is no opt-in to withhold it.
	webRepo.EXPECT().ProjectContactNavigation(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil).AnyTimes()
	buffer := NewWebAnalyticsBuffer(webRepo, logger.NewLogger(), WebAnalyticsBufferConfig{})

	svc := NewWebAnalyticsService(workspaceRepo, contactRepo, buffer, &fakeGeoLookup{},
		mocks.NewMockAuthService(ctrl), mocks.NewMockTaskRepository(ctrl), limiter, logger.NewLogger())
	return svc, contactRepo, limiter, buffer, stamped
}

// stampedIdentity records the sessions handed to the repository so a test can
// assert whether the beat carried an identity.
type stampedIdentity struct {
	sessions []*domain.WebSession
}

// only returns the single session this beat produced, failing loudly rather than
// silently asserting against nothing.
func (s *stampedIdentity) only(t *testing.T) *domain.WebSession {
	t.Helper()
	require.Len(t, s.sessions, 1, "expected exactly one session to reach the repository")
	return s.sessions[0]
}

// TestWebAnalyticsIdentityCreatesContact covers the reversal of the original
// "known contacts only" gate: a signature proves the customer's own server
// vouched for the address, so an address it names becomes a contact.
//
// Every failure mode here must cost the IDENTITY at most, never the beat — a
// visitor's pageview is not the customer's to lose over a throttle or a database
// blip.
func TestWebAnalyticsIdentityCreatesContact(t *testing.T) {
	const secretKey = "workspace-secret-key-for-identify"
	settings := &domain.WebAnalyticsSettings{Enabled: true, Filters: domain.DefaultWebFilters()}
	receivedAt := time.Now().UTC()

	identifiedPayload := func(t *testing.T, email string) *domain.WebTrackPayload {
		t.Helper()
		payload := webTrackTestPayload(t, receivedAt)
		hmac := domain.ComputeWebIdentifyHMAC(email, secretKey)
		payload.ContactEmail = &email
		payload.ContactEmailHMAC = &hmac
		return payload
	}

	t.Run("an unknown verified address is created and identifies the beat", func(t *testing.T) {
		svc, contactRepo, _, buffer, stamped := newWebAnalyticsIdentityService(t, settings, secretKey)
		payload := identifiedPayload(t, "visitor@example.com")

		contactRepo.EXPECT().GetContactByEmail(gomock.Any(), "ws1", "visitor@example.com").
			Return(nil, domain.ErrContactNotFound).Times(1)
		var created *domain.Contact
		contactRepo.EXPECT().CreateContactIfAbsent(gomock.Any(), "ws1", gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, c *domain.Contact) (bool, error) {
				created = c
				return true, nil
			}).Times(1)

		require.NoError(t, svc.Track(context.Background(), payload, domain.WebRequestMeta{ReceivedAt: receivedAt}))
		require.NotNil(t, created, "the address must be created, not dropped")
		assert.Equal(t, "visitor@example.com", created.Email)

		buffer.FlushAll(context.Background())
		session := stamped.only(t)
		require.NotNil(t, session.ContactEmail, "the beat must carry the identity it just created")
		assert.Equal(t, "visitor@example.com", *session.ContactEmail)
	})

	t.Run("an existing contact is never modified", func(t *testing.T) {
		svc, contactRepo, _, _, _ := newWebAnalyticsIdentityService(t, settings, secretKey)
		payload := identifiedPayload(t, "known@example.com")

		contactRepo.EXPECT().GetContactByEmail(gomock.Any(), "ws1", "known@example.com").
			Return(&domain.Contact{Email: "known@example.com"}, nil).Times(1)
		// No CreateContactIfAbsent and no UpsertContact expectation: gomock fails
		// the test if either is called. A beat must never rewrite a stored profile.

		require.NoError(t, svc.Track(context.Background(), payload, domain.WebRequestMeta{ReceivedAt: receivedAt}))
	})

	t.Run("a throttled creation costs the identity, never the beat", func(t *testing.T) {
		svc, contactRepo, limiter, buffer, stamped := newWebAnalyticsIdentityService(t, settings, secretKey)
		limiter.SetPolicy(webIdentifyCreateLimit, 0, time.Minute)
		payload := identifiedPayload(t, "flood@example.com")

		contactRepo.EXPECT().GetContactByEmail(gomock.Any(), "ws1", "flood@example.com").
			Return(nil, domain.ErrContactNotFound).AnyTimes()
		contactRepo.EXPECT().CreateContactIfAbsent(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

		require.NoError(t, svc.Track(context.Background(), payload, domain.WebRequestMeta{ReceivedAt: receivedAt}),
			"a throttle must not turn into a rejected beat the SDK would retry")

		buffer.FlushAll(context.Background())
		assert.Nil(t, stamped.only(t).ContactEmail,
			"a session must never be stamped with an address the database does not hold")
	})

	t.Run("a failed creation does not identify the session", func(t *testing.T) {
		svc, contactRepo, _, buffer, stamped := newWebAnalyticsIdentityService(t, settings, secretKey)
		payload := identifiedPayload(t, "broken@example.com")

		contactRepo.EXPECT().GetContactByEmail(gomock.Any(), "ws1", "broken@example.com").
			Return(nil, domain.ErrContactNotFound).Times(1)
		contactRepo.EXPECT().CreateContactIfAbsent(gomock.Any(), "ws1", gomock.Any()).
			Return(false, errors.New("connection reset")).Times(1)

		require.NoError(t, svc.Track(context.Background(), payload, domain.WebRequestMeta{ReceivedAt: receivedAt}))

		buffer.FlushAll(context.Background())
		assert.Nil(t, stamped.only(t).ContactEmail,
			"a failed create must leave the session anonymous, not name a contact that does not exist")
	})

	t.Run("an over-long seed field is dropped rather than failing the insert", func(t *testing.T) {
		// contacts.language is VARCHAR(50) and nothing upstream bounds the
		// browser's reported language, so storing it verbatim would make the
		// INSERT fail — and because the contact is then never created, every
		// later beat from that visitor retries and fails identically. A silent,
		// permanent identity outage for one address, in exchange for a field.
		svc, contactRepo, _, buffer, stamped := newWebAnalyticsIdentityService(t, settings, secretKey)
		payload := identifiedPayload(t, "verbose@example.com")
		payload.Attributes.Language = strings.Repeat("x", 51)
		// Note this is rejected by the supported-language allowlist, not by a
		// width check — there is no longer a width check for language, and a
		// test that passes for a reason other than its name is worth naming.
		payload.Attributes.Timezone = "Europe/Paris"

		contactRepo.EXPECT().GetContactByEmail(gomock.Any(), "ws1", "verbose@example.com").
			Return(nil, domain.ErrContactNotFound).Times(1)
		var created *domain.Contact
		contactRepo.EXPECT().CreateContactIfAbsent(gomock.Any(), "ws1", gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, c *domain.Contact) (bool, error) {
				created = c
				return true, nil
			}).Times(1)

		require.NoError(t, svc.Track(context.Background(), payload, domain.WebRequestMeta{ReceivedAt: receivedAt}))
		require.NotNil(t, created)
		assert.Nil(t, created.Language, "an unstorable language must be dropped, not truncated or passed through")
		require.NotNil(t, created.Timezone, "the fields that do fit are still seeded")
		assert.Equal(t, "Europe/Paris", created.Timezone.String)

		buffer.FlushAll(context.Background())
		session := stamped.only(t)
		require.NotNil(t, session.ContactEmail, "the visitor is still identified")
	})

	t.Run("a regional language tag keeps its region when the product supports it", func(t *testing.T) {
		// SupportedLanguages carries both "pt" and "pt-BR". Cutting at the hyphen
		// unconditionally stored "pt", which no pt-BR template translation can
		// ever match — the exact failure the seeding exists to avoid.
		svc, contactRepo, _, _, _ := newWebAnalyticsIdentityService(t, settings, secretKey)
		payload := identifiedPayload(t, "brasileira@example.com")
		payload.Attributes.Language = "pt-BR"

		contactRepo.EXPECT().GetContactByEmail(gomock.Any(), "ws1", "brasileira@example.com").
			Return(nil, domain.ErrContactNotFound).Times(1)
		var created *domain.Contact
		contactRepo.EXPECT().CreateContactIfAbsent(gomock.Any(), "ws1", gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, c *domain.Contact) (bool, error) {
				created = c
				return true, nil
			}).Times(1)

		require.NoError(t, svc.Track(context.Background(), payload, domain.WebRequestMeta{ReceivedAt: receivedAt}))
		require.NotNil(t, created)
		require.NotNil(t, created.Language)
		assert.Equal(t, "pt-BR", created.Language.String)
	})

	t.Run("an unsupported language is dropped rather than stored", func(t *testing.T) {
		svc, contactRepo, _, _, _ := newWebAnalyticsIdentityService(t, settings, secretKey)
		payload := identifiedPayload(t, "klingon@example.com")
		payload.Attributes.Language = "tlh"

		contactRepo.EXPECT().GetContactByEmail(gomock.Any(), "ws1", "klingon@example.com").
			Return(nil, domain.ErrContactNotFound).Times(1)
		var created *domain.Contact
		contactRepo.EXPECT().CreateContactIfAbsent(gomock.Any(), "ws1", gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, c *domain.Contact) (bool, error) {
				created = c
				return true, nil
			}).Times(1)

		require.NoError(t, svc.Track(context.Background(), payload, domain.WebRequestMeta{ReceivedAt: receivedAt}))
		require.NotNil(t, created)
		assert.Nil(t, created.Language)
	})

	t.Run("a timezone that is not a real zone is dropped", func(t *testing.T) {
		// "Local" is what time.LoadLocation accepts and an allowlist does not: it
		// is not a zone, it is whatever the server's own is. Stored on a contact
		// it would be a name nothing else in the product recognises — the
		// console's picker, the send-time scheduler and every template that
		// formats a date all expect an IANA name.
		svc, contactRepo, _, _, _ := newWebAnalyticsIdentityService(t, settings, secretKey)
		payload := identifiedPayload(t, "zoned@example.com")
		payload.Attributes.Timezone = "Local"

		contactRepo.EXPECT().GetContactByEmail(gomock.Any(), "ws1", "zoned@example.com").
			Return(nil, domain.ErrContactNotFound).Times(1)
		var created *domain.Contact
		contactRepo.EXPECT().CreateContactIfAbsent(gomock.Any(), "ws1", gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, c *domain.Contact) (bool, error) {
				created = c
				return true, nil
			}).Times(1)

		require.NoError(t, svc.Track(context.Background(), payload, domain.WebRequestMeta{ReceivedAt: receivedAt}))
		require.NotNil(t, created)
		assert.Nil(t, created.Timezone, "a name the allowlist rejects must not reach the contact")
	})

	t.Run("a real IANA timezone is kept", func(t *testing.T) {
		// The counterpart, so the guard cannot pass by rejecting everything.
		svc, contactRepo, _, _, _ := newWebAnalyticsIdentityService(t, settings, secretKey)
		payload := identifiedPayload(t, "parisian@example.com")
		payload.Attributes.Timezone = "Europe/Paris"

		contactRepo.EXPECT().GetContactByEmail(gomock.Any(), "ws1", "parisian@example.com").
			Return(nil, domain.ErrContactNotFound).Times(1)
		var created *domain.Contact
		contactRepo.EXPECT().CreateContactIfAbsent(gomock.Any(), "ws1", gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, c *domain.Contact) (bool, error) {
				created = c
				return true, nil
			}).Times(1)

		require.NoError(t, svc.Track(context.Background(), payload, domain.WebRequestMeta{ReceivedAt: receivedAt}))
		require.NotNil(t, created)
		require.NotNil(t, created.Timezone)
		assert.Equal(t, "Europe/Paris", created.Timezone.String)
	})

	t.Run("a forged signature creates nothing and never reaches the database", func(t *testing.T) {
		svc, contactRepo, _, buffer, stamped := newWebAnalyticsIdentityService(t, settings, secretKey)
		payload := identifiedPayload(t, "victim@example.com")
		forged := "00000000000000000000000000000000000000000000000000000000000000ff"
		payload.ContactEmailHMAC = &forged

		contactRepo.EXPECT().GetContactByEmail(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
		contactRepo.EXPECT().CreateContactIfAbsent(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

		require.NoError(t, svc.Track(context.Background(), payload, domain.WebRequestMeta{ReceivedAt: receivedAt}))

		buffer.FlushAll(context.Background())
		assert.Nil(t, stamped.only(t).ContactEmail, "a forged signature must not identify anyone")
	})
}
