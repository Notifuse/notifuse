package migrations

import (
	"context"
	"fmt"

	"github.com/Notifuse/notifuse/internal/database/schema"
)

// v45WebSessionsUpdatedAtIndexSQL creates the BRIN index on web_sessions.updated_at that the
// live views read, on the partitioned parent alone (schema.WebSessionsUpdatedAtParentIndex).
// It reads no partition and writes one catalog entry; the index stays invalid, so the planner
// ignores it, until the web analytics maintenance worker has built and attached the index of
// each existing partition without blocking writes (BuildWebSessionsUpdatedAtIndex, a few
// minutes after the start). A partition created later gets its index at creation. A new
// workspace builds the whole index at once on empty tables (schema.WebSessionsUpdatedAtIndex).
//
// Building it here instead, on every partition, would hold back every write to web_sessions
// until the workspace commits, for a read of every partition: CREATE INDEX CONCURRENTLY
// cannot run in the migration's transaction, nor on a partitioned table.
//
// The statement still takes a SHARE lock on the parent, held until the workspace commits.
// The part runs last in the workspace update, so that lock is taken just before the commit
// and holds the sessions' writes back only for that commit, where taking it first would hold
// them back during the parts before it. It runs only when the index does not exist yet, so a
// rerun takes no lock.
const v45WebSessionsUpdatedAtIndexSQL = `DO $$
BEGIN
	IF to_regclass('` + schema.WebSessionsUpdatedAtIndexName + `') IS NULL THEN
		` + schema.WebSessionsUpdatedAtParentIndex + `;
	END IF;
END $$`

// webSessionsUpdatedAtWorkspaceV45Statements is the updated_at index part of the v45 workspace
// migration. lock_timeout bounds the wait for the SHARE lock behind a transaction already
// writing to web_sessions: past 5 seconds the migration fails and is retried at the next
// start. It is saved first and restored afterwards, so a part added after this one keeps the
// value the transaction had.
func webSessionsUpdatedAtWorkspaceV45Statements() []string {
	return []string{
		`SELECT set_config('notifuse.v45_web_sessions_lock_timeout', current_setting('lock_timeout'), true)`,
		`SET LOCAL lock_timeout = '5s'`,
		v45WebSessionsUpdatedAtIndexSQL,
		`SELECT set_config('lock_timeout', current_setting('notifuse.v45_web_sessions_lock_timeout'), true)`,
	}
}

// applyWebSessionsUpdatedAtWorkspaceV45 runs the updated_at index part of the v45 workspace
// migration in the migration's transaction. It is idempotent.
func applyWebSessionsUpdatedAtWorkspaceV45(ctx context.Context, db DBExecutor) error {
	for _, statement := range webSessionsUpdatedAtWorkspaceV45Statements() {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("web_sessions updated_at index (v45): %w", err)
		}
	}
	return nil
}
