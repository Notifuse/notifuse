package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/lib/pq"
)

// permanentTaskRestoreLockID is the transaction advisory lock RestorePermanentTasks holds, so
// that two restores never create the same task twice: tasks has no unique key on
// (workspace_id, type). The other keys of this family: 4_242_000_001 rotates SECRET_KEY,
// 4_242_000_002 checks DKIM records, 4_242_000_003 runs migrations.
const permanentTaskRestoreLockID int64 = 4_242_000_004

// restoreStuckPermanentTasksSQL sets pending, due now, every permanent task that the
// scheduler would never pick up again: failed or completed, paused with no run date, or
// running with no lease and no write for ten minutes. A running task with a lease is left to
// its run; GetNextBatch takes it again once the lease has passed. One with no lease counts as
// held, as in notHeld, until it has been silent longer than any run lasts (MaxRuntime plus
// taskExecutionGrace is 80 s for these tasks).
const restoreStuckPermanentTasksSQL = `
	UPDATE tasks
	SET status = 'pending', retry_count = 0, next_run_after = $2::timestamp, timeout_after = NULL,
		error_message = NULL, updated_at = $2::timestamp
	WHERE type = ANY($1)
	  AND ($3::text = '' OR workspace_id = $3)
	  AND (
		status IN ('failed', 'completed')
		OR (status = 'paused' AND next_run_after IS NULL)
		OR (status = 'running' AND timeout_after IS NULL AND updated_at <= $2::timestamp - INTERVAL '10 minutes')
	  )`

// removeOrphanPermanentTasksSQL deletes the permanent tasks of workspaces that no longer
// exist: they would run, and fail on the missing database, for ever.
const removeOrphanPermanentTasksSQL = `
	DELETE FROM tasks
	WHERE type = ANY($1)
	  AND NOT EXISTS (SELECT 1 FROM workspaces w WHERE w.id = tasks.workspace_id)`

// createMissingPermanentTasksSQL creates one task of a permanent type for every workspace
// that has none. The parameters in the select list are cast: a parameter there has no column
// to take its type from. FOR KEY SHARE makes it wait for a workspace deletion that holds the
// row; once that commits, the row is gone and nothing is created for it. Unlike FOR SHARE, it
// lets ordinary updates of the row through. The times come from the caller, in UTC: the
// columns have no time zone, and CURRENT_TIMESTAMP would store the server's local time.
const createMissingPermanentTasksSQL = `
	INSERT INTO tasks (
		id, workspace_id, type, status, progress, state, created_at, updated_at,
		next_run_after, max_runtime, max_retries, retry_count, retry_interval
	)
	SELECT gen_random_uuid(), w.id, $1::varchar, 'pending', 0, $2::jsonb,
		$3::timestamp, $3::timestamp, $3::timestamp, $5::integer, $6::integer, 0, $7::integer
	FROM workspaces w
	WHERE NOT EXISTS (SELECT 1 FROM tasks t WHERE t.workspace_id = w.id AND t.type = $1::varchar)
	  AND ($4::text = '' OR w.id = $4)
	FOR KEY SHARE OF w`

// RestorePermanentTasks makes the permanent tasks (domain.PermanentTaskTypes) of every
// workspace runnable, or of the one workspace named when workspaceID is not empty. A
// permanent task that stops for good freezes its workspace's segments without telling anyone,
// and nothing else runs it again: ResetTask and TriggerTask take recurring tasks only.
//
// In one READ COMMITTED transaction, under permanentTaskRestoreLockID, it sets the stuck ones
// pending and due now, deletes those whose workspace is gone (on a full restore only), and
// creates the missing ones.
func (r *TaskRepository) RestorePermanentTasks(ctx context.Context, workspaceID string) (domain.PermanentTaskRestore, error) {
	var result domain.PermanentTaskRestore

	// READ COMMITTED whatever the server's default: a restore that waited on the lock must
	// see the tasks the one before it created.
	tx, err := r.systemDB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return result, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, permanentTaskRestoreLockID); err != nil {
		return result, fmt.Errorf("failed to take the permanent task lock: %w", err)
	}

	now := time.Now().UTC()
	types := pq.Array(domain.PermanentTaskTypes)

	revived, err := execRowsAffected(ctx, tx, restoreStuckPermanentTasksSQL, types, now, workspaceID)
	if err != nil {
		return result, fmt.Errorf("failed to restore stuck permanent tasks: %w", err)
	}
	result.Revived = revived

	if workspaceID == "" {
		removed, err := execRowsAffected(ctx, tx, removeOrphanPermanentTasksSQL, types)
		if err != nil {
			return result, fmt.Errorf("failed to remove orphan permanent tasks: %w", err)
		}
		result.Removed = removed
	}

	for _, taskType := range domain.PermanentTaskTypes {
		state, err := json.Marshal(&domain.TaskState{Message: domain.PermanentTaskMessage(taskType)})
		if err != nil {
			return result, fmt.Errorf("failed to marshal the state of %s: %w", taskType, err)
		}
		created, err := execRowsAffected(ctx, tx, createMissingPermanentTasksSQL,
			taskType, string(state), now, workspaceID,
			domain.PermanentTaskMaxRuntime, domain.PermanentTaskMaxRetries, domain.PermanentTaskRetryInterval)
		if err != nil {
			return result, fmt.Errorf("failed to create missing %s tasks: %w", taskType, err)
		}
		result.Created += created
	}

	if err := tx.Commit(); err != nil {
		return domain.PermanentTaskRestore{}, fmt.Errorf("failed to commit permanent task restore: %w", err)
	}
	return result, nil
}

// execRowsAffected runs a statement in tx and returns the number of rows it changed.
func execRowsAffected(ctx context.Context, tx *sql.Tx, query string, args ...interface{}) (int64, error) {
	res, err := tx.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
