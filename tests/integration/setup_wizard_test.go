package integration

import (
	"encoding/json"
	"net/http"
	"os"
	"testing"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/app"
	"github.com/Notifuse/notifuse/tests/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSetupWizardFlow tests the complete setup wizard flow
func TestSetupWizardFlow(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	// Create a custom test suite that doesn't seed the installation data
	// This allows us to test the setup wizard from scratch
	suite := createUninstalledTestSuite(t)
	defer suite.Cleanup()

	client := suite.APIClient

	t.Run("Status - Not Installed", func(t *testing.T) {
		// Check that the system is not installed
		resp, err := client.Get("/api/setup.status")
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var statusResp map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&statusResp)
		require.NoError(t, err)

		assert.False(t, statusResp["is_installed"].(bool), "System should not be installed initially")
	})

	t.Run("Initialize System", func(t *testing.T) {
		// Initialize the system
		initReq := map[string]interface{}{
			"root_email":      "admin@example.com",
			"api_endpoint":    suite.ServerManager.GetURL(),
			"smtp_host":       "localhost",
			"smtp_port":       1025,
			"smtp_from_email": "test@example.com",
			"smtp_from_name":  "Test Notifuse",
		}

		resp, err := client.Post("/api/setup.initialize", initReq)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var initResp map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&initResp)
		require.NoError(t, err)

		assert.True(t, initResp["success"].(bool), "Setup should succeed")
		assert.Contains(t, initResp["message"].(string), "restarting", "Should indicate server is restarting")
	})

	t.Run("Status - Installed", func(t *testing.T) {
		// Check that the system is now installed
		resp, err := client.Get("/api/setup.status")
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var statusResp map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&statusResp)
		require.NoError(t, err)

		assert.True(t, statusResp["is_installed"].(bool), "System should be installed now")
	})

	t.Run("Prevent Re-initialization", func(t *testing.T) {
		// Try to initialize again - should be rejected gracefully
		initReq := map[string]interface{}{
			"root_email":      "admin2@example.com",
			"api_endpoint":    suite.ServerManager.GetURL(),
			"smtp_host":       "localhost",
			"smtp_port":       1025,
			"smtp_from_email": "test@example.com",
			"smtp_from_name":  "Test Notifuse",
		}

		resp, err := client.Post("/api/setup.initialize", initReq)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var initResp map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&initResp)
		require.NoError(t, err)

		assert.True(t, initResp["success"].(bool), "Should return success for already installed")
		assert.Contains(t, initResp["message"].(string), "already completed", "Should indicate system is already installed")
	})
}

// TestSetupWizardWithJWT tests setup using JWT authentication (SECRET_KEY based)
func TestSetupWizardWithJWT(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := createUninstalledTestSuite(t)
	defer suite.Cleanup()

	client := suite.APIClient

	t.Run("Initialize with JWT", func(t *testing.T) {
		// System now uses JWT with SECRET_KEY (no need to provide keys)
		initReq := map[string]interface{}{
			"root_email":      "admin@example.com",
			"api_endpoint":    suite.ServerManager.GetURL(),
			"smtp_host":       "localhost",
			"smtp_port":       1025,
			"smtp_from_email": "test@example.com",
			"smtp_from_name":  "Test Notifuse",
		}

		resp, err := client.Post("/api/setup.initialize", initReq)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var initResp map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&initResp)
		require.NoError(t, err)

		assert.True(t, initResp["success"].(bool), "Setup should succeed")
		assert.Contains(t, initResp["message"].(string), "restarting", "Should indicate server is restarting")
	})
}

// TestSetupWizardValidation tests validation of setup wizard inputs
func TestSetupWizardValidation(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := createUninstalledTestSuite(t)
	defer suite.Cleanup()

	client := suite.APIClient

	testCases := []struct {
		name        string
		request     map[string]interface{}
		expectError bool
	}{
		{
			name: "Missing Root Email",
			request: map[string]interface{}{
				"smtp_host":       "localhost",
				"smtp_port":       1025,
				"smtp_from_email": "test@example.com",
			},
			expectError: true,
		},
		{
			name: "Missing SMTP Host",
			request: map[string]interface{}{
				"root_email":      "admin@example.com",
				"smtp_port":       1025,
				"smtp_from_email": "test@example.com",
			},
			expectError: true,
		},
		{
			name: "Missing SMTP From Email",
			request: map[string]interface{}{
				"root_email": "admin@example.com",
				"smtp_host":  "localhost",
				"smtp_port":  1025,
			},
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := client.Post("/api/setup.initialize", tc.request)
			require.NoError(t, err)
			defer resp.Body.Close()

			if tc.expectError {
				assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
			} else {
				assert.Equal(t, http.StatusOK, resp.StatusCode)
			}
		})
	}
}

// TestSetupWizardSMTPTest tests the SMTP connection testing endpoint
func TestSetupWizardSMTPTest(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := createUninstalledTestSuite(t)
	defer suite.Cleanup()

	client := suite.APIClient

	t.Run("Test SMTP Connection - Success", func(t *testing.T) {
		// Test with valid MailHog settings (running in Docker Compose)
		testReq := map[string]interface{}{
			"smtp_host": "localhost",
			"smtp_port": 1025,
		}

		resp, err := client.Post("/api/setup.testSmtp", testReq)
		require.NoError(t, err)
		defer resp.Body.Close()

		// MailHog may not be available in all test environments, so we accept both success and failure
		// The important thing is that the endpoint is working and returning proper responses
		if resp.StatusCode == http.StatusOK {
			var testResp map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&testResp)
			require.NoError(t, err)
			assert.True(t, testResp["success"].(bool), "SMTP test should succeed when MailHog is available")
		} else {
			// MailHog might not be available, which is okay
			assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "Should return bad request when SMTP is unavailable")
		}
	})

	t.Run("Test SMTP Connection - Invalid Host", func(t *testing.T) {
		// Test with invalid SMTP settings
		testReq := map[string]interface{}{
			"smtp_host": "invalid-host-that-does-not-exist.com",
			"smtp_port": 587,
		}

		resp, err := client.Post("/api/setup.testSmtp", testReq)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

		var errorResp map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&errorResp)
		require.NoError(t, err)

		assert.NotEmpty(t, errorResp["error"], "Should return error message")
	})

	t.Run("Test SMTP After Installation - Forbidden", func(t *testing.T) {
		// First install the system
		initReq := map[string]interface{}{
			"root_email":      "admin@example.com",
			"api_endpoint":    suite.ServerManager.GetURL(),
			"smtp_host":       "localhost",
			"smtp_port":       1025,
			"smtp_from_email": "test@example.com",
			"smtp_from_name":  "Test Notifuse",
		}

		resp, err := client.Post("/api/setup.initialize", initReq)
		require.NoError(t, err)
		resp.Body.Close()

		// Now try to test SMTP - should be forbidden
		testReq := map[string]interface{}{
			"smtp_host": "localhost",
			"smtp_port": 1025,
		}

		resp, err = client.Post("/api/setup.testSmtp", testReq)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	})
}

// TestSetupWizardEnvironmentOverrides tests that environment variables override setup wizard inputs
func TestSetupWizardEnvironmentOverrides(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	// This test would require setting environment variables before app initialization
	// For now, we'll skip this as it requires more complex test infrastructure
	t.Skip("Environment override testing requires complex test infrastructure")
}

// TestSetupWizardWithServerRestart tests that setup completion triggers server shutdown
// In production, Docker/systemd restarts the process with fresh configuration
func TestSetupWizardWithServerRestart(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := createUninstalledTestSuite(t)
	defer suite.Cleanup()

	client := suite.APIClient

	t.Run("Complete Setup Triggers Shutdown", func(t *testing.T) {
		// Step 1: Complete setup wizard with full SMTP configuration
		rootEmail := "admin@example.com"

		// Use environment variable for SMTP host (for containerized test environments)
		// Default to localhost for non-containerized environments
		smtpHost := os.Getenv("TEST_SMTP_HOST")
		if smtpHost == "" {
			smtpHost = "localhost"
		}

		initReq := map[string]interface{}{
			"root_email":      rootEmail,
			"api_endpoint":    suite.ServerManager.GetURL(),
			"smtp_host":       smtpHost,
			"smtp_port":       1025,
			"smtp_username":   "testuser",
			"smtp_password":   "testpass",
			"smtp_from_email": "noreply@example.com", // Important: non-empty from email
			"smtp_from_name":  "Test System",
		}

		resp, err := client.Post("/api/setup.initialize", initReq)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode, "Setup should succeed")

		var setupResp map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&setupResp)
		require.NoError(t, err)

		assert.True(t, setupResp["success"].(bool), "Setup should succeed")
		assert.Contains(t, setupResp["message"].(string), "restarting",
			"Response should indicate server is restarting")

		// Note: In production, Docker/systemd would restart the process here.
		// The test suite will be cleaned up, simulating process termination.
	})

	t.Run("Fresh Start After Simulated Restart", func(t *testing.T) {
		// Simulate what happens after Docker restarts the container:
		// The old app instance shuts down, and a new one starts with fresh config.
		// We verify this by creating a fresh test suite that loads config from database.

		// Clean up original suite (simulates process shutdown)
		suite.Cleanup()

		// Create fresh test suite (simulates Docker restart)
		// This will create a new app that loads config from the database where setup was saved
		freshSuite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
			return app.NewApp(cfg)
		})
		defer freshSuite.Cleanup()

		// Verify the fresh app loaded config correctly by testing signin
		signinReq := map[string]interface{}{
			"email": "admin@example.com",
		}

		signinResp, err := freshSuite.APIClient.Post("/api/user.signin", signinReq)
		require.NoError(t, err)
		defer signinResp.Body.Close()

		var signinResult map[string]interface{}
		err = json.NewDecoder(signinResp.Body).Decode(&signinResult)
		require.NoError(t, err)

		// Verify no mail parsing errors (the original bug is fixed by restart)
		if errorMsg, ok := signinResult["error"].(string); ok {
			assert.NotContains(t, errorMsg, "failed to parse mail address",
				"Fresh start should not have mail parsing error")
			assert.NotContains(t, errorMsg, "mail: invalid string",
				"Fresh start should not have invalid mail error")
		}
	})
}

// createUninstalledTestSuite creates a test suite without seeding installation data
// This allows testing the setup wizard from a clean state
func createUninstalledTestSuite(t *testing.T) *testutil.IntegrationTestSuite {
	// Skip if not running integration tests
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	suite := &testutil.IntegrationTestSuite{T: t}

	// Setup database WITHOUT seeding installation settings
	suite.DBManager = testutil.NewDatabaseManager()
	suite.DBManager.SkipInstallationSeeding() // Skip seeding is_installed=true
	err := suite.DBManager.Setup()
	require.NoError(t, err, "Failed to setup test database")

	// Wait for database to be ready
	err = suite.DBManager.WaitForDatabase(30)
	require.NoError(t, err, "Database not ready")

	// Setup server WITHOUT seeding installation data
	suite.ServerManager = testutil.NewServerManager(func(cfg *config.Config) testutil.AppInterface {
		// Override config to mark as NOT installed
		cfg.IsInstalled = false

		// CRITICAL: Set to production environment to use SMTPMailer (not ConsoleMailer)
		// This replicates the real bug scenario
		cfg.Environment = "production"

		// CRITICAL: Empty SMTP config to replicate production bug scenario
		// In production, before setup, there's no SMTP config in database
		cfg.SMTP.FromEmail = ""        // Empty email will cause parsing error
		cfg.SMTP.FromName = "Notifuse" // Default name only
		cfg.SMTP.Host = "localhost"    // Minimal config to allow app to start
		cfg.SMTP.Port = 1025
		cfg.SMTP.Username = ""
		cfg.SMTP.Password = ""

		return app.NewApp(cfg)
	}, suite.DBManager)

	err = suite.ServerManager.Start()
	require.NoError(t, err, "Failed to start test server")

	// Setup API client
	suite.APIClient = testutil.NewAPIClient(suite.ServerManager.GetURL())

	// Setup data factory with repositories from the app
	appInstance := suite.ServerManager.GetApp()
	suite.DataFactory = testutil.NewTestDataFactory(
		suite.DBManager.GetDB(),
		appInstance.GetUserRepository(),
		appInstance.GetWorkspaceRepository(),
		appInstance.GetContactRepository(),
		appInstance.GetListRepository(),
		appInstance.GetTemplateRepository(),
		appInstance.GetBroadcastRepository(),
		appInstance.GetMessageHistoryRepository(),
		appInstance.GetContactListRepository(),
		appInstance.GetTransactionalNotificationRepository(),
	)

	// DO NOT seed test data - we want a clean slate for setup wizard testing

	suite.Config = suite.ServerManager.GetApp().GetConfig()

	return suite
}
