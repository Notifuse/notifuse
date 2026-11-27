//go:build integration
// +build integration

package integration

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lithammer/shortuuid/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/tests/testutil"
)

// TestRecreatedContactKeepsOptOutForContactCreatedAutomations: when a deleted contact comes
// back, their kept opt-outs are put back before the contact.created entry enrolls them in
// automations, so an automation that leaves out the unsubscribed of a list leaves them out.
// PostgreSQL fires the AFTER INSERT triggers of contacts in the alphabetical order of their
// names; with the restore trigger named after contact_changes_trigger, the automation's
// condition read contact_lists before the opt-out was back and enrolled the contact.
func TestRecreatedContactKeepsOptOutForContactCreatedAutomations(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer func() { suite.Cleanup() }()

	factory := suite.DataFactory
	client := suite.APIClient

	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	require.NoError(t, factory.AddUserToWorkspace(user.ID, workspace.ID, "owner"))
	require.NoError(t, client.Login(user.Email, "password"))
	client.SetWorkspaceID(workspace.ID)
	db, err := factory.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)

	list, err := factory.CreateList(workspace.ID)
	require.NoError(t, err)
	template, err := factory.CreateTemplate(workspace.ID)
	require.NoError(t, err)

	post := func(path string, body map[string]interface{}) {
		t.Helper()
		resp, err := client.Post(path, body)
		require.NoError(t, err)
		text, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		require.Equalf(t, http.StatusOK, resp.StatusCode, "%s: %s", path, text)
	}

	// A contact unsubscribed from the list, deleted: their opt-out is kept as a hash.
	returning := fmt.Sprintf("returning-%s@example.com", strings.ToLower(uuid.New().String()[:8]))
	_, err = factory.CreateContact(workspace.ID, testutil.WithContactEmail(returning))
	require.NoError(t, err)
	_, err = factory.CreateContactList(workspace.ID, testutil.WithContactListEmail(returning),
		testutil.WithContactListListID(list.ID), testutil.WithContactListStatus(domain.ContactListStatusUnsubscribed))
	require.NoError(t, err)
	post("/api/contacts.delete", map[string]interface{}{"workspace_id": workspace.ID, "email": returning})
	var hashes int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM contact_opt_out_hashes`).Scan(&hashes))
	require.Equal(t, 1, hashes, "the deletion keeps the opt-out")

	// An automation started by contact.created for everyone not unsubscribed from the list.
	automationID := shortuuid.New()
	triggerNodeID := shortuuid.New()
	emailNodeID := shortuuid.New()
	resp, err := client.CreateAutomation(map[string]interface{}{
		"workspace_id": workspace.ID,
		"automation": map[string]interface{}{
			"id":           automationID,
			"workspace_id": workspace.ID,
			"name":         "Welcome, unless unsubscribed",
			"status":       "draft",
			"list_id":      list.ID,
			"trigger": map[string]interface{}{
				"event_kind": "contact.created",
				"frequency":  "once",
				"conditions": map[string]interface{}{
					"kind": "leaf",
					"leaf": map[string]interface{}{
						"source": "contact_lists",
						"contact_list": map[string]interface{}{
							"operator": "not_in",
							"list_id":  list.ID,
							"status":   string(domain.ContactListStatusUnsubscribed),
						},
					},
				},
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
	text, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	require.Equalf(t, http.StatusCreated, resp.StatusCode, "automations.create: %s", text)
	resp, err = client.ActivateAutomation(map[string]interface{}{"workspace_id": workspace.ID, "automation_id": automationID})
	require.NoError(t, err)
	text, _ = io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	require.Equalf(t, http.StatusOK, resp.StatusCode, "automations.activate: %s", text)

	// A new contact enters: the automation fires on contact.created.
	newcomer := fmt.Sprintf("newcomer-%s@example.com", strings.ToLower(uuid.New().String()[:8]))
	post("/api/contacts.upsert", map[string]interface{}{"workspace_id": workspace.ID, "contact": map[string]interface{}{"email": newcomer}})
	require.NotNil(t, waitForEnrollment(t, factory, workspace.ID, automationID, newcomer, 5*time.Second),
		"a contact with no opt-out is enrolled")

	// The deleted contact comes back: the opt-out is restored, and the automation skips them.
	post("/api/contacts.upsert", map[string]interface{}{"workspace_id": workspace.ID, "contact": map[string]interface{}{"email": returning}})
	var status string
	require.NoError(t, db.QueryRow(`SELECT status FROM contact_lists WHERE email = $1 AND list_id = $2`, returning, list.ID).Scan(&status))
	require.Equal(t, string(domain.ContactListStatusUnsubscribed), status, "the opt-out is back")
	// The enrollment is written inside the insert that fired the trigger: once the upsert has
	// answered, the answer is final.
	enrolled, err := factory.FindContactAutomation(workspace.ID, automationID, returning)
	require.NoError(t, err)
	assert.Nil(t, enrolled, "a returning contact unsubscribed from the list is not enrolled")
}
