package domain

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/pkg/analytics"
)

func TestWebAnalyticsSchemas(t *testing.T) {
	t.Run("three schemas with partition hints on session_date", func(t *testing.T) {
		schemas := WebAnalyticsSchemas(nil, "UTC")
		require.Len(t, schemas, 3)
		for _, name := range []string{"web_sessions", "web_pages", "web_goals"} {
			schema, ok := schemas[name]
			require.True(t, ok, name)
			assert.Equal(t, name, schema.Name, "schema key must equal the table name")
			require.NotNil(t, schema.PartitionHint, name)
			assert.Equal(t, "session_date", schema.PartitionHint.Column)
			assert.Equal(t, WebSessionIDMaxAge, schema.PartitionHint.SlackBefore)
		}
	})

	t.Run("bounce threshold defaults to 10s and follows settings", func(t *testing.T) {
		bounce := WebAnalyticsSchemas(nil, "UTC")["web_sessions"].Measures["bounce_rate"]
		assert.Contains(t, bounce.SQL, "duration_ms < 10000")

		bounce = WebAnalyticsSchemas(&WebAnalyticsSettings{BounceThresholdSeconds: 25}, "UTC")["web_sessions"].Measures["bounce_rate"]
		assert.Contains(t, bounce.SQL, "duration_ms < 25000")
	})

	t.Run("stm titles come from custom dimension labels", func(t *testing.T) {
		schemas := WebAnalyticsSchemas(&WebAnalyticsSettings{
			CustomDimensionLabels: map[string]string{"custom_1": "Plan", "custom_7": "Cohort"},
		}, "UTC")
		dims := schemas["web_sessions"].Dimensions
		assert.Equal(t, "Plan", dims["custom_1"].Title)
		assert.Equal(t, "Cohort", dims["custom_7"].Title)
		assert.Equal(t, "Custom 2", dims["custom_2"].Title)
		assert.Equal(t, "Cohort", schemas["web_goals"].Dimensions["custom_7"].Title, "labels apply to goals too")
	})

	t.Run("every measure and dimension carries SQL", func(t *testing.T) {
		for name, schema := range WebAnalyticsSchemas(nil, "UTC") {
			for measure, def := range schema.Measures {
				assert.NotEmpty(t, def.SQL, "%s.%s", name, measure)
				assert.NotEmpty(t, def.Title, "%s.%s", name, measure)
			}
			for dimension, def := range schema.Dimensions {
				assert.NotEmpty(t, def.SQL, "%s.%s", name, dimension)
			}
		}
	})

	t.Run("percentile measures pass through the builder unwrapped", func(t *testing.T) {
		schema := WebAnalyticsSchemas(nil, "UTC")["web_sessions"]
		query := analytics.Query{
			Schema:   "web_sessions",
			Measures: []string{"median_duration", "sessions", "bounce_rate", "pageviews"},
		}
		sqlStr, _, err := analytics.NewSQLBuilder().BuildSQL(query, schema)
		require.NoError(t, err)
		assert.Contains(t, sqlStr, "PERCENTILE_CONT(0.5) WITHIN GROUP (ORDER BY duration_ms)")
		assert.NotContains(t, sqlStr, "COUNT(ROUND", "raw SQL measures must not be re-wrapped")
		assert.Contains(t, sqlStr, "SUM(pageview_count)", "sum-typed measures wrap the bare column")
		assert.Contains(t, sqlStr, "FILTER (WHERE duration_ms < 10000)")
	})

	t.Run("boolean dimensions are self-parenthesized text expressions", func(t *testing.T) {
		schemas := WebAnalyticsSchemas(nil, "UTC")
		assert.Equal(t, "(is_direct::text)", schemas["web_sessions"].Dimensions["is_direct"].SQL)
		assert.Equal(t, "(is_exit::text)", schemas["web_pages"].Dimensions["is_exit_page"].SQL)

		// And they group/filter correctly through the builder.
		query := analytics.Query{
			Schema:     "web_sessions",
			Measures:   []string{"sessions"},
			Dimensions: []string{"is_direct"},
			Filters:    []analytics.Filter{{Member: "is_direct", Operator: "equals", Values: []string{"true"}}},
		}
		sqlStr, args, err := analytics.NewSQLBuilder().BuildSQL(query, schemas["web_sessions"])
		require.NoError(t, err)
		assert.Contains(t, sqlStr, "(is_direct::text) AS is_direct")
		assert.Contains(t, sqlStr, "GROUP BY (is_direct::text)")
		assert.Contains(t, args, "true")
	})

	t.Run("cyclic time dimensions use EXTRACT on the schema's time column", func(t *testing.T) {
		schemas := WebAnalyticsSchemas(nil, "UTC")
		assert.Contains(t, schemas["web_sessions"].Dimensions["hour_of_day"].SQL, "EXTRACT(HOUR FROM created_at)")
		assert.Contains(t, schemas["web_goals"].Dimensions["hour_of_day"].SQL, "EXTRACT(HOUR FROM goal_at)")
		assert.Contains(t, schemas["web_sessions"].Dimensions["is_weekend"].SQL, "ISODOW")
	})
}

func TestResolveAnalyticsSchemas(t *testing.T) {
	t.Run("nil settings expose only the predefined schemas", func(t *testing.T) {
		schemas := ResolveAnalyticsSchemas(nil, "UTC")
		assert.Len(t, schemas, len(PredefinedSchemas))
		assert.NotContains(t, schemas, "web_sessions")
	})

	t.Run("configured settings add the three web schemas", func(t *testing.T) {
		schemas := ResolveAnalyticsSchemas(&WebAnalyticsSettings{}, "UTC")
		assert.Len(t, schemas, len(PredefinedSchemas)+3)
		assert.Contains(t, schemas, "web_sessions")
		assert.Contains(t, schemas, "message_history", "predefined schemas kept")
	})

	t.Run("web schema names never collide with predefined ones", func(t *testing.T) {
		for name := range WebAnalyticsSchemas(nil, "UTC") {
			_, collision := PredefinedSchemas[name]
			assert.False(t, collision, name)
			assert.True(t, strings.HasPrefix(name, "web_"), name)
		}
	})
}

func TestWebAnalyticsSchemasTimezone(t *testing.T) {
	t.Run("cyclic dimensions are extracted in the query timezone", func(t *testing.T) {
		// "Traffic by hour of day" asks about the visitor's local morning. Read
		// in UTC it would place a 9am peak in Los Angeles at 4pm, and the
		// heat map's own click-to-filter would then disagree with the chart
		// above it.
		dims := WebAnalyticsSchemas(nil, "America/Los_Angeles")["web_sessions"].Dimensions
		for _, name := range []string{"hour_of_day", "day_of_week", "is_weekend", "year", "month", "day", "week_number"} {
			assert.Contains(t, dims[name].SQL, "AT TIME ZONE 'America/Los_Angeles'", name)
		}
		assert.Contains(t, WebAnalyticsSchemas(nil, "America/Los_Angeles")["web_goals"].
			Dimensions["hour_of_day"].SQL, "goal_at AT TIME ZONE", "goals extract from their own clock")
	})

	t.Run("UTC and unusable timezones leave the column bare", func(t *testing.T) {
		for _, timezone := range []string{"UTC", "", "'; DROP TABLE web_sessions; --"} {
			sql := WebAnalyticsSchemas(nil, timezone)["web_sessions"].Dimensions["hour_of_day"].SQL
			assert.Equal(t, "(EXTRACT(HOUR FROM created_at))::int", sql, timezone)
		}
	})

	t.Run("a timezone never reaches SQL unescaped", func(t *testing.T) {
		sql := WebAnalyticsSchemas(nil, "Europe/Paris'; DROP TABLE web_sessions; --")["web_sessions"].
			Dimensions["hour_of_day"].SQL
		assert.NotContains(t, sql, "DROP TABLE")
	})

	t.Run("the nullable user id groups its anonymous rows under the empty string", func(t *testing.T) {
		// Every other dimension is NOT NULL DEFAULT '', so "is empty" is an
		// equality with ''. Left raw, the anonymous bucket would be a NULL that
		// no filter the console can build is able to select again.
		for _, schema := range []string{"web_sessions", "web_pages", "web_goals"} {
			assert.Equal(t, "COALESCE(contact_email, '')",
				WebAnalyticsSchemas(nil, "UTC")[schema].Dimensions["contact_email"].SQL, schema)
		}
	})
}
