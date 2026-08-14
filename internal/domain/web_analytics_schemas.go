package domain

import (
	"fmt"
	"strconv"

	"github.com/Notifuse/notifuse/pkg/analytics"
)

// WebAnalyticsSchemas builds the cube schemas for the three web analytics
// tables. Unlike PredefinedSchemas these are per-workspace: the bounce
// threshold is baked into the bounce_rate measure and the custom_* dimension
// titles come from the workspace's custom dimension labels. Metric names and
// dimension names follow Staminads so ported dashboards stay meaningful.
//
// They are also per-query, because the cyclic time dimensions (hour of day,
// day of week, …) have to be extracted in the timezone the report is read in:
// "traffic by hour" answers a question about the visitor's local morning, not
// about UTC. Pass the query's timezone; an empty or unusable one means UTC.
//
// Nil-receiver-safe on settings (defaults apply).
func WebAnalyticsSchemas(settings *WebAnalyticsSettings, timezone string) map[string]analytics.SchemaDefinition {
	bounceMs := settings.BounceThresholdMs()

	// localTime renders the expression the cyclic dimensions extract from.
	localTime := func(column string) string {
		zone := analytics.SanitizeTimezone(timezone)
		if zone == "" || zone == "UTC" {
			return column
		}
		return fmt.Sprintf("(%s AT TIME ZONE '%s')", column, zone)
	}

	partitionHint := &analytics.PartitionHint{
		Column: "session_date",
		// A session's rows sit in the partition of its uuid-derived start
		// date; created_at/entered_at/goal_at can drift from it by up to the
		// session-id acceptance window.
		SlackBefore: WebSessionIDMaxAge,
		SlackAfter:  WebSessionIDMaxAge,
	}

	customTitle := func(slot int) string {
		key := "custom_" + strconv.Itoa(slot)
		if settings != nil {
			if label, ok := settings.CustomDimensionLabels[key]; ok && label != "" {
				return label
			}
		}
		return fmt.Sprintf("Custom %d", slot)
	}

	// Attribution dimensions shared by web_sessions and web_goals (the goal
	// table denormalizes the session snapshot precisely so these group
	// without joins).
	attributionDimensions := func(timeColumn string) map[string]analytics.DimensionDefinition {
		dims := map[string]analytics.DimensionDefinition{
			"referrer":        {Type: "string", Title: "Referrer", SQL: "referrer"},
			"referrer_domain": {Type: "string", Title: "Referrer Domain", SQL: "referrer_domain"},
			"referrer_path":   {Type: "string", Title: "Referrer Path", SQL: "referrer_path"},
			"is_direct":       {Type: "string", Title: "Is Direct", SQL: "(is_direct::text)"},
			"landing_page":    {Type: "string", Title: "Landing Page", SQL: "landing_page"},
			"landing_domain":  {Type: "string", Title: "Landing Domain", SQL: "landing_domain"},
			"landing_path":    {Type: "string", Title: "Landing Path", SQL: "landing_path"},

			"utm_source":   {Type: "string", Title: "UTM Source", SQL: "utm_source"},
			"utm_medium":   {Type: "string", Title: "UTM Medium", SQL: "utm_medium"},
			"utm_campaign": {Type: "string", Title: "UTM Campaign", SQL: "utm_campaign"},
			"utm_term":     {Type: "string", Title: "UTM Term", SQL: "utm_term"},
			"utm_content":  {Type: "string", Title: "UTM Content", SQL: "utm_content"},

			"channel":       {Type: "string", Title: "Channel", SQL: "channel"},
			"channel_group": {Type: "string", Title: "Channel Group", SQL: "channel_group"},

			"device":          {Type: "string", Title: "Device", SQL: "device"},
			"browser":         {Type: "string", Title: "Browser", SQL: "browser"},
			"browser_type":    {Type: "string", Title: "Browser Type", SQL: "browser_type"},
			"os":              {Type: "string", Title: "Operating System", SQL: "os"},
			"connection_type": {Type: "string", Title: "Connection Type", SQL: "connection_type"},
			"screen_width":    {Type: "number", Title: "Screen Width", SQL: "screen_width"},
			"screen_height":   {Type: "number", Title: "Screen Height", SQL: "screen_height"},
			"viewport_width":  {Type: "number", Title: "Viewport Width", SQL: "viewport_width"},
			"viewport_height": {Type: "number", Title: "Viewport Height", SQL: "viewport_height"},

			"country":   {Type: "string", Title: "Country", SQL: "country"},
			"region":    {Type: "string", Title: "Region", SQL: "region"},
			"city":      {Type: "string", Title: "City", SQL: "city"},
			"latitude":  {Type: "number", Title: "Latitude", SQL: "latitude"},
			"longitude": {Type: "number", Title: "Longitude", SQL: "longitude"},
			"language":  {Type: "string", Title: "Language", SQL: "language"},
			"timezone":  {Type: "string", Title: "Timezone", SQL: "timezone"},

			// Nullable, unlike every other dimension here: folding NULL into the
			// empty string keeps the anonymous bucket selectable by the same
			// "is empty" filter that works everywhere else.
			"contact_email": {Type: "string", Title: "Contact Email", SQL: "COALESCE(contact_email, '')"},

			// Any dimension whose SQL is an expression rather than a bare column
			// must parenthesize ITSELF. The query builder interpolates this string
			// unwrapped into the select list, GROUP BY, filter comparisons and an
			// "<sql> AT TIME ZONE '<tz>'" wrap — in the last two, an unbracketed
			// expression binds to its final operand instead of the whole thing, and
			// the result is a query that runs and quietly answers the wrong question.
			//
			// Cyclic time dimensions.
			"hour_of_day": {Type: "number", Title: "Hour of Day", SQL: fmt.Sprintf("(EXTRACT(HOUR FROM %s))::int", localTime(timeColumn))},
			"day_of_week": {Type: "number", Title: "Day of Week", SQL: fmt.Sprintf("(EXTRACT(ISODOW FROM %s))::int", localTime(timeColumn))},
			"is_weekend":  {Type: "string", Title: "Is Weekend", SQL: fmt.Sprintf("(CASE WHEN EXTRACT(ISODOW FROM %s) IN (6, 7) THEN 'true' ELSE 'false' END)", localTime(timeColumn))},
			"year":        {Type: "number", Title: "Year", SQL: fmt.Sprintf("(EXTRACT(YEAR FROM %s))::int", localTime(timeColumn))},
			"month":       {Type: "number", Title: "Month", SQL: fmt.Sprintf("(EXTRACT(MONTH FROM %s))::int", localTime(timeColumn))},
			"day":         {Type: "number", Title: "Day", SQL: fmt.Sprintf("(EXTRACT(DAY FROM %s))::int", localTime(timeColumn))},
			"week_number": {Type: "number", Title: "Week Number", SQL: fmt.Sprintf("(EXTRACT(WEEK FROM %s))::int", localTime(timeColumn))},
		}
		for slot := 1; slot <= 10; slot++ {
			key := "custom_" + strconv.Itoa(slot)
			dims[key] = analytics.DimensionDefinition{Type: "string", Title: customTitle(slot), SQL: key}
		}
		return dims
	}

	sessionsDimensions := attributionDimensions("created_at")
	sessionsDimensions["created_at"] = analytics.DimensionDefinition{Type: "time", Title: "Session Start", SQL: "created_at"}
	sessionsDimensions["updated_at"] = analytics.DimensionDefinition{Type: "time", Title: "Last Activity", SQL: "updated_at"}
	sessionsDimensions["exit_path"] = analytics.DimensionDefinition{Type: "string", Title: "Exit Path", SQL: "exit_path"}
	sessionsDimensions["duration"] = analytics.DimensionDefinition{Type: "number", Title: "Duration (ms)", SQL: "duration_ms"}
	sessionsDimensions["pageview_count"] = analytics.DimensionDefinition{Type: "number", Title: "Pageview Count", SQL: "pageview_count"}
	sessionsDimensions["sdk_version"] = analytics.DimensionDefinition{Type: "string", Title: "SDK Version", SQL: "sdk_version"}

	goalsDimensions := attributionDimensions("goal_at")
	goalsDimensions["goal_at"] = analytics.DimensionDefinition{Type: "time", Title: "Goal Time", SQL: "goal_at"}
	goalsDimensions["goal_name"] = analytics.DimensionDefinition{Type: "string", Title: "Goal Name", SQL: "goal_name"}
	goalsDimensions["goal_path"] = analytics.DimensionDefinition{Type: "string", Title: "Goal Path", SQL: "path"}
	goalsDimensions["goal_value"] = analytics.DimensionDefinition{Type: "number", Title: "Goal Value", SQL: "goal_value"}
	goalsDimensions["goal_type"] = analytics.DimensionDefinition{Type: "string", Title: "Goal Type", SQL: "goal_type"}

	return map[string]analytics.SchemaDefinition{
		"web_sessions": {
			Name:          "web_sessions",
			PartitionHint: partitionHint,
			Measures: map[string]analytics.MeasureDefinition{
				"sessions": {Type: "count", Title: "Sessions", SQL: "COUNT(*)",
					Description: "Number of sessions"},
				"median_duration": {Type: "number", Title: "TimeScore",
					SQL:         "ROUND((PERCENTILE_CONT(0.5) WITHIN GROUP (ORDER BY duration_ms))::numeric / 1000.0, 1)",
					Description: "Median session engaged time in seconds"},
				"avg_scroll": {Type: "number", Title: "Avg Scroll Depth", SQL: "ROUND(AVG(max_scroll), 1)",
					Description: "Average maximum scroll depth (%)"},
				"median_scroll": {Type: "number", Title: "Median Scroll Depth",
					SQL: "ROUND((PERCENTILE_CONT(0.5) WITHIN GROUP (ORDER BY max_scroll))::numeric, 1)"},
				"bounce_rate": {Type: "number", Title: "Bounce Rate",
					SQL:         fmt.Sprintf("ROUND((COUNT(*) FILTER (WHERE duration_ms < %d)) * 100.0 / NULLIF(COUNT(*), 0), 2)", bounceMs),
					Description: fmt.Sprintf("Share of sessions with less than %ds of engaged time", bounceMs/1000)},
				"pageviews": {Type: "sum", Title: "Pageviews", SQL: "pageview_count",
					Description: "Total pageviews across sessions"},
				"pages_per_session": {Type: "number", Title: "Pages / Session", SQL: "ROUND(AVG(pageview_count), 2)"},
				"median_page_duration": {Type: "number", Title: "Median Page Duration",
					SQL: "ROUND((PERCENTILE_CONT(0.5) WITHIN GROUP (ORDER BY median_page_duration_ms))::numeric / 1000.0, 1)"},
				"goal_conversions": {Type: "count", Title: "Sessions with Goals", SQL: "*",
					Filters:     []analytics.MeasureFilter{{SQL: "goal_count > 0"}},
					Description: "Sessions that fired at least one goal"},
				"goal_value": {Type: "sum", Title: "Goal Value", SQL: "goal_value"},
				"contacts": {Type: "count_distinct", Title: "Identified Contacts", SQL: "contact_email",
					Description: "Distinct contacts identified via identify()"},
			},
			Dimensions: sessionsDimensions,
		},
		"web_pages": {
			Name:          "web_pages",
			PartitionHint: partitionHint,
			Measures: map[string]analytics.MeasureDefinition{
				"page_count":   {Type: "count", Title: "Page Views", SQL: "COUNT(*)"},
				"unique_pages": {Type: "count_distinct", Title: "Unique Pages", SQL: "path"},
				"page_duration": {Type: "number", Title: "Median Page Duration",
					SQL: "ROUND((PERCENTILE_CONT(0.5) WITHIN GROUP (ORDER BY duration_ms))::numeric / 1000.0, 1)"},
				"page_scroll": {Type: "number", Title: "Median Scroll Depth",
					SQL: "ROUND((PERCENTILE_CONT(0.5) WITHIN GROUP (ORDER BY max_scroll))::numeric, 1)"},
				"landing_page_count": {Type: "count", Title: "Entries", SQL: "*",
					Filters: []analytics.MeasureFilter{{SQL: "is_landing = TRUE"}}},
				"exit_page_count": {Type: "count", Title: "Exits", SQL: "*",
					Filters: []analytics.MeasureFilter{{SQL: "is_exit = TRUE"}}},
				"exit_rate": {Type: "number", Title: "Exit Rate",
					SQL: "ROUND((COUNT(*) FILTER (WHERE is_exit = TRUE)) * 100.0 / NULLIF(COUNT(*), 0), 2)"},
			},
			Dimensions: map[string]analytics.DimensionDefinition{
				"entered_at":      {Type: "time", Title: "Entered At", SQL: "entered_at"},
				"page_path":       {Type: "string", Title: "Page Path", SQL: "path"},
				"page_number":     {Type: "number", Title: "Page Number", SQL: "page_number"},
				"is_landing_page": {Type: "string", Title: "Is Landing Page", SQL: "(is_landing::text)"},
				"is_exit_page":    {Type: "string", Title: "Is Exit Page", SQL: "(is_exit::text)"},
				"page_entry_type": {Type: "string", Title: "Entry Type", SQL: "entry_type"},
				"contact_email":   {Type: "string", Title: "Contact Email", SQL: "COALESCE(contact_email, '')"},
			},
		},
		"web_goals": {
			Name:          "web_goals",
			PartitionHint: partitionHint,
			Measures: map[string]analytics.MeasureDefinition{
				"goals":          {Type: "count", Title: "Goals", SQL: "COUNT(*)"},
				"sum_goal_value": {Type: "sum", Title: "Total Goal Value", SQL: "goal_value"},
				"avg_goal_value": {Type: "number", Title: "Avg Goal Value", SQL: "ROUND(AVG(goal_value)::numeric, 2)"},
				"median_goal_value": {Type: "number", Title: "Median Goal Value",
					SQL: "ROUND((PERCENTILE_CONT(0.5) WITHIN GROUP (ORDER BY goal_value))::numeric, 2)"},
				"unique_sessions_with_goals": {Type: "count_distinct", Title: "Converting Sessions", SQL: "session_id"},
			},
			Dimensions: goalsDimensions,
		},
	}
}

// WebAnalyticsSchemaNames are the cube schemas backed by the web analytics
// tables. They are gated by the web_analytics permission rather than plain
// workspace membership.
var WebAnalyticsSchemaNames = map[string]bool{
	"web_sessions": true,
	"web_pages":    true,
	"web_goals":    true,
}

// IsWebAnalyticsSchema reports whether a schema holds visitor-level data.
func IsWebAnalyticsSchema(name string) bool {
	return WebAnalyticsSchemaNames[name]
}

// ResolveAnalyticsSchemas merges the static predefined schemas with the
// per-workspace web analytics schemas. Web analytics schemas are only exposed
// when the feature is configured on the workspace (enabled or not, so
// dashboards keep working while ingestion is paused).
func ResolveAnalyticsSchemas(settings *WebAnalyticsSettings, timezone string) map[string]analytics.SchemaDefinition {
	merged := make(map[string]analytics.SchemaDefinition, len(PredefinedSchemas)+3)
	for name, schema := range PredefinedSchemas {
		merged[name] = schema
	}
	if settings != nil {
		for name, schema := range WebAnalyticsSchemas(settings, timezone) {
			merged[name] = schema
		}
	}
	return merged
}
