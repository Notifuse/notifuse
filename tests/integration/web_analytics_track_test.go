//go:build integration
// +build integration

package integration

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/tests/testutil"
)

// waUUIDv7At builds a UUIDv7 embedding the given timestamp (the SDK generates
// these client-side; the server derives the partition date from it).
func waUUIDv7At(ts time.Time, salt byte) string {
	ms := ts.UnixMilli()
	var b [16]byte
	b[0], b[1], b[2] = byte(ms>>40), byte(ms>>32), byte(ms>>24)
	b[3], b[4], b[5] = byte(ms>>16), byte(ms>>8), byte(ms)
	b[6] = 0x74
	b[7] = salt
	b[8] = 0x92
	b[9] = salt
	for i := 10; i < 16; i++ {
		b[i] = 0xCD
	}
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

type waBeat struct {
	workspaceID string
	sessionID   string
	seq         int64
	actions     []map[string]interface{}
	dimensions  map[string]string
	utmIDFrom   string
}

func (b waBeat) body(t *testing.T, now time.Time) []byte {
	t.Helper()
	attrs := map[string]interface{}{
		"landing_page": "https://shop.example.com/landing",
		"referrer":     "https://www.google.com/",
		"user_agent":   "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 Chrome/126.0.0.0 Safari/537.36",
	}
	if b.utmIDFrom != "" {
		attrs["utm_id"] = "abc123"
		attrs["utm_id_from"] = b.utmIDFrom
	}
	payload := map[string]interface{}{
		"workspace_id": b.workspaceID,
		"session_id":   b.sessionID,
		"actions":      b.actions,
		"attributes":   attrs,
		"created_at":   now.Add(-2 * time.Minute).UnixMilli(),
		"updated_at":   now.UnixMilli(),
		"sent_at":      now.UnixMilli(),
		"sdk_version":  "1.0.0",
		"seq":          b.seq,
	}
	if b.dimensions != nil {
		payload["dimensions"] = b.dimensions
	}
	raw, err := json.Marshal(payload)
	require.NoError(t, err)
	return raw
}

func waPageview(path string, number int, durationMs int64, scroll int, now time.Time) map[string]interface{} {
	return map[string]interface{}{
		"type": "pageview", "path": path, "page_number": number,
		"duration": durationMs, "scroll": scroll,
		"entered_at": now.Add(-2 * time.Minute).UnixMilli(),
		"exited_at":  now.UnixMilli(),
	}
}

func waPostBeat(t *testing.T, baseURL string, body []byte, headers map[string]string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, baseURL+"/track", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "text/plain;charset=UTF-8") // SDK avoids preflights
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh) Chrome/126.0")
	req.Header.Set("Origin", "https://shop.example.com")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	return resp
}

func waCloseAndDecode(t *testing.T, resp *http.Response) map[string]interface{} {
	t.Helper()
	defer func() { _ = resp.Body.Close() }()
	var body map[string]interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	return body
}

func waScanSession(t *testing.T, db *sql.DB, sessionID string) (pageviewCount int, durationMs int64, goalCount int, channel string, beatSeq int64) {
	t.Helper()
	err := db.QueryRow(`SELECT pageview_count, duration_ms, goal_count, channel, beat_seq FROM web_sessions WHERE id = $1`, sessionID).
		Scan(&pageviewCount, &durationMs, &goalCount, &channel, &beatSeq)
	require.NoError(t, err)
	return
}

func TestWebAnalyticsTrackEndToEnd(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer func() { suite.Cleanup() }()

	// A workspace with web analytics enabled and the default attribution
	// rules; a second one left untouched (feature disabled).
	workspace, err := suite.DataFactory.CreateWorkspace(func(w *domain.Workspace) {
		w.Settings.WebAnalytics = &domain.WebAnalyticsSettings{
			Enabled:                true,
			BounceThresholdSeconds: 10,
			Filters:                domain.DefaultWebFilters(),
			GeoEnabled:             false,
		}
	})
	require.NoError(t, err)

	disabledWorkspace, err := suite.DataFactory.CreateWorkspace()
	require.NoError(t, err)

	baseURL := suite.ServerManager.GetURL()
	buffer := suite.ServerManager.GetApp().GetWebAnalyticsBuffer()
	require.NotNil(t, buffer)

	wsDB, err := suite.DBManager.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)

	now := time.Now().UTC()
	sessionID := waUUIDv7At(now.Add(-2*time.Minute), 0x01)

	t.Run("first beat creates session and page rows", func(t *testing.T) {
		beat := waBeat{
			workspaceID: workspace.ID, sessionID: sessionID, seq: 1,
			actions:    []map[string]interface{}{waPageview("/landing", 1, 1500, 30, now)},
			dimensions: map[string]string{"custom_1": "variant-a"},
			utmIDFrom:  "gclid",
		}
		resp := waPostBeat(t, baseURL, beat.body(t, now), nil)
		body := waCloseAndDecode(t, resp)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, true, body["success"])

		buffer.FlushAll(context.Background())

		pageviews, duration, goals, channel, seq := waScanSession(t, wsDB, sessionID)
		assert.Equal(t, 1, pageviews)
		assert.Equal(t, int64(1500), duration)
		assert.Zero(t, goals)
		assert.Equal(t, "google-ads", channel, "gclid click id classified by the default rules")
		assert.Equal(t, int64(1), seq)

		var isExit bool
		var stm1 string
		require.NoError(t, wsDB.QueryRow(`SELECT p.is_exit, s.custom_1 FROM web_pages p JOIN web_sessions s ON s.id = p.session_id WHERE p.session_id = $1 AND p.page_number = 1`, sessionID).Scan(&isExit, &stm1))
		assert.True(t, isExit)
		assert.Equal(t, "variant-a", stm1)
	})

	t.Run("second beat upserts cumulatively: no duplicate session, exit moves, goal lands", func(t *testing.T) {
		beat := waBeat{
			workspaceID: workspace.ID, sessionID: sessionID, seq: 2,
			actions: []map[string]interface{}{
				waPageview("/landing", 1, 1500, 30, now),
				waPageview("/pricing", 2, 2500, 70, now),
				{"type": "goal", "name": "signup", "path": "/pricing", "page_number": 2,
					"timestamp": now.UnixMilli(), "value": 9.5,
					"properties": map[string]string{"plan": "starter"}},
			},
			utmIDFrom: "gclid",
		}
		resp := waPostBeat(t, baseURL, beat.body(t, now), nil)
		_ = waCloseAndDecode(t, resp)
		buffer.FlushAll(context.Background())

		var sessionCount int
		require.NoError(t, wsDB.QueryRow(`SELECT COUNT(*) FROM web_sessions WHERE id = $1`, sessionID).Scan(&sessionCount))
		assert.Equal(t, 1, sessionCount, "cumulative beats must upsert, not duplicate")

		pageviews, duration, goals, _, seq := waScanSession(t, wsDB, sessionID)
		assert.Equal(t, 2, pageviews)
		assert.Equal(t, int64(4000), duration, "duration is the sum of page focus")
		assert.Equal(t, 1, goals)
		assert.Equal(t, int64(2), seq)

		var exit1, exit2 bool
		require.NoError(t, wsDB.QueryRow(`SELECT is_exit FROM web_pages WHERE session_id = $1 AND page_number = 1`, sessionID).Scan(&exit1))
		require.NoError(t, wsDB.QueryRow(`SELECT is_exit FROM web_pages WHERE session_id = $1 AND page_number = 2`, sessionID).Scan(&exit2))
		assert.False(t, exit1, "exit flag moved off the first page")
		assert.True(t, exit2)

		var goalValue float64
		var goalChannel string
		require.NoError(t, wsDB.QueryRow(`SELECT goal_value, channel FROM web_goals WHERE session_id = $1 AND goal_name = 'signup'`, sessionID).Scan(&goalValue, &goalChannel))
		assert.InDelta(t, 9.5, goalValue, 1e-6)
		assert.Equal(t, "google-ads", goalChannel, "goal snapshot carries session attribution")
	})

	t.Run("identical replay and stale beat cannot regress the row", func(t *testing.T) {
		// Replay of seq 2 (network retry): idempotent.
		beat := waBeat{
			workspaceID: workspace.ID, sessionID: sessionID, seq: 2,
			actions:   []map[string]interface{}{waPageview("/landing", 1, 1500, 30, now)},
			utmIDFrom: "gclid",
		}
		resp := waPostBeat(t, baseURL, beat.body(t, now), nil)
		_ = waCloseAndDecode(t, resp)
		buffer.FlushAll(context.Background())

		// A stale seq-1 beat (offline queue replay) must also be ignored.
		stale := waBeat{
			workspaceID: workspace.ID, sessionID: sessionID, seq: 1,
			actions:   []map[string]interface{}{waPageview("/landing", 1, 100, 5, now)},
			utmIDFrom: "gclid",
		}
		resp = waPostBeat(t, baseURL, stale.body(t, now), nil)
		_ = waCloseAndDecode(t, resp)
		buffer.FlushAll(context.Background())

		pageviews, duration, goals, _, seq := waScanSession(t, wsDB, sessionID)
		assert.Equal(t, 2, pageviews, "stale beats must not shrink the session")
		assert.Equal(t, int64(4000), duration)
		assert.Equal(t, 1, goals)
		assert.Equal(t, int64(2), seq)
	})

	t.Run("allowed domains reject foreign origins silently", func(t *testing.T) {
		restricted, err := suite.DataFactory.CreateWorkspace(func(w *domain.Workspace) {
			w.Settings.WebAnalytics = &domain.WebAnalyticsSettings{
				Enabled:        true,
				AllowedDomains: []string{"*.example.com"},
			}
		})
		require.NoError(t, err)
		restrictedDB, err := suite.DBManager.GetWorkspaceDB(restricted.ID)
		require.NoError(t, err)

		foreignSession := waUUIDv7At(now, 0x02)
		beat := waBeat{
			workspaceID: restricted.ID, sessionID: foreignSession, seq: 1,
			actions: []map[string]interface{}{waPageview("/", 1, 100, 0, now)},
		}
		resp := waPostBeat(t, baseURL, beat.body(t, now), map[string]string{"Origin": "https://evil.io", "Referer": ""})
		body := waCloseAndDecode(t, resp)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, true, body["success"], "rejection must be indistinguishable from success")

		buffer.FlushAll(context.Background())
		var count int
		require.NoError(t, restrictedDB.QueryRow(`SELECT COUNT(*) FROM web_sessions WHERE id = $1`, foreignSession).Scan(&count))
		assert.Zero(t, count)

		// Allowed origin passes.
		okBeat := waBeat{
			workspaceID: restricted.ID, sessionID: waUUIDv7At(now, 0x03), seq: 1,
			actions: []map[string]interface{}{waPageview("/", 1, 100, 0, now)},
		}
		resp = waPostBeat(t, baseURL, okBeat.body(t, now), map[string]string{"Origin": "https://app.example.com"})
		_ = waCloseAndDecode(t, resp)
		buffer.FlushAll(context.Background())
		require.NoError(t, restrictedDB.QueryRow(`SELECT COUNT(*) FROM web_sessions`).Scan(&count))
		assert.Equal(t, 1, count)
	})

	t.Run("disabled workspace drops beats silently", func(t *testing.T) {
		beat := waBeat{
			workspaceID: disabledWorkspace.ID, sessionID: waUUIDv7At(now, 0x04), seq: 1,
			actions: []map[string]interface{}{waPageview("/", 1, 100, 0, now)},
		}
		resp := waPostBeat(t, baseURL, beat.body(t, now), nil)
		body := waCloseAndDecode(t, resp)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, true, body["success"])

		buffer.FlushAll(context.Background())
		disabledDB, err := suite.DBManager.GetWorkspaceDB(disabledWorkspace.ID)
		require.NoError(t, err)
		var count int
		require.NoError(t, disabledDB.QueryRow(`SELECT COUNT(*) FROM web_sessions`).Scan(&count))
		assert.Zero(t, count)
	})

	t.Run("session ids outside the window are rejected with 400", func(t *testing.T) {
		old := waBeat{
			workspaceID: workspace.ID, sessionID: waUUIDv7At(now.Add(-72*time.Hour), 0x05), seq: 1,
			actions: []map[string]interface{}{waPageview("/", 1, 100, 0, now)},
		}
		resp := waPostBeat(t, baseURL, old.body(t, now), nil)
		body := waCloseAndDecode(t, resp)
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		assert.Equal(t, false, body["success"])
	})

	t.Run("bot user agents are absorbed", func(t *testing.T) {
		botSession := waUUIDv7At(now, 0x06)
		beat := waBeat{
			workspaceID: workspace.ID, sessionID: botSession, seq: 1,
			actions: []map[string]interface{}{waPageview("/", 1, 100, 0, now)},
		}
		resp := waPostBeat(t, baseURL, beat.body(t, now), map[string]string{"User-Agent": "Googlebot/2.1"})
		body := waCloseAndDecode(t, resp)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, true, body["success"])

		buffer.FlushAll(context.Background())
		var count int
		require.NoError(t, wsDB.QueryRow(`SELECT COUNT(*) FROM web_sessions WHERE id = $1`, botSession).Scan(&count))
		assert.Zero(t, count)
	})
}
