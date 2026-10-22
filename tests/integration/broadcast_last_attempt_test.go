package integration

import (
	"context"
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
// attempt that starts with one retry left (count 2 of 3) fails, the task
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

// TestBroadcastResumeDuringLastAttempt_KeepsItsRetries covers a resume that
// lands while the last attempt holds the task: the resume gives the task a
// fresh retry budget, so when that attempt then fails the task is retried, and
// the broadcast must stay as the resume left it.
//
// The run is held inside its enqueue by a trigger waiting on an advisory lock
// the test holds, then fails once the test releases it. The lock is held and
// released on one dedicated connection: a session advisory lock belongs to the
// connection that took it, and a pooled Exec may land on another.
func TestBroadcastResumeDuringLastAttempt_KeepsItsRetries(t *testing.T) {
	const contacts = 60

	h := setupPhase2(t, contacts, 6000)
	defer h.Cleanup()

	h.scheduleAsync(t)
	initTick(t, h)

	ctx := context.Background()
	wsDB, err := h.suite.DBManager.GetWorkspaceDB(h.workspaceID)
	require.NoError(t, err)

	conn, err := wsDB.Conn(ctx)
	require.NoError(t, err)
	defer conn.Close()
	_, err = conn.ExecContext(ctx, `SELECT pg_advisory_lock($1)`, heldEnqueueLock)
	require.NoError(t, err)
	locked := true
	unlock := func() {
		if locked {
			_, unlockErr := conn.ExecContext(ctx, `SELECT pg_advisory_unlock($1)`, heldEnqueueLock)
			require.NoError(t, unlockErr)
			locked = false
		}
	}
	defer unlock()

	_, err = wsDB.Exec(fmt.Sprintf(`CREATE OR REPLACE FUNCTION notifuse_test_held_enqueue() RETURNS trigger AS $$
		BEGIN PERFORM pg_advisory_xact_lock(%d); RAISE EXCEPTION 'injected enqueue failure'; END;
		$$ LANGUAGE plpgsql`, heldEnqueueLock))
	require.NoError(t, err)
	_, err = wsDB.Exec(`CREATE TRIGGER notifuse_test_held_enqueue
		BEFORE INSERT ON email_queue
		FOR EACH STATEMENT EXECUTE FUNCTION notifuse_test_held_enqueue()`)
	require.NoError(t, err)
	defer func() {
		_, _ = wsDB.Exec(`DROP TRIGGER IF EXISTS notifuse_test_held_enqueue ON email_queue`)
	}()

	setRetryCount(t, h, 3)

	// The last attempt, run in the background: its answer comes back on a
	// channel so a failure is reported by the test goroutine.
	type runResult struct {
		status int
		err    error
	}
	done := make(chan runResult, 1)
	go func() {
		resp, runErr := h.client.ExecuteTask(map[string]interface{}{
			"workspace_id": h.workspaceID,
			"id":           h.taskID,
		})
		if runErr != nil {
			done <- runResult{err: runErr}
			return
		}
		resp.Body.Close()
		done <- runResult{status: resp.StatusCode}
	}()

	waitCtx, cancel := context.WithTimeout(ctx, statusFlipTimeout)
	defer cancel()
	require.NoError(t, testutil.WaitForConditionWithContext(waitCtx, t, func() bool {
		var waiting int
		qErr := wsDB.QueryRow(fmt.Sprintf(`SELECT count(*) FROM pg_locks
			WHERE locktype = 'advisory' AND NOT granted AND objid = %d
			  AND database = (SELECT oid FROM pg_database WHERE datname = current_database())`,
			heldEnqueueLock)).Scan(&waiting)
		return qErr == nil && waiting > 0
	}, 50*time.Millisecond, "the last attempt never reached its enqueue"))

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

	require.NoError(t, testutil.WaitForConditionWithContext(waitCtx, t, func() bool {
		status, retries, _ := taskRow(t, h)
		return status == "running" && retries == 0
	}, 50*time.Millisecond, "the resume never refilled the budget of the task its run holds"))

	unlock()

	select {
	case res := <-done:
		require.NoError(t, res.err)
		assert.Equal(t, http.StatusInternalServerError, res.status,
			"the injected enqueue failure must fail the run")
	case <-time.After(statusFlipTimeout):
		t.Fatal("the last attempt did not end after the lock was released")
	}

	status, retries, _ := taskRow(t, h)
	bd := h.getBroadcast(t)
	t.Logf("after the last attempt failed past a resume: task_status=%s retry_count=%d broadcast_status=%v",
		status, retries, bd["status"])

	assert.Equal(t, "pending", status, "the resume's fresh budget makes this failure a retried one")
	assert.Equal(t, 1, retries)
	assert.Equal(t, "processing", bd["status"],
		"a broadcast resumed during its last attempt must not be marked failed when its task retries")

	_, err = wsDB.Exec(`DROP TRIGGER IF EXISTS notifuse_test_held_enqueue ON email_queue`)
	require.NoError(t, err)
	finishBroadcast(t, h, contacts)
}
