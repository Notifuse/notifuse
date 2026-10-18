package migrations

import (
	"context"
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

func treeRows(t *testing.T, trees ...interface{}) *sqlmock.Rows {
	t.Helper()
	rows := sqlmock.NewRows([]string{"tree"})
	for _, tree := range trees {
		switch v := tree.(type) {
		case []byte:
			rows.AddRow(v)
		case *domain.TreeNode:
			rows.AddRow(v42TreeJSON(t, v))
		default:
			t.Fatalf("unsupported tree fixture %T", tree)
		}
	}
	return rows
}

const (
	v42FunctionsQuery = `SELECT p.oid, pg_get_functiondef\(p.oid\)\s+FROM pg_proc p\s+WHERE p.proname LIKE 'automation\\_trigger\\_%'\s+AND p.prosrc LIKE '%FROM contact_lists cl JOIN lists l%'\s+AND p.prosrc NOT LIKE '%cl.deleted_at IS NULL%'`
	// One statement, so a segment saved while the migration runs is never overwritten with
	// SQL derived from the version read before it.
	v42SegmentPatchStmt  = `UPDATE segments\s+SET generated_sql = replace\(generated_sql, \$1, \$2\)\s+WHERE status <> 'deleted'\s+AND strpos\(generated_sql, \$1\) > 0\s+AND strpos\(generated_sql, \$3\) > 0\s+AND strpos\(generated_sql, \$4\) = 0`
	v42SegmentTreesQuery = `SELECT tree FROM segments\s+WHERE status <> 'deleted'`
	v42RequeueStmt       = `INSERT INTO contact_segment_queue \(email, queued_at\)\s+SELECT DISTINCT cl.email, NOW\(\)\s+FROM contact_lists cl\s+JOIN lists l ON l.id = cl.list_id\s+WHERE cl.deleted_at IS NOT NULL\s+AND l.deleted_at IS NULL\s+AND cl.list_id = ANY\(\$1\)\s+ORDER BY cl.email\s+ON CONFLICT \(email\) DO UPDATE SET queued_at = EXCLUDED.queued_at`
)

func expectNoFunctions(mock sqlmock.Sqlmock) {
	mock.ExpectQuery(v42FunctionsQuery).WillReturnRows(sqlmock.NewRows([]string{"oid", "pg_get_functiondef"}))
}

// expectSegmentPatch expects the in-place patch of stored segment SQL: the list condition's
// old ending gains the removed-row filter, only where the SQL holds a list condition and not
// the filter yet. generated_args is not touched: the filter takes no parameter.
func expectSegmentPatch(mock sqlmock.Sqlmock) *sqlmock.ExpectedExec {
	return mock.ExpectExec(v42SegmentPatchStmt).WithArgs(
		" AND l.deleted_at IS NULL)",
		" AND cl.deleted_at IS NULL AND l.deleted_at IS NULL)",
		"FROM contact_lists cl JOIN lists l",
		"cl.deleted_at IS NULL",
	)
}

func expectNoSegments(mock sqlmock.Sqlmock) {
	expectSegmentPatch(mock).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(v42SegmentTreesQuery).WillReturnRows(sqlmock.NewRows([]string{"tree"}))
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

func v42Branch(operator string, leaves ...*domain.TreeNode) *domain.TreeNode {
	return &domain.TreeNode{Kind: "branch", Branch: &domain.TreeNodeBranch{Operator: operator, Leaves: leaves}}
}

func v42ContactLeaf(filters ...*domain.DimensionFilter) *domain.TreeNode {
	return &domain.TreeNode{
		Kind: "leaf",
		Leaf: &domain.TreeNodeLeaf{Source: "contacts", Contact: &domain.ContactCondition{Filters: filters}},
	}
}

// The patch is a text substitution, so it is right only if the list condition's old
// ending occurs in compiled SQL nowhere but at the end of a list condition. That holds
// beside a condition on each other source (contacts, including inlined JSON keys,
// contact_timeline and custom_events_goals), in a segment and in a trigger function,
// and the selections pick the old output and skip the new.
func TestV42Migration_PatchTurnsPre42OutputIntoTheCurrentOutput(t *testing.T) {
	active := "active"
	unsubscribed := "unsubscribed"
	anytime := "anytime"
	country := &domain.DimensionFilter{FieldName: "country", FieldType: "string", Operator: "equals", StringValues: []string{"FR"}}
	trees := map[string]*domain.TreeNode{
		"in a list with a status": v42ListLeaf("in", "listX", &active),
		"not in a list":           v42ListLeaf("not_in", "listX", nil),
		"two list conditions": v42Branch("and",
			v42ListLeaf("in", "listX", &active),
			v42ListLeaf("not_in", "listY", nil)),
		"a list or a contact field": v42Branch("or",
			v42ListLeaf("in", "listX", nil),
			v42ContactLeaf(country)),
		"a list and a JSON key": v42Branch("and",
			v42ListLeaf("in", "listX", &active),
			v42ContactLeaf(&domain.DimensionFilter{FieldName: "custom_json_1", FieldType: "json", Operator: "is_not_set", JSONPath: []string{"tier"}}),
			v42ContactLeaf(&domain.DimensionFilter{FieldName: "custom_json_2", FieldType: "string", Operator: "equals", JSONPath: []string{"plan", "name"}, StringValues: []string{"gold"}})),
		"a list and a timeline count": v42Branch("and",
			v42ListLeaf("in", "listX", nil),
			&domain.TreeNode{Kind: "leaf", Leaf: &domain.TreeNodeLeaf{
				Source:          "contact_timeline",
				ContactTimeline: &domain.ContactTimelineCondition{Kind: "email_opened", CountOperator: "at_least", CountValue: 5, TimeframeOperator: &anytime},
			}}),
		"a list and a goal": v42Branch("and",
			v42ListLeaf("not_in", "listX", nil),
			&domain.TreeNode{Kind: "leaf", Leaf: &domain.TreeNodeLeaf{
				Source:           "custom_events_goals",
				CustomEventsGoal: &domain.CustomEventsGoalCondition{GoalType: "purchase", AggregateOperator: "count", Operator: "gte", Value: 1, TimeframeOperator: "anytime"},
			}}),
		"nested branches": v42Branch("or",
			v42Branch("and", v42ListLeaf("in", "listX", &active), v42ContactLeaf(country)),
			v42Branch("and", v42ListLeaf("not_in", "listY", nil), v42ListLeaf("in", "listZ", &unsubscribed))),
	}

	for name, tree := range trees {
		t.Run(name, func(t *testing.T) {
			current, pre42 := v42SegmentSQL(t, tree)
			fnCurrent, fnPre42 := v42TriggerFunction(t, "auto1", tree)

			for kind, out := range map[string][2]string{"segment": {current, pre42}, "trigger function": {fnCurrent, fnPre42}} {
				now, old := out[0], out[1]
				require.NotEqual(t, now, old, "%s: the fix must change this output", kind)
				assert.Equal(t, now, strings.ReplaceAll(old, v42ListPredicateOld, v42ListPredicateNew), kind)
				assert.Contains(t, old, v42ListConditionTag, kind)
				assert.NotContains(t, old, v42RemovedRowFilter, kind)
				assert.Contains(t, now, v42RemovedRowFilter, kind)
			}
		})
	}
}

// Part 5's order: timeout, both contact_lists functions, the automation functions, the
// segments, and the re-queue last.
func TestV42Migration_RunsItsStepsInOrder(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	active := "active"
	newFn, oldFn := v42TriggerFunction(t, "auto1", v42ListLeaf("in", "listX", &active))

	expectV42Prelude(mock)
	mock.ExpectQuery(v42FunctionsQuery).WillReturnRows(functionRows(oldFn))
	expectPatch(mock, newFn, nil)
	expectSegmentPatch(mock).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(v42SegmentTreesQuery).WillReturnRows(treeRows(t, v42ListLeaf("in", "listX", &active)))
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

// Stored segment SQL is patched by one statement, whatever the tree holds, and nothing
// else is written to segments: no recompile, no generated_args, no version or status. List
// IDs come from every non-deleted segment's tree, including one whose SQL was already
// new; trees that don't parse or no longer compile contribute nothing; the re-queue runs
// with exactly those IDs.
func TestV42Migration_PatchesSegmentsInPlaceAndRequeuesTheirRemovedMembers(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	active := "active"
	// Already recompiled by v36/v37 on a database upgraded from below 37.
	inW := v42ListLeaf("in", "listW", nil)
	// Parses, but no longer compiles (a contact_lists leaf without a list ID).
	broken := v42ListLeaf("in", "", nil)
	_, _, buildErr := service.NewQueryBuilder().BuildSQL(broken)
	require.Error(t, buildErr, "the fixture must be a tree the builder refuses")

	expectV42Prelude(mock)
	expectNoFunctions(mock)
	expectSegmentPatch(mock).WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectQuery(v42SegmentTreesQuery).WillReturnRows(treeRows(t,
		v42ListLeaf("in", "listX", &active),
		v42ListLeaf("not_in", "listX", nil),
		inW,
		[]byte("{not json"),
		broken,
		v42ContactsOnlyTree(),
	))
	mock.ExpectExec(v42RequeueStmt).
		WithArgs(pq.Array([]string{"listW", "listX"})).
		WillReturnResult(sqlmock.NewResult(0, 2))

	require.NoError(t, runV42(t, db))
	require.NoError(t, mock.ExpectationsWereMet())
}

// No segment names a list: nothing is re-queued.
func TestV42Migration_NoListConditionMeansNoRequeue(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	expectV42Prelude(mock)
	expectNoFunctions(mock)
	expectSegmentPatch(mock).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(v42SegmentTreesQuery).WillReturnRows(treeRows(t, v42ContactsOnlyTree()))

	require.NoError(t, runV42(t, db))
	require.NoError(t, mock.ExpectationsWereMet())
}

// A failure outside the per-function savepoints aborts the workspace migration, so the
// transaction rolls back and the version is not recorded.
func TestV42Migration_PropagatesErrors(t *testing.T) {
	t.Run("disabling statement_timeout", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		mock.ExpectExec(exactly("SET LOCAL statement_timeout = 0")).WillReturnError(errors.New("boom"))

		err = runV42(t, db)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to disable statement_timeout")
		require.NoError(t, mock.ExpectationsWereMet())
	})

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

	t.Run("patching segments", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		expectV42Prelude(mock)
		expectNoFunctions(mock)
		expectSegmentPatch(mock).WillReturnError(errors.New("boom"))

		err = runV42(t, db)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to patch segment queries")
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("reading segment trees", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		expectV42Prelude(mock)
		expectNoFunctions(mock)
		expectSegmentPatch(mock).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectQuery(v42SegmentTreesQuery).WillReturnError(errors.New("boom"))

		err = runV42(t, db)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to query segment trees")
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("re-queueing", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		expectV42Prelude(mock)
		expectNoFunctions(mock)
		expectSegmentPatch(mock).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectQuery(v42SegmentTreesQuery).WillReturnRows(treeRows(t, v42ListLeaf("in", "listW", nil)))
		mock.ExpectExec(v42RequeueStmt).WillReturnError(errors.New("boom"))

		err = runV42(t, db)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "re-queue")
		require.NoError(t, mock.ExpectationsWereMet())
	})
}

// A list condition nested in a branch names its list as a top-level one does, and an
// empty branch or a null leaf names none.
func TestV42Migration_RequeuesListsNamedInsideBranches(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	expectV42Prelude(mock)
	expectNoFunctions(mock)
	expectSegmentPatch(mock).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(v42SegmentTreesQuery).WillReturnRows(treeRows(t,
		v42Branch("and",
			v42ContactsOnlyTree(),
			v42Branch("or", v42ListLeaf("in", "listA", nil), v42ListLeaf("not_in", "listB", nil)),
		),
		v42Branch("and"),
		[]byte(`{"kind":"branch","branch":{"operator":"and","leaves":[null]}}`),
	))
	mock.ExpectExec(v42RequeueStmt).
		WithArgs(pq.Array([]string{"listA", "listB"})).
		WillReturnResult(sqlmock.NewResult(0, 2))

	require.NoError(t, runV42(t, db))
	require.NoError(t, mock.ExpectationsWereMet())
}

// A selected function whose list condition does not end the way the old builder ended it
// is left alone: no savepoint, no DDL.
func TestV42Migration_FunctionWithoutTheOldEndingIsNotRewritten(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	active := "active"
	_, oldFn := v42TriggerFunction(t, "auto1", v42ListLeaf("in", "listX", &active))
	noOldEnding := strings.ReplaceAll(oldFn, " AND l.deleted_at IS NULL", "")
	require.Contains(t, noOldEnding, "FROM contact_lists cl JOIN lists l")
	require.NotContains(t, noOldEnding, " AND l.deleted_at IS NULL)")

	expectV42Prelude(mock)
	mock.ExpectQuery(v42FunctionsQuery).WillReturnRows(functionRows(noOldEnding))
	expectNoSegments(mock)

	require.NoError(t, runV42(t, db))
	require.NoError(t, mock.ExpectationsWereMet())
}

// Failures reading the functions, managing a savepoint or reading segment trees abort the
// workspace migration too; only a failed patch of one function is contained.
func TestV42Migration_PropagatesReadAndSavepointErrors(t *testing.T) {
	active := "active"
	newFn, oldFn := v42TriggerFunction(t, "auto1", v42ListLeaf("in", "listX", &active))
	boom := errors.New("boom")

	tests := []struct {
		name    string
		expect  func(mock sqlmock.Sqlmock)
		wantErr string
	}{
		{
			name: "querying automation trigger functions",
			expect: func(mock sqlmock.Sqlmock) {
				expectV42Prelude(mock)
				mock.ExpectQuery(v42FunctionsQuery).WillReturnError(boom)
			},
			wantErr: "failed to query automation trigger functions",
		},
		{
			name: "scanning an automation trigger function",
			expect: func(mock sqlmock.Sqlmock) {
				expectV42Prelude(mock)
				mock.ExpectQuery(v42FunctionsQuery).WillReturnRows(
					sqlmock.NewRows([]string{"oid", "pg_get_functiondef"}).AddRow("not an oid", oldFn)).
					RowsWillBeClosed()
			},
			wantErr: "failed to scan automation trigger function",
		},
		{
			name: "iterating automation trigger functions",
			expect: func(mock sqlmock.Sqlmock) {
				expectV42Prelude(mock)
				mock.ExpectQuery(v42FunctionsQuery).WillReturnRows(functionRows(oldFn).RowError(0, boom))
			},
			wantErr: "error iterating automation trigger functions",
		},
		{
			name: "creating a savepoint",
			expect: func(mock sqlmock.Sqlmock) {
				expectV42Prelude(mock)
				mock.ExpectQuery(v42FunctionsQuery).WillReturnRows(functionRows(oldFn))
				mock.ExpectExec(exactly("SAVEPOINT v42_automation_trigger")).WillReturnError(boom)
			},
			wantErr: "failed to create savepoint",
		},
		{
			name: "rolling back to a savepoint",
			expect: func(mock sqlmock.Sqlmock) {
				expectV42Prelude(mock)
				mock.ExpectQuery(v42FunctionsQuery).WillReturnRows(functionRows(oldFn))
				mock.ExpectExec(exactly("SAVEPOINT v42_automation_trigger")).WillReturnResult(sqlmock.NewResult(0, 0))
				mock.ExpectExec(exactly(newFn)).WillReturnError(errors.New("syntax error"))
				mock.ExpectExec(exactly("ROLLBACK TO SAVEPOINT v42_automation_trigger")).WillReturnError(boom)
			},
			wantErr: "failed to roll back savepoint",
		},
		{
			name: "releasing a savepoint",
			expect: func(mock sqlmock.Sqlmock) {
				expectV42Prelude(mock)
				mock.ExpectQuery(v42FunctionsQuery).WillReturnRows(functionRows(oldFn))
				mock.ExpectExec(exactly("SAVEPOINT v42_automation_trigger")).WillReturnResult(sqlmock.NewResult(0, 0))
				mock.ExpectExec(exactly(newFn)).WillReturnResult(sqlmock.NewResult(0, 0))
				mock.ExpectExec(exactly("RELEASE SAVEPOINT v42_automation_trigger")).WillReturnError(boom)
			},
			wantErr: "failed to release savepoint",
		},
		{
			name: "scanning a segment tree",
			expect: func(mock sqlmock.Sqlmock) {
				expectV42Prelude(mock)
				expectNoFunctions(mock)
				expectSegmentPatch(mock).WillReturnResult(sqlmock.NewResult(0, 0))
				// A column count that doesn't match the destinations is what reliably
				// fails a scan into *[]byte.
				mock.ExpectQuery(v42SegmentTreesQuery).WillReturnRows(
					sqlmock.NewRows([]string{"tree", "extra"}).AddRow([]byte("{}"), "x")).
					RowsWillBeClosed()
			},
			wantErr: "failed to scan segment tree",
		},
		{
			name: "iterating segment trees",
			expect: func(mock sqlmock.Sqlmock) {
				expectV42Prelude(mock)
				expectNoFunctions(mock)
				expectSegmentPatch(mock).WillReturnResult(sqlmock.NewResult(0, 0))
				mock.ExpectQuery(v42SegmentTreesQuery).WillReturnRows(
					treeRows(t, v42ListLeaf("in", "listW", nil)).RowError(0, boom))
			},
			wantErr: "error iterating segment trees",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()

			tt.expect(mock)

			err = runV42(t, db)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
