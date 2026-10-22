package integration

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/tests/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// End-to-end coverage of which failed run finalizes its broadcast. The task
// service retries a failed run while the task has retries left; the broadcast
// orchestrator used to mark the broadcast failed one attempt earlier, and the
// retry then sent the rest of the list and flipped it back to processed.
//
// Every run here is driven through the dispatch endpoint (no scheduler runs in
// this harness), with enqueue failures injected by a trigger on email_queue
// (failEnqueue, broadcast_ab_failure_test.go).
//
// Plan: plans/fix-broadcast-last-attempt-plan.md

// heldEnqueueLock is the advisory lock key the held-enqueue trigger waits on.
const heldEnqueueLock = 74260001

// setRetryCount stands in for the attempts that already failed in a row.
func setRetryCount(t *testing.T, h *phase2Harness, retryCount int) {
	t.Helper()
	_, err := h.suite.DBManager.GetDB().Exec(
		`UPDATE tasks SET retry_count = $1 WHERE id = $2`, retryCount, h.taskID)
	require.NoError(t, err)
}

// finishBroadcast drives the task until the broadcast is processed and checks
// every recipient was enqueued exactly once.
func finishBroadcast(t *testing.T, h *phase2Harness, contacts int) {
	t.Helper()

	for i := 0; i < 6; i++ {
		runTaskOK(t, h)
		if bd := h.getBroadcast(t); bd["status"] == "processed" {
			break
		}
	}
	assert.Equal(t, "processed", h.waitForBroadcastStatus(t, []string{"processed"}, statusFlipTimeout))

	wsDB, err := h.suite.DBManager.GetWorkspaceDB(h.workspaceID)
	require.NoError(t, err)
	var rows, distinct int
	require.NoError(t, wsDB.QueryRow(
		`SELECT count(*), count(DISTINCT contact_email) FROM email_queue WHERE source_id = $1`,
		h.broadcastID).Scan(&rows, &distinct))
	assert.Equal(t, contacts, rows, "every recipient must be enqueued")
	assert.Equal(t, rows, distinct, "no recipient may be enqueued twice")
}

// TestBroadcastFailureWithRetriesLeft_StaysInFlight pins the defect itself: the
// attempt that starts with a retry left (count 2 of 3) fails, the task
// service schedules the retry, and the broadcast must stay in flight rather
// than read failed until that retry flips it back.
func TestBroadcastFailureWithRetriesLeft_StaysInFlight(t *testing.T) {
	const contacts = 60

	h := setupPhase2(t, contacts, 6000)
	defer h.Cleanup()

	h.scheduleAsync(t)
	initTick(t, h)

	clearFault := failEnqueue(t, h)
	setRetryCount(t, h, 2)

	assert.Equal(t, http.StatusInternalServerError, runTask(t, h),
		"the injected enqueue failure must fail the run")

	status, retries, _ := taskRow(t, h)
	bd := h.getBroadcast(t)
	t.Logf("after a failure with a retry left: task_status=%s retry_count=%d broadcast_status=%v",
		status, retries, bd["status"])

	assert.Equal(t, "pending", status, "the task service retries this failure")
	assert.Equal(t, 3, retries)
	assert.Equal(t, "processing", bd["status"],
		"a broadcast whose task will retry must not be marked failed")

	clearFault()
	finishBroadcast(t, h, contacts)
}

// TestBroadcastFailureOnLastAttempt_FailsWithItsTask pins the other side: the
// attempt that starts with no retry left fails, and the broadcast and its task
// end failed together.
func TestBroadcastFailureOnLastAttempt_FailsWithItsTask(t *testing.T) {
	const contacts = 60

	h := setupPhase2(t, contacts, 6000)
	defer h.Cleanup()

	h.scheduleAsync(t)
	initTick(t, h)

	defer failEnqueue(t, h)()
	setRetryCount(t, h, 3)

	assert.Equal(t, http.StatusInternalServerError, runTask(t, h),
		"the injected enqueue failure must fail the run")

	status, retries, _ := taskRow(t, h)
	bd := h.getBroadcast(t)
	t.Logf("after a failure on the last attempt: task_status=%s retry_count=%d broadcast_status=%v",
		status, retries, bd["status"])

	assert.Equal(t, "failed", status, "the task service gives up after its last attempt")
	assert.Equal(t, 4, retries)
	assert.Equal(t, "failed", bd["status"], "the broadcast fails with its task")
}

// TestBroadcastABStartFailureOnLastAttempt_FailsWithItsTask covers an error
// the last attempt returned without handing it to its finalizer: the A/B run
// that cannot move its broadcast to testing. The task fails for good, so the
// broadcast must fail with it rather than stay sending with nothing left to
// run it.
func TestBroadcastABStartFailureOnLastAttempt_FailsWithItsTask(t *testing.T) {
	const contacts = 60

	h := setupPhase2(t, contacts, 6000)
	defer h.Cleanup()

	variationB, err := h.factory.CreateTemplate(h.workspaceID,
		testutil.WithTemplateName("Variation B"),
		testutil.WithTemplateSubject(h.subject))
	require.NoError(t, err)
	existing := h.getBroadcast(t)
	updateResp, err := h.client.UpdateBroadcast(map[string]interface{}{
		"workspace_id": h.workspaceID,
		"id":           h.broadcastID,
		"name":         existing["name"],
		"audience":     existing["audience"],
		"schedule":     existing["schedule"],
		"test_settings": map[string]interface{}{
			"enabled":                 true,
			"sample_percentage":       50,
			"auto_send_winner":        false,
			"auto_send_winner_metric": "open_rate",
			"test_duration_hours":     0,
			"variations": []map[string]interface{}{
				{"variation_name": "Version A", "template_id": h.templateID},
				{"variation_name": "Version B", "template_id": variationB.ID},
			},
		},
	})
	require.NoError(t, err)
	updateResp.Body.Close()
	require.Equal(t, http.StatusOK, updateResp.StatusCode)

	h.scheduleAsync(t)
	initTick(t, h)

	// The move to testing fails; any other status write goes through.
	wsDB, err := h.suite.DBManager.GetWorkspaceDB(h.workspaceID)
	require.NoError(t, err)
	_, err = wsDB.Exec(`CREATE OR REPLACE FUNCTION notifuse_test_refuse_testing() RETURNS trigger AS $$
		BEGIN RAISE EXCEPTION 'injected status write failure'; END;
		$$ LANGUAGE plpgsql`)
	require.NoError(t, err)
	_, err = wsDB.Exec(`CREATE TRIGGER notifuse_test_refuse_testing
		BEFORE UPDATE ON broadcasts
		FOR EACH ROW WHEN (NEW.status = 'testing') EXECUTE FUNCTION notifuse_test_refuse_testing()`)
	require.NoError(t, err)
	defer func() {
		_, _ = wsDB.Exec(`DROP TRIGGER IF EXISTS notifuse_test_refuse_testing ON broadcasts`)
	}()
	setRetryCount(t, h, 3)

	assert.Equal(t, http.StatusInternalServerError, runTask(t, h),
		"the failed move to testing must fail the run")

	status, retries, errMsg := taskRow(t, h)
	bd := h.getBroadcast(t)
	t.Logf("after the A/B start failed on the last attempt: task_status=%s retry_count=%d broadcast_status=%v error=%q",
		status, retries, bd["status"], errMsg)

	assert.Contains(t, errMsg, "failed to update broadcast status to testing")
	assert.Equal(t, "failed", status, "the task service gives up after its last attempt")
	assert.Equal(t, 4, retries)
	assert.Equal(t, "failed", bd["status"], "the broadcast fails with its task")
}

// heldRun is a last attempt held inside its enqueue: a trigger on email_queue
// waits on an advisory lock the test holds, then raises once it is released.
// The lock is taken and released on one dedicated connection, since a session
// advisory lock belongs to the connection that took it and a pooled Exec may
// land on another.
type heldRun struct {
	t      *testing.T
	h      *phase2Harness
	conn   *sql.Conn
	locked bool
	done   chan heldRunResult
}

type heldRunResult struct {
	status int
	err    error
}

// holdLastAttempt starts the task's last attempt (count 3 of 3) in the
// background and returns once it waits on the lock inside its enqueue.
func holdLastAttempt(t *testing.T, h *phase2Harness) *heldRun {
	t.Helper()
	ctx := context.Background()

	// A failed check below ends the test before the caller defers cleanup,
	// so the helper undoes its own setup: a run left waiting on the lock
	// would hold up the teardown.
	run := &heldRun{t: t, h: h, done: make(chan heldRunResult, 1)}
	held := false
	defer func() {
		if !held {
			run.cleanup()
		}
	}()

	wsDB, err := h.suite.DBManager.GetWorkspaceDB(h.workspaceID)
	require.NoError(t, err)

	_, err = wsDB.Exec(fmt.Sprintf(`CREATE OR REPLACE FUNCTION notifuse_test_held_enqueue() RETURNS trigger AS $$
		BEGIN PERFORM pg_advisory_xact_lock(%d); RAISE EXCEPTION 'injected enqueue failure'; END;
		$$ LANGUAGE plpgsql`, heldEnqueueLock))
	require.NoError(t, err)
	_, err = wsDB.Exec(`CREATE TRIGGER notifuse_test_held_enqueue
		BEFORE INSERT ON email_queue
		FOR EACH STATEMENT EXECUTE FUNCTION notifuse_test_held_enqueue()`)
	require.NoError(t, err)

	run.conn, err = wsDB.Conn(ctx)
	require.NoError(t, err)
	_, err = run.conn.ExecContext(ctx, `SELECT pg_advisory_lock($1)`, heldEnqueueLock)
	require.NoError(t, err)
	run.locked = true

	setRetryCount(t, h, 3)

	// Its answer comes back on a channel so a failure is reported by the
	// test goroutine.
	go func() {
		resp, runErr := h.client.ExecuteTask(map[string]interface{}{
			"workspace_id": h.workspaceID,
			"id":           h.taskID,
		})
		if runErr != nil {
			run.done <- heldRunResult{err: runErr}
			return
		}
		resp.Body.Close()
		run.done <- heldRunResult{status: resp.StatusCode}
	}()

	waitForUngrantedLock(t, h, fmt.Sprintf("locktype = 'advisory' AND objid = %d", heldEnqueueLock),
		"the last attempt never reached its enqueue")
	held = true
	return run
}

// release lets the held run's enqueue fail.
func (r *heldRun) release() {
	r.t.Helper()
	if !r.locked {
		return
	}
	_, err := r.conn.ExecContext(context.Background(), `SELECT pg_advisory_unlock($1)`, heldEnqueueLock)
	require.NoError(r.t, err)
	r.locked = false
}

// result waits for the held run's answer.
func (r *heldRun) result() heldRunResult {
	r.t.Helper()
	select {
	case res := <-r.done:
		return res
	case <-time.After(statusFlipTimeout):
		r.t.Fatal("the last attempt did not end after the lock was released")
		return heldRunResult{}
	}
}

// cleanup releases the lock before dropping the trigger: the run's INSERT
// waits on the lock inside the trigger, so a DROP TRIGGER issued first would
// queue behind it.
func (r *heldRun) cleanup() {
	if r.locked {
		_, _ = r.conn.ExecContext(context.Background(), `SELECT pg_advisory_unlock($1)`, heldEnqueueLock)
		r.locked = false
	}
	if r.conn != nil {
		_ = r.conn.Close()
	}
	r.dropTrigger()
}

// dropTrigger lets the broadcast's next runs enqueue.
func (r *heldRun) dropTrigger() {
	wsDB, err := r.h.suite.DBManager.GetWorkspaceDB(r.h.workspaceID)
	if err != nil {
		return
	}
	_, _ = wsDB.Exec(`DROP TRIGGER IF EXISTS notifuse_test_held_enqueue ON email_queue`)
}

// waitForUngrantedLock polls pg_locks until a backend connected to the
// workspace database waits on a lock matching cond. The database is the
// backend's: a row-lock wait is on the holder's transaction id, which pg_locks
// lists with no database.
func waitForUngrantedLock(t *testing.T, h *phase2Harness, cond, message string) {
	t.Helper()
	wsDB, err := h.suite.DBManager.GetWorkspaceDB(h.workspaceID)
	require.NoError(t, err)

	waitCtx, cancel := context.WithTimeout(context.Background(), statusFlipTimeout)
	defer cancel()
	require.NoError(t, testutil.WaitForConditionWithContext(waitCtx, t, func() bool {
		var waiting int
		qErr := wsDB.QueryRow(`SELECT count(*) FROM pg_locks l
			JOIN pg_stat_activity a ON a.pid = l.pid
			WHERE NOT l.granted AND ` + cond + `
			  AND a.datname = current_database()`).Scan(&waiting)
		return qErr == nil && waiting > 0
	}, 50*time.Millisecond, message))
}

// TestBroadcastResumeDuringLastAttempt_KeepsItsRetries covers a resume that
// lands while the last attempt holds the task: the resume gives the task a
// fresh retry budget, so when that attempt then fails the task is retried, and
// the broadcast must stay as the resume left it.
func TestBroadcastResumeDuringLastAttempt_KeepsItsRetries(t *testing.T) {
	const contacts = 60

	h := setupPhase2(t, contacts, 6000)
	defer h.Cleanup()

	h.scheduleAsync(t)
	initTick(t, h)

	run := holdLastAttempt(t, h)
	defer run.cleanup()

	pauseResp, err := h.client.PauseBroadcast(map[string]interface{}{
		"workspace_id": h.workspaceID, "id": h.broadcastID})
	require.NoError(t, err)
	pauseResp.Body.Close()
	require.Equal(t, http.StatusOK, pauseResp.StatusCode)
	h.waitForBroadcastStatus(t, []string{"paused"}, statusFlipTimeout)

	resumeResp, err := h.client.ResumeBroadcast(map[string]interface{}{
		"workspace_id": h.workspaceID, "id": h.broadcastID})
	require.NoError(t, err)
	resumeResp.Body.Close()
	require.Equal(t, http.StatusOK, resumeResp.StatusCode)
	h.waitForBroadcastStatus(t, []string{"processing"}, statusFlipTimeout)

	waitCtx, cancel := context.WithTimeout(context.Background(), statusFlipTimeout)
	defer cancel()
	require.NoError(t, testutil.WaitForConditionWithContext(waitCtx, t, func() bool {
		status, retries, _ := taskRow(t, h)
		return status == "running" && retries == 0
	}, 50*time.Millisecond, "the resume never refilled the budget of the task its run holds"))

	run.release()
	res := run.result()
	require.NoError(t, res.err)
	assert.Equal(t, http.StatusInternalServerError, res.status,
		"the injected enqueue failure must fail the run")

	status, retries, _ := taskRow(t, h)
	bd := h.getBroadcast(t)
	t.Logf("after the last attempt failed past a resume: task_status=%s retry_count=%d broadcast_status=%v",
		status, retries, bd["status"])

	assert.Equal(t, "pending", status, "the resume's fresh budget makes this failure a retried one")
	assert.Equal(t, 1, retries)
	assert.Equal(t, "processing", bd["status"],
		"a broadcast resumed during its last attempt must not be marked failed when its task retries")

	run.dropTrigger()
	finishBroadcast(t, h, contacts)
}

// TestBroadcastPauseDuringLastAttemptFailure_IsKept covers a pause whose
// transaction commits while the failed last attempt finalizes its broadcast:
// the finalizer reads the broadcast still sending, and its write then waits on
// the row the pause holds. The write must find the pause on the row and keep
// it, where a write decided on the read turned it into a failure, which cannot
// be resumed.
func TestBroadcastPauseDuringLastAttemptFailure_IsKept(t *testing.T) {
	const contacts = 60

	h := setupPhase2(t, contacts, 6000)
	defer h.Cleanup()

	h.scheduleAsync(t)
	initTick(t, h)

	run := holdLastAttempt(t, h)
	defer run.cleanup()

	// A pause in flight: the broadcast's row is set paused by a transaction
	// that stays open, as PauseBroadcast's does while it pauses the queue rows
	// and runs the task handler.
	wsDB, err := h.suite.DBManager.GetWorkspaceDB(h.workspaceID)
	require.NoError(t, err)
	pause, err := wsDB.BeginTx(context.Background(), nil)
	require.NoError(t, err)
	defer func() { _ = pause.Rollback() }()
	_, err = pause.Exec(`UPDATE broadcasts SET status = 'paused', paused_at = now() WHERE id = $1`, h.broadcastID)
	require.NoError(t, err)

	run.release()

	// The finalizer read the committed status and now waits on the row lock.
	waitForUngrantedLock(t, h, "locktype IN ('transactionid', 'tuple')",
		"the finalizer's write never waited on the pause's row lock")
	require.NoError(t, pause.Commit())

	res := run.result()
	require.NoError(t, res.err)
	assert.Equal(t, http.StatusInternalServerError, res.status,
		"the injected enqueue failure must fail the run")

	status, retries, _ := taskRow(t, h)
	bd := h.getBroadcast(t)
	t.Logf("after the last attempt failed under a pause: task_status=%s retry_count=%d broadcast_status=%v",
		status, retries, bd["status"])

	assert.Equal(t, "failed", status, "the last attempt failed for good")
	assert.Equal(t, 4, retries)
	assert.Equal(t, "paused", bd["status"],
		"a pause committed during the last attempt must not be overwritten with failed")

	// And it can still be finished: resuming wakes the failed task.
	run.dropTrigger()
	resumeResp, err := h.client.ResumeBroadcast(map[string]interface{}{
		"workspace_id": h.workspaceID, "id": h.broadcastID})
	require.NoError(t, err)
	resumeResp.Body.Close()
	require.Equal(t, http.StatusOK, resumeResp.StatusCode)
	finishBroadcast(t, h, contacts)
}
