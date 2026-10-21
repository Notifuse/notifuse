package integration

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	httpHandler "github.com/Notifuse/notifuse/internal/http"
	"github.com/Notifuse/notifuse/internal/service"
	"github.com/Notifuse/notifuse/pkg/crypto"
	"github.com/Notifuse/notifuse/tests/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The harness runs with SECRET_KEY and SIGNING_KEY set to different values, as an
// installation that moved off a shared SECRET_KEY does. Everything that signs must
// answer to the signing key alone, while stored data keeps decrypting with SECRET_KEY.
func TestSigningKeySeparation(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer func() { suite.Cleanup() }()

	client := suite.APIClient
	cfg := suite.ServerManager.GetApp().GetConfig()
	require.NotEqual(t, cfg.Security.SecretKey, cfg.Security.SigningKey,
		"the harness must run with distinct keys for these checks to mean anything")

	rootSignin := func(t *testing.T, key string) int {
		t.Helper()
		timestamp := time.Now().Unix()
		signature := crypto.ComputeHMAC256([]byte(fmt.Sprintf("%s:%d", cfg.RootEmail, timestamp)), key)
		resp, err := client.Post("/api/user.rootSignin", domain.RootSigninInput{
			Email:     cfg.RootEmail,
			Timestamp: timestamp,
			Signature: signature,
		})
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()
		return resp.StatusCode
	}

	t.Run("root sign-in verifies with the signing key only", func(t *testing.T) {
		assert.Equal(t, http.StatusUnauthorized, rootSignin(t, cfg.Security.SecretKey))
		assert.Equal(t, http.StatusOK, rootSignin(t, cfg.Security.SigningKey))
	})

	t.Run("task dispatch verifies with the signing key only", func(t *testing.T) {
		request := map[string]interface{}{"workspace_id": "no-such-workspace", "id": "no-such-task"}

		client.SetSigningSecret(cfg.Security.SecretKey)
		defer client.SetSigningSecret(testutil.TestSigningKey)
		resp, err := client.ExecuteTask(request)
		require.NoError(t, err)
		_ = resp.Body.Close()
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)

		client.SetSigningSecret(cfg.Security.SigningKey)
		resp, err = client.ExecuteTask(request)
		require.NoError(t, err)
		_ = resp.Body.Close()
		// Past the signature check: the task simply does not exist.
		assert.NotEqual(t, http.StatusUnauthorized, resp.StatusCode)
	})

	t.Run("usage reads verify with the signing key only", func(t *testing.T) {
		readUsage := func(t *testing.T, key string) int {
			t.Helper()
			const path = "/api/usage.get"
			timestamp := time.Now().Unix()
			req, err := http.NewRequest(http.MethodGet, suite.ServerManager.GetURL()+path, nil)
			require.NoError(t, err)
			req.Header.Set(httpHandler.UsageTimestampHeader, fmt.Sprintf("%d", timestamp))
			req.Header.Set(httpHandler.UsageSignatureHeader,
				domain.SignUsageRequest(domain.UsageSigningKey(key), timestamp, path))
			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()
			return resp.StatusCode
		}

		assert.Equal(t, http.StatusUnauthorized, readUsage(t, cfg.Security.SecretKey))
		assert.Equal(t, http.StatusOK, readUsage(t, cfg.Security.SigningKey))
	})

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

	t.Run("an API key token authenticates only an API key account", func(t *testing.T) {
		apiUser, err := factory.CreateAPIKey(workspace.ID)
		require.NoError(t, err)
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

		accept := func(t *testing.T, token string) int {
			t.Helper()
			resp, err := client.Post("/api/workspaces.acceptInvitation", map[string]string{"token": token})
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()
			return resp.StatusCode
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
