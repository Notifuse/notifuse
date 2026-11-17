//go:build integration

package integration

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A scheduled broadcast leaves scheduled when its sending starts: an A/B test sends its
// sample and waits for its winner, as one sent right away does, and any other broadcast
// shows as sending. An A/B broadcast can be paused and cancelled at every step of its test.

// schedulePast schedules the broadcast two minutes ago, so its task is due at once.
func (h *singleExecHarness) schedulePast(broadcastID string) {
	at := time.Now().UTC().Add(-2 * time.Minute)
	h.post(h.client.ScheduleBroadcast, map[string]interface{}{
		"workspace_id":           h.workspaceID,
		"id":                     broadcastID,
		"send_now":               false,
		"scheduled_date":         at.Format("2006-01-02"),
		"scheduled_time":         at.Format("15:04"),
		"timezone":               "UTC",
		"use_recipient_timezone": false,
	})
}

// recordStatuses logs every status the broadcast is written with, in order, so a status
// a single run passes through is seen. It returns the log's reader.
func (h *singleExecHarness) recordStatuses(broadcastID string) func() []domain.BroadcastStatus {
	h.t.Helper()
	_, err := h.db.Exec(`CREATE TABLE IF NOT EXISTS notifuse_test_status_log (
		seq bigserial PRIMARY KEY, broadcast_id varchar(255) NOT NULL, status varchar(20) NOT NULL)`)
	require.NoError(h.t, err)
	_, err = h.db.Exec(`CREATE OR REPLACE FUNCTION notifuse_test_log_status() RETURNS trigger AS $$
		BEGIN
			INSERT INTO notifuse_test_status_log (broadcast_id, status) VALUES (NEW.id, NEW.status);
			RETURN NEW;
		END;
		$$ LANGUAGE plpgsql`)
	require.NoError(h.t, err)
	_, err = h.db.Exec(`CREATE TRIGGER notifuse_test_log_status
		AFTER UPDATE OF status ON broadcasts
		FOR EACH ROW WHEN (OLD.status IS DISTINCT FROM NEW.status) EXECUTE FUNCTION notifuse_test_log_status()`)
	require.NoError(h.t, err)
	h.t.Cleanup(func() {
		_, _ = h.db.Exec(`DROP TRIGGER IF EXISTS notifuse_test_log_status ON broadcasts`)
		_, _ = h.db.Exec(`DROP FUNCTION IF EXISTS notifuse_test_log_status()`)
		_, _ = h.db.Exec(`DROP TABLE IF EXISTS notifuse_test_status_log`)
	})
	return func() []domain.BroadcastStatus {
		rows, err := h.db.Query(`SELECT status FROM notifuse_test_status_log WHERE broadcast_id = $1 ORDER BY seq`, broadcastID)
		require.NoError(h.t, err)
		defer rows.Close()
		var out []domain.BroadcastStatus
		for rows.Next() {
			var s string
			require.NoError(h.t, rows.Scan(&s))
			out = append(out, domain.BroadcastStatus(s))
		}
		require.NoError(h.t, rows.Err())
		return out
	}
}

func (h *singleExecHarness) broadcast(broadcastID string) *domain.Broadcast {
	h.t.Helper()
	b, err := h.suite.ServerManager.GetApp().GetBroadcastRepository().GetBroadcast(h.t.Context(), h.workspaceID, broadcastID)
	require.NoError(h.t, err)
	return b
}

// sent counts the emails of the broadcast in the queue or sent.
func (h *singleExecHarness) sent(broadcastID string) int {
	n := 0
	for _, variations := range h.enqueued(broadcastID, map[string]string{}) {
		n += len(variations)
	}
	return n
}

// assertSampleOfBoth checks the test sample got one of the two variations each.
func assertSampleOfBoth(t *testing.T, got map[string][]string, sample int) {
	t.Helper()
	total := 0
	for email, variations := range got {
		total += len(variations)
		require.Len(t, variations, 1, "%s gets the test once", email)
		assert.Contains(t, []string{"A", "B"}, variations[0])
	}
	assert.Equal(t, sample, total, "the test goes to its sample only")
}

func TestBroadcastScheduledABTest_SendsItsSampleAndWaitsForItsWinner(t *testing.T) {
	h := setupSingleExecution(t, false)
	const contacts = 20
	listID := h.seed("sched-ab", contacts)
	a, b := h.template("A", 0), h.template("B", 0)
	names := map[string]string{a.ID: "A", b.ID: "B"}
	broadcastID := h.abBroadcast(listID, a, b)
	statuses := h.recordStatuses(broadcastID)

	h.schedulePast(broadcastID)
	require.Equal(t, domain.BroadcastStatusScheduled, h.status(broadcastID))
	task := h.task(broadcastID)
	h.executeUntil(task.ID, broadcastID, domain.BroadcastStatusTestCompleted)

	assert.Equal(t, []domain.BroadcastStatus{
		domain.BroadcastStatusScheduled,
		domain.BroadcastStatusTesting,
		domain.BroadcastStatusTestCompleted,
	}, statuses(), "the scheduled broadcast goes through its test phase")
	tested := h.broadcast(broadcastID)
	assert.NotNil(t, tested.TestSentAt, "the test is dated")
	assert.NotNil(t, tested.StartedAt, "the sending is dated")
	assert.Equal(t, contacts/2, tested.TestPhaseRecipientCount)
	assertSampleOfBoth(t, h.enqueued(broadcastID, names), contacts/2)

	assert.Equal(t, http.StatusOK, h.execute(task.ID), "a run while the test awaits its winner")
	assert.Equal(t, contacts/2, h.sent(broadcastID), "nothing more goes out before the pick")

	h.pickWinner(broadcastID, a)
	h.executeUntil(task.ID, broadcastID, domain.BroadcastStatusProcessed)
	got := h.enqueued(broadcastID, names)
	assertEachContactOnce(t, got, contacts)
	assertWinnerOnly(t, got, h.outsideSample(contacts/2), "A")
}

func TestBroadcastScheduledSend_ShowsAsSendingOnceStarted(t *testing.T) {
	h := setupSingleExecution(t, false)
	const contacts = 20
	listID := h.seed("sched-one", contacts)
	tpl := h.template("P", 0)
	broadcastID := h.createBroadcast(listID, map[string]interface{}{
		"enabled":    false,
		"variations": []map[string]interface{}{{"variation_name": "P", "template_id": tpl.ID}},
	})
	statuses := h.recordStatuses(broadcastID)

	h.schedulePast(broadcastID)
	task := h.task(broadcastID)
	h.executeUntil(task.ID, broadcastID, domain.BroadcastStatusProcessed)

	assert.Equal(t, []domain.BroadcastStatus{
		domain.BroadcastStatusScheduled,
		domain.BroadcastStatusProcessing,
		domain.BroadcastStatusProcessed,
	}, statuses())
	assertEachContactOnce(t, h.enqueued(broadcastID, map[string]string{tpl.ID: "P"}), contacts)
}

// abAwaitingWinner sends an A/B broadcast now and runs it until its test awaits a winner.
func abAwaitingWinner(t *testing.T, prefix string, contacts int) (h *singleExecHarness, broadcastID, taskID string, a, b *domain.Template) {
	h = setupSingleExecution(t, false)
	listID := h.seed(prefix, contacts)
	a, b = h.template("A", 0), h.template("B", 0)
	broadcastID = h.abBroadcast(listID, a, b)
	h.sendNow(broadcastID)
	taskID = h.task(broadcastID).ID
	h.executeUntil(taskID, broadcastID, domain.BroadcastStatusTestCompleted)
	require.Equal(t, contacts/2, h.sent(broadcastID))
	return h, broadcastID, taskID, a, b
}

func (h *singleExecHarness) pause(broadcastID string) {
	h.post(h.client.PauseBroadcast, map[string]interface{}{"workspace_id": h.workspaceID, "id": broadcastID})
}

func (h *singleExecHarness) resume(broadcastID string) {
	h.post(h.client.ResumeBroadcast, map[string]interface{}{"workspace_id": h.workspaceID, "id": broadcastID})
}

func TestBroadcastABTest_PausedWhileAwaitingItsWinner(t *testing.T) {
	const contacts = 20
	h, broadcastID, taskID, a, b := abAwaitingWinner(t, "ab-pause-wait", contacts)

	h.pause(broadcastID)
	require.Equal(t, domain.BroadcastStatusPaused, h.status(broadcastID))
	h.execute(taskID)
	assert.Equal(t, domain.BroadcastStatusPaused, h.status(broadcastID), "a run leaves the pause alone")

	h.resume(broadcastID)
	assert.Equal(t, domain.BroadcastStatusTestCompleted, h.status(broadcastID), "the resume goes back to waiting for the winner")
	h.pickWinner(broadcastID, b)
	h.executeUntil(taskID, broadcastID, domain.BroadcastStatusProcessed)

	got := h.enqueued(broadcastID, map[string]string{a.ID: "A", b.ID: "B"})
	assertEachContactOnce(t, got, contacts)
	assertWinnerOnly(t, got, h.outsideSample(contacts/2), "B")
}

func TestBroadcastABTest_PausedOnceItsWinnerIsPicked(t *testing.T) {
	const contacts = 20
	h, broadcastID, taskID, a, b := abAwaitingWinner(t, "ab-pause-winner", contacts)

	h.pickWinner(broadcastID, a)
	h.pause(broadcastID)
	require.Equal(t, domain.BroadcastStatusPaused, h.status(broadcastID))
	h.execute(taskID)
	assert.Equal(t, contacts/2, h.sent(broadcastID), "the winner does not go out while paused")

	h.resume(broadcastID)
	assert.Equal(t, domain.BroadcastStatusWinnerSelected, h.status(broadcastID), "the resume goes back to sending the winner")
	h.executeUntil(taskID, broadcastID, domain.BroadcastStatusProcessed)

	got := h.enqueued(broadcastID, map[string]string{a.ID: "A", b.ID: "B"})
	assertEachContactOnce(t, got, contacts)
	assertWinnerOnly(t, got, h.outsideSample(contacts/2), "A")
}

func TestBroadcastABTest_CancelledWhileAwaitingItsWinner(t *testing.T) {
	const contacts = 20
	h, broadcastID, taskID, _, b := abAwaitingWinner(t, "ab-cancel", contacts)

	h.post(h.client.CancelBroadcast, map[string]interface{}{"workspace_id": h.workspaceID, "id": broadcastID})
	require.Equal(t, domain.BroadcastStatusCancelled, h.status(broadcastID))
	var queued int
	require.NoError(t, h.db.QueryRow(`SELECT count(*) FROM email_queue
		WHERE source_type = 'broadcast' AND source_id = $1 AND status = 'pending'`, broadcastID).Scan(&queued))
	assert.Zero(t, queued, "the cancel removes the emails still waiting in the queue")
	after := h.sent(broadcastID)

	resp, err := h.client.SelectBroadcastWinner(map[string]interface{}{"workspace_id": h.workspaceID, "id": broadcastID, "template_id": b.ID})
	require.NoError(t, err)
	_ = resp.Body.Close()
	assert.NotEqual(t, http.StatusOK, resp.StatusCode, "no winner is picked for a cancelled test")

	assert.Equal(t, http.StatusOK, h.execute(taskID))
	assert.Equal(t, domain.BroadcastStatusCancelled, h.status(broadcastID))
	assert.Equal(t, after, h.sent(broadcastID), "nothing more goes out")
	assert.Equal(t, domain.TaskStatusCompleted, h.task(broadcastID).Status, fmt.Sprintf("the run ends the task of broadcast %s", broadcastID))
}
