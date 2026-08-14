package service

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
)

// AllowedContactFields defines valid field names for updated_fields filter (prevents SQL injection)
var AllowedContactFields = map[string]bool{
	// Core fields
	"external_id": true, "timezone": true, "language": true,
	"first_name": true, "last_name": true, "phone": true,
	// Address fields
	"address_line_1": true, "address_line_2": true,
	"country": true, "postcode": true, "state": true,
	// Custom string fields
	"custom_string_1": true, "custom_string_2": true, "custom_string_3": true,
	"custom_string_4": true, "custom_string_5": true,
	// Custom number fields
	"custom_number_1": true, "custom_number_2": true, "custom_number_3": true,
	"custom_number_4": true, "custom_number_5": true,
	// Custom datetime fields
	"custom_datetime_1": true, "custom_datetime_2": true, "custom_datetime_3": true,
	"custom_datetime_4": true, "custom_datetime_5": true,
	// Custom JSON fields
	"custom_json_1": true, "custom_json_2": true, "custom_json_3": true,
	"custom_json_4": true, "custom_json_5": true,
}

// identifierPattern accepts what may be concatenated into a PostgreSQL identifier.
// Automation ids in practice are uuids or shortuuids; this rejects everything else
// before it reaches DDL.
var identifierPattern = regexp.MustCompile(`^[A-Za-z0-9_]+$`)

// AutomationTriggerName builds the trigger and function identifier for an automation, or
// reports why the id cannot be one.
//
// Every path that emits automation trigger DDL must go through here. The name is
// interpolated as a bare identifier, and lib/pq sends these statements with no arguments —
// over the simple query protocol, which executes every statement in the string. Not every
// caller has an id that has been through Automation.Validate either: automations.delete
// validates only that the id is non-empty and never loads the row, so its id is unbounded
// and entirely caller-supplied.
func AutomationTriggerName(automationID string) (string, error) {
	// Hyphens are stripped so a uuid becomes a legal identifier.
	safeID := strings.ReplaceAll(automationID, "-", "")
	if !identifierPattern.MatchString(safeID) {
		return "", fmt.Errorf("automation id %q cannot be used in a trigger name: only letters, digits, underscores and hyphens are allowed", automationID)
	}
	return "automation_trigger_" + safeID, nil
}

// TriggerSQL contains the generated SQL statements for an automation trigger
type TriggerSQL struct {
	FunctionName string // automation_trigger_{id}
	FunctionBody string // CREATE OR REPLACE FUNCTION ...
	TriggerName  string // automation_trigger_{id}
	TriggerDDL   string // CREATE TRIGGER ... WHEN (...) EXECUTE FUNCTION ...
	DropTrigger  string // DROP TRIGGER IF EXISTS ... ON contact_timeline
	DropFunction string // DROP FUNCTION IF EXISTS ...
	WHENClause   string // The WHEN clause alone (for storage/debugging) - values embedded

	// ConditionGuard is the compiled trigger.Conditions expression, empty when the
	// automation has none. It lives in the function body rather than the WHEN clause
	// because it compiles to correlated subqueries and PostgreSQL rejects any subquery
	// in a trigger WHEN at parse time (SQLSTATE 0A000, transformSubLink).
	ConditionGuard string

	// ValidationQuery is the same expression compiled against a literal email, wrapped
	// so it can be planned before the function is installed. CREATE FUNCTION only
	// syntax-checks a plpgsql body — it does not resolve column names — so without this
	// probe a condition naming a column that does not exist installs cleanly and then
	// aborts every write to contact_timeline. Empty when there is no guard.
	ValidationQuery string
}

// AutomationTriggerGenerator generates PostgreSQL trigger SQL from automation configuration
type AutomationTriggerGenerator struct {
	queryBuilder *QueryBuilder
}

// NewAutomationTriggerGenerator creates a new trigger generator
func NewAutomationTriggerGenerator(queryBuilder *QueryBuilder) *AutomationTriggerGenerator {
	return &AutomationTriggerGenerator{
		queryBuilder: queryBuilder,
	}
}

// Generate creates TriggerSQL for the given automation
func (g *AutomationTriggerGenerator) Generate(automation *domain.Automation) (*TriggerSQL, error) {
	if automation == nil {
		return nil, fmt.Errorf("automation is nil")
	}
	if automation.Trigger == nil {
		return nil, fmt.Errorf("automation trigger config is nil")
	}
	if automation.Trigger.EventKind == "" {
		return nil, fmt.Errorf("automation must have an event kind")
	}
	if automation.RootNodeID == "" {
		return nil, fmt.Errorf("automation must have a root node ID")
	}

	// Build WHEN clause (values already embedded, no args returned)
	whenClause, err := g.buildWHENClause(automation)
	if err != nil {
		return nil, fmt.Errorf("failed to build WHEN clause: %w", err)
	}

	// Build the conditions guard that goes inside the function body
	guard, validationQuery, err := g.buildConditionGuard(automation)
	if err != nil {
		return nil, err
	}

	// An automation whose id is refused here could never have had a working trigger:
	// the DDL would not parse.
	triggerName, err := AutomationTriggerName(automation.ID)
	if err != nil {
		return nil, err
	}
	functionName := triggerName

	// Build function body
	functionBody := g.buildFunctionBody(functionName, automation, guard)

	// Build trigger DDL
	triggerDDL := g.buildTriggerDDL(triggerName, functionName, whenClause)

	return &TriggerSQL{
		FunctionName:    functionName,
		FunctionBody:    functionBody,
		TriggerName:     triggerName,
		TriggerDDL:      triggerDDL,
		DropTrigger:     fmt.Sprintf("DROP TRIGGER IF EXISTS %s ON contact_timeline", triggerName),
		DropFunction:    fmt.Sprintf("DROP FUNCTION IF EXISTS %s()", functionName),
		WHENClause:      whenClause,
		ConditionGuard:  guard,
		ValidationQuery: validationQuery,
	}, nil
}

// buildWHENClause builds the WHEN clause for the trigger
func (g *AutomationTriggerGenerator) buildWHENClause(automation *domain.Automation) (string, error) {
	var conditions []string
	trigger := automation.Trigger

	// 1. Event kind filter (required)
	// For custom_event, the kind is "custom_event.{name}" in the timeline
	if trigger.EventKind == "custom_event" && trigger.CustomEventName != nil && *trigger.CustomEventName != "" {
		// Custom event with specific name filter
		conditions = append(conditions, fmt.Sprintf("NEW.kind = %s", sqlLiteral("custom_event."+*trigger.CustomEventName)))
	} else {
		// Every event kind (contact.*, list.*, segment.*, email.*) equals the
		// contact_timeline.kind written by the DB triggers, so match verbatim.
		// (email.sent / email.delivered are not valid automation kinds — see
		// domain.ValidEventKinds — so a WHEN clause on them would never match a row.)
		conditions = append(conditions, fmt.Sprintf("NEW.kind = %s", sqlLiteral(trigger.EventKind)))
	}

	// 2. List ID filter (for list.* events) - entity_id stores list_id
	if trigger.ListID != nil && *trigger.ListID != "" && strings.HasPrefix(trigger.EventKind, "list.") {
		conditions = append(conditions, fmt.Sprintf("NEW.entity_id = %s", sqlLiteral(*trigger.ListID)))
	}

	// 3. Segment ID filter (for segment.* events) - entity_id stores segment_id
	if trigger.SegmentID != nil && *trigger.SegmentID != "" && strings.HasPrefix(trigger.EventKind, "segment.") {
		conditions = append(conditions, fmt.Sprintf("NEW.entity_id = %s", sqlLiteral(*trigger.SegmentID)))
	}

	// 4. Updated fields filter (for contact.updated events) - checks if specific fields were changed
	if trigger.EventKind == "contact.updated" && len(trigger.UpdatedFields) > 0 {
		fieldChecks := make([]string, 0, len(trigger.UpdatedFields))
		for _, field := range trigger.UpdatedFields {
			if !AllowedContactFields[field] {
				return "", fmt.Errorf("invalid updated_field: %s", field)
			}
			// Use JSONB ? operator to check if field exists in changes
			fieldChecks = append(fieldChecks, fmt.Sprintf("NEW.changes ? %s", sqlLiteral(field)))
		}
		if len(fieldChecks) > 0 {
			conditions = append(conditions, "("+strings.Join(fieldChecks, " OR ")+")")
		}
	}

	// trigger.Conditions is deliberately absent here. Every leaf the condition
	// compiler supports produces a subquery — EXISTS, NOT EXISTS, or a scalar
	// (SELECT COUNT(*) ...) comparison — and PostgreSQL rejects any subquery in a
	// trigger WHEN at parse time. Conditions are evaluated in the function body
	// instead; see buildConditionGuard.

	// Combine with AND
	return strings.Join(conditions, " AND "), nil
}

// buildConditionGuard compiles trigger.Conditions into a boolean expression for use
// inside the trigger function body, plus a query that plans the same expression so a
// bad column reference is caught at install time rather than on the next contact write.
// Both are empty when the automation has no conditions.
func (g *AutomationTriggerGenerator) buildConditionGuard(automation *domain.Automation) (string, string, error) {
	conditions := automation.Trigger.Conditions
	if conditions == nil {
		return "", "", nil
	}

	guard, err := g.compileConditions(conditions, "NEW.email")
	if err != nil {
		return "", "", err
	}
	if guard == "" {
		return "", "", nil
	}

	// Same tree, but with the contact's email replaced by a literal so the expression
	// can be planned outside a trigger. EXPLAIN resolves every column without reading
	// a single row.
	probe, err := g.compileConditions(conditions, "''::text")
	if err != nil {
		return "", "", err
	}

	return guard, fmt.Sprintf("EXPLAIN SELECT (%s)", probe), nil
}

// compileConditions runs the shared segment compiler and embeds its arguments, which
// neither a trigger WHEN clause nor a plpgsql body can supply as parameters.
func (g *AutomationTriggerGenerator) compileConditions(conditions *domain.TreeNode, emailRef string) (string, error) {
	conditionSQL, args, err := g.queryBuilder.BuildTriggerCondition(conditions, emailRef)
	if err != nil {
		return "", fmt.Errorf("failed to build TreeNode conditions: %w", err)
	}
	if conditionSQL == "" {
		return "", nil
	}

	embeddedSQL, err := embedArgs(conditionSQL, args)
	if err != nil {
		return "", fmt.Errorf("failed to embed args: %w", err)
	}
	return embeddedSQL, nil
}

// buildFunctionBody generates the function body SQL. When the automation has trigger
// conditions they are evaluated here, wrapped around the enrollment call, because the
// WHEN clause cannot hold them.
func (g *AutomationTriggerGenerator) buildFunctionBody(functionName string, automation *domain.Automation, guard string) string {
	frequency := string(automation.Trigger.Frequency)
	if frequency == "" {
		frequency = "every_time"
	}

	var body string
	if guard == "" {
		body = fmt.Sprintf(`BEGIN
    PERFORM automation_enroll_contact(
        %s,
        NEW.email,
        %s,
        %s
    );
    RETURN NEW;
END;`,
			sqlLiteral(automation.ID),
			sqlLiteral(automation.RootNodeID),
			sqlLiteral(frequency),
		)
	} else {
		// An AFTER INSERT body can already see the row that fired it, so a
		// contact_timeline count condition counts the triggering event itself.
		body = fmt.Sprintf(`BEGIN
    IF (%s) THEN
        PERFORM automation_enroll_contact(
            %s,
            NEW.email,
            %s,
            %s
        );
    END IF;
    RETURN NEW;
END;`,
			guard,
			sqlLiteral(automation.ID),
			sqlLiteral(automation.RootNodeID),
			sqlLiteral(frequency),
		)
	}

	tag := dollarQuoteTag(body)

	return fmt.Sprintf(`CREATE OR REPLACE FUNCTION %s()
RETURNS TRIGGER AS %s
%s
%s LANGUAGE plpgsql`,
		functionName,
		tag,
		body,
		tag,
	)
}

// dollarQuoteTag picks a dollar-quote delimiter that does not occur inside body.
// Condition values reach the function body verbatim and nothing else escapes a
// dollar sign, so the tag has to be chosen against the assembled payload.
func dollarQuoteTag(body string) string {
	if !strings.Contains(body, "$$") {
		return "$$"
	}
	for i := 0; ; i++ {
		tag := fmt.Sprintf("$fn%d$", i)
		if !strings.Contains(body, tag) {
			return tag
		}
	}
}

// buildTriggerDDL generates the trigger DDL SQL
func (g *AutomationTriggerGenerator) buildTriggerDDL(triggerName, functionName, whenClause string) string {
	return fmt.Sprintf(`CREATE TRIGGER %s
AFTER INSERT ON contact_timeline
FOR EACH ROW
WHEN (%s)
EXECUTE FUNCTION %s()`,
		triggerName,
		whenClause,
		functionName,
	)
}

// escapeString escapes single quotes for SQL string literals
func escapeString(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}

// sqlLiteral renders s as a complete SQL string literal, quotes included.
// A value carrying a backslash is emitted as an E” literal with the backslash
// doubled, so escaping holds whether or not standard_conforming_strings is on;
// everything else keeps the plain form.
func sqlLiteral(s string) string {
	escaped := escapeString(s)
	if !strings.Contains(escaped, `\`) {
		return "'" + escaped + "'"
	}
	return `E'` + strings.ReplaceAll(escaped, `\`, `\\`) + "'"
}

// placeholderRegex matches PostgreSQL placeholders ($1, $2, etc.)
var placeholderRegex = regexp.MustCompile(`\$(\d+)`)

// insideStringLiteral marks every byte of sql that sits within a single-quoted string
// literal, so a $N appearing there can be recognised as data rather than a placeholder.
//
// This matters because not everything the query builder emits is a placeholder: object keys
// reach the SQL as inlined literals (QueryBuilder.buildJSONPath writes
// custom_json_1['<key>'], defended by doubling quotes), and those keys are caller-supplied.
// A key of literally "$1" would otherwise be expanded here, splicing another filter's value
// — quotes and all — into the middle of a literal and letting it close the subscript and
// continue the expression. A guard collapsing to "... OR TRUE OR ..." enrols every contact
// while claiming to filter, and the same shape reads any table the workspace role can see.
func insideStringLiteral(sql string) []bool {
	inside := make([]bool, len(sql))
	inLiteral := false
	isEString := false

	for i := 0; i < len(sql); i++ {
		c := sql[i]

		if !inLiteral {
			if c == '\'' {
				inLiteral = true
				// E'...' gives backslash its escaping meaning; a plain literal does not
				// (with standard_conforming_strings on, its backslashes are data). Reading
				// the prefix wrong would mis-track where the literal ends.
				isEString = i > 0 && (sql[i-1] == 'E' || sql[i-1] == 'e') &&
					(i < 2 || !isIdentifierByte(sql[i-2]))
			}
			continue
		}

		inside[i] = true

		if isEString && c == '\\' && i+1 < len(sql) {
			inside[i+1] = true
			i++
			continue
		}
		if c == '\'' {
			// A doubled quote is an escaped quote, not the end of the literal.
			if i+1 < len(sql) && sql[i+1] == '\'' {
				inside[i+1] = true
				i++
				continue
			}
			inLiteral = false
			isEString = false
		}
	}

	return inside
}

func isIdentifierByte(c byte) bool {
	return c == '_' ||
		(c >= 'a' && c <= 'z') ||
		(c >= 'A' && c <= 'Z') ||
		(c >= '0' && c <= '9')
}

// embedArgs replaces PostgreSQL placeholders ($1, $2, etc.) with properly escaped values.
// This is necessary because PostgreSQL trigger WHEN clauses cannot use parameterized queries.
// The function handles proper escaping to prevent SQL injection.
func embedArgs(sql string, args []interface{}) (string, error) {
	// Find all placeholders and their positions
	matches := placeholderRegex.FindAllStringSubmatchIndex(sql, -1)

	if len(matches) == 0 {
		return sql, nil
	}

	inLiteral := insideStringLiteral(sql)

	// Build a list of replacements (position, length, replacement string)
	type replacement struct {
		start       int
		end         int
		placeholder int
		value       string
	}
	var replacements []replacement

	for _, match := range matches {
		// match[0] and match[1] are the full match start/end
		// match[2] and match[3] are the capture group (the number)
		fullStart, fullEnd := match[0], match[1]

		// Inside a literal this is the caller's text, not a placeholder. Leaving it be is
		// also why the range check below cannot fire on it: '$9' as an object key is data.
		if inLiteral[fullStart] {
			continue
		}

		numStr := sql[match[2]:match[3]]

		// A placeholder left behind is not harmless: CREATE FUNCTION accepts a body
		// carrying one, then every matching INSERT aborts with 42P02. Fail here instead.
		num, err := strconv.Atoi(numStr)
		if err != nil {
			return "", fmt.Errorf("unparsable placeholder $%s in generated SQL", numStr)
		}

		if num < 1 || num > len(args) {
			return "", fmt.Errorf("placeholder $%d has no argument (%d provided)", num, len(args))
		}

		arg := args[num-1]
		escapedValue, err := escapeArg(arg)
		if err != nil {
			return "", fmt.Errorf("failed to escape arg at position %d: %w", num, err)
		}

		replacements = append(replacements, replacement{
			start:       fullStart,
			end:         fullEnd,
			placeholder: num,
			value:       escapedValue,
		})
	}

	// Sort by position in reverse order so we can replace without affecting indices
	sort.Slice(replacements, func(i, j int) bool {
		return replacements[i].start > replacements[j].start
	})

	// Apply replacements
	result := sql
	for _, r := range replacements {
		result = result[:r.start] + r.value + result[r.end:]
	}

	return result, nil
}

// escapeArg converts an argument to its SQL literal representation with proper escaping
func escapeArg(arg interface{}) (string, error) {
	if arg == nil {
		return "NULL", nil
	}

	switch v := arg.(type) {
	case string:
		return sqlLiteral(v), nil
	case time.Time:
		// Date-valued conditions (in_date_range, before_date, after_date, and any
		// time-typed contact filter) reach here; without this case they fail before
		// any SQL is generated.
		return fmt.Sprintf("%s::timestamptz", sqlLiteral(v.UTC().Format(time.RFC3339Nano))), nil
	case int:
		return strconv.Itoa(v), nil
	case int32:
		return strconv.FormatInt(int64(v), 10), nil
	case int64:
		return strconv.FormatInt(v, 10), nil
	case float32:
		return strconv.FormatFloat(float64(v), 'f', -1, 32), nil
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64), nil
	case bool:
		if v {
			return "TRUE", nil
		}
		return "FALSE", nil
	default:
		return "", fmt.Errorf("unsupported arg type %T", arg)
	}
}
