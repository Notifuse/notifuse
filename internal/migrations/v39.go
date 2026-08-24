package migrations

import (
	"context"
	"fmt"

	"github.com/Notifuse/notifuse/config"
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
type V39Migration struct{}

func (m *V39Migration) GetMajorVersion() float64 { return 39.0 }

func (m *V39Migration) HasSystemUpdate() bool { return true }

func (m *V39Migration) HasWorkspaceUpdate() bool { return false }

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
	return nil
}

func init() {
	Register(&V39Migration{})
}
