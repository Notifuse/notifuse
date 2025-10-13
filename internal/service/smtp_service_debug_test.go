package service

import (
	"context"
	"testing"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wneessen/go-mail"
)

// TestDebugFromNameInActualEmail is a debugging test that simulates
// the exact flow from database retrieval to email sending
func TestDebugFromNameInActualEmail(t *testing.T) {
	t.Run("Debug: Full flow with actual go-mail message", func(t *testing.T) {
		// Step 1: Simulate database data (what would be stored)
		sender := domain.EmailSender{
			ID:        "sender-123",
			Email:     "noreply@notifuse.com",
			Name:      "Notifuse", // This is what should be in the database
			IsDefault: true,
		}

		emailProvider := &domain.EmailProvider{
			Kind:    domain.EmailProviderKindSMTP,
			Senders: []domain.EmailSender{sender},
			SMTP: &domain.SMTPSettings{
				Host:     "smtp.example.com",
				Port:     587,
				Username: "user",
				Password: "pass",
				UseTLS:   true,
			},
		}

		// Step 2: Get sender (simulating what services do)
		retrievedSender := emailProvider.GetSender(sender.ID)
		require.NotNil(t, retrievedSender)

		// CHECKPOINT 1: Verify sender has name after retrieval
		t.Logf("CHECKPOINT 1 - Retrieved Sender Name: '%s'", retrievedSender.Name)
		assert.Equal(t, "Notifuse", retrievedSender.Name,
			"Sender name should be 'Notifuse' after retrieval")

		// Step 3: Create SendEmailProviderRequest (what email service does)
		request := domain.SendEmailProviderRequest{
			WorkspaceID:   "workspace-123",
			IntegrationID: "integration-123",
			MessageID:     "message-123",
			FromAddress:   retrievedSender.Email,
			FromName:      retrievedSender.Name, // CRITICAL: This should be "Notifuse"
			To:            "user@example.com",
			Subject:       "Test Email",
			Content:       "<h1>Test</h1>",
			Provider:      emailProvider,
		}

		// CHECKPOINT 2: Verify request has FromName
		t.Logf("CHECKPOINT 2 - Request FromName: '%s'", request.FromName)
		assert.Equal(t, "Notifuse", request.FromName,
			"Request FromName should be 'Notifuse'")

		// Step 4: Create go-mail message (what SMTP service does)
		msg := mail.NewMsg()
		err := msg.FromFormat(request.FromName, request.FromAddress)
		require.NoError(t, err)

		// Step 5: Verify the From header in the message
		from := msg.GetFrom()
		require.Len(t, from, 1)

		// CHECKPOINT 3: Verify go-mail has the name
		t.Logf("CHECKPOINT 3 - Go-Mail From Name: '%s'", from[0].Name)
		t.Logf("CHECKPOINT 3 - Go-Mail From Email: '%s'", from[0].Address)
		t.Logf("CHECKPOINT 3 - Go-Mail From String: '%s'", from[0].String())

		assert.Equal(t, "Notifuse", from[0].Name,
			"Go-Mail message should have 'Notifuse' as From name")
		assert.Equal(t, "noreply@notifuse.com", from[0].Address)
		assert.Equal(t, `"Notifuse" <noreply@notifuse.com>`, from[0].String(),
			"Go-Mail should format From as '\"Notifuse\" <noreply@notifuse.com>'")
	})

	t.Run("Debug: What happens when FromName is empty string", func(t *testing.T) {
		// Simulate the problematic scenario where FromName is empty
		sender := domain.EmailSender{
			ID:        "sender-123",
			Email:     "noreply@notifuse.com",
			Name:      "", // EMPTY NAME - This is the problem!
			IsDefault: true,
		}

		emailProvider := &domain.EmailProvider{
			Kind:    domain.EmailProviderKindSMTP,
			Senders: []domain.EmailSender{sender},
		}

		retrievedSender := emailProvider.GetSender(sender.ID)
		require.NotNil(t, retrievedSender)

		t.Logf("PROBLEM CASE - Sender Name: '%s' (empty)", retrievedSender.Name)

		// If FromName is empty, validation should fail
		request := domain.SendEmailProviderRequest{
			WorkspaceID:   "workspace-123",
			IntegrationID: "integration-123",
			MessageID:     "message-123",
			FromAddress:   retrievedSender.Email,
			FromName:      retrievedSender.Name, // Empty string
			To:            "user@example.com",
			Subject:       "Test",
			Content:       "Content",
			Provider:      emailProvider,
		}

		err := request.Validate()
		t.Logf("PROBLEM CASE - Validation error (expected): %v", err)
		assert.Error(t, err, "Validation should fail when FromName is empty")
		assert.Contains(t, err.Error(), "from name is required")

		// But if someone bypasses validation and creates the message anyway...
		msg := mail.NewMsg()
		_ = msg.FromFormat(retrievedSender.Name, retrievedSender.Email)

		from := msg.GetFrom()
		if len(from) > 0 {
			t.Logf("PROBLEM CASE - Go-Mail From String: '%s'", from[0].String())
			// With empty name, go-mail outputs: <email@example.com>
			assert.Equal(t, "<noreply@notifuse.com>", from[0].String(),
				"With empty name, From should be '<email>' without name")
		}
	})

	t.Run("Debug: Check if GetSender might return wrong sender", func(t *testing.T) {
		// Test edge case: multiple senders, wrong one selected
		defaultSender := domain.EmailSender{
			ID:        "default-id",
			Email:     "default@notifuse.com",
			Name:      "Default Sender",
			IsDefault: true,
		}

		emptySender := domain.EmailSender{
			ID:        "empty-id",
			Email:     "empty@notifuse.com",
			Name:      "", // No name
			IsDefault: false,
		}

		emailProvider := &domain.EmailProvider{
			Kind:    domain.EmailProviderKindSMTP,
			Senders: []domain.EmailSender{defaultSender, emptySender},
		}

		// Get default sender - should have name
		sender1 := emailProvider.GetSender("")
		require.NotNil(t, sender1)
		assert.Equal(t, "Default Sender", sender1.Name)
		t.Logf("Default sender has name: '%s'", sender1.Name)

		// Get empty sender by ID - will have empty name
		sender2 := emailProvider.GetSender("empty-id")
		require.NotNil(t, sender2)
		assert.Equal(t, "", sender2.Name)
		t.Logf("Empty sender has name: '%s' (empty)", sender2.Name)
	})
}

// TestRealWorldScenarios tests real-world scenarios that could cause missing From names
func TestRealWorldScenarios(t *testing.T) {
	t.Run("Scenario 1: Sender created without name in database", func(t *testing.T) {
		// This would fail EmailProvider.Validate() but let's test it anyway
		sender := domain.EmailSender{
			ID:    "sender-123",
			Email: "test@example.com",
			Name:  "", // Empty - should not happen with proper validation
		}

		provider := &domain.EmailProvider{
			Kind:               domain.EmailProviderKindSMTP,
			RateLimitPerMinute: 25,
			Senders:            []domain.EmailSender{sender},
			SMTP: &domain.SMTPSettings{
				Host: "smtp.example.com", Port: 587,
				Username: "user", Password: "pass", UseTLS: true,
			},
		}

		// This should fail validation
		err := provider.Validate("passphrase")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "sender name is required")

		t.Log("✅ EmailProvider validation correctly catches empty sender name")
	})

	t.Run("Scenario 2: Template references wrong sender ID", func(t *testing.T) {
		correctSender := domain.NewEmailSender("correct@example.com", "Correct Sender")
		wrongSender := domain.NewEmailSender("wrong@example.com", "")

		provider := &domain.EmailProvider{
			Kind:    domain.EmailProviderKindSMTP,
			Senders: []domain.EmailSender{correctSender, wrongSender},
		}

		// If template references the wrong sender ID...
		retrievedSender := provider.GetSender(wrongSender.ID)
		require.NotNil(t, retrievedSender)

		if retrievedSender.Name == "" {
			t.Log("⚠️ Template is using sender with empty name!")
			t.Logf("   Sender ID: %s, Email: %s, Name: '%s'",
				retrievedSender.ID, retrievedSender.Email, retrievedSender.Name)
		}
	})

	t.Run("Scenario 3: Verify validation prevents empty names", func(t *testing.T) {
		ctx := context.Background()

		// This is what should happen: validation prevents empty names
		badRequest := domain.SendEmailProviderRequest{
			WorkspaceID:   "workspace-123",
			IntegrationID: "integration-123",
			MessageID:     "message-123",
			FromAddress:   "test@example.com",
			FromName:      "", // Empty
			To:            "recipient@example.com",
			Subject:       "Test",
			Content:       "Content",
			Provider: &domain.EmailProvider{
				Kind: domain.EmailProviderKindSMTP,
				SMTP: &domain.SMTPSettings{
					Host: "smtp.example.com", Port: 587,
					Username: "user", Password: "pass", UseTLS: true,
				},
			},
		}

		err := badRequest.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "from name is required")

		t.Log("✅ SendEmailProviderRequest validation correctly requires FromName")
		t.Logf("   Context: %v", ctx) // Use ctx to avoid unused variable error
	})
}
