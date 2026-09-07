package schema

// Audit log: one row per control-plane action, in the SYSTEM database only.
//
// The table lives beside workspaces rather than inside each workspace database
// on purpose. A workspace database is dropped when the workspace is deleted, and
// "who deleted the workspace, and what did they do in the week before" is the
// first question an audit trail exists to answer; rows keyed by workspace_id in
// the system database survive that deletion. It also means one migration, one
// backup, and a plain query for the root user's deployment-wide view.
//
// Rows are append-only. The guard trigger below refuses UPDATE and TRUNCATE
// unconditionally and DELETE unless the transaction-local setting named by
// AuditPurgeGUC is 'on', which only audit_logs_purge and
// audit_logs_purge_orphans set. That stops the application role and casual
// SQL; it does not stop the table owner or a superuser, who can disable the
// trigger, set the GUC by hand or drop the table. The guarantee is "the
// application cannot rewrite history", not "nobody can", and the docs say so.
//
// Shared verbatim by internal/database/init.go (fresh installs) and the v41
// migration (upgrades), so the two cannot drift.

// AuditPurgeGUC is the transaction-local setting the purge functions switch on
// before deleting. The guard trigger reads the same name; keep them together.
const AuditPurgeGUC = "notifuse.audit_purge"

// AuditLogsTableDefinitions returns the DDL for the audit_logs table, its
// indexes, the append-only guard and the two purge functions, in an order that
// is safe to run more than once. The DO block that attaches the triggers
// resolves audit_logs::regclass, so it must stay after the CREATE TABLE.
func AuditLogsTableDefinitions() []string {
	return []string{
		`CREATE TABLE IF NOT EXISTS audit_logs (
	id VARCHAR(32) PRIMARY KEY,
	occurred_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
	workspace_id VARCHAR(32),
	action VARCHAR(100) NOT NULL,
	category VARCHAR(50) NOT NULL,
	outcome VARCHAR(20) NOT NULL,
	status_code INTEGER,
	actor_type VARCHAR(20) NOT NULL,
	actor_id VARCHAR(64),
	actor_email VARCHAR(255),
	actor_name VARCHAR(255),
	actor_role VARCHAR(20),
	auth_method VARCHAR(20),
	target_type VARCHAR(50),
	target_id VARCHAR(255),
	target_name VARCHAR(255),
	ip_address VARCHAR(45),
	user_agent TEXT,
	request_id VARCHAR(64),
	changes JSONB,
	metadata JSONB
)`,
		`CREATE INDEX IF NOT EXISTS idx_audit_logs_workspace_time ON audit_logs (workspace_id, occurred_at DESC, id DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_audit_logs_time ON audit_logs (occurred_at DESC, id DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_audit_logs_actor ON audit_logs (actor_id, occurred_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_audit_logs_action ON audit_logs (action, occurred_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_audit_logs_target ON audit_logs (target_type, target_id, occurred_at DESC)`,
		// The guard. One function serves both triggers: TG_OP tells it which
		// statement it is refusing, and only a DELETE under the purge setting is
		// let through. insufficient_privilege (42501) rather than a generic raise,
		// so a caller can tell "the log refused you" from "the query was wrong".
		`CREATE OR REPLACE FUNCTION audit_logs_guard() RETURNS TRIGGER AS $$
BEGIN
	IF TG_OP = 'DELETE' AND current_setting('` + AuditPurgeGUC + `', true) = 'on' THEN
		RETURN OLD;
	END IF;
	RAISE EXCEPTION 'audit_logs is append-only: % refused', TG_OP
		USING ERRCODE = 'insufficient_privilege';
END;
$$ LANGUAGE plpgsql`,
		`DO $$
BEGIN
	IF NOT EXISTS (
		SELECT 1 FROM pg_trigger
		WHERE tgname = 'audit_logs_guard' AND tgrelid = 'audit_logs'::regclass
	) THEN
		CREATE TRIGGER audit_logs_guard
			BEFORE UPDATE OR DELETE ON audit_logs
			FOR EACH ROW EXECUTE FUNCTION audit_logs_guard();
	END IF;
	IF NOT EXISTS (
		SELECT 1 FROM pg_trigger
		WHERE tgname = 'audit_logs_no_truncate' AND tgrelid = 'audit_logs'::regclass
	) THEN
		CREATE TRIGGER audit_logs_no_truncate
			BEFORE TRUNCATE ON audit_logs
			FOR EACH STATEMENT EXECUTE FUNCTION audit_logs_guard();
	END IF;
END $$`,
		// Retention. set_config(..., true) is transaction-local, so the setting
		// cannot leak past the statement that called the function; it is still
		// switched back off before returning so that a caller who wraps this in a
		// longer transaction cannot follow it with a bare DELETE.
		//
		// A NULL p_workspace_id purges the deployment-level rows (workspace_id IS
		// NULL) and nothing else; a workspace id purges that workspace's rows and
		// nothing else.
		`CREATE OR REPLACE FUNCTION audit_logs_purge(p_workspace_id VARCHAR, p_before TIMESTAMPTZ) RETURNS BIGINT AS $$
DECLARE
	deleted BIGINT;
BEGIN
	PERFORM set_config('` + AuditPurgeGUC + `', 'on', true);
	DELETE FROM audit_logs
	WHERE occurred_at < p_before
	  AND ((p_workspace_id IS NULL AND workspace_id IS NULL) OR workspace_id = p_workspace_id);
	GET DIAGNOSTICS deleted = ROW_COUNT;
	PERFORM set_config('` + AuditPurgeGUC + `', 'off', true);
	RETURN deleted;
END;
$$ LANGUAGE plpgsql`,
		// Rows of workspaces that no longer exist have no retention setting of
		// their own; the caller passes the ids that still exist and the deployment
		// default as the cutoff. An empty p_keep means "every workspace is gone",
		// which is why the caller must not call this when listing workspaces
		// failed — a NULL p_keep deletes nothing, an empty one deletes everything
		// older than p_before.
		`CREATE OR REPLACE FUNCTION audit_logs_purge_orphans(p_keep VARCHAR[], p_before TIMESTAMPTZ) RETURNS BIGINT AS $$
DECLARE
	deleted BIGINT;
BEGIN
	PERFORM set_config('` + AuditPurgeGUC + `', 'on', true);
	DELETE FROM audit_logs
	WHERE occurred_at < p_before
	  AND workspace_id IS NOT NULL
	  AND NOT (workspace_id = ANY(p_keep));
	GET DIAGNOSTICS deleted = ROW_COUNT;
	PERFORM set_config('` + AuditPurgeGUC + `', 'off', true);
	RETURN deleted;
END;
$$ LANGUAGE plpgsql`,
	}
}
