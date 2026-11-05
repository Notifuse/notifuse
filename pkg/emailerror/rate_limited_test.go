package emailerror

import (
	"errors"
	"testing"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The strings below are the ones each provider's SendEmail actually returns: SES through
// wrapSESError, the HTTP providers through their non-OK status branch, SMTP through
// smtpReplyDetail wrapped by SendEmail.
func TestClassifier_RateLimited(t *testing.T) {
	c := NewClassifier()

	cases := []struct {
		name        string
		kind        domain.EmailProviderKind
		msg         string
		wantType    ErrorType
		wantRetry   bool
		rateLimited bool
	}{
		// Throttles: provider, retryable, rate limited.
		{"SES v2 sending rate", domain.EmailProviderKindSES, "SES error: TooManyRequestsException: Maximum sending rate exceeded.", ErrorTypeProvider, true, true},
		{"SES generic throttling", domain.EmailProviderKindSES, "SES error: api error Throttling: Maximum sending rate exceeded.", ErrorTypeProvider, true, true},
		{"SES throttling exception", domain.EmailProviderKindSES, "SES error: ThrottlingException: Rate exceeded", ErrorTypeProvider, true, true},
		{"SendGrid 429", domain.EmailProviderKindSendGrid, "API returned non-OK status code 429", ErrorTypeProvider, true, true},
		{"Mailgun 429", domain.EmailProviderKindMailgun, "API returned non-OK status code 429", ErrorTypeProvider, true, true},
		{"Mailjet 429", domain.EmailProviderKindMailjet, "API returned non-OK status code 429", ErrorTypeProvider, true, true},
		{"SparkPost 429", domain.EmailProviderKindSparkPost, "API returned non-OK status code 429", ErrorTypeProvider, true, true},
		{"Postmark 429", domain.EmailProviderKindPostmark, `postmark API error (429): {"ErrorCode":0,"Message":"Rate limit exceeded"}`, ErrorTypeProvider, true, true},
		{"SMTP SES endpoint", domain.EmailProviderKindSMTP, "failed to send email: message rejected with code 454: Throttling failure: Maximum sending rate exceeded.", ErrorTypeProvider, true, true},
		{"SMTP Postfix at MAIL FROM", domain.EmailProviderKindSMTP, "failed to send email: MAIL FROM rejected with code 451: 4.7.1 Error: too much mail from 10.0.0.1", ErrorTypeProvider, true, true},
		{"SMTP submission rate at DATA", domain.EmailProviderKindSMTP, "failed to send email: DATA rejected with code 450: 4.7.0 Message submission rate for this client has exceeded the configured limit", ErrorTypeProvider, true, true},
		{"SMTP sender rate limit at RCPT", domain.EmailProviderKindSMTP, "failed to send email: RCPT TO rejected for a@b.c with code 450: 4.7.1 <a@b.c>: Recipient address rejected: rate limit exceeded", ErrorTypeProvider, true, true},
		{"unknown kind 429", "bogus-provider", "status code: 429", ErrorTypeProvider, true, true},

		// Quotas and other provider refusals: provider, retryable, not rate limited.
		{"SES daily quota", domain.EmailProviderKindSES, "SES error: TooManyRequestsException: Daily message quota exceeded.", ErrorTypeProvider, true, false},
		{"SES daily limit without the word quota", domain.EmailProviderKindSES, "SES error: TooManyRequestsException: Daily sending limit exceeded", ErrorTypeProvider, true, false},
		{"Postmark monthly", domain.EmailProviderKindPostmark, `postmark API error (429): {"ErrorCode":0,"Message":"Monthly limit reached"}`, ErrorTypeProvider, true, false},
		{"SMTP SES endpoint daily quota", domain.EmailProviderKindSMTP, "failed to send email: message rejected with code 454: Throttling failure: Daily message quota exceeded.", ErrorTypeProvider, true, false},
		{"SMTP daily quota", domain.EmailProviderKindSMTP, "failed to send email: message rejected with code 452: 4.3.1 Daily sending quota exceeded", ErrorTypeProvider, true, false},
		{"SMTP hourly limit", domain.EmailProviderKindSMTP, "failed to send email: message rejected with code 451: 4.7.1 hourly rate limit exceeded", ErrorTypeProvider, true, false},
		{"SMTP greeting refused", domain.EmailProviderKindSMTP, "failed to send email: unexpected greeting code: 421", ErrorTypeProvider, true, false},
		{"SMTP recipient receiving too fast", domain.EmailProviderKindSMTP, "failed to send email: RCPT TO rejected for a@b.c with code 450: 4.2.1 The user you are trying to contact is receiving mail at a rate that prevents additional messages from being delivered", ErrorTypeProvider, true, false},
		{"SMTP mailbox busy at RCPT", domain.EmailProviderKindSMTP, "failed to send email: RCPT TO rejected for a@b.c with code 450: mailbox busy, too many messages", ErrorTypeProvider, true, false},
		{"SMTP 4xx without a throttle phrase", domain.EmailProviderKindSMTP, "failed to send email: message rejected with code 451: 4.3.0 local error in processing", ErrorTypeProvider, true, false},

		// Unchanged.
		{"SparkPost sending limit", domain.EmailProviderKindSparkPost, "API returned non-OK status code 420", ErrorTypeUnknown, false, false},
		{"SendGrid 500", domain.EmailProviderKindSendGrid, "API returned non-OK status code 500", ErrorTypeProvider, true, false},
		{"SES access denied", domain.EmailProviderKindSES, "SES error: AccessDeniedException: User is not authorized", ErrorTypeProvider, false, false},
		{"SMTP 5xx wins", domain.EmailProviderKindSMTP, "failed to send email: message rejected with code 550: user unknown, rate limit", ErrorTypeRecipient, false, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := c.Classify(errors.New(tc.msg), tc.kind)
			require.NotNil(t, got)
			assert.Equal(t, tc.wantType, got.Type)
			assert.Equal(t, tc.wantRetry, got.Retryable)
			assert.Equal(t, tc.rateLimited, got.RateLimited)
		})
	}
}

// A recipient address that happens to contain a throttle phrase must not make a refusal
// look like a throttle: only the server's reply text is read.
func TestSMTPReplyText(t *testing.T) {
	text, ok := smtpReplyText("failed to send email: DATA rejected for ratelimit@b.c with code 451: 4.3.0 local error")
	require.True(t, ok)
	assert.Equal(t, "4.3.0 local error", text)
	assert.False(t, isSMTPThrottle(text))

	_, ok = smtpReplyText("connection reset by peer")
	assert.False(t, ok)
}
