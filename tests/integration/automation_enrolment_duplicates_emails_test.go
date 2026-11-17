package integration

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/app"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/tests/testutil"
	"github.com/google/uuid"
	shortuuid "github.com/lithammer/shortuuid/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAutomationEnrolmentDuplicatesEmails runs the custom event update, two-event import and
// forced 'once' race cases of TestAutomationEnrolmentDuplicates to the end: the automation
// scheduler and the email queue worker run, the workspace sends through Mailpit, and each case
// counts the emails a contact received rather than the journeys started.
func TestAutomationEnrolmentDuplicatesEmails(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, suite.ServerManager.StartBackgroundWorkers(ctx))

	client := suite.APIClient
	factory := suite.DataFactory

	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	require.NoError(t, factory.AddUserToWorkspace(user.ID, workspace.ID, "owner"))
	_, err = factory.SetupWorkspaceWithSMTPProvider(workspace.ID)
	require.NoError(t, err)
	require.NoError(t, client.Login(user.Email, "password"))
	client.SetWorkspaceID(workspace.ID)
	require.NoError(t, factory.SetWorkspaceCustomEndpointURL(workspace.ID, suite.ServerManager.GetURL()))

	list, err := factory.CreateList(workspace.ID)
	require.NoError(t, err)
	template, err := factory.CreateTemplate(workspace.ID)
	require.NoError(t, err)
	workspaceDB, err := factory.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)

	readBody := func(resp *http.Response) string {
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		return string(body)
	}

	// liveAutomation creates and activates an automation that sends one email on the custom
	// event eventName.
	liveAutomation := func(t *testing.T, eventName, frequency string) string {
		t.Helper()
		automationID, triggerNodeID, emailNodeID := shortuuid.New(), shortuuid.New(), shortuuid.New()
		resp, err := client.CreateAutomation(map[string]interface{}{
			"workspace_id": workspace.ID,
			"automation": map[string]interface{}{
				"id":           automationID,
				"workspace_id": workspace.ID,
				"name":         "Enrolment emails " + eventName,
				"status":       "draft",
				"list_id":      list.ID,
				"trigger": map[string]interface{}{
					"event_kind":        "custom_event",
					"custom_event_name": eventName,
					"frequency":         frequency,
				},
				"root_node_id": triggerNodeID,
				"nodes": []map[string]interface{}{
					{"id": triggerNodeID, "automation_id": automationID, "type": "trigger", "config": map[string]interface{}{},
						"next_node_id": emailNodeID, "position": map[string]interface{}{"x": 0, "y": 0}},
					{"id": emailNodeID, "automation_id": automationID, "type": "email",
						"config": map[string]interface{}{"template_id": template.ID}, "position": map[string]interface{}{"x": 0, "y": 100}},
				},
				"stats": map[string]interface{}{"enrolled": 0, "completed": 0, "exited": 0, "failed": 0},
			},
		})
		require.NoError(t, err)
		body := readBody(resp)
		require.Equal(t, http.StatusCreated, resp.StatusCode, "create automation: %s", body)
		resp, err = client.ActivateAutomation(map[string]interface{}{"workspace_id": workspace.ID, "automation_id": automationID})
		require.NoError(t, err)
		body = readBody(resp)
		require.Equal(t, http.StatusOK, resp.StatusCode, "activate automation: %s", body)
		return automationID
	}

	newEventName := func(prefix string) string { return prefix + "_" + uuid.New().String()[:8] }

	// member creates a contact on the automation's list.
	member := func(t *testing.T, prefix string) string {
		t.Helper()
		email := fmt.Sprintf("%s-%s@example.com", prefix, uuid.New().String()[:8])
		_, err := factory.CreateContact(workspace.ID, testutil.WithContactEmail(email))
		require.NoError(t, err)
		_, err = factory.CreateContactList(workspace.ID,
			testutil.WithContactListEmail(email),
			testutil.WithContactListListID(list.ID),
			testutil.WithContactListStatus(domain.ContactListStatusActive))
		require.NoError(t, err)
		return email
	}

	upsert := func(t *testing.T, fields map[string]interface{}) {
		t.Helper()
		fields["workspace_id"] = workspace.ID
		resp, err := client.Post("/api/customEvents.upsert", fields)
		require.NoError(t, err)
		body := readBody(resp)
		require.Less(t, resp.StatusCode, 300, "customEvents.upsert: %s", body)
	}

	mailpit := &http.Client{Timeout: 5 * time.Second}

	// received counts the emails Mailpit holds for one recipient.
	received := func(email string) (int, error) {
		resp, err := mailpit.Get("http://localhost:8025/api/v1/messages?limit=10000")
		if err != nil {
			return 0, err
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode != http.StatusOK {
			return 0, fmt.Errorf("mailpit answered %d", resp.StatusCode)
		}
		var listing testutil.MailpitMessagesResponse
		if err := json.NewDecoder(resp.Body).Decode(&listing); err != nil {
			return 0, err
		}
		n := 0
		for _, msg := range listing.Messages {
			for _, to := range msg.To {
				if strings.EqualFold(to.Address, email) {
					n++
				}
			}
		}
		return n, nil
	}

	// pending counts what is still on its way to email: active journeys and queued emails. Every
	// journey, a duplicate included, exists once the write that enrolled it has returned.
	pending := func(email string) (int, error) {
		var n int
		err := workspaceDB.QueryRowContext(ctx, `SELECT
			(SELECT COUNT(*) FROM contact_automations WHERE contact_email = $1 AND status = 'active') +
			(SELECT COUNT(*) FROM email_queue WHERE contact_email = $1)`, email).Scan(&n)
		return n, err
	}

	// settled waits until email has received at least want emails and nothing is left to send to
	// it, and returns what it received: a duplicate journey has sent its email by then.
	settled := func(t *testing.T, email string, want int) int {
		t.Helper()
		require.Eventually(t, func() bool {
			left, err := pending(email)
			if err != nil || left > 0 {
				return false
			}
			n, err := received(email)
			return err == nil && n >= want
		}, 30*time.Second, 250*time.Millisecond, "%s should receive %d emails and have none left to send", email, want)
		// Mailpit lists a message once it has stored it, which can trail the send by a moment.
		time.Sleep(time.Second)
		n, err := received(email)
		require.NoError(t, err)
		return n
	}

	t.Run("an event sends one email, and its update, deletion and restore none", func(t *testing.T) {
		eventName := newEventName("order_placed")
		liveAutomation(t, eventName, "every_time")
		email := member(t, "order")
		externalID := "order-" + uuid.New().String()[:8]
		recorded := time.Now().UTC().Add(-2 * time.Hour).Truncate(time.Second)

		upsert(t, map[string]interface{}{"email": email, "event_name": eventName, "external_id": externalID,
			"occurred_at": recorded.Format(time.RFC3339), "properties": map[string]interface{}{"total": 10}})
		require.Equal(t, 1, settled(t, email, 1), "recording the event sends the email")

		upsert(t, map[string]interface{}{"email": email, "event_name": eventName, "external_id": externalID,
			"occurred_at": recorded.Add(time.Hour).Format(time.RFC3339), "properties": map[string]interface{}{"total": 12}})
		upsert(t, map[string]interface{}{"email": email, "event_name": eventName, "external_id": externalID,
			"deleted_at": time.Now().UTC().Format(time.RFC3339)})
		upsert(t, map[string]interface{}{"email": email, "event_name": eventName, "external_id": externalID,
			"occurred_at": recorded.Add(time.Hour).Format(time.RFC3339)})
		assert.Equal(t, 1, settled(t, email, 1), "the update, the deletion and the restore send nothing")

		upsert(t, map[string]interface{}{"email": email, "event_name": eventName, "external_id": "order-" + uuid.New().String()[:8]})
		assert.Equal(t, 2, settled(t, email, 2), "a new external_id sends the email again")
	})

	t.Run("an import with two events of one name for one contact sends two emails", func(t *testing.T) {
		eventName := newEventName("purchase")
		liveAutomation(t, eventName, "every_time")
		email := member(t, "buyer")

		resp, err := client.Post("/api/customEvents.import", map[string]interface{}{
			"workspace_id": workspace.ID,
			"events": []map[string]interface{}{
				{"email": email, "event_name": eventName, "external_id": "p-" + uuid.New().String()[:8]},
				{"email": email, "event_name": eventName, "external_id": "p-" + uuid.New().String()[:8]},
			},
		})
		require.NoError(t, err)
		body := readBody(resp)
		require.Less(t, resp.StatusCode, 300, "customEvents.import: %s", body)

		assert.Equal(t, 2, settled(t, email, 2))
	})

	// The race forced: the second enrolment is seen waiting for the first before the first commits.
	t.Run("two enrolments of one contact into a once automation at the same time send one email", func(t *testing.T) {
		eventName := newEventName("signup")
		liveAutomation(t, eventName, "once")
		email := member(t, "signup")

		insertRow := func(tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx, `INSERT INTO contact_timeline (email, operation, entity_type, kind, entity_id, changes, created_at)
				VALUES ($1, 'insert', 'custom_event', $2, $3, '{}'::jsonb, NOW())`, email, "custom_event."+eventName, "s-"+uuid.New().String()[:8])
			return err
		}

		first, err := workspaceDB.BeginTx(ctx, nil)
		require.NoError(t, err)
		defer func() { _ = first.Rollback() }()
		var firstPID int
		require.NoError(t, first.QueryRowContext(ctx, `SELECT pg_backend_pid()`).Scan(&firstPID))
		require.NoError(t, insertRow(first))

		second := make(chan error, 1)
		go func() {
			tx, err := workspaceDB.BeginTx(ctx, nil)
			if err != nil {
				second <- err
				return
			}
			if err := insertRow(tx); err != nil {
				_ = tx.Rollback()
				second <- err
				return
			}
			second <- tx.Commit()
		}()

		require.Eventually(t, func() bool {
			var blocked int
			err := workspaceDB.QueryRowContext(ctx,
				`SELECT COUNT(*) FROM pg_stat_activity WHERE $1 = ANY(pg_blocking_pids(pid))`, firstPID).Scan(&blocked)
			return err == nil && blocked > 0
		}, 10*time.Second, 20*time.Millisecond, "the second enrolment never waited for the first")

		require.NoError(t, first.Commit())
		select {
		case err := <-second:
			require.NoError(t, err)
		case <-time.After(10 * time.Second):
			t.Fatal("the second enrolment did not finish")
		}

		assert.Equal(t, 1, settled(t, email, 1))
	})
}
