# Template i18n Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add template-level i18n using a Liquid `t` filter so emails are automatically sent in the contact's language.

**Architecture:** Translation keys (`{{ "key" | t }}`) stored as nested JSON per locale in the template's `translations` JSONB column. Workspace-level shared translations in a new `workspace_translations` table. A custom Liquid filter resolves keys at render time with a locale fallback chain (contact.language → base language → template default → workspace default). No changes to the transactional API — language selection is automatic.

**Tech Stack:** Go 1.25 (backend), liquidgo (Liquid engine), PostgreSQL JSONB, React 18 + Ant Design + TypeScript (frontend), Vitest (frontend tests), Go standard testing + testify + gomock (backend tests).

**Design doc:** `docs/plans/2026-02-24-template-i18n-design.md`

---

## Task 1: Translation Utility Functions (Domain Layer)

Core helper functions for locale resolution, nested key lookup, placeholder interpolation, and translation merging. These are pure functions with no dependencies — the foundation for everything else.

**Files:**
- Create: `internal/domain/translation.go`
- Create: `internal/domain/translation_test.go`

### Step 1: Write failing tests for `ResolveNestedKey`

This function traverses a nested `map[string]interface{}` using a dot-separated key path and returns the string value.

```go
// internal/domain/translation_test.go
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
```

### Step 2: Run tests to verify they fail

Run: `cd /var/www/forks/notifuse && go test ./internal/domain/ -run TestResolveNestedKey -v`
Expected: Compilation error — `ResolveNestedKey` undefined.

### Step 3: Implement `ResolveNestedKey`

```go
// internal/domain/translation.go
package domain

import (
	"strings"
)

// ResolveNestedKey traverses a nested map using a dot-separated key path
// and returns the string value. Returns empty string if key not found or value is not a string.
func ResolveNestedKey(data map[string]interface{}, key string) string {
	if data == nil || key == "" {
		return ""
	}

	parts := strings.Split(key, ".")
	var current interface{} = data

	for _, part := range parts {
		m, ok := current.(map[string]interface{})
		if !ok {
			return ""
		}
		current, ok = m[part]
		if !ok {
			return ""
		}
	}

	if str, ok := current.(string); ok {
		return str
	}
	return ""
}
```

### Step 4: Run tests to verify they pass

Run: `cd /var/www/forks/notifuse && go test ./internal/domain/ -run TestResolveNestedKey -v`
Expected: All PASS.

### Step 5: Write failing tests for `InterpolatePlaceholders`

This function replaces `{{ name }}` style placeholders in a translation value with provided key-value arguments.

```go
// Append to internal/domain/translation_test.go

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
```

### Step 6: Run tests to verify they fail

Run: `cd /var/www/forks/notifuse && go test ./internal/domain/ -run TestInterpolatePlaceholders -v`
Expected: Compilation error.

### Step 7: Implement `InterpolatePlaceholders`

```go
// Append to internal/domain/translation.go

import (
	"fmt"
	"regexp"
	"strings"
)

var placeholderRegex = regexp.MustCompile(`\{\{\s*(\w+)\s*\}\}`)

// InterpolatePlaceholders replaces {{ key }} placeholders in a translation value
// with the corresponding values from the args map.
func InterpolatePlaceholders(value string, args map[string]interface{}) string {
	if args == nil || len(args) == 0 {
		return value
	}

	return placeholderRegex.ReplaceAllStringFunc(value, func(match string) string {
		submatch := placeholderRegex.FindStringSubmatch(match)
		if len(submatch) < 2 {
			return match
		}
		key := submatch[1]
		if val, ok := args[key]; ok {
			return fmt.Sprintf("%v", val)
		}
		return match // leave unresolved placeholders as-is
	})
}
```

### Step 8: Run tests to verify they pass

Run: `cd /var/www/forks/notifuse && go test ./internal/domain/ -run TestInterpolatePlaceholders -v`
Expected: All PASS.

### Step 9: Write failing tests for `ResolveLocale`

The locale fallback chain: exact match → base language → template default → workspace default.

```go
// Append to internal/domain/translation_test.go

func TestResolveLocale(t *testing.T) {
	tests := []struct {
		name              string
		contactLanguage   string
		availableLocales  []string
		templateDefault   *string
		workspaceDefault  string
		expected          string
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
```

### Step 10: Run tests to verify they fail

Run: `cd /var/www/forks/notifuse && go test ./internal/domain/ -run TestResolveLocale -v`
Expected: Compilation error.

### Step 11: Implement `ResolveLocale`

```go
// Append to internal/domain/translation.go

// ResolveLocale determines the best locale to use given a contact's language preference,
// available translation locales, and fallback defaults.
// Fallback chain: exact match → base language → template default → workspace default.
func ResolveLocale(contactLanguage string, availableLocales []string, templateDefault *string, workspaceDefault string) string {
	if contactLanguage == "" {
		if templateDefault != nil && *templateDefault != "" {
			return *templateDefault
		}
		return workspaceDefault
	}

	contactLang := strings.ToLower(contactLanguage)

	// 1. Exact match (case-insensitive)
	for _, locale := range availableLocales {
		if strings.ToLower(locale) == contactLang {
			return locale
		}
	}

	// 2. Base language match (e.g., "pt-BR" → "pt")
	if idx := strings.Index(contactLang, "-"); idx > 0 {
		baseLang := contactLang[:idx]
		for _, locale := range availableLocales {
			if strings.ToLower(locale) == baseLang {
				return locale
			}
		}
	}

	// 3. Template default language
	if templateDefault != nil && *templateDefault != "" {
		return *templateDefault
	}

	// 4. Workspace default language
	return workspaceDefault
}
```

### Step 12: Run tests to verify they pass

Run: `cd /var/www/forks/notifuse && go test ./internal/domain/ -run TestResolveLocale -v`
Expected: All PASS.

### Step 13: Write failing tests for `MergeTranslations`

Deep-merges two translation maps. Template translations take priority over workspace translations.

```go
// Append to internal/domain/translation_test.go

func TestMergeTranslations(t *testing.T) {
	tests := []struct {
		name      string
		base      map[string]interface{}
		override  map[string]interface{}
		expected  map[string]interface{}
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
```

### Step 14: Run tests, verify fail, implement, verify pass

Run: `cd /var/www/forks/notifuse && go test ./internal/domain/ -run TestMergeTranslations -v`

```go
// Append to internal/domain/translation.go

// MergeTranslations deep-merges two translation maps. Values in override take priority.
func MergeTranslations(base, override map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})

	// Copy base
	for k, v := range base {
		result[k] = v
	}

	// Merge override
	for k, v := range override {
		if baseVal, exists := result[k]; exists {
			// If both are maps, deep merge
			baseMap, baseIsMap := baseVal.(map[string]interface{})
			overrideMap, overrideIsMap := v.(map[string]interface{})
			if baseIsMap && overrideIsMap {
				result[k] = MergeTranslations(baseMap, overrideMap)
				continue
			}
		}
		result[k] = v
	}

	return result
}
```

Run: `cd /var/www/forks/notifuse && go test ./internal/domain/ -run TestMergeTranslations -v`
Expected: All PASS.

### Step 15: Commit

```bash
git add internal/domain/translation.go internal/domain/translation_test.go
git commit -m "feat(i18n): add translation utility functions

Locale resolution, nested key lookup, placeholder interpolation,
and translation merging — pure functions with full test coverage."
```

---

## Task 2: Liquid `t` Filter

Register a custom `T` filter on the `SecureLiquidEngine` that resolves translation keys during Liquid rendering.

**Files:**
- Create: `pkg/notifuse_mjml/translation_filter.go`
- Create: `pkg/notifuse_mjml/translation_filter_test.go`
- Modify: `pkg/notifuse_mjml/liquid_secure.go` (add `RegisterTranslations` method)

### Step 1: Write failing tests for the translation filter

```go
// pkg/notifuse_mjml/translation_filter_test.go
package notifuse_mjml

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTranslationFilter_SimpleKey(t *testing.T) {
	engine := NewSecureLiquidEngine()
	translations := map[string]interface{}{
		"welcome": map[string]interface{}{
			"heading": "Welcome!",
		},
	}
	engine.RegisterTranslations(translations)

	result, err := engine.Render(`{{ "welcome.heading" | t }}`, map[string]interface{}{})
	require.NoError(t, err)
	assert.Equal(t, "Welcome!", result)
}

func TestTranslationFilter_MissingKey(t *testing.T) {
	engine := NewSecureLiquidEngine()
	translations := map[string]interface{}{}
	engine.RegisterTranslations(translations)

	result, err := engine.Render(`{{ "missing.key" | t }}`, map[string]interface{}{})
	require.NoError(t, err)
	assert.Equal(t, "[Missing translation: missing.key]", result)
}

func TestTranslationFilter_WithPlaceholders(t *testing.T) {
	engine := NewSecureLiquidEngine()
	translations := map[string]interface{}{
		"welcome": map[string]interface{}{
			"greeting": "Hello {{ name }}, welcome to {{ site }}!",
		},
	}
	engine.RegisterTranslations(translations)

	// The liquidgo filter receives named keyword args as a map
	result, err := engine.Render(
		`{{ "welcome.greeting" | t: name: "John", site: "Notifuse" }}`,
		map[string]interface{}{},
	)
	require.NoError(t, err)
	assert.Equal(t, "Hello John, welcome to Notifuse!", result)
}

func TestTranslationFilter_WithContactVariable(t *testing.T) {
	engine := NewSecureLiquidEngine()
	translations := map[string]interface{}{
		"welcome": map[string]interface{}{
			"greeting": "Hello {{ name }}!",
		},
	}
	engine.RegisterTranslations(translations)

	result, err := engine.Render(
		`{{ "welcome.greeting" | t: name: contact.first_name }}`,
		map[string]interface{}{
			"contact": map[string]interface{}{
				"first_name": "Alice",
			},
		},
	)
	require.NoError(t, err)
	assert.Equal(t, "Hello Alice!", result)
}

func TestTranslationFilter_FlatKey(t *testing.T) {
	engine := NewSecureLiquidEngine()
	translations := map[string]interface{}{
		"flat_key": "Flat value",
	}
	engine.RegisterTranslations(translations)

	result, err := engine.Render(`{{ "flat_key" | t }}`, map[string]interface{}{})
	require.NoError(t, err)
	assert.Equal(t, "Flat value", result)
}

func TestTranslationFilter_NoRegistration(t *testing.T) {
	// When no translations registered, t filter should return missing translation marker
	engine := NewSecureLiquidEngine()

	result, err := engine.Render(`{{ "some.key" | t }}`, map[string]interface{}{})
	require.NoError(t, err)
	assert.Equal(t, "[Missing translation: some.key]", result)
}
```

### Step 2: Run tests to verify they fail

Run: `cd /var/www/forks/notifuse && go test ./pkg/notifuse_mjml/ -run TestTranslationFilter -v`
Expected: Compilation error.

### Step 3: Implement the translation filter

```go
// pkg/notifuse_mjml/translation_filter.go
package notifuse_mjml

import (
	"fmt"

	"github.com/Notifuse/notifuse/internal/domain"
)

// TranslationFilters provides the Liquid `t` filter for resolving translation keys.
// Register with SecureLiquidEngine.RegisterTranslations().
type TranslationFilters struct {
	translations map[string]interface{}
}

// T is the Liquid filter: {{ "welcome.heading" | t }}
// With placeholders: {{ "welcome.greeting" | t: name: "John" }}
//
// liquidgo calls this method with:
//   - input: the piped value (the translation key string)
//   - args: variadic positional args (unused for now)
//
// liquidgo passes keyword args (name: value) as the last element
// in args if it's a map[string]interface{}.
func (tf *TranslationFilters) T(input interface{}, args ...interface{}) interface{} {
	keyStr := fmt.Sprintf("%v", input)

	value := domain.ResolveNestedKey(tf.translations, keyStr)
	if value == "" {
		return "[Missing translation: " + keyStr + "]"
	}

	// Check if last arg is a keyword args map
	var kwargs map[string]interface{}
	if len(args) > 0 {
		if m, ok := args[len(args)-1].(map[string]interface{}); ok {
			kwargs = m
		}
	}

	if len(kwargs) > 0 {
		value = domain.InterpolatePlaceholders(value, kwargs)
	}

	return value
}
```

### Step 4: Add `RegisterTranslations` to `SecureLiquidEngine`

Modify `pkg/notifuse_mjml/liquid_secure.go`. Add this method after the existing methods:

```go
// RegisterTranslations registers translation data for the Liquid t filter.
// Must be called before Render. Translations should be a merged map (template + workspace).
func (s *SecureLiquidEngine) RegisterTranslations(translations map[string]interface{}) {
	if translations == nil {
		translations = map[string]interface{}{}
	}
	filter := &TranslationFilters{translations: translations}
	s.env.RegisterFilter(filter)
}
```

### Step 5: Run tests to verify they pass

Run: `cd /var/www/forks/notifuse && go test ./pkg/notifuse_mjml/ -run TestTranslationFilter -v`
Expected: All PASS. (Note: the keyword args test may need adjustment based on how liquidgo passes them — see Step 6.)

### Step 6: Debug and fix keyword args if needed

liquidgo's filter invocation passes keyword args differently depending on the parsing mode. Check how they arrive in the `T` method by adding a temporary debug log. The `laxParseFilterExpressions` function in `liquidgo/liquid/variable.go:272` shows: `result = []interface{}{filterName, filterArgs}` where `keywordArgs` is appended as element [2] if present. At invocation time (`variable.go:360-390`), positional args are passed as separate params and keyword args as the final map. Adjust the `T` method signature if the args arrive differently.

### Step 7: Run all existing Liquid tests to verify no regressions

Run: `cd /var/www/forks/notifuse && go test ./pkg/notifuse_mjml/ -v`
Expected: All existing tests still pass.

### Step 8: Commit

```bash
git add pkg/notifuse_mjml/translation_filter.go pkg/notifuse_mjml/translation_filter_test.go pkg/notifuse_mjml/liquid_secure.go
git commit -m "feat(i18n): add Liquid t filter for translation key resolution

Registers a TranslationFilters struct on the Liquid engine that resolves
nested keys with {{ \"key\" | t }} syntax and supports placeholders
via named args: {{ \"key\" | t: name: contact.first_name }}"
```

---

## Task 3: Domain Model Changes

Add `Translations` and `DefaultLanguage` fields to the `Template` struct, and `DefaultLanguage`/`SupportedLanguages` to `WorkspaceSettings`. Add `WorkspaceTranslation` entity.

**Files:**
- Modify: `internal/domain/template.go` (Template struct, Validate, scan helpers, request types)
- Modify: `internal/domain/template_test.go` (update tests)
- Modify: `internal/domain/workspace.go` (WorkspaceSettings struct)
- Create: `internal/domain/workspace_translation.go`
- Create: `internal/domain/workspace_translation_test.go`

### Step 1: Add fields to `Template` struct

In `internal/domain/template.go`, add two fields to the `Template` struct (after `Settings`):

```go
type Template struct {
	// ... existing fields through Settings ...
	Settings        MapOfAny       `json:"settings"`
	Translations    MapOfInterfaces `json:"translations"`     // locale → nested key-value map
	DefaultLanguage *string        `json:"default_language"`  // overrides workspace default if set
	CreatedAt       time.Time      `json:"created_at"`
	// ... rest unchanged ...
}
```

Note: `Translations` needs a custom type that implements `sql.Scanner` and `driver.Valuer` for JSONB storage. Define `MapOfInterfaces` as `map[string]map[string]interface{}` with scanner methods, or reuse the existing `MapOfAny` pattern and cast at usage sites. The simplest approach: store as `MapOfAny` (which is `map[string]interface{}` with JSONB scan/value support already implemented) and cast the inner values at read time.

Actually, the cleanest approach is to store `Translations` as `MapOfAny` since JSONB deserialization produces `map[string]interface{}` naturally:

```go
Translations    MapOfAny  `json:"translations"`     // {locale: {nested key-value}}
DefaultLanguage *string   `json:"default_language"`  // overrides workspace default
```

### Step 2: Add `Translations` to `EmailTemplate` scan/serialization

The `Translations` field uses `MapOfAny` which already has `Scan()` and `Value()` methods. The `DefaultLanguage` is a nullable `*string` which maps to `sql.NullString` in the scanner.

### Step 3: Update `Template.Validate()` to validate translations

Add validation in the `Validate()` method:

```go
// Validate translations if provided
if w.Translations != nil {
	for locale, content := range w.Translations {
		if locale == "" {
			return fmt.Errorf("invalid template: translation locale cannot be empty")
		}
		if len(locale) > 10 {
			return fmt.Errorf("invalid template: translation locale '%s' exceeds max length of 10", locale)
		}
		if content == nil {
			return fmt.Errorf("invalid template: translation content for locale '%s' cannot be nil", locale)
		}
	}
}

// Validate default_language if set
if w.DefaultLanguage != nil && *w.DefaultLanguage != "" {
	if len(*w.DefaultLanguage) > 10 {
		return fmt.Errorf("invalid template: default_language exceeds max length of 10")
	}
}
```

### Step 4: Update `CreateTemplateRequest` and `UpdateTemplateRequest`

These request types include the `Template` field, so `Translations` and `DefaultLanguage` flow through automatically via JSON deserialization. No changes needed to request types.

### Step 5: Add language fields to `WorkspaceSettings`

In `internal/domain/workspace.go`, add to the `WorkspaceSettings` struct (after `BlogSettings`):

```go
type WorkspaceSettings struct {
	// ... existing fields ...
	BlogSettings     *BlogSettings   `json:"blog_settings,omitempty"`
	DefaultLanguage  string          `json:"default_language,omitempty"`  // e.g., "en"
	SupportedLanguages []string      `json:"supported_languages,omitempty"` // e.g., ["en", "fr", "de"]

	// decoded secret key, not stored in the database
	SecretKey string `json:"-"`
}
```

Since `WorkspaceSettings` is stored as JSONB in the `workspaces` table, existing workspaces will have these fields absent in JSON. Go will deserialize them as zero values (`""` and `nil`). Add a helper to get the effective default language:

```go
// GetDefaultLanguage returns the workspace's default language, defaulting to "en" if not set.
func (ws *WorkspaceSettings) GetDefaultLanguage() string {
	if ws.DefaultLanguage != "" {
		return ws.DefaultLanguage
	}
	return "en"
}

// GetSupportedLanguages returns the workspace's supported languages, defaulting to ["en"] if not set.
func (ws *WorkspaceSettings) GetSupportedLanguages() []string {
	if len(ws.SupportedLanguages) > 0 {
		return ws.SupportedLanguages
	}
	return []string{"en"}
}
```

### Step 6: Create `WorkspaceTranslation` entity

```go
// internal/domain/workspace_translation.go
package domain

import (
	"context"
	"fmt"
	"time"
)

// WorkspaceTranslation represents translations for a single locale at the workspace level.
type WorkspaceTranslation struct {
	Locale    string   `json:"locale"`
	Content   MapOfAny `json:"content"` // nested key-value translation map
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Validate validates the workspace translation.
func (wt *WorkspaceTranslation) Validate() error {
	if wt.Locale == "" {
		return fmt.Errorf("locale is required")
	}
	if len(wt.Locale) > 10 {
		return fmt.Errorf("locale exceeds max length of 10")
	}
	if wt.Content == nil {
		return fmt.Errorf("content is required")
	}
	return nil
}

// WorkspaceTranslationRepository defines the data access interface for workspace translations.
type WorkspaceTranslationRepository interface {
	Upsert(ctx context.Context, workspaceID string, translation *WorkspaceTranslation) error
	GetByLocale(ctx context.Context, workspaceID string, locale string) (*WorkspaceTranslation, error)
	List(ctx context.Context, workspaceID string) ([]*WorkspaceTranslation, error)
	Delete(ctx context.Context, workspaceID string, locale string) error
}

// Request/Response types for workspace translations API
type UpsertWorkspaceTranslationRequest struct {
	WorkspaceID string   `json:"workspace_id"`
	Locale      string   `json:"locale"`
	Content     MapOfAny `json:"content"`
}

func (r *UpsertWorkspaceTranslationRequest) Validate() error {
	if r.WorkspaceID == "" {
		return fmt.Errorf("workspace_id is required")
	}
	if r.Locale == "" {
		return fmt.Errorf("locale is required")
	}
	if len(r.Locale) > 10 {
		return fmt.Errorf("locale exceeds max length of 10")
	}
	if r.Content == nil {
		return fmt.Errorf("content is required")
	}
	return nil
}

type ListWorkspaceTranslationsRequest struct {
	WorkspaceID string `json:"workspace_id"`
}

type DeleteWorkspaceTranslationRequest struct {
	WorkspaceID string `json:"workspace_id"`
	Locale      string `json:"locale"`
}
```

### Step 7: Write tests for new domain types

```go
// internal/domain/workspace_translation_test.go
package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWorkspaceTranslation_Validate(t *testing.T) {
	tests := []struct {
		name      string
		wt        WorkspaceTranslation
		expectErr bool
	}{
		{"valid", WorkspaceTranslation{Locale: "en", Content: MapOfAny{"key": "value"}}, false},
		{"empty locale", WorkspaceTranslation{Locale: "", Content: MapOfAny{"key": "value"}}, true},
		{"locale too long", WorkspaceTranslation{Locale: "12345678901", Content: MapOfAny{"key": "value"}}, true},
		{"nil content", WorkspaceTranslation{Locale: "en", Content: nil}, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.wt.Validate()
			if tc.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestWorkspaceSettings_GetDefaultLanguage(t *testing.T) {
	ws := &WorkspaceSettings{}
	assert.Equal(t, "en", ws.GetDefaultLanguage())

	ws.DefaultLanguage = "fr"
	assert.Equal(t, "fr", ws.GetDefaultLanguage())
}

func TestWorkspaceSettings_GetSupportedLanguages(t *testing.T) {
	ws := &WorkspaceSettings{}
	assert.Equal(t, []string{"en"}, ws.GetSupportedLanguages())

	ws.SupportedLanguages = []string{"en", "fr", "de"}
	assert.Equal(t, []string{"en", "fr", "de"}, ws.GetSupportedLanguages())
}
```

### Step 8: Run all domain tests

Run: `cd /var/www/forks/notifuse && go test ./internal/domain/ -v`
Expected: All PASS (new and existing tests).

### Step 9: Commit

```bash
git add internal/domain/translation.go internal/domain/translation_test.go internal/domain/template.go internal/domain/template_test.go internal/domain/workspace.go internal/domain/workspace_translation.go internal/domain/workspace_translation_test.go
git commit -m "feat(i18n): add translation fields to domain models

Template: translations (JSONB) + default_language.
WorkspaceSettings: default_language + supported_languages.
New WorkspaceTranslation entity with repository interface."
```

---

## Task 4: Database Migration V28

Add `translations` and `default_language` columns to the workspace `templates` table. Create `workspace_translations` table. No system database changes needed (workspace language settings are in the existing JSONB `settings` column).

**Files:**
- Create: `internal/migrations/v28.go`
- Create: `internal/migrations/v28_test.go`
- Modify: `config/config.go` (bump VERSION to "28.0")
- Modify: `internal/database/schema/` (update workspace table schema for new installs)

### Step 1: Create V28 migration

```go
// internal/migrations/v28.go
package migrations

import (
	"context"
	"fmt"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
)

// V28Migration adds template i18n support.
//
// This migration adds:
// - translations: JSONB column on templates for per-locale translation key-value maps
// - default_language: VARCHAR column on templates for per-template language override
// - workspace_translations: new table for workspace-level shared translations
type V28Migration struct{}

func (m *V28Migration) GetMajorVersion() float64 {
	return 28.0
}

func (m *V28Migration) HasSystemUpdate() bool {
	return false
}

func (m *V28Migration) HasWorkspaceUpdate() bool {
	return true
}

func (m *V28Migration) ShouldRestartServer() bool {
	return false
}

func (m *V28Migration) UpdateSystem(ctx context.Context, cfg *config.Config, db DBExecutor) error {
	return nil
}

func (m *V28Migration) UpdateWorkspace(ctx context.Context, cfg *config.Config, workspace *domain.Workspace, db DBExecutor) error {
	// Add translations column to templates table
	_, err := db.ExecContext(ctx, `
		ALTER TABLE templates
		ADD COLUMN IF NOT EXISTS translations JSONB NOT NULL DEFAULT '{}'::jsonb
	`)
	if err != nil {
		return fmt.Errorf("failed to add translations column: %w", err)
	}

	// Add default_language column to templates table
	_, err = db.ExecContext(ctx, `
		ALTER TABLE templates
		ADD COLUMN IF NOT EXISTS default_language VARCHAR(10) DEFAULT NULL
	`)
	if err != nil {
		return fmt.Errorf("failed to add default_language column: %w", err)
	}

	// Create workspace_translations table
	_, err = db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS workspace_translations (
			locale VARCHAR(10) NOT NULL PRIMARY KEY,
			content JSONB NOT NULL DEFAULT '{}'::jsonb,
			created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create workspace_translations table: %w", err)
	}

	return nil
}

func init() {
	Register(&V28Migration{})
}
```

### Step 2: Write migration test

Follow the existing pattern from `v27_test.go`:

```go
// internal/migrations/v28_test.go
package migrations

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/stretchr/testify/assert"
)

func TestV28Migration_GetMajorVersion(t *testing.T) {
	m := &V28Migration{}
	assert.Equal(t, 28.0, m.GetMajorVersion())
}

func TestV28Migration_HasSystemUpdate(t *testing.T) {
	m := &V28Migration{}
	assert.False(t, m.HasSystemUpdate())
}

func TestV28Migration_HasWorkspaceUpdate(t *testing.T) {
	m := &V28Migration{}
	assert.True(t, m.HasWorkspaceUpdate())
}

func TestV28Migration_UpdateWorkspace(t *testing.T) {
	m := &V28Migration{}
	cfg := &config.Config{}
	workspace := &domain.Workspace{ID: "test"}

	t.Run("success", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		assert.NoError(t, err)
		defer db.Close()

		// Expect: add translations column
		mock.ExpectExec("ALTER TABLE templates").WillReturnResult(sqlmock.NewResult(0, 0))
		// Expect: add default_language column
		mock.ExpectExec("ALTER TABLE templates").WillReturnResult(sqlmock.NewResult(0, 0))
		// Expect: create workspace_translations table
		mock.ExpectExec("CREATE TABLE IF NOT EXISTS workspace_translations").WillReturnResult(sqlmock.NewResult(0, 0))

		err = m.UpdateWorkspace(context.Background(), cfg, workspace, db)
		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("translations column error", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		assert.NoError(t, err)
		defer db.Close()

		mock.ExpectExec("ALTER TABLE templates").WillReturnError(fmt.Errorf("db error"))

		err = m.UpdateWorkspace(context.Background(), cfg, workspace, db)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to add translations column")
	})
}
```

### Step 3: Bump VERSION in config

In `config/config.go` line 17, change:

```go
const VERSION = "28.0"
```

### Step 4: Update workspace DB init schema

In `internal/database/schema/` (the workspace tables file), add the `translations` and `default_language` columns to the `templates` CREATE TABLE statement, and add the `workspace_translations` CREATE TABLE. This ensures new workspace databases get the correct schema on first creation.

### Step 5: Run migration tests

Run: `cd /var/www/forks/notifuse && go test ./internal/migrations/ -run TestV28 -v`
Expected: All PASS.

### Step 6: Commit

```bash
git add internal/migrations/v28.go internal/migrations/v28_test.go config/config.go internal/database/schema/
git commit -m "feat(i18n): add V28 migration for template translations

Adds translations JSONB and default_language columns to templates table.
Creates workspace_translations table for shared translations."
```

---

## Task 5: Repository Layer

Update the template repository to read/write the new columns. Create the workspace translations repository.

**Files:**
- Modify: `internal/repository/template_postgres.go` (add new columns to INSERT/SELECT, update scanner)
- Modify: `internal/repository/template_postgres_test.go`
- Create: `internal/repository/workspace_translation_postgres.go`
- Create: `internal/repository/workspace_translation_postgres_test.go`

### Step 1: Update template repository — scanner

In `internal/repository/template_postgres.go`, update `scanTemplate()` to scan the two new columns. Add them after `settings`:

```go
func scanTemplate(scanner interface{ Scan(dest ...interface{}) error }) (*domain.Template, error) {
	var (
		template        domain.Template
		templateMacroID sql.NullString
		integrationID   sql.NullString
		defaultLanguage sql.NullString
	)

	err := scanner.Scan(
		&template.ID,
		&template.Name,
		&template.Version,
		&template.Channel,
		&template.Email,
		&template.Web,
		&template.Category,
		&templateMacroID,
		&integrationID,
		&template.TestData,
		&template.Settings,
		&template.Translations,    // NEW
		&defaultLanguage,          // NEW
		&template.CreatedAt,
		&template.UpdatedAt,
	)
	// ... existing null handling ...
	if defaultLanguage.Valid {
		template.DefaultLanguage = &defaultLanguage.String
	}
	// ...
}
```

### Step 2: Update template repository — INSERT columns

Add `translations` and `default_language` to the `CreateTemplate` and `UpdateTemplate` INSERT statements. Follow the existing squirrel pattern.

### Step 3: Update template repository — SELECT columns

Add `translations` and `default_language` to all SELECT column lists (in `GetTemplateByID`, `GetTemplates`, etc.).

### Step 4: Create workspace translations repository

```go
// internal/repository/workspace_translation_postgres.go
package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/Notifuse/notifuse/internal/domain"
)

type WorkspaceTranslationPostgresRepository struct {
	getWorkspaceDB func(workspaceID string) (*sql.DB, error)
}

func NewWorkspaceTranslationPostgresRepository(
	getWorkspaceDB func(workspaceID string) (*sql.DB, error),
) *WorkspaceTranslationPostgresRepository {
	return &WorkspaceTranslationPostgresRepository{getWorkspaceDB: getWorkspaceDB}
}

func (r *WorkspaceTranslationPostgresRepository) Upsert(ctx context.Context, workspaceID string, translation *domain.WorkspaceTranslation) error {
	db, err := r.getWorkspaceDB(workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get workspace db: %w", err)
	}

	now := time.Now()
	query, args, err := sq.Insert("workspace_translations").
		Columns("locale", "content", "created_at", "updated_at").
		Values(translation.Locale, translation.Content, now, now).
		Suffix("ON CONFLICT (locale) DO UPDATE SET content = EXCLUDED.content, updated_at = EXCLUDED.updated_at").
		PlaceholderFormat(sq.Dollar).
		ToSql()
	if err != nil {
		return fmt.Errorf("failed to build query: %w", err)
	}

	_, err = db.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("failed to upsert workspace translation: %w", err)
	}

	return nil
}

func (r *WorkspaceTranslationPostgresRepository) GetByLocale(ctx context.Context, workspaceID string, locale string) (*domain.WorkspaceTranslation, error) {
	db, err := r.getWorkspaceDB(workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace db: %w", err)
	}

	query, args, err := sq.Select("locale", "content", "created_at", "updated_at").
		From("workspace_translations").
		Where(sq.Eq{"locale": locale}).
		PlaceholderFormat(sq.Dollar).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build query: %w", err)
	}

	var wt domain.WorkspaceTranslation
	err = db.QueryRowContext(ctx, query, args...).Scan(
		&wt.Locale,
		&wt.Content,
		&wt.CreatedAt,
		&wt.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil // not found is not an error — fallback to empty
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace translation: %w", err)
	}

	return &wt, nil
}

func (r *WorkspaceTranslationPostgresRepository) List(ctx context.Context, workspaceID string) ([]*domain.WorkspaceTranslation, error) {
	db, err := r.getWorkspaceDB(workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace db: %w", err)
	}

	query, args, err := sq.Select("locale", "content", "created_at", "updated_at").
		From("workspace_translations").
		OrderBy("locale ASC").
		PlaceholderFormat(sq.Dollar).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build query: %w", err)
	}

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list workspace translations: %w", err)
	}
	defer rows.Close()

	var translations []*domain.WorkspaceTranslation
	for rows.Next() {
		var wt domain.WorkspaceTranslation
		if err := rows.Scan(&wt.Locale, &wt.Content, &wt.CreatedAt, &wt.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan workspace translation: %w", err)
		}
		translations = append(translations, &wt)
	}

	return translations, rows.Err()
}

func (r *WorkspaceTranslationPostgresRepository) Delete(ctx context.Context, workspaceID string, locale string) error {
	db, err := r.getWorkspaceDB(workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get workspace db: %w", err)
	}

	query, args, err := sq.Delete("workspace_translations").
		Where(sq.Eq{"locale": locale}).
		PlaceholderFormat(sq.Dollar).
		ToSql()
	if err != nil {
		return fmt.Errorf("failed to build query: %w", err)
	}

	_, err = db.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("failed to delete workspace translation: %w", err)
	}

	return nil
}
```

### Step 5: Write repository tests with sqlmock

Follow the existing pattern in `template_postgres_test.go`. Test Upsert, GetByLocale (found + not found), List, Delete.

### Step 6: Run repository tests

Run: `cd /var/www/forks/notifuse && go test ./internal/repository/ -v`
Expected: All PASS.

### Step 7: Generate mocks

Run `go generate` or manually create mock for `WorkspaceTranslationRepository` interface using gomock, following the existing mock patterns.

### Step 8: Commit

```bash
git add internal/repository/template_postgres.go internal/repository/template_postgres_test.go internal/repository/workspace_translation_postgres.go internal/repository/workspace_translation_postgres_test.go
git commit -m "feat(i18n): add repository layer for translations

Update template repository with translations/default_language columns.
New WorkspaceTranslationPostgresRepository with full CRUD + sqlmock tests."
```

---

## Task 6: Service Layer — Workspace Translations

Create the workspace translations service and wire it into the rendering pipeline.

**Files:**
- Create: `internal/service/workspace_translation_service.go`
- Create: `internal/service/workspace_translation_service_test.go`

### Step 1: Create workspace translation service

```go
// internal/service/workspace_translation_service.go
package service

import (
	"context"
	"fmt"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/logger"
)

type WorkspaceTranslationService struct {
	repo        domain.WorkspaceTranslationRepository
	authService domain.AuthService
	logger      logger.Logger
}

func NewWorkspaceTranslationService(
	repo domain.WorkspaceTranslationRepository,
	authService domain.AuthService,
	logger logger.Logger,
) *WorkspaceTranslationService {
	return &WorkspaceTranslationService{
		repo:        repo,
		authService: authService,
		logger:      logger,
	}
}

func (s *WorkspaceTranslationService) Upsert(ctx context.Context, req domain.UpsertWorkspaceTranslationRequest) error {
	if err := req.Validate(); err != nil {
		return err
	}

	// Authenticate
	if ctx.Value(domain.SystemCallKey) == nil {
		var err error
		ctx, _, _, err = s.authService.AuthenticateUserForWorkspace(ctx, req.WorkspaceID)
		if err != nil {
			return fmt.Errorf("authentication failed: %w", err)
		}
	}

	now := time.Now()
	translation := &domain.WorkspaceTranslation{
		Locale:    req.Locale,
		Content:   req.Content,
		CreatedAt: now,
		UpdatedAt: now,
	}

	return s.repo.Upsert(ctx, req.WorkspaceID, translation)
}

func (s *WorkspaceTranslationService) List(ctx context.Context, workspaceID string) ([]*domain.WorkspaceTranslation, error) {
	if ctx.Value(domain.SystemCallKey) == nil {
		var err error
		ctx, _, _, err = s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
		if err != nil {
			return nil, fmt.Errorf("authentication failed: %w", err)
		}
	}

	return s.repo.List(ctx, workspaceID)
}

func (s *WorkspaceTranslationService) GetByLocale(ctx context.Context, workspaceID string, locale string) (*domain.WorkspaceTranslation, error) {
	return s.repo.GetByLocale(ctx, workspaceID, locale)
}

func (s *WorkspaceTranslationService) Delete(ctx context.Context, workspaceID string, locale string) error {
	if ctx.Value(domain.SystemCallKey) == nil {
		var err error
		ctx, _, _, err = s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
		if err != nil {
			return fmt.Errorf("authentication failed: %w", err)
		}
	}

	return s.repo.Delete(ctx, workspaceID, locale)
}
```

### Step 2: Write service tests with gomock

Follow the pattern in `template_service_test.go`. Test auth, validation, and delegation to repo.

### Step 3: Run service tests

Run: `cd /var/www/forks/notifuse && go test ./internal/service/ -run TestWorkspaceTranslation -v`
Expected: All PASS.

### Step 4: Commit

```bash
git add internal/service/workspace_translation_service.go internal/service/workspace_translation_service_test.go
git commit -m "feat(i18n): add workspace translation service

CRUD operations for workspace-level translations with auth + validation."
```

---

## Task 7: Wire Translations Into Rendering Pipeline

The critical integration point. Modify `CompileTemplate`, `SendEmailForTemplate`, and broadcast senders to resolve locale, merge translations, and register the `t` filter.

**Files:**
- Modify: `pkg/notifuse_mjml/template_compilation.go` (accept translations in CompileTemplateRequest)
- Modify: `pkg/notifuse_mjml/converter.go` (pass translations to Liquid engine)
- Modify: `internal/domain/template.go` (add Translations to CompileTemplateRequest)
- Modify: `internal/service/email_service.go` (resolve locale, merge translations before compilation)
- Modify: `internal/service/broadcast/queue_message_sender.go` (same)

### Step 1: Add `Translations` to `CompileTemplateRequest`

In `internal/domain/template.go`, find `CompileTemplateRequest` (currently in `pkg/notifuse_mjml/template_compilation.go`) and add:

```go
type CompileTemplateRequest struct {
	// ... existing fields ...
	Translations map[string]interface{} // merged translations for resolved locale (optional)
}
```

### Step 2: Pass translations through the compilation pipeline

In `pkg/notifuse_mjml/template_compilation.go`, when `PreserveLiquid` is false and `req.Translations` is non-nil, create the engine with translations registered:

In `ConvertJSONToMJMLWithData` (or wherever the Liquid engine is created), before rendering:

```go
if req.Translations != nil && len(req.Translations) > 0 {
    engine.RegisterTranslations(req.Translations)
}
```

The key integration point is in `processLiquidContent` (converter.go) which creates a new `SecureLiquidEngine`. This function needs to accept an optional translations map and register it before rendering.

Update `processLiquidContent` signature:

```go
func processLiquidContent(content string, templateData map[string]interface{}, context string) (string, error)
```

to:

```go
func processLiquidContentWithTranslations(content string, templateData map[string]interface{}, context string, translations map[string]interface{}) (string, error)
```

And in the new function, after creating the engine:

```go
engine := NewSecureLiquidEngine()
if translations != nil {
    engine.RegisterTranslations(translations)
}
```

Keep the original `processLiquidContent` as a wrapper that passes `nil` for translations to maintain backward compatibility.

Also update `ProcessLiquidTemplate` (the public function used by email_service.go for subject rendering):

```go
func ProcessLiquidTemplateWithTranslations(content string, templateData map[string]interface{}, context string, translations map[string]interface{}) (string, error) {
    return processLiquidContentWithTranslations(content, templateData, context, translations)
}
```

### Step 3: Wire translations into `SendEmailForTemplate`

In `internal/service/email_service.go`, in `SendEmailForTemplate()`, after getting the template (line ~258) and before building the compile request (line ~310):

```go
// Resolve locale and merge translations
var mergedTranslations map[string]interface{}
if template.Translations != nil && len(template.Translations) > 0 {
    // Get contact language from the template data
    contactLang := ""
    if contactData, ok := request.MessageData.Data["contact"].(map[string]interface{}); ok {
        if lang, ok := contactData["language"].(string); ok {
            contactLang = lang
        }
    }

    // Get workspace settings for default language
    workspaceDefaultLang := workspace.Settings.GetDefaultLanguage()

    // Get available locales from template translations
    availableLocales := make([]string, 0, len(template.Translations))
    for locale := range template.Translations {
        availableLocales = append(availableLocales, locale)
    }

    // Resolve best locale
    resolvedLocale := domain.ResolveLocale(contactLang, availableLocales, template.DefaultLanguage, workspaceDefaultLang)

    // Get template translations for resolved locale
    templateTranslations, _ := template.Translations[resolvedLocale].(map[string]interface{})

    // Get workspace translations for resolved locale (best effort)
    var workspaceTranslations map[string]interface{}
    wsTranslation, err := s.workspaceTranslationRepo.GetByLocale(ctx, request.WorkspaceID, resolvedLocale)
    if err == nil && wsTranslation != nil {
        workspaceTranslations = wsTranslation.Content
    }

    // Merge: workspace base, template override
    mergedTranslations = domain.MergeTranslations(workspaceTranslations, templateTranslations)
}
```

Then pass `mergedTranslations` into the compile request and the subject rendering call.

### Step 4: Wire translations into broadcast sender

Same pattern in `internal/service/broadcast/queue_message_sender.go` in `buildQueueEntry()`. The template and workspace translations should be loaded once per batch (not per recipient). Only the locale resolution changes per contact.

### Step 5: Write integration-style tests

Test the full flow: template with translations → compile with contact language → verify correct translation appears in output.

### Step 6: Run all tests

Run: `cd /var/www/forks/notifuse && make test-unit`
Expected: All PASS.

### Step 7: Commit

```bash
git add pkg/notifuse_mjml/template_compilation.go pkg/notifuse_mjml/converter.go internal/domain/template.go internal/service/email_service.go internal/service/broadcast/queue_message_sender.go
git commit -m "feat(i18n): wire translations into rendering pipeline

Resolve locale from contact.language, merge template + workspace
translations, register t filter on Liquid engine before compilation.
Works for both transactional and broadcast sends."
```

---

## Task 8: HTTP Handler — Workspace Translations API

Add REST endpoints for workspace translation CRUD.

**Files:**
- Create: `internal/http/workspace_translation_handler.go`
- Create: `internal/http/workspace_translation_handler_test.go`
- Modify: `internal/http/router.go` (or wherever routes are registered)

### Step 1: Create handler

Follow the existing pattern from `template_handler.go`. Create handlers for:

- `POST /api/workspace_translations.upsert` — JSON body: `{workspace_id, locale, content}`
- `GET /api/workspace_translations.list` — query param: `workspace_id`
- `POST /api/workspace_translations.delete` — JSON body: `{workspace_id, locale}`

### Step 2: Register routes

Add routes in the router file, following the existing pattern with auth middleware.

### Step 3: Write handler tests

Use `httptest.NewRecorder()` and follow the pattern in `template_handler_test.go`.

### Step 4: Run handler tests

Run: `cd /var/www/forks/notifuse && go test ./internal/http/ -run TestWorkspaceTranslation -v`
Expected: All PASS.

### Step 5: Commit

```bash
git add internal/http/workspace_translation_handler.go internal/http/workspace_translation_handler_test.go internal/http/router.go
git commit -m "feat(i18n): add workspace translations API endpoints

POST workspace_translations.upsert, GET .list, POST .delete"
```

---

## Task 9: Dependency Wiring

Wire the new repository, service, and handler into the application bootstrap.

**Files:**
- Modify: `cmd/api/main.go` (or wherever DI wiring happens)

### Step 1: Find the DI wiring location

Look at `cmd/api/main.go` or `cmd/api/server.go` for where repositories and services are constructed. Add:

```go
// Repository
workspaceTranslationRepo := repository.NewWorkspaceTranslationPostgresRepository(getWorkspaceDB)

// Service
workspaceTranslationService := service.NewWorkspaceTranslationService(workspaceTranslationRepo, authService, appLogger)

// Handler
workspaceTranslationHandler := http.NewWorkspaceTranslationHandler(workspaceTranslationService)
```

Also wire `workspaceTranslationRepo` into `EmailService` (it needs it for locale resolution during send).

### Step 2: Verify the application compiles and starts

Run: `cd /var/www/forks/notifuse && go build ./cmd/api/`
Expected: Builds successfully.

### Step 3: Commit

```bash
git add cmd/api/
git commit -m "feat(i18n): wire translation dependencies into application bootstrap"
```

---

## Task 10: Frontend — API Types & Service

Add TypeScript types and API functions for translations.

**Files:**
- Modify: `console/src/services/api/template.ts` (add translations fields to Template interface)
- Create: `console/src/services/api/workspace-translations.ts`

### Step 1: Update Template interface

In `console/src/services/api/template.ts`, add to the `Template` interface:

```typescript
export interface Template {
  // ... existing fields ...
  translations?: Record<string, Record<string, unknown>>  // locale → nested key-value
  default_language?: string
}
```

### Step 2: Create workspace translations API service

```typescript
// console/src/services/api/workspace-translations.ts
import { apiClient } from './client'

export interface WorkspaceTranslation {
  locale: string
  content: Record<string, unknown>
  created_at: string
  updated_at: string
}

export async function listWorkspaceTranslations(workspaceId: string): Promise<WorkspaceTranslation[]> {
  const response = await apiClient.get('/api/workspace_translations.list', {
    params: { workspace_id: workspaceId },
  })
  return response.data.translations || []
}

export async function upsertWorkspaceTranslation(
  workspaceId: string,
  locale: string,
  content: Record<string, unknown>
): Promise<void> {
  await apiClient.post('/api/workspace_translations.upsert', {
    workspace_id: workspaceId,
    locale,
    content,
  })
}

export async function deleteWorkspaceTranslation(
  workspaceId: string,
  locale: string
): Promise<void> {
  await apiClient.post('/api/workspace_translations.delete', {
    workspace_id: workspaceId,
    locale,
  })
}
```

### Step 3: Commit

```bash
git add console/src/services/api/template.ts console/src/services/api/workspace-translations.ts
git commit -m "feat(i18n): add frontend API types and service for translations"
```

---

## Task 11: Frontend — Translations Panel Component

Build the translations management UI panel for the template editor.

**Files:**
- Create: `console/src/components/templates/TranslationsPanel.tsx`
- Modify: `console/src/components/templates/CreateTemplateDrawer.tsx` (integrate panel)

### Step 1: Build the TranslationsPanel component

Key features:
- Displays translation keys grouped by nested prefix (collapsible tree)
- Input fields per supported language for each key
- Default language marked as required (checkmark indicator)
- Missing translations shown with warning indicator
- "Add Key" button with dot-path input
- "Delete Key" button per key
- Import/Export JSON buttons

Use Ant Design components: `Collapse`, `Input`, `Button`, `Upload`, `Tag`, `Tooltip`, `Space`.

Use the workspace's `supported_languages` to determine which locale columns to show.

The component receives and updates the `translations` field on the Template object (controlled state, lifting state up to the parent drawer).

### Step 2: Integrate into CreateTemplateDrawer

Add a "Translations" tab alongside existing tabs in the template editor. When selected, show the `TranslationsPanel` with the current template's translations.

Use `useLingui()` for all user-facing strings (following the i18n pattern established in the console).

### Step 3: Test the panel

Write a Vitest test for the TranslationsPanel component:
- Renders translation keys
- Adding a key works
- Deleting a key works
- Import JSON works
- Export JSON produces correct output

### Step 4: Run frontend tests

Run: `cd /var/www/forks/notifuse/console && pnpm test`
Expected: All PASS.

### Step 5: Commit

```bash
git add console/src/components/templates/TranslationsPanel.tsx console/src/components/templates/CreateTemplateDrawer.tsx
git commit -m "feat(i18n): add translations panel to template editor

Collapsible key tree with per-locale inputs, add/delete keys,
JSON import/export. Integrated as a tab in the template editor drawer."
```

---

## Task 12: Frontend — Workspace Language Settings

Add language configuration to the workspace settings page.

**Files:**
- Modify: `console/src/pages/WorkspaceSettingsPage.tsx` (add language settings section)
- Create: `console/src/components/settings/LanguageSettings.tsx`

### Step 1: Create LanguageSettings component

A settings section that allows:
- Setting the workspace default language (dropdown with common language codes)
- Managing supported languages (tag-based multi-select)

Uses Ant Design `Select` with predefined language options (en, fr, de, es, pt, it, nl, ja, ko, zh, ru, ar, etc.).

### Step 2: Add to WorkspaceSettingsPage

Add `'languages'` to the `validSections` array and render the `LanguageSettings` component when that section is active.

### Step 3: Run frontend tests

Run: `cd /var/www/forks/notifuse/console && pnpm test`
Expected: All PASS.

### Step 4: Extract i18n strings

Run: `cd /var/www/forks/notifuse/console && pnpm run lingui:extract`

### Step 5: Commit

```bash
git add console/src/pages/WorkspaceSettingsPage.tsx console/src/components/settings/LanguageSettings.tsx console/src/i18n/
git commit -m "feat(i18n): add workspace language settings UI

Default language and supported languages configuration in workspace settings."
```

---

## Task 13: Final Integration Testing & Cleanup

End-to-end verification that the full pipeline works.

**Files:**
- Run all backend tests
- Run all frontend tests
- Manual smoke test checklist

### Step 1: Run full backend test suite

Run: `cd /var/www/forks/notifuse && make test-unit`
Expected: All PASS.

### Step 2: Run frontend tests

Run: `cd /var/www/forks/notifuse/console && pnpm test`
Expected: All PASS.

### Step 3: Run linting

Run: `cd /var/www/forks/notifuse/console && pnpm run lint`
Expected: No errors.

### Step 4: Build check

Run: `cd /var/www/forks/notifuse && go build ./cmd/api/`
Run: `cd /var/www/forks/notifuse/console && pnpm run build`
Expected: Both build successfully.

### Step 5: Manual smoke test checklist

- [ ] Create workspace, set default language to "en", supported languages to ["en", "fr"]
- [ ] Create template with `{{ "welcome.heading" | t }}` in content
- [ ] Add translations: en → "Welcome!", fr → "Bienvenue !"
- [ ] Preview template — shows English
- [ ] Send transactional email to contact with language "fr" — receives French content
- [ ] Send transactional email to contact with no language — receives English (default)
- [ ] Send transactional email to contact with language "pt-BR" — receives English (fallback)
- [ ] Test placeholder: `{{ "greeting" | t: name: contact.first_name }}` with translation "Hello {{ name }}!"
- [ ] Import/export JSON translations round-trip
- [ ] Workspace translations: create shared key, reference from template, verify it resolves

### Step 6: Final commit

```bash
git commit -m "feat(i18n): template-level internationalization

Implements issue #268. Templates can now use {{ \"key\" | t }} syntax
to reference translation keys. Translations stored as nested JSON
per locale. Automatic language resolution from contact.language
with fallback chain. Workspace-level shared translations supported."
```

---

## Summary

| Task | Description | Layer |
|------|-------------|-------|
| 1 | Translation utility functions | Domain |
| 2 | Liquid `t` filter | Rendering engine |
| 3 | Domain model changes | Domain |
| 4 | V28 database migration | Migration |
| 5 | Repository layer | Repository |
| 6 | Workspace translation service | Service |
| 7 | Wire into rendering pipeline | Service + Rendering |
| 8 | HTTP handler for workspace translations | HTTP |
| 9 | Dependency wiring | Bootstrap |
| 10 | Frontend API types & service | Frontend |
| 11 | Translations panel component | Frontend |
| 12 | Workspace language settings | Frontend |
| 13 | Integration testing & cleanup | Testing |

Tasks 1-2 are independent and can be done in parallel.
Tasks 3-6 must be sequential (domain → migration → repo → service).
Tasks 7-9 depend on 1-6.
Tasks 10-12 depend on 3 (types) but can start frontend work after Task 3.
Task 13 depends on everything.
