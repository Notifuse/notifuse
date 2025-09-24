package broadcast

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBroadcastError_Error(t *testing.T) {
	// Test error without task ID
	err1 := &BroadcastError{
		Code:      ErrCodeSendFailed,
		Message:   "Failed to send email",
		Retryable: true,
		Err:       errors.New("connection error"),
	}
	expected1 := "[SEND_FAILED] Failed to send email: connection error"
	assert.Equal(t, expected1, err1.Error())

	// Test error with task ID
	err2 := &BroadcastError{
		Code:      ErrCodeTaskTimeout,
		Message:   "Timeout occurred",
		TaskID:    "task-123",
		Retryable: true,
		Err:       errors.New("deadline exceeded"),
	}
	expected2 := "[TASK_TIMEOUT] Timeout occurred (task: task-123): deadline exceeded"
	assert.Equal(t, expected2, err2.Error())
}

func TestBroadcastError_Unwrap(t *testing.T) {
	originalErr := errors.New("original error")
	broadcastErr := &BroadcastError{
		Code:      ErrCodeTemplateMissing,
		Message:   "Template not found",
		Retryable: false,
		Err:       originalErr,
	}

	unwrappedErr := broadcastErr.Unwrap()
	assert.Equal(t, originalErr, unwrappedErr)
}

func TestNewBroadcastError(t *testing.T) {
	originalErr := errors.New("some error")
	broadcastErr := NewBroadcastError(
		ErrCodeRateLimitExceeded,
		"Rate limit exceeded",
		true,
		originalErr,
	)

	assert.Equal(t, ErrCodeRateLimitExceeded, broadcastErr.Code)
	assert.Equal(t, "Rate limit exceeded", broadcastErr.Message)
	assert.Equal(t, true, broadcastErr.Retryable)
	assert.Equal(t, originalErr, broadcastErr.Err)
	assert.Equal(t, "", broadcastErr.TaskID)
}

func TestNewBroadcastErrorWithTask(t *testing.T) {
	originalErr := errors.New("some error")
	taskID := "task-456"
	broadcastErr := NewBroadcastErrorWithTask(
		ErrCodeCircuitOpen,
		"Circuit breaker open",
		taskID,
		false,
		originalErr,
	)

	assert.Equal(t, ErrCodeCircuitOpen, broadcastErr.Code)
	assert.Equal(t, "Circuit breaker open", broadcastErr.Message)
	assert.Equal(t, taskID, broadcastErr.TaskID)
	assert.Equal(t, false, broadcastErr.Retryable)
	assert.Equal(t, originalErr, broadcastErr.Err)
}

func TestIsRetryable(t *testing.T) {
	testCases := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "Nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "Regular error",
			err:      errors.New("regular error"),
			expected: false,
		},
		{
			name: "Retryable broadcast error",
			err: &BroadcastError{
				Code:      ErrCodeSendFailed,
				Message:   "Send failed",
				Retryable: true,
				Err:       errors.New("temp failure"),
			},
			expected: true,
		},
		{
			name: "Non-retryable broadcast error",
			err: &BroadcastError{
				Code:      ErrCodeBroadcastInvalid,
				Message:   "Invalid broadcast",
				Retryable: false,
				Err:       errors.New("validation error"),
			},
			expected: false,
		},
		{
			name: "Broadcast cancelled error",
			err: &BroadcastError{
				Code:      ErrCodeBroadcastCancelled,
				Message:   "Broadcast cancelled",
				Retryable: false,
				Err:       nil,
			},
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := IsRetryable(tc.err)
			assert.Equal(t, tc.expected, result)
		})
	}
}
