# Email Marketing Queue System - Implementation Plan

## Overview

PostgreSQL-based marketing email queue that unifies broadcast and automation email sending with provider-level rate limiting. Transactional emails remain as direct synchronous calls (no queue).

## Architecture

```
+------------------+     +------------------+     +------------------+
|   Broadcasts     |     |   Automations    |     | Transactional    |
|   Orchestrator   |     |  EmailNodeExec   |     | (Direct Send)    |
+--------+---------+     +--------+---------+     +------------------+
         |                        |                       |
         v                        v                       |
    +---------------------------------+                   |
    |     Marketing Queue (PG)       |                   |
    |   per-workspace email_queue    |                   |
    +---------------------------------+                   |
                  |                                       |
                  v                                       v
    +---------------------------------+     +------------------+
    |      Queue Worker Pool         |     |   EmailService   |
    |   (provider rate limiters)     |---->|   (ESP dispatch) |
    +---------------------------------+     +------------------+
```

## Key Decisions

| Aspect | Decision |
|--------|----------|
| **Transactional** | Direct send, no queue (lowest latency) |
| **Broadcasts** | Hybrid: Keep orchestrator (A/B, circuit breaker), enqueue to marketing queue |
| **Automations** | Enqueue to marketing queue (non-blocking) |
| **Rate Limiting** | Per-integration (uses `EmailProvider.RateLimitPerMinute` from workspace settings) |

---

## Implementation Phases

### Phase 1: Domain Layer

**Create:** `internal/domain/email_queue.go`

```go
// EmailQueueEntry - queued email with status, priority, payload
type EmailQueueEntry struct {
    ID, WorkspaceID, Status, SourceType, SourceID string
    Priority int  // 5 = marketing (broadcasts & automations)
    ProviderKind EmailProviderKind
    ContactEmail, MessageID, TemplateID string
    Payload EmailQueuePayload  // JSONB with all send data
    Attempts, MaxAttempts int
    LastError *string
    NextRetryAt *time.Time
    CreatedAt, UpdatedAt time.Time
}

// EmailQueueRepository interface
type EmailQueueRepository interface {
    Enqueue(ctx, workspaceID string, entries []*EmailQueueEntry) error
    FetchPending(ctx, workspaceID string, providerKind, limit int) ([]*EmailQueueEntry, error)
    MarkAsSent(ctx, workspaceID string, ids []string) error
    MarkAsFailed(ctx, workspaceID, id, errorMsg string, nextRetryAt *time.Time) error
    MoveToDeadLetter(ctx, workspaceID string, entry, finalError string) error
}
```

---

### Phase 2: Database Migration

**Create:** `internal/migrations/v21.go`

```sql
-- email_queue table (per workspace)
CREATE TABLE email_queue (
    id VARCHAR(36) PRIMARY KEY,
    status VARCHAR(20) DEFAULT 'pending',  -- pending, processing, sent, failed
    priority INTEGER DEFAULT 5,
    source_type VARCHAR(20) NOT NULL,      -- broadcast, automation
    source_id VARCHAR(36) NOT NULL,
    integration_id VARCHAR(36) NOT NULL,
    provider_kind VARCHAR(20) NOT NULL,
    contact_email VARCHAR(255) NOT NULL,
    message_id VARCHAR(100) NOT NULL,
    template_id VARCHAR(36) NOT NULL,
    payload JSONB NOT NULL,
    attempts INTEGER DEFAULT 0,
    max_attempts INTEGER DEFAULT 3,
    last_error TEXT,
    next_retry_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    processed_at TIMESTAMPTZ
);

-- Index for fetching pending by provider
CREATE INDEX idx_email_queue_pending_provider
ON email_queue(provider_kind, priority, created_at)
WHERE status = 'pending' AND (next_retry_at IS NULL OR next_retry_at <= NOW());

-- Dead letter table
CREATE TABLE email_queue_dead_letter (
    id VARCHAR(36) PRIMARY KEY,
    original_entry_id VARCHAR(36) NOT NULL,
    source_type VARCHAR(20) NOT NULL,
    source_id VARCHAR(36) NOT NULL,
    contact_email VARCHAR(255) NOT NULL,
    payload JSONB NOT NULL,
    final_error TEXT NOT NULL,
    attempts INTEGER NOT NULL,
    failed_at TIMESTAMPTZ DEFAULT NOW()
);
```

**Update:** `config/config.go` - Change VERSION to "21.0"

---

### Phase 3: Repository Implementation

**Create:** `internal/repository/email_queue_postgres.go`

Key features:
- `FetchPending`: Use `FOR UPDATE SKIP LOCKED` for concurrent workers
- `Enqueue`: Batch insert for efficiency
- `MarkAsFailed`: Calculate exponential backoff (1min, 2min, 4min)

---

### Phase 4: Rate Limiter Service

**Create:** `internal/service/queue/rate_limiter.go`

Rate limiting is per-integration, using the `RateLimitPerMinute` configured in workspace integration settings.

```go
// IntegrationRateLimiter manages rate limits per integration
type IntegrationRateLimiter struct {
    limiters sync.Map  // map[integrationID]*rate.Limiter
    mu       sync.Mutex
}

func NewIntegrationRateLimiter() *IntegrationRateLimiter

// GetOrCreateLimiter returns a limiter for the integration, creating one if needed
func (irl *IntegrationRateLimiter) GetOrCreateLimiter(integrationID string, ratePerMinute int) *rate.Limiter {
    // Convert rate per minute to rate per second
    ratePerSecond := float64(ratePerMinute) / 60.0

    if existing, ok := irl.limiters.Load(integrationID); ok {
        limiter := existing.(*rate.Limiter)
        limiter.SetLimit(rate.Limit(ratePerSecond))  // Update if rate changed
        return limiter
    }

    // Create new limiter with burst of 1 (strict rate limiting)
    limiter := rate.NewLimiter(rate.Limit(ratePerSecond), 1)
    irl.limiters.Store(integrationID, limiter)
    return limiter
}

// Wait blocks until the integration's rate limiter allows an event
func (irl *IntegrationRateLimiter) Wait(ctx, integrationID string, ratePerMinute int) error
```

The worker fetches the integration's `EmailProvider.RateLimitPerMinute` from the queue entry payload and uses that for rate limiting.

---

### Phase 5: Queue Worker Service

**Create:** `internal/service/queue/worker.go`

```go
type EmailQueueWorker struct {
    queueRepo     EmailQueueRepository
    emailService  EmailServiceInterface
    rateLimiter   *IntegrationRateLimiter  // Per-integration rate limiting
    config        *EmailQueueWorkerConfig  // WorkerCount: 10, PollInterval: 1s
}

func (w *EmailQueueWorker) Start(ctx) error    // Start worker goroutines
func (w *EmailQueueWorker) Stop()              // Graceful shutdown
func (w *EmailQueueWorker) processEntry(ctx, entry) error {
    // 1. Wait for integration's rate limiter (using RateLimitPerMinute from payload)
    ratePerMin := entry.Payload.ProviderConfig["rate_limit_per_minute"].(int)
    w.rateLimiter.Wait(ctx, entry.IntegrationID, ratePerMin)

    // 2. Build SendEmailProviderRequest from payload
    // 3. Call emailService.SendEmail()
    // 4. Mark as sent or handle failure with retry
}
```

---

### Phase 6: Broadcast Integration (Hybrid)

**Modify:** `internal/service/broadcast/orchestrator.go`

- Add `EmailQueueRepository` dependency
- Replace `SendBatch()` calls with `enqueueBatch()`:

```go
func (o *BroadcastOrchestrator) enqueueBatch(ctx, recipients, templates) (enqueued int, err error) {
    entries := make([]*EmailQueueEntry, 0, len(recipients))
    for _, recipient := range recipients {
        // Build entry with compiled email payload
        entry := &EmailQueueEntry{
            SourceType: "broadcast",
            SourceID:   broadcastID,
            Priority:   5,
            // ... rest of fields
        }
        entries = append(entries, entry)
    }
    return len(entries), o.queueRepo.Enqueue(ctx, workspaceID, entries)
}
```

**Keep existing:** A/B testing, circuit breaker, batch fetching, progress tracking

**Modify:** `internal/service/broadcast/message_sender.go`
- Remove `enforceRateLimit()` method (lines 151-197) - queue workers handle rate limiting

---

### Phase 7: Automation Integration

**Modify:** `internal/service/automation_node_executor.go`

Change `EmailNodeExecutor.Execute()` (lines 121-239):

```go
// Replace synchronous email send with queue enqueue
entry := &EmailQueueEntry{
    SourceType:   "automation",
    SourceID:     params.Automation.ID,
    Priority:     5,
    ProviderKind: emailProvider.Kind,
    ContactEmail: params.ContactData.Email,
    Payload:      emailPayload,
}

if err := e.queueRepo.Enqueue(ctx, workspaceID, []*EmailQueueEntry{entry}); err != nil {
    return nil, fmt.Errorf("failed to enqueue email: %w", err)
}

// Return immediately - non-blocking
return &NodeExecutionResult{
    Output: buildNodeOutput(NodeTypeEmail, map[string]interface{}{
        "message_id": messageID,
        "queued":     true,
    }),
}, nil
```

---

### Phase 8: Service Initialization

**Modify:** `internal/app/app.go`

```go
// Add to App struct
emailQueueRepo   domain.EmailQueueRepository
emailQueueWorker *queue.EmailQueueWorker

// In InitRepositories()
a.emailQueueRepo = repository.NewEmailQueueRepository(a.workspaceRepo)

// In InitServices()
a.emailQueueWorker = queue.NewEmailQueueWorker(
    a.emailQueueRepo, a.workspaceRepo, a.emailService,
    queue.DefaultWorkerConfig(), a.logger,
)

// In Start()
a.emailQueueWorker.Start(a.shutdownCtx)

// In Shutdown()
a.emailQueueWorker.Stop()
```

---

## Files Summary

| Action | File | Purpose |
|--------|------|---------|
| Create | `internal/domain/email_queue.go` | Domain types and repository interface |
| Create | `internal/migrations/v21.go` | Database migration |
| Create | `internal/repository/email_queue_postgres.go` | Repository implementation |
| Create | `internal/service/queue/rate_limiter.go` | Provider rate limiting |
| Create | `internal/service/queue/worker.go` | Queue worker pool |
| Modify | `internal/service/broadcast/orchestrator.go` | Enqueue instead of direct send |
| Modify | `internal/service/broadcast/message_sender.go` | Remove rate limiting |
| Modify | `internal/service/automation_node_executor.go` | Enqueue instead of sync send |
| Modify | `internal/app/app.go` | Initialize queue worker |
| Modify | `config/config.go` | Update VERSION to 21.0 |

---

## Testing

**Unit Tests to Create:**
- `internal/domain/email_queue_test.go`
- `internal/repository/email_queue_postgres_test.go`
- `internal/service/queue/rate_limiter_test.go`
- `internal/service/queue/worker_test.go`
- `internal/migrations/v21_test.go`

**Test Commands:**
```bash
make test-domain      # Domain tests
make test-repo        # Repository tests
make test-service     # Service tests
make test-migrations  # Migration tests
make test-integration # End-to-end tests
```

---

## Rollout Strategy

1. Deploy with queue code (workers not processing yet)
2. Run migration (creates tables)
3. Enable queue workers
4. Monitor queue depth, processing rate, dead letter queue
5. Remove legacy rate limiting code after verification
