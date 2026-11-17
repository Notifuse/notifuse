//go:build integration

package integration

import (
	"net/http"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/tests/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The real pipeline, live scheduler, queue worker and SMTP delivery to Mailpit: a scheduled
// A/B test sends its sample, waits for its winner and sends the winner to the rest; an
// automatic winner leaves out the time the test spent paused.
//
// Plan: Notifuse/architecture plans/notifuse/fix-scheduled-ab-repair-and-pause-clock-plan.md

func TestBroadcastScheduledABTest_E2E_SendsTheSampleThenTheWinner(t *testing.T) {
	h := setupSingleExecution(t, true)
	require.NoError(t, testutil.ClearMailpitMessages(t))
	const contacts = 40
	listID := h.seed("e2e-sched", contacts)
	a, b := h.template("A", 0), h.template("B", 0)
	broadcastID := h.abBroadcast(listID, a, b)

	h.schedulePast(broadcastID)
	h.waitFor(time.Minute, 200*time.Millisecond, "the test to wait for its winner", func() bool {
		return h.status(broadcastID) == domain.BroadcastStatusTestCompleted
	})
	require.NoError(t, testutil.WaitForQueueEmpty(t, h.suite.ServerManager.GetApp().GetEmailQueueRepository(), h.workspaceID, time.Minute))
	h.waitFor(time.Minute, 500*time.Millisecond, "Mailpit to receive the sample", func() bool {
		n := 0
		for _, variations := range h.received() {
			n += len(variations)
		}
		return n == contacts/2
	})
	assertSampleOfBoth(t, h.received(), contacts/2)

	h.pickWinner(broadcastID, b)
	got := h.delivered(broadcastID)
	assertEachContactOnce(t, got, contacts)
	assertWinnerOnly(t, got, h.outsideSample(contacts/2), "B")
}

func TestBroadcastABTest_E2E_AutomaticWinnerLeavesThePauseOut(t *testing.T) {
	h := setupSingleExecution(t, true)
	require.NoError(t, testutil.ClearMailpitMessages(t))
	require.NoError(t, h.factory.EnableEmailTracking(h.workspaceID))
	const contacts = 40
	listID := h.seed("e2e-auto", contacts)
	a, b := h.template("A", 0), h.template("B", 0)
	broadcastID := h.createBroadcast(listID, map[string]interface{}{
		"enabled": true, "sample_percentage": 50, "auto_send_winner": true,
		"auto_send_winner_metric": "open_rate", "test_duration_hours": 1,
		"variations": []map[string]interface{}{
			{"variation_name": "A", "template_id": a.ID},
			{"variation_name": "B", "template_id": b.ID},
		},
	})

	h.schedulePast(broadcastID)
	h.waitFor(time.Minute, 200*time.Millisecond, "the test to wait for its winner", func() bool {
		return h.status(broadcastID) == domain.BroadcastStatusTestCompleted
	})
	require.NoError(t, testutil.WaitForQueueEmpty(t, h.suite.ServerManager.GetApp().GetEmailQueueRepository(), h.workspaceID, time.Minute))
	var tracked int
	require.NoError(t, h.db.QueryRow(`SELECT count(*) FROM message_history WHERE broadcast_id = $1 AND opens_tracked`, broadcastID).Scan(&tracked))
	require.Equal(t, contacts/2, tracked, "the sample's opens are tracked")
	// B's sample opens its emails: B is the winner the evaluation picks.
	_, err := h.db.Exec(`UPDATE message_history SET opened_at = now() WHERE broadcast_id = $1 AND template_id = $2`, broadcastID, b.ID)
	require.NoError(t, err)

	// Sent three hours ago, paused two and a half of them: 30 minutes of a 1-hour test.
	h.pause(broadcastID)
	_, err = h.db.Exec(`UPDATE broadcasts SET test_sent_at = now() - interval '3 hours', paused_at = now() - interval '150 minutes' WHERE id = $1`, broadcastID)
	require.NoError(t, err)
	resumedAt := time.Now().UTC()
	h.resume(broadcastID)

	resumed := h.broadcast(broadcastID)
	assert.InDelta(t, 150*60, resumed.TestPausedSeconds, 60, "the resume counts the time paused")
	h.waitFor(time.Minute, 200*time.Millisecond, "a run after the resume", func() bool {
		last := h.task(broadcastID).LastRunAt
		return last != nil && last.After(resumedAt.Add(time.Second))
	})
	assert.Equal(t, domain.BroadcastStatusTestCompleted, h.status(broadcastID), "no winner before the test time has passed")
	assert.Nil(t, h.broadcast(broadcastID).WinningTemplate)

	// Four hours since the test, two and a half paused: the hour has passed.
	_, err = h.db.Exec(`UPDATE broadcasts SET test_sent_at = now() - interval '4 hours' WHERE id = $1`, broadcastID)
	require.NoError(t, err)
	got := h.delivered(broadcastID)
	winner := h.broadcast(broadcastID).WinningTemplate
	require.NotNil(t, winner)
	assert.Equal(t, b.ID, *winner, "the variation opened wins")
	assertEachContactOnce(t, got, contacts)
	assertWinnerOnly(t, got, h.outsideSample(contacts/2), "B")
}

// The resume reads the broadcast without its lock: two at once count the pause once, and
// the second is refused.
func TestBroadcastABTest_ConcurrentResumesCountThePauseOnce(t *testing.T) {
	const contacts = 20
	h, broadcastID, _, _, _ := abAwaitingWinner(t, "ab-resume-twice", contacts)

	h.pause(broadcastID)
	_, err := h.db.Exec(`UPDATE broadcasts SET paused_at = paused_at - interval '1 hour' WHERE id = $1`, broadcastID)
	require.NoError(t, err)
	// The first resume holds the broadcast's row until the second one waits on it.
	release := h.holdLock(`SELECT 1 FROM broadcasts WHERE id = '` + broadcastID + `' FOR UPDATE`)
	answers := make(chan execution, 2)
	for i := 0; i < 2; i++ {
		go func() {
			resp, err := h.client.ResumeBroadcast(map[string]interface{}{"workspace_id": h.workspaceID, "id": broadcastID})
			if err != nil {
				answers <- execution{err: err}
				return
			}
			_ = resp.Body.Close()
			answers <- execution{code: resp.StatusCode}
		}()
	}
	h.waitUntil("both resumes to wait for the row", func() bool { return h.blockedOn("%test_paused_seconds%") == 2 })
	release()

	first, second := h.answer(answers), h.answer(answers)
	ok := 0
	for _, code := range []int{first, second} {
		if code == http.StatusOK {
			ok++
		}
	}
	assert.Equal(t, 1, ok, "one resume goes through, the other is refused (%d, %d)", first, second)
	resumed := h.broadcast(broadcastID)
	assert.Equal(t, domain.BroadcastStatusTestCompleted, resumed.Status)
	assert.InDelta(t, 3600, resumed.TestPausedSeconds, 60, "the hour paused is counted once")
}
