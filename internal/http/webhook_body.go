package http

import (
	"errors"
	"io"
	"net/http"
	"time"
)

// webhookBodyReadTimeout is how long a public webhook body may take to arrive.
// Providers give up on a delivery well before this (SparkPost after 10 s,
// Supabase hooks after a few seconds), so it never cuts one short.
const webhookBodyReadTimeout = 30 * time.Second

// readWebhookBody reads the body of a public webhook request, refusing one
// larger than maxBytes with an *http.MaxBytesError: from its Content-Length
// before reading anything when the sender declares one, otherwise as soon as
// the body passes the limit.
//
// When timeout is positive the body must arrive within it. The read deadline is
// set on the connection, so it does not apply where the response writer gives
// no access to it (tracing wraps the writer that way). Once the body is read the
// deadline is lifted. net/http watches the connection with a background read
// once a body is consumed and lifts the deadline itself when it starts that read
// at the end of a body, but for an empty body the read started before the
// handler ran: a deadline left on it would cancel the request context while the
// handler is still working.
//
// On error the deadline is moved to the past. net/http drains up to 256 KiB of
// an unread body after the handler, whatever the response says, and that drain
// must not wait on a client that stopped sending. Answer the request with
// writeWebhookBodyError.
func readWebhookBody(w http.ResponseWriter, r *http.Request, maxBytes int64, timeout time.Duration) (body []byte, err error) {
	rc := http.NewResponseController(w)
	if r.ContentLength > maxBytes {
		if timeout > 0 {
			_ = rc.SetReadDeadline(time.Unix(1, 0))
		}
		return nil, &http.MaxBytesError{Limit: maxBytes}
	}

	var deadline time.Time
	if timeout > 0 {
		deadline = time.Now().Add(timeout)
		if rc.SetReadDeadline(deadline) != nil {
			deadline = time.Time{}
		}
	}

	body, err = io.ReadAll(http.MaxBytesReader(w, r.Body, maxBytes))
	if err != nil {
		if !deadline.IsZero() {
			_ = rc.SetReadDeadline(time.Unix(1, 0))
		}
		return nil, err
	}
	if !deadline.IsZero() {
		_ = rc.SetReadDeadline(time.Time{})
	}
	return body, nil
}

// writeWebhookBodyError answers a request whose body readWebhookBody refused:
// 413 when it was too large, 400 otherwise. The connection is closed after the
// response rather than drained for the next request.
func writeWebhookBodyError(w http.ResponseWriter, err error) {
	w.Header().Set("Connection", "close")
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		WriteJSONError(w, "Request body too large", http.StatusRequestEntityTooLarge)
		return
	}
	WriteJSONError(w, "Failed to read request body", http.StatusBadRequest)
}
