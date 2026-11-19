package migrations

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
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
	// Skipped names the segments whose tree could not be recompiled, or that were deleted while
	// they were, with why. They keep their stored query.
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
	for _, seg := range segments {
		// A segment saved meanwhile by a live instance is read again and recompiled from what
		// was saved, a few times; one that keeps changing fails the run, so that the migration
		// is not recorded and runs again at the next start.
		for attempt := 1; ; attempt++ {
			outcome, err := recompileSegment(ctx, db, qb, seg, &result)
			if err != nil {
				return result, err
			}
			if outcome != segmentSavedMeanwhile {
				break
			}
			if attempt == segmentRegenAttempts {
				return result, fmt.Errorf("segment %s was saved again each time it was recompiled; run the migration again", seg.id)
			}
			again, found, readErr := readSegment(ctx, db, seg.id)
			if readErr != nil {
				return result, readErr
			}
			if !found {
				result.Skipped[seg.id] = "deleted while it was recompiled"
				break
			}
			seg = again
		}
	}

	return result, nil
}

// storedSegment is a segment as RegenerateSegmentSQL reads it.
type storedSegment struct {
	id       string
	tree     []byte
	sql      *string
	argsJSON []byte
}

// segmentRegenAttempts bounds how often a segment saved during its recompilation is read again.
const segmentRegenAttempts = 3

type segmentOutcome int

const (
	segmentDone segmentOutcome = iota
	segmentSavedMeanwhile
)

// recompileSegment recompiles one segment as read, and stores the new query only if the segment
// still holds what was read.
func recompileSegment(ctx context.Context, db DBExecutor, qb *service.QueryBuilder, s storedSegment, result *SegmentSQLResult) (segmentOutcome, error) {
	var tree domain.TreeNode
	if unmarshalErr := json.Unmarshal(s.tree, &tree); unmarshalErr != nil {
		result.Skipped[s.id] = fmt.Sprintf("tree is not valid JSON: %v", unmarshalErr)
		return segmentDone, nil
	}
	sqlQuery, args, buildErr := qb.BuildSQL(&tree)
	if buildErr != nil {
		result.Skipped[s.id] = buildErr.Error()
		return segmentDone, nil
	}
	same, compareErr := sameArgs(s.argsJSON, args)
	if compareErr != nil {
		result.Skipped[s.id] = fmt.Sprintf("arguments do not encode: %v", compareErr)
		return segmentDone, nil
	}
	if s.sql != nil && *s.sql == sqlQuery && same {
		return segmentDone, nil
	}
	// Stored as the service stores them (domain.JSONArray), so that a rerun compares like
	// with like. Only if the segment is still the one read: a segment saved meanwhile by a
	// live instance (cmd/regen-json-filters runs beside one) holds a new tree, which this
	// query was not compiled from. The query alone does not tell: a new value in the same
	// condition changes only the tree and the arguments.
	res, execErr := db.ExecContext(ctx, segmentRegenUpdateSQL,
		sqlQuery, domain.JSONArray(args), s.id, s.sql, string(s.tree), nullableJSON(s.argsJSON))
	if execErr != nil {
		return segmentDone, fmt.Errorf("failed to update segment %s: %w", s.id, execErr)
	}
	updated, rowsErr := res.RowsAffected()
	if rowsErr != nil {
		return segmentDone, fmt.Errorf("failed to read the update of segment %s: %w", s.id, rowsErr)
	}
	if updated == 0 {
		return segmentSavedMeanwhile, nil
	}
	result.Recompiled++
	return segmentDone, nil
}

// readSegment reads one segment again; found is false once it is deleted.
func readSegment(ctx context.Context, db DBExecutor, id string) (storedSegment, bool, error) {
	s := storedSegment{id: id}
	err := db.QueryRowContext(ctx, `SELECT tree, generated_sql, generated_args FROM segments WHERE id = $1 AND status <> 'deleted'`, id).
		Scan(&s.tree, &s.sql, &s.argsJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return s, false, nil
	}
	if err != nil {
		return s, false, fmt.Errorf("failed to read segment %s again: %w", id, err)
	}
	return s, true, nil
}

// segmentRegenUpdateSQL stores a recompiled query if the segment still holds the query, tree and
// arguments that were read: jsonb compares the tree and the arguments by value.
const segmentRegenUpdateSQL = `
			UPDATE segments
			SET generated_sql = $1, generated_args = $2, recompute_after = NOW(), db_updated_at = NOW()
			WHERE id = $3 AND status <> 'deleted' AND generated_sql IS NOT DISTINCT FROM $4
			  AND tree = $5::jsonb AND generated_args IS NOT DISTINCT FROM $6::jsonb
		`

// nullableJSON passes stored JSON as text, and SQL NULL as nil.
func nullableJSON(b []byte) interface{} {
	if b == nil {
		return nil
	}
	return string(b)
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
