package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/app"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/tests/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSubscribeE2E_FullFlow tests the complete subscribe flow:
// 1. API call to /subscribe
// 2. Contact created in database
// 3. Contact added to contact_lists table
// 4. Timeline entry created (database trigger worked)
//
// This test verifies the bug: "Failed to add to list: ApiError: Failed to subscribe to lists"
// where the API creates the contact but fails to add them to a list.
func TestSubscribeE2E_FullFlow(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer func() { suite.Cleanup() }()

	// Get base URL for the public endpoint
	baseURL := suite.ServerManager.GetURL()

	// Create test workspace
	workspace, err := suite.DataFactory.CreateWorkspace()
	require.NoError(t, err, "Failed to create test workspace")

	// Create a public test list for subscriptions
	list, err := suite.DataFactory.CreateList(workspace.ID, testutil.WithListPublic(true))
	require.NoError(t, err, "Failed to create test list")

	// Get workspace database connection for verification
	workspaceDB, err := suite.DBManager.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err, "Failed to get workspace database connection")

	t.Run("subscribe creates contact and adds to list", func(t *testing.T) {
		email := fmt.Sprintf("subscribe-e2e-test-%d@example.com", time.Now().UnixNano())

		// Step 1: Call /subscribe API
		subscribeReq := domain.SubscribeToListsRequest{
			WorkspaceID: workspace.ID,
			Contact: domain.Contact{
				Email: email,
			},
			ListIDs: []string{list.ID},
		}

		body, err := json.Marshal(subscribeReq)
		require.NoError(t, err)

		req, err := http.NewRequest("POST", baseURL+"/subscribe", bytes.NewBuffer(body))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")

		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Do(req)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		// Read response body for debugging
		var respBody map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&respBody); err == nil {
			t.Logf("Response status: %d, body: %+v", resp.StatusCode, respBody)
		}

		// Step 2: Assert API returns 200
		assert.Equal(t, http.StatusOK, resp.StatusCode, "Subscribe API should return 200 OK")

		// Step 3: Verify contact exists in contacts table
		var contactEmail string
		err = workspaceDB.QueryRow(`
			SELECT email FROM contacts WHERE email = $1
		`, email).Scan(&contactEmail)
		require.NoError(t, err, "Contact should exist in contacts table")
		assert.Equal(t, email, contactEmail, "Contact email should match")

		// Step 4: Verify contact_lists entry exists (THIS IS THE KEY CHECK)
		var listStatus string
		var listID string
		err = workspaceDB.QueryRow(`
			SELECT list_id, status FROM contact_lists
			WHERE email = $1 AND list_id = $2
		`, email, list.ID).Scan(&listID, &listStatus)
		require.NoError(t, err, "Contact should be in contact_lists table - THIS IS THE BUG LOCATION")
		assert.Equal(t, list.ID, listID, "List ID should match")
		assert.Equal(t, "active", listStatus, "Status should be active")

		// Step 5: Verify timeline entry was created (trigger worked)
		var timelineCount int
		err = workspaceDB.QueryRow(`
			SELECT COUNT(*) FROM contact_timeline
			WHERE email = $1
			AND entity_type = 'contact_list'
			AND kind = 'list.subscribed'
		`, email).Scan(&timelineCount)
		require.NoError(t, err, "Should be able to query contact_timeline")
		assert.GreaterOrEqual(t, timelineCount, 1, "Timeline entry should exist for list subscription (trigger worked)")

		t.Logf("SUCCESS: Contact %s subscribed to list %s, timeline entry created", email, list.ID)
	})

	t.Run("subscribe with contact details", func(t *testing.T) {
		email := fmt.Sprintf("subscribe-details-test-%d@example.com", time.Now().UnixNano())
		firstName := "John"
		lastName := "Doe"

		subscribeReq := domain.SubscribeToListsRequest{
			WorkspaceID: workspace.ID,
			Contact: domain.Contact{
				Email:     email,
				FirstName: &domain.NullableString{String: firstName, IsNull: false},
				LastName:  &domain.NullableString{String: lastName, IsNull: false},
			},
			ListIDs: []string{list.ID},
		}

		body, err := json.Marshal(subscribeReq)
		require.NoError(t, err)

		req, err := http.NewRequest("POST", baseURL+"/subscribe", bytes.NewBuffer(body))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")

		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Do(req)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusOK, resp.StatusCode, "Subscribe API should return 200 OK")

		// Verify contact details were saved
		var savedFirstName, savedLastName string
		err = workspaceDB.QueryRow(`
			SELECT first_name, last_name FROM contacts WHERE email = $1
		`, email).Scan(&savedFirstName, &savedLastName)
		require.NoError(t, err, "Contact should exist with details")
		assert.Equal(t, firstName, savedFirstName, "First name should be saved")
		assert.Equal(t, lastName, savedLastName, "Last name should be saved")

		// Verify list subscription
		var listStatus string
		err = workspaceDB.QueryRow(`
			SELECT status FROM contact_lists
			WHERE email = $1 AND list_id = $2
		`, email, list.ID).Scan(&listStatus)
		require.NoError(t, err, "Contact should be in contact_lists table")
		assert.Equal(t, "active", listStatus)
	})

	t.Run("subscribe to multiple lists", func(t *testing.T) {
		// Create a second list
		list2, err := suite.DataFactory.CreateList(workspace.ID, testutil.WithListPublic(true))
		require.NoError(t, err, "Failed to create second test list")

		email := fmt.Sprintf("subscribe-multi-test-%d@example.com", time.Now().UnixNano())

		subscribeReq := domain.SubscribeToListsRequest{
			WorkspaceID: workspace.ID,
			Contact: domain.Contact{
				Email: email,
			},
			ListIDs: []string{list.ID, list2.ID},
		}

		body, err := json.Marshal(subscribeReq)
		require.NoError(t, err)

		req, err := http.NewRequest("POST", baseURL+"/subscribe", bytes.NewBuffer(body))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")

		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Do(req)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusOK, resp.StatusCode, "Subscribe API should return 200 OK")

		// Verify subscription to both lists
		var count int
		err = workspaceDB.QueryRow(`
			SELECT COUNT(*) FROM contact_lists
			WHERE email = $1 AND list_id IN ($2, $3) AND status = 'active'
		`, email, list.ID, list2.ID).Scan(&count)
		require.NoError(t, err, "Should be able to query contact_lists")
		assert.Equal(t, 2, count, "Contact should be subscribed to both lists")

		// Verify timeline entries for both subscriptions
		var timelineCount int
		err = workspaceDB.QueryRow(`
			SELECT COUNT(*) FROM contact_timeline
			WHERE email = $1
			AND entity_type = 'contact_list'
			AND kind = 'list.subscribed'
		`, email).Scan(&timelineCount)
		require.NoError(t, err)
		assert.Equal(t, 2, timelineCount, "Should have timeline entries for both list subscriptions")
	})

	t.Run("subscribe to double opt-in list creates pending status", func(t *testing.T) {
		// Create a double opt-in list
		doubleOptInList, err := suite.DataFactory.CreateList(workspace.ID,
			testutil.WithListPublic(true),
			testutil.WithListDoubleOptin(true),
		)
		require.NoError(t, err, "Failed to create double opt-in list")

		email := fmt.Sprintf("subscribe-doi-test-%d@example.com", time.Now().UnixNano())

		subscribeReq := domain.SubscribeToListsRequest{
			WorkspaceID: workspace.ID,
			Contact: domain.Contact{
				Email: email,
			},
			ListIDs: []string{doubleOptInList.ID},
		}

		body, err := json.Marshal(subscribeReq)
		require.NoError(t, err)

		req, err := http.NewRequest("POST", baseURL+"/subscribe", bytes.NewBuffer(body))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")

		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Do(req)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusOK, resp.StatusCode, "Subscribe API should return 200 OK")

		// Verify contact is in pending status for double opt-in list
		var listStatus string
		err = workspaceDB.QueryRow(`
			SELECT status FROM contact_lists
			WHERE email = $1 AND list_id = $2
		`, email, doubleOptInList.ID).Scan(&listStatus)
		require.NoError(t, err, "Contact should be in contact_lists table")
		assert.Equal(t, "pending", listStatus, "Status should be pending for double opt-in list")

		// Verify timeline entry with pending status
		var timelineKind string
		err = workspaceDB.QueryRow(`
			SELECT kind FROM contact_timeline
			WHERE email = $1
			AND entity_type = 'contact_list'
			AND entity_id = $2
			ORDER BY created_at DESC
			LIMIT 1
		`, email, doubleOptInList.ID).Scan(&timelineKind)
		require.NoError(t, err, "Timeline entry should exist")
		assert.Equal(t, "list.pending", timelineKind, "Timeline kind should be list.pending for double opt-in")
	})

	t.Run("subscribe to non-public list fails for unauthenticated request", func(t *testing.T) {
		// Create a private (non-public) list
		privateList, err := suite.DataFactory.CreateList(workspace.ID,
			testutil.WithListPublic(false),
		)
		require.NoError(t, err, "Failed to create private list")

		email := fmt.Sprintf("subscribe-private-test-%d@example.com", time.Now().UnixNano())

		subscribeReq := domain.SubscribeToListsRequest{
			WorkspaceID: workspace.ID,
			Contact: domain.Contact{
				Email: email,
			},
			ListIDs: []string{privateList.ID},
		}

		body, err := json.Marshal(subscribeReq)
		require.NoError(t, err)

		req, err := http.NewRequest("POST", baseURL+"/subscribe", bytes.NewBuffer(body))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")

		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Do(req)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		// Should fail because list is not public (400 Bad Request)
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode,
			"Subscribe to private list should fail for unauthenticated request")

		// Verify contact was NOT added to the private list
		var count int
		err = workspaceDB.QueryRow(`
			SELECT COUNT(*) FROM contact_lists
			WHERE email = $1 AND list_id = $2
		`, email, privateList.ID).Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 0, count, "Contact should NOT be in private list")
	})

	t.Run("subscribe to non-existent list fails", func(t *testing.T) {
		email := fmt.Sprintf("subscribe-nolist-test-%d@example.com", time.Now().UnixNano())

		subscribeReq := domain.SubscribeToListsRequest{
			WorkspaceID: workspace.ID,
			Contact: domain.Contact{
				Email: email,
			},
			ListIDs: []string{"non-existent-list-id"},
		}

		body, err := json.Marshal(subscribeReq)
		require.NoError(t, err)

		req, err := http.NewRequest("POST", baseURL+"/subscribe", bytes.NewBuffer(body))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")

		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Do(req)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		// Should fail because list doesn't exist
		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode,
			"Subscribe to non-existent list should fail")
	})
}

// TestSubscribeE2E_WebhookTriggersExist verifies that webhook triggers work
// when contacts are added to lists (if webhook_subscriptions exist)
func TestSubscribeE2E_WebhookTriggersExist(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer func() { suite.Cleanup() }()

	baseURL := suite.ServerManager.GetURL()

	workspace, err := suite.DataFactory.CreateWorkspace()
	require.NoError(t, err)

	list, err := suite.DataFactory.CreateList(workspace.ID, testutil.WithListPublic(true))
	require.NoError(t, err)

	workspaceDB, err := suite.DBManager.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)

	t.Run("verify webhook tables exist", func(t *testing.T) {
		// This test verifies that the webhook tables were created by migrations
		// If they don't exist, the subscribe trigger would fail

		var tableExists bool
		err = workspaceDB.QueryRow(`
			SELECT EXISTS (
				SELECT FROM information_schema.tables
				WHERE table_schema = 'public'
				AND table_name = 'webhook_subscriptions'
			)
		`).Scan(&tableExists)
		require.NoError(t, err)
		assert.True(t, tableExists, "webhook_subscriptions table should exist")

		err = workspaceDB.QueryRow(`
			SELECT EXISTS (
				SELECT FROM information_schema.tables
				WHERE table_schema = 'public'
				AND table_name = 'webhook_deliveries'
			)
		`).Scan(&tableExists)
		require.NoError(t, err)
		assert.True(t, tableExists, "webhook_deliveries table should exist")
	})

	t.Run("subscribe works even without webhook subscriptions", func(t *testing.T) {
		email := fmt.Sprintf("webhook-test-%d@example.com", time.Now().UnixNano())

		subscribeReq := domain.SubscribeToListsRequest{
			WorkspaceID: workspace.ID,
			Contact: domain.Contact{
				Email: email,
			},
			ListIDs: []string{list.ID},
		}

		body, err := json.Marshal(subscribeReq)
		require.NoError(t, err)

		req, err := http.NewRequest("POST", baseURL+"/subscribe", bytes.NewBuffer(body))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")

		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Do(req)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		// Should succeed even though there are no webhook subscriptions
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		// Verify subscription was created
		var count int
		err = workspaceDB.QueryRow(`
			SELECT COUNT(*) FROM contact_lists
			WHERE email = $1 AND list_id = $2
		`, email, list.ID).Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 1, count, "Contact should be subscribed to list")
	})
}

// TestSubscribeE2E_DatabaseTriggers specifically tests that database triggers work correctly
func TestSubscribeE2E_DatabaseTriggers(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer func() { suite.Cleanup() }()

	baseURL := suite.ServerManager.GetURL()

	workspace, err := suite.DataFactory.CreateWorkspace()
	require.NoError(t, err)

	list, err := suite.DataFactory.CreateList(workspace.ID, testutil.WithListPublic(true))
	require.NoError(t, err)

	workspaceDB, err := suite.DBManager.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)

	t.Run("verify trigger functions exist", func(t *testing.T) {
		var functionExists bool

		// Check track_contact_list_changes trigger function
		err = workspaceDB.QueryRow(`
			SELECT EXISTS (
				SELECT 1 FROM pg_proc p
				JOIN pg_namespace n ON p.pronamespace = n.oid
				WHERE n.nspname = 'public' AND p.proname = 'track_contact_list_changes'
			)
		`).Scan(&functionExists)
		require.NoError(t, err)
		assert.True(t, functionExists, "track_contact_list_changes function should exist")

		// Check webhook_contact_lists_trigger function
		err = workspaceDB.QueryRow(`
			SELECT EXISTS (
				SELECT 1 FROM pg_proc p
				JOIN pg_namespace n ON p.pronamespace = n.oid
				WHERE n.nspname = 'public' AND p.proname = 'webhook_contact_lists_trigger'
			)
		`).Scan(&functionExists)
		require.NoError(t, err)
		assert.True(t, functionExists, "webhook_contact_lists_trigger function should exist")
	})

	t.Run("verify triggers are attached to contact_lists table", func(t *testing.T) {
		var triggerExists bool

		// Check contact_list_changes_trigger (this is the actual trigger name in init.go)
		err = workspaceDB.QueryRow(`
			SELECT EXISTS (
				SELECT 1 FROM pg_trigger t
				JOIN pg_class c ON t.tgrelid = c.oid
				WHERE c.relname = 'contact_lists'
				AND t.tgname = 'contact_list_changes_trigger'
			)
		`).Scan(&triggerExists)
		require.NoError(t, err)
		assert.True(t, triggerExists, "contact_list_changes_trigger should be attached to contact_lists")

		// Check webhook_contact_lists trigger
		err = workspaceDB.QueryRow(`
			SELECT EXISTS (
				SELECT 1 FROM pg_trigger t
				JOIN pg_class c ON t.tgrelid = c.oid
				WHERE c.relname = 'contact_lists'
				AND t.tgname = 'webhook_contact_lists'
			)
		`).Scan(&triggerExists)
		require.NoError(t, err)
		assert.True(t, triggerExists, "webhook_contact_lists trigger should be attached to contact_lists")
	})

	t.Run("triggers fire correctly on subscribe", func(t *testing.T) {
		email := fmt.Sprintf("trigger-test-%d@example.com", time.Now().UnixNano())

		// Get count before subscribe
		var timelineCountBefore int
		err = workspaceDB.QueryRow(`SELECT COUNT(*) FROM contact_timeline WHERE email = $1`, email).Scan(&timelineCountBefore)
		require.NoError(t, err)
		assert.Equal(t, 0, timelineCountBefore, "No timeline entries should exist before subscribe")

		// Subscribe
		subscribeReq := domain.SubscribeToListsRequest{
			WorkspaceID: workspace.ID,
			Contact: domain.Contact{
				Email: email,
			},
			ListIDs: []string{list.ID},
		}

		body, err := json.Marshal(subscribeReq)
		require.NoError(t, err)

		req, err := http.NewRequest("POST", baseURL+"/subscribe", bytes.NewBuffer(body))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")

		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Do(req)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		require.Equal(t, http.StatusOK, resp.StatusCode, "Subscribe should succeed")

		// Verify timeline entry was created by trigger
		var timelineCountAfter int
		err = workspaceDB.QueryRow(`
			SELECT COUNT(*) FROM contact_timeline
			WHERE email = $1 AND entity_type = 'contact_list'
		`, email).Scan(&timelineCountAfter)
		require.NoError(t, err)
		assert.Equal(t, 1, timelineCountAfter, "Timeline entry should be created by trigger")

		// Verify timeline entry has correct kind
		var kind string
		err = workspaceDB.QueryRow(`
			SELECT kind FROM contact_timeline
			WHERE email = $1 AND entity_type = 'contact_list'
		`, email).Scan(&kind)
		require.NoError(t, err)
		assert.Equal(t, "list.subscribed", kind, "Timeline kind should be 'list.subscribed'")
	})
}
