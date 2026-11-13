# tasks.execute lookup errors — plan

## Goal

`POST /api/tasks.execute` is the endpoint the scheduler's HTTP fan-out calls to run a task
when the internal scheduler is off (external cron calls `/api/cron`, which then dispatches
each due task to `tasks.execute`). Two of its answers get a failure
on our side wrong, the same pattern #569 fixes on `tasks.reset` and `tasks.trigger`:

1. The handler reads the task with `TaskService.GetTask` to compute its timeout, and
   answers **any** error from that read with **404 and the raw error text**
   (`WriteJSONError(w, err.Error(), http.StatusNotFound)`), a database failure included
   (`pq: …`, a connection error).
2. `TaskService.ExecuteTask` turns **any** error from its `GetTx` read into
   `*domain.ErrNotFound`, so a database failure between the handler's read and the
   service's read also answers 404, and the original error survives only in a log line.

A failure on our side must be a 500 with a generic message, logged with its error; 404
stays for a task that does not exist.

No schema change, no migration, no environment variable.

## Relation to #569

This fix needs the repository to return `domain.ErrTaskNotFound` for a missing task
(`GetTx` on `sql.ErrNoRows`, and `Get`, which goes through `GetTx`). #569 introduces that,
but this branch must not depend on its merge order: without it, `errors.Is` would not match
dev's `fmt.Errorf("task not found")` and a missing task would answer 500, which two existing
integration tests catch (`tests/integration/task_handler_test.go`, "should fail with invalid
task ID" and "execute endpoint accepts a signed dispatch").

So this branch carries #569's `GetTx` hunk and its test assertion **byte for byte**
(identical hunks merge without conflict, whatever the order). The `MarkAsPendingTx` hunk of
#569 is not needed here and is left to it.

## Changes

### `internal/repository/task_postgres.go`

`GetTx` returns `domain.ErrTaskNotFound` for `sql.ErrNoRows` instead of
`fmt.Errorf("task not found")`, identical to #569. The text is unchanged, so the callers
that match the string (the `GetTask` handler, `DeleteTask`, the fallback in
`authorizeExistingTask`) behave as before. No other consumer of `Get` / `GetTx` branches on
the error (`broadcast/orchestrator.go`, `MarkAsFailedTx`).

Test file: `internal/repository/task_tx_branches_test.go`, `TestTaskRepository_GetTx_Branches`
"no row": `assert.ErrorIs(t, err, domain.ErrTaskNotFound, …)`, the same line as #569.

### `internal/service/task_service.go`

`ExecuteTask`, the `GetTx` error inside the setup transaction:
- `errors.Is(err, domain.ErrTaskNotFound)` → `&domain.ErrNotFound{Entity: "task", ID: taskID}`
  (unchanged answer for a missing task);
- any other error → `fmt.Errorf("failed to load task for execution: %w", err)` (the
  repository already prefixes "failed to get task: "), so the caller can see the cause with
  `errors.Is` and the handler's default branch answers 500.
The log line ("Failed to get task for execution") is kept for both.

`WithTransaction` returns the callback's error unwrapped, so the handler's type switch
sees the `*domain.ErrNotFound` as before.

The other callers of `ExecuteTask` (the in-process scheduler, the segment service's
triggers, which run in goroutines) only log its error; nothing there branches on `*domain.ErrNotFound`.

Test files:
- `internal/service/task_service_test.go`, `TestTaskService_ExecuteTask` "Task not found
  error": the `GetTx` mock switches from `fmt.Errorf("task not found")` to
  `domain.ErrTaskNotFound`, as the repository now returns (regression guard for the 404).
- new `internal/service/task_service_execute_lookup_errors_test.go`,
  `TestTaskService_ExecuteTask_LookupFailure`: `GetTx` answering a database error →
  `errors.Is` that error, not a `*domain.ErrNotFound`, `MarkAsRunningTx` not called (strict
  mock). Fails on dev, which answers `*domain.ErrNotFound`.

### `internal/http/task_handler.go`

`ExecuteTask`, the `GetTask` read:
- `errors.Is(err, domain.ErrTaskNotFound)` → 404 "Task not found";
- any other error → logged (task id, workspace id, error) and 500 "Failed to get task",
  no internal text in the body.
The `ExecuteTask` service error mapping is unchanged: `*domain.ErrNotFound` → 404, a wrapped
lookup failure falls to the default branch → logged, 500 "Failed to execute task".

Test files:
- `internal/http/task_handler_test.go`, `TestTaskHandler_ExecuteTask` "Task not found
  error": the `GetTask` mock switches from `&domain.ErrNotFound{…}` (which `GetTask` never
  returns) to `domain.ErrTaskNotFound`, and asserts the body "Task not found" (fails on dev,
  which echoes the lowercase error text).
- new `internal/http/task_handler_execute_lookup_errors_test.go`,
  `TestTaskHandler_ExecuteTask_LookupErrors`, with HTTP dispatch enabled and a request signed
  with `newSignedExecuteRequest`:
  - `GetTask` answers a database error (`errors.New("pq: connection refused")`) → 500 with
    exactly "Failed to get task", the internal text absent, `ExecuteTask` not called. Fails
    on dev (404 with the text).
  - regression guards (pass on dev too): `GetTask` succeeds and `ExecuteTask` answers
    `fmt.Errorf("failed to load task for execution: %w", dbErr)` → 500 "Failed to execute
    task", the internal text absent; `ExecuteTask` answers `&domain.ErrNotFound{…}` → 404.

### `CHANGELOG.md`

Under 44.0, one **Improvement** line: with the internal scheduler off, `tasks.execute`
answers 500 without the internal error when the database fails while reading the task, and
404 only for a task that does not exist. Placed apart from #569's line so the two merge
without conflict; once both are in, the two lines can be folded into one.

## Known, not handled here

- The `GetTask` and `DeleteTask` handlers still map an error by its text ("not found"),
  and `authorizeExistingTask` falls back to it after `errors.Is`; no realistic database
  failure text matches, so it is left as is.
- No test pins that `repo.Get` passes the sentinel through `WithTransaction` unwrapped; the
  integration tests above cover it end to end.

## Verification

```bash
go test -race ./internal/repository/ -run 'TestTaskRepository_GetTx'
go test -race ./internal/service/ -run 'TestTaskService_ExecuteTask'
go test -race ./internal/http/ -run 'TestTaskHandler'
make test-unit
INTEGRATION_TESTS=true go test -race -tags integration,licdev ./tests/integration/ -run 'TestTaskHandler$|TestTaskAuthentication$'
```

## Spec review

Reviewed by an independent sub-agent on 2026-11-13. Folded in above: carrying #569's
`GetTx` hunk so the branch does not depend on the merge order (two integration tests would
otherwise turn red), the two existing tests that change (`task_service_test.go` and
`task_handler_test.go` "Task not found error"), the integration command, a service prefix
that does not repeat the repository's, test names matching the `-run` patterns, which
tests prove the change and which are guards, and the changelog wording.
