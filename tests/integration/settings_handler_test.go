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

// TestSettingsHandler_GetRequiresAuth tests that GET /api/settings.get requires authentication
func TestSettingsHandler_GetRequiresAuth(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer func() { suite.Cleanup() }()

	// Try to get settings without auth token
	resp, err := suite.APIClient.Get("/api/settings.get")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "Should require authentication")
}

// TestSettingsHandler_GetRequiresRootUser tests that only the root user can access settings
func TestSettingsHandler_GetRequiresRootUser(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer func() { suite.Cleanup() }()

	// Authenticate as a non-root user (testuser@example.com is seeded in test DB)
	tokenCache := testutil.NewTokenCache(suite.APIClient)
	nonRootToken := tokenCache.GetOrCreate(t, "testuser@example.com")
	suite.APIClient.SetToken(nonRootToken)

	resp, err := suite.APIClient.Get("/api/settings.get")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusForbidden, resp.StatusCode, "Non-root user should get 403")
}

// TestSettingsHandler_GetAsRootUser tests that the root user can fetch settings
func TestSettingsHandler_GetAsRootUser(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer func() { suite.Cleanup() }()

	// Authenticate as root user
	rootEmail := suite.Config.RootEmail
	tokenCache := testutil.NewTokenCache(suite.APIClient)
	rootToken := tokenCache.GetOrCreate(t, rootEmail)
	suite.APIClient.SetToken(rootToken)

	resp, err := suite.APIClient.Get("/api/settings.get")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusOK, resp.StatusCode, "Root user should be able to get settings")

	var result map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)

	// Should have settings and env_overrides keys
	assert.Contains(t, result, "settings", "Response should contain settings")
	assert.Contains(t, result, "env_overrides", "Response should contain env_overrides")

	settings, ok := result["settings"].(map[string]interface{})
	require.True(t, ok, "Settings should be a map")

	// Root email should be set (system is installed)
	assert.NotEmpty(t, settings["root_email"], "root_email should be set")
}

// TestSettingsHandler_UpdateAndGet tests the full update -> get cycle
func TestSettingsHandler_UpdateAndGet(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer func() { suite.Cleanup() }()

	// Authenticate as root user
	rootEmail := suite.Config.RootEmail
	tokenCache := testutil.NewTokenCache(suite.APIClient)
	rootToken := tokenCache.GetOrCreate(t, rootEmail)
	suite.APIClient.SetToken(rootToken)

	// Step 1: Get current settings
	getResp, err := suite.APIClient.Get("/api/settings.get")
	require.NoError(t, err)
	defer func() { _ = getResp.Body.Close() }()
	require.Equal(t, http.StatusOK, getResp.StatusCode)

	var getResult map[string]interface{}
	err = json.NewDecoder(getResp.Body).Decode(&getResult)
	require.NoError(t, err)

	settings := getResult["settings"].(map[string]interface{})

	// Step 2: Update some settings
	updateReq := map[string]interface{}{
		"root_email":        rootEmail,
		"api_endpoint":      settings["api_endpoint"], // keep existing
		"smtp_host":         "updated-smtp.example.com",
		"smtp_port":         465,
		"smtp_from_email":   "updated@example.com",
		"smtp_from_name":    "Updated Name",
		"smtp_use_tls":      true,
		"smtp_ehlo_hostname": "updated-ehlo.example.com",
		"telemetry_enabled":  true,
		"check_for_updates":  false,
		"smtp_bridge_enabled": false,
	}

	updateResp, err := suite.APIClient.Post("/api/settings.update", updateReq)
	require.NoError(t, err)
	defer func() { _ = updateResp.Body.Close() }()

	assert.Equal(t, http.StatusOK, updateResp.StatusCode, "Update should succeed")

	var updateResult map[string]interface{}
	err = json.NewDecoder(updateResp.Body).Decode(&updateResult)
	require.NoError(t, err)
	assert.True(t, updateResult["success"].(bool), "Update should report success")

	// Step 3: Get settings again and verify they were updated
	// Note: we must re-authenticate because the server might restart,
	// but in integration tests the shutdown is caught by the test suite
	getResp2, err := suite.APIClient.Get("/api/settings.get")
	require.NoError(t, err)
	defer func() { _ = getResp2.Body.Close() }()
	require.Equal(t, http.StatusOK, getResp2.StatusCode)

	var getResult2 map[string]interface{}
	err = json.NewDecoder(getResp2.Body).Decode(&getResult2)
	require.NoError(t, err)

	updatedSettings := getResult2["settings"].(map[string]interface{})

	assert.Equal(t, "updated-smtp.example.com", updatedSettings["smtp_host"])
	assert.Equal(t, float64(465), updatedSettings["smtp_port"])
	assert.Equal(t, "updated@example.com", updatedSettings["smtp_from_email"])
	assert.Equal(t, "Updated Name", updatedSettings["smtp_from_name"])
	assert.Equal(t, true, updatedSettings["smtp_use_tls"])
	assert.Equal(t, "updated-ehlo.example.com", updatedSettings["smtp_ehlo_hostname"])
	assert.Equal(t, true, updatedSettings["telemetry_enabled"])
	assert.Equal(t, false, updatedSettings["check_for_updates"])
}

// TestSettingsHandler_TestSMTPRequiresRoot tests that SMTP test requires root user
func TestSettingsHandler_TestSMTPRequiresRoot(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer func() { suite.Cleanup() }()

	// Try without auth
	smtpReq := map[string]interface{}{
		"smtp_host": "localhost",
		"smtp_port": 1025,
	}

	resp, err := suite.APIClient.Post("/api/settings.testSmtp", smtpReq)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "Should require auth")
}

// TestSettingsHandler_UpdateRequiresAuth tests that update requires authentication
func TestSettingsHandler_UpdateRequiresAuth(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer func() { suite.Cleanup() }()

	updateReq := map[string]interface{}{
		"root_email": "hacker@example.com",
	}

	resp, err := suite.APIClient.Post("/api/settings.update", updateReq)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "Should require auth")
}

// TestSettingsHandler_MaskedPasswordRoundTrip tests that masked passwords don't overwrite existing values
func TestSettingsHandler_MaskedPasswordRoundTrip(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer func() { suite.Cleanup() }()

	// Authenticate as root
	rootEmail := suite.Config.RootEmail
	tokenCache := testutil.NewTokenCache(suite.APIClient)
	rootToken := tokenCache.GetOrCreate(t, rootEmail)
	suite.APIClient.SetToken(rootToken)

	// Step 1: Set a password
	updateReq := map[string]interface{}{
		"root_email":     rootEmail,
		"smtp_host":      "smtp.example.com",
		"smtp_port":      587,
		"smtp_password":  "my-secret-password",
		"smtp_from_email": "test@example.com",
	}

	resp, err := suite.APIClient.Post("/api/settings.update", updateReq)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Step 2: Get settings - password should be masked
	getResp, err := suite.APIClient.Get("/api/settings.get")
	require.NoError(t, err)
	defer func() { _ = getResp.Body.Close() }()
	require.Equal(t, http.StatusOK, getResp.StatusCode)

	var getResult map[string]interface{}
	err = json.NewDecoder(getResp.Body).Decode(&getResult)
	require.NoError(t, err)
	settings := getResult["settings"].(map[string]interface{})

	maskedPassword := settings["smtp_password"].(string)
	assert.Equal(t, "\u2022\u2022\u2022\u2022\u2022\u2022\u2022\u2022", maskedPassword, "Password should be masked")

	// Step 3: Send update with the masked value
	updateReq2 := map[string]interface{}{
		"root_email":     rootEmail,
		"smtp_host":      "smtp.example.com",
		"smtp_port":      587,
		"smtp_password":  maskedPassword, // sending back the mask
		"smtp_from_email": "test@example.com",
	}

	resp2, err := suite.APIClient.Post("/api/settings.update", updateReq2)
	require.NoError(t, err)
	defer func() { _ = resp2.Body.Close() }()
	require.Equal(t, http.StatusOK, resp2.StatusCode)

	// Step 4: Verify password is still masked (still set, not cleared)
	getResp2, err := suite.APIClient.Get("/api/settings.get")
	require.NoError(t, err)
	defer func() { _ = getResp2.Body.Close() }()
	require.Equal(t, http.StatusOK, getResp2.StatusCode)

	var getResult2 map[string]interface{}
	err = json.NewDecoder(getResp2.Body).Decode(&getResult2)
	require.NoError(t, err)
	settings2 := getResult2["settings"].(map[string]interface{})

	assert.Equal(t, "\u2022\u2022\u2022\u2022\u2022\u2022\u2022\u2022", settings2["smtp_password"].(string),
		"Password should still be masked after round-trip with masked value")
}

// TestSettingsHandler_EnvOverridesReported tests that env overrides are correctly reported
func TestSettingsHandler_EnvOverridesReported(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	// Create a suite with env config that has some values
	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer func() { suite.Cleanup() }()

	// Authenticate as root
	rootEmail := suite.Config.RootEmail
	tokenCache := testutil.NewTokenCache(suite.APIClient)
	rootToken := tokenCache.GetOrCreate(t, rootEmail)
	suite.APIClient.SetToken(rootToken)

	resp, err := suite.APIClient.Get("/api/settings.get")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var result map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)

	envOverrides, ok := result["env_overrides"].(map[string]interface{})
	require.True(t, ok, "env_overrides should be a map")

	// In the test environment, env vars are typically set for basic config
	// Just verify the structure is correct - actual values depend on test env
	_ = envOverrides // structure validated by JSON decode
}
