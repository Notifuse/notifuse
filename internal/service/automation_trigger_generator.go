package service

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/Notifuse/notifuse/internal/domain"
)

// TriggerSQL contains the generated SQL statements for an automation trigger
type TriggerSQL struct {
	FunctionName string // automation_trigger_{id}
	FunctionBody string // CREATE OR REPLACE FUNCTION ...
	TriggerName  string // automation_trigger_{id}
	TriggerDDL   string // CREATE TRIGGER ... WHEN (...) EXECUTE FUNCTION ...
	DropTrigger  string // DROP TRIGGER IF EXISTS ... ON contact_timeline
	DropFunction string // DROP FUNCTION IF EXISTS ...
	WHENClause   string // The WHEN clause alone (for storage/debugging) - values embedded
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
	if len(automation.Trigger.EventKinds) == 0 {
		return nil, fmt.Errorf("automation must have at least one event kind")
	}
	if automation.RootNodeID == "" {
		return nil, fmt.Errorf("automation must have a root node ID")
	}

	// Build WHEN clause (values already embedded, no args returned)
	whenClause, err := g.buildWHENClause(automation)
	if err != nil {
		return nil, fmt.Errorf("failed to build WHEN clause: %w", err)
	}

	// Generate trigger name (remove hyphens from UUID for valid PostgreSQL identifier)
	safeID := strings.ReplaceAll(automation.ID, "-", "")
	triggerName := fmt.Sprintf("automation_trigger_%s", safeID)
	functionName := triggerName

	// Build function body
	functionBody := g.buildFunctionBody(functionName, automation)

	// Build trigger DDL
	triggerDDL := g.buildTriggerDDL(triggerName, functionName, whenClause)

	return &TriggerSQL{
		FunctionName: functionName,
		FunctionBody: functionBody,
		TriggerName:  triggerName,
		TriggerDDL:   triggerDDL,
		DropTrigger:  fmt.Sprintf("DROP TRIGGER IF EXISTS %s ON contact_timeline", triggerName),
		DropFunction: fmt.Sprintf("DROP FUNCTION IF EXISTS %s()", functionName),
		WHENClause:   whenClause,
	}, nil
}

// buildWHENClause builds the WHEN clause for the trigger
func (g *AutomationTriggerGenerator) buildWHENClause(automation *domain.Automation) (string, error) {
	var conditions []string

	// 1. Event kinds filter (required)
	eventKindFilter := g.buildEventKindFilter(automation.Trigger.EventKinds)
	conditions = append(conditions, eventKindFilter)

	// 2. TreeNode conditions (optional)
	if automation.Trigger.Conditions != nil {
		// Get SQL with placeholders and args
		conditionSQL, args, err := g.queryBuilder.BuildTriggerCondition(automation.Trigger.Conditions, "NEW.email")
		if err != nil {
			return "", fmt.Errorf("failed to build TreeNode conditions: %w", err)
		}
		if conditionSQL != "" {
			// Embed args into SQL (trigger WHEN clauses can't use parameters)
			embeddedSQL, err := embedArgs(conditionSQL, args)
			if err != nil {
				return "", fmt.Errorf("failed to embed args: %w", err)
			}
			conditions = append(conditions, embeddedSQL)
		}
	}

	// Combine with AND
	return strings.Join(conditions, " AND "), nil
}

// buildEventKindFilter generates SQL for filtering by event kinds
func (g *AutomationTriggerGenerator) buildEventKindFilter(eventKinds []string) string {
	if len(eventKinds) == 1 {
		return fmt.Sprintf("NEW.kind = '%s'", escapeString(eventKinds[0]))
	}

	// Multiple event kinds: IN clause
	escaped := make([]string, len(eventKinds))
	for i, kind := range eventKinds {
		escaped[i] = fmt.Sprintf("'%s'", escapeString(kind))
	}
	return fmt.Sprintf("NEW.kind IN (%s)", strings.Join(escaped, ", "))
}

// buildFunctionBody generates the function body SQL
func (g *AutomationTriggerGenerator) buildFunctionBody(functionName string, automation *domain.Automation) string {
	frequency := string(automation.Trigger.Frequency)
	if frequency == "" {
		frequency = "every_time"
	}

	return fmt.Sprintf(`CREATE OR REPLACE FUNCTION %s()
RETURNS TRIGGER AS $$
BEGIN
    PERFORM automation_enroll_contact(
        '%s',
        NEW.email,
        '%s',
        '%s',
        '%s'
    );
    RETURN NEW;
END;
$$ LANGUAGE plpgsql`,
		functionName,
		escapeString(automation.ID),
		escapeString(automation.RootNodeID),
		escapeString(automation.ListID),
		escapeString(frequency),
	)
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

// embedArgs replaces PostgreSQL placeholders ($1, $2, etc.) with properly escaped values.
// This is necessary because PostgreSQL trigger WHEN clauses cannot use parameterized queries.
// The function handles proper escaping to prevent SQL injection.
func embedArgs(sql string, args []interface{}) (string, error) {
	if len(args) == 0 {
		return sql, nil
	}

	// Find all placeholders and their positions
	placeholderRegex := regexp.MustCompile(`\$(\d+)`)
	matches := placeholderRegex.FindAllStringSubmatchIndex(sql, -1)

	if len(matches) == 0 {
		return sql, nil
	}

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
		numStr := sql[match[2]:match[3]]

		num, err := strconv.Atoi(numStr)
		if err != nil {
			continue // Skip invalid placeholder numbers
		}

		if num < 1 || num > len(args) {
			continue // Skip out of range placeholders
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
		// Escape single quotes by doubling them
		escaped := strings.ReplaceAll(v, "'", "''")
		return fmt.Sprintf("'%s'", escaped), nil
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
