package domain

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func validGoal() *CustomEventsGoalCondition {
	return &CustomEventsGoalCondition{
		GoalType:          GoalTypePurchase,
		AggregateOperator: "sum",
		Operator:          "gte",
		Value:             100,
		TimeframeOperator: "anytime",
	}
}

func goalLeaf(goal *CustomEventsGoalCondition) *TreeNode {
	return &TreeNode{Kind: "leaf", Leaf: &TreeNodeLeaf{Source: "custom_events_goals", CustomEventsGoal: goal}}
}

func timelineLeaf(timeline *ContactTimelineCondition) *TreeNode {
	return &TreeNode{Kind: "leaf", Leaf: &TreeNodeLeaf{Source: "contact_timeline", ContactTimeline: timeline}}
}

func validTimeline() *ContactTimelineCondition {
	return &ContactTimelineCondition{Kind: "email.opened", CountOperator: "at_least", CountValue: 1}
}

func stringFilter() *DimensionFilter {
	return &DimensionFilter{FieldName: "email", FieldType: "string", Operator: "equals", StringValues: []string{"a@example.com"}}
}

// A segment tree that does not validate is refused before it is compiled to SQL.
func TestTreeNode_Validate_Refusals(t *testing.T) {
	strPtr := func(s string) *string { return &s }

	cases := []struct {
		name    string
		tree    *TreeNode
		wantErr string
	}{
		{
			name:    "nil leaf in a branch",
			tree:    &TreeNode{Kind: "branch", Branch: &TreeNodeBranch{Operator: "and", Leaves: []*TreeNode{nil}}},
			wantErr: "branch leaf 0 is nil",
		},
		{
			name:    "contact_lists leaf without its condition",
			tree:    &TreeNode{Kind: "leaf", Leaf: &TreeNodeLeaf{Source: "contact_lists"}},
			wantErr: "must have 'contact_list' field",
		},
		{
			name:    "contact_timeline leaf without its condition",
			tree:    &TreeNode{Kind: "leaf", Leaf: &TreeNodeLeaf{Source: "contact_timeline"}},
			wantErr: "must have 'contact_timeline' field",
		},
		{
			name:    "contact_timeline leaf with an invalid condition",
			tree:    timelineLeaf(&ContactTimelineCondition{CountOperator: "at_least"}),
			wantErr: "contact_timeline condition must have 'kind'",
		},
		{
			name:    "custom_events_goals leaf without its condition",
			tree:    &TreeNode{Kind: "leaf", Leaf: &TreeNodeLeaf{Source: "custom_events_goals"}},
			wantErr: "must have 'custom_events_goal' field",
		},
		{
			name:    "contacts leaf without filters",
			tree:    &TreeNode{Kind: "leaf", Leaf: &TreeNodeLeaf{Source: "contacts", Contact: &ContactCondition{}}},
			wantErr: "contact condition must have at least one filter",
		},
		{
			name: "contacts leaf with a nil filter",
			tree: &TreeNode{Kind: "leaf", Leaf: &TreeNodeLeaf{Source: "contacts",
				Contact: &ContactCondition{Filters: []*DimensionFilter{nil}}}},
			wantErr: "filter 0 is nil",
		},
		{
			name: "timeline with an unknown timeframe",
			tree: func() *TreeNode {
				c := validTimeline()
				c.TimeframeOperator = strPtr("next_week")
				return timelineLeaf(c)
			}(),
			wantErr: "invalid timeframe_operator: next_week",
		},
		{
			name: "timeline with a nil filter",
			tree: func() *TreeNode {
				c := validTimeline()
				c.Filters = []*DimensionFilter{nil}
				return timelineLeaf(c)
			}(),
			wantErr: "filter 0 is nil",
		},
		{
			name: "timeline with an invalid filter",
			tree: func() *TreeNode {
				c := validTimeline()
				c.Filters = []*DimensionFilter{stringFilter(), {FieldType: "string"}}
				return timelineLeaf(c)
			}(),
			wantErr: "filter 1: filter must have 'field_name'",
		},
		{
			name:    "goal without a type",
			tree:    goalLeaf(&CustomEventsGoalCondition{}),
			wantErr: "must have 'goal_type'",
		},
		{
			name: "goal with an unknown type",
			tree: func() *TreeNode {
				g := validGoal()
				g.GoalType = "donation"
				return goalLeaf(g)
			}(),
			wantErr: "invalid goal_type: donation",
		},
		{
			name: "goal with an unknown aggregate",
			tree: func() *TreeNode {
				g := validGoal()
				g.AggregateOperator = "median"
				return goalLeaf(g)
			}(),
			wantErr: "invalid aggregate_operator: median",
		},
		{
			name: "goal with an unknown operator",
			tree: func() *TreeNode {
				g := validGoal()
				g.Operator = "gt"
				return goalLeaf(g)
			}(),
			wantErr: "invalid operator: gt",
		},
		{
			name: "goal between without its upper bound",
			tree: func() *TreeNode {
				g := validGoal()
				g.Operator = "between"
				return goalLeaf(g)
			}(),
			wantErr: "between operator requires 'value_2'",
		},
		{
			name: "goal with an unknown timeframe",
			tree: func() *TreeNode {
				g := validGoal()
				g.TimeframeOperator = "forever"
				return goalLeaf(g)
			}(),
			wantErr: "invalid timeframe_operator: forever",
		},
		{
			name: "goal timeframe without values",
			tree: func() *TreeNode {
				g := validGoal()
				g.TimeframeOperator = "in_the_last_days"
				return goalLeaf(g)
			}(),
			wantErr: "timeframe_values required for timeframe_operator 'in_the_last_days'",
		},
		{
			name: "goal with a nil filter",
			tree: func() *TreeNode {
				g := validGoal()
				g.Filters = []*DimensionFilter{nil}
				return goalLeaf(g)
			}(),
			wantErr: "filter 0 is nil",
		},
		{
			name: "goal with an invalid filter",
			tree: func() *TreeNode {
				g := validGoal()
				g.Filters = []*DimensionFilter{{FieldName: "amount", FieldType: "number", Operator: "gte"}}
				return goalLeaf(g)
			}(),
			wantErr: "filter 0: number filter must have 'number_values'",
		},
		{
			name: "in_array filter without values",
			tree: &TreeNode{Kind: "leaf", Leaf: &TreeNodeLeaf{Source: "contacts", Contact: &ContactCondition{
				Filters: []*DimensionFilter{{FieldName: "custom_json_1", FieldType: "string", Operator: "in_array", JSONPath: []string{"tags"}}},
			}}},
			wantErr: "in_array operator must have 'string_values'",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.tree.Validate()

			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

func TestTreeNode_Validate_AcceptsGoalAndTimelineLeaves(t *testing.T) {
	between := validGoal()
	between.Operator = "between"
	upper := 500.0
	between.Value2 = &upper
	between.TimeframeOperator = "in_the_last_days"
	between.TimeframeValues = []string{"30"}
	between.Filters = []*DimensionFilter{stringFilter()}

	anyGoal := validGoal()
	anyGoal.GoalType = "*"

	timeline := validTimeline()
	lastDays := "in_the_last_days"
	timeline.TimeframeOperator = &lastDays
	timeline.TimeframeValues = []string{"7"}
	assert.NoError(t, timelineLeaf(timeline).Validate(), "a timeline over the last days")

	for name, goal := range map[string]*CustomEventsGoalCondition{"between, last 30 days, filtered": between, "any goal type": anyGoal} {
		t.Run(name, func(t *testing.T) {
			assert.NoError(t, goalLeaf(goal).Validate())
		})
	}
}

// A goal counted over the last days moves with the clock, so the segment is rebuilt daily.
func TestTreeNode_HasRelativeDates_GoalLeaves(t *testing.T) {
	recent := validGoal()
	recent.TimeframeOperator = "in_the_last_days"
	recent.TimeframeValues = []string{"7"}

	assert.True(t, goalLeaf(recent).HasRelativeDates())
	assert.False(t, goalLeaf(validGoal()).HasRelativeDates())
	assert.False(t, (&TreeNode{Kind: "unknown"}).HasRelativeDates())
}

func TestTreeNodeFromMapOfAny_RefusesAWrongShape(t *testing.T) {
	_, err := TreeNodeFromMapOfAny(MapOfAny{"kind": 5})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to unmarshal tree node")
}

// A value JSON cannot hold is reported, not turned into a broken document.
func TestTreeNode_ToMapOfAny_RefusesNaN(t *testing.T) {
	goal := validGoal()
	goal.Value = math.NaN()

	_, err := goalLeaf(goal).ToMapOfAny()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to marshal tree node")
}
