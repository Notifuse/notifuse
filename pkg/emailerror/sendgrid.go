package emailerror

// SendGrid error classification
//
// RECIPIENT ERRORS (should NOT trigger circuit breaker):
// - Invalid email address
// - Mailbox not found
// - User unknown / does not exist
// - 550, 551, 552, 553, 554 SMTP codes
//
// PROVIDER ERRORS (SHOULD trigger circuit breaker):
// - HTTP 429: Rate limit exceeded
// - HTTP 5xx: Server errors
// - Authentication errors
// - Service unavailable

// SendGrid recipient error patterns
var sendgridRecipientPatterns = []string{
	"invalid email",
	"mailbox not found",
	"user unknown",
	"does not exist",
	"invalid recipient",
	"recipient rejected",
	"550",
	"551",
	"552",
	"553",
	"554",
	"no such user",
	"address rejected",
}

// SendGrid provider error patterns
var sendgridProviderPatterns = []string{
	"rate limit",
	"too many requests",
	"throttl",
	"service unavailable",
	"internal server error",
	"authentication",
	"unauthorized",
	"forbidden",
	"api error",
	"timeout",
}

func (c *Classifier) classifySendGridError(err error, errStr string, httpStatus int) *ClassifiedError {
	result := &ClassifiedError{
		Original:   err,
		Provider:   "sendgrid",
		HTTPStatus: httpStatus,
		Retryable:  true,
	}

	// Check for recipient-specific errors
	if containsAny(errStr, sendgridRecipientPatterns) {
		result.Type = ErrorTypeRecipient
		result.Retryable = false
		return result
	}

	// Check for provider errors
	if containsAny(errStr, sendgridProviderPatterns) {
		result.Type = ErrorTypeProvider
		// Rate limit, server errors, and service unavailable are retryable
		result.Retryable = httpStatus >= 500 || httpStatus == 429 || containsAny(errStr, []string{"rate limit", "too many", "throttl", "timeout", "service unavailable"})
		return result
	}

	// Fallback to HTTP status classification
	if httpStatus > 0 {
		result.Type = classifyByHTTPStatus(httpStatus)
		result.Retryable = httpStatus >= 500 || httpStatus == 429
		return result
	}

	// Unknown error - treat as provider error for safety
	result.Type = ErrorTypeUnknown
	result.Retryable = true
	return result
}
