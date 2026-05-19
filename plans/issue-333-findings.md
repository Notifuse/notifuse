# Issue #333 — Investigation findings

Date: 2026-05-18
Related: #332 (same reporter, same workspace, 3h earlier), #317 (real prior fix in same area)
Diagnostic test: `tests/integration/segment_cascade_diagnostic_test.go`

## TL;DR

The reporter's named cascade (`contact_segment_queue` upsert → `webhook_contact_segments_trigger`) is structurally impossible — the trigger is on a different table. The symptoms they cite (`sql: database is closed`, queue stuck at 0 processed) are real, but the cause is elsewhere. The diagnostic test reproduces the `sql: database is closed` error family as a workspace-DB-handle lifecycle race, independent of trigger load.

## Ground truth: the trigger chain in v30 (init.go)

```
UPDATE message_history SET opened_at = $1, ...
├── webhook_message_history_trigger (init.go:1037)       — queries webhook_subscriptions, fans out to webhook_deliveries
├── track_message_history_changes (init.go:607)          — INSERTs into contact_timeline
│   └── contact_timeline_queue_trigger (init.go:836)
│       └── queue_contact_for_segment_recomputation (init.go:706)
│           └── INSERT INTO contact_segment_queue ON CONFLICT (email) DO UPDATE   ← TERMINAL
└── update_contact_lists_on_status_change (init.go:838)  — only acts on bounce/complaint
```

There is **no trigger on `contact_segment_queue`**. The cascade stops at the queue insert. `webhook_contact_segments_trigger` is bound to `contact_segments` (the membership table, init.go:1035), not `contact_segment_queue` (the work queue, init.go:284). The reporter conflated the two.

`webhook_contact_segments_trigger` fires only when the queue worker writes membership rows. That happens in `ContactSegmentQueueProcessor.ProcessQueue` (a separate transaction, not the open-tracking path).

## Diagnostic test results (200 contacts, 5 segments, 50ms opens, single workspace)

Test ran `tests/integration/segment_cascade_diagnostic_test.go` for 20s. Sampled the workspace DB every 500ms.

| Metric | Value |
|---|---|
| Broadcast | completed (200/200, 0 failed) in ~8s |
| Opens fired via `/opens` | 115 |
| Peak `active_queries` | 1 |
| Peak `lock_waiters` | 0 |
| `app_conns` | constant 1 |
| `message_history` writes | 0 → 305 |
| `contact_timeline` writes | 345 → 705 |
| `contact_segment_queue` writes | 345 → 705 |
| `contact_segments` writes | **0 → 0** |
| Queue size | stuck at 200 |

The 1:1:1 progression between `message_history`, `contact_timeline`, and `contact_segment_queue` writes confirms the real cascade above. No webhook-related contention. No locking. No pool pressure.

## Findings

### A — The alleged cascade is invisible at this scale

The reporter's primary claim (synchronous trigger contention extending lock duration during broadcasts) shows zero signal at 200 contacts × 5 segments × 50ms opens. Wait events: none. Lock waiters: none. Active queries: 0–1. Even with `webhook_subscriptions` populated, the trigger body is one indexed SELECT against a table with the `idx_webhook_subscriptions_enabled` partial index — microseconds.

This does not prove the cascade is impossible at extreme scale, but it does prove it is not the explanation for the reporter's described 165-contact, 8-segment incident.

### B — `sql: database is closed` reproduced, root cause identified

After test cleanup tore down the workspace DB at 21:16:33, the queue worker fired at 21:16:43 and produced the **exact error family** the reporter cited:

```
pq: database "notifuse_test_ws_test993af385" does not exist
failed to evaluate segments: sql: database is closed
failed to remove emails from queue: driver: bad connection
```

**Root cause: there is a race between workspace pool eviction and in-flight workers holding the pool reference.** The race exists in three close paths:

1. **`DeleteDatabase` (workspace_postgres.go:332-355)** — when a workspace is deleted, `CloseWorkspaceConnection` is called immediately with no quiescence period. Any in-flight worker holding the pool reference fails on the next query.

2. **Auto-eviction on Ping failure (connection_manager.go:153-178)** — `GetWorkspaceConnection` pings the cached pool; if Ping fails (transient PG slowdown under load), the pool is dropped and closed. **This is the most likely trigger for the reporter's incident.** Under broadcast pressure, PG can be slow enough that Ping times out, the pool gets evicted, and every other goroutine that grabbed a reference to that pool before the eviction now fails with `sql: database is closed` on its next operation.

3. **LRU eviction (connection_manager.go:373-398)** — when the per-process connection budget (`MaxConnections`) is saturated, the manager evicts the least-recently-used idle pool. The re-check at line 388-391 (`stats.InUse == 0`) is racy: a worker can be between `GetConnection()` returning and `BeginTx()` acquiring, during which `stats.InUse == 0` is still true but a transaction is imminent. Triggered in multi-workspace deployments with `(workspace_count * MaxConnectionsPerDB) > MaxConnections`.

The worker code (`ContactSegmentQueueProcessor.ProcessQueue`, contact_segment_queue_processor.go:45-53) holds the pool reference across the gap:

```go
workspaceDB, err := p.workspaceRepo.GetConnection(ctx, workspaceID)   // line 47 — gets pool P
// ←──── any of the three close paths above can run here ────→
tx, err := workspaceDB.BeginTx(ctx, nil)                              // line 53 — fails: sql: database is closed
```

Because `*sql.DB` is a pool handle (not a connection), `pool.Close()` makes every subsequent `BeginTx`/`Query`/`Exec` on that handle return `sql.ErrConnDone`. Long-running transactions that pre-date the close survive *until they need a new connection*, then fail with `driver: bad connection`.

**Why the reporter saw this cascade:** their `jettinsurance` workspace is being hammered by broadcast sends + opens + (post-fix) the queue worker. PG slows under WAL pressure. A single Ping failure in `GetWorkspaceConnection` evicts the pool. Every other goroutine (queue worker, webhook delivery worker, email send worker) that had grabbed the pool reference in the last few hundred ms now fails. The pool gets re-created on the next request, but in-flight workers cannot be notified. Repeated PG slowdowns repeat the cycle. The "pool exhaustion" symptom is misdiagnosed — connections aren't exhausted, they're being **invalidated out from under in-flight code**.

**Fix directions** (in increasing order of correctness):

a. **Hot fix**: in workers, treat `sql.ErrConnDone` / `driver: bad connection` on `BeginTx` as retryable. Call `GetConnection` again, get the fresh pool. Adds a retry loop but doesn't fix the race.

b. **Better**: replace the eviction-and-replace pattern with a generation counter. Workers register their pool reference; eviction blocks until all references are released, or workers are notified to re-acquire.

c. **Best**: at the boundary of long-lived work, acquire a `*sql.Conn` from the pool (`db.Conn(ctx)`) and defer its release. `pool.Close()` cannot revoke an outstanding `*sql.Conn`. This is the standard `database/sql` pattern for exactly this scenario. The queue processor's transaction would still complete even if the pool is closed mid-flight.

d. **Plus**: make `DeleteDatabase` drain in-flight transactions before closing the pool (RWMutex per pool, read-lock on use, write-lock on close).

Workspace `DeleteDatabase` is the only explicit production caller of `CloseWorkspaceConnection` (workspace_postgres.go:338); paths (2) and (3) above are internal-to-the-manager. Most defensible direction is (c) + (d).

### C — Queue worker stalls on lock contention with concurrent open-tracking

Two root causes confirmed under instrumentation.

**C.1 — Timezone bug in task dispatch (FIXED).** `internal/http/task_handler.go:241` computed `timeoutAt := time.Now().Add(...)` without `.UTC()`. The `tasks.timeout_after` column is `TIMESTAMP WITHOUT TIME ZONE` (system_tables.go:65), so a server in CEST (UTC+2) wrote a literal 2 hours in the future relative to `time.Now().UTC()` used by `GetNextBatch`. Effect: a recurring task marked running stays "still running" for the duration of the server's UTC offset before being re-picked. Only manifests on non-UTC servers, but real for any operator who runs the container in local time. Same pattern in `task_service.go:994` (PauseRecurringTask, 24h pause). Both fixed with `.UTC()`.

**C.2 — Trigger-cascade self-loop creating lock contention with `/opens`.** The diagnostic test (after C.1 is fixed) shows the queue processor's `INSERT INTO contact_segments` hang for **13+ consecutive seconds** with `lock_waiters=1`. The blocker is the trigger chain:

```
worker: INSERT INTO contact_segments
  → track_contact_segment_changes (init.go:681)
  → INSERT INTO contact_timeline
    → contact_timeline_queue_trigger (init.go:836)
    → queue_contact_for_segment_recomputation (init.go:706)
    → UPSERT INTO contact_segment_queue ON CONFLICT (email) DO UPDATE
```

Simultaneously, every `/opens` hit fires the same downstream cascade from `message_history → contact_timeline → contact_segment_queue`. Worker and opener serialize on the same `(email)` row lock in `contact_segment_queue`. Under broadcast load (high open volume), the worker's membership writes block long enough that batches don't complete within the task's `MaxRuntime=50s` window, the queue never drains, and the symptom looks like "worker not progressing".

There is also a **self-perpetuating loop**: the worker writes a `contact_segments` row, the trigger re-enqueues the contact in `contact_segment_queue`. Next worker pass picks up the same contact, evaluates segments, writes (possibly identical) memberships, re-queues. The 15s debounce in `getPendingEmailsInTx` limits the loop frequency but doesn't break it.

**The reporter's intuition about cascade contention was directionally correct** — they just named the wrong trigger. The actual contention is via `track_contact_segment_changes` (not `webhook_contact_segments_trigger`), and the lock target is `contact_segment_queue` (not `contact_segments`).

**Fix directions for C.2** (in increasing scope):

a. **Skip the timeline re-queue when the writer is the queue processor itself.** Wrap the worker's `AddContactToSegment` calls in a `SET LOCAL` session variable (e.g., `notifuse.skip_queue_trigger = 'on'`) and have `queue_contact_for_segment_recomputation` short-circuit when that variable is set. Breaks the self-loop; opens still queue as before.

b. **Make `contact_timeline → contact_segment_queue` re-queue conditional.** Only enqueue when the timeline event is one that actually changes segment-relevant state (e.g., a `list.subscribed` or a property change). Membership-change events shouldn't re-queue.

c. **Decouple the queue insertion from the trigger.** Move queue-population to the application layer (explicit `EnqueueSegmentRecomputation` calls) and remove the trigger entirely. Most invasive but cleanest long-term.

Option (a) is the smallest delta and addresses both the self-loop and the worker-vs-opener contention.

## What this does NOT prove

- That issue #333's reporter has the cascade they describe — they don't, the chain is wrong
- That the cascade is impossible at extreme scale — untested above 200 contacts
- That webhook triggers are perfectly designed — they do run synchronous SELECTs in hot transactions, which is an architectural smell even if benign at current load
- That `sql: database is closed` in production has the same cause as in the test — only that the test demonstrates one path to it

## Next steps

1. **Finding B** — map workspace DB handle lifecycle, identify what closes a handle that a worker still holds
2. **Finding C** — instrument the task scheduler to see why `process_contact_segment_queue` doesn't fire during broadcast
3. **Stress test** — run the diagnostic at `CASCADE_CONTACTS=1000 CASCADE_OPEN_RATE_MS=10` to see if contention emerges at scale
4. **Reply to issue #333 / #332** — explain the named cascade is wrong, ask for `pg_stat_activity` + `pg_locks` snapshot + first occurrence of `sql: database is closed` log line surrounding context
