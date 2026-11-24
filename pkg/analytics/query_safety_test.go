package analytics

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func safetySchema() SchemaDefinition {
	return SchemaDefinition{
		Name: "web_sessions",
		Measures: map[string]MeasureDefinition{
			"sessions":        {Type: "count", SQL: "COUNT(*)"},
			"pageviews":       {Type: "sum", SQL: "pageview_count"},
			"median_duration": {Type: "number", SQL: "ROUND((PERCENTILE_CONT(0.5) WITHIN GROUP (ORDER BY duration_ms))::numeric / 1000.0, 1)"},
			"last_seen":       {Type: "max", SQL: "updated_at"},
		},
		Dimensions: map[string]DimensionDefinition{
			"created_at":   {Type: "time", SQL: "created_at"},
			"updated_at":   {Type: "time", SQL: "updated_at"},
			"channel":      {Type: "string", SQL: "channel"},
			"landing_path": {Type: "string", SQL: "landing_path"},
			"duration":     {Type: "number", SQL: "duration_ms"},
		},
	}
}

func safetySchemas() map[string]SchemaDefinition {
	return map[string]SchemaDefinition{"web_sessions": safetySchema()}
}

// requireQueryError asserts a refusal the caller can act on: a *QueryError whose message
// contains want.
func requireQueryError(t *testing.T, err error, want string) {
	t.Helper()
	require.Error(t, err)
	var queryErr *QueryError
	require.True(t, errors.As(err, &queryErr), "want a *QueryError, got %T: %v", err, err)
	assert.Contains(t, queryErr.Message, want)
}

func dailySeries(from, to string) []TimeDimension {
	return []TimeDimension{{Dimension: "created_at", Granularity: "day", DateRange: &[2]string{from, to}}}
}

func TestGapFillPeriodCap(t *testing.T) {
	t.Run("a range of centuries by the hour is refused before any period is built", func(t *testing.T) {
		query := Query{Schema: "web_sessions", Measures: []string{"sessions"},
			TimeDimensions: []TimeDimension{{Dimension: "created_at", Granularity: "hour",
				DateRange: &[2]string{"0001-01-01", "9999-12-31"}}}}
		requireQueryError(t, DefaultValidate(query, safetySchemas()), "more than the 50000 a series may have")
	})

	t.Run("the cap counts periods exactly", func(t *testing.T) {
		start := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
		atCap := start.Add(time.Duration(maxGapFilledRows-1) * time.Hour)
		query := Query{Schema: "web_sessions", Measures: []string{"sessions"},
			TimeDimensions: []TimeDimension{{Dimension: "created_at", Granularity: "hour",
				DateRange: &[2]string{start.Format(time.RFC3339), atCap.Format(time.RFC3339)}}}}
		assert.NoError(t, DefaultValidate(query, safetySchemas()))

		query.TimeDimensions[0].DateRange[1] = atCap.Add(time.Hour).Format(time.RFC3339)
		requireQueryError(t, DefaultValidate(query, safetySchemas()), "50001 periods")
	})

	t.Run("the filler refuses a range past the cap on its own", func(t *testing.T) {
		start := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
		query := Query{Measures: []string{"sessions"},
			TimeDimensions: []TimeDimension{{Dimension: "created_at", Granularity: "hour",
				DateRange: &[2]string{start.Format(time.RFC3339), start.Add(maxGapFilledRows * time.Hour).Format(time.RFC3339)}}}}
		_, err := fillTimeSeriesGaps(nil, query, nil)
		requireQueryError(t, err, "more than 50000 periods")
	})

	t.Run("countPeriods matches the range the filler builds", func(t *testing.T) {
		cases := []struct{ from, to time.Time }{
			{time.Date(2026, 1, 1, 5, 0, 0, 0, time.UTC), time.Date(2026, 1, 3, 7, 0, 0, 0, time.UTC)},
			{time.Date(2025, 12, 31, 0, 0, 0, 0, time.UTC), time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)},
			{time.Date(2026, 11, 22, 0, 0, 0, 0, time.UTC), time.Date(2026, 11, 24, 0, 0, 0, 0, time.UTC)}, // Sunday to Tuesday
			{time.Date(2019, 6, 15, 0, 0, 0, 0, time.UTC), time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)},
		}
		for _, granularity := range validGranularities {
			for _, c := range cases {
				assert.Equal(t, int64(len(generateTimeRange(c.from, c.to, granularity))), countPeriods(c.from, c.to, granularity),
					"%s from %s to %s", granularity, c.from, c.to)
			}
		}
	})

	t.Run("a range that ends before it starts, or does not parse, is refused", func(t *testing.T) {
		query := Query{Schema: "web_sessions", Measures: []string{"sessions"}, TimeDimensions: dailySeries("2026-02-01", "2026-01-01")}
		requireQueryError(t, DefaultValidate(query, safetySchemas()), "ends before it starts")

		query.TimeDimensions = dailySeries("last week", "2026-01-01")
		requireQueryError(t, DefaultValidate(query, safetySchemas()), `dateRange bound "last week"`)
	})
}

// runQuery runs a query through Query.Query against sqlmock and returns the SQL it ran.
func runQuery(t *testing.T, query Query, columns []string, rows [][]driver.Value) ([]map[string]interface{}, string, error) {
	t.Helper()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	result := sqlmock.NewRows(columns)
	for _, row := range rows {
		result.AddRow(row...)
	}
	mock.ExpectQuery(".*").WillReturnRows(result)

	recorder := &recordingQuerier{db: db}
	response, err := query.Query(context.Background(), recorder, safetySchema())
	if err != nil {
		return nil, recorder.ran, err
	}
	return response.Data, recorder.ran, nil
}

// recordingQuerier keeps the SQL a query ran.
type recordingQuerier struct {
	db  *sql.DB
	ran string
}

func (r *recordingQuerier) QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	r.ran = query
	return r.db.QueryContext(ctx, query, args...)
}

func TestGapFilledOrderOffsetLimit(t *testing.T) {
	// Four days, data on two of them.
	base := Query{Schema: "web_sessions", Measures: []string{"sessions"}, TimeDimensions: dailySeries("2026-08-01", "2026-08-04")}
	rows := [][]driver.Value{
		{time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC), int64(5)},
		{time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC), int64(9)},
	}
	columns := []string{"created_at_day", "sessions"}
	days := func(data []map[string]interface{}) []string {
		out := make([]string, len(data))
		for i, row := range data {
			out[i] = fmt.Sprintf("%s=%v", row["created_at_day"].(string)[:10], row["sessions"])
		}
		return out
	}

	t.Run("the limit applies after the filling, in the requested order", func(t *testing.T) {
		query := base
		query.Order = map[string]string{"sessions": "desc"}
		query.Limit = intPtr(2)
		data, ran, err := runQuery(t, query, columns, rows)
		require.NoError(t, err)
		assert.Equal(t, []string{"2026-08-03=9", "2026-08-01=5"}, days(data))
		// The database returns the whole range: a row cut there would come back as a zero.
		assert.Contains(t, ran, fmt.Sprintf("LIMIT %d", MaxRows+1))
		assert.NotContains(t, ran, "LIMIT 2")
	})

	t.Run("the offset pages through the filled periods", func(t *testing.T) {
		query := base
		query.Order = map[string]string{"sessions": "desc"}
		query.Limit = intPtr(2)
		query.Offset = intPtr(2)
		data, ran, err := runQuery(t, query, columns, rows)
		require.NoError(t, err)
		// Equal on the measure, the filled days keep their period order.
		assert.Equal(t, []string{"2026-08-02=0", "2026-08-04=0"}, days(data))
		assert.NotContains(t, ran, "OFFSET")
	})

	t.Run("the time bucket orders a series, descending", func(t *testing.T) {
		query := base
		query.Order = map[string]string{"created_at": "desc"}
		data, ran, err := runQuery(t, query, columns, rows)
		require.NoError(t, err)
		assert.Equal(t, []string{"2026-08-04=0", "2026-08-03=9", "2026-08-02=0", "2026-08-01=5"}, days(data))
		assert.Contains(t, ran, "ORDER BY created_at_day DESC")
	})

	t.Run("a query without a date range keeps its limit in SQL", func(t *testing.T) {
		query := Query{Schema: "web_sessions", Measures: []string{"sessions"}, Dimensions: []string{"channel"}, Limit: intPtr(3), Offset: intPtr(1)}
		_, ran, err := runQuery(t, query, []string{"sessions", "channel"}, nil)
		require.NoError(t, err)
		assert.Contains(t, ran, "LIMIT 3 OFFSET 1")
	})

	t.Run("a gap-filled order needs its measure selected", func(t *testing.T) {
		query := base
		query.Order = map[string]string{"pageviews": "desc"}
		requireQueryError(t, DefaultValidate(query, safetySchemas()), `order by measure "pageviews" needs it in measures`)
	})
}

func TestSortRowsComparesAsTheDatabase(t *testing.T) {
	// A NUMERIC measure comes back as text: "10.5" sorts above "9.0" as a number would.
	data := []map[string]interface{}{
		{"median_duration": "9.0", "channel": "b"},
		{"median_duration": nil, "channel": "c"},
		{"median_duration": "10.5", "channel": "a"},
		{"median_duration": 0, "channel": "d"},
	}
	query := Query{Measures: []string{"median_duration"}, Dimensions: []string{"channel"},
		Order: map[string]string{"median_duration": "desc"}}
	sortRows(data, query, nil)
	var got []string
	for _, row := range data {
		got = append(got, row["channel"].(string))
	}
	// The null sorts last, as NULLS LAST does.
	assert.Equal(t, []string{"a", "b", "d", "c"}, got)
}

func TestRowBound(t *testing.T) {
	t.Run("a limit past MaxRows is refused", func(t *testing.T) {
		query := Query{Schema: "web_sessions", Measures: []string{"sessions"}, Limit: intPtr(MaxRows + 1)}
		requireQueryError(t, DefaultValidate(query, safetySchemas()), "limit must be between 0 and 50000")
	})

	t.Run("a query without a limit reads at most one row past MaxRows", func(t *testing.T) {
		query := Query{Schema: "web_sessions", Measures: []string{"sessions"}, Dimensions: []string{"channel"}}
		_, ran, err := runQuery(t, query, []string{"sessions", "channel"}, nil)
		require.NoError(t, err)
		assert.Contains(t, ran, fmt.Sprintf("LIMIT %d", MaxRows+1))
	})

	t.Run("a result past MaxRows without a limit is refused", func(t *testing.T) {
		query := Query{Schema: "web_sessions", Measures: []string{"sessions"}, Dimensions: []string{"channel"}}
		rows := make([][]driver.Value, MaxRows+1)
		for i := range rows {
			rows[i] = []driver.Value{int64(1), fmt.Sprintf("c%d", i)}
		}
		_, _, err := runQuery(t, query, []string{"sessions", "channel"}, rows)
		requireQueryError(t, err, "more than 50000 rows")
	})
}

func TestTypedValidation(t *testing.T) {
	schemas := safetySchemas()
	query := func(mutate func(*Query)) Query {
		q := Query{Schema: "web_sessions", Measures: []string{"sessions"}}
		mutate(&q)
		return q
	}

	cases := []struct {
		name  string
		query Query
		want  string
	}{
		{"a text dimension as the time dimension", query(func(q *Query) {
			q.TimeDimensions = []TimeDimension{{Dimension: "channel", Granularity: "day"}}
		}), `time dimension "channel" is a string dimension`},
		{"a LIKE on a number", query(func(q *Query) {
			q.Filters = []Filter{{Member: "duration", Operator: "contains", Values: []string{"5"}}}
		}), `operator "contains" does not apply to number dimension "duration"`},
		{"a date operator on text", query(func(q *Query) {
			q.Filters = []Filter{{Member: "channel", Operator: "inDateRange", Values: []string{"2026-01-01", "2026-02-01"}}}
		}), `operator "inDateRange" does not apply to string dimension "channel"`},
		{"a LIKE on a time", query(func(q *Query) {
			q.Filters = []Filter{{Member: "created_at", Operator: "startsWith", Values: []string{"2026"}}}
		}), `operator "startsWith" does not apply to time dimension "created_at"`},
		{"text compared with a number dimension", query(func(q *Query) {
			q.Filters = []Filter{{Member: "duration", Operator: "gt", Values: []string{"long"}}}
		}), `takes numbers, and "long" is not one`},
		{"text compared with a count", query(func(q *Query) {
			q.Having = []Filter{{Member: "sessions", Operator: "gt", Values: []string{"many"}}}
		}), `takes numbers, and "many" is not one`},
		{"two values for a single-value operator", query(func(q *Query) {
			q.Filters = []Filter{{Member: "duration", Operator: "gt", Values: []string{"1", "2"}}}
		}), "takes exactly one value, got 2"},
		{"one value for a range", query(func(q *Query) {
			q.Filters = []Filter{{Member: "created_at", Operator: "inDateRange", Values: []string{"2026-01-01"}}}
		}), "takes exactly two values, got 1"},
		{"nothing selected", Query{Schema: "web_sessions"}, "at least one measure or dimension"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			requireQueryError(t, DefaultValidate(c.query, schemas), c.want)
		})
	}

	t.Run("what each type takes is still accepted", func(t *testing.T) {
		accepted := query(func(q *Query) {
			q.Filters = []Filter{
				{Member: "channel", Operator: "contains", Values: []string{"mail"}},
				{Member: "channel", Operator: "gt", Values: []string{"a"}},
				{Member: "duration", Operator: "gte", Values: []string{"1.5"}},
				{Member: "duration", Operator: "in", Values: []string{"1", "2"}},
				{Member: "created_at", Operator: "inDateRange", Values: []string{"2026-01-01T00:00:00Z", "2026-02-01T00:00:00Z"}},
				{Member: "landing_path", Operator: "notSet"},
			}
			q.Having = []Filter{
				{Member: "sessions", Operator: "gte", Values: []string{"10"}},
				// A max of a timestamp compares with a timestamp.
				{Member: "last_seen", Operator: "gt", Values: []string{"2026-01-01"}},
			}
		})
		assert.NoError(t, DefaultValidate(accepted, schemas))
	})
}

func TestTimezoneLocalAndEmpty(t *testing.T) {
	schemas := safetySchemas()

	t.Run("Local is refused", func(t *testing.T) {
		for _, tz := range []string{"Local", "local"} {
			q := Query{Schema: "web_sessions", Measures: []string{"sessions"}, Timezone: &tz}
			err := DefaultValidate(q, schemas)
			requireQueryError(t, err, "is not an IANA time zone name")
			assert.ErrorIs(t, err, ErrInvalidTimezone)
		}
	})

	t.Run("an empty timezone means UTC, in the buckets too", func(t *testing.T) {
		empty := ""
		q := Query{Schema: "web_sessions", Measures: []string{"sessions"}, Timezone: &empty,
			TimeDimensions: dailySeries("2026-08-01", "2026-08-02")}
		require.NoError(t, DefaultValidate(q, schemas))
		assert.Equal(t, "UTC", q.GetDefaultTimezone())
		sqlStr, _ := buildSQL(t, q, safetySchema())
		assert.Contains(t, sqlStr, "DATE_TRUNC('day', created_at AT TIME ZONE 'UTC')")
	})
}

func TestMeasuresRefusedInFilters(t *testing.T) {
	query := Query{Schema: "web_sessions", Measures: []string{"sessions"},
		Filters: []Filter{{Member: "pageviews", Operator: "gt", Values: []string{"3"}}}}

	requireQueryError(t, DefaultValidate(query, safetySchemas()), `"pageviews" is a measure: put conditions on measures in having`)

	_, _, err := NewSQLBuilder().BuildSQL(query, safetySchema())
	requireQueryError(t, err, "put conditions on measures in having")
}

func TestLikeWildcardsAreEscaped(t *testing.T) {
	cases := []struct {
		operator, value, sql, pattern string
	}{
		{"contains", `50%_off\`, `landing_path LIKE $1 ESCAPE '\'`, `%50\%\_off\\%`},
		{"notContains", "spring_sale", `landing_path NOT LIKE $1 ESCAPE '\'`, `%spring\_sale%`},
		{"startsWith", "/promo_", `landing_path LIKE $1 ESCAPE '\'`, `/promo\_%`},
		{"notStartsWith", "%", `landing_path NOT LIKE $1 ESCAPE '\'`, `\%%`},
		{"endsWith", "_v1", `landing_path LIKE $1 ESCAPE '\'`, `%\_v1`},
		{"notEndsWith", "100%", `landing_path NOT LIKE $1 ESCAPE '\'`, `%100\%`},
	}
	for _, c := range cases {
		t.Run(c.operator, func(t *testing.T) {
			query := Query{Schema: "web_sessions", Measures: []string{"sessions"},
				Filters: []Filter{{Member: "landing_path", Operator: c.operator, Values: []string{c.value}}}}
			sqlStr, args := buildSQL(t, query, safetySchema())
			assert.Contains(t, sqlStr, "WHERE "+c.sql)
			assert.Equal(t, []interface{}{c.pattern}, args)
		})
	}
}

func TestOrderShapes(t *testing.T) {
	series := Query{Schema: "web_sessions", Measures: []string{"sessions"}, Dimensions: []string{"channel"},
		TimeDimensions: []TimeDimension{{Dimension: "created_at", Granularity: "day"}}}

	t.Run("the time dimension orders by its bucket, under either name", func(t *testing.T) {
		for _, field := range []string{"created_at", "created_at_day"} {
			q := series
			q.Order = map[string]string{field: "asc"}
			require.NoError(t, DefaultValidate(q, safetySchemas()))
			sqlStr, _ := buildSQL(t, q, safetySchema())
			assert.Contains(t, sqlStr, "ORDER BY created_at_day ASC", field)
		}
	})

	t.Run("a dimension the query does not group by is refused", func(t *testing.T) {
		q := series
		q.Order = map[string]string{"landing_path": "asc"}
		requireQueryError(t, DefaultValidate(q, safetySchemas()), `order by dimension "landing_path" needs it in dimensions`)
		_, _, err := NewSQLBuilder().BuildSQL(q, safetySchema())
		requireQueryError(t, err, `order by dimension "landing_path"`)
	})

	t.Run("several keys sort in the order the JSON object lists them, every time", func(t *testing.T) {
		var q Query
		require.NoError(t, json.Unmarshal([]byte(`{"schema":"web_sessions","measures":["sessions"],"dimensions":["channel"],
			"order":{"sessions":"desc","channel":"asc"}}`), &q))
		for i := 0; i < 50; i++ {
			sqlStr, _ := buildSQL(t, q, safetySchema())
			require.Contains(t, sqlStr, "ORDER BY COUNT(*) DESC NULLS LAST, channel ASC")
		}

		require.NoError(t, json.Unmarshal([]byte(`{"schema":"web_sessions","measures":["sessions"],"dimensions":["channel"],
			"order":{"channel":"asc","sessions":"desc"}}`), &q))
		sqlStr, _ := buildSQL(t, q, safetySchema())
		assert.Contains(t, sqlStr, "ORDER BY channel ASC, COUNT(*) DESC NULLS LAST")
	})

	t.Run("a query built in Go sorts its keys by name", func(t *testing.T) {
		q := Query{Schema: "web_sessions", Measures: []string{"sessions"}, Dimensions: []string{"channel"},
			Order: map[string]string{"sessions": "desc", "channel": "asc"}}
		for i := 0; i < 50; i++ {
			sqlStr, _ := buildSQL(t, q, safetySchema())
			require.Contains(t, sqlStr, "ORDER BY channel ASC, COUNT(*) DESC NULLS LAST")
		}
	})
}

func TestOneTimeDimension(t *testing.T) {
	query := Query{Schema: "web_sessions", Measures: []string{"sessions"},
		TimeDimensions: []TimeDimension{
			{Dimension: "created_at", Granularity: "day", DateRange: &[2]string{"2026-08-01", "2026-08-04"}},
			{Dimension: "updated_at", Granularity: "hour"},
		}}
	requireQueryError(t, DefaultValidate(query, safetySchemas()), "at most one time dimension, got 2")
}

func TestBareEndDateIsTheWholeDay(t *testing.T) {
	losAngeles := "America/Los_Angeles"
	for _, granularity := range validGranularities {
		t.Run(granularity, func(t *testing.T) {
			series := func(from, to string) []TimeDimension {
				return []TimeDimension{{Dimension: "created_at", Granularity: granularity, DateRange: &[2]string{from, to}}}
			}
			query := Query{Schema: "web_sessions", Measures: []string{"sessions"}, TimeDimensions: series("2026-11-20 08:30:00", "2026-11-20")}
			assert.NoError(t, DefaultValidate(query, safetySchemas()), "a range from a time to the end of its day")

			// 08:30 UTC is 00:30 in Los Angeles, after the local midnight of the 20th.
			query.TimeDimensions = series("2026-11-20T08:30:00Z", "2026-11-20")
			query.Timezone = &losAngeles
			assert.NoError(t, DefaultValidate(query, safetySchemas()), "an instant that the timezone moves into the day")

			// 23:00 UTC on the 20th is the 21st in Tokyo: after the whole of the 20th there.
			tokyo := "Asia/Tokyo"
			query.Timezone = &tokyo
			query.TimeDimensions = series("2026-11-20T23:00:00Z", "2026-11-20")
			requireQueryError(t, DefaultValidate(query, safetySchemas()), "ends before it starts")

			query.Timezone = nil
			query.TimeDimensions = series("2026-11-21", "2026-11-20 23:59:59")
			requireQueryError(t, DefaultValidate(query, safetySchemas()), "ends before it starts")
		})
	}

	t.Run("the series holds the one day", func(t *testing.T) {
		query := Query{Measures: []string{"sessions"},
			TimeDimensions: []TimeDimension{{Dimension: "created_at", Granularity: "day", DateRange: &[2]string{"2026-11-20 08:30:00", "2026-11-20"}}}}
		filled, err := fillTimeSeriesGaps(nil, query, nil)
		require.NoError(t, err)
		require.Len(t, filled, 1)
		assert.Equal(t, "2026-11-20T00:00:00Z", filled[0]["created_at_day"])
	})
}

// The query the console's web analytics assistant sends for a grouped series: a dimension, a
// day granularity over a long range, a limit of 400, a measure descending.
func assistantSeries() Query {
	return Query{Schema: "web_sessions", Measures: []string{"sessions"}, Dimensions: []string{"landing_path"},
		TimeDimensions: dailySeries("2026-05-01", "2026-07-29"), Limit: intPtr(400),
		Order: map[string]string{"sessions": "desc"}}
}

func TestGroupedSeriesReadsTheTopGroupsOnly(t *testing.T) {
	t.Run("the SQL ranks the rows and keeps the groups of the first offset+limit", func(t *testing.T) {
		query := assistantSeries()
		query.Offset = intPtr(10)
		require.NoError(t, DefaultValidate(query, safetySchemas()))
		sqlStr, _ := buildSQL(t, query.executionQuery(), safetySchema())
		assert.Contains(t, sqlStr, "SELECT sessions, landing_path, created_at_day FROM (SELECT ranked.*, MIN(analytics_cell_rank) OVER (PARTITION BY landing_path) AS analytics_group_rank FROM (SELECT cells.*, ROW_NUMBER() OVER (ORDER BY sessions DESC NULLS LAST) AS analytics_cell_rank FROM (SELECT (COUNT(*)) AS sessions, landing_path AS landing_path,")
		assert.Contains(t, sqlStr, "GROUP BY landing_path, created_at_day) AS cells) AS ranked) AS top_groups WHERE analytics_group_rank <= 410 ORDER BY sessions DESC NULLS LAST LIMIT 50001")
		// The grouped rows are ranked whole: no limit or offset inside.
		assert.Equal(t, 1, strings.Count(sqlStr, "LIMIT"))
		assert.NotContains(t, sqlStr, "OFFSET")
	})

	t.Run("a series without a limit, or without a dimension, reads its whole range", func(t *testing.T) {
		query := assistantSeries()
		query.Limit = nil
		sqlStr, _ := buildSQL(t, query.executionQuery(), safetySchema())
		assert.NotContains(t, sqlStr, "analytics_group_rank")

		query = assistantSeries()
		query.Dimensions = nil
		sqlStr, _ = buildSQL(t, query.executionQuery(), safetySchema())
		assert.NotContains(t, sqlStr, "analytics_group_rank")
	})

	t.Run("the series of 400 groups over 90 days is served, sorted and limited", func(t *testing.T) {
		// What the database returns for more than 50,000 grouped rows (600 paths over 90 days):
		// the full series of the 400 groups that hold the first 400 rows, 36,000 rows.
		start := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
		var rows [][]driver.Value
		for group := 0; group < 400; group++ {
			for day := 0; day < 90; day++ {
				if group == 7 && day == 3 {
					continue // a day without sessions: the filling gives it a zero
				}
				rows = append(rows, []driver.Value{int64(1 + (group*90+day)%97), fmt.Sprintf("/p%03d", group), start.AddDate(0, 0, day)})
			}
		}
		data, ran, err := runQuery(t, assistantSeries(), []string{"sessions", "landing_path", "created_at_day"}, rows)
		require.NoError(t, err)
		assert.Contains(t, ran, "analytics_group_rank <= 400")
		require.Len(t, data, 400)
		want := make([]int64, len(rows))
		for i, row := range rows {
			want[i] = row[0].(int64)
		}
		slices.SortFunc(want, func(a, b int64) int { return int(b - a) })
		got := make([]int64, len(data))
		for i, row := range data {
			got[i] = row["sessions"].(int64)
		}
		assert.Equal(t, want[:400], got)
	})

	t.Run("when even the top groups' series pass MaxRows, the query is refused", func(t *testing.T) {
		rows := make([][]driver.Value, MaxRows+1)
		for i := range rows {
			rows[i] = []driver.Value{int64(1), fmt.Sprintf("/p%d", i/90), time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, i%90)}
		}
		_, _, err := runQuery(t, assistantSeries(), []string{"sessions", "landing_path", "created_at_day"}, rows)
		requireQueryError(t, err, "the series of the groups that hold the first 400 rows return more than 50000 rows")
	})

	t.Run("the page from the top groups is the page from every row", func(t *testing.T) {
		// 60 groups over 30 days with holes, every count distinct: the page must not depend on
		// the groups left out.
		query := Query{Schema: "web_sessions", Measures: []string{"sessions"}, Dimensions: []string{"landing_path"},
			TimeDimensions: dailySeries("2026-05-01", "2026-05-30"), Limit: intPtr(25), Offset: intPtr(5),
			Order: map[string]string{"sessions": "desc"}}
		start := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
		type cell struct {
			sessions int64
			path     string
			day      time.Time
		}
		var cells []cell
		for group := 0; group < 60; group++ {
			for day := 0; day < 30; day++ {
				if (group+day)%4 == 0 {
					continue
				}
				cells = append(cells, cell{int64((group*7919+day*104729)%100000 + 1), fmt.Sprintf("/p%02d", group), start.AddDate(0, 0, day)})
			}
		}
		toRows := func(cells []cell) [][]driver.Value {
			out := make([][]driver.Value, len(cells))
			for i, c := range cells {
				out[i] = []driver.Value{c.sessions, c.path, c.day}
			}
			return out
		}
		columns := []string{"sessions", "landing_path", "created_at_day"}

		// Every row, as a query without the ranking would read them.
		everyRow, _, err := runQuery(t, query, columns, toRows(cells))
		require.NoError(t, err)

		// The database's ranking: the groups of the first 30 rows by sessions, then their series.
		ranked := slices.Clone(cells)
		slices.SortFunc(ranked, func(a, b cell) int { return int(b.sessions - a.sessions) })
		kept := map[string]bool{}
		for _, c := range ranked[:30] {
			kept[c.path] = true
		}
		var series []cell
		for _, c := range cells {
			if kept[c.path] {
				series = append(series, c)
			}
		}
		require.Less(t, len(series), len(cells))
		topGroups, _, err := runQuery(t, query, columns, toRows(series))
		require.NoError(t, err)
		assert.Equal(t, everyRow, topGroups)
	})
}

func TestSortRowsByDimensionType(t *testing.T) {
	order := func(values []interface{}, query Query, types map[string]string) []interface{} {
		data := make([]map[string]interface{}, len(values))
		for i, v := range values {
			data[i] = map[string]interface{}{"value": v}
		}
		sortRows(data, query, types)
		out := make([]interface{}, len(data))
		for i, row := range data {
			out[i] = row["value"]
		}
		return out
	}

	t.Run("a number dimension returned as text sorts as numbers", func(t *testing.T) {
		query := Query{Dimensions: []string{"value"}, Order: map[string]string{"value": "asc"}}
		values := []interface{}{"100", "9", "25"}
		assert.Equal(t, []interface{}{"9", "25", "100"}, order(values, query, map[string]string{"value": "number"}))
		query.Order["value"] = "desc"
		assert.Equal(t, []interface{}{"100", "25", "9"}, order(values, query, map[string]string{"value": "number"}))
	})

	t.Run("text sorts by the Unicode collation, as a linguistic database collation does", func(t *testing.T) {
		query := Query{Dimensions: []string{"value"}, Order: map[string]string{"value": "asc"}}
		values := []interface{}{"éclair", "Banana", "apple"}
		assert.Equal(t, []interface{}{"apple", "Banana", "éclair"}, order(values, query, map[string]string{"value": "string"}))
		query.Order["value"] = "desc"
		assert.Equal(t, []interface{}{"éclair", "Banana", "apple"}, order(values, query, map[string]string{"value": "string"}))
	})

	t.Run("a text dimension that holds numbers still sorts as text", func(t *testing.T) {
		query := Query{Dimensions: []string{"value"}, Order: map[string]string{"value": "asc"}}
		assert.Equal(t, []interface{}{"100", "25", "9"}, order([]interface{}{"9", "100", "25"}, query, map[string]string{"value": "string"}))
	})
}

func TestOrderDirectionDefaultsToAscending(t *testing.T) {
	for _, direction := range []string{"", "up", "ASC", "Desc"} {
		t.Run(fmt.Sprintf("%q", direction), func(t *testing.T) {
			query := Query{Schema: "web_sessions", Measures: []string{"sessions"}, Dimensions: []string{"channel"},
				Order: map[string]string{"channel": direction}}
			require.NoError(t, DefaultValidate(query, safetySchemas()))
			sqlStr, _ := buildSQL(t, query, safetySchema())
			want, rows := "ORDER BY channel ASC", []interface{}{"a", "b", "c"}
			if strings.EqualFold(direction, "desc") {
				want, rows = "ORDER BY channel DESC", []interface{}{"c", "b", "a"}
			}
			assert.Contains(t, sqlStr, want)

			// A filled series sorts in Go the same way.
			query.TimeDimensions = dailySeries("2026-08-01", "2026-08-01")
			data := []map[string]interface{}{{"channel": "b"}, {"channel": "c"}, {"channel": "a"}}
			sortRows(data, query, nil)
			got := []interface{}{data[0]["channel"], data[1]["channel"], data[2]["channel"]}
			assert.Equal(t, rows, got)
		})
	}
}
