package migrations

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/service"
	"github.com/lib/pq"
)

// regenerateAutomationTriggers rebuilds the installed trigger of every live automation with
// the current generator, from the configuration stored against it. v38 runs it to repair
// triggers that no longer implemented their automation (see
// V38Migration.healAutomationTriggerConditions); v44 runs it, bodies only, so every trigger
// passes the event that fired it to automation_enroll_contact.
//
// With bodiesOnly, only each trigger's function is replaced (CREATE OR REPLACE FUNCTION), its
// trigger and WHEN clause staying as installed: no lock is taken on contact_timeline, so it
// can run inside a migration that already holds other tables, and a trigger can never be
// broadened, which the full regeneration has to guard against.
//
// Repair-only: an automation with no installed trigger is skipped rather than armed, and a
// trigger is never traded for a broader one. Each automation gets its own savepoint, so a
// failure rolls back just that automation and leaves its existing trigger in place, except a
// failure that says nothing about the automation (a lock timeout, a deadlock), which fails the
// whole run so that the migration is not recorded and runs again (isTransientRegenError). prefix
// starts the error messages ("v38"), and savepoint names the savepoint. generator builds the
// trigger bodies: each migration passes the one matching the enrolment functions it installed.
//
// It returns how many triggers it replaced and the automations it left as they were, with why, so
// that a caller run by a person (cmd/regen-json-filters) can say so; the migrations only need the
// error.
func regenerateAutomationTriggers(ctx context.Context, db DBExecutor, generator *service.AutomationTriggerGenerator, prefix, savepoint string, bodiesOnly bool) (AutomationTriggerResult, error) {
	skipped := map[string]string{}
	replaced := 0
	result := func() AutomationTriggerResult { return AutomationTriggerResult{Replaced: replaced, Skipped: skipped} }
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
		return result(), fmt.Errorf("%s: failed to query automations with trigger conditions: %w", prefix, err)
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
			return result(), fmt.Errorf("%s: failed to scan automation: %w", prefix, scanErr)
		}
		a.rootNodeID = rootNodeID.String
		a.installedDef = installedDef.String
		automations = append(automations, a)
	}
	if iterErr := rows.Err(); iterErr != nil {
		_ = rows.Close()
		return result(), fmt.Errorf("%s: error iterating automations: %w", prefix, iterErr)
	}
	if closeErr := rows.Close(); closeErr != nil {
		return result(), fmt.Errorf("%s: failed to close automations rows: %w", prefix, closeErr)
	}

	if len(automations) == 0 {
		return result(), nil
	}

	for _, a := range automations {
		var trigger domain.TimelineTriggerConfig
		if unmarshalErr := json.Unmarshal(a.triggerConfig, &trigger); unmarshalErr != nil {
			// Unparseable config — the installed trigger was already unusable; leave it.
			skipped[a.id] = fmt.Sprintf("trigger configuration is not valid JSON: %v", unmarshalErr)
			continue
		}

		automation := &domain.Automation{ID: a.id, RootNodeID: a.rootNodeID, Trigger: &trigger}
		triggerSQL, genErr := generator.Generate(automation)
		if genErr != nil {
			// Incomplete or corrupt automation config — skip rather than abort.
			skipped[a.id] = genErr.Error()
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
		if !bodiesOnly && strings.Contains(a.installedDef, "changes ?") &&
			!strings.Contains(triggerSQL.WHENClause, "changes ?") {
			skipped[a.id] = "the stored configuration would broaden the installed trigger"
			continue
		}

		// One savepoint per automation, so a failure rolls back just this automation and
		// leaves its existing trigger in place. Failing hard instead would abort the whole
		// workspace migration, never record the version, and re-fail on every restart —
		// and here the fallback is merely the status quo, an over-enrolling trigger. The
		// exception is a lock or a deadlock behind a concurrent transaction
		// (isTransientRegenError): it says nothing about the automation and passes, so the
		// run fails and the next start tries again.
		if _, err := db.ExecContext(ctx, "SAVEPOINT "+savepoint); err != nil {
			return result(), fmt.Errorf("%s: failed to create savepoint: %w", prefix, err)
		}

		regenFailed := false

		// Same order as AutomationRepository.CreateAutomationTrigger — including running
		// the probe last. Probing first would take ACCESS SHARE on contact_timeline and
		// then upgrade to the ACCESS EXCLUSIVE the DROPs need; during a rolling restart the
		// old instance is still serving automations.activate, and that upgrade is exactly
		// what deadlocks. The savepoint undoes the DDL either way, so nothing invalid
		// survives a failed probe.
		statements := []string{
			triggerSQL.DropTrigger,
			triggerSQL.DropFunction,
			triggerSQL.FunctionBody,
			triggerSQL.TriggerDDL,
		}
		if bodiesOnly {
			statements = []string{triggerSQL.FunctionBody}
		}
		for _, stmt := range statements {
			if _, execErr := db.ExecContext(ctx, stmt); execErr != nil {
				if isTransientRegenError(execErr) {
					return result(), fmt.Errorf("%s: automation %s: %w (the trigger is left as it was; the migration runs again at the next start)", prefix, a.id, execErr)
				}
				regenFailed = true
				skipped[a.id] = execErr.Error()
				break
			}
		}

		// Resolve the condition's column references. CREATE FUNCTION only syntax-checks a
		// plpgsql body, so a condition naming a column this workspace does not have would
		// otherwise survive the migration and then abort every write to contact_timeline —
		// for a workspace that was working before it started.
		if !regenFailed && triggerSQL.ValidationQuery != "" {
			probeRows, probeErr := db.QueryContext(ctx, triggerSQL.ValidationQuery)
			if probeErr != nil && isTransientRegenError(probeErr) {
				return result(), fmt.Errorf("%s: automation %s: %w (the trigger is left as it was; the migration runs again at the next start)", prefix, a.id, probeErr)
			}
			if probeErr != nil {
				regenFailed = true
				skipped[a.id] = probeErr.Error()
			} else {
				// An error surfacing while the rows stream counts as a failed probe too:
				// treating it as success would keep a trigger nothing validated.
				closeErr := probeRows.Close()
				for _, streamErr := range []error{probeRows.Err(), closeErr} {
					if streamErr != nil && isTransientRegenError(streamErr) {
						return result(), fmt.Errorf("%s: automation %s: %w (the trigger is left as it was; the migration runs again at the next start)", prefix, a.id, streamErr)
					}
				}
				if probeRows.Err() != nil || closeErr != nil {
					regenFailed = true
					skipped[a.id] = "the trigger's conditions could not be checked against this workspace"
				}
			}
		}

		if !regenFailed {
			replaced++
		}
		if regenFailed {
			if _, err := db.ExecContext(ctx, "ROLLBACK TO SAVEPOINT "+savepoint); err != nil {
				return result(), fmt.Errorf("%s: failed to roll back savepoint: %w", prefix, err)
			}
		}
		if _, err := db.ExecContext(ctx, "RELEASE SAVEPOINT "+savepoint); err != nil {
			return result(), fmt.Errorf("%s: failed to release savepoint: %w", prefix, err)
		}
	}

	return result(), nil
}

// isTransientRegenError reports a failure that says nothing about the automation itself: a lock
// not obtained within lock_timeout, a deadlock or a serialization failure, behind a concurrent
// transaction. A statement cancelled by statement_timeout (57014) is not one: on a role that sets
// it, the same statement would time out again at every start, which would keep the server down. Skipping the automation then would leave a valid one on its old
// trigger body for good, once the migration records its version; failing lets the next start run
// the migration again.
func isTransientRegenError(err error) bool {
	var pqErr *pq.Error
	if !errors.As(err, &pqErr) {
		return false
	}
	switch pqErr.Code {
	case "55P03", "40P01", "40001":
		return true
	}
	return false
}

// AutomationTriggerResult says what a trigger regeneration did: the triggers replaced, and the
// automations left as they were, with why.
type AutomationTriggerResult struct {
	Replaced int
	Skipped  map[string]string
}

// RegenerateAutomationTriggerBodies replaces the function body of every live automation's trigger
// with what the current generator builds, as v44 does, and returns the automations left as they
// were, with why. For cmd/regen-json-filters, on a database already at 44: run inside a
// transaction, since it uses savepoints.
func RegenerateAutomationTriggerBodies(ctx context.Context, db DBExecutor) (AutomationTriggerResult, error) {
	generator := service.NewAutomationTriggerGenerator(service.NewQueryBuilder())
	return regenerateAutomationTriggers(ctx, db, generator, "regen", "regen_automation_trigger_bodies", true)
}
