package http

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/http/middleware"
	"github.com/Notifuse/notifuse/internal/mcp"
	"github.com/Notifuse/notifuse/internal/service"
)

// MCPAuthFailRateLimitNamespace bounds, per IP, the requests to /mcp whose
// credential fails, and with them the database lookups an attacker can cause.
const MCPAuthFailRateLimitNamespace = "mcp_auth_fail"

// MCPAuthenticator resolves an API key token: a JWT of type api_key, signed
// by this installation, whose account still exists and is an API key.
type MCPAuthenticator interface {
	AuthenticateAPIKeyToken(ctx context.Context, token string) (*domain.User, error)
}

// MCPTokenResolver resolves an OAuth access token to its grant and key.
type MCPTokenResolver interface {
	Unavailable() string
	ResolveAccessToken(ctx context.Context, token string) (*domain.OAuthAccess, error)
}

// MCPRateLimiter is the process's rate limiter, as /mcp uses it.
type MCPRateLimiter interface {
	Allow(namespace, key string) bool
}

// MCPHandlerConfig configures the /mcp handler.
type MCPHandlerConfig struct {
	// Enabled is MCP_ENABLED.
	Enabled bool
	// IsDemo turns MCP off whatever Enabled says.
	IsDemo bool
	// APIEndpoint returns API_ENDPOINT, the URL clients reach this server at.
	APIEndpoint func() string
	// AllowedOrigins is MCP_ALLOWED_ORIGINS.
	AllowedOrigins []string
	Auth           MCPAuthenticator
	// OAuth resolves OAuth access tokens; nil refuses them.
	OAuth       MCPTokenResolver
	RateLimiter MCPRateLimiter
	// Server is the MCP server's Streamable HTTP handler.
	Server http.Handler
}

// MCPHandler serves /mcp. It runs, in this order: the instance switch and
// demo mode, the host check, the path, the Origin check, CORS and its
// preflight, the per-IP bucket of failed credentials, the credential, and
// only then the MCP server. Nothing reads a credential before the Origin is
// accepted, and no JSON-RPC message is read before the credential is.
type MCPHandler struct {
	cfg MCPHandlerConfig
}

// NewMCPHandler builds the /mcp handler.
func NewMCPHandler(cfg MCPHandlerConfig) *MCPHandler {
	return &MCPHandler{cfg: cfg}
}

// RegisterRoutes mounts /mcp, and /mcp/ so that a path under it answers JSON
// rather than the console redirect.
func (h *MCPHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.Handle(middleware.MCPPath, h)
	mux.Handle(middleware.MCPPath+"/", h)
}

func (h *MCPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !h.cfg.Enabled || h.cfg.IsDemo {
		WriteJSONError(w, "MCP is not enabled on this server", http.StatusNotFound)
		return
	}

	endpoint, err := domain.CanonicalAPIEndpoint(h.apiEndpoint())
	if err != nil {
		WriteJSONError(w, "MCP needs API_ENDPOINT to be set to this server's public URL", http.StatusNotFound)
		return
	}
	if !endpoint.MatchesHost(r.Host) {
		writeJSON(w, http.StatusNotFound, map[string]string{
			"error":         "MCP is served on " + endpoint.Host + " only",
			"expected_host": endpoint.Host,
		})
		return
	}
	mcpURL := endpoint.MCPURL()
	if r.URL.Path != middleware.MCPPath {
		writeJSON(w, http.StatusNotFound, map[string]string{
			"error":   "not found: the MCP endpoint is " + mcpURL,
			"mcp_url": mcpURL,
		})
		return
	}

	if origin := r.Header.Get("Origin"); origin != "" && !h.originAllowed(endpoint, origin) {
		WriteJSONError(w, "Origin not allowed", http.StatusForbidden)
		return
	}
	if middleware.ApplyMCPCORS(w, r) {
		return
	}

	// The failed-credential bucket is charged on a refusal only, and read only
	// then: a valid key is never refused because of the bucket, so a caller who
	// forges X-Forwarded-For cannot lock someone else out. Only a token signed
	// with this installation's secret reaches the database, so the bucket runs
	// after the signature check. It is keyed on middleware.ClientIP, which
	// trusts the first X-Forwarded-For entry. A caller who sends a new value on
	// each request gets a new bucket each time, so the bucket bounds guessing
	// only behind a proxy that overwrites that header.
	clientIP := middleware.ClientIP(r)
	caller, outcome := h.authenticate(r)
	switch outcome {
	case mcpCredentialRefused:
		w.Header().Set("WWW-Authenticate", h.challenge(endpoint, r))
		if !h.cfg.RateLimiter.Allow(MCPAuthFailRateLimitNamespace, clientIP) {
			WriteJSONError(w, "Too many failed authentication attempts", http.StatusTooManyRequests)
			return
		}
		WriteJSONError(w, "A valid API key or OAuth access token is required", http.StatusUnauthorized)
		return
	case mcpCredentialUnchecked:
		// Our failure, not the caller's: a 401 would make a client drop a good key.
		WriteJSONError(w, "The credential could not be checked, try again", http.StatusInternalServerError)
		return
	}

	// The key's claims, as RequireAuth sets them for an API key: never a
	// session ID or a system-call marker.
	ctx := context.WithValue(r.Context(), domain.UserIDKey, caller.APIKeyUserID)
	ctx = context.WithValue(ctx, domain.UserTypeKey, string(domain.UserTypeAPIKey))
	ctx = domain.WithMCPCaller(ctx, caller)
	ctx = mcp.WithRequestMeta(ctx, mcp.RequestMeta{
		IP:        clientIP,
		UserAgent: r.UserAgent(),
		RequestID: w.Header().Get(middleware.RequestIDHeader),
	})
	h.cfg.Server.ServeHTTP(w, r.WithContext(ctx))
}

func (h *MCPHandler) apiEndpoint() string {
	if h.cfg.APIEndpoint == nil {
		return ""
	}
	return h.cfg.APIEndpoint()
}

// originAllowed accepts the canonical origin and MCP_ALLOWED_ORIGINS, compared
// as a browser serializes an origin.
func (h *MCPHandler) originAllowed(endpoint domain.CanonicalEndpoint, origin string) bool {
	origin = strings.ToLower(origin)
	if origin == endpoint.Origin {
		return true
	}
	for _, allowed := range h.cfg.AllowedOrigins {
		if origin == strings.ToLower(strings.TrimRight(allowed, "/")) {
			return true
		}
	}
	return false
}

// challenge is the WWW-Authenticate of a refusal: Bearer, with the protected
// resource metadata that starts OAuth discovery when the authorization server
// is available (RFC 9728 §5.1), and invalid_token when a credential was sent.
func (h *MCPHandler) challenge(endpoint domain.CanonicalEndpoint, r *http.Request) string {
	params := []string{}
	if h.cfg.OAuth != nil && h.cfg.OAuth.Unavailable() == "" {
		params = append(params, `resource_metadata="`+ProtectedResourceMetadataURL(endpoint)+`"`, `scope="`+domain.OAuthScope+`"`)
	}
	if strings.TrimSpace(r.Header.Get("Authorization")) != "" {
		params = append(params, `error="invalid_token"`)
	}
	if len(params) == 0 {
		return "Bearer"
	}
	return "Bearer " + strings.Join(params, ", ")
}

type mcpCredentialOutcome int

const (
	mcpCredentialAccepted mcpCredentialOutcome = iota
	// mcpCredentialRefused: no bearer, an API key token that is not one of
	// this installation or whose account is gone, an OAuth access token that
	// does not resolve, a refresh token, or anything else.
	mcpCredentialRefused
	// mcpCredentialUnchecked: the server could not check it (secret or database).
	mcpCredentialUnchecked
)

// authenticate resolves the bearer credential into the MCP caller: an OAuth
// access token (nfmcp_at_) through its grant to an API key, or an API key
// token. A refresh token, a console session, an invitation, a revoked key and
// anything else fail. Only a token signed by this installation, or one with
// the access token prefix, reaches the database.
func (h *MCPHandler) authenticate(r *http.Request) (*domain.MCPCaller, mcpCredentialOutcome) {
	scheme, token, found := strings.Cut(r.Header.Get("Authorization"), " ")
	token = strings.TrimSpace(token)
	if !found || !strings.EqualFold(scheme, "Bearer") || token == "" {
		return nil, mcpCredentialRefused
	}

	switch {
	case strings.HasPrefix(token, domain.OAuthAccessTokenPrefix):
		if h.cfg.OAuth == nil {
			return nil, mcpCredentialRefused
		}
		access, err := h.cfg.OAuth.ResolveAccessToken(r.Context(), token)
		switch {
		case errors.Is(err, domain.ErrOAuthInvalidToken):
			return nil, mcpCredentialRefused
		case err != nil:
			return nil, mcpCredentialUnchecked
		case access == nil || access.Grant == nil || access.Key == nil || access.Key.Type != domain.UserTypeAPIKey:
			return nil, mcpCredentialRefused
		}
		return &domain.MCPCaller{
			Kind:         domain.MCPCredentialOAuth,
			APIKeyUserID: access.Key.ID,
			GrantID:      access.Grant.ID,
			ClientName:   access.Grant.ClientName,
		}, mcpCredentialAccepted
	case strings.HasPrefix(token, domain.OAuthRefreshTokenPrefix):
		// A refresh token is for the token endpoint only.
		return nil, mcpCredentialRefused
	}

	user, err := h.cfg.Auth.AuthenticateAPIKeyToken(r.Context(), token)
	switch {
	case errors.Is(err, domain.ErrNotAnAPIKey), errors.Is(err, service.ErrUserNotFound), errors.Is(err, domain.ErrAPIKeyRevoked):
		return nil, mcpCredentialRefused
	case err != nil:
		return nil, mcpCredentialUnchecked
	case user == nil || user.Type != domain.UserTypeAPIKey:
		return nil, mcpCredentialRefused
	}
	return &domain.MCPCaller{Kind: domain.MCPCredentialHeader, APIKeyUserID: user.ID}, mcpCredentialAccepted
}
