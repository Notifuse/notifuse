package integration

import (
	"context"
	"fmt"
	"io"
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

// TestAutomationExitOnReplySendsTheLastEmail runs an Exit on reply automation whose last
// step is an email. The executor completes the journey in the pass that queues that
// email, and the queue worker checks the journey again right before sending it: the email
// must still go out, since the contact did not reply.
func TestAutomationExitOnReplySendsTheLastEmail(t *testing.T) {
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

	list, err := factory.CreateList(workspace.ID)
	require.NoError(t, err)
	template, err := factory.CreateTemplate(workspace.ID, testutil.WithTemplateEmailContent(`Hello, last step.`))
	require.NoError(t, err)

	automationID := shortuuid.New()
	eventName := "exit_on_reply_last_" + uuid.New().String()[:8]
	triggerNodeID := shortuuid.New()
	emailNodeID := shortuuid.New()
	resp, err := client.CreateAutomation(map[string]interface{}{
		"workspace_id": workspace.ID,
		"automation": map[string]interface{}{
			"id":            automationID,
			"workspace_id":  workspace.ID,
			"name":          "Exit on reply, last step an email",
			"status":        "draft",
			"list_id":       list.ID,
			"exit_on_reply": true,
			"trigger": map[string]interface{}{
				"event_kind":        "custom_event",
				"custom_event_name": eventName,
				"frequency":         "every_time",
			},
			"root_node_id": triggerNodeID,
			"nodes": []map[string]interface{}{
				{
					"id": triggerNodeID, "automation_id": automationID, "type": "trigger",
					"config": map[string]interface{}{}, "next_node_id": emailNodeID,
					"position": map[string]interface{}{"x": 0, "y": 0},
				},
				{
					"id": emailNodeID, "automation_id": automationID, "type": "email",
					"config":   map[string]interface{}{"template_id": template.ID},
					"position": map[string]interface{}{"x": 0, "y": 100},
				},
			},
			"stats": map[string]interface{}{"enrolled": 0, "completed": 0, "exited": 0, "failed": 0},
		},
	})
	require.NoError(t, err)
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode, "create automation: %s", body)

	resp, err = client.ActivateAutomation(map[string]interface{}{"workspace_id": workspace.ID, "automation_id": automationID})
	require.NoError(t, err)
	body, _ = io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode, "activate automation: %s", body)

	workspaceDB, err := factory.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)
	var exitOnReply bool
	require.NoError(t, workspaceDB.QueryRowContext(ctx, `SELECT exit_on_reply FROM automations WHERE id = $1`, automationID).Scan(&exitOnReply))
	require.True(t, exitOnReply, "the automation must stop on reply for the worker to check the journey")

	email := fmt.Sprintf("exit-on-reply-last-%s@example.com", uuid.New().String()[:8])
	_, err = factory.CreateContact(workspace.ID, testutil.WithContactEmail(email))
	require.NoError(t, err)
	_, err = factory.CreateContactList(workspace.ID,
		testutil.WithContactListEmail(email),
		testutil.WithContactListListID(list.ID),
		testutil.WithContactListStatus(domain.ContactListStatusActive))
	require.NoError(t, err)
	require.NoError(t, factory.CreateCustomEvent(workspace.ID, email, eventName, map[string]interface{}{}))

	journey := waitForAutomationStatus(t, factory, workspace.ID, automationID, email, domain.ContactAutomationStatusCompleted, 20*time.Second)
	require.NotNil(t, journey)

	_, err = testutil.WaitForMailpitMessageByRecipient(t, email, 20*time.Second)
	assert.NoError(t, err, "the last email of a completed Exit on reply journey must be sent")
}
