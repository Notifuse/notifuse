package migrations

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/service"
)

// V36Migration heals automation triggers for "email.*" event kinds.
//
// Automations fire AFTER INSERT ON contact_timeline and match NEW.kind against the
// trigger's event kind. The console stores email triggers using the dotted
// "email.<verb>" convention (e.g. "email.clicked"), but track_message_history_changes()
// writes the channel-suffixed / generic forms ("click_email", "open_email",
// "insert_message_history", "update_message_history"). Every previously-activated
// email.* automation trigger therefore had a WHEN clause that could never match a real
// timeline row and has silently never fired.
//
// The trigger generator now translates these kinds (see emailEventKindToTimelineKind in
// the service package), which fixes any automation activated from this version onward.
// This migration repairs the automations that were ALREADY live by regenerating their
// installed trigger DDL. It is safe because the broken triggers never fired, so
// re-creating them cannot disrupt any in-flight enrollment.
//
// The regeneration reuses the canonical trigger generator (rather than a hand-written
// SQL rewrite) so the healed DDL is byte-for-byte what a fresh activation would produce;
// pg_get_triggerdef() normalization (lower-casing, ::text casts) makes an in-place string
// rewrite of the stored definition fragile and error-prone.
type V36Migration struct{}

func (m *V36Migration) GetMajorVersion() float64 { return 36.0 }

func (m *V36Migration) HasSystemUpdate() bool { return false }

func (m *V36Migration) HasWorkspaceUpdate() bool { return true }

func (m *V36Migration) ShouldRestartServer() bool { return false }

func (m *V36Migration) UpdateSystem(ctx context.Context, cfg *config.Config, db DBExecutor) error {
	return nil
}

func (m *V36Migration) UpdateWorkspace(ctx context.Context, cfg *config.Config, workspace *domain.Workspace, db DBExecutor) error {
	// The workspace migration runs inside a single transaction, so the result set must be
	// fully read and closed before issuing the regeneration statements on the same
	// connection. Collect the live email.* automations first, then regenerate.
	type liveAutomation struct {
		id         string
		rootNodeID string
		trigger    *domain.TimelineTriggerConfig
	}

	rows, err := db.QueryContext(ctx, `
		SELECT id, root_node_id, trigger_config
		FROM automations
		WHERE status = 'live' AND deleted_at IS NULL
	`)
	if err != nil {
		return fmt.Errorf("failed to query live automations: %w", err)
	}

	var toFix []liveAutomation
	for rows.Next() {
		var id string
		// root_node_id is nullable in the schema; scan defensively so a NULL cannot error
		// the scan and abort the whole migration (which would block server startup). A
		// missing root node makes Generate() fail, so the automation is skipped below.
		var rootNodeID sql.NullString
		var triggerJSON []byte
		if scanErr := rows.Scan(&id, &rootNodeID, &triggerJSON); scanErr != nil {
			_ = rows.Close()
			return fmt.Errorf("failed to scan automation: %w", scanErr)
		}

		var trigger domain.TimelineTriggerConfig
		if unmarshalErr := json.Unmarshal(triggerJSON, &trigger); unmarshalErr != nil {
			_ = rows.Close()
			return fmt.Errorf("failed to unmarshal trigger config for automation %s: %w", id, unmarshalErr)
		}

		// Only email.* triggers were affected by the kind mismatch; leave the rest
		// (contact.*, list.*, segment.*, custom_event) untouched to minimize churn.
		if strings.HasPrefix(trigger.EventKind, "email.") {
			t := trigger
			toFix = append(toFix, liveAutomation{id: id, rootNodeID: rootNodeID.String, trigger: &t})
		}
	}
	if iterErr := rows.Err(); iterErr != nil {
		_ = rows.Close()
		return fmt.Errorf("error iterating automations: %w", iterErr)
	}
	if closeErr := rows.Close(); closeErr != nil {
		return fmt.Errorf("failed to close automations rows: %w", closeErr)
	}

	if len(toFix) == 0 {
		return nil
	}

	generator := service.NewAutomationTriggerGenerator(service.NewQueryBuilder())
	for _, a := range toFix {
		// Never let a single automation abort the migration: a workspace migration error
		// aborts the entire run (manager.go), which would block server startup for the
		// whole instance. Every automation here was already broken (its trigger never
		// fired), so skipping one leaves it no worse than before.

		// Trigger-level Conditions compile to a WHEN clause containing a subquery, which
		// PostgreSQL rejects ("cannot use subquery in trigger WHEN condition"). Such an
		// automation can only be live if the conditions were added via Update after
		// activation (Update does not regenerate the trigger), so its installed trigger
		// never enforced them anyway — leave it untouched.
		if a.trigger.Conditions != nil {
			continue
		}

		automation := &domain.Automation{
			ID:         a.id,
			RootNodeID: a.rootNodeID,
			Trigger:    a.trigger,
		}

		triggerSQL, genErr := generator.Generate(automation)
		if genErr != nil {
			// Incomplete/corrupt automation config — skip rather than abort.
			continue
		}

		// Guard each automation's DDL with a savepoint so an unexpected CREATE failure
		// rolls back just this automation (restoring its original trigger) instead of
		// poisoning the whole transaction and aborting startup.
		if _, err := db.ExecContext(ctx, "SAVEPOINT v36_regen"); err != nil {
			return fmt.Errorf("failed to create savepoint: %w", err)
		}

		regenFailed := false
		// Execute in the same order as AutomationRepository.CreateAutomationTrigger:
		// drop existing trigger, drop existing function, create function, create trigger.
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

		if regenFailed {
			// Clear the aborted-transaction state and undo this automation's partial DDL.
			if _, err := db.ExecContext(ctx, "ROLLBACK TO SAVEPOINT v36_regen"); err != nil {
				return fmt.Errorf("failed to roll back savepoint: %w", err)
			}
		}
		if _, err := db.ExecContext(ctx, "RELEASE SAVEPOINT v36_regen"); err != nil {
			return fmt.Errorf("failed to release savepoint: %w", err)
		}
	}

	return nil
}

func init() {
	Register(&V36Migration{})
}
