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
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/database/schema"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/service"
)

// The fix's only change to compiled SQL; removing it from the builder's current output
// gives exactly what the builder produced before 42.
const v42Fix = " AND cl.deleted_at IS NULL"

func v42Pre42(newSQL string) string { return strings.ReplaceAll(newSQL, v42Fix, "") }

func v42ListLeaf(operator, listID string, status *string) *domain.TreeNode {
	return &domain.TreeNode{
		Kind: "leaf",
		Leaf: &domain.TreeNodeLeaf{
			Source:      "contact_lists",
			ContactList: &domain.ContactListCondition{Operator: operator, ListID: listID, Status: status},
		},
	}
}

func v42TreeJSON(t *testing.T, tree *domain.TreeNode) []byte {
	t.Helper()
	b, err := json.Marshal(tree)
	require.NoError(t, err)
	return b
}

// v42SegmentSQL compiles a tree with the real builder and returns its current and
// pre-42 SQL.
func v42SegmentSQL(t *testing.T, tree *domain.TreeNode) (string, string) {
	t.Helper()
	sql, _, err := service.NewQueryBuilder().BuildSQL(tree)
	require.NoError(t, err)
	return sql, v42Pre42(sql)
}

// v42TriggerFunction generates the trigger function of an automation whose trigger
// condition is the given tree, with the real generator, and returns its current and
// pre-42 definitions.
func v42TriggerFunction(t *testing.T, id string, tree *domain.TreeNode) (string, string) {
	t.Helper()
	gen := service.NewAutomationTriggerGenerator(service.NewQueryBuilder())
	result, err := gen.Generate(&domain.Automation{
		ID:         id,
		ListID:     "listX",
		RootNodeID: "node1",
		Trigger: &domain.TimelineTriggerConfig{
			EventKind:  "custom_event.purchase",
			Frequency:  domain.TriggerFrequencyEveryTime,
			Conditions: tree,
		},
	})
	require.NoError(t, err)
	return result.FunctionBody, v42Pre42(result.FunctionBody)
}

// exactSQL matches a bound argument equal to want.
type exactSQL struct{ want string }

func (e exactSQL) Match(v driver.Value) bool {
	s, ok := v.(string)
	return ok && s == e.want
}

// exactly turns a statement into a pattern matching only that statement (sqlmock's
// default matcher is an unanchored regexp, after collapsing whitespace on both sides).
func exactly(stmt string) string { return "^" + regexp.QuoteMeta(stmt) + "$" }

func expectV42Prelude(mock sqlmock.Sqlmock) {
	mock.ExpectExec(exactly("SET LOCAL statement_timeout = 0")).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(exactly(schema.TrackContactListChangesFunction())).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(exactly(schema.WebhookContactListsTriggerFunction())).WillReturnResult(sqlmock.NewResult(0, 0))
}

func expectPatch(mock sqlmock.Sqlmock, patched string, execErr error) {
	mock.ExpectExec(exactly("SAVEPOINT v42_automation_trigger")).WillReturnResult(sqlmock.NewResult(0, 0))
	if execErr != nil {
		mock.ExpectExec(exactly(patched)).WillReturnError(execErr)
		mock.ExpectExec(exactly("ROLLBACK TO SAVEPOINT v42_automation_trigger")).WillReturnResult(sqlmock.NewResult(0, 0))
	} else {
		mock.ExpectExec(exactly(patched)).WillReturnResult(sqlmock.NewResult(0, 0))
	}
	mock.ExpectExec(exactly("RELEASE SAVEPOINT v42_automation_trigger")).WillReturnResult(sqlmock.NewResult(0, 0))
}

func functionRows(defs ...string) *sqlmock.Rows {
	rows := sqlmock.NewRows([]string{"oid", "pg_get_functiondef"})
	for i, def := range defs {
		rows.AddRow(int64(1000+i), def)
	}
	return rows
}

func segmentRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "tree", "generated_sql"})
}

const (
	v42FunctionsQuery = `SELECT p.oid, pg_get_functiondef\(p.oid\)\s+FROM pg_proc p\s+WHERE p.proname LIKE 'automation\\_trigger\\_%'\s+AND p.prosrc LIKE '%FROM contact_lists cl JOIN lists l%'\s+AND p.prosrc NOT LIKE '%cl.deleted_at IS NULL%'`
	v42SegmentsQuery  = `SELECT id, tree, generated_sql FROM segments\s+WHERE status <> 'deleted'`
	v42RequeueStmt    = `INSERT INTO contact_segment_queue \(email, queued_at\)\s+SELECT DISTINCT cl.email, NOW\(\)\s+FROM contact_lists cl\s+JOIN lists l ON l.id = cl.list_id\s+WHERE cl.deleted_at IS NOT NULL\s+AND l.deleted_at IS NULL\s+AND cl.list_id = ANY\(\$1\)\s+ON CONFLICT \(email\) DO UPDATE SET queued_at = EXCLUDED.queued_at`
)

func expectNoFunctions(mock sqlmock.Sqlmock) {
	mock.ExpectQuery(v42FunctionsQuery).WillReturnRows(sqlmock.NewRows([]string{"oid", "pg_get_functiondef"}))
}

func expectNoSegments(mock sqlmock.Sqlmock) {
	mock.ExpectQuery(v42SegmentsQuery).WillReturnRows(sqlmock.NewRows([]string{"id", "tree", "generated_sql"}))
}

func runV42(t *testing.T, db DBExecutor) error {
	t.Helper()
	return (&V42Migration{}).UpdateWorkspace(context.Background(), &config.Config{}, &domain.Workspace{ID: "ws1"}, db)
}

func TestV42Migration_Metadata(t *testing.T) {
	m := &V42Migration{}
	assert.Equal(t, 42.0, m.GetMajorVersion())
	assert.False(t, m.HasSystemUpdate())
	assert.True(t, m.HasWorkspaceUpdate())
	assert.False(t, m.ShouldRestartServer())
}

func TestV42Migration_IsRegistered(t *testing.T) {
	migration, ok := GetRegisteredMigration(42.0)
	require.True(t, ok, "v42 must be registered so it runs on startup")
	assert.IsType(t, &V42Migration{}, migration)
}

func TestV42Migration_UpdateSystemIsANoOp(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	require.NoError(t, (&V42Migration{}).UpdateSystem(context.Background(), &config.Config{}, db))
	require.NoError(t, mock.ExpectationsWereMet())
}

// The fixtures the migration repairs are the builder's real pre-42 output. These are the
// strings HEAD c974080a printed; deriving them from the current builder keeps the tests
// below tied to the real producer.
func TestV42Migration_Pre42FixturesMatchTheOldBuilder(t *testing.T) {
	active := "active"

	_, oldIn := v42SegmentSQL(t, v42ListLeaf("in", "listX", &active))
	assert.Equal(t, "SELECT email FROM contacts WHERE EXISTS (SELECT 1 FROM contact_lists cl JOIN lists l ON cl.list_id = l.id WHERE cl.email = contacts.email AND cl.list_id = $1 AND cl.status = $2 AND l.deleted_at IS NULL)", oldIn)

	_, oldNotIn := v42SegmentSQL(t, v42ListLeaf("not_in", "listX", nil))
	assert.Equal(t, "SELECT email FROM contacts WHERE NOT EXISTS (SELECT 1 FROM contact_lists cl JOIN lists l ON cl.list_id = l.id WHERE cl.email = contacts.email AND cl.list_id = $1 AND l.deleted_at IS NULL)", oldNotIn)

	_, oldFn := v42TriggerFunction(t, "auto1", v42ListLeaf("in", "listX", &active))
	assert.Contains(t, oldFn, "IF (EXISTS (SELECT 1 FROM contact_lists cl JOIN lists l ON cl.list_id = l.id WHERE cl.email = NEW.email AND cl.list_id = 'listX' AND cl.status = 'active' AND l.deleted_at IS NULL)) THEN")
}

// Part 5's order: timeout, both contact_lists functions, the automation functions, the
// segments, and the re-queue last.
func TestV42Migration_RunsItsStepsInOrder(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	active := "active"
	newFn, oldFn := v42TriggerFunction(t, "auto1", v42ListLeaf("in", "listX", &active))
	tree := v42ListLeaf("in", "listX", &active)
	newSQL, oldSQL := v42SegmentSQL(t, tree)

	expectV42Prelude(mock)
	mock.ExpectQuery(v42FunctionsQuery).WillReturnRows(functionRows(oldFn))
	expectPatch(mock, newFn, nil)
	mock.ExpectQuery(v42SegmentsQuery).
		WillReturnRows(segmentRows().AddRow("seg1", v42TreeJSON(t, tree), oldSQL))
	mock.ExpectExec("UPDATE segments SET generated_sql").
		WithArgs(exactSQL{newSQL}, []byte(`["listX","active"]`), "seg1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(v42RequeueStmt).WithArgs(pq.Array([]string{"listX"})).WillReturnResult(sqlmock.NewResult(0, 3))

	require.NoError(t, runV42(t, db))
	require.NoError(t, mock.ExpectationsWereMet())
}

// A selected function is run with the patched definition, which is byte-identical to
// what the fixed generator installs; a failed patch rolls back to its savepoint, and the
// next function is still patched.
func TestV42Migration_PatchesEachFunctionInItsOwnSavepoint(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	active := "active"
	newFail, oldFail := v42TriggerFunction(t, "autofail", v42ListLeaf("in", "listX", &active))
	newOK, oldOK := v42TriggerFunction(t, "autook", &domain.TreeNode{
		Kind: "branch",
		Branch: &domain.TreeNodeBranch{
			Operator: "and",
			Leaves: []*domain.TreeNode{
				v42ListLeaf("in", "listX", &active),
				v42ListLeaf("not_in", "listY", nil),
			},
		},
	})
	require.NotEqual(t, newFail, oldFail)
	require.NotEqual(t, newOK, oldOK)

	expectV42Prelude(mock)
	mock.ExpectQuery(v42FunctionsQuery).WillReturnRows(functionRows(oldFail, oldOK))
	expectPatch(mock, newFail, errors.New("syntax error"))
	expectPatch(mock, newOK, nil)
	expectNoSegments(mock)

	require.NoError(t, runV42(t, db))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestV42Migration_NoSelectedFunctionMeansNoDDL(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	expectV42Prelude(mock)
	expectNoFunctions(mock)
	expectNoSegments(mock)

	require.NoError(t, runV42(t, db))
	require.NoError(t, mock.ExpectationsWereMet())
}

func v42ContactsOnlyTree() *domain.TreeNode {
	return &domain.TreeNode{
		Kind: "leaf",
		Leaf: &domain.TreeNodeLeaf{
			Source: "contacts",
			Contact: &domain.ContactCondition{Filters: []*domain.DimensionFilter{
				{FieldName: "country", FieldType: "string", Operator: "equals", StringValues: []string{"FR"}},
			}},
		},
	}
}

// Old segment SQL is recompiled; trees that don't parse or no longer compile keep their
// SQL; list IDs come from every non-deleted segment, including one whose SQL is already
// new; the re-queue runs with exactly those IDs.
func TestV42Migration_RecompilesSegmentsAndRequeuesTheirRemovedMembers(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	active := "active"
	inX := v42ListLeaf("in", "listX", &active)
	newInX, oldInX := v42SegmentSQL(t, inX)
	notInX := v42ListLeaf("not_in", "listX", nil)
	newNotInX, oldNotInX := v42SegmentSQL(t, notInX)
	// Already recompiled by v36/v37 on a database upgraded from below 37.
	inW := v42ListLeaf("in", "listW", nil)
	newInW, _ := v42SegmentSQL(t, inW)
	// Parses, but no longer compiles (a contact_lists leaf without a list ID).
	broken := v42ListLeaf("in", "", nil)
	contactsOnly := v42ContactsOnlyTree()
	contactsSQL, _ := v42SegmentSQL(t, contactsOnly)

	expectV42Prelude(mock)
	expectNoFunctions(mock)
	mock.ExpectQuery(v42SegmentsQuery).WillReturnRows(segmentRows().
		AddRow("segIn", v42TreeJSON(t, inX), oldInX).
		AddRow("segNotIn", v42TreeJSON(t, notInX), oldNotInX).
		AddRow("segW", v42TreeJSON(t, inW), newInW).
		AddRow("segUnparseable", []byte("{not json"), oldInX).
		AddRow("segBroken", v42TreeJSON(t, broken), oldInX).
		AddRow("segContacts", v42TreeJSON(t, contactsOnly), contactsSQL).
		AddRow("segNoSQL", v42TreeJSON(t, contactsOnly), nil))
	mock.ExpectExec("UPDATE segments SET generated_sql").
		WithArgs(exactSQL{newInX}, []byte(`["listX","active"]`), "segIn").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE segments SET generated_sql").
		WithArgs(exactSQL{newNotInX}, []byte(`["listX"]`), "segNotIn").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(v42RequeueStmt).
		WithArgs(pq.Array([]string{"listW", "listX"})).
		WillReturnResult(sqlmock.NewResult(0, 2))

	require.NoError(t, runV42(t, db))
	require.NoError(t, mock.ExpectationsWereMet())

	assert.Contains(t, newInX, "cl.deleted_at IS NULL")
	assert.Contains(t, newNotInX, "NOT EXISTS")
}

// No segment names a list: nothing is re-queued.
func TestV42Migration_NoListConditionMeansNoRequeue(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	contactsOnly := v42ContactsOnlyTree()
	contactsSQL, _ := v42SegmentSQL(t, contactsOnly)

	expectV42Prelude(mock)
	expectNoFunctions(mock)
	mock.ExpectQuery(v42SegmentsQuery).WillReturnRows(segmentRows().
		AddRow("segContacts", v42TreeJSON(t, contactsOnly), contactsSQL))

	require.NoError(t, runV42(t, db))
	require.NoError(t, mock.ExpectationsWereMet())
}

// A failure outside the per-function savepoints aborts the workspace migration, so the
// transaction rolls back and the version is not recorded.
func TestV42Migration_PropagatesErrors(t *testing.T) {
	t.Run("installing a contact_lists trigger function", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		mock.ExpectExec(exactly("SET LOCAL statement_timeout = 0")).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec(exactly(schema.TrackContactListChangesFunction())).WillReturnError(errors.New("boom"))

		err = runV42(t, db)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "contact_lists trigger function")
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("updating a segment", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		inX := v42ListLeaf("in", "listX", nil)
		_, oldInX := v42SegmentSQL(t, inX)

		expectV42Prelude(mock)
		expectNoFunctions(mock)
		mock.ExpectQuery(v42SegmentsQuery).WillReturnRows(segmentRows().AddRow("segIn", v42TreeJSON(t, inX), oldInX))
		mock.ExpectExec("UPDATE segments SET generated_sql").WillReturnError(errors.New("boom"))

		err = runV42(t, db)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "segIn")
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("re-queueing", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		inW := v42ListLeaf("in", "listW", nil)
		newInW, _ := v42SegmentSQL(t, inW)

		expectV42Prelude(mock)
		expectNoFunctions(mock)
		mock.ExpectQuery(v42SegmentsQuery).WillReturnRows(segmentRows().AddRow("segW", v42TreeJSON(t, inW), newInW))
		mock.ExpectExec(v42RequeueStmt).WillReturnError(errors.New("boom"))

		err = runV42(t, db)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "re-queue")
		require.NoError(t, mock.ExpectationsWereMet())
	})
}
