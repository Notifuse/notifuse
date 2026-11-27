package migrations

import (
	"context"
	"fmt"

	"github.com/Notifuse/notifuse/internal/database/schema"
	"github.com/Notifuse/notifuse/internal/domain"
)

// dataRetentionWorkspaceV45Statements is the data retention part of the v45 workspace
// migration (Lot 0 of the data retention plan): the opt-out hashes, their key, hash
// function and restore trigger, and the new bodies of the three functions that honour them.
// Every body comes from the schema package, which internal/database/init.go also uses, so
// a workspace created at 45 and one migrated to it cannot differ.
func dataRetentionWorkspaceV45Statements() []string {
	statements := []string{
		// Writes nothing for an erased contact's messages, so the redaction of
		// status_info is not copied into the timeline.
		schema.TrackMessageHistoryChangesFunction(),
		// Record a restored opt-out under its own kind, and send no webhook for it.
		schema.TrackContactListChangesFunction(),
		schema.WebhookContactListsTriggerFunction(),
	}
	statements = append(statements, schema.ContactOptOutStatements()...)
	// Lot 1, engagement clock: the table, and a readiness marker left unset until the
	// workspace's backfill task has read its history.
	statements = append(statements, schema.ContactEngagementStatements()...)
	statements = append(statements, schema.MigratedWorkspaceDataRetentionMarker)
	// Lot 1, engine: run reports, the erasure work table, the message history cursor, and
	// the three functions that honour notifuse.retention_purge (no segment.left for an
	// erased contact, contact.deleted with the address only).
	statements = append(statements, schema.DataRetentionStatements()...)
	// Lot 1, D13: the mirror of the contact policy and the function the expiring-soon segment
	// condition calls.
	statements = append(statements, schema.DataRetentionMirrorStatements()...)
	statements = append(statements, schema.ContactRetentionExpiresAtFunction())
	return append(statements,
		schema.TrackContactSegmentChangesFunction(),
		schema.WebhookContactSegmentsTriggerFunction(),
		schema.WebhookContactsTriggerFunction(),
	)
}

// dataRetentionSystemV45Statements is the data retention part of the v45 system migration:
// one engagement backfill task per existing workspace, unless one is already there (tasks
// has no unique key on workspace and type, and a crash between the migration's commit and
// the version stamp runs v45 again). A workspace created at 45 needs none: it starts ready.
//
// The task columns are timestamps without a time zone that hold UTC, and the scheduler
// compares next_run_after with the UTC time: CURRENT_TIMESTAMP alone would store the server's
// local time, and on a server ahead of UTC the tasks would wait for the difference.
func dataRetentionSystemV45Statements() []string {
	return []string{
		`INSERT INTO tasks (id, workspace_id, type, status, next_run_after, max_runtime, max_retries, retry_interval, progress, state, created_at, updated_at)
		SELECT gen_random_uuid(), w.id, '` + domain.DataRetentionBackfillTaskType + `', 'pending', CURRENT_TIMESTAMP AT TIME ZONE 'UTC', 300, 10, 300, 0,
			'{"message": "Read the engagement of existing contacts from their history"}'::jsonb, CURRENT_TIMESTAMP AT TIME ZONE 'UTC', CURRENT_TIMESTAMP AT TIME ZONE 'UTC'
		FROM workspaces w
		WHERE NOT EXISTS (
			SELECT 1 FROM tasks t WHERE t.workspace_id = w.id AND t.type = '` + domain.DataRetentionBackfillTaskType + `'
		)`,
	}
}

// applyDataRetentionSystemV45 runs the data retention part of the v45 system migration in
// the migration's transaction. It is idempotent.
func applyDataRetentionSystemV45(ctx context.Context, db DBExecutor) error {
	for _, statement := range dataRetentionSystemV45Statements() {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("data retention (v45 system): %w", err)
		}
	}
	return nil
}

// applyDataRetentionWorkspaceV45 runs the data retention part of the v45 workspace
// migration in the migration's transaction. It is idempotent.
func applyDataRetentionWorkspaceV45(ctx context.Context, db DBExecutor) error {
	for _, statement := range dataRetentionWorkspaceV45Statements() {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("data retention (v45): %w", err)
		}
	}
	return nil
}
