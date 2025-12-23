package emailerror

// SES (Amazon Simple Email Service) error classification
//
// RECIPIENT ERRORS (should NOT trigger circuit breaker):
// - MessageRejected: Email address is not verified, invalid format
// - Invalid recipient address
// - Mailbox unavailable
//
// PROVIDER ERRORS (SHOULD trigger circuit breaker):
// - ThrottlingException: Rate exceeded
// - LimitExceededException: Quota exceeded
// - ServiceUnavailable: Service down
// - AccessDeniedException: Auth/permission issues
// - InvalidClientTokenId: Invalid credentials
// - SignatureDoesNotMatch: Invalid credentials
// - ExpiredToken: Invalid credentials

// SES recipient error patterns
var sesRecipientPatterns = []string{
	"messagerejected",
	"email address is not verified",
	"invalid recipient",
	"mailbox unavailable",
	"mailbox not found",
	"user unknown",
	"address rejected",
	"no recipients",
	"recipient rejected",
}

// SES provider error patterns
var sesProviderPatterns = []string{
	"throttling",
	"throttlingexception",
	"limitexceeded",
	"quota exceeded",
	"daily message quota",
	"serviceunavailable",
	"service unavailable",
	"accessdenied",
	"accessdeniedexception",
	"invalidclienttokenid",
	"signaturedoesnotmatch",
	"expiredtoken",
	"expired token",
	"account is paused",
	"account paused",
	"sending paused",
	"configurationset",
}

func (c *Classifier) classifySESError(err error, errStr string, httpStatus int) *ClassifiedError {
	result := &ClassifiedError{
		Original:   err,
		Provider:   "ses",
		HTTPStatus: httpStatus,
		Retryable:  true,
	}

	// Check for recipient-specific errors first
	if containsAny(errStr, sesRecipientPatterns) {
		// Special case: "MessageRejected" can be both recipient and sender issue
		// If it mentions "not verified" with sender context, it might be provider issue
		// But if it's about recipient, it's a recipient error
		if containsAny(errStr, []string{"sender", "from address"}) && containsAny(errStr, []string{"not verified"}) {
			result.Type = ErrorTypeProvider
			result.Retryable = false // Need to fix sender verification
			return result
		}

		result.Type = ErrorTypeRecipient
		result.Retryable = false // Don't retry recipient errors
		return result
	}

	// Check for provider errors
	if containsAny(errStr, sesProviderPatterns) {
		result.Type = ErrorTypeProvider

		// Throttling and quota errors are retryable (after backoff)
		if containsAny(errStr, []string{"throttl", "quota"}) {
			result.Retryable = true
		} else {
			// Auth errors typically need manual intervention
			result.Retryable = false
		}
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
