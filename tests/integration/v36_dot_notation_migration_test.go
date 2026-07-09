package integration

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/app"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/migrations"
	"github.com/Notifuse/notifuse/internal/service"
	"github.com/Notifuse/notifuse/tests/testutil"
)

// TestV36DotNotationDataMigration verifies the two v36 data-migration behaviors that the
// sqlmock unit tests only cover at the "statements were issued" level: against a real
// Postgres it seeds legacy suffixed data, runs the migration, and asserts the data ended up
// correctly renamed.
//
//   - contact_timeline rows carrying a legacy kind (click_email / open_email /
//     insert_message_history) are rewritten to the dotted kind; unrelated kinds are untouched.
//   - a stored segment filtering on the legacy kind has BOTH its tree and its compiled
//     generated_sql/generated_args rewritten (recomputation runs the compiled query, whose
//     bound args embed the old literal, so the tree alone is not enough).
func TestV36DotNotationDataMigration(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	factory := suite.DataFactory

	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	workspaceID := workspace.ID

	workspaceDB, err := factory.GetWorkspaceDB(workspaceID)
	require.NoError(t, err)

	email := "v36-migrate@example.com"
	_, err = factory.CreateContact(workspaceID, testutil.WithContactEmail(email))
	require.NoError(t, err)

	// Seed legacy timeline rows plus one dot-notation row (list.subscribed) that must NOT change.
	require.NoError(t, factory.CreateContactTimelineEvent(workspaceID, email, "click_email", map[string]interface{}{"n": 1}))
	require.NoError(t, factory.CreateContactTimelineEvent(workspaceID, email, "open_email", map[string]interface{}{"n": 1}))
	require.NoError(t, factory.CreateContactTimelineEvent(workspaceID, email, "insert_message_history", map[string]interface{}{"n": 1}))
	require.NoError(t, factory.CreateContactTimelineEvent(workspaceID, email, "list.subscribed", map[string]interface{}{"n": 1}))

	// Seed a live segment that filters on the legacy click_email kind, compiling its stored
	// query exactly as the service would have — so generated_args embeds "click_email".
	legacyTree := &domain.TreeNode{
		Kind: "branch",
		Branch: &domain.TreeNodeBranch{
			Operator: "and",
			Leaves: []*domain.TreeNode{
				{Kind: "leaf", Leaf: &domain.TreeNodeLeaf{
					Source:          "contact_timeline",
					ContactTimeline: &domain.ContactTimelineCondition{Kind: "click_email", CountOperator: "at_least", CountValue: 1},
				}},
			},
		},
	}
	segmentID := seedLegacySegment(t, workspaceDB, legacyTree)

	// Seed a PAUSED automation whose trigger_config conditions and filter-node conditions both
	// reference legacy kinds (branch/filter node conditions reuse the segment tree and are
	// evaluated at runtime via the query builder). Paused (not live) exercises that v36
	// rewrites embedded kinds for non-live automations too — otherwise re-activating it later
	// would evaluate the condition against a kind that no longer exists.
	automationID := seedLegacyAutomation(t, workspaceDB)

	// Run v36 exactly as the migration manager does (each workspace migration in its own tx;
	// v36 uses SAVEPOINTs, which require a transaction).
	tx, err := workspaceDB.BeginTx(context.Background(), nil)
	require.NoError(t, err)
	if mErr := (&migrations.V36Migration{}).UpdateWorkspace(context.Background(), &config.Config{}, workspace, tx); mErr != nil {
		_ = tx.Rollback()
		t.Fatalf("v36 UpdateWorkspace failed: %v", mErr)
	}
	require.NoError(t, tx.Commit())

	t.Run("timeline rows are renamed", func(t *testing.T) {
		for _, tc := range []struct{ oldKind, newKind string }{
			{"click_email", "email.clicked"},
			{"open_email", "email.opened"},
			{"insert_message_history", "email.sent"},
		} {
			renamed, err := factory.GetContactTimelineEvents(workspaceID, email, tc.newKind)
			require.NoError(t, err)
			assert.Len(t, renamed, 1, "expected one %q row after migration", tc.newKind)

			old, err := factory.GetContactTimelineEvents(workspaceID, email, tc.oldKind)
			require.NoError(t, err)
			assert.Empty(t, old, "no %q rows should remain", tc.oldKind)
		}

		// A non-message_history dot kind must be left untouched.
		unchanged, err := factory.GetContactTimelineEvents(workspaceID, email, "list.subscribed")
		require.NoError(t, err)
		assert.Len(t, unchanged, 1, "list.subscribed must be untouched")
	})

	t.Run("segment tree and compiled query are rewritten", func(t *testing.T) {
		var treeJSON, argsJSON []byte
		var genSQL sql.NullString
		err := workspaceDB.QueryRowContext(context.Background(),
			`SELECT tree, generated_sql, generated_args FROM segments WHERE id = $1`, segmentID).
			Scan(&treeJSON, &genSQL, &argsJSON)
		require.NoError(t, err)

		var tree domain.TreeNode
		require.NoError(t, json.Unmarshal(treeJSON, &tree))
		require.NotNil(t, tree.Branch)
		require.Len(t, tree.Branch.Leaves, 1)
		assert.Equal(t, "email.clicked", tree.Branch.Leaves[0].Leaf.ContactTimeline.Kind,
			"stored tree kind must be renamed")

		// The compiled query's bound args are what recomputation runs, so the old literal must
		// be gone from them and the new one present.
		assert.Contains(t, string(argsJSON), "email.clicked", "generated_args must bind the new kind")
		assert.NotContains(t, string(argsJSON), "click_email", "generated_args must not bind the old kind")
	})

	t.Run("automation trigger_config and node conditions are rewritten", func(t *testing.T) {
		var triggerConfig, nodes []byte
		err := workspaceDB.QueryRowContext(context.Background(),
			`SELECT trigger_config, nodes FROM automations WHERE id = $1`, automationID).
			Scan(&triggerConfig, &nodes)
		require.NoError(t, err)

		// trigger_config.conditions referenced click_email; the filter node referenced open_email.
		assert.Contains(t, string(triggerConfig), "email.clicked", "trigger conditions must be renamed")
		assert.NotContains(t, string(triggerConfig), "click_email")
		assert.Contains(t, string(nodes), "email.opened", "filter node condition must be renamed")
		assert.NotContains(t, string(nodes), "open_email")
	})
}

// seedLegacyAutomation inserts a live automation whose stored trigger_config conditions and
// filter-node conditions reference legacy timeline kinds, and returns its id.
func seedLegacyAutomation(t *testing.T, workspaceDB *sql.DB) string {
	t.Helper()

	automationID := "autolegacy"
	// contact.updated base kind (so no trigger DDL regen), with a trigger-level condition on
	// click_email and a filter node condition on open_email.
	triggerConfig := []byte(`{"event_kind":"contact.updated","frequency":"every_time","conditions":` +
		`{"kind":"leaf","leaf":{"source":"contact_timeline","contact_timeline":{"kind":"click_email","count_operator":"at_least","count_value":1}}}}`)
	nodes := []byte(`[{"id":"n1","automation_id":"autolegacy","type":"filter","config":{"conditions":` +
		`{"kind":"leaf","leaf":{"source":"contact_timeline","contact_timeline":{"kind":"open_email","count_operator":"at_least","count_value":1}}}},` +
		`"position":{"x":0,"y":0}}]`)

	_, err := workspaceDB.ExecContext(context.Background(), `
		INSERT INTO automations (id, workspace_id, name, status, exit_on_reply,
			trigger_config, root_node_id, nodes)
		VALUES ($1, 'ws', 'Legacy Automation', 'paused', false, $2, 'n1', $3)`,
		automationID, triggerConfig, nodes)
	require.NoError(t, err)

	// Sanity: preconditions hold before migration.
	require.Contains(t, string(triggerConfig), "click_email")
	require.Contains(t, string(nodes), "open_email")
	return automationID
}

// seedLegacySegment inserts a segment whose stored tree and compiled generated_sql/args are
// built exactly as the service would have before the rename, and returns its id.
func seedLegacySegment(t *testing.T, workspaceDB *sql.DB, tree *domain.TreeNode) string {
	t.Helper()

	sqlQuery, args, err := service.NewQueryBuilder().BuildSQL(tree)
	require.NoError(t, err)

	treeMap, err := tree.ToMapOfAny()
	require.NoError(t, err)
	treeJSON, err := json.Marshal(treeMap)
	require.NoError(t, err)
	argsJSON, err := json.Marshal(args)
	require.NoError(t, err)

	segmentID := "seglegacy"
	_, err = workspaceDB.ExecContext(context.Background(), `
		INSERT INTO segments (id, name, color, tree, timezone, version, status,
			generated_sql, generated_args, recompute_after, db_created_at, db_updated_at)
		VALUES ($1, 'Legacy Clickers', '#FF5733', $2, 'UTC', 1, 'active', $3, $4, NULL,
			CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		segmentID, treeJSON, sqlQuery, argsJSON)
	require.NoError(t, err)

	// Sanity: the seeded compiled query must actually embed the legacy literal.
	require.Contains(t, string(argsJSON), "click_email", "seed precondition: legacy args embed click_email")
	return segmentID
}
