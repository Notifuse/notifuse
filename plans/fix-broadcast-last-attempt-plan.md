# Fix: a broadcast marked failed while its task still retries

Created 2026-10-22 on `dev` @ `cb63fc9` (PR #468 merged); line numbers are from that commit. Status: **implemented** (2026-10-22). Amended after Gemini's counter-review (raw answer `reviews/fix-broadcast-last-attempt-gemini-brut-plan.md`, each remark checked in `reviews/fix-broadcast-last-attempt-gemini-verif-plan.md`, both in the project files), then after the max-effort code review below (Part 2 steps 2-3, Part 3), which Gemini counter-reviewed again (`reviews/fix-broadcast-last-attempt-gemini-brut-plan-v2.md`, checked in `-verif-plan-v2.md`), then after a second max-effort review (Part 2 step 5, Part 3's query, the integration helper), counter-reviewed by Gemini a third time (`-brut-plan-v3.md`, checked in `-verif-plan-v3.md`), then after a third max-effort review (Part 2 steps 5-6, Part 3's binding), counter-reviewed by Gemini a fourth time (`-brut-plan-v4.md`, checked in `-verif-plan-v4.md`), then after a fourth max-effort review (Part 2 steps 5-6, Part 3's empty list), counter-reviewed by Gemini a fifth time (`-brut-plan-v5.md`, checked in `-verif-plan-v5.md`).

Follow-up to `plans/fix-broadcast-duplicate-sends-plan.md`, whose "Not in scope" lists this defect; Pierre chose to fix it in its own PR. No new table, no migration, no new environment variable.

## Verified facts

- **F1 — The task service retries while the count is below the limit.** `MarkAsFailedTx` reads the task (`internal/repository/task_postgres.go:858-861`), schedules a retry when `task.RetryCount < task.MaxRetries` (`:870-875`) and increments `retry_count` either way (`:881`). With the `MaxRetries: 3` every broadcast task is created with (`internal/service/task_service.go:1160`), attempts run at counts 0, 1, 2 and 3: the task is marked failed only when the attempt that started at count 3, the fourth, fails.
- **F2 — The orchestrator calls the third attempt the last.** `Process` sets `isLastRetry := task.RetryCount >= task.MaxRetries-1` (`internal/service/broadcast/orchestrator.go:482`) from the task read when the run started (`task_service.go:620`). When a run with `isLastRetry` returns an error, its deferred finalizer (`orchestrator.go:489-584`) marks the broadcast `failed`, or `paused` with a reason if the run was interrupted (`:521-529`, `:558-564`). So the run that starts at count 2 marks the broadcast failed; the task service then schedules a fourth attempt (F1), which resumes at the saved cursor, sends the rest of the list and ends the broadcast `processed`. The loop only stops on `cancelled` and `paused` (`orchestrator.go:980-1010`), never on `failed`.
- **F3 — The count can drop during a run.** A resume while a run holds the task calls `ResetRetryBudget` (`task_service.go:1357`, `task_postgres.go:1280-1298`), which sets `retry_count = 0` without touching the run; the run keeps the decision it made at its start. The resume's own `WakeTask` is refused for a held task (`task_postgres.go:1225-1276`). Nothing else changes the count of a held task, except `MarkAsFailed` from `handleBroadcastCancelled` (`task_service.go:1561`), after which the broadcast is cancelled anyway. `MarkAsPendingTx` (`:1017`) and `WakeTask` (`:1234`) reset it too, but neither writes a held task.
- **F4 — Interrupted runs.** A run cut short by a cancellation is released without consuming the budget (`ReleaseTask`, `task_service.go:776-815`, `:963-998`), so its count is unchanged when it runs again. A run stuck past its lease is marked failed by the task service (`:820-841`) while the processor goroutine may still be unwinding: its finalizer can then run after `MarkAsFailedTx` has already incremented the count. The finalizer's pause on interruption on the last attempt is pinned in `orchestrator_interrupt_test.go` and `tests/integration/broadcast_interruption_test.go:686`, and promised in the changelog (`CHANGELOG.md`, "A broadcast interrupted past its retry limit is now paused with a reason").
- **F5 — Nothing else marks a broadcast failed.** `BroadcastStatusFailed` is only set by that finalizer (`orchestrator.go:526`); nothing publishes `broadcast.failed`.
- **F6 — The broadcast write.** The finalizer writes the whole row it read through `UpdateBroadcast` (`orchestrator.go:567-568`), whose `UPDATE` refuses a `cancelled` or `processed` row (`internal/repository/broadcast_postgres.go:281-283`): a cancel during the run is kept, even one committing while the write waits on its row lock, since PostgreSQL re-evaluates the `WHERE` after the wait. A pause is not: `PauseBroadcast` sets `paused` with `UpdateBroadcastStatusTx` and then pauses the queue rows and runs the task handler in the same transaction (`broadcast_service.go:525-560`), so the finalizer can read `processing`, then write `failed` over the committed pause, which cannot be resumed. A resume runs `ResetRetryBudget` inside its transaction, through `PublishWithAck`, which waits for the handler (`internal/domain/event.go:81-130`); the reset commits on the system database before the broadcast's `processing` commits.
- **F7 — Tests that pin the current rule:** `orchestrator_process_test.go:708`, `orchestrator_test.go:3235`, `orchestrator_interrupt_test.go:96`, `:167`, `:286` (all `RetryCount: 2, MaxRetries: 3`), and the integration test above, which sets `max_retries = 1` with `retry_count = 0` to make the attempt it interrupts the last. The pause reason counts attempts as `task.RetryCount+1` (`orchestrator.go:562-563`).

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
2. A run that starts as the last attempt checks again before finalizing, after the circuit-breaker and recipient-feed skips (`:491-512`, unchanged) and after its broadcast read (step 3; read first since the code review): it re-reads the task with `o.taskRepo.Get` on the detached `finalizeCtx`, and finalizes only if the stored task still has no retries left. A resume during the run (F3) gave it a fresh budget, so the failure will be retried and the broadcast is left as it is. If the read fails, the finalizer keeps the decision the run started with, as today, so a broadcast cannot be stranded in a sending status by a failed read. A small helper keeps the defer readable:

   ```go
   // stillLastAttempt re-reads the task before a run that started as its last
   // attempt finalizes the broadcast: a resume during the run refills the retry
   // budget (ResetRetryBudget), and the failure is then retried.
   func (o *BroadcastOrchestrator) stillLastAttempt(ctx context.Context, task *domain.Task) bool
   ```

   The stored count is the right one in every case of F4: a released run keeps its count; a run whose failure the task service recorded first was at count `MaxRetries` or more when it started, so the stored count is above the limit and the task is failed.
3. The finalizer leaves a `cancelled`, `paused` or `processed` broadcast as it is, deciding on the row it writes (Gemini's review, made race-free after the code review). A cancel or a pause during the last attempt is only seen by the loop on its next refresh (`orchestrator.go:980-1010`); a cancel was already kept (F6), a pause was turned into a terminal failure. The finalizer reads the broadcast first and returns at once on one of those statuses, which also spares the task read; otherwise it sets the target status and writes it with Part 3, which refuses those statuses on the row itself, so a pause committing while the write waits on its row lock is kept too. A paused broadcast whose task ended failed is resumed like any other: the resume wakes a failed task (`wakeableStatuses`, `task_postgres.go:1338-1343`). The list names the statuses to keep rather than the sending ones, since a broadcast scheduled for later keeps `scheduled` while its task sends. Reading the broadcast before the task also narrows the resume race: a read that is no longer `paused` after a pause means the resume committed, so its reset did (F6), and the task read sees the fresh budget.
4. The pause reason keeps `task.RetryCount+1`: on the last attempt it is the number of attempts in a row, this one included (4 with the default limit), since a run whose budget was reset no longer gets here.
5. Every error the last attempt returns reaches the finalizer (second and third code reviews). The defer read a local `err` that every error return sets first (`err = …; return false, err`), except two: the A/B run whose move to `testing` fails, whose `if err := …` shadows it, and the winner phase with no winner picked. On the last attempt both left the broadcast sending under a task that failed for good. `Process` now names its results `(allDone bool, err error)`, so the defer sees whatever `Process` returns: both returns reach it unchanged, and a future return cannot skip it again. The circuit-breaker completion branch (`orchestrator.go:1455-1461`), the one other return that did not set `err`, is removed: its flag is never set after its declaration (`:647`), and with named results it would reach the finalizer.
6. The interruption is decided right before the write (third code review). A cancellation that lands while the finalizer reads makes `ExecuteTask` release the task without spending its budget (`task_service.go:975`, and its `ctx.Done()` branch at `:781`), so the broadcast must end `paused` there, not `failed`. The context is asked again after the reads, where it used to be asked before them, and once more after a `failed` write (fourth code review): a cancellation landing while that write runs or waits on the row lock also releases the task, so the broadcast is then written `paused` with its reason (`failed` is not a kept status, so that write goes through). What is left is a cancellation between that last check and `ExecuteTask`'s own, the time `Process` takes to return.

### Part 3 — a status write that keeps a status committed meanwhile

`domain.BroadcastRepository` gains

```go
// UpdateBroadcastStatusUnless writes the broadcast's status, pause time and
// pause reason, unless its stored status is one of unless. It decides on the
// row it writes, so one of those statuses committed after the caller's read is
// kept. It reports whether it wrote.
UpdateBroadcastStatusUnless(ctx context.Context, broadcast *Broadcast, unless []BroadcastStatus) (bool, error)
```

implemented in `broadcast_postgres.go` as one `UPDATE broadcasts SET status, paused_at, pause_reason, updated_at WHERE id AND workspace_id AND status <> ALL($7)` on the workspace connection, the list passed as one array (`pq.Array(unless)`, which takes the named string type; `contact_postgres.go:1845` passes a list the same way), so the query text is fixed; a nil list is bound as an empty one, since `pq.Array` binds a nil slice as `NULL` and `status <> ALL(NULL)` matches no row; no row → `false, nil`. Only the status fields are written, so a pick of the A/B winner or any other field changed since the read is not rolled back, where `UpdateBroadcast` rewrote the whole row. Mock regenerated with the existing `//go:generate` line. No schema change.

The interruption branch is unchanged apart from the rule: an interruption on the last attempt still pauses the broadcast (F4), now on the attempt the task service treats as the last.

### Rejected alternatives

- **`>= MaxRetries` only (Gemini's first option).** Leaves F3: a resume during the last attempt, then a failure, still marks the broadcast failed while the task, reading count 0, retries and ends it `processed`.
- **Finalizing from the task service, after `MarkAsFailedTx` decides.** Exact by construction, but it needs a new hook on `TaskProcessor`, a result from `MarkAsFailed`, and it drops the pause on interruption, since a released task is never failed (F4). More change than the defect needs.

## Tests (written first)

| Implementation file | Test file | Cases |
|---|---|---|
| `internal/domain/task.go` | `internal/domain/task_test.go` | `TestTask_HasRetriesLeft`: count below the limit → true; one below (2 of 3) → true; equal → false; above → false; limit 0 → false |
| `internal/repository/task_postgres.go` | `internal/repository/task_postgres_test.go` | `TestTaskRepository_MarkAsFailed` gains the boundary: count 2 of 3 → `pending` with a retry time (counts 0 → pending and 3 → failed are already covered) |
| `internal/repository/broadcast_postgres.go` | `internal/repository/broadcast_postgres_test.go` | `UpdateBroadcastStatusUnless`: writes the status fields with the excluded statuses as one array argument → `true`; a nil list is bound as an empty array; no row (excluded status or missing broadcast) → `false`, no error; exec error and connection error → error |
| `internal/service/broadcast/orchestrator.go` | `orchestrator_last_attempt_test.go` (new) | Failed runs: 2 of 3 → no write, no task read; 3 of 3 → `failed`; 0 of 0 → `failed`; re-read shows count 0 → no write; re-read fails → `failed`; broadcast read back `cancelled`, `paused` or `processed` → no write and no task read; the write refused by the row (`false`) → no error from the finalizer; the broadcast read before the task. Interrupted runs: 2 of 3 → no pause; 3 of 3 → `paused`, reason "after 4 attempts"; a stuck run whose failure was recorded first (count 4, task failed) → `paused`; read back `cancelled` → no write. Errors returned past the recipient count (step 5): an A/B run whose move to `testing` fails, and a winner phase with no winner picked, each at 3 of 3 → `failed`. A genuine error whose context is cancelled while the finalizer reads (step 6) → `paused`; cancelled while its `failed` write runs → a second write, `paused` |
| same | `orchestrator_process_test.go`, `orchestrator_interrupt_test.go`, `orchestrator_test.go` | Tests pinning the old rule (F7) move to count 3 with a re-read that rides a live context, and expect the conditional write; the recipient-feed pause test still writes once (its pause) and reads no task; the template-validation test (a task with no retries allowed) expects the re-read, and so does the winner-phase test with no winner picked, which now reaches the finalizer (step 5) |
| end to end | `tests/integration/broadcast_last_attempt_test.go` (new) | `TestBroadcastFailureWithRetriesLeft_StaysInFlight`: injected enqueue failure at count 2 of 3 → task `pending` at count 3, broadcast still `processing`; fault cleared → `processed`, every recipient enqueued once. `TestBroadcastFailureOnLastAttempt_FailsWithItsTask`: same at count 3 → task `failed`, broadcast `failed`. `TestBroadcastResumeDuringLastAttempt_KeepsItsRetries`: count 3, the enqueue held inside a trigger that waits on `pg_advisory_xact_lock` and then raises, the lock held on a dedicated `*sql.Conn` (a pooled `Exec` could unlock on another connection) and released before the trigger is dropped on teardown; pause and resume while the run is held, wait for `retry_count = 0`, release → broadcast `processing`, task `pending`; fault cleared → `processed`. `TestBroadcastPauseDuringLastAttemptFailure_IsKept`: count 3, the run held the same way; a transaction sets the broadcast `paused` and stays open; release → the finalizer reads `processing` and its write waits on the row lock (seen in `pg_locks` as an ungranted `transactionid` or `tuple` lock); commit → broadcast `paused`, task `failed`; resume → `processed`. `TestBroadcastABStartFailureOnLastAttempt_FailsWithItsTask` (step 5): an A/B broadcast at count 3, its move to `testing` refused by a trigger on `broadcasts` → task `failed`, broadcast `failed` (it stayed `processing` before). `holdLastAttempt` undoes what it set up (lock, connection, trigger) when one of its own checks fails, since the caller's deferred cleanup is not registered yet |
| same | `tests/integration/broadcast_interruption_test.go` | `TestBroadcastInterruptedOnLastRetry_PausesWithReason` sets `max_retries = 0` (no retry left) instead of 1 |

Each new test fails on `dev` before the fix (the integration ones see `failed` where they expect `processing` or `paused`), except the domain test, which does not compile, and the `failed` cases, which pin the agreement. `TestBroadcastResumeDuringLastAttempt_KeepsItsRetries` also fails with `>= MaxRetries` alone (checked), and `TestBroadcastPauseDuringLastAttemptFailure_IsKept` with a guard on the read alone.

## Changelog

Under 42.0 (WIP), one line:

- **Improvement**: A broadcast whose sending keeps failing is now marked failed once its last retry has failed, instead of one attempt early while that retry went on to send to the rest of the list. A pause made during that last attempt is kept.

## Not in scope

- **A pause and a resume both committing between the finalizer's task read and its write.** The finalizer then saw the spent budget, and its write finds `processing` again; the task service reads the fresh budget and retries. A pause alone is kept (Part 3), and a resume after a pause the broadcast read saw is caught by the task read (step 3), so this takes a whole pause-resume cycle within a few milliseconds. Closing it would take a compare-and-swap on the row's `updated_at` and a loop that runs the decision again when the write is refused (third review), or finalizing from the task service (rejected above); neither is worth it for two user actions within a few milliseconds.
- **A run stuck past its lease whose task is resumed and then marked failed, with `max_retries` lowered to 1 by hand.** The re-read then sees count 1 on a pending task and finalizes. Broadcast tasks are created with 3.
- **The orchestrator does not stop on a `failed` broadcast** (F2). Stopping there as it stops on `cancelled` (`allDone = true`, no status write) would end every window left here `failed` rather than failed then `processed` (Gemini, second and third reviews). Rejected: it changes what the send loop does with a status it has never stopped on, beyond this fix, for windows that need a pause and a resume, or a deploy, within milliseconds of the finalizer; in the pause-resume one the user's last action is a resume, so sending the rest is what they asked for.
- **A run stuck past its lease pauses its broadcast while its task is failed** (Gemini's second remark). Intended: an interruption leaves the broadcast resumable, and the resume wakes the failed task.
- **A failed or interrupted broadcast keeps the `enqueued_count` it had** (Gemini's second review). Only the completion paths write the column (`orchestrator.go:768`, `:1540`); `dev`'s whole-row write wrote back the value it read. The console's progress reads the task state's count instead.
- **A failed task re-read marks the broadcast failed** (second code review). The reviewer's case: the system database fails the read, then `MarkAsFailed` too, the lease expires and the task runs again at the same count, sending the rest and ending `processed`, the defect this plan fixes. Leaving the broadcast untouched instead strands it in a sending status whenever the read fails and `MarkAsFailed` does not, with nothing visible to the user. The first ends in the right status after a wrong one; the second never does. Kept.
- **Overlapping runs.** A run stuck past its lease is marked failed by its `ExecuteTask` (F4) while the scheduler may already have handed the task to a new run, since `MarkAsFailedTx` checks no ownership. If that raises the stored count to the limit during a run that started with a retry left, that run does not finalize, and its own failure then fails the task with the broadcast still sending. It takes a processor that ignores its context for the 30-second grace, a re-dispatch within the instant the lease expires, and the count at exactly one below the limit. The lease and the grace exist to prevent that overlap (`task_service.go:664-671`); closing it belongs there.
- **A panic in the last attempt skips the finalizer** (third review), as on `dev`: `ExecuteTask` recovers it and fails the task, and the broadcast stays sending. Recovering in the defer to finalize and then panicking again would replace the panic's stack in the task service's log; a panic is a bug to fix where it happens.
- **The circuit-breaker and recipient-feed skips trust the pause their own path wrote** (third review), as on `dev`. If that write failed on the last attempt, the broadcast stays sending under a failed task. It takes a database failure in that one write.
- **The completion write overwrites a pause committed while it waits** (fourth review), as on `dev`. The run's last write when it finishes sending, and its move to `testing`, are whole-row `UpdateBroadcast` calls, which only refuse `cancelled` and `processed` rows: a pause whose transaction commits while that write waits on the row lock becomes `processed`. Same pattern as Part 3, on another write; a follow-up.
- **The finalizer's detached context has no deadline** (fourth review), as on `dev`: a hung database blocks the processor goroutine after `ExecuteTask` has given up.
- **The task re-read locks the row.** `TaskRepository.Get` reads `FOR UPDATE` in a short transaction (`task_postgres.go:153-176`), as every caller of it does; the finalizer reads it only on a last-attempt failure, and a concurrent `ReleaseTask` or `MarkAsFailed` waits for that one read.

## Code review

Max-effort review of `9d854a4` (one pass, all angles, findings re-checked against PostgreSQL):

| # | Finding | Outcome |
|---|---|---|
| 1 | The cancelled/paused guard decides on its read: a pause whose transaction commits while the finalizer's `UPDATE` waits on the row lock is overwritten with `failed` (reproduced on PostgreSQL) | Fixed: Part 3, the write refuses those statuses on the row |
| 2 | Reading the task before the broadcast leaves the resume race open; reading the broadcast first closes it except for a whole pause-resume cycle between the two reads | Fixed: the broadcast is read first (step 3); "Not in scope" reworded |
| 3 | The integration test dropped its trigger before releasing the advisory lock on teardown, so a failed check while the run was held blocked teardown behind the run | Fixed: the lock is released before the trigger is dropped |
| 4 | A cancel was already kept by `UpdateBroadcastTx`'s `WHERE`: the plan's claim and the changelog's "a cancel is kept" were wrong | Fixed: F6 states it, the changelog only names the pause |
| 5 | The plan's test table named files and tests that do not exist | Fixed: the table names `orchestrator_last_attempt_test.go` and `broadcast_last_attempt_test.go` |
| 6 | The re-read takes `FOR UPDATE` over the whole row for two counters | Kept: see "Not in scope" |

Second max-effort review, of `5d3ad04` (rebased on `dev` @ `2369599`):

| # | Finding | Outcome |
|---|---|---|
| 1 | Two error returns (the A/B move to `testing`, a winner phase with no winner) skip the finalizer, which reads `err` | Fixed: Part 2 step 5 |
| 2 | A failed task re-read marks the broadcast failed, which can come back as failed-then-processed if `MarkAsFailed` fails too | Kept: see "Not in scope" |
| 3 | A run whose count reaches the limit during the run, through an overlapping stuck run's `MarkAsFailed`, never finalizes | Kept: see "Not in scope" |
| 4 | Root cause: a run never stops on a `failed` broadcast | Kept: already rejected (F2, "Not in scope") |
| 5 | The interface comment promised that no status committed after the read is overwritten; only the listed ones are kept | Fixed: Part 3's comment |
| 6 | The re-read takes `FOR UPDATE` | Kept: see "Not in scope" |
| 7 | The excluded statuses were bound as hand-numbered placeholders where the repositories pass a list as one array | Fixed: Part 3, `status <> ALL($7)` |
| 8 | `holdLastAttempt` left its lock, connection and trigger behind when one of its own checks failed | Fixed: it undoes them itself |

Third max-effort review, of `fd795f2`:

| # | Finding | Outcome |
|---|---|---|
| 1 | The interruption is decided before the finalizer's reads: a cancellation during them releases the task while the finalizer writes `failed` | Fixed: Part 2 step 6 |
| 2 | A panic in the last attempt skips the finalizer | Kept: see "Not in scope" |
| 3 | The circuit-breaker and recipient-feed skips assume their own pause write succeeded | Kept: see "Not in scope" |
| 4 | The pause-resume window could be closed with a compare-and-swap on `updated_at` | Kept, reason corrected in "Not in scope" |
| 5 | The two returns were fixed one by one where named results fix the class; the branch that ruled them out is dead code | Fixed: Part 2 step 5, named results, dead branch removed |
| 6 | Stopping on a `failed` broadcast would close every remaining window | Kept, reason corrected in "Not in scope" |
| 7 | `pq.Array` takes `[]domain.BroadcastStatus` directly (checked: `{"cancelled","paused"}`), the copy and its comment are wrong | Fixed: Part 3 |

Fourth max-effort review, of `23c6014`:

| # | Finding | Outcome |
|---|---|---|
| 1 | A cancellation landing while the `failed` write runs or waits on its row lock releases the task with the broadcast `failed` (reproduced) | Fixed: Part 2 step 6, the context is asked again after that write |
| 2 | The completion write and the move to `testing` overwrite a pause committed while they wait (reproduced, as on `dev`) | Kept: see "Not in scope", a follow-up |
| 3 | The circuit-breaker and recipient-feed skips are redundant with the kept statuses and strand the broadcast if their own pause write failed | Kept: see "Not in scope" (third review, 3); the changelog no longer says "whatever error ended it" |
| 4 | A nil list is bound as `NULL`, and the write then matches no row | Fixed: Part 3 |
| 5 | The re-read takes `FOR UPDATE`, and the detached context has no deadline | Kept: see "Not in scope" |
| 6 | Root cause: the finalizer predicts the task service's decision | Kept: see "Rejected alternatives" and "Not in scope" |
| 7 | With named results, `err = X; return false, err` is redundant | Fixed: the two returns are back to `dev`'s lines |

## Steps

1. Domain test, then `HasRetriesLeft`; repository test, then `MarkAsFailedTx`.
2. Orchestrator tests (new and moved), then Part 2.
3. Repository test, then Part 3; regenerate the `BroadcastRepository` mock.
4. Integration tests, red on `dev`, then green.
5. Changelog line.
6. Run `make test-domain`, `make test-repo`, `make test-service` (with `go test ./internal/service/broadcast/...`), `go vet ./...`, and the integration suite (`make test-integration`, at least `./tests/integration/ -run 'Broadcast|Task'`).
