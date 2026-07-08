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
	assert.False(t, m.HasSystemUpdate())
}

func TestV36Migration_HasWorkspaceUpdate(t *testing.T) {
	m := &V36Migration{}
	assert.True(t, m.HasWorkspaceUpdate())
}

func TestV36Migration_ShouldRestartServer(t *testing.T) {
	m := &V36Migration{}
	assert.False(t, m.ShouldRestartServer())
}

func TestV36Migration_UpdateSystem(t *testing.T) {
	m := &V36Migration{}
	cfg := &config.Config{}

	db, _, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	err = m.UpdateSystem(context.Background(), cfg, db)
	assert.NoError(t, err)
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

func TestV36Migration_UpdateWorkspace_RegeneratesEmailTrigger(t *testing.T) {
	m := &V36Migration{}
	cfg := &config.Config{}
	workspace := &domain.Workspace{ID: "ws1"}

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	rows := sqlmock.NewRows([]string{"id", "root_node_id", "trigger_config"}).
		AddRow("clickauto", "node1", triggerConfigJSON(t, "email.clicked"))
	mock.ExpectQuery("SELECT id, root_node_id, trigger_config").WillReturnRows(rows)

	// Each automation's DDL is wrapped in a savepoint. The CREATE TRIGGER must reference
	// the translated timeline kind (click_email), never the dotted form (email.clicked).
	mock.ExpectExec("SAVEPOINT v36_regen").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("DROP TRIGGER IF EXISTS automation_trigger_clickauto").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("DROP FUNCTION IF EXISTS automation_trigger_clickauto").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("CREATE OR REPLACE FUNCTION automation_trigger_clickauto").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("NEW.kind = 'click_email'").
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

	// A live email.clicked automation that carries trigger-level Conditions would compile
	// to a WHEN clause with a subquery, which Postgres rejects. It must be skipped (no
	// regeneration statements) so the migration cannot fail and block startup.
	cfgJSON, err := json.Marshal(domain.TimelineTriggerConfig{
		EventKind:  "email.clicked",
		Frequency:  domain.TriggerFrequencyEveryTime,
		Conditions: &domain.TreeNode{Kind: "leaf", Leaf: &domain.TreeNodeLeaf{Source: "contacts"}},
	})
	require.NoError(t, err)

	rows := sqlmock.NewRows([]string{"id", "root_node_id", "trigger_config"}).
		AddRow("condauto", "node1", cfgJSON)
	mock.ExpectQuery("SELECT id, root_node_id, trigger_config").WillReturnRows(rows)
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

	rows := sqlmock.NewRows([]string{"id", "root_node_id", "trigger_config"}).
		AddRow("failauto", "node1", triggerConfigJSON(t, "email.clicked"))
	mock.ExpectQuery("SELECT id, root_node_id, trigger_config").WillReturnRows(rows)

	// CREATE TRIGGER fails; the savepoint is rolled back and released, and the migration
	// still succeeds (the automation is left as-is rather than aborting the whole run).
	mock.ExpectExec("SAVEPOINT v36_regen").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("DROP TRIGGER IF EXISTS automation_trigger_failauto").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("DROP FUNCTION IF EXISTS automation_trigger_failauto").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("CREATE OR REPLACE FUNCTION automation_trigger_failauto").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("NEW.kind = 'click_email'").
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

	// A live contact.created automation is unaffected by the kind mismatch and must be
	// left untouched (no regeneration statements issued).
	rows := sqlmock.NewRows([]string{"id", "root_node_id", "trigger_config"}).
		AddRow("contactauto", "node1", triggerConfigJSON(t, "contact.created"))
	mock.ExpectQuery("SELECT id, root_node_id, trigger_config").WillReturnRows(rows)

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

	rows := sqlmock.NewRows([]string{"id", "root_node_id", "trigger_config"})
	mock.ExpectQuery("SELECT id, root_node_id, trigger_config").WillReturnRows(rows)

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

	// email.delivered is intentionally not mapped (loop risk), so v36 regenerates it to the
	// verbatim (still-inert) kind — a harmless no-op that must not fail the migration.
	rows := sqlmock.NewRows([]string{"id", "root_node_id", "trigger_config"}).
		AddRow("delivauto", "node1", triggerConfigJSON(t, "email.delivered"))
	mock.ExpectQuery("SELECT id, root_node_id, trigger_config").WillReturnRows(rows)

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

func TestV36Migration_UpdateWorkspace_QueryError(t *testing.T) {
	m := &V36Migration{}
	cfg := &config.Config{}
	workspace := &domain.Workspace{ID: "ws1"}

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery("SELECT id, root_node_id, trigger_config").
		WillReturnError(assert.AnError)

	err = m.UpdateWorkspace(context.Background(), cfg, workspace, db)
	assert.Error(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}
