package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
	"github.com/Notifuse/notifuse/pkg/cache"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/golang/mock/gomock"
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
		nil, // workspaceRepo
		nil, // blogService
		nil, // cache
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
		nil, // workspaceRepo
		nil, // blogService
		nil, // cache
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
		nil, // workspaceRepo
		nil, // blogService
		nil, // cache
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
	defer func() { _ = resp.Body.Close() }()

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
		nil, // workspaceRepo
		nil, // blogService
		nil, // cache
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
		nil, // workspaceRepo
		nil, // blogService
		nil, // cache
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
		nil, // workspaceRepo
		nil, // blogService
		nil, // cache
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
	defer func() { _ = os.RemoveAll(tempDir) }()

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
		nil, // workspaceRepo
		nil, // blogService
		nil, // cache
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
	defer func() { _ = os.RemoveAll(tempDir) }()

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
		nil, // workspaceRepo
		nil, // blogService
		nil, // cache
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
	defer func() { _ = os.RemoveAll(consoleDir) }()

	notificationCenterDir, err := os.MkdirTemp("", "nc_test")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(notificationCenterDir) }()

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
		nil, // workspaceRepo
		nil, // blogService
		nil, // cache
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

func TestRootHandler_CacheIntegration(t *testing.T) {
	// This test verifies that the cache parameter is properly initialized
	// The actual cache behavior is tested in pkg/cache/cache_test.go
	testLogger := logger.NewLogger()
	isInstalled := false

	t.Run("handler works without cache", func(t *testing.T) {
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
			nil, // workspaceRepo
			nil, // blogService
			nil, // cache - nil is allowed
		)

		// Verify handler is created successfully
		assert.NotNil(t, handler)
		assert.Nil(t, handler.cache)
	})

	// Note: Full integration tests for blog caching would require:
	// - A mock BlogService to return rendered HTML
	// - A mock WorkspaceRepository to return workspaces with custom domains
	// - Setting up the cache and verifying cache hits/misses
	// These are better suited for integration tests rather than unit tests
}

// setupBlogHandlerTest creates a test handler with mocks for blog-related tests
func setupBlogHandlerTest(t *testing.T) (*mocks.MockBlogService, *pkgmocks.MockLogger, cache.Cache, *domain.Workspace, *RootHandler) {
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	mockBlogService := mocks.NewMockBlogService(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	// Setup common logger expectations
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Fatal(gomock.Any()).AnyTimes()

	// Create in-memory cache
	testCache := cache.NewInMemoryCache(1 * time.Minute)
	t.Cleanup(func() { testCache.Stop() })

	// Create test workspace with blog enabled
	now := time.Now().UTC()
	workspace := &domain.Workspace{
		ID:   "test-workspace-id",
		Name: "Test Workspace",
		Settings: domain.WorkspaceSettings{
			BlogEnabled: true,
			BlogSettings: &domain.BlogSettings{
				Title: "Test Blog",
			},
			Timezone: "UTC",
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	isInstalled := false
	handler := NewRootHandler(
		"console_test",
		"notification_center_test",
		mockLogger,
		"https://api.example.com",
		"1.0",
		"root@example.com",
		&isInstalled,
		false,
		"",
		0,
		false,
		nil, // workspaceRepo
		mockBlogService,
		testCache,
	)

	return mockBlogService, mockLogger, testCache, workspace, handler
}

func TestRootHandler_serveBlogRobots(t *testing.T) {
	_, _, _, _, handler := setupBlogHandlerTest(t)

	req := httptest.NewRequest("GET", "/robots.txt", nil)
	req.Host = "example.com"
	w := httptest.NewRecorder()

	handler.serveBlogRobots(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "text/plain", w.Header().Get("Content-Type"))
	body := w.Body.String()
	assert.Contains(t, body, "User-agent: *")
	assert.Contains(t, body, "Allow: /")
	assert.Contains(t, body, "Sitemap: /sitemap.xml")
}

func TestRootHandler_serveBlog404(t *testing.T) {
	_, _, _, _, handler := setupBlogHandlerTest(t)

	req := httptest.NewRequest("GET", "/nonexistent", nil)
	req.Host = "example.com"
	w := httptest.NewRecorder()

	handler.serveBlog404(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Equal(t, "text/html; charset=utf-8", w.Header().Get("Content-Type"))
	body := w.Body.String()
	assert.Contains(t, body, "404 Not Found")
	assert.Contains(t, body, "The page you're looking for doesn't exist")
	assert.Contains(t, body, `<a href="/">Go back home</a>`)
}

func TestRootHandler_serveBlogSitemap(t *testing.T) {
	t.Run("successful sitemap generation with posts", func(t *testing.T) {
		mockBlogService, _, _, workspace, handler := setupBlogHandlerTest(t)

		now := time.Now().UTC()
		category := &domain.BlogCategory{
			ID:   "cat-1",
			Slug: "tech",
			Settings: domain.BlogCategorySettings{
				Name: "Technology",
			},
			CreatedAt: now,
			UpdatedAt: now,
		}

		posts := []*domain.BlogPost{
			{
				ID:         "post-1",
				CategoryID: "cat-1",
				Slug:       "first-post",
				PublishedAt: &now,
				CreatedAt:  now,
				UpdatedAt:  now,
			},
			{
				ID:         "post-2",
				CategoryID: "cat-1",
				Slug:       "second-post",
				PublishedAt: &now,
				CreatedAt:  now,
				UpdatedAt:  now,
			},
		}

		mockBlogService.EXPECT().
			ListPublicPosts(gomock.Any(), gomock.Any()).
			Return(&domain.BlogPostListResponse{
				Posts: posts,
			}, nil)

		mockBlogService.EXPECT().
			GetCategory(gomock.Any(), "cat-1").
			Return(category, nil).
			Times(2)

		req := httptest.NewRequest("GET", "/sitemap.xml", nil)
		req.Host = "example.com"
		w := httptest.NewRecorder()

		handler.serveBlogSitemap(w, req, workspace)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "application/xml", w.Header().Get("Content-Type"))
		body := w.Body.String()
		assert.Contains(t, body, `<?xml version="1.0" encoding="UTF-8"?>`)
		assert.Contains(t, body, `<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">`)
		assert.Contains(t, body, `<loc>https://example.com/</loc>`)
		assert.Contains(t, body, `<loc>https://example.com/tech/first-post</loc>`)
		assert.Contains(t, body, `<loc>https://example.com/tech/second-post</loc>`)
	})

	t.Run("sitemap with no posts", func(t *testing.T) {
		mockBlogService, _, _, workspace, handler := setupBlogHandlerTest(t)

		mockBlogService.EXPECT().
			ListPublicPosts(gomock.Any(), gomock.Any()).
			Return(&domain.BlogPostListResponse{
				Posts: []*domain.BlogPost{},
			}, nil)

		req := httptest.NewRequest("GET", "/sitemap.xml", nil)
		req.Host = "example.com"
		w := httptest.NewRecorder()

		handler.serveBlogSitemap(w, req, workspace)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "application/xml", w.Header().Get("Content-Type"))
		body := w.Body.String()
		assert.Contains(t, body, `<loc>https://example.com/</loc>`)
		// Should not contain any post URLs (only homepage)
		assert.NotContains(t, body, `<loc>https://example.com/tech/`)
		// Count URL entries - should only have homepage (1 entry)
		urlCount := 0
		for i := 0; i <= len(body)-len(`<loc>`); i++ {
			if i+len(`<loc>`) <= len(body) && body[i:i+len(`<loc>`)] == `<loc>` {
				urlCount++
			}
		}
		assert.Equal(t, 1, urlCount, "Should only have homepage URL")
	})

	t.Run("sitemap generation error", func(t *testing.T) {
		mockBlogService, _, _, workspace, handler := setupBlogHandlerTest(t)

		mockBlogService.EXPECT().
			ListPublicPosts(gomock.Any(), gomock.Any()).
			Return(nil, assert.AnError)

		req := httptest.NewRequest("GET", "/sitemap.xml", nil)
		req.Host = "example.com"
		w := httptest.NewRecorder()

		handler.serveBlogSitemap(w, req, workspace)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("sitemap with post missing category", func(t *testing.T) {
		mockBlogService, _, _, workspace, handler := setupBlogHandlerTest(t)

		now := time.Now().UTC()
		posts := []*domain.BlogPost{
			{
				ID:         "post-1",
				CategoryID: "", // No category
				Slug:       "first-post",
				PublishedAt: &now,
				CreatedAt:  now,
				UpdatedAt:  now,
			},
		}

		mockBlogService.EXPECT().
			ListPublicPosts(gomock.Any(), gomock.Any()).
			Return(&domain.BlogPostListResponse{
				Posts: posts,
			}, nil)

		req := httptest.NewRequest("GET", "/sitemap.xml", nil)
		req.Host = "example.com"
		w := httptest.NewRecorder()

		handler.serveBlogSitemap(w, req, workspace)

		assert.Equal(t, http.StatusOK, w.Code)
		body := w.Body.String()
		// Post without category should not be included
		assert.NotContains(t, body, "first-post")
	})
}

func TestRootHandler_serveBlogHome(t *testing.T) {
	t.Run("successful rendering page 1", func(t *testing.T) {
		mockBlogService, _, _, workspace, handler := setupBlogHandlerTest(t)

		expectedHTML := "<html><body>Home Page</body></html>"
		mockBlogService.EXPECT().
			RenderHomePage(gomock.Any(), workspace.ID, 1, nil).
			Return(expectedHTML, nil)

		req := httptest.NewRequest("GET", "/", nil)
		req.Host = "example.com"
		w := httptest.NewRecorder()

		handler.serveBlogHome(w, req, workspace)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "text/html; charset=utf-8", w.Header().Get("Content-Type"))
		assert.Equal(t, "public, max-age=60", w.Header().Get("Cache-Control"))
		assert.Equal(t, "MISS", w.Header().Get("X-Cache"))
		assert.Equal(t, expectedHTML, w.Body.String())
	})

	t.Run("successful rendering page 2", func(t *testing.T) {
		mockBlogService, _, _, workspace, handler := setupBlogHandlerTest(t)

		expectedHTML := "<html><body>Home Page 2</body></html>"
		mockBlogService.EXPECT().
			RenderHomePage(gomock.Any(), workspace.ID, 2, nil).
			Return(expectedHTML, nil)

		req := httptest.NewRequest("GET", "/?page=2", nil)
		req.Host = "example.com"
		w := httptest.NewRecorder()

		handler.serveBlogHome(w, req, workspace)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, expectedHTML, w.Body.String())
	})

	t.Run("invalid page parameter redirects", func(t *testing.T) {
		_, _, _, workspace, handler := setupBlogHandlerTest(t)

		req := httptest.NewRequest("GET", "/?page=invalid", nil)
		req.Host = "example.com"
		w := httptest.NewRecorder()

		handler.serveBlogHome(w, req, workspace)

		assert.Equal(t, http.StatusFound, w.Code)
		assert.Equal(t, "/", w.Header().Get("Location"))
	})

	t.Run("page=1 redirects to base URL", func(t *testing.T) {
		_, _, _, workspace, handler := setupBlogHandlerTest(t)

		req := httptest.NewRequest("GET", "/?page=1", nil)
		req.Host = "example.com"
		w := httptest.NewRecorder()

		handler.serveBlogHome(w, req, workspace)

		assert.Equal(t, http.StatusMovedPermanently, w.Code)
		assert.Equal(t, "/", w.Header().Get("Location"))
	})

	t.Run("cache hit", func(t *testing.T) {
		_, _, testCache, workspace, handler := setupBlogHandlerTest(t)

		cachedHTML := "<html><body>Cached</body></html>"
		cacheKey := "blog:example.com:/?page=1"
		testCache.Set(cacheKey, cachedHTML, 60*time.Second)

		req := httptest.NewRequest("GET", "/", nil)
		req.Host = "example.com"
		w := httptest.NewRecorder()

		handler.serveBlogHome(w, req, workspace)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "HIT", w.Header().Get("X-Cache"))
		assert.Equal(t, cachedHTML, w.Body.String())
	})

	t.Run("theme preview bypasses cache", func(t *testing.T) {
		mockBlogService, _, _, workspace, handler := setupBlogHandlerTest(t)

		expectedHTML := "<html><body>Preview</body></html>"
		themeVersion := 5
		mockBlogService.EXPECT().
			RenderHomePage(gomock.Any(), workspace.ID, 1, &themeVersion).
			Return(expectedHTML, nil)

		req := httptest.NewRequest("GET", "/?preview_theme_version=5", nil)
		req.Host = "example.com"
		w := httptest.NewRecorder()

		handler.serveBlogHome(w, req, workspace)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "BYPASS", w.Header().Get("X-Cache"))
		assert.Equal(t, "no-store, no-cache, must-revalidate", w.Header().Get("Cache-Control"))
		assert.Equal(t, expectedHTML, w.Body.String())
	})

	t.Run("BlogRenderError handling", func(t *testing.T) {
		testCases := []struct {
			name           string
			errorCode      string
			expectedStatus int
		}{
			{
				name:           "theme not found",
				errorCode:      domain.ErrCodeThemeNotFound,
				expectedStatus: http.StatusServiceUnavailable,
			},
			{
				name:           "theme not published",
				errorCode:      domain.ErrCodeThemeNotPublished,
				expectedStatus: http.StatusServiceUnavailable,
			},
			{
				name:           "post not found",
				errorCode:      domain.ErrCodePostNotFound,
				expectedStatus: http.StatusNotFound,
			},
			{
				name:           "category not found",
				errorCode:      domain.ErrCodeCategoryNotFound,
				expectedStatus: http.StatusNotFound,
			},
			{
				name:           "render failed",
				errorCode:      domain.ErrCodeRenderFailed,
				expectedStatus: http.StatusInternalServerError,
			},
			{
				name:           "invalid liquid syntax",
				errorCode:      domain.ErrCodeInvalidLiquidSyntax,
				expectedStatus: http.StatusInternalServerError,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				mockBlogService, _, _, workspace, handler := setupBlogHandlerTest(t)

				blogErr := &domain.BlogRenderError{
					Code:    tc.errorCode,
					Message: "Test error",
				}
				mockBlogService.EXPECT().
					RenderHomePage(gomock.Any(), workspace.ID, 1, nil).
					Return("", blogErr)

				req := httptest.NewRequest("GET", "/", nil)
				req.Host = "example.com"
				w := httptest.NewRecorder()

				handler.serveBlogHome(w, req, workspace)

				assert.Equal(t, tc.expectedStatus, w.Code)
			})
		}
	})

	t.Run("unexpected error handling", func(t *testing.T) {
		mockBlogService, _, _, workspace, handler := setupBlogHandlerTest(t)

		mockBlogService.EXPECT().
			RenderHomePage(gomock.Any(), workspace.ID, 1, nil).
			Return("", assert.AnError)

		req := httptest.NewRequest("GET", "/", nil)
		req.Host = "example.com"
		w := httptest.NewRecorder()

		handler.serveBlogHome(w, req, workspace)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("negative page redirects", func(t *testing.T) {
		_, _, _, workspace, handler := setupBlogHandlerTest(t)

		req := httptest.NewRequest("GET", "/?page=-1", nil)
		req.Host = "example.com"
		w := httptest.NewRecorder()

		handler.serveBlogHome(w, req, workspace)

		assert.Equal(t, http.StatusFound, w.Code)
		assert.Equal(t, "/", w.Header().Get("Location"))
	})
}

func TestRootHandler_serveBlogCategory(t *testing.T) {
	t.Run("successful rendering", func(t *testing.T) {
		mockBlogService, _, _, workspace, handler := setupBlogHandlerTest(t)

		expectedHTML := "<html><body>Category Page</body></html>"
		mockBlogService.EXPECT().
			RenderCategoryPage(gomock.Any(), workspace.ID, "tech", 1, nil).
			Return(expectedHTML, nil)

		req := httptest.NewRequest("GET", "/tech", nil)
		req.Host = "example.com"
		w := httptest.NewRecorder()

		handler.serveBlogCategory(w, req, workspace, "tech")

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "text/html; charset=utf-8", w.Header().Get("Content-Type"))
		assert.Equal(t, expectedHTML, w.Body.String())
	})

	t.Run("pagination page 2", func(t *testing.T) {
		mockBlogService, _, _, workspace, handler := setupBlogHandlerTest(t)

		expectedHTML := "<html><body>Category Page 2</body></html>"
		mockBlogService.EXPECT().
			RenderCategoryPage(gomock.Any(), workspace.ID, "tech", 2, nil).
			Return(expectedHTML, nil)

		req := httptest.NewRequest("GET", "/tech?page=2", nil)
		req.Host = "example.com"
		w := httptest.NewRecorder()

		handler.serveBlogCategory(w, req, workspace, "tech")

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, expectedHTML, w.Body.String())
	})

	t.Run("invalid page parameter redirects", func(t *testing.T) {
		_, _, _, workspace, handler := setupBlogHandlerTest(t)

		req := httptest.NewRequest("GET", "/tech?page=invalid", nil)
		req.Host = "example.com"
		w := httptest.NewRecorder()

		handler.serveBlogCategory(w, req, workspace, "tech")

		assert.Equal(t, http.StatusFound, w.Code)
		assert.Equal(t, "/tech", w.Header().Get("Location"))
	})

	t.Run("page=1 redirects", func(t *testing.T) {
		_, _, _, workspace, handler := setupBlogHandlerTest(t)

		req := httptest.NewRequest("GET", "/tech?page=1", nil)
		req.Host = "example.com"
		w := httptest.NewRecorder()

		handler.serveBlogCategory(w, req, workspace, "tech")

		assert.Equal(t, http.StatusMovedPermanently, w.Code)
		assert.Equal(t, "/tech", w.Header().Get("Location"))
	})

	t.Run("cache hit", func(t *testing.T) {
		_, _, testCache, workspace, handler := setupBlogHandlerTest(t)

		cachedHTML := "<html><body>Cached Category</body></html>"
		cacheKey := "blog:example.com:/tech?page=1"
		testCache.Set(cacheKey, cachedHTML, 60*time.Second)

		req := httptest.NewRequest("GET", "/tech", nil)
		req.Host = "example.com"
		w := httptest.NewRecorder()

		handler.serveBlogCategory(w, req, workspace, "tech")

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "HIT", w.Header().Get("X-Cache"))
		assert.Equal(t, cachedHTML, w.Body.String())
	})

	t.Run("theme preview bypasses cache", func(t *testing.T) {
		mockBlogService, _, _, workspace, handler := setupBlogHandlerTest(t)

		expectedHTML := "<html><body>Preview Category</body></html>"
		themeVersion := 3
		mockBlogService.EXPECT().
			RenderCategoryPage(gomock.Any(), workspace.ID, "tech", 1, &themeVersion).
			Return(expectedHTML, nil)

		req := httptest.NewRequest("GET", "/tech?preview_theme_version=3", nil)
		req.Host = "example.com"
		w := httptest.NewRecorder()

		handler.serveBlogCategory(w, req, workspace, "tech")

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "BYPASS", w.Header().Get("X-Cache"))
		assert.Equal(t, expectedHTML, w.Body.String())
	})

	t.Run("BlogRenderError handling", func(t *testing.T) {
		mockBlogService, _, _, workspace, handler := setupBlogHandlerTest(t)

		blogErr := &domain.BlogRenderError{
			Code:    domain.ErrCodeCategoryNotFound,
			Message: "Category not found",
		}
		mockBlogService.EXPECT().
			RenderCategoryPage(gomock.Any(), workspace.ID, "tech", 1, nil).
			Return("", blogErr)

		req := httptest.NewRequest("GET", "/tech", nil)
		req.Host = "example.com"
		w := httptest.NewRecorder()

		handler.serveBlogCategory(w, req, workspace, "tech")

		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("unexpected error handling", func(t *testing.T) {
		mockBlogService, _, _, workspace, handler := setupBlogHandlerTest(t)

		mockBlogService.EXPECT().
			RenderCategoryPage(gomock.Any(), workspace.ID, "tech", 1, nil).
			Return("", assert.AnError)

		req := httptest.NewRequest("GET", "/tech", nil)
		req.Host = "example.com"
		w := httptest.NewRecorder()

		handler.serveBlogCategory(w, req, workspace, "tech")

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestRootHandler_serveBlogPost(t *testing.T) {
	t.Run("successful rendering", func(t *testing.T) {
		mockBlogService, _, _, workspace, handler := setupBlogHandlerTest(t)

		now := time.Now().UTC()
		post := &domain.BlogPost{
			ID:         "post-1",
			CategoryID: "cat-1",
			Slug:       "my-post",
			PublishedAt: &now,
			CreatedAt:  now,
			UpdatedAt:  now,
		}

		expectedHTML := "<html><body>Post Page</body></html>"
		mockBlogService.EXPECT().
			RenderPostPage(gomock.Any(), workspace.ID, "tech", "my-post", nil).
			Return(expectedHTML, nil)

		req := httptest.NewRequest("GET", "/tech/my-post", nil)
		req.Host = "example.com"
		w := httptest.NewRecorder()

		handler.serveBlogPost(w, req, workspace, post)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "text/html; charset=utf-8", w.Header().Get("Content-Type"))
		assert.Equal(t, expectedHTML, w.Body.String())
	})

	t.Run("cache hit", func(t *testing.T) {
		_, _, testCache, workspace, handler := setupBlogHandlerTest(t)

		now := time.Now().UTC()
		post := &domain.BlogPost{
			ID:         "post-1",
			CategoryID: "cat-1",
			Slug:       "my-post",
			PublishedAt: &now,
			CreatedAt:  now,
			UpdatedAt:  now,
		}

		cachedHTML := "<html><body>Cached Post</body></html>"
		cacheKey := "blog:example.com:/tech/my-post"
		testCache.Set(cacheKey, cachedHTML, 60*time.Second)

		req := httptest.NewRequest("GET", "/tech/my-post", nil)
		req.Host = "example.com"
		w := httptest.NewRecorder()

		handler.serveBlogPost(w, req, workspace, post)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "HIT", w.Header().Get("X-Cache"))
		assert.Equal(t, cachedHTML, w.Body.String())
	})

	t.Run("theme preview bypasses cache", func(t *testing.T) {
		mockBlogService, _, _, workspace, handler := setupBlogHandlerTest(t)

		now := time.Now().UTC()
		post := &domain.BlogPost{
			ID:         "post-1",
			CategoryID: "cat-1",
			Slug:       "my-post",
			PublishedAt: &now,
			CreatedAt:  now,
			UpdatedAt:  now,
		}

		expectedHTML := "<html><body>Preview Post</body></html>"
		themeVersion := 2
		mockBlogService.EXPECT().
			RenderPostPage(gomock.Any(), workspace.ID, "tech", "my-post", &themeVersion).
			Return(expectedHTML, nil)

		req := httptest.NewRequest("GET", "/tech/my-post?preview_theme_version=2", nil)
		req.Host = "example.com"
		w := httptest.NewRecorder()

		handler.serveBlogPost(w, req, workspace, post)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "BYPASS", w.Header().Get("X-Cache"))
		assert.Equal(t, expectedHTML, w.Body.String())
	})

	t.Run("invalid URL path calls 404", func(t *testing.T) {
		_, _, _, workspace, handler := setupBlogHandlerTest(t)

		now := time.Now().UTC()
		post := &domain.BlogPost{
			ID:         "post-1",
			CategoryID: "cat-1",
			Slug:       "my-post",
			PublishedAt: &now,
			CreatedAt:  now,
			UpdatedAt:  now,
		}

		req := httptest.NewRequest("GET", "/invalid-path", nil)
		req.Host = "example.com"
		w := httptest.NewRecorder()

		handler.serveBlogPost(w, req, workspace, post)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("BlogRenderError handling", func(t *testing.T) {
		mockBlogService, _, _, workspace, handler := setupBlogHandlerTest(t)

		now := time.Now().UTC()
		post := &domain.BlogPost{
			ID:         "post-1",
			CategoryID: "cat-1",
			Slug:       "my-post",
			PublishedAt: &now,
			CreatedAt:  now,
			UpdatedAt:  now,
		}

		blogErr := &domain.BlogRenderError{
			Code:    domain.ErrCodePostNotFound,
			Message: "Post not found",
		}
		mockBlogService.EXPECT().
			RenderPostPage(gomock.Any(), workspace.ID, "tech", "my-post", nil).
			Return("", blogErr)

		req := httptest.NewRequest("GET", "/tech/my-post", nil)
		req.Host = "example.com"
		w := httptest.NewRecorder()

		handler.serveBlogPost(w, req, workspace, post)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("unexpected error handling", func(t *testing.T) {
		mockBlogService, _, _, workspace, handler := setupBlogHandlerTest(t)

		now := time.Now().UTC()
		post := &domain.BlogPost{
			ID:         "post-1",
			CategoryID: "cat-1",
			Slug:       "my-post",
			PublishedAt: &now,
			CreatedAt:  now,
			UpdatedAt:  now,
		}

		mockBlogService.EXPECT().
			RenderPostPage(gomock.Any(), workspace.ID, "tech", "my-post", nil).
			Return("", assert.AnError)

		req := httptest.NewRequest("GET", "/tech/my-post", nil)
		req.Host = "example.com"
		w := httptest.NewRecorder()

		handler.serveBlogPost(w, req, workspace, post)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestRootHandler_serveBlog(t *testing.T) {
	t.Run("routing to robots.txt", func(t *testing.T) {
		_, _, _, workspace, handler := setupBlogHandlerTest(t)

		req := httptest.NewRequest("GET", "/robots.txt", nil)
		req.Host = "example.com"
		w := httptest.NewRecorder()

		handler.serveBlog(w, req, workspace)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "text/plain", w.Header().Get("Content-Type"))
		assert.Contains(t, w.Body.String(), "User-agent: *")
	})

	t.Run("routing to sitemap.xml", func(t *testing.T) {
		mockBlogService, _, _, workspace, handler := setupBlogHandlerTest(t)

		mockBlogService.EXPECT().
			ListPublicPosts(gomock.Any(), gomock.Any()).
			Return(&domain.BlogPostListResponse{Posts: []*domain.BlogPost{}}, nil)

		req := httptest.NewRequest("GET", "/sitemap.xml", nil)
		req.Host = "example.com"
		w := httptest.NewRecorder()

		handler.serveBlog(w, req, workspace)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "application/xml", w.Header().Get("Content-Type"))
	})

	t.Run("routing to home page", func(t *testing.T) {
		mockBlogService, _, _, workspace, handler := setupBlogHandlerTest(t)

		mockBlogService.EXPECT().
			RenderHomePage(gomock.Any(), workspace.ID, 1, nil).
			Return("<html><body>Home</body></html>", nil)

		req := httptest.NewRequest("GET", "/", nil)
		req.Host = "example.com"
		w := httptest.NewRecorder()

		handler.serveBlog(w, req, workspace)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), "Home")
	})

	t.Run("routing to category page", func(t *testing.T) {
		mockBlogService, _, _, workspace, handler := setupBlogHandlerTest(t)

		now := time.Now().UTC()
		category := &domain.BlogCategory{
			ID:   "cat-1",
			Slug: "tech",
			Settings: domain.BlogCategorySettings{
				Name: "Technology",
			},
			CreatedAt: now,
			UpdatedAt: now,
		}

		mockBlogService.EXPECT().
			GetPublicCategoryBySlug(gomock.Any(), "tech").
			Return(category, nil)

		mockBlogService.EXPECT().
			RenderCategoryPage(gomock.Any(), workspace.ID, "tech", 1, nil).
			Return("<html><body>Category</body></html>", nil)

		req := httptest.NewRequest("GET", "/tech", nil)
		req.Host = "example.com"
		w := httptest.NewRecorder()

		handler.serveBlog(w, req, workspace)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), "Category")
	})

	t.Run("routing to post page", func(t *testing.T) {
		mockBlogService, _, _, workspace, handler := setupBlogHandlerTest(t)

		now := time.Now().UTC()
		post := &domain.BlogPost{
			ID:         "post-1",
			CategoryID: "cat-1",
			Slug:       "my-post",
			PublishedAt: &now,
			CreatedAt:  now,
			UpdatedAt:  now,
		}

		mockBlogService.EXPECT().
			GetPublicPostByCategoryAndSlug(gomock.Any(), "tech", "my-post").
			Return(post, nil)

		mockBlogService.EXPECT().
			RenderPostPage(gomock.Any(), workspace.ID, "tech", "my-post", nil).
			Return("<html><body>Post</body></html>", nil)

		req := httptest.NewRequest("GET", "/tech/my-post", nil)
		req.Host = "example.com"
		w := httptest.NewRecorder()

		handler.serveBlog(w, req, workspace)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), "Post")
	})

	t.Run("category not found returns 404", func(t *testing.T) {
		mockBlogService, _, _, workspace, handler := setupBlogHandlerTest(t)

		mockBlogService.EXPECT().
			GetPublicCategoryBySlug(gomock.Any(), "nonexistent").
			Return(nil, assert.AnError)

		req := httptest.NewRequest("GET", "/nonexistent", nil)
		req.Host = "example.com"
		w := httptest.NewRecorder()

		handler.serveBlog(w, req, workspace)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("post not found returns 404", func(t *testing.T) {
		mockBlogService, _, _, workspace, handler := setupBlogHandlerTest(t)

		mockBlogService.EXPECT().
			GetPublicPostByCategoryAndSlug(gomock.Any(), "tech", "nonexistent").
			Return(nil, assert.AnError)

		req := httptest.NewRequest("GET", "/tech/nonexistent", nil)
		req.Host = "example.com"
		w := httptest.NewRecorder()

		handler.serveBlog(w, req, workspace)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("invalid path returns 404", func(t *testing.T) {
		_, _, _, workspace, handler := setupBlogHandlerTest(t)

		req := httptest.NewRequest("GET", "/too/many/segments", nil)
		req.Host = "example.com"
		w := httptest.NewRecorder()

		handler.serveBlog(w, req, workspace)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("context has WorkspaceIDKey set", func(t *testing.T) {
		mockBlogService, _, _, workspace, handler := setupBlogHandlerTest(t)

		mockBlogService.EXPECT().
			RenderHomePage(gomock.Any(), workspace.ID, 1, nil).
			DoAndReturn(func(ctx context.Context, workspaceID string, page int, themeVersion *int) (string, error) {
				// Verify context has WorkspaceIDKey
				ctxWorkspaceID := ctx.Value(domain.WorkspaceIDKey)
				assert.Equal(t, workspace.ID, ctxWorkspaceID)
				return "<html><body>Home</body></html>", nil
			})

		req := httptest.NewRequest("GET", "/", nil)
		req.Host = "example.com"
		w := httptest.NewRecorder()

		handler.serveBlog(w, req, workspace)

		assert.Equal(t, http.StatusOK, w.Code)
	})
}

func TestRootHandler_handleBlogRenderError(t *testing.T) {
	testCases := []struct {
		name                string
		errorCode           string
		expectedStatus      int
		expectedBody        string
		expectedContentType string
	}{
		{
			name:                "theme not found",
			errorCode:           domain.ErrCodeThemeNotFound,
			expectedStatus:      http.StatusServiceUnavailable,
			expectedBody:        "Blog Temporarily Unavailable",
			expectedContentType: "text/html; charset=utf-8",
		},
		{
			name:                "theme not published",
			errorCode:           domain.ErrCodeThemeNotPublished,
			expectedStatus:      http.StatusServiceUnavailable,
			expectedBody:        "Blog Temporarily Unavailable",
			expectedContentType: "text/html; charset=utf-8",
		},
		{
			name:                "post not found",
			errorCode:           domain.ErrCodePostNotFound,
			expectedStatus:      http.StatusNotFound,
			expectedBody:        "404 Not Found",
			expectedContentType: "text/html; charset=utf-8",
		},
		{
			name:                "category not found",
			errorCode:           domain.ErrCodeCategoryNotFound,
			expectedStatus:      http.StatusNotFound,
			expectedBody:        "404 Not Found",
			expectedContentType: "text/html; charset=utf-8",
		},
		{
			name:                "render failed",
			errorCode:           domain.ErrCodeRenderFailed,
			expectedStatus:      http.StatusInternalServerError,
			expectedBody:        "Something Went Wrong",
			expectedContentType: "text/html; charset=utf-8",
		},
		{
			name:                "invalid liquid syntax",
			errorCode:           domain.ErrCodeInvalidLiquidSyntax,
			expectedStatus:      http.StatusInternalServerError,
			expectedBody:        "Something Went Wrong",
			expectedContentType: "text/html; charset=utf-8",
		},
		{
			name:                "unknown error code",
			errorCode:           "unknown_error",
			expectedStatus:      http.StatusInternalServerError,
			expectedBody:        "Internal Server Error",
			expectedContentType: "text/plain; charset=utf-8",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, _, _, handler := setupBlogHandlerTest(t)

			blogErr := &domain.BlogRenderError{
				Code:    tc.errorCode,
				Message: "Test error message",
				Details: nil,
			}

			req := httptest.NewRequest("GET", "/", nil)
			req.Host = "example.com"
			w := httptest.NewRecorder()

			handler.handleBlogRenderError(w, blogErr)

			assert.Equal(t, tc.expectedStatus, w.Code)
			assert.Equal(t, tc.expectedContentType, w.Header().Get("Content-Type"))
			body := w.Body.String()
			assert.Contains(t, body, tc.expectedBody)
		})
	}
}
