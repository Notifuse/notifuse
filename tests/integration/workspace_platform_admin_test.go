package integration

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/tests/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWorkspacePlatformAdminOverrideSuite verifies the ROOT_EMAIL "god-mode" override
// end-to-end through the real HTTP + Postgres stack, covering real usage, misuse, and
// revocation.
//
// The scenario mirrors production: ROOT_EMAIL holds two operators. The first creates a
// workspace; the second — a DISTINCT identity who never created or joined it — must get full
// owner access purely from being a configured root. A non-owner member and a non-root stranger
// must stay gated, and dropping the second operator from ROOT_EMAIL must revoke access at once.
//
// suite.Config is the live *config.Config the AuthService IsRootEmail closure reads, so we can
// add/remove a second root at runtime (equivalent to a multi-value ROOT_EMAIL) without touching
// any shared test fixture. Each suite owns its app, so this does not affect other tests.
func TestWorkspacePlatformAdminOverrideSuite(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer suite.Cleanup()

	client := suite.APIClient
	db := suite.DBManager.GetDB()
	tokenCache := testutil.NewTokenCache(client)

	const (
		firstRootEmail  = "test@example.com"             // primary root (matches the seeded config)
		secondRootEmail = "workspace-viewer@example.com" // a DISTINCT seeded identity, made root below
		memberEmail     = "workspace-member@example.com" // a real non-owner member
		strangerEmail   = "non-member@example.com"       // non-root, non-member
	)

	firstRootToken := tokenCache.GetOrCreate(t, firstRootEmail)

	// First operator creates the workspace (becomes the creator/owner with a real row).
	workspaceID := createTestWorkspaceWithToken(t, client, firstRootToken, "Platform Admin Override WS")

	// Add a real non-owner member to the workspace (used by the misuse checks).
	// It holds workspace:read and nothing else: workspaces.get is gated on that
	// permission, so a zero-permission row would be denied the read this suite
	// expects a member to have, and the owner-action checks below would pass for
	// the wrong reason.
	var memberID string
	require.NoError(t, db.QueryRow(`SELECT id FROM users WHERE email = $1`, memberEmail).Scan(&memberID))
	_, err := db.Exec(`
		INSERT INTO user_workspaces (user_id, workspace_id, role, permissions, created_at, updated_at)
		VALUES ($1, $2, 'member', '{"workspace": {"read": true, "write": false}}'::jsonb, NOW(), NOW())
		ON CONFLICT (user_id, workspace_id) DO UPDATE SET role = 'member', permissions = EXCLUDED.permissions`,
		memberID, workspaceID)
	require.NoError(t, err)

	// Promote a SECOND, distinct identity to platform admin at runtime — i.e. as if
	// ROOT_EMAIL="test@example.com,workspace-viewer@example.com". The second root holds NO
	// membership row for this workspace, so all of its access must come from the override.
	originalRoot := suite.Config.RootEmail
	suite.Config.RootEmail = originalRoot + "," + secondRootEmail
	defer func() { suite.Config.RootEmail = originalRoot }()
	secondRootToken := tokenCache.GetOrCreate(t, secondRootEmail)

	// helper: assert a request was denied. Authorization denials map to 403 Forbidden.
	assertDenied := func(t *testing.T, resp *http.Response) {
		t.Helper()
		assert.Equal(t, http.StatusForbidden, resp.StatusCode, "expected the request to be denied with 403")
	}
	listContainsWorkspace := func(t *testing.T, resp *http.Response, wsID string) bool {
		t.Helper()
		var workspaces []struct {
			ID string `json:"id"`
		}
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&workspaces))
		for _, w := range workspaces {
			if w.ID == wsID {
				return true
			}
		}
		return false
	}

	// ========================= REAL USAGE: the second platform admin =========================
	t.Run("second platform admin (never joined) has full owner access", func(t *testing.T) {
		client.SetToken(secondRootToken)

		t.Run("can GET the workspace", func(t *testing.T) {
			resp, err := client.Get("/api/workspaces.get", map[string]string{"id": workspaceID})
			require.NoError(t, err)
			defer resp.Body.Close()
			assert.Equal(t, http.StatusOK, resp.StatusCode)
		})

		t.Run("sees it in the workspace list (root List branch)", func(t *testing.T) {
			resp, err := client.Get("/api/workspaces.list")
			require.NoError(t, err)
			defer resp.Body.Close()
			require.Equal(t, http.StatusOK, resp.StatusCode)
			assert.True(t, listContainsWorkspace(t, resp, workspaceID),
				"a platform admin should see a workspace it never joined")
		})

		t.Run("can perform an owner action (update)", func(t *testing.T) {
			resp, err := client.Post("/api/workspaces.update", domain.UpdateWorkspaceRequest{
				ID:   workspaceID,
				Name: "Renamed By Second Platform Admin",
				Settings: domain.WorkspaceSettings{
					Timezone:        "UTC",
					DefaultLanguage: "en",
					Languages:       []string{"en"},
				},
			})
			require.NoError(t, err)
			defer resp.Body.Close()
			assert.Equal(t, http.StatusOK, resp.StatusCode)
		})

		t.Run("is surfaced as a virtual owner in the members list", func(t *testing.T) {
			resp, err := client.Get("/api/workspaces.members", map[string]string{"id": workspaceID})
			require.NoError(t, err)
			defer resp.Body.Close()
			require.Equal(t, http.StatusOK, resp.StatusCode)

			var response map[string]interface{}
			require.NoError(t, json.NewDecoder(resp.Body).Decode(&response))
			members, ok := response["members"].([]interface{})
			require.True(t, ok)

			found := false
			for _, m := range members {
				mm := m.(map[string]interface{})
				if mm["email"] == secondRootEmail && mm["role"] == "owner" {
					found = true
				}
			}
			assert.True(t, found, "the second platform admin should appear as a virtual owner")
		})
	})

	// ========================= MISUSE: a real non-owner member =========================
	t.Run("non-owner member cannot perform owner actions", func(t *testing.T) {
		memberToken := tokenCache.GetOrCreate(t, memberEmail)
		client.SetToken(memberToken)

		t.Run("member can read the workspace", func(t *testing.T) {
			resp, err := client.Get("/api/workspaces.get", map[string]string{"id": workspaceID})
			require.NoError(t, err)
			defer resp.Body.Close()
			assert.Equal(t, http.StatusOK, resp.StatusCode)
		})

		t.Run("member is denied an owner action (update)", func(t *testing.T) {
			resp, err := client.Post("/api/workspaces.update", domain.UpdateWorkspaceRequest{
				ID:   workspaceID,
				Name: "Member Should Not Rename",
				Settings: domain.WorkspaceSettings{
					Timezone:        "UTC",
					DefaultLanguage: "en",
					Languages:       []string{"en"},
				},
			})
			require.NoError(t, err)
			defer resp.Body.Close()
			assertDenied(t, resp)
		})
	})

	// ========================= MISUSE: a non-root non-member =========================
	t.Run("non-root non-member is denied", func(t *testing.T) {
		strangerToken := tokenCache.GetOrCreate(t, strangerEmail)
		client.SetToken(strangerToken)

		t.Run("denied GET", func(t *testing.T) {
			resp, err := client.Get("/api/workspaces.get", map[string]string{"id": workspaceID})
			require.NoError(t, err)
			defer resp.Body.Close()
			assertDenied(t, resp)
		})

		t.Run("does not see it in the list", func(t *testing.T) {
			resp, err := client.Get("/api/workspaces.list")
			require.NoError(t, err)
			defer resp.Body.Close()
			require.Equal(t, http.StatusOK, resp.StatusCode)
			assert.False(t, listContainsWorkspace(t, resp, workspaceID),
				"a non-root non-member must not see the workspace")
		})
	})

	// ========================= REVOCATION: drop the second root from ROOT_EMAIL =========================
	t.Run("removing an email from ROOT_EMAIL revokes access immediately", func(t *testing.T) {
		suite.Config.RootEmail = originalRoot // second root is no longer configured
		client.SetToken(secondRootToken)

		t.Run("former root can no longer GET the workspace", func(t *testing.T) {
			resp, err := client.Get("/api/workspaces.get", map[string]string{"id": workspaceID})
			require.NoError(t, err)
			defer resp.Body.Close()
			assertDenied(t, resp)
		})

		t.Run("former root no longer sees it in the list", func(t *testing.T) {
			resp, err := client.Get("/api/workspaces.list")
			require.NoError(t, err)
			defer resp.Body.Close()
			require.Equal(t, http.StatusOK, resp.StatusCode)
			assert.False(t, listContainsWorkspace(t, resp, workspaceID),
				"a de-rooted user must lose access to workspaces it never joined")
		})
	})
}
