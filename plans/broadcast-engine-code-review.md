# Broadcast Engine Code Review

**Date:** 2025-12-22 (Updated: 2025-12-23)
**Status:** Investigation Complete - 8 of 9 issues validated
**Scope:** Full flow from scheduling to email delivery

---

## Architecture Overview

```
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│  API Handler    │────▶│ BroadcastService│────▶│   TaskService   │
│ broadcast_handler.go  │ broadcast_service.go  │ task_service.go │
│ (schedule call) │     │ (create task)   │     │ (store task)    │
└─────────────────┘     └─────────────────┘     └────────┬────────┘
                                                         │
                        ┌────────────────────────────────┘
                        ▼
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│  Orchestrator   │────▶│  MessageSender  │────▶│  EmailQueue     │
│ orchestrator.go │     │ queue_message_  │     │ email_queue_    │
│ (batch process) │     │ sender.go       │     │ postgres.go     │
└─────────────────┘     └─────────────────┘     └────────┬────────┘
                                                         │
                        ┌────────────────────────────────┘
                        ▼
┌─────────────────┐     ┌─────────────────┐
│  Queue Worker   │────▶│ MessageHistory  │
│ worker.go       │     │ message_history_│
│ (send via ESP)  │     │ postgre.go      │
└─────────────────┘     └─────────────────┘
```

### Key Files

| Component | File | Lines |
|-----------|------|-------|
| API Entry | `internal/http/broadcast_handler.go` | 228-260 |
| Service Logic | `internal/service/broadcast_service.go` | 246-383 |
| Task Creation | `internal/service/task_service.go` | 695-877 |
| Orchestration | `internal/service/broadcast/orchestrator.go` | 391+ |
| Queue Insertion | `internal/service/broadcast/queue_message_sender.go` | all |
| Queue Storage | `internal/repository/email_queue_postgres.go` | all |
| Worker | `internal/service/queue/worker.go` | 137-344 |
| Message History | `internal/repository/message_history_postgre.go` | all |

---

## Critical Issues

### ~~1. Race Condition in Task Execution~~ ✅ MITIGATED

**Location:** `internal/service/task_service.go:852-864`

**Code:**
```go
if sendNow && status == string(domain.BroadcastStatusProcessing) && s.autoExecuteImmediate {
    go func() {
        time.Sleep(100 * time.Millisecond)  // ← Appears to be a race window
        if execErr := s.ExecutePendingTasks(context.Background(), 1); execErr != nil {
            // ...
        }
    }()
}
```

**Original Concern:**
Between transaction commit and goroutine execution (100ms window), the cron scheduler could pick up the same task, leading to duplicate email sends.

**Investigation Result (2025-12-23):**
This issue is **already mitigated** by existing code. The `GetNextBatch` function at `internal/repository/task_postgres.go:552` uses:
```go
.Suffix("FOR UPDATE SKIP LOCKED")
```

This means if both the goroutine and cron scheduler call `ExecutePendingTasks` simultaneously, only one will acquire each task. The other will skip it due to `SKIP LOCKED`.

**Status:** ✅ Fixed (2025-12-23). Removed 100ms sleep by moving goroutine outside the transaction callback. The goroutine now runs after the transaction commits, eliminating the need for arbitrary timing delays.

---

### 2. Unbounded DB Queries in Orchestrator Loop

**Location:** `internal/service/broadcast/orchestrator.go:811-813`

**Code:**
```go
for {
    // Called EVERY batch iteration - no caching
    if refreshed, refreshErr := o.broadcastRepo.GetBroadcast(ctx, task.WorkspaceID, broadcastState.BroadcastID); refreshErr == nil && refreshed != nil {
        broadcast = refreshed
    }
    // ... process batch
}
```

**Problem:**
For a broadcast with 1M recipients at batch size 1000:
- 1000 iterations = 1000 DB queries just for status refresh
- No caching, no rate limiting on refresh
- Each query hits broadcast table

**Impact:** HIGH - Unnecessary DB load, slower processing

**Recommended Fix:**
```go
// Cache broadcast with TTL
var lastRefresh time.Time
var cachedBroadcast *domain.Broadcast

for {
    if time.Since(lastRefresh) > 30*time.Second {
        if refreshed, _ := o.broadcastRepo.GetBroadcast(...); refreshed != nil {
            cachedBroadcast = refreshed
            lastRefresh = time.Now()
        }
    }
    // Use cachedBroadcast
}
```

---

### 3. Missing Index for Message History Broadcast Queries

**Location:** `internal/migrations/v20.go`

**Current State:**
- Has `idx_message_history_automation_id`
- **Missing `idx_message_history_broadcast_id`**

**Problem:**
Queries like `SELECT * FROM message_history WHERE broadcast_id = $1` perform **full table scans**.

**Impact:** HIGH - Slow dashboard stats, slow broadcast analytics

**Recommended Fix:**
Add to migration v22:
```sql
CREATE INDEX idx_message_history_broadcast_id
ON message_history(broadcast_id)
WHERE broadcast_id IS NOT NULL;

CREATE INDEX idx_message_history_broadcast_template
ON message_history(broadcast_id, template_id)
WHERE broadcast_id IS NOT NULL;
```

---

## High Priority Issues

### 4. Worker Polls ALL Workspaces

**Location:** `internal/service/queue/worker.go:154-160`

**Code:**
```go
func (w *EmailQueueWorker) processAllWorkspaces() {
    workspaces, err := w.workspaceRepo.List(w.ctx)  // Gets ALL workspaces
    // ...
    for _, workspace := range workspaces {
        // Process each one, even if empty queue
    }
}
```

**Problem:**
With 1000 workspaces where 99% are idle:
- 1000 workspace iterations per poll cycle
- 1000 queue peek queries (most return empty)
- Wasted CPU and DB I/O

**Impact:** MEDIUM - Performance degradation at scale

**Recommended Fix:**
```go
// Track workspaces with pending emails
// Only poll workspaces known to have queue entries
// Use event-driven notification when entries added

func (w *EmailQueueWorker) processActiveWorkspaces() {
    activeWorkspaces, _ := w.queueRepo.GetWorkspacesWithPendingEmails(w.ctx)
    for _, wsID := range activeWorkspaces {
        w.processWorkspace(wsID)
    }
}
```

---

### 5. N+1 Template Loading for A/B Tests

**Location:** `internal/service/broadcast/orchestrator.go:100-130`

**Code:**
```go
templates := make(map[string]*domain.Template)
for _, templateID := range templateIDs {
    template, err := o.templateRepo.GetTemplateByID(ctx, workspaceID, templateID, 0)
    if err != nil {
        continue
    }
    templates[templateID] = template
}
```

**Problem:**
For A/B test with 5 variations: 5 separate database queries.

**Impact:** MEDIUM - Slower broadcast startup for A/B tests

**Recommended Fix:**
```go
// Add to TemplateRepository interface:
GetTemplatesByIDs(ctx context.Context, workspaceID string, templateIDs []string) (map[string]*Template, error)

// Implementation:
SELECT * FROM templates WHERE id = ANY($1) AND workspace_id = $2
```

---

### 6. Scalability Limit: 100 Workspaces

**Location:** `internal/service/task_service.go:232`

**Code:**
```go
// TODO/problem: if new number of workspaces is above 100, this will not work
// we need to have a way to scale this
if maxTasks <= 0 {
    maxTasks = 100 // Default value
}
```

**Problem:**
Hard-coded 100 workspace limit in cron executor. Tasks from workspace 101+ won't execute in a single cron cycle.

**Impact:** HIGH - Tasks silently dropped at scale

**Recommended Fix:**
- Implement cursor-based pagination for workspaces
- Or process tasks regardless of workspace count
- Or distribute across multiple worker instances

---

## Medium Priority Issues

### 7. Non-Atomic Broadcast State Updates

**Location:** `internal/service/broadcast/orchestrator.go:598-604`

**Code:**
```go
// First: update broadcast status
if updateErr := o.broadcastRepo.UpdateBroadcast(context.Background(), broadcast); updateErr != nil {
    // Error path...
}

// SEPARATE OPERATION: set enqueued count
if setErr := o.broadcastRepo.SetEnqueuedCount(context.Background(), task.WorkspaceID, broadcastState.BroadcastID, 0); setErr != nil {
    // Only logs error, doesn't rollback
}
```

**Problem:**
Two separate database calls. If `SetEnqueuedCount` fails:
- Broadcast marked as processed
- But enqueued_count never set
- Dashboard shows incorrect data

**Impact:** MEDIUM - Data inconsistency

**Recommended Fix:**
```go
// Combine in single transaction
err := o.broadcastRepo.UpdateBroadcastWithEnqueuedCount(ctx, broadcast, enqueuedCount)
```

---

### 8. Rate Limiter Not Workspace-Isolated

**Location:** `internal/service/queue/worker.go:238-254`

**Code:**
```go
ratePerMinute := entry.Payload.RateLimitPerMinute
if ratePerMinute <= 0 {
    ratePerMinute = integration.EmailProvider.RateLimitPerMinute
}
// Wait for rate limiter
if err := w.rateLimiter.Wait(w.ctx, entry.IntegrationID, ratePerMinute); err != nil {
```

**Problem:**
Rate limiter key is only `integrationID`. If two workspaces share the same ESP integration, one workspace's heavy traffic slows down another workspace.

**Impact:** MEDIUM - Cross-workspace interference

**Recommended Fix:**
```go
// Use hierarchical rate limiting
key := fmt.Sprintf("%s:%s", workspace.ID, entry.IntegrationID)
w.rateLimiter.Wait(w.ctx, key, ratePerMinute)
```

---

### 9. Message History Upsert in Hot Path

**Location:** `internal/service/queue/worker.go:349-399`

**Code:**
```go
func (w *EmailQueueWorker) processEntry(workspace *domain.Workspace, entry *domain.EmailQueueEntry) {
    // ... send email ...

    // Called for EVERY email - in critical path
    w.upsertMessageHistory(w.ctx, workspace.ID, workspace.Settings.SecretKey, entry, sendErr)
}
```

**Problem:**
Each email send triggers:
- JSON marshaling
- Encryption of message data
- Individual database transaction
- Upsert with ON CONFLICT

At 1000 emails/second, this adds significant overhead.

**Impact:** MEDIUM - Latency per email

**Recommended Fix:**
```go
// Option A: Batch upserts (50-100 per transaction)
// Option B: Async queue for message history writes
// Option C: Accept eventual consistency, write in background goroutine
```

---

## Email Queue Indexes (Well Designed)

The v21 migration has proper indexes for the email_queue table:

| Index | Purpose | Query Pattern |
|-------|---------|---------------|
| `idx_email_queue_pending` | Fetch pending entries | `WHERE status = 'pending' AND next_retry_at <= NOW()` |
| `idx_email_queue_next_retry` | Retry scheduling | `WHERE status = 'pending' ORDER BY next_retry_at` |
| `idx_email_queue_source` | Broadcast filtering | `WHERE source_type = $1 AND source_id = $2` |
| `idx_email_queue_integration` | Rate limit queries | `WHERE integration_id = $1` |
| `idx_email_queue_dead_letter_source` | Dead letter queries | `WHERE is_dead_letter = true` |

---

## Missing Links Summary

| Gap | Location | Fix Effort | Impact | Status |
|-----|----------|------------|--------|--------|
| No message_history broadcast index | v20 migration | Low | High | Open |
| No batch template loading | orchestrator.go | Medium | Medium | Open |
| No workspace filtering in worker | worker.go | Medium | Medium | Open |
| ~~No task execution lock~~ | task_postgres.go | - | - | ✅ Already implemented via `SKIP LOCKED` |
| Non-atomic broadcast updates | orchestrator.go | Low | Medium | Open |
| Unbounded broadcast refresh | orchestrator.go | Low | High | Open |

---

## Performance Analysis

### Current Bottlenecks

| Operation | Current Complexity | Optimal | Issue |
|-----------|-------------------|---------|-------|
| Orchestrator broadcast refresh | O(batches) queries | O(1) cached | Unbounded loop |
| Template loading (5 variations) | 5 queries | 1 query | N+1 pattern |
| Worker workspace scan | O(all workspaces) | O(active) | No filtering |
| Message history per email | 1 tx each | Batched | Hot path overhead |
| Stats query by broadcast | Full scan | Index scan | Missing index |

### Estimated Impact at Scale

**Scenario:** 100 workspaces, 1M recipient broadcast, 5 A/B variations

| Issue | Extra Load |
|-------|------------|
| Broadcast refresh (1000 batches) | +1000 queries |
| Template N+1 | +4 queries |
| Worker polling idle workspaces | +99 queries/cycle |
| Message history individual upserts | +1M transactions |

---

## Recommended Priority Order

### Phase 1: Critical Fixes (Do First)

1. ~~**Add task execution lock**~~ ✅ Already implemented via `FOR UPDATE SKIP LOCKED` in `GetNextBatch`

2. **Add message_history broadcast_id index** - Needed for stats/dashboard
   - Add migration v22
   - Estimated effort: 30 minutes

### Phase 2: Performance Improvements

3. **Cache broadcast in orchestrator** - Reduces DB load significantly
   - Add 30-60s TTL cache
   - Estimated effort: 1-2 hours

4. **Implement batch template loading** - Faster A/B test startup
   - Add `GetTemplatesByIDs` to repository
   - Estimated effort: 2-3 hours

5. **Filter worker by active workspaces** - Reduces polling overhead
   - Track workspaces with pending queue entries
   - Estimated effort: 4-6 hours

### Phase 3: Scalability

6. **Remove 100 workspace limit** - For large installations
   - Implement cursor-based pagination
   - Estimated effort: 3-4 hours

7. **Batch message history upserts** - Reduce per-email overhead
   - Collect 50-100 entries, upsert in batch
   - Estimated effort: 4-6 hours

---

## Testing Gaps Identified

During this review, the following test gaps were noted:

1. **Integration test for message_history after broadcast** - Added in this session
2. **Integration test for enqueued_count on broadcast API** - Added in this session
3. **No load test for 100+ workspaces** - Task scalability untested
4. ~~**No concurrent task execution test**~~ - Not needed; `FOR UPDATE SKIP LOCKED` prevents race conditions

---

## Changelog

- **2025-12-22**: Initial comprehensive review completed
- **2025-12-22**: Added integration test for message_history verification
- **2025-12-22**: Added integration test for broadcast enqueued_count verification
- **2025-12-23**: Investigation completed - Issue #1 (race condition) confirmed mitigated by existing `FOR UPDATE SKIP LOCKED` in `task_postgres.go:552`. 8 of 9 issues remain valid.
- **2025-12-23**: Refactored `handleBroadcastScheduled` to remove 100ms sleep - goroutine now runs after transaction commits.
