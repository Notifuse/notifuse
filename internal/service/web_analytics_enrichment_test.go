package service

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/geoip"
)

// testUUIDv7At builds a UUIDv7 string embedding the given timestamp.
func testUUIDv7At(ts time.Time) string {
	ms := ts.UnixMilli()
	var b [16]byte
	b[0], b[1], b[2] = byte(ms>>40), byte(ms>>32), byte(ms>>24)
	b[3], b[4], b[5] = byte(ms>>16), byte(ms>>8), byte(ms)
	b[6] = 0x71
	b[7] = 0x23
	b[8] = 0x91
	b[9] = 0x45
	for i := 10; i < 16; i++ {
		b[i] = 0xAB
	}
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func TestWebClockSkew(t *testing.T) {
	receivedAt := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	ms := func(t time.Time) *int64 { v := t.UnixMilli(); return &v }

	assert.Equal(t, time.Duration(0), webClockSkew(nil, receivedAt), "no sent_at: trust client clocks")
	assert.Equal(t, time.Duration(0), webClockSkew(ms(receivedAt.Add(-3*time.Second)), receivedAt), "within threshold: no correction")
	assert.Equal(t, time.Duration(0), webClockSkew(ms(receivedAt.Add(4*time.Second)), receivedAt))
	assert.Equal(t, 10*time.Second, webClockSkew(ms(receivedAt.Add(-10*time.Second)), receivedAt), "client 10s behind: shift forward")
	assert.Equal(t, -2*time.Minute, webClockSkew(ms(receivedAt.Add(2*time.Minute)), receivedAt), "client ahead: shift back")
}

func TestWebURLParts(t *testing.T) {
	cases := []struct {
		raw, domain, path string
	}{
		{"https://www.Example.com/pricing?utm_source=x", "www.example.com", "/pricing"},
		{"https://example.com", "example.com", "/"},
		{"https://example.com:8443/a/b", "example.com", "/a/b"},
		{"http://news.ycombinator.com/item", "news.ycombinator.com", "/item"},
		{"", "", ""},
		{"   ", "", ""},
		{"not a url", "", ""},
		{"/relative/only", "", ""},
	}
	for _, tc := range cases {
		domain, path := webURLParts(tc.raw)
		assert.Equal(t, tc.domain, domain, tc.raw)
		assert.Equal(t, tc.path, path, tc.raw)
	}
}

func TestBoundDimension(t *testing.T) {
	// Device, browser and OS are parsed in the browser (ua-parser-js with
	// Client Hints) and arrive as plain client input, so the server only
	// defaults and bounds them.
	assert.Equal(t, "desktop", boundDimension("", "desktop"), "missing value falls back")
	assert.Equal(t, "desktop", boundDimension("   ", "desktop"), "blank value falls back")
	assert.Equal(t, "", boundDimension("", ""), "no fallback stays empty")
	assert.Equal(t, "Chrome", boundDimension("Chrome", "Unknown"))
	assert.Len(t, boundDimension(strings.Repeat("x", 5000), "Unknown"), 200, "hostile length is capped")
}

func TestApplyWebGeo(t *testing.T) {
	lat, lon := 48.8566, 2.3522
	raw := geoip.Result{Country: "FR", Region: "Île-de-France", City: "Paris", Latitude: &lat, Longitude: &lon}

	t.Run("geo disabled wipes everything", func(t *testing.T) {
		out := applyWebGeo(raw, &domain.WebAnalyticsSettings{GeoEnabled: false})
		assert.Equal(t, domain.WebGeoResult{}, out)
	})

	t.Run("privacy knobs drop city/region and fuzz coordinates", func(t *testing.T) {
		out := applyWebGeo(raw, &domain.WebAnalyticsSettings{
			GeoEnabled: true, GeoStoreCity: false, GeoStoreRegion: false, GeoCoordsPrecision: 0,
		})
		assert.Equal(t, "FR", out.Country)
		assert.Empty(t, out.City)
		assert.Empty(t, out.Region)
		require.NotNil(t, out.Latitude)
		assert.InDelta(t, 49, *out.Latitude, 1e-9)
		assert.InDelta(t, 2, *out.Longitude, 1e-9)
	})

	t.Run("full precision keeps two decimals", func(t *testing.T) {
		out := applyWebGeo(raw, &domain.WebAnalyticsSettings{
			GeoEnabled: true, GeoStoreCity: true, GeoStoreRegion: true, GeoCoordsPrecision: 2,
		})
		assert.Equal(t, "Paris", out.City)
		assert.InDelta(t, 48.86, *out.Latitude, 1e-9)
	})
}

func webTestSettings() *domain.WebAnalyticsSettings {
	return &domain.WebAnalyticsSettings{
		Enabled:            true,
		GeoEnabled:         true,
		GeoStoreCity:       true,
		GeoStoreRegion:     true,
		GeoCoordsPrecision: 2,
		Filters:            domain.DefaultWebFilters(),
	}
}

func TestBuildWebRows(t *testing.T) {
	receivedAt := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	sessionStart := receivedAt.Add(-10 * time.Minute)
	sessionID := testUUIDv7At(sessionStart)

	basePayload := func() *domain.WebTrackPayload {
		sentAt := receivedAt.UnixMilli()
		return &domain.WebTrackPayload{
			WorkspaceID: "ws1",
			SessionID:   sessionID,
			Seq:         7,
			CreatedAt:   sessionStart.UnixMilli(),
			UpdatedAt:   receivedAt.Add(-2 * time.Second).UnixMilli(),
			SentAt:      &sentAt,
			SDKVersion:  "1.0.0",
			Dimensions:  map[string]string{"custom_2": "pro-plan", "ignored": "x"},
			Attributes: &domain.WebSessionAttributes{
				Referrer:    "https://www.Google.com/search",
				LandingPage: "https://shop.example.com/landing?q=1",
				UTMSource:   "google",
				UTMMedium:   "cpc",
				UserAgent:   "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 Chrome/126.0.0.0 Safari/537.36",
				// Parsed in the browser by the SDK and sent as-is.
				Device: "desktop", Browser: "Chrome", OS: "macOS",
				ScreenWidth: 2560, ScreenHeight: 1440,
				Language: "fr-FR", Timezone: "Europe/Paris",
			},
			Actions: []domain.WebTrackAction{
				{Type: "pageview", Path: "/landing", PageNumber: 1, Duration: 1000, Scroll: 30,
					EnteredAt: sessionStart.UnixMilli(), ExitedAt: sessionStart.Add(30 * time.Second).UnixMilli()},
				{Type: "pageview", Path: "/pricing", PageNumber: 2, Duration: 3000, Scroll: 80,
					EnteredAt: sessionStart.Add(30 * time.Second).UnixMilli(), ExitedAt: sessionStart.Add(2 * time.Minute).UnixMilli()},
				// 20000 makes the durations skewed (median 3000 != mean 8000),
				// so median/mean confusion cannot slip through.
				{Type: "pageview", Path: "/checkout", PageNumber: 3, Duration: 20000, Scroll: 55,
					EnteredAt: sessionStart.Add(2 * time.Minute).UnixMilli(), ExitedAt: sessionStart.Add(3 * time.Minute).UnixMilli()},
				{Type: "goal", Name: "purchase", Path: "/checkout", PageNumber: 3, Value: 49.9,
					Timestamp:  sessionStart.Add(3 * time.Minute).UnixMilli(),
					Properties: map[string]string{"plan": "pro"}},
			},
		}
	}

	t.Run("session aggregates: sum duration, median, exit, counts", func(t *testing.T) {
		session, pages, goals, err := BuildWebRows(basePayload(), webTestSettings(), geoip.Result{}, receivedAt, nil)
		require.NoError(t, err)

		assert.Equal(t, int64(7), session.BeatSeq)
		assert.Equal(t, 3, session.PageviewCount)
		assert.Equal(t, int64(24000), session.DurationMs, "duration is the SUM of page focus (decided fix), not the max (20000)")
		assert.Equal(t, int64(3000), session.MedianPageDurationMs, "median of 1000/3000/20000, not the mean (8000)")
		assert.Equal(t, 80, session.MaxScroll)
		assert.Equal(t, "/checkout", session.ExitPath)
		assert.Equal(t, 1, session.GoalCount)
		assert.InDelta(t, 49.9, session.GoalValue, 1e-9)
		// Identity is no longer carried by the beat: it arrives verified, via the
		// W2 ingest path, so a payload alone leaves the session anonymous.
		assert.Nil(t, session.ContactEmail)
		assert.Equal(t, "1.0.0", session.SDKVersion)

		require.Len(t, pages, 3)
		assert.True(t, pages[0].IsLanding)
		assert.Equal(t, domain.WebEntryTypeLanding, pages[0].EntryType)
		assert.False(t, pages[0].IsExit)
		assert.False(t, pages[1].IsExit)
		assert.True(t, pages[2].IsExit, "last page carries the exit flag")
		assert.Equal(t, domain.WebEntryTypeNavigation, pages[2].EntryType)

		require.Len(t, goals, 1)
		assert.Equal(t, "purchase", goals[0].GoalName)
		assert.Equal(t, map[string]string{"plan": "pro"}, goals[0].Properties)
	})

	t.Run("uuid drives session_date and identity; timestamps corrected only beyond skew threshold", func(t *testing.T) {
		payload := basePayload()
		// Client clock 10 minutes ahead: sent_at says "now + 10min".
		sentAt := receivedAt.Add(10 * time.Minute).UnixMilli()
		payload.SentAt = &sentAt

		session, pages, goals, err := BuildWebRows(payload, webTestSettings(), geoip.Result{}, receivedAt, nil)
		require.NoError(t, err)

		assert.Equal(t, time.Date(2026, 8, 8, 0, 0, 0, 0, time.UTC), session.SessionDate)
		// created_at shifted back by 10 minutes.
		assert.Equal(t, sessionStart.Add(-10*time.Minute).UnixMilli(), session.CreatedAt.UnixMilli())
		assert.Equal(t, pages[0].EnteredAt.UnixMilli(), sessionStart.Add(-10*time.Minute).UnixMilli())
		// The goal's dedup key keeps the ORIGINAL client timestamp; only
		// goal_at is corrected.
		assert.Equal(t, sessionStart.Add(3*time.Minute).UnixMilli(), goals[0].ClientTsMs)
		assert.Equal(t, sessionStart.Add(-7*time.Minute).UnixMilli(), goals[0].GoalAt.UnixMilli())
	})

	t.Run("attribution: URL parsing, UA parsing, filters and custom dimensions", func(t *testing.T) {
		session, _, goals, err := BuildWebRows(basePayload(), webTestSettings(), geoip.Result{}, receivedAt, nil)
		require.NoError(t, err)

		assert.Equal(t, "www.google.com", session.ReferrerDomain)
		assert.Equal(t, "/search", session.ReferrerPath)
		assert.Equal(t, "shop.example.com", session.LandingDomain)
		assert.Equal(t, "/landing", session.LandingPath)
		assert.False(t, session.IsDirect)

		assert.Equal(t, "desktop", session.Device)
		assert.Equal(t, "Chrome", session.Browser)
		assert.Equal(t, "macOS", session.OS)

		// google + cpc UTM → Google Ads via default rules.
		assert.Equal(t, "google-ads", session.Channel)
		assert.Equal(t, "search-paid", session.ChannelGroup)
		assert.Equal(t, "google-ads", goals[0].Channel, "goal snapshot carries attribution")

		assert.Equal(t, "pro-plan", session.Custom2)
		assert.Empty(t, session.Custom1)
	})

	t.Run("direct traffic and geo knobs", func(t *testing.T) {
		payload := basePayload()
		payload.Attributes.Referrer = ""
		payload.Attributes.UTMSource = ""
		payload.Attributes.UTMMedium = ""
		lat, lon := 48.8566, 2.3522
		geo := geoip.Result{Country: "FR", Region: "IDF", City: "Paris", Latitude: &lat, Longitude: &lon}

		session, _, _, err := BuildWebRows(payload, webTestSettings(), geo, receivedAt, nil)
		require.NoError(t, err)

		assert.True(t, session.IsDirect)
		assert.Equal(t, "direct", session.Channel)
		assert.Equal(t, "FR", session.Country)
		assert.Equal(t, "Paris", session.City)
		assert.InDelta(t, 48.86, *session.Latitude, 1e-9)
	})

	// A session minted while the visitor is already on the site carries one of
	// the site's own pages as its referrer — the SDK rotates onto a fresh id
	// whenever the inactivity window lapses. Recorded as-is, it replaces the
	// visit's acquisition source with the site itself.
	t.Run("self-referral is dropped and the visit reads as direct", func(t *testing.T) {
		payload := basePayload()
		// Mixed case on purpose: both sides are compared after webURLParts has
		// lowercased them.
		payload.Attributes.Referrer = "https://SHOP.example.com/compare/"
		payload.Attributes.UTMSource = ""
		payload.Attributes.UTMMedium = ""

		session, _, goals, err := BuildWebRows(payload, webTestSettings(), geoip.Result{}, receivedAt, nil)
		require.NoError(t, err)

		assert.Empty(t, session.Referrer)
		assert.Empty(t, session.ReferrerDomain)
		assert.Empty(t, session.ReferrerPath)
		assert.True(t, session.IsDirect)
		assert.Equal(t, "direct", session.Channel, "rules run after the drop, on the corrected is_direct")
		assert.Equal(t, "shop.example.com", session.LandingDomain, "only the referrer is cleared")

		require.Len(t, goals, 1)
		assert.Empty(t, goals[0].ReferrerDomain, "the goal snapshot carries the corrected attribution")
		assert.True(t, goals[0].IsDirect)
	})

	t.Run("self-referral keeps the campaign it arrived with", func(t *testing.T) {
		payload := basePayload()
		payload.Attributes.Referrer = "https://shop.example.com/compare/"

		session, _, _, err := BuildWebRows(payload, webTestSettings(), geoip.Result{}, receivedAt, nil)
		require.NoError(t, err)

		assert.True(t, session.IsDirect)
		assert.Equal(t, "google", session.UTMSource)
		assert.Equal(t, "google-ads", session.Channel, "UTM outranks Direct Traffic, which needs empty UTMs")
	})

	t.Run("another host of the same site is a real referral", func(t *testing.T) {
		payload := basePayload()
		payload.Attributes.Referrer = "https://docs.shop.example.com/guide"

		session, _, _, err := BuildWebRows(payload, webTestSettings(), geoip.Result{}, receivedAt, nil)
		require.NoError(t, err)

		assert.Equal(t, "docs.shop.example.com", session.ReferrerDomain)
		assert.Equal(t, "/guide", session.ReferrerPath)
		assert.False(t, session.IsDirect)
	})

	t.Run("nil settings: no filters, defaults still sane", func(t *testing.T) {
		session, _, _, err := BuildWebRows(basePayload(), nil, geoip.Result{}, receivedAt, nil)
		require.NoError(t, err)
		assert.Empty(t, session.Channel, "no rules, no channel")
		assert.Equal(t, "desktop", session.Device)
	})

	t.Run("invalid session id is rejected", func(t *testing.T) {
		payload := basePayload()
		payload.SessionID = "8a9c1a1e-6f0e-4d17-9d5a-6b1f6e2d3c4b"
		_, _, _, err := BuildWebRows(payload, webTestSettings(), geoip.Result{}, receivedAt, nil)
		assert.Error(t, err)
	})
}

func TestMedianInt64(t *testing.T) {
	assert.Equal(t, int64(0), medianInt64(nil))
	assert.Equal(t, int64(5), medianInt64([]int64{5}))
	// Skewed data on purpose: with symmetric values the mean equals the median,
	// so a mean-instead-of-median regression would pass unnoticed.
	assert.Equal(t, int64(2000), medianInt64([]int64{30000, 1000, 2000}), "odd count: middle value, not the mean (11000)")
	assert.Equal(t, int64(2500), medianInt64([]int64{90000, 1000, 2000, 3000}), "even count: mean of middle two (2500), not of all (24000)")
	assert.Equal(t, int64(10), medianInt64([]int64{10, 10, 10, 1000000}), "one huge outlier must not move the median")
	assert.Equal(t, int64(2), medianInt64([]int64{1, 2}), "1.5 rounds to 2")
}

// The goal type is declared by the site and must survive enrichment unchanged.
// Normalisation already happened during payload validation, so this stage only
// has to carry the value across — but "only has to carry it" is exactly the kind
// of step that gets forgotten when a field is added.
func TestBuildWebRowsCarriesGoalType(t *testing.T) {
	receivedAt := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	sessionStart := receivedAt.Add(-10 * time.Minute)
	sessionID := testUUIDv7At(sessionStart)

	payloadWithGoalType := func(goalType string) *domain.WebTrackPayload {
		sentAt := receivedAt.UnixMilli()
		return &domain.WebTrackPayload{
			WorkspaceID: "ws1",
			SessionID:   sessionID,
			Seq:         1,
			CreatedAt:   sessionStart.UnixMilli(),
			UpdatedAt:   receivedAt.UnixMilli(),
			SentAt:      &sentAt,
			SDKVersion:  "1.0.0",
			Attributes:  &domain.WebSessionAttributes{LandingPage: "https://shop.example.com/"},
			Actions: []domain.WebTrackAction{
				{Type: "pageview", Path: "/checkout", PageNumber: 1,
					EnteredAt: sessionStart.UnixMilli(), ExitedAt: sessionStart.Add(time.Minute).UnixMilli()},
				{Type: "goal", Name: "purchase", Path: "/checkout", PageNumber: 1, Value: 49.9,
					GoalType:  goalType,
					Timestamp: sessionStart.Add(2 * time.Minute).UnixMilli()},
			},
		}
	}

	for _, goalType := range domain.ValidGoalTypes {
		t.Run(goalType, func(t *testing.T) {
			_, _, goals, err := BuildWebRows(payloadWithGoalType(goalType), webTestSettings(), geoip.Result{}, receivedAt, nil)
			require.NoError(t, err)
			require.Len(t, goals, 1)
			assert.Equal(t, goalType, goals[0].GoalType)
		})
	}

	t.Run("an untyped action reaches the row as empty, never as a guess", func(t *testing.T) {
		_, _, goals, err := BuildWebRows(payloadWithGoalType(""), webTestSettings(), geoip.Result{}, receivedAt, nil)
		require.NoError(t, err)
		require.Len(t, goals, 1)
		assert.Equal(t, "", goals[0].GoalType,
			"enrichment must not invent a type; defaulting belongs to validation and to the bridge")
	})
}

// TestApplyWebGeoClampsCoordinatesToStoredPlaceNames covers what the existing
// TestApplyWebGeo does not: the case where the toggles and the precision setting
// disagree.
//
// The old behaviour treated them as independent, so switching "Store city" off
// while leaving the default precision of 2 — which the console labels "City level
// (~1km)" — kept storing a city-accurate coordinate. The setting appeared
// honoured and was not.
func TestApplyWebGeoClampsCoordinatesToStoredPlaceNames(t *testing.T) {
	lat, lon := 48.8566, 2.3522
	raw := geoip.Result{Country: "FR", Region: "Île-de-France", City: "Paris", Latitude: &lat, Longitude: &lon}

	t.Run("city off clamps a city-level precision down to region level", func(t *testing.T) {
		out := applyWebGeo(raw, &domain.WebAnalyticsSettings{
			GeoEnabled: true, GeoStoreCity: false, GeoStoreRegion: true, GeoCoordsPrecision: 2,
		})
		assert.Empty(t, out.City)
		require.NotNil(t, out.Latitude)
		assert.InDelta(t, 48.9, *out.Latitude, 1e-9, "~11km, not the ~1km the raw setting asked for")
		assert.InDelta(t, 2.4, *out.Longitude, 1e-9)
	})

	t.Run("city and region off clamp to country level", func(t *testing.T) {
		out := applyWebGeo(raw, &domain.WebAnalyticsSettings{
			GeoEnabled: true, GeoStoreCity: false, GeoStoreRegion: false, GeoCoordsPrecision: 2,
		})
		assert.Empty(t, out.City)
		assert.Empty(t, out.Region)
		require.NotNil(t, out.Latitude)
		assert.InDelta(t, 49, *out.Latitude, 1e-9)
	})

	// The guard against over-clamping: a workspace that stores city names has
	// agreed to city-level detail, and must keep getting it.
	t.Run("city on keeps the configured precision", func(t *testing.T) {
		out := applyWebGeo(raw, &domain.WebAnalyticsSettings{
			GeoEnabled: true, GeoStoreCity: true, GeoStoreRegion: false, GeoCoordsPrecision: 2,
		})
		assert.Equal(t, "Paris", out.City)
		require.NotNil(t, out.Latitude)
		assert.InDelta(t, 48.86, *out.Latitude, 1e-9)
	})

	// Coordinates are clamped, never dropped: an empty Live map reads as "nobody
	// is online", which is a worse bug than a coarse pin.
	t.Run("coordinates are never removed while geo is on", func(t *testing.T) {
		out := applyWebGeo(raw, &domain.WebAnalyticsSettings{
			GeoEnabled: true, GeoStoreCity: false, GeoStoreRegion: false, GeoCoordsPrecision: 0,
		})
		require.NotNil(t, out.Latitude, "the Live map still needs a pin")
		require.NotNil(t, out.Longitude)
	})

	t.Run("nil settings behave as everything on", func(t *testing.T) {
		out := applyWebGeo(raw, nil)
		assert.Equal(t, "Paris", out.City)
		require.NotNil(t, out.Latitude)
		assert.InDelta(t, 48.86, *out.Latitude, 1e-9)
	})
}

// The clamp has to reach goal rows too. Goals carry their own copy of the geo
// columns, and they are never re-upserted by a later beat — so a coordinate
// stored too finely on a goal row stays that way for good.
func TestBuildWebRowsClampsGeoOnGoalRowsToo(t *testing.T) {
	receivedAt := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	sessionStart := receivedAt.Add(-10 * time.Minute)
	sessionID := testUUIDv7At(sessionStart)
	lat, lon := 48.8566, 2.3522
	sentAt := receivedAt.UnixMilli()

	payload := &domain.WebTrackPayload{
		WorkspaceID: "ws1",
		SessionID:   sessionID,
		Seq:         1,
		CreatedAt:   sessionStart.UnixMilli(),
		UpdatedAt:   receivedAt.UnixMilli(),
		SentAt:      &sentAt,
		SDKVersion:  "1.0.0",
		Attributes:  &domain.WebSessionAttributes{LandingPage: "https://shop.example.com/"},
		Actions: []domain.WebTrackAction{
			{Type: "pageview", Path: "/checkout", PageNumber: 1,
				EnteredAt: sessionStart.UnixMilli(), ExitedAt: sessionStart.Add(time.Minute).UnixMilli()},
			{Type: "goal", Name: "purchase", Path: "/checkout", PageNumber: 1, GoalType: "purchase",
				Timestamp: sessionStart.Add(2 * time.Minute).UnixMilli()},
		},
	}

	settings := &domain.WebAnalyticsSettings{
		Enabled: true, GeoEnabled: true,
		GeoStoreCity: false, GeoStoreRegion: false, GeoCoordsPrecision: 2,
		Filters: domain.DefaultWebFilters(),
	}
	geo := geoip.Result{Country: "FR", Region: "Île-de-France", City: "Paris", Latitude: &lat, Longitude: &lon}

	session, _, goals, err := BuildWebRows(payload, settings, geo, receivedAt, nil)
	require.NoError(t, err)
	require.Len(t, goals, 1)

	require.NotNil(t, session.Latitude)
	assert.InDelta(t, 49, *session.Latitude, 1e-9)

	require.NotNil(t, goals[0].Latitude, "a goal row carries its own geo copy")
	assert.InDelta(t, 49, *goals[0].Latitude, 1e-9,
		"goal rows are never re-upserted, so an over-precise coordinate here is permanent")
	assert.Empty(t, goals[0].City)
}
