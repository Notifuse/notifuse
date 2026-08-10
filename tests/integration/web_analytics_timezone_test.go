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
	"github.com/Notifuse/notifuse/pkg/analytics"
	"github.com/Notifuse/notifuse/tests/testutil"
)

// TestWebAnalyticsTimezoneBoundaries covers the two places a timezone can quietly
// corrupt a report: which bucket a session falls into, and which partition the
// planner is allowed to read.
//
// The partition key is the UTC date derived from the session id, but a report is
// read in the workspace's timezone. Far enough east, a local day begins in the
// previous UTC day — and on the first of the month, in the previous month's
// partition. If the pruning predicate is derived from local wall clock rather
// than the instant it resolves to, those rows vanish from the report while
// sitting perfectly well in the table.
func TestWebAnalyticsTimezoneBoundaries(t *testing.T) {
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

	// Auckland is UTC+12 in August, so a local day starts at 12:00 UTC the day
	// before. Sessions are anchored relative to now so they stay inside the
	// partition pruning window whenever the suite runs.
	auckland, err := time.LoadLocation("Pacific/Auckland")
	require.NoError(t, err)

	now := time.Now().UTC()
	// The first local midnight of the current local month, and the UTC instant
	// it corresponds to — which is in the previous UTC month whenever the
	// offset is positive.
	localNow := now.In(auckland)
	localMonthStart := time.Date(localNow.Year(), localNow.Month(), 1, 0, 0, 0, 0, auckland)
	if localMonthStart.After(localNow.Add(-24 * time.Hour)) {
		// Too close to the boundary to have a full local day; step back a month.
		localMonthStart = localMonthStart.AddDate(0, -1, 0)
	}

	beforeUTCMidnight := localMonthStart.Add(30 * time.Minute) // local 00:30
	afterUTCMidnight := localMonthStart.Add(18 * time.Hour)    // local 18:00
	previousLocalDay := localMonthStart.Add(-2 * time.Hour)    // last day of the previous local month

	for _, month := range []time.Time{
		now, now.AddDate(0, -1, 0), now.AddDate(0, -2, 0),
	} {
		for _, table := range schema.WebAnalyticsTableNames {
			_, err := wsDB.Exec(schema.WebAnalyticsPartitionDDL(table, month))
			require.NoError(t, err)
		}
	}

	insert := func(salt byte, at time.Time, channel string) {
		utc := at.UTC()
		_, err := wsDB.Exec(`
			INSERT INTO web_sessions (session_date, id, beat_seq, created_at, updated_at,
				duration_ms, pageview_count, channel)
			VALUES ($1, $2, 1, $3, $3, 30000, 1, $4)`,
			// Exactly how ingest derives it: the UTC date of the session start.
			utc.Format("2006-01-02"), waUUIDv7At(utc, salt), utc, channel)
		require.NoError(t, err)
	}

	insert(0x61, beforeUTCMidnight, "early-local")
	insert(0x62, afterUTCMidnight, "late-local")
	insert(0x63, previousLocalDay, "previous-local-day")

	require.NoError(t, suite.APIClient.Login("testuser@example.com", ""))

	localDay := localMonthStart.Format("2006-01-02")

	t.Run("a local day gathers the sessions that fall in it, not in the UTC day", func(t *testing.T) {
		result := waRunQuery(t, suite, workspace.ID, map[string]interface{}{
			"schema": "web_sessions", "measures": []string{"sessions"},
			"timezone": "Pacific/Auckland",
			"timeDimensions": []map[string]interface{}{{
				"dimension": "created_at", "granularity": "day",
				"dateRange": []string{localDay, localDay},
			}},
		})

		total := 0.0
		for _, row := range result.Data {
			total += waNumber(t, row["sessions"])
		}
		assert.Equal(t, float64(2), total,
			"both sessions of the local day, including the one whose UTC date is the day before")
	})

	t.Run("the row living in the previous month's partition is still read", func(t *testing.T) {
		// beforeUTCMidnight has a session_date in the previous UTC month
		// whenever the local month starts on the 1st. The pruning predicate has
		// to widen far enough to let the planner into that partition.
		var storedMonth string
		require.NoError(t, wsDB.QueryRow(
			`SELECT to_char(session_date, 'YYYY-MM') FROM web_sessions WHERE channel = 'early-local'`,
		).Scan(&storedMonth))

		// A breakdown carries its range on a plain filter, which is the path
		// whose pruning predicate has to reach into the older partition. It
		// also cannot be expressed as a time dimension: the gap filler keys
		// rows by bucket alone and would collapse the channels into one row.
		result := waRunQuery(t, suite, workspace.ID, map[string]interface{}{
			"schema": "web_sessions", "measures": []string{"sessions"},
			"dimensions": []string{"channel"},
			"timezone":   "Pacific/Auckland",
			"filters": []map[string]interface{}{{
				"member": "created_at", "operator": "inDateRange",
				"values": []string{
					localMonthStart.UTC().Format(time.RFC3339),
					localMonthStart.AddDate(0, 0, 1).Add(-time.Millisecond).UTC().Format(time.RFC3339),
				},
			}},
		})

		channels := map[string]float64{}
		for _, row := range result.Data {
			channel, _ := row["channel"].(string)
			channels[channel] += waNumber(t, row["sessions"])
		}
		assert.Equal(t, float64(1), channels["early-local"],
			"stored in partition %s, queried as a local day of the next month", storedMonth)
		assert.Equal(t, float64(1), channels["late-local"])
		assert.Zero(t, channels["previous-local-day"], "belongs to the previous local day")
	})

	t.Run("the same instants bucket differently in a western timezone", func(t *testing.T) {
		// Los Angeles is UTC-7/8, so the same instants fall on the previous
		// local day. This is the mirror of the case above and catches a fix
		// that merely hard-codes a positive offset.
		losAngeles, err := time.LoadLocation("America/Los_Angeles")
		require.NoError(t, err)
		westernLocal := beforeUTCMidnight.In(losAngeles)
		westernStart := time.Date(westernLocal.Year(), westernLocal.Month(), westernLocal.Day(),
			0, 0, 0, 0, losAngeles)

		result := waRunQuery(t, suite, workspace.ID, map[string]interface{}{
			"schema": "web_sessions", "measures": []string{"sessions"},
			"dimensions": []string{"channel"},
			"timezone":   "America/Los_Angeles",
			"filters": []map[string]interface{}{{
				"member": "created_at", "operator": "inDateRange",
				"values": []string{
					westernStart.UTC().Format(time.RFC3339),
					westernStart.AddDate(0, 0, 1).Add(-time.Millisecond).UTC().Format(time.RFC3339),
				},
			}},
		})

		channels := map[string]float64{}
		for _, row := range result.Data {
			channel, _ := row["channel"].(string)
			channels[channel] += waNumber(t, row["sessions"])
		}
		assert.Equal(t, float64(1), channels["early-local"])
	})

	t.Run("buckets are labelled as local midnights", func(t *testing.T) {
		// The console renders the bucket as a wall clock. A bucket carrying the
		// server's offset instead of the query's would shift every label.
		result := waRunQuery(t, suite, workspace.ID, map[string]interface{}{
			"schema": "web_sessions", "measures": []string{"sessions"},
			"timezone": "Pacific/Auckland",
			"timeDimensions": []map[string]interface{}{{
				"dimension": "created_at", "granularity": "day",
				"dateRange": []string{localDay, localDay},
			}},
		})
		require.NotEmpty(t, result.Data)
		assert.Equal(t, localDay+"T00:00:00Z", result.Data[0]["created_at_day"],
			"the local day's midnight, serialized without an offset")
	})
}

// TestWebAnalyticsNonUTCDatabaseSession runs the console's own chart query
// against a database session that is not on UTC.
//
// This is the condition both flat-line defects needed and no test had: the
// suite's Postgres runs on UTC, where the server's days and UTC days coincide,
// so a bucket truncated in the server's zone looked identical to one truncated
// in the query's. On any host configured otherwise — which is most self-hosted
// installs — every populated bucket missed the key the gap filler generates and
// the entire series came back as zeros.
func TestWebAnalyticsNonUTCDatabaseSession(t *testing.T) {
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

	wsDB, err := suite.DBManager.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)

	now := time.Now().UTC()
	for _, month := range []time.Time{now, now.AddDate(0, -1, 0)} {
		for _, table := range schema.WebAnalyticsTableNames {
			_, err := wsDB.Exec(schema.WebAnalyticsPartitionDDL(table, month))
			require.NoError(t, err)
		}
	}

	// Three days, three different counts, all mid-morning UTC so the expected
	// bucket is unambiguous whatever the server's offset.
	firstDay := now.Truncate(24*time.Hour).AddDate(0, 0, -3)
	perDay := map[int]int{0: 3, 1: 2, 2: 1}
	salt := byte(0x70)
	for offset, count := range perDay {
		for i := 0; i < count; i++ {
			at := firstDay.AddDate(0, 0, offset).Add(9*time.Hour + time.Duration(i)*time.Minute)
			_, err := wsDB.Exec(`
				INSERT INTO web_sessions (session_date, id, beat_seq, created_at, updated_at,
					duration_ms, pageview_count, channel)
				VALUES ($1, $2, 1, $3, $3, 30000, 1, 'direct')`,
				at.Format("2006-01-02"), waUUIDv7At(at, salt), at)
			require.NoError(t, err)
			salt++
		}
	}

	// One connection, so the session setting below is the one every query uses.
	wsDB.SetMaxOpenConns(1)
	_, err = wsDB.Exec("SET TIME ZONE 'Europe/Paris'")
	require.NoError(t, err)

	var sessionTimezone string
	require.NoError(t, wsDB.QueryRow("SHOW timezone").Scan(&sessionTimezone))
	require.Equal(t, "Europe/Paris", sessionTimezone,
		"the whole point of this test is a session that is not on UTC")

	utc := "UTC"
	query := analytics.Query{
		Schema:   "web_sessions",
		Measures: []string{"sessions"},
		Timezone: &utc,
		TimeDimensions: []analytics.TimeDimension{{
			Dimension:   "created_at",
			Granularity: "day",
			DateRange: &[2]string{
				firstDay.Format("2006-01-02"),
				firstDay.AddDate(0, 0, 2).Format("2006-01-02"),
			},
		}},
	}

	schemas := domain.WebAnalyticsSchemas(&domain.WebAnalyticsSettings{BounceThresholdSeconds: 10}, utc)
	sqlStr, args, err := analytics.NewSQLBuilder().BuildSQL(query, schemas["web_sessions"])
	require.NoError(t, err)

	rows, err := wsDB.Query(sqlStr, args...)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()

	data, err := analytics.ProcessRows(rows, query)
	require.NoError(t, err)

	byBucket := map[string]float64{}
	for _, row := range data {
		byBucket[fmt.Sprintf("%v", row["created_at_day"])] += waNumber(t, row["sessions"])
	}

	for offset, expected := range perDay {
		bucket := firstDay.AddDate(0, 0, offset).Format("2006-01-02") + "T00:00:00Z"
		assert.Equal(t, float64(expected), byBucket[bucket],
			"bucket %s should survive a non-UTC database session; got %v", bucket, byBucket)
	}
}
