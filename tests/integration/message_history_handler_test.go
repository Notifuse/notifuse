package integration

import (
	"context"
	"encoding/json"
	"io"
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

func TestMessageHistoryHandler(t *testing.T) {
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

	t.Run("Messages List Endpoint", func(t *testing.T) {
		testMessagesList(t, client, factory, workspace.ID)
	})

	t.Run("Broadcast Stats Endpoint", func(t *testing.T) {
		testBroadcastStats(t, client, factory, workspace.ID)
	})
}

func testMessagesList(t *testing.T, client *testutil.APIClient, factory *testutil.TestDataFactory, workspaceID string) {
	t.Run("GET /api/messages.list", func(t *testing.T) {
		t.Run("should return empty list when no messages exist", func(t *testing.T) {
			resp, err := client.Get("/api/messages.list", map[string]string{
				"workspace_id": workspaceID,
			})
			require.NoError(t, err)
			defer func() { _ = _ = resp.Body.Close() }()

			assert.Equal(t, http.StatusOK, resp.StatusCode)

			var result domain.MessageListResult
			err = json.NewDecoder(resp.Body).Decode(&result)
			require.NoError(t, err)

			assert.Empty(t, result.Messages)
			assert.Empty(t, result.NextCursor)
			assert.False(t, result.HasMore)
		})

		t.Run("should return 400 when workspace_id is missing", func(t *testing.T) {
			// Clear workspace_id from client to test missing workspace_id scenario
			originalWorkspaceID := client.GetWorkspaceID()
			client.SetWorkspaceID("")
			defer client.SetWorkspaceID(originalWorkspaceID) // Restore for other tests

			resp, err := client.Get("/api/messages.list")
			require.NoError(t, err)
			defer func() { _ = _ = resp.Body.Close() }()

			// Debug: print response status and body
			body, _ := io.ReadAll(resp.Body)
			t.Logf("Response status: %d, body: %s", resp.StatusCode, string(body))

			assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		})

		t.Run("should return 405 for non-GET methods", func(t *testing.T) {
			resp, err := client.Post("/api/messages.list", nil, map[string]string{
				"workspace_id": workspaceID,
			})
			require.NoError(t, err)
			defer func() { _ = _ = resp.Body.Close() }()

			assert.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)
		})

		t.Run("should handle pagination parameters", func(t *testing.T) {
			// Create some test messages
			contact, err := factory.CreateContact(workspaceID)
			require.NoError(t, err)
			template, err := factory.CreateTemplate(workspaceID)
			require.NoError(t, err)
			broadcast, err := factory.CreateBroadcast(workspaceID)
			require.NoError(t, err)

			// Create multiple messages
			for i := 0; i < 5; i++ {
				_, err := factory.CreateMessageHistory(workspaceID, testutil.WithMessageContact(contact.Email),
					testutil.WithMessageTemplate(template.ID), testutil.WithMessageBroadcast(broadcast.ID))
				require.NoError(t, err)
			}

			// Test with limit
			resp, err := client.Get("/api/messages.list", map[string]string{
				"workspace_id": workspaceID,
				"limit":        "3",
			})
			require.NoError(t, err)
			defer func() { _ = _ = resp.Body.Close() }()

			assert.Equal(t, http.StatusOK, resp.StatusCode)

			var result domain.MessageListResult
			err = json.NewDecoder(resp.Body).Decode(&result)
			require.NoError(t, err)

			assert.Len(t, result.Messages, 3)
			assert.True(t, result.HasMore)
			assert.NotEmpty(t, result.NextCursor)
		})

		t.Run("should handle filter parameters", func(t *testing.T) {
			// Create contact and template
			contact, err := factory.CreateContact(workspaceID)
			require.NoError(t, err)
			template, err := factory.CreateTemplate(workspaceID)
			require.NoError(t, err)

			// Create messages with different channels
			_, err = factory.CreateMessageHistory(workspaceID,
				testutil.WithMessageContact(contact.Email),
				testutil.WithMessageTemplate(template.ID),
				testutil.WithMessageChannel("email"))
			require.NoError(t, err)

			_, err = factory.CreateMessageHistory(workspaceID,
				testutil.WithMessageContact(contact.Email),
				testutil.WithMessageTemplate(template.ID),
				testutil.WithMessageChannel("sms"))
			require.NoError(t, err)

			// Filter by channel
			resp, err := client.Get("/api/messages.list", map[string]string{
				"workspace_id": workspaceID,
				"channel":      "email",
			})
			require.NoError(t, err)
			defer func() { _ = _ = resp.Body.Close() }()

			assert.Equal(t, http.StatusOK, resp.StatusCode)

			var result domain.MessageListResult
			err = json.NewDecoder(resp.Body).Decode(&result)
			require.NoError(t, err)

			// Should only return email messages
			for _, msg := range result.Messages {
				assert.Equal(t, "email", msg.Channel)
			}
		})

		t.Run("should validate filter parameters", func(t *testing.T) {
			// Test invalid channel
			resp, err := client.Get("/api/messages.list", map[string]string{
				"workspace_id": workspaceID,
				"channel":      "invalid_channel",
			})
			require.NoError(t, err)
			defer func() { _ = _ = resp.Body.Close() }()

			assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		})

		t.Run("should handle boolean filters", func(t *testing.T) {
			// Create contact and template
			contact, err := factory.CreateContact(workspaceID)
			require.NoError(t, err)
			template, err := factory.CreateTemplate(workspaceID)
			require.NoError(t, err)

			// Create messages with different statuses
			_, err = factory.CreateMessageHistory(workspaceID,
				testutil.WithMessageContact(contact.Email),
				testutil.WithMessageTemplate(template.ID),
				testutil.WithMessageDelivered(true))
			require.NoError(t, err)

			_, err = factory.CreateMessageHistory(workspaceID,
				testutil.WithMessageContact(contact.Email),
				testutil.WithMessageTemplate(template.ID),
				testutil.WithMessageDelivered(false))
			require.NoError(t, err)

			// Filter by delivered status
			resp, err := client.Get("/api/messages.list", map[string]string{
				"workspace_id": workspaceID,
				"is_delivered": "true",
			})
			require.NoError(t, err)
			defer func() { _ = _ = resp.Body.Close() }()

			assert.Equal(t, http.StatusOK, resp.StatusCode)

			var result domain.MessageListResult
			err = json.NewDecoder(resp.Body).Decode(&result)
			require.NoError(t, err)

			// Should only return delivered messages
			for _, msg := range result.Messages {
				assert.NotNil(t, msg.DeliveredAt)
			}
		})

		t.Run("should handle time range filters", func(t *testing.T) {
			// Create contact and template
			contact, err := factory.CreateContact(workspaceID)
			require.NoError(t, err)
			template, err := factory.CreateTemplate(workspaceID)
			require.NoError(t, err)

			now := time.Now().UTC()

			// Create messages with different sent times
			_, err = factory.CreateMessageHistory(workspaceID,
				testutil.WithMessageContact(contact.Email),
				testutil.WithMessageTemplate(template.ID),
				testutil.WithMessageSentAt(now.Add(-2*time.Hour)))
			require.NoError(t, err)

			_, err = factory.CreateMessageHistory(workspaceID,
				testutil.WithMessageContact(contact.Email),
				testutil.WithMessageTemplate(template.ID),
				testutil.WithMessageSentAt(now.Add(-1*time.Hour)))
			require.NoError(t, err)

			// Filter by time range
			resp, err := client.Get("/api/messages.list", map[string]string{
				"workspace_id": workspaceID,
				"sent_after":   now.Add(-90 * time.Minute).Format(time.RFC3339),
				"sent_before":  now.Format(time.RFC3339),
			})
			require.NoError(t, err)
			defer func() { _ = _ = resp.Body.Close() }()

			assert.Equal(t, http.StatusOK, resp.StatusCode)

			var result domain.MessageListResult
			err = json.NewDecoder(resp.Body).Decode(&result)
			require.NoError(t, err)

			// Should only return messages within the time range
			for _, msg := range result.Messages {
				assert.True(t, msg.SentAt.After(now.Add(-90*time.Minute)))
				assert.True(t, msg.SentAt.Before(now))
			}
		})

		t.Run("should handle invalid time format", func(t *testing.T) {
			resp, err := client.Get("/api/messages.list", map[string]string{
				"workspace_id": workspaceID,
				"sent_after":   "invalid-time-format",
			})
			require.NoError(t, err)
			defer func() { _ = _ = resp.Body.Close() }()

			assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		})
	})
}

func testBroadcastStats(t *testing.T, client *testutil.APIClient, factory *testutil.TestDataFactory, workspaceID string) {
	t.Run("GET /api/messages.broadcastStats", func(t *testing.T) {
		t.Run("should return stats for existing broadcast", func(t *testing.T) {
			// Create test data
			contact, err := factory.CreateContact(workspaceID)
			require.NoError(t, err)
			template, err := factory.CreateTemplate(workspaceID)
			require.NoError(t, err)
			broadcast, err := factory.CreateBroadcast(workspaceID)
			require.NoError(t, err)

			// Create messages with different statuses
			_, err = factory.CreateMessageHistory(workspaceID,
				testutil.WithMessageContact(contact.Email),
				testutil.WithMessageTemplate(template.ID),
				testutil.WithMessageBroadcast(broadcast.ID),
				testutil.WithMessageDelivered(true))
			require.NoError(t, err)

			_, err = factory.CreateMessageHistory(workspaceID,
				testutil.WithMessageContact(contact.Email),
				testutil.WithMessageTemplate(template.ID),
				testutil.WithMessageBroadcast(broadcast.ID),
				testutil.WithMessageOpened(true))
			require.NoError(t, err)

			resp, err := client.Get("/api/messages.broadcastStats", map[string]string{
				"workspace_id": workspaceID,
				"broadcast_id": broadcast.ID,
			})
			require.NoError(t, err)
			defer func() { _ = _ = resp.Body.Close() }()

			assert.Equal(t, http.StatusOK, resp.StatusCode)

			var result map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&result)
			require.NoError(t, err)

			assert.Equal(t, broadcast.ID, result["broadcast_id"])
			assert.Contains(t, result, "stats")

			stats, ok := result["stats"].(map[string]interface{})
			require.True(t, ok)
			assert.Contains(t, stats, "total_sent")
			assert.Contains(t, stats, "total_delivered")
			assert.Contains(t, stats, "total_opened")
		})

		t.Run("should return 400 when broadcast_id is missing", func(t *testing.T) {
			resp, err := client.Get("/api/messages.broadcastStats", map[string]string{
				"workspace_id": workspaceID,
			})
			require.NoError(t, err)
			defer func() { _ = _ = resp.Body.Close() }()

			assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		})

		t.Run("should return 400 when workspace_id is missing", func(t *testing.T) {
			// Clear workspace_id from client to test missing workspace_id scenario
			originalWorkspaceID := client.GetWorkspaceID()
			client.SetWorkspaceID("")
			defer client.SetWorkspaceID(originalWorkspaceID) // Restore for other tests

			resp, err := client.Get("/api/messages.broadcastStats", map[string]string{
				"broadcast_id": "test-broadcast-id",
			})
			require.NoError(t, err)
			defer func() { _ = _ = resp.Body.Close() }()

			// Debug: print response status and body
			body, _ := io.ReadAll(resp.Body)
			t.Logf("Response status: %d, body: %s", resp.StatusCode, string(body))

			assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		})

		t.Run("should return 405 for non-GET methods", func(t *testing.T) {
			resp, err := client.Post("/api/messages.broadcastStats", nil, map[string]string{
				"workspace_id": workspaceID,
				"broadcast_id": "test-broadcast-id",
			})
			require.NoError(t, err)
			defer func() { _ = _ = resp.Body.Close() }()

			assert.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)
		})

		t.Run("should handle non-existent broadcast", func(t *testing.T) {
			resp, err := client.Get("/api/messages.broadcastStats", map[string]string{
				"workspace_id": workspaceID,
				"broadcast_id": "non-existent-broadcast-id",
			})
			require.NoError(t, err)
			defer func() { _ = _ = resp.Body.Close() }()

			// Should return OK with empty stats
			assert.Equal(t, http.StatusOK, resp.StatusCode)

			var result map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&result)
			require.NoError(t, err)

			assert.Equal(t, "non-existent-broadcast-id", result["broadcast_id"])
			assert.Contains(t, result, "stats")
		})
	})
}

func TestMessageHistoryAuthentication(t *testing.T) {
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

	t.Run("should require authentication", func(t *testing.T) {
		// Don't login, make requests without auth
		client.SetToken("")

		t.Run("messages.list", func(t *testing.T) {
			resp, err := client.Get("/api/messages.list", map[string]string{
				"workspace_id": workspace.ID,
			})
			require.NoError(t, err)
			defer func() { _ = _ = resp.Body.Close() }()

			assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		})

		t.Run("messages.broadcastStats", func(t *testing.T) {
			resp, err := client.Get("/api/messages.broadcastStats", map[string]string{
				"workspace_id": workspace.ID,
				"broadcast_id": "test-broadcast-id",
			})
			require.NoError(t, err)
			defer func() { _ = _ = resp.Body.Close() }()

			assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		})
	})
}

func TestMessageHistoryDataFactory(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer _ = suite.Cleanup()

	factory := suite.DataFactory
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)

	t.Run("CreateMessageHistory", func(t *testing.T) {
		contact, err := factory.CreateContact(workspace.ID)
		require.NoError(t, err)
		template, err := factory.CreateTemplate(workspace.ID)
		require.NoError(t, err)

		message, err := factory.CreateMessageHistory(workspace.ID,
			testutil.WithMessageContact(contact.Email),
			testutil.WithMessageTemplate(template.ID))
		require.NoError(t, err)
		require.NotNil(t, message)

		assert.NotEmpty(t, message.ID)
		assert.Equal(t, contact.Email, message.ContactEmail)
		assert.Equal(t, template.ID, message.TemplateID)
		assert.Equal(t, "email", message.Channel) // default channel
		assert.NotZero(t, message.SentAt)
		assert.NotZero(t, message.CreatedAt)
		assert.NotZero(t, message.UpdatedAt)
	})

	t.Run("CreateMessageHistory with options", func(t *testing.T) {
		contact, err := factory.CreateContact(workspace.ID)
		require.NoError(t, err)
		template, err := factory.CreateTemplate(workspace.ID)
		require.NoError(t, err)
		broadcast, err := factory.CreateBroadcast(workspace.ID)
		require.NoError(t, err)

		now := time.Now().UTC()
		message, err := factory.CreateMessageHistory(workspace.ID,
			testutil.WithMessageContact(contact.Email),
			testutil.WithMessageTemplate(template.ID),
			testutil.WithMessageBroadcast(broadcast.ID),
			testutil.WithMessageChannel("sms"),
			testutil.WithMessageSentAt(now),
			testutil.WithMessageDelivered(true),
			testutil.WithMessageOpened(true))
		require.NoError(t, err)

		assert.Equal(t, contact.Email, message.ContactEmail)
		assert.Equal(t, template.ID, message.TemplateID)
		assert.Equal(t, broadcast.ID, *message.BroadcastID)
		assert.Equal(t, "sms", message.Channel)
		assert.Equal(t, now.Format(time.RFC3339), message.SentAt.Format(time.RFC3339))
		assert.NotNil(t, message.DeliveredAt)
		assert.NotNil(t, message.OpenedAt)
	})

	t.Run("CreateMessageHistory persisted to database", func(t *testing.T) {
		contact, err := factory.CreateContact(workspace.ID)
		require.NoError(t, err)
		template, err := factory.CreateTemplate(workspace.ID)
		require.NoError(t, err)

		message, err := factory.CreateMessageHistory(workspace.ID,
			testutil.WithMessageContact(contact.Email),
			testutil.WithMessageTemplate(template.ID))
		require.NoError(t, err)

		// Verify message exists in database using repository
		app := suite.ServerManager.GetApp()
		messageHistoryRepo := app.GetMessageHistoryRepository()
		workspaceRepo := app.GetWorkspaceRepository()
		ws, err := workspaceRepo.GetByID(context.Background(), workspace.ID)
		require.NoError(t, err)

		retrievedMessage, err := messageHistoryRepo.Get(context.Background(), workspace.ID, ws.Settings.SecretKey, message.ID)
		require.NoError(t, err)
		require.NotNil(t, retrievedMessage)
		assert.Equal(t, contact.Email, retrievedMessage.ContactEmail)
		assert.Equal(t, template.ID, retrievedMessage.TemplateID)
	})
}

func TestMessageDataEncryption(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer _ = suite.Cleanup()

	factory := suite.DataFactory
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)

	t.Run("Message data is encrypted in database", func(t *testing.T) {
		contact, err := factory.CreateContact(workspace.ID)
		require.NoError(t, err)
		template, err := factory.CreateTemplate(workspace.ID)
		require.NoError(t, err)

		// Create message with specific data to verify encryption
		testData := map[string]interface{}{
			"email":      "test@example.com",
			"first_name": "John",
			"last_name":  "Doe",
			"token":      "secret-token-12345",
			"amount":     99.99,
		}

		message, err := factory.CreateMessageHistory(workspace.ID,
			testutil.WithMessageContact(contact.Email),
			testutil.WithMessageTemplate(template.ID),
			func(m *domain.MessageHistory) {
				m.MessageData.Data = testData
			})
		require.NoError(t, err)
		require.NotNil(t, message)

		// Query the database directly to verify data is encrypted
		app := suite.ServerManager.GetApp()
		workspaceRepo := app.GetWorkspaceRepository()
		workspaceDB, err := workspaceRepo.GetConnection(context.Background(), workspace.ID)
		require.NoError(t, err)

		var rawMessageData json.RawMessage
		query := `SELECT message_data FROM message_history WHERE id = $1`
		err = workspaceDB.QueryRowContext(context.Background(), query, message.ID).Scan(&rawMessageData)
		require.NoError(t, err)

		// Parse the raw JSON to verify it contains _encrypted key
		var messageData map[string]interface{}
		err = json.Unmarshal(rawMessageData, &messageData)
		require.NoError(t, err)

		// Verify the data field contains the _encrypted marker
		data, ok := messageData["data"].(map[string]interface{})
		require.True(t, ok, "message_data.data should be a map")

		encryptedValue, hasEncrypted := data["_encrypted"]
		assert.True(t, hasEncrypted, "message_data.data should contain _encrypted key")
		assert.NotEmpty(t, encryptedValue, "_encrypted value should not be empty")

		// Verify it's a string (hex-encoded encrypted data)
		encryptedStr, ok := encryptedValue.(string)
		assert.True(t, ok, "_encrypted value should be a string")
		assert.Greater(t, len(encryptedStr), 50, "encrypted data should be substantial")

		// Verify metadata is NOT encrypted (should be plaintext)
		metadata, ok := messageData["metadata"].(map[string]interface{})
		assert.True(t, ok, "metadata should exist")
		// Metadata should not have _encrypted key
		_, hasEncryptedMetadata := metadata["_encrypted"]
		assert.False(t, hasEncryptedMetadata, "metadata should not be encrypted")
	})

	t.Run("Encrypted data is properly decrypted when retrieved", func(t *testing.T) {
		contact, err := factory.CreateContact(workspace.ID)
		require.NoError(t, err)
		template, err := factory.CreateTemplate(workspace.ID)
		require.NoError(t, err)

		// Create message with specific test data
		originalData := map[string]interface{}{
			"email":       "user@example.com",
			"first_name":  "Jane",
			"last_name":   "Smith",
			"api_key":     "sk_test_abcdef123456",
			"amount":      250.50,
			"items":       []interface{}{"item1", "item2", "item3"},
			"nested_data": map[string]interface{}{"key": "value", "count": float64(42)},
		}

		message, err := factory.CreateMessageHistory(workspace.ID,
			testutil.WithMessageContact(contact.Email),
			testutil.WithMessageTemplate(template.ID),
			func(m *domain.MessageHistory) {
				m.MessageData.Data = originalData
				m.MessageData.Metadata = map[string]interface{}{
					"source":     "test",
					"test_value": true,
				}
			})
		require.NoError(t, err)

		// Retrieve the message using the repository (should decrypt automatically)
		app := suite.ServerManager.GetApp()
		messageHistoryRepo := app.GetMessageHistoryRepository()
		workspaceRepo := app.GetWorkspaceRepository()
		ws, err := workspaceRepo.GetByID(context.Background(), workspace.ID)
		require.NoError(t, err)

		retrievedMessage, err := messageHistoryRepo.Get(context.Background(), workspace.ID, ws.Settings.SecretKey, message.ID)
		require.NoError(t, err)
		require.NotNil(t, retrievedMessage)

		// Verify the decrypted data matches the original
		assert.Equal(t, originalData["email"], retrievedMessage.MessageData.Data["email"])
		assert.Equal(t, originalData["first_name"], retrievedMessage.MessageData.Data["first_name"])
		assert.Equal(t, originalData["last_name"], retrievedMessage.MessageData.Data["last_name"])
		assert.Equal(t, originalData["api_key"], retrievedMessage.MessageData.Data["api_key"])
		assert.Equal(t, originalData["amount"], retrievedMessage.MessageData.Data["amount"])

		// Verify complex nested structures are preserved
		items, ok := retrievedMessage.MessageData.Data["items"].([]interface{})
		assert.True(t, ok)
		assert.Len(t, items, 3)
		assert.Equal(t, "item1", items[0])

		nestedData, ok := retrievedMessage.MessageData.Data["nested_data"].(map[string]interface{})
		assert.True(t, ok)
		assert.Equal(t, "value", nestedData["key"])
		assert.Equal(t, float64(42), nestedData["count"])

		// Verify metadata is preserved and not encrypted
		assert.Equal(t, "test", retrievedMessage.MessageData.Metadata["source"])
		assert.Equal(t, true, retrievedMessage.MessageData.Metadata["test_value"])
	})

	t.Run("Backward compatibility: unencrypted messages can still be read", func(t *testing.T) {
		contact, err := factory.CreateContact(workspace.ID)
		require.NoError(t, err)
		template, err := factory.CreateTemplate(workspace.ID)
		require.NoError(t, err)

		// Insert an old-style unencrypted message directly into the database
		app := suite.ServerManager.GetApp()
		workspaceRepo := app.GetWorkspaceRepository()
		workspaceDB, err := workspaceRepo.GetConnection(context.Background(), workspace.ID)
		require.NoError(t, err)

		messageID := "legacy-msg-" + time.Now().Format("20060102150405")
		legacyData := map[string]interface{}{
			"subject": "Old Message",
			"body":    "This is legacy data",
			"token":   "legacy-token-xyz",
		}

		messageData := domain.MessageData{
			Data: legacyData,
			Metadata: map[string]interface{}{
				"legacy": true,
			},
		}

		messageDataJSON, err := json.Marshal(messageData)
		require.NoError(t, err)

		now := time.Now().UTC()
		insertQuery := `
			INSERT INTO message_history (
				id, contact_email, template_id, template_version, channel,
				message_data, sent_at, created_at, updated_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		`
		_, err = workspaceDB.ExecContext(context.Background(), insertQuery,
			messageID, contact.Email, template.ID, 1, "email",
			messageDataJSON, now, now, now)
		require.NoError(t, err)

		// Retrieve the legacy message (should work without decryption)
		messageHistoryRepo := app.GetMessageHistoryRepository()
		ws, err := workspaceRepo.GetByID(context.Background(), workspace.ID)
		require.NoError(t, err)

		retrievedMessage, err := messageHistoryRepo.Get(context.Background(), workspace.ID, ws.Settings.SecretKey, messageID)
		require.NoError(t, err)
		require.NotNil(t, retrievedMessage)

		// Verify the legacy data is readable
		assert.Equal(t, "Old Message", retrievedMessage.MessageData.Data["subject"])
		assert.Equal(t, "This is legacy data", retrievedMessage.MessageData.Data["body"])
		assert.Equal(t, "legacy-token-xyz", retrievedMessage.MessageData.Data["token"])
		assert.Equal(t, true, retrievedMessage.MessageData.Metadata["legacy"])

		// Verify there's no _encrypted key in the decrypted data
		_, hasEncrypted := retrievedMessage.MessageData.Data["_encrypted"]
		assert.False(t, hasEncrypted, "decrypted data should not contain _encrypted key")
	})
}
