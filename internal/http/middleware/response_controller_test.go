package middleware

import (
	"bufio"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// hidingWriter wraps a ResponseWriter without Unwrap, as tracing's writer does.
type hidingWriter struct{ http.ResponseWriter }

func hideWriter(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(hidingWriter{w}, r)
	})
}

// stalledBodyRead sends a request whose body stops after a few bytes to a
// handler that sets a short read deadline through SetReadDeadline and then
// reads the body, and returns what SetReadDeadline and the read returned.
func stalledBodyRead(t *testing.T, wrap func(http.Handler) http.Handler) (setErr, readErr error) {
	t.Helper()
	done := make(chan struct{})
	srv := httptest.NewServer(wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(done)
		setErr = SetReadDeadline(w, r, time.Now().Add(100*time.Millisecond))
		_, readErr = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusBadRequest)
	})))
	t.Cleanup(srv.Close)

	conn, err := net.Dial("tcp", srv.Listener.Addr().String())
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	require.NoError(t, conn.SetDeadline(time.Now().Add(time.Second)))
	_, err = io.WriteString(conn, "POST / HTTP/1.1\r\nHost: test\r\nContent-Length: 1024\r\n\r\n0123456789")
	require.NoError(t, err)

	if resp, err := http.ReadResponse(bufio.NewReader(conn), nil); err == nil {
		_ = resp.Body.Close()
	}
	_ = conn.Close() // ends the read if no deadline was set
	<-done
	return setErr, readErr
}

func TestSetReadDeadline_ReachesTheConnectionBehindAWrapper(t *testing.T) {
	setErr, readErr := stalledBodyRead(t, func(h http.Handler) http.Handler {
		return KeepResponseController(hideWriter(h))
	})

	assert.NoError(t, setErr)
	assert.True(t, errors.Is(readErr, os.ErrDeadlineExceeded), "the body read stops at the deadline, got %v", readErr)
}

func TestSetReadDeadline_WithoutKeepResponseControllerUsesTheWriter(t *testing.T) {
	t.Run("hidden writer", func(t *testing.T) {
		setErr, _ := stalledBodyRead(t, hideWriter)

		assert.ErrorIs(t, setErr, http.ErrNotSupported)
	})

	t.Run("server writer", func(t *testing.T) {
		setErr, readErr := stalledBodyRead(t, func(h http.Handler) http.Handler { return h })

		assert.NoError(t, setErr)
		assert.True(t, errors.Is(readErr, os.ErrDeadlineExceeded), "the body read stops at the deadline, got %v", readErr)
	})
}
