//go:build integration

package integration

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
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

// A broadcast reaches each contact once, and after an A/B winner is picked the
// rest of the list gets the winner only, however the pick, a pause or a resume
// lands against the run that holds the broadcast's task.
//
// The first tests hold the run inside a batch with a table lock and ask for a
// second execution through tasks.execute, as a scheduler tick does, so the
// interleaving is fixed. The last one runs the real pipeline, live scheduler,
// queue worker and SMTP delivery to Mailpit, and counts what was received.
//
// Plan: plans/fix-broadcast-duplicate-sends-plan.md

type singleExecHarness struct {
	t           *testing.T
	suite       *testutil.IntegrationTestSuite
	client      *testutil.APIClient
	factory     *testutil.TestDataFactory
	workspaceID string
	db          *sql.DB
	// tag is in every template subject, so a test finds its own emails in Mailpit.
	tag string
}

func setupSingleExecution(t *testing.T, liveScheduler bool) *singleExecHarness {
	t.Helper()
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	t.Cleanup(testutil.CleanupTestEnvironment)

	newApp := func(cfg *config.Config) testutil.AppInterface { return app.NewApp(cfg) }
	var suite *testutil.IntegrationTestSuite
	if liveScheduler {
		suite = testutil.NewIntegrationTestSuiteWithLiveScheduler(t, newApp)
	} else {
		suite = testutil.NewIntegrationTestSuite(t, newApp)
	}
	t.Cleanup(suite.Cleanup)

	h := &singleExecHarness{
		t:       t,
		suite:   suite,
		client:  suite.APIClient,
		factory: suite.DataFactory,
		tag:     "single-exec-" + uuid.New().String()[:8],
	}
	user, err := h.factory.CreateUser()
	require.NoError(t, err)
	workspace, err := h.factory.CreateWorkspace()
	require.NoError(t, err)
	h.workspaceID = workspace.ID
	require.NoError(t, h.factory.AddUserToWorkspace(user.ID, workspace.ID, "owner"))
	_, err = h.factory.SetupWorkspaceWithSMTPProvider(workspace.ID,
		testutil.WithIntegrationEmailProvider(domain.EmailProvider{
			Kind:               domain.EmailProviderKindSMTP,
			Senders:            []domain.EmailSender{domain.NewEmailSender("noreply@notifuse.test", "Single execution")},
			SMTP:               &domain.SMTPSettings{Host: "localhost", Port: 1025, UseTLS: false},
			RateLimitPerMinute: 60000,
		}))
	require.NoError(t, err)
	require.NoError(t, h.client.Login(user.Email, "password"))
	h.client.SetWorkspaceID(workspace.ID)
	h.db, err = h.factory.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)
	return h
}

// seed creates a list of n contacts named prefix-00000@example.com and so on.
func (h *singleExecHarness) seed(prefix string, n int) string {
	list, err := h.factory.CreateList(h.workspaceID)
	require.NoError(h.t, err)
	_, err = h.db.Exec(`INSERT INTO contacts (email)
		SELECT format('%s-%s@example.com', $1::text, lpad(i::text, 5, '0')) FROM generate_series(0, $2::int - 1) AS i`, prefix, n)
	require.NoError(h.t, err)
	_, err = h.db.Exec(`INSERT INTO contact_lists (email, list_id, status)
		SELECT format('%s-%s@example.com', $1::text, lpad(i::text, 5, '0')), $3, 'active' FROM generate_series(0, $2::int - 1) AS i`, prefix, n, list.ID)
	require.NoError(h.t, err)
	return list.ID
}

// template creates a template whose subject is "<tag> <name>". slowLoop > 0
// prepends an empty Liquid loop, which costs render time and prints nothing, so
// a batch takes long enough for a pick to land while it is prepared.
func (h *singleExecHarness) template(name string, slowLoop int) *domain.Template {
	subject := h.tag + " " + name
	if slowLoop > 0 {
		subject = fmt.Sprintf("{%% for i in (1..%d) %%}{%% endfor %%}%s", slowLoop, subject)
	}
	tpl, err := h.factory.CreateTemplate(h.workspaceID, testutil.WithTemplateName(name), testutil.WithTemplateSubject(subject))
	require.NoError(h.t, err)
	return tpl
}

func (h *singleExecHarness) createBroadcast(listID string, testSettings map[string]interface{}) string {
	resp, err := h.client.CreateBroadcast(map[string]interface{}{
		"workspace_id":  h.workspaceID,
		"name":          h.tag,
		"audience":      map[string]interface{}{"list": listID, "exclude_unsubscribed": true},
		"schedule":      map[string]interface{}{"is_scheduled": false},
		"test_settings": testSettings,
	})
	require.NoError(h.t, err)
	var created map[string]interface{}
	require.NoError(h.t, json.NewDecoder(resp.Body).Decode(&created))
	_ = resp.Body.Close()
	require.Equal(h.t, http.StatusCreated, resp.StatusCode, created)
	return created["broadcast"].(map[string]interface{})["id"].(string)
}

// abBroadcast tests a and b on half the list and leaves the pick to a person.
func (h *singleExecHarness) abBroadcast(listID string, a, b *domain.Template) string {
	return h.createBroadcast(listID, map[string]interface{}{
		"enabled": true, "sample_percentage": 50, "auto_send_winner": false, "test_duration_hours": 1,
		"variations": []map[string]interface{}{
			{"variation_name": "A", "template_id": a.ID},
			{"variation_name": "B", "template_id": b.ID},
		},
	})
}

func (h *singleExecHarness) post(call func(map[string]interface{}) (*http.Response, error), body map[string]interface{}) {
	h.t.Helper()
	resp, err := call(body)
	require.NoError(h.t, err)
	b, _ := h.client.ReadBody(resp)
	require.Equal(h.t, http.StatusOK, resp.StatusCode, b)
}

func (h *singleExecHarness) sendNow(broadcastID string) {
	h.post(h.client.ScheduleBroadcast, map[string]interface{}{"workspace_id": h.workspaceID, "id": broadcastID, "send_now": true})
}

func (h *singleExecHarness) pickWinner(broadcastID string, winner *domain.Template) {
	h.post(h.client.SelectBroadcastWinner, map[string]interface{}{"workspace_id": h.workspaceID, "id": broadcastID, "template_id": winner.ID})
}

func (h *singleExecHarness) task(broadcastID string) *domain.Task {
	task, err := h.suite.ServerManager.GetApp().GetTaskRepository().GetTaskByBroadcastID(context.Background(), h.workspaceID, broadcastID)
	require.NoError(h.t, err)
	return task
}

// execute asks for one execution of the task, as a scheduler tick does, and
// returns the HTTP status.
func (h *singleExecHarness) execute(taskID string) int {
	h.t.Helper()
	return h.answer(h.start(taskID))
}

// execution is what one tasks.execute call answered.
type execution struct {
	code int
	err  error
}

// start asks for an execution in the background. Only the test goroutine may
// fail the test, so the answer, error included, comes back on the channel.
func (h *singleExecHarness) start(taskID string) <-chan execution {
	done := make(chan execution, 1)
	go func() {
		resp, err := h.client.ExecuteTask(map[string]interface{}{"workspace_id": h.workspaceID, "id": taskID})
		if err != nil {
			done <- execution{err: err}
			return
		}
		_ = resp.Body.Close()
		done <- execution{code: resp.StatusCode}
	}()
	return done
}

// answer waits for an execution started with start and returns its HTTP status.
func (h *singleExecHarness) answer(done <-chan execution) int {
	h.t.Helper()
	result := <-done
	require.NoError(h.t, result.err)
	return result.code
}

func (h *singleExecHarness) status(broadcastID string) domain.BroadcastStatus {
	b, err := h.suite.ServerManager.GetApp().GetBroadcastRepository().GetBroadcast(context.Background(), h.workspaceID, broadcastID)
	require.NoError(h.t, err)
	return b.Status
}

// executeUntil runs the task until the broadcast reaches want. A refused
// execution (another one holds the task) is retried after a pause.
func (h *singleExecHarness) executeUntil(taskID, broadcastID string, want domain.BroadcastStatus) {
	h.t.Helper()
	deadline := time.Now().Add(statusFlipTimeout)
	for h.status(broadcastID) != want {
		require.True(h.t, time.Now().Before(deadline), "broadcast still %s, waiting for %s", h.status(broadcastID), want)
		if h.execute(taskID) == http.StatusConflict {
			time.Sleep(100 * time.Millisecond)
		}
	}
}

// enqueued returns, for each contact, the variation of every email the
// broadcast put in the queue or sent, by name.
func (h *singleExecHarness) enqueued(broadcastID string, names map[string]string) map[string][]string {
	rows, err := h.db.Query(`
		SELECT contact_email, template_id FROM email_queue WHERE source_type = 'broadcast' AND source_id = $1
		UNION ALL
		SELECT contact_email, template_id FROM message_history WHERE broadcast_id = $1`, broadcastID)
	require.NoError(h.t, err)
	defer rows.Close()
	out := map[string][]string{}
	for rows.Next() {
		var email, templateID string
		require.NoError(h.t, rows.Scan(&email, &templateID))
		out[email] = append(out[email], names[templateID])
	}
	require.NoError(h.t, rows.Err())
	return out
}

func (h *singleExecHarness) pausedRows(broadcastID string) int {
	var n int
	require.NoError(h.t, h.db.QueryRow(`SELECT count(*) FROM email_queue WHERE source_type = 'broadcast' AND source_id = $1 AND status = 'paused'`,
		broadcastID).Scan(&n))
	return n
}

// outsideSample returns the contacts the winner phase sends to: the list after
// the test sample, in the order the recipient query pages through it.
func (h *singleExecHarness) outsideSample(sample int) []string {
	rows, err := h.db.Query(`SELECT email FROM contacts ORDER BY email OFFSET $1`, sample)
	require.NoError(h.t, err)
	defer rows.Close()
	var emails []string
	for rows.Next() {
		var email string
		require.NoError(h.t, rows.Scan(&email))
		emails = append(emails, email)
	}
	require.NoError(h.t, rows.Err())
	return emails
}

// holdLock takes a table lock in the workspace database and returns its
// release. The release also runs at cleanup, so a failing test never leaves a
// run blocked.
func (h *singleExecHarness) holdLock(statement string) func() {
	tx, err := h.db.BeginTx(context.Background(), nil)
	require.NoError(h.t, err)
	_, err = tx.Exec(statement)
	require.NoError(h.t, err)
	var once sync.Once
	release := func() { once.Do(func() { require.NoError(h.t, tx.Commit()) }) }
	h.t.Cleanup(func() { once.Do(func() { _ = tx.Rollback() }) })
	return release
}

// blockedOn counts the workspace queries waiting on a lock whose text starts
// like queryLike (a LIKE pattern).
func (h *singleExecHarness) blockedOn(queryLike string) int {
	var n int
	require.NoError(h.t, h.db.QueryRow(`SELECT count(*) FROM pg_stat_activity
		WHERE datname = current_database() AND wait_event_type = 'Lock' AND query LIKE $1`, queryLike).Scan(&n))
	return n
}

func (h *singleExecHarness) waitUntil(what string, cond func() bool) {
	h.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), statusFlipTimeout)
	defer cancel()
	require.NoError(h.t, testutil.WaitForConditionWithContext(ctx, h.t, cond, 20*time.Millisecond, what))
}

// secondExecution asks for an execution while the first one is held by the
// lock, and waits until it either returned or is held by the same lock, as the
// first one is.
func (h *singleExecHarness) secondExecution(taskID, blockedLike string) (wait func() int) {
	done := h.start(taskID)
	var returned *execution
	h.waitUntil("the second execution to return or block", func() bool {
		select {
		case result := <-done:
			returned = &result
			return true
		default:
			return h.blockedOn(blockedLike) == 2
		}
	})
	return func() int {
		h.t.Helper()
		if returned == nil {
			result := <-done
			returned = &result
		}
		require.NoError(h.t, returned.err)
		return returned.code
	}
}

func assertEachContactOnce(t *testing.T, got map[string][]string, contacts int) {
	t.Helper()
	total, twice := 0, 0
	for _, variations := range got {
		total += len(variations)
		if len(variations) > 1 {
			twice++
		}
	}
	assert.Len(t, got, contacts, "every contact gets the broadcast")
	assert.Equal(t, contacts, total, "one email per contact")
	assert.Zero(t, twice, "no contact gets the broadcast twice")
}

func assertWinnerOnly(t *testing.T, got map[string][]string, contacts []string, winner string) {
	t.Helper()
	require.NotEmpty(t, contacts)
	other := 0
	for _, email := range contacts {
		for _, variation := range got[email] {
			if variation != winner {
				other++
			}
		}
	}
	assert.Zero(t, other, "after the pick only the winning variation goes out")
}

const enqueueLike = "INSERT INTO email_queue%"

func TestBroadcastSingleExecution_WinnerPickedThroughTheAPIMidRun(t *testing.T) {
	h := setupSingleExecution(t, false)
	const contacts = 300
	listID := h.seed("pick", contacts)
	a, b := h.template("A", 0), h.template("B", 0)
	broadcastID := h.abBroadcast(listID, a, b)
	h.sendNow(broadcastID)
	task := h.task(broadcastID)
	require.Equal(t, http.StatusOK, h.execute(task.ID), "the counting run")

	// The first run enqueues its first test batch and is held there.
	release := h.holdLock(`LOCK TABLE email_queue IN SHARE ROW EXCLUSIVE MODE`)
	first := h.start(task.ID)
	h.waitUntil("the first run to block in its enqueue", func() bool { return h.blockedOn(enqueueLike) == 1 })
	require.Equal(t, domain.BroadcastStatusTesting, h.status(broadcastID))

	h.pickWinner(broadcastID, a)
	assert.Equal(t, domain.TaskStatusRunning, h.task(broadcastID).Status, "the task is still the first run's")

	second := h.secondExecution(task.ID, enqueueLike)
	release()
	assert.Equal(t, http.StatusOK, h.answer(first))
	assert.Equal(t, http.StatusConflict, second(), "a second execution is refused while the first holds the task")

	h.executeUntil(task.ID, broadcastID, domain.BroadcastStatusProcessed)
	got := h.enqueued(broadcastID, map[string]string{a.ID: "A", b.ID: "B"})
	assertEachContactOnce(t, got, contacts)
	assertWinnerOnly(t, got, h.outsideSample(contacts/2), "A")
	assert.Zero(t, h.pausedRows(broadcastID))
}

func TestBroadcastSingleExecution_PauseAndResumeMidBatch(t *testing.T) {
	h := setupSingleExecution(t, false)
	const contacts = 300
	listID := h.seed("pause", contacts)
	tpl := h.template("P", 0)
	broadcastID := h.createBroadcast(listID, map[string]interface{}{
		"enabled":    false,
		"variations": []map[string]interface{}{{"variation_name": "P", "template_id": tpl.ID}},
	})
	h.sendNow(broadcastID)
	task := h.task(broadcastID)
	require.Equal(t, http.StatusOK, h.execute(task.ID), "the counting run")

	// The first run is held in its first page fetch, so the pause and the
	// resume both land before it reads the broadcast again.
	const fetchLike = "SELECT c.email%FROM contacts c%"
	release := h.holdLock(`LOCK TABLE contacts IN ACCESS EXCLUSIVE MODE`)
	first := h.start(task.ID)
	h.waitUntil("the first run to block in its page fetch", func() bool { return h.blockedOn(fetchLike) == 1 })

	h.post(h.client.PauseBroadcast, map[string]interface{}{"workspace_id": h.workspaceID, "id": broadcastID})
	h.post(h.client.ResumeBroadcast, map[string]interface{}{"workspace_id": h.workspaceID, "id": broadcastID})
	held := h.task(broadcastID)
	assert.Equal(t, domain.TaskStatusRunning, held.Status, "the task is still the first run's")
	assert.NotNil(t, held.TimeoutAfter, "with its lease")

	second := h.secondExecution(task.ID, fetchLike)
	release()
	assert.Equal(t, http.StatusOK, h.answer(first))
	assert.Equal(t, http.StatusConflict, second(), "a second execution is refused while the first holds the task")

	h.executeUntil(task.ID, broadcastID, domain.BroadcastStatusProcessed)
	assertEachContactOnce(t, h.enqueued(broadcastID, map[string]string{tpl.ID: "P"}), contacts)
	assert.Zero(t, h.pausedRows(broadcastID), "no email of the broadcast stays paused")
}

// TestBroadcastSingleExecution_WinnerPickedAfterTheTestPhase is the console
// flow: the test phase ends, a waiting run passes, the winner is picked, and
// the rest of the list is sent.
func TestBroadcastSingleExecution_WinnerPickedAfterTheTestPhase(t *testing.T) {
	h := setupSingleExecution(t, false)
	const contacts = 300
	listID := h.seed("console", contacts)
	a, b := h.template("A", 0), h.template("B", 0)
	broadcastID := h.abBroadcast(listID, a, b)
	h.sendNow(broadcastID)
	task := h.task(broadcastID)

	h.executeUntil(task.ID, broadcastID, domain.BroadcastStatusTestCompleted)
	assert.Equal(t, http.StatusOK, h.execute(task.ID), "a run while the test awaits its winner")
	h.pickWinner(broadcastID, b)
	h.executeUntil(task.ID, broadcastID, domain.BroadcastStatusProcessed)

	got := h.enqueued(broadcastID, map[string]string{a.ID: "A", b.ID: "B"})
	assertEachContactOnce(t, got, contacts)
	assertWinnerOnly(t, got, h.outsideSample(contacts/2), "B")
}

// TestBroadcastSingleExecution_E2E_WinnerPickedThroughTheAPIDuringTheTestPhase
// runs the real pipeline and counts the emails Mailpit received.
func TestBroadcastSingleExecution_E2E_WinnerPickedThroughTheAPIDuringTheTestPhase(t *testing.T) {
	h := setupSingleExecution(t, true)
	require.NoError(t, testutil.ClearMailpitMessages(t))
	const contacts = 400
	listID := h.seed("e2e", contacts)
	a, b := h.template("A", 20000), h.template("B", 20000)
	broadcastID := h.abBroadcast(listID, a, b)

	h.sendNow(broadcastID)
	h.waitUntil("the test phase to start", func() bool { return h.status(broadcastID) == domain.BroadcastStatusTesting })
	h.pickWinner(broadcastID, a)

	got := h.delivered(broadcastID)
	assertEachContactOnce(t, got, contacts)
	assertWinnerOnly(t, got, h.outsideSample(contacts/2), "A")
}

// received returns, for each recipient, the variation of every email Mailpit
// received from this test, by name.
func (h *singleExecHarness) received() map[string][]string {
	out := map[string][]string{}
	httpClient := &http.Client{Timeout: 30 * time.Second}
	for start := 0; ; start += 1000 {
		resp, err := httpClient.Get(fmt.Sprintf("http://localhost:8025/api/v1/messages?start=%d&limit=1000", start))
		require.NoError(h.t, err)
		var page testutil.MailpitMessagesResponse
		require.NoError(h.t, json.NewDecoder(resp.Body).Decode(&page))
		_ = resp.Body.Close()
		for _, message := range page.Messages {
			if !strings.Contains(message.Subject, h.tag) {
				continue
			}
			for _, to := range message.To {
				email := strings.ToLower(to.Address)
				out[email] = append(out[email], strings.TrimPrefix(message.Subject, h.tag+" "))
			}
		}
		if len(page.Messages) < 1000 {
			return out
		}
	}
}

// delivered waits for the broadcast to finish, its queue to drain and Mailpit to
// stop receiving, then returns what Mailpit received.
func (h *singleExecHarness) delivered(broadcastID string) map[string][]string {
	h.t.Helper()
	deadline := time.Now().Add(4 * time.Minute)
	for status := h.status(broadcastID); status != domain.BroadcastStatusProcessed; status = h.status(broadcastID) {
		require.NotContains(h.t, []domain.BroadcastStatus{domain.BroadcastStatusFailed, domain.BroadcastStatusCancelled}, status)
		require.True(h.t, time.Now().Before(deadline), "broadcast still %s", status)
		time.Sleep(200 * time.Millisecond)
	}
	require.NoError(h.t, testutil.WaitForQueueEmpty(h.t, h.suite.ServerManager.GetApp().GetEmailQueueRepository(), h.workspaceID, 4*time.Minute))
	last, stableSince := -1, time.Now()
	for time.Since(stableSince) < 3*time.Second {
		n := 0
		for _, variations := range h.received() {
			n += len(variations)
		}
		if n != last {
			last, stableSince = n, time.Now()
		}
		time.Sleep(500 * time.Millisecond)
	}
	return h.received()
}
