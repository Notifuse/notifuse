package migrations

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// RegenerateAutomationTriggerBodies is what cmd/regen-json-filters runs on a database already at
// 44: it reports the automations it left as they were, which the migrations only swallow.
func TestRegenerateAutomationTriggerBodies(t *testing.T) {
	t.Run("replaces a body and reports nothing", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		mock.ExpectQuery(v38HealQuery).WillReturnRows(v38HealRows().AddRow("cartauto", "node1", v44CustomEventTriggerConfig(t), v38InstalledDef))
		mock.ExpectExec("^SAVEPOINT regen_automation_trigger_bodies$").WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("^CREATE OR REPLACE FUNCTION automation_trigger_cartauto").WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("^RELEASE SAVEPOINT regen_automation_trigger_bodies$").WillReturnResult(sqlmock.NewResult(0, 0))

		skipped, err := RegenerateAutomationTriggerBodies(context.Background(), db)
		require.NoError(t, err)
		assert.Empty(t, skipped)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("a body that fails rolls back to its savepoint and is reported", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		mock.ExpectQuery(v38HealQuery).WillReturnRows(v38HealRows().AddRow("cartauto", "node1", v44CustomEventTriggerConfig(t), v38InstalledDef))
		mock.ExpectExec("^SAVEPOINT regen_automation_trigger_bodies$").WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("^CREATE OR REPLACE FUNCTION automation_trigger_cartauto").
			WillReturnError(errors.New("function pg_input_is_valid(text, unknown) does not exist"))
		mock.ExpectExec("^ROLLBACK TO SAVEPOINT regen_automation_trigger_bodies$").WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("^RELEASE SAVEPOINT regen_automation_trigger_bodies$").WillReturnResult(sqlmock.NewResult(0, 0))

		skipped, err := RegenerateAutomationTriggerBodies(context.Background(), db)
		require.NoError(t, err)
		assert.Equal(t, map[string]string{"cartauto": "function pg_input_is_valid(text, unknown) does not exist"}, skipped)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("a configuration that does not parse is reported, with nothing run", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		mock.ExpectQuery(v38HealQuery).WillReturnRows(v38HealRows().AddRow("broken", "node1", []byte(`not json`), v38InstalledDef))

		skipped, err := RegenerateAutomationTriggerBodies(context.Background(), db)
		require.NoError(t, err)
		assert.Contains(t, skipped, "broken")
		require.NoError(t, mock.ExpectationsWereMet())
	})
}
