//go:build integration
// +build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/database/schema"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/tests/testutil"
)

// TestWebAnalyticsDimensionSemantics pins what a dimension *means* once it
// reaches SQL, for the cases where the console's intent and a naive expression
// diverge: a clock that has to be the reader's, a nullable column that has to
// stay selectable, and numeric dimensions that arrive as strings because URL
// state has no types.
func TestWebAnalyticsDimensionSemantics(t *testing.T) {
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

	now := time.Now().UTC()
	for _, month := range []time.Time{now, now.AddDate(0, 0, -3)} {
		for _, table := range schema.WebAnalyticsTableNames {
			_, err := wsDB.Exec(schema.WebAnalyticsPartitionDDL(table, month))
			require.NoError(t, err)
		}
	}

	insert := func(salt byte, at time.Time, userID interface{}, durationMs int) {
		_, err := wsDB.Exec(`
			INSERT INTO web_sessions (session_date, id, beat_seq, created_at, updated_at,
				duration_ms, pageview_count, channel, contact_email)
			VALUES ($1, $2, 1, $3, $3, $4, 1, 'direct', $5)`,
			at.Format("2006-01-02"), waUUIDv7At(at, salt), at, durationMs, userID)
		require.NoError(t, err)
	}

	// One session late on a UTC day and one early on the next: in a timezone
	// two hours ahead they belong to the same local day and to consecutive
	// local hours, which is the whole point of the assertions below.
	day := now.Truncate(24 * time.Hour).AddDate(0, 0, -1)
	lateUTC := day.Add(23*time.Hour + 30*time.Minute)
	earlyUTC := day.Add(25*time.Hour + 30*time.Minute)
	insert(0x41, lateUTC, "user_a", 30000)
	insert(0x42, earlyUTC, nil, 10000)

	require.NoError(t, suite.APIClient.Login("testuser@example.com", ""))

	sessionsBy := func(result analyticsQueryResponse, dimension string) map[float64]float64 {
		byValue := map[float64]float64{}
		for _, row := range result.Data {
			byValue[waNumber(t, row[dimension])] += waNumber(t, row["sessions"])
		}
		return byValue
	}

	t.Run("hour of day is the reader's clock, not the server's", func(t *testing.T) {
		// A "traffic by hour" report answers a question about the visitor's
		// local morning. Extracted in UTC it would put a 9am peak in Los
		// Angeles at 4pm, and the heat map's own click-to-filter would then
		// disagree with the chart above it.
		utc := waRunQuery(t, suite, workspace.ID, map[string]interface{}{
			"schema": "web_sessions", "measures": []string{"sessions"},
			"dimensions": []string{"hour_of_day"}, "timezone": "UTC",
		})
		assert.Equal(t, map[float64]float64{23: 1, 1: 1}, sessionsBy(utc, "hour_of_day"))

		// Europe/Paris is UTC+2 in summer and UTC+1 in winter, so the expected
		// hours are derived rather than hardcoded.
		paris, err := time.LoadLocation("Europe/Paris")
		require.NoError(t, err)
		local := waRunQuery(t, suite, workspace.ID, map[string]interface{}{
			"schema": "web_sessions", "measures": []string{"sessions"},
			"dimensions": []string{"hour_of_day"}, "timezone": "Europe/Paris",
		})
		assert.Equal(t, map[float64]float64{
			float64(lateUTC.In(paris).Hour()):  1,
			float64(earlyUTC.In(paris).Hour()): 1,
		}, sessionsBy(local, "hour_of_day"))
	})

	t.Run("day of week follows the same clock", func(t *testing.T) {
		paris, err := time.LoadLocation("Europe/Paris")
		require.NoError(t, err)
		result := waRunQuery(t, suite, workspace.ID, map[string]interface{}{
			"schema": "web_sessions", "measures": []string{"sessions"},
			"dimensions": []string{"day_of_week"}, "timezone": "Europe/Paris",
		})

		// Both sessions land on the same local day even though they straddle
		// UTC midnight.
		isoDay := func(ts time.Time) float64 {
			weekday := int(ts.In(paris).Weekday())
			if weekday == 0 {
				return 7 // Sunday, in PostgreSQL's ISO numbering
			}
			return float64(weekday)
		}
		require.Equal(t, isoDay(lateUTC), isoDay(earlyUTC), "fixture must straddle UTC midnight only")
		assert.Equal(t, map[float64]float64{isoDay(lateUTC): 2}, sessionsBy(result, "day_of_week"))
	})

	t.Run("a heat-map cell click filters on numeric dimensions", func(t *testing.T) {
		// URL state is untyped, so every filter value reaches the engine as a
		// string even when the dimension is an integer expression.
		result := waRunQuery(t, suite, workspace.ID, map[string]interface{}{
			"schema": "web_sessions", "measures": []string{"sessions"},
			"timezone": "UTC",
			"filters": []map[string]interface{}{
				{"member": "hour_of_day", "operator": "equals", "values": []string{"23"}},
			},
		})
		require.Len(t, result.Data, 1)
		assert.Equal(t, float64(1), waNumber(t, result.Data[0]["sessions"]))
	})

	t.Run("the anonymous visitors bucket stays selectable", func(t *testing.T) {
		// contact_email is the one exposed dimension that is nullable. Grouped raw it
		// produces a NULL bucket that no filter the console can build would
		// ever match again, so drilling into it would look like "no data".
		breakdown := waRunQuery(t, suite, workspace.ID, map[string]interface{}{
			"schema": "web_sessions", "measures": []string{"sessions"},
			"dimensions": []string{"contact_email"},
		})
		byUser := map[string]float64{}
		for _, row := range breakdown.Data {
			value, _ := row["contact_email"].(string)
			byUser[value] += waNumber(t, row["sessions"])
		}
		assert.Equal(t, map[string]float64{"user_a": 1, "": 1}, byUser)

		empty := waRunQuery(t, suite, workspace.ID, map[string]interface{}{
			"schema": "web_sessions", "measures": []string{"sessions"},
			"filters": []map[string]interface{}{
				{"member": "contact_email", "operator": "equals", "values": []string{""}},
			},
		})
		require.Len(t, empty.Data, 1)
		assert.Equal(t, float64(1), waNumber(t, empty.Data[0]["sessions"]),
			"is-empty must select the very rows the breakdown showed as empty")

		identified := waRunQuery(t, suite, workspace.ID, map[string]interface{}{
			"schema": "web_sessions", "measures": []string{"sessions", "contacts"},
			"filters": []map[string]interface{}{
				{"member": "contact_email", "operator": "notEquals", "values": []string{""}},
			},
		})
		require.Len(t, identified.Data, 1)
		assert.Equal(t, float64(1), waNumber(t, identified.Data[0]["sessions"]))
		assert.Equal(t, float64(1), waNumber(t, identified.Data[0]["contacts"]),
			"the distinct-user count still ignores the anonymous rows")
	})

	t.Run("a threshold on a percentile measure is a valid HAVING", func(t *testing.T) {
		// Explore lets an operator ask for "TimeScore above N", which lands on
		// an ordered-set aggregate rather than a plain one.
		result := waRunQuery(t, suite, workspace.ID, map[string]interface{}{
			"schema": "web_sessions", "measures": []string{"sessions", "median_duration"},
			"dimensions": []string{"channel"},
			"having": []map[string]interface{}{
				{"member": "median_duration", "operator": "gt", "values": []string{"15"}},
			},
		})
		require.Len(t, result.Data, 1, "the median of 10s and 30s is 20s, above the threshold")
		assert.InDelta(t, 20.0, waNumber(t, result.Data[0]["median_duration"]), 0.01)

		excluded := waRunQuery(t, suite, workspace.ID, map[string]interface{}{
			"schema": "web_sessions", "measures": []string{"sessions", "median_duration"},
			"dimensions": []string{"channel"},
			"having": []map[string]interface{}{
				{"member": "median_duration", "operator": "gt", "values": []string{"25"}},
			},
		})
		assert.Empty(t, excluded.Data)
	})

	t.Run("measures arrive as strings when PostgreSQL returns numerics", func(t *testing.T) {
		// Not a defect, but everything reading a measure has to go through the
		// same parser or a median silently becomes NaN.
		result := waRunQuery(t, suite, workspace.ID, map[string]interface{}{
			"schema": "web_sessions", "measures": []string{"median_duration", "bounce_rate"},
		})
		require.Len(t, result.Data, 1)
		assert.IsType(t, "", result.Data[0]["median_duration"])
	})
}
