//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/app"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/service"
	"github.com/Notifuse/notifuse/tests/testutil"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestScheduledBroadcast_LiveScheduler_Issue317 exercises the exact pipeline
// the #317 reporter was watching: the real TaskScheduler ticker dispatching via
// HTTP to /api/tasks.execute, then the real email queue worker sending SMTP to
// Mailpit.
//
// This test is the counterpart of broadcast_scheduled_past_due_test.go — which
// runs the same scenario with APIEndpoint="" (direct execution). The two live
// in separate files so failure localization is immediate: if past_due passes
// and live_scheduler fails, the bug is in the HTTP-dispatch branch at
// internal/service/task_service.go:260-373.
//
// Symptom check on failure: if the broadcast never advances past scheduled,
// the task stays pending with progress=0, and enqueued_count=0, we flag it as
// REPRODUCED #317 (grep-friendly marker) with a full state snapshot.
func TestScheduledBroadcast_LiveScheduler_Issue317(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuiteWithLiveScheduler(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	client := suite.APIClient
	factory := suite.DataFactory

	// Unique subject tag so Mailpit can filter just this test's message, even
	// if other tests have used the shared Mailpit before.
	uniqueTag := uuid.New().String()[:8]
	uniqueSubject := fmt.Sprintf("Live-Scheduler Repro %s", uniqueTag)

	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	require.NoError(t, factory.AddUserToWorkspace(user.ID, workspace.ID, "owner"))

	_, err = factory.SetupWorkspaceWithSMTPProvider(workspace.ID,
		testutil.WithIntegrationEmailProvider(domain.EmailProvider{
			Kind: domain.EmailProviderKindSMTP,
			Senders: []domain.EmailSender{
				domain.NewEmailSender("noreply@notifuse.test", "Live Scheduler Test"),
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

	list, err := factory.CreateList(workspace.ID, testutil.WithListName("Live Scheduler List"))
	require.NoError(t, err)

	contactEmail := fmt.Sprintf("live-sched-%s@example.com", uniqueTag)
	contact, err := factory.CreateContact(workspace.ID, testutil.WithContactEmail(contactEmail))
	require.NoError(t, err)

	_, err = factory.CreateContactList(workspace.ID,
		testutil.WithContactListEmail(contact.Email),
		testutil.WithContactListListID(list.ID),
		testutil.WithContactListStatus(domain.ContactListStatusActive))
	require.NoError(t, err)

	template, err := factory.CreateTemplate(workspace.ID,
		testutil.WithTemplateName("Live Scheduler Template"),
		testutil.WithTemplateSubject(uniqueSubject))
	require.NoError(t, err)

	broadcast, err := factory.CreateBroadcast(workspace.ID,
		testutil.WithBroadcastName("Live Scheduler Past-Due Broadcast"),
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

	// Schedule for 2 minutes ago UTC — past-due, so the task's next_run_after is
	// immediately in the past. Validator at domain/broadcast.go:ScheduleBroadcastRequest.Validate
	// accepts past times.
	pastTime := time.Now().UTC().Add(-2 * time.Minute)
	t.Logf("Scheduling broadcast for past time: %s (live scheduler will pick it up on next tick)",
		pastTime.Format(time.RFC3339))

	scheduleResp, err := client.ScheduleBroadcast(map[string]interface{}{
		"workspace_id":           workspace.ID,
		"id":                     broadcast.ID,
		"send_now":               false,
		"scheduled_date":         pastTime.Format("2006-01-02"),
		"scheduled_time":         pastTime.Format("15:04"),
		"timezone":               "UTC",
		"use_recipient_timezone": false,
	})
	require.NoError(t, err)
	defer scheduleResp.Body.Close()
	require.Equal(t, http.StatusOK, scheduleResp.StatusCode)

	// Give the broadcast-scheduled event handler time to create the task row.
	time.Sleep(2 * time.Second)

	taskID, nextRunAfter := findBroadcastTask(t, client, workspace.ID, broadcast.ID)
	require.NotEmpty(t, taskID, "send_broadcast task should have been created")
	require.False(t, nextRunAfter.IsZero(), "task.next_run_after should be set")
	assert.True(t, nextRunAfter.Before(time.Now().UTC()),
		"task.next_run_after (%s) should be in the past for a past-due schedule", nextRunAfter)

	// **Crucially** we do NOT call /api/cron. The live TaskScheduler started by
	// the harness (harness ticks at 500ms, see NewServerManagerWithLiveScheduler)
	// should pick up the task and dispatch it over HTTP to /api/tasks.execute.
	// Budget: 2 scheduler ticks (~1s) + orchestrator init+process (~500ms) +
	// queue worker poll (≤1s) + SMTP → Mailpit (~100ms) + Mailpit poll (≤2s)
	// ≈ 4.5s. 15s deadline gives ample headroom even under contention.

	deadline := time.Now().Add(15 * time.Second)
	var broadcastStatus, taskStatus string
	var progress float64
	var enqueued int

	for time.Now().Before(deadline) {
		broadcastStatus, enqueued = getBroadcastStatusAndEnqueued(t, client, broadcast.ID)
		taskStatus, progress = getTaskStatusAndProgress(t, client, workspace.ID, taskID)
		t.Logf("poll: broadcast=%s enqueued=%d | task=%s progress=%.1f",
			broadcastStatus, enqueued, taskStatus, progress)

		if broadcastStatus == string(domain.BroadcastStatusProcessed) {
			break
		}
		time.Sleep(1 * time.Second)
	}

	// Symptom-match failure: flag as REPRODUCED #317 for grep.
	if broadcastStatus == string(domain.BroadcastStatusScheduled) &&
		enqueued == 0 &&
		taskStatus == string(domain.TaskStatusPending) &&
		progress == 0 {
		// Capture extended state for debugging. getTaskStateDetail returns the
		// send_broadcast sub-state so we can see whether TotalRecipients was
		// persisted (bug would show TotalRecipients=0 every cycle).
		_, _, sendState := getTaskStateDetail(t, client, workspace.ID, taskID)
		cronStatus := getCronStatus(t, client)
		t.Logf("send_broadcast state at failure: %+v", sendState)
		t.Logf("cron status at failure: %+v", cronStatus)

		t.Fatalf("REPRODUCED #317: past-due scheduled broadcast never advanced via live scheduler — "+
			"broadcast.status=%s enqueued=%d task.status=%s progress=%.1f",
			broadcastStatus, enqueued, taskStatus, progress)
	}

	assert.Equal(t, string(domain.BroadcastStatusProcessed), broadcastStatus,
		"past-due scheduled broadcast should reach processed via the live scheduler")
	assert.Equal(t, string(domain.TaskStatusCompleted), taskStatus,
		"send_broadcast task should complete")
	assert.InDelta(t, 100, progress, 0.01, "task progress should reach 100")

	// End-to-end: verify the email actually landed in Mailpit. This confirms
	// the whole HTTP-dispatch → orchestrator → queue worker → SMTP chain runs.
	require.NoError(t,
		testutil.WaitForMailpitMessages(t, uniqueSubject, 1, 15*time.Second),
		"broadcast email should arrive in Mailpit")
}

// TestScheduledBroadcast_LiveScheduler_MultiWorkspace_Issue317 pressures the
// scheduler with N workspaces running past-due broadcasts + recurring segment
// queue tasks. Closest approximation of the reporter's multi-workspace env
// the harness can produce without running their exact compose stack.
//
// Pressure model (verified):
//   - Each workspace has its own workspace DB; MaxConnectionsPerDB=10.
//   - `process_contact_segment_queue` uses the workspace DB and holds its
//     connection up to ~50s per run (internal/service/contact_segment_queue_processor.go:47).
//   - Each send_broadcast task holds a workspace DB connection while the
//     orchestrator loads recipients/templates/etc.
//   - Shared SYSTEM DB takes load from every concurrent
//     MarkAsRunning/MarkAsPending transaction — N workspaces × 2 concurrent
//     DB ops can pinch the 10-conn pool.
//
// If any broadcast stays stuck after 30s with the #317 symptom tuple, we flag
// it as REPRODUCED #317 and log per-workspace state snapshots so the output
// directly names the affected broadcast(s).
func TestScheduledBroadcast_LiveScheduler_MultiWorkspace_Issue317(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	// Reporter has 3 workspaces — sized to that. Before the fix the bug
	// reproduced at both N=3 and N=8 (we verified N=8 prior to fix);
	// keeping N=3 here documents that the minimum reproducer matches the
	// reporter's environment exactly.
	const N = 3

	suite := testutil.NewIntegrationTestSuiteWithLiveScheduler(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	client := suite.APIClient
	factory := suite.DataFactory
	appIface := suite.ServerManager.GetApp()
	taskRepo := appIface.GetTaskRepository()

	// Each workspace's own owner + auth — scheduled broadcasts run under the
	// workspace's integration, but the API calls to set things up need an auth'd user.
	// Using a single root user who's owner in all N workspaces keeps setup short.
	rootUser, err := factory.CreateUser()
	require.NoError(t, err)
	require.NoError(t, client.Login(rootUser.Email, "password"))

	subjectPrefix := fmt.Sprintf("MW-Repro-%s", uuid.New().String()[:8])

	type wsContext struct {
		workspaceID string
		broadcastID string
		taskID      string
		subject     string
	}
	workspaces := make([]wsContext, 0, N)

	for i := 0; i < N; i++ {
		workspace, err := factory.CreateWorkspace()
		require.NoError(t, err, "create workspace #%d", i)
		require.NoError(t, factory.AddUserToWorkspace(rootUser.ID, workspace.ID, "owner"),
			"add user to workspace #%d", i)

		_, err = factory.SetupWorkspaceWithSMTPProvider(workspace.ID,
			testutil.WithIntegrationEmailProvider(domain.EmailProvider{
				Kind: domain.EmailProviderKindSMTP,
				Senders: []domain.EmailSender{
					domain.NewEmailSender(
						fmt.Sprintf("noreply+%d@notifuse.test", i),
						fmt.Sprintf("MW Test %d", i)),
				},
				SMTP: &domain.SMTPSettings{
					Host:   "localhost",
					Port:   1025,
					UseTLS: false,
				},
				RateLimitPerMinute: 2000,
			}))
		require.NoError(t, err, "SMTP provider #%d", i)

		// Seed the recurring segment queue task — this is what the issue
		// reporter's multi-workspace setup has alongside scheduled broadcasts.
		require.NoError(t,
			service.EnsureContactSegmentQueueProcessingTask(context.Background(), taskRepo, workspace.ID),
			"seed segment queue task #%d", i)

		list, err := factory.CreateList(workspace.ID,
			testutil.WithListName(fmt.Sprintf("MW List %d", i)))
		require.NoError(t, err, "create list #%d", i)

		contactEmail := fmt.Sprintf("mw-%d-%s@example.com", i, uuid.New().String()[:8])
		contact, err := factory.CreateContact(workspace.ID, testutil.WithContactEmail(contactEmail))
		require.NoError(t, err, "create contact #%d", i)

		_, err = factory.CreateContactList(workspace.ID,
			testutil.WithContactListEmail(contact.Email),
			testutil.WithContactListListID(list.ID),
			testutil.WithContactListStatus(domain.ContactListStatusActive))
		require.NoError(t, err, "create contact-list #%d", i)

		// Unique per-workspace subject so Mailpit can count matches correctly
		// even under concurrency.
		// Padded index so "-00" doesn't substring-match "-01" etc. —
		// Mailpit's subject: search is substring, not exact-match.
		subject := fmt.Sprintf("%s-%02d", subjectPrefix, i)
		template, err := factory.CreateTemplate(workspace.ID,
			testutil.WithTemplateName(fmt.Sprintf("MW Template %d", i)),
			testutil.WithTemplateSubject(subject))
		require.NoError(t, err, "create template #%d", i)

		broadcast, err := factory.CreateBroadcast(workspace.ID,
			testutil.WithBroadcastName(fmt.Sprintf("MW Broadcast %d", i)),
			testutil.WithBroadcastAudience(domain.AudienceSettings{
				List:                list.ID,
				ExcludeUnsubscribed: true,
			}))
		require.NoError(t, err, "create broadcast #%d", i)

		broadcast.TestSettings.Variations[0].TemplateID = template.ID
		client.SetWorkspaceID(workspace.ID)
		updateResp, err := client.UpdateBroadcast(map[string]interface{}{
			"workspace_id":  workspace.ID,
			"id":            broadcast.ID,
			"name":          broadcast.Name,
			"audience":      broadcast.Audience,
			"schedule":      broadcast.Schedule,
			"test_settings": broadcast.TestSettings,
		})
		require.NoError(t, err, "update broadcast #%d", i)
		updateResp.Body.Close()

		pastTime := time.Now().UTC().Add(-2 * time.Minute)
		scheduleResp, err := client.ScheduleBroadcast(map[string]interface{}{
			"workspace_id":           workspace.ID,
			"id":                     broadcast.ID,
			"send_now":               false,
			"scheduled_date":         pastTime.Format("2006-01-02"),
			"scheduled_time":         pastTime.Format("15:04"),
			"timezone":               "UTC",
			"use_recipient_timezone": false,
		})
		require.NoError(t, err, "schedule broadcast #%d", i)
		require.Equal(t, http.StatusOK, scheduleResp.StatusCode,
			"schedule broadcast #%d should return 200", i)
		scheduleResp.Body.Close()

		workspaces = append(workspaces, wsContext{
			workspaceID: workspace.ID,
			broadcastID: broadcast.ID,
			subject:     subject,
		})
	}

	// Give the event-handler loop time to create all send_broadcast task rows.
	time.Sleep(3 * time.Second)

	// Snapshot task IDs — we'll reference them for state-inspection on failure.
	for i := range workspaces {
		client.SetWorkspaceID(workspaces[i].workspaceID)
		taskID, _ := findBroadcastTask(t, client, workspaces[i].workspaceID, workspaces[i].broadcastID)
		require.NotEmpty(t, taskID, "send_broadcast task should exist for workspace %d", i)
		workspaces[i].taskID = taskID
	}

	// The live scheduler ticks every 500ms. Orchestrator is 2-tick init, queue
	// worker polls every 1s, SMTP→Mailpit ~100ms. For one workspace: ~4s. For
	// 8 concurrent workspaces competing for the system DB pool, allow 30s.
	//
	// NOTE: the APIClient holds a single workspace_id; poll helpers read it
	// from there, so we must SetWorkspaceID per iteration to query each
	// workspace's broadcasts correctly.
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		allDone := true
		for i := range workspaces {
			client.SetWorkspaceID(workspaces[i].workspaceID)
			status, _ := getBroadcastStatusAndEnqueued(t, client, workspaces[i].broadcastID)
			if status != string(domain.BroadcastStatusProcessed) {
				allDone = false
			}
		}
		if allDone {
			break
		}
		time.Sleep(1 * time.Second)
	}

	// Evaluate + report.
	var stuck []int
	for i := range workspaces {
		client.SetWorkspaceID(workspaces[i].workspaceID)
		bStatus, enq := getBroadcastStatusAndEnqueued(t, client, workspaces[i].broadcastID)
		tStatus, progress, sendState := getTaskStateDetail(t, client, workspaces[i].workspaceID, workspaces[i].taskID)
		t.Logf("workspace[%d] broadcast=%s enqueued=%d | task=%s progress=%.1f",
			i, bStatus, enq, tStatus, progress)

		if bStatus == string(domain.BroadcastStatusScheduled) &&
			enq == 0 &&
			tStatus == string(domain.TaskStatusPending) &&
			progress == 0 {
			t.Logf("workspace[%d] symptoms match #317 — send_state: %+v", i, sendState)
			stuck = append(stuck, i)
			continue
		}
		assert.Equal(t, string(domain.BroadcastStatusProcessed), bStatus,
			"workspace %d broadcast should reach processed", i)
	}

	if len(stuck) > 0 {
		t.Fatalf("REPRODUCED #317 under multi-workspace pressure: %d/%d broadcasts stuck — workspaces %v",
			len(stuck), N, stuck)
	}

	// End-to-end: Mailpit should have received one email per workspace (each
	// with its unique subject). Validating individually so a single
	// delivery-failure surfaces the specific workspace.
	for i := range workspaces {
		require.NoError(t,
			testutil.WaitForMailpitMessages(t, workspaces[i].subject, 1, 15*time.Second),
			"workspace %d email should arrive in Mailpit", i)
	}
}

// TestTask_StaleRunning_Reaped covers the secondary failure mode from #317:
//
//	"Found a task with status=running, timeout_after 20 days in the past,
//	 max_runtime=50s long exceeded. tasks.execute rejects it with 'task
//	 already running', so it blocks its own execution forever."
//
// Before the fix in MarkAsRunningTx, a row stuck in running with expired
// timeout_after would loop: GetNextBatch returns it (it qualifies under
// `status=running AND timeout_after <= now`), the scheduler dispatches it,
// the handler calls MarkAsRunningTx which only accepted pending/paused, gets
// ErrTaskAlreadyRunning, returns 409. Next tick: same cycle. Forever.
//
// After the fix, MarkAsRunningTx also accepts running with expired timeout,
// so the dispatch claims the row, sets a fresh timeout, and proceeds.
func TestTask_StaleRunning_Reaped_Issue317(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuiteWithLiveScheduler(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	client := suite.APIClient
	factory := suite.DataFactory
	appIface := suite.ServerManager.GetApp()
	taskRepo := appIface.GetTaskRepository()

	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	require.NoError(t, factory.AddUserToWorkspace(user.ID, workspace.ID, "owner"))

	_, err = factory.SetupWorkspaceWithSMTPProvider(workspace.ID,
		testutil.WithIntegrationEmailProvider(domain.EmailProvider{
			Kind: domain.EmailProviderKindSMTP,
			Senders: []domain.EmailSender{
				domain.NewEmailSender("noreply@notifuse.test", "Stale Running Test"),
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

	uniqueTag := uuid.New().String()[:8]
	uniqueSubject := fmt.Sprintf("Stale-Running Repro %s", uniqueTag)

	list, err := factory.CreateList(workspace.ID, testutil.WithListName("Stale Running List"))
	require.NoError(t, err)
	contactEmail := fmt.Sprintf("stale-%s@example.com", uniqueTag)
	contact, err := factory.CreateContact(workspace.ID, testutil.WithContactEmail(contactEmail))
	require.NoError(t, err)
	_, err = factory.CreateContactList(workspace.ID,
		testutil.WithContactListEmail(contact.Email),
		testutil.WithContactListListID(list.ID),
		testutil.WithContactListStatus(domain.ContactListStatusActive))
	require.NoError(t, err)
	template, err := factory.CreateTemplate(workspace.ID,
		testutil.WithTemplateName("Stale Running Template"),
		testutil.WithTemplateSubject(uniqueSubject))
	require.NoError(t, err)
	broadcast, err := factory.CreateBroadcast(workspace.ID,
		testutil.WithBroadcastName("Stale Running Broadcast"),
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

	// Seed a send_broadcast task directly in status=running with timeout_after
	// long in the past — simulating a task whose prior execution died after
	// MarkAsRunningTx but before MarkAsPending/Completed/Failed. The reporter
	// found an instance with timeout_after 20 days past.
	bcastIDCopy := broadcast.ID
	expiredTimeout := time.Now().UTC().Add(-20 * 24 * time.Hour)
	staleTask := &domain.Task{
		WorkspaceID:   workspace.ID,
		Type:          "send_broadcast",
		Status:        domain.TaskStatusRunning,
		BroadcastID:   &bcastIDCopy,
		TimeoutAfter:  &expiredTimeout,
		NextRunAfter:  nil,
		MaxRuntime:    50,
		MaxRetries:    3,
		RetryCount:    0,
		RetryInterval: 60,
		State: &domain.TaskState{
			Progress: 0,
			SendBroadcast: &domain.SendBroadcastState{
				BroadcastID: broadcast.ID,
				ChannelType: "email",
				Phase:       "single",
			},
		},
	}
	require.NoError(t, taskRepo.Create(context.Background(), workspace.ID, staleTask))

	// The live scheduler will pick the stale task up on its next 500ms tick
	// (GetNextBatch includes running+expired-timeout). Before the fix the
	// handler's MarkAsRunningTx refused it (409 loop). After the fix it's
	// re-claimed and runs to completion.
	deadline := time.Now().Add(15 * time.Second)
	var taskStatus, broadcastStatus string
	for time.Now().Before(deadline) {
		broadcastStatus, _ = getBroadcastStatusAndEnqueued(t, client, broadcast.ID)
		taskStatus, _ = getTaskStatusAndProgress(t, client, workspace.ID, staleTask.ID)
		t.Logf("poll: broadcast=%s task=%s", broadcastStatus, taskStatus)
		if broadcastStatus == string(domain.BroadcastStatusProcessed) {
			break
		}
		time.Sleep(1 * time.Second)
	}

	assert.Equal(t, string(domain.BroadcastStatusProcessed), broadcastStatus,
		"stale-running task should be reaped and the broadcast should reach processed")
	require.NoError(t,
		testutil.WaitForMailpitMessages(t, uniqueSubject, 1, 15*time.Second),
		"broadcast email should land in Mailpit after reap")
}

// getCronStatus reads /api/cron.status and returns its decoded body. Used to
// snapshot the last cron-run timestamp for the failure report in #317 repros.
func getCronStatus(t *testing.T, client *testutil.APIClient) map[string]interface{} {
	t.Helper()
	resp, err := client.Get("/api/cron.status")
	if err != nil {
		return map[string]interface{}{"error": err.Error()}
	}
	defer resp.Body.Close()
	var body map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return map[string]interface{}{"decode_error": err.Error()}
	}
	return body
}
