package emailerror

// Postmark error classification
//
// RECIPIENT ERRORS (should NOT trigger circuit breaker):
// - HTTP 406: Inactive recipient (hard bounce, complaint)
// - Invalid email address format
// - Unsubscribed recipient
// - Hard bounce
//
// PROVIDER ERRORS (SHOULD trigger circuit breaker):
// - HTTP 429: Rate limit exceeded
// - HTTP 401: Authentication failure
// - HTTP 500/502/503: Server errors

// Postmark recipient error patterns
var postmarkRecipientPatterns = []string{
	"inactive recipient",
	"invalid email",
	"invalid address",
	"hard bounce",
	"hardbounce",
	"unsubscribed",
	"spam complaint",
	"recipient not found",
	"mailbox not found",
}

// Postmark provider error patterns
var postmarkProviderPatterns = []string{
	"rate limit",
	"ratelimit",
	"too many requests",
	"unauthorized",
	"authentication",
	"invalid api",
	"api key",
	"server error",
	"internal error",
	"service unavailable",
}

func (c *Classifier) classifyPostmarkError(err error, errStr string, httpStatus int) *ClassifiedError {
	result := &ClassifiedError{
		Original:   err,
		Provider:   "postmark",
		HTTPStatus: httpStatus,
		Retryable:  true,
	}

	// HTTP 406 is specifically for inactive recipients in Postmark
	if httpStatus == 406 {
		result.Type = ErrorTypeRecipient
		result.Retryable = false
		return result
	}

	// Check for recipient-specific errors
	if containsAny(errStr, postmarkRecipientPatterns) {
		result.Type = ErrorTypeRecipient
		result.Retryable = false
		return result
	}

	// Check for provider errors
	if containsAny(errStr, postmarkProviderPatterns) {
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
