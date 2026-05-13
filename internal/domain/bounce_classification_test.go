package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestClassifyBounce(t *testing.T) {
	testCases := []struct {
		name     string
		in       BounceInput
		expected BounceClassification
	}{
		// ---- SES: Permanent → Hard ----
		{
			name:     "SES Permanent/General",
			in:       BounceInput{Provider: EmailProviderKindSES, Type: "Permanent", Subtype: "General"},
			expected: BounceClassificationHard,
		},
		{
			name:     "SES Permanent/NoEmail",
			in:       BounceInput{Provider: EmailProviderKindSES, Type: "Permanent", Subtype: "NoEmail"},
			expected: BounceClassificationHard,
		},
		{
			name:     "SES Permanent/Suppressed",
			in:       BounceInput{Provider: EmailProviderKindSES, Type: "Permanent", Subtype: "Suppressed"},
			expected: BounceClassificationHard,
		},
		{
			name:     "SES Permanent/OnAccountSuppressionList",
			in:       BounceInput{Provider: EmailProviderKindSES, Type: "Permanent", Subtype: "OnAccountSuppressionList"},
			expected: BounceClassificationHard,
		},
		{
			name:     "SES Permanent/UnsubscribedRecipient",
			in:       BounceInput{Provider: EmailProviderKindSES, Type: "Permanent", Subtype: "UnsubscribedRecipient"},
			expected: BounceClassificationHard,
		},

		// ---- SES: Transient — SoftCount ----
		{
			name:     "SES Transient/General no diagnostic → SoftCount",
			in:       BounceInput{Provider: EmailProviderKindSES, Type: "Transient", Subtype: "General"},
			expected: BounceClassificationSoftCount,
		},
		{
			name:     "SES Transient/MailboxFull",
			in:       BounceInput{Provider: EmailProviderKindSES, Type: "Transient", Subtype: "MailboxFull"},
			expected: BounceClassificationSoftCount,
		},
		{
			name:     "SES Undetermined",
			in:       BounceInput{Provider: EmailProviderKindSES, Type: "Undetermined", Subtype: "Undetermined"},
			expected: BounceClassificationSoftCount,
		},

		// ---- SES: Transient — SoftIgnore ----
		{
			name:     "SES Transient/MessageTooLarge",
			in:       BounceInput{Provider: EmailProviderKindSES, Type: "Transient", Subtype: "MessageTooLarge"},
			expected: BounceClassificationSoftIgnore,
		},
		{
			name:     "SES Transient/ContentRejected",
			in:       BounceInput{Provider: EmailProviderKindSES, Type: "Transient", Subtype: "ContentRejected"},
			expected: BounceClassificationSoftIgnore,
		},
		{
			name:     "SES Transient/AttachmentRejected",
			in:       BounceInput{Provider: EmailProviderKindSES, Type: "Transient", Subtype: "AttachmentRejected"},
			expected: BounceClassificationSoftIgnore,
		},

		// ---- SES: Transient/General with retry-exhausted diagnostics → Hard ----
		{
			name: "SES Transient/General with SMTP 4.4.7 → Hard",
			in: BounceInput{
				Provider:   EmailProviderKindSES,
				Type:       "Transient",
				Subtype:    "General",
				Diagnostic: "smtp; 550 4.4.7 Message expired: unable to deliver in 840 minutes. <421 4.4.1 Failed to establish connection>",
			},
			expected: BounceClassificationHard,
		},
		{
			name: "SES Transient/General with 'Message expired' only → Hard",
			in: BounceInput{
				Provider:   EmailProviderKindSES,
				Type:       "Transient",
				Subtype:    "General",
				Diagnostic: "Message expired: unable to deliver after retries",
			},
			expected: BounceClassificationHard,
		},
		{
			name: "SES Transient/General with lowercase '550 4.4.7 message expired' → Hard",
			in: BounceInput{
				Provider:   EmailProviderKindSES,
				Type:       "Transient",
				Subtype:    "General",
				Diagnostic: "550 4.4.7 message expired",
			},
			expected: BounceClassificationHard,
		},
		{
			name: "SES Transient/General mixed case 'MESSAGE EXPIRED' → Hard",
			in: BounceInput{
				Provider:   EmailProviderKindSES,
				Type:       "Transient",
				Subtype:    "General",
				Diagnostic: "smtp; 421 4.0.0 MESSAGE EXPIRED",
			},
			expected: BounceClassificationHard,
		},
		{
			name: "SES Transient/General with unrelated 4.4 code → SoftCount",
			in: BounceInput{
				Provider:   EmailProviderKindSES,
				Type:       "Transient",
				Subtype:    "General",
				Diagnostic: "smtp; 421 4.4.1 connection refused",
			},
			expected: BounceClassificationSoftCount,
		},
		{
			name: "SES Transient/MailboxFull retry-exhaustion diagnostic does NOT promote",
			in: BounceInput{
				Provider:   EmailProviderKindSES,
				Type:       "Transient",
				Subtype:    "MailboxFull",
				Diagnostic: "smtp; 550 4.4.7 Message expired",
			},
			expected: BounceClassificationSoftCount,
		},

		// ---- SES: case-insensitive type/subtype ----
		{
			name:     "SES case-insensitive permanent/general",
			in:       BounceInput{Provider: EmailProviderKindSES, Type: "permanent", Subtype: "general"},
			expected: BounceClassificationHard,
		},
		{
			name:     "SES case-insensitive TRANSIENT/MESSAGETOOLARGE",
			in:       BounceInput{Provider: EmailProviderKindSES, Type: "TRANSIENT", Subtype: "MESSAGETOOLARGE"},
			expected: BounceClassificationSoftIgnore,
		},

		// ---- Mailgun ----
		{
			name:     "Mailgun hardbounce",
			in:       BounceInput{Provider: EmailProviderKindMailgun, Type: "hardbounce"},
			expected: BounceClassificationHard,
		},
		{
			name:     "Mailgun permanent",
			in:       BounceInput{Provider: EmailProviderKindMailgun, Subtype: "permanent"},
			expected: BounceClassificationHard,
		},
		{
			name:     "Mailgun softbounce",
			in:       BounceInput{Provider: EmailProviderKindMailgun, Type: "softbounce"},
			expected: BounceClassificationSoftCount,
		},
		{
			name:     "Mailgun temporary category",
			in:       BounceInput{Provider: EmailProviderKindMailgun, Subtype: "temporary"},
			expected: BounceClassificationSoftCount,
		},

		// ---- Mailjet ----
		{
			name:     "Mailjet hardbounce",
			in:       BounceInput{Provider: EmailProviderKindMailjet, Type: "hardbounce"},
			expected: BounceClassificationHard,
		},
		{
			name:     "Mailjet softbounce",
			in:       BounceInput{Provider: EmailProviderKindMailjet, Type: "softbounce"},
			expected: BounceClassificationSoftCount,
		},

		// ---- Postmark ----
		{
			name:     "Postmark HardBounce",
			in:       BounceInput{Provider: EmailProviderKindPostmark, Type: "HardBounce"},
			expected: BounceClassificationHard,
		},
		{
			name:     "Postmark Blocked",
			in:       BounceInput{Provider: EmailProviderKindPostmark, Type: "Blocked"},
			expected: BounceClassificationHard,
		},
		{
			name:     "Postmark SoftBounce",
			in:       BounceInput{Provider: EmailProviderKindPostmark, Type: "SoftBounce"},
			expected: BounceClassificationSoftCount,
		},
		{
			name:     "Postmark MessageContent → SoftIgnore",
			in:       BounceInput{Provider: EmailProviderKindPostmark, Type: "MessageContent"},
			expected: BounceClassificationSoftIgnore,
		},
		{
			name:     "Postmark RuleBlocked → SoftIgnore",
			in:       BounceInput{Provider: EmailProviderKindPostmark, Type: "RuleBlocked"},
			expected: BounceClassificationSoftIgnore,
		},

		// ---- SparkPost ----
		{
			name:     "SparkPost class 10 (Invalid Recipient)",
			in:       BounceInput{Provider: EmailProviderKindSparkPost, Subtype: "10"},
			expected: BounceClassificationHard,
		},
		{
			name:     "SparkPost class 30 (No RCPT)",
			in:       BounceInput{Provider: EmailProviderKindSparkPost, Subtype: "30"},
			expected: BounceClassificationHard,
		},
		{
			name:     "SparkPost class 90 (Unsubscribe)",
			in:       BounceInput{Provider: EmailProviderKindSparkPost, Subtype: "90"},
			expected: BounceClassificationHard,
		},
		{
			name:     "SparkPost class 20 (Soft)",
			in:       BounceInput{Provider: EmailProviderKindSparkPost, Subtype: "20"},
			expected: BounceClassificationSoftCount,
		},
		{
			name:     "SparkPost class 50 (Block — treated as soft)",
			in:       BounceInput{Provider: EmailProviderKindSparkPost, Subtype: "50"},
			expected: BounceClassificationSoftCount,
		},

		// ---- SendGrid ----
		{
			name:     "SendGrid type=bounce → Hard",
			in:       BounceInput{Provider: EmailProviderKindSendGrid, Type: "bounce"},
			expected: BounceClassificationHard,
		},
		{
			name:     "SendGrid Invalid Address category → Hard",
			in:       BounceInput{Provider: EmailProviderKindSendGrid, Subtype: "Invalid Address"},
			expected: BounceClassificationHard,
		},
		{
			name:     "SendGrid blocked → SoftCount",
			in:       BounceInput{Provider: EmailProviderKindSendGrid, Type: "blocked"},
			expected: BounceClassificationSoftCount,
		},
		{
			name:     "SendGrid dropped → SoftCount",
			in:       BounceInput{Provider: EmailProviderKindSendGrid, Type: "dropped"},
			expected: BounceClassificationSoftCount,
		},
		{
			name:     "SendGrid technical → SoftCount",
			in:       BounceInput{Provider: EmailProviderKindSendGrid, Subtype: "technical"},
			expected: BounceClassificationSoftCount,
		},

		// ---- SMTP ----
		{
			name:     "SMTP hardbounce",
			in:       BounceInput{Provider: EmailProviderKindSMTP, Type: "hardbounce"},
			expected: BounceClassificationHard,
		},
		{
			name:     "SMTP softbounce",
			in:       BounceInput{Provider: EmailProviderKindSMTP, Type: "softbounce"},
			expected: BounceClassificationSoftCount,
		},

		// ---- Empty / unknown defaults ----
		{
			name:     "Empty input defaults to SoftCount",
			in:       BounceInput{Provider: EmailProviderKindSES},
			expected: BounceClassificationSoftCount,
		},
		{
			name:     "Unknown provider defaults to SoftCount",
			in:       BounceInput{Provider: "made-up", Type: "anything"},
			expected: BounceClassificationSoftCount,
		},
		{
			name:     "SES unknown type defaults to SoftCount",
			in:       BounceInput{Provider: EmailProviderKindSES, Type: "weird"},
			expected: BounceClassificationSoftCount,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := ClassifyBounce(tc.in)
			assert.Equal(t, tc.expected, got)
		})
	}
}

func TestIsRetryExhaustedDiagnostic(t *testing.T) {
	cases := []struct {
		in       string
		expected bool
	}{
		{"", false},
		{"smtp; 550 4.4.7 Message expired: unable to deliver", true},
		{"550 4.4.7 message expired", true},
		{"Message expired", true},
		{"MESSAGE EXPIRED after retries", true},
		{"smtp; 421 4.4.1 connection refused", false},
		{"4.4.71 not a match", false}, // word-boundary guard
		{"some other error", false},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			assert.Equal(t, c.expected, isRetryExhaustedDiagnostic(c.in))
		})
	}
}

func TestDefaultSoftBounceThreshold(t *testing.T) {
	assert.Equal(t, 5, DefaultSoftBounceThreshold)
}
