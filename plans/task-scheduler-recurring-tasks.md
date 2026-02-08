# Task Scheduler Improvements: Recurring Tasks for Native Integrations

## Overview

This plan adds recurring task support to the existing TaskScheduler to enable native integrations (like Staminads) to poll external APIs at configurable intervals. The design reuses the existing task infrastructure with minimal modifications.

## Current Implementation Analysis

### Existing Task Structure (`internal/domain/task.go`)

```go
type Task struct {
    ID            string
    WorkspaceID   string
    Type          string
    Status        TaskStatus      // pending, running, completed, failed, paused
    Progress      float64
    State         *TaskState      // Typed state with specialized fields
    NextRunAfter  *time.Time      // Already used for scheduling
    // ... other fields
}
```

### Current Task Execution Flow (`internal/service/task_service.go`)

1. `TaskScheduler` polls every 5 seconds
2. `GetNextBatch` fetches pending/paused tasks where `next_run_after <= now`
3. `ExecuteTask` runs the processor
4. When processor returns `completed=true` → `MarkAsCompleted`
5. When processor returns `completed=false` → `MarkAsPending` with `next_run_after = now`

### Key Insight

The current system already supports tasks that "continue" by returning `completed=false`. For recurring tasks, we need to:
1. Add a recurring interval field
2. Modify completion behavior to reschedule instead of completing
3. Add integration ID linkage for management

---

## Proposed Changes

### 1. Domain Model Changes (`internal/domain/task.go`)

Add new fields to the `Task` struct:

```go
type Task struct {
    // ... existing fields ...

    // Recurring task support
    RecurringInterval *int64  `json:"recurring_interval,omitempty"` // Interval in seconds (nil = not recurring)
    IntegrationID     *string `json:"integration_id,omitempty"`     // Link to integration for management
}
```

**Why `*int64` instead of `*time.Duration`?**
- JSON serialization is cleaner with seconds
- Database storage is simpler (INTEGER column)
- Matches existing `RetryInterval int` pattern

**Add helper method to Task:**
```go
// IsRecurring returns true if this task has a valid recurring interval
func (t *Task) IsRecurring() bool {
    return t.RecurringInterval != nil && *t.RecurringInterval > 0
}
```

Add new specialized state for integration sync tasks:

```go
// IntegrationSyncState contains state for integration sync tasks
type IntegrationSyncState struct {
    IntegrationID   string     `json:"integration_id"`
    IntegrationType string     `json:"integration_type"` // e.g., "staminads"
    Cursor          string     `json:"cursor,omitempty"` // Pagination cursor for incremental sync
    LastSyncAt      *time.Time `json:"last_sync_at,omitempty"`
    LastSuccessAt   *time.Time `json:"last_success_at,omitempty"` // Last successful sync (distinct from LastSyncAt)
    EventsImported  int64      `json:"events_imported"`  // Total events imported
    LastEventCount  int        `json:"last_event_count"` // Events imported in last run
    ConsecErrors    int        `json:"consec_errors"`    // Consecutive error count
    LastError       *string    `json:"last_error,omitempty"`
    LastErrorType   string     `json:"last_error_type,omitempty"` // "transient", "permanent", or "unknown"
}

// Error type constants for classification
const (
    ErrorTypeTransient = "transient" // Network timeouts, rate limits - use backoff
    ErrorTypePermanent = "permanent" // Invalid API key, disabled integration - fail immediately
    ErrorTypeUnknown   = "unknown"   // Default, treat as transient
)

// Update TaskState to include the new specialized state
type TaskState struct {
    Progress float64 `json:"progress,omitempty"`
    Message  string  `json:"message,omitempty"`

    // Specialized states - only one will be used
    SendBroadcast   *SendBroadcastState   `json:"send_broadcast,omitempty"`
    BuildSegment    *BuildSegmentState    `json:"build_segment,omitempty"`
    IntegrationSync *IntegrationSyncState `json:"integration_sync,omitempty"` // NEW
}
```

### 2. Database Schema Changes

Add new columns to the `tasks` table:

```sql
ALTER TABLE tasks
ADD COLUMN IF NOT EXISTS recurring_interval INTEGER DEFAULT NULL,
ADD COLUMN IF NOT EXISTS integration_id VARCHAR(36) DEFAULT NULL;

-- Index for finding tasks by integration
CREATE INDEX IF NOT EXISTS idx_tasks_integration_id ON tasks(integration_id) WHERE integration_id IS NOT NULL;
```

### 3. Repository Changes (`internal/repository/task_postgres.go`)

Update all queries to include the new columns:

**Create/Update queries:**
- Add `recurring_interval` and `integration_id` to INSERT and UPDATE statements

**GetNextBatch query:**
- No changes needed - recurring tasks use the same `next_run_after` mechanism

**New method implementation - GetTaskByIntegrationID:**
```go
func (r *TaskRepository) GetTaskByIntegrationID(ctx context.Context, workspace, integrationID string) (*domain.Task, error) {
    query := psql.Select(
        "id", "workspace_id", "type", "status", "progress", "state",
        "error_message", "created_at", "updated_at", "last_run_at",
        "completed_at", "next_run_after", "timeout_after", "max_runtime",
        "max_retries", "retry_count", "retry_interval", "broadcast_id",
        "recurring_interval", "integration_id",
    ).From("tasks").Where(sq.Eq{
        "workspace_id":   workspace,
        "integration_id": integrationID,
    })

    sql, args, err := query.ToSql()
    if err != nil {
        return nil, fmt.Errorf("failed to build query: %w", err)
    }

    task := &domain.Task{}
    var recurringInterval, integrationID sql.NullInt64, sql.NullString
    // ... scan all fields including new nullable columns ...

    if recurringInterval.Valid {
        task.RecurringInterval = &recurringInterval.Int64
    }
    if integrationID.Valid {
        task.IntegrationID = &integrationID.String
    }

    return task, nil
}
```

**Row scanner updates required:**
- All `Scan` calls must be updated to handle `recurring_interval` (sql.NullInt64) and `integration_id` (sql.NullString)
- Consider creating a helper: `scanTaskRow(row *sql.Row) (*domain.Task, error)`

### 4. Task Service Changes (`internal/service/task_service.go`)

Modify `ExecuteTask` completion handling:

```go
// In ExecuteTask, after processor returns completed=true
case completed := <-done:
    if completed {
        // Check if this is a recurring task
        if task.RecurringInterval != nil && *task.RecurringInterval > 0 {
            // Reschedule instead of completing
            nextRun := time.Now().UTC().Add(time.Duration(*task.RecurringInterval) * time.Second)
            if err := s.repo.MarkAsPending(bgCtx, workspace, taskID, nextRun, 0, task.State); err != nil {
                // handle error
            }
            s.logger.WithFields(map[string]interface{}{
                "task_id":      taskID,
                "workspace_id": workspace,
                "next_run":     nextRun,
            }).Info("Recurring task rescheduled")
        } else {
            // Non-recurring task - mark as completed (existing behavior)
            if err := s.repo.MarkAsCompleted(bgCtx, workspace, taskID, task.State); err != nil {
                // handle error
            }
        }
    }
```

### 5. Task Repository Interface Update (`internal/domain/task.go`)

Add error sentinel for task not found:

```go
var ErrTaskNotFound = errors.New("task not found")
```

Add method for finding tasks by integration:

```go
type TaskRepository interface {
    // ... existing methods ...

    // GetTaskByIntegrationID retrieves the task for a specific integration
    GetTaskByIntegrationID(ctx context.Context, workspace, integrationID string) (*Task, error)
    GetTaskByIntegrationIDTx(ctx context.Context, tx *sql.Tx, workspace, integrationID string) (*Task, error)
}
```

### 6. New Task Type Registration

Add to `getTaskTypes()` in `task_service.go`:

```go
func getTaskTypes() []string {
    return []string{
        "import_contacts",
        "export_contacts",
        "send_broadcast",
        "generate_report",
        "build_segment",
        "process_contact_segment_queue",
        "check_segment_recompute",
        "sync_integration", // NEW - generic integration sync task
    }
}
```

---

## Integration Lifecycle Management

### Creating a Recurring Task for an Integration

When an integration is enabled/created:

```go
func (s *IntegrationService) CreateSyncTask(ctx context.Context, workspaceID, integrationID string, settings IntegrationSettings) error {
    interval := int64(60) // 60 seconds default, configurable per integration

    task := &domain.Task{
        WorkspaceID:       workspaceID,
        Type:              "sync_integration",
        Status:            domain.TaskStatusPending,
        IntegrationID:     &integrationID,
        RecurringInterval: &interval,
        State: &domain.TaskState{
            Progress: 0,
            Message:  "Initializing sync",
            IntegrationSync: &domain.IntegrationSyncState{
                IntegrationID:   integrationID,
                IntegrationType: settings.Type, // e.g., "staminads"
            },
        },
        MaxRuntime:    50,
        MaxRetries:    3,
        RetryInterval: 300,
    }

    return s.taskService.CreateTask(ctx, workspaceID, task)
}
```

### Pausing/Disabling Integration Sync

```go
func (s *IntegrationService) PauseSyncTask(ctx context.Context, workspaceID, integrationID string) error {
    task, err := s.taskRepo.GetTaskByIntegrationID(ctx, workspaceID, integrationID)
    if err != nil {
        return err
    }

    // Pause indefinitely (24 hours, or until manually resumed)
    nextRun := time.Now().Add(24 * time.Hour)
    return s.taskRepo.MarkAsPaused(ctx, workspaceID, task.ID, nextRun, task.Progress, task.State)
}
```

### Resuming Integration Sync

```go
func (s *IntegrationService) ResumeSyncTask(ctx context.Context, workspaceID, integrationID string) error {
    task, err := s.taskRepo.GetTaskByIntegrationID(ctx, workspaceID, integrationID)
    if err != nil {
        return err
    }

    nextRun := time.Now().UTC()
    return s.taskRepo.MarkAsPending(ctx, workspaceID, task.ID, nextRun, task.Progress, task.State)
}
```

### Deleting Integration (cleanup task)

```go
func (s *IntegrationService) DeleteIntegration(ctx context.Context, workspaceID, integrationID string) error {
    // Delete associated sync task
    task, err := s.taskRepo.GetTaskByIntegrationID(ctx, workspaceID, integrationID)
    if err == nil && task != nil {
        _ = s.taskRepo.Delete(ctx, workspaceID, task.ID)
    }

    // Delete integration
    return s.integrationRepo.Delete(ctx, workspaceID, integrationID)
}
```

### Resetting a Failed Sync Task

When a recurring task has failed (after 10 consecutive errors or permanent error), provide a way to reset it:

```go
func (s *IntegrationService) ResetSyncTask(ctx context.Context, workspaceID, integrationID string) error {
    task, err := s.taskRepo.GetTaskByIntegrationID(ctx, workspaceID, integrationID)
    if err != nil {
        return fmt.Errorf("failed to get task: %w", err)
    }

    if task.Status != domain.TaskStatusFailed {
        return fmt.Errorf("task is not in failed state, current status: %s", task.Status)
    }

    // Reset error state
    if task.State != nil && task.State.IntegrationSync != nil {
        task.State.IntegrationSync.ConsecErrors = 0
        task.State.IntegrationSync.LastError = nil
        task.State.IntegrationSync.LastErrorType = ""
    }

    // Reset retry count and reschedule immediately
    task.RetryCount = 0
    nextRun := time.Now().UTC()

    return s.taskRepo.MarkAsPending(ctx, workspaceID, task.ID, nextRun, 0, task.State)
}
```

---

## Task Flow Diagram

```
┌─────────────────────────────────────────────────────────────────┐
│                 Recurring Task Lifecycle                         │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  Integration Created/Enabled                                    │
│         │                                                       │
│         ▼                                                       │
│  ┌─────────────┐                                                │
│  │   pending   │◄────────────────────────────────┐              │
│  │ next_run=now│                                 │              │
│  └──────┬──────┘                                 │              │
│         │ scheduler picks up                     │              │
│         ▼                                        │              │
│  ┌─────────────┐                                 │              │
│  │   running   │                                 │              │
│  └──────┬──────┘                                 │              │
│         │                                        │              │
│         ▼                                        │              │
│  ┌─────────────────────┐                         │              │
│  │ Processor executes  │                         │              │
│  │ - fetch from API    │                         │              │
│  │ - transform events  │                         │              │
│  │ - upsert to DB      │                         │              │
│  │ - update cursor     │                         │              │
│  └──────┬──────────────┘                         │              │
│         │                                        │              │
│         ▼                                        │              │
│    completed=true                                │              │
│         │                                        │              │
│         ▼                                        │              │
│  ┌──────────────────┐     yes                    │              │
│  │ recurring_interval├───────────────────────────┘              │
│  │    is set?       │   next_run = now + interval               │
│  └────────┬─────────┘   status = pending                        │
│           │ no                                                  │
│           ▼                                                     │
│    ┌─────────────┐                                              │
│    │  completed  │ (non-recurring tasks only)                   │
│    └─────────────┘                                              │
│                                                                 │
│  Integration Disabled → task.status = paused                    │
│  Integration Deleted  → task deleted                            │
└─────────────────────────────────────────────────────────────────┘
```

---

## Error Handling for Recurring Tasks

Recurring tasks need special error handling to prevent infinite retry loops:

```go
// In IntegrationSyncState
type IntegrationSyncState struct {
    // ...
    ConsecErrors int     `json:"consec_errors"` // Consecutive error count
    LastError    *string `json:"last_error,omitempty"`
}
```

**Processor behavior:**
1. On success: reset `ConsecErrors` to 0, return `completed=true`
2. On transient error: increment `ConsecErrors`, return `completed=true` (will be rescheduled)
3. On persistent error (e.g., invalid API key): return error, task marked as failed

**Backoff strategy - applied at reschedule time in task_service.go (NOT in processor):**

> **Important:** Do NOT modify `task.RecurringInterval` in the processor. The backoff should be calculated when rescheduling to avoid permanently altering the stored interval.

```go
// In task_service.go ExecuteTask, when rescheduling a recurring task:
if task.IsRecurring() {
    interval := *task.RecurringInterval

    // Apply exponential backoff based on consecutive errors in state
    if task.State != nil && task.State.IntegrationSync != nil {
        consecErrors := task.State.IntegrationSync.ConsecErrors
        if consecErrors > 0 {
            backoff := int64(min(consecErrors * consecErrors * 10, 3600)) // max 1 hour
            interval += backoff
        }
    }

    // Add jitter (10% of interval) to prevent task clustering in distributed systems
    jitter := time.Duration(rand.Int63n(interval / 10)) * time.Second
    nextRun := time.Now().UTC().Add(time.Duration(interval)*time.Second + jitter)

    // Reset retry count on successful recurring completion
    task.RetryCount = 0

    if err := s.repo.MarkAsPending(bgCtx, workspace, taskID, nextRun, 0, task.State); err != nil {
        // Handle case where task was deleted during execution
        if errors.Is(err, domain.ErrTaskNotFound) {
            s.logger.Info("Recurring task was deleted during execution, skipping reschedule",
                "task_id", taskID, "workspace_id", workspace)
            return nil
        }
        // handle other errors
    }

    s.logger.WithFields(map[string]interface{}{
        "task_id":      taskID,
        "workspace_id": workspace,
        "next_run":     nextRun,
        "interval":     interval,
        "jitter_ms":    jitter.Milliseconds(),
    }).Info("Recurring task rescheduled")
}
```

**Processor classifies errors and updates state:**
```go
func (p *IntegrationSyncProcessor) Process(ctx context.Context, task *domain.Task, timeoutAt time.Time) (bool, error) {
    state := task.State.IntegrationSync

    // ... sync logic ...

    if err != nil {
        errMsg := err.Error()
        state.LastError = &errMsg
        state.LastSyncAt = ptr(time.Now().UTC())

        // Classify error type
        errorType := classifyError(err)
        state.LastErrorType = errorType

        // Permanent errors fail immediately (invalid API key, disabled integration, etc.)
        if errorType == domain.ErrorTypePermanent {
            return false, fmt.Errorf("permanent error, stopping sync: %w", err)
        }

        // Transient errors increment counter and use backoff
        state.ConsecErrors++

        // If too many consecutive transient errors, fail the task
        if state.ConsecErrors >= 10 {
            return false, fmt.Errorf("too many consecutive errors (%d): %w", state.ConsecErrors, err)
        }

        // Return completed=true to trigger reschedule (with backoff applied by service)
        return true, nil
    }

    // Success - reset error counter and update timestamps
    now := time.Now().UTC()
    state.ConsecErrors = 0
    state.LastError = nil
    state.LastErrorType = ""
    state.LastSyncAt = &now
    state.LastSuccessAt = &now

    return true, nil
}

// classifyError determines if an error is transient or permanent
func classifyError(err error) string {
    errMsg := strings.ToLower(err.Error())

    // Permanent errors - no point retrying
    permanentPatterns := []string{
        "invalid api key", "unauthorized", "403 forbidden",
        "integration disabled", "account suspended", "invalid credentials",
    }
    for _, pattern := range permanentPatterns {
        if strings.Contains(errMsg, pattern) {
            return domain.ErrorTypePermanent
        }
    }

    // Transient errors - retry with backoff
    transientPatterns := []string{
        "timeout", "connection refused", "rate limit", "429",
        "503", "502", "504", "temporary", "retry",
    }
    for _, pattern := range transientPatterns {
        if strings.Contains(errMsg, pattern) {
            return domain.ErrorTypeTransient
        }
    }

    return domain.ErrorTypeUnknown // Treat unknown as transient
}
```

---

## Migration Plan

### Database Migration (v28.go)

> **Note:** Migrations v1-v27 already exist. This must be v28.

```go
type V28Migration struct{}

func (m *V28Migration) GetMajorVersion() float64 { return 28.0 }
func (m *V28Migration) HasSystemUpdate() bool    { return true }
func (m *V28Migration) HasWorkspaceUpdate() bool { return false }

func (m *V28Migration) UpdateSystem(ctx context.Context, config *config.Config, db DBExecutor) error {
    _, err := db.ExecContext(ctx, `
        ALTER TABLE tasks
        ADD COLUMN IF NOT EXISTS recurring_interval INTEGER DEFAULT NULL;

        ALTER TABLE tasks
        ADD COLUMN IF NOT EXISTS integration_id VARCHAR(36) DEFAULT NULL;

        CREATE INDEX IF NOT EXISTS idx_tasks_integration_id
        ON tasks(integration_id) WHERE integration_id IS NOT NULL;
    `)
    return err
}
```

---

## Files to Modify

| File | Changes |
|------|---------|
| `internal/domain/task.go` | Add `RecurringInterval`, `IntegrationID` fields; add `IntegrationSyncState` |
| `internal/repository/task_postgres.go` | Update all queries for new columns; add `GetTaskByIntegrationID` |
| `internal/service/task_service.go` | Modify completion handling for recurring tasks; add `sync_integration` type |
| `internal/migrations/v8.go` | Database schema migration |

## Files to Create

| File | Purpose |
|------|---------|
| `internal/service/integration_sync_processor.go` | Generic processor that dispatches to specific integration handlers |

---

## Testing Strategy

### Unit Tests

1. **Domain tests** (`internal/domain/task_test.go`):
   - Test `IntegrationSyncState` JSON serialization
   - Test `Task` with recurring fields

2. **Repository tests** (`internal/repository/task_postgres_test.go`):
   - Test Create/Update with recurring fields
   - Test `GetTaskByIntegrationID`
   - Test GetNextBatch includes recurring tasks correctly

3. **Service tests** (`internal/service/task_service_test.go`):
   - Test recurring task reschedules after completion
   - Test non-recurring task completes normally
   - Test recurring task with errors

### Integration Tests

1. Full lifecycle test:
   - Create recurring task
   - Execute and verify rescheduling
   - Pause and verify not picked up
   - Resume and verify picks up again
   - Delete and verify cleanup

---

## Configuration

No new configuration needed. Recurring interval is set per-task when created. Default intervals can be defined per integration type in their respective services.

---

## Benefits of This Approach

1. **Minimal changes** - Reuses existing TaskScheduler infrastructure
2. **Visible to users** - Tasks appear in task list with status, last run, etc.
3. **Manageable** - Pause/resume/delete via existing task APIs
4. **State preserved** - Cursor survives restarts, stored in task state
5. **Extensible** - Same pattern works for any integration (Staminads, Segment, Mixpanel, etc.)
6. **No new scheduler** - No additional background workers or complexity
7. **Consistent patterns** - Follows existing broadcast task patterns

---

## Processor Registration

The `sync_integration` processor must be registered during app initialization:

```go
// In internal/app/app.go or equivalent initialization code
func (a *App) initializeTaskProcessors() {
    // ... existing processors ...

    // Register integration sync processor
    integrationSyncProcessor := service.NewIntegrationSyncProcessor(
        a.integrationService,
        a.customEventRepo,
        a.logger,
    )
    a.taskService.RegisterProcessor(integrationSyncProcessor)
}
```

---

## Concurrency Considerations

**Problem:** If a recurring task takes longer than its interval, the scheduler might pick it up again while still running.

**Solution:** The existing `status = 'running'` check in `GetNextBatch` already prevents this:
```sql
WHERE status IN ('pending', 'paused') AND next_run_after <= NOW()
```

**Recommendation:** Document that `recurring_interval` should be greater than `max_runtime` to avoid edge cases where a task times out and immediately reschedules.

---

## Test Commands

After implementation, run these tests in order:

```bash
# 1. Domain layer tests (after adding IntegrationSyncState, Task fields)
make test-domain

# 2. Repository tests (after updating queries and adding GetTaskByIntegrationID)
make test-repo

# 3. Service tests (after modifying ExecuteTask completion handling)
make test-service

# 4. Migration tests
make test-migrations

# 5. Full integration test
make test-integration

# 6. Coverage report
make coverage
```

---

## Future Considerations

1. **Rate limiting per integration** - Could add rate limit tracking in `IntegrationSyncState`
2. **Metrics/observability** - Add sync metrics (events/second, latency, errors)
3. **Manual trigger** - API endpoint to trigger immediate sync (set `next_run_after = now`)
4. **Configurable intervals** - Allow users to configure sync frequency per integration
