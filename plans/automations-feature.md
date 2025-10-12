# Automations Feature Implementation Plan

## Overview

This plan implements a comprehensive automation system for Notifuse that allows users to create visual workflow automations triggered by contact timeline events. Automations run asynchronously using the existing task system (similar to segments) and provide a drag-and-drop visual interface built with ReactFlow.

## Architecture Overview

Automations follow the same async processing pattern as segments:
1. **Event-driven**: Contact timeline events trigger automation executions
2. **Task-based processing**: Automations run as background tasks
3. **Evergreen task**: A recurring task checks for pending automation executions
4. **State tracking**: Automation execution state is persisted for resumability

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
    
    -- Statistics
    executions_count INTEGER DEFAULT 0,
    successes_count INTEGER DEFAULT 0,
    failures_count INTEGER DEFAULT 0,
    
    -- Timestamps
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    last_executed_at TIMESTAMP WITH TIME ZONE
);

CREATE INDEX IF NOT EXISTS idx_automations_status ON automations(status);
CREATE INDEX IF NOT EXISTS idx_automations_trigger_type ON automations(trigger_type);
```

### Automation Executions Table

Tracks individual automation runs for specific contacts.

```sql
CREATE TABLE IF NOT EXISTS automation_executions (
    id VARCHAR(36) PRIMARY KEY,
    automation_id VARCHAR(36) NOT NULL REFERENCES automations(id) ON DELETE CASCADE,
    contact_email VARCHAR(255) NOT NULL,
    
    -- Execution state
    status VARCHAR(20) NOT NULL DEFAULT 'pending', -- pending, running, completed, failed, cancelled
    current_node_id VARCHAR(36), -- Current node in flow being executed
    execution_path JSONB, -- Array of node IDs that have been executed
    execution_data JSONB, -- Data accumulated during execution
    
    -- Scheduling
    scheduled_at TIMESTAMP WITH TIME ZONE,
    started_at TIMESTAMP WITH TIME ZONE,
    completed_at TIMESTAMP WITH TIME ZONE,
    
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
            executions_count INTEGER DEFAULT 0,
            successes_count INTEGER DEFAULT 0,
            failures_count INTEGER DEFAULT 0,
            created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
            updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
            last_executed_at TIMESTAMP WITH TIME ZONE
        )
    `)
    if err != nil {
        return err
    }

    // Create indexes
    _, err = db.ExecContext(ctx, `
        CREATE INDEX IF NOT EXISTS idx_automations_status ON automations(status);
        CREATE INDEX IF NOT EXISTS idx_automations_trigger_type ON automations(trigger_type);
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
            execution_data JSONB,
            scheduled_at TIMESTAMP WITH TIME ZONE,
            started_at TIMESTAMP WITH TIME ZONE,
            completed_at TIMESTAMP WITH TIME ZONE,
            error_message TEXT,
            retry_count INTEGER DEFAULT 0,
            created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
            updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
        )
    `)
    if err != nil {
        return err
    }

    // Create indexes
    _, err = db.ExecContext(ctx, `
        CREATE INDEX IF NOT EXISTS idx_automation_executions_automation_id ON automation_executions(automation_id);
        CREATE INDEX IF NOT EXISTS idx_automation_executions_contact_email ON automation_executions(contact_email);
        CREATE INDEX IF NOT EXISTS idx_automation_executions_status ON automation_executions(status);
        CREATE INDEX IF NOT EXISTS idx_automation_executions_scheduled_at ON automation_executions(scheduled_at);
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
    Type     string                 `json:"type"` // trigger, delay, split_test, send_email, update_property, update_list_status, webhook
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
    Label  string `json:"label,omitempty"` // For split test A/B labeling
}

// TriggerConfig contains trigger-specific configuration
type TriggerConfig struct {
    ListID      *string                `json:"list_id,omitempty"`      // For list triggers
    EventName   *string                `json:"event_name,omitempty"`   // For custom event triggers
    Conditions  []TriggerCondition     `json:"conditions,omitempty"`   // Additional filtering
    Metadata    map[string]interface{} `json:"metadata,omitempty"`
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
    Operator string      `json:"operator"` // equals, not_equals, contains, gt, lt, etc.
    Value    interface{} `json:"value"`
}

// Automation represents an automation workflow
type Automation struct {
    ID             string         `json:"id"`
    Name           string         `json:"name"`
    Description    *string        `json:"description,omitempty"`
    Status         AutomationStatus `json:"status"`
    FlowDefinition FlowDefinition `json:"flow_definition"`
    TriggerType    TriggerType    `json:"trigger_type"`
    TriggerConfig  *TriggerConfig `json:"trigger_config,omitempty"`
    
    // Statistics
    ExecutionsCount int `json:"executions_count"`
    SuccessesCount  int `json:"successes_count"`
    FailuresCount   int `json:"failures_count"`
    
    // Timestamps
    CreatedAt       time.Time  `json:"created_at"`
    UpdatedAt       time.Time  `json:"updated_at"`
    LastExecutedAt  *time.Time `json:"last_executed_at,omitempty"`
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
    // Validate that there's exactly one trigger node
    triggerCount := 0
    for _, node := range a.FlowDefinition.Nodes {
        if node.Type == "trigger" {
            triggerCount++
        }
    }
    if triggerCount != 1 {
        return fmt.Errorf("flow must have exactly one trigger node")
    }
    return nil
}

// AutomationExecution represents a single execution of an automation for a contact
type AutomationExecution struct {
    ID            string                 `json:"id"`
    AutomationID  string                 `json:"automation_id"`
    ContactEmail  string                 `json:"contact_email"`
    Status        string                 `json:"status"` // pending, running, completed, failed, cancelled
    CurrentNodeID *string                `json:"current_node_id,omitempty"`
    ExecutionPath []string               `json:"execution_path,omitempty"`
    ExecutionData map[string]interface{} `json:"execution_data,omitempty"`
    
    // Scheduling
    ScheduledAt *time.Time `json:"scheduled_at,omitempty"`
    StartedAt   *time.Time `json:"started_at,omitempty"`
    CompletedAt *time.Time `json:"completed_at,omitempty"`
    
    // Error tracking
    ErrorMessage *string `json:"error_message,omitempty"`
    RetryCount   int     `json:"retry_count"`
    
    // Timestamps
    CreatedAt time.Time `json:"created_at"`
    UpdatedAt time.Time `json:"updated_at"`
}

// AutomationService defines the business logic for automations
type AutomationService interface {
    // CRUD operations
    Create(ctx context.Context, workspaceID string, automation *Automation) error
    Get(ctx context.Context, workspaceID, id string) (*Automation, error)
    Update(ctx context.Context, workspaceID string, automation *Automation) error
    Delete(ctx context.Context, workspaceID, id string) error
    List(ctx context.Context, workspaceID string, filter AutomationFilter) (*AutomationListResponse, error)
    
    // Activation
    Activate(ctx context.Context, workspaceID, id string) error
    Pause(ctx context.Context, workspaceID, id string) error
    
    // Execution management
    TriggerExecution(ctx context.Context, workspaceID, automationID, contactEmail string, eventData map[string]interface{}) error
    GetExecution(ctx context.Context, workspaceID, executionID string) (*AutomationExecution, error)
    ListExecutions(ctx context.Context, workspaceID, automationID string, filter ExecutionFilter) (*ExecutionListResponse, error)
    
    // Event bus integration
    SubscribeToContactEvents(eventBus EventBus)
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
    
    // Batch queries
    GetActiveAutomationsByTrigger(ctx context.Context, workspaceID string, triggerType TriggerType) ([]*Automation, error)
    GetPendingExecutions(ctx context.Context, workspaceID string, limit int) ([]*AutomationExecution, error)
    
    // Statistics
    IncrementExecutionCount(ctx context.Context, workspaceID, automationID string) error
    IncrementSuccessCount(ctx context.Context, workspaceID, automationID string) error
    IncrementFailureCount(ctx context.Context, workspaceID, automationID string) error
}

// AutomationFilter for listing automations
type AutomationFilter struct {
    Status      []AutomationStatus
    TriggerType []TriggerType
    Limit       int
    Offset      int
}

// ExecutionFilter for listing executions
type ExecutionFilter struct {
    AutomationID string
    ContactEmail string
    Status       []string
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
}

func (r *CreateAutomationRequest) Validate() (*Automation, error) {
    automation := &Automation{
        Name:           r.Name,
        Description:    r.Description,
        Status:         AutomationStatusDraft,
        FlowDefinition: r.FlowDefinition,
        TriggerType:    r.TriggerType,
        TriggerConfig:  r.TriggerConfig,
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
}
```

### Event Integration

Add new event types to `internal/domain/event.go`:

```go
// Add to existing EventType constants
const (
    // ... existing events ...
    
    // Contact events for automation triggers
    EventContactCreated       EventType = "contact.created"
    EventContactUpdated       EventType = "contact.updated"
    EventContactListSubscribed   EventType = "contact.list_subscribed"
    EventContactListUnsubscribed EventType = "contact.list_unsubscribed"
)
```

## Backend Repository Layer

**File**: `internal/repository/automation_postgres.go` (create new)

Implements `AutomationRepository` interface with PostgreSQL queries using Squirrel query builder.

Key methods:
- `Create`, `Get`, `Update`, `Delete`, `List` for automations
- `CreateExecution`, `GetExecution`, `UpdateExecution`, `ListExecutions` for executions
- `GetActiveAutomationsByTrigger` - finds active automations for a trigger type
- `GetPendingExecutions` - gets executions scheduled to run now
- Statistics increment methods

## Backend Service Layer

### Automation Service

**File**: `internal/service/automation_service.go` (create new)

Implements business logic and orchestration:

```go
type AutomationService struct {
    automationRepo   domain.AutomationRepository
    contactRepo      domain.ContactRepository
    transactionalSvc domain.TransactionalService
    listService      domain.ListService
    taskService      domain.TaskService
    eventBus         domain.EventBus
    logger           logger.Logger
}

// Key methods:
// - Create, Get, Update, Delete, List (CRUD)
// - Activate, Pause (status management)
// - TriggerExecution (creates automation execution record)
// - SubscribeToContactEvents (listens for trigger events)
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
    // 4. Execute nodes sequentially following edges
    // 5. Handle delays by pausing task
    // 6. Save execution state
    // 7. Return completion status
}
```

Node execution logic:
- **Trigger**: Entry point (no action)
- **Delay**: Schedule next execution (pause task)
- **Split Test**: Randomly choose A or B path
- **Send Transactional Email**: Call transactional service
- **Update Contact Property**: Call contact service
- **Update List Status**: Call list service
- **Webhook**: HTTP POST to configured URL

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
    // 3. Mark task as pending (not complete) to keep it recurring
    return false, nil // false = not complete, reschedule
}

// Helper function to ensure task exists for workspace
func EnsureAutomationCheckTask(ctx context.Context, taskRepo domain.TaskRepository, workspaceID string) error {
    // Similar to EnsureSegmentRecomputeTask
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

// GET  /api/automation.execution.get?workspace_id=X&id=Y
// GET  /api/automation.execution.list?workspace_id=X&automation_id=Y
```

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
  type: string // 'trigger' | 'delay' | 'split_test' | 'send_email' | 'update_property' | 'update_list_status' | 'webhook'
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
  metadata?: Record<string, any>
}

export interface TriggerCondition {
  field: string
  operator: string
  value: any
}

export interface Automation {
  id: string
  name: string
  description?: string
  status: 'draft' | 'active' | 'paused'
  flow_definition: FlowDefinition
  trigger_type: string
  trigger_config?: TriggerConfig
  executions_count: number
  successes_count: number
  failures_count: number
  created_at: string
  updated_at: string
  last_executed_at?: string
}

export interface AutomationExecution {
  id: string
  automation_id: string
  contact_email: string
  status: string
  current_node_id?: string
  execution_path?: string[]
  execution_data?: Record<string, any>
  scheduled_at?: string
  started_at?: string
  completed_at?: string
  error_message?: string
  retry_count: number
  created_at: string
  updated_at: string
}

// API functions
export const createAutomation = async (workspaceId: string, data: CreateAutomationRequest): Promise<Automation>
export const getAutomation = async (workspaceId: string, id: string): Promise<Automation>
export const updateAutomation = async (workspaceId: string, id: string, data: UpdateAutomationRequest): Promise<Automation>
export const deleteAutomation = async (workspaceId: string, id: string): Promise<void>
export const listAutomations = async (workspaceId: string, params?: ListAutomationsParams): Promise<AutomationListResponse>
export const activateAutomation = async (workspaceId: string, id: string): Promise<Automation>
export const pauseAutomation = async (workspaceId: string, id: string): Promise<Automation>

export const getExecution = async (workspaceId: string, executionId: string): Promise<AutomationExecution>
export const listExecutions = async (workspaceId: string, automationId: string, params?: ListExecutionsParams): Promise<ExecutionListResponse>
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
  
  // Node palette sidebar
  // Canvas area with ReactFlow
  // Configuration panel for selected node
  // Save/Activate controls
}
```

### Custom Node Components

**Files**: Create in `console/src/components/automations/nodes/`

Each node type has a custom React component:

- `TriggerNode.tsx` - Entry point, shows trigger type
- `DelayNode.tsx` - Shows delay duration
- `SplitTestNode.tsx` - Shows A/B split configuration
- `SendEmailNode.tsx` - Shows template selection
- `UpdatePropertyNode.tsx` - Shows property and value
- `UpdateListStatusNode.tsx` - Shows list and status
- `WebhookNode.tsx` - Shows URL and method

### Automation List Page

**File**: `console/src/pages/Automations.tsx` (create new)

Table view of all automations with:
- Status badges (draft, active, paused)
- Statistics (executions, success rate)
- Actions (edit, activate/pause, delete)
- Create new button

### Execution History Drawer

**File**: `console/src/components/automations/ExecutionHistoryDrawer.tsx` (create new)

Shows execution history for an automation:
- List of executions with status
- Contact email
- Execution path visualization
- Error messages
- Timestamps

### Node Configuration Panels

**File**: `console/src/components/automations/NodeConfigPanel.tsx` (create new)

Configuration drawer for selected node with forms for:

- **Delay Node**: Duration input (hours, minutes)
- **Split Test Node**: Split percentage, branch labels
- **Send Email Node**: Transactional template selector
- **Update Property Node**: Property selector, value input
- **Update List Status Node**: List selector, status radio (subscribed/unsubscribed)
- **Webhook Node**: URL input, method selector, headers, body template

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
   - Node/edge structure validation

2. **Migration Tests** (`internal/migrations/v7_test.go`):
   - Table creation
   - Index creation
   - Default values

3. **Repository Tests** (`internal/repository/automation_postgres_test.go`):
   - CRUD operations
   - Query filtering
   - Execution state management

4. **Service Tests** (`internal/service/automation_service_test.go`):
   - Business logic
   - Event handling
   - Trigger execution

5. **Processor Tests**:
   - `internal/service/automation_execution_processor_test.go`
   - `internal/service/automation_check_task_processor_test.go`
   - Node execution logic
   - State persistence
   - Error handling

6. **HTTP Tests** (`internal/http/automation_handler_test.go`):
   - Endpoint responses
   - Request validation
   - Error cases

### Frontend Tests

1. **Component Tests**:
   - Node components rendering
   - Configuration panels
   - Flow builder interactions

2. **API Integration Tests**:
   - Mock API calls
   - Response handling
   - Error states

## Implementation Order

1. ✅ **Create TODO list** - Track all implementation tasks
2. **Database Migration** - Create v7 migration with tables and indexes
3. **Domain Models** - Create automation domain types
4. **Event Types** - Add contact event types for triggers
5. **Repository Layer** - Implement PostgreSQL operations
6. **Service Layer** - Implement automation service
7. **Node Executors** - Implement each node type execution logic
8. **Execution Processor** - Implement task processor for running automations
9. **Check Task Processor** - Implement evergreen checker task
10. **HTTP Handlers** - Create API endpoints
11. **App Integration** - Wire up services and processors
12. **Workspace Integration** - Ensure check task on workspace creation
13. **Frontend Dependencies** - Add ReactFlow to package.json
14. **Frontend Types** - Create TypeScript interfaces
15. **Frontend API Client** - Implement API functions
16. **Custom Node Components** - Create React components for each node type
17. **Automation Builder** - Create main builder page with ReactFlow
18. **Node Config Panel** - Create configuration UI for nodes
19. **Automations List** - Create list view page
20. **Execution History** - Create execution viewer
21. **Frontend Routing** - Add automation routes
22. **Backend Tests** - Write comprehensive test suite
23. **Frontend Tests** - Write component and integration tests
24. **Integration Testing** - Test end-to-end flows
25. **Documentation** - Update CHANGELOG.md

## Files to Create

### Backend
- `internal/migrations/v7.go` (or v8.go based on version)
- `internal/domain/automation.go`
- `internal/repository/automation_postgres.go`
- `internal/repository/automation_postgres_test.go`
- `internal/service/automation_service.go`
- `internal/service/automation_service_test.go`
- `internal/service/automation_execution_processor.go`
- `internal/service/automation_execution_processor_test.go`
- `internal/service/automation_check_task_processor.go`
- `internal/service/automation_check_task_processor_test.go`
- `internal/http/automation_handler.go`
- `internal/http/automation_handler_test.go`

### Frontend
- `console/src/services/api/automations.ts`
- `console/src/pages/Automations.tsx`
- `console/src/pages/AutomationBuilder.tsx`
- `console/src/components/automations/nodes/TriggerNode.tsx`
- `console/src/components/automations/nodes/DelayNode.tsx`
- `console/src/components/automations/nodes/SplitTestNode.tsx`
- `console/src/components/automations/nodes/SendEmailNode.tsx`
- `console/src/components/automations/nodes/UpdatePropertyNode.tsx`
- `console/src/components/automations/nodes/UpdateListStatusNode.tsx`
- `console/src/components/automations/nodes/WebhookNode.tsx`
- `console/src/components/automations/NodeConfigPanel.tsx`
- `console/src/components/automations/ExecutionHistoryDrawer.tsx`
- `console/src/components/automations/AutomationStats.tsx`

## Files to Modify

### Backend
- `internal/domain/event.go` - Add contact event types
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

1. **Idempotency**: Automation executions should be idempotent to handle retries
2. **State Management**: Execution state must be persisted for resumability
3. **Error Handling**: Failed nodes should be retried with backoff
4. **Performance**: Batch process pending executions efficiently
5. **Scalability**: Design for future distributed execution
6. **Testing**: Comprehensive test coverage for all node types
7. **UI/UX**: Intuitive drag-and-drop with clear visual feedback
8. **Documentation**: Clear examples for each node type

## Future Enhancements

- **Conditional logic**: If/else branching based on contact properties
- **Wait for event**: Pause execution until specific event occurs
- **Time windows**: Only execute during certain hours/days
- **Frequency capping**: Limit executions per contact per time period
- **Advanced analytics**: Conversion tracking, funnel visualization
- **Version control**: Track changes to automation flows
- **A/B test reporting**: Detailed statistics on split test performance
- **Bulk actions**: Apply automation to existing contacts
- **Import/Export**: Share automation templates

## Success Metrics

- Automations can be created and activated without errors
- Contact events trigger automation executions
- All node types execute correctly
- State is preserved across task pauses
- Execution history is accurate
- UI is intuitive and responsive
- Test coverage > 80% for backend
- Test coverage > 70% for frontend

---

**Last Updated**: 2025-10-12
**Status**: Planning Complete, Ready for Implementation
