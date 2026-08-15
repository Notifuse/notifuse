package domain

import (
	"errors"
	"fmt"
	"testing"
)

func TestErrNotFound_Error(t *testing.T) {
	err := &ErrNotFound{
		Entity: "broadcast",
		ID:     "12345",
	}

	expected := "broadcast not found with ID: 12345"
	if err.Error() != expected {
		t.Errorf("Expected error message '%s', got '%s'", expected, err.Error())
	}
}

func TestErrTaskExecution_Error(t *testing.T) {
	// Test with nil wrapped error
	err1 := &ErrTaskExecution{
		TaskID: "task123",
		Reason: "processor not found",
	}

	expected1 := "task execution failed [task123]: processor not found"
	if err1.Error() != expected1 {
		t.Errorf("Expected error message '%s', got '%s'", expected1, err1.Error())
	}

	// Test with wrapped error
	underlyingErr := fmt.Errorf("database connection failed")
	err2 := &ErrTaskExecution{
		TaskID: "task456",
		Reason: "database error",
		Err:    underlyingErr,
	}

	expected2 := "task execution failed [task456]: database error - database connection failed"
	if err2.Error() != expected2 {
		t.Errorf("Expected error message '%s', got '%s'", expected2, err2.Error())
	}

	// Test error unwrapping
	if !errors.Is(err2, underlyingErr) {
		t.Error("errors.Is() failed to find the wrapped error")
	}
}

func TestErrTaskTimeout_Error(t *testing.T) {
	err := &ErrTaskTimeout{
		TaskID:     "task789",
		MaxRuntime: 60,
	}

	expected := "task timed out [task789] after 60 seconds"
	if err.Error() != expected {
		t.Errorf("Expected error message '%s', got '%s'", expected, err.Error())
	}
}

func TestErrorTypeAssertion(t *testing.T) {
	// Test that we can properly use type assertions with these errors
	var err error

	// Create an ErrNotFound
	err = &ErrNotFound{Entity: "task", ID: "123"}

	// Type assertion should work
	if _, ok := err.(*ErrNotFound); !ok {
		t.Error("Type assertion for ErrNotFound failed")
	}

	// Create an ErrTaskExecution
	err = &ErrTaskExecution{TaskID: "456", Reason: "test"}

	// Type assertion should work
	if _, ok := err.(*ErrTaskExecution); !ok {
		t.Error("Type assertion for ErrTaskExecution failed")
	}

	// Negative test - wrong type
	if _, ok := err.(*ErrNotFound); ok {
		t.Error("Type assertion incorrectly succeeded for wrong error type")
	}
}

func TestPermissionError_Error(t *testing.T) {
	// Test PermissionError.Error method - this was at 0% coverage
	err := &PermissionError{
		Resource:   PermissionResourceWorkspace,
		Permission: PermissionTypeRead,
		Message:    "You do not have permission to read this workspace",
	}

	expected := "You do not have permission to read this workspace"
	if err.Error() != expected {
		t.Errorf("Expected error message '%s', got '%s'", expected, err.Error())
	}

	// Test with different message
	err2 := &PermissionError{
		Resource:   PermissionResourceContacts,
		Permission: PermissionTypeWrite,
		Message:    "Access denied",
	}

	expected2 := "Access denied"
	if err2.Error() != expected2 {
		t.Errorf("Expected error message '%s', got '%s'", expected2, err2.Error())
	}

	// Test that Error() returns the Message field directly
	if err.Error() != err.Message {
		t.Error("Error() should return the Message field")
	}
}

func TestTriggerConditionError_Error(t *testing.T) {
	err := NewTriggerConditionError("invalid trigger conditions: column \"country\" does not exist")

	expected := "invalid trigger conditions: column \"country\" does not exist"
	if err.Error() != expected {
		t.Errorf("Expected error message '%s', got '%s'", expected, err.Error())
	}

	if err.Error() != err.Message {
		t.Error("Error() should return the Message field")
	}
}

// The HTTP handlers answer 400 by matching this type with errors.As after the service and
// repository layers have wrapped it with %w, so the match must survive that wrapping.
func TestTriggerConditionError_As(t *testing.T) {
	original := NewTriggerConditionError("branch must have at least one leaf")
	wrapped := fmt.Errorf("failed to create automation trigger: %w", original)
	doubleWrapped := fmt.Errorf("activate automation: %w", wrapped)

	var target *TriggerConditionError
	if !errors.As(doubleWrapped, &target) {
		t.Fatal("errors.As() failed to find TriggerConditionError through the wrapping")
	}

	if target != original {
		t.Error("errors.As() should yield the original TriggerConditionError")
	}

	if target.Error() != "branch must have at least one leaf" {
		t.Errorf("Expected message 'branch must have at least one leaf', got '%s'", target.Error())
	}

	// A different error type must not match, or every failure would answer 400.
	var negative *TriggerConditionError
	if errors.As(fmt.Errorf("wrapped: %w", &ErrNotFound{Entity: "automation", ID: "123"}), &negative) {
		t.Error("errors.As() matched TriggerConditionError on an unrelated error")
	}
}

func TestAutomationConflictError_Error(t *testing.T) {
	err := NewAutomationConflictError("auto-123", AutomationStatusLive)

	expected := "automation auto-123 was concurrently changed: it was no longer live"
	if err.Error() != expected {
		t.Errorf("Expected error message '%s', got '%s'", expected, err.Error())
	}
}

// The handlers answer 409 by matching this type with errors.As after the service has wrapped
// it with %w, so the match must survive that wrapping.
func TestAutomationConflictError_As(t *testing.T) {
	original := NewAutomationConflictError("auto-123", AutomationStatusLive)
	doubleWrapped := fmt.Errorf("pause automation: %w", fmt.Errorf("failed to update automation status: %w", original))

	var target *AutomationConflictError
	if !errors.As(doubleWrapped, &target) {
		t.Fatal("errors.As() failed to find AutomationConflictError through the wrapping")
	}
	if target != original {
		t.Error("errors.As() should yield the original AutomationConflictError")
	}
	if target.AutomationID != "auto-123" {
		t.Errorf("Expected automation ID 'auto-123', got '%s'", target.AutomationID)
	}
	if target.ExpectedStatus != AutomationStatusLive {
		t.Errorf("Expected status live, got '%s'", target.ExpectedStatus)
	}

	// A conflict must not be confused with a bad trigger configuration: one is a 409 the
	// caller can retry after reloading, the other a 400 they cannot retry at all.
	var conditionErr *TriggerConditionError
	if errors.As(doubleWrapped, &conditionErr) {
		t.Error("errors.As() matched TriggerConditionError on a conflict error")
	}
}
