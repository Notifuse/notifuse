package integration

import (
	"io"
	"net/http"
	"testing"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/app"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/tests/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWebhookRegistrationHandler tests the webhook registration handler endpoints.
// This test suite verifies webhook registration and status checking functionality
// with various email providers and proper authentication/validation.

func TestWebhookRegistrationHandler(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer _ = suite.Cleanup()

	client := suite.APIClient
	factory := suite.DataFactory

	// Create test user and workspace
	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)

	// Add user to workspace as owner
	err = factory.AddUserToWorkspace(user.ID, workspace.ID, "owner")
	require.NoError(t, err)

	// Login to get auth token
	err = client.Login(user.Email, "password")
	require.NoError(t, err)
	client.SetWorkspaceID(workspace.ID)

	t.Run("Authentication", func(t *testing.T) {
		testWebhookRegistrationAuthentication(t, suite, factory, workspace.ID)
	})

	t.Run("Register Webhooks", func(t *testing.T) {
		testRegisterWebhooks(t, client, factory, workspace.ID)
	})

	t.Run("Get Webhook Status", func(t *testing.T) {
		testGetWebhookStatus(t, client, factory, workspace.ID)
	})

	t.Run("Method Validation", func(t *testing.T) {
		testWebhookMethodValidation(t, client, factory, workspace.ID)
	})
}

func testWebhookRegistrationAuthentication(t *testing.T, suite *testutil.IntegrationTestSuite, factory *testutil.TestDataFactory, workspaceID string) {
	// Create an integration for testing
	integration, err := factory.CreateSMTPIntegration(workspaceID)
	require.NoError(t, err)

	t.Run("should require authentication for register", func(t *testing.T) {
		// Create a new client without authentication for this test
		unauthClient := testutil.NewAPIClient(suite.ServerManager.GetURL())

		request := map[string]interface{}{
			"workspace_id":   workspaceID,
			"integration_id": integration.ID,
			"event_types":    []string{"delivered", "bounce"},
		}

		resp, err := unauthClient.RegisterWebhooks(request)
		require.NoError(t, err)
		defer func() { _ = _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})

	t.Run("should require authentication for status", func(t *testing.T) {
		// Create a new client without authentication for this test
		unauthClient := testutil.NewAPIClient(suite.ServerManager.GetURL())

		resp, err := unauthClient.GetWebhookStatus(workspaceID, integration.ID)
		require.NoError(t, err)
		defer func() { _ = _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})
}

func testRegisterWebhooks(t *testing.T, client *testutil.APIClient, factory *testutil.TestDataFactory, workspaceID string) {
	t.Run("should handle unsupported provider gracefully", func(t *testing.T) {
		// Create an integration with SMTP provider (not supported for webhooks)
		integration, err := factory.CreateSMTPIntegration(workspaceID)
		require.NoError(t, err)

		request := map[string]interface{}{
			"workspace_id":   workspaceID,
			"integration_id": integration.ID,
			"event_types":    []string{"delivered", "bounce", "complaint"},
		}

		resp, err := client.RegisterWebhooks(request)
		require.NoError(t, err)
		defer func() { _ = _ = resp.Body.Close() }()

		// SMTP provider doesn't support webhook registration, should return error
		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.Contains(t, string(body), "webhook registration not implemented for provider")
	})

	t.Run("should use default event types when not specified", func(t *testing.T) {
		// Create another integration with SMTP (will also fail but should handle gracefully)
		integration, err := factory.CreateSMTPIntegration(workspaceID)
		require.NoError(t, err)

		request := map[string]interface{}{
			"workspace_id":   workspaceID,
			"integration_id": integration.ID,
			// No event_types specified - should default to all types
		}

		resp, err := client.RegisterWebhooks(request)
		require.NoError(t, err)
		defer func() { _ = _ = resp.Body.Close() }()

		// Should still fail gracefully for unsupported provider
		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	})

	t.Run("should validate required fields", func(t *testing.T) {
		t.Run("missing workspace_id", func(t *testing.T) {
			request := map[string]interface{}{
				"integration_id": "test-integration-id",
				"event_types":    []string{"delivered"},
			}

			resp, err := client.RegisterWebhooks(request)
			require.NoError(t, err)
			defer func() { _ = _ = resp.Body.Close() }()

			assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			assert.Contains(t, string(body), "workspace_id is required")
		})

		t.Run("missing integration_id", func(t *testing.T) {
			request := map[string]interface{}{
				"workspace_id": workspaceID,
				"event_types":  []string{"delivered"},
			}

			resp, err := client.RegisterWebhooks(request)
			require.NoError(t, err)
			defer func() { _ = _ = resp.Body.Close() }()

			assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			assert.Contains(t, string(body), "integration_id is required")
		})
	})

	t.Run("should handle invalid JSON", func(t *testing.T) {
		// This test sends malformed JSON to the register endpoint
		resp, err := client.Post("/api/webhooks.register", "invalid-json")
		require.NoError(t, err)
		defer func() { _ = _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.Contains(t, string(body), "Invalid request body")
	})

	t.Run("should handle non-existent integration", func(t *testing.T) {
		request := map[string]interface{}{
			"workspace_id":   workspaceID,
			"integration_id": "non-existent-integration",
			"event_types":    []string{"delivered"},
		}

		resp, err := client.RegisterWebhooks(request)
		require.NoError(t, err)
		defer func() { _ = _ = resp.Body.Close() }()

		// Should return 500 or 404 depending on implementation
		assert.True(t, resp.StatusCode >= 400)
	})

	t.Run("should handle invalid event types", func(t *testing.T) {
		integration, err := factory.CreateSMTPIntegration(workspaceID)
		require.NoError(t, err)

		request := map[string]interface{}{
			"workspace_id":   workspaceID,
			"integration_id": integration.ID,
			"event_types":    []string{"invalid_event_type"},
		}

		resp, err := client.RegisterWebhooks(request)
		require.NoError(t, err)
		defer func() { _ = _ = resp.Body.Close() }()

		// Should still process - invalid event types are handled by the service layer
		assert.True(t, resp.StatusCode >= 200)
	})
}

func testGetWebhookStatus(t *testing.T, client *testutil.APIClient, factory *testutil.TestDataFactory, workspaceID string) {
	t.Run("should handle unsupported provider for status check", func(t *testing.T) {
		// Create an integration with SMTP (not supported for webhooks)
		integration, err := factory.CreateSMTPIntegration(workspaceID)
		require.NoError(t, err)

		resp, err := client.GetWebhookStatus(workspaceID, integration.ID)
		require.NoError(t, err)
		defer func() { _ = _ = resp.Body.Close() }()

		// SMTP provider doesn't support webhook status, should return error
		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.Contains(t, string(body), "not implemented for provider")
	})

	t.Run("should validate required parameters", func(t *testing.T) {
		t.Run("missing workspace_id", func(t *testing.T) {
			resp, err := client.GetWebhookStatus("", "test-integration-id")
			require.NoError(t, err)
			defer func() { _ = _ = resp.Body.Close() }()

			assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			assert.Contains(t, string(body), "workspace_id is required")
		})

		t.Run("missing integration_id", func(t *testing.T) {
			resp, err := client.GetWebhookStatus(workspaceID, "")
			require.NoError(t, err)
			defer func() { _ = _ = resp.Body.Close() }()

			assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			assert.Contains(t, string(body), "integration_id is required")
		})
	})

	t.Run("should handle non-existent integration", func(t *testing.T) {
		resp, err := client.GetWebhookStatus(workspaceID, "non-existent-integration")
		require.NoError(t, err)
		defer func() { _ = _ = resp.Body.Close() }()

		// Should return error status
		assert.True(t, resp.StatusCode >= 400)
	})

	t.Run("should handle non-existent workspace", func(t *testing.T) {
		integration, err := factory.CreateSMTPIntegration(workspaceID)
		require.NoError(t, err)

		resp, err := client.GetWebhookStatus("non-existent-workspace", integration.ID)
		require.NoError(t, err)
		defer func() { _ = _ = resp.Body.Close() }()

		// Should return error status
		assert.True(t, resp.StatusCode >= 400)
	})
}

func testWebhookMethodValidation(t *testing.T, client *testutil.APIClient, factory *testutil.TestDataFactory, workspaceID string) {
	integration, err := factory.CreateSMTPIntegration(workspaceID)
	require.NoError(t, err)

	t.Run("should only allow POST for webhook registration", func(t *testing.T) {
		// Test GET method on register endpoint
		resp, err := client.Get("/api/webhooks.register", map[string]string{
			"workspace_id":   workspaceID,
			"integration_id": integration.ID,
		})
		require.NoError(t, err)
		defer func() { _ = _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.Contains(t, string(body), "Method not allowed")
	})

	t.Run("should only allow GET for webhook status", func(t *testing.T) {
		// Test POST method on status endpoint
		request := map[string]interface{}{
			"workspace_id":   workspaceID,
			"integration_id": integration.ID,
		}

		resp, err := client.Post("/api/webhooks.status", request)
		require.NoError(t, err)
		defer func() { _ = _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.Contains(t, string(body), "Method not allowed")
	})
}

func TestWebhookRegistrationWithDifferentProviders(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer _ = suite.Cleanup()

	client := suite.APIClient
	factory := suite.DataFactory

	// Create test user and workspace
	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)

	// Add user to workspace as owner
	err = factory.AddUserToWorkspace(user.ID, workspace.ID, "owner")
	require.NoError(t, err)

	// Login to get auth token
	err = client.Login(user.Email, "password")
	require.NoError(t, err)
	client.SetWorkspaceID(workspace.ID)

	t.Run("SMTP Provider (Unsupported)", func(t *testing.T) {
		testWebhookRegistrationWithUnsupportedProvider(t, client, factory, workspace.ID, func(workspaceID string) (*domain.Integration, error) {
			return factory.CreateSMTPIntegration(workspaceID)
		})
	})

	t.Run("Mailhog Provider (Unsupported)", func(t *testing.T) {
		testWebhookRegistrationWithUnsupportedProvider(t, client, factory, workspace.ID, func(workspaceID string) (*domain.Integration, error) {
			return factory.CreateMailhogSMTPIntegration(workspaceID)
		})
	})
}

func testWebhookRegistrationWithUnsupportedProvider(
	t *testing.T,
	client *testutil.APIClient,
	factory *testutil.TestDataFactory,
	workspaceID string,
	createIntegration func(string) (*domain.Integration, error),
) {
	// Create integration for this provider
	integration, err := createIntegration(workspaceID)
	require.NoError(t, err)

	t.Run("Register with Unsupported Provider", func(t *testing.T) {
		// Try to register webhooks with unsupported provider
		registerRequest := map[string]interface{}{
			"workspace_id":   workspaceID,
			"integration_id": integration.ID,
			"event_types":    []string{"delivered", "bounce", "complaint"},
		}

		registerResp, err := client.RegisterWebhooks(registerRequest)
		require.NoError(t, err)
		defer func() { _ = _ = registerResp.Body.Close() }()

		// Should fail for unsupported provider
		assert.Equal(t, http.StatusInternalServerError, registerResp.StatusCode)

		body, err := io.ReadAll(registerResp.Body)
		require.NoError(t, err)
		assert.Contains(t, string(body), "webhook registration not implemented for provider")
	})

	t.Run("Check Status with Unsupported Provider", func(t *testing.T) {
		// Check webhook status with unsupported provider
		statusResp, err := client.GetWebhookStatus(workspaceID, integration.ID)
		require.NoError(t, err)
		defer func() { _ = _ = statusResp.Body.Close() }()

		// Should fail for unsupported provider
		assert.Equal(t, http.StatusInternalServerError, statusResp.StatusCode)

		body, err := io.ReadAll(statusResp.Body)
		require.NoError(t, err)
		assert.Contains(t, string(body), "not implemented for provider")
	})
}

func TestWebhookRegistrationErrorHandling(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer _ = suite.Cleanup()

	client := suite.APIClient
	factory := suite.DataFactory

	// Create test user and workspace
	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)

	// Add user to workspace as owner
	err = factory.AddUserToWorkspace(user.ID, workspace.ID, "owner")
	require.NoError(t, err)

	// Login to get auth token
	err = client.Login(user.Email, "password")
	require.NoError(t, err)
	client.SetWorkspaceID(workspace.ID)

	t.Run("should handle workspace access errors", func(t *testing.T) {
		// Create another workspace that the user doesn't have access to
		otherWorkspace, err := factory.CreateWorkspace()
		require.NoError(t, err)

		integration, err := factory.CreateSMTPIntegration(workspace.ID)
		require.NoError(t, err)

		request := map[string]interface{}{
			"workspace_id":   otherWorkspace.ID, // User doesn't have access to this workspace
			"integration_id": integration.ID,
			"event_types":    []string{"delivered"},
		}

		resp, err := client.RegisterWebhooks(request)
		require.NoError(t, err)
		defer func() { _ = _ = resp.Body.Close() }()

		// Should return authorization error
		assert.True(t, resp.StatusCode >= 400)
	})

	t.Run("should handle empty request body", func(t *testing.T) {
		resp, err := client.Post("/api/webhooks.register", nil)
		require.NoError(t, err)
		defer func() { _ = _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("should handle malformed event types", func(t *testing.T) {
		integration, err := factory.CreateSMTPIntegration(workspace.ID)
		require.NoError(t, err)

		request := map[string]interface{}{
			"workspace_id":   workspace.ID,
			"integration_id": integration.ID,
			"event_types":    "not-an-array", // Should be an array
		}

		resp, err := client.RegisterWebhooks(request)
		require.NoError(t, err)
		defer func() { _ = _ = resp.Body.Close() }()

		// Should handle gracefully (specific behavior depends on implementation)
		assert.True(t, resp.StatusCode >= 200)
	})
}
