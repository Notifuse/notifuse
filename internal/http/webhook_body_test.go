package http

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/http/middleware"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// countingReader records how many bytes were read from it.
type countingReader struct {
	r    io.Reader
	read int
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.read += n
	return n, err
}

func TestReadWebhookBody_ReturnsABodyOfExactlyTheLimit(t *testing.T) {
	body := bytes.Repeat([]byte("a"), 64)
	req := httptest.NewRequest(http.MethodPost, "/webhooks/email", bytes.NewReader(body))

	got, err := readWebhookBody(httptest.NewRecorder(), req, 64)

	require.NoError(t, err)
	assert.Equal(t, body, got)
}

func TestReadWebhookBody_RefusesADeclaredLengthOverTheLimitWithoutReading(t *testing.T) {
	src := &countingReader{r: bytes.NewReader(bytes.Repeat([]byte("a"), 65))}
	req := httptest.NewRequest(http.MethodPost, "/webhooks/email", src)
	req.ContentLength = 65

	got, err := readWebhookBody(httptest.NewRecorder(), req, 64)

	var tooLarge *http.MaxBytesError
	require.ErrorAs(t, err, &tooLarge)
	assert.Equal(t, int64(64), tooLarge.Limit)
	assert.Nil(t, got)
	assert.Equal(t, 0, src.read, "a body declared too large must not be read at all")
}

func TestReadWebhookBody_RefusesAnUndeclaredBodyOverTheLimit(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/webhooks/email", bytes.NewReader(bytes.Repeat([]byte("a"), 65)))
	req.ContentLength = -1 // chunked: the size is only known by reading

	got, err := readWebhookBody(httptest.NewRecorder(), req, 64)

	var tooLarge *http.MaxBytesError
	require.ErrorAs(t, err, &tooLarge)
	assert.Nil(t, got)
}

func TestReadWebhookBody_PassesReadErrorsThrough(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/webhooks/email", nil)
	req.Body = io.NopCloser(&errorReader{})

	_, err := readWebhookBody(httptest.NewRecorder(), req, 64)

	require.Error(t, err)
	var tooLarge *http.MaxBytesError
	assert.False(t, errors.As(err, &tooLarge))
}

func TestWriteWebhookBodyError(t *testing.T) {
	cases := []struct {
		name    string
		err     error
		status  int
		message string
	}{
		{"too large", &http.MaxBytesError{Limit: 64}, http.StatusRequestEntityTooLarge, "Request body too large"},
		{"too large, wrapped", fmt.Errorf("reading: %w", &http.MaxBytesError{Limit: 64}), http.StatusRequestEntityTooLarge, "Request body too large"},
		{"other read error", errors.New("connection reset"), http.StatusBadRequest, "Failed to read request body"},
		{"deadline", os.ErrDeadlineExceeded, http.StatusBadRequest, "Failed to read request body"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()

			writeWebhookBodyError(w, httptest.NewRequest(http.MethodPost, "/webhooks/email", nil), tc.err)

			assert.Equal(t, tc.status, w.Code)
			assert.Equal(t, "close", w.Header().Get("Connection"))
			var response map[string]string
			require.NoError(t, json.NewDecoder(w.Body).Decode(&response))
			assert.Equal(t, tc.message, response["error"])
		})
	}

	t.Run("HTTP/2", func(t *testing.T) {
		// HTTP/2 turns Connection: close into a GOAWAY for the whole connection.
		req := httptest.NewRequest(http.MethodPost, "/webhooks/email", nil)
		req.Proto, req.ProtoMajor, req.ProtoMinor = "HTTP/2.0", 2, 0
		w := httptest.NewRecorder()

		writeWebhookBodyError(w, req, &http.MaxBytesError{Limit: 64})

		assert.Equal(t, http.StatusRequestEntityTooLarge, w.Code)
		assert.Empty(t, w.Header().Get("Connection"))
	})
}

// webhookBodyHandler sets the body deadline and reads the body as the webhook
// handlers do, answering 200 with the body length or the body error.
func webhookBodyHandler(maxBytes int64, timeout time.Duration, afterRead func(r *http.Request) int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		setWebhookBodyDeadline(w, r, timeout)
		body, err := readWebhookBody(w, r, maxBytes)
		if err != nil {
			writeWebhookBodyError(w, r, err)
			return
		}
		status := http.StatusOK
		if afterRead != nil {
			status = afterRead(r)
		}
		w.WriteHeader(status)
		_, _ = fmt.Fprintf(w, "%d", len(body))
	}
}

// webhookBodyServer serves webhookBodyHandler on a real connection, where the
// read deadline applies.
func webhookBodyServer(t *testing.T, maxBytes int64, timeout time.Duration, afterRead func(r *http.Request) int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(webhookBodyHandler(maxBytes, timeout, afterRead))
	t.Cleanup(srv.Close)
	return srv
}

// rawRequest sends head (and body bytes, if any) on a raw connection the client
// never closes, and returns the response and how long the server took to close
// the connection after it.
func rawRequest(t *testing.T, srv *httptest.Server, head string, body []byte) (*http.Response, time.Duration, error) {
	t.Helper()
	conn, err := net.Dial("tcp", srv.Listener.Addr().String())
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	require.NoError(t, conn.SetDeadline(time.Now().Add(5*time.Second)))

	_, err = io.WriteString(conn, head)
	require.NoError(t, err)
	if len(body) > 0 {
		_, err = conn.Write(body)
		require.NoError(t, err)
	}

	reader := bufio.NewReader(conn)
	resp, err := http.ReadResponse(reader, nil)
	require.NoError(t, err)
	_, _ = io.ReadAll(resp.Body)
	_ = resp.Body.Close()

	start := time.Now()
	_, closeErr := reader.ReadByte()
	return resp, time.Since(start), closeErr
}

func TestReadWebhookBody_StalledBodyIsAnsweredAndTheConnectionClosed(t *testing.T) {
	srv := webhookBodyServer(t, 1<<20, 200*time.Millisecond, nil)

	start := time.Now()
	resp, closedAfter, closeErr := rawRequest(t, srv,
		"POST / HTTP/1.1\r\nHost: test\r\nContent-Length: 1024\r\n\r\n", []byte("0123456789"))

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	assert.True(t, resp.Close, "the response says Connection: close")
	assert.GreaterOrEqual(t, time.Since(start), 200*time.Millisecond, "the body gets its full deadline")
	assert.ErrorIs(t, closeErr, io.EOF, "the server closes the connection instead of waiting for the rest of the body")
	assert.Less(t, closedAfter, 2*time.Second)
}

func TestReadWebhookBody_DeclaredLengthOverTheLimitIsRefusedWithoutWaitingForTheBody(t *testing.T) {
	srv := webhookBodyServer(t, 1<<20, 200*time.Millisecond, nil)

	resp, closedAfter, closeErr := rawRequest(t, srv,
		fmt.Sprintf("POST / HTTP/1.1\r\nHost: test\r\nContent-Length: %d\r\n\r\n", (1<<20)+1), nil)

	assert.Equal(t, http.StatusRequestEntityTooLarge, resp.StatusCode)
	assert.True(t, resp.Close, "the response says Connection: close")
	assert.ErrorIs(t, closeErr, io.EOF)
	assert.Less(t, closedAfter, 2*time.Second)
}

func TestReadWebhookBody_ChunkedBodyThatStallsPastTheLimitIsRefusedAtOnce(t *testing.T) {
	// The body passes the limit and the client then stops without the final
	// chunk. net/http tries to drain what follows; that must not wait for the
	// 30 s the body had to arrive.
	srv := webhookBodyServer(t, 1024, 30*time.Second, nil)

	start := time.Now()
	resp, closedAfter, closeErr := rawRequest(t, srv,
		"POST / HTTP/1.1\r\nHost: test\r\nTransfer-Encoding: chunked\r\n\r\n800\r\n", bytes.Repeat([]byte("a"), 1100))

	assert.Equal(t, http.StatusRequestEntityTooLarge, resp.StatusCode)
	assert.ErrorIs(t, closeErr, io.EOF)
	assert.Less(t, time.Since(start), 2*time.Second)
	assert.Less(t, closedAfter, 2*time.Second)
}

func TestReadWebhookBody_ChunkedBodyOverTheLimitIsRefused(t *testing.T) {
	srv := webhookBodyServer(t, 1<<20, 2*time.Second, nil)

	req, err := http.NewRequest(http.MethodPost, srv.URL, io.MultiReader(bytes.NewReader(bytes.Repeat([]byte("a"), (1<<20)+1))))
	require.NoError(t, err)
	req.ContentLength = -1
	resp, err := srv.Client().Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusRequestEntityTooLarge, resp.StatusCode)
}

func TestReadWebhookBody_DeadlineIsLiftedOnceTheBodyIsRead(t *testing.T) {
	// The handler outlives the read deadline after reading the body. For an
	// empty body net/http's background read is already waiting on the
	// connection, and a deadline left on it would cancel the request context.
	for name, body := range map[string]string{"empty body": "", "body": `{"event":"delivered"}`} {
		t.Run(name, func(t *testing.T) {
			var ctxErr error
			srv := webhookBodyServer(t, 1<<20, 100*time.Millisecond, func(r *http.Request) int {
				time.Sleep(400 * time.Millisecond)
				ctxErr = r.Context().Err()
				return http.StatusOK
			})

			resp, err := srv.Client().Post(srv.URL, "application/json", strings.NewReader(body))
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			assert.Equal(t, http.StatusOK, resp.StatusCode)
			assert.NoError(t, ctxErr)
		})
	}
}

func TestSetWebhookBodyDeadline_BoundsTheBodyOfARequestRefusedBeforeReadingIt(t *testing.T) {
	// net/http drains the unread body of a refused request before sending the
	// answer, so the deadline set at the start of the handler must bound it.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		setWebhookBodyDeadline(w, r, 200*time.Millisecond)
		WriteJSONError(w, "Provider is required", http.StatusBadRequest)
	}))
	t.Cleanup(srv.Close)

	start := time.Now()
	resp, closedAfter, closeErr := rawRequest(t, srv,
		"POST / HTTP/1.1\r\nHost: test\r\nContent-Length: 1024\r\n\r\n", []byte("0123456789"))

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	assert.Less(t, time.Since(start), 2*time.Second)
	assert.ErrorIs(t, closeErr, io.EOF, "the server closes the connection instead of waiting for the rest of the body")
	assert.Less(t, closedAfter, 2*time.Second)
}

func TestReadWebhookBody_HTTP2ConnectionOutlivesARefusedBody(t *testing.T) {
	srv := httptest.NewUnstartedServer(webhookBodyHandler(64, 2*time.Second, nil))
	srv.EnableHTTP2 = true
	srv.StartTLS()
	t.Cleanup(srv.Close)
	client := srv.Client()

	post := func(body []byte) (*http.Response, bool) {
		t.Helper()
		var reused bool
		trace := &httptrace.ClientTrace{GotConn: func(info httptrace.GotConnInfo) { reused = info.Reused }}
		req, err := http.NewRequestWithContext(httptrace.WithClientTrace(context.Background(), trace),
			http.MethodPost, srv.URL, bytes.NewReader(body))
		require.NoError(t, err)
		resp, err := client.Do(req)
		require.NoError(t, err)
		_, _ = io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		return resp, reused
	}

	refused, _ := post(bytes.Repeat([]byte("a"), 65))
	require.Equal(t, 2, refused.ProtoMajor)
	assert.Equal(t, http.StatusRequestEntityTooLarge, refused.StatusCode)

	next, reused := post([]byte("{}"))
	assert.Equal(t, http.StatusOK, next.StatusCode)
	assert.True(t, reused, "the next request reuses the connection")
}

func TestReadWebhookBody_ChunkedBodyOverTheLimitIsAnsweredBehindTracing(t *testing.T) {
	// Tracing hides the server's writer. net/http must still learn the body
	// passed the limit, or it closes the connection on the unread rest of the
	// body at once and the client gets a reset instead of the 413.
	srv := httptest.NewServer(middleware.KeepServerWriter(middleware.TracingMiddleware(webhookBodyHandler(1024, 2*time.Second, nil))))
	t.Cleanup(srv.Close)

	for i := 0; i < 5; i++ {
		req, err := http.NewRequest(http.MethodPost, srv.URL, io.MultiReader(bytes.NewReader(bytes.Repeat([]byte("a"), 4<<20))))
		require.NoError(t, err)
		req.ContentLength = -1
		resp, err := srv.Client().Do(req)
		require.NoError(t, err)
		_ = resp.Body.Close()

		assert.Equal(t, http.StatusRequestEntityTooLarge, resp.StatusCode)
	}
}

func TestReadWebhookBody_DeclaredLengthOverTheLimitIsAnsweredBeforeClosing(t *testing.T) {
	// The body is refused unread. A client sending it anyway must still read
	// the 413 before the server closes on the unread body, also when the
	// request says Connection: close or expects a 100 Continue it does not
	// wait for (the transport's ExpectContinueTimeout is 0).
	srv := httptest.NewServer(middleware.KeepServerWriter(middleware.TracingMiddleware(webhookBodyHandler(1<<20, 2*time.Second, nil))))
	t.Cleanup(srv.Close)

	for name, prepare := range map[string]func(req *http.Request){
		"Connection: close":    func(req *http.Request) { req.Close = true },
		"Expect: 100-continue": func(req *http.Request) { req.Header.Set("Expect", "100-continue") },
	} {
		t.Run(name, func(t *testing.T) {
			for i := 0; i < 5; i++ {
				req, err := http.NewRequest(http.MethodPost, srv.URL, bytes.NewReader(bytes.Repeat([]byte("a"), (1<<20)+1)))
				require.NoError(t, err)
				prepare(req)
				resp, err := srv.Client().Do(req)
				require.NoError(t, err)
				_ = resp.Body.Close()

				assert.Equal(t, http.StatusRequestEntityTooLarge, resp.StatusCode)
			}
		})
	}
}
