package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
)

// JSONB key filters address free-form keys (event properties, timeline
// changes). Two invariants keep them safe and correct: the key is always a
// bound argument placed before the value arguments (placeholder numbering
// depends on it), and typed comparisons cast the extracted text so '10' > '2'
// numerically and invalid stored text yields NULL instead of an error.

func TestParseJSONBKeyFilter(t *testing.T) {
	qb := NewQueryBuilder()
	const tmpl = "ce.properties->>%s"

	t.Run("validation failures are precise and consume no placeholder", func(t *testing.T) {
		_, _, next, err := qb.parseJSONBKeyFilter(nil, 4, tmpl, castDirectly)
		require.EqualError(t, err, "filter cannot be nil")
		assert.Equal(t, 4, next)

		_, _, _, err = qb.parseJSONBKeyFilter(
			&domain.DimensionFilter{Operator: "equals"}, 1, tmpl, castDirectly)
		require.EqualError(t, err, "filter must have 'field_name'")

		_, _, _, err = qb.parseJSONBKeyFilter(
			&domain.DimensionFilter{FieldName: "plan", Operator: "approx", FieldType: "string"}, 1, tmpl, castDirectly)
		require.EqualError(t, err, "invalid operator: approx")

		_, _, _, err = qb.parseJSONBKeyFilter(
			&domain.DimensionFilter{FieldName: "plan", Operator: "equals", FieldType: "boolean"}, 1, tmpl, castDirectly)
		require.EqualError(t, err, "invalid field type: boolean")

		_, _, _, err = qb.parseJSONBKeyFilter(
			&domain.DimensionFilter{FieldName: "plan", Operator: "equals", FieldType: "string"}, 1, tmpl, castDirectly)
		require.EqualError(t, err, "string filter must have 'string_values'")
	})

	t.Run("string equality binds key first, then value", func(t *testing.T) {
		cond, args, next, err := qb.parseJSONBKeyFilter(
			&domain.DimensionFilter{
				FieldName: "plan", Operator: "equals", FieldType: "string",
				StringValues: []string{"pro"},
			}, 3, tmpl, castDirectly)
		require.NoError(t, err)
		assert.Equal(t, "ce.properties->>$3 = $4", cond)
		assert.Equal(t, []interface{}{"plan", "pro"}, args,
			"key argument must precede value arguments or every $n shifts")
		assert.Equal(t, 5, next)
	})

	t.Run("is_set needs no value but still binds the key", func(t *testing.T) {
		cond, args, next, err := qb.parseJSONBKeyFilter(
			&domain.DimensionFilter{FieldName: "plan", Operator: "is_set", FieldType: "string"},
			2, tmpl, castDirectly)
		require.NoError(t, err)
		assert.Equal(t, "ce.properties->>$2 IS NOT NULL", cond)
		assert.Equal(t, []interface{}{"plan"}, args)
		assert.Equal(t, 3, next)
	})

	t.Run("number comparisons cast the extracted text", func(t *testing.T) {
		cond, args, next, err := qb.parseJSONBKeyFilter(
			&domain.DimensionFilter{
				FieldName: "total", Operator: "gt", FieldType: "number",
				NumberValues: []float64{100},
			}, 1, tmpl, castDirectly)
		require.NoError(t, err)
		assert.Equal(t, "(ce.properties->>$1)::numeric > $2", cond,
			"without the cast, '99' > '100' lexicographically")
		assert.Equal(t, []interface{}{"total", 100.0}, args)
		assert.Equal(t, 3, next)
	})

	t.Run("defensive casts turn non-numeric stored text into NULL, not an error", func(t *testing.T) {
		cond, _, _, err := qb.parseJSONBKeyFilter(
			&domain.DimensionFilter{
				FieldName: "total", Operator: "gt", FieldType: "number",
				NumberValues: []float64{100},
			}, 1, tmpl, castDefensively)
		require.NoError(t, err)
		assert.Contains(t, cond, "CASE WHEN ce.properties->>$1 ~ ")
		assert.Contains(t, cond, "THEN (ce.properties->>$1)::numeric END > $2")
	})

	t.Run("relative-day operators require a time field", func(t *testing.T) {
		_, _, _, err := qb.parseJSONBKeyFilter(
			&domain.DimensionFilter{
				FieldName: "at", Operator: "in_the_last_days", FieldType: "string",
				StringValues: []string{"30"},
			}, 1, tmpl, castDirectly)
		require.EqualError(t, err,
			"operator in_the_last_days can only be used with a date value, not string")
	})

	t.Run("contains matches case-insensitively with wildcards added by the builder", func(t *testing.T) {
		cond, args, _, err := qb.parseJSONBKeyFilter(
			&domain.DimensionFilter{
				FieldName: "city", Operator: "contains", FieldType: "string",
				StringValues: []string{"par"},
			}, 1, tmpl, castDirectly)
		require.NoError(t, err)
		assert.Contains(t, cond, "ILIKE")
		require.Len(t, args, 2)
		assert.Equal(t, "city", args[0])
		assert.Equal(t, "%par%", args[1], "the raw value must not reach LIKE unescaped-by-wildcards")
	})
}
