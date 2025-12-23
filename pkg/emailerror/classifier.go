package emailerror

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/Notifuse/notifuse/internal/domain"
)

// Classifier classifies email sending errors by provider type
type Classifier struct{}

// NewClassifier creates a new error classifier
func NewClassifier() *Classifier {
	return &Classifier{}
}

// Classify analyzes an error and returns a ClassifiedError with type information
func (c *Classifier) Classify(err error, provider domain.EmailProviderKind) *ClassifiedError {
	if err == nil {
		return nil
	}

	errStr := err.Error()
	httpStatus := extractHTTPStatus(errStr)

	switch provider {
	case domain.EmailProviderKindSES:
		return c.classifySESError(err, errStr, httpStatus)
	case domain.EmailProviderKindPostmark:
		return c.classifyPostmarkError(err, errStr, httpStatus)
	case domain.EmailProviderKindMailgun:
		return c.classifyMailgunError(err, errStr, httpStatus)
	case domain.EmailProviderKindMailjet:
		return c.classifyMailjetError(err, errStr, httpStatus)
	case domain.EmailProviderKindSparkPost:
		return c.classifySparkPostError(err, errStr, httpStatus)
	case domain.EmailProviderKindSMTP:
		return c.classifySMTPError(err, errStr, httpStatus)
	default:
		return c.classifyUnknownProvider(err, errStr, httpStatus)
	}
}

// HTTP status extraction patterns
var (
	// Matches patterns like "status code: 429", "status_code: 500", "status code 503"
	httpStatusRegex = regexp.MustCompile(`(?i)status[_\s]code[:\s]*(\d{3})`)

	// Matches patterns like "HTTP 429", "http/1.1 500"
	httpPrefixRegex = regexp.MustCompile(`(?i)http[/\d.]*\s*(\d{3})`)

	// Matches patterns like "(429)", "[500]"
	bracketStatusRegex = regexp.MustCompile(`[\[(](\d{3})[\])]`)
)

// extractHTTPStatus attempts to extract HTTP status code from error message
func extractHTTPStatus(errStr string) int {
	// Try main pattern first
	if matches := httpStatusRegex.FindStringSubmatch(errStr); len(matches) >= 2 {
		if status, err := strconv.Atoi(matches[1]); err == nil {
			return status
		}
	}

	// Try HTTP prefix pattern
	if matches := httpPrefixRegex.FindStringSubmatch(errStr); len(matches) >= 2 {
		if status, err := strconv.Atoi(matches[1]); err == nil {
			return status
		}
	}

	// Try bracket pattern
	if matches := bracketStatusRegex.FindStringSubmatch(errStr); len(matches) >= 2 {
		if status, err := strconv.Atoi(matches[1]); err == nil {
			return status
		}
	}

	return 0
}

// classifyByHTTPStatus provides classification based on HTTP status code
func classifyByHTTPStatus(status int) ErrorType {
	switch {
	// Postmark-specific: 406 is inactive recipient
	case status == 406:
		return ErrorTypeRecipient

	// Rate limiting and server errors are provider issues
	case status == 429:
		return ErrorTypeProvider
	case status >= 500:
		return ErrorTypeProvider

	// Auth errors are provider issues (wrong API key, etc.)
	case status == 401, status == 403:
		return ErrorTypeProvider

	// 4xx errors (except 406, 429) might be recipient or request issues
	case status >= 400 && status < 500:
		return ErrorTypeUnknown

	default:
		return ErrorTypeUnknown
	}
}

// containsAny checks if the error string contains any of the patterns (case-insensitive)
func containsAny(errStr string, patterns []string) bool {
	errLower := strings.ToLower(errStr)
	for _, pattern := range patterns {
		if strings.Contains(errLower, strings.ToLower(pattern)) {
			return true
		}
	}
	return false
}

// classifyUnknownProvider handles errors from unknown providers
func (c *Classifier) classifyUnknownProvider(err error, errStr string, httpStatus int) *ClassifiedError {
	result := &ClassifiedError{
		Original:   err,
		Provider:   "unknown",
		HTTPStatus: httpStatus,
		Retryable:  true,
	}

	// Try HTTP status classification
	if httpStatus > 0 {
		result.Type = classifyByHTTPStatus(httpStatus)
		result.Retryable = httpStatus >= 500 || httpStatus == 429
		return result
	}

	// Default to unknown (treated as provider error)
	result.Type = ErrorTypeUnknown
	return result
}
