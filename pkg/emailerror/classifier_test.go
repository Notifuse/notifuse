package emailerror

import (
	"errors"
	"testing"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/stretchr/testify/assert"
)

func TestClassifier_ClassifySES(t *testing.T) {
	classifier := NewClassifier()

	tests := []struct {
		name         string
		err          error
		expectedType ErrorType
		retryable    bool
	}{
		{
			name:         "recipient error - MessageRejected",
			err:          errors.New("MessageRejected: Email address is not verified"),
			expectedType: ErrorTypeRecipient,
			retryable:    false,
		},
		{
			name:         "recipient error - invalid email",
			err:          errors.New("Error: Email address is not verified"),
			expectedType: ErrorTypeRecipient,
			retryable:    false,
		},
		{
			name:         "provider error - throttling",
			err:          errors.New("ThrottlingException: Rate exceeded"),
			expectedType: ErrorTypeProvider,
			retryable:    true,
		},
		{
			name:         "provider error - quota exceeded",
			err:          errors.New("Error: sending quota exceeded"),
			expectedType: ErrorTypeProvider,
			retryable:    true,
		},
		{
			name:         "provider error - access denied (not retryable)",
			err:          errors.New("AccessDeniedException: User is not authorized"),
			expectedType: ErrorTypeProvider,
			retryable:    false, // Auth errors need manual intervention
		},
		{
			name:         "provider error - service unavailable (not retryable)",
			err:          errors.New("ServiceUnavailable: The service is unavailable"),
			expectedType: ErrorTypeProvider,
			retryable:    false, // Per SES classifier, non-throttle/quota errors are not retryable
		},
		{
			name:         "unknown error",
			err:          errors.New("some random error"),
			expectedType: ErrorTypeUnknown,
			retryable:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := classifier.Classify(tt.err, domain.EmailProviderKindSES)
			assert.Equal(t, tt.expectedType, result.Type)
			assert.Equal(t, tt.retryable, result.Retryable)
			assert.Equal(t, "ses", result.Provider)
		})
	}
}

func TestClassifier_ClassifyPostmark(t *testing.T) {
	classifier := NewClassifier()

	tests := []struct {
		name         string
		err          error
		expectedType ErrorType
		retryable    bool
	}{
		{
			name:         "recipient error - inactive recipient",
			err:          errors.New("Error: inactive recipient"),
			expectedType: ErrorTypeRecipient,
			retryable:    false,
		},
		{
			name:         "recipient error - hard bounce",
			err:          errors.New("Error: hard bounce from recipient"),
			expectedType: ErrorTypeRecipient,
			retryable:    false,
		},
		{
			name:         "recipient error - invalid email",
			err:          errors.New("Error: Invalid email address format"),
			expectedType: ErrorTypeRecipient,
			retryable:    false,
		},
		{
			name:         "provider error - rate limit",
			err:          errors.New("Error: rate limit exceeded"),
			expectedType: ErrorTypeProvider,
			retryable:    true,
		},
		{
			name:         "provider error - auth failed (not retryable without HTTP status)",
			err:          errors.New("Error: authentication failed"),
			expectedType: ErrorTypeProvider,
			retryable:    false, // Auth errors without 5xx/429 status are not retryable
		},
		{
			name:         "provider error - HTTP 429",
			err:          errors.New("status code: 429"),
			expectedType: ErrorTypeProvider,
			retryable:    true,
		},
		{
			name:         "provider error - HTTP 500",
			err:          errors.New("status code: 500"),
			expectedType: ErrorTypeProvider,
			retryable:    true,
		},
		{
			name:         "recipient error - HTTP 406",
			err:          errors.New("status code: 406"),
			expectedType: ErrorTypeRecipient,
			retryable:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := classifier.Classify(tt.err, domain.EmailProviderKindPostmark)
			assert.Equal(t, tt.expectedType, result.Type)
			assert.Equal(t, tt.retryable, result.Retryable)
			assert.Equal(t, "postmark", result.Provider)
		})
	}
}

func TestClassifier_ClassifyMailgun(t *testing.T) {
	classifier := NewClassifier()

	tests := []struct {
		name         string
		err          error
		expectedType ErrorType
		retryable    bool
	}{
		{
			name:         "recipient error - 550",
			err:          errors.New("550 mailbox unavailable"),
			expectedType: ErrorTypeRecipient,
			retryable:    false,
		},
		{
			name:         "recipient error - invalid recipient",
			err:          errors.New("Error: invalid recipient address"),
			expectedType: ErrorTypeRecipient,
			retryable:    false,
		},
		{
			name:         "provider error - rate limit",
			err:          errors.New("Error: rate limit exceeded"),
			expectedType: ErrorTypeProvider,
			retryable:    true,
		},
		{
			name:         "provider error - unauthorized (not retryable)",
			err:          errors.New("status code: 401 unauthorized"),
			expectedType: ErrorTypeProvider,
			retryable:    false, // 401 is not retryable (need to fix credentials)
		},
		{
			name:         "provider error - service unavailable",
			err:          errors.New("Error: service unavailable"),
			expectedType: ErrorTypeProvider,
			retryable:    true, // service unavailable is retryable
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := classifier.Classify(tt.err, domain.EmailProviderKindMailgun)
			assert.Equal(t, tt.expectedType, result.Type)
			assert.Equal(t, tt.retryable, result.Retryable)
			assert.Equal(t, "mailgun", result.Provider)
		})
	}
}

func TestClassifier_ClassifyMailjet(t *testing.T) {
	classifier := NewClassifier()

	tests := []struct {
		name         string
		err          error
		expectedType ErrorType
		retryable    bool
	}{
		{
			name:         "recipient error - hard bounce",
			err:          errors.New(`"hard_bounce": true`),
			expectedType: ErrorTypeRecipient,
			retryable:    false,
		},
		{
			name:         "recipient error - blocked",
			err:          errors.New("Error: blocked recipient"),
			expectedType: ErrorTypeRecipient,
			retryable:    false,
		},
		{
			name:         "provider error - rate limit",
			err:          errors.New("status code: 429 too many requests"),
			expectedType: ErrorTypeProvider,
			retryable:    true,
		},
		{
			name:         "provider error - server error",
			err:          errors.New("status code: 503 service unavailable"),
			expectedType: ErrorTypeProvider,
			retryable:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := classifier.Classify(tt.err, domain.EmailProviderKindMailjet)
			assert.Equal(t, tt.expectedType, result.Type)
			assert.Equal(t, tt.retryable, result.Retryable)
			assert.Equal(t, "mailjet", result.Provider)
		})
	}
}

func TestClassifier_ClassifySparkPost(t *testing.T) {
	classifier := NewClassifier()

	tests := []struct {
		name         string
		err          error
		expectedType ErrorType
		retryable    bool
	}{
		{
			name:         "recipient error - no valid recipients (5002)",
			err:          errors.New(`"code": "5002"`),
			expectedType: ErrorTypeRecipient,
			retryable:    false,
		},
		{
			name:         "recipient error - invalid email format (2008)",
			err:          errors.New(`"code": "2008"`),
			expectedType: ErrorTypeRecipient,
			retryable:    false,
		},
		{
			name:         "recipient error - invalid recipient",
			err:          errors.New("Error: invalid recipient address"),
			expectedType: ErrorTypeRecipient,
			retryable:    false,
		},
		{
			name:         "provider error - rate limit",
			err:          errors.New("status code: 429"),
			expectedType: ErrorTypeProvider,
			retryable:    true,
		},
		{
			name:         "provider error - transmission error",
			err:          errors.New("Error: transmission error"),
			expectedType: ErrorTypeProvider,
			retryable:    false, // Without 5xx/429/rate limit keywords, not retryable
		},
		{
			name:         "provider error - too many requests",
			err:          errors.New("Error: too many requests"),
			expectedType: ErrorTypeProvider,
			retryable:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := classifier.Classify(tt.err, domain.EmailProviderKindSparkPost)
			assert.Equal(t, tt.expectedType, result.Type)
			assert.Equal(t, tt.retryable, result.Retryable)
			assert.Equal(t, "sparkpost", result.Provider)
		})
	}
}

func TestClassifier_ClassifySMTP(t *testing.T) {
	classifier := NewClassifier()

	tests := []struct {
		name         string
		err          error
		expectedType ErrorType
		retryable    bool
	}{
		{
			name:         "recipient error - 550 mailbox unavailable",
			err:          errors.New("550 mailbox unavailable"),
			expectedType: ErrorTypeRecipient,
			retryable:    false,
		},
		{
			name:         "recipient error - 5.1.1 mailbox does not exist",
			err:          errors.New("5.1.1 The email account that you tried to reach does not exist"),
			expectedType: ErrorTypeRecipient,
			retryable:    false,
		},
		{
			name:         "recipient error - user unknown",
			err:          errors.New("Error: user unknown"),
			expectedType: ErrorTypeRecipient,
			retryable:    false,
		},
		{
			name:         "recipient error - mailbox full",
			err:          errors.New("552 mailbox full"),
			expectedType: ErrorTypeRecipient,
			retryable:    false,
		},
		{
			name:         "provider error - 421 service unavailable",
			err:          errors.New("421 Service temporarily unavailable"),
			expectedType: ErrorTypeProvider,
			retryable:    true,
		},
		{
			name:         "provider error - connection timeout",
			err:          errors.New("Error: connection timeout"),
			expectedType: ErrorTypeProvider,
			retryable:    true,
		},
		{
			name:         "provider error - TLS handshake",
			err:          errors.New("Error: TLS handshake failed"),
			expectedType: ErrorTypeProvider,
			retryable:    true,
		},
		{
			name:         "provider error - greylisted",
			err:          errors.New("Error: greylisted, try again later"),
			expectedType: ErrorTypeProvider,
			retryable:    true,
		},
		{
			name:         "provider error - authentication failed",
			err:          errors.New("Error: authentication failed"),
			expectedType: ErrorTypeProvider,
			retryable:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := classifier.Classify(tt.err, domain.EmailProviderKindSMTP)
			assert.Equal(t, tt.expectedType, result.Type)
			assert.Equal(t, tt.retryable, result.Retryable)
			assert.Equal(t, "smtp", result.Provider)
		})
	}
}

func TestClassifier_HTTPStatusExtraction(t *testing.T) {
	tests := []struct {
		name           string
		errMsg         string
		expectedStatus int
	}{
		{
			name:           "status code format",
			errMsg:         "status code: 429",
			expectedStatus: 429,
		},
		{
			name:           "status_code format",
			errMsg:         "status_code: 500",
			expectedStatus: 500,
		},
		{
			name:           "no status code",
			errMsg:         "some error without status",
			expectedStatus: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractHTTPStatus(tt.errMsg)
			assert.Equal(t, tt.expectedStatus, result)
		})
	}
}

func TestClassifier_UnknownProvider(t *testing.T) {
	classifier := NewClassifier()

	err := errors.New("some error")
	result := classifier.Classify(err, "unknown_provider")

	assert.Equal(t, ErrorTypeUnknown, result.Type)
	assert.True(t, result.Retryable)
	assert.Equal(t, "unknown", result.Provider)
}

func TestClassifiedError_Methods(t *testing.T) {
	t.Run("IsRecipientError", func(t *testing.T) {
		recipientErr := &ClassifiedError{Type: ErrorTypeRecipient}
		providerErr := &ClassifiedError{Type: ErrorTypeProvider}
		unknownErr := &ClassifiedError{Type: ErrorTypeUnknown}

		assert.True(t, recipientErr.IsRecipientError())
		assert.False(t, providerErr.IsRecipientError())
		assert.False(t, unknownErr.IsRecipientError())
	})

	t.Run("IsProviderError", func(t *testing.T) {
		recipientErr := &ClassifiedError{Type: ErrorTypeRecipient}
		providerErr := &ClassifiedError{Type: ErrorTypeProvider}
		unknownErr := &ClassifiedError{Type: ErrorTypeUnknown}

		assert.False(t, recipientErr.IsProviderError())
		assert.True(t, providerErr.IsProviderError())
		assert.True(t, unknownErr.IsProviderError()) // Unknown treated as provider
	})

	t.Run("Error and Unwrap", func(t *testing.T) {
		originalErr := errors.New("original error")
		classifiedErr := &ClassifiedError{
			Original: originalErr,
			Type:     ErrorTypeProvider,
		}

		assert.Equal(t, "original error", classifiedErr.Error())
		assert.Equal(t, originalErr, classifiedErr.Unwrap())
	})
}

func TestClassifyByHTTPStatus(t *testing.T) {
	tests := []struct {
		status       int
		expectedType ErrorType
	}{
		{406, ErrorTypeRecipient},
		{429, ErrorTypeProvider},
		{401, ErrorTypeProvider},
		{403, ErrorTypeProvider},
		{500, ErrorTypeProvider},
		{502, ErrorTypeProvider},
		{503, ErrorTypeProvider},
		{200, ErrorTypeUnknown},
		{400, ErrorTypeUnknown},
		{404, ErrorTypeUnknown},
	}

	for _, tt := range tests {
		t.Run(string(rune(tt.status)), func(t *testing.T) {
			result := classifyByHTTPStatus(tt.status)
			assert.Equal(t, tt.expectedType, result)
		})
	}
}
