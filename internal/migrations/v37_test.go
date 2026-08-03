package migrations

import (
	"context"
	"database/sql/driver"
	"encoding/json"
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
	mock.ExpectExec("ALTER TABLE contact_timeline").WillReturnResult(sqlmock.NewResult(0, 0))

	m := &V37Migration{}
	require.NoError(t, m.UpdateWorkspace(context.Background(), &config.Config{}, &domain.Workspace{ID: "ws1"}, db))
	require.NoError(t, mock.ExpectationsWereMet())
}
