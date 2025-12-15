package domain

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"time"
)

//go:generate mockgen -destination mocks/mock_automation_repository.go -package mocks github.com/Notifuse/notifuse/internal/domain AutomationRepository

// AutomationStatus represents the status of an automation
type AutomationStatus string

const (
	AutomationStatusDraft  AutomationStatus = "draft"
	AutomationStatusLive   AutomationStatus = "live"
	AutomationStatusPaused AutomationStatus = "paused"
)

// IsValid checks if the automation status is valid
func (s AutomationStatus) IsValid() bool {
	switch s {
	case AutomationStatusDraft, AutomationStatusLive, AutomationStatusPaused:
		return true
	default:
		return false
	}
}

// TriggerFrequency defines how often an automation should trigger for a contact
type TriggerFrequency string

const (
	TriggerFrequencyOnce      TriggerFrequency = "once"       // Only trigger on first occurrence
	TriggerFrequencyEveryTime TriggerFrequency = "every_time" // Trigger on each occurrence
)

// IsValid checks if the trigger frequency is valid
func (f TriggerFrequency) IsValid() bool {
	switch f {
	case TriggerFrequencyOnce, TriggerFrequencyEveryTime:
		return true
	default:
		return false
	}
}

// NodeType represents the type of automation node
type NodeType string

const (
	NodeTypeTrigger        NodeType = "trigger"
	NodeTypeDelay          NodeType = "delay"
	NodeTypeEmail          NodeType = "email"
	NodeTypeBranch         NodeType = "branch"
	NodeTypeFilter         NodeType = "filter"
	NodeTypeAddToList      NodeType = "add_to_list"
	NodeTypeRemoveFromList NodeType = "remove_from_list"
	NodeTypeABTest         NodeType = "ab_test"
)

// IsValid checks if the node type is valid
func (t NodeType) IsValid() bool {
	switch t {
	case NodeTypeTrigger, NodeTypeDelay, NodeTypeEmail, NodeTypeBranch,
		NodeTypeFilter, NodeTypeAddToList, NodeTypeRemoveFromList,
		NodeTypeABTest:
		return true
	default:
		return false
	}
}

// ContactAutomationStatus represents the status of a contact's journey in an automation
type ContactAutomationStatus string

const (
	ContactAutomationStatusActive    ContactAutomationStatus = "active"
	ContactAutomationStatusCompleted ContactAutomationStatus = "completed"
	ContactAutomationStatusExited    ContactAutomationStatus = "exited"
	ContactAutomationStatusFailed    ContactAutomationStatus = "failed"
)

// IsValid checks if the contact automation status is valid
func (s ContactAutomationStatus) IsValid() bool {
	switch s {
	case ContactAutomationStatusActive, ContactAutomationStatusCompleted,
		ContactAutomationStatusExited, ContactAutomationStatusFailed:
		return true
	default:
		return false
	}
}

// NodeAction represents an action in the automation node execution log
type NodeAction string

const (
	NodeActionEntered    NodeAction = "entered"
	NodeActionProcessing NodeAction = "processing"
	NodeActionCompleted  NodeAction = "completed"
	NodeActionFailed     NodeAction = "failed"
	NodeActionSkipped    NodeAction = "skipped"
)

// IsValid checks if the node action is valid
func (a NodeAction) IsValid() bool {
	switch a {
	case NodeActionEntered, NodeActionProcessing, NodeActionCompleted,
		NodeActionFailed, NodeActionSkipped:
		return true
	default:
		return false
	}
}

// TimelineTriggerConfig defines the trigger configuration for an automation
type TimelineTriggerConfig struct {
	EventKinds []string         `json:"event_kinds"` // Timeline event types to listen for
	Conditions *TreeNode        `json:"conditions"`  // Reuse segments condition system
	Frequency  TriggerFrequency `json:"frequency"`
}

// Validate validates the trigger configuration
func (c *TimelineTriggerConfig) Validate() error {
	if len(c.EventKinds) == 0 {
		return fmt.Errorf("at least one event kind is required")
	}

	for _, kind := range c.EventKinds {
		if kind == "" {
			return fmt.Errorf("event kind cannot be empty")
		}
	}

	if !c.Frequency.IsValid() {
		return fmt.Errorf("invalid trigger frequency: %s", c.Frequency)
	}

	return nil
}

// AutomationStats holds statistics for an automation
type AutomationStats struct {
	Enrolled  int64 `json:"enrolled"`
	Completed int64 `json:"completed"`
	Exited    int64 `json:"exited"`
	Failed    int64 `json:"failed"`
}

// Automation represents an email marketing automation workflow
type Automation struct {
	ID          string                 `json:"id"`
	WorkspaceID string                 `json:"workspace_id"`
	Name        string                 `json:"name"`
	Status      AutomationStatus       `json:"status"`
	ListID      string                 `json:"list_id"`
	Trigger     *TimelineTriggerConfig `json:"trigger"`
	TriggerSQL  *string                `json:"trigger_sql,omitempty"` // Generated SQL for WHEN clause
	RootNodeID  string                 `json:"root_node_id"`
	Nodes       []*AutomationNode      `json:"nodes"`                 // Embedded workflow nodes
	Stats       *AutomationStats       `json:"stats,omitempty"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
	DeletedAt   *time.Time             `json:"deleted_at,omitempty"` // Soft-delete timestamp
}

// GetNodeByID finds a node in the automation's Nodes array by ID
func (a *Automation) GetNodeByID(nodeID string) *AutomationNode {
	for _, n := range a.Nodes {
		if n.ID == nodeID {
			return n
		}
	}
	return nil
}

// Validate validates the automation
func (a *Automation) Validate() error {
	if a.ID == "" {
		return fmt.Errorf("id is required")
	}
	if len(a.ID) > 32 {
		return fmt.Errorf("id cannot exceed 32 characters")
	}

	if a.WorkspaceID == "" {
		return fmt.Errorf("workspace_id is required")
	}

	if a.Name == "" {
		return fmt.Errorf("name is required")
	}
	if len(a.Name) > 255 {
		return fmt.Errorf("name cannot exceed 255 characters")
	}

	if !a.Status.IsValid() {
		return fmt.Errorf("invalid automation status: %s", a.Status)
	}

	// Note: list_id is optional - event-based automations may not have a list

	if a.Trigger == nil {
		return fmt.Errorf("trigger configuration is required")
	}
	if err := a.Trigger.Validate(); err != nil {
		return err
	}

	// Validate embedded nodes
	for i, node := range a.Nodes {
		if node == nil {
			return fmt.Errorf("node at index %d is nil", i)
		}
		if err := node.Validate(); err != nil {
			return fmt.Errorf("invalid node %s: %w", node.ID, err)
		}
	}

	// Validate root_node_id references a valid node (only if nodes exist)
	if len(a.Nodes) > 0 {
		if a.RootNodeID == "" {
			return fmt.Errorf("root_node_id is required when nodes are present")
		}
		if a.GetNodeByID(a.RootNodeID) == nil {
			return fmt.Errorf("root_node_id %s does not reference a valid node", a.RootNodeID)
		}
	}

	return nil
}

// HasEmailNodeRestriction returns true if email nodes are not allowed for this automation.
// Email nodes require a list to be configured because emails need contact data from list membership.
func (a *Automation) HasEmailNodeRestriction() bool {
	return a.ListID == ""
}

// NodePosition represents the visual position of a node in the flow editor
type NodePosition struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// AutomationNode represents a node in an automation workflow
type AutomationNode struct {
	ID           string                 `json:"id"`
	AutomationID string                 `json:"automation_id"`
	Type         NodeType               `json:"type"`
	Config       map[string]interface{} `json:"config"`
	NextNodeID   *string                `json:"next_node_id,omitempty"`
	Position     NodePosition           `json:"position"`
	CreatedAt    time.Time              `json:"created_at"`
}

// Validate validates the automation node
func (n *AutomationNode) Validate() error {
	if n.ID == "" {
		return fmt.Errorf("id is required")
	}
	if len(n.ID) > 32 {
		return fmt.Errorf("id cannot exceed 32 characters")
	}

	if n.AutomationID == "" {
		return fmt.Errorf("automation_id is required")
	}

	if !n.Type.IsValid() {
		return fmt.Errorf("invalid node type: %s", n.Type)
	}

	if n.Config == nil {
		return fmt.Errorf("config is required")
	}

	return nil
}

// ValidateForAutomation validates the node in context of its parent automation.
// This includes additional checks like email node restrictions.
func (n *AutomationNode) ValidateForAutomation(automation *Automation) error {
	if err := n.Validate(); err != nil {
		return err
	}

	// Email nodes require a list to be configured - emails need contact data from list membership
	if n.Type == NodeTypeEmail && automation.HasEmailNodeRestriction() {
		return fmt.Errorf("email nodes require a list to be configured - emails need contact data from list membership")
	}

	return nil
}

// HasEmailNodes checks if any nodes in the provided list are email nodes
func HasEmailNodes(nodes []*AutomationNode) bool {
	for _, node := range nodes {
		if node.Type == NodeTypeEmail {
			return true
		}
	}
	return false
}

// ContactAutomation tracks a contact's journey through an automation
type ContactAutomation struct {
	ID            string                  `json:"id"`
	AutomationID  string                  `json:"automation_id"`
	ContactEmail  string                  `json:"contact_email"`
	CurrentNodeID *string                 `json:"current_node_id,omitempty"`
	Status        ContactAutomationStatus `json:"status"`
	ExitReason    *string                 `json:"exit_reason,omitempty"` // Why contact exited: completed, filter_rejected, automation_node_deleted, manual, unsubscribed
	EnteredAt     time.Time               `json:"entered_at"`
	ScheduledAt   *time.Time              `json:"scheduled_at,omitempty"`
	Context       map[string]interface{}  `json:"context,omitempty"`
	RetryCount    int                     `json:"retry_count"`
	LastError     *string                 `json:"last_error,omitempty"`
	LastRetryAt   *time.Time              `json:"last_retry_at,omitempty"`
	MaxRetries    int                     `json:"max_retries"`
}

// simple email regex for validation
var emailRegexAutomation = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)

// Validate validates the contact automation
func (ca *ContactAutomation) Validate() error {
	if ca.ID == "" {
		return fmt.Errorf("id is required")
	}

	if ca.AutomationID == "" {
		return fmt.Errorf("automation_id is required")
	}

	if ca.ContactEmail == "" {
		return fmt.Errorf("contact_email is required")
	}
	if !emailRegexAutomation.MatchString(ca.ContactEmail) {
		return fmt.Errorf("invalid email format")
	}

	if !ca.Status.IsValid() {
		return fmt.Errorf("invalid contact automation status: %s", ca.Status)
	}

	if ca.RetryCount < 0 {
		return fmt.Errorf("retry_count cannot be negative")
	}

	if ca.MaxRetries < 0 {
		return fmt.Errorf("max_retries cannot be negative")
	}

	return nil
}

// ContactAutomationWithWorkspace includes workspace ID for global processing
type ContactAutomationWithWorkspace struct {
	WorkspaceID string
	ContactAutomation
}

// NodeExecution tracks a contact's progress through an automation node
type NodeExecution struct {
	ID                  string                 `json:"id"`
	ContactAutomationID string                 `json:"contact_automation_id"`
	NodeID              string                 `json:"node_id"`
	NodeType            NodeType               `json:"node_type"`
	Action              NodeAction             `json:"action"`
	EnteredAt           time.Time              `json:"entered_at"`
	CompletedAt         *time.Time             `json:"completed_at,omitempty"`
	DurationMs          *int64                 `json:"duration_ms,omitempty"`
	Output              map[string]interface{} `json:"output,omitempty"`
	Error               *string                `json:"error,omitempty"`
}

// Validate validates the node execution entry
func (e *NodeExecution) Validate() error {
	if e.ID == "" {
		return fmt.Errorf("id is required")
	}

	if e.ContactAutomationID == "" {
		return fmt.Errorf("contact_automation_id is required")
	}

	if e.NodeID == "" {
		return fmt.Errorf("node_id is required")
	}

	if !e.NodeType.IsValid() {
		return fmt.Errorf("invalid node type: %s", e.NodeType)
	}

	if !e.Action.IsValid() {
		return fmt.Errorf("invalid node action: %s", e.Action)
	}

	return nil
}

// Node configuration types

// DelayNodeConfig configures a delay node
type DelayNodeConfig struct {
	Duration int    `json:"duration"`
	Unit     string `json:"unit"` // "minutes", "hours", "days"
}

// Validate validates the delay node config
func (c DelayNodeConfig) Validate() error {
	if c.Duration <= 0 {
		return fmt.Errorf("duration must be positive")
	}

	switch c.Unit {
	case "minutes", "hours", "days":
		return nil
	default:
		return fmt.Errorf("invalid unit: %s (must be minutes, hours, or days)", c.Unit)
	}
}

// EmailNodeConfig configures an email node
type EmailNodeConfig struct {
	TemplateID      string  `json:"template_id"`
	SubjectOverride *string `json:"subject_override,omitempty"`
	FromOverride    *string `json:"from_override,omitempty"`
}

// Validate validates the email node config
func (c EmailNodeConfig) Validate() error {
	if c.TemplateID == "" {
		return fmt.Errorf("template_id is required")
	}
	return nil
}

// BranchPath represents a branch path in a branch node
type BranchPath struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Conditions *TreeNode `json:"conditions"`
	NextNodeID string    `json:"next_node_id"`
}

// BranchNodeConfig configures a branch node
type BranchNodeConfig struct {
	Paths         []BranchPath `json:"paths"`
	DefaultPathID string       `json:"default_path_id"`
}

// FilterNodeConfig configures a filter node
type FilterNodeConfig struct {
	Conditions     *TreeNode `json:"conditions"`
	ContinueNodeID string    `json:"continue_node_id"`
	ExitNodeID     string    `json:"exit_node_id"`
}

// AddToListNodeConfig configures an add-to-list node
type AddToListNodeConfig struct {
	ListID   string                 `json:"list_id"`
	Status   string                 `json:"status"` // "subscribed", "pending"
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// Validate validates the add-to-list node config
func (c AddToListNodeConfig) Validate() error {
	if c.ListID == "" {
		return fmt.Errorf("list_id is required")
	}
	if c.Status != "subscribed" && c.Status != "pending" {
		return fmt.Errorf("invalid status: %s (must be subscribed or pending)", c.Status)
	}
	return nil
}

// RemoveFromListNodeConfig configures a remove-from-list node
type RemoveFromListNodeConfig struct {
	ListID string `json:"list_id"`
}

// Validate validates the remove-from-list node config
func (c RemoveFromListNodeConfig) Validate() error {
	if c.ListID == "" {
		return fmt.Errorf("list_id is required")
	}
	return nil
}

// ABTestVariant represents a variant in an A/B test node
type ABTestVariant struct {
	ID         string `json:"id"`           // "A", "B", etc.
	Name       string `json:"name"`         // "Control", "Variant B", etc.
	Weight     int    `json:"weight"`       // 1-100
	NextNodeID string `json:"next_node_id"` // Node to execute for this variant
}

// Validate validates the A/B test variant
func (v ABTestVariant) Validate() error {
	if v.ID == "" {
		return fmt.Errorf("variant id is required")
	}
	if v.Name == "" {
		return fmt.Errorf("variant name is required")
	}
	if v.Weight < 1 || v.Weight > 100 {
		return fmt.Errorf("variant weight must be between 1 and 100")
	}
	if v.NextNodeID == "" {
		return fmt.Errorf("variant next_node_id is required")
	}
	return nil
}

// ABTestNodeConfig configures an A/B test node
type ABTestNodeConfig struct {
	Variants []ABTestVariant `json:"variants"`
}

// Validate validates the A/B test node config
func (c ABTestNodeConfig) Validate() error {
	if len(c.Variants) < 2 {
		return fmt.Errorf("at least 2 variants are required for A/B test")
	}

	totalWeight := 0
	seenIDs := make(map[string]bool)

	for i, v := range c.Variants {
		if err := v.Validate(); err != nil {
			return fmt.Errorf("variant %d: %w", i, err)
		}
		if seenIDs[v.ID] {
			return fmt.Errorf("duplicate variant id: %s", v.ID)
		}
		seenIDs[v.ID] = true
		totalWeight += v.Weight
	}

	if totalWeight != 100 {
		return fmt.Errorf("variant weights must sum to 100, got %d", totalWeight)
	}

	return nil
}

// AutomationFilter defines filtering options for listing automations
type AutomationFilter struct {
	Status         []AutomationStatus
	ListID         string
	IncludeDeleted bool // When true, includes soft-deleted automations in results
	Limit          int
	Offset         int
}

// ContactAutomationFilter defines filtering options for listing contact automations
type ContactAutomationFilter struct {
	AutomationID string
	ContactEmail string
	Status       []ContactAutomationStatus
	ScheduledBy  *time.Time // Get contacts scheduled before this time
	Limit        int
	Offset       int
}

// AutomationRepository defines the interface for automation persistence
type AutomationRepository interface {
	// Transaction support
	WithTransaction(ctx context.Context, fn func(*sql.Tx) error) error

	// Automation CRUD (nodes are embedded in automation as JSONB)
	Create(ctx context.Context, workspaceID string, automation *Automation) error
	CreateTx(ctx context.Context, tx *sql.Tx, workspaceID string, automation *Automation) error
	GetByID(ctx context.Context, workspaceID, id string) (*Automation, error)
	GetByIDTx(ctx context.Context, tx *sql.Tx, workspaceID, id string) (*Automation, error)
	List(ctx context.Context, workspaceID string, filter AutomationFilter) ([]*Automation, int, error)
	Update(ctx context.Context, workspaceID string, automation *Automation) error
	UpdateTx(ctx context.Context, tx *sql.Tx, workspaceID string, automation *Automation) error
	Delete(ctx context.Context, workspaceID, id string) error
	DeleteTx(ctx context.Context, tx *sql.Tx, workspaceID, id string) error

	// Trigger management (dynamic SQL execution)
	CreateAutomationTrigger(ctx context.Context, workspaceID string, automation *Automation) error
	DropAutomationTrigger(ctx context.Context, workspaceID, automationID string) error

	// Contact automation operations
	GetContactAutomation(ctx context.Context, workspaceID, id string) (*ContactAutomation, error)
	GetContactAutomationTx(ctx context.Context, tx *sql.Tx, workspaceID, id string) (*ContactAutomation, error)
	GetContactAutomationByEmail(ctx context.Context, workspaceID, automationID, email string) (*ContactAutomation, error)
	ListContactAutomations(ctx context.Context, workspaceID string, filter ContactAutomationFilter) ([]*ContactAutomation, int, error)
	UpdateContactAutomation(ctx context.Context, workspaceID string, ca *ContactAutomation) error
	UpdateContactAutomationTx(ctx context.Context, tx *sql.Tx, workspaceID string, ca *ContactAutomation) error
	GetScheduledContactAutomations(ctx context.Context, workspaceID string, beforeTime time.Time, limit int) ([]*ContactAutomation, error)

	// Global scheduling (across all workspaces with round-robin)
	GetScheduledContactAutomationsGlobal(ctx context.Context, beforeTime time.Time, limit int) ([]*ContactAutomationWithWorkspace, error)

	// Node execution logging
	CreateNodeExecution(ctx context.Context, workspaceID string, entry *NodeExecution) error
	CreateNodeExecutionTx(ctx context.Context, tx *sql.Tx, workspaceID string, entry *NodeExecution) error
	GetNodeExecutions(ctx context.Context, workspaceID, contactAutomationID string) ([]*NodeExecution, error)
	UpdateNodeExecution(ctx context.Context, workspaceID string, entry *NodeExecution) error
	UpdateNodeExecutionTx(ctx context.Context, tx *sql.Tx, workspaceID string, entry *NodeExecution) error

	// Stats
	UpdateAutomationStats(ctx context.Context, workspaceID, automationID string, stats *AutomationStats) error
	UpdateAutomationStatsTx(ctx context.Context, tx *sql.Tx, workspaceID, automationID string, stats *AutomationStats) error
	IncrementAutomationStat(ctx context.Context, workspaceID, automationID, statName string) error
}

//go:generate mockgen -destination mocks/mock_automation_service.go -package mocks github.com/Notifuse/notifuse/internal/domain AutomationService

// AutomationService defines the interface for automation business logic
type AutomationService interface {
	// CRUD (nodes are embedded in automation)
	Create(ctx context.Context, workspaceID string, automation *Automation) error
	Get(ctx context.Context, workspaceID, automationID string) (*Automation, error)
	List(ctx context.Context, workspaceID string, filter AutomationFilter) ([]*Automation, int, error)
	Update(ctx context.Context, workspaceID string, automation *Automation) error
	Delete(ctx context.Context, workspaceID, automationID string) error

	// Status management
	Activate(ctx context.Context, workspaceID, automationID string) error
	Pause(ctx context.Context, workspaceID, automationID string) error

	// Node executions/debugging
	GetContactNodeExecutions(ctx context.Context, workspaceID, automationID, email string) (*ContactAutomation, []*NodeExecution, error)
}

// HTTP Request/Response types for automation API

// CreateAutomationRequest represents the request to create an automation
type CreateAutomationRequest struct {
	WorkspaceID string      `json:"workspace_id"`
	Automation  *Automation `json:"automation"`
}

// Validate validates the create automation request
func (r *CreateAutomationRequest) Validate() error {
	if r.WorkspaceID == "" {
		return fmt.Errorf("workspace_id is required")
	}
	if r.Automation == nil {
		return fmt.Errorf("automation is required")
	}
	// Set workspace ID on automation if not set
	if r.Automation.WorkspaceID == "" {
		r.Automation.WorkspaceID = r.WorkspaceID
	}
	return r.Automation.Validate()
}

// UpdateAutomationRequest represents the request to update an automation
type UpdateAutomationRequest struct {
	WorkspaceID string      `json:"workspace_id"`
	Automation  *Automation `json:"automation"`
}

// Validate validates the update automation request
func (r *UpdateAutomationRequest) Validate() error {
	if r.WorkspaceID == "" {
		return fmt.Errorf("workspace_id is required")
	}
	if r.Automation == nil {
		return fmt.Errorf("automation is required")
	}
	if r.Automation.ID == "" {
		return fmt.Errorf("automation id is required")
	}
	// Set workspace ID on automation if not set
	if r.Automation.WorkspaceID == "" {
		r.Automation.WorkspaceID = r.WorkspaceID
	}
	return r.Automation.Validate()
}

// GetAutomationRequest represents the request to get an automation
type GetAutomationRequest struct {
	WorkspaceID  string `json:"workspace_id"`
	AutomationID string `json:"automation_id"`
}

// FromURLParams parses the request from URL parameters
func (r *GetAutomationRequest) FromURLParams(params map[string][]string) error {
	if v, ok := params["workspace_id"]; ok && len(v) > 0 {
		r.WorkspaceID = v[0]
	}
	if v, ok := params["automation_id"]; ok && len(v) > 0 {
		r.AutomationID = v[0]
	}
	return r.Validate()
}

// Validate validates the get automation request
func (r *GetAutomationRequest) Validate() error {
	if r.WorkspaceID == "" {
		return fmt.Errorf("workspace_id is required")
	}
	if r.AutomationID == "" {
		return fmt.Errorf("automation_id is required")
	}
	return nil
}

// ListAutomationsRequest represents the request to list automations
type ListAutomationsRequest struct {
	WorkspaceID string             `json:"workspace_id"`
	Status      []AutomationStatus `json:"status,omitempty"`
	ListID      string             `json:"list_id,omitempty"`
	Limit       int                `json:"limit,omitempty"`
	Offset      int                `json:"offset,omitempty"`
}

// FromURLParams parses the request from URL parameters
func (r *ListAutomationsRequest) FromURLParams(params map[string][]string) error {
	if v, ok := params["workspace_id"]; ok && len(v) > 0 {
		r.WorkspaceID = v[0]
	}
	if v, ok := params["status"]; ok {
		for _, s := range v {
			r.Status = append(r.Status, AutomationStatus(s))
		}
	}
	if v, ok := params["list_id"]; ok && len(v) > 0 {
		r.ListID = v[0]
	}
	// Parse limit and offset if provided
	if v, ok := params["limit"]; ok && len(v) > 0 {
		var limit int
		_, _ = fmt.Sscanf(v[0], "%d", &limit)
		r.Limit = limit
	}
	if v, ok := params["offset"]; ok && len(v) > 0 {
		var offset int
		_, _ = fmt.Sscanf(v[0], "%d", &offset)
		r.Offset = offset
	}
	return r.Validate()
}

// Validate validates the list automations request
func (r *ListAutomationsRequest) Validate() error {
	if r.WorkspaceID == "" {
		return fmt.Errorf("workspace_id is required")
	}
	return nil
}

// ToFilter converts the request to an AutomationFilter
func (r *ListAutomationsRequest) ToFilter() AutomationFilter {
	return AutomationFilter{
		Status: r.Status,
		ListID: r.ListID,
		Limit:  r.Limit,
		Offset: r.Offset,
	}
}

// DeleteAutomationRequest represents the request to delete an automation
type DeleteAutomationRequest struct {
	WorkspaceID  string `json:"workspace_id"`
	AutomationID string `json:"automation_id"`
}

// Validate validates the delete automation request
func (r *DeleteAutomationRequest) Validate() error {
	if r.WorkspaceID == "" {
		return fmt.Errorf("workspace_id is required")
	}
	if r.AutomationID == "" {
		return fmt.Errorf("automation_id is required")
	}
	return nil
}

// ActivateAutomationRequest represents the request to activate an automation
type ActivateAutomationRequest struct {
	WorkspaceID  string `json:"workspace_id"`
	AutomationID string `json:"automation_id"`
}

// Validate validates the activate automation request
func (r *ActivateAutomationRequest) Validate() error {
	if r.WorkspaceID == "" {
		return fmt.Errorf("workspace_id is required")
	}
	if r.AutomationID == "" {
		return fmt.Errorf("automation_id is required")
	}
	return nil
}

// PauseAutomationRequest represents the request to pause an automation
type PauseAutomationRequest struct {
	WorkspaceID  string `json:"workspace_id"`
	AutomationID string `json:"automation_id"`
}

// Validate validates the pause automation request
func (r *PauseAutomationRequest) Validate() error {
	if r.WorkspaceID == "" {
		return fmt.Errorf("workspace_id is required")
	}
	if r.AutomationID == "" {
		return fmt.Errorf("automation_id is required")
	}
	return nil
}

// GetContactNodeExecutionsRequest represents the request to get a contact's node executions
type GetContactNodeExecutionsRequest struct {
	WorkspaceID  string `json:"workspace_id"`
	AutomationID string `json:"automation_id"`
	Email        string `json:"email"`
}

// FromURLParams parses the request from URL parameters
func (r *GetContactNodeExecutionsRequest) FromURLParams(params map[string][]string) error {
	if v, ok := params["workspace_id"]; ok && len(v) > 0 {
		r.WorkspaceID = v[0]
	}
	if v, ok := params["automation_id"]; ok && len(v) > 0 {
		r.AutomationID = v[0]
	}
	if v, ok := params["email"]; ok && len(v) > 0 {
		r.Email = v[0]
	}
	return r.Validate()
}

// Validate validates the get contact node executions request
func (r *GetContactNodeExecutionsRequest) Validate() error {
	if r.WorkspaceID == "" {
		return fmt.Errorf("workspace_id is required")
	}
	if r.AutomationID == "" {
		return fmt.Errorf("automation_id is required")
	}
	if r.Email == "" {
		return fmt.Errorf("email is required")
	}
	return nil
}
