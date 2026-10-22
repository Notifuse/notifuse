package integration

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/service"
	"github.com/Notifuse/notifuse/tests/testutil"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Tokens authenticate only what they were issued for, invitations only their own
// invitee, and the workspace's encrypted secret key is served to no one.
func TestAuthHardening(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer func() { suite.Cleanup() }()

	client := suite.APIClient
	cfg := suite.ServerManager.GetApp().GetConfig()

	authService := suite.ServerManager.GetApp().GetAuthService().(*service.AuthService)
	factory := suite.DataFactory

	owner, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	require.NoError(t, factory.AddUserToWorkspace(owner.ID, workspace.ID, "owner"))

	listLists := func(t *testing.T, token string) int {
		t.Helper()
		apiClient := testutil.NewAPIClient(suite.ServerManager.GetURL())
		apiClient.SetToken(token)
		resp, err := apiClient.Get("/api/lists.list", map[string]string{"workspace_id": workspace.ID})
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()
		return resp.StatusCode
	}

	apiUser, err := factory.CreateAPIKey(workspace.ID)
	require.NoError(t, err)

	t.Run("an API key token authenticates only an API key account", func(t *testing.T) {
		assert.Equal(t, http.StatusOK, listLists(t, authService.GenerateAPIAuthToken(apiUser)))

		// Same signature, same claims shape, but naming the workspace owner.
		assert.Equal(t, http.StatusUnauthorized, listLists(t, authService.GenerateAPIAuthToken(owner)))
	})

	t.Run("an invitation token must match its stored invitation", func(t *testing.T) {
		rootToken := performCompleteSignInFlow(t, client, cfg.RootEmail)
		workspaceID := createTestWorkspaceWithToken(t, client, rootToken, "Invitation Binding WS")

		inviter := testutil.NewAPIClient(suite.ServerManager.GetURL())
		inviter.SetToken(rootToken)
		var invited struct {
			Token      string                     `json:"token"`
			Invitation domain.WorkspaceInvitation `json:"invitation"`
		}
		require.NoError(t, inviter.PostJSON("/api/workspaces.inviteMember", domain.InviteMemberRequest{
			WorkspaceID: workspaceID,
			Email:       "invitee-binding@example.com",
			Permissions: domain.NewFullPermissions(),
		}, &invited))
		require.NotEmpty(t, invited.Token)

		// The workspace's encrypted secret key reaches neither the invitee, who may
		// never join, nor a member.
		var verified struct {
			Workspace struct {
				Settings map[string]interface{} `json:"settings"`
			} `json:"workspace"`
		}
		require.NoError(t, client.PostJSON("/api/workspaces.verifyInvitationToken",
			map[string]string{"token": invited.Token}, &verified))
		require.NotEmpty(t, verified.Workspace.Settings)
		assert.NotContains(t, verified.Workspace.Settings, "encrypted_secret_key")
		var got struct {
			Workspace struct {
				Settings map[string]interface{} `json:"settings"`
			} `json:"workspace"`
		}
		require.NoError(t, inviter.GetJSON("/api/workspaces.get", &got, map[string]string{"id": workspaceID}))
		require.NotEmpty(t, got.Workspace.Settings)
		assert.NotContains(t, got.Workspace.Settings, "encrypted_secret_key")

		// The address of another workspace's API key is not a person to invite.
		resp, err := inviter.Post("/api/workspaces.inviteMember", domain.InviteMemberRequest{
			WorkspaceID: workspaceID,
			Email:       apiUser.Email,
			Permissions: domain.NewFullPermissions(),
		})
		require.NoError(t, err)
		_ = resp.Body.Close()
		assert.Equal(t, http.StatusForbidden, resp.StatusCode)
		var keyMemberships int
		require.NoError(t, suite.DBManager.GetDB().QueryRow(
			`SELECT count(*) FROM user_workspaces WHERE user_id = $1 AND workspace_id = $2`, apiUser.ID, workspaceID).Scan(&keyMemberships))
		assert.Equal(t, 0, keyMemberships, "the key was not seated")

		accept := func(t *testing.T, token string) int {
			t.Helper()
			resp, err := client.Post("/api/workspaces.acceptInvitation", map[string]string{"token": token})
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()
			return resp.StatusCode
		}

		// A link that no longer opens answers 404, not 401, which would sign out a
		// member who opens it: one that expired, one signed with another key, and a
		// token that is not an invitation's at all.
		verify := func(t *testing.T, token string) int {
			t.Helper()
			resp, err := client.Post("/api/workspaces.verifyInvitationToken", map[string]string{"token": token})
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()
			return resp.StatusCode
		}
		expired := invited.Invitation
		expired.ExpiresAt = time.Now().Add(-time.Hour)
		signedWithAnotherKey, err := jwt.NewWithClaims(jwt.SigningMethodHS256, service.InvitationClaims{
			InvitationID: invited.Invitation.ID,
			WorkspaceID:  invited.Invitation.WorkspaceID,
			Email:        invited.Invitation.Email,
			RegisteredClaims: jwt.RegisteredClaims{
				ExpiresAt: jwt.NewNumericDate(invited.Invitation.ExpiresAt),
				IssuedAt:  jwt.NewNumericDate(time.Now()),
			},
		}).SignedString([]byte("another-installations-secret-key-32-bytes"))
		require.NoError(t, err)
		for name, token := range map[string]string{
			"expired":                 authService.GenerateInvitationToken(&expired),
			"signed with another key": signedWithAnotherKey,
			"a session token":         rootToken,
		} {
			assert.Equal(t, http.StatusNotFound, verify(t, token), name)
			assert.Equal(t, http.StatusNotFound, accept(t, token), name)
		}

		// A validly signed token for the same invitation, but naming an existing account.
		otherEmail := invited.Invitation
		otherEmail.Email = owner.Email
		assert.Equal(t, http.StatusNotFound, accept(t, authService.GenerateInvitationToken(&otherEmail)))

		otherWorkspace := invited.Invitation
		otherWorkspace.WorkspaceID = workspace.ID
		assert.Equal(t, http.StatusNotFound, accept(t, authService.GenerateInvitationToken(&otherWorkspace)))

		db := suite.DBManager.GetDB()
		var ownerMemberships, inviteeAccounts int
		require.NoError(t, db.QueryRow(`SELECT count(*) FROM user_workspaces WHERE user_id = $1`, owner.ID).Scan(&ownerMemberships))
		require.NoError(t, db.QueryRow(`SELECT count(*) FROM users WHERE lower(email) = $1`, "invitee-binding@example.com").Scan(&inviteeAccounts))
		assert.Equal(t, 1, ownerMemberships, "the refused tokens seated nobody")
		assert.Equal(t, 0, inviteeAccounts, "the refused tokens created no account")

		assert.Equal(t, http.StatusOK, accept(t, invited.Token))
		// Accepted: the link has nothing left to open.
		assert.Equal(t, http.StatusNotFound, accept(t, invited.Token))
	})

	// Runs last: signing root out ends every root session.
	t.Run("root-only routes answer a live root session only", func(t *testing.T) {
		rootToken := performCompleteSignInFlow(t, client, cfg.RootEmail)
		settingsGet := func(t *testing.T, token string) int {
			t.Helper()
			apiClient := testutil.NewAPIClient(suite.ServerManager.GetURL())
			apiClient.SetToken(token)
			resp, err := apiClient.Get("/api/settings.get")
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()
			return resp.StatusCode
		}
		assert.Equal(t, http.StatusOK, settingsGet(t, rootToken))

		root, err := suite.ServerManager.GetApp().GetUserRepository().GetUserByEmail(context.Background(), cfg.RootEmail)
		require.NoError(t, err)
		assert.Equal(t, http.StatusForbidden, settingsGet(t, authService.GenerateAPIAuthToken(root)),
			"an API key token naming root is not root's session")

		signedIn := testutil.NewAPIClient(suite.ServerManager.GetURL())
		signedIn.SetToken(rootToken)
		resp, err := signedIn.Post("/api/user.logout", nil)
		require.NoError(t, err)
		_ = resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, http.StatusUnauthorized, settingsGet(t, rootToken), "a signed-out session is refused")
	})
}
