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

// isRelativeDayOperator reports whether the operator compares against a rolling window whose
// value is a day count rather than a date. These only make sense against a timestamp, and their
// value must be read as a plain integer instead of being parsed as a date.
func isRelativeDayOperator(operator string) bool {
	return operator == "in_the_last_days" || operator == "not_in_the_last_days"
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
		// Matches rows outside the window, including rows where the date was never set:
		// "has not converted in the last 30 days" must cover contacts who never converted.
		"not_in_the_last_days": {sql: "", requiresValue: true}, // Special handling in buildCondition

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
		// Segments: a value that cannot be cast fails the query outright, which is a
		// person waiting on a report rather than a write path going down.
		condition, newArgs, newArgIndex, err := qb.parseFilter(filter, argIndex, castDirectly)
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
func (qb *QueryBuilder) parseFilter(filter *domain.DimensionFilter, argIndex int, mode castMode) (string, []interface{}, int, error) {
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
		return qb.buildJSONCondition(fieldCfg.dbColumn, filter, argIndex, mode)
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

	// Special handling for the relative-day operators: the value is a day count, not a date
	if isRelativeDayOperator(filter.Operator) {
		// Guard the combination rather than emitting SQL that only fails at execution: comparing
		// a text or numeric column against NOW() - INTERVAL errors out inside the segment build,
		// far from the request that defined the filter.
		if fieldCfg.fieldType != "time" {
			return "", nil, argIndex, fmt.Errorf("operator %s can only be used with a date field, not %s", filter.Operator, filter.FieldName)
		}
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

// contactsEmailRef is the email reference used when conditions are compiled against the contacts
// table itself (segment queries), as opposed to a trigger context where it is e.g. "NEW.email".
const contactsEmailRef = "contacts.email"

// parseContactListConditions generates SQL for contact_lists filtering
// Uses EXISTS subquery to check if contact is in specific list(s)
func (qb *QueryBuilder) parseContactListConditions(contactList *domain.ContactListCondition, argIndex int) (string, []interface{}, int, error) {
	return qb.parseContactListConditionsWithEmailRef(contactList, argIndex, contactsEmailRef)
}

// parseContactTimelineConditions generates SQL for contact_timeline filtering
// Uses subquery to count timeline events matching criteria
func (qb *QueryBuilder) parseContactTimelineConditions(timeline *domain.ContactTimelineCondition, argIndex int) (string, []interface{}, int, error) {
	return qb.parseContactTimelineConditionsWithEmailRef(timeline, argIndex, contactsEmailRef)
}

// parseCustomEventsGoalCondition generates SQL for custom_events goal-based filtering
// Uses EXISTS subquery with aggregation to check goal metrics (LTV, transaction count, etc.)
func (qb *QueryBuilder) parseCustomEventsGoalCondition(goal *domain.CustomEventsGoalCondition, argIndex int) (string, []interface{}, int, error) {
	return qb.parseCustomEventsGoalConditionWithEmailRef(goal, argIndex, contactsEmailRef)
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
	// Timeline event fields live in the contact_timeline.changes JSONB column, which the
	// database triggers populate as {field: {old, new}} (an insert only sets "new"). Read
	// the "new" value — the resulting value of the change. (There is no "metadata" column;
	// referencing one produced SQL that failed at execution.)
	// Cast defensively, not directly. That was safe while every value in `changes`
	// came from a database trigger reading a typed column — uniform per key, so a
	// cast could not meet an odd row. The web analytics projection broke that
	// premise: it is an application writer, and several of its keys (path,
	// landing_path, exit_path, referrer_domain, utm_*) hold text a visitor
	// supplied to a public endpoint. A numeric or date operator against one of
	// them — refused by the console, reachable through the API — would otherwise
	// compile to (…)::numeric over free text and abort the whole statement, and
	// whether the kind predicate filters the offending row first is plan-
	// dependent. A segment would work until the planner changed its mind, then
	// fail every recompute with its count frozen.
	return qb.parseJSONBKeyFilter(filter, argIndex, "ct.changes->%s->>'new'", castDefensively)
}

// parseEventPropertyFilter parses a dimension filter against the custom_events.properties payload.
// Property keys are arbitrary (whatever the caller sent with the event), so they are never part of
// the SQL text.
func (qb *QueryBuilder) parseEventPropertyFilter(filter *domain.DimensionFilter, argIndex int) (string, []interface{}, int, error) {
	return qb.parseJSONBKeyFilter(filter, argIndex, "ce.properties->>%s", castDefensively)
}

// castMode selects how a JSONB value is converted before it is compared.
type castMode bool

const (
	// castDirectly casts the extracted text straight to the target type.
	castDirectly castMode = false
	// castDefensively yields NULL for a value that cannot be converted, instead of letting the
	// cast abort the statement.
	//
	// custom_events.properties is an arbitrary caller payload: nothing constrains a key to hold
	// the same type across events, so one event carrying {"renewed_at": "soon"} makes
	// ('soon')::timestamptz kill the whole segment query. Worse, whether it does is
	// plan-dependent — the identical filter returns rows while the odd row stays outside the
	// scanned set and starts failing when it does not, so in production it surfaces long after
	// the event was written and gets blamed on whatever changed most recently.
	castDefensively castMode = true
)

// Permissive enough to accept everything the direct cast would have: a JSON number and a
// numeric string both reach here as text, and PostgreSQL tolerates surrounding whitespace.
const numericTextPattern = `^\s*[-+]?(\d+(\.\d*)?|\.\d+)([eE][-+]?\d+)?\s*$`

// Dates live in JSON as strings, so only the ISO-8601 shape is recognised — which is what the
// console tells the user to store and the only form that is unambiguous across locales.
const timestampTextPattern = `^\s*\d{4}-\d{2}-\d{2}`

// castNumeric renders the numeric conversion of a text expression under the given mode.
func castNumeric(textExpr string, mode castMode) string {
	if mode == castDefensively {
		return fmt.Sprintf("CASE WHEN %s ~ '%s' THEN (%s)::numeric END", textExpr, numericTextPattern, textExpr)
	}
	return fmt.Sprintf("(%s)::numeric", textExpr)
}

// guardWholeColumnCast keeps a scalar stored in a custom_json column out of a numeric or date
// comparison that addresses the column itself rather than a value inside it.
//
// Those five columns used to hold nothing but an object or an array — contacts.upsert answered 400
// for anything else — so (custom_json_1::text)::numeric raised on every non-null row and such a
// filter failed loudly and always. A scalar is now stored as-is, matching what lists.subscribe had
// always accepted, and PostgreSQL converts one without complaint: ('42'::jsonb)::text::numeric is
// 42, and ('"2024-01-01"'::jsonb)::text::timestamptz is a real timestamp. The filter therefore
// started matching some rows and raising on others, and which of the two a customer saw depended on
// the rows the planner happened to reach first.
//
// Only the whole-column form needs this. With a path, the subscript already answers NULL on a
// scalar, and a typeof guard there would break an array index such as custom_json_1[0].
//
// CASE rather than a conjunct: PostgreSQL does not promise to evaluate AND left to right, so
// `jsonb_typeof(col) = 'object' AND (col::text)::numeric > $1` can still be reordered behind the
// cast it is meant to protect. An object keeps raising, which is what the segment path chose for
// values it cannot convert.
func guardWholeColumnCast(castExpr string, dbColumn string, hasJSONPath bool) string {
	if hasJSONPath {
		return castExpr
	}
	return fmt.Sprintf("CASE WHEN jsonb_typeof(%s) = 'object' THEN %s END", dbColumn, castExpr)
}

// castTimestamp renders the timestamp conversion of a text expression under the given mode.
func castTimestamp(textExpr string, mode castMode) string {
	if mode == castDefensively {
		return fmt.Sprintf("CASE WHEN %s ~ '%s' THEN (%s)::timestamptz END", textExpr, timestampTextPattern, textExpr)
	}
	return fmt.Sprintf("(%s)::timestamptz", textExpr)
}

// parseJSONBKeyFilter builds a condition on a JSONB value addressed by a caller-supplied key.
//
// pathTemplate must contain exactly one %s verb, which receives the *placeholder* for the key —
// never the key itself. Field names here are free-form (event property keys, timeline change
// keys), so they cannot be whitelisted the way contact columns are; interpolating one into the
// SQL text let a crafted field_name close the quote and append arbitrary SQL, which the segment
// preview count then leaked one boolean at a time.
func (qb *QueryBuilder) parseJSONBKeyFilter(filter *domain.DimensionFilter, argIndex int, pathTemplate string, mode castMode) (string, []interface{}, int, error) {
	if filter == nil {
		return "", nil, argIndex, fmt.Errorf("filter cannot be nil")
	}

	if filter.FieldName == "" {
		return "", nil, argIndex, fmt.Errorf("filter must have 'field_name'")
	}

	// Validate operator
	sqlOp, ok := qb.allowedOperators[filter.Operator]
	if !ok {
		return "", nil, argIndex, fmt.Errorf("invalid operator: %s", filter.Operator)
	}

	// The key is bound as an argument, so it must be appended before any value arguments to keep
	// the placeholder numbering sequential.
	args := []interface{}{filter.FieldName}
	fieldPath := fmt.Sprintf(pathTemplate, fmt.Sprintf("$%d", argIndex))
	argIndex++

	// Handle operators that don't require values
	if !sqlOp.requiresValue {
		return fmt.Sprintf("%s %s", fieldPath, sqlOp.sql), args, argIndex, nil
	}

	// Relative-day operators compare a timestamp against a rolling window, so their value is a
	// day count and the key has to be read as a date whatever else it could be cast to.
	if isRelativeDayOperator(filter.Operator) {
		if filter.FieldType != "time" {
			return "", nil, argIndex, fmt.Errorf("operator %s can only be used with a date value, not %s", filter.Operator, filter.FieldType)
		}
		values, err := qb.getStringValues(filter)
		if err != nil {
			return "", nil, argIndex, err
		}
		condition, valueArgs, newArgIndex, err := qb.buildCondition(
			castTimestamp(fieldPath, mode), filter.Operator, sqlOp, values, argIndex)
		if err != nil {
			return "", nil, argIndex, err
		}
		return condition, append(args, valueArgs...), newArgIndex, nil
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
		fieldPath = castNumeric(fieldPath, mode)
	case "time":
		values, err = qb.getTimeValues(filter)
		// For time comparisons in JSONB, cast to timestamptz
		fieldPath = castTimestamp(fieldPath, mode)
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
	condition, valueArgs, newArgIndex, err := qb.buildCondition(fieldPath, filter.Operator, sqlOp, values, argIndex)
	if err != nil {
		return "", nil, argIndex, err
	}

	return condition, append(args, valueArgs...), newArgIndex, nil
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

	case "in_the_last_days", "not_in_the_last_days":
		// Special handling for relative date filters
		if len(values) != 1 {
			return "", nil, argIndex, fmt.Errorf("%s requires 1 value", operator)
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
		if operator == "not_in_the_last_days" {
			// NULL-inclusive on purpose: a contact whose date was never set has not done the thing
			// in the last N days either, and a plain NOT (col > ...) would silently drop them.
			condition := fmt.Sprintf("(%s IS NULL OR %s <= NOW() - INTERVAL '%d days')", dbColumn, dbColumn, days)
			return condition, args, argIndex, nil
		}
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
func (qb *QueryBuilder) buildJSONCondition(dbColumn string, filter *domain.DimensionFilter, argIndex int, mode castMode) (string, []interface{}, int, error) {
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
		// A key belongs to an object, so require one before looking the key up.
		//
		// jsonb's ? operator does not require that: it matches a top-level scalar string as
		// readily as an object key, so on PostgreSQL 17 '"gold"'::jsonb ? 'gold' is true. That
		// was unreachable while contacts.upsert refused anything but an object or an array in
		// these five columns, and stopped being unreachable when the endpoint widened to accept
		// a scalar (matching what lists.subscribe had always stored). Unguarded, importing
		// contacts whose custom_json_1 is the bare string "tier" drops every one of them into
		// any segment carrying `custom_json_1.tier is_set`, though they hold no such key and no
		// object at all — and the only symptom is the wrong recipients on the next broadcast.
		//
		// A plain conjunct is enough here because ? cannot raise, so it does not matter whether
		// the planner reaches it before jsonb_typeof. The casts further down need a CASE.
		key := filter.JSONPath[0]
		args = append(args, key)
		condition := fmt.Sprintf("(jsonb_typeof(%s) = 'object' AND %s ? $%d)", dbColumn, dbColumn, argIndex)
		if filter.Operator == "is_not_set" {
			condition = "NOT " + condition
		}
		return condition, args, argIndex + 1, nil
	}

	// Build the JSON path using PostgreSQL subscript notation
	jsonPath := qb.buildJSONPath(dbColumn, filter.JSONPath)
	// An empty path addresses the column itself, which is the one shape a stored scalar can reach
	// through a cast. See guardWholeColumnCast.
	hasJSONPath := len(filter.JSONPath) > 0

	// Relative-day operators need the same treatment as elsewhere: a day count for a value and a
	// timestamp to compare it against. They carry no SQL operator of their own, so falling
	// through to the generic comparison below emitted "<expr>  $1" — a syntax error that only
	// surfaced when the segment ran.
	if isRelativeDayOperator(filter.Operator) {
		if filter.FieldType != "time" {
			return "", nil, argIndex, fmt.Errorf("operator %s can only be used with a date value, not %s", filter.Operator, filter.FieldType)
		}
		values, err := qb.getStringValues(filter)
		if err != nil {
			return "", nil, argIndex, err
		}
		return qb.buildCondition(
			guardWholeColumnCast(castTimestamp(fmt.Sprintf("%s::text", jsonPath), mode), dbColumn, hasJSONPath),
			filter.Operator, sqlOp, values, argIndex)
	}

	// Handle array-specific operators
	if filter.Operator == "in_array" {
		// Use JSONB ? operator for array containment
		if len(filter.StringValues) == 0 {
			return "", nil, argIndex, fmt.Errorf("in_array requires string_values")
		}
		args = append(args, filter.StringValues[0])
		// The same ? quirk as the existence check above, reached through a different operator: a
		// stored scalar string satisfies `in_array "gold"` simply by being "gold".
		//
		// This guard keeps objects as well as arrays, unlike the one above, because is_set reads
		// only JSONPath[0] and so can only ask about a top-level key. A nested key can be asked
		// about through in_array alone, and that has worked since long before a scalar could be
		// stored here, so narrowing to 'array' would take a capability away rather than close a
		// hole the widening opened.
		condition := fmt.Sprintf("(jsonb_typeof(%s) IN ('object', 'array') AND %s ? $%d)", jsonPath, jsonPath, argIndex)
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
		fieldExpr = guardWholeColumnCast(castNumeric(fmt.Sprintf("%s::text", jsonPath), mode), dbColumn, hasJSONPath)
	case "time":
		// Extract as text, then cast to timestamptz
		fieldExpr = guardWholeColumnCast(castTimestamp(fmt.Sprintf("%s::text", jsonPath), mode), dbColumn, hasJSONPath)
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
//
// Unlike the free-form keys in parseJSONBKeyFilter, these segments stay in the SQL text and are
// defended by quote doubling. That is safe — a crafted segment collapses into an inert key lookup
// rather than escaping the subscript — but it is the same shape as the interpolation that did turn
// out to be exploitable, so treat any change here as security-sensitive. Binding them instead is
// not a like-for-like swap: subscript notation takes a literal, and switching to the -> operator
// chain would change how array indices and missing keys behave.
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

// BuildTriggerCondition generates SQL for use in PostgreSQL trigger WHEN clauses.
// Unlike BuildSQL which returns "SELECT email FROM contacts WHERE ...",
// this returns an EXISTS subquery format with the email reference substituted.
//
// Parameters:
//   - tree: The TreeNode conditions to translate
//   - emailRef: The email reference in trigger context (e.g., "NEW.email")
//
// Returns SQL condition string and args (args will be embedded by caller using embedArgs)
func (qb *QueryBuilder) BuildTriggerCondition(tree *domain.TreeNode, emailRef string) (string, []interface{}, error) {
	if tree == nil {
		return "", nil, nil // No conditions = no filter
	}

	// Validate the tree structure
	if err := tree.Validate(); err != nil {
		return "", nil, fmt.Errorf("invalid tree: %w", err)
	}

	// Parse the tree recursively with email reference
	condition, args, _, err := qb.parseNodeWithEmailRef(tree, 1, emailRef)
	if err != nil {
		return "", nil, err
	}

	return condition, args, nil
}

// parseNodeWithEmailRef recursively parses a tree node with custom email reference
func (qb *QueryBuilder) parseNodeWithEmailRef(node *domain.TreeNode, argIndex int, emailRef string) (string, []interface{}, int, error) {
	switch node.Kind {
	case "branch":
		return qb.parseBranchWithEmailRef(node.Branch, argIndex, emailRef)
	case "leaf":
		return qb.parseLeafWithEmailRef(node.Leaf, argIndex, emailRef)
	default:
		return "", nil, argIndex, fmt.Errorf("invalid node kind: %s", node.Kind)
	}
}

// parseBranchWithEmailRef parses a branch node with custom email reference
func (qb *QueryBuilder) parseBranchWithEmailRef(branch *domain.TreeNodeBranch, argIndex int, emailRef string) (string, []interface{}, int, error) {
	if branch == nil {
		return "", nil, argIndex, fmt.Errorf("branch cannot be nil")
	}

	var conditions []string
	var args []interface{}

	for _, leaf := range branch.Leaves {
		condition, newArgs, newArgIndex, err := qb.parseNodeWithEmailRef(leaf, argIndex, emailRef)
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

// parseLeafWithEmailRef parses a leaf node with custom email reference
func (qb *QueryBuilder) parseLeafWithEmailRef(leaf *domain.TreeNodeLeaf, argIndex int, emailRef string) (string, []interface{}, int, error) {
	if leaf == nil {
		return "", nil, argIndex, fmt.Errorf("leaf cannot be nil")
	}

	switch leaf.Source {
	case "contacts":
		if leaf.Contact == nil {
			return "", nil, argIndex, fmt.Errorf("leaf with source 'contacts' must have 'contact' field")
		}
		return qb.parseContactConditionsForTrigger(leaf.Contact, argIndex, emailRef)

	case "contact_lists":
		if leaf.ContactList == nil {
			return "", nil, argIndex, fmt.Errorf("leaf with source 'contact_lists' must have 'contact_list' field")
		}
		return qb.parseContactListConditionsWithEmailRef(leaf.ContactList, argIndex, emailRef)

	case "contact_timeline":
		if leaf.ContactTimeline == nil {
			return "", nil, argIndex, fmt.Errorf("leaf with source 'contact_timeline' must have 'contact_timeline' field")
		}
		return qb.parseContactTimelineConditionsWithEmailRef(leaf.ContactTimeline, argIndex, emailRef)

	case "custom_events_goals":
		if leaf.CustomEventsGoal == nil {
			return "", nil, argIndex, fmt.Errorf("leaf with source 'custom_events_goals' must have 'custom_events_goal' field")
		}
		return qb.parseCustomEventsGoalConditionWithEmailRef(leaf.CustomEventsGoal, argIndex, emailRef)

	default:
		return "", nil, argIndex, fmt.Errorf("unsupported source: %s", leaf.Source)
	}
}

// parseContactConditionsForTrigger generates an EXISTS subquery for contact conditions in trigger context
func (qb *QueryBuilder) parseContactConditionsForTrigger(contact *domain.ContactCondition, argIndex int, emailRef string) (string, []interface{}, int, error) {
	if contact == nil {
		return "", nil, argIndex, fmt.Errorf("contact condition cannot be nil")
	}

	var conditions []string
	var args []interface{}

	for _, filter := range contact.Filters {
		// Triggers: cast defensively. This expression runs inside a trigger function on
		// contact_timeline, which is written by triggers on contacts, contact_lists,
		// message_history, custom_events, contact_segments and inbound_webhook_events — so a
		// cast that raises takes all of those writes down for the contact concerned, and the
		// install probe cannot foresee it because EXPLAIN plans without executing. A contact
		// whose custom_json value does not convert simply does not match.
		condition, newArgs, newArgIndex, err := qb.parseFilter(filter, argIndex, castDefensively)
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
		// No conditions, just check contact exists
		existsClause := fmt.Sprintf("EXISTS (SELECT 1 FROM contacts WHERE email = %s)", emailRef)
		return existsClause, args, argIndex, nil
	}

	// Contact conditions are ANDed together
	whereClause := strings.Join(conditions, " AND ")
	existsClause := fmt.Sprintf("EXISTS (SELECT 1 FROM contacts WHERE email = %s AND %s)", emailRef, whereClause)
	return existsClause, args, argIndex, nil
}

// parseContactListConditionsWithEmailRef generates SQL for contact_lists filtering with custom email reference
func (qb *QueryBuilder) parseContactListConditionsWithEmailRef(contactList *domain.ContactListCondition, argIndex int, emailRef string) (string, []interface{}, int, error) {
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

	// Build the EXISTS clause with custom email reference
	whereClause := strings.Join(conditions, " AND ")
	existsClause := fmt.Sprintf(
		"EXISTS (SELECT 1 FROM contact_lists cl JOIN lists l ON cl.list_id = l.id WHERE cl.email = %s AND %s)",
		emailRef,
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

// parseContactTimelineConditionsWithEmailRef generates SQL for contact_timeline filtering with custom email reference
func (qb *QueryBuilder) parseContactTimelineConditionsWithEmailRef(timeline *domain.ContactTimelineCondition, argIndex int, emailRef string) (string, []interface{}, int, error) {
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

	// Scope to specific sent messages via message_history when template/broadcast/link
	// filters are set. All are ANDed into one subquery on the message the timeline row
	// points to (ct.entity_id = message_history.id). link_url does a case-insensitive
	// substring match against the clicked destination URLs stored as keys of the
	// clicked_links JSONB (populated for click events only).
	var mhConds []string
	if timeline.TemplateID != nil && *timeline.TemplateID != "" {
		args = append(args, *timeline.TemplateID)
		mhConds = append(mhConds, fmt.Sprintf("template_id = $%d", argIndex))
		argIndex++
	}
	if timeline.BroadcastID != nil && *timeline.BroadcastID != "" {
		args = append(args, *timeline.BroadcastID)
		mhConds = append(mhConds, fmt.Sprintf("broadcast_id = $%d", argIndex))
		argIndex++
	}
	if timeline.LinkURL != nil && *timeline.LinkURL != "" {
		// Literal, case-insensitive substring match via strpos (NOT ILIKE): URLs routinely
		// contain '%' (percent-encoding) and '_', which ILIKE would treat as wildcards.
		// jsonb_object_keys errors on non-object values, so coerce a malformed clicked_links
		// to an empty object first (same planner-independent guard the click-writer uses).
		args = append(args, *timeline.LinkURL)
		mhConds = append(mhConds, fmt.Sprintf(
			"EXISTS (SELECT 1 FROM jsonb_object_keys(CASE WHEN jsonb_typeof(clicked_links) = 'object' THEN clicked_links ELSE '{}'::jsonb END) AS k WHERE strpos(lower(k), lower($%d)) > 0)", argIndex))
		argIndex++
	}
	if len(mhConds) > 0 {
		conditions = append(conditions, fmt.Sprintf(
			"ct.entity_id IN (SELECT id FROM message_history WHERE %s)", strings.Join(mhConds, " AND ")))
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
	// Use custom email reference instead of contacts.email
	countCondition := fmt.Sprintf(
		"(SELECT COUNT(*) FROM contact_timeline ct WHERE ct.email = %s AND %s) %s $%d",
		emailRef,
		whereClause,
		countComparison,
		argIndex,
	)
	argIndex++

	return countCondition, args, argIndex, nil
}

// parseCustomEventsGoalConditionWithEmailRef generates SQL for custom_events goal filtering with custom email reference
func (qb *QueryBuilder) parseCustomEventsGoalConditionWithEmailRef(goal *domain.CustomEventsGoalCondition, argIndex int, emailRef string) (string, []interface{}, int, error) {
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
		// Wildcard: the row must carry a type. Deliberate, not lazy.
		//
		// Untyped rows are made matchable by stamping goal_type at WRITE time
		// (web_analytics_contact_bridge.go), never by relaxing this predicate.
		// Relaxing it would change SQL already frozen in segments.generated_sql and
		// in installed automation trigger functions, so it needs a recompile
		// migration (v36 and v37 are the precedent) — and it would bypass the
		// partial index at database/init.go:367, which is WHERE goal_type IS NOT NULL.
		conditions = append(conditions, "ce.goal_type IS NOT NULL")
	}

	// Filter by goal_name if provided
	if goal.GoalName != nil && *goal.GoalName != "" {
		args = append(args, *goal.GoalName)
		conditions = append(conditions, fmt.Sprintf("ce.goal_name = $%d", argIndex))
		argIndex++
	}

	// Filter by the custom event name if provided
	if goal.EventName != nil && *goal.EventName != "" {
		args = append(args, *goal.EventName)
		conditions = append(conditions, fmt.Sprintf("ce.event_name = $%d", argIndex))
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

	// Narrow the matched events by their properties payload
	for _, filter := range goal.Filters {
		filterCondition, filterArgs, newArgIndex, err := qb.parseEventPropertyFilter(filter, argIndex)
		if err != nil {
			return "", nil, argIndex, err
		}
		if filterCondition != "" {
			conditions = append(conditions, filterCondition)
			args = append(args, filterArgs...)
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

	// Build the EXISTS subquery with GROUP BY and HAVING, using custom email reference
	whereClause := strings.Join(conditions, " AND ")
	existsClause := fmt.Sprintf(
		"EXISTS (SELECT 1 FROM custom_events ce WHERE ce.email = %s AND %s GROUP BY ce.email HAVING %s)",
		emailRef,
		whereClause,
		comparison,
	)

	// Negation has to wrap the whole leaf rather than invert the comparison: the subquery groups
	// by email, so a contact with no matching events produces no group and fails the EXISTS
	// whatever the HAVING says. NOT EXISTS is the only form that also matches those contacts —
	// which is exactly who "has not purchased in the last 30 days" is about.
	if goal.Negate {
		existsClause = "NOT " + existsClause
	}

	return existsClause, args, argIndex, nil
}
