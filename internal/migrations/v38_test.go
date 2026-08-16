package migrations

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/database/schema"
	"github.com/Notifuse/notifuse/internal/domain"
)

func TestV38Migration_Metadata(t *testing.T) {
	m := &V38Migration{}
	assert.Equal(t, 38.0, m.GetMajorVersion())
	assert.True(t, m.HasSystemUpdate())
	assert.True(t, m.HasWorkspaceUpdate())
	assert.False(t, m.ShouldRestartServer())
}

func TestV38Migration_IsRegistered(t *testing.T) {
	migration, ok := GetRegisteredMigration(38.0)
	require.True(t, ok, "v38 must be registered so it runs on startup")
	assert.IsType(t, &V38Migration{}, migration)
}

// Both statements must grant web_analytics and keep the two guards: the
// object check (a scalar would concatenate into an array and corrupt the row)
// and the "already granted" check (so a re-run cannot revoke a narrowed grant).
const (
	v38UserWorkspacesGrant = `(?s)UPDATE user_workspaces.*` +
		`"web_analytics": \{"read": true, "write": true\}.*` +
		`jsonb_typeof\(permissions\) = 'object'.*` +
		`NOT permissions \? 'web_analytics'`
	v38InvitationsGrant = `(?s)UPDATE workspace_invitations.*` +
		`"web_analytics": \{"read": true, "write": true\}.*` +
		`jsonb_typeof\(permissions\) = 'object'.*` +
		`NOT permissions \? 'web_analytics'`
)

func TestV38Migration_UpdateSystem(t *testing.T) {
	m := &V38Migration{}
	ctx := context.Background()

	t.Run("grants web_analytics to existing members and pending invitations", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		// Without this, HasPermission denies web_analytics for every non-owner
		// member: their stored permission map predates the resource.
		mock.ExpectExec(v38UserWorkspacesGrant).WillReturnResult(sqlmock.NewResult(0, 3))
		mock.ExpectExec(v38InvitationsGrant).WillReturnResult(sqlmock.NewResult(0, 1))

		require.NoError(t, m.UpdateSystem(ctx, &config.Config{}, db))
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("reports a failed user_workspaces backfill", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		mock.ExpectExec(v38UserWorkspacesGrant).WillReturnError(errors.New("boom"))

		err = m.UpdateSystem(ctx, &config.Config{}, db)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "user workspaces")
	})

	t.Run("reports a failed invitations backfill", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		mock.ExpectExec(v38UserWorkspacesGrant).WillReturnResult(sqlmock.NewResult(0, 3))
		mock.ExpectExec(v38InvitationsGrant).WillReturnError(errors.New("boom"))

		err = m.UpdateSystem(ctx, &config.Config{}, db)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "workspace invitations")
	})
}

func expectV38WorkspaceDDL(mock sqlmock.Sqlmock) {
	expectV38SchemaDDL(mock)
	// UpdateWorkspace ends by looking for live automations whose trigger carries conditions;
	// the common case finds none.
	mock.ExpectQuery(v38HealQuery).WillReturnRows(v38HealRows())
}

// expectV38SchemaDDL expects every DDL statement UpdateWorkspace issues, in the
// order it issues them: annotations, then the web analytics parents, then the
// usage meter, then the shared functions and the monthly partitions.
func expectV38SchemaDDL(mock sqlmock.Sqlmock) {
	expectV38AnnotationsDDL(mock)
	expectV38WebAnalyticsDDL(mock)
	expectV38UsageDDL(mock)
	expectV38WebAnalyticsFunctionsDDL(mock)
	expectV38WebAnalyticsPartitionDDL(mock)
}

// Pinned by name rather than by the generic shape, for the same reason as the
// annotations statements: the monthly_usage table and the partial index the
// timeline meter counts through must each fail here individually if dropped.
// The index predicate is pinned too — losing it does not break the meter's
// query, it just silently degrades it to a seq scan of contact_timeline.
func expectV38UsageDDL(mock sqlmock.Sqlmock) {
	mock.ExpectExec("CREATE TABLE IF NOT EXISTS monthly_usage").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("(?s)CREATE INDEX IF NOT EXISTS idx_contact_timeline_billable.*WHERE entity_type NOT IN").
		WillReturnResult(sqlmock.NewResult(0, 0))
}

// The annotations statements are pinned by name rather than by the generic
// "CREATE ... IF NOT EXISTS" shape used below, so that dropping one of them
// fails here instead of being absorbed by the web analytics expectations. The
// unique index expectation also pins its partial predicate: without WHERE
// source_id IS NOT NULL the index is no longer the arbiter CreateFromSource
// names in its ON CONFLICT, and every automatic annotation raises 42P10.
func expectV38AnnotationsDDL(mock sqlmock.Sqlmock) {
	mock.ExpectExec("CREATE TABLE IF NOT EXISTS annotations").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_annotations_annotated_at").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("CREATE UNIQUE INDEX IF NOT EXISTS idx_annotations_source .*WHERE source_id IS NOT NULL").
		WillReturnResult(sqlmock.NewResult(0, 0))
}

func expectV38WebAnalyticsDDL(mock sqlmock.Sqlmock) {
	for range schema.WebAnalyticsTableDefinitions() {
		mock.ExpectExec("(?s)CREATE (TABLE|INDEX) IF NOT EXISTS").WillReturnResult(sqlmock.NewResult(0, 0))
	}
}

func expectV38WebAnalyticsFunctionsDDL(mock sqlmock.Sqlmock) {
	// Upgrading workspaces must also pick up the webhook trigger body that keeps
	// bridged web goals from fanning out to third-party subscribers — the
	// new-workspace path installs the same function from the same source.
	mock.ExpectExec("(?s)CREATE OR REPLACE FUNCTION webhook_custom_events_trigger").
		WillReturnResult(sqlmock.NewResult(0, 0))
	// And the enrolment guard. A trigger outlives its automation being live — Pause drops it
	// only after writing the status, Delete discards a failed drop, and two concurrent
	// transitions can leave it installed against a paused row — so the function every
	// installed trigger calls is where "is this automation still running?" has to be asked.
	// The regex pins the guard, not just the install: a body that lost it would still match
	// on the name alone.
	mock.ExpectExec("(?s)CREATE OR REPLACE FUNCTION automation_enroll_contact.*status = 'live'").
		WillReturnResult(sqlmock.NewResult(0, 0))
}

func expectV38WebAnalyticsPartitionDDL(mock sqlmock.Sqlmock) {
	now := time.Now().UTC()
	for _, month := range []time.Time{now, now.AddDate(0, 1, 0)} {
		for _, table := range schema.WebAnalyticsTableNames {
			mock.ExpectExec(regexp.QuoteMeta(schema.WebAnalyticsPartitionName(table, month)) + " PARTITION OF").
				WillReturnResult(sqlmock.NewResult(0, 0))
		}
	}
}

func TestV38Migration_UpdateWorkspace(t *testing.T) {
	workspace := &domain.Workspace{ID: "ws1"}

	t.Run("creates parents, indexes, and current+next partitions", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		expectV38WorkspaceDDL(mock)

		m := &V38Migration{}
		require.NoError(t, m.UpdateWorkspace(context.Background(), &config.Config{}, workspace, db))
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("re-run is harmless (every statement is IF NOT EXISTS)", func(t *testing.T) {
		for _, stmt := range schema.WebAnalyticsTableDefinitions() {
			assert.Contains(t, stmt, "IF NOT EXISTS")
		}
		for _, stmt := range schema.AnnotationsTableDefinitions() {
			assert.Contains(t, stmt, "IF NOT EXISTS")
		}
		for _, stmt := range schema.UsageTableDefinitions() {
			assert.Contains(t, stmt, "IF NOT EXISTS")
		}
		// expectV38AnnotationsDDL and expectV38UsageDDL name each of their
		// statements individually, so an extra definition would arrive there
		// unexpected.
		require.Len(t, schema.AnnotationsTableDefinitions(), 3)
		require.Len(t, schema.UsageTableDefinitions(), 2)
		assert.Contains(t, schema.WebAnalyticsPartitionDDL("web_sessions", time.Now()), "IF NOT EXISTS")

		// And executing twice issues the same idempotent statements again.
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()
		expectV38WorkspaceDDL(mock)
		expectV38WorkspaceDDL(mock)

		m := &V38Migration{}
		require.NoError(t, m.UpdateWorkspace(context.Background(), &config.Config{}, workspace, db))
		require.NoError(t, m.UpdateWorkspace(context.Background(), &config.Config{}, workspace, db))
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("surfaces DDL failures with the workspace id", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		// The first statement UpdateWorkspace issues is the annotations CREATE TABLE;
		// this pins the error wrapping, not which table failed.
		mock.ExpectExec("(?s)CREATE (TABLE|INDEX) IF NOT EXISTS").WillReturnError(errors.New("boom"))

		m := &V38Migration{}
		err = m.UpdateWorkspace(context.Background(), &config.Config{}, workspace, db)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "ws1")
		assert.Contains(t, err.Error(), "boom")
	})

	t.Run("surfaces a failed automation heal with the workspace id", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		expectV38SchemaDDL(mock)
		mock.ExpectQuery(v38HealQuery).WillReturnError(errors.New("boom"))

		m := &V38Migration{}
		err = m.UpdateWorkspace(context.Background(), &config.Config{}, workspace, db)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "ws1")
		assert.Contains(t, err.Error(), "boom")
	})
}

// v38HealQuery matches the query that selects the live automations to repair.
const v38HealQuery = "SELECT a.id, a.root_node_id, a.trigger_config"

func v38HealRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "root_node_id", "trigger_config", "installed_def"})
}

// v38InstalledDef is what pg_get_triggerdef reports for an automation whose installed
// trigger carries no field filter — the common case, and the one the heal may rewrite.
const v38InstalledDef = "CREATE TRIGGER automation_trigger_x AFTER INSERT ON contact_timeline " +
	"FOR EACH ROW WHEN ((new.kind = 'contact.created'::text)) EXECUTE FUNCTION automation_trigger_x()"

// v38ConditionalTriggerConfig is a stored trigger_config carrying conditions: the shape the
// heal step exists for. Its compiled guard references contacts.country, so it shows up
// verbatim in the regenerated function body.
func v38ConditionalTriggerConfig(t *testing.T) []byte {
	t.Helper()
	b, err := json.Marshal(domain.TimelineTriggerConfig{
		EventKind: "contact.created",
		Frequency: domain.TriggerFrequencyOnce,
		Conditions: &domain.TreeNode{
			Kind: "leaf",
			Leaf: &domain.TreeNodeLeaf{
				Source: "contacts",
				Contact: &domain.ContactCondition{
					Filters: []*domain.DimensionFilter{
						{
							FieldName:    "country",
							FieldType:    "string",
							Operator:     "equals",
							StringValues: []string{"US"},
						},
					},
				},
			},
		},
	})
	require.NoError(t, err)
	return b
}

// v38StatementRecorder is the default regexp query matcher plus a record of every statement
// the code under test issued. sqlmock reports an unexpected statement by returning an error
// from Exec/Query, and healAutomationTriggerConditions deliberately swallows per-automation
// errors, so a statement that must never run would otherwise leave every expectation
// fulfilled and the test green.
type v38StatementRecorder struct {
	issued []string
}

func (r *v38StatementRecorder) Match(expectedSQL, actualSQL string) error {
	// One statement can be offered to more than one expectation; record it once.
	if len(r.issued) == 0 || r.issued[len(r.issued)-1] != actualSQL {
		r.issued = append(r.issued, actualSQL)
	}
	return sqlmock.QueryMatcherRegexp.Match(expectedSQL, actualSQL)
}

func (r *v38StatementRecorder) all() string {
	return strings.Join(r.issued, "\n")
}

func (r *v38StatementRecorder) firstWithPrefix(t *testing.T, prefix string) string {
	t.Helper()
	for _, stmt := range r.issued {
		if strings.HasPrefix(strings.TrimSpace(stmt), prefix) {
			return stmt
		}
	}
	require.FailNowf(t, "statement not issued", "no statement starting with %q in:\n%s", prefix, r.all())
	return ""
}

func TestV38Migration_HealAutomationTriggerConditions_QueryShape(t *testing.T) {
	rec := &v38StatementRecorder{}
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(rec))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(v38HealQuery).WillReturnRows(v38HealRows())

	m := &V38Migration{}
	require.NoError(t, m.healAutomationTriggerConditions(context.Background(), db))
	require.Len(t, rec.issued, 1)
	query := rec.issued[0]

	// The query is the filter, so its text is what decides which automations get their
	// trigger rewritten. It must NOT narrow to automations carrying conditions: updates
	// never regenerated the trigger before this release, so an automation whose event_kind,
	// list_id, segment_id or updated_fields was edited while live is running a trigger that
	// does not match its stored configuration, conditions or no conditions. Nothing else
	// will ever notice it, because a change is detected from here on by comparing the
	// incoming row against the stored one — and a stale trigger makes neither disagree.
	assert.NotContains(t, query, "conditions")

	// Repair-only: an automation with no installed trigger has never fired and must be left
	// alone rather than armed mid-migration.
	assert.Contains(t, query, "pg_trigger")
	assert.Contains(t, query, "status = 'live'")
	assert.Contains(t, query, "deleted_at IS NULL")

	// The name built here is compared against pg_trigger.tgname, which holds what PostgreSQL
	// stored — and CREATE TRIGGER folds an unquoted identifier to lower case. Without lower()
	// every automation whose id carries an upper-case letter is silently skipped by the
	// repair. Console ids are lower-case uuids; ids supplied through the API need not be.
	assert.Contains(t, query, "lower('automation_trigger_'")
}

func TestV38Migration_HealAutomationTriggerConditions(t *testing.T) {
	m := &V38Migration{}
	ctx := context.Background()

	t.Run("regenerates the trigger of a live automation with conditions", func(t *testing.T) {
		rec := &v38StatementRecorder{}
		db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(rec))
		require.NoError(t, err)
		defer db.Close()

		mock.ExpectQuery(v38HealQuery).WillReturnRows(
			v38HealRows().AddRow("condauto", "node1", v38ConditionalTriggerConfig(t), v38InstalledDef))

		mock.ExpectExec("^SAVEPOINT v38_automation_conditions$").WillReturnResult(sqlmock.NewResult(0, 0))
		// Same order as the repository's trigger creation: drop trigger, drop function,
		// create function, create trigger — and then the probe.
		mock.ExpectExec("DROP TRIGGER IF EXISTS automation_trigger_condauto ON contact_timeline").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("DROP FUNCTION IF EXISTS automation_trigger_condauto").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE OR REPLACE FUNCTION automation_trigger_condauto").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE TRIGGER automation_trigger_condauto").
			WillReturnResult(sqlmock.NewResult(0, 0))
		// The probe runs last, matching the repository. Planning it first would take
		// ACCESS SHARE on contact_timeline and then upgrade to the ACCESS EXCLUSIVE the
		// DROPs need — the interleaving that deadlocks against an activation served by an
		// instance still running during a rolling restart. The savepoint undoes the DDL
		// either way, so a failed probe still leaves nothing installed.
		mock.ExpectQuery("EXPLAIN SELECT").WillReturnRows(sqlmock.NewRows([]string{"QUERY PLAN"}))
		mock.ExpectExec("^RELEASE SAVEPOINT v38_automation_conditions$").WillReturnResult(sqlmock.NewResult(0, 0))

		require.NoError(t, m.healAutomationTriggerConditions(ctx, db))
		assert.NoError(t, mock.ExpectationsWereMet())

		// The repair is only worth anything if the conditions land in the function body: the
		// WHEN clause compiles them to correlated subqueries, which PostgreSQL rejects at
		// CREATE TRIGGER time (SQLSTATE 0A000).
		functionBody := rec.firstWithPrefix(t, "CREATE OR REPLACE FUNCTION")
		assert.Contains(t, functionBody, "country = 'US'")
		assert.Contains(t, functionBody, "IF (")
		triggerDDL := rec.firstWithPrefix(t, "CREATE TRIGGER")
		assert.NotContains(t, triggerDDL, "SELECT")
		assert.NotContains(t, triggerDDL, "EXISTS")
	})

	// The stored config is authoritative only because nothing quietly removed part of it —
	// and something did. The console rebuilt trigger_config from a fixed list of fields that
	// omitted updated_fields, and an update overwrites the column wholesale, so one console
	// save of an API-created contact.updated automation dropped its field filter from the
	// row while the narrow trigger stayed installed (updates ran no DDL). Regenerating from
	// that row would widen it to every contact update.
	t.Run("refuses to replace a narrower installed trigger with a broader one", func(t *testing.T) {
		rec := &v38StatementRecorder{}
		db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(rec))
		require.NoError(t, err)
		defer db.Close()

		narrow := "CREATE TRIGGER automation_trigger_narrow AFTER INSERT ON contact_timeline " +
			"FOR EACH ROW WHEN (((new.kind = 'contact.updated'::text) AND (new.changes ? 'first_name'::text))) " +
			"EXECUTE FUNCTION automation_trigger_narrow()"

		stripped, err := json.Marshal(domain.TimelineTriggerConfig{
			EventKind: "contact.updated",
			Frequency: domain.TriggerFrequencyOnce,
		})
		require.NoError(t, err)

		mock.ExpectQuery(v38HealQuery).WillReturnRows(
			v38HealRows().AddRow("narrow", "node1", stripped, narrow))

		// No savepoint, no DDL: the automation is left exactly as it is.
		require.NoError(t, m.healAutomationTriggerConditions(ctx, db))
		assert.NoError(t, mock.ExpectationsWereMet())
		assert.NotContains(t, rec.all(), "DROP TRIGGER",
			"a trigger filtering on changed fields must not be replaced by one that does not")
	})

	t.Run("does nothing when no automation matches", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		mock.ExpectQuery(v38HealQuery).WillReturnRows(v38HealRows())

		// No savepoint and no DDL: an unexpected statement here would come back as an error
		// from ExecContext, and the savepoint failures are reported rather than swallowed.
		require.NoError(t, m.healAutomationTriggerConditions(ctx, db))
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("skips an automation whose trigger config does not parse", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		// Its installed trigger was already unusable; nothing can be regenerated from this.
		mock.ExpectQuery(v38HealQuery).WillReturnRows(
			v38HealRows().AddRow("badjson", "node1", []byte("{not json"), v38InstalledDef))

		require.NoError(t, m.healAutomationTriggerConditions(ctx, db))
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("skips an automation the generator rejects", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		// A NULL root_node_id leaves the generator with nothing to enroll into; skipped
		// rather than aborting the workspace migration.
		mock.ExpectQuery(v38HealQuery).WillReturnRows(
			v38HealRows().AddRow("norootnode", nil, v38ConditionalTriggerConfig(t), v38InstalledDef))

		require.NoError(t, m.healAutomationTriggerConditions(ctx, db))
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("installs nothing when the condition probe fails", func(t *testing.T) {
		rec := &v38StatementRecorder{}
		db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(rec))
		require.NoError(t, err)
		defer db.Close()

		mock.ExpectQuery(v38HealQuery).WillReturnRows(
			v38HealRows().AddRow("probefail", "node1", v38ConditionalTriggerConfig(t), v38InstalledDef))

		mock.ExpectExec("^SAVEPOINT v38_automation_conditions$").WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("DROP TRIGGER IF EXISTS automation_trigger_probefail ON contact_timeline").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("DROP FUNCTION IF EXISTS automation_trigger_probefail").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE OR REPLACE FUNCTION automation_trigger_probefail").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE TRIGGER automation_trigger_probefail").
			WillReturnResult(sqlmock.NewResult(0, 0))
		// The condition names something this workspace does not have. CREATE FUNCTION only
		// syntax-checks a plpgsql body, so it accepted the function a moment ago; keeping it
		// would abort every write to contact_timeline for a workspace that was working
		// before the upgrade. The savepoint is what takes it back out.
		mock.ExpectQuery("EXPLAIN SELECT").WillReturnError(assert.AnError)
		mock.ExpectExec("^ROLLBACK TO SAVEPOINT v38_automation_conditions$").WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("^RELEASE SAVEPOINT v38_automation_conditions$").WillReturnResult(sqlmock.NewResult(0, 0))

		// Nil overall: failing hard would abort the whole workspace migration, never record
		// the version, and re-fail on every restart, instance-wide. The fallback here is
		// merely the status quo — a trigger that over-enrolls because it ignores conditions.
		require.NoError(t, m.healAutomationTriggerConditions(ctx, db))
		assert.NoError(t, mock.ExpectationsWereMet())

		// The DDL ran, so what protects the workspace is the rollback, not the ordering.
		// Assert it was issued and that nothing was released without it.
		assert.Contains(t, rec.all(), "ROLLBACK TO SAVEPOINT v38_automation_conditions",
			"a failed probe must take the freshly installed trigger back out")
	})

	t.Run("rolls back to the savepoint when a DDL statement fails", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		mock.ExpectQuery(v38HealQuery).WillReturnRows(
			v38HealRows().AddRow("ddlfail", "node1", v38ConditionalTriggerConfig(t), v38InstalledDef))

		mock.ExpectExec("^SAVEPOINT v38_automation_conditions$").WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("DROP TRIGGER IF EXISTS automation_trigger_ddlfail").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("DROP FUNCTION IF EXISTS automation_trigger_ddlfail").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE OR REPLACE FUNCTION automation_trigger_ddlfail").
			WillReturnError(assert.AnError)
		// The dropped trigger comes back with the savepoint, so the automation keeps the
		// trigger it had.
		mock.ExpectExec("^ROLLBACK TO SAVEPOINT v38_automation_conditions$").WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("^RELEASE SAVEPOINT v38_automation_conditions$").WillReturnResult(sqlmock.NewResult(0, 0))

		require.NoError(t, m.healAutomationTriggerConditions(ctx, db),
			"one automation's failure must not fail the migration")
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("reports a scan failure instead of skipping it", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		// A NULL id cannot be scanned. Infrastructure failures are not per-automation
		// failures: they say the read itself is unreliable, so the migration stops.
		mock.ExpectQuery(v38HealQuery).WillReturnRows(
			v38HealRows().AddRow(nil, "node1", v38ConditionalTriggerConfig(t), v38InstalledDef))

		err = m.healAutomationTriggerConditions(ctx, db)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to scan automation")
	})
}
