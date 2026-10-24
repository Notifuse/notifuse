package emailerror

import (
	"errors"
	"testing"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func checkClassified(t *testing.T, got *ClassifiedError, wantType ErrorType, wantRetry bool, wantProvider string, wantStatus int) {
	t.Helper()
	require.NotNil(t, got)
	assert.Equal(t, wantType, got.Type)
	assert.Equal(t, wantRetry, got.Retryable)
	assert.Equal(t, wantProvider, got.Provider)
	assert.Equal(t, wantStatus, got.HTTPStatus)
}

func TestCoverage_ClassifyNilAndExtractHTTPStatus(t *testing.T) {
	c := NewClassifier()
	assert.Nil(t, c.Classify(nil, domain.EmailProviderKindSES))
	assert.Nil(t, c.Classify(nil, domain.EmailProviderKindSMTP))

	cases := []struct {
		msg  string
		want int
	}{
		{"status code: 429", 429},
		{"status_code:500", 500},
		{"status code 503", 503},
		{"STATUS CODE: 301", 301},
		{"HTTP/1.1 500 Internal Server Error", 500},
		{"http 429", 429},
		{"HTTP/2 200", 200},
		{"(429)", 429},
		{"[503]", 503},
		{"error (42) short", 0},
		{"error (1234) four digits", 0},
		{"no status at all", 0},
		{"", 0},
		{"status code: abc", 0},
		// The status-code pattern wins over the bracket pattern.
		{"status code: 404 (500)", 404},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, extractHTTPStatus(tc.msg), tc.msg)
	}
}

func TestCoverage_ClassifyByHTTPStatusBoundaries(t *testing.T) {
	cases := []struct {
		status int
		want   ErrorType
	}{
		{0, ErrorTypeUnknown},
		{200, ErrorTypeUnknown},
		{301, ErrorTypeUnknown},
		{399, ErrorTypeUnknown},
		{400, ErrorTypeUnknown},
		{402, ErrorTypeUnknown},
		{404, ErrorTypeUnknown},
		{405, ErrorTypeUnknown},
		{406, ErrorTypeRecipient},
		{407, ErrorTypeUnknown},
		{429, ErrorTypeProvider},
		{430, ErrorTypeUnknown},
		{499, ErrorTypeUnknown},
		{401, ErrorTypeProvider},
		{403, ErrorTypeProvider},
		{500, ErrorTypeProvider},
		{502, ErrorTypeProvider},
		{599, ErrorTypeProvider},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, classifyByHTTPStatus(tc.status), "status %d", tc.status)
	}
}

func TestCoverage_UnknownProviderBranches(t *testing.T) {
	c := NewClassifier()
	for _, kind := range []domain.EmailProviderKind{"", "bogus-provider"} {
		checkClassified(t, c.Classify(errors.New("status code: 500"), kind), ErrorTypeProvider, true, "unknown", 500)
		checkClassified(t, c.Classify(errors.New("status code: 429"), kind), ErrorTypeProvider, true, "unknown", 429)
		checkClassified(t, c.Classify(errors.New("(406)"), kind), ErrorTypeRecipient, false, "unknown", 406)
		checkClassified(t, c.Classify(errors.New("status code: 404"), kind), ErrorTypeUnknown, false, "unknown", 404)
		checkClassified(t, c.Classify(errors.New("status code: 401"), kind), ErrorTypeProvider, false, "unknown", 401)
		checkClassified(t, c.Classify(errors.New("plain failure"), kind), ErrorTypeUnknown, true, "unknown", 0)
	}
}

func TestCoverage_ClassifiedErrorNilOriginalAndCircuitBreaker(t *testing.T) {
	empty := &ClassifiedError{}
	assert.Equal(t, "", empty.Error())
	assert.Nil(t, empty.Unwrap())

	wrapped := errors.New("boom")
	ce := &ClassifiedError{Original: wrapped, Type: ErrorTypeProvider}
	assert.Equal(t, "boom", ce.Error())
	assert.True(t, errors.Is(ce, wrapped))

	assert.False(t, (&ClassifiedError{Type: ErrorTypeRecipient}).ShouldTriggerCircuitBreaker())
	assert.True(t, (&ClassifiedError{Type: ErrorTypeProvider}).ShouldTriggerCircuitBreaker())
	assert.True(t, (&ClassifiedError{Type: ErrorTypeUnknown}).ShouldTriggerCircuitBreaker())
	assert.False(t, (&ClassifiedError{Type: ErrorTypeRecipient}).IsProviderError())
	assert.True(t, (&ClassifiedError{Type: ErrorTypeUnknown}).IsProviderError())
}

func TestCoverage_MailgunFallbacks(t *testing.T) {
	c := NewClassifier()
	kind := domain.EmailProviderKindMailgun
	checkClassified(t, c.Classify(errors.New("totally novel failure"), kind), ErrorTypeUnknown, true, "mailgun", 0)
	checkClassified(t, c.Classify(errors.New("status code: 503"), kind), ErrorTypeProvider, true, "mailgun", 503)
	checkClassified(t, c.Classify(errors.New("status code: 404"), kind), ErrorTypeUnknown, false, "mailgun", 404)
	// Provider phrase without any retry signal is not retryable.
	checkClassified(t, c.Classify(errors.New("unauthorized"), kind), ErrorTypeProvider, false, "mailgun", 0)
	checkClassified(t, c.Classify(errors.New("421: try later"), kind), ErrorTypeProvider, true, "mailgun", 0)
	// Recipient phrases win over provider phrases.
	checkClassified(t, c.Classify(errors.New("550 mailbox unavailable: rate limit"), kind), ErrorTypeRecipient, false, "mailgun", 0)
	checkClassified(t, c.Classify(errors.New("552: storage exceeded"), kind), ErrorTypeRecipient, false, "mailgun", 0)
	checkClassified(t, c.Classify(errors.New("invalid api key"), kind), ErrorTypeProvider, false, "mailgun", 0)
}

func TestCoverage_MailjetFallbacks(t *testing.T) {
	c := NewClassifier()
	kind := domain.EmailProviderKindMailjet
	checkClassified(t, c.Classify(errors.New("totally novel failure"), kind), ErrorTypeUnknown, true, "mailjet", 0)
	checkClassified(t, c.Classify(errors.New("status code: 500"), kind), ErrorTypeProvider, true, "mailjet", 500)
	checkClassified(t, c.Classify(errors.New("status code: 400"), kind), ErrorTypeUnknown, false, "mailjet", 400)
	checkClassified(t, c.Classify(errors.New("unauthorized"), kind), ErrorTypeProvider, false, "mailjet", 0)
	checkClassified(t, c.Classify(errors.New("not found"), kind), ErrorTypeProvider, false, "mailjet", 0)
	checkClassified(t, c.Classify(errors.New("preblocked address"), kind), ErrorTypeRecipient, false, "mailjet", 0)
	// The pattern list holds a literal "error_related_to.*recipient": only the
	// literal string matches, because containsAny is a substring check.
	checkClassified(t, c.Classify(errors.New(`error_related_to.*recipient`), kind), ErrorTypeRecipient, false, "mailjet", 0)
}

func TestCoverage_SESSenderVerificationAndFallbacks(t *testing.T) {
	c := NewClassifier()
	kind := domain.EmailProviderKindSES
	// Sender-side "not verified" is a provider problem, not a recipient one.
	checkClassified(t, c.Classify(errors.New("MessageRejected: sender is not verified"), kind), ErrorTypeProvider, false, "ses", 0)
	checkClassified(t, c.Classify(errors.New("MessageRejected: from address is not verified"), kind), ErrorTypeProvider, false, "ses", 0)
	// Without sender context the same wording stays a recipient error.
	checkClassified(t, c.Classify(errors.New("MessageRejected: recipient is not verified"), kind), ErrorTypeRecipient, false, "ses", 0)
	// Non-throttle provider errors are not retryable.
	checkClassified(t, c.Classify(errors.New("ConfigurationSetDoesNotExistException"), kind), ErrorTypeProvider, false, "ses", 0)
	checkClassified(t, c.Classify(errors.New("ExpiredToken: token expired"), kind), ErrorTypeProvider, false, "ses", 0)
	checkClassified(t, c.Classify(errors.New("Account is paused"), kind), ErrorTypeProvider, false, "ses", 0)
	// HTTP fallback and unknown default.
	checkClassified(t, c.Classify(errors.New("status code: 503"), kind), ErrorTypeProvider, true, "ses", 503)
	checkClassified(t, c.Classify(errors.New("status code: 400"), kind), ErrorTypeUnknown, false, "ses", 400)
	checkClassified(t, c.Classify(errors.New("totally novel failure"), kind), ErrorTypeUnknown, true, "ses", 0)
}

func TestCoverage_SMTPCodeExtractionBoundaries(t *testing.T) {
	cases := []struct {
		msg  string
		code int
		ok   bool
	}{
		{"550 mailbox unavailable", 550, true},
		{"421 service unavailable", 421, true},
		{"message rejected with code 452: quota", 452, true},
		{"message rejected with code: 550", 550, true},
		{"RCPT TO rejected for a@b.c with code 550: x", 550, true},
		{"server said 550 ok", 0, false},     // code only counts at start or after "code"
		{"350 intermediate reply", 0, false}, // 3xx is not matched
		{"code 5500 too long", 0, false},
		{"no digits here", 0, false},
		{"", 0, false},
	}
	for _, tc := range cases {
		code, ok := extractSMTPCode(tc.msg)
		assert.Equal(t, tc.ok, ok, tc.msg)
		assert.Equal(t, tc.code, code, tc.msg)
	}

	for code, want := range map[int]bool{421: false, 450: false, 549: false, 550: true, 551: true, 554: true, 555: false, 500: false} {
		assert.Equal(t, want, smtpCodeIsRecipient(code), "code %d", code)
	}
}

func TestCoverage_SMTPClassificationPrecedence(t *testing.T) {
	c := NewClassifier()
	kind := domain.EmailProviderKindSMTP
	// A 53x code is a provider fault even when the text says auth failed.
	checkClassified(t, c.Classify(errors.New("code 535: authentication failed"), kind), ErrorTypeProvider, true, "smtp", 0)
	// The reply code beats the phrase lists in both directions.
	checkClassified(t, c.Classify(errors.New("code 550: try again later"), kind), ErrorTypeRecipient, false, "smtp", 0)
	checkClassified(t, c.Classify(errors.New("code 450: mailbox unavailable"), kind), ErrorTypeProvider, true, "smtp", 0)
	// Phrase-list recipient codes that the code regex cannot see (enhanced codes).
	checkClassified(t, c.Classify(errors.New("5.2.2 mailbox full"), kind), ErrorTypeRecipient, false, "smtp", 0)
	// HTTP fallback via a bracketed status the SMTP code regex cannot see.
	checkClassified(t, c.Classify(errors.New("(500)"), kind), ErrorTypeProvider, true, "smtp", 500)
	checkClassified(t, c.Classify(errors.New("(404)"), kind), ErrorTypeUnknown, false, "smtp", 404)
	checkClassified(t, c.Classify(errors.New("totally novel failure"), kind), ErrorTypeUnknown, true, "smtp", 0)
}

func TestCoverage_SendGridSparkPostPostmarkFallbacks(t *testing.T) {
	c := NewClassifier()
	sg := domain.EmailProviderKindSendGrid
	checkClassified(t, c.Classify(errors.New("totally novel failure"), sg), ErrorTypeUnknown, true, "sendgrid", 0)
	checkClassified(t, c.Classify(errors.New("(404)"), sg), ErrorTypeUnknown, false, "sendgrid", 404)
	checkClassified(t, c.Classify(errors.New("api error"), sg), ErrorTypeProvider, false, "sendgrid", 0)
	checkClassified(t, c.Classify(errors.New("Request timeout"), sg), ErrorTypeProvider, true, "sendgrid", 0)

	sp := domain.EmailProviderKindSparkPost
	checkClassified(t, c.Classify(errors.New("totally novel failure"), sp), ErrorTypeUnknown, true, "sparkpost", 0)
	checkClassified(t, c.Classify(errors.New("(500)"), sp), ErrorTypeProvider, true, "sparkpost", 500)
	checkClassified(t, c.Classify(errors.New("sending limit reached"), sp), ErrorTypeProvider, false, "sparkpost", 0)
	checkClassified(t, c.Classify(errors.New("no valid recipients"), sp), ErrorTypeRecipient, false, "sparkpost", 0)

	pm := domain.EmailProviderKindPostmark
	checkClassified(t, c.Classify(errors.New("totally novel failure"), pm), ErrorTypeUnknown, true, "postmark", 0)
	checkClassified(t, c.Classify(errors.New("(429)"), pm), ErrorTypeProvider, true, "postmark", 429)
	checkClassified(t, c.Classify(errors.New("(400)"), pm), ErrorTypeUnknown, false, "postmark", 400)
	checkClassified(t, c.Classify(errors.New("invalid api key"), pm), ErrorTypeProvider, false, "postmark", 0)
	checkClassified(t, c.Classify(errors.New("spam complaint"), pm), ErrorTypeRecipient, false, "postmark", 0)
	// HTTP status is preserved on pattern-matched results too.
	r := c.Classify(errors.New("status code: 500 server error"), pm)
	checkClassified(t, r, ErrorTypeProvider, true, "postmark", 500)
}
