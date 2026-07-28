package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/app"
	"github.com/Notifuse/notifuse/tests/testutil"
)

// TestSegmentBroadcastLinkPreview exercises the broadcast/link click condition through the
// FULL segment service (the /api/segments.preview endpoint: auth -> validation -> SQL build
// -> execution against the workspace DB), not just BuildSQL in isolation. It also proves the
// link match is a LITERAL substring (strpos), not an ILIKE pattern: a "_" search must match
// nothing rather than every URL.
func TestSegmentBroadcastLinkPreview(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	factory := suite.DataFactory
	client := suite.APIClient

	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	workspaceID := workspace.ID

	user, err := factory.CreateUser()
	require.NoError(t, err)
	require.NoError(t, factory.AddUserToWorkspace(user.ID, workspaceID, "owner"))
	require.NoError(t, client.Login(user.Email, "password"))
	client.SetWorkspaceID(workspaceID)

	template, err := factory.CreateTemplate(workspaceID)
	require.NoError(t, err)

	workspaceDB, err := factory.GetWorkspaceDB(workspaceID)
	require.NoError(t, err)
	ctx := context.Background()

	seedClick := func(email, broadcastID, clickedURL string) {
		_, err := factory.CreateContact(workspaceID, testutil.WithContactEmail(email))
		require.NoError(t, err)
		msg, err := factory.CreateMessageHistory(workspaceID,
			testutil.WithMessageHistoryContactEmail(email),
			testutil.WithMessageHistoryTemplateID(template.ID))
		require.NoError(t, err)
		_, err = workspaceDB.ExecContext(ctx,
			`UPDATE message_history SET broadcast_id = $1,
			     clicked_links = jsonb_build_object($2::text, jsonb_build_object('count', 1))
			 WHERE id = $3`,
			broadcastID, clickedURL, msg.ID)
		require.NoError(t, err)
		require.NoError(t, factory.CreateContactTimelineEvent(workspaceID, email, "email.clicked",
			map[string]interface{}{"entity_id": msg.ID}))
	}

	// No seeded URL contains a literal '_' (hyphen, not underscore), so a bare "_" search
	// must match nothing under literal (strpos) matching.
	seedClick("buyer@prev.com", "summer-sale", "https://acme.com/pricing?utm-campaign=summer")
	seedClick("browser@prev.com", "summer-sale", "https://acme.com/home")
	seedClick("winter@prev.com", "winter-sale", "https://acme.com/pricing")

	// previewCount posts a email.clicked timeline condition (optionally scoped) and returns the
	// matched-contact count reported by the real segment service.
	previewCount := func(t *testing.T, tl map[string]interface{}) int {
		tl["kind"] = "email.clicked"
		if _, ok := tl["count_operator"]; !ok {
			tl["count_operator"] = "at_least"
		}
		if _, ok := tl["count_value"]; !ok {
			tl["count_value"] = 1
		}
		resp, err := client.Post("/api/segments.preview", map[string]interface{}{
			"workspace_id": workspaceID,
			"tree": map[string]interface{}{
				"kind": "leaf",
				"leaf": map[string]interface{}{"source": "contact_timeline", "contact_timeline": tl},
			},
			"limit": 100,
		})
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()
		require.Equal(t, http.StatusOK, resp.StatusCode, "preview must succeed")

		var result struct {
			TotalCount int `json:"total_count"`
		}
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
		return result.TotalCount
	}

	t.Run("broadcast scope via the real service", func(t *testing.T) {
		require.Equal(t, 2, previewCount(t, map[string]interface{}{"broadcast_id": "summer-sale"}),
			"buyer + browser clicked in summer-sale")
	})

	t.Run("broadcast + link narrows through the full stack", func(t *testing.T) {
		require.Equal(t, 1, previewCount(t, map[string]interface{}{
			"broadcast_id": "summer-sale", "link_url": "/pricing"}),
			"only buyer clicked /pricing in summer-sale (winter is a different broadcast)")
	})

	t.Run("link match is literal, not an ILIKE wildcard", func(t *testing.T) {
		// A bare underscore is an ILIKE 'any-char' wildcard; as a literal substring it
		// appears in none of the seeded URLs. strpos must return 0, not all-3.
		require.Equal(t, 0, previewCount(t, map[string]interface{}{"link_url": "_"}),
			"'_' must be matched literally (0), not as a wildcard (would be 3 under ILIKE)")
	})

	t.Run("exactly-0 scoped to a broadcast finds contacts who did not click there", func(t *testing.T) {
		// buyer + browser clicked in summer-sale; only winter did not. COUNT-based, so
		// "exactly 0 clicks scoped to summer-sale" matches winter.
		require.Equal(t, 1, previewCount(t, map[string]interface{}{
			"broadcast_id": "summer-sale", "count_operator": "exactly", "count_value": 0}),
			"only winter has zero clicks in summer-sale")
	})
}
