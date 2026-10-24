package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
)

// Structural contract of BuildSQL/BuildTriggerCondition: malformed trees are
// rejected with precise errors (a vague failure here becomes a 500 in segment
// preview), and goal conditions compile to an EXISTS with the aggregate in
// HAVING — never a plain WHERE, which would count wrong.

func TestBuildSQL_TreeStructureErrors(t *testing.T) {
	qb := NewQueryBuilder()

	t.Run("nil tree is rejected", func(t *testing.T) {
		_, _, err := qb.BuildSQL(nil)
		require.EqualError(t, err, "tree cannot be nil")
	})

	t.Run("a tree failing domain validation is rejected as invalid", func(t *testing.T) {
		_, _, err := qb.BuildSQL(&domain.TreeNode{Kind: "sideways"})
		require.ErrorContains(t, err, "invalid tree:")
		assert.Contains(t, err.Error(), "invalid tree node kind: sideways")
	})

	t.Run("each leaf source requires its payload, named in the error", func(t *testing.T) {
		cases := []struct {
			source string
			leaf   *domain.TreeNodeLeaf
			want   string
		}{
			{"contacts", &domain.TreeNodeLeaf{Source: "contacts"},
				"leaf with source 'contacts' must have 'contact' field"},
			{"contact_lists", &domain.TreeNodeLeaf{Source: "contact_lists"},
				"leaf with source 'contact_lists' must have 'contact_list' field"},
			{"contact_timeline", &domain.TreeNodeLeaf{Source: "contact_timeline"},
				"leaf with source 'contact_timeline' must have 'contact_timeline' field"},
			{"custom_events_goals", &domain.TreeNodeLeaf{Source: "custom_events_goals"},
				"leaf with source 'custom_events_goals' must have 'custom_events_goal' field"},
		}
		for _, tc := range cases {
			_, _, _, err := qb.parseLeaf(tc.leaf, 1)
			assert.EqualError(t, err, tc.want, "source %s", tc.source)
		}
	})

	t.Run("unsupported source lists the supported ones", func(t *testing.T) {
		_, _, _, err := qb.parseLeaf(&domain.TreeNodeLeaf{Source: "orders"}, 1)
		require.EqualError(t, err,
			"unsupported source: orders (supported: 'contacts', 'contact_lists', 'contact_timeline', 'custom_events_goals')")
	})

	t.Run("nil leaf and nil branch are errors, not panics", func(t *testing.T) {
		_, _, _, err := qb.parseLeaf(nil, 1)
		require.EqualError(t, err, "leaf cannot be nil")
		_, _, _, err = qb.parseBranch(nil, 1)
		require.EqualError(t, err, "branch cannot be nil")
		_, _, _, err = qb.parseContactConditions(nil, 1)
		require.EqualError(t, err, "contact condition cannot be nil")
	})

	t.Run("an empty branch compiles to no condition rather than ()", func(t *testing.T) {
		cond, args, next, err := qb.parseBranch(&domain.TreeNodeBranch{Operator: "and"}, 3)
		require.NoError(t, err)
		assert.Equal(t, "", cond)
		assert.Empty(t, args)
		assert.Equal(t, 3, next)
	})

	t.Run("OR branches join with OR and keep placeholder order", func(t *testing.T) {
		tree := &domain.TreeNode{
			Kind: "branch",
			Branch: &domain.TreeNodeBranch{
				Operator: "or",
				Leaves: []*domain.TreeNode{
					{Kind: "leaf", Leaf: &domain.TreeNodeLeaf{Source: "contacts", Contact: &domain.ContactCondition{
						Filters: []*domain.DimensionFilter{
							{FieldName: "country", FieldType: "string", Operator: "equals", StringValues: []string{"FR"}},
						},
					}}},
					{Kind: "leaf", Leaf: &domain.TreeNodeLeaf{Source: "contacts", Contact: &domain.ContactCondition{
						Filters: []*domain.DimensionFilter{
							{FieldName: "country", FieldType: "string", Operator: "equals", StringValues: []string{"DE"}},
						},
					}}},
				},
			},
		}
		sql, args, err := qb.BuildSQL(tree)
		require.NoError(t, err)
		assert.Contains(t, sql, " OR ")
		assert.NotContains(t, sql, " AND (", "the two leaves are alternatives, not a conjunction")
		assert.Equal(t, []interface{}{"FR", "DE"}, args)
		assert.Contains(t, sql, "$1")
		assert.Contains(t, sql, "$2")
	})
}

func TestBuildSQL_GoalConditions(t *testing.T) {
	qb := NewQueryBuilder()
	goalLeaf := func(goal *domain.CustomEventsGoalCondition) *domain.TreeNode {
		return &domain.TreeNode{
			Kind: "leaf",
			Leaf: &domain.TreeNodeLeaf{Source: "custom_events_goals", CustomEventsGoal: goal},
		}
	}

	t.Run("sum gte compiles to EXISTS with HAVING and ordered args", func(t *testing.T) {
		sql, args, err := qb.BuildSQL(goalLeaf(&domain.CustomEventsGoalCondition{
			GoalType: "purchase", TimeframeOperator: "anytime", AggregateOperator: "sum", Operator: "gte", Value: 250,
		}))
		require.NoError(t, err)
		assert.Contains(t, sql, "EXISTS (SELECT 1 FROM custom_events ce WHERE ce.email = contacts.email")
		assert.Contains(t, sql, "ce.deleted_at IS NULL", "soft-deleted events never count")
		assert.Contains(t, sql, "ce.goal_type = $1")
		assert.Contains(t, sql, "GROUP BY ce.email HAVING COALESCE(SUM(ce.goal_value), 0) >= $2")
		assert.Equal(t, []interface{}{"purchase", 250.0}, args)
	})

	t.Run("wildcard goal type requires a typed row instead of matching one", func(t *testing.T) {
		sql, args, err := qb.BuildSQL(goalLeaf(&domain.CustomEventsGoalCondition{
			GoalType: "*", TimeframeOperator: "anytime", AggregateOperator: "count", Operator: "gte", Value: 1,
		}))
		require.NoError(t, err)
		assert.Contains(t, sql, "ce.goal_type IS NOT NULL")
		assert.NotContains(t, sql, "ce.goal_type = $")
		assert.Equal(t, []interface{}{1.0}, args, "the value takes the first placeholder")
	})

	t.Run("invalid goal type, aggregate and operator are each rejected", func(t *testing.T) {
		_, _, err := qb.BuildSQL(goalLeaf(&domain.CustomEventsGoalCondition{
			GoalType: "donation", TimeframeOperator: "anytime", AggregateOperator: "sum", Operator: "gte", Value: 1,
		}))
		require.ErrorContains(t, err, "invalid goal_type: donation")

		_, _, err = qb.BuildSQL(goalLeaf(&domain.CustomEventsGoalCondition{
			GoalType: "purchase", TimeframeOperator: "anytime", AggregateOperator: "median", Operator: "gte", Value: 1,
		}))
		require.ErrorContains(t, err, "invalid aggregate_operator: median")

		_, _, err = qb.BuildSQL(goalLeaf(&domain.CustomEventsGoalCondition{
			GoalType: "purchase", TimeframeOperator: "anytime", AggregateOperator: "sum", Operator: "approx", Value: 1,
		}))
		require.ErrorContains(t, err, "invalid operator: approx")
	})

	t.Run("between needs value_2 and consumes two placeholders", func(t *testing.T) {
		_, _, err := qb.BuildSQL(goalLeaf(&domain.CustomEventsGoalCondition{
			GoalType: "purchase", TimeframeOperator: "anytime", AggregateOperator: "sum", Operator: "between", Value: 10,
		}))
		require.ErrorContains(t, err, "between operator requires 'value_2'")

		high := 99.0
		sql, args, err := qb.BuildSQL(goalLeaf(&domain.CustomEventsGoalCondition{
			GoalType: "purchase", TimeframeOperator: "anytime", AggregateOperator: "sum", Operator: "between", Value: 10, Value2: &high,
		}))
		require.NoError(t, err)
		assert.Contains(t, sql, "BETWEEN $2 AND $3")
		assert.Equal(t, []interface{}{"purchase", 10.0, 99.0}, args)
	})

	t.Run("negation wraps the whole EXISTS, never just the comparison", func(t *testing.T) {
		sql, _, err := qb.BuildSQL(goalLeaf(&domain.CustomEventsGoalCondition{
			GoalType: "purchase", TimeframeOperator: "anytime", AggregateOperator: "count", Operator: "gte", Value: 1, Negate: true,
		}))
		require.NoError(t, err)
		assert.Contains(t, sql, "NOT EXISTS (SELECT 1 FROM custom_events",
			"contacts with zero matching events must match a negated goal")
	})

	t.Run("goal name and event name filters take placeholders in field order", func(t *testing.T) {
		name, event := "VIP", "shopify.order"
		sql, args, err := qb.BuildSQL(goalLeaf(&domain.CustomEventsGoalCondition{
			GoalType: "purchase", TimeframeOperator: "anytime", GoalName: &name, EventName: &event,
			AggregateOperator: "count", Operator: "eq", Value: 2,
		}))
		require.NoError(t, err)
		assert.Contains(t, sql, "ce.goal_type = $1")
		assert.Contains(t, sql, "ce.goal_name = $2")
		assert.Contains(t, sql, "ce.event_name = $3")
		assert.Contains(t, sql, "COUNT(*) = $4")
		assert.Equal(t, []interface{}{"purchase", "VIP", "shopify.order", 2.0}, args)
	})
}

func TestBuildTriggerCondition_Basics(t *testing.T) {
	qb := NewQueryBuilder()

	t.Run("nil tree means no filter", func(t *testing.T) {
		cond, args, err := qb.BuildTriggerCondition(nil, "NEW.email")
		require.NoError(t, err)
		assert.Equal(t, "", cond)
		assert.Nil(t, args)
	})

	t.Run("the email reference is substituted into the condition", func(t *testing.T) {
		tree := &domain.TreeNode{
			Kind: "leaf",
			Leaf: &domain.TreeNodeLeaf{Source: "contacts", Contact: &domain.ContactCondition{
				Filters: []*domain.DimensionFilter{
					{FieldName: "country", FieldType: "string", Operator: "equals", StringValues: []string{"FR"}},
				},
			}},
		}
		cond, args, err := qb.BuildTriggerCondition(tree, "NEW.email")
		require.NoError(t, err)
		assert.Contains(t, cond, "NEW.email")
		assert.Equal(t, []interface{}{"FR"}, args)
	})
}
