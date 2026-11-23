package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/Notifuse/notifuse/pkg/logger"
)

const (
	oauthTestIssuer    = "https://notifuse.example.com"
	oauthTestResource  = "https://notifuse.example.com/mcp"
	oauthTestClientID  = "https://app.example.com/oauth/client.json"
	oauthTestRedirect  = "https://app.example.com/callback"
	oauthTestVerifier  = "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	oauthTestChallenge = "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"
	oauthTestKeyToken  = "eyJhbGciOiJIUzI1NiJ9.the-pasted-key.signature"
	oauthTestKeyID     = "key-1"
	oauthTestOwnerID   = "owner-1"
	oauthTestMemberID  = "member-1"
	oauthTestRootID    = "root-1"
	// oauthTestStrangerID owns ws2 and nothing in ws1.
	oauthTestStrangerID = "stranger-1"
)

var oauthTestMeta = domain.OAuthCallMeta{IP: "203.0.113.9", UserAgent: "test-agent", RequestID: "req-1"}

type oauthTestEnv struct {
	t        *testing.T
	repo     *fakeOAuthRepo
	auth     *fakeOAuthAuth
	ws       *fakeOAuthWorkspaces
	fetcher  *fakeOAuthFetcher
	recorder *recordingAuditRecorder
	svc      *OAuthService
	keys     *WorkspaceService
	cfg      *config.Config

	clockMu     sync.Mutex
	clock       time.Time
	apiEndpoint string
	licensed    bool
}

func (e *oauthTestEnv) now() time.Time {
	e.clockMu.Lock()
	defer e.clockMu.Unlock()
	return e.clock
}

func (e *oauthTestEnv) advance(d time.Duration) {
	e.clockMu.Lock()
	defer e.clockMu.Unlock()
	e.clock = e.clock.Add(d)
}

func (e *oauthTestEnv) Entitlements() domain.Entitlements {
	grants := domain.CommunityEntitlements()
	if e.licensed {
		grants.Features = []domain.Feature{domain.FeatureRBAC}
	}
	return grants
}

func oauthDocument(clientID, name string, redirectURIs ...string) *OAuthClientDocument {
	body, _ := json.Marshal(map[string]any{"client_id": clientID, "client_name": name, "redirect_uris": redirectURIs})
	return &OAuthClientDocument{Body: body, Header: http.Header{"Cache-Control": {"max-age=3600"}}}
}

func newOAuthTestEnv(t *testing.T) *oauthTestEnv {
	t.Helper()
	repo := newFakeOAuthRepo()
	auth := newFakeOAuthAuth(repo)
	auth.rootEmail = "root@example.com"
	e := &oauthTestEnv{
		t:           t,
		repo:        repo,
		auth:        auth,
		fetcher:     &fakeOAuthFetcher{documents: map[string]*OAuthClientDocument{}, errs: map[string]error{}},
		recorder:    &recordingAuditRecorder{},
		clock:       time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC),
		apiEndpoint: oauthTestIssuer,
	}
	e.ws = &fakeOAuthWorkspaces{auth: auth, workspaces: map[string]*domain.Workspace{
		"ws1":    {ID: "ws1", Name: "Acme"},
		"ws2":    {ID: "ws2", Name: "Other"},
		"ws-off": {ID: "ws-off", Name: "Off", Settings: domain.WorkspaceSettings{MCP: &domain.MCPSettings{Disabled: true}}},
	}}
	e.fetcher.documents[oauthTestClientID] = oauthDocument(oauthTestClientID, "Example App", oauthTestRedirect, "http://localhost/callback")

	auth.accounts[oauthTestKeyID] = &domain.User{ID: oauthTestKeyID, Email: "agent@notifuse.example.com", Type: domain.UserTypeAPIKey}
	auth.apiKeys[oauthTestKeyToken] = oauthTestKeyID
	auth.memberships[oauthTestKeyID] = map[string]*domain.UserWorkspace{"ws1": {UserID: oauthTestKeyID, WorkspaceID: "ws1", Role: "member"}}
	for id, email := range map[string]string{oauthTestOwnerID: "owner@example.com", oauthTestMemberID: "member@example.com", oauthTestRootID: "root@example.com", oauthTestStrangerID: "stranger@example.com"} {
		auth.accounts[id] = &domain.User{ID: id, Email: email, Type: domain.UserTypeUser}
		auth.sessions[id] = true
	}
	auth.memberships[oauthTestOwnerID] = map[string]*domain.UserWorkspace{
		"ws1":    {UserID: oauthTestOwnerID, WorkspaceID: "ws1", Role: "owner"},
		"ws-off": {UserID: oauthTestOwnerID, WorkspaceID: "ws-off", Role: "owner"},
	}
	auth.memberships[oauthTestMemberID] = map[string]*domain.UserWorkspace{"ws1": {UserID: oauthTestMemberID, WorkspaceID: "ws1", Role: "member"}}
	auth.memberships[oauthTestStrangerID] = map[string]*domain.UserWorkspace{"ws2": {UserID: oauthTestStrangerID, WorkspaceID: "ws2", Role: "owner"}}

	e.cfg = &config.Config{APIEndpoint: oauthTestIssuer, RootEmail: "root@example.com"}
	e.keys = NewWorkspaceService(nil, nil, nil, logger.NewLoggerWithLevel("disabled"), nil, auth, nil, e.cfg, nil, nil, nil, nil, nil, "", nil, nil, nil, e, WorkspaceInvitationAuth{})
	e.svc = NewOAuthService(OAuthServiceConfig{
		Repo:        repo,
		Auth:        auth,
		Keys:        e.keys,
		Workspaces:  e.ws,
		Fetcher:     e.fetcher,
		Recorder:    e.recorder,
		APIEndpoint: func() string { return e.apiEndpoint },
		IsRootEmail: e.cfg.IsRootEmail,
		Now:         e.now,
	})
	return e
}

func (e *oauthTestEnv) params() domain.OAuthAuthorizeParams {
	return domain.OAuthAuthorizeParams{
		ResponseType: "code", ClientID: oauthTestClientID, RedirectURI: oauthTestRedirect, State: "st-1",
		CodeChallenge: oauthTestChallenge, CodeChallengeMethod: "S256",
	}
}

func (e *oauthTestEnv) authorize() *domain.OAuthAuthorizationStarted {
	e.t.Helper()
	started, err := e.svc.CreateAuthorizationRequest(context.Background(), e.params())
	require.NoError(e.t, err)
	return started
}

func oauthSessionCtx(userID string) context.Context {
	ctx := context.WithValue(context.Background(), domain.UserIDKey, userID)
	ctx = context.WithValue(ctx, domain.UserTypeKey, string(domain.UserTypeUser))
	return context.WithValue(ctx, domain.SessionIDKey, "session-"+userID)
}

func oauthKeyCtx(userID string) context.Context {
	ctx := context.WithValue(context.Background(), domain.UserIDKey, userID)
	return context.WithValue(ctx, domain.UserTypeKey, string(domain.UserTypeAPIKey))
}

func redirectParams(t *testing.T, redirect *domain.OAuthRedirect) url.Values {
	t.Helper()
	parsed, err := url.Parse(redirect.RedirectURL)
	require.NoError(t, err)
	return parsed.Query()
}

// approve runs the pasted-key path and returns the code.
func (e *oauthTestEnv) approve(started *domain.OAuthAuthorizationStarted) string {
	e.t.Helper()
	redirect, err := e.svc.ApproveWithKey(context.Background(), domain.OAuthApproveWithKeyRequest{RequestID: started.RequestID, APIKey: oauthTestKeyToken, BrowserBinding: started.BrowserBinding}, oauthTestMeta)
	require.NoError(e.t, err)
	return redirectParams(e.t, redirect).Get("code")
}

func (e *oauthTestEnv) exchangeRequest(code string) domain.OAuthCodeExchange {
	return domain.OAuthCodeExchange{Code: code, ClientID: oauthTestClientID, RedirectURI: oauthTestRedirect, CodeVerifier: oauthTestVerifier}
}

// connect runs authorize, approve and exchange.
func (e *oauthTestEnv) connect() *domain.OAuthTokenResponse {
	e.t.Helper()
	tokens, err := e.svc.ExchangeCode(context.Background(), e.exchangeRequest(e.approve(e.authorize())), oauthTestMeta)
	require.NoError(e.t, err)
	return tokens
}

func (e *oauthTestEnv) refresh(refreshToken string) (*domain.OAuthTokenResponse, error) {
	return e.svc.Refresh(context.Background(), domain.OAuthRefresh{RefreshToken: refreshToken, ClientID: oauthTestClientID}, oauthTestMeta)
}

func (e *oauthTestEnv) resolves(accessToken string) bool {
	_, err := e.svc.ResolveAccessToken(context.Background(), accessToken)
	if err != nil && !errors.Is(err, domain.ErrOAuthInvalidToken) {
		e.t.Fatalf("unexpected resolution error: %v", err)
	}
	return err == nil
}

func requireInvalidGrant(t *testing.T, err error) {
	t.Helper()
	var oauthErr *domain.OAuthError
	require.ErrorAs(t, err, &oauthErr)
	assert.Equal(t, domain.OAuthErrInvalidGrant, oauthErr.Code)
}

func requireAuthorizeReason(t *testing.T, err error, reason string) {
	t.Helper()
	var authorizeErr *domain.OAuthAuthorizeError
	require.ErrorAs(t, err, &authorizeErr)
	assert.Equal(t, reason, authorizeErr.Reason)
}

// Authorization requests

func TestOAuthService_CreateAuthorizationRequest(t *testing.T) {
	e := newOAuthTestEnv(t)
	started := e.authorize()

	assert.Len(t, started.RequestID, 22, "128 random bits, base64url")
	request := e.repo.request(started.RequestID)
	assert.Equal(t, oauthTestClientID, request.ClientID)
	assert.Equal(t, domain.OAuthClientSnapshot{ClientID: oauthTestClientID, ClientName: "Example App", RedirectURIs: []string{oauthTestRedirect, "http://localhost/callback"}}, request.ClientSnapshot)
	assert.Equal(t, oauthTestResource, request.Resource, "a client that omits resource is bound to the canonical one")
	assert.Equal(t, "st-1", request.State)
	assert.Equal(t, e.now().Add(10*time.Minute), request.ExpiresAt)
	assert.Equal(t, request.ExpiresAt, started.ExpiresAt)
	assert.Nil(t, request.ConsumedAt)
	assert.Equal(t, domain.HashOAuthSecret(started.BrowserBinding), request.BrowserBindingHash)
	assert.NotContains(t, request.BrowserBindingHash, started.BrowserBinding, "the cookie is stored hashed")

	t.Run("the document is cached", func(t *testing.T) {
		e.authorize()
		assert.Equal(t, 1, e.fetcher.count())
	})
	t.Run("a resource is compared canonically", func(t *testing.T) {
		p := e.params()
		p.Resource = "HTTPS://Notifuse.Example.com:443/mcp/"
		_, err := e.svc.CreateAuthorizationRequest(context.Background(), p)
		require.NoError(t, err)
		p.Resource = "https://notifuse.example.com/other"
		_, err = e.svc.CreateAuthorizationRequest(context.Background(), p)
		requireAuthorizeReason(t, err, domain.OAuthAuthorizeInvalidResource)
	})
	t.Run("a loopback redirect matches on any port", func(t *testing.T) {
		p := e.params()
		p.RedirectURI = "http://localhost:51234/callback"
		_, err := e.svc.CreateAuthorizationRequest(context.Background(), p)
		require.NoError(t, err)
	})
	t.Run("a redirect the client does not list is refused", func(t *testing.T) {
		p := e.params()
		p.RedirectURI = "https://app.example.com/other"
		_, err := e.svc.CreateAuthorizationRequest(context.Background(), p)
		requireAuthorizeReason(t, err, domain.OAuthAuthorizeInvalidRedirect)
	})
}

// Syntax is checked before the document is fetched: a made-up request costs
// no outbound call.
func TestOAuthService_CreateAuthorizationRequest_RefusesBeforeFetching(t *testing.T) {
	for name, tc := range map[string]struct {
		edit   func(*domain.OAuthAuthorizeParams)
		reason string
	}{
		"response_type token":         {func(p *domain.OAuthAuthorizeParams) { p.ResponseType = "token" }, domain.OAuthAuthorizeUnsupportedResponseType},
		"a client_id that is no URL":  {func(p *domain.OAuthAuthorizeParams) { p.ClientID = "cursor-client" }, domain.OAuthAuthorizeNotCIMD},
		"an http client_id":           {func(p *domain.OAuthAuthorizeParams) { p.ClientID = "http://app.example.com/client" }, domain.OAuthAuthorizeNotCIMD},
		"a client_id with a fragment": {func(p *domain.OAuthAuthorizeParams) { p.ClientID = oauthTestClientID + "#x" }, domain.OAuthAuthorizeInvalidClientID},
		"a client_id with user info":  {func(p *domain.OAuthAuthorizeParams) { p.ClientID = "https://u@app.example.com/client" }, domain.OAuthAuthorizeInvalidClientID},
		"a client_id with a dot":      {func(p *domain.OAuthAuthorizeParams) { p.ClientID = "https://app.example.com/a/../client" }, domain.OAuthAuthorizeInvalidClientID},
		"a client_id at the root":     {func(p *domain.OAuthAuthorizeParams) { p.ClientID = "https://app.example.com/" }, domain.OAuthAuthorizeInvalidClientID},
		"a redirect with a fragment":  {func(p *domain.OAuthAuthorizeParams) { p.RedirectURI = oauthTestRedirect + "#x" }, domain.OAuthAuthorizeInvalidRedirect},
		"a javascript redirect":       {func(p *domain.OAuthAuthorizeParams) { p.RedirectURI = "javascript:alert(1)" }, domain.OAuthAuthorizeInvalidRedirect},
		"a data redirect":             {func(p *domain.OAuthAuthorizeParams) { p.RedirectURI = "data:text/html,x" }, domain.OAuthAuthorizeInvalidRedirect},
		"an http redirect":            {func(p *domain.OAuthAuthorizeParams) { p.RedirectURI = "http://app.example.com/callback" }, domain.OAuthAuthorizeInvalidRedirect},
		"no challenge method":         {func(p *domain.OAuthAuthorizeParams) { p.CodeChallengeMethod = "" }, domain.OAuthAuthorizeInvalidPKCE},
		"the plain method":            {func(p *domain.OAuthAuthorizeParams) { p.CodeChallengeMethod = "plain" }, domain.OAuthAuthorizeInvalidPKCE},
		"a short challenge":           {func(p *domain.OAuthAuthorizeParams) { p.CodeChallenge = "abc" }, domain.OAuthAuthorizeInvalidPKCE},
		"another resource":            {func(p *domain.OAuthAuthorizeParams) { p.Resource = "https://evil.example/mcp" }, domain.OAuthAuthorizeInvalidResource},
	} {
		t.Run(name, func(t *testing.T) {
			e := newOAuthTestEnv(t)
			p := e.params()
			tc.edit(&p)
			_, err := e.svc.CreateAuthorizationRequest(context.Background(), p)
			requireAuthorizeReason(t, err, tc.reason)
			assert.Zero(t, e.fetcher.count())
		})
	}
}

func TestOAuthService_Unavailable(t *testing.T) {
	for endpoint, reason := range map[string]string{
		"http://notifuse.example.com":  "API_ENDPOINT is not https",
		"https://example.com/notifuse": "API_ENDPOINT has a path",
		"":                             "API_ENDPOINT is not an http(s) URL",
		"http://localhost:8080":        "",
	} {
		e := newOAuthTestEnv(t)
		e.apiEndpoint = endpoint
		assert.Equal(t, reason, e.svc.Unavailable(), endpoint)
		if reason != "" {
			_, err := e.svc.CreateAuthorizationRequest(context.Background(), e.params())
			assert.ErrorIs(t, err, domain.ErrOAuthUnavailable, endpoint)
			_, err = e.svc.Issuer()
			assert.ErrorIs(t, err, domain.ErrOAuthUnavailable, endpoint)
		}
	}
}

// Client ID Metadata Documents

func TestOAuthService_ClientDocuments(t *testing.T) {
	t.Run("ChatGPT's and Claude Code's documents are accepted", func(t *testing.T) {
		e := newOAuthTestEnv(t)
		chatGPT := "https://chatgpt.com/oauth/client-metadata.json"
		e.fetcher.documents[chatGPT] = &OAuthClientDocument{Body: []byte(`{
			"client_id": "https://chatgpt.com/oauth/client-metadata.json",
			"client_name": "ChatGPT",
			"redirect_uris": ["https://chatgpt.com/connector_platform_oauth_redirect"],
			"token_endpoint_auth_methods_supported": ["none", "private_key_jwt"],
			"token_endpoint_auth_method": "private_key_jwt"
		}`)}
		claudeCode := "https://claude.ai/oauth/claude-code-client-metadata"
		e.fetcher.documents[claudeCode] = &OAuthClientDocument{Body: []byte(`{
			"client_id": "https://claude.ai/oauth/claude-code-client-metadata",
			"client_name": "Claude Code",
			"redirect_uris": ["http://localhost/callback", "http://127.0.0.1/callback"],
			"token_endpoint_auth_method": "none"
		}`)}
		p := e.params()
		p.ClientID, p.RedirectURI = chatGPT, "https://chatgpt.com/connector_platform_oauth_redirect"
		_, err := e.svc.CreateAuthorizationRequest(context.Background(), p)
		require.NoError(t, err)
		p.ClientID, p.RedirectURI = claudeCode, "http://127.0.0.1:43123/callback"
		_, err = e.svc.CreateAuthorizationRequest(context.Background(), p)
		require.NoError(t, err)
	})

	for name, body := range map[string]string{
		"plural without none":          `{"client_id": "` + oauthTestClientID + `", "client_name": "A", "redirect_uris": ["` + oauthTestRedirect + `"], "token_endpoint_auth_methods_supported": ["private_key_jwt"]}`,
		"singular private_key_jwt":     `{"client_id": "` + oauthTestClientID + `", "client_name": "A", "redirect_uris": ["` + oauthTestRedirect + `"], "token_endpoint_auth_method": "private_key_jwt"}`,
		"a client_id mismatch":         `{"client_id": "https://app.example.com/other.json", "client_name": "A", "redirect_uris": ["` + oauthTestRedirect + `"]}`,
		"a client_name over 200":       `{"client_id": "` + oauthTestClientID + `", "client_name": "` + strings.Repeat("a", 201) + `", "redirect_uris": ["` + oauthTestRedirect + `"]}`,
		"21 redirect URIs":             `{"client_id": "` + oauthTestClientID + `", "client_name": "A", "redirect_uris": ["` + strings.TrimSuffix(strings.Repeat(oauthTestRedirect+`", "`, 21), `", "`) + `"]}`,
		"a redirect URI with fragment": `{"client_id": "` + oauthTestClientID + `", "client_name": "A", "redirect_uris": ["` + oauthTestRedirect + `#x"]}`,
		"not JSON":                     `<html></html>`,
	} {
		t.Run(name+" is refused and remembered", func(t *testing.T) {
			e := newOAuthTestEnv(t)
			e.fetcher.documents[oauthTestClientID] = &OAuthClientDocument{Body: []byte(body)}
			_, err := e.svc.CreateAuthorizationRequest(context.Background(), e.params())
			requireAuthorizeReason(t, err, domain.OAuthAuthorizeClientMetadata)
			client, ok := e.repo.client(oauthTestClientID)
			require.True(t, ok)
			require.NotNil(t, client.FetchFailedUntil)
			assert.Equal(t, e.now().Add(5*time.Minute), *client.FetchFailedUntil)
		})
	}

	t.Run("a document with neither authentication field is accepted as none", func(t *testing.T) {
		e := newOAuthTestEnv(t)
		e.authorize()
	})
}

// A failed fetch is cached for 5 minutes, then the document is fetched again.
func TestOAuthService_NegativeCache(t *testing.T) {
	e := newOAuthTestEnv(t)
	e.fetcher.errs[oauthTestClientID] = errors.New("the client metadata document answered 302, not 200")
	_, err := e.svc.CreateAuthorizationRequest(context.Background(), e.params())
	requireAuthorizeReason(t, err, domain.OAuthAuthorizeClientMetadata)
	assert.Contains(t, err.Error(), "302")
	assert.Equal(t, 1, e.fetcher.count())

	delete(e.fetcher.errs, oauthTestClientID)
	e.advance(5*time.Minute - time.Nanosecond)
	_, err = e.svc.CreateAuthorizationRequest(context.Background(), e.params())
	requireAuthorizeReason(t, err, domain.OAuthAuthorizeClientMetadata)
	assert.Equal(t, 1, e.fetcher.count(), "no fetch while the failure is cached")

	e.advance(time.Nanosecond)
	e.authorize()
	assert.Equal(t, 2, e.fetcher.count())
	client, _ := e.repo.client(oauthTestClientID)
	assert.Nil(t, client.FetchFailedUntil)
}

// A fetched document is cached for what its headers say, within 5 min to 24 h.
func TestOAuthService_PositiveCacheBounds(t *testing.T) {
	for header, ttl := range map[string]time.Duration{
		"max-age=60":     5 * time.Minute,
		"max-age=3600":   time.Hour,
		"max-age=172800": 24 * time.Hour,
		"no-store":       5 * time.Minute,
		"":               5 * time.Minute,
	} {
		e := newOAuthTestEnv(t)
		e.fetcher.documents[oauthTestClientID].Header = http.Header{"Cache-Control": {header}}
		e.authorize()
		client, _ := e.repo.client(oauthTestClientID)
		require.NotNil(t, client.CacheExpiresAt, header)
		assert.Equal(t, e.now().Add(ttl), *client.CacheExpiresAt, header)

		e.advance(ttl - time.Nanosecond)
		e.authorize()
		assert.Equal(t, 1, e.fetcher.count(), "%q: cached until its expiry", header)
		e.advance(time.Nanosecond)
		e.authorize()
		assert.Equal(t, 2, e.fetcher.count(), "%q: fetched again at its expiry", header)
	}
}

func TestOAuthClientCacheTTL(t *testing.T) {
	now := time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)
	for name, tc := range map[string]struct {
		header http.Header
		want   time.Duration
	}{
		"max-age within bounds":     {http.Header{"Cache-Control": {"public, max-age=7200"}}, 2 * time.Hour},
		"max-age below 5 minutes":   {http.Header{"Cache-Control": {"max-age=299"}}, 5 * time.Minute},
		"max-age at 5 minutes":      {http.Header{"Cache-Control": {"max-age=300"}}, 5 * time.Minute},
		"max-age at 24 hours":       {http.Header{"Cache-Control": {"max-age=86400"}}, 24 * time.Hour},
		"max-age above 24 hours":    {http.Header{"Cache-Control": {"max-age=86401"}}, 24 * time.Hour},
		"an enormous max-age":       {http.Header{"Cache-Control": {"max-age=999999999999999"}}, 24 * time.Hour},
		"no-cache":                  {http.Header{"Cache-Control": {"no-cache, max-age=7200"}}, 5 * time.Minute},
		"Expires":                   {http.Header{"Expires": {now.Add(3 * time.Hour).Format(http.TimeFormat)}}, 3 * time.Hour},
		"Expires in the past":       {http.Header{"Expires": {now.Add(-time.Hour).Format(http.TimeFormat)}}, 5 * time.Minute},
		"max-age wins over Expires": {http.Header{"Cache-Control": {"max-age=3600"}, "Expires": {now.Add(3 * time.Hour).Format(http.TimeFormat)}}, time.Hour},
		"nothing":                   {http.Header{}, 5 * time.Minute},
	} {
		assert.Equal(t, tc.want, oauthClientCacheTTL(tc.header, now), name)
	}
}

// Records that never led to a grant are capped at 10 000; the known clients,
// and a client already on record, are outside the cap.
func TestOAuthService_ClientCap(t *testing.T) {
	e := newOAuthTestEnv(t)
	atCap := 10000
	e.repo.grantlessOverride = &atCap

	_, err := e.svc.CreateAuthorizationRequest(context.Background(), e.params())
	requireAuthorizeReason(t, err, domain.OAuthAuthorizeClientCap)
	assert.Zero(t, e.fetcher.count(), "no fetch at the cap")
	_, recorded := e.repo.client(oauthTestClientID)
	assert.False(t, recorded, "no record at the cap")

	claude := "https://claude.ai/oauth/claude-code-client-metadata"
	e.fetcher.documents[claude] = oauthDocument(claude, "Claude Code", "http://localhost/callback")
	p := e.params()
	p.ClientID, p.RedirectURI = claude, "http://localhost:9999/callback"
	_, err = e.svc.CreateAuthorizationRequest(context.Background(), p)
	require.NoError(t, err, "a known client is outside the cap")

	belowCap := 9999
	e.repo.grantlessOverride = &belowCap
	e.authorize()
	e.repo.grantlessOverride = &atCap
	e.advance(2 * time.Hour)
	e.authorize()
	assert.Equal(t, 3, e.fetcher.count(), "a client on record is fetched again at the cap")
}

// A known client is any path on a known host, its host compared without case
// on ASCII letters only. Past the cap, only a document the host serves gets a
// record: a failed fetch is not recorded, and below the cap it is recorded and
// counts.
func TestOAuthService_ClientCap_KnownHosts(t *testing.T) {
	atCap := 10000
	authorize := func(e *oauthTestEnv, clientID string) error {
		p := e.params()
		p.ClientID, p.RedirectURI = clientID, "http://localhost:9999/callback"
		_, err := e.svc.CreateAuthorizationRequest(context.Background(), p)
		return err
	}

	t.Run("a mixed-case known host with a document is outside the cap", func(t *testing.T) {
		e := newOAuthTestEnv(t)
		e.repo.grantlessOverride = &atCap
		clientID := "https://Claude.AI/oauth/claude-code-client-metadata"
		e.fetcher.documents[clientID] = oauthDocument(clientID, "Claude Code", "http://localhost/callback")
		require.NoError(t, authorize(e, clientID))
		_, recorded := e.repo.client(clientID)
		assert.True(t, recorded)
	})

	t.Run("a look-alike host is inside the cap", func(t *testing.T) {
		e := newOAuthTestEnv(t)
		e.repo.grantlessOverride = &atCap
		for _, clientID := range []string{
			"https://claude.a\u0130/x",
			"https://claude.ai.evil.example/x",
			"https://claude.ai:8443/x",
		} {
			requireAuthorizeReason(t, authorize(e, clientID), domain.OAuthAuthorizeClientCap)
		}
		assert.Zero(t, e.fetcher.count())
	})

	t.Run("past the cap, a failed fetch on a known host records nothing", func(t *testing.T) {
		e := newOAuthTestEnv(t)
		e.repo.grantlessOverride = &atCap
		for _, clientID := range []string{"https://claude.ai/arbitrary-1", "https://CHATGPT.com/arbitrary-2"} {
			e.fetcher.errs[clientID] = errors.New("404")
			requireAuthorizeReason(t, authorize(e, clientID), domain.OAuthAuthorizeClientMetadata)
			_, recorded := e.repo.client(clientID)
			assert.False(t, recorded, clientID)
		}
		assert.Zero(t, e.repo.called("MarkClientFetchFailed"))
	})

	t.Run("below the cap, a failed fetch on a known host is recorded and counts", func(t *testing.T) {
		e := newOAuthTestEnv(t)
		clientID := "https://claude.ai/arbitrary"
		e.fetcher.errs[clientID] = errors.New("404")
		requireAuthorizeReason(t, authorize(e, clientID), domain.OAuthAuthorizeClientMetadata)
		_, recorded := e.repo.client(clientID)
		require.True(t, recorded)
		count, err := e.repo.CountGrantlessClients(context.Background(), domain.OAuthKnownClientHosts)
		require.NoError(t, err)
		assert.Equal(t, 1, count)
	})
}

// The token endpoint uses the snapshot taken at authorization: the document
// may be gone, the exchange and the refresh never fetch it.
func TestOAuthService_NoFetchAtTheTokenEndpoint(t *testing.T) {
	e := newOAuthTestEnv(t)
	code := e.approve(e.authorize())
	require.Equal(t, 1, e.fetcher.count())
	e.fetcher.errs[oauthTestClientID] = errors.New("gone")

	tokens, err := e.svc.ExchangeCode(context.Background(), e.exchangeRequest(code), oauthTestMeta)
	require.NoError(t, err)
	_, err = e.refresh(tokens.RefreshToken)
	require.NoError(t, err)
	assert.Equal(t, 1, e.fetcher.count())
}

// The authorization page

func TestOAuthService_GetAuthorizationRequest(t *testing.T) {
	e := newOAuthTestEnv(t)
	p := e.params()
	p.RedirectURI = "http://localhost:4000/callback"
	started, err := e.svc.CreateAuthorizationRequest(context.Background(), p)
	require.NoError(t, err)

	view, err := e.svc.GetAuthorizationRequest(context.Background(), started.RequestID)
	require.NoError(t, err)
	assert.Equal(t, "Example App", view.ClientName)
	assert.Equal(t, "app.example.com", view.ClientHost)
	assert.False(t, view.KnownClient, "the page cautions about an unknown host")
	assert.Equal(t, "localhost:4000", view.RedirectHost)
	assert.True(t, view.LoopbackRedirect)
	assert.Empty(t, view.OwnedWorkspaces, "no session, no shortcut")

	view, err = e.svc.GetAuthorizationRequest(oauthSessionCtx(oauthTestOwnerID), started.RequestID)
	require.NoError(t, err)
	assert.ElementsMatch(t, []domain.OAuthOwnedWorkspace{{ID: "ws1", Name: "Acme"}, {ID: "ws-off", Name: "Off", MCPDisabled: true}}, view.OwnedWorkspaces)

	view, err = e.svc.GetAuthorizationRequest(oauthSessionCtx(oauthTestRootID), started.RequestID)
	require.NoError(t, err)
	assert.Equal(t, []domain.OAuthOwnedWorkspace{{ID: "ws1", Name: "Acme"}, {ID: "ws2", Name: "Other"}, {ID: "ws-off", Name: "Off", MCPDisabled: true}}, view.OwnedWorkspaces, "root owns every workspace")

	view, err = e.svc.GetAuthorizationRequest(oauthSessionCtx(oauthTestMemberID), started.RequestID)
	require.NoError(t, err)
	assert.Empty(t, view.OwnedWorkspaces)
	view, err = e.svc.GetAuthorizationRequest(oauthKeyCtx(oauthTestKeyID), started.RequestID)
	require.NoError(t, err)
	assert.Empty(t, view.OwnedWorkspaces)

	_, err = e.svc.GetAuthorizationRequest(context.Background(), "unknown")
	assert.ErrorIs(t, err, domain.ErrOAuthRequestUnavailable)
	e.advance(10 * time.Minute)
	_, err = e.svc.GetAuthorizationRequest(context.Background(), started.RequestID)
	assert.ErrorIs(t, err, domain.ErrOAuthRequestUnavailable)
}

func TestOAuthService_KnownClientOnThePage(t *testing.T) {
	e := newOAuthTestEnv(t)
	claude := "https://claude.ai/oauth/claude-code-client-metadata"
	e.fetcher.documents[claude] = oauthDocument(claude, "Claude Code", "http://localhost/callback")
	p := e.params()
	p.ClientID, p.RedirectURI = claude, "http://localhost:9999/callback"
	started, err := e.svc.CreateAuthorizationRequest(context.Background(), p)
	require.NoError(t, err)
	view, err := e.svc.GetAuthorizationRequest(context.Background(), started.RequestID)
	require.NoError(t, err)
	assert.True(t, view.KnownClient)
	assert.Equal(t, "claude.ai", view.ClientHost)
}

// oauth.approveWithKey

func TestOAuthService_ApproveWithKey(t *testing.T) {
	e := newOAuthTestEnv(t)
	started := e.authorize()
	redirect, err := e.svc.ApproveWithKey(context.Background(), domain.OAuthApproveWithKeyRequest{RequestID: started.RequestID, APIKey: oauthTestKeyToken, BrowserBinding: started.BrowserBinding}, oauthTestMeta)
	require.NoError(t, err)

	assert.True(t, strings.HasPrefix(redirect.RedirectURL, oauthTestRedirect+"?"))
	params := redirectParams(t, redirect)
	assert.Equal(t, "st-1", params.Get("state"))
	assert.Equal(t, oauthTestIssuer, params.Get("iss"), "iss is the issuer byte for byte")
	assert.Len(t, params.Get("code"), 43, "32 random bytes, base64url")

	grant := e.repo.onlyGrant()
	assert.Equal(t, oauthTestKeyID, grant.APIKeyUserID)
	assert.Equal(t, "ws1", grant.WorkspaceID)
	assert.Equal(t, oauthTestClientID, grant.ClientID)
	assert.Equal(t, "Example App", grant.ClientName)
	assert.Equal(t, oauthTestResource, grant.Resource)
	assert.Equal(t, domain.OAuthGrantViaKey, grant.CreatedVia)
	assert.Empty(t, grant.CreatedByUserID)
	assert.NotNil(t, e.repo.request(started.RequestID).ConsumedAt)

	code := e.repo.codes[domain.HashOAuthSecret(params.Get("code"))]
	assert.Equal(t, grant.ID, code.GrantID)
	assert.Equal(t, e.now().Add(60*time.Second), code.ExpiresAt)

	rows := e.recorder.all()
	require.Len(t, rows, 1)
	assert.Equal(t, "oauth.approveWithKey", rows[0].Action)
	assert.Equal(t, domain.AuditCategoryAPIKeys, rows[0].Category)
	assert.Equal(t, "ws1", rows[0].WorkspaceID)
	assert.Equal(t, domain.AuditActorAPIKey, rows[0].ActorType)
	assert.Equal(t, oauthTestKeyID, rows[0].ActorID)
	assert.Equal(t, domain.AuditAuthAPIKey, rows[0].AuthMethod)
	assert.Equal(t, domain.AuditTargetOAuthGrant, rows[0].TargetType)
	assert.Equal(t, grant.ID, rows[0].TargetID)
	assert.Equal(t, "Example App", rows[0].Metadata["oauth_client"])
	assert.Equal(t, "203.0.113.9", rows[0].IPAddress)

	// The key is the proof, never kept: no stored row and no audit row holds it.
	stored, err := json.Marshal([]any{e.repo.clients, e.repo.requests, e.repo.grants, e.repo.codes, e.repo.tokens, e.repo.keys, rows})
	require.NoError(t, err)
	assert.NotContains(t, string(stored), oauthTestKeyToken)
	assert.NotContains(t, string(stored), "the-pasted-key")
	assert.NotContains(t, redirect.RedirectURL, "the-pasted-key")
}

func TestOAuthService_ApproveWithKey_Refusals(t *testing.T) {
	e := newOAuthTestEnv(t)
	e.auth.apiKeys["revoked-key"] = "deleted-key"
	e.auth.accounts["off-key"] = &domain.User{ID: "off-key", Type: domain.UserTypeAPIKey}
	e.auth.apiKeys["off-key-token"] = "off-key"
	e.auth.memberships["off-key"] = map[string]*domain.UserWorkspace{"ws-off": {UserID: "off-key", WorkspaceID: "ws-off", Role: "member"}}

	approve := func(started *domain.OAuthAuthorizationStarted, key, binding string) error {
		_, err := e.svc.ApproveWithKey(context.Background(), domain.OAuthApproveWithKeyRequest{RequestID: started.RequestID, APIKey: key, BrowserBinding: binding}, oauthTestMeta)
		return err
	}
	for name, key := range map[string]string{
		"a revoked key":           "revoked-key",
		"a console session token": "eyJ.session.token",
		"an invitation token":     "eyJ.invitation.token",
		"a malformed string":      "hello",
		"an empty string":         "",
	} {
		started := e.authorize()
		assert.ErrorIs(t, approve(started, key, started.BrowserBinding), domain.ErrOAuthNotAnAPIKey, name)
		assert.Nil(t, e.repo.request(started.RequestID).ConsumedAt, "%s: the request stays usable", name)
	}

	started := e.authorize()
	assert.ErrorIs(t, approve(started, "off-key-token", started.BrowserBinding), domain.ErrOAuthWorkspaceMCPDisabled)

	assert.ErrorIs(t, approve(started, oauthTestKeyToken, ""), domain.ErrOAuthBrowserMismatch, "no cookie")
	other := e.authorize()
	assert.ErrorIs(t, approve(started, oauthTestKeyToken, other.BrowserBinding), domain.ErrOAuthBrowserMismatch, "another request's cookie")

	require.NoError(t, approve(started, oauthTestKeyToken, started.BrowserBinding))
	assert.ErrorIs(t, approve(started, oauthTestKeyToken, started.BrowserBinding), domain.ErrOAuthRequestUnavailable, "approve replay")

	denied := e.authorize()
	_, err := e.svc.Deny(context.Background(), domain.OAuthDenyRequest{RequestID: denied.RequestID, BrowserBinding: denied.BrowserBinding})
	require.NoError(t, err)
	assert.ErrorIs(t, approve(denied, oauthTestKeyToken, denied.BrowserBinding), domain.ErrOAuthRequestUnavailable, "approve after deny")

	grants, _ := e.repo.counts()
	assert.Equal(t, 1, grants)
}

func TestOAuthService_RequestExpiresAfterTenMinutes(t *testing.T) {
	e := newOAuthTestEnv(t)
	started := e.authorize()
	e.advance(10*time.Minute - time.Nanosecond)
	late := e.authorize()
	e.approve(started)

	e.advance(10 * time.Minute)
	_, err := e.svc.ApproveWithKey(context.Background(), domain.OAuthApproveWithKeyRequest{RequestID: late.RequestID, APIKey: oauthTestKeyToken, BrowserBinding: late.BrowserBinding}, oauthTestMeta)
	assert.ErrorIs(t, err, domain.ErrOAuthRequestUnavailable)
}

func TestOAuthService_RedirectKeepsTheClientsQuery(t *testing.T) {
	e := newOAuthTestEnv(t)
	e.fetcher.documents[oauthTestClientID] = oauthDocument(oauthTestClientID, "Example App", oauthTestRedirect+"?tenant=a%20b")
	p := e.params()
	p.RedirectURI = oauthTestRedirect + "?tenant=a%20b"
	p.State = ""
	started, err := e.svc.CreateAuthorizationRequest(context.Background(), p)
	require.NoError(t, err)
	redirect, err := e.svc.ApproveWithKey(context.Background(), domain.OAuthApproveWithKeyRequest{RequestID: started.RequestID, APIKey: oauthTestKeyToken, BrowserBinding: started.BrowserBinding}, oauthTestMeta)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(redirect.RedirectURL, oauthTestRedirect+"?tenant=a%20b&code="))
	params := redirectParams(t, redirect)
	assert.False(t, params.Has("state"), "no state when the client sent none")
	assert.Equal(t, oauthTestIssuer, params.Get("iss"))
}

// oauth.deny

func TestOAuthService_Deny(t *testing.T) {
	e := newOAuthTestEnv(t)
	started := e.authorize()

	_, err := e.svc.Deny(context.Background(), domain.OAuthDenyRequest{RequestID: started.RequestID})
	assert.ErrorIs(t, err, domain.ErrOAuthBrowserMismatch)

	redirect, err := e.svc.Deny(context.Background(), domain.OAuthDenyRequest{RequestID: started.RequestID, BrowserBinding: started.BrowserBinding})
	require.NoError(t, err)
	params := redirectParams(t, redirect)
	assert.Equal(t, url.Values{"error": {"access_denied"}, "state": {"st-1"}, "iss": {oauthTestIssuer}}, params)
	assert.NotNil(t, e.repo.request(started.RequestID).ConsumedAt)

	_, err = e.svc.Deny(context.Background(), domain.OAuthDenyRequest{RequestID: started.RequestID, BrowserBinding: started.BrowserBinding})
	assert.ErrorIs(t, err, domain.ErrOAuthRequestUnavailable, "deny replay")
	assert.Empty(t, e.recorder.all(), "a denial creates nothing and records nothing")
}

// oauth.createKeyAndApprove

func (e *oauthTestEnv) createKey(ctx context.Context, started *domain.OAuthAuthorizationStarted, workspaceID string, permissions domain.UserPermissions) (*domain.OAuthRedirect, error) {
	return e.svc.CreateKeyAndApprove(ctx, domain.OAuthCreateKeyAndApproveRequest{RequestID: started.RequestID, WorkspaceID: workspaceID, Name: "claude-agent", Permissions: permissions, BrowserBinding: started.BrowserBinding}, oauthTestMeta)
}

func TestOAuthService_CreateKeyAndApprove(t *testing.T) {
	e := newOAuthTestEnv(t)
	started := e.authorize()
	redirect, err := e.createKey(oauthSessionCtx(oauthTestOwnerID), started, "ws1", nil)
	require.NoError(t, err)

	grants, keys := e.repo.counts()
	require.Equal(t, 1, grants)
	require.Equal(t, 1, keys)
	grant := e.repo.onlyGrant()
	key := e.repo.keys[grant.APIKeyUserID]
	assert.Equal(t, domain.UserTypeAPIKey, key.Type)
	assert.Equal(t, "claude-agent@notifuse.example.com", key.Email)
	membership := e.repo.memberships[key.ID]
	assert.Equal(t, "ws1", membership.WorkspaceID)
	assert.Equal(t, "member", membership.Role)
	assert.Equal(t, domain.NewFullPermissions(), membership.Permissions, "no permissions means full access")
	assert.Equal(t, domain.OAuthGrantViaOwnerShortcut, grant.CreatedVia)
	assert.Equal(t, oauthTestOwnerID, grant.CreatedByUserID)
	assert.Equal(t, "ws1", grant.WorkspaceID)
	assert.Zero(t, e.auth.minted, "the key's token is never minted")
	assert.NotNil(t, e.repo.request(started.RequestID).ConsumedAt)

	params := redirectParams(t, redirect)
	assert.Equal(t, oauthTestIssuer, params.Get("iss"))
	_, err = e.svc.ExchangeCode(context.Background(), e.exchangeRequest(params.Get("code")), oauthTestMeta)
	require.NoError(t, err)

	rows := e.recorder.all()
	require.Len(t, rows, 2)
	assert.Equal(t, "workspaces.createAPIKey", rows[0].Action)
	assert.Equal(t, domain.AuditTargetAPIKey, rows[0].TargetType)
	assert.Equal(t, key.ID, rows[0].TargetID)
	assert.Equal(t, "oauth.createKeyAndApprove", rows[1].Action)
	assert.Equal(t, grant.ID, rows[1].TargetID)
	for _, row := range rows {
		assert.Equal(t, oauthTestOwnerID, row.ActorID)
		assert.Equal(t, domain.AuditActorUser, row.ActorType)
		assert.Equal(t, domain.AuditRoleOwner, row.ActorRole)
		assert.Equal(t, "ws1", row.WorkspaceID)
	}
}

func TestOAuthService_CreateKeyAndApprove_RootCountsAsOwner(t *testing.T) {
	e := newOAuthTestEnv(t)
	_, err := e.createKey(oauthSessionCtx(oauthTestRootID), e.authorize(), "ws2", nil)
	require.NoError(t, err)
	assert.Equal(t, oauthTestRootID, e.repo.onlyGrant().CreatedByUserID)
}

// A refusal, wherever it comes from, creates no key, no grant and leaves the
// request usable unless it was the request itself that failed.
func TestOAuthService_CreateKeyAndApprove_CreatesNoKeyOnRefusal(t *testing.T) {
	restricted := domain.UserPermissions{domain.PermissionResourceContacts: {Read: true}}
	for name, tc := range map[string]struct {
		ctx   func() context.Context
		setup func(e *oauthTestEnv, started *domain.OAuthAuthorizationStarted) *domain.OAuthAuthorizationStarted
		ws    string
		perms domain.UserPermissions
		check func(t *testing.T, err error)
	}{
		"a member": {ctx: func() context.Context { return oauthSessionCtx(oauthTestMemberID) }, ws: "ws1", check: func(t *testing.T, err error) {
			var unauthorized *domain.ErrUnauthorized
			assert.ErrorAs(t, err, &unauthorized)
		}},
		"no session": {ctx: context.Background, ws: "ws1", check: func(t *testing.T, err error) {
			var unauthorized *domain.ErrUnauthorized
			assert.ErrorAs(t, err, &unauthorized)
		}},
		"an expired session": {ctx: func() context.Context {
			ctx := oauthSessionCtx(oauthTestOwnerID)
			return context.WithValue(ctx, domain.UserIDKey, "ghost")
		}, ws: "ws1", check: func(t *testing.T, err error) {
			assert.ErrorIs(t, err, domain.ErrSessionAuthFailed, "answered 401, as every workspace route answers a dead session")
		}},
		"an API key": {ctx: func() context.Context { return oauthKeyCtx(oauthTestKeyID) }, ws: "ws1", check: func(t *testing.T, err error) {
			var unauthorized *domain.ErrUnauthorized
			assert.ErrorAs(t, err, &unauthorized)
		}},
		"another workspace's owner": {ctx: func() context.Context { return oauthSessionCtx(oauthTestOwnerID) }, ws: "ws2", check: func(t *testing.T, err error) {
			var unauthorized *domain.ErrUnauthorized
			assert.ErrorAs(t, err, &unauthorized)
		}},
		"a restricted key without rbac": {ctx: func() context.Context { return oauthSessionCtx(oauthTestOwnerID) }, ws: "ws1", perms: restricted, check: func(t *testing.T, err error) {
			var notLicensed *domain.ErrFeatureNotLicensed
			assert.ErrorAs(t, err, &notLicensed)
		}},
		"a switched-off workspace": {ctx: func() context.Context { return oauthSessionCtx(oauthTestOwnerID) }, ws: "ws-off", check: func(t *testing.T, err error) {
			assert.ErrorIs(t, err, domain.ErrOAuthWorkspaceMCPDisabled)
		}},
		"an expired request": {ctx: func() context.Context { return oauthSessionCtx(oauthTestOwnerID) }, ws: "ws1",
			setup: func(e *oauthTestEnv, started *domain.OAuthAuthorizationStarted) *domain.OAuthAuthorizationStarted {
				e.advance(10 * time.Minute)
				return started
			}, check: func(t *testing.T, err error) { assert.ErrorIs(t, err, domain.ErrOAuthRequestUnavailable) }},
		"a consumed request": {ctx: func() context.Context { return oauthSessionCtx(oauthTestOwnerID) }, ws: "ws1",
			setup: func(e *oauthTestEnv, started *domain.OAuthAuthorizationStarted) *domain.OAuthAuthorizationStarted {
				_, err := e.svc.Deny(context.Background(), domain.OAuthDenyRequest{RequestID: started.RequestID, BrowserBinding: started.BrowserBinding})
				require.NoError(e.t, err)
				return started
			}, check: func(t *testing.T, err error) { assert.ErrorIs(t, err, domain.ErrOAuthRequestUnavailable) }},
		"a missing request cookie": {ctx: func() context.Context { return oauthSessionCtx(oauthTestOwnerID) }, ws: "ws1",
			setup: func(e *oauthTestEnv, started *domain.OAuthAuthorizationStarted) *domain.OAuthAuthorizationStarted {
				return &domain.OAuthAuthorizationStarted{RequestID: started.RequestID}
			}, check: func(t *testing.T, err error) { assert.ErrorIs(t, err, domain.ErrOAuthBrowserMismatch) }},
		"a foreign request cookie": {ctx: func() context.Context { return oauthSessionCtx(oauthTestOwnerID) }, ws: "ws1",
			setup: func(e *oauthTestEnv, started *domain.OAuthAuthorizationStarted) *domain.OAuthAuthorizationStarted {
				return &domain.OAuthAuthorizationStarted{RequestID: started.RequestID, BrowserBinding: e.authorize().BrowserBinding}
			}, check: func(t *testing.T, err error) { assert.ErrorIs(t, err, domain.ErrOAuthBrowserMismatch) }},
	} {
		t.Run(name, func(t *testing.T) {
			e := newOAuthTestEnv(t)
			started := e.authorize()
			if tc.setup != nil {
				started = tc.setup(e, started)
			}
			_, err := e.createKey(tc.ctx(), started, tc.ws, tc.perms)
			require.Error(t, err)
			tc.check(t, err)
			grants, keys := e.repo.counts()
			assert.Zero(t, grants)
			assert.Zero(t, keys)
			assert.Zero(t, e.repo.called("CreateAPIKeyAccount"), "no key creation was attempted")
			assert.Empty(t, e.recorder.all())
		})
	}
}

func TestOAuthService_CreateKeyAndApprove_RestrictedKeyWithRBAC(t *testing.T) {
	e := newOAuthTestEnv(t)
	e.licensed = true
	restricted := domain.UserPermissions{domain.PermissionResourceContacts: {Read: true}}
	_, err := e.createKey(oauthSessionCtx(oauthTestOwnerID), e.authorize(), "ws1", restricted)
	require.NoError(t, err)
	grant := e.repo.onlyGrant()
	assert.Equal(t, restricted, e.repo.memberships[grant.APIKeyUserID].Permissions)
}

// One transaction: a failure after the key's rows were written rolls them back
// with the grant, and leaves the request unconsumed.
func TestOAuthService_CreateKeyAndApprove_RollsBackTheKey(t *testing.T) {
	e := newOAuthTestEnv(t)
	e.repo.failOn["CreateCode"] = errors.New("disk full")
	started := e.authorize()
	_, err := e.createKey(oauthSessionCtx(oauthTestOwnerID), started, "ws1", nil)
	require.ErrorContains(t, err, "disk full")
	assert.Equal(t, 1, e.repo.called("CreateAPIKeyAccount"))
	grants, keys := e.repo.counts()
	assert.Zero(t, grants)
	assert.Zero(t, keys)
	assert.Nil(t, e.repo.request(started.RequestID).ConsumedAt)
	assert.Empty(t, e.recorder.all())
}

// Key addresses are unique across the deployment: a name already taken is
// ErrOAuthKeyNameTaken, which the handler answers with 409, and nothing is
// created.
func TestOAuthService_CreateKeyAndApprove_NameTaken(t *testing.T) {
	e := newOAuthTestEnv(t)
	_, err := e.createKey(oauthSessionCtx(oauthTestOwnerID), e.authorize(), "ws1", nil)
	require.NoError(t, err)
	recorded := len(e.recorder.all())

	started := e.authorize()
	_, err = e.createKey(oauthSessionCtx(oauthTestOwnerID), started, "ws1", nil)
	require.ErrorIs(t, err, domain.ErrOAuthKeyNameTaken)
	assert.Contains(t, err.Error(), "pick another name")
	grants, keys := e.repo.counts()
	assert.Equal(t, 1, grants)
	assert.Equal(t, 1, keys)
	assert.Nil(t, e.repo.request(started.RequestID).ConsumedAt, "the request stays usable for another name")
	assert.Len(t, e.recorder.all(), recorded)
}

func TestOAuthService_CreateKeyAndApprove_InvalidName(t *testing.T) {
	e := newOAuthTestEnv(t)
	started := e.authorize()
	_, err := e.svc.CreateKeyAndApprove(oauthSessionCtx(oauthTestOwnerID), domain.OAuthCreateKeyAndApproveRequest{RequestID: started.RequestID, WorkspaceID: "ws1", Name: "Claude Agent!", BrowserBinding: started.BrowserBinding}, oauthTestMeta)
	var validation domain.ValidationError
	require.ErrorAs(t, err, &validation)
	_, keys := e.repo.counts()
	assert.Zero(t, keys)
}

// The code exchange

func TestOAuthService_ExchangeCode(t *testing.T) {
	e := newOAuthTestEnv(t)
	tokens := e.connect()

	assert.True(t, strings.HasPrefix(tokens.AccessToken, "nfmcp_at_"))
	assert.True(t, strings.HasPrefix(tokens.RefreshToken, "nfmcp_rt_"))
	assert.Len(t, strings.TrimPrefix(tokens.AccessToken, "nfmcp_at_"), 43, "32 random bytes")
	assert.Len(t, strings.TrimPrefix(tokens.RefreshToken, "nfmcp_rt_"), 43, "32 random bytes")
	assert.Equal(t, "Bearer", tokens.TokenType)
	assert.Equal(t, 3600, tokens.ExpiresIn)
	assert.Equal(t, "mcp", tokens.Scope)

	access, refresh := e.repo.token(tokens.AccessToken), e.repo.token(tokens.RefreshToken)
	assert.Equal(t, domain.OAuthTokenKindAccess, access.Kind)
	assert.Equal(t, domain.OAuthTokenKindRefresh, refresh.Kind)
	assert.Equal(t, access.PairID, refresh.PairID, "issued together, one pair")
	assert.Equal(t, e.now().Add(time.Hour), access.ExpiresAt)
	assert.Equal(t, e.now().Add(90*24*time.Hour), refresh.ExpiresAt)
	stored, _ := json.Marshal(e.repo.tokens)
	assert.NotContains(t, string(stored), tokens.AccessToken, "tokens are stored hashed")
	assert.NotContains(t, string(stored), tokens.RefreshToken)

	access2, err := e.svc.ResolveAccessToken(context.Background(), tokens.AccessToken)
	require.NoError(t, err)
	assert.Equal(t, oauthTestKeyID, access2.Key.ID)
	assert.Equal(t, domain.UserTypeAPIKey, access2.Key.Type)
}

func TestOAuthService_ExchangeCode_Checks(t *testing.T) {
	for name, tc := range map[string]func(*domain.OAuthCodeExchange){
		"another client_id":             func(r *domain.OAuthCodeExchange) { r.ClientID = "https://evil.example/client.json" },
		"another redirect_uri":          func(r *domain.OAuthCodeExchange) { r.RedirectURI = oauthTestRedirect + "/" },
		"a wrong verifier":              func(r *domain.OAuthCodeExchange) { r.CodeVerifier = strings.Repeat("a", 43) },
		"a verifier outside the syntax": func(r *domain.OAuthCodeExchange) { r.CodeVerifier = "short" },
		"another resource":              func(r *domain.OAuthCodeExchange) { r.Resource = "https://evil.example/mcp" },
		"an unknown code":               func(r *domain.OAuthCodeExchange) { r.Code = "unknown" },
	} {
		t.Run(name, func(t *testing.T) {
			e := newOAuthTestEnv(t)
			request := e.exchangeRequest(e.approve(e.authorize()))
			tc(&request)
			_, err := e.svc.ExchangeCode(context.Background(), request, oauthTestMeta)
			requireInvalidGrant(t, err)
			assert.Empty(t, e.repo.tokens)
		})
	}

	t.Run("the canonical resource is accepted, and an omitted one", func(t *testing.T) {
		e := newOAuthTestEnv(t)
		request := e.exchangeRequest(e.approve(e.authorize()))
		request.Resource = "HTTPS://notifuse.example.com:443/mcp"
		_, err := e.svc.ExchangeCode(context.Background(), request, oauthTestMeta)
		require.NoError(t, err)
	})

	t.Run("a redirect on another loopback port is not the authorized one", func(t *testing.T) {
		e := newOAuthTestEnv(t)
		p := e.params()
		p.RedirectURI = "http://localhost:4000/callback"
		started, err := e.svc.CreateAuthorizationRequest(context.Background(), p)
		require.NoError(t, err)
		request := e.exchangeRequest(e.approve(started))
		request.RedirectURI = "http://localhost:4001/callback"
		_, err = e.svc.ExchangeCode(context.Background(), request, oauthTestMeta)
		requireInvalidGrant(t, err)
	})

	t.Run("missing parameters", func(t *testing.T) {
		e := newOAuthTestEnv(t)
		_, err := e.svc.ExchangeCode(context.Background(), domain.OAuthCodeExchange{Code: "x", ClientID: oauthTestClientID}, oauthTestMeta)
		var oauthErr *domain.OAuthError
		require.ErrorAs(t, err, &oauthErr)
		assert.Equal(t, domain.OAuthErrInvalidRequest, oauthErr.Code)
	})

	t.Run("after an API_ENDPOINT change", func(t *testing.T) {
		e := newOAuthTestEnv(t)
		request := e.exchangeRequest(e.approve(e.authorize()))
		e.apiEndpoint = "https://new.example.com"
		_, err := e.svc.ExchangeCode(context.Background(), request, oauthTestMeta)
		requireInvalidGrant(t, err)
	})
}

// A code is single use: presenting it again revokes what it granted.
func TestOAuthService_ExchangeCode_ReuseRevokesTheGrant(t *testing.T) {
	e := newOAuthTestEnv(t)
	request := e.exchangeRequest(e.approve(e.authorize()))
	tokens, err := e.svc.ExchangeCode(context.Background(), request, oauthTestMeta)
	require.NoError(t, err)

	_, err = e.svc.ExchangeCode(context.Background(), request, oauthTestMeta)
	requireInvalidGrant(t, err)
	grant := e.repo.onlyGrant()
	assert.True(t, grant.Revoked())
	assert.Equal(t, domain.OAuthRevokedCodeReuse, grant.RevokedReason)
	assert.False(t, e.resolves(tokens.AccessToken))
	_, err = e.refresh(tokens.RefreshToken)
	requireInvalidGrant(t, err)

	rows := e.recorder.all()
	last := rows[len(rows)-1]
	assert.Equal(t, "oauth.revokeGrant", last.Action)
	assert.Equal(t, domain.OAuthRevokedCodeReuse, last.Metadata["reason"])
	assert.Equal(t, domain.AuditAuthOAuth, last.AuthMethod)
	assert.Equal(t, oauthTestKeyID, last.ActorID)
	assert.Equal(t, "ws1", last.WorkspaceID)
}

func TestOAuthService_ExchangeCode_FailedCheckBurnsTheCode(t *testing.T) {
	e := newOAuthTestEnv(t)
	request := e.exchangeRequest(e.approve(e.authorize()))
	wrong := request
	wrong.CodeVerifier = strings.Repeat("b", 43)
	_, err := e.svc.ExchangeCode(context.Background(), wrong, oauthTestMeta)
	requireInvalidGrant(t, err)
	_, err = e.svc.ExchangeCode(context.Background(), request, oauthTestMeta)
	requireInvalidGrant(t, err)
	assert.True(t, e.repo.onlyGrant().Revoked())
}

func TestOAuthService_CodeExpiresAfterSixtySeconds(t *testing.T) {
	e := newOAuthTestEnv(t)
	onTime := e.exchangeRequest(e.approve(e.authorize()))
	late := e.exchangeRequest(e.approve(e.authorize()))
	e.advance(60*time.Second - time.Nanosecond)
	_, err := e.svc.ExchangeCode(context.Background(), onTime, oauthTestMeta)
	require.NoError(t, err)
	e.advance(time.Nanosecond)
	_, err = e.svc.ExchangeCode(context.Background(), late, oauthTestMeta)
	requireInvalidGrant(t, err)
	revoked := 0
	for _, grant := range e.repo.grants {
		if grant.Revoked() {
			revoked++
		}
	}
	assert.Zero(t, revoked, "an expired code is no reuse")
}

// Refresh

func TestOAuthService_Refresh_Rotates(t *testing.T) {
	e := newOAuthTestEnv(t)
	first := e.connect()
	e.advance(time.Minute)
	second, err := e.refresh(first.RefreshToken)
	require.NoError(t, err)

	assert.NotEqual(t, first.RefreshToken, second.RefreshToken)
	assert.False(t, e.resolves(first.AccessToken), "rotation invalidates the grant's live access tokens")
	assert.True(t, e.resolves(second.AccessToken))
	old := e.repo.token(first.RefreshToken)
	require.NotNil(t, old.RotatedAt)
	assert.Equal(t, domain.HashOAuthSecret(second.RefreshToken), old.ReplacedByHash)
	assert.Equal(t, e.now().Add(90*24*time.Hour), e.repo.token(second.RefreshToken).ExpiresAt, "90 days from its last use")
	assert.Equal(t, e.repo.token(second.AccessToken).PairID, e.repo.token(second.RefreshToken).PairID)
}

func TestOAuthService_Refresh_ReplacedTokenOutsideGraceRevokesTheGrant(t *testing.T) {
	e := newOAuthTestEnv(t)
	first := e.connect()
	second, err := e.refresh(first.RefreshToken)
	require.NoError(t, err)

	e.advance(60 * time.Second)
	_, err = e.refresh(first.RefreshToken)
	requireInvalidGrant(t, err)
	grant := e.repo.onlyGrant()
	assert.True(t, grant.Revoked(), "at exactly 60 s the grace is over")
	assert.Equal(t, domain.OAuthRevokedTokenReuse, grant.RevokedReason)
	assert.False(t, e.resolves(second.AccessToken))
	_, err = e.refresh(second.RefreshToken)
	requireInvalidGrant(t, err)

	rows := e.recorder.all()
	last := rows[len(rows)-1]
	assert.Equal(t, "oauth.revokeGrant", last.Action)
	assert.Equal(t, domain.OAuthRevokedTokenReuse, last.Metadata["reason"])
}

// A client that lost the answer to its refresh presents the old token again
// within 60 s: it gets a new pair, once, and the pair it never received goes.
func TestOAuthService_Refresh_Grace(t *testing.T) {
	e := newOAuthTestEnv(t)
	first := e.connect()
	lost, err := e.refresh(first.RefreshToken)
	require.NoError(t, err)

	e.advance(60*time.Second - time.Nanosecond)
	retried, err := e.refresh(first.RefreshToken)
	require.NoError(t, err)
	assert.False(t, e.repo.onlyGrant().Revoked())
	assert.True(t, e.resolves(retried.AccessToken))
	assert.False(t, e.resolves(lost.AccessToken), "the successor pair is invalidated")
	assert.NotNil(t, e.repo.token(lost.RefreshToken).InvalidatedAt)
	old := e.repo.token(first.RefreshToken)
	assert.Equal(t, e.now().Add(-(60*time.Second - time.Nanosecond)), *old.RotatedAt, "rotated_at never moves")

	t.Run("a second grace replay revokes the grant", func(t *testing.T) {
		_, err := e.refresh(first.RefreshToken)
		requireInvalidGrant(t, err)
		assert.True(t, e.repo.onlyGrant().Revoked())
		assert.False(t, e.resolves(retried.AccessToken))
	})
}

func TestOAuthService_Refresh_GraceReplayAfterTheSuccessorRotatedRevokes(t *testing.T) {
	e := newOAuthTestEnv(t)
	first := e.connect()
	second, err := e.refresh(first.RefreshToken)
	require.NoError(t, err)
	_, err = e.refresh(second.RefreshToken)
	require.NoError(t, err)

	e.advance(10 * time.Second)
	_, err = e.refresh(first.RefreshToken)
	requireInvalidGrant(t, err)
	assert.True(t, e.repo.onlyGrant().Revoked())
}

// An invalidated refresh token revokes the grant whenever it comes back.
func TestOAuthService_Refresh_InvalidatedTokenRevokesAtAnyTime(t *testing.T) {
	e := newOAuthTestEnv(t)
	first := e.connect()
	lost, err := e.refresh(first.RefreshToken)
	require.NoError(t, err)
	retried, err := e.refresh(first.RefreshToken)
	require.NoError(t, err)

	e.advance(30 * 24 * time.Hour)
	third, err := e.refresh(retried.RefreshToken)
	require.NoError(t, err)
	_, err = e.refresh(lost.RefreshToken)
	requireInvalidGrant(t, err)
	assert.True(t, e.repo.onlyGrant().Revoked())
	assert.False(t, e.resolves(third.AccessToken))
}

// Every refresh of one grant holds its lock: a token and its successor
// refreshed together leave one chain at most, and here none, since one of the
// two is a replay.
func TestOAuthService_Refresh_ConcurrentTokenAndSuccessor(t *testing.T) {
	for round := 0; round < 20; round++ {
		e := newOAuthTestEnv(t)
		first := e.connect()
		second, err := e.refresh(first.RefreshToken)
		require.NoError(t, err)

		var wg sync.WaitGroup
		results := make([]error, 2)
		for i, token := range []string{first.RefreshToken, second.RefreshToken} {
			wg.Add(1)
			go func(i int, token string) {
				defer wg.Done()
				_, results[i] = e.refresh(token)
			}(i, token)
		}
		wg.Wait()

		succeeded := 0
		for _, err := range results {
			if err == nil {
				succeeded++
				continue
			}
			requireInvalidGrant(t, err)
		}
		assert.Equal(t, 1, succeeded, "round %d", round)
		assert.True(t, e.repo.onlyGrant().Revoked(), "round %d: the replay revokes the grant", round)
	}
}

func TestOAuthService_Refresh_Checks(t *testing.T) {
	e := newOAuthTestEnv(t)
	tokens := e.connect()

	_, err := e.svc.Refresh(context.Background(), domain.OAuthRefresh{RefreshToken: tokens.RefreshToken, ClientID: "https://evil.example/client.json"}, oauthTestMeta)
	requireInvalidGrant(t, err)
	_, err = e.refresh(tokens.AccessToken)
	requireInvalidGrant(t, err)
	_, err = e.refresh("nfmcp_rt_unknown")
	requireInvalidGrant(t, err)
	_, err = e.svc.Refresh(context.Background(), domain.OAuthRefresh{RefreshToken: tokens.RefreshToken, ClientID: oauthTestClientID, Resource: "https://other.example/mcp"}, oauthTestMeta)
	requireInvalidGrant(t, err)
	assert.False(t, e.repo.onlyGrant().Revoked(), "none of these is a reuse")

	_, err = e.svc.Refresh(context.Background(), domain.OAuthRefresh{RefreshToken: tokens.RefreshToken, ClientID: oauthTestClientID, Resource: "https://NOTIFUSE.example.com/mcp"}, oauthTestMeta)
	require.NoError(t, err)
}

func TestOAuthService_Refresh_AfterAnAPIEndpointChange(t *testing.T) {
	e := newOAuthTestEnv(t)
	tokens := e.connect()
	e.apiEndpoint = "https://mail.example.com"
	_, err := e.refresh(tokens.RefreshToken)
	requireInvalidGrant(t, err)
	assert.False(t, e.resolves(tokens.AccessToken), "the grant is bound to the old resource")
}

func TestOAuthService_RefreshTokenExpiresAfterNinetyDays(t *testing.T) {
	e := newOAuthTestEnv(t)
	onTime := e.connect()
	late := e.connect()
	e.advance(90*24*time.Hour - time.Nanosecond)
	_, err := e.refresh(onTime.RefreshToken)
	require.NoError(t, err)
	e.advance(time.Nanosecond)
	_, err = e.refresh(late.RefreshToken)
	requireInvalidGrant(t, err)
}

func TestOAuthService_AccessTokenExpiresAfterOneHour(t *testing.T) {
	e := newOAuthTestEnv(t)
	tokens := e.connect()
	e.advance(time.Hour - time.Nanosecond)
	assert.True(t, e.resolves(tokens.AccessToken))
	e.advance(time.Nanosecond)
	assert.False(t, e.resolves(tokens.AccessToken))
}

// A deleted key ends its connections: its grants go with it.
func TestOAuthService_DeletedKey(t *testing.T) {
	e := newOAuthTestEnv(t)
	tokens := e.connect()
	e.repo.deleteKey(oauthTestKeyID)
	delete(e.auth.accounts, oauthTestKeyID)
	assert.False(t, e.resolves(tokens.AccessToken))
	_, err := e.refresh(tokens.RefreshToken)
	requireInvalidGrant(t, err)
}

// Token resolution loads the key at every call, as a header key is loaded:
// a key whose account is gone or is no API key never resolves, and a change to
// the key applies at the next call.
func TestOAuthService_ResolveAccessToken(t *testing.T) {
	e := newOAuthTestEnv(t)
	tokens := e.connect()

	access, err := e.svc.ResolveAccessToken(context.Background(), tokens.AccessToken)
	require.NoError(t, err)
	assert.Equal(t, e.repo.onlyGrant().ID, access.Grant.ID)
	assert.Equal(t, "agent@notifuse.example.com", access.Key.Email)

	e.auth.accounts[oauthTestKeyID] = &domain.User{ID: oauthTestKeyID, Email: "renamed@notifuse.example.com", Type: domain.UserTypeAPIKey}
	access, err = e.svc.ResolveAccessToken(context.Background(), tokens.AccessToken)
	require.NoError(t, err)
	assert.Equal(t, "renamed@notifuse.example.com", access.Key.Email)

	e.auth.accounts[oauthTestKeyID] = &domain.User{ID: oauthTestKeyID, Type: domain.UserTypeUser}
	_, err = e.svc.ResolveAccessToken(context.Background(), tokens.AccessToken)
	assert.ErrorIs(t, err, domain.ErrOAuthInvalidToken, "an OAuth token never resolves to a person's account")

	assert.False(t, e.resolves(tokens.RefreshToken), "a refresh token is no access token")
	assert.False(t, e.resolves("nfmcp_at_unknown"))
	assert.False(t, e.resolves(""))
	assert.Equal(t, e.now(), *e.repo.onlyGrant().LastUsedAt)
}

// RFC 7009

func TestOAuthService_Revoke(t *testing.T) {
	revoke := func(e *oauthTestEnv, token, clientID string) error {
		return e.svc.Revoke(context.Background(), domain.OAuthTokenRevocation{Token: token, ClientID: clientID}, oauthTestMeta)
	}

	t.Run("an access token is revoked and the grant lives on", func(t *testing.T) {
		e := newOAuthTestEnv(t)
		tokens := e.connect()
		require.NoError(t, revoke(e, tokens.AccessToken, oauthTestClientID))
		assert.False(t, e.resolves(tokens.AccessToken))
		assert.False(t, e.repo.onlyGrant().Revoked())
		_, err := e.refresh(tokens.RefreshToken)
		require.NoError(t, err)
		rows := e.recorder.all()
		last := rows[len(rows)-1]
		assert.Equal(t, "oauth.revoke", last.Action)
		assert.Equal(t, domain.OAuthTokenKindAccess, last.Metadata["token_kind"])
		assert.Equal(t, domain.AuditAuthOAuth, last.AuthMethod)
	})

	t.Run("a refresh token revokes the grant", func(t *testing.T) {
		e := newOAuthTestEnv(t)
		tokens := e.connect()
		require.NoError(t, revoke(e, tokens.RefreshToken, oauthTestClientID))
		grant := e.repo.onlyGrant()
		assert.True(t, grant.Revoked())
		assert.Equal(t, domain.OAuthRevokedByClient, grant.RevokedReason)
		assert.False(t, e.resolves(tokens.AccessToken))
	})

	t.Run("an unknown or foreign token answers success and changes nothing", func(t *testing.T) {
		e := newOAuthTestEnv(t)
		tokens := e.connect()
		rows := len(e.recorder.all())
		require.NoError(t, revoke(e, "nfmcp_rt_unknown", oauthTestClientID))
		require.NoError(t, revoke(e, tokens.RefreshToken, "https://evil.example/client.json"))
		require.NoError(t, revoke(e, tokens.AccessToken, "https://evil.example/client.json"))
		assert.False(t, e.repo.onlyGrant().Revoked())
		assert.True(t, e.resolves(tokens.AccessToken))
		assert.Len(t, e.recorder.all(), rows)
	})

	t.Run("client_id is required", func(t *testing.T) {
		e := newOAuthTestEnv(t)
		var oauthErr *domain.OAuthError
		require.ErrorAs(t, revoke(e, "x", ""), &oauthErr)
		assert.Equal(t, domain.OAuthErrInvalidRequest, oauthErr.Code)
	})
}

// The MCP tab's connections

func TestOAuthService_WorkspaceGrants(t *testing.T) {
	e := newOAuthTestEnv(t)
	first := e.connect()
	second := e.connect()
	grants, err := e.svc.ListWorkspaceGrants(oauthSessionCtx(oauthTestOwnerID), "ws1")
	require.NoError(t, err)
	require.Len(t, grants, 2, "one key backing two connections")

	_, err = e.svc.ListWorkspaceGrants(oauthSessionCtx(oauthTestRootID), "ws1")
	require.NoError(t, err, "root counts as owner")
	for name, ctx := range map[string]context.Context{
		"a member":                  oauthSessionCtx(oauthTestMemberID),
		"another workspace's owner": oauthSessionCtx(oauthTestStrangerID),
		"an API key":                oauthKeyCtx(oauthTestKeyID),
		"no caller":                 context.Background(),
	} {
		_, err := e.svc.ListWorkspaceGrants(ctx, "ws1")
		var unauthorized *domain.ErrUnauthorized
		assert.ErrorAs(t, err, &unauthorized, name)
	}
	// A session that failed verification is not authenticated: 401, as every
	// other workspace route answers it, never a 403.
	expired := context.WithValue(oauthSessionCtx(oauthTestOwnerID), domain.UserIDKey, "ghost")
	_, err = e.svc.ListWorkspaceGrants(expired, "ws1")
	assert.ErrorIs(t, err, domain.ErrSessionAuthFailed)
	e.auth.sessions[oauthTestOwnerID] = false
	_, err = e.svc.ListWorkspaceGrants(oauthSessionCtx(oauthTestOwnerID), "ws1")
	assert.ErrorIs(t, err, domain.ErrSessionAuthFailed)
	e.auth.sessions[oauthTestOwnerID] = true

	firstGrant := e.repo.tokens[domain.HashOAuthSecret(first.AccessToken)].GrantID
	assert.ErrorIs(t, e.svc.RevokeGrant(oauthSessionCtx(oauthTestStrangerID), firstGrant, oauthTestMeta), domain.ErrOAuthGrantNotFound)
	var unauthorized *domain.ErrUnauthorized
	assert.ErrorAs(t, e.svc.RevokeGrant(oauthSessionCtx(oauthTestMemberID), firstGrant, oauthTestMeta), &unauthorized)
	assert.ErrorAs(t, e.svc.RevokeGrant(oauthKeyCtx(oauthTestKeyID), firstGrant, oauthTestMeta), &unauthorized)
	assert.ErrorIs(t, e.svc.RevokeGrant(oauthSessionCtx(oauthTestOwnerID), "missing", oauthTestMeta), domain.ErrOAuthGrantNotFound)
	assert.ErrorIs(t, e.svc.RevokeGrant(expired, firstGrant, oauthTestMeta), domain.ErrSessionAuthFailed)

	require.NoError(t, e.svc.RevokeGrant(oauthSessionCtx(oauthTestOwnerID), firstGrant, oauthTestMeta))
	assert.Equal(t, domain.OAuthRevokedByOwner, e.repo.grant(firstGrant).RevokedReason)
	assert.False(t, e.resolves(first.AccessToken))
	assert.True(t, e.resolves(second.AccessToken), "the key and its other connection stay")
	_, keyStillThere := e.auth.accounts[oauthTestKeyID]
	assert.True(t, keyStillThere)

	rows := e.recorder.all()
	last := rows[len(rows)-1]
	assert.Equal(t, "oauth.revokeGrant", last.Action)
	assert.Equal(t, oauthTestOwnerID, last.ActorID)
	assert.Equal(t, domain.OAuthRevokedByOwner, last.Metadata["reason"])
	count := len(rows)
	require.NoError(t, e.svc.RevokeGrant(oauthSessionCtx(oauthTestOwnerID), firstGrant, oauthTestMeta), "revoking twice is no error")
	assert.Len(t, e.recorder.all(), count, "and records nothing")
}

// The audit rows the service writes are its self-recorded routes, nothing
// else, and never carry a token.
func TestOAuthService_AuditActionsAreSelfRecorded(t *testing.T) {
	e := newOAuthTestEnv(t)
	tokens := e.connect()
	_, err := e.createKey(oauthSessionCtx(oauthTestOwnerID), e.authorize(), "ws1", nil)
	require.NoError(t, err)
	require.NoError(t, e.svc.Revoke(context.Background(), domain.OAuthTokenRevocation{Token: tokens.AccessToken, ClientID: oauthTestClientID}, oauthTestMeta))
	require.NoError(t, e.svc.RevokeGrant(oauthSessionCtx(oauthTestOwnerID), e.repo.token(tokens.RefreshToken).GrantID, oauthTestMeta))

	rows := e.recorder.all()
	actions := make([]string, 0, len(rows))
	for _, row := range rows {
		actions = append(actions, row.Action)
		if row.Action != "workspaces.createAPIKey" {
			_, ok := domain.AuditSelfRecordedRoutes[row.Action]
			assert.True(t, ok, row.Action)
		}
	}
	assert.Equal(t, []string{"oauth.approveWithKey", "workspaces.createAPIKey", "oauth.createKeyAndApprove", "oauth.revoke", "oauth.revokeGrant"}, actions)
	encoded, _ := json.Marshal(rows)
	assert.NotContains(t, string(encoded), tokens.AccessToken)
	assert.NotContains(t, string(encoded), tokens.RefreshToken)
}

// The periodic job, in its order, with the service's clock.
func TestOAuthService_RunMaintenance(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := mocks.NewMockOAuthRepository(ctrl)
	now := time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)
	svc := NewOAuthService(OAuthServiceConfig{Repo: repo, Now: func() time.Time { return now }})

	gomock.InOrder(
		repo.EXPECT().RevokeGrantsWithoutCredential(gomock.Any(), now).Return(int64(2), nil),
		repo.EXPECT().PurgeExpired(gomock.Any(), now).Return(int64(3), int64(4), int64(5), nil),
		repo.EXPECT().PurgeClients(gomock.Any(), now).Return(int64(6), nil),
	)
	result, err := svc.RunMaintenance(context.Background())
	require.NoError(t, err)
	assert.Equal(t, &domain.OAuthPurgeResult{GrantsRevoked: 2, RequestsPurged: 3, CodesPurged: 4, TokensPurged: 5, ClientsPurged: 6}, result)

	repo.EXPECT().RevokeGrantsWithoutCredential(gomock.Any(), now).Return(int64(0), errors.New("boom"))
	_, err = svc.RunMaintenance(context.Background())
	assert.ErrorContains(t, err, "boom")
}
