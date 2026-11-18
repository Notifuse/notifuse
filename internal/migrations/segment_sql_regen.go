package migrations

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/service"
)

// SegmentSQLResult says what RegenerateSegmentSQL did in one workspace.
type SegmentSQLResult struct {
	// Recompiled counts the segments whose stored query changed and was replaced; each of them
	// is marked for a rebuild.
	Recompiled int
	// Skipped names the segments whose tree could not be recompiled, with why. They keep their
	// stored query.
	Skipped map[string]string
}

// RegenerateSegmentSQL recompiles every segment's tree with the current query builder and, where
// the result differs from the stored generated_sql / generated_args, stores it and marks the
// segment for a rebuild (recompute_after = NOW()).
//
// A segment's query is compiled once, when its tree is saved, and read from the segment from then
// on (segment_build_processor.go, contact_segment_queue_processor.go). A change to how the builder
// compiles a condition therefore reaches no existing segment by itself: 44.0 changes how a value
// inside a custom JSON column is read (as the text it holds rather than its JSON text, which kept a
// string's quotes) and makes the defensive casts test the conversion. Comparing with what is stored
// rather than looking for a pattern catches every shape the change touched, and makes a rerun a
// no-op.
//
// The mark is needed because a changed query can change membership, and only a rebuild removes the
// contacts that leave. The recompute check picks marked active segments; a build that completes
// clears the mark of a segment without relative dates (SegmentBuildProcessor.rescheduleRecompute).
//
// A tree that no longer compiles keeps its stored query and is reported rather than failing the
// migration, which would block server startup. The rows are read and closed before any update,
// because a workspace migration shares one connection.
func RegenerateSegmentSQL(ctx context.Context, db DBExecutor) (SegmentSQLResult, error) {
	result := SegmentSQLResult{Skipped: map[string]string{}}

	rows, err := db.QueryContext(ctx, `
		SELECT id, tree, generated_sql, generated_args FROM segments
		WHERE status <> 'deleted'
		ORDER BY id
	`)
	if err != nil {
		return result, fmt.Errorf("failed to query segments: %w", err)
	}

	type storedSegment struct {
		id       string
		tree     []byte
		sql      *string
		argsJSON []byte
	}
	var segments []storedSegment
	for rows.Next() {
		var s storedSegment
		if scanErr := rows.Scan(&s.id, &s.tree, &s.sql, &s.argsJSON); scanErr != nil {
			_ = rows.Close()
			return result, fmt.Errorf("failed to scan segment: %w", scanErr)
		}
		segments = append(segments, s)
	}
	if iterErr := rows.Err(); iterErr != nil {
		_ = rows.Close()
		return result, fmt.Errorf("error iterating segments: %w", iterErr)
	}
	if closeErr := rows.Close(); closeErr != nil {
		return result, fmt.Errorf("failed to close segments rows: %w", closeErr)
	}

	qb := service.NewQueryBuilder()
	for _, s := range segments {
		var tree domain.TreeNode
		if unmarshalErr := json.Unmarshal(s.tree, &tree); unmarshalErr != nil {
			result.Skipped[s.id] = fmt.Sprintf("tree is not valid JSON: %v", unmarshalErr)
			continue
		}
		sqlQuery, args, buildErr := qb.BuildSQL(&tree)
		if buildErr != nil {
			result.Skipped[s.id] = buildErr.Error()
			continue
		}
		same, compareErr := sameArgs(s.argsJSON, args)
		if compareErr != nil {
			result.Skipped[s.id] = fmt.Sprintf("arguments do not encode: %v", compareErr)
			continue
		}
		if s.sql != nil && *s.sql == sqlQuery && same {
			continue
		}
		// Stored as the service stores them (domain.JSONArray), so that a rerun compares like
		// with like. Only if the query is still the one read: a segment saved meanwhile by a live
		// instance (cmd/regen-json-filters runs beside one) holds a query compiled from its new
		// tree, which this one must not replace.
		if _, execErr := db.ExecContext(ctx, `
			UPDATE segments
			SET generated_sql = $1, generated_args = $2, recompute_after = NOW(), db_updated_at = NOW()
			WHERE id = $3 AND generated_sql IS NOT DISTINCT FROM $4
		`, sqlQuery, domain.JSONArray(args), s.id, s.sql); execErr != nil {
			return result, fmt.Errorf("failed to update segment %s: %w", s.id, execErr)
		}
		result.Recompiled++
	}

	return result, nil
}

// sameArgs reports whether the stored generated_args hold args. They are compared by value, since
// jsonb hands them back with its own spacing, and no arguments match SQL NULL, JSON null and an
// empty array alike: which one a segment holds depends on what wrote it.
func sameArgs(stored []byte, args []interface{}) (bool, error) {
	encoded, err := json.Marshal(args)
	if err != nil {
		return false, err
	}
	var want, got []interface{}
	if err := json.Unmarshal(encoded, &want); err != nil {
		return false, err
	}
	if len(stored) > 0 {
		if json.Unmarshal(stored, &got) != nil {
			return false, nil
		}
	}
	if len(want) == 0 && len(got) == 0 {
		return true, nil
	}
	return reflect.DeepEqual(want, got), nil
}
