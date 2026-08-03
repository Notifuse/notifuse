package migrations

import (
	"context"
	"encoding/json"
	"fmt"

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
// not rewrite the table.
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
	// transaction (manager.go), so the ALTER below holds an ACCESS EXCLUSIVE lock on
	// contact_timeline until that transaction commits — and every workspace write reaches that
	// table through a trigger. Widening a varchar is metadata-only and therefore fast, so doing
	// it last keeps the window where writes are blocked as short as possible instead of spanning
	// a loop over every segment.
	if err := m.recompileSegmentQueries(ctx, db); err != nil {
		return err
	}

	// Re-running is a no-op: ALTER ... TYPE to the width it already has changes nothing.
	if _, err := db.ExecContext(ctx, `
		ALTER TABLE contact_timeline
		ALTER COLUMN kind TYPE VARCHAR(150)
	`); err != nil {
		return fmt.Errorf("v37: failed to widen contact_timeline.kind: %w", err)
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
