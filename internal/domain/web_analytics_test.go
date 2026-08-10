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

	t.Run("rejects ids more than 10min in the future", func(t *testing.T) {
		_, _, err := SessionDateFromUUIDv7(uuidV7At(now.Add(11*time.Minute)), now)
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

	t.Run("unknown action type rejected", func(t *testing.T) {
		p := trackPayloadJSON(t, now, map[string]interface{}{
			"actions": []map[string]interface{}{{"type": "click", "path": "/p", "page_number": 1}},
		})
		assert.ErrorContains(t, p.Validate(now), "unknown action type")
	})

	t.Run("pageview scroll out of range", func(t *testing.T) {
		p := trackPayloadJSON(t, now, map[string]interface{}{
			"actions": []map[string]interface{}{{"type": "pageview", "path": "/p", "page_number": 1, "scroll": 101}},
		})
		assert.ErrorContains(t, p.Validate(now), "scroll")
	})

	t.Run("pageview exited before entered", func(t *testing.T) {
		p := trackPayloadJSON(t, now, map[string]interface{}{
			"actions": []map[string]interface{}{{
				"type": "pageview", "path": "/p", "page_number": 1,
				"entered_at": now.UnixMilli(), "exited_at": now.Add(-time.Minute).UnixMilli(),
			}},
		})
		assert.ErrorContains(t, p.Validate(now), "exited_at")
	})

	t.Run("goal without name rejected", func(t *testing.T) {
		p := trackPayloadJSON(t, now, map[string]interface{}{
			"actions": []map[string]interface{}{{
				"type": "goal", "path": "/p", "page_number": 1, "timestamp": now.UnixMilli(),
			}},
		})
		assert.ErrorContains(t, p.Validate(now), "goal name")
	})

	t.Run("goal name too long", func(t *testing.T) {
		p := trackPayloadJSON(t, now, map[string]interface{}{
			"actions": []map[string]interface{}{{
				"type": "goal", "path": "/p", "page_number": 1,
				"name": strings.Repeat("g", WebTrackMaxGoalNameLength+1), "timestamp": now.UnixMilli(),
			}},
		})
		assert.ErrorContains(t, p.Validate(now), "goal name")
	})

	t.Run("path too long", func(t *testing.T) {
		p := trackPayloadJSON(t, now, map[string]interface{}{
			"actions": []map[string]interface{}{{
				"type": "pageview", "path": "/" + strings.Repeat("a", WebTrackMaxPathLength), "page_number": 1,
			}},
		})
		assert.ErrorContains(t, p.Validate(now), "path")
	})

	t.Run("page_number below one rejected", func(t *testing.T) {
		p := trackPayloadJSON(t, now, map[string]interface{}{
			"actions": []map[string]interface{}{{"type": "pageview", "path": "/p", "page_number": 0}},
		})
		assert.ErrorContains(t, p.Validate(now), "page_number")
	})

	t.Run("user_id too long", func(t *testing.T) {
		p := trackPayloadJSON(t, now, map[string]interface{}{"user_id": strings.Repeat("u", WebTrackMaxUserIDLength+1)})
		assert.ErrorContains(t, p.Validate(now), "user_id")
	})

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
