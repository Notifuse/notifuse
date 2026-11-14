package integration

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/app"
	"github.com/Notifuse/notifuse/tests/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAutomationGetNotFound: automations.get, update, delete, activate and pause on an
// automation that does not exist answer 404, not the 500 they used to.
func TestAutomationGetNotFound(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	client := suite.APIClient
	factory := suite.DataFactory

	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	require.NoError(t, factory.AddUserToWorkspace(user.ID, workspace.ID, "owner"))
	require.NoError(t, client.Login(user.Email, "password"))
	client.SetWorkspaceID(workspace.ID)

	resp, err := client.GetAutomation("does-not-exist")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assertNotFound := func(t *testing.T, resp *http.Response) {
		t.Helper()
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
		var body map[string]interface{}
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
		assert.Equal(t, "Automation not found", body["error"])
	}
	assertNotFound(t, resp)

	list, err := factory.CreateList(workspace.ID)
	require.NoError(t, err)
	unknown := map[string]interface{}{"workspace_id": workspace.ID, "automation_id": "does-not-exist"}
	for name, call := range map[string]func() (*http.Response, error){
		"update": func() (*http.Response, error) {
			return client.UpdateAutomation(map[string]interface{}{
				"workspace_id": workspace.ID,
				"automation": map[string]interface{}{
					"id": "does-not-exist", "workspace_id": workspace.ID, "name": "Unknown", "status": "draft",
					"list_id":      list.ID,
					"trigger":      map[string]interface{}{"event_kind": "custom_event", "custom_event_name": "unknown", "frequency": "once"},
					"root_node_id": "root",
					"nodes": []map[string]interface{}{{"id": "root", "automation_id": "does-not-exist", "type": "trigger",
						"config": map[string]interface{}{}, "position": map[string]interface{}{"x": 0, "y": 0}}},
				},
			})
		},
		"delete":   func() (*http.Response, error) { return client.DeleteAutomation(unknown) },
		"activate": func() (*http.Response, error) { return client.ActivateAutomation(unknown) },
		"pause":    func() (*http.Response, error) { return client.PauseAutomation(unknown) },
	} {
		t.Run(name, func(t *testing.T) {
			resp, err := call()
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()
			assertNotFound(t, resp)
		})
	}
}
