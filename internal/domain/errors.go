package domain

import (
	"fmt"
)

// Common error types
type ErrNotFound struct {
	Entity string
	ID     string
}

func (e *ErrNotFound) Error() string {
	return fmt.Sprintf("%s not found with ID: %s", e.Entity, e.ID)
}

// Task-specific errors
type ErrTaskExecution struct {
	TaskID string
	Reason string
	Err    error
}

func (e *ErrTaskExecution) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("task execution failed [%s]: %s - %v", e.TaskID, e.Reason, e.Err)
	}
	return fmt.Sprintf("task execution failed [%s]: %s", e.TaskID, e.Reason)
}

func (e *ErrTaskExecution) Unwrap() error {
	return e.Err
}

type ErrTaskTimeout struct {
	TaskID     string
	MaxRuntime int
}

func (e *ErrTaskTimeout) Error() string {
	return fmt.Sprintf("task timed out [%s] after %d seconds", e.TaskID, e.MaxRuntime)
}

// ValidationError represents an error that occurs due to invalid input or parameters
type ValidationError struct {
	Message string
}

// Error implements the error interface
func (e ValidationError) Error() string {
	return fmt.Sprintf("validation error: %s", e.Message)
}

// NewValidationError creates a new validation error with the given message
func NewValidationError(message string) error {
	return ValidationError{
		Message: message,
	}
}

// PermissionError represents insufficient permissions for an operation
type PermissionError struct {
	Resource   PermissionResource `json:"resource"`
	Permission PermissionType     `json:"permission"`
	Message    string             `json:"message"`
}

// Error implements the error interface
func (e *PermissionError) Error() string {
	return e.Message
}

// NewPermissionError creates a new permission error
func NewPermissionError(resource PermissionResource, permission PermissionType, message string) *PermissionError {
	return &PermissionError{
		Resource:   resource,
		Permission: permission,
		Message:    message,
	}
}

// ErrInsufficientPermissions is the default insufficient permissions error
var ErrInsufficientPermissions = NewPermissionError(
	PermissionResourceWorkspace,
	PermissionTypeRead,
	"Insufficient permissions",
)
