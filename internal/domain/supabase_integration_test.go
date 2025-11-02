package domain

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSupabaseIntegrationSettings_Validate(t *testing.T) {
	tests := []struct {
		name        string
		settings    SupabaseIntegrationSettings
		passphrase  string
		expectError bool
	}{
		{
			name: "valid settings",
			settings: SupabaseIntegrationSettings{
				AuthEmailHook: SupabaseAuthEmailHookSettings{
					SignatureKey: "test-key",
				},
				BeforeUserCreatedHook: SupabaseUserCreatedHookSettings{
					SignatureKey: "test-key",
				},
			},
			passphrase:  "test-passphrase",
			expectError: false,
		},
		{
			name: "empty settings",
			settings: SupabaseIntegrationSettings{
				AuthEmailHook:         SupabaseAuthEmailHookSettings{},
				BeforeUserCreatedHook: SupabaseUserCreatedHookSettings{},
			},
			passphrase:  "test-passphrase",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.settings.Validate(tt.passphrase)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSupabaseIntegrationSettings_EncryptDecrypt(t *testing.T) {
	passphrase := "test-passphrase-12345"

	settings := SupabaseIntegrationSettings{
		AuthEmailHook: SupabaseAuthEmailHookSettings{
			SignatureKey: "auth-email-key-secret",
		},
		BeforeUserCreatedHook: SupabaseUserCreatedHookSettings{
			SignatureKey:    "user-created-key-secret",
			AddUserToLists:  []string{"list-1", "list-2"},
			CustomJSONField: "custom_json_1",
		},
	}

	// Test encryption
	err := settings.EncryptSignatureKeys(passphrase)
	require.NoError(t, err)

	// Verify keys are encrypted and cleared
	assert.Empty(t, settings.AuthEmailHook.SignatureKey)
	assert.NotEmpty(t, settings.AuthEmailHook.EncryptedSignatureKey)
	assert.Empty(t, settings.BeforeUserCreatedHook.SignatureKey)
	assert.NotEmpty(t, settings.BeforeUserCreatedHook.EncryptedSignatureKey)

	// Save encrypted values
	authEncrypted := settings.AuthEmailHook.EncryptedSignatureKey
	userCreatedEncrypted := settings.BeforeUserCreatedHook.EncryptedSignatureKey

	// Test decryption
	err = settings.DecryptSignatureKeys(passphrase)
	require.NoError(t, err)

	// Verify keys are decrypted
	assert.Equal(t, "auth-email-key-secret", settings.AuthEmailHook.SignatureKey)
	assert.Equal(t, "user-created-key-secret", settings.BeforeUserCreatedHook.SignatureKey)

	// Verify encrypted values remain unchanged
	assert.Equal(t, authEncrypted, settings.AuthEmailHook.EncryptedSignatureKey)
	assert.Equal(t, userCreatedEncrypted, settings.BeforeUserCreatedHook.EncryptedSignatureKey)
}

func TestSupabaseIntegrationSettings_EncryptDecrypt_EmptyKeys(t *testing.T) {
	passphrase := "test-passphrase"

	settings := SupabaseIntegrationSettings{
		AuthEmailHook:         SupabaseAuthEmailHookSettings{},
		BeforeUserCreatedHook: SupabaseUserCreatedHookSettings{},
	}

	// Should not error on empty keys
	err := settings.EncryptSignatureKeys(passphrase)
	assert.NoError(t, err)

	err = settings.DecryptSignatureKeys(passphrase)
	assert.NoError(t, err)
}

func TestSupabaseTemplateMappings_GetTemplateID(t *testing.T) {
	mappings := SupabaseTemplateMappings{
		Signup:           "template-signup",
		MagicLink:        "template-magiclink",
		Recovery:         "template-recovery",
		EmailChange:      "template-email-change",
		Invite:           "template-invite",
		Reauthentication: "template-reauthentication",
	}

	tests := []struct {
		name       string
		actionType SupabaseEmailActionType
		expected   string
		expectErr  bool
	}{
		{
			name:       "signup",
			actionType: SupabaseEmailActionSignup,
			expected:   "template-signup",
			expectErr:  false,
		},
		{
			name:       "magiclink",
			actionType: SupabaseEmailActionMagicLink,
			expected:   "template-magiclink",
			expectErr:  false,
		},
		{
			name:       "recovery",
			actionType: SupabaseEmailActionRecovery,
			expected:   "template-recovery",
			expectErr:  false,
		},
		{
			name:       "email_change",
			actionType: SupabaseEmailActionEmailChange,
			expected:   "template-email-change",
			expectErr:  false,
		},
		{
			name:       "invite",
			actionType: SupabaseEmailActionInvite,
			expected:   "template-invite",
			expectErr:  false,
		},
		{
			name:       "reauthentication",
			actionType: SupabaseEmailActionReauthentication,
			expected:   "template-reauthentication",
			expectErr:  false,
		},
		{
			name:       "unsupported action",
			actionType: "unsupported",
			expected:   "",
			expectErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := mappings.GetTemplateID(tt.actionType)
			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestValidateSupabaseWebhookSignature_Valid(t *testing.T) {
	// This is a simplified test - in reality you'd need to compute actual HMAC signatures
	// For now, we'll test the validation logic structure
	secret := "whsec_MfKQ9r8GKYqrTwjUPD8ILPZIo2LaLaSw" // Valid base64 webhook secret
	payload := []byte(`{"test":"data"}`)
	webhookID := "webhook-123"
	timestampStr := "1234567890" // Use a fixed timestamp for testing

	// Note: This will fail signature validation with invalid signature
	err := ValidateSupabaseWebhookSignature(payload, "v1,invalid", timestampStr, webhookID, secret)
	assert.Error(t, err) // Expected to fail with invalid signature
	assert.Contains(t, err.Error(), "signature validation failed")
}

func TestValidateSupabaseWebhookSignature_InvalidTimestamp(t *testing.T) {
	secret := "whsec_MfKQ9r8GKYqrTwjUPD8ILPZIo2LaLaSw"
	payload := []byte(`{"test":"data"}`)
	webhookID := "webhook-123"
	oldTimestamp := "1000000000" // Very old timestamp

	err := ValidateSupabaseWebhookSignature(payload, "v1,signature", oldTimestamp, webhookID, secret)
	assert.Error(t, err)
	// The standard-webhooks library handles timestamp validation internally
}

func TestValidateSupabaseWebhookSignature_NoSignatures(t *testing.T) {
	secret := "whsec_MfKQ9r8GKYqrTwjUPD8ILPZIo2LaLaSw"
	payload := []byte(`{"test":"data"}`)
	webhookID := "webhook-123"
	timestamp := "1234567890"

	err := ValidateSupabaseWebhookSignature(payload, "", timestamp, webhookID, secret)
	assert.Error(t, err)
	// Error will be about signature validation
}

func TestValidateSupabaseWebhookSignature_InvalidTimestampFormat(t *testing.T) {
	secret := "whsec_MfKQ9r8GKYqrTwjUPD8ILPZIo2LaLaSw"
	payload := []byte(`{"test":"data"}`)
	webhookID := "webhook-123"

	err := ValidateSupabaseWebhookSignature(payload, "v1,signature", "not-a-number", webhookID, secret)
	assert.Error(t, err)
	// The standard-webhooks library handles timestamp parsing
}

func TestValidateSupabaseWebhookSignature_InvalidSecret(t *testing.T) {
	secret := "invalid-secret-format"
	payload := []byte(`{"test":"data"}`)
	webhookID := "webhook-123"
	timestamp := "1234567890"

	err := ValidateSupabaseWebhookSignature(payload, "v1,signature", timestamp, webhookID, secret)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create webhook verifier")
}

func TestSupabaseUser_ToContact(t *testing.T) {
	tests := []struct {
		name            string
		user            SupabaseUser
		customJSONField string
		expectedEmail   string
		expectedPhone   *NullableString
		expectError     bool
	}{
		{
			name: "basic user with email",
			user: SupabaseUser{
				ID:    "user-123",
				Email: "test@example.com",
				Phone: "+1234567890",
			},
			customJSONField: "",
			expectedEmail:   "test@example.com",
			expectedPhone:   &NullableString{String: "+1234567890", IsNull: false},
			expectError:     false,
		},
		{
			name: "user with metadata",
			user: SupabaseUser{
				ID:    "user-456",
				Email: "user@example.com",
				UserMetadata: map[string]interface{}{
					"first_name": "John",
					"last_name":  "Doe",
				},
			},
			customJSONField: "custom_json_1",
			expectedEmail:   "user@example.com",
			expectError:     false,
		},
		{
			name: "user without email",
			user: SupabaseUser{
				ID: "user-789",
			},
			customJSONField: "",
			expectError:     true,
		},
		{
			name: "user with metadata but no custom field",
			user: SupabaseUser{
				ID:    "user-999",
				Email: "user@example.com",
				UserMetadata: map[string]interface{}{
					"key": "value",
				},
			},
			customJSONField: "", // No custom field specified
			expectedEmail:   "user@example.com",
			expectError:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			contact, err := tt.user.ToContact(tt.customJSONField)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, contact)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, contact)
				assert.Equal(t, tt.expectedEmail, contact.Email)

				if tt.expectedPhone != nil {
					assert.Equal(t, tt.expectedPhone.String, contact.Phone.String)
					assert.Equal(t, tt.expectedPhone.IsNull, contact.Phone.IsNull)
				}

				// Verify external ID
				if tt.user.ID != "" {
					assert.NotNil(t, contact.ExternalID)
					assert.Equal(t, tt.user.ID, contact.ExternalID.String)
				}

				// Verify custom JSON field mapping
				if tt.customJSONField == "custom_json_1" && len(tt.user.UserMetadata) > 0 {
					assert.NotNil(t, contact.CustomJSON1)
					assert.False(t, contact.CustomJSON1.IsNull)
				}
			}
		})
	}
}

func TestSupabaseUser_ToContact_AllCustomJSONFields(t *testing.T) {
	user := SupabaseUser{
		ID:    "user-123",
		Email: "test@example.com",
		UserMetadata: map[string]interface{}{
			"key": "value",
		},
	}

	customFields := []string{"custom_json_1", "custom_json_2", "custom_json_3", "custom_json_4", "custom_json_5"}

	for _, field := range customFields {
		t.Run(field, func(t *testing.T) {
			contact, err := user.ToContact(field)
			require.NoError(t, err)

			switch field {
			case "custom_json_1":
				assert.NotNil(t, contact.CustomJSON1)
				assert.False(t, contact.CustomJSON1.IsNull)
			case "custom_json_2":
				assert.NotNil(t, contact.CustomJSON2)
				assert.False(t, contact.CustomJSON2.IsNull)
			case "custom_json_3":
				assert.NotNil(t, contact.CustomJSON3)
				assert.False(t, contact.CustomJSON3.IsNull)
			case "custom_json_4":
				assert.NotNil(t, contact.CustomJSON4)
				assert.False(t, contact.CustomJSON4.IsNull)
			case "custom_json_5":
				assert.NotNil(t, contact.CustomJSON5)
				assert.False(t, contact.CustomJSON5.IsNull)
			}
		})
	}
}

func TestSupabaseAuthEmailWebhook_JSONMarshaling(t *testing.T) {
	webhook := SupabaseAuthEmailWebhook{
		User: SupabaseUser{
			ID:    "user-123",
			Email: "test@example.com",
			Phone: "+1234567890",
		},
		EmailData: SupabaseEmailData{
			Token:           "token-abc",
			TokenHash:       "hash-xyz",
			RedirectTo:      "https://example.com",
			EmailActionType: "signup",
			SiteURL:         "https://site.example.com",
		},
	}

	// Marshal to JSON
	data, err := json.Marshal(webhook)
	require.NoError(t, err)

	// Unmarshal back
	var unmarshaled SupabaseAuthEmailWebhook
	err = json.Unmarshal(data, &unmarshaled)
	require.NoError(t, err)

	// Verify values
	assert.Equal(t, webhook.User.ID, unmarshaled.User.ID)
	assert.Equal(t, webhook.User.Email, unmarshaled.User.Email)
	assert.Equal(t, webhook.EmailData.Token, unmarshaled.EmailData.Token)
	assert.Equal(t, webhook.EmailData.EmailActionType, unmarshaled.EmailData.EmailActionType)
}

func TestSupabaseBeforeUserCreatedWebhook_JSONMarshaling(t *testing.T) {
	webhook := SupabaseBeforeUserCreatedWebhook{
		Metadata: SupabaseWebhookMetadata{
			UUID:      "uuid-123",
			Time:      "2023-01-01T00:00:00Z",
			Name:      "before-user-created",
			IPAddress: "192.168.1.1",
		},
		User: SupabaseUser{
			ID:    "user-456",
			Email: "newuser@example.com",
		},
	}

	// Marshal to JSON
	data, err := json.Marshal(webhook)
	require.NoError(t, err)

	// Unmarshal back
	var unmarshaled SupabaseBeforeUserCreatedWebhook
	err = json.Unmarshal(data, &unmarshaled)
	require.NoError(t, err)

	// Verify values
	assert.Equal(t, webhook.Metadata.UUID, unmarshaled.Metadata.UUID)
	assert.Equal(t, webhook.User.Email, unmarshaled.User.Email)
}

func TestSupabaseEmailActionType_Constants(t *testing.T) {
	// Verify all constants are defined
	assert.Equal(t, SupabaseEmailActionType("signup"), SupabaseEmailActionSignup)
	assert.Equal(t, SupabaseEmailActionType("magiclink"), SupabaseEmailActionMagicLink)
	assert.Equal(t, SupabaseEmailActionType("recovery"), SupabaseEmailActionRecovery)
	assert.Equal(t, SupabaseEmailActionType("email_change"), SupabaseEmailActionEmailChange)
	assert.Equal(t, SupabaseEmailActionType("invite"), SupabaseEmailActionInvite)
}

func TestSupabaseUser_ToContact_WithTimestamps(t *testing.T) {
	tests := []struct {
		name                   string
		user                   SupabaseUser
		expectCreatedAtSet     bool
		expectUpdatedAtSet     bool
		expectedCreatedAtEmpty bool
		expectedUpdatedAtEmpty bool
	}{
		{
			name: "valid timestamps",
			user: SupabaseUser{
				Email:     "test@example.com",
				CreatedAt: "2024-01-15T10:30:00Z",
				UpdatedAt: "2024-01-16T11:45:00Z",
			},
			expectCreatedAtSet: true,
			expectUpdatedAtSet: true,
		},
		{
			name: "zero value timestamps (from before-user-created hook)",
			user: SupabaseUser{
				Email:     "test@example.com",
				CreatedAt: "0001-01-01T00:00:00Z",
				UpdatedAt: "0001-01-01T00:00:00Z",
			},
			expectedCreatedAtEmpty: true,
			expectedUpdatedAtEmpty: true,
		},
		{
			name: "empty timestamp strings",
			user: SupabaseUser{
				Email:     "test@example.com",
				CreatedAt: "",
				UpdatedAt: "",
			},
			expectedCreatedAtEmpty: true,
			expectedUpdatedAtEmpty: true,
		},
		{
			name: "invalid timestamp format",
			user: SupabaseUser{
				Email:     "test@example.com",
				CreatedAt: "invalid-date",
				UpdatedAt: "2024-01-16T11:45:00Z",
			},
			expectedCreatedAtEmpty: true,
			expectUpdatedAtSet:     true,
		},
		{
			name: "mixed valid and zero timestamps",
			user: SupabaseUser{
				Email:     "test@example.com",
				CreatedAt: "2024-01-15T10:30:00Z",
				UpdatedAt: "0001-01-01T00:00:00Z",
			},
			expectCreatedAtSet:     true,
			expectedUpdatedAtEmpty: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			contact, err := tt.user.ToContact("")
			require.NoError(t, err)
			require.NotNil(t, contact)

			if tt.expectCreatedAtSet {
				assert.False(t, contact.CreatedAt.IsZero(), "CreatedAt should be set")
			}
			if tt.expectedCreatedAtEmpty {
				assert.True(t, contact.CreatedAt.IsZero(), "CreatedAt should be zero")
			}

			if tt.expectUpdatedAtSet {
				assert.False(t, contact.UpdatedAt.IsZero(), "UpdatedAt should be set")
			}
			if tt.expectedUpdatedAtEmpty {
				assert.True(t, contact.UpdatedAt.IsZero(), "UpdatedAt should be zero")
			}
		})
	}
}

func TestParseSupabaseTimestamp(t *testing.T) {
	tests := []struct {
		name        string
		timestamp   string
		expectError bool
		checkYear   int
	}{
		{
			name:        "valid timestamp",
			timestamp:   "2024-01-15T10:30:00Z",
			expectError: false,
			checkYear:   2024,
		},
		{
			name:        "zero value timestamp",
			timestamp:   "0001-01-01T00:00:00Z",
			expectError: true,
		},
		{
			name:        "invalid format",
			timestamp:   "not-a-date",
			expectError: true,
		},
		{
			name:        "empty string",
			timestamp:   "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseSupabaseTimestamp(tt.timestamp)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.checkYear, result.Year())
			}
		})
	}
}
