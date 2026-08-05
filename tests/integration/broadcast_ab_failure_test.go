package integration

import (
	"net/http"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/tests/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A/B testing under the batch-error change. Ending a run on a failed enqueue
// instead of carrying on affects every phase, and the A/B path has a single
// end-to-end test covering the happy path only. Failing mid-test-phase leaves
// the phase incomplete with the cursor part-way through the test audience —
// resumable in principle, previously untested.
//
// Plan: plans/task-orchestrator-test-coverage-plan.md

// failEnqueue makes every email_queue insert abort, the way a transient
// database error does mid-broadcast. Returns a function that clears the fault.
func failEnqueue(t *testing.T, h *phase2Harness) func() {
	t.Helper()

	wsDB, err := h.suite.DBManager.GetWorkspaceDB(h.workspaceID)
	require.NoError(t, err)

	_, err = wsDB.Exec(`CREATE OR REPLACE FUNCTION notifuse_test_fail_enqueue() RETURNS trigger AS $$
		BEGIN RAISE EXCEPTION 'injected enqueue failure'; END;
		$$ LANGUAGE plpgsql`)
	require.NoError(t, err)
	_, err = wsDB.Exec(`CREATE TRIGGER notifuse_test_fail_enqueue
		BEFORE INSERT ON email_queue
		FOR EACH STATEMENT EXECUTE FUNCTION notifuse_test_fail_enqueue()`)
	require.NoError(t, err)

	cleared := false
	return func() {
		if cleared {
			return
		}
		_, dropErr := wsDB.Exec(`DROP TRIGGER IF EXISTS notifuse_test_fail_enqueue ON email_queue`)
		require.NoError(t, dropErr)
		cleared = true
	}
}

// TestBroadcastABTest_BatchFailureMidTestPhase_ResumesWithoutSkipping checks
// that a failed batch during the A/B test phase neither advances the phase nor
// loses the recipients it was in the middle of.
func TestBroadcastABTest_BatchFailureMidTestPhase_ResumesWithoutSkipping(t *testing.T) {
	const contacts = 200
	const testPhaseCount = 100 // sample_percentage 50 of 200

	h := setupPhase2(t, contacts, 6000)
	defer h.Cleanup()

	// A second variation, so the broadcast is a real A/B test.
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

	clearFault := failEnqueue(t, h)
	defer clearFault()

	h.scheduleAsync(t)
	initTick(t, h)

	// One working tick with every enqueue failing. Deliberately one, so the test
	// does not depend on the task's max_retries.
	assert.Equal(t, http.StatusInternalServerError, runTask(t, h),
		"a batch that could not be enqueued must surface as a failed run")

	bd := h.getBroadcast(t)
	status, _ := bd["status"].(string)
	t.Logf("with enqueue failing: broadcast_status=%s enqueued=%d", status, queuedCount(t, h))

	require.Equal(t, int64(0), queuedCount(t, h), "fault injection should block every insert")
	assert.Equal(t, "testing", status,
		"the test phase must stay in flight — never advance to test_completed on zero sends")

	// Clear the fault: the test audience must still be reachable, i.e. the
	// cursor never moved past the batch that failed.
	clearFault()

	// Drive to completion. The status code is not asserted here: once the test
	// phase finishes the task is completed and no longer claimable, so a 409 on
	// the following tick is the expected end of the sequence, not a failure.
	for i := 0; i < 10; i++ {
		code := runTask(t, h)
		bd := h.getBroadcast(t)
		t.Logf("recovery tick %d: status=%d broadcast=%v enqueued=%d",
			i+1, code, bd["status"], queuedCount(t, h))
		if bd["status"] == "test_completed" {
			break
		}
	}

	finalStatus := h.waitForBroadcastStatus(t, []string{"test_completed"}, 30*time.Second)
	assert.Equal(t, "test_completed", finalStatus,
		"the test phase completes once its recipients are enqueued")

	enqueued := queuedCount(t, h)
	t.Logf("after clearing the fault: enqueued=%d (test phase = %d of %d)",
		enqueued, testPhaseCount, contacts)
	assert.EqualValues(t, testPhaseCount, enqueued,
		"exactly the test sample, with nobody skipped by the failed batch and nobody sent twice")
}
