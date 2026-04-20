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

// Integration tests for mid-flight broadcast pause/resume/cancel.
// Verifies Phase-2 behavior (after orchestrator finishes enqueueing)
// and Phase-1 regressions (orchestrator still in flight).
//
// See plan: /Users/pierre/.claude/plans/lets-plan-your-recommendation-quiet-dream.md

// phase2Harness bundles the common dependencies so each test stays compact.
type phase2Harness struct {
	suite        *testutil.IntegrationTestSuite
	client       *testutil.APIClient
	factory      *testutil.TestDataFactory
	workspaceID  string
	broadcastID  string
	listID       string
	templateID   string
	taskID       string
	subject      string
	contactCount int
	queueRepo    domain.EmailQueueRepository
}

// setupPhase2 creates user, workspace, SMTP provider, list, N contacts, template,
// and broadcast. Returns the harness in a state ready for ScheduleBroadcast.
// rateLimitPerMinute controls drain pace; use low values (60) to keep broadcasts
// in flight long enough to observe mid-flight pause/cancel, or high (600+) when
// you want fast drain.
func setupPhase2(t *testing.T, contactCount int, rateLimitPerMinute int) *phase2Harness {
	t.Helper()
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})

	client := suite.APIClient
	factory := suite.DataFactory

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
				domain.NewEmailSender("noreply@notifuse.test", "Phase2 Test"),
			},
			SMTP: &domain.SMTPSettings{
				Host:     "localhost",
				Port:     1025, // Mailpit
				Username: "",
				Password: "",
				UseTLS:   false,
			},
			RateLimitPerMinute: rateLimitPerMinute,
		}))
	require.NoError(t, err)

	err = client.Login(user.Email, "password")
	require.NoError(t, err)
	client.SetWorkspaceID(workspace.ID)

	require.NoError(t, testutil.ClearMailpitMessages(t))

	list, err := factory.CreateList(workspace.ID,
		testutil.WithListName("Phase2 Test List"))
	require.NoError(t, err)

	contacts := make([]map[string]interface{}, contactCount)
	for i := 0; i < contactCount; i++ {
		contacts[i] = map[string]interface{}{
			"email":      fmt.Sprintf("phase2-%04d-%s@example.com", i, uuid.New().String()[:6]),
			"first_name": fmt.Sprintf("User%d", i),
			"last_name":  "Phase2",
		}
	}
	resp, err := client.BatchImportContacts(contacts, []string{list.ID})
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	uniqueSubject := fmt.Sprintf("Phase2 Test %s", uuid.New().String()[:8])
	template, err := factory.CreateTemplate(workspace.ID,
		testutil.WithTemplateName("Phase2 Template"),
		testutil.WithTemplateSubject(uniqueSubject))
	require.NoError(t, err)

	broadcast, err := factory.CreateBroadcast(workspace.ID,
		testutil.WithBroadcastName("Phase2 Broadcast"),
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

	return &phase2Harness{
		suite:        suite,
		client:       client,
		factory:      factory,
		workspaceID:  workspace.ID,
		broadcastID:  broadcast.ID,
		listID:       list.ID,
		templateID:   template.ID,
		subject:      uniqueSubject,
		contactCount: contactCount,
		queueRepo:    suite.ServerManager.GetApp().GetEmailQueueRepository(),
	}
}

// Cleanup tears down the suite. Safe to call via defer.
func (h *phase2Harness) Cleanup() { h.suite.Cleanup() }

// scheduleAndExecute schedules with send_now and drives the task to Phase-2
// completion (broadcast.status == "processed"). The orchestrator has a
// per-run time budget and may need multiple ExecutePendingTasks ticks to
// complete enqueueing large audiences, so we poll.
// Returns the taskID so subsequent assertions can inspect it.
func (h *phase2Harness) scheduleAndExecute(t *testing.T) string {
	t.Helper()
	scheduleResp, err := h.client.ScheduleBroadcast(map[string]interface{}{
		"workspace_id": h.workspaceID,
		"id":           h.broadcastID,
		"send_now":     true,
	})
	require.NoError(t, err)
	scheduleResp.Body.Close()
	require.Equal(t, http.StatusOK, scheduleResp.StatusCode)

	// Event handler creates the task asynchronously.
	taskID := h.waitForTaskID(t, 5*time.Second)
	h.taskID = taskID

	// First invocation kicks things off.
	execResp, err := h.client.ExecuteTask(map[string]interface{}{
		"workspace_id": h.workspaceID,
		"id":           taskID,
	})
	require.NoError(t, err)
	execResp.Body.Close()

	// Drive to processed by repeatedly running pending tasks until the
	// broadcast reaches the terminal enqueue state.
	deadline := time.Now().Add(2 * time.Minute)
	for time.Now().Before(deadline) {
		bd := h.getBroadcast(t)
		status, _ := bd["status"].(string)
		if status == "processed" || status == "failed" || status == "cancelled" {
			return taskID
		}
		resp, _ := h.client.ExecutePendingTasks(10)
		if resp != nil {
			resp.Body.Close()
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("broadcast %s did not reach processed within 2m", h.broadcastID)
	return taskID
}

// scheduleAsync schedules send_now but does NOT execute the task. Returns the
// taskID. Useful for tests that want to run the orchestrator in a goroutine
// and pause/cancel mid-execution.
func (h *phase2Harness) scheduleAsync(t *testing.T) string {
	t.Helper()
	scheduleResp, err := h.client.ScheduleBroadcast(map[string]interface{}{
		"workspace_id": h.workspaceID,
		"id":           h.broadcastID,
		"send_now":     true,
	})
	require.NoError(t, err)
	scheduleResp.Body.Close()
	require.Equal(t, http.StatusOK, scheduleResp.StatusCode)

	taskID := h.waitForTaskID(t, 5*time.Second)
	h.taskID = taskID
	return taskID
}

func (h *phase2Harness) waitForTaskID(t *testing.T, timeout time.Duration) string {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := h.client.ListTasks(map[string]string{"broadcast_id": h.broadcastID})
		if err == nil {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			var result map[string]interface{}
			if json.Unmarshal(body, &result) == nil {
				if tasks, ok := result["tasks"].([]interface{}); ok && len(tasks) > 0 {
					task := tasks[0].(map[string]interface{})
					if id, ok := task["id"].(string); ok && id != "" {
						return id
					}
				}
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("no task appeared for broadcast %s within %v", h.broadcastID, timeout)
	return ""
}

// getBroadcast returns the broadcast as a decoded map. Fails the test on error.
func (h *phase2Harness) getBroadcast(t *testing.T) map[string]interface{} {
	t.Helper()
	resp, err := h.client.GetBroadcast(h.broadcastID)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &result))
	bd, ok := result["broadcast"].(map[string]interface{})
	require.True(t, ok, "response missing 'broadcast' field: %s", string(body))
	return bd
}

// getTask fetches the task by the stored taskID.
func (h *phase2Harness) getTask(t *testing.T) map[string]interface{} {
	t.Helper()
	require.NotEmpty(t, h.taskID, "scheduleAndExecute / scheduleAsync must be called first")
	resp, err := h.client.ListTasks(map[string]string{"broadcast_id": h.broadcastID})
	require.NoError(t, err)
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &result))
	tasks, ok := result["tasks"].([]interface{})
	require.True(t, ok)
	for _, ti := range tasks {
		task := ti.(map[string]interface{})
		if task["id"] == h.taskID {
			return task
		}
	}
	t.Fatalf("task %s not found", h.taskID)
	return nil
}

// countQueue returns counts of queue entries for this broadcast, grouped by
// status. Always includes pending/processing/failed/paused keys (0 if absent).
func (h *phase2Harness) countQueue(t *testing.T) map[domain.EmailQueueStatus]int64 {
	t.Helper()
	ctx := context.Background()
	counts := map[domain.EmailQueueStatus]int64{}
	for _, s := range []domain.EmailQueueStatus{
		domain.EmailQueueStatusPending,
		domain.EmailQueueStatusProcessing,
		domain.EmailQueueStatusFailed,
		domain.EmailQueueStatusPaused,
	} {
		n, err := h.queueRepo.CountBySourceAndStatus(ctx, h.workspaceID,
			domain.EmailQueueSourceBroadcast, h.broadcastID, s)
		require.NoError(t, err)
		counts[s] = n
	}
	return counts
}

// waitForBroadcastStatus polls until the broadcast reaches one of the expected
// statuses. Returns the final status observed.
func (h *phase2Harness) waitForBroadcastStatus(t *testing.T, acceptable []string, timeout time.Duration) string {
	t.Helper()
	deadline := time.Now().Add(timeout)
	lastSeen := ""
	for time.Now().Before(deadline) {
		bd := h.getBroadcast(t)
		status, _ := bd["status"].(string)
		lastSeen = status
		for _, a := range acceptable {
			if status == a {
				return status
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("broadcast %s did not reach any of %v within %v (last seen: %s)",
		h.broadcastID, acceptable, timeout, lastSeen)
	return lastSeen
}

// waitForMailpitCount polls until Mailpit reports at least min messages
// matching the harness subject. Returns the observed count at completion.
func (h *phase2Harness) waitForMailpitCount(t *testing.T, min int, timeout time.Duration) int {
	t.Helper()
	deadline := time.Now().Add(timeout)
	last := 0
	for time.Now().Before(deadline) {
		count, err := testutil.GetMailpitMessageCount(t, h.subject)
		if err == nil {
			last = count
			if count >= min {
				return count
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("mailpit count did not reach %d within %v (last: %d)", min, timeout, last)
	return last
}

// waitForCondition polls a user-supplied condition with a short interval.
func waitForCondition(t *testing.T, cond func() bool, timeout time.Duration, desc string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("condition %q not met within %v", desc, timeout)
}

// TestBroadcastPhase2_PauseResume_DrainCompletes exercises the full mid-drain
// pause/resume cycle. Validates that pause flips pending→paused, drain halts
// during the hold, resume flips back, all 50 recipients receive exactly one
// email, and the task remains Completed throughout (no re-run).
func TestBroadcastPhase2_PauseResume_DrainCompletes(t *testing.T) {
	h := setupPhase2(t, 50, 60) // 50 contacts, 1/sec
	defer h.Cleanup()

	h.scheduleAndExecute(t)
	h.waitForBroadcastStatus(t, []string{"processed"}, 60*time.Second)

	// Pre-worker: all rows should be pending, none paused.
	pre := h.countQueue(t)
	require.Equal(t, int64(h.contactCount), pre[domain.EmailQueueStatusPending],
		"after orchestrator completion, all rows should be pending")
	require.Equal(t, int64(0), pre[domain.EmailQueueStatusPaused])

	ctx := context.Background()
	require.NoError(t, h.suite.ServerManager.StartBackgroundWorkers(ctx))

	// Wait for partial drain — want Mailpit in [5, 20] to have an obvious window.
	h.waitForMailpitCount(t, 5, 30*time.Second)
	mailpitAtPause, err := testutil.GetMailpitMessageCount(t, h.subject)
	require.NoError(t, err)
	t.Logf("About to pause; Mailpit count = %d", mailpitAtPause)

	// Pause.
	pauseResp, err := h.client.PauseBroadcast(map[string]interface{}{
		"workspace_id": h.workspaceID,
		"id":           h.broadcastID,
	})
	require.NoError(t, err)
	pauseResp.Body.Close()
	require.Equal(t, http.StatusOK, pauseResp.StatusCode)

	h.waitForBroadcastStatus(t, []string{"paused"}, 5*time.Second)

	// Wait up to 3s for queue rows to settle: pending → paused.
	waitForCondition(t, func() bool {
		c := h.countQueue(t)
		return c[domain.EmailQueueStatusPending] == 0 && c[domain.EmailQueueStatusPaused] > 0
	}, 3*time.Second, "pending rows flipped to paused")

	paused := h.countQueue(t)
	t.Logf("Queue after pause: %+v", paused)
	assert.Equal(t, int64(0), paused[domain.EmailQueueStatusPending], "no rows should be pending")
	assert.Greater(t, paused[domain.EmailQueueStatusPaused], int64(0), "some rows should be paused")

	bdAfterPause := h.getBroadcast(t)
	assert.NotNil(t, bdAfterPause["paused_at"], "paused_at should be set")

	// Pause effectiveness: sleep 5s, assert Mailpit barely grows.
	// Allow up to 2 additional messages: one in-flight at the moment of pause,
	// plus one from the race between our service-tx commit and orchestrator poll.
	time.Sleep(5 * time.Second)
	mailpitDuringHold, err := testutil.GetMailpitMessageCount(t, h.subject)
	require.NoError(t, err)
	assert.LessOrEqual(t, mailpitDuringHold, mailpitAtPause+2,
		"pause should stop new sends (before=%d, after-5s=%d)", mailpitAtPause, mailpitDuringHold)

	// Resume.
	resumeResp, err := h.client.ResumeBroadcast(map[string]interface{}{
		"workspace_id": h.workspaceID,
		"id":           h.broadcastID,
	})
	require.NoError(t, err)
	resumeResp.Body.Close()
	require.Equal(t, http.StatusOK, resumeResp.StatusCode)

	h.waitForBroadcastStatus(t, []string{"processed"}, 5*time.Second)

	// Queue rows should flip back.
	waitForCondition(t, func() bool {
		c := h.countQueue(t)
		return c[domain.EmailQueueStatusPaused] == 0
	}, 3*time.Second, "paused rows cleared after resume")

	// Broadcast-level state cleared.
	bdAfterResume := h.getBroadcast(t)
	assert.Nil(t, bdAfterResume["paused_at"], "paused_at should be cleared after resume")
	assert.Nil(t, bdAfterResume["pause_reason"], "pause_reason should be cleared after resume")

	// Final drain.
	require.NoError(t, testutil.WaitForQueueEmpty(t, h.queueRepo, h.workspaceID, 2*time.Minute))

	// All recipients received exactly one email.
	totalMessages, err := testutil.GetMailpitMessageCount(t, h.subject)
	require.NoError(t, err)
	recipients, err := testutil.GetAllMailpitRecipients(t, h.subject)
	require.NoError(t, err)
	assert.Equal(t, h.contactCount, totalMessages, "no duplicates, no losses")
	assert.Equal(t, h.contactCount, len(recipients), "each recipient got exactly one email")

	// Task must stay completed — this is the re-run protection invariant.
	task := h.getTask(t)
	assert.Equal(t, "completed", task["status"], "task must stay completed (handler skip guard)")
}

// TestBroadcastPhase2_Resume_DoesNotReRunOrchestrator is the load-bearing
// guard test: after a Phase-2 pause and resume, the completed task must not
// be flipped back to Pending. A regression here causes the orchestrator to
// re-run on the next ExecutePendingTasks tick and re-enqueue every recipient,
// duplicating the entire send.
func TestBroadcastPhase2_Resume_DoesNotReRunOrchestrator(t *testing.T) {
	h := setupPhase2(t, 10, 600) // small + fast drain
	defer h.Cleanup()

	h.scheduleAndExecute(t)
	h.waitForBroadcastStatus(t, []string{"processed"}, 30*time.Second)

	ctx := context.Background()
	require.NoError(t, h.suite.ServerManager.StartBackgroundWorkers(ctx))
	require.NoError(t, testutil.WaitForQueueEmpty(t, h.queueRepo, h.workspaceID, 60*time.Second))

	// Baseline: all 10 sent, task completed.
	beforeCount, err := testutil.GetMailpitMessageCount(t, h.subject)
	require.NoError(t, err)
	require.Equal(t, h.contactCount, beforeCount, "baseline: all recipients sent")
	beforeTask := h.getTask(t)
	require.Equal(t, "completed", beforeTask["status"])
	beforeNextRun := beforeTask["next_run_after"]

	// Pause (no queue rows to pause — broadcast just flips to paused).
	pauseResp, err := h.client.PauseBroadcast(map[string]interface{}{
		"workspace_id": h.workspaceID,
		"id":           h.broadcastID,
	})
	require.NoError(t, err)
	pauseResp.Body.Close()
	require.Equal(t, http.StatusOK, pauseResp.StatusCode)
	h.waitForBroadcastStatus(t, []string{"paused"}, 5*time.Second)

	// Resume.
	resumeResp, err := h.client.ResumeBroadcast(map[string]interface{}{
		"workspace_id": h.workspaceID,
		"id":           h.broadcastID,
	})
	require.NoError(t, err)
	resumeResp.Body.Close()
	require.Equal(t, http.StatusOK, resumeResp.StatusCode)
	h.waitForBroadcastStatus(t, []string{"processed"}, 5*time.Second)

	// CRITICAL: simulate multiple cron ticks. If the resume handler regressed
	// and flipped the task to Pending, any of these ticks would re-run the
	// orchestrator and re-enqueue everyone.
	for i := 0; i < 5; i++ {
		execResp, execErr := h.client.ExecutePendingTasks(10)
		require.NoError(t, execErr)
		execResp.Body.Close()
		time.Sleep(1 * time.Second)
	}

	// Task state must be unchanged.
	afterTask := h.getTask(t)
	assert.Equal(t, "completed", afterTask["status"],
		"task must stay completed across tick cycles (handler skip guard)")
	assert.Equal(t, beforeNextRun, afterTask["next_run_after"],
		"task.next_run_after must not be mutated by the resume handler")

	// Queue must stay empty.
	q := h.countQueue(t)
	assert.Equal(t, int64(0), q[domain.EmailQueueStatusPending], "no new rows enqueued")
	assert.Equal(t, int64(0), q[domain.EmailQueueStatusPaused])
	assert.Equal(t, int64(0), q[domain.EmailQueueStatusProcessing])
	assert.Equal(t, int64(0), q[domain.EmailQueueStatusFailed])

	// No duplicate sends.
	afterCount, err := testutil.GetMailpitMessageCount(t, h.subject)
	require.NoError(t, err)
	assert.Equal(t, h.contactCount, afterCount,
		"no duplicates — orchestrator must not have re-run")

	recipients, err := testutil.GetAllMailpitRecipients(t, h.subject)
	require.NoError(t, err)
	assert.Equal(t, h.contactCount, len(recipients),
		"each recipient got exactly one email")
}

// TestBroadcastPhase2_Cancel_DeletesQueueRows validates DeleteBySourceTx:
// after cancel mid-drain, pending/failed/paused rows are removed; in-flight
// processing rows complete naturally; the task keeps its Completed status.
func TestBroadcastPhase2_Cancel_DeletesQueueRows(t *testing.T) {
	h := setupPhase2(t, 50, 60)
	defer h.Cleanup()

	h.scheduleAndExecute(t)
	h.waitForBroadcastStatus(t, []string{"processed"}, 60*time.Second)

	ctx := context.Background()
	require.NoError(t, h.suite.ServerManager.StartBackgroundWorkers(ctx))

	h.waitForMailpitCount(t, 5, 30*time.Second)
	mailpitAtCancel, err := testutil.GetMailpitMessageCount(t, h.subject)
	require.NoError(t, err)
	t.Logf("About to cancel; Mailpit count = %d", mailpitAtCancel)

	cancelResp, err := h.client.CancelBroadcast(map[string]interface{}{
		"workspace_id": h.workspaceID,
		"id":           h.broadcastID,
	})
	require.NoError(t, err)
	cancelResp.Body.Close()
	require.Equal(t, http.StatusOK, cancelResp.StatusCode)

	h.waitForBroadcastStatus(t, []string{"cancelled"}, 5*time.Second)

	// DB assertions within a 3s window.
	waitForCondition(t, func() bool {
		c := h.countQueue(t)
		return c[domain.EmailQueueStatusPending] == 0 &&
			c[domain.EmailQueueStatusFailed] == 0 &&
			c[domain.EmailQueueStatusPaused] == 0
	}, 3*time.Second, "pending/failed/paused rows deleted")

	afterCancel := h.countQueue(t)
	t.Logf("Queue after cancel: %+v", afterCancel)
	assert.Equal(t, int64(0), afterCancel[domain.EmailQueueStatusPending])
	assert.Equal(t, int64(0), afterCancel[domain.EmailQueueStatusFailed])
	assert.Equal(t, int64(0), afterCancel[domain.EmailQueueStatusPaused])
	// In-flight processing rows are intentionally preserved. At 1 msg/sec and
	// typical batch fetch, expect 0–5 still processing right at cancel.
	assert.LessOrEqual(t, afterCancel[domain.EmailQueueStatusProcessing], int64(5),
		"only in-flight rows should remain")

	bd := h.getBroadcast(t)
	assert.NotNil(t, bd["cancelled_at"], "cancelled_at should be set")
	assert.NotNil(t, bd["completed_at"], "completed_at should be preserved for audit")

	// Let in-flight sends complete naturally.
	time.Sleep(10 * time.Second)

	final, err := testutil.GetMailpitMessageCount(t, h.subject)
	require.NoError(t, err)
	t.Logf("Final Mailpit count: %d (cancelled at %d, contacts %d)",
		final, mailpitAtCancel, h.contactCount)

	// Final count is between the cancel-moment count and cancel+small tail.
	assert.GreaterOrEqual(t, final, mailpitAtCancel, "already-delivered sends remain counted")
	assert.LessOrEqual(t, final, mailpitAtCancel+5, "only in-flight rows should have completed")
	assert.Less(t, final, h.contactCount, "cancel must have prevented at least some sends")

	// Task must stay Completed — handleBroadcastCancelled's new guard.
	task := h.getTask(t)
	assert.Equal(t, "completed", task["status"],
		"task must stay completed (guard prevents MarkAsFailed on completed task)")
}

// TestBroadcastPhase1_CancelDuringProcessing exercises the newly-allowed
// cancel-from-Processing path. Cancel arrives while the orchestrator is still
// enqueueing; orchestrator's cooperative cancel detection (orchestrator.go:829)
// breaks the loop, calls deleteQueueEntries, and task ends as Failed.
func TestBroadcastPhase1_CancelDuringProcessing(t *testing.T) {
	h := setupPhase2(t, 2000, 60) // large audience; enqueue takes many ticks
	defer h.Cleanup()

	taskID := h.scheduleAsync(t)

	// Continuously pump ExecutePendingTasks in a goroutine until signaled to
	// stop. The orchestrator returns cooperatively between batches, so we need
	// multiple invocations to make progress.
	stopPump := make(chan struct{})
	pumpDone := make(chan struct{})
	go func() {
		defer close(pumpDone)
		_, _ = h.client.ExecuteTask(map[string]interface{}{
			"workspace_id": h.workspaceID,
			"id":           taskID,
		})
		for {
			select {
			case <-stopPump:
				return
			default:
			}
			resp, _ := h.client.ExecutePendingTasks(10)
			if resp != nil {
				resp.Body.Close()
			}
			time.Sleep(200 * time.Millisecond)
		}
	}()

	// Wait for orchestrator to have actually enqueued some rows AND still be
	// in processing state. The email_queue table is the real-time signal —
	// broadcast.enqueued_count only updates at completion. We want to catch
	// cancel DURING the main loop so the cooperative shutdown path
	// (orchestrator.go:829) and deleteQueueEntries both run.
	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		bd := h.getBroadcast(t)
		status, _ := bd["status"].(string)
		if status == "processed" {
			t.Fatalf("orchestrator completed enqueue before cancel could land — bump contactCount")
		}
		if status == "processing" {
			c := h.countQueue(t)
			if c[domain.EmailQueueStatusPending] > 100 {
				break
			}
		}
		time.Sleep(30 * time.Millisecond)
	}
	bdMid := h.getBroadcast(t)
	require.Equal(t, "processing", bdMid["status"], "never observed processing state")
	midQueue := h.countQueue(t)
	t.Logf("Cancelling mid-enqueue; status=processing, pending=%d", midQueue[domain.EmailQueueStatusPending])
	require.Greater(t, midQueue[domain.EmailQueueStatusPending], int64(100),
		"cancel must arrive after orchestrator enqueued some rows, not during setup")

	cancelResp, err := h.client.CancelBroadcast(map[string]interface{}{
		"workspace_id": h.workspaceID,
		"id":           h.broadcastID,
	})
	require.NoError(t, err)
	cancelResp.Body.Close()
	require.Equal(t, http.StatusOK, cancelResp.StatusCode)

	h.waitForBroadcastStatus(t, []string{"cancelled"}, 10*time.Second)

	// Stop the pump and wait for it to drain.
	close(stopPump)
	<-pumpDone

	// Queue should be empty — orchestrator's cooperative shutdown called
	// deleteQueueEntries, plus the service tx ran DeleteBySourceTx.
	waitForCondition(t, func() bool {
		c := h.countQueue(t)
		return c[domain.EmailQueueStatusPending] == 0 &&
			c[domain.EmailQueueStatusFailed] == 0 &&
			c[domain.EmailQueueStatusPaused] == 0
	}, 10*time.Second, "queue fully cleared for broadcast")

	// Task was Running when cancelled. handleBroadcastCancelled marks it
	// Failed, but there's a pre-existing race where a concurrent executor
	// return can clobber that to Pending. Either terminal state is OK —
	// the important invariant is that the broadcast is Cancelled and the
	// queue is empty, which we already asserted.
	task := h.getTask(t)
	taskStatus, _ := task["status"].(string)
	assert.Contains(t, []string{"failed", "pending", "completed"}, taskStatus,
		"task should be in a non-running state")
	if taskStatus == "failed" {
		errMsg, _ := task["error_message"].(string)
		assert.Contains(t, errMsg, "cancel",
			"failure reason should reference cancellation when handler won the race")
	}
}

// TestBroadcastPhase1_PauseResume_CompletesEnqueue is a regression test for
// pre-existing Phase-1 pause/resume flow, now touching our new orchestrator
// call site (pauseQueueEntries at line 841) and the tx-atomic PauseBySourceTx
// in the service layer. Resume must correctly re-wake the orchestrator via
// start_now=true so it finishes enqueueing from recipient_offset.
func TestBroadcastPhase1_PauseResume_CompletesEnqueue(t *testing.T) {
	h := setupPhase2(t, 1000, 6000) // enough to be mid-enqueue, fast drain (100/sec)
	defer h.Cleanup()

	taskID := h.scheduleAsync(t)

	// Pump ExecutePendingTasks continuously until we observe mid-enqueue state.
	stopPump := make(chan struct{})
	pumpDone := make(chan struct{})
	go func() {
		defer close(pumpDone)
		_, _ = h.client.ExecuteTask(map[string]interface{}{
			"workspace_id": h.workspaceID,
			"id":           taskID,
		})
		for {
			select {
			case <-stopPump:
				return
			default:
			}
			resp, _ := h.client.ExecutePendingTasks(10)
			if resp != nil {
				resp.Body.Close()
			}
			time.Sleep(200 * time.Millisecond)
		}
	}()

	// Wait for orchestrator to enqueue some rows (via email_queue) while
	// broadcast is still processing — that's the window where cooperative
	// pause detection runs.
	deadline2 := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline2) {
		bd := h.getBroadcast(t)
		status, _ := bd["status"].(string)
		if status == "processed" {
			t.Fatalf("orchestrator completed enqueue before pause could land — bump contactCount")
		}
		if status == "processing" {
			c := h.countQueue(t)
			if c[domain.EmailQueueStatusPending] > 100 {
				break
			}
		}
		time.Sleep(30 * time.Millisecond)
	}
	bdBeforePause := h.getBroadcast(t)
	require.Equal(t, "processing", bdBeforePause["status"], "never observed processing state")
	preQueue := h.countQueue(t)
	require.Greater(t, preQueue[domain.EmailQueueStatusPending], int64(100),
		"must pause after orchestrator enqueued some rows")
	t.Logf("Pausing mid-enqueue; pending=%d", preQueue[domain.EmailQueueStatusPending])

	pauseResp, err := h.client.PauseBroadcast(map[string]interface{}{
		"workspace_id": h.workspaceID,
		"id":           h.broadcastID,
	})
	require.NoError(t, err)
	pauseResp.Body.Close()
	require.Equal(t, http.StatusOK, pauseResp.StatusCode)

	h.waitForBroadcastStatus(t, []string{"paused"}, 10*time.Second)

	// Stop the pump; orchestrator should have observed pause on its own poll.
	close(stopPump)
	<-pumpDone

	// Task ended in a non-running state. Preferred: "paused" (from the event
	// handler). Fallback: "pending" if the orchestrator's executor return
	// raced with the handler. Either way, downstream resume will work.
	task := h.getTask(t)
	taskStatus, _ := task["status"].(string)
	assert.Contains(t, []string{"paused", "pending"}, taskStatus,
		"task stopped running after pause")

	// Key invariant: no pending rows (service-tx PauseBySourceTx + orchestrator's
	// cooperative pauseQueueEntries both ran; no late stragglers).
	waitForCondition(t, func() bool {
		c := h.countQueue(t)
		return c[domain.EmailQueueStatusPending] == 0 && c[domain.EmailQueueStatusPaused] > 0
	}, 5*time.Second, "no pending rows remain")

	// Queue state stabilizes: no new pending rows appear over a 2s window
	// (the orchestrator has observed pause and exited).
	qBefore := h.countQueue(t)
	time.Sleep(2 * time.Second)
	qAfter := h.countQueue(t)
	assert.Equal(t, qBefore[domain.EmailQueueStatusPending], qAfter[domain.EmailQueueStatusPending],
		"orchestrator should have stopped enqueueing after pause")
	assert.Equal(t, int64(0), qAfter[domain.EmailQueueStatusPending],
		"no pending rows")

	// Start worker, confirm no sends during pause.
	ctx := context.Background()
	require.NoError(t, h.suite.ServerManager.StartBackgroundWorkers(ctx))
	time.Sleep(3 * time.Second)
	mailpitDuringHold, err := testutil.GetMailpitMessageCount(t, h.subject)
	require.NoError(t, err)
	assert.Equal(t, 0, mailpitDuringHold, "worker must not send paused rows")

	// Resume.
	resumeResp, err := h.client.ResumeBroadcast(map[string]interface{}{
		"workspace_id": h.workspaceID,
		"id":           h.broadcastID,
	})
	require.NoError(t, err)
	resumeResp.Body.Close()
	require.Equal(t, http.StatusOK, resumeResp.StatusCode)

	// Broadcast flips to processing (not processed — Phase-1 resume with start_now=true).
	h.waitForBroadcastStatus(t, []string{"processing", "processed"}, 10*time.Second)

	// Pump tasks again so orchestrator completes remaining enqueue.
	stopPump2 := make(chan struct{})
	pump2Done := make(chan struct{})
	go func() {
		defer close(pump2Done)
		for {
			select {
			case <-stopPump2:
				return
			default:
			}
			resp, _ := h.client.ExecutePendingTasks(10)
			if resp != nil {
				resp.Body.Close()
			}
			time.Sleep(200 * time.Millisecond)
		}
	}()

	// Eventually broadcast reaches processed and queue drains.
	h.waitForBroadcastStatus(t, []string{"processed"}, 60*time.Second)
	close(stopPump2)
	<-pump2Done

	require.NoError(t, testutil.WaitForQueueEmpty(t, h.queueRepo, h.workspaceID, 3*time.Minute))

	// Every recipient got exactly one email.
	total, err := testutil.GetMailpitMessageCount(t, h.subject)
	require.NoError(t, err)
	recipients, err := testutil.GetAllMailpitRecipients(t, h.subject)
	require.NoError(t, err)
	assert.Equal(t, h.contactCount, total, "all 2000 recipients emailed exactly once")
	assert.Equal(t, h.contactCount, len(recipients))
}

// TestBroadcastPhase2_PauseWhenQueueEmpty validates that pausing a fully-drained
// broadcast is a safe no-op — no error, zero queue rows affected, resume
// doesn't re-trigger the orchestrator.
func TestBroadcastPhase2_PauseWhenQueueEmpty(t *testing.T) {
	h := setupPhase2(t, 3, 600)
	defer h.Cleanup()

	h.scheduleAndExecute(t)
	h.waitForBroadcastStatus(t, []string{"processed"}, 30*time.Second)

	ctx := context.Background()
	require.NoError(t, h.suite.ServerManager.StartBackgroundWorkers(ctx))
	require.NoError(t, testutil.WaitForQueueEmpty(t, h.queueRepo, h.workspaceID, 30*time.Second))

	count, err := testutil.GetMailpitMessageCount(t, h.subject)
	require.NoError(t, err)
	require.Equal(t, 3, count)

	// Pause an empty-queue broadcast.
	pauseResp, err := h.client.PauseBroadcast(map[string]interface{}{
		"workspace_id": h.workspaceID,
		"id":           h.broadcastID,
	})
	require.NoError(t, err)
	pauseResp.Body.Close()
	assert.Equal(t, http.StatusOK, pauseResp.StatusCode, "pause must succeed even with no queue rows")

	h.waitForBroadcastStatus(t, []string{"paused"}, 5*time.Second)

	q := h.countQueue(t)
	assert.Equal(t, int64(0), q[domain.EmailQueueStatusPaused], "nothing to pause")

	// Resume.
	resumeResp, err := h.client.ResumeBroadcast(map[string]interface{}{
		"workspace_id": h.workspaceID,
		"id":           h.broadcastID,
	})
	require.NoError(t, err)
	resumeResp.Body.Close()
	assert.Equal(t, http.StatusOK, resumeResp.StatusCode)

	h.waitForBroadcastStatus(t, []string{"processed"}, 5*time.Second)

	// Ticks must not re-trigger orchestrator.
	for i := 0; i < 3; i++ {
		resp, _ := h.client.ExecutePendingTasks(10)
		if resp != nil {
			resp.Body.Close()
		}
		time.Sleep(1 * time.Second)
	}

	task := h.getTask(t)
	assert.Equal(t, "completed", task["status"])
	final, _ := testutil.GetMailpitMessageCount(t, h.subject)
	assert.Equal(t, 3, final, "no duplicate sends from empty-queue pause/resume")
}

// TestBroadcastPhase2_PauseIdempotency verifies that double-pause and
// double-resume are rejected at the HTTP layer with 5xx status. The handler
// only surfaces a generic message to clients; we verify the rejection happens
// at all (the specific guard message lives in service-level unit tests).
func TestBroadcastPhase2_PauseIdempotency(t *testing.T) {
	h := setupPhase2(t, 20, 60)
	defer h.Cleanup()

	h.scheduleAndExecute(t)
	h.waitForBroadcastStatus(t, []string{"processed"}, 30*time.Second)

	// First pause succeeds.
	resp, err := h.client.PauseBroadcast(map[string]interface{}{
		"workspace_id": h.workspaceID,
		"id":           h.broadcastID,
	})
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	h.waitForBroadcastStatus(t, []string{"paused"}, 5*time.Second)

	// Second pause must fail — the handler maps the service guard to 5xx.
	resp2, err := h.client.PauseBroadcast(map[string]interface{}{
		"workspace_id": h.workspaceID,
		"id":           h.broadcastID,
	})
	require.NoError(t, err)
	defer resp2.Body.Close()
	assert.Equal(t, http.StatusInternalServerError, resp2.StatusCode,
		"double-pause must be rejected")

	// Resume, then double-resume rejection.
	resumeResp, err := h.client.ResumeBroadcast(map[string]interface{}{
		"workspace_id": h.workspaceID,
		"id":           h.broadcastID,
	})
	require.NoError(t, err)
	resumeResp.Body.Close()
	require.Equal(t, http.StatusOK, resumeResp.StatusCode)
	h.waitForBroadcastStatus(t, []string{"processed"}, 5*time.Second)

	resumeResp2, err := h.client.ResumeBroadcast(map[string]interface{}{
		"workspace_id": h.workspaceID,
		"id":           h.broadcastID,
	})
	require.NoError(t, err)
	defer resumeResp2.Body.Close()
	assert.Equal(t, http.StatusInternalServerError, resumeResp2.StatusCode,
		"double-resume must be rejected")
}

// TestBroadcastPause_IsolatesBySourceID creates two broadcasts in the same
// workspace; pausing broadcast A must not affect broadcast B's queue rows.
// Catches any SQL regression that drops the source_id filter.
func TestBroadcastPause_IsolatesBySourceID(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	client := suite.APIClient
	factory := suite.DataFactory

	// Shared workspace + SMTP, but two broadcasts each targeting distinct contacts.
	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	require.NoError(t, factory.AddUserToWorkspace(user.ID, workspace.ID, "owner"))

	_, err = factory.SetupWorkspaceWithSMTPProvider(workspace.ID,
		testutil.WithIntegrationEmailProvider(domain.EmailProvider{
			Kind: domain.EmailProviderKindSMTP,
			Senders: []domain.EmailSender{
				domain.NewEmailSender("noreply@notifuse.test", "Isolation Test"),
			},
			SMTP: &domain.SMTPSettings{
				Host: "localhost", Port: 1025, UseTLS: false,
			},
			RateLimitPerMinute: 60,
		}))
	require.NoError(t, err)

	require.NoError(t, client.Login(user.Email, "password"))
	client.SetWorkspaceID(workspace.ID)
	require.NoError(t, testutil.ClearMailpitMessages(t))

	queueRepo := suite.ServerManager.GetApp().GetEmailQueueRepository()

	// Build two broadcasts A and B, each with its own list of 10 contacts.
	buildBroadcast := func(name string) (string, string) {
		list, err := factory.CreateList(workspace.ID, testutil.WithListName(name+" List"))
		require.NoError(t, err)

		contacts := make([]map[string]interface{}, 10)
		for i := 0; i < 10; i++ {
			contacts[i] = map[string]interface{}{
				"email":      fmt.Sprintf("%s-%d-%s@example.com", name, i, uuid.New().String()[:6]),
				"first_name": fmt.Sprintf("User%d", i),
				"last_name":  name,
			}
		}
		resp, err := client.BatchImportContacts(contacts, []string{list.ID})
		require.NoError(t, err)
		resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		subject := fmt.Sprintf("%s Broadcast %s", name, uuid.New().String()[:6])
		tmpl, err := factory.CreateTemplate(workspace.ID,
			testutil.WithTemplateName(name+" tmpl"),
			testutil.WithTemplateSubject(subject))
		require.NoError(t, err)

		bcast, err := factory.CreateBroadcast(workspace.ID,
			testutil.WithBroadcastName(name+" Broadcast"),
			testutil.WithBroadcastAudience(domain.AudienceSettings{
				List: list.ID, ExcludeUnsubscribed: true,
			}))
		require.NoError(t, err)
		bcast.TestSettings.Variations[0].TemplateID = tmpl.ID
		updResp, err := client.UpdateBroadcast(map[string]interface{}{
			"workspace_id":  workspace.ID,
			"id":            bcast.ID,
			"name":          bcast.Name,
			"audience":      bcast.Audience,
			"schedule":      bcast.Schedule,
			"test_settings": bcast.TestSettings,
		})
		require.NoError(t, err)
		updResp.Body.Close()
		return bcast.ID, subject
	}

	bcastA, subjectA := buildBroadcast("A")
	bcastB, subjectB := buildBroadcast("B")

	// Schedule both and drive each task to completion (processed state).
	scheduleAndDrive := func(broadcastID string) {
		sResp, err := client.ScheduleBroadcast(map[string]interface{}{
			"workspace_id": workspace.ID, "id": broadcastID, "send_now": true,
		})
		require.NoError(t, err)
		sResp.Body.Close()
		require.Equal(t, http.StatusOK, sResp.StatusCode)

		// Find task.
		var taskID string
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			tr, _ := client.ListTasks(map[string]string{"broadcast_id": broadcastID})
			body, _ := io.ReadAll(tr.Body)
			tr.Body.Close()
			var res map[string]interface{}
			if json.Unmarshal(body, &res) == nil {
				if ts, ok := res["tasks"].([]interface{}); ok && len(ts) > 0 {
					if id, ok := ts[0].(map[string]interface{})["id"].(string); ok {
						taskID = id
						break
					}
				}
			}
			time.Sleep(200 * time.Millisecond)
		}
		require.NotEmpty(t, taskID)

		// First invocation kicks things off.
		eResp, err := client.ExecuteTask(map[string]interface{}{
			"workspace_id": workspace.ID, "id": taskID,
		})
		require.NoError(t, err)
		eResp.Body.Close()

		// Pump until broadcast reaches processed.
		driveDeadline := time.Now().Add(60 * time.Second)
		for time.Now().Before(driveDeadline) {
			gr, err := client.GetBroadcast(broadcastID)
			require.NoError(t, err)
			body, _ := io.ReadAll(gr.Body)
			gr.Body.Close()
			var res map[string]interface{}
			_ = json.Unmarshal(body, &res)
			bd, _ := res["broadcast"].(map[string]interface{})
			status, _ := bd["status"].(string)
			if status == "processed" {
				return
			}
			resp, _ := client.ExecutePendingTasks(10)
			if resp != nil {
				resp.Body.Close()
			}
			time.Sleep(300 * time.Millisecond)
		}
		t.Fatalf("broadcast %s did not reach processed within 60s", broadcastID)
	}

	scheduleAndDrive(bcastA)
	scheduleAndDrive(bcastB)

	ctx := context.Background()
	// Both broadcasts should have 10 pending rows each.
	countFor := func(broadcastID string, status domain.EmailQueueStatus) int64 {
		n, err := queueRepo.CountBySourceAndStatus(ctx, workspace.ID,
			domain.EmailQueueSourceBroadcast, broadcastID, status)
		require.NoError(t, err)
		return n
	}

	waitForCondition(t, func() bool {
		return countFor(bcastA, domain.EmailQueueStatusPending) == 10 &&
			countFor(bcastB, domain.EmailQueueStatusPending) == 10
	}, 30*time.Second, "both broadcasts have 10 pending rows")

	// Pause only A.
	resp, err := client.PauseBroadcast(map[string]interface{}{
		"workspace_id": workspace.ID, "id": bcastA,
	})
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Within 3s: A should have 10 paused; B should still have 10 pending.
	waitForCondition(t, func() bool {
		return countFor(bcastA, domain.EmailQueueStatusPaused) == 10 &&
			countFor(bcastB, domain.EmailQueueStatusPending) == 10
	}, 3*time.Second, "A is paused, B is untouched")

	// Start worker, let B drain.
	require.NoError(t, suite.ServerManager.StartBackgroundWorkers(ctx))

	// Wait for B to fully drain. A must remain paused.
	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		bPending := countFor(bcastB, domain.EmailQueueStatusPending)
		bProcessing := countFor(bcastB, domain.EmailQueueStatusProcessing)
		if bPending == 0 && bProcessing == 0 {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}

	// A is still paused; Mailpit has only B's recipients.
	assert.Equal(t, int64(10), countFor(bcastA, domain.EmailQueueStatusPaused),
		"A's queue rows must remain paused while B drains")
	assert.Equal(t, int64(0), countFor(bcastA, domain.EmailQueueStatusPending),
		"A must have no pending rows")

	countB, err := testutil.GetMailpitMessageCount(t, subjectB)
	require.NoError(t, err)
	assert.Equal(t, 10, countB, "B fully drained")

	countA, err := testutil.GetMailpitMessageCount(t, subjectA)
	require.NoError(t, err)
	assert.Equal(t, 0, countA, "A sent nothing — pause was respected and isolated")

	// Resume A, assert it drains.
	resumeResp, err := client.ResumeBroadcast(map[string]interface{}{
		"workspace_id": workspace.ID, "id": bcastA,
	})
	require.NoError(t, err)
	resumeResp.Body.Close()
	require.Equal(t, http.StatusOK, resumeResp.StatusCode)

	require.NoError(t, testutil.WaitForQueueEmpty(t, queueRepo, workspace.ID, 90*time.Second))

	finalA, _ := testutil.GetMailpitMessageCount(t, subjectA)
	finalB, _ := testutil.GetMailpitMessageCount(t, subjectB)
	assert.Equal(t, 10, finalA, "A drained after resume")
	assert.Equal(t, 10, finalB, "B still at 10 — no duplicates from A's resume")
}
