package analytics

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefaultValidate(t *testing.T) {
	// Create test schemas
	testSchemas := map[string]SchemaDefinition{
		"message_history": {
			Name: "message_history",
			Measures: map[string]MeasureDefinition{
				"count_sent":      {Type: "count", SQL: "COUNT(*) FILTER (WHERE sent_at IS NOT NULL)", Description: "Total sent messages"},
				"count_delivered": {Type: "count", SQL: "COUNT(*) FILTER (WHERE delivered_at IS NOT NULL)", Description: "Total delivered messages"},
			},
			Dimensions: map[string]DimensionDefinition{
				"created_at":    {Type: "time", SQL: "created_at", Description: "Creation timestamp"},
				"contact_email": {Type: "string", SQL: "contact_email", Description: "Recipient email"},
			},
		},
	}

	tests := []struct {
		name    string
		query   Query
		wantErr bool
		errType error
	}{
		{
			name: "valid query",
			query: Query{
				Schema:     "message_history",
				Measures:   []string{"count_sent", "count_delivered"},
				Dimensions: []string{"contact_email"},
				TimeDimensions: []TimeDimension{{
					Dimension:   "created_at",
					Granularity: "day",
				}},
			},
			wantErr: false,
		},
		{
			name: "invalid schema",
			query: Query{
				Schema:   "invalid_schema",
				Measures: []string{"count"},
			},
			wantErr: true,
			errType: ErrInvalidSchema,
		},
		{
			name: "invalid measure",
			query: Query{
				Schema:   "message_history",
				Measures: []string{"invalid_measure"},
			},
			wantErr: true,
			errType: ErrUnsupportedMeasure,
		},
		{
			name: "invalid dimension",
			query: Query{
				Schema:     "message_history",
				Measures:   []string{"count_sent"},
				Dimensions: []string{"invalid_dimension"},
			},
			wantErr: true,
			errType: ErrUnsupportedDimension,
		},
		{
			name: "invalid granularity",
			query: Query{
				Schema:   "message_history",
				Measures: []string{"count_sent"},
				TimeDimensions: []TimeDimension{{
					Dimension:   "created_at",
					Granularity: "invalid_granularity",
				}},
			},
			wantErr: true,
			errType: ErrUnsupportedGranularity,
		},
		{
			name: "invalid filter operator",
			query: Query{
				Schema:   "message_history",
				Measures: []string{"count_sent"},
				Filters: []Filter{{
					Member:   "contact_email",
					Operator: "invalid_operator",
					Values:   []string{"test@example.com"},
				}},
			},
			wantErr: true,
			errType: ErrUnsupportedOperator,
		},
		{
			name: "empty filter values",
			query: Query{
				Schema:   "message_history",
				Measures: []string{"count_sent"},
				Filters: []Filter{{
					Member:   "contact_email",
					Operator: "equals",
					Values:   []string{},
				}},
			},
			wantErr: true,
		},
		{
			name: "invalid timezone",
			query: Query{
				Schema:   "message_history",
				Measures: []string{"count_sent"},
				Timezone: stringPtr("Invalid/Timezone"),
			},
			wantErr: true,
			errType: ErrInvalidTimezone,
		},
		{
			name: "negative limit",
			query: Query{
				Schema:   "message_history",
				Measures: []string{"count_sent"},
				Limit:    intPtr(-1),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := DefaultValidate(tt.query, testSchemas)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errType != nil {
					assert.ErrorIs(t, err, tt.errType)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestQuery_Validate(t *testing.T) {
	testSchemas := map[string]SchemaDefinition{
		"test_schema": {
			Name: "test_schema",
			Measures: map[string]MeasureDefinition{
				"count": {Type: "count", SQL: "COUNT(*)", Description: "Total count"},
			},
			Dimensions: map[string]DimensionDefinition{
				"created_at": {Type: "time", SQL: "created_at", Description: "Creation timestamp"},
			},
		},
	}

	query := Query{
		Schema:   "test_schema",
		Measures: []string{"count"},
		TimeDimensions: []TimeDimension{{
			Dimension:   "created_at",
			Granularity: "day",
		}},
	}

	err := query.Validate(testSchemas)
	assert.NoError(t, err)
}

func TestQuery_ValidateInvalidSchema(t *testing.T) {
	testSchemas := map[string]SchemaDefinition{}

	query := Query{
		Schema:   "nonexistent_schema",
		Measures: []string{"count"},
	}

	err := query.Validate(testSchemas)
	assert.ErrorIs(t, err, ErrInvalidSchema)
}
