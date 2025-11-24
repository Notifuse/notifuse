package domain

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTransactionalChannelConstants(t *testing.T) {
	// Test that channel constants are defined correctly
	assert.Equal(t, TransactionalChannel("email"), TransactionalChannelEmail)
}

func TestChannelTemplates_Value(t *testing.T) {
	// Create test templates
	templates := ChannelTemplates{
		TransactionalChannelEmail: ChannelTemplate{
			TemplateID: "template-123",

			Settings: MapOfAny{
				"subject": "Test Subject",
			},
		},
	}

	// Test Value method
	value, err := templates.Value()
	require.NoError(t, err)

	// Verify value can be unmarshaled back to the same structure
	var actual ChannelTemplates
	err = json.Unmarshal(value.([]byte), &actual)
	require.NoError(t, err)

	assert.Equal(t, templates[TransactionalChannelEmail].TemplateID, actual[TransactionalChannelEmail].TemplateID)
	assert.Equal(t,
		templates[TransactionalChannelEmail].Settings["subject"],
		actual[TransactionalChannelEmail].Settings["subject"])
}

func TestChannelTemplates_Scan(t *testing.T) {
	// Create test templates to marshal
	original := ChannelTemplates{
		TransactionalChannelEmail: ChannelTemplate{
			TemplateID: "template-456",

			Settings: MapOfAny{
				"from_name": "Test Sender",
			},
		},
	}

	// Marshal to JSON bytes
	valueBytes, err := json.Marshal(original)
	require.NoError(t, err)

	// Test Scan method
	var scanned ChannelTemplates
	err = scanned.Scan(valueBytes)
	require.NoError(t, err)

	// Verify scanned matches original
	assert.Equal(t, original[TransactionalChannelEmail].TemplateID, scanned[TransactionalChannelEmail].TemplateID)
	assert.Equal(t,
		original[TransactionalChannelEmail].Settings["from_name"],
		scanned[TransactionalChannelEmail].Settings["from_name"])
}

func TestChannelTemplates_Scan_WithNil(t *testing.T) {
	// Test scanning nil value
	var templates ChannelTemplates
	err := templates.Scan(nil)
	require.NoError(t, err)
	assert.Empty(t, templates)
}

func TestChannelTemplates_Scan_WithInvalidType(t *testing.T) {
	// Test scanning invalid value type
	var templates ChannelTemplates
	err := templates.Scan(123) // Not a []byte
	require.Error(t, err)
	assert.Contains(t, err.Error(), "type assertion to []byte failed")
}

func TestTransactionalNotificationStructure(t *testing.T) {
	// Test that the struct has all required fields with the correct types
	now := time.Now().UTC()

	notification := TransactionalNotification{
		ID:          "notification-123",
		Name:        "Welcome Email",
		Description: "Sent when a user registers",
		Channels: ChannelTemplates{
			TransactionalChannelEmail: ChannelTemplate{
				TemplateID: "template-123",

				Settings: MapOfAny{
					"subject": "Welcome!",
				},
			},
		},

		Metadata:  MapOfAny{"category": "onboarding"},
		CreatedAt: now,
		UpdatedAt: now,
		DeletedAt: nil,
	}

	// Verify field values
	assert.Equal(t, "notification-123", notification.ID)
	assert.Equal(t, "Welcome Email", notification.Name)
	assert.Equal(t, "Sent when a user registers", notification.Description)
	assert.Equal(t, MapOfAny{"category": "onboarding"}, notification.Metadata)
	assert.Equal(t, now, notification.CreatedAt)
	assert.Equal(t, now, notification.UpdatedAt)
	assert.Nil(t, notification.DeletedAt)

	// Verify channel template
	template := notification.Channels[TransactionalChannelEmail]
	assert.Equal(t, "template-123", template.TemplateID)

	assert.Equal(t, "Welcome!", template.Settings["subject"])
}

func TestTransactionalNotificationCreateParams(t *testing.T) {
	// Test that the create params struct has all required fields
	params := TransactionalNotificationCreateParams{
		ID:          "notification-123",
		Name:        "Welcome Email",
		Description: "Sent when a user registers",
		Channels: ChannelTemplates{
			TransactionalChannelEmail: ChannelTemplate{
				TemplateID: "template-123",
			},
		},

		Metadata: MapOfAny{"category": "onboarding"},
	}

	// Verify field values
	assert.Equal(t, "notification-123", params.ID)
	assert.Equal(t, "Welcome Email", params.Name)
	assert.Equal(t, "Sent when a user registers", params.Description)

	assert.Equal(t, MapOfAny{"category": "onboarding"}, params.Metadata)

	// Verify channel template
	template := params.Channels[TransactionalChannelEmail]
	assert.Equal(t, "template-123", template.TemplateID)

}

func TestTransactionalNotificationUpdateParams(t *testing.T) {
	// Test that the update params struct has all required fields
	params := TransactionalNotificationUpdateParams{
		Name:        "Updated Welcome Email",
		Description: "Updated description",
		Channels: ChannelTemplates{
			TransactionalChannelEmail: ChannelTemplate{
				TemplateID: "template-456",
			},
		},

		Metadata: MapOfAny{"category": "updated"},
	}

	// Verify field values
	assert.Equal(t, "Updated Welcome Email", params.Name)
	assert.Equal(t, "Updated description", params.Description)

	assert.Equal(t, MapOfAny{"category": "updated"}, params.Metadata)

	// Verify channel template
	template := params.Channels[TransactionalChannelEmail]
	assert.Equal(t, "template-456", template.TemplateID)

}

func TestTransactionalNotificationSendParams(t *testing.T) {
	// Test that the send params struct has all required fields
	params := TransactionalNotificationSendParams{
		ID: "notification-123",
		Contact: &Contact{
			Email: "john@example.com",
		},
		Channels: []TransactionalChannel{TransactionalChannelEmail},
		Data: MapOfAny{
			"name":  "John Doe",
			"email": "john@example.com",
		},
		Metadata: MapOfAny{"source": "registration"},
		EmailOptions: EmailOptions{
			CC:      []string{"manager@example.com", "support@example.com"},
			BCC:     []string{"archive@example.com"},
			ReplyTo: "replies@example.com",
		},
	}

	// Verify field values
	assert.Equal(t, "notification-123", params.ID)
	assert.Equal(t, "john@example.com", params.Contact.Email)
	assert.Equal(t, []TransactionalChannel{TransactionalChannelEmail}, params.Channels)
	assert.Equal(t, MapOfAny{"name": "John Doe", "email": "john@example.com"}, params.Data)
	assert.Equal(t, MapOfAny{"source": "registration"}, params.Metadata)
	assert.Equal(t, []string{"manager@example.com", "support@example.com"}, params.EmailOptions.CC)
	assert.Equal(t, []string{"archive@example.com"}, params.EmailOptions.BCC)
	assert.Equal(t, "replies@example.com", params.EmailOptions.ReplyTo)
}

// Tests for request validation methods and URL parameter handling

func TestListTransactionalRequest_FromURLParams(t *testing.T) {
	tests := []struct {
		name    string
		values  map[string][]string
		want    ListTransactionalRequest
		wantErr bool
	}{
		{
			name: "valid basic request",
			values: map[string][]string{
				"workspace_id": {"workspace-123"},
			},
			want: ListTransactionalRequest{
				WorkspaceID: "workspace-123",
				Filter:      map[string]interface{}{},
			},
			wantErr: false,
		},
		{
			name: "valid request with  search",
			values: map[string][]string{
				"workspace_id": {"workspace-123"},
				"search":       {"welcome"},
			},
			want: ListTransactionalRequest{
				WorkspaceID: "workspace-123",

				Search: "welcome",
				Filter: map[string]interface{}{
					"search": "welcome",
				},
			},
			wantErr: false,
		},
		{
			name: "valid request with pagination",
			values: map[string][]string{
				"workspace_id": {"workspace-123"},
				"limit":        {"10"},
				"offset":       {"20"},
			},
			want: ListTransactionalRequest{
				WorkspaceID: "workspace-123",
				Limit:       10,
				Offset:      20,
				Filter:      map[string]interface{}{},
			},
			wantErr: false,
		},
		{
			name:    "missing workspace_id",
			values:  map[string][]string{},
			want:    ListTransactionalRequest{},
			wantErr: true,
		},
		{
			name: "invalid limit value",
			values: map[string][]string{
				"workspace_id": {"workspace-123"},
				"limit":        {"invalid"},
			},
			want: ListTransactionalRequest{
				WorkspaceID: "workspace-123",
				Filter:      map[string]interface{}{},
			},
			wantErr: false, // Invalid limit doesn't cause an error, just ignores the value
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := ListTransactionalRequest{}
			err := req.FromURLParams(tt.values)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want.WorkspaceID, req.WorkspaceID)
				assert.Equal(t, tt.want.Search, req.Search)
				assert.Equal(t, tt.want.Limit, req.Limit)
				assert.Equal(t, tt.want.Offset, req.Offset)

				// Check filter values
				for k, v := range tt.want.Filter {
					assert.Equal(t, v, req.Filter[k])
				}
			}
		})
	}
}

func TestGetTransactionalRequest_FromURLParams(t *testing.T) {
	tests := []struct {
		name    string
		values  map[string][]string
		want    GetTransactionalRequest
		wantErr bool
	}{
		{
			name: "valid request",
			values: map[string][]string{
				"workspace_id": {"workspace-123"},
				"id":           {"notification-456"},
			},
			want: GetTransactionalRequest{
				WorkspaceID: "workspace-123",
				ID:          "notification-456",
			},
			wantErr: false,
		},
		{
			name: "missing workspace_id",
			values: map[string][]string{
				"id": {"notification-456"},
			},
			want:    GetTransactionalRequest{},
			wantErr: true,
		},
		{
			name: "missing id",
			values: map[string][]string{
				"workspace_id": {"workspace-123"},
			},
			want:    GetTransactionalRequest{WorkspaceID: "workspace-123"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := GetTransactionalRequest{}
			err := req.FromURLParams(tt.values)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want.WorkspaceID, req.WorkspaceID)
				assert.Equal(t, tt.want.ID, req.ID)
			}
		})
	}
}

func TestCreateTransactionalRequest_Validate(t *testing.T) {
	tests := []struct {
		name    string
		req     CreateTransactionalRequest
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid request",
			req: CreateTransactionalRequest{
				WorkspaceID: "workspace-123",
				Notification: TransactionalNotificationCreateParams{
					ID:   "notification-456",
					Name: "Welcome Email",

					Channels: ChannelTemplates{
						TransactionalChannelEmail: ChannelTemplate{
							TemplateID: "template-789",
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "missing workspace_id",
			req: CreateTransactionalRequest{
				Notification: TransactionalNotificationCreateParams{
					ID:   "notification-456",
					Name: "Welcome Email",

					Channels: ChannelTemplates{
						TransactionalChannelEmail: ChannelTemplate{
							TemplateID: "template-789",
						},
					},
				},
			},
			wantErr: true,
			errMsg:  "workspace_id is required",
		},
		{
			name: "missing notification.id",
			req: CreateTransactionalRequest{
				WorkspaceID: "workspace-123",
				Notification: TransactionalNotificationCreateParams{
					Name: "Welcome Email",

					Channels: ChannelTemplates{
						TransactionalChannelEmail: ChannelTemplate{
							TemplateID: "template-789",
						},
					},
				},
			},
			wantErr: true,
			errMsg:  "notification.id is required",
		},
		{
			name: "missing notification.name",
			req: CreateTransactionalRequest{
				WorkspaceID: "workspace-123",
				Notification: TransactionalNotificationCreateParams{
					ID: "notification-456",

					Channels: ChannelTemplates{
						TransactionalChannelEmail: ChannelTemplate{
							TemplateID: "template-789",
						},
					},
				},
			},
			wantErr: true,
			errMsg:  "notification.name is required",
		},
		{
			name: "empty channels",
			req: CreateTransactionalRequest{
				WorkspaceID: "workspace-123",
				Notification: TransactionalNotificationCreateParams{
					ID:   "notification-456",
					Name: "Welcome Email",

					Channels: ChannelTemplates{},
				},
			},
			wantErr: true,
			errMsg:  "notification must have at least one channel",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Validate()

			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestUpdateTransactionalRequest_Validate(t *testing.T) {
	tests := []struct {
		name    string
		req     UpdateTransactionalRequest
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid request with name update",
			req: UpdateTransactionalRequest{
				WorkspaceID: "workspace-123",
				ID:          "notification-456",
				Updates: TransactionalNotificationUpdateParams{
					Name: "Updated Email",
				},
			},
			wantErr: false,
		},
		{
			name: "valid request with multiple updates",
			req: UpdateTransactionalRequest{
				WorkspaceID: "workspace-123",
				ID:          "notification-456",
				Updates: TransactionalNotificationUpdateParams{
					Name:        "Updated Email",
					Description: "Updated description",
				},
			},
			wantErr: false,
		},
		{
			name: "valid request with channel updates",
			req: UpdateTransactionalRequest{
				WorkspaceID: "workspace-123",
				ID:          "notification-456",
				Updates: TransactionalNotificationUpdateParams{
					Channels: ChannelTemplates{
						TransactionalChannelEmail: ChannelTemplate{
							TemplateID: "new-template-789",
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "missing workspace_id",
			req: UpdateTransactionalRequest{
				ID: "notification-456",
				Updates: TransactionalNotificationUpdateParams{
					Name: "Updated Email",
				},
			},
			wantErr: true,
			errMsg:  "workspace_id is required",
		},
		{
			name: "missing id",
			req: UpdateTransactionalRequest{
				WorkspaceID: "workspace-123",
				Updates: TransactionalNotificationUpdateParams{
					Name: "Updated Email",
				},
			},
			wantErr: true,
			errMsg:  "id is required",
		},
		{
			name: "no updates provided",
			req: UpdateTransactionalRequest{
				WorkspaceID: "workspace-123",
				ID:          "notification-456",
				Updates:     TransactionalNotificationUpdateParams{},
			},
			wantErr: true,
			errMsg:  "at least one field must be updated",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Validate()

			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestDeleteTransactionalRequest_Validate(t *testing.T) {
	tests := []struct {
		name    string
		req     DeleteTransactionalRequest
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid request",
			req: DeleteTransactionalRequest{
				WorkspaceID: "workspace-123",
				ID:          "notification-456",
			},
			wantErr: false,
		},
		{
			name: "missing workspace_id",
			req: DeleteTransactionalRequest{
				ID: "notification-456",
			},
			wantErr: true,
			errMsg:  "workspace_id is required",
		},
		{
			name: "missing id",
			req: DeleteTransactionalRequest{
				WorkspaceID: "workspace-123",
			},
			wantErr: true,
			errMsg:  "id is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Validate()

			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSendTransactionalRequest_Validate(t *testing.T) {
	tests := []struct {
		name    string
		req     SendTransactionalRequest
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid request",
			req: SendTransactionalRequest{
				WorkspaceID: "workspace-123",
				Notification: TransactionalNotificationSendParams{
					ID: "notification-456",
					Contact: &Contact{
						Email: "contact@example.com",
					},
					Channels: []TransactionalChannel{TransactionalChannelEmail},
					Data: MapOfAny{
						"name": "John Doe",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "valid request with cc and bcc",
			req: SendTransactionalRequest{
				WorkspaceID: "workspace-123",
				Notification: TransactionalNotificationSendParams{
					ID: "notification-456",
					Contact: &Contact{
						Email: "contact@example.com",
					},
					Channels: []TransactionalChannel{TransactionalChannelEmail},
					EmailOptions: EmailOptions{
						CC:      []string{"cc1@example.com", "cc2@example.com"},
						BCC:     []string{"bcc@example.com"},
						ReplyTo: "replies@example.com",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "valid request with channels",
			req: SendTransactionalRequest{
				WorkspaceID: "workspace-123",
				Notification: TransactionalNotificationSendParams{
					ID: "notification-456",
					Contact: &Contact{
						Email: "contact@example.com",
					},
					Channels: []TransactionalChannel{TransactionalChannelEmail},
				},
			},
			wantErr: false,
		},
		{
			name: "missing workspace_id",
			req: SendTransactionalRequest{
				Notification: TransactionalNotificationSendParams{
					ID: "notification-456",
					Contact: &Contact{
						Email: "contact@example.com",
					},
					Channels: []TransactionalChannel{TransactionalChannelEmail},
				},
			},
			wantErr: true,
			errMsg:  "workspace_id is required",
		},
		{
			name: "missing notification.id",
			req: SendTransactionalRequest{
				WorkspaceID: "workspace-123",
				Notification: TransactionalNotificationSendParams{
					Contact: &Contact{
						Email: "contact@example.com",
					},
					Channels: []TransactionalChannel{TransactionalChannelEmail},
				},
			},
			wantErr: true,
			errMsg:  "notification.id is required",
		},
		{
			name: "missing notification.contact",
			req: SendTransactionalRequest{
				WorkspaceID: "workspace-123",
				Notification: TransactionalNotificationSendParams{
					ID:       "notification-456",
					Channels: []TransactionalChannel{TransactionalChannelEmail},
				},
			},
			wantErr: true,
			errMsg:  "notification.contact is required",
		},
		{
			name: "invalid cc email",
			req: SendTransactionalRequest{
				WorkspaceID: "workspace-123",
				Notification: TransactionalNotificationSendParams{
					ID: "notification-456",
					Contact: &Contact{
						Email: "contact@example.com",
					},
					Channels: []TransactionalChannel{TransactionalChannelEmail},
					EmailOptions: EmailOptions{
						CC: []string{"not-an-email"},
					},
				},
			},
			wantErr: true,
			errMsg:  "cc 'not-an-email' must be a valid email address",
		},
		{
			name: "invalid bcc email",
			req: SendTransactionalRequest{
				WorkspaceID: "workspace-123",
				Notification: TransactionalNotificationSendParams{
					ID: "notification-456",
					Contact: &Contact{
						Email: "contact@example.com",
					},
					Channels: []TransactionalChannel{TransactionalChannelEmail},
					EmailOptions: EmailOptions{
						BCC: []string{"not-an-email"},
					},
				},
			},
			wantErr: true,
			errMsg:  "bcc 'not-an-email' must be a valid email address",
		},
		{
			name: "invalid replyTo email",
			req: SendTransactionalRequest{
				WorkspaceID: "workspace-123",
				Notification: TransactionalNotificationSendParams{
					ID: "notification-456",
					Contact: &Contact{
						Email: "contact@example.com",
					},
					Channels: []TransactionalChannel{TransactionalChannelEmail},
					EmailOptions: EmailOptions{
						ReplyTo: "not-an-email",
					},
				},
			},
			wantErr: true,
			errMsg:  "replyTo 'not-an-email' must be a valid email address",
		},
		{
			name: "missing channels",
			req: SendTransactionalRequest{
				WorkspaceID: "workspace-123",
				Notification: TransactionalNotificationSendParams{
					ID: "notification-456",
					Contact: &Contact{
						Email: "contact@example.com",
					},
					// Missing or empty Channels
					Channels: []TransactionalChannel{},
				},
			},
			wantErr: true,
			errMsg:  "notification must have at least one channel",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Validate()

			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// Test to ensure validation errors are created correctly
func TestListTransactionalRequest_FromURLParams_EdgeCases(t *testing.T) {
	// Test for properly handling non-integer values
	req := ListTransactionalRequest{}
	values := map[string][]string{
		"workspace_id": {"workspace-123"},
		"limit":        {"abc"}, // Non-integer value
		"offset":       {"xyz"}, // Non-integer value
	}
	err := req.FromURLParams(values)
	require.NoError(t, err)
	assert.Equal(t, "workspace-123", req.WorkspaceID)
	assert.Equal(t, 0, req.Limit)  // Should default to 0 for invalid value
	assert.Equal(t, 0, req.Offset) // Should default to 0 for invalid value

	// Test with empty arrays for values
	req = ListTransactionalRequest{}
	values = map[string][]string{
		"workspace_id": {},
	}
	err = req.FromURLParams(values)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "workspace_id is required")

	// Test with multiple values (should take first one)
	req = ListTransactionalRequest{}
	values = map[string][]string{
		"workspace_id": {"workspace-123", "workspace-456"},
		"limit":        {"10", "20"},
	}
	err = req.FromURLParams(values)
	require.NoError(t, err)
	assert.Equal(t, "workspace-123", req.WorkspaceID)
	assert.Equal(t, 10, req.Limit)
}

func TestListTransactionalRequest_Filter(t *testing.T) {
	// Test filter population with search
	req := ListTransactionalRequest{}
	values := map[string][]string{
		"workspace_id": {"workspace-123"},
		"search":       {"welcome"},
	}
	err := req.FromURLParams(values)
	require.NoError(t, err)
	assert.Equal(t, "welcome", req.Filter["search"])
}

// Test more edge cases for GetTransactionalRequest
func TestGetTransactionalRequest_FromURLParams_EdgeCases(t *testing.T) {
	// Test with empty ID array
	req := GetTransactionalRequest{}
	values := map[string][]string{
		"workspace_id": {"workspace-123"},
		"id":           {},
	}
	err := req.FromURLParams(values)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "id is required")

	// Test with multiple values in arrays (should take first one)
	req = GetTransactionalRequest{}
	values = map[string][]string{
		"workspace_id": {"workspace-123", "workspace-456"},
		"id":           {"notification-123", "notification-456"},
	}
	err = req.FromURLParams(values)
	require.NoError(t, err)
	assert.Equal(t, "workspace-123", req.WorkspaceID)
	assert.Equal(t, "notification-123", req.ID)
}

// Additional tests for CreateTransactionalRequest validation
func TestCreateTransactionalRequest_Validate_EdgeCases(t *testing.T) {
	// Test with nil channels map
	req := CreateTransactionalRequest{
		WorkspaceID: "workspace-123",
		Notification: TransactionalNotificationCreateParams{
			ID:   "notification-456",
			Name: "Welcome Email",

			Channels: nil, // Nil channels
		},
	}
	err := req.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "notification must have at least one channel")

	// Test with empty values
	req = CreateTransactionalRequest{
		WorkspaceID: "",
		Notification: TransactionalNotificationCreateParams{
			ID:   "",
			Name: "",
		},
	}
	err = req.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "workspace_id is required")

	// Create a valid request with minimal required fields
	req = CreateTransactionalRequest{
		WorkspaceID: "workspace-123",
		Notification: TransactionalNotificationCreateParams{
			ID:   "notification-456",
			Name: "Welcome Email",

			Channels: ChannelTemplates{
				TransactionalChannelEmail: ChannelTemplate{
					TemplateID: "template-789",
				},
			},
		},
	}
	err = req.Validate()
	require.NoError(t, err)
}

// Test metadata handling in requests
func TestRequestsWithMetadata(t *testing.T) {
	// Test create request with metadata
	createReq := CreateTransactionalRequest{
		WorkspaceID: "workspace-123",
		Notification: TransactionalNotificationCreateParams{
			ID:   "notification-456",
			Name: "Welcome Email",

			Channels: ChannelTemplates{
				TransactionalChannelEmail: ChannelTemplate{
					TemplateID: "template-789",
				},
			},
			Metadata: MapOfAny{
				"category":   "onboarding",
				"importance": "high",
				"tags":       []string{"welcome", "new-user"},
			},
		},
	}
	err := createReq.Validate()
	require.NoError(t, err)

	// Test update request with only metadata change
	updateReq := UpdateTransactionalRequest{
		WorkspaceID: "workspace-123",
		ID:          "notification-456",
		Updates: TransactionalNotificationUpdateParams{
			Metadata: MapOfAny{
				"category":   "account",
				"importance": "medium",
				"tags":       []string{"account", "update"},
			},
		},
	}
	err = updateReq.Validate()
	require.NoError(t, err)

	// Test send request with metadata
	sendReq := SendTransactionalRequest{
		WorkspaceID: "workspace-123",
		Notification: TransactionalNotificationSendParams{
			ID: "notification-456",
			Contact: &Contact{
				Email: "contact@example.com",
			},
			Channels: []TransactionalChannel{TransactionalChannelEmail},
			Metadata: MapOfAny{
				"source":      "api",
				"campaign_id": "campaign-123",
				"tracking":    true,
			},
		},
	}
	err = sendReq.Validate()
	require.NoError(t, err)
}

// Test channel settings in templates
func TestChannelTemplateSettings(t *testing.T) {
	// Test email channel template with various settings
	emailSettings := MapOfAny{
		"subject":    "Welcome to Our Service",
		"from_name":  "Support Team",
		"from_email": "support@example.com",
		"reply_to":   "no-reply@example.com",
		"cc":         []string{"manager@example.com", "team@example.com"},
		"bcc":        []string{"archive@example.com", "logs@example.com"},
		"custom_field": map[string]interface{}{
			"tracking_id": "abc123",
			"department":  "sales",
		},
	}

	template := ChannelTemplate{
		TemplateID: "template-123",
		Settings:   emailSettings,
	}

	// Create notification with this template
	req := CreateTransactionalRequest{
		WorkspaceID: "workspace-123",
		Notification: TransactionalNotificationCreateParams{
			ID:   "notification-456",
			Name: "Welcome Email",
			Channels: ChannelTemplates{
				TransactionalChannelEmail: template,
			},
		},
	}
	err := req.Validate()
	require.NoError(t, err)

	// Verify the settings are preserved
	assert.Equal(t, "Welcome to Our Service", req.Notification.Channels[TransactionalChannelEmail].Settings["subject"])
	assert.Equal(t, "Support Team", req.Notification.Channels[TransactionalChannelEmail].Settings["from_name"])
	assert.Equal(t, "support@example.com", req.Notification.Channels[TransactionalChannelEmail].Settings["from_email"])

	// Verify reply_to, cc, and bcc settings
	assert.Equal(t, "no-reply@example.com", req.Notification.Channels[TransactionalChannelEmail].Settings["reply_to"])

	// Verify cc addresses
	ccAddresses, ok := req.Notification.Channels[TransactionalChannelEmail].Settings["cc"].([]string)
	require.True(t, ok, "cc should be a string array")
	assert.Len(t, ccAddresses, 2)
	assert.Contains(t, ccAddresses, "manager@example.com")
	assert.Contains(t, ccAddresses, "team@example.com")

	// Verify bcc addresses
	bccAddresses, ok := req.Notification.Channels[TransactionalChannelEmail].Settings["bcc"].([]string)
	require.True(t, ok, "bcc should be a string array")
	assert.Len(t, bccAddresses, 2)
	assert.Contains(t, bccAddresses, "archive@example.com")
	assert.Contains(t, bccAddresses, "logs@example.com")

	// Test access to nested values
	customField := req.Notification.Channels[TransactionalChannelEmail].Settings["custom_field"].(map[string]interface{})
	assert.Equal(t, "abc123", customField["tracking_id"])
	assert.Equal(t, "sales", customField["department"])
}

// Test the additional fields in TransactionalNotificationSendParams when sending
func TestTransactionalNotificationSendParamsWithCcBcc(t *testing.T) {
	// Test sending with cc and bcc
	sendParams := TransactionalNotificationSendParams{
		ID: "notification-123",
		Contact: &Contact{
			Email: "contact@example.com",
		},
		Channels: []TransactionalChannel{TransactionalChannelEmail},
		Data: MapOfAny{
			"name": "John Doe",
		},
		EmailOptions: EmailOptions{
			CC:      []string{"cc1@example.com", "cc2@example.com"},
			BCC:     []string{"bcc@example.com"},
			ReplyTo: "replies@example.com",
		},
	}

	// Create a send request
	sendReq := SendTransactionalRequest{
		WorkspaceID:  "workspace-123",
		Notification: sendParams,
	}

	// Validate the request
	err := sendReq.Validate()
	require.NoError(t, err, "Valid request with cc, bcc and replyTo should not fail validation")

	// Verify values are preserved
	assert.Equal(t, []string{"cc1@example.com", "cc2@example.com"}, sendReq.Notification.EmailOptions.CC)
	assert.Equal(t, []string{"bcc@example.com"}, sendReq.Notification.EmailOptions.BCC)
	assert.Equal(t, "replies@example.com", sendReq.Notification.EmailOptions.ReplyTo)

	// Test JSON serialization and deserialization
	jsonData, err := json.Marshal(sendReq)
	require.NoError(t, err)

	var unmarshaled SendTransactionalRequest
	err = json.Unmarshal(jsonData, &unmarshaled)
	require.NoError(t, err)

	// Verify values are preserved through serialization
	assert.Equal(t, sendReq.Notification.EmailOptions.CC, unmarshaled.Notification.EmailOptions.CC)
	assert.Equal(t, sendReq.Notification.EmailOptions.BCC, unmarshaled.Notification.EmailOptions.BCC)
	assert.Equal(t, sendReq.Notification.EmailOptions.ReplyTo, unmarshaled.Notification.EmailOptions.ReplyTo)
}

// TestChannelTemplates_ComplexDataStructures tests the serialization and deserialization
// of complex nested data structures in ChannelTemplates
func TestChannelTemplates_ComplexDataStructures(t *testing.T) {
	// Create a complex template with nested data structures
	original := ChannelTemplates{
		TransactionalChannelEmail: ChannelTemplate{
			TemplateID: "complex-template",

			Settings: MapOfAny{
				"simple_string": "value",
				"simple_number": 123,
				"simple_bool":   true,
				"array_strings": []string{"one", "two", "three"},
				"array_numbers": []int{1, 2, 3},
				"nested_object": map[string]interface{}{
					"key1": "value1",
					"key2": 42,
					"nested_array": []interface{}{
						"string",
						123,
						map[string]interface{}{"deep_key": "deep_value"},
					},
				},
				"mixed_array": []interface{}{
					"string_value",
					42,
					true,
					[]string{"a", "b", "c"},
					map[string]interface{}{"key": "value"},
				},
				"null_value": nil,
			},
		},
	}

	// Test Value method - serialization
	value, err := original.Value()
	require.NoError(t, err)
	require.NotNil(t, value)
	jsonBytes, ok := value.([]byte)
	require.True(t, ok, "Value should return []byte")

	// Test Scan method - deserialization
	var scanned ChannelTemplates
	err = scanned.Scan(jsonBytes)
	require.NoError(t, err)

	// Verify all complex data was preserved
	originalTemplate := original[TransactionalChannelEmail]
	scannedTemplate := scanned[TransactionalChannelEmail]

	// Verify top-level properties
	assert.Equal(t, originalTemplate.TemplateID, scannedTemplate.TemplateID)

	// Verify simple settings
	assert.Equal(t, "value", scannedTemplate.Settings["simple_string"])
	assert.Equal(t, float64(123), scannedTemplate.Settings["simple_number"]) // JSON unmarshals numbers as float64
	assert.Equal(t, true, scannedTemplate.Settings["simple_bool"])
	assert.Nil(t, scannedTemplate.Settings["null_value"])

	// Verify array settings
	arrayStrings, ok := scannedTemplate.Settings["array_strings"].([]interface{})
	require.True(t, ok, "array_strings should be deserialized as []interface{}")
	assert.Equal(t, "one", arrayStrings[0])
	assert.Equal(t, "two", arrayStrings[1])
	assert.Equal(t, "three", arrayStrings[2])

	arrayNumbers, ok := scannedTemplate.Settings["array_numbers"].([]interface{})
	require.True(t, ok, "array_numbers should be deserialized as []interface{}")
	assert.Equal(t, float64(1), arrayNumbers[0])
	assert.Equal(t, float64(2), arrayNumbers[1])
	assert.Equal(t, float64(3), arrayNumbers[2])

	// Verify nested object
	nestedObject, ok := scannedTemplate.Settings["nested_object"].(map[string]interface{})
	require.True(t, ok, "nested_object should be deserialized as map[string]interface{}")
	assert.Equal(t, "value1", nestedObject["key1"])
	assert.Equal(t, float64(42), nestedObject["key2"])

	// Verify nested array in nested object
	nestedArray, ok := nestedObject["nested_array"].([]interface{})
	require.True(t, ok, "nested_array should be deserialized as []interface{}")
	assert.Equal(t, "string", nestedArray[0])
	assert.Equal(t, float64(123), nestedArray[1])

	// Verify deeply nested map
	deepMap, ok := nestedArray[2].(map[string]interface{})
	require.True(t, ok, "deepMap should be deserialized as map[string]interface{}")
	assert.Equal(t, "deep_value", deepMap["deep_key"])

	// Verify mixed array
	mixedArray, ok := scannedTemplate.Settings["mixed_array"].([]interface{})
	require.True(t, ok, "mixed_array should be deserialized as []interface{}")
	assert.Equal(t, "string_value", mixedArray[0])
	assert.Equal(t, float64(42), mixedArray[1])
	assert.Equal(t, true, mixedArray[2])

	// Verify array in mixed array
	nestedStringArray, ok := mixedArray[3].([]interface{})
	require.True(t, ok, "nestedStringArray should be deserialized as []interface{}")
	assert.Equal(t, "a", nestedStringArray[0])
	assert.Equal(t, "b", nestedStringArray[1])
	assert.Equal(t, "c", nestedStringArray[2])

	// Verify map in mixed array
	nestedMap, ok := mixedArray[4].(map[string]interface{})
	require.True(t, ok, "nestedMap should be deserialized as map[string]interface{}")
	assert.Equal(t, "value", nestedMap["key"])

	// Round-trip again and compare JSON representation for full equality
	originalJSON, err := json.Marshal(original)
	require.NoError(t, err)
	scannedJSON, err := json.Marshal(scanned)
	require.NoError(t, err)
	assert.JSONEq(t, string(originalJSON), string(scannedJSON), "JSON representations should be equal")
}

// TestTransactionalChannelHandling tests the handling of different TransactionalChannel values
func TestTransactionalChannelHandling(t *testing.T) {
	// Test email channel
	emailChannel := TransactionalChannelEmail
	assert.Equal(t, TransactionalChannel("email"), emailChannel)

	// Test that the channel value is preserved in templates
	templates := ChannelTemplates{
		emailChannel: ChannelTemplate{
			TemplateID: "template-123",
		},
	}

	// Verify the template exists for the email channel
	_, exists := templates[emailChannel]
	assert.True(t, exists)

	// Test serialization and deserialization of channels
	value, err := templates.Value()
	require.NoError(t, err)

	var scanned ChannelTemplates
	err = scanned.Scan(value)
	require.NoError(t, err)

	// Verify the channel is preserved
	_, exists = scanned[emailChannel]
	assert.True(t, exists)

	// Test using the channel in SendParams
	sendParams := TransactionalNotificationSendParams{
		ID: "notification-123",
		Contact: &Contact{
			Email: "contact@example.com",
		},
		Channels: []TransactionalChannel{emailChannel},
	}

	// Verify the channel is in the list
	assert.Contains(t, sendParams.Channels, emailChannel)

	// Test JSON serialization of channels
	data, err := json.Marshal(sendParams)
	require.NoError(t, err)

	var unmarshaled TransactionalNotificationSendParams
	err = json.Unmarshal(data, &unmarshaled)
	require.NoError(t, err)

	// Verify the channel was preserved through JSON serialization
	assert.Contains(t, unmarshaled.Channels, emailChannel)
}

func TestTestTemplateRequest_Validate(t *testing.T) {
	// Test TestTemplateRequest.Validate - this was at 0% coverage
	tests := []struct {
		name    string
		req     TestTemplateRequest
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid request",
			req: TestTemplateRequest{
				WorkspaceID:    "workspace-123",
				TemplateID:     "template-456",
				IntegrationID:  "integration-789",
				SenderID:       "sender-101",
				RecipientEmail: "test@example.com",
			},
			wantErr: false,
		},
		{
			name: "valid request with email options",
			req: TestTemplateRequest{
				WorkspaceID:    "workspace-123",
				TemplateID:     "template-456",
				IntegrationID:  "integration-789",
				SenderID:       "sender-101",
				RecipientEmail: "test@example.com",
				EmailOptions: EmailOptions{
					CC:      []string{"cc@example.com"},
					BCC:     []string{"bcc@example.com"},
					ReplyTo: "reply@example.com",
				},
			},
			wantErr: false,
		},
		{
			name: "missing workspace_id",
			req: TestTemplateRequest{
				TemplateID:     "template-456",
				IntegrationID:  "integration-789",
				SenderID:       "sender-101",
				RecipientEmail: "test@example.com",
			},
			wantErr: true,
			errMsg:  "workspace_id is required",
		},
		{
			name: "missing template_id",
			req: TestTemplateRequest{
				WorkspaceID:    "workspace-123",
				IntegrationID:  "integration-789",
				SenderID:       "sender-101",
				RecipientEmail: "test@example.com",
			},
			wantErr: true,
			errMsg:  "template_id is required",
		},
		{
			name: "missing integration_id",
			req: TestTemplateRequest{
				WorkspaceID:    "workspace-123",
				TemplateID:     "template-456",
				SenderID:       "sender-101",
				RecipientEmail: "test@example.com",
			},
			wantErr: true,
			errMsg:  "integration_id is required",
		},
		{
			name: "missing sender_id",
			req: TestTemplateRequest{
				WorkspaceID:    "workspace-123",
				TemplateID:     "template-456",
				IntegrationID:  "integration-789",
				RecipientEmail: "test@example.com",
			},
			wantErr: true,
			errMsg:  "sender_id is required",
		},
		{
			name: "missing recipient_email",
			req: TestTemplateRequest{
				WorkspaceID:   "workspace-123",
				TemplateID:    "template-456",
				IntegrationID: "integration-789",
				SenderID:      "sender-101",
			},
			wantErr: true,
			errMsg:  "recipient_email is required",
		},
		{
			name: "invalid recipient_email format",
			req: TestTemplateRequest{
				WorkspaceID:    "workspace-123",
				TemplateID:     "template-456",
				IntegrationID:  "integration-789",
				SenderID:       "sender-101",
				RecipientEmail: "not-an-email",
			},
			wantErr: true,
			errMsg:  "invalid recipient_email format",
		},
		{
			name: "empty recipient_email",
			req: TestTemplateRequest{
				WorkspaceID:    "workspace-123",
				TemplateID:     "template-456",
				IntegrationID:  "integration-789",
				SenderID:       "sender-101",
				RecipientEmail: "",
			},
			wantErr: true,
			errMsg:  "recipient_email is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Validate()

			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
