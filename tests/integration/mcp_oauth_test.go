//go:build integration
// +build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/app"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/service"
	"github.com/Notifuse/notifuse/tests/testutil"
)

// The client of these tests: Claude's metadata document, served by the
// injected fetcher instead of the network.
const (
	oauthClientID    = "https://claude.ai/oauth/mcp-oauth-client-metadata"
	oauthRedirectURI = "https://claude.ai/api/mcp/auth_callback"
	oauthVerifier    = "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk-integration"
)

// oauthDocuments serves Client ID Metadata Documents from memory.
type oauthDocuments map[string]string

func (d oauthDocuments) Fetch(ctx context.Context, clientID string) (*service.OAuthClientDocument, error) {
	body, ok := d[clientID]
	if !ok {
		return nil, fmt.Errorf("no document at %s", clientID)
	}
	return &service.OAuthClientDocument{Body: []byte(body), Header: http.Header{"Cache-Control": {"max-age=3600"}}}, nil
}

var oauthClaudeDocument = oauthDocuments{
	oauthClientID: `{"client_id":"` + oauthClientID + `","client_name":"Claude","redirect_uris":["` + oauthRedirectURI + `"],` +
		`"grant_types":["authorization_code","refresh_token"],"response_types":["code"],"token_endpoint_auth_method":"none"}`,
}

// oauthHTTP speaks to the server as a browser and an OAuth client do: no
// redirect followed, cookies passed by hand, raw bodies.
type oauthHTTP struct {
	t   *testing.T
	url string
}

type oauthResponse struct {
	status int
	header http.Header
	body   string
}

func (r oauthResponse) json(t *testing.T) map[string]interface{} {
	t.Helper()
	var out map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(r.body), &out), r.body)
	return out
}

func (c oauthHTTP) do(method, path, contentType, body string, headers map[string]string) oauthResponse {
	c.t.Helper()
	req, err := http.NewRequest(method, c.url+path, strings.NewReader(body))
	require.NoError(c.t, err)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	for name, value := range headers {
		if name == "Host" {
			req.Host = value
			continue
		}
		req.Header.Set(name, value)
	}
	client := &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	require.NoError(c.t, err)
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(resp.Body)
	require.NoError(c.t, err)
	return oauthResponse{status: resp.StatusCode, header: resp.Header, body: string(data)}
}

func (c oauthHTTP) get(path string, headers map[string]string) oauthResponse {
	return c.do(http.MethodGet, path, "", "", headers)
}

func (c oauthHTTP) form(path string, values url.Values) oauthResponse {
	return c.do(http.MethodPost, path, "application/x-www-form-urlencoded", values.Encode(), nil)
}

func (c oauthHTTP) call(path, body string, headers map[string]string) oauthResponse {
	return c.do(http.MethodPost, path, "application/json", body, headers)
}

// authorize starts an authorization request and returns its ID and the
// Cookie header that binds it to this "browser".
func (c oauthHTTP) authorize(state string) (requestID, cookie string) {
	c.t.Helper()
	query := url.Values{
		"response_type":         {"code"},
		"client_id":             {oauthClientID},
		"redirect_uri":          {oauthRedirectURI},
		"state":                 {state},
		"code_challenge":        {domain.PKCEChallengeS256(oauthVerifier)},
		"code_challenge_method": {"S256"},
		"resource":              {c.url + "/mcp"},
		"scope":                 {"mcp offline_access"},
	}
	resp := c.get("/oauth/authorize?"+query.Encode(), nil)
	require.Equal(c.t, http.StatusFound, resp.status, resp.body)
	assert.Equal(c.t, "no-store", resp.header.Get("Cache-Control"))
	assert.Empty(c.t, resp.header.Get("Access-Control-Allow-Origin"))
	location, err := url.Parse(resp.header.Get("Location"))
	require.NoError(c.t, err)
	require.Equal(c.t, "/console/oauth/authorize", location.Path)
	requestID = location.Query().Get("request")
	require.NotEmpty(c.t, requestID)

	cookies := (&http.Response{Header: resp.header}).Cookies()
	require.Len(c.t, cookies, 1)
	require.Equal(c.t, "nf_oauth_"+requestID, cookies[0].Name, "loopback http: no __Host- prefix")
	assert.True(c.t, cookies[0].HttpOnly)
	assert.Equal(c.t, http.SameSiteLaxMode, cookies[0].SameSite)
	return requestID, cookies[0].Name + "=" + cookies[0].Value
}

// redirectParams reads the redirect_url of an approval or a denial.
func (c oauthHTTP) redirectParams(resp oauthResponse) url.Values {
	c.t.Helper()
	require.Equal(c.t, http.StatusOK, resp.status, resp.body)
	redirect, err := url.Parse(resp.json(c.t)["redirect_url"].(string))
	require.NoError(c.t, err)
	require.Equal(c.t, oauthRedirectURI, redirect.Scheme+"://"+redirect.Host+redirect.Path)
	return redirect.Query()
}

type oauthTokens struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
	Scope        string `json:"scope"`
}

func (c oauthHTTP) tokens(resp oauthResponse) oauthTokens {
	c.t.Helper()
	require.Equal(c.t, http.StatusOK, resp.status, resp.body)
	assert.Equal(c.t, "no-store", resp.header.Get("Cache-Control"))
	var out oauthTokens
	require.NoError(c.t, json.Unmarshal([]byte(resp.body), &out))
	require.True(c.t, strings.HasPrefix(out.AccessToken, "nfmcp_at_"), out.AccessToken)
	require.True(c.t, strings.HasPrefix(out.RefreshToken, "nfmcp_rt_"), out.RefreshToken)
	assert.Equal(c.t, "Bearer", out.TokenType)
	assert.Equal(c.t, 3600, out.ExpiresIn)
	assert.Equal(c.t, "mcp", out.Scope)
	return out
}

func (c oauthHTTP) exchange(code string) oauthResponse {
	return c.form("/oauth/token", url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"client_id":     {oauthClientID},
		"redirect_uri":  {oauthRedirectURI},
		"code_verifier": {oauthVerifier},
		"resource":      {c.url + "/mcp"},
	})
}

func (c oauthHTTP) refresh(token string) oauthResponse {
	return c.form("/oauth/token", url.Values{"grant_type": {"refresh_token"}, "refresh_token": {token}, "client_id": {oauthClientID}})
}

// connectWithKey runs the whole flow with a pasted key and returns the tokens.
func (c oauthHTTP) connectWithKey(apiKey, state string) oauthTokens {
	c.t.Helper()
	requestID, cookie := c.authorize(state)
	params := c.redirectParams(c.call("/api/oauth.approveWithKey",
		fmt.Sprintf(`{"request_id":%q,"api_key":%q}`, requestID, apiKey), map[string]string{"Cookie": cookie}))
	return c.tokens(c.exchange(params.Get("code")))
}

// mcpUnauthorized posts to /mcp with a bearer and expects a 401.
func mcpStatus(t *testing.T, base, token string) (int, string) {
	t.Helper()
	client := &mcpClient{t: t, url: base, token: token}
	resp := client.post("server/discover", "", "")
	return resp.status, resp.body
}

func oauthSuite(t *testing.T, factory func(*config.Config) testutil.AppInterface) (*testutil.IntegrationTestSuite, oauthHTTP) {
	t.Helper()
	suite := testutil.NewIntegrationTestSuite(t, factory)
	// API_ENDPOINT is the loopback URL of the test server, on which OAuth is
	// available over plain http.
	suite.Config.APIEndpoint = suite.ServerManager.GetURL()
	suite.ServerManager.GetApp().GetOAuthService().SetFetcher(oauthClaudeDocument)
	return suite, oauthHTTP{t: t, url: suite.ServerManager.GetURL()}
}

// The OAuth authorization server of /mcp, end to end: discovery, a key pasted
// on the authorization page, the token endpoint, /mcp with the access token,
// rotation, revocation, the owner shortcut, and the refusals around them.
func TestOAuthMCP(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite, c := oauthSuite(t, mcpAppFactory)
	defer func() { suite.Cleanup() }()
	base := c.url

	factory := suite.DataFactory
	authService := suite.ServerManager.GetApp().GetAuthService().(*service.AuthService)
	owner, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	require.NoError(t, factory.AddUserToWorkspace(owner.ID, workspace.ID, "owner"))
	member, err := factory.CreateUser()
	require.NoError(t, err)
	require.NoError(t, factory.AddUserToWorkspace(member.ID, workspace.ID, "member"))

	ownerClient := testutil.NewAPIClient(base)
	require.NoError(t, ownerClient.Login(owner.Email, "password"))
	memberClient := testutil.NewAPIClient(base)
	require.NoError(t, memberClient.Login(member.Email, "password"))
	ownerSession := map[string]string{"Authorization": "Bearer " + ownerClient.GetToken()}

	var issuer string
	t.Run("the discovery documents", func(t *testing.T) {
		resp := c.get("/.well-known/oauth-protected-resource/mcp", map[string]string{"Origin": "https://claude.ai"})
		require.Equal(t, http.StatusOK, resp.status, resp.body)
		assert.JSONEq(t, fmt.Sprintf(`{"resource":"%s/mcp","authorization_servers":[%q],"scopes_supported":["mcp"],"bearer_methods_supported":["header"]}`, base, base), resp.body)
		assert.Equal(t, "*", resp.header.Get("Access-Control-Allow-Origin"))
		assert.Empty(t, resp.header.Get("Access-Control-Allow-Credentials"))

		resp = c.get("/.well-known/oauth-authorization-server", nil)
		require.Equal(t, http.StatusOK, resp.status, resp.body)
		metadata := resp.json(t)
		issuer = metadata["issuer"].(string)
		assert.Equal(t, base, issuer)
		assert.Equal(t, base+"/oauth/token", metadata["token_endpoint"])
		assert.Equal(t, true, metadata["client_id_metadata_document_supported"])
		assert.Equal(t, true, metadata["authorization_response_iss_parameter_supported"])
		assert.Equal(t, []interface{}{"none"}, metadata["token_endpoint_auth_methods_supported"])
		assert.Equal(t, []interface{}{"S256"}, metadata["code_challenge_methods_supported"])
		assert.Equal(t, []interface{}{"mcp", "offline_access"}, metadata["scopes_supported"])
		assert.NotContains(t, metadata, "registration_endpoint")

		for _, path := range []string{"/.well-known/openid-configuration", "/oauth/register"} {
			resp := c.call(path, `{}`, nil)
			assert.Equal(t, http.StatusNotFound, resp.status, path)
			assert.Equal(t, "application/json", resp.header.Get("Content-Type"), path)
		}

		// /mcp without a credential points at the protected resource metadata.
		status, _ := mcpStatus(t, base, "")
		assert.Equal(t, http.StatusUnauthorized, status)
		req, err := http.NewRequest(http.MethodPost, base+"/mcp", strings.NewReader(`{}`))
		require.NoError(t, err)
		httpResp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		_ = httpResp.Body.Close()
		assert.Equal(t, fmt.Sprintf(`Bearer resource_metadata="%s/.well-known/oauth-protected-resource/mcp", scope="mcp"`, base), httpResp.Header.Get("WWW-Authenticate"))
	})
	require.NotEmpty(t, issuer)

	fullKey, err := factory.CreateAPIKey(workspace.ID)
	require.NoError(t, err)
	fullKeyToken := authService.GenerateAPIAuthToken(fullKey)

	t.Run("the full flow with a pasted key", func(t *testing.T) {
		requestID, cookie := c.authorize("state-1")

		resp := c.get("/api/oauth.getAuthorizationRequest?request_id="+url.QueryEscape(requestID), nil)
		require.Equal(t, http.StatusOK, resp.status, resp.body)
		view := resp.json(t)
		assert.Equal(t, "Claude", view["client_name"])
		assert.Equal(t, "claude.ai", view["client_host"])
		assert.Equal(t, true, view["known_client"])
		assert.NotContains(t, view, "owned_workspaces", "no session, no shortcut")

		// The request cookie is required: without it, or from another request,
		// the approval is refused and the request stays usable.
		body := fmt.Sprintf(`{"request_id":%q,"api_key":%q}`, requestID, fullKeyToken)
		resp = c.call("/api/oauth.approveWithKey", body, nil)
		assert.Equal(t, http.StatusForbidden, resp.status)
		assert.Equal(t, "oauth_browser_mismatch", resp.json(t)["error"])
		resp = c.call("/api/oauth.approveWithKey", body, map[string]string{"Cookie": "nf_oauth_" + requestID + "=forged"})
		assert.Equal(t, http.StatusForbidden, resp.status)

		// What is not an API key is refused, with the code the page shows.
		resp = c.call("/api/oauth.approveWithKey", fmt.Sprintf(`{"request_id":%q,"api_key":%q}`, requestID, ownerClient.GetToken()), map[string]string{"Cookie": cookie})
		assert.Equal(t, http.StatusBadRequest, resp.status)
		assert.Equal(t, "oauth_not_an_api_key", resp.json(t)["error"])

		params := c.redirectParams(c.call("/api/oauth.approveWithKey", body, map[string]string{"Cookie": cookie}))
		assert.Equal(t, "state-1", params.Get("state"))
		assert.Equal(t, issuer, params.Get("iss"), "iss is the metadata issuer byte for byte")
		code := params.Get("code")
		require.NotEmpty(t, code)

		// The request is consumed.
		resp = c.call("/api/oauth.approveWithKey", body, map[string]string{"Cookie": cookie})
		assert.Equal(t, http.StatusGone, resp.status)

		tokens := c.tokens(c.exchange(code))
		// A code is used once; a second use revokes what it granted.
		resp = c.exchange(code)
		assert.Equal(t, http.StatusBadRequest, resp.status)
		assert.Equal(t, "invalid_grant", resp.json(t)["error"])
		status, _ := mcpStatus(t, base, tokens.AccessToken)
		assert.Equal(t, http.StatusUnauthorized, status, "the replayed code revoked the grant")

		// Start again, and use the tokens.
		tokens = c.connectWithKey(fullKeyToken, "state-2")
		oauthClient := &mcpClient{t: t, url: base, token: tokens.AccessToken}
		result := oauthClient.call("list_workspaces", `{}`)
		require.False(t, result.IsError, string(result.StructuredContent))
		assert.Contains(t, string(result.StructuredContent), workspace.ID)

		// A refresh token is no bearer for /mcp, and an access token no refresh
		// token for the token endpoint.
		status, _ = mcpStatus(t, base, tokens.RefreshToken)
		assert.Equal(t, http.StatusUnauthorized, status)
		resp = c.refresh(tokens.AccessToken)
		assert.Equal(t, http.StatusBadRequest, resp.status)
		assert.Equal(t, "invalid_grant", resp.json(t)["error"])

		// An access token is refused on /api/: it is no JWT.
		apiResp := c.get("/api/workspaces.list", map[string]string{"Authorization": "Bearer " + tokens.AccessToken})
		assert.Equal(t, http.StatusUnauthorized, apiResp.status)

		// Rotation: a new pair, the old access token dead, the new one alive.
		rotated := c.tokens(c.refresh(tokens.RefreshToken))
		assert.NotEqual(t, tokens.RefreshToken, rotated.RefreshToken)
		status, _ = mcpStatus(t, base, tokens.AccessToken)
		assert.Equal(t, http.StatusUnauthorized, status, "rotation invalidates the live access tokens")
		status, body = mcpStatus(t, base, rotated.AccessToken)
		assert.Equal(t, http.StatusOK, status, body)

		// Revocation: an unknown token answers 200; the refresh token revokes
		// the grant, and with it the access token.
		resp = c.form("/oauth/revoke", url.Values{"token": {"nfmcp_rt_unknown"}, "client_id": {oauthClientID}})
		assert.Equal(t, http.StatusOK, resp.status)
		assert.Equal(t, "*", resp.header.Get("Access-Control-Allow-Origin"))
		resp = c.form("/oauth/revoke", url.Values{"token": {rotated.RefreshToken}, "client_id": {"https://other.example/client"}})
		assert.Equal(t, http.StatusOK, resp.status, "another client's token: 200, nothing changes")
		status, _ = mcpStatus(t, base, rotated.AccessToken)
		assert.Equal(t, http.StatusOK, status)

		resp = c.form("/oauth/revoke", url.Values{"token": {rotated.RefreshToken}, "client_id": {oauthClientID}})
		assert.Equal(t, http.StatusOK, resp.status)
		status, _ = mcpStatus(t, base, rotated.AccessToken)
		assert.Equal(t, http.StatusUnauthorized, status)
		resp = c.refresh(rotated.RefreshToken)
		assert.Equal(t, "invalid_grant", resp.json(t)["error"])

		// The approval and the client's revocation are in the workspace's log.
		for _, action := range []string{"oauth.approveWithKey", "oauth.revoke"} {
			page := listAuditLogs(t, ownerClient, map[string]string{"workspace_id": workspace.ID, "actions": action, "limit": "10"})
			require.NotEmpty(t, page.Logs, action)
			assert.Equal(t, "Claude", page.Logs[0].TargetName, action)
			assert.Equal(t, fullKey.ID, page.Logs[0].ActorID, action)
		}
	})

	t.Run("a denial redirects with access_denied, the state and iss", func(t *testing.T) {
		requestID, cookie := c.authorize("state-deny")
		resp := c.call("/api/oauth.deny", fmt.Sprintf(`{"request_id":%q}`, requestID), nil)
		assert.Equal(t, http.StatusForbidden, resp.status, "the cookie is required")
		params := c.redirectParams(c.call("/api/oauth.deny", fmt.Sprintf(`{"request_id":%q}`, requestID), map[string]string{"Cookie": cookie}))
		assert.Equal(t, "access_denied", params.Get("error"))
		assert.Equal(t, "state-deny", params.Get("state"))
		assert.Equal(t, issuer, params.Get("iss"))
		assert.Empty(t, params.Get("code"))
	})

	t.Run("the owner shortcut", func(t *testing.T) {
		requestID, cookie := c.authorize("state-owner")

		resp := c.get("/api/oauth.getAuthorizationRequest?request_id="+url.QueryEscape(requestID), ownerSession)
		require.Equal(t, http.StatusOK, resp.status, resp.body)
		owned := resp.json(t)["owned_workspaces"].([]interface{})
		require.Len(t, owned, 1)
		assert.Equal(t, workspace.ID, owned[0].(map[string]interface{})["id"])
		assert.Equal(t, false, owned[0].(map[string]interface{})["mcp_disabled"])

		body := fmt.Sprintf(`{"request_id":%q,"workspace_id":%q,"name":"claude","permissions":{"contacts":{"read":true,"write":false}}}`, requestID, workspace.ID)
		// Never an API key, nor an OAuth token, nor a member.
		resp = c.call("/api/oauth.createKeyAndApprove", body, map[string]string{"Cookie": cookie, "Authorization": "Bearer " + fullKeyToken})
		assert.Equal(t, http.StatusForbidden, resp.status)
		resp = c.call("/api/oauth.createKeyAndApprove", body, map[string]string{"Cookie": cookie, "Authorization": "Bearer nfmcp_at_forged"})
		assert.Equal(t, http.StatusUnauthorized, resp.status)
		resp = c.call("/api/oauth.createKeyAndApprove", body, map[string]string{"Cookie": cookie, "Authorization": "Bearer " + memberClient.GetToken()})
		assert.Equal(t, http.StatusForbidden, resp.status)
		// Nor without the cookie.
		resp = c.call("/api/oauth.createKeyAndApprove", body, ownerSession)
		assert.Equal(t, http.StatusForbidden, resp.status)
		assert.Equal(t, "oauth_browser_mismatch", resp.json(t)["error"])

		params := c.redirectParams(c.call("/api/oauth.createKeyAndApprove", body, map[string]string{"Cookie": cookie, "Authorization": ownerSession["Authorization"]}))
		assert.Equal(t, "state-owner", params.Get("state"))
		assert.Equal(t, issuer, params.Get("iss"))
		tokens := c.tokens(c.exchange(params.Get("code")))

		// The new key reads contacts and nothing else.
		oauthClient := &mcpClient{t: t, url: base, token: tokens.AccessToken}
		assert.Contains(t, oauthClient.toolNames(), "search_contacts")
		assert.NotContains(t, oauthClient.toolNames(), "list_lists")

		// The owner lists the connection; a member, another workspace's owner
		// and an OAuth token cannot.
		listPath := "/api/oauth.listWorkspaceGrants?workspace_id=" + workspace.ID
		resp = c.get(listPath, ownerSession)
		require.Equal(t, http.StatusOK, resp.status, resp.body)
		var list struct {
			Grants []domain.OAuthGrantListItem `json:"grants"`
		}
		require.NoError(t, json.Unmarshal([]byte(resp.body), &list))
		var shortcut *domain.OAuthGrantListItem
		for i := range list.Grants {
			if list.Grants[i].CreatedVia == domain.OAuthGrantViaOwnerShortcut {
				shortcut = &list.Grants[i]
			}
		}
		require.NotNil(t, shortcut, resp.body)
		assert.Equal(t, owner.ID, shortcut.CreatedByUserID)
		assert.Equal(t, domain.OAuthGrantStatusActive, shortcut.Status)

		resp = c.get(listPath, map[string]string{"Authorization": "Bearer " + memberClient.GetToken()})
		assert.Equal(t, http.StatusForbidden, resp.status)
		stranger, err := factory.CreateUser()
		require.NoError(t, err)
		otherWorkspace, err := factory.CreateWorkspace()
		require.NoError(t, err)
		require.NoError(t, factory.AddUserToWorkspace(stranger.ID, otherWorkspace.ID, "owner"))
		strangerClient := testutil.NewAPIClient(base)
		require.NoError(t, strangerClient.Login(stranger.Email, "password"))
		resp = c.get(listPath, map[string]string{"Authorization": "Bearer " + strangerClient.GetToken()})
		assert.Equal(t, http.StatusForbidden, resp.status)
		resp = c.get(listPath, map[string]string{"Authorization": "Bearer " + tokens.AccessToken})
		assert.Equal(t, http.StatusUnauthorized, resp.status)

		// Revoking the connection keeps the key and ends the tokens. Another
		// workspace's owner is told the grant does not exist.
		resp = c.call("/api/oauth.revokeGrant", fmt.Sprintf(`{"grant_id":%q}`, shortcut.ID), map[string]string{"Authorization": "Bearer " + strangerClient.GetToken()})
		assert.Equal(t, http.StatusNotFound, resp.status)
		resp = c.call("/api/oauth.revokeGrant", fmt.Sprintf(`{"grant_id":%q}`, shortcut.ID), ownerSession)
		require.Equal(t, http.StatusOK, resp.status, resp.body)
		status, _ := mcpStatus(t, base, tokens.AccessToken)
		assert.Equal(t, http.StatusUnauthorized, status)
		key, err := suite.ServerManager.GetApp().GetUserRepository().GetUserByID(context.Background(), shortcut.APIKeyUserID)
		require.NoError(t, err)
		assert.Equal(t, domain.UserTypeAPIKey, key.Type)

		for _, action := range []string{"workspaces.createAPIKey", "oauth.createKeyAndApprove", "oauth.revokeGrant"} {
			page := listAuditLogs(t, ownerClient, map[string]string{"workspace_id": workspace.ID, "actions": action, "limit": "10"})
			require.NotEmpty(t, page.Logs, action)
			assert.Equal(t, owner.ID, page.Logs[0].ActorID, action)
		}
	})

	t.Run("a key restricted to reads is refused every other tool over OAuth", func(t *testing.T) {
		readOnly, err := factory.CreateAPIKey(workspace.ID, testutil.WithAPIKeyPermissions(domain.UserPermissions{
			domain.PermissionResourceContacts:       {Read: true},
			domain.PermissionResourceLists:          {Read: true},
			domain.PermissionResourceTemplates:      {Read: true},
			domain.PermissionResourceBroadcasts:     {Read: true},
			domain.PermissionResourceTransactional:  {Read: true},
			domain.PermissionResourceWorkspace:      {Read: true},
			domain.PermissionResourceMessageHistory: {Read: true},
			domain.PermissionResourceBlog:           {Read: true},
			domain.PermissionResourceAutomations:    {Read: true},
			domain.PermissionResourceSegments:       {Read: true},
		}))
		require.NoError(t, err)
		tokens := c.connectWithKey(authService.GenerateAPIAuthToken(readOnly), "state-read")
		oauthClient := &mcpClient{t: t, url: base, token: tokens.AccessToken}
		listed := oauthClient.toolNames()

		args := func(fields string) string {
			return fmt.Sprintf(`{"workspace_id":%q,%s}`, workspace.ID, fields)
		}
		const tree = `{"kind":"leaf","leaf":{"source":"contacts","contact":{"filters":[{"field_name":"email","field_type":"string","operator":"is_set"}]}}}`
		const body = `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"Hello"}]}]}`
		for tool, arguments := range map[string]string{
			"upsert_contact":                    args(`"email":"a@example.com"`),
			"delete_contact":                    args(`"email":"a@example.com"`),
			"track_custom_event":                args(`"email":"a@example.com","event_name":"signup","external_id":"e1","occurred_at":"2026-11-20T10:00:00Z"`),
			"create_list":                       args(`"id":"refused","name":"Refused"`),
			"update_list":                       args(`"id":"refused","name":"Refused"`),
			"delete_list":                       args(`"id":"refused"`),
			"subscribe_contact_to_list":         args(`"email":"a@example.com","list_id":"refused"`),
			"set_contact_list_status":           args(`"email":"a@example.com","list_id":"refused","status":"active"`),
			"remove_contact_from_list":          args(`"email":"a@example.com","list_id":"refused"`),
			"create_segment":                    args(`"id":"refused","name":"Refused","tree":` + tree),
			"update_segment":                    args(`"id":"refused","name":"Refused"`),
			"delete_segment":                    args(`"id":"refused"`),
			"create_template":                   args(`"id":"refused","name":"Refused","category":"marketing","subject":"Hi","mjml_source":"<mjml><mj-body></mj-body></mjml>"`),
			"update_template":                   args(`"id":"refused","name":"Refused"`),
			"delete_template":                   args(`"id":"refused"`),
			"create_broadcast":                  args(`"name":"Refused","list_id":"refused"`),
			"update_broadcast":                  args(`"broadcast_id":"refused","name":"Refused"`),
			"send_broadcast_to_contact":         args(`"broadcast_id":"refused","recipient_email":"a@example.com"`),
			"schedule_broadcast":                args(`"broadcast_id":"refused","when":"now"`),
			"select_broadcast_winner":           args(`"broadcast_id":"refused","template_id":"refused"`),
			"pause_broadcast":                   args(`"broadcast_id":"refused"`),
			"resume_broadcast":                  args(`"broadcast_id":"refused"`),
			"cancel_broadcast":                  args(`"broadcast_id":"refused"`),
			"unschedule_broadcast":              args(`"broadcast_id":"refused"`),
			"delete_broadcast":                  args(`"broadcast_id":"refused"`),
			"create_transactional_notification": args(`"notification_id":"refused","name":"Refused","email_template_id":"refused"`),
			"update_transactional_notification": args(`"notification_id":"refused","name":"Refused"`),
			"delete_transactional_notification": args(`"notification_id":"refused"`),
			"send_transactional":                args(`"notification_id":"refused","external_id":"x1","email":"a@example.com"`),
			"send_template_test":                args(`"template_id":"refused","recipient_email":"a@example.com"`),
			"activate_automation":               args(`"automation_id":"refused"`),
			"pause_automation":                  args(`"automation_id":"refused"`),
			"delete_automation":                 args(`"automation_id":"refused"`),
			"create_blog_post":                  args(`"category_id":"refused","slug":"refused","title":"Refused","body":` + body),
			"update_blog_post":                  args(`"id":"refused","title":"Refused"`),
			"publish_blog_post":                 args(`"id":"refused"`),
			"unpublish_blog_post":               args(`"id":"refused"`),
			"delete_blog_post":                  args(`"id":"refused"`),
		} {
			assert.NotContains(t, listed, tool)
			result := oauthClient.call(tool, arguments)
			assert.Equal(t, "permission_denied", result.errorCode(t), tool)
		}
		// The reads work.
		result := oauthClient.call("search_contacts", args(`"limit":1`))
		assert.False(t, result.IsError, string(result.StructuredContent))
	})

	t.Run("deleting the key ends its connection", func(t *testing.T) {
		doomed, err := factory.CreateAPIKey(workspace.ID)
		require.NoError(t, err)
		tokens := c.connectWithKey(authService.GenerateAPIAuthToken(doomed), "state-doomed")
		status, body := mcpStatus(t, base, tokens.AccessToken)
		require.Equal(t, http.StatusOK, status, body)

		resp, err := ownerClient.PostRaw("/api/workspaces.removeMember", fmt.Sprintf(`{"workspace_id":%q,"user_id":%q}`, workspace.ID, doomed.ID))
		require.NoError(t, err)
		_ = resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		status, _ = mcpStatus(t, base, tokens.AccessToken)
		assert.Equal(t, http.StatusUnauthorized, status)
		refreshed := c.refresh(tokens.RefreshToken)
		assert.Equal(t, http.StatusBadRequest, refreshed.status)
		assert.Equal(t, "invalid_grant", refreshed.json(t)["error"])
	})

	t.Run("a foreign host gets a 404 on /oauth/ and .well-known", func(t *testing.T) {
		for _, path := range []string{"/.well-known/oauth-authorization-server", "/oauth/token"} {
			resp := c.do(http.MethodGet, path, "", "", map[string]string{"Host": "other.example.com"})
			assert.Equal(t, http.StatusNotFound, resp.status, path)
			assert.Equal(t, "application/json", resp.header.Get("Content-Type"), path)
		}
	})

	// Last: it changes API_ENDPOINT.
	t.Run("OAuth is unavailable when API_ENDPOINT is plain http on a public host", func(t *testing.T) {
		previous := suite.Config.APIEndpoint
		suite.Config.APIEndpoint = "http://notifuse.example.com"
		defer func() { suite.Config.APIEndpoint = previous }()
		host := map[string]string{"Host": "notifuse.example.com"}

		for _, path := range []string{"/.well-known/oauth-protected-resource/mcp", "/.well-known/oauth-authorization-server", "/oauth/authorize?response_type=code", "/oauth/token"} {
			resp := c.do(http.MethodGet, path, "", "", host)
			assert.Equal(t, http.StatusNotFound, resp.status, path)
			assert.Contains(t, resp.body, "API_ENDPOINT is not https", path)
		}
		// /mcp still takes API keys, and its 401 no longer points at OAuth.
		resp := c.do(http.MethodPost, "/mcp", "application/json", `{}`, host)
		assert.Equal(t, http.StatusUnauthorized, resp.status)
		assert.Equal(t, "Bearer", resp.header.Get("WWW-Authenticate"))
	})
}

// The owner shortcut follows the instance's two-factor requirement: a session
// that has not passed a second factor gets the 403 the rest of /api/ gives.
func TestOAuthMCP_OwnerShortcutNeedsTheRequiredSecondFactor(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite, c := oauthSuite(t, func(cfg *config.Config) testutil.AppInterface {
		cfg.MCP.Enabled = true
		cfg.RequireTwoFactor = true
		return app.NewApp(cfg)
	})
	defer func() { suite.Cleanup() }()

	const email = "testuser@example.com" // owner of test-workspace-id
	session := testutil.NewAPIClient(c.url)
	session.SetToken(tfSignIn(t, session, email))

	requestID, cookie := c.authorize("state-2fa")
	resp := c.call("/api/oauth.createKeyAndApprove",
		fmt.Sprintf(`{"request_id":%q,"workspace_id":"test-workspace-id","name":"claude"}`, requestID),
		map[string]string{"Cookie": cookie, "Authorization": "Bearer " + session.GetToken()})
	assert.Equal(t, http.StatusForbidden, resp.status)
	assert.Equal(t, "two_factor_required", resp.json(t)["error"])

	// Nor does the authorization page list the workspaces such a session owns.
	resp = c.get("/api/oauth.getAuthorizationRequest?request_id="+url.QueryEscape(requestID), map[string]string{"Authorization": "Bearer " + session.GetToken()})
	require.Equal(t, http.StatusOK, resp.status, resp.body)
	assert.NotContains(t, resp.json(t), "owned_workspaces")
}
