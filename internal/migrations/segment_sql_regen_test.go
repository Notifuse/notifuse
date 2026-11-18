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

var segmentSelect = regexp.QuoteMeta(`SELECT id, tree, generated_sql, generated_args FROM segments`)
var segmentUpdate = regexp.QuoteMeta(`UPDATE segments`)

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
			WithArgs(current, sqlmock.AnyArg(), "seg1").
			WillReturnResult(sqlmock.NewResult(0, 1))

		result, err := RegenerateSegmentSQL(context.Background(), db)
		require.NoError(t, err)
		assert.Equal(t, 1, result.Recompiled)
		assert.Empty(t, result.Skipped)
		assert.Contains(t, current, "#>> '{}'")
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
		mock.ExpectExec(segmentUpdate).WithArgs(current, sqlmock.AnyArg(), "seg1").
			WillReturnResult(sqlmock.NewResult(0, 1))

		result, err := RegenerateSegmentSQL(context.Background(), db)
		require.NoError(t, err)
		assert.Equal(t, 1, result.Recompiled)
		require.NoError(t, mock.ExpectationsWereMet())
	})
}
