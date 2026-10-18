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

	// Bundles cached before the SDK coerced goal fields, and send queues saved
	// by them, still send these types; a beat carrying them used to fail as a
	// whole.
	t.Run("wrongly typed goals are coerced, and an undecodable one is dropped alone", func(t *testing.T) {
		typedSession := waUUIDv7At(now, 0x07)
		goal := func(name string, extra map[string]interface{}) map[string]interface{} {
			g := map[string]interface{}{"type": "goal", "name": name, "path": "/checkout",
				"page_number": 1, "timestamp": now.UnixMilli()}
			for k, v := range extra {
				g[k] = v
			}
			return g
		}
		beat := waBeat{
			workspaceID: workspace.ID, sessionID: typedSession, seq: 1,
			actions: []map[string]interface{}{
				waPageview("/checkout", 1, 1000, 10, now),
				goal("purchase", map[string]interface{}{"value": "49.90",
					"properties": map[string]interface{}{"order_id": 12345, "first_order": true, "coupon": nil}}),
				goal("lead", map[string]interface{}{"properties": map[string]interface{}{"items": map[string]string{"sku": "A1"}}}),
				goal("signup", map[string]interface{}{"value": 1}),
			},
		}
		resp := waPostBeat(t, baseURL, beat.body(t, now), nil)
		body := waCloseAndDecode(t, resp)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, true, body["success"])
		buffer.FlushAll(context.Background())

		pageviews, _, _, _, _ := waScanSession(t, wsDB, typedSession)
		assert.Equal(t, 1, pageviews, "the rest of the beat is kept")

		rows, err := wsDB.Query(`SELECT goal_name, goal_value, COALESCE(properties::text, '') FROM web_goals WHERE session_id = $1 ORDER BY goal_name`, typedSession)
		require.NoError(t, err)
		defer func() { _ = rows.Close() }()
		type storedGoal struct {
			value      float64
			properties string
		}
		stored := map[string]storedGoal{}
		for rows.Next() {
			var name string
			var g storedGoal
			require.NoError(t, rows.Scan(&name, &g.value, &g.properties))
			stored[name] = g
		}
		require.NoError(t, rows.Err())

		require.Contains(t, stored, "purchase")
		assert.InDelta(t, 49.9, stored["purchase"].value, 1e-4, "a numeric string value is stored as the number")
		assert.JSONEq(t, `{"order_id":"12345","first_order":"true","coupon":""}`, stored["purchase"].properties)
		require.Contains(t, stored, "signup")
		assert.InDelta(t, 1.0, stored["signup"].value, 1e-6)
		assert.NotContains(t, stored, "lead", "a nested property has no flat form, so that goal alone is dropped")
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

// TestWebAnalyticsTrackNullCharacter posts, for each kind of text a beat
// stores, a beat holding a NUL and a clean beat from another visitor, so both
// sit in the same buffer batch. PostgreSQL stores U+0000 in neither text nor
// jsonb, and the flush writes a workspace's batch in one transaction, so the
// NUL used to abort the clean visitor's rows too, and once retrying had failed
// as well both sessions were dropped for good.
func TestWebAnalyticsTrackNullCharacter(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer func() { suite.Cleanup() }()

	workspace, err := suite.DataFactory.CreateWorkspace(func(w *domain.Workspace) {
		w.Settings.WebAnalytics = &domain.WebAnalyticsSettings{Enabled: true, Filters: domain.DefaultWebFilters()}
	})
	require.NoError(t, err)

	baseURL := suite.ServerManager.GetURL()
	buffer := suite.ServerManager.GetApp().GetWebAnalyticsBuffer()
	require.NotNil(t, buffer)
	wsDB, err := suite.DBManager.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)

	now := time.Now().UTC()
	ts := now.UnixMilli()

	// Everything below is JSON text rather than Go values marshalled to JSON, so
	// each \u0000 is exactly the escape that travels on the wire. beatAs takes
	// the session id as JSON text too.
	beatAs := func(sessionIDJSON, fields string) []byte {
		return []byte(fmt.Sprintf(`{"workspace_id":%q,"session_id":%s,"created_at":%d,"updated_at":%d,"sent_at":%d,"seq":1,%s}`,
			workspace.ID, sessionIDJSON, ts-60000, ts, ts, fields))
	}
	beat := func(sessionID, fields string) []byte {
		return beatAs(fmt.Sprintf("%q", sessionID), fields)
	}
	landing := fmt.Sprintf(`{"type":"pageview","path":"/landing","page_number":1,"duration":1000,"scroll":10,"entered_at":%d,"exited_at":%d}`, ts-60000, ts)
	newsletter := fmt.Sprintf(`{"type":"goal","name":"newsletter","goal_type":"lead","path":"/landing","page_number":1,"timestamp":%d}`, ts)
	// goal takes its name, path and properties as JSON text, so a case can write
	// the escape into any of them.
	goal := func(name, path, properties string) string {
		return fmt.Sprintf(`{"type":"goal","name":"%s","goal_type":"lead","path":"%s","page_number":1,"timestamp":%d,"properties":%s}`,
			name, path, ts+1, properties)
	}
	actions := func(extra string) string {
		return `"actions":[` + landing + `,` + newsletter + extra + `]`
	}
	const cleanAttributes = `"attributes":{"landing_page":"https://shop.example.com/landing","referrer":"https://www.google.com/"},"sdk_version":"1.0.0"`

	count := func(t *testing.T, query string, args ...interface{}) int {
		t.Helper()
		var n int
		require.NoError(t, wsDB.QueryRow(query, args...).Scan(&n))
		return n
	}
	sessionText := func(t *testing.T, column, sessionID string) string {
		t.Helper()
		var value string
		require.NoError(t, wsDB.QueryRow(`SELECT `+column+` FROM web_sessions WHERE id = $1`, sessionID).Scan(&value))
		return value
	}
	// keepsOnlyTheCleanActions is what an action carrying a NUL must cost: that
	// action, and nothing else of the beat.
	keepsOnlyTheCleanActions := func(t *testing.T, sessionID string) {
		var paths string
		require.NoError(t, wsDB.QueryRow(`SELECT COALESCE(string_agg(path, ',' ORDER BY page_number), '') FROM web_pages WHERE session_id = $1`, sessionID).Scan(&paths))
		assert.Equal(t, "/landing", paths)
		var goals string
		require.NoError(t, wsDB.QueryRow(`SELECT COALESCE(string_agg(goal_name, ',' ORDER BY goal_name), '') FROM web_goals WHERE session_id = $1`, sessionID).Scan(&goals))
		assert.Equal(t, "newsletter", goals)
	}
	stores := func(column, want string) func(*testing.T, string) {
		return func(t *testing.T, sessionID string) {
			assert.Equal(t, want, sessionText(t, column, sessionID), column)
			keepsOnlyTheCleanActions(t, sessionID)
		}
	}
	// postCleanBeatAndFlush sends the other visitor's beat, flushes, and checks
	// that visitor lost nothing.
	postCleanBeatAndFlush := func(t *testing.T, cleanSession string) {
		resp := waPostBeat(t, baseURL, beat(cleanSession, `"actions":[`+landing+`,`+newsletter+`],`+cleanAttributes), nil)
		_ = waCloseAndDecode(t, resp)
		require.Equal(t, http.StatusOK, resp.StatusCode)

		// The test server never starts the buffer's flush ticker, so both
		// beats are in the next batch. Flushed again so that a failed batch
		// is retried as well: a retry cannot help while the NUL is in it.
		buffer.FlushAll(context.Background())
		buffer.FlushAll(context.Background())

		assert.Equal(t, 1, count(t, `SELECT COUNT(*) FROM web_sessions WHERE id = $1`, cleanSession),
			"the clean visitor's session must not be lost with another visitor's NUL")
		assert.Equal(t, 1, count(t, `SELECT COUNT(*) FROM web_pages WHERE session_id = $1`, cleanSession))
		assert.Equal(t, 1, count(t, `SELECT COUNT(*) FROM web_goals WHERE session_id = $1 AND goal_name = 'newsletter'`, cleanSession))
	}

	cases := []struct {
		name   string
		fields string
		check  func(t *testing.T, sessionID string)
	}{
		{"pageview path", actions(`,{"type":"pageview","path":"/pri\u0000cing","page_number":2}`) + "," + cleanAttributes, keepsOnlyTheCleanActions},
		{"goal name", actions(`,`+goal(`sign\u0000up`, "/landing", `{}`)) + "," + cleanAttributes, keepsOnlyTheCleanActions},
		{"goal path", actions(`,`+goal("signup", `/check\u0000out`, `{}`)) + "," + cleanAttributes, keepsOnlyTheCleanActions},
		{"goal property key", actions(`,`+goal("signup", "/landing", `{"pl\u0000an":"pro"}`)) + "," + cleanAttributes, keepsOnlyTheCleanActions},
		{"goal property value", actions(`,`+goal("signup", "/landing", `{"plan":"pro\u0000"}`)) + "," + cleanAttributes, keepsOnlyTheCleanActions},
		{"UTM tag", actions("") + `,"attributes":{"landing_page":"https://shop.example.com/landing","utm_source":"goo\u0000gle"}`,
			stores("utm_source", "google")},
		{"custom dimension", actions("") + "," + cleanAttributes + `,"dimensions":{"custom_1":"vari\u0000ant"}`,
			stores("custom_1", "variant")},
		{"SDK version", actions("") + `,"attributes":{"landing_page":"https://shop.example.com/landing"},"sdk_version":"1.0\u00000"`,
			stores("sdk_version", "1.00")},
		{"browser details", actions("") + `,"attributes":{"landing_page":"https://shop.example.com/landing",` +
			`"device":"desk\u0000top","browser":"Chr\u0000ome","browser_type":"brow\u0000ser","os":"mac\u0000OS",` +
			`"connection_type":"4\u0000g","language":"en\u0000-US","timezone":"Europe/\u0000Paris","user_agent":"Mozilla/5.0\u0000 (Macintosh)"}`,
			func(t *testing.T, sessionID string) {
				for column, want := range map[string]string{
					"device": "desktop", "browser": "Chrome", "browser_type": "browser", "os": "macOS",
					"connection_type": "4g", "language": "en-US", "timezone": "Europe/Paris", "user_agent": "Mozilla/5.0 (Macintosh)",
				} {
					assert.Equal(t, want, sessionText(t, column, sessionID), column)
				}
				keepsOnlyTheCleanActions(t, sessionID)
			}},
		{"landing page", actions("") + `,"attributes":{"landing_page":"https://shop.example.com/land\u0000ing"}`,
			func(t *testing.T, sessionID string) {
				assert.Equal(t, "https://shop.example.com/landing", sessionText(t, "landing_page", sessionID))
				assert.Equal(t, "/landing", sessionText(t, "landing_path", sessionID))
				keepsOnlyTheCleanActions(t, sessionID)
			}},
		// %00 is ordinary URL text, and what a browser reports for such an
		// address; enrichment decodes the path, which is where the byte appears.
		{"percent-encoded NUL in the landing page", actions("") + `,"attributes":{"landing_page":"https://shop.example.com/sp%00ring"}`,
			func(t *testing.T, sessionID string) {
				assert.Equal(t, "https://shop.example.com/sp%00ring", sessionText(t, "landing_page", sessionID))
				assert.Equal(t, "/sp%00ring", sessionText(t, "landing_path", sessionID))
				keepsOnlyTheCleanActions(t, sessionID)
			}},
		{"percent-encoded NUL in the referrer", actions("") + `,"attributes":{"landing_page":"https://shop.example.com/landing","referrer":"https://news.example.org/a%00b"}`,
			stores("referrer_path", "/a%00b")},
	}

	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			nulSession := waUUIDv7At(now.Add(-time.Minute), byte(0x10+2*i))
			cleanSession := waUUIDv7At(now.Add(-time.Minute), byte(0x11+2*i))

			resp := waPostBeat(t, baseURL, beat(nulSession, tc.fields), nil)
			body := waCloseAndDecode(t, resp)
			require.Equal(t, http.StatusOK, resp.StatusCode, "the SDK reads a 400 as permanent and resends these fields on every beat")
			assert.Equal(t, true, body["success"])

			postCleanBeatAndFlush(t, cleanSession)

			require.Equal(t, 1, count(t, `SELECT COUNT(*) FROM web_sessions WHERE id = $1`, nulSession),
				"the visitor whose beat carried the NUL keeps their session too")
			tc.check(t, nulSession)
		})
	}

	t.Run("session id", func(t *testing.T) {
		// uuid.Parse never reads the first and last byte of an id in the braced
		// form, where the braces go, and the session row stores the id as sent.
		// The SDK only ever sends the canonical form, so refusing such an id
		// costs no visitor their beat.
		inner := waUUIDv7At(now.Add(-time.Minute), 0x7e)
		cleanSession := waUUIDv7At(now.Add(-time.Minute), 0x7f)

		resp := waPostBeat(t, baseURL, beatAs(`"\u0000`+inner+`\u0000"`, actions("")+","+cleanAttributes), nil)
		body := waCloseAndDecode(t, resp)
		// Not a require: where the id is accepted, the checks below show what
		// that costs the other visitor.
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		assert.Contains(t, fmt.Sprint(body["error"]), "session_id")

		postCleanBeatAndFlush(t, cleanSession)

		assert.Zero(t, count(t, `SELECT COUNT(*) FROM web_sessions WHERE id = $1`, inner))
	})
}
