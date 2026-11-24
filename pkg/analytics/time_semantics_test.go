package analytics

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func timeSemanticsSchema() SchemaDefinition {
	return SchemaDefinition{
		Name: "web_sessions",
		Measures: map[string]MeasureDefinition{
			"sessions":    {Type: "count", SQL: "COUNT(*)"},
			"pageviews":   {Type: "sum", SQL: "pageview_count"},
			"bounce_rate": {Type: "number", SQL: "ROUND(AVG(bounced), 2)"},
		},
		Dimensions: map[string]DimensionDefinition{
			"created_at":    {Type: "time", SQL: "created_at"},
			"channel":       {Type: "string", SQL: "channel"},
			"contact_email": {Type: "string", SQL: "COALESCE(contact_email, '')", NullableColumn: "contact_email"},
		},
		PartitionHint: &PartitionHint{Column: "session_date"},
	}
}

func whereOf(t *testing.T, sqlStr string) string {
	t.Helper()
	index := strings.Index(sqlStr, "WHERE")
	require.GreaterOrEqual(t, index, 0, sqlStr)
	return sqlStr[index:]
}

// A date operator on a time dimension reads its values as a time dimension's dateRange does:
// in the query's timezone, a bare date naming its whole day.
func TestDateOperatorFilters(t *testing.T) {
	paris := "Europe/Paris"
	query := func(timezone *string, operator string, values ...string) Query {
		return Query{
			Schema: "web_sessions", Measures: []string{"sessions"}, Timezone: timezone,
			Filters: []Filter{{Member: "created_at", Operator: operator, Values: values}},
		}
	}
	// Paris midnights in August (UTC+2).
	aug1 := time.Date(2026, 7, 31, 22, 0, 0, 0, time.UTC)
	aug8 := time.Date(2026, 8, 7, 22, 0, 0, 0, time.UTC)
	aug9 := time.Date(2026, 8, 8, 22, 0, 0, 0, time.UTC)

	t.Run("inDateRange reads bare dates in the timezone and keeps the whole last day", func(t *testing.T) {
		sqlStr, args := buildSQL(t, query(&paris, "inDateRange", "2026-08-01", "2026-08-08"), timeSemanticsSchema())
		where := whereOf(t, sqlStr)
		assert.Contains(t, where, "(created_at >= $1 AND created_at < $2)")
		assert.NotContains(t, where, "AT TIME ZONE")
		require.Len(t, args, 4)
		assert.Equal(t, aug1, args[0])
		assert.Equal(t, aug9, args[1])
		// Pruning reads the same bounds: from Jul 31 (UTC) to Aug 8 (UTC), the last instant of
		// Aug 8 in Paris.
		assert.Equal(t, "2026-07-31", args[2])
		assert.Equal(t, "2026-08-08", args[3])
	})

	t.Run("a single bare day is the whole of that day", func(t *testing.T) {
		_, args := buildSQL(t, query(&paris, "inDateRange", "2026-08-08", "2026-08-08"), timeSemanticsSchema())
		assert.Equal(t, aug8, args[0])
		assert.Equal(t, aug9, args[1])
	})

	t.Run("an instant as the end is included as given", func(t *testing.T) {
		sqlStr, args := buildSQL(t, query(&paris, "inDateRange", "2026-08-01T00:00:00Z", "2026-08-08T12:00:00Z"), timeSemanticsSchema())
		assert.Contains(t, whereOf(t, sqlStr), "(created_at >= $1 AND created_at <= $2)")
		assert.Equal(t, time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC), args[0])
		assert.Equal(t, time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC), args[1])
	})

	t.Run("a date and time without offset is read in the timezone", func(t *testing.T) {
		_, args := buildSQL(t, query(&paris, "inDateRange", "2026-08-01 08:00:00", "2026-08-01 18:00:00"), timeSemanticsSchema())
		assert.Equal(t, time.Date(2026, 8, 1, 6, 0, 0, 0, time.UTC), args[0])
		assert.Equal(t, time.Date(2026, 8, 1, 16, 0, 0, 0, time.UTC), args[1])
	})

	t.Run("notInDateRange keeps what lies outside the whole days", func(t *testing.T) {
		sqlStr, args := buildSQL(t, query(&paris, "notInDateRange", "2026-08-01", "2026-08-08"), timeSemanticsSchema())
		assert.Contains(t, whereOf(t, sqlStr), "(created_at < $1 OR created_at >= $2)")
		assert.Equal(t, []interface{}{aug1, aug9}, args, "no pruning: the rows lie outside the range")
	})

	t.Run("beforeDate stops at the start of the day named", func(t *testing.T) {
		sqlStr, args := buildSQL(t, query(&paris, "beforeDate", "2026-08-08"), timeSemanticsSchema())
		assert.Contains(t, whereOf(t, sqlStr), "created_at < $1")
		assert.Equal(t, []interface{}{aug8}, args)
	})

	t.Run("afterDate starts after the day named", func(t *testing.T) {
		sqlStr, args := buildSQL(t, query(&paris, "afterDate", "2026-08-08"), timeSemanticsSchema())
		assert.Contains(t, whereOf(t, sqlStr), "created_at >= $1")
		assert.Equal(t, []interface{}{aug9}, args)
	})

	t.Run("afterDate an instant leaves the instant out", func(t *testing.T) {
		sqlStr, args := buildSQL(t, query(&paris, "afterDate", "2026-08-08T10:00:00Z"), timeSemanticsSchema())
		assert.Contains(t, whereOf(t, sqlStr), "created_at > $1")
		assert.Equal(t, []interface{}{time.Date(2026, 8, 8, 10, 0, 0, 0, time.UTC)}, args)
	})

	t.Run("without a timezone the days are UTC days", func(t *testing.T) {
		_, args := buildSQL(t, query(nil, "inDateRange", "2026-08-01", "2026-08-08"), timeSemanticsSchema())
		assert.Equal(t, time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC), args[0])
		assert.Equal(t, time.Date(2026, 8, 9, 0, 0, 0, 0, time.UTC), args[1])
	})

	t.Run("a value that does not parse is compared as given", func(t *testing.T) {
		sqlStr, args := buildSQL(t, query(&paris, "inDateRange", "last week", "2026-08-08"), timeSemanticsSchema())
		assert.Contains(t, whereOf(t, sqlStr), "(created_at >= $1 AND created_at <= $2)")
		assert.Equal(t, []interface{}{"last week", "2026-08-08"}, args, "no pruning either")
	})

	t.Run("a date operator on a dimension that is not a time one is left as it was", func(t *testing.T) {
		schema := timeSemanticsSchema()
		schema.Dimensions["day_label"] = DimensionDefinition{SQL: "day_label"}
		q := Query{Schema: "web_sessions", Measures: []string{"sessions"},
			Filters: []Filter{{Member: "day_label", Operator: "inDateRange", Values: []string{"2026-08-01", "2026-08-08"}}}}
		_, args := buildSQL(t, q, schema)
		assert.Equal(t, []interface{}{"2026-08-01", "2026-08-08"}, args)
	})
}

// A time dimension's dateRange ending on a bare date stops before the next midnight, so the
// last microsecond of the day is in.
func TestDateRangeEndIsTheNextMidnightExcluded(t *testing.T) {
	paris := "Europe/Paris"
	q := Query{Schema: "web_sessions", Measures: []string{"sessions"}, Timezone: &paris,
		TimeDimensions: []TimeDimension{{Dimension: "created_at", Granularity: "day",
			// The day the clocks go back in Paris has 25 hours: its end is its own next midnight.
			DateRange: &[2]string{"2026-10-25", "2026-10-25"}}}}
	sqlStr, args := buildSQL(t, q, timeSemanticsSchema())
	assert.Contains(t, whereOf(t, sqlStr), "created_at >= $1 AND created_at < $2")
	assert.Equal(t, time.Date(2026, 10, 24, 22, 0, 0, 0, time.UTC), args[0])
	assert.Equal(t, time.Date(2026, 10, 25, 23, 0, 0, 0, time.UTC), args[1])
	assert.Equal(t, 25*time.Hour, args[1].(time.Time).Sub(args[0].(time.Time)))
}

// A dimension that reads a NULL column as the empty string compares the column itself where it can, so that
// set and notSet see the empty value and an equality to an address can use the column's index.
func TestNullableColumnFilters(t *testing.T) {
	query := func(operator string, values ...string) Query {
		return Query{Schema: "web_sessions", Measures: []string{"sessions"},
			Filters: []Filter{{Member: "contact_email", Operator: operator, Values: values}}}
	}
	for _, tc := range []struct {
		name     string
		query    Query
		where    string
		wantArgs []interface{}
	}{
		{"set is a non-empty value", query("set"), "WHERE contact_email <> $1", []interface{}{""}},
		{"notSet is NULL or empty", query("notSet"), "WHERE (contact_email IS NULL OR contact_email = $1)", []interface{}{""}},
		{"equals an address compares the bare column", query("equals", "a@example.com"),
			"WHERE contact_email = $1", []interface{}{"a@example.com"}},
		{"in addresses compares the bare column", query("in", "a@example.com", "b@example.com"),
			"WHERE contact_email IN ($1,$2)", []interface{}{"a@example.com", "b@example.com"}},
		// The console's "is empty" and "is not empty": the empty value matches NULL.
		{"equals the empty value keeps the folded expression", query("equals", ""),
			"WHERE COALESCE(contact_email, '') = $1", []interface{}{""}},
		{"notEquals the empty value keeps the folded expression", query("notEquals", ""),
			"WHERE COALESCE(contact_email, '') <> $1", []interface{}{""}},
		{"in with the empty value keeps the folded expression", query("in", "", "a@example.com"),
			"WHERE COALESCE(contact_email, '') IN ($1,$2)", []interface{}{"", "a@example.com"}},
		// An anonymous session is not a@example.com: the folded expression keeps it.
		{"notEquals an address keeps the folded expression", query("notEquals", "a@example.com"),
			"WHERE COALESCE(contact_email, '') <> $1", []interface{}{"a@example.com"}},
		{"contains keeps the folded expression", query("contains", "example"),
			"WHERE COALESCE(contact_email, '') LIKE $1 ESCAPE '\\'", []interface{}{"%example%"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sqlStr, args := buildSQL(t, tc.query, timeSemanticsSchema())
			assert.Equal(t, tc.where, whereOf(t, sqlStr))
			assert.Equal(t, tc.wantArgs, args)
		})
	}

	t.Run("set and notSet on a dimension without NullableColumn are unchanged", func(t *testing.T) {
		q := Query{Schema: "web_sessions", Measures: []string{"sessions"},
			Filters: []Filter{{Member: "channel", Operator: "set"}}}
		sqlStr, _ := buildSQL(t, q, timeSemanticsSchema())
		assert.Equal(t, "WHERE channel IS NOT NULL", whereOf(t, sqlStr))
	})
}

// A period with no row gives counts and sums 0, and a number measure (a median, a rate) no value.
func TestGapFilledNumberMeasuresAreNull(t *testing.T) {
	q := Query{Schema: "web_sessions", Measures: []string{"sessions", "pageviews", "bounce_rate"},
		TimeDimensions: []TimeDimension{{Dimension: "created_at", Granularity: "day",
			DateRange: &[2]string{"2026-08-01", "2026-08-02"}}}}
	data := []map[string]interface{}{
		{"created_at_day": "2026-08-01T00:00:00Z", "sessions": int64(4), "pageviews": int64(9), "bounce_rate": "25.00"},
	}
	result, err := fillTimeSeriesGaps(data, q, timeSemanticsSchema().MeasureTypes())
	require.NoError(t, err)
	require.Len(t, result, 2)
	assert.Equal(t, data[0], result[0])
	assert.Equal(t, map[string]interface{}{
		"created_at_day": "2026-08-02T00:00:00Z", "sessions": 0, "pageviews": 0, "bounce_rate": nil,
	}, result[1])
}

// Hourly buckets are wall-clock hours of the query's timezone: the hour the clocks skip has no
// period, and the hour they repeat is one period.
func TestHourlyGapFillingAcrossDaylightSaving(t *testing.T) {
	paris := "Europe/Paris"
	hourly := func(day string) Query {
		return Query{Schema: "web_sessions", Measures: []string{"sessions"}, Timezone: &paris,
			TimeDimensions: []TimeDimension{{Dimension: "created_at", Granularity: "hour",
				DateRange: &[2]string{day, day}}}}
	}
	labels := func(rows []map[string]interface{}) []string {
		out := make([]string, 0, len(rows))
		for _, row := range rows {
			out = append(out, row["created_at_hour"].(string))
		}
		return out
	}

	t.Run("the day the clocks go forward has no 02:00", func(t *testing.T) {
		// A single day with data takes the data-driven path; from 01:00 to 03:00 it would step
		// through 02:00.
		data := []map[string]interface{}{
			{"created_at_hour": "2026-03-29T01:00:00Z", "sessions": int64(1)},
			{"created_at_hour": "2026-03-29T03:00:00Z", "sessions": int64(2)},
		}
		result, err := fillTimeSeriesGaps(data, hourly("2026-03-29"), nil)
		require.NoError(t, err)
		assert.Equal(t, []string{"2026-03-29T01:00:00Z", "2026-03-29T03:00:00Z"}, labels(result))
	})

	t.Run("over several days the skipped hour is left out too", func(t *testing.T) {
		q := hourly("2026-03-28")
		q.TimeDimensions[0].DateRange = &[2]string{"2026-03-28", "2026-03-29"}
		result, err := fillTimeSeriesGaps(nil, q, nil)
		require.NoError(t, err)
		got := labels(result)
		assert.Len(t, got, 47, "24 hours, then 23")
		assert.NotContains(t, got, "2026-03-29T02:00:00Z")
		assert.Contains(t, got, "2026-03-29T01:00:00Z")
		assert.Contains(t, got, "2026-03-29T03:00:00Z")
	})

	t.Run("the day the clocks go back has one 02:00, holding both hours", func(t *testing.T) {
		// The database groups both 02:00 hours in the bucket labeled 02:00.
		data := []map[string]interface{}{
			{"created_at_hour": "2026-10-25T02:00:00Z", "sessions": int64(5)},
		}
		q := hourly("2026-10-24")
		q.TimeDimensions[0].DateRange = &[2]string{"2026-10-24", "2026-10-25"}
		result, err := fillTimeSeriesGaps(data, q, nil)
		require.NoError(t, err)
		got := labels(result)
		assert.Len(t, got, 48, "24 wall-clock hours each day")
		count := 0
		for _, row := range result {
			if row["created_at_hour"] == "2026-10-25T02:00:00Z" {
				count++
				assert.Equal(t, int64(5), row["sessions"])
			}
		}
		assert.Equal(t, 1, count)
	})

	t.Run("a label the data holds is kept", func(t *testing.T) {
		data := []map[string]interface{}{{"created_at_hour": "2026-03-29T02:00:00Z", "sessions": int64(1)}}
		q := hourly("2026-03-28")
		q.TimeDimensions[0].DateRange = &[2]string{"2026-03-28", "2026-03-29"}
		result, err := fillTimeSeriesGaps(data, q, nil)
		require.NoError(t, err)
		assert.Contains(t, labels(result), "2026-03-29T02:00:00Z")
	})

	t.Run("a single day with no data has each of its local hours", func(t *testing.T) {
		for day, want := range map[string]int{"2026-08-08": 24, "2026-03-29": 23, "2026-10-25": 24} {
			result, err := fillTimeSeriesGaps(nil, hourly(day), nil)
			require.NoError(t, err)
			got := labels(result)
			assert.Len(t, got, want, day)
			assert.Equal(t, day+"T00:00:00Z", got[0], day)
			assert.Equal(t, day+"T23:00:00Z", got[len(got)-1], day)
		}
		result, err := fillTimeSeriesGaps(nil, hourly("2026-03-29"), nil)
		require.NoError(t, err)
		assert.NotContains(t, labels(result), "2026-03-29T02:00:00Z")
	})

	t.Run("UTC has every hour", func(t *testing.T) {
		q := hourly("2026-03-28")
		q.TimeDimensions[0].DateRange = &[2]string{"2026-03-28", "2026-03-29"}
		utc := "UTC"
		q.Timezone = &utc
		result, err := fillTimeSeriesGaps(nil, q, nil)
		require.NoError(t, err)
		assert.Len(t, result, 48)
		assert.Contains(t, labels(result), "2026-03-29T02:00:00Z")
	})
}

func TestLocalHourExists(t *testing.T) {
	paris, err := time.LoadLocation("Europe/Paris")
	require.NoError(t, err)
	lordHowe, err := time.LoadLocation("Australia/Lord_Howe")
	require.NoError(t, err)

	assert.False(t, localHourExists(time.Date(2026, 3, 29, 2, 0, 0, 0, time.UTC), paris), "skipped")
	assert.True(t, localHourExists(time.Date(2026, 3, 29, 3, 0, 0, 0, time.UTC), paris))
	assert.True(t, localHourExists(time.Date(2026, 10, 25, 2, 0, 0, 0, time.UTC), paris), "repeated")
	// Lord Howe moves its clocks by half an hour: on 2026-10-04 they go from 02:00 to 02:30, so
	// the hour 02:00 still holds half an hour of wall clock.
	assert.True(t, localHourExists(time.Date(2026, 10, 4, 2, 0, 0, 0, time.UTC), lordHowe))
}

// A measure's Cast applies to its aggregate after its filters, so that a FILTER clause still
// follows the aggregate, in SELECT, HAVING and ORDER BY alike.
func TestMeasureCastWrapsTheFilteredAggregate(t *testing.T) {
	schema := SchemaDefinition{
		Name: "web_goals",
		Measures: map[string]MeasureDefinition{
			"purchase_value": {Type: "sum", SQL: "goal_value::text::numeric", Cast: "float8",
				Filters: []MeasureFilter{{SQL: "goal_type = 'purchase'"}}},
		},
		Dimensions: map[string]DimensionDefinition{"goal_name": {Type: "string", SQL: "goal_name"}},
	}
	q := Query{Schema: "web_goals", Measures: []string{"purchase_value"}, Dimensions: []string{"goal_name"},
		Having: []Filter{{Member: "purchase_value", Operator: "gt", Values: []string{"10"}}},
		Order:  map[string]string{"purchase_value": "desc"}}
	sqlStr, args := buildSQL(t, q, schema)
	aggregate := "(SUM(goal_value::text::numeric) FILTER (WHERE goal_type = 'purchase'))::float8"
	assert.Equal(t, "SELECT ("+aggregate+") AS purchase_value, goal_name AS goal_name FROM web_goals GROUP BY goal_name"+
		" HAVING "+aggregate+" > $1 ORDER BY "+aggregate+" DESC NULLS LAST", sqlStr)
	assert.Equal(t, []interface{}{"10"}, args)
}
