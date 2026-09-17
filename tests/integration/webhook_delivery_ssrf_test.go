//go:build integration
// +build integration

package integration

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/app"
	"github.com/Notifuse/notifuse/tests/testutil"
)

// TestWebhookDeliverySSRFProtection is the secure-default counterpart to the
// webhook suites that opt out.
//
// Deliberately NO cfg.Webhook.AllowPrivateDeliveryHosts override: this is a
// stock deployment, and the point is that a workspace member holding
// webhook_subscriptions:write cannot aim the server at the private network.
//
// It proves the two layers are wired, which is the part no unit test can reach.
// The save-time layer only exists in app wiring through the flag handed to
// NewWebhookSubscriptionService, and the dial-time layer only exists through the
// client handed to NewWebhookDeliveryWorker. Everything about how a refusal is
// then accounted for — retried, never counted against the subscription — is in
// TestWebhookDeliveryWorker_ssrfRefusalDoesNotRetireTheSubscription, because it
// needs a failure threshold this suite cannot reach in reasonable time.
func TestWebhookDeliverySSRFProtection(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer func() { suite.Cleanup() }()

	client := suite.APIClient
	factory := suite.DataFactory

	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	require.NoError(t, factory.AddUserToWorkspace(user.ID, workspace.ID, "owner"))
	require.NoError(t, client.Login(user.Email, "password"))
	client.SetWorkspaceID(workspace.ID)

	// The workspace owner is the most privileged role a subscription can be
	// created by, so a refusal here is a refusal for everyone.
	t.Run("create refuses a URL that names an internal target", func(t *testing.T) {
		for _, url := range []string{
			"http://127.0.0.1:8080/hook",
			"http://169.254.169.254/latest/meta-data/",
			"http://10.0.0.5/hook",
			"http://localhost:3000/hook",
			"http://svc.default.svc.cluster.local/hook",
		} {
			resp, err := client.Post("/api/webhookSubscriptions.create", map[string]interface{}{
				"workspace_id": workspace.ID,
				"name":         "ssrf attempt",
				"url":          url,
				"event_types":  []string{"contact.created"},
			})
			require.NoError(t, err)
			body, _ := readAllAndClose(resp)
			assert.Equal(t, http.StatusBadRequest, resp.StatusCode,
				"must refuse %s, got %d: %s", url, resp.StatusCode, body)
		}
	})

	// The guard must not cost legitimate receivers anything, including the query
	// string a webhook URL routinely carries its token in.
	t.Run("create still accepts a public receiver with a token query", func(t *testing.T) {
		resp, err := client.Post("/api/webhookSubscriptions.create", map[string]interface{}{
			"workspace_id": workspace.ID,
			"name":         "public receiver",
			"url":          "https://hooks.example.com/notifuse?token=abc123",
			"event_types":  []string{"contact.created"},
		})
		require.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusCreated, resp.StatusCode)
	})

	// A subscription whose private URL predates the guard still exists in the
	// database, and the delivery client is what has to refuse it. Written
	// straight to the row, because the save-time layer above is exactly what
	// stops this being reachable through the API any more.
	t.Run("the delivery client refuses a stored private URL", func(t *testing.T) {
		var reached int32
		receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(&reached, 1)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("SHOULD NEVER BE READ"))
		}))
		defer receiver.Close()

		createResp, err := client.Post("/api/webhookSubscriptions.create", map[string]interface{}{
			"workspace_id": workspace.ID,
			"name":         "predates the guard",
			"url":          "https://hooks.example.com/legacy",
			"event_types":  []string{"contact.created"},
		})
		require.NoError(t, err)
		var created map[string]interface{}
		require.NoError(t, json.NewDecoder(createResp.Body).Decode(&created))
		_ = createResp.Body.Close()
		require.Equal(t, http.StatusCreated, createResp.StatusCode)

		sub, ok := created["subscription"].(map[string]interface{})
		require.True(t, ok, "unexpected create response: %v", created)
		subscriptionID, _ := sub["id"].(string)
		require.NotEmpty(t, subscriptionID)

		workspaceDB, err := suite.DBManager.GetWorkspaceDB(workspace.ID)
		require.NoError(t, err)
		_, err = workspaceDB.Exec(
			`UPDATE webhook_subscriptions SET url = $1 WHERE id = $2`, receiver.URL, subscriptionID)
		require.NoError(t, err)

		testResp, err := client.Post("/api/webhookSubscriptions.test", map[string]interface{}{
			"workspace_id": workspace.ID,
			"id":           subscriptionID,
			"event_type":   "contact.created",
		})
		require.NoError(t, err)
		var result map[string]interface{}
		require.NoError(t, json.NewDecoder(testResp.Body).Decode(&result))
		_ = testResp.Body.Close()

		// The endpoint answers 200 either way — it reports the delivery's outcome
		// in the body rather than in the status — so asserting the status here
		// would pass whether or not the guard exists.
		require.Equal(t, http.StatusOK, testResp.StatusCode)
		assert.Equal(t, false, result["success"], "the guard must refuse a private target: %v", result)
		assert.Contains(t, result["error"], "private or reserved IP address",
			"the refusal must say why: %v", result)
		assert.Empty(t, result["response_body"],
			"nothing from an internal target may be reflected back to the caller")

		// The whole point of the finding: no connection was ever made, so there is
		// no response to leak and no port to probe.
		time.Sleep(200 * time.Millisecond)
		assert.Equal(t, int32(0), atomic.LoadInt32(&reached),
			"the receiver must never have been contacted")
	})
}

func readAllAndClose(resp *http.Response) (string, error) {
	defer resp.Body.Close()
	var sb []byte
	buf := make([]byte, 512)
	n, _ := resp.Body.Read(buf)
	sb = append(sb, buf[:n]...)
	return string(sb), nil
}
