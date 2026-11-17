package integration

import (
	"encoding/json"
	"net/http"
	"testing"

	appconfig "github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/tests/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestUserCheckForUpdates checks, through the real HTTP and Postgres stack, which consoles
// user.me tells to ask updates.notifuse.com for the releases they are missing: the root user's
// and a workspace owner's, never a mere member's, and nobody's once CHECK_FOR_UPDATES is off.
func TestUserCheckForUpdates(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer suite.Cleanup()

	client := suite.APIClient
	db := suite.DBManager.GetDB()
	tokens := testutil.NewTokenCache(client)

	const (
		rootEmail   = "test@example.com"
		ownerEmail  = "workspace-owner@example.com"
		memberEmail = "workspace-member@example.com"
	)

	rootToken := tokens.GetOrCreate(t, rootEmail)
	workspaceID := createTestWorkspaceWithToken(t, client, rootToken, "Check For Updates WS")

	addRow := func(email, role string) {
		var userID string
		require.NoError(t, db.QueryRow(`SELECT id FROM users WHERE email = $1`, email).Scan(&userID))
		_, err := db.Exec(`
			INSERT INTO user_workspaces (user_id, workspace_id, role, permissions, created_at, updated_at)
			VALUES ($1, $2, $3, '{"workspace": {"read": true, "write": false}}'::jsonb, NOW(), NOW())
			ON CONFLICT (user_id, workspace_id) DO UPDATE SET role = EXCLUDED.role`,
			userID, workspaceID, role)
		require.NoError(t, err)
	}
	addRow(ownerEmail, "owner")
	addRow(memberEmail, "member")

	me := func(t *testing.T, email string) map[string]interface{} {
		t.Helper()
		req, err := http.NewRequest(http.MethodGet, suite.ServerManager.GetURL()+"/api/user.me", nil)
		require.NoError(t, err)
		req.Header.Set("Authorization", "Bearer "+tokens.GetOrCreate(t, email))
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()
		require.Equal(t, http.StatusOK, resp.StatusCode)
		var body map[string]interface{}
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
		return body
	}

	// suite.Config is the live *config.Config the handler reads.
	original := suite.Config.CheckForUpdates
	defer func() { suite.Config.CheckForUpdates = original }()

	t.Run("with CHECK_FOR_UPDATES on", func(t *testing.T) {
		suite.Config.CheckForUpdates = true

		assert.Equal(t, true, me(t, rootEmail)["check_for_updates"], "root")
		assert.Equal(t, true, me(t, ownerEmail)["check_for_updates"], "owner")
		assert.Equal(t, false, me(t, memberEmail)["check_for_updates"], "member")
		assert.Equal(t, appconfig.VERSION, me(t, ownerEmail)["version"])
	})

	t.Run("with CHECK_FOR_UPDATES off", func(t *testing.T) {
		suite.Config.CheckForUpdates = false

		assert.Equal(t, false, me(t, rootEmail)["check_for_updates"], "root")
		assert.Equal(t, false, me(t, ownerEmail)["check_for_updates"], "owner")
	})
}
