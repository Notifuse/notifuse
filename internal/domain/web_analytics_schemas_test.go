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

	t.Run("TimeScore and bounce rate read human sessions only", func(t *testing.T) {
		measures := WebAnalyticsSchemas(nil, "UTC")["web_sessions"].Measures
		assert.Contains(t, measures["median_duration"].SQL,
			"WITHIN GROUP (ORDER BY duration_ms) FILTER (WHERE NOT (beat_seq <= 1 AND duration_ms < 1000))")
		bounce := measures["bounce_rate"].SQL
		assert.Contains(t, bounce, "FILTER (WHERE NOT (beat_seq <= 1 AND duration_ms < 1000) AND duration_ms < 10000)",
			"the numerator counts human bounces")
		assert.Contains(t, bounce, "NULLIF(COUNT(*) FILTER (WHERE NOT (beat_seq <= 1 AND duration_ms < 1000)), 0)",
			"the denominator is human sessions, not every session")
		assert.Equal(t, "COUNT(*)", measures["sessions"].SQL, "sessions still count automated traffic")
	})

	// Scroll depth, pages per session and time on a page read the same population as
	// TimeScore: an automated session scrolls nothing and views one page, so counting it
	// pulled these down on the sites crawlers visit most.
	t.Run("every engagement measure reads human sessions only", func(t *testing.T) {
		measures := WebAnalyticsSchemas(nil, "UTC")["web_sessions"].Measures
		human := "FILTER (WHERE " + webSessionHumanSQL + ")"
		for _, name := range []string{"median_duration", "bounce_rate", "avg_scroll", "median_scroll", "pages_per_session", "median_page_duration"} {
			assert.Contains(t, measures[name].SQL, human, name)
			assert.Contains(t, measures[name].Description, "automated traffic is left out", name)
		}
		assert.Equal(t, "ROUND(AVG(max_scroll) FILTER (WHERE NOT (beat_seq <= 1 AND duration_ms < 1000)), 1)", measures["avg_scroll"].SQL)
		assert.Equal(t, "ROUND(AVG(pageview_count) FILTER (WHERE NOT (beat_seq <= 1 AND duration_ms < 1000)), 2)", measures["pages_per_session"].SQL)
		// Volume measures still count every session.
		for _, name := range []string{"sessions", "pageviews", "goal_conversions", "contacts"} {
			assert.NotContains(t, measures[name].SQL, human, name)
		}
	})

	// The page measures cannot tell a session's automated traffic from a page row, and say
	// so: their titles are the web_sessions measures' that leave it out.
	t.Run("page measures say they include automated traffic", func(t *testing.T) {
		measures := WebAnalyticsSchemas(nil, "UTC")["web_pages"].Measures
		for _, name := range []string{"page_duration", "page_scroll"} {
			assert.Contains(t, measures[name].Description, "Includes automated traffic", name)
			assert.NotContains(t, measures[name].SQL, webSessionHumanSQL, name)
		}
	})

	// The entry type is read from is_landing, which the ingestion recomputes across the
	// session's tabs. A page stored before 45.0 keeps the per-tab entry_type: the first
	// page of a second tab holds 'landing' there although it entered nothing.
	t.Run("page entry type follows the session's landing page", func(t *testing.T) {
		dimension := WebAnalyticsSchemas(nil, "UTC")["web_pages"].Dimensions["page_entry_type"]
		assert.Equal(t, "(CASE WHEN is_landing THEN '"+WebEntryTypeLanding+"' ELSE '"+WebEntryTypeNavigation+"' END)", dimension.SQL)
		assert.NotContains(t, dimension.SQL, "entry_type")
	})

	t.Run("automated traffic: dimension and measures share one rule", func(t *testing.T) {
		schema := WebAnalyticsSchemas(nil, "UTC")["web_sessions"]
		assert.Equal(t, "(CASE WHEN (beat_seq <= 1 AND duration_ms < 1000) THEN 'automated' ELSE 'human' END)",
			schema.Dimensions["traffic_type"].SQL)

		query := analytics.Query{
			Schema:     "web_sessions",
			Measures:   []string{"automated_sessions", "automated_share"},
			Dimensions: []string{"traffic_type"},
			Filters:    []analytics.Filter{{Member: "traffic_type", Operator: "equals", Values: []string{"automated"}}},
		}
		sqlStr, args, err := analytics.NewSQLBuilder().BuildSQL(query, schema)
		require.NoError(t, err)
		assert.Contains(t, sqlStr, "COUNT(*) FILTER (WHERE (beat_seq <= 1 AND duration_ms < 1000))")
		assert.Contains(t, sqlStr, "* 100.0 / NULLIF(COUNT(*), 0), 2)")
		assert.Contains(t, sqlStr, "GROUP BY (CASE WHEN (beat_seq <= 1 AND duration_ms < 1000) THEN 'automated' ELSE 'human' END)")
		assert.Contains(t, args, "automated")
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
		assert.Contains(t, sqlStr, "AND duration_ms < 10000)")
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
		assert.Contains(t, schemas["web_sessions"].Dimensions["hour_of_day"].SQL, "EXTRACT(HOUR FROM (created_at AT TIME ZONE 'UTC'))")
		assert.Contains(t, schemas["web_goals"].Dimensions["hour_of_day"].SQL, "EXTRACT(HOUR FROM (goal_at AT TIME ZONE 'UTC'))")
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

	t.Run("UTC and unusable timezones name UTC rather than leave the column bare", func(t *testing.T) {
		// EXTRACT on a bare timestamptz reads it in the database session's zone: on a server
		// not set to UTC, a UTC report would get the server's hours and days.
		dims := WebAnalyticsSchemas(nil, "UTC")["web_sessions"].Dimensions
		for _, name := range []string{"hour_of_day", "day_of_week", "is_weekend", "year", "month", "day", "week_number"} {
			assert.Contains(t, dims[name].SQL, "(created_at AT TIME ZONE 'UTC')", name)
		}
		for _, timezone := range []string{"UTC", "", "'; DROP TABLE web_sessions; --"} {
			sql := WebAnalyticsSchemas(nil, timezone)["web_sessions"].Dimensions["hour_of_day"].SQL
			assert.Equal(t, "(EXTRACT(HOUR FROM (created_at AT TIME ZONE 'UTC')))::int", sql, timezone)
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
			dimension := WebAnalyticsSchemas(nil, "UTC")[schema].Dimensions["contact_email"]
			assert.Equal(t, "COALESCE(contact_email, '')", dimension.SQL, schema)
			// set, notSet and an equality to an address compare the column itself.
			assert.Equal(t, "contact_email", dimension.NullableColumn, schema)
		}
	})

	t.Run("set and notSet on contact_email mean identified and anonymous", func(t *testing.T) {
		schema := WebAnalyticsSchemas(nil, "UTC")["web_sessions"]
		for operator, where := range map[string]string{
			"set":    "WHERE contact_email <> $1",
			"notSet": "WHERE (contact_email IS NULL OR contact_email = $1)",
		} {
			sqlStr, _, err := analytics.NewSQLBuilder().BuildSQL(analytics.Query{
				Schema: "web_sessions", Measures: []string{"sessions"},
				Filters: []analytics.Filter{{Member: "contact_email", Operator: operator}},
			}, schema)
			require.NoError(t, err)
			assert.True(t, strings.HasSuffix(sqlStr, where), "%s: %s", operator, sqlStr)
		}
	})

	t.Run("goal values are summed exactly, as a number", func(t *testing.T) {
		schemas := WebAnalyticsSchemas(nil, "UTC")
		for _, measure := range []analytics.MeasureDefinition{
			schemas["web_sessions"].Measures["goal_value"], schemas["web_goals"].Measures["sum_goal_value"],
		} {
			assert.Equal(t, "sum", measure.Type, "an empty period still totals 0")
			assert.Equal(t, "goal_value::text::numeric", measure.SQL)
			assert.Equal(t, "float8", measure.Cast)
		}
		sqlStr, _, err := analytics.NewSQLBuilder().BuildSQL(analytics.Query{
			Schema: "web_goals", Measures: []string{"sum_goal_value"},
		}, schemas["web_goals"])
		require.NoError(t, err)
		assert.Equal(t, "SELECT ((SUM(goal_value::text::numeric))::float8) AS sum_goal_value FROM web_goals", sqlStr)
	})
}
