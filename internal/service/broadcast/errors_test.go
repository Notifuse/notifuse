package broadcast

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

// TestIsInterrupted pins which failures count as "the run was cut short"
// rather than "the broadcast is broken". Only context cancellation qualifies:
// these used to surface as BROADCAST_NOT_FOUND, sending everyone reading the
// logs looking for a deleted broadcast.
func TestIsInterrupted(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"cancelled", context.Canceled, true},
		{"deadline exceeded", context.DeadlineExceeded, true},
		{"wrapped in fmt.Errorf", fmt.Errorf("get connection: %w", context.Canceled), true},
		{"wrapped in a BroadcastError", NewBroadcastError(ErrCodeSendFailed, "enqueue", true, context.Canceled), true},
		{"unrelated error", errors.New("boom"), false},
		{"unrelated BroadcastError", NewBroadcastError(ErrCodeTemplateMissing, "missing", false, nil), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsInterrupted(tt.err))
		})
	}
}

// TestWrapIfInterrupted checks the conversion used by the recipient-fetch paths.
func TestWrapIfInterrupted(t *testing.T) {
	t.Run("converts a cancellation", func(t *testing.T) {
		wrapped, ok := wrapIfInterrupted(fmt.Errorf("get connection: %w", context.Canceled))
		require.True(t, ok)
		assert.Equal(t, ErrCodeInterrupted, wrapped.Code)
		assert.True(t, wrapped.Retryable)
		assert.ErrorIs(t, wrapped, context.Canceled)
	})

	t.Run("leaves other errors to the caller", func(t *testing.T) {
		wrapped, ok := wrapIfInterrupted(errors.New("no such broadcast"))
		assert.False(t, ok)
		assert.Nil(t, wrapped)
	})
}
