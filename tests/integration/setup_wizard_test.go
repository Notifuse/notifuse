package integration

import (
	"encoding/json"
	"net/http"
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

	t.Run("Generate PASETO Keys", func(t *testing.T) {
		// Initialize with generated keys
		initReq := map[string]interface{}{
			"root_email":           "admin@example.com",
			"api_endpoint":         suite.ServerManager.GetURL(),
			"generate_paseto_keys": true,
			"smtp_host":            "localhost",
			"smtp_port":            1025,
			"smtp_from_email":      "test@example.com",
			"smtp_from_name":       "Test Notifuse",
		}

		resp, err := client.Post("/api/setup.initialize", initReq)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var initResp map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&initResp)
		require.NoError(t, err)

		assert.True(t, initResp["success"].(bool), "Setup should succeed")
		assert.NotNil(t, initResp["paseto_keys"], "Generated keys should be returned")

		// Verify keys were returned
		keys := initResp["paseto_keys"].(map[string]interface{})
		assert.NotEmpty(t, keys["public_key"], "Public key should be generated")
		assert.NotEmpty(t, keys["private_key"], "Private key should be generated")
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
			"root_email":           "admin2@example.com",
			"api_endpoint":         suite.ServerManager.GetURL(),
			"generate_paseto_keys": true,
			"smtp_host":            "localhost",
			"smtp_port":            1025,
			"smtp_from_email":      "test@example.com",
			"smtp_from_name":       "Test Notifuse",
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

// TestSetupWizardWithProvidedKeys tests setup with user-provided PASETO keys
func TestSetupWizardWithProvidedKeys(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := createUninstalledTestSuite(t)
	defer suite.Cleanup()

	client := suite.APIClient

	t.Run("Initialize with Provided Keys", func(t *testing.T) {
		// Use hardcoded test keys
		privateKeyB64 := "UayDa4OMDpm3CvIT+iSC39iDyPlsui0pNQYDEZ1pbo1LsIrO4p/aVuCBWz6LiYvzj9pc+gn0gLwRd0CoHV+nxw=="
		publicKeyB64 := "S7CKzuKf2lbggVs+i4mL84/aXPoJ9IC8EXdAqB1fp8c="

		initReq := map[string]interface{}{
			"root_email":           "admin@example.com",
			"api_endpoint":         suite.ServerManager.GetURL(),
			"generate_paseto_keys": false,
			"paseto_private_key":   privateKeyB64,
			"paseto_public_key":    publicKeyB64,
			"smtp_host":            "localhost",
			"smtp_port":            1025,
			"smtp_from_email":      "test@example.com",
			"smtp_from_name":       "Test Notifuse",
		}

		resp, err := client.Post("/api/setup.initialize", initReq)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var initResp map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&initResp)
		require.NoError(t, err)

		assert.True(t, initResp["success"].(bool), "Setup should succeed")
		assert.Nil(t, initResp["paseto_keys"], "Keys should not be returned when provided by user")
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
				"generate_paseto_keys": true,
				"smtp_host":            "localhost",
				"smtp_port":            1025,
				"smtp_from_email":      "test@example.com",
			},
			expectError: true,
		},
		{
			name: "Missing SMTP Host",
			request: map[string]interface{}{
				"root_email":           "admin@example.com",
				"generate_paseto_keys": true,
				"smtp_port":            1025,
				"smtp_from_email":      "test@example.com",
			},
			expectError: true,
		},
		{
			name: "Missing SMTP From Email",
			request: map[string]interface{}{
				"root_email":           "admin@example.com",
				"generate_paseto_keys": true,
				"smtp_host":            "localhost",
				"smtp_port":            1025,
			},
			expectError: true,
		},
		{
			name: "Missing PASETO Keys When Not Generating",
			request: map[string]interface{}{
				"root_email":           "admin@example.com",
				"generate_paseto_keys": false,
				"smtp_host":            "localhost",
				"smtp_port":            1025,
				"smtp_from_email":      "test@example.com",
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
		// Test with valid MailHog settings (running in docker-compose)
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
			"root_email":           "admin@example.com",
			"api_endpoint":         suite.ServerManager.GetURL(),
			"generate_paseto_keys": true,
			"smtp_host":            "localhost",
			"smtp_port":            1025,
			"smtp_from_email":      "test@example.com",
			"smtp_from_name":       "Test Notifuse",
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

// TestSetupWizardSigninImmediatelyAfterCompletion tests that signin works immediately after setup
// This test verifies the bug fix where mailer wasn't being reinitialized after setup completion
func TestSetupWizardSigninImmediatelyAfterCompletion(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := createUninstalledTestSuite(t)
	defer suite.Cleanup()

	client := suite.APIClient

	t.Run("Complete Setup and Signin Without Restart", func(t *testing.T) {
		// Step 1: Complete setup wizard with full SMTP configuration
		rootEmail := "admin@example.com"
		initReq := map[string]interface{}{
			"root_email":           rootEmail,
			"api_endpoint":         suite.ServerManager.GetURL(),
			"generate_paseto_keys": true,
			"smtp_host":            "localhost",
			"smtp_port":            1025,
			"smtp_username":        "testuser",
			"smtp_password":        "testpass",
			"smtp_from_email":      "noreply@example.com", // Important: non-empty from email
			"smtp_from_name":       "Test System",
		}

		resp, err := client.Post("/api/setup.initialize", initReq)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode, "Setup should succeed")

		var setupResp map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&setupResp)
		require.NoError(t, err)

		assert.True(t, setupResp["success"].(bool), "Setup should succeed")

		// Step 2: Immediately attempt signin (WITHOUT restarting the service)
		// This is where the bug manifests - mailer still has empty FromEmail
		signinReq := map[string]interface{}{
			"email": rootEmail,
		}

		signinResp, err := client.Post("/api/user.signin", signinReq)
		require.NoError(t, err)
		defer signinResp.Body.Close()

		// Read response body for debugging
		var signinResult map[string]interface{}
		err = json.NewDecoder(signinResp.Body).Decode(&signinResult)
		require.NoError(t, err)

		// Step 3: Verify no mail parsing error
		// Bug symptom: "failed to parse mail address \"Notifuse\" <>"
		if errorMsg, ok := signinResult["error"].(string); ok {
			assert.NotContains(t, errorMsg, "failed to parse mail address",
				"Should not have mail address parsing error after setup")
			assert.NotContains(t, errorMsg, "mail: invalid string",
				"Should not have invalid mail string error after setup")
			assert.NotContains(t, errorMsg, "failed to set email from address",
				"Should not have email from address error after setup")
		}

		// Step 4: Verify signin succeeded or properly failed (not with mail error)
		// In development mode, magic code is returned directly
		// In production mode, we just verify no mail errors occurred
		if signinResp.StatusCode == http.StatusOK {
			// Success - magic code sent or returned
			t.Log("Signin succeeded - mailer was properly reinitialized")
		} else {
			// If it failed, make sure it's not due to mail configuration
			t.Logf("Signin status: %d, response: %v", signinResp.StatusCode, signinResult)
			
			// Even if signin failed for other reasons (user not found, etc.),
			// it should NOT be due to mail parsing errors
			if errorMsg, ok := signinResult["error"].(string); ok {
				require.NotContains(t, errorMsg, "parse mail address",
					"Signin should not fail due to mail address parsing errors")
			}
		}
	})

	t.Run("Verify Mailer Config Updated After Setup", func(t *testing.T) {
		// Additional verification: Check that subsequent mail operations work
		// by attempting another signin operation
		signinReq := map[string]interface{}{
			"email": "admin@example.com",
		}

		resp, err := client.Post("/api/user.signin", signinReq)
		require.NoError(t, err)
		defer resp.Body.Close()

		var result map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&result)
		require.NoError(t, err)

		// Verify no mail-related errors
		if errorMsg, ok := result["error"].(string); ok {
			assert.NotContains(t, errorMsg, "failed to parse mail address",
				"Subsequent signin should also work without mail errors")
			assert.NotContains(t, errorMsg, "failed to set email from address",
				"Subsequent signin should also work without mail errors")
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
		cfg.Security.PasetoPrivateKeyBytes = nil
		cfg.Security.PasetoPublicKeyBytes = nil
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
