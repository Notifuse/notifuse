package migrations

import (
	"context"
	"fmt"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/database/schema"
	"github.com/Notifuse/notifuse/internal/domain"
)

// V39Migration grants the three new permission resources — segments,
// webhook_subscriptions and webhook_events — to existing members and pending
// invitations in the system database. Permissions are stored as a frozen map
// per membership and HasPermission denies any resource missing from it, so
// without this backfill every non-owner member would lose access to endpoints
// they can call today (owners bypass the map entirely).
//
// It then normalises the memberships whose permissions column is SQL NULL to an
// empty object. Access is unchanged — NULL and '{}' both deny everything for a
// non-owner — but the row becomes editable in the console and legible to future
// backfills, instead of being skipped forever by every jsonb_typeof guard.
// It also brings every workspace database up to the outbound-webhook
// lifecycle work in one pass: subscription attribution (source), automatic
// disabling of dead endpoints (consecutive_failures, disabled_reason), the
// delivery claim that stops a multi-replica deployment delivering everything
// twice (claimed_at), the foreign key that makes deleting a subscription take
// its queued deliveries with it, and the reinstalled trigger functions that add
// per-list and per-segment filtering.
//
// The whole thing runs in one transaction, because that is how the migration
// manager runs every migration — one transaction per database, every lock held
// until commit. So this migration's lock window is its entire duration, and the
// two statements that are not metadata-only set that duration: validating the
// foreign key scans webhook_deliveries, and the partial index builds from a heap
// scan. That matters because the enqueue path is a synchronous INSERT from
// AFTER-row triggers, so an AccessExclusiveLock on webhook_deliveries blocks not
// just webhooks but every customer write to contacts, contact_lists,
// contact_segments and message_history for as long as it is held. The retention
// window caps how large that table can get — CleanupOldDeliveries keeps seven
// days — which is what makes a single-transaction migration defensible here.
//
// Statement order is therefore deliberate and is asserted by the tests:
//
//	lock_timeout    bound the WAIT for a lock, so a migration blocked behind
//	                something else fails and retries rather than queueing every
//	                customer write behind its own pending lock request.
//	trigger bodies  cheap, and first so that a second replica racing this same
//	                migration collides here and aborts before doing real work.
//	ADD COLUMN      catalogue-only; PostgreSQL has treated a non-volatile
//	                DEFAULT as a metadata update since 11.
//	orphan sweep    must precede the constraint: validating over an existing
//	                orphan raises, and orphans are guaranteed to exist because
//	                webhook_deliveries had no foreign key until now.
//	ADD CONSTRAINT  validating, not NOT VALID. Deferring validation buys nothing
//	                inside a single transaction: the ADD COLUMN above already
//	                holds an AccessExclusiveLock until commit, so the weaker
//	                ShareUpdateExclusiveLock a later VALIDATE would take is
//	                moot, and splitting it would mean two passes for one result.
//	reset stranded  rows left in 'delivering' by a crashed worker.
//	CREATE INDEX    last, so the heap scan is the only work left and there is
//	                nothing further to index.
type V39Migration struct{}

func (m *V39Migration) GetMajorVersion() float64 { return 39.0 }

func (m *V39Migration) HasSystemUpdate() bool { return true }

func (m *V39Migration) HasWorkspaceUpdate() bool { return true }

func (m *V39Migration) ShouldRestartServer() bool { return false }

func (m *V39Migration) UpdateSystem(ctx context.Context, cfg *config.Config, db DBExecutor) error {
	// The defaults sit on the LEFT of ||. jsonb || is a shallow merge in which
	// the right operand wins on duplicate keys, so `defaults || permissions`
	// means any stored grant survives by construction and nothing can ever be
	// widened back. Putting the grant on the right is the trap: guarded on one
	// resource, a row already holding that key is skipped and never receives the
	// others; guarded on all three, a narrowed row passes and its grants are
	// overwritten back to read+write.
	//
	// jsonb_typeof(...) = 'object' is the guard, not IS NOT NULL: concatenating
	// an object onto a JSON scalar does not fail, it silently produces an array
	// ('null'::jsonb || '{"a":1}'::jsonb yields [null, {"a": 1}]), which no
	// longer scans into UserPermissions and would lock the member out of the
	// workspace entirely.
	const grant = `'{"segments":              {"read": true, "write": true},
	                 "webhook_subscriptions": {"read": true, "write": true},
	                 "webhook_events":        {"read": true, "write": true}}'::jsonb || permissions`

	// An empty object is a member who can do nothing, and it stays that way: the
	// normalisation below turns every SQL-NULL row into exactly this, and the
	// version stamp is written only after the migration transaction commits (a
	// separate statement in Manager.RunMigrations). A crash in between re-runs
	// UpdateSystem against a table where those rows are now '{}', so without this
	// exclusion the second run would hand every zero-permission member read+write
	// on all three new resources — the escalation the statement order below
	// prevents on the first run, arriving by another door on the second.
	const needsGrant = `permissions <> '{}'::jsonb
	                AND NOT (permissions ? 'segments'
	                     AND permissions ? 'webhook_subscriptions'
	                     AND permissions ? 'webhook_events')`

	_, err := db.ExecContext(ctx, `
		UPDATE user_workspaces
		SET permissions = `+grant+`
		WHERE jsonb_typeof(permissions) = 'object'
		AND `+needsGrant+`
	`)
	if err != nil {
		return fmt.Errorf("v39: failed to add scoping permissions to user workspaces: %w", err)
	}

	_, err = db.ExecContext(ctx, `
		UPDATE workspace_invitations
		SET permissions = `+grant+`
		WHERE jsonb_typeof(permissions) = 'object'
		AND `+needsGrant+`
	`)
	if err != nil {
		return fmt.Errorf("v39: failed to add scoping permissions to workspace invitations: %w", err)
	}

	// LAST statements in UpdateSystem. Run before the grants, they would turn
	// every SQL-NULL row into '{}' and hand it to jsonb_typeof as an object. The
	// '{}' exclusion in needsGrant already refuses those rows, so this is the
	// second of two defences against the same escalation rather than the only
	// one — which is the arrangement an irreversible migration deserves.
	_, err = db.ExecContext(ctx, `
		UPDATE user_workspaces
		SET permissions = '{}'::jsonb
		WHERE permissions IS NULL
	`)
	if err != nil {
		return fmt.Errorf("v39: failed to normalise null permissions on user workspaces: %w", err)
	}

	_, err = db.ExecContext(ctx, `
		UPDATE workspace_invitations
		SET permissions = '{}'::jsonb
		WHERE permissions IS NULL
	`)
	if err != nil {
		return fmt.Errorf("v39: failed to normalise null permissions on workspace invitations: %w", err)
	}

	return nil
}

func (m *V39Migration) UpdateWorkspace(ctx context.Context, cfg *config.Config, workspace *domain.Workspace, db DBExecutor) error {
	// SET LOCAL, so it reverts when the migration transaction ends rather than
	// riding on a pooled connection into ordinary request traffic.
	//
	// lock_timeout bounds how long a statement WAITS for a lock, not how long it
	// holds one. Nothing below expects to wait. It matters for the case where
	// something else already holds a conflicting lock — a second replica running
	// this same migration, a manual ALTER, an autovacuum truncate — where the
	// alternative to yielding is a migration that queues every customer write
	// behind its own pending lock request. Failing here rolls the workspace back
	// to its pre-migration state and retries on the next startup, which is
	// strictly better than an outage.
	if _, err := db.ExecContext(ctx, `SET LOCAL lock_timeout = '5s'`); err != nil {
		return fmt.Errorf("v39: failed to set lock timeout: %w", err)
	}

	// Four of the five bodies lived in two unshared copies — v19 and the
	// workspace initializer — so a workspace upgraded from v19 and one created
	// last week could in principle emit different payloads for identical data.
	// Installing all five from the shared generator converges them and adds the
	// list_ids / segment_ids filtering to the two that support it.
	//
	// Only the functions are reinstalled. Reattaching the triggers would mean
	// DROP TRIGGER + CREATE TRIGGER, which takes a ShareRowExclusiveLock on
	// contacts, contact_lists, contact_segments, message_history and
	// custom_events and holds it until commit, and it would buy nothing: a
	// trigger already attached picks up a replaced body on its next invocation.
	for _, fn := range schema.WebhookTriggerFunctions() {
		if _, err := db.ExecContext(ctx, fn); err != nil {
			return fmt.Errorf("v39: failed to reinstall webhook trigger functions: %w", err)
		}
	}

	// consecutive_failures is NOT NULL DEFAULT 0 rather than nullable: the
	// counter is read on every failed delivery and compared against a threshold,
	// and a NULL that has to be coalesced at every read site is one forgotten
	// COALESCE away from a subscription that can never be auto-disabled.
	// Existing rows adopt the default without a rewrite.
	//
	// failing_since is nullable and stays NULL for every existing row, which is
	// the correct starting state: NULL means "not currently failing", so an
	// upgraded workspace begins its first run of failures from the first failure
	// after the upgrade rather than inheriting a window that already looks hours
	// old.
	if _, err := db.ExecContext(ctx, `
		ALTER TABLE webhook_subscriptions
		ADD COLUMN IF NOT EXISTS source VARCHAR(32),
		ADD COLUMN IF NOT EXISTS consecutive_failures INT NOT NULL DEFAULT 0,
		ADD COLUMN IF NOT EXISTS failing_since TIMESTAMPTZ,
		ADD COLUMN IF NOT EXISTS disabled_reason TEXT
	`); err != nil {
		return fmt.Errorf("v39: failed to add lifecycle columns to webhook_subscriptions: %w", err)
	}

	if _, err := db.ExecContext(ctx, `
		ALTER TABLE webhook_deliveries
		ADD COLUMN IF NOT EXISTS claimed_at TIMESTAMPTZ
	`); err != nil {
		return fmt.Errorf("v39: failed to add claimed_at to webhook_deliveries: %w", err)
	}

	// Orphans exist because webhook_deliveries had no foreign key until now and
	// deleting a subscription left its queued rows behind. Each one is a poison
	// pill: the worker fails to load the subscription, and before this release it
	// skipped the row without writing to it, so the row kept matching the pending
	// predicate for the whole retention window, permanently consuming a slot in
	// every batch.
	//
	// subscription_id is NOT NULL, so this predicate is exactly "the referenced
	// subscription is gone" and nothing else. One statement rather than batches:
	// the whole migration is a single transaction, so locks are held until commit
	// either way and batching would add round trips while removing nothing.
	if _, err := db.ExecContext(ctx, `
		DELETE FROM webhook_deliveries d
		WHERE NOT EXISTS (
			SELECT 1 FROM webhook_subscriptions s WHERE s.id = d.subscription_id
		)
	`); err != nil {
		return fmt.Errorf("v39: failed to sweep orphaned webhook deliveries: %w", err)
	}

	// ADD CONSTRAINT has no IF NOT EXISTS, so the pg_constraint lookup is what
	// makes this re-runnable. Three cases, and all three have to be handled:
	// absent, which is a workspace upgrading into this release; present and
	// already valid, which is a workspace created after this ships and declaring
	// the constraint inline in its DDL; and present but NOT VALID, which is not
	// reachable from a released build but is cheap to reconcile and would
	// otherwise leave the constraint unvalidated forever.
	//
	// The name is the one the workspace DDL declares explicitly, so upgraded and
	// fresh workspaces converge on the same constraint.
	if _, err := db.ExecContext(ctx, `
		DO $$
		BEGIN
			IF NOT EXISTS (
				SELECT 1 FROM pg_constraint
				WHERE conname = 'webhook_deliveries_subscription_id_fkey'
				AND conrelid = to_regclass('webhook_deliveries')
			) THEN
				ALTER TABLE webhook_deliveries
				ADD CONSTRAINT webhook_deliveries_subscription_id_fkey
				FOREIGN KEY (subscription_id) REFERENCES webhook_subscriptions(id)
				ON DELETE CASCADE;
			ELSIF EXISTS (
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
		return fmt.Errorf("v39: failed to add webhook_deliveries subscription foreign key: %w", err)
	}

	// 'delivering' is a status no build before this one ever wrote — the constant
	// existed and was never used — so any row carrying it was claimed by a worker
	// that has since died, or by a replica already running the new code while
	// this workspace was still being migrated. Either way nothing is going to
	// release it. Left alone the row would sit outside the pending predicate
	// forever, which is precisely the stranded-row class the claim exists to
	// eliminate. claimed_at is cleared with it so the row is indistinguishable
	// from one that was never claimed.
	if _, err := db.ExecContext(ctx, `
		UPDATE webhook_deliveries
		SET status = 'pending', claimed_at = NULL
		WHERE status = 'delivering'
	`); err != nil {
		return fmt.Errorf("v39: failed to reset stranded delivering rows: %w", err)
	}

	// A claimed row's status becomes 'delivering', which drops it out of
	// idx_webhook_deliveries_pending, so the reclaim sweep needs its own entry
	// point. Partial on the same predicate the sweep uses, so it stays about the
	// size of the in-flight batch instead of the retention window.
	//
	// Last, because a partial index is still built by scanning the whole heap:
	// running it here means the scan is the only work left in the transaction,
	// and it runs after the reset above so there is nothing further to index.
	if _, err := db.ExecContext(ctx, `
		CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_claimed
		ON webhook_deliveries(claimed_at) WHERE status = 'delivering'
	`); err != nil {
		return fmt.Errorf("v39: failed to create webhook_deliveries claimed index: %w", err)
	}

	return nil
}

func init() {
	Register(&V39Migration{})
}
