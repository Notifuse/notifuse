# Template-level i18n — Design Document

**Issue**: [#268](https://github.com/Notifuse/notifuse/issues/268)
**Branch**: `feat/template-i18n`
**Date**: 2026-02-24

## Problem

Contacts have a `language` field (synced via notification center since v27.2), but there is no built-in way to send email content in that language. Current workarounds — duplicating templates per language or using verbose `{% if contact.language == "fr" %}` blocks — are manual, error-prone, and don't scale.

## Solution

Translation keys with a string catalog, using a Liquid `t` filter. Templates reference translatable strings via `{{ "key" | t }}`, and translations are stored as nested JSON per locale alongside the template. The system resolves the correct locale at render time based on `contact.language`.

## Design Decisions

| Decision | Choice |
|---|---|
| Approach | Translation keys with string catalog (Option A from issue) |
| Scope | Both template-level and workspace-level translations |
| Resolution | Same `{{ "key" \| t }}` syntax; template-first, then workspace fallback |
| Key format | Nested, dot-separated (e.g., `welcome.heading`) |
| Syntax | Liquid filter: `{{ "key" \| t }}` with placeholder support via named args |
| Language config | Workspace default + per-template override |
| Storage | Inline JSONB on existing `templates` table + new `workspace_translations` table |
| Editor UX (v1) | Translations panel alongside the visual editor |

## 1. Liquid `t` Filter

### Syntax

```liquid
<!-- Simple translation -->
{{ "welcome.heading" | t }}

<!-- With placeholders -->
{{ "welcome.greeting" | t: name: contact.first_name }}

<!-- Works in subject lines too -->
Subject: {{ "welcome.subject" | t }}
```

### Translation JSON (per locale)

```json
{
  "welcome": {
    "heading": "Welcome!",
    "greeting": "Hello {{ name }}!",
    "subject": "Welcome to Notifuse"
  }
}
```

Placeholder values like `{{ name }}` inside translation strings are interpolated with the named arguments passed to the filter.

### Implementation

Register a `TranslationFilters` struct on the `SecureLiquidEngine` via `env.RegisterFilter()`:

```go
type TranslationFilters struct {
    translations map[string]interface{} // merged: template (priority) + workspace
    locale       string
}

func (tf *TranslationFilters) T(key interface{}, args ...interface{}) interface{} {
    keyStr := fmt.Sprintf("%v", key)

    // 1. Resolve nested key via dot-path traversal
    value := resolveNestedKey(tf.translations, keyStr)
    if value == "" {
        return "[Missing translation: " + keyStr + "]"
    }

    // 2. Interpolate placeholders if named args provided
    if len(args) > 0 {
        value = interpolatePlaceholders(value, args)
    }

    return value
}
```

### Locale Resolution (fallback chain)

```
1. contact.language exact match   (e.g., "pt-BR")
2. contact.language base match    (e.g., "pt")
3. template.default_language      (if set, overrides workspace)
4. workspace.default_language     (e.g., "en")
```

### Translation Merging at Render Time

```
1. Load workspace translations for resolved locale
2. Load template translations for resolved locale
3. Deep-merge: template translations override workspace translations
4. Pass merged map to TranslationFilters
```

A template key `welcome.heading` shadows a workspace key `welcome.heading`, but a workspace key `common.footer` is accessible if the template doesn't define it.

## 2. Data Model & Storage

### Modified: `Template` struct

```go
type Template struct {
    // ... existing fields ...
    Translations    map[string]map[string]interface{} `json:"translations"`     // locale → nested key-value
    DefaultLanguage *string                           `json:"default_language"` // nullable, overrides workspace
}
```

### Modified: `WorkspaceSettings` struct (inside Workspace.Settings JSONB)

```go
type WorkspaceSettings struct {
    // ... existing fields ...
    DefaultLanguage    string   `json:"default_language,omitempty"`    // e.g., "en"
    SupportedLanguages []string `json:"supported_languages,omitempty"` // e.g., ["en", "fr", "de"]
}
```

### New: `WorkspaceTranslation` entity

```go
type WorkspaceTranslation struct {
    Locale    string                 `json:"locale"`
    Content   map[string]interface{} `json:"content"` // nested key-value
    CreatedAt time.Time              `json:"created_at"`
    UpdatedAt time.Time              `json:"updated_at"`
}
```

## 3. Database Migration (V28)

Non-breaking, additive migration. Existing templates get empty `{}` translations and `NULL` default_language (inheriting workspace default).

### System database

No schema changes needed. `default_language` and `supported_languages` are added to the `WorkspaceSettings` Go struct. Since `WorkspaceSettings` is stored as JSONB in the existing `workspaces.settings` column, the new fields are automatically handled — existing workspaces will have these fields absent in JSON, and Go will deserialize them as zero values (falling back to `"en"` and `["en"]` via helper methods).

### Workspace database

```sql
ALTER TABLE templates
  ADD COLUMN IF NOT EXISTS translations JSONB NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE templates
  ADD COLUMN IF NOT EXISTS default_language VARCHAR(10);

CREATE TABLE IF NOT EXISTS workspace_translations (
    locale VARCHAR(10) NOT NULL PRIMARY KEY,
    content JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);
```

## 4. API Surface

### New endpoints (workspace translations)

```
POST /api/workspace_translations.upsert   — create/update translations for a locale
GET  /api/workspace_translations.list      — list all workspace translations
POST /api/workspace_translations.delete    — delete translations for a locale
POST /api/workspace_translations.import    — bulk import JSON per locale
GET  /api/workspace_translations.export    — export all translations as JSON
```

### Modified endpoints (templates)

Template translations are part of the existing `templates.create` and `templates.update` payloads via the new `translations` field. No new template-specific endpoints needed.

### Transactional API (no changes)

Language resolution is automatic from `contact.language`:

```json
{
  "template_id": "welcome_email",
  "contact": { "email": "user@example.com", "language": "fr" }
}
```

### Send flow

```
SendNotification()
  → resolve template
  → resolve locale from contact.language + fallback chain
  → load workspace translations for locale
  → merge template translations (priority) over workspace translations
  → register TranslationFilters with merged map
  → render Liquid (subject + body) — t filter resolves keys
  → compile MJML → HTML
  → send
```

Broadcasts follow the same flow, but per-contact in the batch loop. Template + workspace translations are loaded once; only locale resolution changes per contact.

## 5. Frontend — Translations Panel

New component `console/src/components/templates/TranslationsPanel.tsx`, integrated into the existing template editor drawer.

### Behavior

- A "Translations" tab/button in the template editor
- Collapsible list of translation keys grouped by nested prefix
- Each key expands to show input fields per supported language (from workspace `supported_languages`)
- Default language value is required, others optional
- "Add Key" button for creating new keys with dot-path input
- Import/Export buttons for bulk JSON per locale

### Wireframe

```
┌─ Translations Panel ──────────────────────────┐
│                                                │
│  Language: [en ▾] (preview selector)           │
│                                                │
│  ▼ welcome                                     │
│    heading                                     │
│      en: [Welcome!                    ] ✓      │
│      fr: [Bienvenue !                 ]        │
│      de: [Willkommen!                 ]        │
│                                                │
│    greeting                                    │
│      en: [Hello {{ name }}!           ] ✓      │
│      fr: [Bonjour {{ name }} !        ]        │
│      de: [                            ] ⚠      │
│                                                │
│  ▼ cta                                         │
│    button                                      │
│      en: [Get Started                 ] ✓      │
│      fr: [Commencer                   ]        │
│      de: [Loslegen                    ]        │
│                                                │
│  [+ Add Key]  [Import JSON]  [Export JSON]     │
└────────────────────────────────────────────────┘

✓ = default language (required)
⚠ = missing translation (will fall back to default)
```

### Workspace translations UI

Not in v1 scope. Workspace-level translations are managed via API (import/export JSON). A dedicated settings page can come later.

## 6. Import/Export Format

Per-locale JSON files with nested structure, matching internal storage:

```json
// en.json
{
  "welcome": {
    "heading": "Welcome!",
    "greeting": "Hello {{ name }}!"
  },
  "cta": {
    "button": "Get Started"
  }
}
```

Export produces one JSON file per locale. Import uses upsert semantics: new keys are added, existing keys are overwritten, absent keys are untouched. Both template-level and workspace-level translations use the same format.

## 7. Testing Strategy

| Layer | What to test |
|---|---|
| Domain | `Template.Validate()` with translations, locale fallback resolution, nested key resolution, placeholder interpolation |
| Service | Translation merging (template over workspace), `CompileTemplate` with translations, language resolution from contact |
| Repository | CRUD with translations JSONB, `workspace_translations` table operations |
| HTTP | New `workspace_translations` endpoints, template create/update with translations |
| Liquid filter | `T` filter: simple key, nested key, missing key fallback, placeholders with named args, locale resolution chain |
| Migration | V28 idempotency |
| Frontend | TranslationsPanel component, import/export flow |

### Edge cases

- Contact with no `language` set → falls back to workspace default
- Contact with `pt-BR` when only `pt` translations exist → base language match
- Translation key exists in workspace but not template → workspace value used
- Translation value contains Liquid (`{{ contact.first_name }}`) → rendered correctly after `t` filter resolves
- Empty translations `{}` → template renders with fallback markers (`[Missing translation: key]`)

## Prior Art

- **Novu**: Translation keys (`{{t.key}}`) with i18next under the hood. Enterprise feature. Uses preprocessing trick to protect keys from Liquid rendering. No placeholder support in filter syntax.
- **Shopify**: `{{ "key" | t: name: value }}` filter with named args for placeholders. Nested JSON locale files. Our approach is closest to this.
- **Symfony/Twig**: `{{ "key" | trans({"%name%": value}) }}` filter. Similar concept, different placeholder syntax.

Our design takes Shopify's filter approach (most natural for Liquid) with Novu's dual-scope model (template + workspace translations) and clean fallback chain.
