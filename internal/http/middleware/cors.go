package middleware

import (
	"net/http"
	"os"
	"strings"

	"github.com/Notifuse/notifuse/internal/domain"
)

// CORSMiddleware handles CORS headers for all requests but the MCP endpoint
// and the OAuth authorization server, which answer their own (ApplyMCPCORS,
// ApplyOAuthCORS): CORS_ALLOW_ORIGIN and credentials do not apply there.
func CORSMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if IsMCPPath(r.URL.Path) || IsOAuthPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		// Get allowed origin from environment variable with default value "*"
		allowOrigin, varies := resolveAllowOrigin(os.Getenv("CORS_ALLOW_ORIGIN"), r.Header.Get("Origin"))
		if varies {
			w.Header().Add("Vary", "Origin")
		}
		if allowOrigin != "" {
			w.Header().Set("Access-Control-Allow-Origin", allowOrigin)
		}

		// Allow specific HTTP methods
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")

		// Allow specific headers
		w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Content-Length, Accept-Encoding, Authorization")

		// Allow credentials
		w.Header().Set("Access-Control-Allow-Credentials", "true")

		// Handle preflight requests
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// resolveAllowOrigin picks the Access-Control-Allow-Origin value for one request.
//
// Browsers accept only "*" or the single exact origin of the page making the request, so a
// comma-separated CORS_ALLOW_ORIGIN cannot be sent as written: the browser compares the whole
// string to its origin and refuses every response. Each workspace custom domain serves the
// console too, and that console calls API_ENDPOINT from its own origin, so an operator
// restricting CORS has to list several origins. A list answers the request's Origin when it is
// listed and nothing otherwise; varies reports that the answer depends on Origin, so a shared
// cache does not hand one origin's answer to another.
//
// Unset or blank allows every origin, as does a "*" entry. A single origin is sent whatever the
// request's Origin, as it always was. A value naming no origin at all, such as ",", allows none:
// it used to reach browsers as written, which refused it, and must not turn into "*".
func resolveAllowOrigin(configured, requestOrigin string) (allowOrigin string, varies bool) {
	if strings.TrimSpace(configured) == "" {
		return "*", false
	}

	var origins []string
	for origin := range strings.SplitSeq(configured, ",") {
		// A browser serializes an origin as a lowercase scheme://host[:port] with no trailing
		// slash; normalizing the configured side the same way lets an exact comparison match.
		origin = strings.ToLower(strings.TrimRight(strings.TrimSpace(origin), "/"))
		if origin == "*" {
			return "*", false
		}
		if origin != "" {
			origins = append(origins, origin)
		}
	}

	switch len(origins) {
	case 0:
		return "", false
	case 1:
		return origins[0], false
	}

	// Exact comparison, and the configured value is what goes back: the request's Origin is
	// never reflected, so no look-alike of a listed origin is ever answered with itself.
	for _, origin := range origins {
		if origin == requestOrigin {
			return origin, true
		}
	}
	return "", true
}

// MCPPath is the path of the MCP endpoint.
const MCPPath = domain.MCPPath

// mcpAllowedHeaders are the request headers an MCP client may send from a
// browser: the API's own plus the MCP transport's.
const mcpAllowedHeaders = "Accept, Content-Type, Content-Length, Accept-Encoding, Authorization, Mcp-Protocol-Version, Mcp-Method, Mcp-Name, Mcp-Session-Id"

// IsMCPPath reports whether a path belongs to the MCP endpoint, which handles
// CORS itself.
func IsMCPPath(path string) bool {
	return path == MCPPath || strings.HasPrefix(path, MCPPath+"/")
}

// ApplyMCPCORS writes the CORS headers of an MCP response for a request whose
// Origin the caller has already accepted. A request without Origin gets none.
// The origin is reflected, never "*", and credentials are never allowed: an MCP
// client sends its key in Authorization, not in a cookie. It reports whether
// the request was a preflight, which it answers.
func ApplyMCPCORS(w http.ResponseWriter, r *http.Request) (preflight bool) {
	origin := r.Header.Get("Origin")
	if origin != "" {
		w.Header().Add("Vary", "Origin")
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", mcpAllowedHeaders)
		w.Header().Set("Access-Control-Expose-Headers", "WWW-Authenticate")
	}
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return true
	}
	return false
}

// The paths of the OAuth authorization server of /mcp, and of the discovery
// documents clients read before it.
const (
	OAuthPathPrefix              = "/oauth/"
	OAuthAuthorizePath           = "/oauth/authorize"
	OAuthTokenPath               = "/oauth/token"
	OAuthRevokePath              = "/oauth/revoke"
	OAuthProtectedResourcePath   = "/.well-known/oauth-protected-resource"
	OAuthAuthorizationServerPath = "/.well-known/oauth-authorization-server"
	OpenIDConfigurationPath      = "/.well-known/openid-configuration"
)

// IsOAuthPath reports whether a path belongs to the OAuth authorization
// server or its discovery documents, which handle CORS themselves.
func IsOAuthPath(path string) bool {
	if path == "/oauth" || strings.HasPrefix(path, OAuthPathPrefix) {
		return true
	}
	for _, document := range []string{OAuthProtectedResourcePath, OAuthAuthorizationServerPath, OpenIDConfigurationPath} {
		if path == document || strings.HasPrefix(path, document+"/") {
			return true
		}
	}
	return false
}

// ApplyOAuthCORS writes the CORS headers of the token and revocation
// endpoints and the discovery documents: any origin may call them, since a
// browser-based client holds no cookie of this server and sends none.
// Credentials are never allowed. It reports whether the request was a
// preflight, which it answers. GET /oauth/authorize is a navigation and never
// calls it.
func ApplyOAuthCORS(w http.ResponseWriter, r *http.Request) (preflight bool) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", mcpAllowedHeaders)
	w.Header().Set("Access-Control-Expose-Headers", "WWW-Authenticate")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return true
	}
	return false
}
