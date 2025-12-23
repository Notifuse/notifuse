package emailerror

// Mailgun error classification
//
// RECIPIENT ERRORS (should NOT trigger circuit breaker):
// - SMTP 550: Mailbox unavailable / doesn't exist
// - SMTP 551: User not local
// - SMTP 552: Storage exceeded
// - SMTP 553: Mailbox name not allowed
// - SMTP 554: Transaction failed (often spam/blocked)
//
// PROVIDER ERRORS (SHOULD trigger circuit breaker):
// - HTTP 401: Unauthorized (invalid API key)
// - HTTP 403: Forbidden
// - HTTP 429: Rate limit exceeded
// - HTTP 500/502/503: Server errors
// - SMTP 421: Service unavailable

// Mailgun recipient error patterns (SMTP-based)
var mailgunRecipientPatterns = []string{
	"550 ",
	"550:",
	"551 ",
	"551:",
	"552 ",
	"552:",
	"553 ",
	"553:",
	"554 ",
	"554:",
	"mailbox unavailable",
	"mailbox not found",
	"user not found",
	"user unknown",
	"no such user",
	"recipient rejected",
	"invalid recipient",
	"does not exist",
	"storage exceeded",
	"mailbox full",
}

// Mailgun provider error patterns
var mailgunProviderPatterns = []string{
	"421 ",
	"421:",
	"unauthorized",
	"forbidden",
	"rate limit",
	"too many requests",
	"service unavailable",
	"internal server error",
	"bad gateway",
	"authentication failed",
	"invalid api key",
	"api key",
}

func (c *Classifier) classifyMailgunError(err error, errStr string, httpStatus int) *ClassifiedError {
	result := &ClassifiedError{
		Original:   err,
		Provider:   "mailgun",
		HTTPStatus: httpStatus,
		Retryable:  true,
	}

	// Check for recipient-specific errors (SMTP codes)
	if containsAny(errStr, mailgunRecipientPatterns) {
		result.Type = ErrorTypeRecipient
		result.Retryable = false
		return result
	}

	// Check for provider errors
	if containsAny(errStr, mailgunProviderPatterns) {
		result.Type = ErrorTypeProvider
		// Rate limit and server errors are retryable
		result.Retryable = httpStatus >= 500 || httpStatus == 429 || containsAny(errStr, []string{"421", "rate limit", "too many", "service unavailable"})
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
