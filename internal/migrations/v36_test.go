package migrations

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
)

func TestV36Migration_GetMajorVersion(t *testing.T) {
	m := &V36Migration{}
	assert.Equal(t, 36.0, m.GetMajorVersion())
}

func TestV36Migration_HasSystemUpdate(t *testing.T) {
	m := &V36Migration{}
	assert.True(t, m.HasSystemUpdate())
}

func TestV36Migration_HasWorkspaceUpdate(t *testing.T) {
	m := &V36Migration{}
	assert.True(t, m.HasWorkspaceUpdate())
}

func TestV36Migration_ShouldRestartServer(t *testing.T) {
	m := &V36Migration{}
	assert.False(t, m.ShouldRestartServer(), "no restart requested before the system update ran")
}

func TestV36Migration_UpdateSystem_HealsScopes(t *testing.T) {
	m := &V36Migration{}
	cfg := &config.Config{}

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	// A legacy bare-"openid" row is rewritten to the full default; the restart
	// signal fires only because a row was actually healed.
	mock.ExpectExec(`UPDATE settings SET value = 'openid email profile'`).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err = m.UpdateSystem(context.Background(), cfg, db)
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
	assert.True(t, m.ShouldRestartServer(), "healing a row must request a restart")
}

func TestV36Migration_UpdateSystem_NoRowHealed_NoRestart(t *testing.T) {
	m := &V36Migration{}
	cfg := &config.Config{}

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	// The common case: no broken row exists — the upgrade must not force a
	// pointless full process restart.
	mock.ExpectExec(`UPDATE settings SET value = 'openid email profile'`).
		WillReturnResult(sqlmock.NewResult(0, 0))

	err = m.UpdateSystem(context.Background(), cfg, db)
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
	assert.False(t, m.ShouldRestartServer(), "no heal, no restart")
}

func TestV36Migration_UpdateSystem_Error(t *testing.T) {
	m := &V36Migration{}
	cfg := &config.Config{}

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectExec(`UPDATE settings`).WillReturnError(assert.AnError)

	err = m.UpdateSystem(context.Background(), cfg, db)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "v36: failed to heal oidc_scopes")
}

func triggerConfigJSON(t *testing.T, kind string) []byte {
	t.Helper()
	b, err := json.Marshal(domain.TimelineTriggerConfig{
		EventKind: kind,
		Frequency: domain.TriggerFrequencyEveryTime,
	})
	require.NoError(t, err)
	return b
}

// expectV36Preamble sets up the statements every UpdateWorkspace run issues before it reaches
// automations: recreate the writer trigger, rewrite legacy timeline rows, and read segments.
// segmentRows is what the segments query returns (use emptySegmentRows for the common case).
func expectV36Preamble(mock sqlmock.Sqlmock, segmentRows *sqlmock.Rows) {
	mock.ExpectExec("SET LOCAL statement_timeout").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("CREATE OR REPLACE FUNCTION track_message_history_changes").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("UPDATE contact_timeline SET kind").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("SELECT id, tree FROM segments").WillReturnRows(segmentRows)
}

func emptySegmentRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "tree"})
}

func TestRenameTimelineKindsInTree(t *testing.T) {
	// Nested tree: an AND branch with one renamed timeline leaf, one already-dotted timeline
	// leaf, and one non-timeline leaf. Only the legacy kind should change.
	tree := &domain.TreeNode{
		Kind: "branch",
		Branch: &domain.TreeNodeBranch{
			Operator: "and",
			Leaves: []*domain.TreeNode{
				{Kind: "leaf", Leaf: &domain.TreeNodeLeaf{
					Source:          "contact_timeline",
					ContactTimeline: &domain.ContactTimelineCondition{Kind: "click_email", CountOperator: "at_least", CountValue: 1},
				}},
				{Kind: "leaf", Leaf: &domain.TreeNodeLeaf{
					Source:          "contact_timeline",
					ContactTimeline: &domain.ContactTimelineCondition{Kind: "list.subscribed", CountOperator: "at_least", CountValue: 1},
				}},
				{Kind: "leaf", Leaf: &domain.TreeNodeLeaf{Source: "contacts"}},
			},
		},
	}

	changed := renameTimelineKindsInTree(tree)
	assert.True(t, changed)
	assert.Equal(t, "email.clicked", tree.Branch.Leaves[0].Leaf.ContactTimeline.Kind)
	assert.Equal(t, "list.subscribed", tree.Branch.Leaves[1].Leaf.ContactTimeline.Kind, "non-renamed kinds untouched")

	// A tree with no legacy timeline kinds is reported unchanged.
	unchanged := &domain.TreeNode{Kind: "leaf", Leaf: &domain.TreeNodeLeaf{Source: "contacts"}}
	assert.False(t, renameTimelineKindsInTree(unchanged))
}

func TestRewriteTimelineKindsInJSON(t *testing.T) {
	// A filter-node-shaped document with a nested contact_timeline condition (the branch/filter
	// nodes reuse the segment tree). The legacy kind must be renamed wherever it is nested.
	raw := []byte(`{"config":{"conditions":{"kind":"branch","branch":{"operator":"and","leaves":[` +
		`{"kind":"leaf","leaf":{"source":"contact_timeline","contact_timeline":{"kind":"click_email","count_operator":"at_least","count_value":1}}},` +
		`{"kind":"leaf","leaf":{"source":"contacts"}}]}}}}`)
	out, changed := rewriteTimelineKindsInJSON(raw)
	assert.True(t, changed)
	assert.Contains(t, string(out), "email.clicked")
	assert.NotContains(t, string(out), "click_email")

	// A document with a matching literal in a NON-timeline field (a contact filter matching the
	// text "click_email") must be left untouched — only contact_timeline.kind is renamed.
	safe := []byte(`{"leaf":{"source":"contacts","contact":{"filters":[{"field_name":"custom_string_1","string_values":["click_email"]}]}}}`)
	_, changedSafe := rewriteTimelineKindsInJSON(safe)
	assert.False(t, changedSafe, "must not touch a non-timeline string value")

	// Empty / no-match inputs are no-ops.
	_, changedEmpty := rewriteTimelineKindsInJSON(nil)
	assert.False(t, changedEmpty)
}

func TestV36Migration_UpdateWorkspace_RewritesAutomationNodeConditions(t *testing.T) {
	m := &V36Migration{}
	cfg := &config.Config{}
	workspace := &domain.Workspace{ID: "ws1"}

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	expectV36Preamble(mock, emptySegmentRows())

	// A PAUSED contact.updated automation whose filter node condition references the legacy
	// click_email kind: its nodes JSONB must still be rewritten (a paused automation is
	// re-activated later and its condition evaluated at runtime). Non-live + non-email, so no
	// trigger DDL is regenerated.
	nodesJSON := []byte(`[{"id":"n1","type":"filter","config":{"conditions":{"kind":"leaf","leaf":` +
		`{"source":"contact_timeline","contact_timeline":{"kind":"click_email","count_operator":"at_least","count_value":1}}}}}]`)
	rows := sqlmock.NewRows([]string{"id", "status", "root_node_id", "trigger_config", "nodes"}).
		AddRow("filterauto", "paused", "node1", triggerConfigJSON(t, "contact.updated"), nodesJSON)
	mock.ExpectQuery("SELECT id, status, root_node_id, trigger_config").WillReturnRows(rows)
	mock.ExpectExec("UPDATE automations SET trigger_config").
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), "filterauto").
		WillReturnResult(sqlmock.NewResult(0, 1))

	err = m.UpdateWorkspace(context.Background(), cfg, workspace, db)
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestV36Migration_UpdateWorkspace_RewritesTimelineAndTrigger(t *testing.T) {
	m := &V36Migration{}
	cfg := &config.Config{}
	workspace := &domain.Workspace{ID: "ws1"}

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	// Trigger recreate + timeline rewrite + empty segments + no live automations.
	expectV36Preamble(mock, emptySegmentRows())
	mock.ExpectQuery("SELECT id, status, root_node_id, trigger_config").
		WillReturnRows(sqlmock.NewRows([]string{"id", "status", "root_node_id", "trigger_config", "nodes"}))

	err = m.UpdateWorkspace(context.Background(), cfg, workspace, db)
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestV36Migration_UpdateWorkspace_RewritesSegmentTree(t *testing.T) {
	m := &V36Migration{}
	cfg := &config.Config{}
	workspace := &domain.Workspace{ID: "ws1"}

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	// A stored segment filtering on the legacy click_email kind must be rewritten (tree +
	// regenerated compiled query) so it keeps matching after the timeline rename.
	tree := domain.TreeNode{
		Kind: "branch",
		Branch: &domain.TreeNodeBranch{
			Operator: "and",
			Leaves: []*domain.TreeNode{
				{Kind: "leaf", Leaf: &domain.TreeNodeLeaf{
					Source:          "contact_timeline",
					ContactTimeline: &domain.ContactTimelineCondition{Kind: "click_email", CountOperator: "at_least", CountValue: 1},
				}},
			},
		},
	}
	treeJSON, err := json.Marshal(tree)
	require.NoError(t, err)

	segRows := sqlmock.NewRows([]string{"id", "tree"}).AddRow("seg1", treeJSON)
	expectV36Preamble(mock, segRows)
	mock.ExpectExec("UPDATE segments SET tree").
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), "seg1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT id, status, root_node_id, trigger_config").
		WillReturnRows(sqlmock.NewRows([]string{"id", "status", "root_node_id", "trigger_config", "nodes"}))

	err = m.UpdateWorkspace(context.Background(), cfg, workspace, db)
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestV36Migration_UpdateWorkspace_LeavesUnrelatedSegmentUntouched(t *testing.T) {
	m := &V36Migration{}
	cfg := &config.Config{}
	workspace := &domain.Workspace{ID: "ws1"}

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	// A segment with no timeline condition must not be rewritten (no UPDATE segments).
	tree := domain.TreeNode{Kind: "leaf", Leaf: &domain.TreeNodeLeaf{Source: "contacts"}}
	treeJSON, err := json.Marshal(tree)
	require.NoError(t, err)

	segRows := sqlmock.NewRows([]string{"id", "tree"}).AddRow("seg1", treeJSON)
	expectV36Preamble(mock, segRows)
	// No UPDATE segments expected.
	mock.ExpectQuery("SELECT id, status, root_node_id, trigger_config").
		WillReturnRows(sqlmock.NewRows([]string{"id", "status", "root_node_id", "trigger_config", "nodes"}))

	err = m.UpdateWorkspace(context.Background(), cfg, workspace, db)
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestV36Migration_UpdateWorkspace_RegeneratesEmailTrigger(t *testing.T) {
	m := &V36Migration{}
	cfg := &config.Config{}
	workspace := &domain.Workspace{ID: "ws1"}

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	expectV36Preamble(mock, emptySegmentRows())

	rows := sqlmock.NewRows([]string{"id", "status", "root_node_id", "trigger_config", "nodes"}).
		AddRow("clickauto", "live", "node1", triggerConfigJSON(t, "email.clicked"), []byte("[]"))
	mock.ExpectQuery("SELECT id, status, root_node_id, trigger_config").WillReturnRows(rows)

	// Each automation's DDL is wrapped in a savepoint. The CREATE TRIGGER must now reference
	// the dotted timeline kind (email.clicked) verbatim — the translation map is gone.
	mock.ExpectExec("SAVEPOINT v36_regen").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("DROP TRIGGER IF EXISTS automation_trigger_clickauto").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("DROP FUNCTION IF EXISTS automation_trigger_clickauto").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("CREATE OR REPLACE FUNCTION automation_trigger_clickauto").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("NEW.kind = 'email.clicked'").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("RELEASE SAVEPOINT v36_regen").WillReturnResult(sqlmock.NewResult(0, 0))

	err = m.UpdateWorkspace(context.Background(), cfg, workspace, db)
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestV36Migration_UpdateWorkspace_SkipsAutomationWithConditions(t *testing.T) {
	m := &V36Migration{}
	cfg := &config.Config{}
	workspace := &domain.Workspace{ID: "ws1"}

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	expectV36Preamble(mock, emptySegmentRows())

	// A live email.clicked automation that carries trigger-level Conditions would compile
	// to a WHEN clause with a subquery, which Postgres rejects. It must be skipped (no
	// regeneration statements) so the migration cannot fail and block startup.
	cfgJSON, err := json.Marshal(domain.TimelineTriggerConfig{
		EventKind:  "email.clicked",
		Frequency:  domain.TriggerFrequencyEveryTime,
		Conditions: &domain.TreeNode{Kind: "leaf", Leaf: &domain.TreeNodeLeaf{Source: "contacts"}},
	})
	require.NoError(t, err)

	rows := sqlmock.NewRows([]string{"id", "status", "root_node_id", "trigger_config", "nodes"}).
		AddRow("condauto", "live", "node1", cfgJSON, []byte("[]"))
	mock.ExpectQuery("SELECT id, status, root_node_id, trigger_config").WillReturnRows(rows)
	// No SAVEPOINT / DDL expected — the automation is skipped.

	err = m.UpdateWorkspace(context.Background(), cfg, workspace, db)
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestV36Migration_UpdateWorkspace_RegenFailureIsSkippedNotFatal(t *testing.T) {
	m := &V36Migration{}
	cfg := &config.Config{}
	workspace := &domain.Workspace{ID: "ws1"}

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	expectV36Preamble(mock, emptySegmentRows())

	rows := sqlmock.NewRows([]string{"id", "status", "root_node_id", "trigger_config", "nodes"}).
		AddRow("failauto", "live", "node1", triggerConfigJSON(t, "email.clicked"), []byte("[]"))
	mock.ExpectQuery("SELECT id, status, root_node_id, trigger_config").WillReturnRows(rows)

	// CREATE TRIGGER fails; the savepoint is rolled back and released, and the migration
	// still succeeds (the automation is left as-is rather than aborting the whole run).
	mock.ExpectExec("SAVEPOINT v36_regen").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("DROP TRIGGER IF EXISTS automation_trigger_failauto").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("DROP FUNCTION IF EXISTS automation_trigger_failauto").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("CREATE OR REPLACE FUNCTION automation_trigger_failauto").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("NEW.kind = 'email.clicked'").
		WillReturnError(assert.AnError)
	mock.ExpectExec("ROLLBACK TO SAVEPOINT v36_regen").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("RELEASE SAVEPOINT v36_regen").WillReturnResult(sqlmock.NewResult(0, 0))

	err = m.UpdateWorkspace(context.Background(), cfg, workspace, db)
	assert.NoError(t, err, "a single automation's regen failure must not fail the migration")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestV36Migration_UpdateWorkspace_SkipsNonEmailTriggers(t *testing.T) {
	m := &V36Migration{}
	cfg := &config.Config{}
	workspace := &domain.Workspace{ID: "ws1"}

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	expectV36Preamble(mock, emptySegmentRows())

	// A live contact.created automation is unaffected by the rename and must be left
	// untouched (no regeneration statements issued).
	rows := sqlmock.NewRows([]string{"id", "status", "root_node_id", "trigger_config", "nodes"}).
		AddRow("contactauto", "live", "node1", triggerConfigJSON(t, "contact.created"), []byte("[]"))
	mock.ExpectQuery("SELECT id, status, root_node_id, trigger_config").WillReturnRows(rows)

	err = m.UpdateWorkspace(context.Background(), cfg, workspace, db)
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestV36Migration_UpdateWorkspace_UnmappedEmailKindRegeneratesVerbatim(t *testing.T) {
	m := &V36Migration{}
	cfg := &config.Config{}
	workspace := &domain.Workspace{ID: "ws1"}

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	expectV36Preamble(mock, emptySegmentRows())

	// email.delivered is not a valid automation kind anymore, but a legacy live automation
	// could still carry it; v36 regenerates it verbatim (still inert) without failing.
	rows := sqlmock.NewRows([]string{"id", "status", "root_node_id", "trigger_config", "nodes"}).
		AddRow("delivauto", "live", "node1", triggerConfigJSON(t, "email.delivered"), []byte("[]"))
	mock.ExpectQuery("SELECT id, status, root_node_id, trigger_config").WillReturnRows(rows)

	mock.ExpectExec("SAVEPOINT v36_regen").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("DROP TRIGGER IF EXISTS automation_trigger_delivauto").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("DROP FUNCTION IF EXISTS automation_trigger_delivauto").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("CREATE OR REPLACE FUNCTION automation_trigger_delivauto").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("NEW.kind = 'email.delivered'").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("RELEASE SAVEPOINT v36_regen").WillReturnResult(sqlmock.NewResult(0, 0))

	err = m.UpdateWorkspace(context.Background(), cfg, workspace, db)
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestV36Migration_UpdateWorkspace_NoLiveAutomations(t *testing.T) {
	m := &V36Migration{}
	cfg := &config.Config{}
	workspace := &domain.Workspace{ID: "ws1"}

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	expectV36Preamble(mock, emptySegmentRows())

	rows := sqlmock.NewRows([]string{"id", "status", "root_node_id", "trigger_config", "nodes"})
	mock.ExpectQuery("SELECT id, status, root_node_id, trigger_config").WillReturnRows(rows)

	err = m.UpdateWorkspace(context.Background(), cfg, workspace, db)
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestV36Migration_UpdateWorkspace_TriggerRecreateError(t *testing.T) {
	m := &V36Migration{}
	cfg := &config.Config{}
	workspace := &domain.Workspace{ID: "ws1"}

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	// statement_timeout is disabled first; a failure recreating the writer trigger then aborts
	// the migration immediately.
	mock.ExpectExec("SET LOCAL statement_timeout").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("CREATE OR REPLACE FUNCTION track_message_history_changes").
		WillReturnError(assert.AnError)

	err = m.UpdateWorkspace(context.Background(), cfg, workspace, db)
	assert.Error(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestV36Migration_UpdateWorkspace_AutomationsQueryError(t *testing.T) {
	m := &V36Migration{}
	cfg := &config.Config{}
	workspace := &domain.Workspace{ID: "ws1"}

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	expectV36Preamble(mock, emptySegmentRows())
	mock.ExpectQuery("SELECT id, status, root_node_id, trigger_config").
		WillReturnError(assert.AnError)

	err = m.UpdateWorkspace(context.Background(), cfg, workspace, db)
	assert.Error(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}
