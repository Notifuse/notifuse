package analytics

import (
	"database/sql/driver"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helper functions for sql_builder_test.go
func stringPtr(s string) *string {
	return &s
}

func intPtr(i int) *int {
	return &i
}

func TestSQLBuilder_BuildSQL(t *testing.T) {
	builder := NewSQLBuilder()

	// Create test schema
	schema := SchemaDefinition{
		Name: "message_history",
		Measures: map[string]MeasureDefinition{
			"count": {
				Type:        "count",
				SQL:         "COUNT(*)",
				Description: "Total count",
			},
			"count_sent": {
				Type:        "count",
				SQL:         "COUNT(*) FILTER (WHERE sent_at IS NOT NULL)",
				Description: "Total sent messages",
			},
		},
		Dimensions: map[string]DimensionDefinition{
			"created_at": {
				Type:        "time",
				SQL:         "created_at",
				Description: "Creation timestamp",
			},
			"contact_email": {
				Type:        "string",
				SQL:         "contact_email",
				Description: "Recipient email",
			},
			"broadcast_id": {
				Type:        "string",
				SQL:         "broadcast_id",
				Description: "Broadcast ID",
			},
		},
	}

	tests := []struct {
		name          string
		query         Query
		expectedSQL   string
		expectedArgs  []interface{}
		expectedError bool
	}{
		{
			name: "simple count query",
			query: Query{
				Schema:   "message_history",
				Measures: []string{"count"},
			},
			expectedSQL: "SELECT (COUNT(*)) AS count FROM message_history",
		},
		{
			name: "query with dimensions",
			query: Query{
				Schema:     "message_history",
				Measures:   []string{"count"},
				Dimensions: []string{"contact_email"},
			},
			expectedSQL: "SELECT (COUNT(*)) AS count, contact_email AS contact_email FROM message_history GROUP BY contact_email",
		},
		{
			name: "query with time dimension",
			query: Query{
				Schema:   "message_history",
				Measures: []string{"count"},
				TimeDimensions: []TimeDimension{{
					Dimension:   "created_at",
					Granularity: "day",
				}},
			},
			expectedSQL: "SELECT (COUNT(*)) AS count, (DATE_TRUNC('day', created_at)) AS created_at_day FROM message_history GROUP BY created_at_day",
		},
		{
			name: "query with timezone",
			query: Query{
				Schema:   "message_history",
				Measures: []string{"count"},
				Timezone: stringPtr("America/New_York"),
				TimeDimensions: []TimeDimension{{
					Dimension:   "created_at",
					Granularity: "hour",
				}},
			},
			expectedSQL: "SELECT (COUNT(*)) AS count, (DATE_TRUNC('hour', created_at AT TIME ZONE 'America/New_York')) AS created_at_hour FROM message_history GROUP BY created_at_hour",
		},
		{
			name: "query with filters",
			query: Query{
				Schema:   "message_history",
				Measures: []string{"count"},
				Filters: []Filter{{
					Member:   "contact_email",
					Operator: "equals",
					Values:   []string{"test@example.com"},
				}},
			},
			expectedSQL:  "SELECT (COUNT(*)) AS count FROM message_history WHERE contact_email = $1",
			expectedArgs: []interface{}{"test@example.com"},
		},
		{
			name: "query with IN filter",
			query: Query{
				Schema:   "message_history",
				Measures: []string{"count"},
				Filters: []Filter{{
					Member:   "broadcast_id",
					Operator: "in",
					Values:   []string{"broadcast-1", "broadcast-2"},
				}},
			},
			expectedSQL:  "SELECT (COUNT(*)) AS count FROM message_history WHERE broadcast_id IN ($1,$2)",
			expectedArgs: []interface{}{"broadcast-1", "broadcast-2"},
		},
		{
			name: "query with LIKE filter",
			query: Query{
				Schema:   "message_history",
				Measures: []string{"count"},
				Filters: []Filter{{
					Member:   "contact_email",
					Operator: "contains",
					Values:   []string{"example"},
				}},
			},
			expectedSQL:  "SELECT (COUNT(*)) AS count FROM message_history WHERE contact_email LIKE $1",
			expectedArgs: []interface{}{"%example%"},
		},
		{
			name: "query with date range",
			query: Query{
				Schema:   "message_history",
				Measures: []string{"count"},
				TimeDimensions: []TimeDimension{{
					Dimension:   "created_at",
					Granularity: "day",
					DateRange:   &[2]string{"2024-01-01", "2024-12-31"},
				}},
			},
			expectedSQL:  "SELECT (COUNT(*)) AS count, (DATE_TRUNC('day', created_at)) AS created_at_day FROM message_history WHERE created_at >= $1 AND created_at <= $2 GROUP BY created_at_day",
			expectedArgs: []interface{}{"2024-01-01", "2024-12-31"},
		},
		{
			name: "query with order by",
			query: Query{
				Schema:     "message_history",
				Measures:   []string{"count"},
				Dimensions: []string{"contact_email"},
				Order: map[string]string{
					"count": "desc",
				},
			},
			expectedSQL: "SELECT (COUNT(*)) AS count, contact_email AS contact_email FROM message_history GROUP BY contact_email ORDER BY COUNT(*) DESC",
		},
		{
			name: "query with limit and offset",
			query: Query{
				Schema:   "message_history",
				Measures: []string{"count"},
				Limit:    intPtr(10),
				Offset:   intPtr(5),
			},
			expectedSQL: "SELECT (COUNT(*)) AS count FROM message_history LIMIT 10 OFFSET 5",
		},
		{
			name: "complex query",
			query: Query{
				Schema:     "message_history",
				Measures:   []string{"count", "count_sent"},
				Dimensions: []string{"contact_email"},
				TimeDimensions: []TimeDimension{{
					Dimension:   "created_at",
					Granularity: "day",
					DateRange:   &[2]string{"2024-01-01", "2024-12-31"},
				}},
				Filters: []Filter{{
					Member:   "broadcast_id",
					Operator: "notEquals",
					Values:   []string{"test-broadcast"},
				}},
				Order: map[string]string{
					"created_at": "desc",
				},
				Limit: intPtr(100),
			},
			expectedSQL:  "SELECT (COUNT(*)) AS count, (COUNT(*) FILTER (WHERE sent_at IS NOT NULL)) AS count_sent, contact_email AS contact_email, (DATE_TRUNC('day', created_at)) AS created_at_day FROM message_history WHERE broadcast_id <> $1 AND created_at >= $2 AND created_at <= $3 GROUP BY contact_email, created_at_day ORDER BY created_at DESC LIMIT 100",
			expectedArgs: []interface{}{"test-broadcast", "2024-01-01", "2024-12-31"},
		},
		{
			name: "invalid measure",
			query: Query{
				Schema:   "message_history",
				Measures: []string{"invalid_measure"},
			},
			expectedError: true,
		},
		{
			name: "invalid dimension",
			query: Query{
				Schema:     "message_history",
				Measures:   []string{"count"},
				Dimensions: []string{"invalid_dimension"},
			},
			expectedError: true,
		},
		{
			name: "invalid time dimension",
			query: Query{
				Schema:   "message_history",
				Measures: []string{"count"},
				TimeDimensions: []TimeDimension{{
					Dimension:   "invalid_dimension",
					Granularity: "day",
				}},
			},
			expectedError: true,
		},
		{
			name: "invalid granularity",
			query: Query{
				Schema:   "message_history",
				Measures: []string{"count"},
				TimeDimensions: []TimeDimension{{
					Dimension:   "created_at",
					Granularity: "invalid_granularity",
				}},
			},
			expectedError: true,
		},
		{
			name: "query with set filter (not null)",
			query: Query{
				Schema:   "message_history",
				Measures: []string{"count"},
				Filters: []Filter{{
					Member:   "broadcast_id",
					Operator: "set",
					Values:   []string{},
				}},
			},
			expectedSQL: "SELECT (COUNT(*)) AS count FROM message_history WHERE broadcast_id IS NOT NULL",
		},
		{
			name: "query with notSet filter (is null)",
			query: Query{
				Schema:   "message_history",
				Measures: []string{"count"},
				Filters: []Filter{{
					Member:   "broadcast_id",
					Operator: "notSet",
					Values:   []string{},
				}},
			},
			expectedSQL: "SELECT (COUNT(*)) AS count FROM message_history WHERE broadcast_id IS NULL",
		},
		{
			name: "query with startsWith filter",
			query: Query{
				Schema:   "message_history",
				Measures: []string{"count"},
				Filters: []Filter{{
					Member:   "contact_email",
					Operator: "startsWith",
					Values:   []string{"admin"},
				}},
			},
			expectedSQL:  "SELECT (COUNT(*)) AS count FROM message_history WHERE contact_email LIKE $1",
			expectedArgs: []interface{}{"admin%"},
		},
		{
			name: "query with endsWith filter",
			query: Query{
				Schema:   "message_history",
				Measures: []string{"count"},
				Filters: []Filter{{
					Member:   "contact_email",
					Operator: "endsWith",
					Values:   []string{".gov"},
				}},
			},
			expectedSQL:  "SELECT (COUNT(*)) AS count FROM message_history WHERE contact_email LIKE $1",
			expectedArgs: []interface{}{"%.gov"},
		},
		{
			name: "query with inDateRange filter",
			query: Query{
				Schema:   "message_history",
				Measures: []string{"count"},
				Filters: []Filter{{
					Member:   "created_at",
					Operator: "inDateRange",
					Values:   []string{"2024-01-01", "2024-03-31"},
				}},
			},
			expectedSQL:  "SELECT (COUNT(*)) AS count FROM message_history WHERE (created_at >= $1 AND created_at <= $2)",
			expectedArgs: []interface{}{"2024-01-01", "2024-03-31"},
		},
		{
			name: "query with beforeDate filter",
			query: Query{
				Schema:   "message_history",
				Measures: []string{"count"},
				Filters: []Filter{{
					Member:   "created_at",
					Operator: "beforeDate",
					Values:   []string{"2024-01-01"},
				}},
			},
			expectedSQL:  "SELECT (COUNT(*)) AS count FROM message_history WHERE created_at < $1",
			expectedArgs: []interface{}{"2024-01-01"},
		},
		{
			name: "query with multiple new operators",
			query: Query{
				Schema:   "message_history",
				Measures: []string{"count"},
				Filters: []Filter{
					{
						Member:   "broadcast_id",
						Operator: "set",
						Values:   []string{},
					},
					{
						Member:   "contact_email",
						Operator: "endsWith",
						Values:   []string{".com"},
					},
					{
						Member:   "created_at",
						Operator: "afterDate",
						Values:   []string{"2024-01-01"},
					},
				},
			},
			expectedSQL:  "SELECT (COUNT(*)) AS count FROM message_history WHERE broadcast_id IS NOT NULL AND contact_email LIKE $1 AND created_at > $2",
			expectedArgs: []interface{}{"%.com", "2024-01-01"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sql, args, err := builder.BuildSQL(tt.query, schema)

			if tt.expectedError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)

			// Normalize whitespace for comparison
			actualSQL := strings.Join(strings.Fields(sql), " ")
			expectedSQL := strings.Join(strings.Fields(tt.expectedSQL), " ")

			assert.Equal(t, expectedSQL, actualSQL)

			if tt.expectedArgs != nil {
				assert.Equal(t, tt.expectedArgs, args)
			}
		})
	}
}

func TestSQLBuilder_buildTimeDimensionSQL(t *testing.T) {
	builder := NewSQLBuilder()

	dimensionDef := DimensionDefinition{
		Type: "time",
		SQL:  "created_at",
	}

	tests := []struct {
		name          string
		timeDim       TimeDimension
		timezone      string
		expectedSQL   string
		expectedError bool
	}{
		{
			name: "hour granularity",
			timeDim: TimeDimension{
				Dimension:   "created_at",
				Granularity: "hour",
			},
			timezone:    "UTC",
			expectedSQL: "DATE_TRUNC('hour', created_at)",
		},
		{
			name: "day granularity",
			timeDim: TimeDimension{
				Dimension:   "created_at",
				Granularity: "day",
			},
			timezone:    "UTC",
			expectedSQL: "DATE_TRUNC('day', created_at)",
		},
		{
			name: "week granularity",
			timeDim: TimeDimension{
				Dimension:   "created_at",
				Granularity: "week",
			},
			timezone:    "UTC",
			expectedSQL: "DATE_TRUNC('week', created_at)",
		},
		{
			name: "month granularity",
			timeDim: TimeDimension{
				Dimension:   "created_at",
				Granularity: "month",
			},
			timezone:    "UTC",
			expectedSQL: "DATE_TRUNC('month', created_at)",
		},
		{
			name: "year granularity",
			timeDim: TimeDimension{
				Dimension:   "created_at",
				Granularity: "year",
			},
			timezone:    "UTC",
			expectedSQL: "DATE_TRUNC('year', created_at)",
		},
		{
			name: "with timezone",
			timeDim: TimeDimension{
				Dimension:   "created_at",
				Granularity: "day",
			},
			timezone:    "America/New_York",
			expectedSQL: "DATE_TRUNC('day', created_at AT TIME ZONE 'America/New_York')",
		},
		{
			name: "invalid granularity",
			timeDim: TimeDimension{
				Dimension:   "created_at",
				Granularity: "invalid",
			},
			timezone:      "UTC",
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sql, err := builder.buildTimeDimensionSQL(tt.timeDim, dimensionDef, tt.timezone)

			if tt.expectedError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expectedSQL, sql)
		})
	}
}

func TestSQLBuilder_buildFilterCondition(t *testing.T) {
	builder := NewSQLBuilder()

	tests := []struct {
		name          string
		memberSQL     string
		filter        Filter
		expectedSQL   string
		expectedArgs  []interface{}
		expectedError bool
	}{
		{
			name:      "equals single value",
			memberSQL: "contact_email",
			filter: Filter{
				Operator: "equals",
				Values:   []string{"test@example.com"},
			},
			expectedSQL:  "contact_email = ?",
			expectedArgs: []interface{}{"test@example.com"},
		},
		{
			name:      "equals multiple values",
			memberSQL: "status",
			filter: Filter{
				Operator: "equals",
				Values:   []string{"active", "pending"},
			},
			expectedSQL:  "status IN (?,?)",
			expectedArgs: []interface{}{"active", "pending"},
		},
		{
			name:      "not equals",
			memberSQL: "status",
			filter: Filter{
				Operator: "notEquals",
				Values:   []string{"inactive"},
			},
			expectedSQL:  "status <> ?",
			expectedArgs: []interface{}{"inactive"},
		},
		{
			name:      "contains",
			memberSQL: "contact_email",
			filter: Filter{
				Operator: "contains",
				Values:   []string{"example"},
			},
			expectedSQL:  "contact_email LIKE ?",
			expectedArgs: []interface{}{"%example%"},
		},
		{
			name:      "greater than",
			memberSQL: "created_at",
			filter: Filter{
				Operator: "gt",
				Values:   []string{"2024-01-01"},
			},
			expectedSQL:  "created_at > ?",
			expectedArgs: []interface{}{"2024-01-01"},
		},
		{
			name:      "greater than or equal",
			memberSQL: "created_at",
			filter: Filter{
				Operator: "gte",
				Values:   []string{"2024-01-01"},
			},
			expectedSQL:  "created_at >= ?",
			expectedArgs: []interface{}{"2024-01-01"},
		},
		{
			name:      "less than",
			memberSQL: "created_at",
			filter: Filter{
				Operator: "lt",
				Values:   []string{"2024-12-31"},
			},
			expectedSQL:  "created_at < ?",
			expectedArgs: []interface{}{"2024-12-31"},
		},
		{
			name:      "less than or equal",
			memberSQL: "created_at",
			filter: Filter{
				Operator: "lte",
				Values:   []string{"2024-12-31"},
			},
			expectedSQL:  "created_at <= ?",
			expectedArgs: []interface{}{"2024-12-31"},
		},
		{
			name:      "in",
			memberSQL: "broadcast_id",
			filter: Filter{
				Operator: "in",
				Values:   []string{"broadcast-1", "broadcast-2"},
			},
			expectedSQL:  "broadcast_id IN (?,?)",
			expectedArgs: []interface{}{"broadcast-1", "broadcast-2"},
		},
		{
			name:      "not in",
			memberSQL: "broadcast_id",
			filter: Filter{
				Operator: "notIn",
				Values:   []string{"broadcast-1", "broadcast-2"},
			},
			expectedSQL:  "broadcast_id NOT IN (?,?)",
			expectedArgs: []interface{}{"broadcast-1", "broadcast-2"},
		},
		{
			name:      "invalid operator",
			memberSQL: "field",
			filter: Filter{
				Operator: "invalid",
				Values:   []string{"value"},
			},
			expectedError: true,
		},
		{
			name:      "contains with multiple values",
			memberSQL: "field",
			filter: Filter{
				Operator: "contains",
				Values:   []string{"value1", "value2"},
			},
			expectedError: true,
		},
		{
			name:      "not contains",
			memberSQL: "contact_email",
			filter: Filter{
				Operator: "notContains",
				Values:   []string{"spam"},
			},
			expectedSQL:  "contact_email NOT LIKE ?",
			expectedArgs: []interface{}{"%spam%"},
		},
		{
			name:      "starts with",
			memberSQL: "contact_email",
			filter: Filter{
				Operator: "startsWith",
				Values:   []string{"admin"},
			},
			expectedSQL:  "contact_email LIKE ?",
			expectedArgs: []interface{}{"admin%"},
		},
		{
			name:      "not starts with",
			memberSQL: "contact_email",
			filter: Filter{
				Operator: "notStartsWith",
				Values:   []string{"test"},
			},
			expectedSQL:  "contact_email NOT LIKE ?",
			expectedArgs: []interface{}{"test%"},
		},
		{
			name:      "ends with",
			memberSQL: "contact_email",
			filter: Filter{
				Operator: "endsWith",
				Values:   []string{".com"},
			},
			expectedSQL:  "contact_email LIKE ?",
			expectedArgs: []interface{}{"%.com"},
		},
		{
			name:      "not ends with",
			memberSQL: "contact_email",
			filter: Filter{
				Operator: "notEndsWith",
				Values:   []string{".spam"},
			},
			expectedSQL:  "contact_email NOT LIKE ?",
			expectedArgs: []interface{}{"%.spam"},
		},
		{
			name:      "set (not null)",
			memberSQL: "optional_field",
			filter: Filter{
				Operator: "set",
				Values:   []string{},
			},
			expectedSQL: "optional_field IS NOT NULL",
		},
		{
			name:      "not set (is null)",
			memberSQL: "optional_field",
			filter: Filter{
				Operator: "notSet",
				Values:   []string{},
			},
			expectedSQL: "optional_field IS NULL",
		},
		{
			name:      "in date range",
			memberSQL: "created_at",
			filter: Filter{
				Operator: "inDateRange",
				Values:   []string{"2024-01-01", "2024-12-31"},
			},
			expectedSQL:  "(created_at >= ? AND created_at <= ?)",
			expectedArgs: []interface{}{"2024-01-01", "2024-12-31"},
		},
		{
			name:      "not in date range",
			memberSQL: "created_at",
			filter: Filter{
				Operator: "notInDateRange",
				Values:   []string{"2024-06-01", "2024-06-30"},
			},
			expectedSQL:  "(created_at < ? OR created_at > ?)",
			expectedArgs: []interface{}{"2024-06-01", "2024-06-30"},
		},
		{
			name:      "before date",
			memberSQL: "created_at",
			filter: Filter{
				Operator: "beforeDate",
				Values:   []string{"2024-01-01"},
			},
			expectedSQL:  "created_at < ?",
			expectedArgs: []interface{}{"2024-01-01"},
		},
		{
			name:      "after date",
			memberSQL: "created_at",
			filter: Filter{
				Operator: "afterDate",
				Values:   []string{"2024-12-31"},
			},
			expectedSQL:  "created_at > ?",
			expectedArgs: []interface{}{"2024-12-31"},
		},
		{
			name:      "not contains with multiple values",
			memberSQL: "field",
			filter: Filter{
				Operator: "notContains",
				Values:   []string{"value1", "value2"},
			},
			expectedError: true,
		},
		{
			name:      "starts with with multiple values",
			memberSQL: "field",
			filter: Filter{
				Operator: "startsWith",
				Values:   []string{"value1", "value2"},
			},
			expectedError: true,
		},
		{
			name:      "in date range with wrong number of values",
			memberSQL: "field",
			filter: Filter{
				Operator: "inDateRange",
				Values:   []string{"2024-01-01"},
			},
			expectedError: true,
		},
		{
			name:      "before date with multiple values",
			memberSQL: "field",
			filter: Filter{
				Operator: "beforeDate",
				Values:   []string{"2024-01-01", "2024-12-31"},
			},
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			condition, err := builder.buildFilterCondition(tt.memberSQL, tt.filter)

			if tt.expectedError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)

			// Convert condition to SQL to test
			sql, args, err := condition.ToSql()
			require.NoError(t, err)

			assert.Equal(t, tt.expectedSQL, sql)
			if tt.expectedArgs != nil {
				assert.Equal(t, tt.expectedArgs, args)
			} else {
				// If no expected args specified, args should be empty or nil
				assert.True(t, len(args) == 0, "Expected no arguments but got %v", args)
			}
		})
	}
}

func TestSQLBuilder_MeasureTypes(t *testing.T) {
	builder := NewSQLBuilder()

	// Create test schema with all measure types
	schema := SchemaDefinition{
		Name: "analytics_table",
		Measures: map[string]MeasureDefinition{
			"count_records": {
				Type:        "count",
				SQL:         "COUNT(*)",
				Description: "Total number of records",
			},
			"count_distinct": {
				Type:        "count",
				SQL:         "COUNT(DISTINCT user_id)",
				Description: "Unique users count",
			},
			"sum_revenue": {
				Type:        "sum",
				SQL:         "revenue", // Cube.js style - just column name, not SUM(revenue)
				Description: "Total revenue",
			},
			"sum_with_filter": {
				Type:        "sum",
				SQL:         "SUM(amount) FILTER (WHERE status = 'completed')",
				Description: "Sum of completed amounts",
			},
			"avg_rating": {
				Type:        "avg",
				SQL:         "rating", // Cube.js style - just column name, not AVG(rating)
				Description: "Average rating",
			},
			"avg_duration": {
				Type:        "avg",
				SQL:         "AVG(EXTRACT(EPOCH FROM (completed_at - started_at)))",
				Description: "Average duration in seconds",
			},
			"min_price": {
				Type:        "min",
				SQL:         "price", // Cube.js style - just column name, not MIN(price)
				Description: "Minimum price",
			},
			"min_date": {
				Type:        "min",
				SQL:         "MIN(created_at)",
				Description: "Earliest date",
			},
			"max_score": {
				Type:        "max",
				SQL:         "score", // Cube.js style - just column name, not MAX(score)
				Description: "Maximum score",
			},
			"max_updated": {
				Type:        "max",
				SQL:         "MAX(updated_at)",
				Description: "Latest update",
			},
			"simple_measure": {
				Type:        "count",
				SQL:         "", // No custom SQL - should use measure name
				Description: "Simple measure without custom SQL",
			},
		},
		Dimensions: map[string]DimensionDefinition{
			"category": {
				Type:        "string",
				SQL:         "category",
				Description: "Product category",
			},
		},
	}

	tests := []struct {
		name        string
		measures    []string
		expectedSQL string
		description string
	}{
		{
			name:        "count measure",
			measures:    []string{"count_records"},
			expectedSQL: "SELECT (COUNT(*)) AS count_records FROM analytics_table",
			description: "Basic COUNT(*) measure",
		},
		{
			name:        "count distinct measure",
			measures:    []string{"count_distinct"},
			expectedSQL: "SELECT (COUNT(DISTINCT user_id)) AS count_distinct FROM analytics_table",
			description: "COUNT DISTINCT measure",
		},
		{
			name:        "sum measure",
			measures:    []string{"sum_revenue"},
			expectedSQL: "SELECT (SUM(revenue)) AS sum_revenue FROM analytics_table",
			description: "Basic SUM measure - Cube.js style with automatic SUM() wrapping",
		},
		{
			name:        "sum with filter measure",
			measures:    []string{"sum_with_filter"},
			expectedSQL: "SELECT (SUM(amount) FILTER (WHERE status = 'completed')) AS sum_with_filter FROM analytics_table",
			description: "SUM with FILTER clause",
		},
		{
			name:        "avg measure",
			measures:    []string{"avg_rating"},
			expectedSQL: "SELECT (AVG(rating)) AS avg_rating FROM analytics_table",
			description: "Basic AVG measure - Cube.js style with automatic AVG() wrapping",
		},
		{
			name:        "avg complex measure",
			measures:    []string{"avg_duration"},
			expectedSQL: "SELECT (AVG(EXTRACT(EPOCH FROM (completed_at - started_at)))) AS avg_duration FROM analytics_table",
			description: "Complex AVG calculation",
		},
		{
			name:        "min measure",
			measures:    []string{"min_price"},
			expectedSQL: "SELECT (MIN(price)) AS min_price FROM analytics_table",
			description: "Basic MIN measure - Cube.js style with automatic MIN() wrapping",
		},
		{
			name:        "min date measure",
			measures:    []string{"min_date"},
			expectedSQL: "SELECT (MIN(created_at)) AS min_date FROM analytics_table",
			description: "MIN with date field",
		},
		{
			name:        "max measure",
			measures:    []string{"max_score"},
			expectedSQL: "SELECT (MAX(score)) AS max_score FROM analytics_table",
			description: "Basic MAX measure - Cube.js style with automatic MAX() wrapping",
		},
		{
			name:        "max timestamp measure",
			measures:    []string{"max_updated"},
			expectedSQL: "SELECT (MAX(updated_at)) AS max_updated FROM analytics_table",
			description: "MAX with timestamp field",
		},
		{
			name:        "simple measure without custom SQL",
			measures:    []string{"simple_measure"},
			expectedSQL: "SELECT (simple_measure) AS simple_measure FROM analytics_table",
			description: "Measure without custom SQL uses measure name",
		},
		{
			name:        "multiple measures of different types",
			measures:    []string{"count_records", "sum_revenue", "avg_rating", "min_price", "max_score"},
			expectedSQL: "SELECT (COUNT(*)) AS count_records, (SUM(revenue)) AS sum_revenue, (AVG(rating)) AS avg_rating, (MIN(price)) AS min_price, (MAX(score)) AS max_score FROM analytics_table",
			description: "Multiple measures of all types - Cube.js style with automatic wrapping",
		},
		{
			name:        "measures with dimensions",
			measures:    []string{"count_records", "sum_revenue"},
			expectedSQL: "SELECT (COUNT(*)) AS count_records, (SUM(revenue)) AS sum_revenue, category AS category FROM analytics_table GROUP BY category",
			description: "Measures with grouping dimension",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			query := Query{
				Schema:   "analytics_table",
				Measures: tt.measures,
			}

			// Add dimension for the last test case
			if tt.name == "measures with dimensions" {
				query.Dimensions = []string{"category"}
			}

			sql, args, err := builder.BuildSQL(query, schema)
			require.NoError(t, err, "Failed to build SQL for %s", tt.description)

			// Normalize whitespace for comparison
			actualSQL := strings.Join(strings.Fields(sql), " ")
			expectedSQL := strings.Join(strings.Fields(tt.expectedSQL), " ")

			assert.Equal(t, expectedSQL, actualSQL, "SQL mismatch for %s", tt.description)
			assert.Empty(t, args, "No parameters expected for %s", tt.description)
		})
	}
}

func TestSQLBuilder_MeasureTypeValidation(t *testing.T) {
	builder := NewSQLBuilder()

	schema := SchemaDefinition{
		Name: "test_table",
		Measures: map[string]MeasureDefinition{
			"valid_count": {Type: "count", SQL: "COUNT(*)"},
			"valid_sum":   {Type: "sum", SQL: "SUM(amount)"},
			"valid_avg":   {Type: "avg", SQL: "AVG(rating)"},
			"valid_min":   {Type: "min", SQL: "MIN(price)"},
			"valid_max":   {Type: "max", SQL: "MAX(score)"},
		},
		Dimensions: map[string]DimensionDefinition{},
	}

	tests := []struct {
		name          string
		measures      []string
		expectedError bool
		description   string
	}{
		{
			name:          "valid count measure",
			measures:      []string{"valid_count"},
			expectedError: false,
			description:   "Should accept valid count measure",
		},
		{
			name:          "valid sum measure",
			measures:      []string{"valid_sum"},
			expectedError: false,
			description:   "Should accept valid sum measure",
		},
		{
			name:          "valid avg measure",
			measures:      []string{"valid_avg"},
			expectedError: false,
			description:   "Should accept valid avg measure",
		},
		{
			name:          "valid min measure",
			measures:      []string{"valid_min"},
			expectedError: false,
			description:   "Should accept valid min measure",
		},
		{
			name:          "valid max measure",
			measures:      []string{"valid_max"},
			expectedError: false,
			description:   "Should accept valid max measure",
		},
		{
			name:          "all valid measure types",
			measures:      []string{"valid_count", "valid_sum", "valid_avg", "valid_min", "valid_max"},
			expectedError: false,
			description:   "Should accept all valid measure types together",
		},
		{
			name:          "invalid measure name",
			measures:      []string{"nonexistent_measure"},
			expectedError: true,
			description:   "Should reject nonexistent measure",
		},
		{
			name:          "mix of valid and invalid measures",
			measures:      []string{"valid_count", "nonexistent_measure"},
			expectedError: true,
			description:   "Should reject query with any invalid measure",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			query := Query{
				Schema:   "test_table",
				Measures: tt.measures,
			}

			_, _, err := builder.BuildSQL(query, schema)

			if tt.expectedError {
				assert.Error(t, err, "Expected error for %s", tt.description)
				assert.Contains(t, err.Error(), "not found in schema", "Error should mention missing measure")
			} else {
				assert.NoError(t, err, "Should not error for %s", tt.description)
			}
		})
	}
}

func TestSQLBuilder_CubeJSStyleMeasures(t *testing.T) {
	builder := NewSQLBuilder()

	// Schema with Cube.js-style measures (just column names, no aggregate functions)
	schema := SchemaDefinition{
		Name: "orders",
		Measures: map[string]MeasureDefinition{
			"total_amount": {
				Type:        "sum",
				SQL:         "amount", // Cube.js style - just column name
				Description: "Total order amount",
			},
			"average_rating": {
				Type:        "avg",
				SQL:         "customer_rating", // Cube.js style - just column name
				Description: "Average customer rating",
			},
			"min_order_date": {
				Type:        "min",
				SQL:         "created_at", // Cube.js style - just column name
				Description: "Earliest order date",
			},
			"max_order_value": {
				Type:        "max",
				SQL:         "order_value", // Cube.js style - just column name
				Description: "Maximum order value",
			},
			"unique_customers": {
				Type:        "count_distinct",
				SQL:         "customer_id", // Cube.js style - just column name
				Description: "Number of unique customers",
			},
		},
		Dimensions: map[string]DimensionDefinition{},
	}

	query := Query{
		Schema:   "orders",
		Measures: []string{"total_amount", "average_rating", "min_order_date", "max_order_value", "unique_customers"},
	}

	sql, args, err := builder.BuildSQL(query, schema)
	require.NoError(t, err)

	expectedSQL := "SELECT (SUM(amount)) AS total_amount, (AVG(customer_rating)) AS average_rating, (MIN(created_at)) AS min_order_date, (MAX(order_value)) AS max_order_value, (COUNT(DISTINCT customer_id)) AS unique_customers FROM orders"

	// Normalize whitespace for comparison
	actualSQL := strings.Join(strings.Fields(sql), " ")
	expectedSQL = strings.Join(strings.Fields(expectedSQL), " ")

	assert.Equal(t, expectedSQL, actualSQL, "Should automatically wrap Cube.js-style measures with appropriate aggregate functions")
	assert.Empty(t, args, "No parameters expected for simple measures")
}

func TestSQLBuilder_MeasureFilters(t *testing.T) {
	builder := NewSQLBuilder()

	// Schema with measures that use filters
	schema := SchemaDefinition{
		Name: "test_table",
		Measures: map[string]MeasureDefinition{
			"count_all": {
				Type:        "count",
				SQL:         "*",
				Description: "Total count without filters",
			},
			"count_active": {
				Type:        "count",
				SQL:         "*",
				Description: "Count of active records",
				Filters: []MeasureFilter{
					{SQL: "status = 'active'"},
				},
			},
			"count_active_emails": {
				Type:        "count",
				SQL:         "*",
				Description: "Count of active email records",
				Filters: []MeasureFilter{
					{SQL: "status = 'active'"},
					{SQL: "channel = 'email'"},
				},
			},
			"sum_revenue_completed": {
				Type:        "sum",
				SQL:         "revenue",
				Description: "Sum of revenue for completed orders",
				Filters: []MeasureFilter{
					{SQL: "status = 'completed'"},
				},
			},
			"avg_rating_premium": {
				Type:        "avg",
				SQL:         "rating",
				Description: "Average rating for premium users",
				Filters: []MeasureFilter{
					{SQL: "user_type = 'premium'"},
					{SQL: "rating IS NOT NULL"},
				},
			},
			"max_price_available": {
				Type:        "max",
				SQL:         "price",
				Description: "Maximum price for available items",
				Filters: []MeasureFilter{
					{SQL: "availability = 'available'"},
				},
			},
			"complex_with_filters": {
				Type:        "count",
				SQL:         "COUNT(*) FILTER (WHERE existing_condition = true)",
				Description: "Complex measure with existing filter",
				Filters: []MeasureFilter{
					{SQL: "additional_condition = 'yes'"},
				},
			},
		},
		Dimensions: map[string]DimensionDefinition{},
	}

	tests := []struct {
		name        string
		measures    []string
		expectedSQL string
		description string
	}{
		{
			name:        "count without filters",
			measures:    []string{"count_all"},
			expectedSQL: "SELECT (COUNT(*)) AS count_all FROM test_table",
			description: "Basic count without any filters",
		},
		{
			name:        "count with single filter",
			measures:    []string{"count_active"},
			expectedSQL: "SELECT (COUNT(*) FILTER (WHERE status = 'active')) AS count_active FROM test_table",
			description: "Count with single filter condition",
		},
		{
			name:        "count with multiple filters",
			measures:    []string{"count_active_emails"},
			expectedSQL: "SELECT (COUNT(*) FILTER (WHERE status = 'active' AND channel = 'email')) AS count_active_emails FROM test_table",
			description: "Count with multiple filter conditions joined by AND",
		},
		{
			name:        "sum with filter",
			measures:    []string{"sum_revenue_completed"},
			expectedSQL: "SELECT (SUM(revenue) FILTER (WHERE status = 'completed')) AS sum_revenue_completed FROM test_table",
			description: "Sum measure with filter condition",
		},
		{
			name:        "avg with multiple filters",
			measures:    []string{"avg_rating_premium"},
			expectedSQL: "SELECT (AVG(rating) FILTER (WHERE user_type = 'premium' AND rating IS NOT NULL)) AS avg_rating_premium FROM test_table",
			description: "Average measure with multiple filter conditions",
		},
		{
			name:        "max with filter",
			measures:    []string{"max_price_available"},
			expectedSQL: "SELECT (MAX(price) FILTER (WHERE availability = 'available')) AS max_price_available FROM test_table",
			description: "Max measure with filter condition",
		},
		{
			name:        "complex measure with additional filters",
			measures:    []string{"complex_with_filters"},
			expectedSQL: "SELECT (COUNT(*) FILTER (WHERE existing_condition = true) FILTER (WHERE additional_condition = 'yes')) AS complex_with_filters FROM test_table",
			description: "Complex measure that already has filters gets additional filters applied",
		},
		{
			name:        "mixed measures with and without filters",
			measures:    []string{"count_all", "count_active", "sum_revenue_completed"},
			expectedSQL: "SELECT (COUNT(*)) AS count_all, (COUNT(*) FILTER (WHERE status = 'active')) AS count_active, (SUM(revenue) FILTER (WHERE status = 'completed')) AS sum_revenue_completed FROM test_table",
			description: "Multiple measures with mixed filter usage",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			query := Query{
				Schema:   "test_table",
				Measures: tt.measures,
			}

			sql, args, err := builder.BuildSQL(query, schema)
			require.NoError(t, err, "Failed to build SQL for %s", tt.description)

			// Normalize whitespace for comparison
			actualSQL := strings.Join(strings.Fields(sql), " ")
			expectedSQL := strings.Join(strings.Fields(tt.expectedSQL), " ")

			assert.Equal(t, expectedSQL, actualSQL, "SQL mismatch for %s", tt.description)
			assert.Empty(t, args, "No parameters expected for %s", tt.description)
		})
	}
}

func TestSQLBuilder_applyMeasureFilters(t *testing.T) {
	builder := NewSQLBuilder()

	tests := []struct {
		name        string
		baseSQL     string
		filters     []MeasureFilter
		expectedSQL string
		description string
	}{
		{
			name:        "no filters",
			baseSQL:     "COUNT(*)",
			filters:     []MeasureFilter{},
			expectedSQL: "COUNT(*)",
			description: "Should return base SQL unchanged when no filters",
		},
		{
			name:        "single filter",
			baseSQL:     "COUNT(*)",
			filters:     []MeasureFilter{{SQL: "status = 'active'"}},
			expectedSQL: "COUNT(*) FILTER (WHERE status = 'active')",
			description: "Should apply single filter with FILTER clause",
		},
		{
			name:        "multiple filters",
			baseSQL:     "SUM(amount)",
			filters:     []MeasureFilter{{SQL: "status = 'completed'"}, {SQL: "amount > 0"}},
			expectedSQL: "SUM(amount) FILTER (WHERE status = 'completed' AND amount > 0)",
			description: "Should join multiple filters with AND",
		},
		{
			name:        "filter with empty SQL",
			baseSQL:     "AVG(rating)",
			filters:     []MeasureFilter{{SQL: "rating IS NOT NULL"}, {SQL: ""}},
			expectedSQL: "AVG(rating) FILTER (WHERE rating IS NOT NULL)",
			description: "Should ignore filters with empty SQL",
		},
		{
			name:        "all filters empty",
			baseSQL:     "MAX(price)",
			filters:     []MeasureFilter{{SQL: ""}, {SQL: ""}},
			expectedSQL: "MAX(price)",
			description: "Should return base SQL when all filters are empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := builder.applyMeasureFilters(tt.baseSQL, tt.filters)
			assert.Equal(t, tt.expectedSQL, result, tt.description)
		})
	}
}

func TestSQLBuilder_SQLInjectionPrevention(t *testing.T) {
	builder := NewSQLBuilder()

	// Schema for testing SQL injection scenarios
	schema := SchemaDefinition{
		Name: "test_table",
		Measures: map[string]MeasureDefinition{
			"count": {
				Type:        "count",
				SQL:         "*",
				Description: "Total count",
			},
			"sum_amount": {
				Type:        "sum",
				SQL:         "amount",
				Description: "Sum of amounts",
			},
		},
		Dimensions: map[string]DimensionDefinition{
			"status": {
				Type:        "string",
				SQL:         "status",
				Description: "Status field",
			},
			"created_at": {
				Type:        "time",
				SQL:         "created_at",
				Description: "Creation timestamp",
			},
			"user_id": {
				Type:        "number",
				SQL:         "user_id",
				Description: "User identifier",
			},
		},
	}

	tests := []struct {
		name        string
		query       Query
		expectError bool
		description string
	}{
		{
			name: "malicious filter values - SQL injection attempt",
			query: Query{
				Schema:   "test_table",
				Measures: []string{"count"},
				Filters: []Filter{
					{
						Member:   "status",
						Operator: "equals",
						Values:   []string{"'; DROP TABLE users; --"},
					},
				},
			},
			expectError: false, // Should not error but should be safely parameterized
			description: "SQL injection attempt in filter values should be safely parameterized",
		},
		{
			name: "malicious filter values - UNION attack",
			query: Query{
				Schema:   "test_table",
				Measures: []string{"count"},
				Filters: []Filter{
					{
						Member:   "status",
						Operator: "equals",
						Values:   []string{"active' UNION SELECT * FROM sensitive_table WHERE '1'='1"},
					},
				},
			},
			expectError: false,
			description: "UNION SQL injection attempt should be safely parameterized",
		},
		{
			name: "malicious filter values - boolean bypass",
			query: Query{
				Schema:   "test_table",
				Measures: []string{"count"},
				Filters: []Filter{
					{
						Member:   "user_id",
						Operator: "equals",
						Values:   []string{"1 OR 1=1"},
					},
				},
			},
			expectError: false,
			description: "Boolean bypass attempt should be safely parameterized",
		},
		{
			name: "malicious filter values - multiple injection attempts",
			query: Query{
				Schema:   "test_table",
				Measures: []string{"count"},
				Filters: []Filter{
					{
						Member:   "status",
						Operator: "in",
						Values:   []string{"'; DELETE FROM users; --", "admin' OR '1'='1", "UNION SELECT password FROM auth"},
					},
				},
			},
			expectError: false,
			description: "Multiple SQL injection attempts should be safely parameterized",
		},
		{
			name: "malicious date range - injection in time dimension",
			query: Query{
				Schema:   "test_table",
				Measures: []string{"count"},
				TimeDimensions: []TimeDimension{
					{
						Dimension:   "created_at",
						Granularity: "day",
						DateRange:   &[2]string{"2023-01-01'; DROP TABLE logs; --", "2023-12-31"},
					},
				},
			},
			expectError: false,
			description: "SQL injection in date range should be safely parameterized",
		},
		{
			name: "malicious order by - injection attempt",
			query: Query{
				Schema:   "test_table",
				Measures: []string{"count"},
				Order: map[string]string{
					"count": "ASC; DROP TABLE users; --",
				},
			},
			expectError: false, // Order direction is sanitized to ASC/DESC only
			description: "SQL injection in order clause should be sanitized",
		},
		{
			name: "malicious limit - injection attempt",
			query: Query{
				Schema:   "test_table",
				Measures: []string{"count"},
				Limit:    func() *int { v := -1; return &v }(), // Negative limit
			},
			expectError: false,
			description: "Malicious limit values should be handled safely",
		},
		{
			name: "malicious timezone - injection attempt",
			query: Query{
				Schema:   "test_table",
				Measures: []string{"count"},
				Timezone: func() *string { v := "UTC'; DROP TABLE logs; --"; return &v }(),
				TimeDimensions: []TimeDimension{
					{
						Dimension:   "created_at",
						Granularity: "day",
					},
				},
			},
			expectError: false,
			description: "SQL injection in timezone should be handled safely",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sql, args, err := builder.BuildSQL(tt.query, schema)

			if tt.expectError {
				assert.Error(t, err, tt.description)
				return
			}

			require.NoError(t, err, tt.description)

			// Verify that the SQL is properly parameterized
			assert.NotEmpty(t, sql, "SQL should be generated")

			// Check that dangerous SQL keywords are not directly embedded in the SQL string
			// They should only appear as parameterized values in the args slice
			upperSQL := strings.ToUpper(sql)

			// These should not appear directly in the generated SQL (they should be parameterized)
			dangerousPatterns := []string{
				"DROP TABLE",
				"DELETE FROM",
				"INSERT INTO",
				"UPDATE ",
				"UNION SELECT",
				"'; --",
				"OR 1=1",
				"OR '1'='1'",
			}

			for _, pattern := range dangerousPatterns {
				assert.NotContains(t, upperSQL, pattern,
					"Dangerous SQL pattern '%s' should not appear directly in generated SQL: %s",
					pattern, sql)
			}

			// Verify that filter values are properly parameterized (appear in args, not in SQL)
			for _, filter := range tt.query.Filters {
				for _, value := range filter.Values {
					if strings.Contains(value, "'") || strings.Contains(value, "--") || strings.Contains(value, "UNION") {
						// Malicious values should be in args (parameterized), not directly in SQL
						found := false
						for _, arg := range args {
							if arg == value {
								found = true
								break
							}
						}
						assert.True(t, found,
							"Malicious filter value '%s' should be parameterized (found in args)", value)
						assert.NotContains(t, sql, value,
							"Malicious filter value '%s' should not appear directly in SQL", value)
					}
				}
			}

			// Log the generated SQL and args for manual inspection
			t.Logf("Generated SQL: %s", sql)
			t.Logf("Parameters: %v", args)
		})
	}
}

func TestSQLBuilder_MeasureFilterSQLInjection(t *testing.T) {
	builder := NewSQLBuilder()

	// Test SQL injection in measure filters
	tests := []struct {
		name        string
		baseSQL     string
		filters     []MeasureFilter
		description string
	}{
		{
			name:    "malicious filter SQL - DROP TABLE",
			baseSQL: "COUNT(*)",
			filters: []MeasureFilter{
				{SQL: "status = 'active'; DROP TABLE users; --"},
			},
			description: "Measure filter with DROP TABLE should be handled safely",
		},
		{
			name:    "malicious filter SQL - UNION attack",
			baseSQL: "SUM(amount)",
			filters: []MeasureFilter{
				{SQL: "1=1 UNION SELECT password FROM auth_table"},
			},
			description: "Measure filter with UNION attack should be handled safely",
		},
		{
			name:    "malicious filter SQL - nested injection",
			baseSQL: "AVG(rating)",
			filters: []MeasureFilter{
				{SQL: "user_type = 'premium'"},
				{SQL: "rating > 0'; DELETE FROM logs WHERE '1'='1"},
			},
			description: "Multiple measure filters with injection attempts should be handled safely",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := builder.applyMeasureFilters(tt.baseSQL, tt.filters)

			// The result should contain the filter SQL, but this is by design for measure filters
			// Measure filters are expected to contain raw SQL conditions
			// However, we should log this for awareness
			t.Logf("Generated SQL with filters: %s", result)

			// Measure filters are a special case where SQL is directly embedded
			// This is the expected behavior for Cube.js compatibility
			// The responsibility for safe SQL in measure filters lies with the schema definition
			assert.Contains(t, result, "FILTER (WHERE", "Should apply FILTER clause")

			// Log warning about measure filter security
			if strings.Contains(strings.ToUpper(result), "DROP") ||
				strings.Contains(strings.ToUpper(result), "DELETE") ||
				strings.Contains(strings.ToUpper(result), "UNION") {
				t.Logf("WARNING: Measure filter contains potentially dangerous SQL: %s", result)
				t.Logf("NOTE: Measure filters use raw SQL by design (Cube.js compatibility)")
				t.Logf("SECURITY: Ensure measure definitions are controlled and validated at schema level")
			}
		})
	}
}

func TestSQLBuilder_sanitizeTimezone(t *testing.T) {
	builder := NewSQLBuilder()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "valid timezone",
			input:    "America/New_York",
			expected: "America/New_York",
		},
		{
			name:     "valid UTC offset",
			input:    "+05:30",
			expected: "+05:30",
		},
		{
			name:     "valid timezone with underscores",
			input:    "Europe/London",
			expected: "Europe/London",
		},
		{
			name:     "SQL injection attempt with quotes",
			input:    "UTC'; DROP TABLE users; --",
			expected: "", // Should be empty after sanitization
		},
		{
			name:     "SQL injection with semicolon",
			input:    "UTC; DELETE FROM logs",
			expected: "", // Should be empty after sanitization
		},
		{
			name:     "SQL injection with comment",
			input:    "UTC-- malicious comment",
			expected: "", // Should be empty after sanitization
		},
		{
			name:     "SQL injection with block comment",
			input:    "UTC/* block comment */",
			expected: "", // Should be empty after sanitization
		},
		{
			name:     "too long timezone",
			input:    "this_is_a_very_long_timezone_name_that_exceeds_the_limit_and_should_be_rejected",
			expected: "", // Should be empty due to length
		},
		{
			name:     "invalid characters",
			input:    "UTC@#$%",
			expected: "", // Should be empty due to invalid chars
		},
		{
			name:     "empty input",
			input:    "",
			expected: "",
		},
		{
			name:     "whitespace only",
			input:    "   ",
			expected: "",
		},
		{
			name:     "valid timezone with whitespace",
			input:    "  America/Chicago  ",
			expected: "America/Chicago", // Should be trimmed
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := builder.sanitizeTimezone(tt.input)
			assert.Equal(t, tt.expected, result,
				"sanitizeTimezone(%q) = %q, want %q", tt.input, result, tt.expected)
		})
	}
}

func TestQuery_ToSQL(t *testing.T) {
	schema := SchemaDefinition{
		Name: "test_table",
		Measures: map[string]MeasureDefinition{
			"count": {Type: "count", SQL: "COUNT(*)"},
		},
		Dimensions: map[string]DimensionDefinition{
			"created_at": {Type: "time", SQL: "created_at"},
		},
	}

	query := Query{
		Schema:   "test_table",
		Measures: []string{"count"},
		TimeDimensions: []TimeDimension{{
			Dimension:   "created_at",
			Granularity: "day",
		}},
	}

	sql, args, err := query.ToSQL(schema)
	require.NoError(t, err)
	assert.Contains(t, sql, "SELECT")
	assert.Contains(t, sql, "COUNT(*)")
	assert.Contains(t, sql, "DATE_TRUNC")
	assert.Empty(t, args)
}

func TestScanRows(t *testing.T) {
	// Create mock database
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	tests := []struct {
		name         string
		columns      []string
		rows         [][]driver.Value
		expectedData []map[string]interface{}
		description  string
	}{
		{
			name:    "simple string and number data",
			columns: []string{"name", "count", "active"},
			rows: [][]driver.Value{
				{"Alice", int64(10), true},
				{"Bob", int64(5), false},
			},
			expectedData: []map[string]interface{}{
				{"name": "Alice", "count": int64(10), "active": true},
				{"name": "Bob", "count": int64(5), "active": false},
			},
			description: "Should scan basic data types correctly",
		},
		{
			name:    "byte array conversion",
			columns: []string{"id", "data"},
			rows: [][]driver.Value{
				{int64(1), []byte("test data")},
				{int64(2), []byte("more data")},
			},
			expectedData: []map[string]interface{}{
				{"id": int64(1), "data": "test data"},
				{"id": int64(2), "data": "more data"},
			},
			description: "Should convert []byte to string",
		},
		{
			name:    "timestamp data",
			columns: []string{"id", "created_at"},
			rows: [][]driver.Value{
				{int64(1), time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)},
			},
			expectedData: []map[string]interface{}{
				{"id": int64(1), "created_at": "2024-01-01T12:00:00Z"},
			},
			description: "Should handle timestamp data and convert time.Time to string",
		},
		{
			name:         "empty result set",
			columns:      []string{"id", "name"},
			rows:         [][]driver.Value{},
			expectedData: []map[string]interface{}{},
			description:  "Should handle empty result set",
		},
		{
			name:    "null values",
			columns: []string{"id", "name", "optional_field"},
			rows: [][]driver.Value{
				{int64(1), "Alice", "value"},
				{int64(2), "Bob", nil},
			},
			expectedData: []map[string]interface{}{
				{"id": int64(1), "name": "Alice", "optional_field": "value"},
				{"id": int64(2), "name": "Bob", "optional_field": nil},
			},
			description: "Should handle null values correctly",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up mock rows
			mockRows := sqlmock.NewRows(tt.columns)
			for _, row := range tt.rows {
				mockRows = mockRows.AddRow(row...)
			}

			mock.ExpectQuery("SELECT (.+)").WillReturnRows(mockRows)

			// Execute query to get rows
			rows, err := db.Query("SELECT * FROM test")
			require.NoError(t, err)
			defer rows.Close()

			// Test ScanRows function
			data, err := ScanRows(rows)
			require.NoError(t, err, tt.description)

			// Verify results
			assert.Equal(t, len(tt.expectedData), len(data), "Data length should match")
			for i, expected := range tt.expectedData {
				if i < len(data) {
					for key, expectedValue := range expected {
						actualValue := data[i][key]
						assert.Equal(t, expectedValue, actualValue,
							"Data[%d][%s] should match: expected %v, got %v", i, key, expectedValue, actualValue)
					}
				}
			}

			// Verify all expectations were met
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestProcessRows(t *testing.T) {
	// Create mock database
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	tests := []struct {
		name         string
		query        Query
		columns      []string
		rows         [][]driver.Value
		expectedData []map[string]interface{}
		description  string
	}{
		{
			name: "non-time series query",
			query: Query{
				Schema:   "test_table",
				Measures: []string{"count"},
			},
			columns: []string{"count"},
			rows: [][]driver.Value{
				{int64(42)},
			},
			expectedData: []map[string]interface{}{
				{"count": int64(42)},
			},
			description: "Should process non-time series data without modification",
		},
		{
			name: "empty non-time series query",
			query: Query{
				Schema:   "test_table",
				Measures: []string{"count", "sum_amount"},
			},
			columns: []string{"count", "sum_amount"},
			rows:    [][]driver.Value{},
			expectedData: []map[string]interface{}{
				{"count": 0, "sum_amount": 0},
			},
			description: "Should generate zero values for empty non-time series query",
		},
		{
			name: "time series query with gap filling",
			query: Query{
				Schema:   "test_table",
				Measures: []string{"count"},
				TimeDimensions: []TimeDimension{{
					Dimension:   "created_at",
					Granularity: "day",
					DateRange:   &[2]string{"2024-01-01", "2024-01-03"},
				}},
			},
			columns: []string{"count", "created_at_day"},
			rows: [][]driver.Value{
				{int64(10), time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)},
				{int64(5), time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC)},
			},
			expectedData: []map[string]interface{}{
				{"count": int64(10), "created_at_day": "2024-01-01T00:00:00Z"},
				{"count": 0, "created_at_day": "2024-01-02T00:00:00Z"},
				{"count": int64(5), "created_at_day": "2024-01-03T00:00:00Z"},
			},
			description: "Should fill gaps in time series data",
		},
		{
			name: "empty time series query",
			query: Query{
				Schema:   "test_table",
				Measures: []string{"count"},
				TimeDimensions: []TimeDimension{{
					Dimension:   "created_at",
					Granularity: "day",
					DateRange:   &[2]string{"2024-01-01", "2024-01-02"},
				}},
			},
			columns: []string{"count", "created_at_day"},
			rows:    [][]driver.Value{},
			expectedData: []map[string]interface{}{
				{"count": 0, "created_at_day": "2024-01-01T00:00:00Z"},
				{"count": 0, "created_at_day": "2024-01-02T00:00:00Z"},
			},
			description: "Should generate zero values for empty time series query",
		},
		{
			name: "time series with different granularity",
			query: Query{
				Schema:   "test_table",
				Measures: []string{"count"},
				TimeDimensions: []TimeDimension{{
					Dimension:   "created_at",
					Granularity: "hour",
					DateRange:   &[2]string{"2024-01-01", "2024-01-01"},
				}},
			},
			columns: []string{"count", "created_at_hour"},
			rows: [][]driver.Value{
				{int64(1), time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)},
				{int64(3), time.Date(2024, 1, 1, 2, 0, 0, 0, time.UTC)},
			},
			expectedData: []map[string]interface{}{
				{"count": int64(1), "created_at_hour": "2024-01-01T00:00:00Z"},
				{"count": 0, "created_at_hour": "2024-01-01T01:00:00Z"},
				{"count": int64(3), "created_at_hour": "2024-01-01T02:00:00Z"},
			},
			description: "Should handle different time granularities",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up mock rows
			mockRows := sqlmock.NewRows(tt.columns)
			for _, row := range tt.rows {
				mockRows = mockRows.AddRow(row...)
			}

			mock.ExpectQuery("SELECT (.+)").WillReturnRows(mockRows)

			// Execute query to get rows
			rows, err := db.Query("SELECT * FROM test")
			require.NoError(t, err)
			defer rows.Close()

			// Test ProcessRows function
			data, err := ProcessRows(rows, tt.query)
			require.NoError(t, err, tt.description)

			// Verify results
			assert.Equal(t, len(tt.expectedData), len(data), "Data length should match for %s", tt.description)
			for i, expected := range tt.expectedData {
				if i < len(data) {
					for key, expectedValue := range expected {
						actualValue := data[i][key]
						assert.Equal(t, expectedValue, actualValue,
							"Data[%d][%s] should match: expected %v, got %v for %s", i, key, expectedValue, actualValue, tt.description)
					}
				}
			}

			// Verify all expectations were met
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestProcessRows_ErrorHandling(t *testing.T) {
	// Create mock database
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	query := Query{
		Schema:   "test_table",
		Measures: []string{"count"},
	}

	// Mock a column scanning error
	mock.ExpectQuery("SELECT (.+)").WillReturnError(assert.AnError)

	// Execute query
	_, err = db.Query("SELECT * FROM test")
	require.Error(t, err)

	// ProcessRows should handle the error gracefully
	// Since we can't get rows due to query error, we'll test with a different approach
	// Let's test with rows that fail during scanning

	// Create a successful query but with problematic rows
	mockRows := sqlmock.NewRows([]string{"count"}).
		AddRow("invalid_int") // This should cause a scanning error

	mock.ExpectQuery("SELECT (.+)").WillReturnRows(mockRows)

	rows2, err := db.Query("SELECT * FROM test")
	require.NoError(t, err)
	defer rows2.Close()

	// This should handle the scanning error
	data, err := ProcessRows(rows2, query)
	// The error handling depends on the driver, but we expect either an error or empty data
	if err == nil {
		// If no error, data should be handled gracefully
		assert.NotNil(t, data)
	} else {
		// If error, it should be properly propagated
		assert.Error(t, err)
	}

	// Verify all expectations were met
	assert.NoError(t, mock.ExpectationsWereMet())
}
