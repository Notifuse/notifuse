package integration

import (
	"context"
	"database/sql"
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
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/tests/testutil"
)

// TestAutomationTriggerConditions activates condition-bearing automations through the real
// API and lets PostgreSQL judge the DDL.
//
// It exists because the layer below cannot: a generator unit test compares the produced SQL
// against a string its own author wrote, so it is green on SQL the database refuses just as
// readily as on SQL it accepts. That is not a gap in those tests, it is structural. And it is
// how this shipped — trigger conditions were compiled into the CREATE TRIGGER ... WHEN clause
// as correlated subqueries, which PostgreSQL rejects at parse time (SQLSTATE 0A000, "cannot
// use subquery in trigger WHEN condition"), so an automation carrying conditions could never
// be activated in any workspace while every string assertion below kept passing. Conditions
// now live in the generated trigger function body, where subqueries are legal, and an
// install-time probe resolves their column references before any DDL runs.
func TestAutomationTriggerConditions(t *testing.T) {
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

	// --- condition tree builders -------------------------------------------------------

	stringFilter := func(field, value string) map[string]interface{} {
		return map[string]interface{}{
			"field_name":    field,
			"field_type":    "string",
			"operator":      "equals",
			"string_values": []string{value},
		}
	}

	contactsCondition := func(filters ...map[string]interface{}) map[string]interface{} {
		return map[string]interface{}{
			"kind": "leaf",
			"leaf": map[string]interface{}{
				"source":  "contacts",
				"contact": map[string]interface{}{"filters": filters},
			},
		}
	}

	// A filter the generator compiles happily and the database refuses: "country" is a
	// varchar column, so reading it as a date yields "country = '...'::timestamptz", for
	// which PostgreSQL has no operator. The field name and the field type both come from
	// the caller and nothing before the database cross-checks them.
	mistypedCondition := contactsCondition(map[string]interface{}{
		"field_name":    "country",
		"field_type":    "time",
		"operator":      "equals",
		"string_values": []string{"2020-01-01T00:00:00Z"},
	})

	// --- automation helpers ------------------------------------------------------------

	// newAutomation builds the payload of a draft automation whose trigger fires on
	// eventKind and carries conditions (nil for none).
	newAutomation := func(name, eventKind string, conditions map[string]interface{}) map[string]interface{} {
		automationID := shortuuid.New()
		triggerNodeID := shortuuid.New()
		emailNodeID := shortuuid.New()

		trigger := map[string]interface{}{
			"event_kind": eventKind,
			"frequency":  "once",
		}
		if conditions != nil {
			trigger["conditions"] = conditions
		}

		return map[string]interface{}{
			"id":           automationID,
			"workspace_id": workspaceID,
			"name":         name,
			"status":       "draft",
			"list_id":      list.ID,
			"trigger":      trigger,
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

	readBody := func(t *testing.T, resp *http.Response) string {
		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		require.NoError(t, readErr)
		return string(body)
	}

	createDraft := func(t *testing.T, automation map[string]interface{}) string {
		resp, createErr := client.CreateAutomation(map[string]interface{}{
			"workspace_id": workspaceID,
			"automation":   automation,
		})
		require.NoError(t, createErr)
		body := readBody(t, resp)
		require.Equalf(t, http.StatusCreated, resp.StatusCode, "CreateAutomation: %s", body)
		return automation["id"].(string)
	}

	// activate calls the activate endpoint and hands back what the API answered, so a
	// subtest can assert on a refusal as easily as on a success.
	activate := func(t *testing.T, automationID string) (int, string) {
		resp, activateErr := client.ActivateAutomation(map[string]interface{}{
			"workspace_id":  workspaceID,
			"automation_id": automationID,
		})
		require.NoError(t, activateErr)
		return resp.StatusCode, readBody(t, resp)
	}

	// activateLive creates the automation and activates it, failing the test unless the
	// API answered 200.
	activateLive := func(t *testing.T, automation map[string]interface{}) string {
		automationID := createDraft(t, automation)
		status, body := activate(t, automationID)
		require.Equalf(t, http.StatusOK, status, "ActivateAutomation: %s", body)
		return automationID
	}

	triggerName := func(automationID string) string {
		// PostgreSQL folds unquoted identifiers to lower case, so the catalog name is the
		// lower-cased form of the mixed-case shortuuid used in the CREATE TRIGGER DDL.
		return strings.ToLower("automation_trigger_" + strings.ReplaceAll(automationID, "-", ""))
	}

	triggerDef := func(t *testing.T, automationID string) string {
		var def sql.NullString
		queryErr := workspaceDB.QueryRowContext(ctx,
			`SELECT pg_get_triggerdef(oid) FROM pg_trigger WHERE tgname = $1 AND NOT tgisinternal`,
			triggerName(automationID)).Scan(&def)
		require.NoError(t, queryErr, "trigger %s should exist", triggerName(automationID))
		return def.String
	}

	functionSource := func(t *testing.T, automationID string) string {
		var src string
		queryErr := workspaceDB.QueryRowContext(ctx,
			`SELECT prosrc FROM pg_proc WHERE proname = $1`, triggerName(automationID)).Scan(&src)
		require.NoError(t, queryErr, "trigger function %s should exist", triggerName(automationID))
		return src
	}

	insertEvent := func(t *testing.T, email, kind string) {
		require.NoError(t, factory.CreateContactTimelineEvent(workspaceID, email, kind,
			map[string]interface{}{"at": time.Now().UTC().Format(time.RFC3339)}))
	}

	// enrollment reports the contact's enrollment in the automation, or nil when there is
	// none. The enrollment is written by automation_enroll_contact inside the INSERT that
	// fired the trigger, so once the insert has returned the answer is final and a
	// "not enrolled" assertion needs no waiting.
	enrollment := func(t *testing.T, automationID, email string) *domain.ContactAutomation {
		ca, getErr := factory.FindContactAutomation(workspaceID, automationID, email)
		require.NoError(t, getErr, "unexpected error reading enrollment")
		return ca
	}

	countContacts := func(t *testing.T) int {
		var count int
		require.NoError(t, workspaceDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM contacts`).Scan(&count))
		return count
	}

	// Every leaf source the condition compiler supports must survive the round trip through
	// PostgreSQL: the compiled expression belongs in the function body, and the WHEN clause
	// must stay free of the subqueries that made activation impossible.
	t.Run("LeafSourcesInstallInTheFunctionBody", func(t *testing.T) {
		sources := []struct {
			name          string
			conditions    map[string]interface{}
			bodyFragments []string
		}{
			{
				name:       "contacts",
				conditions: contactsCondition(stringFilter("country", "US")),
				bodyFragments: []string{
					"EXISTS (SELECT 1 FROM contacts WHERE email = NEW.email AND country = 'US')",
				},
			},
			{
				name: "contact_lists",
				conditions: map[string]interface{}{
					"kind": "leaf",
					"leaf": map[string]interface{}{
						"source": "contact_lists",
						"contact_list": map[string]interface{}{
							"operator": "in",
							"list_id":  list.ID,
							"status":   "active",
						},
					},
				},
				bodyFragments: []string{
					"EXISTS (SELECT 1 FROM contact_lists cl JOIN lists l ON cl.list_id = l.id WHERE cl.email = NEW.email",
					"cl.list_id = '" + list.ID + "'",
				},
			},
			{
				name: "contact_timeline",
				conditions: map[string]interface{}{
					"kind": "leaf",
					"leaf": map[string]interface{}{
						"source": "contact_timeline",
						"contact_timeline": map[string]interface{}{
							"kind":               "email.opened",
							"count_operator":     "at_least",
							"count_value":        2,
							"timeframe_operator": "anytime",
						},
					},
				},
				// A timeline leaf compiles to a scalar count comparison rather than an
				// EXISTS, so asserting on "EXISTS" alone would miss it entirely.
				bodyFragments: []string{
					"(SELECT COUNT(*) FROM contact_timeline ct WHERE ct.email = NEW.email AND ct.kind = 'email.opened') >= 2",
				},
			},
			{
				name: "custom_events_goals",
				conditions: map[string]interface{}{
					"kind": "leaf",
					"leaf": map[string]interface{}{
						"source": "custom_events_goals",
						"custom_events_goal": map[string]interface{}{
							"goal_type":          "purchase",
							"aggregate_operator": "sum",
							"operator":           "gte",
							"value":              100,
							"timeframe_operator": "anytime",
						},
					},
				},
				bodyFragments: []string{
					"EXISTS (SELECT 1 FROM custom_events ce WHERE ce.email = NEW.email",
					"GROUP BY ce.email HAVING COALESCE(SUM(ce.goal_value), 0) >= 100",
				},
			},
		}

		for _, source := range sources {
			source := source
			t.Run(source.name, func(t *testing.T) {
				automationID := activateLive(t, newAutomation(
					"Conditions "+source.name, "email.clicked", source.conditions))

				assert.NotContains(t, triggerDef(t, automationID), "(SELECT",
					"a subquery in the WHEN clause is what PostgreSQL rejects (SQLSTATE 0A000)")

				body := functionSource(t, automationID)
				assert.Contains(t, body, "IF (", "the condition must guard the enrollment call")
				for _, fragment := range source.bodyFragments {
					assert.Contains(t, body, fragment, "the compiled condition must be in the function body")
				}
			})
		}
	})

	// Installed and enforced are different claims: an installed trigger whose guard always
	// evaluates true enrolls everyone, and no assertion on the SQL text would notice.
	t.Run("ConditionFiltersEnrollment", func(t *testing.T) {
		automationID := activateLive(t, newAutomation(
			"Conditions Filter Country", "email.clicked", contactsCondition(stringFilter("country", "US"))))

		matching := "conditions-country-us@example.com"
		_, createErr := factory.CreateContact(workspaceID,
			testutil.WithContactEmail(matching), testutil.WithContactCountry("US"))
		require.NoError(t, createErr)

		other := "conditions-country-fr@example.com"
		_, createErr = factory.CreateContact(workspaceID,
			testutil.WithContactEmail(other), testutil.WithContactCountry("FR"))
		require.NoError(t, createErr)

		insertEvent(t, matching, "email.clicked")
		insertEvent(t, other, "email.clicked")

		require.NotNil(t, waitForEnrollment(t, factory, workspaceID, automationID, matching, 3*time.Second),
			"the contact the condition describes should be enrolled")
		assert.Nil(t, enrollment(t, automationID, other),
			"the contact the condition excludes should not be enrolled")
	})

	// The triggering event counts towards its own condition. An AFTER INSERT body already
	// sees the row that fired it, so "at least 2 email.opened" is satisfied by the second
	// open rather than the third. This is the intended product behaviour — the condition
	// describes the contact's state at the moment the automation considers them, including
	// the event that brought them to it — and it is pinned here because it is invisible in
	// the generated SQL and would otherwise drift the first time the guard is moved.
	t.Run("TimelineCountIncludesTheTriggeringEvent", func(t *testing.T) {
		automationID := activateLive(t, newAutomation(
			"Conditions Inclusive Count", "email.opened", map[string]interface{}{
				"kind": "leaf",
				"leaf": map[string]interface{}{
					"source": "contact_timeline",
					"contact_timeline": map[string]interface{}{
						"kind":               "email.opened",
						"count_operator":     "at_least",
						"count_value":        2,
						"timeframe_operator": "anytime",
					},
				},
			}))

		email := "conditions-inclusive-count@example.com"
		_, createErr := factory.CreateContact(workspaceID, testutil.WithContactEmail(email))
		require.NoError(t, createErr)

		insertEvent(t, email, "email.opened")
		require.Nil(t, enrollment(t, automationID, email),
			"the first open leaves the contact one short of the condition")

		insertEvent(t, email, "email.opened")
		assert.NotNil(t, waitForEnrollment(t, factory, workspaceID, automationID, email, 3*time.Second),
			"the second open is itself the second event counted, so it satisfies the condition")
	})

	// Before the fix a date-valued condition never reached the database at all: the
	// argument escaper had no case for time.Time and failed with "unsupported arg type
	// time.Time" while generating the SQL.
	t.Run("DateValuedConditionInstallsAndEvaluates", func(t *testing.T) {
		automationID := activateLive(t, newAutomation(
			"Conditions After Date", "email.clicked", contactsCondition(map[string]interface{}{
				"field_name":    "created_at",
				"field_type":    "time",
				"operator":      "after_date",
				"string_values": []string{"2020-01-01T00:00:00Z"},
			})))

		body := functionSource(t, automationID)
		assert.Contains(t, body, "created_at > '2020-01-01T00:00:00Z'::timestamptz",
			"the date value must reach the body as a typed literal")

		email := "conditions-after-date@example.com"
		_, createErr := factory.CreateContact(workspaceID, testutil.WithContactEmail(email))
		require.NoError(t, createErr)

		insertEvent(t, email, "email.clicked")

		assert.NotNil(t, waitForEnrollment(t, factory, workspaceID, automationID, email, 3*time.Second),
			"a contact created after the date should satisfy the condition")
	})

	// Filter values are caller input that reaches the function body verbatim, inside a
	// dollar-quoted CREATE FUNCTION. They must land as data.
	t.Run("AdversarialFilterValuesAreData", func(t *testing.T) {
		const (
			dollarQuote    = "$$"
			functionTag    = "$fn0$"
			sqlInjection   = "'); DROP TABLE contacts; --"
			injectionEmail = "conditions-injection@example.com"
		)

		contactsBefore := countContacts(t)

		automationID := activateLive(t, newAutomation(
			"Conditions Adversarial Values", "email.clicked", contactsCondition(
				stringFilter("custom_string_1", dollarQuote),
				stringFilter("custom_string_2", functionTag),
				stringFilter("custom_string_3", sqlInjection),
			)))

		body := functionSource(t, automationID)
		assert.Contains(t, body, "custom_string_1 = '"+dollarQuote+"'")
		assert.Contains(t, body, "custom_string_2 = '"+functionTag+"'")
		assert.Contains(t, body, "custom_string_3 = '''); DROP TABLE contacts; --'",
			"the quote in the value must be doubled, leaving it a string literal")

		// A second automation whose only filter is the injection payload, driven end to
		// end: the contact holding that exact string enrols, which is only possible if the
		// value was compared as data.
		roundTripID := activateLive(t, newAutomation(
			"Conditions Injection Round Trip", "email.clicked",
			contactsCondition(stringFilter("custom_string_1", sqlInjection))))

		_, createErr := factory.CreateContact(workspaceID,
			testutil.WithContactEmail(injectionEmail), testutil.WithContactCustomString1(sqlInjection))
		require.NoError(t, createErr)

		insertEvent(t, injectionEmail, "email.clicked")

		assert.NotNil(t, waitForEnrollment(t, factory, workspaceID, roundTripID, injectionEmail, 3*time.Second),
			"the contact whose field holds the payload should match the filter")

		var contactsRelation sql.NullString
		require.NoError(t, workspaceDB.QueryRowContext(ctx,
			`SELECT to_regclass('contacts')::text`).Scan(&contactsRelation))
		assert.True(t, contactsRelation.Valid, "the contacts table must still exist")
		assert.Equal(t, contactsBefore+1, countContacts(t),
			"only the contact this subtest created may have changed")
	})

	// The probe is why a condition the database cannot resolve is a 400 instead of a
	// workspace-wide outage: CREATE FUNCTION only syntax-checks a plpgsql body, so without
	// it the broken guard installs cleanly and then aborts every write to contact_timeline
	// — the table fed by contacts, lists, message history, custom events, segments and
	// inbound webhooks.
	t.Run("ConditionTheDatabaseRefusesIsRejectedAtActivation", func(t *testing.T) {
		automationID := createDraft(t, newAutomation(
			"Conditions Unresolvable", "email.clicked", mistypedCondition))

		status, body := activate(t, automationID)
		require.Equal(t, http.StatusBadRequest, status, "activation body: %s", body)
		assert.Contains(t, body, "invalid trigger conditions")
		assert.Contains(t, body, "operator does not exist",
			"the caller needs PostgreSQL's own reason to fix the condition")
		assert.NotContains(t, body, "Failed to activate automation",
			"the generic message says nothing about what is wrong")

		// Nothing was installed, so nothing has to be cleaned up either.
		var installed int
		require.NoError(t, workspaceDB.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM pg_trigger WHERE tgname = $1 AND NOT tgisinternal`,
			triggerName(automationID)).Scan(&installed))
		assert.Zero(t, installed, "a refused activation must not leave a trigger behind")

		// The point of the whole exercise: contact events still flow.
		email := "conditions-timeline-writable@example.com"
		_, createErr := factory.CreateContact(workspaceID, testutil.WithContactEmail(email))
		require.NoError(t, createErr)
		assert.NoError(t, factory.CreateContactTimelineEvent(workspaceID, email, "email.clicked",
			map[string]interface{}{"at": time.Now().UTC().Format(time.RFC3339)}),
			"contact_timeline must stay writable after a refused activation")
	})

	// Re-installing the trigger of a live automation drops the running one first. Both DROPs
	// and both CREATEs are one transaction, so a configuration the database refuses leaves
	// the automation running exactly the trigger it was running before — not a dropped
	// trigger and an automation that silently stops firing.
	//
	// The re-install goes through update rather than pause-then-activate: pausing drops the
	// trigger on purpose, which is not the case this protects.
	// A condition that cannot evaluate must not match — it must not abort the write.
	//
	// custom_json values are whatever the caller stored, so nothing guarantees a key holds
	// the same type across contacts. Read as a number, a contact holding a string used to
	// raise inside the trigger function, and because contact_timeline is written by triggers
	// on contacts, lists, message history, custom events, segments and inbound webhooks,
	// that contact could no longer be written to at all. The install probe cannot foresee
	// it: EXPLAIN plans without executing, and the failure depends on the row.
	t.Run("AnUnconvertibleJSONValueDoesNotBreakTheWrite", func(t *testing.T) {
		automationID := activateLive(t, newAutomation(
			"Conditions JSON Cast", "email.clicked",
			contactsCondition(map[string]interface{}{
				"field_name":    "custom_json_1",
				"field_type":    "number",
				"operator":      "gte",
				"json_path":     []string{"score"},
				"number_values": []float64{10},
			})))

		matching := "conditions-json-good@example.com"
		_, createErr := factory.CreateContact(workspaceID, testutil.WithContactEmail(matching))
		require.NoError(t, createErr)
		_, execErr := workspaceDB.ExecContext(ctx,
			`UPDATE contacts SET custom_json_1 = '{"score":42}'::jsonb WHERE email = $1`, matching)
		require.NoError(t, execErr)

		// The same key holding a string rather than a number.
		unconvertible := "conditions-json-bad@example.com"
		_, createErr = factory.CreateContact(workspaceID, testutil.WithContactEmail(unconvertible))
		require.NoError(t, createErr)
		_, execErr = workspaceDB.ExecContext(ctx,
			`UPDATE contacts SET custom_json_1 = '{"score":"abc"}'::jsonb WHERE email = $1`, unconvertible)
		require.NoError(t, execErr)

		insertEvent(t, matching, "email.clicked")

		// The write that used to fail with `invalid input syntax for type numeric`.
		require.NotPanics(t, func() { insertEvent(t, unconvertible, "email.clicked") },
			"a contact whose value does not convert must still be writable")

		require.NotNil(t, waitForEnrollment(t, factory, workspaceID, automationID, matching, 3*time.Second),
			"the contact whose value converts and matches should be enrolled")
		assert.Nil(t, enrollment(t, automationID, unconvertible),
			"a value that cannot be read as a number does not match")
	})

	t.Run("FailedReinstallKeepsTheWorkingTrigger", func(t *testing.T) {
		automation := newAutomation(
			"Conditions Reinstall", "email.clicked", contactsCondition(stringFilter("country", "US")))
		automationID := activateLive(t, automation)

		defBefore := triggerDef(t, automationID)
		bodyBefore := functionSource(t, automationID)

		// Same automation, live, with a condition the database will refuse.
		automation["status"] = "live"
		automation["trigger"].(map[string]interface{})["conditions"] = mistypedCondition

		resp, updateErr := client.UpdateAutomation(map[string]interface{}{
			"workspace_id": workspaceID,
			"automation":   automation,
		})
		require.NoError(t, updateErr)
		updateBody := readBody(t, resp)
		require.Equal(t, http.StatusBadRequest, resp.StatusCode, "update body: %s", updateBody)

		assert.Equal(t, defBefore, triggerDef(t, automationID),
			"the trigger the automation was running must survive byte-for-byte")
		assert.Equal(t, bodyBefore, functionSource(t, automationID),
			"and so must the function it executes")

		// Still running, not merely still installed.
		email := "conditions-reinstall-survivor@example.com"
		_, createErr := factory.CreateContact(workspaceID,
			testutil.WithContactEmail(email), testutil.WithContactCountry("US"))
		require.NoError(t, createErr)

		insertEvent(t, email, "email.clicked")

		assert.NotNil(t, waitForEnrollment(t, factory, workspaceID, automationID, email, 3*time.Second),
			"the surviving trigger should still enroll a matching contact")
	})
}
