package http

import (
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/Notifuse/notifuse/internal/http/middleware"
)

// webhookBodyReadTimeout is how long a public webhook body may take to arrive.
// Providers give up on a delivery well before this (SparkPost after 10 s,
// Supabase hooks after a few seconds), so it never cuts one short.
const webhookBodyReadTimeout = 30 * time.Second

// setWebhookBodyDeadline gives the body of a public webhook request timeout to
// arrive, from now; nothing when timeout is not positive. Call it first in the
// handler: net/http drains the body of a request refused without reading it
// before sending the answer, and the deadline bounds that drain too. The
// deadline is set through the server's own writer (see
// middleware.KeepServerWriter); readWebhookBody lifts it.
func setWebhookBodyDeadline(w http.ResponseWriter, r *http.Request, timeout time.Duration) {
	if timeout > 0 {
		_ = middleware.SetReadDeadline(w, r, time.Now().Add(timeout))
	}
}

// readWebhookBody reads the body of a public webhook request, refusing one
// larger than maxBytes with an *http.MaxBytesError: from its Content-Length
// before reading anything when the sender declares one, otherwise as soon as
// the body passes the limit. maxBytes must be above the 256 KiB net/http drains
// after a handler: it then gives up on a declared body over the limit without
// reading it, and leaves the client time to read the answer before closing.
//
// Once the body is read the deadline setWebhookBodyDeadline set is lifted.
// net/http watches the connection with a background read once a body is
// consumed and lifts the deadline itself when it starts that read at the end of
// a body, but for an empty body the read started before the handler ran: a
// deadline left on it would cancel the request context while the handler is
// still working.
//
// On error the deadline is moved to the past. net/http drains up to 256 KiB of
// an unread body after the handler, whatever the response says, and that drain
// must not wait on a client that stopped sending. Answer the request with
// writeWebhookBodyError.
func readWebhookBody(w http.ResponseWriter, r *http.Request, maxBytes int64) ([]byte, error) {
	if r.ContentLength > maxBytes {
		_ = middleware.SetReadDeadline(w, r, time.Unix(1, 0))
		return nil, &http.MaxBytesError{Limit: maxBytes}
	}

	body, err := io.ReadAll(middleware.MaxBytesReader(w, r, maxBytes))
	if err != nil {
		_ = middleware.SetReadDeadline(w, r, time.Unix(1, 0))
		return nil, err
	}
	_ = middleware.SetReadDeadline(w, r, time.Time{})
	return body, nil
}

// writeWebhookBodyError answers a request whose body readWebhookBody refused:
// 413 when it was too large, 400 otherwise. On HTTP/1.x the connection is
// closed after the response rather than drained for the next request; HTTP/2
// would close every stream of the connection, and the refused stream's body
// is already ended.
func writeWebhookBodyError(w http.ResponseWriter, r *http.Request, err error) {
	if r.ProtoMajor == 1 {
		w.Header().Set("Connection", "close")
	}
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		WriteJSONError(w, "Request body too large", http.StatusRequestEntityTooLarge)
		return
	}
	WriteJSONError(w, "Failed to read request body", http.StatusBadRequest)
}
