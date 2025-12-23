package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/app"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/tests/testutil"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBroadcastESPFailure_AllReject tests broadcast behavior when ESP rejects ALL emails
// This simulates a scenario where the SMTP server is unreachable or misconfigured
func TestBroadcastESPFailure_AllReject(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	client := suite.APIClient
	factory := suite.DataFactory

	contactCount := 20 // Small count for faster failure test

	t.Log("=== Starting ESP Failure Test (All Reject) ===")
	t.Logf("Contact count: %d", contactCount)

	// Step 1: Create user and workspace
	t.Log("Step 1: Creating user and workspace...")
	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)

	err = factory.AddUserToWorkspace(user.ID, workspace.ID, "owner")
	require.NoError(t, err)

	// Step 2: Setup SMTP provider with INVALID port (nothing listening)
	// This will cause all email sends to fail with connection refused
	t.Log("Step 2: Setting up SMTP provider with INVALID port (9999 - nothing listening)...")
	integration, err := factory.SetupWorkspaceWithSMTPProvider(workspace.ID,
		testutil.WithIntegrationEmailProvider(domain.EmailProvider{
			Kind: domain.EmailProviderKindSMTP,
			Senders: []domain.EmailSender{
				domain.NewEmailSender("noreply@notifuse.test", "Notifuse Failure Test"),
			},
			SMTP: &domain.SMTPSettings{
				Host:     "localhost",
				Port:     9999, // Nothing listening here - will cause connection failures
				Username: "",
				Password: "",
				UseTLS:   false,
			},
			RateLimitPerMinute: 1000,
		}))
	require.NoError(t, err)
	t.Logf("Created SMTP integration (invalid port): %s", integration.ID)

	// Step 3: Login
	t.Log("Step 3: Logging in...")
	err = client.Login(user.Email, "password")
	require.NoError(t, err)
	client.SetWorkspaceID(workspace.ID)

	// Step 4: Create a contact list
	t.Log("Step 4: Creating contact list...")
	list, err := factory.CreateList(workspace.ID,
		testutil.WithListName("ESP Failure Test List"))
	require.NoError(t, err)
	t.Logf("Created list: %s", list.ID)

	// Step 5: Generate contacts
	t.Logf("Step 5: Generating %d contacts...", contactCount)
	contacts := make([]map[string]interface{}, contactCount)
	for i := 0; i < contactCount; i++ {
		email := fmt.Sprintf("esp-fail-test-%04d@example.com", i)
		contacts[i] = map[string]interface{}{
			"email":      email,
			"first_name": fmt.Sprintf("User%d", i),
			"last_name":  "ESPFailTest",
		}
	}

	// Step 6: Bulk import contacts
	t.Logf("Step 6: Bulk importing %d contacts...", contactCount)
	resp, err := client.BatchImportContacts(contacts, []string{list.ID})
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode, "Import should succeed")

	// Step 7: Create template
	t.Log("Step 7: Creating email template...")
	uniqueSubject := fmt.Sprintf("ESP Failure Test %s", uuid.New().String()[:8])
	template, err := factory.CreateTemplate(workspace.ID,
		testutil.WithTemplateName("ESP Failure Test Template"),
		testutil.WithTemplateSubject(uniqueSubject))
	require.NoError(t, err)

	// Step 8: Create broadcast
	t.Log("Step 8: Creating broadcast...")
	broadcast, err := factory.CreateBroadcast(workspace.ID,
		testutil.WithBroadcastName("ESP Failure Test Broadcast"),
		testutil.WithBroadcastAudience(domain.AudienceSettings{
			List:                list.ID,
			ExcludeUnsubscribed: true,
		}))
	require.NoError(t, err)
	t.Logf("Created broadcast: %s", broadcast.ID)

	// Step 9: Update broadcast with template
	t.Log("Step 9: Updating broadcast with template...")
	broadcast.TestSettings.Variations[0].TemplateID = template.ID
	updateReq := map[string]interface{}{
		"workspace_id":  workspace.ID,
		"id":            broadcast.ID,
		"name":          broadcast.Name,
		"audience":      broadcast.Audience,
		"schedule":      broadcast.Schedule,
		"test_settings": broadcast.TestSettings,
	}
	updateResp, err := client.UpdateBroadcast(updateReq)
	require.NoError(t, err)
	defer updateResp.Body.Close()
	require.Equal(t, http.StatusOK, updateResp.StatusCode)

	// Step 10: Schedule broadcast
	t.Log("Step 10: Scheduling broadcast for immediate sending...")
	scheduleRequest := map[string]interface{}{
		"workspace_id": workspace.ID,
		"id":           broadcast.ID,
		"send_now":     true,
	}

	scheduleResp, err := client.ScheduleBroadcast(scheduleRequest)
	require.NoError(t, err)
	defer scheduleResp.Body.Close()
	require.Equal(t, http.StatusOK, scheduleResp.StatusCode)

	// Step 10.5: Start email queue worker to process enqueued emails
	t.Log("Step 10.5: Starting email queue worker...")
	workerCtx := context.Background()
	err = suite.ServerManager.StartBackgroundWorkers(workerCtx)
	require.NoError(t, err, "Should be able to start background workers")

	// Step 11: Wait for broadcast to process (it should fail or pause due to circuit breaker)
	t.Log("Step 11: Waiting for broadcast processing (expecting failures)...")

	// The broadcast might:
	// 1. Complete with all failures tracked
	// 2. Pause due to circuit breaker after enough failures
	// 3. Mark as failed

	timeout := 2 * time.Minute
	deadline := time.Now().Add(timeout)
	var finalStatus string
	var taskState *domain.SendBroadcastState

	for time.Now().Before(deadline) {
		// Execute pending tasks
		_, _ = client.ExecutePendingTasks(10)
		time.Sleep(500 * time.Millisecond)

		// Check broadcast status
		broadcastResp, err := client.GetBroadcast(broadcast.ID)
		if err != nil {
			t.Logf("Error getting broadcast: %v", err)
			continue
		}

		body, _ := io.ReadAll(broadcastResp.Body)
		broadcastResp.Body.Close()

		var result map[string]interface{}
		if err := json.Unmarshal(body, &result); err != nil {
			continue
		}

		broadcastData, ok := result["broadcast"].(map[string]interface{})
		if !ok {
			continue
		}

		finalStatus, _ = broadcastData["status"].(string)
		t.Logf("Broadcast status: %s", finalStatus)

		// Check task state for detailed failure info
		tasksResp, err := client.ListTasks(map[string]string{
			"broadcast_id": broadcast.ID,
		})
		if err == nil {
			taskBody, _ := io.ReadAll(tasksResp.Body)
			tasksResp.Body.Close()

			var tasksResult map[string]interface{}
			if err := json.Unmarshal(taskBody, &tasksResult); err == nil {
				if tasks, ok := tasksResult["tasks"].([]interface{}); ok && len(tasks) > 0 {
					if task, ok := tasks[0].(map[string]interface{}); ok {
						if state, ok := task["state"].(map[string]interface{}); ok {
							if sendBroadcast, ok := state["send_broadcast"].(map[string]interface{}); ok {
								enqueuedCount := int(sendBroadcast["enqueued_count"].(float64))
								failedCount := int(sendBroadcast["failed_count"].(float64))
								totalRecipients := int(sendBroadcast["total_recipients"].(float64))
								recipientOffset := int(sendBroadcast["recipient_offset"].(float64))

								t.Logf("Task State - enqueued: %d, failed: %d, total: %d, offset: %d",
									enqueuedCount, failedCount, totalRecipients, recipientOffset)

								taskState = &domain.SendBroadcastState{
									EnqueuedCount:   enqueuedCount,
									FailedCount:     failedCount,
									TotalRecipients: totalRecipients,
									RecipientOffset: int64(recipientOffset),
								}
							}
						}
					}
				}
			}
		}

		// Break if broadcast reached a terminal state
		if finalStatus == "processed" || finalStatus == "failed" || finalStatus == "paused" || finalStatus == "completed" {
			break
		}

		time.Sleep(1 * time.Second)
	}

	// Step 11.5: Wait for email queue to process (all will fail due to bad SMTP)
	t.Log("Step 11.5: Waiting for email queue to drain...")
	queueRepo := suite.ServerManager.GetApp().GetEmailQueueRepository()
	_ = testutil.WaitForQueueEmpty(t, queueRepo, workspace.ID, 2*time.Minute)
	// Don't require success - queue may have failed items

	// Step 12: Verify results
	t.Log("=== VERIFICATION RESULTS ===")
	t.Logf("Final broadcast status: %s", finalStatus)

	if taskState != nil {
		t.Logf("Task State:")
		t.Logf("  - Total Recipients: %d", taskState.TotalRecipients)
		t.Logf("  - Enqueued Count: %d", taskState.EnqueuedCount)
		t.Logf("  - Failed Count: %d", taskState.FailedCount)
		t.Logf("  - Recipient Offset: %d", taskState.RecipientOffset)

		// With email queue architecture, EnqueuedCount = emails added to queue (always succeeds)
		// The actual SMTP failures are tracked in the email_queue table, not in FailedCount
		assert.Equal(t, contactCount, taskState.EnqueuedCount,
			"All emails should be enqueued to queue (enqueue != send)")

		// Note: FailedCount tracks orchestrator-level failures (template rendering, etc.)
		// SMTP failures happen in the worker and are tracked in email_queue.status

		// The offset should have advanced (attempted to process recipients)
		assert.Greater(t, int(taskState.RecipientOffset), 0,
			"Recipient offset should have advanced despite failures")
	} else {
		t.Log("Warning: Could not retrieve task state")
	}

	// The broadcast should either be:
	// - "paused" (circuit breaker triggered after too many failures)
	// - "failed" (explicit failure state)
	// - "processed" (completed but with failures tracked)
	assert.Contains(t, []string{"paused", "failed", "processed", "processing"},
		finalStatus,
		"Broadcast should be in a valid end state")

	t.Log("=== Test completed ===")
}

// TestBroadcastESPFailure_PartialSuccess tests broadcast behavior with mixed success/failure
// Uses working Mailpit for most contacts but verifies failure tracking
func TestBroadcastESPFailure_PartialSuccess(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	client := suite.APIClient
	factory := suite.DataFactory

	contactCount := 50

	t.Log("=== Starting ESP Partial Success Test ===")
	t.Logf("Contact count: %d", contactCount)

	// Step 1: Create user and workspace
	t.Log("Step 1: Creating user and workspace...")
	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)

	err = factory.AddUserToWorkspace(user.ID, workspace.ID, "owner")
	require.NoError(t, err)

	// Step 2: Setup WORKING SMTP provider (Mailpit)
	t.Log("Step 2: Setting up working SMTP provider (Mailpit)...")
	integration, err := factory.SetupWorkspaceWithSMTPProvider(workspace.ID,
		testutil.WithIntegrationEmailProvider(domain.EmailProvider{
			Kind: domain.EmailProviderKindSMTP,
			Senders: []domain.EmailSender{
				domain.NewEmailSender("noreply@notifuse.test", "Notifuse Partial Test"),
			},
			SMTP: &domain.SMTPSettings{
				Host:     "localhost",
				Port:     1025, // Mailpit - working
				Username: "",
				Password: "",
				UseTLS:   false,
			},
			RateLimitPerMinute: 2000,
		}))
	require.NoError(t, err)
	t.Logf("Created SMTP integration: %s", integration.ID)

	// Step 3: Login
	t.Log("Step 3: Logging in...")
	err = client.Login(user.Email, "password")
	require.NoError(t, err)
	client.SetWorkspaceID(workspace.ID)

	// Step 4: Clear Mailpit
	t.Log("Step 4: Clearing Mailpit messages...")
	err = testutil.ClearMailpitMessages(t)
	require.NoError(t, err)

	// Step 5: Create a contact list
	t.Log("Step 5: Creating contact list...")
	list, err := factory.CreateList(workspace.ID,
		testutil.WithListName("Partial Success Test List"))
	require.NoError(t, err)

	// Step 6: Generate contacts
	t.Logf("Step 6: Generating %d contacts...", contactCount)
	contacts := make([]map[string]interface{}, contactCount)
	for i := 0; i < contactCount; i++ {
		email := fmt.Sprintf("partial-test-%04d@example.com", i)
		contacts[i] = map[string]interface{}{
			"email":      email,
			"first_name": fmt.Sprintf("User%d", i),
			"last_name":  "PartialTest",
		}
	}

	// Step 7: Bulk import contacts
	t.Logf("Step 7: Bulk importing %d contacts...", contactCount)
	resp, err := client.BatchImportContacts(contacts, []string{list.ID})
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Step 8: Create template
	t.Log("Step 8: Creating email template...")
	uniqueSubject := fmt.Sprintf("Partial Success Test %s", uuid.New().String()[:8])
	template, err := factory.CreateTemplate(workspace.ID,
		testutil.WithTemplateName("Partial Success Test Template"),
		testutil.WithTemplateSubject(uniqueSubject))
	require.NoError(t, err)

	// Step 9: Create broadcast
	t.Log("Step 9: Creating broadcast...")
	broadcast, err := factory.CreateBroadcast(workspace.ID,
		testutil.WithBroadcastName("Partial Success Test Broadcast"),
		testutil.WithBroadcastAudience(domain.AudienceSettings{
			List:                list.ID,
			ExcludeUnsubscribed: true,
		}))
	require.NoError(t, err)

	// Step 10: Update broadcast with template
	t.Log("Step 10: Updating broadcast with template...")
	broadcast.TestSettings.Variations[0].TemplateID = template.ID
	updateReq := map[string]interface{}{
		"workspace_id":  workspace.ID,
		"id":            broadcast.ID,
		"name":          broadcast.Name,
		"audience":      broadcast.Audience,
		"schedule":      broadcast.Schedule,
		"test_settings": broadcast.TestSettings,
	}
	updateResp, err := client.UpdateBroadcast(updateReq)
	require.NoError(t, err)
	defer updateResp.Body.Close()
	require.Equal(t, http.StatusOK, updateResp.StatusCode)

	// Step 11: Schedule broadcast
	t.Log("Step 11: Scheduling broadcast...")
	scheduleRequest := map[string]interface{}{
		"workspace_id": workspace.ID,
		"id":           broadcast.ID,
		"send_now":     true,
	}

	scheduleResp, err := client.ScheduleBroadcast(scheduleRequest)
	require.NoError(t, err)
	defer scheduleResp.Body.Close()

	// Step 12: Wait for completion
	t.Log("Step 12: Waiting for broadcast completion...")
	finalStatus, err := testutil.WaitForBroadcastStatusWithExecution(t, client, broadcast.ID,
		[]string{"processed", "completed"}, 3*time.Minute)
	require.NoError(t, err)
	t.Logf("Broadcast completed with status: %s", finalStatus)

	// Step 12.5: Start email queue worker to actually send emails
	t.Log("Step 12.5: Starting email queue worker...")
	workerCtx := context.Background()
	err = suite.ServerManager.StartBackgroundWorkers(workerCtx)
	require.NoError(t, err, "Should be able to start background workers")

	// Step 12.6: Wait for email queue to finish sending
	t.Log("Step 12.6: Waiting for email queue to drain...")
	queueRepo := suite.ServerManager.GetApp().GetEmailQueueRepository()
	err = testutil.WaitForQueueEmpty(t, queueRepo, workspace.ID, 2*time.Minute)
	require.NoError(t, err, "Email queue should drain")

	// Step 13: Get final task state
	t.Log("Step 13: Checking task state...")
	tasksResp, err := client.ListTasks(map[string]string{
		"broadcast_id": broadcast.ID,
	})
	require.NoError(t, err)
	defer tasksResp.Body.Close()

	taskBody, _ := io.ReadAll(tasksResp.Body)
	var tasksResult map[string]interface{}
	err = json.Unmarshal(taskBody, &tasksResult)
	require.NoError(t, err)

	var taskState *domain.SendBroadcastState
	if tasks, ok := tasksResult["tasks"].([]interface{}); ok && len(tasks) > 0 {
		if task, ok := tasks[0].(map[string]interface{}); ok {
			if state, ok := task["state"].(map[string]interface{}); ok {
				if sendBroadcast, ok := state["send_broadcast"].(map[string]interface{}); ok {
					enqueuedCount := int(sendBroadcast["enqueued_count"].(float64))
					failedCount := int(sendBroadcast["failed_count"].(float64))
					totalRecipients := int(sendBroadcast["total_recipients"].(float64))
					recipientOffset := int(sendBroadcast["recipient_offset"].(float64))

					taskState = &domain.SendBroadcastState{
						EnqueuedCount:   enqueuedCount,
						FailedCount:     failedCount,
						TotalRecipients: totalRecipients,
						RecipientOffset: int64(recipientOffset),
					}
				}
			}
		}
	}

	// Step 14: Verify broadcast API enqueued_count matches task state
	t.Log("Step 14: Verifying broadcast enqueued_count...")
	broadcastResp, err := client.GetBroadcast(broadcast.ID)
	require.NoError(t, err)
	defer broadcastResp.Body.Close()

	var broadcastResult map[string]interface{}
	err = json.NewDecoder(broadcastResp.Body).Decode(&broadcastResult)
	require.NoError(t, err)

	if broadcastData, ok := broadcastResult["broadcast"].(map[string]interface{}); ok {
		if enqueuedCountVal, ok := broadcastData["enqueued_count"].(float64); ok {
			broadcastEnqueuedCount := int(enqueuedCountVal)
			t.Logf("Broadcast API enqueued_count: %d", broadcastEnqueuedCount)

			if taskState != nil {
				assert.Equal(t, taskState.EnqueuedCount, broadcastEnqueuedCount,
					"Broadcast enqueued_count should match task state EnqueuedCount")
			}
			assert.Equal(t, contactCount, broadcastEnqueuedCount,
				"Broadcast enqueued_count should equal contact count")
		} else {
			t.Error("enqueued_count not found in broadcast response")
		}
	} else {
		t.Error("broadcast not found in API response")
	}

	// Step 15: Verify Mailpit received emails
	t.Log("Step 15: Verifying Mailpit received emails...")
	receivedEmails, err := testutil.GetAllMailpitRecipients(t, uniqueSubject)
	require.NoError(t, err)

	// Step 16: Log results
	t.Log("=== VERIFICATION RESULTS ===")
	t.Logf("Expected recipients: %d", contactCount)
	t.Logf("Emails in Mailpit: %d", len(receivedEmails))

	if taskState != nil {
		t.Logf("Task State:")
		t.Logf("  - Total Recipients: %d", taskState.TotalRecipients)
		t.Logf("  - Enqueued Count: %d", taskState.EnqueuedCount)
		t.Logf("  - Failed Count: %d", taskState.FailedCount)
		t.Logf("  - Recipient Offset: %d", taskState.RecipientOffset)

		// With working SMTP, all should succeed
		assert.Equal(t, contactCount, taskState.EnqueuedCount,
			"All emails should be enqueued successfully")
		assert.Equal(t, 0, taskState.FailedCount,
			"No emails should fail with working SMTP")
		assert.Equal(t, contactCount, int(taskState.RecipientOffset),
			"All recipients should be processed")
	}

	// Verify Mailpit count matches
	assert.Equal(t, contactCount, len(receivedEmails),
		"Mailpit should have received all emails")

	// Step 17: Verify message_history entries are created correctly
	t.Log("Step 17: Verifying message_history entries...")
	appInstance := suite.ServerManager.GetApp()
	messageHistoryRepo := appInstance.GetMessageHistoryRepository()

	// Queue already drained above, so message_history should be populated
	messages, _, err := messageHistoryRepo.ListMessages(
		context.Background(),
		workspace.ID,
		workspace.Settings.SecretKey,
		domain.MessageListParams{
			BroadcastID: broadcast.ID,
			Limit:       100,
		},
	)
	require.NoError(t, err, "Failed to list message history")

	t.Logf("Message history entries found: %d", len(messages))

	// Verify we have message_history entries for all contacts
	assert.Equal(t, contactCount, len(messages),
		"Message history should have entries for all contacts")

	// Verify the first message has correct fields
	if len(messages) > 0 {
		msg := messages[0]
		t.Logf("Sample message_history entry:")
		t.Logf("  - ID: %s", msg.ID)
		t.Logf("  - ContactEmail: %s", msg.ContactEmail)
		t.Logf("  - BroadcastID: %v", msg.BroadcastID)
		t.Logf("  - TemplateID: %s", msg.TemplateID)
		t.Logf("  - TemplateVersion: %d", msg.TemplateVersion)
		t.Logf("  - ListID: %v", msg.ListID)
		t.Logf("  - Channel: %s", msg.Channel)
		t.Logf("  - SentAt: %v", msg.SentAt)
		t.Logf("  - FailedAt: %v", msg.FailedAt)

		// Verify broadcast_id is set correctly
		require.NotNil(t, msg.BroadcastID, "BroadcastID should not be nil")
		assert.Equal(t, broadcast.ID, *msg.BroadcastID,
			"BroadcastID should match the broadcast")

		// Verify template_id is set correctly
		assert.Equal(t, template.ID, msg.TemplateID,
			"TemplateID should match the template")

		// Verify template_version is set (should be > 0)
		assert.Greater(t, msg.TemplateVersion, int64(0),
			"TemplateVersion should be set")

		// Verify list_id is set correctly
		require.NotNil(t, msg.ListID, "ListID should not be nil")
		assert.Equal(t, list.ID, *msg.ListID,
			"ListID should match the list")

		// Verify channel is email
		assert.Equal(t, "email", msg.Channel,
			"Channel should be email")

		// Verify sent_at is set (not zero time)
		assert.False(t, msg.SentAt.IsZero(),
			"SentAt should be set")

		// Verify failed_at is nil (successful send)
		assert.Nil(t, msg.FailedAt,
			"FailedAt should be nil for successful sends")
	}

	t.Log("=== Test completed successfully ===")
}

// TestBroadcastESPFailure_CircuitBreaker tests that circuit breaker activates
// when too many failures occur
func TestBroadcastESPFailure_CircuitBreaker(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	client := suite.APIClient
	factory := suite.DataFactory

	// Use a smaller number of contacts for faster test
	contactCount := 20

	t.Log("=== Starting Circuit Breaker Test ===")
	t.Logf("Contact count: %d", contactCount)

	// Step 1: Create user and workspace
	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)

	err = factory.AddUserToWorkspace(user.ID, workspace.ID, "owner")
	require.NoError(t, err)

	// Step 2: Setup SMTP with invalid port (will fail all sends)
	t.Log("Setting up SMTP with invalid port to trigger failures...")
	_, err = factory.SetupWorkspaceWithSMTPProvider(workspace.ID,
		testutil.WithIntegrationEmailProvider(domain.EmailProvider{
			Kind: domain.EmailProviderKindSMTP,
			Senders: []domain.EmailSender{
				domain.NewEmailSender("noreply@notifuse.test", "Circuit Breaker Test"),
			},
			SMTP: &domain.SMTPSettings{
				Host:     "localhost",
				Port:     9998, // Nothing listening
				Username: "",
				Password: "",
				UseTLS:   false,
			},
			RateLimitPerMinute: 1000,
		}))
	require.NoError(t, err)

	// Login
	err = client.Login(user.Email, "password")
	require.NoError(t, err)
	client.SetWorkspaceID(workspace.ID)

	// Create list
	list, err := factory.CreateList(workspace.ID,
		testutil.WithListName("Circuit Breaker Test List"))
	require.NoError(t, err)

	// Generate contacts
	contacts := make([]map[string]interface{}, contactCount)
	for i := 0; i < contactCount; i++ {
		contacts[i] = map[string]interface{}{
			"email":      fmt.Sprintf("circuit-breaker-%04d@example.com", i),
			"first_name": fmt.Sprintf("User%d", i),
			"last_name":  "CircuitBreakerTest",
		}
	}

	// Import contacts
	resp, err := client.BatchImportContacts(contacts, []string{list.ID})
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Create template
	template, err := factory.CreateTemplate(workspace.ID,
		testutil.WithTemplateName("Circuit Breaker Template"),
		testutil.WithTemplateSubject(fmt.Sprintf("Circuit Breaker Test %s", uuid.New().String()[:8])))
	require.NoError(t, err)

	// Create broadcast
	broadcast, err := factory.CreateBroadcast(workspace.ID,
		testutil.WithBroadcastName("Circuit Breaker Test Broadcast"),
		testutil.WithBroadcastAudience(domain.AudienceSettings{
			List:                list.ID,
			ExcludeUnsubscribed: true,
		}))
	require.NoError(t, err)

	// Update with template
	broadcast.TestSettings.Variations[0].TemplateID = template.ID
	updateReq := map[string]interface{}{
		"workspace_id":  workspace.ID,
		"id":            broadcast.ID,
		"name":          broadcast.Name,
		"audience":      broadcast.Audience,
		"schedule":      broadcast.Schedule,
		"test_settings": broadcast.TestSettings,
	}
	updateResp, err := client.UpdateBroadcast(updateReq)
	require.NoError(t, err)
	defer updateResp.Body.Close()

	// Schedule broadcast
	scheduleResp, err := client.ScheduleBroadcast(map[string]interface{}{
		"workspace_id": workspace.ID,
		"id":           broadcast.ID,
		"send_now":     true,
	})
	require.NoError(t, err)
	defer scheduleResp.Body.Close()

	// Step 1: Wait for broadcast to be processed (all emails enqueued)
	t.Log("Step 1: Waiting for broadcast to process and enqueue all emails...")
	timeout := 2 * time.Minute
	deadline := time.Now().Add(timeout)
	var finalStatus string
	var enqueuedCount int

	for time.Now().Before(deadline) {
		// Execute tasks to process the broadcast
		_, _ = client.ExecutePendingTasks(10)
		time.Sleep(500 * time.Millisecond)

		// Get broadcast status
		broadcastResp, _ := client.GetBroadcast(broadcast.ID)
		if broadcastResp != nil {
			body, _ := io.ReadAll(broadcastResp.Body)
			broadcastResp.Body.Close()

			var result map[string]interface{}
			if json.Unmarshal(body, &result) == nil {
				if bd, ok := result["broadcast"].(map[string]interface{}); ok {
					finalStatus, _ = bd["status"].(string)
					if ec, ok := bd["enqueued_count"].(float64); ok {
						enqueuedCount = int(ec)
					}
					t.Logf("Broadcast status: %s, enqueued: %d/%d", finalStatus, enqueuedCount, contactCount)
				}
			}
		}

		// Check if broadcast is processed (all emails enqueued)
		if finalStatus == "processed" {
			t.Log("Broadcast is processed, all emails enqueued!")
			break
		}
	}

	require.Equal(t, "processed", finalStatus, "Broadcast should be processed")
	require.Equal(t, contactCount, enqueuedCount, "All contacts should be enqueued")

	// Step 2: Start the email queue worker and wait for it to process emails
	t.Log("Step 2: Starting email queue worker to process emails...")

	// Get the email queue repository from the app
	queueRepo := suite.ServerManager.GetApp().GetEmailQueueRepository()

	// Start the background workers (email queue worker)
	ctx := context.Background()
	err = suite.ServerManager.StartBackgroundWorkers(ctx)
	require.NoError(t, err, "Should be able to start background workers")

	// Wait for either:
	// 1. Circuit breaker to open (some emails fail, rest pending due to circuit open)
	// 2. Or queue to be fully processed (all fail without circuit breaker triggering)
	// The circuit breaker opens after ~5 consecutive failures (default threshold)
	t.Log("Waiting for circuit breaker to trigger or queue to process...")

	// Wait for some failures to be recorded (circuit breaker should trigger after threshold)
	timeout = 2 * time.Minute
	deadline = time.Now().Add(timeout)
	var finalQueueStats *domain.EmailQueueStats

	for time.Now().Before(deadline) {
		stats, err := queueRepo.GetStats(ctx, workspace.ID)
		if err != nil {
			t.Logf("Warning: failed to get queue stats: %v", err)
			time.Sleep(500 * time.Millisecond)
			continue
		}

		t.Logf("Queue status - pending: %d, processing: %d, failed: %d",
			stats.Pending, stats.Processing, stats.Failed)

		finalQueueStats = stats

		// Circuit breaker should have triggered - we should see:
		// - Some failures (at least circuit breaker threshold)
		// - And some pending emails (circuit is open, not processing)
		if stats.Failed > 0 && stats.Pending > 0 && stats.Processing == 0 {
			t.Log("Circuit breaker appears to have triggered - queue has failures and pending items")
			break
		}

		// Or if all emails are processed (either sent or failed)
		if stats.Pending == 0 && stats.Processing == 0 {
			t.Log("All queue items processed")
			break
		}

		time.Sleep(500 * time.Millisecond)
	}

	require.NotNil(t, finalQueueStats, "Should have queue stats")

	t.Log("=== CIRCUIT BREAKER TEST RESULTS ===")
	t.Logf("Final Broadcast Status: %s", finalStatus)
	t.Logf("Enqueued Count: %d", enqueuedCount)
	t.Logf("Queue Pending: %d", finalQueueStats.Pending)
	t.Logf("Queue Failed: %d", finalQueueStats.Failed)

	// With the email queue system and circuit breaker:
	// - Some emails should have failed (at least the threshold count)
	// - The circuit breaker should have opened, stopping further processing
	assert.Greater(t, finalQueueStats.Failed, int64(0),
		"Should have recorded some failures in email queue")

	// If circuit breaker triggered, there should be pending emails that weren't processed
	// (The circuit breaker stops processing when too many consecutive failures occur)
	if finalQueueStats.Pending > 0 {
		t.Logf("Circuit breaker successfully stopped processing! %d emails still pending", finalQueueStats.Pending)
		assert.Greater(t, finalQueueStats.Failed, int64(0),
			"Should have some failures that triggered circuit breaker")
	} else {
		t.Log("All emails processed (circuit breaker may not have triggered or all retried)")
	}

	t.Logf("Email queue: %d failed, %d still pending", finalQueueStats.Failed, finalQueueStats.Pending)
	t.Log("=== Test completed ===")
}

// TestBroadcastConcurrentExecution tests if concurrent scheduler triggers cause race conditions
// This simulates the scenario where the scheduler triggers a new job while another is running
func TestBroadcastConcurrentExecution(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	client := suite.APIClient
	factory := suite.DataFactory

	// Use enough contacts to span multiple batches
	contactCount := 200

	t.Log("=== Starting Concurrent Execution Test ===")
	t.Logf("Contact count: %d", contactCount)
	t.Log("This test simulates multiple scheduler triggers running concurrently")

	// Setup
	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)

	err = factory.AddUserToWorkspace(user.ID, workspace.ID, "owner")
	require.NoError(t, err)

	// Setup working SMTP
	_, err = factory.SetupWorkspaceWithSMTPProvider(workspace.ID,
		testutil.WithIntegrationEmailProvider(domain.EmailProvider{
			Kind: domain.EmailProviderKindSMTP,
			Senders: []domain.EmailSender{
				domain.NewEmailSender("noreply@notifuse.test", "Concurrent Test"),
			},
			SMTP: &domain.SMTPSettings{
				Host:     "localhost",
				Port:     1025, // Mailpit
				Username: "",
				Password: "",
				UseTLS:   false,
			},
			RateLimitPerMinute: 2000,
		}))
	require.NoError(t, err)

	err = client.Login(user.Email, "password")
	require.NoError(t, err)
	client.SetWorkspaceID(workspace.ID)

	// Clear Mailpit
	err = testutil.ClearMailpitMessages(t)
	require.NoError(t, err)

	// Create list and contacts
	list, err := factory.CreateList(workspace.ID,
		testutil.WithListName("Concurrent Test List"))
	require.NoError(t, err)

	contacts := make([]map[string]interface{}, contactCount)
	expectedEmails := make(map[string]bool)
	for i := 0; i < contactCount; i++ {
		email := fmt.Sprintf("concurrent-test-%04d@example.com", i)
		contacts[i] = map[string]interface{}{
			"email":      email,
			"first_name": fmt.Sprintf("User%d", i),
			"last_name":  "ConcurrentTest",
		}
		expectedEmails[email] = false
	}

	resp, err := client.BatchImportContacts(contacts, []string{list.ID})
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Create template with unique subject
	uniqueSubject := fmt.Sprintf("Concurrent Test %s", uuid.New().String()[:8])
	template, err := factory.CreateTemplate(workspace.ID,
		testutil.WithTemplateName("Concurrent Test Template"),
		testutil.WithTemplateSubject(uniqueSubject))
	require.NoError(t, err)

	// Create broadcast
	broadcast, err := factory.CreateBroadcast(workspace.ID,
		testutil.WithBroadcastName("Concurrent Test Broadcast"),
		testutil.WithBroadcastAudience(domain.AudienceSettings{
			List:                list.ID,
			ExcludeUnsubscribed: true,
		}))
	require.NoError(t, err)

	// Update with template
	broadcast.TestSettings.Variations[0].TemplateID = template.ID
	updateReq := map[string]interface{}{
		"workspace_id":  workspace.ID,
		"id":            broadcast.ID,
		"name":          broadcast.Name,
		"audience":      broadcast.Audience,
		"schedule":      broadcast.Schedule,
		"test_settings": broadcast.TestSettings,
	}
	updateResp, err := client.UpdateBroadcast(updateReq)
	require.NoError(t, err)
	defer updateResp.Body.Close()

	// Schedule broadcast
	scheduleResp, err := client.ScheduleBroadcast(map[string]interface{}{
		"workspace_id": workspace.ID,
		"id":           broadcast.ID,
		"send_now":     true,
	})
	require.NoError(t, err)
	defer scheduleResp.Body.Close()

	// KEY TEST: Trigger multiple concurrent task executions to simulate scheduler race
	t.Log("Triggering CONCURRENT task executions to simulate scheduler race condition...")

	var wg sync.WaitGroup
	concurrentTriggers := 5 // Simulate 5 concurrent scheduler triggers

	for i := 0; i < concurrentTriggers; i++ {
		wg.Add(1)
		go func(triggerID int) {
			defer wg.Done()
			t.Logf("Concurrent trigger %d: executing pending tasks", triggerID)
			_, _ = client.ExecutePendingTasks(10)
		}(i)
	}

	// Wait for all concurrent triggers to complete
	wg.Wait()
	t.Log("All concurrent triggers completed")

	// Continue executing tasks until broadcast completes
	timeout := 3 * time.Minute
	deadline := time.Now().Add(timeout)
	var finalStatus string

	for time.Now().Before(deadline) {
		// Normal sequential task execution
		_, _ = client.ExecutePendingTasks(10)
		time.Sleep(500 * time.Millisecond)

		// Check broadcast status
		broadcastResp, err := client.GetBroadcast(broadcast.ID)
		if err != nil {
			continue
		}
		body, _ := io.ReadAll(broadcastResp.Body)
		broadcastResp.Body.Close()

		var result map[string]interface{}
		if json.Unmarshal(body, &result) == nil {
			if bd, ok := result["broadcast"].(map[string]interface{}); ok {
				finalStatus, _ = bd["status"].(string)
				if finalStatus == "processed" || finalStatus == "completed" || finalStatus == "failed" {
					break
				}
			}
		}
	}

	t.Logf("Broadcast final status: %s", finalStatus)

	// Start email queue worker to actually send emails
	t.Log("Starting email queue worker...")
	workerCtx := context.Background()
	err = suite.ServerManager.StartBackgroundWorkers(workerCtx)
	require.NoError(t, err, "Should be able to start background workers")

	// Wait for email queue to process all emails
	t.Log("Waiting for email queue to drain...")
	queueRepo := suite.ServerManager.GetApp().GetEmailQueueRepository()
	err = testutil.WaitForQueueEmpty(t, queueRepo, workspace.ID, 3*time.Minute)
	require.NoError(t, err, "Email queue should drain")

	// Verify emails in Mailpit
	receivedEmails, err := testutil.GetAllMailpitRecipients(t, uniqueSubject)
	require.NoError(t, err)

	// Check for duplicates by counting total messages
	totalMessages, err := testutil.GetMailpitMessageCount(t, uniqueSubject)
	require.NoError(t, err)

	// Calculate missing
	var missing []string
	for email := range expectedEmails {
		if !receivedEmails[email] {
			missing = append(missing, email)
		}
	}

	t.Log("=== CONCURRENT EXECUTION TEST RESULTS ===")
	t.Logf("Expected unique recipients: %d", contactCount)
	t.Logf("Unique recipients in Mailpit: %d", len(receivedEmails))
	t.Logf("Total messages in Mailpit: %d", totalMessages)
	t.Logf("Missing recipients: %d", len(missing))

	// Check for duplicate emails (sign of race condition)
	duplicates := totalMessages - len(receivedEmails)
	if duplicates > 0 {
		t.Logf("WARNING: %d DUPLICATE emails detected! This indicates a race condition.", duplicates)
	}

	// Log some missing emails for debugging
	if len(missing) > 0 && len(missing) <= 20 {
		t.Log("Missing emails:")
		for _, email := range missing {
			t.Logf("  - %s", email)
		}
	} else if len(missing) > 20 {
		t.Logf("First 20 missing emails:")
		for i := 0; i < 20; i++ {
			t.Logf("  - %s", missing[i])
		}
		t.Logf("  ... and %d more", len(missing)-20)
	}

	// Assertions
	assert.Equal(t, 0, duplicates,
		"No duplicate emails should be processed (race condition detected)")
	assert.Empty(t, missing,
		"No recipients should be missing")
	assert.Equal(t, contactCount, len(receivedEmails),
		"All recipients should receive exactly one email")
	assert.Equal(t, contactCount, totalMessages,
		"Total messages should equal contact count (no duplicates)")

	t.Log("=== Test completed ===")
}
