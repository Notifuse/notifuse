//go:build integration
// +build integration

package integration

import (
	"context"
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

	deleteList := func(t *testing.T, listID string) apiResult {
		t.Helper()
		resp, err := client.Post("/api/lists.delete", map[string]interface{}{
			"workspace_id": workspaceID,
			"id":           listID,
		})
		return send(t, resp, err)
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

	triggerInstalled := func(t *testing.T, automationID string) bool {
		t.Helper()
		var installed bool
		require.NoError(t, db.QueryRowContext(bg, `
			SELECT EXISTS (
				SELECT 1 FROM pg_trigger
				WHERE tgrelid = to_regclass('contact_timeline')
				  AND NOT tgisinternal
				  AND tgname = $1
			)`, strings.ToLower("automation_trigger_"+strings.ReplaceAll(automationID, "-", ""))).Scan(&installed))
		return installed
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

	// waitForLockWait waits until a statement matching pattern is blocked on a row lock, which
	// is what the request racing the open transaction below has to do.
	waitForLockWait := func(t *testing.T, pattern string) {
		t.Helper()
		testutil.WaitForCondition(t, func() bool {
			var waiting int
			err := db.QueryRowContext(bg, `
				SELECT COUNT(*) FROM pg_stat_activity
				WHERE datname = current_database() AND wait_event_type = 'Lock' AND query LIKE $1`,
				pattern).Scan(&waiting)
			return err == nil && waiting > 0
		}, 10*time.Second, "waiting for a statement like "+pattern+" to wait on a lock")
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
		waitForLockWait(t, "%FROM lists WHERE id = ANY%FOR SHARE%")
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
		waitForLockWait(t, "UPDATE lists SET deleted_at%")
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
