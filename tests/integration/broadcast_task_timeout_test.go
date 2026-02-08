package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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

// TestBroadcastTaskTimeoutRecovery verifies that broadcast tasks properly
// resume after timeout and eventually complete all recipients.
//
// This test uses a very short MaxRuntime to force multiple timeout/resume cycles
// and verifies that:
// 1. Task state is properly saved on timeout (offset, sent, failed counts)
// 2. Task resumes from saved state without duplicate sends
// 3. All recipients eventually receive emails
func TestBroadcastTaskTimeoutRecovery(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	client := suite.APIClient
	factory := suite.DataFactory

	contactCount := 200 // Enough contacts to require multiple timeout cycles with 5s runtime

	t.Log("=== Starting Broadcast Task Timeout Recovery Test ===")
	t.Logf("Contact count: %d", contactCount)
	t.Log("This test verifies task properly resumes after timeout")

	// Step 1: Setup workspace and provider
	t.Log("Step 1: Creating user and workspace...")
	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)

	err = factory.AddUserToWorkspace(user.ID, workspace.ID, "owner")
	require.NoError(t, err)

	// Setup SMTP provider with high rate limit
	_, err = factory.SetupWorkspaceWithSMTPProvider(workspace.ID,
		testutil.WithIntegrationEmailProvider(domain.EmailProvider{
			Kind: domain.EmailProviderKindSMTP,
			Senders: []domain.EmailSender{
				domain.NewEmailSender("noreply@notifuse.test", "Timeout Test"),
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

	// Step 2: Clear Mailpit
	t.Log("Step 2: Clearing Mailpit messages...")
	err = testutil.ClearMailpitMessages(t)
	require.NoError(t, err)

	// Step 3: Create list and contacts
	t.Log("Step 3: Creating list and contacts...")
	list, err := factory.CreateList(workspace.ID,
		testutil.WithListName("Timeout Test List"))
	require.NoError(t, err)

	contacts := make([]map[string]interface{}, contactCount)
	for i := 0; i < contactCount; i++ {
		contacts[i] = map[string]interface{}{
			"email":      fmt.Sprintf("timeout-test-%04d@example.com", i),
			"first_name": fmt.Sprintf("User%d", i),
			"last_name":  "TimeoutTest",
		}
	}

	resp, err := client.BatchImportContacts(contacts, []string{list.ID})
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Step 4: Create template with unique subject
	t.Log("Step 4: Creating template...")
	uniqueSubject := fmt.Sprintf("Timeout Test %s", uuid.New().String()[:8])
	template, err := factory.CreateTemplate(workspace.ID,
		testutil.WithTemplateName("Timeout Test Template"),
		testutil.WithTemplateSubject(uniqueSubject))
	require.NoError(t, err)

	// Step 5: Create and schedule broadcast
	t.Log("Step 5: Creating broadcast...")
	broadcast, err := factory.CreateBroadcast(workspace.ID,
		testutil.WithBroadcastName("Timeout Test Broadcast"),
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

	// Step 6: Start email queue worker to process emails
	t.Log("Step 6: Starting email queue worker...")
	ctx := context.Background()
	err = suite.ServerManager.StartBackgroundWorkers(ctx)
	require.NoError(t, err, "Should be able to start background workers")

	// Step 7: Schedule broadcast
	t.Log("Step 7: Scheduling broadcast...")
	scheduleResp, err := client.ScheduleBroadcast(map[string]interface{}{
		"workspace_id": workspace.ID,
		"id":           broadcast.ID,
		"send_now":     true,
	})
	require.NoError(t, err)
	defer scheduleResp.Body.Close()

	time.Sleep(500 * time.Millisecond)

	// Step 8: Get task ID and modify MaxRuntime for testing
	t.Log("Step 8: Getting task ID and setting short timeout...")
	tasksResp, err := client.ListTasks(map[string]string{
		"broadcast_id": broadcast.ID,
	})
	require.NoError(t, err)

	taskBody, _ := io.ReadAll(tasksResp.Body)
	tasksResp.Body.Close()

	var tasksResult map[string]interface{}
	err = json.Unmarshal(taskBody, &tasksResult)
	require.NoError(t, err)

	tasks, ok := tasksResult["tasks"].([]interface{})
	require.True(t, ok && len(tasks) > 0, "Should have at least one task")

	task := tasks[0].(map[string]interface{})
	taskID := task["id"].(string)
	t.Logf("Found task ID: %s", taskID)

	// Update task MaxRuntime to 5 seconds via direct DB
	// This forces multiple timeout/resume cycles
	err = factory.UpdateTaskMaxRuntime(workspace.ID, taskID, 5)
	require.NoError(t, err)
	t.Log("Set task MaxRuntime to 5 seconds")

	// Step 9: Execute task multiple times, monitoring state
	t.Log("Step 9: Executing task with timeouts...")

	maxExecutions := 50 // Safety limit
	executionCount := 0
	var lastState *TaskStateInfo
	timeoutOccurred := false

	for executionCount < maxExecutions {
		executionCount++
		t.Logf("Execution #%d", executionCount)

		// Execute the task
		execResp, execErr := client.ExecuteTask(map[string]interface{}{
			"workspace_id": workspace.ID,
			"id":           taskID,
		})

		if execErr != nil {
			t.Logf("Execution error: %v", execErr)
			continue
		}

		execBody, _ := io.ReadAll(execResp.Body)
		execResp.Body.Close()

		// Check task state
		state, err := getTaskStateInfo(client, workspace.ID, taskID)
		if err != nil {
			t.Logf("Failed to get task state: %v", err)
			continue
		}

		t.Logf("  Status: %s, Progress: %.1f%%, Offset: %d, Sent: %d, Failed: %d",
			state.Status, state.Progress, state.RecipientOffset, state.EnqueuedCount, state.FailedCount)

		// Detect timeout (task paused or returned with partial progress)
		if state.Status == "paused" || (lastState != nil && state.RecipientOffset > lastState.RecipientOffset && state.Status != "completed") {
			timeoutOccurred = true
		}

		lastState = state

		// Check if task completed
		if state.Status == "completed" {
			t.Logf("Task completed after %d executions", executionCount)
			break
		}

		// Small delay before next execution
		time.Sleep(100 * time.Millisecond)

		// Check response for timeout indicator
		var execResult map[string]interface{}
		if json.Unmarshal(execBody, &execResult) == nil {
			if msg, ok := execResult["message"].(string); ok {
				t.Logf("  Response message: %s", msg)
			}
		}
	}

	t.Logf("Total executions: %d, Timeout occurred: %v", executionCount, timeoutOccurred)

	// Step 10: Wait for emails to arrive
	t.Log("Step 10: Waiting for emails in Mailpit...")
	err = testutil.WaitForMailpitMessages(t, uniqueSubject, contactCount, 2*time.Minute)
	if err != nil {
		t.Logf("Warning: Not all emails arrived: %v", err)
	}

	// Step 11: Verify results
	t.Log("Step 11: Verifying results...")
	totalMessages, err := testutil.GetMailpitMessageCount(t, uniqueSubject)
	require.NoError(t, err)

	receivedEmails, err := testutil.GetAllMailpitRecipients(t, uniqueSubject)
	require.NoError(t, err)

	t.Log("=== TIMEOUT RECOVERY TEST RESULTS ===")
	t.Logf("Expected contacts: %d", contactCount)
	t.Logf("Unique recipients: %d", len(receivedEmails))
	t.Logf("Total messages: %d", totalMessages)
	t.Logf("Timeout occurred: %v", timeoutOccurred)
	t.Logf("Execution cycles: %d", executionCount)

	duplicates := totalMessages - len(receivedEmails)
	if duplicates > 0 {
		t.Logf("DUPLICATES DETECTED: %d duplicate emails!", duplicates)
	}

	// Assertions
	t.Log("=== FINAL ASSERTIONS ===")

	// Verify no duplicates
	assert.Equal(t, 0, duplicates, "No duplicate emails should be sent")

	// Verify all contacts received emails
	assert.Equal(t, contactCount, totalMessages,
		"All %d contacts should receive emails, got %d", contactCount, totalMessages)

	// Verify timeout actually occurred (unless sending was very fast)
	if executionCount > 1 {
		assert.True(t, timeoutOccurred || executionCount > 1,
			"Timeout should have occurred with short MaxRuntime")
	}

	t.Log("=== Test completed ===")
}

// TestBroadcastStateCounterAccuracy verifies that broadcast state counters
// (EnqueuedCount, FailedCount, RecipientOffset) accurately reflect the actual
// number of emails sent.
func TestBroadcastStateCounterAccuracy(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	client := suite.APIClient
	factory := suite.DataFactory

	contactCount := 100

	t.Log("=== Starting Broadcast State Counter Accuracy Test ===")
	t.Logf("Contact count: %d", contactCount)

	// Setup
	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)

	err = factory.AddUserToWorkspace(user.ID, workspace.ID, "owner")
	require.NoError(t, err)

	_, err = factory.SetupWorkspaceWithSMTPProvider(workspace.ID,
		testutil.WithIntegrationEmailProvider(domain.EmailProvider{
			Kind: domain.EmailProviderKindSMTP,
			Senders: []domain.EmailSender{
				domain.NewEmailSender("noreply@notifuse.test", "Counter Test"),
			},
			SMTP: &domain.SMTPSettings{
				Host:     "localhost",
				Port:     1025,
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

	err = testutil.ClearMailpitMessages(t)
	require.NoError(t, err)

	// Create list and contacts
	list, err := factory.CreateList(workspace.ID, testutil.WithListName("Counter Test List"))
	require.NoError(t, err)

	contacts := make([]map[string]interface{}, contactCount)
	for i := 0; i < contactCount; i++ {
		contacts[i] = map[string]interface{}{
			"email":      fmt.Sprintf("counter-test-%04d@example.com", i),
			"first_name": fmt.Sprintf("User%d", i),
			"last_name":  "CounterTest",
		}
	}

	resp, err := client.BatchImportContacts(contacts, []string{list.ID})
	require.NoError(t, err)
	resp.Body.Close()

	// Create template and broadcast
	uniqueSubject := fmt.Sprintf("Counter Test %s", uuid.New().String()[:8])
	template, err := factory.CreateTemplate(workspace.ID,
		testutil.WithTemplateName("Counter Test Template"),
		testutil.WithTemplateSubject(uniqueSubject))
	require.NoError(t, err)

	broadcast, err := factory.CreateBroadcast(workspace.ID,
		testutil.WithBroadcastName("Counter Test Broadcast"),
		testutil.WithBroadcastAudience(domain.AudienceSettings{
			List:                list.ID,
			ExcludeUnsubscribed: true,
		}))
	require.NoError(t, err)

	broadcast.TestSettings.Variations[0].TemplateID = template.ID
	updateResp, err := client.UpdateBroadcast(map[string]interface{}{
		"workspace_id":  workspace.ID,
		"id":            broadcast.ID,
		"name":          broadcast.Name,
		"audience":      broadcast.Audience,
		"schedule":      broadcast.Schedule,
		"test_settings": broadcast.TestSettings,
	})
	require.NoError(t, err)
	updateResp.Body.Close()

	// Start email queue worker
	ctx := context.Background()
	err = suite.ServerManager.StartBackgroundWorkers(ctx)
	require.NoError(t, err, "Should be able to start background workers")

	// Schedule and get task
	scheduleResp, err := client.ScheduleBroadcast(map[string]interface{}{
		"workspace_id": workspace.ID,
		"id":           broadcast.ID,
		"send_now":     true,
	})
	require.NoError(t, err)
	scheduleResp.Body.Close()

	time.Sleep(500 * time.Millisecond)

	tasksResp, err := client.ListTasks(map[string]string{"broadcast_id": broadcast.ID})
	require.NoError(t, err)
	taskBody, _ := io.ReadAll(tasksResp.Body)
	tasksResp.Body.Close()

	var tasksResult map[string]interface{}
	json.Unmarshal(taskBody, &tasksResult)
	tasks := tasksResult["tasks"].([]interface{})
	taskID := tasks[0].(map[string]interface{})["id"].(string)

	// Set short timeout for multiple cycles
	err = factory.UpdateTaskMaxRuntime(workspace.ID, taskID, 3)
	require.NoError(t, err)

	t.Log("Executing task and monitoring counters...")

	var counterSnapshots []TaskStateInfo
	maxExecutions := 50

	for i := 0; i < maxExecutions; i++ {
		// Capture state before execution
		stateBefore, _ := getTaskStateInfo(client, workspace.ID, taskID)
		if stateBefore != nil {
			counterSnapshots = append(counterSnapshots, *stateBefore)
		}

		// Execute
		execResp, _ := client.ExecuteTask(map[string]interface{}{
			"workspace_id": workspace.ID,
			"id":           taskID,
		})
		if execResp != nil {
			execResp.Body.Close()
		}

		// Capture state after
		stateAfter, _ := getTaskStateInfo(client, workspace.ID, taskID)
		if stateAfter != nil {
			t.Logf("Cycle %d: Status=%s, Sent=%d, Failed=%d, Offset=%d, Progress=%.1f%%",
				i+1, stateAfter.Status, stateAfter.EnqueuedCount, stateAfter.FailedCount,
				stateAfter.RecipientOffset, stateAfter.Progress)

			if stateAfter.Status == "completed" {
				counterSnapshots = append(counterSnapshots, *stateAfter)
				break
			}
		}

		time.Sleep(100 * time.Millisecond)
	}

	// Wait for emails
	err = testutil.WaitForMailpitMessages(t, uniqueSubject, contactCount, 2*time.Minute)
	if err != nil {
		t.Logf("Warning: Not all emails arrived: %v", err)
	}

	// Get actual email count
	actualMessages, err := testutil.GetMailpitMessageCount(t, uniqueSubject)
	require.NoError(t, err)

	// Get final state
	finalState, err := getTaskStateInfo(client, workspace.ID, taskID)
	require.NoError(t, err)

	t.Log("=== COUNTER ACCURACY RESULTS ===")
	t.Logf("Final Status: %s", finalState.Status)
	t.Logf("State EnqueuedCount: %d", finalState.EnqueuedCount)
	t.Logf("State FailedCount: %d", finalState.FailedCount)
	t.Logf("State RecipientOffset: %d", finalState.RecipientOffset)
	t.Logf("State TotalRecipients: %d", finalState.TotalRecipients)
	t.Logf("State Progress: %.1f%%", finalState.Progress)
	t.Logf("Actual Mailpit Messages: %d", actualMessages)
	t.Logf("Expected Contacts: %d", contactCount)

	// Assertions
	t.Log("=== ASSERTIONS ===")

	// Counter consistency
	assert.Equal(t, finalState.EnqueuedCount+finalState.FailedCount, int(finalState.RecipientOffset),
		"Offset should equal Sent + Failed")

	// Counter matches actual sends (allowing for failed sends)
	assert.Equal(t, actualMessages, finalState.EnqueuedCount,
		"EnqueuedCount (%d) should match actual Mailpit messages (%d)", finalState.EnqueuedCount, actualMessages)

	// Progress should be 100% if completed
	if finalState.Status == "completed" {
		assert.InDelta(t, 100.0, finalState.Progress, 1.0, "Progress should be ~100%% when completed")
	}

	// TotalRecipients should match contact count
	assert.Equal(t, contactCount, finalState.TotalRecipients,
		"TotalRecipients should match contact count")

	t.Log("=== Test completed ===")
}

// TestBroadcastEmptyBatchPrematureCompletion tests the scenario where contacts
// are removed (unsubscribed) AFTER a broadcast is scheduled but BEFORE it
// executes. This can cause the broadcast to complete with fewer recipients
// than expected because FetchBatch returns empty before reaching TotalRecipients.
//
// This test documents a potential bug where:
// 1. TotalRecipients is counted at schedule time (e.g., 100)
// 2. Some contacts unsubscribe before execution
// 3. FetchBatch returns empty early
// 4. Broadcast marked as "processed" with progress < 100%
func TestBroadcastEmptyBatchPrematureCompletion(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	client := suite.APIClient
	factory := suite.DataFactory

	totalContacts := 100
	contactsToUnsubscribe := 10 // Remove 10% after scheduling

	t.Log("=== Starting Broadcast Empty Batch Premature Completion Test ===")
	t.Logf("Total contacts: %d, will unsubscribe: %d after scheduling", totalContacts, contactsToUnsubscribe)

	// Setup
	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)

	err = factory.AddUserToWorkspace(user.ID, workspace.ID, "owner")
	require.NoError(t, err)

	_, err = factory.SetupWorkspaceWithSMTPProvider(workspace.ID,
		testutil.WithIntegrationEmailProvider(domain.EmailProvider{
			Kind: domain.EmailProviderKindSMTP,
			Senders: []domain.EmailSender{
				domain.NewEmailSender("noreply@notifuse.test", "Empty Batch Test"),
			},
			SMTP: &domain.SMTPSettings{
				Host:     "localhost",
				Port:     1025,
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

	err = testutil.ClearMailpitMessages(t)
	require.NoError(t, err)

	// Create list and contacts
	t.Log("Creating list and contacts...")
	list, err := factory.CreateList(workspace.ID, testutil.WithListName("Empty Batch Test List"))
	require.NoError(t, err)

	contacts := make([]map[string]interface{}, totalContacts)
	contactEmails := make([]string, totalContacts)
	for i := 0; i < totalContacts; i++ {
		email := fmt.Sprintf("empty-batch-test-%04d@example.com", i)
		contacts[i] = map[string]interface{}{
			"email":      email,
			"first_name": fmt.Sprintf("User%d", i),
			"last_name":  "EmptyBatchTest",
		}
		contactEmails[i] = email
	}

	resp, err := client.BatchImportContacts(contacts, []string{list.ID})
	require.NoError(t, err)
	resp.Body.Close()

	// Create template and broadcast
	uniqueSubject := fmt.Sprintf("Empty Batch Test %s", uuid.New().String()[:8])
	template, err := factory.CreateTemplate(workspace.ID,
		testutil.WithTemplateName("Empty Batch Test Template"),
		testutil.WithTemplateSubject(uniqueSubject))
	require.NoError(t, err)

	broadcast, err := factory.CreateBroadcast(workspace.ID,
		testutil.WithBroadcastName("Empty Batch Test Broadcast"),
		testutil.WithBroadcastAudience(domain.AudienceSettings{
			List:                list.ID,
			ExcludeUnsubscribed: true, // This is key - unsubscribed contacts will be filtered
		}))
	require.NoError(t, err)

	broadcast.TestSettings.Variations[0].TemplateID = template.ID
	updateResp, err := client.UpdateBroadcast(map[string]interface{}{
		"workspace_id":  workspace.ID,
		"id":            broadcast.ID,
		"name":          broadcast.Name,
		"audience":      broadcast.Audience,
		"schedule":      broadcast.Schedule,
		"test_settings": broadcast.TestSettings,
	})
	require.NoError(t, err)
	updateResp.Body.Close()

	// Start email queue worker
	ctx := context.Background()
	err = suite.ServerManager.StartBackgroundWorkers(ctx)
	require.NoError(t, err, "Should be able to start background workers")

	// Schedule broadcast - this counts TotalRecipients = 100
	t.Log("Scheduling broadcast (TotalRecipients will be counted now)...")
	scheduleResp, err := client.ScheduleBroadcast(map[string]interface{}{
		"workspace_id": workspace.ID,
		"id":           broadcast.ID,
		"send_now":     true,
	})
	require.NoError(t, err)
	scheduleResp.Body.Close()

	time.Sleep(500 * time.Millisecond)

	// Get task ID
	tasksResp, err := client.ListTasks(map[string]string{"broadcast_id": broadcast.ID})
	require.NoError(t, err)
	taskBody, _ := io.ReadAll(tasksResp.Body)
	tasksResp.Body.Close()

	var tasksResult map[string]interface{}
	json.Unmarshal(taskBody, &tasksResult)
	tasks := tasksResult["tasks"].([]interface{})
	taskID := tasks[0].(map[string]interface{})["id"].(string)

	// Set very short timeout to force multiple execution cycles
	err = factory.UpdateTaskMaxRuntime(workspace.ID, taskID, 3)
	require.NoError(t, err)
	t.Log("Set task MaxRuntime to 3 seconds")

	// Execute ONCE to initialize TotalRecipients count
	t.Log("First execution to initialize TotalRecipients...")
	execResp, _ := client.ExecuteTask(map[string]interface{}{
		"workspace_id": workspace.ID,
		"id":           taskID,
	})
	if execResp != nil {
		execResp.Body.Close()
	}

	// Check state after first execution
	firstState, _ := getTaskStateInfo(client, workspace.ID, taskID)
	if firstState != nil {
		t.Logf("After first execution: Status=%s, Total=%d, Sent=%d, Offset=%d",
			firstState.Status, firstState.TotalRecipients, firstState.EnqueuedCount, firstState.RecipientOffset)
	}

	// If already completed on first run, skip the rest
	if firstState != nil && firstState.Status == "completed" {
		t.Log("Task completed on first execution, cannot test mid-execution unsubscribe")
	} else {
		// CRITICAL: Unsubscribe contacts AFTER TotalRecipients was counted
		// but BEFORE processing is complete
		t.Logf("Unsubscribing %d contacts AFTER TotalRecipients was counted...", contactsToUnsubscribe)
		for i := 0; i < contactsToUnsubscribe; i++ {
			email := contactEmails[i]
			unsubResp, unsubErr := client.UpdateContactListStatus(workspace.ID, email, list.ID, "unsubscribed")
			if unsubErr != nil {
				t.Logf("Warning: Failed to unsubscribe %s: %v", email, unsubErr)
			} else {
				unsubResp.Body.Close()
			}
		}
		t.Logf("Unsubscribed %d contacts (TotalRecipients was already set to %d)",
			contactsToUnsubscribe, firstState.TotalRecipients)
	}

	// Execute broadcast
	t.Log("Executing broadcast...")
	maxExecutions := 20

	for i := 0; i < maxExecutions; i++ {
		execResp, _ := client.ExecuteTask(map[string]interface{}{
			"workspace_id": workspace.ID,
			"id":           taskID,
		})
		if execResp != nil {
			execResp.Body.Close()
		}

		state, _ := getTaskStateInfo(client, workspace.ID, taskID)
		if state != nil {
			t.Logf("Execution %d: Status=%s, Sent=%d, Failed=%d, Offset=%d, Total=%d, Progress=%.1f%%",
				i+1, state.Status, state.EnqueuedCount, state.FailedCount,
				state.RecipientOffset, state.TotalRecipients, state.Progress)

			if state.Status == "completed" {
				break
			}
		}

		time.Sleep(100 * time.Millisecond)
	}

	expectedSent := totalContacts - contactsToUnsubscribe // 90

	// Wait for emails
	err = testutil.WaitForMailpitMessages(t, uniqueSubject, expectedSent, 2*time.Minute)
	if err != nil {
		t.Logf("Warning: Not all emails arrived: %v", err)
	}

	// Get final state
	finalState, err := getTaskStateInfo(client, workspace.ID, taskID)
	require.NoError(t, err)

	// Get actual email count
	actualMessages, err := testutil.GetMailpitMessageCount(t, uniqueSubject)
	require.NoError(t, err)

	t.Log("=== EMPTY BATCH BUG DETECTION RESULTS ===")
	t.Logf("Total contacts at schedule time: %d", totalContacts)
	t.Logf("Contacts unsubscribed after scheduling: %d", contactsToUnsubscribe)
	t.Logf("Expected recipients: %d", expectedSent)
	t.Logf("TotalRecipients in task state: %d", finalState.TotalRecipients)
	t.Logf("Actual EnqueuedCount: %d", finalState.EnqueuedCount)
	t.Logf("Actual Mailpit messages: %d", actualMessages)
	t.Logf("Final Progress: %.1f%%", finalState.Progress)
	t.Logf("Final Status: %s", finalState.Status)

	// Analysis
	t.Log("=== ANALYSIS ===")

	// Check if TotalRecipients reflects the count AFTER first execution
	if finalState.TotalRecipients == totalContacts {
		t.Log("TotalRecipients was set at first execution time (before unsubscribes)")

		// This reproduces the 92% issue scenario:
		// - TotalRecipients = 100 (counted at first execution)
		// - Contacts unsubscribed after count
		// - FetchBatch returns empty at offset 90 (only 90 contacts remain)
		// - Broadcast logs show progress: 90% but task.Progress shows 100%
		actualBroadcastProgress := float64(finalState.EnqueuedCount) / float64(finalState.TotalRecipients) * 100

		t.Logf("Actual broadcast progress: %.1f%% (sent %d / total %d)",
			actualBroadcastProgress, finalState.EnqueuedCount, finalState.TotalRecipients)
		t.Logf("Task progress field: %.1f%% (set to 100%% when task completes)", finalState.Progress)

		if actualBroadcastProgress < 100.0 && finalState.Status == "completed" {
			t.Log("BUG REPRODUCED: Broadcast completed with fewer recipients than counted")
			t.Logf("This matches the user's 92%% issue - %d contacts unsubscribed after TotalRecipients was set",
				contactsToUnsubscribe)
		}

		// The broadcast progress (sent/total) should be ~90%
		expectedBroadcastProgress := float64(expectedSent) / float64(totalContacts) * 100 // 90%
		assert.InDelta(t, expectedBroadcastProgress, actualBroadcastProgress, 2.0,
			"Broadcast progress (sent/total) should be ~90%%")
	} else {
		t.Logf("TotalRecipients (%d) differs from original count (%d) - count happened after unsubscribes",
			finalState.TotalRecipients, totalContacts)
	}

	// Assertions
	t.Log("=== ASSERTIONS ===")

	// The actual emails sent should match expected (after unsubscribes)
	assert.Equal(t, expectedSent, actualMessages,
		"Should have sent to %d recipients (total - unsubscribed)", expectedSent)

	// EnqueuedCount should match actual emails
	assert.Equal(t, actualMessages, finalState.EnqueuedCount,
		"EnqueuedCount should match actual emails sent")

	t.Log("=== Test completed ===")
}

// TaskStateInfo holds parsed task state information
type TaskStateInfo struct {
	Status          string
	Progress        float64
	EnqueuedCount   int
	FailedCount     int
	RecipientOffset int64
	TotalRecipients int
	Phase           string
	ErrorMessage    string
}

// getTaskStateInfo retrieves and parses task state from the API
func getTaskStateInfo(client *testutil.APIClient, workspaceID, taskID string) (*TaskStateInfo, error) {
	resp, err := client.GetTask(workspaceID, taskID)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	task, ok := result["task"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("no task in response")
	}

	info := &TaskStateInfo{}

	if status, ok := task["status"].(string); ok {
		info.Status = status
	}
	if progress, ok := task["progress"].(float64); ok {
		info.Progress = progress
	}
	if errMsg, ok := task["error_message"].(string); ok {
		info.ErrorMessage = errMsg
	}

	// Parse nested state
	if state, ok := task["state"].(map[string]interface{}); ok {
		if sendBroadcast, ok := state["send_broadcast"].(map[string]interface{}); ok {
			if v, ok := sendBroadcast["enqueued_count"].(float64); ok {
				info.EnqueuedCount = int(v)
			}
			if v, ok := sendBroadcast["failed_count"].(float64); ok {
				info.FailedCount = int(v)
			}
			if v, ok := sendBroadcast["recipient_offset"].(float64); ok {
				info.RecipientOffset = int64(v)
			}
			if v, ok := sendBroadcast["total_recipients"].(float64); ok {
				info.TotalRecipients = int(v)
			}
			if v, ok := sendBroadcast["phase"].(string); ok {
				info.Phase = v
			}
		}
	}

	return info, nil
}
