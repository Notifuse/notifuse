package mcp

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/Notifuse/notifuse/internal/domain"
)

// The stable codes of a tool error. A client may switch on them; the message
// is for people.
const (
	CodeNotFound         = "not_found"
	CodePermissionDenied = "permission_denied"
	CodeValidation       = "validation"
	CodeInvalidState     = "invalid_state"
	CodeConflict         = "conflict"
	CodeLicenceRequired  = "licence_required"
	CodeRateLimited      = "rate_limited"
	CodeInternal         = "internal"
)

// ToolError is the error object of an isError result. Resource and Permission
// name the missing grant of a permission refusal, Feature and RequiredTier the
// licence a refusal asks for, and Status the current state of an object that
// refused an action.
type ToolError struct {
	Code         string `json:"code"`
	Message      string `json:"message"`
	Resource     string `json:"resource,omitempty"`
	Permission   string `json:"permission,omitempty"`
	Feature      string `json:"feature,omitempty"`
	RequiredTier string `json:"required_tier,omitempty"`
	Status       string `json:"status,omitempty"`
}

// Error implements the error interface, so a tool handler can return one.
func (e *ToolError) Error() string {
	return e.Code + ": " + e.Message
}

// auditStatus is the HTTP status an audit row records for the outcome.
func (e *ToolError) auditStatus() int {
	switch e.Code {
	case CodeValidation:
		return http.StatusBadRequest
	case CodeLicenceRequired:
		return http.StatusPaymentRequired
	case CodePermissionDenied:
		return http.StatusForbidden
	case CodeNotFound:
		return http.StatusNotFound
	case CodeInvalidState, CodeConflict:
		return http.StatusConflict
	case CodeRateLimited:
		return http.StatusTooManyRequests
	default:
		return http.StatusInternalServerError
	}
}

func validationError(format string, args ...any) *ToolError {
	return &ToolError{Code: CodeValidation, Message: fmt.Sprintf(format, args...)}
}

func permissionDenied(resource domain.PermissionResource, verb domain.PermissionType) *ToolError {
	return &ToolError{
		Code:       CodePermissionDenied,
		Message:    fmt.Sprintf("this API key lacks the %s %s permission", resource, verb),
		Resource:   string(resource),
		Permission: string(verb),
	}
}

func rateLimitedError() *ToolError {
	return &ToolError{Code: CodeRateLimited, Message: "too many calls for this API key; retry in a minute"}
}

// workspaceNotFound is the one answer for a workspace the caller cannot reach:
// nonexistent, without the key's membership, or switched off for MCP.
func workspaceNotFound() *ToolError {
	return &ToolError{Code: CodeNotFound, Message: "workspace not found"}
}

func internalError() *ToolError {
	return &ToolError{Code: CodeInternal, Message: "internal error"}
}

// toToolError maps what a service returned to a tool error. Wrapped errors
// are seen through. Anything unrecognised is internal, and its text is never
// sent: it can name tables, hosts or other workspaces.
func toToolError(err error) *ToolError {
	if err == nil {
		return nil
	}
	var toolErr *ToolError
	if errors.As(err, &toolErr) {
		return toolErr
	}
	if errors.Is(err, domain.ErrUserNotInWorkspace) {
		return workspaceNotFound()
	}
	var workspaceMissing *domain.ErrWorkspaceNotFound
	if errors.As(err, &workspaceMissing) {
		return workspaceNotFound()
	}
	var notLicensed *domain.ErrFeatureNotLicensed
	if errors.As(err, &notLicensed) {
		return &ToolError{
			Code:         CodeLicenceRequired,
			Message:      notLicensed.Message,
			Feature:      string(notLicensed.Feature),
			RequiredTier: notLicensed.RequiredTier,
		}
	}
	var permErr *domain.PermissionError
	if errors.As(err, &permErr) {
		return &ToolError{
			Code:       CodePermissionDenied,
			Message:    permErr.Message,
			Resource:   string(permErr.Resource),
			Permission: string(permErr.Permission),
		}
	}
	var unauthorized *domain.ErrUnauthorized
	if errors.As(err, &unauthorized) {
		return &ToolError{Code: CodePermissionDenied, Message: unauthorized.Message}
	}
	if message, ok := notFoundMessage(err); ok {
		return &ToolError{Code: CodeNotFound, Message: message}
	}
	var invalidState *domain.InvalidStateError
	if errors.As(err, &invalidState) {
		return &ToolError{Code: CodeInvalidState, Message: invalidState.Error(), Status: invalidState.Status}
	}
	var exists *domain.AlreadyExistsError
	if errors.As(err, &exists) {
		return &ToolError{Code: CodeConflict, Message: exists.Error()}
	}
	// A contact value a segment condition cannot convert. Only a caller holding
	// contacts:read reaches the query that raises it, so naming the contact
	// discloses nothing it could not read.
	var segmentValue *domain.SegmentValueError
	if errors.As(err, &segmentValue) {
		message, _ := CapText(segmentValue.Error())
		return &ToolError{Code: CodeValidation, Message: message}
	}
	var validation domain.ValidationError
	if errors.As(err, &validation) {
		return &ToolError{Code: CodeValidation, Message: validation.Message}
	}
	return internalError()
}

// notFoundMessage recognises the typed not-found errors of the services the
// tools call, and gives the message to send.
func notFoundMessage(err error) (string, bool) {
	var entity *domain.ErrNotFound
	if errors.As(err, &entity) {
		return entity.Error(), true
	}
	var list *domain.ErrListNotFound
	if errors.As(err, &list) {
		return list.Error(), true
	}
	var segment *domain.ErrSegmentNotFound
	if errors.As(err, &segment) {
		return "segment not found", true
	}
	var membership *domain.ErrContactListNotFound
	if errors.As(err, &membership) {
		return membership.Error(), true
	}
	var broadcast *domain.ErrBroadcastNotFound
	if errors.As(err, &broadcast) {
		return broadcast.Error(), true
	}
	var template *domain.ErrTemplateNotFound
	if errors.As(err, &template) {
		return template.Error(), true
	}
	if errors.Is(err, domain.ErrContactNotFound) {
		return "contact not found", true
	}
	return "", false
}
