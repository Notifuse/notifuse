package analytics

import (
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func extensionTestSchema(hint *PartitionHint) SchemaDefinition {
	return SchemaDefinition{
		Name: "web_sessions",
		Measures: map[string]MeasureDefinition{
			"sessions": {Type: "count", SQL: "COUNT(*)"},
			"bounces": {Type: "count", SQL: "*",
				Filters: []MeasureFilter{{SQL: "duration_ms < 10000"}}},
			"median_duration": {Type: "number", SQL: "ROUND((PERCENTILE_CONT(0.5) WITHIN GROUP (ORDER BY duration_ms))::numeric / 1000.0, 1)"},
		},
		Dimensions: map[string]DimensionDefinition{
			"created_at": {Type: "time", SQL: "created_at"},
			"channel":    {Type: "string", SQL: "channel"},
		},
		PartitionHint: hint,
	}
}

func buildSQL(t *testing.T, query Query, schema SchemaDefinition) (string, []interface{}) {
	t.Helper()
	sqlStr, args, err := NewSQLBuilder().BuildSQL(query, schema)
	require.NoError(t, err)
	return sqlStr, args
}

func TestSargableTimeRanges(t *testing.T) {
	tz := "Europe/Paris"

	t.Run("non-UTC bounds become UTC instants; no AT TIME ZONE in WHERE", func(t *testing.T) {
		query := Query{
			Schema:   "web_sessions",
			Measures: []string{"sessions"},
			Timezone: &tz,
			TimeDimensions: []TimeDimension{{
				Dimension: "created_at", Granularity: "day",
				DateRange: &[2]string{"2026-08-01", "2026-08-08"},
			}},
		}
		sqlStr, args := buildSQL(t, query, extensionTestSchema(nil))

		// Bucketing keeps the timezone conversion; the WHERE must not.
		assert.Contains(t, sqlStr, "DATE_TRUNC('day', created_at AT TIME ZONE 'Europe/Paris')")
		whereClause := sqlStr[strings.Index(sqlStr, "WHERE"):]
		assert.NotContains(t, whereClause, "AT TIME ZONE", "range filter must stay sargable")
		assert.Contains(t, whereClause, "created_at >= $")
		assert.Contains(t, whereClause, "created_at <= $")

		// Paris midnight of Aug 1 (UTC+2) is 22:00 UTC on Jul 31, and the
		// range covers all of Aug 8 in Paris.
		require.Len(t, args, 2)
		start, ok := args[0].(time.Time)
		require.True(t, ok, "bounds must be bound as instants")
		assert.Equal(t, time.Date(2026, 7, 31, 22, 0, 0, 0, time.UTC), start.UTC())
		end := args[1].(time.Time)
		assert.Equal(t, time.Date(2026, 8, 8, 21, 59, 59, int(999*time.Millisecond), time.UTC), end.UTC())
	})

	t.Run("DST boundary is handled by the location database", func(t *testing.T) {
		query := Query{
			Schema:   "web_sessions",
			Measures: []string{"sessions"},
			Timezone: &tz,
			TimeDimensions: []TimeDimension{{
				Dimension: "created_at", Granularity: "day",
				// Paris switches to CEST on 2026-03-29.
				DateRange: &[2]string{"2026-03-28", "2026-03-30"},
			}},
		}
		_, args := buildSQL(t, query, extensionTestSchema(nil))
		start := args[0].(time.Time).UTC()
		end := args[1].(time.Time).UTC()
		assert.Equal(t, time.Date(2026, 3, 27, 23, 0, 0, 0, time.UTC), start, "before the switch: UTC+1")
		assert.Equal(t, time.Date(2026, 3, 30, 21, 59, 59, int(999*time.Millisecond), time.UTC), end,
			"the end of Mar 30, after the switch: UTC+2")
	})

	t.Run("a bare end date covers its whole day", func(t *testing.T) {
		// Otherwise a report ending today shows nothing for today, and a
		// single-day range comes back empty — and the caller cannot say "end
		// of that day" itself, because the gap filler only parses bare dates.
		query := Query{
			Schema:   "web_sessions",
			Measures: []string{"sessions"},
			TimeDimensions: []TimeDimension{{
				Dimension: "created_at", Granularity: "day",
				DateRange: &[2]string{"2026-08-01", "2026-08-08"},
			}},
		}
		sqlStr, args := buildSQL(t, query, extensionTestSchema(nil))
		// The bucket is pinned to UTC rather than left to the database
		// session's zone; only the WHERE clause stays free of AT TIME ZONE.
		assert.Contains(t, sqlStr, "DATE_TRUNC('day', created_at AT TIME ZONE 'UTC')")
		whereClause := sqlStr[strings.Index(sqlStr, "WHERE"):]
		assert.NotContains(t, whereClause, "AT TIME ZONE")
		assert.Equal(t, []interface{}{
			time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
			time.Date(2026, 8, 8, 23, 59, 59, int(999*time.Millisecond), time.UTC),
		}, args)
	})

	t.Run("an explicit instant is taken as given, not widened", func(t *testing.T) {
		query := Query{
			Schema:   "web_sessions",
			Measures: []string{"sessions"},
			TimeDimensions: []TimeDimension{{
				Dimension: "created_at", Granularity: "hour",
				DateRange: &[2]string{"2026-08-01T00:00:00Z", "2026-08-01T12:00:00Z"},
			}},
		}
		_, args := buildSQL(t, query, extensionTestSchema(nil))
		assert.Equal(t, time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC), args[1].(time.Time).UTC())
	})

	t.Run("unparseable bounds fall back to the legacy wrapped comparison", func(t *testing.T) {
		query := Query{
			Schema:   "web_sessions",
			Measures: []string{"sessions"},
			Timezone: &tz,
			TimeDimensions: []TimeDimension{{
				Dimension: "created_at", Granularity: "day",
				DateRange: &[2]string{"last tuesday", "now"},
			}},
		}
		sqlStr, args := buildSQL(t, query, extensionTestSchema(nil))
		whereClause := sqlStr[strings.Index(sqlStr, "WHERE"):]
		assert.Contains(t, whereClause, "created_at AT TIME ZONE 'Europe/Paris' >= $")
		assert.Equal(t, []interface{}{"last tuesday", "now"}, args)
	})

	t.Run("datetime and RFC3339 bounds parse too", func(t *testing.T) {
		query := Query{
			Schema:   "web_sessions",
			Measures: []string{"sessions"},
			Timezone: &tz,
			TimeDimensions: []TimeDimension{{
				Dimension: "created_at", Granularity: "hour",
				DateRange: &[2]string{"2026-08-01 12:30:00", "2026-08-01T18:00:00+02:00"},
			}},
		}
		_, args := buildSQL(t, query, extensionTestSchema(nil))
		assert.Equal(t, time.Date(2026, 8, 1, 10, 30, 0, 0, time.UTC), args[0].(time.Time).UTC())
		assert.Equal(t, time.Date(2026, 8, 1, 16, 0, 0, 0, time.UTC), args[1].(time.Time).UTC())
	})
}

func TestHavingMetricFilters(t *testing.T) {
	t.Run("renders HAVING on the aggregated measure", func(t *testing.T) {
		query := Query{
			Schema:     "web_sessions",
			Measures:   []string{"sessions"},
			Dimensions: []string{"channel"},
			Having:     []Filter{{Member: "sessions", Operator: "gte", Values: []string{"10"}}},
		}
		sqlStr, args := buildSQL(t, query, extensionTestSchema(nil))
		assert.Contains(t, sqlStr, "HAVING COUNT(*) >= $")
		assert.Contains(t, args, "10")
	})

	t.Run("measure filters survive inside HAVING", func(t *testing.T) {
		query := Query{
			Schema:     "web_sessions",
			Measures:   []string{"bounces"},
			Dimensions: []string{"channel"},
			Having:     []Filter{{Member: "bounces", Operator: "gt", Values: []string{"5"}}},
		}
		sqlStr, _ := buildSQL(t, query, extensionTestSchema(nil))
		assert.Contains(t, sqlStr, "HAVING COUNT(*) FILTER (WHERE duration_ms < 10000) > $")
	})

	t.Run("unknown member fails the build", func(t *testing.T) {
		query := Query{
			Schema:   "web_sessions",
			Measures: []string{"sessions"},
			Having:   []Filter{{Member: "nope", Operator: "gt", Values: []string{"5"}}},
		}
		_, _, err := NewSQLBuilder().BuildSQL(query, extensionTestSchema(nil))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "having member")
	})

	t.Run("validation rejects dimensions and bad operators in having", func(t *testing.T) {
		schemas := map[string]SchemaDefinition{"web_sessions": extensionTestSchema(nil)}

		query := Query{Schema: "web_sessions", Measures: []string{"sessions"},
			Having: []Filter{{Member: "channel", Operator: "gt", Values: []string{"5"}}}}
		assert.ErrorContains(t, DefaultValidate(query, schemas), "must be a measure")

		query.Having = []Filter{{Member: "sessions", Operator: "contains", Values: []string{"5"}}}
		assert.ErrorIs(t, DefaultValidate(query, schemas), ErrUnsupportedOperator)

		query.Having = []Filter{{Member: "sessions", Operator: "gt"}}
		assert.ErrorContains(t, DefaultValidate(query, schemas), "values cannot be empty")

		query.Having = []Filter{{Member: "sessions", Operator: "gte", Values: []string{"10"}}}
		assert.NoError(t, DefaultValidate(query, schemas))
	})
}

func TestPartitionHintPruning(t *testing.T) {
	hint := &PartitionHint{Column: "session_date", SlackBefore: 48 * time.Hour, SlackAfter: 48 * time.Hour}

	t.Run("dateRange widens into partition key bounds", func(t *testing.T) {
		query := Query{
			Schema:   "web_sessions",
			Measures: []string{"sessions"},
			TimeDimensions: []TimeDimension{{
				Dimension: "created_at", Granularity: "day",
				DateRange: &[2]string{"2026-08-05", "2026-08-08"},
			}},
		}
		sqlStr, args := buildSQL(t, query, extensionTestSchema(hint))
		assert.Contains(t, sqlStr, "session_date >= $")
		assert.Contains(t, sqlStr, "session_date <= $")
		assert.Contains(t, args, "2026-08-03", "start widened by the slack")
		assert.Contains(t, args, "2026-08-10", "end widened by the slack")
	})

	t.Run("non-UTC ranges prune too", func(t *testing.T) {
		tz := "America/New_York"
		query := Query{
			Schema:   "web_sessions",
			Measures: []string{"sessions"},
			Timezone: &tz,
			TimeDimensions: []TimeDimension{{
				Dimension: "created_at", Granularity: "day",
				DateRange: &[2]string{"2026-08-05", "2026-08-08"},
			}},
		}
		sqlStr, args := buildSQL(t, query, extensionTestSchema(hint))
		assert.Contains(t, sqlStr, "session_date >= $")
		// NY midnight Aug 5 = 04:00 UTC Aug 5; minus 48h = Aug 3.
		assert.Contains(t, args, "2026-08-03")
	})

	t.Run("an inDateRange filter prunes like a time dimension", func(t *testing.T) {
		// Dashboard breakdowns group by a dimension, so they cannot bucket the
		// range into a time dimension; the range rides a plain filter instead
		// and must still reach the partition key.
		query := Query{
			Schema:     "web_sessions",
			Measures:   []string{"sessions"},
			Dimensions: []string{"channel"},
			Filters: []Filter{{
				Member:   "created_at",
				Operator: "inDateRange",
				Values:   []string{"2026-08-05T00:00:00.000Z", "2026-08-08T23:59:59.999Z"},
			}},
		}
		sqlStr, args := buildSQL(t, query, extensionTestSchema(hint))
		assert.Contains(t, sqlStr, "session_date >= $")
		assert.Contains(t, sqlStr, "session_date <= $")
		assert.Contains(t, args, "2026-08-03", "start widened by the slack")
		assert.Contains(t, args, "2026-08-10", "end widened by the slack")
	})

	t.Run("only time dimensions prune, and only for range operators", func(t *testing.T) {
		query := Query{
			Schema:     "web_sessions",
			Measures:   []string{"sessions"},
			Dimensions: []string{"channel"},
			Filters: []Filter{
				// A date-looking range over a text column says nothing about
				// when the rows were written, so it must not bound partitions.
				{Member: "channel", Operator: "inDateRange", Values: []string{"2026-08-05", "2026-08-08"}},
				{Member: "created_at", Operator: "afterDate", Values: []string{"2026-08-05"}},
			},
		}
		sqlStr, _ := buildSQL(t, query, extensionTestSchema(hint))
		assert.NotContains(t, sqlStr, "session_date")
	})

	t.Run("unparseable filter bounds leave the query unpruned rather than wrong", func(t *testing.T) {
		query := Query{
			Schema:   "web_sessions",
			Measures: []string{"sessions"},
			Filters: []Filter{{
				Member:   "created_at",
				Operator: "inDateRange",
				Values:   []string{"last monday", "today"},
			}},
		}
		sqlStr, _ := buildSQL(t, query, extensionTestSchema(hint))
		assert.NotContains(t, sqlStr, "session_date")
	})

	t.Run("no hint, no extra predicate; no dateRange, no pruning clause", func(t *testing.T) {
		query := Query{
			Schema:   "web_sessions",
			Measures: []string{"sessions"},
			TimeDimensions: []TimeDimension{{
				Dimension: "created_at", Granularity: "day",
				DateRange: &[2]string{"2026-08-05", "2026-08-08"},
			}},
		}
		sqlStr, _ := buildSQL(t, query, extensionTestSchema(nil))
		assert.NotContains(t, sqlStr, "session_date")

		query.TimeDimensions = []TimeDimension{{Dimension: "created_at", Granularity: "day"}}
		sqlStr, _ = buildSQL(t, query, extensionTestSchema(hint))
		assert.NotContains(t, sqlStr, "session_date")
	})
}

func TestEmptyResultShape(t *testing.T) {
	// ProcessRows needs real *sql.Rows, so these go through the same helper the
	// repository uses, with sqlmock standing in for the database.
	run := func(t *testing.T, query Query, columns []string) []map[string]interface{} {
		t.Helper()
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mock.ExpectQuery("SELECT").WillReturnRows(sqlmock.NewRows(columns))
		rows, err := db.Query("SELECT 1")
		require.NoError(t, err)
		defer func() { _ = rows.Close() }()

		data, err := ProcessRows(rows, query)
		require.NoError(t, err)
		return data
	}

	t.Run("a totals query with no matching rows still answers zero", func(t *testing.T) {
		// A KPI tile has to render a number, and "no sessions this week" is a
		// number.
		data := run(t, Query{Measures: []string{"sessions", "bounce_rate"}}, []string{"sessions", "bounce_rate"})
		require.Len(t, data, 1)
		assert.Equal(t, 0, data[0]["sessions"])
		assert.Equal(t, 0, data[0]["bounce_rate"])
	})

	t.Run("a breakdown with no matching rows is empty, not one phantom row", func(t *testing.T) {
		// Filling it in would put a row with an empty dimension and zero
		// measures at the top of every table, which reads as real data and
		// suppresses the widget's own empty state.
		data := run(t, Query{
			Measures:   []string{"sessions"},
			Dimensions: []string{"channel"},
		}, []string{"channel", "sessions"})
		assert.Empty(t, data)
	})
}

func TestTimeBucketsSerializeInUTC(t *testing.T) {
	// The database session's timezone is not the query's timezone. A bucket
	// that keeps the server's offset no longer matches the keys the gap filler
	// generates, and every populated bucket is silently replaced by a zero.
	paris, err := time.LoadLocation("Europe/Paris")
	require.NoError(t, err)

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	bucket := time.Date(2026, 8, 9, 0, 0, 0, 0, paris)
	mock.ExpectQuery("SELECT").WillReturnRows(
		sqlmock.NewRows([]string{"created_at_day", "sessions"}).AddRow(bucket, 42),
	)
	rows, err := db.Query("SELECT 1")
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()

	data, err := ScanRows(rows)
	require.NoError(t, err)
	require.Len(t, data, 1)
	assert.Equal(t, "2026-08-08T22:00:00Z", data[0]["created_at_day"],
		"the same instant, expressed the way the gap filler keys it")
}

func TestGapFillKeepsRealBuckets(t *testing.T) {
	// The regression this guards: a populated day arriving with a non-UTC
	// offset used to be dropped and re-emitted as zero.
	paris, err := time.LoadLocation("Europe/Paris")
	require.NoError(t, err)

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	mock.ExpectQuery("SELECT").WillReturnRows(
		sqlmock.NewRows([]string{"created_at_day", "sessions"}).
			AddRow(time.Date(2026, 8, 8, 2, 0, 0, 0, paris), 7).
			AddRow(time.Date(2026, 8, 9, 2, 0, 0, 0, paris), 9),
	)
	rows, err := db.Query("SELECT 1")
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()

	data, err := ProcessRows(rows, Query{
		Measures: []string{"sessions"},
		TimeDimensions: []TimeDimension{{
			Dimension: "created_at", Granularity: "day",
			DateRange: &[2]string{"2026-08-08", "2026-08-09"},
		}},
	})
	require.NoError(t, err)

	total := 0
	for _, row := range data {
		switch v := row["sessions"].(type) {
		case int:
			total += v
		case int64:
			total += int(v)
		}
	}
	assert.Equal(t, 16, total, "both populated days survived gap filling")
}

func TestBucketsArePinnedToTheQueryTimezone(t *testing.T) {
	// DATE_TRUNC on a timestamptz truncates in the database session's zone.
	// Leaving UTC unwrapped therefore means "group by the server's days", and
	// on a server set to anything else the buckets land off UTC midnight,
	// miss the keys the gap filler generates, and the whole series zeroes out.
	query := Query{
		Schema:   "web_sessions",
		Measures: []string{"sessions"},
		TimeDimensions: []TimeDimension{{
			Dimension: "created_at", Granularity: "day",
			DateRange: &[2]string{"2026-08-01", "2026-08-08"},
		}},
	}
	sqlStr, _ := buildSQL(t, query, extensionTestSchema(nil))
	assert.Contains(t, sqlStr, "DATE_TRUNC('day', created_at AT TIME ZONE 'UTC')",
		"an unqualified UTC query must still say UTC out loud")

	tz := "America/Los_Angeles"
	query.Timezone = &tz
	sqlStr, _ = buildSQL(t, query, extensionTestSchema(nil))
	assert.Contains(t, sqlStr, "DATE_TRUNC('day', created_at AT TIME ZONE 'America/Los_Angeles')")
}
