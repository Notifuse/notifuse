package integration

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/app"
	"github.com/Notifuse/notifuse/tests/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWebhookSubscriptionE2E tests the complete webhook subscription flow end-to-end via API
// This test covers:
// - Full CRUD operations on webhook subscriptions
// - Input validation for webhook subscriptions
// - Secret regeneration
// - Enable/disable functionality
// - Custom event filters
// - Event types listing
func TestWebhookSubscriptionE2E(t *testing.T) {
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

	// Set up authentication
	err = client.Login(user.Email, "password")
	require.NoError(t, err)
	client.SetWorkspaceID(workspace.ID)

	t.Run("Webhook Subscription CRUD Operations", func(t *testing.T) {
		testWebhookSubscriptionCRUD(t, client, workspace.ID)
	})

	t.Run("Webhook Subscription Validation", func(t *testing.T) {
		testWebhookSubscriptionValidation(t, client, workspace.ID)
	})

	t.Run("Webhook Subscription with Custom Event Filters", func(t *testing.T) {
		testWebhookSubscriptionCustomFilters(t, client, workspace.ID)
	})

	t.Run("Multiple Webhook Subscriptions", func(t *testing.T) {
		testMultipleWebhookSubscriptions(t, client, workspace.ID)
	})
}

// testWebhookSubscriptionCRUD tests full CRUD operations on webhook subscriptions
func testWebhookSubscriptionCRUD(t *testing.T, client *testutil.APIClient, workspaceID string) {
	var subscriptionID string
	var originalSecret string

	// CREATE - Test creating a webhook subscription
	t.Run("Create Webhook Subscription", func(t *testing.T) {
		createReq := map[string]interface{}{
			"workspace_id": workspaceID,
			"name":         "Test Webhook Subscription",
			"url":          "https://example.com/webhook",
			"description":  "Test webhook for integration testing",
			"event_types":  []string{"contact.created", "contact.updated", "email.sent"},
		}

		resp, err := client.Post("/api/webhookSubscriptions.create", createReq)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusCreated, resp.StatusCode)

		var response map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&response)
		require.NoError(t, err)

		subscription := response["subscription"].(map[string]interface{})
		subscriptionID = subscription["id"].(string)
		originalSecret = subscription["secret"].(string)

		assert.NotEmpty(t, subscriptionID)
		assert.Equal(t, "Test Webhook Subscription", subscription["name"])
		assert.Equal(t, "https://example.com/webhook", subscription["url"])
		assert.NotEmpty(t, originalSecret, "Secret should be generated")
		assert.Equal(t, true, subscription["enabled"], "Webhook should be enabled by default")

		// Verify event types
		eventTypes := subscription["event_types"].([]interface{})
		assert.Len(t, eventTypes, 3)
		assert.Contains(t, eventTypes, "contact.created")
		assert.Contains(t, eventTypes, "contact.updated")
		assert.Contains(t, eventTypes, "email.sent")
	})

	// READ - Test getting a webhook subscription
	t.Run("Get Webhook Subscription", func(t *testing.T) {
		resp, err := client.Get("/api/webhookSubscriptions.get", map[string]string{
			"workspace_id": workspaceID,
			"id":           subscriptionID,
		})
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var response map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&response)
		require.NoError(t, err)

		subscription := response["subscription"].(map[string]interface{})
		assert.Equal(t, subscriptionID, subscription["id"])
		assert.Equal(t, "Test Webhook Subscription", subscription["name"])
	})

	// LIST - Test listing webhook subscriptions
	t.Run("List Webhook Subscriptions", func(t *testing.T) {
		resp, err := client.Get("/api/webhookSubscriptions.list", map[string]string{
			"workspace_id": workspaceID,
		})
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var response map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&response)
		require.NoError(t, err)

		subscriptions := response["subscriptions"].([]interface{})
		assert.GreaterOrEqual(t, len(subscriptions), 1, "Should have at least one subscription")

		// Find our subscription
		var found bool
		for _, sub := range subscriptions {
			subMap := sub.(map[string]interface{})
			if subMap["id"].(string) == subscriptionID {
				found = true
				assert.Equal(t, "Test Webhook Subscription", subMap["name"])
				break
			}
		}
		assert.True(t, found, "Should find created subscription in list")
	})

	// UPDATE - Test updating a webhook subscription
	t.Run("Update Webhook Subscription", func(t *testing.T) {
		updateReq := map[string]interface{}{
			"workspace_id": workspaceID,
			"id":           subscriptionID,
			"name":         "Updated Webhook Name",
			"url":          "https://updated.example.com/webhook",
			"description":  "Updated description",
			"event_types":  []string{"contact.created", "contact.deleted", "email.delivered"},
			"enabled":      true,
		}

		resp, err := client.Post("/api/webhookSubscriptions.update", updateReq)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var response map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&response)
		require.NoError(t, err)

		subscription := response["subscription"].(map[string]interface{})
		assert.Equal(t, "Updated Webhook Name", subscription["name"])
		assert.Equal(t, "https://updated.example.com/webhook", subscription["url"])
		assert.Equal(t, originalSecret, subscription["secret"], "Secret should not change on update")

		eventTypes := subscription["event_types"].([]interface{})
		assert.Len(t, eventTypes, 3)
		assert.Contains(t, eventTypes, "contact.created")
		assert.Contains(t, eventTypes, "contact.deleted")
		assert.Contains(t, eventTypes, "email.delivered")
	})

	// TOGGLE - Test enabling/disabling a webhook subscription
	t.Run("Toggle Webhook Subscription", func(t *testing.T) {
		// Disable the webhook
		toggleReq := map[string]interface{}{
			"workspace_id": workspaceID,
			"id":           subscriptionID,
			"enabled":      false,
		}

		resp, err := client.Post("/api/webhookSubscriptions.toggle", toggleReq)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var response map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&response)
		require.NoError(t, err)

		subscription := response["subscription"].(map[string]interface{})
		assert.Equal(t, false, subscription["enabled"])

		// Re-enable the webhook
		toggleReq["enabled"] = true
		resp, err = client.Post("/api/webhookSubscriptions.toggle", toggleReq)
		require.NoError(t, err)
		defer resp.Body.Close()

		err = json.NewDecoder(resp.Body).Decode(&response)
		require.NoError(t, err)

		subscription = response["subscription"].(map[string]interface{})
		assert.Equal(t, true, subscription["enabled"])
	})

	// REGENERATE SECRET - Test regenerating webhook secret
	t.Run("Regenerate Webhook Secret", func(t *testing.T) {
		regenerateReq := map[string]interface{}{
			"workspace_id": workspaceID,
			"id":           subscriptionID,
		}

		resp, err := client.Post("/api/webhookSubscriptions.regenerateSecret", regenerateReq)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var response map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&response)
		require.NoError(t, err)

		subscription := response["subscription"].(map[string]interface{})
		newSecret := subscription["secret"].(string)

		assert.NotEmpty(t, newSecret)
		assert.NotEqual(t, originalSecret, newSecret, "Secret should be different after regeneration")
		assert.Greater(t, len(newSecret), 40, "Secret should be sufficiently long (base64 encoded 32 bytes)")
	})

	// GET EVENT TYPES - Test getting available event types
	t.Run("Get Event Types", func(t *testing.T) {
		resp, err := client.Get("/api/webhookSubscriptions.eventTypes", map[string]string{
			"workspace_id": workspaceID,
		})
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var response map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&response)
		require.NoError(t, err)

		eventTypes := response["event_types"].([]interface{})
		assert.Greater(t, len(eventTypes), 0)

		// Verify some expected event types exist
		expectedTypes := []string{
			"contact.created", "contact.updated", "contact.deleted",
			"email.sent", "email.delivered", "email.opened",
			"list.subscribed", "list.unsubscribed",
			"custom_event.created",
		}

		for _, expected := range expectedTypes {
			assert.Contains(t, eventTypes, expected)
		}
	})

	// DELETE - Test deleting a webhook subscription
	t.Run("Delete Webhook Subscription", func(t *testing.T) {
		deleteReq := map[string]interface{}{
			"workspace_id": workspaceID,
			"id":           subscriptionID,
		}

		resp, err := client.Post("/api/webhookSubscriptions.delete", deleteReq)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		// Verify it's deleted
		getResp, err := client.Get("/api/webhookSubscriptions.get", map[string]string{
			"workspace_id": workspaceID,
			"id":           subscriptionID,
		})
		require.NoError(t, err)
		defer getResp.Body.Close()

		assert.Equal(t, http.StatusNotFound, getResp.StatusCode)
	})
}

// testWebhookSubscriptionValidation tests input validation for webhook subscriptions
func testWebhookSubscriptionValidation(t *testing.T, client *testutil.APIClient, workspaceID string) {
	t.Run("Empty Name Validation", func(t *testing.T) {
		createReq := map[string]interface{}{
			"workspace_id": workspaceID,
			"name":         "",
			"url":          "https://example.com/webhook",
			"event_types":  []string{"contact.created"},
		}

		resp, err := client.Post("/api/webhookSubscriptions.create", createReq)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("Invalid URL Validation", func(t *testing.T) {
		invalidURLs := []string{
			"",
			"not-a-url",
			"ftp://example.com",
			"ws://example.com",
			"https://",
		}

		for _, invalidURL := range invalidURLs {
			createReq := map[string]interface{}{
				"workspace_id": workspaceID,
				"name":         "Test Webhook",
				"url":          invalidURL,
				"event_types":  []string{"contact.created"},
			}

			resp, err := client.Post("/api/webhookSubscriptions.create", createReq)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "Should reject invalid URL: %s", invalidURL)
		}
	})

	t.Run("Empty Event Types Validation", func(t *testing.T) {
		createReq := map[string]interface{}{
			"workspace_id": workspaceID,
			"name":         "Test Webhook",
			"url":          "https://example.com/webhook",
			"event_types":  []string{},
		}

		resp, err := client.Post("/api/webhookSubscriptions.create", createReq)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("Invalid Event Type Validation", func(t *testing.T) {
		createReq := map[string]interface{}{
			"workspace_id": workspaceID,
			"name":         "Test Webhook",
			"url":          "https://example.com/webhook",
			"event_types":  []string{"contact.created", "invalid.event.type"},
		}

		resp, err := client.Post("/api/webhookSubscriptions.create", createReq)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("Valid HTTP and HTTPS URLs", func(t *testing.T) {
		validURLs := []string{
			"https://example.com/webhook",
			"http://example.com/webhook",
			"https://example.com:8080/webhook",
			"https://api.example.com/webhooks?token=abc123",
		}

		for i, validURL := range validURLs {
			createReq := map[string]interface{}{
				"workspace_id": workspaceID,
				"name":         "Test Webhook " + string(rune('A'+i)),
				"url":          validURL,
				"event_types":  []string{"contact.created"},
			}

			resp, err := client.Post("/api/webhookSubscriptions.create", createReq)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusCreated, resp.StatusCode, "Should accept valid URL: %s", validURL)

			// Clean up
			var response map[string]interface{}
			json.NewDecoder(resp.Body).Decode(&response)
			if subscription, ok := response["subscription"].(map[string]interface{}); ok {
				subID := subscription["id"].(string)
				deleteReq := map[string]interface{}{
					"workspace_id": workspaceID,
					"id":           subID,
				}
				delResp, _ := client.Post("/api/webhookSubscriptions.delete", deleteReq)
				if delResp != nil {
					delResp.Body.Close()
				}
			}
		}
	})
}

// testWebhookSubscriptionCustomFilters tests webhook subscriptions with custom event filters
func testWebhookSubscriptionCustomFilters(t *testing.T, client *testutil.APIClient, workspaceID string) {
	t.Run("Create with Custom Event Filters", func(t *testing.T) {
		createReq := map[string]interface{}{
			"workspace_id": workspaceID,
			"name":         "Custom Filter Webhook",
			"url":          "https://example.com/custom-webhook",
			"description":  "Webhook with custom event filters",
			"event_types":  []string{"custom_event.created", "custom_event.updated"},
			"custom_event_filters": map[string]interface{}{
				"goal_types":  []string{"conversion", "engagement"},
				"event_names": []string{"purchase", "signup", "trial_started"},
			},
		}

		resp, err := client.Post("/api/webhookSubscriptions.create", createReq)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusCreated, resp.StatusCode)

		var response map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&response)
		require.NoError(t, err)

		subscription := response["subscription"].(map[string]interface{})
		subscriptionID := subscription["id"].(string)

		// Verify custom event filters
		require.NotNil(t, subscription["custom_event_filters"])
		filters := subscription["custom_event_filters"].(map[string]interface{})

		goalTypes := filters["goal_types"].([]interface{})
		assert.Len(t, goalTypes, 2)
		assert.Contains(t, goalTypes, "conversion")
		assert.Contains(t, goalTypes, "engagement")

		eventNames := filters["event_names"].([]interface{})
		assert.Len(t, eventNames, 3)
		assert.Contains(t, eventNames, "purchase")
		assert.Contains(t, eventNames, "signup")
		assert.Contains(t, eventNames, "trial_started")

		// Clean up
		deleteReq := map[string]interface{}{
			"workspace_id": workspaceID,
			"id":           subscriptionID,
		}
		delResp, _ := client.Post("/api/webhookSubscriptions.delete", deleteReq)
		if delResp != nil {
			delResp.Body.Close()
		}
	})
}

// testMultipleWebhookSubscriptions tests creating and managing multiple webhook subscriptions
func testMultipleWebhookSubscriptions(t *testing.T, client *testutil.APIClient, workspaceID string) {
	subscriptionIDs := make([]string, 0)

	t.Run("Create Multiple Subscriptions", func(t *testing.T) {
		for i := 1; i <= 5; i++ {
			createReq := map[string]interface{}{
				"workspace_id": workspaceID,
				"name":         "Webhook " + string(rune('A'+i-1)),
				"url":          "https://example.com/webhook" + string(rune('0'+i)),
				"event_types":  []string{"contact.created"},
			}

			resp, err := client.Post("/api/webhookSubscriptions.create", createReq)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusCreated, resp.StatusCode)

			var response map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&response)
			require.NoError(t, err)

			subscription := response["subscription"].(map[string]interface{})
			subscriptionIDs = append(subscriptionIDs, subscription["id"].(string))
		}
	})

	t.Run("List All Subscriptions", func(t *testing.T) {
		resp, err := client.Get("/api/webhookSubscriptions.list", map[string]string{
			"workspace_id": workspaceID,
		})
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var response map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&response)
		require.NoError(t, err)

		subscriptions := response["subscriptions"].([]interface{})
		assert.GreaterOrEqual(t, len(subscriptions), 5, "Should have at least 5 subscriptions")

		// Verify all our subscriptions are present
		foundCount := 0
		for _, sub := range subscriptions {
			subMap := sub.(map[string]interface{})
			subID := subMap["id"].(string)
			for _, createdID := range subscriptionIDs {
				if subID == createdID {
					foundCount++
					break
				}
			}
		}
		assert.Equal(t, 5, foundCount, "Should find all created subscriptions")
	})

	t.Run("Verify Unique IDs and Secrets", func(t *testing.T) {
		ids := make(map[string]bool)
		secrets := make(map[string]bool)

		for _, subID := range subscriptionIDs {
			resp, err := client.Get("/api/webhookSubscriptions.get", map[string]string{
				"workspace_id": workspaceID,
				"id":           subID,
			})
			require.NoError(t, err)
			defer resp.Body.Close()

			var response map[string]interface{}
			json.NewDecoder(resp.Body).Decode(&response)

			subscription := response["subscription"].(map[string]interface{})
			id := subscription["id"].(string)
			secret := subscription["secret"].(string)

			assert.False(t, ids[id], "ID should be unique: %s", id)
			assert.False(t, secrets[secret], "Secret should be unique: %s", secret)

			ids[id] = true
			secrets[secret] = true
		}
	})

	// Clean up all created subscriptions
	t.Run("Delete All Subscriptions", func(t *testing.T) {
		for _, subID := range subscriptionIDs {
			deleteReq := map[string]interface{}{
				"workspace_id": workspaceID,
				"id":           subID,
			}

			resp, err := client.Post("/api/webhookSubscriptions.delete", deleteReq)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusOK, resp.StatusCode)
		}

		// Verify all deleted
		for _, subID := range subscriptionIDs {
			resp, err := client.Get("/api/webhookSubscriptions.get", map[string]string{
				"workspace_id": workspaceID,
				"id":           subID,
			})
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusNotFound, resp.StatusCode)
		}
	})
}

// TestWebhookPayloadAndSignatureVerification tests the complete webhook delivery flow
// including payload structure and HMAC signature verification from the receiver's perspective.
// This test demonstrates how a webhook consumer should verify incoming webhooks.
func TestWebhookPayloadAndSignatureVerification(t *testing.T) {
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

	// Set up authentication
	err = client.Login(user.Email, "password")
	require.NoError(t, err)
	client.SetWorkspaceID(workspace.ID)

	t.Run("Verify Webhook Signature and Payload Structure", func(t *testing.T) {
		// Channel to capture webhook data
		type webhookCapture struct {
			headers http.Header
			body    []byte
		}
		captured := make(chan webhookCapture, 1)

		// Create a test HTTP server that simulates a webhook receiver
		testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}

			captured <- webhookCapture{
				headers: r.Header.Clone(),
				body:    body,
			}

			w.WriteHeader(http.StatusOK)
			w.Write([]byte("OK"))
		}))
		defer testServer.Close()

		// Create a webhook subscription pointing to our test server
		createReq := map[string]interface{}{
			"workspace_id": workspace.ID,
			"name":         "Signature Test Webhook",
			"url":          testServer.URL,
			"event_types":  []string{"contact.created"},
		}

		resp, err := client.Post("/api/webhookSubscriptions.create", createReq)
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusCreated, resp.StatusCode)

		var createResponse map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&createResponse)
		require.NoError(t, err)

		subscription := createResponse["subscription"].(map[string]interface{})
		subscriptionID := subscription["id"].(string)
		secret := subscription["secret"].(string)

		// Clean up at the end
		defer func() {
			deleteReq := map[string]interface{}{
				"workspace_id": workspace.ID,
				"id":           subscriptionID,
			}
			delResp, _ := client.Post("/api/webhookSubscriptions.delete", deleteReq)
			if delResp != nil {
				delResp.Body.Close()
			}
		}()

		// Send a test webhook
		testReq := map[string]interface{}{
			"workspace_id": workspace.ID,
			"id":           subscriptionID,
			"event_type":   "contact.created",
		}

		testResp, err := client.Post("/api/webhookSubscriptions.test", testReq)
		require.NoError(t, err)
		defer testResp.Body.Close()
		require.Equal(t, http.StatusOK, testResp.StatusCode)

		// Wait for webhook to be captured
		var cap webhookCapture
		select {
		case cap = <-captured:
		case <-time.After(5 * time.Second):
			t.Fatal("Timeout waiting for webhook delivery")
		}

		// === VERIFY HEADERS ===
		t.Run("Headers Present", func(t *testing.T) {
			assert.Equal(t, "application/json", cap.headers.Get("Content-Type"), "Content-Type should be application/json")
			assert.NotEmpty(t, cap.headers.Get("Webhook-Id"), "webhook-id header should be present")
			assert.NotEmpty(t, cap.headers.Get("Webhook-Timestamp"), "webhook-timestamp header should be present")
			assert.NotEmpty(t, cap.headers.Get("Webhook-Signature"), "webhook-signature header should be present")
		})

		// === VERIFY PAYLOAD STRUCTURE ===
		t.Run("Payload Structure", func(t *testing.T) {
			var payload map[string]interface{}
			err := json.Unmarshal(cap.body, &payload)
			require.NoError(t, err, "Payload should be valid JSON")

			// Verify required envelope fields
			assert.NotEmpty(t, payload["id"], "Payload should have 'id' field")
			assert.Equal(t, "contact.created", payload["type"], "Payload should have correct 'type' field")
			assert.Equal(t, workspace.ID, payload["workspace_id"], "Payload should have correct 'workspace_id' field")
			assert.NotEmpty(t, payload["timestamp"], "Payload should have 'timestamp' field")
			assert.NotNil(t, payload["data"], "Payload should have 'data' field")

			// Verify timestamp is valid RFC3339
			timestampStr, ok := payload["timestamp"].(string)
			require.True(t, ok, "Timestamp should be a string")
			_, err = time.Parse(time.RFC3339, timestampStr)
			assert.NoError(t, err, "Timestamp should be valid RFC3339 format")

			// Verify data contains expected test contact fields
			data, ok := payload["data"].(map[string]interface{})
			require.True(t, ok, "Data should be an object")
			assert.NotEmpty(t, data["email"], "Data should contain email field")
		})

		// === VERIFY SIGNATURE (RECEIVER'S PERSPECTIVE) ===
		t.Run("Signature Verification", func(t *testing.T) {
			webhookID := cap.headers.Get("Webhook-Id")
			timestampStr := cap.headers.Get("Webhook-Timestamp")
			signatureHeader := cap.headers.Get("Webhook-Signature")

			// Parse timestamp
			timestamp, err := strconv.ParseInt(timestampStr, 10, 64)
			require.NoError(t, err, "Timestamp should be a valid integer")

			// Verify timestamp is not too old (within 5 minutes for test)
			now := time.Now().Unix()
			assert.LessOrEqual(t, now-timestamp, int64(300), "Timestamp should be within 5 minutes")

			// Extract signature from header (format: "v1,<base64_signature>")
			assert.True(t, strings.HasPrefix(signatureHeader, "v1,"), "Signature should start with 'v1,'")
			receivedSignature := strings.TrimPrefix(signatureHeader, "v1,")

			// Compute expected signature using the same algorithm as Notifuse
			// signedContent = "{msgID}.{timestamp}.{payload}"
			signedContent := fmt.Sprintf("%s.%d.%s", webhookID, timestamp, string(cap.body))
			h := hmac.New(sha256.New, []byte(secret))
			h.Write([]byte(signedContent))
			expectedSignature := base64.StdEncoding.EncodeToString(h.Sum(nil))

			// Verify signatures match
			assert.Equal(t, expectedSignature, receivedSignature, "HMAC signature should match")

			// Also verify using constant-time comparison (recommended for production)
			decodedReceived, err := base64.StdEncoding.DecodeString(receivedSignature)
			require.NoError(t, err, "Received signature should be valid base64")
			decodedExpected, err := base64.StdEncoding.DecodeString(expectedSignature)
			require.NoError(t, err, "Expected signature should be valid base64")
			assert.True(t, hmac.Equal(decodedReceived, decodedExpected), "Signatures should match using constant-time comparison")
		})

		// === VERIFY SIGNATURE FAILS WITH WRONG SECRET ===
		t.Run("Signature Fails With Wrong Secret", func(t *testing.T) {
			webhookID := cap.headers.Get("Webhook-Id")
			timestampStr := cap.headers.Get("Webhook-Timestamp")
			signatureHeader := cap.headers.Get("Webhook-Signature")

			timestamp, _ := strconv.ParseInt(timestampStr, 10, 64)
			receivedSignature := strings.TrimPrefix(signatureHeader, "v1,")

			// Compute signature with wrong secret
			wrongSecret := "wrong-secret-12345"
			signedContent := fmt.Sprintf("%s.%d.%s", webhookID, timestamp, string(cap.body))
			h := hmac.New(sha256.New, []byte(wrongSecret))
			h.Write([]byte(signedContent))
			wrongSignature := base64.StdEncoding.EncodeToString(h.Sum(nil))

			assert.NotEqual(t, wrongSignature, receivedSignature, "Signature with wrong secret should not match")
		})

		// === VERIFY SIGNATURE FAILS WITH TAMPERED PAYLOAD ===
		t.Run("Signature Fails With Tampered Payload", func(t *testing.T) {
			webhookID := cap.headers.Get("Webhook-Id")
			timestampStr := cap.headers.Get("Webhook-Timestamp")
			signatureHeader := cap.headers.Get("Webhook-Signature")

			timestamp, _ := strconv.ParseInt(timestampStr, 10, 64)
			receivedSignature := strings.TrimPrefix(signatureHeader, "v1,")

			// Compute signature with tampered payload
			tamperedPayload := `{"id":"tampered","type":"contact.created","data":{"email":"hacker@evil.com"}}`
			signedContent := fmt.Sprintf("%s.%d.%s", webhookID, timestamp, tamperedPayload)
			h := hmac.New(sha256.New, []byte(secret))
			h.Write([]byte(signedContent))
			tamperedSignature := base64.StdEncoding.EncodeToString(h.Sum(nil))

			assert.NotEqual(t, tamperedSignature, receivedSignature, "Signature with tampered payload should not match")
		})
	})

	t.Run("Verify Multiple Event Types Have Correct Payloads", func(t *testing.T) {
		// Track received webhooks
		var mu sync.Mutex
		receivedWebhooks := make(map[string]map[string]interface{})

		testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			var payload map[string]interface{}
			json.Unmarshal(body, &payload)

			mu.Lock()
			if eventType, ok := payload["type"].(string); ok {
				receivedWebhooks[eventType] = payload
			}
			mu.Unlock()

			w.WriteHeader(http.StatusOK)
		}))
		defer testServer.Close()

		// Create webhook subscription for multiple event types
		createReq := map[string]interface{}{
			"workspace_id": workspace.ID,
			"name":         "Multi-Event Webhook",
			"url":          testServer.URL,
			"event_types":  []string{"contact.created", "email.sent", "list.subscribed"},
		}

		resp, err := client.Post("/api/webhookSubscriptions.create", createReq)
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusCreated, resp.StatusCode)

		var createResponse map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&createResponse)
		subscription := createResponse["subscription"].(map[string]interface{})
		subscriptionID := subscription["id"].(string)

		defer func() {
			deleteReq := map[string]interface{}{
				"workspace_id": workspace.ID,
				"id":           subscriptionID,
			}
			delResp, _ := client.Post("/api/webhookSubscriptions.delete", deleteReq)
			if delResp != nil {
				delResp.Body.Close()
			}
		}()

		// Test each event type
		eventTypes := []string{"contact.created", "email.sent", "list.subscribed"}
		for _, eventType := range eventTypes {
			testReq := map[string]interface{}{
				"workspace_id": workspace.ID,
				"id":           subscriptionID,
				"event_type":   eventType,
			}

			testResp, err := client.Post("/api/webhookSubscriptions.test", testReq)
			require.NoError(t, err)
			testResp.Body.Close()

			// Give time for webhook to be delivered
			time.Sleep(100 * time.Millisecond)
		}

		// Wait a bit more for all webhooks to arrive
		time.Sleep(500 * time.Millisecond)

		// Verify each event type was received with correct structure
		mu.Lock()
		defer mu.Unlock()

		for _, eventType := range eventTypes {
			payload, exists := receivedWebhooks[eventType]
			assert.True(t, exists, "Should have received webhook for event type: %s", eventType)
			if exists {
				assert.Equal(t, eventType, payload["type"], "Event type should match")
				assert.NotEmpty(t, payload["id"], "Payload should have ID")
				assert.Equal(t, workspace.ID, payload["workspace_id"], "Workspace ID should match")
				assert.NotNil(t, payload["data"], "Payload should have data")
			}
		}
	})
}

// verifyWebhookSignature is a helper function demonstrating how to verify webhook signatures.
// This can be used as a reference implementation for webhook consumers.
func verifyWebhookSignature(webhookID, timestampStr, signatureHeader, secret string, payload []byte) (bool, error) {
	// Parse timestamp
	timestamp, err := strconv.ParseInt(timestampStr, 10, 64)
	if err != nil {
		return false, fmt.Errorf("invalid timestamp: %w", err)
	}

	// Check timestamp is not too old (5 minutes tolerance)
	now := time.Now().Unix()
	if now-timestamp > 300 {
		return false, fmt.Errorf("webhook timestamp too old")
	}

	// Extract signature
	if !strings.HasPrefix(signatureHeader, "v1,") {
		return false, fmt.Errorf("invalid signature format")
	}
	receivedSig := strings.TrimPrefix(signatureHeader, "v1,")

	// Compute expected signature
	signedContent := fmt.Sprintf("%s.%d.%s", webhookID, timestamp, string(payload))
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(signedContent))
	expectedSig := base64.StdEncoding.EncodeToString(h.Sum(nil))

	// Constant-time comparison
	decodedReceived, err := base64.StdEncoding.DecodeString(receivedSig)
	if err != nil {
		return false, fmt.Errorf("invalid signature encoding: %w", err)
	}
	decodedExpected, _ := base64.StdEncoding.DecodeString(expectedSig)

	return hmac.Equal(decodedReceived, decodedExpected), nil
}
