package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResolveNestedKey(t *testing.T) {
	translations := map[string]interface{}{
		"welcome": map[string]interface{}{
			"heading":  "Welcome!",
			"greeting": "Hello {{ name }}!",
		},
		"cta": map[string]interface{}{
			"button": "Get Started",
		},
		"flat_key": "Flat value",
	}

	tests := []struct {
		name     string
		data     map[string]interface{}
		key      string
		expected string
	}{
		{"nested key", translations, "welcome.heading", "Welcome!"},
		{"deeper nested", translations, "welcome.greeting", "Hello {{ name }}!"},
		{"different group", translations, "cta.button", "Get Started"},
		{"flat key", translations, "flat_key", "Flat value"},
		{"missing key", translations, "welcome.missing", ""},
		{"missing group", translations, "nonexistent.key", ""},
		{"empty key", translations, "", ""},
		{"nil data", nil, "welcome.heading", ""},
		{"empty data", map[string]interface{}{}, "welcome.heading", ""},
		{"key pointing to map not string", translations, "welcome", ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := ResolveNestedKey(tc.data, tc.key)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestInterpolatePlaceholders(t *testing.T) {
	tests := []struct {
		name     string
		value    string
		args     map[string]interface{}
		expected string
	}{
		{
			"single placeholder",
			"Hello {{ name }}!",
			map[string]interface{}{"name": "John"},
			"Hello John!",
		},
		{
			"multiple placeholders",
			"{{ greeting }} {{ name }}, welcome to {{ site }}!",
			map[string]interface{}{"greeting": "Hello", "name": "Jane", "site": "Notifuse"},
			"Hello Jane, welcome to Notifuse!",
		},
		{
			"no placeholders",
			"Hello World!",
			map[string]interface{}{"name": "John"},
			"Hello World!",
		},
		{
			"placeholder without matching arg",
			"Hello {{ name }}!",
			map[string]interface{}{},
			"Hello {{ name }}!",
		},
		{
			"nil args",
			"Hello {{ name }}!",
			nil,
			"Hello {{ name }}!",
		},
		{
			"no spaces in placeholder",
			"Hello {{name}}!",
			map[string]interface{}{"name": "John"},
			"Hello John!",
		},
		{
			"extra spaces in placeholder",
			"Hello {{  name  }}!",
			map[string]interface{}{"name": "John"},
			"Hello John!",
		},
		{
			"numeric value",
			"You have {{ count }} items",
			map[string]interface{}{"count": 5},
			"You have 5 items",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := InterpolatePlaceholders(tc.value, tc.args)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestResolveLocale(t *testing.T) {
	tests := []struct {
		name             string
		contactLanguage  string
		availableLocales []string
		templateDefault  *string
		workspaceDefault string
		expected         string
	}{
		{
			"exact match",
			"fr",
			[]string{"en", "fr", "de"},
			nil,
			"en",
			"fr",
		},
		{
			"exact match with region",
			"pt-BR",
			[]string{"en", "pt-BR", "pt"},
			nil,
			"en",
			"pt-BR",
		},
		{
			"base language fallback",
			"pt-BR",
			[]string{"en", "pt"},
			nil,
			"en",
			"pt",
		},
		{
			"template default fallback",
			"ja",
			[]string{"en", "fr"},
			strPtr("fr"),
			"en",
			"fr",
		},
		{
			"workspace default fallback",
			"ja",
			[]string{"en", "fr"},
			nil,
			"en",
			"en",
		},
		{
			"empty contact language uses workspace default",
			"",
			[]string{"en", "fr"},
			nil,
			"en",
			"en",
		},
		{
			"case insensitive match",
			"FR",
			[]string{"en", "fr"},
			nil,
			"en",
			"fr",
		},
		{
			"workspace default when no locales available",
			"fr",
			[]string{},
			nil,
			"en",
			"en",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := ResolveLocale(tc.contactLanguage, tc.availableLocales, tc.templateDefault, tc.workspaceDefault)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func strPtr(s string) *string {
	return &s
}

func TestMergeTranslations(t *testing.T) {
	tests := []struct {
		name     string
		base     map[string]interface{}
		override map[string]interface{}
		expected map[string]interface{}
	}{
		{
			"override wins",
			map[string]interface{}{"welcome": map[string]interface{}{"heading": "Base"}},
			map[string]interface{}{"welcome": map[string]interface{}{"heading": "Override"}},
			map[string]interface{}{"welcome": map[string]interface{}{"heading": "Override"}},
		},
		{
			"deep merge adds missing keys",
			map[string]interface{}{"welcome": map[string]interface{}{"heading": "Base"}},
			map[string]interface{}{"welcome": map[string]interface{}{"body": "Override body"}},
			map[string]interface{}{"welcome": map[string]interface{}{"heading": "Base", "body": "Override body"}},
		},
		{
			"nil base",
			nil,
			map[string]interface{}{"key": "value"},
			map[string]interface{}{"key": "value"},
		},
		{
			"nil override",
			map[string]interface{}{"key": "value"},
			nil,
			map[string]interface{}{"key": "value"},
		},
		{
			"both nil",
			nil,
			nil,
			map[string]interface{}{},
		},
		{
			"disjoint keys",
			map[string]interface{}{"a": "1"},
			map[string]interface{}{"b": "2"},
			map[string]interface{}{"a": "1", "b": "2"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := MergeTranslations(tc.base, tc.override)
			assert.Equal(t, tc.expected, result)
		})
	}
}
