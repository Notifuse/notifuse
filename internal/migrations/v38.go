package migrations

import (
	"context"
	"fmt"
	"time"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/database/schema"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/service"
)

// V38Migration adds the web analytics tables to every workspace database:
// web_sessions, web_pages and web_goals, all declaratively partitioned by
// session_date with monthly partitions. The DDL is shared with the
// new-workspace initializer (internal/database/schema) so both paths create
// identical schemas. The current and next monthly partitions are created here;
// the web analytics maintenance worker keeps creating them going forward.
//
// It also creates the unpartitioned annotations table, from the same shared
// definitions, so charts on an upgraded workspace have somewhere to read their
// markers from.
//
// It also grants the new web_analytics permission to existing members and
// pending invitations in the system database. Permissions are stored as a
// frozen map per membership, and HasPermission denies any resource missing
// from it, so without this backfill every non-owner member would be locked out
// of the feature (owners bypass the map entirely).
//
// Finally it regenerates the installed trigger of live automations whose trigger
// carries conditions, which until now were compiled into the CREATE TRIGGER WHEN
// clause and therefore never enforced. See healAutomationTriggerConditions.
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

// NOTE for anyone adding a column to the web analytics tables.
//
// `web_goals.goal_type` was added by amending schema.WebAnalyticsTableDefinitions()
// with no migration file and no VERSION bump. That was correct here, and the
// reasoning is narrow enough to be worth stating:
//
//   - This function and internal/database/init.go both iterate the SAME shared
//     definitions, so amending them covers fresh installs and every database
//     below 38.
//   - Folding a step into this file would no-op on exactly the databases that
//     need it — see the dispatcher note in .claude/skills/create-migration.
//   - 38.0 was unreleased, so no customer database had run the pre-amendment DDL.
//     Only local/preview/demo were at 38.0, and they were rebuilt by hand.
//
// The trick dies the moment a migration inlines its own DDL instead of iterating
// the shared definitions — and no test catches that. If 38.0 has shipped by the
// time you read this, the answer is a new major, not another amendment.
func (m *V38Migration) UpdateWorkspace(ctx context.Context, cfg *config.Config, workspace *domain.Workspace, db DBExecutor) error {
	for _, query := range schema.AnnotationsTableDefinitions() {
		if _, err := db.ExecContext(ctx, query); err != nil {
			return fmt.Errorf("v38: failed to create annotations table for workspace %s: %w", workspace.ID, err)
		}
	}

	for _, query := range schema.WebAnalyticsTableDefinitions() {
		if _, err := db.ExecContext(ctx, query); err != nil {
			return fmt.Errorf("v38: failed to create web analytics table for workspace %s: %w", workspace.ID, err)
		}
	}

	for _, query := range schema.UsageTableDefinitions() {
		if _, err := db.ExecContext(ctx, query); err != nil {
			return fmt.Errorf("v38: failed to create usage table for workspace %s: %w", workspace.ID, err)
		}
	}

	// Keep the webhook trigger in step with internal/database/init.go: bridged
	// web goals must not fan out to third-party subscribers. Both paths have to
	// carry the same function body or a fresh install and an upgraded one behave
	// differently for the same data.
	if _, err := db.ExecContext(ctx, schema.WebhookCustomEventsTriggerFunction()); err != nil {
		return fmt.Errorf("v38: failed to update webhook custom events trigger for workspace %s: %w", workspace.ID, err)
	}

	// Same reasoning for the enrolment function: it now refuses to enrol for an automation
	// that is not live, which is what makes a trigger left installed against a paused, draft
	// or soft-deleted row harmless. Existing workspaces need it precisely because their
	// triggers are the ones that may already have outlived their automation.
	if _, err := db.ExecContext(ctx, schema.AutomationEnrollContactFunction()); err != nil {
		return fmt.Errorf("v38: failed to update automation enroll function for workspace %s: %w", workspace.ID, err)
	}

	now := time.Now().UTC()
	for _, month := range []time.Time{now, now.AddDate(0, 1, 0)} {
		for _, table := range schema.WebAnalyticsTableNames {
			if _, err := db.ExecContext(ctx, schema.WebAnalyticsPartitionDDL(table, month)); err != nil {
				return fmt.Errorf("v38: failed to create %s partition for workspace %s: %w", table, workspace.ID, err)
			}
		}
	}

	if err := m.healAutomationTriggerConditions(ctx, db); err != nil {
		return fmt.Errorf("v38: failed to heal automation trigger conditions for workspace %s: %w", workspace.ID, err)
	}

	return nil
}

// healAutomationTriggerConditions rebuilds the installed trigger of every live automation
// from the configuration actually stored against it.
//
// Two ways a workspace arrives here with a trigger that does not implement its automation.
// Trigger conditions used to compile into the CREATE TRIGGER ... WHEN clause as subqueries,
// which PostgreSQL rejects outright, so an automation carrying them was activated without
// them and edited afterwards — leaving a trigger that enrols every matching contact and
// enforces nothing. And updates never regenerated the trigger at all, so editing an event
// kind, list, segment or updated_fields on a live automation left it firing on the old one.
// Both are invisible from here on, because a change is now detected by comparing the
// incoming row against the stored row, and a stale trigger makes neither of them disagree.
//
// Repair-only, deliberately: an automation with no installed trigger is skipped rather than
// armed. Rows created directly with status 'live' — which the API used to accept without ever
// running DDL — have never fired in their life, and starting them here, mid-migration and
// carrying every unrelated edit made since, would not be a repair.
func (m *V38Migration) healAutomationTriggerConditions(ctx context.Context, db DBExecutor) error {
	// The bodies as v38 shipped them: the four-argument enrolment, which is the one v38
	// installs. v44 replaces their bodies again to pass the event.
	generator := service.NewAutomationTriggerGenerator(service.NewQueryBuilder()).WithoutTriggerEvent()
	_, err := regenerateAutomationTriggers(ctx, db, generator, "v38", "v38_automation_conditions", false)
	return err
}

func init() {
	Register(&V38Migration{})
}
