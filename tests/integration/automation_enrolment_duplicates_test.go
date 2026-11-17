package integration

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/app"
	"github.com/Notifuse/notifuse/internal/migrations"
	"github.com/Notifuse/notifuse/tests/testutil"
	"github.com/google/uuid"
	shortuuid "github.com/lithammer/shortuuid/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAutomationEnrolmentDuplicates checks, on a real database, when a timeline row starts a
// journey: a custom event starts one when it is recorded, not when it is updated, deleted or
// restored; two events of one name for one contact in one batch start two journeys instead of
// refusing the batch; and two enrolments of one contact into a 'once' automation at the same time
// start one journey. Journeys are read from contact_automations: no worker runs, so they stay
// where the trigger put them.
func TestAutomationEnrolmentDuplicates(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	client := suite.APIClient
	factory := suite.DataFactory
	ctx := context.Background()

	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	require.NoError(t, factory.AddUserToWorkspace(user.ID, workspace.ID, "owner"))
	require.NoError(t, client.Login(user.Email, "password"))
	client.SetWorkspaceID(workspace.ID)

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

	// liveAutomation creates and activates an automation on the custom event eventName.
	liveAutomation := func(t *testing.T, eventName, frequency string, conditions map[string]interface{}) string {
		t.Helper()
		automationID, triggerNodeID, emailNodeID := shortuuid.New(), shortuuid.New(), shortuuid.New()
		trigger := map[string]interface{}{
			"event_kind":        "custom_event",
			"custom_event_name": eventName,
			"frequency":         frequency,
		}
		if conditions != nil {
			trigger["conditions"] = conditions
		}
		resp, err := client.CreateAutomation(map[string]interface{}{
			"workspace_id": workspace.ID,
			"automation": map[string]interface{}{
				"id":           automationID,
				"workspace_id": workspace.ID,
				"name":         "Enrolment " + eventName,
				"status":       "draft",
				"list_id":      list.ID,
				"trigger":      trigger,
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
	newEmail := func(prefix string) string { return fmt.Sprintf("%s-%s@example.com", prefix, uuid.New().String()[:8]) }

	// upsert posts one event through the API, as an application does.
	upsert := func(t *testing.T, fields map[string]interface{}) {
		t.Helper()
		fields["workspace_id"] = workspace.ID
		resp, err := client.Post("/api/customEvents.upsert", fields)
		require.NoError(t, err)
		body := readBody(resp)
		require.Less(t, resp.StatusCode, 300, "customEvents.upsert: %s", body)
	}

	journeys := func(t *testing.T, automationID, email string) int {
		t.Helper()
		var n int
		require.NoError(t, workspaceDB.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM contact_automations WHERE automation_id = $1 AND contact_email = $2`,
			automationID, email).Scan(&n))
		return n
	}

	timelineRows := func(t *testing.T, email, eventName, operation string) int {
		t.Helper()
		var n int
		require.NoError(t, workspaceDB.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM contact_timeline WHERE email = $1 AND kind = $2 AND operation = $3`,
			email, "custom_event."+eventName, operation).Scan(&n))
		return n
	}

	t.Run("an every_time event starts one journey, and its updates, deletion and restore none", func(t *testing.T) {
		eventName := newEventName("order_placed")
		automationID := liveAutomation(t, eventName, "every_time", nil)
		email := newEmail("order")
		externalID := "order-" + uuid.New().String()[:8]
		recorded := time.Now().UTC().Add(-2 * time.Hour).Truncate(time.Second)

		upsert(t, map[string]interface{}{"email": email, "event_name": eventName, "external_id": externalID,
			"occurred_at": recorded.Format(time.RFC3339), "properties": map[string]interface{}{"total": 10}})
		require.Equal(t, 1, journeys(t, automationID, email), "recording the event starts the journey")

		// A later occurred_at updates the stored event.
		upsert(t, map[string]interface{}{"email": email, "event_name": eventName, "external_id": externalID,
			"occurred_at": recorded.Add(time.Hour).Format(time.RFC3339), "properties": map[string]interface{}{"total": 12}})
		// A soft delete, then a restore.
		upsert(t, map[string]interface{}{"email": email, "event_name": eventName, "external_id": externalID,
			"deleted_at": time.Now().UTC().Format(time.RFC3339)})
		upsert(t, map[string]interface{}{"email": email, "event_name": eventName, "external_id": externalID,
			"occurred_at": recorded.Add(time.Hour).Format(time.RFC3339)})

		assert.Equal(t, 3, timelineRows(t, email, eventName, "update"),
			"the update, the deletion and the restore are still on the contact's timeline")
		assert.Equal(t, 1, journeys(t, automationID, email), "none of them starts the journey again")

		// Another occurrence is another event.
		upsert(t, map[string]interface{}{"email": email, "event_name": eventName, "external_id": "order-" + uuid.New().String()[:8]})
		assert.Equal(t, 2, journeys(t, automationID, email), "a new external_id starts a new journey")
	})

	t.Run("a batch with two events of one name for one contact starts two journeys", func(t *testing.T) {
		eventName := newEventName("purchase")
		automationID := liveAutomation(t, eventName, "every_time", nil)
		email := newEmail("buyer")

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

		assert.Equal(t, 2, journeys(t, automationID, email))
		var events int
		require.NoError(t, workspaceDB.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM custom_events WHERE email = $1 AND event_name = $2`, email, eventName).Scan(&events))
		assert.Equal(t, 2, events, "the batch was stored")
	})

	t.Run("two enrolments of one contact into a once automation at the same time start one journey", func(t *testing.T) {
		eventName := newEventName("signup")
		automationID := liveAutomation(t, eventName, "once", nil)
		email := newEmail("signup")
		externalID := "signup-" + uuid.New().String()[:8]

		// The timeline row the custom event trigger writes, inserted directly: the race is in
		// the enrolment it fires.
		insertRow := func(tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx, `INSERT INTO contact_timeline (email, operation, entity_type, kind, entity_id, changes, created_at)
				VALUES ($1, 'insert', 'custom_event', $2, $3, '{}'::jsonb, NOW())`, email, "custom_event."+eventName, externalID)
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

		// The second enrolment must be blocked by the first transaction before it commits;
		// otherwise its existence check alone would keep it out. It blocks on the first one's
		// automation_trigger_log row: the automation_trigger_* triggers fire before
		// contact_timeline_queue_trigger (name order), so the trigger log is the first row the
		// two transactions share.
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

		assert.Equal(t, 1, journeys(t, automationID, email))
		var logRows, enrolled int
		require.NoError(t, workspaceDB.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM automation_trigger_log WHERE automation_id = $1 AND contact_email = $2`,
			automationID, email).Scan(&logRows))
		assert.Equal(t, 1, logRows)
		require.NoError(t, workspaceDB.QueryRowContext(ctx,
			`SELECT COALESCE((stats->>'enrolled')::int, 0) FROM automations WHERE id = $1`, automationID).Scan(&enrolled))
		assert.Equal(t, 1, enrolled)
	})

	t.Run("an automation with a condition starts on the event and not on its update", func(t *testing.T) {
		eventName := newEventName("booking")
		automationID := liveAutomation(t, eventName, "every_time", map[string]interface{}{
			"kind": "leaf",
			"leaf": map[string]interface{}{
				"source": "contacts",
				"contact": map[string]interface{}{"filters": []map[string]interface{}{{
					"field_name": "country", "field_type": "string", "operator": "equals", "string_values": []string{"FR"},
				}}},
			},
		})
		email, elsewhere := newEmail("booking"), newEmail("booking")
		_, err := workspaceDB.ExecContext(ctx, `INSERT INTO contacts (email, country, created_at, updated_at) VALUES ($1, 'FR', NOW(), NOW()), ($2, 'US', NOW(), NOW())`, email, elsewhere)
		require.NoError(t, err)
		externalID := "b-" + uuid.New().String()[:8]
		recorded := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)

		upsert(t, map[string]interface{}{"email": email, "event_name": eventName, "external_id": externalID,
			"occurred_at": recorded.Format(time.RFC3339)})
		require.Equal(t, 1, journeys(t, automationID, email))
		upsert(t, map[string]interface{}{"email": email, "event_name": eventName, "external_id": externalID,
			"occurred_at": recorded.Add(time.Minute).Format(time.RFC3339)})
		assert.Equal(t, 1, journeys(t, automationID, email))

		// The condition still applies to a new event.
		upsert(t, map[string]interface{}{"email": elsewhere, "event_name": eventName, "external_id": "b-" + uuid.New().String()[:8]})
		assert.Equal(t, 0, journeys(t, automationID, elsewhere), "a contact outside the condition is not enrolled")
	})

	// A workspace upgraded to 44 has triggers whose bodies were installed before it: they enrol on
	// every row of the kind until v44 replaces them.
	t.Run("v44 stops a trigger installed before it from starting on an update", func(t *testing.T) {
		eventName := newEventName("renewal")
		automationID := liveAutomation(t, eventName, "every_time", nil)
		function := strings.ToLower("automation_trigger_" + strings.ReplaceAll(automationID, "-", ""))

		// The body every trigger had before 44.0, written by hand rather than generated.
		_, err := workspaceDB.ExecContext(ctx, fmt.Sprintf(`CREATE OR REPLACE FUNCTION %s() RETURNS TRIGGER AS $$
BEGIN
    PERFORM automation_enroll_contact('%s', NEW.email, (SELECT root_node_id FROM automations WHERE id = '%s'), 'every_time');
    RETURN NEW;
END;
$$ LANGUAGE plpgsql`, function, automationID, automationID))
		require.NoError(t, err)

		email := newEmail("renewal")
		externalID := "r-" + uuid.New().String()[:8]
		recorded := time.Now().UTC().Add(-3 * time.Hour).Truncate(time.Second)
		upsert(t, map[string]interface{}{"email": email, "event_name": eventName, "external_id": externalID,
			"occurred_at": recorded.Format(time.RFC3339)})
		upsert(t, map[string]interface{}{"email": email, "event_name": eventName, "external_id": externalID,
			"occurred_at": recorded.Add(time.Hour).Format(time.RFC3339)})
		require.Equal(t, 2, journeys(t, automationID, email), "a body from before 44 enrols on the update")

		tx, err := workspaceDB.BeginTx(ctx, nil)
		require.NoError(t, err)
		require.NoError(t, (&migrations.V44Migration{}).UpdateWorkspace(ctx, &config.Config{}, workspace, tx))
		require.NoError(t, tx.Commit())

		upsert(t, map[string]interface{}{"email": email, "event_name": eventName, "external_id": externalID,
			"occurred_at": recorded.Add(2 * time.Hour).Format(time.RFC3339)})
		assert.Equal(t, 2, journeys(t, automationID, email), "after v44 the update starts nothing")
	})
}
