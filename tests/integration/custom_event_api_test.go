package integration

import (
	"database/sql"
	"encoding/json"
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

func TestCustomEventAPIEndpoints(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer func() { suite.Cleanup() }()

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

	t.Run("Upsert Custom Event", func(t *testing.T) {
		testUpsertCustomEvent(t, client, workspace.ID)
	})

	t.Run("Get Custom Event", func(t *testing.T) {
		testGetCustomEvent(t, client, workspace.ID)
	})

	t.Run("List Custom Events", func(t *testing.T) {
		testListCustomEvents(t, client, workspace.ID)
	})

	t.Run("Import Custom Events", func(t *testing.T) {
		testImportCustomEvents(t, client, workspace.ID)
	})

	t.Run("Goal Tracking", func(t *testing.T) {
		testGoalTracking(t, client, workspace.ID)
	})

	t.Run("Soft Delete", func(t *testing.T) {
		testSoftDelete(t, client, workspace.ID)
	})

	t.Run("Validation Errors", func(t *testing.T) {
		testValidationErrors(t, client, workspace.ID)
	})

	t.Run("Timeline Integration", func(t *testing.T) {
		// Get workspace database for direct timeline verification
		workspaceDB, err := suite.DBManager.GetWorkspaceDB(workspace.ID)
		require.NoError(t, err, "Failed to get workspace database")
		testTimelineIntegration(t, client, workspace.ID, workspaceDB)
	})
}

func testUpsertCustomEvent(t *testing.T, client *testutil.APIClient, workspaceID string) {
	t.Run("should upsert event with all fields", func(t *testing.T) {
		email := testutil.GenerateTestEmail()
		externalID := "order_" + testutil.GenerateRandomString(8)
		goalName := "first_purchase"
		goalType := domain.GoalTypePurchase
		goalValue := 99.99
		occurredAt := time.Now().Add(-1 * time.Hour)
		integrationID := "shopify_123"

		req := map[string]interface{}{
			"workspace_id":   workspaceID,
			"email":          email,
			"event_name":     "order.completed",
			"external_id":    externalID,
			"properties":     map[string]interface{}{"total": 99.99, "items": 3},
			"occurred_at":    occurredAt.Format(time.RFC3339),
			"integration_id": integrationID,
			"goal_name":      goalName,
			"goal_type":      goalType,
			"goal_value":     goalValue,
		}

		resp, err := client.Post("/api/customEvents.upsert", req)
		require.NoError(t, err)
		defer resp.Body.Close()

		require.Equal(t, http.StatusOK, resp.StatusCode, "Should return 200 OK")

		var result map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&result)
		require.NoError(t, err)

		eventData, ok := result["event"].(map[string]interface{})
		require.True(t, ok, "Response should contain event")

		assert.Equal(t, externalID, eventData["external_id"])
		assert.Equal(t, email, eventData["email"])
		assert.Equal(t, "order.completed", eventData["event_name"])
		assert.Equal(t, goalName, eventData["goal_name"])
		assert.Equal(t, goalType, eventData["goal_type"])
		assert.Equal(t, goalValue, eventData["goal_value"])
		assert.Equal(t, integrationID, eventData["integration_id"])
		assert.NotNil(t, eventData["properties"])
	})

	t.Run("should upsert event with minimal fields", func(t *testing.T) {
		email := testutil.GenerateTestEmail()
		externalID := "event_" + testutil.GenerateRandomString(8)

		req := map[string]interface{}{
			"workspace_id": workspaceID,
			"email":        email,
			"event_name":   "page.view",
			"external_id":  externalID,
		}

		resp, err := client.Post("/api/customEvents.upsert", req)
		require.NoError(t, err)
		defer resp.Body.Close()

		require.Equal(t, http.StatusOK, resp.StatusCode, "Should return 200 OK for minimal fields")

		var result map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&result)
		require.NoError(t, err)

		eventData := result["event"].(map[string]interface{})
		assert.Equal(t, externalID, eventData["external_id"])
		assert.Equal(t, email, eventData["email"])
		assert.Equal(t, "page.view", eventData["event_name"])
		assert.Equal(t, "api", eventData["source"])
		// Goal fields should be nil/absent
		assert.Nil(t, eventData["goal_name"])
		assert.Nil(t, eventData["goal_type"])
		assert.Nil(t, eventData["goal_value"])
	})

	t.Run("should update existing event on upsert", func(t *testing.T) {
		email := testutil.GenerateTestEmail()
		externalID := "order_update_" + testutil.GenerateRandomString(8)
		eventName := "order.status"

		// First upsert
		req1 := map[string]interface{}{
			"workspace_id": workspaceID,
			"email":        email,
			"event_name":   eventName,
			"external_id":  externalID,
			"properties":   map[string]interface{}{"status": "pending"},
			"occurred_at":  time.Now().Add(-2 * time.Hour).Format(time.RFC3339),
		}

		resp1, err := client.Post("/api/customEvents.upsert", req1)
		require.NoError(t, err)
		resp1.Body.Close()
		require.Equal(t, http.StatusOK, resp1.StatusCode)

		// Second upsert with newer occurred_at
		req2 := map[string]interface{}{
			"workspace_id": workspaceID,
			"email":        email,
			"event_name":   eventName,
			"external_id":  externalID,
			"properties":   map[string]interface{}{"status": "shipped"},
			"occurred_at":  time.Now().Format(time.RFC3339),
		}

		resp2, err := client.Post("/api/customEvents.upsert", req2)
		require.NoError(t, err)
		defer resp2.Body.Close()
		require.Equal(t, http.StatusOK, resp2.StatusCode)

		// Verify the updated event
		getResp, err := client.Get("/api/customEvents.get", map[string]string{
			"workspace_id": workspaceID,
			"event_name":   eventName,
			"external_id":  externalID,
		})
		require.NoError(t, err)
		defer getResp.Body.Close()

		var result map[string]interface{}
		err = json.NewDecoder(getResp.Body).Decode(&result)
		require.NoError(t, err)

		eventData := result["event"].(map[string]interface{})
		props := eventData["properties"].(map[string]interface{})
		assert.Equal(t, "shipped", props["status"], "Properties should be updated")
	})

	t.Run("should auto-create contact if not exists", func(t *testing.T) {
		// Use a completely new email that doesn't exist
		email := "newcontact_" + testutil.GenerateRandomString(8) + "@example.com"
		externalID := "signup_" + testutil.GenerateRandomString(8)

		req := map[string]interface{}{
			"workspace_id": workspaceID,
			"email":        email,
			"event_name":   "user.signup",
			"external_id":  externalID,
		}

		resp, err := client.Post("/api/customEvents.upsert", req)
		require.NoError(t, err)
		defer resp.Body.Close()

		require.Equal(t, http.StatusOK, resp.StatusCode, "Should succeed even if contact doesn't exist")

		// Verify the event was created
		var result map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&result)
		require.NoError(t, err)

		eventData := result["event"].(map[string]interface{})
		assert.Equal(t, email, eventData["email"])
	})
}

func testGetCustomEvent(t *testing.T, client *testutil.APIClient, workspaceID string) {
	// Create a test event first
	email := testutil.GenerateTestEmail()
	externalID := "get_test_" + testutil.GenerateRandomString(8)
	eventName := "test.get"

	createReq := map[string]interface{}{
		"workspace_id": workspaceID,
		"email":        email,
		"event_name":   eventName,
		"external_id":  externalID,
		"properties":   map[string]interface{}{"test": true},
	}

	createResp, err := client.Post("/api/customEvents.upsert", createReq)
	require.NoError(t, err)
	createResp.Body.Close()
	require.Equal(t, http.StatusOK, createResp.StatusCode)

	t.Run("should get event by event_name and external_id", func(t *testing.T) {
		resp, err := client.Get("/api/customEvents.get", map[string]string{
			"workspace_id": workspaceID,
			"event_name":   eventName,
			"external_id":  externalID,
		})
		require.NoError(t, err)
		defer resp.Body.Close()

		require.Equal(t, http.StatusOK, resp.StatusCode)

		var result map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&result)
		require.NoError(t, err)

		eventData := result["event"].(map[string]interface{})
		assert.Equal(t, externalID, eventData["external_id"])
		assert.Equal(t, email, eventData["email"])
		assert.Equal(t, eventName, eventData["event_name"])
	})

	t.Run("should return 404 for non-existent event", func(t *testing.T) {
		resp, err := client.Get("/api/customEvents.get", map[string]string{
			"workspace_id": workspaceID,
			"event_name":   "nonexistent.event",
			"external_id":  "nonexistent_id",
		})
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	t.Run("should return 400 when required params missing", func(t *testing.T) {
		// Missing external_id
		resp, err := client.Get("/api/customEvents.get", map[string]string{
			"workspace_id": workspaceID,
			"event_name":   eventName,
		})
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})
}

func testListCustomEvents(t *testing.T, client *testutil.APIClient, workspaceID string) {
	// Create multiple test events
	email := testutil.GenerateTestEmail()
	eventName := "list.test"

	for i := 0; i < 5; i++ {
		req := map[string]interface{}{
			"workspace_id": workspaceID,
			"email":        email,
			"event_name":   eventName,
			"external_id":  "list_" + testutil.GenerateRandomString(8),
		}

		resp, err := client.Post("/api/customEvents.upsert", req)
		require.NoError(t, err)
		resp.Body.Close()
	}

	t.Run("should list events by email", func(t *testing.T) {
		resp, err := client.Get("/api/customEvents.list", map[string]string{
			"workspace_id": workspaceID,
			"email":        email,
		})
		require.NoError(t, err)
		defer resp.Body.Close()

		require.Equal(t, http.StatusOK, resp.StatusCode)

		var result map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&result)
		require.NoError(t, err)

		events := result["events"].([]interface{})
		assert.GreaterOrEqual(t, len(events), 5, "Should return at least 5 events")

		count := int(result["count"].(float64))
		assert.Equal(t, len(events), count)
	})

	t.Run("should list events by event_name", func(t *testing.T) {
		resp, err := client.Get("/api/customEvents.list", map[string]string{
			"workspace_id": workspaceID,
			"event_name":   eventName,
		})
		require.NoError(t, err)
		defer resp.Body.Close()

		require.Equal(t, http.StatusOK, resp.StatusCode)

		var result map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&result)
		require.NoError(t, err)

		events := result["events"].([]interface{})
		assert.GreaterOrEqual(t, len(events), 5, "Should return at least 5 events")
	})

	t.Run("should support pagination", func(t *testing.T) {
		resp, err := client.Get("/api/customEvents.list", map[string]string{
			"workspace_id": workspaceID,
			"email":        email,
			"limit":        "2",
			"offset":       "0",
		})
		require.NoError(t, err)
		defer resp.Body.Close()

		require.Equal(t, http.StatusOK, resp.StatusCode)

		var result map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&result)
		require.NoError(t, err)

		events := result["events"].([]interface{})
		assert.Equal(t, 2, len(events), "Should respect limit")
	})

	t.Run("should return 400 when neither email nor event_name provided", func(t *testing.T) {
		resp, err := client.Get("/api/customEvents.list", map[string]string{
			"workspace_id": workspaceID,
		})
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})
}

func testImportCustomEvents(t *testing.T, client *testutil.APIClient, workspaceID string) {
	t.Run("should import multiple events", func(t *testing.T) {
		email1 := testutil.GenerateTestEmail()
		email2 := testutil.GenerateTestEmail()

		events := []map[string]interface{}{
			{
				"external_id": "import_" + testutil.GenerateRandomString(8),
				"email":       email1,
				"event_name":  "import.test1",
				"properties":  map[string]interface{}{"batch": 1},
			},
			{
				"external_id": "import_" + testutil.GenerateRandomString(8),
				"email":       email2,
				"event_name":  "import.test2",
				"properties":  map[string]interface{}{"batch": 2},
				"goal_type":   domain.GoalTypeLead,
				"goal_name":   "form_submission",
			},
			{
				"external_id": "import_" + testutil.GenerateRandomString(8),
				"email":       email1,
				"event_name":  "import.test3",
				"properties":  map[string]interface{}{"batch": 3},
			},
		}

		req := map[string]interface{}{
			"workspace_id": workspaceID,
			"events":       events,
		}

		resp, err := client.Post("/api/customEvents.import", req)
		require.NoError(t, err)
		defer resp.Body.Close()

		require.Equal(t, http.StatusCreated, resp.StatusCode)

		var result map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&result)
		require.NoError(t, err)

		eventIDs := result["event_ids"].([]interface{})
		assert.Equal(t, 3, len(eventIDs), "Should return 3 event IDs")

		count := int(result["count"].(float64))
		assert.Equal(t, 3, count)
	})

	t.Run("should import events with goal tracking", func(t *testing.T) {
		email := testutil.GenerateTestEmail()
		externalID := "import_goal_" + testutil.GenerateRandomString(8)
		goalValue := 149.99

		events := []map[string]interface{}{
			{
				"external_id": externalID,
				"email":       email,
				"event_name":  "purchase.completed",
				"properties":  map[string]interface{}{"product": "Premium Plan"},
				"goal_type":   domain.GoalTypePurchase,
				"goal_name":   "subscription_purchase",
				"goal_value":  goalValue,
			},
		}

		req := map[string]interface{}{
			"workspace_id": workspaceID,
			"events":       events,
		}

		resp, err := client.Post("/api/customEvents.import", req)
		require.NoError(t, err)
		defer resp.Body.Close()

		require.Equal(t, http.StatusCreated, resp.StatusCode)

		// Verify the imported event has goal fields
		getResp, err := client.Get("/api/customEvents.get", map[string]string{
			"workspace_id": workspaceID,
			"event_name":   "purchase.completed",
			"external_id":  externalID,
		})
		require.NoError(t, err)
		defer getResp.Body.Close()

		var result map[string]interface{}
		err = json.NewDecoder(getResp.Body).Decode(&result)
		require.NoError(t, err)

		eventData := result["event"].(map[string]interface{})
		assert.Equal(t, domain.GoalTypePurchase, eventData["goal_type"])
		assert.Equal(t, "subscription_purchase", eventData["goal_name"])
		assert.Equal(t, goalValue, eventData["goal_value"])
	})

	t.Run("should reject import with more than 50 events", func(t *testing.T) {
		events := make([]map[string]interface{}, 51)
		for i := 0; i < 51; i++ {
			events[i] = map[string]interface{}{
				"external_id": "bulk_" + testutil.GenerateRandomString(8),
				"email":       testutil.GenerateTestEmail(),
				"event_name":  "bulk.test",
			}
		}

		req := map[string]interface{}{
			"workspace_id": workspaceID,
			"events":       events,
		}

		resp, err := client.Post("/api/customEvents.import", req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "Should reject import with more than 50 events")
	})

	t.Run("should reject empty events array", func(t *testing.T) {
		req := map[string]interface{}{
			"workspace_id": workspaceID,
			"events":       []map[string]interface{}{},
		}

		resp, err := client.Post("/api/customEvents.import", req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "Should reject empty events array")
	})
}

func testGoalTracking(t *testing.T, client *testutil.APIClient, workspaceID string) {
	t.Run("should create event with purchase goal", func(t *testing.T) {
		email := testutil.GenerateTestEmail()
		externalID := "purchase_" + testutil.GenerateRandomString(8)
		goalValue := 299.99

		req := map[string]interface{}{
			"workspace_id": workspaceID,
			"email":        email,
			"event_name":   "checkout.completed",
			"external_id":  externalID,
			"goal_type":    domain.GoalTypePurchase,
			"goal_name":    "checkout_conversion",
			"goal_value":   goalValue,
		}

		resp, err := client.Post("/api/customEvents.upsert", req)
		require.NoError(t, err)
		defer resp.Body.Close()

		require.Equal(t, http.StatusOK, resp.StatusCode)

		var result map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&result)
		require.NoError(t, err)

		eventData := result["event"].(map[string]interface{})
		assert.Equal(t, domain.GoalTypePurchase, eventData["goal_type"])
		assert.Equal(t, goalValue, eventData["goal_value"])
	})

	t.Run("should create event with subscription goal", func(t *testing.T) {
		email := testutil.GenerateTestEmail()
		externalID := "sub_" + testutil.GenerateRandomString(8)
		goalValue := 49.99

		req := map[string]interface{}{
			"workspace_id": workspaceID,
			"email":        email,
			"event_name":   "subscription.started",
			"external_id":  externalID,
			"goal_type":    domain.GoalTypeSubscription,
			"goal_name":    "monthly_plan",
			"goal_value":   goalValue,
		}

		resp, err := client.Post("/api/customEvents.upsert", req)
		require.NoError(t, err)
		defer resp.Body.Close()

		require.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("should create event with lead goal without value", func(t *testing.T) {
		email := testutil.GenerateTestEmail()
		externalID := "lead_" + testutil.GenerateRandomString(8)

		req := map[string]interface{}{
			"workspace_id": workspaceID,
			"email":        email,
			"event_name":   "form.submitted",
			"external_id":  externalID,
			"goal_type":    domain.GoalTypeLead,
			"goal_name":    "contact_form",
			// No goal_value - should be optional for lead type
		}

		resp, err := client.Post("/api/customEvents.upsert", req)
		require.NoError(t, err)
		defer resp.Body.Close()

		require.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("should support negative goal_value for refunds", func(t *testing.T) {
		email := testutil.GenerateTestEmail()
		externalID := "refund_" + testutil.GenerateRandomString(8)
		goalValue := -50.0

		req := map[string]interface{}{
			"workspace_id": workspaceID,
			"email":        email,
			"event_name":   "order.refunded",
			"external_id":  externalID,
			"goal_type":    domain.GoalTypePurchase,
			"goal_name":    "partial_refund",
			"goal_value":   goalValue,
		}

		resp, err := client.Post("/api/customEvents.upsert", req)
		require.NoError(t, err)
		defer resp.Body.Close()

		require.Equal(t, http.StatusOK, resp.StatusCode)

		var result map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&result)
		require.NoError(t, err)

		eventData := result["event"].(map[string]interface{})
		assert.Equal(t, goalValue, eventData["goal_value"])
	})
}

func testSoftDelete(t *testing.T, client *testutil.APIClient, workspaceID string) {
	email := testutil.GenerateTestEmail()
	externalID := "softdelete_" + testutil.GenerateRandomString(8)
	eventName := "test.softdelete"

	// Create an event
	createReq := map[string]interface{}{
		"workspace_id": workspaceID,
		"email":        email,
		"event_name":   eventName,
		"external_id":  externalID,
	}

	createResp, err := client.Post("/api/customEvents.upsert", createReq)
	require.NoError(t, err)
	createResp.Body.Close()
	require.Equal(t, http.StatusOK, createResp.StatusCode)

	t.Run("should soft-delete event via upsert", func(t *testing.T) {
		deleteTime := time.Now()
		deleteReq := map[string]interface{}{
			"workspace_id": workspaceID,
			"email":        email,
			"event_name":   eventName,
			"external_id":  externalID,
			"deleted_at":   deleteTime.Format(time.RFC3339),
		}

		resp, err := client.Post("/api/customEvents.upsert", deleteReq)
		require.NoError(t, err)
		defer resp.Body.Close()

		require.Equal(t, http.StatusOK, resp.StatusCode)

		var result map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&result)
		require.NoError(t, err)

		eventData := result["event"].(map[string]interface{})
		assert.NotNil(t, eventData["deleted_at"], "deleted_at should be set")
	})

	t.Run("should exclude soft-deleted events from get", func(t *testing.T) {
		resp, err := client.Get("/api/customEvents.get", map[string]string{
			"workspace_id": workspaceID,
			"event_name":   eventName,
			"external_id":  externalID,
		})
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusNotFound, resp.StatusCode, "Soft-deleted events should not be found")
	})

	t.Run("should exclude soft-deleted events from list", func(t *testing.T) {
		resp, err := client.Get("/api/customEvents.list", map[string]string{
			"workspace_id": workspaceID,
			"email":        email,
		})
		require.NoError(t, err)
		defer resp.Body.Close()

		require.Equal(t, http.StatusOK, resp.StatusCode)

		var result map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&result)
		require.NoError(t, err)

		// Events might be nil if all events for this email were soft-deleted
		if result["events"] != nil {
			events := result["events"].([]interface{})
			// Should not contain the soft-deleted event
			for _, e := range events {
				event := e.(map[string]interface{})
				assert.NotEqual(t, externalID, event["external_id"], "Soft-deleted event should not appear in list")
			}
		}
		// If events is nil or empty, that's also correct - the soft-deleted event is excluded
	})
}

func testTimelineIntegration(t *testing.T, client *testutil.APIClient, workspaceID string, db *sql.DB) {
	t.Run("should create timeline entry when custom event is created", func(t *testing.T) {
		email := testutil.GenerateTestEmail()
		externalID := "timeline_test_" + testutil.GenerateRandomString(8)
		eventName := "order.completed"

		// Create a custom event
		req := map[string]interface{}{
			"workspace_id": workspaceID,
			"email":        email,
			"event_name":   eventName,
			"external_id":  externalID,
			"properties":   map[string]interface{}{"total": 99.99},
			"goal_type":    domain.GoalTypePurchase,
			"goal_name":    "first_purchase",
			"goal_value":   99.99,
		}

		resp, err := client.Post("/api/customEvents.upsert", req)
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		// Wait briefly for trigger to execute
		time.Sleep(100 * time.Millisecond)

		// Query the contact_timeline table directly to verify the entry was created
		var count int
		err = db.QueryRow(`
			SELECT COUNT(*)
			FROM contact_timeline
			WHERE email = $1
			AND entity_type = 'custom_event'
			AND kind = $2
		`, email, eventName).Scan(&count)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, count, 1, "Timeline entry should be created for custom event")

		// Verify the timeline entry has correct data
		var entityType, kind, operation string
		err = db.QueryRow(`
			SELECT entity_type, kind, operation
			FROM contact_timeline
			WHERE email = $1
			AND entity_type = 'custom_event'
			AND kind = $2
			ORDER BY created_at DESC
			LIMIT 1
		`, email, eventName).Scan(&entityType, &kind, &operation)
		require.NoError(t, err)
		assert.Equal(t, "custom_event", entityType)
		assert.Equal(t, eventName, kind)
		assert.Equal(t, "insert", operation)
	})

	t.Run("should update timeline entry when custom event is updated", func(t *testing.T) {
		email := testutil.GenerateTestEmail()
		externalID := "timeline_update_" + testutil.GenerateRandomString(8)
		eventName := "subscription.renewed"

		// Create initial event
		req1 := map[string]interface{}{
			"workspace_id": workspaceID,
			"email":        email,
			"event_name":   eventName,
			"external_id":  externalID,
			"properties":   map[string]interface{}{"plan": "basic"},
		}

		resp1, err := client.Post("/api/customEvents.upsert", req1)
		require.NoError(t, err)
		resp1.Body.Close()
		require.Equal(t, http.StatusOK, resp1.StatusCode)

		time.Sleep(100 * time.Millisecond)

		// Update the event
		req2 := map[string]interface{}{
			"workspace_id": workspaceID,
			"email":        email,
			"event_name":   eventName,
			"external_id":  externalID,
			"properties":   map[string]interface{}{"plan": "premium"},
		}

		resp2, err := client.Post("/api/customEvents.upsert", req2)
		require.NoError(t, err)
		resp2.Body.Close()
		require.Equal(t, http.StatusOK, resp2.StatusCode)

		time.Sleep(100 * time.Millisecond)

		// Count timeline entries - should have at least 2 (insert and update)
		var count int
		err = db.QueryRow(`
			SELECT COUNT(*)
			FROM contact_timeline
			WHERE email = $1
			AND entity_type = 'custom_event'
			AND kind = $2
		`, email, eventName).Scan(&count)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, count, 2, "Should have both insert and update timeline entries")

		// Verify we have an update operation
		var hasUpdate bool
		err = db.QueryRow(`
			SELECT EXISTS(
				SELECT 1 FROM contact_timeline
				WHERE email = $1
				AND entity_type = 'custom_event'
				AND kind = $2
				AND operation = 'update'
			)
		`, email, eventName).Scan(&hasUpdate)
		require.NoError(t, err)
		assert.True(t, hasUpdate, "Should have an update operation in timeline")
	})
}

func testValidationErrors(t *testing.T, client *testutil.APIClient, workspaceID string) {
	t.Run("should return error for missing email", func(t *testing.T) {
		req := map[string]interface{}{
			"workspace_id": workspaceID,
			"event_name":   "test.event",
			"external_id":  "test_" + testutil.GenerateRandomString(8),
		}

		resp, err := client.Post("/api/customEvents.upsert", req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("should return error for missing external_id", func(t *testing.T) {
		req := map[string]interface{}{
			"workspace_id": workspaceID,
			"email":        testutil.GenerateTestEmail(),
			"event_name":   "test.event",
		}

		resp, err := client.Post("/api/customEvents.upsert", req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("should return error for missing event_name", func(t *testing.T) {
		req := map[string]interface{}{
			"workspace_id": workspaceID,
			"email":        testutil.GenerateTestEmail(),
			"external_id":  "test_" + testutil.GenerateRandomString(8),
		}

		resp, err := client.Post("/api/customEvents.upsert", req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("should return error for invalid goal_type", func(t *testing.T) {
		req := map[string]interface{}{
			"workspace_id": workspaceID,
			"email":        testutil.GenerateTestEmail(),
			"event_name":   "test.event",
			"external_id":  "test_" + testutil.GenerateRandomString(8),
			"goal_type":    "invalid_type",
		}

		resp, err := client.Post("/api/customEvents.upsert", req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("should return error for purchase without goal_value", func(t *testing.T) {
		req := map[string]interface{}{
			"workspace_id": workspaceID,
			"email":        testutil.GenerateTestEmail(),
			"event_name":   "test.event",
			"external_id":  "test_" + testutil.GenerateRandomString(8),
			"goal_type":    domain.GoalTypePurchase,
			// Missing goal_value - should fail
		}

		resp, err := client.Post("/api/customEvents.upsert", req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("should return error for subscription without goal_value", func(t *testing.T) {
		req := map[string]interface{}{
			"workspace_id": workspaceID,
			"email":        testutil.GenerateTestEmail(),
			"event_name":   "test.event",
			"external_id":  "test_" + testutil.GenerateRandomString(8),
			"goal_type":    domain.GoalTypeSubscription,
			// Missing goal_value - should fail
		}

		resp, err := client.Post("/api/customEvents.upsert", req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("should return error for invalid event_name format", func(t *testing.T) {
		req := map[string]interface{}{
			"workspace_id": workspaceID,
			"email":        testutil.GenerateTestEmail(),
			"event_name":   "Invalid Event Name!", // Contains uppercase and special chars
			"external_id":  "test_" + testutil.GenerateRandomString(8),
		}

		resp, err := client.Post("/api/customEvents.upsert", req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})
}
