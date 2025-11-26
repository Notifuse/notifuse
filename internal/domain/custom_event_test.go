package domain

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCustomEvent_Validate(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name    string
		event   CustomEvent
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid custom event with all fields",
			event: CustomEvent{
				ExternalID: "order_12345",
				Email:      "user@example.com",
				EventName:  "orders/fulfilled",
				Properties: map[string]interface{}{
					"total":    99.99,
					"items":    3,
					"currency": "USD",
				},
				OccurredAt: now,
				Source:     "api",
			},
			wantErr: false,
		},
		{
			name: "valid event with dotted notation",
			event: CustomEvent{
				ExternalID: "payment_123",
				Email:      "user@example.com",
				EventName:  "payment.succeeded",
				Properties: map[string]interface{}{"amount": 100},
				OccurredAt: now,
				Source:     "integration",
			},
			wantErr: false,
		},
		{
			name: "valid event with underscore notation",
			event: CustomEvent{
				ExternalID: "trial_123",
				Email:      "user@example.com",
				EventName:  "trial_started",
				Properties: map[string]interface{}{"plan": "pro"},
				OccurredAt: now,
				Source:     "import",
			},
			wantErr: false,
		},
		{
			name: "valid event with nil properties (should auto-initialize)",
			event: CustomEvent{
				ExternalID: "event_123",
				Email:      "user@example.com",
				EventName:  "event.test",
				Properties: nil,
				OccurredAt: now,
				Source:     "api",
			},
			wantErr: false,
		},
		{
			name: "valid event with integration_id",
			event: CustomEvent{
				ExternalID: "webhook_123",
				Email:      "user@example.com",
				EventName:  "customers/create",
				Properties: map[string]interface{}{},
				OccurredAt: now,
				Source:     "integration",
				IntegrationID: func() *string {
					s := "shopify_integration_1"
					return &s
				}(),
			},
			wantErr: false,
		},
		{
			name: "missing external_id",
			event: CustomEvent{
				Email:      "user@example.com",
				EventName:  "test.event",
				Properties: map[string]interface{}{},
				OccurredAt: now,
				Source:     "api",
			},
			wantErr: true,
			errMsg:  "external_id is required",
		},
		{
			name: "missing email",
			event: CustomEvent{
				ExternalID: "event_123",
				EventName:  "test.event",
				Properties: map[string]interface{}{},
				OccurredAt: now,
				Source:     "api",
			},
			wantErr: true,
			errMsg:  "email is required",
		},
		{
			name: "missing event_name",
			event: CustomEvent{
				ExternalID: "event_123",
				Email:      "user@example.com",
				Properties: map[string]interface{}{},
				OccurredAt: now,
				Source:     "api",
			},
			wantErr: true,
			errMsg:  "event_name is required",
		},
		{
			name: "event_name too long",
			event: CustomEvent{
				ExternalID: "event_123",
				Email:      "user@example.com",
				EventName:  "a_very_long_event_name_that_exceeds_the_maximum_allowed_length_of_one_hundred_characters_and_should_fail_validation_check",
				Properties: map[string]interface{}{},
				OccurredAt: now,
				Source:     "api",
			},
			wantErr: true,
			errMsg:  "event_name must be 100 characters or less",
		},
		{
			name: "invalid event_name with uppercase",
			event: CustomEvent{
				ExternalID: "event_123",
				Email:      "user@example.com",
				EventName:  "Orders/Fulfilled",
				Properties: map[string]interface{}{},
				OccurredAt: now,
				Source:     "api",
			},
			wantErr: true,
			errMsg:  "event_name must contain only lowercase letters, numbers, underscores, dots, and slashes",
		},
		{
			name: "invalid event_name with spaces",
			event: CustomEvent{
				ExternalID: "event_123",
				Email:      "user@example.com",
				EventName:  "order fulfilled",
				Properties: map[string]interface{}{},
				OccurredAt: now,
				Source:     "api",
			},
			wantErr: true,
			errMsg:  "event_name must contain only lowercase letters, numbers, underscores, dots, and slashes",
		},
		{
			name: "invalid event_name with special characters",
			event: CustomEvent{
				ExternalID: "event_123",
				Email:      "user@example.com",
				EventName:  "order@fulfilled!",
				Properties: map[string]interface{}{},
				OccurredAt: now,
				Source:     "api",
			},
			wantErr: true,
			errMsg:  "event_name must contain only lowercase letters, numbers, underscores, dots, and slashes",
		},
		{
			name: "missing occurred_at",
			event: CustomEvent{
				ExternalID: "event_123",
				Email:      "user@example.com",
				EventName:  "test.event",
				Properties: map[string]interface{}{},
				Source:     "api",
			},
			wantErr: true,
			errMsg:  "occurred_at is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.event.Validate()
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
				// Verify properties is initialized even if it was nil
				assert.NotNil(t, tt.event.Properties)
			}
		})
	}
}

func TestCreateCustomEventRequest_Validate(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name    string
		req     CreateCustomEventRequest
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid request with all fields",
			req: CreateCustomEventRequest{
				WorkspaceID: "workspace_123",
				Email:       "user@example.com",
				EventName:   "orders/fulfilled",
				ExternalID:  "order_12345",
				Properties: map[string]interface{}{
					"total": 99.99,
				},
				OccurredAt: &now,
			},
			wantErr: false,
		},
		{
			name: "valid request with minimal fields",
			req: CreateCustomEventRequest{
				WorkspaceID: "workspace_123",
				Email:       "user@example.com",
				EventName:   "test.event",
				ExternalID:  "test_123",
			},
			wantErr: false,
		},
		{
			name: "valid request with nil properties (should auto-initialize)",
			req: CreateCustomEventRequest{
				WorkspaceID: "workspace_123",
				Email:       "user@example.com",
				EventName:   "test.event",
				ExternalID:  "test_123",
				Properties:  nil,
			},
			wantErr: false,
		},
		{
			name: "missing workspace_id",
			req: CreateCustomEventRequest{
				Email:      "user@example.com",
				EventName:  "test.event",
				ExternalID: "test_123",
			},
			wantErr: true,
			errMsg:  "workspace_id is required",
		},
		{
			name: "missing email",
			req: CreateCustomEventRequest{
				WorkspaceID: "workspace_123",
				EventName:   "test.event",
				ExternalID:  "test_123",
			},
			wantErr: true,
			errMsg:  "email is required",
		},
		{
			name: "missing event_name",
			req: CreateCustomEventRequest{
				WorkspaceID: "workspace_123",
				Email:       "user@example.com",
				ExternalID:  "test_123",
			},
			wantErr: true,
			errMsg:  "event_name is required",
		},
		{
			name: "missing external_id",
			req: CreateCustomEventRequest{
				WorkspaceID: "workspace_123",
				Email:       "user@example.com",
				EventName:   "test.event",
			},
			wantErr: true,
			errMsg:  "external_id is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Validate()
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
				// Verify properties is initialized even if it was nil
				assert.NotNil(t, tt.req.Properties)
			}
		})
	}
}

func TestImportCustomEventsRequest_Validate(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name    string
		req     ImportCustomEventsRequest
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid batch with single event",
			req: ImportCustomEventsRequest{
				WorkspaceID: "workspace_123",
				Events: []*CustomEvent{
					{
						ExternalID: "event_1",
						Email:      "user@example.com",
						EventName:  "test.event",
						Properties: map[string]interface{}{},
						OccurredAt: now,
						Source:     "api",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "valid batch with 50 events (max)",
			req: ImportCustomEventsRequest{
				WorkspaceID: "workspace_123",
				Events: func() []*CustomEvent {
					events := make([]*CustomEvent, 50)
					for i := 0; i < 50; i++ {
						events[i] = &CustomEvent{
							ExternalID: "event_" + string(rune(i)),
							Email:      "user@example.com",
							EventName:  "test.event",
							Properties: map[string]interface{}{},
							OccurredAt: now,
							Source:     "api",
						}
					}
					return events
				}(),
			},
			wantErr: false,
		},
		{
			name: "valid batch with multiple events",
			req: ImportCustomEventsRequest{
				WorkspaceID: "workspace_123",
				Events: []*CustomEvent{
					{
						ExternalID: "event_1",
						Email:      "user1@example.com",
						EventName:  "orders/fulfilled",
						Properties: map[string]interface{}{"total": 99.99},
						OccurredAt: now,
						Source:     "api",
					},
					{
						ExternalID: "event_2",
						Email:      "user2@example.com",
						EventName:  "payment.succeeded",
						Properties: map[string]interface{}{"amount": 50.00},
						OccurredAt: now,
						Source:     "integration",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "missing workspace_id",
			req: ImportCustomEventsRequest{
				Events: []*CustomEvent{
					{
						ExternalID: "event_1",
						Email:      "user@example.com",
						EventName:  "test.event",
						Properties: map[string]interface{}{},
						OccurredAt: now,
						Source:     "api",
					},
				},
			},
			wantErr: true,
			errMsg:  "workspace_id is required",
		},
		{
			name: "empty events array",
			req: ImportCustomEventsRequest{
				WorkspaceID: "workspace_123",
				Events:      []*CustomEvent{},
			},
			wantErr: true,
			errMsg:  "events array cannot be empty",
		},
		{
			name: "too many events (51)",
			req: ImportCustomEventsRequest{
				WorkspaceID: "workspace_123",
				Events: func() []*CustomEvent {
					events := make([]*CustomEvent, 51)
					for i := 0; i < 51; i++ {
						events[i] = &CustomEvent{
							ExternalID: "event_" + string(rune(i)),
							Email:      "user@example.com",
							EventName:  "test.event",
							Properties: map[string]interface{}{},
							OccurredAt: now,
							Source:     "api",
						}
					}
					return events
				}(),
			},
			wantErr: true,
			errMsg:  "cannot import more than 50 events at once",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Validate()
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestListCustomEventsRequest_Validate(t *testing.T) {
	eventName := "test.event"

	tests := []struct {
		name    string
		req     ListCustomEventsRequest
		wantErr bool
		errMsg  string
		check   func(*testing.T, ListCustomEventsRequest)
	}{
		{
			name: "valid request with email",
			req: ListCustomEventsRequest{
				WorkspaceID: "workspace_123",
				Email:       "user@example.com",
				Limit:       50,
				Offset:      0,
			},
			wantErr: false,
		},
		{
			name: "valid request with event_name",
			req: ListCustomEventsRequest{
				WorkspaceID: "workspace_123",
				EventName:   &eventName,
				Limit:       50,
				Offset:      0,
			},
			wantErr: false,
		},
		{
			name: "valid request with both email and event_name",
			req: ListCustomEventsRequest{
				WorkspaceID: "workspace_123",
				Email:       "user@example.com",
				EventName:   &eventName,
				Limit:       25,
				Offset:      10,
			},
			wantErr: false,
		},
		{
			name: "defaults limit when zero",
			req: ListCustomEventsRequest{
				WorkspaceID: "workspace_123",
				Email:       "user@example.com",
				Limit:       0,
			},
			wantErr: false,
			check: func(t *testing.T, req ListCustomEventsRequest) {
				assert.Equal(t, 50, req.Limit, "should default to 50")
			},
		},
		{
			name: "caps limit at 100",
			req: ListCustomEventsRequest{
				WorkspaceID: "workspace_123",
				Email:       "user@example.com",
				Limit:       200,
			},
			wantErr: false,
			check: func(t *testing.T, req ListCustomEventsRequest) {
				assert.Equal(t, 100, req.Limit, "should cap at 100")
			},
		},
		{
			name: "resets negative offset to zero",
			req: ListCustomEventsRequest{
				WorkspaceID: "workspace_123",
				Email:       "user@example.com",
				Limit:       50,
				Offset:      -10,
			},
			wantErr: false,
			check: func(t *testing.T, req ListCustomEventsRequest) {
				assert.Equal(t, 0, req.Offset, "should reset negative offset to 0")
			},
		},
		{
			name: "missing workspace_id",
			req: ListCustomEventsRequest{
				Email: "user@example.com",
			},
			wantErr: true,
			errMsg:  "workspace_id is required",
		},
		{
			name: "missing both email and event_name",
			req: ListCustomEventsRequest{
				WorkspaceID: "workspace_123",
			},
			wantErr: true,
			errMsg:  "either email or event_name is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Validate()
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
				if tt.check != nil {
					tt.check(t, tt.req)
				}
			}
		})
	}
}

func TestIsValidEventName(t *testing.T) {
	tests := []struct {
		name      string
		eventName string
		want      bool
	}{
		// Valid formats
		{"webhook slash notation", "orders/fulfilled", true},
		{"dotted notation", "payment.succeeded", true},
		{"underscore notation", "trial_started", true},
		{"mixed notation", "shopify.orders/fulfilled", true},
		{"with numbers", "event123.test456", true},
		{"all lowercase letters", "event", true},
		{"all numbers", "123456", true},
		{"complex valid format", "app_v2.orders/created_2025", true},

		// Invalid formats
		{"uppercase letters", "Orders/Fulfilled", false},
		{"spaces", "order fulfilled", false},
		{"special character @", "order@fulfilled", false},
		{"special character !", "order!fulfilled", false},
		{"special character #", "order#fulfilled", false},
		{"special character $", "order$fulfilled", false},
		{"special character %", "order%fulfilled", false},
		{"special character &", "order&fulfilled", false},
		{"special character *", "order*fulfilled", false},
		{"special character +", "order+fulfilled", false},
		{"special character =", "order=fulfilled", false},
		{"parentheses", "order(fulfilled)", false},
		{"brackets", "order[fulfilled]", false},
		{"braces", "order{fulfilled}", false},
		{"pipe", "order|fulfilled", false},
		{"backslash", "order\\fulfilled", false},
		{"colon", "order:fulfilled", false},
		{"semicolon", "order;fulfilled", false},
		{"question mark", "order?fulfilled", false},
		{"comma", "order,fulfilled", false},
		{"empty string", "", false},
		{"only spaces", "   ", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isValidEventName(tt.eventName)
			assert.Equal(t, tt.want, got, "isValidEventName(%q) = %v, want %v", tt.eventName, got, tt.want)
		})
	}
}
