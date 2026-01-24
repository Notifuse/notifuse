package domain

import (
	"testing"

	"github.com/Notifuse/notifuse/pkg/analytics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPredefinedSchemas(t *testing.T) {
	// Test that all expected schemas exist
	expectedSchemas := []string{"message_history", "contacts", "broadcasts", "webhook_deliveries", "email_queue", "automation_node_executions"}

	for _, schemaName := range expectedSchemas {
		t.Run("schema_"+schemaName, func(t *testing.T) {
			schema, exists := PredefinedSchemas[schemaName]
			assert.True(t, exists, "Schema %s should exist", schemaName)
			assert.Equal(t, schemaName, schema.Name)
			assert.NotEmpty(t, schema.Measures, "Schema %s should have measures", schemaName)
			assert.NotEmpty(t, schema.Dimensions, "Schema %s should have dimensions", schemaName)
		})
	}
}

func TestAnalyticsQueryValidation(t *testing.T) {
	tests := []struct {
		name    string
		query   analytics.Query
		wantErr bool
		errType error
	}{
		{
			name: "valid message_history query",
			query: analytics.Query{
				Schema:     "message_history",
				Measures:   []string{"count_sent", "count_delivered"},
				Dimensions: []string{"contact_email"},
				TimeDimensions: []analytics.TimeDimension{{
					Dimension:   "created_at",
					Granularity: "day",
				}},
			},
			wantErr: false,
		},
		{
			name: "valid message_history query with external_id dimension for transactional stats",
			query: analytics.Query{
				Schema:     "message_history",
				Measures:   []string{"count_sent", "count_delivered"},
				Dimensions: []string{"external_id"},
				Filters: []analytics.Filter{{
					Member:   "broadcast_id",
					Operator: "notSet",
					Values:   []string{},
				}},
			},
			wantErr: false,
		},
		{
			name: "invalid schema",
			query: analytics.Query{
				Schema:   "invalid_schema",
				Measures: []string{"count"},
			},
			wantErr: true,
			errType: analytics.ErrInvalidSchema,
		},
		{
			name: "invalid measure",
			query: analytics.Query{
				Schema:   "message_history",
				Measures: []string{"invalid_measure"},
			},
			wantErr: true,
			errType: analytics.ErrUnsupportedMeasure,
		},
		{
			name: "invalid dimension",
			query: analytics.Query{
				Schema:     "message_history",
				Measures:   []string{"count_sent"},
				Dimensions: []string{"invalid_dimension"},
			},
			wantErr: true,
			errType: analytics.ErrUnsupportedDimension,
		},
		{
			name: "invalid granularity",
			query: analytics.Query{
				Schema:   "message_history",
				Measures: []string{"count_sent"},
				TimeDimensions: []analytics.TimeDimension{{
					Dimension:   "created_at",
					Granularity: "invalid_granularity",
				}},
			},
			wantErr: true,
			errType: analytics.ErrUnsupportedGranularity,
		},
		{
			name: "invalid filter operator",
			query: analytics.Query{
				Schema:   "message_history",
				Measures: []string{"count_sent"},
				Filters: []analytics.Filter{{
					Member:   "contact_email",
					Operator: "invalid_operator",
					Values:   []string{"test@example.com"},
				}},
			},
			wantErr: true,
			errType: analytics.ErrUnsupportedOperator,
		},
		{
			name: "empty filter values",
			query: analytics.Query{
				Schema:   "message_history",
				Measures: []string{"count_sent"},
				Filters: []analytics.Filter{{
					Member:   "contact_email",
					Operator: "equals",
					Values:   []string{},
				}},
			},
			wantErr: true,
		},
		{
			name: "invalid timezone",
			query: analytics.Query{
				Schema:   "message_history",
				Measures: []string{"count_sent"},
				Timezone: &[]string{"Invalid/Timezone"}[0],
			},
			wantErr: true,
			errType: analytics.ErrInvalidTimezone,
		},
		{
			name: "negative limit",
			query: analytics.Query{
				Schema:   "message_history",
				Measures: []string{"count_sent"},
				Limit:    intPtr(-1),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.query.Validate(PredefinedSchemas)
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

func TestSchemaDefinitionStructure(t *testing.T) {
	// Test message_history schema structure
	schema := PredefinedSchemas["message_history"]

	// Test required measures
	requiredMeasures := []string{"count", "count_sent", "count_delivered", "count_bounced", "count_complained", "count_opened", "count_clicked", "count_unsubscribed", "count_failed"}
	for _, measure := range requiredMeasures {
		assert.Contains(t, schema.Measures, measure, "message_history should have measure %s", measure)
	}

	// Test required dimensions
	requiredDimensions := []string{"created_at", "sent_at", "contact_email", "broadcast_id", "channel", "template_id", "external_id"}
	for _, dimension := range requiredDimensions {
		assert.Contains(t, schema.Dimensions, dimension, "message_history should have dimension %s", dimension)
	}
}

func TestContactsSchema(t *testing.T) {
	schema := PredefinedSchemas["contacts"]

	// Test measures
	assert.Contains(t, schema.Measures, "count")

	// Test key dimensions
	requiredDimensions := []string{"created_at", "email", "first_name", "last_name", "external_id", "timezone", "country"}
	for _, dimension := range requiredDimensions {
		assert.Contains(t, schema.Dimensions, dimension, "contacts should have dimension %s", dimension)
	}
}

func TestBroadcastsSchema(t *testing.T) {
	schema := PredefinedSchemas["broadcasts"]

	// Test measures
	requiredMeasures := []string{"count", "test_recipients", "winner_recipients"}
	for _, measure := range requiredMeasures {
		assert.Contains(t, schema.Measures, measure, "broadcasts should have measure %s", measure)
	}

	// Test key dimensions
	requiredDimensions := []string{"id", "name", "status", "created_at", "started_at", "completed_at", "workspace_id"}
	for _, dimension := range requiredDimensions {
		assert.Contains(t, schema.Dimensions, dimension, "broadcasts should have dimension %s", dimension)
	}
}

func TestEmailQueueSchema(t *testing.T) {
	schema := PredefinedSchemas["email_queue"]

	// Test measures
	requiredMeasures := []string{"count", "count_pending", "count_processing", "count_failed", "count_broadcast", "count_automation", "avg_attempts", "max_attempts", "count_retryable"}
	for _, measure := range requiredMeasures {
		assert.Contains(t, schema.Measures, measure, "email_queue should have measure %s", measure)
	}

	// Test dimensions
	requiredDimensions := []string{"created_at", "updated_at", "next_retry_at", "status", "source_type", "source_id", "integration_id", "provider_kind", "priority", "template_id", "contact_email"}
	for _, dimension := range requiredDimensions {
		assert.Contains(t, schema.Dimensions, dimension, "email_queue should have dimension %s", dimension)
	}
}

func TestAutomationNodeExecutionsSchema(t *testing.T) {
	schema := PredefinedSchemas["automation_node_executions"]

	// Test measures
	requiredMeasures := []string{"count", "count_entered", "count_completed", "count_failed", "count_skipped"}
	for _, measure := range requiredMeasures {
		assert.Contains(t, schema.Measures, measure, "automation_node_executions should have measure %s", measure)
	}

	// Test dimensions
	requiredDimensions := []string{"automation_id", "node_id", "node_type", "action", "entered_at"}
	for _, dimension := range requiredDimensions {
		assert.Contains(t, schema.Dimensions, dimension, "automation_node_executions should have dimension %s", dimension)
	}
}

func TestPredefinedSchemasWithFilters(t *testing.T) {
	// Test that our new filter-based measures generate valid SQL
	builder := analytics.NewSQLBuilder()

	tests := []struct {
		name     string
		schema   string
		measure  string
		expected string
	}{
		{
			name:     "message history - count sent",
			schema:   "message_history",
			measure:  "count_sent",
			expected: "COUNT(*) FILTER (WHERE sent_at IS NOT NULL)",
		},
		{
			name:     "message history - count sent emails",
			schema:   "message_history",
			measure:  "count_sent_emails",
			expected: "COUNT(*) FILTER (WHERE sent_at IS NOT NULL AND channel = 'email')",
		},
		{
			name:     "contacts - count active",
			schema:   "contacts",
			measure:  "count_active",
			expected: "COUNT(*) FILTER (WHERE status = 'active')",
		},
		{
			name:     "broadcasts - count completed",
			schema:   "broadcasts",
			measure:  "completed_broadcasts_count",
			expected: "COUNT(*) FILTER (WHERE status = 'completed')",
		},
		{
			name:     "broadcasts - avg recipients completed",
			schema:   "broadcasts",
			measure:  "avg_recipients_completed",
			expected: "AVG(recipient_count) FILTER (WHERE status = 'completed')",
		},
		{
			name:     "email_queue - count pending",
			schema:   "email_queue",
			measure:  "count_pending",
			expected: "COUNT(*) FILTER (WHERE status = 'pending')",
		},
		{
			name:     "email_queue - count retryable",
			schema:   "email_queue",
			measure:  "count_retryable",
			expected: "COUNT(*) FILTER (WHERE status = 'failed' AND attempts < max_attempts)",
		},
		{
			name:     "automation_node_executions - count entered",
			schema:   "automation_node_executions",
			measure:  "count_entered",
			expected: "COUNT(*) FILTER (WHERE action = 'entered')",
		},
		{
			name:     "automation_node_executions - count completed",
			schema:   "automation_node_executions",
			measure:  "count_completed",
			expected: "COUNT(*) FILTER (WHERE action = 'completed')",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			schema, exists := PredefinedSchemas[tt.schema]
			require.True(t, exists, "Schema %s should exist", tt.schema)

			query := analytics.Query{
				Schema:   tt.schema,
				Measures: []string{tt.measure},
			}

			sql, args, err := builder.BuildSQL(query, schema)
			require.NoError(t, err, "Should build SQL successfully")

			// Check that the expected filter pattern appears in the SQL
			assert.Contains(t, sql, tt.expected, "SQL should contain expected filter pattern")
			assert.Empty(t, args, "Should not have parameters for filter-based measures")

			t.Logf("Generated SQL: %s", sql)
		})
	}
}

// Helper functions
func intPtr(i int) *int {
	return &i
}
