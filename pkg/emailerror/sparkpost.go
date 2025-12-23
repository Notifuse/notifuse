package emailerror

// SparkPost error classification
//
// RECIPIENT ERRORS (should NOT trigger circuit breaker):
// - Code 5002: No valid recipients in list
// - Code 2008: No local part specified in sender address
// - Invalid recipient
//
// PROVIDER ERRORS (SHOULD trigger circuit breaker):
// - HTTP 429: Rate limit exceeded
// - HTTP 5xx: Server errors
// - Codes 2000-3999: Transmission errors
// - Codes 1000-1999: API service errors

// SparkPost recipient error patterns
var sparkpostRecipientPatterns = []string{
	"5002",
	"2008",
	"no valid recipients",
	"invalid recipient",
	"recipient rejected",
	"mailbox not found",
	"user unknown",
}

// SparkPost provider error patterns
var sparkpostProviderPatterns = []string{
	"rate limit",
	"too many requests",
	"throttl",
	"service unavailable",
	"internal server error",
	"transmission error",
	"sending limit",
	"api error",
	"authentication",
	"unauthorized",
}

func (c *Classifier) classifySparkPostError(err error, errStr string, httpStatus int) *ClassifiedError {
	result := &ClassifiedError{
		Original:   err,
		Provider:   "sparkpost",
		HTTPStatus: httpStatus,
		Retryable:  true,
	}

	// Check for recipient-specific errors
	if containsAny(errStr, sparkpostRecipientPatterns) {
		result.Type = ErrorTypeRecipient
		result.Retryable = false
		return result
	}

	// Check for provider errors
	if containsAny(errStr, sparkpostProviderPatterns) {
		result.Type = ErrorTypeProvider
		// Rate limit and server errors are retryable
		result.Retryable = httpStatus >= 500 || httpStatus == 429 || containsAny(errStr, []string{"rate limit", "too many", "throttl"})
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
