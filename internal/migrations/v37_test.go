package migrations

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
)

// parameterizedSQL asserts the regenerated segment query binds the timeline change key instead
// of splicing it into the SQL text — the whole point of the recompile. sqlmock only compares the
// statement prefix, so without a real matcher here the migration could write anything at all.
type parameterizedSQL struct{}

func (parameterizedSQL) Match(v driver.Value) bool {
	s, ok := v.(string)
	if !ok {
		return false
	}
	return strings.Contains(s, "ct.changes->$2->>'new'") && !strings.Contains(s, "ct.changes->'")
}

func TestV37Migration_Metadata(t *testing.T) {
	m := &V37Migration{}
	assert.Equal(t, 37.0, m.GetMajorVersion())
	assert.False(t, m.HasSystemUpdate())
	assert.True(t, m.HasWorkspaceUpdate())
	assert.False(t, m.ShouldRestartServer())
}

func TestV37Migration_IsRegistered(t *testing.T) {
	migration, ok := GetRegisteredMigration(37.0)
	require.True(t, ok, "v37 must be registered so it runs on startup")
	assert.IsType(t, &V37Migration{}, migration)
}

func TestV37Migration_UpdateSystem_IsNoOp(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	m := &V37Migration{}
	require.NoError(t, m.UpdateSystem(context.Background(), &config.Config{}, db))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestV37Migration_UpdateWorkspace_WidensKindColumn(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectExec("SET LOCAL statement_timeout").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("SELECT id, tree FROM segments").
		WillReturnRows(sqlmock.NewRows([]string{"id", "tree"}))
	expectKindWidth(mock, 50)
	expectNoDependentTriggers(mock)
	mock.ExpectExec("ALTER TABLE contact_timeline").WillReturnResult(sqlmock.NewResult(0, 0))

	m := &V37Migration{}
	require.NoError(t, m.UpdateWorkspace(context.Background(), &config.Config{}, &domain.Workspace{ID: "ws1"}, db))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestV37Migration_RecompilesSegmentWithInterpolatedTimelineKey(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	tree := &domain.TreeNode{
		Kind: "leaf",
		Leaf: &domain.TreeNodeLeaf{
			Source: "contact_timeline",
			ContactTimeline: &domain.ContactTimelineCondition{
				Kind:          "custom_event.purchase",
				CountOperator: "at_least",
				CountValue:    1,
				Filters: []*domain.DimensionFilter{
					{FieldName: "goal_value", FieldType: "number", Operator: "gte", NumberValues: []float64{100}},
				},
			},
		},
	}
	treeJSON, err := json.Marshal(tree)
	require.NoError(t, err)

	mock.ExpectExec("SET LOCAL statement_timeout").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("SELECT id, tree FROM segments").
		WillReturnRows(sqlmock.NewRows([]string{"id", "tree"}).AddRow("seg1", treeJSON))
	mock.ExpectExec("UPDATE segments SET generated_sql").
		WithArgs(parameterizedSQL{}, sqlmock.AnyArg(), "seg1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	expectKindWidth(mock, 50)
	expectNoDependentTriggers(mock)
	mock.ExpectExec("ALTER TABLE contact_timeline").WillReturnResult(sqlmock.NewResult(0, 0))

	m := &V37Migration{}
	require.NoError(t, m.UpdateWorkspace(context.Background(), &config.Config{}, &domain.Workspace{ID: "ws1"}, db))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestV37Migration_SkipsSegmentWithUnparseableTree(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectExec("SET LOCAL statement_timeout").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("SELECT id, tree FROM segments").
		WillReturnRows(sqlmock.NewRows([]string{"id", "tree"}).AddRow("seg1", []byte("{not json")))
	expectKindWidth(mock, 50)
	expectNoDependentTriggers(mock)
	mock.ExpectExec("ALTER TABLE contact_timeline").WillReturnResult(sqlmock.NewResult(0, 0))
	// No UPDATE expected: a tree that cannot be decoded must not blank the stored query, and it
	// must not abort the migration either (that would block server startup).

	m := &V37Migration{}
	require.NoError(t, m.UpdateWorkspace(context.Background(), &config.Config{}, &domain.Workspace{ID: "ws1"}, db))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestV37Migration_SkipsSegmentWhoseTreeNoLongerCompiles(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	// Valid JSON, invalid tree (a leaf with no source), so BuildSQL fails.
	treeJSON := []byte(`{"kind":"leaf","leaf":{}}`)

	mock.ExpectExec("SET LOCAL statement_timeout").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("SELECT id, tree FROM segments").
		WillReturnRows(sqlmock.NewRows([]string{"id", "tree"}).AddRow("seg1", treeJSON))
	expectKindWidth(mock, 50)
	expectNoDependentTriggers(mock)
	mock.ExpectExec("ALTER TABLE contact_timeline").WillReturnResult(sqlmock.NewResult(0, 0))

	m := &V37Migration{}
	require.NoError(t, m.UpdateWorkspace(context.Background(), &config.Config{}, &domain.Workspace{ID: "ws1"}, db))
	require.NoError(t, mock.ExpectationsWereMet())
}

// automationTriggerDef is the definition PostgreSQL reports for the trigger a live automation
// installs: an AFTER INSERT trigger whose WHEN clause reads contact_timeline.kind, which is
// exactly what makes the column impossible to alter in place.
const automationTriggerDef = `CREATE TRIGGER automation_trigger_98392e3e AFTER INSERT ON public.contact_timeline ` +
	`FOR EACH ROW WHEN (((new.kind)::text = 'contact.created'::text)) EXECUTE FUNCTION automation_trigger_98392e3e()`

// expectKindWidth stands in for the declared width of contact_timeline.kind. atttypmod carries
// the declared length plus the 4 byte varlena header, which is how the migration reads it.
func expectKindWidth(mock sqlmock.Sqlmock, declaredLength int) {
	expectKindTypmod(mock, declaredLength+4)
}

// expectKindTypmod stands in for the raw atttypmod, so a column with no declared length (-1) can
// be expressed too.
func expectKindTypmod(mock sqlmock.Sqlmock, typmod int) {
	mock.ExpectQuery("atttypmod").
		WillReturnRows(sqlmock.NewRows([]string{"atttypmod"}).AddRow(typmod))
}

// expectNoDependentTriggers stands in for a workspace with no live automation: nothing depends on
// the column, so the migration must not issue any DROP or CREATE around the ALTER.
func expectNoDependentTriggers(mock sqlmock.Sqlmock) {
	mock.ExpectQuery("FROM pg_trigger").
		WillReturnRows(sqlmock.NewRows([]string{"tgname", "tgenabled", "tgdef"}))
}

func TestV37Migration_RecreatesTriggersDependingOnKind(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectExec("SET LOCAL statement_timeout").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("SELECT id, tree FROM segments").
		WillReturnRows(sqlmock.NewRows([]string{"id", "tree"}))
	expectKindWidth(mock, 50)
	mock.ExpectQuery("FROM pg_trigger").
		WillReturnRows(sqlmock.NewRows([]string{"tgname", "tgenabled", "tgdef"}).
			AddRow("automation_trigger_98392e3e", "O", automationTriggerDef))
	// The order is the whole point: the trigger has to be gone before the ALTER, and back
	// afterwards, inside the migration's transaction.
	mock.ExpectExec(regexp.QuoteMeta(`DROP TRIGGER "automation_trigger_98392e3e" ON contact_timeline`)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("ALTER TABLE contact_timeline").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(regexp.QuoteMeta(automationTriggerDef)).WillReturnResult(sqlmock.NewResult(0, 0))

	m := &V37Migration{}
	require.NoError(t, m.UpdateWorkspace(context.Background(), &config.Config{}, &domain.Workspace{ID: "ws1"}, db))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestV37Migration_PreservesDisabledTriggerState(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	disabledDef := `CREATE TRIGGER paused_probe AFTER INSERT ON public.contact_timeline ` +
		`FOR EACH ROW WHEN (((new.kind)::text = 'contact.created'::text)) EXECUTE FUNCTION probe()`

	mock.ExpectExec("SET LOCAL statement_timeout").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("SELECT id, tree FROM segments").
		WillReturnRows(sqlmock.NewRows([]string{"id", "tree"}))
	expectKindWidth(mock, 50)
	mock.ExpectQuery("FROM pg_trigger").
		WillReturnRows(sqlmock.NewRows([]string{"tgname", "tgenabled", "tgdef"}).
			AddRow("paused_probe", "D", disabledDef))
	mock.ExpectExec(regexp.QuoteMeta(`DROP TRIGGER "paused_probe" ON contact_timeline`)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("ALTER TABLE contact_timeline").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(regexp.QuoteMeta(disabledDef)).WillReturnResult(sqlmock.NewResult(0, 0))
	// CREATE TRIGGER always produces an enabled trigger, so a disabled one has to be switched
	// off again — otherwise the migration silently starts firing something that was turned off.
	mock.ExpectExec(regexp.QuoteMeta(`ALTER TABLE contact_timeline DISABLE TRIGGER "paused_probe"`)).
		WillReturnResult(sqlmock.NewResult(0, 0))

	m := &V37Migration{}
	require.NoError(t, m.UpdateWorkspace(context.Background(), &config.Config{}, &domain.Workspace{ID: "ws1"}, db))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestV37Migration_FailsWhenTriggerCannotBeRecreated(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectExec("SET LOCAL statement_timeout").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("SELECT id, tree FROM segments").
		WillReturnRows(sqlmock.NewRows([]string{"id", "tree"}))
	expectKindWidth(mock, 50)
	mock.ExpectQuery("FROM pg_trigger").
		WillReturnRows(sqlmock.NewRows([]string{"tgname", "tgenabled", "tgdef"}).
			AddRow("automation_trigger_98392e3e", "O", automationTriggerDef))
	mock.ExpectExec(regexp.QuoteMeta(`DROP TRIGGER "automation_trigger_98392e3e" ON contact_timeline`)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("ALTER TABLE contact_timeline").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(regexp.QuoteMeta(automationTriggerDef)).WillReturnError(errors.New("boom"))

	// A trigger that cannot be put back must fail the migration so the transaction rolls back and
	// the workspace keeps its automations, rather than completing with the trigger missing.
	m := &V37Migration{}
	updateErr := m.UpdateWorkspace(context.Background(), &config.Config{}, &domain.Workspace{ID: "ws1"}, db)
	require.Error(t, updateErr)
	assert.Contains(t, updateErr.Error(), "automation_trigger_98392e3e")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestV37Migration_LeavesAnAlreadyWidenedColumnAlone(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectExec("SET LOCAL statement_timeout").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("SELECT id, tree FROM segments").
		WillReturnRows(sqlmock.NewRows([]string{"id", "tree"}))
	expectKindWidth(mock, 150)
	// Nothing else: a workspace that is already at the target width must not have its triggers
	// dropped and recreated, nor take the exclusive lock an ALTER needs. This happens on every
	// retry after another workspace in the same install failed.

	m := &V37Migration{}
	require.NoError(t, m.UpdateWorkspace(context.Background(), &config.Config{}, &domain.Workspace{ID: "ws1"}, db))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestV37Migration_LeavesAColumnWithNoLengthLimitAlone(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectExec("SET LOCAL statement_timeout").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("SELECT id, tree FROM segments").
		WillReturnRows(sqlmock.NewRows([]string{"id", "tree"}))
	expectKindTypmod(mock, -1)
	// atttypmod -1 means no declared length, i.e. TEXT or an unqualified VARCHAR — already able to
	// hold anything the timeline triggers write. Narrowing it to VARCHAR(150) would fail outright
	// on any row longer than that and abort startup all over again, so it must be left alone.

	m := &V37Migration{}
	require.NoError(t, m.UpdateWorkspace(context.Background(), &config.Config{}, &domain.Workspace{ID: "ws1"}, db))
	require.NoError(t, mock.ExpectationsWereMet())
}
