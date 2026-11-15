//go:build integration

package integration

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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

// unscheduleFixture is a workspace with one subscribed contact, a template and the database
// connections the assertions read.
type unscheduleFixture struct {
	t           *testing.T
	client      *testutil.APIClient
	factory     *testutil.TestDataFactory
	systemDB    *sql.DB
	workspaceDB *sql.DB
	workspaceID string
	listID      string
	templateID  string
}

func newUnscheduleFixture(t *testing.T, suite *testutil.IntegrationTestSuite) *unscheduleFixture {
	t.Helper()
	client, factory := suite.APIClient, suite.DataFactory
	tag := uuid.New().String()[:8]

	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	require.NoError(t, factory.AddUserToWorkspace(user.ID, workspace.ID, "owner"))
	_, err = factory.SetupWorkspaceWithSMTPProvider(workspace.ID,
		testutil.WithIntegrationEmailProvider(domain.EmailProvider{
			Kind:    domain.EmailProviderKindSMTP,
			Senders: []domain.EmailSender{domain.NewEmailSender("noreply@notifuse.test", "Unschedule")},
			SMTP:    &domain.SMTPSettings{Host: "localhost", Port: 1025},
		}))
	require.NoError(t, err)
	require.NoError(t, client.Login(user.Email, "password"))
	client.SetWorkspaceID(workspace.ID)

	list, err := factory.CreateList(workspace.ID)
	require.NoError(t, err)
	contact, err := factory.CreateContact(workspace.ID, testutil.WithContactEmail(fmt.Sprintf("unschedule-%s@example.com", tag)))
	require.NoError(t, err)
	_, err = factory.CreateContactList(workspace.ID,
		testutil.WithContactListEmail(contact.Email),
		testutil.WithContactListListID(list.ID),
		testutil.WithContactListStatus(domain.ContactListStatusActive))
	require.NoError(t, err)
	template, err := factory.CreateTemplate(workspace.ID, testutil.WithTemplateSubject("Unschedule "+tag))
	require.NoError(t, err)

	workspaceDB, err := factory.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)
	return &unscheduleFixture{
		t: t, client: client, factory: factory,
		systemDB: suite.DBManager.GetDB(), workspaceDB: workspaceDB,
		workspaceID: workspace.ID, listID: list.ID, templateID: template.ID,
	}
}

// draft creates a draft of the fixture's list and template.
func (f *unscheduleFixture) draft() string {
	f.t.Helper()
	broadcast, err := f.factory.CreateBroadcast(f.workspaceID,
		testutil.WithBroadcastAudience(domain.AudienceSettings{List: f.listID, ExcludeUnsubscribed: true}))
	require.NoError(f.t, err)
	broadcast.TestSettings.Variations[0].TemplateID = f.templateID
	resp, err := f.client.UpdateBroadcast(map[string]interface{}{
		"workspace_id": f.workspaceID, "id": broadcast.ID, "name": broadcast.Name,
		"audience": broadcast.Audience, "schedule": broadcast.Schedule, "test_settings": broadcast.TestSettings,
	})
	require.NoError(f.t, err)
	require.Equal(f.t, http.StatusOK, resp.StatusCode)
	_ = resp.Body.Close()
	return broadcast.ID
}

// schedule books the broadcast for at, in UTC.
func (f *unscheduleFixture) schedule(id string, at time.Time) {
	f.t.Helper()
	resp, err := f.client.ScheduleBroadcast(map[string]interface{}{
		"workspace_id": f.workspaceID, "id": id, "send_now": false,
		"scheduled_date": at.UTC().Format("2006-01-02"), "scheduled_time": at.UTC().Format("15:04"), "timezone": "UTC",
	})
	require.NoError(f.t, err)
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	require.Equal(f.t, http.StatusOK, resp.StatusCode, string(body))
}

// unschedule calls broadcasts.unschedule and returns its status and error message.
func (f *unscheduleFixture) unschedule(id string) (int, string) {
	f.t.Helper()
	resp, err := f.client.UnscheduleBroadcast(map[string]interface{}{"workspace_id": f.workspaceID, "id": id})
	require.NoError(f.t, err)
	defer func() { _ = resp.Body.Close() }()
	var body map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	message, _ := body["error"].(string)
	return resp.StatusCode, message
}

// state reads the broadcast's status, booking and date, and its send tasks.
func (f *unscheduleFixture) state(id string) (status string, isScheduled bool, date string, tasks int) {
	f.t.Helper()
	require.NoError(f.t, f.workspaceDB.QueryRow(
		`SELECT status, COALESCE((schedule->>'is_scheduled')::boolean, false), COALESCE(schedule->>'scheduled_date', '')
		 FROM broadcasts WHERE id = $1`, id).Scan(&status, &isScheduled, &date))
	require.NoError(f.t, f.systemDB.QueryRow(
		`SELECT COUNT(*) FROM tasks WHERE workspace_id = $1 AND broadcast_id = $2 AND type = 'send_broadcast'`,
		f.workspaceID, id).Scan(&tasks))
	return status, isScheduled, date, tasks
}

// emails counts the broadcast's emails in the queue and in the message history.
func (f *unscheduleFixture) emails(id string) int {
	f.t.Helper()
	var count int
	require.NoError(f.t, f.workspaceDB.QueryRow(
		`SELECT (SELECT COUNT(*) FROM email_queue WHERE source_type = 'broadcast' AND source_id = $1)
		      + (SELECT COUNT(*) FROM message_history WHERE broadcast_id = $1)`, id).Scan(&count))
	return count
}

func TestBroadcastUnschedule(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()
	f := newUnscheduleFixture(t, suite)

	t.Run("a scheduled broadcast goes back to draft with its date, loses its task, and sends nothing", func(t *testing.T) {
		id := f.draft()
		tomorrow := time.Now().UTC().Add(24 * time.Hour)
		f.schedule(id, tomorrow)
		status, isScheduled, _, tasks := f.state(id)
		require.Equal(t, "scheduled", status)
		require.True(t, isScheduled)
		require.Equal(t, 1, tasks)

		code, message := f.unschedule(id)
		require.Equal(t, http.StatusOK, code, message)

		status, isScheduled, date, tasks := f.state(id)
		assert.Equal(t, "draft", status)
		assert.False(t, isScheduled)
		assert.Equal(t, tomorrow.Format("2006-01-02"), date, "the date stays for the next schedule")
		assert.Equal(t, 0, tasks)

		// Due tasks run: none is this broadcast's.
		_, _ = f.client.ExecutePendingTasks(10)
		time.Sleep(time.Second)
		assert.Equal(t, 0, f.emails(id))

		// A second call finds a draft with no task: it is not scheduled.
		code, message = f.unschedule(id)
		assert.Equal(t, http.StatusBadRequest, code)
		assert.Contains(t, message, "not scheduled")
	})

	t.Run("a broadcast sent back to draft can be scheduled again, and then sends", func(t *testing.T) {
		id := f.draft()
		f.schedule(id, time.Now().UTC().Add(24*time.Hour))
		code, message := f.unschedule(id)
		require.Equal(t, http.StatusOK, code, message)

		f.schedule(id, time.Now().UTC().Add(-2*time.Minute))
		_, _, _, tasks := f.state(id)
		require.Equal(t, 1, tasks, "a new task")

		require.Eventually(t, func() bool {
			_, _ = f.client.ExecutePendingTasks(10)
			return f.emails(id) > 0
		}, 30*time.Second, 500*time.Millisecond, "the rescheduled broadcast sends")

		// Its sending has started: it can only be cancelled now.
		code, message = f.unschedule(id)
		assert.Equal(t, http.StatusBadRequest, code)

		// Even with no start date and its status back to scheduled (a failed date write, a send
		// under way at the v44 upgrade), and no task left to tell, the emails queued or sent
		// refuse it.
		_, err := f.workspaceDB.Exec(`UPDATE broadcasts SET status = 'scheduled', started_at = NULL WHERE id = $1`, id)
		require.NoError(t, err)
		_, err = f.systemDB.Exec(`DELETE FROM tasks WHERE workspace_id = $1 AND broadcast_id = $2`, f.workspaceID, id)
		require.NoError(t, err)
		code, message = f.unschedule(id)
		assert.Equal(t, http.StatusBadRequest, code)
		assert.Contains(t, message, "cancel it instead")
	})

	t.Run("a draft left with its task by an earlier call loses the task on the next call", func(t *testing.T) {
		id := f.draft()
		f.schedule(id, time.Now().UTC().Add(24*time.Hour))
		// The draft committed and the task deletion failed.
		_, err := f.workspaceDB.Exec(`UPDATE broadcasts SET status = 'draft' WHERE id = $1`, id)
		require.NoError(t, err)

		code, message := f.unschedule(id)
		require.Equal(t, http.StatusOK, code, message)
		_, _, _, tasks := f.state(id)
		assert.Equal(t, 0, tasks)
	})

	t.Run("a cancelled broadcast is refused", func(t *testing.T) {
		id := f.draft()
		f.schedule(id, time.Now().UTC().Add(24*time.Hour))
		resp, err := f.client.CancelBroadcast(map[string]interface{}{"workspace_id": f.workspaceID, "id": id})
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		_ = resp.Body.Close()

		code, message := f.unschedule(id)
		assert.Equal(t, http.StatusBadRequest, code)
		assert.Contains(t, message, "cancelled")
	})

	t.Run("a save sent with the unschedule never leaves a scheduled broadcast without a task", func(t *testing.T) {
		for round := 0; round < 6; round++ {
			id := f.draft()
			f.schedule(id, time.Now().UTC().Add(24*time.Hour))

			var wg sync.WaitGroup
			start := make(chan struct{})
			unscheduleStatus := 0
			wg.Add(2)
			go func() {
				defer wg.Done()
				<-start
				resp, err := f.client.UnscheduleBroadcast(map[string]interface{}{"workspace_id": f.workspaceID, "id": id})
				if err == nil {
					unscheduleStatus = resp.StatusCode
					_ = resp.Body.Close()
				}
			}()
			go func() {
				defer wg.Done()
				<-start
				resp, err := f.client.UpdateBroadcast(map[string]interface{}{
					"workspace_id": f.workspaceID, "id": id, "name": fmt.Sprintf("Renamed %d", round),
				})
				if err == nil {
					_ = resp.Body.Close()
				}
			}()
			close(start)
			wg.Wait()

			require.Equal(t, http.StatusOK, unscheduleStatus, "round %d: the unschedule succeeds", round)
			status, isScheduled, _, tasks := f.state(id)
			assert.Equal(t, "draft", status, "round %d", round)
			assert.False(t, isScheduled, "round %d", round)
			assert.Equal(t, 0, tasks, "round %d", round)
		}
	})
}
