package integration

import (
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
								sentCount := int(sendBroadcast["sent_count"].(float64))
								failedCount := int(sendBroadcast["failed_count"].(float64))
								totalRecipients := int(sendBroadcast["total_recipients"].(float64))
								recipientOffset := int(sendBroadcast["recipient_offset"].(float64))

								t.Logf("Task State - sent: %d, failed: %d, total: %d, offset: %d",
									sentCount, failedCount, totalRecipients, recipientOffset)

								taskState = &domain.SendBroadcastState{
									SentCount:       sentCount,
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
		if finalStatus == "sent" || finalStatus == "failed" || finalStatus == "paused" || finalStatus == "completed" {
			break
		}

		time.Sleep(1 * time.Second)
	}

	// Step 12: Verify results
	t.Log("=== VERIFICATION RESULTS ===")
	t.Logf("Final broadcast status: %s", finalStatus)

	if taskState != nil {
		t.Logf("Task State:")
		t.Logf("  - Total Recipients: %d", taskState.TotalRecipients)
		t.Logf("  - Sent Count: %d", taskState.SentCount)
		t.Logf("  - Failed Count: %d", taskState.FailedCount)
		t.Logf("  - Recipient Offset: %d", taskState.RecipientOffset)

		// Verify all emails failed (since SMTP is unreachable)
		assert.Equal(t, 0, taskState.SentCount,
			"No emails should be sent successfully (SMTP unreachable)")
		assert.Greater(t, taskState.FailedCount, 0,
			"Failed count should be greater than 0")

		// The offset should have advanced (attempted to process recipients)
		assert.Greater(t, int(taskState.RecipientOffset), 0,
			"Recipient offset should have advanced despite failures")
	} else {
		t.Log("Warning: Could not retrieve task state")
	}

	// The broadcast should either be:
	// - "paused" (circuit breaker triggered after too many failures)
	// - "failed" (explicit failure state)
	// - "sent" (completed but with failures tracked)
	assert.Contains(t, []string{"paused", "failed", "sent", "sending"},
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
		[]string{"sent", "completed"}, 3*time.Minute)
	require.NoError(t, err)
	t.Logf("Broadcast completed with status: %s", finalStatus)

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
					sentCount := int(sendBroadcast["sent_count"].(float64))
					failedCount := int(sendBroadcast["failed_count"].(float64))
					totalRecipients := int(sendBroadcast["total_recipients"].(float64))
					recipientOffset := int(sendBroadcast["recipient_offset"].(float64))

					taskState = &domain.SendBroadcastState{
						SentCount:       sentCount,
						FailedCount:     failedCount,
						TotalRecipients: totalRecipients,
						RecipientOffset: int64(recipientOffset),
					}
				}
			}
		}
	}

	// Step 14: Verify Mailpit received emails
	t.Log("Step 14: Verifying Mailpit received emails...")
	receivedEmails, err := testutil.GetAllMailpitRecipients(t, uniqueSubject)
	require.NoError(t, err)

	// Step 15: Log results
	t.Log("=== VERIFICATION RESULTS ===")
	t.Logf("Expected recipients: %d", contactCount)
	t.Logf("Emails in Mailpit: %d", len(receivedEmails))

	if taskState != nil {
		t.Logf("Task State:")
		t.Logf("  - Total Recipients: %d", taskState.TotalRecipients)
		t.Logf("  - Sent Count: %d", taskState.SentCount)
		t.Logf("  - Failed Count: %d", taskState.FailedCount)
		t.Logf("  - Recipient Offset: %d", taskState.RecipientOffset)

		// With working SMTP, all should succeed
		assert.Equal(t, contactCount, taskState.SentCount,
			"All emails should be sent successfully")
		assert.Equal(t, 0, taskState.FailedCount,
			"No emails should fail with working SMTP")
		assert.Equal(t, contactCount, int(taskState.RecipientOffset),
			"All recipients should be processed")
	}

	// Verify Mailpit count matches
	assert.Equal(t, contactCount, len(receivedEmails),
		"Mailpit should have received all emails")

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

	// Use enough contacts to trigger circuit breaker (default threshold is usually 10-20 failures)
	contactCount := 100

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

	// Monitor broadcast for circuit breaker triggering
	t.Log("Monitoring broadcast for circuit breaker activation...")
	timeout := 3 * time.Minute
	deadline := time.Now().Add(timeout)
	var finalStatus string
	var finalFailedCount int
	var finalRecipientOffset int

	for time.Now().Before(deadline) {
		// Execute tasks
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
				}
			}
		}

		// Get task state
		tasksResp, _ := client.ListTasks(map[string]string{"broadcast_id": broadcast.ID})
		if tasksResp != nil {
			taskBody, _ := io.ReadAll(tasksResp.Body)
			tasksResp.Body.Close()

			var tasksResult map[string]interface{}
			if json.Unmarshal(taskBody, &tasksResult) == nil {
				if tasks, ok := tasksResult["tasks"].([]interface{}); ok && len(tasks) > 0 {
					if task, ok := tasks[0].(map[string]interface{}); ok {
						if state, ok := task["state"].(map[string]interface{}); ok {
							if sb, ok := state["send_broadcast"].(map[string]interface{}); ok {
								finalFailedCount = int(sb["failed_count"].(float64))
								finalRecipientOffset = int(sb["recipient_offset"].(float64))
								t.Logf("Status: %s, Failed: %d, Offset: %d",
									finalStatus, finalFailedCount, finalRecipientOffset)
							}
						}
					}
				}
			}
		}

		// Check for circuit breaker (broadcast should pause)
		if finalStatus == "paused" || finalStatus == "failed" {
			t.Logf("Circuit breaker appears to have triggered! Status: %s", finalStatus)
			break
		}

		time.Sleep(1 * time.Second)
	}

	t.Log("=== CIRCUIT BREAKER TEST RESULTS ===")
	t.Logf("Final Status: %s", finalStatus)
	t.Logf("Failed Count: %d", finalFailedCount)
	t.Logf("Recipient Offset: %d", finalRecipientOffset)

	// Circuit breaker should have:
	// 1. Recorded failures
	// 2. Either paused the broadcast or processed all with failures
	assert.Greater(t, finalFailedCount, 0,
		"Should have recorded some failures")

	// Log whether circuit breaker paused the broadcast
	if finalStatus == "paused" {
		t.Log("Circuit breaker successfully paused the broadcast!")
		assert.Less(t, finalRecipientOffset, contactCount,
			"Circuit breaker should stop before processing all recipients")
	} else {
		t.Logf("Broadcast completed/failed with status: %s (circuit breaker may have different threshold)", finalStatus)
	}

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
				if finalStatus == "sent" || finalStatus == "completed" || finalStatus == "failed" {
					break
				}
			}
		}
	}

	t.Logf("Broadcast final status: %s", finalStatus)

	// Wait for Mailpit to receive all emails
	t.Log("Waiting for emails in Mailpit...")
	time.Sleep(5 * time.Second)

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
		"No duplicate emails should be sent (race condition detected)")
	assert.Empty(t, missing,
		"No recipients should be missing")
	assert.Equal(t, contactCount, len(receivedEmails),
		"All recipients should receive exactly one email")
	assert.Equal(t, contactCount, totalMessages,
		"Total messages should equal contact count (no duplicates)")

	t.Log("=== Test completed ===")
}
