# Hang-up test reads the task status before it is written — plan

## The problem

`TestTaskExecute_ClientHangupDoesNotAbortTask` (`tests/integration/broadcast_interruption_test.go`) failed on the `dev` push run after the merge of #489:

```
after hang-up: enqueued=300 task_status=running retry_count=0 error=""
expected: "completed"  actual: "running"
```

The test waits until the 300 queue rows are visible (`waitForQueuedCount`), then reads the task row once. The task is marked completed by `MarkAsCompleted` (`internal/service/task_service.go`), a separate write that follows the commit of the last enqueue batch. A read between the two sees `running`. The harness runs no email queue worker, so the change merged in #489 is not on this path; the test passed on that PR's CI and in a full local run.

## Fix

In that test, read the status with the existing `waitForTaskStatus(t, h, "completed", 10*time.Second)` (same file), then read `retry_count` and `error_message` with `taskRow` once the status has settled. Assertions are unchanged: a run that never completes still fails, after the wait instead of at once.

`TestCron_ClientHangupDoesNotAbortTasks` and `TestCron_ReturnsImmediately` stop at the same point, so their cleanup can run under the task's last writes, and the first asserts `retry_count` on a task still running. Both now also wait for, and assert, `completed`.

## Not in scope

- Production code: the completion write coming after the last enqueue is expected.

## Tests

| File | Change |
| --- | --- |
| `tests/integration/broadcast_interruption_test.go` | `TestTaskExecute_ClientHangupDoesNotAbortTask`, `TestCron_ReturnsImmediately` and `TestCron_ClientHangupDoesNotAbortTasks` wait for the task to complete |

## Final step — run the tests

- `INTEGRATION_TESTS=true go test -race -tags integration,licdev -count=20 ./tests/integration/ -run 'TestTaskExecute_ClientHangupDoesNotAbortTask|TestCron_'`
- `make test-integration`

## Plan review outcomes

Gemini (`/mnt/project-files/reviews/fix-hangup-test-task-status-gemini-brut-plan.md`) confirmed the diagnosis and pointed out that both cron tests end at the same point. Adopted: they wait for and assert `completed` too.
