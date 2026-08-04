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

// v37TriggerState is the part of a trigger's catalog entry that must survive the migration
// untouched: its exact definition and whether it is enabled.
type v37TriggerState struct {
	Definition string
	Enabled    string
}

// TestV37KindWideningWithDependentTriggers covers the case that made v37 fatal in the field.
// PostgreSQL refuses ALTER COLUMN ... TYPE while any trigger depends on the column, and every
// live automation installs exactly such a trigger on contact_timeline: its WHEN clause reads
// NEW.kind. A workspace with one live automation therefore aborted the whole workspace
// migration, and because the version is only written after every workspace succeeds, the server
// failed to boot again on each restart.
//
// The migration has to drop those triggers, widen the column and put them back exactly as they
// were — including their enabled state — inside its own transaction, so a failure anywhere
// leaves the workspace with its automations intact.
func TestV37KindWideningWithDependentTriggers(t *testing.T) {
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

	email := "v37triggers@example.com"
	// Created before the automation trigger exists: contact.created would otherwise try to
	// enroll the contact into an automation that has no row in the automations table.
	_, err = factory.CreateContact(workspace.ID, testutil.WithContactEmail(email))
	require.NoError(t, err)

	// Put the column back to its pre-37 width.
	_, err = workspaceDB.ExecContext(ctx,
		`ALTER TABLE contact_timeline ALTER COLUMN kind TYPE VARCHAR(50)`)
	require.NoError(t, err)

	// The real thing: the DDL a live automation installs, straight from the generator the
	// automation service uses, so this test tracks production and not a lookalike.
	automation := &domain.Automation{
		ID:         "98392e3e-98e4-47aa-b8c6-b95175ad5ba3",
		Name:       "contact created",
		Status:     domain.AutomationStatusLive,
		RootNodeID: "root-node",
		Trigger: &domain.TimelineTriggerConfig{
			EventKind: "contact.created",
			Frequency: domain.TriggerFrequencyEveryTime,
		},
	}
	triggerSQL, err := service.NewAutomationTriggerGenerator(service.NewQueryBuilder()).Generate(automation)
	require.NoError(t, err)
	_, err = workspaceDB.ExecContext(ctx, triggerSQL.FunctionBody)
	require.NoError(t, err)
	_, err = workspaceDB.ExecContext(ctx, triggerSQL.TriggerDDL)
	require.NoError(t, err)

	// A probe trigger whose WHEN clause also reads kind, but which records into a table this
	// test owns — the only way to prove a recreated trigger still fires.
	_, err = workspaceDB.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS v37_probe (kind VARCHAR(150));
		CREATE OR REPLACE FUNCTION v37_probe_fn() RETURNS TRIGGER AS $$
		BEGIN
			INSERT INTO v37_probe (kind) VALUES (NEW.kind);
			RETURN NEW;
		END;
		$$ LANGUAGE plpgsql;
		CREATE TRIGGER v37_probe_trigger AFTER INSERT ON contact_timeline
			FOR EACH ROW WHEN (NEW.kind LIKE 'custom_event.%')
			EXECUTE FUNCTION v37_probe_fn();

		-- A deliberately disabled trigger: recreating it from its definition must not quietly
		-- switch it back on.
		CREATE TRIGGER v37_disabled_trigger AFTER INSERT ON contact_timeline
			FOR EACH ROW WHEN (NEW.kind = 'contact.created')
			EXECUTE FUNCTION v37_probe_fn();
		ALTER TABLE contact_timeline DISABLE TRIGGER v37_disabled_trigger;

		-- A column dependency with no WHEN clause at all: UPDATE OF kind blocks the ALTER just
		-- the same, so matching on the WHEN clause alone would miss it.
		CREATE TRIGGER v37_update_of_kind AFTER UPDATE OF kind ON contact_timeline
			FOR EACH ROW EXECUTE FUNCTION v37_probe_fn();
	`)
	require.NoError(t, err)

	before := v37TriggerStates(t, workspaceDB)
	require.Contains(t, before, triggerSQL.TriggerName)
	require.Contains(t, before, "contact_timeline_queue_trigger",
		"the segment queue trigger must be part of the comparison, it has no column dependency")
	require.Equal(t, "D", before["v37_disabled_trigger"].Enabled)

	t.Run("the bare ALTER is refused while a trigger depends on the column", func(t *testing.T) {
		// The premise of the fix. Rolled back so the migration below starts from a pre-37 column.
		tx, txErr := workspaceDB.BeginTx(ctx, nil)
		require.NoError(t, txErr)
		defer func() { _ = tx.Rollback() }()

		_, alterErr := tx.ExecContext(ctx,
			`ALTER TABLE contact_timeline ALTER COLUMN kind TYPE VARCHAR(150)`)
		require.Error(t, alterErr)
		assert.Contains(t, alterErr.Error(), "cannot alter type of a column used in a trigger definition")
	})

	require.NoError(t, (&migrations.V37Migration{}).UpdateWorkspace(ctx, &config.Config{}, workspace, workspaceDB),
		"a workspace with a live automation must migrate, not abort startup")

	t.Run("the column is widened", func(t *testing.T) {
		var length int
		require.NoError(t, workspaceDB.QueryRowContext(ctx, `
			SELECT character_maximum_length FROM information_schema.columns
			WHERE table_name = 'contact_timeline' AND column_name = 'kind'`).Scan(&length))
		assert.Equal(t, 150, length)
	})

	t.Run("every trigger is restored exactly as it was", func(t *testing.T) {
		assert.Equal(t, before, v37TriggerStates(t, workspaceDB),
			"definitions and enabled state must be identical, including the disabled one")
	})

	t.Run("a recreated trigger still fires", func(t *testing.T) {
		longName := strings.Repeat("b", 100)
		_, insertErr := workspaceDB.ExecContext(ctx, `
			INSERT INTO custom_events (event_name, external_id, email, properties, occurred_at, source)
			VALUES ($1, 'fires', $2, '{}', NOW(), 'test')`, longName, email)
		require.NoError(t, insertErr, "the widened column must accept a 100 character event name")

		var probed string
		require.NoError(t, workspaceDB.QueryRowContext(ctx,
			`SELECT kind FROM v37_probe ORDER BY kind LIMIT 1`).Scan(&probed))
		assert.Equal(t, "custom_event."+longName, probed,
			"the recreated WHEN trigger must fire on the widened column")
	})

	t.Run("re-running the migration leaves the triggers untouched", func(t *testing.T) {
		// Trigger oids, not just definitions: a workspace that is already widened must not have
		// its automation triggers dropped and recreated on every retry of the migration.
		oidsBefore := v37TriggerOIDs(t, workspaceDB)

		require.NoError(t, (&migrations.V37Migration{}).UpdateWorkspace(ctx, &config.Config{}, workspace, workspaceDB))

		assert.Equal(t, before, v37TriggerStates(t, workspaceDB))
		assert.Equal(t, oidsBefore, v37TriggerOIDs(t, workspaceDB),
			"the triggers must be the same objects, not recreated equivalents")
	})
}

// v37TriggerStates reads every non-internal trigger on contact_timeline with its exact
// definition and enabled flag.
func v37TriggerStates(t *testing.T, db *sql.DB) map[string]v37TriggerState {
	t.Helper()

	rows, err := db.QueryContext(context.Background(), `
		SELECT tgname, tgenabled, pg_get_triggerdef(oid)
		FROM pg_trigger
		WHERE tgrelid = 'contact_timeline'::regclass AND NOT tgisinternal
		ORDER BY tgname`)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()

	states := map[string]v37TriggerState{}
	for rows.Next() {
		var name, enabled, def string
		require.NoError(t, rows.Scan(&name, &enabled, &def))
		states[name] = v37TriggerState{Definition: def, Enabled: enabled}
	}
	require.NoError(t, rows.Err())
	return states
}

// v37TriggerOIDs reads the catalog identity of every non-internal trigger on contact_timeline.
// A dropped and recreated trigger gets a new oid, so this distinguishes "left alone" from
// "rebuilt identically".
func v37TriggerOIDs(t *testing.T, db *sql.DB) map[string]int64 {
	t.Helper()

	rows, err := db.QueryContext(context.Background(), `
		SELECT tgname, oid FROM pg_trigger
		WHERE tgrelid = 'contact_timeline'::regclass AND NOT tgisinternal`)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()

	oids := map[string]int64{}
	for rows.Next() {
		var name string
		var oid int64
		require.NoError(t, rows.Scan(&name, &oid))
		oids[name] = oid
	}
	require.NoError(t, rows.Err())
	return oids
}
