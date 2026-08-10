package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func strPtr(s string) *string { return &s }

func TestEvaluateWebFilterCondition(t *testing.T) {
	fields := map[string]string{
		"utm_source":      "google",
		"referrer_domain": "news.ycombinator.com",
		"utm_medium":      "",
	}

	cases := []struct {
		name string
		cond WebFilterCondition
		want bool
	}{
		{"equals match", WebFilterCondition{Field: "utm_source", Operator: WebFilterOpEquals, Value: "google"}, true},
		{"equals mismatch", WebFilterCondition{Field: "utm_source", Operator: WebFilterOpEquals, Value: "bing"}, false},
		{"not_equals", WebFilterCondition{Field: "utm_source", Operator: WebFilterOpNotEquals, Value: "bing"}, true},
		{"contains", WebFilterCondition{Field: "referrer_domain", Operator: WebFilterOpContains, Value: "ycombinator"}, true},
		{"not_contains", WebFilterCondition{Field: "referrer_domain", Operator: WebFilterOpNotContains, Value: "reddit"}, true},
		{"regex match", WebFilterCondition{Field: "utm_source", Operator: WebFilterOpRegex, Value: "^goo.*$"}, true},
		{"regex mismatch", WebFilterCondition{Field: "utm_source", Operator: WebFilterOpRegex, Value: "^bing$"}, false},
		{"invalid regex never matches", WebFilterCondition{Field: "utm_source", Operator: WebFilterOpRegex, Value: "("}, false},
		{"is_not_empty on set field", WebFilterCondition{Field: "utm_source", Operator: WebFilterOpIsNotEmpty}, true},
		{"is_empty on set field", WebFilterCondition{Field: "utm_source", Operator: WebFilterOpIsEmpty}, false},

		// Empty string and missing fields only match is_empty.
		{"is_empty on empty string", WebFilterCondition{Field: "utm_medium", Operator: WebFilterOpIsEmpty}, true},
		{"equals on empty string", WebFilterCondition{Field: "utm_medium", Operator: WebFilterOpEquals, Value: ""}, false},
		{"not_equals on empty string", WebFilterCondition{Field: "utm_medium", Operator: WebFilterOpNotEquals, Value: "x"}, false},
		{"is_empty on missing field", WebFilterCondition{Field: "utm_campaign", Operator: WebFilterOpIsEmpty}, true},
		{"is_not_empty on missing field", WebFilterCondition{Field: "utm_campaign", Operator: WebFilterOpIsNotEmpty}, false},
		{"contains on missing field", WebFilterCondition{Field: "utm_campaign", Operator: WebFilterOpContains, Value: "x"}, false},
		{"unknown operator", WebFilterCondition{Field: "utm_source", Operator: "like"}, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, EvaluateWebFilterCondition(tc.cond, fields))
		})
	}
}

func TestEvaluateWebFilters(t *testing.T) {
	setChannel := func(id, name string, priority int, conditions []WebFilterCondition, value string) WebFilter {
		return WebFilter{
			ID: id, Name: name, Priority: priority, Enabled: true,
			Conditions: conditions,
			Operations: []WebFilterOperation{{Dimension: "channel", Action: WebFilterActionSetValue, Value: value}},
		}
	}

	t.Run("higher priority wins regardless of slice order", func(t *testing.T) {
		filters := []WebFilter{
			setChannel("f1", "low", 100, nil, "low-value"),
			setChannel("f2", "high", 900, nil, "high-value"),
		}
		result := EvaluateWebFilters(filters, map[string]string{})
		require.Contains(t, result, "channel")
		assert.Equal(t, strPtr("high-value"), result["channel"])
	})

	t.Run("empty conditions always match, AND semantics otherwise", func(t *testing.T) {
		filters := []WebFilter{
			setChannel("f1", "both", 500, []WebFilterCondition{
				{Field: "utm_source", Operator: WebFilterOpEquals, Value: "google"},
				{Field: "utm_medium", Operator: WebFilterOpEquals, Value: "cpc"},
			}, "google-ads"),
		}
		match := EvaluateWebFilters(filters, map[string]string{"utm_source": "google", "utm_medium": "cpc"})
		assert.Equal(t, strPtr("google-ads"), match["channel"])

		noMatch := EvaluateWebFilters(filters, map[string]string{"utm_source": "google"})
		assert.NotContains(t, noMatch, "channel")
	})

	t.Run("disabled filters are skipped", func(t *testing.T) {
		f := setChannel("f1", "off", 900, nil, "x")
		f.Enabled = false
		result := EvaluateWebFilters([]WebFilter{f}, map[string]string{})
		assert.Empty(t, result)
	})

	t.Run("equal priorities: later rule in evaluation order overwrites", func(t *testing.T) {
		filters := []WebFilter{
			setChannel("f1", "first", 500, nil, "first"),
			setChannel("f2", "second", 500, nil, "second"),
		}
		result := EvaluateWebFilters(filters, map[string]string{})
		// The first evaluated sets the value; the second has equal (not lower)
		// priority so it overwrites — Staminads parity: `<` comparison lets
		// equal priority through, and the later rule in evaluation order wins.
		assert.Equal(t, strPtr("second"), result["channel"])
	})

	t.Run("unset_value produces explicit null", func(t *testing.T) {
		filters := []WebFilter{{
			ID: "f1", Name: "unset", Priority: 800, Enabled: true,
			Operations: []WebFilterOperation{{Dimension: "custom_3", Action: WebFilterActionUnsetValue}},
		}}
		result := EvaluateWebFilters(filters, map[string]string{})
		require.Contains(t, result, "custom_3")
		assert.Nil(t, result["custom_3"])
	})

	t.Run("set_default_value only fills unset or null dimensions", func(t *testing.T) {
		defaultRule := WebFilter{
			ID: "fallback", Name: "fallback", Priority: 10, Enabled: true,
			Operations: []WebFilterOperation{{Dimension: "channel", Action: WebFilterActionSetDefaultValue, Value: "not-mapped"}},
		}

		// Nothing set → default applies.
		result := EvaluateWebFilters([]WebFilter{defaultRule}, map[string]string{})
		assert.Equal(t, strPtr("not-mapped"), result["channel"])

		// Higher-priority set_value ran first → default does not overwrite.
		result = EvaluateWebFilters([]WebFilter{
			setChannel("f1", "real", 900, nil, "google-ads"),
			defaultRule,
		}, map[string]string{})
		assert.Equal(t, strPtr("google-ads"), result["channel"])

		// Explicit unset (null) → default fills it back.
		result = EvaluateWebFilters([]WebFilter{
			{ID: "f1", Name: "unset", Priority: 900, Enabled: true,
				Operations: []WebFilterOperation{{Dimension: "channel", Action: WebFilterActionUnsetValue}}},
			defaultRule,
		}, map[string]string{})
		assert.Equal(t, strPtr("not-mapped"), result["channel"])
	})

	t.Run("default does not claim priority: later set_value still overrides", func(t *testing.T) {
		result := EvaluateWebFilters([]WebFilter{
			{ID: "d", Name: "default-first", Priority: 950, Enabled: true,
				Operations: []WebFilterOperation{{Dimension: "channel", Action: WebFilterActionSetDefaultValue, Value: "early-default"}}},
			setChannel("f1", "later", 100, nil, "real-value"),
		}, map[string]string{})
		assert.Equal(t, strPtr("real-value"), result["channel"])
	})

	t.Run("lower priority cannot overwrite a set dimension", func(t *testing.T) {
		result := EvaluateWebFilters([]WebFilter{
			setChannel("hi", "hi", 900, nil, "keep-me"),
			setChannel("lo", "lo", 100, nil, "discard"),
		}, map[string]string{})
		assert.Equal(t, strPtr("keep-me"), result["channel"])
	})
}

func TestComputeWebFiltersVersion(t *testing.T) {
	f1 := WebFilter{ID: "a", Priority: 100, Enabled: true,
		Conditions: []WebFilterCondition{{Field: "utm_source", Operator: WebFilterOpEquals, Value: "x"}},
		Operations: []WebFilterOperation{{Dimension: "channel", Action: WebFilterActionSetValue, Value: "y"}}}
	f2 := WebFilter{ID: "b", Priority: 200, Enabled: true,
		Operations: []WebFilterOperation{{Dimension: "channel", Action: WebFilterActionSetValue, Value: "z"}}}

	t.Run("stable across slice ordering", func(t *testing.T) {
		v1 := ComputeWebFiltersVersion([]WebFilter{f1, f2})
		v2 := ComputeWebFiltersVersion([]WebFilter{f2, f1})
		assert.Equal(t, v1, v2)
		assert.Len(t, v1, 8)
	})

	t.Run("ignores cosmetic fields, changes on semantic fields", func(t *testing.T) {
		base := ComputeWebFiltersVersion([]WebFilter{f1})

		renamed := f1
		renamed.Name = "renamed"
		renamed.Order = 42
		renamed.Version = "stale"
		assert.Equal(t, base, ComputeWebFiltersVersion([]WebFilter{renamed}))

		reprioritized := f1
		reprioritized.Priority = 101
		assert.NotEqual(t, base, ComputeWebFiltersVersion([]WebFilter{reprioritized}))

		disabled := f1
		disabled.Enabled = false
		assert.NotEqual(t, base, ComputeWebFiltersVersion([]WebFilter{disabled}))
	})
}

func TestWebFilterValidate(t *testing.T) {
	valid := WebFilter{
		ID: "f1", Name: "rule", Priority: 500, Enabled: true,
		Conditions: []WebFilterCondition{{Field: "utm_source", Operator: WebFilterOpRegex, Value: "^google$"}},
		Operations: []WebFilterOperation{{Dimension: "channel", Action: WebFilterActionSetValue, Value: "x"}},
	}
	assert.NoError(t, valid.Validate())

	t.Run("rejections", func(t *testing.T) {
		f := valid
		f.ID = ""
		assert.ErrorContains(t, f.Validate(), "id")

		f = valid
		f.Name = " "
		assert.ErrorContains(t, f.Validate(), "name")

		f = valid
		f.Priority = 1001
		assert.ErrorContains(t, f.Validate(), "priority")

		f = valid
		f.Conditions = []WebFilterCondition{{Field: "not_a_field", Operator: WebFilterOpEquals, Value: "x"}}
		assert.ErrorContains(t, f.Validate(), "source field")

		f = valid
		f.Conditions = []WebFilterCondition{{Field: "utm_source", Operator: "like", Value: "x"}}
		assert.ErrorContains(t, f.Validate(), "operator")

		f = valid
		f.Conditions = []WebFilterCondition{{Field: "utm_source", Operator: WebFilterOpRegex, Value: "("}}
		assert.ErrorContains(t, f.Validate(), "compile")

		f = valid
		f.Operations = nil
		assert.ErrorContains(t, f.Validate(), "operation")

		f = valid
		f.Operations = []WebFilterOperation{{Dimension: "utm_id", Action: WebFilterActionSetValue, Value: "x"}}
		assert.ErrorContains(t, f.Validate(), "not writable")

		f = valid
		f.Operations = []WebFilterOperation{{Dimension: "channel", Action: WebFilterActionSetValue}}
		assert.ErrorContains(t, f.Validate(), "requires a value")

		f = valid
		f.Operations = []WebFilterOperation{{Dimension: "channel", Action: "delete"}}
		assert.ErrorContains(t, f.Validate(), "action")
	})

	t.Run("unset_value needs no value", func(t *testing.T) {
		f := valid
		f.Operations = []WebFilterOperation{{Dimension: "custom_1", Action: WebFilterActionUnsetValue}}
		assert.NoError(t, f.Validate())
	})
}
