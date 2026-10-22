package integration

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/app"
	"github.com/Notifuse/notifuse/tests/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// End-to-end coverage for the interruption/retry defects behind the
// TWOSTONE&Sons report (2026-06-30): a 4,300-recipient broadcast that died at
// ~2,200 because one cancelled context was allowed to kill a run, consume the
// retry budget, skip a batch of recipients and strand the broadcast.
//
// Every test here drives the real HTTP server. The standard harness
// (NewIntegrationTestSuite) keeps TaskScheduler.Enabled=false, which also turns
// off autoExecuteImmediate, so nothing executes tasks behind our back — all
// progress observed after a stimulus was made by the run that stimulus started.
//
// Plan: plans/broadcast-interruption-resilience-plan.md

// callWithClientTimeout issues a request whose client gives up after timeout,
// tearing down the connection while the server may still be working. This is
// what the task dispatcher's 53s HTTP timeout does to a 50s task budget in
// production, and what any reverse proxy read-timeout does to /api/cron.
//
// The client's own fate is deliberately not asserted: before the fix the server
// blocks and the client times out, after the fix /api/cron answers immediately.
// The stimulus is identical either way; only the server's reaction differs.
func callWithClientTimeout(t *testing.T, method, url, body string, timeout time.Duration) {
	t.Helper()

	var payload io.Reader
	if body != "" {
		payload = bytes.NewBufferString(body)
	}
	req, err := http.NewRequest(method, url, payload)
	require.NoError(t, err)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}

	// The dispatch endpoint has no session to authenticate against: it is
	// signed with the installation's SECRET_KEY, the way the scheduler signs it.
	if strings.HasSuffix(url, taskExecutePath) {
		for name, value := range testutil.TaskExecuteHeaders(taskExecutePath, []byte(body), testutil.TestSecretKey) {
			req.Header.Set(name, value)
		}
	}

	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		t.Logf("client gave up after %v as intended: %v", timeout, err)
		return
	}
	defer resp.Body.Close()
	t.Logf("server replied %d within %v", resp.StatusCode, timeout)
}

// taskRow reads the task straight from the system database, bypassing the API
// so retry_count and error_message are observable exactly as stored.
func taskRow(t *testing.T, h *phase2Harness) (status string, retryCount int, errorMessage string) {
	t.Helper()
	require.NotEmpty(t, h.taskID, "scheduleAsync must run first")

	var msg sql.NullString
	err := h.suite.DBManager.GetDB().QueryRow(
		`SELECT status, retry_count, error_message FROM tasks WHERE id = $1`, h.taskID,
	).Scan(&status, &retryCount, &msg)
	require.NoError(t, err)
	return status, retryCount, msg.String
}

// queuedCount totals email_queue rows for the harness broadcast across every
// status. No queue worker runs in this harness, so rows accumulate as enqueued.
func queuedCount(t *testing.T, h *phase2Harness) int64 {
	t.Helper()
	var total int64
	for _, n := range h.countQueue(t) {
		total += n
	}
	return total
}

// waitForQueuedCount polls until at least want rows are enqueued, returning
// whatever it last observed. Unlike waitForCondition it does not abort the
// test, so a failing run still reports every assertion.
func waitForQueuedCount(t *testing.T, h *phase2Harness, want int64, timeout time.Duration) int64 {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var last int64
	for time.Now().Before(deadline) {
		last = queuedCount(t, h)
		if last >= want {
			return last
		}
		time.Sleep(200 * time.Millisecond)
	}
	return last
}

// slowEnqueue makes every email_queue insert take at least d, so a run can be
// interrupted while it is inside the enqueue transaction instead of racing the
// wall clock. This reproduces the production shape of the incident: a batch
// INSERT carrying 50 fully compiled emails, slow enough under WAL pressure to
// straddle the deadline. Returns a function that removes the delay.
func slowEnqueue(t *testing.T, h *phase2Harness, d time.Duration) func() {
	t.Helper()

	wsDB, err := h.suite.DBManager.GetWorkspaceDB(h.workspaceID)
	require.NoError(t, err)

	_, err = wsDB.Exec(fmt.Sprintf(`CREATE OR REPLACE FUNCTION notifuse_test_slow_enqueue() RETURNS trigger AS $$
		BEGIN PERFORM pg_sleep(%f); RETURN NULL; END;
		$$ LANGUAGE plpgsql`, d.Seconds()))
	require.NoError(t, err)
	_, err = wsDB.Exec(`CREATE TRIGGER notifuse_test_slow_enqueue
		BEFORE INSERT ON email_queue
		FOR EACH STATEMENT EXECUTE FUNCTION notifuse_test_slow_enqueue()`)
	require.NoError(t, err)

	return func() {
		_, _ = wsDB.Exec(`DROP TRIGGER IF EXISTS notifuse_test_slow_enqueue ON email_queue`)
	}
}

// runTask drives one execution of the harness task through the public
// dispatch endpoint. MarkAsRunningTx accepts any pending/paused task
// regardless of next_run_after, so this also drives a task sitting in a retry
// backoff.
func runTask(t *testing.T, h *phase2Harness) int {
	t.Helper()
	resp, err := h.client.ExecuteTask(map[string]interface{}{
		"workspace_id": h.workspaceID,
		"id":           h.taskID,
	})
	require.NoError(t, err)
	defer resp.Body.Close()
	return resp.StatusCode
}

// runTaskOK drives one execution and fails unless the handler reports success.
// Without this a 409 (already running) or 500 (execution failed) passes
// silently and turns a later assertion into a puzzle.
func runTaskOK(t *testing.T, h *phase2Harness) {
	t.Helper()
	require.Equal(t, http.StatusOK, runTask(t, h), "task execution should have succeeded")
}

// initTick runs the first execution, which only counts recipients and returns
// early — no email is enqueued yet.
func initTick(t *testing.T, h *phase2Harness) {
	t.Helper()
	runTaskOK(t, h)
	require.Equal(t, int64(0), queuedCount(t, h),
		"the initialisation tick must not enqueue anything")
}

// TestTaskExecute_ClientHangupDoesNotAbortTask pins defect 1 (task execution
// bound to the dispatcher's connection), defect 4 (cancellation counted as a
// failure) and defect 8 (cancellation reported as BROADCAST_NOT_FOUND).
//
// Before the fix the handler runs the task on r.Context(), so the hang-up
// cancels the enqueue transaction mid-batch: nothing more is enqueued, the task
// is marked failed, and its error is "[BROADCAST_NOT_FOUND] broadcast not
// found: failed to get workspace connection: context canceled" — the exact line
// the reporter sent us.
func TestTaskExecute_ClientHangupDoesNotAbortTask(t *testing.T) {
	const contacts = 300

	h := setupPhase2(t, contacts, 6000)
	defer h.Cleanup()

	h.scheduleAsync(t)
	initTick(t, h)

	defer slowEnqueue(t, h, 200*time.Millisecond)()

	// The working tick, abandoned by its dispatcher 300ms in — one batch has
	// landed and the next is inside its INSERT.
	callWithClientTimeout(t, http.MethodPost,
		h.suite.ServerManager.GetURL()+"/api/tasks.execute",
		fmt.Sprintf(`{"workspace_id":%q,"id":%q}`, h.workspaceID, h.taskID),
		300*time.Millisecond)

	enqueued := waitForQueuedCount(t, h, contacts, 45*time.Second)
	status, retries, errMsg := taskRow(t, h)
	t.Logf("after hang-up: enqueued=%d task_status=%s retry_count=%d error=%q",
		enqueued, status, retries, errMsg)

	assert.GreaterOrEqual(t, enqueued, int64(contacts),
		"the run must survive its dispatcher hanging up and enqueue every recipient")
	// The run finishing cleanly is the point: a hang-up must leave no trace on
	// the task. (That an *actually* interrupted run keeps its retry budget is
	// pinned separately, in TestTaskService_ExecuteTask_InterruptionDoesNotConsumeRetryBudget —
	// no run reaches that path once the handler is detached. The misleading
	// BROADCAST_NOT_FOUND classification is pinned in
	// TestFetchBatch_CancelledContextIsInterrupted.)
	assert.Equal(t, "completed", status, "the abandoned run should have finished the broadcast")
	assert.Equal(t, 0, retries, "a completed run must leave a clean retry budget")
	assert.Empty(t, errMsg, "a completed run must leave no error behind")
}

// TestCron_ReturnsImmediately pins the blocking half of defect 2. Since v32.3
// /api/cron executes tasks in-process and only answers once every due task has
// finished its slice, so an external cron with a short timeout gives up (and
// takes the running tasks down with it — see the next test).
func TestCron_ReturnsImmediately(t *testing.T) {
	const contacts = 400

	h := setupPhase2(t, contacts, 6000)
	defer h.Cleanup()

	h.scheduleAsync(t)
	initTick(t, h)

	defer slowEnqueue(t, h, 200*time.Millisecond)()

	start := time.Now()
	resp, err := http.Get(h.suite.ServerManager.GetURL() + "/api/cron?max_tasks=10")
	elapsed := time.Since(start)
	require.NoError(t, err)
	defer resp.Body.Close()
	t.Logf("/api/cron replied %d after %s", resp.StatusCode, elapsed)

	assert.Equal(t, http.StatusAccepted, resp.StatusCode,
		"cron should acknowledge the trigger and run the work in the background")
	assert.Less(t, elapsed, time.Second,
		"cron must not hold the caller open for the whole task run")

	enqueued := waitForQueuedCount(t, h, contacts, 60*time.Second)
	assert.GreaterOrEqual(t, enqueued, int64(contacts),
		"the work must still happen after the endpoint has answered")
}

// TestCron_ClientHangupDoesNotAbortTasks pins the cancellation half of defect 2:
// /api/cron passes r.Context() into in-process execution, so a cron client (or
// a proxy) that gives up kills every task that trigger started.
func TestCron_ClientHangupDoesNotAbortTasks(t *testing.T) {
	const contacts = 300

	h := setupPhase2(t, contacts, 6000)
	defer h.Cleanup()

	h.scheduleAsync(t)
	initTick(t, h)

	defer slowEnqueue(t, h, 200*time.Millisecond)()

	callWithClientTimeout(t, http.MethodGet,
		h.suite.ServerManager.GetURL()+"/api/cron?max_tasks=10", "",
		300*time.Millisecond)

	enqueued := waitForQueuedCount(t, h, contacts, 45*time.Second)
	status, retries, _ := taskRow(t, h)
	t.Logf("after cron hang-up: enqueued=%d task_status=%s retry_count=%d",
		enqueued, status, retries)

	assert.GreaterOrEqual(t, enqueued, int64(contacts),
		"a cron caller hanging up must not abort the tasks it started")
	assert.Equal(t, 0, retries,
		"an interrupted run must not consume the retry budget")
}

// TestBroadcastPauseResume_DoesNotConsumeRetryBudget pins defect 3.
// MarkAsPausedTx increments retry_count, and nothing ever resets it, so three
// pause/resume cycles — a pure user action on a perfectly healthy broadcast —
// silently exhaust max_retries and leave the broadcast one hiccup from death.
func TestBroadcastPauseResume_DoesNotConsumeRetryBudget(t *testing.T) {
	h := setupPhase2(t, 10, 6000)
	defer h.Cleanup()

	h.scheduleAsync(t)

	for cycle := 1; cycle <= 3; cycle++ {
		pauseResp, err := h.client.PauseBroadcast(map[string]interface{}{
			"workspace_id": h.workspaceID,
			"id":           h.broadcastID,
		})
		require.NoError(t, err)
		pauseResp.Body.Close()
		require.Equal(t, http.StatusOK, pauseResp.StatusCode, "pause cycle %d", cycle)
		h.waitForBroadcastStatus(t, []string{"paused"}, 10*time.Second)

		resumeResp, err := h.client.ResumeBroadcast(map[string]interface{}{
			"workspace_id": h.workspaceID,
			"id":           h.broadcastID,
		})
		require.NoError(t, err)
		resumeResp.Body.Close()
		require.Equal(t, http.StatusOK, resumeResp.StatusCode, "resume cycle %d", cycle)
		h.waitForBroadcastStatus(t, []string{"processing", "scheduled"}, 10*time.Second)

		_, retries, _ := taskRow(t, h)
		t.Logf("after pause/resume cycle %d: retry_count=%d", cycle, retries)
	}

	_, retries, _ := taskRow(t, h)
	assert.Equal(t, 0, retries,
		"pausing a broadcast is a user action, not a failed attempt")
}

// TestBroadcastEnqueueFailure_DoesNotSkipRecipients pins defect 5. A batch-level
// enqueue failure writes nothing, yet the sender reports the whole batch as
// processed and the orchestrator advances both the offset and the keyset cursor
// past it — so those recipients are never enqueued, never retried, and the
// broadcast still declares itself processed.
func TestBroadcastEnqueueFailure_DoesNotSkipRecipients(t *testing.T) {
	const contacts = 120 // more than two fetch batches (FetchBatchSize = 50)

	h := setupPhase2(t, contacts, 6000)
	defer h.Cleanup()

	wsDB, err := h.suite.DBManager.GetWorkspaceDB(h.workspaceID)
	require.NoError(t, err)

	// Fault injection: every INSERT into email_queue aborts, the way a
	// transient database error does mid-broadcast.
	_, err = wsDB.Exec(`CREATE OR REPLACE FUNCTION notifuse_test_fail_enqueue() RETURNS trigger AS $$
		BEGIN RAISE EXCEPTION 'injected enqueue failure'; END;
		$$ LANGUAGE plpgsql`)
	require.NoError(t, err)
	_, err = wsDB.Exec(`CREATE TRIGGER notifuse_test_fail_enqueue
		BEFORE INSERT ON email_queue
		FOR EACH STATEMENT EXECUTE FUNCTION notifuse_test_fail_enqueue()`)
	require.NoError(t, err)

	faultActive := true
	clearFault := func() {
		if !faultActive {
			return
		}
		_, dropErr := wsDB.Exec(`DROP TRIGGER IF EXISTS notifuse_test_fail_enqueue ON email_queue`)
		require.NoError(t, dropErr)
		faultActive = false
	}
	defer clearFault()

	h.scheduleAsync(t)
	initTick(t, h)

	// A single working tick is enough: before the fix one tick walked the whole
	// audience, skipping every batch, and declared the broadcast processed.
	// Deliberately only one, so the test does not depend on the task's
	// max_retries — the tick after the budget runs out would be rejected as
	// already-failed and the failure would surface somewhere confusing.
	assert.Equal(t, http.StatusInternalServerError, runTask(t, h),
		"a batch that could not be enqueued must surface as a failed run, not a silent skip")

	bd := h.getBroadcast(t)
	status, _ := bd["status"].(string)
	enqueuedDuringFault := queuedCount(t, h)
	t.Logf("with enqueue failing: broadcast_status=%s enqueued=%d", status, enqueuedDuringFault)

	require.Equal(t, int64(0), enqueuedDuringFault, "fault injection should block every insert")
	assert.Equal(t, "processing", status,
		"a broadcast that enqueued nobody must stay in flight — never report itself processed")

	// Clear the fault: the recipients skipped while it was active must still be
	// reachable, i.e. the cursor must not have moved past them.
	clearFault()

	for i := 0; i < 6; i++ {
		runTaskOK(t, h)
		if queuedCount(t, h) >= contacts {
			break
		}
	}

	enqueued := waitForQueuedCount(t, h, contacts, 30*time.Second)
	t.Logf("after clearing the fault: enqueued=%d of %d", enqueued, contacts)
	assert.EqualValues(t, contacts, enqueued,
		"no recipient may be skipped by a transient enqueue failure")

	finalStatus := h.waitForBroadcastStatus(t, []string{"processed"}, 15*time.Second)
	assert.Equal(t, "processed", finalStatus,
		"once every recipient is enqueued the broadcast completes normally")
}

// TestBroadcastRetryBudget_IsRepaidByProgress covers the half of the retry
// defect that the pause/resume test does not reach: max_retries must be a
// consecutive-failure budget, not a lifetime one. A broadcast legitimately
// spans dozens of slices, and before the fix every failure along the way was
// remembered forever — three transient hiccups hours apart killed a send that
// was otherwise making steady progress.
func TestBroadcastRetryBudget_IsRepaidByProgress(t *testing.T) {
	// Enough recipients that a slice cannot finish the audience below.
	const contacts = 300

	h := setupPhase2(t, contacts, 6000)
	defer h.Cleanup()

	wsDB, err := h.suite.DBManager.GetWorkspaceDB(h.workspaceID)
	require.NoError(t, err)

	h.scheduleAsync(t)
	initTick(t, h)

	// One transient failure.
	_, err = wsDB.Exec(`CREATE OR REPLACE FUNCTION notifuse_test_fail_enqueue() RETURNS trigger AS $$
		BEGIN RAISE EXCEPTION 'injected enqueue failure'; END;
		$$ LANGUAGE plpgsql`)
	require.NoError(t, err)
	_, err = wsDB.Exec(`CREATE TRIGGER notifuse_test_fail_enqueue
		BEFORE INSERT ON email_queue
		FOR EACH STATEMENT EXECUTE FUNCTION notifuse_test_fail_enqueue()`)
	require.NoError(t, err)

	assert.Equal(t, http.StatusInternalServerError, runTask(t, h))

	_, retriesAfterFailure, _ := taskRow(t, h)
	require.Equal(t, 1, retriesAfterFailure, "the failed attempt should be counted")

	_, err = wsDB.Exec(`DROP TRIGGER IF EXISTS notifuse_test_fail_enqueue ON email_queue`)
	require.NoError(t, err)

	// Now let the task complete a *partial* slice: slow the enqueue down and
	// shrink the run budget so it stops on time with work still to do, which is
	// the "continue in the next run" path that repays the budget.
	defer slowEnqueue(t, h, 300*time.Millisecond)()
	_, err = h.suite.DBManager.GetDB().Exec(
		`UPDATE tasks SET max_runtime = 1 WHERE id = $1`, h.taskID)
	require.NoError(t, err)

	runTaskOK(t, h)

	status, retries, _ := taskRow(t, h)
	enqueued := queuedCount(t, h)
	t.Logf("after a partial but successful slice: task_status=%s retry_count=%d enqueued=%d",
		status, retries, enqueued)

	require.Greater(t, enqueued, int64(0), "the slice must have made real progress")
	require.Less(t, enqueued, int64(contacts), "the slice must have stopped before finishing")
	assert.Equal(t, "pending", status, "the task should be queued to continue")
	assert.Equal(t, 0, retries,
		"a slice that made progress must repay the retry budget it spent earlier")
}

// TestBroadcastResume_ClearsRetryBudget pins defect 7. handleBroadcastResumed
// writes the task back through repo.Update, which persists retry_count
// verbatim — so resuming a broadcast that already spent its budget hands it
// straight back, one failure from being marked failed for good.
func TestBroadcastResume_ClearsRetryBudget(t *testing.T) {
	const contacts = 10

	h := setupPhase2(t, contacts, 6000)
	defer h.Cleanup()

	h.scheduleAsync(t)

	// Stand in for a broadcast that has already burned its budget on transient
	// interruptions.
	_, err := h.suite.DBManager.GetDB().Exec(
		`UPDATE tasks SET retry_count = 3 WHERE id = $1`, h.taskID)
	require.NoError(t, err)

	pauseResp, err := h.client.PauseBroadcast(map[string]interface{}{
		"workspace_id": h.workspaceID,
		"id":           h.broadcastID,
	})
	require.NoError(t, err)
	pauseResp.Body.Close()
	require.Equal(t, http.StatusOK, pauseResp.StatusCode)
	h.waitForBroadcastStatus(t, []string{"paused"}, 10*time.Second)

	resumeResp, err := h.client.ResumeBroadcast(map[string]interface{}{
		"workspace_id": h.workspaceID,
		"id":           h.broadcastID,
	})
	require.NoError(t, err)
	resumeResp.Body.Close()
	require.Equal(t, http.StatusOK, resumeResp.StatusCode)
	h.waitForBroadcastStatus(t, []string{"processing", "scheduled"}, 10*time.Second)

	_, retries, _ := taskRow(t, h)
	t.Logf("after resume: retry_count=%d", retries)
	assert.Equal(t, 0, retries,
		"resuming a broadcast must hand its task a fresh retry budget")

	// And it must still be able to finish.
	for i := 0; i < 5; i++ {
		runTask(t, h)
		if queuedCount(t, h) >= contacts {
			break
		}
	}
	assert.EqualValues(t, contacts, queuedCount(t, h),
		"a resumed broadcast must complete its enqueue")
}

// ---------------------------------------------------------------------------
// Interruption: a run cut short by its context going away.
//
// Nothing reachable over HTTP can cancel a task any more — that was the point
// of detaching the handlers — so these tests drive the scheduler on a context
// they own and cancel it mid-slice. In direct-execution mode that context is
// the ancestor of the one the processor runs on.
//
// Two rules these tests obey, both learned the hard way:
//   - Poll for terminal state. The orchestrator's finalising defer runs inside
//     the processor goroutine, which is still unwinding when ExecuteTask returns.
//   - Never assert on which select branch won. A cancellation makes both
//     ctx.Done() and the processor's error ready, and Go picks at random; the
//     two paths differ only in requeue delay.
// ---------------------------------------------------------------------------

// setupInterruptible builds a harness whose scheduler runs on ctx, so the test
// can interrupt a slice by cancelling it.
//
// The rate limit is deliberately 1/min: this wiring starts the email queue
// worker, and a fast drain would delete the very rows these assertions count.
func setupInterruptible(t *testing.T, ctx context.Context, contactCount int) *phase2Harness {
	t.Helper()
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()

	suite := testutil.NewIntegrationTestSuiteWithDirectSchedulerCtx(t, ctx,
		func(cfg *config.Config) testutil.AppInterface { return app.NewApp(cfg) })

	return setupPhase2WithSuite(t, suite, contactCount, 1)
}

// waitForCommittedBatch blocks until the running slice has committed at least
// one batch. That is the window where an interruption is meaningful: the task
// is mid-send, with work already durable and more still to do. Waiting on the
// task's status alone is not enough — it flips to running before its first
// INSERT lands.
func waitForCommittedBatch(t *testing.T, h *phase2Harness, timeout time.Duration) int64 {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if n := queuedCount(t, h); n > 0 {
			return n
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("no batch was committed within %v", timeout)
	return 0
}

// waitForQueuedGrowth blocks until the queue has grown past baseline, i.e. the
// *current* slice has committed a batch. Needed wherever rows already exist
// from an earlier slice, which a plain "any rows?" check would mistake for
// progress.
func waitForQueuedGrowth(t *testing.T, h *phase2Harness, baseline int64, timeout time.Duration) int64 {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if n := queuedCount(t, h); n > baseline {
			return n
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("the queue did not grow past %d within %v", baseline, timeout)
	return 0
}

// waitForTaskStatus polls the task row, returning the last status seen.
func waitForTaskStatus(t *testing.T, h *phase2Harness, want string, timeout time.Duration) string {
	t.Helper()
	deadline := time.Now().Add(timeout)
	last := ""
	for time.Now().Before(deadline) {
		last, _, _ = taskRow(t, h)
		if last == want {
			return last
		}
		time.Sleep(100 * time.Millisecond)
	}
	return last
}

// TestBroadcastInterrupted_ReleasesWithoutSpendingBudget pins the defect that
// turned one cancelled context into a dead broadcast: an execution cut short
// from outside was recorded as a failed attempt, so three restarts — or three
// proxy timeouts — exhausted max_retries on a send that was making progress.
func TestBroadcastInterrupted_ReleasesWithoutSpendingBudget(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	h := setupInterruptible(t, ctx, 300)
	defer h.Cleanup()

	defer slowEnqueue(t, h, 300*time.Millisecond)()

	h.scheduleAsync(t)
	committed := waitForCommittedBatch(t, h, 60*time.Second)

	cancel() // the server is going down under a running slice

	status := waitForTaskStatus(t, h, "pending", 30*time.Second)
	_, retries, errMsg := taskRow(t, h)
	t.Logf("after interruption: task_status=%s retry_count=%d committed=%d error=%q",
		status, retries, committed, errMsg)

	assert.Equal(t, "pending", status, "an interrupted task must be re-queued, not left running")
	assert.Equal(t, 0, retries, "an interruption is not a failed attempt")
	assert.Contains(t, errMsg, "interrupted",
		"the task should record why it was re-queued")
}

// TestBroadcastInterrupted_ResumesAndCompletes covers the other half: the work
// already committed survives, and the batch that was cut is re-processed rather
// than skipped or duplicated.
func TestBroadcastInterrupted_ResumesAndCompletes(t *testing.T) {
	const contacts = 200

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	h := setupInterruptible(t, ctx, contacts)
	defer h.Cleanup()

	dropSlowEnqueue := slowEnqueue(t, h, 300*time.Millisecond)

	h.scheduleAsync(t)
	waitForCommittedBatch(t, h, 60*time.Second)
	cancel()
	waitForTaskStatus(t, h, "pending", 30*time.Second)

	// The scheduler is gone with the context, but the server still serves, so
	// the task can be driven to completion by hand — through /api/cron, not
	// /api/tasks.execute. This harness runs with the internal scheduler ON, and
	// that is exactly the shape in which the dispatch endpoint answers 404
	// (pinned by TestTaskExecute_SchedulerEnabledAnswers404); /api/cron is always
	// live and detaches from the request, so it runs the pending task in-process.
	dropSlowEnqueue()

	// The interrupted run left the task in its retry backoff. /api/tasks.execute
	// used to ignore next_run_after, so this test could drive the task straight
	// away; cron only picks up what is due, so bring the task forward instead of
	// waiting out the backoff.
	_, err := h.suite.DBManager.GetDB().Exec(
		`UPDATE tasks SET next_run_after = NOW() - INTERVAL '1 minute' WHERE id = $1`, h.taskID)
	require.NoError(t, err)

	cronDeadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(cronDeadline) {
		resp, err := http.Get(h.suite.ServerManager.GetURL() + "/api/cron?max_tasks=10")
		require.NoError(t, err)
		_ = resp.Body.Close()
		require.Equal(t, http.StatusAccepted, resp.StatusCode, "cron should accept the trigger")

		bd := h.getBroadcast(t)
		if status, _ := bd["status"].(string); status == "processed" {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}

	finalStatus := h.waitForBroadcastStatus(t, []string{"processed"}, 30*time.Second)
	assert.Equal(t, "processed", finalStatus, "an interrupted broadcast must be able to finish")

	bd := h.getBroadcast(t)
	enqueued, _ := bd["enqueued_count"].(float64)
	assert.EqualValues(t, contacts, enqueued,
		"every recipient exactly once — no gap from the cut batch, no duplicate from re-processing it")

	// And no contact was written twice into the queue.
	wsDB, err := h.suite.DBManager.GetWorkspaceDB(h.workspaceID)
	require.NoError(t, err)
	var rows, distinct int
	require.NoError(t, wsDB.QueryRow(
		`SELECT count(*), count(DISTINCT contact_email) FROM email_queue WHERE source_id = $1`,
		h.broadcastID).Scan(&rows, &distinct))
	assert.Equal(t, rows, distinct, "a re-processed batch must not enqueue anyone twice")
}

// TestBroadcastInterruptedOnLastRetry_PausesWithReason is the end-to-end
// counterpart of orchestrator_interrupt_test.go: an interruption that exhausts
// the budget must leave the broadcast resumable, with the reason on the row,
// rather than failed — and the terminal write must land despite the context
// that caused it being dead.
func TestBroadcastInterruptedOnLastRetry_PausesWithReason(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	h := setupInterruptible(t, ctx, 200)
	defer h.Cleanup()

	dropSlowEnqueue := slowEnqueue(t, h, 300*time.Millisecond)

	h.scheduleAsync(t)

	// Make every attempt the last one. Pausing first keeps the scheduler off the
	// task while we edit it — max_retries is read into memory when a run starts,
	// so it has to be set before the run we interrupt, and unlike retry_count it
	// survives MarkAsPending.
	pauseResp, err := h.client.PauseBroadcast(map[string]interface{}{
		"workspace_id": h.workspaceID, "id": h.broadcastID})
	require.NoError(t, err)
	pauseResp.Body.Close()
	h.waitForBroadcastStatus(t, []string{"paused"}, 15*time.Second)

	_, err = h.suite.DBManager.GetDB().Exec(
		`UPDATE tasks SET max_retries = 1 WHERE id = $1`, h.taskID)
	require.NoError(t, err)

	// Restart it through the database rather than the resume endpoint: resuming
	// fires an immediate execution on context.Background(), which is by
	// construction immune to the cancellation this test depends on. Going
	// through the row hands the task back to the scheduler, whose context we own.
	wsDB, err := h.suite.DBManager.GetWorkspaceDB(h.workspaceID)
	require.NoError(t, err)
	_, err = wsDB.Exec(
		`UPDATE broadcasts SET status = 'processing', paused_at = NULL, pause_reason = NULL WHERE id = $1`,
		h.broadcastID)
	require.NoError(t, err)

	baseline := queuedCount(t, h)
	_, err = h.suite.DBManager.GetDB().Exec(
		`UPDATE tasks SET status = 'pending', next_run_after = now(), timeout_after = NULL WHERE id = $1`,
		h.taskID)
	require.NoError(t, err)

	waitForQueuedGrowth(t, h, baseline, 60*time.Second)
	cancel()

	// The defer that finalises the broadcast runs on a detached context inside a
	// goroutine that is still unwinding, so poll.
	status := h.waitForBroadcastStatus(t, []string{"paused", "failed"}, 30*time.Second)
	bd := h.getBroadcast(t)
	reason, _ := bd["pause_reason"].(string)
	t.Logf("after interruption on the last attempt: broadcast_status=%s pause_reason=%q", status, reason)

	assert.Equal(t, "paused", status,
		"an interruption must leave the broadcast resumable, never failed")
	assert.NotEmpty(t, reason, "the pause must explain itself")

	// And it can still be finished.
	dropSlowEnqueue()
	resumeResp2, err := h.client.ResumeBroadcast(map[string]interface{}{
		"workspace_id": h.workspaceID, "id": h.broadcastID})
	require.NoError(t, err)
	resumeResp2.Body.Close()

	// The resume triggers a run of its own on a detached context, so there is
	// nothing to dispatch here — and nothing this harness could dispatch with:
	// /api/tasks.execute answers 404 on an instance whose internal scheduler is
	// on. Poll the broadcast.
	for i := 0; i < 30; i++ {
		if bd := h.getBroadcast(t); bd["status"] == "processed" {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	assert.Equal(t, "processed",
		h.waitForBroadcastStatus(t, []string{"processed"}, 30*time.Second),
		"a broadcast paused by an interruption must resume from its cursor and finish")
}
