# nodeStats: "Reached" counted as passes, without the DISTINCT

`automations.nodeStats` (44.0, unreleased) takes 8.2 s for "All time" on an automation of 1,000,000
journeys. 3.9 s of that is the `COUNT(DISTINCT contact_automation_id)` of the visits. A first plan
(scratchpad, reviewed by Opus xhigh) tried to keep the distinct-journeys number without the DISTINCT. The
review showed this cannot be exact:
- automations may loop (no cycle check in `Automation.Validate`);
- two replicas can run the same journey (the `FOR UPDATE SKIP LOCKED` of
  `GetScheduledContactAutomations` runs outside a transaction);
- periods.

Pierre chose (decision card, 2026-11-13) to **redefine the step's count as passes**. A loop counts each
time round. A retry stays one pass. The double run across replicas is a separate bug, reported to Pierre
and not handled here.

## Definition

A **pass** on step X is one time a journey reached X. Each pass is exactly one of these:
- **Passed**: an attempt that succeeded or is still running. That is one execution row with action
  `processing` or `completed` (`skipped` is in the enum with no writer and counts with them). Failed
  attempts before it in the same pass are rows with action `failed`, so they are not counted.
- **Not passed yet**: the journey is on X now, or ended on X, without a successful attempt in this pass.
  It is still active (just enrolled at the root, or waiting for a retry), `failed` on X, or `exited` while
  waiting for a retry.

passes(X) = passed rows(X) + not-passed journeys(X). With no loops and no double run, this is the number
of distinct journeys, as today.

Field and wording stay `visits` / "Reached"; only the definition in the API reference and the docs
changes.

## Measured (local PostgreSQL 16, port 15433, `ws_explain`, 1 M journeys, 4.96 M executions, warm)

| Statement | Time |
|---|---|
| Visits today (`COUNT(DISTINCT …)` and the other counters) | 3.9 s |
| The same with `COUNT(*) FILTER (WHERE action IN ('processing','completed','skipped'))` | 0.6 s |
| The same today with `SET LOCAL work_mem = '64MB'` | 6.0 s |
| Not-passed statement (shape of the in-place one, 30,000 candidate journeys) | about 0.14 s |

Re-measured on the final SQL in step 5 of Verification.

## Change

### `internal/repository/automation_stats_postgres.go`

1. **`nodeStatsVisitsSQL`, second column.** Replace it with
   `COUNT(*) FILTER (WHERE action IN ('processing', 'completed', 'skipped'))`. Period, other columns
   and grouping stay as they are. The comment changes to describe passes.
2. **New `nodeStatsNotPassedSQL`.** It runs in the same read-only snapshot, right after the visits
   statement, and adds to `Visits`:

   ```sql
   SELECT ca.current_node_id, COUNT(*)
   FROM contact_automations ca
   CROSS JOIN LATERAL (
       SELECT e.node_id, e.action, e.entered_at
       FROM automation_node_executions e
       WHERE e.contact_automation_id = ca.id
       ORDER BY e.entered_at DESC
       LIMIT 1
   ) l
   WHERE ca.automation_id = $1 AND ca.status IN ('active', 'failed', 'exited')
     AND ca.current_node_id IS NOT NULL
     AND l.node_id = ca.current_node_id AND l.action IN ('entered', 'failed')%s
   GROUP BY 1
   ```

   - **Why the last execution.** A journey still in a pass has its last row on its current step, and that
     row is `entered` (enrolled at the root, not processed yet) or `failed`:
     - when the executor processes the step, it writes the `processing` row before `handleError`'s
       `failed` row, and `handleError` keeps `current_node_id`;
     - on a loop, earlier passes left `completed` rows on X, so "no successful row at X" would be wrong.
       Looking at the last row is right.
     - A journey that moved on after a delay has its last row on the delay (`completed`), so it does not
       count on its next step before reaching it.
   - **Period.** `periodCondition("l.entered_at", …)`: the pass counts when its last row entered in the
     period. A passed pass is dated by its successful attempt (`entered_at` of that row). So a journey
     enrolled before the period and processed in it counts in the period, once.
   - **Cost and indexes.**
     - `idx_contact_automations_automation (automation_id, status)` for the candidates.
     - `idx_node_executions_contact_automation (contact_automation_id, entered_at DESC)` for the last
       row, the same plan as the in-place statement.
     - The cost grows with the journeys not completed, which the in-place statement already pays.
   - Scan: `node(id).Visits += n`. The error is named `not passed`.
3. Nothing else changes: domain struct, service, handler, other statements, console.

### `internal/domain/automation_node_stats.go`
Comment on `Visits`: "passes on the step: each time a journey reached it (a loop counts each time
round, a retry is the same pass)".

### API reference and docs
- `openapi/paths/automations.yaml:111` and `openapi/components/schemas/automation.yaml:235`: change
  "distinct journeys that reached the step; a retry is the same visit" to passes.
- Then `make openapi-bundle` for `openapi.json`, and copy the result to `../docs/openapi.json` (docs PR).
- `../docs/features/automations.mdx:405` "Reached": "Times journeys reached the step (a loop counts each
  time round)".
- CHANGELOG: nothing. 44.0 is unreleased, and the nodeStats bullet says "visits", which stays true.

## Tests (written first, run red, then green)

### `internal/repository/automation_stats_postgres_test.go` (sqlmock)
- `visitsQuery` pins the new `FILTER (WHERE action IN ('processing', 'completed', 'skipped'))` text, so
  the old column fails the test.
- `expectNodeStatsQueries` expects `nodeStatsNotPassedSQL` after the visits statement, with the period
  args when there is a period, and the pinned text `l.action IN ('entered', 'failed')`.
- **"all time with email".** email-1's `Visits` is the visits row plus the not-passed row. A step that
  only the new statement returns gets `Visits` set and empty maps.
- **"a period bounds every statement but the journeys in place".** It also covers the new statement, with
  `l.entered_at` in the text and `$2`/`$3`.
- **New case.** An error from the new statement is returned and contains `not passed`, and the snapshot
  is rolled back.

### `tests/integration/automation_node_stats_test.go` (the real executor and scheduler)

**Existing assertions, kept or changed:**
- A's visits per step (3, 3, 3, 2, 1), and B's webhook `Visits == 1` while retrying and after the retry
  succeeds. These hold under passes: they are the regression check.
- "an email still in the queue…" inserts three more `completed` email-1 rows for the same journey. That
  is three more passes, so `Visits` becomes 6 and the message says each pass counts. The later email-1
  `Visits` assertions follow (3 → 6): "a period…" and "a member without message history read…".

**New subtests.** Each one gets its own automation and contacts, named so it does not disturb A and B.
1. **A loop.**
   - Automation L: trigger → webhook W1 (200) → webhook W2 (200) → back to W1, receiver counting calls.
     The scheduler runs at most 10 steps per tick per journey and re-schedules it now.
   - Wait for W1 `Visits >= 3`, then pause L (`automations.pause`) and wait until two reads in a row agree.
   - W1 `Visits` equals the number of `completed` W1 rows in the DB and is greater than 1, where
     DISTINCT gives 1.
   - If the journey keeps a `processing` row in flight when paused, the check counts `processing` too.
2. **Enrolled, not processed yet.**
   - Insert a `contact_automations` row on A (status `active`, `current_node_id` = A's root,
     `scheduled_at` tomorrow so the scheduler leaves it) and its `entered` row, as the enrollment
     function does.
   - The root's `Visits` goes up by 1. The insert goes on A after its other subtests, or on its own
     automation if ordering gets in the way.
3. **Retries exhausted.**
   - Automation C: trigger → webhook (always 500). After each failure, bring `scheduled_at` back as the
     retry subtest does, until the journey is `failed` (`max_retries` default).
   - Webhook `Visits` = 1, `FailedJourneys` = 1, `FailedAttempts` = the number of attempts.
4. **An exit while waiting for a retry.**
   - A second journey on C after its first failure, moved to `exited` with the same `UPDATE` as
     `ExitContactJourneysOnReply` (`status = 'exited', scheduled_at = NULL, exit_reason = 'replied'`),
     or the real call if the suite exposes the repository.
   - The webhook counts it once.
5. **A period.**
   - Backdate B's successful webhook row by 40 days and ask for the last 30 days: the webhook's pass
     leaves that period.
   - On C's failed journey, a period that contains its last failed row but not its `entered` row still
     counts the not-passed pass once.

## Known limits (written in the code comment and in the plan)

- **A process killed in the middle of a step** (crash, deploy, cancelled context) leaves a `processing`
  row that never completes. The next tick runs the step again, which counts one more pass. This is the
  same as a double run.
- **Two replicas running the same journey** count two passes, which is true: the step ran twice. That
  scheduler bug is separate.
- **`persistIfActive` failing at line 187.** The `failed` row is then on the next step while the journey
  stays on X, so that pass is counted on neither step. This is rare.
- **The last row is picked by `entered_at`.** Two rows of the same journey with the same timestamp have
  no defined order. The executor writes them in sequence, so it does not happen in practice.

## Verification
1. `go test -race ./internal/repository -run TestAutomationRepository_GetNodeStats`: red, then green.
2. `TEST_DB_HOST=localhost TEST_DB_PORT=15433 INTEGRATION_TESTS=true go test -race -tags integration,licdev ./tests/integration/ -run 'TestAutomationNodeStats$' -v`,
   with a local SMTP sink on 1025: red on the new subtests, then green, three runs in a row.
3. `make test-repo`, `make test-service`, `make test-domain`, `go vet ./...` and
   `go vet -tags integration,licdev ./tests/integration/`.
4. `make openapi-lint openapi-bundle`; in docs, `mint broken-links`.
5. Re-measure on `ws_explain`: today's visits statement against the new pair on `aut-big`. Then stop the
   cluster (`pg_ctlcluster 16 main stop`) and the SMTP sink.

## Plan review (Opus xhigh) and corrections

The approach is sound: on the main paths, passed rows plus not-passed journeys count each pass once.
Blocking points, corrected:

1. **The line-187 limit was misdescribed.** `CurrentNodeID` moves to Y (`:154`) before the persist
   (`:185`). So when the persist fails, `handleError` writes its failed row on Y, and X's `processing`
   row stays (it still counts on X).
   - If `handleError`'s own persist succeeds, the count is exact (X 1, Y 1 while retrying).
   - If it fails too, X runs again and counts twice, like a crash.
   - The limit is reworded that way.
2. **No test covered the LATERAL.** That is a loop with a pass not yet passed. The loop test now has
   W2's receiver answer 500 from its Nth call, and the 2-minute backoff stops the loop by itself. No
   pause is needed. Assertions:
   - W2 `Visits` = completed W2 rows + 1, and `Retrying` = 1;
   - W1 `Visits` = W1's calls (the receiver tells W1 and W2 apart by path).
3. **The retry subtests were racy.** `handleError` writes its failed row (`:318`) before it persists the
   backoff (`:321`).
   - Before rewinding `scheduled_at`, each step polls `retry_count = k` in the DB.
   - Subtest 4's second journey on C starts after subtest 3 asserted `Visits` = 1, then asserts 2.
   - Subtest 5b backdates the webhook's earlier attempts (their processing→failed rows and handleError
     rows), not the `entered` row, which is on the root. Assertions:
     - `Visits` 1 and `FailedAttempts` 1 in the period;
     - 0 and 2 in the earlier window, which its other attempts fall into.

Non-blocking points taken:
- **Cost.** The new statement also reads the `failed` and `exited` journeys that still have a current
  step, which the in-place statement does not, so the plan's "already paid" was wrong. Measured by the
  reviewer: 85 ms warm on `ws_explain`, and 0.56 s warm on a 200k-journey proxy. That is acceptable, and
  re-measured in step 5.
- **Comment.** `console/src/services/api/automation.ts:376` says "Distinct journeys…" and changes too.
- **Known limits.**
  - Mixed clocks: the `entered` row takes the database's `NOW()` and the other rows take the app's
    clock, so a retry inside the skew window shows 0 on its step. The in-place statement has the same
    flaw.
  - A lost `UpdateNodeExecution(failed)` is folded into the crash limit.
  - A loop back to the root makes the trigger card's "entered" count each time round.
- **Docs.** They say how a period dates a pass: by its successful attempt, otherwise by its last
  attempt. So the root is dated when it is processed, not at enrollment.
- **Regression check.** In `tests/integration/demo_automations_test.go`, each node's `Visits` must equal
  `COUNT(DISTINCT contact_automation_id)` on the demo history, which has no loops.
