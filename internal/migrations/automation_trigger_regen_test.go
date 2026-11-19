package migrations

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
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

		result, err := RegenerateAutomationTriggerBodies(context.Background(), db)
		require.NoError(t, err)
		assert.Equal(t, 1, result.Replaced)
		assert.Empty(t, result.Skipped)
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

		result, err := RegenerateAutomationTriggerBodies(context.Background(), db)
		require.NoError(t, err)
		assert.Equal(t, 0, result.Replaced)
		assert.Equal(t, map[string]string{"cartauto": "function pg_input_is_valid(text, unknown) does not exist"}, result.Skipped)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("a lock timeout fails the run instead of skipping the automation", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		mock.ExpectQuery(v38HealQuery).WillReturnRows(v38HealRows().AddRow("cartauto", "node1", v44CustomEventTriggerConfig(t), v38InstalledDef))
		mock.ExpectExec("^SAVEPOINT regen_automation_trigger_bodies$").WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("^CREATE OR REPLACE FUNCTION automation_trigger_cartauto").
			WillReturnError(&pq.Error{Code: "55P03", Message: "canceling statement due to lock timeout"})

		result, err := RegenerateAutomationTriggerBodies(context.Background(), db)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "automation cartauto")
		assert.Contains(t, err.Error(), "lock timeout")
		assert.Equal(t, 0, result.Replaced)
		assert.Empty(t, result.Skipped, "a valid automation is not reported as left behind")
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("a configuration that does not parse is reported, with nothing run", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		mock.ExpectQuery(v38HealQuery).WillReturnRows(v38HealRows().AddRow("broken", "node1", []byte(`not json`), v38InstalledDef))

		result, err := RegenerateAutomationTriggerBodies(context.Background(), db)
		require.NoError(t, err)
		assert.Contains(t, result.Skipped, "broken")
		require.NoError(t, mock.ExpectationsWereMet())
	})
}
