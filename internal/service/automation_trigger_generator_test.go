package service

import (
	"testing"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEmbedArgs(t *testing.T) {
	tests := []struct {
		name    string
		sql     string
		args    []interface{}
		want    string
		wantErr bool
	}{
		{
			name: "no args",
			sql:  "country = 'US'",
			args: nil,
			want: "country = 'US'",
		},
		{
			name: "empty args",
			sql:  "country = 'US'",
			args: []interface{}{},
			want: "country = 'US'",
		},
		{
			name: "string arg",
			sql:  "country = $1",
			args: []interface{}{"US"},
			want: "country = 'US'",
		},
		{
			name: "string with single quote - SQL injection prevention",
			sql:  "name = $1",
			args: []interface{}{"O'Brien"},
			want: "name = 'O''Brien'",
		},
		{
			name: "string with multiple single quotes",
			sql:  "name = $1",
			args: []interface{}{"It's a 'test'"},
			want: "name = 'It''s a ''test'''",
		},
		{
			name: "integer arg",
			sql:  "count >= $1",
			args: []interface{}{5},
			want: "count >= 5",
		},
		{
			name: "int64 arg",
			sql:  "count >= $1",
			args: []interface{}{int64(100)},
			want: "count >= 100",
		},
		{
			name: "int32 arg",
			sql:  "count >= $1",
			args: []interface{}{int32(50)},
			want: "count >= 50",
		},
		{
			name: "float64 arg",
			sql:  "value >= $1",
			args: []interface{}{99.99},
			want: "value >= 99.99",
		},
		{
			name: "float32 arg",
			sql:  "value >= $1",
			args: []interface{}{float32(42.5)},
			want: "value >= 42.5",
		},
		{
			name: "boolean true",
			sql:  "active = $1",
			args: []interface{}{true},
			want: "active = TRUE",
		},
		{
			name: "boolean false",
			sql:  "active = $1",
			args: []interface{}{false},
			want: "active = FALSE",
		},
		{
			name: "boolean args combined",
			sql:  "active = $1 AND verified = $2",
			args: []interface{}{true, false},
			want: "active = TRUE AND verified = FALSE",
		},
		{
			name: "multiple args of different types",
			sql:  "country = $1 AND status = $2 AND count >= $3",
			args: []interface{}{"US", "active", 10},
			want: "country = 'US' AND status = 'active' AND count >= 10",
		},
		{
			name: "null arg",
			sql:  "deleted_at = $1",
			args: []interface{}{nil},
			want: "deleted_at = NULL",
		},
		{
			name: "complex query with multiple placeholders",
			sql:  "EXISTS (SELECT 1 FROM contacts WHERE email = NEW.email AND country = $1 AND age >= $2)",
			args: []interface{}{"France", 25},
			want: "EXISTS (SELECT 1 FROM contacts WHERE email = NEW.email AND country = 'France' AND age >= 25)",
		},
		{
			name: "placeholder not at word boundary",
			sql:  "value IN ($1, $2, $3)",
			args: []interface{}{"a", "b", "c"},
			want: "value IN ('a', 'b', 'c')",
		},
		{
			name: "double digit placeholders",
			sql:  "$1 AND $2 AND $3 AND $4 AND $5 AND $6 AND $7 AND $8 AND $9 AND $10",
			args: []interface{}{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j"},
			want: "'a' AND 'b' AND 'c' AND 'd' AND 'e' AND 'f' AND 'g' AND 'h' AND 'i' AND 'j'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := embedArgs(tt.sql, tt.args)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestEmbedArgs_UnsupportedType(t *testing.T) {
	// Test with unsupported type should return error
	type customType struct{}
	_, err := embedArgs("value = $1", []interface{}{customType{}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported arg type")
}

func TestNewAutomationTriggerGenerator(t *testing.T) {
	qb := NewQueryBuilder()
	gen := NewAutomationTriggerGenerator(qb)
	require.NotNil(t, gen)
	assert.NotNil(t, gen.queryBuilder)
}

func TestAutomationTriggerGenerator_Generate(t *testing.T) {
	qb := NewQueryBuilder()
	gen := NewAutomationTriggerGenerator(qb)

	t.Run("nil automation returns error", func(t *testing.T) {
		_, err := gen.Generate(nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "automation is nil")
	})

	t.Run("nil trigger returns error", func(t *testing.T) {
		automation := &domain.Automation{
			ID:         "test123",
			ListID:     "list1",
			RootNodeID: "node1",
			Trigger:    nil,
		}
		_, err := gen.Generate(automation)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "trigger config is nil")
	})

	t.Run("missing event kind returns error", func(t *testing.T) {
		automation := &domain.Automation{
			ID:         "test123",
			ListID:     "list1",
			RootNodeID: "node1",
			Trigger: &domain.TimelineTriggerConfig{
				EventKind: "",
				Frequency: domain.TriggerFrequencyOnce,
			},
		}
		_, err := gen.Generate(automation)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must have an event kind")
	})

	t.Run("missing root node ID returns error", func(t *testing.T) {
		automation := &domain.Automation{
			ID:         "test123",
			ListID:     "list1",
			RootNodeID: "",
			Trigger: &domain.TimelineTriggerConfig{
				EventKind: "contact.created",
				Frequency: domain.TriggerFrequencyOnce,
			},
		}
		_, err := gen.Generate(automation)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "root node ID")
	})

	t.Run("single event kind without conditions", func(t *testing.T) {
		automation := &domain.Automation{
			ID:         "test123",
			ListID:     "list1",
			RootNodeID: "node1",
			Trigger: &domain.TimelineTriggerConfig{
				EventKind: "contact.created",
				Frequency: domain.TriggerFrequencyOnce,
			},
		}

		result, err := gen.Generate(automation)
		require.NoError(t, err)
		require.NotNil(t, result)

		assert.Equal(t, "automation_trigger_test123", result.TriggerName)
		assert.Equal(t, "automation_trigger_test123", result.FunctionName)
		assert.Contains(t, result.WHENClause, "NEW.kind = 'contact.created'")
		assert.NotContains(t, result.WHENClause, "EXISTS") // No TreeNode conditions
		assert.Contains(t, result.FunctionBody, "CREATE OR REPLACE FUNCTION automation_trigger_test123()")
		assert.Contains(t, result.FunctionBody, "automation_enroll_contact")
		assert.Contains(t, result.TriggerDDL, "CREATE TRIGGER automation_trigger_test123")
		assert.Contains(t, result.TriggerDDL, "AFTER INSERT ON contact_timeline")
		assert.Contains(t, result.DropTrigger, "DROP TRIGGER IF EXISTS automation_trigger_test123")
		assert.Contains(t, result.DropFunction, "DROP FUNCTION IF EXISTS automation_trigger_test123()")
	})

	t.Run("event kind with TreeNode conditions - values are embedded", func(t *testing.T) {
		automation := &domain.Automation{
			ID:         "test789",
			ListID:     "list1",
			RootNodeID: "node1",
			Trigger: &domain.TimelineTriggerConfig{
				EventKind: "contact.created",
				Frequency: domain.TriggerFrequencyOnce,
				Conditions: &domain.TreeNode{
					Kind: "leaf",
					Leaf: &domain.TreeNodeLeaf{
						Source: "contacts",
						Contact: &domain.ContactCondition{
							Filters: []*domain.DimensionFilter{
								{
									FieldName:    "country",
									FieldType:    "string",
									Operator:     "equals",
									StringValues: []string{"US"},
								},
							},
						},
					},
				},
			},
		}

		result, err := gen.Generate(automation)
		require.NoError(t, err)
		require.NotNil(t, result)

		assert.Contains(t, result.WHENClause, "NEW.kind = 'contact.created'")
		assert.Contains(t, result.WHENClause, "EXISTS (SELECT 1 FROM contacts WHERE email = NEW.email")
		// Values are embedded, not placeholders
		assert.Contains(t, result.WHENClause, "country = 'US'")
		assert.NotContains(t, result.WHENClause, "$1") // No placeholders
	})

	t.Run("contact list membership condition", func(t *testing.T) {
		automation := &domain.Automation{
			ID:         "testlistcond",
			ListID:     "list1",
			RootNodeID: "node1",
			Trigger: &domain.TimelineTriggerConfig{
				EventKind: "email.delivered",
				Frequency: domain.TriggerFrequencyEveryTime,
				Conditions: &domain.TreeNode{
					Kind: "leaf",
					Leaf: &domain.TreeNodeLeaf{
						Source: "contact_lists",
						ContactList: &domain.ContactListCondition{
							Operator: "in",
							ListID:   "premium_members",
						},
					},
				},
			},
		}

		result, err := gen.Generate(automation)
		require.NoError(t, err)
		require.NotNil(t, result)

		assert.Contains(t, result.WHENClause, "NEW.kind = 'email.delivered'")
		assert.Contains(t, result.WHENClause, "EXISTS (SELECT 1 FROM contact_lists cl")
		assert.Contains(t, result.WHENClause, "cl.email = NEW.email")
		assert.Contains(t, result.WHENClause, "'premium_members'") // Embedded value
	})

	t.Run("escapes SQL injection in automation ID", func(t *testing.T) {
		automation := &domain.Automation{
			ID:         "test'; DROP TABLE--",
			ListID:     "list1",
			RootNodeID: "node1",
			Trigger: &domain.TimelineTriggerConfig{
				EventKind: "contact.created",
				Frequency: domain.TriggerFrequencyOnce,
			},
		}

		result, err := gen.Generate(automation)
		require.NoError(t, err)
		require.NotNil(t, result)

		// Single quotes should be escaped
		assert.Contains(t, result.FunctionBody, "test''; DROP TABLE--")
	})

	t.Run("escapes SQL injection in event kind", func(t *testing.T) {
		automation := &domain.Automation{
			ID:         "test123",
			ListID:     "list1",
			RootNodeID: "node1",
			Trigger: &domain.TimelineTriggerConfig{
				EventKind: "insert'; DROP TABLE--",
				Frequency: domain.TriggerFrequencyOnce,
			},
		}

		result, err := gen.Generate(automation)
		require.NoError(t, err)
		require.NotNil(t, result)

		// Single quotes should be escaped
		assert.Contains(t, result.WHENClause, "insert''; DROP TABLE--")
	})

	t.Run("frequency defaults to every_time when empty", func(t *testing.T) {
		automation := &domain.Automation{
			ID:         "test123",
			ListID:     "list1",
			RootNodeID: "node1",
			Trigger: &domain.TimelineTriggerConfig{
				EventKind: "contact.created",
				Frequency: "", // Empty
			},
		}

		result, err := gen.Generate(automation)
		require.NoError(t, err)
		require.NotNil(t, result)

		assert.Contains(t, result.FunctionBody, "every_time")
	})

	t.Run("function body includes correct parameters", func(t *testing.T) {
		automation := &domain.Automation{
			ID:         "auto123",
			ListID:     "mylist456",
			RootNodeID: "rootnode789",
			Trigger: &domain.TimelineTriggerConfig{
				EventKind: "contact.created",
				Frequency: domain.TriggerFrequencyOnce,
			},
		}

		result, err := gen.Generate(automation)
		require.NoError(t, err)
		require.NotNil(t, result)

		// Check function body contains all parameters
		assert.Contains(t, result.FunctionBody, "'auto123'")       // automation ID
		assert.Contains(t, result.FunctionBody, "'rootnode789'")   // root node ID
		assert.Contains(t, result.FunctionBody, "'mylist456'")     // list ID
		assert.Contains(t, result.FunctionBody, "'once'")          // frequency
		assert.Contains(t, result.FunctionBody, "NEW.email")       // email reference
		assert.Contains(t, result.FunctionBody, "LANGUAGE plpgsql")
	})

	t.Run("AND branch with multiple conditions", func(t *testing.T) {
		automation := &domain.Automation{
			ID:         "testbranch",
			ListID:     "list1",
			RootNodeID: "node1",
			Trigger: &domain.TimelineTriggerConfig{
				EventKind: "contact.created",
				Frequency: domain.TriggerFrequencyOnce,
				Conditions: &domain.TreeNode{
					Kind: "branch",
					Branch: &domain.TreeNodeBranch{
						Operator: "and",
						Leaves: []*domain.TreeNode{
							{
								Kind: "leaf",
								Leaf: &domain.TreeNodeLeaf{
									Source: "contacts",
									Contact: &domain.ContactCondition{
										Filters: []*domain.DimensionFilter{
											{
												FieldName:    "country",
												FieldType:    "string",
												Operator:     "equals",
												StringValues: []string{"US"},
											},
										},
									},
								},
							},
							{
								Kind: "leaf",
								Leaf: &domain.TreeNodeLeaf{
									Source: "contact_lists",
									ContactList: &domain.ContactListCondition{
										Operator: "in",
										ListID:   "premium",
									},
								},
							},
						},
					},
				},
			},
		}

		result, err := gen.Generate(automation)
		require.NoError(t, err)
		require.NotNil(t, result)

		assert.Contains(t, result.WHENClause, "NEW.kind = 'contact.created'")
		assert.Contains(t, result.WHENClause, "country = 'US'")
		assert.Contains(t, result.WHENClause, "'premium'")
		// Should have AND between the two conditions
		assert.Contains(t, result.WHENClause, " AND ")
	})

	t.Run("list event with list_id filter", func(t *testing.T) {
		listID := "mylist123"
		automation := &domain.Automation{
			ID:         "testlist",
			ListID:     "list1",
			RootNodeID: "node1",
			Trigger: &domain.TimelineTriggerConfig{
				EventKind: "list.subscribed",
				ListID:    &listID,
				Frequency: domain.TriggerFrequencyOnce,
			},
		}

		result, err := gen.Generate(automation)
		require.NoError(t, err)
		require.NotNil(t, result)

		assert.Contains(t, result.WHENClause, "NEW.kind = 'list.subscribed'")
		assert.Contains(t, result.WHENClause, "NEW.entity_id = 'mylist123'")
	})

	t.Run("segment event with segment_id filter", func(t *testing.T) {
		segmentID := "segment456"
		automation := &domain.Automation{
			ID:         "testsegment",
			ListID:     "list1",
			RootNodeID: "node1",
			Trigger: &domain.TimelineTriggerConfig{
				EventKind: "segment.joined",
				SegmentID: &segmentID,
				Frequency: domain.TriggerFrequencyOnce,
			},
		}

		result, err := gen.Generate(automation)
		require.NoError(t, err)
		require.NotNil(t, result)

		assert.Contains(t, result.WHENClause, "NEW.kind = 'segment.joined'")
		assert.Contains(t, result.WHENClause, "NEW.entity_id = 'segment456'")
	})

	t.Run("custom_event with custom_event_name filter", func(t *testing.T) {
		customEventName := "purchase"
		automation := &domain.Automation{
			ID:         "testcustom",
			ListID:     "list1",
			RootNodeID: "node1",
			Trigger: &domain.TimelineTriggerConfig{
				EventKind:       "custom_event",
				CustomEventName: &customEventName,
				Frequency:       domain.TriggerFrequencyOnce,
			},
		}

		result, err := gen.Generate(automation)
		require.NoError(t, err)
		require.NotNil(t, result)

		// custom_event with name should produce "custom_event.purchase" format
		assert.Contains(t, result.WHENClause, "NEW.kind = 'custom_event.purchase'")
	})

	t.Run("email event (no additional filter)", func(t *testing.T) {
		automation := &domain.Automation{
			ID:         "testemail",
			ListID:     "list1",
			RootNodeID: "node1",
			Trigger: &domain.TimelineTriggerConfig{
				EventKind: "email.opened",
				Frequency: domain.TriggerFrequencyEveryTime,
			},
		}

		result, err := gen.Generate(automation)
		require.NoError(t, err)
		require.NotNil(t, result)

		assert.Contains(t, result.WHENClause, "NEW.kind = 'email.opened'")
		// Should NOT have entity_id filter for email events
		assert.NotContains(t, result.WHENClause, "NEW.entity_id")
	})
}
