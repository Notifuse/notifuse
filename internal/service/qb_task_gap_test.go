package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Gap coverage for query_builder.go and task_service.go branches left
// uncovered by the existing suites. QueryBuilder expectations pin the exact
// SQL text, args and next placeholder index together; TaskService
// expectations pin exact errors and use strict gomock controllers so a
// repository call that must not happen fails the test.

func TestGapQB_ParseNodeGuards(t *testing.T) {
	qb := NewQueryBuilder()

	t.Run("parseNode rejects an unknown kind by name", func(t *testing.T) {
		cond, args, next, err := qb.parseNode(&domain.TreeNode{Kind: "sideways"}, 4)
		require.EqualError(t, err, "invalid node kind: sideways")
		assert.Equal(t, "", cond)
		assert.Nil(t, args)
		assert.Equal(t, 4, next, "index must not advance on error")
	})

	t.Run("parseBranch propagates a child error with no partial SQL", func(t *testing.T) {
		branch := &domain.TreeNodeBranch{
			Operator: "and",
			Leaves:   []*domain.TreeNode{{Kind: "sideways"}},
		}
		cond, args, _, err := qb.parseBranch(branch, 1)
		require.EqualError(t, err, "invalid node kind: sideways")
		assert.Equal(t, "", cond, "a failed child must not leak a partial condition")
		assert.Nil(t, args)
	})

	t.Run("empty contact conditions compile to no condition", func(t *testing.T) {
		// Reachable only below BuildSQL: domain validation requires at least
		// one filter, so this pins the builder's own degenerate contract.
		cond, args, next, err := qb.parseContactConditions(&domain.ContactCondition{}, 7)
		require.NoError(t, err)
		assert.Equal(t, "", cond)
		assert.Nil(t, args)
		assert.Equal(t, 7, next)
	})
}

func TestGapQB_ParseFilterGuardsAndDefaults(t *testing.T) {
	qb := NewQueryBuilder()

	t.Run("nil filter and unknown field are named errors", func(t *testing.T) {
		_, _, _, err := qb.parseFilter(nil, 1, castDirectly)
		require.EqualError(t, err, "filter cannot be nil")

		_, _, _, err = qb.parseFilter(&domain.DimensionFilter{
			FieldName: "nickname", FieldType: "string", Operator: "equals",
			StringValues: []string{"x"},
		}, 1, castDirectly)
		require.EqualError(t, err, "invalid field name: nickname")
	})

	t.Run("unknown field type is rejected after the whitelist lookup", func(t *testing.T) {
		_, _, _, err := qb.parseFilter(&domain.DimensionFilter{
			FieldName: "country", FieldType: "boolean", Operator: "equals",
			StringValues: []string{"FR"},
		}, 1, castDirectly)
		require.EqualError(t, err, "invalid field type: boolean")
	})

	t.Run("missing values fail per field type with exact messages", func(t *testing.T) {
		_, _, _, err := qb.parseFilter(&domain.DimensionFilter{
			FieldName: "country", FieldType: "string", Operator: "equals",
		}, 1, castDirectly)
		require.EqualError(t, err, "string filter must have 'string_values'")

		_, _, _, err = qb.parseFilter(&domain.DimensionFilter{
			FieldName: "custom_number_1", FieldType: "number", Operator: "gt",
		}, 1, castDirectly)
		require.EqualError(t, err, "number filter must have 'number_values'")

		_, _, _, err = qb.parseFilter(&domain.DimensionFilter{
			FieldName: "created_at", FieldType: "time", Operator: "equals",
		}, 1, castDirectly)
		require.EqualError(t, err, "time filter must have 'string_values' (ISO8601 dates)")
	})

	t.Run("unparseable time value names the accepted formats", func(t *testing.T) {
		_, _, _, err := qb.parseFilter(&domain.DimensionFilter{
			FieldName: "created_at", FieldType: "time", Operator: "equals",
			StringValues: []string{"next week"},
		}, 1, castDirectly)
		require.EqualError(t, err, "invalid time value: next week (expected ISO8601 or YYYY-MM-DD)")
	})

	t.Run("empty field type falls back to the whitelist type", func(t *testing.T) {
		cond, args, next, err := qb.parseFilter(&domain.DimensionFilter{
			FieldName: "custom_number_2", Operator: "lt",
			NumberValues: []float64{3.5},
		}, 2, castDirectly)
		require.NoError(t, err)
		assert.Equal(t, "custom_number_2 < $2", cond)
		assert.Equal(t, []interface{}{3.5}, args)
		assert.Equal(t, 3, next)

		cond, args, next, err = qb.parseFilter(&domain.DimensionFilter{
			FieldName: "created_at", Operator: "gt",
			StringValues: []string{"2026-01-02T03:04:05Z"},
		}, 1, castDirectly)
		require.NoError(t, err)
		assert.Equal(t, "created_at > $1", cond)
		assert.Equal(t, []interface{}{time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)}, args)
		assert.Equal(t, 2, next)
	})

	t.Run("relative-day operator on a numeric field is refused", func(t *testing.T) {
		_, _, _, err := qb.parseFilter(&domain.DimensionFilter{
			FieldName: "custom_number_1", FieldType: "number", Operator: "in_the_last_days",
			StringValues: []string{"5"},
		}, 1, castDirectly)
		require.EqualError(t, err,
			"operator in_the_last_days can only be used with a date field, not custom_number_1")
	})
}

func TestGapQB_ValueExtractors(t *testing.T) {
	qb := NewQueryBuilder()

	t.Run("empty value lists are exact errors", func(t *testing.T) {
		_, err := qb.getStringValues(&domain.DimensionFilter{})
		require.EqualError(t, err, "string filter must have 'string_values'")
		_, err = qb.getNumberValues(&domain.DimensionFilter{})
		require.EqualError(t, err, "number filter must have 'number_values'")
		_, err = qb.getTimeValues(&domain.DimensionFilter{})
		require.EqualError(t, err, "time filter must have 'string_values' (ISO8601 dates)")
	})

	t.Run("date-only time values fall back to midnight UTC", func(t *testing.T) {
		values, err := qb.getTimeValues(&domain.DimensionFilter{
			StringValues: []string{"2026-02-03"},
		})
		require.NoError(t, err)
		assert.Equal(t, []interface{}{time.Date(2026, 2, 3, 0, 0, 0, 0, time.UTC)}, values)
	})
}

func TestGapQB_BuildConditionBoundaries(t *testing.T) {
	qb := NewQueryBuilder()
	contains := qb.allowedOperators["contains"]
	equals := qb.allowedOperators["equals"]
	between := qb.allowedOperators["in_date_range"]
	lastDays := qb.allowedOperators["in_the_last_days"]
	notLastDays := qb.allowedOperators["not_in_the_last_days"]

	t.Run("contains requires at least one string value", func(t *testing.T) {
		_, _, _, err := qb.buildCondition("email", "contains", contains, nil, 1)
		require.EqualError(t, err, "contains/not_contains requires at least one value")

		_, _, _, err = qb.buildCondition("email", "contains", contains, []interface{}{123}, 1)
		require.EqualError(t, err, "contains/not_contains requires string value")

		_, _, _, err = qb.buildCondition("email", "contains", contains,
			[]interface{}{"ok", 42}, 1)
		require.EqualError(t, err, "contains/not_contains requires string values")
	})

	t.Run("date range requires exactly two values", func(t *testing.T) {
		_, _, _, err := qb.buildCondition("created_at", "not_in_date_range", between,
			[]interface{}{time.Now()}, 1)
		require.EqualError(t, err, "not_in_date_range requires exactly 2 values (start and end)")
	})

	t.Run("standard operators require exactly one value", func(t *testing.T) {
		_, _, _, err := qb.buildCondition("country", "equals", equals, nil, 1)
		require.EqualError(t, err, "equals requires exactly one value")
		_, _, _, err = qb.buildCondition("country", "equals", equals,
			[]interface{}{"FR", "DE"}, 1)
		require.EqualError(t, err, "equals requires exactly one value")
	})

	t.Run("last-days requires one value and rejects bad day types", func(t *testing.T) {
		_, _, _, err := qb.buildCondition("created_at", "in_the_last_days", lastDays,
			[]interface{}{"1", "2"}, 1)
		require.EqualError(t, err, "in_the_last_days requires 1 value")

		_, _, _, err = qb.buildCondition("created_at", "in_the_last_days", lastDays,
			[]interface{}{true}, 1)
		require.EqualError(t, err, "invalid days value type")

		_, _, _, err = qb.buildCondition("created_at", "in_the_last_days", lastDays,
			[]interface{}{"abc"}, 1)
		require.ErrorContains(t, err, "invalid days value:")
	})

	t.Run("int and float day counts inline only the parsed integer", func(t *testing.T) {
		// Direct callers can hand buildCondition typed values; float days
		// truncate toward zero and never reach the SQL as raw text.
		cond, args, next, err := qb.buildCondition("created_at", "in_the_last_days", lastDays,
			[]interface{}{7}, 3)
		require.NoError(t, err)
		assert.Equal(t, "created_at > NOW() - INTERVAL '7 days'", cond)
		assert.Empty(t, args)
		assert.Equal(t, 3, next, "an inlined interval consumes no placeholder")

		cond, _, _, err = qb.buildCondition("created_at", "in_the_last_days", lastDays,
			[]interface{}{7.9}, 1)
		require.NoError(t, err)
		assert.Equal(t, "created_at > NOW() - INTERVAL '7 days'", cond)
	})

	t.Run("not-in-the-last-days keeps unset dates via the NULL disjunct", func(t *testing.T) {
		cond, args, next, err := qb.buildCondition("created_at", "not_in_the_last_days", notLastDays,
			[]interface{}{30}, 2)
		require.NoError(t, err)
		assert.Equal(t, "(created_at IS NULL OR created_at <= NOW() - INTERVAL '30 days')", cond)
		assert.Empty(t, args)
		assert.Equal(t, 2, next)
	})
}

func TestGapQB_BuildJSONConditionBranches(t *testing.T) {
	qb := NewQueryBuilder()

	t.Run("unknown JSON operator is named as a JSON error", func(t *testing.T) {
		_, _, _, err := qb.buildJSONCondition("custom_json_1", &domain.DimensionFilter{
			FieldName: "custom_json_1", FieldType: "string", Operator: "approx",
			StringValues: []string{"x"},
		}, 1, castDirectly)
		require.EqualError(t, err, "invalid operator for JSON field: approx")
	})

	t.Run("existence on the whole column needs no path and no args", func(t *testing.T) {
		cond, args, next, err := qb.buildJSONCondition("custom_json_1", &domain.DimensionFilter{
			FieldName: "custom_json_1", FieldType: "string", Operator: "is_set",
		}, 4, castDirectly)
		require.NoError(t, err)
		assert.Equal(t, "custom_json_1 IS NOT NULL", cond)
		assert.Nil(t, args)
		assert.Equal(t, 4, next)

		cond, args, next, err = qb.buildJSONCondition("custom_json_1", &domain.DimensionFilter{
			FieldName: "custom_json_1", FieldType: "string", Operator: "is_not_set",
		}, 4, castDirectly)
		require.NoError(t, err)
		assert.Equal(t, "custom_json_1 IS NULL", cond)
		assert.Nil(t, args)
		assert.Equal(t, 4, next)
	})

	t.Run("in_array without values and unknown JSON field types are refused", func(t *testing.T) {
		_, _, _, err := qb.buildJSONCondition("custom_json_1", &domain.DimensionFilter{
			FieldName: "custom_json_1", FieldType: "string", Operator: "in_array",
		}, 1, castDirectly)
		require.EqualError(t, err, "in_array requires string_values")

		_, _, _, err = qb.buildJSONCondition("custom_json_1", &domain.DimensionFilter{
			FieldName: "custom_json_1", FieldType: "boolean", Operator: "equals",
			StringValues: []string{"x"},
		}, 1, castDirectly)
		require.EqualError(t, err, "invalid field_type for JSON field: boolean")
	})

	t.Run("JSON equality with two values is refused by arity", func(t *testing.T) {
		_, _, _, err := qb.buildJSONCondition("custom_json_1", &domain.DimensionFilter{
			FieldName: "custom_json_1", FieldType: "string", Operator: "equals",
			StringValues: []string{"a", "b"}, JSONPath: []string{"tier"},
		}, 1, castDirectly)
		require.EqualError(t, err, "equals requires exactly one value")
	})

	t.Run("multi-value JSON contains ORs ILIKE arms in placeholder order", func(t *testing.T) {
		cond, args, next, err := qb.buildJSONCondition("custom_json_1", &domain.DimensionFilter{
			FieldName: "custom_json_1", FieldType: "string", Operator: "contains",
			StringValues: []string{"gold", "silver"}, JSONPath: []string{"tier"},
		}, 1, castDirectly)
		require.NoError(t, err)
		assert.Equal(t,
			"(custom_json_1['tier']::text ILIKE $1 OR custom_json_1['tier']::text ILIKE $2)", cond)
		assert.Equal(t, []interface{}{"%gold%", "%silver%"}, args)
		assert.Equal(t, 3, next)
	})

	t.Run("single JSON not_contains uses NOT ILIKE", func(t *testing.T) {
		cond, args, next, err := qb.buildJSONCondition("custom_json_1", &domain.DimensionFilter{
			FieldName: "custom_json_1", FieldType: "string", Operator: "not_contains",
			StringValues: []string{"trial"}, JSONPath: []string{"tier"},
		}, 2, castDirectly)
		require.NoError(t, err)
		assert.Equal(t, "custom_json_1['tier']::text NOT ILIKE $2", cond)
		assert.Equal(t, []interface{}{"%trial%"}, args)
		assert.Equal(t, 3, next)
	})

	t.Run("whole-column numeric comparison is guarded against stored scalars", func(t *testing.T) {
		cond, args, next, err := qb.buildJSONCondition("custom_json_1", &domain.DimensionFilter{
			FieldName: "custom_json_1", FieldType: "number", Operator: "gt",
			NumberValues: []float64{5},
		}, 1, castDirectly)
		require.NoError(t, err)
		assert.Equal(t,
			"CASE WHEN jsonb_typeof(custom_json_1) = 'object' THEN (custom_json_1::text)::numeric END > $1",
			cond)
		assert.Equal(t, []interface{}{5.0}, args)
		assert.Equal(t, 2, next)
	})

	t.Run("JSON relative-day operators demand a time value", func(t *testing.T) {
		_, _, _, err := qb.buildJSONCondition("custom_json_1", &domain.DimensionFilter{
			FieldName: "custom_json_1", FieldType: "string", Operator: "in_the_last_days",
			StringValues: []string{"5"}, JSONPath: []string{"at"},
		}, 1, castDirectly)
		require.EqualError(t, err,
			"operator in_the_last_days can only be used with a date value, not string")

		_, _, _, err = qb.buildJSONCondition("custom_json_1", &domain.DimensionFilter{
			FieldName: "custom_json_1", FieldType: "time", Operator: "in_the_last_days",
			JSONPath: []string{"at"},
		}, 1, castDirectly)
		require.EqualError(t, err, "string filter must have 'string_values'")
	})

	t.Run("JSONB key filters cast time and defensive numbers exactly", func(t *testing.T) {
		cond, args, next, err := qb.parseJSONBKeyFilter(&domain.DimensionFilter{
			FieldName: "seen_at", FieldType: "time", Operator: "gt",
			StringValues: []string{"2026-01-02T03:04:05Z"},
		}, 1, "ce.properties->>%s", castDirectly)
		require.NoError(t, err)
		assert.Equal(t, "(ce.properties->>$1)::timestamptz > $2", cond)
		assert.Equal(t, []interface{}{"seen_at", time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)}, args)
		assert.Equal(t, 3, next)

		cond, args, next, err = qb.parseJSONBKeyFilter(&domain.DimensionFilter{
			FieldName: "total", FieldType: "number", Operator: "gt",
			NumberValues: []float64{100},
		}, 1, "ce.properties->>%s", castDefensively)
		require.NoError(t, err)
		assert.Equal(t, `CASE WHEN ce.properties->>$1 ~ '^\s*[-+]?(\d+(\.\d*)?|\.\d+)([eE][-+]?\d+)?\s*$' THEN (ce.properties->>$1)::numeric END > $2`, cond)
		assert.Equal(t, []interface{}{"total", 100.0}, args)
		assert.Equal(t, 3, next)
	})
}

func TestGapQB_IsNumericBoundaries(t *testing.T) {
	qb := NewQueryBuilder()
	cases := []struct {
		in   string
		want bool
	}{
		{"", false}, // empty string must not become an array subscript
		{"0", true},
		{"007", true},
		{"123", true},
		{"12.0", false},
		{"-3", false},
		{"3 ", false},
		{"a1", false},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, qb.isNumeric(tc.in), "isNumeric(%q)", tc.in)
	}
}

func TestGapQB_TriggerLeafGuards(t *testing.T) {
	qb := NewQueryBuilder()

	t.Run("trigger node and branch guards mirror the segment ones", func(t *testing.T) {
		_, _, _, err := qb.parseNodeWithEmailRef(&domain.TreeNode{Kind: "sideways"}, 1, "NEW.email")
		require.EqualError(t, err, "invalid node kind: sideways")

		_, _, _, err = qb.parseBranchWithEmailRef(nil, 1, "NEW.email")
		require.EqualError(t, err, "branch cannot be nil")

		cond, args, next, err := qb.parseBranchWithEmailRef(
			&domain.TreeNodeBranch{Operator: "and"}, 5, "NEW.email")
		require.NoError(t, err)
		assert.Equal(t, "", cond)
		assert.Empty(t, args)
		assert.Equal(t, 5, next)
	})

	t.Run("every trigger leaf payload is required by name", func(t *testing.T) {
		_, _, _, err := qb.parseLeafWithEmailRef(nil, 1, "NEW.email")
		require.EqualError(t, err, "leaf cannot be nil")

		cases := []struct {
			leaf *domain.TreeNodeLeaf
			want string
		}{
			{&domain.TreeNodeLeaf{Source: "contacts"},
				"leaf with source 'contacts' must have 'contact' field"},
			{&domain.TreeNodeLeaf{Source: "contact_lists"},
				"leaf with source 'contact_lists' must have 'contact_list' field"},
			{&domain.TreeNodeLeaf{Source: "contact_timeline"},
				"leaf with source 'contact_timeline' must have 'contact_timeline' field"},
			{&domain.TreeNodeLeaf{Source: "custom_events_goals"},
				"leaf with source 'custom_events_goals' must have 'custom_events_goal' field"},
		}
		for _, tc := range cases {
			_, _, _, err := qb.parseLeafWithEmailRef(tc.leaf, 1, "NEW.email")
			assert.EqualError(t, err, tc.want)
		}
	})

	t.Run("trigger unsupported source does not list supported sources", func(t *testing.T) {
		// Pins the deliberate message difference from parseLeaf, whose error
		// enumerates the supported sources.
		_, _, _, err := qb.parseLeafWithEmailRef(
			&domain.TreeNodeLeaf{Source: "orders"}, 1, "NEW.email")
		require.EqualError(t, err, "unsupported source: orders")
	})

	t.Run("trigger branches OR email-scoped EXISTS arms in order", func(t *testing.T) {
		leaf := func(country string) *domain.TreeNode {
			return &domain.TreeNode{Kind: "leaf", Leaf: &domain.TreeNodeLeaf{
				Source: "contacts",
				Contact: &domain.ContactCondition{Filters: []*domain.DimensionFilter{
					{FieldName: "country", FieldType: "string", Operator: "equals",
						StringValues: []string{country}},
				}},
			}}
		}
		cond, args, next, err := qb.parseBranchWithEmailRef(
			&domain.TreeNodeBranch{Operator: "or", Leaves: []*domain.TreeNode{leaf("FR"), leaf("DE")}},
			1, "NEW.email")
		require.NoError(t, err)
		assert.Equal(t,
			"(EXISTS (SELECT 1 FROM contacts WHERE email = NEW.email AND country = $1)"+
				" OR EXISTS (SELECT 1 FROM contacts WHERE email = NEW.email AND country = $2))", cond)
		assert.Equal(t, []interface{}{"FR", "DE"}, args)
		assert.Equal(t, 3, next)
	})

	t.Run("trigger contact conditions with no filters only require existence", func(t *testing.T) {
		_, _, _, err := qb.parseContactConditionsForTrigger(nil, 1, "NEW.email")
		require.EqualError(t, err, "contact condition cannot be nil")

		cond, args, next, err := qb.parseContactConditionsForTrigger(
			&domain.ContactCondition{}, 2, "NEW.email")
		require.NoError(t, err)
		assert.Equal(t, "EXISTS (SELECT 1 FROM contacts WHERE email = NEW.email)", cond)
		assert.Empty(t, args)
		assert.Equal(t, 2, next)

		_, _, _, err = qb.parseContactConditionsForTrigger(
			&domain.ContactCondition{Filters: []*domain.DimensionFilter{
				{FieldName: "nickname", FieldType: "string", Operator: "equals",
					StringValues: []string{"x"}},
			}}, 1, "NEW.email")
		require.EqualError(t, err, "invalid field name: nickname")
	})

	t.Run("trigger build surfaces a timeframe failure from the goal parser", func(t *testing.T) {
		tree := &domain.TreeNode{Kind: "leaf", Leaf: &domain.TreeNodeLeaf{
			Source: "custom_events_goals",
			CustomEventsGoal: &domain.CustomEventsGoalCondition{
				GoalType: "purchase", AggregateOperator: "sum", Operator: "gte", Value: 1,
				TimeframeOperator: "in_the_last_days", TimeframeValues: []string{"abc"},
			},
		}}
		_, _, err := qb.BuildTriggerCondition(tree, "NEW.email")
		require.ErrorContains(t, err, "invalid days value:")
	})
}

func TestGapQB_TriggerListTimelineGoalGuards(t *testing.T) {
	qb := NewQueryBuilder()

	t.Run("contact list trigger guards and NOT IN shape", func(t *testing.T) {
		_, _, _, err := qb.parseContactListConditionsWithEmailRef(nil, 1, "NEW.email")
		require.EqualError(t, err, "contact_list condition cannot be nil")

		_, _, _, err = qb.parseContactListConditionsWithEmailRef(
			&domain.ContactListCondition{Operator: "in"}, 1, "NEW.email")
		require.EqualError(t, err, "contact_list must have 'list_id'")

		_, _, _, err = qb.parseContactListConditionsWithEmailRef(
			&domain.ContactListCondition{Operator: "maybe", ListID: "list-9"}, 1, "NEW.email")
		require.EqualError(t, err, "invalid contact_list operator: maybe (must be 'in' or 'not_in')")

		status := "active"
		cond, args, next, err := qb.parseContactListConditionsWithEmailRef(
			&domain.ContactListCondition{Operator: "not_in", ListID: "list-9", Status: &status},
			1, "NEW.email")
		require.NoError(t, err)
		assert.Equal(t, "NOT EXISTS (SELECT 1 FROM contact_lists cl JOIN lists l ON cl.list_id = l.id "+
			"WHERE cl.email = NEW.email AND cl.list_id = $1 AND cl.status = $2 "+
			"AND cl.deleted_at IS NULL AND l.deleted_at IS NULL)", cond)
		assert.Equal(t, []interface{}{"list-9", "active"}, args)
		assert.Equal(t, 3, next)
	})

	t.Run("timeline trigger guards reject missing and bad operators", func(t *testing.T) {
		_, _, _, err := qb.parseContactTimelineConditionsWithEmailRef(nil, 1, "NEW.email")
		require.EqualError(t, err, "contact_timeline condition cannot be nil")

		_, _, _, err = qb.parseContactTimelineConditionsWithEmailRef(
			&domain.ContactTimelineCondition{CountOperator: "at_least"}, 1, "NEW.email")
		require.EqualError(t, err, "contact_timeline must have 'kind'")

		_, _, _, err = qb.parseContactTimelineConditionsWithEmailRef(
			&domain.ContactTimelineCondition{Kind: "email.sent"}, 1, "NEW.email")
		require.EqualError(t, err, "contact_timeline must have 'count_operator'")

		_, _, _, err = qb.parseContactTimelineConditionsWithEmailRef(
			&domain.ContactTimelineCondition{Kind: "email.sent", CountOperator: "many", CountValue: 1},
			1, "NEW.email")
		require.EqualError(t, err,
			"invalid count_operator: many (must be 'at_least', 'at_most', or 'exactly')")
	})

	t.Run("timeline trigger propagates timeframe and filter failures", func(t *testing.T) {
		op := "before_date"
		_, _, _, err := qb.parseContactTimelineConditionsWithEmailRef(
			&domain.ContactTimelineCondition{
				Kind: "email.opened", CountOperator: "at_least", CountValue: 1,
				TimeframeOperator: &op, TimeframeValues: []string{"garbage"},
			}, 1, "NEW.email")
		require.ErrorContains(t, err, "invalid time:")

		_, _, _, err = qb.parseContactTimelineConditionsWithEmailRef(
			&domain.ContactTimelineCondition{
				Kind: "email.opened", CountOperator: "at_least", CountValue: 1,
				Filters: []*domain.DimensionFilter{
					{FieldName: "", FieldType: "string", Operator: "equals",
						StringValues: []string{"x"}},
				},
			}, 1, "NEW.email")
		require.EqualError(t, err, "filter must have 'field_name'")
	})

	t.Run("timeline at_most and template scoping compile exactly", func(t *testing.T) {
		cond, args, next, err := qb.parseContactTimelineConditionsWithEmailRef(
			&domain.ContactTimelineCondition{Kind: "email.sent", CountOperator: "at_most", CountValue: 3},
			1, "NEW.email")
		require.NoError(t, err)
		assert.Equal(t, "(SELECT COUNT(*) FROM contact_timeline ct WHERE ct.email = NEW.email "+
			"AND ct.kind = $1) <= $2", cond)
		assert.Equal(t, []interface{}{"email.sent", 3}, args)
		assert.Equal(t, 3, next)

		tpl := "tpl-1"
		cond, args, next, err = qb.parseContactTimelineConditionsWithEmailRef(
			&domain.ContactTimelineCondition{
				Kind: "email.opened", CountOperator: "at_least", CountValue: 1, TemplateID: &tpl,
			}, 1, "NEW.email")
		require.NoError(t, err)
		assert.Equal(t, "(SELECT COUNT(*) FROM contact_timeline ct WHERE ct.email = NEW.email "+
			"AND ct.kind = $1 AND ct.entity_id IN (SELECT id FROM message_history WHERE template_id = $2)) >= $3", cond)
		assert.Equal(t, []interface{}{"email.opened", "tpl-1", 1}, args)
		assert.Equal(t, 4, next)
	})

	t.Run("goal aggregates avg min max and lte compile exactly", func(t *testing.T) {
		goal := func(agg, op string, value float64) *domain.CustomEventsGoalCondition {
			return &domain.CustomEventsGoalCondition{
				GoalType: "purchase", AggregateOperator: agg, Operator: op, Value: value,
			}
		}
		cond, args, next, err := qb.parseCustomEventsGoalConditionWithEmailRef(
			goal("avg", "gte", 250), 1, "NEW.email")
		require.NoError(t, err)
		assert.Equal(t, "EXISTS (SELECT 1 FROM custom_events ce WHERE ce.email = NEW.email "+
			"AND ce.deleted_at IS NULL AND ce.goal_type = $1 GROUP BY ce.email "+
			"HAVING COALESCE(AVG(ce.goal_value), 0) >= $2)", cond)
		assert.Equal(t, []interface{}{"purchase", 250.0}, args)
		assert.Equal(t, 3, next)

		cond, _, _, err = qb.parseCustomEventsGoalConditionWithEmailRef(goal("min", "lte", 5), 1, "NEW.email")
		require.NoError(t, err)
		assert.Contains(t, cond, "HAVING MIN(ce.goal_value) <= $2")

		cond, _, _, err = qb.parseCustomEventsGoalConditionWithEmailRef(goal("max", "eq", 9), 1, "NEW.email")
		require.NoError(t, err)
		assert.Contains(t, cond, "HAVING MAX(ce.goal_value) = $2")
	})

	t.Run("goal trigger guards name each bad enum", func(t *testing.T) {
		_, _, _, err := qb.parseCustomEventsGoalConditionWithEmailRef(nil, 1, "NEW.email")
		require.EqualError(t, err, "custom_events_goal condition cannot be nil")

		_, _, _, err = qb.parseCustomEventsGoalConditionWithEmailRef(
			&domain.CustomEventsGoalCondition{
				GoalType: "donation", AggregateOperator: "sum", Operator: "gte", Value: 1,
			}, 1, "NEW.email")
		require.EqualError(t, err, "invalid goal_type: donation (must be one of: "+
			"[purchase subscription lead signup booking trial other] or '*' for all)")

		_, _, _, err = qb.parseCustomEventsGoalConditionWithEmailRef(
			&domain.CustomEventsGoalCondition{
				GoalType: "purchase", AggregateOperator: "median", Operator: "gte", Value: 1,
			}, 1, "NEW.email")
		require.EqualError(t, err, "invalid aggregate_operator: median")

		_, _, _, err = qb.parseCustomEventsGoalConditionWithEmailRef(
			&domain.CustomEventsGoalCondition{
				GoalType: "purchase", AggregateOperator: "sum", Operator: "approx", Value: 1,
			}, 1, "NEW.email")
		require.EqualError(t, err, "invalid operator: approx")

		_, _, _, err = qb.parseCustomEventsGoalConditionWithEmailRef(
			&domain.CustomEventsGoalCondition{
				GoalType: "purchase", AggregateOperator: "sum", Operator: "between", Value: 1,
			}, 1, "NEW.email")
		require.EqualError(t, err, "between operator requires value_2")

		_, _, _, err = qb.parseCustomEventsGoalConditionWithEmailRef(
			&domain.CustomEventsGoalCondition{
				GoalType: "purchase", AggregateOperator: "sum", Operator: "gte", Value: 1,
				TimeframeOperator: "before_date", TimeframeValues: []string{"garbage"},
			}, 1, "NEW.email")
		require.ErrorContains(t, err, "invalid time:")

		_, _, _, err = qb.parseCustomEventsGoalConditionWithEmailRef(
			&domain.CustomEventsGoalCondition{
				GoalType: "purchase", AggregateOperator: "sum", Operator: "gte", Value: 1,
				Filters: []*domain.DimensionFilter{
					{FieldName: "", FieldType: "string", Operator: "equals",
						StringValues: []string{"x"}},
				},
			}, 1, "NEW.email")
		require.EqualError(t, err, "filter must have 'field_name'")
	})
}

func gapTaskLogger(ctrl *gomock.Controller) *pkgmocks.MockLogger {
	l := pkgmocks.NewMockLogger(ctrl)
	l.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(l).AnyTimes()
	l.EXPECT().WithFields(gomock.Any()).Return(l).AnyTimes()
	l.EXPECT().Debug(gomock.Any()).AnyTimes()
	l.EXPECT().Info(gomock.Any()).AnyTimes()
	l.EXPECT().Warn(gomock.Any()).AnyTimes()
	l.EXPECT().Error(gomock.Any()).AnyTimes()
	return l
}

// setupGapExecService returns a service whose repository runs transaction
// callbacks inline, mirroring the established task_service_test pattern.
func setupGapExecService(t *testing.T) (*TaskService, *mocks.MockTaskRepository, *gomock.Controller) {
	t.Helper()
	ctrl := gomock.NewController(t)
	repo := mocks.NewMockTaskRepository(ctrl)
	settingRepo := mocks.NewMockSettingRepository(ctrl)
	svc := NewTaskService(repo, settingRepo, gapTaskLogger(ctrl), nil, "http://localhost:8080")
	svc.SetAutoExecuteImmediate(false)
	repo.EXPECT().WithTransaction(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, fn func(*sql.Tx) error) error {
			return fn(nil)
		}).AnyTimes()
	return svc, repo, ctrl
}

func registerGapSendBroadcast(ctrl *gomock.Controller, svc *TaskService) *mocks.MockTaskProcessor {
	proc := mocks.NewMockTaskProcessor(ctrl)
	for _, supportedType := range getTaskTypes() {
		proc.EXPECT().CanProcess(supportedType).
			Return(supportedType == "send_broadcast").AnyTimes()
	}
	svc.RegisterProcessor(proc)
	return proc
}

func gapPendingTask() *domain.Task {
	bid := "bc-gap"
	return &domain.Task{
		ID: "task-gap", WorkspaceID: "ws-gap", Type: "send_broadcast",
		Status: domain.TaskStatusPending, MaxRuntime: 60, BroadcastID: &bid,
	}
}

func expectGapRunning(repo *mocks.MockTaskRepository, task *domain.Task) {
	repo.EXPECT().GetTx(gomock.Any(), gomock.Any(), task.WorkspaceID, task.ID).
		Return(task, nil)
	repo.EXPECT().MarkAsRunningTx(gomock.Any(), gomock.Any(), task.WorkspaceID, task.ID, gomock.Any()).
		Return(nil)
}

func TestGapTask_WithTransactionUnsupportedRepository(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// A nil repository cannot satisfy the transaction interface the service
	// asserts on, so the callback must never run.
	svc := NewTaskService(nil, mocks.NewMockSettingRepository(ctrl), gapTaskLogger(ctrl), nil, "")
	called := false
	err := svc.WithTransaction(context.Background(), func(tx *sql.Tx) error {
		called = true
		return nil
	})
	require.EqualError(t, err, "repository does not support transactions")
	assert.False(t, called, "fn must not run when transactions are unsupported")
}

func TestGapTask_NewDispatchRequestInvalidEndpoint(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	svc := NewTaskService(mocks.NewMockTaskRepository(ctrl),
		mocks.NewMockSettingRepository(ctrl), gapTaskLogger(ctrl), nil, "http://[::1")
	req, cancel, err := svc.newDispatchRequest(context.Background(),
		&domain.Task{ID: "task-gap", MaxRuntime: 60}, []byte(`{}`))
	require.Error(t, err)
	assert.Nil(t, req)
	assert.Nil(t, cancel, "a failed build must hand back no cancel func to leak")
}

func TestGapTask_GetTaskCarriesBroadcastID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mocks.NewMockTaskRepository(ctrl)
	svc := NewTaskService(repo, mocks.NewMockSettingRepository(ctrl),
		gapTaskLogger(ctrl), nil, "http://localhost:8080")

	bid := "bc-gap-get"
	want := &domain.Task{ID: "task-gap", WorkspaceID: "ws-gap",
		Type: "send_broadcast", Status: domain.TaskStatusPending, BroadcastID: &bid}
	repo.EXPECT().Get(gomock.Any(), "ws-gap", "task-gap").Return(want, nil)

	got, err := svc.GetTask(context.Background(), "ws-gap", "task-gap")
	require.NoError(t, err)
	require.True(t, got == want, "GetTask must return the repository task unchanged")
	require.NotNil(t, got.BroadcastID)
	assert.Equal(t, "bc-gap-get", *got.BroadcastID)
}

func TestGapTask_ExecuteTaskCancelledBeforeStart(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Strict controller: no repository expectation is set, so any lookup,
	// lease or write after the cancellation guard fails this test.
	repo := mocks.NewMockTaskRepository(ctrl)
	svc := NewTaskService(repo, mocks.NewMockSettingRepository(ctrl),
		gapTaskLogger(ctrl), nil, "http://localhost:8080")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := svc.ExecuteTask(ctx, "ws-gap", "task-gap", time.Now().Add(time.Minute))
	require.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, context.Canceled, err)
}

func TestGapTask_ExecuteTaskTerminalWriteFailures(t *testing.T) {
	timeoutAt := func() time.Time { return time.Now().Add(time.Minute) }

	t.Run("completion write failure is a task execution error", func(t *testing.T) {
		svc, repo, ctrl := setupGapExecService(t)
		defer ctrl.Finish()
		proc := registerGapSendBroadcast(ctrl, svc)
		task := gapPendingTask()
		expectGapRunning(repo, task)
		proc.EXPECT().Process(gomock.Any(), gomock.Any(), gomock.Any()).Return(true, nil)
		sentinel := errors.New("complete write unavailable")
		repo.EXPECT().MarkAsCompleted(gomock.Any(), "ws-gap", "task-gap", gomock.Any()).
			Return(sentinel)
		// No MarkAsFailed expectation: a completed run whose completion
		// write failed must not also be marked failed.

		err := svc.ExecuteTask(context.Background(), "ws-gap", "task-gap", timeoutAt())
		var execErr *domain.ErrTaskExecution
		require.ErrorAs(t, err, &execErr)
		assert.Equal(t, "task-gap", execErr.TaskID)
		assert.Equal(t, "failed to mark task as completed", execErr.Reason)
		assert.Equal(t, sentinel, execErr.Err)
	})

	t.Run("pending write failure after partial progress", func(t *testing.T) {
		svc, repo, ctrl := setupGapExecService(t)
		defer ctrl.Finish()
		proc := registerGapSendBroadcast(ctrl, svc)
		task := gapPendingTask()
		task.Progress = 40
		expectGapRunning(repo, task)
		proc.EXPECT().Process(gomock.Any(), gomock.Any(), gomock.Any()).Return(false, nil)
		sentinel := errors.New("pending write unavailable")
		repo.EXPECT().MarkAsPending(gomock.Any(), "ws-gap", "task-gap",
			gomock.Any(), float64(40), gomock.Any()).Return(sentinel)

		err := svc.ExecuteTask(context.Background(), "ws-gap", "task-gap", timeoutAt())
		var execErr *domain.ErrTaskExecution
		require.ErrorAs(t, err, &execErr)
		assert.Equal(t, "failed to mark task as pending", execErr.Reason)
		assert.Equal(t, sentinel, execErr.Err)
	})

	t.Run("failure write failure wraps without hiding the store error", func(t *testing.T) {
		svc, repo, ctrl := setupGapExecService(t)
		defer ctrl.Finish()
		proc := registerGapSendBroadcast(ctrl, svc)
		task := gapPendingTask()
		expectGapRunning(repo, task)
		proc.EXPECT().Process(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(false, errors.New("processor exploded"))
		sentinel := errors.New("failed write unavailable")
		repo.EXPECT().MarkAsFailed(gomock.Any(), "ws-gap", "task-gap", gomock.Any()).
			Return(sentinel)

		err := svc.ExecuteTask(context.Background(), "ws-gap", "task-gap", timeoutAt())
		require.EqualError(t, err, "failed to mark task as failed: failed write unavailable")
		assert.ErrorIs(t, err, sentinel)
	})

	t.Run("recurring task deleted mid-run finishes quietly", func(t *testing.T) {
		svc, repo, ctrl := setupGapExecService(t)
		defer ctrl.Finish()
		proc := registerGapSendBroadcast(ctrl, svc)
		task := gapPendingTask()
		interval := int64(3600)
		task.RecurringInterval = &interval
		expectGapRunning(repo, task)
		proc.EXPECT().Process(gomock.Any(), gomock.Any(), gomock.Any()).Return(true, nil)
		repo.EXPECT().MarkAsPending(gomock.Any(), "ws-gap", "task-gap",
			gomock.Any(), float64(0), gomock.Any()).Return(domain.ErrTaskNotFound)

		err := svc.ExecuteTask(context.Background(), "ws-gap", "task-gap", timeoutAt())
		assert.NoError(t, err, "deletion during execution is not a failure")
	})

	t.Run("recurring reschedule failure names the reschedule step", func(t *testing.T) {
		svc, repo, ctrl := setupGapExecService(t)
		defer ctrl.Finish()
		proc := registerGapSendBroadcast(ctrl, svc)
		task := gapPendingTask()
		interval := int64(3600)
		task.RecurringInterval = &interval
		expectGapRunning(repo, task)
		proc.EXPECT().Process(gomock.Any(), gomock.Any(), gomock.Any()).Return(true, nil)
		sentinel := errors.New("reschedule write unavailable")
		repo.EXPECT().MarkAsPending(gomock.Any(), "ws-gap", "task-gap",
			gomock.Any(), float64(0), gomock.Any()).Return(sentinel)

		err := svc.ExecuteTask(context.Background(), "ws-gap", "task-gap", timeoutAt())
		var execErr *domain.ErrTaskExecution
		require.ErrorAs(t, err, &execErr)
		assert.Equal(t, "failed to reschedule recurring task", execErr.Reason)
		assert.Equal(t, sentinel, execErr.Err)
	})

	t.Run("recurring backoff penalizes the delay, never the stored interval", func(t *testing.T) {
		svc, repo, ctrl := setupGapExecService(t)
		defer ctrl.Finish()
		proc := registerGapSendBroadcast(ctrl, svc)
		task := gapPendingTask()
		interval := int64(3600)
		task.RecurringInterval = &interval
		task.State = &domain.TaskState{IntegrationSync: &domain.IntegrationSyncState{ConsecErrors: 3}}
		expectGapRunning(repo, task)
		proc.EXPECT().Process(gomock.Any(), gomock.Any(), gomock.Any()).Return(true, nil)

		var nextRun time.Time
		repo.EXPECT().MarkAsPending(gomock.Any(), "ws-gap", "task-gap",
			gomock.Any(), float64(0), gomock.Any()).
			DoAndReturn(func(_ context.Context, _, _ string, at time.Time, _ float64, _ *domain.TaskState) error {
				nextRun = at
				return nil
			})

		before := time.Now().UTC()
		err := svc.ExecuteTask(context.Background(), "ws-gap", "task-gap", timeoutAt())
		require.NoError(t, err)
		// 3 errors -> 3*3*10 = 90s backoff on 3600s, plus up to 10% jitter.
		low := before.Add(3690 * time.Second)
		high := time.Now().UTC().Add(4059 * time.Second)
		assert.False(t, nextRun.Before(low.Add(-2*time.Second)),
			"next run %v must include the 90s backoff (low %v)", nextRun, low)
		assert.False(t, nextRun.After(high.Add(2*time.Second)),
			"next run %v must stay within interval+backoff+jitter (high %v)", nextRun, high)
		assert.Equal(t, int64(3600), *task.RecurringInterval,
			"the backoff penalty must not rewrite the configured cadence")
	})
}

func TestGapTask_ExecuteTaskInterruptedReleaseOutcomes(t *testing.T) {
	setup := func(t *testing.T, releaseErr error) (*TaskService, *mocks.MockTaskRepository, *gomock.Controller, *string) {
		svc, repo, ctrl := setupGapExecService(t)
		proc := registerGapSendBroadcast(ctrl, svc)
		task := gapPendingTask()
		expectGapRunning(repo, task)
		proc.EXPECT().Process(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(false, fmt.Errorf("feed aborted: %w", context.Canceled))
		var reason string
		repo.EXPECT().ReleaseTask(gomock.Any(), "ws-gap", "task-gap", gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, _, _ string, r string, _ time.Time) error {
				reason = r
				return releaseErr
			})
		// No MarkAsFailed expectation: an interrupted run re-queues and
		// must not consume retry budget through the failure path.
		return svc, repo, ctrl, &reason
	}

	t.Run("already-claimed task keeps the interruption error", func(t *testing.T) {
		svc, _, ctrl, reason := setup(t, domain.ErrTaskNotRunning)
		defer ctrl.Finish()
		err := svc.ExecuteTask(context.Background(), "ws-gap", "task-gap", time.Now().Add(time.Minute))
		var execErr *domain.ErrTaskExecution
		require.ErrorAs(t, err, &execErr)
		assert.Equal(t, "processing failed", execErr.Reason)
		assert.Equal(t,
			"task execution failed [task-gap]: processing failed - feed aborted: context canceled",
			*reason, "the release must carry the execution error text")
	})

	t.Run("release failure wraps the store error", func(t *testing.T) {
		sentinel := errors.New("release store unavailable")
		svc, _, ctrl, _ := setup(t, sentinel)
		defer ctrl.Finish()
		err := svc.ExecuteTask(context.Background(), "ws-gap", "task-gap", time.Now().Add(time.Minute))
		require.EqualError(t, err, "failed to release interrupted task: release store unavailable")
		assert.ErrorIs(t, err, sentinel)
	})

	t.Run("released task returns the interruption, not a failure", func(t *testing.T) {
		svc, _, ctrl, _ := setup(t, nil)
		defer ctrl.Finish()
		err := svc.ExecuteTask(context.Background(), "ws-gap", "task-gap", time.Now().Add(time.Minute))
		var execErr *domain.ErrTaskExecution
		require.ErrorAs(t, err, &execErr)
		assert.ErrorIs(t, err, context.Canceled)
	})
}

func TestGapTask_ExecuteTaskTimeoutWriteFailure(t *testing.T) {
	svc, repo, ctrl := setupGapExecService(t)
	defer ctrl.Finish()
	proc := registerGapSendBroadcast(ctrl, svc)
	task := gapPendingTask()
	expectGapRunning(repo, task)

	block := make(chan struct{})
	t.Cleanup(func() { close(block) })
	proc.EXPECT().Process(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(context.Context, *domain.Task, time.Time) (bool, error) {
			<-block // ignores its context: only the grace deadline frees the caller
			return false, nil
		})
	sentinel := errors.New("timeout write unavailable")
	repo.EXPECT().MarkAsFailed(gomock.Any(), "ws-gap", "task-gap", gomock.Any()).
		Return(sentinel)

	timeoutAt := time.Now().Add(-taskExecutionGrace).Add(700 * time.Millisecond)
	start := time.Now()
	err := svc.ExecuteTask(context.Background(), "ws-gap", "task-gap", timeoutAt)
	assert.Less(t, time.Since(start), 5*time.Second,
		"a stuck processor must not hold ExecuteTask past the grace deadline")
	require.EqualError(t, err, "failed to mark timed-out task as failed: timeout write unavailable")
	assert.ErrorIs(t, err, sentinel)
}

func TestGapTask_HandleBroadcastPausedGuards(t *testing.T) {
	newSvc := func(t *testing.T) (*TaskService, *mocks.MockTaskRepository, *gomock.Controller) {
		ctrl := gomock.NewController(t)
		repo := mocks.NewMockTaskRepository(ctrl)
		svc := NewTaskService(repo, mocks.NewMockSettingRepository(ctrl),
			gapTaskLogger(ctrl), nil, "http://localhost:8080")
		svc.SetAutoExecuteImmediate(false)
		return svc, repo, ctrl
	}
	payload := func(data map[string]interface{}) domain.EventPayload {
		return domain.EventPayload{Type: domain.EventBroadcastPaused, WorkspaceID: "ws-gap", Data: data}
	}

	t.Run("missing mistyped and empty broadcast ids never reach the repository", func(t *testing.T) {
		for _, data := range []map[string]interface{}{
			nil,
			{},
			{"broadcast_id": 42},
			{"broadcast_id": ""},
		} {
			svc, _, ctrl := newSvc(t)
			// No repository expectation: any call fails the controller.
			svc.handleBroadcastPaused(context.Background(), payload(data))
			ctrl.Finish()
		}
	})

	t.Run("unknown broadcast leaves the task untouched", func(t *testing.T) {
		svc, repo, ctrl := newSvc(t)
		defer ctrl.Finish()
		repo.EXPECT().GetTaskByBroadcastID(gomock.Any(), "ws-gap", "bc-gap").
			Return(nil, errors.New("sql: no rows in result set"))
		// No MarkAsPaused expectation: lookup failure must not pause anything.
		svc.handleBroadcastPaused(context.Background(),
			payload(map[string]interface{}{"broadcast_id": "bc-gap"}))
	})

	t.Run("pause write failure is swallowed after exactly one attempt", func(t *testing.T) {
		svc, repo, ctrl := newSvc(t)
		defer ctrl.Finish()
		task := &domain.Task{ID: "task-gap", WorkspaceID: "ws-gap",
			Type: "send_broadcast", Status: domain.TaskStatusPending}
		repo.EXPECT().GetTaskByBroadcastID(gomock.Any(), "ws-gap", "bc-gap").Return(task, nil)
		var pausedAt time.Time
		repo.EXPECT().MarkAsPaused(gomock.Any(), "ws-gap", "task-gap", gomock.Any()).
			DoAndReturn(func(_ context.Context, _, _ string, at time.Time) error {
				pausedAt = at
				return errors.New("pause write unavailable")
			})

		before := time.Now().UTC()
		svc.handleBroadcastPaused(context.Background(),
			payload(map[string]interface{}{"broadcast_id": "bc-gap"}))
		delay := pausedAt.Sub(before)
		assert.True(t, delay > 23*time.Hour && delay < 25*time.Hour,
			"a pause must shelve the task for about 24h, got %v", delay)
	})
}

func TestGapTask_ResetTaskWriteFailure(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mocks.NewMockTaskRepository(ctrl)
	svc := NewTaskService(repo, mocks.NewMockSettingRepository(ctrl),
		gapTaskLogger(ctrl), nil, "http://localhost:8080")

	interval := int64(3600)
	lastErr := "boom"
	task := &domain.Task{
		ID: "task-gap", WorkspaceID: "ws-gap", Type: "sync_integration",
		Status: domain.TaskStatusFailed, RecurringInterval: &interval,
		State: &domain.TaskState{IntegrationSync: &domain.IntegrationSyncState{
			ConsecErrors: 5, LastError: &lastErr, LastErrorType: "transient",
		}},
	}
	repo.EXPECT().Get(gomock.Any(), "ws-gap", "task-gap").Return(task, nil)
	sentinel := errors.New("reset write unavailable")
	repo.EXPECT().MarkAsPending(gomock.Any(), "ws-gap", "task-gap",
		gomock.Any(), float64(0), gomock.Any()).
		DoAndReturn(func(_ context.Context, _, _ string, _ time.Time, _ float64, state *domain.TaskState) error {
			// The error state must already be cleared in the state handed
			// to the store, or the reset would resurrect it on the next run.
			assert.Equal(t, 0, state.IntegrationSync.ConsecErrors)
			assert.Nil(t, state.IntegrationSync.LastError)
			assert.Equal(t, "", state.IntegrationSync.LastErrorType)
			return sentinel
		})

	err := svc.ResetTask(context.Background(), "ws-gap", "task-gap")
	require.EqualError(t, err, "failed to reset task: reset write unavailable")
	assert.ErrorIs(t, err, sentinel)
}

func TestGapTask_BroadcastScheduledUnparseableTime(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mocks.NewMockTaskRepository(ctrl)
	svc := NewTaskService(repo, mocks.NewMockSettingRepository(ctrl),
		gapTaskLogger(ctrl), nil, "http://localhost:8080")
	svc.SetAutoExecuteImmediate(false)
	repo.EXPECT().WithTransaction(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, fn func(*sql.Tx) error) error {
			return fn(nil)
		}).AnyTimes()
	repo.EXPECT().GetTaskByBroadcastID(gomock.Any(), "ws-gap", "bc-gap-sched").
		Return(nil, errors.New("sql: no rows in result set"))

	var created *domain.Task
	repo.EXPECT().Create(gomock.Any(), "ws-gap", gomock.Any()).
		DoAndReturn(func(_ context.Context, _ string, task *domain.Task) error {
			created = task
			return nil
		})

	svc.handleBroadcastScheduled(context.Background(), domain.EventPayload{
		Type: domain.EventBroadcastScheduled, WorkspaceID: "ws-gap", EntityID: "bc-gap-sched",
		Data: map[string]interface{}{
			"send_now": false, "status": string(domain.BroadcastStatusScheduled),
			"scheduled_time": "not-a-time",
		},
	})

	require.NotNil(t, created, "a scheduled broadcast with no task must create one")
	assert.Equal(t, "send_broadcast", created.Type)
	assert.Equal(t, domain.TaskStatusPending, created.Status)
	require.NotNil(t, created.BroadcastID)
	assert.Equal(t, "bc-gap-sched", *created.BroadcastID)
	assert.Nil(t, created.NextRunAfter,
		"an unparseable scheduled_time must leave the task unscheduled, not invent a time")
	require.NotNil(t, created.State)
	require.NotNil(t, created.State.SendBroadcast)
	assert.Equal(t, "bc-gap-sched", created.State.SendBroadcast.BroadcastID)
}

func TestGapTask_ExecutePendingTasksConflictDispatch(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	var hits atomic.Int32
	var gotPath, gotTaskID, gotBody atomic.Value
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		gotPath.Store(r.URL.Path)
		gotTaskID.Store(r.Header.Get("X-Task-ID"))
		gotBody.Store(string(body))
		hits.Add(1)
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte("already running"))
	}))
	defer server.Close()

	repo := mocks.NewMockTaskRepository(ctrl)
	settingRepo := mocks.NewMockSettingRepository(ctrl)
	svc := NewTaskService(repo, settingRepo, gapTaskLogger(ctrl), nil, server.URL)
	svc.SetAutoExecuteImmediate(false)

	settingRepo.EXPECT().SetLastCronRun(gomock.Any()).Return(nil)
	bid := "bc-gap-409"
	repo.EXPECT().GetNextBatch(gomock.Any(), 1).Return([]*domain.Task{
		{ID: "task-gap-409", WorkspaceID: "ws-gap", Type: "send_broadcast",
			Status: domain.TaskStatusPending, MaxRuntime: 60, BroadcastID: &bid},
	}, nil)

	require.NoError(t, svc.ExecutePendingTasks(context.Background(), 1))
	require.Eventually(t, func() bool { return hits.Load() == 1 },
		5*time.Second, 10*time.Millisecond, "the due task must be dispatched exactly once")
	assert.Equal(t, "/api/tasks.execute", gotPath.Load())
	assert.Equal(t, "task-gap-409", gotTaskID.Load())
	body, _ := gotBody.Load().(string)
	assert.Contains(t, body, `"workspace_id":"ws-gap"`)
	assert.Contains(t, body, `"id":"task-gap-409"`)
}
