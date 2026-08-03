package integration

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
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

// TestV37KindWideningMigration exercises the two v37 workspace behaviors that the sqlmock unit
// tests only cover at the "statements were issued" level. A freshly created workspace already
// has the widened column and repaired segments, so both are first reverted to their pre-37
// state and the migration is then run against real Postgres.
//
//   - contact_timeline.kind is widened, so the custom_events trigger can write
//     'custom_event.<event_name>' for an event name up to the 100 characters the API accepts.
//   - a stored segment whose compiled query still splices a timeline change key into the SQL
//     text is recompiled to the parameterized form, without waiting to be re-saved.
func TestV37KindWideningMigration(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	factory := suite.DataFactory
	ctx := context.Background()

	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)

	workspaceDB, err := factory.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)

	email := "v37@example.com"
	_, err = factory.CreateContact(workspace.ID, testutil.WithContactEmail(email))
	require.NoError(t, err)

	// Put the column back to its pre-37 width.
	_, err = workspaceDB.ExecContext(ctx,
		`ALTER TABLE contact_timeline ALTER COLUMN kind TYPE VARCHAR(50)`)
	require.NoError(t, err)

	longName := strings.Repeat("a", 100) // the maximum domain.CustomEvent.Validate accepts
	insertLongEvent := func(externalID string) error {
		_, execErr := workspaceDB.ExecContext(ctx, `
			INSERT INTO custom_events (event_name, external_id, email, properties, occurred_at, source)
			VALUES ($1, $2, $3, '{}', NOW(), 'test')`, longName, externalID, email)
		return execErr
	}

	t.Run("the pre-migration column rejects a long event name", func(t *testing.T) {
		// Guards the migration's premise: without the widening the AFTER INSERT trigger
		// overflows and takes the whole custom_events insert down with it.
		err := insertLongEvent("before")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "too long")
	})

	// Seed a segment holding the pre-fix compiled query: the change key spliced into the SQL
	// text rather than bound. This is what a segment saved before the upgrade looks like.
	vulnerableSQL := "SELECT email FROM contacts WHERE (SELECT COUNT(*) FROM contact_timeline ct " +
		"WHERE ct.email = contacts.email AND ct.kind = $1 AND ct.changes->'goal_type'->>'new' = $2) >= $3"
	segmentID := seedV37Segment(t, workspaceDB, vulnerableSQL,
		domain.JSONArray{"custom_event.shopify.order", "purchase", 1})

	// A second segment whose tree cannot compile: the migration must skip it rather than blank
	// its stored query or abort the workspace (which would block server startup).
	brokenID := seedV37BrokenSegment(t, workspaceDB, vulnerableSQL)

	require.NoError(t, (&migrations.V37Migration{}).UpdateWorkspace(ctx, &config.Config{}, workspace, workspaceDB))

	t.Run("a 100 character event name is recorded whole after the migration", func(t *testing.T) {
		require.NoError(t, insertLongEvent("after"))

		var kind string
		require.NoError(t, workspaceDB.QueryRowContext(ctx,
			`SELECT kind FROM contact_timeline WHERE email = $1 AND entity_type = 'custom_event'`,
			email).Scan(&kind))
		assert.Equal(t, "custom_event."+longName, kind, "the kind must not be truncated")
	})

	t.Run("the stored segment query is recompiled to the parameterized form", func(t *testing.T) {
		var genSQL sql.NullString
		var argsJSON []byte
		require.NoError(t, workspaceDB.QueryRowContext(ctx,
			`SELECT generated_sql, generated_args FROM segments WHERE id = $1`, segmentID).
			Scan(&genSQL, &argsJSON))

		assert.NotContains(t, genSQL.String, "ct.changes->'",
			"the change key must no longer be spliced into the SQL text")
		assert.Contains(t, genSQL.String, "ct.changes->$2->>'new'",
			"the change key must be bound as an argument")
		assert.Contains(t, string(argsJSON), "goal_type",
			"the key must now travel as an argument")

		// The repaired query must still run, and still mean the same thing.
		var args []interface{}
		require.NoError(t, json.Unmarshal(argsJSON, &args))
		rows, err := workspaceDB.QueryContext(ctx, genSQL.String, args...)
		require.NoError(t, err, "the recompiled query must execute")
		_ = rows.Close()
	})

	t.Run("a segment whose tree cannot compile keeps its stored query", func(t *testing.T) {
		var genSQL sql.NullString
		require.NoError(t, workspaceDB.QueryRowContext(ctx,
			`SELECT generated_sql FROM segments WHERE id = $1`, brokenID).Scan(&genSQL))
		assert.Equal(t, vulnerableSQL, genSQL.String,
			"an uncompilable tree must be left alone, not blanked")
	})

	t.Run("re-running the migration is a no-op", func(t *testing.T) {
		var before string
		require.NoError(t, workspaceDB.QueryRowContext(ctx,
			`SELECT generated_sql FROM segments WHERE id = $1`, segmentID).Scan(&before))

		require.NoError(t, (&migrations.V37Migration{}).UpdateWorkspace(ctx, &config.Config{}, workspace, workspaceDB))

		var after string
		require.NoError(t, workspaceDB.QueryRowContext(ctx,
			`SELECT generated_sql FROM segments WHERE id = $1`, segmentID).Scan(&after))
		assert.Equal(t, before, after)
	})
}

// seedV37Segment stores a segment whose tree compiles cleanly but whose generated_sql is the
// pre-fix interpolated form, i.e. a segment saved before the upgrade.
func seedV37Segment(t *testing.T, db *sql.DB, generatedSQL string, args domain.JSONArray) string {
	t.Helper()

	tree := &domain.TreeNode{
		Kind: "leaf",
		Leaf: &domain.TreeNodeLeaf{
			Source: "contact_timeline",
			ContactTimeline: &domain.ContactTimelineCondition{
				Kind:          "custom_event.shopify.order",
				CountOperator: "at_least",
				CountValue:    1,
				Filters: []*domain.DimensionFilter{
					{FieldName: "goal_type", FieldType: "string", Operator: "equals",
						StringValues: []string{"purchase"}},
				},
			},
		},
	}
	// Sanity-check the premise: the builder must now produce the parameterized form.
	compiled, _, err := service.NewQueryBuilder().BuildSQL(tree)
	require.NoError(t, err)
	require.Contains(t, compiled, "ct.changes->$2->>'new'")

	return insertV37Segment(t, db, "v37seg", tree, generatedSQL, args)
}

// seedV37BrokenSegment stores a segment whose tree is valid JSON but no longer compiles.
func seedV37BrokenSegment(t *testing.T, db *sql.DB, generatedSQL string) string {
	t.Helper()

	tree := &domain.TreeNode{
		Kind: "leaf",
		Leaf: &domain.TreeNodeLeaf{
			Source: "contact_timeline",
			ContactTimeline: &domain.ContactTimelineCondition{
				Kind:          "custom_event.shopify.order",
				CountOperator: "at_least",
				CountValue:    1,
				Filters: []*domain.DimensionFilter{
					// An unknown field type: decodes fine, fails to compile.
					{FieldName: "goal_type", FieldType: "nonsense", Operator: "equals",
						StringValues: []string{"purchase"}},
				},
			},
		},
	}
	return insertV37Segment(t, db, "v37broken", tree, generatedSQL, domain.JSONArray{"x"})
}

func insertV37Segment(t *testing.T, db *sql.DB, id string, tree *domain.TreeNode, generatedSQL string, args domain.JSONArray) string {
	t.Helper()

	treeJSON, err := json.Marshal(tree)
	require.NoError(t, err)
	argsJSON, err := json.Marshal(args)
	require.NoError(t, err)

	_, err = db.ExecContext(context.Background(), `
		INSERT INTO segments (
			id, name, color, tree, timezone, version, status,
			generated_sql, generated_args, recompute_after, db_created_at, db_updated_at
		) VALUES ($1, $2, '#FF5733', $3, 'UTC', 1, 'active', $4, $5, NULL, NOW(), NOW())`,
		id, "Segment "+id, treeJSON, generatedSQL, argsJSON)
	require.NoError(t, err)
	return id
}
