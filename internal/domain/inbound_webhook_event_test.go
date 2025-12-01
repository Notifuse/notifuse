package domain

import (
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewInboundWebhookEvent(t *testing.T) {
	// Arrange
	id := "test-id"
	eventType := EmailEventDelivered
	source := WebhookSourceSES
	integrationID := "integration-123"
	recipientEmail := "test@example.com"
	messageID := "message-123"
	timestamp := time.Now()
	rawPayload := `{"test": "payload"}`

	// Act
	event := NewInboundWebhookEvent(
		id,
		eventType,
		source,
		integrationID,
		recipientEmail,
		&messageID,
		timestamp,
		rawPayload,
	)

	// Assert
	assert.Equal(t, id, event.ID)
	assert.Equal(t, eventType, event.Type)
	assert.Equal(t, source, event.Source)
	assert.Equal(t, integrationID, event.IntegrationID)
	assert.Equal(t, recipientEmail, event.RecipientEmail)
	assert.NotNil(t, event.MessageID)
	assert.Equal(t, messageID, *event.MessageID)
	assert.Equal(t, timestamp, event.Timestamp)
	assert.Equal(t, rawPayload, event.RawPayload)
}

func TestErrInboundWebhookEventNotFound_Error(t *testing.T) {
	// Arrange
	id := "test-id-123"
	err := &ErrInboundWebhookEventNotFound{ID: id}

	// Act
	message := err.Error()

	// Assert
	expected := "inbound webhook event with ID test-id-123 not found"
	assert.Equal(t, expected, message)
}

func TestInboundWebhookEventListParams_FromQuery(t *testing.T) {
	tests := []struct {
		name        string
		queryParams map[string]string
		expected    InboundWebhookEventListParams
		expectError bool
	}{
		{
			name: "valid basic parameters",
			queryParams: map[string]string{
				"workspace_id":    "workspace-123",
				"cursor":          "cursor-abc",
				"limit":           "50",
				"event_type":      "delivered",
				"recipient_email": "test@example.com",
				"message_id":      "msg-123",
			},
			expected: InboundWebhookEventListParams{
				WorkspaceID:    "workspace-123",
				Cursor:         "cursor-abc",
				Limit:          50,
				EventType:      EmailEventDelivered,
				RecipientEmail: "test@example.com",
				MessageID:      "msg-123",
			},
			expectError: false,
		},
		{
			name: "minimal valid parameters",
			queryParams: map[string]string{
				"workspace_id": "workspace-123",
			},
			expected: InboundWebhookEventListParams{
				WorkspaceID: "workspace-123",
				Limit:       20, // default limit
			},
			expectError: false,
		},
		{
			name: "with time filters",
			queryParams: map[string]string{
				"workspace_id":     "workspace-123",
				"timestamp_after":  "2023-01-01T00:00:00Z",
				"timestamp_before": "2023-12-31T23:59:59Z",
			},
			expected: InboundWebhookEventListParams{
				WorkspaceID:     "workspace-123",
				Limit:           20,
				TimestampAfter:  timePtr(time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)),
				TimestampBefore: timePtr(time.Date(2023, 12, 31, 23, 59, 59, 0, time.UTC)),
			},
			expectError: false,
		},
		{
			name: "invalid limit",
			queryParams: map[string]string{
				"workspace_id": "workspace-123",
				"limit":        "invalid",
			},
			expectError: true,
		},
		{
			name: "missing workspace_id",
			queryParams: map[string]string{
				"limit": "10",
			},
			expectError: true,
		},
		{
			name: "invalid email",
			queryParams: map[string]string{
				"workspace_id":    "workspace-123",
				"recipient_email": "invalid-email",
			},
			expectError: true,
		},
		{
			name: "invalid event type",
			queryParams: map[string]string{
				"workspace_id": "workspace-123",
				"event_type":   "invalid_type",
			},
			expectError: true,
		},
		{
			name: "limit exceeds maximum",
			queryParams: map[string]string{
				"workspace_id": "workspace-123",
				"limit":        "150",
			},
			expected: InboundWebhookEventListParams{
				WorkspaceID: "workspace-123",
				Limit:       100, // capped at maximum
			},
			expectError: false,
		},
		{
			name: "negative limit",
			queryParams: map[string]string{
				"workspace_id": "workspace-123",
				"limit":        "-10",
			},
			expectError: true,
		},
		{
			name: "invalid timestamp format",
			queryParams: map[string]string{
				"workspace_id":    "workspace-123",
				"timestamp_after": "invalid-date",
			},
			expectError: true,
		},
		{
			name: "timestamp_after after timestamp_before",
			queryParams: map[string]string{
				"workspace_id":     "workspace-123",
				"timestamp_after":  "2023-12-31T23:59:59Z",
				"timestamp_before": "2023-01-01T00:00:00Z",
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			query := url.Values{}
			for key, value := range tt.queryParams {
				query.Set(key, value)
			}

			params := &InboundWebhookEventListParams{}

			// Act
			err := params.FromQuery(query)

			// Assert
			if tt.expectError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected.WorkspaceID, params.WorkspaceID)
				assert.Equal(t, tt.expected.Cursor, params.Cursor)
				assert.Equal(t, tt.expected.Limit, params.Limit)
				assert.Equal(t, tt.expected.EventType, params.EventType)
				assert.Equal(t, tt.expected.RecipientEmail, params.RecipientEmail)
				assert.Equal(t, tt.expected.MessageID, params.MessageID)

				if tt.expected.TimestampAfter != nil {
					require.NotNil(t, params.TimestampAfter)
					assert.True(t, tt.expected.TimestampAfter.Equal(*params.TimestampAfter))
				} else {
					assert.Nil(t, params.TimestampAfter)
				}

				if tt.expected.TimestampBefore != nil {
					require.NotNil(t, params.TimestampBefore)
					assert.True(t, tt.expected.TimestampBefore.Equal(*params.TimestampBefore))
				} else {
					assert.Nil(t, params.TimestampBefore)
				}
			}
		})
	}
}

func TestInboundWebhookEventListParams_Validate(t *testing.T) {
	tests := []struct {
		name        string
		params      InboundWebhookEventListParams
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid parameters",
			params: InboundWebhookEventListParams{
				WorkspaceID:    "workspace-123",
				Limit:          50,
				EventType:      EmailEventDelivered,
				RecipientEmail: "test@example.com",
			},
			expectError: false,
		},
		{
			name: "missing workspace_id",
			params: InboundWebhookEventListParams{
				Limit: 50,
			},
			expectError: true,
			errorMsg:    "workspace_id is required",
		},
		{
			name: "negative limit",
			params: InboundWebhookEventListParams{
				WorkspaceID: "workspace-123",
				Limit:       -10,
			},
			expectError: true,
			errorMsg:    "limit cannot be negative",
		},
		{
			name: "invalid event type",
			params: InboundWebhookEventListParams{
				WorkspaceID: "workspace-123",
				EventType:   "invalid_type",
			},
			expectError: true,
			errorMsg:    "invalid event type: invalid_type",
		},
		{
			name: "invalid email format",
			params: InboundWebhookEventListParams{
				WorkspaceID:    "workspace-123",
				RecipientEmail: "invalid-email",
			},
			expectError: true,
			errorMsg:    "invalid contact email format",
		},
		{
			name: "timestamp_after after timestamp_before",
			params: InboundWebhookEventListParams{
				WorkspaceID:     "workspace-123",
				TimestampAfter:  timePtr(time.Date(2023, 12, 31, 0, 0, 0, 0, time.UTC)),
				TimestampBefore: timePtr(time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)),
			},
			expectError: true,
			errorMsg:    "timestamp_after must be before timestamp_before",
		},
		{
			name: "valid time range",
			params: InboundWebhookEventListParams{
				WorkspaceID:     "workspace-123",
				TimestampAfter:  timePtr(time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)),
				TimestampBefore: timePtr(time.Date(2023, 12, 31, 0, 0, 0, 0, time.UTC)),
			},
			expectError: false,
		},
		{
			name: "limit exceeds maximum gets capped",
			params: InboundWebhookEventListParams{
				WorkspaceID: "workspace-123",
				Limit:       150,
			},
			expectError: false,
		},
		{
			name: "zero limit gets default",
			params: InboundWebhookEventListParams{
				WorkspaceID: "workspace-123",
				Limit:       0,
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			err := tt.params.Validate()

			// Assert
			if tt.expectError {
				require.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)

				// Check that limits are properly set
				if tt.params.Limit > 100 {
					assert.Equal(t, 100, tt.params.Limit)
				} else if tt.params.Limit == 0 {
					assert.Equal(t, 20, tt.params.Limit)
				}
			}
		})
	}
}

func TestEmailEventType_Constants(t *testing.T) {
	// Test that the constants are defined correctly
	assert.Equal(t, EmailEventType("delivered"), EmailEventDelivered)
	assert.Equal(t, EmailEventType("bounce"), EmailEventBounce)
	assert.Equal(t, EmailEventType("complaint"), EmailEventComplaint)
	assert.Equal(t, EmailEventType("auth_email"), EmailEventAuthEmail)
	assert.Equal(t, EmailEventType("before_user_created"), EmailEventBeforeUserCreated)
}

func TestWebhookSource_Constants(t *testing.T) {
	// Test that the webhook source constants are defined correctly
	assert.Equal(t, WebhookSource("ses"), WebhookSourceSES)
	assert.Equal(t, WebhookSource("postmark"), WebhookSourcePostmark)
	assert.Equal(t, WebhookSource("mailgun"), WebhookSourceMailgun)
	assert.Equal(t, WebhookSource("sparkpost"), WebhookSourceSparkPost)
	assert.Equal(t, WebhookSource("mailjet"), WebhookSourceMailjet)
	assert.Equal(t, WebhookSource("smtp"), WebhookSourceSMTP)
	assert.Equal(t, WebhookSource("supabase"), WebhookSourceSupabase)
}

func TestNewInboundWebhookEvent_NullableMessageID(t *testing.T) {
	// Test Supabase webhook with nil message_id
	id := "test-id"
	eventType := EmailEventAuthEmail
	source := WebhookSourceSupabase
	integrationID := "integration-123"
	recipientEmail := "test@example.com"
	timestamp := time.Now()
	rawPayload := `{"test": "payload"}`

	event := NewInboundWebhookEvent(
		id,
		eventType,
		source,
		integrationID,
		recipientEmail,
		nil, // Supabase webhooks don't have message_id
		timestamp,
		rawPayload,
	)

	assert.Equal(t, id, event.ID)
	assert.Equal(t, eventType, event.Type)
	assert.Equal(t, source, event.Source)
	assert.Equal(t, integrationID, event.IntegrationID)
	assert.Equal(t, recipientEmail, event.RecipientEmail)
	assert.Nil(t, event.MessageID)
	assert.Equal(t, timestamp, event.Timestamp)
	assert.Equal(t, rawPayload, event.RawPayload)
}

func TestInboundWebhookEvent_BounceFields(t *testing.T) {
	// Test that bounce-specific fields can be set
	event := &InboundWebhookEvent{
		ID:               "test-id",
		Type:             EmailEventBounce,
		BounceType:       "permanent",
		BounceCategory:   "suppressed",
		BounceDiagnostic: "Email address is suppressed",
	}

	assert.Equal(t, "permanent", event.BounceType)
	assert.Equal(t, "suppressed", event.BounceCategory)
	assert.Equal(t, "Email address is suppressed", event.BounceDiagnostic)
}

func TestInboundWebhookEvent_ComplaintFields(t *testing.T) {
	// Test that complaint-specific fields can be set
	event := &InboundWebhookEvent{
		ID:                    "test-id",
		Type:                  EmailEventComplaint,
		ComplaintFeedbackType: "abuse",
	}

	assert.Equal(t, "abuse", event.ComplaintFeedbackType)
}

func TestInboundWebhookEventListResult(t *testing.T) {
	// Test the result structure
	events := []*InboundWebhookEvent{
		{ID: "event-1", Type: EmailEventDelivered},
		{ID: "event-2", Type: EmailEventBounce},
	}

	result := &InboundWebhookEventListResult{
		Events:     events,
		NextCursor: "next-cursor",
		HasMore:    true,
	}

	assert.Len(t, result.Events, 2)
	assert.Equal(t, "next-cursor", result.NextCursor)
	assert.True(t, result.HasMore)
}
