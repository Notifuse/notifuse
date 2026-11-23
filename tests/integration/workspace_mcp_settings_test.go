package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/service"
	"github.com/Notifuse/notifuse/tests/testutil"
)

// The workspace MCP switch: written by the owner through its own route, read
// back on workspaces.get, untouched by workspaces.update, and without effect
// on API keys calling /api/*.
func TestWorkspaceMCPSettings(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer func() { suite.Cleanup() }()

	factory := suite.DataFactory
	authService := suite.ServerManager.GetApp().GetAuthService().(*service.AuthService)

	owner, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	require.NoError(t, factory.AddUserToWorkspace(owner.ID, workspace.ID, "owner"))
	apiKey, err := factory.CreateAPIKey(workspace.ID)
	require.NoError(t, err)

	ownerClient := testutil.NewAPIClient(suite.ServerManager.GetURL())
	require.NoError(t, ownerClient.Login(owner.Email, "password"))
	keyClient := testutil.NewAPIClient(suite.ServerManager.GetURL())
	keyClient.SetToken(authService.GenerateAPIAuthToken(apiKey))

	postRaw := func(t *testing.T, client *testutil.APIClient, endpoint, body string) (int, string) {
		t.Helper()
		resp, err := client.PostRaw(endpoint, body)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()
		data, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		return resp.StatusCode, string(data)
	}
	setMCP := func(t *testing.T, client *testutil.APIClient, disabled string) (int, string) {
		t.Helper()
		return postRaw(t, client, "/api/workspaces.setMCPSettings",
			fmt.Sprintf(`{"workspace_id":%q,"disabled":%s}`, workspace.ID, disabled))
	}
	// storedMCP reads the switch back as workspaces.get serves it: the raw
	// "mcp" value of the settings, absent when never set.
	storedMCP := func(t *testing.T) (json.RawMessage, bool) {
		t.Helper()
		resp, err := ownerClient.Get("/api/workspaces.get", map[string]string{"id": workspace.ID})
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()
		data, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode, string(data))
		var got struct {
			Workspace struct {
				Settings map[string]json.RawMessage `json:"settings"`
			} `json:"workspace"`
		}
		require.NoError(t, json.Unmarshal(data, &got))
		raw, ok := got.Workspace.Settings["mcp"]
		return raw, ok
	}
	listLists := func(t *testing.T) int {
		t.Helper()
		resp, err := keyClient.Get("/api/lists.list", map[string]string{"workspace_id": workspace.ID})
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()
		return resp.StatusCode
	}

	t.Run("a workspace starts with no switch stored", func(t *testing.T) {
		_, ok := storedMCP(t)
		assert.False(t, ok)
	})

	t.Run("a body without disabled is refused", func(t *testing.T) {
		status, _ := postRaw(t, ownerClient, "/api/workspaces.setMCPSettings", fmt.Sprintf(`{"workspace_id":%q}`, workspace.ID))
		assert.Equal(t, http.StatusBadRequest, status)
		_, ok := storedMCP(t)
		assert.False(t, ok, "nothing was stored")
	})

	t.Run("an API key cannot switch it", func(t *testing.T) {
		status, _ := setMCP(t, keyClient, "true")
		assert.Equal(t, http.StatusForbidden, status)
		_, ok := storedMCP(t)
		assert.False(t, ok)
	})

	t.Run("the owner switches MCP off", func(t *testing.T) {
		status, body := setMCP(t, ownerClient, "true")
		require.Equal(t, http.StatusOK, status, body)
		assert.JSONEq(t, `{"mcp":{"disabled":true}}`, body)

		raw, ok := storedMCP(t)
		require.True(t, ok)
		assert.JSONEq(t, `{"disabled":true}`, string(raw))
	})

	t.Run("an API key still works on /api/* while MCP is off", func(t *testing.T) {
		assert.Equal(t, http.StatusOK, listLists(t))
	})

	t.Run("workspaces.update carrying mcp does not change it", func(t *testing.T) {
		for _, mcp := range []string{`{"disabled":false}`, `null`} {
			status, body := postRaw(t, ownerClient, "/api/workspaces.update", fmt.Sprintf(
				`{"id":%q,"name":"MCP switch workspace","settings":{"timezone":"UTC","default_language":"en","languages":["en"],"mcp":%s}}`,
				workspace.ID, mcp))
			require.Equal(t, http.StatusOK, status, body)

			raw, ok := storedMCP(t)
			require.True(t, ok, mcp)
			assert.JSONEq(t, `{"disabled":true}`, string(raw), mcp)
		}
	})

	t.Run("the owner switches MCP back on", func(t *testing.T) {
		status, body := setMCP(t, ownerClient, "false")
		require.Equal(t, http.StatusOK, status, body)
		assert.JSONEq(t, `{"mcp":{"disabled":false}}`, body)

		raw, ok := storedMCP(t)
		require.True(t, ok)
		assert.JSONEq(t, `{"disabled":false}`, string(raw))
		assert.Equal(t, http.StatusOK, listLists(t))
	})

	t.Run("each change is in the audit log", func(t *testing.T) {
		page := listAuditLogs(t, ownerClient, map[string]string{"workspace_id": workspace.ID, "actions": "workspaces.setMCPSettings"})
		var succeeded, denied int
		for _, row := range page.Logs {
			require.Equal(t, "workspaces.setMCPSettings", row.Action)
			assert.Equal(t, domain.AuditTargetWorkspace, row.TargetType)
			assert.Equal(t, workspace.ID, row.TargetID)
			switch row.Outcome {
			case domain.AuditOutcomeSuccess:
				succeeded++
			case domain.AuditOutcomeDenied:
				denied++
			}
		}
		assert.Equal(t, 2, succeeded, "off, then on")
		assert.Equal(t, 1, denied, "the API key's attempt")
	})

	// Runs last: it switches MCP off again. A settings save made from a copy read
	// before the switch must not write the old switch back.
	t.Run("a save from a copy read before the switch keeps it off", func(t *testing.T) {
		repo := suite.ServerManager.GetApp().GetWorkspaceRepository()
		stale, err := repo.GetByID(context.Background(), workspace.ID)
		require.NoError(t, err)
		require.True(t, stale.MCPEnabled())

		status, body := setMCP(t, ownerClient, "true")
		require.Equal(t, http.StatusOK, status, body)

		stale.Settings.Timezone = "Europe/Paris"
		require.NoError(t, repo.Update(context.Background(), stale))

		raw, ok := storedMCP(t)
		require.True(t, ok)
		assert.JSONEq(t, `{"disabled":true}`, string(raw), "the stale save must not switch MCP back on")
		saved, err := repo.GetByID(context.Background(), workspace.ID)
		require.NoError(t, err)
		assert.Equal(t, "Europe/Paris", saved.Settings.Timezone, "the rest of the save is kept")
	})
}
