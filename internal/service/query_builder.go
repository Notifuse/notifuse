package service

import (
	"fmt"
	"strings"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
)

// QueryBuilder converts segment tree structures into safe, parameterized SQL queries
type QueryBuilder struct {
	allowedFields    map[string]fieldConfig
	allowedOperators map[string]sqlOperator
}

// fieldConfig defines metadata for a field
type fieldConfig struct {
	dbColumn  string
	fieldType string // "string", "number", "time"
}

// sqlOperator defines how to convert an operator to SQL
type sqlOperator struct {
	sql           string
	requiresValue bool
}

// NewQueryBuilder creates a new query builder with field and operator whitelists
func NewQueryBuilder() *QueryBuilder {
	qb := &QueryBuilder{
		allowedFields:    make(map[string]fieldConfig),
		allowedOperators: make(map[string]sqlOperator),
	}

	// Initialize field whitelist for contacts table
	qb.initializeContactFields()

	// Initialize operator whitelist
	qb.initializeOperators()

	return qb
}

// initializeContactFields sets up the whitelist of allowed contact fields
func (qb *QueryBuilder) initializeContactFields() {
	// String fields
	stringFields := []string{
		"email", "external_id", "timezone", "language",
		"first_name", "last_name", "phone",
		"address_line_1", "address_line_2", "country", "postcode", "state",
		"job_title",
		"custom_string_1", "custom_string_2", "custom_string_3", "custom_string_4", "custom_string_5",
	}
	for _, field := range stringFields {
		qb.allowedFields[field] = fieldConfig{
			dbColumn:  field,
			fieldType: "string",
		}
	}

	// Number fields
	numberFields := []string{
		"lifetime_value", "orders_count",
		"custom_number_1", "custom_number_2", "custom_number_3", "custom_number_4", "custom_number_5",
	}
	for _, field := range numberFields {
		qb.allowedFields[field] = fieldConfig{
			dbColumn:  field,
			fieldType: "number",
		}
	}

	// Time fields
	timeFields := []string{
		"last_order_at", "created_at", "updated_at",
		"custom_datetime_1", "custom_datetime_2", "custom_datetime_3", "custom_datetime_4", "custom_datetime_5",
	}
	for _, field := range timeFields {
		qb.allowedFields[field] = fieldConfig{
			dbColumn:  field,
			fieldType: "time",
		}
	}
}

// initializeOperators sets up the whitelist of allowed operators
func (qb *QueryBuilder) initializeOperators() {
	qb.allowedOperators = map[string]sqlOperator{
		// Comparison operators
		"equals":     {sql: "=", requiresValue: true},
		"not_equals": {sql: "!=", requiresValue: true},
		"gt":         {sql: ">", requiresValue: true},
		"gte":        {sql: ">=", requiresValue: true},
		"lt":         {sql: "<", requiresValue: true},
		"lte":        {sql: "<=", requiresValue: true},

		// String operators
		"contains":     {sql: "ILIKE", requiresValue: true}, // Case-insensitive LIKE
		"not_contains": {sql: "NOT ILIKE", requiresValue: true},

		// Null checks
		"is_set":     {sql: "IS NOT NULL", requiresValue: false},
		"is_not_set": {sql: "IS NULL", requiresValue: false},

		// Date range operators (will be handled specially)
		"in_date_range":     {sql: "BETWEEN", requiresValue: true},
		"not_in_date_range": {sql: "NOT BETWEEN", requiresValue: true},
		"before_date":       {sql: "<", requiresValue: true},
		"after_date":        {sql: ">", requiresValue: true},
	}
}

// BuildSQL converts a segment tree into parameterized SQL
// Returns: sql string, args []interface{}, error
func (qb *QueryBuilder) BuildSQL(tree *domain.TreeNode) (string, []interface{}, error) {
	if tree == nil {
		return "", nil, fmt.Errorf("tree cannot be nil")
	}

	// Validate the tree structure
	if err := tree.Validate(); err != nil {
		return "", nil, fmt.Errorf("invalid tree: %w", err)
	}

	// Start with base query
	var conditions []string
	var args []interface{}
	argIndex := 1

	// Parse the tree recursively
	condition, newArgs, _, err := qb.parseNode(tree, argIndex)
	if err != nil {
		return "", nil, err
	}

	if condition != "" {
		conditions = append(conditions, condition)
		args = append(args, newArgs...)
	}

	// Build final SQL
	sql := "SELECT email FROM contacts"
	if len(conditions) > 0 {
		sql += " WHERE " + strings.Join(conditions, " AND ")
	}

	return sql, args, nil
}

// parseNode recursively parses a tree node
func (qb *QueryBuilder) parseNode(node *domain.TreeNode, argIndex int) (string, []interface{}, int, error) {
	switch node.Kind {
	case "branch":
		return qb.parseBranch(node.Branch, argIndex)
	case "leaf":
		return qb.parseLeaf(node.Leaf, argIndex)
	default:
		return "", nil, argIndex, fmt.Errorf("invalid node kind: %s", node.Kind)
	}
}

// parseBranch parses a branch node (AND/OR operator with children)
func (qb *QueryBuilder) parseBranch(branch *domain.TreeNodeBranch, argIndex int) (string, []interface{}, int, error) {
	if branch == nil {
		return "", nil, argIndex, fmt.Errorf("branch cannot be nil")
	}

	var conditions []string
	var args []interface{}

	for _, leaf := range branch.Leaves {
		condition, newArgs, newArgIndex, err := qb.parseNode(leaf, argIndex)
		if err != nil {
			return "", nil, argIndex, err
		}

		if condition != "" {
			conditions = append(conditions, condition)
			args = append(args, newArgs...)
			argIndex = newArgIndex
		}
	}

	if len(conditions) == 0 {
		return "", nil, argIndex, nil
	}

	sqlOperator := " AND "
	if branch.Operator == "or" {
		sqlOperator = " OR "
	}

	// Wrap in parentheses for proper precedence
	result := "(" + strings.Join(conditions, sqlOperator) + ")"
	return result, args, argIndex, nil
}

// parseLeaf parses a leaf node (actual condition)
func (qb *QueryBuilder) parseLeaf(leaf *domain.TreeNodeLeaf, argIndex int) (string, []interface{}, int, error) {
	if leaf == nil {
		return "", nil, argIndex, fmt.Errorf("leaf cannot be nil")
	}

	switch leaf.Table {
	case "contacts":
		if leaf.Contact == nil {
			return "", nil, argIndex, fmt.Errorf("leaf with table 'contacts' must have 'contact' field")
		}
		return qb.parseContactConditions(leaf.Contact, argIndex)

	case "contact_lists":
		if leaf.ContactList == nil {
			return "", nil, argIndex, fmt.Errorf("leaf with table 'contact_lists' must have 'contact_list' field")
		}
		return qb.parseContactListConditions(leaf.ContactList, argIndex)

	case "contact_timeline":
		if leaf.ContactTimeline == nil {
			return "", nil, argIndex, fmt.Errorf("leaf with table 'contact_timeline' must have 'contact_timeline' field")
		}
		return qb.parseContactTimelineConditions(leaf.ContactTimeline, argIndex)

	default:
		return "", nil, argIndex, fmt.Errorf("unsupported table: %s (supported: 'contacts', 'contact_lists', 'contact_timeline')", leaf.Table)
	}
}

// parseContactConditions parses contact filter conditions
func (qb *QueryBuilder) parseContactConditions(contact *domain.ContactCondition, argIndex int) (string, []interface{}, int, error) {
	if contact == nil {
		return "", nil, argIndex, fmt.Errorf("contact condition cannot be nil")
	}

	var conditions []string
	var args []interface{}

	for _, filter := range contact.Filters {
		condition, newArgs, newArgIndex, err := qb.parseFilter(filter, argIndex)
		if err != nil {
			return "", nil, argIndex, err
		}

		if condition != "" {
			conditions = append(conditions, condition)
			args = append(args, newArgs...)
			argIndex = newArgIndex
		}
	}

	if len(conditions) == 0 {
		return "", nil, argIndex, nil
	}

	// Contact conditions are ANDed together
	result := "(" + strings.Join(conditions, " AND ") + ")"
	return result, args, argIndex, nil
}

// parseFilter parses a single filter (field + operator + value)
func (qb *QueryBuilder) parseFilter(filter *domain.DimensionFilter, argIndex int) (string, []interface{}, int, error) {
	if filter == nil {
		return "", nil, argIndex, fmt.Errorf("filter cannot be nil")
	}

	// Validate field exists in whitelist
	fieldCfg, ok := qb.allowedFields[filter.FieldName]
	if !ok {
		return "", nil, argIndex, fmt.Errorf("invalid field name: %s", filter.FieldName)
	}

	// Validate operator exists in whitelist
	sqlOp, ok := qb.allowedOperators[filter.Operator]
	if !ok {
		return "", nil, argIndex, fmt.Errorf("invalid operator: %s", filter.Operator)
	}

	// Handle operators that don't require values
	if !sqlOp.requiresValue {
		return fmt.Sprintf("%s %s", fieldCfg.dbColumn, sqlOp.sql), nil, argIndex, nil
	}

	// Get values based on field type
	var values []interface{}
	var err error

	fieldType := filter.FieldType
	if fieldType == "" {
		fieldType = fieldCfg.fieldType // Use whitelist type if not provided
	}

	switch fieldType {
	case "string":
		values, err = qb.getStringValues(filter)
	case "number":
		values, err = qb.getNumberValues(filter)
	case "time":
		values, err = qb.getTimeValues(filter)
	default:
		return "", nil, argIndex, fmt.Errorf("invalid field type: %s", fieldType)
	}

	if err != nil {
		return "", nil, argIndex, err
	}

	if len(values) == 0 {
		return "", nil, argIndex, fmt.Errorf("filter must have values for operator %s", filter.Operator)
	}

	// Build SQL condition based on operator
	return qb.buildCondition(fieldCfg.dbColumn, filter.Operator, sqlOp, values, argIndex)
}

// getStringValues extracts string values from filter
func (qb *QueryBuilder) getStringValues(filter *domain.DimensionFilter) ([]interface{}, error) {
	if len(filter.StringValues) == 0 {
		return nil, fmt.Errorf("string filter must have 'string_values'")
	}

	var values []interface{}
	for _, v := range filter.StringValues {
		values = append(values, v)
	}

	return values, nil
}

// getNumberValues extracts number values from filter
func (qb *QueryBuilder) getNumberValues(filter *domain.DimensionFilter) ([]interface{}, error) {
	if len(filter.NumberValues) == 0 {
		return nil, fmt.Errorf("number filter must have 'number_values'")
	}

	var values []interface{}
	for _, v := range filter.NumberValues {
		values = append(values, v)
	}

	return values, nil
}

// getTimeValues extracts time values from filter
func (qb *QueryBuilder) getTimeValues(filter *domain.DimensionFilter) ([]interface{}, error) {
	// Time values come as strings in StringValues
	if len(filter.StringValues) == 0 {
		return nil, fmt.Errorf("time filter must have 'string_values' (ISO8601 dates)")
	}

	var values []interface{}
	for _, str := range filter.StringValues {
		// Parse and validate time
		t, err := time.Parse(time.RFC3339, str)
		if err != nil {
			// Try alternative format
			t, err = time.Parse("2006-01-02", str)
			if err != nil {
				return nil, fmt.Errorf("invalid time value: %s (expected ISO8601 or YYYY-MM-DD)", str)
			}
		}

		values = append(values, t)
	}

	return values, nil
}

// parseContactListConditions generates SQL for contact_lists filtering
// Uses EXISTS subquery to check if contact is in specific list(s)
func (qb *QueryBuilder) parseContactListConditions(contactList *domain.ContactListCondition, argIndex int) (string, []interface{}, int, error) {
	if contactList == nil {
		return "", nil, argIndex, fmt.Errorf("contact_list condition cannot be nil")
	}

	if contactList.ListID == "" {
		return "", nil, argIndex, fmt.Errorf("contact_list must have 'list_id'")
	}

	var args []interface{}
	var conditions []string

	// Build the EXISTS subquery
	args = append(args, contactList.ListID)
	conditions = append(conditions, fmt.Sprintf("cl.list_id = $%d", argIndex))
	argIndex++

	// Add status filter if provided
	if contactList.Status != nil && *contactList.Status != "" {
		args = append(args, *contactList.Status)
		conditions = append(conditions, fmt.Sprintf("cl.status = $%d", argIndex))
		argIndex++
	}

	// Add check for non-deleted lists
	conditions = append(conditions, "l.deleted_at IS NULL")

	// Build the EXISTS clause
	whereClause := strings.Join(conditions, " AND ")
	existsClause := fmt.Sprintf(
		"EXISTS (SELECT 1 FROM contact_lists cl JOIN lists l ON cl.list_id = l.id WHERE cl.email = contacts.email AND %s)",
		whereClause,
	)

	// Handle NOT IN operator
	if contactList.Operator == "not_in" {
		existsClause = "NOT " + existsClause
	} else if contactList.Operator != "in" && contactList.Operator != "" {
		return "", nil, argIndex, fmt.Errorf("invalid contact_list operator: %s (must be 'in' or 'not_in')", contactList.Operator)
	}

	return existsClause, args, argIndex, nil
}

// parseContactTimelineConditions generates SQL for contact_timeline filtering
// Uses subquery to count timeline events matching criteria
func (qb *QueryBuilder) parseContactTimelineConditions(timeline *domain.ContactTimelineCondition, argIndex int) (string, []interface{}, int, error) {
	if timeline == nil {
		return "", nil, argIndex, fmt.Errorf("contact_timeline condition cannot be nil")
	}

	if timeline.Kind == "" {
		return "", nil, argIndex, fmt.Errorf("contact_timeline must have 'kind'")
	}

	if timeline.CountOperator == "" {
		return "", nil, argIndex, fmt.Errorf("contact_timeline must have 'count_operator'")
	}

	var args []interface{}
	var conditions []string

	// Base condition: event kind
	args = append(args, timeline.Kind)
	conditions = append(conditions, fmt.Sprintf("ct.kind = $%d", argIndex))
	argIndex++

	// Add timeframe conditions if specified
	if timeline.TimeframeOperator != nil && *timeline.TimeframeOperator != "" && *timeline.TimeframeOperator != "anytime" {
		timeCondition, timeArgs, newArgIndex, err := qb.parseTimeframeCondition(*timeline.TimeframeOperator, timeline.TimeframeValues, argIndex)
		if err != nil {
			return "", nil, argIndex, err
		}
		if timeCondition != "" {
			conditions = append(conditions, timeCondition)
			args = append(args, timeArgs...)
			argIndex = newArgIndex
		}
	}

	// Add dimension filters if specified
	if len(timeline.Filters) > 0 {
		for _, filter := range timeline.Filters {
			// Parse filter using existing logic, but prefix with "ct."
			filterCondition, filterArgs, newArgIndex, err := qb.parseTimelineFilter(filter, argIndex)
			if err != nil {
				return "", nil, argIndex, err
			}
			if filterCondition != "" {
				conditions = append(conditions, filterCondition)
				args = append(args, filterArgs...)
				argIndex = newArgIndex
			}
		}
	}

	// Build the subquery WHERE clause
	whereClause := strings.Join(conditions, " AND ")

	// Build the count comparison
	var countComparison string
	switch timeline.CountOperator {
	case "at_least":
		countComparison = ">="
	case "at_most":
		countComparison = "<="
	case "exactly":
		countComparison = "="
	default:
		return "", nil, argIndex, fmt.Errorf("invalid count_operator: %s (must be 'at_least', 'at_most', or 'exactly')", timeline.CountOperator)
	}

	args = append(args, timeline.CountValue)
	countCondition := fmt.Sprintf(
		"(SELECT COUNT(*) FROM contact_timeline ct WHERE ct.email = contacts.email AND %s) %s $%d",
		whereClause,
		countComparison,
		argIndex,
	)
	argIndex++

	return countCondition, args, argIndex, nil
}

// parseTimeframeCondition generates SQL for timeline timeframe filters
func (qb *QueryBuilder) parseTimeframeCondition(operator string, values []string, argIndex int) (string, []interface{}, int, error) {
	var args []interface{}

	switch operator {
	case "in_date_range":
		if len(values) != 2 {
			return "", nil, argIndex, fmt.Errorf("in_date_range requires 2 values (start and end)")
		}
		startTime, err := time.Parse(time.RFC3339, values[0])
		if err != nil {
			return "", nil, argIndex, fmt.Errorf("invalid start time: %w", err)
		}
		endTime, err := time.Parse(time.RFC3339, values[1])
		if err != nil {
			return "", nil, argIndex, fmt.Errorf("invalid end time: %w", err)
		}
		args = append(args, startTime, endTime)
		condition := fmt.Sprintf("ct.created_at BETWEEN $%d AND $%d", argIndex, argIndex+1)
		return condition, args, argIndex + 2, nil

	case "before_date":
		if len(values) != 1 {
			return "", nil, argIndex, fmt.Errorf("before_date requires 1 value")
		}
		t, err := time.Parse(time.RFC3339, values[0])
		if err != nil {
			return "", nil, argIndex, fmt.Errorf("invalid time: %w", err)
		}
		args = append(args, t)
		condition := fmt.Sprintf("ct.created_at < $%d", argIndex)
		return condition, args, argIndex + 1, nil

	case "after_date":
		if len(values) != 1 {
			return "", nil, argIndex, fmt.Errorf("after_date requires 1 value")
		}
		t, err := time.Parse(time.RFC3339, values[0])
		if err != nil {
			return "", nil, argIndex, fmt.Errorf("invalid time: %w", err)
		}
		args = append(args, t)
		condition := fmt.Sprintf("ct.created_at > $%d", argIndex)
		return condition, args, argIndex + 1, nil

	case "in_the_last_days":
		if len(values) != 1 {
			return "", nil, argIndex, fmt.Errorf("in_the_last_days requires 1 value (number of days)")
		}
		// Parse the number of days
		var days int
		_, err := fmt.Sscanf(values[0], "%d", &days)
		if err != nil {
			return "", nil, argIndex, fmt.Errorf("invalid days value: %w", err)
		}
		// Note: Not using parameterized query for interval as PostgreSQL doesn't support it directly
		// But the value is parsed as int so it's safe from SQL injection
		condition := fmt.Sprintf("ct.created_at > NOW() - INTERVAL '%d days'", days)
		return condition, args, argIndex, nil

	default:
		return "", nil, argIndex, fmt.Errorf("unsupported timeframe operator: %s", operator)
	}
}

// parseTimelineFilter parses a dimension filter for timeline events
func (qb *QueryBuilder) parseTimelineFilter(filter *domain.DimensionFilter, argIndex int) (string, []interface{}, int, error) {
	if filter == nil {
		return "", nil, argIndex, fmt.Errorf("filter cannot be nil")
	}

	// Timeline metadata is stored in JSONB, so we need to use JSON operators
	// For now, support common timeline metadata fields
	fieldPath := fmt.Sprintf("ct.metadata->>'%s'", filter.FieldName)

	// Validate operator
	sqlOp, ok := qb.allowedOperators[filter.Operator]
	if !ok {
		return "", nil, argIndex, fmt.Errorf("invalid operator: %s", filter.Operator)
	}

	// Handle operators that don't require values
	if !sqlOp.requiresValue {
		return fmt.Sprintf("%s %s", fieldPath, sqlOp.sql), nil, argIndex, nil
	}

	// Get values based on field type
	var values []interface{}
	var err error

	switch filter.FieldType {
	case "string":
		values, err = qb.getStringValues(filter)
	case "number":
		values, err = qb.getNumberValues(filter)
		// For number comparisons in JSONB, cast to numeric
		fieldPath = fmt.Sprintf("(%s)::numeric", fieldPath)
	case "time":
		values, err = qb.getTimeValues(filter)
		// For time comparisons in JSONB, cast to timestamp
		fieldPath = fmt.Sprintf("(%s)::timestamp", fieldPath)
	default:
		return "", nil, argIndex, fmt.Errorf("invalid field type: %s", filter.FieldType)
	}

	if err != nil {
		return "", nil, argIndex, err
	}

	if len(values) == 0 {
		return "", nil, argIndex, fmt.Errorf("filter must have values for operator %s", filter.Operator)
	}

	// Build SQL condition
	return qb.buildCondition(fieldPath, filter.Operator, sqlOp, values, argIndex)
}

// buildCondition builds the SQL condition with parameterized values
func (qb *QueryBuilder) buildCondition(dbColumn, operator string, sqlOp sqlOperator, values []interface{}, argIndex int) (string, []interface{}, int, error) {
	var args []interface{}

	switch operator {
	case "contains", "not_contains":
		// ILIKE requires % wildcards
		if len(values) != 1 {
			return "", nil, argIndex, fmt.Errorf("contains/not_contains requires exactly one value")
		}
		str, ok := values[0].(string)
		if !ok {
			return "", nil, argIndex, fmt.Errorf("contains/not_contains requires string value")
		}
		args = append(args, "%"+str+"%")
		condition := fmt.Sprintf("%s %s $%d", dbColumn, sqlOp.sql, argIndex)
		return condition, args, argIndex + 1, nil

	case "in_date_range", "not_in_date_range":
		// BETWEEN requires exactly 2 values
		if len(values) != 2 {
			return "", nil, argIndex, fmt.Errorf("%s requires exactly 2 values (start and end)", operator)
		}
		args = append(args, values[0], values[1])
		condition := fmt.Sprintf("%s %s $%d AND $%d", dbColumn, sqlOp.sql, argIndex, argIndex+1)
		return condition, args, argIndex + 2, nil

	default:
		// Standard comparison operators
		if len(values) != 1 {
			return "", nil, argIndex, fmt.Errorf("%s requires exactly one value", operator)
		}
		args = append(args, values[0])
		condition := fmt.Sprintf("%s %s $%d", dbColumn, sqlOp.sql, argIndex)
		return condition, args, argIndex + 1, nil
	}
}
