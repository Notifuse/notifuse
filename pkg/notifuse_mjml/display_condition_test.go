package notifuse_mjml

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// displayConditionFixture is shared with the console evaluator, so the editor's
// preview and sending agree on every case.
type displayConditionFixture struct {
	Evaluate []struct {
		Name      string                 `json:"name"`
		Data      map[string]interface{} `json:"data"`
		Condition *DisplayCondition      `json:"condition"`
		Expected  bool                   `json:"expected"`
	} `json:"evaluate"`
	Validate []struct {
		Name      string            `json:"name"`
		Condition *DisplayCondition `json:"condition"`
		Valid     bool              `json:"valid"`
	} `json:"validate"`
}

func loadDisplayConditionFixture(t testing.TB) displayConditionFixture {
	t.Helper()
	raw, err := os.ReadFile("testdata/display_conditions.json")
	require.NoError(t, err)
	var fixture displayConditionFixture
	require.NoError(t, json.Unmarshal(raw, &fixture))
	return fixture
}

func TestDisplayConditionFixture_Evaluate(t *testing.T) {
	fixture := loadDisplayConditionFixture(t)
	require.NotEmpty(t, fixture.Evaluate)
	for _, tc := range fixture.Evaluate {
		t.Run(tc.Name, func(t *testing.T) {
			got, err := tc.Condition.Evaluate(tc.Data)
			require.NoError(t, err)
			assert.Equal(t, tc.Expected, got)
		})
	}
}

func TestDisplayConditionFixture_Validate(t *testing.T) {
	fixture := loadDisplayConditionFixture(t)
	require.NotEmpty(t, fixture.Validate)
	for _, tc := range fixture.Validate {
		t.Run(tc.Name, func(t *testing.T) {
			err := tc.Condition.Validate()
			if tc.Valid {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

// Every operator must be pinned both ways, so a generator that always shows (or always
// hides) the block cannot pass the fixture.
func TestDisplayConditionFixture_CoversEveryOperatorBothWays(t *testing.T) {
	fixture := loadDisplayConditionFixture(t)
	outcomes := map[string]map[bool]bool{}
	for _, tc := range fixture.Evaluate {
		if tc.Condition == nil || len(tc.Condition.Rules) != 1 {
			continue
		}
		op := tc.Condition.Rules[0].Operator
		if outcomes[op] == nil {
			outcomes[op] = map[bool]bool{}
		}
		outcomes[op][tc.Expected] = true
	}
	for _, op := range []string{
		DisplayConditionIsTrue, DisplayConditionIsBlank, DisplayConditionIsPresent, DisplayConditionIsAbsent,
		DisplayConditionEq, DisplayConditionNeq, DisplayConditionContains, DisplayConditionNotContains,
		DisplayConditionGt, DisplayConditionGte, DisplayConditionLt, DisplayConditionLte,
	} {
		assert.True(t, outcomes[op][true], "%s has no case that shows the block", op)
		assert.True(t, outcomes[op][false], "%s has no case that hides the block", op)
	}
}

// renderCondition renders the condition's Liquid around "1" as Evaluate does, returning
// the raw output.
func renderCondition(t *testing.T, open, close string, data map[string]interface{}) string {
	t.Helper()
	if data == nil {
		data = map[string]interface{}{}
	}
	out, err := NewSecureLiquidEngine().RenderWithTimeout(open+"1"+close, data)
	require.NoError(t, err)
	return out
}

// The "1 or nothing" safety net in Evaluate must never be what decides a supported
// case, and liquidgo swallows a rule's error inside a flag construction, so each rule is
// also rendered alone and must not print an error text.
func TestDisplayConditionFixture_OutputIsOneOrEmpty(t *testing.T) {
	fixture := loadDisplayConditionFixture(t)
	for _, tc := range fixture.Evaluate {
		t.Run(tc.Name, func(t *testing.T) {
			open, close := tc.Condition.toLiquid(newConditionNames(""))
			out := renderCondition(t, open, close, tc.Data)
			assert.Contains(t, []string{"1", ""}, out)
			for i, rule := range tc.Condition.Rules {
				names := newConditionNames("")
				open := rule.toLiquid(names, i) + "{% if " + names.ok + " %}"
				out := renderCondition(t, open, "{% endif %}", tc.Data)
				assert.Contains(t, []string{"1", ""}, out, "rule %d alone", i)
			}
		})
	}
}

// The export suffixes the generated names with the block id; it must decide every case
// as Evaluate does.
func TestDisplayConditionFixture_ExportMatchesEvaluate(t *testing.T) {
	fixture := loadDisplayConditionFixture(t)
	for _, tc := range fixture.Evaluate {
		t.Run(tc.Name, func(t *testing.T) {
			open, close := tc.Condition.ToLiquid("block-42")
			assert.NotContains(t, open+close, "<", "the export must stay valid MJML")
			got := renderCondition(t, open, close, tc.Data) == "1"
			if tc.Condition == nil || len(tc.Condition.Rules) == 0 {
				assert.Equal(t, "", open+close)
				return
			}
			assert.Equal(t, tc.Expected, got)
		})
	}
}

func TestDisplayCondition_Evaluate(t *testing.T) {
	t.Run("nil condition shows the block", func(t *testing.T) {
		var c *DisplayCondition
		ok, err := c.Evaluate(map[string]interface{}{"a": 1})
		require.NoError(t, err)
		assert.True(t, ok)
	})
	t.Run("nil data hides a presence rule", func(t *testing.T) {
		c := &DisplayCondition{Match: "all", Rules: []DisplayConditionRule{{Variable: "contact.email", Operator: DisplayConditionIsPresent}}}
		ok, err := c.Evaluate(nil)
		require.NoError(t, err)
		assert.False(t, ok)
	})
	t.Run("a data key does not leak into the next evaluation", func(t *testing.T) {
		c := &DisplayCondition{Match: "all", Rules: []DisplayConditionRule{{Variable: "contact.vip", Operator: DisplayConditionIsTrue}}}
		ok, err := c.Evaluate(map[string]interface{}{"contact": map[string]interface{}{"vip": true}})
		require.NoError(t, err)
		assert.True(t, ok)
		ok, err = c.Evaluate(map[string]interface{}{"contact": map[string]interface{}{}})
		require.NoError(t, err)
		assert.False(t, ok)
	})
}

// The largest condition validation accepts must render under the engine's template size
// cap, or every send of the template would fail.
func TestDisplayCondition_LargestConditionFitsTheEngine(t *testing.T) {
	longValue := strings.Repeat("🎉", MaxDisplayConditionValueLen)
	longVariable := "a" + strings.Repeat(".b", (MaxDisplayConditionVariableLen-1)/2)
	require.LessOrEqual(t, len(longVariable), MaxDisplayConditionVariableLen)

	for _, operator := range []string{DisplayConditionEq, DisplayConditionNotContains, DisplayConditionLte} {
		t.Run(operator, func(t *testing.T) {
			value := longValue
			if displayConditionOrderOperators[operator] {
				value = "-12345678901234." + strings.Repeat("1", MaxDisplayConditionValueLen-16)
			}
			rules := make([]DisplayConditionRule, MaxDisplayConditionRules)
			for i := range rules {
				rules[i] = DisplayConditionRule{Variable: longVariable, Operator: operator, Value: stringPtr(value)}
			}
			c := &DisplayCondition{Match: "any", Rules: rules}
			require.NoError(t, c.Validate())

			open, close := c.ToLiquid(strings.Repeat("x", 40))
			assert.Less(t, len(open+"1"+close), DefaultMaxTemplateSize)
			_, err := c.Evaluate(map[string]interface{}{})
			assert.NoError(t, err)
			out := renderCondition(t, open, close, map[string]interface{}{})
			assert.Contains(t, []string{"1", ""}, out)
		})
	}
}

func TestDisplayCondition_ToLiquid(t *testing.T) {
	cases := []struct {
		name      string
		condition *DisplayCondition
		open      string
	}{
		{
			name:      "is_true",
			condition: &DisplayCondition{Match: "all", Rules: []DisplayConditionRule{{Variable: "contact.vip", Operator: DisplayConditionIsTrue}}},
			open:      `{% assign __dc_blk_1 = true %}{% assign __dc_ok_blk_1 = false %}{% if contact.vip == true or contact.vip == "true" %}{% assign __dc_ok_blk_1 = true %}{% endif %}{% unless __dc_ok_blk_1 %}{% assign __dc_blk_1 = false %}{% endunless %}{% if __dc_blk_1 %}`,
		},
		{
			name: "any, is_blank and neq on a number",
			condition: &DisplayCondition{Match: "any", Rules: []DisplayConditionRule{
				{Variable: "a", Operator: DisplayConditionIsBlank},
				{Variable: "b", Operator: DisplayConditionNeq, Value: stringPtr("100")},
			}},
			open: `{% assign __dc_blk_1 = false %}{% assign __dc_ok_blk_1 = false %}{% if a == blank or a == false or a == empty %}{% assign __dc_ok_blk_1 = true %}{% endif %}{% if __dc_ok_blk_1 %}{% assign __dc_blk_1 = true %}{% endif %}{% assign __dc_ok_blk_1 = true %}{% if b == "100" or b == 100 %}{% assign __dc_ok_blk_1 = false %}{% endif %}{% if __dc_ok_blk_1 %}{% assign __dc_blk_1 = true %}{% endif %}{% if __dc_blk_1 %}`,
		},
		{
			name:      "encoded value",
			condition: &DisplayCondition{Match: "all", Rules: []DisplayConditionRule{{Variable: "a", Operator: DisplayConditionContains, Value: stringPtr("R&D <x>")}}},
			open:      `{% assign __dc_blk_1 = true %}{% assign __dc_v0_blk_1 = "R%26D+%3Cx%3E" | url_decode %}{% assign __dc_ok_blk_1 = false %}{% if a contains __dc_v0_blk_1 %}{% assign __dc_ok_blk_1 = true %}{% endif %}{% unless __dc_ok_blk_1 %}{% assign __dc_blk_1 = false %}{% endunless %}{% if __dc_blk_1 %}`,
		},
		{
			name:      "eq true matches the boolean",
			condition: &DisplayCondition{Match: "all", Rules: []DisplayConditionRule{{Variable: "a", Operator: DisplayConditionEq, Value: stringPtr("true")}}},
			open:      `{% assign __dc_blk_1 = true %}{% assign __dc_ok_blk_1 = false %}{% if a == "true" or a == true %}{% assign __dc_ok_blk_1 = true %}{% endif %}{% unless __dc_ok_blk_1 %}{% assign __dc_blk_1 = false %}{% endunless %}{% if __dc_blk_1 %}`,
		},
		{
			name:      "normalised number on lt, without a less-than sign",
			condition: &DisplayCondition{Match: "all", Rules: []DisplayConditionRule{{Variable: "a", Operator: DisplayConditionLt, Value: stringPtr("+.5e1")}}},
			open:      `{% assign __dc_blk_1 = true %}{% assign __dc_ok_blk_1 = false %}{% assign __dc_vs_blk_1 = a | append: "" | strip %}{% assign __dc_c_blk_1 = __dc_vs_blk_1 | slice: 0 %}{% assign __dc_b_blk_1 = __dc_vs_blk_1 %}{% if __dc_c_blk_1 == "-" or __dc_c_blk_1 == "+" %}{% assign __dc_b_blk_1 = __dc_vs_blk_1 | slice: 1, 1000000 %}{% endif %}{% assign __dc_f_blk_1 = __dc_b_blk_1 | slice: 0 %}{% assign __dc_r_blk_1 = __dc_b_blk_1 | remove: "0" | remove: "1" | remove: "2" | remove: "3" | remove: "4" | remove: "5" | remove: "6" | remove: "7" | remove: "8" | remove: "9" %}{% assign __dc_p_blk_1 = __dc_b_blk_1 | split: "." | size %}{% if __dc_r_blk_1 == "" or __dc_r_blk_1 == "." %}{% if 3 > __dc_p_blk_1 %}{% if __dc_f_blk_1 != "" %}{% if "0123456789" contains __dc_f_blk_1 %}{% assign __dc_i_blk_1 = __dc_b_blk_1 | split: "." | first | size %}{% if 16 > __dc_i_blk_1 %}{% assign __dc_n_blk_1 = __dc_b_blk_1 | plus: 0 %}{% if __dc_c_blk_1 == "-" %}{% assign __dc_n_blk_1 = 0 | minus: __dc_n_blk_1 %}{% endif %}{% if 5 > __dc_n_blk_1 %}{% assign __dc_ok_blk_1 = true %}{% endif %}{% endif %}{% endif %}{% endif %}{% endif %}{% endif %}{% unless __dc_ok_blk_1 %}{% assign __dc_blk_1 = false %}{% endunless %}{% if __dc_blk_1 %}`,
		},
		{
			name:      "unknown operator is a false rule",
			condition: &DisplayCondition{Match: "all", Rules: []DisplayConditionRule{{Variable: "a", Operator: "between", Value: stringPtr("1")}}},
			open:      `{% assign __dc_blk_1 = true %}{% assign __dc_ok_blk_1 = false %}{% unless __dc_ok_blk_1 %}{% assign __dc_blk_1 = false %}{% endunless %}{% if __dc_blk_1 %}`,
		},
		{
			name:      "variable with Liquid in it is a false rule",
			condition: &DisplayCondition{Match: "all", Rules: []DisplayConditionRule{{Variable: "a %}{{ x", Operator: DisplayConditionIsPresent}}},
			open:      `{% assign __dc_blk_1 = true %}{% assign __dc_ok_blk_1 = false %}{% unless __dc_ok_blk_1 %}{% assign __dc_blk_1 = false %}{% endunless %}{% if __dc_blk_1 %}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			open, close := tc.condition.ToLiquid("blk-1")
			assert.Equal(t, tc.open, open)
			assert.Equal(t, "{% endif %}", close)
		})
	}

	t.Run("no condition and no rules write nothing", func(t *testing.T) {
		var c *DisplayCondition
		open, close := c.ToLiquid("x")
		assert.Empty(t, open+close)
		open, close = (&DisplayCondition{Match: "all"}).ToLiquid("x")
		assert.Empty(t, open+close)
	})
	t.Run("block id is sanitised and truncated", func(t *testing.T) {
		assert.Equal(t, "_a_b_c", exportSuffix("a-b.c"))
		assert.Equal(t, "_0123456789abcdef", exportSuffix("0123456789abcdefXYZ"))
		assert.Equal(t, "", exportSuffix(""))
	})
}

func TestValidateDisplayConditions(t *testing.T) {
	condition := `"display_condition":{"match":"all","rules":[{"variable":"contact.vip","operator":"is_true"}]}`
	block := func(id, typ string, extra string, children ...string) string {
		s := `{"id":"` + id + `","type":"` + typ + `"`
		if extra != "" {
			s += "," + extra
		}
		return s + `,"children":[` + strings.Join(children, ",") + `]}`
	}
	parse := func(t *testing.T, raw string) EmailBlock {
		t.Helper()
		tree, err := UnmarshalEmailBlock([]byte(raw))
		require.NoError(t, err)
		return tree
	}
	body := func(children ...string) string {
		return block("root", "mjml", "", block("body", "mj-body", "", children...))
	}

	t.Run("accepted on every eligible type under mj-body", func(t *testing.T) {
		for _, typ := range []string{"mj-wrapper", "mj-section", "mj-text", "mj-image", "mj-button", "mj-divider", "mj-spacer", "mj-social", "mj-raw"} {
			tree := parse(t, body(block("b1", typ, condition)))
			assert.NoError(t, ValidateDisplayConditions(tree), typ)
		}
	})
	t.Run("accepted deep in a wrapper", func(t *testing.T) {
		tree := parse(t, body(block("w", "mj-wrapper", "", block("s", "mj-section", "", block("c", "mj-column", "", block("t", "mj-text", condition))))))
		assert.NoError(t, ValidateDisplayConditions(tree))
	})
	t.Run("refused on ineligible types, naming the block", func(t *testing.T) {
		for _, typ := range []string{"mj-column", "mj-group", "mj-liquid", "mj-body"} {
			tree := parse(t, body(block("bad-1", typ, condition)))
			err := ValidateDisplayConditions(tree)
			require.Error(t, err, typ)
			assert.Contains(t, err.Error(), "bad-1")
			assert.Contains(t, err.Error(), typ)
		}
	})
	t.Run("refused under mj-head", func(t *testing.T) {
		head := block("root", "mjml", "",
			block("head", "mj-head", "", block("raw-1", "mj-raw", condition)),
			block("body", "mj-body", ""))
		err := ValidateDisplayConditions(parse(t, head))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "raw-1")

		attrs := block("root", "mjml", "",
			block("head", "mj-head", "", block("attrs", "mj-attributes", "", block("text-1", "mj-text", condition))),
			block("body", "mj-body", ""))
		err = ValidateDisplayConditions(parse(t, attrs))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "text-1")
	})
	t.Run("an invalid condition names the block", func(t *testing.T) {
		tree := parse(t, body(block("t-9", "mj-text", `"display_condition":{"match":"all","rules":[]}`)))
		err := ValidateDisplayConditions(tree)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "t-9")
	})
	t.Run("a saved block", func(t *testing.T) {
		assert.NoError(t, ValidateDisplayConditionsInBlock(parse(t, block("s", "mj-section", condition))))
		assert.Error(t, ValidateDisplayConditionsInBlock(parse(t, block("c", "mj-column", condition))))
	})
	t.Run("nil trees", func(t *testing.T) {
		assert.NoError(t, ValidateDisplayConditions(nil))
		var typedNil *MJTextBlock
		assert.NoError(t, ValidateDisplayConditions(typedNil))
	})
}

func BenchmarkDisplayConditionEvaluate(b *testing.B) {
	c := &DisplayCondition{Match: "all", Rules: []DisplayConditionRule{
		{Variable: "contact.custom_string_1", Operator: DisplayConditionEq, Value: stringPtr("vip")},
		{Variable: "contact.custom_number_1", Operator: DisplayConditionGte, Value: stringPtr("42")},
		{Variable: "contact.first_name", Operator: DisplayConditionIsPresent},
	}}
	data := map[string]interface{}{"contact": map[string]interface{}{"custom_string_1": "vip", "custom_number_1": 42.0, "first_name": "Ada"}}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := c.Evaluate(data); err != nil {
			b.Fatal(err)
		}
	}
}

// Sends evaluate conditions from many goroutines through the engine pool and its parse
// caches; run with -race.
func TestDisplayCondition_EvaluateConcurrently(t *testing.T) {
	c := &DisplayCondition{Match: "all", Rules: []DisplayConditionRule{
		{Variable: "contact.custom_number_1", Operator: DisplayConditionGt, Value: stringPtr("10")},
		{Variable: "contact.custom_string_1", Operator: DisplayConditionEq, Value: stringPtr("vip gold")},
	}}
	done := make(chan error, 16)
	for g := 0; g < 16; g++ {
		go func(g int) {
			for i := 0; i < 50; i++ {
				number := float64((g*50 + i) % 20)
				data := map[string]interface{}{"contact": map[string]interface{}{"custom_number_1": number, "custom_string_1": "vip gold"}}
				got, err := c.Evaluate(data)
				if err != nil {
					done <- err
					return
				}
				if got != (number > 10) {
					done <- fmt.Errorf("custom_number_1 %v: got %v", number, got)
					return
				}
			}
			done <- nil
		}(g)
	}
	for g := 0; g < 16; g++ {
		require.NoError(t, <-done)
	}
}
