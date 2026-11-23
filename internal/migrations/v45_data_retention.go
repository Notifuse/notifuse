package migrations

import (
	"context"
	"fmt"

	"github.com/Notifuse/notifuse/internal/database/schema"
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
	return append(statements, schema.ContactOptOutStatements()...)
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
