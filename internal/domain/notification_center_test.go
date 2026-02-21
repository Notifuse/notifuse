package domain

import (
	"context"
	"net/url"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/pkg/crypto"
	"github.com/stretchr/testify/assert"
)

func TestNotificationCenterRequest_Validate(t *testing.T) {
	tests := []struct {
		name    string
		request NotificationCenterRequest
		wantErr bool
	}{
		{
			name: "valid request with all required fields",
			request: NotificationCenterRequest{
				Email:       "test@example.com",
				EmailHMAC:   "hmac_value",
				WorkspaceID: "workspace_123",
			},
			wantErr: false,
		},
		{
			name: "missing email",
			request: NotificationCenterRequest{
				EmailHMAC:   "hmac_value",
				WorkspaceID: "workspace_123",
			},
			wantErr: true,
		},
		{
			name: "missing email_hmac",
			request: NotificationCenterRequest{
				Email:       "test@example.com",
				WorkspaceID: "workspace_123",
			},
			wantErr: true,
		},
		{
			name: "missing workspace_id",
			request: NotificationCenterRequest{
				Email:     "test@example.com",
				EmailHMAC: "hmac_value",
			},
			wantErr: true,
		},
		{
			name:    "empty request",
			request: NotificationCenterRequest{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.request.Validate()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestNotificationCenterRequest_FromURLValues(t *testing.T) {
	tests := []struct {
		name      string
		urlValues url.Values
		wantErr   bool
		expected  NotificationCenterRequest
	}{
		{
			name: "valid url values with all required fields",
			urlValues: url.Values{
				"email":        []string{"test@example.com"},
				"email_hmac":   []string{"hmac_value"},
				"workspace_id": []string{"workspace_123"},
			},
			wantErr: false,
			expected: NotificationCenterRequest{
				Email:       "test@example.com",
				EmailHMAC:   "hmac_value",
				WorkspaceID: "workspace_123",
			},
		},
		{
			name: "valid url values with confirm action and optional fields",
			urlValues: url.Values{
				"email":        []string{"test@example.com"},
				"email_hmac":   []string{"hmac_value"},
				"workspace_id": []string{"workspace_123"},
				"action":       []string{"confirm"},
				"lid":          []string{"list_456"},
				"mid":          []string{"msg_789"},
			},
			wantErr: false,
			expected: NotificationCenterRequest{
				Email:       "test@example.com",
				EmailHMAC:   "hmac_value",
				WorkspaceID: "workspace_123",
				Action:      "confirm",
				ListID:      "list_456",
				MessageID:   "msg_789",
			},
		},
		{
			name: "missing email",
			urlValues: url.Values{
				"email_hmac":   []string{"hmac_value"},
				"workspace_id": []string{"workspace_123"},
			},
			wantErr: true,
		},
		{
			name: "missing email_hmac",
			urlValues: url.Values{
				"email":        []string{"test@example.com"},
				"workspace_id": []string{"workspace_123"},
			},
			wantErr: true,
		},
		{
			name: "missing workspace_id",
			urlValues: url.Values{
				"email":      []string{"test@example.com"},
				"email_hmac": []string{"hmac_value"},
			},
			wantErr: true,
		},
		{
			name:      "empty url values",
			urlValues: url.Values{},
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := NotificationCenterRequest{}
			err := request.FromURLValues(tt.urlValues)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected.Email, request.Email)
				assert.Equal(t, tt.expected.EmailHMAC, request.EmailHMAC)
				assert.Equal(t, tt.expected.WorkspaceID, request.WorkspaceID)
				assert.Equal(t, tt.expected.Action, request.Action)
				assert.Equal(t, tt.expected.ListID, request.ListID)
				assert.Equal(t, tt.expected.MessageID, request.MessageID)
			}
		})
	}
}

func TestNotificationCenterResponse_Structure(t *testing.T) {
	// Test the structure of NotificationCenterResponse to ensure it has the expected fields
	response := ContactPreferencesResponse{}

	// Use reflection or direct field access to verify structure
	t.Run("has contact field", func(t *testing.T) {
		// Just checking that we can set and access the fields appropriately
		contact := &Contact{Email: "test@example.com"}
		response.Contact = contact
		assert.Equal(t, contact, response.Contact)
	})

	t.Run("has public lists field", func(t *testing.T) {
		now := time.Now()
		publicLists := []*List{
			{ID: "list1", Name: "Public List 1", CreatedAt: now, UpdatedAt: now},
			{ID: "list2", Name: "Public List 2", CreatedAt: now, UpdatedAt: now},
		}
		response.PublicLists = publicLists
		assert.Equal(t, publicLists, response.PublicLists)
		assert.Len(t, response.PublicLists, 2)
	})

	t.Run("has contact lists field", func(t *testing.T) {
		contactLists := []*ContactList{
			{Email: "test@example.com", ListID: "list1", Status: ContactListStatusActive},
			{Email: "test@example.com", ListID: "list2", Status: ContactListStatusActive},
		}
		response.ContactLists = contactLists
		assert.Equal(t, contactLists, response.ContactLists)
		assert.Len(t, response.ContactLists, 2)
	})

	t.Run("has logo URL field", func(t *testing.T) {
		logoURL := "https://example.com/logo.png"
		response.LogoURL = logoURL
		assert.Equal(t, logoURL, response.LogoURL)
	})

	t.Run("has website URL field", func(t *testing.T) {
		websiteURL := "https://example.com"
		response.WebsiteURL = websiteURL
		assert.Equal(t, websiteURL, response.WebsiteURL)
	})
}

// Additional tests for the Email HMAC functions
func TestComputeEmailHMAC(t *testing.T) {
	tests := []struct {
		name      string
		email     string
		secretKey string
		expected  string
	}{
		{
			name:      "regular email",
			email:     "test@example.com",
			secretKey: "secret-key-123",
			expected:  crypto.ComputeHMAC256([]byte("test@example.com"), "secret-key-123"),
		},
		{
			name:      "email with special characters",
			email:     "test+special@example.com",
			secretKey: "secret-key-123",
			expected:  crypto.ComputeHMAC256([]byte("test+special@example.com"), "secret-key-123"),
		},
		{
			name:      "empty email",
			email:     "",
			secretKey: "secret-key-123",
			expected:  crypto.ComputeHMAC256([]byte(""), "secret-key-123"),
		},
		{
			name:      "empty secret key",
			email:     "test@example.com",
			secretKey: "",
			expected:  crypto.ComputeHMAC256([]byte("test@example.com"), ""),
		},
		{
			name:      "both empty",
			email:     "",
			secretKey: "",
			expected:  crypto.ComputeHMAC256([]byte(""), ""),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ComputeEmailHMAC(tt.email, tt.secretKey)
			assert.Equal(t, tt.expected, result)

			// Double check with the crypto package directly
			directResult := crypto.ComputeHMAC256([]byte(tt.email), tt.secretKey)
			assert.Equal(t, directResult, result)
		})
	}
}

func TestVerifyEmailHMAC_WithComputeEmailHMAC(t *testing.T) {
	// Test that VerifyEmailHMAC correctly uses ComputeEmailHMAC
	tests := []struct {
		name       string
		email      string
		secretKey  string
		modifyHMAC func(string) string // Function to modify the HMAC for negative tests
		want       bool
	}{
		{
			name:       "valid HMAC verification",
			email:      "test@example.com",
			secretKey:  "secret-key-123",
			modifyHMAC: func(hmac string) string { return hmac }, // return unchanged
			want:       true,
		},
		{
			name:       "invalid HMAC verification",
			email:      "test@example.com",
			secretKey:  "secret-key-123",
			modifyHMAC: func(hmac string) string { return hmac + "invalid" }, // tamper with the HMAC
			want:       false,
		},
		{
			name:       "empty email",
			email:      "",
			secretKey:  "secret-key-123",
			modifyHMAC: func(hmac string) string { return hmac },
			want:       true, // Empty email is still a valid input, HMAC will be computed for empty string
		},
		{
			name:       "empty secret key",
			email:      "test@example.com",
			secretKey:  "",
			modifyHMAC: func(hmac string) string { return hmac },
			want:       true, // Empty secret key is still a valid input, though not secure
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Compute a valid HMAC first
			validHMAC := ComputeEmailHMAC(tt.email, tt.secretKey)

			// Apply the modification function (if any)
			providedHMAC := tt.modifyHMAC(validHMAC)

			// Verify the (potentially modified) HMAC
			result := VerifyEmailHMAC(tt.email, providedHMAC, tt.secretKey)
			assert.Equal(t, tt.want, result)
		})
	}
}

// This test is a simplified version of TestVerifyEmailHMAC in contact_test.go
// but it's included here for completeness of notification center related tests
func TestVerifyEmailHMAC_Comprehensive(t *testing.T) {
	email := "test@example.com"
	secretKey := "super-secret-key"

	// Compute a valid HMAC
	validHMAC := ComputeEmailHMAC(email, secretKey)

	// Test correct verification
	t.Run("valid HMAC verification", func(t *testing.T) {
		result := VerifyEmailHMAC(email, validHMAC, secretKey)
		assert.True(t, result)
	})

	// Test invalid HMAC
	t.Run("invalid HMAC verification", func(t *testing.T) {
		invalidHMAC := "invalid-hmac-value"
		result := VerifyEmailHMAC(email, invalidHMAC, secretKey)
		assert.False(t, result)
	})

	// Test different email
	t.Run("different email HMAC verification", func(t *testing.T) {
		differentEmail := "other@example.com"
		result := VerifyEmailHMAC(differentEmail, validHMAC, secretKey)
		assert.False(t, result)
	})

	// Test different secret key
	t.Run("different secret key HMAC verification", func(t *testing.T) {
		differentKey := "different-secret-key"
		result := VerifyEmailHMAC(email, validHMAC, differentKey)
		assert.False(t, result)
	})
}

func TestUpdateContactPreferencesRequest_Validate(t *testing.T) {
	tests := []struct {
		name    string
		request UpdateContactPreferencesRequest
		wantErr string
	}{
		{
			name: "valid with both language and timezone",
			request: UpdateContactPreferencesRequest{
				WorkspaceID: "ws1",
				Email:       "test@example.com",
				EmailHMAC:   "hmac",
				Language:    "fr",
				Timezone:    "Europe/Paris",
			},
		},
		{
			name: "valid with language only",
			request: UpdateContactPreferencesRequest{
				WorkspaceID: "ws1",
				Email:       "test@example.com",
				EmailHMAC:   "hmac",
				Language:    "en",
			},
		},
		{
			name: "valid with timezone only",
			request: UpdateContactPreferencesRequest{
				WorkspaceID: "ws1",
				Email:       "test@example.com",
				EmailHMAC:   "hmac",
				Timezone:    "America/New_York",
			},
		},
		{
			name: "missing workspace_id",
			request: UpdateContactPreferencesRequest{
				Email:     "test@example.com",
				EmailHMAC: "hmac",
				Language:  "fr",
			},
			wantErr: "workspace_id is required",
		},
		{
			name: "missing email",
			request: UpdateContactPreferencesRequest{
				WorkspaceID: "ws1",
				EmailHMAC:   "hmac",
				Language:    "fr",
			},
			wantErr: "email is required",
		},
		{
			name: "missing email_hmac",
			request: UpdateContactPreferencesRequest{
				WorkspaceID: "ws1",
				Email:       "test@example.com",
				Language:    "fr",
			},
			wantErr: "email_hmac is required",
		},
		{
			name: "no language or timezone",
			request: UpdateContactPreferencesRequest{
				WorkspaceID: "ws1",
				Email:       "test@example.com",
				EmailHMAC:   "hmac",
			},
			wantErr: "at least one of language or timezone must be provided",
		},
		{
			name: "invalid language - too long",
			request: UpdateContactPreferencesRequest{
				WorkspaceID: "ws1",
				Email:       "test@example.com",
				EmailHMAC:   "hmac",
				Language:    "fra",
			},
			wantErr: "language must be a 2-letter lowercase code",
		},
		{
			name: "invalid language - uppercase",
			request: UpdateContactPreferencesRequest{
				WorkspaceID: "ws1",
				Email:       "test@example.com",
				EmailHMAC:   "hmac",
				Language:    "FR",
			},
			wantErr: "language must be a 2-letter lowercase code",
		},
		{
			name: "invalid language - digits",
			request: UpdateContactPreferencesRequest{
				WorkspaceID: "ws1",
				Email:       "test@example.com",
				EmailHMAC:   "hmac",
				Language:    "12",
			},
			wantErr: "language must be a 2-letter lowercase code",
		},
		{
			name: "invalid timezone - too short",
			request: UpdateContactPreferencesRequest{
				WorkspaceID: "ws1",
				Email:       "test@example.com",
				EmailHMAC:   "hmac",
				Timezone:    "X",
			},
			wantErr: "timezone must be between 2 and 50 characters",
		},
		{
			name: "invalid timezone - too long",
			request: UpdateContactPreferencesRequest{
				WorkspaceID: "ws1",
				Email:       "test@example.com",
				EmailHMAC:   "hmac",
				Timezone:    "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
			},
			wantErr: "timezone must be between 2 and 50 characters",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.request.Validate()
			if tt.wantErr != "" {
				assert.EqualError(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestNotificationCenterServiceInterface(t *testing.T) {
	// This test is simply a placeholder to ensure our interface definition is correct
	// We're not testing actual implementation here
	t.Run("placeholder for interface test", func(t *testing.T) {
		// Verify interface is properly defined by checking it has the expected method
		var _ NotificationCenterService = (*mockNotificationCenter)(nil)
	})
}

// Simple mock implementation of NotificationCenterService
type mockNotificationCenter struct{}

func (m *mockNotificationCenter) GetContactPreferences(_ context.Context, _ string, _ string, _ string) (*ContactPreferencesResponse, error) {
	return nil, nil
}

func (m *mockNotificationCenter) UpdateContactPreferences(_ context.Context, _ *UpdateContactPreferencesRequest) error {
	return nil
}
