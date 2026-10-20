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

// The console shows this message when an activation or a save is refused, so it has to name
// the lists — and not call deleted a list that never existed: that one is named by the id the
// automation gives, which is most likely a typo.
func TestAutomationDeletedListError_Error(t *testing.T) {
	tests := []struct {
		name  string
		lists []ListRef
		want  string
	}{
		{
			name:  "one deleted list",
			lists: []ListRef{{ID: "news", Name: "Newsletter"}},
			want:  `this automation uses the deleted list "Newsletter": a live automation cannot use a deleted list`,
		},
		{
			name:  "two deleted lists",
			lists: []ListRef{{ID: "news", Name: "Newsletter"}, {ID: "vip", Name: "VIP"}},
			want:  `this automation uses the deleted lists "Newsletter" and "VIP": a live automation cannot use a deleted list`,
		},
		{
			name:  "a list that does not exist, by its id",
			lists: []ListRef{{ID: "newsltter", Missing: true}},
			want:  `this automation uses the list "newsltter", which does not exist: a live automation cannot use a list that does not exist`,
		},
		{
			name:  "two lists that do not exist",
			lists: []ListRef{{ID: "a", Missing: true}, {ID: "b", Missing: true}},
			want:  `this automation uses the lists "a" and "b", which do not exist: a live automation cannot use a list that does not exist`,
		},
		{
			name:  "a deleted list and one that does not exist",
			lists: []ListRef{{ID: "news", Name: "Newsletter"}, {ID: "newsltter", Missing: true}},
			want:  `this automation uses the deleted list "Newsletter", and the list "newsltter", which does not exist: a live automation cannot use a deleted list or one that does not exist`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := NewAutomationDeletedListError("auto-123", tt.lists)
			if err.Error() != tt.want {
				t.Errorf("Expected error message '%s', got '%s'", tt.want, err.Error())
			}
		})
	}
}

// The handlers answer 400 by matching this type with errors.As after the service has wrapped
// it, and must not mistake it for the conflict they answer 409.
func TestAutomationDeletedListError_As(t *testing.T) {
	original := NewAutomationDeletedListError("auto-123", []ListRef{{ID: "news", Name: "Newsletter"}})
	wrapped := fmt.Errorf("failed to update automation status: %w", original)

	var target *AutomationDeletedListError
	if !errors.As(wrapped, &target) {
		t.Fatal("errors.As() failed to find AutomationDeletedListError through the wrapping")
	}
	if target != original {
		t.Error("errors.As() should yield the original AutomationDeletedListError")
	}

	var conflictErr *AutomationConflictError
	if errors.As(wrapped, &conflictErr) {
		t.Error("errors.As() matched AutomationConflictError on a deleted list error")
	}
}

// A failed update undone by pausing the automation. The caller is told what failed, as before,
// and then that the automation is no longer live and why, naming the lists the way the refusal
// to save does: told only of the failure, they would save again and take the success for live.
func TestAutomationPausedError_Error(t *testing.T) {
	failure := errors.New("failed to update automation trigger: invalid trigger conditions: operator does not exist")

	tests := []struct {
		name  string
		lists []ListRef
		want  string
	}{
		{
			name:  "its previous version uses a deleted list",
			lists: []ListRef{{ID: "news", Name: "Newsletter"}},
			want:  failure.Error() + `; the automation was paused, because its previous version uses the deleted list "Newsletter"`,
		},
		{
			name:  "its previous version uses a deleted list and one that does not exist",
			lists: []ListRef{{ID: "news", Name: "Newsletter"}, {ID: "newsltter", Missing: true}},
			want:  failure.Error() + `; the automation was paused, because its previous version uses the deleted list "Newsletter", and the list "newsltter", which does not exist`,
		},
		{
			name: "its previous version could not be put back",
			want: failure.Error() + "; the automation was paused, because its previous version could not be put back",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := NewAutomationPausedError("auto-123", tt.lists, failure)
			if err.Error() != tt.want {
				t.Errorf("Expected error message '%s', got '%s'", tt.want, err.Error())
			}
			if want := tt.want[len(failure.Error())+len("; "):]; err.PauseNotice() != want {
				t.Errorf("Expected pause notice '%s', got '%s'", want, err.PauseNotice())
			}
		})
	}
}

// The handler answers with the status of what failed, which it finds through the pause, and
// must not take the pause for a refusal of the update itself: the update was stored.
func TestAutomationPausedError_As(t *testing.T) {
	condition := NewTriggerConditionError("invalid trigger conditions: operator does not exist")
	paused := NewAutomationPausedError("auto-123", []ListRef{{ID: "news", Name: "Newsletter"}},
		fmt.Errorf("failed to update automation trigger: %w", condition))
	wrapped := fmt.Errorf("failed to update automation: %w", paused)

	var target *AutomationPausedError
	if !errors.As(wrapped, &target) || target != paused {
		t.Fatal("errors.As() should yield the original AutomationPausedError through the wrapping")
	}

	var conditionErr *TriggerConditionError
	if !errors.As(wrapped, &conditionErr) || conditionErr != condition {
		t.Error("errors.As() should find the failure the pause wraps")
	}

	var deletedList *AutomationDeletedListError
	if errors.As(wrapped, &deletedList) {
		t.Error("errors.As() matched AutomationDeletedListError on an update that was stored")
	}
}

func TestQuotedNames(t *testing.T) {
	tests := []struct {
		names []string
		want  string
	}{
		{nil, ``},
		{[]string{"A"}, `"A"`},
		{[]string{"A", "B"}, `"A" and "B"`},
		{[]string{"A", "B", "C"}, `"A", "B" and "C"`},
		{[]string{"A", "B", "C", "D"}, `"A", "B", "C" and 1 more`},
		{[]string{"A", "B", "C", "D", "E", "F"}, `"A", "B", "C" and 3 more`},
		// Read by people: a quote or an accent in a name is not escaped.
		{[]string{`Spring "sale"`, "Été"}, `"Spring "sale"" and "Été"`},
	}

	for _, tt := range tests {
		if got := quotedNames(tt.names); got != tt.want {
			t.Errorf("quotedNames(%q) = %s, want %s", tt.names, got, tt.want)
		}
	}
}
