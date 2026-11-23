package integration

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/tests/testutil"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A broadcast whose emails are still queued cannot be deleted: the queue worker would
// send them for a broadcast nobody can see, and nothing would ever resume the paused
// ones. broadcasts.delete answers 409 until a cancel empties the queue.
func TestBroadcastDelete_RefusedWhileEmailsAreQueued(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer func() { suite.Cleanup() }()

	client := suite.APIClient
	factory := suite.DataFactory

	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	user, err := factory.CreateUser()
	require.NoError(t, err)
	require.NoError(t, factory.AddUserToWorkspace(user.ID, workspace.ID, "owner"))
	require.NoError(t, client.Login(user.Email, "password"))
	client.SetWorkspaceID(workspace.ID)

	broadcast, err := factory.CreateBroadcast(workspace.ID, testutil.WithBroadcastStatus(domain.BroadcastStatusProcessed))
	require.NoError(t, err)

	wsDB, err := suite.DBManager.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)
	// Rows the worker will not pick up during the test: a pending row and a retrying
	// one both due tomorrow, and a paused one. A row given up on is added too, to show
	// that it does not hold the delete back on its own.
	rows := []struct{ status, nextRetry string }{
		{"pending", "NOW() + INTERVAL '1 day'"},
		{"paused", "NULL"},
		{"failed", "NOW() + INTERVAL '1 day'"},
		{"failed", "NULL"},
	}
	for i, row := range rows {
		_, err := wsDB.Exec(fmt.Sprintf(`
			INSERT INTO email_queue
				(id, status, priority, source_type, source_id, integration_id, provider_kind,
				 contact_email, message_id, template_id, payload, attempts, next_retry_at)
			VALUES ($1, $2, 5, 'broadcast', $3, 'i-1', 'smtp', $4, $5, 'tpl-1', '{"subject":"hi"}'::jsonb, 1, %s)`, row.nextRetry),
			uuid.NewString(), row.status, broadcast.ID, fmt.Sprintf("r%d@example.com", i), uuid.NewString())
		require.NoError(t, err)
	}

	deleteBody := fmt.Sprintf(`{"workspace_id":%q,"id":%q}`, workspace.ID, broadcast.ID)
	readBody := func(resp *http.Response) string {
		t.Helper()
		defer func() { _ = resp.Body.Close() }()
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		return string(body)
	}

	resp, err := client.PostRaw("/api/broadcasts.delete", deleteBody)
	require.NoError(t, err)
	assert.Equal(t, http.StatusConflict, resp.StatusCode)
	assert.JSONEq(t, fmt.Sprintf(`{
		"error": "cannot delete broadcast %[1]s: its status is processed. Its email queue still holds 3 emails to send: cancel it first, which empties the queue.",
		"invalid_state": {"entity":"broadcast","id":%[1]q,"action":"delete","status":"processed",
			"reason":"Its email queue still holds 3 emails to send: cancel it first, which empties the queue."}
	}`, broadcast.ID), readBody(resp))

	var left int
	require.NoError(t, wsDB.QueryRow(`SELECT count(*) FROM broadcasts WHERE id = $1`, broadcast.ID).Scan(&left))
	assert.Equal(t, 1, left, "the refused delete leaves the broadcast")

	resp, err = client.PostRaw("/api/broadcasts.cancel", deleteBody)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode, readBody(resp))

	resp, err = client.PostRaw("/api/broadcasts.delete", deleteBody)
	require.NoError(t, err)
	body := readBody(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode, body)
	var ok map[string]any
	require.NoError(t, json.Unmarshal([]byte(body), &ok))
	assert.Equal(t, true, ok["success"])

	require.NoError(t, wsDB.QueryRow(`SELECT count(*) FROM broadcasts WHERE id = $1`, broadcast.ID).Scan(&left))
	assert.Equal(t, 0, left)
	var queued int
	require.NoError(t, wsDB.QueryRow(`SELECT count(*) FROM email_queue WHERE source_id = $1`, broadcast.ID).Scan(&queued))
	assert.Equal(t, 0, queued, "the cancel emptied the queue")
}

// deleteQueueFixture is a workspace whose owner is signed in, with a way to put rows
// in a broadcast's email queue that the worker will not pick up during the test.
type deleteQueueFixture struct {
	suite     *testutil.IntegrationTestSuite
	client    *testutil.APIClient
	workspace *domain.Workspace
	wsDB      *sql.DB
}

func newDeleteQueueFixture(t *testing.T) *deleteQueueFixture {
	t.Helper()
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	t.Cleanup(testutil.CleanupTestEnvironment)

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	t.Cleanup(func() { suite.Cleanup() })

	workspace, err := suite.DataFactory.CreateWorkspace()
	require.NoError(t, err)
	user, err := suite.DataFactory.CreateUser()
	require.NoError(t, err)
	require.NoError(t, suite.DataFactory.AddUserToWorkspace(user.ID, workspace.ID, "owner"))
	require.NoError(t, suite.APIClient.Login(user.Email, "password"))
	suite.APIClient.SetWorkspaceID(workspace.ID)

	wsDB, err := suite.DBManager.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)
	return &deleteQueueFixture{suite: suite, client: suite.APIClient, workspace: workspace, wsDB: wsDB}
}

func (f *deleteQueueFixture) broadcast(t *testing.T, status domain.BroadcastStatus) *domain.Broadcast {
	t.Helper()
	b, err := f.suite.DataFactory.CreateBroadcast(f.workspace.ID, testutil.WithBroadcastStatus(status))
	require.NoError(t, err)
	return b
}

func (f *deleteQueueFixture) queueRow(t *testing.T, broadcastID, status, nextRetry string) {
	t.Helper()
	_, err := f.wsDB.Exec(fmt.Sprintf(`
		INSERT INTO email_queue
			(id, status, priority, source_type, source_id, integration_id, provider_kind,
			 contact_email, message_id, template_id, payload, attempts, next_retry_at)
		VALUES ($1, $2, 5, 'broadcast', $3, 'i-1', 'smtp', $4, $5, 'tpl-1', '{"subject":"hi"}'::jsonb, 1, %s)`, nextRetry),
		uuid.NewString(), status, broadcastID, uuid.NewString()+"@example.com", uuid.NewString())
	require.NoError(t, err)
}

func (f *deleteQueueFixture) post(t *testing.T, endpoint, broadcastID string) (int, string) {
	t.Helper()
	resp, err := f.client.PostRaw(endpoint, fmt.Sprintf(`{"workspace_id":%q,"id":%q}`, f.workspace.ID, broadcastID))
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp.StatusCode, string(body)
}

func (f *deleteQueueFixture) count(t *testing.T, query, broadcastID string) int {
	t.Helper()
	var n int
	require.NoError(t, f.wsDB.QueryRow(query, broadcastID).Scan(&n))
	return n
}

func TestBroadcastDelete_ScheduledProcessingAndAbandonedRows(t *testing.T) {
	f := newDeleteQueueFixture(t)

	// A scheduled broadcast keeps a send task that would outlive it, so it has to be
	// unscheduled first; unscheduling removes the task and leaves a draft.
	t.Run("a scheduled broadcast is refused until it is unscheduled", func(t *testing.T) {
		b := f.broadcast(t, domain.BroadcastStatusScheduled)

		code, body := f.post(t, "/api/broadcasts.delete", b.ID)
		assert.Equal(t, http.StatusConflict, code)
		assert.JSONEq(t, fmt.Sprintf(`{
			"error": "cannot delete broadcast %[1]s: its status is scheduled. Unschedule it first: unscheduling removes its send task.",
			"invalid_state": {"entity":"broadcast","id":%[1]q,"action":"delete","status":"scheduled",
				"reason":"Unschedule it first: unscheduling removes its send task."}
		}`, b.ID), body)

		code, body = f.post(t, "/api/broadcasts.unschedule", b.ID)
		require.Equal(t, http.StatusOK, code, body)
		code, body = f.post(t, "/api/broadcasts.delete", b.ID)
		require.Equal(t, http.StatusOK, code, body)
		assert.Equal(t, 0, f.count(t, `SELECT count(*) FROM broadcasts WHERE id = $1`, b.ID))
	})

	// Cancelling leaves the rows being sent, so it would not empty this queue.
	t.Run("rows being sent ask to wait even when the broadcast can be cancelled", func(t *testing.T) {
		b := f.broadcast(t, domain.BroadcastStatusProcessed)
		f.queueRow(t, b.ID, "processing", "NULL")

		code, body := f.post(t, "/api/broadcasts.delete", b.ID)
		assert.Equal(t, http.StatusConflict, code)
		assert.JSONEq(t, fmt.Sprintf(`{
			"error": "cannot delete broadcast %[1]s: its status is processed. Its email queue still holds 1 email to send: wait for the queue to empty.",
			"invalid_state": {"entity":"broadcast","id":%[1]q,"action":"delete","status":"processed",
				"reason":"Its email queue still holds 1 email to send: wait for the queue to empty."}
		}`, b.ID), body)
	})

	t.Run("the rows given up on go with the broadcast", func(t *testing.T) {
		b := f.broadcast(t, domain.BroadcastStatusProcessed)
		f.queueRow(t, b.ID, "failed", "NULL")
		f.queueRow(t, b.ID, "failed", "NULL")

		code, body := f.post(t, "/api/broadcasts.delete", b.ID)
		require.Equal(t, http.StatusOK, code, body)
		assert.Equal(t, 0, f.count(t, `SELECT count(*) FROM broadcasts WHERE id = $1`, b.ID))
		assert.Equal(t, 0, f.count(t, `SELECT count(*) FROM email_queue WHERE source_id = $1`, b.ID),
			"no queue row outlives its broadcast")
	})
}

// A retry takes the broadcast's row lock, the one a delete holds, so a delete that
// commits while the retry waits leaves nothing requeued: the retry then finds no
// broadcast and answers 404.
func TestBroadcastRetry_WaitsForADeleteHoldingTheBroadcast(t *testing.T) {
	f := newDeleteQueueFixture(t)
	_, err := f.suite.DataFactory.SetupWorkspaceWithSMTPProvider(f.workspace.ID)
	require.NoError(t, err)

	b := f.broadcast(t, domain.BroadcastStatusProcessed)
	f.queueRow(t, b.ID, "failed", "NULL")

	// Take the lock the delete takes, and do what the delete does under it.
	tx, err := f.wsDB.Begin()
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	_, err = tx.Exec(`SELECT id FROM broadcasts WHERE id = $1 FOR UPDATE`, b.ID)
	require.NoError(t, err)

	type answer struct {
		code int
		body string
	}
	answered := make(chan answer, 1)
	go func() {
		resp, err := f.client.PostRaw("/api/broadcasts.retryFailed", fmt.Sprintf(`{"workspace_id":%q,"id":%q}`, f.workspace.ID, b.ID))
		if err != nil {
			answered <- answer{body: err.Error()}
			return
		}
		defer func() { _ = resp.Body.Close() }()
		body, _ := io.ReadAll(resp.Body)
		answered <- answer{code: resp.StatusCode, body: string(body)}
	}()

	select {
	case a := <-answered:
		t.Fatalf("the retry answered while the broadcast was locked: %d %s", a.code, a.body)
	case <-time.After(2 * time.Second):
	}

	_, err = tx.Exec(`DELETE FROM email_queue WHERE source_id = $1 AND status = 'failed' AND next_retry_at IS NULL`, b.ID)
	require.NoError(t, err)
	_, err = tx.Exec(`DELETE FROM broadcasts WHERE id = $1`, b.ID)
	require.NoError(t, err)
	require.NoError(t, tx.Commit())

	select {
	case a := <-answered:
		assert.Equal(t, http.StatusNotFound, a.code, a.body)
		assert.JSONEq(t, `{"error":"Broadcast not found"}`, a.body)
	case <-time.After(30 * time.Second):
		t.Fatal("the retry never answered")
	}
	assert.Equal(t, 0, f.count(t, `SELECT count(*) FROM email_queue WHERE source_id = $1`, b.ID))
}
