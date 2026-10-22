package integration

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/tests/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Guards that stop the same work running twice. Both are unit-tested against
// mocks; these drive the real server, which is where the locking actually
// happens. Both matter more since task execution was detached from the
// dispatching connection: a task now outlives the request that started it, so
// overlapping triggers are likelier than they were.
//
// Plan: plans/task-orchestrator-test-coverage-plan.md

// taskExecutePath is the one endpoint these helpers have to sign.
const taskExecutePath = "/api/tasks.execute"

// postJSON issues a request without testify assertions, so it is safe to call
// from a goroutine (require.FailNow outside the test goroutine is undefined).
// A dispatch to taskExecutePath carries the signature that endpoint is
// authenticated by, since it has no session to authenticate against.
func postJSON(url, body string) (int, error) {
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	if strings.HasSuffix(url, taskExecutePath) {
		for name, value := range testutil.TaskExecuteHeaders(taskExecutePath, []byte(body), testutil.TestSecretKey) {
			req.Header.Set(name, value)
		}
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode, nil
}

// getCron triggers the cron endpoint and returns the status plus decoded body.
func getCron(t *testing.T, h *phase2Harness) (int, map[string]interface{}) {
	t.Helper()
	resp, err := http.Get(h.suite.ServerManager.GetURL() + "/api/cron?max_tasks=10")
	require.NoError(t, err)
	defer resp.Body.Close()

	var decoded map[string]interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&decoded))
	return resp.StatusCode, decoded
}

// duplicateEnqueueCheck reports the total queue rows and how many distinct
// recipients they cover. The two must be equal: nobody may be enqueued twice.
func duplicateEnqueueCheck(t *testing.T, h *phase2Harness) (rows int, distinct int) {
	t.Helper()
	wsDB, err := h.suite.DBManager.GetWorkspaceDB(h.workspaceID)
	require.NoError(t, err)
	require.NoError(t, wsDB.QueryRow(
		`SELECT count(*), count(DISTINCT contact_email) FROM email_queue WHERE source_id = $1`,
		h.broadcastID).Scan(&rows, &distinct))
	return rows, distinct
}

// TestTaskExecute_ConcurrentDispatchClaimsOnce pins the claim guard end to end.
// Two dispatches racing for one task must not both run it — the loser has to be
// turned away, or a broadcast gets enqueued twice.
func TestTaskExecute_ConcurrentDispatchClaimsOnce(t *testing.T) {
	h := setupPhase2(t, 200, 6000)
	defer h.Cleanup()

	h.scheduleAsync(t)
	initTick(t, h)

	// The winner must still be working when the loser arrives, otherwise the
	// second dispatch finds a finished task and the race never happens.
	defer slowEnqueue(t, h, 300*time.Millisecond)()

	url := h.suite.ServerManager.GetURL() + "/api/tasks.execute"
	body := fmt.Sprintf(`{"workspace_id":%q,"id":%q}`, h.workspaceID, h.taskID)

	var wg sync.WaitGroup
	codes := make([]int, 2)
	errs := make([]error, 2)
	for i := range codes {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			codes[i], errs[i] = postJSON(url, body)
		}(i)
	}
	wg.Wait()

	require.NoError(t, errs[0])
	require.NoError(t, errs[1])

	sort.Ints(codes)
	t.Logf("concurrent dispatch status codes: %v", codes)
	assert.Equal(t, []int{http.StatusOK, http.StatusConflict}, codes,
		"exactly one dispatch may claim the task; the other must be turned away")

	rows, distinct := duplicateEnqueueCheck(t, h)
	assert.Equal(t, rows, distinct,
		"a rejected dispatch must not have enqueued anyone a second time")
}

// TestCron_ConcurrentTriggerIsRejected pins the single-flight guard on
// /api/cron. The endpoint is public and unauthenticated, and since it answers
// immediately this guard is the only thing between it and unbounded background
// runs.
func TestCron_ConcurrentTriggerIsRejected(t *testing.T) {
	h := setupPhase2(t, 200, 6000)
	defer h.Cleanup()

	h.scheduleAsync(t)
	initTick(t, h)

	// Keeps the first run in flight long enough for the second trigger to land.
	defer slowEnqueue(t, h, 300*time.Millisecond)()

	// Deterministic by construction: the guard is claimed synchronously, before
	// the first response is written, so a trigger sent after that response
	// cannot fail to see it.
	firstCode, firstBody := getCron(t, h)
	assert.Equal(t, http.StatusAccepted, firstCode)
	assert.NotEqual(t, true, firstBody["already_running"],
		"the first trigger should start a run")

	secondCode, secondBody := getCron(t, h)
	t.Logf("second trigger: status=%d body=%v", secondCode, secondBody)
	assert.Equal(t, http.StatusAccepted, secondCode)
	assert.Equal(t, true, secondBody["already_running"],
		"a trigger arriving while a run is in flight must not start another")

	// Let the first run finish so the suite tears down cleanly.
	waitForQueuedCount(t, h, 1, 30*time.Second)
	rows, distinct := duplicateEnqueueCheck(t, h)
	assert.Equal(t, rows, distinct, "the rejected trigger must not have duplicated work")
}
