//go:build integration
// +build integration

package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/database/schema"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/tests/testutil"
)

// TestWebAnalyticsConsoleQueryShapes exercises the queries the console's web
// analytics tabs actually emit, as opposed to the engine's capabilities in
// general.
//
// The two differ in ways that matter. Dashboard and explore widgets group by a
// dimension, so they cannot put their range on a time dimension — a
// granularity would split every row per bucket — and instead send it as a
// plain inDateRange filter holding absolute instants, converted from the
// operator's local days in the browser. That path has its own date parsing and
// its own partition-pruning predicate, and a mistake in either silently drops
// rows from a report rather than failing anything.
func TestWebAnalyticsConsoleQueryShapes(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer func() { suite.Cleanup() }()

	workspace, err := suite.DataFactory.CreateWorkspace(func(w *domain.Workspace) {
		w.Settings.WebAnalytics = &domain.WebAnalyticsSettings{
			Enabled:                true,
			BounceThresholdSeconds: 10,
			Filters:                domain.DefaultWebFilters(),
		}
	})
	require.NoError(t, err)

	workspaceRepo := suite.ServerManager.GetApp().GetWorkspaceRepository()
	require.NoError(t, workspaceRepo.AddUserToWorkspace(context.Background(), &domain.UserWorkspace{
		UserID:      "550e8400-e29b-41d4-a716-446655440000", // testuser@example.com
		WorkspaceID: workspace.ID,
		Role:        "owner",
		Permissions: domain.FullPermissions,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}))

	wsDB, err := suite.DBManager.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)

	// Rows are written directly so created_at can sit days in the past: the
	// ingest path always stamps sessions with the current time, which cannot
	// express a period boundary.
	now := time.Now().UTC()
	for _, month := range []time.Time{now, now.AddDate(0, 0, -7), now.AddDate(0, -1, 0)} {
		for _, table := range schema.WebAnalyticsTableNames {
			_, err := wsDB.Exec(schema.WebAnalyticsPartitionDDL(table, month))
			require.NoError(t, err)
		}
	}

	insertSession := func(salt byte, createdAt time.Time, channel, utmSource, utmCampaign string, durationMs, pageviews int) string {
		id := waUUIDv7At(createdAt, salt)
		_, err := wsDB.Exec(`
			INSERT INTO web_sessions (session_date, id, beat_seq, created_at, updated_at,
				duration_ms, pageview_count, channel, utm_source, utm_campaign)
			VALUES ($1, $2, 1, $3, $3, $4, $5, $6, $7, $8)`,
			createdAt.Format("2006-01-02"), id, createdAt, durationMs, pageviews,
			channel, utmSource, utmCampaign)
		require.NoError(t, err)
		return id
	}

	insertGoal := func(sessionID string, sessionDate, goalAt time.Time, name string, value float64) {
		_, err := wsDB.Exec(`
			INSERT INTO web_goals (session_date, session_id, goal_name, client_ts_ms, beat_seq,
				goal_at, goal_value, channel)
			VALUES ($1, $2, $3, $4, 1, $5, $6, 'google-ads')`,
			sessionDate.Format("2006-01-02"), sessionID, name, goalAt.UnixMilli(), goalAt, value)
		require.NoError(t, err)
	}

	// Current period: four sessions across three days. Previous period: one,
	// four days back, which must never appear in a current-period report.
	first := insertSession(0x21, now.Add(-1*time.Hour), "google-ads", "google", "launch", 30000, 1)
	second := insertSession(0x22, now.Add(-2*time.Hour), "google-ads", "google", "", 10000, 1)
	insertSession(0x23, now.Add(-3*time.Hour), "organic-search", "google", "", 20000, 2)
	older := insertSession(0x24, now.AddDate(0, 0, -2), "google-ads", "bing", "always-on", 5000, 1)
	insertSession(0x25, now.AddDate(0, 0, -4), "newsletter", "newsletter", "", 40000, 3)

	insertGoal(first, now.Add(-1*time.Hour), now.Add(-1*time.Hour), "purchase", 100)
	insertGoal(second, now.Add(-2*time.Hour), now.Add(-2*time.Hour), "purchase", 50)
	insertGoal(older, now.AddDate(0, 0, -2), now.AddDate(0, 0, -2), "signup", 0)

	require.NoError(t, suite.APIClient.Login("testuser@example.com", ""))

	// What the browser sends: local day boundaries already converted to
	// instants, RFC 3339 with milliseconds.
	instant := func(ts time.Time) string { return ts.Format("2006-01-02T15:04:05.000Z07:00") }
	currentStart := instant(now.AddDate(0, 0, -2).Truncate(24 * time.Hour))
	currentEnd := instant(now.Truncate(24 * time.Hour).Add(24*time.Hour - time.Millisecond))
	previousStart := instant(now.AddDate(0, 0, -5).Truncate(24 * time.Hour))
	previousEnd := instant(now.AddDate(0, 0, -3).Truncate(24 * time.Hour).Add(24*time.Hour - time.Millisecond))

	rangeFilter := func(start, end string) map[string]interface{} {
		return map[string]interface{}{
			"member": "created_at", "operator": "inDateRange", "values": []string{start, end},
		}
	}

	sessionsByChannel := func(result analyticsQueryResponse) map[string]float64 {
		byChannel := map[string]float64{}
		for _, row := range result.Data {
			channel, _ := row["channel"].(string)
			byChannel[channel] += waNumber(t, row["sessions"])
		}
		return byChannel
	}

	t.Run("dashboard breakdown: the range rides a filter, not a time dimension", func(t *testing.T) {
		result := waRunQuery(t, suite, workspace.ID, map[string]interface{}{
			"schema":     "web_sessions",
			"measures":   []string{"sessions", "median_duration"},
			"dimensions": []string{"channel"},
			"timezone":   "UTC",
			"filters":    []map[string]interface{}{rangeFilter(currentStart, currentEnd)},
			"order":      map[string]string{"sessions": "desc"},
			"limit":      7,
		})

		byChannel := sessionsByChannel(result)
		assert.Equal(t, float64(3), byChannel["google-ads"],
			"includes the session from two days ago, which the pruning predicate must not cut")
		assert.Equal(t, float64(1), byChannel["organic-search"])
		assert.NotContains(t, byChannel, "newsletter", "the previous period must stay out")
	})

	t.Run("absolute instants mean the same thing in any query timezone", func(t *testing.T) {
		// The bounds are already instants, so the timezone only drives
		// bucketing. If the engine ever reinterpreted them as local wall
		// clock, this count would drift by a day at the edges.
		utc := waRunQuery(t, suite, workspace.ID, map[string]interface{}{
			"schema":   "web_sessions",
			"measures": []string{"sessions"},
			"timezone": "UTC",
			"filters":  []map[string]interface{}{rangeFilter(currentStart, currentEnd)},
		})
		paris := waRunQuery(t, suite, workspace.ID, map[string]interface{}{
			"schema":   "web_sessions",
			"measures": []string{"sessions"},
			"timezone": "Europe/Paris",
			"filters":  []map[string]interface{}{rangeFilter(currentStart, currentEnd)},
		})

		require.Len(t, utc.Data, 1)
		require.Len(t, paris.Data, 1)
		assert.Equal(t, float64(4), waNumber(t, utc.Data[0]["sessions"]))
		assert.Equal(t, waNumber(t, utc.Data[0]["sessions"]), waNumber(t, paris.Data[0]["sessions"]))
	})

	t.Run("the comparison period is the same query over the earlier window", func(t *testing.T) {
		result := waRunQuery(t, suite, workspace.ID, map[string]interface{}{
			"schema":     "web_sessions",
			"measures":   []string{"sessions"},
			"dimensions": []string{"channel"},
			"timezone":   "UTC",
			"filters":    []map[string]interface{}{rangeFilter(previousStart, previousEnd)},
		})

		byChannel := sessionsByChannel(result)
		assert.Equal(t, float64(1), byChannel["newsletter"])
		assert.NotContains(t, byChannel, "google-ads",
			"the two windows must not overlap, or every change rate is wrong")
	})

	t.Run("explore drill-down: ancestor filter plus the min-sessions threshold", func(t *testing.T) {
		// What expanding a "google-ads" row sends: group by the parent and the
		// child dimension, filter on the parent's value, and drop combinations
		// under the threshold.
		result := waRunQuery(t, suite, workspace.ID, map[string]interface{}{
			"schema":     "web_sessions",
			"measures":   []string{"sessions", "bounce_rate", "median_scroll"},
			"dimensions": []string{"channel", "utm_source"},
			"timezone":   "UTC",
			"filters": []map[string]interface{}{
				rangeFilter(currentStart, currentEnd),
				{"member": "channel", "operator": "equals", "values": []string{"google-ads"}},
			},
			"having": []map[string]interface{}{
				{"member": "sessions", "operator": "gte", "values": []string{"2"}},
			},
			"order": map[string]string{"sessions": "desc"},
			"limit": 100,
		})

		require.Len(t, result.Data, 1, "bing has a single session and is below the threshold")
		assert.Equal(t, "google-ads", result.Data[0]["channel"])
		assert.Equal(t, "google", result.Data[0]["utm_source"])
		assert.Equal(t, float64(2), waNumber(t, result.Data[0]["sessions"]))
	})

	t.Run("is-not-empty is an inequality against the empty string", func(t *testing.T) {
		// Dimensions are NOT NULL DEFAULT '', so the console translates its
		// isEmpty/isNotEmpty operators to comparisons with ''. A translation
		// to set/notSet instead would match every row, since none are NULL.
		result := waRunQuery(t, suite, workspace.ID, map[string]interface{}{
			"schema":     "web_sessions",
			"measures":   []string{"sessions"},
			"dimensions": []string{"utm_campaign"},
			"timezone":   "UTC",
			"filters": []map[string]interface{}{
				rangeFilter(currentStart, currentEnd),
				{"member": "utm_campaign", "operator": "notEquals", "values": []string{""}},
			},
		})

		campaigns := map[string]float64{}
		for _, row := range result.Data {
			campaign, _ := row["utm_campaign"].(string)
			campaigns[campaign] += waNumber(t, row["sessions"])
		}
		assert.Equal(t, map[string]float64{"launch": 1, "always-on": 1}, campaigns)
	})

	t.Run("a breakdown with nothing to show is empty, not one phantom row", func(t *testing.T) {
		// Every dashboard table renders whatever rows come back. A row invented
		// for an empty result reads as real data — an "(empty)" entry with zero
		// sessions — and suppresses the widget's own empty state.
		result := waRunQuery(t, suite, workspace.ID, map[string]interface{}{
			"schema": "web_sessions", "measures": []string{"sessions"},
			"dimensions": []string{"channel"}, "timezone": "UTC",
			"filters": []map[string]interface{}{
				rangeFilter(currentStart, currentEnd),
				{"member": "channel", "operator": "equals", "values": []string{"no-such-channel"}},
			},
		})
		assert.Empty(t, result.Data)
	})

	t.Run("goal series buckets by day and zero-fills the quiet ones", func(t *testing.T) {
		// A goal card plots one goal over time: bucketed on the goal table's
		// own time dimension, filtered to a single goal name.
		days := []string{
			now.AddDate(0, 0, -2).Format("2006-01-02"),
			now.Format("2006-01-02"),
		}
		result := waRunQuery(t, suite, workspace.ID, map[string]interface{}{
			"schema":   "web_goals",
			"measures": []string{"goals", "sum_goal_value"},
			"timezone": "UTC",
			"timeDimensions": []map[string]interface{}{{
				"dimension": "goal_at", "granularity": "day", "dateRange": days,
			}},
			"filters": []map[string]interface{}{
				{"member": "goal_name", "operator": "equals", "values": []string{"purchase"}},
			},
		})

		require.Len(t, result.Data, 3, "one row per day in the range, quiet days included")

		var goals, value float64
		var zeroDays int
		for _, row := range result.Data {
			require.Contains(t, row, "goal_at_day", "the chart reads the bucket column by name")
			count := waNumber(t, row["goals"])
			goals += count
			value += waNumber(t, row["sum_goal_value"])
			if count == 0 {
				zeroDays++
			}
		}
		assert.Equal(t, float64(2), goals, "the signup goal is excluded by the goal_name filter")
		assert.InDelta(t, 150.0, value, 0.01)
		assert.Equal(t, 2, zeroDays, "a gap in the series must be a zero, not a missing point")
	})

	t.Run("a range filter over a quiet period answers with zeros, not an error", func(t *testing.T) {
		// Every KPI tile renders whatever comes back; an empty period has to
		// produce a readable zero rather than an empty response the card would
		// have to special-case.
		quietStart := instant(now.AddDate(0, 0, -30))
		quietEnd := instant(now.AddDate(0, 0, -20))
		result := waRunQuery(t, suite, workspace.ID, map[string]interface{}{
			"schema":   "web_sessions",
			"measures": []string{"sessions", "bounce_rate"},
			"timezone": "UTC",
			"filters":  []map[string]interface{}{rangeFilter(quietStart, quietEnd)},
		})

		require.Len(t, result.Data, 1)
		assert.Equal(t, float64(0), waNumber(t, result.Data[0]["sessions"]))
	})

	t.Run("the pruning predicate is present and does not cut the range short", func(t *testing.T) {
		// A month-long window crossing a partition boundary: the widened
		// session_date bounds must still cover the oldest row in range.
		wideStart := instant(now.AddDate(0, 0, -7).Truncate(24 * time.Hour))
		result := waRunQuery(t, suite, workspace.ID, map[string]interface{}{
			"schema":     "web_sessions",
			"measures":   []string{"sessions"},
			"dimensions": []string{"channel"},
			"timezone":   "UTC",
			"filters":    []map[string]interface{}{rangeFilter(wideStart, currentEnd)},
		})

		byChannel := sessionsByChannel(result)
		total := 0.0
		for _, count := range byChannel {
			total += count
		}
		assert.Equal(t, float64(5), total, fmt.Sprintf("all five sessions sit inside the window: %v", byChannel))
	})

	t.Run("explore summary: the best-performing combination of the whole report", func(t *testing.T) {
		// The "Best TimeScore" tile is the one query on the page that groups by
		// every dimension of the report at once, orders by a median rather than
		// a count, and asks for a single row.
		//
		// Keep this request body in step with the expectation in
		// console/src/components/web_analytics/lib/query.test.ts: that test
		// proves the console still builds this shape, this one proves
		// PostgreSQL executes it and picks the right winner. Neither is much
		// use without the other.
		//
		// Ten days back, in its own window: every other subtest here ranges
		// over the last week, so these rows are invisible to all of them.
		bestAt := now.AddDate(0, 0, -10).Truncate(24 * time.Hour).Add(9 * time.Hour)
		for _, table := range schema.WebAnalyticsTableNames {
			_, err := wsDB.Exec(schema.WebAnalyticsPartitionDDL(table, bestAt))
			require.NoError(t, err)
		}

		// One outstanding session on its own, and a merely good pair. The
		// single session wins on median and loses on count, which is exactly
		// what separates "best-performing" from "busiest".
		insertSession(0x31, bestAt, "referral", "hn", "", 90000, 1)
		insertSession(0x32, bestAt.Add(time.Hour), "paid-social", "meta", "", 40000, 1)
		insertSession(0x33, bestAt.Add(2*time.Hour), "paid-social", "meta", "", 60000, 1)
		insertSession(0x34, bestAt.Add(3*time.Hour), "organic-search", "ddg", "", 5000, 1)

		bestStart := instant(now.AddDate(0, 0, -11).Truncate(24 * time.Hour))
		bestEnd := instant(now.AddDate(0, 0, -9).Truncate(24 * time.Hour).Add(24*time.Hour - time.Millisecond))

		bestQuery := func(having []map[string]interface{}) map[string]interface{} {
			query := map[string]interface{}{
				"schema":     "web_sessions",
				"measures":   []string{"sessions", "median_duration"},
				"dimensions": []string{"channel", "utm_source"},
				"timezone":   "UTC",
				"filters":    []map[string]interface{}{rangeFilter(bestStart, bestEnd)},
				"order":      map[string]string{"median_duration": "desc"},
				"limit":      1,
			}
			if having != nil {
				query["having"] = having
			}
			return query
		}

		// Ordering by an ordered-set aggregate is the part that could simply
		// fail to parse; that it returns the 90s row rather than the pair is
		// the part that could silently be sorted by the wrong column.
		top := waRunQuery(t, suite, workspace.ID, bestQuery(nil))

		require.Len(t, top.Data, 1, "the tile reads a single row")
		assert.Equal(t, "referral", top.Data[0]["channel"])
		assert.Equal(t, "hn", top.Data[0]["utm_source"])
		assert.Equal(t, 90.0, waNumber(t, top.Data[0]["median_duration"]))

		// Every grouped dimension comes back keyed by its own name. The
		// console's staleness guard depends entirely on this: a row missing a
		// key is how it recognises a result left over from a previous
		// dimension list, rather than a genuinely empty value.
		//
		// A computed dimension is the case worth pinning. `channel` is a plain
		// column that would answer to its own name with or without an alias,
		// but day_of_week is `(EXTRACT(ISODOW FROM ...))::int` and would come
		// back under whatever PostgreSQL chose to call it.
		isoDow := int(bestAt.Weekday())
		if isoDow == 0 {
			isoDow = 7 // Go counts Sunday as 0; ISODOW counts it as 7.
		}
		computed := waRunQuery(t, suite, workspace.ID, map[string]interface{}{
			"schema":     "web_sessions",
			"measures":   []string{"sessions", "median_duration"},
			"dimensions": []string{"channel", "day_of_week"},
			"timezone":   "UTC",
			"filters":    []map[string]interface{}{rangeFilter(bestStart, bestEnd)},
			"order":      map[string]string{"median_duration": "desc"},
			"limit":      1,
		})

		require.Len(t, computed.Data, 1)
		assert.Equal(t, "referral", computed.Data[0]["channel"])
		_, present := computed.Data[0]["day_of_week"]
		require.True(t, present, "the row carries a day_of_week column, not an engine-chosen name")
		assert.Equal(t, float64(isoDow), waNumber(t, computed.Data[0]["day_of_week"]))

		// With the threshold the winner changes hands: the 90s combination is
		// a single visitor and drops out. This is the rule the four totals
		// tiles beside it deliberately do not apply, and the only layer that
		// can prove it is this one, because it is enforced in SQL.
		filtered := waRunQuery(t, suite, workspace.ID, bestQuery([]map[string]interface{}{
			{"member": "sessions", "operator": "gte", "values": []string{"2"}},
		}))

		require.Len(t, filtered.Data, 1)
		assert.Equal(t, "paid-social", filtered.Data[0]["channel"])
		assert.Equal(t, "meta", filtered.Data[0]["utm_source"])
		assert.Equal(t, 50.0, waNumber(t, filtered.Data[0]["median_duration"]),
			"the median of a 40s and a 60s session")
	})
}

// TestWebAnalyticsLiveWindow covers the one query in the console that does not
// ask "what happened in this period" but "who is here now". It ranges over
// last activity rather than session start, which is a different column with a
// different relationship to the partition key.
func TestWebAnalyticsLiveWindow(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer func() { suite.Cleanup() }()

	workspace, err := suite.DataFactory.CreateWorkspace(func(w *domain.Workspace) {
		w.Settings.WebAnalytics = &domain.WebAnalyticsSettings{
			Enabled: true, BounceThresholdSeconds: 10, Filters: domain.DefaultWebFilters(),
		}
	})
	require.NoError(t, err)

	workspaceRepo := suite.ServerManager.GetApp().GetWorkspaceRepository()
	require.NoError(t, workspaceRepo.AddUserToWorkspace(context.Background(), &domain.UserWorkspace{
		UserID:      "550e8400-e29b-41d4-a716-446655440000", // testuser@example.com
		WorkspaceID: workspace.ID,
		Role:        "owner",
		Permissions: domain.FullPermissions,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}))

	wsDB, err := suite.DBManager.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)

	now := time.Now().UTC()
	for _, month := range []time.Time{now, now.AddDate(0, -1, 0)} {
		for _, table := range schema.WebAnalyticsTableNames {
			_, err := wsDB.Exec(schema.WebAnalyticsPartitionDDL(table, month))
			require.NoError(t, err)
		}
	}

	insert := func(salt byte, createdAt, updatedAt time.Time, channel string) {
		_, err := wsDB.Exec(`
			INSERT INTO web_sessions (session_date, id, beat_seq, created_at, updated_at,
				duration_ms, pageview_count, channel)
			VALUES ($1, $2, 1, $3, $4, 30000, 1, $5)`,
			createdAt.Format("2006-01-02"), waUUIDv7At(createdAt, salt), createdAt, updatedAt, channel)
		require.NoError(t, err)
	}

	// A visitor who landed three hours ago and is still reading, one who
	// arrived and left three hours ago, and one who just landed.
	insert(0x51, now.Add(-3*time.Hour), now.Add(-2*time.Minute), "still-reading")
	insert(0x52, now.Add(-3*time.Hour), now.Add(-3*time.Hour), "long-gone")
	insert(0x53, now.Add(-5*time.Minute), now.Add(-1*time.Minute), "just-arrived")

	require.NoError(t, suite.APIClient.Login("testuser@example.com", ""))

	instant := func(ts time.Time) string { return ts.Format("2006-01-02T15:04:05.000Z07:00") }
	window := []string{instant(now.Add(-30 * time.Minute)), instant(now.Add(5 * time.Minute))}

	channels := func(member string) map[string]float64 {
		result := waRunQuery(t, suite, workspace.ID, map[string]interface{}{
			"schema": "web_sessions", "measures": []string{"sessions"},
			"dimensions": []string{"channel"}, "timezone": "UTC",
			"filters": []map[string]interface{}{
				{"member": member, "operator": "inDateRange", "values": window},
			},
		})
		byChannel := map[string]float64{}
		for _, row := range result.Data {
			channel, _ := row["channel"].(string)
			byChannel[channel] += waNumber(t, row["sessions"])
		}
		return byChannel
	}

	t.Run("last activity counts a session that started before the window", func(t *testing.T) {
		// Ranging over created_at instead would answer "who arrived recently",
		// which drops the visitor who has been reading for an hour — the one
		// the view exists to show. It would also keep the session that started
		// inside the window but has since left, if the window were long enough.
		assert.Equal(t, map[string]float64{"still-reading": 1, "just-arrived": 1}, channels("updated_at"))
	})

	t.Run("session start answers a different question", func(t *testing.T) {
		assert.Equal(t, map[string]float64{"just-arrived": 1}, channels("created_at"))
	})

	t.Run("a live session outlives the partition-pruning slack it is given", func(t *testing.T) {
		// The pruning predicate widens the requested range by the session-id
		// acceptance window before constraining session_date. A session whose
		// start is older than that widening — kept alive for days by continuous
		// activity — is pruned out of a live window that should show it. Well
		// beyond any realistic browsing session, but the boundary is real.
		old := now.AddDate(0, 0, -5)
		for _, table := range schema.WebAnalyticsTableNames {
			_, err := wsDB.Exec(schema.WebAnalyticsPartitionDDL(table, old))
			require.NoError(t, err)
		}
		insert(0x54, old, now.Add(-1*time.Minute), "week-long-tab")

		assert.NotContains(t, channels("updated_at"), "week-long-tab",
			"documents the known limit; drop the pruning slack assumption to include it")
	})
}
