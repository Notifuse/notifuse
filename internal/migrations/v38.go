package migrations

import (
	"context"
	"fmt"
	"time"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/database/schema"
	"github.com/Notifuse/notifuse/internal/domain"
)

// V38Migration adds the web analytics tables to every workspace database:
// web_sessions, web_pages and web_goals, all declaratively partitioned by
// session_date with monthly partitions. The DDL is shared with the
// new-workspace initializer (internal/database/schema) so both paths create
// identical schemas. The current and next monthly partitions are created here;
// the web analytics maintenance worker keeps creating them going forward.
//
// It also grants the new web_analytics permission to existing members and
// pending invitations in the system database. Permissions are stored as a
// frozen map per membership, and HasPermission denies any resource missing
// from it, so without this backfill every non-owner member would be locked out
// of the feature (owners bypass the map entirely).
type V38Migration struct{}

func (m *V38Migration) GetMajorVersion() float64 { return 38.0 }

func (m *V38Migration) HasSystemUpdate() bool { return true }

func (m *V38Migration) HasWorkspaceUpdate() bool { return true }

func (m *V38Migration) ShouldRestartServer() bool { return false }

func (m *V38Migration) UpdateSystem(ctx context.Context, cfg *config.Config, db DBExecutor) error {
	// Same grant every member-facing feature has shipped with (blog in v17,
	// automations in v20, llm in v22): existing members keep seeing everything
	// they could see before the upgrade.
	//
	// jsonb_typeof(...) = 'object' is the guard, not IS NOT NULL: concatenating
	// an object onto a JSON scalar does not fail, it silently produces an array
	// ('null'::jsonb || '{"a":1}'::jsonb yields [null, {"a": 1}]), which no
	// longer scans into UserPermissions and would lock the member out of the
	// workspace entirely. A SQL NULL means "no permissions at all" and is left
	// untouched, since that member has no access to any resource today.
	const grant = `permissions || '{"web_analytics": {"read": true, "write": true}}'::jsonb`

	_, err := db.ExecContext(ctx, `
		UPDATE user_workspaces
		SET permissions = `+grant+`
		WHERE jsonb_typeof(permissions) = 'object'
		AND NOT permissions ? 'web_analytics'
	`)
	if err != nil {
		return fmt.Errorf("v38: failed to add web analytics permissions to user workspaces: %w", err)
	}

	_, err = db.ExecContext(ctx, `
		UPDATE workspace_invitations
		SET permissions = `+grant+`
		WHERE jsonb_typeof(permissions) = 'object'
		AND NOT permissions ? 'web_analytics'
	`)
	if err != nil {
		return fmt.Errorf("v38: failed to add web analytics permissions to workspace invitations: %w", err)
	}

	return nil
}

func (m *V38Migration) UpdateWorkspace(ctx context.Context, cfg *config.Config, workspace *domain.Workspace, db DBExecutor) error {
	for _, query := range schema.WebAnalyticsTableDefinitions() {
		if _, err := db.ExecContext(ctx, query); err != nil {
			return fmt.Errorf("v38: failed to create web analytics table for workspace %s: %w", workspace.ID, err)
		}
	}

	// Keep the webhook trigger in step with internal/database/init.go: bridged
	// web goals must not fan out to third-party subscribers. Both paths have to
	// carry the same function body or a fresh install and an upgraded one behave
	// differently for the same data.
	if _, err := db.ExecContext(ctx, schema.WebhookCustomEventsTriggerFunction()); err != nil {
		return fmt.Errorf("v38: failed to update webhook custom events trigger for workspace %s: %w", workspace.ID, err)
	}

	now := time.Now().UTC()
	for _, month := range []time.Time{now, now.AddDate(0, 1, 0)} {
		for _, table := range schema.WebAnalyticsTableNames {
			if _, err := db.ExecContext(ctx, schema.WebAnalyticsPartitionDDL(table, month)); err != nil {
				return fmt.Errorf("v38: failed to create %s partition for workspace %s: %w", table, workspace.ID, err)
			}
		}
	}
	return nil
}

func init() {
	Register(&V38Migration{})
}
