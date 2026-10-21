package middleware

import (
	"context"
	"io"
	"net/http"
	"time"
)

// serverWriterKey is the request context key of the writer saved by
// KeepServerWriter.
type serverWriterKey struct{}

// KeepServerWriter saves the server's own ResponseWriter in the request
// context, for SetReadDeadline and MaxBytesReader. Install it outermost:
// writers further in can hide it (tracing's has no Unwrap), and net/http only
// acts through its own writer.
func KeepServerWriter(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), serverWriterKey{}, w)))
	})
}

// serverWriter returns the writer KeepServerWriter saved for r, or w when there
// is none.
func serverWriter(w http.ResponseWriter, r *http.Request) http.ResponseWriter {
	if sw, ok := r.Context().Value(serverWriterKey{}).(http.ResponseWriter); ok {
		return sw
	}
	return w
}

// SetReadDeadline sets the read deadline of the request's connection (of its
// stream, in HTTP/2) through the server's own writer.
func SetReadDeadline(w http.ResponseWriter, r *http.Request, deadline time.Time) error {
	return http.NewResponseController(serverWriter(w, r)).SetReadDeadline(deadline)
}

// MaxBytesReader is http.MaxBytesReader on the request body, reporting to the
// server's own writer: net/http then closes the connection after the answer to
// a body over the limit, leaving the client time to read the answer first.
func MaxBytesReader(w http.ResponseWriter, r *http.Request, n int64) io.ReadCloser {
	return http.MaxBytesReader(serverWriter(w, r), r.Body, n)
}
