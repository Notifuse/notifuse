package domain

import (
	"fmt"
	"strings"
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

// ErrTaskAlreadyRunning is returned when attempting to execute a task that is already running
type ErrTaskAlreadyRunning struct {
	TaskID string
}

func (e *ErrTaskAlreadyRunning) Error() string {
	return fmt.Sprintf("task already running [%s]", e.TaskID)
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

// TriggerConditionError represents an automation trigger configuration that cannot be
// compiled or installed — a bad condition tree, an unsupported value type, or a column
// the workspace database does not have. The offending input came from the caller, so
// this answers 400 with its message rather than a generic 500 with nothing in it.
type TriggerConditionError struct {
	Message string `json:"message"`
}

// Error implements the error interface
func (e *TriggerConditionError) Error() string {
	return e.Message
}

// NewTriggerConditionError creates a new trigger condition error
func NewTriggerConditionError(message string) *TriggerConditionError {
	return &TriggerConditionError{Message: message}
}

// AutomationConflictError reports a status transition that was computed from a row another
// transition has since changed. Activate, Pause and Update each read the automation, decide
// whether to install or drop its trigger, and write the row back; writing through a status
// predicate means the loser of a race stops instead of overwriting the winner from a stale
// read — and, more importantly, stops before emitting DDL from a decision that no longer
// holds.
//
// It answers 409 rather than 500: nothing is broken and nothing about the request was wrong.
// Reloading and retrying is exactly what works.
type AutomationConflictError struct {
	AutomationID   string           `json:"automation_id"`
	ExpectedStatus AutomationStatus `json:"expected_status"`
}

// Error implements the error interface. The observed status is deliberately absent: a write
// that matched no row does not report what the row became, and re-reading it to find out
// would be one more racy read reported as fact.
func (e *AutomationConflictError) Error() string {
	return fmt.Sprintf("automation %s was concurrently changed: it was no longer %s", e.AutomationID, e.ExpectedStatus)
}

// NewAutomationConflictError creates a new automation conflict error
func NewAutomationConflictError(automationID string, expectedStatus AutomationStatus) *AutomationConflictError {
	return &AutomationConflictError{AutomationID: automationID, ExpectedStatus: expectedStatus}
}

// AutomationDeletedListError refuses a write that would leave an automation live while a list
// it requires (Automation.RequiredListIDs) is deleted: an activation, or a change to a live
// automation. Live, it would fail or misbehave on every contact that reached the step using
// that list. A list with no row at all is reported the same way, by its id.
//
// It answers 400 rather than 409, like a trigger configuration that cannot be installed: the
// automation's own configuration is what has to change, and retrying as it is cannot work.
type AutomationDeletedListError struct {
	AutomationID string
	Lists        []ListRef
}

// Error implements the error interface
func (e *AutomationDeletedListError) Error() string {
	names := make([]string, len(e.Lists))
	for i, list := range e.Lists {
		names[i] = list.label()
	}
	if len(names) == 1 {
		return fmt.Sprintf("this automation uses the deleted list %s: a live automation cannot use a deleted list", quotedNames(names))
	}
	return fmt.Sprintf("this automation uses the deleted lists %s: a live automation cannot use a deleted list", quotedNames(names))
}

// NewAutomationDeletedListError creates a new automation deleted list error
func NewAutomationDeletedListError(automationID string, lists []ListRef) *AutomationDeletedListError {
	return &AutomationDeletedListError{AutomationID: automationID, Lists: lists}
}

// quotedNamesShown is how many names quotedNames spells out before it counts the rest.
const quotedNamesShown = 3

// quoted puts a name in double quotes for a message. Not %q: the message is read by people,
// and a quote or an accent in a name should not come out as an escape sequence.
func quoted(name string) string {
	return `"` + name + `"`
}

// quotedNames joins names for a message: "A", "A" and "B", "A", "B" and "C". Past
// quotedNamesShown it names that many and counts the rest ("A", "B", "C" and 2 more), so a
// message about many stays one readable line.
func quotedNames(names []string) string {
	shown := make([]string, 0, quotedNamesShown)
	for _, name := range names {
		if len(shown) == quotedNamesShown {
			break
		}
		shown = append(shown, quoted(name))
	}

	if rest := len(names) - len(shown); rest > 0 {
		return strings.Join(shown, ", ") + fmt.Sprintf(" and %d more", rest)
	}
	if len(shown) <= 1 {
		return strings.Join(shown, "")
	}
	return strings.Join(shown[:len(shown)-1], ", ") + " and " + shown[len(shown)-1]
}

// ErrInsufficientPermissions is the default insufficient permissions error
var ErrInsufficientPermissions = NewPermissionError(
	PermissionResourceWorkspace,
	PermissionTypeRead,
	"Insufficient permissions",
)
