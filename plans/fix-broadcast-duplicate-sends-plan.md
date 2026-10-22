# Fix: broadcasts sent twice to the same contacts, and the losing A/B variation sent after a manual pick

Created 2026-10-20 on `dev` @ `c6a0a9c7`; line numbers are from that commit. Status: **implemented** (2026-10-21). Amended after two self-reviews and Gemini's counter-review (raw answer `reviews/fix-broadcast-duplicate-sends-gemini-brut-plan.md`, each remark checked in `reviews/fix-broadcast-duplicate-sends-gemini-verif-plan.md`, both in the project files), then after the code reviews below and two more counter-reviews by Gemini of those amendments (`reviews/fix-broadcast-duplicate-sends-gemini-brut-plan-v2.md` and `-v3.md`, checked in `reviews/fix-broadcast-duplicate-sends-gemini-verif-plan-v2.md` and `-v3.md`).

- Decided with Pierre on 2026-10-20: no new table and no migration. A second execution of the same broadcast task is prevented where tasks change status, rather than duplicates being filtered at enqueue time.
- Review of the 42.0 recipient query, with the e2e evidence: `audits/broadcast-unicite-destinataires.md` in the project files.

## What recipients see

Reproduced end to end, with the live task scheduler (500 ms ticks, HTTP dispatch), the email queue worker, SMTP delivery to Mailpit and PostgreSQL 17 (`en_US.utf8`), counting the emails Mailpit received. No lock or artificial wait was used.

| Scenario | Received | Expected |
|----------|----------|----------|
| A/B broadcast, winner picked through the API as the test phase starts | 2,000 emails, 1,000 contacts, every contact twice | 1,000 |
| Same, picked halfway through the test phase | 1,650 emails, 650 contacts twice, 818 emails of the losing variation | 1,000, losing variation only in the test sample |
| Plain broadcast paused and resumed while a slow batch is prepared | 550 emails to 300 contacts, the 250 after the first batch twice | 300 |
| Console flow: test phase ends, winner picked, rest of the list sent | 400 emails, no duplicate, but 94 of the 200 contacts outside the sample got the losing variation | winner only outside the sample |

The 42.0 recipient query itself is sound: 1,140 emails to 1,140 contacts while 823 audience changes ran during the send.

## Verified facts

- **F1 — The launch gate trusts the status.** `MarkAsRunningTx` (`internal/repository/task_postgres.go:744-795`) takes a `pending` or `paused` task whatever its `timeout_after`, and a `running` one only once `timeout_after <= now`. `GetNextBatch` selects with the same rule (`:563-578`). A run's lease is `timeoutAt + taskExecutionGrace` (`internal/service/task_service.go:671`, grace 30 s at `:39`). Two schedulers that select the same pending task are already safe: the second `MarkAsRunningTx` re-evaluates its `WHERE` on the committed `running` row and gets `ErrTaskAlreadyRunning`.
- **F2 — Picking a winner flips a running task to pending.** `SelectWinner` (`internal/service/broadcast_service.go:1449`) accepts `testing` and `processing` when `auto_send_winner` is false (`:1474-1480`), then reads the task and writes it back with `Status = pending` through `UpdateTx` (`:1510-1524`), which rewrites every column, `state` included, from that read (`task_postgres.go:269-342`). With `TASK_SCHEDULER_ENABLED` (default true, `config/config.go:536`) it starts `ExecutePendingTasks(1)` 100 ms later (`broadcast_service.go:1528-1538`); otherwise the next tick takes the task. The second execution starts from the state `SelectWinner` read, while the first one sees the winner on its next refresh and carries on in the winner phase (`internal/service/broadcast/orchestrator.go:986-994`).
- **F3 — Pausing then resuming does the same.** `handleBroadcastPaused` writes `paused`, a stale state and `timeout_after = NULL` whatever the task's status (`task_service.go:1277`, `task_postgres.go:919-970`). `handleBroadcastResumed` then writes `pending` through `UpdateTx` (`task_service.go:1340-1350`). A run reads the broadcast's status only between batches (`orchestrator.go:953-984`), so when both land inside one batch it never sees the pause, and the resumed task is taken by a second execution.
- **F4 — A run's end writes the status unconditionally.** `MarkAsPendingTx` (`task_postgres.go:979-1033`) has no condition on the status; `ExecuteTask` calls it after every slice (`task_service.go:945`) and to reschedule recurring tasks (`:880`). Once two executions overlap, the first to finish puts the task back to pending while the other still runs, and the next tick starts a third. `MarkAsCompletedTx` (`:803-847`) and `MarkAsFailedTx` (`:857-909`) have no status condition either. The end-of-run write is also the only one that still lands after something flipped the task's status during the run: `SaveStateTx` only writes a `running` row (`task_postgres.go:706-711`), so the cursor the run reached reaches the database through it.
- **F5 — `TriggerTask` checks the status in memory and then writes it.** It accepts recurring tasks only (`task_service.go:1619-1621`), so never a broadcast task, but it refuses a running task it has read (`:1624-1626`) and then calls `MarkAsPending` (`:1636`), so an execution of an integration sync that starts in between is overtaken the same way.
- **F6 — Nothing deduplicates downstream.** `email_queue` and `message_history` have no unique key on the broadcast and the contact (`internal/database/init.go:197-231`, `:508-532`); each enqueued email gets a fresh message id (`internal/service/broadcast/queue_message_sender.go:145`).
- **F7 — A run that starts after a manual pick loads every variation.** The phase is set only when it is empty (`orchestrator.go:775`); a run whose saved phase is `test` keeps it, loads the templates of every variation (`:843-863`, `:867`), and only then switches to `winner` (`:932-936`). The queue sender picks a template at random whenever it holds more than one (`queue_message_sender.go:432-459`), and `SendBatch` reads the broadcast for every batch (`:118`) but does not look at its winner. This is the console flow of the table above.
- **F8 — A run that switches to the winner phase mid-run keeps them too.** The switches at `orchestrator.go:988-994`, `:1006-1010`, `:1050-1054` and `:1115-1119` leave `templates` as loaded.
- **F9 — Automatic winner selection is not affected.** `evaluateWinner` sets the phase to `winner` before the templates load (`orchestrator.go:756`, `:1596`), so only the winner is loaded.
- **F10 — The other writers of a broadcast task's status.** `handleBroadcastScheduled` (`task_service.go:1118`) updates an existing task only for a broadcast being scheduled, which has no running task; `handleBroadcastCancelled` (`:1535`) calls `MarkAsFailed`, which clears the lease and, while retries remain, leaves the task `pending` for `RetryInterval` (300 s for a broadcast task, `:1160-1161`) rather than failed (`task_postgres.go:870-875`); the running execution stops on its next refresh (`orchestrator.go:958-968`) and ends `completed`, and a later run stops the same way, so a cancellation sends nothing twice. `handleBroadcastFailed` (`:1473`) and `handleBroadcastSent` (`:1416`) are subscribed but nothing publishes `broadcast.failed` or `broadcast.sent`. `PauseBroadcast` and the provider circuit breaker only act on a `processing` or `processed` broadcast (`broadcast_service.go:507-514`, `:953`), and `CancelBroadcast` adds `scheduled` and `paused` (`:784-791`), so none of them meets the A/B statuses. The orchestrator publishes `broadcast.paused` itself for the circuit breaker and recipient feed failures (`orchestrator.go:1222`, `:1283`) through `Publish`, whose handlers run in their own goroutines (`internal/domain/event.go:163-175`), and then returns an error, which `ExecuteTask` turns into `MarkAsFailed`. The queue worker's provider circuit breaker pauses every broadcast on the integration and publishes `broadcast.paused` too (`PauseForCircuitBreaker`, `broadcast_service.go:934-1011`, called from `internal/app/app.go:1220`); a run holding the task sees that pause on its next refresh like a user's. `PauseBroadcast` and `ResumeBroadcast` publish with `PublishWithAck` inside their transaction (`broadcast_service.go:552`, `:708`), so the task handlers run before the broadcast's new status commits.

## The fix

### Part 1 — waking a task leaves a running execution alone

**New method — `internal/domain/task.go`, `TaskRepository`:**

```go
// WakeTask puts a waiting task back in the queue: pending, due at nextRunAfter, with a
// fresh retry budget and no error message. It returns false, and changes nothing, for a
// task an execution holds (running, unless its lease has expired) and for a completed
// one. It writes neither the state nor the progress: they belong to the execution that
// saves them.
WakeTask(ctx context.Context, workspace, id string, nextRunAfter time.Time) (bool, error)
```

**Implementation — `internal/repository/task_postgres.go`:** one conditional `UPDATE … SET status = 'pending', next_run_after = $, retry_count = 0, error_message = NULL, timeout_after = NULL, updated_at = $now WHERE id = $ AND workspace_id = $ AND ((status IN ('pending', 'paused', 'failed') AND (timeout_after IS NULL OR timeout_after <= $now)) OR (status = 'running' AND timeout_after <= $now))`, the same shape as `MarkAsRunningTx` after Part 3. The held rule is one helper, `notHeld`, shared by `WakeTask`, `MarkAsPausedTx` and `MarkAsRunningTx` (added after the code review: a waiting row with a live lease is held too, so a wake or a pause must not clear its lease). One row → `true`. No row → a `SELECT 1` tells a missing task (`domain.ErrTaskNotFound`) from a held or completed one (`false`). The running clause is the one `MarkAsRunningTx` uses to reap, so a `running` row with no lease counts as held. Times in UTC (the columns are `TIMESTAMP WITHOUT TIME ZONE`, as `MarkAsRunningTx` assumes). PostgreSQL re-evaluates the `WHERE` of an `UPDATE` that waited on the row lock, so a wake and a launch never both win. Mock regenerated with the existing `//go:generate` line.

**Callers:**
1. `SelectWinner` (`broadcast_service.go:1510-1524`): `GetTaskByBroadcastID` for the id, then `WakeTask` instead of mutating and `Update`. When it returns false, log at info that the running execution switches to the winner itself (F2), and return success. Start the immediate execution only when the task was woken. `ErrTaskNotFound` keeps today's "no task" answer (`:1512-1515`). A failed task gets a fresh retry budget, as a resume already gives it.
2. `handleBroadcastResumed` (`task_service.go:1340-1380`): keep the completed check (`:1329`), replace the field mutations and `Update` with `ResetRetryBudget` then `WakeTask`; when the wake returns false, log and skip the immediate execution. `ResetRetryBudget` was added after the third review: the resume promises a fresh retry budget, and without it a run that failed after a refused wake counted on from the retries it had already spent, so its next run could be its last. It comes before the wake (fifth review): reset after a refused wake, a run that failed for good in between was left failed, with nothing to wake it, under a broadcast the resume had just set sending. In this order, a run that fails before the reset is woken by the wake, and one that fails after it reads the fresh budget and is retried. `ResetRetryBudget` is one `UPDATE tasks SET retry_count = 0, updated_at = $now WHERE id = $ AND workspace_id = $ AND status <> 'completed'`: it leaves the status, schedule, lease, state and progress to the run that holds the task, and does nothing for a missing task.
3. `TriggerTask` (`task_service.go:1634-1636`): keep its checks, replace `MarkAsPending` with `WakeTask`; false → `&domain.ErrTaskAlreadyRunning{TaskID}`. `MarkAsPending` already resets the retry count there, so the only other difference is that it no longer rewrites the state it read (F5).

When the task is held, nothing is lost: a run that is sending sees the winner or the resume on its next refresh (`orchestrator.go:953-994`), and a run that is already ending writes `pending` due now (`task_service.go:941-945`), so the next tick resumes the broadcast; a run that fails instead is retried after its retry interval, with the budget a resume reset. In the console flow the held run is one of the short waiting runs at `test_completed`, whose `handleTestPhaseCompletion` re-reads the broadcast and switches to the winner when it sees it (`orchestrator.go:1494-1510`).

### Part 2 — pausing leaves a running execution alone

`MarkAsPausedTx` (only caller: `handleBroadcastPaused`) adds the held rule of `WakeTask` to its `UPDATE` (`notHeld` with `pending`, `paused` and `failed` as the waiting statuses, so a task its run completed after the handler read it stays completed; found in the second review) and no longer writes `state` or `progress`, so its signature loses both: the handler read them before the write, and a run that ended its slice in between saved a later cursor, which the pause then rewound (found in the code review; the next run resent that run's last batch); no row → a `SELECT 1` returns `domain.ErrTaskNotFound` ("task not found", the message of today's error) for a missing task and `&domain.ErrTaskAlreadyRunning{TaskID: id}` for a held one. `handleBroadcastPaused` treats that error as expected: info log, the running execution sees the pause before its next batch, pauses the rows it enqueued and stops (`orchestrator.go:970-984`). Its end then writes `pending`, as it already does today after `MarkAsPaused` (F4), so nothing changes once that execution has ended. Its `SaveState` calls keep landing, where today they stop at the pause (F4). A pause from the orchestrator's own circuit breaker or feed failure (F10) ends in `MarkAsFailed`, as it does today. The handler runs inside the pause's transaction (F10), so the run may read the broadcast as still sending on one more refresh and enqueue one more batch, which the service's `PauseBySourceTx` has not paused; the run pauses it when it sees the pause (`orchestrator.go:980`), as today.

### Part 3 — the launch gate also checks the lease

`MarkAsRunningTx` and `GetNextBatch`: a `pending` or `paused` task is taken, and selected, only when `timeout_after IS NULL OR timeout_after <= now`. The `running` clause is unchanged. Every end-of-run write clears `timeout_after` (`MarkAsPendingTx`, `MarkAsCompletedTx`, `MarkAsFailedTx`, `ReleaseTaskTx`, `MarkAsPausedTx`), so only a write that set the status during a run leaves a live lease on a waiting task. This is the "check before launching" safety net for any such writer, present or future (`UpdateTx` callers included). Filtering in `GetNextBatch` too keeps the scheduler from dispatching such a task on every tick only to have the launch refused and logged as an error (`task_service.go:570-575`).

Other task types: the remaining `UpdateTx` callers that set `pending` (`EnsureContactSegmentQueueProcessingTask`, `EnsureSegmentRecomputeTask`, run when a workspace is created, `workspace_service.go:374-380`) copy the live lease they read, so they stop starting a second execution of a task that is running; the waiting task starts once the running one has ended, since its `MarkAsPending` clears the lease, or once the lease has expired.

### Considered and dropped: a run's end only writes a task it still holds

A `MarkRunAsPending` conditioned on `status = 'running'` was in the first draft. Parts 1 to 3 already keep a second execution from starting, so the overlap it guarded against (F4) no longer happens, and it would lose data: when anything flips a running task's status, the run's `SaveState` calls stop landing (F4) and its end-of-run write is the only one that saves its cursor, so refusing it would make the next run resend everything the first one enqueued after the flip. A cancelled broadcast's task that the end puts back to pending is stopped by the next run, which sees the cancellation on its first refresh (`orchestrator.go:958-968`). The end-of-run writes stay as they are.

### Part 4 — the winner phase sends the winner only

**`internal/service/broadcast/orchestrator.go`:**
1. Move the switch to `winner` of a run that starts after the pick (`:932-936`) before the template selection (`:843`), so such a run loads only the winning template (F7).
2. Before `SendBatch` (`:1147`), when the phase is `winner`, pass a map holding only `*broadcast.WinningTemplate` (the broadcast is refreshed each iteration, `:955`). Load it if it is not in the map, through `LoadTemplates` then `ValidateTemplates` as at `:867-892`, since `LoadTemplates` skips a template it can't load (`:152-165`). A `winner` phase without a winning template, or a winner that can't be loaded or validated, fails the run like the existing check at `:851-855` instead of sending another variation (F8).

## Not in scope

- **A crash between an enqueue and the cursor save.** The enqueue commits in the workspace database and the cursor in the system database (`orchestrator.go:1366`), so a kill in between resends that batch, at most `FetchBatchSize` contacts; so does a run that fails after a cursor save failed (`:1366-1386`), since `MarkAsFailed` writes no state. It can't be closed reliably without a per-recipient record: the worker deletes the queue row before writing `message_history` (`internal/service/queue/worker.go:459`, `:469`).
- **A tick that takes the task between `WakeTask` and the commit of the broadcast.** `SelectWinner` and `handleBroadcastResumed` wake the task inside the broadcast's workspace transaction, as their `Update` does today. A run starting in that instant reads the broadcast without its winner, and may then overwrite it (next item but one), or still paused, and then re-pauses its rows after the resume committed (next item). No duplicate. Waking after the commit would change what a failed wake does to the pick or the resume.
- **An execution stuck past its lease plus grace** is cancelled and marked failed (`task_service.go:812-842`); `SaveStateTx` still ignores a row it no longer holds (`task_postgres.go:710`) instead of telling the run to stop, and the orchestrator saves with `context.Background()` (`orchestrator.go:427`), so a goroutine still unwinding could write an older cursor over a later run's while that run holds the task.
- **A paused broadcast's task runs again on every tick** until resumed, once one execution has run while it was paused: that execution ends with `pending` due now (F4, `task_service.go:941-945`). Today a pause that lands during a run already ends this way, since the run's end overwrites `MarkAsPaused`; a pause between two slices still parks the task for 24 hours (`:1274-1277`). With Part 2, a pause that lands during a run that is then interrupted (shutdown, or a crash once the lease expires) ends this way too, where it used to stay parked, since the pause no longer takes the task from its run. No duplicate: each of those runs sees the pause on its first refresh and stops. Ending such a run with `paused` needs the processor to report why it stopped.
- **`ResetTask`'s read-then-write race**: it checks `failed` in memory, then calls `MarkAsPending`, as `TriggerTask` did. Recurring tasks only, and it takes two resets of the same task and a tick between them.
- **`handleTestPhaseCompletion` can overwrite a winner picked between its re-read and its update** (`orchestrator.go:1495-1533`, `UpdateBroadcastTx` writes `winning_template`). So can the first sending run's switch to `testing` (`orchestrator.go:772-813`) for a pick that lands between its read and its write. Both write a whole broadcast row the run read; both predate this fix, and the pick then has to be made again at `test_completed`.
- **An end-of-run write racing a run that took the task at the lease's expiry.** The execution's deadline is its lease (`task_service.go:608`, `:671`), so it would take a run that overran its slice by the whole grace and finished in the same instant. Closing it needs an execution token, which is a schema change.
- **Queue rows paused after a resume.** A run that saw the pause just before a resume committed calls `pauseQueueEntries` (`orchestrator.go:980`), which pauses every queued row of the broadcast, after the resume resumed them. They stay paused until the broadcast is paused and resumed again. Unchanged by this fix, and no duplicate.
- **A wake during the re-dispatch gap of an interrupted run.** `ReleaseTask` (shutdown) and `MarkAsFailed` (deadline) put the task back to `pending` with no lease and `next_run_after` a grace or a retry interval ahead, while the interrupted run's goroutine may still be unwinding. `WakeTask` sees no lease, so a pick or a resume in that window makes the task due now, as their `Update` does today. The unwinding run can then save an older cursor over the new run's, the one-batch resend of the first item. Covering it means giving those writes a lease until `next_run_after`, which would also hold back a resume after an ordinary failure.
- **A broadcast marked failed while its task still retries.** The orchestrator decides that a run is the last attempt when the run starts (`isLastRetry`, `orchestrator.go:482`), one attempt before `MarkAsFailedTx` stops retrying (`task_postgres.go:870`), and on a failure its deferred finalizer marks the broadcast failed (`:490-568`); the retry then resumes at the saved cursor and ends the broadcast `processed`. A resume during a held run that `ResetRetryBudget` gives a fresh budget adds the same outcome for the rest of that run, since the run keeps the decision it made at its start; the resume's `Update` on `dev` reset the count the same way. No duplicate within the broadcast. Aligning the two rules changes when every broadcast is declared failed, and the tests that pin the current rule, so it belongs in its own fix; Gemini's third review would have it here.
- **A resume whose task lookup fails** is logged at debug as "no task" (`task_service.go:1326-1331`), so a task paused between two slices waits out its 24 hours. Unchanged by this fix.
- **A run that ends after a cancellation marked its task failed** puts it back to `pending` or `completed` (`MarkAsPendingTx` and `MarkAsCompletedTx` have no status condition); the run has stopped enqueueing by then, and a later run stops on its first refresh (`orchestrator.go:958-968`).

## Code review

Max-effort review of `567d4c7f`, each finding checked against the code:

| # | Finding | Outcome |
|---|---|---|
| 1 | `handleBroadcastPaused` wrote back the state it read; a run that ended its slice in between had saved a later cursor | Fixed: `MarkAsPaused` writes neither state nor progress (Part 2) |
| 2 | `WakeTask` and `MarkAsPausedTx` cleared the live lease of a waiting row that `MarkAsRunningTx` treats as held | Fixed: one `notHeld` rule for the three (Part 1) |
| 3 | The first sending run's switch to `testing` can overwrite a pick | Pre-existing, added to "Not in scope" |
| 4 | A run that already read `paused` re-pauses the rows a resume released | Pre-existing, already in "Not in scope" |
| 5 | The queue sender could honour `WinningTemplate` itself | Kept in the orchestrator, which owns the phase and checks that the winner loads and validates |
| 6 | Integration helpers failed the test from background goroutines | Fixed: the answer and its error come back on a channel |
| 7 | The duplicate-send entry is an Improvement, not a Fix | Kept: the project's rule is to present an edge-case fix as an Improvement |
| 8 | `SelectWinner` reads the task, then wakes it in a second transaction | Kept: a manual pick, and the read gives the id the immediate execution logs |

Max-effort review of `47632968`:

| # | Finding | Outcome |
|---|---|---|
| 1 | `MarkAsPausedTx` still matched a completed task, so a pause racing the run's last `MarkAsCompleted` reopened it, and a resume ran the finished broadcast again | Fixed: the pause takes pending, paused and failed tasks only |
| 2 | The pause's lookup-error branch had no test | Test added |

Max-effort review of `779ac089`:

| # | Finding | Outcome |
|---|---|---|
| 1 | `WakeTask` makes a task in the re-dispatch gap of an interrupted run due now | Pre-existing, added to "Not in scope" |
| 2 | A resume refused by `WakeTask` dropped the fresh retry budget it promises | Fixed: `ResetRetryBudget` (Part 1) |
| 3 | A resume treats a failed task lookup as "no task" | Pre-existing, added to "Not in scope" |
| 4 | The single-execution harness had its own polling loop | Fixed: it uses `testutil.WaitForConditionWithContext` |
| 5 | The completed-task resume test still named `Update` as the forbidden write | Fixed: it names and forbids `WakeTask` |

Gemini's second counter-review (above) found nothing more to change; the log line of a resume refused by `WakeTask` now names a just-completed task too.

Max-effort review of `06510b6b` with the held-resume log line on top (a commit since folded into the next one):

| # | Finding | Outcome |
|---|---|---|
| 1 | `isLastRetry` is one attempt earlier than `MarkAsFailedTx`'s rule, so a broadcast is marked failed on an attempt that is retried | Pre-existing, added to "Not in scope" |
| 2 | `ResetRetryBudget` cannot change the decision a held run made at its start, so the window above spans the whole run | Same as `dev`; the "Not in scope" item now says so |
| 3 | Two harness helpers still had their own polling loops | Fixed: every wait goes through `testutil.WaitForConditionWithContext` |
| 4 | A commit message named the counter-review's model | Reworded before pushing |
| 5 | "Not in scope" still listed `TriggerTask`'s read-then-write race, which `WakeTask` closes | Removed |
| 6 | A recurring-task test comment said `TriggerTask` calls `MarkAsPending` | Fixed: it names `WakeTask` |
| 7 | The wake and the pause each spelled out their status list | Fixed: one `wakeableStatuses` |
| 8 | `winnerTemplates` builds a one-entry map per batch | Kept: one map per batch of recipients, and the filter next to the send covers every way into the winner phase |

Max-effort review of `540a5b34`:

| # | Finding | Outcome |
|---|---|---|
| 1 | A run that failed for good between the refused wake and the reset was left failed under a resumed broadcast | Fixed: the reset comes before the wake (Part 1) |
| 2 | A pause during a run that is then interrupted no longer parks the task, which then runs on every tick; an interruption comment still named the pause | Added to the "Not in scope" item on that loop; comment corrected |
| 3 | `ResetTask` keeps the read-then-write that `TriggerTask` lost | Pre-existing, recurring tasks only, added to "Not in scope" |
| 4 | The pause's not-found error was not `domain.ErrTaskNotFound`, unlike the wake's | Fixed, same message |
| 5 | The single-execution harness repeats the user, workspace and provider setup | Kept: each integration harness seeds its own fixtures, and sharing one means reshaping a harness other tests use |

Max-effort review of the reset-then-wake commit:

| # | Finding | Outcome |
|---|---|---|
| 1 | The interruption comment claimed `ReleaseTask` leaves alone a task another execution reaped, which its `status = 'running'` condition does not | Comment corrected; the race itself is the "end-of-run write racing a run that took the task at the lease's expiry" item |
| 2 | The resume could reset and wake in one repository call | Kept: the order is correct under every interleaving, and a resume is a user action, so the second `UPDATE` costs nothing measurable |

## Tests

| Implementation file | Test file | Cases |
|---|---|---|
| `internal/repository/task_postgres.go` | `internal/repository/task_postgres_test.go` (sqlmock) | `WakeTask`: the conditional UPDATE sets neither `state` nor `progress` and carries the held condition; one row → true; no row and the task exists → false; no row and no task → `ErrTaskNotFound`; update and select errors. `MarkAsPausedTx`: no row and the task exists → `ErrTaskAlreadyRunning`; no row and no task → the not-found error; a lookup error is reported as such; a waiting row is paused as before; the statement writes neither `state` nor `progress`. `MarkAsRunningTx` and `GetNextBatch`: the query requires an expired or empty lease for pending and paused. `ResetRetryBudget`: the statement sets `retry_count` and none of `status`, `timeout_after`, `next_run_after`, `state` or `progress`, and skips completed tasks; no row is not an error; an update error is reported. |
| same | `tests/integration/task_lease_test.go` (real PostgreSQL) | The same rules with real timestamps: a running task with a live lease is neither woken nor paused nor taken again; once the lease has expired it is; a waiting task with a live lease is neither selected by `GetNextBatch` nor taken, woken or paused, and is taken once `MarkAsPending` has cleared it; a pause keeps the cursor a run saved and leaves a completed task completed; pending, paused and failed tasks are woken and a completed one is not; `WakeTask` keeps the state a run saved; a running row with no lease is neither woken nor paused; `ResetRetryBudget` gives a held task its retries back and leaves it running with its lease and state, so a failure after it is retried, and leaves a completed task's count as it was. |
| `internal/service/broadcast_service.go` | `internal/service/broadcast_service_test.go` | `SelectWinner` calls `WakeTask` with the task's id and never `Update`; a held task (false) still saves the winner and returns nil; `ErrTaskNotFound` returns nil; another `WakeTask` error is returned. |
| `internal/service/task_service.go` | `internal/service/task_service_test.go` | `handleBroadcastResumed` calls `ResetRetryBudget` then `WakeTask`, in that order and neither for a completed task, starts the immediate execution only when woken, and still wakes the task when the reset fails; `handleBroadcastPaused` accepts `ErrTaskAlreadyRunning` and still pauses a waiting task; `TriggerTask` calls `WakeTask` and maps false to `ErrTaskAlreadyRunning`. Existing expectations on `Update` in `handleBroadcastResumed` and on `MarkAsPending` in `TriggerTask` move to `WakeTask`. |
| `internal/service/broadcast/orchestrator.go` | `internal/service/broadcast/orchestrator_winner_templates_test.go` (new, gomock) | A run starting with phase `test` on a `winner_selected` broadcast loads and sends only the winner; a run that sees the winner on its refresh sends its next batch with the winner only; a winner phase with no winning template, or a winner that can't be loaded or fails validation, fails without sending. |
| whole flow | `tests/integration/broadcast_single_execution_test.go` (new) | Deterministic, a table lock holding the first run inside a batch and the second execution asked for through `tasks.execute` as a tick would: winner picked through the API mid-run, and pause then resume mid-batch, each refuse the second execution and enqueue every contact once, the winner phase only with the winner, and leave no row of the broadcast paused. Console flow: winner picked at `test_completed`, every email enqueued after the sample uses the winner. E2E with the live scheduler and Mailpit: the API pick during the test phase delivers one email per contact and only the winner outside the sample. |

No test name or comment states a count that an assertion doesn't pin.

## Changelog (`## [42.0] - WIP`)

- **Fix**: When an A/B test's winner is picked by hand, the rest of the list now receives only the winning variation.
- **Improvement**: A broadcast now reaches each contact once when its A/B winner is picked through the API during the test phase, or when it is paused and resumed while a batch is being prepared.

## Steps

1. Tests first, per the table, and see each new one fail for the reason it names.
2. Parts 1 to 4, regenerating the task repository mock.
3. `make test-repo`, `make test-service`, `make test-http`, `make test-unit`.
4. `INTEGRATION_TESTS=true go test -race -tags integration,licdev -run 'TaskLease|BroadcastSingleExecution|BroadcastABTesting|BroadcastPhase|BroadcastPause|BroadcastResume|BroadcastInterrupt|BroadcastRetryBudget|BroadcastEnqueueFailure|BroadcastTaskTimeout|CircuitBreakerBroadcast|TaskConcurrent|TaskExecute|Cron_|RecurringTask' ./tests/integration/ -v`, then `make test-integration`.
