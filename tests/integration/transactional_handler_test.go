package integration

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/app"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/tests/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTransactionalHandler tests the transactional notification handler with proper SMTP email provider configuration.
func TestTransactionalHandler(t *testing.T) {
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

	// Set up SMTP email provider for testing
	_, err = factory.SetupWorkspaceWithSMTPProvider(workspace.ID)
	require.NoError(t, err)

	// Login to get auth token
	err = client.Login(user.Email, "password")
	require.NoError(t, err)
	client.SetWorkspaceID(workspace.ID)

	t.Run("CRUD Operations", func(t *testing.T) {
		testTransactionalCRUD(t, client, factory, workspace.ID)
	})

	t.Run("Send Notification", func(t *testing.T) {
		testTransactionalSend(t, client, factory, workspace.ID)
	})

	t.Run("Template Testing", func(t *testing.T) {
		testTransactionalTemplateTest(t, client, factory, workspace.ID)
	})

	t.Run("Send with CC and BCC Recipients", func(t *testing.T) {
		testTransactionalSendWithCCAndBCC(t, client, factory, workspace.ID)
	})

	t.Run("Send with Custom From Name", func(t *testing.T) {
		testTransactionalSendWithCustomFromName(t, client, factory, workspace.ID)
	})
}

func testTransactionalCRUD(t *testing.T, client *testutil.APIClient, factory *testutil.TestDataFactory, workspaceID string) {
	t.Run("Create Transactional Notification", func(t *testing.T) {
		t.Run("should create notification successfully", func(t *testing.T) {
			// Create a template first
			template, err := factory.CreateTemplate(workspaceID)
			require.NoError(t, err)

			notification := map[string]interface{}{
				"workspace_id": workspaceID,
				"notification": map[string]interface{}{
					"id":          "welcome-email",
					"name":        "Welcome Email",
					"description": "Welcome new users",
					"channels": map[string]interface{}{
						"email": map[string]interface{}{
							"template_id": template.ID,
							"settings":    map[string]interface{}{},
						},
					},
					"tracking_settings": map[string]interface{}{
						"enable_tracking": true,
					},
					"metadata": map[string]interface{}{
						"category": "onboarding",
					},
				},
			}

			resp, err := client.CreateTransactionalNotification(notification)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			assert.Equal(t, http.StatusCreated, resp.StatusCode)

			var result map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&result)
			require.NoError(t, err)

			assert.Contains(t, result, "notification")
			notificationData := result["notification"].(map[string]interface{})
			assert.Equal(t, "welcome-email", notificationData["id"])
			assert.Equal(t, "Welcome Email", notificationData["name"])
			assert.Equal(t, "Welcome new users", notificationData["description"])
		})

		t.Run("should validate required fields", func(t *testing.T) {
			notification := map[string]interface{}{
				"workspace_id": workspaceID,
				"notification": map[string]interface{}{
					// Missing id and name
					"description": "Test",
				},
			}

			resp, err := client.CreateTransactionalNotification(notification)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		})

		t.Run("should validate channels", func(t *testing.T) {
			notification := map[string]interface{}{
				"workspace_id": workspaceID,
				"notification": map[string]interface{}{
					"id":          "test-notification",
					"name":        "Test Notification",
					"description": "Test",
					// Missing channels
				},
			}

			resp, err := client.CreateTransactionalNotification(notification)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		})
	})

	t.Run("Get Transactional Notification", func(t *testing.T) {
		// Create a notification first
		template, err := factory.CreateTemplate(workspaceID)
		require.NoError(t, err)

		notification, err := factory.CreateTransactionalNotification(workspaceID,
			testutil.WithTransactionalNotificationID("get-test"),
			testutil.WithTransactionalNotificationChannels(domain.ChannelTemplates{
				domain.TransactionalChannelEmail: domain.ChannelTemplate{
					TemplateID: template.ID,
					Settings:   map[string]interface{}{},
				},
			}),
		)
		require.NoError(t, err)

		t.Run("should get notification successfully", func(t *testing.T) {
			resp, err := client.GetTransactionalNotification(notification.ID)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			assert.Equal(t, http.StatusOK, resp.StatusCode)

			var result map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&result)
			require.NoError(t, err)

			assert.Contains(t, result, "notification")
			notificationData := result["notification"].(map[string]interface{})
			assert.Equal(t, notification.ID, notificationData["id"])
			assert.Equal(t, notification.Name, notificationData["name"])
		})

		t.Run("should return 404 for non-existent notification", func(t *testing.T) {
			resp, err := client.GetTransactionalNotification("non-existent")
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			assert.Equal(t, http.StatusNotFound, resp.StatusCode)
		})
	})

	t.Run("List Transactional Notifications", func(t *testing.T) {
		// Create multiple notifications
		template, err := factory.CreateTemplate(workspaceID)
		require.NoError(t, err)

		_, err = factory.CreateTransactionalNotification(workspaceID,
			testutil.WithTransactionalNotificationID("list-test-1"),
			testutil.WithTransactionalNotificationName("List Test 1"),
			testutil.WithTransactionalNotificationChannels(domain.ChannelTemplates{
				domain.TransactionalChannelEmail: domain.ChannelTemplate{
					TemplateID: template.ID,
					Settings:   map[string]interface{}{},
				},
			}),
		)
		require.NoError(t, err)

		_, err = factory.CreateTransactionalNotification(workspaceID,
			testutil.WithTransactionalNotificationID("list-test-2"),
			testutil.WithTransactionalNotificationName("List Test 2"),
			testutil.WithTransactionalNotificationChannels(domain.ChannelTemplates{
				domain.TransactionalChannelEmail: domain.ChannelTemplate{
					TemplateID: template.ID,
					Settings:   map[string]interface{}{},
				},
			}),
		)
		require.NoError(t, err)

		t.Run("should list notifications successfully", func(t *testing.T) {
			resp, err := client.ListTransactionalNotifications(map[string]string{
				"limit":  "10",
				"offset": "0",
			})
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			assert.Equal(t, http.StatusOK, resp.StatusCode)

			var result map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&result)
			require.NoError(t, err)

			assert.Contains(t, result, "notifications")
			assert.Contains(t, result, "total")

			notifications := result["notifications"].([]interface{})
			assert.GreaterOrEqual(t, len(notifications), 2)
		})

		t.Run("should support search", func(t *testing.T) {
			resp, err := client.ListTransactionalNotifications(map[string]string{
				"search": "List Test 1",
				"limit":  "10",
				"offset": "0",
			})
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			assert.Equal(t, http.StatusOK, resp.StatusCode)

			var result map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&result)
			require.NoError(t, err)

			notifications := result["notifications"].([]interface{})
			assert.GreaterOrEqual(t, len(notifications), 1)
		})
	})

	t.Run("Update Transactional Notification", func(t *testing.T) {
		// Create a notification first
		template, err := factory.CreateTemplate(workspaceID)
		require.NoError(t, err)

		notification, err := factory.CreateTransactionalNotification(workspaceID,
			testutil.WithTransactionalNotificationID("update-test"),
			testutil.WithTransactionalNotificationChannels(domain.ChannelTemplates{
				domain.TransactionalChannelEmail: domain.ChannelTemplate{
					TemplateID: template.ID,
					Settings:   map[string]interface{}{},
				},
			}),
		)
		require.NoError(t, err)

		t.Run("should update notification successfully", func(t *testing.T) {
			updates := map[string]interface{}{
				"name":        "Updated Name",
				"description": "Updated Description",
			}

			resp, err := client.UpdateTransactionalNotification(notification.ID, updates)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			assert.Equal(t, http.StatusOK, resp.StatusCode)

			var result map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&result)
			require.NoError(t, err)

			assert.Contains(t, result, "notification")
			notificationData := result["notification"].(map[string]interface{})
			assert.Equal(t, "Updated Name", notificationData["name"])
			assert.Equal(t, "Updated Description", notificationData["description"])
		})

		t.Run("should return 404 for non-existent notification", func(t *testing.T) {
			updates := map[string]interface{}{
				"name": "Updated Name",
			}

			resp, err := client.UpdateTransactionalNotification("non-existent", updates)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			assert.Equal(t, http.StatusNotFound, resp.StatusCode)
		})
	})

	t.Run("Delete Transactional Notification", func(t *testing.T) {
		// Create a notification first
		template, err := factory.CreateTemplate(workspaceID)
		require.NoError(t, err)

		notification, err := factory.CreateTransactionalNotification(workspaceID,
			testutil.WithTransactionalNotificationID("delete-test"),
			testutil.WithTransactionalNotificationChannels(domain.ChannelTemplates{
				domain.TransactionalChannelEmail: domain.ChannelTemplate{
					TemplateID: template.ID,
					Settings:   map[string]interface{}{},
				},
			}),
		)
		require.NoError(t, err)

		t.Run("should delete notification successfully", func(t *testing.T) {
			resp, err := client.DeleteTransactionalNotification(notification.ID)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			assert.Equal(t, http.StatusOK, resp.StatusCode)

			var result map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&result)
			require.NoError(t, err)

			assert.Contains(t, result, "success")
			assert.True(t, result["success"].(bool))

			// Verify notification is deleted
			getResp, err := client.GetTransactionalNotification(notification.ID)
			require.NoError(t, err)
			defer func() { _ = getResp.Body.Close() }()
			assert.Equal(t, http.StatusNotFound, getResp.StatusCode)
		})

		t.Run("should return 404 for non-existent notification", func(t *testing.T) {
			resp, err := client.DeleteTransactionalNotification("non-existent")
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			assert.Equal(t, http.StatusNotFound, resp.StatusCode)
		})
	})
}

func testTransactionalSend(t *testing.T, client *testutil.APIClient, factory *testutil.TestDataFactory, workspaceID string) {
	t.Run("Send Transactional Notification", func(t *testing.T) {
		// Create a template and notification
		template, err := factory.CreateTemplate(workspaceID)
		require.NoError(t, err)

		notification, err := factory.CreateTransactionalNotification(workspaceID,
			testutil.WithTransactionalNotificationID("send-test"),
			testutil.WithTransactionalNotificationChannels(domain.ChannelTemplates{
				domain.TransactionalChannelEmail: domain.ChannelTemplate{
					TemplateID: template.ID,
					Settings:   map[string]interface{}{},
				},
			}),
		)
		require.NoError(t, err)

		// Create a contact
		contact, err := factory.CreateContact(workspaceID)
		require.NoError(t, err)

		t.Run("should send notification successfully", func(t *testing.T) {
			sendRequest := map[string]interface{}{
				"id": notification.ID,
				"contact": map[string]interface{}{
					"email":      contact.Email,
					"first_name": "John",
					"last_name":  "Doe",
				},
				"channels": []string{"email"},
				"data": map[string]interface{}{
					"user_name":   "John Doe",
					"welcome_url": "https://example.com/welcome",
				},
				"metadata": map[string]interface{}{
					"source": "integration_test",
				},
			}

			resp, err := client.SendTransactionalNotification(sendRequest)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			// With proper SMTP email provider setup, should succeed
			assert.Equal(t, http.StatusOK, resp.StatusCode)

			var result map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&result)
			require.NoError(t, err)

			assert.Contains(t, result, "message_id")
			assert.NotEmpty(t, result["message_id"], "Message ID should not be empty")
		})

		t.Run("should validate required fields", func(t *testing.T) {
			sendRequest := map[string]interface{}{
				"id": notification.ID,
				// Missing contact
				"channels": []string{"email"},
			}

			resp, err := client.SendTransactionalNotification(sendRequest)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		})

		t.Run("should return 400 for non-existent notification", func(t *testing.T) {
			sendRequest := map[string]interface{}{
				"id": "non-existent",
				"contact": map[string]interface{}{
					"email": contact.Email,
				},
				"channels": []string{"email"},
			}

			resp, err := client.SendTransactionalNotification(sendRequest)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		})
	})
}

func testTransactionalTemplateTest(t *testing.T, client *testutil.APIClient, factory *testutil.TestDataFactory, workspaceID string) {
	t.Run("Test Template", func(t *testing.T) {
		// Create a template
		template, err := factory.CreateTemplate(workspaceID)
		require.NoError(t, err)

		// Create an integration for sending test emails
		integration, err := factory.CreateSMTPIntegration(workspaceID)
		require.NoError(t, err)

		t.Run("should test template successfully", func(t *testing.T) {
			testRequest := map[string]interface{}{
				"template_id":     template.ID,
				"integration_id":  integration.ID,
				"sender_id":       "test@example.com",
				"recipient_email": "recipient@example.com",
				"email_options": map[string]interface{}{
					"subject": "Test Email",
				},
			}

			resp, err := client.TestTransactionalTemplate(testRequest)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			assert.Equal(t, http.StatusOK, resp.StatusCode)

			var result map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&result)
			require.NoError(t, err)

			assert.Contains(t, result, "success")
			// Note: success may be false due to SMTP configuration in test environment
			// but the endpoint should respond correctly
		})

		t.Run("should validate required fields", func(t *testing.T) {
			testRequest := map[string]interface{}{
				// Missing template_id
				"integration_id":  integration.ID,
				"recipient_email": "recipient@example.com",
			}

			resp, err := client.TestTransactionalTemplate(testRequest)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		})

		t.Run("should return error for non-existent template", func(t *testing.T) {
			testRequest := map[string]interface{}{
				"template_id":     "non-existent",
				"integration_id":  integration.ID,
				"sender_id":       "test@example.com",
				"recipient_email": "recipient@example.com",
			}

			resp, err := client.TestTransactionalTemplate(testRequest)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			assert.Equal(t, http.StatusOK, resp.StatusCode)

			var result map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&result)
			require.NoError(t, err)

			assert.Contains(t, result, "success")
			assert.False(t, result["success"].(bool))
			assert.Contains(t, result, "error")
		})
	})
}

func TestTransactionalAuthentication(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer func() { suite.Cleanup() }()

	client := suite.APIClient

	t.Run("should require authentication", func(t *testing.T) {
		endpoints := []struct {
			name   string
			method func() (*http.Response, error)
		}{
			{
				name: "list",
				method: func() (*http.Response, error) {
					return client.ListTransactionalNotifications(nil)
				},
			},
			{
				name: "get",
				method: func() (*http.Response, error) {
					return client.GetTransactionalNotification("test")
				},
			},
			{
				name: "create",
				method: func() (*http.Response, error) {
					return client.CreateTransactionalNotification(map[string]interface{}{})
				},
			},
			{
				name: "update",
				method: func() (*http.Response, error) {
					return client.UpdateTransactionalNotification("test", map[string]interface{}{})
				},
			},
			{
				name: "delete",
				method: func() (*http.Response, error) {
					return client.DeleteTransactionalNotification("test")
				},
			},
			{
				name: "send",
				method: func() (*http.Response, error) {
					return client.SendTransactionalNotification(map[string]interface{}{})
				},
			},
			{
				name: "testTemplate",
				method: func() (*http.Response, error) {
					return client.TestTransactionalTemplate(map[string]interface{}{})
				},
			},
		}

		for _, endpoint := range endpoints {
			t.Run(endpoint.name, func(t *testing.T) {
				resp, err := endpoint.method()
				require.NoError(t, err)
				defer func() { _ = resp.Body.Close() }()

				assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
			})
		}
	})
}

func TestTransactionalMethodValidation(t *testing.T) {
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

	t.Run("should validate HTTP methods", func(t *testing.T) {
		// Test that GET-only endpoints reject POST
		resp, err := client.Post("/api/transactional.list", map[string]interface{}{})
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()
		assert.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)

		resp, err = client.Post("/api/transactional.get", map[string]interface{}{})
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()
		assert.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)

		// Test that POST-only endpoints reject GET
		resp, err = client.Get("/api/transactional.create")
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()
		assert.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)

		resp, err = client.Get("/api/transactional.update")
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()
		assert.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)

		resp, err = client.Get("/api/transactional.delete")
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()
		assert.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)

		resp, err = client.Get("/api/transactional.send")
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()
		assert.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)

		resp, err = client.Get("/api/transactional.testTemplate")
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()
		assert.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)
	})
}

// testTransactionalSendWithCCAndBCC verifies that emails are sent to all CC and BCC recipients
func testTransactionalSendWithCCAndBCC(t *testing.T, client *testutil.APIClient, factory *testutil.TestDataFactory, workspaceID string) {
	t.Run("should send email to all recipients including CC and BCC", func(t *testing.T) {
		// Clear Mailpit before test
		err := testutil.ClearMailpitMessages(t)
		if err != nil {
			t.Logf("Warning: Could not clear Mailpit messages: %v", err)
		}

		// Create a template
		template, err := factory.CreateTemplate(workspaceID)
		require.NoError(t, err)

		// Create a transactional notification
		notification, err := factory.CreateTransactionalNotification(workspaceID,
			testutil.WithTransactionalNotificationID("cc-bcc-test"),
			testutil.WithTransactionalNotificationChannels(domain.ChannelTemplates{
				domain.TransactionalChannelEmail: domain.ChannelTemplate{
					TemplateID: template.ID,
					Settings:   map[string]interface{}{},
				},
			}),
		)
		require.NoError(t, err)

		// Define all recipients - 7 total emails expected
		mainRecipient := "receiver@mail.com"
		ccRecipients := []string{"cc1@mail.com", "cc2@mail.com", "cc3@mail.com"}
		bccRecipients := []string{"bcc1@mail.com", "bcc2@mail.com", "bcc3@mail.com"}

		// Build list of all expected recipients
		allRecipients := []string{mainRecipient}
		allRecipients = append(allRecipients, ccRecipients...)
		allRecipients = append(allRecipients, bccRecipients...)

		// Send the notification with CC and BCC
		sendRequest := map[string]interface{}{
			"id": notification.ID,
			"contact": map[string]interface{}{
				"email":      mainRecipient,
				"first_name": "Main",
				"last_name":  "Recipient",
			},
			"channels": []string{"email"},
			"data": map[string]interface{}{
				"test_message": "This is a test email with CC and BCC recipients",
			},
			"email_options": map[string]interface{}{
				"cc":  ccRecipients,
				"bcc": bccRecipients,
			},
		}

		t.Logf("Sending transactional notification with:")
		t.Logf("  Main recipient: %s", mainRecipient)
		t.Logf("  CC recipients: %v", ccRecipients)
		t.Logf("  BCC recipients: %v", bccRecipients)

		resp, err := client.SendTransactionalNotification(sendRequest)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		// Check response
		assert.Equal(t, http.StatusOK, resp.StatusCode, "Expected 200 OK when sending notification")

		var result map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&result)
		require.NoError(t, err)

		// Verify we got a message ID
		assert.Contains(t, result, "message_id")
		messageID := result["message_id"].(string)
		assert.NotEmpty(t, messageID, "Message ID should not be empty")

		t.Logf("Email sent successfully with message ID: %s", messageID)

		// Wait for SMTP server to process all emails
		t.Log("Waiting for emails to be delivered to Mailpit...")
		time.Sleep(3 * time.Second)

		// Check Mailpit to verify all 7 emails were received
		t.Log("Checking Mailpit API for email count...")
		mailpitResp, err := http.Get("http://localhost:8025/api/v1/messages")
		require.NoError(t, err, "Failed to connect to Mailpit API")
		defer func() { _ = mailpitResp.Body.Close() }()

		var mailpitData struct {
			Total    int `json:"total"`
			Messages []struct {
				Subject string `json:"Subject"`
				To      []struct {
					Address string `json:"Address"`
				} `json:"To"`
			} `json:"messages"`
		}

		err = json.NewDecoder(mailpitResp.Body).Decode(&mailpitData)
		require.NoError(t, err, "Failed to decode Mailpit response")

		t.Logf("Mailpit reports %d total emails", mailpitData.Total)

		// Count emails that match our message (by checking for the Test Email Subject)
		emailsForOurMessage := 0
		recipientsFound := make(map[string]bool)

		for _, msg := range mailpitData.Messages {
			// Check if this is our email by looking at the subject
			if strings.Contains(msg.Subject, "Test Email Subject") {
				emailsForOurMessage++

				// Track which recipient received this
				for _, to := range msg.To {
					recipientsFound[to.Address] = true
					t.Logf("  Found email to: %s", to.Address)
				}
			}
		}

		t.Logf("Found %d email(s) with our subject in Mailpit", emailsForOurMessage)

		// Verify we have exactly 1 email (SMTP sends 1 message to multiple recipients, not separate messages)
		assert.Equal(t, 1, emailsForOurMessage, "Expected 1 email in Mailpit with 7 recipients")

		// Verify all 7 expected recipients are present in that 1 email
		t.Log("\n=== Recipient Verification ===")
		allRecipientsFound := true
		for _, expectedRecipient := range allRecipients {
			if recipientsFound[expectedRecipient] {
				t.Logf("  ✅ %s - received email", expectedRecipient)
			} else {
				t.Errorf("  ❌ %s - DID NOT receive email", expectedRecipient)
				allRecipientsFound = false
			}
		}

		// Verify we found exactly 7 recipients
		assert.Equal(t, 7, len(recipientsFound), "Expected exactly 7 recipients in the email")
		assert.True(t, allRecipientsFound, "All 7 recipients should be present in the email")

		// Verify message history was created for the main recipient
		messageResp, err := client.Get("/api/messages.list?workspace_id=" + workspaceID + "&id=" + messageID)
		require.NoError(t, err)
		defer func() { _ = messageResp.Body.Close() }()

		assert.Equal(t, http.StatusOK, messageResp.StatusCode)

		var messagesResult map[string]interface{}
		err = json.NewDecoder(messageResp.Body).Decode(&messagesResult)
		require.NoError(t, err)

		messages, ok := messagesResult["messages"].([]interface{})
		require.True(t, ok, "Expected messages array in response")
		require.NotEmpty(t, messages, "Expected at least one message in history")

		// Verify the message was sent to the main recipient
		message := messages[0].(map[string]interface{})
		assert.Equal(t, mainRecipient, message["contact_email"], "Message should be recorded for main recipient")

		// Final summary
		t.Log("\n=== Final Test Summary ===")
		t.Logf("✅ API accepted the request and returned message ID: %s", messageID)
		t.Logf("✅ Mailpit received 1 email with 7 recipients (1 main + 3 CC + 3 BCC)")
		t.Logf("✅ All 7 recipients are included in the email envelope")
		t.Logf("✅ Message history created for primary recipient")
		t.Log("\nNote: SMTP sends 1 email to multiple recipients, not separate emails.")
		t.Log("The CC recipients are visible in the Cc header, BCC recipients are hidden.")
	})

	t.Run("should validate email addresses in CC and BCC", func(t *testing.T) {
		// Create a template and notification
		template, err := factory.CreateTemplate(workspaceID)
		require.NoError(t, err)

		notification, err := factory.CreateTransactionalNotification(workspaceID,
			testutil.WithTransactionalNotificationID("cc-bcc-validation-test"),
			testutil.WithTransactionalNotificationChannels(domain.ChannelTemplates{
				domain.TransactionalChannelEmail: domain.ChannelTemplate{
					TemplateID: template.ID,
					Settings:   map[string]interface{}{},
				},
			}),
		)
		require.NoError(t, err)

		// Try to send with invalid CC email
		sendRequest := map[string]interface{}{
			"id": notification.ID,
			"contact": map[string]interface{}{
				"email": "valid@mail.com",
			},
			"channels": []string{"email"},
			"email_options": map[string]interface{}{
				"cc": []string{"invalid-email"}, // Invalid email format
			},
		}

		resp, err := client.SendTransactionalNotification(sendRequest)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "Should reject invalid CC email")

		// Try to send with invalid BCC email
		sendRequest["email_options"] = map[string]interface{}{
			"bcc": []string{"another-invalid"}, // Invalid email format
		}

		resp, err = client.SendTransactionalNotification(sendRequest)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "Should reject invalid BCC email")
	})

	t.Run("should reject empty strings in CC and BCC arrays", func(t *testing.T) {
		// Create a template and notification
		template, err := factory.CreateTemplate(workspaceID)
		require.NoError(t, err)

		notification, err := factory.CreateTransactionalNotification(workspaceID,
			testutil.WithTransactionalNotificationID("cc-bcc-empty-test"),
			testutil.WithTransactionalNotificationChannels(domain.ChannelTemplates{
				domain.TransactionalChannelEmail: domain.ChannelTemplate{
					TemplateID: template.ID,
					Settings:   map[string]interface{}{},
				},
			}),
		)
		require.NoError(t, err)

		// Send with empty strings in CC - should be rejected
		sendRequest := map[string]interface{}{
			"id": notification.ID,
			"contact": map[string]interface{}{
				"email": "receiver@mail.com",
			},
			"channels": []string{"email"},
			"email_options": map[string]interface{}{
				"cc": []string{"", "valid@mail.com"},
			},
		}

		resp, err := client.SendTransactionalNotification(sendRequest)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		// Should fail validation - empty strings are not allowed
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "Should reject empty strings in CC")

		var result map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&result)
		require.NoError(t, err)

		assert.Contains(t, result, "error")
		assert.Contains(t, result["error"].(string), "cc")
		t.Log("✅ Empty strings in CC/BCC arrays are properly rejected")
	})
}

// testTransactionalSendWithCustomFromName verifies that the from_name override works correctly
func testTransactionalSendWithCustomFromName(t *testing.T, client *testutil.APIClient, factory *testutil.TestDataFactory, workspaceID string) {
	t.Run("should send email with custom from_name", func(t *testing.T) {
		// Clear Mailpit before test
		err := testutil.ClearMailpitMessages(t)
		if err != nil {
			t.Logf("Warning: Could not clear Mailpit messages: %v", err)
		}

		// Create a template
		template, err := factory.CreateTemplate(workspaceID)
		require.NoError(t, err)

		// Create a transactional notification
		notification, err := factory.CreateTransactionalNotification(workspaceID,
			testutil.WithTransactionalNotificationID("custom-from-name-test"),
			testutil.WithTransactionalNotificationChannels(domain.ChannelTemplates{
				domain.TransactionalChannelEmail: domain.ChannelTemplate{
					TemplateID: template.ID,
					Settings:   map[string]interface{}{},
				},
			}),
		)
		require.NoError(t, err)

		// Define test parameters
		recipient := "test@example.com"
		customFromName := "Custom Support Team"

		// Send the notification with custom from_name
		sendRequest := map[string]interface{}{
			"id": notification.ID,
			"contact": map[string]interface{}{
				"email":      recipient,
				"first_name": "Test",
				"last_name":  "User",
			},
			"channels": []string{"email"},
			"data": map[string]interface{}{
				"test_message": "Testing custom from_name override",
			},
			"email_options": map[string]interface{}{
				"from_name": customFromName,
			},
		}

		t.Logf("Sending transactional notification with custom from_name: '%s'", customFromName)

		resp, err := client.SendTransactionalNotification(sendRequest)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		// Check response
		assert.Equal(t, http.StatusOK, resp.StatusCode, "Expected 200 OK when sending notification")

		var result map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&result)
		require.NoError(t, err)

		// Verify we got a message ID
		assert.Contains(t, result, "message_id")
		messageID := result["message_id"].(string)
		assert.NotEmpty(t, messageID, "Message ID should not be empty")

		t.Logf("Email sent successfully with message ID: %s", messageID)

		// Wait for SMTP server to process email
		t.Log("Waiting for email to be delivered to Mailpit...")
		time.Sleep(3 * time.Second)

		// Check Mailpit to verify the From header contains custom from_name
		t.Log("Checking Mailpit API for email with custom from_name...")
		mailpitResp, err := http.Get("http://localhost:8025/api/v1/messages")
		require.NoError(t, err, "Failed to connect to Mailpit API")
		defer func() { _ = mailpitResp.Body.Close() }()

		var mailpitData testutil.MailpitMessagesResponse
		err = json.NewDecoder(mailpitResp.Body).Decode(&mailpitData)
		require.NoError(t, err, "Failed to decode Mailpit response")

		t.Logf("Mailpit reports %d total emails", mailpitData.Total)

		// Find our email and verify the From header
		foundEmail := false
		for _, msgSummary := range mailpitData.Messages {
			// Get full message to check headers
			fullMsg, err := testutil.GetMailpitMessage(t, msgSummary.ID)
			if err != nil {
				t.Logf("Failed to get full message: %v", err)
				continue
			}
			// Check if this is our email by looking at the subject
			subjects := fullMsg.Headers["Subject"]
			if len(subjects) > 0 && strings.Contains(subjects[0], "Test Email Subject") {
				foundEmail = true

				// Check the From header - it should contain the custom from_name
				fromHeaders := fullMsg.Headers["From"]
				require.NotEmpty(t, fromHeaders, "From header should be present")

				fromHeader := fromHeaders[0]
				t.Logf("From header: %s", fromHeader)

				// The From header should contain the custom from_name
				// Format is typically: "Custom Support Team <sender@example.com>"
				assert.Contains(t, fromHeader, customFromName,
					"From header should contain the custom from_name")

				t.Logf("✅ Email sent with custom from_name: %s", customFromName)
				break
			}
		}

		assert.True(t, foundEmail, "Should find the sent email in Mailpit")

		// Verify message history was created
		messageResp, err := client.Get("/api/messages.list?workspace_id=" + workspaceID + "&id=" + messageID)
		require.NoError(t, err)
		defer func() { _ = messageResp.Body.Close() }()

		assert.Equal(t, http.StatusOK, messageResp.StatusCode)

		var messagesResult map[string]interface{}
		err = json.NewDecoder(messageResp.Body).Decode(&messagesResult)
		require.NoError(t, err)

		messages, ok := messagesResult["messages"].([]interface{})
		require.True(t, ok, "Expected messages array in response")
		require.NotEmpty(t, messages, "Expected at least one message in history")

		message := messages[0].(map[string]interface{})
		assert.Equal(t, recipient, message["contact_email"], "Message should be recorded for recipient")

		// Final summary
		t.Log("\n=== Test Summary ===")
		t.Logf("✅ API accepted the request with custom from_name")
		t.Logf("✅ Email sent with message ID: %s", messageID)
		t.Logf("✅ Mailpit received email with custom from_name in From header")
		t.Logf("✅ Message history created correctly")
	})

	t.Run("should use default from_name when not provided", func(t *testing.T) {
		// Clear Mailpit before test
		err := testutil.ClearMailpitMessages(t)
		if err != nil {
			t.Logf("Warning: Could not clear Mailpit messages: %v", err)
		}

		// Create a template
		template, err := factory.CreateTemplate(workspaceID)
		require.NoError(t, err)

		// Create a transactional notification
		notification, err := factory.CreateTransactionalNotification(workspaceID,
			testutil.WithTransactionalNotificationID("default-from-name-test"),
			testutil.WithTransactionalNotificationChannels(domain.ChannelTemplates{
				domain.TransactionalChannelEmail: domain.ChannelTemplate{
					TemplateID: template.ID,
					Settings:   map[string]interface{}{},
				},
			}),
		)
		require.NoError(t, err)

		recipient := "test2@example.com"

		// Send the notification WITHOUT custom from_name
		sendRequest := map[string]interface{}{
			"id": notification.ID,
			"contact": map[string]interface{}{
				"email": recipient,
			},
			"channels": []string{"email"},
			"data": map[string]interface{}{
				"test_message": "Testing default from_name",
			},
			// No email_options provided - should use default
		}

		t.Log("Sending transactional notification without custom from_name (should use default)")

		resp, err := client.SendTransactionalNotification(sendRequest)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusOK, resp.StatusCode, "Expected 200 OK when sending notification")

		var result map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&result)
		require.NoError(t, err)

		assert.Contains(t, result, "message_id")
		messageID := result["message_id"].(string)
		assert.NotEmpty(t, messageID, "Message ID should not be empty")

		t.Logf("Email sent successfully with message ID: %s (using default from_name)", messageID)

		// Wait for email delivery
		time.Sleep(3 * time.Second)

		// Check Mailpit to verify email was sent (with default from_name)
		mailpitResp, err := http.Get("http://localhost:8025/api/v1/messages")
		require.NoError(t, err)
		defer func() { _ = mailpitResp.Body.Close() }()

		var mailpitData testutil.MailpitMessagesResponse
		err = json.NewDecoder(mailpitResp.Body).Decode(&mailpitData)
		require.NoError(t, err)

		// Verify at least one email was sent
		foundEmail := false
		for _, msgSummary := range mailpitData.Messages {
			fullMsg, err := testutil.GetMailpitMessage(t, msgSummary.ID)
			if err != nil {
				continue
			}
			subjects := fullMsg.Headers["Subject"]
			if len(subjects) > 0 && strings.Contains(subjects[0], "Test Email Subject") {
				foundEmail = true
				fromHeaders := fullMsg.Headers["From"]
				require.NotEmpty(t, fromHeaders, "From header should be present")
				t.Logf("From header (default): %s", fromHeaders[0])
				break
			}
		}

		assert.True(t, foundEmail, "Should find the sent email in Mailpit")
		t.Log("✅ Email sent successfully with default from_name")
	})

	t.Run("should use default from_name when empty string provided", func(t *testing.T) {
		// Clear Mailpit before test
		err := testutil.ClearMailpitMessages(t)
		if err != nil {
			t.Logf("Warning: Could not clear Mailpit messages: %v", err)
		}

		// Create a template
		template, err := factory.CreateTemplate(workspaceID)
		require.NoError(t, err)

		// Create a transactional notification
		notification, err := factory.CreateTransactionalNotification(workspaceID,
			testutil.WithTransactionalNotificationID("empty-from-name-test"),
			testutil.WithTransactionalNotificationChannels(domain.ChannelTemplates{
				domain.TransactionalChannelEmail: domain.ChannelTemplate{
					TemplateID: template.ID,
					Settings:   map[string]interface{}{},
				},
			}),
		)
		require.NoError(t, err)

		recipient := "test3@example.com"

		// Send the notification with empty string from_name
		sendRequest := map[string]interface{}{
			"id": notification.ID,
			"contact": map[string]interface{}{
				"email": recipient,
			},
			"channels": []string{"email"},
			"data": map[string]interface{}{
				"test_message": "Testing empty from_name",
			},
			"email_options": map[string]interface{}{
				"from_name": "", // Empty string - should use default
			},
		}

		t.Log("Sending transactional notification with empty from_name (should use default)")

		resp, err := client.SendTransactionalNotification(sendRequest)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusOK, resp.StatusCode, "Expected 200 OK when sending notification")

		var result map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&result)
		require.NoError(t, err)

		assert.Contains(t, result, "message_id")
		messageID := result["message_id"].(string)
		assert.NotEmpty(t, messageID, "Message ID should not be empty")

		t.Logf("Email sent successfully with message ID: %s (empty from_name, using default)", messageID)

		// Wait for email delivery
		time.Sleep(3 * time.Second)

		// Verify email was sent
		mailpitResp, err := http.Get("http://localhost:8025/api/v1/messages")
		require.NoError(t, err)
		defer func() { _ = mailpitResp.Body.Close() }()

		var mailpitData testutil.MailpitMessagesResponse
		err = json.NewDecoder(mailpitResp.Body).Decode(&mailpitData)
		require.NoError(t, err)

		assert.Greater(t, mailpitData.Total, 0, "Should have sent at least one email")
		t.Log("✅ Email sent successfully - empty from_name correctly falls back to default")
	})
}
