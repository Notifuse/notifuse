package migrations

import (
	"context"
	"fmt"

	"github.com/Notifuse/notifuse/internal/database/schema"
)

// v45ExternalIDUniqueSQL makes message_history.external_id unique, so that two
// transactional.send calls carrying the same external_id cannot both send.
//
// An empty external_id is no key: the messages stored with an empty string get NULL first,
// which the index leaves out, so they are neither deduplicated against each other nor refused
// by the index.
//
// Existing duplicates are resolved next: for each external_id held by several messages, the
// oldest (created_at, then id) keeps it and the others get NULL. The rows themselves stay:
// each one is an email that was sent, with its timeline entries, its events and its
// statistics, and only the key it was sent under is given up.
//
// The whole of it runs only when the index does not exist yet, so a rerun takes no lock and
// reads nothing. The first run takes a SHARE lock on message_history before the two
// UPDATEs: the build needs it anyway, and taking it first keeps a send from adding an empty
// key or a duplicate before the build, which would fail the build and the migration. Writes
// to message_history then wait until the workspace commits, for two reads of the table and
// the build of an index that holds only the keyed rows. The UPDATEs write only the empty keys
// and the duplicates, and clearing external_id fires nothing in the message_history
// triggers, which act on status timestamps and links. CREATE INDEX CONCURRENTLY cannot run
// in the migration's transaction, and the migration manager has no step after the commit.
const v45ExternalIDUniqueSQL = `DO $$
BEGIN
	IF to_regclass('` + schema.MessageHistoryExternalIDIndexName + `') IS NULL THEN
		LOCK TABLE message_history IN SHARE MODE;
		UPDATE message_history SET external_id = NULL WHERE external_id = '';
		UPDATE message_history AS m
		SET external_id = NULL
		FROM (
			SELECT id, ROW_NUMBER() OVER (PARTITION BY external_id ORDER BY created_at, id) AS position
			FROM message_history
			WHERE external_id IS NOT NULL
		) AS ranked
		WHERE m.id = ranked.id AND ranked.position > 1;
		` + schema.MessageHistoryExternalIDIndex + `;
	END IF;
END $$`

// externalIDWorkspaceV45Statements is the external_id part of the v45 workspace migration.
// statement_timeout is lifted around the build, since a build cut short would fail the
// migration again at every start, and lock_timeout bounds the wait for the SHARE lock behind
// a transaction already writing to message_history: past 5 seconds the migration fails and is
// retried at the next start. Both are saved first and restored afterwards, so the parts that
// run after this one keep the values the transaction had, whatever set them.
func externalIDWorkspaceV45Statements() []string {
	return []string{
		`SELECT set_config('notifuse.v45_external_id_statement_timeout', current_setting('statement_timeout'), true)`,
		`SELECT set_config('notifuse.v45_external_id_lock_timeout', current_setting('lock_timeout'), true)`,
		`SET LOCAL statement_timeout = 0`,
		`SET LOCAL lock_timeout = '5s'`,
		v45ExternalIDUniqueSQL,
		`SELECT set_config('lock_timeout', current_setting('notifuse.v45_external_id_lock_timeout'), true)`,
		`SELECT set_config('statement_timeout', current_setting('notifuse.v45_external_id_statement_timeout'), true)`,
	}
}

// applyExternalIDWorkspaceV45 runs the external_id part of the v45 workspace migration in the
// migration's transaction. It is idempotent.
func applyExternalIDWorkspaceV45(ctx context.Context, db DBExecutor) error {
	for _, statement := range externalIDWorkspaceV45Statements() {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("unique external_id (v45): %w", err)
		}
	}
	return nil
}
