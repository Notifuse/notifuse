package middleware

import (
	"bufio"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
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
		return KeepServerWriter(hideWriter(h))
	})

	assert.NoError(t, setErr)
	assert.True(t, errors.Is(readErr, os.ErrDeadlineExceeded), "the body read stops at the deadline, got %v", readErr)
}

func TestSetReadDeadline_WithoutKeepServerWriterUsesTheHandlerWriter(t *testing.T) {
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

func TestMaxBytesReader_TellsTheServerWriterBehindAWrapper(t *testing.T) {
	// net/http closes the connection after the answer, waiting a moment so the
	// client reads it, only when its own writer learns the body passed the
	// limit. Here the whole body has arrived, so without that net/http would
	// drain it and keep the connection open.
	for name, wrap := range map[string]func(http.Handler) http.Handler{
		"kept behind a wrapper": func(h http.Handler) http.Handler { return KeepServerWriter(hideWriter(h)) },
		"server writer":         func(h http.Handler) http.Handler { return h },
	} {
		t.Run(name, func(t *testing.T) {
			var readErr error
			srv := httptest.NewServer(wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, readErr = io.ReadAll(MaxBytesReader(w, r, 1024))
				w.WriteHeader(http.StatusRequestEntityTooLarge)
			})))
			t.Cleanup(srv.Close)

			req, err := http.NewRequest(http.MethodPost, srv.URL, io.MultiReader(strings.NewReader(strings.Repeat("a", 2048))))
			require.NoError(t, err)
			req.ContentLength = -1
			resp, err := srv.Client().Do(req)
			require.NoError(t, err)
			_ = resp.Body.Close()

			var tooLarge *http.MaxBytesError
			assert.ErrorAs(t, readErr, &tooLarge)
			assert.True(t, resp.Close, "net/http closes the connection after a body over the limit")
		})
	}
}
