package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"strings"

	"github.com/golang-jwt/jwt/v5"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/http/middleware"
	"github.com/Notifuse/notifuse/internal/service"
	"github.com/Notifuse/notifuse/pkg/logger"
)

// Rate-limit namespaces of the OAuth authorization server, all per minute.
const (
	// OAuthAuthorizeRateLimitNamespace bounds GET /oauth/authorize per IP: each
	// new client_id costs a metadata document fetch.
	OAuthAuthorizeRateLimitNamespace = "oauth_authorize"
	// OAuthApproveRateLimitNamespace bounds oauth.approveWithKey per IP, the
	// one call that checks a pasted API key without a session.
	OAuthApproveRateLimitNamespace = "oauth_approve"
	// OAuthTokenIPRateLimitNamespace bounds the token and revocation endpoints
	// per IP. Hosted clients share their egress addresses, so it is generous.
	OAuthTokenIPRateLimitNamespace = "oauth_token_ip"
	// OAuthTokenCredentialRateLimitNamespace bounds the token endpoint per
	// presented code or refresh token, keyed on its SHA-256.
	OAuthTokenCredentialRateLimitNamespace = "oauth_token_credential"
)

// oauthRequestCookiePrefix names the cookie that binds an authorization
// request to the browser that started it. Each request has its own cookie, so
// two applications connected at once in one browser do not overwrite each
// other's. Over https the __Host- prefix pins it to this exact origin.
const (
	oauthRequestCookiePrefix       = "nf_oauth_"
	oauthSecureRequestCookiePrefix = "__Host-nf_oauth_"
)

// oauthFormMaxBytes caps the form body of the token and revocation endpoints.
const oauthFormMaxBytes = 64 * 1024

// OAuthHandlerConfig configures the OAuth handler.
type OAuthHandlerConfig struct {
	// Enabled is MCP_ENABLED: the authorization server exists for /mcp only.
	Enabled bool
	// IsDemo turns it off whatever Enabled says.
	IsDemo bool
	// APIEndpoint returns API_ENDPOINT, read at each request.
	APIEndpoint  func() string
	Service      domain.OAuthService
	GetJWTSecret func() ([]byte, error)
	RateLimiter  MCPRateLimiter
	Logger       logger.Logger
}

// OAuthHandler serves the OAuth authorization server of /mcp: the discovery
// documents, /oauth/authorize, /oauth/token and /oauth/revoke on the canonical
// host, and the console calls of the authorization page and the MCP tab on
// /api/. Every response of the first group runs, in this order: the instance
// switch and demo mode, the host check, the OAuth availability check, CORS,
// then the endpoint.
type OAuthHandler struct {
	cfg OAuthHandlerConfig
}

// NewOAuthHandler builds the OAuth handler.
func NewOAuthHandler(cfg OAuthHandlerConfig) *OAuthHandler {
	return &OAuthHandler{cfg: cfg}
}

// RegisterRoutes mounts the endpoints. Each .well-known document is mounted
// with its subtree, so that a path under it answers JSON rather than the
// console redirect.
func (h *OAuthHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.Handle(middleware.OAuthProtectedResourcePath, h.public(h.protectedResourceMetadata))
	mux.Handle(middleware.OAuthProtectedResourcePath+"/", h.public(h.protectedResourceMetadata))
	mux.Handle(middleware.OAuthAuthorizationServerPath, h.public(h.authorizationServerMetadata))
	mux.Handle(middleware.OAuthAuthorizationServerPath+"/", h.public(h.authorizationServerMetadata))
	mux.HandleFunc(middleware.OpenIDConfigurationPath, h.openIDConfiguration)
	mux.HandleFunc(middleware.OpenIDConfigurationPath+"/", h.openIDConfiguration)
	mux.Handle("/oauth", h.public(h.notFound))
	mux.Handle(middleware.OAuthPathPrefix, h.public(h.notFound))
	mux.Handle(middleware.OAuthAuthorizePath, h.public(h.authorize))
	mux.Handle(middleware.OAuthTokenPath, h.public(h.token))
	mux.Handle(middleware.OAuthRevokePath, h.public(h.revoke))

	requireAuth := middleware.NewAuthMiddleware(h.cfg.GetJWTSecret).RequireAuth()
	mux.Handle("/api/oauth.getAuthorizationRequest", h.available(http.HandlerFunc(h.getAuthorizationRequest)))
	mux.Handle("/api/oauth.approveWithKey", h.available(http.HandlerFunc(h.approveWithKey)))
	mux.Handle("/api/oauth.createKeyAndApprove", h.available(requireAuth(http.HandlerFunc(h.createKeyAndApprove))))
	mux.Handle("/api/oauth.deny", h.available(http.HandlerFunc(h.deny)))
	mux.Handle("/api/oauth.listWorkspaceGrants", requireAuth(http.HandlerFunc(h.listWorkspaceGrants)))
	mux.Handle("/api/oauth.revokeGrant", requireAuth(http.HandlerFunc(h.revokeGrant)))
}

// oauthEndpoint is a public endpoint, called once the canonical endpoint is
// known and the request has passed the checks every public endpoint shares.
type oauthEndpoint func(w http.ResponseWriter, r *http.Request, endpoint domain.CanonicalEndpoint)

// unavailableReason says why the authorization server answers 404, or "".
func (h *OAuthHandler) unavailableReason() string {
	if !h.cfg.Enabled || h.cfg.IsDemo {
		return "MCP is not enabled on this server"
	}
	if h.cfg.Service == nil {
		return "OAuth is unavailable on this server"
	}
	if reason := h.cfg.Service.Unavailable(); reason != "" {
		return "OAuth is unavailable on this server: " + reason
	}
	return ""
}

// public runs the checks of the discovery documents and the /oauth/
// endpoints: the switches, the host, OAuth availability. A refusal is a JSON
// 404, never the console redirect.
func (h *OAuthHandler) public(next oauthEndpoint) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !h.cfg.Enabled || h.cfg.IsDemo {
			WriteJSONError(w, "MCP is not enabled on this server", http.StatusNotFound)
			return
		}
		endpoint, err := domain.CanonicalAPIEndpoint(h.apiEndpoint())
		if err != nil {
			WriteJSONError(w, "OAuth is unavailable on this server: "+err.Error(), http.StatusNotFound)
			return
		}
		if !endpoint.MatchesHost(r.Host) {
			writeJSON(w, http.StatusNotFound, map[string]string{
				"error":         "OAuth is served on " + endpoint.Host + " only",
				"expected_host": endpoint.Host,
			})
			return
		}
		if reason := h.unavailableReason(); reason != "" {
			WriteJSONError(w, reason, http.StatusNotFound)
			return
		}
		next(w, r, endpoint)
	})
}

// available refuses the authorization page's calls with a JSON 404 while the
// authorization server is off.
func (h *OAuthHandler) available(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if reason := h.unavailableReason(); reason != "" {
			WriteJSONError(w, reason, http.StatusNotFound)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (h *OAuthHandler) apiEndpoint() string {
	if h.cfg.APIEndpoint == nil {
		return ""
	}
	return h.cfg.APIEndpoint()
}

func (h *OAuthHandler) allow(namespace, key string) bool {
	return h.cfg.RateLimiter == nil || h.cfg.RateLimiter.Allow(namespace, key)
}

func callMeta(w http.ResponseWriter, r *http.Request) domain.OAuthCallMeta {
	return domain.OAuthCallMeta{
		IP:        middleware.ClientIP(r),
		UserAgent: r.UserAgent(),
		RequestID: w.Header().Get(middleware.RequestIDHeader),
	}
}

// ProtectedResourceMetadataURL is the URL of the protected resource metadata
// of /mcp (RFC 9728), which a 401 on /mcp names in resource_metadata.
func ProtectedResourceMetadataURL(endpoint domain.CanonicalEndpoint) string {
	return endpoint.Origin + middleware.OAuthProtectedResourcePath + domain.MCPPath
}

// protectedResourceMetadata answers GET /.well-known/oauth-protected-resource
// and its /mcp variant (RFC 9728). offline_access is the authorization
// server's business and is not listed here.
func (h *OAuthHandler) protectedResourceMetadata(w http.ResponseWriter, r *http.Request, endpoint domain.CanonicalEndpoint) {
	if middleware.ApplyOAuthCORS(w, r) {
		return
	}
	if r.URL.Path != middleware.OAuthProtectedResourcePath && r.URL.Path != middleware.OAuthProtectedResourcePath+domain.MCPPath {
		WriteJSONError(w, "not found: the protected resource metadata is at "+ProtectedResourceMetadataURL(endpoint), http.StatusNotFound)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"resource":                 endpoint.MCPURL(),
		"authorization_servers":    []string{endpoint.URL},
		"scopes_supported":         []string{domain.OAuthScope},
		"bearer_methods_supported": []string{"header"},
	})
}

// authorizationServerMetadata answers GET
// /.well-known/oauth-authorization-server (RFC 8414). There is no
// registration_endpoint: clients identify themselves with a Client ID
// Metadata Document.
func (h *OAuthHandler) authorizationServerMetadata(w http.ResponseWriter, r *http.Request, endpoint domain.CanonicalEndpoint) {
	if middleware.ApplyOAuthCORS(w, r) {
		return
	}
	if r.URL.Path != middleware.OAuthAuthorizationServerPath {
		WriteJSONError(w, "not found: the authorization server metadata is at "+endpoint.Origin+middleware.OAuthAuthorizationServerPath, http.StatusNotFound)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"issuer":                                         endpoint.URL,
		"authorization_endpoint":                         endpoint.URL + middleware.OAuthAuthorizePath,
		"token_endpoint":                                 endpoint.URL + middleware.OAuthTokenPath,
		"revocation_endpoint":                            endpoint.URL + middleware.OAuthRevokePath,
		"response_types_supported":                       []string{"code"},
		"grant_types_supported":                          []string{"authorization_code", "refresh_token"},
		"code_challenge_methods_supported":               []string{"S256"},
		"token_endpoint_auth_methods_supported":          []string{"none"},
		"revocation_endpoint_auth_methods_supported":     []string{"none"},
		"client_id_metadata_document_supported":          true,
		"authorization_response_iss_parameter_supported": true,
		"scopes_supported":                               []string{domain.OAuthScope, "offline_access"},
	})
}

// openIDConfiguration always answers a JSON 404: Notifuse is no OpenID
// provider, and a client probing for one must not get the console.
func (h *OAuthHandler) openIDConfiguration(w http.ResponseWriter, r *http.Request) {
	WriteJSONError(w, "not found: this server is not an OpenID provider", http.StatusNotFound)
}

// notFound answers every /oauth/ path without an endpoint, /oauth/register
// included: clients register with a Client ID Metadata Document.
func (h *OAuthHandler) notFound(w http.ResponseWriter, r *http.Request, endpoint domain.CanonicalEndpoint) {
	WriteJSONError(w, "not found", http.StatusNotFound)
}

// --- GET /oauth/authorize ---

// securityHeaders are the headers of every authorization response: never
// cached, never framed, and never leaking its query (state, code challenge)
// in a Referer.
func securityHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'")
	w.Header().Set("Referrer-Policy", "no-referrer")
}

// authorizeParams are the parameters GET /oauth/authorize reads; each may
// appear once (RFC 6749 §3.1).
var authorizeParams = []string{"response_type", "client_id", "redirect_uri", "state", "code_challenge", "code_challenge_method", "resource", "scope"}

// authorize validates an authorization request through the service. A
// refusal is an error page and never a redirect: until the client is
// checked, its redirect URI is nobody's. On success it binds the request to
// this browser with a cookie and sends the browser to the console's
// authorization page.
func (h *OAuthHandler) authorize(w http.ResponseWriter, r *http.Request, endpoint domain.CanonicalEndpoint) {
	securityHeaders(w)
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		renderOAuthErrorPage(w, http.StatusMethodNotAllowed, "invalid_request", "This address only takes GET requests.")
		return
	}
	if !h.allow(OAuthAuthorizeRateLimitNamespace, middleware.ClientIP(r)) {
		w.Header().Set("Retry-After", "60")
		renderOAuthErrorPage(w, http.StatusTooManyRequests, "rate_limited", "Too many connection attempts from this address. Wait a minute, then start again from your application.")
		return
	}
	query := r.URL.Query()
	for _, name := range authorizeParams {
		if len(query[name]) > 1 {
			renderOAuthErrorPage(w, http.StatusBadRequest, "invalid_request", fmt.Sprintf("The %s parameter appears more than once.", name))
			return
		}
	}
	started, err := h.cfg.Service.CreateAuthorizationRequest(r.Context(), domain.OAuthAuthorizeParams{
		ResponseType:        query.Get("response_type"),
		ClientID:            query.Get("client_id"),
		RedirectURI:         query.Get("redirect_uri"),
		State:               query.Get("state"),
		CodeChallenge:       query.Get("code_challenge"),
		CodeChallengeMethod: query.Get("code_challenge_method"),
		Resource:            query.Get("resource"),
	})
	var refusal *domain.OAuthAuthorizeError
	switch {
	case errors.As(err, &refusal):
		renderOAuthErrorPage(w, http.StatusBadRequest, refusal.Reason, refusal.Message)
		return
	case errors.Is(err, domain.ErrOAuthUnavailable):
		WriteJSONError(w, "OAuth is unavailable on this server", http.StatusNotFound)
		return
	case err != nil:
		h.logError("Failed to start an OAuth authorization request", err)
		renderOAuthErrorPage(w, http.StatusInternalServerError, "server_error", "The server could not start the connection. Try again from your application.")
		return
	}
	http.SetCookie(w, h.requestCookie(endpoint, started.RequestID, started.BrowserBinding, int(domain.OAuthRequestLifetime.Seconds())))
	http.Redirect(w, r, "/console/oauth/authorize?request="+url.QueryEscape(started.RequestID), http.StatusFound)
}

// requestCookieName is the name of a request's binding cookie. The __Host-
// prefix needs Secure, which a browser refuses on plain http: over loopback
// http (development) the cookie goes without both, as the OIDC flow cookie
// does.
func requestCookieName(endpoint domain.CanonicalEndpoint, requestID string) string {
	if endpoint.Scheme == "https" {
		return oauthSecureRequestCookiePrefix + requestID
	}
	return oauthRequestCookiePrefix + requestID
}

// requestCookie is the cookie binding a request to the browser: HttpOnly,
// SameSite=Lax (it must reach the API from the console page, same site), path
// /, and gone with the request. A negative maxAge deletes it.
func (h *OAuthHandler) requestCookie(endpoint domain.CanonicalEndpoint, requestID, value string, maxAge int) *http.Cookie {
	return &http.Cookie{
		Name:     requestCookieName(endpoint, requestID),
		Value:    value,
		Path:     "/",
		MaxAge:   maxAge,
		Secure:   endpoint.Scheme == "https",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	}
}

// browserBinding reads the cookie of a request, "" when there is none. A
// request ID that cannot be a cookie name has no cookie.
func (h *OAuthHandler) browserBinding(r *http.Request, requestID string) string {
	if requestID == "" || strings.ContainsAny(requestID, " \t\r\n;,=\"\\") {
		return ""
	}
	endpoint, err := domain.CanonicalAPIEndpoint(h.apiEndpoint())
	if err != nil {
		return ""
	}
	cookie, err := r.Cookie(requestCookieName(endpoint, requestID))
	if err != nil {
		return ""
	}
	return cookie.Value
}

// clearBinding deletes the cookie of a request the browser has finished.
func (h *OAuthHandler) clearBinding(w http.ResponseWriter, requestID string) {
	endpoint, err := domain.CanonicalAPIEndpoint(h.apiEndpoint())
	if err != nil || requestID == "" {
		return
	}
	http.SetCookie(w, h.requestCookie(endpoint, requestID, "", -1))
}

var oauthErrorPage = template.Must(template.New("oauth-error").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="robots" content="noindex">
<title>Connection refused · Notifuse</title>
<style>
body{font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,sans-serif;background:#f5f5f7;color:#1d1d1f;margin:0;display:flex;min-height:100vh;align-items:center;justify-content:center}
main{background:#fff;border-radius:12px;box-shadow:0 1px 3px rgba(0,0,0,.08);padding:32px;max-width:480px;margin:16px}
h1{font-size:20px;margin:0 0 12px}
p{line-height:1.5;margin:0 0 12px}
code{font-size:13px;color:#6e6e73}
</style>
</head>
<body>
<main>
<h1>This application cannot connect to Notifuse</h1>
<p>{{.Message}}</p>
<p>Nothing was shared with the application. You can close this page.</p>
<p><code>{{.Reason}}</code></p>
</main>
</body>
</html>
`))

// renderOAuthErrorPage writes the error page of /oauth/authorize. The
// message is escaped: it may quote what the client sent.
func renderOAuthErrorPage(w http.ResponseWriter, status int, reason, message string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_ = oauthErrorPage.Execute(w, struct{ Reason, Message string }{Reason: reason, Message: message})
}

// --- POST /oauth/token and POST /oauth/revoke ---

// writeOAuthError answers an RFC 6749 §5.2 error: 400, or 401 for
// invalid_client.
func writeOAuthError(w http.ResponseWriter, status int, code, description string) {
	body := map[string]string{"error": code}
	if description != "" {
		body["error_description"] = description
	}
	writeJSON(w, status, body)
}

// readForm parses a form-urlencoded POST body, and only the body: a code or a
// token never travels in a URL. A parameter sent twice is invalid_request
// (RFC 6749 §3.2), so a client cannot send two resources and be bound to one.
func readForm(w http.ResponseWriter, r *http.Request) (url.Values, bool) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeOAuthError(w, http.StatusMethodNotAllowed, domain.OAuthErrInvalidRequest, "use POST")
		return nil, false
	}
	mediaType := strings.ToLower(strings.TrimSpace(strings.Split(r.Header.Get("Content-Type"), ";")[0]))
	if mediaType != "application/x-www-form-urlencoded" {
		writeOAuthError(w, http.StatusBadRequest, domain.OAuthErrInvalidRequest, "the body must be application/x-www-form-urlencoded")
		return nil, false
	}
	r.Body = http.MaxBytesReader(w, r.Body, oauthFormMaxBytes)
	if err := r.ParseForm(); err != nil {
		writeOAuthError(w, http.StatusBadRequest, domain.OAuthErrInvalidRequest, "the body is not a valid form")
		return nil, false
	}
	for name, values := range r.PostForm {
		if len(values) > 1 {
			writeOAuthError(w, http.StatusBadRequest, domain.OAuthErrInvalidRequest, "the "+name+" parameter appears more than once")
			return nil, false
		}
	}
	return r.PostForm, true
}

// writeTokenError answers a service error of the token or revocation
// endpoint.
func (h *OAuthHandler) writeTokenError(w http.ResponseWriter, err error, action string) {
	var oauthErr *domain.OAuthError
	switch {
	case errors.As(err, &oauthErr):
		status := http.StatusBadRequest
		if oauthErr.Code == domain.OAuthErrInvalidClient {
			status = http.StatusUnauthorized
		}
		writeOAuthError(w, status, oauthErr.Code, oauthErr.Description)
	case errors.Is(err, domain.ErrOAuthUnavailable):
		WriteJSONError(w, "OAuth is unavailable on this server", http.StatusNotFound)
	default:
		h.logError(action, err)
		writeOAuthError(w, http.StatusInternalServerError, "server_error", "")
	}
}

func tooManyTokenRequests(w http.ResponseWriter) {
	w.Header().Set("Retry-After", "60")
	writeOAuthError(w, http.StatusTooManyRequests, "slow_down", "too many requests: try again in a minute")
}

// token is POST /oauth/token: the authorization_code and refresh_token
// grants, for public clients (no client authentication). The response is
// never cached.
func (h *OAuthHandler) token(w http.ResponseWriter, r *http.Request, endpoint domain.CanonicalEndpoint) {
	if middleware.ApplyOAuthCORS(w, r) {
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	if !h.allow(OAuthTokenIPRateLimitNamespace, middleware.ClientIP(r)) {
		tooManyTokenRequests(w)
		return
	}
	form, ok := readForm(w, r)
	if !ok {
		return
	}
	meta := callMeta(w, r)

	var response *domain.OAuthTokenResponse
	var err error
	switch grantType := form.Get("grant_type"); grantType {
	case "authorization_code":
		if code := form.Get("code"); code != "" && !h.allow(OAuthTokenCredentialRateLimitNamespace, domain.HashOAuthSecret(code)) {
			tooManyTokenRequests(w)
			return
		}
		response, err = h.cfg.Service.ExchangeCode(r.Context(), domain.OAuthCodeExchange{
			Code:         form.Get("code"),
			ClientID:     form.Get("client_id"),
			RedirectURI:  form.Get("redirect_uri"),
			CodeVerifier: form.Get("code_verifier"),
			Resource:     form.Get("resource"),
		}, meta)
	case "refresh_token":
		if token := form.Get("refresh_token"); token != "" && !h.allow(OAuthTokenCredentialRateLimitNamespace, domain.HashOAuthSecret(token)) {
			tooManyTokenRequests(w)
			return
		}
		response, err = h.cfg.Service.Refresh(r.Context(), domain.OAuthRefresh{
			RefreshToken: form.Get("refresh_token"),
			ClientID:     form.Get("client_id"),
			Resource:     form.Get("resource"),
		}, meta)
	case "":
		writeOAuthError(w, http.StatusBadRequest, domain.OAuthErrInvalidRequest, "grant_type is required")
		return
	default:
		writeOAuthError(w, http.StatusBadRequest, domain.OAuthErrUnsupportedGrantType, "grant_type must be authorization_code or refresh_token")
		return
	}
	if err != nil {
		h.writeTokenError(w, err, "Failed to answer an OAuth token request")
		return
	}
	writeJSON(w, http.StatusOK, response)
}

// revoke is POST /oauth/revoke (RFC 7009). An unknown token, or another
// client's, answers 200 like a revoked one.
func (h *OAuthHandler) revoke(w http.ResponseWriter, r *http.Request, endpoint domain.CanonicalEndpoint) {
	if middleware.ApplyOAuthCORS(w, r) {
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	if !h.allow(OAuthTokenIPRateLimitNamespace, middleware.ClientIP(r)) {
		tooManyTokenRequests(w)
		return
	}
	form, ok := readForm(w, r)
	if !ok {
		return
	}
	err := h.cfg.Service.Revoke(r.Context(), domain.OAuthTokenRevocation{Token: form.Get("token"), ClientID: form.Get("client_id")}, callMeta(w, r))
	if err != nil {
		h.writeTokenError(w, err, "Failed to answer an OAuth revocation request")
		return
	}
	w.WriteHeader(http.StatusOK)
}

// --- The console calls (/api/oauth.*) ---

// Error codes of the console calls, in the "error" key of the body. The
// authorization page switches on them; "message" is for people.
const (
	oauthRequestUnavailableCode    = "oauth_request_unavailable"
	oauthBrowserMismatchCode       = "oauth_browser_mismatch"
	oauthNotAnAPIKeyCode           = "oauth_not_an_api_key"
	oauthWorkspaceMCPDisabledCode  = "oauth_workspace_mcp_disabled"
	oauthKeyNameTakenCode          = "oauth_key_name_taken"
	oauthGrantNotFoundCode         = "oauth_grant_not_found"
	oauthTooManyApprovalsCode      = "rate_limited"
	oauthUnavailableOnServerReason = "OAuth is unavailable on this server"
)

// writeConsoleError answers an error of a console call. None of the
// request's refusals is a 401: the console drops its session on any 401.
// Only a console session that failed verification answers 401, through
// writeServiceError, as on every other workspace route.
func (h *OAuthHandler) writeConsoleError(w http.ResponseWriter, err error, action string) {
	code := func(status int, code string) {
		writeJSON(w, status, map[string]string{"error": code, "message": err.Error()})
	}
	switch {
	case errors.Is(err, domain.ErrOAuthRequestUnavailable):
		code(http.StatusGone, oauthRequestUnavailableCode)
	case errors.Is(err, domain.ErrOAuthBrowserMismatch):
		code(http.StatusForbidden, oauthBrowserMismatchCode)
	case errors.Is(err, domain.ErrOAuthNotAnAPIKey):
		code(http.StatusBadRequest, oauthNotAnAPIKeyCode)
	case errors.Is(err, domain.ErrOAuthWorkspaceMCPDisabled):
		code(http.StatusConflict, oauthWorkspaceMCPDisabledCode)
	case errors.Is(err, domain.ErrOAuthKeyNameTaken):
		code(http.StatusConflict, oauthKeyNameTakenCode)
	case errors.Is(err, domain.ErrOAuthGrantNotFound):
		code(http.StatusNotFound, oauthGrantNotFoundCode)
	case errors.Is(err, domain.ErrOAuthUnavailable):
		WriteJSONError(w, oauthUnavailableOnServerReason, http.StatusNotFound)
	case writeServiceError(w, err, "You are not allowed to do this"):
	default:
		h.logError(action, err)
		WriteJSONError(w, "Internal server error", http.StatusInternalServerError)
	}
}

func decodeOAuthCall(w http.ResponseWriter, r *http.Request, v interface{}) bool {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return false
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, oauthFormMaxBytes)).Decode(v); err != nil {
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return false
	}
	return true
}

// optionalSession reads the console session of a call that needs none: a
// valid session token of type user puts its claims in the context, as
// RequireAuth does; anything else leaves the call anonymous.
func (h *OAuthHandler) optionalSession(r *http.Request) context.Context {
	ctx := r.Context()
	scheme, token, found := strings.Cut(r.Header.Get("Authorization"), " ")
	if !found || scheme != "Bearer" || token == "" || h.cfg.GetJWTSecret == nil {
		return ctx
	}
	secret, err := h.cfg.GetJWTSecret()
	if err != nil {
		return ctx
	}
	claims := &service.UserClaims{}
	parsed, err := jwt.ParseWithClaims(token, claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return secret, nil
	})
	if err != nil || !parsed.Valid || claims.UserID == "" || claims.Type != string(domain.UserTypeUser) || claims.SessionID == "" {
		return ctx
	}
	ctx = context.WithValue(ctx, domain.UserIDKey, claims.UserID)
	ctx = context.WithValue(ctx, domain.UserTypeKey, claims.Type)
	return context.WithValue(ctx, domain.SessionIDKey, claims.SessionID)
}

// getAuthorizationRequest is GET /api/oauth.getAuthorizationRequest: what the
// authorization page shows. With the console session of an owner it also
// lists the workspaces the caller owns.
func (h *OAuthHandler) getAuthorizationRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	view, err := h.cfg.Service.GetAuthorizationRequest(h.optionalSession(r), r.URL.Query().Get("request_id"))
	if err != nil {
		h.writeConsoleError(w, err, "Failed to read an OAuth authorization request")
		return
	}
	writeJSON(w, http.StatusOK, view)
}

// approveWithKey is POST /api/oauth.approveWithKey: the key pasted on the
// page. No session; limited per IP. The body, which holds the key, is never
// logged.
func (h *OAuthHandler) approveWithKey(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method == http.MethodPost && !h.allow(OAuthApproveRateLimitNamespace, middleware.ClientIP(r)) {
		w.Header().Set("Retry-After", "60")
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": oauthTooManyApprovalsCode, "message": "Too many attempts from this address: wait a minute, then try again"})
		return
	}
	var req domain.OAuthApproveWithKeyRequest
	if !decodeOAuthCall(w, r, &req) {
		return
	}
	req.BrowserBinding = h.browserBinding(r, req.RequestID)
	redirect, err := h.cfg.Service.ApproveWithKey(r.Context(), req, callMeta(w, r))
	if err != nil {
		h.writeConsoleError(w, err, "Failed to approve an OAuth request with an API key")
		return
	}
	h.clearBinding(w, req.RequestID)
	writeJSON(w, http.StatusOK, redirect)
}

// createKeyAndApprove is POST /api/oauth.createKeyAndApprove, the owner
// shortcut: a console session only, never an API key, and through
// RequireAuth never an OAuth token, which is no JWT. The session must pass
// the instance's two-factor requirement, which the service's workspace check
// applies.
func (h *OAuthHandler) createKeyAndApprove(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if !isConsoleSession(r.Context()) {
		WriteJSONError(w, "Creating a key for an application needs a console session", http.StatusForbidden)
		return
	}
	var req domain.OAuthCreateKeyAndApproveRequest
	if !decodeOAuthCall(w, r, &req) {
		return
	}
	req.BrowserBinding = h.browserBinding(r, req.RequestID)
	redirect, err := h.cfg.Service.CreateKeyAndApprove(r.Context(), req, callMeta(w, r))
	if err != nil {
		h.writeConsoleError(w, err, "Failed to create a key for an OAuth request")
		return
	}
	h.clearBinding(w, req.RequestID)
	writeJSON(w, http.StatusOK, redirect)
}

// deny is POST /api/oauth.deny.
func (h *OAuthHandler) deny(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	var req domain.OAuthDenyRequest
	if !decodeOAuthCall(w, r, &req) {
		return
	}
	req.BrowserBinding = h.browserBinding(r, req.RequestID)
	redirect, err := h.cfg.Service.Deny(r.Context(), req)
	if err != nil {
		h.writeConsoleError(w, err, "Failed to deny an OAuth request")
		return
	}
	h.clearBinding(w, req.RequestID)
	writeJSON(w, http.StatusOK, redirect)
}

// listWorkspaceGrants is GET /api/oauth.listWorkspaceGrants: the connections
// of a workspace's keys, for its owners.
func (h *OAuthHandler) listWorkspaceGrants(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	grants, err := h.cfg.Service.ListWorkspaceGrants(r.Context(), r.URL.Query().Get("workspace_id"))
	if err != nil {
		h.writeConsoleError(w, err, "Failed to list OAuth connections")
		return
	}
	if grants == nil {
		grants = []*domain.OAuthGrantListItem{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"grants": grants})
}

// revokeGrant is POST /api/oauth.revokeGrant: an owner ends one connection.
func (h *OAuthHandler) revokeGrant(w http.ResponseWriter, r *http.Request) {
	var req struct {
		GrantID string `json:"grant_id"`
	}
	if !decodeOAuthCall(w, r, &req) {
		return
	}
	if err := h.cfg.Service.RevokeGrant(r.Context(), req.GrantID, callMeta(w, r)); err != nil {
		h.writeConsoleError(w, err, "Failed to revoke an OAuth connection")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}

func (h *OAuthHandler) logError(message string, err error) {
	if h.cfg.Logger == nil {
		return
	}
	h.cfg.Logger.WithField("error", err.Error()).Error(message)
}
