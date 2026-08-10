//go:build integration
// +build integration

package integration

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/tests/testutil"
)

// TestWebAnalyticsMultiTabSession proves that two browser tabs sharing one
// session id are stored as disjoint writers.
//
// Tabs share a session id (it lives in localStorage) but keep their own
// cumulative action list and their own beat counter (both live in
// sessionStorage). So they are independent writers with unrelated seq counters,
// and every layer between the wire and the table has to agree on that: the
// buffer's ordering guard, the batch dedupe, the primary keys, and the session
// aggregates. A regression in any one of them silently drops or overwrites a
// whole tab's browsing, which is invisible in single-tab fixtures.
func TestWebAnalyticsMultiTabSession(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer func() { suite.Cleanup() }()

	workspace, err := suite.DataFactory.CreateWorkspace(func(w *domain.Workspace) {
		w.Settings.WebAnalytics = &domain.WebAnalyticsSettings{
			Enabled: true,
			Filters: domain.DefaultWebFilters(),
		}
	})
	require.NoError(t, err)

	baseURL := suite.ServerManager.GetURL()
	buffer := suite.ServerManager.GetApp().GetWebAnalyticsBuffer()
	wsDB, err := suite.DBManager.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)

	now := time.Now().UTC()
	const (
		tabA int64 = 111
		tabB int64 = 222
	)

	// Explicit entry/exit stamps: the entry/exit assertions turn on exactly
	// those, and the shared waPageview helper hardcodes them.
	page := func(path string, number int, durationMs int64, entered, exited time.Time) map[string]interface{} {
		return map[string]interface{}{
			"type": "pageview", "path": path, "page_number": number,
			"duration": durationMs, "scroll": 10,
			"entered_at": entered.UnixMilli(),
			"exited_at":  exited.UnixMilli(),
		}
	}
	goal := func(name string, tsMs int64) map[string]interface{} {
		return map[string]interface{}{
			"type": "goal", "name": name, "page_number": 1,
			"timestamp": tsMs, "value": 1.5, "path": "/checkout",
		}
	}

	sendBeat := func(t *testing.T, sessionID string, tabID, seq int64, actions []map[string]interface{}) {
		t.Helper()
		body, err := json.Marshal(map[string]interface{}{
			"workspace_id": workspace.ID,
			"session_id":   sessionID,
			"tab_id":       tabID,
			"actions":      actions,
			"attributes": map[string]interface{}{
				"landing_page": "https://shop.example.com/landing",
				"user_agent":   "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 Chrome/126.0.0.0 Safari/537.36",
			},
			"created_at":  now.Add(-15 * time.Minute).UnixMilli(),
			"updated_at":  now.UnixMilli(),
			"sent_at":     now.UnixMilli(),
			"sdk_version": "1.0.0",
			"seq":         seq,
		})
		require.NoError(t, err)
		decoded := waCloseAndDecode(t, waPostBeat(t, baseURL, body, nil))
		require.Equal(t, true, decoded["success"], "beat rejected: %v", decoded)
		buffer.FlushAll(context.Background())
	}

	sessionID := waUUIDv7At(now.Add(-12*time.Minute), 0x71)

	// Tab A opens first and browses two pages; tab B is opened from a link and
	// browses one, which it numbers page 1 — the collision that used to
	// overwrite tab A's first page.
	//
	// Tab A's own last page exits BEFORE tab B's does, and tab B's only page
	// enters AFTER tab A's first: so the session's true entry belongs to tab A
	// and its true exit to tab B. Per-tab flags would mark two of each.
	aP1Enter, aP1Exit := now.Add(-12*time.Minute), now.Add(-10*time.Minute)
	aP2Enter, aP2Exit := now.Add(-10*time.Minute), now.Add(-7*time.Minute)
	bP1Enter, bP1Exit := now.Add(-9*time.Minute), now.Add(-2*time.Minute)

	sendBeat(t, sessionID, tabA, 1, []map[string]interface{}{
		page("/a-first", 1, 1000, aP1Enter, aP1Exit),
	})
	sendBeat(t, sessionID, tabA, 2, []map[string]interface{}{
		page("/a-first", 1, 1000, aP1Enter, aP1Exit),
		page("/a-second", 2, 2000, aP2Enter, aP2Exit),
	})
	// Tab B's counter starts at 1, far behind tab A's. An ordering guard keyed
	// on the session alone discards this beat and everything after it.
	sendBeat(t, sessionID, tabB, 1, []map[string]interface{}{
		page("/b-first", 1, 4000, bP1Enter, bP1Exit),
	})

	t.Run("both tabs' page 1 survive as distinct rows", func(t *testing.T) {
		rows, err := wsDB.Query(
			`SELECT tab_id, page_number, path FROM web_pages WHERE session_id = $1 ORDER BY tab_id, page_number`,
			sessionID)
		require.NoError(t, err)
		defer rows.Close()

		type key struct {
			tab  int64
			page int
		}
		got := map[key]string{}
		for rows.Next() {
			var k key
			var path string
			require.NoError(t, rows.Scan(&k.tab, &k.page, &path))
			got[k] = path
		}
		require.NoError(t, rows.Err())

		assert.Equal(t, map[key]string{
			{tabA, 1}: "/a-first",
			{tabA, 2}: "/a-second",
			{tabB, 1}: "/b-first",
		}, got, "each tab owns its own page numbering")
	})

	t.Run("one session row, aggregated across both tabs", func(t *testing.T) {
		var sessions, pageviews, durationMs int
		require.NoError(t, wsDB.QueryRow(
			`SELECT COUNT(*) FROM web_sessions WHERE id = $1`, sessionID).Scan(&sessions))
		assert.Equal(t, 1, sessions, "tabs share one session row")

		require.NoError(t, wsDB.QueryRow(
			`SELECT pageview_count, duration_ms FROM web_sessions WHERE id = $1`,
			sessionID).Scan(&pageviews, &durationMs))

		// Deliberately not 1 or 2: those are what a single tab's payload would
		// report, and either would look plausible on its own.
		assert.Equal(t, 3, pageviews)
		assert.Equal(t, 7000, durationMs, "engaged time sums across tabs")
	})

	t.Run("exactly one entry and one exit, chosen globally", func(t *testing.T) {
		var landings, exits int
		require.NoError(t, wsDB.QueryRow(
			`SELECT COUNT(*) FILTER (WHERE is_landing), COUNT(*) FILTER (WHERE is_exit)
			 FROM web_pages WHERE session_id = $1`, sessionID).Scan(&landings, &exits))
		assert.Equal(t, 1, landings, "a three-tab visitor must not register three entries")
		assert.Equal(t, 1, exits)

		var landingPath, exitPath string
		require.NoError(t, wsDB.QueryRow(
			`SELECT path FROM web_pages WHERE session_id = $1 AND is_landing`, sessionID).Scan(&landingPath))
		require.NoError(t, wsDB.QueryRow(
			`SELECT path FROM web_pages WHERE session_id = $1 AND is_exit`, sessionID).Scan(&exitPath))
		assert.Equal(t, "/a-first", landingPath, "earliest entry across all tabs")
		assert.Equal(t, "/b-first", exitPath, "latest exit across all tabs, not tab A's own last page")
	})

	t.Run("the same goal in two tabs is two conversions", func(t *testing.T) {
		// Same name, same client millisecond: without tab_id in the key these
		// collapse into one row and half the conversions vanish.
		ts := now.Add(-3 * time.Minute).UnixMilli()
		sendBeat(t, sessionID, tabA, 3, []map[string]interface{}{
			page("/a-first", 1, 1000, aP1Enter, aP1Exit),
			page("/a-second", 2, 2000, aP2Enter, aP2Exit),
			goal("checkout", ts),
		})
		sendBeat(t, sessionID, tabB, 2, []map[string]interface{}{
			page("/b-first", 1, 4000, bP1Enter, bP1Exit),
			goal("checkout", ts),
		})

		var goals int
		require.NoError(t, wsDB.QueryRow(
			`SELECT COUNT(*) FROM web_goals WHERE session_id = $1 AND goal_name = 'checkout'`,
			sessionID).Scan(&goals))
		assert.Equal(t, 2, goals)

		var goalCount int
		require.NoError(t, wsDB.QueryRow(
			`SELECT goal_count FROM web_sessions WHERE id = $1`, sessionID).Scan(&goalCount))
		assert.Equal(t, 2, goalCount, "the session row counts both tabs' conversions")
	})

	t.Run("replaying a stale beat changes nothing", func(t *testing.T) {
		var beforePageviews, beforeDuration, beforeGoals int
		require.NoError(t, wsDB.QueryRow(
			`SELECT pageview_count, duration_ms, goal_count FROM web_sessions WHERE id = $1`,
			sessionID).Scan(&beforePageviews, &beforeDuration, &beforeGoals))

		// Tab A's very first beat, replayed from an offline queue long after
		// both tabs moved on. It carries one page and no goal.
		sendBeat(t, sessionID, tabA, 1, []map[string]interface{}{
			page("/a-first", 1, 1000, aP1Enter, aP1Exit),
		})

		var afterPageviews, afterDuration, afterGoals int
		require.NoError(t, wsDB.QueryRow(
			`SELECT pageview_count, duration_ms, goal_count FROM web_sessions WHERE id = $1`,
			sessionID).Scan(&afterPageviews, &afterDuration, &afterGoals))

		assert.Equal(t, beforePageviews, afterPageviews)
		assert.Equal(t, beforeDuration, afterDuration)
		assert.Equal(t, beforeGoals, afterGoals)
	})
}
