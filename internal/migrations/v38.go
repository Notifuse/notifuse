package migrations

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
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
	// Every live automation with an installed trigger, not only the ones carrying
	// conditions. Updates never regenerated the trigger before this release, so any edit to
	// event_kind, list_id, segment_id or updated_fields left the automation firing on its
	// previous configuration — and nothing else will ever notice, because from here on a
	// change is detected by comparing the incoming row against the stored one, which
	// already agrees with itself. Pause and re-activate was the only escape.
	rows, err := db.QueryContext(ctx, `
		SELECT a.id, a.root_node_id, a.trigger_config,
		       (SELECT pg_get_triggerdef(t.oid) FROM pg_trigger t
		         WHERE t.tgrelid = to_regclass('contact_timeline')
		           AND NOT t.tgisinternal
		           AND t.tgname = lower('automation_trigger_' || replace(a.id, '-', '')))
		FROM automations a
		WHERE a.status = 'live'
		  AND a.deleted_at IS NULL
		  AND EXISTS (
		      SELECT 1 FROM pg_trigger t
		      WHERE t.tgrelid = to_regclass('contact_timeline')
		        AND NOT t.tgisinternal
		        AND t.tgname = lower('automation_trigger_' || replace(a.id, '-', ''))
		  )
	`)
	if err != nil {
		return fmt.Errorf("v38: failed to query automations with trigger conditions: %w", err)
	}

	type conditionalAutomation struct {
		id            string
		rootNodeID    string
		triggerConfig []byte
		installedDef  string
	}

	var automations []conditionalAutomation
	for rows.Next() {
		var a conditionalAutomation
		var rootNodeID, installedDef sql.NullString
		if scanErr := rows.Scan(&a.id, &rootNodeID, &a.triggerConfig, &installedDef); scanErr != nil {
			_ = rows.Close()
			return fmt.Errorf("v38: failed to scan automation: %w", scanErr)
		}
		a.rootNodeID = rootNodeID.String
		a.installedDef = installedDef.String
		automations = append(automations, a)
	}
	if iterErr := rows.Err(); iterErr != nil {
		_ = rows.Close()
		return fmt.Errorf("v38: error iterating automations: %w", iterErr)
	}
	if closeErr := rows.Close(); closeErr != nil {
		return fmt.Errorf("v38: failed to close automations rows: %w", closeErr)
	}

	if len(automations) == 0 {
		return nil
	}

	generator := service.NewAutomationTriggerGenerator(service.NewQueryBuilder())
	for _, a := range automations {
		var trigger domain.TimelineTriggerConfig
		if unmarshalErr := json.Unmarshal(a.triggerConfig, &trigger); unmarshalErr != nil {
			// Unparseable config — the installed trigger was already unusable; leave it.
			continue
		}

		automation := &domain.Automation{ID: a.id, RootNodeID: a.rootNodeID, Trigger: &trigger}
		triggerSQL, genErr := generator.Generate(automation)
		if genErr != nil {
			// Incomplete or corrupt automation config — skip rather than abort.
			continue
		}

		// Never trade a narrow trigger for a broader one. The stored config is only
		// authoritative if nothing has quietly removed part of it, and something did: the
		// console rebuilt trigger_config from a fixed list of fields that omitted
		// updated_fields, and an update overwrites the column wholesale. One console save
		// of an API-created contact.updated automation therefore dropped its field filter
		// from the row while the narrow trigger stayed installed, because updates ran no
		// DDL. Regenerating from that row would widen it to every contact update — with a
		// send node, a mass send nobody configured.
		//
		// An automation carrying conditions is unaffected: it could never have been
		// activated before this release, so its installed clause never mentions changes.
		if strings.Contains(a.installedDef, "changes ?") &&
			!strings.Contains(triggerSQL.WHENClause, "changes ?") {
			continue
		}

		// One savepoint per automation, so a failure rolls back just this automation and
		// leaves its existing trigger in place. Failing hard instead would abort the whole
		// workspace migration, never record the version, and re-fail on every restart —
		// and here the fallback is merely the status quo, an over-enrolling trigger.
		if _, err := db.ExecContext(ctx, "SAVEPOINT v38_automation_conditions"); err != nil {
			return fmt.Errorf("v38: failed to create savepoint: %w", err)
		}

		regenFailed := false

		// Same order as AutomationRepository.CreateAutomationTrigger — including running
		// the probe last. Probing first would take ACCESS SHARE on contact_timeline and
		// then upgrade to the ACCESS EXCLUSIVE the DROPs need; during a rolling restart the
		// old instance is still serving automations.activate, and that upgrade is exactly
		// what deadlocks. The savepoint undoes the DDL either way, so nothing invalid
		// survives a failed probe.
		for _, stmt := range []string{
			triggerSQL.DropTrigger,
			triggerSQL.DropFunction,
			triggerSQL.FunctionBody,
			triggerSQL.TriggerDDL,
		} {
			if _, execErr := db.ExecContext(ctx, stmt); execErr != nil {
				regenFailed = true
				break
			}
		}

		// Resolve the condition's column references. CREATE FUNCTION only syntax-checks a
		// plpgsql body, so a condition naming a column this workspace does not have would
		// otherwise survive the migration and then abort every write to contact_timeline —
		// for a workspace that was working before it started.
		if !regenFailed && triggerSQL.ValidationQuery != "" {
			probeRows, probeErr := db.QueryContext(ctx, triggerSQL.ValidationQuery)
			if probeErr != nil {
				regenFailed = true
			} else {
				// An error surfacing while the rows stream counts as a failed probe too:
				// treating it as success would keep a trigger nothing validated.
				closeErr := probeRows.Close()
				if probeRows.Err() != nil || closeErr != nil {
					regenFailed = true
				}
			}
		}

		if regenFailed {
			if _, err := db.ExecContext(ctx, "ROLLBACK TO SAVEPOINT v38_automation_conditions"); err != nil {
				return fmt.Errorf("v38: failed to roll back savepoint: %w", err)
			}
		}
		if _, err := db.ExecContext(ctx, "RELEASE SAVEPOINT v38_automation_conditions"); err != nil {
			return fmt.Errorf("v38: failed to release savepoint: %w", err)
		}
	}

	return nil
}

func init() {
	Register(&V38Migration{})
}
