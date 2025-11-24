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
