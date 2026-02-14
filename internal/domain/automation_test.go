package domain

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAutomationStatus_IsValid(t *testing.T) {
	tests := []struct {
		name   string
		status AutomationStatus
		want   bool
	}{
		{"draft is valid", AutomationStatusDraft, true},
		{"live is valid", AutomationStatusLive, true},
		{"paused is valid", AutomationStatusPaused, true},
		{"empty is invalid", AutomationStatus(""), false},
		{"unknown is invalid", AutomationStatus("unknown"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.status.IsValid())
		})
	}
}

func TestTriggerFrequency_IsValid(t *testing.T) {
	tests := []struct {
		name      string
		frequency TriggerFrequency
		want      bool
	}{
		{"once is valid", TriggerFrequencyOnce, true},
		{"every_time is valid", TriggerFrequencyEveryTime, true},
		{"empty is invalid", TriggerFrequency(""), false},
		{"unknown is invalid", TriggerFrequency("unknown"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.frequency.IsValid())
		})
	}
}

func TestNodeType_IsValid(t *testing.T) {
	tests := []struct {
		name     string
		nodeType NodeType
		want     bool
	}{
		{"trigger is valid", NodeTypeTrigger, true},
		{"delay is valid", NodeTypeDelay, true},
		{"email is valid", NodeTypeEmail, true},
		{"branch is valid", NodeTypeBranch, true},
		{"filter is valid", NodeTypeFilter, true},
		{"add_to_list is valid", NodeTypeAddToList, true},
		{"remove_from_list is valid", NodeTypeRemoveFromList, true},
		{"ab_test is valid", NodeTypeABTest, true},
		{"empty is invalid", NodeType(""), false},
		{"unknown is invalid", NodeType("unknown"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.nodeType.IsValid())
		})
	}
}

func TestContactAutomationStatus_IsValid(t *testing.T) {
	tests := []struct {
		name   string
		status ContactAutomationStatus
		want   bool
	}{
		{"active is valid", ContactAutomationStatusActive, true},
		{"completed is valid", ContactAutomationStatusCompleted, true},
		{"exited is valid", ContactAutomationStatusExited, true},
		{"failed is valid", ContactAutomationStatusFailed, true},
		{"empty is invalid", ContactAutomationStatus(""), false},
		{"unknown is invalid", ContactAutomationStatus("unknown"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.status.IsValid())
		})
	}
}

func TestNodeAction_IsValid(t *testing.T) {
	tests := []struct {
		name   string
		action NodeAction
		want   bool
	}{
		{"entered is valid", NodeActionEntered, true},
		{"processing is valid", NodeActionProcessing, true},
		{"completed is valid", NodeActionCompleted, true},
		{"failed is valid", NodeActionFailed, true},
		{"skipped is valid", NodeActionSkipped, true},
		{"empty is invalid", NodeAction(""), false},
		{"unknown is invalid", NodeAction("unknown"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.action.IsValid())
		})
	}
}

func validTimelineTriggerConfig() *TimelineTriggerConfig {
	listID := "list123"
	return &TimelineTriggerConfig{
		EventKind: "list.subscribed",
		ListID:    &listID,
		Frequency: TriggerFrequencyOnce,
	}
}

func TestTimelineTriggerConfig_Validate(t *testing.T) {
	listID := "list123"
	segmentID := "segment123"
	customEventName := "purchase"

	tests := []struct {
		name    string
		config  *TimelineTriggerConfig
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid config - list event with list_id",
			config:  validTimelineTriggerConfig(),
			wantErr: false,
		},
		{
			name: "valid config - contact.created (no list_id required)",
			config: &TimelineTriggerConfig{
				EventKind: "contact.created",
				Frequency: TriggerFrequencyOnce,
			},
			wantErr: false,
		},
		{
			name: "valid config - segment event with segment_id",
			config: &TimelineTriggerConfig{
				EventKind: "segment.joined",
				SegmentID: &segmentID,
				Frequency: TriggerFrequencyOnce,
			},
			wantErr: false,
		},
		{
			name: "valid config - custom_event with custom_event_name",
			config: &TimelineTriggerConfig{
				EventKind:       "custom_event",
				CustomEventName: &customEventName,
				Frequency:       TriggerFrequencyOnce,
			},
			wantErr: false,
		},
		{
			name: "valid config with conditions",
			config: &TimelineTriggerConfig{
				EventKind: "list.subscribed",
				ListID:    &listID,
				Conditions: &TreeNode{
					Kind: "leaf",
					Leaf: &TreeNodeLeaf{
						Source: "contacts",
						Contact: &ContactCondition{
							Filters: []*DimensionFilter{
								{
									FieldName:    "email",
									FieldType:    "string",
									Operator:     "equals",
									StringValues: []string{"test@example.com"},
								},
							},
						},
					},
				},
				Frequency: TriggerFrequencyEveryTime,
			},
			wantErr: false,
		},
		{
			name: "empty event kind",
			config: &TimelineTriggerConfig{
				EventKind: "",
				Frequency: TriggerFrequencyOnce,
			},
			wantErr: true,
			errMsg:  "event kind is required",
		},
		{
			name: "invalid event kind",
			config: &TimelineTriggerConfig{
				EventKind: "invalid.event",
				Frequency: TriggerFrequencyOnce,
			},
			wantErr: true,
			errMsg:  "invalid event kind",
		},
		{
			name: "invalid frequency",
			config: &TimelineTriggerConfig{
				EventKind: "contact.created",
				Frequency: TriggerFrequency("invalid"),
			},
			wantErr: true,
			errMsg:  "invalid trigger frequency",
		},
		{
			name: "list event missing list_id",
			config: &TimelineTriggerConfig{
				EventKind: "list.subscribed",
				Frequency: TriggerFrequencyOnce,
			},
			wantErr: true,
			errMsg:  "list_id is required for list events",
		},
		{
			name: "segment event missing segment_id",
			config: &TimelineTriggerConfig{
				EventKind: "segment.joined",
				Frequency: TriggerFrequencyOnce,
			},
			wantErr: true,
			errMsg:  "segment_id is required for segment events",
		},
		{
			name: "custom_event missing custom_event_name",
			config: &TimelineTriggerConfig{
				EventKind: "custom_event",
				Frequency: TriggerFrequencyOnce,
			},
			wantErr: true,
			errMsg:  "custom_event_name is required for custom events",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func validAutomation() *Automation {
	return &Automation{
		ID:          "auto123",
		WorkspaceID: "ws123",
		Name:        "Welcome Series",
		Status:      AutomationStatusDraft,
		ListID:      "list123",
		Trigger:     validTimelineTriggerConfig(),
		RootNodeID:  "node123",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
}

func TestAutomation_Validate(t *testing.T) {
	tests := []struct {
		name       string
		automation *Automation
		wantErr    bool
		errMsg     string
	}{
		{
			name:       "valid automation",
			automation: validAutomation(),
			wantErr:    false,
		},
		{
			name: "empty ID",
			automation: func() *Automation {
				a := validAutomation()
				a.ID = ""
				return a
			}(),
			wantErr: true,
			errMsg:  "id is required",
		},
		{
			name: "ID too long",
			automation: func() *Automation {
				a := validAutomation()
				a.ID = string(make([]byte, 37))
				return a
			}(),
			wantErr: true,
			errMsg:  "id cannot exceed 36 characters",
		},
		{
			name: "empty workspace ID",
			automation: func() *Automation {
				a := validAutomation()
				a.WorkspaceID = ""
				return a
			}(),
			wantErr: true,
			errMsg:  "workspace_id is required",
		},
		{
			name: "empty name",
			automation: func() *Automation {
				a := validAutomation()
				a.Name = ""
				return a
			}(),
			wantErr: true,
			errMsg:  "name is required",
		},
		{
			name: "name too long",
			automation: func() *Automation {
				a := validAutomation()
				a.Name = string(make([]byte, 256))
				return a
			}(),
			wantErr: true,
			errMsg:  "name cannot exceed 255 characters",
		},
		{
			name: "invalid status",
			automation: func() *Automation {
				a := validAutomation()
				a.Status = AutomationStatus("invalid")
				return a
			}(),
			wantErr: true,
			errMsg:  "invalid automation status",
		},
		{
			name: "empty list ID is valid (event-based automation)",
			automation: func() *Automation {
				a := validAutomation()
				a.ListID = ""
				return a
			}(),
			wantErr: false,
		},
		{
			name: "nil trigger config",
			automation: func() *Automation {
				a := validAutomation()
				a.Trigger = nil
				return a
			}(),
			wantErr: true,
			errMsg:  "trigger configuration is required",
		},
		{
			name: "invalid trigger config",
			automation: func() *Automation {
				a := validAutomation()
				a.Trigger = &TimelineTriggerConfig{
					EventKind: "",
					Frequency: TriggerFrequencyOnce,
				}
				return a
			}(),
			wantErr: true,
			errMsg:  "event kind is required",
		},
		{
			name: "valid automation with nodes and valid root_node_id",
			automation: func() *Automation {
				a := validAutomation()
				a.Nodes = []*AutomationNode{
					{
						ID:           "node1",
						AutomationID: a.ID,
						Type:         NodeTypeTrigger,
						Config:       map[string]interface{}{},
					},
					{
						ID:           "node2",
						AutomationID: a.ID,
						Type:         NodeTypeDelay,
						Config:       map[string]interface{}{},
					},
				}
				a.RootNodeID = "node1"
				return a
			}(),
			wantErr: false,
		},
		{
			name: "empty nodes array is valid",
			automation: func() *Automation {
				a := validAutomation()
				a.Nodes = []*AutomationNode{}
				a.RootNodeID = ""
				return a
			}(),
			wantErr: false,
		},
		{
			name: "nil node in nodes array",
			automation: func() *Automation {
				a := validAutomation()
				a.Nodes = []*AutomationNode{nil}
				a.RootNodeID = "node1"
				return a
			}(),
			wantErr: true,
			errMsg:  "node at index 0 is nil",
		},
		{
			name: "invalid node in nodes array",
			automation: func() *Automation {
				a := validAutomation()
				a.Nodes = []*AutomationNode{
					{
						ID:           "", // invalid - empty ID
						AutomationID: a.ID,
						Type:         NodeTypeTrigger,
						Config:       map[string]interface{}{},
					},
				}
				a.RootNodeID = "node1"
				return a
			}(),
			wantErr: true,
			errMsg:  "id is required",
		},
		{
			name: "nodes present but empty root_node_id",
			automation: func() *Automation {
				a := validAutomation()
				a.Nodes = []*AutomationNode{
					{
						ID:           "node1",
						AutomationID: a.ID,
						Type:         NodeTypeTrigger,
						Config:       map[string]interface{}{},
					},
				}
				a.RootNodeID = ""
				return a
			}(),
			wantErr: true,
			errMsg:  "root_node_id is required when nodes are present",
		},
		{
			name: "root_node_id does not reference valid node",
			automation: func() *Automation {
				a := validAutomation()
				a.Nodes = []*AutomationNode{
					{
						ID:           "node1",
						AutomationID: a.ID,
						Type:         NodeTypeTrigger,
						Config:       map[string]interface{}{},
					},
				}
				a.RootNodeID = "nonexistent_node"
				return a
			}(),
			wantErr: true,
			errMsg:  "root_node_id nonexistent_node does not reference a valid node",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.automation.Validate()
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func validAutomationNode() *AutomationNode {
	return &AutomationNode{
		ID:           "node123",
		AutomationID: "auto123",
		Type:         NodeTypeDelay,
		Config: map[string]interface{}{
			"duration": 24,
			"unit":     "hours",
		},
		Position:  NodePosition{X: 100, Y: 200},
		CreatedAt: time.Now(),
	}
}

func TestAutomationNode_Validate(t *testing.T) {
	tests := []struct {
		name    string
		node    *AutomationNode
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid node",
			node:    validAutomationNode(),
			wantErr: false,
		},
		{
			name: "valid node with next_node_id",
			node: func() *AutomationNode {
				n := validAutomationNode()
				nextID := "node456"
				n.NextNodeID = &nextID
				return n
			}(),
			wantErr: false,
		},
		{
			name: "empty ID",
			node: func() *AutomationNode {
				n := validAutomationNode()
				n.ID = ""
				return n
			}(),
			wantErr: true,
			errMsg:  "id is required",
		},
		{
			name: "ID too long",
			node: func() *AutomationNode {
				n := validAutomationNode()
				n.ID = string(make([]byte, 37))
				return n
			}(),
			wantErr: true,
			errMsg:  "id cannot exceed 36 characters",
		},
		{
			name: "empty automation ID",
			node: func() *AutomationNode {
				n := validAutomationNode()
				n.AutomationID = ""
				return n
			}(),
			wantErr: true,
			errMsg:  "automation_id is required",
		},
		{
			name: "invalid node type",
			node: func() *AutomationNode {
				n := validAutomationNode()
				n.Type = NodeType("invalid")
				return n
			}(),
			wantErr: true,
			errMsg:  "invalid node type",
		},
		{
			name: "nil config",
			node: func() *AutomationNode {
				n := validAutomationNode()
				n.Config = nil
				return n
			}(),
			wantErr: true,
			errMsg:  "config is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.node.Validate()
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func validContactAutomation() *ContactAutomation {
	nodeID := "node123"
	return &ContactAutomation{
		ID:            "ca123",
		AutomationID:  "auto123",
		ContactEmail:  "test@example.com",
		CurrentNodeID: &nodeID,
		Status:        ContactAutomationStatusActive,
		EnteredAt:     time.Now(),
		Context:       map[string]interface{}{},
		RetryCount:    0,
		MaxRetries:    3,
	}
}

func TestContactAutomation_Validate(t *testing.T) {
	tests := []struct {
		name    string
		ca      *ContactAutomation
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid contact automation",
			ca:      validContactAutomation(),
			wantErr: false,
		},
		{
			name: "empty ID",
			ca: func() *ContactAutomation {
				ca := validContactAutomation()
				ca.ID = ""
				return ca
			}(),
			wantErr: true,
			errMsg:  "id is required",
		},
		{
			name: "empty automation ID",
			ca: func() *ContactAutomation {
				ca := validContactAutomation()
				ca.AutomationID = ""
				return ca
			}(),
			wantErr: true,
			errMsg:  "automation_id is required",
		},
		{
			name: "empty contact email",
			ca: func() *ContactAutomation {
				ca := validContactAutomation()
				ca.ContactEmail = ""
				return ca
			}(),
			wantErr: true,
			errMsg:  "contact_email is required",
		},
		{
			name: "invalid email format",
			ca: func() *ContactAutomation {
				ca := validContactAutomation()
				ca.ContactEmail = "invalid-email"
				return ca
			}(),
			wantErr: true,
			errMsg:  "invalid email format",
		},
		{
			name: "invalid status",
			ca: func() *ContactAutomation {
				ca := validContactAutomation()
				ca.Status = ContactAutomationStatus("invalid")
				return ca
			}(),
			wantErr: true,
			errMsg:  "invalid contact automation status",
		},
		{
			name: "negative retry count",
			ca: func() *ContactAutomation {
				ca := validContactAutomation()
				ca.RetryCount = -1
				return ca
			}(),
			wantErr: true,
			errMsg:  "retry_count cannot be negative",
		},
		{
			name: "negative max retries",
			ca: func() *ContactAutomation {
				ca := validContactAutomation()
				ca.MaxRetries = -1
				return ca
			}(),
			wantErr: true,
			errMsg:  "max_retries cannot be negative",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.ca.Validate()
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func validNodeExecution() *NodeExecution {
	return &NodeExecution{
		ID:                  "entry123",
		ContactAutomationID: "ca123",
		NodeID:              "node123",
		NodeType:            NodeTypeEmail,
		Action:              NodeActionEntered,
		EnteredAt:           time.Now(),
		Output:              map[string]interface{}{},
	}
}

func TestNodeExecution_Validate(t *testing.T) {
	tests := []struct {
		name    string
		entry   *NodeExecution
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid entry",
			entry:   validNodeExecution(),
			wantErr: false,
		},
		{
			name: "valid entry with completed time",
			entry: func() *NodeExecution {
				e := validNodeExecution()
				completedAt := time.Now()
				e.CompletedAt = &completedAt
				durationMs := int64(100)
				e.DurationMs = &durationMs
				return e
			}(),
			wantErr: false,
		},
		{
			name: "empty ID",
			entry: func() *NodeExecution {
				e := validNodeExecution()
				e.ID = ""
				return e
			}(),
			wantErr: true,
			errMsg:  "id is required",
		},
		{
			name: "empty contact automation ID",
			entry: func() *NodeExecution {
				e := validNodeExecution()
				e.ContactAutomationID = ""
				return e
			}(),
			wantErr: true,
			errMsg:  "contact_automation_id is required",
		},
		{
			name: "empty node ID",
			entry: func() *NodeExecution {
				e := validNodeExecution()
				e.NodeID = ""
				return e
			}(),
			wantErr: true,
			errMsg:  "node_id is required",
		},
		{
			name: "invalid node type",
			entry: func() *NodeExecution {
				e := validNodeExecution()
				e.NodeType = NodeType("invalid")
				return e
			}(),
			wantErr: true,
			errMsg:  "invalid node type",
		},
		{
			name: "invalid action",
			entry: func() *NodeExecution {
				e := validNodeExecution()
				e.Action = NodeAction("invalid")
				return e
			}(),
			wantErr: true,
			errMsg:  "invalid node action",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.entry.Validate()
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestAutomation_JSON(t *testing.T) {
	automation := validAutomation()
	automation.Stats = &AutomationStats{
		Enrolled:  100,
		Completed: 50,
		Exited:    10,
		Failed:    5,
	}

	// Marshal to JSON
	data, err := json.Marshal(automation)
	require.NoError(t, err)

	// Unmarshal back
	var decoded Automation
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, automation.ID, decoded.ID)
	assert.Equal(t, automation.Name, decoded.Name)
	assert.Equal(t, automation.Status, decoded.Status)
	assert.Equal(t, automation.ListID, decoded.ListID)
	assert.Equal(t, automation.Trigger.EventKind, decoded.Trigger.EventKind)
	assert.Equal(t, automation.Trigger.Frequency, decoded.Trigger.Frequency)
	assert.Equal(t, automation.Stats.Enrolled, decoded.Stats.Enrolled)
}

func TestAutomationNode_JSON(t *testing.T) {
	node := validAutomationNode()
	nextID := "node456"
	node.NextNodeID = &nextID

	// Marshal to JSON
	data, err := json.Marshal(node)
	require.NoError(t, err)

	// Unmarshal back
	var decoded AutomationNode
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, node.ID, decoded.ID)
	assert.Equal(t, node.AutomationID, decoded.AutomationID)
	assert.Equal(t, node.Type, decoded.Type)
	assert.Equal(t, *node.NextNodeID, *decoded.NextNodeID)
	assert.Equal(t, node.Position.X, decoded.Position.X)
	assert.Equal(t, node.Position.Y, decoded.Position.Y)
}

func TestNodePosition_JSON(t *testing.T) {
	pos := NodePosition{X: 150, Y: 250}

	data, err := json.Marshal(pos)
	require.NoError(t, err)

	var decoded NodePosition
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, pos.X, decoded.X)
	assert.Equal(t, pos.Y, decoded.Y)
}

func TestDelayNodeConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  DelayNodeConfig
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid config - hours",
			config:  DelayNodeConfig{Duration: 24, Unit: "hours"},
			wantErr: false,
		},
		{
			name:    "valid config - minutes",
			config:  DelayNodeConfig{Duration: 30, Unit: "minutes"},
			wantErr: false,
		},
		{
			name:    "valid config - days",
			config:  DelayNodeConfig{Duration: 7, Unit: "days"},
			wantErr: false,
		},
		{
			name:    "zero duration",
			config:  DelayNodeConfig{Duration: 0, Unit: "hours"},
			wantErr: true,
			errMsg:  "duration must be positive",
		},
		{
			name:    "negative duration",
			config:  DelayNodeConfig{Duration: -1, Unit: "hours"},
			wantErr: true,
			errMsg:  "duration must be positive",
		},
		{
			name:    "invalid unit",
			config:  DelayNodeConfig{Duration: 1, Unit: "weeks"},
			wantErr: true,
			errMsg:  "invalid unit",
		},
		{
			name:    "empty unit",
			config:  DelayNodeConfig{Duration: 1, Unit: ""},
			wantErr: true,
			errMsg:  "invalid unit",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestEmailNodeConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  EmailNodeConfig
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid config",
			config:  EmailNodeConfig{TemplateID: "tmpl123"},
			wantErr: false,
		},
		{
			name: "valid config with overrides",
			config: EmailNodeConfig{
				TemplateID:      "tmpl123",
				SubjectOverride: automationStringPtr("Custom Subject"),
				FromOverride:    automationStringPtr("custom@example.com"),
			},
			wantErr: false,
		},
		{
			name: "valid config with integration_id override",
			config: EmailNodeConfig{
				TemplateID:    "tmpl123",
				IntegrationID: automationStringPtr("integration456"),
			},
			wantErr: false,
		},
		{
			name: "valid config with empty integration_id",
			config: EmailNodeConfig{
				TemplateID:    "tmpl123",
				IntegrationID: automationStringPtr(""),
			},
			wantErr: false,
		},
		{
			name:    "empty template ID",
			config:  EmailNodeConfig{TemplateID: ""},
			wantErr: true,
			errMsg:  "template_id is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestAddToListNodeConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  AddToListNodeConfig
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid config - active status",
			config:  AddToListNodeConfig{ListID: "list123", Status: "active"},
			wantErr: false,
		},
		{
			name:    "valid config - pending status",
			config:  AddToListNodeConfig{ListID: "list123", Status: "pending"},
			wantErr: false,
		},
		{
			name:    "empty list ID",
			config:  AddToListNodeConfig{ListID: "", Status: "active"},
			wantErr: true,
			errMsg:  "list_id is required",
		},
		{
			name:    "invalid status - subscribed is not valid",
			config:  AddToListNodeConfig{ListID: "list123", Status: "subscribed"},
			wantErr: true,
			errMsg:  "invalid status: subscribed",
		},
		{
			name:    "invalid status - unknown",
			config:  AddToListNodeConfig{ListID: "list123", Status: "invalid"},
			wantErr: true,
			errMsg:  "invalid status",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestRemoveFromListNodeConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  RemoveFromListNodeConfig
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid config",
			config:  RemoveFromListNodeConfig{ListID: "list123"},
			wantErr: false,
		},
		{
			name:    "empty list ID",
			config:  RemoveFromListNodeConfig{ListID: ""},
			wantErr: true,
			errMsg:  "list_id is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestListStatusBranchNodeConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  ListStatusBranchNodeConfig
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid config with all branches",
			config: ListStatusBranchNodeConfig{
				ListID:          "list123",
				NotInListNodeID: "node1",
				ActiveNodeID:    "node2",
				NonActiveNodeID: "node3",
			},
			wantErr: false,
		},
		{
			name: "valid config with only not_in_list branch",
			config: ListStatusBranchNodeConfig{
				ListID:          "list123",
				NotInListNodeID: "node1",
			},
			wantErr: false,
		},
		{
			name: "valid config with only active branch",
			config: ListStatusBranchNodeConfig{
				ListID:       "list123",
				ActiveNodeID: "node1",
			},
			wantErr: false,
		},
		{
			name: "valid config with only non_active branch",
			config: ListStatusBranchNodeConfig{
				ListID:          "list123",
				NonActiveNodeID: "node1",
			},
			wantErr: false,
		},
		{
			name:    "empty list ID",
			config:  ListStatusBranchNodeConfig{ListID: "", NotInListNodeID: "node1"},
			wantErr: true,
			errMsg:  "list_id is required",
		},
		{
			name:    "no branch targets",
			config:  ListStatusBranchNodeConfig{ListID: "list123"},
			wantErr: true,
			errMsg:  "at least one branch must have a target node",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestNodeType_IsValid_ListStatusBranch(t *testing.T) {
	assert.True(t, NodeTypeListStatusBranch.IsValid())
}

// Helper function - using automationStringPtr to avoid conflict with other test files
func automationStringPtr(s string) *string {
	return &s
}

func TestAutomation_HasEmailNodeRestriction(t *testing.T) {
	tests := []struct {
		name   string
		listID string
		want   bool
	}{
		{
			name:   "empty list ID - email nodes restricted",
			listID: "",
			want:   true,
		},
		{
			name:   "list ID set - email nodes allowed",
			listID: "list123",
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := validAutomation()
			a.ListID = tt.listID
			assert.Equal(t, tt.want, a.HasEmailNodeRestriction())
		})
	}
}

func TestAutomationNode_ValidateForAutomation(t *testing.T) {
	tests := []struct {
		name       string
		node       *AutomationNode
		automation *Automation
		wantErr    bool
		errMsg     string
	}{
		{
			name: "email node with list ID - valid",
			node: func() *AutomationNode {
				n := validAutomationNode()
				n.Type = NodeTypeEmail
				n.Config = map[string]interface{}{"template_id": "tpl123"}
				return n
			}(),
			automation: validAutomation(), // has list_id
			wantErr:    false,
		},
		{
			name: "email node without list ID - invalid",
			node: func() *AutomationNode {
				n := validAutomationNode()
				n.Type = NodeTypeEmail
				n.Config = map[string]interface{}{"template_id": "tpl123"}
				return n
			}(),
			automation: func() *Automation {
				a := validAutomation()
				a.ListID = ""
				return a
			}(),
			wantErr: true,
			errMsg:  "email nodes require a list to be configured",
		},
		{
			name: "delay node without list ID - valid",
			node: func() *AutomationNode {
				n := validAutomationNode()
				n.Type = NodeTypeDelay
				return n
			}(),
			automation: func() *Automation {
				a := validAutomation()
				a.ListID = ""
				return a
			}(),
			wantErr: false,
		},
		{
			name: "branch node without list ID - valid",
			node: func() *AutomationNode {
				n := validAutomationNode()
				n.Type = NodeTypeBranch
				return n
			}(),
			automation: func() *Automation {
				a := validAutomation()
				a.ListID = ""
				return a
			}(),
			wantErr: false,
		},
		{
			name: "filter node without list ID - valid",
			node: func() *AutomationNode {
				n := validAutomationNode()
				n.Type = NodeTypeFilter
				return n
			}(),
			automation: func() *Automation {
				a := validAutomation()
				a.ListID = ""
				return a
			}(),
			wantErr: false,
		},
		{
			name: "add_to_list node without list ID - valid",
			node: func() *AutomationNode {
				n := validAutomationNode()
				n.Type = NodeTypeAddToList
				return n
			}(),
			automation: func() *Automation {
				a := validAutomation()
				a.ListID = ""
				return a
			}(),
			wantErr: false,
		},
		{
			name: "remove_from_list node without list ID - valid",
			node: func() *AutomationNode {
				n := validAutomationNode()
				n.Type = NodeTypeRemoveFromList
				return n
			}(),
			automation: func() *Automation {
				a := validAutomation()
				a.ListID = ""
				return a
			}(),
			wantErr: false,
		},
		{
			name: "invalid node - propagates validation error",
			node: func() *AutomationNode {
				n := validAutomationNode()
				n.ID = "" // Invalid node
				return n
			}(),
			automation: validAutomation(),
			wantErr:    true,
			errMsg:     "id is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.node.ValidateForAutomation(tt.automation)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestHasEmailNodes(t *testing.T) {
	tests := []struct {
		name  string
		nodes []*AutomationNode
		want  bool
	}{
		{
			name:  "nil nodes",
			nodes: nil,
			want:  false,
		},
		{
			name:  "empty nodes",
			nodes: []*AutomationNode{},
			want:  false,
		},
		{
			name: "only non-email nodes",
			nodes: []*AutomationNode{
				{Type: NodeTypeDelay},
				{Type: NodeTypeBranch},
				{Type: NodeTypeFilter},
			},
			want: false,
		},
		{
			name: "has email node",
			nodes: []*AutomationNode{
				{Type: NodeTypeDelay},
				{Type: NodeTypeEmail},
				{Type: NodeTypeBranch},
			},
			want: true,
		},
		{
			name: "only email node",
			nodes: []*AutomationNode{
				{Type: NodeTypeEmail},
			},
			want: true,
		},
		{
			name: "multiple email nodes",
			nodes: []*AutomationNode{
				{Type: NodeTypeEmail},
				{Type: NodeTypeDelay},
				{Type: NodeTypeEmail},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, HasEmailNodes(tt.nodes))
		})
	}
}

func validABTestVariant() ABTestVariant {
	return ABTestVariant{
		ID:         "A",
		Name:       "Control",
		Weight:     50,
		NextNodeID: "node_a",
	}
}

func TestABTestVariant_Validate(t *testing.T) {
	tests := []struct {
		name    string
		variant ABTestVariant
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid variant",
			variant: validABTestVariant(),
			wantErr: false,
		},
		{
			name: "empty ID",
			variant: func() ABTestVariant {
				v := validABTestVariant()
				v.ID = ""
				return v
			}(),
			wantErr: true,
			errMsg:  "variant id is required",
		},
		{
			name: "empty name",
			variant: func() ABTestVariant {
				v := validABTestVariant()
				v.Name = ""
				return v
			}(),
			wantErr: true,
			errMsg:  "variant name is required",
		},
		{
			name: "weight too low (0)",
			variant: func() ABTestVariant {
				v := validABTestVariant()
				v.Weight = 0
				return v
			}(),
			wantErr: true,
			errMsg:  "variant weight must be between 1 and 100",
		},
		{
			name: "weight negative",
			variant: func() ABTestVariant {
				v := validABTestVariant()
				v.Weight = -1
				return v
			}(),
			wantErr: true,
			errMsg:  "variant weight must be between 1 and 100",
		},
		{
			name: "weight too high (101)",
			variant: func() ABTestVariant {
				v := validABTestVariant()
				v.Weight = 101
				return v
			}(),
			wantErr: true,
			errMsg:  "variant weight must be between 1 and 100",
		},
		{
			name: "weight at minimum (1)",
			variant: func() ABTestVariant {
				v := validABTestVariant()
				v.Weight = 1
				return v
			}(),
			wantErr: false,
		},
		{
			name: "weight at maximum (100)",
			variant: func() ABTestVariant {
				v := validABTestVariant()
				v.Weight = 100
				return v
			}(),
			wantErr: false,
		},
		{
			name: "empty next_node_id",
			variant: func() ABTestVariant {
				v := validABTestVariant()
				v.NextNodeID = ""
				return v
			}(),
			wantErr: true,
			errMsg:  "variant next_node_id is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.variant.Validate()
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func validABTestNodeConfig() ABTestNodeConfig {
	return ABTestNodeConfig{
		Variants: []ABTestVariant{
			{ID: "A", Name: "Control", Weight: 50, NextNodeID: "node_a"},
			{ID: "B", Name: "Variant B", Weight: 50, NextNodeID: "node_b"},
		},
	}
}

func TestAutomation_DeletedAt(t *testing.T) {
	t.Run("automation with DeletedAt nil is not deleted", func(t *testing.T) {
		a := validAutomation()
		assert.Nil(t, a.DeletedAt)
	})

	t.Run("automation with DeletedAt set is deleted", func(t *testing.T) {
		a := validAutomation()
		now := time.Now()
		a.DeletedAt = &now
		assert.NotNil(t, a.DeletedAt)
		assert.Equal(t, now, *a.DeletedAt)
	})

	t.Run("automation with DeletedAt serializes to JSON correctly", func(t *testing.T) {
		a := validAutomation()
		now := time.Now().UTC().Truncate(time.Second)
		a.DeletedAt = &now

		// Marshal to JSON
		data, err := json.Marshal(a)
		require.NoError(t, err)

		// Unmarshal back
		var decoded Automation
		err = json.Unmarshal(data, &decoded)
		require.NoError(t, err)

		assert.NotNil(t, decoded.DeletedAt)
		assert.Equal(t, now.Unix(), decoded.DeletedAt.Unix())
	})

	t.Run("automation without DeletedAt omits field in JSON", func(t *testing.T) {
		a := validAutomation()
		a.DeletedAt = nil

		data, err := json.Marshal(a)
		require.NoError(t, err)

		// Check that deleted_at is not in the JSON
		assert.NotContains(t, string(data), "deleted_at")
	})
}

func TestAutomationFilter_IncludeDeleted(t *testing.T) {
	t.Run("default filter excludes deleted", func(t *testing.T) {
		filter := AutomationFilter{}
		assert.False(t, filter.IncludeDeleted)
	})

	t.Run("filter with IncludeDeleted true includes deleted", func(t *testing.T) {
		filter := AutomationFilter{
			IncludeDeleted: true,
		}
		assert.True(t, filter.IncludeDeleted)
	})

	t.Run("filter combines with other options", func(t *testing.T) {
		filter := AutomationFilter{
			Status:         []AutomationStatus{AutomationStatusLive},
			ListID:         "list123",
			IncludeDeleted: true,
			Limit:          10,
			Offset:         5,
		}
		assert.True(t, filter.IncludeDeleted)
		assert.Equal(t, []AutomationStatus{AutomationStatusLive}, filter.Status)
		assert.Equal(t, "list123", filter.ListID)
		assert.Equal(t, 10, filter.Limit)
		assert.Equal(t, 5, filter.Offset)
	})
}

func TestABTestNodeConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  ABTestNodeConfig
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid config with 2 variants",
			config:  validABTestNodeConfig(),
			wantErr: false,
		},
		{
			name: "valid config with 3 variants",
			config: ABTestNodeConfig{
				Variants: []ABTestVariant{
					{ID: "A", Name: "Control", Weight: 33, NextNodeID: "node_a"},
					{ID: "B", Name: "Variant B", Weight: 33, NextNodeID: "node_b"},
					{ID: "C", Name: "Variant C", Weight: 34, NextNodeID: "node_c"},
				},
			},
			wantErr: false,
		},
		{
			name: "valid config with unequal weights",
			config: ABTestNodeConfig{
				Variants: []ABTestVariant{
					{ID: "A", Name: "Control", Weight: 90, NextNodeID: "node_a"},
					{ID: "B", Name: "Variant B", Weight: 10, NextNodeID: "node_b"},
				},
			},
			wantErr: false,
		},
		{
			name: "less than 2 variants - empty",
			config: ABTestNodeConfig{
				Variants: []ABTestVariant{},
			},
			wantErr: true,
			errMsg:  "at least 2 variants are required",
		},
		{
			name: "less than 2 variants - only 1",
			config: ABTestNodeConfig{
				Variants: []ABTestVariant{
					{ID: "A", Name: "Control", Weight: 100, NextNodeID: "node_a"},
				},
			},
			wantErr: true,
			errMsg:  "at least 2 variants are required",
		},
		{
			name: "weights don't sum to 100 - too low",
			config: ABTestNodeConfig{
				Variants: []ABTestVariant{
					{ID: "A", Name: "Control", Weight: 40, NextNodeID: "node_a"},
					{ID: "B", Name: "Variant B", Weight: 40, NextNodeID: "node_b"},
				},
			},
			wantErr: true,
			errMsg:  "variant weights must sum to 100, got 80",
		},
		{
			name: "weights don't sum to 100 - too high",
			config: ABTestNodeConfig{
				Variants: []ABTestVariant{
					{ID: "A", Name: "Control", Weight: 60, NextNodeID: "node_a"},
					{ID: "B", Name: "Variant B", Weight: 60, NextNodeID: "node_b"},
				},
			},
			wantErr: true,
			errMsg:  "variant weights must sum to 100, got 120",
		},
		{
			name: "duplicate variant IDs",
			config: ABTestNodeConfig{
				Variants: []ABTestVariant{
					{ID: "A", Name: "Control", Weight: 50, NextNodeID: "node_a"},
					{ID: "A", Name: "Variant B", Weight: 50, NextNodeID: "node_b"},
				},
			},
			wantErr: true,
			errMsg:  "duplicate variant id: A",
		},
		{
			name: "invalid variant in config",
			config: ABTestNodeConfig{
				Variants: []ABTestVariant{
					{ID: "A", Name: "Control", Weight: 50, NextNodeID: "node_a"},
					{ID: "", Name: "Variant B", Weight: 50, NextNodeID: "node_b"}, // invalid - empty ID
				},
			},
			wantErr: true,
			errMsg:  "variant 1: variant id is required",
		},
		{
			name: "variant with invalid weight",
			config: ABTestNodeConfig{
				Variants: []ABTestVariant{
					{ID: "A", Name: "Control", Weight: 50, NextNodeID: "node_a"},
					{ID: "B", Name: "Variant B", Weight: 0, NextNodeID: "node_b"}, // invalid - weight 0
				},
			},
			wantErr: true,
			errMsg:  "variant 1: variant weight must be between 1 and 100",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
