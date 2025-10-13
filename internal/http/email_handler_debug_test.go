package http

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEmailProviderRequestDeserialization tests that sender names are preserved during JSON deserialization
func TestEmailProviderRequestDeserialization(t *testing.T) {
	t.Run("Sender name preserved in JSON deserialization", func(t *testing.T) {
		// Create a request payload mimicking what the frontend sends
		requestPayload := map[string]interface{}{
			"workspace_id": "workspace-123",
			"to":           "test@example.com",
			"provider": map[string]interface{}{
				"kind":                 "smtp",
				"rate_limit_per_minute": 25,
				"senders": []map[string]interface{}{
					{
						"id":         "sender-123",
						"email":      "noreply@notifuse.com",
						"name":       "Notifuse", // This is the critical field
						"is_default": true,
					},
				},
				"smtp": map[string]interface{}{
					"host":     "smtp.example.com",
					"port":     587,
					"username": "user@example.com",
					"password": "password",
					"use_tls":  true,
				},
			},
		}

		// Serialize to JSON (what frontend does)
		jsonData, err := json.Marshal(requestPayload)
		require.NoError(t, err)

		t.Logf("JSON payload: %s", string(jsonData))

		// Deserialize to Go struct (what backend does)
		var req domain.TestEmailProviderRequest
		err = json.NewDecoder(bytes.NewReader(jsonData)).Decode(&req)
		require.NoError(t, err)

		// Verify the provider is correctly deserialized
		require.NotNil(t, req.Provider.SMTP)
		assert.Equal(t, domain.EmailProviderKindSMTP, req.Provider.Kind)
		assert.Len(t, req.Provider.Senders, 1)

		// CRITICAL CHECK: Verify sender name is preserved
		sender := req.Provider.Senders[0]
		assert.Equal(t, "sender-123", sender.ID)
		assert.Equal(t, "noreply@notifuse.com", sender.Email)
		assert.Equal(t, "Notifuse", sender.Name, "Sender name should be preserved after JSON deserialization")
		assert.True(t, sender.IsDefault)

		t.Logf("✅ Sender deserialized correctly:")
		t.Logf("   ID: %s", sender.ID)
		t.Logf("   Email: %s", sender.Email)
		t.Logf("   Name: '%s'", sender.Name)
		t.Logf("   IsDefault: %v", sender.IsDefault)
	})

	t.Run("Empty sender name is detected", func(t *testing.T) {
		// Test with empty sender name
		requestPayload := map[string]interface{}{
			"workspace_id": "workspace-123",
			"to":           "test@example.com",
			"provider": map[string]interface{}{
				"kind": "smtp",
				"senders": []map[string]interface{}{
					{
						"id":         "sender-123",
						"email":      "noreply@notifuse.com",
						"name":       "", // Empty name
						"is_default": true,
					},
				},
				"smtp": map[string]interface{}{
					"host": "smtp.example.com",
					"port": 587,
				},
			},
		}

		jsonData, err := json.Marshal(requestPayload)
		require.NoError(t, err)

		var req domain.TestEmailProviderRequest
		err = json.NewDecoder(bytes.NewReader(jsonData)).Decode(&req)
		require.NoError(t, err)

		sender := req.Provider.Senders[0]
		assert.Empty(t, sender.Name, "Empty sender name should remain empty (not converted to something else)")

		t.Logf("⚠️ Empty name case:")
		t.Logf("   Email: %s", sender.Email)
		t.Logf("   Name: '%s' (empty)", sender.Name)
	})

	t.Run("Missing name field in JSON", func(t *testing.T) {
		// Test without name field at all
		requestPayload := map[string]interface{}{
			"workspace_id": "workspace-123",
			"to":           "test@example.com",
			"provider": map[string]interface{}{
				"kind": "smtp",
				"senders": []map[string]interface{}{
					{
						"id":         "sender-123",
						"email":      "noreply@notifuse.com",
						// "name" field is missing entirely
						"is_default": true,
					},
				},
			},
		}

		jsonData, err := json.Marshal(requestPayload)
		require.NoError(t, err)

		t.Logf("JSON without name field: %s", string(jsonData))

		var req domain.TestEmailProviderRequest
		err = json.NewDecoder(bytes.NewReader(jsonData)).Decode(&req)
		require.NoError(t, err)

		sender := req.Provider.Senders[0]
		assert.Empty(t, sender.Name, "Missing name field should result in empty string")

		t.Logf("⚠️ Missing name field case:")
		t.Logf("   Email: %s", sender.Email)
		t.Logf("   Name: '%s' (should be empty)", sender.Name)
	})
}

// TestHandleTestEmailProviderWithSenderName tests the full handler with sender name
func TestHandleTestEmailProviderWithSenderName(t *testing.T) {
	t.Run("Handler receives sender name correctly - simplified test", func(t *testing.T) {
		// Just test that the JSON deserialization works correctly
		// The actual handler testing is covered in email_handler_test.go

		requestPayload := domain.TestEmailProviderRequest{
			WorkspaceID: "workspace-123",
			To:          "test@example.com",
			Provider: domain.EmailProvider{
				Kind:               domain.EmailProviderKindSMTP,
				RateLimitPerMinute: 25,
				Senders: []domain.EmailSender{
					{
						ID:        "sender-123",
						Email:     "noreply@notifuse.com",
						Name:      "Notifuse Platform", // Sender name
						IsDefault: true,
					},
				},
				SMTP: &domain.SMTPSettings{
					Host:     "smtp.example.com",
					Port:     587,
					Username: "user",
					Password: "pass",
					UseTLS:   true,
				},
			},
		}

		// Serialize and deserialize
		reqBody, err := json.Marshal(requestPayload)
		require.NoError(t, err)

		var deserialized domain.TestEmailProviderRequest
		err = json.NewDecoder(bytes.NewReader(reqBody)).Decode(&deserialized)
		require.NoError(t, err)

		// Verify the provider was deserialized correctly with sender name
		require.Len(t, deserialized.Provider.Senders, 1)
		assert.Equal(t, "Notifuse Platform", deserialized.Provider.Senders[0].Name,
			"Sender name should be preserved through JSON round-trip")

		t.Logf("✅ JSON round-trip test passed")
		t.Logf("   Sender name preserved: '%s'", deserialized.Provider.Senders[0].Name)
	})
}

// TestFrontendToBackendFlow simulates the exact frontend-to-backend flow
func TestFrontendToBackendFlow(t *testing.T) {
	t.Run("Complete frontend-to-backend flow preserves sender name", func(t *testing.T) {
		// Step 1: Frontend creates the payload (simulated)
		frontendPayload := `{
			"workspace_id": "workspace-123",
			"to": "test@example.com",
			"provider": {
				"kind": "smtp",
				"rate_limit_per_minute": 25,
				"senders": [
					{
						"id": "sender-abc",
						"email": "hello@notifuse.com",
						"name": "hello",
						"is_default": true
					}
				],
				"smtp": {
					"host": "smtp.example.com",
					"port": 587,
					"username": "user@example.com",
					"password": "password",
					"use_tls": true
				}
			}
		}`

		t.Logf("Frontend sends: %s", frontendPayload)

		// Step 2: Backend receives and deserializes
		var req domain.TestEmailProviderRequest
		err := json.NewDecoder(bytes.NewReader([]byte(frontendPayload))).Decode(&req)
		require.NoError(t, err)

		// Step 3: Verify deserialization
		require.Len(t, req.Provider.Senders, 1)
		sender := req.Provider.Senders[0]

		t.Logf("✅ Backend received sender:")
		t.Logf("   ID: %s", sender.ID)
		t.Logf("   Email: %s", sender.Email)
		t.Logf("   Name: '%s'", sender.Name)
		t.Logf("   IsDefault: %v", sender.IsDefault)

		assert.Equal(t, "hello", sender.Name, "Sender name 'hello' should be preserved")
		assert.Equal(t, "hello@notifuse.com", sender.Email)
		assert.True(t, sender.IsDefault)

		// Step 4: Verify it would be used correctly in SendEmailProviderRequest
		emailRequest := domain.SendEmailProviderRequest{
			WorkspaceID:   req.WorkspaceID,
			IntegrationID: "test-integration",
			MessageID:     "msg-123",
			FromAddress:   sender.Email,
			FromName:      sender.Name, // This should be "hello"
			To:            req.To,
			Subject:       "Test",
			Content:       "Test",
			Provider:      &req.Provider,
		}

		assert.Equal(t, "hello", emailRequest.FromName,
			"FromName in SendEmailProviderRequest should be 'hello'")

		t.Logf("✅ Would create SendEmailProviderRequest with FromName: '%s'", emailRequest.FromName)
	})
}
