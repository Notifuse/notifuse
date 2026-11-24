//go:build integration
// +build integration

package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/database/schema"
	"github.com/Notifuse/notifuse/internal/domain"
	pgdb "github.com/Notifuse/notifuse/pkg/database"
	"github.com/Notifuse/notifuse/tests/testutil"
)

// TestWebAnalyticsTimeSemantics runs, through /api/analytics.query, the queries whose answer
// depends on how the engine reads time and empty values: cyclic dimensions on a database whose
// session is not on UTC, contact_email set and notSet, date filters in the query's timezone,
// empty periods of a rate, the last microsecond of a day, the two daylight-saving days and the
// total of many goal values.
func TestWebAnalyticsTimeSemantics(t *testing.T) {
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

	for _, month := range []time.Time{
		time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC),
	} {
		for _, table := range schema.WebAnalyticsTableNames {
			_, err := wsDB.Exec(schema.WebAnalyticsPartitionDDL(table, month))
			require.NoError(t, err)
		}
	}

	salt := byte(0x20)
	// insert stores a session the way ingest does (session_date is the UTC date of its start),
	// human (two beats, 30 s) unless duration says otherwise.
	insert := func(at time.Time, channel string, contactEmail interface{}, durationMs int) {
		t.Helper()
		utc := at.UTC()
		_, err := wsDB.Exec(`
			INSERT INTO web_sessions (session_date, id, beat_seq, created_at, updated_at,
				duration_ms, pageview_count, channel, contact_email)
			VALUES ($1, $2, 2, $3, $3, $4, 1, $5, $6)`,
			utc.Format("2006-01-02"), waUUIDv7At(utc, salt), utc, durationMs, channel, contactEmail)
		require.NoError(t, err)
		salt++
	}
	paris, err := time.LoadLocation("Europe/Paris")
	require.NoError(t, err)

	require.NoError(t, suite.APIClient.Login("testuser@example.com", ""))

	query := func(q map[string]interface{}) []map[string]interface{} {
		t.Helper()
		return waRunQuery(t, suite, workspace.ID, q).Data
	}
	channelIs := func(channel string) map[string]interface{} {
		return map[string]interface{}{"member": "channel", "operator": "equals", "values": []string{channel}}
	}
	total := func(rows []map[string]interface{}, measure string) float64 {
		sum := 0.0
		for _, row := range rows {
			sum += waNumber(t, row[measure])
		}
		return sum
	}

	t.Run("set and notSet on contact_email mean identified and anonymous", func(t *testing.T) {
		day := time.Date(2026, 8, 4, 10, 0, 0, 0, time.UTC)
		insert(day, "identity", nil, 30000)
		insert(day.Add(time.Minute), "identity", nil, 30000)
		insert(day.Add(2*time.Minute), "identity", "ada@example.com", 30000)

		count := func(operator string, values ...string) float64 {
			filter := map[string]interface{}{"member": "contact_email", "operator": operator}
			if values != nil {
				filter["values"] = values
			}
			return total(query(map[string]interface{}{
				"schema": "web_sessions", "measures": []string{"sessions"},
				"filters": []map[string]interface{}{channelIs("identity"), filter},
			}), "sessions")
		}
		assert.Equal(t, float64(1), count("set"), "the identified session")
		assert.Equal(t, float64(2), count("notSet"), "the anonymous sessions")
		assert.Equal(t, float64(1), count("equals", "ada@example.com"))
		// The console's "is empty" and "is not empty".
		assert.Equal(t, float64(2), count("equals", ""))
		assert.Equal(t, float64(1), count("notEquals", ""))
		// Grouped, the anonymous sessions still come back under the empty string.
		rows := query(map[string]interface{}{
			"schema": "web_sessions", "measures": []string{"sessions"}, "dimensions": []string{"contact_email"},
			"filters": []map[string]interface{}{channelIs("identity")},
		})
		byEmail := map[string]float64{}
		for _, row := range rows {
			byEmail[fmt.Sprint(row["contact_email"])] = waNumber(t, row["sessions"])
		}
		assert.Equal(t, map[string]float64{"": 2, "ada@example.com": 1}, byEmail)
	})

	t.Run("date filters read bare dates in the timezone and keep the whole last day", func(t *testing.T) {
		// In Paris (UTC+2 in summer): the first minutes of Aug 1, the last half hour of Aug 8,
		// and the first half hour of Aug 9.
		insert(time.Date(2026, 8, 1, 0, 30, 0, 0, paris), "datefilter", nil, 30000)
		insert(time.Date(2026, 8, 8, 23, 30, 0, 0, paris), "datefilter", nil, 30000)
		insert(time.Date(2026, 8, 9, 0, 30, 0, 0, paris), "datefilter", nil, 30000)

		count := func(operator string, values ...string) float64 {
			return total(query(map[string]interface{}{
				"schema": "web_sessions", "measures": []string{"sessions"}, "timezone": "Europe/Paris",
				"filters": []map[string]interface{}{
					channelIs("datefilter"),
					{"member": "created_at", "operator": operator, "values": values},
				},
			}), "sessions")
		}
		assert.Equal(t, float64(2), count("inDateRange", "2026-08-01", "2026-08-08"), "Aug 1 to the end of Aug 8, Paris time")
		assert.Equal(t, float64(1), count("inDateRange", "2026-08-08", "2026-08-08"), "a single day")
		assert.Equal(t, float64(1), count("notInDateRange", "2026-08-01", "2026-08-08"))
		assert.Equal(t, float64(1), count("afterDate", "2026-08-08"), "after the whole of Aug 8")
		assert.Equal(t, float64(2), count("beforeDate", "2026-08-09"))
	})

	t.Run("an empty period has no rate and no median, and zero sessions", func(t *testing.T) {
		insert(time.Date(2026, 8, 12, 9, 0, 0, 0, time.UTC), "gaps", nil, 30000)
		insert(time.Date(2026, 8, 12, 9, 5, 0, 0, time.UTC), "gaps", nil, 3000)

		rows := query(map[string]interface{}{
			"schema":   "web_sessions",
			"measures": []string{"sessions", "pageviews", "bounce_rate", "median_duration"},
			"filters":  []map[string]interface{}{channelIs("gaps")},
			"timeDimensions": []map[string]interface{}{{
				"dimension": "created_at", "granularity": "day", "dateRange": []string{"2026-08-12", "2026-08-13"},
			}},
		})
		require.Len(t, rows, 2)
		assert.Equal(t, "2026-08-12T00:00:00Z", rows[0]["created_at_day"])
		assert.Equal(t, float64(50), waNumber(t, rows[0]["bounce_rate"]))
		assert.Equal(t, "2026-08-13T00:00:00Z", rows[1]["created_at_day"])
		assert.Equal(t, float64(0), rows[1]["sessions"])
		assert.Equal(t, float64(0), rows[1]["pageviews"])
		assert.Contains(t, rows[1], "bounce_rate")
		assert.Nil(t, rows[1]["bounce_rate"], "no session, no bounce rate: 0 would read as nobody bouncing")
		assert.Nil(t, rows[1]["median_duration"])
	})

	t.Run("a bare end date keeps the last microsecond of its day", func(t *testing.T) {
		insert(time.Date(2026, 8, 14, 23, 59, 59, 999500000, time.UTC), "lastmicro", nil, 30000)

		rows := query(map[string]interface{}{
			"schema": "web_sessions", "measures": []string{"sessions"},
			"filters": []map[string]interface{}{channelIs("lastmicro")},
			"timeDimensions": []map[string]interface{}{{
				"dimension": "created_at", "granularity": "day", "dateRange": []string{"2026-08-14", "2026-08-14"},
			}},
		})
		assert.Equal(t, float64(1), total(rows, "sessions"))
	})

	hourly := func(channel, day string) []map[string]interface{} {
		return query(map[string]interface{}{
			"schema": "web_sessions", "measures": []string{"sessions"}, "timezone": "Europe/Paris",
			"filters": []map[string]interface{}{channelIs(channel)},
			"timeDimensions": []map[string]interface{}{{
				"dimension": "created_at", "granularity": "hour", "dateRange": []string{day, day},
			}},
		})
	}

	t.Run("the day the clocks go forward has no 02:00 hour", func(t *testing.T) {
		// 01:30 CET and 03:30 CEST: 02:00 to 03:00 does not exist in Paris that night.
		insert(time.Date(2026, 3, 29, 0, 30, 0, 0, time.UTC), "spring", nil, 30000)
		insert(time.Date(2026, 3, 29, 1, 30, 0, 0, time.UTC), "spring", nil, 30000)

		byHour := map[string]float64{}
		for _, row := range hourly("spring", "2026-03-29") {
			byHour[fmt.Sprint(row["created_at_hour"])] = waNumber(t, row["sessions"])
		}
		assert.Equal(t, map[string]float64{
			"2026-03-29T01:00:00Z": 1,
			"2026-03-29T03:00:00Z": 1,
		}, byHour)
	})

	t.Run("an hourly day with no data has each of its hours", func(t *testing.T) {
		rows := hourly("nothing", "2026-08-08")
		require.Len(t, rows, 24)
		assert.Equal(t, "2026-08-08T00:00:00Z", rows[0]["created_at_hour"])
		assert.Equal(t, "2026-08-08T23:00:00Z", rows[23]["created_at_hour"])
		assert.Equal(t, float64(0), rows[23]["sessions"])
	})

	t.Run("the day the clocks go back has one 02:00 hour holding both", func(t *testing.T) {
		// 02:30 CEST, then 02:30 CET an hour later: both in the wall-clock hour 02:00.
		insert(time.Date(2026, 10, 25, 0, 30, 0, 0, time.UTC), "autumn", nil, 30000)
		insert(time.Date(2026, 10, 25, 1, 30, 0, 0, time.UTC), "autumn", nil, 30000)

		rows := hourly("autumn", "2026-10-25")
		require.Len(t, rows, 1)
		assert.Equal(t, "2026-10-25T02:00:00Z", rows[0]["created_at_hour"])
		assert.Equal(t, float64(2), waNumber(t, rows[0]["sessions"]))
	})

	t.Run("goal values add up exactly", func(t *testing.T) {
		// 100,000 goals of 19.99: summed as REAL, the total comes to 1,999,836.6.
		_, err := wsDB.Exec(`
			INSERT INTO web_goals (session_date, session_id, goal_name, client_ts_ms, goal_at, goal_value, channel)
			SELECT DATE '2026-08-20', '0198c000-0000-7000-8000-000000000001'::uuid, 'drift', n,
				TIMESTAMPTZ '2026-08-20 10:00:00+00', 19.99, 'drift'
			FROM generate_series(1, 100000) AS n`)
		require.NoError(t, err)
		_, err = wsDB.Exec(`
			INSERT INTO web_sessions (session_date, id, beat_seq, created_at, updated_at, goal_count, goal_value, channel)
			SELECT DATE '2026-08-20', gen_random_uuid(), 2, TIMESTAMPTZ '2026-08-20 10:00:00+00',
				TIMESTAMPTZ '2026-08-20 10:00:00+00', 1, 19.99, 'drift'
			FROM generate_series(1, 100000)`)
		require.NoError(t, err)

		goals := query(map[string]interface{}{
			"schema": "web_goals", "measures": []string{"sum_goal_value"},
			"filters": []map[string]interface{}{channelIs("drift")},
		})
		require.Len(t, goals, 1)
		assert.Equal(t, 1999000.0, goals[0]["sum_goal_value"], "a JSON number, exact to the cent")

		sessions := query(map[string]interface{}{
			"schema": "web_sessions", "measures": []string{"goal_value"},
			"filters": []map[string]interface{}{channelIs("drift")},
		})
		require.Len(t, sessions, 1)
		assert.Equal(t, 1999000.0, sessions[0]["goal_value"])
	})

	t.Run("cyclic dimensions are UTC ones on a database whose session is not on UTC", func(t *testing.T) {
		// Sunday Aug 2, 20:00 UTC is Monday 05:00 in Tokyo.
		insert(time.Date(2026, 8, 2, 20, 0, 0, 0, time.UTC), "clock", nil, 30000)

		// The workspace database's sessions start in Tokyo time; the app's pool is closed so
		// that its next connection takes it.
		var database string
		require.NoError(t, wsDB.QueryRow(`SELECT current_database()`).Scan(&database))
		setZone := func(clause string) {
			_, err := wsDB.Exec(`ALTER DATABASE ` + pq.QuoteIdentifier(database) + ` ` + clause)
			require.NoError(t, err)
			manager, err := pgdb.GetConnectionManager()
			require.NoError(t, err)
			require.NoError(t, manager.CloseWorkspaceConnection(workspace.ID))
		}
		setZone(`SET timezone = 'Asia/Tokyo'`)
		defer setZone(`RESET timezone`)

		for _, timezone := range []interface{}{"UTC", nil} {
			q := map[string]interface{}{
				"schema": "web_sessions", "measures": []string{"sessions"},
				"dimensions": []string{"hour_of_day", "day_of_week", "is_weekend", "day"},
				"filters":    []map[string]interface{}{channelIs("clock")},
			}
			if timezone != nil {
				q["timezone"] = timezone
			}
			rows := query(q)
			require.Len(t, rows, 1, "timezone %v", timezone)
			assert.Equal(t, float64(20), waNumber(t, rows[0]["hour_of_day"]), "timezone %v", timezone)
			assert.Equal(t, float64(7), waNumber(t, rows[0]["day_of_week"]), "Sunday; timezone %v", timezone)
			assert.Equal(t, "true", rows[0]["is_weekend"], "timezone %v", timezone)
			assert.Equal(t, float64(2), waNumber(t, rows[0]["day"]), "timezone %v", timezone)
		}
	})
}
