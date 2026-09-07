package schema

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuditLogsTableDefinitions_Idempotent(t *testing.T) {
	defs := AuditLogsTableDefinitions()
	require.NotEmpty(t, defs)

	for _, stmt := range defs {
		idempotent := strings.Contains(stmt, "IF NOT EXISTS") ||
			strings.Contains(stmt, "CREATE OR REPLACE")
		assert.True(t, idempotent, "every statement must be safe to run twice: %s", stmt)
		// House rule from system_tables.go: constraints never live in the CREATE
		// TABLE, and this table references nothing anyway — a row must outlive the
		// workspace and the user it names.
		assert.NotContains(t, stmt, "REFERENCES")
		assert.NotContains(t, stmt, " CHECK ")
	}

	joined := strings.Join(defs, "\n")
	assert.Contains(t, joined, "CREATE TABLE IF NOT EXISTS audit_logs (")
	assert.Contains(t, joined, "workspace_id VARCHAR(32),", "workspace_id is nullable: deployment-level rows have none")
	assert.NotContains(t, joined, "PARTITION")
}

func TestAuditLogsTableDefinitions_TriggersResolveRegclassAfterCreateTable(t *testing.T) {
	defs := AuditLogsTableDefinitions()

	createTable, attach := -1, -1
	for i, stmt := range defs {
		if strings.HasPrefix(stmt, "CREATE TABLE IF NOT EXISTS audit_logs") {
			createTable = i
		}
		if strings.Contains(stmt, "'audit_logs'::regclass") {
			attach = i
		}
	}
	require.NotEqual(t, -1, createTable)
	require.NotEqual(t, -1, attach, "the trigger DO block must guard on pg_trigger with ::regclass")
	// ::regclass is resolved when the statement is planned; before the table
	// exists it raises instead of skipping.
	assert.Greater(t, attach, createTable)
}

func TestAuditLogsGuard_RefusesEverythingButPurgeDeletes(t *testing.T) {
	joined := strings.Join(AuditLogsTableDefinitions(), "\n")

	guardStart := strings.Index(joined, "CREATE OR REPLACE FUNCTION audit_logs_guard()")
	require.NotEqual(t, -1, guardStart)
	guard := joined[guardStart:]
	guard = guard[:strings.Index(guard, "LANGUAGE plpgsql")]

	// The only way out without an exception is a DELETE under the purge setting.
	assert.Contains(t, guard, "TG_OP = 'DELETE' AND current_setting('"+AuditPurgeGUC+"', true) = 'on'")
	assert.Contains(t, guard, "RETURN OLD")
	assert.Contains(t, guard, "RAISE EXCEPTION")
	assert.Contains(t, guard, "insufficient_privilege")

	// Both triggers attach the same function; TRUNCATE is statement-level.
	assert.Contains(t, joined, "BEFORE UPDATE OR DELETE ON audit_logs")
	assert.Contains(t, joined, "FOR EACH ROW EXECUTE FUNCTION audit_logs_guard()")
	assert.Contains(t, joined, "BEFORE TRUNCATE ON audit_logs")
	assert.Contains(t, joined, "FOR EACH STATEMENT EXECUTE FUNCTION audit_logs_guard()")
}

func TestAuditLogsPurgeFunctions_SetTheGUCTransactionLocally(t *testing.T) {
	joined := strings.Join(AuditLogsTableDefinitions(), "\n")

	for _, fn := range []string{"audit_logs_purge(p_workspace_id VARCHAR, p_before TIMESTAMPTZ)", "audit_logs_purge_orphans(p_keep VARCHAR[], p_before TIMESTAMPTZ)"} {
		start := strings.Index(joined, "CREATE OR REPLACE FUNCTION "+fn)
		require.NotEqual(t, -1, start, fn)
		body := joined[start:]
		body = body[:strings.Index(body, "LANGUAGE plpgsql")]

		// is_local = true: the setting dies with the transaction, and it is
		// switched off again before the function returns.
		assert.Contains(t, body, "set_config('"+AuditPurgeGUC+"', 'on', true)", fn)
		assert.Contains(t, body, "set_config('"+AuditPurgeGUC+"', 'off', true)", fn)
		assert.Contains(t, body, "occurred_at < p_before", fn)
		assert.Contains(t, body, "GET DIAGNOSTICS deleted = ROW_COUNT", fn)
	}

	// Scoping: a NULL workspace purges only deployment rows; orphans never touch
	// deployment rows.
	assert.Contains(t, joined, "(p_workspace_id IS NULL AND workspace_id IS NULL) OR workspace_id = p_workspace_id")
	assert.Contains(t, joined, "workspace_id IS NOT NULL\n\t  AND NOT (workspace_id = ANY(p_keep))")
}

func TestAuditLogsTableDefinitions_StatementCount(t *testing.T) {
	defs := AuditLogsTableDefinitions()
	require.Len(t, defs, 10, "table, five indexes, guard function, trigger block, two purge functions")
}

func TestTableNames_IncludesAuditLogs(t *testing.T) {
	assert.Contains(t, TableNames, "audit_logs")
}
