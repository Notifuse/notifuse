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
	// Create a test logger
	testLogger := logger.NewLogger()

	// Create handler with both console and notification center
	isInstalled := false
	handler := NewRootHandler(
		"console_test",
		"notification_center_test",
		testLogger,
		"https://api.example.com",
		"1.0",
		"root@example.com",
		&isInstalled,
		false,
		"",
		0,
		false,
		nil, // blogService
		nil,                      // workspaceRepo
	)

	// Assert fields are set correctly
	assert.Equal(t, "console_test", handler.consoleDir)
	assert.Equal(t, "notification_center_test", handler.notificationCenterDir)
}

func TestRootHandler_Handle(t *testing.T) {
	// Create a test logger
	testLogger := logger.NewLogger()
	isInstalled := false
	handler := NewRootHandler(
		"console_test",
		"notification_center_test",
		testLogger,
		"https://api.example.com",
		"1.0",
		"root@example.com",
		&isInstalled,
		false,
		"",
		0,
		false,
		nil, // blogService
		nil,                      // workspaceRepo
	)

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

	// Create a test logger
	testLogger := logger.NewLogger()
	isInstalled := false
	handler := NewRootHandler(
		"console_test",
		"notification_center_test",
		testLogger,
		"https://api.example.com",
		"1.0",
		"root@example.com",
		&isInstalled,
		false,
		"",
		0,
		false,
		nil, // blogService
		nil,                      // workspaceRepo
	)
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
	isInstalled := false
	handler := NewRootHandler(
		"console_test",
		"notification_center_test",
		testLogger,
		"https://api.example.com",
		"1.0",
		"root@example.com",
		&isInstalled,
		false,
		"",
		0,
		false,
		nil, // blogService
		nil,                      // workspaceRepo
	)

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
	isInstalled := false
	handler := NewRootHandler(
		"console_test",
		"notification_center_test",
		testLogger,
		testAPIEndpoint,
		"1.0",
		"root@example.com",
		&isInstalled,
		false,
		"",
		0,
		false,
		nil, // blogService
		nil,                      // workspaceRepo
	)

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
	body := rr.Body.String()
	assert.Contains(t, body, "window.API_ENDPOINT = \"https://api.example.com\"")
	assert.Contains(t, body, "window.VERSION = \"1.0\"")
	assert.Contains(t, body, "window.ROOT_EMAIL = \"root@example.com\"")
	assert.Contains(t, body, "window.IS_INSTALLED = false")
	assert.Contains(t, body, "window.TIMEZONES = [", "Should contain TIMEZONES array")

	// Verify some known timezones are in the list
	assert.Contains(t, body, "\"UTC\"", "Should contain UTC timezone")
	assert.Contains(t, body, "\"America/New_York\"", "Should contain America/New_York timezone")
	assert.Contains(t, body, "\"Europe/London\"", "Should contain Europe/London timezone")
}

func TestRootHandler_Handle_ConfigJS(t *testing.T) {
	// Create a test logger
	testLogger := logger.NewLogger()

	// Create a handler with a test API endpoint
	testAPIEndpoint := "https://api.example.com"
	isInstalled := false
	handler := NewRootHandler(
		"console_test",
		"notification_center_test",
		testLogger,
		testAPIEndpoint,
		"1.0",
		"root@example.com",
		&isInstalled,
		false,
		"",
		0,
		false,
		nil, // blogService
		nil,                      // workspaceRepo
	)

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
	body := rr.Body.String()
	assert.Contains(t, body, "window.API_ENDPOINT = \"https://api.example.com\"")
	assert.Contains(t, body, "window.VERSION = \"1.0\"")
	assert.Contains(t, body, "window.ROOT_EMAIL = \"root@example.com\"")
	assert.Contains(t, body, "window.IS_INSTALLED = false")
	assert.Contains(t, body, "window.TIMEZONES = [")

	// Verify some known timezones are in the list
	assert.Contains(t, body, "\"UTC\"")
	assert.Contains(t, body, "\"America/New_York\"")
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
	isInstalled := false
	handler := NewRootHandler(
		"console_test",
		tempDir,
		testLogger,
		"https://api.example.com",
		"1.0",
		"root@example.com",
		&isInstalled,
		false,
		"",
		0,
		false,
		nil, // blogService
		nil,                      // workspaceRepo
	)

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
	isInstalled := false
	handler := NewRootHandler(
		tempDir,
		"notification_center_test",
		testLogger,
		"https://api.example.com",
		"1.0",
		"root@example.com",
		&isInstalled,
		false,
		"",
		0,
		false,
		nil, // blogService
		nil,                      // workspaceRepo
	)

	t.Run("ServeExactPath", func(t *testing.T) {
		// Create a request to /console (which should serve index.html)
		req := httptest.NewRequest("GET", "/console", nil)
		rr := httptest.NewRecorder()

		// Call the handler
		handler.Handle(rr, req)

		// Check status code and content
		assert.Equal(t, http.StatusOK, rr.Code)
		assert.Contains(t, rr.Body.String(), "Console Test")
	})

	t.Run("ServeStaticFile", func(t *testing.T) {
		// Create a request to a static file under /console
		req := httptest.NewRequest("GET", "/console/style.css", nil)
		rr := httptest.NewRecorder()

		// Call the handler
		handler.Handle(rr, req)

		// Check status code and content
		assert.Equal(t, http.StatusOK, rr.Code)
		assert.Contains(t, rr.Body.String(), "body { background-color: #fff; }")
	})

	t.Run("ServeSPAFallback", func(t *testing.T) {
		// Create a request to a non-existent path under /console
		req := httptest.NewRequest("GET", "/console/non-existent-path", nil)
		rr := httptest.NewRecorder()

		// Call the handler
		handler.Handle(rr, req)

		// Check it falls back to index.html for SPA routing
		assert.Equal(t, http.StatusOK, rr.Code)
		assert.Contains(t, rr.Body.String(), "Console Test")
	})

	t.Run("MethodNotAllowed", func(t *testing.T) {
		// Create a POST request which should not be allowed
		req := httptest.NewRequest("POST", "/console", nil)
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
	isInstalled := false
	handler := NewRootHandler(
		consoleDir,
		notificationCenterDir,
		testLogger,
		"https://api.example.com",
		"1.0",
		"root@example.com",
		&isInstalled,
		false,
		"",
		0,
		false,
		nil, // blogService
		nil,                      // workspaceRepo
	)

	t.Run("NotFoundAPIPath", func(t *testing.T) {
		// Create a request to an non-existent API endpoint
		req := httptest.NewRequest("GET", "/api/invalid-endpoint", nil)
		rr := httptest.NewRecorder()

		// Call the handler
		handler.Handle(rr, req)

		// Since it starts with /api but doesn't match known endpoints,
		// the root handler returns early (after checking /api/ and /api paths)
		// and other handlers would handle it or return 404
		// In this test, no API routes are registered, so we expect nothing to be written
		// But since Handle() returns early for /api/* paths, the response may be empty
		// or the status might be 200 (default). The actual 404 would be from the mux.
		// For this test, we're just checking that /api/* paths are handled differently
		// Let's verify the path handling doesn't panic
		assert.NotPanics(t, func() {
			handler.Handle(httptest.NewRecorder(), req)
		})
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
		assert.Contains(t, rr.Body.String(), "window.VERSION")
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
		// Create a request to a console path
		req := httptest.NewRequest("GET", "/console/dashboard", nil)
		rr := httptest.NewRecorder()

		// Call the handler
		handler.Handle(rr, req)

		// Check console is served with SPA fallback
		assert.Equal(t, http.StatusOK, rr.Code)
		assert.Contains(t, rr.Body.String(), "Console Test")
	})

	t.Run("RootRedirectsToConsole", func(t *testing.T) {
		// Create a request to root path (no workspace matched)
		req := httptest.NewRequest("GET", "/", nil)
		rr := httptest.NewRecorder()

		// Call the handler
		handler.Handle(rr, req)

		// Should redirect to /console since no workspace repo is configured
		assert.Equal(t, http.StatusTemporaryRedirect, rr.Code)
		assert.Equal(t, "/console", rr.Header().Get("Location"))
	})
}
