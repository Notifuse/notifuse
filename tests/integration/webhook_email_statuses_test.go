package integration

import (
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/app"
	"github.com/Notifuse/notifuse/tests/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWebhookEmailStatuses: /webhooks/email answers 404 for a workspace or an integration that
// does not exist and 400 for a body the provider's parser cannot read, so the provider stops
// sending it. The 500 for a failure on our side is covered by the service and handler tests.
func TestWebhookEmailStatuses(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	client := suite.APIClient
	factory := suite.DataFactory
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	ses, err := factory.CreateSESIntegration(workspace.ID)
	require.NoError(t, err)

	post := func(t *testing.T, workspaceID, integrationID, body string) (int, string) {
		t.Helper()
		resp, err := client.PostRaw(fmt.Sprintf("/webhooks/email?provider=ses&workspace_id=%s&integration_id=%s", workspaceID, integrationID), body)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()
		text, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(text)
	}

	t.Run("an unknown integration", func(t *testing.T) {
		status, body := post(t, workspace.ID, "does-not-exist", `{"Type":"Notification","Message":"{}"}`)
		assert.Equal(t, http.StatusNotFound, status, body)
	})

	t.Run("an unknown workspace", func(t *testing.T) {
		status, body := post(t, "nows"+workspace.ID[:6], ses.ID, `{"Type":"Notification","Message":"{}"}`)
		assert.Equal(t, http.StatusNotFound, status, body)
	})

	t.Run("a garbled body", func(t *testing.T) {
		status, body := post(t, workspace.ID, ses.ID, `{not json`)
		assert.Equal(t, http.StatusBadRequest, status, body)
		assert.Contains(t, body, "Invalid webhook payload")
	})
}
