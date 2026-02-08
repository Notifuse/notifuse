package broadcast

import (
	"errors"
	"fmt"
)

var (
	// ErrRecipientSkipped is returned when a recipient is skipped due to feed error
	ErrRecipientSkipped = errors.New("recipient skipped due to feed error")
	// ErrBroadcastShouldPause is returned when broadcast should pause due to consecutive feed failures
	ErrBroadcastShouldPause = errors.New("broadcast should pause due to consecutive feed failures")
)

// ErrorCode represents specific error conditions in the broadcast system
type ErrorCode string

const (
	// Template related errors
	ErrCodeTemplateMissing ErrorCode = "TEMPLATE_MISSING"
	ErrCodeTemplateInvalid ErrorCode = "TEMPLATE_INVALID"
	ErrCodeTemplateCompile ErrorCode = "TEMPLATE_COMPILE_FAILED"
	ErrCodeSenderNotFound  ErrorCode = "SENDER_NOT_FOUND"

	// Recipient related errors
	ErrCodeRecipientFetch ErrorCode = "RECIPIENT_FETCH_FAILED"
	ErrCodeNoRecipients   ErrorCode = "NO_RECIPIENTS"

	// Broadcast related errors
	ErrCodeBroadcastNotFound ErrorCode = "BROADCAST_NOT_FOUND"
	ErrCodeBroadcastInvalid  ErrorCode = "BROADCAST_INVALID"

	// Sending related errors
	ErrCodeSendFailed        ErrorCode = "SEND_FAILED"
	ErrCodeRateLimitExceeded ErrorCode = "RATE_LIMIT_EXCEEDED"
	ErrCodeCircuitOpen       ErrorCode = "CIRCUIT_OPEN"

	// Task related errors
	ErrCodeTaskStateInvalid   ErrorCode = "TASK_STATE_INVALID"
	ErrCodeTaskTimeout        ErrorCode = "TASK_TIMEOUT"
	ErrCodeBroadcastCancelled ErrorCode = "BROADCAST_CANCELLED"

	// Recipient feed errors
	ErrCodeRecipientFeedFailed  ErrorCode = "RECIPIENT_FEED_FAILED"
	ErrCodeRecipientSkipped     ErrorCode = "RECIPIENT_SKIPPED"
	ErrCodeBroadcastShouldPause ErrorCode = "BROADCAST_SHOULD_PAUSE"
)

// BroadcastError represents an error in the broadcast system with context
type BroadcastError struct {
	Code      ErrorCode
	Message   string
	TaskID    string
	Retryable bool
	Err       error
}

// Error implements the error interface
func (e *BroadcastError) Error() string {
	if e.Err != nil {
		if e.TaskID != "" {
			return fmt.Sprintf("[%s] %s (task: %s): %v", e.Code, e.Message, e.TaskID, e.Err)
		}
		return fmt.Sprintf("[%s] %s: %v", e.Code, e.Message, e.Err)
	}
	// Fallback when Err is nil
	if e.TaskID != "" {
		return fmt.Sprintf("[%s] %s (task: %s)", e.Code, e.Message, e.TaskID)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

// Unwrap returns the underlying error
func (e *BroadcastError) Unwrap() error {
	return e.Err
}

// NewBroadcastError creates a new broadcast error
func NewBroadcastError(code ErrorCode, message string, retryable bool, err error) *BroadcastError {
	return &BroadcastError{
		Code:      code,
		Message:   message,
		Retryable: retryable,
		Err:       err,
	}
}

// NewBroadcastErrorWithTask creates a new broadcast error with task ID
func NewBroadcastErrorWithTask(code ErrorCode, message string, taskID string, retryable bool, err error) *BroadcastError {
	return &BroadcastError{
		Code:      code,
		Message:   message,
		TaskID:    taskID,
		Retryable: retryable,
		Err:       err,
	}
}

// IsRetryable returns whether the error is retryable
func IsRetryable(err error) bool {
	if err == nil {
		return false
	}
	if e, ok := err.(*BroadcastError); ok {
		return e.Retryable
	}
	return false
}
