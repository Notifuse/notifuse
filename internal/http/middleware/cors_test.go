package middleware

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCORSMiddleware(t *testing.T) {
	// Common test handler
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Create the middleware handler
	handler := CORSMiddleware(next)

	t.Run("with default origin", func(t *testing.T) {
		// Save current environment variable and restore it after the test
		originalOrigin := os.Getenv("CORS_ALLOW_ORIGIN")
		defer func() { _ = os.Setenv("CORS_ALLOW_ORIGIN", originalOrigin) }()

		// Clear environment variable to test default behavior
		_ = os.Unsetenv("CORS_ALLOW_ORIGIN")

		// Create a test request
		req := httptest.NewRequest("GET", "/api/test", nil)
		w := httptest.NewRecorder()

		// Call the handler
		handler.ServeHTTP(w, req)

		// Assert response status
		assert.Equal(t, http.StatusOK, w.Code)

		// Assert CORS headers
		assert.Equal(t, "*", w.Header().Get("Access-Control-Allow-Origin"))
		assert.Equal(t, "GET, POST, PUT, DELETE, OPTIONS", w.Header().Get("Access-Control-Allow-Methods"))
		assert.Equal(t, "Accept, Content-Type, Content-Length, Accept-Encoding, Authorization", w.Header().Get("Access-Control-Allow-Headers"))
		assert.Equal(t, "true", w.Header().Get("Access-Control-Allow-Credentials"))
	})

	t.Run("with custom origin", func(t *testing.T) {
		// Save current environment variable and restore it after the test
		originalOrigin := os.Getenv("CORS_ALLOW_ORIGIN")
		defer func() { _ = os.Setenv("CORS_ALLOW_ORIGIN", originalOrigin) }()

		// Set custom origin
		_ = os.Setenv("CORS_ALLOW_ORIGIN", "https://example.com")

		// Create a test request
		req := httptest.NewRequest("GET", "/api/test", nil)
		w := httptest.NewRecorder()

		// Call the handler
		handler.ServeHTTP(w, req)

		// Assert response status
		assert.Equal(t, http.StatusOK, w.Code)

		// Assert CORS headers
		assert.Equal(t, "https://example.com", w.Header().Get("Access-Control-Allow-Origin"))
		assert.Equal(t, "GET, POST, PUT, DELETE, OPTIONS", w.Header().Get("Access-Control-Allow-Methods"))
		assert.Equal(t, "Accept, Content-Type, Content-Length, Accept-Encoding, Authorization", w.Header().Get("Access-Control-Allow-Headers"))
		assert.Equal(t, "true", w.Header().Get("Access-Control-Allow-Credentials"))
	})

	t.Run("with OPTIONS request", func(t *testing.T) {
		// Create a test OPTIONS request
		req := httptest.NewRequest("OPTIONS", "/api/test", nil)
		w := httptest.NewRecorder()

		// Call the handler
		handler.ServeHTTP(w, req)

		// Assert response status for preflight request
		assert.Equal(t, http.StatusOK, w.Code)

		// Assert CORS headers
		assert.Equal(t, "GET, POST, PUT, DELETE, OPTIONS", w.Header().Get("Access-Control-Allow-Methods"))
		assert.Equal(t, "Accept, Content-Type, Content-Length, Accept-Encoding, Authorization", w.Header().Get("Access-Control-Allow-Headers"))
		assert.Equal(t, "true", w.Header().Get("Access-Control-Allow-Credentials"))
	})
}

// A console opened on a workspace custom domain calls API_ENDPOINT from that domain's origin, so
// an operator who restricts CORS lists the main origin and every custom domain.
func TestCORSMiddleware_OriginList(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := CORSMiddleware(next)

	serve := func(method, origin string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, "/api/user.signin", nil)
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		return w
	}

	t.Run("answers a listed origin with that origin alone", func(t *testing.T) {
		t.Setenv("CORS_ALLOW_ORIGIN", "https://notifuse.example.com,https://news.customer.com")

		w := serve("POST", "https://news.customer.com")

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "https://news.customer.com", w.Header().Get("Access-Control-Allow-Origin"))
		assert.Equal(t, "Origin", w.Header().Get("Vary"))
	})

	t.Run("answers the preflight of a listed origin", func(t *testing.T) {
		t.Setenv("CORS_ALLOW_ORIGIN", "https://notifuse.example.com,https://news.customer.com")

		w := serve("OPTIONS", "https://notifuse.example.com")

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "https://notifuse.example.com", w.Header().Get("Access-Control-Allow-Origin"))
		assert.Equal(t, "Accept, Content-Type, Content-Length, Accept-Encoding, Authorization", w.Header().Get("Access-Control-Allow-Headers"))
	})

	t.Run("omits the header for an origin not listed", func(t *testing.T) {
		t.Setenv("CORS_ALLOW_ORIGIN", "https://notifuse.example.com,https://news.customer.com")

		w := serve("POST", "https://evil.example.net")

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Empty(t, w.Header().Values("Access-Control-Allow-Origin"))
		assert.Equal(t, "Origin", w.Header().Get("Vary"))
	})

	t.Run("omits the header when the request carries no Origin", func(t *testing.T) {
		t.Setenv("CORS_ALLOW_ORIGIN", "https://notifuse.example.com,https://news.customer.com")

		w := serve("GET", "")

		assert.Empty(t, w.Header().Values("Access-Control-Allow-Origin"))
	})

	t.Run("tolerates spaces, a trailing slash and letter case in the list", func(t *testing.T) {
		t.Setenv("CORS_ALLOW_ORIGIN", " https://notifuse.example.com , HTTPS://News.Customer.com/ ")

		w := serve("POST", "https://news.customer.com")

		assert.Equal(t, "https://news.customer.com", w.Header().Get("Access-Control-Allow-Origin"))
	})

	t.Run("a wildcard anywhere in the list allows every origin", func(t *testing.T) {
		t.Setenv("CORS_ALLOW_ORIGIN", "https://notifuse.example.com,*")

		w := serve("POST", "https://news.customer.com")

		assert.Equal(t, "*", w.Header().Get("Access-Control-Allow-Origin"))
		assert.Empty(t, w.Header().Get("Vary"))
	})

	t.Run("never answers a look-alike of a listed origin", func(t *testing.T) {
		t.Setenv("CORS_ALLOW_ORIGIN", "https://notifuse.example.com,https://news.customer.com")

		for _, origin := range []string{
			"https://news.customer.com.evil.net", // listed origin as a prefix
			"https://evil.news.customer.com",     // listed origin as a suffix
			"https://news.customer.co",           // listed origin truncated
			"http://news.customer.com",           // other scheme
			"https://news.customer.com:8443",     // other port
			"https://news.customer.com/",         // not how a browser writes an origin
			"HTTPS://NEWS.CUSTOMER.COM",          // nor this: browsers send lowercase
			"null",                               // sandboxed frames, file:// pages
		} {
			w := serve("POST", origin)

			assert.Empty(t, w.Header().Values("Access-Control-Allow-Origin"), origin)
			assert.Equal(t, "Origin", w.Header().Get("Vary"), origin)
		}
	})

	t.Run("a value that names no origin allows none", func(t *testing.T) {
		for _, configured := range []string{",", " , ", "/"} {
			t.Setenv("CORS_ALLOW_ORIGIN", configured)

			w := serve("POST", "https://news.customer.com")

			assert.Empty(t, w.Header().Values("Access-Control-Allow-Origin"), configured)
		}
	})

	t.Run("a blank value allows every origin, like an unset one", func(t *testing.T) {
		t.Setenv("CORS_ALLOW_ORIGIN", "   ")

		w := serve("POST", "https://news.customer.com")

		assert.Equal(t, "*", w.Header().Get("Access-Control-Allow-Origin"))
	})

	t.Run("a single origin is sent whatever the request Origin", func(t *testing.T) {
		t.Setenv("CORS_ALLOW_ORIGIN", "https://notifuse.example.com/")

		w := serve("POST", "https://news.customer.com")

		assert.Equal(t, "https://notifuse.example.com", w.Header().Get("Access-Control-Allow-Origin"))
		assert.Empty(t, w.Header().Get("Vary"))
	})
}

// The global CORS policy leaves the MCP endpoint and the OAuth authorization
// server alone, whatever
// CORS_ALLOW_ORIGIN says: no header written and a preflight passed through to
// the MCP handler, which answers it. Every other path keeps the global policy.
func TestCORSMiddleware_LeavesTheMCPEndpointToItsOwnPolicy(t *testing.T) {
	t.Setenv("CORS_ALLOW_ORIGIN", "https://console.example.com")

	reached := 0
	handler := CORSMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached++
		w.WriteHeader(http.StatusTeapot)
	}))

	for _, path := range []string{"/mcp", "/mcp/", "/mcp/anything",
		"/oauth", "/oauth/authorize", "/oauth/token", "/oauth/revoke", "/oauth/register",
		"/.well-known/oauth-protected-resource", "/.well-known/oauth-protected-resource/mcp",
		"/.well-known/oauth-authorization-server", "/.well-known/oauth-authorization-server/mcp",
		"/.well-known/openid-configuration", "/.well-known/openid-configuration/mcp"} {
		for _, method := range []string{http.MethodPost, http.MethodOptions} {
			reached = 0
			req := httptest.NewRequest(method, path, nil)
			req.Header.Set("Origin", "https://console.example.com")
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)

			assert.Equal(t, 1, reached, "%s %s reaches the MCP handler", method, path)
			assert.Equal(t, http.StatusTeapot, w.Code)
			for _, header := range []string{"Access-Control-Allow-Origin", "Access-Control-Allow-Credentials", "Access-Control-Allow-Headers", "Access-Control-Allow-Methods"} {
				assert.Empty(t, w.Header().Get(header), "%s %s must not carry %s", method, path, header)
			}
		}
	}

	for _, path := range []string{"/api/contacts.list", "/mcpx", "/console/mcp", "/api/oauth.approveWithKey", "/oauthx", "/.well-known/security.txt", "/console/oauth/authorize"} {
		reached = 0
		req := httptest.NewRequest(http.MethodOptions, path, nil)
		req.Header.Set("Origin", "https://console.example.com")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		assert.Equal(t, 0, reached, "%s preflight is answered by the global policy", path)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "https://console.example.com", w.Header().Get("Access-Control-Allow-Origin"))
		assert.Equal(t, "true", w.Header().Get("Access-Control-Allow-Credentials"))
		assert.Equal(t, "Accept, Content-Type, Content-Length, Accept-Encoding, Authorization", w.Header().Get("Access-Control-Allow-Headers"))
	}
}

func TestApplyMCPCORS(t *testing.T) {
	t.Run("an accepted origin is reflected, without credentials", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
		req.Header.Set("Origin", "http://localhost:6274")
		w := httptest.NewRecorder()

		assert.False(t, ApplyMCPCORS(w, req))
		assert.Equal(t, "http://localhost:6274", w.Header().Get("Access-Control-Allow-Origin"))
		assert.Equal(t, "Origin", w.Header().Get("Vary"))
		assert.Empty(t, w.Header().Get("Access-Control-Allow-Credentials"))
		assert.Equal(t, "Accept, Content-Type, Content-Length, Accept-Encoding, Authorization, Mcp-Protocol-Version, Mcp-Method, Mcp-Name, Mcp-Session-Id",
			w.Header().Get("Access-Control-Allow-Headers"))
		assert.Equal(t, "WWW-Authenticate", w.Header().Get("Access-Control-Expose-Headers"))
	})

	t.Run("a preflight is answered", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodOptions, "/mcp", nil)
		req.Header.Set("Origin", "http://localhost:6274")
		w := httptest.NewRecorder()

		assert.True(t, ApplyMCPCORS(w, req))
		assert.Equal(t, http.StatusNoContent, w.Code)
		assert.Equal(t, "http://localhost:6274", w.Header().Get("Access-Control-Allow-Origin"))
	})

	t.Run("no Origin, no CORS header", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
		w := httptest.NewRecorder()

		assert.False(t, ApplyMCPCORS(w, req))
		assert.Empty(t, w.Header().Get("Access-Control-Allow-Origin"))
		assert.Empty(t, w.Header().Get("Access-Control-Allow-Headers"))
	})
}

func TestApplyOAuthCORS(t *testing.T) {
	t.Run("any origin, never credentials", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/oauth/token", nil)
		req.Header.Set("Origin", "https://claude.ai")
		w := httptest.NewRecorder()

		assert.False(t, ApplyOAuthCORS(w, req))
		assert.Equal(t, "*", w.Header().Get("Access-Control-Allow-Origin"))
		assert.Empty(t, w.Header().Get("Access-Control-Allow-Credentials"))
		assert.Equal(t, "GET, POST, OPTIONS", w.Header().Get("Access-Control-Allow-Methods"))
		assert.Equal(t, "WWW-Authenticate", w.Header().Get("Access-Control-Expose-Headers"))
	})

	t.Run("a preflight is answered", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodOptions, "/oauth/token", nil)
		w := httptest.NewRecorder()

		assert.True(t, ApplyOAuthCORS(w, req))
		assert.Equal(t, http.StatusNoContent, w.Code)
	})
}
