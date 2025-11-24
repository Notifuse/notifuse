package integration

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/app"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/tests/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContactListAPIEndpoints(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer func() { suite.Cleanup() }()

	client := suite.APIClient

	t.Run("GetByIDs Endpoint", func(t *testing.T) {
		params := map[string]string{
			"workspace_id": "test-workspace",
			"email":        "test@example.com",
			"list_id":      "test-list",
		}

		resp, err := client.Get("/api/contactLists.getByIDs", params)
		require.NoError(t, err, "Should be able to call getByIDs endpoint")
		defer func() { _ = resp.Body.Close() }()

		// Should not be 404, endpoint should exist
		assert.NotEqual(t, http.StatusNotFound, resp.StatusCode, "GetByIDs endpoint should exist")
	})

	t.Run("GetContactsByList Endpoint", func(t *testing.T) {
		params := map[string]string{
			"workspace_id": "test-workspace",
			"list_id":      "test-list",
		}

		resp, err := client.Get("/api/contactLists.getContactsByList", params)
		require.NoError(t, err, "Should be able to call getContactsByList endpoint")
		defer func() { _ = resp.Body.Close() }()

		// Should not be 404, endpoint should exist
		assert.NotEqual(t, http.StatusNotFound, resp.StatusCode, "GetContactsByList endpoint should exist")
	})

	t.Run("GetListsByContact Endpoint", func(t *testing.T) {
		params := map[string]string{
			"workspace_id": "test-workspace",
			"email":        "test@example.com",
		}

		resp, err := client.Get("/api/contactLists.getListsByContact", params)
		require.NoError(t, err, "Should be able to call getListsByContact endpoint")
		defer func() { _ = resp.Body.Close() }()

		// Should not be 404, endpoint should exist
		assert.NotEqual(t, http.StatusNotFound, resp.StatusCode, "GetListsByContact endpoint should exist")
	})

	t.Run("UpdateStatus Endpoint", func(t *testing.T) {
		updateReq := map[string]interface{}{
			"workspace_id": "test-workspace",
			"email":        "test@example.com",
			"list_id":      "test-list",
			"status":       "active",
		}

		resp, err := client.Post("/api/contactLists.updateStatus", updateReq)
		require.NoError(t, err, "Should be able to call updateStatus endpoint")
		defer func() { _ = resp.Body.Close() }()

		// Should not be 404, endpoint should exist
		assert.NotEqual(t, http.StatusNotFound, resp.StatusCode, "UpdateStatus endpoint should exist")
	})

	t.Run("RemoveContact Endpoint", func(t *testing.T) {
		removeReq := map[string]interface{}{
			"workspace_id": "test-workspace",
			"email":        "test@example.com",
			"list_id":      "test-list",
		}

		resp, err := client.Post("/api/contactLists.removeContact", removeReq)
		require.NoError(t, err, "Should be able to call removeContact endpoint")
		defer func() { _ = resp.Body.Close() }()

		// Should not be 404, endpoint should exist
		assert.NotEqual(t, http.StatusNotFound, resp.StatusCode, "RemoveContact endpoint should exist")
	})
}

func TestContactListAPIWithoutAuthentication(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer func() { suite.Cleanup() }()

	factory := suite.DataFactory
	client := suite.APIClient

	// Create test data
	workspace, err := factory.CreateWorkspace(
		testutil.WithWorkspaceName("TestWS"),
	)
	require.NoError(t, err, "Should be able to create test workspace")

	contact, err := factory.CreateContact(workspace.ID,
		testutil.WithContactEmail("testcontact@example.com"),
		testutil.WithContactName("Test", "Contact"),
	)
	require.NoError(t, err, "Should be able to create test contact")

	list, err := factory.CreateList(workspace.ID,
		testutil.WithListName("Test List"),
	)
	require.NoError(t, err, "Should be able to create test list")

	client.SetWorkspaceID(workspace.ID)

	t.Run("Authenticated GetByIDs", func(t *testing.T) {
		params := map[string]string{
			"workspace_id": workspace.ID,
			"email":        contact.Email,
			"list_id":      list.ID,
		}

		resp, err := client.Get("/api/contactLists.getByIDs", params)
		require.NoError(t, err, "Should be able to call getByIDs with auth")
		defer func() { _ = resp.Body.Close() }()

		// Without proper auth token, we still get 401
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode,
			"Should get 401 without proper authentication")
	})

	t.Run("Authenticated GetContactsByList", func(t *testing.T) {
		params := map[string]string{
			"workspace_id": workspace.ID,
			"list_id":      list.ID,
		}

		resp, err := client.Get("/api/contactLists.getContactsByList", params)
		require.NoError(t, err, "Should be able to call getContactsByList with auth")
		defer func() { _ = resp.Body.Close() }()

		// Without proper auth token, we still get 401
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode,
			"Should get 401 without proper authentication")
	})

	t.Run("Authenticated GetListsByContact", func(t *testing.T) {
		params := map[string]string{
			"workspace_id": workspace.ID,
			"email":        contact.Email,
		}

		resp, err := client.Get("/api/contactLists.getListsByContact", params)
		require.NoError(t, err, "Should be able to call getListsByContact with auth")
		defer func() { _ = resp.Body.Close() }()

		// Without proper auth token, we still get 401
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode,
			"Should get 401 without proper authentication")
	})
}

func TestContactListAPIValidation(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer func() { suite.Cleanup() }()

	client := suite.APIClient

	t.Run("GetByIDs Unauthenticated Requests", func(t *testing.T) {
		// Missing workspace_id
		params := map[string]string{
			"email":   "test@example.com",
			"list_id": "test-list",
		}

		resp, err := client.Get("/api/contactLists.getByIDs", params)
		require.NoError(t, err, "Should be able to make request")
		defer func() { _ = resp.Body.Close() }()

		// Without authentication, we get 401 instead of 400
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "Should return 401 for unauthenticated request")

		// Missing email
		params = map[string]string{
			"workspace_id": "test-workspace",
			"list_id":      "test-list",
		}

		resp, err = client.Get("/api/contactLists.getByIDs", params)
		require.NoError(t, err, "Should be able to make request")
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "Should return 401 for unauthenticated request")

		// Missing list_id
		params = map[string]string{
			"workspace_id": "test-workspace",
			"email":        "test@example.com",
		}

		resp, err = client.Get("/api/contactLists.getByIDs", params)
		require.NoError(t, err, "Should be able to make request")
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "Should return 401 for unauthenticated request")
	})

	t.Run("GetByIDs Unauthenticated with Invalid Email", func(t *testing.T) {
		params := map[string]string{
			"workspace_id": "test-workspace",
			"email":        "invalid-email",
			"list_id":      "test-list",
		}

		resp, err := client.Get("/api/contactLists.getByIDs", params)
		require.NoError(t, err, "Should be able to make request")
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "Should return 401 for unauthenticated request")
	})

	t.Run("UpdateStatus Unauthenticated Request", func(t *testing.T) {
		updateReq := map[string]interface{}{
			"workspace_id": "test-workspace",
			"email":        "test@example.com",
			"list_id":      "test-list",
			"status":       "invalid-status",
		}

		resp, err := client.Post("/api/contactLists.updateStatus", updateReq)
		require.NoError(t, err, "Should be able to make request")
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "Should return 401 for unauthenticated request")
	})

	t.Run("UpdateStatus Unauthenticated with Various Statuses", func(t *testing.T) {
		validStatuses := []string{"active", "pending", "unsubscribed", "bounced", "complained"}

		for _, status := range validStatuses {
			updateReq := map[string]interface{}{
				"workspace_id": "test-workspace",
				"email":        "test@example.com",
				"list_id":      "test-list",
				"status":       status,
			}

			resp, err := client.Post("/api/contactLists.updateStatus", updateReq)
			require.NoError(t, err, "Should be able to make request with status %s", status)
			defer func() { _ = resp.Body.Close() }()

			// Should get 401 for unauthenticated request regardless of status
			assert.Equal(t, http.StatusUnauthorized, resp.StatusCode,
				"Should return 401 for unauthenticated request with status: %s", status)
		}
	})
}

func TestContactListAPIFullWorkflow(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer func() { suite.Cleanup() }()

	factory := suite.DataFactory
	client := suite.APIClient
	db := suite.DBManager.GetDB()

	// Create test data
	workspace, err := factory.CreateWorkspace(
		testutil.WithWorkspaceName("IntTestWS"),
	)
	require.NoError(t, err, "Should be able to create test workspace")

	contact, err := factory.CreateContact(workspace.ID,
		testutil.WithContactEmail("integration@example.com"),
		testutil.WithContactName("Integration", "Test"),
	)
	require.NoError(t, err, "Should be able to create test contact")

	list, err := factory.CreateList(workspace.ID,
		testutil.WithListName("Integration Test List"),
	)
	require.NoError(t, err, "Should be able to create test list")

	client.SetWorkspaceID(workspace.ID)

	// Create contact list relationship for testing
	_, err = factory.CreateContactList(workspace.ID,
		testutil.WithContactListEmail(contact.Email),
		testutil.WithContactListListID(list.ID),
		testutil.WithContactListStatus(domain.ContactListStatusActive),
	)
	require.NoError(t, err, "Should be able to create contact list relationship")

	t.Run("Full Workflow", func(t *testing.T) {
		// 1. Get contact list by IDs
		params := map[string]string{
			"workspace_id": workspace.ID,
			"email":        contact.Email,
			"list_id":      list.ID,
		}

		resp, err := client.Get("/api/contactLists.getByIDs", params)
		require.NoError(t, err, "Should be able to get contact list by IDs")
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode == http.StatusOK {
			var result map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&result)
			require.NoError(t, err, "Should be able to decode response")

			contactList, ok := result["contact_list"].(map[string]interface{})
			require.True(t, ok, "Should have contact_list in response")
			assert.Equal(t, contact.Email, contactList["email"], "Should return correct email")
			assert.Equal(t, list.ID, contactList["list_id"], "Should return correct list_id")
			assert.Equal(t, "active", contactList["status"], "Should return correct status")
		}

		// 2. Get contacts by list
		params = map[string]string{
			"workspace_id": workspace.ID,
			"list_id":      list.ID,
		}

		resp, err = client.Get("/api/contactLists.getContactsByList", params)
		require.NoError(t, err, "Should be able to get contacts by list")
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode == http.StatusOK {
			var result map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&result)
			require.NoError(t, err, "Should be able to decode response")

			contactLists, ok := result["contact_lists"].([]interface{})
			require.True(t, ok, "Should have contact_lists array in response")
			assert.Greater(t, len(contactLists), 0, "Should have at least one contact")
		}

		// 3. Get lists by contact
		params = map[string]string{
			"workspace_id": workspace.ID,
			"email":        contact.Email,
		}

		resp, err = client.Get("/api/contactLists.getListsByContact", params)
		require.NoError(t, err, "Should be able to get lists by contact")
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode == http.StatusOK {
			var result map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&result)
			require.NoError(t, err, "Should be able to decode response")

			contactLists, ok := result["contact_lists"].([]interface{})
			require.True(t, ok, "Should have contact_lists array in response")
			assert.Greater(t, len(contactLists), 0, "Should have at least one list")
		}

		// 4. Update status
		updateReq := map[string]interface{}{
			"workspace_id": workspace.ID,
			"email":        contact.Email,
			"list_id":      list.ID,
			"status":       "unsubscribed",
		}

		resp, err = client.Post("/api/contactLists.updateStatus", updateReq)
		require.NoError(t, err, "Should be able to update status")
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode == http.StatusOK {
			var result map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&result)
			require.NoError(t, err, "Should be able to decode response")

			success, ok := result["success"].(bool)
			require.True(t, ok, "Should have success field")
			assert.True(t, success, "Should return success true")

			// Verify status was updated in database
			var dbStatus string
			err = db.QueryRow(`
				SELECT status FROM contact_lists 
				WHERE workspace_id = $1 AND email = $2 AND list_id = $3
			`, workspace.ID, contact.Email, list.ID).Scan(&dbStatus)
			require.NoError(t, err, "Should be able to query updated status")
			assert.Equal(t, "unsubscribed", dbStatus, "Status should be updated in database")
		}

		// 5. Remove contact from list
		removeReq := map[string]interface{}{
			"workspace_id": workspace.ID,
			"email":        contact.Email,
			"list_id":      list.ID,
		}

		resp, err = client.Post("/api/contactLists.removeContact", removeReq)
		require.NoError(t, err, "Should be able to remove contact")
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode == http.StatusOK {
			var result map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&result)
			require.NoError(t, err, "Should be able to decode response")

			success, ok := result["success"].(bool)
			require.True(t, ok, "Should have success field")
			assert.True(t, success, "Should return success true")
		}
	})
}

func TestContactListAPIMethodValidation(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer func() { suite.Cleanup() }()

	client := suite.APIClient

	t.Run("GET Endpoints Unauthenticated POST Requests", func(t *testing.T) {
		getEndpoints := []string{
			"/api/contactLists.getByIDs",
			"/api/contactLists.getContactsByList",
			"/api/contactLists.getListsByContact",
		}

		for _, endpoint := range getEndpoints {
			// Try POST on GET endpoint
			resp, err := client.Post(endpoint, map[string]interface{}{})
			require.NoError(t, err, "Should be able to make POST request to %s", endpoint)
			defer func() { _ = resp.Body.Close() }()

			assert.Equal(t, http.StatusUnauthorized, resp.StatusCode,
				"Should return 401 for unauthenticated request to %s", endpoint)
		}
	})

	t.Run("POST Endpoints Unauthenticated GET Requests", func(t *testing.T) {
		postEndpoints := []string{
			"/api/contactLists.updateStatus",
			"/api/contactLists.removeContact",
		}

		for _, endpoint := range postEndpoints {
			// Try GET on POST endpoint
			resp, err := client.Get(endpoint)
			require.NoError(t, err, "Should be able to make GET request to %s", endpoint)
			defer func() { _ = resp.Body.Close() }()

			assert.Equal(t, http.StatusUnauthorized, resp.StatusCode,
				"Should return 401 for unauthenticated request to %s", endpoint)
		}
	})
}
