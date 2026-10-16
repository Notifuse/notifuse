//go:build integration
// +build integration

package integration

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	shortuuid "github.com/lithammer/shortuuid/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/app"
	"github.com/Notifuse/notifuse/tests/testutil"
)

// TestAutomationWebhookNodeSSRFProtection is the secure-default counterpart to
// TestAutomation, which opts out so its webhook node can reach a loopback receiver.
//
// Deliberately NO cfg.Webhook.AllowPrivateDeliveryHosts override: a member holding
// automations:write must not be able to aim a webhook node at the deployment's own
// network and read the response back through automations.nodeExecutions.
func TestAutomationWebhookNodeSSRFProtection(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	require.NoError(t, suite.ServerManager.StartBackgroundWorkers(context.Background()))

	factory := suite.DataFactory
	client := suite.APIClient

	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	require.NoError(t, factory.AddUserToWorkspace(user.ID, workspace.ID, "owner"))
	_, err = factory.SetupWorkspaceWithSMTPProvider(workspace.ID)
	require.NoError(t, err)
	require.NoError(t, client.Login(user.Email, "password"))
	client.SetWorkspaceID(workspace.ID)

	// trigger (custom event) → webhook. The webhook node is second so the
	// stored-URL subtest can address it as nodes[1].
	automationBody := func(automationID, webhookNodeID, eventName, url string) map[string]interface{} {
		triggerNodeID := shortuuid.New()
		return map[string]interface{}{
			"workspace_id": workspace.ID,
			"automation": map[string]interface{}{
				"id":           automationID,
				"workspace_id": workspace.ID,
				"name":         "SSRF " + automationID,
				"status":       "draft",
				"trigger": map[string]interface{}{
					"event_kind":        "custom_event",
					"custom_event_name": eventName,
					"frequency":         "once",
				},
				"root_node_id": triggerNodeID,
				"nodes": []map[string]interface{}{
					{
						"id":            triggerNodeID,
						"automation_id": automationID,
						"type":          "trigger",
						"config":        map[string]interface{}{},
						"next_node_id":  webhookNodeID,
						"position":      map[string]interface{}{"x": 0, "y": 0},
					},
					{
						"id":            webhookNodeID,
						"automation_id": automationID,
						"type":          "webhook",
						"config":        map[string]interface{}{"url": url},
						"position":      map[string]interface{}{"x": 0, "y": 100},
					},
				},
			},
		}
	}

	readBody := func(resp *http.Response) string {
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return string(b)
	}

	t.Run("create refuses a webhook node aimed at an internal target", func(t *testing.T) {
		for _, url := range []string{
			"http://127.0.0.1:8080/hook",
			"http://[::1]:8080/hook",
			"http://169.254.169.254/latest/meta-data/",
			"http://10.0.0.5/hook",
			"http://localhost:3000/hook",
			"http://svc.default.svc.cluster.local/hook",
		} {
			resp, err := client.CreateAutomation(automationBody(shortuuid.New(), shortuuid.New(), "ssrf_create", url))
			require.NoError(t, err)
			body := readBody(resp)
			assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "must refuse %s, got %d: %s", url, resp.StatusCode, body)
			assert.Contains(t, body, "WEBHOOK_DELIVERY_ALLOW_PRIVATE_HOSTS", "the refusal must say how to opt out: %s", body)
		}
	})

	t.Run("update refuses switching a node to an internal target", func(t *testing.T) {
		automationID, webhookNodeID := shortuuid.New(), shortuuid.New()
		resp, err := client.CreateAutomation(automationBody(automationID, webhookNodeID, "ssrf_update", "https://hooks.example.com/notifuse?token=abc123"))
		require.NoError(t, err)
		require.Equal(t, http.StatusCreated, resp.StatusCode, readBody(resp))

		resp, err = client.UpdateAutomation(automationBody(automationID, webhookNodeID, "ssrf_update", "http://10.0.0.5/hook"))
		require.NoError(t, err)
		body := readBody(resp)
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "got %d: %s", resp.StatusCode, body)
		assert.Contains(t, body, "WEBHOOK_DELIVERY_ALLOW_PRIVATE_HOSTS", body)
	})

	t.Run("a public receiver and an unconfigured draft are still saved", func(t *testing.T) {
		for _, url := range []string{"https://hooks.example.com/notifuse?token=abc123", ""} {
			resp, err := client.CreateAutomation(automationBody(shortuuid.New(), shortuuid.New(), "ssrf_public", url))
			require.NoError(t, err)
			body := readBody(resp)
			assert.Equal(t, http.StatusCreated, resp.StatusCode, "must accept %q, got %d: %s", url, resp.StatusCode, body)
		}
	})

	t.Run("the executor refuses a stored private URL", func(t *testing.T) {
		var hits int32
		receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(&hits, 1)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"leak":"SHOULD NEVER BE READ"}`))
		}))
		defer receiver.Close()

		automationID, webhookNodeID := shortuuid.New(), shortuuid.New()
		const eventName = "ssrf_stored_private_url"
		resp, err := client.CreateAutomation(automationBody(automationID, webhookNodeID, eventName, "https://hooks.example.com/legacy"))
		require.NoError(t, err)
		require.Equal(t, http.StatusCreated, resp.StatusCode, readBody(resp))

		// As if the URL predated the guard: the save-time layer is what makes this
		// unreachable through the API, so it is written straight to the row.
		workspaceDB, err := suite.DBManager.GetWorkspaceDB(workspace.ID)
		require.NoError(t, err)
		_, err = workspaceDB.Exec(
			`UPDATE automations SET nodes = jsonb_set(nodes, '{1,config,url}', to_jsonb($1::text)) WHERE id = $2`,
			receiver.URL, automationID)
		require.NoError(t, err)

		getResp, err := client.GetAutomation(automationID)
		require.NoError(t, err)
		require.Contains(t, readBody(getResp), receiver.URL, "the stored URL must have been flipped")

		activateResp, err := client.ActivateAutomation(map[string]interface{}{
			"workspace_id":  workspace.ID,
			"automation_id": automationID,
		})
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, activateResp.StatusCode, readBody(activateResp))

		email := "ssrf-stored-" + shortuuid.New() + "@example.com"
		_, err = factory.CreateContact(workspace.ID, testutil.WithContactEmail(email))
		require.NoError(t, err)
		require.NoError(t, factory.CreateCustomEvent(workspace.ID, email, eventName, nil))

		// Polled by hand rather than with testutil.WaitForCondition so that a run
		// without the guard fails with the evidence — the receiver's hit count and
		// what the node stored — instead of a bare timeout.
		var refused, contactAutomation map[string]interface{}
		var lastSeen []map[string]interface{}
		deadline := time.Now().Add(15 * time.Second)
		for refused == nil && time.Now().Before(deadline) {
			time.Sleep(200 * time.Millisecond)
			resp, err := client.GetContactNodeExecutions(automationID, email)
			if err != nil {
				continue
			}
			var result struct {
				ContactAutomation map[string]interface{}   `json:"contact_automation"`
				NodeExecutions    []map[string]interface{} `json:"node_executions"`
			}
			decodeErr := json.NewDecoder(resp.Body).Decode(&result)
			resp.Body.Close()
			if resp.StatusCode != http.StatusOK || decodeErr != nil {
				continue
			}
			lastSeen = result.NodeExecutions
			for _, exec := range result.NodeExecutions {
				// handleError adds a second failed row under the same node_id with the
				// placeholder type "trigger"; the webhook node's own row is the one to read.
				if exec["node_id"] == webhookNodeID && exec["node_type"] == "webhook" && exec["action"] == "failed" {
					refused, contactAutomation = exec, result.ContactAutomation
				}
			}
		}
		require.NotNil(t, refused,
			"no refused webhook execution after 15s; receiver hits=%d; node executions seen: %v",
			atomic.LoadInt32(&hits), lastSeen)

		assert.Contains(t, refused["error"], "private or reserved")
		assert.Contains(t, refused["error"], "WEBHOOK_DELIVERY_ALLOW_PRIVATE_HOSTS")
		if output, ok := refused["output"].(map[string]interface{}); ok {
			assert.NotContains(t, output, "response", "nothing from an internal target may be stored")
		}
		assert.Contains(t, contactAutomation["last_error"], "WEBHOOK_DELIVERY_ALLOW_PRIVATE_HOSTS")

		time.Sleep(200 * time.Millisecond)
		assert.Equal(t, int32(0), atomic.LoadInt32(&hits), "the receiver must never have been contacted")
	})
}
