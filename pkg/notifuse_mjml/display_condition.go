package notifuse_mjml

import (
	"fmt"
	"math"
	"net/url"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

// DisplayCondition shows a block only when rules on the template data hold, all of
// them ("all") or at least one ("any"). It is translated into Liquid evaluated by
// liquidgo, the same Liquid that wraps the block in the MJML export, so sending and a
// template moved to code mode behave the same.
type DisplayCondition struct {
	Match string                 `json:"match"`
	Rules []DisplayConditionRule `json:"rules"`
}

// DisplayConditionRule tests one variable of the template data.
type DisplayConditionRule struct {
	Variable string  `json:"variable"`
	Operator string  `json:"operator"`
	Value    *string `json:"value,omitempty"`
}

// Display condition operators.
const (
	DisplayConditionIsTrue      = "is_true"
	DisplayConditionIsBlank     = "is_blank"
	DisplayConditionIsPresent   = "is_present"
	DisplayConditionIsAbsent    = "is_absent"
	DisplayConditionEq          = "eq"
	DisplayConditionNeq         = "neq"
	DisplayConditionContains    = "contains"
	DisplayConditionNotContains = "not_contains"
	DisplayConditionGt          = "gt"
	DisplayConditionGte         = "gte"
	DisplayConditionLt          = "lt"
	DisplayConditionLte         = "lte"
)

// Display condition limits.
const (
	MaxDisplayConditionRules       = 20
	MaxDisplayConditionVariableLen = 200
	MaxDisplayConditionValueLen    = 200 // Unicode code points
)

var displayConditionOperatorsWithoutValue = map[string]bool{
	DisplayConditionIsTrue:    true,
	DisplayConditionIsBlank:   true,
	DisplayConditionIsPresent: true,
	DisplayConditionIsAbsent:  true,
}

var displayConditionOrderOperators = map[string]bool{
	DisplayConditionGt:  true,
	DisplayConditionGte: true,
	DisplayConditionLt:  true,
	DisplayConditionLte: true,
}

var displayConditionValueOperators = map[string]bool{
	DisplayConditionEq:          true,
	DisplayConditionNeq:         true,
	DisplayConditionContains:    true,
	DisplayConditionNotContains: true,
	DisplayConditionGt:          true,
	DisplayConditionGte:         true,
	DisplayConditionLt:          true,
	DisplayConditionLte:         true,
}

// conditionEligibleTypes are the blocks a condition may hide: hiding a column or a
// group would leave a hole in a layout of explicit widths.
var conditionEligibleTypes = map[MJMLComponentType]bool{
	MJMLComponentMjWrapper: true,
	MJMLComponentMjSection: true,
	MJMLComponentMjText:    true,
	MJMLComponentMjImage:   true,
	MJMLComponentMjButton:  true,
	MJMLComponentMjDivider: true,
	MJMLComponentMjSpacer:  true,
	MJMLComponentMjSocial:  true,
	MJMLComponentMjRaw:     true,
}

var (
	conditionVariableRegex = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*(\.[A-Za-z0-9_]+)*$`)
	conditionNumberRegex   = regexp.MustCompile(`^[+-]?(\d+(\.\d*)?|\.\d+)([eE][+-]?\d+)?$`)
	// a value written inline in the generated Liquid; any other value is decoded at
	// render time, so no character of it can break the Liquid or the MJML
	conditionInlineValueRegex = regexp.MustCompile(`^[A-Za-z0-9_.:/@-]+$`)
	// an eq/neq value that also matches the number it reads as
	conditionEqNumberRegex = regexp.MustCompile(`^-?\d+(\.\d+)?$`)
	blockIDSanitizer       = regexp.MustCompile(`[^A-Za-z0-9_]`)
)

var conditionReservedFirstSegments = map[string]bool{
	"true": true, "false": true, "nil": true, "null": true, "empty": true,
	"blank": true, "and": true, "or": true, "contains": true,
}

var conditionUnsupportedSegments = map[string]bool{"empty": true, "to_liquid": true}

// Validate reports the first reason the condition cannot be saved.
func (c *DisplayCondition) Validate() error {
	if c == nil {
		return nil
	}
	if c.Match != "all" && c.Match != "any" {
		return fmt.Errorf("display condition match must be \"all\" or \"any\"")
	}
	if len(c.Rules) == 0 {
		return fmt.Errorf("display condition needs at least one rule")
	}
	if len(c.Rules) > MaxDisplayConditionRules {
		return fmt.Errorf("display condition has more than %d rules", MaxDisplayConditionRules)
	}
	for i, rule := range c.Rules {
		if err := rule.validate(); err != nil {
			return fmt.Errorf("display condition rule %d: %w", i+1, err)
		}
	}
	return nil
}

func (r DisplayConditionRule) validate() error {
	if !validConditionVariable(r.Variable) {
		return fmt.Errorf("invalid variable %q", r.Variable)
	}
	if displayConditionOperatorsWithoutValue[r.Operator] {
		if r.Value != nil {
			return fmt.Errorf("operator %s takes no value", r.Operator)
		}
		return nil
	}
	if !displayConditionValueOperators[r.Operator] {
		return fmt.Errorf("unknown operator %q", r.Operator)
	}
	if r.Value == nil || strings.TrimSpace(*r.Value) == "" {
		return fmt.Errorf("operator %s needs a value", r.Operator)
	}
	value := *r.Value
	if utf8.RuneCountInString(value) > MaxDisplayConditionValueLen {
		return fmt.Errorf("value is longer than %d characters", MaxDisplayConditionValueLen)
	}
	for _, delimiter := range []string{"{{", "}}", "{%", "%}"} {
		if strings.Contains(value, delimiter) {
			return fmt.Errorf("value cannot contain %s", delimiter)
		}
	}
	if displayConditionOrderOperators[r.Operator] {
		if _, ok := conditionNumber(value); !ok {
			return fmt.Errorf("operator %s needs a number of at most 15 digits before the decimal point", r.Operator)
		}
	}
	return nil
}

func validConditionVariable(variable string) bool {
	if len(variable) > MaxDisplayConditionVariableLen || !conditionVariableRegex.MatchString(variable) {
		return false
	}
	segments := strings.Split(variable, ".")
	if conditionReservedFirstSegments[segments[0]] || strings.HasPrefix(segments[0], "__dc") {
		return false
	}
	// liquidgo reads these as lookup commands, which the console's evaluator does not
	// mirror: refused so the editor and the send never disagree.
	for _, segment := range segments[1:] {
		if conditionUnsupportedSegments[segment] {
			return false
		}
	}
	return true
}

// maxConditionNumber bounds the numbers a condition compares: below it, a number has
// at most 15 integer digits, so liquidgo (integers as int64, decimals as float64) and
// the console's evaluator (float64 only) read every one of them alike.
const maxConditionNumber = 1e15

// conditionNumber parses an order operator's value, normalised as Liquid writes it.
func conditionNumber(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if !conditionNumberRegex.MatchString(value) {
		return "", false
	}
	f, err := strconv.ParseFloat(value, 64)
	if err != nil || math.IsNaN(f) || math.Abs(f) >= maxConditionNumber {
		return "", false
	}
	if f == 0 {
		f = 0 // no "-0"
	}
	return strconv.FormatFloat(f, 'f', -1, 64), true
}

// conditionNames are the variables the generated Liquid assigns. Evaluate uses the
// bare names, so liquidgo's parse caches do not grow per block; the export suffixes
// them with the block id, which is harmless even on a collision since each one is
// assigned right before it is read.
type conditionNames struct {
	flag, ok, vs, c, b, f, r, p, i, n string
	value                             func(i int) string
}

func newConditionNames(suffix string) conditionNames {
	name := func(base string) string { return base + suffix }
	return conditionNames{
		flag: name("__dc"), ok: name("__dc_ok"), vs: name("__dc_vs"), c: name("__dc_c"),
		b: name("__dc_b"), f: name("__dc_f"), r: name("__dc_r"), p: name("__dc_p"), i: name("__dc_i"), n: name("__dc_n"),
		value: func(i int) string { return fmt.Sprintf("__dc_v%d%s", i, suffix) },
	}
}

// exportSuffix derives the variable suffix of a block's exported condition.
func exportSuffix(blockID string) string {
	s := blockIDSanitizer.ReplaceAllString(blockID, "_")
	if len(s) > 16 {
		s = s[:16]
	}
	if s == "" {
		return ""
	}
	return "_" + s
}

// ToLiquid returns the Liquid that opens and closes the block. Generation never fails:
// an unknown match is "all", a rule that cannot be generated is false, and a condition
// without rules shows the block. Each part is one line with no whitespace between
// tags, so the block renders exactly as it would unwrapped.
func (c *DisplayCondition) ToLiquid(blockID string) (open, close string) {
	return c.toLiquid(newConditionNames(exportSuffix(blockID)))
}

func (c *DisplayCondition) toLiquid(names conditionNames) (open, close string) {
	if c == nil || len(c.Rules) == 0 {
		return "", ""
	}
	var b strings.Builder
	matchAny := c.Match == "any"
	if matchAny {
		fmt.Fprintf(&b, "{%% assign %s = false %%}", names.flag)
	} else {
		fmt.Fprintf(&b, "{%% assign %s = true %%}", names.flag)
	}
	for i, rule := range c.Rules {
		b.WriteString(rule.toLiquid(names, i))
		if matchAny {
			fmt.Fprintf(&b, "{%% if %s %%}{%% assign %s = true %%}{%% endif %%}", names.ok, names.flag)
		} else {
			fmt.Fprintf(&b, "{%% unless %s %%}{%% assign %s = false %%}{%% endunless %%}", names.ok, names.flag)
		}
	}
	fmt.Fprintf(&b, "{%% if %s %%}", names.flag)
	return b.String(), "{% endif %}"
}

// toLiquid sets names.ok to whether the rule holds.
func (r DisplayConditionRule) toLiquid(names conditionNames, index int) string {
	set := func(condition string) string {
		return fmt.Sprintf("{%% assign %s = false %%}{%% if %s %%}{%% assign %s = true %%}{%% endif %%}", names.ok, condition, names.ok)
	}
	setUnless := func(condition string) string {
		return fmt.Sprintf("{%% assign %s = true %%}{%% if %s %%}{%% assign %s = false %%}{%% endif %%}", names.ok, condition, names.ok)
	}
	never := fmt.Sprintf("{%% assign %s = false %%}", names.ok)
	if !validConditionVariable(r.Variable) {
		return never
	}
	v := r.Variable
	switch r.Operator {
	case DisplayConditionIsTrue:
		return set(fmt.Sprintf(`%s == true or %s == "true"`, v, v))
	case DisplayConditionIsBlank:
		return set(fmt.Sprintf("%s == blank or %s == false or %s == empty", v, v, v))
	case DisplayConditionIsPresent:
		return set(fmt.Sprintf("%s != blank and %s != empty", v, v))
	case DisplayConditionIsAbsent:
		return set(fmt.Sprintf("%s == blank or %s == empty", v, v))
	}
	if r.Value == nil || strings.TrimSpace(*r.Value) == "" {
		return never
	}
	value := *r.Value
	if displayConditionOrderOperators[r.Operator] {
		number, ok := conditionNumber(value)
		if !ok {
			return never
		}
		return names.numericGuard(v, orderComparison(r.Operator, names.n, number))
	}
	prelude, literal := names.valueLiteral(index, value)
	alternatives := []string{literal}
	if number, ok := eqNumber(value); ok {
		alternatives = append(alternatives, number)
	}
	switch r.Operator {
	case DisplayConditionEq, DisplayConditionNeq:
		if value == "true" || value == "false" {
			alternatives = append(alternatives, value)
		}
		parts := make([]string, len(alternatives))
		for i, alt := range alternatives {
			parts[i] = fmt.Sprintf("%s == %s", v, alt)
		}
		if r.Operator == DisplayConditionEq {
			return prelude + set(strings.Join(parts, " or "))
		}
		return prelude + setUnless(strings.Join(parts, " or "))
	case DisplayConditionContains, DisplayConditionNotContains:
		parts := make([]string, len(alternatives))
		for i, alt := range alternatives {
			parts[i] = fmt.Sprintf("%s contains %s", v, alt)
		}
		if r.Operator == DisplayConditionContains {
			return prelude + set(strings.Join(parts, " or "))
		}
		return prelude + setUnless(strings.Join(parts, " or "))
	}
	return never
}

// eqNumber is the number an eq/neq value also matches: "100" matches 100 and 100.0.
func eqNumber(value string) (string, bool) {
	if !conditionEqNumberRegex.MatchString(value) {
		return "", false
	}
	return conditionNumber(value)
}

// valueLiteral writes value as a Liquid string, inline when it is made of safe
// characters, otherwise assigned from its URL encoding.
func (names conditionNames) valueLiteral(index int, value string) (prelude, literal string) {
	if conditionInlineValueRegex.MatchString(value) {
		return "", `"` + value + `"`
	}
	name := names.value(index)
	return fmt.Sprintf(`{%% assign %s = "%s" | url_decode %%}`, name, url.QueryEscape(value)), name
}

// orderComparison compares the number n with value, without "<" so the export stays
// valid MJML.
func orderComparison(operator, n, value string) string {
	switch operator {
	case DisplayConditionGt:
		return fmt.Sprintf("%s > %s", n, value)
	case DisplayConditionGte:
		return fmt.Sprintf("%s >= %s", n, value)
	case DisplayConditionLt:
		return fmt.Sprintf("%s > %s", value, n)
	default:
		return fmt.Sprintf("%s >= %s", value, n)
	}
}

// numericGuard sets names.ok to comparison when v is a number or a decimal text
// ("19.90", " 150 ", "-5", "+5", "007"), read into names.n; text, booleans, lists,
// objects and absent values are not numbers, nor are "1e2" and ".5", nor numbers of
// more than 15 integer digits (see maxConditionNumber).
func (names conditionNames) numericGuard(v, comparison string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "{%% assign %s = false %%}", names.ok)
	fmt.Fprintf(&b, `{%% assign %s = %s | append: "" | strip %%}`, names.vs, v)
	fmt.Fprintf(&b, `{%% assign %s = %s | slice: 0 %%}`, names.c, names.vs)
	fmt.Fprintf(&b, `{%% assign %s = %s %%}`, names.b, names.vs)
	fmt.Fprintf(&b, `{%% if %s == "-" or %s == "+" %%}{%% assign %s = %s | slice: 1, 1000000 %%}{%% endif %%}`, names.c, names.c, names.b, names.vs)
	fmt.Fprintf(&b, `{%% assign %s = %s | slice: 0 %%}`, names.f, names.b)
	fmt.Fprintf(&b, `{%% assign %s = %s`, names.r, names.b)
	for d := 0; d <= 9; d++ {
		fmt.Fprintf(&b, ` | remove: "%d"`, d)
	}
	b.WriteString(" %}")
	fmt.Fprintf(&b, `{%% assign %s = %s | split: "." | size %%}`, names.p, names.b)
	fmt.Fprintf(&b, `{%% if %s == "" or %s == "." %%}{%% if 3 > %s %%}{%% if %s != "" %%}{%% if "0123456789" contains %s %%}`, names.r, names.r, names.p, names.f, names.f)
	fmt.Fprintf(&b, `{%% assign %s = %s | split: "." | first | size %%}{%% if 16 > %s %%}`, names.i, names.b, names.i)
	fmt.Fprintf(&b, `{%% assign %s = %s | plus: 0 %%}{%% if %s == "-" %%}{%% assign %s = 0 | minus: %s %%}{%% endif %%}`, names.n, names.b, names.c, names.n, names.n)
	fmt.Fprintf(&b, `{%% if %s %%}{%% assign %s = true %%}{%% endif %%}`, comparison, names.ok)
	b.WriteString("{% endif %}{% endif %}{% endif %}{% endif %}{% endif %}")
	return b.String()
}

// Evaluate reports whether the block shows with data, with the Liquid the export
// writes, rendered by liquidgo and parsed once per pooled engine (every name the
// generated Liquid reads is assigned first, so a cached template is safe to reuse). A nil condition shows the block without rendering.
// Any output other than "1" or nothing (a Liquid error text) hides the block. The
// error is the engine's own failure (timeout, size), never the condition's result.
func (c *DisplayCondition) Evaluate(data map[string]interface{}) (bool, error) {
	if c == nil || len(c.Rules) == 0 {
		return true, nil
	}
	open, close := c.toLiquid(newConditionNames(""))
	data = withoutConditionNames(data)
	engine := liquidEnginePool.Get().(*SecureLiquidEngine)
	out, err := engine.renderCached(open+"1"+close, data)
	if err != nil {
		// as in processLiquidContent, an engine that failed is not repooled
		return false, fmt.Errorf("display condition: %w", err)
	}
	liquidEnginePool.Put(engine)
	return out == "1", nil
}

// withoutConditionNames returns data without its top-level keys named like the
// generated variables: liquidgo reads data before assigns, so a "__dc" key would
// otherwise decide the condition. The data is copied only when it has such a key.
func withoutConditionNames(data map[string]interface{}) map[string]interface{} {
	if data == nil {
		return map[string]interface{}{}
	}
	hasConditionName := false
	for key := range data {
		if strings.HasPrefix(key, "__dc") {
			hasConditionName = true
			break
		}
	}
	if !hasConditionName {
		return data
	}
	clean := make(map[string]interface{}, len(data))
	for key, value := range data {
		if !strings.HasPrefix(key, "__dc") {
			clean[key] = value
		}
	}
	return clean
}

// IsConditionEligible reports whether a block of this type may carry a condition when
// it sits under mj-body.
func IsConditionEligible(blockType MJMLComponentType) bool {
	return conditionEligibleTypes[blockType]
}

// ValidateDisplayConditions checks every condition of a template tree: only on
// eligible blocks under mj-body, and each one valid.
func ValidateDisplayConditions(tree EmailBlock) error {
	return validateDisplayConditionsIn(tree, false)
}

// ValidateDisplayConditionsInBlock checks a saved block, which has no mj-head context.
func ValidateDisplayConditionsInBlock(block EmailBlock) error {
	return validateDisplayConditionsIn(block, false)
}

func validateDisplayConditionsIn(block EmailBlock, inHead bool) error {
	if isNilBlock(block) {
		return nil
	}
	if block.GetType() == MJMLComponentMjHead {
		inHead = true
	}
	if condition := block.GetDisplayCondition(); condition != nil {
		if inHead || !IsConditionEligible(block.GetType()) {
			return fmt.Errorf("block %s (%s) cannot have a display condition", block.GetID(), block.GetType())
		}
		// a hidden block drops its mj-liquid blocks, which can hold one half of a tag
		// pair ({% if %} here, {% endif %} outside): the rest of the email would no
		// longer parse, for the recipients the block is hidden from only
		if hasLiquidBlock(block) {
			return fmt.Errorf("block %s (%s) holds an mj-liquid block and cannot have a display condition", block.GetID(), block.GetType())
		}
		if err := condition.Validate(); err != nil {
			return fmt.Errorf("block %s: %w", block.GetID(), err)
		}
	}
	for _, child := range block.GetChildren() {
		if err := validateDisplayConditionsIn(child, inHead); err != nil {
			return err
		}
	}
	return nil
}

// hasLiquidBlock reports whether an mj-liquid block sits under block.
func hasLiquidBlock(block EmailBlock) bool {
	for _, child := range block.GetChildren() {
		if isNilBlock(child) {
			continue
		}
		if child.GetType() == MJMLComponentMjLiquid || hasLiquidBlock(child) {
			return true
		}
	}
	return false
}

// isNilBlock reports a nil block, including a typed nil pointer in the interface.
func isNilBlock(block EmailBlock) bool {
	if block == nil {
		return true
	}
	v := reflect.ValueOf(block)
	return v.Kind() == reflect.Ptr && v.IsNil()
}
