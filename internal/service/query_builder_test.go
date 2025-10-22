package service

import (
	"strings"
	"testing"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQueryBuilder_BuildSQL_SimpleConditions(t *testing.T) {
	qb := NewQueryBuilder()

	t.Run("single string equals condition", func(t *testing.T) {
		tree := &domain.TreeNode{
			Kind: "leaf",
			Leaf: &domain.TreeNodeLeaf{
				Table: "contacts",
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
		}

		sql, args, err := qb.BuildSQL(tree)
		require.NoError(t, err)
		assert.Equal(t, "SELECT email FROM contacts WHERE (country = $1)", sql)
		assert.Equal(t, []interface{}{"US"}, args)
	})

	t.Run("single number gte condition", func(t *testing.T) {
		tree := &domain.TreeNode{
			Kind: "leaf",
			Leaf: &domain.TreeNodeLeaf{
				Table: "contacts",
				Contact: &domain.ContactCondition{
					Filters: []*domain.DimensionFilter{
						{
							FieldName:    "orders_count",
							FieldType:    "number",
							Operator:     "gte",
							NumberValues: []float64{5.0},
						},
					},
				},
			},
		}

		sql, args, err := qb.BuildSQL(tree)
		require.NoError(t, err)
		assert.Equal(t, "SELECT email FROM contacts WHERE (orders_count >= $1)", sql)
		assert.Equal(t, []interface{}{5.0}, args)
	})

	t.Run("is_set condition (no value needed)", func(t *testing.T) {
		tree := &domain.TreeNode{
			Kind: "leaf",
			Leaf: &domain.TreeNodeLeaf{
				Table: "contacts",
				Contact: &domain.ContactCondition{
					Filters: []*domain.DimensionFilter{
						{
							FieldName: "phone",
							FieldType: "string",
							Operator:  "is_set",
						},
					},
				},
			},
		}

		sql, args, err := qb.BuildSQL(tree)
		require.NoError(t, err)
		assert.Equal(t, "SELECT email FROM contacts WHERE (phone IS NOT NULL)", sql)
		assert.Empty(t, args)
	})

	t.Run("contains condition with wildcards", func(t *testing.T) {
		tree := &domain.TreeNode{
			Kind: "leaf",
			Leaf: &domain.TreeNodeLeaf{
				Table: "contacts",
				Contact: &domain.ContactCondition{
					Filters: []*domain.DimensionFilter{
						{
							FieldName:    "email",
							FieldType:    "string",
							Operator:     "contains",
							StringValues: []string{"@example.com"},
						},
					},
				},
			},
		}

		sql, args, err := qb.BuildSQL(tree)
		require.NoError(t, err)
		assert.Equal(t, "SELECT email FROM contacts WHERE (email ILIKE $1)", sql)
		assert.Equal(t, []interface{}{"%@example.com%"}, args)
	})

	t.Run("contains with multiple values (OR logic)", func(t *testing.T) {
		tree := &domain.TreeNode{
			Kind: "leaf",
			Leaf: &domain.TreeNodeLeaf{
				Table: "contacts",
				Contact: &domain.ContactCondition{
					Filters: []*domain.DimensionFilter{
						{
							FieldName:    "country",
							FieldType:    "string",
							Operator:     "contains",
							StringValues: []string{"United", "States", "America"},
						},
					},
				},
			},
		}

		sql, args, err := qb.BuildSQL(tree)
		require.NoError(t, err)
		assert.Contains(t, sql, "country ILIKE $1")
		assert.Contains(t, sql, "country ILIKE $2")
		assert.Contains(t, sql, "country ILIKE $3")
		assert.Contains(t, sql, " OR ")
		assert.Equal(t, []interface{}{"%United%", "%States%", "%America%"}, args)
	})

	t.Run("not_contains with multiple values (OR logic)", func(t *testing.T) {
		tree := &domain.TreeNode{
			Kind: "leaf",
			Leaf: &domain.TreeNodeLeaf{
				Table: "contacts",
				Contact: &domain.ContactCondition{
					Filters: []*domain.DimensionFilter{
						{
							FieldName:    "email",
							FieldType:    "string",
							Operator:     "not_contains",
							StringValues: []string{"spam", "test"},
						},
					},
				},
			},
		}

		sql, args, err := qb.BuildSQL(tree)
		require.NoError(t, err)
		assert.Contains(t, sql, "email NOT ILIKE $1")
		assert.Contains(t, sql, "email NOT ILIKE $2")
		assert.Contains(t, sql, " OR ")
		assert.Equal(t, []interface{}{"%spam%", "%test%"}, args)
	})

	t.Run("time in_date_range condition", func(t *testing.T) {
		tree := &domain.TreeNode{
			Kind: "leaf",
			Leaf: &domain.TreeNodeLeaf{
				Table: "contacts",
				Contact: &domain.ContactCondition{
					Filters: []*domain.DimensionFilter{
						{
							FieldName:    "created_at",
							FieldType:    "time",
							Operator:     "in_date_range",
							StringValues: []string{"2023-01-01", "2023-12-31"},
						},
					},
				},
			},
		}

		sql, args, err := qb.BuildSQL(tree)
		require.NoError(t, err)
		assert.Contains(t, sql, "created_at BETWEEN $1 AND $2")
		assert.Len(t, args, 2)
	})

	t.Run("time in_the_last_days condition", func(t *testing.T) {
		tree := &domain.TreeNode{
			Kind: "leaf",
			Leaf: &domain.TreeNodeLeaf{
				Table: "contacts",
				Contact: &domain.ContactCondition{
					Filters: []*domain.DimensionFilter{
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

		sql, args, err := qb.BuildSQL(tree)
		require.NoError(t, err)
		assert.Contains(t, sql, "created_at > NOW() - INTERVAL '30 days'")
		assert.Empty(t, args) // No args needed as days value is embedded in SQL
	})

	t.Run("time in_the_last_days with numeric value", func(t *testing.T) {
		tree := &domain.TreeNode{
			Kind: "leaf",
			Leaf: &domain.TreeNodeLeaf{
				Table: "contacts",
				Contact: &domain.ContactCondition{
					Filters: []*domain.DimensionFilter{
						{
							FieldName:    "updated_at",
							FieldType:    "time",
							Operator:     "in_the_last_days",
							StringValues: []string{"7"},
						},
					},
				},
			},
		}

		sql, args, err := qb.BuildSQL(tree)
		require.NoError(t, err)
		assert.Contains(t, sql, "updated_at > NOW() - INTERVAL '7 days'")
		assert.Empty(t, args)
	})
}

func TestQueryBuilder_BuildSQL_MultipleFilters(t *testing.T) {
	qb := NewQueryBuilder()

	t.Run("multiple filters in single contact condition (ANDed)", func(t *testing.T) {
		tree := &domain.TreeNode{
			Kind: "leaf",
			Leaf: &domain.TreeNodeLeaf{
				Table: "contacts",
				Contact: &domain.ContactCondition{
					Filters: []*domain.DimensionFilter{
						{
							FieldName:    "country",
							FieldType:    "string",
							Operator:     "equals",
							StringValues: []string{"US"},
						},
						{
							FieldName:    "orders_count",
							FieldType:    "number",
							Operator:     "gte",
							NumberValues: []float64{5.0},
						},
					},
				},
			},
		}

		sql, args, err := qb.BuildSQL(tree)
		require.NoError(t, err)
		assert.Contains(t, sql, "country = $1")
		assert.Contains(t, sql, "orders_count >= $2")
		assert.Contains(t, sql, " AND ")
		assert.Equal(t, []interface{}{"US", 5.0}, args)
	})

	t.Run("contains with multiple values combined with other filter", func(t *testing.T) {
		tree := &domain.TreeNode{
			Kind: "leaf",
			Leaf: &domain.TreeNodeLeaf{
				Table: "contacts",
				Contact: &domain.ContactCondition{
					Filters: []*domain.DimensionFilter{
						{
							FieldName:    "country",
							FieldType:    "string",
							Operator:     "contains",
							StringValues: []string{"United", "States"},
						},
						{
							FieldName:    "orders_count",
							FieldType:    "number",
							Operator:     "gte",
							NumberValues: []float64{5.0},
						},
					},
				},
			},
		}

		sql, args, err := qb.BuildSQL(tree)
		require.NoError(t, err)
		// Contains with multiple values should be wrapped in parentheses
		assert.Contains(t, sql, "(country ILIKE $1 OR country ILIKE $2)")
		assert.Contains(t, sql, "orders_count >= $3")
		assert.Contains(t, sql, " AND ")
		assert.Equal(t, []interface{}{"%United%", "%States%", 5.0}, args)
	})
}

func TestQueryBuilder_BuildSQL_MultipleValuesContains(t *testing.T) {
	qb := NewQueryBuilder()

	t.Run("OR branch with contains having multiple values", func(t *testing.T) {
		// Test realistic scenario: country contains "USA" OR "Canada" OR "Mexico"
		// combined with another OR branch for different criteria
		tree := &domain.TreeNode{
			Kind: "branch",
			Branch: &domain.TreeNodeBranch{
				Operator: "or",
				Leaves: []*domain.TreeNode{
					{
						Kind: "leaf",
						Leaf: &domain.TreeNodeLeaf{
							Table: "contacts",
							Contact: &domain.ContactCondition{
								Filters: []*domain.DimensionFilter{
									{
										FieldName:    "country",
										FieldType:    "string",
										Operator:     "contains",
										StringValues: []string{"USA", "Canada", "Mexico"},
									},
								},
							},
						},
					},
					{
						Kind: "leaf",
						Leaf: &domain.TreeNodeLeaf{
							Table: "contacts",
							Contact: &domain.ContactCondition{
								Filters: []*domain.DimensionFilter{
									{
										FieldName:    "email",
										FieldType:    "string",
										Operator:     "contains",
										StringValues: []string{"@vip.com", "@premium.com"},
									},
								},
							},
						},
					},
				},
			},
		}

		sql, args, err := qb.BuildSQL(tree)
		require.NoError(t, err)

		// Should have proper parentheses structure
		assert.Contains(t, sql, "(country ILIKE $1 OR country ILIKE $2 OR country ILIKE $3)")
		assert.Contains(t, sql, "(email ILIKE $4 OR email ILIKE $5)")
		assert.Contains(t, sql, " OR ")

		// Check args are properly ordered
		assert.Equal(t, []interface{}{
			"%USA%", "%Canada%", "%Mexico%",
			"%@vip.com%", "%@premium.com%",
		}, args)
	})
}

func TestQueryBuilder_BuildSQL_BranchConditions(t *testing.T) {
	qb := NewQueryBuilder()

	t.Run("AND branch with two leaves", func(t *testing.T) {
		tree := &domain.TreeNode{
			Kind: "branch",
			Branch: &domain.TreeNodeBranch{
				Operator: "and",
				Leaves: []*domain.TreeNode{
					{
						Kind: "leaf",
						Leaf: &domain.TreeNodeLeaf{
							Table: "contacts",
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
							Table: "contacts",
							Contact: &domain.ContactCondition{
								Filters: []*domain.DimensionFilter{
									{
										FieldName:    "orders_count",
										FieldType:    "number",
										Operator:     "gte",
										NumberValues: []float64{5.0},
									},
								},
							},
						},
					},
				},
			},
		}

		sql, args, err := qb.BuildSQL(tree)
		require.NoError(t, err)
		assert.Contains(t, sql, "country = $1")
		assert.Contains(t, sql, "orders_count >= $2")
		assert.Contains(t, sql, " AND ")
		assert.Equal(t, []interface{}{"US", 5.0}, args)
	})

	t.Run("OR branch with two leaves", func(t *testing.T) {
		tree := &domain.TreeNode{
			Kind: "branch",
			Branch: &domain.TreeNodeBranch{
				Operator: "or",
				Leaves: []*domain.TreeNode{
					{
						Kind: "leaf",
						Leaf: &domain.TreeNodeLeaf{
							Table: "contacts",
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
							Table: "contacts",
							Contact: &domain.ContactCondition{
								Filters: []*domain.DimensionFilter{
									{
										FieldName:    "country",
										FieldType:    "string",
										Operator:     "equals",
										StringValues: []string{"CA"},
									},
								},
							},
						},
					},
				},
			},
		}

		sql, args, err := qb.BuildSQL(tree)
		require.NoError(t, err)
		assert.Contains(t, sql, "country = $1")
		assert.Contains(t, sql, "country = $2")
		assert.Contains(t, sql, " OR ")
		assert.Equal(t, []interface{}{"US", "CA"}, args)
	})

	t.Run("nested branches (complex tree)", func(t *testing.T) {
		// (country = US AND orders >= 5) OR (country = CA AND orders >= 10)
		tree := &domain.TreeNode{
			Kind: "branch",
			Branch: &domain.TreeNodeBranch{
				Operator: "or",
				Leaves: []*domain.TreeNode{
					{
						Kind: "branch",
						Branch: &domain.TreeNodeBranch{
							Operator: "and",
							Leaves: []*domain.TreeNode{
								{
									Kind: "leaf",
									Leaf: &domain.TreeNodeLeaf{
										Table: "contacts",
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
										Table: "contacts",
										Contact: &domain.ContactCondition{
											Filters: []*domain.DimensionFilter{
												{
													FieldName:    "orders_count",
													FieldType:    "number",
													Operator:     "gte",
													NumberValues: []float64{5.0},
												},
											},
										},
									},
								},
							},
						},
					},
					{
						Kind: "branch",
						Branch: &domain.TreeNodeBranch{
							Operator: "and",
							Leaves: []*domain.TreeNode{
								{
									Kind: "leaf",
									Leaf: &domain.TreeNodeLeaf{
										Table: "contacts",
										Contact: &domain.ContactCondition{
											Filters: []*domain.DimensionFilter{
												{
													FieldName:    "country",
													FieldType:    "string",
													Operator:     "equals",
													StringValues: []string{"CA"},
												},
											},
										},
									},
								},
								{
									Kind: "leaf",
									Leaf: &domain.TreeNodeLeaf{
										Table: "contacts",
										Contact: &domain.ContactCondition{
											Filters: []*domain.DimensionFilter{
												{
													FieldName:    "orders_count",
													FieldType:    "number",
													Operator:     "gte",
													NumberValues: []float64{10.0},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		}

		sql, args, err := qb.BuildSQL(tree)
		require.NoError(t, err)

		// Should have nested parentheses
		assert.True(t, strings.Contains(sql, "(") && strings.Contains(sql, ")"))
		assert.Contains(t, sql, " OR ")
		assert.Contains(t, sql, " AND ")

		// All 4 conditions should be present
		assert.Contains(t, sql, "country = $1")
		assert.Contains(t, sql, "orders_count >= $2")
		assert.Contains(t, sql, "country = $3")
		assert.Contains(t, sql, "orders_count >= $4")

		assert.Equal(t, []interface{}{"US", 5.0, "CA", 10.0}, args)
	})
}

func TestQueryBuilder_BuildSQL_AllOperators(t *testing.T) {
	qb := NewQueryBuilder()

	tests := []struct {
		name     string
		operator string
		sqlPart  string
	}{
		{"equals", "equals", "="},
		{"not_equals", "not_equals", "!="},
		{"gt", "gt", ">"},
		{"gte", "gte", ">="},
		{"lt", "lt", "<"},
		{"lte", "lte", "<="},
		{"contains", "contains", "ILIKE"},
		{"not_contains", "not_contains", "NOT ILIKE"},
		{"is_set", "is_set", "IS NOT NULL"},
		{"is_not_set", "is_not_set", "IS NULL"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filter := &domain.DimensionFilter{
				FieldName: "country",
				FieldType: "string",
				Operator:  tt.operator,
			}

			// Add values for operators that need them
			if tt.operator != "is_set" && tt.operator != "is_not_set" {
				filter.StringValues = []string{"test"}
			}

			tree := &domain.TreeNode{
				Kind: "leaf",
				Leaf: &domain.TreeNodeLeaf{
					Table: "contacts",
					Contact: &domain.ContactCondition{
						Filters: []*domain.DimensionFilter{filter},
					},
				},
			}

			sql, _, err := qb.BuildSQL(tree)
			require.NoError(t, err)
			assert.Contains(t, sql, tt.sqlPart)
		})
	}
}

func TestQueryBuilder_BuildSQL_Validation(t *testing.T) {
	qb := NewQueryBuilder()

	t.Run("nil tree", func(t *testing.T) {
		_, _, err := qb.BuildSQL(nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "tree cannot be nil")
	})

	t.Run("invalid tree structure", func(t *testing.T) {
		tree := &domain.TreeNode{
			Kind: "leaf",
			// Missing Leaf field
		}

		_, _, err := qb.BuildSQL(tree)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid tree")
	})

	t.Run("invalid field name", func(t *testing.T) {
		tree := &domain.TreeNode{
			Kind: "leaf",
			Leaf: &domain.TreeNodeLeaf{
				Table: "contacts",
				Contact: &domain.ContactCondition{
					Filters: []*domain.DimensionFilter{
						{
							FieldName:    "invalid_field_name",
							FieldType:    "string",
							Operator:     "equals",
							StringValues: []string{"test"},
						},
					},
				},
			},
		}

		_, _, err := qb.BuildSQL(tree)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid field name")
	})

	t.Run("invalid operator", func(t *testing.T) {
		tree := &domain.TreeNode{
			Kind: "leaf",
			Leaf: &domain.TreeNodeLeaf{
				Table: "contacts",
				Contact: &domain.ContactCondition{
					Filters: []*domain.DimensionFilter{
						{
							FieldName:    "country",
							FieldType:    "string",
							Operator:     "invalid_operator",
							StringValues: []string{"test"},
						},
					},
				},
			},
		}

		_, _, err := qb.BuildSQL(tree)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid operator")
	})

	t.Run("unsupported table", func(t *testing.T) {
		tree := &domain.TreeNode{
			Kind: "leaf",
			Leaf: &domain.TreeNodeLeaf{
				Table: "unsupported_table",
				Contact: &domain.ContactCondition{
					Filters: []*domain.DimensionFilter{
						{
							FieldName:    "country",
							FieldType:    "string",
							Operator:     "equals",
							StringValues: []string{"test"},
						},
					},
				},
			},
		}

		_, _, err := qb.BuildSQL(tree)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid table")
	})

	t.Run("contains with no values", func(t *testing.T) {
		tree := &domain.TreeNode{
			Kind: "leaf",
			Leaf: &domain.TreeNodeLeaf{
				Table: "contacts",
				Contact: &domain.ContactCondition{
					Filters: []*domain.DimensionFilter{
						{
							FieldName:    "country",
							FieldType:    "string",
							Operator:     "contains",
							StringValues: []string{},
						},
					},
				},
			},
		}

		_, _, err := qb.BuildSQL(tree)
		require.Error(t, err)
		// Error comes from tree validation, not from buildCondition
		assert.Contains(t, err.Error(), "must have 'string_values'")
	})
}

func TestQueryBuilder_BuildSQL_ParameterizedQueries(t *testing.T) {
	qb := NewQueryBuilder()

	t.Run("parameterized values prevent SQL injection", func(t *testing.T) {
		// Even with malicious input, it should be safely parameterized
		tree := &domain.TreeNode{
			Kind: "leaf",
			Leaf: &domain.TreeNodeLeaf{
				Table: "contacts",
				Contact: &domain.ContactCondition{
					Filters: []*domain.DimensionFilter{
						{
							FieldName:    "country",
							FieldType:    "string",
							Operator:     "equals",
							StringValues: []string{"'; DROP TABLE contacts; --"},
						},
					},
				},
			},
		}

		sql, args, err := qb.BuildSQL(tree)
		require.NoError(t, err)

		// SQL should use parameter
		assert.Contains(t, sql, "$1")
		assert.NotContains(t, sql, "DROP TABLE")

		// Malicious input should be in args (safely parameterized)
		assert.Equal(t, []interface{}{"'; DROP TABLE contacts; --"}, args)
	})

	t.Run("parameter indices increment correctly", func(t *testing.T) {
		tree := &domain.TreeNode{
			Kind: "leaf",
			Leaf: &domain.TreeNodeLeaf{
				Table: "contacts",
				Contact: &domain.ContactCondition{
					Filters: []*domain.DimensionFilter{
						{
							FieldName:    "country",
							FieldType:    "string",
							Operator:     "equals",
							StringValues: []string{"US"},
						},
						{
							FieldName:    "state",
							FieldType:    "string",
							Operator:     "equals",
							StringValues: []string{"CA"},
						},
						{
							FieldName:    "orders_count",
							FieldType:    "number",
							Operator:     "gte",
							NumberValues: []float64{5.0},
						},
					},
				},
			},
		}

		sql, args, err := qb.BuildSQL(tree)
		require.NoError(t, err)

		assert.Contains(t, sql, "$1")
		assert.Contains(t, sql, "$2")
		assert.Contains(t, sql, "$3")
		assert.Equal(t, []interface{}{"US", "CA", 5.0}, args)
	})
}

func TestQueryBuilder_ContactLists(t *testing.T) {
	qb := NewQueryBuilder()

	t.Run("contact in list with ID only", func(t *testing.T) {
		tree := &domain.TreeNode{
			Kind: "leaf",
			Leaf: &domain.TreeNodeLeaf{
				Table: "contact_lists",
				ContactList: &domain.ContactListCondition{
					Operator: "in",
					ListID:   "list123",
				},
			},
		}

		sql, args, err := qb.BuildSQL(tree)
		require.NoError(t, err)

		// Should generate EXISTS subquery
		assert.Contains(t, sql, "EXISTS")
		assert.Contains(t, sql, "FROM contact_lists cl")
		assert.Contains(t, sql, "JOIN lists l ON cl.list_id = l.id")
		assert.Contains(t, sql, "WHERE cl.email = contacts.email")
		assert.Contains(t, sql, "cl.list_id = $1")
		assert.Contains(t, sql, "l.deleted_at IS NULL")
		assert.Equal(t, []interface{}{"list123"}, args)
	})

	t.Run("contact in list with status filter", func(t *testing.T) {
		status := "active"
		tree := &domain.TreeNode{
			Kind: "leaf",
			Leaf: &domain.TreeNodeLeaf{
				Table: "contact_lists",
				ContactList: &domain.ContactListCondition{
					Operator: "in",
					ListID:   "list456",
					Status:   &status,
				},
			},
		}

		sql, args, err := qb.BuildSQL(tree)
		require.NoError(t, err)

		assert.Contains(t, sql, "EXISTS")
		assert.Contains(t, sql, "cl.list_id = $1")
		assert.Contains(t, sql, "cl.status = $2")
		assert.Equal(t, []interface{}{"list456", "active"}, args)
	})

	t.Run("contact NOT in list", func(t *testing.T) {
		tree := &domain.TreeNode{
			Kind: "leaf",
			Leaf: &domain.TreeNodeLeaf{
				Table: "contact_lists",
				ContactList: &domain.ContactListCondition{
					Operator: "not_in",
					ListID:   "list789",
				},
			},
		}

		sql, args, err := qb.BuildSQL(tree)
		require.NoError(t, err)

		assert.Contains(t, sql, "NOT EXISTS")
		assert.Equal(t, []interface{}{"list789"}, args)
	})

	t.Run("missing list_id", func(t *testing.T) {
		tree := &domain.TreeNode{
			Kind: "leaf",
			Leaf: &domain.TreeNodeLeaf{
				Table: "contact_lists",
				ContactList: &domain.ContactListCondition{
					Operator: "in",
					ListID:   "",
				},
			},
		}

		_, _, err := qb.BuildSQL(tree)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must have 'list_id'")
	})

	t.Run("invalid operator", func(t *testing.T) {
		tree := &domain.TreeNode{
			Kind: "leaf",
			Leaf: &domain.TreeNodeLeaf{
				Table: "contact_lists",
				ContactList: &domain.ContactListCondition{
					Operator: "invalid",
					ListID:   "list123",
				},
			},
		}

		_, _, err := qb.BuildSQL(tree)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid contact_list operator")
	})

	t.Run("combined with contact filters", func(t *testing.T) {
		tree := &domain.TreeNode{
			Kind: "branch",
			Branch: &domain.TreeNodeBranch{
				Operator: "and",
				Leaves: []*domain.TreeNode{
					{
						Kind: "leaf",
						Leaf: &domain.TreeNodeLeaf{
							Table: "contacts",
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
							Table: "contact_lists",
							ContactList: &domain.ContactListCondition{
								Operator: "in",
								ListID:   "list123",
							},
						},
					},
				},
			},
		}

		sql, args, err := qb.BuildSQL(tree)
		require.NoError(t, err)

		assert.Contains(t, sql, "country = $1")
		assert.Contains(t, sql, "EXISTS")
		assert.Contains(t, sql, "cl.list_id = $2")
		assert.Equal(t, []interface{}{"US", "list123"}, args)
	})
}

func TestQueryBuilder_ContactTimeline(t *testing.T) {
	qb := NewQueryBuilder()

	t.Run("timeline event count - at least", func(t *testing.T) {
		tree := &domain.TreeNode{
			Kind: "leaf",
			Leaf: &domain.TreeNodeLeaf{
				Table: "contact_timeline",
				ContactTimeline: &domain.ContactTimelineCondition{
					Kind:          "email_opened",
					CountOperator: "at_least",
					CountValue:    5,
				},
			},
		}

		sql, args, err := qb.BuildSQL(tree)
		require.NoError(t, err)

		assert.Contains(t, sql, "SELECT COUNT(*)")
		assert.Contains(t, sql, "FROM contact_timeline ct")
		assert.Contains(t, sql, "WHERE ct.email = contacts.email")
		assert.Contains(t, sql, "ct.kind = $1")
		assert.Contains(t, sql, ">= $2")
		assert.Equal(t, []interface{}{"email_opened", 5}, args)
	})

	t.Run("timeline event count - exactly", func(t *testing.T) {
		tree := &domain.TreeNode{
			Kind: "leaf",
			Leaf: &domain.TreeNodeLeaf{
				Table: "contact_timeline",
				ContactTimeline: &domain.ContactTimelineCondition{
					Kind:          "purchase",
					CountOperator: "exactly",
					CountValue:    1,
				},
			},
		}

		sql, args, err := qb.BuildSQL(tree)
		require.NoError(t, err)

		assert.Contains(t, sql, "= $2")
		assert.Equal(t, []interface{}{"purchase", 1}, args)
	})

	t.Run("timeline event count - at most", func(t *testing.T) {
		tree := &domain.TreeNode{
			Kind: "leaf",
			Leaf: &domain.TreeNodeLeaf{
				Table: "contact_timeline",
				ContactTimeline: &domain.ContactTimelineCondition{
					Kind:          "email_bounced",
					CountOperator: "at_most",
					CountValue:    2,
				},
			},
		}

		sql, args, err := qb.BuildSQL(tree)
		require.NoError(t, err)

		assert.Contains(t, sql, "<= $2")
		assert.Equal(t, []interface{}{"email_bounced", 2}, args)
	})

	t.Run("timeline with date range timeframe", func(t *testing.T) {
		timeframeOp := "in_date_range"
		tree := &domain.TreeNode{
			Kind: "leaf",
			Leaf: &domain.TreeNodeLeaf{
				Table: "contact_timeline",
				ContactTimeline: &domain.ContactTimelineCondition{
					Kind:              "email_sent",
					CountOperator:     "at_least",
					CountValue:        3,
					TimeframeOperator: &timeframeOp,
					TimeframeValues:   []string{"2024-01-01T00:00:00Z", "2024-12-31T23:59:59Z"},
				},
			},
		}

		sql, args, err := qb.BuildSQL(tree)
		require.NoError(t, err)

		assert.Contains(t, sql, "ct.kind = $1")
		assert.Contains(t, sql, "ct.created_at BETWEEN $2 AND $3")
		assert.Contains(t, sql, ">= $4")
		assert.Equal(t, 4, len(args))
		assert.Equal(t, "email_sent", args[0])
		assert.Equal(t, 3, args[3])
	})

	t.Run("timeline with before_date timeframe", func(t *testing.T) {
		timeframeOp := "before_date"
		tree := &domain.TreeNode{
			Kind: "leaf",
			Leaf: &domain.TreeNodeLeaf{
				Table: "contact_timeline",
				ContactTimeline: &domain.ContactTimelineCondition{
					Kind:              "unsubscribe",
					CountOperator:     "at_least",
					CountValue:        1,
					TimeframeOperator: &timeframeOp,
					TimeframeValues:   []string{"2024-01-01T00:00:00Z"},
				},
			},
		}

		sql, args, err := qb.BuildSQL(tree)
		require.NoError(t, err)

		assert.Contains(t, sql, "ct.created_at < $2")
		assert.Equal(t, 3, len(args))
	})

	t.Run("timeline with after_date timeframe", func(t *testing.T) {
		timeframeOp := "after_date"
		tree := &domain.TreeNode{
			Kind: "leaf",
			Leaf: &domain.TreeNodeLeaf{
				Table: "contact_timeline",
				ContactTimeline: &domain.ContactTimelineCondition{
					Kind:              "purchase",
					CountOperator:     "at_least",
					CountValue:        1,
					TimeframeOperator: &timeframeOp,
					TimeframeValues:   []string{"2024-01-01T00:00:00Z"},
				},
			},
		}

		sql, args, err := qb.BuildSQL(tree)
		require.NoError(t, err)

		assert.Contains(t, sql, "ct.created_at > $2")
		assert.Equal(t, 3, len(args))
	})

	t.Run("timeline with in_the_last_days timeframe", func(t *testing.T) {
		timeframeOp := "in_the_last_days"
		tree := &domain.TreeNode{
			Kind: "leaf",
			Leaf: &domain.TreeNodeLeaf{
				Table: "contact_timeline",
				ContactTimeline: &domain.ContactTimelineCondition{
					Kind:              "email_clicked",
					CountOperator:     "at_least",
					CountValue:        2,
					TimeframeOperator: &timeframeOp,
					TimeframeValues:   []string{"30"},
				},
			},
		}

		sql, args, err := qb.BuildSQL(tree)
		require.NoError(t, err)

		assert.Contains(t, sql, "ct.created_at > NOW() - INTERVAL '30 days'")
		assert.Equal(t, 2, len(args)) // kind + count (days not parameterized)
	})

	t.Run("timeline with metadata filters", func(t *testing.T) {
		tree := &domain.TreeNode{
			Kind: "leaf",
			Leaf: &domain.TreeNodeLeaf{
				Table: "contact_timeline",
				ContactTimeline: &domain.ContactTimelineCondition{
					Kind:          "purchase",
					CountOperator: "at_least",
					CountValue:    1,
					Filters: []*domain.DimensionFilter{
						{
							FieldName:    "product_id",
							FieldType:    "string",
							Operator:     "equals",
							StringValues: []string{"prod_123"},
						},
					},
				},
			},
		}

		sql, args, err := qb.BuildSQL(tree)
		require.NoError(t, err)

		assert.Contains(t, sql, "ct.metadata->>'product_id' = $2")
		assert.Equal(t, []interface{}{"purchase", "prod_123", 1}, args)
	})

	t.Run("timeline with number metadata filter", func(t *testing.T) {
		tree := &domain.TreeNode{
			Kind: "leaf",
			Leaf: &domain.TreeNodeLeaf{
				Table: "contact_timeline",
				ContactTimeline: &domain.ContactTimelineCondition{
					Kind:          "purchase",
					CountOperator: "at_least",
					CountValue:    1,
					Filters: []*domain.DimensionFilter{
						{
							FieldName:    "amount",
							FieldType:    "number",
							Operator:     "gte",
							NumberValues: []float64{100.0},
						},
					},
				},
			},
		}

		sql, args, err := qb.BuildSQL(tree)
		require.NoError(t, err)

		// Should cast JSONB field to numeric for comparison
		assert.Contains(t, sql, "(ct.metadata->>'amount')::numeric >= $2")
		assert.Equal(t, []interface{}{"purchase", 100.0, 1}, args)
	})

	t.Run("missing kind", func(t *testing.T) {
		tree := &domain.TreeNode{
			Kind: "leaf",
			Leaf: &domain.TreeNodeLeaf{
				Table: "contact_timeline",
				ContactTimeline: &domain.ContactTimelineCondition{
					Kind:          "",
					CountOperator: "at_least",
					CountValue:    1,
				},
			},
		}

		_, _, err := qb.BuildSQL(tree)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must have 'kind'")
	})

	t.Run("missing count_operator", func(t *testing.T) {
		tree := &domain.TreeNode{
			Kind: "leaf",
			Leaf: &domain.TreeNodeLeaf{
				Table: "contact_timeline",
				ContactTimeline: &domain.ContactTimelineCondition{
					Kind:          "email_sent",
					CountOperator: "",
					CountValue:    1,
				},
			},
		}

		_, _, err := qb.BuildSQL(tree)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid count_operator")
	})

	t.Run("invalid count_operator", func(t *testing.T) {
		tree := &domain.TreeNode{
			Kind: "leaf",
			Leaf: &domain.TreeNodeLeaf{
				Table: "contact_timeline",
				ContactTimeline: &domain.ContactTimelineCondition{
					Kind:          "email_sent",
					CountOperator: "invalid",
					CountValue:    1,
				},
			},
		}

		_, _, err := qb.BuildSQL(tree)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid count_operator")
	})

	t.Run("combined with contact and list filters", func(t *testing.T) {
		tree := &domain.TreeNode{
			Kind: "branch",
			Branch: &domain.TreeNodeBranch{
				Operator: "and",
				Leaves: []*domain.TreeNode{
					{
						Kind: "leaf",
						Leaf: &domain.TreeNodeLeaf{
							Table: "contacts",
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
							Table: "contact_lists",
							ContactList: &domain.ContactListCondition{
								Operator: "in",
								ListID:   "list123",
							},
						},
					},
					{
						Kind: "leaf",
						Leaf: &domain.TreeNodeLeaf{
							Table: "contact_timeline",
							ContactTimeline: &domain.ContactTimelineCondition{
								Kind:          "purchase",
								CountOperator: "at_least",
								CountValue:    1,
							},
						},
					},
				},
			},
		}

		sql, args, err := qb.BuildSQL(tree)
		require.NoError(t, err)

		// Should have all three conditions ANDed together
		assert.Contains(t, sql, "country = $1")
		assert.Contains(t, sql, "EXISTS")
		assert.Contains(t, sql, "cl.list_id = $2")
		assert.Contains(t, sql, "SELECT COUNT(*)")
		assert.Contains(t, sql, "ct.kind = $3")
		assert.Equal(t, []interface{}{"US", "list123", "purchase", 1}, args)
	})
}
