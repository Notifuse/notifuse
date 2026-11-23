package http

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/golang-jwt/jwt/v5"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/http/middleware"
	"github.com/Notifuse/notifuse/internal/mcp"
	"github.com/Notifuse/notifuse/internal/service"
)

// MCPAuthFailRateLimitNamespace bounds, per IP, the requests to /mcp whose
// credential fails, and with them the database lookups an attacker can cause.
const MCPAuthFailRateLimitNamespace = "mcp_auth_fail"

// MCPAuthenticator loads the account an API key token names.
type MCPAuthenticator interface {
	AuthenticateUserFromContext(ctx context.Context) (*domain.User, error)
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
	GetJWTSecret   func() ([]byte, error)
	Auth           MCPAuthenticator
	RateLimiter    MCPRateLimiter
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

// canonicalEndpoint is API_ENDPOINT reduced to what clients compare: a
// lowercase scheme and host, the port only when it is not the scheme's
// default, and no trailing slash.
type canonicalEndpoint struct {
	// url is the canonical URL, path included.
	url string
	// origin is scheme://host[:port].
	origin string
	// host is host[:port], as a Host header carries it.
	host string
	// hasPort reports whether host carries a port.
	hasPort bool
}

// canonicalAPIEndpoint derives the canonical endpoint from API_ENDPOINT.
func canonicalAPIEndpoint(apiEndpoint string) (canonicalEndpoint, error) {
	parsed, err := url.Parse(strings.TrimSpace(apiEndpoint))
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return canonicalEndpoint{}, fmt.Errorf("API_ENDPOINT is not an http(s) URL")
	}
	scheme := strings.ToLower(parsed.Scheme)
	hostname := strings.ToLower(parsed.Hostname())
	port := parsed.Port()
	if (scheme == "http" && port == "80") || (scheme == "https" && port == "443") {
		port = ""
	}
	host := hostname
	if strings.Contains(hostname, ":") {
		host = "[" + hostname + "]"
	}
	if port != "" {
		host = net.JoinHostPort(hostname, port)
	}
	origin := scheme + "://" + host
	return canonicalEndpoint{
		url:     origin + strings.TrimRight(parsed.EscapedPath(), "/"),
		origin:  origin,
		host:    host,
		hasPort: port != "",
	}, nil
}

// matchesHost compares a request's Host header with the canonical host. The
// port is compared when the canonical URL has one.
func (c canonicalEndpoint) matchesHost(requestHost string) bool {
	requestHost = strings.ToLower(requestHost)
	if c.hasPort {
		return requestHost == c.host
	}
	if hostname, _, err := net.SplitHostPort(requestHost); err == nil {
		if strings.Contains(hostname, ":") {
			hostname = "[" + hostname + "]"
		}
		requestHost = hostname
	}
	return requestHost == c.host
}

func (h *MCPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !h.cfg.Enabled || h.cfg.IsDemo {
		WriteJSONError(w, "MCP is not enabled on this server", http.StatusNotFound)
		return
	}

	endpoint, err := canonicalAPIEndpoint(h.apiEndpoint())
	if err != nil {
		WriteJSONError(w, "MCP needs API_ENDPOINT to be set to this server's public URL", http.StatusNotFound)
		return
	}
	if !endpoint.matchesHost(r.Host) {
		writeJSON(w, http.StatusNotFound, map[string]string{
			"error":         "MCP is served on " + endpoint.host + " only",
			"expected_host": endpoint.host,
		})
		return
	}
	mcpURL := endpoint.url + middleware.MCPPath
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
	ctx, user, outcome := h.authenticate(r)
	switch outcome {
	case mcpCredentialRefused:
		w.Header().Set("WWW-Authenticate", "Bearer")
		if !h.cfg.RateLimiter.Allow(MCPAuthFailRateLimitNamespace, clientIP) {
			WriteJSONError(w, "Too many failed authentication attempts", http.StatusTooManyRequests)
			return
		}
		WriteJSONError(w, "A valid API key is required", http.StatusUnauthorized)
		return
	case mcpCredentialUnchecked:
		// Our failure, not the caller's: a 401 would make a client drop a good key.
		WriteJSONError(w, "The credential could not be checked, try again", http.StatusInternalServerError)
		return
	}

	ctx = domain.WithMCPCaller(ctx, &domain.MCPCaller{Kind: domain.MCPCredentialHeader, APIKeyUserID: user.ID})
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
func (h *MCPHandler) originAllowed(endpoint canonicalEndpoint, origin string) bool {
	origin = strings.ToLower(origin)
	if origin == endpoint.origin {
		return true
	}
	for _, allowed := range h.cfg.AllowedOrigins {
		if origin == strings.ToLower(strings.TrimRight(allowed, "/")) {
			return true
		}
	}
	return false
}

// authenticate resolves the bearer credential: a JWT of type api_key, signed
// by this installation, whose account still exists and is an API key. A
// console session, an invitation, a revoked key and anything else fail. It
// sets the user ID and type as RequireAuth does, never a session ID or a
// system-call marker.
type mcpCredentialOutcome int

const (
	mcpCredentialAccepted mcpCredentialOutcome = iota
	// mcpCredentialRefused: no bearer, not a JWT of this installation, not an
	// API key token, or a key whose account is gone.
	mcpCredentialRefused
	// mcpCredentialUnchecked: the server could not check it (secret or database).
	mcpCredentialUnchecked
)

func (h *MCPHandler) authenticate(r *http.Request) (context.Context, *domain.User, mcpCredentialOutcome) {
	scheme, token, found := strings.Cut(r.Header.Get("Authorization"), " ")
	if !found || !strings.EqualFold(scheme, "Bearer") || strings.TrimSpace(token) == "" {
		return nil, nil, mcpCredentialRefused
	}
	secret, err := h.cfg.GetJWTSecret()
	if err != nil {
		return nil, nil, mcpCredentialUnchecked
	}
	claims := &service.UserClaims{}
	parsed, err := jwt.ParseWithClaims(strings.TrimSpace(token), claims, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return secret, nil
	})
	if err != nil || !parsed.Valid || claims.UserID == "" || claims.Type != string(domain.UserTypeAPIKey) {
		return nil, nil, mcpCredentialRefused
	}

	ctx := context.WithValue(r.Context(), domain.UserIDKey, claims.UserID)
	ctx = context.WithValue(ctx, domain.UserTypeKey, claims.Type)
	user, err := h.cfg.Auth.AuthenticateUserFromContext(ctx)
	switch {
	case errors.Is(err, service.ErrUserNotFound), errors.Is(err, domain.ErrAPIKeyRevoked):
		return nil, nil, mcpCredentialRefused
	case err != nil:
		return nil, nil, mcpCredentialUnchecked
	case user == nil || user.Type != domain.UserTypeAPIKey:
		return nil, nil, mcpCredentialRefused
	}
	return ctx, user, mcpCredentialAccepted
}
