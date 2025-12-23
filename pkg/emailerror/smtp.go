package emailerror

// SMTP error classification
//
// RECIPIENT ERRORS (5xx permanent failures - should NOT trigger circuit breaker):
// - 550: Mailbox unavailable (recipient doesn't exist)
// - 551: User not local (routing issue)
// - 552: Storage exceeded (mailbox full)
// - 553: Mailbox name not allowed (invalid format)
//
// PROVIDER ERRORS (4xx temporary failures - SHOULD trigger circuit breaker):
// - 421: Service temporarily unavailable
// - 450: Mailbox busy
// - 451: Local error in processing
// - 452: Insufficient storage
// - Connection timeouts, TLS failures

// SMTP recipient error patterns (5xx permanent failures)
var smtpRecipientPatterns = []string{
	"550 ",
	"550:",
	"551 ",
	"551:",
	"552 ",
	"552:",
	"553 ",
	"553:",
	"5.1.1", // Mailbox does not exist
	"5.1.2", // Bad destination mailbox
	"5.1.3", // Bad destination mailbox syntax
	"5.2.1", // Mailbox disabled
	"5.2.2", // Mailbox full
	"5.7.1", // Delivery not authorized (often recipient policy)
	"mailbox unavailable",
	"mailbox not found",
	"user unknown",
	"no such user",
	"recipient rejected",
	"does not exist",
	"mailbox full",
	"over quota",
}

// SMTP provider error patterns (4xx temporary failures, connection issues)
var smtpProviderPatterns = []string{
	"421 ",
	"421:",
	"450 ",
	"450:",
	"451 ",
	"451:",
	"452 ",
	"452:",
	"4.7.1", // Delivery not authorized (server policy)
	"connection refused",
	"connection reset",
	"connection timeout",
	"timed out",
	"timeout",
	"tls handshake",
	"tls error",
	"ssl error",
	"authentication failed",
	"auth failed",
	"login failed",
	"service unavailable",
	"try again later",
	"temporary failure",
	"greylisted",
	"greylist",
}

func (c *Classifier) classifySMTPError(err error, errStr string, httpStatus int) *ClassifiedError {
	result := &ClassifiedError{
		Original:   err,
		Provider:   "smtp",
		HTTPStatus: httpStatus,
		Retryable:  true,
	}

	// Check for recipient-specific errors (5xx permanent failures)
	if containsAny(errStr, smtpRecipientPatterns) {
		result.Type = ErrorTypeRecipient
		result.Retryable = false
		return result
	}

	// Check for provider errors (4xx temporary failures, connection issues)
	if containsAny(errStr, smtpProviderPatterns) {
		result.Type = ErrorTypeProvider
		// Most SMTP temporary errors are retryable
		result.Retryable = true
		return result
	}

	// Fallback to HTTP status classification (if applicable)
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
