package integration

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
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

// TestAutomationListStepRetry runs a journey through the real scheduler: trigger → add_to_list →
// remove_from_list. A failed list write must keep the journey on its step and retry it, not move
// on as if the contact had been added; removing a contact that was never in the list is not a
// failure.
func TestAutomationListStepRetry(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	ctx := context.Background()
	require.NoError(t, suite.ServerManager.StartBackgroundWorkers(ctx))

	factory := suite.DataFactory
	client := suite.APIClient

	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	require.NoError(t, factory.AddUserToWorkspace(user.ID, workspace.ID, "owner"))
	require.NoError(t, client.Login(user.Email, "password"))
	client.SetWorkspaceID(workspace.ID)
	db, err := suite.DBManager.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)

	addList, err := factory.CreateList(workspace.ID)
	require.NoError(t, err)
	otherList, err := factory.CreateList(workspace.ID)
	require.NoError(t, err)

	automationID := shortuuid.New()
	triggerNodeID := shortuuid.New()
	addNodeID := shortuuid.New()
	removeNodeID := shortuuid.New()
	eventName := "list_step_retry_" + uuid.New().String()[:8]

	resp, err := client.CreateAutomation(map[string]interface{}{
		"workspace_id": workspace.ID,
		"automation": map[string]interface{}{
			"id":           automationID,
			"workspace_id": workspace.ID,
			"name":         "List step retry",
			"status":       "draft",
			"trigger": map[string]interface{}{
				"event_kind": "custom_event", "custom_event_name": eventName,
				"frequency": "once",
			},
			"root_node_id": triggerNodeID,
			"nodes": []map[string]interface{}{
				{
					"id": triggerNodeID, "automation_id": automationID, "type": "trigger",
					"config": map[string]interface{}{}, "next_node_id": addNodeID,
					"position": map[string]interface{}{"x": 0, "y": 0},
				},
				{
					"id": addNodeID, "automation_id": automationID, "type": "add_to_list",
					"config":       map[string]interface{}{"list_id": addList.ID, "status": "active"},
					"next_node_id": removeNodeID,
					"position":     map[string]interface{}{"x": 0, "y": 100},
				},
				{
					"id": removeNodeID, "automation_id": automationID, "type": "remove_from_list",
					"config":   map[string]interface{}{"list_id": otherList.ID},
					"position": map[string]interface{}{"x": 0, "y": 200},
				},
			},
			"stats": map[string]interface{}{"enrolled": 0, "completed": 0, "exited": 0, "failed": 0},
		},
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	_ = resp.Body.Close()

	resp, err = client.ActivateAutomation(map[string]interface{}{
		"workspace_id":  workspace.ID,
		"automation_id": automationID,
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	_ = resp.Body.Close()

	email := fmt.Sprintf("list-retry-%s@example.com", uuid.New().String()[:8])
	_, err = factory.CreateContact(workspace.ID, testutil.WithContactEmail(email))
	require.NoError(t, err)

	dropTrigger := failInsertsFor(t, db, "contact_lists", email)
	require.NoError(t, factory.CreateCustomEvent(workspace.ID, email, eventName, nil))

	// The add fails: the journey stays on the step, with a retry scheduled.
	var ca *domain.ContactAutomation
	testutil.WaitForCondition(t, func() bool {
		ca, err = factory.GetContactAutomation(workspace.ID, automationID, email)
		return err == nil && ca != nil && ca.RetryCount >= 1
	}, 15*time.Second, "waiting for the failed add_to_list step to be scheduled for retry")
	assert.Equal(t, domain.ContactAutomationStatusActive, ca.Status)
	assert.Equal(t, 1, ca.RetryCount)
	require.NotNil(t, ca.CurrentNodeID)
	assert.Equal(t, addNodeID, *ca.CurrentNodeID, "the journey does not move past a list write that failed")
	require.NotNil(t, ca.ScheduledAt)
	assert.True(t, ca.ScheduledAt.After(time.Now()), "the retry waits for its backoff")
	assert.Equal(t, 0, listRowCount(t, db, email, addList.ID))

	// The write works again; bring the retry forward instead of waiting for its backoff.
	dropTrigger()
	_, err = db.ExecContext(ctx, `UPDATE contact_automations SET scheduled_at = NOW() WHERE id = $1`, ca.ID)
	require.NoError(t, err)

	completed := waitForAutomationComplete(t, factory, workspace.ID, automationID, email, 15*time.Second)
	require.NotNil(t, completed)
	assert.Equal(t, domain.ContactAutomationStatusCompleted, completed.Status,
		"removing a contact that was never in the list does not fail the journey")
	assert.Equal(t, 1, listRowCount(t, db, email, addList.ID), "the retried step added the contact")
	assert.Equal(t, 0, listRowCount(t, db, email, otherList.ID))
}

// listRowCount counts the contact_lists rows of a contact still in a list.
func listRowCount(t *testing.T, db *sql.DB, email, listID string) int {
	t.Helper()
	var n int
	require.NoError(t, db.QueryRowContext(context.Background(),
		`SELECT count(*) FROM contact_lists WHERE email = $1 AND list_id = $2 AND deleted_at IS NULL`,
		email, listID).Scan(&n))
	return n
}
