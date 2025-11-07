package domain

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTreeNode_Validate(t *testing.T) {
	t.Run("valid simple leaf node", func(t *testing.T) {
		node := &TreeNode{
			Kind: "leaf",
			Leaf: &TreeNodeLeaf{
				Table: "contacts",
				Contact: &ContactCondition{
					Filters: []*DimensionFilter{
						{
							FieldName:    "country",
							FieldType:    "string",
							Operator:     "equals",
							StringValues: []string{"US"},
						},
					},
				},
			},
		}

		err := node.Validate()
		assert.NoError(t, err)
	})

	t.Run("valid branch node", func(t *testing.T) {
		node := &TreeNode{
			Kind: "branch",
			Branch: &TreeNodeBranch{
				Operator: "and",
				Leaves: []*TreeNode{
					{
						Kind: "leaf",
						Leaf: &TreeNodeLeaf{
							Table: "contacts",
							Contact: &ContactCondition{
								Filters: []*DimensionFilter{
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
						Leaf: &TreeNodeLeaf{
							Table: "contacts",
							Contact: &ContactCondition{
								Filters: []*DimensionFilter{
									{
										FieldName:    "orders_count",
										FieldType:    "number",
										Operator:     "gte",
										NumberValues: []float64{5},
									},
								},
							},
						},
					},
				},
			},
		}

		err := node.Validate()
		assert.NoError(t, err)
	})

	t.Run("missing kind", func(t *testing.T) {
		node := &TreeNode{}
		err := node.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must have 'kind'")
	})

	t.Run("invalid kind", func(t *testing.T) {
		node := &TreeNode{Kind: "invalid"}
		err := node.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid tree node kind")
	})

	t.Run("branch without branch field", func(t *testing.T) {
		node := &TreeNode{Kind: "branch"}
		err := node.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must have 'branch' field")
	})

	t.Run("leaf without leaf field", func(t *testing.T) {
		node := &TreeNode{Kind: "leaf"}
		err := node.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must have 'leaf' field")
	})
}

func TestTreeNodeBranch_Validate(t *testing.T) {
	t.Run("valid AND branch", func(t *testing.T) {
		branch := &TreeNodeBranch{
			Operator: "and",
			Leaves: []*TreeNode{
				{
					Kind: "leaf",
					Leaf: &TreeNodeLeaf{
						Table: "contacts",
						Contact: &ContactCondition{
							Filters: []*DimensionFilter{
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

		err := branch.Validate()
		assert.NoError(t, err)
	})

	t.Run("invalid operator", func(t *testing.T) {
		branch := &TreeNodeBranch{
			Operator: "invalid",
			Leaves: []*TreeNode{
				{
					Kind: "leaf",
					Leaf: &TreeNodeLeaf{
						Table: "contacts",
						Contact: &ContactCondition{
							Filters: []*DimensionFilter{
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

		err := branch.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid branch operator")
	})

	t.Run("empty leaves", func(t *testing.T) {
		branch := &TreeNodeBranch{
			Operator: "and",
			Leaves:   []*TreeNode{},
		}

		err := branch.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must have at least one leaf")
	})
}

func TestTreeNodeLeaf_Validate(t *testing.T) {
	t.Run("valid contacts leaf", func(t *testing.T) {
		leaf := &TreeNodeLeaf{
			Table: "contacts",
			Contact: &ContactCondition{
				Filters: []*DimensionFilter{
					{
						FieldName:    "country",
						FieldType:    "string",
						Operator:     "equals",
						StringValues: []string{"US"},
					},
				},
			},
		}

		err := leaf.Validate()
		assert.NoError(t, err)
	})

	t.Run("missing table", func(t *testing.T) {
		leaf := &TreeNodeLeaf{}
		err := leaf.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must have 'table'")
	})

	t.Run("invalid table", func(t *testing.T) {
		leaf := &TreeNodeLeaf{Table: "invalid"}
		err := leaf.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid table")
	})

	t.Run("contacts table without contact field", func(t *testing.T) {
		leaf := &TreeNodeLeaf{Table: "contacts"}
		err := leaf.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must have 'contact' field")
	})
}

func TestDimensionFilter_Validate(t *testing.T) {
	t.Run("valid string filter", func(t *testing.T) {
		filter := &DimensionFilter{
			FieldName:    "country",
			FieldType:    "string",
			Operator:     "equals",
			StringValues: []string{"US"},
		}

		err := filter.Validate()
		assert.NoError(t, err)
	})

	t.Run("valid number filter", func(t *testing.T) {
		filter := &DimensionFilter{
			FieldName:    "orders_count",
			FieldType:    "number",
			Operator:     "gte",
			NumberValues: []float64{5},
		}

		err := filter.Validate()
		assert.NoError(t, err)
	})

	t.Run("valid is_set filter (no values needed)", func(t *testing.T) {
		filter := &DimensionFilter{
			FieldName: "phone",
			FieldType: "string",
			Operator:  "is_set",
		}

		err := filter.Validate()
		assert.NoError(t, err)
	})

	t.Run("missing field_name", func(t *testing.T) {
		filter := &DimensionFilter{
			FieldType:    "string",
			Operator:     "equals",
			StringValues: []string{"US"},
		}

		err := filter.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must have 'field_name'")
	})

	t.Run("missing field_type", func(t *testing.T) {
		filter := &DimensionFilter{
			FieldName:    "country",
			Operator:     "equals",
			StringValues: []string{"US"},
		}

		err := filter.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must have 'field_type'")
	})

	t.Run("invalid field_type", func(t *testing.T) {
		filter := &DimensionFilter{
			FieldName:    "country",
			FieldType:    "invalid",
			Operator:     "equals",
			StringValues: []string{"US"},
		}

		err := filter.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid field_type")
	})

	t.Run("missing operator", func(t *testing.T) {
		filter := &DimensionFilter{
			FieldName:    "country",
			FieldType:    "string",
			StringValues: []string{"US"},
		}

		err := filter.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must have 'operator'")
	})

	t.Run("string filter without values", func(t *testing.T) {
		filter := &DimensionFilter{
			FieldName: "country",
			FieldType: "string",
			Operator:  "equals",
		}

		err := filter.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must have 'string_values'")
	})

	t.Run("number filter without values", func(t *testing.T) {
		filter := &DimensionFilter{
			FieldName: "orders_count",
			FieldType: "number",
			Operator:  "gte",
		}

		err := filter.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must have 'number_values'")
	})

	// JSON filter tests
	t.Run("valid JSON filter with json_path", func(t *testing.T) {
		filter := &DimensionFilter{
			FieldName:    "custom_json_1",
			FieldType:    "json",
			Operator:     "equals",
			JSONPath:     []string{"user", "name"},
			StringValues: []string{"John"},
		}

		err := filter.Validate()
		assert.NoError(t, err)
	})

	t.Run("valid JSON filter with number type casting", func(t *testing.T) {
		filter := &DimensionFilter{
			FieldName:    "custom_json_2",
			FieldType:    "number",
			Operator:     "gt",
			JSONPath:     []string{"age"},
			NumberValues: []float64{25},
		}

		err := filter.Validate()
		assert.NoError(t, err)
	})

	t.Run("valid JSON filter with time type casting", func(t *testing.T) {
		filter := &DimensionFilter{
			FieldName:    "custom_json_3",
			FieldType:    "time",
			Operator:     "lt",
			JSONPath:     []string{"last_login"},
			StringValues: []string{"2024-01-01T00:00:00Z"},
		}

		err := filter.Validate()
		assert.NoError(t, err)
	})

	t.Run("valid JSON filter with array index in path", func(t *testing.T) {
		filter := &DimensionFilter{
			FieldName:    "custom_json_4",
			FieldType:    "json",
			Operator:     "equals",
			JSONPath:     []string{"items", "0", "name"},
			StringValues: []string{"Product A"},
		}

		err := filter.Validate()
		assert.NoError(t, err)
	})

	t.Run("JSON filter with invalid field_name", func(t *testing.T) {
		filter := &DimensionFilter{
			FieldName:    "invalid_field",
			FieldType:    "json",
			Operator:     "equals",
			JSONPath:     []string{"name"},
			StringValues: []string{"John"},
		}

		err := filter.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "can only be used with custom_json fields")
	})

	t.Run("json_path used with non-JSON field", func(t *testing.T) {
		filter := &DimensionFilter{
			FieldName:    "country",
			FieldType:    "string",
			Operator:     "equals",
			JSONPath:     []string{"name"},
			StringValues: []string{"US"},
		}

		err := filter.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "can only be used with custom_json fields")
	})

	t.Run("JSON filter with empty json_path segment", func(t *testing.T) {
		filter := &DimensionFilter{
			FieldName:    "custom_json_1",
			FieldType:    "json",
			Operator:     "equals",
			JSONPath:     []string{"user", "", "name"},
			StringValues: []string{"John"},
		}

		err := filter.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "json_path segment 1 is empty")
	})

	t.Run("JSON filter with missing json_path (non-existence check)", func(t *testing.T) {
		filter := &DimensionFilter{
			FieldName:    "custom_json_1",
			FieldType:    "json",
			Operator:     "equals",
			StringValues: []string{"John"},
		}

		err := filter.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must have 'json_path'")
	})

	t.Run("JSON filter with is_set operator (no json_path required)", func(t *testing.T) {
		filter := &DimensionFilter{
			FieldName: "custom_json_1",
			FieldType: "json",
			Operator:  "is_set",
		}

		err := filter.Validate()
		assert.NoError(t, err)
	})

	t.Run("valid in_array operator", func(t *testing.T) {
		filter := &DimensionFilter{
			FieldName:    "custom_json_1",
			FieldType:    "json",
			Operator:     "in_array",
			JSONPath:     []string{"tags"},
			StringValues: []string{"premium"},
		}

		err := filter.Validate()
		assert.NoError(t, err)
	})
}

func TestTreeNode_JSONMarshaling(t *testing.T) {
	t.Run("marshal and unmarshal simple leaf", func(t *testing.T) {
		original := &TreeNode{
			Kind: "leaf",
			Leaf: &TreeNodeLeaf{
				Table: "contacts",
				Contact: &ContactCondition{
					Filters: []*DimensionFilter{
						{
							FieldName:    "country",
							FieldType:    "string",
							Operator:     "equals",
							StringValues: []string{"US"},
						},
					},
				},
			},
		}

		// Marshal to JSON
		jsonData, err := json.Marshal(original)
		require.NoError(t, err)

		// Unmarshal back
		var restored TreeNode
		err = json.Unmarshal(jsonData, &restored)
		require.NoError(t, err)

		// Verify
		assert.Equal(t, original.Kind, restored.Kind)
		assert.NotNil(t, restored.Leaf)
		assert.Equal(t, original.Leaf.Table, restored.Leaf.Table)
		assert.NotNil(t, restored.Leaf.Contact)
		assert.Len(t, restored.Leaf.Contact.Filters, 1)
		assert.Equal(t, "country", restored.Leaf.Contact.Filters[0].FieldName)
	})

	t.Run("marshal and unmarshal complex branch", func(t *testing.T) {
		original := &TreeNode{
			Kind: "branch",
			Branch: &TreeNodeBranch{
				Operator: "and",
				Leaves: []*TreeNode{
					{
						Kind: "leaf",
						Leaf: &TreeNodeLeaf{
							Table: "contacts",
							Contact: &ContactCondition{
								Filters: []*DimensionFilter{
									{
										FieldName:    "orders_count",
										FieldType:    "number",
										Operator:     "gte",
										NumberValues: []float64{5},
									},
								},
							},
						},
					},
					{
						Kind: "leaf",
						Leaf: &TreeNodeLeaf{
							Table: "contacts",
							Contact: &ContactCondition{
								Filters: []*DimensionFilter{
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
			},
		}

		// Marshal to JSON
		jsonData, err := json.Marshal(original)
		require.NoError(t, err)

		// Unmarshal back
		var restored TreeNode
		err = json.Unmarshal(jsonData, &restored)
		require.NoError(t, err)

		// Verify
		assert.Equal(t, original.Kind, restored.Kind)
		assert.NotNil(t, restored.Branch)
		assert.Equal(t, "and", restored.Branch.Operator)
		assert.Len(t, restored.Branch.Leaves, 2)
	})
}

func TestTreeNode_ToMapOfAny(t *testing.T) {
	t.Run("convert to MapOfAny", func(t *testing.T) {
		node := &TreeNode{
			Kind: "leaf",
			Leaf: &TreeNodeLeaf{
				Table: "contacts",
				Contact: &ContactCondition{
					Filters: []*DimensionFilter{
						{
							FieldName:    "country",
							FieldType:    "string",
							Operator:     "equals",
							StringValues: []string{"US"},
						},
					},
				},
			},
		}

		mapData, err := node.ToMapOfAny()
		require.NoError(t, err)
		assert.Equal(t, "leaf", mapData["kind"])
		assert.NotNil(t, mapData["leaf"])
	})
}

func TestTreeNodeFromMapOfAny(t *testing.T) {
	t.Run("convert from MapOfAny", func(t *testing.T) {
		mapData := MapOfAny{
			"kind": "leaf",
			"leaf": map[string]interface{}{
				"table": "contacts",
				"contact": map[string]interface{}{
					"filters": []interface{}{
						map[string]interface{}{
							"field_name":    "country",
							"field_type":    "string",
							"operator":      "equals",
							"string_values": []interface{}{"US"},
						},
					},
				},
			},
		}

		node, err := TreeNodeFromMapOfAny(mapData)
		require.NoError(t, err)
		assert.Equal(t, "leaf", node.Kind)
		assert.NotNil(t, node.Leaf)
		assert.Equal(t, "contacts", node.Leaf.Table)
	})
}

func TestTreeNodeFromJSON(t *testing.T) {
	t.Run("parse from JSON string", func(t *testing.T) {
		jsonStr := `{
			"kind": "leaf",
			"leaf": {
				"table": "contacts",
				"contact": {
					"filters": [{
						"field_name": "country",
						"field_type": "string",
						"operator": "equals",
						"string_values": ["US"]
					}]
				}
			}
		}`

		node, err := TreeNodeFromJSON(jsonStr)
		require.NoError(t, err)
		assert.Equal(t, "leaf", node.Kind)
		assert.NotNil(t, node.Leaf)
		assert.Equal(t, "contacts", node.Leaf.Table)

		// Validate it
		err = node.Validate()
		assert.NoError(t, err)
	})

	t.Run("invalid JSON", func(t *testing.T) {
		jsonStr := `{invalid json`
		_, err := TreeNodeFromJSON(jsonStr)
		require.Error(t, err)
	})
}

func TestTreeNode_HasRelativeDates(t *testing.T) {
	t.Run("returns true for in_the_last_days operator", func(t *testing.T) {
		inTheLastDays := "in_the_last_days"
		node := &TreeNode{
			Kind: "leaf",
			Leaf: &TreeNodeLeaf{
				Table: "contact_timeline",
				ContactTimeline: &ContactTimelineCondition{
					Kind:              "open_email",
					CountOperator:     "at_least",
					CountValue:        1,
					TimeframeOperator: &inTheLastDays,
					TimeframeValues:   []string{"7"},
				},
			},
		}

		assert.True(t, node.HasRelativeDates())
	})

	t.Run("returns false for anytime operator", func(t *testing.T) {
		anytime := "anytime"
		node := &TreeNode{
			Kind: "leaf",
			Leaf: &TreeNodeLeaf{
				Table: "contact_timeline",
				ContactTimeline: &ContactTimelineCondition{
					Kind:              "open_email",
					CountOperator:     "at_least",
					CountValue:        1,
					TimeframeOperator: &anytime,
				},
			},
		}

		assert.False(t, node.HasRelativeDates())
	})

	t.Run("returns false for contact conditions without relative dates", func(t *testing.T) {
		node := &TreeNode{
			Kind: "leaf",
			Leaf: &TreeNodeLeaf{
				Table: "contacts",
				Contact: &ContactCondition{
					Filters: []*DimensionFilter{
						{
							FieldName:    "country",
							FieldType:    "string",
							Operator:     "equals",
							StringValues: []string{"US"},
						},
					},
				},
			},
		}

		assert.False(t, node.HasRelativeDates())
	})

	t.Run("returns true for contact property with in_the_last_days filter", func(t *testing.T) {
		node := &TreeNode{
			Kind: "leaf",
			Leaf: &TreeNodeLeaf{
				Table: "contacts",
				Contact: &ContactCondition{
					Filters: []*DimensionFilter{
						{
							FieldName:    "created_at",
							FieldType:    "time",
							Operator:     "in_the_last_days",
							StringValues: []string{"30"},
						},
					},
				},
			},
		}

		assert.True(t, node.HasRelativeDates())
	})

	t.Run("returns true for contact with multiple filters including relative date", func(t *testing.T) {
		node := &TreeNode{
			Kind: "leaf",
			Leaf: &TreeNodeLeaf{
				Table: "contacts",
				Contact: &ContactCondition{
					Filters: []*DimensionFilter{
						{
							FieldName:    "country",
							FieldType:    "string",
							Operator:     "equals",
							StringValues: []string{"US"},
						},
						{
							FieldName:    "created_at",
							FieldType:    "time",
							Operator:     "in_the_last_days",
							StringValues: []string{"7"},
						},
					},
				},
			},
		}

		assert.True(t, node.HasRelativeDates())
	})

	t.Run("returns true for branch with relative dates in one leaf", func(t *testing.T) {
		inTheLastDays := "in_the_last_days"
		node := &TreeNode{
			Kind: "branch",
			Branch: &TreeNodeBranch{
				Operator: "and",
				Leaves: []*TreeNode{
					{
						Kind: "leaf",
						Leaf: &TreeNodeLeaf{
							Table: "contacts",
							Contact: &ContactCondition{
								Filters: []*DimensionFilter{
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
						Leaf: &TreeNodeLeaf{
							Table: "contact_timeline",
							ContactTimeline: &ContactTimelineCondition{
								Kind:              "open_email",
								CountOperator:     "at_least",
								CountValue:        1,
								TimeframeOperator: &inTheLastDays,
								TimeframeValues:   []string{"7"},
							},
						},
					},
				},
			},
		}

		assert.True(t, node.HasRelativeDates())
	})

	t.Run("returns false for branch without relative dates", func(t *testing.T) {
		node := &TreeNode{
			Kind: "branch",
			Branch: &TreeNodeBranch{
				Operator: "and",
				Leaves: []*TreeNode{
					{
						Kind: "leaf",
						Leaf: &TreeNodeLeaf{
							Table: "contacts",
							Contact: &ContactCondition{
								Filters: []*DimensionFilter{
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
						Leaf: &TreeNodeLeaf{
							Table: "contact_lists",
							ContactList: &ContactListCondition{
								Operator: "in",
								ListID:   "test-list",
							},
						},
					},
				},
			},
		}

		assert.False(t, node.HasRelativeDates())
	})

	t.Run("returns false for nil node", func(t *testing.T) {
		var node *TreeNode
		assert.False(t, node.HasRelativeDates())
	})

	t.Run("returns false for nil leaf", func(t *testing.T) {
		node := &TreeNode{
			Kind: "leaf",
			Leaf: nil,
		}

		assert.False(t, node.HasRelativeDates())
	})

	t.Run("returns false for nil branch", func(t *testing.T) {
		node := &TreeNode{
			Kind:   "branch",
			Branch: nil,
		}

		assert.False(t, node.HasRelativeDates())
	})
}
