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

// TestScheduledBroadcast_PastDue_IssuesGH317 reproduces the end-to-end flow from
// https://github.com/Notifuse/notifuse/issues/317 — a broadcast whose scheduled
// time has already passed is supposed to be picked up by the scheduler's
// ExecutePendingTasks loop and processed to completion.
//
// Reported symptoms (v29.5, Docker Compose, Postgres 17, multi-workspace):
//   - scheduler dispatches via HTTP every ~20s, logs 200 OK ("dispatched successfully")
//   - the task.execute handler never appears to actually run end-to-end
//   - task stays at status=pending, progress=0; broadcast stays at status=scheduled,
//     enqueued_count=0
//   - curl-ing /api/tasks.execute manually unblocks it immediately
//   - also affects process_contact_segment_queue (not broadcast-specific)
//
// What this test does:
//  1. Creates a scheduled broadcast with a past-due scheduled_date/scheduled_time
//     so the resulting send_broadcast task has next_run_after in the past.
//  2. Drives GET /api/cron (the same entry point the internal scheduler hits
//     via HTTP dispatch) repeatedly.
//  3. Asserts the broadcast reaches a non-scheduled status within the polling
//     window — i.e. the cron loop both picks up and executes the task.
//
// NOTE: the test harness runs with APIEndpoint="" (see tests/testutil/server.go),
// which means ExecutePendingTasks takes the "direct execution" branch rather than
// the HTTP-dispatch branch the issue is reporting. This test therefore validates
// the orchestrator-level flow (two-tick init → send pattern) end-to-end, but
// cannot by itself reproduce an HTTP-dispatch-specific regression. See the
// subtest "state_persists_across_ticks" below for coverage of the most plausible
// silent-failure mode in the shared path.
func TestScheduledBroadcast_PastDue_IssuesGH317(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	client := suite.APIClient
	factory := suite.DataFactory

	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	require.NoError(t, factory.AddUserToWorkspace(user.ID, workspace.ID, "owner"))

	_, err = factory.SetupWorkspaceWithSMTPProvider(workspace.ID,
		testutil.WithIntegrationEmailProvider(domain.EmailProvider{
			Kind: domain.EmailProviderKindSMTP,
			Senders: []domain.EmailSender{
				domain.NewEmailSender("noreply@notifuse.test", "Past-Due Test"),
			},
			SMTP: &domain.SMTPSettings{
				Host:   "localhost",
				Port:   1025,
				UseTLS: false,
			},
			RateLimitPerMinute: 2000,
		}))
	require.NoError(t, err)

	require.NoError(t, client.Login(user.Email, "password"))
	client.SetWorkspaceID(workspace.ID)

	// A single recipient is enough to observe whether the orchestrator enqueues
	// anything at all — the failure mode in the issue is "nothing ever happens".
	list, err := factory.CreateList(workspace.ID, testutil.WithListName("Past-Due List"))
	require.NoError(t, err)

	contactEmail := fmt.Sprintf("past-due-%s@example.com", uuid.New().String()[:8])
	contact, err := factory.CreateContact(workspace.ID, testutil.WithContactEmail(contactEmail))
	require.NoError(t, err)

	_, err = factory.CreateContactList(workspace.ID,
		testutil.WithContactListEmail(contact.Email),
		testutil.WithContactListListID(list.ID),
		testutil.WithContactListStatus(domain.ContactListStatusActive))
	require.NoError(t, err)

	template, err := factory.CreateTemplate(workspace.ID)
	require.NoError(t, err)

	broadcast, err := factory.CreateBroadcast(workspace.ID,
		testutil.WithBroadcastName("Past-Due Scheduled Broadcast"),
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

	// Schedule the broadcast for two minutes ago in UTC — well past due. The
	// ScheduleBroadcastRequest validator accepts past dates (see
	// domain/broadcast.go:ScheduleBroadcastRequest.Validate) and produces a task
	// whose next_run_after is in the past, matching the reported scenario.
	pastTime := time.Now().UTC().Add(-2 * time.Minute)
	scheduledDate := pastTime.Format("2006-01-02")
	scheduledTime := pastTime.Format("15:04")

	t.Logf("Scheduling broadcast for past time: %s %s UTC", scheduledDate, scheduledTime)

	scheduleResp, err := client.ScheduleBroadcast(map[string]interface{}{
		"workspace_id":           workspace.ID,
		"id":                     broadcast.ID,
		"send_now":               false,
		"scheduled_date":         scheduledDate,
		"scheduled_time":         scheduledTime,
		"timezone":               "UTC",
		"use_recipient_timezone": false,
	})
	require.NoError(t, err)
	defer scheduleResp.Body.Close()
	require.Equal(t, http.StatusOK, scheduleResp.StatusCode, "Schedule request should succeed")

	// Give the event handler time to create the send_broadcast task.
	time.Sleep(2 * time.Second)

	taskID, taskNextRunAfter := findBroadcastTask(t, client, workspace.ID, broadcast.ID)
	require.NotEmpty(t, taskID, "send_broadcast task should be created for the scheduled broadcast")
	require.False(t, taskNextRunAfter.IsZero(), "task.NextRunAfter should be set for a scheduled broadcast")
	assert.True(t, taskNextRunAfter.Before(time.Now().UTC()),
		"task.NextRunAfter (%s) should be in the past — we scheduled for -2min", taskNextRunAfter)

	t.Run("state_persists_across_ticks", func(t *testing.T) {
		// On tick 1 the orchestrator takes the early-return path in
		// orchestrator.go:Process (around line 677): it populates
		// broadcastState.TotalRecipients, assigns it back to task.State.SendBroadcast,
		// and returns (false, nil) without enqueueing anyone. TaskService's
		// completed=false branch then calls MarkAsPending and persists task.State
		// to the DB.
		//
		// The issue's symptoms are consistent with this persisted state being lost
		// or ignored on tick 2, forcing every subsequent tick to re-enter the
		// init path and return early again — which would look exactly like
		// "dispatched successfully every 20s, progress never advances".
		//
		// This subtest pins down the invariant: after one cron tick the state
		// row must carry TotalRecipients>0 for the task, so tick 2 bypasses the
		// init block and actually sends.
		execResp, err := client.ExecutePendingTasks(10)
		require.NoError(t, err)
		execResp.Body.Close()

		// Short settle so the task-service goroutine finishes its MarkAsPending.
		time.Sleep(500 * time.Millisecond)

		taskStatus, taskProgress, sendState := getTaskStateDetail(t, client, workspace.ID, taskID)
		t.Logf("after tick 1: task_status=%s progress=%.1f send_state=%+v", taskStatus, taskProgress, sendState)

		assert.Equal(t, string(domain.TaskStatusPending), taskStatus,
			"after tick 1 the task should be re-scheduled as pending for tick 2")
		require.NotNil(t, sendState, "task.State.SendBroadcast should be persisted after tick 1")
		assert.Greater(t, sendState["total_recipients"], float64(0),
			"tick 1 must persist TotalRecipients — otherwise tick 2 re-enters the init block and nothing ever sends (matches issue #317 symptom)")
	})

	// Drive cron for up to 30s (the issue describes ~20s scheduler ticks). We poll
	// for the broadcast to transition out of "scheduled" into any non-scheduled
	// state. Each iteration calls /api/cron which invokes
	// TaskService.ExecutePendingTasks, exactly as the internal scheduler would.
	finalBroadcastStatus, finalTaskStatus, finalProgress, finalEnqueued := "", "", 0.0, 0
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		execResp, execErr := client.ExecutePendingTasks(10)
		if execErr == nil {
			execResp.Body.Close()
		}
		time.Sleep(1 * time.Second)

		finalBroadcastStatus, finalEnqueued = getBroadcastStatusAndEnqueued(t, client, broadcast.ID)
		finalTaskStatus, finalProgress = getTaskStatusAndProgress(t, client, workspace.ID, taskID)
		t.Logf("poll: broadcast_status=%s enqueued=%d | task_status=%s progress=%.1f",
			finalBroadcastStatus, finalEnqueued, finalTaskStatus, finalProgress)

		if finalBroadcastStatus != string(domain.BroadcastStatusScheduled) &&
			finalBroadcastStatus != string(domain.BroadcastStatusDraft) {
			break
		}
	}

	// If the bug from #317 is present, we'd observe the symptom pattern:
	//   broadcast.status == "scheduled", enqueued_count == 0,
	//   task.status == "pending",       progress == 0.
	// Flag that explicitly so the failure mode matches the issue's wording.
	if finalBroadcastStatus == string(domain.BroadcastStatusScheduled) &&
		finalEnqueued == 0 &&
		finalTaskStatus == string(domain.TaskStatusPending) &&
		finalProgress == 0 {
		t.Fatalf("REPRODUCED #317: past-due scheduled broadcast never advanced — "+
			"broadcast.status=%s enqueued=%d task.status=%s progress=%.1f",
			finalBroadcastStatus, finalEnqueued, finalTaskStatus, finalProgress)
	}

	assert.Contains(t,
		[]string{string(domain.BroadcastStatusProcessed), string(domain.BroadcastStatusProcessing)},
		finalBroadcastStatus,
		"past-due scheduled broadcast should reach processing/processed; got %q", finalBroadcastStatus)
}

// findBroadcastTask returns the send_broadcast task for a broadcast and its
// current next_run_after (zero-value if unset).
func findBroadcastTask(t *testing.T, client *testutil.APIClient, workspaceID, broadcastID string) (string, time.Time) {
	t.Helper()
	resp, err := client.ListTasks(map[string]string{
		"workspace_id": workspaceID,
		"type":         "send_broadcast",
	})
	require.NoError(t, err)
	defer resp.Body.Close()

	var body map[string]interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))

	tasks, _ := body["tasks"].([]interface{})
	for _, ti := range tasks {
		task := ti.(map[string]interface{})
		if bid, ok := task["broadcast_id"].(string); ok && bid == broadcastID {
			id, _ := task["id"].(string)
			var nra time.Time
			if raw, ok := task["next_run_after"].(string); ok && raw != "" {
				if parsed, err := time.Parse(time.RFC3339, raw); err == nil {
					nra = parsed
				}
			}
			return id, nra
		}
	}
	return "", time.Time{}
}

func getTaskStatusAndProgress(t *testing.T, client *testutil.APIClient, workspaceID, taskID string) (string, float64) {
	t.Helper()
	status, progress, _ := getTaskStateDetail(t, client, workspaceID, taskID)
	return status, progress
}

// getTaskStateDetail returns status, progress, and the send_broadcast sub-state
// (as a map[string]interface{} — values come from JSON unmarshalling so counts
// are float64). Returns (", "", 0, nil) if the task can't be fetched/decoded.
func getTaskStateDetail(t *testing.T, client *testutil.APIClient, workspaceID, taskID string) (string, float64, map[string]interface{}) {
	t.Helper()
	resp, err := client.Get(fmt.Sprintf("/api/tasks.get?workspace_id=%s&id=%s", workspaceID, taskID))
	require.NoError(t, err)
	defer resp.Body.Close()

	var body map[string]interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))

	task, _ := body["task"].(map[string]interface{})
	if task == nil {
		return "", 0, nil
	}
	status, _ := task["status"].(string)
	progress, _ := task["progress"].(float64)

	var send map[string]interface{}
	if state, ok := task["state"].(map[string]interface{}); ok {
		if s, ok := state["send_broadcast"].(map[string]interface{}); ok {
			send = s
		}
	}
	return status, progress, send
}

func getBroadcastStatusAndEnqueued(t *testing.T, client *testutil.APIClient, broadcastID string) (string, int) {
	t.Helper()
	resp, err := client.GetBroadcast(broadcastID)
	require.NoError(t, err)
	defer resp.Body.Close()

	var body map[string]interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))

	b, _ := body["broadcast"].(map[string]interface{})
	if b == nil {
		return "", 0
	}
	status, _ := b["status"].(string)
	enqueued := 0
	if v, ok := b["enqueued_count"].(float64); ok {
		enqueued = int(v)
	}
	return status, enqueued
}
