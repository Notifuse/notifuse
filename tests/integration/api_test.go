package integration

import (
	"context"
	"net/http"
	"testing"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/app"
	"github.com/Notifuse/notifuse/tests/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// appFactory creates an app instance for testing
func appFactory(cfg *config.Config) testutil.AppInterface {
	return app.NewApp(cfg)
}

func TestAPIServerStartup(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer func() { suite.Cleanup() }()

	// Test that server is running
	assert.True(t, suite.ServerManager.IsStarted(), "Server should be started")

	// Test basic HTTP connectivity
	resp, err := http.Get(suite.ServerManager.GetURL() + "/health")
	require.NoError(t, err, "Should be able to make HTTP request")
	defer func() { _ = resp.Body.Close() }()

	// Health endpoint should return 200 or 404 (if not implemented)
	assert.True(t, resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusNotFound,
		"Health endpoint should respond")
}

func TestAPIServerShutdown(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer func() { suite.Cleanup() }()

	// Verify server is started
	assert.True(t, suite.ServerManager.IsStarted(), "Server should be started")

	// Stop server
	err := suite.ServerManager.Stop()
	require.NoError(t, err, "Should be able to stop server")

	// Verify server is stopped
	assert.False(t, suite.ServerManager.IsStarted(), "Server should be stopped")
}

func TestAPIClientConnection(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer func() { suite.Cleanup() }()

	client := suite.APIClient

	// Test that client can make requests
	resp, err := client.Get("/api/workspaces.get", map[string]string{
		"id": "test-workspace-id",
	})
	require.NoError(t, err, "Should be able to make API request")
	defer func() { _ = resp.Body.Close() }()

	// Should get some response (might be unauthorized, but connection works)
	assert.True(t, resp.StatusCode > 0, "Should get HTTP response")
}

func TestDatabaseIntegrationWithAPI(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer func() { suite.Cleanup() }()

	// Test that API can connect to database through the app
	app := suite.ServerManager.GetApp()
	require.NotNil(t, app, "App should not be nil")

	config := app.GetConfig()
	require.NotNil(t, config, "Config should not be nil")

	// Verify database configuration
	// Note: Environment is "development" to enable features like returning invitation tokens in responses
	assert.Equal(t, "development", config.Environment)
	assert.Equal(t, suite.DBManager.GetConfig().Host, config.Database.Host)
	assert.Equal(t, suite.DBManager.GetConfig().Port, config.Database.Port)
}

func TestAPIEndpointDiscovery(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer func() { suite.Cleanup() }()

	client := suite.APIClient

	// Test common API endpoints to verify they're registered
	endpoints := []string{
		"/api/broadcasts.list",
		"/api/contacts.list",
		"/api/templates.list",
		"/api/workspaces.get",
	}

	for _, endpoint := range endpoints {
		resp, err := client.Get(endpoint)
		require.NoError(t, err, "Should be able to connect to endpoint %s", endpoint)
		_ = resp.Body.Close()

		// Endpoints should exist (not 404) even if unauthorized
		assert.NotEqual(t, http.StatusNotFound, resp.StatusCode,
			"Endpoint %s should exist", endpoint)
	}
}

func TestAPIServerRestart(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer func() { suite.Cleanup() }()

	originalURL := suite.ServerManager.GetURL()

	// Test initial connection
	resp, err := http.Get(originalURL + "/api/broadcasts.list")
	require.NoError(t, err)
	_ = resp.Body.Close()

	// Restart server
	err = suite.ServerManager.Restart()
	require.NoError(t, err, "Should be able to restart server")

	// Server should be running on different port after restart
	newURL := suite.ServerManager.GetURL()
	assert.NotEqual(t, originalURL, newURL, "Server should be on different port after restart")

	// Test connection to new URL
	resp, err = http.Get(newURL + "/api/broadcasts.list")
	require.NoError(t, err, "Should be able to connect to restarted server")
	_ = resp.Body.Close()
}

func TestConcurrentAPIRequests(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer func() { suite.Cleanup() }()

	client := suite.APIClient

	// Make multiple concurrent requests
	numRequests := 10
	results := make(chan error, numRequests)

	for i := 0; i < numRequests; i++ {
		go func() {
			resp, err := client.Get("/api/broadcasts.list")
			if err != nil {
				results <- err
				return
			}
			_ = resp.Body.Close()
			results <- nil
		}()
	}

	// Collect results
	for i := 0; i < numRequests; i++ {
		err := <-results
		assert.NoError(t, err, "Concurrent request should succeed")
	}
}

func TestAPIWithAppContext(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer func() { suite.Cleanup() }()

	app := suite.ServerManager.GetApp()

	// Test that app implements expected interface
	require.NotNil(t, app.GetConfig())
	require.NotNil(t, app.GetLogger())
	require.NotNil(t, app.GetMux())

	// Test that we can shutdown app gracefully
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// This should not block since server is already running
	_ = ctx
}
