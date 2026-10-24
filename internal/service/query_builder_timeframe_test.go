package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The timeframe parsers build placeholder-numbered SQL. An off-by-one in the
// returned argIndex shifts every subsequent condition's $n onto the wrong
// argument, so these tests pin the SQL text, the args, and the next index
// together for every operator, starting from a non-1 index.

func TestParseGoalTimeframeCondition(t *testing.T) {
	qb := NewQueryBuilder()

	t.Run("in_date_range with RFC3339 values", func(t *testing.T) {
		cond, args, next, err := qb.parseGoalTimeframeCondition(
			"in_date_range", []string{"2026-01-02T03:04:05Z", "2026-02-03T04:05:06Z"}, 3)
		require.NoError(t, err)
		assert.Equal(t, "ce.occurred_at BETWEEN $3 AND $4", cond)
		require.Len(t, args, 2)
		assert.Equal(t, time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC), args[0])
		assert.Equal(t, time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC), args[1])
		assert.Equal(t, 5, next, "two placeholders consumed")
	})

	t.Run("in_date_range falls back to date-only values", func(t *testing.T) {
		cond, args, next, err := qb.parseGoalTimeframeCondition(
			"in_date_range", []string{"2026-01-02", "2026-02-03"}, 1)
		require.NoError(t, err)
		assert.Equal(t, "ce.occurred_at BETWEEN $1 AND $2", cond)
		require.Len(t, args, 2)
		assert.Equal(t, time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC), args[0])
		assert.Equal(t, 3, next)
	})

	t.Run("in_date_range requires exactly two values", func(t *testing.T) {
		for _, values := range [][]string{nil, {"2026-01-02"}, {"2026-01-02", "2026-02-03", "2026-03-04"}} {
			_, _, next, err := qb.parseGoalTimeframeCondition("in_date_range", values, 7)
			require.EqualError(t, err, "in_date_range requires 2 values (start and end)")
			assert.Equal(t, 7, next, "index must not advance on error")
		}
	})

	t.Run("in_date_range reports which bound is invalid", func(t *testing.T) {
		_, _, _, err := qb.parseGoalTimeframeCondition("in_date_range",
			[]string{"not-a-date", "2026-02-03"}, 1)
		require.ErrorContains(t, err, "invalid start time:")

		_, _, _, err = qb.parseGoalTimeframeCondition("in_date_range",
			[]string{"2026-01-02", "not-a-date"}, 1)
		require.ErrorContains(t, err, "invalid end time:")
	})

	t.Run("before_date and after_date use strict and loose comparisons", func(t *testing.T) {
		cond, args, next, err := qb.parseGoalTimeframeCondition("before_date", []string{"2026-01-02"}, 2)
		require.NoError(t, err)
		assert.Equal(t, "ce.occurred_at < $2", cond)
		require.Len(t, args, 1)
		assert.Equal(t, 3, next)

		cond, args, next, err = qb.parseGoalTimeframeCondition("after_date", []string{"2026-01-02T00:00:00Z"}, 4)
		require.NoError(t, err)
		assert.Equal(t, "ce.occurred_at > $4", cond)
		require.Len(t, args, 1)
		assert.Equal(t, 5, next)
	})

	t.Run("before_date requires one parseable value", func(t *testing.T) {
		_, _, _, err := qb.parseGoalTimeframeCondition("before_date", nil, 1)
		require.EqualError(t, err, "before_date requires 1 value")

		_, _, _, err = qb.parseGoalTimeframeCondition("after_date", []string{"2026-01-02", "extra"}, 1)
		require.EqualError(t, err, "after_date requires 1 value")

		_, _, _, err = qb.parseGoalTimeframeCondition("before_date", []string{"garbage"}, 1)
		require.ErrorContains(t, err, "invalid time:")
	})

	t.Run("in_the_last_days inlines the parsed int and consumes no placeholder", func(t *testing.T) {
		cond, args, next, err := qb.parseGoalTimeframeCondition("in_the_last_days", []string{"30"}, 6)
		require.NoError(t, err)
		assert.Equal(t, "ce.occurred_at > NOW() - INTERVAL '30 days'", cond)
		assert.Empty(t, args)
		assert.Equal(t, 6, next, "an inlined interval must not consume a placeholder")
	})

	t.Run("in_the_last_days only ever inlines the parsed integer", func(t *testing.T) {
		// The value is inlined into SQL. Sscanf stops at the first non-digit,
		// so trailing text is dropped and only the int reaches the query —
		// pin that the raw string can never leak into the SQL.
		cond, _, _, err := qb.parseGoalTimeframeCondition("in_the_last_days",
			[]string{"30 days'; DROP TABLE contacts; --"}, 1)
		require.NoError(t, err)
		assert.Equal(t, "ce.occurred_at > NOW() - INTERVAL '30 days'", cond)
		assert.NotContains(t, cond, "DROP")

		_, _, _, err = qb.parseGoalTimeframeCondition("in_the_last_days", []string{"abc"}, 1)
		require.ErrorContains(t, err, "invalid days value:")

		_, _, _, err = qb.parseGoalTimeframeCondition("in_the_last_days", nil, 1)
		require.EqualError(t, err, "in_the_last_days requires 1 value (number of days)")
	})

	t.Run("unknown operator is rejected by name", func(t *testing.T) {
		_, _, _, err := qb.parseGoalTimeframeCondition("next_week", []string{"1"}, 1)
		require.EqualError(t, err, "unsupported goal timeframe operator: next_week")
	})
}

func TestParseTimeframeCondition(t *testing.T) {
	qb := NewQueryBuilder()

	t.Run("in_date_range targets contact_timeline created_at", func(t *testing.T) {
		cond, args, next, err := qb.parseTimeframeCondition(
			"in_date_range", []string{"2026-01-02T03:04:05Z", "2026-02-03T04:05:06Z"}, 2)
		require.NoError(t, err)
		assert.Equal(t, "ct.created_at BETWEEN $2 AND $3", cond)
		require.Len(t, args, 2)
		assert.Equal(t, 4, next)
	})

	t.Run("date-only values are rejected here, unlike goal timeframes", func(t *testing.T) {
		// parseTimeframeCondition has no 2006-01-02 fallback: pin the
		// asymmetry so unifying the two parsers is a deliberate change.
		_, _, _, err := qb.parseTimeframeCondition("in_date_range",
			[]string{"2026-01-02", "2026-02-03"}, 1)
		require.ErrorContains(t, err, "invalid start time:")

		_, _, _, err = qb.parseTimeframeCondition("before_date", []string{"2026-01-02"}, 1)
		require.ErrorContains(t, err, "invalid time:")
	})

	t.Run("before and after comparisons and arity", func(t *testing.T) {
		cond, _, next, err := qb.parseTimeframeCondition("before_date", []string{"2026-01-02T00:00:00Z"}, 5)
		require.NoError(t, err)
		assert.Equal(t, "ct.created_at < $5", cond)
		assert.Equal(t, 6, next)

		cond, _, next, err = qb.parseTimeframeCondition("after_date", []string{"2026-01-02T00:00:00Z"}, 5)
		require.NoError(t, err)
		assert.Equal(t, "ct.created_at > $5", cond)
		assert.Equal(t, 6, next)

		_, _, _, err = qb.parseTimeframeCondition("before_date", nil, 1)
		require.EqualError(t, err, "before_date requires 1 value")
		_, _, _, err = qb.parseTimeframeCondition("in_date_range", []string{"2026-01-02T00:00:00Z"}, 1)
		require.EqualError(t, err, "in_date_range requires 2 values (start and end)")
	})

	t.Run("in_the_last_days inlines days against ct.created_at", func(t *testing.T) {
		cond, args, next, err := qb.parseTimeframeCondition("in_the_last_days", []string{"7"}, 3)
		require.NoError(t, err)
		assert.Equal(t, "ct.created_at > NOW() - INTERVAL '7 days'", cond)
		assert.Empty(t, args)
		assert.Equal(t, 3, next)

		_, _, _, err = qb.parseTimeframeCondition("in_the_last_days", []string{"seven"}, 1)
		require.ErrorContains(t, err, "invalid days value:")
	})

	t.Run("unknown operator is rejected by name", func(t *testing.T) {
		_, _, _, err := qb.parseTimeframeCondition("yesterday", []string{"1"}, 1)
		require.EqualError(t, err, "unsupported timeframe operator: yesterday")
	})
}
