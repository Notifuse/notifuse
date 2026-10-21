package middleware

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"time"
)

// serverWriterKey is the request context key of the writer saved by
// KeepServerWriter.
type serverWriterKey struct{}

// KeepServerWriter saves the server's own ResponseWriter in the request
// context, for SetReadDeadline, MaxBytesReader and ReportBodyTooLarge. Install
// it outermost: writers further in can hide it (tracing's has no Unwrap), and
// net/http only acts through its own writer. The writer is released when the
// handler returns, since contexts derived from the request's can outlive it.
func KeepServerWriter(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		saved := new(atomic.Pointer[http.ResponseWriter])
		saved.Store(&w)
		defer saved.Store(nil)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), serverWriterKey{}, saved)))
	})
}

// serverWriter returns the writer KeepServerWriter saved for r, or w when there
// is none.
func serverWriter(w http.ResponseWriter, r *http.Request) http.ResponseWriter {
	if saved, ok := r.Context().Value(serverWriterKey{}).(*atomic.Pointer[http.ResponseWriter]); ok {
		if sw := saved.Load(); sw != nil {
			return *sw
		}
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

// ReportBodyTooLarge tells net/http, through the server's own writer, that the
// request body is over a limit, without reading it, as MaxBytesReader does
// when a read passes its limit. Reading one byte of a separate reader through
// a zero-limit http.MaxBytesReader is the only way net/http offers.
func ReportBodyTooLarge(w http.ResponseWriter, r *http.Request) {
	_, _ = http.MaxBytesReader(serverWriter(w, r), io.NopCloser(strings.NewReader("x")), 0).Read(make([]byte, 1))
}
