# Automation Feature - Backend Performance Code Review

## Executive Summary

The automation feature is well-architected with clean separation of concerns, but has several performance bottlenecks that will limit scalability. Current throughput ceiling: **~300 contacts/minute**. The design works well for small-to-medium workloads but needs optimization for high-volume scenarios.

---

## Architecture Overview

| Component | File | Purpose |
|-----------|------|---------|
| Scheduler | `internal/service/automation_scheduler.go` | Polls every 10s, batch size 50 |
| Executor | `internal/service/automation_executor.go` | Processes contacts through nodes |
| Node Executors | `internal/service/automation_node_executor.go` | Type-specific node logic |
| Repository | `internal/repository/automation_postgres.go` | Data access with FOR UPDATE SKIP LOCKED |
| Trigger Generator | `internal/service/automation_trigger_generator.go` | Dynamic PG trigger SQL |

---

## Critical Performance Issues

### 1. Sequential Contact Processing (HIGH IMPACT)

**Location:** `automation_executor.go:191-201`

```go
for _, ca := range contacts {
    if err := e.Execute(ctx, ca.WorkspaceID, &ca.ContactAutomation); err != nil {
        // Continue with other contacts
        continue
    }
    processed++
}
```

**Problem:** Contacts are processed one-by-one in a loop. A batch of 50 contacts with email nodes takes 50 sequential API calls.

**Impact:**
- If each email takes 200ms, 50 contacts = 10 seconds
- Blocks entire batch while slow nodes execute
- Webhook nodes with 30s timeout can block for 25+ minutes per batch

**Recommendation:** Use a worker pool with goroutines:
```go
var wg sync.WaitGroup
sem := make(chan struct{}, 10) // Limit concurrent workers
for _, ca := range contacts {
    wg.Add(1)
    go func(c ContactAutomation) {
        defer wg.Done()
        sem <- struct{}{}
        defer func() { <-sem }()
        e.Execute(ctx, c.WorkspaceID, &c)
    }(ca)
}
wg.Wait()
```

---

### 2. N+1 Query Pattern in Execute() (HIGH IMPACT)

**Location:** `automation_executor.go:67-127`

Each `Execute()` call makes 4+ separate database queries:
1. `GetByID()` - Fetch automation (line 71)
2. `GetContactByEmail()` - Fetch contact data (line 103)
3. `CreateNodeExecution()` - Create audit entry (line 110)
4. `GetNodeExecutions()` - Build context from history (line 113)
5. Plus more for update/stats

**Impact:** For 50 contacts: 50 × 4 = **200+ queries per batch**

**Recommendation:** Bulk prefetch before processing:
```go
// Before processing loop
automations := prefetchAutomations(contacts)
contactsData := prefetchContacts(contacts)
// Then lookup from maps instead of individual queries
```

---

### 3. Synchronous Email Sends (HIGH IMPACT)

**Location:** `automation_node_executor.go:217`

```go
err = e.emailService.SendEmailForTemplate(ctx, request)
```

**Problem:** Email node waits for ESP API response before returning. Slow ESP = blocked automation.

**Impact:**
- SES latency: 100-300ms per email
- SMTP: 500ms-2s per email
- Rate limits: ESP may throttle, causing delays

**Recommendation:** Decouple email delivery:
1. Queue email to internal message queue (Redis, PG LISTEN/NOTIFY)
2. Return immediately with `message_id`
3. Background worker handles actual sending
4. Update message_history async

---

### 4. Webhook 30-Second Timeout (MEDIUM IMPACT)

**Location:** `automation_node_executor.go:768`

```go
httpClient: &http.Client{Timeout: 30 * time.Second}
```

**Problem:** A single slow webhook blocks the processing thread for up to 30 seconds.

**Impact:** One slow webhook in a batch of 50 can delay the entire batch by 30 seconds.

**Recommendation:**
- Reduce default timeout to 10s
- Make timeout configurable per webhook
- Consider async webhook delivery with callback

---

### 5. Context Reconstruction from Node Executions (MEDIUM IMPACT)

**Location:** `automation_executor.go:311-324`

```go
func (e *AutomationExecutor) buildContextFromNodeExecutions(...) {
    entries, err := e.automationRepo.GetNodeExecutions(ctx, workspaceID, contactAutomationID)
    // ... iterate all entries
}
```

**Problem:** Fetches ALL node executions for a contact to reconstruct context. For long automations (20+ nodes), this grows unbounded.

**Impact:**
- Query returns more rows as contact progresses
- Memory allocation grows with automation length

**Recommendation:**
- Store essential context in `contact_automations.context` column (already exists)
- Only fetch node outputs when needed, not entire history
- Add LIMIT to query or only fetch last N executions

---

### 6. Round-Robin Workspace Query (MEDIUM IMPACT)

**Location:** `automation_postgres.go:841-879`

```go
for _, ws := range workspaces {
    contacts, err := r.GetScheduledContactAutomations(ctx, ws.ID, beforeTime, perWorkspace)
    // ...
}
```

**Problem:** Sequential queries to each workspace database. With 100 workspaces, this means 100 sequential queries per poll cycle.

**Impact:**
- 100 workspaces × 50ms/query = 5 seconds just to fetch scheduled contacts
- Delays processing start

**Recommendation:**
- Query workspaces in parallel with goroutines
- Add workspace-level caching of database connections
- Consider single query across all workspaces if feasible

---

### 7. Database Trigger Proliferation (MEDIUM IMPACT)

**Location:** `automation_trigger_generator.go`, `v20.go`

Each live automation creates a PostgreSQL trigger on `contact_timeline` table.

**Problem:**
- 100 live automations = 100 triggers firing on every timeline INSERT
- Each trigger invokes `automation_enroll_contact()` function

**Impact:**
- Timeline INSERT latency increases with automation count
- CPU overhead for trigger evaluation
- Potential lock contention

**Recommendation:**
- Consider a single trigger that dispatches to a function checking active automations
- Cache automation trigger conditions in memory
- Use NOTIFY/LISTEN instead of inline execution

---

### 8. No Rate Limiting (MEDIUM IMPACT)

**Location:** N/A - missing feature

**Problem:** No throttling on:
- Enrollment rate (mass import could enroll thousands instantly)
- Email sending rate (ESP limits not respected)
- Webhook calls (could DDoS external endpoints)

**Impact:**
- ESP rate limit errors causing retries
- Webhook endpoints overwhelmed
- Unpredictable system load

**Recommendation:** Add rate limiters:
```go
type RateLimiter struct {
    emailsPerSecond   int // Per workspace or per provider
    webhooksPerSecond int
    enrollmentsPerSec int
}
```

---

### 9. Stats Update Contention (LOW IMPACT)

**Location:** `automation_postgres.go:1102-1141`

```go
query := fmt.Sprintf(`
    UPDATE automations
    SET stats = COALESCE(stats, '{}'::jsonb) ||
        jsonb_build_object('%s', COALESCE((stats->>'%s')::int, 0) + 1),
    ...
`)
```

**Problem:** Each contact completion/exit updates the same automation row.

**Impact:**
- Row-level lock contention with high throughput
- Minor performance impact

**Recommendation:**
- Batch stats updates (every 10 contacts or every 5 seconds)
- Use `INSERT ... ON CONFLICT DO UPDATE` pattern
- Consider separate stats table with periodic aggregation

---

### 10. Missing Indexes for Common Queries (LOW IMPACT)

**Observation:** While indexes exist, some common access patterns may benefit from additional indexes:

```sql
-- For GetNodeExecutions ordered by entered_at
-- Already exists: idx_node_executions_contact_automation

-- Consider adding for retry queries:
CREATE INDEX idx_contact_automations_retry
ON contact_automations(last_retry_at)
WHERE status = 'active' AND retry_count > 0;
```

---

## Positive Design Patterns

The codebase has several well-implemented patterns:

1. **FOR UPDATE SKIP LOCKED** (`automation_postgres.go:793`) - Excellent for preventing duplicate processing
2. **Embedded nodes as JSONB** - Reduces JOINs, single fetch for automation + nodes
3. **Exponential backoff** (`automation_executor.go:228-231`) - Proper retry handling
4. **Partial indexes** (`v20.go`) - Efficient index usage for active records
5. **Soft deletes** - Preserves audit trail
6. **Strategy pattern for node executors** - Clean, extensible design
7. **Workspace isolation** - Good multi-tenant separation

---

## Throughput Analysis

### Current Limits

| Metric | Value | Bottleneck |
|--------|-------|------------|
| Poll interval | 10 seconds | Configuration |
| Batch size | 50 contacts | Configuration |
| Processing | Sequential | Code design |
| Max throughput | 300/min | Poll × Batch / 60 |

### With Recommended Changes

| Change | New Throughput |
|--------|----------------|
| Parallel processing (10 workers) | ~3,000/min |
| Reduce poll to 2 seconds | ~1,500/min |
| Increase batch to 200 | ~1,200/min |
| All combined | ~15,000/min |

---

## Database Query Patterns

### Hot Queries (per batch)

1. `GetScheduledContactAutomationsGlobal` - 1 query per workspace
2. `GetByID` (automation) - 1 per contact (N+1)
3. `GetContactByEmail` - 1 per contact (N+1)
4. `GetNodeExecutions` - 1 per contact (N+1)
5. `CreateNodeExecution` - 2 per contact (start + end)
6. `UpdateContactAutomation` - 1 per contact
7. `IncrementAutomationStat` - 1 per contact

**Total per 50-contact batch: ~350-400 queries**

---

## Recommendations Priority

### Immediate (High Impact, Low Effort)
1. Add parallel processing with worker pool
2. Reduce webhook timeout to 10 seconds
3. Make poll interval configurable (default 2-5 seconds)

### Short-term (High Impact, Medium Effort)
4. Bulk prefetch automations and contacts
5. Queue email sends instead of synchronous
6. Limit context reconstruction query

### Medium-term (Medium Impact, Higher Effort)
7. Add rate limiting for emails/webhooks
8. Parallelize workspace queries
9. Batch stats updates

### Long-term (Architectural)
10. Consolidate database triggers into single dispatcher
11. Consider message queue for async processing
12. Add horizontal scaling support with distributed locking

---

## Files for Modification

If implementing fixes:

| Priority | File | Changes |
|----------|------|---------|
| P0 | `internal/service/automation_executor.go` | Parallel processing, bulk prefetch |
| P0 | `internal/service/automation_scheduler.go` | Configurable interval |
| P1 | `internal/service/automation_node_executor.go` | Async email, configurable webhook timeout |
| P1 | `internal/repository/automation_postgres.go` | Bulk queries, parallel workspace fetch |
| P2 | `internal/app/app.go` | Rate limiter injection |

---

## Conclusion

The automation feature is **production-ready for small-to-medium workloads** (up to ~300 contacts/minute). For larger scale:

- **Quick wins**: Parallel processing + reduced poll interval = 10x improvement
- **Full optimization**: Could reach 15,000+ contacts/minute with all recommendations

The codebase is clean and well-structured, making these optimizations straightforward to implement without major architectural changes.
