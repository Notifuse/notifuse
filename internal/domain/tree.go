package domain

import (
	"encoding/json"
	"fmt"
)

// TreeNode represents a node in the segment tree structure
// It can be either a branch (AND/OR operator) or a leaf (actual condition)
type TreeNode struct {
	Kind   string          `json:"kind"` // "branch" or "leaf"
	Branch *TreeNodeBranch `json:"branch,omitempty"`
	Leaf   *TreeNodeLeaf   `json:"leaf,omitempty"`
}

// TreeNodeBranch represents a logical operator (AND/OR) with child nodes
type TreeNodeBranch struct {
	Operator string      `json:"operator"` // "and" or "or"
	Leaves   []*TreeNode `json:"leaves"`
}

// TreeNodeLeaf represents an actual condition on a table
type TreeNodeLeaf struct {
	Table           string                    `json:"table"` // "contacts", "contact_lists", "contact_timeline"
	Contact         *ContactCondition         `json:"contact,omitempty"`
	ContactList     *ContactListCondition     `json:"contact_list,omitempty"`
	ContactTimeline *ContactTimelineCondition `json:"contact_timeline,omitempty"`
}

// ContactCondition represents filters on the contacts table
type ContactCondition struct {
	Filters []*DimensionFilter `json:"filters"`
}

// ContactListCondition represents membership conditions for contact lists
type ContactListCondition struct {
	Operator string  `json:"operator"` // "in" or "not_in"
	ListID   string  `json:"list_id"`
	Status   *string `json:"status,omitempty"`
}

// ContactTimelineCondition represents conditions on contact timeline events
type ContactTimelineCondition struct {
	Kind              string             `json:"kind"`           // Timeline event kind
	CountOperator     string             `json:"count_operator"` // "at_least", "at_most", "exactly"
	CountValue        int                `json:"count_value"`
	TimeframeOperator *string            `json:"timeframe_operator,omitempty"` // "anytime", "in_date_range", "before_date", "after_date", "in_the_last_days"
	TimeframeValues   []string           `json:"timeframe_values,omitempty"`
	Filters           []*DimensionFilter `json:"filters,omitempty"`
}

// DimensionFilter represents a single filter condition on a field
type DimensionFilter struct {
	FieldName    string    `json:"field_name"`
	FieldType    string    `json:"field_type"` // "string", "number", "time"
	Operator     string    `json:"operator"`   // "equals", "not_equals", "gt", "gte", "lt", "lte", "contains", etc.
	StringValues []string  `json:"string_values,omitempty"`
	NumberValues []float64 `json:"number_values,omitempty"`
}

// Validate validates the tree structure
func (t *TreeNode) Validate() error {
	if t.Kind == "" {
		return fmt.Errorf("tree node must have 'kind' field")
	}

	switch t.Kind {
	case "branch":
		if t.Branch == nil {
			return fmt.Errorf("branch node must have 'branch' field")
		}
		return t.Branch.Validate()
	case "leaf":
		if t.Leaf == nil {
			return fmt.Errorf("leaf node must have 'leaf' field")
		}
		return t.Leaf.Validate()
	default:
		return fmt.Errorf("invalid tree node kind: %s (must be 'branch' or 'leaf')", t.Kind)
	}
}

// Validate validates a branch node
func (b *TreeNodeBranch) Validate() error {
	if b.Operator != "and" && b.Operator != "or" {
		return fmt.Errorf("invalid branch operator: %s (must be 'and' or 'or')", b.Operator)
	}

	if len(b.Leaves) == 0 {
		return fmt.Errorf("branch must have at least one leaf")
	}

	for i, leaf := range b.Leaves {
		if leaf == nil {
			return fmt.Errorf("branch leaf %d is nil", i)
		}
		if err := leaf.Validate(); err != nil {
			return fmt.Errorf("branch leaf %d: %w", i, err)
		}
	}

	return nil
}

// Validate validates a leaf node
func (l *TreeNodeLeaf) Validate() error {
	if l.Table == "" {
		return fmt.Errorf("leaf must have 'table' field")
	}

	switch l.Table {
	case "contacts":
		if l.Contact == nil {
			return fmt.Errorf("leaf with table 'contacts' must have 'contact' field")
		}
		return l.Contact.Validate()
	case "contact_lists":
		if l.ContactList == nil {
			return fmt.Errorf("leaf with table 'contact_lists' must have 'contact_list' field")
		}
		return l.ContactList.Validate()
	case "contact_timeline":
		if l.ContactTimeline == nil {
			return fmt.Errorf("leaf with table 'contact_timeline' must have 'contact_timeline' field")
		}
		return l.ContactTimeline.Validate()
	default:
		return fmt.Errorf("invalid table: %s (must be 'contacts', 'contact_lists', or 'contact_timeline')", l.Table)
	}
}

// Validate validates contact conditions
func (c *ContactCondition) Validate() error {
	if len(c.Filters) == 0 {
		return fmt.Errorf("contact condition must have at least one filter")
	}

	for i, filter := range c.Filters {
		if filter == nil {
			return fmt.Errorf("filter %d is nil", i)
		}
		if err := filter.Validate(); err != nil {
			return fmt.Errorf("filter %d: %w", i, err)
		}
	}

	return nil
}

// Validate validates contact list conditions
func (c *ContactListCondition) Validate() error {
	if c.Operator != "in" && c.Operator != "not_in" {
		return fmt.Errorf("invalid contact_list operator: %s (must be 'in' or 'not_in')", c.Operator)
	}

	if c.ListID == "" {
		return fmt.Errorf("contact_list condition must have 'list_id'")
	}

	return nil
}

// Validate validates contact timeline conditions
func (c *ContactTimelineCondition) Validate() error {
	if c.Kind == "" {
		return fmt.Errorf("contact_timeline condition must have 'kind'")
	}

	if c.CountOperator != "at_least" && c.CountOperator != "at_most" && c.CountOperator != "exactly" {
		return fmt.Errorf("invalid count_operator: %s (must be 'at_least', 'at_most', or 'exactly')", c.CountOperator)
	}

	if c.CountValue < 0 {
		return fmt.Errorf("count_value must be non-negative")
	}

	if c.TimeframeOperator != nil {
		switch *c.TimeframeOperator {
		case "anytime", "in_date_range", "before_date", "after_date", "in_the_last_days":
			// Valid
		default:
			return fmt.Errorf("invalid timeframe_operator: %s", *c.TimeframeOperator)
		}
	}

	// Validate filters if present
	for i, filter := range c.Filters {
		if filter == nil {
			return fmt.Errorf("filter %d is nil", i)
		}
		if err := filter.Validate(); err != nil {
			return fmt.Errorf("filter %d: %w", i, err)
		}
	}

	return nil
}

// Validate validates a dimension filter
func (f *DimensionFilter) Validate() error {
	if f.FieldName == "" {
		return fmt.Errorf("filter must have 'field_name'")
	}

	if f.FieldType == "" {
		return fmt.Errorf("filter must have 'field_type'")
	}

	if f.FieldType != "string" && f.FieldType != "number" && f.FieldType != "time" {
		return fmt.Errorf("invalid field_type: %s (must be 'string', 'number', or 'time')", f.FieldType)
	}

	if f.Operator == "" {
		return fmt.Errorf("filter must have 'operator'")
	}

	// Validate that we have appropriate values based on field type
	// (except for operators like is_set/is_not_set that don't need values)
	if f.Operator != "is_set" && f.Operator != "is_not_set" {
		switch f.FieldType {
		case "string", "time":
			if len(f.StringValues) == 0 {
				return fmt.Errorf("%s filter must have 'string_values'", f.FieldType)
			}
		case "number":
			if len(f.NumberValues) == 0 {
				return fmt.Errorf("number filter must have 'number_values'")
			}
		}
	}

	return nil
}

// ToMapOfAny converts a TreeNode to MapOfAny (for backwards compatibility)
func (t *TreeNode) ToMapOfAny() (MapOfAny, error) {
	data, err := json.Marshal(t)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal tree node: %w", err)
	}

	var result MapOfAny
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal tree node: %w", err)
	}

	return result, nil
}

// TreeNodeFromMapOfAny converts a MapOfAny to a TreeNode
func TreeNodeFromMapOfAny(data MapOfAny) (*TreeNode, error) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal map: %w", err)
	}

	var node TreeNode
	if err := json.Unmarshal(jsonData, &node); err != nil {
		return nil, fmt.Errorf("failed to unmarshal tree node: %w", err)
	}

	return &node, nil
}

// TreeNodeFromJSON parses a JSON string into a TreeNode
func TreeNodeFromJSON(jsonStr string) (*TreeNode, error) {
	var node TreeNode
	if err := json.Unmarshal([]byte(jsonStr), &node); err != nil {
		return nil, fmt.Errorf("failed to parse tree node JSON: %w", err)
	}

	return &node, nil
}

// HasRelativeDates checks if the tree contains any relative date filters
// that require daily recomputation (e.g., "in_the_last_days")
func (t *TreeNode) HasRelativeDates() bool {
	if t == nil {
		return false
	}

	switch t.Kind {
	case "branch":
		if t.Branch == nil {
			return false
		}
		// Check all child leaves
		for _, leaf := range t.Branch.Leaves {
			if leaf.HasRelativeDates() {
				return true
			}
		}
		return false

	case "leaf":
		if t.Leaf == nil {
			return false
		}
		// Check contact timeline conditions for relative date operators
		if t.Leaf.ContactTimeline != nil {
			if t.Leaf.ContactTimeline.TimeframeOperator != nil &&
				*t.Leaf.ContactTimeline.TimeframeOperator == "in_the_last_days" {
				return true
			}
		}
		// Check contact property filters for relative date operators
		if t.Leaf.Contact != nil && t.Leaf.Contact.Filters != nil {
			for _, filter := range t.Leaf.Contact.Filters {
				if filter.Operator == "in_the_last_days" {
					return true
				}
			}
		}
		return false

	default:
		return false
	}
}
