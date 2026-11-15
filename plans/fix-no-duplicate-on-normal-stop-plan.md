# Fix: no email, webhook or broadcast batch sent twice on a normal stop

A normal stop (SIGTERM, a deploy, a restart after saving SMTP or OIDC settings) must not cut work that may
already have reached its receiver. Three places still do:

- **Automation webhook node (architecture F-80).** On a stop, the node under way keeps its context for
  `automationShutdownDrain` (20 s, `automation_executor.go:84`), but the webhook node's client waits up to
  30 s (`newOutgoingWebhookClient(..., 30*time.Second)` at `app.go:1410`, and the fallback at
  `automation_node_executor.go:1157`). A call whose endpoint answers between 20 and 30 s is cut, the journey
  stays on the node, and the call is posted again once the claim ends.
- **Email queue (architecture F-16).** `EmailQueueWorker.Stop` waits `ShutdownGrace` (10 s), cancels the sends,
  waits `AbortWait` (5 s) and returns (`worker.go:299-334`). A cut send leaves its row `processing`, and the row
  is sent again two minutes after the restart. Sends are not bounded below the grace:
  - the HTTP providers share a client with a 10 s timeout (`app.go:886-903`), and the stop-on-reply guard and
    the pre-send `message_history` upsert run before it on the same `sendCtx`;
  - SES (AWS SDK v2, `awshttp.NewBuildableClient`) has no overall timeout, and retries;
  - SMTP (`sendRawEmailWithSettings`) ignores its context: the dial has 30 s, each TLS handshake 30 s, the
    OAuth2 token request 30 s, and every SMTP command after the handshake has no deadline at all
    (`smtp_service.go:287`, `:341`, `:488`).
- **Broadcast runs started outside the scheduler (part of architecture F-30).** "Send now", `SelectWinner`
  and a resume with `start_now` run the task through `RunTaskNow` on `context.Background()`
  (`task_service.go:1203-1223`), and `/api/cron` and the HTTP-dispatched `tasks.execute` run on
  `context.WithoutCancel(r.Context())` (`task_handler.go:424`, `:564`). The stop neither cancels nor waits for
  them. One cut between the commit of `Enqueue` and the `SaveState` that follows leaves the cursor behind, and
  the next run enqueues that batch again with new message IDs: those recipients get the broadcast twice.
  Scheduler-started runs are safe already: their context is cancelled at the stop, so `Enqueue` fails or rolls
  back, and the save runs on `leaseContext` (`orchestrator.go:572-583`).

Today the stops run one after the other in `App.Shutdown` (`app.go:1974-2030`): web analytics (10 s), task
scheduler (5 s), automation scheduler (29 s), queue (10 + 5 s), SMTP bridge (10 s), 69 s of the 70 s
`gracefulShutdownTimeout` (`main.go:22`). Note that the queue and the automation batch already stop *claiming*
at the start of the stop (T0), since both run on the app's shutdown context, cancelled at `app.go:1978`; only
their wait clocks start at their `Stop`.

Decided with Pierre (15/11): fix the webhook node and the queue, **without an idempotency key**. A crash stays
tolerated (D-01, D-03); a normal stop does not. No SQL, no schema change, no new environment variable.
**To decide (card in the thread):** the send bound of §2 (option A below is the one implemented unless Pierre
picks B).

## Design

### 1. Webhook node: a drain longer than the webhook's 30 s

The webhook keeps its 30 s (no change for users). The drain grows to cover it:

- `automationShutdownDrain` 20 s → 35 s: the 30 s of the webhook client plus 5 s for the writes that can come
  before the call once the stop began (`CreateNodeExecution`, `buildContextFromNodeExecutions`,
  `automation_executor.go:189-199`, which have no deadline of their own: a database slow at that moment can
  still push the node past the drain; the comment says so).
- `const WebhookNodeTimeout = 30 * time.Second` in `automation_node_executor.go`, used by `app.go` and the
  constructor's fallback, so the two numbers live side by side; a test pins
  `automationShutdownDrain - WebhookNodeTimeout >= 5s`.
- `WebhookNodeExecutor.Execute` bounds the request with `context.WithTimeout(ctx, e.timeout)` (a `timeout`
  field defaulting to `WebhookNodeTimeout`, a zero value meaning the default), so the node is bounded whatever
  client it gets.
- The automation scheduler's `Stop` becomes drain + `automationStopMargin` = 44 s, which no longer fits after the
  others: it runs beside them (§3).
- Comments with "30 s" or "20 s" follow: `automation_executor.go:77`, `:81-84`, `:366`;
  `automation_node_executor.go:1151`; `app.go:1407-1409`; `automation_scheduler.go:26-40`.

### 2. Email queue: every send bounded, the stop waiting longer

**Option A (recommended, implemented unless Pierre picks B): a long bound in normal running, a long wait at the
stop.**

- `EmailQueueWorkerConfig.SendTimeout`, default **100 s**: the deadline of one claimed row, from its claim
  (`MarkAsProcessing`) to the provider's answer, below the 2-minute reclaim of a `processing` row
  (`email_queue_postgres.go:154`). Today an SMTP or SES send has no bound, and one over 2 minutes is already
  sent again by the reclaim (first half of F-07); with the bound it is cut before and retried once. Only a send
  between 100 s and 2 minutes, sent once today, may now be sent again: that is a provider 10 times slower than
  the HTTP providers' own 10 s.
- `ShutdownGrace` default 10 s → **40 s**, `AbortWait` stays 5 s. A send still running 40 s after the stop began
  is cut, as today one running 10 s after is: no HTTP provider send (10 s), only an SMTP or SES send slower than
  40 s. F-16 stays open for that case, with its severity reconsidered.
- **Option B (not recommended): a short bound always.** `SendTimeout` 35 s and `ShutdownGrace` 40 s: a normal
  stop never cuts a send, but every SMTP or SES send between 35 s and 2 minutes, sent once today, is cut and may
  be sent again, stop or not. Same code, other numbers.
- `processEntry` derives `sendCtx, cancel := context.WithTimeout(w.sendCtx, w.config.SendTimeout)` right after
  the claim, for the stop-on-reply guard, the pre-send upsert and `SendEmail`; the post-send writes stay on
  `writeCtx`. `withSendDefaults` fills a zero `SendTimeout`.
- Before `SendEmail`, a `sendCtx` already done (the guard or the upsert used the budget, a slow database) does
  not call the provider: the row is released as `retryAfterGuardError` does, not counted by the breaker
  (`worker.go:774-778`, `:834`).
- A send cut by its own `SendTimeout` (not by the worker's abort, still told apart by `w.sendCtx.Err()`) is
  classified and retried like an HTTP timeout today. **Classification fix:** `classifySMTPError` matches recipient
  codes ("5.1.1", "5.7.1"…) by substring on the whole message before "timeout" (`pkg/emailerror/smtp.go:25-65`,
  `classifier.go:113-121`), and a read timeout's text carries IP addresses that can contain such a sequence: the
  email would be failed for good. `Classify` checks `errors.Is(err, context.DeadlineExceeded)` or a
  `net.Error` with `Timeout()` first, for every provider, and returns a retryable provider timeout.
- **SMTP honours its context.** `sendRawEmailWithSettings(ctx, ...)`:
  - dial with `dialer.DialContext(ctx, ...)`;
  - when ctx has a deadline, the connection's deadline is that deadline for the whole exchange; each TLS
    handshake keeps its own bound (`min(now + dial timeout, ctx deadline)`) and is then reset to the ctx deadline,
    not to zero;
  - `context.AfterFunc(ctx, func() { conn.Close() })` interrupts the exchange when ctx is cancelled (the worker's
    abort). A transactional send on a request context is cut when the client disconnects, as the HTTP providers'
    sends already are; a cancel after the final 250 still returns success (QUIT's error is ignored, `:497`);
  - `sendRawEmail` (deprecated) and the tests pass a context.
- **OAuth2 token request honours its context.** `OAuth2TokenProvider.GetAccessToken(ctx, settings)` with
  `http.NewRequestWithContext`; the client's 30 s timeout stays. The mock is hand-written
  (`mockOAuth2TokenService`, `smtp_service_test.go:1743-1770`) and edited, with the callers in
  `oauth2_cache_invalidation_test.go`. The service's mutex held during a fetch (`oauth2_token_service.go:55-56`)
  is not context-aware: a send waiting on it can outlive its deadline, which sends nothing and duplicates nothing.
- SES needs no change: the SDK reads the context's deadline.
- `Stop`'s comment and the config's drop "an SMTP send cannot be interrupted".

### 3. `App.Shutdown`: the long stops side by side

- The email queue's `Stop` and the automation scheduler's `Stop` start at the beginning of `Shutdown`, each in
  its own goroutine, beside the web analytics flush and the task scheduler's stop; `Shutdown` waits for both
  before the rate limiter and the SMTP bridge, where it called them before. Rows the schedulers enqueue meanwhile
  stay `queued` and go out after the restart.
- Worst case before the bridge: max(10 + 5, 35 + 9, 40 + 5) = 45 s, so 55 s with the bridge, against 69 s today,
  inside the 70 s; the HTTP server keeps what is left, as today.
- A small helper (`runBeside(beside []func(), sequence ...func())`) or inline channels; a nil worker or
  scheduler, and the demo mode, behave as today.

### 4. Broadcast runs outside the scheduler: cancelled and waited for at the stop

- `TaskService` gets the app's shutdown context (constructor option or `SetShutdownContext`) and a
  `sync.WaitGroup`. `runTaskNow` runs on a context cancelled with it (instead of `context.Background()`) and is
  counted in the group; a `WaitImmediateRuns(timeout)` lets `Shutdown` wait for them, bounded (5 s, as the task
  scheduler's stop), after the task scheduler's stop and before `cleanupResources`. A run cancelled at the stop
  behaves as a scheduler run: `Enqueue` fails or rolls back, and the save runs on `leaseContext`.
- `TaskHandler` gets the same shutdown context (constructor parameter): `/api/cron` and `tasks.execute` keep
  `WithoutCancel(r.Context())` (a client that gives up must not cut a run) but are also cancelled when the
  shutdown context is (`context.AfterFunc`). The HTTP server's shutdown already waits for the handler, which now
  returns soon after T0.
- The rest of F-30 (event handlers, telemetry, the webhook delivery worker) is not about duplicates and stays out.

### 5. Cloud: tenant pods get the time the app asks for

`manager/internal/k8s/app.go` builds the tenant Deployment without `terminationGracePeriodSeconds`, so
Kubernetes kills the pod 30 s after SIGTERM: a stop that lasts longer (an automation step or an email still
sending) is cut as by a crash. Set it to 75 s (notifuse's `gracefulShutdownTimeout` 70 s + 5 s, compose's
`stop_grace_period`, what the 44.0 changelog already tells Kubernetes users to set) in `CreateApp` and in
`UpdateApp` (`app.go:93-157`, `:274-299`, the only builders). Kubernetes uses the grace period of the pod being
deleted, so the first update after this change still uses 30 s (and the one that adds the field rolls the pods);
the next ones use 75 s. Separate PR to `cloud` main, shipped no later than the 44.0 rollout.

### 6. Docs

Nothing user-visible changes for the webhook node (30 s kept). `self-hosting` pages: none states the stop
durations. No docs PR unless the code review finds a page that does.

### 7. Changelog (44.0)

- The unreleased 44.0 bullet at `CHANGELOG.md:63` ("for up to 20 seconds") becomes: an automation step already
  under way finishes and is recorded before the server exits, for up to 35 seconds, so a webhook step whose
  endpoint takes its full 30 seconds is not posted twice either. (The drain is not in 43.0: no `Fix` line.)
- **Fix** (43.0 shipped the 10 s grace and an SMTP send that ignored cancellation): an email being sent when
  Notifuse stops normally is waited for up to 40 seconds instead of 10, so it is no longer sent again after the
  restart. SMTP and SES sends are now cut after 100 seconds and retried (they had no limit, and one over 2 minutes
  was already sent again).
- **Fix**: a broadcast sent with "Send now", a winner selection, a resume, or a run started by `/api/cron`, is
  stopped cleanly when Notifuse stops, so no batch of it is sent twice after the restart.

### 8. After the merge (architecture repository, separate PR)

Mark F-80 `fixed`, narrow F-16 (sends slower than the grace) or mark it `fixed` under option B, settle the first
half of F-07, narrow F-30, with the merged notifuse commit; update `flows/email-queue.md` (P-EQ-01, P-EQ-03,
P-EQ-10, P-EQ-14, the stop), `flows/contact-timeline.md` (the drain), `flows/broadcast.md` (runs outside the
scheduler) and `INDEX.md` §Shutdown (the order of the stops, the goroutine table), moving `verified_against`.
Tick F-16 in TODO "Trop gros" if it is fixed.

## Files and tests

| Implementation file | Test file | Test cases |
|---|---|---|
| `internal/service/automation_executor.go` | `internal/service/automation_executor_shutdown_test.go` | F-80 regression with the `shutdownDrain` override and the executor's webhook timeout: the stop fires as a webhook node starts, the endpoint answers just under the node's timeout, the journey moves past the node and the endpoint is posted to once (fails when the timeout is at least the drain); `automationShutdownDrain` is 35 s and exceeds `WebhookNodeTimeout` by at least 5 s. |
| `internal/service/automation_node_executor.go` | `internal/service/automation_node_executor_test.go` | A webhook node whose endpoint does not answer returns an error at the executor's timeout (50 ms, server holding the request); the request's context has a deadline with a client that has none; a zero timeout means `WebhookNodeTimeout`; the nil fallback client's timeout is `WebhookNodeTimeout` (the test at `:4103` changes). |
| `internal/service/automation_scheduler.go` | `internal/service/automation_scheduler_test.go` | `Stop`'s timeout is the drain plus the margin (44 s, pinned). |
| `internal/service/queue/worker.go` | `internal/service/queue/worker_test.go` | Defaults: `SendTimeout` 100 s, `ShutdownGrace` 40 s, `AbortWait` 5 s (the tests at `:34-35`, `:95-96` change); `withSendDefaults` fills a zero `SendTimeout`; a `SendEmail` that blocks until its context ends is cut at `SendTimeout` and handed to `handleError` for a retry, counted by the breaker; a guard lookup that blocks is cut at `SendTimeout` and released by `retryAfterGuardError`; a pre-send upsert that uses the whole budget leaves `SendEmail` uncalled and the row released, not counted; `Stop` with a send in flight that ends before the grace waits for it, `MarkAsSent` runs and the abort does not fire; `Stop` still aborts at the grace a send that outlasts it (existing test kept). |
| `pkg/emailerror/classifier.go` | `pkg/emailerror/classifier_test.go` | For each provider kind, a `context.DeadlineExceeded` and a `net.Error` timeout are retryable provider errors; an SMTP read timeout whose addresses contain "5.1.1" is a retryable timeout, not a recipient error. |
| `internal/service/smtp_service.go` | `internal/service/smtp_service_test.go`, `internal/service/smtp_service_tls_test.go` | Under `within(...)` so that the old code fails instead of hanging: a server that accepts and never greets returns at the context's deadline; a server that stalls after `DATA` returns at the deadline; cancelling mid-exchange closes the connection and returns at once; a cancelled context fails before connecting; a cancel after the final 250 still returns success; with no deadline the exchange behaves as today (existing tests); TLS (implicit and STARTTLS): the handshake deadline is reset to the context's deadline, not cleared. |
| `internal/service/oauth2_token_service.go` | `internal/service/oauth2_token_service_test.go`, `oauth2_cache_invalidation_test.go`, `smtp_service_test.go` | The token request is cancelled with its context; the SMTP send passes its context to the provider. |
| `internal/service/task_service.go` | `internal/service/task_service_test.go` | `runTaskNow` runs on a context cancelled at the shutdown context's cancel; `WaitImmediateRuns` waits for a run in flight and returns at its timeout when the run outlasts it; a run started after the stop does not start. |
| `internal/http/task_handler.go` | `internal/http/task_handler_test.go` | `/api/cron` and `tasks.execute`: the execution context is not cancelled by the client going away, and is cancelled by the shutdown context. |
| `internal/app/app.go` | `internal/app/app_test.go` | `runBeside` runs the side functions concurrently with the sequence and returns once all ended (blocking funcs released in turn); `Shutdown` with a running queue worker leaves it stopped and waits for the immediate runs. |
| `CHANGELOG.md` | n/a | n/a |
| `cloud/manager/internal/k8s/app.go` | `cloud/manager/internal/k8s/app_test.go` | The created Deployment's pod spec has `terminationGracePeriodSeconds` 75; `UpdateApp` sets it on a Deployment that had none. |

## Out of scope

- A crash (D-01, D-03).
- Transactional sends and the SMTP bridge, outside the queue: deduplicated by `external_id` where the caller gives
  one (`transactional_service.go:681-697`); the bridge's 10 s shutdown under its 30 s handler deadline, and the
  restart after a settings save, which returns before requests in flight end (`main.go:63-69`). Listed in
  `known-not-handled.md`.
- The webhook delivery worker (subscriptions): cut at T0 and delivered again, with a `webhook-id` receivers can
  deduplicate on.

## Test commands

```
go test -race ./internal/service/ -run 'Webhook|AutomationExecutor|AutomationScheduler|SMTP|OAuth2|TaskService|RunTaskNow'
go test -race ./internal/service/queue/... ./pkg/emailerror/...
go test -race ./internal/http/ -run TaskHandler
go test -race ./internal/app/...
make test-unit
go vet ./...
make test-integration   # the queue, broadcast and automation integration tests
cd ../cloud/manager && make test && make lint
```
