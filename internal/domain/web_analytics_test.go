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
		// Magnitude, not just sign: each of these lands in a narrow Postgres
		// column, and flushOnce runs a whole workspace batch in ONE transaction —
		// so an out-of-range value used to abort every other visitor's rows too,
		// and the buffer then deleted those sessions after two failed attempts.
		{"page_number beyond the action cap", map[string]interface{}{"type": "pageview", "path": "/p", "page_number": 40000}},
		{"duration beyond a day", map[string]interface{}{"type": "pageview", "path": "/p", "page_number": 1, "duration": int64(WebTrackMaxDurationMs) + 1}},
		{"pageview timestamp beyond TIMESTAMPTZ range", map[string]interface{}{
			"type": "pageview", "path": "/p", "page_number": 1,
			"entered_at": int64(9223372036854775807), "exited_at": int64(9223372036854775807)}},
		{"goal value beyond the column range", map[string]interface{}{
			"type": "goal", "name": "purchase", "page_number": 1,
			"timestamp": now.UnixMilli(), "value": 1e300}},
		{"goal timestamp beyond TIMESTAMPTZ range", map[string]interface{}{
			"type": "goal", "name": "purchase", "page_number": 1,
			"timestamp": int64(9223372036854775807)}},
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

	// The matching loop is shared with notifuse_mjml.MatchesAllowedHost, which
	// reads the same list for a different purpose. These are the cases where a
	// regression would not fail loudly: it would quietly stop accepting a
	// customer's beats, or quietly start accepting a stranger's.
	t.Run("beat-origin matching survives delegation to the shared matcher", func(t *testing.T) {
		s := &WebAnalyticsSettings{AllowedDomains: []string{" Example.COM ", "*.Shop.IO"}}
		assert.True(t, s.MatchesAllowedDomain("example.com"), "stored whitespace and case must still match")
		assert.True(t, s.MatchesAllowedDomain(" APP.shop.io "))
		assert.False(t, s.MatchesAllowedDomain("shop.io.evil.com"), "the wildcard is a suffix, not a substring")

		// A blank entry is not a wildcard: it must widen nothing.
		blank := &WebAnalyticsSettings{AllowedDomains: []string{"", "example.com"}}
		assert.True(t, blank.MatchesAllowedDomain("example.com"))
		assert.False(t, blank.MatchesAllowedDomain("other.com"))
		assert.False(t, blank.MatchesAllowedDomain(""))

		// A "*.com" stored before validateAllowedDomain refused it is skipped by
		// the shared matcher, so it admits no origin here either. Deliberate: an
		// entry the product will not accept must not keep letting traffic in on
		// one side while the other ignores it. Such a workspace has to name its
		// own domain, which is also what makes its beats identifiable again.
		legacy := &WebAnalyticsSettings{AllowedDomains: []string{"*.com", "shop.example.com"}}
		assert.False(t, legacy.MatchesAllowedDomain("anything.com"))
		assert.True(t, legacy.MatchesAllowedDomain("shop.example.com"), "the other entries keep working")
	})

	// Empty means "every origin may beat" here and "no host receives an identity
	// token" in notifuse_mjml.MatchesAllowedHost. Sharing the loop must not have
	// dragged that rule across: an unconfigured workspace losing its own traffic
	// is a silent outage.
	t.Run("an empty list still accepts every origin", func(t *testing.T) {
		s := &WebAnalyticsSettings{Enabled: true}
		assert.True(t, s.MatchesAllowedDomain("anything.example"))
		assert.True(t, (&WebAnalyticsSettings{AllowedDomains: []string{}}).MatchesAllowedDomain("anything.example"))
	})

	// The list gates release of a per-recipient identity token as well as beat
	// origins, so a wildcard over a bare suffix would hand every recipient's
	// identity to every .com link in the workspace's emails.
	t.Run("a wildcard over an effective TLD is rejected at save time", func(t *testing.T) {
		for _, d := range []string{"*.com", "*.io", "*.", "*.localhost"} {
			err := (&WebAnalyticsSettings{AllowedDomains: []string{d}}).Validate()
			assert.ErrorContains(t, err, "allowed domain", "%q must not be storable", d)
			assert.ErrorContains(t, err, "*.example.com", "the message must show what a usable wildcard looks like")
		}
		assert.NoError(t, (&WebAnalyticsSettings{AllowedDomains: []string{"*.example.com", "*.shop.example.com"}}).Validate())

		// Honest limit of a dot count: a public suffix of two labels reads the
		// same as a registrable domain without a public-suffix list, so this
		// check alone is not what keeps such a value from ever matching.
		assert.NoError(t, (&WebAnalyticsSettings{AllowedDomains: []string{"*.co.uk"}}).Validate())
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

// TestBuildWebIdentifyTokenBound pins the mint bound to the resolve bound. The
// token is hex-encoded AES-GCM, so its length grows with the address, while
// WebTrackMaxEmailLength admits contacts whose token overshoots
// WebTrackMaxIdentifyTokenLength. Such a token used to mint happily, ride in
// every link of the email, and be dropped at the beat without a word: the
// identity never happened and nothing said why. It has to fail where the
// address is still in hand.
func TestBuildWebIdentifyTokenBound(t *testing.T) {
	now := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	const secret = "workspace-secret-key"

	// Grow an otherwise legal address until the minted token stops resolving,
	// deriving the frontier from the code rather than restating a number that
	// would rot the moment the payload or the encoding changes.
	longest, shortestRejected := 0, 0
	for n := len("@example.com") + 1; n <= WebTrackMaxEmailLength; n++ {
		email := strings.Repeat("a", n-len("@example.com")) + "@example.com"
		require.Len(t, email, n)
		token, err := BuildWebIdentifyToken(email, secret, WebIdentifyTokenTTL, now)
		if err != nil {
			if shortestRejected == 0 {
				shortestRejected = n
			}
			assert.Empty(t, token, "a rejected mint must not hand back a token to append")
			continue
		}
		require.Zero(t, shortestRejected, "acceptance must not resume after a rejection at %d", shortestRejected)
		longest = n
		require.LessOrEqual(t, len(token), WebTrackMaxIdentifyTokenLength)

		got, ok := ResolveWebIdentity(&WebTrackPayload{IdentifyToken: &token}, secret, now)
		require.True(t, ok, "a %d-character address minted a token the beat then discarded", n)
		assert.Equal(t, strings.ToLower(email), got)
	}

	t.Logf("longest identifiable address: %d characters (of the %d a contact may have)", longest, WebTrackMaxEmailLength)
	require.NotZero(t, longest, "no address at all could be identified")
	require.NotZero(t, shortestRejected,
		"every address up to WebTrackMaxEmailLength now fits: the mint bound is dead code and this test no longer guards anything")
	assert.Equal(t, longest+1, shortestRejected, "the frontier must be a single step, not a range of silent failures")

	t.Run("the error names both bounds so the operator can act on it", func(t *testing.T) {
		email := strings.Repeat("a", shortestRejected-len("@example.com")) + "@example.com"
		_, err := BuildWebIdentifyToken(email, secret, WebIdentifyTokenTTL, now)
		require.Error(t, err)
		assert.ErrorContains(t, err, fmt.Sprintf("%d", WebTrackMaxIdentifyTokenLength))
		assert.ErrorContains(t, err, fmt.Sprintf("%d-character address", len(email)))
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

// TestWebTrackActionAcceptsFractionalMilliseconds pins the wire contract against
// a payload captured verbatim from a real headless Chrome running the shipped
// minified bundle.
//
// The SDK accumulates focus time from performance.now(), so "duration" arrives
// fractional. encoding/json refuses a fractional number for an int64 field and
// that error rejects the ENTIRE payload, so a single ordinary pageview sank the
// whole session with a 400 and web analytics recorded nothing at all from real
// browsers. Every other test here hand-builds actions with round integers,
// which is exactly why none of them could see it.
func TestWebTrackActionAcceptsFractionalMilliseconds(t *testing.T) {
	t.Run("a beat captured from a real browser decodes", func(t *testing.T) {
		// Verbatim capture — do not tidy the numbers. Their untidiness is the
		// property under test.
		const captured = `{"workspace_id":"wire_test","session_id":"019ff09b-720d-7a50-b187-f498b227c39a","tab_id":5297670035850299,"actions":[{"type":"pageview","path":"/","page_number":1,"duration":1473.3999999761581,"scroll":0,"entered_at":1786448146957,"exited_at":1786448148431},{"type":"pageview","path":"/pricing","page_number":2,"duration":100.69999998807907,"scroll":0,"entered_at":1786448148431,"exited_at":1786448148531},{"type":"goal","name":"wire_goal","path":"/pricing","page_number":2,"timestamp":1786448149930,"value":42}],"attributes":{"landing_page":"http://127.0.0.1:57124/"},"created_at":1786448146957,"updated_at":1786448149931,"sdk_version":"38.0","seq":3}`

		var p WebTrackPayload
		require.NoError(t, json.Unmarshal([]byte(captured), &p),
			"a real browser beat must decode; a fractional duration must not sink the payload")
		require.Len(t, p.Actions, 3)

		// Rounded, not truncated: storage is integer milliseconds.
		assert.Equal(t, int64(1473), p.Actions[0].Duration)
		assert.Equal(t, int64(101), p.Actions[1].Duration, "100.699… rounds up, it is not truncated to 100")

		// Integer-valued fields must survive the float round-trip exactly. The
		// goal timestamp is baked into the dedup ExternalID, so a value that
		// shifted by one would silently duplicate every conversion.
		assert.Equal(t, int64(1786448146957), p.Actions[0].EnteredAt)
		assert.Equal(t, int64(1786448148431), p.Actions[0].ExitedAt)
		assert.Equal(t, int64(1786448149930), p.Actions[2].Timestamp)
		assert.Equal(t, float64(42), p.Actions[2].Value)

		// Non-ms fields must still decode through the shadowed alias.
		assert.Equal(t, "pageview", p.Actions[0].Type)
		assert.Equal(t, "/pricing", p.Actions[1].Path)
		assert.Equal(t, 2, p.Actions[1].PageNumber)
		assert.Equal(t, "wire_goal", p.Actions[2].Name)
		assert.Equal(t, int64(5297670035850299), p.TabID)
	})

	t.Run("a fractional epoch timestamp keeps millisecond identity", func(t *testing.T) {
		// float64 holds a millisecond epoch exactly (well under 2^53), so
		// rounding must not perturb it.
		var a WebTrackAction
		require.NoError(t, json.Unmarshal([]byte(
			`{"type":"goal","name":"g","path":"/","page_number":1,"timestamp":1786448149930.4}`), &a))
		assert.Equal(t, int64(1786448149930), a.Timestamp)
	})

	t.Run("a non-finite duration is zeroed rather than becoming a garbage integer", func(t *testing.T) {
		// JSON has no literal for these, but a client can still emit the
		// strings; encoding/json rejects them, and the action must not be left
		// carrying an int64 built from NaN.
		var a WebTrackAction
		err := json.Unmarshal([]byte(`{"type":"pageview","path":"/","page_number":1,"duration":1e400}`), &a)
		require.Error(t, err, "an out-of-range float is still a decode error")
		assert.Zero(t, a.Duration)
	})

	t.Run("an integer duration still decodes unchanged", func(t *testing.T) {
		var a WebTrackAction
		require.NoError(t, json.Unmarshal([]byte(
			`{"type":"pageview","path":"/","page_number":1,"duration":1500,"entered_at":10,"exited_at":20}`), &a))
		assert.Equal(t, int64(1500), a.Duration)
		assert.Equal(t, int64(10), a.EnteredAt)
		assert.Equal(t, int64(20), a.ExitedAt)
	})
}
