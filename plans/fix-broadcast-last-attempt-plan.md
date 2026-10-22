# Fix: a broadcast marked failed while its task still retries

Created 2026-10-22 on `dev` @ `cb63fc9` (PR #468 merged); line numbers are from that commit. Status: **implemented** (2026-10-22). Amended after Gemini's counter-review (raw answer `reviews/fix-broadcast-last-attempt-gemini-brut-plan.md`, each remark checked in `reviews/fix-broadcast-last-attempt-gemini-verif-plan.md`, both in the project files).

Follow-up to `plans/fix-broadcast-duplicate-sends-plan.md`, whose "Not in scope" lists this defect; Pierre chose to fix it in its own PR. No new table, no migration, no new environment variable.

## Verified facts

- **F1 — The task service retries while the count is below the limit.** `MarkAsFailedTx` reads the task (`internal/repository/task_postgres.go:858-861`), schedules a retry when `task.RetryCount < task.MaxRetries` (`:870-875`) and increments `retry_count` either way (`:881`). With the `MaxRetries: 3` every broadcast task is created with (`internal/service/task_service.go:1160`), attempts run at counts 0, 1, 2 and 3: the task is marked failed only when the attempt that started at count 3, the fourth, fails.
- **F2 — The orchestrator calls the third attempt the last.** `Process` sets `isLastRetry := task.RetryCount >= task.MaxRetries-1` (`internal/service/broadcast/orchestrator.go:482`) from the task read when the run started (`task_service.go:620`). When a run with `isLastRetry` returns an error, its deferred finalizer (`orchestrator.go:489-584`) marks the broadcast `failed`, or `paused` with a reason if the run was interrupted (`:521-529`, `:558-564`). So the run that starts at count 2 marks the broadcast failed; the task service then schedules a fourth attempt (F1), which resumes at the saved cursor, sends the rest of the list and ends the broadcast `processed`. The loop only stops on `cancelled` and `paused` (`orchestrator.go:980-1010`), never on `failed`.
- **F3 — The count can drop during a run.** A resume while a run holds the task calls `ResetRetryBudget` (`task_service.go:1357`, `task_postgres.go:1280-1298`), which sets `retry_count = 0` without touching the run; the run keeps the decision it made at its start. The resume's own `WakeTask` is refused for a held task (`task_postgres.go:1225-1276`). Nothing else changes the count of a held task, except `MarkAsFailed` from `handleBroadcastCancelled` (`task_service.go:1561`), after which the broadcast is cancelled anyway. `MarkAsPendingTx` (`:1017`) and `WakeTask` (`:1234`) reset it too, but neither writes a held task.
- **F4 — Interrupted runs.** A run cut short by a cancellation is released without consuming the budget (`ReleaseTask`, `task_service.go:776-815`, `:963-998`), so its count is unchanged when it runs again. A run stuck past its lease is marked failed by the task service (`:820-841`) while the processor goroutine may still be unwinding: its finalizer can then run after `MarkAsFailedTx` has already incremented the count. The finalizer's pause on interruption on the last attempt is pinned in `orchestrator_interrupt_test.go` and `tests/integration/broadcast_interruption_test.go:686`, and promised in the changelog (`CHANGELOG.md`, "A broadcast interrupted past its retry limit is now paused with a reason").
- **F5 — Nothing else marks a broadcast failed.** `BroadcastStatusFailed` is only set by that finalizer (`orchestrator.go:526`); nothing publishes `broadcast.failed`.
- **F6 — Tests that pin the current rule:** `orchestrator_process_test.go:708`, `orchestrator_test.go:3235`, `orchestrator_interrupt_test.go:96`, `:167`, `:286` (all `RetryCount: 2, MaxRetries: 3`), and the integration test above, which sets `max_retries = 1` with `retry_count = 0` to make the attempt it interrupts the last. The pause reason counts attempts as `task.RetryCount+1` (`orchestrator.go:562-563`).

## The fix

### Part 1 — one rule for "retries left"

`internal/domain/task.go`: add

```go
// HasRetriesLeft reports whether the task service schedules another attempt
// when the current one fails: MarkAsFailedTx retries while the count of failed
// attempts in a row is below MaxRetries.
func (t *Task) HasRetriesLeft() bool { return t.RetryCount < t.MaxRetries }
```

`MarkAsFailedTx` uses it in place of its inline comparison (`task_postgres.go:870`). Same behaviour; the orchestrator now reads the rule from the same place, so the two cannot drift again.

### Part 2 — the orchestrator finalizes only on the attempt the task service gives up on

In `Process` (`orchestrator.go:482-584`):

1. `isLastRetry` becomes `lastAttempt := !task.HasRetriesLeft()`: with `MaxRetries` 3, the run that starts at count 3. A run that starts with retries left never finalizes; its count cannot rise during the run except by its own failure being recorded (F3, F4), and that failure is then retried.
2. A run that starts as the last attempt checks again before finalizing, after the circuit-breaker and recipient-feed skips (`:491-512`, unchanged) and before its broadcast read: it re-reads the task with `o.taskRepo.Get` on the detached `finalizeCtx`, and finalizes only if the stored task still has no retries left. A resume during the run (F3) gave it a fresh budget, so the failure will be retried and the broadcast is left as it is. If the read fails, the finalizer keeps the decision the run started with, as today, so a broadcast cannot be stranded in a sending status by a failed read. A small helper keeps the defer readable:

   ```go
   // stillLastAttempt re-reads the task before a run that started as its last
   // attempt finalizes the broadcast: a resume during the run refills the retry
   // budget (ResetRetryBudget), and the failure is then retried.
   func (o *BroadcastOrchestrator) stillLastAttempt(ctx context.Context, task *domain.Task) bool
   ```

   The stored count is the right one in every case of F4: a released run keeps its count; a run whose failure the task service recorded first was at count `MaxRetries` or more when it started, so the stored count is above the limit and the task is failed.
3. The finalizer leaves a broadcast that is `cancelled` or `paused` when it reads it as it is (added after Gemini's review). A cancel or a pause during the last attempt is only seen by the loop on its next refresh (`orchestrator.go:980-1010`), so a run that failed before then overwrote the user's cancel with `failed`, or turned a resumable pause into a terminal failure; `dev` does the same from the third attempt. A paused broadcast whose task ended failed is resumed like any other: the resume wakes a failed task (`wakeableStatuses`, `task_postgres.go:1338-1343`). The guard lists the two statuses rather than the sending ones, since a broadcast scheduled for later keeps `scheduled` while its task sends.
4. The pause reason keeps `task.RetryCount+1`: on the last attempt it is the number of attempts in a row, this one included (4 with the default limit), since a run whose budget was reset no longer gets here.

The interruption branch is unchanged apart from the rule: an interruption on the last attempt still pauses the broadcast (F4), now on the attempt the task service treats as the last.

### Rejected alternatives

- **`>= MaxRetries` only (Gemini's first option).** Leaves F3: a resume during the last attempt, then a failure, still marks the broadcast failed while the task, reading count 0, retries and ends it `processed`.
- **Finalizing from the task service, after `MarkAsFailedTx` decides.** Exact by construction, but it needs a new hook on `TaskProcessor`, a result from `MarkAsFailed`, and it drops the pause on interruption, since a released task is never failed (F4). More change than the defect needs.

## Tests (written first)

| Implementation file | Test file | Cases |
|---|---|---|
| `internal/domain/task.go` | `internal/domain/task_test.go` | `TestTask_HasRetriesLeft`: count below the limit → true; one below (2 of 3) → true; equal → false; above → false; limit 0 → false |
| `internal/repository/task_postgres.go` | `internal/repository/task_postgres_test.go` | `TestTaskRepository_MarkAsFailed` gains the boundary: count 2 of 3 → `pending` with a retry time (counts 0 → pending and 3 → failed are already covered) |
| `internal/service/broadcast/orchestrator.go` | `orchestrator_process_test.go` | New `TestProcess_FailureWithRetriesLeft_LeavesBroadcastToTheRetry`: count 2 of 3, genuine error → no `UpdateBroadcast`, no task read. `TestProcess_LastRetryError` moves to count 3 and expects the re-read (count 3) → `failed`. New: re-read shows count 0 (a resume refilled the budget) → no `UpdateBroadcast`. New: re-read fails → `failed`, as the run decided at its start |
| same | `orchestrator_process_test.go` | New: last attempt, genuine error, broadcast read back `cancelled` → no `UpdateBroadcast`; same with `paused` → no `UpdateBroadcast`; interrupted run on the last attempt with the broadcast read back `cancelled` → no `UpdateBroadcast` |
| same | `orchestrator_interrupt_test.go` | The three tests move to count 3 with a re-read; the cancelled-context one also asserts the re-read rides a live context and the reason says "after 4 attempts". New: cancelled run at count 2 of 3 → no pause |
| same | `orchestrator_test.go` | The recipient-feed pause test (`:3235`) moves to count 3; still exactly one `UpdateBroadcast` (the pause) and no task read |
| end to end | `tests/integration/broadcast_interruption_test.go` | New `TestBroadcastFailureWithRetriesLeft_StaysInFlight`: injected enqueue failure at count 2 of 3 → task `pending` at count 3, broadcast still `processing`; fault cleared → broadcast `processed`, every recipient enqueued once. New `TestBroadcastFailureOnLastAttempt_FailsWithItsTask`: same at count 3 → task `failed`, broadcast `failed`. New `TestBroadcastResumeDuringLastAttempt_KeepsItsRetries`: count 3, the enqueue held inside a trigger that waits on `pg_advisory_xact_lock` and then raises, while the test holds the lock on a dedicated `*sql.Conn` (a pooled `Exec` could unlock on another connection) and waits until `pg_locks` shows the run waiting; pause and resume while the run is held, wait for `retry_count = 0`, release → broadcast not `failed`, task `pending`; fault cleared → `processed`. `TestBroadcastInterruptedOnLastRetry_PausesWithReason` sets `max_retries = 0` (no retry left) instead of 1 |

Each new test fails on `dev` before the fix (the integration ones see `failed` where they expect `processing`), except the domain test, which does not compile, and the `failed` cases, which pin the agreement.

## Changelog

Under 42.0 (WIP), one line:

- **Improvement**: A broadcast whose sending keeps failing is now marked failed only once its last retry has failed, instead of one attempt earlier while that retry went on to send to the rest of the list.

## Not in scope

- **A resume between the finalizer's re-read and `MarkAsFailedTx`'s own read.** The finalizer reads first, then writes the broadcast, then the task service records the failure; a resume in that gap of a few milliseconds still sees the broadcast marked failed while the task retries. Closing it means finalizing from the task service (rejected above).
- **A run stuck past its lease whose task is resumed and then marked failed, with `max_retries` lowered to 1 by hand.** The re-read then sees count 1 on a pending task and finalizes. Broadcast tasks are created with 3.
- **The orchestrator does not stop on a `failed` broadcast** (F2). With this fix the finalizer no longer marks a broadcast failed while its task retries, apart from the gap above, which takes a pause and a resume landing within it: the user's last action is then a resume, so the retry sending the rest is what they asked for. Stopping there would also need its own way to end the task (a run ending with `allDone = false` reruns on every tick). Gemini suggested it; rejected for these reasons.
- **A run stuck past its lease pauses its broadcast while its task is failed** (Gemini's second remark). Intended: an interruption leaves the broadcast resumable, and the resume wakes the failed task.

## Steps

1. Domain test, then `HasRetriesLeft`; repository test, then `MarkAsFailedTx`.
2. Orchestrator tests (new and moved), then Part 2.
3. Integration tests, red on `dev`, then green.
4. Changelog line.
5. Run `make test-domain`, `make test-repo`, `make test-service` (with `go test ./internal/service/broadcast/...`), `go vet ./...`, and the integration suite (`make test-integration`, at least `./tests/integration/ -run 'Broadcast|Task'`).
