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

Add new specialized state for integration sync tasks:

```go
// IntegrationSyncState contains state for integration sync tasks
type IntegrationSyncState struct {
    IntegrationID   string     `json:"integration_id"`
    IntegrationType string     `json:"integration_type"` // e.g., "staminads"
    Cursor          string     `json:"cursor,omitempty"` // Pagination cursor for incremental sync
    LastSyncAt      *time.Time `json:"last_sync_at,omitempty"`
    EventsImported  int64      `json:"events_imported"`  // Total events imported
    LastEventCount  int        `json:"last_event_count"` // Events imported in last run
    ConsecErrors    int        `json:"consec_errors"`    // Consecutive error count
    LastError       *string    `json:"last_error,omitempty"`
}

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

**Backoff strategy in processor:**
```go
func (p *IntegrationSyncProcessor) Process(ctx context.Context, task *domain.Task, timeoutAt time.Time) (bool, error) {
    state := task.State.IntegrationSync

    // Exponential backoff based on consecutive errors
    if state.ConsecErrors > 0 {
        backoff := min(state.ConsecErrors * state.ConsecErrors * 10, 3600) // max 1 hour
        if task.RecurringInterval != nil {
            // Temporarily increase interval
            adjustedInterval := *task.RecurringInterval + int64(backoff)
            task.RecurringInterval = &adjustedInterval
        }
    }

    // ... sync logic ...

    if err != nil {
        state.ConsecErrors++
        state.LastError = &err.Error()

        // If too many consecutive errors, fail the task
        if state.ConsecErrors >= 10 {
            return false, fmt.Errorf("too many consecutive errors: %w", err)
        }

        // Otherwise, let it reschedule
        return true, nil
    }

    // Success - reset error counter
    state.ConsecErrors = 0
    state.LastError = nil
    state.LastSyncAt = ptr(time.Now().UTC())

    return true, nil
}
```

---

## Migration Plan

### Database Migration (v8.go or appropriate version)

```go
type V8Migration struct{}

func (m *V8Migration) GetMajorVersion() float64 { return 8.0 }
func (m *V8Migration) HasSystemUpdate() bool    { return true }
func (m *V8Migration) HasWorkspaceUpdate() bool { return false }

func (m *V8Migration) UpdateSystem(ctx context.Context, config *config.Config, db DBExecutor) error {
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

## Future Considerations

1. **Rate limiting per integration** - Could add rate limit tracking in `IntegrationSyncState`
2. **Metrics/observability** - Add sync metrics (events/second, latency, errors)
3. **Manual trigger** - API endpoint to trigger immediate sync (set `next_run_after = now`)
4. **Configurable intervals** - Allow users to configure sync frequency per integration
