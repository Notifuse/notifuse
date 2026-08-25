package migrations

import (
	"context"
	"fmt"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/database/schema"
	"github.com/Notifuse/notifuse/internal/domain"
)

// V41Migration is the expensive half of the outbound-webhook split that v40
// opened. v40 added the foreign key NOT VALID, which returns instantly and still
// rejects every new orphan, so from the moment it committed no fresh orphan can
// appear. This migration finishes the job on the rows that were already there:
// it sweeps the existing orphans and validates the constraint, then reinstalls
// the five webhook trigger functions from the shared generator.
//
// Splitting it out is what keeps the lock window survivable. Migrations run
// inside one transaction per workspace database, so every lock is held until
// commit, and a validating ADD CONSTRAINT takes a ShareRowExclusiveLock across a
// full scan of webhook_deliveries. Deliveries are enqueued by AFTER-row triggers
// inside the customer's own write transactions, so blocking INSERT on that table
// blocks every write to contacts, contact_lists, contact_segments and
// message_history — a write outage, not a webhook outage. VALIDATE CONSTRAINT on
// its own takes only a ShareUpdateExclusiveLock, which permits concurrent
// INSERT, UPDATE and DELETE. That difference is the entire reason for the split.
//
// The trigger reinstall is the other half of the story. Four of the five bodies
// lived in two unshared copies — internal/migrations/v19.go and the workspace
// initializer — so a workspace upgraded from v19 and a workspace created last
// week could in principle be running different bodies and emitting different
// payloads for identical data. Installing all five from
// internal/database/schema converges them and adds the list_ids / segment_ids
// filtering to the two that support it.
type V41Migration struct{}

func (m *V41Migration) GetMajorVersion() float64 { return 41.0 }

func (m *V41Migration) HasSystemUpdate() bool { return false }

func (m *V41Migration) HasWorkspaceUpdate() bool { return true }

func (m *V41Migration) ShouldRestartServer() bool { return false }

func (m *V41Migration) UpdateSystem(ctx context.Context, cfg *config.Config, db DBExecutor) error {
	return nil
}

func (m *V41Migration) UpdateWorkspace(ctx context.Context, cfg *config.Config, workspace *domain.Workspace, db DBExecutor) error {
	// SET LOCAL, so it reverts when the migration transaction ends rather than
	// riding on a pooled connection into ordinary request traffic.
	//
	// lock_timeout bounds how long a statement WAITS for a lock, not how long it
	// holds one. Nothing below expects to wait: the sweep takes a
	// RowExclusiveLock and VALIDATE CONSTRAINT a ShareUpdateExclusiveLock, and
	// neither conflicts with the DML this database is doing. It matters for the
	// case where something else already holds a conflicting lock — a second
	// replica running this same migration, a manual ALTER, an autovacuum's
	// truncate phase — where the alternative to yielding is a migration that
	// queues every customer write behind it while it waits. Failing here rolls
	// back to v40's already-protective state and retries on the next startup,
	// which is strictly better than an outage.
	if _, err := db.ExecContext(ctx, `SET LOCAL lock_timeout = '5s'`); err != nil {
		return fmt.Errorf("v41: failed to set lock timeout: %w", err)
	}

	// Reinstalled before the sweep and the validation, not after, because a
	// second replica running this migration concurrently will collide on the
	// first CREATE OR REPLACE and abort on the lock timeout. Doing the cheap
	// statements first means the loser of that race throws away nothing.
	// Replacing a function body does not block callers: a trigger already
	// attached to the function keeps executing the old body until this
	// transaction commits, so no customer write waits on this.
	//
	// Only the functions are reinstalled. Reattaching the triggers would mean
	// DROP TRIGGER + CREATE TRIGGER, which takes a ShareRowExclusiveLock on
	// contacts, contact_lists, contact_segments, message_history and
	// custom_events and holds it until commit — the very outage this split
	// exists to avoid — and it would buy nothing, because an attached trigger
	// picks up a replaced body on its next invocation.
	for _, fn := range schema.WebhookTriggerFunctions() {
		if _, err := db.ExecContext(ctx, fn); err != nil {
			return fmt.Errorf("v41: failed to reinstall webhook trigger functions: %w", err)
		}
	}

	// Orphans exist because webhook_deliveries had no foreign key until v40 and
	// deleting a subscription left its queued rows behind. Each one is a poison
	// pill: the worker fails to load the subscription, skips the row without
	// writing to it, and the row keeps matching the pending predicate for the
	// whole retention window, permanently consuming a slot in every batch.
	//
	// subscription_id is NOT NULL, so this predicate is exactly "the referenced
	// subscription is gone" and nothing else. Deleting in one statement rather
	// than in batches because the whole migration is a single transaction: locks
	// are held until commit either way, so batching would add round trips and
	// remove nothing.
	if _, err := db.ExecContext(ctx, `
		DELETE FROM webhook_deliveries d
		WHERE NOT EXISTS (
			SELECT 1 FROM webhook_subscriptions s WHERE s.id = d.subscription_id
		)
	`); err != nil {
		return fmt.Errorf("v41: failed to sweep orphaned webhook deliveries: %w", err)
	}

	// convalidated is the guard, not mere existence. A workspace created after
	// v40 shipped declares this constraint inline and already validated, and
	// re-validating it would take a ShareUpdateExclusiveLock and re-scan the
	// table for a result the catalogue already knows. A workspace upgraded
	// through v40 has it NOT VALID and needs the scan exactly once; the same
	// guard then makes a re-run after a rolled-back transaction a no-op.
	//
	// The lookup also covers the case where the constraint is missing entirely:
	// no row matches, so this does nothing rather than raising on a name that is
	// not there.
	if _, err := db.ExecContext(ctx, `
		DO $$
		BEGIN
			IF EXISTS (
				SELECT 1 FROM pg_constraint
				WHERE conname = 'webhook_deliveries_subscription_id_fkey'
				AND conrelid = to_regclass('webhook_deliveries')
				AND NOT convalidated
			) THEN
				ALTER TABLE webhook_deliveries
				VALIDATE CONSTRAINT webhook_deliveries_subscription_id_fkey;
			END IF;
		END $$
	`); err != nil {
		return fmt.Errorf("v41: failed to validate webhook_deliveries subscription foreign key: %w", err)
	}

	return nil
}

func init() {
	Register(&V41Migration{})
}
