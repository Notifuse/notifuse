# Template i18n Documentation Update Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Update all project documentation to reflect the template i18n feature (v28.0).

**Architecture:** Update OpenAPI specs (schemas + paths + root), CHANGELOG, and CLAUDE.md. No external docs site changes (docs.notifuse.com is maintained separately).

**Tech Stack:** YAML (OpenAPI 3.0.3), Markdown.

**Design doc:** `docs/plans/2026-02-24-template-i18n-design.md`

---

## Task 1: Update OpenAPI Template Schema

Add `translations` and `default_language` fields to the Template schema, and update Create/Update/Compile request types.

**Files:**
- Modify: `openapi/components/schemas/template.yaml`

### Step 1: Add fields to Template schema

After the `settings` property (line 64), add:

```yaml
    translations:
      type: object
      additionalProperties:
        type: object
        additionalProperties: true
      description: |
        Per-locale translation key-value maps. Keys are dot-separated paths (e.g., "welcome.heading").
        Values are strings that may contain {{ placeholder }} syntax for named arguments.
        Outer keys are locale codes (e.g., "en", "fr", "pt-BR").
      example:
        en:
          welcome:
            heading: "Welcome!"
            greeting: "Hello {{ name }}!"
        fr:
          welcome:
            heading: "Bienvenue !"
            greeting: "Bonjour {{ name }} !"
    default_language:
      type: string
      nullable: true
      description: Override the workspace default language for this template. When null, inherits from workspace settings.
      example: en
      maxLength: 10
```

### Step 2: Add to CreateTemplateRequest properties

After `settings` (line 199), add:

```yaml
    translations:
      type: object
      additionalProperties:
        type: object
        additionalProperties: true
      description: Per-locale translation key-value maps
    default_language:
      type: string
      nullable: true
      description: Override the workspace default language for this template
      maxLength: 10
```

### Step 3: Add to UpdateTemplateRequest properties

Same addition as CreateTemplateRequest, after `settings` (line 260).

### Step 4: Add `translations` to CompileTemplateRequest

After `channel` (line 310), add:

```yaml
    translations:
      type: object
      additionalProperties: true
      description: Merged translations map for a specific locale, used by the Liquid `t` filter during compilation
```

### Step 5: Verify YAML is valid

Run: `cd /var/www/forks/notifuse && python3 -c "import yaml; yaml.safe_load(open('openapi/components/schemas/template.yaml'))" 2>&1 || echo "YAML invalid"`
Expected: No errors.

### Step 6: Commit

```bash
git add openapi/components/schemas/template.yaml
git commit -m "docs: add translations fields to OpenAPI template schema"
```

---

## Task 2: Create OpenAPI Workspace Translations Schema

New schema file for the workspace translations API types.

**Files:**
- Create: `openapi/components/schemas/workspace-translation.yaml`

### Step 1: Create the schema file

```yaml
WorkspaceTranslation:
  type: object
  properties:
    locale:
      type: string
      description: Locale code (e.g., "en", "fr", "pt-BR")
      example: en
      maxLength: 10
    content:
      type: object
      additionalProperties: true
      description: |
        Nested key-value translation map. Keys use dot-separated paths.
        Values are strings, optionally containing {{ placeholder }} syntax.
      example:
        common:
          greeting: "Hello"
          footer: "Unsubscribe from our emails"
    created_at:
      type: string
      format: date-time
      description: When the translation was created
    updated_at:
      type: string
      format: date-time
      description: When the translation was last updated
  required:
    - locale
    - content

UpsertWorkspaceTranslationRequest:
  type: object
  required:
    - workspace_id
    - locale
    - content
  properties:
    workspace_id:
      type: string
      description: The ID of the workspace
      example: ws_1234567890
    locale:
      type: string
      description: Locale code
      example: fr
      maxLength: 10
    content:
      type: object
      additionalProperties: true
      description: Nested key-value translation map
      example:
        common:
          greeting: "Bonjour"
          footer: "Se désabonner de nos emails"

DeleteWorkspaceTranslationRequest:
  type: object
  required:
    - workspace_id
    - locale
  properties:
    workspace_id:
      type: string
      description: The ID of the workspace
      example: ws_1234567890
    locale:
      type: string
      description: Locale code to delete
      example: fr
```

### Step 2: Commit

```bash
git add openapi/components/schemas/workspace-translation.yaml
git commit -m "docs: add OpenAPI schema for workspace translations"
```

---

## Task 3: Create OpenAPI Workspace Translations Paths

New paths file for the workspace translations API endpoints.

**Files:**
- Create: `openapi/paths/workspace-translations.yaml`

### Step 1: Create the paths file

```yaml
/api/workspace_translations.list:
  get:
    summary: List workspace translations
    description: Retrieves all workspace-level translations. Returns one entry per locale with its nested key-value content.
    operationId: listWorkspaceTranslations
    security:
      - BearerAuth: []
    parameters:
      - name: workspace_id
        in: query
        required: true
        schema:
          type: string
        description: The ID of the workspace
        example: ws_1234567890
    responses:
      '200':
        description: List of workspace translations retrieved successfully
        content:
          application/json:
            schema:
              type: object
              properties:
                translations:
                  type: array
                  items:
                    $ref: '../components/schemas/workspace-translation.yaml#/WorkspaceTranslation'
      '400':
        description: Bad request - validation failed
        content:
          application/json:
            schema:
              $ref: '../components/schemas/common.yaml#/ErrorResponse'
      '401':
        description: Unauthorized
        content:
          application/json:
            schema:
              $ref: '../components/schemas/common.yaml#/ErrorResponse'

/api/workspace_translations.upsert:
  post:
    summary: Create or update workspace translation
    description: |
      Creates or updates translations for a specific locale at the workspace level.
      If translations for the locale already exist, they are replaced.
      Workspace translations are shared across all templates and resolved when a template
      uses `{{ "key" | t }}` and the key is not found in the template's own translations.
    operationId: upsertWorkspaceTranslation
    security:
      - BearerAuth: []
    requestBody:
      required: true
      content:
        application/json:
          schema:
            $ref: '../components/schemas/workspace-translation.yaml#/UpsertWorkspaceTranslationRequest'
    responses:
      '200':
        description: Translation upserted successfully
        content:
          application/json:
            schema:
              type: object
              properties:
                success:
                  type: boolean
                  example: true
      '400':
        description: Bad request - validation failed
        content:
          application/json:
            schema:
              $ref: '../components/schemas/common.yaml#/ErrorResponse'
      '401':
        description: Unauthorized
        content:
          application/json:
            schema:
              $ref: '../components/schemas/common.yaml#/ErrorResponse'

/api/workspace_translations.delete:
  post:
    summary: Delete workspace translation
    description: Deletes all translations for a specific locale at the workspace level.
    operationId: deleteWorkspaceTranslation
    security:
      - BearerAuth: []
    requestBody:
      required: true
      content:
        application/json:
          schema:
            $ref: '../components/schemas/workspace-translation.yaml#/DeleteWorkspaceTranslationRequest'
    responses:
      '200':
        description: Translation deleted successfully
        content:
          application/json:
            schema:
              type: object
              properties:
                success:
                  type: boolean
                  example: true
      '400':
        description: Bad request - validation failed
        content:
          application/json:
            schema:
              $ref: '../components/schemas/common.yaml#/ErrorResponse'
      '401':
        description: Unauthorized
        content:
          application/json:
            schema:
              $ref: '../components/schemas/common.yaml#/ErrorResponse'
```

### Step 2: Commit

```bash
git add openapi/paths/workspace-translations.yaml
git commit -m "docs: add OpenAPI paths for workspace translations API"
```

---

## Task 4: Update OpenAPI Root File

Register the new schemas and paths in the root `openapi.yaml`.

**Files:**
- Modify: `openapi/openapi.yaml`

### Step 1: Add workspace translation paths

After the templates paths block (after line 75: `/api/templates.compile`), add:

```yaml
  /api/workspace_translations.list:
    $ref: './paths/workspace-translations.yaml#/~1api~1workspace_translations.list'
  /api/workspace_translations.upsert:
    $ref: './paths/workspace-translations.yaml#/~1api~1workspace_translations.upsert'
  /api/workspace_translations.delete:
    $ref: './paths/workspace-translations.yaml#/~1api~1workspace_translations.delete'
```

### Step 2: Add workspace translation schema refs

After the template schema refs in the `components.schemas` section (after line 220: `TrackingSettings`), add:

```yaml
    WorkspaceTranslation:
      $ref: './components/schemas/workspace-translation.yaml#/WorkspaceTranslation'
    UpsertWorkspaceTranslationRequest:
      $ref: './components/schemas/workspace-translation.yaml#/UpsertWorkspaceTranslationRequest'
    DeleteWorkspaceTranslationRequest:
      $ref: './components/schemas/workspace-translation.yaml#/DeleteWorkspaceTranslationRequest'
```

### Step 3: Commit

```bash
git add openapi/openapi.yaml
git commit -m "docs: register workspace translations in OpenAPI root"
```

---

## Task 5: Update CHANGELOG

Add v28.0 entry to the changelog.

**Files:**
- Modify: `CHANGELOG.md`

### Step 1: Add v28.0 entry at the top (after line 3)

```markdown
## [28.0] - 2026-XX-XX

### New Features

- **Template i18n**: Auto-select email content based on contact language (#268)
  - **Liquid `t` filter**: Use `{{ "key" | t }}` in templates to reference translation keys
  - **Placeholder support**: Pass dynamic values with `{{ "greeting" | t: name: contact.first_name }}`
  - **Nested keys**: Dot-separated key paths (e.g., `welcome.heading`, `cta.button`)
  - **Per-template translations**: Store translation key-value maps per locale as part of the template
  - **Workspace translations**: Shared translation catalog available to all templates in a workspace
  - **Automatic locale resolution**: Fallback chain from `contact.language` → base language → template default → workspace default
  - **Translations panel**: Manage translation keys and per-locale values in the template editor
  - **Import/Export**: Bulk upload/download translations as JSON files per locale
- **Workspace language settings**: Configure default language and supported languages in workspace settings

### Database Migration

- Added `translations` JSONB column and `default_language` VARCHAR column to `templates` table (workspace migration)
- Created `workspace_translations` table for workspace-level shared translations (workspace migration)
```

Use the actual release date when shipping. The `XX-XX` placeholder should be replaced at release time.

### Step 2: Commit

```bash
git add CHANGELOG.md
git commit -m "docs: add v28.0 changelog entry for template i18n"
```

---

## Task 6: Update CLAUDE.md — Migration Section

Update the migration documentation section in CLAUDE.md to reflect V28 as the latest migration and add template i18n context.

**Files:**
- Modify: `CLAUDE.md`

### Step 1: Update the migration example

In the CLAUDE.md section "Creating Database Migrations", the example shows V7. Update the comment about the current version number. Find the text:

```
2. **Create Migration File**: Create a new file in `internal/migrations/` (e.g., `v7.go`)
```

No change needed here — this is a generic example and doesn't reference a specific current version. But ensure the VERSION constant reference is accurate. Search for any mention of `VERSION = "27.2"` or similar — there shouldn't be one, as CLAUDE.md references it generically.

### Step 2: Add template i18n to the "Available Data Structure" context

In the CLAUDE.md section about templates or wherever the available template variables are documented, note that the `t` filter is now available:

This may not be explicitly documented in CLAUDE.md. If there's a section about Liquid templating or available filters, add:

```markdown
#### Translation Filter (v28.0+)

Templates can reference translatable strings using the Liquid `t` filter:

```liquid
{{ "welcome.heading" | t }}
{{ "welcome.greeting" | t: name: contact.first_name }}
```

Translations are stored per-locale as nested JSON on the template's `translations` field. The system resolves the best locale from `contact.language` with a fallback chain: exact match → base language → template default → workspace default.
```

### Step 3: Commit

```bash
git add CLAUDE.md
git commit -m "docs: update CLAUDE.md with template i18n documentation"
```

---

## Task 7: Update Design Doc with Migration Correction

Fix the design doc migration section — `WorkspaceSettings` is stored as JSONB in the `settings` column, so `default_language` and `supported_languages` go into the struct, not as separate table columns. The design doc currently shows `ALTER TABLE workspaces ADD COLUMN` which is incorrect.

**Files:**
- Modify: `docs/plans/2026-02-24-template-i18n-design.md`

### Step 1: Fix Section 3 (Database Migration)

Replace the "System database" SQL block with:

```markdown
### System database

No schema changes needed. `default_language` and `supported_languages` are added to the `WorkspaceSettings` Go struct. Since `WorkspaceSettings` is stored as JSONB in the existing `workspaces.settings` column, the new fields are automatically handled — existing workspaces will have these fields absent in JSON, and Go will deserialize them as zero values (falling back to `"en"` and `["en"]` via helper methods).
```

### Step 2: Update Section 2 (Data Model)

Replace the "Modified: `Workspace` struct" subsection to clarify these fields go on `WorkspaceSettings`, not `Workspace`:

```markdown
### Modified: `WorkspaceSettings` struct (inside Workspace.Settings JSONB)

```go
type WorkspaceSettings struct {
    // ... existing fields ...
    DefaultLanguage    string   `json:"default_language,omitempty"`    // e.g., "en"
    SupportedLanguages []string `json:"supported_languages,omitempty"` // e.g., ["en", "fr", "de"]
}
```
```

### Step 3: Commit

```bash
git add docs/plans/2026-02-24-template-i18n-design.md
git commit -m "docs: fix design doc migration section — language settings go in WorkspaceSettings JSONB"
```

---

## Summary

| Task | File(s) | What changes |
|------|---------|-------------|
| 1 | `openapi/components/schemas/template.yaml` | Add `translations`, `default_language` to Template + request schemas |
| 2 | `openapi/components/schemas/workspace-translation.yaml` (new) | WorkspaceTranslation + request/response types |
| 3 | `openapi/paths/workspace-translations.yaml` (new) | `.list`, `.upsert`, `.delete` endpoint definitions |
| 4 | `openapi/openapi.yaml` | Register new paths and schemas |
| 5 | `CHANGELOG.md` | v28.0 entry with feature list + migration notes |
| 6 | `CLAUDE.md` | Add `t` filter documentation to templates/Liquid section |
| 7 | `docs/plans/2026-02-24-template-i18n-design.md` | Fix migration section (WorkspaceSettings JSONB, not new columns) |

Tasks 1-4 are the OpenAPI updates (sequential — schemas before paths before root).
Task 5-6 are independent markdown updates.
Task 7 is a correction to an existing doc.
