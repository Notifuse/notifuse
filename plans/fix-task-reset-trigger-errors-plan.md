# Task reset / trigger errors and text timestamps — plan

## Goal

Three defects found while raising the test coverage (PR #568):

1. `POST /api/tasks.reset` and `POST /api/tasks.trigger` answer any error the service
   does not classify with **400 and the raw error text**, database failures included
   (`failed to reset task: pq: …`). A failure on our side must be a 500 with a generic
   message, logged; only a refusal the caller can act on is a 400.
2. `TaskService.ResetTask` and `TaskService.TriggerTask` turn **any** error from the task
   lookup into `domain.ErrTaskNotFound`, so a database failure between the handler's
   authorization read and the service's own read answers 404.
3. `NullableTime.Scan([]byte)` can never succeed: it delegates to `sql.NullTime.Scan`,
   which refuses `[]byte`. A timestamp read as text (a driver in text mode, a column cast
   to text) fails the whole row.

No schema change, no migration, no environment variable.

## Changes

### `internal/repository/task_postgres.go`

`GetTx` returns `domain.ErrTaskNotFound` for `sql.ErrNoRows`, and `MarkAsPendingTx`
returns it when no row is updated, instead of `fmt.Errorf("task not found")`. The text is
the same ("task not found"), so the callers that match the string (the `GetTask` handler, `authorizeExistingTask`)
or carry it on (the `ExecuteTask` handler, `MarkAsFailedTx`, the orchestrator) behave as before; callers can now use `errors.Is`. `Get` goes through `GetTx` inside
`WithTransaction`, which returns the callback's error unwrapped. The `MarkAsPendingTx`
change keeps a task deleted between reset's read and its write a 404 rather than a 500,
and makes the "recurring task deleted during execution" branch of `ExecuteTask`
(`errors.Is(err, domain.ErrTaskNotFound)` after `MarkAsPending`) reachable.

Test files: `internal/repository/task_tx_branches_test.go` (no row →
`errors.Is(err, domain.ErrTaskNotFound)` besides the text), `internal/repository/task_postgres_test.go`
(`MarkAsPending` / `MarkAsPendingTx` updating no row → `ErrTaskNotFound`).

### `internal/service/task_service.go`

`ResetTask` and `TriggerTask`:
- lookup error that is `domain.ErrTaskNotFound` → `domain.ErrTaskNotFound`;
- any other lookup error → `fmt.Errorf("failed to get task: %w", err)`;
- the state refusals become `domain.ValidationError`, built with
  `domain.NewValidationError` (a value: a pointer would not match `writeValidationError`'s
  value target and would drop to 500), the existing type the HTTP layer maps to 400: "task is not in failed state, current status:
  …", "task is not a recurring task", "task cannot be triggered in current status: …".
  Their text gains the type's "validation error: " prefix.
- `ErrTaskAlreadyRunning` and the write failures (`failed to reset task: %w`,
  `failed to trigger task: %w`) are unchanged.

Test files: `internal/service/task_service_test.go` (the two "task not found" cases mock
`Get` returning `domain.ErrTaskNotFound`, as the repository now does, instead of a plain
`"not found"` error), `internal/service/task_service_reset_trigger_errors_test.go`:
- reset / trigger: lookup failure → wraps the repository error (`errors.Is` on it), not
  `ErrTaskNotFound`;
- each state refusal → `errors.As` a `domain.ValidationError`, with its full prefixed text;
- a task deleted before the write (`MarkAsPending` / `WakeTask` answering
  `ErrTaskNotFound`) → `ErrTaskNotFound`.

### `internal/http/task_handler.go`

`ResetTask` and `TriggerTask` map the service error as:
- `domain.ErrTaskNotFound` → 404 "Task not found" (unchanged);
- `*domain.ErrTaskAlreadyRunning` → 409 (trigger, unchanged);
- `domain.ValidationError` → 400 with its text (`writeValidationError`);
- anything else → logged, 500 "Failed to reset task" / "Failed to trigger task", no
  internal text in the body.

Test file: `internal/http/task_handler_reset_trigger_errors_test.go`, for reset and
trigger, with the task found by the handler's authorization read:
- the service answers `ErrTaskNotFound`, bare or wrapped → 404 "Task not found";
- a `domain.ValidationError` → 400 with its full prefixed text;
- a database failure (`fmt.Errorf("failed to reset task: %w", …)`) → 500 with exactly
  "Failed to reset task" / "Failed to trigger task", the internal text absent.
The existing cases in `task_handler_test.go` keep passing.

### `internal/domain/nullables.go`

`NullableTime.Scan` reads a timestamp given as text, `[]byte` or `string`, instead of
delegating `[]byte` to `sql.NullTime`. Accepted layouts, tried in order: RFC 3339 with
optional fractional seconds, the same without zone (`to_json` of a `timestamp`),
PostgreSQL's text output with a numeric zone (`2006-01-02 15:04:05.999999999-07:00` and
`-07`), without zone, and a bare date. A zoneless value is read as UTC, as lib/pq does and
as the `TIMESTAMP` columns are written. Text that matches none is an error naming the
text. Refused on purpose, since a `time.Time` cannot hold them or Go cannot parse them:
`infinity` (a `time.Time` cannot hold it), BC dates, years above 9999 and historic
second-level offsets (`time.Parse` does not read them). A refused text leaves the value as it was.

No production code scans into `NullableTime` today (contacts go through `sql.NullTime`,
and lib/pq decodes timestamps to `time.Time` itself), so the change is not visible with the
shipped driver.

Test files: `internal/domain/nullable_time_text_test.go` (each layout as `[]byte` and as
`string` → the expected instant, compared with `Equal`; `invalid`, `2026-13-45`, empty,
`infinity`, a BC date → error), `internal/domain/feed_nullable_gap_test.go` (its `[]byte`
case pinned the old failure; it now scans into a fresh NULL value and pins the parsed instant), `internal/domain/nullables_test.go`
(the string case is renamed "scan text that is not a time").

### `CHANGELOG.md`

Under 44.0, an **Improvement** line naming `tasks.reset` and `tasks.trigger`: 500 when the
database fails, without the internal error, 404 only for a task that no longer exists, and
state refusals still 400 with their text now prefixed "validation error: ". No client in
the repository reads that text (the console never calls these endpoints; the integration
tests check the status only). No line for `NullableTime`.

## Interplay with PR #568

`internal/http/task_handler_refusals_test.go` (#568) uses plain `errors.New` state errors
as reset / trigger refusals and expects 400 with the bare text. Once both PRs are in, those
would answer 500. #568 switches now to `domain.NewValidationError` fixtures with prefixed
expectations: on dev today any non-not-found error answers 400 with its text, so they pass
before and after this PR, whatever the merge order.

## Known, not handled here

- `TaskService.ExecuteTask` turns any `GetTx` error into `*domain.ErrNotFound`, and the
  `tasks.execute` handler answers a `GetTask` failure with 404 and the raw error text: the
  same pattern as defect 2, on an endpoint only the scheduler calls (it logs 404 and 500
  alike), left for its own change.

## Spec review

Reviewed by an independent sub-agent on 2026-11-13. Confirmed remarks are folded in
above: the existing service tests and the `feed_nullable_gap_test.go` case that pinned the
old behaviour, `MarkAsPendingTx` returning `ErrTaskNotFound` (otherwise a task deleted
mid-reset would answer 500), handler cases that reach the service, `NewValidationError`
as a value, the zoneless RFC 3339 layout and the formats refused on purpose, the changelog
wording, and an order-independent fix of the #568 fixtures.

## Verification

```bash
go test -race ./internal/repository/ -run 'TestTaskRepository|GetTx'
go test -race ./internal/service/ -run 'TestTaskService_(ResetTask|TriggerTask|ResetAndTrigger)'
go test -race ./internal/http/ -run 'TaskHandler'
go test -race ./internal/domain/ -run 'NullableTime|Gap|Nullable'
make test-unit
```
