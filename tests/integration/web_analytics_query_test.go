//go:build integration
// +build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/database/schema"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/tests/testutil"
)

type analyticsQueryResponse struct {
	Data []map[string]interface{} `json:"data"`
}

func waRunQuery(t *testing.T, suite *testutil.IntegrationTestSuite, workspaceID string, query map[string]interface{}) analyticsQueryResponse {
	t.Helper()
	resp, err := suite.APIClient.Post("/api/analytics.query", map[string]interface{}{
		"workspace_id": workspaceID,
		"query":        query,
	})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var decoded analyticsQueryResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&decoded))
	return decoded
}

func waNumber(t *testing.T, v interface{}) float64 {
	t.Helper()
	switch n := v.(type) {
	case float64:
		return n
	// Through the HTTP API every number arrives as float64; a test that queries
	// the database directly sees the driver's own types.
	case int64:
		return float64(n)
	case int:
		return float64(n)
	case string:
		var f float64
		_, err := fmt.Sscanf(n, "%f", &f)
		require.NoError(t, err)
		return f
	case nil:
		return 0
	default:
		t.Fatalf("unexpected numeric type %T", v)
		return 0
	}
}

func TestWebAnalyticsQueryEndToEnd(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer func() { suite.Cleanup() }()

	workspace, err := suite.DataFactory.CreateWorkspace(func(w *domain.Workspace) {
		w.Settings.WebAnalytics = &domain.WebAnalyticsSettings{
			Enabled:                true,
			BounceThresholdSeconds: 20,
			Filters:                domain.DefaultWebFilters(),
		}
	})
	require.NoError(t, err)

	// The seeded test user must be a member to query analytics.
	workspaceRepo := suite.ServerManager.GetApp().GetWorkspaceRepository()
	require.NoError(t, workspaceRepo.AddUserToWorkspace(context.Background(), &domain.UserWorkspace{
		UserID:      "550e8400-e29b-41d4-a716-446655440000", // testuser@example.com
		WorkspaceID: workspace.ID,
		Role:        "owner",
		Permissions: domain.FullPermissions,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}))

	baseURL := suite.ServerManager.GetURL()
	buffer := suite.ServerManager.GetApp().GetWebAnalyticsBuffer()
	now := time.Now().UTC()

	// Five sessions: paid (gclid, 30s, one 100 goal), two direct (5s, 15s),
	// one organic with two pages (25s total), and one direct session that
	// started 44h ago. Durations are chosen so the 20s workspace threshold and
	// the 10s default produce different bounce rates.
	beats := []waBeat{
		{workspaceID: workspace.ID, sessionID: waUUIDv7At(now.Add(-10*time.Minute), 0x11), seq: 1, utmIDFrom: "gclid",
			actions: []map[string]interface{}{
				waPageview("/landing", 1, 30000, 50, now),
				{"type": "goal", "name": "purchase", "path": "/landing", "page_number": 1,
					"timestamp": now.UnixMilli(), "value": 100.0},
			}},
		// 5s and 15s straddle the workspace's 20s bounce threshold but sit on
		// the same side of the 10s default, so a hardcoded default cannot
		// reproduce the expected bounce rate.
		{workspaceID: workspace.ID, sessionID: waUUIDv7At(now.Add(-9*time.Minute), 0x12), seq: 1,
			actions: []map[string]interface{}{waPageview("/", 1, 5000, 10, now)}},
		{workspaceID: workspace.ID, sessionID: waUUIDv7At(now.Add(-8*time.Minute), 0x13), seq: 1,
			actions: []map[string]interface{}{waPageview("/", 1, 15000, 20, now)}},
		{workspaceID: workspace.ID, sessionID: waUUIDv7At(now.Add(-7*time.Minute), 0x14), seq: 1,
			actions: []map[string]interface{}{
				waPageview("/blog", 1, 10000, 40, now),
				waPageview("/pricing", 2, 15000, 90, now),
			}},
		// A session that STARTED ~44h ago and is still beating: its
		// session_date lags created_at by two days, which is exactly what the
		// partition-pruning slack exists to tolerate. Without the slack (or
		// with its sign flipped) this session silently vanishes from results.
		{workspaceID: workspace.ID, sessionID: waUUIDv7At(now.Add(-44*time.Hour), 0x15), seq: 1,
			actions: []map[string]interface{}{waPageview("/old", 1, 12000, 15, now)}},
	}
	for i, beat := range beats {
		headers := map[string]string{}
		if i == 1 || i == 2 || i == 4 {
			headers["Origin"] = "" // direct beats: no referrer via attributes below
		}
		body := beat.body(t, now)
		if i == 1 || i == 2 || i == 4 {
			// Strip the referrer to make these sessions direct.
			var m map[string]interface{}
			require.NoError(t, json.Unmarshal(body, &m))
			attrs := m["attributes"].(map[string]interface{})
			delete(attrs, "referrer")
			body, _ = json.Marshal(m)
		}
		resp := waPostBeat(t, baseURL, body, headers)
		_ = waCloseAndDecode(t, resp)
	}
	buffer.FlushAll(context.Background())

	require.NoError(t, suite.APIClient.Login("testuser@example.com", ""))

	dateRange := []string{now.AddDate(0, 0, -2).Format("2006-01-02"), now.AddDate(0, 0, 2).Format("2006-01-02")}

	t.Run("session KPIs with a non-UTC timezone (sargable range + pruning hint)", func(t *testing.T) {
		result := waRunQuery(t, suite, workspace.ID, map[string]interface{}{
			"schema":   "web_sessions",
			"measures": []string{"sessions", "bounce_rate", "pageviews", "median_duration"},
			"timezone": "Europe/Paris",
			"timeDimensions": []map[string]interface{}{{
				"dimension": "created_at", "granularity": "day", "dateRange": dateRange,
			}},
		})
		require.NotEmpty(t, result.Data)

		var sessions, pageviews float64
		for _, row := range result.Data {
			sessions += waNumber(t, row["sessions"])
			pageviews += waNumber(t, row["pageviews"])
		}
		assert.Equal(t, float64(5), sessions, "includes the 44h-old session (partition-pruning slack)")
		assert.Equal(t, float64(6), pageviews)
	})

	t.Run("aggregate without buckets: bounce rate and median honor the workspace threshold", func(t *testing.T) {
		result := waRunQuery(t, suite, workspace.ID, map[string]interface{}{
			"schema":   "web_sessions",
			"measures": []string{"sessions", "bounce_rate", "median_duration"},
		})
		require.Len(t, result.Data, 1)
		row := result.Data[0]
		assert.Equal(t, float64(5), waNumber(t, row["sessions"]))
		// 5s, 12s and 15s are under the workspace threshold of 20s. Under the
		// 10s default only 5s would bounce (20%), so this pins the setting.
		assert.InDelta(t, 60.0, waNumber(t, row["bounce_rate"]), 0.01, "3 of 5 sessions under the 20s threshold")
		assert.InDelta(t, 15.0, waNumber(t, row["median_duration"]), 0.01, "median of 5,12,15,25,30s")
	})

	t.Run("channel breakdown with a HAVING metric filter", func(t *testing.T) {
		result := waRunQuery(t, suite, workspace.ID, map[string]interface{}{
			"schema":     "web_sessions",
			"measures":   []string{"sessions"},
			"dimensions": []string{"channel"},
			"having":     []map[string]interface{}{{"member": "sessions", "operator": "gte", "values": []string{"2"}}},
		})
		require.Len(t, result.Data, 1, "only the direct channel has >= 2 sessions")
		assert.Equal(t, "direct", result.Data[0]["channel"])
		assert.Equal(t, float64(3), waNumber(t, result.Data[0]["sessions"]))
	})

	t.Run("cyclic dimension grouping works", func(t *testing.T) {
		result := waRunQuery(t, suite, workspace.ID, map[string]interface{}{
			"schema":     "web_sessions",
			"measures":   []string{"sessions"},
			"dimensions": []string{"hour_of_day"},
		})
		require.NotEmpty(t, result.Data)
		var total float64
		for _, row := range result.Data {
			total += waNumber(t, row["sessions"])
		}
		assert.Equal(t, float64(5), total)
	})

	t.Run("pages: exit rate is real (Staminads' dead metric resurrected)", func(t *testing.T) {
		result := waRunQuery(t, suite, workspace.ID, map[string]interface{}{
			"schema":   "web_pages",
			"measures": []string{"page_count", "exit_page_count", "exit_rate", "landing_page_count"},
		})
		require.Len(t, result.Data, 1)
		row := result.Data[0]
		assert.Equal(t, float64(6), waNumber(t, row["page_count"]))
		assert.Equal(t, float64(5), waNumber(t, row["exit_page_count"]), "one exit per session")
		assert.InDelta(t, 83.33, waNumber(t, row["exit_rate"]), 0.01)
		assert.Equal(t, float64(5), waNumber(t, row["landing_page_count"]))
	})

	t.Run("goals with attribution dimensions", func(t *testing.T) {
		result := waRunQuery(t, suite, workspace.ID, map[string]interface{}{
			"schema":     "web_goals",
			"measures":   []string{"goals", "sum_goal_value", "unique_sessions_with_goals"},
			"dimensions": []string{"goal_name", "channel"},
		})
		require.Len(t, result.Data, 1)
		row := result.Data[0]
		assert.Equal(t, "purchase", row["goal_name"])
		assert.Equal(t, "google-ads", row["channel"])
		assert.Equal(t, float64(1), waNumber(t, row["goals"]))
		assert.InDelta(t, 100.0, waNumber(t, row["sum_goal_value"]), 0.01)
		assert.Equal(t, float64(1), waNumber(t, row["unique_sessions_with_goals"]))
	})

	t.Run("historical rows near the range start are not pruned away", func(t *testing.T) {
		// The partition-pruning predicate widens the requested range before
		// constraining session_date. If that widening is wrong (or applied in
		// the wrong direction), rows sitting near the START of a long range
		// silently disappear from reports while still occupying storage —
		// the most dangerous failure mode of the optimisation, and invisible
		// to fixtures built only from freshly ingested sessions.
		wsDB, err := suite.DBManager.GetWorkspaceDB(workspace.ID)
		require.NoError(t, err)

		old := now.AddDate(0, 0, -10)
		for _, table := range schema.WebAnalyticsTableNames {
			_, err := wsDB.Exec(schema.WebAnalyticsPartitionDDL(table, old))
			require.NoError(t, err)
		}

		_, err = wsDB.Exec(`
			INSERT INTO web_sessions (session_date, id, beat_seq, created_at, updated_at,
				duration_ms, pageview_count, channel)
			VALUES ($1, $2, 1, $3, $3, 42000, 1, 'archived')`,
			old.Format("2006-01-02"), waUUIDv7At(now.Add(-time.Hour), 0x16), old)
		require.NoError(t, err)

		result := waRunQuery(t, suite, workspace.ID, map[string]interface{}{
			"schema":     "web_sessions",
			"measures":   []string{"sessions"},
			"dimensions": []string{"channel"},
			"timeDimensions": []map[string]interface{}{{
				"dimension": "created_at", "granularity": "day",
				"dateRange": []string{now.AddDate(0, 0, -11).Format("2006-01-02"), now.AddDate(0, 0, 1).Format("2006-01-02")},
			}},
		})

		var archived float64
		for _, row := range result.Data {
			if row["channel"] == "archived" {
				archived += waNumber(t, row["sessions"])
			}
		}
		assert.Equal(t, float64(1), archived,
			"a 10-day-old session must still be returned by a query covering it")
	})

	t.Run("analytics.schemas exposes the web schemas for this workspace", func(t *testing.T) {
		resp, err := suite.APIClient.Post("/api/analytics.schemas", map[string]interface{}{
			"workspace_id": workspace.ID,
		})
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		var schemas map[string]interface{}
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&schemas))
		raw, _ := json.Marshal(schemas)
		assert.Contains(t, string(raw), "web_sessions")
		assert.Contains(t, string(raw), "bounce_rate")
	})
}
