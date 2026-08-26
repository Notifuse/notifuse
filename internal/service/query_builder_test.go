package service

import (
	"database/sql"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/Notifuse/notifuse/internal/domain"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQueryBuilder_BuildSQL_SimpleConditions(t *testing.T) {
	qb := NewQueryBuilder()

	t.Run("single string equals condition", func(t *testing.T) {
		tree := &domain.TreeNode{
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
				Source: "contacts",
				Contact: &domain.ContactCondition{
					Filters: []*domain.DimensionFilter{
						{
							FieldName:    "custom_number_1",
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
		assert.Equal(t, "SELECT email FROM contacts WHERE (custom_number_1 >= $1)", sql)
		assert.Equal(t, []interface{}{5.0}, args)
	})

	t.Run("is_set condition (no value needed)", func(t *testing.T) {
		tree := &domain.TreeNode{
			Kind: "leaf",
			Leaf: &domain.TreeNodeLeaf{
				Source: "contacts",
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
				Source: "contacts",
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
				Source: "contacts",
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
				Source: "contacts",
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
				Source: "contacts",
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
				Source: "contacts",
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
				Source: "contacts",
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
				Source: "contacts",
				Contact: &domain.ContactCondition{
					Filters: []*domain.DimensionFilter{
						{
							FieldName:    "country",
							FieldType:    "string",
							Operator:     "equals",
							StringValues: []string{"US"},
						},
						{
							FieldName:    "custom_number_1",
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
		assert.Contains(t, sql, "custom_number_1 >= $2")
		assert.Contains(t, sql, " AND ")
		assert.Equal(t, []interface{}{"US", 5.0}, args)
	})

	t.Run("contains with multiple values combined with other filter", func(t *testing.T) {
		tree := &domain.TreeNode{
			Kind: "leaf",
			Leaf: &domain.TreeNodeLeaf{
				Source: "contacts",
				Contact: &domain.ContactCondition{
					Filters: []*domain.DimensionFilter{
						{
							FieldName:    "country",
							FieldType:    "string",
							Operator:     "contains",
							StringValues: []string{"United", "States"},
						},
						{
							FieldName:    "custom_number_1",
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
		assert.Contains(t, sql, "custom_number_1 >= $3")
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
							Source: "contacts",
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
							Source: "contacts",
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
							Source: "contacts",
							Contact: &domain.ContactCondition{
								Filters: []*domain.DimensionFilter{
									{
										FieldName:    "custom_number_1",
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
		assert.Contains(t, sql, "custom_number_1 >= $2")
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
							Source: "contacts",
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
										Source: "contacts",
										Contact: &domain.ContactCondition{
											Filters: []*domain.DimensionFilter{
												{
													FieldName:    "custom_number_1",
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
										Source: "contacts",
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
										Source: "contacts",
										Contact: &domain.ContactCondition{
											Filters: []*domain.DimensionFilter{
												{
													FieldName:    "custom_number_1",
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
		assert.Contains(t, sql, "custom_number_1 >= $2")
		assert.Contains(t, sql, "country = $3")
		assert.Contains(t, sql, "custom_number_1 >= $4")

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
					Source: "contacts",
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
				Source: "contacts",
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
				Source: "contacts",
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

	t.Run("unsupported source", func(t *testing.T) {
		tree := &domain.TreeNode{
			Kind: "leaf",
			Leaf: &domain.TreeNodeLeaf{
				Source: "unsupported_source",
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
		assert.Contains(t, err.Error(), "invalid source")
	})

	t.Run("contains with no values", func(t *testing.T) {
		tree := &domain.TreeNode{
			Kind: "leaf",
			Leaf: &domain.TreeNodeLeaf{
				Source: "contacts",
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
				Source: "contacts",
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
				Source: "contacts",
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
							FieldName:    "custom_number_1",
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
				Source: "contact_lists",
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
				Source: "contact_lists",
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
				Source: "contact_lists",
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
				Source: "contact_lists",
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
				Source: "contact_lists",
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
				Source: "contact_timeline",
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
				Source: "contact_timeline",
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
				Source: "contact_timeline",
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

	t.Run("timeline event count - exactly 0 (never)", func(t *testing.T) {
		tree := &domain.TreeNode{
			Kind: "leaf",
			Leaf: &domain.TreeNodeLeaf{
				Source: "contact_timeline",
				ContactTimeline: &domain.ContactTimelineCondition{
					Kind:          "email.opened",
					CountOperator: "exactly",
					CountValue:    0,
				},
			},
		}

		sql, args, err := qb.BuildSQL(tree)
		require.NoError(t, err)

		assert.Contains(t, sql, "SELECT COUNT(*)")
		assert.Contains(t, sql, "ct.kind = $1")
		assert.Contains(t, sql, "= $2")
		assert.Equal(t, []interface{}{"email.opened", 0}, args)
	})

	t.Run("timeline event count - at most 0 (never)", func(t *testing.T) {
		tree := &domain.TreeNode{
			Kind: "leaf",
			Leaf: &domain.TreeNodeLeaf{
				Source: "contact_timeline",
				ContactTimeline: &domain.ContactTimelineCondition{
					Kind:          "email.clicked",
					CountOperator: "at_most",
					CountValue:    0,
				},
			},
		}

		sql, args, err := qb.BuildSQL(tree)
		require.NoError(t, err)

		assert.Contains(t, sql, "<= $2")
		assert.Equal(t, []interface{}{"email.clicked", 0}, args)
	})

	t.Run("timeline event count - at least 0 (always true)", func(t *testing.T) {
		tree := &domain.TreeNode{
			Kind: "leaf",
			Leaf: &domain.TreeNodeLeaf{
				Source: "contact_timeline",
				ContactTimeline: &domain.ContactTimelineCondition{
					Kind:          "email_opened",
					CountOperator: "at_least",
					CountValue:    0,
				},
			},
		}

		sql, args, err := qb.BuildSQL(tree)
		require.NoError(t, err)

		assert.Contains(t, sql, ">= $2")
		assert.Equal(t, []interface{}{"email_opened", 0}, args)
	})

	t.Run("timeline with date range timeframe", func(t *testing.T) {
		timeframeOp := "in_date_range"
		tree := &domain.TreeNode{
			Kind: "leaf",
			Leaf: &domain.TreeNodeLeaf{
				Source: "contact_timeline",
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
				Source: "contact_timeline",
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
				Source: "contact_timeline",
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
				Source: "contact_timeline",
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
				Source: "contact_timeline",
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

		// The change key is bound as an argument, never spliced into the SQL text
		assert.Contains(t, sql, "ct.changes->$2->>'new' = $3")
		assert.Equal(t, []interface{}{"purchase", "product_id", "prod_123", 1}, args)
	})

	t.Run("timeline with number metadata filter", func(t *testing.T) {
		tree := &domain.TreeNode{
			Kind: "leaf",
			Leaf: &domain.TreeNodeLeaf{
				Source: "contact_timeline",
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

		// Cast to numeric, but only for values that look numeric. `changes` is no
		// longer trigger-written from typed columns alone — the web analytics
		// projection stores visitor-supplied text under keys like `path` — so an
		// unguarded cast would abort the whole statement on one odd row, and
		// whether the kind predicate excludes that row first is plan-dependent.
		assert.Contains(t, sql, "(ct.changes->$2->>'new')::numeric END >= $3")
		assert.Contains(t, sql, "CASE WHEN ct.changes->$2->>'new' ~ ",
			"a non-numeric value must yield NULL, not abort the segment")
		assert.Equal(t, []interface{}{"purchase", "amount", 100.0, 1}, args)
	})

	t.Run("missing kind", func(t *testing.T) {
		tree := &domain.TreeNode{
			Kind: "leaf",
			Leaf: &domain.TreeNodeLeaf{
				Source: "contact_timeline",
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
				Source: "contact_timeline",
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
				Source: "contact_timeline",
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
								ListID:   "list123",
							},
						},
					},
					{
						Kind: "leaf",
						Leaf: &domain.TreeNodeLeaf{
							Source: "contact_timeline",
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

	t.Run("timeline with template_id filter", func(t *testing.T) {
		templateID := "welcome-email"
		tree := &domain.TreeNode{
			Kind: "leaf",
			Leaf: &domain.TreeNodeLeaf{
				Source: "contact_timeline",
				ContactTimeline: &domain.ContactTimelineCondition{
					Kind:          "email.opened",
					CountOperator: "at_least",
					CountValue:    1,
					TemplateID:    &templateID,
				},
			},
		}

		sql, args, err := qb.BuildSQL(tree)
		require.NoError(t, err)

		assert.Contains(t, sql, "ct.kind = $1")
		assert.Contains(t, sql, "ct.entity_id IN (SELECT id FROM message_history WHERE template_id = $2)")
		assert.Contains(t, sql, ">= $3")
		assert.Equal(t, []interface{}{"email.opened", "welcome-email", 1}, args)
	})

	t.Run("timeline without template_id - unchanged SQL", func(t *testing.T) {
		tree := &domain.TreeNode{
			Kind: "leaf",
			Leaf: &domain.TreeNodeLeaf{
				Source: "contact_timeline",
				ContactTimeline: &domain.ContactTimelineCondition{
					Kind:          "email.opened",
					CountOperator: "at_least",
					CountValue:    1,
				},
			},
		}

		sql, args, err := qb.BuildSQL(tree)
		require.NoError(t, err)

		assert.NotContains(t, sql, "message_history")
		assert.Equal(t, []interface{}{"email.opened", 1}, args)
	})

	t.Run("timeline with broadcast_id filter", func(t *testing.T) {
		broadcastID := "summer-sale"
		tree := &domain.TreeNode{
			Kind: "leaf",
			Leaf: &domain.TreeNodeLeaf{
				Source: "contact_timeline",
				ContactTimeline: &domain.ContactTimelineCondition{
					Kind:          "email.clicked",
					CountOperator: "at_least",
					CountValue:    1,
					BroadcastID:   &broadcastID,
				},
			},
		}

		sql, args, err := qb.BuildSQL(tree)
		require.NoError(t, err)

		assert.Contains(t, sql, "ct.entity_id IN (SELECT id FROM message_history WHERE broadcast_id = $2)")
		assert.Equal(t, []interface{}{"email.clicked", "summer-sale", 1}, args)
	})

	t.Run("timeline with link_url contains filter", func(t *testing.T) {
		linkURL := "/pricing"
		tree := &domain.TreeNode{
			Kind: "leaf",
			Leaf: &domain.TreeNodeLeaf{
				Source: "contact_timeline",
				ContactTimeline: &domain.ContactTimelineCondition{
					Kind:          "email.clicked",
					CountOperator: "at_least",
					CountValue:    1,
					LinkURL:       &linkURL,
				},
			},
		}

		sql, args, err := qb.BuildSQL(tree)
		require.NoError(t, err)

		// Literal case-insensitive substring match against clicked_links keys (strpos, not
		// ILIKE, so '%'/'_' in a URL are matched literally); a malformed clicked_links is
		// coerced to an empty object so jsonb_object_keys never errors.
		assert.Contains(t, sql, "CASE WHEN jsonb_typeof(clicked_links) = 'object'")
		assert.Contains(t, sql, "strpos(lower(k), lower($2)) > 0")
		assert.Equal(t, []interface{}{"email.clicked", "/pricing", 1}, args)
	})

	t.Run("timeline with broadcast_id and link_url combined", func(t *testing.T) {
		broadcastID := "summer-sale"
		linkURL := "/pricing"
		tree := &domain.TreeNode{
			Kind: "leaf",
			Leaf: &domain.TreeNodeLeaf{
				Source: "contact_timeline",
				ContactTimeline: &domain.ContactTimelineCondition{
					Kind:          "email.clicked",
					CountOperator: "at_least",
					CountValue:    1,
					BroadcastID:   &broadcastID,
					LinkURL:       &linkURL,
				},
			},
		}

		sql, args, err := qb.BuildSQL(tree)
		require.NoError(t, err)

		// Both scopes ANDed into a single message_history subquery.
		assert.Contains(t, sql, "broadcast_id = $2 AND EXISTS (SELECT 1 FROM jsonb_object_keys(CASE WHEN jsonb_typeof(clicked_links)")
		assert.Equal(t, []interface{}{"email.clicked", "summer-sale", "/pricing", 1}, args)
	})

	t.Run("timeline with template_id and timeframe", func(t *testing.T) {
		templateID := "promo-email"
		timeframeOp := "in_the_last_days"
		tree := &domain.TreeNode{
			Kind: "leaf",
			Leaf: &domain.TreeNodeLeaf{
				Source: "contact_timeline",
				ContactTimeline: &domain.ContactTimelineCondition{
					Kind:              "email.clicked",
					CountOperator:     "at_least",
					CountValue:        2,
					TemplateID:        &templateID,
					TimeframeOperator: &timeframeOp,
					TimeframeValues:   []string{"30"},
				},
			},
		}

		sql, args, err := qb.BuildSQL(tree)
		require.NoError(t, err)

		assert.Contains(t, sql, "ct.kind = $1")
		assert.Contains(t, sql, "ct.entity_id IN (SELECT id FROM message_history WHERE template_id = $2)")
		assert.Contains(t, sql, "ct.created_at > NOW() - INTERVAL '30 days'")
		assert.Contains(t, sql, ">= $3")
		assert.Equal(t, []interface{}{"email.clicked", "promo-email", 2}, args)
	})
}

func TestQueryBuilder_BuildSQL_JSONFiltering(t *testing.T) {
	qb := NewQueryBuilder()

	t.Run("simple JSON path - string equals", func(t *testing.T) {
		tree := &domain.TreeNode{
			Kind: "leaf",
			Leaf: &domain.TreeNodeLeaf{
				Source: "contacts",
				Contact: &domain.ContactCondition{
					Filters: []*domain.DimensionFilter{
						{
							FieldName:    "custom_json_1",
							FieldType:    "json",
							Operator:     "equals",
							JSONPath:     []string{"name"},
							StringValues: []string{"John"},
						},
					},
				},
			},
		}

		sql, args, err := qb.BuildSQL(tree)
		require.NoError(t, err)
		assert.Contains(t, sql, "custom_json_1['name']::text = $1")
		assert.Equal(t, []interface{}{"John"}, args)
	})

	t.Run("nested JSON path - multiple levels", func(t *testing.T) {
		tree := &domain.TreeNode{
			Kind: "leaf",
			Leaf: &domain.TreeNodeLeaf{
				Source: "contacts",
				Contact: &domain.ContactCondition{
					Filters: []*domain.DimensionFilter{
						{
							FieldName:    "custom_json_1",
							FieldType:    "json",
							Operator:     "equals",
							JSONPath:     []string{"user", "profile", "country"},
							StringValues: []string{"US"},
						},
					},
				},
			},
		}

		sql, args, err := qb.BuildSQL(tree)
		require.NoError(t, err)
		assert.Contains(t, sql, "custom_json_1['user']['profile']['country']::text = $1")
		assert.Equal(t, []interface{}{"US"}, args)
	})

	t.Run("array element access by index", func(t *testing.T) {
		tree := &domain.TreeNode{
			Kind: "leaf",
			Leaf: &domain.TreeNodeLeaf{
				Source: "contacts",
				Contact: &domain.ContactCondition{
					Filters: []*domain.DimensionFilter{
						{
							FieldName:    "custom_json_2",
							FieldType:    "json",
							Operator:     "equals",
							JSONPath:     []string{"items", "0", "name"},
							StringValues: []string{"Product A"},
						},
					},
				},
			},
		}

		sql, args, err := qb.BuildSQL(tree)
		require.NoError(t, err)
		assert.Contains(t, sql, "custom_json_2['items'][0]['name']::text = $1")
		assert.Equal(t, []interface{}{"Product A"}, args)
	})

	t.Run("JSON number field - greater than", func(t *testing.T) {
		tree := &domain.TreeNode{
			Kind: "leaf",
			Leaf: &domain.TreeNodeLeaf{
				Source: "contacts",
				Contact: &domain.ContactCondition{
					Filters: []*domain.DimensionFilter{
						{
							FieldName:    "custom_json_1",
							FieldType:    "number",
							Operator:     "gt",
							JSONPath:     []string{"user", "age"},
							NumberValues: []float64{25},
						},
					},
				},
			},
		}

		sql, args, err := qb.BuildSQL(tree)
		require.NoError(t, err)
		assert.Contains(t, sql, "(custom_json_1['user']['age']::text)::numeric > $1")
		assert.Equal(t, []interface{}{25.0}, args)
	})

	t.Run("JSON time field - before date", func(t *testing.T) {
		tree := &domain.TreeNode{
			Kind: "leaf",
			Leaf: &domain.TreeNodeLeaf{
				Source: "contacts",
				Contact: &domain.ContactCondition{
					Filters: []*domain.DimensionFilter{
						{
							FieldName:    "custom_json_3",
							FieldType:    "time",
							Operator:     "lt",
							JSONPath:     []string{"last_login"},
							StringValues: []string{"2024-01-01T00:00:00Z"},
						},
					},
				},
			},
		}

		sql, args, err := qb.BuildSQL(tree)
		require.NoError(t, err)
		assert.Contains(t, sql, "(custom_json_3['last_login']::text)::timestamptz < $1")
		assert.Len(t, args, 1)
	})

	t.Run("JSON contains operator", func(t *testing.T) {
		tree := &domain.TreeNode{
			Kind: "leaf",
			Leaf: &domain.TreeNodeLeaf{
				Source: "contacts",
				Contact: &domain.ContactCondition{
					Filters: []*domain.DimensionFilter{
						{
							FieldName:    "custom_json_1",
							FieldType:    "json",
							Operator:     "contains",
							JSONPath:     []string{"description"},
							StringValues: []string{"premium"},
						},
					},
				},
			},
		}

		sql, args, err := qb.BuildSQL(tree)
		require.NoError(t, err)
		assert.Contains(t, sql, "custom_json_1['description']::text ILIKE $1")
		assert.Equal(t, []interface{}{"%premium%"}, args)
	})

	t.Run("JSON field existence check - is_set", func(t *testing.T) {
		tree := &domain.TreeNode{
			Kind: "leaf",
			Leaf: &domain.TreeNodeLeaf{
				Source: "contacts",
				Contact: &domain.ContactCondition{
					Filters: []*domain.DimensionFilter{
						{
							FieldName: "custom_json_1",
							FieldType: "json",
							Operator:  "is_set",
							JSONPath:  []string{"user"},
						},
					},
				},
			},
		}

		sql, args, err := qb.BuildSQL(tree)
		require.NoError(t, err)
		// The typeof test is not decoration: jsonb's ? matches a top-level scalar string too, so
		// `custom_json_1 ? 'user'` alone is satisfied by the bare string "user".
		assert.Contains(t, sql, "(jsonb_typeof(custom_json_1) = 'object' AND custom_json_1 ? $1)")
		assert.Equal(t, []interface{}{"user"}, args)
	})

	t.Run("JSON field existence check - is_not_set", func(t *testing.T) {
		tree := &domain.TreeNode{
			Kind: "leaf",
			Leaf: &domain.TreeNodeLeaf{
				Source: "contacts",
				Contact: &domain.ContactCondition{
					Filters: []*domain.DimensionFilter{
						{
							FieldName: "custom_json_2",
							FieldType: "json",
							Operator:  "is_not_set",
							JSONPath:  []string{"premium"},
						},
					},
				},
			},
		}

		sql, args, err := qb.BuildSQL(tree)
		require.NoError(t, err)
		assert.Contains(t, sql, "NOT (jsonb_typeof(custom_json_2) = 'object' AND custom_json_2 ? $1)")
		assert.Equal(t, []interface{}{"premium"}, args)
	})

	t.Run("JSON key with special characters", func(t *testing.T) {
		tree := &domain.TreeNode{
			Kind: "leaf",
			Leaf: &domain.TreeNodeLeaf{
				Source: "contacts",
				Contact: &domain.ContactCondition{
					Filters: []*domain.DimensionFilter{
						{
							FieldName:    "custom_json_1",
							FieldType:    "json",
							Operator:     "equals",
							JSONPath:     []string{"user's name"},
							StringValues: []string{"John"},
						},
					},
				},
			},
		}

		sql, args, err := qb.BuildSQL(tree)
		require.NoError(t, err)
		// Should escape single quotes
		assert.Contains(t, sql, "custom_json_1['user''s name']::text = $1")
		assert.Equal(t, []interface{}{"John"}, args)
	})

	t.Run("JSON in_array operator", func(t *testing.T) {
		tree := &domain.TreeNode{
			Kind: "leaf",
			Leaf: &domain.TreeNodeLeaf{
				Source: "contacts",
				Contact: &domain.ContactCondition{
					Filters: []*domain.DimensionFilter{
						{
							FieldName:    "custom_json_1",
							FieldType:    "json",
							Operator:     "in_array",
							JSONPath:     []string{"tags"},
							StringValues: []string{"premium"},
						},
					},
				},
			},
		}

		sql, args, err := qb.BuildSQL(tree)
		require.NoError(t, err)
		// Objects stay eligible here — in_array is the only way to ask about a nested key, since
		// is_set reads JSONPath[0] alone — but a scalar never is.
		assert.Contains(t, sql, "(jsonb_typeof(custom_json_1['tags']) IN ('object', 'array') AND custom_json_1['tags'] ? $1)")
		assert.Equal(t, []interface{}{"premium"}, args)
	})

	t.Run("multiple JSON filters combined with AND", func(t *testing.T) {
		tree := &domain.TreeNode{
			Kind: "leaf",
			Leaf: &domain.TreeNodeLeaf{
				Source: "contacts",
				Contact: &domain.ContactCondition{
					Filters: []*domain.DimensionFilter{
						{
							FieldName:    "custom_json_1",
							FieldType:    "json",
							Operator:     "equals",
							JSONPath:     []string{"type"},
							StringValues: []string{"premium"},
						},
						{
							FieldName:    "custom_json_1",
							FieldType:    "number",
							Operator:     "gte",
							JSONPath:     []string{"score"},
							NumberValues: []float64{100},
						},
					},
				},
			},
		}

		sql, args, err := qb.BuildSQL(tree)
		require.NoError(t, err)
		assert.Contains(t, sql, "custom_json_1['type']::text = $1")
		assert.Contains(t, sql, "(custom_json_1['score']::text)::numeric >= $2")
		assert.Contains(t, sql, " AND ")
		assert.Equal(t, []interface{}{"premium", 100.0}, args)
	})

	t.Run("JSON filter combined with regular field", func(t *testing.T) {
		tree := &domain.TreeNode{
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
						{
							FieldName:    "custom_json_1",
							FieldType:    "json",
							Operator:     "equals",
							JSONPath:     []string{"subscription", "tier"},
							StringValues: []string{"gold"},
						},
					},
				},
			},
		}

		sql, args, err := qb.BuildSQL(tree)
		require.NoError(t, err)
		assert.Contains(t, sql, "country = $1")
		assert.Contains(t, sql, "custom_json_1['subscription']['tier']::text = $2")
		assert.Contains(t, sql, " AND ")
		assert.Equal(t, []interface{}{"US", "gold"}, args)
	})

	t.Run("mixed path with objects and arrays", func(t *testing.T) {
		tree := &domain.TreeNode{
			Kind: "leaf",
			Leaf: &domain.TreeNodeLeaf{
				Source: "contacts",
				Contact: &domain.ContactCondition{
					Filters: []*domain.DimensionFilter{
						{
							FieldName:    "custom_json_5",
							FieldType:    "json",
							Operator:     "equals",
							JSONPath:     []string{"users", "0", "profile", "tags", "1"},
							StringValues: []string{"verified"},
						},
					},
				},
			},
		}

		sql, args, err := qb.BuildSQL(tree)
		require.NoError(t, err)
		assert.Contains(t, sql, "custom_json_5['users'][0]['profile']['tags'][1]::text = $1")
		assert.Equal(t, []interface{}{"verified"}, args)
	})
}

// ============================================================================
// BuildTriggerCondition Tests
// ============================================================================

func TestQueryBuilder_BuildTriggerCondition(t *testing.T) {
	qb := NewQueryBuilder()

	t.Run("nil tree returns empty", func(t *testing.T) {
		sql, args, err := qb.BuildTriggerCondition(nil, "NEW.email")
		require.NoError(t, err)
		assert.Equal(t, "", sql)
		assert.Nil(t, args)
	})

	t.Run("simple contact condition", func(t *testing.T) {
		tree := &domain.TreeNode{
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
		}

		sql, args, err := qb.BuildTriggerCondition(tree, "NEW.email")
		require.NoError(t, err)

		// Should wrap in EXISTS with NEW.email reference
		assert.Contains(t, sql, "EXISTS")
		assert.Contains(t, sql, "SELECT 1 FROM contacts")
		assert.Contains(t, sql, "email = NEW.email")
		assert.Contains(t, sql, "country = $1")
		assert.Equal(t, []interface{}{"US"}, args)
	})

	t.Run("contact list membership - in", func(t *testing.T) {
		tree := &domain.TreeNode{
			Kind: "leaf",
			Leaf: &domain.TreeNodeLeaf{
				Source: "contact_lists",
				ContactList: &domain.ContactListCondition{
					Operator: "in",
					ListID:   "vip_list",
				},
			},
		}

		sql, args, err := qb.BuildTriggerCondition(tree, "NEW.email")
		require.NoError(t, err)

		// Should use NEW.email instead of contacts.email
		assert.Contains(t, sql, "EXISTS")
		assert.Contains(t, sql, "FROM contact_lists cl")
		assert.Contains(t, sql, "cl.email = NEW.email")
		assert.Contains(t, sql, "cl.list_id = $1")
		assert.Equal(t, []interface{}{"vip_list"}, args)
	})

	t.Run("contact list membership - not_in", func(t *testing.T) {
		tree := &domain.TreeNode{
			Kind: "leaf",
			Leaf: &domain.TreeNodeLeaf{
				Source: "contact_lists",
				ContactList: &domain.ContactListCondition{
					Operator: "not_in",
					ListID:   "blocklist",
				},
			},
		}

		sql, args, err := qb.BuildTriggerCondition(tree, "NEW.email")
		require.NoError(t, err)

		assert.Contains(t, sql, "NOT EXISTS")
		assert.Contains(t, sql, "cl.email = NEW.email")
		assert.Equal(t, []interface{}{"blocklist"}, args)
	})

	t.Run("timeline count condition", func(t *testing.T) {
		tree := &domain.TreeNode{
			Kind: "leaf",
			Leaf: &domain.TreeNodeLeaf{
				Source: "contact_timeline",
				ContactTimeline: &domain.ContactTimelineCondition{
					Kind:          "email.updated",
					CountOperator: "at_least",
					CountValue:    5,
				},
			},
		}

		sql, args, err := qb.BuildTriggerCondition(tree, "NEW.email")
		require.NoError(t, err)

		// Should use NEW.email instead of contacts.email in subquery
		assert.Contains(t, sql, "SELECT COUNT(*)")
		assert.Contains(t, sql, "ct.email = NEW.email")
		assert.Contains(t, sql, "ct.kind = $1")
		assert.Contains(t, sql, ">= $2")
		assert.Equal(t, []interface{}{"email.updated", 5}, args)
	})

	t.Run("AND branch with multiple conditions", func(t *testing.T) {
		tree := &domain.TreeNode{
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
		}

		sql, args, err := qb.BuildTriggerCondition(tree, "NEW.email")
		require.NoError(t, err)

		// Should have both conditions with NEW.email
		assert.Contains(t, sql, "country = $1")
		assert.Contains(t, sql, "cl.email = NEW.email")
		assert.Contains(t, sql, "cl.list_id = $2")
		assert.Contains(t, sql, " AND ")
		assert.Equal(t, []interface{}{"US", "premium"}, args)
	})

	t.Run("OR branch with multiple conditions", func(t *testing.T) {
		tree := &domain.TreeNode{
			Kind: "branch",
			Branch: &domain.TreeNodeBranch{
				Operator: "or",
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
							Source: "contacts",
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

		sql, args, err := qb.BuildTriggerCondition(tree, "NEW.email")
		require.NoError(t, err)

		assert.Contains(t, sql, "country = $1")
		assert.Contains(t, sql, "country = $2")
		assert.Contains(t, sql, " OR ")
		assert.Equal(t, []interface{}{"US", "CA"}, args)
	})

	t.Run("different email references", func(t *testing.T) {
		tree := &domain.TreeNode{
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
		}

		// Test with different email reference
		sql, _, err := qb.BuildTriggerCondition(tree, "OLD.email")
		require.NoError(t, err)
		assert.Contains(t, sql, "email = OLD.email")

		// Test with table.column reference
		sql2, _, err := qb.BuildTriggerCondition(tree, "ct.email")
		require.NoError(t, err)
		assert.Contains(t, sql2, "email = ct.email")
	})

	t.Run("invalid tree structure returns error", func(t *testing.T) {
		tree := &domain.TreeNode{
			Kind: "leaf",
			// Missing Leaf field
		}

		_, _, err := qb.BuildTriggerCondition(tree, "NEW.email")
		require.Error(t, err)
	})

	t.Run("multiple filters in contact condition", func(t *testing.T) {
		tree := &domain.TreeNode{
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
						{
							FieldName:    "custom_number_1",
							FieldType:    "number",
							Operator:     "gte",
							NumberValues: []float64{100},
						},
					},
				},
			},
		}

		sql, args, err := qb.BuildTriggerCondition(tree, "NEW.email")
		require.NoError(t, err)

		assert.Contains(t, sql, "country = $1")
		assert.Contains(t, sql, "custom_number_1 >= $2")
		assert.Contains(t, sql, " AND ")
		assert.Equal(t, []interface{}{"US", 100.0}, args)
	})

	t.Run("timeline with template_id filter using email ref", func(t *testing.T) {
		templateID := "welcome-email"
		tree := &domain.TreeNode{
			Kind: "leaf",
			Leaf: &domain.TreeNodeLeaf{
				Source: "contact_timeline",
				ContactTimeline: &domain.ContactTimelineCondition{
					Kind:          "email.opened",
					CountOperator: "at_least",
					CountValue:    1,
					TemplateID:    &templateID,
				},
			},
		}

		sql, args, err := qb.BuildTriggerCondition(tree, "NEW.email")
		require.NoError(t, err)

		assert.Contains(t, sql, "ct.email = NEW.email")
		assert.Contains(t, sql, "ct.entity_id IN (SELECT id FROM message_history WHERE template_id = $2)")
		assert.Equal(t, []interface{}{"email.opened", "welcome-email", 1}, args)
	})

	t.Run("timeline with broadcast_id and link_url using email ref", func(t *testing.T) {
		broadcastID := "summer-sale"
		linkURL := "/pricing"
		tree := &domain.TreeNode{
			Kind: "leaf",
			Leaf: &domain.TreeNodeLeaf{
				Source: "contact_timeline",
				ContactTimeline: &domain.ContactTimelineCondition{
					Kind:          "email.clicked",
					CountOperator: "at_least",
					CountValue:    1,
					BroadcastID:   &broadcastID,
					LinkURL:       &linkURL,
				},
			},
		}

		sql, args, err := qb.BuildTriggerCondition(tree, "NEW.email")
		require.NoError(t, err)

		// The automation filter/branch path (WithEmailRef) applies the same scoping.
		assert.Contains(t, sql, "ct.email = NEW.email")
		assert.Contains(t, sql, "broadcast_id = $2 AND EXISTS (SELECT 1 FROM jsonb_object_keys(CASE WHEN jsonb_typeof(clicked_links)")
		assert.Equal(t, []interface{}{"email.clicked", "summer-sale", "/pricing", 1}, args)
	})
}

func TestQueryBuilder_NotInTheLastDays(t *testing.T) {
	qb := NewQueryBuilder()

	contactTree := func(filter *domain.DimensionFilter) *domain.TreeNode {
		return &domain.TreeNode{
			Kind: "leaf",
			Leaf: &domain.TreeNodeLeaf{
				Source:  "contacts",
				Contact: &domain.ContactCondition{Filters: []*domain.DimensionFilter{filter}},
			},
		}
	}

	t.Run("includes rows whose date was never set", func(t *testing.T) {
		sql, args, err := qb.BuildSQL(contactTree(&domain.DimensionFilter{
			FieldName:    "custom_datetime_1",
			FieldType:    "time",
			Operator:     "not_in_the_last_days",
			StringValues: []string{"30"},
		}))
		require.NoError(t, err)

		// A bare NOT (col > ...) would evaluate to NULL for unset dates and drop those contacts,
		// who are exactly the audience for "has not converted in the last 30 days".
		assert.Contains(t, sql, "(custom_datetime_1 IS NULL OR custom_datetime_1 <= NOW() - INTERVAL '30 days')")
		assert.Empty(t, args)
	})

	t.Run("is the complement of in_the_last_days", func(t *testing.T) {
		positive, _, err := qb.BuildSQL(contactTree(&domain.DimensionFilter{
			FieldName:    "custom_datetime_2",
			FieldType:    "time",
			Operator:     "in_the_last_days",
			StringValues: []string{"7"},
		}))
		require.NoError(t, err)
		assert.Contains(t, positive, "custom_datetime_2 > NOW() - INTERVAL '7 days'")
		assert.NotContains(t, positive, "IS NULL")
	})

	t.Run("only the leading integer reaches the interval", func(t *testing.T) {
		// The day count is the one value interpolated into the SQL text rather than bound, so it
		// must never carry anything but an int. Sscanf stops at the first non-digit, which keeps
		// a crafted suffix out of the query entirely.
		sql, _, err := qb.BuildSQL(contactTree(&domain.DimensionFilter{
			FieldName:    "custom_datetime_1",
			FieldType:    "time",
			Operator:     "not_in_the_last_days",
			StringValues: []string{"30 days'; DROP TABLE contacts; --"},
		}))
		require.NoError(t, err)

		assert.Contains(t, sql, "INTERVAL '30 days'")
		assert.NotContains(t, sql, "DROP TABLE")
	})

	t.Run("rejects a day count with no digits", func(t *testing.T) {
		_, _, err := qb.BuildSQL(contactTree(&domain.DimensionFilter{
			FieldName:    "custom_datetime_1",
			FieldType:    "time",
			Operator:     "not_in_the_last_days",
			StringValues: []string{"abc"},
		}))
		require.Error(t, err)
	})

	t.Run("rejects an unknown field", func(t *testing.T) {
		_, _, err := qb.BuildSQL(contactTree(&domain.DimensionFilter{
			FieldName:    "not_a_column",
			FieldType:    "time",
			Operator:     "not_in_the_last_days",
			StringValues: []string{"30"},
		}))
		require.Error(t, err)
	})
}

func TestQueryBuilder_CustomEventsGoal_Negate(t *testing.T) {
	qb := NewQueryBuilder()

	goalTree := func(goal *domain.CustomEventsGoalCondition) *domain.TreeNode {
		return &domain.TreeNode{
			Kind: "leaf",
			Leaf: &domain.TreeNodeLeaf{Source: "custom_events_goals", CustomEventsGoal: goal},
		}
	}

	base := func() *domain.CustomEventsGoalCondition {
		return &domain.CustomEventsGoalCondition{
			GoalType:          "purchase",
			AggregateOperator: "count",
			Operator:          "gte",
			Value:             1,
			TimeframeOperator: "in_the_last_days",
			TimeframeValues:   []string{"30"},
		}
	}

	t.Run("negate wraps the leaf in NOT EXISTS", func(t *testing.T) {
		goal := base()
		goal.Negate = true

		sql, args, err := qb.BuildSQL(goalTree(goal))
		require.NoError(t, err)

		assert.Contains(t, sql, "NOT EXISTS (SELECT 1 FROM custom_events ce")
		assert.Contains(t, sql, "ce.occurred_at > NOW() - INTERVAL '30 days'")
		assert.Equal(t, []interface{}{"purchase", 1.0}, args)
	})

	t.Run("without negate the SQL is unchanged", func(t *testing.T) {
		sql, _, err := qb.BuildSQL(goalTree(base()))
		require.NoError(t, err)

		assert.Contains(t, sql, "EXISTS (SELECT 1 FROM custom_events ce")
		assert.NotContains(t, sql, "NOT EXISTS")
	})

	t.Run("negate applies in trigger context too", func(t *testing.T) {
		goal := base()
		goal.Negate = true

		sql, _, err := qb.BuildTriggerCondition(goalTree(goal), "NEW.email")
		require.NoError(t, err)

		assert.Contains(t, sql, "NOT EXISTS (SELECT 1 FROM custom_events ce WHERE ce.email = NEW.email")
	})
}

func TestQueryBuilder_CustomEventsGoal_EventFilters(t *testing.T) {
	qb := NewQueryBuilder()

	goalTree := func(goal *domain.CustomEventsGoalCondition) *domain.TreeNode {
		return &domain.TreeNode{
			Kind: "leaf",
			Leaf: &domain.TreeNodeLeaf{Source: "custom_events_goals", CustomEventsGoal: goal},
		}
	}

	t.Run("event_name is parameterized", func(t *testing.T) {
		eventName := "shopify.order"
		sql, args, err := qb.BuildSQL(goalTree(&domain.CustomEventsGoalCondition{
			GoalType:          "purchase",
			EventName:         &eventName,
			AggregateOperator: "count",
			Operator:          "gte",
			Value:             1,
			TimeframeOperator: "anytime",
		}))
		require.NoError(t, err)

		assert.Contains(t, sql, "ce.event_name = $2")
		assert.Equal(t, []interface{}{"purchase", "shopify.order", 1.0}, args)
	})

	t.Run("property filter binds the key as an argument", func(t *testing.T) {
		sql, args, err := qb.BuildSQL(goalTree(&domain.CustomEventsGoalCondition{
			GoalType:          "purchase",
			AggregateOperator: "count",
			Operator:          "gte",
			Value:             1,
			TimeframeOperator: "anytime",
			Filters: []*domain.DimensionFilter{
				{FieldName: "sku", FieldType: "string", Operator: "equals", StringValues: []string{"A-1"}},
			},
		}))
		require.NoError(t, err)

		assert.Contains(t, sql, "ce.properties->>$2 = $3")
		assert.NotContains(t, sql, "'sku'")
		assert.Equal(t, []interface{}{"purchase", "sku", "A-1", 1.0}, args)
	})

	t.Run("number property filter casts to numeric", func(t *testing.T) {
		sql, args, err := qb.BuildSQL(goalTree(&domain.CustomEventsGoalCondition{
			GoalType:          "purchase",
			AggregateOperator: "sum",
			Operator:          "gte",
			Value:             1,
			TimeframeOperator: "anytime",
			Filters: []*domain.DimensionFilter{
				{FieldName: "quantity", FieldType: "number", Operator: "gte", NumberValues: []float64{2}},
			},
		}))
		require.NoError(t, err)

		// Guarded: an event whose "quantity" holds a non-numeric value must yield NULL rather
		// than abort the whole segment query.
		assert.Contains(t, sql, "CASE WHEN ce.properties->>$2 ~ ")
		assert.Contains(t, sql, "THEN (ce.properties->>$2)::numeric END >= $3")
		assert.Equal(t, []interface{}{"purchase", "quantity", 2.0, 1.0}, args)
	})

	t.Run("multiple filters keep placeholder numbering sequential", func(t *testing.T) {
		goalName := "checkout"
		sql, args, err := qb.BuildSQL(goalTree(&domain.CustomEventsGoalCondition{
			GoalType:          "purchase",
			GoalName:          &goalName,
			AggregateOperator: "count",
			Operator:          "between",
			Value:             1,
			Value2:            func() *float64 { v := 5.0; return &v }(),
			TimeframeOperator: "anytime",
			Filters: []*domain.DimensionFilter{
				{FieldName: "sku", FieldType: "string", Operator: "equals", StringValues: []string{"A-1"}},
				{FieldName: "country", FieldType: "string", Operator: "equals", StringValues: []string{"FR"}},
			},
		}))
		require.NoError(t, err)

		// goal_type, goal_name, sku key, sku value, country key, country value, value, value_2
		assert.Len(t, args, 8)
		assert.Contains(t, sql, "$8")
		assert.NotContains(t, sql, "$9")
	})

	t.Run("rejects a filter with no field name", func(t *testing.T) {
		_, _, err := qb.BuildSQL(goalTree(&domain.CustomEventsGoalCondition{
			GoalType:          "purchase",
			AggregateOperator: "count",
			Operator:          "gte",
			Value:             1,
			TimeframeOperator: "anytime",
			Filters: []*domain.DimensionFilter{
				{FieldName: "", FieldType: "string", Operator: "equals", StringValues: []string{"x"}},
			},
		}))
		require.Error(t, err)
	})
}

func TestQueryBuilder_JSONBKeysAreNeverInterpolated(t *testing.T) {
	qb := NewQueryBuilder()

	// A field name crafted to close the quote and append a sub-select. Interpolating it produced
	// valid SQL whose result the segment preview count leaked one boolean at a time.
	const hostile = `goal_value'->>'new' = (SELECT val FROM secrets) -- `

	t.Run("timeline change key", func(t *testing.T) {
		sql, args, err := qb.BuildSQL(&domain.TreeNode{
			Kind: "leaf",
			Leaf: &domain.TreeNodeLeaf{
				Source: "contact_timeline",
				ContactTimeline: &domain.ContactTimelineCondition{
					Kind:          "custom_event.purchase",
					CountOperator: "at_least",
					CountValue:    1,
					Filters: []*domain.DimensionFilter{
						{FieldName: hostile, FieldType: "string", Operator: "equals", StringValues: []string{"10"}},
					},
				},
			},
		})
		require.NoError(t, err)

		assert.NotContains(t, sql, "SELECT val FROM secrets")
		assert.NotContains(t, sql, "--")
		assert.Contains(t, sql, "ct.changes->$2->>'new'")
		assert.Contains(t, args, hostile)
	})

	t.Run("event property key", func(t *testing.T) {
		sql, args, err := qb.BuildSQL(&domain.TreeNode{
			Kind: "leaf",
			Leaf: &domain.TreeNodeLeaf{
				Source: "custom_events_goals",
				CustomEventsGoal: &domain.CustomEventsGoalCondition{
					GoalType:          "purchase",
					AggregateOperator: "count",
					Operator:          "gte",
					Value:             1,
					TimeframeOperator: "anytime",
					Filters: []*domain.DimensionFilter{
						{FieldName: hostile, FieldType: "string", Operator: "equals", StringValues: []string{"10"}},
					},
				},
			},
		})
		require.NoError(t, err)

		assert.NotContains(t, sql, "SELECT val FROM secrets")
		assert.NotContains(t, sql, "--")
		assert.Contains(t, sql, "ce.properties->>$2")
		assert.Contains(t, args, hostile)
	})
}

func TestQueryBuilder_RelativeDayOperatorsRequireADateField(t *testing.T) {
	qb := NewQueryBuilder()

	// These operators carry no SQL operator of their own and compare against NOW() - INTERVAL.
	// Applied to anything but a timestamp they used to emit SQL that only failed once the
	// segment ran — a syntax error for JSONB paths, a type error for text columns — far from
	// the request that defined the filter.
	for _, operator := range []string{"in_the_last_days", "not_in_the_last_days"} {
		t.Run(operator+" on a text contact column is rejected", func(t *testing.T) {
			_, _, err := qb.BuildSQL(&domain.TreeNode{
				Kind: "leaf",
				Leaf: &domain.TreeNodeLeaf{
					Source: "contacts",
					Contact: &domain.ContactCondition{Filters: []*domain.DimensionFilter{
						{FieldName: "custom_string_1", FieldType: "string", Operator: operator, StringValues: []string{"30"}},
					}},
				},
			})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "date field")
		})

		t.Run(operator+" on a custom_json path typed as string is rejected", func(t *testing.T) {
			_, _, err := qb.BuildSQL(&domain.TreeNode{
				Kind: "leaf",
				Leaf: &domain.TreeNodeLeaf{
					Source: "contacts",
					Contact: &domain.ContactCondition{Filters: []*domain.DimensionFilter{
						{FieldName: "custom_json_1", FieldType: "string", Operator: operator,
							JSONPath: []string{"renewed_at"}, StringValues: []string{"30"}},
					}},
				},
			})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "date value")
		})

		t.Run(operator+" on an event property typed as string is rejected", func(t *testing.T) {
			_, _, err := qb.BuildSQL(&domain.TreeNode{
				Kind: "leaf",
				Leaf: &domain.TreeNodeLeaf{
					Source: "custom_events_goals",
					CustomEventsGoal: &domain.CustomEventsGoalCondition{
						GoalType: "purchase", AggregateOperator: "count", Operator: "gte", Value: 1,
						TimeframeOperator: "anytime",
						Filters: []*domain.DimensionFilter{
							{FieldName: "renewed_at", FieldType: "string", Operator: operator, StringValues: []string{"30"}},
						},
					},
				},
			})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "date value")
		})
	}

	t.Run("not_in_the_last_days on a custom_json path typed as time casts and stays NULL-inclusive", func(t *testing.T) {
		sql, args, err := qb.BuildSQL(&domain.TreeNode{
			Kind: "leaf",
			Leaf: &domain.TreeNodeLeaf{
				Source: "contacts",
				Contact: &domain.ContactCondition{Filters: []*domain.DimensionFilter{
					{FieldName: "custom_json_1", FieldType: "time", Operator: "not_in_the_last_days",
						JSONPath: []string{"renewed_at"}, StringValues: []string{"30"}},
				}},
			},
		})
		require.NoError(t, err)

		assert.Contains(t, sql, "::timestamptz IS NULL OR")
		assert.Contains(t, sql, "INTERVAL '30 days'")
		assert.Empty(t, args)
	})

	t.Run("not_in_the_last_days on a time event property reuses one bound key", func(t *testing.T) {
		sql, args, err := qb.BuildSQL(&domain.TreeNode{
			Kind: "leaf",
			Leaf: &domain.TreeNodeLeaf{
				Source: "custom_events_goals",
				CustomEventsGoal: &domain.CustomEventsGoalCondition{
					GoalType: "purchase", AggregateOperator: "count", Operator: "gte", Value: 1,
					TimeframeOperator: "anytime",
					Filters: []*domain.DimensionFilter{
						{FieldName: "renewed_at", FieldType: "time", Operator: "not_in_the_last_days", StringValues: []string{"30"}},
					},
				},
			},
		})
		require.NoError(t, err)

		// The key is bound once and referenced twice; the args must not gain a second copy.
		assert.Equal(t, 2, strings.Count(sql, "(ce.properties->>$2)::timestamptz"))
		assert.Equal(t, []interface{}{"purchase", "renewed_at", 1.0}, args)
		assert.NotContains(t, sql, "$4")
	})
}

func TestQueryBuilder_NotInTheLastDaysBoundary(t *testing.T) {
	qb := NewQueryBuilder()

	// The two operators must partition the non-NULL rows exactly: > for the window, <= outside
	// it. A boundary row (exactly N days old) belongs to precisely one of them, and any drift
	// here silently double-counts or strands contacts on the day their window rolls over.
	positive, _, err := qb.BuildSQL(&domain.TreeNode{
		Kind: "leaf",
		Leaf: &domain.TreeNodeLeaf{
			Source: "contacts",
			Contact: &domain.ContactCondition{Filters: []*domain.DimensionFilter{
				{FieldName: "custom_datetime_1", FieldType: "time", Operator: "in_the_last_days", StringValues: []string{"30"}},
			}},
		},
	})
	require.NoError(t, err)

	negative, _, err := qb.BuildSQL(&domain.TreeNode{
		Kind: "leaf",
		Leaf: &domain.TreeNodeLeaf{
			Source: "contacts",
			Contact: &domain.ContactCondition{Filters: []*domain.DimensionFilter{
				{FieldName: "custom_datetime_1", FieldType: "time", Operator: "not_in_the_last_days", StringValues: []string{"30"}},
			}},
		},
	})
	require.NoError(t, err)

	assert.Contains(t, positive, "custom_datetime_1 > NOW() - INTERVAL '30 days'")
	assert.Contains(t, negative, "custom_datetime_1 <= NOW() - INTERVAL '30 days'")
	// Strictly complementary comparisons, so the boundary instant falls on exactly one side.
	assert.NotContains(t, negative, "custom_datetime_1 < NOW()")
	// Plus the NULLs, which the positive form can never match.
	assert.Contains(t, negative, "custom_datetime_1 IS NULL OR")
	assert.NotContains(t, positive, "IS NULL")
}

// ============================================================================
// A scalar stored in a custom_json column must not satisfy a key or numeric filter
// ============================================================================

// contacts.upsert used to answer 400 for anything but an object or an array in the five
// custom_json columns, so every expression the segment builder emits for them could assume a
// container. It no longer can: a scalar is stored as-is, matching what lists.subscribe had always
// accepted. Two of those expressions quietly change meaning when the value is a scalar, and both
// change it in the direction of matching contacts that hold nothing of the sort.
func TestQueryBuilder_JSONScalarCannotSatisfyKeyOrNumericFilters(t *testing.T) {
	qb := NewQueryBuilder()

	segmentSQL := func(t *testing.T, filter *domain.DimensionFilter) string {
		t.Helper()
		sql, _, err := qb.BuildSQL(&domain.TreeNode{
			Kind: "leaf",
			Leaf: &domain.TreeNodeLeaf{
				Source:  "contacts",
				Contact: &domain.ContactCondition{Filters: []*domain.DimensionFilter{filter}},
			},
		})
		require.NoError(t, err)
		return sql
	}

	t.Run("a key check requires an object, in the segment and the trigger form alike", func(t *testing.T) {
		filter := &domain.DimensionFilter{
			FieldName: "custom_json_1",
			FieldType: "json",
			Operator:  "is_set",
			JSONPath:  []string{"tier"},
		}

		assert.Contains(t, segmentSQL(t, filter),
			"(jsonb_typeof(custom_json_1) = 'object' AND custom_json_1 ? $1)")

		// The same filter compiled into an automation's PL/pgSQL trigger body, where a wrong
		// answer enrols the wrong contacts instead of mailing them.
		trigger, _, err := qb.BuildTriggerCondition(&domain.TreeNode{
			Kind: "leaf",
			Leaf: &domain.TreeNodeLeaf{
				Source:  "contacts",
				Contact: &domain.ContactCondition{Filters: []*domain.DimensionFilter{filter}},
			},
		}, "NEW.email")
		require.NoError(t, err)
		assert.Contains(t, trigger, "(jsonb_typeof(custom_json_1) = 'object' AND custom_json_1 ? $1)")
	})

	// field_type "string" rather than "json": tree validation demands a json_path from a "json"
	// filter, so the whole-column form of in_array is reachable only through the API, and only
	// spelled this way. It routes on the column's whitelist type, not the filter's.
	t.Run("an element check requires a container", func(t *testing.T) {
		sql := segmentSQL(t, &domain.DimensionFilter{
			FieldName:    "custom_json_1",
			FieldType:    "string",
			Operator:     "in_array",
			StringValues: []string{"gold"},
		})
		assert.Contains(t, sql, "(jsonb_typeof(custom_json_1) IN ('object', 'array') AND custom_json_1 ? $1)")
	})

	t.Run("a numeric comparison against the whole column is guarded", func(t *testing.T) {
		sql := segmentSQL(t, &domain.DimensionFilter{
			FieldName:    "custom_json_1",
			FieldType:    "number",
			Operator:     "gte",
			NumberValues: []float64{10},
		})
		assert.Contains(t, sql,
			"CASE WHEN jsonb_typeof(custom_json_1) = 'object' THEN (custom_json_1::text)::numeric END >= $1")
	})

	t.Run("a date comparison against the whole column is guarded", func(t *testing.T) {
		sql := segmentSQL(t, &domain.DimensionFilter{
			FieldName:    "custom_json_1",
			FieldType:    "time",
			Operator:     "after_date",
			StringValues: []string{"2024-01-01T00:00:00Z"},
		})
		assert.Contains(t, sql,
			"CASE WHEN jsonb_typeof(custom_json_1) = 'object' THEN (custom_json_1::text)::timestamptz END > $1")
	})

	t.Run("a relative-day comparison against the whole column is guarded", func(t *testing.T) {
		sql := segmentSQL(t, &domain.DimensionFilter{
			FieldName:    "custom_json_1",
			FieldType:    "time",
			Operator:     "in_the_last_days",
			StringValues: []string{"7"},
		})
		assert.Contains(t, sql,
			"CASE WHEN jsonb_typeof(custom_json_1) = 'object' THEN (custom_json_1::text)::timestamptz END > NOW() - INTERVAL '7 days'")
	})

	// The guard belongs to the whole-column form only. A path already answers NULL on a scalar,
	// and a jsonb_typeof(custom_json_1) = 'object' test in front of an array index would refuse
	// the array it is indexing into.
	t.Run("addressing a value inside the column is left alone", func(t *testing.T) {
		byKey := segmentSQL(t, &domain.DimensionFilter{
			FieldName:    "custom_json_1",
			FieldType:    "number",
			Operator:     "gte",
			JSONPath:     []string{"score"},
			NumberValues: []float64{10},
		})
		assert.Contains(t, byKey, "(custom_json_1['score']::text)::numeric >= $1")
		assert.NotContains(t, byKey, "jsonb_typeof")

		byIndex := segmentSQL(t, &domain.DimensionFilter{
			FieldName:    "custom_json_1",
			FieldType:    "number",
			Operator:     "gte",
			JSONPath:     []string{"0"},
			NumberValues: []float64{10},
		})
		assert.Contains(t, byIndex, "(custom_json_1[0]::text)::numeric >= $1")
		assert.NotContains(t, byIndex, "jsonb_typeof")
	})
}

// openSegmentTestPostgres connects to the PostgreSQL that tests/compose.test.yaml starts, or skips.
//
// Everything above asserts a string, which is exactly the kind of test the scalar hole walked past:
// `custom_json_1 ? $1` reads as an object-key lookup and is not one, and no assertion over Go
// strings can tell the difference. Only the server settles what the emitted SQL means, so these
// cases run against it.
func openSegmentTestPostgres(t *testing.T) *sql.DB {
	t.Helper()

	host := envOrDefault("TEST_DB_HOST", "localhost")
	port := envOrDefault("TEST_DB_PORT", "5433")
	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable connect_timeout=2",
		host, port,
		envOrDefault("TEST_DB_USER", "notifuse_test"),
		envOrDefault("TEST_DB_PASSWORD", "test_password"),
		envOrDefault("TEST_DB_NAME", "postgres"),
	)

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Skipf("no test PostgreSQL at %s:%s: %v", host, port, err)
	}
	// A single connection, because the fixtures live in a TEMP table and only the session that
	// created it can see one.
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		db.Close()
		t.Skipf("no test PostgreSQL at %s:%s (docker compose -f tests/compose.test.yaml up -d): %v", host, port, err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// seedJSONContacts creates the TEMP contacts table the generated SQL selects from and fills it with
// one row per custom_json_1 shape. The table is named "contacts" because BuildSQL hard-codes that.
func seedJSONContacts(t *testing.T, db *sql.DB, rows map[string]string) {
	t.Helper()

	_, err := db.Exec(`DROP TABLE IF EXISTS contacts`)
	require.NoError(t, err)
	_, err = db.Exec(`CREATE TEMP TABLE contacts (email text PRIMARY KEY, custom_json_1 jsonb)`)
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = db.Exec(`DROP TABLE IF EXISTS contacts`) })

	for email, value := range rows {
		if value == "" {
			_, err = db.Exec(`INSERT INTO contacts (email, custom_json_1) VALUES ($1, NULL)`, email)
		} else {
			_, err = db.Exec(`INSERT INTO contacts (email, custom_json_1) VALUES ($1, $2::jsonb)`, email, value)
		}
		require.NoError(t, err)
	}
}

func matchedEmails(t *testing.T, db *sql.DB, query string, args []interface{}) []string {
	t.Helper()

	rows, err := db.Query(query, args...)
	require.NoError(t, err, "query: %s", query)
	defer rows.Close()

	var emails []string
	for rows.Next() {
		var email string
		require.NoError(t, rows.Scan(&email))
		emails = append(emails, email)
	}
	require.NoError(t, rows.Err())
	sort.Strings(emails)
	return emails
}

func TestQueryBuilder_JSONScalarSemanticsOnPostgres(t *testing.T) {
	db := openSegmentTestPostgres(t)
	qb := NewQueryBuilder()

	buildSegment := func(t *testing.T, filter *domain.DimensionFilter) (string, []interface{}) {
		t.Helper()
		query, args, err := qb.BuildSQL(&domain.TreeNode{
			Kind: "leaf",
			Leaf: &domain.TreeNodeLeaf{
				Source:  "contacts",
				Contact: &domain.ContactCondition{Filters: []*domain.DimensionFilter{filter}},
			},
		})
		require.NoError(t, err)
		return query, args
	}

	t.Run("only a contact holding the key matches is_set", func(t *testing.T) {
		seedJSONContacts(t, db, map[string]string{
			"object@example.com": `{"tier":"gold"}`,
			"scalar@example.com": `"tier"`,
			"array@example.com":  `["tier"]`,
			"number@example.com": `42`,
			"null@example.com":   ``,
		})

		query, args := buildSegment(t, &domain.DimensionFilter{
			FieldName: "custom_json_1",
			FieldType: "json",
			Operator:  "is_set",
			JSONPath:  []string{"tier"},
		})

		// scalar@example.com is the contact this finding is about: `'"tier"'::jsonb ? 'tier'` is
		// true on PostgreSQL 17, so before the guard the bare string "tier" joined every segment
		// asking for the key. array@example.com went in for the same reason.
		assert.Equal(t, []string{"object@example.com"}, matchedEmails(t, db, query, args))
	})

	t.Run("a contact holding the key stops matching is_not_set", func(t *testing.T) {
		seedJSONContacts(t, db, map[string]string{
			"object@example.com": `{"tier":"gold"}`,
			"scalar@example.com": `"tier"`,
			"other@example.com":  `{"plan":"free"}`,
		})

		query, args := buildSegment(t, &domain.DimensionFilter{
			FieldName: "custom_json_1",
			FieldType: "json",
			Operator:  "is_not_set",
			JSONPath:  []string{"tier"},
		})

		assert.Equal(t, []string{"other@example.com", "scalar@example.com"},
			matchedEmails(t, db, query, args))
	})

	t.Run("only a container matches in_array", func(t *testing.T) {
		seedJSONContacts(t, db, map[string]string{
			"array@example.com":  `["gold","silver"]`,
			"object@example.com": `{"gold":true}`,
			"scalar@example.com": `"gold"`,
			"other@example.com":  `["silver"]`,
		})

		query, args := buildSegment(t, &domain.DimensionFilter{
			FieldName:    "custom_json_1",
			FieldType:    "string",
			Operator:     "in_array",
			StringValues: []string{"gold"},
		})

		assert.Equal(t, []string{"array@example.com", "object@example.com"},
			matchedEmails(t, db, query, args))
	})

	// The sibling defect. A number in the column made (custom_json_1::text)::numeric succeed where
	// it had raised on every row, so a filter that used to fail loudly and always started matching
	// instead — and whether it matched or raised depended on which rows the planner reached, which
	// is a segment that works until the plan changes.
	t.Run("a stored number does not satisfy a whole-column numeric comparison", func(t *testing.T) {
		seedJSONContacts(t, db, map[string]string{
			"number@example.com": `42`,
			"string@example.com": `"42"`,
		})

		query, args := buildSegment(t, &domain.DimensionFilter{
			FieldName:    "custom_json_1",
			FieldType:    "number",
			Operator:     "gte",
			NumberValues: []float64{10},
		})

		assert.Empty(t, matchedEmails(t, db, query, args))
	})

	t.Run("a stored date string does not satisfy a whole-column date comparison", func(t *testing.T) {
		seedJSONContacts(t, db, map[string]string{
			"date@example.com": `"2024-06-01"`,
		})

		query, args := buildSegment(t, &domain.DimensionFilter{
			FieldName:    "custom_json_1",
			FieldType:    "time",
			Operator:     "after_date",
			StringValues: []string{"2024-01-01T00:00:00Z"},
		})

		assert.Empty(t, matchedEmails(t, db, query, args))
	})

	// The trigger form casts defensively, so nothing raises there and the wrong answer is silent.
	// It also nests the new guard inside the existing CASE, which is the part only the server can
	// confirm is still valid SQL.
	t.Run("the trigger form guards the same numeric comparison", func(t *testing.T) {
		seedJSONContacts(t, db, map[string]string{
			"number@example.com": `42`,
			"scored@example.com": `{"score":42}`,
		})

		condition, args, err := qb.BuildTriggerCondition(&domain.TreeNode{
			Kind: "leaf",
			Leaf: &domain.TreeNodeLeaf{
				Source: "contacts",
				Contact: &domain.ContactCondition{Filters: []*domain.DimensionFilter{{
					FieldName:    "custom_json_1",
					FieldType:    "number",
					Operator:     "gte",
					NumberValues: []float64{10},
				}}},
			},
		}, "c.email")
		require.NoError(t, err)

		assert.Empty(t, matchedEmails(t,
			db, "SELECT c.email FROM contacts c WHERE "+condition, args))
	})

	// Guarding the whole-column form must not cost the addressed forms, which are what the console
	// actually builds: it requires a JSON path for every operator except is_set/is_not_set.
	t.Run("a value addressed inside the column still matches", func(t *testing.T) {
		seedJSONContacts(t, db, map[string]string{
			"key@example.com":    `{"score":42}`,
			"index@example.com":  `[42]`,
			"scalar@example.com": `42`,
			"low@example.com":    `{"score":1}`,
		})

		byKey, keyArgs := buildSegment(t, &domain.DimensionFilter{
			FieldName:    "custom_json_1",
			FieldType:    "number",
			Operator:     "gte",
			JSONPath:     []string{"score"},
			NumberValues: []float64{10},
		})
		assert.Equal(t, []string{"key@example.com"}, matchedEmails(t, db, byKey, keyArgs))

		byIndex, indexArgs := buildSegment(t, &domain.DimensionFilter{
			FieldName:    "custom_json_1",
			FieldType:    "number",
			Operator:     "gte",
			JSONPath:     []string{"0"},
			NumberValues: []float64{10},
		})
		assert.Equal(t, []string{"index@example.com"}, matchedEmails(t, db, byIndex, indexArgs))
	})
}
