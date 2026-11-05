# Hang-up test reads the task status before it is written — plan

## The problem

`TestTaskExecute_ClientHangupDoesNotAbortTask` (`tests/integration/broadcast_interruption_test.go`) failed on the `dev` push run after the merge of #489:

```
after hang-up: enqueued=300 task_status=running retry_count=0 error=""
expected: "completed"  actual: "running"
```

The test waits until the 300 queue rows are visible (`waitForQueuedCount`), then reads the task row once. The task is marked completed by `MarkAsCompleted` (`internal/service/task_service.go`), a separate write that follows the commit of the last enqueue batch. A read between the two sees `running`. The harness runs no email queue worker, so the change merged in #489 is not on this path; the test passed on that PR's CI and in a full local run.

## Fix

In that test, wait for the task row to read `completed` (a new `waitForTaskRow`, which returns the whole row from one query; the existing `waitForTaskStatus` now wraps it), then count the queue once. `completed` is written after the last batch commits, so the count read then is final. Assertions are unchanged: a run that never completes still fails, at the end of the wait.

`TestCron_ClientHangupDoesNotAbortTasks` and `TestCron_ReturnsImmediately` stop at the same point, and the first asserts `retry_count` on a task that can still be running. Both now wait for, and assert, `completed` the same way. This does not wait for the whole cron run to end, only for the broadcast task.

## Not in scope

- Production code: the completion write coming after the last enqueue is expected.

## Tests

| File | Change |
| --- | --- |
| `tests/integration/broadcast_interruption_test.go` | `TestTaskExecute_ClientHangupDoesNotAbortTask`, `TestCron_ReturnsImmediately` and `TestCron_ClientHangupDoesNotAbortTasks` wait for the task to complete |

## Plan review outcomes

A Gemini counter-review confirmed the diagnosis (the queue rows are in the workspace database, the task row in the system one, with no transaction across them) and pointed out that both cron tests end at the same point. Adopted: they wait for and assert `completed` too.

## Final step — run the tests

- `INTEGRATION_TESTS=true go test -race -tags integration,licdev -count=20 -timeout 40m ./tests/integration/ -run 'TestTaskExecute_ClientHangupDoesNotAbortTask|TestCron_'`
- `make test-integration`
