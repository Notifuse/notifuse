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

// TestTaskConcurrentExecutionRaceCondition verifies that MarkAsRunningTx
// properly prevents duplicate task execution.
//
// This test exposes a race condition in the current implementation where
// multiple executors can mark the same task as "running" and process it
// concurrently, leading to duplicate emails being sent.
//
// The bug is in internal/repository/task_postgres.go:MarkAsRunningTx which
// does NOT check if the task is already running before marking it.
//
// Expected behavior:
// - With current (buggy) code: Test may FAIL with duplicate emails
// - After fix: Test should PASS with exactly contactCount emails
func TestTaskConcurrentExecutionRaceCondition(t *testing.T) {
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

	t.Log("=== Starting Task Concurrent Execution Race Condition Test ===")
	t.Logf("Contact count: %d", contactCount)
	t.Log("This test verifies that concurrent task execution doesn't cause duplicate emails")

	// Step 1: Setup
	t.Log("Step 1: Creating user and workspace...")
	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)

	err = factory.AddUserToWorkspace(user.ID, workspace.ID, "owner")
	require.NoError(t, err)

	// Setup working SMTP with high rate limit
	_, err = factory.SetupWorkspaceWithSMTPProvider(workspace.ID,
		testutil.WithIntegrationEmailProvider(domain.EmailProvider{
			Kind: domain.EmailProviderKindSMTP,
			Senders: []domain.EmailSender{
				domain.NewEmailSender("noreply@notifuse.test", "Race Condition Test"),
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
		testutil.WithListName("Race Condition Test List"))
	require.NoError(t, err)

	contacts := make([]map[string]interface{}, contactCount)
	for i := 0; i < contactCount; i++ {
		contacts[i] = map[string]interface{}{
			"email":      fmt.Sprintf("race-test-%04d@example.com", i),
			"first_name": fmt.Sprintf("User%d", i),
			"last_name":  "RaceTest",
		}
	}

	resp, err := client.BatchImportContacts(contacts, []string{list.ID})
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Step 4: Create template with unique subject
	t.Log("Step 4: Creating template...")
	uniqueSubject := fmt.Sprintf("Race Condition Test %s", uuid.New().String()[:8])
	template, err := factory.CreateTemplate(workspace.ID,
		testutil.WithTemplateName("Race Condition Test Template"),
		testutil.WithTemplateSubject(uniqueSubject))
	require.NoError(t, err)

	// Step 5: Create broadcast
	t.Log("Step 5: Creating broadcast...")
	broadcast, err := factory.CreateBroadcast(workspace.ID,
		testutil.WithBroadcastName("Race Condition Test Broadcast"),
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

	// Step 6: Schedule broadcast (creates the task)
	t.Log("Step 6: Scheduling broadcast...")
	scheduleResp, err := client.ScheduleBroadcast(map[string]interface{}{
		"workspace_id": workspace.ID,
		"id":           broadcast.ID,
		"send_now":     true,
	})
	require.NoError(t, err)
	defer scheduleResp.Body.Close()

	// Give the system a moment to create the task
	time.Sleep(500 * time.Millisecond)

	// Step 7: Get the task ID
	t.Log("Step 7: Getting task ID...")
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

	// Step 8: KEY TEST - Concurrent direct task execution
	t.Log("Step 8: Launching CONCURRENT task executions...")
	t.Log("Each executor will try to execute the SAME task simultaneously")

	concurrentExecutors := 5
	var wg sync.WaitGroup
	executionResults := make([]int, concurrentExecutors)

	for i := 0; i < concurrentExecutors; i++ {
		wg.Add(1)
		go func(executorID int) {
			defer wg.Done()
			t.Logf("Executor %d: Starting task execution", executorID)

			execResp, execErr := client.ExecuteTask(map[string]interface{}{
				"workspace_id": workspace.ID,
				"id":           taskID,
			})

			if execErr != nil {
				t.Logf("Executor %d: Error - %v", executorID, execErr)
				executionResults[executorID] = -1
				return
			}
			defer execResp.Body.Close()

			executionResults[executorID] = execResp.StatusCode
			t.Logf("Executor %d: Completed with status %d", executorID, execResp.StatusCode)
		}(i)
	}

	wg.Wait()
	t.Log("All concurrent executors completed")

	// Log execution results
	successCount := 0
	for i, status := range executionResults {
		t.Logf("Executor %d result: %d", i, status)
		if status == http.StatusOK {
			successCount++
		}
	}
	t.Logf("Successful executions: %d / %d", successCount, concurrentExecutors)

	// Step 9: Wait for broadcast completion
	t.Log("Step 9: Waiting for broadcast to complete...")
	timeout := 3 * time.Minute
	deadline := time.Now().Add(timeout)
	var finalStatus string

	for time.Now().Before(deadline) {
		// Execute any remaining pending work
		_, _ = client.ExecutePendingTasks(10)
		time.Sleep(500 * time.Millisecond)

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

	// Step 10: Wait for emails to arrive
	t.Log("Step 10: Waiting for emails in Mailpit...")
	time.Sleep(5 * time.Second)

	// Step 11: Verify results
	t.Log("Step 11: Verifying results...")
	totalMessages, err := testutil.GetMailpitMessageCount(t, uniqueSubject)
	require.NoError(t, err)

	receivedEmails, err := testutil.GetAllMailpitRecipients(t, uniqueSubject)
	require.NoError(t, err)

	t.Log("=== RACE CONDITION TEST RESULTS ===")
	t.Logf("Expected contacts: %d", contactCount)
	t.Logf("Unique recipients: %d", len(receivedEmails))
	t.Logf("Total messages: %d", totalMessages)

	duplicates := totalMessages - len(receivedEmails)
	if duplicates > 0 {
		t.Logf("DUPLICATES DETECTED: %d duplicate emails!", duplicates)
	}

	// Calculate if we have MORE emails than contacts (race condition symptom)
	extraEmails := totalMessages - contactCount
	if extraEmails > 0 {
		t.Logf("RACE CONDITION DETECTED: %d extra emails sent!", extraEmails)
		t.Logf("This indicates multiple executors processed the same task concurrently")
	}

	// Final assertions
	t.Log("=== FINAL ASSERTIONS ===")

	// Primary assertion: No more emails than contacts
	assert.LessOrEqual(t, totalMessages, contactCount,
		"RACE CONDITION: More emails (%d) than contacts (%d) - duplicate execution detected!",
		totalMessages, contactCount)

	// Secondary assertion: No duplicates within what was sent
	assert.Equal(t, 0, duplicates,
		"No duplicate emails should be sent to the same recipient")

	// Ideal case: Exactly the right number of emails
	// Note: This may fail if the race condition causes some emails to be skipped
	if totalMessages < contactCount {
		t.Logf("Warning: Fewer emails (%d) than contacts (%d) - some may have been skipped",
			totalMessages, contactCount)
	}

	t.Log("=== Test completed ===")
}
