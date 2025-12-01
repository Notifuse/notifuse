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
	fieldType string // "string", "number", "time", "json"
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
		"created_at", "updated_at",
		"custom_datetime_1", "custom_datetime_2", "custom_datetime_3", "custom_datetime_4", "custom_datetime_5",
	}
	for _, field := range timeFields {
		qb.allowedFields[field] = fieldConfig{
			dbColumn:  field,
			fieldType: "time",
		}
	}

	// JSON fields (stored as JSONB in PostgreSQL)
	jsonFields := []string{
		"custom_json_1", "custom_json_2", "custom_json_3", "custom_json_4", "custom_json_5",
	}
	for _, field := range jsonFields {
		qb.allowedFields[field] = fieldConfig{
			dbColumn:  field,
			fieldType: "json",
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
		"in_the_last_days":  {sql: "", requiresValue: true}, // Special handling in buildCondition

		// JSON array operators
		"in_array": {sql: "?", requiresValue: true}, // JSONB array containment check
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

	switch leaf.Source {
	case "contacts":
		if leaf.Contact == nil {
			return "", nil, argIndex, fmt.Errorf("leaf with source 'contacts' must have 'contact' field")
		}
		return qb.parseContactConditions(leaf.Contact, argIndex)

	case "contact_lists":
		if leaf.ContactList == nil {
			return "", nil, argIndex, fmt.Errorf("leaf with source 'contact_lists' must have 'contact_list' field")
		}
		return qb.parseContactListConditions(leaf.ContactList, argIndex)

	case "contact_timeline":
		if leaf.ContactTimeline == nil {
			return "", nil, argIndex, fmt.Errorf("leaf with source 'contact_timeline' must have 'contact_timeline' field")
		}
		return qb.parseContactTimelineConditions(leaf.ContactTimeline, argIndex)

	case "custom_events_goals":
		if leaf.CustomEventsGoal == nil {
			return "", nil, argIndex, fmt.Errorf("leaf with source 'custom_events_goals' must have 'custom_events_goal' field")
		}
		return qb.parseCustomEventsGoalCondition(leaf.CustomEventsGoal, argIndex)

	default:
		return "", nil, argIndex, fmt.Errorf("unsupported source: %s (supported: 'contacts', 'contact_lists', 'contact_timeline', 'custom_events_goals')", leaf.Source)
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

	// Route JSON fields to specialized handler
	if fieldCfg.fieldType == "json" {
		return qb.buildJSONCondition(fieldCfg.dbColumn, filter, argIndex)
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

	// Special handling for in_the_last_days: treat as string (days count) not as date
	if filter.Operator == "in_the_last_days" {
		values, err = qb.getStringValues(filter)
	} else {
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

// parseCustomEventsGoalCondition generates SQL for custom_events goal-based filtering
// Uses EXISTS subquery with aggregation to check goal metrics (LTV, transaction count, etc.)
func (qb *QueryBuilder) parseCustomEventsGoalCondition(goal *domain.CustomEventsGoalCondition, argIndex int) (string, []interface{}, int, error) {
	if goal == nil {
		return "", nil, argIndex, fmt.Errorf("custom_events_goal condition cannot be nil")
	}

	var args []interface{}
	var conditions []string

	// Always exclude soft-deleted events
	conditions = append(conditions, "ce.deleted_at IS NULL")

	// Filter by goal_type if not "*" (wildcard for all)
	if goal.GoalType != "*" {
		// Validate goal_type against allowed values
		validGoalType := false
		for _, t := range domain.ValidGoalTypes {
			if goal.GoalType == t {
				validGoalType = true
				break
			}
		}
		if !validGoalType {
			return "", nil, argIndex, fmt.Errorf("invalid goal_type: %s (must be one of: %v or '*' for all)", goal.GoalType, domain.ValidGoalTypes)
		}

		args = append(args, goal.GoalType)
		conditions = append(conditions, fmt.Sprintf("ce.goal_type = $%d", argIndex))
		argIndex++
	} else {
		// For wildcard, just ensure goal_type is set
		conditions = append(conditions, "ce.goal_type IS NOT NULL")
	}

	// Filter by goal_name if provided
	if goal.GoalName != nil && *goal.GoalName != "" {
		args = append(args, *goal.GoalName)
		conditions = append(conditions, fmt.Sprintf("ce.goal_name = $%d", argIndex))
		argIndex++
	}

	// Add timeframe conditions
	if goal.TimeframeOperator != "" && goal.TimeframeOperator != "anytime" {
		timeCondition, timeArgs, newArgIndex, err := qb.parseGoalTimeframeCondition(goal.TimeframeOperator, goal.TimeframeValues, argIndex)
		if err != nil {
			return "", nil, argIndex, err
		}
		if timeCondition != "" {
			conditions = append(conditions, timeCondition)
			args = append(args, timeArgs...)
			argIndex = newArgIndex
		}
	}

	// Build aggregate expression
	var aggExpr string
	switch goal.AggregateOperator {
	case "sum":
		aggExpr = "COALESCE(SUM(ce.goal_value), 0)"
	case "count":
		aggExpr = "COUNT(*)"
	case "avg":
		aggExpr = "COALESCE(AVG(ce.goal_value), 0)"
	case "min":
		aggExpr = "MIN(ce.goal_value)"
	case "max":
		aggExpr = "MAX(ce.goal_value)"
	default:
		return "", nil, argIndex, fmt.Errorf("invalid aggregate_operator: %s", goal.AggregateOperator)
	}

	// Build comparison expression
	var comparison string
	switch goal.Operator {
	case "gte":
		args = append(args, goal.Value)
		comparison = fmt.Sprintf("%s >= $%d", aggExpr, argIndex)
		argIndex++
	case "lte":
		args = append(args, goal.Value)
		comparison = fmt.Sprintf("%s <= $%d", aggExpr, argIndex)
		argIndex++
	case "eq":
		args = append(args, goal.Value)
		comparison = fmt.Sprintf("%s = $%d", aggExpr, argIndex)
		argIndex++
	case "between":
		if goal.Value2 == nil {
			return "", nil, argIndex, fmt.Errorf("between operator requires value_2")
		}
		args = append(args, goal.Value, *goal.Value2)
		comparison = fmt.Sprintf("%s BETWEEN $%d AND $%d", aggExpr, argIndex, argIndex+1)
		argIndex += 2
	default:
		return "", nil, argIndex, fmt.Errorf("invalid operator: %s", goal.Operator)
	}

	// Build the EXISTS subquery with GROUP BY and HAVING
	whereClause := strings.Join(conditions, " AND ")
	existsClause := fmt.Sprintf(
		"EXISTS (SELECT 1 FROM custom_events ce WHERE ce.email = contacts.email AND %s GROUP BY ce.email HAVING %s)",
		whereClause,
		comparison,
	)

	return existsClause, args, argIndex, nil
}

// parseGoalTimeframeCondition generates SQL for goal timeframe filters
func (qb *QueryBuilder) parseGoalTimeframeCondition(operator string, values []string, argIndex int) (string, []interface{}, int, error) {
	var args []interface{}

	switch operator {
	case "in_date_range":
		if len(values) != 2 {
			return "", nil, argIndex, fmt.Errorf("in_date_range requires 2 values (start and end)")
		}
		startTime, err := time.Parse(time.RFC3339, values[0])
		if err != nil {
			startTime, err = time.Parse("2006-01-02", values[0])
			if err != nil {
				return "", nil, argIndex, fmt.Errorf("invalid start time: %w", err)
			}
		}
		endTime, err := time.Parse(time.RFC3339, values[1])
		if err != nil {
			endTime, err = time.Parse("2006-01-02", values[1])
			if err != nil {
				return "", nil, argIndex, fmt.Errorf("invalid end time: %w", err)
			}
		}
		args = append(args, startTime, endTime)
		condition := fmt.Sprintf("ce.occurred_at BETWEEN $%d AND $%d", argIndex, argIndex+1)
		return condition, args, argIndex + 2, nil

	case "before_date":
		if len(values) != 1 {
			return "", nil, argIndex, fmt.Errorf("before_date requires 1 value")
		}
		t, err := time.Parse(time.RFC3339, values[0])
		if err != nil {
			t, err = time.Parse("2006-01-02", values[0])
			if err != nil {
				return "", nil, argIndex, fmt.Errorf("invalid time: %w", err)
			}
		}
		args = append(args, t)
		condition := fmt.Sprintf("ce.occurred_at < $%d", argIndex)
		return condition, args, argIndex + 1, nil

	case "after_date":
		if len(values) != 1 {
			return "", nil, argIndex, fmt.Errorf("after_date requires 1 value")
		}
		t, err := time.Parse(time.RFC3339, values[0])
		if err != nil {
			t, err = time.Parse("2006-01-02", values[0])
			if err != nil {
				return "", nil, argIndex, fmt.Errorf("invalid time: %w", err)
			}
		}
		args = append(args, t)
		condition := fmt.Sprintf("ce.occurred_at > $%d", argIndex)
		return condition, args, argIndex + 1, nil

	case "in_the_last_days":
		if len(values) != 1 {
			return "", nil, argIndex, fmt.Errorf("in_the_last_days requires 1 value (number of days)")
		}
		var days int
		_, err := fmt.Sscanf(values[0], "%d", &days)
		if err != nil {
			return "", nil, argIndex, fmt.Errorf("invalid days value: %w", err)
		}
		// Safe from SQL injection: days is parsed as int
		condition := fmt.Sprintf("ce.occurred_at > NOW() - INTERVAL '%d days'", days)
		return condition, args, argIndex, nil

	default:
		return "", nil, argIndex, fmt.Errorf("unsupported goal timeframe operator: %s", operator)
	}
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
		// For time comparisons in JSONB, cast to timestamptz
		fieldPath = fmt.Sprintf("(%s)::timestamptz", fieldPath)
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
		if len(values) == 0 {
			return "", nil, argIndex, fmt.Errorf("contains/not_contains requires at least one value")
		}

		// Single value case - simpler SQL
		if len(values) == 1 {
			str, ok := values[0].(string)
			if !ok {
				return "", nil, argIndex, fmt.Errorf("contains/not_contains requires string value")
			}
			args = append(args, "%"+str+"%")
			condition := fmt.Sprintf("%s %s $%d", dbColumn, sqlOp.sql, argIndex)
			return condition, args, argIndex + 1, nil
		}

		// Multiple values case - generate OR conditions
		var orConditions []string
		for _, val := range values {
			str, ok := val.(string)
			if !ok {
				return "", nil, argIndex, fmt.Errorf("contains/not_contains requires string values")
			}
			args = append(args, "%"+str+"%")
			orConditions = append(orConditions, fmt.Sprintf("%s %s $%d", dbColumn, sqlOp.sql, argIndex))
			argIndex++
		}
		// Wrap multiple conditions in parentheses with OR
		condition := "(" + strings.Join(orConditions, " OR ") + ")"
		return condition, args, argIndex, nil

	case "in_date_range", "not_in_date_range":
		// BETWEEN requires exactly 2 values
		if len(values) != 2 {
			return "", nil, argIndex, fmt.Errorf("%s requires exactly 2 values (start and end)", operator)
		}
		args = append(args, values[0], values[1])
		condition := fmt.Sprintf("%s %s $%d AND $%d", dbColumn, sqlOp.sql, argIndex, argIndex+1)
		return condition, args, argIndex + 2, nil

	case "in_the_last_days":
		// Special handling for relative date filters
		if len(values) != 1 {
			return "", nil, argIndex, fmt.Errorf("in_the_last_days requires 1 value")
		}
		var days int
		switch v := values[0].(type) {
		case string:
			_, err := fmt.Sscanf(v, "%d", &days)
			if err != nil {
				return "", nil, argIndex, fmt.Errorf("invalid days value: %w", err)
			}
		case int:
			days = v
		case float64:
			days = int(v)
		default:
			return "", nil, argIndex, fmt.Errorf("invalid days value type")
		}
		// Note: Not using parameterized query for interval as PostgreSQL doesn't support it directly
		// But the value is parsed as int so it's safe from SQL injection
		condition := fmt.Sprintf("%s > NOW() - INTERVAL '%d days'", dbColumn, days)
		return condition, args, argIndex, nil

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

// buildJSONCondition builds SQL conditions for JSON/JSONB fields
// Uses PostgreSQL 17 subscript notation and proper type casting
func (qb *QueryBuilder) buildJSONCondition(dbColumn string, filter *domain.DimensionFilter, argIndex int) (string, []interface{}, int, error) {
	var args []interface{}

	// Validate operator
	sqlOp, ok := qb.allowedOperators[filter.Operator]
	if !ok {
		return "", nil, argIndex, fmt.Errorf("invalid operator for JSON field: %s", filter.Operator)
	}

	// Handle existence checks on the JSON field itself
	if filter.Operator == "is_set" || filter.Operator == "is_not_set" {
		if len(filter.JSONPath) == 0 {
			// Check if the entire JSON field is set/not set
			condition := fmt.Sprintf("%s %s", dbColumn, sqlOp.sql)
			return condition, nil, argIndex, nil
		}
		// Check if a specific key exists in the JSON
		// Use the ? operator for existence check
		key := filter.JSONPath[0]
		args = append(args, key)
		if filter.Operator == "is_set" {
			condition := fmt.Sprintf("%s ? $%d", dbColumn, argIndex)
			return condition, args, argIndex + 1, nil
		}
		condition := fmt.Sprintf("NOT (%s ? $%d)", dbColumn, argIndex)
		return condition, args, argIndex + 1, nil
	}

	// Build the JSON path using PostgreSQL subscript notation
	jsonPath := qb.buildJSONPath(dbColumn, filter.JSONPath)

	// Handle array-specific operators
	if filter.Operator == "in_array" {
		// Use JSONB ? operator for array containment
		if len(filter.StringValues) == 0 {
			return "", nil, argIndex, fmt.Errorf("in_array requires string_values")
		}
		args = append(args, filter.StringValues[0])
		condition := fmt.Sprintf("%s ? $%d", jsonPath, argIndex)
		return condition, args, argIndex + 1, nil
	}

	// For regular value comparisons, extract and cast the JSON value
	// Extract as text first, then cast to target type
	var fieldExpr string
	switch filter.FieldType {
	case "string", "json":
		// Extract as text
		fieldExpr = fmt.Sprintf("%s::text", jsonPath)
	case "number":
		// Extract as text, then cast to numeric
		fieldExpr = fmt.Sprintf("(%s::text)::numeric", jsonPath)
	case "time":
		// Extract as text, then cast to timestamptz
		fieldExpr = fmt.Sprintf("(%s::text)::timestamptz", jsonPath)
	default:
		return "", nil, argIndex, fmt.Errorf("invalid field_type for JSON field: %s", filter.FieldType)
	}

	// Get values based on the field type
	var values []interface{}
	var err error

	switch filter.FieldType {
	case "string", "json":
		values, err = qb.getStringValues(filter)
	case "number":
		values, err = qb.getNumberValues(filter)
	case "time":
		values, err = qb.getTimeValues(filter)
	default:
		return "", nil, argIndex, fmt.Errorf("unsupported field type for JSON: %s", filter.FieldType)
	}

	if err != nil {
		return "", nil, argIndex, err
	}

	if len(values) == 0 {
		return "", nil, argIndex, fmt.Errorf("filter must have values for operator %s", filter.Operator)
	}

	// Build the condition using standard operators
	switch filter.Operator {
	case "contains", "not_contains":
		// ILIKE for string containment
		if len(values) == 1 {
			str, ok := values[0].(string)
			if !ok {
				return "", nil, argIndex, fmt.Errorf("contains/not_contains requires string value")
			}
			args = append(args, "%"+str+"%")
			operator := "ILIKE"
			if filter.Operator == "not_contains" {
				operator = "NOT ILIKE"
			}
			condition := fmt.Sprintf("%s %s $%d", fieldExpr, operator, argIndex)
			return condition, args, argIndex + 1, nil
		}
		// Multiple values case
		var orConditions []string
		operator := "ILIKE"
		if filter.Operator == "not_contains" {
			operator = "NOT ILIKE"
		}
		for _, val := range values {
			str, ok := val.(string)
			if !ok {
				return "", nil, argIndex, fmt.Errorf("contains/not_contains requires string values")
			}
			args = append(args, "%"+str+"%")
			orConditions = append(orConditions, fmt.Sprintf("%s %s $%d", fieldExpr, operator, argIndex))
			argIndex++
		}
		condition := "(" + strings.Join(orConditions, " OR ") + ")"
		return condition, args, argIndex, nil

	default:
		// Standard comparison operators (equals, not_equals, gt, gte, lt, lte)
		if len(values) != 1 {
			return "", nil, argIndex, fmt.Errorf("%s requires exactly one value", filter.Operator)
		}
		args = append(args, values[0])
		condition := fmt.Sprintf("%s %s $%d", fieldExpr, sqlOp.sql, argIndex)
		return condition, args, argIndex + 1, nil
	}
}

// buildJSONPath constructs a PostgreSQL JSONB path expression using subscript notation
// Detects numeric strings and uses them as array indices
func (qb *QueryBuilder) buildJSONPath(dbColumn string, path []string) string {
	if len(path) == 0 {
		return dbColumn
	}

	result := dbColumn
	for _, segment := range path {
		// Check if segment is numeric (array index)
		if qb.isNumeric(segment) {
			// Use array subscript notation
			result = fmt.Sprintf("%s[%s]", result, segment)
		} else {
			// Use object key subscript notation
			// Escape single quotes in keys
			escapedSegment := strings.ReplaceAll(segment, "'", "''")
			result = fmt.Sprintf("%s['%s']", result, escapedSegment)
		}
	}
	return result
}

// isNumeric checks if a string represents a numeric value (for array indices)
func (qb *QueryBuilder) isNumeric(s string) bool {
	if len(s) == 0 {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
