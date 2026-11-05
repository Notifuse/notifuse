# Hang-up test reads the task status before it is written — plan

## The problem

`TestTaskExecute_ClientHangupDoesNotAbortTask` (`tests/integration/broadcast_interruption_test.go`) failed on the `dev` push run after the merge of #489:

```
after hang-up: enqueued=300 task_status=running retry_count=0 error=""
expected: "completed"  actual: "running"
```

The test waits until the 300 queue rows are visible (`waitForQueuedCount`), then reads the task row once. The task is marked completed by `MarkAsCompleted` (`internal/service/task_service.go`), a separate write that follows the commit of the last enqueue batch. A read between the two sees `running`. The harness runs no email queue worker, so the change merged in #489 is not on this path; the test passed on that PR's CI and in a full local run.

## Fix

In that test, wait for the task row to read `completed` (a new `waitForTaskRow`, which returns the status, retry count and error read in one query; the existing `waitForTaskStatus` now wraps it), then count the queue once. `completed` is written after the last batch commits, so the count read then is final. The test's assertions are unchanged: a run that never completes still fails, at the end of the wait.

`TestCron_ClientHangupDoesNotAbortTasks` and `TestCron_ReturnsImmediately` stop at the same point, and the first asserts `retry_count` on a task that can still be running. Both now wait for the task to complete and gain assertions that it did, with a clean retry count. This does not wait for the whole cron run to end, only for the broadcast task.

`TestCron_ConcurrentTriggerIsRejected` (`tests/integration/task_concurrency_test.go`) let the first run go once one row was queued, so its duplicate check could run before the run had enqueued everything and the harness could tear down under it. It now waits for, and asserts, `completed`.

## Not in scope

- Production code: the completion write coming after the last enqueue is expected.

## Tests

| File | Change |
| --- | --- |
| `tests/integration/broadcast_interruption_test.go` | `TestTaskExecute_ClientHangupDoesNotAbortTask`, `TestCron_ReturnsImmediately` and `TestCron_ClientHangupDoesNotAbortTasks` wait for the task to complete; `TestBroadcastInterrupted_ReleasesWithoutSpendingBudget` reads its pending row in one query with `waitForTaskRow`; `TestBroadcastInterrupted_ResumesAndCompletes` checks the task was handed back as `pending` |
| `tests/integration/task_concurrency_test.go` | `TestCron_ConcurrentTriggerIsRejected` waits for the first run to complete before its duplicate check |

## Plan review outcomes

A Gemini counter-review confirmed the diagnosis (the queue rows are in the workspace database, the task row in the system one, with no transaction across them) and pointed out that both cron tests end at the same point. Adopted: they wait for and assert `completed` too.

## Final step — run the tests

- `INTEGRATION_TESTS=true go test -race -tags integration,licdev -count=20 -timeout 40m ./tests/integration/ -run 'TestTaskExecute_ClientHangupDoesNotAbortTask|TestCron_|TestBroadcastInterrupted'`
- `make test-integration`
