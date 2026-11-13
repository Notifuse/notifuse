# Automations: one instance at a time per journey

With several Notifuse instances on the same database, two automation schedulers can run the same journey
at the same time. The step then runs twice, and an email step sends twice (each run makes a new
`message_id`, `automation_node_executor.go:299`). Pierre asked for the fix (2026-11-13).

## Cause (verified)

- `GetScheduledContactAutomations` (`internal/repository/automation_postgres.go:1185`) selects the due
  journeys with `FOR UPDATE OF ca SKIP LOCKED` through `db.QueryContext`, outside any transaction, so the
  row locks end with the statement.
- `ProcessBatch` (`internal/service/automation_executor.go:237`) then runs the journeys one after the
  other, from the copies it read.
- A journey is only marked taken when its first step persists (the in-tick lease, `executor.go:171-179`,
  5 min). Until then it is still due.
- So another instance that ticks while the first one is still in its batch selects the same journeys:
  every journey in both batches runs twice. `UpdateContactAutomationIfActive` checks only the status.
- Order of magnitude (estimated, not measured): about 0.5 % per wake-up at low traffic with two instances,
  about 12 % of a backlog batch, close to one in two with slow steps.

## Change

### 1. Claim at pick: `internal/repository/automation_postgres.go`

`GetScheduledContactAutomations` becomes a claim. In one statement, it selects and locks the due rows as
today, sets `scheduled_at = NOW() + lease` on them, and returns them:

```sql
WITH picked AS (
    SELECT ca.id, ca.scheduled_at AS due_at
    FROM contact_automations ca
    JOIN automations a ON ca.automation_id = a.id
    WHERE ca.status = 'active' AND ca.scheduled_at <= NOW()
      AND a.status = 'live' AND a.deleted_at IS NULL
    ORDER BY ca.scheduled_at ASC
    LIMIT $1
    FOR UPDATE OF ca SKIP LOCKED
), claimed AS (
    UPDATE contact_automations ca
    SET scheduled_at = NOW() + $2 * INTERVAL '1 millisecond'
    FROM picked
    WHERE ca.id = picked.id
    RETURNING ca.id, ca.automation_id, ca.contact_email, ca.current_node_id, ca.status,
              ca.exit_reason, ca.entered_at, ca.scheduled_at, ca.context, ca.retry_count, ca.last_error,
              ca.last_retry_at, ca.max_retries, picked.due_at
)
SELECT id, automation_id, contact_email, current_node_id, status, exit_reason, entered_at,
       scheduled_at, context, retry_count, last_error, last_retry_at, max_retries
FROM claimed ORDER BY due_at ASC
```

- The data-modifying CTE runs in one statement, so the lock lasts until the update is done.
- Another instance's `SELECT … scheduled_at <= now` no longer sees a claimed row, and `SKIP LOCKED` skips
  a row that is being claimed.
- Signature: `ClaimScheduledContactAutomations(ctx, workspaceID, limit int, lease time.Duration)`, renamed from `GetScheduledContactAutomations` because it now writes. The same
  rename applies to the global variant (`ClaimScheduledContactAutomationsGlobal`), which keeps its
  round-robin.
- The rows come back with `ScheduledAt` set to the claim (`NOW() + lease`). That value is the **claim
  token** for step 2.

### 2. Re-claim before each journey: `internal/service/automation_executor.go`

A batch can outlast the lease: 50 journeys with a webhook of up to 30 s each comes to 25 min. Once a
journey's claim has expired, another instance can claim it again. So right before `Execute` on each
journey, `ProcessBatch` renews its claim with a compare-and-set on the token:

```sql
UPDATE contact_automations SET scheduled_at = NOW() + $3 * INTERVAL '1 millisecond'
WHERE id = $1 AND status = 'active' AND scheduled_at = $2
```

- New repository method `RenewContactAutomationClaim(ctx, workspaceID, id string, token time.Time,
  lease time.Duration) (renewed bool, err error)`.
- **No row:** another instance re-claimed the journey (so the token differs), or the journey was
  exited or completed meanwhile. `ProcessBatch` skips it and logs at debug level.
- **An error:** logged, and the journey is skipped. It stays claimed until its lease ends, and is then
  due again.
- The token is the `scheduled_at` that PostgreSQL returned, compared as `timestamptz` (microseconds), so
  it round-trips exactly.
- The lease is `inTickResumeLease` (5 min): it only has to cover one journey's run between the renewal
  and its first persist, which is one step (30 s at most for a webhook). After that the in-tick lease
  takes over as today.
- `Execute` itself is unchanged.

### 3. Effects to state

- **A journey whose automation is paused** between the claim and its run: `Execute` returns without
  persisting (`executor.go:85-89`), so `scheduled_at` stays at the claim. On resume it waits out the
  remaining lease (5 min at most) instead of being due at once.
- **An instance that dies in the middle of a batch:** its claimed journeys wait out the lease (5 min at
  most) before another instance takes them. Today they would be taken at the next tick, and possibly run
  twice.
- No schema change and no migration. No config change. The lease reuses the existing constant.

## Tests (written first, run red, then green)

### Repository, sqlmock: `internal/repository/automation_postgres_test.go`, `automation_postgres_coverage_test.go`
- `ClaimScheduledContactAutomations`:
  - the statement has the `FOR UPDATE OF ca SKIP LOCKED`, the `UPDATE … SET scheduled_at` and the
    `RETURNING`;
  - its args are limit and lease in ms;
  - the rows are scanned, `ScheduledAt` holds the claim;
  - empty result, scan error and query error are covered.
- The global variant keeps its round-robin tests, renamed.
- `RenewContactAutomationClaim`: true when one row is updated, false when none is, and an error is
  wrapped.

### Executor, gomock: `internal/service/automation_executor_test.go`, `automation_scheduler_test.go`
- `ProcessBatch` claims with `inTickResumeLease`, renews each journey's claim with the token it got,
  and runs `Execute` only on the renewed ones.
- It skips a journey whose renewal is false, and one whose renewal errors, and goes on with the next.
- The processed count leaves skipped journeys out.
- The scheduler tests follow the rename.
- Regenerate `internal/domain/mocks/mock_automation_repository.go` with mockgen (golang/mock v1.6.0, as
  pinned in go.mod).

### Integration, real PostgreSQL: new `tests/integration/automation_claim_test.go`
1. **Two instances, one backlog.**
   - An automation: trigger → webhook to an `httptest` receiver that counts calls per contact and answers
     after 50 ms, so a batch takes a while.
   - Enrol 40 contacts. The app's background workers are not started, so nothing processes them
     meanwhile.
   - Build two `AutomationExecutor`s on the app's repositories. A runs `ProcessBatch(ctx, 50)` in a loop;
     B starts its own loop when the receiver gets A's first call, so B claims while A's batch is running.
     Both loop until every journey is `completed`.
   - Every contact got exactly one call, and the webhook step has one `completed` execution per journey.
   - Without the fix both batches hold all 40 journeys, so the check fails (red first).
2. **Concurrent claims are disjoint.** Two goroutines call `ClaimScheduledContactAutomations` together on
   60 due journeys with limit 50. The two sets do not overlap and together hold all 60. Each
   claimed row's `scheduled_at` is in the future.
3. **An expired claim taken over.**
   - Instance A claims a journey with a 1 ms lease. Once that lease has run out, instance B claims it
     again.
   - A's renewal with its token is false, and B's renewal with its own token is true.
   - A alone, after its lease ran out, still renews (one instance).
   - A journey exited meanwhile (status `exited`) is not renewed.

## Changelog

44.0, under **Fix**, since the scheduler is released behaviour: "With several Notifuse instances, a
contact could go through the same automation step twice and receive the same email twice; an instance now
claims each journey before it runs it, so another instance leaves it alone."

## Out of scope
- The nodeStats "passes" change (another PR).
- A claim-token column, or a separate worker or leader election.

## Verification
1. `go test -race ./internal/repository -run 'ScheduledContactAutomations|RenewContactAutomationClaim'`,
   then `make test-repo`.
2. `go test -race ./internal/service -run 'ProcessBatch|AutomationScheduler'`, then `make test-service`.
3. `go vet ./...`, and `go vet -tags integration,licdev ./tests/integration/`.
4. `TEST_DB_HOST=localhost TEST_DB_PORT=15433 INTEGRATION_TESTS=true go test -race -tags integration,licdev ./tests/integration/ -run 'TestAutomationClaim|TestAutomationNodeStats$|TestAutomationTransitionRace|TestAutomationE2E' -v`,
   with a local SMTP sink on 1025. Red before the fix on case 1, then green three runs in a row.
5. Stop the PostgreSQL 16 cluster (`pg_ctlcluster 16 main stop`) and the SMTP sink.

## Plan review (Opus xhigh) and corrections

(The sections above were updated to the reviewed version, including the tests as written.)

The approach holds. Two blocking points, both corrected:

1. **The global variant trimmed the rows it fetched** (`perWorkspace = limit/len+1`, then cut to
   `limit`). Once fetching is claiming, the cut rows would stay claimed for 5 min without running.
   - Each workspace is now asked `min(perWorkspace, limit - len(collected))`, and nothing is trimmed.
   - A sqlmock test with one workspace checks that the LIMIT is 50.
   - The error the loop swallowed is logged now, since a scan error can leave rows claimed.
2. **Test 1 was not reliably red.** Two selects started on a barrier overlap, and their locks then work
   most of the time; the reviewer measured 187 of 200 runs split cleanly. The bug needs the second
   select to start after the first ended, while its batch is still running. So B's `ProcessBatch` now
   starts when the receiver gets A's first call, then both loop until every journey is completed.

Non-blocking points taken:
- **The database clock decides what is due.** The claim's WHERE uses `ca.scheduled_at <= NOW()` instead
  of the app's time, so app clock skew cannot defeat the lease. The `before` argument goes: the signature
  is `ClaimScheduledContactAutomations(ctx, workspaceID, limit, lease)`.
- **The claim lists its columns.** The final SELECT lists them (no `SELECT *`) and orders by `due_at`
  without returning it.
- **`ProcessBatch` guards the token.** It skips a journey with a nil token (logged as an error), and
  stops the loop when `ctx.Err() != nil`, so a shutdown does not log one renewal error per journey.
- **Test 3 rewritten.** A claims with a 1 ms lease. After it expires B claims, and A's renewal is false
  while B's is true. Separately, A alone after its lease expired renews true (one instance).
- **Executor tests.** The `ProcessBatch` and scheduler unit tests get the renewal expectations and a
  non-nil `ScheduledAt`.
- **The second executor in test 1.** It is built from `app.GetWorkspaceRepository()`, plus
  `NewAutomationRepository(ws, NewAutomationTriggerGenerator(NewQueryBuilder()))` and
  `NewContactTimelineRepository(ws)`, with a plain `http.Client` for the webhook.
- **Mocks.** Regenerated with `go run github.com/golang/mock/mockgen@v1.6.0`.
- The `updated_at` remark goes: `contact_automations` has no such column and no triggers.

Known, not handled:
- **A graceful shutdown leaves the claims of the journeys it had not run yet.** They wait out the lease
  (5 min at most) before another instance takes them.
- **Clocks.** The claim and its renewal use the database clock, while every `scheduled_at` the app writes
  (delays, retry backoff, the in-tick lease, the hand-off after `maxNodesPerTick`) uses the app's clock.
  What is due is now decided by the database clock, so an app clock skew shifts all of those times.
- **A context that does not decode.** If one claimed row's `context` JSON fails to unmarshal, the claim of
  that workspace returns an error after its whole batch was claimed, so that batch waits 5 min each time.
  Before this change the same error blocked the workspace on every tick.
- **Log noise.** A workspace whose claim keeps failing logs one error per tick.
- **Write load.** Each tick now writes every claimed row once, and each journey once more on renewal.
- **A poison row.** A journey whose run always fails before persisting stays claimed and is retried every
  5 min, instead of every tick.
- **A step or a stall running past the 5-min lease.** `UpdateContactAutomationIfActive` checks only the
  status. If one journey's run stalls past the lease (GC, waiting on the pool or on a lock), another
  instance can re-claim it, and the first one's later persist overwrites the second's state. Webhooks
  cap at 30 s, so this needs a stall over 4.5 min. Adding `scheduled_at = token` to every persist would
  close it, but that reaches into every write path of the executor, so it is left for later.
