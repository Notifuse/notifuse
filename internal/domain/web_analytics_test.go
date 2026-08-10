package domain

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// uuidV7At builds a syntactically valid UUIDv7 string whose embedded
// timestamp is the given time.
func uuidV7At(ts time.Time) string {
	ms := ts.UnixMilli()
	var b [16]byte
	b[0] = byte(ms >> 40)
	b[1] = byte(ms >> 32)
	b[2] = byte(ms >> 24)
	b[3] = byte(ms >> 16)
	b[4] = byte(ms >> 8)
	b[5] = byte(ms)
	b[6] = 0x70 | 0x0A // version 7
	b[7] = 0xBC
	b[8] = 0x80 | 0x11 // RFC 4122 variant
	b[9] = 0x22
	for i := 10; i < 16; i++ {
		b[i] = byte(i)
	}
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func TestSessionDateFromUUIDv7(t *testing.T) {
	now := time.Date(2026, 8, 8, 15, 0, 0, 0, time.UTC)

	t.Run("valid v7 id yields its embedded date and start time", func(t *testing.T) {
		start := time.Date(2026, 8, 7, 23, 59, 30, 0, time.UTC)
		date, gotStart, err := SessionDateFromUUIDv7(uuidV7At(start), now)
		require.NoError(t, err)
		assert.Equal(t, time.Date(2026, 8, 7, 0, 0, 0, 0, time.UTC), date)
		assert.Equal(t, start.UnixMilli(), gotStart.UnixMilli())
	})

	t.Run("stable across repeated calls", func(t *testing.T) {
		id := uuidV7At(now.Add(-2 * time.Hour))
		d1, s1, err1 := SessionDateFromUUIDv7(id, now)
		d2, s2, err2 := SessionDateFromUUIDv7(id, now.Add(20*time.Hour))
		require.NoError(t, err1)
		require.NoError(t, err2)
		assert.Equal(t, d1, d2)
		assert.Equal(t, s1, s2)
	})

	t.Run("rejects non-UUID", func(t *testing.T) {
		_, _, err := SessionDateFromUUIDv7("not-a-uuid", now)
		assert.Error(t, err)
	})

	t.Run("rejects UUIDv4", func(t *testing.T) {
		_, _, err := SessionDateFromUUIDv7("8a9c1a1e-6f0e-4d17-9d5a-6b1f6e2d3c4b", now)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "UUIDv7")
	})

	t.Run("rejects ids older than 48h", func(t *testing.T) {
		_, _, err := SessionDateFromUUIDv7(uuidV7At(now.Add(-49*time.Hour)), now)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "too old")
	})

	t.Run("accepts id just inside the 48h window", func(t *testing.T) {
		_, _, err := SessionDateFromUUIDv7(uuidV7At(now.Add(-47*time.Hour-59*time.Minute)), now)
		assert.NoError(t, err)
	})

	t.Run("rejects ids beyond the future bound", func(t *testing.T) {
		_, _, err := SessionDateFromUUIDv7(uuidV7At(now.Add(WebSessionIDMaxFuture+time.Minute)), now)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "future")
	})

	t.Run("accepts small future skew", func(t *testing.T) {
		_, _, err := SessionDateFromUUIDv7(uuidV7At(now.Add(5*time.Minute)), now)
		assert.NoError(t, err)
	})
}

// trackPayloadJSON builds a raw JSON beat the way the SDK would send it, with
// overridable fields — validation tests must exercise the wire format, not Go
// struct literals.
func trackPayloadJSON(t *testing.T, now time.Time, overrides map[string]interface{}) *WebTrackPayload {
	t.Helper()
	base := map[string]interface{}{
		"workspace_id": "ws1",
		"session_id":   uuidV7At(now.Add(-time.Minute)),
		"actions": []map[string]interface{}{
			{
				"type":        "pageview",
				"path":        "/home",
				"page_number": 1,
				"duration":    1500,
				"scroll":      40,
				"entered_at":  now.Add(-time.Minute).UnixMilli(),
				"exited_at":   now.UnixMilli(),
			},
		},
		"attributes": map[string]interface{}{
			"landing_page": "https://example.com/home",
			"user_agent":   "Mozilla/5.0",
		},
		"created_at":  now.Add(-time.Minute).UnixMilli(),
		"updated_at":  now.UnixMilli(),
		"sent_at":     now.UnixMilli(),
		"sdk_version": "1.0.0",
		"seq":         3,
	}
	for k, v := range overrides {
		if v == nil {
			delete(base, k)
		} else {
			base[k] = v
		}
	}
	raw, err := json.Marshal(base)
	require.NoError(t, err)
	var payload WebTrackPayload
	require.NoError(t, json.Unmarshal(raw, &payload))
	return &payload
}

func TestWebTrackPayloadValidate(t *testing.T) {
	now := time.Date(2026, 8, 8, 15, 0, 0, 0, time.UTC)

	t.Run("valid payload passes", func(t *testing.T) {
		p := trackPayloadJSON(t, now, nil)
		assert.NoError(t, p.Validate(now))
	})

	t.Run("empty actions is valid (early-return beat)", func(t *testing.T) {
		p := trackPayloadJSON(t, now, map[string]interface{}{"actions": []map[string]interface{}{}})
		assert.NoError(t, p.Validate(now))
	})

	t.Run("missing workspace_id", func(t *testing.T) {
		p := trackPayloadJSON(t, now, map[string]interface{}{"workspace_id": nil})
		assert.ErrorContains(t, p.Validate(now), "workspace_id")
	})

	t.Run("missing session_id", func(t *testing.T) {
		p := trackPayloadJSON(t, now, map[string]interface{}{"session_id": nil})
		assert.ErrorContains(t, p.Validate(now), "session_id")
	})

	t.Run("v4 session_id rejected", func(t *testing.T) {
		p := trackPayloadJSON(t, now, map[string]interface{}{"session_id": "8a9c1a1e-6f0e-4d17-9d5a-6b1f6e2d3c4b"})
		assert.ErrorContains(t, p.Validate(now), "session_id")
	})

	t.Run("created_at is ignored: the session id is the source of truth", func(t *testing.T) {
		// The SDK resends the session's birth time on every beat. Validating it
		// against server time rejected long-lived sessions outright — and a 400
		// is never retried — while the id already pins the session start and
		// its partition. Absent, ancient and absurd values are all accepted and
		// simply unused.
		for _, value := range []interface{}{
			nil,
			now.Add(-25 * time.Hour).UnixMilli(),
			now.Add(-40 * time.Hour).UnixMilli(),
			int64(0),
		} {
			p := trackPayloadJSON(t, now, map[string]interface{}{"created_at": value})
			assert.NoError(t, p.Validate(now), "created_at=%v", value)
		}
	})

	t.Run("a session still beating after 24h is accepted", func(t *testing.T) {
		// The id window (48h) is what bounds session age; created_at must not
		// impose a second, stricter limit on the same fact.
		sessionStart := now.Add(-30 * time.Hour)
		p := trackPayloadJSON(t, now, map[string]interface{}{
			"session_id": uuidV7At(sessionStart),
			"created_at": sessionStart.UnixMilli(),
		})
		assert.NoError(t, p.Validate(now))
	})

	t.Run("updated_at in the far future", func(t *testing.T) {
		p := trackPayloadJSON(t, now, map[string]interface{}{"updated_at": now.Add(25 * time.Hour).UnixMilli()})
		assert.ErrorContains(t, p.Validate(now), "updated_at")
	})

	t.Run("negative seq rejected, missing seq defaults to zero", func(t *testing.T) {
		p := trackPayloadJSON(t, now, map[string]interface{}{"seq": -1})
		assert.ErrorContains(t, p.Validate(now), "seq")

		p = trackPayloadJSON(t, now, map[string]interface{}{"seq": nil})
		assert.NoError(t, p.Validate(now))
		assert.Equal(t, int64(0), p.Seq)
	})

	t.Run("too many actions", func(t *testing.T) {
		actions := make([]map[string]interface{}, WebTrackMaxActions+1)
		for i := range actions {
			actions[i] = map[string]interface{}{
				"type": "pageview", "path": "/p", "page_number": i + 1,
			}
		}
		p := trackPayloadJSON(t, now, map[string]interface{}{"actions": actions})
		assert.ErrorContains(t, p.Validate(now), "actions")
	})

	// A malformed action is dropped, never fatal: actions[] is cumulative, so a
	// beat rejected for one bad entry would reject every later beat of that
	// session too. Asserting the action is *gone* still proves
	// WebTrackAction.Validate rejects the shape — if it stopped rejecting, the
	// action would survive and these would fail.
	for _, tc := range []struct {
		name   string
		action map[string]interface{}
	}{
		{"unknown action type", map[string]interface{}{"type": "click", "path": "/p", "page_number": 1}},
		{"pageview scroll out of range", map[string]interface{}{"type": "pageview", "path": "/p", "page_number": 1, "scroll": 101}},
		{"pageview exited before entered", map[string]interface{}{
			"type": "pageview", "path": "/p", "page_number": 1,
			"entered_at": now.UnixMilli(), "exited_at": now.Add(-time.Minute).UnixMilli(),
		}},
		{"goal without name", map[string]interface{}{"type": "goal", "path": "/p", "page_number": 1, "timestamp": now.UnixMilli()}},
		{"goal name too long", map[string]interface{}{
			"type": "goal", "path": "/p", "page_number": 1,
			"name": strings.Repeat("g", WebTrackMaxGoalNameLength+1), "timestamp": now.UnixMilli(),
		}},
		{"path too long", map[string]interface{}{
			"type": "pageview", "path": "/" + strings.Repeat("a", WebTrackMaxPathLength), "page_number": 1,
		}},
		{"page_number below one", map[string]interface{}{"type": "pageview", "path": "/p", "page_number": 0}},
	} {
		t.Run(tc.name+" is dropped, not fatal", func(t *testing.T) {
			p := trackPayloadJSON(t, now, map[string]interface{}{
				"actions": []map[string]interface{}{tc.action},
			})
			require.NoError(t, p.Validate(now))
			assert.Empty(t, p.Actions)
		})
	}

	t.Run("oversized stm dimension value rejected, unknown keys tolerated", func(t *testing.T) {
		p := trackPayloadJSON(t, now, map[string]interface{}{
			"dimensions": map[string]string{"custom_1": strings.Repeat("v", WebTrackMaxDimensionValueLength+1)},
		})
		assert.ErrorContains(t, p.Validate(now), "custom_1")

		p = trackPayloadJSON(t, now, map[string]interface{}{
			"dimensions": map[string]string{"other": strings.Repeat("v", 500), "custom_2": "ok"},
		})
		assert.NoError(t, p.Validate(now))
	})
}

func TestWebAnalyticsSettings(t *testing.T) {
	t.Run("nil settings validate and default the bounce threshold", func(t *testing.T) {
		var s *WebAnalyticsSettings
		assert.NoError(t, s.Validate())
		assert.Equal(t, 10000, s.BounceThresholdMs())
		assert.True(t, s.MatchesAllowedDomain("anything.example"))
	})

	t.Run("bounce threshold conversion", func(t *testing.T) {
		s := &WebAnalyticsSettings{BounceThresholdSeconds: 25}
		assert.Equal(t, 25000, s.BounceThresholdMs())
		s.BounceThresholdSeconds = 0
		assert.Equal(t, 10000, s.BounceThresholdMs())
	})

	t.Run("validation rejects bad values", func(t *testing.T) {
		assert.ErrorContains(t, (&WebAnalyticsSettings{BounceThresholdSeconds: -1}).Validate(), "bounce_threshold")
		assert.ErrorContains(t, (&WebAnalyticsSettings{GeoCoordsPrecision: 3}).Validate(), "geo_coordinates_precision")
		assert.ErrorContains(t, (&WebAnalyticsSettings{AllowedDomains: []string{"https://x.com"}}).Validate(), "allowed domain")
		assert.ErrorContains(t, (&WebAnalyticsSettings{AllowedDomains: []string{""}}).Validate(), "allowed domain")
		assert.ErrorContains(t, (&WebAnalyticsSettings{CustomDimensionLabels: map[string]string{"custom_11": "x"}}).Validate(), "custom_1..custom_10")
	})

	t.Run("valid settings pass, including a filter", func(t *testing.T) {
		s := &WebAnalyticsSettings{
			Enabled:                true,
			AllowedDomains:         []string{"example.com", "*.example.org"},
			BounceThresholdSeconds: 15,
			CustomDimensionLabels:  map[string]string{"custom_1": "Plan"},
			GeoEnabled:             true,
			GeoStoreCity:           true,
			GeoStoreRegion:         true,
			GeoCoordsPrecision:     2,
			Filters:                DefaultWebFilters(),
		}
		assert.NoError(t, s.Validate())
	})

	t.Run("allowed domain matching", func(t *testing.T) {
		s := &WebAnalyticsSettings{AllowedDomains: []string{"example.com", "*.shop.io"}}
		assert.True(t, s.MatchesAllowedDomain("example.com"))
		assert.True(t, s.MatchesAllowedDomain("EXAMPLE.com"))
		assert.False(t, s.MatchesAllowedDomain("sub.example.com"))
		assert.True(t, s.MatchesAllowedDomain("shop.io"), "wildcard matches the apex too")
		assert.True(t, s.MatchesAllowedDomain("app.shop.io"))
		assert.True(t, s.MatchesAllowedDomain("a.b.shop.io"))
		assert.False(t, s.MatchesAllowedDomain("evilshop.io"))
		assert.False(t, s.MatchesAllowedDomain(""))
	})

	t.Run("JSON round-trip inside workspace settings omits when nil", func(t *testing.T) {
		ws := WorkspaceSettings{Timezone: "UTC"}
		raw, err := json.Marshal(ws)
		require.NoError(t, err)
		assert.NotContains(t, string(raw), "web_analytics")

		ws.WebAnalytics = &WebAnalyticsSettings{Enabled: true, BounceThresholdSeconds: 12}
		raw, err = json.Marshal(ws)
		require.NoError(t, err)
		var back WorkspaceSettings
		require.NoError(t, json.Unmarshal(raw, &back))
		require.NotNil(t, back.WebAnalytics)
		assert.True(t, back.WebAnalytics.Enabled)
		assert.Equal(t, 12, back.WebAnalytics.BounceThresholdSeconds)
	})
}

// TestSessionDateFromUUIDv7ClockSkew covers W0.2: a device clock running fast is
// the common case, not an attack. The SDK mints the session id from Date.now(),
// so the id inherits the whole skew, and rejecting it means that visitor records
// nothing at all — the SDK treats the 400 as permanent and never retries.
func TestSessionDateFromUUIDv7ClockSkew(t *testing.T) {
	now := time.Date(2026, 8, 8, 15, 0, 0, 0, time.UTC)

	for _, skew := range []time.Duration{15 * time.Minute, time.Hour, 20 * time.Hour} {
		t.Run("accepts a clock "+skew.String()+" fast", func(t *testing.T) {
			_, _, err := SessionDateFromUUIDv7(uuidV7At(now.Add(skew)), now)
			assert.NoError(t, err)
		})
	}

	t.Run("future bound matches the beat window", func(t *testing.T) {
		// Keeping the two windows equal is what stops the id from silently
		// overriding updated_at as the binding constraint on the future side,
		// and it bounds partition creation to one day ahead — which the
		// maintenance worker already provisions.
		assert.Equal(t, WebTrackTimeBounds, WebSessionIDMaxFuture)
	})

	t.Run("still rejects beyond the beat window", func(t *testing.T) {
		_, _, err := SessionDateFromUUIDv7(uuidV7At(now.Add(WebTrackTimeBounds+time.Hour)), now)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "future")
	})

	t.Run("session_date is stable across UTC midnight", func(t *testing.T) {
		// The regression test for the clamp that must NOT be applied: session_date
		// is a pure function of the id, so a clock-fast visitor's session cannot
		// change partition — and therefore primary key — as the server's day rolls.
		id := uuidV7At(time.Date(2026, 8, 9, 0, 5, 0, 0, time.UTC))
		before, _, err1 := SessionDateFromUUIDv7(id, time.Date(2026, 8, 8, 23, 50, 0, 0, time.UTC))
		after, _, err2 := SessionDateFromUUIDv7(id, time.Date(2026, 8, 9, 0, 10, 0, 0, time.UTC))
		require.NoError(t, err1)
		require.NoError(t, err2)
		assert.Equal(t, before, after)
	})
}

// TestWebTrackPayloadDropsMalformedActions covers W0.4 (server half): actions[]
// is cumulative, so one poisoned entry rejected wholesale would reject every
// subsequent beat of that session forever. One bad action must cost one action.
func TestWebTrackPayloadDropsMalformedActions(t *testing.T) {
	now := time.Date(2026, 8, 8, 15, 0, 0, 0, time.UTC)
	pageview := func(n int, dur int64) map[string]interface{} {
		return map[string]interface{}{
			"type": "pageview", "path": fmt.Sprintf("/p%d", n), "page_number": n,
			"duration": dur, "scroll": 10,
			"entered_at": now.Add(-time.Minute).UnixMilli(), "exited_at": now.UnixMilli(),
		}
	}

	t.Run("one negative-duration action among five is dropped, the rest survive", func(t *testing.T) {
		p := trackPayloadJSON(t, now, map[string]interface{}{
			"actions": []map[string]interface{}{
				pageview(1, 100), pageview(2, 100), pageview(3, -5), pageview(4, 100), pageview(5, 100),
			},
		})
		require.NoError(t, p.Validate(now))
		require.Len(t, p.Actions, 4)
		for _, a := range p.Actions {
			assert.NotEqual(t, "/p3", a.Path)
		}
	})

	t.Run("a nameless goal is dropped without taking the pageviews with it", func(t *testing.T) {
		p := trackPayloadJSON(t, now, map[string]interface{}{
			"actions": []map[string]interface{}{
				pageview(1, 100),
				{"type": "goal", "name": "", "page_number": 1, "timestamp": now.UnixMilli()},
				{"type": "goal", "name": "purchase", "page_number": 1, "timestamp": now.UnixMilli(), "value": 9.99},
			},
		})
		require.NoError(t, p.Validate(now))
		require.Len(t, p.Actions, 2)
		assert.Equal(t, "purchase", p.Actions[1].Name)
	})

	t.Run("an all-malformed payload validates to an empty action list", func(t *testing.T) {
		// The service already treats zero actions as a silent success, so the beat
		// is accepted and records nothing — never a 400 the SDK reads as permanent.
		p := trackPayloadJSON(t, now, map[string]interface{}{
			"actions": []map[string]interface{}{pageview(1, -1)},
		})
		require.NoError(t, p.Validate(now))
		assert.Empty(t, p.Actions)
	})

	t.Run("payload-level failures still reject", func(t *testing.T) {
		p := trackPayloadJSON(t, now, map[string]interface{}{"workspace_id": ""})
		assert.Error(t, p.Validate(now))
	})
}

// TestResolveWebIdentity covers W2: /track is public and unauthenticated, so an
// email on the wire is worth nothing until a signature ties it to the
// workspace secret. These cases are the difference between "a contact was
// identified" and "anyone can write to any contact's timeline".
func TestResolveWebIdentity(t *testing.T) {
	now := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	const secret = "workspace-secret-key"
	const other = "a-different-workspace-secret"
	email := "Alice@Example.com" // deliberately mixed case

	ptr := func(s string) *string { return &s }

	t.Run("a valid HMAC identifies the contact, normalized for storage", func(t *testing.T) {
		// Verify against the RAW address the customer signed, then normalize —
		// doing it the other way round fails every HMAC ever minted.
		got, ok := ResolveWebIdentity(&WebTrackPayload{
			ContactEmail:     ptr(email),
			ContactEmailHMAC: ptr(ComputeWebIdentifyHMAC(email, secret)),
		}, secret, now)
		require.True(t, ok)
		assert.Equal(t, "alice@example.com", got)
	})

	t.Run("the analytics HMAC is domain-separated from the subscription one", func(t *testing.T) {
		// ComputeEmailHMAC already authorizes subscription writes and ships in
		// every email Notifuse sends. If the two were interchangeable, an
		// unsubscribe link scraped from a forwarded email would identify, and an
		// analytics credential lifted from page JS could change subscriptions.
		assert.NotEqual(t, ComputeEmailHMAC(email, secret), ComputeWebIdentifyHMAC(email, secret))

		_, ok := ResolveWebIdentity(&WebTrackPayload{
			ContactEmail:     ptr(email),
			ContactEmailHMAC: ptr(ComputeEmailHMAC(email, secret)),
		}, secret, now)
		assert.False(t, ok, "a notification-center HMAC must not identify")
	})

	for _, tc := range []struct {
		name    string
		payload *WebTrackPayload
	}{
		{"wrong hmac", &WebTrackPayload{ContactEmail: ptr(email), ContactEmailHMAC: ptr("deadbeef")}},
		{"hmac for a different email", &WebTrackPayload{
			ContactEmail: ptr(email), ContactEmailHMAC: ptr(ComputeWebIdentifyHMAC("mallory@example.com", secret))}},
		{"hmac under another workspace's secret", &WebTrackPayload{
			ContactEmail: ptr(email), ContactEmailHMAC: ptr(ComputeWebIdentifyHMAC(email, other))}},
		{"email without hmac", &WebTrackPayload{ContactEmail: ptr(email)}},
		{"hmac without email", &WebTrackPayload{ContactEmailHMAC: ptr("abc")}},
		{"nothing at all", &WebTrackPayload{}},
		{"over-length email", &WebTrackPayload{
			ContactEmail: ptr(strings.Repeat("e", WebTrackMaxEmailLength+1)), ContactEmailHMAC: ptr("x")}},
	} {
		t.Run(tc.name+" is rejected", func(t *testing.T) {
			_, ok := ResolveWebIdentity(tc.payload, secret, now)
			assert.False(t, ok)
		})
	}

	t.Run("a signed token identifies without exposing the address in the URL", func(t *testing.T) {
		token, err := BuildWebIdentifyToken(email, secret, 30*24*time.Hour, now)
		require.NoError(t, err)
		assert.NotContains(t, token, "alice", "the address must not be readable in the URL")

		got, ok := ResolveWebIdentity(&WebTrackPayload{IdentifyToken: &token}, secret, now)
		require.True(t, ok)
		assert.Equal(t, "alice@example.com", got)
	})

	t.Run("an expired token is rejected", func(t *testing.T) {
		token, err := BuildWebIdentifyToken(email, secret, time.Hour, now.Add(-2*time.Hour))
		require.NoError(t, err)
		_, ok := ResolveWebIdentity(&WebTrackPayload{IdentifyToken: &token}, secret, now)
		assert.False(t, ok)
	})

	t.Run("a token minted for another workspace is rejected", func(t *testing.T) {
		token, err := BuildWebIdentifyToken(email, other, time.Hour, now)
		require.NoError(t, err)
		_, ok := ResolveWebIdentity(&WebTrackPayload{IdentifyToken: &token}, secret, now)
		assert.False(t, ok)
	})

	t.Run("an invalid token fails closed instead of falling through to the hmac", func(t *testing.T) {
		// Trying the next credential after a bad token would let an attacker
		// downgrade past whichever check they cannot satisfy.
		_, ok := ResolveWebIdentity(&WebTrackPayload{
			IdentifyToken:    ptr("not-a-token"),
			ContactEmail:     ptr(email),
			ContactEmailHMAC: ptr(ComputeWebIdentifyHMAC(email, secret)),
		}, secret, now)
		assert.False(t, ok)
	})

	t.Run("an empty workspace secret never identifies", func(t *testing.T) {
		// A workspace with no secret must not accept an HMAC computed over "".
		_, ok := ResolveWebIdentity(&WebTrackPayload{
			ContactEmail:     ptr(email),
			ContactEmailHMAC: ptr(ComputeWebIdentifyHMAC(email, "")),
		}, "", now)
		assert.False(t, ok)
	})
}

// TestWebTrackGoalPropertiesBounds covers W2b: goal properties were bounded by
// nothing at all, and actions[] is cumulative — so one fat properties map is
// re-sent on every later beat until the body crosses the server's 1MB cap, at
// which point EVERY subsequent beat of that session is rejected, forever. This
// is a permanent wedge an honest customer can reach, not merely an abuse vector.
func TestWebTrackGoalPropertiesBounds(t *testing.T) {
	now := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)

	goal := func(props map[string]string) map[string]interface{} {
		return map[string]interface{}{
			"type": "goal", "name": "purchase", "page_number": 1,
			"timestamp": now.UnixMilli(), "properties": props,
		}
	}
	pageview := map[string]interface{}{
		"type": "pageview", "path": "/p", "page_number": 1,
		"duration": 10, "scroll": 5,
		"entered_at": now.Add(-time.Minute).UnixMilli(), "exited_at": now.UnixMilli(),
	}

	t.Run("a reasonable properties map survives", func(t *testing.T) {
		p := trackPayloadJSON(t, now, map[string]interface{}{
			"actions": []map[string]interface{}{goal(map[string]string{"plan": "pro", "seats": "12"})},
		})
		require.NoError(t, p.Validate(now))
		require.Len(t, p.Actions, 1)
		assert.Equal(t, "pro", p.Actions[0].Properties["plan"])
	})

	for _, tc := range []struct {
		name  string
		props map[string]string
	}{
		{"too many keys", func() map[string]string {
			m := map[string]string{}
			for i := 0; i <= WebTrackMaxGoalPropertyKeys; i++ {
				m[fmt.Sprintf("k%d", i)] = "v"
			}
			return m
		}()},
		{"an over-long value", map[string]string{"blob": strings.Repeat("x", WebTrackMaxGoalPropertyValueLength+1)}},
		{"over the total byte budget", func() map[string]string {
			m := map[string]string{}
			per := WebTrackMaxGoalPropertyValueLength
			for i := 0; i < (WebTrackMaxGoalPropertiesBytes/per)+2; i++ {
				m[fmt.Sprintf("k%d", i)] = strings.Repeat("y", per)
			}
			return m
		}()},
	} {
		t.Run(tc.name+" costs the action, never the beat", func(t *testing.T) {
			// Dropping the action is what stops the wedge: the oversized goal
			// never enters the cumulative array, so the NEXT beat is unaffected.
			p := trackPayloadJSON(t, now, map[string]interface{}{
				"actions": []map[string]interface{}{pageview, goal(tc.props)},
			})
			require.NoError(t, p.Validate(now))
			require.Len(t, p.Actions, 1, "the pageview must survive the bad goal")
			assert.Equal(t, WebActionTypePageview, p.Actions[0].Type)
		})
	}
}
