//go:build integration
// +build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/tests/testutil"
)

// What POST /api/analytics.query and /api/analytics.schemas answer, read as raw JSON: rows,
// their count and the members' names, types, titles and descriptions, never SQL. And the
// schemas a query is checked against: the workspace's own, its web schemas included only when
// it has web analytics, read once per query by the repository that runs it.
func TestAnalyticsAPIAnswers(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer func() { suite.Cleanup() }()
	factory := suite.DataFactory
	workspaceRepo := suite.ServerManager.GetApp().GetWorkspaceRepository()

	join := func(workspace *domain.Workspace) {
		t.Helper()
		require.NoError(t, workspaceRepo.AddUserToWorkspace(context.Background(), &domain.UserWorkspace{
			UserID:      "550e8400-e29b-41d4-a716-446655440000", // testuser@example.com
			WorkspaceID: workspace.ID,
			Role:        "owner",
			Permissions: domain.FullPermissions,
			CreatedAt:   time.Now().UTC(),
			UpdatedAt:   time.Now().UTC(),
		}))
	}
	plain, err := factory.CreateWorkspace()
	require.NoError(t, err)
	join(plain)
	web, err := factory.CreateWorkspace(func(w *domain.Workspace) {
		w.Settings.WebAnalytics = &domain.WebAnalyticsSettings{
			Enabled:                true,
			BounceThresholdSeconds: 20,
			Filters:                domain.DefaultWebFilters(),
		}
	})
	require.NoError(t, err)
	join(web)

	template, err := factory.CreateTemplate(plain.ID)
	require.NoError(t, err)
	_, err = factory.CreateMessageHistory(plain.ID, testutil.WithMessageTemplate(template.ID),
		testutil.WithMessageBroadcast("spring_sale"))
	require.NoError(t, err)

	require.NoError(t, suite.APIClient.Login("testuser@example.com", ""))

	post := func(t *testing.T, endpoint, body string) (int, string) {
		t.Helper()
		resp, err := suite.APIClient.PostRaw(endpoint, body)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()
		raw, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		return resp.StatusCode, string(raw)
	}
	query := func(t *testing.T, workspaceID, query string) (int, string) {
		t.Helper()
		return post(t, "/api/analytics.query", fmt.Sprintf(`{"workspace_id":%q,"query":%s}`, workspaceID, query))
	}
	// schemas returns the catalogue of a workspace: each schema's measures and dimensions, by
	// name, as raw JSON objects.
	schemas := func(t *testing.T, workspaceID string) (string, map[string]map[string]map[string]map[string]json.RawMessage) {
		t.Helper()
		status, body := post(t, "/api/analytics.schemas", fmt.Sprintf(`{"workspace_id":%q}`, workspaceID))
		require.Equal(t, http.StatusOK, status, body)
		var decoded struct {
			Schemas map[string]map[string]json.RawMessage `json:"schemas"`
		}
		require.NoError(t, json.Unmarshal([]byte(body), &decoded))
		out := map[string]map[string]map[string]map[string]json.RawMessage{}
		for name, schema := range decoded.Schemas {
			out[name] = map[string]map[string]map[string]json.RawMessage{}
			for _, kind := range []string{"measures", "dimensions"} {
				var members map[string]map[string]json.RawMessage
				require.NoError(t, json.Unmarshal(schema[kind], &members), "%s.%s", name, kind)
				out[name][kind] = members
			}
		}
		return body, out
	}

	t.Run("analytics.query answers the rows and their count, without the SQL", func(t *testing.T) {
		status, body := query(t, plain.ID, `{"schema":"message_history","measures":["count"],"filters":[{"member":"broadcast_id","operator":"equals","values":["spring_sale"]}]}`)
		require.Equal(t, http.StatusOK, status, body)
		assert.JSONEq(t, `{"data":[{"count":1}],"meta":{"total":1}}`, body)
		assert.NotContains(t, body, "spring_sale", "the filter's value is not echoed")
	})

	t.Run("analytics.schemas lists each member without its SQL", func(t *testing.T) {
		body, catalogue := schemas(t, plain.ID)
		require.Contains(t, catalogue, "message_history")
		for schemaName, kinds := range catalogue {
			for kind, members := range kinds {
				for memberName, member := range members {
					for key := range member {
						assert.Contains(t, []string{"type", "title", "description"}, key, "%s.%s.%s", schemaName, kind, memberName)
					}
				}
			}
		}
		assert.NotContains(t, body, `"sql"`)
		assert.NotContains(t, body, `"filters"`)
	})

	t.Run("the web schemas are listed for a workspace with web analytics only", func(t *testing.T) {
		_, catalogue := schemas(t, web.ID)
		assert.Contains(t, catalogue, "web_sessions")
		assert.Contains(t, catalogue, "message_history")
		_, catalogue = schemas(t, plain.ID)
		assert.NotContains(t, catalogue, "web_sessions")
	})

	t.Run("a query the workspace's schemas refuse answers 400 with what to change", func(t *testing.T) {
		for _, tc := range []struct{ workspaceID, query, want string }{
			{plain.ID, `{"schema":"web_sessions","measures":["sessions"]}`, `unknown schema \"web_sessions\"`},
			{plain.ID, `{"schema":"message_history","measures":["count_everything"]}`, `measure \"count_everything\" is not in schema \"message_history\"`},
			{web.ID, `{"schema":"web_sessions","measures":["sessions"],"dimensions":["no_such_dimension"]}`, `no_such_dimension`},
		} {
			status, body := query(t, tc.workspaceID, tc.query)
			assert.Equal(t, http.StatusBadRequest, status, body)
			assert.Contains(t, body, tc.want)
		}
	})
}
