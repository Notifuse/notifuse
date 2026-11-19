//go:build integration
// +build integration

package integration

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/app"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/migrations"
	"github.com/Notifuse/notifuse/internal/service"
	"github.com/Notifuse/notifuse/tests/testutil"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// saveBeforeFirstWrite runs a save of its own just before the first write of the recompilation,
// once its read is done: what a live instance saving the segment at that moment does.
type saveBeforeFirstWrite struct {
	migrations.DBExecutor
	save func() error
	done bool
}

func (s *saveBeforeFirstWrite) ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	if !s.done {
		s.done = true
		if err := s.save(); err != nil {
			return nil, err
		}
	}
	return s.DBExecutor.ExecContext(ctx, query, args...)
}

// TestRegenerateSegmentSQLKeepsASegmentSavedMeanwhile pins the guard of the segment
// recompilation on a real PostgreSQL: a segment saved between the read and the write, with a new
// value in the same condition (same query text, new tree and arguments), is read again and gets
// the 44.0 query of what was saved, never the arguments of the tree read first.
func TestRegenerateSegmentSQLKeepsASegmentSavedMeanwhile(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	bg := context.Background()
	workspace, err := suite.DataFactory.CreateWorkspace()
	require.NoError(t, err)
	db, err := suite.DataFactory.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)

	planTree := func(value string) *domain.TreeNode {
		return &domain.TreeNode{
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
	}
	// What the builder compiled before 44.0, for a tree: the query the recompilation replaces.
	compiledBefore44 := func(t *testing.T, value string) (treeJSON, oldSQL, argsJSON string) {
		t.Helper()
		tree := planTree(value)
		current, args, err := service.NewQueryBuilder().BuildSQL(tree)
		require.NoError(t, err)
		old := strings.ReplaceAll(current, "(custom_json_1['plan'] #>> '{}')", "custom_json_1['plan']::text")
		require.NotEqual(t, current, old)
		tb, err := json.Marshal(tree)
		require.NoError(t, err)
		ab, err := json.Marshal(args)
		require.NoError(t, err)
		return string(tb), old, string(ab)
	}

	tag := strings.ReplaceAll(uuid.New().String(), "-", "")[:8]
	savedID, untouchedID := "rgsv"+tag, "rgun"+tag
	goldTree, goldOld, goldArgs := compiledBefore44(t, "gold")
	for _, id := range []string{savedID, untouchedID} {
		_, err = db.ExecContext(bg, `
			INSERT INTO segments (id, name, color, tree, timezone, version, status, generated_sql, generated_args, db_created_at, db_updated_at)
			VALUES ($1, $1, '#1677ff', $2, 'UTC', 1, 'active', $3, $4, NOW(), NOW())`,
			id, goldTree, goldOld, goldArgs)
		require.NoError(t, err)
	}

	// The live save: "silver" in the same condition, compiled by the 43.x builder, so the
	// stored query text does not change and only the tree and the arguments do.
	silverTree, silverOld, silverArgs := compiledBefore44(t, "silver")
	require.Equal(t, goldOld, silverOld, "the save must leave the query text as it was")
	exec := &saveBeforeFirstWrite{DBExecutor: db, save: func() error {
		_, err := db.ExecContext(bg, `UPDATE segments SET tree = $2, generated_sql = $3, generated_args = $4 WHERE id = $1`,
			savedID, silverTree, silverOld, silverArgs)
		return err
	}}

	result, err := migrations.RegenerateSegmentSQL(bg, exec)
	require.NoError(t, err)
	require.True(t, exec.done, "the save ran before the first write")
	assert.Equal(t, 2, result.Recompiled, "both segments get the 44.0 query, the saved one from its new tree")
	assert.Empty(t, result.Skipped)

	var tree, args, savedSQL string
	var recompute sql.NullTime
	require.NoError(t, db.QueryRowContext(bg,
		`SELECT tree::text, generated_sql, generated_args::text, recompute_after FROM segments WHERE id = $1`, savedID).
		Scan(&tree, &savedSQL, &args, &recompute))
	assert.Contains(t, tree, "silver", "the tree saved meanwhile is kept")
	assert.Contains(t, args, "silver", "the arguments come from the tree saved meanwhile")
	assert.NotContains(t, args, "gold")
	assert.Contains(t, savedSQL, "#>> '{}'", "the saved segment gets the 44.0 query too")
	assert.True(t, recompute.Valid, "and is marked for a rebuild")

	var untouchedSQL string
	require.NoError(t, db.QueryRowContext(bg,
		`SELECT generated_sql FROM segments WHERE id = $1 AND recompute_after IS NOT NULL`, untouchedID).Scan(&untouchedSQL))
	assert.Contains(t, untouchedSQL, "#>> '{}'", "the untouched segment gets the 44.0 query")

	// A rerun finds nothing left to recompile.
	again, err := migrations.RegenerateSegmentSQL(bg, db)
	require.NoError(t, err)
	assert.Equal(t, 0, again.Recompiled)
}
