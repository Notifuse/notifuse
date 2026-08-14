package service

import (
	"strings"
	"testing"
	"time"

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

	t.Run("event kind with TreeNode conditions - values are embedded in the guard", func(t *testing.T) {
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

		// The WHEN clause carries only NEW-row predicates; the conditions subquery is
		// compiled into the function body instead, where PostgreSQL allows subqueries.
		assert.Contains(t, result.WHENClause, "NEW.kind = 'contact.created'")
		assert.NotContains(t, result.WHENClause, "EXISTS")
		assert.Contains(t, result.ConditionGuard, "EXISTS (SELECT 1 FROM contacts WHERE email = NEW.email")
		assert.Contains(t, result.FunctionBody, result.ConditionGuard)
		// Values are embedded, not placeholders
		assert.Contains(t, result.ConditionGuard, "country = 'US'")
		assert.NotContains(t, result.ConditionGuard, "$1") // No placeholders
	})

	t.Run("contact list membership condition", func(t *testing.T) {
		automation := &domain.Automation{
			ID:         "testlistcond",
			ListID:     "list1",
			RootNodeID: "node1",
			Trigger: &domain.TimelineTriggerConfig{
				EventKind: "email.clicked",
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

		// The engagement kind matches the timeline kind verbatim; the list condition
		// embeds as a subquery in the guard, never in the WHEN clause.
		assert.Contains(t, result.WHENClause, "NEW.kind = 'email.clicked'")
		assert.NotContains(t, result.WHENClause, "contact_lists")
		assert.Contains(t, result.ConditionGuard, "EXISTS (SELECT 1 FROM contact_lists cl")
		assert.Contains(t, result.ConditionGuard, "cl.email = NEW.email")
		assert.Contains(t, result.ConditionGuard, "'premium_members'") // Embedded value
		assert.Contains(t, result.FunctionBody, result.ConditionGuard)
	})

	// The automation id is not only embedded as a string literal in the function body —
	// it is also concatenated into the trigger and function NAMES, where quoting does
	// nothing. lib/pq sends these statements without arguments, over the simple query
	// protocol, which executes every statement in the string. So an id that is not a
	// plain identifier is refused outright rather than escaped.
	t.Run("rejects an automation ID that is not a valid identifier", func(t *testing.T) {
		for _, id := range []string{
			"test'; DROP TABLE--",
			"x();DROP TABLE contacts;SELECT now",
			"x ON contact_timeline; DROP TABLE contacts",
			"Welcome Series",
			`weird"quote`,
		} {
			automation := &domain.Automation{
				ID:         id,
				ListID:     "list1",
				RootNodeID: "node1",
				Trigger: &domain.TimelineTriggerConfig{
					EventKind: "contact.created",
					Frequency: domain.TriggerFrequencyOnce,
				},
			}

			result, err := gen.Generate(automation)
			require.Error(t, err, "id %q must be refused, not interpolated into DDL", id)
			assert.Nil(t, result)
			assert.Contains(t, err.Error(), "cannot be used in a trigger name")
		}
	})

	t.Run("accepts the id shapes automations actually use", func(t *testing.T) {
		for _, id := range []string{
			"550e8400-e29b-41d4-a716-446655440000", // uuid v4, hyphens stripped
			"AbC4GktKfWzXpQmR7sTvNy",               // shortuuid, mixed case
			"test123",
		} {
			automation := &domain.Automation{
				ID:         id,
				ListID:     "list1",
				RootNodeID: "node1",
				Trigger: &domain.TimelineTriggerConfig{
					EventKind: "contact.created",
					Frequency: domain.TriggerFrequencyOnce,
				},
			}

			result, err := gen.Generate(automation)
			require.NoError(t, err, "id %q must still work", id)
			require.NotNil(t, result)
		}
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
			ListID:     "mylist456", // list_id is NOT passed to function, only used for unsubscribe URLs
			RootNodeID: "rootnode789",
			Trigger: &domain.TimelineTriggerConfig{
				EventKind: "contact.created",
				Frequency: domain.TriggerFrequencyOnce,
			},
		}

		result, err := gen.Generate(automation)
		require.NoError(t, err)
		require.NotNil(t, result)

		// Check function body contains all parameters (no list_id - it's only for unsubscribe URLs)
		assert.Contains(t, result.FunctionBody, "'auto123'")     // automation ID
		assert.Contains(t, result.FunctionBody, "'rootnode789'") // root node ID
		assert.Contains(t, result.FunctionBody, "'once'")        // frequency
		assert.Contains(t, result.FunctionBody, "NEW.email")     // email reference
		assert.Contains(t, result.FunctionBody, "LANGUAGE plpgsql")
		// Verify list_id is NOT in the function body
		assert.NotContains(t, result.FunctionBody, "'mylist456'")
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
		assert.NotContains(t, result.WHENClause, "country = 'US'")
		assert.Contains(t, result.ConditionGuard, "country = 'US'")
		assert.Contains(t, result.ConditionGuard, "'premium'")
		// Should have AND between the two conditions
		assert.Contains(t, result.ConditionGuard, " AND ")
		assert.Contains(t, result.FunctionBody, result.ConditionGuard)
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

		// email.opened now equals the timeline kind written by track_message_history_changes(),
		// so the WHEN clause references it verbatim (no translation).
		assert.Contains(t, result.WHENClause, "NEW.kind = 'email.opened'")
		// Should NOT have entity_id filter for email events
		assert.NotContains(t, result.WHENClause, "NEW.entity_id")
	})

	t.Run("email.* event kinds match the timeline kind verbatim", func(t *testing.T) {
		// The contact timeline now stores these dotted kinds, so no translation is needed.
		for _, eventKind := range []string{
			"email.opened", "email.clicked", "email.bounced",
			"email.complained", "email.unsubscribed",
		} {
			t.Run(eventKind, func(t *testing.T) {
				automation := &domain.Automation{
					ID:         "map" + strings.ReplaceAll(eventKind, ".", ""),
					ListID:     "list1",
					RootNodeID: "node1",
					Trigger: &domain.TimelineTriggerConfig{
						EventKind: eventKind,
						Frequency: domain.TriggerFrequencyEveryTime,
					},
				}

				result, err := gen.Generate(automation)
				require.NoError(t, err)
				require.NotNil(t, result)

				assert.Contains(t, result.WHENClause, "NEW.kind = '"+eventKind+"'")
			})
		}
	})

	t.Run("legacy email.sent/email.delivered kinds are emitted verbatim", func(t *testing.T) {
		// These were removed from ValidEventKinds, but a legacy live automation could still
		// carry one; Generate must emit it verbatim rather than choke. email.delivered has no
		// matching timeline kind (delivered lands in the generic email.updated), so it stays
		// inert; the generator never collapses it into a generic message_history kind.
		for _, eventKind := range []string{"email.sent", "email.delivered"} {
			automation := &domain.Automation{
				ID:         "unmapped" + strings.ReplaceAll(eventKind, ".", ""),
				ListID:     "list1",
				RootNodeID: "node1",
				Trigger: &domain.TimelineTriggerConfig{
					EventKind: eventKind,
					Frequency: domain.TriggerFrequencyEveryTime,
				},
			}

			result, err := gen.Generate(automation)
			require.NoError(t, err)
			require.NotNil(t, result)

			assert.Contains(t, result.WHENClause, "NEW.kind = '"+eventKind+"'")
			assert.NotContains(t, result.WHENClause, "insert_message_history")
			assert.NotContains(t, result.WHENClause, "update_message_history")
		}
	})

	t.Run("contact.updated with updated_fields filter", func(t *testing.T) {
		automation := &domain.Automation{
			ID:         "testupdated",
			ListID:     "list1",
			RootNodeID: "node1",
			Trigger: &domain.TimelineTriggerConfig{
				EventKind:     "contact.updated",
				UpdatedFields: []string{"first_name", "last_name"},
				Frequency:     domain.TriggerFrequencyEveryTime,
			},
		}

		result, err := gen.Generate(automation)
		require.NoError(t, err)
		require.NotNil(t, result)

		assert.Contains(t, result.WHENClause, "NEW.kind = 'contact.updated'")
		// Should contain JSONB ? operator for each field
		assert.Contains(t, result.WHENClause, "NEW.changes ? 'first_name'")
		assert.Contains(t, result.WHENClause, "NEW.changes ? 'last_name'")
		// Fields should be OR'd together
		assert.Contains(t, result.WHENClause, " OR ")
	})

	t.Run("contact.updated with single updated_field", func(t *testing.T) {
		automation := &domain.Automation{
			ID:         "testsingle",
			ListID:     "list1",
			RootNodeID: "node1",
			Trigger: &domain.TimelineTriggerConfig{
				EventKind:     "contact.updated",
				UpdatedFields: []string{"phone"},
				Frequency:     domain.TriggerFrequencyOnce,
			},
		}

		result, err := gen.Generate(automation)
		require.NoError(t, err)
		require.NotNil(t, result)

		assert.Contains(t, result.WHENClause, "NEW.kind = 'contact.updated'")
		assert.Contains(t, result.WHENClause, "NEW.changes ? 'phone'")
	})

	t.Run("contact.updated without updated_fields (any field change)", func(t *testing.T) {
		automation := &domain.Automation{
			ID:         "testany",
			ListID:     "list1",
			RootNodeID: "node1",
			Trigger: &domain.TimelineTriggerConfig{
				EventKind: "contact.updated",
				Frequency: domain.TriggerFrequencyEveryTime,
			},
		}

		result, err := gen.Generate(automation)
		require.NoError(t, err)
		require.NotNil(t, result)

		assert.Contains(t, result.WHENClause, "NEW.kind = 'contact.updated'")
		// Should NOT contain changes filter
		assert.NotContains(t, result.WHENClause, "NEW.changes ?")
	})

	t.Run("contact.updated with invalid updated_field returns error", func(t *testing.T) {
		automation := &domain.Automation{
			ID:         "testinvalid",
			ListID:     "list1",
			RootNodeID: "node1",
			Trigger: &domain.TimelineTriggerConfig{
				EventKind:     "contact.updated",
				UpdatedFields: []string{"invalid_field_name"},
				Frequency:     domain.TriggerFrequencyEveryTime,
			},
		}

		_, err := gen.Generate(automation)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid updated_field")
	})

	t.Run("contact.updated with phantom photo_url field is rejected", func(t *testing.T) {
		// photo_url is not a contact column and is never written into the timeline
		// changes, so it must not be an allowed updated_field (a filter on it could
		// never match and the automation would silently never fire).
		automation := &domain.Automation{
			ID:         "testphotourl",
			ListID:     "list1",
			RootNodeID: "node1",
			Trigger: &domain.TimelineTriggerConfig{
				EventKind:     "contact.updated",
				UpdatedFields: []string{"photo_url"},
				Frequency:     domain.TriggerFrequencyEveryTime,
			},
		}

		_, err := gen.Generate(automation)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid updated_field")
	})

	t.Run("contact.updated with SQL injection attempt in updated_fields is rejected", func(t *testing.T) {
		automation := &domain.Automation{
			ID:         "testsqlinjection",
			ListID:     "list1",
			RootNodeID: "node1",
			Trigger: &domain.TimelineTriggerConfig{
				EventKind:     "contact.updated",
				UpdatedFields: []string{"first_name'; DROP TABLE--"},
				Frequency:     domain.TriggerFrequencyEveryTime,
			},
		}

		_, err := gen.Generate(automation)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid updated_field")
	})

	t.Run("contact.updated with custom fields", func(t *testing.T) {
		automation := &domain.Automation{
			ID:         "testcustom",
			ListID:     "list1",
			RootNodeID: "node1",
			Trigger: &domain.TimelineTriggerConfig{
				EventKind:     "contact.updated",
				UpdatedFields: []string{"custom_string_1", "custom_number_3", "custom_datetime_5"},
				Frequency:     domain.TriggerFrequencyEveryTime,
			},
		}

		result, err := gen.Generate(automation)
		require.NoError(t, err)
		require.NotNil(t, result)

		assert.Contains(t, result.WHENClause, "NEW.changes ? 'custom_string_1'")
		assert.Contains(t, result.WHENClause, "NEW.changes ? 'custom_number_3'")
		assert.Contains(t, result.WHENClause, "NEW.changes ? 'custom_datetime_5'")
	})

	t.Run("updated_fields ignored for non-contact.updated events", func(t *testing.T) {
		automation := &domain.Automation{
			ID:         "testignored",
			ListID:     "list1",
			RootNodeID: "node1",
			Trigger: &domain.TimelineTriggerConfig{
				EventKind:     "contact.created",
				UpdatedFields: []string{"first_name"}, // Should be ignored
				Frequency:     domain.TriggerFrequencyOnce,
			},
		}

		result, err := gen.Generate(automation)
		require.NoError(t, err)
		require.NotNil(t, result)

		assert.Contains(t, result.WHENClause, "NEW.kind = 'contact.created'")
		// Should NOT contain changes filter for non-contact.updated events
		assert.NotContains(t, result.WHENClause, "NEW.changes ?")
	})
}

// contactsLeaf builds a "contacts" leaf matching a single country.
func contactsLeaf(country string) *domain.TreeNode {
	return &domain.TreeNode{
		Kind: "leaf",
		Leaf: &domain.TreeNodeLeaf{
			Source: "contacts",
			Contact: &domain.ContactCondition{
				Filters: []*domain.DimensionFilter{
					{
						FieldName:    "country",
						FieldType:    "string",
						Operator:     "equals",
						StringValues: []string{country},
					},
				},
			},
		},
	}
}

func contactListsLeaf() *domain.TreeNode {
	return &domain.TreeNode{
		Kind: "leaf",
		Leaf: &domain.TreeNodeLeaf{
			Source: "contact_lists",
			ContactList: &domain.ContactListCondition{
				Operator: "in",
				ListID:   "premium",
			},
		},
	}
}

func contactTimelineLeaf() *domain.TreeNode {
	return &domain.TreeNode{
		Kind: "leaf",
		Leaf: &domain.TreeNodeLeaf{
			Source: "contact_timeline",
			ContactTimeline: &domain.ContactTimelineCondition{
				Kind:          "email.opened",
				CountOperator: "at_least",
				CountValue:    2,
			},
		},
	}
}

func customEventsGoalLeaf() *domain.TreeNode {
	return &domain.TreeNode{
		Kind: "leaf",
		Leaf: &domain.TreeNodeLeaf{
			Source: "custom_events_goals",
			CustomEventsGoal: &domain.CustomEventsGoalCondition{
				GoalType:          "purchase",
				AggregateOperator: "sum",
				Operator:          "gte",
				Value:             100,
				TimeframeOperator: "anytime",
			},
		},
	}
}

func automationWithConditions(id string, conditions *domain.TreeNode) *domain.Automation {
	return &domain.Automation{
		ID:         id,
		ListID:     "list1",
		RootNodeID: "node1",
		Trigger: &domain.TimelineTriggerConfig{
			EventKind:  "contact.created",
			Frequency:  domain.TriggerFrequencyOnce,
			Conditions: conditions,
		},
	}
}

func TestAutomationTriggerGenerator_WHENClauseNeverHoldsASubquery(t *testing.T) {
	// PostgreSQL parses a trigger WHEN clause with no subquery support whatsoever: any
	// subquery there is rejected at CREATE TRIGGER time with SQLSTATE 0A000, "cannot use
	// subquery in trigger WHEN condition". While the compiled conditions were emitted into
	// the WHEN clause, no automation carrying conditions could be activated at all. Every
	// leaf source the condition compiler supports produces a subquery, so the WHEN clause
	// must stay free of them and the conditions must live in the function body.
	//
	// Both markers are checked because they are not interchangeable: the contact_timeline
	// leaf compiles to a scalar "(SELECT COUNT(*) ...) >= n" comparison and contains no
	// EXISTS at all, so asserting only on "EXISTS" would let it through.
	const why = "generated WHEN clause contains a subquery; PostgreSQL rejects it at CREATE TRIGGER " +
		"with SQLSTATE 0A000 (cannot use subquery in trigger WHEN condition), which makes the " +
		"automation impossible to activate — compile conditions into the function body instead"

	qb := NewQueryBuilder()
	gen := NewAutomationTriggerGenerator(qb)

	tests := []struct {
		name       string
		conditions *domain.TreeNode
	}{
		{name: "contacts leaf", conditions: contactsLeaf("US")},
		{name: "contact_lists leaf", conditions: contactListsLeaf()},
		{name: "contact_timeline leaf", conditions: contactTimelineLeaf()},
		{name: "custom_events_goals leaf", conditions: customEventsGoalLeaf()},
		{
			name: "nested AND inside OR across all sources",
			conditions: &domain.TreeNode{
				Kind: "branch",
				Branch: &domain.TreeNodeBranch{
					Operator: "or",
					Leaves: []*domain.TreeNode{
						customEventsGoalLeaf(),
						{
							Kind: "branch",
							Branch: &domain.TreeNodeBranch{
								Operator: "and",
								Leaves: []*domain.TreeNode{
									contactsLeaf("FR"),
									contactListsLeaf(),
									contactTimelineLeaf(),
								},
							},
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := gen.Generate(automationWithConditions("whenclause", tt.conditions))
			require.NoError(t, err)
			require.NotNil(t, result)

			assert.NotContains(t, result.WHENClause, "(SELECT", why)
			assert.NotContains(t, result.WHENClause, "EXISTS", why)
			assert.Contains(t, result.WHENClause, "NEW.kind = 'contact.created'")

			// The conditions were relocated, not dropped.
			require.NotEmpty(t, result.ConditionGuard)
			assert.Contains(t, result.FunctionBody, result.ConditionGuard)
		})
	}
}

func TestAutomationTriggerGenerator_FunctionBodyWithoutConditions(t *testing.T) {
	// Pinned as a whole rather than spot-checked: the migration that repairs live
	// automations regenerates this exact text, so any drift in it is a schema change.
	qb := NewQueryBuilder()
	gen := NewAutomationTriggerGenerator(qb)

	result, err := gen.Generate(&domain.Automation{
		ID:         "golden1",
		ListID:     "list1",
		RootNodeID: "rootnode1",
		Trigger: &domain.TimelineTriggerConfig{
			EventKind: "contact.created",
			Frequency: domain.TriggerFrequencyOnce,
		},
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	const want = `CREATE OR REPLACE FUNCTION automation_trigger_golden1()
RETURNS TRIGGER AS $$
BEGIN
    PERFORM automation_enroll_contact(
        'golden1',
        NEW.email,
        'rootnode1',
        'once'
    );
    RETURN NEW;
END;
$$ LANGUAGE plpgsql`

	assert.Equal(t, want, result.FunctionBody)
	// No conditions means no guard at all, not an always-true one.
	assert.NotContains(t, result.FunctionBody, "IF ")
	assert.Empty(t, result.ConditionGuard)
	assert.Empty(t, result.ValidationQuery)
}

func TestAutomationTriggerGenerator_GuardWrapsEnrollment(t *testing.T) {
	qb := NewQueryBuilder()
	gen := NewAutomationTriggerGenerator(qb)

	result, err := gen.Generate(automationWithConditions("guarded1", contactsLeaf("US")))
	require.NoError(t, err)
	require.NotNil(t, result)

	require.NotEmpty(t, result.ConditionGuard)
	assert.Contains(t, result.FunctionBody, "IF (")
	assert.Contains(t, result.FunctionBody, "END IF;")

	// The guard has to bracket the enrollment call: a guard evaluated anywhere else in the
	// body would enroll every contact the WHEN clause let through.
	guardIdx := strings.Index(result.FunctionBody, "IF ("+result.ConditionGuard+") THEN")
	require.NotEqual(t, -1, guardIdx, "guard is not the IF condition of the function body")
	performIdx := strings.Index(result.FunctionBody, "PERFORM automation_enroll_contact")
	endIfIdx := strings.Index(result.FunctionBody, "END IF;")
	require.NotEqual(t, -1, performIdx)
	require.NotEqual(t, -1, endIfIdx)
	assert.Less(t, guardIdx, performIdx)
	assert.Less(t, performIdx, endIfIdx)
}

func TestAutomationTriggerGenerator_ValidationQuery(t *testing.T) {
	qb := NewQueryBuilder()
	gen := NewAutomationTriggerGenerator(qb)

	t.Run("absent when there are no conditions", func(t *testing.T) {
		result, err := gen.Generate(&domain.Automation{
			ID:         "noprobe",
			ListID:     "list1",
			RootNodeID: "node1",
			Trigger: &domain.TimelineTriggerConfig{
				EventKind: "contact.created",
				Frequency: domain.TriggerFrequencyOnce,
			},
		})
		require.NoError(t, err)
		assert.Empty(t, result.ValidationQuery)
	})

	t.Run("plans the same expression against a literal email", func(t *testing.T) {
		// NEW.email only resolves inside a trigger, so the probe swaps it for a literal and
		// lets EXPLAIN resolve every other column without reading a row.
		result, err := gen.Generate(automationWithConditions("probe1", contactsLeaf("US")))
		require.NoError(t, err)

		assert.True(t, strings.HasPrefix(result.ValidationQuery, "EXPLAIN SELECT ("),
			"validation query must be plannable on its own, got: %s", result.ValidationQuery)
		assert.Contains(t, result.ValidationQuery, "''::text")
		assert.NotContains(t, result.ValidationQuery, "NEW.email")
		assert.Contains(t, result.ValidationQuery, "country = 'US'")
	})
}

func TestDollarQuoteTag(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{name: "plain body", body: "BEGIN\n    RETURN NEW;\nEND;", want: "$$"},
		{name: "body contains $$", body: "BEGIN\n    x := 'a$$b';\nEND;", want: "$fn0$"},
		{name: "body contains $$ and $fn0$", body: "BEGIN\n    x := 'a$$b$fn0$c';\nEND;", want: "$fn1$"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, dollarQuoteTag(tt.body))
		})
	}
}

func TestAutomationTriggerGenerator_DollarQuotedBodySurvivesConditionValue(t *testing.T) {
	// Condition values reach the function body verbatim, so a value carrying $$ would close
	// the default dollar quote early and turn the rest of the body into garbage SQL.
	qb := NewQueryBuilder()
	gen := NewAutomationTriggerGenerator(qb)

	result, err := gen.Generate(automationWithConditions("dollartag", contactsLeaf("US$$X")))
	require.NoError(t, err)

	body := result.FunctionBody
	const header = "RETURNS TRIGGER AS "
	tagStart := strings.Index(body, header)
	require.NotEqual(t, -1, tagStart)
	tagStart += len(header)
	tagEnd := strings.Index(body[tagStart:], "\n")
	require.NotEqual(t, -1, tagEnd)
	tag := body[tagStart : tagStart+tagEnd]

	assert.Equal(t, "$fn0$", tag)
	suffix := tag + " LANGUAGE plpgsql"
	require.True(t, strings.HasSuffix(body, suffix), "closing tag must match the opening one")

	inner := body[tagStart+len(tag) : len(body)-len(suffix)]
	assert.Contains(t, inner, "US$$X")
	assert.NotContains(t, inner, tag, "the chosen tag must not occur inside the quoted body")
}

func TestSQLLiteral(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "plain string", in: "x", want: "'x'"},
		{name: "embedded single quote is doubled", in: "O'Brien", want: "'O''Brien'"},
		{name: "backslash forces an E literal", in: `C:\tmp`, want: `E'C:\\tmp'`},
		{name: "backslash and quote together", in: `a\'b`, want: `E'a\\''b'`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, sqlLiteral(tt.in))
		})
	}
}

func TestEscapeArg_Time(t *testing.T) {
	got, err := escapeArg(time.Date(2026, 3, 4, 5, 6, 7, 89000000, time.UTC))
	require.NoError(t, err)
	assert.Equal(t, "'2026-03-04T05:06:07.089Z'::timestamptz", got)

	// Normalised to UTC so the literal does not depend on the database server's timezone.
	got, err = escapeArg(time.Date(2026, 3, 4, 7, 6, 7, 0, time.FixedZone("CEST", 2*3600)))
	require.NoError(t, err)
	assert.Equal(t, "'2026-03-04T05:06:07Z'::timestamptz", got)
}

func TestAutomationTriggerGenerator_DateValuedConditions(t *testing.T) {
	// Both of these failed outright before time.Time was handled: the condition compiler
	// hands date values to the embedder as time.Time, which was rejected with
	// "unsupported arg type time.Time", so Generate never produced any SQL for them.
	qb := NewQueryBuilder()
	gen := NewAutomationTriggerGenerator(qb)

	t.Run("contacts leaf with after_date filter", func(t *testing.T) {
		conditions := &domain.TreeNode{
			Kind: "leaf",
			Leaf: &domain.TreeNodeLeaf{
				Source: "contacts",
				Contact: &domain.ContactCondition{
					Filters: []*domain.DimensionFilter{
						{
							FieldName:    "custom_datetime_1",
							FieldType:    "time",
							Operator:     "after_date",
							StringValues: []string{"2026-01-15T00:00:00Z"},
						},
					},
				},
			},
		}

		result, err := gen.Generate(automationWithConditions("afterdate", conditions))
		require.NoError(t, err)

		assert.Contains(t, result.ConditionGuard, "custom_datetime_1 > '2026-01-15T00:00:00Z'::timestamptz")
		assert.NotContains(t, result.ConditionGuard, "$1")
		assert.Contains(t, result.FunctionBody, result.ConditionGuard)
	})

	t.Run("contact_timeline leaf with in_date_range timeframe", func(t *testing.T) {
		timeframe := "in_date_range"
		conditions := &domain.TreeNode{
			Kind: "leaf",
			Leaf: &domain.TreeNodeLeaf{
				Source: "contact_timeline",
				ContactTimeline: &domain.ContactTimelineCondition{
					Kind:              "email.opened",
					CountOperator:     "at_least",
					CountValue:        1,
					TimeframeOperator: &timeframe,
					TimeframeValues:   []string{"2026-01-01T00:00:00Z", "2026-02-01T00:00:00Z"},
				},
			},
		}

		result, err := gen.Generate(automationWithConditions("daterange", conditions))
		require.NoError(t, err)

		assert.Contains(t, result.ConditionGuard,
			"ct.created_at BETWEEN '2026-01-01T00:00:00Z'::timestamptz AND '2026-02-01T00:00:00Z'::timestamptz")
		assert.NotContains(t, result.ConditionGuard, "$1")
		assert.Contains(t, result.FunctionBody, result.ConditionGuard)
	})
}

func TestEmbedArgs_LeakedPlaceholderIsAnError(t *testing.T) {
	// A placeholder left behind in the generated SQL is not inert: CREATE FUNCTION happily
	// accepts a body containing one, and then every INSERT matching the trigger aborts with
	// 42P02 (there is no parameter $n), breaking writes to contact_timeline entirely. Failing
	// at generation time keeps that out of the database.
	t.Run("index beyond the argument list", func(t *testing.T) {
		_, err := embedArgs("country = $9", []interface{}{"US", "FR"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "placeholder $9 has no argument (2 provided)")
	})

	t.Run("unparsable placeholder index", func(t *testing.T) {
		_, err := embedArgs("country = $99999999999999999999", []interface{}{"US"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unparsable placeholder")
	})

	t.Run("stray placeholder with no arguments at all", func(t *testing.T) {
		_, err := embedArgs("country = $1", nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "placeholder $1 has no argument (0 provided)")
	})
}

func BenchmarkEmbedArgs(b *testing.B) {
	sql := "NEW.kind = $1 AND NEW.email = $2 AND NEW.list_id = $3 AND NEW.status = $4"
	args := []interface{}{"contact.created", "test@example.com", "list123", "active"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := embedArgs(sql, args); err != nil {
			b.Fatal(err)
		}
	}
}

// A placeholder is only a placeholder outside a string literal. Object keys reach the SQL
// as inlined literals — QueryBuilder.buildJSONPath writes custom_json_1['<key>'] with
// nothing but doubled quotes protecting it — and those keys are caller-supplied. Expanding
// a "$1" key would splice another filter's value, quotes and all, into the middle of that
// literal and let it close the subscript and keep going.
func TestEmbedArgs_LeavesPlaceholdersInsideLiteralsAlone(t *testing.T) {
	t.Run("a placeholder-shaped object key stays data", func(t *testing.T) {
		sql := `custom_json_1['a']::text = $1 AND custom_json_2['$1']::text = $2`
		out, err := embedArgs(sql, []interface{}{"x", "y"})
		require.NoError(t, err)
		assert.Equal(t, `custom_json_1['a']::text = 'x' AND custom_json_2['$1']::text = 'y'`, out)
	})

	t.Run("an out-of-range placeholder inside a literal is not an error", func(t *testing.T) {
		// The range check must not fire on caller data either: '$9' as a key is a key.
		out, err := embedArgs(`col['$9']::text = $1`, []interface{}{"x"})
		require.NoError(t, err)
		assert.Equal(t, `col['$9']::text = 'x'`, out)
	})

	t.Run("a doubled quote does not close the literal", func(t *testing.T) {
		out, err := embedArgs(`col['a''$1b']::text = $1`, []interface{}{"x"})
		require.NoError(t, err)
		assert.Equal(t, `col['a''$1b']::text = 'x'`, out)
	})

	t.Run("an E-string backslash escape does not close the literal", func(t *testing.T) {
		out, err := embedArgs(`col = E'a\'$1' AND other = $1`, []interface{}{"x"})
		require.NoError(t, err)
		assert.Equal(t, `col = E'a\'$1' AND other = 'x'`, out)
	})

	t.Run("a leaked placeholder outside a literal is still an error", func(t *testing.T) {
		_, err := embedArgs(`col = $9`, []interface{}{"x"})
		require.Error(t, err)
	})
}

// End to end through the real compiler: the payload that made the guard unconditionally
// true. Two filters, the second carrying a json_path segment of literally "$1", so the
// first filter's value was expanded inside the second's subscript and the whole EXISTS
// collapsed to "... OR TRUE OR ...".
func TestAutomationTriggerGenerator_JSONPathKeyCannotForgeAPlaceholder(t *testing.T) {
	gen := NewAutomationTriggerGenerator(NewQueryBuilder())

	automation := &domain.Automation{
		ID:         "jsonpath1",
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
								FieldName:    "custom_json_1",
								FieldType:    "json",
								Operator:     "equals",
								JSONPath:     []string{"a"},
								StringValues: []string{`'] IS NOT NULL OR TRUE OR custom_json_2[`},
							},
							{
								FieldName:    "custom_json_2",
								FieldType:    "json",
								Operator:     "equals",
								JSONPath:     []string{"$1"},
								StringValues: []string{"y"},
							},
						},
					},
				},
			},
		},
	}

	// The tree is accepted: DimensionFilter.Validate only requires non-empty segments, so
	// nothing upstream stands between this payload and the generated SQL.
	require.NoError(t, automation.Trigger.Validate())

	result, err := gen.Generate(automation)
	require.NoError(t, err)

	assert.Contains(t, result.ConditionGuard, `custom_json_2['$1']`,
		"the key must survive as data, not become a placeholder")
	assert.NotContains(t, result.ConditionGuard, `custom_json_2['']`,
		"an expanded key is what let the injected text escape the subscript")
}

// A condition that cannot evaluate must not match — it must not abort the statement.
//
// Inside a trigger this is not a preference. contact_timeline is written by triggers on
// contacts, contact_lists, message_history, custom_events, contact_segments and
// inbound_webhook_events, so a cast that raises takes every one of those writes down with
// it for the contact concerned. The install probe cannot see it coming: EXPLAIN plans
// without executing, and the failure depends on the row.
//
// The defensive form already exists for custom_events.properties and contact_timeline.changes
// (see castMode in query_builder.go); the contact's own custom_json_* columns reached the
// trigger through the direct cast.
func TestAutomationTriggerGenerator_JSONCastsCannotAbortTheStatement(t *testing.T) {
	gen := NewAutomationTriggerGenerator(NewQueryBuilder())

	jsonAutomation := func(id, fieldType, operator string, filter *domain.DimensionFilter) *domain.Automation {
		return &domain.Automation{
			ID:         id,
			ListID:     "list1",
			RootNodeID: "node1",
			Trigger: &domain.TimelineTriggerConfig{
				EventKind: "contact.created",
				Frequency: domain.TriggerFrequencyOnce,
				Conditions: &domain.TreeNode{
					Kind: "leaf",
					Leaf: &domain.TreeNodeLeaf{
						Source:  "contacts",
						Contact: &domain.ContactCondition{Filters: []*domain.DimensionFilter{filter}},
					},
				},
			},
		}
	}

	t.Run("a numeric json_path comparison guards the cast", func(t *testing.T) {
		result, err := gen.Generate(jsonAutomation("jsonnum", "number", "gte", &domain.DimensionFilter{
			FieldName:    "custom_json_1",
			FieldType:    "number",
			Operator:     "gte",
			JSONPath:     []string{"score"},
			NumberValues: []float64{10},
		}))
		require.NoError(t, err)

		assert.Contains(t, result.ConditionGuard, "CASE WHEN",
			"a contact holding a non-numeric value must yield NULL, not abort the write")
		assert.NotContains(t, result.ConditionGuard, `(custom_json_1['score']::text)::numeric >=`,
			"the bare cast is what raises on the offending row")
	})

	t.Run("a date json_path comparison guards the cast", func(t *testing.T) {
		result, err := gen.Generate(jsonAutomation("jsondate", "time", "after_date", &domain.DimensionFilter{
			FieldName:    "custom_json_1",
			FieldType:    "time",
			Operator:     "after_date",
			JSONPath:     []string{"renewed_at"},
			StringValues: []string{"2026-01-01T00:00:00Z"},
		}))
		require.NoError(t, err)

		assert.Contains(t, result.ConditionGuard, "CASE WHEN",
			"a contact holding an unparseable date must yield NULL, not abort the write")
	})

	// The segment path is deliberately left alone: there a failed cast breaks one query
	// that a person is waiting on, which is a different trade-off from freezing a contact.
	t.Run("the segment compiler is unchanged", func(t *testing.T) {
		qb := NewQueryBuilder()
		sql, _, err := qb.BuildSQL(&domain.TreeNode{
			Kind: "leaf",
			Leaf: &domain.TreeNodeLeaf{
				Source: "contacts",
				Contact: &domain.ContactCondition{Filters: []*domain.DimensionFilter{{
					FieldName:    "custom_json_1",
					FieldType:    "number",
					Operator:     "gte",
					JSONPath:     []string{"score"},
					NumberValues: []float64{10},
				}}},
			},
		})
		require.NoError(t, err)
		assert.Contains(t, sql, `(custom_json_1['score']::text)::numeric`,
			"changing the segment path is out of scope for this fix")
	})
}
