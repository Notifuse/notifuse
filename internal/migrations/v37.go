package migrations

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/lib/pq"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/service"
)

// V37Migration widens contact_timeline.kind and repairs stored segment queries.
//
// Part 1 — contact_timeline.kind was VARCHAR(50), but track_custom_event_timeline() writes
// 'custom_event.' || event_name and event_name accepts up to 100 characters
// (domain.CustomEvent.Validate). Any event name longer than 37 characters therefore overflowed
// the column, and because the writer is an AFTER INSERT trigger the error aborted the whole
// custom_events insert — such events could not be recorded at all. The column now holds the
// longest kind the triggers can produce (13 + 100) with room to spare. Widening a varchar does
// not rewrite the table, but it still cannot be done in place while a trigger depends on the
// column — see widenKindColumn.
//
// Part 2 — segment membership is computed from the stored segments.generated_sql, not from the
// tree, so a fix in the query builder only reaches segments that are saved again afterwards.
// Timeline dimension filters used to interpolate their field_name straight into the SQL text;
// segments compiled before the fix keep that SQL until they are re-saved. Every segment whose
// stored query carries the interpolated form is recompiled from its tree so the parameterized
// form takes effect immediately.
type V37Migration struct{}

func (m *V37Migration) GetMajorVersion() float64 { return 37.0 }

func (m *V37Migration) HasSystemUpdate() bool { return false }

func (m *V37Migration) HasWorkspaceUpdate() bool { return true }

// ShouldRestartServer indicates if the server should restart after this migration
func (m *V37Migration) ShouldRestartServer() bool { return false }

func (m *V37Migration) UpdateSystem(ctx context.Context, cfg *config.Config, db DBExecutor) error {
	return nil
}

func (m *V37Migration) UpdateWorkspace(ctx context.Context, cfg *config.Config, workspace *domain.Workspace, db DBExecutor) error {
	// Disable statement_timeout for this migration transaction. The segment recompile below is a
	// loop over every segment, and the ALTER has to wait for an exclusive lock on a table that
	// every workspace write touches — either can outlast a globally-configured statement_timeout.
	// If one is aborted the workspace migration rolls back and the version is never bumped, so
	// every subsequent restart re-attempts and fails identically, bricking startup. SET LOCAL is
	// scoped to this transaction only.
	if _, err := db.ExecContext(ctx, "SET LOCAL statement_timeout = 0"); err != nil {
		return fmt.Errorf("v37: failed to disable statement_timeout: %w", err)
	}

	// The segment recompile runs first on purpose. Workspace migrations execute inside one
	// transaction (manager.go), so the widening below holds an ACCESS EXCLUSIVE lock on
	// contact_timeline until that transaction commits — and every workspace write reaches that
	// table through a trigger. Widening a varchar is metadata-only and therefore fast, so doing
	// it last keeps the window where writes are blocked as short as possible instead of spanning
	// a loop over every segment.
	if err := m.recompileSegmentQueries(ctx, db); err != nil {
		return err
	}

	if err := m.widenKindColumn(ctx, db); err != nil {
		return err
	}

	return nil
}

// kindDependentTrigger is a trigger whose definition depends on contact_timeline.kind, captured
// so it can be put back byte for byte after the column has been widened.
type kindDependentTrigger struct {
	name       string
	definition string
	// enabled is pg_trigger.tgenabled: 'O' origin (what CREATE TRIGGER produces), 'D' disabled,
	// 'R' replica, 'A' always.
	enabled string
}

// widenKindColumn widens contact_timeline.kind, around the triggers that depend on it.
//
// PostgreSQL refuses ALTER COLUMN ... TYPE while any trigger definition depends on the column
// ("cannot alter type of a column used in a trigger definition"), and it refuses whether or not
// the change needs a table rewrite — widening a varchar does not. Every live automation installs
// such a trigger on contact_timeline: its WHEN clause reads NEW.kind (see the automation trigger
// generator, where the event-kind filter is unconditional). One live automation was therefore
// enough to abort this migration, and because the database version is only recorded once every
// workspace has succeeded, the server then failed to boot on every restart.
//
// The dependent triggers are dropped and recreated from the definitions PostgreSQL itself
// reports, so the DDL that comes back is identical to whatever created them and this migration
// needs no knowledge of automations. It all happens inside the workspace migration's
// transaction: if any step fails the rollback leaves every trigger in place, and the exclusive
// lock the DROPs take is released with it.
func (m *V37Migration) widenKindColumn(ctx context.Context, db DBExecutor) error {
	// Nothing to do on a column that already holds what the triggers write, and nothing to be
	// gained from finding out the hard way: the statements below drop triggers and take an ACCESS
	// EXCLUSIVE lock, which is wasted work on every retry after a different workspace in the same
	// install failed.
	//
	// atttypmod carries the declared length plus the 4 byte varlena header, and is -1 when there is
	// no declared length at all — TEXT, or VARCHAR without a limit. Such a column is wider than the
	// target, and altering it to VARCHAR(150) would fail outright on any row longer than that, so
	// it is left alone rather than narrowed.
	var typmod int
	if err := db.QueryRowContext(ctx, `
		SELECT atttypmod FROM pg_attribute
		WHERE attrelid = 'contact_timeline'::regclass AND attname = 'kind'
	`).Scan(&typmod); err != nil {
		return fmt.Errorf("v37: failed to read the width of contact_timeline.kind: %w", err)
	}
	if typmod < 0 || typmod >= 150+4 {
		return nil
	}

	dependents, err := m.kindDependentTriggers(ctx, db)
	if err != nil {
		return err
	}

	for _, trigger := range dependents {
		if _, err := db.ExecContext(ctx, fmt.Sprintf(
			"DROP TRIGGER %s ON contact_timeline", pq.QuoteIdentifier(trigger.name),
		)); err != nil {
			return fmt.Errorf("v37: failed to drop trigger %s: %w", trigger.name, err)
		}
	}

	if _, err := db.ExecContext(ctx, `
		ALTER TABLE contact_timeline
		ALTER COLUMN kind TYPE VARCHAR(150)
	`); err != nil {
		return fmt.Errorf("v37: failed to widen contact_timeline.kind: %w", err)
	}

	for _, trigger := range dependents {
		if _, err := db.ExecContext(ctx, trigger.definition); err != nil {
			// Failing here is the point: rolling back keeps the trigger rather than completing
			// the migration with a live automation that no longer fires.
			return fmt.Errorf("v37: failed to recreate trigger %s: %w", trigger.name, err)
		}
		if err := m.restoreTriggerEnabledState(ctx, db, trigger); err != nil {
			return err
		}
	}

	return nil
}

// kindDependentTriggers reads every non-internal trigger that PostgreSQL records a dependency on
// contact_timeline.kind for. Reading pg_depend rather than matching on the WHEN clause text
// captures exactly the set that blocks the ALTER: a WHEN clause reading NEW.kind, but equally an
// "UPDATE OF kind" column list, which carries no WHEN clause at all. Matching on refobjsubid
// picks the column-level dependency only: every trigger also has an auto dependency on the table
// itself, recorded with refobjsubid 0. The result set is fully read and closed before any DDL is
// issued, because the workspace migration shares a single connection.
func (m *V37Migration) kindDependentTriggers(ctx context.Context, db DBExecutor) ([]kindDependentTrigger, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT DISTINCT t.tgname, t.tgenabled, pg_get_triggerdef(t.oid)
		FROM pg_trigger t
		JOIN pg_depend d ON d.classid = 'pg_trigger'::regclass AND d.objid = t.oid
		WHERE d.refclassid = 'pg_class'::regclass
		  AND d.refobjid = 'contact_timeline'::regclass
		  AND d.refobjsubid = (
			SELECT attnum FROM pg_attribute
			WHERE attrelid = 'contact_timeline'::regclass AND attname = 'kind'
		  )
		  AND NOT t.tgisinternal
		ORDER BY t.tgname
	`)
	if err != nil {
		return nil, fmt.Errorf("v37: failed to read triggers depending on contact_timeline.kind: %w", err)
	}

	var triggers []kindDependentTrigger
	for rows.Next() {
		var trigger kindDependentTrigger
		if scanErr := rows.Scan(&trigger.name, &trigger.enabled, &trigger.definition); scanErr != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("v37: failed to scan trigger: %w", scanErr)
		}
		triggers = append(triggers, trigger)
	}
	if iterErr := rows.Err(); iterErr != nil {
		_ = rows.Close()
		return nil, fmt.Errorf("v37: error iterating triggers: %w", iterErr)
	}
	if closeErr := rows.Close(); closeErr != nil {
		return nil, fmt.Errorf("v37: failed to close triggers rows: %w", closeErr)
	}

	return triggers, nil
}

// restoreTriggerEnabledState puts back a state CREATE TRIGGER cannot express. A recreated trigger
// is always enabled for origin, so a trigger that was disabled — or set to fire on a replica, or
// always — has to be switched back, otherwise the migration silently changes when it runs.
func (m *V37Migration) restoreTriggerEnabledState(ctx context.Context, db DBExecutor, trigger kindDependentTrigger) error {
	var action string
	switch trigger.enabled {
	case "D":
		action = "DISABLE TRIGGER"
	case "R":
		action = "ENABLE REPLICA TRIGGER"
	case "A":
		action = "ENABLE ALWAYS TRIGGER"
	default:
		// 'O' is what the recreated trigger already is.
		return nil
	}

	if _, err := db.ExecContext(ctx, fmt.Sprintf(
		"ALTER TABLE contact_timeline %s %s", action, pq.QuoteIdentifier(trigger.name),
	)); err != nil {
		return fmt.Errorf("v37: failed to restore enabled state of trigger %s: %w", trigger.name, err)
	}

	return nil
}

// recompileSegmentQueries rebuilds generated_sql/generated_args from the stored tree for every
// segment whose compiled query still splices a timeline change key into the SQL text
// ("ct.changes->'<key>'"). The parameterized form the query builder now emits is
// "ct.changes->$n", so the LIKE below cannot match an already-repaired segment and a re-run is a
// no-op. The result set is fully read and closed before issuing updates, because the workspace
// migration shares a single connection.
func (m *V37Migration) recompileSegmentQueries(ctx context.Context, db DBExecutor) error {
	rows, err := db.QueryContext(ctx, `
		SELECT id, tree FROM segments
		WHERE generated_sql LIKE '%ct.changes->''%'
	`)
	if err != nil {
		return fmt.Errorf("v37: failed to query segments: %w", err)
	}

	type segmentTree struct {
		id   string
		tree domain.TreeNode
	}
	var pending []segmentTree
	for rows.Next() {
		var id string
		var treeJSON []byte
		if scanErr := rows.Scan(&id, &treeJSON); scanErr != nil {
			_ = rows.Close()
			return fmt.Errorf("v37: failed to scan segment: %w", scanErr)
		}
		var tree domain.TreeNode
		// A malformed tree cannot be recompiled; leave the segment alone rather than abort the
		// migration, which would block server startup.
		if json.Unmarshal(treeJSON, &tree) != nil {
			continue
		}
		pending = append(pending, segmentTree{id: id, tree: tree})
	}
	if iterErr := rows.Err(); iterErr != nil {
		_ = rows.Close()
		return fmt.Errorf("v37: error iterating segments: %w", iterErr)
	}
	if closeErr := rows.Close(); closeErr != nil {
		return fmt.Errorf("v37: failed to close segments rows: %w", closeErr)
	}

	if len(pending) == 0 {
		return nil
	}

	qb := service.NewQueryBuilder()
	for _, p := range pending {
		sqlQuery, args, buildErr := qb.BuildSQL(&p.tree)
		if buildErr != nil {
			// The tree no longer compiles, so it was already producing a query that could not be
			// rebuilt from it. Keep the stored SQL rather than blank the segment.
			continue
		}
		argsJSON, marshalErr := json.Marshal(args)
		if marshalErr != nil {
			continue
		}
		if _, execErr := db.ExecContext(ctx, `
			UPDATE segments SET generated_sql = $1, generated_args = $2
			WHERE id = $3
		`, sqlQuery, argsJSON, p.id); execErr != nil {
			return fmt.Errorf("v37: failed to update segment %s: %w", p.id, execErr)
		}
	}

	return nil
}

func init() {
	Register(&V37Migration{})
}
