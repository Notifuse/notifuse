package repository

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/lib/pq"

	"github.com/Notifuse/notifuse/internal/database/schema"
	"github.com/Notifuse/notifuse/internal/domain"
)

// Attribution backfill: when the workspace's rules change, historical
// web_sessions and web_goals rows are rewritten partition by partition with a
// SET clause that reproduces the Go rule evaluator in SQL (Staminads did the
// same with ClickHouse mutations).
//
// Known, documented divergence from the live evaluator: an unset_value rule
// combined with a lower-precedence set_default_value on the same dimension
// yields '' here but the default's value at ingest time. None of the shipped
// default rules combine the two.

// webFilterResetDimensions are cleared when no rule sets them.
//
// The invariant is narrower than "every dimension a rule may write": it is
// "every dimension whose ONLY source is a rule". channel and channel_group
// qualify — nothing else ever writes them, so no matching rule genuinely means
// no value, and clearing keeps a backfill in parity with the live evaluator.
//
// custom_1..custom_10 do NOT qualify, even though rules may write them. The
// tracker fills them from the beat's own dimensions (see applyWebAttribution in
// web_analytics_enrichment.go), so a workspace can populate them with no rule
// involved. Clearing them on backfill destroyed data the site had supplied, and
// since a backfill is triggered by editing an unrelated attribution rule, the
// loss looked unconnected to the action that caused it.
//
// Consequence, accepted: a custom value written by a rule now survives that
// rule's deletion, because nothing can tell it apart from a tracker-supplied one
// after the fact. A workspace that wants such a value gone says so with an
// unset_value rule, which still clears the rows it matches.
var webFilterResetDimensions = map[string]bool{
	"channel": true, "channel_group": true,
}

// webFilterSourceFieldSQL maps a rule source field to the SQL expression
// yielding its evaluator representation (” for empty, 'true'/'false' for
// booleans).
func webFilterSourceFieldSQL(field string) string {
	if field == "is_direct" {
		return "(CASE WHEN is_direct THEN 'true' ELSE 'false' END)"
	}
	return field
}

// compileWebFilterCondition renders one condition. Evaluator parity: missing
// and empty values match only is_empty.
func compileWebFilterCondition(c domain.WebFilterCondition) (string, error) {
	field := webFilterSourceFieldSQL(c.Field)
	literal := pq.QuoteLiteral(c.Value)
	nonEmpty := field + " <> ''"

	switch c.Operator {
	case domain.WebFilterOpEquals:
		return fmt.Sprintf("(%s AND %s = %s)", nonEmpty, field, literal), nil
	case domain.WebFilterOpNotEquals:
		return fmt.Sprintf("(%s AND %s <> %s)", nonEmpty, field, literal), nil
	case domain.WebFilterOpContains:
		return fmt.Sprintf("(%s AND %s LIKE %s)", nonEmpty, field, pq.QuoteLiteral("%"+escapeLikePattern(c.Value)+"%")), nil
	case domain.WebFilterOpNotContains:
		return fmt.Sprintf("(%s AND %s NOT LIKE %s)", nonEmpty, field, pq.QuoteLiteral("%"+escapeLikePattern(c.Value)+"%")), nil
	case domain.WebFilterOpIsEmpty:
		return fmt.Sprintf("(%s = '')", field), nil
	case domain.WebFilterOpIsNotEmpty:
		return fmt.Sprintf("(%s)", nonEmpty), nil
	case domain.WebFilterOpRegex:
		// Rule regexes are validated as RE2 at save time; PostgreSQL evaluates
		// POSIX ARE, which agrees on the anchored/alternation patterns the
		// fixtures use. The two engines are not identical, so an exotic pattern
		// can classify one way at ingest and the other way here — an accepted
		// limit of running the same rules through two regex implementations, not
		// a bug to chase. Keep rule patterns simple and anchored.
		return fmt.Sprintf("(%s AND %s ~ %s)", nonEmpty, field, literal), nil
	default:
		return "", fmt.Errorf("unsupported backfill condition operator: %q", c.Operator)
	}
}

func compileWebFilterMatch(conditions []domain.WebFilterCondition) (string, error) {
	if len(conditions) == 0 {
		return "TRUE", nil
	}
	parts := make([]string, 0, len(conditions))
	for _, c := range conditions {
		compiled, err := compileWebFilterCondition(c)
		if err != nil {
			return "", err
		}
		parts = append(parts, compiled)
	}
	return "(" + strings.Join(parts, " AND ") + ")", nil
}

// compileWebFilterAssignment renders the value a matched operation writes.
func compileWebFilterAssignment(dimension string, action string, value string) string {
	if dimension == "is_direct" {
		if action == domain.WebFilterActionUnsetValue || value != "true" {
			return "FALSE"
		}
		return "TRUE"
	}
	if action == domain.WebFilterActionUnsetValue {
		return "''"
	}
	return pq.QuoteLiteral(value)
}

// CompileWebFiltersToSetClause compiles the enabled rules into the SET clause
// of a backfill UPDATE. Rules are evaluated as CASE branches per dimension,
// set/unset by priority (descending, stable) first, then set_default_value
// rules; reset-dimensions fall back to ” and passthrough dimensions keep
// their stored value.
func CompileWebFiltersToSetClause(filters []domain.WebFilter) (string, error) {
	enabled := make([]domain.WebFilter, 0, len(filters))
	for _, f := range filters {
		if f.Enabled {
			enabled = append(enabled, f)
		}
	}
	sort.SliceStable(enabled, func(i, j int) bool { return enabled[i].Priority > enabled[j].Priority })

	type branch struct {
		condition  string
		assignment string
		isDefault  bool
	}
	branchesByDimension := map[string][]branch{}

	for _, filter := range enabled {
		match, err := compileWebFilterMatch(filter.Conditions)
		if err != nil {
			return "", fmt.Errorf("filter %s: %w", filter.Name, err)
		}
		for _, op := range filter.Operations {
			if !domain.WebFilterWritableDimensions[op.Dimension] {
				return "", fmt.Errorf("filter %s writes non-writable dimension %q", filter.Name, op.Dimension)
			}
			branchesByDimension[op.Dimension] = append(branchesByDimension[op.Dimension], branch{
				condition:  match,
				assignment: compileWebFilterAssignment(op.Dimension, op.Action, op.Value),
				isDefault:  op.Action == domain.WebFilterActionSetDefaultValue,
			})
		}
	}

	// Deterministic column order for testability.
	dimensions := make([]string, 0, len(webFilterResetDimensions)+len(branchesByDimension))
	for dimension := range webFilterResetDimensions {
		dimensions = append(dimensions, dimension)
	}
	for dimension := range branchesByDimension {
		if !webFilterResetDimensions[dimension] {
			dimensions = append(dimensions, dimension)
		}
	}
	sort.Strings(dimensions)

	assignments := make([]string, 0, len(dimensions))
	for _, dimension := range dimensions {
		branches := branchesByDimension[dimension]

		var elseSQL string
		if webFilterResetDimensions[dimension] {
			elseSQL = "''"
		} else if dimension == "is_direct" {
			elseSQL = "is_direct"
		} else {
			elseSQL = dimension
		}

		if len(branches) == 0 {
			// Reset dimensions clear even without rules (evaluator parity: no
			// rules produce no values); passthrough dimensions need no entry.
			if webFilterResetDimensions[dimension] {
				assignments = append(assignments, fmt.Sprintf("%s = ''", dimension))
			}
			continue
		}

		var b strings.Builder
		b.WriteString(dimension + " = CASE")
		// set/unset first (priority order preserved), defaults last: a
		// default never outranks any set_value regardless of priority.
		for _, isDefaultPass := range []bool{false, true} {
			for _, br := range branches {
				if br.isDefault != isDefaultPass {
					continue
				}
				b.WriteString(" WHEN " + br.condition + " THEN " + br.assignment)
			}
		}
		b.WriteString(" ELSE " + elseSQL + " END")
		assignments = append(assignments, b.String())
	}

	if len(assignments) == 0 {
		return "", fmt.Errorf("no assignments compiled")
	}
	return strings.Join(assignments, ", "), nil
}

// BackfillPartition rewrites one partition with the compiled rules. The
// partition name is validated against the naming scheme before interpolation.
func (r *webAnalyticsRepository) BackfillPartition(ctx context.Context, workspaceID string, partition string, filters []domain.WebFilter) (int64, error) {
	table, _, ok := schema.ParseWebAnalyticsPartitionName(partition)
	if !ok {
		return 0, fmt.Errorf("invalid partition name: %s", partition)
	}
	if table != "web_sessions" && table != "web_goals" {
		return 0, fmt.Errorf("backfill only applies to web_sessions and web_goals partitions, got %s", partition)
	}

	setClause, err := CompileWebFiltersToSetClause(filters)
	if err != nil {
		return 0, fmt.Errorf("failed to compile filters: %w", err)
	}

	db, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return 0, fmt.Errorf("failed to get workspace connection: %w", err)
	}

	result, err := db.ExecContext(ctx, fmt.Sprintf("UPDATE %s SET %s", pq.QuoteIdentifier(partition), setClause))
	if err != nil {
		return 0, fmt.Errorf("failed to backfill %s: %w", partition, err)
	}
	rows, _ := result.RowsAffected()
	return rows, nil
}
