# Webhook request limits plan

**Status**: plan reviewed by Gemini (six rounds), implemented
**Date**: 2026-10-21
**Target**: 42.0 (unreleased). No migration, no version bump.
**Base**: `origin/dev` at `5af5221`.

## Goal

Keep the resources one request can take on the public webhook endpoints bounded, and give the HTTP
server limits on how long a connection may take to send its headers or stay idle.

Today:

- `/webhooks/email` reads the whole body with `io.ReadAll` before anything else
  (`internal/http/inbound_webhook_event_handler.go`, `handleIncomingWebhook`).
- `/webhooks/supabase/auth-email` and `/webhooks/supabase/before-user-created` do the same before the
  signature is checked (`internal/http/supabase_webhook_handler.go`).
- The `http.Server` built in `App.Start` (`internal/app/app.go`) sets no timeout at all.
- For the three providers that batch events (SendGrid, SparkPost, Mailjet with grouped events),
  every event of a batch gets the **whole batch** as its `raw_payload`
  (`string(rawPayload)` inside the per-event loop of `processSendGridWebhook`,
  `processSparkPostWebhook` and `processMailjetWebhook` → `processSingleMailjetEvent`,
  `internal/service/inbound_webhook_event_service.go`). A batch of N events therefore allocates N
  copies of the batch (`string([]byte)` copies) and stores N copies: memory and storage grow with
  N × batch size, so a body limit alone does not bound what one request costs. It also means an
  event's row holds the other recipients of its batch, so erasing a contact
  (`DeleteForEmail` clears `raw_payload` only on that contact's rows) leaves their address in the
  rows of the other events of the same batch.

`/webhooks/email/inbound` (replies) already caps its body at 25 MiB and is rate-limited; it is left as
is (see Out of scope).

## Limits and why these values

### `/webhooks/email`: 10 MiB

Real payloads, by provider:

| Provider | Shape | Largest documented size |
|---|---|---|
| SendGrid | JSON array, batched | posts every ~30 s or when the batch reaches 768 KB |
| Amazon SES | one SNS notification | SNS messages to HTTP/S endpoints are capped at 256 KiB; SES truncates mail headers above 10 KB |
| SparkPost | JSON array, batched | no published cap; SparkPost describes batches of "1 to 350 or more" events and asks receivers not to set an "excessively small" maximum. Notifuse sets one metadata key, so an event is about 2 KB |
| Mailjet | JSON array (event API version 2, grouped) | no published cap; events are about 1 KB |
| Postmark, Mailgun, SMTP relays | one event per request | a few KB (Notifuse does not ask Postmark to include bounce content) |

10 MiB is 13 times SendGrid's batch, 40 times the SNS cap, and about 5,000 SparkPost events, so
no legitimate delivery comes near it, while one request can no longer hold more than 10 MiB.

### `/webhooks/supabase/*`: 1 MiB

Both hooks carry one user object (metadata, identities) and, for the email hook, the email data:
a few KB. 1 MiB leaves two orders of magnitude.

### Per-event raw payload

Each event of a batch keeps its own element of the JSON array as `raw_payload`: the parsers decode the
body into `[]json.RawMessage`, then each element into the provider struct. Memory per request is then
proportional to the body (each element copied once), and storage per event to the event. Single-event
providers (SES, Postmark, Mailgun, SMTP, Mailjet's ungrouped object) are unchanged. The console renders
`JSON.parse(raw_payload)` per event, which an element still satisfies.

### Body read deadline: 30 s, on the same three routes

Without a server `ReadTimeout`, a body has no time limit. The three handlers give the body 30 s to
arrive, through `http.ResponseController.SetReadDeadline`, counted from the start of the handler,
before any validation. net/http drains the body of a request the handler refuses without reading it
(wrong method, missing query parameter or Supabase header), and the deadline bounds that drain too.

- **On success the deadline is lifted.** Once a body is consumed, net/http watches the connection
  with a background read, and a read error there cancels the request context
  (`connReader.backgroundRead` → `handleReadErrorLocked` → `cancelCtx`). When the body ends during the
  handler, net/http clears the deadline itself as it starts that read (`startBackgroundRead`). For an
  empty body, though, the read started before the handler ran, and the deadline set by the handler
  applies to it: left in place, it would cancel the context of a handler still working 30 s later.
  (Checked by experiment, and pinned by a test.)
- **On error the deadline is moved to the past, and on HTTP/1.x the response says `Connection: close`.** net/http
  drains up to 256 KiB of an unread body when it writes the response (`chunkWriter.writeHeader`) and
  again in `body.Close`, which runs whatever the response says. With the deadline cleared, that drain
  would wait on a stalled client with no limit; with the rest of the 30 s kept, a chunked body refused
  for its size after a few milliseconds would still hold the handler for the remaining time. A past
  deadline makes the drain fail at once, and `Connection: close` skips the first drain entirely.
  HTTP/2 turns a `Connection: close` response header into a GOAWAY for the whole connection, which a
  proxy may share among many clients, so it is not sent there; the past deadline already ends the
  stream's body.
- **A declared length over the limit is reported to net/http, without reading the body.** net/http
  only waits a moment before closing a connection with unread body data, so the client reads the
  answer before the kernel resets the connection, when its writer learned that a body passed a limit
  or when it gave up draining a declared body over 256 KiB. The second never happens for a request
  that says `Connection: close`, or `Expect: 100-continue` without waiting: the client would usually
  get a reset instead of the 413. `middleware.ReportBodyTooLarge(w, r)` reports the limit through
  the server's writer with a one-byte read of a zero-limit `http.MaxBytesReader` over its own
  one-byte reader, the only way net/http offers, leaving the request body untouched.

Providers time out well before 30 s themselves (SparkPost 10 s, Supabase hooks a few seconds), so the
deadline never cuts a delivery the provider would have completed. On HTTP/2 (direct TLS), the stream
deadline only closes the body pipe and does not cancel the context.

The deadline goes through the server's own `ResponseWriter`. With `TRACING_ENABLED=true`, OpenCensus'
`ochttp` hands the handler a writer without `Unwrap`, so `http.NewResponseController(w)` there returns
`http.ErrNotSupported` (adding `Unwrap` to `traceResponseWriter` would not help: `ochttp`'s writer sits
between it and the handler). The body limit needs it too: net/http only learns that a body passed
the limit through its own writer, and otherwise closes the connection on the unread rest of a chunked
body at once, so the client usually gets a reset instead of the 413. A middleware outermost in the
server chain, `middleware.KeepServerWriter`, saves the server's writer in the request context;
`middleware.SetReadDeadline(w, r, deadline)` and `middleware.MaxBytesReader(w, r, n)` use it, or `w`
when there is none. Only these two operations reach it, so no inner wrapper that buffers or counts
writes is bypassed. It costs a context value and a shallow copy of the request on every request, as
tracing and the auth middleware already do, and keeps working whatever wrapper is added later. The
writer is released when the handler returns: contexts derived from the request's
(`context.WithoutCancel` for background work) can outlive it, and must neither keep the connection
reachable nor act on it once it serves another request.

### Server: `ReadHeaderTimeout` 20 s, `IdleTimeout` 70 min

- `ReadHeaderTimeout: 20 * time.Second`. Also bounds the TLS handshake (net/http uses the smallest
  non-zero timeout for it) and is reset by net/http once the headers are read, so it never limits a
  body or a handler. On a keep-alive connection it only starts at the first byte of the next request.
- `IdleTimeout: 70 * time.Minute`. Longer than the idle reuse of the proxies Notifuse sits behind,
  up to the longest an operator can configure: Cloudflare reuses an origin connection for up to
  900 s, Google Cloud load balancers 600 s (620 s recommended on the backend), nginx 60 s, AWS ALB
  60 s by default and up to 4000 s, Envoy and Istio 1 h by default. The proxy then always closes an
  idle connection first and never sends a request on one the server is closing (AWS answers 502
  when it does). A shorter value would race with an ALB whose idle timeout was raised, or with a
  service mesh. Today idle keep-alive connections are never closed; a new connection already has
  20 s to send its first request (`ReadHeaderTimeout`).
- `ReadTimeout` and `WriteTimeout` stay unset, on purpose: `WriteTimeout` would cut streamed
  responses (LLM chat, setup wizard, SMTP test, audit export), and `ReadTimeout` would put one time
  limit on every request body of every route, including large API calls (contact imports, emails
  with attachments) sent over slow links.
- `MaxHeaderBytes` keeps net/http's 1 MiB default.
- The Prometheus metrics server (`pkg/tracing`, started when `PrometheusPort` is set) gets the same
  two timeouts. Prometheus reuses a scrape connection for at most a few minutes.

## Behaviour

- A declared `Content-Length` above the limit is refused with **413** before any byte is read;
  net/http then closes the connection without reading the body, once the client has had a moment to
  read the answer.
- A body without `Content-Length` (chunked) is read through `http.MaxBytesReader`; going past the limit
  answers **413**.
- Any other read error (including the 30 s deadline) keeps today's **400** "Failed to read request body".
- Both error answers carry `Connection: close` on HTTP/1.x, and none on HTTP/2.
- A request refused before its body is read (method, query parameters, Supabase headers) gets its
  answer once the body has arrived, or once the 30 s have passed, and the connection is then closed.
- 413 body: `{"error": "Request body too large"}` via `WriteJSONError`.
- For `before-user-created`, a 413 blocks the signup, like today's 400 on a read error; real payloads
  are far below 1 MiB.

## Implementation

1. **`internal/http/webhook_body.go`** (new)
   - `const webhookBodyReadTimeout = 30 * time.Second`
   - `setWebhookBodyDeadline(w http.ResponseWriter, r *http.Request, timeout time.Duration)`: the
     first statement of each handler; sets the read deadline `timeout` from now through
     `middleware.SetReadDeadline` when `timeout > 0` (ignores `ErrNotSupported`).
   - `readWebhookBody(w http.ResponseWriter, r *http.Request, maxBytes int64) ([]byte, error)`:
     Content-Length above `maxBytes` → `&http.MaxBytesError{Limit: maxBytes}` without reading;
     `io.ReadAll(middleware.MaxBytesReader(w, r, maxBytes))`; on error move the deadline to the past and
     return the error; on success clear the deadline.
   - `writeWebhookBodyError(w http.ResponseWriter, r *http.Request, err error)`: sets
     `Connection: close` when `r.ProtoMajor == 1`, then 413 for `*http.MaxBytesError` (also wrapped),
     else 400.
2. **`internal/http/inbound_webhook_event_handler.go`**
   - `const maxEmailWebhookBodyBytes = 10 << 20` with the provider sizes in its comment.
   - `bodyReadTimeout time.Duration` field, set to `webhookBodyReadTimeout` by the constructor
     (tests shorten it).
   - `handleIncomingWebhook` starts with `setWebhookBodyDeadline`, and uses `readWebhookBody` /
     `writeWebhookBodyError` at the place of today's `io.ReadAll`.
3. **`internal/http/supabase_webhook_handler.go`**
   - `const maxSupabaseWebhookBodyBytes = 1 << 20`, `bodyReadTimeout` field, both handlers start with
     `setWebhookBodyDeadline` and use the helpers.
4. **`internal/service/inbound_webhook_event_service.go`**
   - `processSendGridWebhook`, `processSparkPostWebhook`: decode into `[]json.RawMessage`, then each
     element; the event's raw payload is `string(element)`. SparkPost decodes each element into a value,
     not a pointer, so a `null` element is refused like any element without `message_event` instead of
     dereferencing a nil pointer as today.
   - `processMailjetWebhook`: the array branch decodes into `[]json.RawMessage` and passes each element
     to `processSingleMailjetEvent` (its logic unchanged). A non-array body keeps the single-object path.
   - `ProcessWebhook` and the other parsers are not touched (the provider-event thread works there).
5. **`internal/app/app.go`**
   - `newHTTPServer(addr string, handler http.Handler) *http.Server` sets `ReadHeaderTimeout` and
     `IdleTimeout`, with the comments above; `Start` uses it.
   - `(a *App) serverHandler() http.Handler`: the middleware chain `Start` builds today (graceful
     shutdown, tracing when enabled, CORS) moved into a method, so a test can check it, with
     `middleware.KeepServerWriter` added outermost.
6. **`internal/http/middleware/server_writer.go`** (new): `KeepServerWriter(next)`,
   `SetReadDeadline(w, r, deadline)`, `MaxBytesReader(w, r, n)` and `ReportBodyTooLarge(w, r)`, as
   described above; `readWebhookBody` calls `ReportBodyTooLarge` on the declared-length refusal.
7. **`pkg/tracing/tracing.go`**: `newMetricsServer(port int, handler http.Handler) *http.Server`
   with `ReadHeaderTimeout` 20 s and `IdleTimeout` 70 min, used by `initPrometheusExporter`.
8. **`CHANGELOG.md`**: one `**Security**:` bullet under 42.0 saying what is now refused and that a
   batched event keeps its own payload. Nothing for operators to do.
9. **`../docs`** (own PR): `features/logs.mdx` says the webhook event log stores "the complete,
   unmodified webhook data"; it now says each event keeps its data as the provider sent it, a batched
   event its own entry of the batch. No setting or documented limit to update.

## Tests (written first, seen red, then green)

| Implementation file | Test file | Cases |
|---|---|---|
| `internal/http/webhook_body.go` | `internal/http/webhook_body_test.go` | body of exactly `maxBytes` returned whole; `maxBytes+1` with Content-Length → `*http.MaxBytesError` and 0 bytes read from the body; `maxBytes+1` chunked (ContentLength -1) → `*http.MaxBytesError`; read error passed through; `writeWebhookBodyError` → 413 + JSON + `Connection: close` for `MaxBytesError` (also wrapped), 400 + `Connection: close` otherwise, and no `Connection` header for an HTTP/2 request. Over a real `httptest.Server` with a raw `net.Conn` client: a stalled body (Content-Length 1024, 10 bytes sent) gets its 400 within the deadline and the server then closes the connection while the client keeps it open; a chunked body that passes the limit and then stalls (no final chunk) gets 413 and the connection closed well before the 30 s deadline; a declared Content-Length above the limit gets 413 and the connection closed with no body byte sent; a handler that sleeps past the deadline after reading an empty body, or a non-empty one, still has a live `r.Context()`; a handler that sets the deadline and refuses the request without reading a stalled body answers within the deadline and the connection is closed. Over HTTP/2 (TLS `httptest` server): after a 413 the next request reuses the same connection. Behind `KeepServerWriter` and the real `TracingMiddleware`, chunked bodies over the limit are answered 413, repeatedly, without a connection reset, and so are bodies declared over the limit sent with `Connection: close` or with `Expect: 100-continue` by a client that does not wait |
| `internal/http/inbound_webhook_event_handler.go` | `internal/http/inbound_webhook_event_handler_test.go` | body of exactly `10 << 20` bytes reaches `ProcessWebhook` unchanged; `10<<20 + 1` → 413, `ProcessWebhook` not called (Content-Length and chunked); existing `BodyReadError` still 400; over a real server with a short `bodyReadTimeout`: stalled body → 400 without calling the service; a stalled body on a request refused for its method or a missing `provider` → answered and closed within the deadline; `ProcessWebhook` slower than the deadline still sees a live context and answers 200 |
| `internal/http/supabase_webhook_handler.go` | `internal/http/supabase_webhook_handler_test.go` | both routes: `1<<20 + 1` → 413 (Content-Length and chunked) before the service is used; stalled body → 400; a stalled body on a request without the `webhook-*` headers → answered and closed within the deadline |
| `internal/service/inbound_webhook_event_service.go` | `internal/service/inbound_webhook_event_service_test.go` | SendGrid, SparkPost and grouped Mailjet batches of two events: each event's `RawPayload` is its own element (equal to the element's JSON, not containing the other recipient); a single Mailjet object still stores the whole body; a malformed element still fails the batch as today |
| `internal/app/app.go` | `internal/app/app_test.go` | `newHTTPServer` sets `ReadHeaderTimeout == 20*time.Second`, `IdleTimeout == 70*time.Minute`, `ReadTimeout == 0`, `WriteTimeout == 0`, and keeps `Addr`/`Handler`; `serverHandler()` on a real server, around the audit middleware and a probe route, lets the probe's `middleware.SetReadDeadline` succeed, with tracing off and on |
| `internal/http/middleware/server_writer.go` | `internal/http/middleware/server_writer_test.go` | behind a wrapper that hides the server's writer, `SetReadDeadline` succeeds under `KeepServerWriter` and the deadline applies to the connection (a stalled body read fails); without it, it falls back to the handler's writer (`ErrNotSupported` behind the wrapper, success on the server's writer); `MaxBytesReader` past its limit behind the wrapper under `KeepServerWriter`, and on the server's writer, makes net/http close the connection after the answer although the whole body arrived; so does `ReportBodyTooLarge` on a short body never read; a context kept from a finished request no longer reaches the server's writer (`SetReadDeadline` through it falls back to the given writer) |
| `pkg/tracing/tracing.go` | `pkg/tracing/tracing_test.go` | `newMetricsServer` sets `Addr` from the port, the handler, `ReadHeaderTimeout == 20*time.Second` and `IdleTimeout == 70*time.Minute` |
| full stack | `tests/integration/webhook_request_limits_test.go` (new) | SendGrid integration: a body of exactly 10 MiB (one real delivered event, padded with JSON whitespace) → 200 and the message is marked delivered; 10 MiB + 1 → 413 and another message stays untouched, with and without Content-Length; a two-event batch stores each event's own payload. Supabase `before-user-created`: a signed payload of exactly 1 MiB (whitespace padding) → 204 and the contact is created; 1 MiB + 1 → 413 and no contact; `auth-email` 1 MiB + 1 → 413 |

Sizes in tests are literals (`10 << 20`), not the constants they protect.

`internal/http/setup_handler_test.go`: the shutdown mock's flag becomes an `atomic.Bool`. The settings
handler calls it from a goroutine 500 ms after answering, and a settings test answers twice, so two
of those goroutines write the flag together. The new tests in the package keep the test binary alive
past those 500 ms, which lets `-race` see it.

## Out of scope

- `/webhooks/email/inbound`: already capped at 25 MiB and rate-limited per IP and per workspace. A body
  deadline there needs a longer value than 30 s for 25 MiB bodies; separate change.
- Per-IP rate limiting on `/webhooks/email`: providers send bursts from a few addresses during a
  broadcast, and a 429 would delay or drop real bounces.
- Size caps and body deadlines on the other public endpoints (sign-in, notification center): separate change.
- Other routes: a request their handler refuses before reading its body (a missing token on `/api/*`)
  still lets net/http drain up to 256 KiB of it with no deadline. That is common to every route and
  needs a server-wide mechanism.
- Webhook events stored before this version keep the whole batch as `raw_payload`. Contact erasure
  clears rows by `recipient_email`, so an erased address can remain in another recipient's older row.
  Rewriting stored payloads is a separate change.
- The three batch parsers repeat a four-line decode (array of `json.RawMessage`, then each element); a
  generic helper would save little and they differ in error wording and Mailjet's fallback.
- Provider authentication on `/webhooks/email`: separate change.
- `/webhooks/email` answers 400 to any processing failure, transient ones included, and SNS does not
  retry a 4xx: separate change, since it changes how every provider retries.

## Final step

```
go test ./internal/http/... ./internal/service/ ./internal/app/...
make test-unit
INTEGRATION_TESTS=true go test -tags integration,licdev ./tests/integration -run 'TestWebhookRequestLimits|TestSESBounceHandling|TestSupabaseIntegrationE2E|TestMailjetUnsubscribeWebhook|TestContactDeletion' -v
make test-integration
```
