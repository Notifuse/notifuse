# Automations Feature Implementation Plan

## Overview

This plan implements a comprehensive automation system for Notifuse that allows users to create visual workflow automations triggered by contact timeline events. Automations run asynchronously using the existing task system (similar to segments) and provide a drag-and-drop visual interface built with ReactFlow.

**Updated with competitive insights from Loops.co analysis** - incorporating industry best practices for pause protection, parallel branching, and advanced A/B testing while maintaining our enterprise-grade features.

## Architecture Overview

Automations follow the same async processing pattern as segments:
1. **Event-driven**: Contact timeline events trigger automation executions
2. **Task-based processing**: Automations run as background tasks
3. **Evergreen task**: A recurring task checks for pending automation executions
4. **State tracking**: Automation execution state is persisted for resumability
5. **Flow validation**: Comprehensive validation prevents invalid automation flows
6. **Deduplication**: Prevents contacts from entering the same automation multiple times
7. **Variable context**: Data flows between nodes for dynamic workflows

## Database Schema Changes (Migration v7.0 or v8.0)

**Note**: Check current version in `config/config.go` before deciding on migration version number.

### Automations Table

```sql
CREATE TABLE IF NOT EXISTS automations (
    id VARCHAR(36) PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    status VARCHAR(20) NOT NULL DEFAULT 'draft', -- draft, active, paused
    
    -- Flow definition (ReactFlow structure)
    flow_definition JSONB NOT NULL,
    
    -- Trigger configuration
    trigger_type VARCHAR(50) NOT NULL, -- contact_created, contact_updated, list_subscribed, list_unsubscribed, custom_event
    trigger_config JSONB, -- Additional trigger settings
    
    -- Entry rules (deduplication and cooldown)
    entry_rules JSONB DEFAULT '{"allow_multiple": false, "cooldown_hours": 24}'::jsonb,
    
    -- Testing mode
    test_mode BOOLEAN DEFAULT false,
    
    -- Version control
    version INTEGER DEFAULT 1,
    previous_version JSONB, -- Stores previous flow_definition for rollback
    
    -- Pause management (from Loops.co insight)
    paused_at TIMESTAMP WITH TIME ZONE, -- When automation was paused
    pause_warning_sent BOOLEAN DEFAULT false, -- Whether 24h warning was sent
    
    -- Statistics
    executions_count INTEGER DEFAULT 0,
    successes_count INTEGER DEFAULT 0,
    failures_count INTEGER DEFAULT 0,
    
    -- Timestamps
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    last_executed_at TIMESTAMP WITH TIME ZONE,
    created_by VARCHAR(36), -- User ID who created the automation
    updated_by VARCHAR(36)  -- User ID who last updated the automation
);

CREATE INDEX IF NOT EXISTS idx_automations_status ON automations(status);
CREATE INDEX IF NOT EXISTS idx_automations_trigger_type ON automations(trigger_type);
CREATE INDEX IF NOT EXISTS idx_automations_test_mode ON automations(test_mode);
```

### Automation Executions Table

Tracks individual automation runs for specific contacts.

```sql
CREATE TABLE IF NOT EXISTS automation_executions (
    id VARCHAR(36) PRIMARY KEY,
    automation_id VARCHAR(36) NOT NULL REFERENCES automations(id) ON DELETE CASCADE,
    contact_email VARCHAR(255) NOT NULL,
    
    -- Execution state
    status VARCHAR(20) NOT NULL DEFAULT 'pending', -- pending, running, completed, failed, cancelled, waiting_for_event
    current_node_id VARCHAR(36), -- Current node in flow being executed
    execution_path JSONB, -- Array of node IDs that have been executed
    execution_context JSONB, -- Full execution context with variables
    
    -- Completion tracking (no exit node needed)
    completed_at_node_id VARCHAR(36), -- Last node executed before completion
    completion_reason VARCHAR(50), -- natural_end, condition_branch, timeout, error
    
    -- Test mode tracking
    is_test_run BOOLEAN DEFAULT false,
    
    -- Scheduling
    scheduled_at TIMESTAMP WITH TIME ZONE,
    started_at TIMESTAMP WITH TIME ZONE,
    completed_at TIMESTAMP WITH TIME ZONE,
    
    -- Wait for event (for wait_for_event node)
    waiting_for_event_type VARCHAR(50),
    wait_timeout_at TIMESTAMP WITH TIME ZONE,
    
    -- Error tracking
    error_message TEXT,
    retry_count INTEGER DEFAULT 0,
    
    -- Timestamps
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_automation_executions_automation_id ON automation_executions(automation_id);
CREATE INDEX IF NOT EXISTS idx_automation_executions_contact_email ON automation_executions(contact_email);
CREATE INDEX IF NOT EXISTS idx_automation_executions_status ON automation_executions(status);
CREATE INDEX IF NOT EXISTS idx_automation_executions_scheduled_at ON automation_executions(scheduled_at);
CREATE INDEX IF NOT EXISTS idx_automation_executions_is_test_run ON automation_executions(is_test_run);
CREATE INDEX IF NOT EXISTS idx_automation_executions_waiting_for_event ON automation_executions(waiting_for_event_type) WHERE waiting_for_event_type IS NOT NULL;

-- Unique constraint to prevent duplicate active executions
CREATE UNIQUE INDEX IF NOT EXISTS idx_automation_executions_unique_entry 
ON automation_executions(automation_id, contact_email) 
WHERE status IN ('pending', 'running', 'waiting_for_event');
```

### Automation Analytics Table

Tracks daily metrics for each automation.

```sql
CREATE TABLE IF NOT EXISTS automation_analytics (
    id VARCHAR(36) PRIMARY KEY,
    automation_id VARCHAR(36) NOT NULL REFERENCES automations(id) ON DELETE CASCADE,
    date DATE NOT NULL,
    
    -- Funnel metrics
    triggered_count INTEGER DEFAULT 0,
    started_count INTEGER DEFAULT 0,
    completed_count INTEGER DEFAULT 0,
    failed_count INTEGER DEFAULT 0,
    
    -- Node-level metrics
    node_stats JSONB DEFAULT '{}'::jsonb, -- {"node_id": {"executions": 100, "failures": 5, "avg_duration_ms": 250}}
    
    -- Performance
    avg_execution_time_ms INTEGER,
    p95_execution_time_ms INTEGER,
    
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    
    UNIQUE(automation_id, date)
);

CREATE INDEX IF NOT EXISTS idx_automation_analytics_automation_id ON automation_analytics(automation_id);
CREATE INDEX IF NOT EXISTS idx_automation_analytics_date ON automation_analytics(date);
```

### Automation Audit Log Table

Tracks all changes to automations for compliance and debugging.

```sql
CREATE TABLE IF NOT EXISTS automation_audit_log (
    id VARCHAR(36) PRIMARY KEY,
    automation_id VARCHAR(36) NOT NULL,
    user_id VARCHAR(36) NOT NULL,
    action VARCHAR(50) NOT NULL, -- created, updated, activated, paused, deleted, version_rollback
    changes JSONB, -- What changed (for updates)
    version_before INTEGER,
    version_after INTEGER,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_automation_audit_log_automation_id ON automation_audit_log(automation_id);
CREATE INDEX IF NOT EXISTS idx_automation_audit_log_user_id ON automation_audit_log(user_id);
CREATE INDEX IF NOT EXISTS idx_automation_audit_log_created_at ON automation_audit_log(created_at);
```

### Migration File

**File**: `internal/migrations/v7.go` (or v8.go based on current version)

```go
package migrations

import (
    "context"
    "github.com/Notifuse/notifuse/config"
    "github.com/Notifuse/notifuse/internal/domain"
)

type V7Migration struct{}

func (m *V7Migration) GetMajorVersion() float64 { return 7.0 }
func (m *V7Migration) HasSystemUpdate() bool { return false }
func (m *V7Migration) HasWorkspaceUpdate() bool { return true }

func (m *V7Migration) UpdateSystem(ctx context.Context, config *config.Config, db DBExecutor) error {
    return nil
}

func (m *V7Migration) UpdateWorkspace(ctx context.Context, config *config.Config, workspace *domain.Workspace, db DBExecutor) error {
    // Create automations table
    _, err := db.ExecContext(ctx, `
        CREATE TABLE IF NOT EXISTS automations (
            id VARCHAR(36) PRIMARY KEY,
            name VARCHAR(255) NOT NULL,
            description TEXT,
            status VARCHAR(20) NOT NULL DEFAULT 'draft',
            flow_definition JSONB NOT NULL,
            trigger_type VARCHAR(50) NOT NULL,
            trigger_config JSONB,
            entry_rules JSONB DEFAULT '{"allow_multiple": false, "cooldown_hours": 24}'::jsonb,
            test_mode BOOLEAN DEFAULT false,
            version INTEGER DEFAULT 1,
            previous_version JSONB,
            paused_at TIMESTAMP WITH TIME ZONE,
            pause_warning_sent BOOLEAN DEFAULT false,
            executions_count INTEGER DEFAULT 0,
            successes_count INTEGER DEFAULT 0,
            failures_count INTEGER DEFAULT 0,
            created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
            updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
            last_executed_at TIMESTAMP WITH TIME ZONE,
            created_by VARCHAR(36),
            updated_by VARCHAR(36)
        )
    `)
    if err != nil {
        return err
    }

    // Create indexes for automations
    _, err = db.ExecContext(ctx, `
        CREATE INDEX IF NOT EXISTS idx_automations_status ON automations(status);
        CREATE INDEX IF NOT EXISTS idx_automations_trigger_type ON automations(trigger_type);
        CREATE INDEX IF NOT EXISTS idx_automations_test_mode ON automations(test_mode);
    `)
    if err != nil {
        return err
    }

    // Create automation_executions table
    _, err = db.ExecContext(ctx, `
        CREATE TABLE IF NOT EXISTS automation_executions (
            id VARCHAR(36) PRIMARY KEY,
            automation_id VARCHAR(36) NOT NULL REFERENCES automations(id) ON DELETE CASCADE,
            contact_email VARCHAR(255) NOT NULL,
            status VARCHAR(20) NOT NULL DEFAULT 'pending',
            current_node_id VARCHAR(36),
            execution_path JSONB,
            execution_context JSONB,
            completed_at_node_id VARCHAR(36),
            completion_reason VARCHAR(50),
            is_test_run BOOLEAN DEFAULT false,
            scheduled_at TIMESTAMP WITH TIME ZONE,
            started_at TIMESTAMP WITH TIME ZONE,
            completed_at TIMESTAMP WITH TIME ZONE,
            waiting_for_event_type VARCHAR(50),
            wait_timeout_at TIMESTAMP WITH TIME ZONE,
            error_message TEXT,
            retry_count INTEGER DEFAULT 0,
            created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
            updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
        )
    `)
    if err != nil {
        return err
    }

    // Create indexes for automation_executions
    _, err = db.ExecContext(ctx, `
        CREATE INDEX IF NOT EXISTS idx_automation_executions_automation_id ON automation_executions(automation_id);
        CREATE INDEX IF NOT EXISTS idx_automation_executions_contact_email ON automation_executions(contact_email);
        CREATE INDEX IF NOT EXISTS idx_automation_executions_status ON automation_executions(status);
        CREATE INDEX IF NOT EXISTS idx_automation_executions_scheduled_at ON automation_executions(scheduled_at);
        CREATE INDEX IF NOT EXISTS idx_automation_executions_is_test_run ON automation_executions(is_test_run);
        CREATE INDEX IF NOT EXISTS idx_automation_executions_waiting_for_event ON automation_executions(waiting_for_event_type) WHERE waiting_for_event_type IS NOT NULL;
        CREATE UNIQUE INDEX IF NOT EXISTS idx_automation_executions_unique_entry ON automation_executions(automation_id, contact_email) WHERE status IN ('pending', 'running', 'waiting_for_event');
    `)
    if err != nil {
        return err
    }

    // Create automation_analytics table
    _, err = db.ExecContext(ctx, `
        CREATE TABLE IF NOT EXISTS automation_analytics (
            id VARCHAR(36) PRIMARY KEY,
            automation_id VARCHAR(36) NOT NULL REFERENCES automations(id) ON DELETE CASCADE,
            date DATE NOT NULL,
            triggered_count INTEGER DEFAULT 0,
            started_count INTEGER DEFAULT 0,
            completed_count INTEGER DEFAULT 0,
            failed_count INTEGER DEFAULT 0,
            node_stats JSONB DEFAULT '{}'::jsonb,
            avg_execution_time_ms INTEGER,
            p95_execution_time_ms INTEGER,
            created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
            updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
            UNIQUE(automation_id, date)
        )
    `)
    if err != nil {
        return err
    }

    // Create indexes for automation_analytics
    _, err = db.ExecContext(ctx, `
        CREATE INDEX IF NOT EXISTS idx_automation_analytics_automation_id ON automation_analytics(automation_id);
        CREATE INDEX IF NOT EXISTS idx_automation_analytics_date ON automation_analytics(date);
    `)
    if err != nil {
        return err
    }

    // Create automation_audit_log table
    _, err = db.ExecContext(ctx, `
        CREATE TABLE IF NOT EXISTS automation_audit_log (
            id VARCHAR(36) PRIMARY KEY,
            automation_id VARCHAR(36) NOT NULL,
            user_id VARCHAR(36) NOT NULL,
            action VARCHAR(50) NOT NULL,
            changes JSONB,
            version_before INTEGER,
            version_after INTEGER,
            created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
        )
    `)
    if err != nil {
        return err
    }

    // Create indexes for automation_audit_log
    _, err = db.ExecContext(ctx, `
        CREATE INDEX IF NOT EXISTS idx_automation_audit_log_automation_id ON automation_audit_log(automation_id);
        CREATE INDEX IF NOT EXISTS idx_automation_audit_log_user_id ON automation_audit_log(user_id);
        CREATE INDEX IF NOT EXISTS idx_automation_audit_log_created_at ON automation_audit_log(created_at);
    `)
    return err
}

func init() {
    Register(&V7Migration{})
}
```

## Backend Domain Layer

### Domain Models

**File**: `internal/domain/automation.go` (create new)

```go
package domain

import (
    "context"
    "database/sql/driver"
    "encoding/json"
    "errors"
    "fmt"
    "time"
)

//go:generate mockgen -destination mocks/mock_automation_service.go -package mocks github.com/Notifuse/notifuse/internal/domain AutomationService
//go:generate mockgen -destination mocks/mock_automation_repository.go -package mocks github.com/Notifuse/notifuse/internal/domain AutomationRepository

// AutomationStatus represents the status of an automation
type AutomationStatus string

const (
    AutomationStatusDraft  AutomationStatus = "draft"
    AutomationStatusActive AutomationStatus = "active"
    AutomationStatusPaused AutomationStatus = "paused"
)

// TriggerType defines the event that triggers an automation
type TriggerType string

const (
    TriggerContactCreated      TriggerType = "contact_created"
    TriggerContactUpdated      TriggerType = "contact_updated"
    TriggerListSubscribed      TriggerType = "list_subscribed"
    TriggerListUnsubscribed    TriggerType = "list_unsubscribed"
    TriggerCustomEvent         TriggerType = "custom_event"
)

// FlowDefinition represents the ReactFlow structure
type FlowDefinition struct {
    Nodes []FlowNode `json:"nodes"`
    Edges []FlowEdge `json:"edges"`
}

// Value implements driver.Valuer for database storage
func (f FlowDefinition) Value() (driver.Value, error) {
    return json.Marshal(f)
}

// Scan implements sql.Scanner for database retrieval
func (f *FlowDefinition) Scan(value interface{}) error {
    if value == nil {
        return nil
    }
    b, ok := value.([]byte)
    if !ok {
        return errors.New("type assertion to []byte failed")
    }
    return json.Unmarshal(b, f)
}

// FlowNode represents a node in the automation flow
type FlowNode struct {
    ID       string                 `json:"id"`
    Type     string                 `json:"type"` // trigger, delay, split_test, send_email, update_property, update_list_status, webhook, condition, wait_for_event, parallel_condition
    Position NodePosition           `json:"position"`
    Data     map[string]interface{} `json:"data"`
}

// NodePosition represents the visual position of a node
type NodePosition struct {
    X float64 `json:"x"`
    Y float64 `json:"y"`
}

// FlowEdge represents a connection between nodes
type FlowEdge struct {
    ID     string `json:"id"`
    Source string `json:"source"`
    Target string `json:"target"`
    Label  string `json:"label,omitempty"` // For split test A/B labeling or condition outcomes
}

// TriggerConfig contains trigger-specific configuration
type TriggerConfig struct {
    ListID          *string                `json:"list_id,omitempty"`          // For list triggers (list_subscribed/unsubscribed)
    EventName       *string                `json:"event_name,omitempty"`       // For custom event triggers
    Conditions      []TriggerCondition     `json:"conditions,omitempty"`       // Additional filtering conditions
    
    // Audience filtering - restrict trigger to specific contacts
    SegmentIDs      []string               `json:"segment_ids,omitempty"`      // Only trigger for contacts in these segments (ANY match)
    SubscribedLists []string               `json:"subscribed_lists,omitempty"` // Only trigger for contacts subscribed to these lists (ANY match)
    
    Metadata        map[string]interface{} `json:"metadata,omitempty"`
}

// Value implements driver.Valuer for database storage
func (t TriggerConfig) Value() (driver.Value, error) {
    return json.Marshal(t)
}

// Scan implements sql.Scanner for database retrieval
func (t *TriggerConfig) Scan(value interface{}) error {
    if value == nil {
        return nil
    }
    b, ok := value.([]byte)
    if !ok {
        return errors.New("type assertion to []byte failed")
    }
    return json.Unmarshal(b, t)
}

// TriggerCondition represents a condition that must be met for the trigger
type TriggerCondition struct {
    Field    string      `json:"field"`
    Operator string      `json:"operator"` // equals, not_equals, contains, gt, lt, gte, lte, is_null, is_not_null
    Value    interface{} `json:"value"`
}

// EntryRules controls how contacts can enter an automation
type EntryRules struct {
    AllowMultiple  bool `json:"allow_multiple"`   // Can a contact be in this automation multiple times?
    CooldownHours  int  `json:"cooldown_hours"`   // Minimum hours between entries (if AllowMultiple is true)
}

// Value implements driver.Valuer for database storage
func (e EntryRules) Value() (driver.Value, error) {
    return json.Marshal(e)
}

// Scan implements sql.Scanner for database retrieval
func (e *EntryRules) Scan(value interface{}) error {
    if value == nil {
        *e = EntryRules{AllowMultiple: false, CooldownHours: 24}
        return nil
    }
    b, ok := value.([]byte)
    if !ok {
        return errors.New("type assertion to []byte failed")
    }
    return json.Unmarshal(b, e)
}

// Automation represents an automation workflow
type Automation struct {
    ID              string           `json:"id"`
    Name            string           `json:"name"`
    Description     *string          `json:"description,omitempty"`
    Status          AutomationStatus `json:"status"`
    FlowDefinition  FlowDefinition   `json:"flow_definition"`
    TriggerType     TriggerType      `json:"trigger_type"`
    TriggerConfig   *TriggerConfig   `json:"trigger_config,omitempty"`
    EntryRules      EntryRules       `json:"entry_rules"`
    TestMode        bool             `json:"test_mode"`
    Version         int              `json:"version"`
    PreviousVersion *FlowDefinition  `json:"previous_version,omitempty"`
    
    // Pause management (from Loops.co insight)
    PausedAt          *time.Time `json:"paused_at,omitempty"`
    PauseWarningSent  bool       `json:"pause_warning_sent"`
    
    // Statistics
    ExecutionsCount int `json:"executions_count"`
    SuccessesCount  int `json:"successes_count"`
    FailuresCount   int `json:"failures_count"`
    
    // Timestamps
    CreatedAt       time.Time  `json:"created_at"`
    UpdatedAt       time.Time  `json:"updated_at"`
    LastExecutedAt  *time.Time `json:"last_executed_at,omitempty"`
    CreatedBy       *string    `json:"created_by,omitempty"`
    UpdatedBy       *string    `json:"updated_by,omitempty"`
}

// IsPausedTooLong checks if automation has been paused for more than 24 hours
func (a *Automation) IsPausedTooLong() bool {
    if a.Status != AutomationStatusPaused || a.PausedAt == nil {
        return false
    }
    return time.Since(*a.PausedAt) > 24*time.Hour
}

// Validate ensures the automation has all required fields
func (a *Automation) Validate() error {
    if a.Name == "" {
        return fmt.Errorf("name is required")
    }
    if a.TriggerType == "" {
        return fmt.Errorf("trigger_type is required")
    }
    if len(a.FlowDefinition.Nodes) == 0 {
        return fmt.Errorf("flow must have at least one node")
    }
    
    // Use flow validator
    validator := NewFlowValidator()
    validationErrors := validator.Validate(a.FlowDefinition)
    if len(validationErrors) > 0 {
        // Return first error for simplicity
        return fmt.Errorf("flow validation failed: %s", validationErrors[0].Message)
    }
    
    return nil
}

// ExecutionContext contains all data available during automation execution
type ExecutionContext struct {
    Contact       *Contact               `json:"contact"`         // Current contact state
    TriggerData   map[string]interface{} `json:"trigger_data"`    // Data from trigger event
    NodeResults   map[string]interface{} `json:"node_results"`    // Results from each node (key: node_id)
    Variables     map[string]interface{} `json:"variables"`       // User-defined variables
    ExecutionMeta map[string]interface{} `json:"execution_meta"`  // Metadata (start time, etc.)
}

// Value implements driver.Valuer for database storage
func (e ExecutionContext) Value() (driver.Value, error) {
    return json.Marshal(e)
}

// Scan implements sql.Scanner for database retrieval
func (e *ExecutionContext) Scan(value interface{}) error {
    if value == nil {
        *e = ExecutionContext{
            NodeResults:   make(map[string]interface{}),
            Variables:     make(map[string]interface{}),
            TriggerData:   make(map[string]interface{}),
            ExecutionMeta: make(map[string]interface{}),
        }
        return nil
    }
    b, ok := value.([]byte)
    if !ok {
        return errors.New("type assertion to []byte failed")
    }
    return json.Unmarshal(b, e)
}

// AutomationExecution represents a single execution of an automation for a contact
type AutomationExecution struct {
    ID               string           `json:"id"`
    AutomationID     string           `json:"automation_id"`
    ContactEmail     string           `json:"contact_email"`
    Status           string           `json:"status"` // pending, running, completed, failed, cancelled, waiting_for_event
    CurrentNodeID    *string          `json:"current_node_id,omitempty"`
    ExecutionPath    []string         `json:"execution_path,omitempty"`
    ExecutionContext ExecutionContext `json:"execution_context"`
    IsTestRun        bool             `json:"is_test_run"`
    
    // Completion tracking (no exit node needed - track where flow naturally ended)
    CompletedAtNodeID *string `json:"completed_at_node_id,omitempty"` // Last node executed before completion
    CompletionReason  *string `json:"completion_reason,omitempty"`    // "natural_end", "condition_branch", "timeout", "error"
    
    // Scheduling
    ScheduledAt *time.Time `json:"scheduled_at,omitempty"`
    StartedAt   *time.Time `json:"started_at,omitempty"`
    CompletedAt *time.Time `json:"completed_at,omitempty"`
    
    // Wait for event
    WaitingForEventType *string    `json:"waiting_for_event_type,omitempty"`
    WaitTimeoutAt       *time.Time `json:"wait_timeout_at,omitempty"`
    
    // Error tracking
    ErrorMessage *string `json:"error_message,omitempty"`
    RetryCount   int     `json:"retry_count"`
    
    // Timestamps
    CreatedAt time.Time `json:"created_at"`
    UpdatedAt time.Time `json:"updated_at"`
}

// AutomationAnalytics represents daily analytics for an automation
type AutomationAnalytics struct {
    ID           string                 `json:"id"`
    AutomationID string                 `json:"automation_id"`
    Date         time.Time              `json:"date"`
    
    TriggeredCount int `json:"triggered_count"`
    StartedCount   int `json:"started_count"`
    CompletedCount int `json:"completed_count"`
    FailedCount    int `json:"failed_count"`
    
    NodeStats            map[string]NodeStats `json:"node_stats"`
    AvgExecutionTimeMs   int                  `json:"avg_execution_time_ms"`
    P95ExecutionTimeMs   int                  `json:"p95_execution_time_ms"`
    
    CreatedAt time.Time `json:"created_at"`
    UpdatedAt time.Time `json:"updated_at"`
}

// NodeStats contains metrics for a specific node
type NodeStats struct {
    Executions      int `json:"executions"`
    Failures        int `json:"failures"`
    AvgDurationMs   int `json:"avg_duration_ms"`
}

// AutomationAuditLog represents a change to an automation
type AutomationAuditLog struct {
    ID            string                 `json:"id"`
    AutomationID  string                 `json:"automation_id"`
    UserID        string                 `json:"user_id"`
    Action        string                 `json:"action"` // created, updated, activated, paused, deleted, version_rollback
    Changes       map[string]interface{} `json:"changes,omitempty"`
    VersionBefore *int                   `json:"version_before,omitempty"`
    VersionAfter  *int                   `json:"version_after,omitempty"`
    CreatedAt     time.Time              `json:"created_at"`
}

// AutomationService defines the business logic for automations
type AutomationService interface {
    // CRUD operations
    Create(ctx context.Context, workspaceID string, automation *Automation, userID string) error
    Get(ctx context.Context, workspaceID, id string) (*Automation, error)
    Update(ctx context.Context, workspaceID string, automation *Automation, userID string) error
    Delete(ctx context.Context, workspaceID, id string, userID string) error
    List(ctx context.Context, workspaceID string, filter AutomationFilter) (*AutomationListResponse, error)
    
    // Activation
    Activate(ctx context.Context, workspaceID, id string, userID string) error
    Pause(ctx context.Context, workspaceID, id string, userID string) error
    
    // Pause management (from Loops.co insight)
    CheckPauseTimeout(ctx context.Context, workspaceID, id string) error
    SendPauseWarning(ctx context.Context, workspaceID, id string) error
    
    // Version control
    RollbackVersion(ctx context.Context, workspaceID, id string, userID string) error
    
    // Testing
    TestRun(ctx context.Context, workspaceID, automationID, contactEmail string) (*TestRunResult, error)
    
    // Execution management
    TriggerExecution(ctx context.Context, workspaceID, automationID, contactEmail string, eventData map[string]interface{}) error
    CheckTriggerAudience(ctx context.Context, workspaceID string, triggerConfig *TriggerConfig, contactEmail string) (bool, error)
    GetExecution(ctx context.Context, workspaceID, executionID string) (*AutomationExecution, error)
    ListExecutions(ctx context.Context, workspaceID, automationID string, filter ExecutionFilter) (*ExecutionListResponse, error)
    
    // Analytics
    GetAnalytics(ctx context.Context, workspaceID, automationID string, startDate, endDate time.Time) ([]*AutomationAnalytics, error)
    
    // Audit log
    GetAuditLog(ctx context.Context, workspaceID, automationID string, limit, offset int) ([]*AutomationAuditLog, int, error)
    
    // Event bus integration
    SubscribeToContactEvents(eventBus EventBus)
}

// TestRunResult contains the result of a test automation execution
type TestRunResult struct {
    ExecutionID   string                 `json:"execution_id"`
    Success       bool                   `json:"success"`
    NodesExecuted []string               `json:"nodes_executed"`
    NodeResults   map[string]interface{} `json:"node_results"`
    Logs          []string               `json:"logs"`
    Error         *string                `json:"error,omitempty"`
}

// AutomationRepository defines data access for automations
type AutomationRepository interface {
    // Automation CRUD
    Create(ctx context.Context, workspaceID string, automation *Automation) error
    Get(ctx context.Context, workspaceID, id string) (*Automation, error)
    Update(ctx context.Context, workspaceID string, automation *Automation) error
    Delete(ctx context.Context, workspaceID, id string) error
    List(ctx context.Context, workspaceID string, filter AutomationFilter) ([]*Automation, int, error)
    
    // Execution CRUD
    CreateExecution(ctx context.Context, workspaceID string, execution *AutomationExecution) error
    GetExecution(ctx context.Context, workspaceID, id string) (*AutomationExecution, error)
    UpdateExecution(ctx context.Context, workspaceID string, execution *AutomationExecution) error
    ListExecutions(ctx context.Context, workspaceID string, filter ExecutionFilter) ([]*AutomationExecution, int, error)
    GetExecutionsByContact(ctx context.Context, workspaceID, contactEmail string) ([]*AutomationExecution, error)
    
    // Batch queries
    GetActiveAutomationsByTrigger(ctx context.Context, workspaceID string, triggerType TriggerType) ([]*Automation, error)
    GetPendingExecutions(ctx context.Context, workspaceID string, limit int) ([]*AutomationExecution, error)
    GetExecutionsWaitingForEvent(ctx context.Context, workspaceID string, eventType string) ([]*AutomationExecution, error)
    
    // Statistics
    IncrementExecutionCount(ctx context.Context, workspaceID, automationID string) error
    IncrementSuccessCount(ctx context.Context, workspaceID, automationID string) error
    IncrementFailureCount(ctx context.Context, workspaceID, automationID string) error
    
    // Analytics
    UpsertAnalytics(ctx context.Context, workspaceID string, analytics *AutomationAnalytics) error
    GetAnalytics(ctx context.Context, workspaceID, automationID string, startDate, endDate time.Time) ([]*AutomationAnalytics, error)
    
    // Audit log
    CreateAuditLog(ctx context.Context, workspaceID string, log *AutomationAuditLog) error
    GetAuditLog(ctx context.Context, workspaceID, automationID string, limit, offset int) ([]*AutomationAuditLog, int, error)
    
    // Entry deduplication check
    CanContactEnter(ctx context.Context, workspaceID, automationID, contactEmail string, entryRules EntryRules) (bool, error)
    
    // Audience filtering checks
    IsContactInSegments(ctx context.Context, workspaceID, contactEmail string, segmentIDs []string) (bool, error)
    IsContactSubscribedToLists(ctx context.Context, workspaceID, contactEmail string, listIDs []string) (bool, error)
}

// AutomationFilter for listing automations
type AutomationFilter struct {
    Status      []AutomationStatus
    TriggerType []TriggerType
    TestMode    *bool
    Limit       int
    Offset      int
}

// ExecutionFilter for listing executions
type ExecutionFilter struct {
    AutomationID string
    ContactEmail string
    Status       []string
    IsTestRun    *bool
    Limit        int
    Offset       int
}

// Response types
type AutomationListResponse struct {
    Automations []*Automation `json:"automations"`
    TotalCount  int           `json:"total_count"`
    Limit       int           `json:"limit"`
    Offset      int           `json:"offset"`
    HasMore     bool          `json:"has_more"`
}

type ExecutionListResponse struct {
    Executions []*AutomationExecution `json:"executions"`
    TotalCount int                    `json:"total_count"`
    Limit      int                    `json:"limit"`
    Offset     int                    `json:"offset"`
    HasMore    bool                   `json:"has_more"`
}

// Request types for HTTP handlers
type CreateAutomationRequest struct {
    Name           string         `json:"name"`
    Description    *string        `json:"description,omitempty"`
    FlowDefinition FlowDefinition `json:"flow_definition"`
    TriggerType    TriggerType    `json:"trigger_type"`
    TriggerConfig  *TriggerConfig `json:"trigger_config,omitempty"`
    EntryRules     *EntryRules    `json:"entry_rules,omitempty"`
    TestMode       *bool          `json:"test_mode,omitempty"`
}

func (r *CreateAutomationRequest) Validate() (*Automation, error) {
    entryRules := EntryRules{AllowMultiple: false, CooldownHours: 24}
    if r.EntryRules != nil {
        entryRules = *r.EntryRules
    }
    
    testMode := false
    if r.TestMode != nil {
        testMode = *r.TestMode
    }
    
    automation := &Automation{
        Name:           r.Name,
        Description:    r.Description,
        Status:         AutomationStatusDraft,
        FlowDefinition: r.FlowDefinition,
        TriggerType:    r.TriggerType,
        TriggerConfig:  r.TriggerConfig,
        EntryRules:     entryRules,
        TestMode:       testMode,
        Version:        1,
        CreatedAt:      time.Now().UTC(),
        UpdatedAt:      time.Now().UTC(),
    }
    return automation, automation.Validate()
}

type UpdateAutomationRequest struct {
    Name           *string         `json:"name,omitempty"`
    Description    *string         `json:"description,omitempty"`
    FlowDefinition *FlowDefinition `json:"flow_definition,omitempty"`
    TriggerType    *TriggerType    `json:"trigger_type,omitempty"`
    TriggerConfig  *TriggerConfig  `json:"trigger_config,omitempty"`
    EntryRules     *EntryRules     `json:"entry_rules,omitempty"`
    TestMode       *bool           `json:"test_mode,omitempty"`
}
```

### Flow Validation

**File**: `internal/domain/automation_validator.go` (create new)

```go
package domain

import "fmt"

// ValidationError represents a flow validation error
type ValidationError struct {
    Field   string `json:"field"`
    Message string `json:"message"`
}

// FlowValidator validates automation flows
type FlowValidator struct{}

// NewFlowValidator creates a new flow validator
func NewFlowValidator() *FlowValidator {
    return &FlowValidator{}
}

// Validate checks a flow for common errors
func (v *FlowValidator) Validate(flow FlowDefinition) []ValidationError {
    var errors []ValidationError
    
    // Build node and edge maps for quick lookup
    nodeMap := make(map[string]FlowNode)
    for _, node := range flow.Nodes {
        nodeMap[node.ID] = node
    }
    
    // Count trigger nodes
    triggerCount := 0
    for _, node := range flow.Nodes {
        if node.Type == "trigger" {
            triggerCount++
        }
    }
    
    if triggerCount == 0 {
        errors = append(errors, ValidationError{
            Field:   "nodes",
            Message: "Flow must have exactly one trigger node",
        })
    } else if triggerCount > 1 {
        errors = append(errors, ValidationError{
            Field:   "nodes",
            Message: fmt.Sprintf("Flow has %d trigger nodes, must have exactly one", triggerCount),
        })
    }
    
    // Check for orphaned nodes (nodes not reachable from trigger)
    reachableNodes := v.getReachableNodes(flow, nodeMap)
    for _, node := range flow.Nodes {
        if node.Type != "trigger" && !reachableNodes[node.ID] {
            errors = append(errors, ValidationError{
                Field:   "nodes",
                Message: fmt.Sprintf("Node '%s' is not connected to the flow", node.ID),
            })
        }
    }
    
    // Check for circular dependencies
    if v.hasCycles(flow, nodeMap) {
        errors = append(errors, ValidationError{
            Field:   "edges",
            Message: "Flow contains circular dependencies (infinite loop)",
        })
    }
    
    // Validate edge connections
    for _, edge := range flow.Edges {
        sourceNode, sourceExists := nodeMap[edge.Source]
        targetNode, targetExists := nodeMap[edge.Target]
        
        if !sourceExists {
            errors = append(errors, ValidationError{
                Field:   "edges",
                Message: fmt.Sprintf("Edge source node '%s' does not exist", edge.Source),
            })
            continue
        }
        
        if !targetExists {
            errors = append(errors, ValidationError{
                Field:   "edges",
                Message: fmt.Sprintf("Edge target node '%s' does not exist", edge.Target),
            })
            continue
        }
        
        // Trigger cannot be a target
        if targetNode.Type == "trigger" {
            errors = append(errors, ValidationError{
                Field:   "edges",
                Message: "Trigger node cannot be a target of an edge",
            })
        }
    }
    
    // Validate split test nodes have exactly 2 outgoing edges
    edgesBySource := make(map[string]int)
    for _, edge := range flow.Edges {
        edgesBySource[edge.Source]++
    }
    
    for _, node := range flow.Nodes {
        if node.Type == "split_test" {
            outgoingCount := edgesBySource[node.ID]
            if outgoingCount != 2 {
                errors = append(errors, ValidationError{
                    Field:   "nodes",
                    Message: fmt.Sprintf("Split test node '%s' must have exactly 2 outgoing edges, has %d", node.ID, outgoingCount),
                })
            }
        }
        
        // Condition nodes should have at least 2 outgoing edges
        if node.Type == "condition" {
            outgoingCount := edgesBySource[node.ID]
            if outgoingCount < 2 {
                errors = append(errors, ValidationError{
                    Field:   "nodes",
                    Message: fmt.Sprintf("Condition node '%s' should have at least 2 outgoing edges (true/false paths)", node.ID),
                })
            }
        }
        
        // Note: Removed exit node - automations complete naturally when no more edges
    }
    
    return errors
}

// getReachableNodes returns a set of node IDs reachable from the trigger
func (v *FlowValidator) getReachableNodes(flow FlowDefinition, nodeMap map[string]FlowNode) map[string]bool {
    reachable := make(map[string]bool)
    
    // Find trigger node
    var triggerID string
    for _, node := range flow.Nodes {
        if node.Type == "trigger" {
            triggerID = node.ID
            break
        }
    }
    
    if triggerID == "" {
        return reachable
    }
    
    // Build adjacency list
    adjacency := make(map[string][]string)
    for _, edge := range flow.Edges {
        adjacency[edge.Source] = append(adjacency[edge.Source], edge.Target)
    }
    
    // DFS to find reachable nodes
    var dfs func(nodeID string)
    dfs = func(nodeID string) {
        if reachable[nodeID] {
            return
        }
        reachable[nodeID] = true
        for _, neighbor := range adjacency[nodeID] {
            dfs(neighbor)
        }
    }
    
    dfs(triggerID)
    return reachable
}

// hasCycles checks if the flow contains cycles
func (v *FlowValidator) hasCycles(flow FlowDefinition, nodeMap map[string]FlowNode) bool {
    // Build adjacency list
    adjacency := make(map[string][]string)
    for _, edge := range flow.Edges {
        adjacency[edge.Source] = append(adjacency[edge.Source], edge.Target)
    }
    
    visited := make(map[string]bool)
    recStack := make(map[string]bool)
    
    var hasCycle func(nodeID string) bool
    hasCycle = func(nodeID string) bool {
        visited[nodeID] = true
        recStack[nodeID] = true
        
        for _, neighbor := range adjacency[nodeID] {
            if !visited[neighbor] {
                if hasCycle(neighbor) {
                    return true
                }
            } else if recStack[neighbor] {
                return true
            }
        }
        
        recStack[nodeID] = false
        return false
    }
    
    for _, node := range flow.Nodes {
        if !visited[node.ID] {
            if hasCycle(node.ID) {
                return true
            }
        }
    }
    
    return false
}
```

### Event Integration

**File**: `internal/domain/event.go` (modify existing)

```go
// Add to existing EventType constants
const (
    // ... existing events ...
    
    // Contact events for automation triggers
    EventContactCreated            EventType = "contact.created"
    EventContactUpdated            EventType = "contact.updated"
    EventContactListSubscribed     EventType = "contact.list_subscribed"
    EventContactListUnsubscribed   EventType = "contact.list_unsubscribed"
    EventContactCustomEvent        EventType = "contact.custom_event"
)
```

### Permissions Integration

**File**: `internal/domain/workspace.go` (modify existing)

```go
// Add to PermissionResource constants
const (
    // ... existing resources ...
    PermissionResourceAutomations PermissionResource = "automations"
)

// Update FullPermissions map
var FullPermissions = UserPermissions{
    // ... existing permissions ...
    PermissionResourceAutomations: ResourcePermissions{Read: true, Write: true},
}
```

## Backend Repository Layer

**File**: `internal/repository/automation_postgres.go` (create new)

Implements `AutomationRepository` interface with PostgreSQL queries using Squirrel query builder.

Key methods:
- `Create`, `Get`, `Update`, `Delete`, `List` for automations
- `CreateExecution`, `GetExecution`, `UpdateExecution`, `ListExecutions` for executions
- `GetActiveAutomationsByTrigger` - finds active automations for a trigger type
- `GetPendingExecutions` - gets executions scheduled to run now
- `GetExecutionsWaitingForEvent` - gets executions waiting for specific event
- `CanContactEnter` - checks entry rules and deduplication
- `IsContactInSegments` - checks if contact is in any of the specified segments (queries contact_segments table)
- `IsContactSubscribedToLists` - checks if contact is subscribed to any of the specified lists (queries contact_lists table with status='subscribed')
- Statistics increment methods
- Analytics CRUD
- Audit log CRUD

**Audience Filtering Queries**:

```go
// IsContactInSegments checks if contact is in any of the segments
func (r *AutomationPostgresRepository) IsContactInSegments(ctx context.Context, workspaceID, contactEmail string, segmentIDs []string) (bool, error) {
    if len(segmentIDs) == 0 {
        return false, nil
    }
    
    query := squirrel.Select("COUNT(*)").
        From("contact_segments").
        Where(squirrel.Eq{
            "contact_email": contactEmail,
            "segment_id":    segmentIDs,
        }).
        Limit(1)
    
    var count int
    err := query.RunWith(db).QueryRowContext(ctx).Scan(&count)
    return count > 0, err
}

// IsContactSubscribedToLists checks if contact is subscribed to any of the lists
func (r *AutomationPostgresRepository) IsContactSubscribedToLists(ctx context.Context, workspaceID, contactEmail string, listIDs []string) (bool, error) {
    if len(listIDs) == 0 {
        return false, nil
    }
    
    query := squirrel.Select("COUNT(*)").
        From("contact_lists").
        Where(squirrel.Eq{
            "contact_email": contactEmail,
            "list_id":       listIDs,
            "status":        "subscribed",
        }).
        Limit(1)
    
    var count int
    err := query.RunWith(db).QueryRowContext(ctx).Scan(&count)
    return count > 0, err
}
```

## Backend Service Layer

### Automation Service

**File**: `internal/service/automation_service.go` (create new)

Implements business logic and orchestration:

```go
type AutomationService struct {
    automationRepo   domain.AutomationRepository
    contactRepo      domain.ContactRepository
    segmentRepo      domain.SegmentRepository
    listService      domain.ListService
    transactionalSvc domain.TransactionalService
    taskService      domain.TaskService
    eventBus         domain.EventBus
    logger           logger.Logger
}

// Key methods:
// - Create, Get, Update, Delete, List (CRUD with audit logging)
// - Activate, Pause (status management with permissions check)
// - RollbackVersion (restore previous flow_definition)
// - TestRun (execute automation in test mode)
// - TriggerExecution (creates automation execution record with entry rules check and audience filtering)
// - CheckTriggerAudience (validates contact matches segment/list filters)
// - SubscribeToContactEvents (listens for trigger events and wait_for_event completions)
// - GetAnalytics (retrieve analytics data)
// - GetAuditLog (retrieve audit log)
```

**Audience Filtering Logic**:

When an event occurs that could trigger an automation:

1. Get all active automations for that trigger type
2. For each automation:
   - Check entry rules (deduplication)
   - Check trigger audience filters:
     - If `segment_ids` specified: Check if contact is in ANY of the segments
     - If `subscribed_lists` specified: Check if contact is subscribed to ANY of the lists
     - If both specified: Contact must match BOTH conditions (segments AND lists)
     - If neither specified: All contacts match
   - If all checks pass, create execution

```go
func (s *AutomationService) CheckTriggerAudience(ctx context.Context, workspaceID string, triggerConfig *TriggerConfig, contactEmail string) (bool, error) {
    // If no audience filters, all contacts match
    if len(triggerConfig.SegmentIDs) == 0 && len(triggerConfig.SubscribedLists) == 0 {
        return true, nil
    }
    
    // Check segment membership
    if len(triggerConfig.SegmentIDs) > 0 {
        inSegment, err := s.automationRepo.IsContactInSegments(ctx, workspaceID, contactEmail, triggerConfig.SegmentIDs)
        if err != nil {
            return false, err
        }
        if !inSegment {
            return false, nil
        }
    }
    
    // Check list subscription
    if len(triggerConfig.SubscribedLists) > 0 {
        subscribed, err := s.automationRepo.IsContactSubscribedToLists(ctx, workspaceID, contactEmail, triggerConfig.SubscribedLists)
        if err != nil {
            return false, err
        }
        if !subscribed {
            return false, nil
        }
    }
    
    return true, nil
}
```

### Automation Execution Processor

**File**: `internal/service/automation_execution_processor.go` (create new)

Processes individual automation executions. Similar to segment build processor.

```go
type AutomationExecutionProcessor struct {
    automationRepo   domain.AutomationRepository
    contactRepo      domain.ContactRepository
    transactionalSvc domain.TransactionalService
    listService      domain.ListService
    taskRepo         domain.TaskRepository
    logger           logger.Logger
}

// Implements TaskProcessor interface
func (p *AutomationExecutionProcessor) CanProcess(taskType string) bool {
    return taskType == "execute_automation"
}

func (p *AutomationExecutionProcessor) Process(ctx context.Context, task *domain.Task, timeoutAt time.Time) (bool, error) {
    // 1. Get execution from task state
    // 2. Get automation definition
    // 3. Get contact
    // 4. Initialize execution context if first run
    // 5. Execute nodes sequentially following edges
    // 6. Handle delays by pausing task
    // 7. Handle wait_for_event by changing status
    // 8. Save execution state and context
    // 9. Track analytics (node-level and automation-level)
    // 10. Return completion status
}
```

Node execution logic:
- **Trigger**: Entry point (no action, set trigger data in context)
- **Delay**: Schedule next execution (pause task, respect timezone if configured)
- **Split Test**: Randomly choose A or B path (store choice in node results)
- **Condition**: Evaluate conditions against context, choose path
- **Send Transactional Email**: Call transactional service (store result)
- **Update Contact Property**: Call contact service (store result)
- **Update List Status**: Call list service (store result)
- **Webhook**: HTTP POST to configured URL (store response in variables)
- **Wait for Event**: Set status to waiting_for_event, store event type and timeout
- **Natural Completion**: When a node returns empty NextNodeIDs, execution completes (marks completed_at_node_id and completion_reason)

### Node Executors

**File**: `internal/service/automation_node_executors.go` (create new)

Contains execution logic for each node type:

```go
// NodeExecutor interface
type NodeExecutor interface {
    Execute(ctx context.Context, node domain.FlowNode, execution *domain.AutomationExecution, contact *domain.Contact) (*NodeExecutionResult, error)
}

// NodeExecutionResult contains the result of executing a node
type NodeExecutionResult struct {
    NextNodeIDs []string               // IDs of next nodes to execute (empty = natural completion)
    PauseUntil  *time.Time             // If set, pause execution until this time
    WaitForEvent *string               // If set, wait for this event type
    WaitTimeout  *time.Time            // Timeout for wait_for_event
    ResultData   map[string]interface{} // Data to store in node_results
    Variables    map[string]interface{} // Data to store in variables
}

// Implement executors:
// - TriggerExecutor
// - DelayExecutor (with timezone awareness)
// - SplitTestExecutor (enhanced with sample size from Loops.co insight)
// - ConditionExecutor (evaluate rules against context, with continuous monitoring)
// - ParallelConditionExecutor (NEW from Loops.co insight - returns multiple paths)
// - SendEmailExecutor
// - UpdatePropertyExecutor
// - UpdateListStatusExecutor
// - WebhookExecutor (HTTP client with retry)
// - WaitForEventExecutor

// Note: No ExitExecutor needed - automations complete naturally when NextNodeIDs is empty
```

**Enhanced Node Data Structures** (from Loops.co insights):

```go
// Enhanced SplitTestNodeData with sample size control
type SplitTestNodeData struct {
    SampleSize    int                `json:"sample_size"`     // 0-100 percentage
    Variants      []SplitTestVariant `json:"variants"`
    HasControl    bool               `json:"has_control"`
    ControlWeight int                `json:"control_weight"`  // If no control, remaining % exits
}

type SplitTestVariant struct {
    ID     string `json:"id"`
    Label  string `json:"label"`
    Weight int    `json:"weight"` // Percentage within sample
}

// Enhanced ConditionNodeData with continuous monitoring
type ConditionNodeData struct {
    Conditions      []ConditionRule `json:"conditions"`
    Logic           string          `json:"logic"` // "AND" or "OR"
    ContinuousCheck bool            `json:"continuous_check"` // Monitor downstream
    ApplyToNextOnly bool            `json:"apply_to_next_only"` // Only check at next node
}

// NEW: Parallel condition node (from Loops.co insight)
type ParallelConditionNodeData struct {
    Branches []ParallelBranch `json:"branches"`
    // Contact follows ALL matching branches, not just first match
}

type ParallelBranch struct {
    ID         string          `json:"id"`
    Label      string          `json:"label"`
    Conditions []ConditionRule `json:"conditions"`
    Logic      string          `json:"logic"` // "AND" or "OR"
}
```

### Automation Check Task Processor

**File**: `internal/service/automation_check_task_processor.go` (create new)

Evergreen task that runs periodically to check for pending automation executions. Similar to segment recompute checker.

```go
type AutomationCheckTaskProcessor struct {
    automationRepo domain.AutomationRepository
    taskService    domain.TaskService
    logger         logger.Logger
}

// Implements TaskProcessor interface
func (p *AutomationCheckTaskProcessor) CanProcess(taskType string) bool {
    return taskType == "check_automation_executions"
}

func (p *AutomationCheckTaskProcessor) Process(ctx context.Context, task *domain.Task, timeoutAt time.Time) (bool, error) {
    // 1. Get pending executions scheduled to run now
    // 2. For each execution, create execute_automation task
    // 3. Check for wait_for_event timeouts
    // 4. Mark task as pending (not complete) to keep it recurring
    return false, nil // false = not complete, reschedule
}

// Helper function to ensure task exists for workspace
func EnsureAutomationCheckTask(ctx context.Context, taskRepo domain.TaskRepository, workspaceID string) error {
    // Similar to EnsureSegmentRecomputeTask
}
```

### Pause Timeout Checker (NEW from Loops.co insight)

**File**: `internal/service/automation_pause_checker.go` (create new)

Background job to check for automations paused > 24 hours:

```go
type AutomationPauseChecker struct {
    automationRepo   domain.AutomationRepository
    notificationSvc  NotificationService // Email service
    logger           logger.Logger
}

// CheckPausedAutomations runs periodically (e.g., hourly)
func (c *AutomationPauseChecker) CheckPausedAutomations(ctx context.Context) error {
    // 1. Get all paused automations across all workspaces
    // 2. For each automation:
    //    - Check if paused > 24 hours
    //    - If yes and warning not sent:
    //      - Send warning email to owner
    //      - Set pause_warning_sent = true
    //    - If yes and > 48 hours:
    //      - Consider auto-stopping (optional)
    return nil
}
```

## Backend HTTP Layer

**File**: `internal/http/automation_handler.go` (create new)

API endpoints:

```go
// POST /api/automation.create
// GET  /api/automation.get?workspace_id=X&id=Y
// POST /api/automation.update
// POST /api/automation.delete
// GET  /api/automation.list?workspace_id=X
// POST /api/automation.activate
// POST /api/automation.pause
// POST /api/automation.rollback_version
// POST /api/automation.test_run

// GET  /api/automation.execution.get?workspace_id=X&id=Y
// GET  /api/automation.execution.list?workspace_id=X&automation_id=Y

// GET  /api/automation.analytics?workspace_id=X&automation_id=Y&start_date=...&end_date=...
// GET  /api/automation.audit_log?workspace_id=X&automation_id=Y

// POST /api/automation.validate_flow (validate flow without saving)
```

Each endpoint checks `PermissionResourceAutomations` permission before executing.

## Frontend Dependencies

Add ReactFlow to `console/package.json`:

```json
{
  "dependencies": {
    "reactflow": "^11.11.0",
    "@reactflow/core": "^11.11.0",
    "@reactflow/background": "^11.3.9",
    "@reactflow/controls": "^11.2.9",
    "@reactflow/minimap": "^11.7.9"
  }
}
```

## Frontend Type Definitions

**File**: `console/src/services/api/automations.ts` (create new)

```typescript
export interface FlowNode {
  id: string
  type: string // 'trigger' | 'delay' | 'split_test' | 'send_email' | 'update_property' | 'update_list_status' | 'webhook' | 'condition' | 'wait_for_event' | 'parallel_condition'
  position: { x: number; y: number }
  data: Record<string, any>
}

export interface FlowEdge {
  id: string
  source: string
  target: string
  label?: string
}

export interface FlowDefinition {
  nodes: FlowNode[]
  edges: FlowEdge[]
}

export interface TriggerConfig {
  list_id?: string
  event_name?: string
  conditions?: TriggerCondition[]
  segment_ids?: string[]         // Restrict to contacts in these segments
  subscribed_lists?: string[]    // Restrict to contacts subscribed to these lists
  metadata?: Record<string, any>
}

export interface TriggerCondition {
  field: string
  operator: string
  value: any
}

export interface EntryRules {
  allow_multiple: boolean
  cooldown_hours: number
}

export interface Automation {
  id: string
  name: string
  description?: string
  status: 'draft' | 'active' | 'paused'
  flow_definition: FlowDefinition
  trigger_type: string
  trigger_config?: TriggerConfig
  entry_rules: EntryRules
  test_mode: boolean
  version: number
  previous_version?: FlowDefinition
  paused_at?: string              // NEW from Loops.co insight
  pause_warning_sent: boolean     // NEW from Loops.co insight
  executions_count: number
  successes_count: number
  failures_count: number
  created_at: string
  updated_at: string
  last_executed_at?: string
  created_by?: string
  updated_by?: string
}

export interface AutomationExecution {
  id: string
  automation_id: string
  contact_email: string
  status: string
  current_node_id?: string
  execution_path?: string[]
  execution_context?: ExecutionContext
  is_test_run: boolean
  completed_at_node_id?: string  // Last node before natural completion
  completion_reason?: string     // How the automation ended
  scheduled_at?: string
  started_at?: string
  completed_at?: string
  waiting_for_event_type?: string
  wait_timeout_at?: string
  error_message?: string
  retry_count: number
  created_at: string
  updated_at: string
}

export interface ExecutionContext {
  contact?: any
  trigger_data?: Record<string, any>
  node_results?: Record<string, any>
  variables?: Record<string, any>
  execution_meta?: Record<string, any>
}

export interface AutomationAnalytics {
  id: string
  automation_id: string
  date: string
  triggered_count: number
  started_count: number
  completed_count: number
  failed_count: number
  node_stats: Record<string, NodeStats>
  avg_execution_time_ms: number
  p95_execution_time_ms: number
}

export interface NodeStats {
  executions: number
  failures: number
  avg_duration_ms: number
}

export interface AutomationAuditLog {
  id: string
  automation_id: string
  user_id: string
  action: string
  changes?: Record<string, any>
  version_before?: number
  version_after?: number
  created_at: string
}

export interface ValidationError {
  field: string
  message: string
}

export interface TestRunResult {
  execution_id: string
  success: boolean
  nodes_executed: string[]
  node_results: Record<string, any>
  logs: string[]
  error?: string
}

// API functions
export const createAutomation = async (workspaceId: string, data: CreateAutomationRequest): Promise<Automation>
export const getAutomation = async (workspaceId: string, id: string): Promise<Automation>
export const updateAutomation = async (workspaceId: string, id: string, data: UpdateAutomationRequest): Promise<Automation>
export const deleteAutomation = async (workspaceId: string, id: string): Promise<void>
export const listAutomations = async (workspaceId: string, params?: ListAutomationsParams): Promise<AutomationListResponse>
export const activateAutomation = async (workspaceId: string, id: string): Promise<Automation>
export const pauseAutomation = async (workspaceId: string, id: string): Promise<Automation>
export const rollbackVersion = async (workspaceId: string, id: string): Promise<Automation>
export const testRun = async (workspaceId: string, automationId: string, contactEmail: string): Promise<TestRunResult>
export const validateFlow = async (flow: FlowDefinition): Promise<ValidationError[]>

export const getExecution = async (workspaceId: string, executionId: string): Promise<AutomationExecution>
export const listExecutions = async (workspaceId: string, automationId: string, params?: ListExecutionsParams): Promise<ExecutionListResponse>

export const getAnalytics = async (workspaceId: string, automationId: string, startDate: string, endDate: string): Promise<AutomationAnalytics[]>
export const getAuditLog = async (workspaceId: string, automationId: string, params?: PaginationParams): Promise<AuditLogResponse>
```

## Frontend Components

### Automation Builder Page

**File**: `console/src/pages/AutomationBuilder.tsx` (create new)

Main page with ReactFlow canvas for building automations:

```tsx
import ReactFlow, {
  Background,
  Controls,
  MiniMap,
  Node,
  Edge,
  addEdge,
  useNodesState,
  useEdgesState,
} from 'reactflow'
import 'reactflow/dist/style.css'

export const AutomationBuilder = () => {
  const [nodes, setNodes, onNodesChange] = useNodesState([])
  const [edges, setEdges, onEdgesChange] = useEdgesState([])
  const [validationErrors, setValidationErrors] = useState<ValidationError[]>([])
  
  // Node palette sidebar
  // Canvas area with ReactFlow
  // Configuration panel for selected node
  // Validation error display
  // Save/Activate controls
  // Test run button
  // Version history dropdown
}
```

### Custom Node Components

**Files**: Create in `console/src/components/automations/nodes/`

Each node type has a custom React component:

- `TriggerNode.tsx` - Entry point, shows trigger type and audience filters (segments/lists)
- `DelayNode.tsx` - Shows delay duration and timezone setting
- `SplitTestNode.tsx` - Shows A/B split configuration with sample size indicator (enhanced from Loops.co)
- `ConditionNode.tsx` - Shows condition rules with continuous monitoring indicator
- `ParallelConditionNode.tsx` - NEW from Loops.co - Shows multiple branches with "parallel" badge
- `SendEmailNode.tsx` - Shows template selection
- `UpdatePropertyNode.tsx` - Shows property and value
- `UpdateListStatusNode.tsx` - Shows list and status
- `WebhookNode.tsx` - Shows URL and method
- `WaitForEventNode.tsx` - Shows event type and timeout

**Note**: No ExitNode needed - nodes without outgoing edges naturally complete the automation. UI shows terminal nodes with special styling.

### Automation List Page

**File**: `console/src/pages/Automations.tsx` (create new)

Table view of all automations with:
- Status badges (draft, active, paused)
- **Pause warning indicator** (NEW from Loops.co) - shows if paused > 24h
- Test mode indicator
- Version number
- Statistics (executions, success rate)
- Actions (edit, activate/pause, delete, test run)
- Create new button
- Filter by status/test mode

### Execution History Drawer

**File**: `console/src/components/automations/ExecutionHistoryDrawer.tsx` (create new)

Shows execution history for an automation:
- List of executions with status
- Contact email
- Execution path visualization
- Error messages
- Test run indicator
- Timestamps
- Context data viewer

### Node Configuration Panels

**File**: `console/src/components/automations/NodeConfigPanel.tsx` (create new)

Configuration drawer for selected node with forms for:

- **Trigger Node**: 
  - Event type selector (contact_created, contact_updated, etc.)
  - Segment multi-select (restrict to contacts in selected segments)
  - List multi-select (restrict to contacts subscribed to selected lists)
  - Additional conditions builder
  - Help text explaining filter logic (segments AND lists if both specified)
- **Delay Node**: Duration input (hours, minutes), timezone toggle, send at hour selector
- **Split Test Node** (enhanced from Loops.co): 
  - Sample size slider (0-100%)
  - Variant configuration with weights
  - Control branch toggle
  - Warning if sample < 100% and no control
  - Visual distribution preview
- **Condition Node**: 
  - Condition builder (field, operator, value)
  - Logic selector (AND/OR)
  - Continuous monitoring toggle (from Loops.co "All following nodes")
  - "Apply to next only" option
- **Parallel Condition Node** (NEW from Loops.co):
  - Multiple branch builder
  - Each branch has its own conditions
  - Visual indicator showing "parallel execution"
  - Help text: "Contact follows ALL matching branches"
- **Send Email Node**: Transactional template selector
- **Update Property Node**: Property selector, value input with variable support
- **Update List Status Node**: List selector, status radio (subscribed/unsubscribed)
- **Webhook Node**: URL input, method selector, headers, body template with variables
- **Wait for Event Node**: Event type selector, timeout hours

**Note**: No Exit Node configuration needed. Nodes automatically complete when they have no outgoing edges. The UI can show a visual indicator for terminal nodes.

### Analytics Dashboard

**File**: `console/src/components/automations/AnalyticsDashboard.tsx` (create new)

Shows automation performance metrics:
- Funnel chart (triggered → started → completed)
- Success/failure rate over time
- Node-level performance (execution count, failure rate, avg duration)
- Execution time distribution

### Audit Log Viewer

**File**: `console/src/components/automations/AuditLogDrawer.tsx` (create new)

Shows audit history:
- Timeline of changes
- User who made change
- Action type
- Changes diff viewer
- Version numbers

### Test Run Panel

**File**: `console/src/components/automations/TestRunPanel.tsx` (create new)

Test automation execution:
- Contact email input
- Run test button
- Execution log display
- Node-by-node results
- Success/failure indicator

### Pause Warning Modal (NEW from Loops.co)

**File**: `console/src/components/automations/PauseWarningModal.tsx` (create new)

Displays when automation paused > 24 hours:
- Warning message about outdated triggers
- Time since paused
- Option to resume or stop
- Checkbox: "Don't show again for this automation"

## Task Integration

### Initialize Processors

**File**: `internal/app/app.go`

In the app initialization, register the automation processors:

```go
// Create automation processors
automationExecutionProcessor := service.NewAutomationExecutionProcessor(
    automationRepo,
    contactRepo,
    transactionalService,
    listService,
    taskRepo,
    logger,
)

automationCheckProcessor := service.NewAutomationCheckTaskProcessor(
    automationRepo,
    taskService,
    logger,
)

// Register with task service
taskService.RegisterProcessor(automationExecutionProcessor)
taskService.RegisterProcessor(automationCheckProcessor)

// Subscribe to contact events
automationService.SubscribeToContactEvents(eventBus)
```

### Workspace Creation Integration

When a workspace is created, ensure the automation check task exists:

**File**: `internal/service/workspace_service.go`

```go
// In CreateWorkspace method, after workspace creation:
if err := service.EnsureAutomationCheckTask(ctx, s.taskRepo, workspace.ID); err != nil {
    s.logger.WithField("error", err.Error()).Warn("Failed to create automation check task")
}
```

## Frontend Routing

**File**: `console/src/router.tsx`

Add routes:

```typescript
{
  path: '/console/workspaces/$workspaceId/automations',
  component: AutomationsPage,
},
{
  path: '/console/workspaces/$workspaceId/automations/new',
  component: AutomationBuilder,
},
{
  path: '/console/workspaces/$workspaceId/automations/$automationId',
  component: AutomationBuilder,
}
```

## Testing Strategy

### Backend Tests

1. **Domain Tests** (`internal/domain/automation_test.go`):
   - Automation validation
   - Flow definition validation
   - Execution context methods
   - Entry rules logic

2. **Validation Tests** (`internal/domain/automation_validator_test.go`):
   - Flow validation (orphaned nodes, cycles, invalid connections)
   - Edge case scenarios
   - Split test and condition node validation

3. **Migration Tests** (`internal/migrations/v7_test.go`):
   - Table creation
   - Index creation
   - Default values
   - Unique constraints

4. **Repository Tests** (`internal/repository/automation_postgres_test.go`):
   - CRUD operations
   - Query filtering
   - Execution state management
   - Entry deduplication checks
   - Audience filtering queries (IsContactInSegments, IsContactSubscribedToLists)
   - Analytics operations
   - Audit log operations

5. **Service Tests** (`internal/service/automation_service_test.go`):
   - Business logic
   - Event handling
   - Trigger execution with entry rules
   - Audience filtering (segment and list checks)
   - CheckTriggerAudience method with various filter combinations
   - Version control
   - Test run functionality
   - Permissions checking

6. **Processor Tests**:
   - `internal/service/automation_execution_processor_test.go`
   - `internal/service/automation_check_task_processor_test.go`
   - Node execution logic
   - State persistence
   - Error handling
   - Analytics tracking

7. **Node Executor Tests** (`internal/service/automation_node_executors_test.go`):
   - Each node type execution
   - Condition evaluation
   - Variable interpolation
   - Timezone handling

8. **HTTP Tests** (`internal/http/automation_handler_test.go`):
   - Endpoint responses
   - Request validation
   - Error cases
   - Permissions enforcement

### Frontend Tests

1. **Component Tests**:
   - Node components rendering
   - Configuration panels
   - Flow builder interactions
   - Validation error display

2. **API Integration Tests**:
   - Mock API calls
   - Response handling
   - Error states

## Implementation Order

1. ✅ **Create TODO list** - Track all implementation tasks
2. **Database Migration** - Create v7 migration with all tables and indexes (including pause fields)
3. **Domain Models** - Create automation domain types with validation (including enhanced nodes from Loops.co)
4. **Flow Validator** - Implement comprehensive flow validation
5. **Event Types** - Add contact event types for triggers
6. **Permissions** - Add automations to permission system
7. **Repository Layer** - Implement PostgreSQL operations (including pause timeout queries)
8. **Service Layer** - Implement automation service with audit logging and pause management
9. **Node Executors** - Implement each node type execution logic (including parallel condition)
10. **Enhanced Node Data Structures** - Implement split test with sample size, condition with continuous check
11. **Execution Processor** - Implement task processor for running automations
12. **Check Task Processor** - Implement evergreen checker task
13. **Pause Timeout Checker** - Implement 24-hour pause warning system (NEW from Loops.co)
14. **HTTP Handlers** - Create API endpoints with permissions
15. **App Integration** - Wire up services and processors
16. **Workspace Integration** - Ensure check task on workspace creation
17. **Frontend Dependencies** - Add ReactFlow to package.json
18. **Frontend Types** - Create TypeScript interfaces (including pause fields and enhanced nodes)
19. **Frontend API Client** - Implement API functions
20. **Custom Node Components** - Create React components for each node type (including parallel condition)
21. **Enhanced Split Test Config** - Sample size slider and variant weights UI
22. **Continuous Monitoring Toggle** - Add to condition node config
23. **Automation Builder** - Create main builder page with ReactFlow and validation
24. **Node Config Panel** - Create configuration UI for nodes (with enhancements)
25. **Automations List** - Create list view page with filters and pause warnings
26. **Pause Warning Modal** - Create 24-hour pause warning UI (NEW from Loops.co)
27. **Execution History** - Create execution viewer
28. **Analytics Dashboard** - Create analytics visualization
29. **Audit Log Viewer** - Create audit log interface
30. **Test Run Panel** - Create test execution interface
31. **Frontend Routing** - Add automation routes
32. **Backend Tests** - Write comprehensive test suite (including pause logic and enhanced nodes)
33. **Frontend Tests** - Write component and integration tests
34. **Integration Testing** - Test end-to-end flows (including pause warnings)
35. **Documentation** - Update CHANGELOG.md

## Files to Create

### Backend
- `internal/migrations/v7.go` (or v8.go based on version)
- `internal/domain/automation.go`
- `internal/domain/automation_validator.go`
- `internal/domain/automation_test.go`
- `internal/domain/automation_validator_test.go`
- `internal/repository/automation_postgres.go`
- `internal/repository/automation_postgres_test.go`
- `internal/service/automation_service.go`
- `internal/service/automation_service_test.go`
- `internal/service/automation_execution_processor.go`
- `internal/service/automation_execution_processor_test.go`
- `internal/service/automation_check_task_processor.go`
- `internal/service/automation_check_task_processor_test.go`
- `internal/service/automation_pause_checker.go` - NEW from Loops.co insight
- `internal/service/automation_pause_checker_test.go` - NEW
- `internal/service/automation_node_executors.go`
- `internal/service/automation_node_executors_test.go`
- `internal/http/automation_handler.go`
- `internal/http/automation_handler_test.go`

### Frontend
- `console/src/services/api/automations.ts`
- `console/src/pages/Automations.tsx`
- `console/src/pages/AutomationBuilder.tsx`
- `console/src/components/automations/nodes/TriggerNode.tsx`
- `console/src/components/automations/nodes/DelayNode.tsx`
- `console/src/components/automations/nodes/SplitTestNode.tsx`
- `console/src/components/automations/nodes/ConditionNode.tsx`
- `console/src/components/automations/nodes/ParallelConditionNode.tsx` - NEW from Loops.co insight
- `console/src/components/automations/nodes/SendEmailNode.tsx`
- `console/src/components/automations/nodes/UpdatePropertyNode.tsx`
- `console/src/components/automations/nodes/UpdateListStatusNode.tsx`
- `console/src/components/automations/nodes/WebhookNode.tsx`
- `console/src/components/automations/nodes/WaitForEventNode.tsx`
- `console/src/components/automations/NodeConfigPanel.tsx`
- `console/src/components/automations/ExecutionHistoryDrawer.tsx`
- `console/src/components/automations/AnalyticsDashboard.tsx`
- `console/src/components/automations/AuditLogDrawer.tsx`
- `console/src/components/automations/TestRunPanel.tsx`
- `console/src/components/automations/PauseWarningModal.tsx` - NEW from Loops.co insight
- `console/src/components/automations/EnhancedSplitTestConfig.tsx` - NEW from Loops.co insight
- `console/src/components/automations/ContinuousMonitorToggle.tsx` - NEW from Loops.co insight
- `console/src/components/automations/AutomationStats.tsx`

## Files to Modify

### Backend
- `internal/domain/event.go` - Add contact event types
- `internal/domain/workspace.go` - Add automations to permissions
- `internal/app/app.go` - Initialize automation services and processors
- `internal/service/workspace_service.go` - Ensure automation check task on creation
- `internal/service/contact_service.go` - Emit contact events
- `internal/service/list_service.go` - Emit list subscription events
- `config/config.go` - Update VERSION constant

### Frontend
- `console/package.json` - Add ReactFlow dependencies
- `console/src/router.tsx` - Add automation routes
- Navigation menu - Add automations link

## Key Design Considerations

1. **Flow Validation**: Comprehensive validation prevents invalid automations from being activated
2. **Entry Deduplication**: Unique constraint and cooldown rules prevent duplicate executions
3. **Variable Context**: Rich execution context allows data to flow between nodes
4. **Idempotency**: Automation executions should be idempotent to handle retries
5. **State Management**: Execution state must be persisted for resumability
6. **Error Handling**: Failed nodes should be retried with backoff
7. **Performance**: Batch process pending executions efficiently
8. **Scalability**: Design for future distributed execution
9. **Testing**: Comprehensive test coverage for all node types
10. **Testing Mode**: Safe testing without affecting production data
11. **Timezone Awareness**: Respect contact timezones for delay nodes
12. **Analytics**: Track node-level and automation-level metrics
13. **Audit Trail**: Complete history of changes for compliance
14. **Version Control**: Rollback capability for automation flows
15. **Permissions**: Granular access control for automation management
16. **UI/UX**: Intuitive drag-and-drop with clear visual feedback
17. **Documentation**: Clear examples for each node type

## Audience Filtering Use Cases

The audience filtering on triggers enables powerful targeting scenarios:

### Example 1: VIP Welcome Series
```
Trigger: contact_created
Audience Filter: segment_ids = ["vip_customers"]
Flow: Send personalized welcome email → Wait 2 days → Send exclusive offer
```
Only contacts in the "VIP Customers" segment receive this automation.

### Example 2: Newsletter Engagement
```
Trigger: contact_updated (when email_opened property changes)
Audience Filter: subscribed_lists = ["weekly_newsletter"]
Flow: Check if opened > 5 times → Send "Top Reader" badge email
```
Only contacts subscribed to the newsletter list are considered.

### Example 3: Combined Filtering
```
Trigger: list_subscribed
Audience Filter: 
  - segment_ids = ["high_value", "engaged"]
  - subscribed_lists = ["product_updates"]
Flow: Send welcome email → Wait for purchase event (7 days) → Send discount
```
Contact must be in "high_value" OR "engaged" segment AND subscribed to "product_updates" list.

### Example 4: Re-engagement Campaign
```
Trigger: contact_updated (when last_activity changes)
Audience Filter: segment_ids = ["inactive_30_days"]
Flow: Wait 7 days → Send re-engagement email → If no open, send discount
```
Only contacts in the "Inactive 30 Days" segment enter the re-engagement flow.

### Filter Logic Summary
- **No filters**: All contacts trigger the automation
- **Segments only**: Contact must be in ANY of the specified segments
- **Lists only**: Contact must be subscribed to ANY of the specified lists  
- **Both specified**: Contact must match segments (ANY) AND lists (ANY)

This allows precise targeting without creating separate automations for each audience segment.

## Success Metrics

- Automations can be created and activated without errors
- Flow validation prevents invalid automation structures
- Contact events trigger automation executions
- Entry rules prevent duplicate executions
- Audience filtering correctly restricts triggers to matching contacts
- Segment and list filters work independently and combined
- All node types execute correctly (including conditional logic)
- Variables flow correctly between nodes
- State is preserved across task pauses
- Timezone-aware delays work correctly
- Wait for event nodes function properly
- Test mode executions don't affect production
- Execution history is accurate
- Analytics provide useful insights
- Audit log tracks all changes
- Version rollback works correctly
- UI is intuitive and responsive
- Test coverage > 80% for backend
- Test coverage > 70% for frontend

---

---

## Competitive Insights from Loops.co

Based on analysis of Loops.co's Loop Builder (see `/workspace/research/loops-co-analysis.md`), we've incorporated 4 critical enhancements:

### 1. ✅ 24-Hour Pause Protection
- **Problem**: Paused automations can send outdated emails (e.g., "Welcome!" 3 months after signup)
- **Solution**: 
  - Track `paused_at` timestamp
  - Send warning email after 24 hours
  - Show warning badge in UI
  - Block new entries after 24 hours (prevents stale triggers)

### 2. ✅ Enhanced Split Testing
- **Problem**: Basic 50/50 split is too limited
- **Solution**: 
  - Sample size control (0-100%)
  - Variant weights (e.g., 60% A, 40% B)
  - Optional control branch
  - Warning if sample < 100% and no control

### 3. ✅ Parallel Branching
- **Problem**: Condition nodes force single path (either/or)
- **Solution**: 
  - New `parallel_condition` node type
  - Contacts follow ALL matching branches
  - Example: Send both "VIP Welcome" AND "Newsletter" if match both

### 4. ✅ Continuous Audience Monitoring
- **Problem**: One-time checks can't react to changing contact properties
- **Solution**: 
  - Add `continuous_check` flag to condition nodes
  - Contacts auto-exit if stop matching
  - Example: Contact leaves "VIP" segment → exits VIP automation

**Result**: We now have **both** Loops.co's smart safeguards **and** our advanced features (webhooks, variables, version control, audit trail, test mode).

**Positioning**: **"Enterprise Automation Platform"** vs Loops.co's **"Simple Marketing Automation"**

---

**Last Updated**: 2025-10-12
**Status**: Planning Complete with Competitive Enhancements from Loops.co Analysis, Ready for Implementation
