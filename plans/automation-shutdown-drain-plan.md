# Automation shutdown drain plan

## Problem

On a normal stop (SIGTERM, for example during a deployment), `App.Shutdown` cancels the
application's shutdown context first (`a.shutdownCancel()`), then calls
`AutomationScheduler.Stop`, which waits 5 s at most for the running batch.

The scheduler hands that same context to `AutomationExecutor.ProcessBatch`, which passes it to
`Execute`, and `Execute` uses it for every step of a journey: the node's effect (enqueue an email,
post a webhook, add to a list) and the write that moves the journey past the node
(`UpdateContactAutomationIfActive`), plus `handleError`'s retry write and the node execution logs.

So a node whose effect is under way when the stop begins either:

- completes its effect (the email enqueue or the webhook already went out), then fails its
  journey write because the context is cancelled; or
- fails because the context is cancelled, and `handleError`'s write fails for the same reason.

Either way the journey row keeps its claim lease (`scheduled_at` = claim time + 5 min, set by
`ClaimScheduledContactAutomationsGlobal` / `RenewContactAutomationClaim`) and its current node.
Once the lease ends, any instance claims it again and runs the same node a second time. The email
node builds a new message ID on each run, so the contact receives the email twice; a webhook node
posts twice.

The email queue worker already avoids the same trap with `writeCtx`
(`context.WithoutCancel`), so a send that left still gets recorded.

Out of scope: a crash or a kill (SIGKILL) between a node's effect and its journey write. That
window remains and is accepted; this change covers the normal stop only.

## Fix

### 1. `ProcessBatch` runs journeys on a context detached from the stop

`internal/service/automation_executor.go`

- Keep `ctx` (the shutdown context) for what it is meant for: the claim, the claim renewal, and
  the check before each journey (`if ctx.Err() != nil { break }`), so no new journey starts once
  the stop begins. Unchanged.
- Build one run context per batch: `runCtx, cancelRun := context.WithCancel(context.WithoutCancel(ctx))`,
  `defer cancelRun()`.
- Bound it from the stop: a goroutine waits for `ctx.Done()`, then for `e.drainTimeout()`, then
  calls `cancelRun()`; it also returns as soon as `runCtx` is done (batch finished), so it never
  outlives the batch. The deadline counts from the stop, not from the start of the batch: a
  normal batch with slow webhooks is never cut while the app is running.
- Call the journey run with `runCtx`, and with `ctx.Done()` as the stop signal (see 2).

`drainTimeout()` returns the executor's `shutdownDrain` field when it is positive, otherwise the
package constant `automationShutdownDrain = 20 * time.Second`. A field with a fallback, not a
constructor argument, because the tests build `&AutomationExecutor{...}` literals; tests set the
field to a few milliseconds.

### 2. `Execute` starts no node once the stop begins

Without this, the detached context would let a journey go on through up to `maxNodesPerTick` (10)
nodes during the stop, starting new effects that the deadline may cut halfway: the very failure
this change removes.

- `Execute(ctx, workspaceID, ca)` stays as the exported entry point and calls
  `e.execute(ctx, nil, workspaceID, ca)` (a nil channel never fires: same behaviour as today).
- `execute(ctx, stop <-chan struct{}, workspaceID, ca)` holds the current body. A helper
  `stopping := func() bool { select { case <-stop: return true; default: return false } }`.
- At the top of each node iteration, before `CreateNodeExecution` and the node's effect:
  `if stopping() { return nil }`. This also covers the first node, whose start follows a few
  database reads after the check in `ProcessBatch`. The node in progress always finishes and is
  recorded; no new node starts. A journey stopped there keeps the lease of its last write (the
  claim renewal or the in-tick lease) and resumes when it ends, like the journeys left in the batch.
- Where an active journey with no time from its node gets its resume time: today
  `resumeAt = now + inTickResumeLease` on every iteration but the last, `now` on the last. Use
  `now` also when `stopping()`, so a journey whose node finished during the stop is handed back due
  now rather than held 5 minutes. If the stop lands between this decision and the next iteration's
  check, the journey is held by the lease (5 min); it is never stranded, since a time is always set.

### 3. The scheduler: stop signal and wait

`internal/service/automation_scheduler.go`

- `run` derives its context from the one `Start` received and also cancels it when `stopChan`
  closes (`context.WithCancel` + a goroutine that waits for `stopChan` or the context). A `Stop()`
  without a cancelled `Start` context then gets the same drain as the app's shutdown.
- Replace `time.After(5 * time.Second)` with `time.After(s.stopTimeout())`:
  `s.executor.drainTimeout() + s.stopMarginOrDefault()`, where the margin is a field with a
  fallback to `automationStopMargin = 5 * time.Second` (for the writes that fail fast once the run
  context is cancelled and for the loop to return). With the defaults: 25 s. Nil executor: fall back
  to the constant drain.

### 4. Shutdown budget

`App.Shutdown` cancels the shutdown context first (so the drain starts counting from there), then
stops, one after another: web analytics buffer flush (≤ 10 s), task scheduler (≤ 5 s), automation
scheduler (≤ 25 s from its own call, but the drain counts from the cancel, so it overlaps the
earlier steps), email queue worker (≤ 15 s), SMTP bridge (≤ 10 s), then the HTTP shutdown bounded by
min(`shutdownTimeout`, what is left of `cmd/api/main.go`'s 70 s context − 1 s). Realistic worst case
before the HTTP shutdown grows by up to 20 s and is only reached when a webhook node is slow at the
stop; an email node is an enqueue that takes milliseconds. The total stays within main.go's 70 s.
No change to `app.go`: the queue worker's context is cancelled at the very start of `Shutdown`, so
its in-flight sends are not delayed by the wait on the automation scheduler.

Known limits, not addressed:
- A webhook node allows up to 30 s, above the 20 s drain: a webhook still running 20 s after the
  stop is cut and runs again after the restart.
- A platform that kills the process before the drain ends (Docker Compose's default 10 s stop
  timeout, `compose.yaml` sets no `stop_grace_period`) behaves like a crash for what is still
  running then.
- The restart triggered from the settings or setup screens calls `Shutdown(context.Background())`;
  it can take up to 20 s longer when a slow webhook node is running.

## Tests (TDD: written first, red, then the code)

Helper: `testNodeExecutor` (automation_executor_test.go) with two node types when two fakes are
needed (`nodeExecutors` is keyed by type). The mocked `UpdateContactAutomationIfActive` refuses a
done context (returns `ctx.Err()`), like the database does, so tests fail on today's code.

### `internal/service/automation_executor_shutdown_test.go` (new)

`TestAutomationExecutor_ProcessBatch_Shutdown`, plus `TestAutomationExecutor_Execute_WithoutStopSignalRunsEveryNode` (a direct `Execute` keeps running node after node):

1. **A stop during a node lets the node finish and its write land** (red today). One journey,
   node A (fake) → nil. The fake signals it started, waits until the test cancelled the batch
   context, checks its own context is not done, returns `NextNodeID: nil`. Expect the journey
   write to succeed with status completed, `UpdateNodeExecution` completed, `processed == 1`.
   Default drain (no small value: the check would race the timer).
2. **No new node starts after the stop** (red today). A (fake, cancels the batch context from
   inside, returns next = B) → B (a second fake that fails the test if called). Expect one journey
   write with `CurrentNodeID == B` and `ScheduledAt` within a few seconds of now (not now +
   `inTickResumeLease`), and B never run.
3. **No new journey starts after the stop** (regression guard). Two journeys; the first one's node
   cancels the batch context. One renewal and one run; the second journey is neither renewed nor run.
4. **The drain ends at its deadline** (red today: no deadline, the node would block forever).
   `shutdownDrain` = 200 ms. The fake blocks on its context; the test cancels the batch context and
   records the time; the fake's context is done no earlier than the drain after the cancel, and
   `ProcessBatch` returns within a select timeout of a few seconds. `MaxRetries` set to 3 so
   `handleError` takes the retry path (its write fails on the cancelled context).
5. **No deadline while running** (regression guard). Without a stop, `shutdownDrain` = 50 ms, a node
   that takes 300 ms gets a context that is not done and its write lands.

The existing "on shutdown the journeys left keep their claim and are not renewed" subtest stays.

### `internal/service/automation_scheduler_test.go`

- **`Stop` waits for the node in progress and its write** (red today: the write
  runs on the cancelled context and the mock refuses it). Executor drain 2 s, margin 100 ms; the fake node, once
  started, waits for the stop (its context stays alive), sleeps 300 ms, returns. Test: start,
  wait for the node, cancel the `Start` context, call `Stop`; when `Stop` returns, the journey
  write has been made.
- **`Stop` gives up at drain + margin** (red today: the old Stop waits 5 s). Drain 200 ms, margin
  100 ms; a fake node that ignores its context and blocks until the test's cleanup. `Stop` returns
  after at least 300 ms and well under 5 s (assert < 2 s).
- **`Stop` without a cancelled `Start` context starts the drain** (red today). Same blocking-on-ctx
  fake, drain 200 ms; call `Stop` without cancelling: the fake's context is done and `Stop` returns
  under 2 s.
- **Default stop timeout** pinned with literals: `stopTimeout()` of a scheduler built with
  defaults is `25 * time.Second`.

### `tests/integration/automation_claim_test.go`

New subtest in `TestAutomationClaim`, **"a stop during a step lets it finish and record it"**:
trigger → webhook (httptest receiver that blocks until released; the release is deferred before
`receiver.Close`, and the test waits for the call with a timeout), one enrolled journey. Run
`ProcessBatch(stopCtx, 50)` in a goroutine; when the receiver gets the call, cancel `stopCtx`, then
release the receiver. After `ProcessBatch` returns: the journey is `completed` in the database, the
webhook node execution is `completed`, the receiver got one call. Then make the row due again
(`UPDATE ... scheduled_at = NOW() - INTERVAL '1 second'`) and run `ProcessBatch` once more: it
processes 0 and the receiver still has one call. Before the fix the journey stays `active` on the
webhook node and the second run calls the webhook again.

## Docs and changelog

- `CHANGELOG.md`, 44.0 (unreleased): its own line (the behaviour shipped in 43.0), labelled
  **Improvement** as an edge case: a normal stop, such as an upgrade,
  lets an automation step under way finish and record itself (up to 20 s), so it does not run
  again after the restart and its email is not sent twice.
- No user documentation change: no setting, no API.

## Test commands

```
go test -race ./internal/service -run 'AutomationExecutor|AutomationScheduler' -count=1
make test-service
INTEGRATION_TESTS=true go test -race -tags integration,licdev -run 'TestAutomationClaim' ./tests/integration/ -count=1
make test-unit
```
