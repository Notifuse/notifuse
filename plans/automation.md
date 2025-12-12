# Automation System - Backend Architecture

## Overview

Email marketing automation system triggered by contact timeline events, with visual flow builder and complete journey tracking. Backend-focused implementation leveraging existing segments condition engine and contact timeline system.

## Core Architecture

### Data Model

**Automation Entity**
```go
type Automation struct {
    ID          string                 `json:"id"`
    WorkspaceID string                 `json:"workspace_id"`
    Name        string                 `json:"name"`
    Status      AutomationStatus       `json:"status"` // draft, live, paused
    
    // List-based subscription management
    ListID      string                 `json:"list_id"`           // Required - contacts must be subscribed to this list
    ListName    string                 `json:"list_name"`         // Joined for display (not stored)
    
    Trigger     *TimelineTriggerConfig `json:"trigger"`
    RootNodeID  string                 `json:"root_node_id"`
    Stats       *AutomationStats       `json:"stats"`
    CreatedAt   time.Time              `json:"created_at"`
    UpdatedAt   time.Time              `json:"updated_at"`
}

type AutomationStatus string
const (
    AutomationStatusDraft  AutomationStatus = "draft"
    AutomationStatusLive   AutomationStatus = "live" 
    AutomationStatusPaused AutomationStatus = "paused"
)
```

**Node Entity (polymorphic)**
```go
type AutomationNode struct {
    ID           string          `json:"id"`
    AutomationID string          `json:"automation_id"`
    Type         NodeType        `json:"type"`
    Config       NodeConfig      `json:"config"` // JSON, type-specific
    NextNodeID   *string         `json:"next_node_id"`
    Position     NodePosition    `json:"position"` // x, y for visual editor
    CreatedAt    time.Time       `json:"created_at"`
}

type NodeType string
const (
    NodeTypeTrigger        NodeType = "trigger"
    NodeTypeDelay          NodeType = "delay"
    NodeTypeEmail          NodeType = "email"
    NodeTypeBranch         NodeType = "branch"
    NodeTypeFilter         NodeType = "filter"
    NodeTypeExit           NodeType = "exit"
    NodeTypeAddToList      NodeType = "add_to_list"
    NodeTypeRemoveFromList NodeType = "remove_from_list"
)
```

**Contact Journey Tracking**
```go
type ContactAutomation struct {
    ID              string                 `json:"id"`
    AutomationID    string                 `json:"automation_id"`
    ContactEmail    string                 `json:"contact_email"`
    CurrentNodeID   *string                `json:"current_node_id"`
    Status          ContactAutomationStatus `json:"status"`
    EnteredAt       time.Time              `json:"entered_at"`
    ScheduledAt     *time.Time             `json:"scheduled_at"`
    Context         map[string]interface{} `json:"context"`
    
    // Error handling and retry tracking
    RetryCount      int                    `json:"retry_count"`
    LastError       *string                `json:"last_error"`
    LastRetryAt     *time.Time             `json:"last_retry_at"`
    MaxRetries      int                    `json:"max_retries"`      // Configurable per automation
}

type ContactAutomationStatus string
const (
    ContactAutomationStatusActive    ContactAutomationStatus = "active"
    ContactAutomationStatusCompleted ContactAutomationStatus = "completed"
    ContactAutomationStatusExited    ContactAutomationStatus = "exited"
    ContactAutomationStatusFailed    ContactAutomationStatus = "failed"
)

// Journey tracking for troubleshooting
type AutomationJourneyEntry struct {
    ID                    string                 `json:"id"`
    ContactAutomationID   string                 `json:"contact_automation_id"`
    NodeID                string                 `json:"node_id"`
    NodeType              NodeType               `json:"node_type"`
    Action                JourneyAction          `json:"action"`
    EnteredAt             time.Time              `json:"entered_at"`
    CompletedAt           *time.Time             `json:"completed_at"`
    DurationMs            *int64                 `json:"duration_ms"`
    Metadata              map[string]interface{} `json:"metadata"`
    Error                 *string                `json:"error"`
}

type JourneyAction string
const (
    JourneyActionEntered    JourneyAction = "entered"
    JourneyActionProcessing JourneyAction = "processing"
    JourneyActionCompleted  JourneyAction = "completed"
    JourneyActionFailed     JourneyAction = "failed"
    JourneyActionSkipped    JourneyAction = "skipped"
)
```

---

## Trigger System - Contact Timeline Events Only

**Timeline Event Trigger Configuration**
```go
type TimelineTriggerConfig struct {
    EventKinds  []string          `json:"event_kinds"`  // Timeline event types to listen for
    Conditions  *domain.TreeNode  `json:"conditions"`   // Reuse segments condition system
    Frequency   TriggerFrequency  `json:"frequency"`
}

type TriggerFrequency string
const (
    TriggerFrequencyOnce      TriggerFrequency = "once"       // Only trigger on first occurrence
    TriggerFrequencyEveryTime TriggerFrequency = "every_time" // Trigger on each occurrence
)
```

### Supported Timeline Event Kinds

> **Note:** Event kinds were standardized to semantic naming in v18. The automation system uses the same event kinds as the contact_timeline table.

| Event Kind | Description | When Created |
|------------|-------------|--------------|
| **contact.created** | New contact created | Contact API, form submission |
| **contact.updated** | Contact properties changed | Contact update API, form update |
| **list.subscribed** | Subscribed to list (active status) | List management API, subscribe form |
| **list.pending** | Added to list with pending status | Double opt-in flow |
| **list.confirmed** | Double opt-in confirmed | Email confirmation click |
| **list.resubscribed** | Re-subscribed after being unsubscribed/bounced | List management API |
| **list.unsubscribed** | Unsubscribed from list | Unsubscribe link, API |
| **list.bounced** | Email bounced on list | Webhook from email provider |
| **list.complained** | Spam complaint on list | Webhook from email provider |
| **list.removed** | Removed from list (soft delete) | List management API |
| **segment.joined** | Contact enters segment | Segment recomputation |
| **segment.left** | Contact exits segment | Segment recomputation |
| **email.sent** | Email sent to contact | Email broadcast, template send |
| **email.delivered** | Email successfully delivered | Webhook from email provider |
| **email.opened** | Contact opened email | Webhook from email provider |
| **email.clicked** | Contact clicked email link | Webhook from email provider |
| **email.bounced** | Email bounced | Webhook from email provider |
| **email.complained** | Spam complaint | Webhook from email provider |
| **email.unsubscribed** | Unsubscribed via email link | Unsubscribe link click |
| **{event_name}** | Custom event (uses exact event_name) | Custom events API |

### Trigger Examples

**Welcome Series** - Trigger when a new contact is created
```json
{
  "event_kinds": ["contact.created"],
  "conditions": {
    "kind": "leaf",
    "leaf": {
      "source": "contacts",
      "contact": {
        "filters": [{
          "field_name": "custom_string_1",
          "field_type": "string",
          "operator": "equals",
          "string_values": ["website_signup"]
        }]
      }
    }
  },
  "frequency": "once"
}
```

**New Subscriber Flow** - Trigger when contact subscribes to a list
```json
{
  "event_kinds": ["list.subscribed"],
  "conditions": null,
  "frequency": "once"
}
```

**Re-engagement Flow** - Trigger when contact opens an email
```json
{
  "event_kinds": ["email.opened"],
  "conditions": {
    "kind": "leaf",
    "leaf": {
      "source": "contact_timeline",
      "contact_timeline": {
        "kind": "email.opened",
        "count_operator": "exactly",
        "count_value": 1,
        "timeframe_operator": "in_the_last_days",
        "timeframe_values": ["30"]
      }
    }
  },
  "frequency": "once"
}
```

**Segment-Based Upsell** - Trigger when contact enters a segment
```json
{
  "event_kinds": ["segment.joined"],
  "conditions": {
    "kind": "leaf",
    "leaf": {
      "source": "contact_timeline",
      "contact_timeline": {
        "kind": "segment.joined",
        "count_operator": "at_least",
        "count_value": 1,
        "timeframe_operator": "anytime",
        "filters": [{
          "field_name": "segment_id",
          "field_type": "string",
          "operator": "equals",
          "string_values": ["high_value_customers"]
        }]
      }
    }
  },
  "frequency": "every_time"
}
```

**Purchase Follow-up** - Trigger on custom purchase event
```json
{
  "event_kinds": ["order_completed"],
  "conditions": {
    "kind": "leaf",
    "leaf": {
      "source": "custom_events_goals",
      "custom_events_goal": {
        "goal_type": "purchase",
        "aggregate_operator": "sum",
        "operator": "gte",
        "value": 100,
        "timeframe_operator": "anytime"
      }
    }
  },
  "frequency": "every_time"
}
```

---

## Node System - Actions & Flow Control

### Node Configurations

**Delay Node**
```go
type DelayNodeConfig struct {
    Duration int    `json:"duration"`
    Unit     string `json:"unit"` // "minutes", "hours", "days"
}
```

**Email Node**
```go
type EmailNodeConfig struct {
    TemplateID      string  `json:"template_id"`
    SubjectOverride *string `json:"subject_override"`
    FromOverride    *string `json:"from_override"`
}
```

**Branch Node** (reuses segments TreeNode)
```go
type BranchNodeConfig struct {
    Paths []BranchPath `json:"paths"`
    DefaultPathID string `json:"default_path_id"`
}

type BranchPath struct {
    ID         string           `json:"id"`
    Name       string           `json:"name"`
    Conditions *domain.TreeNode `json:"conditions"` // Reuse segments TreeNode
    NextNodeID string           `json:"next_node_id"`
}
```

**Filter Node** (reuses segments TreeNode)
```go
type FilterNodeConfig struct {
    Conditions     *domain.TreeNode `json:"conditions"` // Reuse segments TreeNode
    ContinueNodeID string           `json:"continue_node_id"`
    ExitNodeID     string           `json:"exit_node_id"`
}
```

**Add to List Node**
```go
type AddToListNodeConfig struct {
    ListID string                 `json:"list_id"`
    Status string                 `json:"status"` // "subscribed", "pending"
    Metadata map[string]interface{} `json:"metadata"`
}
```

**Remove from List Node**
```go
type RemoveFromListNodeConfig struct {
    ListID string `json:"list_id"`
}
```

---

## Database Schema

```sql
-- Automations table
CREATE TABLE automations (
    id VARCHAR(32) PRIMARY KEY,
    workspace_id VARCHAR(32) NOT NULL REFERENCES workspaces(id),
    name VARCHAR(255) NOT NULL,
    status VARCHAR(20) DEFAULT 'draft',
    list_id VARCHAR(32) NOT NULL REFERENCES lists(id),  -- Required list association
    trigger_config JSONB NOT NULL,
    root_node_id VARCHAR(32),
    stats JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_automations_workspace_status 
    ON automations(workspace_id, status);
    
CREATE INDEX idx_automations_list
    ON automations(list_id, status);

-- Nodes table
CREATE TABLE automation_nodes (
    id VARCHAR(32) PRIMARY KEY,
    automation_id VARCHAR(32) NOT NULL REFERENCES automations(id) ON DELETE CASCADE,
    type VARCHAR(50) NOT NULL,
    config JSONB NOT NULL,
    next_node_id VARCHAR(32),
    position JSONB DEFAULT '{"x":0,"y":0}',
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_automation_nodes_automation 
    ON automation_nodes(automation_id);

-- Contact enrollment tracking
CREATE TABLE contact_automations (
    id VARCHAR(32) PRIMARY KEY,
    automation_id VARCHAR(32) NOT NULL REFERENCES automations(id),
    contact_email VARCHAR(255) NOT NULL,
    current_node_id VARCHAR(32),
    status VARCHAR(20) DEFAULT 'active',
    entered_at TIMESTAMPTZ DEFAULT NOW(),
    scheduled_at TIMESTAMPTZ,
    context JSONB DEFAULT '{}',
    retry_count INTEGER DEFAULT 0,
    last_error TEXT,
    last_retry_at TIMESTAMPTZ,
    max_retries INTEGER DEFAULT 3,
    UNIQUE(automation_id, contact_email, entered_at)
);

CREATE INDEX idx_contact_automations_scheduled
    ON contact_automations(scheduled_at)
    WHERE status = 'active' AND scheduled_at IS NOT NULL;

CREATE INDEX idx_contact_automations_automation
    ON contact_automations(automation_id, status);

-- Journey tracking for troubleshooting  
CREATE TABLE automation_journey_log (
    id VARCHAR(32) PRIMARY KEY,
    contact_automation_id VARCHAR(32) NOT NULL REFERENCES contact_automations(id) ON DELETE CASCADE,
    node_id VARCHAR(32) NOT NULL,
    node_type VARCHAR(50) NOT NULL,
    action VARCHAR(20) NOT NULL,
    entered_at TIMESTAMPTZ DEFAULT NOW(),
    completed_at TIMESTAMPTZ,
    duration_ms INTEGER,
    metadata JSONB DEFAULT '{}',
    error TEXT
);

CREATE INDEX idx_journey_log_contact_automation 
    ON automation_journey_log(contact_automation_id, entered_at DESC);

-- Trigger tracking to prevent duplicates (for "once" frequency)
CREATE TABLE automation_trigger_log (
    id VARCHAR(32) PRIMARY KEY,
    automation_id VARCHAR(32) NOT NULL REFERENCES automations(id),
    contact_email VARCHAR(255) NOT NULL,
    triggered_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(automation_id, contact_email)  -- One entry per contact per automation
);

CREATE INDEX idx_trigger_log_automation
    ON automation_trigger_log(automation_id, triggered_at DESC);

CREATE INDEX idx_trigger_log_contact
    ON automation_trigger_log(contact_email, automation_id);
```

---

## Backend Implementation Architecture

### Domain Layer

**Repository Interfaces**
```go
type AutomationRepository interface {
    Create(ctx context.Context, workspaceID string, automation *Automation) error
    GetByID(ctx context.Context, workspaceID, id string) (*Automation, error)
    List(ctx context.Context, workspaceID string, filters AutomationFilters) ([]*Automation, error)
    Update(ctx context.Context, workspaceID string, automation *Automation) error
    Delete(ctx context.Context, workspaceID, id string) error
    
    // List-based automation queries
    GetByListID(ctx context.Context, workspaceID, listID string, status AutomationStatus) ([]*Automation, error)
    GetLiveAutomationsByEventKind(ctx context.Context, workspaceID, eventKind string) ([]*Automation, error)
    
    // Node management
    CreateNode(ctx context.Context, workspaceID string, node *AutomationNode) error
    UpdateNode(ctx context.Context, workspaceID string, node *AutomationNode) error
    DeleteNode(ctx context.Context, workspaceID, nodeID string) error
    GetNodes(ctx context.Context, workspaceID, automationID string) ([]*AutomationNode, error)
    GetNode(ctx context.Context, workspaceID, nodeID string) (*AutomationNode, error)
}

// Add to existing ListRepository interface
type ListRepository interface {
    // Existing methods...
    
    // Subscription status checking
    IsContactSubscribed(ctx context.Context, workspaceID, listID, email string) (bool, error)
    GetContactSubscriptionStatus(ctx context.Context, workspaceID, listID, email string) (string, error) // "subscribed", "pending", "unsubscribed"
}

type AutomationExecutionRepository interface {
    // Contact enrollment
    EnrollContact(ctx context.Context, enrollment *ContactAutomation) error
    GetContactAutomation(ctx context.Context, workspaceID, automationID, contactEmail string) (*ContactAutomation, error)
    UpdateContactAutomation(ctx context.Context, workspaceID string, enrollment *ContactAutomation) error
    
    // Journey tracking
    LogJourneyEntry(ctx context.Context, workspaceID string, entry *AutomationJourneyEntry) error
    GetJourneyLog(ctx context.Context, workspaceID, contactAutomationID string) ([]*AutomationJourneyEntry, error)
    
    // Processing queue
    GetScheduledContacts(ctx context.Context, workspaceID string, limit int) ([]*ContactAutomation, error)
    
    // Trigger deduplication
    RecordTriggerExecution(ctx context.Context, workspaceID, automationID, contactEmail, timelineEntryID string) error
    HasTriggerFired(ctx context.Context, workspaceID, automationID, contactEmail, timelineEntryID string) (bool, error)
}
```

**Service Interface**
```go
type AutomationService interface {
    // CRUD operations
    CreateAutomation(ctx context.Context, req *CreateAutomationRequest) (*Automation, error)
    GetAutomation(ctx context.Context, workspaceID, id string) (*Automation, error)
    ListAutomations(ctx context.Context, workspaceID string) ([]*Automation, error)
    UpdateAutomation(ctx context.Context, req *UpdateAutomationRequest) (*Automation, error)
    DeleteAutomation(ctx context.Context, workspaceID, id string) error
    
    // Status management
    ActivateAutomation(ctx context.Context, workspaceID, id string) error
    PauseAutomation(ctx context.Context, workspaceID, id string) error
    
    // Node management
    CreateNode(ctx context.Context, req *CreateNodeRequest) (*AutomationNode, error)
    UpdateNode(ctx context.Context, req *UpdateNodeRequest) (*AutomationNode, error)
    DeleteNode(ctx context.Context, workspaceID, nodeID string) error
    
    // Journey tracking
    GetContactJourney(ctx context.Context, workspaceID, automationID, contactEmail string) (*ContactJourneyResponse, error)
    GetAutomationErrors(ctx context.Context, workspaceID, automationID string) ([]*AutomationError, error)
    
    // Testing
    TestTriggerConditions(ctx context.Context, req *TestTriggerRequest) (*TestTriggerResponse, error)
    TestNodeConditions(ctx context.Context, req *TestNodeConditionsRequest) (*TestNodeConditionsResponse, error)
}
```

### Processing Engine

**Timeline Event Processor**
```go
type TimelineEventProcessor struct {
    automationRepo          AutomationRepository
    executionRepo           AutomationExecutionRepository
    listRepo                domain.ListRepository      // For subscription checks
    segmentQueryBuilder     *service.QueryBuilder      // Reuse from segments
    logger                  logger.Logger
}

// ProcessTimelineEvent is called whenever a contact timeline entry is created
func (p *TimelineEventProcessor) ProcessTimelineEvent(ctx context.Context, workspaceID string, entry *domain.ContactTimelineEntry) error {
    // 1. Find all live automations that could be triggered by this event
    automations, err := p.findMatchingAutomations(ctx, workspaceID, entry.Kind)
    if err != nil {
        return err
    }
    
    for _, automation := range automations {
        // 2. Check trigger conditions using segments QueryBuilder
        matches, err := p.evaluateTriggerConditions(ctx, workspaceID, automation, entry)
        if err != nil {
            p.logger.WithError(err).Error("Failed to evaluate trigger conditions")
            continue
        }
        
        if matches {
            // 3. Check if contact is subscribed to automation's list
            isSubscribed, err := p.listRepo.IsContactSubscribed(ctx, workspaceID, automation.ListID, entry.Email)
            if err != nil {
                p.logger.WithError(err).Error("Failed to check list subscription")
                continue
            }
            
            if !isSubscribed {
                p.logger.WithFields(map[string]interface{}{
                    "contact_email": entry.Email,
                    "automation_id": automation.ID,
                    "list_id":       automation.ListID,
                }).Debug("Contact not subscribed to automation list, skipping enrollment")
                continue
            }
            
            // 4. Check frequency (once vs every_time) and deduplication
            shouldEnroll, err := p.shouldEnrollContact(ctx, workspaceID, automation, entry)
            if err != nil {
                p.logger.WithError(err).Error("Failed to check enrollment eligibility") 
                continue
            }
            
            if shouldEnroll {
                // 5. Enroll contact in automation
                err = p.enrollContact(ctx, workspaceID, automation, entry)
                if err != nil {
                    p.logger.WithError(err).Error("Failed to enroll contact in automation")
                }
            }
        }
    }
    
    return nil
}
```

**Automation Execution Worker**
```go
type AutomationExecutionWorker struct {
    executionRepo       AutomationExecutionRepository
    automationRepo      AutomationRepository
    contactRepo         domain.ContactRepository
    templateRepo        domain.TemplateRepository
    listRepo            domain.ListRepository
    mailer             mailer.Mailer
    segmentQueryBuilder *service.QueryBuilder
    logger             logger.Logger
}

// ProcessScheduledContacts processes contacts scheduled for automation execution
func (w *AutomationExecutionWorker) ProcessScheduledContacts(ctx context.Context, workspaceID string) error {
    contacts, err := w.executionRepo.GetScheduledContacts(ctx, workspaceID, 100)
    if err != nil {
        return err
    }
    
    for _, contactAutomation := range contacts {
        err := w.processContact(ctx, workspaceID, contactAutomation)
        if err != nil {
            w.logger.WithError(err).WithField("contact_automation_id", contactAutomation.ID).Error("Failed to process contact")
        }
    }
    
    return nil
}

func (w *AutomationExecutionWorker) processContact(ctx context.Context, workspaceID string, contactAutomation *ContactAutomation) error {
    // 1. Get current node
    node, err := w.getNode(ctx, workspaceID, contactAutomation.CurrentNodeID)
    if err != nil {
        return err
    }
    
    // 2. Log journey entry
    journeyEntry := &AutomationJourneyEntry{
        ID:                  uuid.New().String(),
        ContactAutomationID: contactAutomation.ID,
        NodeID:             node.ID,
        NodeType:           node.Type,
        Action:             JourneyActionProcessing,
        EnteredAt:          time.Now(),
    }
    
    // 3. For email nodes, verify list subscription before sending
    if node.Type == NodeTypeEmail {
        automation, err := w.automationRepo.GetByID(ctx, workspaceID, contactAutomation.AutomationID)
        if err != nil {
            return fmt.Errorf("failed to get automation: %w", err)
        }
        
        isSubscribed, err := w.listRepo.IsContactSubscribed(ctx, workspaceID, automation.ListID, contactAutomation.ContactEmail)
        if err != nil {
            return fmt.Errorf("failed to check list subscription: %w", err)
        }
        
        if !isSubscribed {
            // Contact unsubscribed from list - exit automation gracefully
            journeyEntry.Action = JourneyActionSkipped
            journeyEntry.Metadata = map[string]interface{}{
                "reason": "unsubscribed_from_list",
                "list_id": automation.ListID,
            }
            
            if logErr := w.executionRepo.LogJourneyEntry(ctx, workspaceID, journeyEntry); logErr != nil {
                w.logger.WithError(logErr).Error("Failed to log journey entry")
            }
            
            return w.exitContactFromAutomation(ctx, workspaceID, contactAutomation, "unsubscribed_from_list")
        }
    }
    
    // 4. Execute node based on type
    nextNodeID, metadata, err := w.executeNode(ctx, workspaceID, node, contactAutomation)
    
    // 5. Update journey entry with results
    journeyEntry.CompletedAt = &time.Time{}
    *journeyEntry.CompletedAt = time.Now()
    journeyEntry.DurationMs = ptr.Int64(journeyEntry.CompletedAt.Sub(journeyEntry.EnteredAt).Milliseconds())
    journeyEntry.Metadata = metadata
    
    if err != nil {
        journeyEntry.Action = JourneyActionFailed
        journeyEntry.Error = ptr.String(err.Error())
    } else {
        journeyEntry.Action = JourneyActionCompleted
    }
    
    // 6. Log the journey entry
    if logErr := w.executionRepo.LogJourneyEntry(ctx, workspaceID, journeyEntry); logErr != nil {
        w.logger.WithError(logErr).Error("Failed to log journey entry")
    }
    
    if err != nil {
        return err
    }
    
    // 7. Advance to next node or complete
    return w.advanceContact(ctx, workspaceID, contactAutomation, nextNodeID)
}
```

---

## API Design

### RPC-Style Endpoints

```go
// Automation CRUD
POST   /api/automation.create
POST   /api/automation.update  
POST   /api/automation.delete
GET    /api/automation.get
GET    /api/automation.list
POST   /api/automation.activate
POST   /api/automation.pause

// Node management
POST   /api/automation.node.create
POST   /api/automation.node.update
POST   /api/automation.node.delete
GET    /api/automation.nodes

// Journey tracking & debugging
GET    /api/automation.journey          // Get contact's journey in automation
GET    /api/automation.errors           // Get failed contacts
GET    /api/automation.stats            // Get automation performance stats

// Testing & validation
POST   /api/automation.trigger.test     // Test trigger conditions against contact
POST   /api/automation.condition.test   // Test TreeNode conditions against contact
POST   /api/automation.validate         // Validate automation configuration
```

### Request/Response Types

**Create Automation**
```go
type CreateAutomationRequest struct {
    WorkspaceID   string                 `json:"workspace_id"`
    Name          string                 `json:"name"`
    ListID        string                 `json:"list_id"`        // Required - automation's target list
    TriggerConfig *TimelineTriggerConfig `json:"trigger_config"`
}

func (r *CreateAutomationRequest) Validate() error {
    if r.WorkspaceID == "" {
        return fmt.Errorf("workspace_id is required")
    }
    if r.Name == "" {
        return fmt.Errorf("name is required")
    }
    if r.ListID == "" {
        return fmt.Errorf("list_id is required")
    }
    if r.TriggerConfig == nil {
        return fmt.Errorf("trigger_config is required")
    }
    return nil
}

type CreateAutomationResponse struct {
    Automation *Automation `json:"automation"`
}
```

**Get Journey**
```go
type GetJourneyRequest struct {
    WorkspaceID   string `json:"workspace_id"`
    AutomationID  string `json:"automation_id"`
    ContactEmail  string `json:"contact_email"`
}

type GetJourneyResponse struct {
    ContactEmail     string                     `json:"contact_email"`
    AutomationName   string                     `json:"automation_name"`
    CurrentStatus    ContactAutomationStatus    `json:"current_status"`
    CurrentNode      *string                    `json:"current_node"`
    EnteredAt        time.Time                  `json:"entered_at"`
    Journey          []*AutomationJourneyEntry  `json:"journey"`
}
```

---

## List-Based Subscription Management

### Automation Service Implementation

**List Validation in Service Layer**
```go
func (s *AutomationService) CreateAutomation(ctx context.Context, req *CreateAutomationRequest) (*Automation, error) {
    // Validate request
    if err := req.Validate(); err != nil {
        return nil, fmt.Errorf("invalid request: %w", err)
    }
    
    // Validate list exists and is active
    list, err := s.listRepo.GetByID(ctx, req.WorkspaceID, req.ListID)
    if err != nil {
        return nil, fmt.Errorf("invalid list_id: %w", err)
    }
    
    if list.DeletedAt != nil {
        return nil, fmt.Errorf("cannot create automation for deleted list")
    }
    
    // Create automation with list association
    automation := &Automation{
        ID:          uuid.New().String(),
        WorkspaceID: req.WorkspaceID,
        Name:        req.Name,
        ListID:      req.ListID,
        Status:      AutomationStatusDraft,
        Trigger:     req.TriggerConfig,
        Stats:       &AutomationStats{},
        CreatedAt:   time.Now(),
        UpdatedAt:   time.Now(),
    }
    
    err = s.automationRepo.Create(ctx, req.WorkspaceID, automation)
    if err != nil {
        return nil, fmt.Errorf("failed to create automation: %w", err)
    }
    
    return automation, nil
}
```

### Unsubscribe Handling

**List-Specific Unsubscribe Endpoints**
```go
// Unsubscribe from specific list
GET /unsubscribe/{listID}?email={email}&token={token}
POST /api/unsubscribe.list

type UnsubscribeFromListRequest struct {
    ListID string `json:"list_id"`
    Email  string `json:"email"`
    Token  string `json:"token"`  // Unsubscribe token for security
}
```

**Unsubscribe Handler Implementation**
```go
func (h *UnsubscribeHandler) HandleListUnsubscribe(ctx context.Context, req *UnsubscribeFromListRequest) error {
    // 1. Validate unsubscribe token
    if !h.validateUnsubscribeToken(req.Token, req.Email, req.ListID) {
        return fmt.Errorf("invalid unsubscribe token")
    }
    
    // 2. Remove contact from list
    err := h.listService.RemoveContact(ctx, h.workspaceID, req.ListID, req.Email)
    if err != nil {
        return fmt.Errorf("failed to remove contact from list: %w", err)
    }
    
    // 3. Exit contact from all automations using this list
    err = h.automationService.ExitContactFromListAutomations(ctx, h.workspaceID, req.ListID, req.Email)
    if err != nil {
        h.logger.WithError(err).Error("Failed to exit contact from list automations")
        // Don't fail the unsubscribe for this - log and continue
    }
    
    // 4. Create timeline entry for audit trail
    timelineEntry := &domain.ContactTimelineEntry{
        ID:         uuid.New().String(),
        Email:      req.Email,
        Operation:  "delete",
        EntityType: "contact_list",
        Kind:       "delete_contact_list",
        EntityID:   &req.ListID,
        Changes: map[string]interface{}{
            "list_id": req.ListID,
            "reason":  "unsubscribe_link",
            "method":  "email_unsubscribe",
        },
        CreatedAt: time.Now(),
    }
    
    return h.contactTimelineService.Create(ctx, h.workspaceID, timelineEntry)
}
```

**Exit From List Automations**
```go
func (s *AutomationService) ExitContactFromListAutomations(ctx context.Context, workspaceID, listID, contactEmail string) error {
    // Find all active automations for this list
    automations, err := s.automationRepo.GetByListID(ctx, workspaceID, listID, AutomationStatusLive)
    if err != nil {
        return err
    }
    
    // Exit contact from each automation
    for _, automation := range automations {
        contactAutomation, err := s.executionRepo.GetContactAutomation(ctx, workspaceID, automation.ID, contactEmail)
        if err != nil {
            if errors.Is(err, domain.ErrNotFound) {
                continue // Contact not in this automation
            }
            return err
        }
        
        if contactAutomation.Status == ContactAutomationStatusActive {
            // Update status to exited
            contactAutomation.Status = ContactAutomationStatusExited
            contactAutomation.ScheduledAt = nil
            
            err = s.executionRepo.UpdateContactAutomation(ctx, workspaceID, contactAutomation)
            if err != nil {
                s.logger.WithError(err).WithFields(map[string]interface{}{
                    "automation_id": automation.ID,
                    "contact_email": contactEmail,
                }).Error("Failed to exit contact from automation")
                continue
            }
            
            // Log journey entry
            journeyEntry := &AutomationJourneyEntry{
                ID:                  uuid.New().String(),
                ContactAutomationID: contactAutomation.ID,
                NodeID:             contactAutomation.CurrentNodeID,
                NodeType:           "exit",
                Action:             JourneyActionCompleted,
                EnteredAt:          time.Now(),
                CompletedAt:        &time.Time{},
                Metadata: map[string]interface{}{
                    "exit_reason": "unsubscribed_from_list",
                    "list_id":     listID,
                },
            }
            *journeyEntry.CompletedAt = time.Now()
            
            if logErr := s.executionRepo.LogJourneyEntry(ctx, workspaceID, journeyEntry); logErr != nil {
                s.logger.WithError(logErr).Error("Failed to log exit journey entry")
            }
        }
    }
    
    return nil
}
```

### Email Integration

**Unsubscribe Link Generation**
```go
func (w *AutomationExecutionWorker) executeEmailNode(ctx context.Context, node *AutomationNode, contactAutomation *ContactAutomation) error {
    // Get automation to access list_id
    automation, err := w.automationRepo.GetByID(ctx, w.workspaceID, contactAutomation.AutomationID)
    if err != nil {
        return err
    }
    
    // Generate list-specific unsubscribe link
    unsubscribeToken := generateUnsubscribeToken(contactAutomation.ContactEmail, automation.ListID, w.secretKey)
    unsubscribeURL := fmt.Sprintf("%s/unsubscribe/%s?email=%s&token=%s",
        w.baseURL, automation.ListID, contactAutomation.ContactEmail, unsubscribeToken)
    
    // Send email with unsubscribe link
    emailConfig := node.Config.(*EmailNodeConfig)
    return w.sendEmailWithUnsubscribe(ctx, emailConfig, contactAutomation, automation.ListID, unsubscribeURL)
}
```

---

## Integration with Existing Systems

### Timeline Event Hook
```go
// Add to contact service when timeline entries are created
func (s *ContactService) createTimelineEntry(ctx context.Context, workspaceID string, entry *domain.ContactTimelineEntry) error {
    // Existing timeline creation logic...
    err := s.timelineRepo.Create(ctx, workspaceID, entry)
    if err != nil {
        return err
    }
    
    // NEW: Trigger automation processing
    if s.automationProcessor != nil {
        go func() {
            if err := s.automationProcessor.ProcessTimelineEvent(context.Background(), workspaceID, entry); err != nil {
                s.logger.WithError(err).Error("Failed to process timeline event for automations")
            }
        }()
    }
    
    return nil
}
```

### Segment Event Integration
```go
// Add to segment service when membership changes
func (s *SegmentService) updateMembership(ctx context.Context, workspaceID, segmentID, contactEmail string, action string) error {
    // Existing membership update logic...
    
    // NEW: Create timeline entry for segment changes
    timelineEntry := &domain.ContactTimelineEntry{
        ID:         uuid.New().String(),
        Email:      contactEmail,
        Operation:  action, // "insert" or "delete"
        EntityType: "segment_membership",
        Kind:       fmt.Sprintf("%s_segment", action), // "insert_segment" or "delete_segment"
        EntityID:   &segmentID,
        Changes:    map[string]interface{}{"segment_id": segmentID},
        CreatedAt:  time.Now(),
    }
    
    return s.contactTimelineService.Create(ctx, workspaceID, timelineEntry)
}
```

---

## Processing Architecture

### Per-Automation Database Triggers (Zero Latency)

Instead of queue-based processing, we use **per-automation PostgreSQL triggers** for instant enrollment:

1. **Timeline Event Created** → Per-automation trigger fires (WHEN clause filters by event kind)
2. **Trigger Function** → Calls `automation_enroll_contact()` synchronously
3. **Enrollment Function** → Checks list subscription, frequency, then inserts into `contact_automations`
4. **Transaction Commits** → Enrollment is atomic with the triggering event (zero latency)
5. **Execution Worker** → Polls `scheduled_at <= now` records for node execution
6. **Node Execution** → Execute current node, log journey, advance to next

**Key Benefits:**
- Zero latency enrollment (same transaction as triggering event)
- No queue table overhead
- Efficient PostgreSQL WHEN clause filtering
- Transactional consistency

### Trigger Management

When an automation is **activated** (draft → live):
```sql
-- Create trigger function for this automation
CREATE OR REPLACE FUNCTION automation_trigger_{automation_id}()
RETURNS TRIGGER AS $$
BEGIN
    PERFORM automation_enroll_contact(
        '{automation_id}',
        NEW.email,
        '{root_node_id}',
        '{list_id}',
        '{frequency}'
    );
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Create trigger on contact_timeline
CREATE TRIGGER automation_trigger_{automation_id}
AFTER INSERT ON contact_timeline
FOR EACH ROW
WHEN (NEW.kind = '{event_kind}')  -- or IN ('kind1', 'kind2') for multiple
EXECUTE FUNCTION automation_trigger_{automation_id}();
```

When an automation is **paused** or **deleted**:
```sql
DROP TRIGGER IF EXISTS automation_trigger_{automation_id} ON contact_timeline;
DROP FUNCTION IF EXISTS automation_trigger_{automation_id}();
```

### Error Handling & Retries
- Failed nodes log errors in journey with retry count
- Exponential backoff for retries (1min, 5min, 15min)
- Max 3 retries before marking contact as failed
- Dead letter queue for manual intervention

### Performance Considerations  
- Timeline event processing is async (goroutine)
- Worker uses `FOR UPDATE SKIP LOCKED` for concurrency
- Journey log uses separate table for write performance
- Trigger deduplication prevents infinite loops

---

## Database Migration Guide

Based on the existing migration system, here's how to create the automation tables migration:

### Step 1: Update Version
Update `config/config.go`:
```go
const VERSION = "20.0"  // Increment from current 19.x
```

### Step 2: Create Migration File
Create `internal/migrations/v20.go`:
```go
package migrations

import (
    "context"
    "fmt"

    "github.com/Notifuse/notifuse/config"
    "github.com/Notifuse/notifuse/internal/domain"
)

// V20Migration adds automation system tables
type V20Migration struct{}

func (m *V20Migration) GetMajorVersion() float64 {
    return 20.0
}

func (m *V20Migration) HasSystemUpdate() bool {
    return false
}

func (m *V20Migration) HasWorkspaceUpdate() bool {
    return true
}

func (m *V20Migration) ShouldRestartServer() bool {
    return false
}

func (m *V20Migration) UpdateSystem(ctx context.Context, config *config.Config, db DBExecutor) error {
    // No system updates needed
    return nil
}

func (m *V20Migration) UpdateWorkspace(ctx context.Context, config *config.Config, workspace *domain.Workspace, db DBExecutor) error {
    // Create automations table
    _, err := db.ExecContext(ctx, `
        CREATE TABLE IF NOT EXISTS automations (
            id VARCHAR(32) PRIMARY KEY,
            workspace_id VARCHAR(32) NOT NULL REFERENCES workspaces(id),
            name VARCHAR(255) NOT NULL,
            status VARCHAR(20) DEFAULT 'draft',
            list_id VARCHAR(32) NOT NULL REFERENCES lists(id),
            trigger_config JSONB NOT NULL,
            root_node_id VARCHAR(32),
            stats JSONB DEFAULT '{}',
            created_at TIMESTAMPTZ DEFAULT NOW(),
            updated_at TIMESTAMPTZ DEFAULT NOW()
        )
    `)
    if err != nil {
        return fmt.Errorf("failed to create automations table: %w", err)
    }

    // Create indexes
    _, err = db.ExecContext(ctx, `
        CREATE INDEX IF NOT EXISTS idx_automations_workspace_status 
            ON automations(workspace_id, status);
        
        CREATE INDEX IF NOT EXISTS idx_automations_list
            ON automations(list_id, status);
    `)
    if err != nil {
        return fmt.Errorf("failed to create automations indexes: %w", err)
    }

    // Create automation_nodes table
    _, err = db.ExecContext(ctx, `
        CREATE TABLE IF NOT EXISTS automation_nodes (
            id VARCHAR(32) PRIMARY KEY,
            automation_id VARCHAR(32) NOT NULL REFERENCES automations(id) ON DELETE CASCADE,
            type VARCHAR(50) NOT NULL,
            config JSONB NOT NULL,
            next_node_id VARCHAR(32),
            position JSONB DEFAULT '{"x":0,"y":0}',
            created_at TIMESTAMPTZ DEFAULT NOW()
        )
    `)
    if err != nil {
        return fmt.Errorf("failed to create automation_nodes table: %w", err)
    }

    _, err = db.ExecContext(ctx, `
        CREATE INDEX IF NOT EXISTS idx_automation_nodes_automation 
            ON automation_nodes(automation_id);
    `)
    if err != nil {
        return fmt.Errorf("failed to create automation_nodes index: %w", err)
    }

    // Create contact_automations table
    _, err = db.ExecContext(ctx, `
        CREATE TABLE IF NOT EXISTS contact_automations (
            id VARCHAR(32) PRIMARY KEY,
            automation_id VARCHAR(32) NOT NULL REFERENCES automations(id),
            contact_email VARCHAR(255) NOT NULL,
            current_node_id VARCHAR(32),
            status VARCHAR(20) DEFAULT 'active',
            entered_at TIMESTAMPTZ DEFAULT NOW(),
            scheduled_at TIMESTAMPTZ,
            context JSONB DEFAULT '{}',
            retry_count INTEGER DEFAULT 0,
            last_error TEXT,
            last_retry_at TIMESTAMPTZ,
            max_retries INTEGER DEFAULT 3,
            UNIQUE(automation_id, contact_email, entered_at)
        )
    `)
    if err != nil {
        return fmt.Errorf("failed to create contact_automations table: %w", err)
    }

    _, err = db.ExecContext(ctx, `
        CREATE INDEX IF NOT EXISTS idx_contact_automations_scheduled
            ON contact_automations(scheduled_at)
            WHERE status = 'active' AND scheduled_at IS NOT NULL;
        
        CREATE INDEX IF NOT EXISTS idx_contact_automations_automation
            ON contact_automations(automation_id, status);
    `)
    if err != nil {
        return fmt.Errorf("failed to create contact_automations indexes: %w", err)
    }

    // Create automation_journey_log table
    _, err = db.ExecContext(ctx, `
        CREATE TABLE IF NOT EXISTS automation_journey_log (
            id VARCHAR(32) PRIMARY KEY,
            contact_automation_id VARCHAR(32) NOT NULL REFERENCES contact_automations(id) ON DELETE CASCADE,
            node_id VARCHAR(32) NOT NULL,
            node_type VARCHAR(50) NOT NULL,
            action VARCHAR(20) NOT NULL,
            entered_at TIMESTAMPTZ DEFAULT NOW(),
            completed_at TIMESTAMPTZ,
            duration_ms INTEGER,
            metadata JSONB DEFAULT '{}',
            error TEXT
        )
    `)
    if err != nil {
        return fmt.Errorf("failed to create automation_journey_log table: %w", err)
    }

    _, err = db.ExecContext(ctx, `
        CREATE INDEX IF NOT EXISTS idx_journey_log_contact_automation 
            ON automation_journey_log(contact_automation_id, entered_at DESC);
    `)
    if err != nil {
        return fmt.Errorf("failed to create journey_log index: %w", err)
    }

    // Create automation_trigger_log table for deduplication
    _, err = db.ExecContext(ctx, `
        CREATE TABLE IF NOT EXISTS automation_trigger_log (
            id VARCHAR(32) PRIMARY KEY,
            automation_id VARCHAR(32) NOT NULL REFERENCES automations(id),
            contact_email VARCHAR(255) NOT NULL,
            timeline_entry_id VARCHAR(32) NOT NULL,
            triggered_at TIMESTAMPTZ DEFAULT NOW(),
            UNIQUE(automation_id, contact_email, timeline_entry_id)
        )
    `)
    if err != nil {
        return fmt.Errorf("failed to create automation_trigger_log table: %w", err)
    }

    _, err = db.ExecContext(ctx, `
        CREATE INDEX IF NOT EXISTS idx_trigger_log_automation
            ON automation_trigger_log(automation_id, triggered_at DESC);
    `)
    if err != nil {
        return fmt.Errorf("failed to create trigger_log index: %w", err)
    }

    return nil
}

func init() {
    Register(&V20Migration{})
}
```

### Step 3: Register Migration
The `init()` function automatically registers the migration with the global registry.

### Step 4: Test Migration
Create `internal/migrations/v20_test.go` following existing test patterns.

### Step 5: Add Enrollment Function
Add to the v20 migration the `automation_enroll_contact()` function that handles:
- List subscription check
- Frequency deduplication (once vs every_time)
- Contact enrollment into `contact_automations`
- Journey log entry creation

```sql
CREATE OR REPLACE FUNCTION automation_enroll_contact(
    p_automation_id VARCHAR(32),
    p_contact_email VARCHAR(255),
    p_root_node_id VARCHAR(32),
    p_list_id VARCHAR(32),
    p_frequency VARCHAR(20)
) RETURNS VOID AS $$
DECLARE
    v_is_subscribed BOOLEAN;
    v_already_triggered BOOLEAN;
BEGIN
    -- Check list subscription
    SELECT EXISTS(
        SELECT 1 FROM contact_lists
        WHERE email = p_contact_email
        AND list_id = p_list_id
        AND status = 'active'
        AND deleted_at IS NULL
    ) INTO v_is_subscribed;

    IF NOT v_is_subscribed THEN
        RETURN;
    END IF;

    -- For "once" frequency, check deduplication
    IF p_frequency = 'once' THEN
        SELECT EXISTS(
            SELECT 1 FROM automation_trigger_log
            WHERE automation_id = p_automation_id
            AND contact_email = p_contact_email
        ) INTO v_already_triggered;

        IF v_already_triggered THEN
            RETURN;
        END IF;

        INSERT INTO automation_trigger_log (id, automation_id, contact_email, triggered_at)
        VALUES (gen_random_uuid()::text, p_automation_id, p_contact_email, NOW())
        ON CONFLICT (automation_id, contact_email) DO NOTHING;
    END IF;

    -- Enroll contact
    INSERT INTO contact_automations (
        id, automation_id, contact_email, current_node_id,
        status, entered_at, scheduled_at
    ) VALUES (
        gen_random_uuid()::text, p_automation_id, p_contact_email,
        p_root_node_id, 'active', NOW(), NOW()
    );
END;
$$ LANGUAGE plpgsql;
```

---

## Error Handling & Retry Logic

### Retry Configuration
```go
type AutomationConfig struct {
    DefaultMaxRetries    int           `json:"default_max_retries"`    // Default: 3
    RetryBackoffBase     time.Duration `json:"retry_backoff_base"`     // Default: 1 minute
    RetryBackoffMax      time.Duration `json:"retry_backoff_max"`      // Default: 15 minutes
    ProcessingTimeout    time.Duration `json:"processing_timeout"`     // Default: 30 seconds
    DeadLetterThreshold  int           `json:"dead_letter_threshold"`  // Default: 5 consecutive failures
}
```

### Retry Logic Implementation
```go
func (w *AutomationExecutionWorker) handleNodeExecutionError(ctx context.Context, contactAutomation *ContactAutomation, err error) error {
    contactAutomation.RetryCount++
    contactAutomation.LastError = ptr.String(err.Error())
    contactAutomation.LastRetryAt = ptr.Time(time.Now())
    
    if contactAutomation.RetryCount >= contactAutomation.MaxRetries {
        // Max retries exceeded - mark as failed
        contactAutomation.Status = ContactAutomationStatusFailed
        contactAutomation.ScheduledAt = nil
        
        w.logger.WithFields(map[string]interface{}{
            "contact_automation_id": contactAutomation.ID,
            "retry_count":          contactAutomation.RetryCount,
            "error":               err.Error(),
        }).Error("Contact automation failed after max retries")
        
        return w.executionRepo.UpdateContactAutomation(ctx, w.workspaceID, contactAutomation)
    }
    
    // Calculate exponential backoff
    backoffDuration := w.calculateBackoff(contactAutomation.RetryCount)
    contactAutomation.ScheduledAt = ptr.Time(time.Now().Add(backoffDuration))
    
    w.logger.WithFields(map[string]interface{}{
        "contact_automation_id": contactAutomation.ID,
        "retry_count":          contactAutomation.RetryCount,
        "next_retry_at":        contactAutomation.ScheduledAt,
        "error":               err.Error(),
    }).Warn("Scheduling automation retry")
    
    return w.executionRepo.UpdateContactAutomation(ctx, w.workspaceID, contactAutomation)
}

func (w *AutomationExecutionWorker) calculateBackoff(retryCount int) time.Duration {
    // Exponential backoff: 1min, 2min, 4min, 8min, max 15min
    backoff := w.config.RetryBackoffBase * time.Duration(1<<(retryCount-1))
    if backoff > w.config.RetryBackoffMax {
        backoff = w.config.RetryBackoffMax
    }
    return backoff
}
```

---

## Implementation Phases

### **Phase 1 - Foundation (MVP)**
**Goal**: Basic automation system with essential functionality

1. **Database Migration (v20.0)**
   - Create all automation tables
   - Add retry tracking fields
   - Migrate message_history timeline events to semantic naming (email.sent, email.opened, etc.)

2. **Core Domain Models**
   - `Automation`, `AutomationNode`, `ContactAutomation` entities
   - Repository interfaces with all required methods
   - Basic validation logic

3. **Essential Repositories**
   - `AutomationRepository` with list-based queries
   - `AutomationExecutionRepository` with retry logic
   - `ListRepository.IsContactSubscribed()` method

4. **Basic CRUD API** 
   - Create/update/delete automations with list validation
   - Simple node management (delay, email, exit only)
   - List association validation

5. **Timeline Hook Integration**
   - Basic timeline event processor
   - Simple trigger matching (no complex conditions)
   - Contact enrollment with subscription checks

6. **Simple Execution Worker**
   - Basic node execution (delay, email, exit)
   - Linear flow processing (no branching)
   - Basic error handling and retries

7. **List-Based Unsubscribe**
   - List-specific unsubscribe URLs in emails
   - Automatic automation exit on unsubscribe

**Phase 1 Limitations**:
- No branch/filter nodes (linear flows only)
- No complex TreeNode conditions (simple trigger matching)
- No journey debugging (basic logging only)
- No advanced retry strategies

---

### **Phase 2 - Advanced Features (Defer)**
**Goal**: Full-featured automation system

1. **Complex Node Types**
   - Branch nodes with TreeNode condition evaluation
   - Filter nodes with segments QueryBuilder integration
   - Add/remove from list action nodes

2. **Advanced Trigger System**
   - Full TreeNode condition support in triggers
   - Complex timeline event filtering
   - Segment enter/exit trigger optimization

3. **Journey Debugging APIs**
   - Contact journey tracking endpoints
   - Error debugging and analytics
   - Performance metrics and monitoring

4. **Advanced Retry Logic**
   - Dead letter queue for manual intervention
   - Retry policies per node type
   - Circuit breaker for failing external services

5. **Visual Editor Support**
   - Node positioning and canvas data
   - Flow validation APIs
   - Template preview and testing

**Why Defer Phase 2**:
- TreeNode condition evaluation is complex
- Journey debugging adds significant API surface
- Visual editor requires frontend development
- Advanced retry logic needs operational tooling

This phased approach ensures a **working automation system quickly** while building toward enterprise features systematically.

---

## TDD Implementation Order

Each component follows **Red-Green-Refactor**:
1. Write failing test(s) first
2. Implement minimal code to pass
3. Refactor if needed
4. Run tests to verify

### Step 1: Migration Tests → Migration
```bash
# RED: Write v20_test.go first
# GREEN: Implement v20.go to pass
go test ./internal/migrations/... -run "V20" -v
```

### Step 2: Domain Tests → Domain Models
```bash
# RED: Write automation_test.go first
# GREEN: Implement automation.go to pass
go test ./internal/domain/... -run "Automation" -v
```

### Step 3: Repository Tests → Repository
```bash
# RED: Write automation_postgres_test.go first
# GREEN: Implement automation_postgres.go to pass
go test ./internal/repository/... -run "Automation" -v
```

### Step 4: Trigger Generator Tests → Generator
```bash
# RED: Write automation_trigger_generator_test.go first
# GREEN: Implement automation_trigger_generator.go to pass
go test ./internal/service/... -run "TriggerGenerator" -v
```

### Step 5: Execution Processor Tests → Processor
```bash
# RED: Write automation_execution_processor_test.go first
# GREEN: Implement automation_execution_processor.go to pass
go test ./internal/service/... -run "ExecutionProcessor" -v
```

### Step 6: Service Tests → Service
```bash
# RED: Write automation_service_test.go first
# GREEN: Implement automation_service.go to pass
go test ./internal/service/... -run "AutomationService" -v
```

### Step 7: Handler Tests → Handlers
```bash
# RED: Write automation_handler_test.go first
# GREEN: Implement automation_handler.go to pass
go test ./internal/http/... -run "Automation" -v
```

### Step 8: Integration
- Update app.go to wire everything together
- Run full test suite: `make test-unit`