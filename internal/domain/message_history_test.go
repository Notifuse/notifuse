package domain

import (
	"database/sql"
	"encoding/json"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMessageEvent_Constants(t *testing.T) {
	t.Run("message event constants", func(t *testing.T) {
		assert.Equal(t, MessageEvent("sent"), MessageEventSent)
		assert.Equal(t, MessageEvent("delivered"), MessageEventDelivered)
		assert.Equal(t, MessageEvent("failed"), MessageEventFailed)
		assert.Equal(t, MessageEvent("opened"), MessageEventOpened)
		assert.Equal(t, MessageEvent("clicked"), MessageEventClicked)
		assert.Equal(t, MessageEvent("bounced"), MessageEventBounced)
		assert.Equal(t, MessageEvent("complained"), MessageEventComplained)
		assert.Equal(t, MessageEvent("unsubscribed"), MessageEventUnsubscribed)
	})
}

func TestMessageEventUpdate(t *testing.T) {
	t.Run("message event update structure", func(t *testing.T) {
		timestamp := time.Now()
		statusInfo := "Delivery successful"

		update := MessageEventUpdate{
			ID:         "msg-123",
			Event:      MessageEventDelivered,
			Timestamp:  timestamp,
			StatusInfo: &statusInfo,
		}

		assert.Equal(t, "msg-123", update.ID)
		assert.Equal(t, MessageEventDelivered, update.Event)
		assert.Equal(t, timestamp, update.Timestamp)
		assert.NotNil(t, update.StatusInfo)
		assert.Equal(t, "Delivery successful", *update.StatusInfo)
	})

	t.Run("message event update without status info", func(t *testing.T) {
		timestamp := time.Now()

		update := MessageEventUpdate{
			ID:        "msg-456",
			Event:     MessageEventOpened,
			Timestamp: timestamp,
		}

		assert.Equal(t, "msg-456", update.ID)
		assert.Equal(t, MessageEventOpened, update.Event)
		assert.Equal(t, timestamp, update.Timestamp)
		assert.Nil(t, update.StatusInfo)
	})
}

func TestMessageHistoryStatusSum(t *testing.T) {
	t.Run("message history status sum structure", func(t *testing.T) {
		stats := MessageHistoryStatusSum{
			TotalSent:         1000,
			TotalDelivered:    950,
			TotalBounced:      25,
			TotalComplained:   5,
			TotalFailed:       20,
			TotalOpened:       400,
			TotalClicked:      150,
			TotalUnsubscribed: 10,
		}

		assert.Equal(t, 1000, stats.TotalSent)
		assert.Equal(t, 950, stats.TotalDelivered)
		assert.Equal(t, 25, stats.TotalBounced)
		assert.Equal(t, 5, stats.TotalComplained)
		assert.Equal(t, 20, stats.TotalFailed)
		assert.Equal(t, 400, stats.TotalOpened)
		assert.Equal(t, 150, stats.TotalClicked)
		assert.Equal(t, 10, stats.TotalUnsubscribed)
	})

	t.Run("zero values", func(t *testing.T) {
		stats := MessageHistoryStatusSum{}

		assert.Equal(t, 0, stats.TotalSent)
		assert.Equal(t, 0, stats.TotalDelivered)
		assert.Equal(t, 0, stats.TotalBounced)
		assert.Equal(t, 0, stats.TotalComplained)
		assert.Equal(t, 0, stats.TotalFailed)
		assert.Equal(t, 0, stats.TotalOpened)
		assert.Equal(t, 0, stats.TotalClicked)
		assert.Equal(t, 0, stats.TotalUnsubscribed)
	})
}

func TestMessageData_Value(t *testing.T) {
	tests := []struct {
		name     string
		data     MessageData
		expected string
		wantErr  bool
	}{
		{
			name: "empty data",
			data: MessageData{
				Data:     map[string]interface{}{},
				Metadata: nil,
			},
			expected: `{"data":{}}`,
			wantErr:  false,
		},
		{
			name: "with data only",
			data: MessageData{
				Data: map[string]interface{}{
					"name":  "John Doe",
					"email": "john@example.com",
				},
				Metadata: nil,
			},
			expected: `{"data":{"email":"john@example.com","name":"John Doe"}}`,
			wantErr:  false,
		},
		{
			name: "with data and metadata",
			data: MessageData{
				Data: map[string]interface{}{
					"name": "John Doe",
				},
				Metadata: map[string]interface{}{
					"source": "signup",
					"tags":   []string{"welcome", "new-user"},
				},
			},
			expected: `{"data":{"name":"John Doe"},"metadata":{"source":"signup","tags":["welcome","new-user"]}}`,
			wantErr:  false,
		},
		{
			name: "with nil data map",
			data: MessageData{
				Data:     nil,
				Metadata: map[string]interface{}{"test": "value"},
			},
			wantErr: false,
		},
		{
			name: "with complex nested data",
			data: MessageData{
				Data: map[string]interface{}{
					"user": map[string]interface{}{
						"id":   123,
						"name": "John",
						"preferences": map[string]interface{}{
							"newsletter": true,
							"frequency":  "weekly",
						},
					},
					"items": []interface{}{
						map[string]interface{}{"id": 1, "name": "Item 1"},
						map[string]interface{}{"id": 2, "name": "Item 2"},
					},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.data.Value()
			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)

			// Convert result to string for comparison
			gotBytes, ok := got.([]byte)
			require.True(t, ok)

			// Verify it's valid JSON
			var result map[string]interface{}
			err = json.Unmarshal(gotBytes, &result)
			require.NoError(t, err)

			// For specific expected values, compare the structure
			if tt.expected != "" {
				var expected map[string]interface{}
				err = json.Unmarshal([]byte(tt.expected), &expected)
				require.NoError(t, err)
				assert.Equal(t, expected, result)
			}
		})
	}
}

func TestMessageData_Scan(t *testing.T) {
	tests := []struct {
		name    string
		input   interface{}
		want    MessageData
		wantErr bool
	}{
		{
			name:  "nil input",
			input: nil,
			want:  MessageData{},
		},
		{
			name:    "invalid type - integer",
			input:   123,
			wantErr: true,
		},
		{
			name:    "invalid type - string",
			input:   "not json bytes",
			wantErr: true,
		},
		{
			name:  "valid json - empty",
			input: []byte(`{"data":{}}`),
			want: MessageData{
				Data: map[string]interface{}{},
			},
		},
		{
			name:  "valid json - with data",
			input: []byte(`{"data":{"name":"John Doe","age":30},"metadata":{"source":"api"}}`),
			want: MessageData{
				Data: map[string]interface{}{
					"name": "John Doe",
					"age":  float64(30),
				},
				Metadata: map[string]interface{}{
					"source": "api",
				},
			},
		},
		{
			name:  "valid json - only metadata",
			input: []byte(`{"metadata":{"campaign":"summer2023"}}`),
			want: MessageData{
				Metadata: map[string]interface{}{
					"campaign": "summer2023",
				},
			},
		},
		{
			name:    "invalid json",
			input:   []byte(`{"data":{"name":"John"`),
			wantErr: true,
		},
		{
			name:  "empty json object",
			input: []byte(`{}`),
			want:  MessageData{},
		},
		{
			name:  "complex nested structure",
			input: []byte(`{"data":{"user":{"id":123,"preferences":{"newsletter":true}}},"metadata":{"version":"v2"}}`),
			want: MessageData{
				Data: map[string]interface{}{
					"user": map[string]interface{}{
						"id": float64(123),
						"preferences": map[string]interface{}{
							"newsletter": true,
						},
					},
				},
				Metadata: map[string]interface{}{
					"version": "v2",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var md MessageData
			err := md.Scan(tt.input)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)

			if tt.input == nil {
				assert.Empty(t, md.Data)
				assert.Empty(t, md.Metadata)
				return
			}

			assert.Equal(t, tt.want.Data, md.Data)
			assert.Equal(t, tt.want.Metadata, md.Metadata)
		})
	}
}

func TestMessageData_Scan_SQLErrors(t *testing.T) {
	t.Run("sql.ErrNoRows for non-byte input", func(t *testing.T) {
		var md MessageData
		err := md.Scan("string input")

		assert.Error(t, err)
		assert.Equal(t, sql.ErrNoRows, err)
	})
}

func TestMessageHistory(t *testing.T) {
	now := time.Now()

	t.Run("complete message history", func(t *testing.T) {
		broadcastID := "broadcast123"
		statusInfo := "Delivered successfully"
		deliveredAt := now.Add(time.Minute)
		openedAt := now.Add(2 * time.Minute)
		clickedAt := now.Add(3 * time.Minute)

		message := MessageHistory{
			ID:              "msg123",
			ContactEmail:    "user@example.com",
			BroadcastID:     &broadcastID,
			TemplateID:      "template789",
			TemplateVersion: 2,
			Channel:         "email",
			StatusInfo:      &statusInfo,
			MessageData: MessageData{
				Data: map[string]interface{}{
					"subject":   "Welcome to our service!",
					"firstName": "John",
					"lastName":  "Doe",
				},
				Metadata: map[string]interface{}{
					"campaign":   "onboarding",
					"ab_test":    "variant_a",
					"utm_source": "email",
					"utm_medium": "newsletter",
				},
			},
			SentAt:      now,
			DeliveredAt: &deliveredAt,
			OpenedAt:    &openedAt,
			ClickedAt:   &clickedAt,
			CreatedAt:   now,
			UpdatedAt:   now.Add(time.Hour),
		}

		// Test basic field values
		assert.Equal(t, "msg123", message.ID)
		assert.Equal(t, "user@example.com", message.ContactEmail)
		assert.Equal(t, "broadcast123", *message.BroadcastID)
		assert.Equal(t, "template789", message.TemplateID)
		assert.Equal(t, int64(2), message.TemplateVersion)
		assert.Equal(t, "email", message.Channel)
		assert.Equal(t, "Delivered successfully", *message.StatusInfo)
		assert.Equal(t, now, message.SentAt)
		assert.Equal(t, deliveredAt, *message.DeliveredAt)
		assert.Equal(t, openedAt, *message.OpenedAt)
		assert.Equal(t, clickedAt, *message.ClickedAt)
		assert.Equal(t, now, message.CreatedAt)
		assert.Equal(t, now.Add(time.Hour), message.UpdatedAt)

		// Test message data
		assert.Equal(t, "Welcome to our service!", message.MessageData.Data["subject"])
		assert.Equal(t, "John", message.MessageData.Data["firstName"])
		assert.Equal(t, "Doe", message.MessageData.Data["lastName"])
		assert.Equal(t, "onboarding", message.MessageData.Metadata["campaign"])
		assert.Equal(t, "variant_a", message.MessageData.Metadata["ab_test"])

		// Test unset optional timestamps are nil
		assert.Nil(t, message.FailedAt)
		assert.Nil(t, message.BouncedAt)
		assert.Nil(t, message.ComplainedAt)
		assert.Nil(t, message.UnsubscribedAt)
	})

	t.Run("minimal message history", func(t *testing.T) {
		message := MessageHistory{
			ID:              "msg456",
			ContactEmail:    "minimal@example.com",
			TemplateID:      "template123",
			TemplateVersion: 1,
			Channel:         "sms",
			MessageData: MessageData{
				Data: map[string]interface{}{
					"message": "Hello!",
				},
			},
			SentAt:    now,
			CreatedAt: now,
			UpdatedAt: now,
		}

		assert.Equal(t, "msg456", message.ID)
		assert.Equal(t, "minimal@example.com", message.ContactEmail)
		assert.Nil(t, message.BroadcastID)
		assert.Equal(t, "template123", message.TemplateID)
		assert.Equal(t, int64(1), message.TemplateVersion)
		assert.Equal(t, "sms", message.Channel)
		assert.Nil(t, message.StatusInfo)
		assert.Equal(t, "Hello!", message.MessageData.Data["message"])
		assert.Nil(t, message.MessageData.Metadata)

		// All optional timestamps should be nil
		assert.Nil(t, message.DeliveredAt)
		assert.Nil(t, message.FailedAt)
		assert.Nil(t, message.OpenedAt)
		assert.Nil(t, message.ClickedAt)
		assert.Nil(t, message.BouncedAt)
		assert.Nil(t, message.ComplainedAt)
		assert.Nil(t, message.UnsubscribedAt)
	})

	t.Run("message with all event timestamps", func(t *testing.T) {
		sentAt := now
		deliveredAt := now.Add(1 * time.Minute)
		openedAt := now.Add(2 * time.Minute)
		clickedAt := now.Add(3 * time.Minute)
		bouncedAt := now.Add(4 * time.Minute)
		complainedAt := now.Add(5 * time.Minute)
		unsubscribedAt := now.Add(6 * time.Minute)
		failedAt := now.Add(7 * time.Minute)

		message := MessageHistory{
			ID:              "msg789",
			ContactEmail:    "events@example.com",
			TemplateID:      "template456",
			TemplateVersion: 3,
			Channel:         "email",
			MessageData:     MessageData{Data: map[string]interface{}{"test": "data"}},
			SentAt:          sentAt,
			DeliveredAt:     &deliveredAt,
			OpenedAt:        &openedAt,
			ClickedAt:       &clickedAt,
			BouncedAt:       &bouncedAt,
			ComplainedAt:    &complainedAt,
			UnsubscribedAt:  &unsubscribedAt,
			FailedAt:        &failedAt,
			CreatedAt:       now,
			UpdatedAt:       now,
		}

		assert.Equal(t, sentAt, message.SentAt)
		assert.Equal(t, deliveredAt, *message.DeliveredAt)
		assert.Equal(t, openedAt, *message.OpenedAt)
		assert.Equal(t, clickedAt, *message.ClickedAt)
		assert.Equal(t, bouncedAt, *message.BouncedAt)
		assert.Equal(t, complainedAt, *message.ComplainedAt)
		assert.Equal(t, unsubscribedAt, *message.UnsubscribedAt)
		assert.Equal(t, failedAt, *message.FailedAt)
	})
}

func TestParseTimeParam(t *testing.T) {
	tests := []struct {
		name      string
		query     url.Values
		paramName string
		wantTime  *time.Time
		wantErr   bool
	}{
		{
			name:      "valid RFC3339 time",
			query:     url.Values{"test_time": []string{"2023-01-15T10:30:00Z"}},
			paramName: "test_time",
			wantTime:  timePtr(time.Date(2023, 1, 15, 10, 30, 0, 0, time.UTC)),
			wantErr:   false,
		},
		{
			name:      "valid RFC3339 time with timezone",
			query:     url.Values{"test_time": []string{"2023-01-15T10:30:00+02:00"}},
			paramName: "test_time",
			wantTime:  timePtr(time.Date(2023, 1, 15, 8, 30, 0, 0, time.UTC)),
			wantErr:   false,
		},
		{
			name:      "empty parameter",
			query:     url.Values{},
			paramName: "test_time",
			wantTime:  nil,
			wantErr:   false,
		},
		{
			name:      "parameter not present",
			query:     url.Values{"other_param": []string{"value"}},
			paramName: "test_time",
			wantTime:  nil,
			wantErr:   false,
		},
		{
			name:      "invalid time format",
			query:     url.Values{"test_time": []string{"2023/01/15 10:30:00"}},
			paramName: "test_time",
			wantTime:  nil,
			wantErr:   true,
		},
		{
			name:      "invalid time format - not RFC3339",
			query:     url.Values{"test_time": []string{"January 15, 2023"}},
			paramName: "test_time",
			wantTime:  nil,
			wantErr:   true,
		},
		{
			name:      "empty string value",
			query:     url.Values{"test_time": []string{""}},
			paramName: "test_time",
			wantTime:  nil,
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var result *time.Time
			err := parseTimeParam(tt.query, tt.paramName, &result)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "invalid "+tt.paramName+" time format")
				return
			}

			require.NoError(t, err)

			if tt.wantTime == nil {
				assert.Nil(t, result)
			} else {
				require.NotNil(t, result)
				assert.True(t, tt.wantTime.Equal(*result))
			}
		})
	}
}

func TestMessageListParams_FromQuery(t *testing.T) {
	tests := []struct {
		name      string
		queryData map[string][]string
		want      MessageListParams
		wantErr   bool
	}{
		{
			name:      "empty query",
			queryData: map[string][]string{},
			want: MessageListParams{
				Limit: 20, // Default limit
			},
			wantErr: false,
		},
		{
			name: "basic string filters",
			queryData: map[string][]string{
				"cursor":        {"next_page"},
				"channel":       {"email"},
				"contact_email": {"contact@example.com"},
				"broadcast_id":  {"a1b2c3d4-e5f6-7a8b-9c0d-1e2f3a4b5c6d"},
				"template_id":   {"7a8b9c0d-1e2f-3a4b-5c6d-7e8f9a0b1c2d"},
			},
			want: MessageListParams{
				Cursor:       "next_page",
				Channel:      "email",
				ContactEmail: "contact@example.com",
				BroadcastID:  "a1b2c3d4-e5f6-7a8b-9c0d-1e2f3a4b5c6d",
				TemplateID:   "7a8b9c0d-1e2f-3a4b-5c6d-7e8f9a0b1c2d",
				Limit:        20, // Default limit
			},
			wantErr: false,
		},
		{
			name: "with id, external_id, and list_id filters",
			queryData: map[string][]string{
				"id":          {"msg-123"},
				"external_id": {"ext-456"},
				"list_id":     {"list-789"},
			},
			want: MessageListParams{
				ID:         "msg-123",
				ExternalID: "ext-456",
				ListID:     "list-789",
				Limit:      20, // Default limit
			},
			wantErr: false,
		},
		{
			name: "with custom limit",
			queryData: map[string][]string{
				"limit": {"50"},
			},
			want: MessageListParams{
				Limit: 50,
			},
			wantErr: false,
		},
		{
			name: "with invalid limit",
			queryData: map[string][]string{
				"limit": {"not_a_number"},
			},
			wantErr: true,
		},
		{
			name: "with all boolean filters true",
			queryData: map[string][]string{
				"is_sent":         {"true"},
				"is_delivered":    {"true"},
				"is_failed":       {"true"},
				"is_opened":       {"true"},
				"is_clicked":      {"true"},
				"is_bounced":      {"true"},
				"is_complained":   {"true"},
				"is_unsubscribed": {"true"},
			},
			want: MessageListParams{
				IsSent:         boolPtr(true),
				IsDelivered:    boolPtr(true),
				IsFailed:       boolPtr(true),
				IsOpened:       boolPtr(true),
				IsClicked:      boolPtr(true),
				IsBounced:      boolPtr(true),
				IsComplained:   boolPtr(true),
				IsUnsubscribed: boolPtr(true),
				Limit:          20, // Default limit
			},
			wantErr: false,
		},
		{
			name: "with all boolean filters false",
			queryData: map[string][]string{
				"is_sent":         {"false"},
				"is_delivered":    {"false"},
				"is_failed":       {"false"},
				"is_opened":       {"false"},
				"is_clicked":      {"false"},
				"is_bounced":      {"false"},
				"is_complained":   {"false"},
				"is_unsubscribed": {"false"},
			},
			want: MessageListParams{
				IsSent:         boolPtr(false),
				IsDelivered:    boolPtr(false),
				IsFailed:       boolPtr(false),
				IsOpened:       boolPtr(false),
				IsClicked:      boolPtr(false),
				IsBounced:      boolPtr(false),
				IsComplained:   boolPtr(false),
				IsUnsubscribed: boolPtr(false),
				Limit:          20, // Default limit
			},
			wantErr: false,
		},
		{
			name: "with invalid boolean filters",
			queryData: map[string][]string{
				"is_sent": {"not_a_boolean"},
			},
			wantErr: true,
		},
		{
			name: "with time parameters",
			queryData: map[string][]string{
				"sent_after":     {"2023-01-01T00:00:00Z"},
				"sent_before":    {"2023-12-31T23:59:59Z"},
				"updated_after":  {"2023-02-01T00:00:00Z"},
				"updated_before": {"2023-11-30T23:59:59Z"},
			},
			want: MessageListParams{
				SentAfter:     timePtr(time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)),
				SentBefore:    timePtr(time.Date(2023, 12, 31, 23, 59, 59, 0, time.UTC)),
				UpdatedAfter:  timePtr(time.Date(2023, 2, 1, 0, 0, 0, 0, time.UTC)),
				UpdatedBefore: timePtr(time.Date(2023, 11, 30, 23, 59, 59, 0, time.UTC)),
				Limit:         20, // Default limit
			},
			wantErr: false,
		},
		{
			name: "with invalid time format",
			queryData: map[string][]string{
				"sent_after": {"2023/01/01"}, // Invalid RFC3339 format
			},
			wantErr: true,
		},
		{
			name: "with invalid channel",
			queryData: map[string][]string{
				"channel": {"invalid_channel"}, // Not one of the allowed values
			},
			wantErr: true,
		},
		{
			name: "with invalid contact_email",
			queryData: map[string][]string{
				"contact_email": {"not-a-email"}, // Not a valid email format
			},
			wantErr: true,
		},
		{
			name: "with invalid broadcast_id",
			queryData: map[string][]string{
				"broadcast_id": {"not-a-uuid"}, // Not a UUID format
			},
			want: MessageListParams{
				BroadcastID: "not-a-uuid",
				Limit:       20, // Default limit
			},
			wantErr: false,
		},
		{
			name: "with invalid template_id",
			queryData: map[string][]string{
				"template_id": {"not-a-uuid"}, // Not a UUID format
			},
			want: MessageListParams{
				TemplateID: "not-a-uuid",
				Limit:      20, // Default limit
			},
			wantErr: false,
		},
		{
			name: "with invalid time range (sent)",
			queryData: map[string][]string{
				"sent_after":  {"2023-12-31T23:59:59Z"},
				"sent_before": {"2023-01-01T00:00:00Z"}, // Before the after date
			},
			wantErr: true,
		},
		{
			name: "with invalid time range (updated)",
			queryData: map[string][]string{
				"updated_after":  {"2023-12-31T23:59:59Z"},
				"updated_before": {"2023-01-01T00:00:00Z"}, // Before the after date
			},
			wantErr: true,
		},
		{
			name: "with too large limit",
			queryData: map[string][]string{
				"limit": {"200"}, // Above the cap of 100
			},
			want: MessageListParams{
				Limit: 100, // Should be capped to 100
			},
			wantErr: false,
		},
		{
			name: "with negative limit",
			queryData: map[string][]string{
				"limit": {"-10"}, // Negative
			},
			wantErr: true,
		},
		{
			name: "with all parameters",
			queryData: map[string][]string{
				"cursor":          {"next_page"},
				"id":              {"msg-999"},
				"external_id":     {"ext-999"},
				"list_id":         {"list-999"},
				"channel":         {"email"},
				"contact_email":   {"contact@example.com"},
				"broadcast_id":    {"a1b2c3d4-e5f6-7a8b-9c0d-1e2f3a4b5c6d"},
				"template_id":     {"7a8b9c0d-1e2f-3a4b-5c6d-7e8f9a0b1c2d"},
				"is_sent":         {"true"},
				"is_delivered":    {"false"},
				"is_failed":       {"true"},
				"is_opened":       {"false"},
				"is_clicked":      {"true"},
				"is_bounced":      {"false"},
				"is_complained":   {"true"},
				"is_unsubscribed": {"false"},
				"limit":           {"50"},
				"sent_after":      {"2023-01-01T00:00:00Z"},
				"sent_before":     {"2023-12-31T23:59:59Z"},
				"updated_after":   {"2023-02-01T00:00:00Z"},
				"updated_before":  {"2023-11-30T23:59:59Z"},
			},
			want: MessageListParams{
				Cursor:         "next_page",
				ID:             "msg-999",
				ExternalID:     "ext-999",
				ListID:         "list-999",
				Channel:        "email",
				ContactEmail:   "contact@example.com",
				BroadcastID:    "a1b2c3d4-e5f6-7a8b-9c0d-1e2f3a4b5c6d",
				TemplateID:     "7a8b9c0d-1e2f-3a4b-5c6d-7e8f9a0b1c2d",
				IsSent:         boolPtr(true),
				IsDelivered:    boolPtr(false),
				IsFailed:       boolPtr(true),
				IsOpened:       boolPtr(false),
				IsClicked:      boolPtr(true),
				IsBounced:      boolPtr(false),
				IsComplained:   boolPtr(true),
				IsUnsubscribed: boolPtr(false),
				Limit:          50,
				SentAfter:      timePtr(time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)),
				SentBefore:     timePtr(time.Date(2023, 12, 31, 23, 59, 59, 0, time.UTC)),
				UpdatedAfter:   timePtr(time.Date(2023, 2, 1, 0, 0, 0, 0, time.UTC)),
				UpdatedBefore:  timePtr(time.Date(2023, 11, 30, 23, 59, 59, 0, time.UTC)),
			},
			wantErr: false,
		},
		{
			name: "with sms channel",
			queryData: map[string][]string{
				"channel": {"sms"},
			},
			want: MessageListParams{
				Channel: "sms",
				Limit:   20,
			},
			wantErr: false,
		},
		{
			name: "with push channel",
			queryData: map[string][]string{
				"channel": {"push"},
			},
			want: MessageListParams{
				Channel: "push",
				Limit:   20,
			},
			wantErr: false,
		},
		{
			name: "with zero limit",
			queryData: map[string][]string{
				"limit": {"0"},
			},
			want: MessageListParams{
				Limit: 20, // Should default to 20
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			queryValues := url.Values(tt.queryData)
			var params MessageListParams
			err := params.FromQuery(queryValues)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want.Cursor, params.Cursor)
			assert.Equal(t, tt.want.Channel, params.Channel)
			assert.Equal(t, tt.want.ContactEmail, params.ContactEmail)
			assert.Equal(t, tt.want.BroadcastID, params.BroadcastID)
			assert.Equal(t, tt.want.TemplateID, params.TemplateID)
			assert.Equal(t, tt.want.Limit, params.Limit)

			// Test all boolean filters
			assertBoolPtr(t, "IsSent", tt.want.IsSent, params.IsSent)
			assertBoolPtr(t, "IsDelivered", tt.want.IsDelivered, params.IsDelivered)
			assertBoolPtr(t, "IsFailed", tt.want.IsFailed, params.IsFailed)
			assertBoolPtr(t, "IsOpened", tt.want.IsOpened, params.IsOpened)
			assertBoolPtr(t, "IsClicked", tt.want.IsClicked, params.IsClicked)
			assertBoolPtr(t, "IsBounced", tt.want.IsBounced, params.IsBounced)
			assertBoolPtr(t, "IsComplained", tt.want.IsComplained, params.IsComplained)
			assertBoolPtr(t, "IsUnsubscribed", tt.want.IsUnsubscribed, params.IsUnsubscribed)

			// Test time filters
			assertTimePtr(t, "SentAfter", tt.want.SentAfter, params.SentAfter)
			assertTimePtr(t, "SentBefore", tt.want.SentBefore, params.SentBefore)
			assertTimePtr(t, "UpdatedAfter", tt.want.UpdatedAfter, params.UpdatedAfter)
			assertTimePtr(t, "UpdatedBefore", tt.want.UpdatedBefore, params.UpdatedBefore)
		})
	}
}

func TestMessageListParams_Validate(t *testing.T) {
	tests := []struct {
		name    string
		params  MessageListParams
		want    MessageListParams
		wantErr bool
	}{
		{
			name:   "default values",
			params: MessageListParams{},
			want: MessageListParams{
				Limit: 20, // Default limit
			},
			wantErr: false,
		},
		{
			name: "negative limit",
			params: MessageListParams{
				Limit: -10,
			},
			wantErr: true,
		},
		{
			name: "zero limit becomes default",
			params: MessageListParams{
				Limit: 0,
			},
			want: MessageListParams{
				Limit: 20, // Default limit
			},
			wantErr: false,
		},
		{
			name: "limit too high gets capped",
			params: MessageListParams{
				Limit: 500,
			},
			want: MessageListParams{
				Limit: 100, // Capped to max
			},
			wantErr: false,
		},
		{
			name: "valid email channel",
			params: MessageListParams{
				Channel: "email",
			},
			want: MessageListParams{
				Channel: "email",
				Limit:   20, // Default limit
			},
			wantErr: false,
		},
		{
			name: "valid sms channel",
			params: MessageListParams{
				Channel: "sms",
			},
			want: MessageListParams{
				Channel: "sms",
				Limit:   20,
			},
			wantErr: false,
		},
		{
			name: "valid push channel",
			params: MessageListParams{
				Channel: "push",
			},
			want: MessageListParams{
				Channel: "push",
				Limit:   20,
			},
			wantErr: false,
		},
		{
			name: "invalid channel",
			params: MessageListParams{
				Channel: "invalid_channel",
			},
			wantErr: true,
		},
		{
			name: "valid email address",
			params: MessageListParams{
				ContactEmail: "user@example.com",
			},
			want: MessageListParams{
				ContactEmail: "user@example.com",
				Limit:        20,
			},
			wantErr: false,
		},
		{
			name: "valid complex email address",
			params: MessageListParams{
				ContactEmail: "user.name+tag@example-domain.co.uk",
			},
			want: MessageListParams{
				ContactEmail: "user.name+tag@example-domain.co.uk",
				Limit:        20,
			},
			wantErr: false,
		},
		{
			name: "invalid email format",
			params: MessageListParams{
				ContactEmail: "not-an-email",
			},
			wantErr: true,
		},
		{
			name: "valid UUIDs",
			params: MessageListParams{
				BroadcastID: "a1b2c3d4-e5f6-7a8b-9c0d-1e2f3a4b5c6d",
				TemplateID:  "7a8b9c0d-1e2f-3a4b-5c6d-7e8f9a0b1c2d",
			},
			want: MessageListParams{
				BroadcastID: "a1b2c3d4-e5f6-7a8b-9c0d-1e2f3a4b5c6d",
				TemplateID:  "7a8b9c0d-1e2f-3a4b-5c6d-7e8f9a0b1c2d",
				Limit:       20, // Default limit
			},
			wantErr: false,
		},
		{
			name: "invalid broadcast ID",
			params: MessageListParams{
				BroadcastID: "not-a-uuid",
			},
			want: MessageListParams{
				BroadcastID: "not-a-uuid",
				Limit:       20, // Default limit
			},
			wantErr: false,
		},
		{
			name: "invalid template ID",
			params: MessageListParams{
				TemplateID: "not-a-uuid",
			},
			want: MessageListParams{
				TemplateID: "not-a-uuid",
				Limit:      20, // Default limit
			},
			wantErr: false,
		},
		{
			name: "valid time ranges",
			params: MessageListParams{
				SentAfter:     timePtr(time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)),
				SentBefore:    timePtr(time.Date(2023, 12, 31, 23, 59, 59, 0, time.UTC)),
				UpdatedAfter:  timePtr(time.Date(2023, 2, 1, 0, 0, 0, 0, time.UTC)),
				UpdatedBefore: timePtr(time.Date(2023, 11, 30, 23, 59, 59, 0, time.UTC)),
			},
			want: MessageListParams{
				SentAfter:     timePtr(time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)),
				SentBefore:    timePtr(time.Date(2023, 12, 31, 23, 59, 59, 0, time.UTC)),
				UpdatedAfter:  timePtr(time.Date(2023, 2, 1, 0, 0, 0, 0, time.UTC)),
				UpdatedBefore: timePtr(time.Date(2023, 11, 30, 23, 59, 59, 0, time.UTC)),
				Limit:         20, // Default limit
			},
			wantErr: false,
		},
		{
			name: "invalid sent time range",
			params: MessageListParams{
				SentAfter:  timePtr(time.Date(2023, 12, 31, 23, 59, 59, 0, time.UTC)),
				SentBefore: timePtr(time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)), // Before the after date
			},
			wantErr: true,
		},
		{
			name: "invalid updated time range",
			params: MessageListParams{
				UpdatedAfter:  timePtr(time.Date(2023, 12, 31, 23, 59, 59, 0, time.UTC)),
				UpdatedBefore: timePtr(time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)), // Before the after date
			},
			wantErr: true,
		},
		{
			name: "edge case - same time for range",
			params: MessageListParams{
				SentAfter:  timePtr(time.Date(2023, 6, 15, 12, 0, 0, 0, time.UTC)),
				SentBefore: timePtr(time.Date(2023, 6, 15, 12, 0, 0, 0, time.UTC)),
			},
			want: MessageListParams{
				SentAfter:  timePtr(time.Date(2023, 6, 15, 12, 0, 0, 0, time.UTC)),
				SentBefore: timePtr(time.Date(2023, 6, 15, 12, 0, 0, 0, time.UTC)),
				Limit:      20,
			},
			wantErr: false,
		},
		{
			name: "limit exactly at maximum",
			params: MessageListParams{
				Limit: 100,
			},
			want: MessageListParams{
				Limit: 100,
			},
			wantErr: false,
		},
		{
			name: "limit just over maximum",
			params: MessageListParams{
				Limit: 101,
			},
			want: MessageListParams{
				Limit: 100, // Should be capped
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Make a copy of the params to validate
			params := tt.params

			err := params.Validate()

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)

			// Check if params were modified as expected
			if tt.want.Limit != 0 {
				assert.Equal(t, tt.want.Limit, params.Limit)
			}
			assert.Equal(t, tt.want.Channel, params.Channel)
			assert.Equal(t, tt.want.ContactEmail, params.ContactEmail)
			assert.Equal(t, tt.want.BroadcastID, params.BroadcastID)
			assert.Equal(t, tt.want.TemplateID, params.TemplateID)

			assertTimePtr(t, "SentAfter", tt.want.SentAfter, params.SentAfter)
			assertTimePtr(t, "SentBefore", tt.want.SentBefore, params.SentBefore)
			assertTimePtr(t, "UpdatedAfter", tt.want.UpdatedAfter, params.UpdatedAfter)
			assertTimePtr(t, "UpdatedBefore", tt.want.UpdatedBefore, params.UpdatedBefore)
		})
	}
}

func TestMessageListResult(t *testing.T) {
	t.Run("message list result structure", func(t *testing.T) {
		now := time.Now()
		messages := []*MessageHistory{
			{
				ID:           "msg1",
				ContactEmail: "user1@example.com",
				TemplateID:   "template1",
				Channel:      "email",
				MessageData:  MessageData{Data: map[string]interface{}{"subject": "Test 1"}},
				SentAt:       now,
				CreatedAt:    now,
				UpdatedAt:    now,
			},
			{
				ID:           "msg2",
				ContactEmail: "user2@example.com",
				TemplateID:   "template2",
				Channel:      "sms",
				MessageData:  MessageData{Data: map[string]interface{}{"message": "Test 2"}},
				SentAt:       now.Add(time.Minute),
				CreatedAt:    now.Add(time.Minute),
				UpdatedAt:    now.Add(time.Minute),
			},
		}

		result := MessageListResult{
			Messages:   messages,
			NextCursor: "next_page_token",
			HasMore:    true,
		}

		assert.Len(t, result.Messages, 2)
		assert.Equal(t, "msg1", result.Messages[0].ID)
		assert.Equal(t, "msg2", result.Messages[1].ID)
		assert.Equal(t, "next_page_token", result.NextCursor)
		assert.True(t, result.HasMore)
	})

	t.Run("empty message list result", func(t *testing.T) {
		result := MessageListResult{
			Messages:   []*MessageHistory{},
			NextCursor: "",
			HasMore:    false,
		}

		assert.Empty(t, result.Messages)
		assert.Equal(t, "", result.NextCursor)
		assert.False(t, result.HasMore)
	})
}

// Helper functions to create pointers
func boolPtr(b bool) *bool {
	return &b
}

func timePtr(t time.Time) *time.Time {
	return &t
}

// Helper function to assert boolean pointer values
func assertBoolPtr(t *testing.T, name string, expected, actual *bool) {
	if expected == nil {
		assert.Nil(t, actual, "%s should be nil", name)
	} else {
		require.NotNil(t, actual, "%s should not be nil", name)
		assert.Equal(t, *expected, *actual, "%s value mismatch", name)
	}
}

// Helper function to assert time pointer values
func assertTimePtr(t *testing.T, name string, expected, actual *time.Time) {
	if expected == nil {
		assert.Nil(t, actual, "%s should be nil", name)
	} else {
		require.NotNil(t, actual, "%s should not be nil", name)
		assert.True(t, expected.Equal(*actual), "%s time mismatch: expected %v, got %v", name, *expected, *actual)
	}
}

func TestChannelOptions_Value(t *testing.T) {
	tests := []struct {
		name     string
		options  ChannelOptions
		expected string
		wantErr  bool
	}{
		{
			name:     "empty options",
			options:  ChannelOptions{},
			expected: `{}`,
			wantErr:  false,
		},
		{
			name: "with from_name only",
			options: ChannelOptions{
				FromName: stringPtr("Custom Sender"),
			},
			expected: `{"from_name":"Custom Sender"}`,
			wantErr:  false,
		},
		{
			name: "with cc only",
			options: ChannelOptions{
				CC: []string{"cc1@example.com", "cc2@example.com"},
			},
			expected: `{"cc":["cc1@example.com","cc2@example.com"]}`,
			wantErr:  false,
		},
		{
			name: "with bcc only",
			options: ChannelOptions{
				BCC: []string{"bcc@example.com"},
			},
			expected: `{"bcc":["bcc@example.com"]}`,
			wantErr:  false,
		},
		{
			name: "with reply_to only",
			options: ChannelOptions{
				ReplyTo: "reply@example.com",
			},
			expected: `{"reply_to":"reply@example.com"}`,
			wantErr:  false,
		},
		{
			name: "with all fields",
			options: ChannelOptions{
				FromName: stringPtr("Test Sender"),
				CC:       []string{"cc@example.com"},
				BCC:      []string{"bcc@example.com"},
				ReplyTo:  "reply@example.com",
			},
			expected: `{"from_name":"Test Sender","cc":["cc@example.com"],"bcc":["bcc@example.com"],"reply_to":"reply@example.com"}`,
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.options.Value()
			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)

			// Convert result to string for comparison
			gotBytes, ok := got.([]byte)
			require.True(t, ok)

			// Verify it's valid JSON
			var result map[string]interface{}
			err = json.Unmarshal(gotBytes, &result)
			require.NoError(t, err)

			// Compare with expected
			var expected map[string]interface{}
			err = json.Unmarshal([]byte(tt.expected), &expected)
			require.NoError(t, err)
			assert.Equal(t, expected, result)
		})
	}
}

func TestChannelOptions_Scan(t *testing.T) {
	tests := []struct {
		name    string
		input   interface{}
		want    *ChannelOptions
		wantErr bool
	}{
		{
			name:  "nil input",
			input: nil,
			want:  &ChannelOptions{},
		},
		{
			name:    "invalid type - integer",
			input:   123,
			wantErr: true,
		},
		{
			name:    "invalid type - string",
			input:   "not json bytes",
			wantErr: true,
		},
		{
			name:  "valid json - empty",
			input: []byte(`{}`),
			want:  &ChannelOptions{},
		},
		{
			name:  "valid json - with from_name",
			input: []byte(`{"from_name":"Test Sender"}`),
			want: &ChannelOptions{
				FromName: stringPtr("Test Sender"),
			},
		},
		{
			name:  "valid json - with cc",
			input: []byte(`{"cc":["cc1@example.com","cc2@example.com"]}`),
			want: &ChannelOptions{
				CC: []string{"cc1@example.com", "cc2@example.com"},
			},
		},
		{
			name:  "valid json - with bcc",
			input: []byte(`{"bcc":["bcc@example.com"]}`),
			want: &ChannelOptions{
				BCC: []string{"bcc@example.com"},
			},
		},
		{
			name:  "valid json - with reply_to",
			input: []byte(`{"reply_to":"reply@example.com"}`),
			want: &ChannelOptions{
				ReplyTo: "reply@example.com",
			},
		},
		{
			name:  "valid json - with all fields",
			input: []byte(`{"from_name":"Test Sender","cc":["cc@example.com"],"bcc":["bcc@example.com"],"reply_to":"reply@example.com"}`),
			want: &ChannelOptions{
				FromName: stringPtr("Test Sender"),
				CC:       []string{"cc@example.com"},
				BCC:      []string{"bcc@example.com"},
				ReplyTo:  "reply@example.com",
			},
		},
		{
			name:    "invalid json",
			input:   []byte(`{"from_name":"Test`),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var co ChannelOptions
			err := co.Scan(tt.input)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)

			if tt.input == nil {
				assert.Nil(t, co.FromName)
				assert.Empty(t, co.CC)
				assert.Empty(t, co.BCC)
				assert.Empty(t, co.ReplyTo)
				return
			}

			if tt.want.FromName == nil {
				assert.Nil(t, co.FromName)
			} else {
				require.NotNil(t, co.FromName)
				assert.Equal(t, *tt.want.FromName, *co.FromName)
			}

			assert.Equal(t, tt.want.CC, co.CC)
			assert.Equal(t, tt.want.BCC, co.BCC)
			assert.Equal(t, tt.want.ReplyTo, co.ReplyTo)
		})
	}
}

func TestChannelOptions_Scan_SQLErrors(t *testing.T) {
	t.Run("sql.ErrNoRows for non-byte input", func(t *testing.T) {
		var co ChannelOptions
		err := co.Scan("string input")

		assert.Error(t, err)
		assert.Equal(t, sql.ErrNoRows, err)
	})
}
