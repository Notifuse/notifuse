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

	"github.com/lithammer/shortuuid/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/app"
	"github.com/Notifuse/notifuse/internal/migrations"
	"github.com/Notifuse/notifuse/tests/testutil"
)

// TestAutomationEmailClickTrigger verifies that automations triggered by an email
// click actually fire. The contact timeline records email clicks under the dot-notation
// kind "email.clicked" (uniformized with the console's trigger event kind), so the trigger
// WHEN clause matches the row directly.
//
// Two subtests:
//   - GeneratorFiresOnClick: a freshly-activated email.clicked automation enrolls the
//     contact when an email.clicked timeline row is inserted.
//   - MigrationHealsBrokenTrigger: an automation whose installed trigger still carries a
//     stale suffixed WHEN clause ("click_email") is regenerated to "email.clicked" by the
//     v36 workspace migration and then fires.
func TestAutomationEmailClickTrigger(t *testing.T) {
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

	// createLiveClickAutomation creates and activates an automation triggered by
	// email.clicked and returns its ID.
	createLiveClickAutomation := func(t *testing.T, name string) string {
		automationID := shortuuid.New()
		triggerNodeID := shortuuid.New()
		emailNodeID := shortuuid.New()

		createReq := map[string]interface{}{
			"workspace_id": workspaceID,
			"automation": map[string]interface{}{
				"id":           automationID,
				"workspace_id": workspaceID,
				"name":         name,
				"status":       "draft",
				"list_id":      list.ID,
				"trigger": map[string]interface{}{
					"event_kind": "email.clicked",
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
			},
		}

		resp, err := client.CreateAutomation(createReq)
		require.NoError(t, err)
		if resp.StatusCode != http.StatusCreated {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			t.Fatalf("CreateAutomation: expected 201, got %d: %s", resp.StatusCode, string(body))
		}
		resp.Body.Close()

		activateResp, err := client.ActivateAutomation(map[string]interface{}{
			"workspace_id":  workspaceID,
			"automation_id": automationID,
		})
		require.NoError(t, err)
		if activateResp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(activateResp.Body)
			activateResp.Body.Close()
			t.Fatalf("ActivateAutomation: expected 200, got %d: %s", activateResp.StatusCode, string(body))
		}
		activateResp.Body.Close()
		return automationID
	}

	triggerName := func(automationID string) string {
		// PostgreSQL folds unquoted identifiers to lower case, so the catalog name is the
		// lower-cased form of the mixed-case shortuuid used in the CREATE TRIGGER DDL.
		return strings.ToLower("automation_trigger_" + strings.ReplaceAll(automationID, "-", ""))
	}

	triggerDef := func(t *testing.T, automationID string) string {
		var def sql.NullString
		err := workspaceDB.QueryRowContext(context.Background(),
			`SELECT pg_get_triggerdef(oid) FROM pg_trigger WHERE tgname = $1 AND NOT tgisinternal`,
			triggerName(automationID)).Scan(&def)
		require.NoError(t, err, "trigger %s should exist", triggerName(automationID))
		return def.String
	}

	insertClick := func(t *testing.T, email string) {
		require.NoError(t, factory.CreateContactTimelineEvent(workspaceID, email, "email.clicked",
			map[string]interface{}{"clicked_at": time.Now().UTC().Format(time.RFC3339)}))
	}

	// runMigration executes v36 inside a transaction, mirroring how the migration manager
	// runs workspace migrations (it wraps each in a BeginTx). v36 uses SAVEPOINTs, which
	// require a transaction — running against the raw *sql.DB (autocommit) would error.
	runMigration := func(t *testing.T) error {
		tx, err := workspaceDB.BeginTx(context.Background(), nil)
		require.NoError(t, err)
		if mErr := (&migrations.V36Migration{}).UpdateWorkspace(context.Background(), &config.Config{}, workspace, tx); mErr != nil {
			_ = tx.Rollback()
			return mErr
		}
		return tx.Commit()
	}

	t.Run("GeneratorFiresOnClick", func(t *testing.T) {
		automationID := createLiveClickAutomation(t, "Email Click Fires E2E")

		// The freshly generated trigger must listen for the dot-notation timeline kind.
		def := triggerDef(t, automationID)
		assert.Contains(t, def, "email.clicked", "trigger WHEN must reference the timeline kind")
		assert.NotContains(t, def, "click_email", "trigger WHEN must not use the old suffixed kind")

		email := "email-click-fires@example.com"
		_, err := factory.CreateContact(workspaceID, testutil.WithContactEmail(email))
		require.NoError(t, err)

		insertClick(t, email)

		ca := waitForEnrollment(t, factory, workspaceID, automationID, email, 3*time.Second)
		require.NotNil(t, ca, "contact should be enrolled after an email click")
	})

	t.Run("MigrationHealsBrokenTrigger", func(t *testing.T) {
		automationID := createLiveClickAutomation(t, "Email Click Heal E2E")

		// Simulate a stale pre-uniformization trigger: re-install it with the old suffixed
		// WHEN clause ("click_email"), which no longer matches the dot-notation timeline rows.
		name := triggerName(automationID)
		_, err := workspaceDB.ExecContext(context.Background(), fmt.Sprintf(
			`DROP TRIGGER IF EXISTS %s ON contact_timeline;
			 CREATE TRIGGER %s AFTER INSERT ON contact_timeline FOR EACH ROW
			 WHEN (NEW.kind = 'click_email') EXECUTE FUNCTION %s()`, name, name, name))
		require.NoError(t, err)
		require.Contains(t, triggerDef(t, automationID), "click_email", "stale trigger should be installed")

		// Run the v36 workspace migration, which regenerates the trigger.
		require.NoError(t, runMigration(t))

		healed := triggerDef(t, automationID)
		assert.Contains(t, healed, "email.clicked", "migration should heal the WHEN clause")
		assert.NotContains(t, healed, "click_email", "migration should remove the old suffixed kind")

		// End-to-end: the healed trigger now enrolls on a click.
		email := "email-click-heal@example.com"
		_, err = factory.CreateContact(workspaceID, testutil.WithContactEmail(email))
		require.NoError(t, err)

		insertClick(t, email)

		ca := waitForEnrollment(t, factory, workspaceID, automationID, email, 3*time.Second)
		require.NotNil(t, ca, "contact should be enrolled after the migration heals the trigger")
	})

	// A live email automation that gained trigger-level Conditions after activation would
	// regenerate to a WHEN clause containing a subquery, which Postgres rejects. v36 must
	// skip it rather than error — an errored workspace migration aborts the whole run and
	// blocks server startup for the entire instance.
	t.Run("MigrationSkipsAutomationWithSubqueryConditions", func(t *testing.T) {
		automationID := createLiveClickAutomation(t, "Email Click Conditions E2E")

		// Inject a contact-source condition into the stored trigger config (as Update
		// would), without regenerating the installed trigger.
		_, err := workspaceDB.ExecContext(context.Background(),
			`UPDATE automations SET trigger_config = jsonb_set(trigger_config, '{conditions}', $1::jsonb) WHERE id = $2`,
			`{"kind":"leaf","leaf":{"source":"contacts","contact":{"filters":[{"field_name":"first_name","field_type":"string","operator":"equals","string_values":["x"]}]}}}`,
			automationID)
		require.NoError(t, err)

		// v36 must complete without error (the conditions automation is skipped).
		require.NoError(t, runMigration(t), "v36 must not abort on an automation with subquery conditions")

		// Its trigger is left intact (still present), not dropped.
		require.NotEmpty(t, triggerDef(t, automationID), "skipped automation keeps its trigger")
	})
}
