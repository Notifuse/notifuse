package analytics

import (
	"context"
	"database/sql/driver"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQuery_GetDefaultTimezone(t *testing.T) {
	tests := []struct {
		name     string
		timezone *string
		expected string
	}{
		{
			name:     "no timezone set",
			timezone: nil,
			expected: "UTC",
		},
		{
			name:     "timezone set",
			timezone: stringPtr("America/New_York"),
			expected: "America/New_York",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := Query{Timezone: tt.timezone}
			assert.Equal(t, tt.expected, q.GetDefaultTimezone())
		})
	}
}

func TestQuery_HasTimeDimensions(t *testing.T) {
	tests := []struct {
		name           string
		timeDimensions []TimeDimension
		expected       bool
	}{
		{
			name:           "no time dimensions",
			timeDimensions: []TimeDimension{},
			expected:       false,
		},
		{
			name: "has time dimensions",
			timeDimensions: []TimeDimension{{
				Dimension:   "created_at",
				Granularity: "day",
			}},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := Query{TimeDimensions: tt.timeDimensions}
			assert.Equal(t, tt.expected, q.HasTimeDimensions())
		})
	}
}

func TestQuery_GetLimit(t *testing.T) {
	tests := []struct {
		name     string
		limit    *int
		expected int
	}{
		{
			name:     "no limit set",
			limit:    nil,
			expected: 1000,
		},
		{
			name:     "limit set",
			limit:    intPtr(50),
			expected: 50,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := Query{Limit: tt.limit}
			assert.Equal(t, tt.expected, q.GetLimit())
		})
	}
}

func TestQuery_GetOffset(t *testing.T) {
	tests := []struct {
		name     string
		offset   *int
		expected int
	}{
		{
			name:     "no offset set",
			offset:   nil,
			expected: 0,
		},
		{
			name:     "offset set",
			offset:   intPtr(100),
			expected: 100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := Query{Offset: tt.offset}
			assert.Equal(t, tt.expected, q.GetOffset())
		})
	}
}

func TestMeta(t *testing.T) {
	meta := Meta{
		Total:         100,
		ExecutionTime: 500 * time.Millisecond,
		Query:         "SELECT COUNT(*) FROM message_history",
		Params:        []interface{}{"workspace-123"},
	}

	assert.Equal(t, 100, meta.Total)
	assert.Equal(t, 500*time.Millisecond, meta.ExecutionTime)
	assert.Equal(t, "SELECT COUNT(*) FROM message_history", meta.Query)
	assert.Equal(t, []interface{}{"workspace-123"}, meta.Params)
}

func TestQuery_Query(t *testing.T) {
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
				SQL:         "COUNT(*)",
				Description: "Count of sent messages",
				Filters: []MeasureFilter{
					{SQL: "sent_at IS NOT NULL"},
				},
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
				Description: "Contact email",
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
		mockRows      [][]driver.Value
		mockColumns   []string
		expectedData  []map[string]interface{}
		expectedError bool
		description   string
	}{
		{
			name: "simple count query",
			query: Query{
				Schema:   "message_history",
				Measures: []string{"count"},
			},
			mockRows:    [][]driver.Value{{int64(42)}},
			mockColumns: []string{"count"},
			expectedData: []map[string]interface{}{
				{"count": int64(42)},
			},
			description: "Should execute simple count query and return results",
		},
		{
			name: "query with dimensions",
			query: Query{
				Schema:     "message_history",
				Measures:   []string{"count"},
				Dimensions: []string{"contact_email"},
			},
			mockRows:    [][]driver.Value{{int64(5), "test@example.com"}, {int64(3), "user@example.com"}},
			mockColumns: []string{"count", "contact_email"},
			expectedData: []map[string]interface{}{
				{"count": int64(5), "contact_email": "test@example.com"},
				{"count": int64(3), "contact_email": "user@example.com"},
			},
			description: "Should execute query with dimensions and return grouped results",
		},
		{
			name: "query with time dimension and gap filling",
			query: Query{
				Schema:   "message_history",
				Measures: []string{"count"},
				TimeDimensions: []TimeDimension{{
					Dimension:   "created_at",
					Granularity: "day",
					DateRange:   &[2]string{"2024-01-01", "2024-01-03"},
				}},
			},
			mockRows:    [][]driver.Value{{int64(10), time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)}},
			mockColumns: []string{"count", "created_at_day"},
			expectedData: []map[string]interface{}{
				{"count": int64(10), "created_at_day": "2024-01-01T00:00:00Z"},
				{"count": 0, "created_at_day": "2024-01-02T00:00:00Z"},
				{"count": 0, "created_at_day": "2024-01-03T00:00:00Z"},
			},
			description: "Should fill time series gaps with zero values",
		},
		{
			name: "query with measure filters",
			query: Query{
				Schema:   "message_history",
				Measures: []string{"count_sent"},
			},
			mockRows:    [][]driver.Value{{int64(25)}},
			mockColumns: []string{"count_sent"},
			expectedData: []map[string]interface{}{
				{"count_sent": int64(25)},
			},
			description: "Should execute query with measure filters",
		},
		{
			name: "empty result set",
			query: Query{
				Schema:   "message_history",
				Measures: []string{"count"},
			},
			mockRows:    [][]driver.Value{},
			mockColumns: []string{"count"},
			expectedData: []map[string]interface{}{
				{"count": 0},
			},
			description: "Should return zero values for empty result set",
		},
		{
			name: "invalid measure",
			query: Query{
				Schema:   "message_history",
				Measures: []string{"invalid_measure"},
			},
			expectedError: true,
			description:   "Should return error for invalid measure",
		},
		{
			name: "invalid dimension",
			query: Query{
				Schema:     "message_history",
				Measures:   []string{"count"},
				Dimensions: []string{"invalid_dimension"},
			},
			expectedError: true,
			description:   "Should return error for invalid dimension",
		},
		{
			name: "query with new operators - set/notSet",
			query: Query{
				Schema:   "message_history",
				Measures: []string{"count"},
				Filters: []Filter{
					{
						Member:   "broadcast_id",
						Operator: "set",
						Values:   []string{},
					},
				},
			},
			mockRows:    [][]driver.Value{{int64(15)}},
			mockColumns: []string{"count"},
			expectedData: []map[string]interface{}{
				{"count": int64(15)},
			},
			description: "Should execute query with set operator (not null check)",
		},
		{
			name: "query with string operators",
			query: Query{
				Schema:   "message_history",
				Measures: []string{"count"},
				Filters: []Filter{
					{
						Member:   "contact_email",
						Operator: "startsWith",
						Values:   []string{"admin"},
					},
				},
			},
			mockRows:    [][]driver.Value{{int64(8)}},
			mockColumns: []string{"count"},
			expectedData: []map[string]interface{}{
				{"count": int64(8)},
			},
			description: "Should execute query with startsWith operator",
		},
		{
			name: "query with date range operators",
			query: Query{
				Schema:   "message_history",
				Measures: []string{"count"},
				Filters: []Filter{
					{
						Member:   "created_at",
						Operator: "inDateRange",
						Values:   []string{"2024-01-01", "2024-03-31"},
					},
				},
			},
			mockRows:    [][]driver.Value{{int64(120)}},
			mockColumns: []string{"count"},
			expectedData: []map[string]interface{}{
				{"count": int64(120)},
			},
			description: "Should execute query with inDateRange operator",
		},
		{
			name: "query with multiple new operators",
			query: Query{
				Schema:   "message_history",
				Measures: []string{"count"},
				Filters: []Filter{
					{
						Member:   "broadcast_id",
						Operator: "notSet",
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
			mockRows:    [][]driver.Value{{int64(45)}},
			mockColumns: []string{"count"},
			expectedData: []map[string]interface{}{
				{"count": int64(45)},
			},
			description: "Should execute query with multiple new operators",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a fresh mock for each test to avoid conflicts
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer func() { _ = db.Close() }()

			if !tt.expectedError {
				// Set up mock expectations
				rows := sqlmock.NewRows(tt.mockColumns)
				for _, row := range tt.mockRows {
					rows = rows.AddRow(row...)
				}

				mock.ExpectQuery(".*").WillReturnRows(rows)
			}

			// Execute the query
			ctx := context.Background()
			response, err := tt.query.Query(ctx, db, schema)

			if tt.expectedError {
				assert.Error(t, err, tt.description)
				return
			}

			require.NoError(t, err, tt.description)
			require.NotNil(t, response, "Response should not be nil")

			// Check data
			assert.Equal(t, len(tt.expectedData), len(response.Data), "Data length should match")
			for i, expected := range tt.expectedData {
				if i < len(response.Data) {
					for key, expectedValue := range expected {
						actualValue := response.Data[i][key]
						assert.Equal(t, expectedValue, actualValue,
							"Data[%d][%s] should match: expected %v, got %v", i, key, expectedValue, actualValue)
					}
				}
			}

			// Check metadata
			assert.NotEmpty(t, response.Meta.Query, "SQL query should be recorded in meta")
			assert.Equal(t, len(response.Data), response.Meta.Total, "Meta total should match data length")

			// Verify all expectations were met
			assert.NoError(t, mock.ExpectationsWereMet(), "All mock expectations should be met")
		})
	}
}

func TestQuery_QueryValidation(t *testing.T) {
	// Create mock database
	db, _, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	schema := SchemaDefinition{
		Name: "test_table",
		Measures: map[string]MeasureDefinition{
			"count": {Type: "count", SQL: "COUNT(*)"},
		},
		Dimensions: map[string]DimensionDefinition{
			"status": {Type: "string", SQL: "status"},
		},
	}

	tests := []struct {
		name        string
		query       Query
		expectedErr string
		description string
	}{
		{
			name: "invalid schema name",
			query: Query{
				Schema:   "nonexistent_schema",
				Measures: []string{"count"},
			},
			expectedErr: "invalid schema",
			description: "Should validate schema existence",
		},
		{
			name: "invalid measure",
			query: Query{
				Schema:   "test_table",
				Measures: []string{"invalid_measure"},
			},
			expectedErr: "unsupported measure",
			description: "Should validate measure existence",
		},
		{
			name: "invalid dimension",
			query: Query{
				Schema:     "test_table",
				Measures:   []string{"count"},
				Dimensions: []string{"invalid_dimension"},
			},
			expectedErr: "unsupported dimension",
			description: "Should validate dimension existence",
		},
		{
			name: "invalid timezone",
			query: Query{
				Schema:   "test_table",
				Measures: []string{"count"},
				Timezone: stringPtr("Invalid/Timezone"),
			},
			expectedErr: "invalid timezone",
			description: "Should validate timezone",
		},
		{
			name: "negative limit",
			query: Query{
				Schema:   "test_table",
				Measures: []string{"count"},
				Limit:    intPtr(-1),
			},
			expectedErr: "limit cannot be negative",
			description: "Should validate limit is not negative",
		},
		{
			name: "negative offset",
			query: Query{
				Schema:   "test_table",
				Measures: []string{"count"},
				Offset:   intPtr(-1),
			},
			expectedErr: "offset cannot be negative",
			description: "Should validate offset is not negative",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			_, err := tt.query.Query(ctx, db, schema)

			require.Error(t, err, tt.description)
			assert.Contains(t, err.Error(), tt.expectedErr, tt.description)
		})
	}
}

func TestQuery_QueryWithDatabase(t *testing.T) {
	// Integration test with mock database
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	schema := SchemaDefinition{
		Name: "analytics_test",
		Measures: map[string]MeasureDefinition{
			"total_count": {
				Type:        "count",
				SQL:         "COUNT(*)",
				Description: "Total records",
			},
			"sum_amount": {
				Type:        "sum",
				SQL:         "amount",
				Description: "Sum of amounts",
			},
		},
		Dimensions: map[string]DimensionDefinition{
			"category": {
				Type:        "string",
				SQL:         "category",
				Description: "Category field",
			},
			"created_at": {
				Type:        "time",
				SQL:         "created_at",
				Description: "Creation time",
			},
		},
	}

	query := Query{
		Schema:     "analytics_test",
		Measures:   []string{"total_count", "sum_amount"},
		Dimensions: []string{"category"},
		Filters: []Filter{{
			Member:   "category",
			Operator: "equals",
			Values:   []string{"premium"},
		}},
		Order: map[string]string{
			"sum_amount": "desc",
		},
		Limit: intPtr(10),
	}

	// Set up mock expectations
	expectedSQL := "SELECT (.+) FROM analytics_test WHERE (.+) GROUP BY (.+) ORDER BY (.+) LIMIT (.+)"
	rows := sqlmock.NewRows([]string{"total_count", "sum_amount", "category"}).
		AddRow(int64(100), float64(50000.50), "premium").
		AddRow(int64(75), float64(25000.25), "premium")

	mock.ExpectQuery(expectedSQL).
		WithArgs("premium").
		WillReturnRows(rows)

	// Execute query
	ctx := context.Background()
	response, err := query.Query(ctx, db, schema)

	require.NoError(t, err)
	require.NotNil(t, response)

	// Verify response structure
	assert.Len(t, response.Data, 2, "Should return 2 rows")
	assert.Equal(t, 2, response.Meta.Total, "Meta total should be 2")
	assert.NotEmpty(t, response.Meta.Query, "Should record executed SQL")
	assert.Len(t, response.Meta.Params, 1, "Should have 1 parameter")
	assert.Equal(t, "premium", response.Meta.Params[0], "Parameter should be 'premium'")

	// Verify data content
	firstRow := response.Data[0]
	assert.Equal(t, int64(100), firstRow["total_count"])
	assert.Equal(t, float64(50000.50), firstRow["sum_amount"])
	assert.Equal(t, "premium", firstRow["category"])

	// Verify all expectations were met
	assert.NoError(t, mock.ExpectationsWereMet())
}
