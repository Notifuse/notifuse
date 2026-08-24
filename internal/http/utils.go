package http

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/Notifuse/notifuse/internal/domain"
)

// WriteJSONError writes a JSON error response with the given message and status code.
// It sets the Content-Type header to application/json and automatically formats
// the response as {"error": "message"}.
func WriteJSONError(w http.ResponseWriter, message string, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error": message,
	})
}

// writeJSON writes a JSON response with the given status code and data.
// It sets the Content-Type header to application/json.
func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writePermissionError answers 403 when err is, or wraps, a *domain.PermissionError,
// reporting whether it wrote the response. errors.As rather than a type assertion:
// services wrap errors on their way up — the authenticate step that sits one line
// above every permission check already does — and a bare assertion would silently
// degrade a wrapped denial into an opaque 500.
// The body carries the resource and permission alongside the message, so a client
// can tell which grant is missing without parsing prose. Both fields are additive:
// the "error" key keeps the shape every existing caller already reads.
func writePermissionError(w http.ResponseWriter, err error) bool {
	var permErr *domain.PermissionError
	if errors.As(err, &permErr) {
		writeJSON(w, http.StatusForbidden, map[string]interface{}{
			"error":      permErr.Error(),
			"resource":   permErr.Resource,
			"permission": permErr.Permission,
		})
		return true
	}
	return false
}

// writeServiceError maps the authorization and lookup errors a service can return
// to HTTP status codes, writing the response and reporting whether it handled the
// error. A permission denial or an authorization failure (not a member / not an
// owner) answers 403 rather than a generic 500, and a missing workspace answers 404.
// It unwraps via errors.As/errors.Is, so it still matches when the service wrapped
// the error on its way up (e.g. "failed to authenticate user: %w").
//
// fallback is the message sent when the matched denial carries none of its own —
// an ErrUnauthorized built without a Message would otherwise answer 403 with an
// empty error string.
//
// Unrecognized errors are left untouched and reported as unhandled, so the caller
// keeps its own mapping (usually a method-specific 500).
func writeServiceError(w http.ResponseWriter, err error, fallback string) bool {
	if writePermissionError(w, err) {
		return true
	}
	var notFound *domain.ErrWorkspaceNotFound
	if errors.As(err, &notFound) {
		WriteJSONError(w, "Workspace not found", http.StatusNotFound)
		return true
	}
	var unauthorized *domain.ErrUnauthorized
	if errors.As(err, &unauthorized) {
		message := unauthorized.Message
		if message == "" {
			message = fallback
		}
		WriteJSONError(w, message, http.StatusForbidden)
		return true
	}
	if errors.Is(err, domain.ErrUserNotInWorkspace) {
		WriteJSONError(w, "You do not have access to this workspace", http.StatusForbidden)
		return true
	}
	return false
}
