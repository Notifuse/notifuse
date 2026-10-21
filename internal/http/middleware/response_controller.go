package middleware

import (
	"context"
	"net/http"
	"time"
)

// responseControllerKey is the request context key of the controller saved by
// KeepResponseController.
type responseControllerKey struct{}

// KeepResponseController saves a controller for the server's own
// ResponseWriter in the request context, for SetReadDeadline. Install it
// outermost: writers further in can hide the connection from
// http.NewResponseController (tracing's has no Unwrap).
func KeepResponseController(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), responseControllerKey{}, http.NewResponseController(w))
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// SetReadDeadline sets the read deadline of the request's connection (of its
// stream, in HTTP/2) through the controller KeepResponseController saved, or
// through w when there is none.
func SetReadDeadline(w http.ResponseWriter, r *http.Request, deadline time.Time) error {
	if rc, ok := r.Context().Value(responseControllerKey{}).(*http.ResponseController); ok {
		return rc.SetReadDeadline(deadline)
	}
	return http.NewResponseController(w).SetReadDeadline(deadline)
}
