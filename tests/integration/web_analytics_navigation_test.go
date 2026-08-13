//go:build integration
// +build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/tests/testutil"
)

// webNavigationCapForTest mirrors webNavigationMaxPagesPerSession, which is
// unexported. Kept as a named constant so a change to the cap surfaces here as a
// failing assertion rather than as a puzzling number.
const webNavigationCapForTest = 100

// TestWebAnalyticsNavigationTimeline covers recording an identified visitor's
// sessions and pageviews on their contact timeline.
//
// The hard part is not writing the rows, it is that the SDK re-sends the whole
// cumulative session every ~10s and never says "this visit is over". So the
// timeline rows are a PROJECTION of the already-settled analytics tables,
// refreshed on every flush and converging on the visit's final state — and the
// assertions below are written around exactly that: the row count must not grow
// with the number of beats, the content must end up matching the final state,
// and a replay from a cold buffer must change nothing.
func TestWebAnalyticsNavigationTimeline(t *testing.T) {
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

	// An authenticated console user, for the segment-preview subtest. Beats
	// themselves are unauthenticated — that is the whole point of /track — but
	// reading a segment back is an ordinary workspace API call.
	user, err := suite.DataFactory.CreateUser()
	require.NoError(t, err)
	require.NoError(t, suite.DataFactory.AddUserToWorkspace(user.ID, workspace.ID, "owner"))
	require.NoError(t, suite.APIClient.Login(user.Email, "password"))

	baseURL := suite.ServerManager.GetURL()
	buffer := suite.ServerManager.GetApp().GetWebAnalyticsBuffer()
	wsDB, err := suite.DBManager.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)
	now := time.Now().UTC()
	ctx := context.Background()

	// pageview builds an action with explicit timestamps, unlike the shared
	// waPageview helper which stamps every page identically — is_landing and
	// is_exit are resolved by entered_at/exited_at across all of a session's
	// tabs, so pages that share a timestamp cannot be told apart.
	pageview := func(path string, number int, enteredAgo, exitedAgo time.Duration, scroll int) map[string]interface{} {
		return map[string]interface{}{
			"type": "pageview", "path": path, "page_number": number,
			"duration":   int64(enteredAgo-exitedAgo) / int64(time.Millisecond),
			"scroll":     scroll,
			"entered_at": now.Add(-enteredAgo).UnixMilli(),
			"exited_at":  now.Add(-exitedAgo).UnixMilli(),
		}
	}

	// post delivers a beat without flushing. beat() is the usual helper; post()
	// exists for the cap test, which needs several writers inside ONE projection
	// run and so cannot flush between them.
	post := func(t *testing.T, workspaceID, sessionID, email string, tabID, seq int64, actions []map[string]interface{}) {
		t.Helper()
		payload := map[string]interface{}{
			"workspace_id": workspaceID,
			"session_id":   sessionID,
			"tab_id":       tabID,
			"actions":      actions,
			"attributes": map[string]interface{}{
				"landing_page": "https://shop.example.com/pricing",
				"referrer":     "https://www.google.com/",
				"utm_source":   "newsletter",
				"user_agent":   "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 Chrome/126.0.0.0 Safari/537.36",
			},
			"created_at":  now.Add(-10 * time.Minute).UnixMilli(),
			"updated_at":  now.UnixMilli(),
			"sent_at":     now.UnixMilli(),
			"sdk_version": "1.0.0",
			"seq":         seq,
		}
		if email != "" {
			payload["contact_email"] = email
			payload["contact_email_hmac"] = domain.ComputeWebIdentifyHMAC(email, workspace.Settings.SecretKey)
		}
		body, err := json.Marshal(payload)
		require.NoError(t, err)
		decoded := waCloseAndDecode(t, waPostBeat(t, baseURL, body, nil))
		require.Equal(t, true, decoded["success"], "beat rejected: %v", decoded)
	}

	beat := func(t *testing.T, workspaceID, sessionID, email string, tabID, seq int64, actions []map[string]interface{}) {
		t.Helper()
		post(t, workspaceID, sessionID, email, tabID, seq, actions)
		buffer.FlushAll(ctx)
	}

	countRows := func(t *testing.T, email, kind string) int {
		t.Helper()
		var n int
		require.NoError(t, wsDB.QueryRow(
			`SELECT count(*) FROM contact_timeline WHERE email = $1 AND kind = $2`, email, kind).Scan(&n))
		return n
	}

	t.Run("an identified visit lands one row per pageview and one per session", func(t *testing.T) {
		const email = "reader@example.com"
		sessionID := waUUIDv7At(now.Add(-9*time.Minute), 0xB1)

		beat(t, workspace.ID, sessionID, email, 3, 1, []map[string]interface{}{
			pageview("/pricing", 1, 8*time.Minute, 7*time.Minute, 88),
			pageview("/docs", 2, 7*time.Minute, 6*time.Minute, 60),
		})

		assert.Equal(t, 2, countRows(t, email, "web.pageview"))
		assert.Equal(t, 1, countRows(t, email, "web.session"))

		// created_at is the page's own entered_at, not the flush time, so the
		// drawer orders a visit's pages the way the visitor walked them.
		var path string
		var createdAt time.Time
		require.NoError(t, wsDB.QueryRow(`
			SELECT changes->'path'->>'new', created_at
			FROM contact_timeline WHERE email = $1 AND kind = 'web.pageview'
			ORDER BY created_at ASC LIMIT 1`, email).Scan(&path, &createdAt))
		assert.Equal(t, "/pricing", path)
		assert.WithinDuration(t, now.Add(-8*time.Minute), createdAt, time.Second)

		// The {key:{new:value}} envelope is not cosmetic: segment conditions on
		// contact_timeline read ct.changes->'<key>'->>'new'. A flat payload would
		// store the same information somewhere no segment can reach.
		var scroll, duration int
		require.NoError(t, wsDB.QueryRow(`
			SELECT (changes->'max_scroll'->>'new')::int, (changes->'duration_ms'->>'new')::int
			FROM contact_timeline
			WHERE email = $1 AND kind = 'web.pageview' AND changes->'path'->>'new' = '/pricing'`,
			email).Scan(&scroll, &duration))
		assert.Equal(t, 88, scroll)
		assert.Equal(t, 60000, duration)
	})

	t.Run("the session row converges instead of accumulating", func(t *testing.T) {
		const email = "walker@example.com"
		sessionID := waUUIDv7At(now.Add(-9*time.Minute), 0xB2)

		beat(t, workspace.ID, sessionID, email, 1, 1, []map[string]interface{}{
			pageview("/", 1, 8*time.Minute, 7*time.Minute, 20),
		})
		var id string
		var pageviews int
		require.NoError(t, wsDB.QueryRow(`
			SELECT id::text, (changes->'pageview_count'->>'new')::int
			FROM contact_timeline WHERE email = $1 AND kind = 'web.session'`, email).Scan(&id, &pageviews))
		assert.Equal(t, 1, pageviews, "the visit is visible while it is still going")

		// Two more beats, each re-sending the whole cumulative session.
		beat(t, workspace.ID, sessionID, email, 1, 2, []map[string]interface{}{
			pageview("/", 1, 8*time.Minute, 7*time.Minute, 20),
			pageview("/features", 2, 7*time.Minute, 5*time.Minute, 45),
		})
		beat(t, workspace.ID, sessionID, email, 1, 3, []map[string]interface{}{
			pageview("/", 1, 8*time.Minute, 7*time.Minute, 20),
			pageview("/features", 2, 7*time.Minute, 5*time.Minute, 45),
			pageview("/signup", 3, 5*time.Minute, 4*time.Minute, 100),
		})

		var afterID string
		var afterPageviews, afterDuration int
		require.NoError(t, wsDB.QueryRow(`
			SELECT id::text, (changes->'pageview_count'->>'new')::int, (changes->'duration_ms'->>'new')::int
			FROM contact_timeline WHERE email = $1 AND kind = 'web.session'`, email).
			Scan(&afterID, &afterPageviews, &afterDuration))
		assert.Equal(t, id, afterID, "three beats must update one row, not append three")
		assert.Equal(t, 3, afterPageviews)
		assert.Equal(t, 240000, afterDuration, "the summary carries the whole visit")

		assert.Equal(t, 1, countRows(t, email, "web.session"))
		assert.Equal(t, 3, countRows(t, email, "web.pageview"))

		// The last page a visitor reaches is the one a settle-signal design loses:
		// the buffer marks an entry clean on any flush and only a new beat marks it
		// dirty again, so "flush once more when the session goes idle" never runs
		// for a writer whose last beat landed just before a periodic flush.
		var exitPath string
		require.NoError(t, wsDB.QueryRow(`
			SELECT changes->'path'->>'new' FROM contact_timeline
			WHERE email = $1 AND kind = 'web.pageview' AND (changes->'is_exit'->>'new')::boolean`,
			email).Scan(&exitPath))
		assert.Equal(t, "/signup", exitPath)
	})

	t.Run("a replay from a cold buffer changes nothing", func(t *testing.T) {
		// Restart, eviction and a second replica all look like this. The buffer
		// keeps no cursor for navigation, so idempotency has to come from the
		// database — the timeline id is derived from the session, tab and page.
		const email = "returning@example.com"
		sessionID := waUUIDv7At(now.Add(-9*time.Minute), 0xB3)
		actions := []map[string]interface{}{
			pageview("/pricing", 1, 8*time.Minute, 7*time.Minute, 30),
			pageview("/docs", 2, 7*time.Minute, 6*time.Minute, 70),
		}

		beat(t, workspace.ID, sessionID, email, 5, 1, actions)
		before := countRows(t, email, "web.pageview")
		require.Equal(t, 2, before)

		var ids []string
		rows, err := wsDB.Query(
			`SELECT id::text FROM contact_timeline WHERE email = $1 AND kind = 'web.pageview' ORDER BY created_at`, email)
		require.NoError(t, err)
		for rows.Next() {
			var id string
			require.NoError(t, rows.Scan(&id))
			ids = append(ids, id)
		}
		require.NoError(t, rows.Err())
		require.NoError(t, rows.Close())

		// Same beat again, higher seq — the whole cumulative payload, exactly as a
		// replayed offline queue or a second process would deliver it.
		beat(t, workspace.ID, sessionID, email, 5, 2, actions)

		assert.Equal(t, 2, countRows(t, email, "web.pageview"), "a replay must not duplicate a visit")
		assert.Equal(t, 1, countRows(t, email, "web.session"))
		var afterIDs []string
		rows, err = wsDB.Query(
			`SELECT id::text FROM contact_timeline WHERE email = $1 AND kind = 'web.pageview' ORDER BY created_at`, email)
		require.NoError(t, err)
		for rows.Next() {
			var id string
			require.NoError(t, rows.Scan(&id))
			afterIDs = append(afterIDs, id)
		}
		require.NoError(t, rows.Err())
		require.NoError(t, rows.Close())
		assert.Equal(t, ids, afterIDs, "the same visit must keep the same rows")
	})

	t.Run("two tabs of one session keep their own pages", func(t *testing.T) {
		const email = "multitab@example.com"
		sessionID := waUUIDv7At(now.Add(-9*time.Minute), 0xB4)

		beat(t, workspace.ID, sessionID, email, 11, 1, []map[string]interface{}{
			pageview("/tab-a", 1, 8*time.Minute, 7*time.Minute, 10),
		})
		beat(t, workspace.ID, sessionID, email, 22, 1, []map[string]interface{}{
			pageview("/tab-b", 1, 6*time.Minute, 5*time.Minute, 20),
		})

		// Both tabs number their pages from 1; the timeline id carries tab_id for
		// the same reason the web_pages primary key does.
		assert.Equal(t, 2, countRows(t, email, "web.pageview"))
		assert.Equal(t, 1, countRows(t, email, "web.session"), "one visit, however many tabs")
	})

	t.Run("the page cap bounds a projection run across all of a session's tabs", func(t *testing.T) {
		// The cap has to be a window over the session, not `page_number <= N`.
		// Page numbers are per tab and every tab renumbers from 1, so an ordinal
		// test bounds each tab separately — and TabID is an unvalidated int64, so
		// a caller holding one valid credential can mint a fresh tab per beat and
		// multiply the cap by however many tabs it claims.
		//
		// Both tabs are posted before a single flush, because the cap bounds one
		// projection RUN. Flushing between them writes tab A's pages, and those
		// rows stay when the second run's window ranks them out — so the stored
		// total for a session can legitimately exceed the cap. That residual is
		// documented on webNavigationMaxPagesPerSession; what is pinned here is
		// the bound that does hold.
		const email = "crawler@example.com"
		sessionID := waUUIDv7At(now.Add(-9*time.Minute), 0xB7)
		const perTab = 60 // two tabs of 60 straddle the cap of 100

		for tab := int64(1); tab <= 2; tab++ {
			actions := make([]map[string]interface{}, 0, perTab)
			for page := 1; page <= perTab; page++ {
				offset := time.Duration(page) * time.Second
				actions = append(actions, pageview(
					fmt.Sprintf("/tab-%d/page-%d", tab, page), page,
					8*time.Minute-offset, 8*time.Minute-offset-500*time.Millisecond, 10))
			}
			post(t, workspace.ID, sessionID, email, tab, 1, actions)
		}
		buffer.FlushAll(ctx)

		var pages int
		require.NoError(t, wsDB.QueryRow(
			`SELECT count(*) FROM web_pages WHERE session_id = $1`, sessionID).Scan(&pages))
		require.Equal(t, 2*perTab, pages, "precondition: both tabs' pages are recorded in analytics")

		assert.Equal(t, webNavigationCapForTest, countRows(t, email, "web.pageview"),
			"one run must not write more than the cap, however many tabs the session claims")
	})

	t.Run("a segment can be built on the recorded navigation", func(t *testing.T) {
		// The reason the projection writes the {key:{new:value}} envelope and the
		// raw kind: both are what a contact_timeline segment condition reads. The
		// console offers these two kinds in the Activity dropdown, so this closes
		// the loop from a beat to a segment that matches a real contact.
		const email = "segmented@example.com"
		sessionID := waUUIDv7At(now.Add(-9*time.Minute), 0xB8)

		beat(t, workspace.ID, sessionID, email, 4, 1, []map[string]interface{}{
			pageview("/pricing", 1, 8*time.Minute, 7*time.Minute, 40),
			pageview("/docs", 2, 7*time.Minute, 6*time.Minute, 50),
			pageview("/signup", 3, 6*time.Minute, 5*time.Minute, 60),
		})

		preview := func(t *testing.T, kind string, count int) int {
			t.Helper()
			resp, err := suite.APIClient.Post("/api/segments.preview", map[string]interface{}{
				"workspace_id": workspace.ID,
				"tree": map[string]interface{}{
					"kind": "leaf",
					"leaf": map[string]interface{}{
						"source": "contact_timeline",
						"contact_timeline": map[string]interface{}{
							"kind":           kind,
							"count_operator": "at_least",
							"count_value":    count,
						},
					},
				},
				"limit": 20,
			})
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()
			require.Equal(t, 200, resp.StatusCode, "the API must accept a web kind")

			var result map[string]interface{}
			require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
			return int(result["total_count"].(float64))
		}

		previewFiltered := func(t *testing.T, kind string, count int, filter map[string]interface{}) int {
			t.Helper()
			resp, err := suite.APIClient.Post("/api/segments.preview", map[string]interface{}{
				"workspace_id": workspace.ID,
				"tree": map[string]interface{}{
					"kind": "leaf",
					"leaf": map[string]interface{}{
						"source": "contact_timeline",
						"contact_timeline": map[string]interface{}{
							"kind":           kind,
							"count_operator": "at_least",
							"count_value":    count,
							"filters":        []map[string]interface{}{filter},
						},
					},
				},
				"limit": 20,
			})
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()
			require.Equal(t, 200, resp.StatusCode, "a filter on the event payload must be accepted")

			var result map[string]interface{}
			require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
			return int(result["total_count"].(float64))
		}

		assert.GreaterOrEqual(t, preview(t, "web.pageview", 3), 1,
			"a visitor with three pageviews must satisfy 'at least 3'")

		// The filter the console now builds: a condition on the event's own
		// payload, compiled as ct.changes->'path'->>'new'. This is what the
		// {key:{new:value}} envelope exists for.
		assert.GreaterOrEqual(t, previewFiltered(t, "web.pageview", 1, map[string]interface{}{
			"field_name":    "path",
			"field_type":    "string",
			"operator":      "equals",
			"string_values": []string{"/signup"},
		}), 1, "a path filter must match the visitor who viewed that path")

		assert.Equal(t, 0, previewFiltered(t, "web.pageview", 1, map[string]interface{}{
			"field_name":    "path",
			"field_type":    "string",
			"operator":      "equals",
			"string_values": []string{"/never-visited-by-anyone"},
		}), "and must not match a path nobody viewed — otherwise the filter is being ignored")

		// A numeric filter goes through a cast, so it is worth its own assertion.
		assert.GreaterOrEqual(t, previewFiltered(t, "web.pageview", 1, map[string]interface{}{
			"field_name":    "duration_ms",
			"field_type":    "number",
			"operator":      "gte",
			"number_values": []float64{30000},
		}), 1, "a numeric filter on the payload must compile and match")
		assert.GreaterOrEqual(t, preview(t, "web.session", 1), 1,
			"one visit must satisfy 'at least 1'")
		// Deliberately far above anything the other subtests leave behind — they
		// share this workspace, and the cap subtest alone parks a contact on 100
		// pageviews, so a threshold near that would match for reasons unrelated to
		// what is being asserted.
		assert.Equal(t, 0, preview(t, "web.pageview", 10000),
			"and the count still has to be met — otherwise the condition is not being applied")
	})

	t.Run("a converged value re-queues the contact for segment recomputation", func(t *testing.T) {
		// The timeline queue trigger is AFTER INSERT, so it fires when a row is
		// first written and never again — but these rows converge, so without an
		// explicit re-queue a segment filtering on duration_ms would evaluate the
		// visit as it stood at its first flush. Now that the console can filter on
		// those values, that lag would show up as a segment that under-matches.
		const email = "converging@example.com"
		sessionID := waUUIDv7At(now.Add(-9*time.Minute), 0xB9)

		drain := func(t *testing.T) {
			t.Helper()
			_, err := wsDB.Exec(`DELETE FROM contact_segment_queue WHERE email = $1`, email)
			require.NoError(t, err)
		}
		queued := func(t *testing.T) int {
			t.Helper()
			var n int
			require.NoError(t, wsDB.QueryRow(
				`SELECT count(*) FROM contact_segment_queue WHERE email = $1`, email).Scan(&n))
			return n
		}

		beat(t, workspace.ID, sessionID, email, 1, 1, []map[string]interface{}{
			pageview("/read", 1, 8*time.Minute, 7*time.Minute, 20),
		})
		drain(t)

		// Same page, longer: no new row, only an update. Under the old behaviour
		// this queued nothing at all.
		beat(t, workspace.ID, sessionID, email, 1, 2, []map[string]interface{}{
			pageview("/read", 1, 8*time.Minute, 4*time.Minute, 90),
		})

		assert.Equal(t, 1, queued(t), "an updated visit must be re-evaluated")

		var duration, scroll int
		require.NoError(t, wsDB.QueryRow(`
			SELECT (changes->'duration_ms'->>'new')::int, (changes->'max_scroll'->>'new')::int
			FROM contact_timeline WHERE email = $1 AND kind = 'web.pageview'`,
			email).Scan(&duration, &scroll))
		assert.Equal(t, 240000, duration, "and the value it is re-evaluated against is the current one")
		assert.Equal(t, 90, scroll)

		// A flush that changes nothing must stay silent, or every heartbeat of
		// every identified visitor becomes a segment recomputation.
		drain(t)
		beat(t, workspace.ID, sessionID, email, 1, 3, []map[string]interface{}{
			pageview("/read", 1, 8*time.Minute, 4*time.Minute, 90),
		})
		assert.Equal(t, 0, queued(t), "an unchanged re-projection must not queue anything")
	})

	t.Run("a run spanning many sessions and two dates projects all of them", func(t *testing.T) {
		// Every other subtest projects exactly one session on one date, so the
		// chunk loop, the per-date id set and the slice bounds are only ever
		// exercised at len(ids)==1 — the paths that bound lock hold time have no
		// coverage at all. This drives the repository directly: getting 200+
		// concurrent sessions through the HTTP path would be slow and would prove
		// less, since what is under test is the chunking, not ingest.
		const email = "fleet@example.com"
		_, err := suite.DataFactory.CreateContact(workspace.ID, func(c *domain.Contact) {
			c.Email = email
		})
		require.NoError(t, err)

		repo := suite.ServerManager.GetApp().GetWebAnalyticsRepository()
		// Two dates, and more sessions than one chunk holds.
		const perDate = 130
		days := []time.Time{now.AddDate(0, 0, -1), now}
		var sessions []*domain.WebSession

		for dayIndex, day := range days {
			sessionDate := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.UTC)
			for i := 0; i < perDate; i++ {
				sessionID := waUUIDv7At(day.Add(-time.Duration(i+1)*time.Minute), byte(0xD0+dayIndex))
				// Distinct ids: waUUIDv7At derives them from the timestamp, and a
				// collision would silently shrink the batch under test.
				sessionID = fmt.Sprintf("%s%04x", sessionID[:len(sessionID)-4], i)

				_, err := wsDB.Exec(`
					INSERT INTO web_sessions (session_date, id, created_at, updated_at, contact_email, pageview_count, duration_ms)
					VALUES ($1, $2, $3, $3, $4, 1, 1000)`,
					sessionDate, sessionID, day.Add(-time.Duration(i+1)*time.Minute), email)
				require.NoError(t, err)
				_, err = wsDB.Exec(`
					INSERT INTO web_pages (session_date, session_id, tab_id, page_number, path, entered_at, exited_at, duration_ms, contact_email)
					VALUES ($1, $2, 1, 1, $3, $4, $5, 1000, $6)`,
					sessionDate, sessionID, fmt.Sprintf("/bulk-%d-%d", dayIndex, i),
					day.Add(-time.Duration(i+1)*time.Minute), day.Add(-time.Duration(i)*time.Minute), email)
				require.NoError(t, err)

				sessions = append(sessions, &domain.WebSession{
					SessionDate: sessionDate, ID: sessionID, ContactEmail: &[]string{email}[0],
				})
			}
		}

		require.NoError(t, repo.ProjectContactNavigation(ctx, workspace.ID, sessions))

		assert.Equal(t, len(days)*perDate, countRows(t, email, "web.session"),
			"every session of every date must be projected, across chunk boundaries")
		assert.Equal(t, len(days)*perDate, countRows(t, email, "web.pageview"))

		// One queue row, not one per chunk: the queue upserts on email.
		var queued int
		require.NoError(t, wsDB.QueryRow(
			`SELECT count(*) FROM contact_segment_queue WHERE email = $1`, email).Scan(&queued))
		assert.Equal(t, 1, queued)

		// And re-running the whole thing changes nothing.
		require.NoError(t, repo.ProjectContactNavigation(ctx, workspace.ID, sessions))
		assert.Equal(t, len(days)*perDate, countRows(t, email, "web.session"))
	})

	t.Run("a visitor who stops identifying still gets their visit completed", func(t *testing.T) {
		// Logout, or an identify token lapsing mid-visit. contact_email is sticky
		// in the database, so the rows stay identified while the beats go
		// anonymous — and the timeline must keep converging rather than freeze on
		// whatever the last identified beat said.
		const email = "loggedout@example.com"
		sessionID := waUUIDv7At(now.Add(-9*time.Minute), 0xBA)

		beat(t, workspace.ID, sessionID, email, 1, 1, []map[string]interface{}{
			pageview("/account", 1, 8*time.Minute, 7*time.Minute, 20),
		})
		// Same session, no credential from here on.
		beat(t, workspace.ID, sessionID, "", 1, 2, []map[string]interface{}{
			pageview("/account", 1, 8*time.Minute, 7*time.Minute, 20),
			pageview("/logout", 2, 7*time.Minute, 6*time.Minute, 40),
		})

		// The page viewed AFTER logging out is genuinely anonymous: the sticky
		// COALESCE preserves identity on rows that already exist, and this one
		// was created by an anonymous beat. So it gets no timeline row of its
		// own, which is the right answer — they were not identified when they
		// viewed it.
		assert.Equal(t, 1, countRows(t, email, "web.pageview"))

		// The visit itself is still theirs, and must keep converging.
		var pageviews int
		var exitPath string
		require.NoError(t, wsDB.QueryRow(`
			SELECT (changes->'pageview_count'->>'new')::int, changes->'exit_path'->>'new'
			FROM contact_timeline WHERE email = $1 AND kind = 'web.session'`, email).Scan(&pageviews, &exitPath))
		assert.Equal(t, 2, pageviews, "the summary must not freeze at the last identified beat")
		assert.Equal(t, "/logout", exitPath)
	})

	t.Run("anonymous browsing writes nothing", func(t *testing.T) {
		sessionID := waUUIDv7At(now.Add(-9*time.Minute), 0xB5)
		beat(t, workspace.ID, sessionID, "", 1, 1, []map[string]interface{}{
			pageview("/pricing", 1, 8*time.Minute, 7*time.Minute, 50),
		})

		// Scoped by entity_id, not by changes->'session_id': only the PAGE
		// projection emits that key, so the previous form could not count a
		// web.session row at all and the session half of this guard could never
		// fail. entity_id is the session id for the summary and is prefixed by it
		// for each page.
		var n int
		require.NoError(t, wsDB.QueryRow(
			`SELECT count(*) FROM contact_timeline
			 WHERE kind IN ('web.pageview', 'web.session')
			   AND (entity_id = $1 OR entity_id LIKE $1 || ':%')`, sessionID).Scan(&n))
		assert.Equal(t, 0, n, "there is no contact to attach an anonymous visit to")
	})

	t.Run("a deleted contact gets no rows even while the web rows still name them", func(t *testing.T) {
		// The erasure guard. AnonymizeContact clears contact_email on the analytics
		// rows, but a beat buffered before the deletion still carries the address —
		// so the projection joins contacts rather than trusting the row.
		const email = "erased@example.com"
		sessionID := waUUIDv7At(now.Add(-9*time.Minute), 0xB6)

		_, err := wsDB.Exec(`
			INSERT INTO web_sessions (session_date, id, created_at, updated_at, contact_email, pageview_count, duration_ms)
			VALUES ($1, $2, $3, $3, $4, 1, 1000)`,
			now.Format("2006-01-02"), sessionID, now.Add(-8*time.Minute), email)
		require.NoError(t, err)
		_, err = wsDB.Exec(`
			INSERT INTO web_pages (session_date, session_id, tab_id, page_number, path, entered_at, exited_at, duration_ms, contact_email)
			VALUES ($1, $2, 1, 1, '/ghost', $3, $4, 1000, $5)`,
			now.Format("2006-01-02"), sessionID, now.Add(-8*time.Minute), now.Add(-7*time.Minute), email)
		require.NoError(t, err)

		require.NoError(t, suite.ServerManager.GetApp().GetWebAnalyticsRepository().
			ProjectContactNavigation(ctx, workspace.ID, []*domain.WebSession{{
				SessionDate:  time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC),
				ID:           sessionID,
				ContactEmail: &[]string{email}[0],
			}}))

		assert.Equal(t, 0, countRows(t, email, "web.pageview"))
		assert.Equal(t, 0, countRows(t, email, "web.session"))
	})
}

// TestWebAnalyticsNavigationNeedsNoSetting pins what replaced the opt-in.
//
// Recording an identified visitor's navigation used to be gated by a
// record_contact_navigation workspace setting. Calling identify() is the opt-in
// now — minting the credential takes the workspace secret, so the customer's own
// server has already decided this visitor may be tied to a contact — and there
// is no setting left to switch on. A workspace that wants anonymous reporting
// simply never calls identify(), which the anonymous case below is here to show.
func TestWebAnalyticsNavigationNeedsNoSetting(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer func() { suite.Cleanup() }()

	// Nothing beyond Enabled: no contact-timeline settings exist.
	workspace, err := suite.DataFactory.CreateWorkspace(func(w *domain.Workspace) {
		w.Settings.WebAnalytics = &domain.WebAnalyticsSettings{
			Enabled: true,
			Filters: domain.DefaultWebFilters(),
		}
	})
	require.NoError(t, err)

	wsDB, err := suite.DBManager.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)
	now := time.Now().UTC()
	baseURL := suite.ServerManager.GetURL()
	buffer := suite.ServerManager.GetApp().GetWebAnalyticsBuffer()

	post := func(t *testing.T, sessionID, email string) {
		t.Helper()
		payload := map[string]interface{}{
			"workspace_id": workspace.ID,
			"session_id":   sessionID,
			"tab_id":       1,
			"actions": []map[string]interface{}{
				waPageview("/pricing", 1, 1000, 20, now),
				waPageview("/docs", 2, 2000, 40, now),
			},
			"attributes": map[string]interface{}{
				"landing_page": "https://shop.example.com/pricing",
				"user_agent":   "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 Chrome/126.0.0.0 Safari/537.36",
			},
			"created_at":  now.Add(-4 * time.Minute).UnixMilli(),
			"updated_at":  now.UnixMilli(),
			"sent_at":     now.UnixMilli(),
			"sdk_version": "1.0.0",
			"seq":         1,
		}
		if email != "" {
			payload["contact_email"] = email
			payload["contact_email_hmac"] = domain.ComputeWebIdentifyHMAC(email, workspace.Settings.SecretKey)
		}
		body, err := json.Marshal(payload)
		require.NoError(t, err)
		decoded := waCloseAndDecode(t, waPostBeat(t, baseURL, body, nil))
		require.Equal(t, true, decoded["success"])
		buffer.FlushAll(context.Background())
	}

	countRows := func(t *testing.T, email string) int {
		t.Helper()
		var n int
		require.NoError(t, wsDB.QueryRow(
			`SELECT count(*) FROM contact_timeline WHERE email = $1 AND kind IN ('web.pageview', 'web.session')`,
			email).Scan(&n))
		return n
	}

	t.Run("identifying is enough to record navigation", func(t *testing.T) {
		const email = "nosetting@example.com"
		post(t, waUUIDv7At(now.Add(-5*time.Minute), 0xC1), email)

		assert.Equal(t, 3, countRows(t, email), "two pageviews and the visit summary")
	})

	t.Run("an anonymous visit still records nothing", func(t *testing.T) {
		sessionID := waUUIDv7At(now.Add(-5*time.Minute), 0xC2)
		post(t, sessionID, "")

		var pages int
		require.NoError(t, wsDB.QueryRow(
			`SELECT count(*) FROM web_pages WHERE session_id = $1`, sessionID).Scan(&pages))
		require.Equal(t, 2, pages, "precondition: the analytics rows are recorded either way")

		var timelineRows int
		require.NoError(t, wsDB.QueryRow(
			`SELECT count(*) FROM contact_timeline
			 WHERE kind IN ('web.pageview', 'web.session')
			   AND (entity_id = $1 OR entity_id LIKE $1 || ':%')`, sessionID).Scan(&timelineRows))
		assert.Equal(t, 0, timelineRows, "no identity, no timeline")
	})
}
