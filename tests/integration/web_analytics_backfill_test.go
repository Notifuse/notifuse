//go:build integration
// +build integration

package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/tests/testutil"
)

func TestWebAnalyticsBackfillEndToEnd(t *testing.T) {
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

	workspaceRepo := suite.ServerManager.GetApp().GetWorkspaceRepository()
	require.NoError(t, workspaceRepo.AddUserToWorkspace(context.Background(), &domain.UserWorkspace{
		UserID:      "550e8400-e29b-41d4-a716-446655440000",
		WorkspaceID: workspace.ID,
		Role:        "owner",
		Permissions: domain.FullPermissions,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}))

	baseURL := suite.ServerManager.GetURL()
	buffer := suite.ServerManager.GetApp().GetWebAnalyticsBuffer()
	now := time.Now().UTC()

	// Two sessions (one paid via gclid, one direct) plus a goal.
	for i, beat := range []waBeat{
		{workspaceID: workspace.ID, sessionID: waUUIDv7At(now.Add(-6*time.Minute), 0x21), seq: 1, utmIDFrom: "gclid",
			actions: []map[string]interface{}{
				waPageview("/a", 1, 1000, 10, now),
				{"type": "goal", "name": "signup", "path": "/a", "page_number": 1, "timestamp": now.UnixMilli(), "value": 1.0},
			}},
		{workspaceID: workspace.ID, sessionID: waUUIDv7At(now.Add(-5*time.Minute), 0x22), seq: 1,
			actions: []map[string]interface{}{waPageview("/b", 1, 1000, 10, now)}},
	} {
		resp := waPostBeat(t, baseURL, beat.body(t, now), nil)
		body := waCloseAndDecode(t, resp)
		require.Equal(t, true, body["success"], "beat %d", i)
	}
	buffer.FlushAll(context.Background())

	wsDB, err := suite.DBManager.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)

	var paidChannel string
	require.NoError(t, wsDB.QueryRow(`SELECT channel FROM web_sessions WHERE utm_id_from = 'gclid'`).Scan(&paidChannel))
	require.Equal(t, "google-ads", paidChannel)

	require.NoError(t, suite.APIClient.Login("testuser@example.com", ""))

	// Replace the rules: everything becomes channel "rebranded".
	newFilters := []domain.WebFilter{{
		ID: "rebrand-rule", Name: "Rebrand", Priority: 500, Enabled: true,
		Operations: []domain.WebFilterOperation{
			{Dimension: "channel", Action: domain.WebFilterActionSetValue, Value: "rebranded"},
			{Dimension: "channel_group", Action: domain.WebFilterActionSetValue, Value: "rebranded-group"},
		},
	}}
	resp, err := suite.APIClient.Post("/api/workspaces.setWebAnalyticsSettings", map[string]interface{}{
		"workspace_id": workspace.ID,
		"settings": map[string]interface{}{
			"enabled":         true,
			"allowed_domains": []string{"example.com"},
			"filters":         newFilters,
		},
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	_ = resp.Body.Close()

	// Start the backfill.
	resp, err = suite.APIClient.Post("/api/webAnalytics.backfillStart", map[string]interface{}{"workspace_id": workspace.ID})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	_ = resp.Body.Close()

	// A concurrent start is rejected while the first is pending.
	resp, err = suite.APIClient.Post("/api/webAnalytics.backfillStart", map[string]interface{}{"workspace_id": workspace.ID})
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	_ = resp.Body.Close()

	// Drive the task system and wait for completion.
	deadline := time.Now().Add(45 * time.Second)
	for {
		require.True(t, time.Now().Before(deadline), "backfill did not complete in time")

		cronResp, err := suite.APIClient.ExecutePendingTasks(10)
		require.NoError(t, err)
		_ = cronResp.Body.Close()

		statusResp, err := suite.APIClient.Post("/api/webAnalytics.backfillStatus", map[string]interface{}{"workspace_id": workspace.ID})
		require.NoError(t, err)
		var statusBody struct {
			Backfill *domain.WebAnalyticsBackfillStatus `json:"backfill"`
		}
		require.NoError(t, json.NewDecoder(statusResp.Body).Decode(&statusBody))
		_ = statusResp.Body.Close()
		require.NotNil(t, statusBody.Backfill)

		if statusBody.Backfill.Status == "completed" {
			require.NotNil(t, statusBody.Backfill.State)
			assert.GreaterOrEqual(t, statusBody.Backfill.State.RowsUpdated, int64(3), "2 sessions + 1 goal rewritten")
			break
		}
		require.NotEqual(t, "failed", statusBody.Backfill.Status, "backfill failed: %+v", statusBody.Backfill)
		time.Sleep(500 * time.Millisecond)
	}

	// Both tables now carry the new attribution.
	var channels []string
	rows, err := wsDB.Query(`SELECT DISTINCT channel FROM web_sessions`)
	require.NoError(t, err)
	for rows.Next() {
		var c string
		require.NoError(t, rows.Scan(&c))
		channels = append(channels, c)
	}
	require.NoError(t, rows.Err())
	_ = rows.Close()
	assert.Equal(t, []string{"rebranded"}, channels)

	var goalChannel, goalGroup string
	require.NoError(t, wsDB.QueryRow(`SELECT channel, channel_group FROM web_goals LIMIT 1`).Scan(&goalChannel, &goalGroup))
	assert.Equal(t, "rebranded", goalChannel)
	assert.Equal(t, "rebranded-group", goalGroup)

	// The paid session's utm_id_from survives (passthrough dimension).
	var utmIDFrom string
	require.NoError(t, wsDB.QueryRow(`SELECT utm_id_from FROM web_sessions WHERE utm_id_from <> ''`).Scan(&utmIDFrom))
	assert.Equal(t, "gclid", utmIDFrom)

	// A backfill must also CLEAR values that no longer match any rule.
	// Without that reset, a dimension keeps a stale classification forever and
	// reports silently disagree with the rules the user is looking at.
	unmatchable := []domain.WebFilter{{
		ID: "never-matches", Name: "Never matches", Priority: 500, Enabled: true,
		Conditions: []domain.WebFilterCondition{
			{Field: "utm_source", Operator: domain.WebFilterOpEquals, Value: "no-such-source"},
		},
		Operations: []domain.WebFilterOperation{
			{Dimension: "channel", Action: domain.WebFilterActionSetValue, Value: "unreachable"},
		},
	}}
	resp, err = suite.APIClient.Post("/api/workspaces.setWebAnalyticsSettings", map[string]interface{}{
		"workspace_id": workspace.ID,
		"settings": map[string]interface{}{
			"enabled":         true,
			"allowed_domains": []string{"example.com"},
			"filters":         unmatchable,
		},
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	_ = resp.Body.Close()

	resp, err = suite.APIClient.Post("/api/webAnalytics.backfillStart", map[string]interface{}{"workspace_id": workspace.ID})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	_ = resp.Body.Close()

	deadline = time.Now().Add(45 * time.Second)
	for {
		require.True(t, time.Now().Before(deadline), "second backfill did not complete in time")
		cronResp, err := suite.APIClient.ExecutePendingTasks(10)
		require.NoError(t, err)
		_ = cronResp.Body.Close()

		statusResp, err := suite.APIClient.Post("/api/webAnalytics.backfillStatus", map[string]interface{}{"workspace_id": workspace.ID})
		require.NoError(t, err)
		var statusBody struct {
			Backfill *domain.WebAnalyticsBackfillStatus `json:"backfill"`
		}
		require.NoError(t, json.NewDecoder(statusResp.Body).Decode(&statusBody))
		_ = statusResp.Body.Close()
		require.NotNil(t, statusBody.Backfill)

		if statusBody.Backfill.Status == "completed" {
			break
		}
		require.NotEqual(t, "failed", statusBody.Backfill.Status, "backfill failed: %+v", statusBody.Backfill)
		time.Sleep(500 * time.Millisecond)
	}

	var stillRebranded int
	require.NoError(t, wsDB.QueryRow(
		`SELECT COUNT(*) FROM web_sessions WHERE channel <> ''`).Scan(&stillRebranded))
	assert.Zero(t, stillRebranded, "rows matching no rule must have their channel cleared, not keep the stale value")

	var goalChannelAfter string
	require.NoError(t, wsDB.QueryRow(`SELECT channel FROM web_goals LIMIT 1`).Scan(&goalChannelAfter))
	assert.Equal(t, "", goalChannelAfter, "goal snapshots must be cleared too")
}
