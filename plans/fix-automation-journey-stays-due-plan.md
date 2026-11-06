# Plan — a contact between two automation steps stays due for the scheduler

## Symptom

`TestAutomation/PauseResume` (`tests/integration/automation_e2e_test.go`) failed once on dev
(bb4cb50, Go run 42184954684, attempt 1) and passed on re-run. After `automations.activate`,
the test's copy of the scheduler query no longer returned the contact. In the log, a batch of
the test app's automation scheduler (`Processed automation batch processed:3`, 70 ms) ran across
the whole pause → resume window (47.618 → 47.688), and the failing query ran at 47.681.

## Root cause (in the product, not the test)

`AutomationExecutor.Execute` (`internal/service/automation_executor.go`) walks several nodes in
one tick and persists the contact automation after every node, "for crash recovery":

```go
contactAutomation.CurrentNodeID = result.NextNodeID
contactAutomation.ScheduledAt = result.ScheduledAt   // nil for every node but delay
...
updated, err := e.automationRepo.UpdateContactAutomationIfActive(ctx, workspaceID, contactAutomation)
```

Only `DelayNodeExecutor` returns a `ScheduledAt`; every other node returns nil, and
`UpdateContactAutomationIfActive` writes `scheduled_at = NULL`. Between two steps of the same
tick, the row is `status='active'`, `current_node_id=<next node>`, `scheduled_at=NULL`. The
scheduler query (`GetScheduledContactAutomations`) selects `ca.scheduled_at <= $1`, which NULL
never satisfies.

The race in the test: the scheduler's batch read the contact, met the paused automation (no-op
or other contacts first), and processed it right after the resume: trigger node persisted with
`scheduled_at = NULL`, the test's query ran (contact not found), then the delay node persisted
`scheduled_at = now + 1 min`.

The same NULL is a real defect whenever the tick stops before the next persist:

1. **More than ten consecutive non-delay steps.** `maxNodesPerTick = 10`; the comment says
   "remaining nodes picked up next tick. State already persisted, so this is safe", but the state
   persisted has `scheduled_at = NULL`, so no later tick picks the contact up: the journey stays
   `active` forever at node 11.
2. **A crash or restart between two persists** of one tick: same stranded row.

## Fix

In `Execute`, after the status is decided, when the contact is still `active` and the node gave
no `ScheduledAt`, persist a resume time instead of NULL:

- on the last iteration of the tick (`iterations == maxNodesPerTick-1`), `now`: the contact is
  handed to the next tick and is due immediately;
- on earlier iterations, `now + 5 min` (a lease, constant `inTickResumeLease`): the executor is
  still stepping this contact, so another instance must not pick it up mid-tick (today the NULL
  happens to prevent that, forever), and a crash before the next persist resumes the journey
  five minutes later. The lease is renewed after every step, so it only has to outlast one
  node (a webhook node waits 30 s at most).

A delay node keeps its future time; completed and exited rows keep `scheduled_at = NULL` as today,
and `handleError` clears it on a journey that fails for good (it would otherwise keep the lease).
The loop's delay exit keeps testing `result.ScheduledAt`, not the lease.

No schema change, no migration, no environment variable.

Not in this PR (decision for Pierre, needs his approval since it is a data migration): repairing
rows already stranded in production (`status='active' AND scheduled_at IS NULL AND
current_node_id IS NOT NULL`).

## Tests (written first)

`internal/service/automation_executor_test.go`

- `TestAutomationExecutor_Execute_StatePersistsAfterEachNode`: also capture `ScheduledAt` at each
  persist; the two intermediate persists carry a time about five minutes ahead (the lease), the
  completing one nil.
- `TestAutomationExecutor_Execute_MaxIterationsLimit`: after the tenth node the contact automation
  is active at node11 with a non-nil `ScheduledAt` not after now; the nine earlier persists carry the
  lease.
- `TestAutomationExecutor_Execute_LoopStopsAtDelayNode`: unchanged, still asserts the delay's
  future time (guards against overwriting it).

`tests/integration/automation_e2e_test.go`

- New subtest `LongChainWithoutDelay`: live automation with a trigger then twelve list steps
  (add_to_list / remove_from_list alternately on one list), no delay. The contact must reach
  `completed` within 10 s. On current code it stays `active` after the tenth node (fails).
- `PauseResume` unchanged: with the fix, its step 10 query (`scheduled_at <= now + 1 h`) sees the
  contact at every point of a concurrent tick (lease of 5 min, or the delay's 1 min).

## CHANGELOG

`[42.3] - WIP`, one **Improvement** line: journeys of more than ten steps in a row without a
delay go through to the end, and a journey interrupted between two steps resumes.

## Gemini counter-review

Raw: `reviews/fix-automation-journey-stays-due-gemini-brut-plan.md` (project files). Point 1 not
confirmed; points 2 and 3 confirmed and folded into the fix (lease instead of `now`). Details in
`reviews/fix-automation-journey-stays-due-gemini-verif-plan.md`.

## Known, not handled

- `GetScheduledContactAutomations` uses `FOR UPDATE ... SKIP LOCKED` outside a transaction, so the
  lock ends with the statement and two instances can pick the same contact (already true today for the contacts of a batch not yet
  processed). Out of scope.

## Commands

```
go test ./internal/service/ -run 'TestAutomationExecutor' -count=1
make test-service
go test ./tests/integration/ -run 'TestAutomation$' -count=1   # with the test database up
```
