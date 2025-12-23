package emailerror

// ErrorType classifies email sending errors for circuit breaker decisions
type ErrorType string

const (
	// ErrorTypeRecipient indicates a recipient-specific error (bad email, mailbox full)
	// These should NOT trigger circuit breaker - the issue is with the recipient, not the provider
	ErrorTypeRecipient ErrorType = "recipient"

	// ErrorTypeProvider indicates a provider/infrastructure error (auth failure, rate limit, service down)
	// These SHOULD trigger circuit breaker - the issue affects all sends
	ErrorTypeProvider ErrorType = "provider"

	// ErrorTypeUnknown indicates an unclassified error
	// Treated as provider error for safety (conservative approach)
	ErrorTypeUnknown ErrorType = "unknown"
)

// ClassifiedError wraps an error with classification metadata for circuit breaker decisions
type ClassifiedError struct {
	// Original is the underlying error
	Original error

	// Type classifies the error as recipient, provider, or unknown
	Type ErrorType

	// Provider is the email provider name (ses, postmark, mailgun, etc.)
	Provider string

	// HTTPStatus is the extracted HTTP status code (0 if not applicable)
	HTTPStatus int

	// Retryable indicates whether this error can be retried
	Retryable bool
}

// Error implements the error interface
func (e *ClassifiedError) Error() string {
	if e.Original == nil {
		return ""
	}
	return e.Original.Error()
}

// Unwrap returns the underlying error for errors.Is/As compatibility
func (e *ClassifiedError) Unwrap() error {
	return e.Original
}

// IsRecipientError returns true if this is a recipient-related error
// Recipient errors should NOT trigger the circuit breaker
func (e *ClassifiedError) IsRecipientError() bool {
	return e.Type == ErrorTypeRecipient
}

// IsProviderError returns true if this is a provider/infrastructure error
// Provider errors SHOULD trigger the circuit breaker
// Unknown errors are treated as provider errors for safety
func (e *ClassifiedError) IsProviderError() bool {
	return e.Type == ErrorTypeProvider || e.Type == ErrorTypeUnknown
}

// ShouldTriggerCircuitBreaker returns true if this error should count toward the circuit breaker threshold
func (e *ClassifiedError) ShouldTriggerCircuitBreaker() bool {
	return e.IsProviderError()
}
