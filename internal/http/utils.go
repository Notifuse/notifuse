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
func writePermissionError(w http.ResponseWriter, err error) bool {
	var permErr *domain.PermissionError
	if errors.As(err, &permErr) {
		WriteJSONError(w, permErr.Error(), http.StatusForbidden)
		return true
	}
	return false
}
