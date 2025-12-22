//go:build integration

package integration

import (
	"encoding/json"
	"fmt"
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

// TestScheduledBroadcast_TaskCreation tests that when a broadcast is scheduled
// for a future time, a task is properly created with the correct NextRunAfter
// timestamp. This test verifies issue #171 - scheduled broadcasts not being sent.
func TestScheduledBroadcast_TaskCreation(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	client := suite.APIClient
	factory := suite.DataFactory

	// Setup: Create user and workspace
	t.Log("Setting up user and workspace...")
	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)

	err = factory.AddUserToWorkspace(user.ID, workspace.ID, "owner")
	require.NoError(t, err)

	// Setup SMTP provider
	t.Log("Setting up SMTP provider...")
	_, err = factory.SetupWorkspaceWithSMTPProvider(workspace.ID,
		testutil.WithIntegrationEmailProvider(domain.EmailProvider{
			Kind: domain.EmailProviderKindSMTP,
			Senders: []domain.EmailSender{
				domain.NewEmailSender("noreply@notifuse.test", "Scheduled Test"),
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

	// Login
	err = client.Login(user.Email, "password")
	require.NoError(t, err)
	client.SetWorkspaceID(workspace.ID)

	t.Run("task is created with correct NextRunAfter when scheduling broadcast with Europe/Paris timezone", func(t *testing.T) {
		// Clean up any existing tasks
		err := testutil.CleanupAllTasks(t, client, workspace.ID)
		require.NoError(t, err)

		// Create list
		list, err := factory.CreateList(workspace.ID,
			testutil.WithListName("Scheduled Test List"))
		require.NoError(t, err)

		// Create contact
		contactEmail := fmt.Sprintf("scheduled-test-%s@example.com", uuid.New().String()[:8])
		contact, err := factory.CreateContact(workspace.ID,
			testutil.WithContactEmail(contactEmail))
		require.NoError(t, err)

		// Add contact to list
		_, err = factory.CreateContactList(workspace.ID,
			testutil.WithContactListEmail(contact.Email),
			testutil.WithContactListListID(list.ID),
			testutil.WithContactListStatus(domain.ContactListStatusActive))
		require.NoError(t, err)

		// Create template
		template, err := factory.CreateTemplate(workspace.ID)
		require.NoError(t, err)

		// Create broadcast
		broadcast, err := factory.CreateBroadcast(workspace.ID,
			testutil.WithBroadcastName("Scheduled Future Broadcast"),
			testutil.WithBroadcastAudience(domain.AudienceSettings{
				List:                list.ID,
				ExcludeUnsubscribed: true,
			}))
		require.NoError(t, err)

		// Update broadcast to use our template
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
		updateResp.Body.Close()

		// Schedule broadcast for tomorrow at 10:00 AM Europe/Paris
		// Europe/Paris in December is UTC+1 (no DST)
		// So 10:00 AM Paris = 09:00 AM UTC
		tomorrow := time.Now().AddDate(0, 0, 1)
		scheduledDate := tomorrow.Format("2006-01-02")
		scheduledTime := "10:00"
		timezone := "Europe/Paris"

		t.Logf("Scheduling broadcast for %s %s %s", scheduledDate, scheduledTime, timezone)

		scheduleResp, err := client.ScheduleBroadcast(map[string]interface{}{
			"workspace_id":           workspace.ID,
			"id":                     broadcast.ID,
			"send_now":               false,
			"scheduled_date":         scheduledDate,
			"scheduled_time":         scheduledTime,
			"timezone":               timezone,
			"use_recipient_timezone": false,
		})
		require.NoError(t, err)
		defer scheduleResp.Body.Close()
		require.Equal(t, http.StatusOK, scheduleResp.StatusCode, "Schedule request should succeed")

		// Give event handler time to create the task
		time.Sleep(2 * time.Second)

		// Query tasks for this broadcast
		tasksResp, err := client.ListTasks(map[string]string{
			"workspace_id": workspace.ID,
			"type":         "send_broadcast",
		})
		require.NoError(t, err)
		defer tasksResp.Body.Close()

		var tasksResult map[string]interface{}
		err = json.NewDecoder(tasksResp.Body).Decode(&tasksResult)
		require.NoError(t, err)

		// Find task with matching broadcast_id
		tasks, ok := tasksResult["tasks"].([]interface{})
		require.True(t, ok, "Expected tasks array in response")

		var foundTask map[string]interface{}
		for _, taskInterface := range tasks {
			task := taskInterface.(map[string]interface{})
			if broadcastID, exists := task["broadcast_id"]; exists && broadcastID == broadcast.ID {
				foundTask = task
				break
			}
		}

		// CRITICAL ASSERTION 1: Task must exist
		require.NotNil(t, foundTask, "Task should be created for scheduled broadcast (BroadcastID: %s)", broadcast.ID)
		t.Logf("Found task: %+v", foundTask)

		// CRITICAL ASSERTION 2: Task status should be pending
		status := foundTask["status"].(string)
		assert.Equal(t, "pending", status, "Task status should be 'pending'")

		// CRITICAL ASSERTION 3: NextRunAfter should NOT be nil
		nextRunAfter := foundTask["next_run_after"]
		require.NotNil(t, nextRunAfter, "Task.NextRunAfter should NOT be nil for scheduled broadcast")
		t.Logf("Task NextRunAfter: %v", nextRunAfter)

		// CRITICAL ASSERTION 4: NextRunAfter should match expected UTC time
		// Parse the next_run_after from the task
		nextRunAfterStr := nextRunAfter.(string)
		parsedNextRunAfter, err := time.Parse(time.RFC3339, nextRunAfterStr)
		require.NoError(t, err, "Should be able to parse next_run_after as RFC3339")

		// Calculate expected time in UTC
		// Tomorrow at 10:00 Europe/Paris = Tomorrow at 09:00 UTC (in December, UTC+1)
		loc, err := time.LoadLocation("Europe/Paris")
		require.NoError(t, err)

		expectedLocalTime := time.Date(
			tomorrow.Year(), tomorrow.Month(), tomorrow.Day(),
			10, 0, 0, 0, loc)
		expectedUTC := expectedLocalTime.UTC()

		t.Logf("Expected NextRunAfter (UTC): %v", expectedUTC)
		t.Logf("Actual NextRunAfter (UTC):   %v", parsedNextRunAfter.UTC())

		// Allow 1 second tolerance for timing differences
		timeDiff := parsedNextRunAfter.UTC().Sub(expectedUTC)
		assert.LessOrEqual(t, timeDiff.Abs().Seconds(), 1.0,
			"NextRunAfter should match expected UTC time within 1 second tolerance. Expected: %v, Got: %v",
			expectedUTC, parsedNextRunAfter.UTC())
	})

	t.Run("task is created with correct NextRunAfter when scheduling with UTC timezone", func(t *testing.T) {
		// Clean up any existing tasks
		err := testutil.CleanupAllTasks(t, client, workspace.ID)
		require.NoError(t, err)

		// Create list
		list, err := factory.CreateList(workspace.ID,
			testutil.WithListName("UTC Scheduled Test List"))
		require.NoError(t, err)

		// Create contact
		contactEmail := fmt.Sprintf("utc-test-%s@example.com", uuid.New().String()[:8])
		contact, err := factory.CreateContact(workspace.ID,
			testutil.WithContactEmail(contactEmail))
		require.NoError(t, err)

		// Add contact to list
		_, err = factory.CreateContactList(workspace.ID,
			testutil.WithContactListEmail(contact.Email),
			testutil.WithContactListListID(list.ID),
			testutil.WithContactListStatus(domain.ContactListStatusActive))
		require.NoError(t, err)

		// Create template
		template, err := factory.CreateTemplate(workspace.ID)
		require.NoError(t, err)

		// Create broadcast
		broadcast, err := factory.CreateBroadcast(workspace.ID,
			testutil.WithBroadcastName("UTC Scheduled Broadcast"),
			testutil.WithBroadcastAudience(domain.AudienceSettings{
				List:                list.ID,
				ExcludeUnsubscribed: true,
			}))
		require.NoError(t, err)

		// Update broadcast to use our template
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
		updateResp.Body.Close()

		// Schedule broadcast for tomorrow at 14:00 UTC
		tomorrow := time.Now().AddDate(0, 0, 1)
		scheduledDate := tomorrow.Format("2006-01-02")
		scheduledTime := "14:00"
		timezone := "UTC"

		t.Logf("Scheduling broadcast for %s %s %s", scheduledDate, scheduledTime, timezone)

		scheduleResp, err := client.ScheduleBroadcast(map[string]interface{}{
			"workspace_id":           workspace.ID,
			"id":                     broadcast.ID,
			"send_now":               false,
			"scheduled_date":         scheduledDate,
			"scheduled_time":         scheduledTime,
			"timezone":               timezone,
			"use_recipient_timezone": false,
		})
		require.NoError(t, err)
		defer scheduleResp.Body.Close()
		require.Equal(t, http.StatusOK, scheduleResp.StatusCode, "Schedule request should succeed")

		// Give event handler time to create the task
		time.Sleep(2 * time.Second)

		// Query tasks for this broadcast
		tasksResp, err := client.ListTasks(map[string]string{
			"workspace_id": workspace.ID,
			"type":         "send_broadcast",
		})
		require.NoError(t, err)
		defer tasksResp.Body.Close()

		var tasksResult map[string]interface{}
		err = json.NewDecoder(tasksResp.Body).Decode(&tasksResult)
		require.NoError(t, err)

		// Find task with matching broadcast_id
		tasks, ok := tasksResult["tasks"].([]interface{})
		require.True(t, ok, "Expected tasks array in response")

		var foundTask map[string]interface{}
		for _, taskInterface := range tasks {
			task := taskInterface.(map[string]interface{})
			if broadcastID, exists := task["broadcast_id"]; exists && broadcastID == broadcast.ID {
				foundTask = task
				break
			}
		}

		// CRITICAL ASSERTION: Task must exist with NextRunAfter set
		require.NotNil(t, foundTask, "Task should be created for scheduled broadcast")

		nextRunAfter := foundTask["next_run_after"]
		require.NotNil(t, nextRunAfter, "Task.NextRunAfter should NOT be nil for scheduled broadcast")

		// Parse and verify
		nextRunAfterStr := nextRunAfter.(string)
		parsedNextRunAfter, err := time.Parse(time.RFC3339, nextRunAfterStr)
		require.NoError(t, err)

		// Expected: tomorrow at 14:00 UTC
		expectedUTC := time.Date(
			tomorrow.Year(), tomorrow.Month(), tomorrow.Day(),
			14, 0, 0, 0, time.UTC)

		timeDiff := parsedNextRunAfter.UTC().Sub(expectedUTC)
		assert.LessOrEqual(t, timeDiff.Abs().Seconds(), 1.0,
			"NextRunAfter should match expected UTC time. Expected: %v, Got: %v",
			expectedUTC, parsedNextRunAfter.UTC())
	})

	t.Run("immediate send (send_now=true) creates task that runs immediately", func(t *testing.T) {
		// This is a control test to verify the scheduler works for immediate sends
		// Clean up any existing tasks
		err := testutil.CleanupAllTasks(t, client, workspace.ID)
		require.NoError(t, err)

		// Create list
		list, err := factory.CreateList(workspace.ID,
			testutil.WithListName("Immediate Send Test List"))
		require.NoError(t, err)

		// Create contact
		contactEmail := fmt.Sprintf("immediate-test-%s@example.com", uuid.New().String()[:8])
		contact, err := factory.CreateContact(workspace.ID,
			testutil.WithContactEmail(contactEmail))
		require.NoError(t, err)

		// Add contact to list
		_, err = factory.CreateContactList(workspace.ID,
			testutil.WithContactListEmail(contact.Email),
			testutil.WithContactListListID(list.ID),
			testutil.WithContactListStatus(domain.ContactListStatusActive))
		require.NoError(t, err)

		// Create template
		template, err := factory.CreateTemplate(workspace.ID)
		require.NoError(t, err)

		// Create broadcast
		broadcast, err := factory.CreateBroadcast(workspace.ID,
			testutil.WithBroadcastName("Immediate Send Broadcast"),
			testutil.WithBroadcastAudience(domain.AudienceSettings{
				List:                list.ID,
				ExcludeUnsubscribed: true,
			}))
		require.NoError(t, err)

		// Update broadcast to use our template
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
		updateResp.Body.Close()

		// Schedule broadcast to send immediately
		t.Log("Scheduling broadcast to send immediately (send_now=true)...")
		scheduleResp, err := client.ScheduleBroadcast(map[string]interface{}{
			"workspace_id": workspace.ID,
			"id":           broadcast.ID,
			"send_now":     true,
		})
		require.NoError(t, err)
		defer scheduleResp.Body.Close()
		require.Equal(t, http.StatusOK, scheduleResp.StatusCode)

		// Wait for broadcast completion
		t.Log("Waiting for broadcast to complete...")
		finalStatus, err := testutil.WaitForBroadcastStatusWithExecution(t, client, broadcast.ID,
			[]string{"processed", "completed"}, 60*time.Second)
		require.NoError(t, err, "Broadcast should complete successfully")

		assert.Contains(t, []string{"processed", "completed"}, finalStatus,
			"Immediate send should complete successfully")
		t.Logf("Broadcast completed with status: %s", finalStatus)
	})
}
