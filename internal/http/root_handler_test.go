package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRootHandler(t *testing.T) {
	handler := NewRootHandler()
	assert.NotNil(t, handler, "Root handler should not be nil")
}

func TestNewRootHandlerWithConsoleAndNotificationCenter(t *testing.T) {
	// Create a test logger
	testLogger := logger.NewLogger()

	// Create handler with both console and notification center
	handler := NewRootHandlerWithConsoleAndNotificationCenter("test_console_dir", "test_notification_center_dir", testLogger, "https://api.example.com")

	// Assert fields are set correctly
	assert.Equal(t, "test_console_dir", handler.consoleDir)
	assert.Equal(t, "test_notification_center_dir", handler.notificationCenterDir)
}

func TestRootHandler_Handle(t *testing.T) {
	handler := NewRootHandler()

	// Create a test request
	req := httptest.NewRequest("GET", "/api", nil)
	w := httptest.NewRecorder()

	// Call the handler
	handler.Handle(w, req)

	// Assert response
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	// Decode response body
	var response map[string]string
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)

	// Assert response content
	assert.Equal(t, "api running", response["status"])
}

func TestRootHandler_RegisterRoutes(t *testing.T) {
	handler := NewRootHandler()
	mux := http.NewServeMux()

	// Register routes
	handler.RegisterRoutes(mux)

	// Create a test server
	server := httptest.NewServer(mux)
	defer server.Close()

	// Send a request
	resp, err := http.Get(server.URL + "/api")
	require.NoError(t, err)
	defer resp.Body.Close()

	// Assert response
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Decode response body
	var response map[string]string
	err = json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err)

	// Assert response content
	assert.Equal(t, "api running", response["status"])
}

func TestRootHandler_RegisterRoutesWithNotificationCenter(t *testing.T) {
	// Create a test logger
	testLogger := logger.NewLogger()

	// Create handler with both console and notification center
	handler := NewRootHandlerWithConsoleAndNotificationCenter("test_console_dir", "test_notification_center_dir", testLogger, "https://api.example.com")

	mux := http.NewServeMux()

	// Register routes
	handler.RegisterRoutes(mux)

	// Test that routes were registered (we can't directly check the mux routes)
	// but we can check that the handler handles the routes correctly
	req := httptest.NewRequest("GET", "/notification-center/", nil)
	rr := httptest.NewRecorder()

	mux.ServeHTTP(rr, req)

	// We expect a 404 because the directory doesn't exist in the test environment
	// but this confirms the route is registered
	assert.Equal(t, http.StatusNotFound, rr.Code)
}

func TestRootHandler_ServeConfigJS(t *testing.T) {
	// Create a test logger
	testLogger := logger.NewLogger()

	// Create a handler with a test API endpoint
	testAPIEndpoint := "https://api.example.com"
	handler := NewRootHandlerWithConsole("test_console_dir", testLogger, testAPIEndpoint)

	// Create a request to /config.js
	req := httptest.NewRequest("GET", "/config.js", nil)
	rr := httptest.NewRecorder()

	// Call the handler directly
	handler.serveConfigJS(rr, req)

	// Check the status code
	assert.Equal(t, http.StatusOK, rr.Code)

	// Check the content type
	assert.Equal(t, "application/javascript", rr.Header().Get("Content-Type"))

	// Check cache control headers
	assert.Equal(t, "no-cache, no-store, must-revalidate", rr.Header().Get("Cache-Control"))
	assert.Equal(t, "no-cache", rr.Header().Get("Pragma"))
	assert.Equal(t, "0", rr.Header().Get("Expires"))

	// Check the body contains the expected JavaScript
	expectedJS := `window.API_ENDPOINT = "https://api.example.com";`
	assert.Equal(t, expectedJS, rr.Body.String())
}

func TestRootHandler_Handle_ConfigJS(t *testing.T) {
	// Create a test logger
	testLogger := logger.NewLogger()

	// Create a handler with a test API endpoint
	testAPIEndpoint := "https://api.example.com"
	handler := NewRootHandlerWithConsole("test_console_dir", testLogger, testAPIEndpoint)

	// Create a request to /config.js
	req := httptest.NewRequest("GET", "/config.js", nil)
	rr := httptest.NewRecorder()

	// Call the general handle method
	handler.Handle(rr, req)

	// Check the status code
	assert.Equal(t, http.StatusOK, rr.Code)

	// Check the content type
	assert.Equal(t, "application/javascript", rr.Header().Get("Content-Type"))

	// Check the body contains the expected JavaScript
	expectedJS := `window.API_ENDPOINT = "https://api.example.com";`
	assert.Equal(t, expectedJS, rr.Body.String())
}

func TestRootHandler_ServeNotificationCenter(t *testing.T) {
	// Create a temporary directory for test notification center files
	tempDir, err := os.MkdirTemp("", "notification_center_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create a test index.html file
	indexContent := "<html><body>Notification Center Test</body></html>"
	err = os.WriteFile(filepath.Join(tempDir, "index.html"), []byte(indexContent), 0644)
	require.NoError(t, err)

	// Create a test logger
	testLogger := logger.NewLogger()

	// Create handler with notification center directory
	handler := NewRootHandlerWithConsoleAndNotificationCenter("", tempDir, testLogger, "https://api.example.com")

	t.Run("ServeExactPath", func(t *testing.T) {
		// Create a request to /notification-center/
		req := httptest.NewRequest("GET", "/notification-center/", nil)
		rr := httptest.NewRecorder()

		// Call the handler
		handler.Handle(rr, req)

		// Check status code and content
		assert.Equal(t, http.StatusOK, rr.Code)
		assert.Contains(t, rr.Body.String(), "Notification Center Test")
	})

	t.Run("ServeSPAFallback", func(t *testing.T) {
		// Create a request to a non-existent path
		req := httptest.NewRequest("GET", "/notification-center/non-existent-path", nil)
		rr := httptest.NewRecorder()

		// Call the handler
		handler.Handle(rr, req)

		// Check it falls back to index.html for SPA routing
		assert.Equal(t, http.StatusOK, rr.Code)
		assert.Contains(t, rr.Body.String(), "Notification Center Test")
	})

	t.Run("MethodNotAllowed", func(t *testing.T) {
		// Create a POST request which should not be allowed
		req := httptest.NewRequest("POST", "/notification-center/", nil)
		rr := httptest.NewRecorder()

		// Call the handler
		handler.Handle(rr, req)

		// Check method not allowed
		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
	})
}

func TestRootHandler_ServeConsole(t *testing.T) {
	// Create a temporary directory for test console files
	tempDir, err := os.MkdirTemp("", "console_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create a test index.html file
	indexContent := "<html><body>Console Test</body></html>"
	err = os.WriteFile(filepath.Join(tempDir, "index.html"), []byte(indexContent), 0644)
	require.NoError(t, err)

	// Create a test CSS file
	cssContent := "body { background-color: #fff; }"
	err = os.WriteFile(filepath.Join(tempDir, "style.css"), []byte(cssContent), 0644)
	require.NoError(t, err)

	// Create a test logger
	testLogger := logger.NewLogger()

	// Create handler with console directory
	handler := NewRootHandlerWithConsole(tempDir, testLogger, "https://api.example.com")

	t.Run("ServeExactPath", func(t *testing.T) {
		// Create a request to root (which should serve index.html)
		req := httptest.NewRequest("GET", "/", nil)
		rr := httptest.NewRecorder()

		// Call the handler
		handler.Handle(rr, req)

		// Check status code and content
		assert.Equal(t, http.StatusOK, rr.Code)
		assert.Contains(t, rr.Body.String(), "Console Test")
	})

	t.Run("ServeStaticFile", func(t *testing.T) {
		// Create a request to a static file
		req := httptest.NewRequest("GET", "/style.css", nil)
		rr := httptest.NewRecorder()

		// Call the handler
		handler.Handle(rr, req)

		// Check status code and content
		assert.Equal(t, http.StatusOK, rr.Code)
		assert.Contains(t, rr.Body.String(), "body { background-color: #fff; }")
	})

	t.Run("ServeSPAFallback", func(t *testing.T) {
		// Create a request to a non-existent path
		req := httptest.NewRequest("GET", "/non-existent-path", nil)
		rr := httptest.NewRecorder()

		// Call the handler
		handler.Handle(rr, req)

		// Check it falls back to index.html for SPA routing
		assert.Equal(t, http.StatusOK, rr.Code)
		assert.Contains(t, rr.Body.String(), "Console Test")
	})

	t.Run("MethodNotAllowed", func(t *testing.T) {
		// Create a POST request which should not be allowed
		req := httptest.NewRequest("POST", "/", nil)
		rr := httptest.NewRecorder()

		// Call the serveConsole method directly
		handler.serveConsole(rr, req)

		// Check method not allowed
		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
	})
}

func TestRootHandler_Handle_Comprehensive(t *testing.T) {
	// Create temporary directories
	consoleDir, err := os.MkdirTemp("", "console_test")
	require.NoError(t, err)
	defer os.RemoveAll(consoleDir)

	notificationCenterDir, err := os.MkdirTemp("", "nc_test")
	require.NoError(t, err)
	defer os.RemoveAll(notificationCenterDir)

	// Create test index files
	consoleIndexContent := "<html><body>Console Test</body></html>"
	err = os.WriteFile(filepath.Join(consoleDir, "index.html"), []byte(consoleIndexContent), 0644)
	require.NoError(t, err)

	ncIndexContent := "<html><body>Notification Center Test</body></html>"
	err = os.WriteFile(filepath.Join(notificationCenterDir, "index.html"), []byte(ncIndexContent), 0644)
	require.NoError(t, err)

	// Create a test logger
	testLogger := logger.NewLogger()

	// Create handler with both console and notification center
	handler := NewRootHandlerWithConsoleAndNotificationCenter(
		consoleDir,
		notificationCenterDir,
		testLogger,
		"https://api.example.com",
	)

	t.Run("NotFoundAPIPath", func(t *testing.T) {
		// Create a request to an non-existent API endpoint
		req := httptest.NewRequest("GET", "/api/invalid-endpoint", nil)
		rr := httptest.NewRecorder()

		// Call the handler
		handler.Handle(rr, req)

		// Since it's not a valid API endpoint and starts with /api,
		// it should return 404
		assert.Equal(t, http.StatusNotFound, rr.Code)
	})

	t.Run("RootAPIPath", func(t *testing.T) {
		// Create a request to /api/
		req := httptest.NewRequest("GET", "/api/", nil)
		rr := httptest.NewRecorder()

		// Call the handler
		handler.Handle(rr, req)

		// Check the response is the API status response
		assert.Equal(t, http.StatusOK, rr.Code)

		// Check content type and body
		assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))
		var response map[string]string
		err := json.NewDecoder(rr.Body).Decode(&response)
		require.NoError(t, err)
		assert.Equal(t, "api running", response["status"])
	})

	t.Run("ConfigJSPath", func(t *testing.T) {
		// Create a request to /config.js
		req := httptest.NewRequest("GET", "/config.js", nil)
		rr := httptest.NewRecorder()

		// Call the handler
		handler.Handle(rr, req)

		// Check status and content
		assert.Equal(t, http.StatusOK, rr.Code)
		assert.Contains(t, rr.Body.String(), "window.API_ENDPOINT")
	})

	t.Run("NotificationCenterPath", func(t *testing.T) {
		// Create a request to /notification-center
		req := httptest.NewRequest("GET", "/notification-center", nil)
		rr := httptest.NewRecorder()

		// Call the handler
		handler.Handle(rr, req)

		// Check status and content
		assert.Equal(t, http.StatusOK, rr.Code)
		assert.Contains(t, rr.Body.String(), "Notification Center Test")
	})

	t.Run("ConsolePath", func(t *testing.T) {
		// Create a request to a non-API path
		req := httptest.NewRequest("GET", "/dashboard", nil)
		rr := httptest.NewRecorder()

		// Call the handler
		handler.Handle(rr, req)

		// Check console is served with SPA fallback
		assert.Equal(t, http.StatusOK, rr.Code)
		assert.Contains(t, rr.Body.String(), "Console Test")
	})
}
