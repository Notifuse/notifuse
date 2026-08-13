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

// TestWebAnalyticsBackfillPreservesTrackerDimensions covers the half of the
// backfill contract that the end-to-end test cannot: what it must NOT rewrite.
//
// A backfill is triggered by editing an attribution rule. It used to clear
// custom_1..custom_10 along with channel and channel_group, on the reasoning that
// a dimension no rule writes has no value — true for channel, false for custom_*,
// which the tracker fills from the beat's own dimensions. So editing an unrelated
// rule silently destroyed data the site had supplied, and nothing connected the
// loss to the action that caused it.
//
// CRITICAL: no beat is sent after the backfill. The session upsert is sticky
// (COALESCE/NULLIF keeps the first non-empty writer), so a later beat carrying
// the same dimensions would restore the values and this test would pass against
// the broken code it exists to catch.
func TestWebAnalyticsBackfillPreservesTrackerDimensions(t *testing.T) {
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

	// One session whose custom dimensions come from the tracker, not from a rule.
	beat := waBeat{
		workspaceID: workspace.ID,
		sessionID:   waUUIDv7At(now.Add(-6*time.Minute), 0x41),
		seq:         1,
		utmIDFrom:   "gclid",
		dimensions:  map[string]string{"custom_1": "pro-plan", "custom_7": "eu-west"},
		actions: []map[string]interface{}{
			waPageview("/pricing", 1, 1000, 10, now),
			{"type": "goal", "name": "signup", "path": "/pricing", "page_number": 1,
				"goal_type": "signup", "timestamp": now.UnixMilli(), "value": 1.0},
		},
	}
	require.Equal(t, true, waCloseAndDecode(t, waPostBeat(t, baseURL, beat.body(t, now), nil))["success"])
	buffer.FlushAll(context.Background())

	wsDB, err := suite.DBManager.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)

	readCustoms := func(t *testing.T, table string) (string, string) {
		t.Helper()
		var c1, c7 string
		require.NoError(t, wsDB.QueryRow(
			`SELECT custom_1, custom_7 FROM `+table+` LIMIT 1`).Scan(&c1, &c7))
		return c1, c7
	}

	// The fixture must really carry them, or the assertion after the backfill
	// would hold for the wrong reason.
	c1, c7 := readCustoms(t, "web_sessions")
	require.Equal(t, "pro-plan", c1)
	require.Equal(t, "eu-west", c7)

	require.NoError(t, suite.APIClient.Login("testuser@example.com", ""))

	// Edit an attribution rule — the ordinary action that triggers a backfill and
	// that has nothing to do with custom dimensions.
	resp, err := suite.APIClient.Post("/api/workspaces.setWebAnalyticsSettings", map[string]interface{}{
		"workspace_id": workspace.ID,
		"settings": map[string]interface{}{
			"enabled":         true,
			"allowed_domains": []string{"example.com"},
			"filters": []domain.WebFilter{{
				ID: "rebrand-rule", Name: "Rebrand", Priority: 500, Enabled: true,
				Operations: []domain.WebFilterOperation{
					{Dimension: "channel", Action: domain.WebFilterActionSetValue, Value: "rebranded"},
				},
			}},
		},
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	_ = resp.Body.Close()

	resp, err = suite.APIClient.Post("/api/webAnalytics.backfillStart", map[string]interface{}{"workspace_id": workspace.ID})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	_ = resp.Body.Close()

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
			break
		}
		require.NotEqual(t, "failed", statusBody.Backfill.Status, "backfill failed: %+v", statusBody.Backfill)
		time.Sleep(500 * time.Millisecond)
	}

	// No beat is sent between here and the assertions. See the note at the top.

	t.Run("session custom dimensions survive the backfill", func(t *testing.T) {
		c1, c7 := readCustoms(t, "web_sessions")
		assert.Equal(t, "pro-plan", c1, "the tracker set this; no rule ever touched it")
		assert.Equal(t, "eu-west", c7)
	})

	t.Run("goal custom dimensions survive too", func(t *testing.T) {
		// web_goals is rewritten by the same clause, and its rows are never
		// re-upserted by a later beat — so a regression here is permanent.
		c1, c7 := readCustoms(t, "web_goals")
		assert.Equal(t, "pro-plan", c1)
		assert.Equal(t, "eu-west", c7)
	})

	t.Run("the backfill still did its actual job", func(t *testing.T) {
		// Preserving custom dimensions must not be achieved by skipping the
		// rewrite altogether.
		var channel string
		require.NoError(t, wsDB.QueryRow(`SELECT channel FROM web_sessions LIMIT 1`).Scan(&channel))
		assert.Equal(t, "rebranded", channel)

		var goalChannel string
		require.NoError(t, wsDB.QueryRow(`SELECT channel FROM web_goals LIMIT 1`).Scan(&goalChannel))
		assert.Equal(t, "rebranded", goalChannel)
	})

	t.Run("a dimension no live rule writes is still cleared", func(t *testing.T) {
		// channel_group had a value from the default rules and no rule writes it
		// now, so it must be empty. This is the behaviour the reset list exists
		// for, and it must survive the narrowing.
		var group string
		require.NoError(t, wsDB.QueryRow(`SELECT channel_group FROM web_sessions LIMIT 1`).Scan(&group))
		assert.Equal(t, "", group)
	})
}
