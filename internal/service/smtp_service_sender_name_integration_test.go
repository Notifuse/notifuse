package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEmailSenderNamePreservation tests that the sender name is preserved
// through the entire flow: JSON serialization -> GetSender -> SendEmailProviderRequest
func TestEmailSenderNamePreservation(t *testing.T) {
	t.Run("Sender name is preserved in JSON serialization", func(t *testing.T) {
		// Create an email provider with a sender that has a name
		sender := domain.NewEmailSender("test@example.com", "Notifuse")
		emailProvider := &domain.EmailProvider{
			Kind:               domain.EmailProviderKindSMTP,
			RateLimitPerMinute: 25,
			Senders:            []domain.EmailSender{sender},
			SMTP: &domain.SMTPSettings{
				Host:     "smtp.example.com",
				Port:     587,
				Username: "user@example.com",
				Password: "password",
				UseTLS:   true,
			},
		}

		// Verify the sender has a name before serialization
		assert.Equal(t, "Notifuse", emailProvider.Senders[0].Name)
		assert.Equal(t, "test@example.com", emailProvider.Senders[0].Email)

		// Serialize to JSON (simulating database storage)
		jsonData, err := json.Marshal(emailProvider)
		require.NoError(t, err)
		
		// Verify the JSON contains the name field
		jsonStr := string(jsonData)
		assert.Contains(t, jsonStr, `"name":"Notifuse"`)
		assert.Contains(t, jsonStr, `"email":"test@example.com"`)

		// Deserialize from JSON (simulating database retrieval)
		var retrievedProvider domain.EmailProvider
		err = json.Unmarshal(jsonData, &retrievedProvider)
		require.NoError(t, err)

		// Verify the sender name is preserved after deserialization
		require.Len(t, retrievedProvider.Senders, 1)
		assert.Equal(t, "Notifuse", retrievedProvider.Senders[0].Name, 
			"Sender name should be preserved after JSON round-trip")
		assert.Equal(t, "test@example.com", retrievedProvider.Senders[0].Email)
	})

	t.Run("GetSender returns sender with name", func(t *testing.T) {
		// Create provider with named sender
		sender := domain.NewEmailSender("hello@notifuse.com", "Notifuse Platform")
		emailProvider := &domain.EmailProvider{
			Kind:    domain.EmailProviderKindSMTP,
			Senders: []domain.EmailSender{sender},
		}

		// Get the sender by ID
		retrievedSender := emailProvider.GetSender(sender.ID)
		require.NotNil(t, retrievedSender)

		// Verify the name is present
		assert.Equal(t, "Notifuse Platform", retrievedSender.Name,
			"GetSender should return sender with name intact")
		assert.Equal(t, "hello@notifuse.com", retrievedSender.Email)
	})

	t.Run("GetSender returns default sender with name", func(t *testing.T) {
		// Create provider with multiple senders
		defaultSender := domain.EmailSender{
			ID:        "default-id",
			Email:     "default@notifuse.com",
			Name:      "Notifuse Default",
			IsDefault: true,
		}
		otherSender := domain.EmailSender{
			ID:        "other-id",
			Email:     "other@notifuse.com",
			Name:      "Notifuse Other",
			IsDefault: false,
		}
		emailProvider := &domain.EmailProvider{
			Kind:    domain.EmailProviderKindSMTP,
			Senders: []domain.EmailSender{defaultSender, otherSender},
		}

		// Get default sender by passing empty ID
		retrievedSender := emailProvider.GetSender("")
		require.NotNil(t, retrievedSender)

		// Verify the default sender's name is returned
		assert.Equal(t, "Notifuse Default", retrievedSender.Name,
			"GetSender with empty ID should return default sender with name")
		assert.Equal(t, "default@notifuse.com", retrievedSender.Email)
		assert.True(t, retrievedSender.IsDefault)
	})

	t.Run("SendEmailProviderRequest validation requires FromName", func(t *testing.T) {
		provider := &domain.EmailProvider{
			Kind: domain.EmailProviderKindSES,
			SES: &domain.AmazonSESSettings{
				Region:    "us-east-1",
				AccessKey: "test-access",
				SecretKey: "test-secret",
			},
		}

		// Create request without FromName
		requestWithoutName := domain.SendEmailProviderRequest{
			WorkspaceID:   "workspace-123",
			IntegrationID: "integration-123",
			MessageID:     "message-123",
			FromAddress:   "test@example.com",
			FromName:      "", // Empty name
			To:            "recipient@example.com",
			Subject:       "Test",
			Content:       "Content",
			Provider:      provider,
		}

		// Validation should fail for empty FromName
		err := requestWithoutName.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "from name is required")

		// Create request with FromName
		requestWithName := domain.SendEmailProviderRequest{
			WorkspaceID:   "workspace-123",
			IntegrationID: "integration-123",
			MessageID:     "message-123",
			FromAddress:   "test@example.com",
			FromName:      "Notifuse", // With name
			To:            "recipient@example.com",
			Subject:       "Test",
			Content:       "Content",
			Provider:      provider,
		}

		// Validation should pass
		err = requestWithName.Validate()
		assert.NoError(t, err)
	})
}

// TestIntegrationSenderNameFlow tests the complete flow of sender name
// from Integration -> EmailProvider -> GetSender -> SendEmailProviderRequest
func TestIntegrationSenderNameFlow(t *testing.T) {
	t.Run("Complete flow preserves sender name", func(t *testing.T) {
		// Step 1: Create an integration with a sender
		sender := domain.NewEmailSender("noreply@notifuse.com", "Notifuse Platform")
		emailProvider := domain.EmailProvider{
			Kind:               domain.EmailProviderKindSMTP,
			RateLimitPerMinute: 25,
			Senders:            []domain.EmailSender{sender},
			SMTP: &domain.SMTPSettings{
				Host:     "smtp.example.com",
				Port:     587,
				Username: "user@example.com",
				Password: "password",
				UseTLS:   true,
			},
		}

		integration := domain.Integration{
			ID:            "int-123",
			Name:          "SMTP Integration",
			Type:          domain.IntegrationTypeEmail,
			EmailProvider: emailProvider,
		}

		// Step 2: Serialize integration to JSON (simulating database storage)
		integrationJSON, err := json.Marshal(integration)
		require.NoError(t, err)

		// Verify name is in JSON
		assert.Contains(t, string(integrationJSON), `"name":"Notifuse Platform"`)

		// Step 3: Deserialize from JSON (simulating database retrieval)
		var retrievedIntegration domain.Integration
		err = json.Unmarshal(integrationJSON, &retrievedIntegration)
		require.NoError(t, err)

		// Step 4: Get sender from email provider
		retrievedSender := retrievedIntegration.EmailProvider.GetSender(sender.ID)
		require.NotNil(t, retrievedSender)

		// Step 5: Verify sender name is preserved
		assert.Equal(t, "Notifuse Platform", retrievedSender.Name,
			"Sender name should be preserved through the entire flow")
		assert.Equal(t, "noreply@notifuse.com", retrievedSender.Email)

		// Step 6: Create SendEmailProviderRequest using the retrieved sender
		request := domain.SendEmailProviderRequest{
			WorkspaceID:   "workspace-123",
			IntegrationID: integration.ID,
			MessageID:     "message-123",
			FromAddress:   retrievedSender.Email,
			FromName:      retrievedSender.Name, // This is the critical field
			To:            "user@example.com",
			Subject:       "Test Email",
			Content:       "<h1>Test</h1>",
			Provider:      &retrievedIntegration.EmailProvider,
		}

		// Step 7: Validate the request
		err = request.Validate()
		assert.NoError(t, err)

		// Step 8: Verify FromName is set correctly
		assert.Equal(t, "Notifuse Platform", request.FromName,
			"FromName should match the sender's name")
		assert.Equal(t, "noreply@notifuse.com", request.FromAddress)
	})
}

// TestEmailProviderValidationWithEmptySenderName tests that validation
// catches empty sender names
func TestEmailProviderValidationWithEmptySenderName(t *testing.T) {
	t.Run("EmailProvider validation fails for empty sender name", func(t *testing.T) {
		// Create sender with empty name
		provider := &domain.EmailProvider{
			Kind:               domain.EmailProviderKindSMTP,
			RateLimitPerMinute: 25,
			Senders: []domain.EmailSender{
				{
					ID:    "sender-123",
					Email: "test@example.com",
					Name:  "", // Empty name - should fail validation
				},
			},
			SMTP: &domain.SMTPSettings{
				Host:     "smtp.example.com",
				Port:     587,
				Username: "user",
				Password: "pass",
				UseTLS:   true,
			},
		}

		// Validation should fail
		err := provider.Validate("passphrase")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "sender name is required")
	})

	t.Run("EmailProvider validation passes with sender name", func(t *testing.T) {
		// Create sender with name
		provider := &domain.EmailProvider{
			Kind:               domain.EmailProviderKindSMTP,
			RateLimitPerMinute: 25,
			Senders: []domain.EmailSender{
				domain.NewEmailSender("test@example.com", "Test Sender"),
			},
			SMTP: &domain.SMTPSettings{
				Host:     "smtp.example.com",
				Port:     587,
				Username: "user",
				Password: "pass",
				UseTLS:   true,
			},
		}

		// Validation should pass
		err := provider.Validate("passphrase")
		assert.NoError(t, err)
	})
}

// TestDiagnoseEmptySenderName provides diagnostic information about
// where sender name might be lost
func TestDiagnoseEmptySenderName(t *testing.T) {
	t.Run("Diagnose: Empty string vs nil sender", func(t *testing.T) {
		ctx := context.Background()

		// Case 1: Sender with empty name (should fail validation)
		senderWithEmptyName := domain.EmailSender{
			ID:    "sender-1",
			Email: "test@example.com",
			Name:  "", // Explicitly empty
		}

		providerWithEmptyName := &domain.EmailProvider{
			Kind:               domain.EmailProviderKindSMTP,
			RateLimitPerMinute: 25,
			Senders:            []domain.EmailSender{senderWithEmptyName},
			SMTP: &domain.SMTPSettings{
				Host: "smtp.example.com", Port: 587,
				Username: "user", Password: "pass", UseTLS: true,
			},
		}

		// This should fail validation
		err := providerWithEmptyName.Validate("passphrase")
		assert.Error(t, err, "Provider with empty sender name should fail validation")

		// Case 2: GetSender on empty senders list
		emptyProvider := &domain.EmailProvider{
			Kind:    domain.EmailProviderKindSMTP,
			Senders: []domain.EmailSender{}, // No senders
		}

		sender := emptyProvider.GetSender("")
		assert.Nil(t, sender, "GetSender should return nil when no senders exist")

		// Case 3: Proper sender with name
		properSender := domain.NewEmailSender("test@example.com", "Proper Name")
		properProvider := &domain.EmailProvider{
			Kind:    domain.EmailProviderKindSMTP,
			Senders: []domain.EmailSender{properSender},
		}

		retrievedSender := properProvider.GetSender(properSender.ID)
		require.NotNil(t, retrievedSender)
		assert.Equal(t, "Proper Name", retrievedSender.Name)
		assert.NotEmpty(t, retrievedSender.Name, "Sender name should not be empty")

		t.Logf("Context: %v", ctx) // Use ctx to avoid unused variable error
	})
}
