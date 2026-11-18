//go:build integration
// +build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/app"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/migrations"
	"github.com/Notifuse/notifuse/internal/service"
	"github.com/Notifuse/notifuse/tests/testutil"
	"github.com/google/uuid"
	shortuuid "github.com/lithammer/shortuuid/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestJSONPathFiltersMigration runs the v44 workspace update over a workspace whose segments and
// automation trigger still hold what the builder compiled before 44.0: a value inside a custom
// JSON column read as its JSON text (custom_json_1['plan']::text), quotes included. A text
// « equals » on it never matched and « not equals » always did; a JSON date never converted in an
// entry condition.
//
// Both kinds of compiled SQL are frozen where they are stored (segments.generated_sql, the
// trigger's function body), so the fixed builder reaches them only through the migration.
func TestJSONPathFiltersMigration(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	client := suite.APIClient
	factory := suite.DataFactory
	bg := context.Background()

	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	require.NoError(t, factory.AddUserToWorkspace(user.ID, workspace.ID, "owner"))
	require.NoError(t, client.Login(user.Email, "password"))
	client.SetWorkspaceID(workspace.ID)

	db, err := factory.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)

	tag := strings.ReplaceAll(uuid.New().String(), "-", "")[:8]

	// --- contacts ----------------------------------------------------------------------

	withJSON := func(t *testing.T, name, value string) string {
		t.Helper()
		email := fmt.Sprintf("jsonpath-%s-%s@example.com", name, tag)
		_, err := factory.CreateContact(workspace.ID, testutil.WithContactEmail(email))
		require.NoError(t, err)
		_, err = db.ExecContext(bg, `UPDATE contacts SET custom_json_1 = $2::jsonb WHERE email = $1`, email, value)
		require.NoError(t, err)
		return email
	}
	gold := withJSON(t, "gold", `{"plan":"gold"}`)
	silver := withJSON(t, "silver", `{"plan":"silver"}`)

	// --- segments holding the old SQL ---------------------------------------------------

	planTree := func(operator string) *domain.TreeNode {
		return &domain.TreeNode{
			Kind: "leaf",
			Leaf: &domain.TreeNodeLeaf{
				Source: "contacts",
				Contact: &domain.ContactCondition{Filters: []*domain.DimensionFilter{{
					FieldName:    "custom_json_1",
					FieldType:    "string",
					Operator:     operator,
					JSONPath:     []string{"plan"},
					StringValues: []string{"gold"},
				}}},
			},
		}
	}
	// What the builder compiled before 44.0: nothing generates it any more.
	oldForm := func(sql string) string {
		return strings.ReplaceAll(sql, "(custom_json_1['plan'] #>> '{}')", "custom_json_1['plan']::text")
	}
	insertOldSegment := func(t *testing.T, id, operator string) {
		t.Helper()
		tree := planTree(operator)
		current, args, err := service.NewQueryBuilder().BuildSQL(tree)
		require.NoError(t, err)
		old := oldForm(current)
		require.NotEqual(t, current, old, "the old form must differ from what the builder emits")
		treeJSON, err := json.Marshal(tree)
		require.NoError(t, err)
		argsJSON, err := json.Marshal(args)
		require.NoError(t, err)
		_, err = db.ExecContext(bg, `
			INSERT INTO segments (id, name, color, tree, timezone, version, status, generated_sql, generated_args, db_created_at, db_updated_at)
			VALUES ($1, $1, '#1677ff', $2, 'UTC', 1, 'active', $3, $4, NOW(), NOW())`,
			id, treeJSON, old, argsJSON)
		require.NoError(t, err)
	}
	equalsID := "jpeq" + tag
	notEqualsID := "jpne" + tag
	insertOldSegment(t, equalsID, "equals")
	insertOldSegment(t, notEqualsID, "not_equals")

	// storedMembers runs a segment's stored query, as a build does, over this test's contacts.
	storedMembers := func(t *testing.T, id string) []string {
		t.Helper()
		var sqlText string
		var argsJSON []byte
		require.NoError(t, db.QueryRowContext(bg, `SELECT generated_sql, generated_args FROM segments WHERE id = $1`, id).
			Scan(&sqlText, &argsJSON))
		var args []interface{}
		require.NoError(t, json.Unmarshal(argsJSON, &args))
		rows, err := db.QueryContext(bg, sqlText, args...)
		require.NoError(t, err, "stored query: %s", sqlText)
		defer rows.Close()
		var emails []string
		for rows.Next() {
			var email string
			require.NoError(t, rows.Scan(&email))
			if email == gold || email == silver {
				emails = append(emails, email)
			}
		}
		require.NoError(t, rows.Err())
		sort.Strings(emails)
		return emails
	}
	recomputeAfter := func(t *testing.T, id string) *time.Time {
		t.Helper()
		var at *time.Time
		require.NoError(t, db.QueryRowContext(bg, `SELECT recompute_after FROM segments WHERE id = $1`, id).Scan(&at))
		return at
	}

	// The defect, as stored: « equals gold » matches nobody, « not equals gold » everybody, and
	// the memberships say so.
	require.Empty(t, storedMembers(t, equalsID))
	require.Equal(t, sortedPair(gold, silver), storedMembers(t, notEqualsID))
	addMember(t, db, gold, notEqualsID, 1)
	addMember(t, db, silver, notEqualsID, 1)

	// --- an automation whose trigger holds the old body --------------------------------

	marker, err := factory.CreateList(workspace.ID)
	require.NoError(t, err)
	post := func(t *testing.T, path string, body map[string]interface{}) (int, string) {
		t.Helper()
		resp, err := client.Post(path, body)
		require.NoError(t, err)
		raw, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		return resp.StatusCode, string(raw)
	}
	automationID := shortuuid.New()
	triggerNodeID := shortuuid.New()
	addNodeID := shortuuid.New()
	status, body := post(t, "/api/automations.create", map[string]interface{}{
		"workspace_id": workspace.ID,
		"automation": map[string]interface{}{
			"id": automationID, "workspace_id": workspace.ID, "name": "JSON date", "status": "draft",
			"trigger": map[string]interface{}{
				"event_kind": "email.clicked", "frequency": "once",
				"conditions": map[string]interface{}{
					"kind": "leaf",
					"leaf": map[string]interface{}{
						"source": "contacts",
						"contact": map[string]interface{}{"filters": []map[string]interface{}{{
							"field_name": "custom_json_1", "field_type": "time", "operator": "after_date",
							"json_path": []string{"renewed_at"}, "string_values": []string{"2026-01-01T00:00:00Z"},
						}}},
					},
				},
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
					"config":   map[string]interface{}{"list_id": marker.ID, "status": "active"},
					"position": map[string]interface{}{"x": 0, "y": 100},
				},
			},
			"stats": map[string]interface{}{"enrolled": 0, "completed": 0, "exited": 0, "failed": 0},
		},
	})
	require.Equal(t, http.StatusCreated, status, "automations.create: %s", body)
	status, body = post(t, "/api/automations.activate", map[string]interface{}{
		"workspace_id": workspace.ID, "automation_id": automationID,
	})
	require.Equal(t, http.StatusOK, status, "automations.activate: %s", body)

	functionName := strings.ToLower("automation_trigger_" + strings.ReplaceAll(automationID, "-", ""))
	functionSource := func(t *testing.T) string {
		t.Helper()
		var def string
		require.NoError(t, db.QueryRowContext(bg,
			`SELECT pg_get_functiondef(oid) FROM pg_proc WHERE proname = $1`, functionName).Scan(&def))
		return def
	}
	// Turn the installed function back into what the generator wrote before 44.0.
	def := functionSource(t)
	require.Contains(t, def, "(custom_json_1['renewed_at'] #>> '{}')")
	_, err = db.ExecContext(bg, strings.ReplaceAll(def, "(custom_json_1['renewed_at'] #>> '{}')", "custom_json_1['renewed_at']::text"))
	require.NoError(t, err)
	require.Contains(t, functionSource(t), "custom_json_1['renewed_at']::text", "the old body must be installed before the migration")

	// --- the v44 workspace update ------------------------------------------------------

	runV44 := func(t *testing.T) {
		t.Helper()
		// A workspace migration runs in its own transaction, which the trigger step's
		// savepoints need.
		tx, err := db.BeginTx(bg, nil)
		require.NoError(t, err)
		if mErr := (&migrations.V44Migration{}).UpdateWorkspace(bg, &config.Config{}, workspace, tx); mErr != nil {
			_ = tx.Rollback()
			t.Fatalf("v44 UpdateWorkspace failed: %v", mErr)
		}
		require.NoError(t, tx.Commit())
	}
	runV44(t)

	// What cmd/regen-json-filters checks before replacing anything.
	var at44 bool
	require.NoError(t, db.QueryRowContext(bg,
		`SELECT to_regprocedure('automation_enroll_contact(varchar,varchar,varchar,varchar,jsonb)') IS NOT NULL`).Scan(&at44))
	require.True(t, at44, "the command's version check must recognise a database at 44")

	t.Run("the segments get the fixed query and are marked for a rebuild", func(t *testing.T) {
		assert.Equal(t, []string{gold}, storedMembers(t, equalsID))
		assert.Equal(t, []string{silver}, storedMembers(t, notEqualsID))
		assert.NotNil(t, recomputeAfter(t, equalsID))
		assert.NotNil(t, recomputeAfter(t, notEqualsID))
	})

	// The recompute check picks the marked segments, their builds move the contacts both ways,
	// and a segment without relative dates comes out of its build unmarked.
	t.Run("the rebuild moves the contacts and clears the mark", func(t *testing.T) {
		taskRepo := suite.ServerManager.GetApp().GetTaskRepository()
		members := func(t *testing.T, id string) []string {
			t.Helper()
			// Every version: a member the rebuild did not rewrite must be gone, not merely stale.
			rows, err := db.QueryContext(bg, `SELECT email FROM contact_segments WHERE segment_id = $1`, id)
			require.NoError(t, err)
			defer rows.Close()
			var emails []string
			for rows.Next() {
				var email string
				require.NoError(t, rows.Scan(&email))
				if email == gold || email == silver {
					emails = append(emails, email)
				}
			}
			require.NoError(t, rows.Err())
			sort.Strings(emails)
			return emails
		}

		runRecomputeCheck(t, bg, client, taskRepo, workspace.ID)
		for _, id := range []string{equalsID, notEqualsID} {
			builds := buildTasks(t, bg, taskRepo, workspace.ID, id)
			require.Len(t, builds, 1, "the check must start one build for %s", id)
			executeTask(t, client, workspace.ID, builds[0].ID)
		}

		assert.Equal(t, []string{gold}, members(t, equalsID))
		assert.Equal(t, []string{silver}, members(t, notEqualsID), "gold leaves the « not equals gold » segment")
		assert.Nil(t, recomputeAfter(t, equalsID), "a segment without relative dates is not rebuilt every day")
		assert.Nil(t, recomputeAfter(t, notEqualsID))
	})

	t.Run("the trigger body reads the JSON date and enrols a later one", func(t *testing.T) {
		src := functionSource(t)
		assert.Contains(t, src, "(custom_json_1['renewed_at'] #>> '{}')")
		assert.NotContains(t, src, "['renewed_at']::text")

		later := withJSON(t, "later", `{"renewed_at":"2026-03-01"}`)
		earlier := withJSON(t, "earlier", `{"renewed_at":"2025-06-01"}`)
		invalid := withJSON(t, "invalid", `{"renewed_at":"2026-02-30"}`)
		for _, email := range []string{later, earlier, invalid} {
			require.NoError(t, factory.CreateContactTimelineEvent(workspace.ID, email, "email.clicked",
				map[string]interface{}{"at": time.Now().UTC().Format(time.RFC3339)}),
				"the write of %s must not fail", email)
		}
		require.NotNil(t, waitForEnrollment(t, factory, workspace.ID, automationID, later, 3*time.Second))
		for _, email := range []string{earlier, invalid} {
			ca, err := factory.FindContactAutomation(workspace.ID, automationID, email)
			require.NoError(t, err)
			assert.Nil(t, ca, "%s must not enrol", email)
		}
	})

	t.Run("a rerun changes nothing", func(t *testing.T) {
		runV44(t)
		assert.Nil(t, recomputeAfter(t, equalsID), "an unchanged query is not marked again")
		assert.Nil(t, recomputeAfter(t, notEqualsID))
	})
}

func sortedPair(a, b string) []string {
	pair := []string{a, b}
	sort.Strings(pair)
	return pair
}
