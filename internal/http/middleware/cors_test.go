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
