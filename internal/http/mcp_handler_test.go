package http

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/Notifuse/notifuse/internal/http/middleware"
	"github.com/Notifuse/notifuse/internal/mcp"
	"github.com/Notifuse/notifuse/internal/service"
	"github.com/Notifuse/notifuse/pkg/ratelimiter"
)

var mcpTestSecret = []byte("mcp-handler-test-secret-32-bytes!")

const mcpDiscoverBody = `{"jsonrpc":"2.0","id":1,"method":"server/discover","params":{"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{}}}}`

func signMCPTestToken(t *testing.T, claims jwt.Claims, secret []byte) string {
	t.Helper()
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(secret)
	require.NoError(t, err)
	return signed
}

func apiKeyToken(t *testing.T, userID string) string {
	return signMCPTestToken(t, service.UserClaims{UserID: userID, Type: string(domain.UserTypeAPIKey), RegisteredClaims: jwt.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))}}, mcpTestSecret)
}

// mcpHandlerFixture is the /mcp handler in front of a probe that records the
// context the MCP server would receive. API keys go through the real
// AuthService, over a mock of its account lookup; OAuth access tokens through
// a mock of the OAuth service.
type mcpHandlerFixture struct {
	handler   *MCPHandler
	accounts  *mocks.MockAuthRepository
	oauth     *mocks.MockOAuthService
	limiter   *ratelimiter.RateLimiter
	reached   int
	reachedCx context.Context
}

func newMCPKeyAuth(accounts domain.AuthRepository) *service.AuthService {
	return service.NewAuthService(service.AuthServiceConfig{
		Repository: accounts,
		GetSecret:  func() ([]byte, error) { return mcpTestSecret, nil },
	})
}

func newMCPHandlerFixture(t *testing.T, edit func(*MCPHandlerConfig)) *mcpHandlerFixture {
	t.Helper()
	ctrl := gomock.NewController(t)
	f := &mcpHandlerFixture{accounts: mocks.NewMockAuthRepository(ctrl), oauth: mocks.NewMockOAuthService(ctrl), limiter: ratelimiter.NewRateLimiter()}
	t.Cleanup(f.limiter.Stop)
	f.limiter.SetPolicy(MCPAuthFailRateLimitNamespace, 60, time.Minute)
	f.oauth.EXPECT().Unavailable().Return("").AnyTimes()
	cfg := MCPHandlerConfig{
		Enabled:     true,
		APIEndpoint: func() string { return "https://notifuse.example.com" },
		Auth:        newMCPKeyAuth(f.accounts),
		OAuth:       f.oauth,
		RateLimiter: f.limiter,
		Server: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			f.reached++
			f.reachedCx = r.Context()
			w.WriteHeader(http.StatusOK)
		}),
	}
	if edit != nil {
		edit(&cfg)
	}
	f.handler = NewMCPHandler(cfg)
	return f
}

func (f *mcpHandlerFixture) expectKey(userID string) {
	f.accounts.EXPECT().GetUserByID(gomock.Any(), userID).Return(&domain.User{ID: userID, Type: domain.UserTypeAPIKey}, nil).AnyTimes()
}

type mcpRequest struct {
	method, path, host, origin, authorization, remoteAddr string
}

func (f *mcpHandlerFixture) serve(req mcpRequest) *httptest.ResponseRecorder {
	if req.method == "" {
		req.method = http.MethodPost
	}
	if req.path == "" {
		req.path = "/mcp"
	}
	if req.host == "" {
		req.host = "notifuse.example.com"
	}
	r := httptest.NewRequest(req.method, req.path, strings.NewReader(mcpDiscoverBody))
	r.Host = req.host
	if req.origin != "" {
		r.Header.Set("Origin", req.origin)
	}
	if req.authorization != "" {
		r.Header.Set("Authorization", req.authorization)
	}
	if req.remoteAddr != "" {
		r.RemoteAddr = req.remoteAddr
	}
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("User-Agent", "agent/1")
	w := httptest.NewRecorder()
	w.Header().Set(middleware.RequestIDHeader, "req-42")
	f.handler.ServeHTTP(w, r)
	return w
}

func decodeJSONBody(t *testing.T, w *httptest.ResponseRecorder) map[string]string {
	t.Helper()
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))
	var body map[string]string
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body), w.Body.String())
	return body
}

func TestMCPHandler_OffUnlessEnabledAndNotDemo(t *testing.T) {
	for name, edit := range map[string]func(*MCPHandlerConfig){
		"MCP_ENABLED off":       func(cfg *MCPHandlerConfig) { cfg.Enabled = false },
		"demo, even if enabled": func(cfg *MCPHandlerConfig) { cfg.IsDemo = true },
	} {
		t.Run(name, func(t *testing.T) {
			f := newMCPHandlerFixture(t, edit)
			w := f.serve(mcpRequest{authorization: "Bearer " + apiKeyToken(t, "key-1")})
			assert.Equal(t, http.StatusNotFound, w.Code)
			assert.Equal(t, "MCP is not enabled on this server", decodeJSONBody(t, w)["error"])
			assert.Equal(t, 0, f.reached)
		})
	}
}

func TestMCPHandler_NeedsAnAPIEndpoint(t *testing.T) {
	for _, endpoint := range []string{"", "not a url", "ftp://notifuse.example.com"} {
		f := newMCPHandlerFixture(t, func(cfg *MCPHandlerConfig) { cfg.APIEndpoint = func() string { return endpoint } })
		w := f.serve(mcpRequest{authorization: "Bearer " + apiKeyToken(t, "key-1")})
		assert.Equal(t, http.StatusNotFound, w.Code, endpoint)
		assert.Equal(t, 0, f.reached)
	}
}

func TestMCPHandler_HostCheck(t *testing.T) {
	t.Run("another host gets a 404 naming the expected one", func(t *testing.T) {
		f := newMCPHandlerFixture(t, nil)
		w := f.serve(mcpRequest{host: "other.example.com", authorization: "Bearer " + apiKeyToken(t, "key-1")})
		assert.Equal(t, http.StatusNotFound, w.Code)
		assert.Equal(t, "notifuse.example.com", decodeJSONBody(t, w)["expected_host"])
		assert.Equal(t, 0, f.reached)
	})

	t.Run("the host is compared lowercase", func(t *testing.T) {
		f := newMCPHandlerFixture(t, func(cfg *MCPHandlerConfig) {
			cfg.APIEndpoint = func() string { return "HTTPS://Notifuse.Example.com/" }
		})
		f.expectKey("key-1")
		w := f.serve(mcpRequest{host: "NOTIFUSE.example.COM", authorization: "Bearer " + apiKeyToken(t, "key-1")})
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("the port is compared when the canonical URL has one", func(t *testing.T) {
		f := newMCPHandlerFixture(t, func(cfg *MCPHandlerConfig) {
			cfg.APIEndpoint = func() string { return "http://localhost:8080" }
		})
		f.expectKey("key-1")
		w := f.serve(mcpRequest{host: "localhost:9090", authorization: "Bearer " + apiKeyToken(t, "key-1")})
		assert.Equal(t, http.StatusNotFound, w.Code)
		assert.Equal(t, "localhost:8080", decodeJSONBody(t, w)["expected_host"])

		w = f.serve(mcpRequest{host: "localhost:8080", authorization: "Bearer " + apiKeyToken(t, "key-1")})
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, 1, f.reached)
	})

	t.Run("a default port in API_ENDPOINT is not one", func(t *testing.T) {
		endpoint, err := domain.CanonicalAPIEndpoint("https://notifuse.example.com:443/")
		require.NoError(t, err)
		assert.Equal(t, "https://notifuse.example.com", endpoint.URL)
		assert.Equal(t, "https://notifuse.example.com", endpoint.Origin)
		assert.Equal(t, "notifuse.example.com", endpoint.Host)
		assert.False(t, endpoint.HasPort)
	})
}

func TestMCPHandler_PathsUnderMCPAnswerJSON(t *testing.T) {
	f := newMCPHandlerFixture(t, nil)
	for _, path := range []string{"/mcp/", "/mcp/tools"} {
		w := f.serve(mcpRequest{path: path, authorization: "Bearer " + apiKeyToken(t, "key-1")})
		assert.Equal(t, http.StatusNotFound, w.Code, path)
		assert.Equal(t, "https://notifuse.example.com/mcp", decodeJSONBody(t, w)["mcp_url"], path)
	}
	assert.Equal(t, 0, f.reached)
}

// A foreign Origin is refused before any credential is read: the auth mock
// expects no call.
func TestMCPHandler_ForeignOriginIsRefusedBeforeTheCredential(t *testing.T) {
	f := newMCPHandlerFixture(t, nil)
	w := f.serve(mcpRequest{origin: "https://evil.example", authorization: "Bearer " + apiKeyToken(t, "key-1")})
	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Empty(t, w.Header().Get("Access-Control-Allow-Origin"))
	assert.Equal(t, 0, f.reached)
}

func TestMCPHandler_AllowedOrigins(t *testing.T) {
	f := newMCPHandlerFixture(t, func(cfg *MCPHandlerConfig) { cfg.AllowedOrigins = []string{"http://localhost:6274"} })
	f.expectKey("key-1")

	for _, origin := range []string{"https://notifuse.example.com", "http://localhost:6274", ""} {
		w := f.serve(mcpRequest{origin: origin, authorization: "Bearer " + apiKeyToken(t, "key-1")})
		assert.Equal(t, http.StatusOK, w.Code, origin)
		assert.Equal(t, origin, w.Header().Get("Access-Control-Allow-Origin"), origin)
		assert.Empty(t, w.Header().Get("Access-Control-Allow-Credentials"))
	}
	assert.Equal(t, 3, f.reached)
}

func TestMCPHandler_PreflightNeedsNoCredential(t *testing.T) {
	f := newMCPHandlerFixture(t, nil)
	w := f.serve(mcpRequest{method: http.MethodOptions, origin: "https://notifuse.example.com"})
	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.Equal(t, "https://notifuse.example.com", w.Header().Get("Access-Control-Allow-Origin"))
	assert.Equal(t, "WWW-Authenticate", w.Header().Get("Access-Control-Expose-Headers"))
	assert.Equal(t, 0, f.reached)
}

// Every credential but a live API key is a 401 with WWW-Authenticate: Bearer,
// answered before the MCP server reads the server/discover body.
func TestMCPHandler_RefusesEveryCredentialButALiveAPIKey(t *testing.T) {
	expiry := jwt.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))}
	cases := map[string]struct {
		authorization string
		account       func(*mocks.MockAuthRepository)
	}{
		"no header":           {authorization: ""},
		"basic scheme":        {authorization: "Basic a2V5Og=="},
		"not a JWT":           {authorization: "Bearer opaque-token"},
		"console session":     {authorization: "Bearer " + signMCPTestToken(t, service.UserClaims{UserID: "u1", Type: "user", SessionID: "s1", RegisteredClaims: expiry}, mcpTestSecret)},
		"invitation":          {authorization: "Bearer " + signMCPTestToken(t, service.InvitationClaims{InvitationID: "i1", WorkspaceID: "ws1", Email: "a@example.com", RegisteredClaims: expiry}, mcpTestSecret)},
		"another secret":      {authorization: "Bearer " + signMCPTestToken(t, service.UserClaims{UserID: "key-1", Type: "api_key", RegisteredClaims: expiry}, []byte("some-other-secret-of-32-bytes!!!"))},
		"expired key":         {authorization: "Bearer " + signMCPTestToken(t, service.UserClaims{UserID: "key-1", Type: "api_key", RegisteredClaims: jwt.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(time.Now().Add(-time.Hour))}}, mcpTestSecret)},
		"api key, no user ID": {authorization: "Bearer " + signMCPTestToken(t, service.UserClaims{Type: "api_key", RegisteredClaims: expiry}, mcpTestSecret)},
		"revoked key": {
			authorization: "Bearer " + apiKeyToken(t, "key-1"),
			account: func(accounts *mocks.MockAuthRepository) {
				accounts.EXPECT().GetUserByID(gomock.Any(), "key-1").Return(nil, sql.ErrNoRows)
			},
		},
		"key naming a person's account": {
			authorization: "Bearer " + apiKeyToken(t, "u1"),
			account: func(accounts *mocks.MockAuthRepository) {
				accounts.EXPECT().GetUserByID(gomock.Any(), "u1").Return(&domain.User{ID: "u1", Type: domain.UserTypeUser}, nil)
			},
		},
		// An OAuth refresh token is for the token endpoint: refused without a lookup.
		"OAuth refresh token": {authorization: "Bearer nfmcp_rt_abcdefghijklmnopqrstuvwxyz0123456789ABCDEFG"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			f := newMCPHandlerFixture(t, func(cfg *MCPHandlerConfig) { cfg.OAuth = nil })
			if tc.account != nil {
				tc.account(f.accounts)
			}
			w := f.serve(mcpRequest{authorization: tc.authorization})
			assert.Equal(t, http.StatusUnauthorized, w.Code)
			challenge := `Bearer error="invalid_token"`
			if tc.authorization == "" {
				challenge = "Bearer"
			}
			assert.Equal(t, challenge, w.Header().Get("WWW-Authenticate"))
			assert.Equal(t, 0, f.reached, "the MCP server never sees the request")
		})
	}
}

// A live key reaches the MCP server with the user ID, the api_key type and
// the MCPCaller in its context, never a session ID or the system-call marker.
func TestMCPHandler_ALiveKeyReachesTheServer(t *testing.T) {
	f := newMCPHandlerFixture(t, nil)
	f.accounts.EXPECT().GetUserByID(gomock.Any(), "key-1").Return(&domain.User{ID: "key-1", Type: domain.UserTypeAPIKey}, nil)

	w := f.serve(mcpRequest{authorization: "bearer " + apiKeyToken(t, "key-1"), remoteAddr: "198.51.100.4:5555"})
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, 1, f.reached)

	ctx := f.reachedCx
	assert.Equal(t, "key-1", ctx.Value(domain.UserIDKey))
	assert.Equal(t, "api_key", ctx.Value(domain.UserTypeKey))
	assert.Nil(t, ctx.Value(domain.SessionIDKey))
	assert.Nil(t, ctx.Value(domain.SystemCallKey))
	assert.Equal(t, &domain.MCPCaller{Kind: domain.MCPCredentialHeader, APIKeyUserID: "key-1"}, domain.MCPCallerFromContext(ctx))
	assert.Equal(t, mcp.RequestMeta{IP: "198.51.100.4", UserAgent: "agent/1", RequestID: "req-42"}, mcp.RequestMetaFromContext(ctx))
}

// mcp_auth_fail: sixty failed credentials a minute per IP. The bucket is
// checked before the credential is looked up, so the sixty-first request is
// refused without a database read; a good credential does not charge it.
func TestMCPHandler_FailedCredentialBucket(t *testing.T) {
	f := newMCPHandlerFixture(t, nil)
	lookups := 0
	dbDown := false
	f.accounts.EXPECT().GetUserByID(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, userID string) (*domain.User, error) {
		lookups++
		if dbDown {
			return nil, errors.New("connection refused")
		}
		if userID == "revoked" {
			return nil, sql.ErrNoRows
		}
		return &domain.User{ID: "key-1", Type: domain.UserTypeAPIKey}, nil
	}).AnyTimes()
	const ip = "198.51.100.4:1"

	t.Run("a token not signed by this installation never reaches the database", func(t *testing.T) {
		w := f.serve(mcpRequest{authorization: "Bearer not-a-jwt", remoteAddr: "203.0.113.9:1"})
		assert.Equal(t, http.StatusUnauthorized, w.Code)
		assert.Equal(t, 0, lookups)
	})

	t.Run("refusals fill the bucket, then answer 429", func(t *testing.T) {
		for i := 0; i < 60; i++ {
			require.Equal(t, http.StatusUnauthorized, f.serve(mcpRequest{authorization: "Bearer " + apiKeyToken(t, "revoked"), remoteAddr: ip}).Code, "attempt %d", i+1)
		}
		w := f.serve(mcpRequest{authorization: "Bearer " + apiKeyToken(t, "revoked"), remoteAddr: ip})
		assert.Equal(t, http.StatusTooManyRequests, w.Code)
	})

	t.Run("a valid key is never refused because of the bucket", func(t *testing.T) {
		w := f.serve(mcpRequest{authorization: "Bearer " + apiKeyToken(t, "key-1"), remoteAddr: ip})
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("a database failure is a 500 and does not use up the bucket", func(t *testing.T) {
		dbDown = true
		defer func() { dbDown = false }()
		other := "198.51.100.5:1"
		for i := 0; i < 61; i++ {
			w := f.serve(mcpRequest{authorization: "Bearer " + apiKeyToken(t, "key-1"), remoteAddr: other})
			require.Equal(t, http.StatusInternalServerError, w.Code, "attempt %d", i+1)
			require.Empty(t, w.Header().Get("WWW-Authenticate"), "a client must not drop a good key")
		}
		dbDown = false
		w := f.serve(mcpRequest{authorization: "Bearer " + apiKeyToken(t, "revoked"), remoteAddr: other})
		assert.Equal(t, http.StatusUnauthorized, w.Code, "the failures above charged nothing")
	})
}

// The SDK's localhost guard is off: a reverse proxy on the same machine
// connects over 127.0.0.1 with the public Host, and that request is served.
func TestMCPHandler_ServesALoopbackConnectionWithTheCanonicalHost(t *testing.T) {
	ctrl := gomock.NewController(t)
	accounts := mocks.NewMockAuthRepository(ctrl)
	accounts.EXPECT().GetUserByID(gomock.Any(), "key-1").Return(&domain.User{ID: "key-1", Type: domain.UserTypeAPIKey}, nil)
	limiter := ratelimiter.NewRateLimiter()
	defer limiter.Stop()
	limiter.SetPolicy(MCPAuthFailRateLimitNamespace, 60, time.Minute)

	server := mcp.NewServer(mcp.Deps{
		Auth:        mocks.NewMockAuthService(ctrl),
		Workspaces:  mocks.NewMockWorkspaceRepository(ctrl),
		Contacts:    mocks.NewMockContactService(ctrl),
		RateLimiter: limiter,
	})
	handler := NewMCPHandler(MCPHandlerConfig{
		Enabled:     true,
		APIEndpoint: func() string { return "https://notifuse.example.com" },
		Auth:        newMCPKeyAuth(accounts),
		RateLimiter: limiter,
		Server:      server.Handler(),
	})
	srv := httptest.NewServer(handler)
	defer srv.Close()

	req, err := http.NewRequest(http.MethodPost, srv.URL+"/mcp", strings.NewReader(mcpDiscoverBody))
	require.NoError(t, err)
	req.Host = "notifuse.example.com"
	req.Header.Set("Authorization", "Bearer "+apiKeyToken(t, "key-1"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Mcp-Protocol-Version", "2026-07-28")
	req.Header.Set("Mcp-Method", "server/discover")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusOK, resp.StatusCode, string(body))
	assert.Contains(t, string(body), `"supportedVersions"`)
}

const mcpTestPRM = `resource_metadata="https://notifuse.example.com/.well-known/oauth-protected-resource/mcp", scope="mcp"`

// An OAuth access token reaches the server as its grant's key: the key's
// claims, and an MCPCaller naming the grant and its client.
func TestMCPHandler_AnOAuthAccessTokenReachesTheServerAsItsKey(t *testing.T) {
	f := newMCPHandlerFixture(t, nil)
	const token = "nfmcp_at_abcdefghijklmnopqrstuvwxyz0123456789ABCDEFG"
	f.oauth.EXPECT().ResolveAccessToken(gomock.Any(), token).Return(&domain.OAuthAccess{
		Grant: &domain.OAuthGrant{ID: "grant-1", APIKeyUserID: "key-1", ClientName: "Claude"},
		Key:   &domain.User{ID: "key-1", Type: domain.UserTypeAPIKey},
	}, nil)

	w := f.serve(mcpRequest{authorization: "Bearer " + token, remoteAddr: "198.51.100.4:5555"})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Equal(t, 1, f.reached)

	ctx := f.reachedCx
	assert.Equal(t, "key-1", ctx.Value(domain.UserIDKey))
	assert.Equal(t, "api_key", ctx.Value(domain.UserTypeKey))
	assert.Nil(t, ctx.Value(domain.SessionIDKey))
	assert.Nil(t, ctx.Value(domain.SystemCallKey))
	assert.Equal(t, &domain.MCPCaller{Kind: domain.MCPCredentialOAuth, APIKeyUserID: "key-1", GrantID: "grant-1", ClientName: "Claude"}, domain.MCPCallerFromContext(ctx))
}

// Every refusal names the protected resource metadata while OAuth is
// available, so that a client without a credential discovers the
// authorization server; a credential that was sent adds invalid_token.
func TestMCPHandler_RefusalsNameTheProtectedResourceMetadata(t *testing.T) {
	const token = "nfmcp_at_abcdefghijklmnopqrstuvwxyz0123456789ABCDEFG"

	t.Run("no credential", func(t *testing.T) {
		f := newMCPHandlerFixture(t, nil)
		w := f.serve(mcpRequest{})
		assert.Equal(t, http.StatusUnauthorized, w.Code)
		assert.Equal(t, "Bearer "+mcpTestPRM, w.Header().Get("WWW-Authenticate"))
	})

	t.Run("an access token that does not resolve", func(t *testing.T) {
		f := newMCPHandlerFixture(t, nil)
		f.oauth.EXPECT().ResolveAccessToken(gomock.Any(), token).Return(nil, domain.ErrOAuthInvalidToken)
		w := f.serve(mcpRequest{authorization: "Bearer " + token})
		assert.Equal(t, http.StatusUnauthorized, w.Code)
		assert.Equal(t, "Bearer "+mcpTestPRM+`, error="invalid_token"`, w.Header().Get("WWW-Authenticate"))
		assert.Equal(t, 0, f.reached)
	})

	t.Run("a refresh token is refused without a lookup", func(t *testing.T) {
		f := newMCPHandlerFixture(t, nil)
		w := f.serve(mcpRequest{authorization: "Bearer nfmcp_rt_abcdefghijklmnopqrstuvwxyz0123456789ABCDEFG"})
		assert.Equal(t, http.StatusUnauthorized, w.Code)
		assert.Equal(t, "Bearer "+mcpTestPRM+`, error="invalid_token"`, w.Header().Get("WWW-Authenticate"))
	})

	t.Run("a revoked API key", func(t *testing.T) {
		f := newMCPHandlerFixture(t, nil)
		f.accounts.EXPECT().GetUserByID(gomock.Any(), "key-1").Return(nil, sql.ErrNoRows)
		w := f.serve(mcpRequest{authorization: "Bearer " + apiKeyToken(t, "key-1")})
		assert.Equal(t, http.StatusUnauthorized, w.Code)
		assert.Equal(t, "Bearer "+mcpTestPRM+`, error="invalid_token"`, w.Header().Get("WWW-Authenticate"))
	})

	t.Run("plain Bearer while OAuth is unavailable", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		unavailable := mocks.NewMockOAuthService(ctrl)
		unavailable.EXPECT().Unavailable().Return("API_ENDPOINT is not https").AnyTimes()
		unavailable.EXPECT().ResolveAccessToken(gomock.Any(), token).Return(nil, domain.ErrOAuthInvalidToken)
		f := newMCPHandlerFixture(t, func(cfg *MCPHandlerConfig) { cfg.OAuth = unavailable })
		w := f.serve(mcpRequest{})
		assert.Equal(t, "Bearer", w.Header().Get("WWW-Authenticate"))
		w = f.serve(mcpRequest{authorization: "Bearer " + token})
		assert.Equal(t, http.StatusUnauthorized, w.Code)
		assert.Equal(t, `Bearer error="invalid_token"`, w.Header().Get("WWW-Authenticate"))
	})
}

// A token the server could not check is a 500, never a 401 that would make a
// client drop it.
func TestMCPHandler_AnAccessTokenThatCannotBeCheckedIsA500(t *testing.T) {
	f := newMCPHandlerFixture(t, nil)
	const token = "nfmcp_at_abcdefghijklmnopqrstuvwxyz0123456789ABCDEFG"
	f.oauth.EXPECT().ResolveAccessToken(gomock.Any(), token).Return(nil, errors.New("connection refused"))
	w := f.serve(mcpRequest{authorization: "Bearer " + token})
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Empty(t, w.Header().Get("WWW-Authenticate"))
	assert.Equal(t, 0, f.reached)
}
