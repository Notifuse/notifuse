package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/Notifuse/notifuse/internal/http/middleware"
	"github.com/Notifuse/notifuse/internal/service"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/Notifuse/notifuse/pkg/ratelimiter"
)

// recordingLogger keeps every line and field written, for the tests that
// check what is never logged.
type recordingLogger struct {
	mu    *sync.Mutex
	lines *[]string
	field string
}

func newRecordingLogger() *recordingLogger {
	return &recordingLogger{mu: &sync.Mutex{}, lines: &[]string{}}
}

func (l *recordingLogger) record(msg string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	*l.lines = append(*l.lines, l.field+" "+msg)
}
func (l *recordingLogger) Debug(msg string) { l.record(msg) }
func (l *recordingLogger) Info(msg string)  { l.record(msg) }
func (l *recordingLogger) Warn(msg string)  { l.record(msg) }
func (l *recordingLogger) Error(msg string) { l.record(msg) }
func (l *recordingLogger) Fatal(msg string) { l.record(msg) }
func (l *recordingLogger) WithField(key string, value interface{}) logger.Logger {
	return &recordingLogger{mu: l.mu, lines: l.lines, field: fmt.Sprintf("%s %s=%v", l.field, key, value)}
}
func (l *recordingLogger) WithFields(fields map[string]interface{}) logger.Logger {
	return &recordingLogger{mu: l.mu, lines: l.lines, field: fmt.Sprintf("%s %v", l.field, fields)}
}
func (l *recordingLogger) all() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return strings.Join(*l.lines, "\n")
}

const (
	oauthTestHost      = "notifuse.example.com"
	oauthTestIssuer    = "https://notifuse.example.com"
	oauthTestRequestID = "req_AbCdEfGh-123_xyz"
	oauthTestBinding   = "browser-binding-secret"
	oauthTestCookie    = "__Host-nf_oauth_" + oauthTestRequestID
	oauthTestAccess    = "nfmcp_at_abcdefghijklmnopqrstuvwxyz0123456789ABCDEFG"
)

type oauthHandlerFixture struct {
	mux     *http.ServeMux
	svc     *mocks.MockOAuthService
	limiter *ratelimiter.RateLimiter
	logs    *recordingLogger
}

// newOAuthHandlerFixture mounts the handler on a mux, with the four limits of
// app.go and a mock service that reports OAuth available.
func newOAuthHandlerFixture(t *testing.T, edit func(*OAuthHandlerConfig)) *oauthHandlerFixture {
	t.Helper()
	ctrl := gomock.NewController(t)
	f := &oauthHandlerFixture{mux: http.NewServeMux(), svc: mocks.NewMockOAuthService(ctrl), limiter: ratelimiter.NewRateLimiter(), logs: newRecordingLogger()}
	t.Cleanup(f.limiter.Stop)
	f.limiter.SetPolicy(OAuthAuthorizeRateLimitNamespace, 30, time.Minute)
	f.limiter.SetPolicy(OAuthApproveRateLimitNamespace, 20, time.Minute)
	f.limiter.SetPolicy(OAuthTokenIPRateLimitNamespace, 600, time.Minute)
	f.limiter.SetPolicy(OAuthTokenCredentialRateLimitNamespace, 10, time.Minute)
	f.svc.EXPECT().Unavailable().Return("").AnyTimes()
	cfg := OAuthHandlerConfig{
		Enabled:      true,
		APIEndpoint:  func() string { return oauthTestIssuer },
		Service:      f.svc,
		GetJWTSecret: func() ([]byte, error) { return mcpTestSecret, nil },
		RateLimiter:  f.limiter,
		Logger:       f.logs,
	}
	if edit != nil {
		edit(&cfg)
	}
	NewOAuthHandler(cfg).RegisterRoutes(f.mux)
	return f
}

type oauthRequest struct {
	method, path, host, body, contentType, authorization, remoteAddr, origin string
	cookies                                                                  []*http.Cookie
}

func (f *oauthHandlerFixture) serve(req oauthRequest) *httptest.ResponseRecorder {
	if req.method == "" {
		req.method = http.MethodGet
	}
	if req.host == "" {
		req.host = oauthTestHost
	}
	r := httptest.NewRequest(req.method, req.path, strings.NewReader(req.body))
	r.Host = req.host
	if req.contentType != "" {
		r.Header.Set("Content-Type", req.contentType)
	}
	if req.authorization != "" {
		r.Header.Set("Authorization", req.authorization)
	}
	if req.remoteAddr != "" {
		r.RemoteAddr = req.remoteAddr
	}
	if req.origin != "" {
		r.Header.Set("Origin", req.origin)
	}
	for _, cookie := range req.cookies {
		r.AddCookie(cookie)
	}
	r.Header.Set("User-Agent", "agent/1")
	w := httptest.NewRecorder()
	w.Header().Set(middleware.RequestIDHeader, "req-42")
	f.mux.ServeHTTP(w, r)
	return w
}

func (f *oauthHandlerFixture) form(path, body string) *httptest.ResponseRecorder {
	return f.serve(oauthRequest{method: http.MethodPost, path: path, body: body, contentType: "application/x-www-form-urlencoded"})
}

func sessionToken(t *testing.T, userID, sessionID string) string {
	return signMCPTestToken(t, service.UserClaims{UserID: userID, Type: string(domain.UserTypeUser), SessionID: sessionID,
		RegisteredClaims: jwt.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))}}, mcpTestSecret)
}

func bindingCookie() *http.Cookie {
	return &http.Cookie{Name: oauthTestCookie, Value: oauthTestBinding}
}

// --- Discovery ---

func TestOAuthHandler_ProtectedResourceMetadata(t *testing.T) {
	f := newOAuthHandlerFixture(t, nil)
	// RFC 9728: clients probe the path-suffixed document first, then the root.
	for _, path := range []string{"/.well-known/oauth-protected-resource/mcp", "/.well-known/oauth-protected-resource"} {
		w := f.serve(oauthRequest{path: path, origin: "https://claude.ai"})
		require.Equal(t, http.StatusOK, w.Code, path)
		assert.JSONEq(t, `{
			"resource": "https://notifuse.example.com/mcp",
			"authorization_servers": ["https://notifuse.example.com"],
			"scopes_supported": ["mcp"],
			"bearer_methods_supported": ["header"]
		}`, w.Body.String(), path)
		assert.Equal(t, "*", w.Header().Get("Access-Control-Allow-Origin"), path)
		assert.Empty(t, w.Header().Get("Access-Control-Allow-Credentials"), path)
	}

	w := f.serve(oauthRequest{path: "/.well-known/oauth-protected-resource/other"})
	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))
}

func TestOAuthHandler_AuthorizationServerMetadata(t *testing.T) {
	f := newOAuthHandlerFixture(t, nil)
	w := f.serve(oauthRequest{path: "/.well-known/oauth-authorization-server"})
	require.Equal(t, http.StatusOK, w.Code)
	// No registration_endpoint: clients identify with a metadata document.
	assert.JSONEq(t, `{
		"issuer": "https://notifuse.example.com",
		"authorization_endpoint": "https://notifuse.example.com/oauth/authorize",
		"token_endpoint": "https://notifuse.example.com/oauth/token",
		"revocation_endpoint": "https://notifuse.example.com/oauth/revoke",
		"response_types_supported": ["code"],
		"grant_types_supported": ["authorization_code", "refresh_token"],
		"code_challenge_methods_supported": ["S256"],
		"token_endpoint_auth_methods_supported": ["none"],
		"revocation_endpoint_auth_methods_supported": ["none"],
		"client_id_metadata_document_supported": true,
		"authorization_response_iss_parameter_supported": true,
		"scopes_supported": ["mcp", "offline_access"]
	}`, w.Body.String())
	assert.Equal(t, "*", w.Header().Get("Access-Control-Allow-Origin"))

	t.Run("a preflight is answered without a body", func(t *testing.T) {
		w := f.serve(oauthRequest{method: http.MethodOptions, path: "/.well-known/oauth-authorization-server", origin: "https://claude.ai"})
		assert.Equal(t, http.StatusNoContent, w.Code)
		assert.Equal(t, "*", w.Header().Get("Access-Control-Allow-Origin"))
		assert.Empty(t, w.Header().Get("Access-Control-Allow-Credentials"))
	})

	t.Run("a path under it is a JSON 404", func(t *testing.T) {
		w := f.serve(oauthRequest{path: "/.well-known/oauth-authorization-server/mcp"})
		assert.Equal(t, http.StatusNotFound, w.Code)
		assert.Equal(t, "application/json", w.Header().Get("Content-Type"))
	})
}

func TestOAuthHandler_OpenIDConfigurationIsAlwaysAJSON404(t *testing.T) {
	for name, edit := range map[string]func(*OAuthHandlerConfig){
		"OAuth available": nil,
		"MCP off":         func(cfg *OAuthHandlerConfig) { cfg.Enabled = false },
	} {
		f := newOAuthHandlerFixture(t, edit)
		for _, path := range []string{"/.well-known/openid-configuration", "/.well-known/openid-configuration/mcp"} {
			w := f.serve(oauthRequest{path: path})
			assert.Equal(t, http.StatusNotFound, w.Code, name)
			assert.Equal(t, "application/json", w.Header().Get("Content-Type"), name)
		}
	}
}

// The discovery documents and /oauth/* answer a JSON 404 while MCP is off, in
// demo mode, and while API_ENDPOINT is plain http on a public host or has a
// path; and on any host but the canonical one.
func TestOAuthHandler_OffAnswersJSON404Everywhere(t *testing.T) {
	paths := []string{
		"/.well-known/oauth-protected-resource/mcp",
		"/.well-known/oauth-protected-resource",
		"/.well-known/oauth-authorization-server",
		"/oauth/authorize",
		"/oauth/token",
		"/oauth/revoke",
	}
	realService := func(endpoint string) func(*OAuthHandlerConfig) {
		return func(cfg *OAuthHandlerConfig) {
			cfg.APIEndpoint = func() string { return endpoint }
			cfg.Service = service.NewOAuthService(service.OAuthServiceConfig{APIEndpoint: func() string { return endpoint }})
		}
	}
	for name, tc := range map[string]struct {
		edit func(*OAuthHandlerConfig)
		host string
	}{
		"MCP_ENABLED off":          {edit: func(cfg *OAuthHandlerConfig) { cfg.Enabled = false }},
		"demo":                     {edit: func(cfg *OAuthHandlerConfig) { cfg.IsDemo = true }},
		"API_ENDPOINT http":        {edit: realService("http://notifuse.example.com")},
		"API_ENDPOINT with a path": {edit: realService("https://notifuse.example.com/notifuse")},
		"another host":             {host: "other.example.com"},
	} {
		t.Run(name, func(t *testing.T) {
			f := newOAuthHandlerFixture(t, tc.edit)
			for _, path := range paths {
				w := f.serve(oauthRequest{path: path, host: tc.host})
				assert.Equal(t, http.StatusNotFound, w.Code, path)
				assert.Equal(t, "application/json", w.Header().Get("Content-Type"), path)
			}
			// The authorization page's calls are off too; they are not
			// bound to the canonical host.
			if tc.host == "" {
				w := f.serve(oauthRequest{path: "/api/oauth.getAuthorizationRequest?request_id=x"})
				assert.Equal(t, http.StatusNotFound, w.Code)
			}
		})
	}

	t.Run("loopback http is available", func(t *testing.T) {
		f := newOAuthHandlerFixture(t, realService("http://localhost:8080"))
		w := f.serve(oauthRequest{path: "/.well-known/oauth-authorization-server", host: "localhost:8080"})
		require.Equal(t, http.StatusOK, w.Code)
		var metadata map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &metadata))
		assert.Equal(t, "http://localhost:8080", metadata["issuer"])
	})
}

func TestOAuthHandler_RegisterIsA404(t *testing.T) {
	f := newOAuthHandlerFixture(t, nil)
	for _, path := range []string{"/oauth/register", "/oauth", "/oauth/"} {
		w := f.serve(oauthRequest{method: http.MethodPost, path: path, body: `{}`, contentType: "application/json"})
		assert.Equal(t, http.StatusNotFound, w.Code, path)
		assert.Equal(t, "application/json", w.Header().Get("Content-Type"), path)
	}
}

// --- GET /oauth/authorize ---

const authorizeQuery = "/oauth/authorize?response_type=code&client_id=https%3A%2F%2Fclaude.ai%2Foauth%2Fclient.json&redirect_uri=https%3A%2F%2Fclaude.ai%2Fapi%2Fmcp%2Fauth_callback&state=st-1&code_challenge=E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM&code_challenge_method=S256&resource=https%3A%2F%2Fnotifuse.example.com%2Fmcp&scope=mcp+offline_access"

func assertAuthorizeHeaders(t *testing.T, w *httptest.ResponseRecorder) {
	t.Helper()
	assert.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	assert.Equal(t, "DENY", w.Header().Get("X-Frame-Options"))
	assert.Contains(t, w.Header().Get("Content-Security-Policy"), "frame-ancestors 'none'")
	assert.Equal(t, "no-referrer", w.Header().Get("Referrer-Policy"))
	// A navigation: no CORS header at all.
	assert.Empty(t, w.Header().Get("Access-Control-Allow-Origin"))
	assert.Empty(t, w.Header().Get("Access-Control-Allow-Credentials"))
}

func TestOAuthHandler_AuthorizeBindsTheBrowserAndRedirectsToTheConsole(t *testing.T) {
	f := newOAuthHandlerFixture(t, nil)
	f.svc.EXPECT().CreateAuthorizationRequest(gomock.Any(), domain.OAuthAuthorizeParams{
		ResponseType:        "code",
		ClientID:            "https://claude.ai/oauth/client.json",
		RedirectURI:         "https://claude.ai/api/mcp/auth_callback",
		State:               "st-1",
		CodeChallenge:       "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM",
		CodeChallengeMethod: "S256",
		Resource:            "https://notifuse.example.com/mcp",
	}).Return(&domain.OAuthAuthorizationStarted{RequestID: oauthTestRequestID, BrowserBinding: oauthTestBinding}, nil)

	w := f.serve(oauthRequest{path: authorizeQuery, origin: "https://claude.ai"})
	require.Equal(t, http.StatusFound, w.Code, w.Body.String())
	assert.Equal(t, "/console/oauth/authorize?request="+oauthTestRequestID, w.Header().Get("Location"))
	assertAuthorizeHeaders(t, w)

	cookies := w.Result().Cookies()
	require.Len(t, cookies, 1)
	cookie := cookies[0]
	assert.Equal(t, oauthTestCookie, cookie.Name)
	assert.Equal(t, oauthTestBinding, cookie.Value)
	assert.Equal(t, "/", cookie.Path)
	assert.Equal(t, 600, cookie.MaxAge)
	assert.True(t, cookie.HttpOnly)
	assert.True(t, cookie.Secure)
	assert.Equal(t, http.SameSiteLaxMode, cookie.SameSite)
	assert.Empty(t, cookie.Domain)
}

// On loopback http (development), the cookie goes without Secure, which a
// browser refuses there, and so without the __Host- prefix.
func TestOAuthHandler_AuthorizeCookieOnLoopbackHTTP(t *testing.T) {
	f := newOAuthHandlerFixture(t, func(cfg *OAuthHandlerConfig) {
		cfg.APIEndpoint = func() string { return "http://localhost:8080" }
	})
	f.svc.EXPECT().CreateAuthorizationRequest(gomock.Any(), gomock.Any()).
		Return(&domain.OAuthAuthorizationStarted{RequestID: oauthTestRequestID, BrowserBinding: oauthTestBinding}, nil)
	w := f.serve(oauthRequest{path: authorizeQuery, host: "localhost:8080"})
	require.Equal(t, http.StatusFound, w.Code)
	cookie := w.Result().Cookies()[0]
	assert.Equal(t, "nf_oauth_"+oauthTestRequestID, cookie.Name)
	assert.False(t, cookie.Secure)
	assert.True(t, cookie.HttpOnly)
}

// A refusal renders an error page and never redirects, whatever redirect_uri
// says: until the client is checked, that URI is nobody's.
func TestOAuthHandler_AuthorizeErrorPageNeverRedirects(t *testing.T) {
	f := newOAuthHandlerFixture(t, nil)
	f.svc.EXPECT().CreateAuthorizationRequest(gomock.Any(), gomock.Any()).Return(nil, &domain.OAuthAuthorizeError{
		Reason:  domain.OAuthAuthorizeInvalidRedirect,
		Message: `redirect_uri is not one of the application's redirect URIs <script>`,
	})
	w := f.serve(oauthRequest{path: authorizeQuery})
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Empty(t, w.Header().Get("Location"))
	assert.Empty(t, w.Result().Cookies())
	assert.Equal(t, "text/html; charset=utf-8", w.Header().Get("Content-Type"))
	assertAuthorizeHeaders(t, w)
	body := w.Body.String()
	assert.Contains(t, body, "invalid_redirect_uri")
	assert.Contains(t, body, "&lt;script&gt;")
	assert.NotContains(t, body, "<script>")
	assert.NotContains(t, body, "claude.ai/api/mcp/auth_callback")
}

func TestOAuthHandler_AuthorizeRefusesARepeatedParameter(t *testing.T) {
	f := newOAuthHandlerFixture(t, nil)
	w := f.serve(oauthRequest{path: authorizeQuery + "&redirect_uri=https%3A%2F%2Fevil.example%2Fcb"})
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "redirect_uri parameter appears more than once")
	assert.Empty(t, w.Header().Get("Location"))
}

func TestOAuthHandler_AuthorizeOnlyTakesGET(t *testing.T) {
	f := newOAuthHandlerFixture(t, nil)
	w := f.serve(oauthRequest{method: http.MethodPost, path: authorizeQuery})
	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	assertAuthorizeHeaders(t, w)
}

// oauth_authorize: thirty a minute per IP, each request counted.
func TestOAuthHandler_AuthorizeRateLimitPerIP(t *testing.T) {
	f := newOAuthHandlerFixture(t, nil)
	f.svc.EXPECT().CreateAuthorizationRequest(gomock.Any(), gomock.Any()).
		Return(nil, &domain.OAuthAuthorizeError{Reason: domain.OAuthAuthorizeNotCIMD, Message: "no"}).Times(30)
	for i := 0; i < 30; i++ {
		require.Equal(t, http.StatusBadRequest, f.serve(oauthRequest{path: authorizeQuery, remoteAddr: "198.51.100.4:1"}).Code, "attempt %d", i+1)
	}
	w := f.serve(oauthRequest{path: authorizeQuery, remoteAddr: "198.51.100.4:1"})
	assert.Equal(t, http.StatusTooManyRequests, w.Code)
	assertAuthorizeHeaders(t, w)
}

// --- POST /oauth/token ---

func TestOAuthHandler_TokenAuthorizationCode(t *testing.T) {
	f := newOAuthHandlerFixture(t, nil)
	f.svc.EXPECT().ExchangeCode(gomock.Any(), domain.OAuthCodeExchange{
		Code:         "code-1",
		ClientID:     "https://claude.ai/oauth/client.json",
		RedirectURI:  "https://claude.ai/api/mcp/auth_callback",
		CodeVerifier: "verifier",
		Resource:     "https://notifuse.example.com/mcp",
	}, domain.OAuthCallMeta{IP: "192.0.2.1", UserAgent: "agent/1", RequestID: "req-42"}).Return(&domain.OAuthTokenResponse{
		AccessToken: oauthTestAccess, TokenType: "Bearer", ExpiresIn: 3600, RefreshToken: "nfmcp_rt_x", Scope: "mcp",
	}, nil)

	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {"code-1"},
		"client_id":     {"https://claude.ai/oauth/client.json"},
		"redirect_uri":  {"https://claude.ai/api/mcp/auth_callback"},
		"code_verifier": {"verifier"},
		"resource":      {"https://notifuse.example.com/mcp"},
	}
	w := f.form("/oauth/token", form.Encode())
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.JSONEq(t, `{"access_token":"`+oauthTestAccess+`","token_type":"Bearer","expires_in":3600,"refresh_token":"nfmcp_rt_x","scope":"mcp"}`, w.Body.String())
	assert.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	assert.Equal(t, "*", w.Header().Get("Access-Control-Allow-Origin"))
	assert.Empty(t, w.Header().Get("Access-Control-Allow-Credentials"))
}

func TestOAuthHandler_TokenRefresh(t *testing.T) {
	f := newOAuthHandlerFixture(t, nil)
	f.svc.EXPECT().Refresh(gomock.Any(), domain.OAuthRefresh{RefreshToken: "nfmcp_rt_x", ClientID: "https://claude.ai/oauth/client.json"}, gomock.Any()).
		Return(nil, domain.NewOAuthInvalidGrant("the refresh token was already used"))
	w := f.form("/oauth/token", "grant_type=refresh_token&refresh_token=nfmcp_rt_x&client_id=https%3A%2F%2Fclaude.ai%2Foauth%2Fclient.json")
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.JSONEq(t, `{"error":"invalid_grant","error_description":"the refresh token was already used"}`, w.Body.String())
	assert.Equal(t, "no-store", w.Header().Get("Cache-Control"))
}

func TestOAuthHandler_TokenRequestRefusals(t *testing.T) {
	f := newOAuthHandlerFixture(t, nil)
	for name, tc := range map[string]struct {
		req  oauthRequest
		want string
	}{
		"a JSON body": {
			req:  oauthRequest{method: http.MethodPost, path: "/oauth/token", body: `{"grant_type":"authorization_code"}`, contentType: "application/json"},
			want: `{"error":"invalid_request","error_description":"the body must be application/x-www-form-urlencoded"}`,
		},
		"a repeated resource": {
			req:  oauthRequest{method: http.MethodPost, path: "/oauth/token", body: "grant_type=authorization_code&code=c&resource=https%3A%2F%2Fnotifuse.example.com%2Fmcp&resource=https%3A%2F%2Fother.example%2Fmcp", contentType: "application/x-www-form-urlencoded"},
			want: `{"error":"invalid_request","error_description":"the resource parameter appears more than once"}`,
		},
		"no grant_type": {
			req:  oauthRequest{method: http.MethodPost, path: "/oauth/token", body: "code=c", contentType: "application/x-www-form-urlencoded"},
			want: `{"error":"invalid_request","error_description":"grant_type is required"}`,
		},
		"another grant_type": {
			req:  oauthRequest{method: http.MethodPost, path: "/oauth/token", body: "grant_type=client_credentials", contentType: "application/x-www-form-urlencoded"},
			want: `{"error":"unsupported_grant_type","error_description":"grant_type must be authorization_code or refresh_token"}`,
		},
		"the code in the query": {
			req:  oauthRequest{method: http.MethodPost, path: "/oauth/token?grant_type=authorization_code&code=c", body: "", contentType: "application/x-www-form-urlencoded"},
			want: `{"error":"invalid_request","error_description":"grant_type is required"}`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			w := f.serve(tc.req)
			assert.Equal(t, http.StatusBadRequest, w.Code)
			assert.JSONEq(t, tc.want, w.Body.String())
			assert.Equal(t, "no-store", w.Header().Get("Cache-Control"))
		})
	}

	t.Run("GET", func(t *testing.T) {
		w := f.serve(oauthRequest{path: "/oauth/token"})
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})
}

// oauth_token_credential: ten a minute per presented code or refresh token,
// whatever the IP; another code is not affected.
func TestOAuthHandler_TokenRateLimitPerCredential(t *testing.T) {
	f := newOAuthHandlerFixture(t, nil)
	f.svc.EXPECT().ExchangeCode(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, domain.NewOAuthInvalidGrant("unknown authorization code")).Times(11)
	for i := 0; i < 10; i++ {
		w := f.serve(oauthRequest{method: http.MethodPost, path: "/oauth/token", body: "grant_type=authorization_code&code=guessed", contentType: "application/x-www-form-urlencoded", remoteAddr: fmt.Sprintf("198.51.100.%d:1", i+1)})
		require.Equal(t, http.StatusBadRequest, w.Code, "attempt %d", i+1)
	}
	w := f.form("/oauth/token", "grant_type=authorization_code&code=guessed")
	assert.Equal(t, http.StatusTooManyRequests, w.Code)
	assert.Equal(t, "slow_down", decodeJSONBody(t, w)["error"])

	assert.Equal(t, http.StatusBadRequest, f.form("/oauth/token", "grant_type=authorization_code&code=another").Code)
}

// oauth_token_ip: six hundred a minute per IP, on the token and revocation
// endpoints together.
func TestOAuthHandler_TokenRateLimitPerIP(t *testing.T) {
	f := newOAuthHandlerFixture(t, nil)
	f.svc.EXPECT().Refresh(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, domain.NewOAuthInvalidGrant("unknown refresh token")).Times(300)
	f.svc.EXPECT().Revoke(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(300)
	for i := 0; i < 300; i++ {
		require.Equal(t, http.StatusBadRequest, f.form("/oauth/token", fmt.Sprintf("grant_type=refresh_token&refresh_token=t%d&client_id=c", i)).Code)
		require.Equal(t, http.StatusOK, f.form("/oauth/revoke", "token=t&client_id=c").Code)
	}
	assert.Equal(t, http.StatusTooManyRequests, f.form("/oauth/token", "grant_type=refresh_token&refresh_token=last&client_id=c").Code)
	assert.Equal(t, http.StatusTooManyRequests, f.form("/oauth/revoke", "token=t&client_id=c").Code)
}

func TestOAuthHandler_TokenPreflight(t *testing.T) {
	f := newOAuthHandlerFixture(t, nil)
	for _, path := range []string{"/oauth/token", "/oauth/revoke"} {
		w := f.serve(oauthRequest{method: http.MethodOptions, path: path, origin: "https://claude.ai"})
		assert.Equal(t, http.StatusNoContent, w.Code, path)
		assert.Equal(t, "*", w.Header().Get("Access-Control-Allow-Origin"), path)
		assert.Empty(t, w.Header().Get("Access-Control-Allow-Credentials"), path)
	}
}

// --- POST /oauth/revoke ---

func TestOAuthHandler_Revoke(t *testing.T) {
	f := newOAuthHandlerFixture(t, nil)
	f.svc.EXPECT().Revoke(gomock.Any(), domain.OAuthTokenRevocation{Token: "nfmcp_rt_x", ClientID: "https://claude.ai/oauth/client.json"}, gomock.Any()).Return(nil)
	w := f.form("/oauth/revoke", "token=nfmcp_rt_x&token_type_hint=refresh_token&client_id=https%3A%2F%2Fclaude.ai%2Foauth%2Fclient.json")
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Empty(t, w.Body.String())
	assert.Equal(t, "*", w.Header().Get("Access-Control-Allow-Origin"))

	f.svc.EXPECT().Revoke(gomock.Any(), domain.OAuthTokenRevocation{Token: "t"}, gomock.Any()).
		Return(&domain.OAuthError{Code: domain.OAuthErrInvalidRequest, Description: "token and client_id are required"})
	w = f.form("/oauth/revoke", "token=t")
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.JSONEq(t, `{"error":"invalid_request","error_description":"token and client_id are required"}`, w.Body.String())
}

// --- The console calls ---

func TestOAuthHandler_GetAuthorizationRequest(t *testing.T) {
	view := &domain.OAuthAuthorizationRequestView{RequestID: oauthTestRequestID, ClientName: "Claude", ClientHost: "claude.ai", KnownClient: true}

	t.Run("anonymous", func(t *testing.T) {
		f := newOAuthHandlerFixture(t, nil)
		f.svc.EXPECT().GetAuthorizationRequest(gomock.Any(), oauthTestRequestID).DoAndReturn(func(ctx context.Context, id string) (*domain.OAuthAuthorizationRequestView, error) {
			assert.Nil(t, ctx.Value(domain.UserIDKey))
			return view, nil
		})
		w := f.serve(oauthRequest{path: "/api/oauth.getAuthorizationRequest?request_id=" + oauthTestRequestID})
		require.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), `"client_name":"Claude"`)
		assert.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	})

	t.Run("with a console session, its claims reach the service", func(t *testing.T) {
		f := newOAuthHandlerFixture(t, nil)
		f.svc.EXPECT().GetAuthorizationRequest(gomock.Any(), oauthTestRequestID).DoAndReturn(func(ctx context.Context, id string) (*domain.OAuthAuthorizationRequestView, error) {
			assert.Equal(t, "u1", ctx.Value(domain.UserIDKey))
			assert.Equal(t, "user", ctx.Value(domain.UserTypeKey))
			assert.Equal(t, "s1", ctx.Value(domain.SessionIDKey))
			return view, nil
		})
		w := f.serve(oauthRequest{path: "/api/oauth.getAuthorizationRequest?request_id=" + oauthTestRequestID, authorization: "Bearer " + sessionToken(t, "u1", "s1")})
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("an API key or a broken token leaves the call anonymous", func(t *testing.T) {
		f := newOAuthHandlerFixture(t, nil)
		f.svc.EXPECT().GetAuthorizationRequest(gomock.Any(), oauthTestRequestID).DoAndReturn(func(ctx context.Context, id string) (*domain.OAuthAuthorizationRequestView, error) {
			assert.Nil(t, ctx.Value(domain.UserIDKey))
			return view, nil
		}).Times(3)
		for _, token := range []string{apiKeyToken(t, "key-1"), "not-a-jwt", oauthTestAccess} {
			w := f.serve(oauthRequest{path: "/api/oauth.getAuthorizationRequest?request_id=" + oauthTestRequestID, authorization: "Bearer " + token})
			assert.Equal(t, http.StatusOK, w.Code)
		}
	})

	t.Run("an expired request", func(t *testing.T) {
		f := newOAuthHandlerFixture(t, nil)
		f.svc.EXPECT().GetAuthorizationRequest(gomock.Any(), "gone").Return(nil, domain.ErrOAuthRequestUnavailable)
		w := f.serve(oauthRequest{path: "/api/oauth.getAuthorizationRequest?request_id=gone"})
		assert.Equal(t, http.StatusGone, w.Code)
		assert.Equal(t, "oauth_request_unavailable", decodeJSONBody(t, w)["error"])
	})
}

// The console call errors: a code the page switches on, and never a 401,
// which would sign the person out of the console. A dead session is the one
// 401, tested with each call below.
func TestOAuthHandler_ConsoleErrorCodes(t *testing.T) {
	for _, tc := range []struct {
		err    error
		status int
		code   string
	}{
		{domain.ErrOAuthRequestUnavailable, http.StatusGone, "oauth_request_unavailable"},
		{domain.ErrOAuthBrowserMismatch, http.StatusForbidden, "oauth_browser_mismatch"},
		{domain.ErrOAuthNotAnAPIKey, http.StatusBadRequest, "oauth_not_an_api_key"},
		{domain.ErrOAuthWorkspaceMCPDisabled, http.StatusConflict, "oauth_workspace_mcp_disabled"},
		{domain.ErrOAuthKeyNameTaken, http.StatusConflict, "oauth_key_name_taken"},
	} {
		f := newOAuthHandlerFixture(t, nil)
		f.svc.EXPECT().ApproveWithKey(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, tc.err)
		w := f.serve(oauthRequest{method: http.MethodPost, path: "/api/oauth.approveWithKey", body: `{"request_id":"r","api_key":"k"}`, contentType: "application/json"})
		assert.Equal(t, tc.status, w.Code, tc.code)
		assert.Equal(t, tc.code, decodeJSONBody(t, w)["error"])
	}
}

func TestOAuthHandler_ApproveWithKey(t *testing.T) {
	const key = "eyJhbGciOiJIUzI1NiJ9.pasted-key-value.signature"

	t.Run("the request cookie reaches the service, and is cleared after", func(t *testing.T) {
		f := newOAuthHandlerFixture(t, nil)
		f.svc.EXPECT().ApproveWithKey(gomock.Any(),
			domain.OAuthApproveWithKeyRequest{RequestID: oauthTestRequestID, APIKey: key, BrowserBinding: oauthTestBinding},
			domain.OAuthCallMeta{IP: "192.0.2.1", UserAgent: "agent/1", RequestID: "req-42"},
		).Return(&domain.OAuthRedirect{RedirectURL: "https://claude.ai/api/mcp/auth_callback?code=c&state=st-1&iss=https%3A%2F%2Fnotifuse.example.com"}, nil)
		w := f.serve(oauthRequest{method: http.MethodPost, path: "/api/oauth.approveWithKey", contentType: "application/json",
			body: fmt.Sprintf(`{"request_id":%q,"api_key":%q}`, oauthTestRequestID, key), cookies: []*http.Cookie{bindingCookie()}})
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		assert.JSONEq(t, `{"redirect_url":"https://claude.ai/api/mcp/auth_callback?code=c&state=st-1&iss=https%3A%2F%2Fnotifuse.example.com"}`, w.Body.String())
		cleared := w.Result().Cookies()
		require.Len(t, cleared, 1)
		assert.Equal(t, oauthTestCookie, cleared[0].Name)
		assert.Equal(t, -1, cleared[0].MaxAge)
	})

	t.Run("without the cookie the binding is empty", func(t *testing.T) {
		f := newOAuthHandlerFixture(t, nil)
		f.svc.EXPECT().ApproveWithKey(gomock.Any(), domain.OAuthApproveWithKeyRequest{RequestID: oauthTestRequestID, APIKey: key}, gomock.Any()).
			Return(nil, domain.ErrOAuthBrowserMismatch)
		// Another request's cookie does not count.
		other := &http.Cookie{Name: "__Host-nf_oauth_another", Value: oauthTestBinding}
		w := f.serve(oauthRequest{method: http.MethodPost, path: "/api/oauth.approveWithKey", contentType: "application/json",
			body: fmt.Sprintf(`{"request_id":%q,"api_key":%q}`, oauthTestRequestID, key), cookies: []*http.Cookie{other}})
		assert.Equal(t, http.StatusForbidden, w.Code)
		assert.Empty(t, w.Result().Cookies())
	})

	t.Run("the key is never logged", func(t *testing.T) {
		f := newOAuthHandlerFixture(t, nil)
		f.svc.EXPECT().ApproveWithKey(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, errors.New("database is down"))
		w := f.serve(oauthRequest{method: http.MethodPost, path: "/api/oauth.approveWithKey", contentType: "application/json",
			body: fmt.Sprintf(`{"request_id":%q,"api_key":%q}`, oauthTestRequestID, key)})
		assert.Equal(t, http.StatusInternalServerError, w.Code)
		logs := f.logs.all()
		assert.Contains(t, logs, "database is down", "the failure is logged")
		assert.NotContains(t, logs, "pasted-key-value")
		assert.NotContains(t, w.Body.String(), "pasted-key-value")
	})

	// oauth_approve: twenty a minute per IP.
	t.Run("rate limited per IP", func(t *testing.T) {
		f := newOAuthHandlerFixture(t, nil)
		f.svc.EXPECT().ApproveWithKey(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, domain.ErrOAuthNotAnAPIKey).Times(20)
		approve := func(remote string) int {
			return f.serve(oauthRequest{method: http.MethodPost, path: "/api/oauth.approveWithKey", contentType: "application/json",
				body: `{"request_id":"r","api_key":"guess"}`, remoteAddr: remote}).Code
		}
		for i := 0; i < 20; i++ {
			require.Equal(t, http.StatusBadRequest, approve("198.51.100.4:1"), "attempt %d", i+1)
		}
		assert.Equal(t, http.StatusTooManyRequests, approve("198.51.100.4:1"))
	})
}

func TestOAuthHandler_CreateKeyAndApprove(t *testing.T) {
	body := fmt.Sprintf(`{"request_id":%q,"workspace_id":"ws1","name":"claude","permissions":{"contacts":{"read":true,"write":false}}}`, oauthTestRequestID)

	t.Run("a console session, with the request cookie", func(t *testing.T) {
		f := newOAuthHandlerFixture(t, nil)
		f.svc.EXPECT().CreateKeyAndApprove(gomock.Any(), domain.OAuthCreateKeyAndApproveRequest{
			RequestID: oauthTestRequestID, WorkspaceID: "ws1", Name: "claude",
			Permissions:    domain.UserPermissions{domain.PermissionResourceContacts: {Read: true}},
			BrowserBinding: oauthTestBinding,
		}, gomock.Any()).DoAndReturn(func(ctx context.Context, req domain.OAuthCreateKeyAndApproveRequest, meta domain.OAuthCallMeta) (*domain.OAuthRedirect, error) {
			assert.Equal(t, "s1", ctx.Value(domain.SessionIDKey))
			return &domain.OAuthRedirect{RedirectURL: "https://claude.ai/cb?code=c"}, nil
		})
		w := f.serve(oauthRequest{method: http.MethodPost, path: "/api/oauth.createKeyAndApprove", contentType: "application/json", body: body,
			authorization: "Bearer " + sessionToken(t, "u1", "s1"), cookies: []*http.Cookie{bindingCookie()}})
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		assert.Equal(t, "https://claude.ai/cb?code=c", decodeJSONBody(t, w)["redirect_url"])
	})

	// The mock expects no call: an API key, an OAuth token and no credential
	// never reach the service.
	t.Run("refused to an API key, an OAuth token and no credential", func(t *testing.T) {
		f := newOAuthHandlerFixture(t, nil)
		for authorization, status := range map[string]int{
			"Bearer " + apiKeyToken(t, "key-1"): http.StatusForbidden,
			"Bearer " + oauthTestAccess:         http.StatusUnauthorized,
			"":                                  http.StatusUnauthorized,
		} {
			w := f.serve(oauthRequest{method: http.MethodPost, path: "/api/oauth.createKeyAndApprove", contentType: "application/json", body: body,
				authorization: authorization, cookies: []*http.Cookie{bindingCookie()}})
			assert.Equal(t, status, w.Code, authorization)
		}
	})

	t.Run("a session without the required second factor gets two_factor_required", func(t *testing.T) {
		f := newOAuthHandlerFixture(t, nil)
		f.svc.EXPECT().CreateKeyAndApprove(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, domain.ErrTwoFactorRequired)
		w := f.serve(oauthRequest{method: http.MethodPost, path: "/api/oauth.createKeyAndApprove", contentType: "application/json", body: body,
			authorization: "Bearer " + sessionToken(t, "u1", "s1"), cookies: []*http.Cookie{bindingCookie()}})
		assert.Equal(t, http.StatusForbidden, w.Code)
		assert.Equal(t, "two_factor_required", decodeJSONBody(t, w)["error"])
	})

	// workspaces.createAPIKey answers 409 for the same conflict.
	t.Run("a key name already taken gets 409", func(t *testing.T) {
		f := newOAuthHandlerFixture(t, nil)
		f.svc.EXPECT().CreateKeyAndApprove(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, domain.ErrOAuthKeyNameTaken)
		w := f.serve(oauthRequest{method: http.MethodPost, path: "/api/oauth.createKeyAndApprove", contentType: "application/json", body: body,
			authorization: "Bearer " + sessionToken(t, "u1", "s1"), cookies: []*http.Cookie{bindingCookie()}})
		assert.Equal(t, http.StatusConflict, w.Code)
		answer := decodeJSONBody(t, w)
		assert.Equal(t, "oauth_key_name_taken", answer["error"])
		assert.Contains(t, answer["message"], "pick another name")
	})

	t.Run("a session that failed verification gets 401", func(t *testing.T) {
		f := newOAuthHandlerFixture(t, nil)
		f.svc.EXPECT().CreateKeyAndApprove(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, oauthDeadSessionErr)
		w := f.serve(oauthRequest{method: http.MethodPost, path: "/api/oauth.createKeyAndApprove", contentType: "application/json", body: body,
			authorization: "Bearer " + sessionToken(t, "u1", "s1"), cookies: []*http.Cookie{bindingCookie()}})
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("a member is refused", func(t *testing.T) {
		f := newOAuthHandlerFixture(t, nil)
		f.svc.EXPECT().CreateKeyAndApprove(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, &domain.ErrUnauthorized{Message: "user is not an owner of the workspace"})
		w := f.serve(oauthRequest{method: http.MethodPost, path: "/api/oauth.createKeyAndApprove", contentType: "application/json", body: body,
			authorization: "Bearer " + sessionToken(t, "u1", "s1")})
		assert.Equal(t, http.StatusForbidden, w.Code)
		assert.Equal(t, "user is not an owner of the workspace", decodeJSONBody(t, w)["error"])
	})
}

// oauthDeadSessionErr is what the service returns for a console session that
// failed verification, as AuthService wraps it.
var oauthDeadSessionErr = fmt.Errorf("%w: %w", domain.ErrSessionAuthFailed, errors.New("session expired"))

func TestOAuthHandler_Deny(t *testing.T) {
	f := newOAuthHandlerFixture(t, nil)
	f.svc.EXPECT().Deny(gomock.Any(), domain.OAuthDenyRequest{RequestID: oauthTestRequestID, BrowserBinding: oauthTestBinding}).
		Return(&domain.OAuthRedirect{RedirectURL: "https://claude.ai/cb?error=access_denied"}, nil)
	w := f.serve(oauthRequest{method: http.MethodPost, path: "/api/oauth.deny", contentType: "application/json",
		body: fmt.Sprintf(`{"request_id":%q}`, oauthTestRequestID), cookies: []*http.Cookie{bindingCookie()}})
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "https://claude.ai/cb?error=access_denied", decodeJSONBody(t, w)["redirect_url"])
	assert.Equal(t, -1, w.Result().Cookies()[0].MaxAge)
}

func TestOAuthHandler_ListWorkspaceGrants(t *testing.T) {
	t.Run("an owner", func(t *testing.T) {
		f := newOAuthHandlerFixture(t, nil)
		f.svc.EXPECT().ListWorkspaceGrants(gomock.Any(), "ws1").Return(nil, nil)
		w := f.serve(oauthRequest{path: "/api/oauth.listWorkspaceGrants?workspace_id=ws1", authorization: "Bearer " + sessionToken(t, "u1", "s1")})
		require.Equal(t, http.StatusOK, w.Code)
		assert.JSONEq(t, `{"grants":[]}`, w.Body.String())
	})

	t.Run("a member", func(t *testing.T) {
		f := newOAuthHandlerFixture(t, nil)
		f.svc.EXPECT().ListWorkspaceGrants(gomock.Any(), "ws1").Return(nil, &domain.ErrUnauthorized{Message: "user is not an owner of the workspace"})
		w := f.serve(oauthRequest{path: "/api/oauth.listWorkspaceGrants?workspace_id=ws1", authorization: "Bearer " + sessionToken(t, "u1", "s1")})
		assert.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("a session that failed verification gets 401", func(t *testing.T) {
		f := newOAuthHandlerFixture(t, nil)
		f.svc.EXPECT().ListWorkspaceGrants(gomock.Any(), "ws1").Return(nil, oauthDeadSessionErr)
		w := f.serve(oauthRequest{path: "/api/oauth.listWorkspaceGrants?workspace_id=ws1", authorization: "Bearer " + sessionToken(t, "u1", "s1")})
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	// OAuth access tokens are opaque: RequireAuth cannot parse them as a JWT,
	// so they never reach an /api/ handler.
	t.Run("an OAuth access token is refused on /api/", func(t *testing.T) {
		f := newOAuthHandlerFixture(t, nil)
		w := f.serve(oauthRequest{path: "/api/oauth.listWorkspaceGrants?workspace_id=ws1", authorization: "Bearer " + oauthTestAccess})
		assert.Equal(t, http.StatusUnauthorized, w.Code)
		assert.Contains(t, decodeJSONBody(t, w)["error"], "Invalid token")
	})
}

func TestOAuthHandler_RevokeGrant(t *testing.T) {
	f := newOAuthHandlerFixture(t, nil)
	f.svc.EXPECT().RevokeGrant(gomock.Any(), "grant-1", gomock.Any()).Return(nil)
	w := f.serve(oauthRequest{method: http.MethodPost, path: "/api/oauth.revokeGrant", contentType: "application/json", body: `{"grant_id":"grant-1"}`,
		authorization: "Bearer " + sessionToken(t, "u1", "s1")})
	assert.Equal(t, http.StatusOK, w.Code)

	f.svc.EXPECT().RevokeGrant(gomock.Any(), "grant-2", gomock.Any()).Return(domain.ErrOAuthGrantNotFound)
	w = f.serve(oauthRequest{method: http.MethodPost, path: "/api/oauth.revokeGrant", contentType: "application/json", body: `{"grant_id":"grant-2"}`,
		authorization: "Bearer " + sessionToken(t, "u1", "s1")})
	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Equal(t, "oauth_grant_not_found", decodeJSONBody(t, w)["error"])

	f.svc.EXPECT().RevokeGrant(gomock.Any(), "grant-3", gomock.Any()).Return(oauthDeadSessionErr)
	w = f.serve(oauthRequest{method: http.MethodPost, path: "/api/oauth.revokeGrant", contentType: "application/json", body: `{"grant_id":"grant-3"}`,
		authorization: "Bearer " + sessionToken(t, "u1", "s1")})
	assert.Equal(t, http.StatusUnauthorized, w.Code, "a session that failed verification")
}
