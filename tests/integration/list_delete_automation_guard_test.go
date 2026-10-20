//go:build integration
// +build integration

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
	pgdb "github.com/Notifuse/notifuse/pkg/database"
	"github.com/Notifuse/notifuse/tests/testutil"
	"github.com/google/uuid"
	"github.com/lib/pq"
	shortuuid "github.com/lithammer/shortuuid/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestListDeletionGuardedByLiveAutomations deletes lists through the API while automations
// use them, then runs the journeys.
//
// lists.delete used to have no in-use check, and lists.get hides a deleted list, so the
// email step of an automation whose list was deleted failed every contact that reached it
// with "failed to get list": the ones already waiting in the journey and every new one,
// once their retries ran out. Activation never looked at the list either, so a paused or
// draft automation whose list was deleted failed its contacts the same way once activated.
func TestListDeletionGuardedByLiveAutomations(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	// The journeys run on the automation scheduler, which the harness only starts on request.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, suite.ServerManager.StartBackgroundWorkers(ctx))

	client := suite.APIClient
	factory := suite.DataFactory
	bg := context.Background()

	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	workspaceID := workspace.ID
	require.NoError(t, factory.AddUserToWorkspace(user.ID, workspaceID, "owner"))
	_, err = factory.SetupWorkspaceWithSMTPProvider(workspaceID)
	require.NoError(t, err)
	require.NoError(t, client.Login(user.Email, "password"))
	client.SetWorkspaceID(workspaceID)

	// A member who may delete lists, and has no access to automations.
	listsMember, err := factory.CreateUser()
	require.NoError(t, err)
	require.NoError(t, factory.AddUserToWorkspaceWithPermissions(listsMember.ID, workspaceID, "member",
		domain.UserPermissions{domain.PermissionResourceLists: {Read: true, Write: true}}))
	listsOnly := testutil.NewAPIClient(suite.ServerManager.GetURL())
	require.NoError(t, listsOnly.Login(listsMember.Email, "password"))
	listsOnly.SetWorkspaceID(workspaceID)

	db, err := factory.GetWorkspaceDB(workspaceID)
	require.NoError(t, err)

	template, err := factory.CreateTemplate(workspaceID)
	require.NoError(t, err)

	// --- helpers -----------------------------------------------------------------------

	type apiResult struct {
		status int
		body   map[string]interface{}
		raw    string
	}

	send := func(t *testing.T, resp *http.Response, err error) apiResult {
		t.Helper()
		require.NoError(t, err)
		raw, readErr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		require.NoError(t, readErr)
		result := apiResult{status: resp.StatusCode, raw: string(raw)}
		_ = json.Unmarshal(raw, &result.body)
		return result
	}

	errorMessage := func(result apiResult) string {
		message, _ := result.body["error"].(string)
		return message
	}

	// named reads the {id, name} entries a refusal lists alongside its message.
	named := func(t *testing.T, result apiResult, key string) map[string]string {
		t.Helper()
		entries, ok := result.body[key].([]interface{})
		require.Truef(t, ok, "the refusal should list its %s: %s", key, result.raw)
		names := map[string]string{}
		for _, entry := range entries {
			fields, ok := entry.(map[string]interface{})
			require.True(t, ok)
			id, _ := fields["id"].(string)
			name, _ := fields["name"].(string)
			names[id] = name
		}
		return names
	}

	createList := func(t *testing.T, name string) string {
		t.Helper()
		listID := "guard" + strings.ReplaceAll(uuid.New().String(), "-", "")[:20]
		resp, err := client.CreateList(map[string]interface{}{
			"workspace_id": workspaceID,
			"id":           listID,
			"name":         name,
		})
		result := send(t, resp, err)
		require.Equalf(t, http.StatusCreated, result.status, "lists.create: %s", result.raw)
		return listID
	}

	deleteListAs := func(t *testing.T, caller *testutil.APIClient, listID string) apiResult {
		t.Helper()
		resp, err := caller.Post("/api/lists.delete", map[string]interface{}{
			"workspace_id": workspaceID,
			"id":           listID,
		})
		return send(t, resp, err)
	}

	deleteList := func(t *testing.T, listID string) apiResult {
		t.Helper()
		return deleteListAs(t, client, listID)
	}

	listStatus := func(t *testing.T, listID string) int {
		t.Helper()
		resp, err := client.GetList(listID)
		return send(t, resp, err).status
	}

	activate := func(t *testing.T, automationID string) apiResult {
		t.Helper()
		resp, err := client.ActivateAutomation(map[string]interface{}{
			"workspace_id":  workspaceID,
			"automation_id": automationID,
		})
		return send(t, resp, err)
	}

	pause := func(t *testing.T, automationID string) {
		t.Helper()
		resp, err := client.PauseAutomation(map[string]interface{}{
			"workspace_id":  workspaceID,
			"automation_id": automationID,
		})
		result := send(t, resp, err)
		require.Equalf(t, http.StatusOK, result.status, "automations.pause: %s", result.raw)
	}

	update := func(t *testing.T, automation map[string]interface{}) apiResult {
		t.Helper()
		resp, err := client.UpdateAutomation(map[string]interface{}{
			"workspace_id": workspaceID,
			"automation":   automation,
		})
		return send(t, resp, err)
	}

	node := func(automationID, nodeID, nodeType string, config map[string]interface{}, next string) map[string]interface{} {
		n := map[string]interface{}{
			"id":            nodeID,
			"automation_id": automationID,
			"type":          nodeType,
			"config":        config,
			"position":      map[string]interface{}{"x": 0, "y": 0},
		}
		if next != "" {
			n["next_node_id"] = next
		}
		return n
	}

	customTrigger := func(eventName string) map[string]interface{} {
		return map[string]interface{}{
			"event_kind":        "custom_event",
			"custom_event_name": eventName,
			"frequency":         "every_time",
		}
	}

	// automation builds the body of an automation whose first node is the root.
	automation := func(automationID, name, listID string, trigger map[string]interface{}, nodes ...map[string]interface{}) map[string]interface{} {
		return map[string]interface{}{
			"id":           automationID,
			"workspace_id": workspaceID,
			"name":         name,
			"status":       "draft",
			"list_id":      listID,
			"trigger":      trigger,
			"root_node_id": nodes[0]["id"],
			"nodes":        nodes,
			"stats":        map[string]interface{}{"enrolled": 0, "completed": 0, "exited": 0, "failed": 0},
		}
	}

	create := func(t *testing.T, body map[string]interface{}) {
		t.Helper()
		resp, err := client.CreateAutomation(map[string]interface{}{
			"workspace_id": workspaceID,
			"automation":   body,
		})
		result := send(t, resp, err)
		require.Equalf(t, http.StatusCreated, result.status, "automations.create: %s", result.raw)
	}

	createLive := func(t *testing.T, body map[string]interface{}) {
		t.Helper()
		create(t, body)
		result := activate(t, body["id"].(string))
		require.Equalf(t, http.StatusOK, result.status, "automations.activate: %s", result.raw)
	}

	storedAutomation := func(t *testing.T, automationID string) (status, listID string) {
		t.Helper()
		require.NoError(t, db.QueryRowContext(bg,
			`SELECT status, COALESCE(list_id, '') FROM automations WHERE id = $1`, automationID).Scan(&status, &listID))
		return status, listID
	}

	// triggerName names the automation's trigger, and the function it runs.
	triggerName := func(automationID string) string {
		return strings.ToLower("automation_trigger_" + strings.ReplaceAll(automationID, "-", ""))
	}

	triggerInstalled := func(t *testing.T, automationID string) bool {
		t.Helper()
		var installed bool
		require.NoError(t, db.QueryRowContext(bg, `
			SELECT EXISTS (
				SELECT 1 FROM pg_trigger
				WHERE tgrelid = to_regclass('contact_timeline')
				  AND NOT tgisinternal
				  AND tgname = $1
			)`, triggerName(automationID)).Scan(&installed))
		return installed
	}

	triggerFunctionInstalled := func(t *testing.T, automationID string) bool {
		t.Helper()
		var installed bool
		require.NoError(t, db.QueryRowContext(bg,
			`SELECT EXISTS (SELECT 1 FROM pg_proc WHERE proname = $1)`, triggerName(automationID)).Scan(&installed))
		return installed
	}

	// installedTrigger is the definition of the automation's installed trigger, which names what
	// it fires on.
	installedTrigger := func(t *testing.T, automationID string) string {
		t.Helper()
		var definition string
		require.NoError(t, db.QueryRowContext(bg, `
			SELECT pg_get_triggerdef(oid) FROM pg_trigger
			WHERE tgrelid = to_regclass('contact_timeline')
			  AND NOT tgisinternal
			  AND tgname = $1`, triggerName(automationID)).Scan(&definition))
		return definition
	}

	newContact := func(t *testing.T, prefix string) string {
		t.Helper()
		email := fmt.Sprintf("%s-%s@example.com", prefix, strings.ToLower(shortuuid.New()))
		_, err := factory.CreateContact(workspaceID, testutil.WithContactEmail(email))
		require.NoError(t, err)
		return email
	}

	subscribe := func(t *testing.T, email, listID string) {
		t.Helper()
		_, err := factory.CreateContactList(workspaceID,
			testutil.WithContactListEmail(email),
			testutil.WithContactListListID(listID),
			testutil.WithContactListStatus(domain.ContactListStatusActive))
		require.NoError(t, err)
	}

	// fire records the automation's event. A live automation enrols the contact inside this
	// very insert, so once it returns, "not enrolled" needs no waiting.
	fire := func(t *testing.T, email, eventName string) {
		t.Helper()
		require.NoError(t, factory.CreateCustomEvent(workspaceID, email, eventName, map[string]interface{}{}))
	}

	// waitAt waits for the contact's journey to be parked on nodeID, which is where a delay
	// leaves it until its time comes.
	waitAt := func(t *testing.T, automationID, email, nodeID string) *domain.ContactAutomation {
		t.Helper()
		var journey *domain.ContactAutomation
		testutil.WaitForCondition(t, func() bool {
			found, err := factory.FindContactAutomation(workspaceID, automationID, email)
			if err != nil || found == nil || found.CurrentNodeID == nil {
				return false
			}
			journey = found
			return *found.CurrentNodeID == nodeID && found.Status == domain.ContactAutomationStatusActive
		}, 10*time.Second, "waiting for "+email+" to wait on node "+nodeID)
		return journey
	}

	// failedSteps returns the error of every step of the journey that failed.
	failedSteps := func(t *testing.T, journeyID string) []string {
		t.Helper()
		executions, err := factory.GetNodeExecutions(workspaceID, journeyID)
		require.NoError(t, err)
		var failures []string
		for _, execution := range executions {
			if execution.Action == domain.NodeActionFailed {
				message := "(no error recorded)"
				if execution.Error != nil {
					message = *execution.Error
				}
				failures = append(failures, fmt.Sprintf("%s: %s", execution.NodeType, message))
			}
		}
		return failures
	}

	stepOutput := func(t *testing.T, journeyID string, nodeType domain.NodeType) map[string]interface{} {
		t.Helper()
		executions, err := factory.GetNodeExecutions(workspaceID, journeyID)
		require.NoError(t, err)
		for _, execution := range executions {
			if execution.NodeType == nodeType && execution.Action == domain.NodeActionCompleted {
				return execution.Output
			}
		}
		t.Fatalf("no completed %s step in journey %s", nodeType, journeyID)
		return nil
	}

	// --- the list of a live automation that sends email ----------------------------------

	t.Run("the list of a live automation that sends email", func(t *testing.T) {
		listName := "Newsletter " + shortuuid.New()[:6]
		listID := createList(t, listName)

		automationID := shortuuid.New()
		automationName := "Welcome series " + shortuuid.New()[:6]
		eventName := "list_guard_" + shortuuid.New()[:8]
		triggerNodeID, delayNodeID, emailNodeID := shortuuid.New(), shortuuid.New(), shortuuid.New()
		createLive(t, automation(automationID, automationName, listID, customTrigger(eventName),
			node(automationID, triggerNodeID, "trigger", map[string]interface{}{}, delayNodeID),
			node(automationID, delayNodeID, "delay", map[string]interface{}{"duration": 1, "unit": "days"}, emailNodeID),
			node(automationID, emailNodeID, "email", map[string]interface{}{"template_id": template.ID}, ""),
		))

		member := newContact(t, "member")
		subscribe(t, member, listID)

		// A contact waiting in the journey, due at the email step a day from now.
		waiting := newContact(t, "waiting")
		fire(t, waiting, eventName)
		waitingJourney := waitAt(t, automationID, waiting, emailNodeID)

		// The deletion is refused, naming the automation.
		result := deleteList(t, listID)
		require.Equalf(t, http.StatusConflict, result.status,
			"deleting the list of a live automation must be refused: %s", result.raw)
		assert.Contains(t, errorMessage(result), automationName)
		assert.Equal(t, map[string]string{automationID: automationName}, named(t, result, "automations"))
		assert.Equal(t, float64(1), result.body["automations_count"])

		// A member who may delete lists but not read automations is refused the same way, and
		// told how many automations use the list, not which.
		result = deleteListAs(t, listsOnly, listID)
		require.Equalf(t, http.StatusConflict, result.status, "%s", result.raw)
		assert.Contains(t, errorMessage(result), "while 1 live automation uses it")
		assert.Equal(t, []interface{}{}, result.body["automations"])
		assert.Equal(t, float64(1), result.body["automations_count"])
		assert.NotContains(t, result.raw, automationName)
		assert.NotContains(t, result.raw, automationID)

		// Nothing of the deletion happened: the list, its member and their timeline are as they were.
		assert.Equal(t, http.StatusOK, listStatus(t, listID), "the list must still be there")
		var memberStatus string
		var memberRemoved bool
		require.NoError(t, db.QueryRowContext(bg,
			`SELECT status, deleted_at IS NOT NULL FROM contact_lists WHERE email = $1 AND list_id = $2`,
			member, listID).Scan(&memberStatus, &memberRemoved))
		assert.Equal(t, string(domain.ContactListStatusActive), memberStatus)
		assert.False(t, memberRemoved, "the member must still be on the list")
		var removals int
		require.NoError(t, db.QueryRowContext(bg,
			`SELECT COUNT(*) FROM contact_timeline WHERE kind = 'list.removed' AND entity_id = $1`, listID).Scan(&removals))
		assert.Zero(t, removals, "a refused deletion must record no removal")

		// And the journey still works: the waiting contact's email step finds its list.
		require.NoError(t, factory.UpdateContactAutomationScheduledAt(workspaceID, waitingJourney.ID, time.Now().UTC().Add(-time.Second)))
		completed := waitForAutomationComplete(t, factory, workspaceID, automationID, waiting, 15*time.Second)
		assert.Zero(t, completed.RetryCount)
		assert.Empty(t, failedSteps(t, completed.ID))

		// A paused automation does not hold the list.
		frozen := newContact(t, "frozen")
		fire(t, frozen, eventName)
		frozenJourney := waitAt(t, automationID, frozen, emailNodeID)
		pause(t, automationID)

		result = deleteList(t, listID)
		require.Equalf(t, http.StatusOK, result.status, "a paused automation must not block the deletion: %s", result.raw)
		assert.Equal(t, http.StatusNotFound, listStatus(t, listID))

		// But it cannot go live again on a deleted list.
		result = activate(t, automationID)
		require.Equalf(t, http.StatusBadRequest, result.status,
			"activating an automation whose list was deleted must be refused: %s", result.raw)
		assert.Contains(t, errorMessage(result), listName)
		assert.Equal(t, map[string]string{listID: listName}, named(t, result, "lists"))
		status, _ := storedAutomation(t, automationID)
		assert.Equal(t, string(domain.AutomationStatusPaused), status)
		assert.False(t, triggerInstalled(t, automationID), "a refused activation must install no trigger")

		// So no contact fails: the one that was waiting stays where it was, even once its time
		// has come, and new events enrol nobody.
		require.NoError(t, factory.UpdateContactAutomationScheduledAt(workspaceID, frozenJourney.ID, time.Now().UTC().Add(-time.Second)))
		time.Sleep(2 * time.Second) // time for the scheduler to have run it, were the automation live
		stillWaiting, err := factory.GetContactAutomation(workspaceID, automationID, frozen)
		require.NoError(t, err)
		assert.Equal(t, domain.ContactAutomationStatusActive, stillWaiting.Status)
		require.NotNil(t, stillWaiting.CurrentNodeID)
		assert.Equal(t, emailNodeID, *stillWaiting.CurrentNodeID)
		assert.Zero(t, stillWaiting.RetryCount)
		assert.Nil(t, stillWaiting.LastError)
		assert.Empty(t, failedSteps(t, stillWaiting.ID))

		newcomer := newContact(t, "newcomer")
		fire(t, newcomer, eventName)
		enrolled, err := factory.FindContactAutomation(workspaceID, automationID, newcomer)
		require.NoError(t, err)
		assert.Nil(t, enrolled, "a paused automation enrols nobody")
	})

	t.Run("a draft automation does not block the deletion and cannot be activated after it", func(t *testing.T) {
		listName := "Draft list " + shortuuid.New()[:6]
		listID := createList(t, listName)

		automationID := shortuuid.New()
		triggerNodeID, emailNodeID := shortuuid.New(), shortuuid.New()
		create(t, automation(automationID, "Draft "+shortuuid.New()[:6], listID, customTrigger("draft_"+shortuuid.New()[:8]),
			node(automationID, triggerNodeID, "trigger", map[string]interface{}{}, emailNodeID),
			node(automationID, emailNodeID, "email", map[string]interface{}{"template_id": template.ID}, ""),
		))

		result := deleteList(t, listID)
		require.Equalf(t, http.StatusOK, result.status, "a draft automation must not block the deletion: %s", result.raw)

		result = activate(t, automationID)
		require.Equalf(t, http.StatusBadRequest, result.status, "%s", result.raw)
		assert.Contains(t, errorMessage(result), listName)
		status, _ := storedAutomation(t, automationID)
		assert.Equal(t, string(domain.AutomationStatusDraft), status)
		assert.False(t, triggerInstalled(t, automationID))
	})

	// The steps and triggers that break once their list is deleted hold it too, even in an
	// automation that sends no email.
	t.Run("a step adding contacts to the list, or a trigger listening to it, blocks the deletion", func(t *testing.T) {
		cases := []struct {
			name  string
			build func(automationID, listID string) map[string]interface{}
		}{
			{
				name: "add to list",
				build: func(automationID, listID string) map[string]interface{} {
					triggerNodeID, addNodeID := shortuuid.New(), shortuuid.New()
					return automation(automationID, "Adds to the list "+shortuuid.New()[:6], "", customTrigger("adds_"+shortuuid.New()[:8]),
						node(automationID, triggerNodeID, "trigger", map[string]interface{}{}, addNodeID),
						node(automationID, addNodeID, "add_to_list", map[string]interface{}{"list_id": listID, "status": "active"}, ""),
					)
				},
			},
			{
				name: "list trigger",
				build: func(automationID, listID string) map[string]interface{} {
					triggerNodeID, delayNodeID := shortuuid.New(), shortuuid.New()
					return automation(automationID, "Listens to the list "+shortuuid.New()[:6], "",
						map[string]interface{}{"event_kind": "list.removed", "list_id": listID, "frequency": "every_time"},
						node(automationID, triggerNodeID, "trigger", map[string]interface{}{}, delayNodeID),
						node(automationID, delayNodeID, "delay", map[string]interface{}{"duration": 1, "unit": "days"}, ""),
					)
				},
			},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				listName := "Held list " + shortuuid.New()[:6]
				listID := createList(t, listName)
				member := newContact(t, "held")
				subscribe(t, member, listID)

				automationID := shortuuid.New()
				body := tc.build(automationID, listID)
				createLive(t, body)

				result := deleteList(t, listID)
				require.Equalf(t, http.StatusConflict, result.status, "%s", result.raw)
				assert.Contains(t, errorMessage(result), body["name"].(string))
				assert.Equal(t, http.StatusOK, listStatus(t, listID))

				pause(t, automationID)
				result = deleteList(t, listID)
				require.Equalf(t, http.StatusOK, result.status, "%s", result.raw)

				// Paused, a list.removed trigger enrolled none of the members its deletion removed.
				enrolled, err := factory.FindContactAutomation(workspaceID, automationID, member)
				require.NoError(t, err)
				assert.Nil(t, enrolled)

				result = activate(t, automationID)
				require.Equalf(t, http.StatusBadRequest, result.status, "%s", result.raw)
				assert.Contains(t, errorMessage(result), listName)
			})
		}
	})

	// A deleted list has no members, which is all a list condition, a list-status branch and
	// a remove-from-list step ask about: they keep working, so they do not hold the list.
	t.Run("steps that only read or remove memberships do not block the deletion and keep working", func(t *testing.T) {
		listID := createList(t, "Read-only list "+shortuuid.New()[:6])
		member := newContact(t, "reader")
		subscribe(t, member, listID)

		inList := map[string]interface{}{
			"kind": "leaf",
			"leaf": map[string]interface{}{
				"source":       "contact_lists",
				"contact_list": map[string]interface{}{"operator": "in", "list_id": listID},
			},
		}
		notInList := map[string]interface{}{
			"kind": "leaf",
			"leaf": map[string]interface{}{
				"source":       "contact_lists",
				"contact_list": map[string]interface{}{"operator": "not_in", "list_id": listID},
			},
		}

		automationID := shortuuid.New()
		eventName := "reads_" + shortuuid.New()[:8]
		triggerNodeID, filterNodeID, branchNodeID, removeNodeID := shortuuid.New(), shortuuid.New(), shortuuid.New(), shortuuid.New()
		trigger := customTrigger(eventName)
		trigger["conditions"] = notInList
		createLive(t, automation(automationID, "Reads the list "+shortuuid.New()[:6], "", trigger,
			node(automationID, triggerNodeID, "trigger", map[string]interface{}{}, filterNodeID),
			node(automationID, filterNodeID, "filter", map[string]interface{}{
				"conditions":       inList,
				"continue_node_id": branchNodeID,
				"exit_node_id":     branchNodeID,
			}, ""),
			node(automationID, branchNodeID, "list_status_branch", map[string]interface{}{
				"list_id":             listID,
				"not_in_list_node_id": removeNodeID,
				"active_node_id":      removeNodeID,
				"non_active_node_id":  removeNodeID,
			}, ""),
			node(automationID, removeNodeID, "remove_from_list", map[string]interface{}{"list_id": listID}, ""),
		))

		// The trigger condition reads the membership: a member is not enrolled.
		fire(t, member, eventName)
		enrolled, err := factory.FindContactAutomation(workspaceID, automationID, member)
		require.NoError(t, err)
		require.Nil(t, enrolled, "the trigger condition should keep a member of the list out")

		result := deleteList(t, listID)
		require.Equalf(t, http.StatusOK, result.status, "%s", result.raw)

		// Once the list is gone the member is in no list: enrolled, then through every step.
		fire(t, member, eventName)
		journey := waitForAutomationComplete(t, factory, workspaceID, automationID, member, 15*time.Second)
		assert.Zero(t, journey.RetryCount)
		assert.Empty(t, failedSteps(t, journey.ID))
		assert.Equal(t, false, stepOutput(t, journey.ID, domain.NodeTypeFilter)["filter_passed"])
		assert.Equal(t, "not_in_list", stepOutput(t, journey.ID, domain.NodeTypeListStatusBranch)["branch_taken"])
	})

	t.Run("a live automation cannot be changed to use a deleted list", func(t *testing.T) {
		deletedName := "Gone list " + shortuuid.New()[:6]
		deletedID := createList(t, deletedName)
		result := deleteList(t, deletedID)
		require.Equalf(t, http.StatusOK, result.status, "%s", result.raw)

		ownListID := createList(t, "Own list "+shortuuid.New()[:6])
		automationID := shortuuid.New()
		eventName := "changes_" + shortuuid.New()[:8]
		triggerNodeID, emailNodeID, addNodeID := shortuuid.New(), shortuuid.New(), shortuuid.New()
		trigger := node(automationID, triggerNodeID, "trigger", map[string]interface{}{}, emailNodeID)
		email := node(automationID, emailNodeID, "email", map[string]interface{}{"template_id": template.ID}, "")
		createLive(t, automation(automationID, "Changed "+shortuuid.New()[:6], ownListID, customTrigger(eventName), trigger, email))

		// Its own list replaced by the deleted one.
		result = update(t, automation(automationID, "Changed list", deletedID, customTrigger(eventName), trigger, email))
		require.Equalf(t, http.StatusBadRequest, result.status, "%s", result.raw)
		assert.Contains(t, errorMessage(result), deletedName)

		// A step added that writes to the deleted list.
		emailThenAdd := node(automationID, emailNodeID, "email", map[string]interface{}{"template_id": template.ID}, addNodeID)
		add := node(automationID, addNodeID, "add_to_list", map[string]interface{}{"list_id": deletedID, "status": "active"}, "")
		result = update(t, automation(automationID, "Changed steps", ownListID, customTrigger(eventName), trigger, emailThenAdd, add))
		require.Equalf(t, http.StatusBadRequest, result.status, "%s", result.raw)
		assert.Contains(t, errorMessage(result), deletedName)

		// A step added that writes to a list that never existed: refused too, and not called
		// deleted.
		neverID := "never" + strings.ReplaceAll(uuid.New().String(), "-", "")[:20]
		addToNever := node(automationID, addNodeID, "add_to_list", map[string]interface{}{"list_id": neverID, "status": "active"}, "")
		result = update(t, automation(automationID, "Changed steps", ownListID, customTrigger(eventName), trigger, emailThenAdd, addToNever))
		require.Equalf(t, http.StatusBadRequest, result.status, "%s", result.raw)
		assert.Equal(t,
			`this automation uses the list "`+neverID+`", which does not exist: a live automation cannot use a list that does not exist`,
			errorMessage(result))
		refused, ok := result.body["lists"].([]interface{})
		require.Truef(t, ok, "the refusal should list the lists: %s", result.raw)
		assert.Equal(t, []interface{}{map[string]interface{}{"id": neverID, "name": "", "missing": true}}, refused)

		status, storedListID := storedAutomation(t, automationID)
		assert.Equal(t, string(domain.AutomationStatusLive), status)
		assert.Equal(t, ownListID, storedListID, "a refused update must write nothing")

		// Paused, the same change is saved; it is going live again that is refused.
		pause(t, automationID)
		result = update(t, automation(automationID, "Changed while paused", deletedID, customTrigger(eventName), trigger, email))
		require.Equalf(t, http.StatusOK, result.status, "%s", result.raw)
		result = activate(t, automationID)
		require.Equalf(t, http.StatusBadRequest, result.status, "%s", result.raw)
		assert.Contains(t, errorMessage(result), deletedName)
	})

	// --- a deletion and an activation at the same moment ---------------------------------

	// draftSendingTo creates a draft automation that emails the members of listID.
	draftSendingTo := func(t *testing.T, listID string) (automationID, name string) {
		t.Helper()
		automationID = shortuuid.New()
		name = "Racing " + shortuuid.New()[:6]
		triggerNodeID, emailNodeID := shortuuid.New(), shortuuid.New()
		create(t, automation(automationID, name, listID, customTrigger("race_"+shortuuid.New()[:8]),
			node(automationID, triggerNodeID, "trigger", map[string]interface{}{}, emailNodeID),
			node(automationID, emailNodeID, "email", map[string]interface{}{"template_id": template.ID}, ""),
		))
		return automationID, name
	}

	listDeleted := func(t *testing.T, listID string) bool {
		t.Helper()
		var deleted bool
		require.NoError(t, db.QueryRowContext(bg, `SELECT deleted_at IS NOT NULL FROM lists WHERE id = $1`, listID).Scan(&deleted))
		return deleted
	}

	// backendPID is the server process running the transaction.
	backendPID := func(t *testing.T, tx *sql.Tx) int {
		t.Helper()
		var pid int
		require.NoError(t, tx.QueryRowContext(bg, `SELECT pg_backend_pid()`).Scan(&pid))
		return pid
	}

	// waitUntilBlockedBy waits until a session of this database waits for a lock held by the
	// process pid: the request racing the transaction that process runs, which has to wait for it.
	waitUntilBlockedBy := func(t *testing.T, pid int) {
		t.Helper()
		testutil.WaitForCondition(t, func() bool {
			var blocked bool
			err := db.QueryRowContext(bg, `
				SELECT EXISTS (
					SELECT 1 FROM pg_stat_activity
					WHERE datname = current_database() AND $1 = ANY(pg_blocking_pids(pid))
				)`, pid).Scan(&blocked)
			return err == nil && blocked
		}, 10*time.Second, fmt.Sprintf("waiting for a session to wait for a lock held by process %d", pid))
	}

	activationWaitsForDeletion := func(t *testing.T) {
		listName := "Racing list " + shortuuid.New()[:6]
		listID := createList(t, listName)
		automationID, _ := draftSendingTo(t, listID)

		// The first statement of a list deletion, left uncommitted: it holds the list row.
		deletion, err := db.BeginTx(bg, nil)
		require.NoError(t, err)
		defer func() { _ = deletion.Rollback() }()
		_, err = deletion.ExecContext(bg, `UPDATE lists SET deleted_at = NOW() WHERE id = $1 AND deleted_at IS NULL`, listID)
		require.NoError(t, err)
		deletionPID := backendPID(t, deletion)

		activated := make(chan apiResult, 1)
		go func() {
			resp, err := client.ActivateAutomation(map[string]interface{}{
				"workspace_id":  workspaceID,
				"automation_id": automationID,
			})
			if err != nil {
				activated <- apiResult{raw: err.Error()}
				return
			}
			raw, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			result := apiResult{status: resp.StatusCode, raw: string(raw)}
			_ = json.Unmarshal(raw, &result.body)
			activated <- result
		}()

		// The activation has to wait for the deletion rather than read the list as it was.
		waitUntilBlockedBy(t, deletionPID)
		require.NoError(t, deletion.Commit())

		result := <-activated
		require.Equalf(t, http.StatusBadRequest, result.status, "%s", result.raw)
		assert.Contains(t, errorMessage(result), listName)
		status, _ := storedAutomation(t, automationID)
		assert.Equal(t, string(domain.AutomationStatusDraft), status)
		assert.False(t, triggerInstalled(t, automationID))
	}

	deletionWaitsForActivation := func(t *testing.T) {
		listID := createList(t, "Racing list "+shortuuid.New()[:6])
		automationID, automationName := draftSendingTo(t, listID)

		// What an activation does before it commits: hold the list, then write the row live.
		activation, err := db.BeginTx(bg, nil)
		require.NoError(t, err)
		defer func() { _ = activation.Rollback() }()
		_, err = activation.ExecContext(bg, `SELECT id FROM lists WHERE id = $1 FOR SHARE`, listID)
		require.NoError(t, err)
		_, err = activation.ExecContext(bg, `UPDATE automations SET status = 'live' WHERE id = $1`, automationID)
		require.NoError(t, err)
		activationPID := backendPID(t, activation)

		deleted := make(chan apiResult, 1)
		go func() {
			resp, err := client.Post("/api/lists.delete", map[string]interface{}{
				"workspace_id": workspaceID,
				"id":           listID,
			})
			if err != nil {
				deleted <- apiResult{raw: err.Error()}
				return
			}
			raw, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			result := apiResult{status: resp.StatusCode, raw: string(raw)}
			_ = json.Unmarshal(raw, &result.body)
			deleted <- result
		}()

		// The deletion has to wait for the activation, and then see it: its check of the
		// automations reads after the lock, not from the snapshot it started with.
		waitUntilBlockedBy(t, activationPID)
		require.NoError(t, activation.Commit())

		result := <-deleted
		require.Equalf(t, http.StatusConflict, result.status, "%s", result.raw)
		assert.Contains(t, errorMessage(result), automationName)
		assert.False(t, listDeleted(t, listID), "the list must still be there")

		// This automation went live without a trigger; put it back so nothing else trips on it.
		_, err = db.ExecContext(bg, `UPDATE automations SET status = 'paused' WHERE id = $1`, automationID)
		require.NoError(t, err)
	}

	t.Run("an activation waits for a deletion in flight and then refuses", activationWaitsForDeletion)
	t.Run("a deletion waits for an activation in flight and then refuses", deletionWaitsForActivation)

	// An update to a live automation stores the new version, then installs its trigger, and puts
	// the previous version back when the install fails. A list only the previous version uses is
	// held by nothing live in between, so it can be deleted then: the previous version goes back
	// paused, never live on the deleted list, and the caller is told.
	type undoneUpdate struct {
		automationID string
		// eventName is the custom event the previous version is triggered by.
		eventName   string
		oldListID   string
		oldListName string
		// body is what the update sends.
		body    map[string]interface{}
		hold    *sql.Tx
		holdPID int
		result  chan apiResult
	}

	// sendUpdate sends an update and answers on the channel, for a test that has to act while
	// the update waits.
	sendUpdate := func(body map[string]interface{}) chan apiResult {
		answer := make(chan apiResult, 1)
		go func() {
			resp, err := client.UpdateAutomation(map[string]interface{}{"workspace_id": workspaceID, "automation": body})
			if err != nil {
				answer <- apiResult{raw: err.Error()}
				return
			}
			raw, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			result := apiResult{status: resp.StatusCode, raw: string(raw)}
			_ = json.Unmarshal(raw, &result.body)
			answer <- result
		}()
		return answer
	}

	// refusedConditions is a trigger whose condition PostgreSQL has no operator for: "country" is
	// text, read here as a date. The install finds that out only once it has replaced the
	// trigger, and rolls back.
	refusedConditions := func(eventName string) map[string]interface{} {
		trigger := customTrigger(eventName)
		trigger["conditions"] = map[string]interface{}{
			"kind": "leaf",
			"leaf": map[string]interface{}{
				"source": "contacts",
				"contact": map[string]interface{}{"filters": []map[string]interface{}{{
					"field_name":    "country",
					"field_type":    "time",
					"operator":      "equals",
					"string_values": []string{"2020-01-01T00:00:00Z"},
				}}},
			},
		}
		return trigger
	}

	// movedTrigger is a trigger that installs cleanly: another event.
	movedTrigger := func(eventName string) map[string]interface{} {
		return customTrigger(eventName + "_moved")
	}

	// liveOnOldList makes a live automation that emails the members of a list of its own, and
	// the update that would move it to another list with the trigger newTrigger builds.
	liveOnOldList := func(t *testing.T, newTrigger func(eventName string) map[string]interface{}) (undoneUpdate, map[string]interface{}) {
		t.Helper()
		u := undoneUpdate{automationID: shortuuid.New(), oldListName: "Old list " + shortuuid.New()[:6]}
		u.oldListID = createList(t, u.oldListName)
		newListID := createList(t, "New list "+shortuuid.New()[:6])

		u.eventName = "undone_" + shortuuid.New()[:8]
		triggerNodeID, emailNodeID := shortuuid.New(), shortuuid.New()
		trigger := node(u.automationID, triggerNodeID, "trigger", map[string]interface{}{}, emailNodeID)
		email := node(u.automationID, emailNodeID, "email", map[string]interface{}{"template_id": template.ID}, "")
		createLive(t, automation(u.automationID, "Undone "+shortuuid.New()[:6], u.oldListID, customTrigger(u.eventName), trigger, email))
		require.True(t, triggerInstalled(t, u.automationID))

		return u, automation(u.automationID, "Undone, moved", newListID, newTrigger(u.eventName), trigger, email)
	}

	// holdContactTimeline opens a write to contact_timeline and leaves it in progress: the trigger
	// DDL of an update waits for it.
	holdContactTimeline := func(t *testing.T) (*sql.Tx, int) {
		t.Helper()
		hold, err := db.BeginTx(bg, nil)
		require.NoError(t, err)
		t.Cleanup(func() { _ = hold.Rollback() })
		_, err = hold.ExecContext(bg, `LOCK TABLE contact_timeline IN ROW EXCLUSIVE MODE`)
		require.NoError(t, err)
		return hold, backendPID(t, hold)
	}

	// startUndoneUpdate holds a write to contact_timeline open, then sends the update: it stores
	// the new version, and its trigger install waits for the write. Meanwhile it deletes the
	// first list.
	startUndoneUpdate := func(t *testing.T, newTrigger func(eventName string) map[string]interface{}) undoneUpdate {
		t.Helper()
		u, body := liveOnOldList(t, newTrigger)
		u.body = body

		u.hold, u.holdPID = holdContactTimeline(t)
		u.result = sendUpdate(body)

		// The install waits while the stored version uses the new list only: nothing live holds
		// the old one, and its deletion goes through.
		waitUntilBlockedBy(t, u.holdPID)
		result := deleteList(t, u.oldListID)
		require.Equalf(t, http.StatusOK, result.status, "%s", result.raw)
		return u
	}

	assertPausedOnOldList := func(t *testing.T, u undoneUpdate) {
		t.Helper()
		status, storedListID := storedAutomation(t, u.automationID)
		assert.Equal(t, string(domain.AutomationStatusPaused), status, "never back live on the deleted list")
		assert.Equal(t, u.oldListID, storedListID, "the version the update replaced")
		assert.False(t, triggerInstalled(t, u.automationID), "no trigger left behind to enrol anyone")
		assert.True(t, listDeleted(t, u.oldListID))
	}

	// assertToldPaused checks the fields a client reads the pause from without parsing the message:
	// paused, and the lists that kept the previous version from going back live, by id and name,
	// when that is why.
	assertToldPaused := func(t *testing.T, result apiResult, lists map[string]string) {
		t.Helper()
		assert.Equalf(t, true, result.body["paused"], "%s", result.raw)
		if lists == nil {
			assert.NotContains(t, result.body, "lists")
		} else {
			assert.Equal(t, lists, named(t, result, "lists"))
		}
		assert.NotContains(t, result.body, "running_previous_trigger")
	}

	// waitUntilDropBlockedBy waits until a session of this database runs a DROP TRIGGER that waits
	// for a lock held by the process pid.
	waitUntilDropBlockedBy := func(t *testing.T, pid int) {
		t.Helper()
		testutil.WaitForCondition(t, func() bool {
			var blocked bool
			err := db.QueryRowContext(bg, `
				SELECT EXISTS (
					SELECT 1 FROM pg_stat_activity
					WHERE datname = current_database()
					  AND query LIKE 'DROP TRIGGER %'
					  AND $1 = ANY(pg_blocking_pids(pid))
				)`, pid).Scan(&blocked)
			return err == nil && blocked
		}, 10*time.Second, fmt.Sprintf("waiting for a DROP TRIGGER to wait for a lock held by process %d", pid))
	}

	// startDeletion runs what lists.delete runs once its in-use check has passed, and leaves it
	// uncommitted, as a deletion with many members would be: the list marked deleted, held until
	// the deletion commits, then its members removed, each removal recorded in contact_timeline,
	// which is held from the first one. It returns once the members are removed.
	startDeletion := func(t *testing.T, listID string) (*sql.Tx, int) {
		t.Helper()
		deletion, err := db.BeginTx(bg, nil)
		require.NoError(t, err)
		t.Cleanup(func() { _ = deletion.Rollback() })
		_, err = deletion.ExecContext(bg, `UPDATE lists SET deleted_at = NOW() WHERE id = $1 AND deleted_at IS NULL`, listID)
		require.NoError(t, err)
		removed, err := deletion.ExecContext(bg, `UPDATE contact_lists SET deleted_at = NOW() WHERE list_id = $1 AND deleted_at IS NULL`, listID)
		require.NoError(t, err)
		members, err := removed.RowsAffected()
		require.NoError(t, err)
		require.Positive(t, members, "a deletion that removes no member holds no contact_timeline")
		return deletion, backendPID(t, deletion)
	}

	t.Run("an update undone after its trigger was refused does not leave the automation live on a list deleted meanwhile", func(t *testing.T) {
		u := startUndoneUpdate(t, refusedConditions)
		// Released well within the install's lock timeout.
		require.NoError(t, u.hold.Rollback())

		// The caller is told why the update failed, and that the automation is no longer live.
		result := <-u.result
		require.Equalf(t, http.StatusBadRequest, result.status, "%s", result.raw)
		assert.Contains(t, errorMessage(result), "invalid trigger conditions")
		assert.Contains(t, errorMessage(result),
			`; the automation was paused, because its previous version uses the deleted list "`+u.oldListName+`"`)
		assertToldPaused(t, result, map[string]string{u.oldListID: u.oldListName})
		assertPausedOnOldList(t, u)

		// The audit row of the failed update records the pause too.
		audited := listAuditLogs(t, client, map[string]string{
			"workspace_id": workspaceID,
			"actions":      "automations.update",
			"target_id":    u.automationID,
		})
		require.Len(t, audited.Logs, 1)
		assert.Equal(t, domain.AuditOutcomeFailure, audited.Logs[0].Outcome)
		assert.Equal(t, true, audited.Logs[0].Metadata["paused"])

		// Saved again as an editor that loaded it live holds it: stored, and answered paused, so
		// the editor does not take the save for a version that runs.
		u.body["status"] = string(domain.AutomationStatusLive)
		saved := update(t, u.body)
		require.Equalf(t, http.StatusOK, saved.status, "%s", saved.raw)
		savedAutomation, _ := saved.body["automation"].(map[string]interface{})
		assert.Equalf(t, string(domain.AutomationStatusPaused), savedAutomation["status"], "%s", saved.raw)
	})

	t.Run("an update undone after its trigger install timed out does not leave the automation live on a list deleted meanwhile", func(t *testing.T) {
		u := startUndoneUpdate(t, movedTrigger)

		// Held through the install's lock timeout. The automation goes back paused, and dropping
		// its trigger then waits for the write in turn, within the same timeout.
		testutil.WaitForCondition(t, func() bool {
			status, _ := storedAutomation(t, u.automationID)
			return status == string(domain.AutomationStatusPaused)
		}, 15*time.Second, "waiting for the automation to go back paused")
		waitUntilBlockedBy(t, u.holdPID)
		require.NoError(t, u.hold.Rollback())

		result := <-u.result
		require.Equalf(t, http.StatusInternalServerError, result.status, "%s", result.raw)
		assert.Equal(t,
			`Failed to update automation; the automation was paused, because its previous version uses the deleted list "`+u.oldListName+`"`,
			errorMessage(result))
		assertToldPaused(t, result, map[string]string{u.oldListID: u.oldListName})
		assertPausedOnOldList(t, u)
	})

	// A list deletion holds the list from its first statement, and contact_timeline from the first
	// member it removes, until it commits: for as long as it takes to remove every member. An update
	// that moved a live automation off the list meanwhile installs its trigger, which waits for
	// contact_timeline and gives up; putting the previous version back waits for the list and gives
	// up; the automation goes back paused, and dropping the previous version's trigger waits for
	// contact_timeline in turn. Every write to contact_timeline queues behind DDL waiting there, so
	// each wait is bounded: the drop gives up like the install, leaving the trigger in place, but
	// inactive, as the automation is paused.
	t.Run("an update made during a deletion of its old list goes back paused, its previous trigger left in place but inactive", func(t *testing.T) {
		// How long each step may wait: the install and the drop of a trigger, for contact_timeline,
		// their lock timeout; putting the previous version back, its budget.
		const lockTimeout, restoreBudget = 5 * time.Second, 5 * time.Second

		u, body := liveOnOldList(t, movedTrigger)
		subscribe(t, newContact(t, "removed"), u.oldListID)
		// Created first: once trigger DDL waits for contact_timeline, so does creating a contact.
		queued := newContact(t, "queued")

		hold, holdPID := holdContactTimeline(t)
		sent := time.Now()
		answer := sendUpdate(body)
		waitUntilBlockedBy(t, holdPID)
		// The stored version no longer uses the list, so the deletion's in-use check passes. Its
		// member removals wait for the install, until it gives up.
		deletion, deletionPID := startDeletion(t, u.oldListID)

		// Paused once the restore has run out of its budget; then the drop waits, and a write
		// queued behind it, an event the previous version is triggered by, waits with it.
		testutil.WaitForCondition(t, func() bool {
			status, _ := storedAutomation(t, u.automationID)
			return status == string(domain.AutomationStatusPaused)
		}, lockTimeout+restoreBudget+5*time.Second, "waiting for the automation to go back paused")
		waitUntilDropBlockedBy(t, deletionPID)
		queuedAt := time.Now()
		fire(t, queued, u.eventName)
		servedAfter := time.Since(queuedAt)

		result := <-answer
		answeredAfter := time.Since(sent)
		require.NoError(t, deletion.Commit())
		require.NoError(t, hold.Rollback())

		// Each step waited as long as it may and no longer: the update answered once the install,
		// the restore and the drop had run out one after the other, and the write queued behind the
		// drop was served once the drop gave up.
		waited := lockTimeout + restoreBudget + lockTimeout
		assert.GreaterOrEqual(t, answeredAfter, waited-time.Second, "a step gave up before its time")
		assert.Less(t, answeredAfter, waited+2*time.Second, "a step waited past its time")
		assert.Less(t, servedAfter, lockTimeout+500*time.Millisecond, "the drop held up the write past its lock timeout")

		require.Equalf(t, http.StatusInternalServerError, result.status, "%s", result.raw)
		assert.Equal(t,
			"Failed to update automation; the automation was paused, because its previous version could not be put back",
			errorMessage(result))
		assertToldPaused(t, result, nil)

		status, storedListID := storedAutomation(t, u.automationID)
		assert.Equal(t, string(domain.AutomationStatusPaused), status, "never back live on the deleted list")
		assert.Equal(t, u.oldListID, storedListID, "the version the update replaced")
		assert.True(t, listDeleted(t, u.oldListID))
		assert.True(t, triggerInstalled(t, u.automationID), "the drop gave up")
		assert.True(t, triggerFunctionInstalled(t, u.automationID), "the drop gave up")
		journey, err := factory.FindContactAutomation(workspaceID, u.automationID, queued)
		require.NoError(t, err)
		assert.Nil(t, journey, "the trigger left in place enrols nobody into a paused automation")
	})

	// The previous version is triggered by members leaving the list being deleted, and its trigger
	// is still installed when the deletion removes them: each enrolment it makes counts in the
	// automation's row, which stays locked until the deletion commits. So the paused write waits
	// for the deletion as the restore does, and neither can be written. The automation is left
	// live, storing the new version and running the trigger of the previous one, and saving that
	// version again would not reinstall it: for that save, nothing the trigger is compiled from
	// has changed. The caller is told, and told what does reinstall it.
	t.Run("an update undone neither way says the automation runs its previous trigger, until paused and activated again", func(t *testing.T) {
		oldListID := createList(t, "Leaving old "+shortuuid.New()[:6])
		newListID := createList(t, "Leaving new "+shortuuid.New()[:6])
		subscribe(t, newContact(t, "leaving"), oldListID)

		automationID := shortuuid.New()
		triggerNodeID, delayNodeID := shortuuid.New(), shortuuid.New()
		trigger := node(automationID, triggerNodeID, "trigger", map[string]interface{}{}, delayNodeID)
		delay := node(automationID, delayNodeID, "delay", map[string]interface{}{"duration": 1, "unit": "days"}, "")
		leaving := func(listID string) map[string]interface{} {
			return map[string]interface{}{"event_kind": "list.removed", "list_id": listID, "frequency": "every_time"}
		}
		createLive(t, automation(automationID, "Leaving "+shortuuid.New()[:6], "", leaving(oldListID), trigger, delay))
		require.Contains(t, installedTrigger(t, automationID), oldListID)

		hold, holdPID := holdContactTimeline(t)
		answer := sendUpdate(automation(automationID, "Leaving, moved", "", leaving(newListID), trigger, delay))
		waitUntilBlockedBy(t, holdPID)
		deletion, _ := startDeletion(t, oldListID)

		result := <-answer
		require.NoError(t, deletion.Commit())
		require.NoError(t, hold.Rollback())

		require.Equalf(t, http.StatusInternalServerError, result.status, "%s", result.raw)
		assert.Equal(t,
			"Failed to update automation; the automation is still live, storing this version but running the previous version's trigger: pause it and activate it again",
			errorMessage(result))
		assert.Equalf(t, true, result.body["running_previous_trigger"], "%s", result.raw)
		assert.NotContains(t, result.body, "paused")

		// As it says.
		status, _ := storedAutomation(t, automationID)
		assert.Equal(t, string(domain.AutomationStatusLive), status)
		var storedTriggerList string
		require.NoError(t, db.QueryRowContext(bg,
			`SELECT trigger_config->>'list_id' FROM automations WHERE id = $1`, automationID).Scan(&storedTriggerList))
		assert.Equal(t, newListID, storedTriggerList)
		assert.Contains(t, installedTrigger(t, automationID), oldListID)

		// And as it says to do.
		pause(t, automationID)
		activated := activate(t, automationID)
		require.Equalf(t, http.StatusOK, activated.status, "%s", activated.raw)
		assert.Contains(t, installedTrigger(t, automationID), newListID)
		assert.NotContains(t, installedTrigger(t, automationID), oldListID)
	})

	// Each side reads what the transaction it waited for committed, which READ COMMITTED gives
	// and a stricter level does not: there the deletion would check the automations as they
	// were before the activation it waited for, and the activation would fail on the deleted
	// list instead of refusing it. So both run at READ COMMITTED whatever the server's default.
	t.Run("the same on a database whose default isolation is repeatable read", func(t *testing.T) {
		var database string
		require.NoError(t, db.QueryRowContext(bg, `SELECT current_database()`).Scan(&database))
		setDefault := func(t *testing.T, clause string) {
			_, err := db.ExecContext(bg, `ALTER DATABASE `+pq.QuoteIdentifier(database)+` `+clause)
			require.NoError(t, err)
			// A session takes the default when it opens, so the app needs new ones. db comes
			// from the app's pool, so it is taken anew as well.
			manager, err := pgdb.GetConnectionManager()
			require.NoError(t, err)
			require.NoError(t, manager.CloseWorkspaceConnection(workspaceID))
			db, err = factory.GetWorkspaceDB(workspaceID)
			require.NoError(t, err)
		}
		setDefault(t, `SET default_transaction_isolation = 'repeatable read'`)
		t.Cleanup(func() { setDefault(t, `RESET default_transaction_isolation`) })
		var isolation string
		require.NoError(t, db.QueryRowContext(bg, `SHOW default_transaction_isolation`).Scan(&isolation))
		require.Equal(t, "repeatable read", isolation, "the app's sessions must now default to it")

		t.Run("an activation waits for a deletion in flight and then refuses", activationWaitsForDeletion)
		t.Run("a deletion waits for an activation in flight and then refuses", deletionWaitsForActivation)
	})

	// Whichever lands first, the other is refused: never a live automation on a deleted list.
	t.Run("concurrent deletion and activation", func(t *testing.T) {
		for round := 0; round < 5; round++ {
			t.Run(fmt.Sprintf("round-%d", round), func(t *testing.T) {
				listID := createList(t, "Racing list "+shortuuid.New()[:6])
				automationID, _ := draftSendingTo(t, listID)

				var deletion, activation apiResult
				done := make(chan struct{}, 2)
				go func() {
					defer func() { done <- struct{}{} }()
					resp, err := client.Post("/api/lists.delete", map[string]interface{}{
						"workspace_id": workspaceID,
						"id":           listID,
					})
					if err == nil {
						deletion.status = resp.StatusCode
						_ = resp.Body.Close()
					}
				}()
				go func() {
					defer func() { done <- struct{}{} }()
					resp, err := client.ActivateAutomation(map[string]interface{}{
						"workspace_id":  workspaceID,
						"automation_id": automationID,
					})
					if err == nil {
						activation.status = resp.StatusCode
						_ = resp.Body.Close()
					}
				}()
				<-done
				<-done

				status, _ := storedAutomation(t, automationID)
				live := status == string(domain.AutomationStatusLive)
				gone := listDeleted(t, listID)
				assert.False(t, live && gone, "a live automation must never be left on a deleted list")

				if gone {
					assert.Equal(t, http.StatusOK, deletion.status)
					assert.Equal(t, http.StatusBadRequest, activation.status)
					assert.False(t, triggerInstalled(t, automationID))
				} else {
					assert.Equal(t, http.StatusConflict, deletion.status)
					assert.Equal(t, http.StatusOK, activation.status)
					assert.True(t, live)
					pause(t, automationID)
				}
			})
		}
	})
}
