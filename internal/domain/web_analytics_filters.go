package domain

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Attribution filters are the channel-grouping rule engine ported from
// Staminads: per-workspace, user-editable rules evaluated at ingest time (and
// compiled to SQL for backfills). Rules read SOURCE fields (utm, referrer,
// device, ...) and write WRITABLE dimensions (channel, channel_group, custom_*,
// utm_*, referrer_domain, is_direct).

// WebFilterOperator values for rule conditions.
const (
	WebFilterOpEquals      = "equals"
	WebFilterOpNotEquals   = "not_equals"
	WebFilterOpContains    = "contains"
	WebFilterOpNotContains = "not_contains"
	WebFilterOpIsEmpty     = "is_empty"
	WebFilterOpIsNotEmpty  = "is_not_empty"
	WebFilterOpRegex       = "regex"
)

// WebFilterAction values for rule operations.
const (
	WebFilterActionSetValue        = "set_value"
	WebFilterActionUnsetValue      = "unset_value"
	WebFilterActionSetDefaultValue = "set_default_value"
)

// WebFilterWritableDimensions is the set of dimensions rules may write.
var WebFilterWritableDimensions = map[string]bool{
	"channel": true, "channel_group": true,
	"custom_1": true, "custom_2": true, "custom_3": true, "custom_4": true, "custom_5": true,
	"custom_6": true, "custom_7": true, "custom_8": true, "custom_9": true, "custom_10": true,
	"utm_source": true, "utm_medium": true, "utm_campaign": true,
	"utm_term": true, "utm_content": true,
	"referrer_domain": true, "is_direct": true,
}

// WebFilterSourceFields is the set of fields rule conditions may read.
var WebFilterSourceFields = map[string]bool{
	"utm_source": true, "utm_medium": true, "utm_campaign": true,
	"utm_term": true, "utm_content": true, "utm_id": true, "utm_id_from": true,
	"referrer": true, "referrer_domain": true, "referrer_path": true,
	"is_direct":    true,
	"landing_page": true, "landing_domain": true, "landing_path": true, "path": true,
	"device": true, "browser": true, "browser_type": true, "os": true,
	"user_agent": true, "connection_type": true,
	"language": true, "timezone": true,
}

// WebFilterCondition is one condition of a rule; all of a rule's conditions
// must match (AND).
type WebFilterCondition struct {
	Field    string `json:"field"`
	Operator string `json:"operator"`
	Value    string `json:"value,omitempty"` // unused by is_empty / is_not_empty
}

// WebFilterOperation is executed when a rule matches.
type WebFilterOperation struct {
	Dimension string `json:"dimension"`
	Action    string `json:"action"`
	Value     string `json:"value,omitempty"` // required for set_value / set_default_value
}

// WebFilter is one attribution rule.
type WebFilter struct {
	ID         string               `json:"id"`
	Name       string               `json:"name"`
	Priority   int                  `json:"priority"` // 0-1000, higher evaluated first
	Order      int                  `json:"order"`    // UI display order
	Tags       []string             `json:"tags,omitempty"`
	Conditions []WebFilterCondition `json:"conditions"`
	Operations []WebFilterOperation `json:"operations"`
	Enabled    bool                 `json:"enabled"`
	Version    string               `json:"version,omitempty"`
	CreatedAt  string               `json:"created_at,omitempty"`
	UpdatedAt  string               `json:"updated_at,omitempty"`
}

// Validate checks a rule. Regex patterns must compile under Go's RE2 — the
// same family Staminads enforced — which also keeps them portable to the SQL
// backfill path as long as patterns stay simple.
func (f *WebFilter) Validate() error {
	if strings.TrimSpace(f.ID) == "" {
		return fmt.Errorf("filter id is required")
	}
	if strings.TrimSpace(f.Name) == "" {
		return fmt.Errorf("filter name is required")
	}
	if f.Priority < 0 || f.Priority > 1000 {
		return fmt.Errorf("priority must be between 0 and 1000")
	}
	for _, c := range f.Conditions {
		if !WebFilterSourceFields[c.Field] {
			return fmt.Errorf("condition field %q is not a valid source field", c.Field)
		}
		switch c.Operator {
		case WebFilterOpEquals, WebFilterOpNotEquals, WebFilterOpContains,
			WebFilterOpNotContains, WebFilterOpIsEmpty, WebFilterOpIsNotEmpty:
		case WebFilterOpRegex:
			if _, err := regexp.Compile(c.Value); err != nil {
				return fmt.Errorf("condition regex %q does not compile: %v", c.Value, err)
			}
		default:
			return fmt.Errorf("unknown condition operator: %q", c.Operator)
		}
	}
	if len(f.Operations) == 0 {
		return fmt.Errorf("filter needs at least one operation")
	}
	for _, op := range f.Operations {
		if !WebFilterWritableDimensions[op.Dimension] {
			return fmt.Errorf("operation dimension %q is not writable", op.Dimension)
		}
		switch op.Action {
		case WebFilterActionSetValue, WebFilterActionSetDefaultValue:
			if op.Value == "" {
				return fmt.Errorf("operation %s on %q requires a value", op.Action, op.Dimension)
			}
		case WebFilterActionUnsetValue:
		default:
			return fmt.Errorf("unknown operation action: %q", op.Action)
		}
	}
	return nil
}

// ComputeWebFiltersVersion hashes the semantic content of a rule set (id,
// conditions, operations, enabled, priority — sorted by id) to an 8-hex-char
// version used for backfill staleness detection. Only comparability within one
// workspace matters; the hash is not byte-compatible with Staminads'.
func ComputeWebFiltersVersion(filters []WebFilter) string {
	type versionedOp struct {
		Dimension string `json:"dimension"`
		Action    string `json:"action"`
		Value     string `json:"value"`
	}
	type versionedCond struct {
		Field    string `json:"field"`
		Operator string `json:"operator"`
		Value    string `json:"value"`
	}
	type versionedFilter struct {
		ID         string          `json:"id"`
		Conditions []versionedCond `json:"conditions"`
		Operations []versionedOp   `json:"operations"`
		Enabled    bool            `json:"enabled"`
		Priority   int             `json:"priority"`
	}

	entries := make([]versionedFilter, 0, len(filters))
	for _, f := range filters {
		vf := versionedFilter{ID: f.ID, Enabled: f.Enabled, Priority: f.Priority}
		for _, c := range f.Conditions {
			vf.Conditions = append(vf.Conditions, versionedCond(c))
		}
		for _, op := range f.Operations {
			vf.Operations = append(vf.Operations, versionedOp(op))
		}
		entries = append(entries, vf)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].ID < entries[j].ID })

	payload, _ := json.Marshal(entries)
	sum := md5.Sum(payload)
	return hex.EncodeToString(sum[:])[:8]
}

// EvaluateWebFilterCondition evaluates one condition against field values.
// Missing or empty field values match only is_empty (Staminads semantics).
func EvaluateWebFilterCondition(c WebFilterCondition, fields map[string]string) bool {
	value, ok := fields[c.Field]
	if !ok || value == "" {
		return c.Operator == WebFilterOpIsEmpty
	}
	switch c.Operator {
	case WebFilterOpEquals:
		return value == c.Value
	case WebFilterOpNotEquals:
		return value != c.Value
	case WebFilterOpContains:
		return strings.Contains(value, c.Value)
	case WebFilterOpNotContains:
		return !strings.Contains(value, c.Value)
	case WebFilterOpIsEmpty:
		return false // non-empty handled above
	case WebFilterOpIsNotEmpty:
		return true
	case WebFilterOpRegex:
		re, err := regexp.Compile(c.Value)
		if err != nil {
			return false // invalid regex never matches
		}
		return re.MatchString(value)
	default:
		return false
	}
}

// WebFilterResult maps written dimensions to their value; a nil value means
// the dimension was explicitly unset.
type WebFilterResult map[string]*string

// EvaluateWebFilters runs the rule set against the given source field values.
//
// Algorithm (Staminads parity): enabled rules sorted by priority descending
// (stable, so equal priorities keep their input order); a rule matches when
// ALL its conditions match (an empty condition list always matches);
// set_value/unset_value write unless a strictly higher priority already wrote
// the dimension; set_default_value only fills dimensions that are still unset
// or null, and never claims priority.
func EvaluateWebFilters(filters []WebFilter, fields map[string]string) WebFilterResult {
	result := WebFilterResult{}
	dimensionPriorities := map[string]int{}

	sorted := make([]WebFilter, 0, len(filters))
	for _, f := range filters {
		if f.Enabled {
			sorted = append(sorted, f)
		}
	}
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Priority > sorted[j].Priority })

	for _, filter := range sorted {
		matches := true
		for _, c := range filter.Conditions {
			if !EvaluateWebFilterCondition(c, fields) {
				matches = false
				break
			}
		}
		if !matches {
			continue
		}

		for _, op := range filter.Operations {
			currentPriority, hasPriority := dimensionPriorities[op.Dimension]
			if !hasPriority {
				currentPriority = -1
			}

			switch op.Action {
			case WebFilterActionSetValue:
				if filter.Priority < currentPriority {
					continue
				}
				v := op.Value
				result[op.Dimension] = &v
				dimensionPriorities[op.Dimension] = filter.Priority
			case WebFilterActionUnsetValue:
				if filter.Priority < currentPriority {
					continue
				}
				result[op.Dimension] = nil
				dimensionPriorities[op.Dimension] = filter.Priority
			case WebFilterActionSetDefaultValue:
				if existing, isSet := result[op.Dimension]; !isSet || existing == nil {
					v := op.Value
					result[op.Dimension] = &v
					// Priority is deliberately not claimed: later (lower
					// priority) set_value rules may still override a default.
				}
			}
		}
	}

	return result
}
