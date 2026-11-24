package domain

import (
	"fmt"
	"strconv"

	"github.com/Notifuse/notifuse/pkg/analytics"
)

// Automated traffic: a session that loaded a page and never sent a second
// beat. The SDK beats again on pagehide, visibilitychange and beforeunload, so
// a person who leaves after half a second still sends one, while a tab closed
// from outside, as crawlers, AI agents and headless browsers driven over CDP
// close theirs, stays at its first beat. That is the signature observed on
// production traffic, not a bot detector: a robot that closes its page through
// the page lifecycle, or stays a second, counts as human. The duration bound
// keeps a single beat that already carries engaged time on the human side.
//
// Two kinds of human session match too. One in its first heartbeat interval,
// so a live "today" view overstates the share by the sessions started in the
// last few seconds, while a finished day is exact. And one whose closing beacon
// the browser dropped before the first heartbeat (an in-app web view torn down
// mid-request).
const (
	WebSessionAutomatedSQL = "(beat_seq <= 1 AND duration_ms < 1000)"
	webSessionHumanSQL     = "NOT " + WebSessionAutomatedSQL

	WebTrafficTypeAutomated = "automated"
	WebTrafficTypeHuman     = "human"
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
	// without joins). event names what the cyclic time dimensions date: the
	// session start or the goal.
	attributionDimensions := func(timeColumn, event string) map[string]analytics.DimensionDefinition {
		dims := map[string]analytics.DimensionDefinition{
			"referrer": {Type: "string", Title: "Referrer", SQL: "referrer",
				Description: "Full URL of the page that sent the visitor; empty for a direct visit"},
			"referrer_domain": {Type: "string", Title: "Referrer Domain", SQL: "referrer_domain",
				Description: "Domain of the referrer; empty for a direct visit or a link from the site itself"},
			"referrer_path": {Type: "string", Title: "Referrer Path", SQL: "referrer_path",
				Description: "Path of the referrer URL"},
			"is_direct": {Type: "string", Title: "Is Direct", SQL: "(is_direct::text)",
				Description: "'true' when the session came with no outside referrer, else 'false'"},
			"landing_page": {Type: "string", Title: "Landing Page", SQL: "landing_page",
				Description: "Full URL of the first page of the session"},
			"landing_domain": {Type: "string", Title: "Landing Domain", SQL: "landing_domain",
				Description: "Domain of the landing page"},
			"landing_path": {Type: "string", Title: "Landing Path", SQL: "landing_path",
				Description: "Path of the landing page"},

			"utm_source": {Type: "string", Title: "UTM Source", SQL: "utm_source",
				Description: "utm_source of the landing page URL, unless a filter rule set it"},
			"utm_medium": {Type: "string", Title: "UTM Medium", SQL: "utm_medium",
				Description: "utm_medium of the landing page URL, unless a filter rule set it"},
			"utm_campaign": {Type: "string", Title: "UTM Campaign", SQL: "utm_campaign",
				Description: "utm_campaign of the landing page URL, unless a filter rule set it"},
			"utm_term": {Type: "string", Title: "UTM Term", SQL: "utm_term",
				Description: "utm_term of the landing page URL, unless a filter rule set it"},
			"utm_content": {Type: "string", Title: "UTM Content", SQL: "utm_content",
				Description: "utm_content of the landing page URL, unless a filter rule set it"},

			"channel": {Type: "string", Title: "Channel", SQL: "channel",
				Description: "Channel set by the workspace's filter rules; empty when no rule matched"},
			"channel_group": {Type: "string", Title: "Channel Group", SQL: "channel_group",
				Description: "Channel group set by the workspace's filter rules; empty when no rule matched"},

			"device": {Type: "string", Title: "Device", SQL: "device",
				Description: "Device class reported by the browser: desktop, mobile or tablet"},
			"browser": {Type: "string", Title: "Browser", SQL: "browser",
				Description: "Browser name, such as Chrome or Safari; Unknown when not detected"},
			"browser_type": {Type: "string", Title: "Browser Type", SQL: "browser_type",
				Description: "Kind of client read from the user agent: crawler, inapp, email, fetcher or cli; empty for a regular browser"},
			"os": {Type: "string", Title: "Operating System", SQL: "os",
				Description: "Operating system name; Unknown when not detected"},
			"connection_type": {Type: "string", Title: "Connection Type", SQL: "connection_type",
				Description: "Effective connection type reported by the browser (slow-2g, 2g, 3g or 4g); empty in browsers that do not report it"},
			"screen_width": {Type: "number", Title: "Screen Width", SQL: "screen_width",
				Description: "Screen width in CSS pixels"},
			"screen_height": {Type: "number", Title: "Screen Height", SQL: "screen_height",
				Description: "Screen height in CSS pixels"},
			"viewport_width": {Type: "number", Title: "Viewport Width", SQL: "viewport_width",
				Description: "Browser window width in CSS pixels"},
			"viewport_height": {Type: "number", Title: "Viewport Height", SQL: "viewport_height",
				Description: "Browser window height in CSS pixels"},

			"country": {Type: "string", Title: "Country", SQL: "country",
				Description: "Country of the visitor's IP address as a two-letter code; empty when geolocation is off"},
			"region": {Type: "string", Title: "Region", SQL: "region",
				Description: "Region of the visitor's IP address; empty when geolocation or region storage is off"},
			"city": {Type: "string", Title: "City", SQL: "city",
				Description: "City of the visitor's IP address; empty when geolocation or city storage is off"},
			"latitude": {Type: "number", Title: "Latitude", SQL: "latitude",
				Description: "Latitude of the visitor's IP address, rounded to the workspace's precision"},
			"longitude": {Type: "number", Title: "Longitude", SQL: "longitude",
				Description: "Longitude of the visitor's IP address, rounded to the workspace's precision"},
			"language": {Type: "string", Title: "Language", SQL: "language",
				Description: "Browser language, such as en-US"},
			"timezone": {Type: "string", Title: "Timezone", SQL: "timezone",
				Description: "Time zone reported by the browser, such as Europe/Paris"},

			// Nullable, unlike every other dimension here: folding NULL into the
			// empty string keeps the anonymous bucket selectable by the same
			// "is empty" filter that works everywhere else.
			"contact_email": {Type: "string", Title: "Contact Email", SQL: "COALESCE(contact_email, '')",
				Description: "Email of the contact identified for the session; empty when the visitor is anonymous"},

			// Any dimension whose SQL is an expression rather than a bare column
			// must parenthesize ITSELF. The query builder interpolates this string
			// unwrapped into the select list, GROUP BY, filter comparisons and an
			// "<sql> AT TIME ZONE '<tz>'" wrap — in the last two, an unbracketed
			// expression binds to its final operand instead of the whole thing, and
			// the result is a query that runs and quietly answers the wrong question.
			//
			// Cyclic time dimensions.
			"hour_of_day": {Type: "number", Title: "Hour of Day", SQL: fmt.Sprintf("(EXTRACT(HOUR FROM %s))::int", localTime(timeColumn)),
				Description: fmt.Sprintf("Hour of the %s, 0 to 23, in the query's time zone", event)},
			"day_of_week": {Type: "number", Title: "Day of Week", SQL: fmt.Sprintf("(EXTRACT(ISODOW FROM %s))::int", localTime(timeColumn)),
				Description: fmt.Sprintf("Day of the week of the %s, 1 for Monday to 7 for Sunday, in the query's time zone", event)},
			"is_weekend": {Type: "string", Title: "Is Weekend", SQL: fmt.Sprintf("(CASE WHEN EXTRACT(ISODOW FROM %s) IN (6, 7) THEN 'true' ELSE 'false' END)", localTime(timeColumn)),
				Description: fmt.Sprintf("'true' when the %s falls on a Saturday or Sunday in the query's time zone, else 'false'", event)},
			"year": {Type: "number", Title: "Year", SQL: fmt.Sprintf("(EXTRACT(YEAR FROM %s))::int", localTime(timeColumn)),
				Description: fmt.Sprintf("Year of the %s in the query's time zone", event)},
			"month": {Type: "number", Title: "Month", SQL: fmt.Sprintf("(EXTRACT(MONTH FROM %s))::int", localTime(timeColumn)),
				Description: fmt.Sprintf("Month of the %s, 1 to 12, in the query's time zone", event)},
			"day": {Type: "number", Title: "Day", SQL: fmt.Sprintf("(EXTRACT(DAY FROM %s))::int", localTime(timeColumn)),
				Description: fmt.Sprintf("Day of the month of the %s, 1 to 31, in the query's time zone", event)},
			"week_number": {Type: "number", Title: "Week Number", SQL: fmt.Sprintf("(EXTRACT(WEEK FROM %s))::int", localTime(timeColumn)),
				Description: fmt.Sprintf("ISO week number of the %s in the query's time zone", event)},
		}
		for slot := 1; slot <= 10; slot++ {
			key := "custom_" + strconv.Itoa(slot)
			dims[key] = analytics.DimensionDefinition{Type: "string", Title: customTitle(slot), SQL: key,
				Description: fmt.Sprintf("Custom dimension %d, sent by the SDK or set by a filter rule", slot)}
		}
		return dims
	}

	sessionsDimensions := attributionDimensions("created_at", "session start")
	sessionsDimensions["created_at"] = analytics.DimensionDefinition{Type: "time", Title: "Session Start", SQL: "created_at",
		Description: "When the session started"}
	sessionsDimensions["updated_at"] = analytics.DimensionDefinition{Type: "time", Title: "Last Activity", SQL: "updated_at",
		Description: "When the session last sent a signal"}
	sessionsDimensions["exit_path"] = analytics.DimensionDefinition{Type: "string", Title: "Exit Path", SQL: "exit_path",
		Description: "Path of the last page viewed in the session"}
	sessionsDimensions["duration"] = analytics.DimensionDefinition{Type: "number", Title: "Duration (ms)", SQL: "duration_ms",
		Description: "Engaged time of the session in milliseconds, summed over its pages"}
	sessionsDimensions["pageview_count"] = analytics.DimensionDefinition{Type: "number", Title: "Pageview Count", SQL: "pageview_count",
		Description: "Pages viewed in the session, across its tabs"}
	sessionsDimensions["sdk_version"] = analytics.DimensionDefinition{Type: "string", Title: "SDK Version", SQL: "sdk_version",
		Description: "Version of the tracking script that sent the session"}
	sessionsDimensions["traffic_type"] = analytics.DimensionDefinition{Type: "string", Title: "Traffic Type",
		SQL: fmt.Sprintf("(CASE WHEN %s THEN '%s' ELSE '%s' END)",
			WebSessionAutomatedSQL, WebTrafficTypeAutomated, WebTrafficTypeHuman),
		Description: fmt.Sprintf("'%s' for a session that loaded a page and sent no further signal, else '%s'",
			WebTrafficTypeAutomated, WebTrafficTypeHuman)}

	goalsDimensions := attributionDimensions("goal_at", "goal")
	goalsDimensions["goal_at"] = analytics.DimensionDefinition{Type: "time", Title: "Goal Time", SQL: "goal_at",
		Description: "When the goal fired"}
	goalsDimensions["goal_name"] = analytics.DimensionDefinition{Type: "string", Title: "Goal Name", SQL: "goal_name",
		Description: "Name the site gave the goal"}
	goalsDimensions["goal_path"] = analytics.DimensionDefinition{Type: "string", Title: "Goal Path", SQL: "path",
		Description: "Path of the page the goal fired on"}
	goalsDimensions["goal_value"] = analytics.DimensionDefinition{Type: "number", Title: "Goal Value", SQL: "goal_value",
		Description: "Value sent with the goal, such as an order amount; 0 when none"}
	goalsDimensions["goal_type"] = analytics.DimensionDefinition{Type: "string", Title: "Goal Type", SQL: "goal_type",
		Description: "Goal type: purchase, subscription, lead, signup, booking, trial or other"}

	// The engagement measures read human sessions only, as median_duration and
	// bounce_rate do: an automated session scrolls nothing and stays one page, and
	// counting it would pull the scroll depth and pages per session down on the
	// sites crawlers visit most.
	human := func(aggregate string) string {
		return fmt.Sprintf("%s FILTER (WHERE %s)", aggregate, webSessionHumanSQL)
	}

	return map[string]analytics.SchemaDefinition{
		"web_sessions": {
			Name:          "web_sessions",
			PartitionHint: partitionHint,
			Measures: map[string]analytics.MeasureDefinition{
				"sessions": {Type: "count", Title: "Sessions", SQL: "COUNT(*)",
					Description: "Number of sessions"},
				"median_duration": {Type: "number", Title: "TimeScore",
					SQL:         fmt.Sprintf("ROUND((%s)::numeric / 1000.0, 1)", human("PERCENTILE_CONT(0.5) WITHIN GROUP (ORDER BY duration_ms)")),
					Description: "Median engaged time of human sessions in seconds; automated traffic is left out"},
				"automated_sessions": {Type: "count", Title: "Automated Sessions", SQL: "*",
					Filters:     []analytics.MeasureFilter{{SQL: WebSessionAutomatedSQL}},
					Description: "Sessions that loaded a page and sent no further signal: crawlers, AI agents, headless browsers"},
				"automated_share": {Type: "number", Title: "Automated Traffic",
					SQL:         fmt.Sprintf("ROUND((COUNT(*) FILTER (WHERE %s)) * 100.0 / NULLIF(COUNT(*), 0), 2)", WebSessionAutomatedSQL),
					Description: "Share of sessions that are automated traffic (%)"},
				"avg_scroll": {Type: "number", Title: "Avg Scroll Depth",
					SQL:         fmt.Sprintf("ROUND(%s, 1)", human("AVG(max_scroll)")),
					Description: "Average of each human session's deepest scroll (%); automated traffic is left out"},
				"median_scroll": {Type: "number", Title: "Median Scroll Depth",
					SQL:         fmt.Sprintf("ROUND((%s)::numeric, 1)", human("PERCENTILE_CONT(0.5) WITHIN GROUP (ORDER BY max_scroll)")),
					Description: "Median of each human session's deepest scroll (%); automated traffic is left out"},
				"bounce_rate": {Type: "number", Title: "Bounce Rate",
					SQL: fmt.Sprintf("ROUND((COUNT(*) FILTER (WHERE %s AND duration_ms < %d)) * 100.0 / NULLIF(COUNT(*) FILTER (WHERE %s), 0), 2)",
						webSessionHumanSQL, bounceMs, webSessionHumanSQL),
					Description: fmt.Sprintf("Share of human sessions with less than %ds of engaged time; automated traffic is left out", bounceMs/1000)},
				"pageviews": {Type: "sum", Title: "Pageviews", SQL: "pageview_count",
					Description: "Total pageviews across sessions"},
				"pages_per_session": {Type: "number", Title: "Pages / Session",
					SQL:         fmt.Sprintf("ROUND(%s, 2)", human("AVG(pageview_count)")),
					Description: "Average pages viewed per human session; automated traffic is left out"},
				"median_page_duration": {Type: "number", Title: "Median Page Duration",
					SQL:         fmt.Sprintf("ROUND((%s)::numeric / 1000.0, 1)", human("PERCENTILE_CONT(0.5) WITHIN GROUP (ORDER BY median_page_duration_ms)")),
					Description: "Median across human sessions of each session's median time on a page, in seconds; automated traffic is left out"},
				"goal_conversions": {Type: "count", Title: "Sessions with Goals", SQL: "*",
					Filters:     []analytics.MeasureFilter{{SQL: "goal_count > 0"}},
					Description: "Sessions that fired at least one goal"},
				"goal_value": {Type: "sum", Title: "Goal Value", SQL: "goal_value",
					Description: "Total value of the goals the sessions fired"},
				"contacts": {Type: "count_distinct", Title: "Identified Contacts", SQL: "contact_email",
					Description: "Distinct contacts identified via identify()"},
			},
			Dimensions: sessionsDimensions,
		},
		"web_pages": {
			Name:          "web_pages",
			PartitionHint: partitionHint,
			Measures: map[string]analytics.MeasureDefinition{
				"page_count": {Type: "count", Title: "Page Views", SQL: "COUNT(*)",
					Description: "Number of page views"},
				"unique_pages": {Type: "count_distinct", Title: "Unique Pages", SQL: "path",
					Description: "Number of distinct page paths"},
				// Unlike the web_sessions measures of the same titles, these count
				// automated traffic. The rule is a session's (its beat_seq and
				// duration_ms across every tab), which a page row does not carry, and
				// a measure is an aggregate's FILTER: a lookup in web_sessions there
				// runs once per page row, not as a join.
				"page_duration": {Type: "number", Title: "Median Page Duration",
					SQL:         "ROUND((PERCENTILE_CONT(0.5) WITHIN GROUP (ORDER BY duration_ms))::numeric / 1000.0, 1)",
					Description: "Median engaged time on a page, in seconds. Includes automated traffic, unlike median_page_duration in web_sessions"},
				"page_scroll": {Type: "number", Title: "Median Scroll Depth",
					SQL:         "ROUND((PERCENTILE_CONT(0.5) WITHIN GROUP (ORDER BY max_scroll))::numeric, 1)",
					Description: "Median deepest scroll on a page (%). Includes automated traffic, unlike median_scroll in web_sessions"},
				"landing_page_count": {Type: "count", Title: "Entries", SQL: "*",
					Filters:     []analytics.MeasureFilter{{SQL: "is_landing = TRUE"}},
					Description: "Page views that started a session, one per session"},
				"exit_page_count": {Type: "count", Title: "Exits", SQL: "*",
					Filters:     []analytics.MeasureFilter{{SQL: "is_exit = TRUE"}},
					Description: "Page views that ended a session, one per session"},
				"exit_rate": {Type: "number", Title: "Exit Rate",
					SQL:         "ROUND((COUNT(*) FILTER (WHERE is_exit = TRUE)) * 100.0 / NULLIF(COUNT(*), 0), 2)",
					Description: "Share of page views that ended the session (%)"},
			},
			Dimensions: map[string]analytics.DimensionDefinition{
				"entered_at": {Type: "time", Title: "Entered At", SQL: "entered_at",
					Description: "When the visitor opened the page"},
				"page_path": {Type: "string", Title: "Page Path", SQL: "path",
					Description: "Path of the page"},
				"page_number": {Type: "number", Title: "Page Number", SQL: "page_number",
					Description: "Position of the page in its browser tab, from 1"},
				"is_landing_page": {Type: "string", Title: "Is Landing Page", SQL: "(is_landing::text)",
					Description: "'true' for the first page of the session across all its tabs, else 'false'"},
				"is_exit_page": {Type: "string", Title: "Is Exit Page", SQL: "(is_exit::text)",
					Description: "'true' for the last page of the session across all its tabs, else 'false'"},
				// Read from is_landing, which the ingestion recomputes across the
				// session's tabs. The rollup now writes entry_type from it as well, but
				// a page stored before 45.0 keeps the per-tab value, where the first
				// page of every tab holds 'landing'.
				"page_entry_type": {Type: "string", Title: "Entry Type",
					SQL: fmt.Sprintf("(CASE WHEN is_landing THEN '%s' ELSE '%s' END)", WebEntryTypeLanding, WebEntryTypeNavigation),
					Description: fmt.Sprintf("'%s' for the first page of the session, '%s' for the others",
						WebEntryTypeLanding, WebEntryTypeNavigation)},
				"contact_email": {Type: "string", Title: "Contact Email", SQL: "COALESCE(contact_email, '')",
					Description: "Email of the contact identified for the session; empty when the visitor is anonymous"},
			},
		},
		"web_goals": {
			Name:          "web_goals",
			PartitionHint: partitionHint,
			Measures: map[string]analytics.MeasureDefinition{
				"goals": {Type: "count", Title: "Goals", SQL: "COUNT(*)",
					Description: "Number of goals fired"},
				"sum_goal_value": {Type: "sum", Title: "Total Goal Value", SQL: "goal_value",
					Description: "Total value of the goals"},
				"avg_goal_value": {Type: "number", Title: "Avg Goal Value", SQL: "ROUND(AVG(goal_value)::numeric, 2)",
					Description: "Average value of a goal"},
				"median_goal_value": {Type: "number", Title: "Median Goal Value",
					SQL:         "ROUND((PERCENTILE_CONT(0.5) WITHIN GROUP (ORDER BY goal_value))::numeric, 2)",
					Description: "Median value of a goal"},
				"unique_sessions_with_goals": {Type: "count_distinct", Title: "Converting Sessions", SQL: "session_id",
					Description: "Number of sessions that fired at least one goal"},
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
