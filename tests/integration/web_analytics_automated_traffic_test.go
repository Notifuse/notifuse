//go:build integration
// +build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/tests/testutil"
)

// TestWebAnalyticsAutomatedTraffic runs the automated-traffic rule end to end:
// beats through /track, the session rollup, then analytics.query. Automated
// sessions still count as sessions, but TimeScore and the bounce rate are
// computed over human sessions only.
func TestWebAnalyticsAutomatedTraffic(t *testing.T) {
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

	baseURL := suite.ServerManager.GetURL()
	buffer := suite.ServerManager.GetApp().GetWebAnalyticsBuffer()
	now := time.Now().UTC()

	short := waUUIDv7At(now.Add(-6*time.Minute), 0x23)
	beats := []waBeat{
		// Humans: 5s (a bounce), 12s and 30s.
		{workspaceID: workspace.ID, sessionID: waUUIDv7At(now.Add(-10*time.Minute), 0x21), seq: 1,
			actions: []map[string]interface{}{waPageview("/", 1, 5000, 10, now)}},
		{workspaceID: workspace.ID, sessionID: waUUIDv7At(now.Add(-9*time.Minute), 0x22), seq: 1,
			actions: []map[string]interface{}{waPageview("/pricing", 1, 12000, 40, now)}},
		{workspaceID: workspace.ID, sessionID: waUUIDv7At(now.Add(-8*time.Minute), 0x24), seq: 1,
			actions: []map[string]interface{}{waPageview("/blog", 1, 30000, 80, now)}},
		// A visitor who left after 600ms: under a second, but the second beat
		// (sent on pagehide) is what makes it human.
		{workspaceID: workspace.ID, sessionID: short, seq: 1,
			actions: []map[string]interface{}{waPageview("/", 1, 0, 0, now)}},
		{workspaceID: workspace.ID, sessionID: short, seq: 2,
			actions: []map[string]interface{}{waPageview("/", 1, 600, 0, now)}},
		// Automated: the first beat and nothing after it.
		{workspaceID: workspace.ID, sessionID: waUUIDv7At(now.Add(-5*time.Minute), 0x25), seq: 1,
			actions: []map[string]interface{}{waPageview("/", 1, 0, 0, now)}},
		{workspaceID: workspace.ID, sessionID: waUUIDv7At(now.Add(-4*time.Minute), 0x26), seq: 1,
			actions: []map[string]interface{}{waPageview("/pricing", 1, 40, 0, now)}},
	}
	for _, beat := range beats {
		resp := waPostBeat(t, baseURL, beat.body(t, now), nil)
		_ = waCloseAndDecode(t, resp)
		// The short session's second beat must land after its first.
		buffer.FlushAll(context.Background())
	}

	require.NoError(t, suite.APIClient.Login("testuser@example.com", ""))

	t.Run("sessions count both, TimeScore and bounce rate count humans", func(t *testing.T) {
		result := waRunQuery(t, suite, workspace.ID, map[string]interface{}{
			"schema": "web_sessions",
			"measures": []string{"sessions", "automated_sessions", "automated_share",
				"median_duration", "bounce_rate"},
		})
		require.Len(t, result.Data, 1)
		row := result.Data[0]
		assert.Equal(t, 6.0, waNumber(t, row["sessions"]))
		assert.Equal(t, 2.0, waNumber(t, row["automated_sessions"]))
		assert.InDelta(t, 33.33, waNumber(t, row["automated_share"]), 0.01)
		// Humans: 0.6s, 5s, 12s, 30s. With the automated sessions counted it would be 2.8s.
		assert.InDelta(t, 8.5, waNumber(t, row["median_duration"]), 0.01, "median of 0.6, 5, 12, 30s")
		// 0.6s and 5s bounce out of four humans; counting automated traffic would give 4 of 6.
		assert.InDelta(t, 50.0, waNumber(t, row["bounce_rate"]), 0.01)
	})

	t.Run("traffic_type groups and filters", func(t *testing.T) {
		result := waRunQuery(t, suite, workspace.ID, map[string]interface{}{
			"schema":     "web_sessions",
			"measures":   []string{"sessions"},
			"dimensions": []string{"traffic_type"},
			"order":      map[string]string{"traffic_type": "asc"},
		})
		require.Len(t, result.Data, 2)
		assert.Equal(t, "automated", result.Data[0]["traffic_type"])
		assert.Equal(t, 2.0, waNumber(t, result.Data[0]["sessions"]))
		assert.Equal(t, "human", result.Data[1]["traffic_type"])
		assert.Equal(t, 4.0, waNumber(t, result.Data[1]["sessions"]))

		filtered := waRunQuery(t, suite, workspace.ID, map[string]interface{}{
			"schema":     "web_sessions",
			"measures":   []string{"sessions"},
			"dimensions": []string{"exit_path"},
			"filters": []map[string]interface{}{
				{"member": "traffic_type", "operator": "equals", "values": []string{"automated"}},
			},
			"order": map[string]string{"exit_path": "asc"},
		})
		require.Len(t, filtered.Data, 2)
		assert.Equal(t, "/", filtered.Data[0]["exit_path"])
		assert.Equal(t, "/pricing", filtered.Data[1]["exit_path"])
	})

	t.Run("a period with only automated traffic has no TimeScore", func(t *testing.T) {
		result := waRunQuery(t, suite, workspace.ID, map[string]interface{}{
			"schema":   "web_sessions",
			"measures": []string{"sessions", "median_duration", "bounce_rate"},
			"filters": []map[string]interface{}{
				{"member": "traffic_type", "operator": "equals", "values": []string{"automated"}},
			},
		})
		require.Len(t, result.Data, 1)
		assert.Equal(t, 2.0, waNumber(t, result.Data[0]["sessions"]))
		assert.Nil(t, result.Data[0]["median_duration"])
		assert.Nil(t, result.Data[0]["bounce_rate"])
	})

	// Last, because it adds sessions the counts above do not expect.
	t.Run("the 1s bound, and a page no human reached sorts last", func(t *testing.T) {
		edge := []waBeat{
			{workspaceID: workspace.ID, sessionID: waUUIDv7At(now.Add(-3*time.Minute), 0x27), seq: 1,
				actions: []map[string]interface{}{waPageview("/edge-999", 1, 999, 0, now)}},
			{workspaceID: workspace.ID, sessionID: waUUIDv7At(now.Add(-2*time.Minute), 0x28), seq: 1,
				actions: []map[string]interface{}{waPageview("/edge-1000", 1, 1000, 0, now)}},
		}
		for _, beat := range edge {
			_ = waCloseAndDecode(t, waPostBeat(t, baseURL, beat.body(t, now), nil))
		}
		buffer.FlushAll(context.Background())

		bound := waRunQuery(t, suite, workspace.ID, map[string]interface{}{
			"schema":     "web_sessions",
			"measures":   []string{"sessions"},
			"dimensions": []string{"exit_path", "traffic_type"},
			"filters": []map[string]interface{}{
				{"member": "exit_path", "operator": "in", "values": []string{"/edge-999", "/edge-1000"}},
			},
			"order": map[string]string{"exit_path": "asc"},
		})
		require.Len(t, bound.Data, 2)
		assert.Equal(t, "/edge-1000", bound.Data[0]["exit_path"])
		assert.Equal(t, "human", bound.Data[0]["traffic_type"], "1000ms in a single beat is a person")
		assert.Equal(t, "/edge-999", bound.Data[1]["exit_path"])
		assert.Equal(t, "automated", bound.Data[1]["traffic_type"], "999ms in a single beat is not")

		// /edge-999 has only an automated session, so no TimeScore: a descending sort must not put it first.
		ranked := waRunQuery(t, suite, workspace.ID, map[string]interface{}{
			"schema":     "web_sessions",
			"measures":   []string{"median_duration", "automated_sessions", "automated_share"},
			"dimensions": []string{"exit_path"},
			"order":      map[string]string{"median_duration": "desc"},
		})
		require.NotEmpty(t, ranked.Data)
		assert.Equal(t, "/blog", ranked.Data[0]["exit_path"], "the longest human median comes first")
		last := ranked.Data[len(ranked.Data)-1]
		assert.Equal(t, "/edge-999", last["exit_path"])
		assert.Nil(t, last["median_duration"])
		assert.Equal(t, 1.0, waNumber(t, last["automated_sessions"]))
		assert.InDelta(t, 100.0, waNumber(t, last["automated_share"]), 0.01)
	})
}
