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
// that list. A list with no row at all is refused too, by its id, as one that does not exist
// (ListRef.Missing).
//
// It answers 400 rather than 409, like a trigger configuration that cannot be installed: the
// automation's own configuration is what has to change, and retrying as it is cannot work.
type AutomationDeletedListError struct {
	AutomationID string
	Lists        []ListRef
}

// Error implements the error interface
func (e *AutomationDeletedListError) Error() string {
	uses, anyDeleted, anyMissing := unusableLists(e.Lists)

	rule := "a live automation cannot use a deleted list"
	if anyMissing && !anyDeleted {
		rule = "a live automation cannot use a list that does not exist"
	} else if anyMissing {
		rule = "a live automation cannot use a deleted list or one that does not exist"
	}

	return fmt.Sprintf("this automation uses %s: %s", uses, rule)
}

// NewAutomationDeletedListError creates a new automation deleted list error
func NewAutomationDeletedListError(automationID string, lists []ListRef) *AutomationDeletedListError {
	return &AutomationDeletedListError{AutomationID: automationID, Lists: lists}
}

// unusableLists names, for a message, lists an automation cannot run on: the deleted ones, then
// the ones that do not exist. It also reports which of the two kinds there are.
func unusableLists(lists []ListRef) (uses string, anyDeleted, anyMissing bool) {
	var deleted, missing []string
	for _, list := range lists {
		if list.Missing {
			missing = append(missing, list.label())
		} else {
			deleted = append(deleted, list.label())
		}
	}

	var phrases []string
	if len(deleted) == 1 {
		phrases = append(phrases, "the deleted list "+quotedNames(deleted))
	} else if len(deleted) > 1 {
		phrases = append(phrases, "the deleted lists "+quotedNames(deleted))
	}
	if len(missing) == 1 {
		phrases = append(phrases, "the list "+quotedNames(missing)+", which does not exist")
	} else if len(missing) > 1 {
		phrases = append(phrases, "the lists "+quotedNames(missing)+", which do not exist")
	}

	return strings.Join(phrases, ", and "), len(deleted) > 0, len(missing) > 0
}

// AutomationPausedError reports an update to a live automation that failed once it was stored,
// when its trigger could not be installed, and that could not be undone by putting the previous
// version back live: the automation was paused instead, on its previous version.
//
// It wraps the update's own failure, which decides how it is answered, and adds the pause:
// told only of the failure, the caller would save again and take the success for an automation
// still live. Lists names the deleted or missing lists the previous version uses, when they are
// what kept it from going back live; it is empty when putting it back failed for another reason.
type AutomationPausedError struct {
	AutomationID string
	Lists        []ListRef
	Err          error
}

// Error implements the error interface: the failure, then the pause.
func (e *AutomationPausedError) Error() string {
	return fmt.Sprintf("%v; %s", e.Err, e.PauseNotice())
}

// Unwrap returns the update's own failure.
func (e *AutomationPausedError) Unwrap() error {
	return e.Err
}

// PauseNotice says that the automation was paused, and why.
func (e *AutomationPausedError) PauseNotice() string {
	if len(e.Lists) == 0 {
		return "the automation was paused, because its previous version could not be put back"
	}
	uses, _, _ := unusableLists(e.Lists)
	return "the automation was paused, because its previous version uses " + uses
}

// NewAutomationPausedError creates a new automation paused error
func NewAutomationPausedError(automationID string, lists []ListRef, err error) *AutomationPausedError {
	return &AutomationPausedError{AutomationID: automationID, Lists: lists, Err: err}
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
