package emailerror

// Mailjet error classification
//
// RECIPIENT ERRORS (should NOT trigger circuit breaker):
// - hard_bounce: true
// - error_related_to: "recipient"
// - blocked events
// - invalid recipient
//
// PROVIDER ERRORS (SHOULD trigger circuit breaker):
// - HTTP 401: Unauthorized
// - HTTP 404: Not found (resource doesn't exist)
// - HTTP 429: Rate limit exceeded
// - HTTP 500/502/503: Server errors

// Mailjet recipient error patterns
var mailjetRecipientPatterns = []string{
	"hard_bounce",
	"hardbounce",
	"hard bounce",
	"error_related_to.*recipient",
	"blocked",
	"preblocked",
	"invalid recipient",
	"recipient rejected",
	"user unknown",
	"mailbox not found",
	"duplicate in campaign",
}

// Mailjet provider error patterns
var mailjetProviderPatterns = []string{
	"unauthorized",
	"authentication",
	"not found",
	"rate limit",
	"too many requests",
	"service unavailable",
	"internal server error",
	"bad gateway",
	"api key",
}

func (c *Classifier) classifyMailjetError(err error, errStr string, httpStatus int) *ClassifiedError {
	result := &ClassifiedError{
		Original:   err,
		Provider:   "mailjet",
		HTTPStatus: httpStatus,
		Retryable:  true,
	}

	// Check for recipient-specific errors
	if containsAny(errStr, mailjetRecipientPatterns) {
		result.Type = ErrorTypeRecipient
		result.Retryable = false
		return result
	}

	// Check for provider errors
	if containsAny(errStr, mailjetProviderPatterns) {
		result.Type = ErrorTypeProvider
		// Rate limit and server errors are retryable
		result.Retryable = httpStatus >= 500 || httpStatus == 429 || containsAny(errStr, []string{"rate limit", "too many"})
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
