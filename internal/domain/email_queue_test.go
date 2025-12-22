package domain

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEmailQueueStatus_Values(t *testing.T) {
	tests := []struct {
		name     string
		status   EmailQueueStatus
		expected string
	}{
		{
			name:     "pending status",
			status:   EmailQueueStatusPending,
			expected: "pending",
		},
		{
			name:     "processing status",
			status:   EmailQueueStatusProcessing,
			expected: "processing",
		},
		{
			name:     "sent status",
			status:   EmailQueueStatusSent,
			expected: "sent",
		},
		{
			name:     "failed status",
			status:   EmailQueueStatusFailed,
			expected: "failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, string(tt.status))
		})
	}
}

func TestEmailQueueSourceType_Values(t *testing.T) {
	tests := []struct {
		name       string
		sourceType EmailQueueSourceType
		expected   string
	}{
		{
			name:       "broadcast source",
			sourceType: EmailQueueSourceBroadcast,
			expected:   "broadcast",
		},
		{
			name:       "automation source",
			sourceType: EmailQueueSourceAutomation,
			expected:   "automation",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, string(tt.sourceType))
		})
	}
}

func TestEmailQueuePriorityMarketing(t *testing.T) {
	assert.Equal(t, 5, EmailQueuePriorityMarketing)
}

func TestCalculateNextRetryTime(t *testing.T) {
	tests := []struct {
		name            string
		attempts        int
		expectedMinutes int
	}{
		{
			name:            "zero attempts defaults to 1 minute",
			attempts:        0,
			expectedMinutes: 1,
		},
		{
			name:            "negative attempts defaults to 1 minute",
			attempts:        -1,
			expectedMinutes: 1,
		},
		{
			name:            "first attempt - 1 minute backoff",
			attempts:        1,
			expectedMinutes: 1,
		},
		{
			name:            "second attempt - 2 minutes backoff",
			attempts:        2,
			expectedMinutes: 2,
		},
		{
			name:            "third attempt - 4 minutes backoff",
			attempts:        3,
			expectedMinutes: 4,
		},
		{
			name:            "fourth attempt - 8 minutes backoff",
			attempts:        4,
			expectedMinutes: 8,
		},
		{
			name:            "fifth attempt - 16 minutes backoff",
			attempts:        5,
			expectedMinutes: 16,
		},
		{
			name:            "tenth attempt - 512 minutes backoff",
			attempts:        10,
			expectedMinutes: 512,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := time.Now().UTC()
			result := CalculateNextRetryTime(tt.attempts)
			after := time.Now().UTC()

			expectedDuration := time.Duration(tt.expectedMinutes) * time.Minute

			// Result should be between before+expectedDuration and after+expectedDuration
			// Allow a small margin for test execution time
			minExpected := before.Add(expectedDuration)
			maxExpected := after.Add(expectedDuration).Add(time.Second) // 1 second margin

			assert.True(t, result.After(minExpected) || result.Equal(minExpected),
				"result %v should be >= %v", result, minExpected)
			assert.True(t, result.Before(maxExpected) || result.Equal(maxExpected),
				"result %v should be <= %v", result, maxExpected)
		})
	}
}

func TestEmailQueuePayload_ToSendEmailProviderRequest(t *testing.T) {
	t.Run("converts all fields correctly", func(t *testing.T) {
		payload := EmailQueuePayload{
			FromAddress:        "sender@example.com",
			FromName:           "Test Sender",
			Subject:            "Test Subject",
			HTMLContent:        "<html><body>Test</body></html>",
			RateLimitPerMinute: 100,
			EmailOptions: EmailOptions{
				ListUnsubscribeURL: "https://example.com/unsubscribe",
			},
		}

		provider := &EmailProvider{
			Kind: EmailProviderKindSMTP,
			SMTP: &SMTPSettings{
				Host: "smtp.example.com",
				Port: 587,
			},
		}

		result := payload.ToSendEmailProviderRequest(
			"workspace123",
			"integration456",
			"message789",
			"recipient@example.com",
			provider,
		)

		require.NotNil(t, result)
		assert.Equal(t, "workspace123", result.WorkspaceID)
		assert.Equal(t, "integration456", result.IntegrationID)
		assert.Equal(t, "message789", result.MessageID)
		assert.Equal(t, "sender@example.com", result.FromAddress)
		assert.Equal(t, "Test Sender", result.FromName)
		assert.Equal(t, "recipient@example.com", result.To)
		assert.Equal(t, "Test Subject", result.Subject)
		assert.Equal(t, "<html><body>Test</body></html>", result.Content)
		assert.Equal(t, provider, result.Provider)
		assert.Equal(t, "https://example.com/unsubscribe", result.EmailOptions.ListUnsubscribeURL)
	})

	t.Run("handles nil provider", func(t *testing.T) {
		payload := EmailQueuePayload{
			FromAddress: "sender@example.com",
			FromName:    "Test Sender",
			Subject:     "Test Subject",
			HTMLContent: "<html><body>Test</body></html>",
		}

		result := payload.ToSendEmailProviderRequest(
			"workspace123",
			"integration456",
			"message789",
			"recipient@example.com",
			nil,
		)

		require.NotNil(t, result)
		assert.Nil(t, result.Provider)
		assert.Equal(t, "workspace123", result.WorkspaceID)
		assert.Equal(t, "recipient@example.com", result.To)
	})

	t.Run("handles empty payload fields", func(t *testing.T) {
		payload := EmailQueuePayload{}

		provider := &EmailProvider{
			Kind: EmailProviderKindSES,
		}

		result := payload.ToSendEmailProviderRequest(
			"workspace123",
			"integration456",
			"message789",
			"recipient@example.com",
			provider,
		)

		require.NotNil(t, result)
		assert.Equal(t, "", result.FromAddress)
		assert.Equal(t, "", result.FromName)
		assert.Equal(t, "", result.Subject)
		assert.Equal(t, "", result.Content)
	})
}

func TestEmailQueueEntry_DefaultValues(t *testing.T) {
	entry := EmailQueueEntry{}

	// Verify zero values for optional fields
	assert.Equal(t, "", entry.ID)
	assert.Equal(t, EmailQueueStatus(""), entry.Status)
	assert.Equal(t, 0, entry.Priority)
	assert.Equal(t, 0, entry.Attempts)
	assert.Equal(t, 0, entry.MaxAttempts)
	assert.Nil(t, entry.LastError)
	assert.Nil(t, entry.NextRetryAt)
	assert.Nil(t, entry.ProcessedAt)
}

func TestEmailQueueStats_DefaultValues(t *testing.T) {
	stats := EmailQueueStats{}

	assert.Equal(t, int64(0), stats.Pending)
	assert.Equal(t, int64(0), stats.Processing)
	assert.Equal(t, int64(0), stats.Sent)
	assert.Equal(t, int64(0), stats.Failed)
	assert.Equal(t, int64(0), stats.DeadLetter)
}

func TestEmailQueueDeadLetter_DefaultValues(t *testing.T) {
	deadLetter := EmailQueueDeadLetter{}

	assert.Equal(t, "", deadLetter.ID)
	assert.Equal(t, "", deadLetter.OriginalEntryID)
	assert.Equal(t, EmailQueueSourceType(""), deadLetter.SourceType)
	assert.Equal(t, "", deadLetter.FinalError)
	assert.Equal(t, 0, deadLetter.Attempts)
}
