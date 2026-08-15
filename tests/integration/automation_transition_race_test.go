package integration

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lithammer/shortuuid/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/app"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/repository"
	"github.com/Notifuse/notifuse/internal/service"
	"github.com/Notifuse/notifuse/tests/testutil"
)

// TestAutomationTransitionRace covers the states an automation's row and its installed
// trigger can end up disagreeing in, and the guard that makes the disagreement harmless.
//
// The three transitions each read the row, decide, write the whole row back, and emit DDL,
// with no lock and — before this — no predicate on the write. Two admins acting at once can
// leave a paused automation with its trigger still installed; and Pause alone, dropping the
// trigger before writing the status and with no compensation, could leave a live automation
// with no trigger at all on nothing more than a client disconnect.
//
// Neither state is detectable after the fact: the scheduler and the executor filter on
// status, so nothing is ever sent, while the trigger keeps enrolling contacts whose journeys
// sit frozen until someone re-activates the automation and the whole backlog thaws at once.
// A unit test cannot show this — the enrolment happens inside PostgreSQL, in a function the
// Go code never calls directly.
func TestAutomationTransitionRace(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	factory := suite.DataFactory
	client := suite.APIClient

	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	workspaceID := workspace.ID

	user, err := factory.CreateUser()
	require.NoError(t, err)
	require.NoError(t, factory.AddUserToWorkspace(user.ID, workspaceID, "owner"))
	require.NoError(t, client.Login(user.Email, "password"))
	client.SetWorkspaceID(workspaceID)

	list, err := factory.CreateList(workspaceID)
	require.NoError(t, err)
	template, err := factory.CreateTemplate(workspaceID)
	require.NoError(t, err)

	workspaceDB, err := factory.GetWorkspaceDB(workspaceID)
	require.NoError(t, err)

	ctx := context.Background()
	const eventKind = "email.opened"

	// --- helpers -----------------------------------------------------------------------

	readBody := func(t *testing.T, resp *http.Response) string {
		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		require.NoError(t, readErr)
		return string(body)
	}

	newAutomation := func(name string) map[string]interface{} {
		automationID := shortuuid.New()
		triggerNodeID := shortuuid.New()
		emailNodeID := shortuuid.New()

		return map[string]interface{}{
			"id":           automationID,
			"workspace_id": workspaceID,
			"name":         name,
			"status":       "draft",
			"list_id":      list.ID,
			"trigger": map[string]interface{}{
				"event_kind": eventKind,
				"frequency":  "once",
			},
			"root_node_id": triggerNodeID,
			"nodes": []map[string]interface{}{
				{
					"id":            triggerNodeID,
					"automation_id": automationID,
					"type":          "trigger",
					"config":        map[string]interface{}{},
					"next_node_id":  emailNodeID,
					"position":      map[string]interface{}{"x": 0, "y": 0},
				},
				{
					"id":            emailNodeID,
					"automation_id": automationID,
					"type":          "email",
					"config":        map[string]interface{}{"template_id": template.ID},
					"position":      map[string]interface{}{"x": 0, "y": 100},
				},
			},
			"stats": map[string]interface{}{"enrolled": 0, "completed": 0, "exited": 0, "failed": 0},
		}
	}

	// activateLive creates the automation and activates it, so the trigger is installed by
	// the same code path production uses.
	activateLive := func(t *testing.T, name string) string {
		payload := newAutomation(name)
		automationID := payload["id"].(string)

		resp, createErr := client.CreateAutomation(map[string]interface{}{
			"workspace_id": workspaceID,
			"automation":   payload,
		})
		require.NoError(t, createErr)
		require.Equalf(t, http.StatusCreated, resp.StatusCode, "CreateAutomation: %s", readBody(t, resp))

		resp, activateErr := client.ActivateAutomation(map[string]interface{}{
			"workspace_id":  workspaceID,
			"automation_id": automationID,
		})
		require.NoError(t, activateErr)
		require.Equalf(t, http.StatusOK, resp.StatusCode, "ActivateAutomation: %s", readBody(t, resp))
		return automationID
	}

	pause := func(t *testing.T, automationID string) (int, string) {
		resp, pauseErr := client.PauseAutomation(map[string]interface{}{
			"workspace_id":  workspaceID,
			"automation_id": automationID,
		})
		require.NoError(t, pauseErr)
		return resp.StatusCode, readBody(t, resp)
	}

	triggerName := func(automationID string) string {
		// PostgreSQL folds unquoted identifiers to lower case, so the catalog name is the
		// lower-cased form of the mixed-case shortuuid used in the CREATE TRIGGER DDL.
		return strings.ToLower("automation_trigger_" + strings.ReplaceAll(automationID, "-", ""))
	}

	triggerInstalled := func(t *testing.T, automationID string) bool {
		var installed bool
		require.NoError(t, workspaceDB.QueryRowContext(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM pg_trigger
				WHERE tgrelid = to_regclass('contact_timeline')
				  AND NOT tgisinternal
				  AND tgname = $1
			)`, triggerName(automationID)).Scan(&installed))
		return installed
	}

	storedStatus := func(t *testing.T, automationID string) string {
		var status string
		require.NoError(t, workspaceDB.QueryRowContext(ctx,
			`SELECT status FROM automations WHERE id = $1`, automationID).Scan(&status))
		return status
	}

	enrolledStat := func(t *testing.T, automationID string) int {
		var enrolled int
		require.NoError(t, workspaceDB.QueryRowContext(ctx,
			`SELECT COALESCE((stats->>'enrolled')::int, 0) FROM automations WHERE id = $1`,
			automationID).Scan(&enrolled))
		return enrolled
	}

	countRows := func(t *testing.T, query string, args ...interface{}) int {
		var count int
		require.NoError(t, workspaceDB.QueryRowContext(ctx, query, args...).Scan(&count))
		return count
	}

	startEvents := func(t *testing.T, automationID string) int {
		return countRows(t, `SELECT COUNT(*) FROM contact_timeline
			WHERE kind = 'automation.start' AND entity_id = $1`, automationID)
	}

	triggerLogRows := func(t *testing.T, automationID string) int {
		return countRows(t, `SELECT COUNT(*) FROM automation_trigger_log WHERE automation_id = $1`, automationID)
	}

	journeys := func(t *testing.T, automationID string) int {
		return countRows(t, `SELECT COUNT(*) FROM contact_automations WHERE automation_id = $1`, automationID)
	}

	// fire inserts the event the automation triggers on. The enrolment, if any, is written
	// by automation_enroll_contact inside this very insert, so once it returns the answer is
	// final and a "did not enrol" assertion needs no waiting.
	fire := func(t *testing.T, email string) {
		require.NoError(t, factory.CreateContactTimelineEvent(workspaceID, email, eventKind,
			map[string]interface{}{"at": time.Now().UTC().Format(time.RFC3339)}))
	}

	newEmail := func() string { return strings.ToLower(shortuuid.New()) + "@example.com" }

	// --- the residual states -----------------------------------------------------------

	// Each case leaves the trigger installed and moves the row out from under it — exactly
	// what the interleavings produce — then fires the event the trigger listens for.
	t.Run("GhostTriggerDoesNotEnrol", func(t *testing.T) {
		cases := []struct {
			name    string
			corrupt string
		}{
			{
				// Update's row write commits, Pause drops the trigger, Update reinstalls it,
				// Pause writes 'paused'.
				name:    "paused row with the trigger still installed",
				corrupt: `UPDATE automations SET status = 'paused' WHERE id = $1`,
			},
			{
				// The mirror interleaving against Activate: Update writes back the draft
				// status it read while the trigger Activate installed stays behind.
				name:    "draft row with the trigger still installed",
				corrupt: `UPDATE automations SET status = 'draft' WHERE id = $1`,
			},
			{
				// DeleteTx drops the trigger with its error discarded, so a failed drop
				// leaves a soft-deleted automation still enrolling.
				name:    "soft-deleted row with the trigger still installed",
				corrupt: `UPDATE automations SET deleted_at = NOW() WHERE id = $1`,
			},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				automationID := activateLive(t, tc.name)
				require.True(t, triggerInstalled(t, automationID), "activation must install the trigger")

				_, execErr := workspaceDB.ExecContext(ctx, tc.corrupt, automationID)
				require.NoError(t, execErr)
				require.True(t, triggerInstalled(t, automationID), "the trigger must still be installed for this test to mean anything")

				fire(t, newEmail())

				assert.Equal(t, 0, journeys(t, automationID), "no contact may be enrolled")
				assert.Equal(t, 0, startEvents(t, automationID), "no automation.start event may be written")
				assert.Equal(t, 0, enrolledStat(t, automationID), "the enrolled stat may not move")
				// Permanent and never cleaned up: a ghost 'once' row would bar that contact
				// from the automation for good once it is live again.
				assert.Equal(t, 0, triggerLogRows(t, automationID), "no dedup row may be recorded")
			})
		}
	})

	// The guard must gate on the automation, not on the event: a live automation still
	// enrols exactly as before.
	t.Run("LiveAutomationStillEnrols", func(t *testing.T) {
		automationID := activateLive(t, "live automation")
		email := newEmail()

		fire(t, email)

		enrollment, findErr := factory.FindContactAutomation(workspaceID, automationID, email)
		require.NoError(t, findErr)
		require.NotNil(t, enrollment, "a live automation must still enrol")
		assert.Equal(t, domain.ContactAutomationStatusActive, enrollment.Status)
		assert.Equal(t, 1, startEvents(t, automationID))
		assert.Equal(t, 1, enrolledStat(t, automationID))
		assert.Equal(t, 1, triggerLogRows(t, automationID), "frequency 'once' still records its dedup row")
	})

	// --- the transitions ---------------------------------------------------------------

	t.Run("PauseLeavesNoTriggerInstalled", func(t *testing.T) {
		automationID := activateLive(t, "pause end state")

		status, body := pause(t, automationID)
		require.Equalf(t, http.StatusOK, status, "PauseAutomation: %s", body)

		assert.Equal(t, string(domain.AutomationStatusPaused), storedStatus(t, automationID))
		assert.False(t, triggerInstalled(t, automationID), "a completed pause must leave no trigger behind")
	})

	// Pause writes the status first and drops second, so an interrupted pause leaves the
	// trigger installed. Retrying must be able to finish the job — otherwise the only way to
	// clear the orphan would be to activate the automation again first.
	t.Run("PauseRepairsOrphanTrigger", func(t *testing.T) {
		automationID := activateLive(t, "orphan trigger repair")
		_, execErr := workspaceDB.ExecContext(ctx,
			`UPDATE automations SET status = 'paused' WHERE id = $1`, automationID)
		require.NoError(t, execErr)
		require.True(t, triggerInstalled(t, automationID))

		status, body := pause(t, automationID)
		require.Equalf(t, http.StatusOK, status, "PauseAutomation: %s", body)

		assert.Equal(t, string(domain.AutomationStatusPaused), storedStatus(t, automationID))
		assert.False(t, triggerInstalled(t, automationID), "a retried pause must drop the orphan trigger")
	})

	// The predicate against a real database, where a sqlmock test can only assert the SQL it
	// was handed back: a write computed from a row another transition has already moved must
	// not land at all.
	t.Run("UpdateIfStatusRejectsStaleStatus", func(t *testing.T) {
		automationID := activateLive(t, "optimistic lock")

		repo := repository.NewAutomationRepositoryWithDB(
			workspaceDB, service.NewAutomationTriggerGenerator(service.NewQueryBuilder()))

		stored, getErr := repo.GetByID(ctx, workspaceID, automationID)
		require.NoError(t, getErr)
		require.Equal(t, domain.AutomationStatusLive, stored.Status)

		stored.Status = domain.AutomationStatusPaused
		updated, updateErr := repo.UpdateIfStatus(ctx, workspaceID, stored, domain.AutomationStatusLive)
		require.NoError(t, updateErr)
		assert.True(t, updated, "the write must land while the stored status still matches")
		assert.Equal(t, string(domain.AutomationStatusPaused), storedStatus(t, automationID))

		// Now replay the same stale decision, as a second admin's request would.
		stale := *stored
		stale.Name = "written from a stale read"
		stale.Status = domain.AutomationStatusLive
		updated, updateErr = repo.UpdateIfStatus(ctx, workspaceID, &stale, domain.AutomationStatusLive)
		require.NoError(t, updateErr, "losing the race is an outcome, not a failure")
		assert.False(t, updated)

		var name, status string
		require.NoError(t, workspaceDB.QueryRowContext(ctx,
			`SELECT name, status FROM automations WHERE id = $1`, automationID).Scan(&name, &status))
		assert.Equal(t, string(domain.AutomationStatusPaused), status, "the status must not be resurrected")
		assert.NotEqual(t, "written from a stale read", name, "no field of a stale write may land")
	})

	// The invariant that has to survive every interleaving. Which admin wins is not the
	// product's business — but a contact may only ever be enrolled by an automation whose
	// stored status says it is running.
	t.Run("ConcurrentUpdateAndPause", func(t *testing.T) {
		for i := 0; i < 5; i++ {
			t.Run(fmt.Sprintf("round-%d", i), func(t *testing.T) {
				automationID := activateLive(t, fmt.Sprintf("concurrent round %d", i))

				edited := newAutomation("edited concurrently")
				edited["id"] = automationID
				// A changed event kind is a changed trigger, which is what makes Update
				// regenerate the DDL and collide with the drop.
				edited["trigger"] = map[string]interface{}{
					"event_kind": eventKind,
					"frequency":  "once",
					"conditions": map[string]interface{}{
						"kind": "leaf",
						"leaf": map[string]interface{}{
							"table": "contacts",
							"contacts": map[string]interface{}{
								"filters": []map[string]interface{}{{
									"field_name":    "country",
									"field_type":    "string",
									"operator":      "equals",
									"string_values": []string{"US"},
								}},
							},
						},
					},
				}

				var wg sync.WaitGroup
				wg.Add(2)
				go func() {
					defer wg.Done()
					resp, updateErr := client.UpdateAutomation(map[string]interface{}{
						"workspace_id": workspaceID,
						"automation":   edited,
					})
					if updateErr == nil {
						resp.Body.Close()
					}
				}()
				go func() {
					defer wg.Done()
					resp, pauseErr := client.PauseAutomation(map[string]interface{}{
						"workspace_id":  workspaceID,
						"automation_id": automationID,
					})
					if pauseErr == nil {
						resp.Body.Close()
					}
				}()
				wg.Wait()

				email := newEmail()
				fire(t, email)

				enrollment, findErr := factory.FindContactAutomation(workspaceID, automationID, email)
				require.NoError(t, findErr)

				if storedStatus(t, automationID) == string(domain.AutomationStatusLive) {
					assert.NotNil(t, enrollment, "a live automation must still enrol after the race")
					return
				}
				assert.Nil(t, enrollment, "an automation that is not live may not enrol, whatever the trigger's state")
				assert.Equal(t, 0, startEvents(t, automationID))
				assert.Equal(t, 0, triggerLogRows(t, automationID))
			})
		}
	})
}
