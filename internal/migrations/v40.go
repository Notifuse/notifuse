package migrations

import (
	"context"
	"fmt"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
)

// V40Migration prepares every workspace database for the outbound-webhook
// lifecycle work: subscription attribution (source), automatic disabling of dead
// endpoints (consecutive_failures, disabled_reason), and the delivery claim that
// stops a multi-replica deployment from delivering everything twice
// (claimed_at). It also adds the foreign key that makes deleting a subscription
// take its queued deliveries with it.
//
// It is deliberately the instant half of a two-migration split, and the split is
// not cosmetic. Migrations run inside one transaction per database, so every
// lock a migration takes is held until commit. Every statement here is metadata
// only: PostgreSQL has treated ADD COLUMN with a non-volatile DEFAULT as a
// catalogue update since 11, and the foreign key is added NOT VALID, which
// returns immediately and skips the validating scan while still enforcing the
// constraint on every new row — so no fresh orphan can appear in the window
// before the expensive half runs.
//
// Validating the constraint here instead would request a ShareRowExclusiveLock
// on webhook_deliveries for the length of a full table scan, and that is not a
// webhook outage, it is a write outage. The enqueue path is a synchronous INSERT
// from AFTER-row triggers, so blocking INSERT on webhook_deliveries blocks every
// customer write to contacts, contact_lists, contact_segments and
// message_history — inside a startup migration whose timeout crash-loops the
// pod. The orphan sweep and VALIDATE CONSTRAINT belong in the following
// migration, where a timeout rolls back to this already-protective state instead
// of to no protection at all.
type V40Migration struct{}

func (m *V40Migration) GetMajorVersion() float64 { return 40.0 }

func (m *V40Migration) HasSystemUpdate() bool { return false }

func (m *V40Migration) HasWorkspaceUpdate() bool { return true }

func (m *V40Migration) ShouldRestartServer() bool { return false }

func (m *V40Migration) UpdateSystem(ctx context.Context, cfg *config.Config, db DBExecutor) error {
	return nil
}

func (m *V40Migration) UpdateWorkspace(ctx context.Context, cfg *config.Config, workspace *domain.Workspace, db DBExecutor) error {
	// consecutive_failures is NOT NULL DEFAULT 0 rather than nullable: the
	// counter is read on every failed delivery and compared against a threshold,
	// and a NULL that has to be coalesced at every read site is one forgotten
	// COALESCE away from a subscription that can never be auto-disabled.
	// Existing rows adopt the default without a rewrite.
	_, err := db.ExecContext(ctx, `
		ALTER TABLE webhook_subscriptions
		ADD COLUMN IF NOT EXISTS source VARCHAR(32),
		ADD COLUMN IF NOT EXISTS consecutive_failures INT NOT NULL DEFAULT 0,
		ADD COLUMN IF NOT EXISTS disabled_reason TEXT
	`)
	if err != nil {
		return fmt.Errorf("v40: failed to add lifecycle columns to webhook_subscriptions: %w", err)
	}

	_, err = db.ExecContext(ctx, `
		ALTER TABLE webhook_deliveries
		ADD COLUMN IF NOT EXISTS claimed_at TIMESTAMPTZ
	`)
	if err != nil {
		return fmt.Errorf("v40: failed to add claimed_at to webhook_deliveries: %w", err)
	}

	// ADD CONSTRAINT has no IF NOT EXISTS, so the pg_constraint lookup is what
	// makes this re-runnable. The name is the one a fresh install declares
	// explicitly in the workspace DDL, so a workspace created after this ships
	// and a workspace upgraded into it converge on the same constraint — the
	// difference being that the fresh one is already validated and this one is
	// not, which the following migration reconciles.
	_, err = db.ExecContext(ctx, `
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
				ON DELETE CASCADE NOT VALID;
			END IF;
		END $$
	`)
	if err != nil {
		return fmt.Errorf("v40: failed to add webhook_deliveries subscription foreign key: %w", err)
	}

	// 'delivering' is a status no build before this one ever wrote — the constant
	// existed and was never used — so any row carrying it was claimed by a worker
	// that has since died, or by a replica already running the new code while
	// this workspace was still being migrated. Either way nothing is going to
	// release it. Left alone the row would sit outside the pending predicate
	// forever, which is precisely the stranded-row class the claim exists to
	// eliminate. claimed_at is cleared with it so the row is indistinguishable
	// from one that was never claimed.
	_, err = db.ExecContext(ctx, `
		UPDATE webhook_deliveries
		SET status = 'pending', claimed_at = NULL
		WHERE status = 'delivering'
	`)
	if err != nil {
		return fmt.Errorf("v40: failed to reset stranded delivering rows: %w", err)
	}

	// A claimed row's status becomes 'delivering', which drops it out of
	// idx_webhook_deliveries_pending, so the reclaim sweep needs its own entry
	// point. Partial on the same predicate the sweep uses, so it stays about the
	// size of the in-flight batch instead of the retention window.
	//
	// This is the one statement here that is not metadata-only: a partial index
	// is still built by scanning the whole heap. It runs last so the scan is the
	// only work left in the transaction, and it runs after the reset above so
	// there is nothing left to index. Note the ALTER TABLE statements have
	// already taken an AccessExclusiveLock held until commit, so this costs
	// duration rather than a stronger lock than the migration already holds.
	_, err = db.ExecContext(ctx, `
		CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_claimed
		ON webhook_deliveries(claimed_at) WHERE status = 'delivering'
	`)
	if err != nil {
		return fmt.Errorf("v40: failed to create webhook_deliveries claimed index: %w", err)
	}

	return nil
}

func init() {
	Register(&V40Migration{})
}
