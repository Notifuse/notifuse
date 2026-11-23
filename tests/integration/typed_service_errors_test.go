//go:build integration

package integration

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/app"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/service"
	"github.com/Notifuse/notifuse/tests/testutil"
)

// TestTypedServiceErrors checks, on the real routes, that a client mistake answers its own
// status rather than the 500 these endpoints used to give: 409 for an action the object's state
// refuses, 404 for an object that is not there, 400 for a segment tree that cannot compile. And
// that a webhook node's secret reaches only a caller who may write automations.
func TestTypedServiceErrors(t *testing.T) {
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
	// A schedule checks the marketing provider before the broadcast's status.
	_, err = factory.SetupWorkspaceWithSMTPProvider(workspace.ID)
	require.NoError(t, err)
	require.NoError(t, client.Login(user.Email, "password"))
	client.SetWorkspaceID(workspace.ID)

	// post sends a raw JSON body, as an API client does, and returns the status and the
	// decoded body.
	post := func(t *testing.T, endpoint, body string) (int, map[string]interface{}) {
		t.Helper()
		resp, err := client.Post(endpoint, json.RawMessage(body))
		require.NoError(t, err)
		return decodeResponse(t, resp)
	}

	t.Run("a schedule of a broadcast that is not a draft answers 409", func(t *testing.T) {
		broadcast, err := factory.CreateBroadcast(workspace.ID, testutil.WithBroadcastStatus(domain.BroadcastStatusProcessing))
		require.NoError(t, err)

		status, body := post(t, "/api/broadcasts.schedule",
			fmt.Sprintf(`{"workspace_id":%q,"id":%q,"send_now":true}`, workspace.ID, broadcast.ID))

		assert.Equal(t, http.StatusConflict, status, "body: %v", body)
		assert.Equal(t, map[string]interface{}{
			"entity": "broadcast", "id": broadcast.ID, "action": "schedule", "status": "processing",
		}, body["invalid_state"])
	})

	t.Run("an unknown transactional notification answers 404", func(t *testing.T) {
		resp, err := client.Get("/api/transactional.get", map[string]string{"id": "nosuchnotification"})
		require.NoError(t, err)
		status, body := decodeResponse(t, resp)

		assert.Equal(t, http.StatusNotFound, status)
		assert.Equal(t, "transactional_notification not found with ID: nosuchnotification", body["error"])
	})

	t.Run("a segment tree the builder cannot compile answers 400", func(t *testing.T) {
		status, body := post(t, "/api/segments.create", fmt.Sprintf(`{
			"workspace_id": %q, "id": "badtree", "name": "Bad tree", "color": "blue", "timezone": "UTC",
			"tree": {"kind": "leaf", "leaf": {"source": "contacts", "contact": {"filters": [
				{"field_name": "no_such_field", "field_type": "string", "operator": "equals", "string_values": ["x"]}
			]}}}
		}`, workspace.ID))

		assert.Equal(t, http.StatusBadRequest, status, "body: %v", body)
		assert.Contains(t, body["error"], "invalid segment tree")
	})

	t.Run("a publish of a published post answers 409", func(t *testing.T) {
		category, err := factory.CreateBlogCategory(workspace.ID)
		require.NoError(t, err)
		blogPost, err := factory.CreateBlogPost(workspace.ID, category.ID)
		require.NoError(t, err)
		publish := fmt.Sprintf(`{"id":%q}`, blogPost.ID)

		status, body := post(t, "/api/blogPosts.publish", publish)
		require.Equal(t, http.StatusOK, status, "body: %v", body)

		status, body = post(t, "/api/blogPosts.publish", publish)
		assert.Equal(t, http.StatusConflict, status, "body: %v", body)
		assert.Equal(t, map[string]interface{}{
			"entity": "blog_post", "id": blogPost.ID, "action": "publish", "status": "published",
		}, body["invalid_state"])
	})

	t.Run("a webhook node's secret reaches only a key that may write automations", func(t *testing.T) {
		automation, err := factory.CreateAutomation(workspace.ID,
			testutil.WithAutomationRootNodeID("hook"),
			testutil.WithAutomationNodes([]*domain.AutomationNode{{
				ID: "hook", Type: domain.NodeTypeWebhook,
				Config: map[string]interface{}{"url": "https://hooks.example.com/in", "secret": "webhook-bearer-s3cret"},
			}}))
		require.NoError(t, err)

		authService := suite.ServerManager.GetApp().GetAuthService().(*service.AuthService)
		keyClient := func(t *testing.T, permissions domain.UserPermissions) *testutil.APIClient {
			t.Helper()
			key, err := factory.CreateAPIKey(workspace.ID, testutil.WithAPIKeyPermissions(permissions))
			require.NoError(t, err)
			keyClient := testutil.NewAPIClient(suite.ServerManager.GetURL())
			keyClient.SetToken(authService.GenerateAPIAuthToken(key))
			keyClient.SetWorkspaceID(workspace.ID)
			return keyClient
		}
		// readBodies returns the raw bodies of automations.get and automations.list.
		readBodies := func(t *testing.T, c *testutil.APIClient) []string {
			t.Helper()
			var bodies []string
			for _, call := range []func() (*http.Response, error){
				func() (*http.Response, error) { return c.GetAutomation(automation.ID) },
				func() (*http.Response, error) { return c.ListAutomations(nil) },
			} {
				resp, err := call()
				require.NoError(t, err)
				raw, err := io.ReadAll(resp.Body)
				_ = resp.Body.Close()
				require.NoError(t, err)
				require.Equal(t, http.StatusOK, resp.StatusCode, string(raw))
				require.Contains(t, string(raw), "https://hooks.example.com/in", "the node itself is served")
				bodies = append(bodies, string(raw))
			}
			return bodies
		}

		reader := keyClient(t, domain.UserPermissions{
			domain.PermissionResourceAutomations: {Read: true, Write: false},
		})
		for _, body := range readBodies(t, reader) {
			assert.NotContains(t, body, "webhook-bearer-s3cret")
		}

		writer := keyClient(t, domain.NewFullPermissions())
		for _, body := range readBodies(t, writer) {
			assert.Contains(t, body, "webhook-bearer-s3cret")
		}
	})
}

func decodeResponse(t *testing.T, resp *http.Response) (int, map[string]interface{}) {
	t.Helper()
	defer func() { _ = resp.Body.Close() }()
	var body map[string]interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	return resp.StatusCode, body
}
