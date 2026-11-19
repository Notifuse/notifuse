package migrations

import (
	"context"
	"encoding/json"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Every segment but the deleted ones: a building one is recompiled too, its build re-reading the
// row on each run (SegmentBuildProcessor.rescheduleRecompute keeps the mark for the next build).
var segmentSelect = `SELECT id, tree, generated_sql, generated_args FROM segments\s+WHERE status <> 'deleted'\s+ORDER BY id`
var segmentUpdate = regexp.QuoteMeta(`UPDATE segments`) +
	`\s+SET generated_sql = \$1, generated_args = \$2, recompute_after = NOW\(\), db_updated_at = NOW\(\)\s+WHERE id = \$3 AND status <> 'deleted' AND generated_sql IS NOT DISTINCT FROM \$4\s+AND tree = \$5::jsonb AND generated_args IS NOT DISTINCT FROM \$6::jsonb`

func jsonEqualsTree(t *testing.T, value string) ([]byte, string, []byte) {
	t.Helper()
	tree := &domain.TreeNode{
		Kind: "leaf",
		Leaf: &domain.TreeNodeLeaf{
			Source: "contacts",
			Contact: &domain.ContactCondition{Filters: []*domain.DimensionFilter{{
				FieldName:    "custom_json_1",
				FieldType:    "string",
				Operator:     "equals",
				JSONPath:     []string{"plan"},
				StringValues: []string{value},
			}}},
		},
	}
	treeJSON, err := json.Marshal(tree)
	require.NoError(t, err)
	sql, args, err := service.NewQueryBuilder().BuildSQL(tree)
	require.NoError(t, err)
	argsJSON, err := json.Marshal(args)
	require.NoError(t, err)
	return treeJSON, sql, argsJSON
}

func TestRegenerateSegmentSQL(t *testing.T) {
	t.Run("a segment whose stored query changed is stored again and marked for a rebuild", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		treeJSON, current, argsJSON := jsonEqualsTree(t, "gold")
		// What 43.x compiled: the JSON text, quotes included.
		old := "SELECT email FROM contacts WHERE custom_json_1['plan']::text = $1"
		mock.ExpectQuery(segmentSelect).WillReturnRows(
			sqlmock.NewRows([]string{"id", "tree", "generated_sql", "generated_args"}).
				AddRow("seg1", treeJSON, old, argsJSON))
		mock.ExpectExec(segmentUpdate).
			WithArgs(current, sqlmock.AnyArg(), "seg1", old, string(treeJSON), string(argsJSON)).
			WillReturnResult(sqlmock.NewResult(0, 1))

		result, err := RegenerateSegmentSQL(context.Background(), db)
		require.NoError(t, err)
		assert.Equal(t, 1, result.Recompiled)
		assert.Empty(t, result.Skipped)
		assert.Contains(t, current, "#>> '{}'")
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("a segment saved while it is recompiled is read again and recompiled from what was saved", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		goldTree, _, goldArgs := jsonEqualsTree(t, "gold")
		silverTree, current, silverArgs := jsonEqualsTree(t, "silver")
		old := "SELECT email FROM contacts WHERE custom_json_1['plan']::text = $1"
		mock.ExpectQuery(segmentSelect).WillReturnRows(
			sqlmock.NewRows([]string{"id", "tree", "generated_sql", "generated_args"}).
				AddRow("seg1", goldTree, old, goldArgs))
		// The guard matches nothing: a live instance saved "silver" meanwhile.
		mock.ExpectExec(segmentUpdate).WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), "seg1", old, string(goldTree), string(goldArgs)).
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectQuery(`SELECT tree, generated_sql, generated_args FROM segments WHERE id = \$1 AND status <> 'deleted'`).
			WithArgs("seg1").
			WillReturnRows(sqlmock.NewRows([]string{"tree", "generated_sql", "generated_args"}).AddRow(silverTree, old, silverArgs))
		mock.ExpectExec(segmentUpdate).WithArgs(current, sqlmock.AnyArg(), "seg1", old, string(silverTree), string(silverArgs)).
			WillReturnResult(sqlmock.NewResult(0, 1))

		result, err := RegenerateSegmentSQL(context.Background(), db)
		require.NoError(t, err)
		assert.Equal(t, 1, result.Recompiled)
		assert.Empty(t, result.Skipped)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("a segment deleted while it is recompiled is reported", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		treeJSON, _, argsJSON := jsonEqualsTree(t, "gold")
		old := "SELECT email FROM contacts WHERE custom_json_1['plan']::text = $1"
		mock.ExpectQuery(segmentSelect).WillReturnRows(
			sqlmock.NewRows([]string{"id", "tree", "generated_sql", "generated_args"}).AddRow("seg1", treeJSON, old, argsJSON))
		mock.ExpectExec(segmentUpdate).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectQuery(`SELECT tree, generated_sql, generated_args FROM segments`).WithArgs("seg1").
			WillReturnRows(sqlmock.NewRows([]string{"tree", "generated_sql", "generated_args"}))

		result, err := RegenerateSegmentSQL(context.Background(), db)
		require.NoError(t, err)
		assert.Equal(t, 0, result.Recompiled)
		assert.Equal(t, "deleted while it was recompiled", result.Skipped["seg1"])
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("a segment saved again at every attempt fails the run", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		treeJSON, _, argsJSON := jsonEqualsTree(t, "gold")
		old := "SELECT email FROM contacts WHERE custom_json_1['plan']::text = $1"
		mock.ExpectQuery(segmentSelect).WillReturnRows(
			sqlmock.NewRows([]string{"id", "tree", "generated_sql", "generated_args"}).AddRow("seg1", treeJSON, old, argsJSON))
		for i := 1; i <= segmentRegenAttempts; i++ {
			mock.ExpectExec(segmentUpdate).WillReturnResult(sqlmock.NewResult(0, 0))
			if i < segmentRegenAttempts {
				mock.ExpectQuery(`SELECT tree, generated_sql, generated_args FROM segments`).WithArgs("seg1").
					WillReturnRows(sqlmock.NewRows([]string{"tree", "generated_sql", "generated_args"}).AddRow(treeJSON, old, argsJSON))
			}
		}

		_, err = RegenerateSegmentSQL(context.Background(), db)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "seg1")
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("an unchanged segment is not written, so a rerun does nothing", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		treeJSON, current, argsJSON := jsonEqualsTree(t, "gold")
		// jsonb hands the arguments back with its own spacing.
		spaced := []byte(`[ "gold" ]`)
		require.JSONEq(t, string(argsJSON), string(spaced))
		mock.ExpectQuery(segmentSelect).WillReturnRows(
			sqlmock.NewRows([]string{"id", "tree", "generated_sql", "generated_args"}).
				AddRow("seg1", treeJSON, current, spaced))

		result, err := RegenerateSegmentSQL(context.Background(), db)
		require.NoError(t, err)
		assert.Equal(t, 0, result.Recompiled)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	// The service stores no arguments as SQL NULL: a segment such as « phone is set » is the same
	// query, not one to rebuild.
	t.Run("a query without arguments is the same whether they are NULL, null or empty", func(t *testing.T) {
		tree := &domain.TreeNode{Kind: "leaf", Leaf: &domain.TreeNodeLeaf{Source: "contacts",
			Contact: &domain.ContactCondition{Filters: []*domain.DimensionFilter{{FieldName: "phone", FieldType: "string", Operator: "is_set"}}}}}
		treeJSON, err := json.Marshal(tree)
		require.NoError(t, err)
		current, args, err := service.NewQueryBuilder().BuildSQL(tree)
		require.NoError(t, err)
		require.Empty(t, args)

		for _, stored := range [][]byte{nil, []byte(`null`), []byte(`[]`)} {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			mock.ExpectQuery(segmentSelect).WillReturnRows(
				sqlmock.NewRows([]string{"id", "tree", "generated_sql", "generated_args"}).AddRow("seg1", treeJSON, current, stored))
			result, err := RegenerateSegmentSQL(context.Background(), db)
			require.NoError(t, err)
			assert.Equal(t, 0, result.Recompiled, "stored %q", stored)
			require.NoError(t, mock.ExpectationsWereMet())
			db.Close()
		}
	})

	t.Run("a tree that does not compile keeps its query and is reported", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		broken := []byte(`{"kind":"leaf","leaf":{"source":"contacts","contact":{"filters":[{"field_name":"no_such_field","field_type":"string","operator":"equals","string_values":["x"]}]}}}`)
		mock.ExpectQuery(segmentSelect).WillReturnRows(
			sqlmock.NewRows([]string{"id", "tree", "generated_sql", "generated_args"}).
				AddRow("broken", broken, "SELECT 1", []byte(`[]`)).
				AddRow("garbled", []byte(`not json`), "SELECT 1", []byte(`[]`)))

		result, err := RegenerateSegmentSQL(context.Background(), db)
		require.NoError(t, err)
		assert.Equal(t, 0, result.Recompiled)
		assert.Contains(t, result.Skipped, "broken")
		assert.Contains(t, result.Skipped, "garbled")
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("a segment never compiled gets its query", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		treeJSON, current, _ := jsonEqualsTree(t, "gold")
		mock.ExpectQuery(segmentSelect).WillReturnRows(
			sqlmock.NewRows([]string{"id", "tree", "generated_sql", "generated_args"}).
				AddRow("seg1", treeJSON, nil, nil))
		mock.ExpectExec(segmentUpdate).WithArgs(current, sqlmock.AnyArg(), "seg1", nil, string(treeJSON), nil).
			WillReturnResult(sqlmock.NewResult(0, 1))

		result, err := RegenerateSegmentSQL(context.Background(), db)
		require.NoError(t, err)
		assert.Equal(t, 1, result.Recompiled)
		require.NoError(t, mock.ExpectationsWereMet())
	})
}
