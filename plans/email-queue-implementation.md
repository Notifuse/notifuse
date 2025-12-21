# Email Queue System - Implementation Plan

## Overview

This plan introduces a PostgreSQL-based email queue system with separate transactional and marketing queues, rate limiting per provider/workspace, and priority-based processing.

---

## Existing Systems Analysis

### Broadcast Orchestrator (Already Implemented)

The broadcast system (`internal/service/broadcast/orchestrator.go`) already has sophisticated email handling:

| Feature | Status | Implementation |
|---------|--------|----------------|
| Rate Limiting | **Yes** | Per-provider, time-based throttling (default 25/min) |
| Batch Processing | **Yes** | 50 recipients per batch with cursor pagination |
| Resumability | **Yes** | Task-based with `last_processed_email` cursor |
| Circuit Breaker | **Yes** | Pauses after 5 consecutive failures |
| A/B Testing | **Yes** | Template variation selection |

**Key Files:**
- `internal/service/broadcast/orchestrator.go` - Main orchestration
- `internal/service/broadcast/message_sender.go` - Rate limiting + sending
- `internal/service/task_service.go` - Cron-based task execution

### Automation Executor (Current State)

The automation system (`internal/service/automation_executor.go`) sends emails synchronously:

| Feature | Status | Notes |
|---------|--------|-------|
| Rate Limiting | **No** | Direct ESP calls |
| Batch Processing | **No** | One contact at a time |
| Circuit Breaker | **No** | Retries via exponential backoff |
| Async Sending | **No** | Blocks until ESP responds |

### Gap Analysis

| Capability | Broadcast | Automation | Transactional |
|------------|-----------|------------|---------------|
| Rate limiting | Yes | **No** | **No** |
| Priority separation | N/A | **No** | **No** |
| Async sending | Partial | **No** | **No** |
| Unified monitoring | No | No | No |

### Recommended Approach

Rather than replacing the broadcast orchestrator, we should:

1. **Keep broadcast orchestrator** - It works well for bulk sends
2. **Add email queue for automations** - Decouple execution from sending
3. **Add email queue for transactional** - Ensure priority delivery
4. **Unify rate limiting** - Share rate limiters across all systems
5. **Unified monitoring** - Single view of all email activity

---

## Goals

1. **Decouple automation email sending** from execution (broadcasts already decoupled via tasks)
2. **Separate transactional and marketing** emails with priority
3. **Unified rate limiting** across all email sources (broadcasts, automations, transactional)
4. **Rate limit per workspace** to ensure fair usage
5. **Reliable delivery** with retries and dead-letter handling
6. **Unified observability** across all email types

---

## Architecture

### Updated Design (Integrating with Broadcast Orchestrator)

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                             Email Sources                                    │
├───────────────────┬────────────────────┬────────────────────────────────────┤
│ Automation        │ Transactional      │ Broadcast                          │
│ Executor          │ (auth, orders)     │ Orchestrator                       │
└─────────┬─────────┴──────────┬─────────┴───────────────┬────────────────────┘
          │                    │                          │
          ▼                    ▼                          │
┌─────────────────────────────────────────────┐           │
│          Email Queue (PostgreSQL)           │           │
│  ┌─────────────────┐  ┌─────────────────┐  │           │
│  │  Transactional  │  │   Marketing     │  │           │
│  │  (priority: 10) │  │  (priority: 1)  │  │           │
│  │  ← Auth emails  │  │  ← Automations  │  │           │
│  └─────────────────┘  └─────────────────┘  │           │
└─────────────────────────────────────────────┘           │
                    │                                      │
                    ▼                                      ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                    Shared Rate Limiter Service                               │
│  ┌──────────────────────────────────────────────────────────────────────┐   │
│  │                        Rate Limiters                                  │   │
│  │  ┌────────────────┐  ┌─────────────────┐  ┌────────────────────────┐ │   │
│  │  │  Per-Provider  │  │  Per-Workspace  │  │   Global Limiter       │ │   │
│  │  │  SES: 14/s     │  │  10/s each      │  │   100/s total          │ │   │
│  │  │  SMTP: 5/s     │  │                 │  │                        │ │   │
│  │  └────────────────┘  └─────────────────┘  └────────────────────────┘ │   │
│  └──────────────────────────────────────────────────────────────────────┘   │
└──────────────────────────────┬──────────────────────────────────────────────┘
                               │
          ┌────────────────────┴────────────────────┐
          ▼                                         ▼
┌─────────────────────────┐             ┌─────────────────────────┐
│  Email Queue Workers    │             │  Broadcast Orchestrator │
│  (transactional + auto) │             │  (uses shared limiters) │
│                         │             │                         │
│  [Worker 1] [Worker 2]  │             │  [Batch Processor]      │
│  [Worker 3] [Worker 4]  │             │  [Circuit Breaker]      │
└───────────┬─────────────┘             └───────────┬─────────────┘
            │                                       │
            └───────────────────┬───────────────────┘
                                ▼
                    ┌─────────────────────────┐
                    │       ESP APIs          │
                    │   SES / SMTP / etc      │
                    └─────────────────────────┘
```

### Key Design Decisions

1. **Broadcasts keep their orchestrator** - The existing task-based system works well for bulk sends with circuit breaker, A/B testing, and resumability
2. **Shared rate limiter service** - All email sources (queue workers + broadcast orchestrator) share the same ESP rate limits
3. **Queue for automation + transactional only** - Decouples sending from processing; broadcasts don't need the queue
4. **Priority in queue** - Transactional emails (password reset, 2FA) get priority over automation marketing emails

### Integration Points

| Component | Current State | Integration |
|-----------|---------------|-------------|
| Broadcast Orchestrator | Has local rate limiter | Inject `SharedRateLimiterService` |
| Automation Executor | Synchronous send | Enqueue to `email_queue` |
| Transactional Services | Direct ESP call | Enqueue with high priority |
| Email Queue Workers | N/A (new) | Use `SharedRateLimiterService` |

---

## Database Schema

### Migration: V21 - Email Queue Tables

**File:** `internal/migrations/v21.go`

```sql
-- Email queue table (per workspace database)
CREATE TABLE email_queue (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Queue classification
    queue_type VARCHAR(20) NOT NULL,        -- 'transactional' | 'marketing'
    priority INT NOT NULL DEFAULT 1,         -- Higher = process first (transactional: 10, marketing: 1)

    -- Routing
    workspace_id VARCHAR(36) NOT NULL,
    provider VARCHAR(50) NOT NULL,           -- 'ses', 'smtp', 'sendgrid', etc.
    integration_id VARCHAR(36) NOT NULL,

    -- Email content
    message_id VARCHAR(100) NOT NULL UNIQUE, -- For deduplication and tracking
    recipient_email VARCHAR(255) NOT NULL,
    payload JSONB NOT NULL,                  -- Full SendEmailRequest serialized

    -- Source tracking
    source_type VARCHAR(50),                 -- 'automation', 'broadcast', 'transactional'
    source_id VARCHAR(36),                   -- automation_id, broadcast_id, etc.

    -- Processing state
    status VARCHAR(20) NOT NULL DEFAULT 'pending',  -- pending, processing, sent, failed, dead
    scheduled_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- Retry handling
    attempts INT NOT NULL DEFAULT 0,
    max_attempts INT NOT NULL DEFAULT 3,
    last_attempt_at TIMESTAMPTZ,
    last_error TEXT,
    next_retry_at TIMESTAMPTZ,

    -- Timestamps
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    processed_at TIMESTAMPTZ,

    -- Constraints
    CONSTRAINT valid_queue_type CHECK (queue_type IN ('transactional', 'marketing')),
    CONSTRAINT valid_status CHECK (status IN ('pending', 'processing', 'sent', 'failed', 'dead'))
);

-- Primary index: fetch pending emails by priority and schedule
-- Transactional emails (priority 10) always come before marketing (priority 1)
CREATE INDEX idx_email_queue_pending
ON email_queue (priority DESC, scheduled_at ASC)
WHERE status = 'pending';

-- Per-provider index for rate limiting queries
CREATE INDEX idx_email_queue_provider
ON email_queue (provider, status, scheduled_at);

-- Per-workspace index for fair scheduling
CREATE INDEX idx_email_queue_workspace
ON email_queue (workspace_id, status, scheduled_at);

-- Retry index
CREATE INDEX idx_email_queue_retry
ON email_queue (next_retry_at)
WHERE status = 'failed' AND attempts < max_attempts;

-- Source tracking index (for automation/broadcast stats)
CREATE INDEX idx_email_queue_source
ON email_queue (source_type, source_id, status);

-- Message ID lookup (for status checks)
CREATE INDEX idx_email_queue_message_id
ON email_queue (message_id);

-- Dead letter queue table for failed emails
CREATE TABLE email_queue_dead_letter (
    id UUID PRIMARY KEY,
    original_id UUID NOT NULL,
    queue_type VARCHAR(20) NOT NULL,
    workspace_id VARCHAR(36) NOT NULL,
    provider VARCHAR(50) NOT NULL,
    message_id VARCHAR(100) NOT NULL,
    recipient_email VARCHAR(255) NOT NULL,
    payload JSONB NOT NULL,
    source_type VARCHAR(50),
    source_id VARCHAR(36),
    attempts INT NOT NULL,
    errors JSONB NOT NULL,              -- Array of all error messages
    created_at TIMESTAMPTZ NOT NULL,    -- Original creation time
    failed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_dead_letter_workspace
ON email_queue_dead_letter (workspace_id, failed_at DESC);

-- Rate limit tracking table (optional, for persistent rate tracking)
CREATE TABLE email_rate_limits (
    id VARCHAR(100) PRIMARY KEY,        -- 'provider:ses' or 'workspace:xyz'
    limit_type VARCHAR(20) NOT NULL,    -- 'provider' | 'workspace'
    tokens_remaining INT NOT NULL,
    last_refill_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    max_tokens INT NOT NULL,
    refill_rate INT NOT NULL,           -- Tokens per second
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

---

## Domain Layer

### File: `internal/domain/email_queue.go`

```go
package domain

import (
    "context"
    "time"
)

// Queue types
const (
    QueueTypeTransactional = "transactional"
    QueueTypeMarketing     = "marketing"
)

// Queue priorities
const (
    PriorityTransactional = 10
    PriorityMarketing     = 1
)

// Email queue statuses
const (
    EmailQueueStatusPending    = "pending"
    EmailQueueStatusProcessing = "processing"
    EmailQueueStatusSent       = "sent"
    EmailQueueStatusFailed     = "failed"
    EmailQueueStatusDead       = "dead"
)

// QueuedEmail represents an email in the queue
type QueuedEmail struct {
    ID             string                 `json:"id"`
    QueueType      string                 `json:"queue_type"`
    Priority       int                    `json:"priority"`
    WorkspaceID    string                 `json:"workspace_id"`
    Provider       string                 `json:"provider"`
    IntegrationID  string                 `json:"integration_id"`
    MessageID      string                 `json:"message_id"`
    RecipientEmail string                 `json:"recipient_email"`
    Payload        map[string]interface{} `json:"payload"`
    SourceType     *string                `json:"source_type,omitempty"`
    SourceID       *string                `json:"source_id,omitempty"`
    Status         string                 `json:"status"`
    ScheduledAt    time.Time              `json:"scheduled_at"`
    Attempts       int                    `json:"attempts"`
    MaxAttempts    int                    `json:"max_attempts"`
    LastAttemptAt  *time.Time             `json:"last_attempt_at,omitempty"`
    LastError      *string                `json:"last_error,omitempty"`
    NextRetryAt    *time.Time             `json:"next_retry_at,omitempty"`
    CreatedAt      time.Time              `json:"created_at"`
    ProcessedAt    *time.Time             `json:"processed_at,omitempty"`
}

// EnqueueEmailRequest contains parameters for queuing an email
type EnqueueEmailRequest struct {
    QueueType      string                 // transactional or marketing
    WorkspaceID    string
    Provider       string
    IntegrationID  string
    MessageID      string
    RecipientEmail string
    Payload        map[string]interface{} // Serialized SendEmailRequest
    SourceType     *string                // automation, broadcast, transactional
    SourceID       *string                // ID of source entity
    ScheduleAt     *time.Time             // Optional: schedule for later
}

// EmailQueueStats contains queue statistics
type EmailQueueStats struct {
    PendingTransactional int `json:"pending_transactional"`
    PendingMarketing     int `json:"pending_marketing"`
    Processing           int `json:"processing"`
    FailedRetryable      int `json:"failed_retryable"`
    DeadLetter           int `json:"dead_letter"`
    SentLast24h          int `json:"sent_last_24h"`
}

// EmailQueueRepository defines data access for email queue
type EmailQueueRepository interface {
    // Enqueue adds an email to the queue
    Enqueue(ctx context.Context, workspaceID string, req *EnqueueEmailRequest) (*QueuedEmail, error)

    // EnqueueBatch adds multiple emails atomically
    EnqueueBatch(ctx context.Context, workspaceID string, reqs []*EnqueueEmailRequest) ([]*QueuedEmail, error)

    // FetchPending retrieves pending emails respecting priority
    // Returns transactional before marketing, ordered by scheduled_at
    FetchPending(ctx context.Context, limit int) ([]*QueuedEmail, error)

    // FetchPendingByProvider retrieves pending emails for a specific provider
    FetchPendingByProvider(ctx context.Context, provider string, limit int) ([]*QueuedEmail, error)

    // MarkProcessing atomically marks an email as processing (with row lock)
    MarkProcessing(ctx context.Context, workspaceID, id string) error

    // MarkSent marks an email as successfully sent
    MarkSent(ctx context.Context, workspaceID, id string) error

    // MarkFailed marks an email as failed with retry scheduling
    MarkFailed(ctx context.Context, workspaceID, id string, err error) error

    // MoveToDead moves a permanently failed email to dead letter queue
    MoveToDead(ctx context.Context, workspaceID, id string, errors []string) error

    // GetStats returns queue statistics
    GetStats(ctx context.Context, workspaceID string) (*EmailQueueStats, error)

    // GetByMessageID retrieves an email by its message ID
    GetByMessageID(ctx context.Context, workspaceID, messageID string) (*QueuedEmail, error)

    // CleanupOld removes old sent/dead entries
    CleanupOld(ctx context.Context, olderThan time.Duration) (int64, error)
}

// EmailQueueService defines business logic for email queue
type EmailQueueService interface {
    // EnqueueTransactional queues a high-priority transactional email
    EnqueueTransactional(ctx context.Context, req *EnqueueEmailRequest) (*QueuedEmail, error)

    // EnqueueMarketing queues a marketing email
    EnqueueMarketing(ctx context.Context, req *EnqueueEmailRequest) (*QueuedEmail, error)

    // EnqueueForAutomation queues an email from automation (marketing priority)
    EnqueueForAutomation(ctx context.Context, automationID string, req *EnqueueEmailRequest) (*QueuedEmail, error)

    // GetEmailStatus returns the status of a queued email
    GetEmailStatus(ctx context.Context, workspaceID, messageID string) (*QueuedEmail, error)

    // GetQueueStats returns queue statistics for a workspace
    GetQueueStats(ctx context.Context, workspaceID string) (*EmailQueueStats, error)
}
```

---

## Service Layer

### File: `internal/service/email_queue_service.go`

```go
package service

import (
    "context"
    "fmt"
    "time"

    "github.com/Notifuse/notifuse/internal/domain"
    "github.com/Notifuse/notifuse/pkg/logger"
    "github.com/google/uuid"
)

type EmailQueueService struct {
    repo   domain.EmailQueueRepository
    logger logger.Logger
}

func NewEmailQueueService(
    repo domain.EmailQueueRepository,
    log logger.Logger,
) *EmailQueueService {
    return &EmailQueueService{
        repo:   repo,
        logger: log,
    }
}

func (s *EmailQueueService) EnqueueTransactional(ctx context.Context, req *EnqueueEmailRequest) (*domain.QueuedEmail, error) {
    req.QueueType = domain.QueueTypeTransactional
    return s.enqueue(ctx, req, domain.PriorityTransactional)
}

func (s *EmailQueueService) EnqueueMarketing(ctx context.Context, req *EnqueueEmailRequest) (*domain.QueuedEmail, error) {
    req.QueueType = domain.QueueTypeMarketing
    return s.enqueue(ctx, req, domain.PriorityMarketing)
}

func (s *EmailQueueService) EnqueueForAutomation(ctx context.Context, automationID string, req *EnqueueEmailRequest) (*domain.QueuedEmail, error) {
    req.QueueType = domain.QueueTypeMarketing
    sourceType := "automation"
    req.SourceType = &sourceType
    req.SourceID = &automationID
    return s.enqueue(ctx, req, domain.PriorityMarketing)
}

func (s *EmailQueueService) enqueue(ctx context.Context, req *EnqueueEmailRequest, priority int) (*domain.QueuedEmail, error) {
    if req.MessageID == "" {
        req.MessageID = fmt.Sprintf("%s_%s", req.WorkspaceID, uuid.NewString())
    }

    queued, err := s.repo.Enqueue(ctx, req.WorkspaceID, req)
    if err != nil {
        s.logger.WithFields(map[string]interface{}{
            "workspace_id": req.WorkspaceID,
            "recipient":    req.RecipientEmail,
            "queue_type":   req.QueueType,
            "error":        err.Error(),
        }).Error("Failed to enqueue email")
        return nil, err
    }

    s.logger.WithFields(map[string]interface{}{
        "queue_id":     queued.ID,
        "message_id":   queued.MessageID,
        "queue_type":   queued.QueueType,
        "recipient":    queued.RecipientEmail,
    }).Debug("Email enqueued")

    return queued, nil
}

func (s *EmailQueueService) GetEmailStatus(ctx context.Context, workspaceID, messageID string) (*domain.QueuedEmail, error) {
    return s.repo.GetByMessageID(ctx, workspaceID, messageID)
}

func (s *EmailQueueService) GetQueueStats(ctx context.Context, workspaceID string) (*domain.EmailQueueStats, error) {
    return s.repo.GetStats(ctx, workspaceID)
}
```

---

## Shared Rate Limiter Service

This service is the **key integration point** between the email queue workers and the broadcast orchestrator. Both systems use this to respect ESP rate limits.

### File: `internal/service/shared_rate_limiter.go`

```go
package service

import (
    "context"
    "sync"
    "time"

    "golang.org/x/time/rate"
)

// ProviderRateLimits defines default rate limits per ESP (emails per second)
var ProviderRateLimits = map[string]int{
    "ses":       14,   // AWS SES default (can be increased via support request)
    "sendgrid":  100,  // Varies by plan
    "mailgun":   5,    // Free tier is low
    "postmark":  10,
    "sparkpost": 50,
    "smtp":      5,    // Conservative for generic SMTP
    "mailjet":   25,
}

// SharedRateLimiterConfig configures the rate limiter service
type SharedRateLimiterConfig struct {
    GlobalRateLimit    int            // Max emails/sec across all sources
    WorkspaceRateLimit int            // Max emails/sec per workspace
    ProviderOverrides  map[string]int // Override default provider limits
}

// DefaultSharedRateLimiterConfig returns sensible defaults
func DefaultSharedRateLimiterConfig() SharedRateLimiterConfig {
    return SharedRateLimiterConfig{
        GlobalRateLimit:    100,
        WorkspaceRateLimit: 20,
        ProviderOverrides:  map[string]int{},
    }
}

// SharedRateLimiterService manages rate limiting across all email sending
// Used by: EmailQueueWorkers, BroadcastOrchestrator
type SharedRateLimiterService struct {
    config            SharedRateLimiterConfig
    globalLimiter     *rate.Limiter
    providerLimiters  map[string]*rate.Limiter
    workspaceLimiters sync.Map // map[string]*rate.Limiter
    mu                sync.RWMutex
}

// NewSharedRateLimiterService creates a new shared rate limiter
func NewSharedRateLimiterService(config SharedRateLimiterConfig) *SharedRateLimiterService {
    // Initialize provider limiters
    providerLimiters := make(map[string]*rate.Limiter)
    for provider, defaultLimit := range ProviderRateLimits {
        limit := defaultLimit
        if override, ok := config.ProviderOverrides[provider]; ok {
            limit = override
        }
        providerLimiters[provider] = rate.NewLimiter(rate.Limit(limit), limit)
    }

    return &SharedRateLimiterService{
        config:           config,
        globalLimiter:    rate.NewLimiter(rate.Limit(config.GlobalRateLimit), config.GlobalRateLimit),
        providerLimiters: providerLimiters,
    }
}

// Wait blocks until all rate limits allow sending
// Called by email queue workers and broadcast orchestrator
func (s *SharedRateLimiterService) Wait(ctx context.Context, workspaceID, provider string) error {
    // 1. Global rate limit
    if err := s.globalLimiter.Wait(ctx); err != nil {
        return err
    }

    // 2. Provider rate limit
    s.mu.RLock()
    providerLimiter, ok := s.providerLimiters[provider]
    s.mu.RUnlock()
    if ok {
        if err := providerLimiter.Wait(ctx); err != nil {
            return err
        }
    }

    // 3. Workspace rate limit
    wsLimiter := s.getWorkspaceLimiter(workspaceID)
    return wsLimiter.Wait(ctx)
}

// Allow checks if sending is allowed without blocking (non-blocking version)
func (s *SharedRateLimiterService) Allow(workspaceID, provider string) bool {
    if !s.globalLimiter.Allow() {
        return false
    }

    s.mu.RLock()
    providerLimiter, ok := s.providerLimiters[provider]
    s.mu.RUnlock()
    if ok && !providerLimiter.Allow() {
        return false
    }

    return s.getWorkspaceLimiter(workspaceID).Allow()
}

// Reserve reserves a token without blocking (for broadcast's time-based approach)
func (s *SharedRateLimiterService) Reserve(workspaceID, provider string) *RateLimitReservation {
    globalRes := s.globalLimiter.Reserve()

    s.mu.RLock()
    providerLimiter, ok := s.providerLimiters[provider]
    s.mu.RUnlock()

    var providerRes *rate.Reservation
    if ok {
        providerRes = providerLimiter.Reserve()
    }

    wsRes := s.getWorkspaceLimiter(workspaceID).Reserve()

    // Return the maximum delay across all limiters
    return &RateLimitReservation{
        globalRes:   globalRes,
        providerRes: providerRes,
        wsRes:       wsRes,
    }
}

func (s *SharedRateLimiterService) getWorkspaceLimiter(workspaceID string) *rate.Limiter {
    if limiter, ok := s.workspaceLimiters.Load(workspaceID); ok {
        return limiter.(*rate.Limiter)
    }

    limiter := rate.NewLimiter(rate.Limit(s.config.WorkspaceRateLimit), s.config.WorkspaceRateLimit)
    actual, _ := s.workspaceLimiters.LoadOrStore(workspaceID, limiter)
    return actual.(*rate.Limiter)
}

// UpdateProviderLimit dynamically updates a provider's rate limit
// Useful if ESP grants higher limits
func (s *SharedRateLimiterService) UpdateProviderLimit(provider string, limitPerSec int) {
    s.mu.Lock()
    defer s.mu.Unlock()
    s.providerLimiters[provider] = rate.NewLimiter(rate.Limit(limitPerSec), limitPerSec)
}

// GetStats returns current rate limiter statistics
func (s *SharedRateLimiterService) GetStats() map[string]interface{} {
    stats := map[string]interface{}{
        "global_tokens": s.globalLimiter.Tokens(),
    }

    providerStats := make(map[string]float64)
    s.mu.RLock()
    for provider, limiter := range s.providerLimiters {
        providerStats[provider] = limiter.Tokens()
    }
    s.mu.RUnlock()
    stats["provider_tokens"] = providerStats

    return stats
}

// RateLimitReservation holds reservations across all limiters
type RateLimitReservation struct {
    globalRes   *rate.Reservation
    providerRes *rate.Reservation
    wsRes       *rate.Reservation
}

// Delay returns the maximum delay required across all reservations
func (r *RateLimitReservation) Delay() time.Duration {
    maxDelay := r.globalRes.Delay()

    if r.providerRes != nil {
        if d := r.providerRes.Delay(); d > maxDelay {
            maxDelay = d
        }
    }

    if d := r.wsRes.Delay(); d > maxDelay {
        maxDelay = d
    }

    return maxDelay
}

// Cancel cancels all reservations (if sending fails)
func (r *RateLimitReservation) Cancel() {
    r.globalRes.Cancel()
    if r.providerRes != nil {
        r.providerRes.Cancel()
    }
    r.wsRes.Cancel()
}
```

### Integration with Broadcast Orchestrator

Modify `internal/service/broadcast/message_sender.go` to use the shared rate limiter:

```go
// Before (local rate limiting):
type MessageSender struct {
    // ... existing fields
    lastSendTime time.Time
    sendMutex    sync.Mutex
}

func (s *MessageSender) waitForRateLimit(ratePerMin int) {
    // Local time-based rate limiting
}

// After (shared rate limiter):
type MessageSender struct {
    // ... existing fields
    rateLimiter *SharedRateLimiterService  // Injected dependency
}

func (s *MessageSender) SendMessage(ctx context.Context, msg *Message) error {
    // Use shared rate limiter before sending
    if err := s.rateLimiter.Wait(ctx, msg.WorkspaceID, msg.Provider); err != nil {
        return err
    }

    // Send email...
}
```

---

## Worker Implementation

### File: `internal/service/email_worker.go`

```go
package service

import (
    "context"
    "sync"
    "time"

    "github.com/Notifuse/notifuse/internal/domain"
    "github.com/Notifuse/notifuse/pkg/logger"
    "golang.org/x/time/rate"
)

// ProviderRateLimits defines rate limits per ESP
var ProviderRateLimits = map[string]int{
    "ses":       14,  // AWS SES default
    "sendgrid":  100, // Depends on plan
    "mailgun":   5,   // Conservative
    "postmark":  10,
    "smtp":      5,   // Conservative for generic SMTP
    "sparkpost": 50,
}

// EmailWorkerConfig contains worker configuration
type EmailWorkerConfig struct {
    WorkerCount         int           // Number of concurrent workers
    PollInterval        time.Duration // How often to poll for emails
    BatchSize           int           // Emails to fetch per poll
    GlobalRateLimit     int           // Max emails/sec globally
    WorkspaceRateLimit  int           // Max emails/sec per workspace
    ShutdownTimeout     time.Duration // Grace period for shutdown
}

// DefaultEmailWorkerConfig returns sensible defaults
func DefaultEmailWorkerConfig() EmailWorkerConfig {
    return EmailWorkerConfig{
        WorkerCount:        10,
        PollInterval:       time.Second,
        BatchSize:          100,
        GlobalRateLimit:    50,
        WorkspaceRateLimit: 10,
        ShutdownTimeout:    30 * time.Second,
    }
}

// EmailWorkerPool manages email sending workers
type EmailWorkerPool struct {
    config          EmailWorkerConfig
    queueRepo       domain.EmailQueueRepository
    emailService    domain.EmailServiceInterface
    logger          logger.Logger

    // Rate limiters
    globalLimiter    *rate.Limiter
    providerLimiters map[string]*rate.Limiter
    workspaceLimiters sync.Map // map[string]*rate.Limiter
    limiterMu        sync.RWMutex

    // Lifecycle
    jobs        chan *domain.QueuedEmail
    stopChan    chan struct{}
    stoppedChan chan struct{}
    wg          sync.WaitGroup
    running     bool
    mu          sync.Mutex
}

// NewEmailWorkerPool creates a new worker pool
func NewEmailWorkerPool(
    config EmailWorkerConfig,
    queueRepo domain.EmailQueueRepository,
    emailService domain.EmailServiceInterface,
    log logger.Logger,
) *EmailWorkerPool {
    // Initialize provider rate limiters
    providerLimiters := make(map[string]*rate.Limiter)
    for provider, limit := range ProviderRateLimits {
        providerLimiters[provider] = rate.NewLimiter(rate.Limit(limit), limit)
    }

    return &EmailWorkerPool{
        config:           config,
        queueRepo:        queueRepo,
        emailService:     emailService,
        logger:           log,
        globalLimiter:    rate.NewLimiter(rate.Limit(config.GlobalRateLimit), config.GlobalRateLimit),
        providerLimiters: providerLimiters,
        jobs:             make(chan *domain.QueuedEmail, config.BatchSize),
        stopChan:         make(chan struct{}),
        stoppedChan:      make(chan struct{}),
    }
}

// Start begins the worker pool
func (p *EmailWorkerPool) Start(ctx context.Context) {
    p.mu.Lock()
    if p.running {
        p.mu.Unlock()
        return
    }
    p.running = true
    p.mu.Unlock()

    p.logger.WithFields(map[string]interface{}{
        "workers":      p.config.WorkerCount,
        "poll_interval": p.config.PollInterval,
        "batch_size":   p.config.BatchSize,
    }).Info("Starting email worker pool")

    // Start workers
    for i := 0; i < p.config.WorkerCount; i++ {
        p.wg.Add(1)
        go p.worker(ctx, i)
    }

    // Start dispatcher
    go p.dispatcher(ctx)
}

// Stop gracefully stops the worker pool
func (p *EmailWorkerPool) Stop() {
    p.mu.Lock()
    if !p.running {
        p.mu.Unlock()
        return
    }
    p.running = false
    p.mu.Unlock()

    p.logger.Info("Stopping email worker pool...")
    close(p.stopChan)

    // Wait for workers with timeout
    done := make(chan struct{})
    go func() {
        p.wg.Wait()
        close(done)
    }()

    select {
    case <-done:
        p.logger.Info("Email worker pool stopped gracefully")
    case <-time.After(p.config.ShutdownTimeout):
        p.logger.Warn("Email worker pool shutdown timeout exceeded")
    }

    close(p.stoppedChan)
}

// dispatcher fetches emails and sends to workers
func (p *EmailWorkerPool) dispatcher(ctx context.Context) {
    ticker := time.NewTicker(p.config.PollInterval)
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            return
        case <-p.stopChan:
            close(p.jobs) // Signal workers to stop
            return
        case <-ticker.C:
            p.fetchAndDispatch(ctx)
        }
    }
}

func (p *EmailWorkerPool) fetchAndDispatch(ctx context.Context) {
    // Fetch pending emails (already ordered by priority)
    emails, err := p.queueRepo.FetchPending(ctx, p.config.BatchSize)
    if err != nil {
        p.logger.WithField("error", err.Error()).Error("Failed to fetch pending emails")
        return
    }

    for _, email := range emails {
        // Mark as processing before sending to worker
        if err := p.queueRepo.MarkProcessing(ctx, email.WorkspaceID, email.ID); err != nil {
            p.logger.WithFields(map[string]interface{}{
                "email_id": email.ID,
                "error":    err.Error(),
            }).Warn("Failed to mark email as processing, skipping")
            continue
        }

        select {
        case p.jobs <- email:
            // Sent to worker
        case <-p.stopChan:
            return
        }
    }
}

// worker processes emails from the jobs channel
func (p *EmailWorkerPool) worker(ctx context.Context, id int) {
    defer p.wg.Done()

    p.logger.WithField("worker_id", id).Debug("Email worker started")

    for email := range p.jobs {
        p.processEmail(ctx, email)
    }

    p.logger.WithField("worker_id", id).Debug("Email worker stopped")
}

func (p *EmailWorkerPool) processEmail(ctx context.Context, email *domain.QueuedEmail) {
    // Apply rate limits
    if err := p.waitForRateLimits(ctx, email); err != nil {
        p.logger.WithFields(map[string]interface{}{
            "email_id": email.ID,
            "error":    err.Error(),
        }).Warn("Rate limit wait cancelled")
        return
    }

    // Send email
    err := p.sendEmail(ctx, email)

    if err != nil {
        p.handleSendError(ctx, email, err)
    } else {
        p.handleSendSuccess(ctx, email)
    }
}

func (p *EmailWorkerPool) waitForRateLimits(ctx context.Context, email *domain.QueuedEmail) error {
    // Global rate limit
    if err := p.globalLimiter.Wait(ctx); err != nil {
        return err
    }

    // Provider rate limit
    p.limiterMu.RLock()
    providerLimiter, ok := p.providerLimiters[email.Provider]
    p.limiterMu.RUnlock()

    if ok {
        if err := providerLimiter.Wait(ctx); err != nil {
            return err
        }
    }

    // Workspace rate limit (lazy initialization)
    wsLimiter := p.getWorkspaceLimiter(email.WorkspaceID)
    if err := wsLimiter.Wait(ctx); err != nil {
        return err
    }

    return nil
}

func (p *EmailWorkerPool) getWorkspaceLimiter(workspaceID string) *rate.Limiter {
    if limiter, ok := p.workspaceLimiters.Load(workspaceID); ok {
        return limiter.(*rate.Limiter)
    }

    limiter := rate.NewLimiter(rate.Limit(p.config.WorkspaceRateLimit), p.config.WorkspaceRateLimit)
    actual, _ := p.workspaceLimiters.LoadOrStore(workspaceID, limiter)
    return actual.(*rate.Limiter)
}

func (p *EmailWorkerPool) sendEmail(ctx context.Context, email *domain.QueuedEmail) error {
    // Deserialize payload back to SendEmailRequest
    // This is a simplified version - actual implementation would reconstruct the full request

    // Call the email service
    // emailService.SendEmailForTemplate(ctx, request)

    // For now, placeholder:
    return nil // TODO: Implement actual send
}

func (p *EmailWorkerPool) handleSendSuccess(ctx context.Context, email *domain.QueuedEmail) {
    if err := p.queueRepo.MarkSent(ctx, email.WorkspaceID, email.ID); err != nil {
        p.logger.WithFields(map[string]interface{}{
            "email_id": email.ID,
            "error":    err.Error(),
        }).Error("Failed to mark email as sent")
    }

    p.logger.WithFields(map[string]interface{}{
        "email_id":   email.ID,
        "message_id": email.MessageID,
        "recipient":  email.RecipientEmail,
        "queue_type": email.QueueType,
    }).Debug("Email sent successfully")
}

func (p *EmailWorkerPool) handleSendError(ctx context.Context, email *domain.QueuedEmail, sendErr error) {
    p.logger.WithFields(map[string]interface{}{
        "email_id":   email.ID,
        "message_id": email.MessageID,
        "recipient":  email.RecipientEmail,
        "attempts":   email.Attempts + 1,
        "error":      sendErr.Error(),
    }).Warn("Failed to send email")

    if email.Attempts+1 >= email.MaxAttempts {
        // Move to dead letter queue
        errors := []string{sendErr.Error()}
        if email.LastError != nil {
            errors = append([]string{*email.LastError}, errors...)
        }

        if err := p.queueRepo.MoveToDead(ctx, email.WorkspaceID, email.ID, errors); err != nil {
            p.logger.WithField("error", err.Error()).Error("Failed to move email to dead letter queue")
        }
    } else {
        // Mark for retry with backoff
        if err := p.queueRepo.MarkFailed(ctx, email.WorkspaceID, email.ID, sendErr); err != nil {
            p.logger.WithField("error", err.Error()).Error("Failed to mark email as failed")
        }
    }
}
```

---

## Repository Implementation

### File: `internal/repository/email_queue_postgres.go`

```go
package repository

import (
    "context"
    "database/sql"
    "encoding/json"
    "fmt"
    "time"

    sq "github.com/Masterminds/squirrel"
    "github.com/Notifuse/notifuse/internal/domain"
    "github.com/google/uuid"
)

type EmailQueueRepository struct {
    workspaceRepo domain.WorkspaceRepository
    db            *sql.DB // For testing
}

func NewEmailQueueRepository(workspaceRepo domain.WorkspaceRepository) domain.EmailQueueRepository {
    return &EmailQueueRepository{workspaceRepo: workspaceRepo}
}

func (r *EmailQueueRepository) getDB(ctx context.Context, workspaceID string) (*sql.DB, error) {
    if r.db != nil {
        return r.db, nil
    }
    return r.workspaceRepo.GetConnection(ctx, workspaceID)
}

var emailQueuePsql = sq.StatementBuilder.PlaceholderFormat(sq.Dollar)

func (r *EmailQueueRepository) Enqueue(ctx context.Context, workspaceID string, req *domain.EnqueueEmailRequest) (*domain.QueuedEmail, error) {
    db, err := r.getDB(ctx, workspaceID)
    if err != nil {
        return nil, err
    }

    id := uuid.NewString()
    priority := domain.PriorityMarketing
    if req.QueueType == domain.QueueTypeTransactional {
        priority = domain.PriorityTransactional
    }

    scheduledAt := time.Now().UTC()
    if req.ScheduleAt != nil {
        scheduledAt = *req.ScheduleAt
    }

    payloadJSON, err := json.Marshal(req.Payload)
    if err != nil {
        return nil, fmt.Errorf("failed to marshal payload: %w", err)
    }

    query := `
        INSERT INTO email_queue (
            id, queue_type, priority, workspace_id, provider, integration_id,
            message_id, recipient_email, payload, source_type, source_id,
            status, scheduled_at, created_at
        ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
        RETURNING id, created_at
    `

    var createdAt time.Time
    err = db.QueryRowContext(ctx, query,
        id, req.QueueType, priority, workspaceID, req.Provider, req.IntegrationID,
        req.MessageID, req.RecipientEmail, payloadJSON, req.SourceType, req.SourceID,
        domain.EmailQueueStatusPending, scheduledAt, time.Now().UTC(),
    ).Scan(&id, &createdAt)

    if err != nil {
        return nil, fmt.Errorf("failed to enqueue email: %w", err)
    }

    return &domain.QueuedEmail{
        ID:             id,
        QueueType:      req.QueueType,
        Priority:       priority,
        WorkspaceID:    workspaceID,
        Provider:       req.Provider,
        IntegrationID:  req.IntegrationID,
        MessageID:      req.MessageID,
        RecipientEmail: req.RecipientEmail,
        Payload:        req.Payload,
        SourceType:     req.SourceType,
        SourceID:       req.SourceID,
        Status:         domain.EmailQueueStatusPending,
        ScheduledAt:    scheduledAt,
        CreatedAt:      createdAt,
    }, nil
}

func (r *EmailQueueRepository) FetchPending(ctx context.Context, limit int) ([]*domain.QueuedEmail, error) {
    // This needs to fetch across all workspaces - similar to GetScheduledContactAutomationsGlobal
    // For simplicity, assuming single-tenant or central queue for now

    query := `
        SELECT id, queue_type, priority, workspace_id, provider, integration_id,
               message_id, recipient_email, payload, source_type, source_id,
               status, scheduled_at, attempts, max_attempts, last_attempt_at,
               last_error, next_retry_at, created_at, processed_at
        FROM email_queue
        WHERE status = 'pending'
          AND scheduled_at <= NOW()
        ORDER BY priority DESC, scheduled_at ASC
        LIMIT $1
        FOR UPDATE SKIP LOCKED
    `

    // Implementation would iterate workspaces similar to automation scheduler
    // Omitted for brevity

    return nil, nil
}

func (r *EmailQueueRepository) MarkProcessing(ctx context.Context, workspaceID, id string) error {
    db, err := r.getDB(ctx, workspaceID)
    if err != nil {
        return err
    }

    query := `
        UPDATE email_queue
        SET status = 'processing', last_attempt_at = NOW(), attempts = attempts + 1
        WHERE id = $1 AND status = 'pending'
    `

    result, err := db.ExecContext(ctx, query, id)
    if err != nil {
        return err
    }

    rows, _ := result.RowsAffected()
    if rows == 0 {
        return fmt.Errorf("email not found or already processing: %s", id)
    }

    return nil
}

func (r *EmailQueueRepository) MarkSent(ctx context.Context, workspaceID, id string) error {
    db, err := r.getDB(ctx, workspaceID)
    if err != nil {
        return err
    }

    query := `
        UPDATE email_queue
        SET status = 'sent', processed_at = NOW()
        WHERE id = $1
    `

    _, err = db.ExecContext(ctx, query, id)
    return err
}

func (r *EmailQueueRepository) MarkFailed(ctx context.Context, workspaceID, id string, sendErr error) error {
    db, err := r.getDB(ctx, workspaceID)
    if err != nil {
        return err
    }

    // Exponential backoff: 1min, 2min, 4min, 8min...
    query := `
        UPDATE email_queue
        SET status = 'failed',
            last_error = $2,
            next_retry_at = NOW() + (INTERVAL '1 minute' * POWER(2, attempts - 1))
        WHERE id = $1
    `

    _, err = db.ExecContext(ctx, query, id, sendErr.Error())
    return err
}

func (r *EmailQueueRepository) MoveToDead(ctx context.Context, workspaceID, id string, errors []string) error {
    db, err := r.getDB(ctx, workspaceID)
    if err != nil {
        return err
    }

    errorsJSON, _ := json.Marshal(errors)

    // Move to dead letter and delete from main queue
    query := `
        WITH deleted AS (
            DELETE FROM email_queue WHERE id = $1 RETURNING *
        )
        INSERT INTO email_queue_dead_letter (
            id, original_id, queue_type, workspace_id, provider, message_id,
            recipient_email, payload, source_type, source_id, attempts, errors,
            created_at, failed_at
        )
        SELECT gen_random_uuid(), id, queue_type, workspace_id, provider, message_id,
               recipient_email, payload, source_type, source_id, attempts, $2::jsonb,
               created_at, NOW()
        FROM deleted
    `

    _, err = db.ExecContext(ctx, query, id, errorsJSON)
    return err
}

func (r *EmailQueueRepository) GetStats(ctx context.Context, workspaceID string) (*domain.EmailQueueStats, error) {
    db, err := r.getDB(ctx, workspaceID)
    if err != nil {
        return nil, err
    }

    query := `
        SELECT
            COUNT(*) FILTER (WHERE status = 'pending' AND queue_type = 'transactional') as pending_transactional,
            COUNT(*) FILTER (WHERE status = 'pending' AND queue_type = 'marketing') as pending_marketing,
            COUNT(*) FILTER (WHERE status = 'processing') as processing,
            COUNT(*) FILTER (WHERE status = 'failed' AND attempts < max_attempts) as failed_retryable,
            (SELECT COUNT(*) FROM email_queue_dead_letter WHERE workspace_id = $1) as dead_letter,
            COUNT(*) FILTER (WHERE status = 'sent' AND processed_at > NOW() - INTERVAL '24 hours') as sent_last_24h
        FROM email_queue
        WHERE workspace_id = $1
    `

    var stats domain.EmailQueueStats
    err = db.QueryRowContext(ctx, query, workspaceID).Scan(
        &stats.PendingTransactional,
        &stats.PendingMarketing,
        &stats.Processing,
        &stats.FailedRetryable,
        &stats.DeadLetter,
        &stats.SentLast24h,
    )

    return &stats, err
}
```

---

## Integration with Automation Executor

### Changes to `internal/service/automation_node_executor.go`

```go
// EmailNodeExecutor - Modified to use queue

func (e *EmailNodeExecutor) Execute(ctx context.Context, params NodeExecutionParams) (*NodeExecutionResult, error) {
    // ... existing validation code ...

    // Instead of sending directly, enqueue the email
    enqueueReq := &domain.EnqueueEmailRequest{
        QueueType:      domain.QueueTypeMarketing,
        WorkspaceID:    params.WorkspaceID,
        Provider:       emailProvider.Kind,
        IntegrationID:  integrationID,
        MessageID:      messageID,
        RecipientEmail: params.ContactData.Email,
        Payload:        serializeEmailRequest(request), // Serialize full request
        SourceType:     strPtr("automation"),
        SourceID:       &params.Automation.ID,
    }

    queued, err := e.emailQueueService.EnqueueForAutomation(ctx, params.Automation.ID, enqueueReq)
    if err != nil {
        return nil, fmt.Errorf("failed to enqueue email: %w", err)
    }

    // Return immediately - email will be sent async
    return &NodeExecutionResult{
        NextNodeID: params.Node.NextNodeID,
        Status:     domain.ContactAutomationStatusActive,
        Output: buildNodeOutput(domain.NodeTypeEmail, map[string]interface{}{
            "template_id": config.TemplateID,
            "message_id":  messageID,
            "queue_id":    queued.ID,
            "to":          params.ContactData.Email,
            "status":      "queued",
        }),
    }, nil
}
```

---

## Application Startup

### Changes to `internal/app/app.go`

```go
// Add to App struct
type App struct {
    // ... existing fields ...
    emailQueueService *service.EmailQueueService
    emailWorkerPool   *service.EmailWorkerPool
}

// In initialization
func (a *App) initializeServices() {
    // ... existing initialization ...

    // Email Queue
    emailQueueRepo := repository.NewEmailQueueRepository(workspaceRepo)
    a.emailQueueService = service.NewEmailQueueService(emailQueueRepo, a.logger)

    // Email Worker Pool
    workerConfig := service.DefaultEmailWorkerConfig()
    a.emailWorkerPool = service.NewEmailWorkerPool(
        workerConfig,
        emailQueueRepo,
        emailService,
        a.logger,
    )
}

// In Start()
func (a *App) Start(ctx context.Context) {
    // ... existing startup ...

    // Start email workers (after a delay, similar to automation scheduler)
    if !a.config.IsDemo() {
        time.AfterFunc(30*time.Second, func() {
            a.emailWorkerPool.Start(ctx)
        })
    }
}

// In Shutdown()
func (a *App) Shutdown() {
    // ... existing shutdown ...
    a.emailWorkerPool.Stop()
}
```

---

## Testing Strategy

### Unit Tests

| Component | Test File | Coverage |
|-----------|-----------|----------|
| EmailQueueService | `email_queue_service_test.go` | Enqueue, priority, deduplication |
| EmailQueueRepository | `email_queue_postgres_test.go` | CRUD, FetchPending ordering, MarkFailed retry |
| EmailWorkerPool | `email_worker_test.go` | Rate limiting, error handling, graceful shutdown |

### Integration Tests

1. **End-to-end queue flow**: Enqueue → Worker picks up → Sends → Marks sent
2. **Priority ordering**: Transactional always before marketing
3. **Rate limiting**: Verify ESP limits respected
4. **Dead letter**: Failed emails move to DLQ after max retries
5. **Automation integration**: Email node enqueues, worker sends

---

## Monitoring & Observability

### Metrics to Add

```go
// Prometheus metrics
var (
    emailQueueDepth = prometheus.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "email_queue_depth",
            Help: "Number of emails in queue by type and status",
        },
        []string{"queue_type", "status"},
    )

    emailSendDuration = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "email_send_duration_seconds",
            Help:    "Time to send an email",
            Buckets: prometheus.DefBuckets,
        },
        []string{"provider", "queue_type"},
    )

    emailSendTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "email_send_total",
            Help: "Total emails sent by outcome",
        },
        []string{"provider", "queue_type", "status"},
    )
)
```

### Health Check Endpoint

```go
// GET /api/health/email-queue
{
    "status": "healthy",
    "workers_running": 10,
    "queue_depth": {
        "transactional_pending": 5,
        "marketing_pending": 1250,
        "processing": 10
    },
    "rate_limits": {
        "global_available": 45,
        "ses_available": 12
    }
}
```

---

## Migration Steps

1. **Phase 1**: Create `SharedRateLimiterService` (no DB changes, can deploy first)
2. **Phase 2**: Integrate shared limiter with broadcast orchestrator (replaces local rate limiter)
3. **Phase 3**: Create email queue tables (V21 migration)
4. **Phase 4**: Implement queue repository and service
5. **Phase 5**: Implement email queue worker pool
6. **Phase 6**: Update `EmailNodeExecutor` to use queue
7. **Phase 7**: Update transactional email senders to use queue
8. **Phase 8**: Add monitoring/metrics
9. **Phase 9**: Load testing and tuning

**Note**: Phases 1-2 can be deployed independently to unify rate limiting before introducing the queue.

---

## Files to Create/Modify

| Action | File | Purpose |
|--------|------|---------|
| Create | `internal/migrations/v21.go` | Email queue tables |
| Create | `internal/domain/email_queue.go` | Queue domain types |
| Create | `internal/service/email_queue_service.go` | Queue enqueue logic |
| Create | `internal/service/email_worker.go` | Worker pool for queue |
| Create | `internal/service/shared_rate_limiter.go` | **Shared rate limiting** |
| Create | `internal/repository/email_queue_postgres.go` | Queue data access |
| Modify | `internal/service/automation_node_executor.go` | Use queue instead of direct send |
| Modify | `internal/service/broadcast/message_sender.go` | **Use shared rate limiter** |
| Modify | `internal/service/broadcast/orchestrator.go` | Inject shared rate limiter |
| Modify | `internal/app/app.go` | Initialize queue + workers |
| Create | `internal/service/email_queue_service_test.go` | Unit tests |
| Create | `internal/service/email_worker_test.go` | Worker tests |
| Create | `internal/service/shared_rate_limiter_test.go` | Rate limiter tests |
| Create | `internal/repository/email_queue_postgres_test.go` | Repository tests |

---

## Configuration Options

```yaml
# config.yaml additions
email_queue:
  enabled: true
  worker_count: 10
  poll_interval: 1s
  batch_size: 100
  global_rate_limit: 50      # emails/sec
  workspace_rate_limit: 10   # emails/sec per workspace

  provider_rate_limits:
    ses: 14
    sendgrid: 100
    smtp: 5
    mailgun: 5
    postmark: 10
    sparkpost: 50

  retry:
    max_attempts: 3
    backoff_base: 1m         # Exponential: 1m, 2m, 4m

  cleanup:
    enabled: true
    sent_retention: 7d       # Keep sent emails for 7 days
    dead_letter_retention: 30d
```

---

## Conclusion

This implementation provides:

- **Separation** of transactional and marketing queues
- **Priority-based** processing (transactional first)
- **Rate limiting** at global, provider, and workspace levels
- **Reliable delivery** with retries and dead-letter queue
- **Observability** with metrics and health checks
- **Graceful shutdown** for zero-downtime deployments

The PostgreSQL-based approach requires no new infrastructure dependencies while providing the reliability and features needed for production email delivery at scale.
