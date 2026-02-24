# Template i18n External Docs Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Update the public docs site (docs.notifuse.com) to document the template i18n feature (v28.0).

**Architecture:** Add a dedicated `features/template-translations.mdx` page as the primary guide, with brief mentions and links from existing pages (templates, workspaces, contacts, transactional API, broadcasts). Update `openapi.json` with workspace_translations endpoints and template schema changes. Update `docs.json` navigation.

**Tech Stack:** Mintlify (MDX), OpenAPI 3.0.3 JSON.

**Design doc:** `docs/plans/2026-02-24-template-i18n-external-docs-design.md`

**Repo:** `/var/www/forks/notifuse-docs` (cloned from https://github.com/Notifuse/docs)

---

## Task 1: Create the dedicated template translations page

The main deliverable. New MDX page covering the full i18n feature.

**Files:**
- Create: `features/template-translations.mdx`

### Step 1: Create the page

Create `features/template-translations.mdx` with this content:

```mdx
---
title: Template Translations
description: 'Send emails in your contacts'' preferred language using translation keys. Define translations per template or share them across your workspace, and Notifuse automatically selects the right language at send time.'
---

## Overview

Template translations let you send email content in each contact's preferred language without duplicating templates. Instead of creating separate templates per language, you write a single template using **translation keys** and provide translations for each supported locale.

When an email is sent, Notifuse automatically resolves the correct locale based on the contact's `language` field and renders the template with the matching translations.

## The `t` Filter

Use the Liquid `t` filter to reference translation keys in your templates:

### Simple Key Lookup

```liquid
{{ "welcome.heading" | t }}
```

If the contact's language is `fr`, this renders the French translation for the key `welcome.heading`.

### Placeholders

Pass dynamic values into translation strings using named arguments:

```liquid
{{ "welcome.greeting" | t: name: contact.first_name }}
```

With the translation string `"Hello {{ name }}!"` and a contact named Sarah, this renders: **Hello Sarah!**

### Subject Lines

The `t` filter works in email subject lines too:

```liquid
{{ "welcome.subject" | t }}
```

### Full Example

**Template:**

```liquid
{{ "welcome.heading" | t }}

{{ "welcome.greeting" | t: name: contact.first_name }}

{{ "welcome.body" | t }}

{{ "cta.button" | t }}
```

**English translations:**

```json
{
  "welcome": {
    "heading": "Welcome!",
    "greeting": "Hello {{ name }}!",
    "body": "Thanks for joining us."
  },
  "cta": {
    "button": "Get Started"
  }
}
```

**French translations:**

```json
{
  "welcome": {
    "heading": "Bienvenue !",
    "greeting": "Bonjour {{ name }} !",
    "body": "Merci de nous avoir rejoints."
  },
  "cta": {
    "button": "Commencer"
  }
}
```

## Translation Keys

Keys use **dot-separated paths** that map to a nested JSON structure. This keeps translations organized:

| Key | JSON Path |
|-----|-----------|
| `welcome.heading` | `{ "welcome": { "heading": "..." } }` |
| `welcome.greeting` | `{ "welcome": { "greeting": "..." } }` |
| `cta.button` | `{ "cta": { "button": "..." } }` |
| `footer.unsubscribe` | `{ "footer": { "unsubscribe": "..." } }` |

If a key is missing for the resolved locale, the template renders `[Missing translation: key.name]` so you can spot untranslated strings.

## Per-Template Translations

Each template has its own `translations` field — a JSON object keyed by locale code, containing the nested key-value translations for that locale.

### Managing Translations in the Editor

The template editor includes a **Translations panel** where you can:

- Add, edit, and remove translation keys
- Provide values for each supported language
- See which keys are missing translations (shown with a warning indicator)
- Preview the template in different languages

### Import / Export

You can bulk-manage translations using JSON files:

- **Export**: Downloads one JSON file per locale (e.g., `en.json`, `fr.json`)
- **Import**: Upload a JSON file for a specific locale. New keys are added, existing keys are overwritten, absent keys are left untouched.

The JSON format matches the nested key structure:

```json
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

## Workspace Translations

Workspace translations are a **shared translation catalog** available to all templates in a workspace. They act as a fallback — if a template doesn't define a key, the workspace translation is used instead.

This is useful for strings that appear across many templates:

- Footer text (`footer.unsubscribe`, `footer.company_name`)
- Common CTAs (`cta.learn_more`, `cta.contact_us`)
- Legal text (`legal.privacy`, `legal.terms`)

Workspace translations are managed via the API:

- `GET /api/workspace_translations.list` — List all workspace translations
- `POST /api/workspace_translations.upsert` — Create or update translations for a locale
- `POST /api/workspace_translations.delete` — Delete translations for a locale

See the [API Reference](/api-reference) for full endpoint documentation.

### Resolution Priority

When a template uses `{{ "key" | t }}`, the system looks for the key in this order:

1. **Template translations** for the resolved locale
2. **Workspace translations** for the resolved locale

Template translations always take priority. A template key `welcome.heading` shadows a workspace key `welcome.heading`, but a workspace key `footer.unsubscribe` is accessible if the template doesn't define it.

## Locale Resolution

When an email is sent, Notifuse determines which locale to use with this fallback chain:

| Priority | Source | Example |
|----------|--------|---------|
| 1 | Contact's `language` (exact match) | `pt-BR` → uses `pt-BR` translations |
| 2 | Contact's `language` (base language) | `pt-BR` → falls back to `pt` if no `pt-BR` |
| 3 | Template's `default_language` | If set, overrides the workspace default |
| 4 | Workspace's default language | Configured in workspace settings (e.g., `en`) |

**Examples:**

- Contact has `language: "fr"` → French translations are used
- Contact has `language: "pt-BR"`, no `pt-BR` translations exist, but `pt` does → Portuguese translations are used
- Contact has no `language` set → Falls back to the template default, then the workspace default
- Template has `default_language: "de"` → German is used as the fallback instead of the workspace default

### Setting the Template Default Language

Each template can optionally set a `default_language` that overrides the workspace default. This is useful when a template is primarily written in a specific language that differs from the workspace default.

## Workspace Language Settings

Configure language defaults in your workspace settings:

- **Default Language**: The fallback language used when a contact has no `language` field set (e.g., `en`)
- **Supported Languages**: The list of languages your workspace supports (e.g., `["en", "fr", "de", "es"]`). This determines which locale columns appear in the translations panel.

## Best Practices

- **Start with your default language**: Always provide complete translations for your workspace's default language first. This ensures every contact sees content, even if their language isn't supported yet.
- **Use workspace translations for shared strings**: Footer text, legal disclaimers, and common CTAs should live in workspace translations to avoid duplication across templates.
- **Keep key names consistent**: Use a predictable naming convention like `section.element` (e.g., `welcome.heading`, `cta.button`, `footer.unsubscribe`).
- **Set `contact.language` on your contacts**: The i18n system relies on the contact's `language` field. Set it via the API, CSV import, or let the [Notification Center](/features/notification-center) auto-detect it.
- **Test with preview**: Use the template editor's language preview selector to check how your email looks in each supported language before sending.
```

### Step 2: Commit

```bash
cd /var/www/forks/notifuse-docs
git add features/template-translations.mdx
git commit -m "docs: add template translations feature page"
```

---

## Task 2: Update templates.mdx with translation mention

Add a brief section linking to the dedicated translations page.

**Files:**
- Modify: `features/templates.mdx`

### Step 1: Add Translations section

After the "Available Data Structure" section (after line 133, at the end of the file), add:

```mdx

## Translations

Templates support built-in internationalization using the Liquid `t` filter. Instead of duplicating templates per language, define translation keys and provide per-locale values:

```liquid
{{ "welcome.heading" | t }}
{{ "welcome.greeting" | t: name: contact.first_name }}
```

Notifuse automatically selects the right language based on the contact's `language` field. For the full guide, see [Template Translations](/features/template-translations).
```

### Step 2: Commit

```bash
cd /var/www/forks/notifuse-docs
git add features/templates.mdx
git commit -m "docs: add translations section to templates page"
```

---

## Task 3: Update workspaces.mdx with language settings

Add a Language Settings section to the workspaces page.

**Files:**
- Modify: `features/workspaces.mdx`

### Step 1: Add Language Settings section

After the "Multi-Tenant Architecture" section (after line 34, at the end of the file), add:

```mdx

## Language Settings

Each workspace can configure language defaults that apply to all templates:

- **Default Language**: The fallback language used when a contact has no `language` field set (e.g., `en`). All templates will use this as their final fallback.
- **Supported Languages**: The list of languages your workspace supports (e.g., English, French, German). This determines which locale columns appear in the template translations panel.

These settings work with [Template Translations](/features/template-translations) to automatically send emails in each contact's preferred language.
```

### Step 2: Commit

```bash
cd /var/www/forks/notifuse-docs
git add features/workspaces.mdx
git commit -m "docs: add language settings section to workspaces page"
```

---

## Task 4: Update contacts.mdx language field description

Expand the `language` field description to mention its role in i18n.

**Files:**
- Modify: `features/contacts.mdx`

### Step 1: Update the language field row

In the Contact Fields table (line 25), find:

```
| `language`                                 | String   | Preferred language                         |
```

Replace with:

```
| `language`                                 | String   | Preferred language (drives [automatic locale resolution](/features/template-translations#locale-resolution) for translated templates) |
```

### Step 2: Commit

```bash
cd /var/www/forks/notifuse-docs
git add features/contacts.mdx
git commit -m "docs: expand language field description with i18n link"
```

---

## Task 5: Update transactional-api.mdx with multi-language note

Add a note about automatic language resolution.

**Files:**
- Modify: `features/transactional-api.mdx`

### Step 1: Add Multi-Language Support section

After the "Key Features" section's last subsection (after "Email Delivery Options", before "## API Endpoint" at line 57), add:

```mdx

### Multi-Language Support

If your templates use [translation keys](/features/template-translations), Notifuse automatically selects the right language based on `contact.language`. No API changes are needed — just make sure your contacts have a `language` field set:

```json
{
  "notification": {
    "contact": {
      "email": "user@example.com",
      "language": "fr"
    }
  }
}
```

The contact's language is resolved through a [fallback chain](/features/template-translations#locale-resolution): exact match → base language → template default → workspace default.
```

### Step 2: Commit

```bash
cd /var/www/forks/notifuse-docs
git add features/transactional-api.mdx
git commit -m "docs: add multi-language support note to transactional API page"
```

---

## Task 6: Update broadcast-campaigns.mdx with multi-language note

Add a note about per-contact language resolution in broadcasts.

**Files:**
- Modify: `features/broadcast-campaigns.mdx`

### Step 1: Add Multi-Language Support section

Before the "## Best Practices" section (before line 358), add:

```mdx

## Multi-Language Support

If your broadcast template uses [translation keys](/features/template-translations), each recipient automatically receives the email in their preferred language based on their `language` field.

The template and workspace translations are loaded once per broadcast. For each recipient, only the locale resolution changes — selecting the right translation set based on the contact's language. This means multi-language broadcasts have no significant performance overhead.

See [Template Translations](/features/template-translations) for how to set up translation keys and manage per-locale content.

```

### Step 2: Commit

```bash
cd /var/www/forks/notifuse-docs
git add features/broadcast-campaigns.mdx
git commit -m "docs: add multi-language support note to broadcast campaigns page"
```

---

## Task 7: Update docs.json navigation

Register the new page and API endpoints in the Mintlify navigation.

**Files:**
- Modify: `docs.json`

### Step 1: Add template-translations to Features nav

In the `docs.json` file, find the Features group pages array. After `"features/templates"` (line 43), add `"features/template-translations"` as the next entry.

Before:
```json
              "features/templates",
              "features/broadcast-campaigns",
```

After:
```json
              "features/templates",
              "features/template-translations",
              "features/broadcast-campaigns",
```

### Step 2: Add Workspace Translations group to API Reference tab

In the API Reference tab's groups array, after the Templates group (after line 153), add a new group:

```json
          {
            "group": "Workspace Translations",
            "openapi": "openapi.json",
            "pages": [
              "GET /api/workspace_translations.list",
              "POST /api/workspace_translations.upsert",
              "POST /api/workspace_translations.delete"
            ]
          },
```

### Step 3: Commit

```bash
cd /var/www/forks/notifuse-docs
git add docs.json
git commit -m "docs: add template translations to navigation"
```

---

## Task 8: Update openapi.json — Template schemas

Add `translations` and `default_language` fields to template-related schemas.

**Files:**
- Modify: `openapi.json`

### Step 1: Add fields to Template schema

In the `Template` schema (components → schemas → Template → properties), after the `settings` property, add:

```json
        "translations": {
          "type": "object",
          "additionalProperties": {
            "type": "object",
            "additionalProperties": true
          },
          "description": "Per-locale translation key-value maps. Keys are dot-separated paths (e.g., \"welcome.heading\"). Values are strings that may contain {{ placeholder }} syntax for named arguments. Outer keys are locale codes (e.g., \"en\", \"fr\", \"pt-BR\").",
          "example": {
            "en": {
              "welcome": {
                "heading": "Welcome!",
                "greeting": "Hello {{ name }}!"
              }
            },
            "fr": {
              "welcome": {
                "heading": "Bienvenue !",
                "greeting": "Bonjour {{ name }} !"
              }
            }
          }
        },
        "default_language": {
          "type": "string",
          "nullable": true,
          "description": "Override the workspace default language for this template. When null, inherits from workspace settings.",
          "example": "en",
          "maxLength": 10
        },
```

### Step 2: Add fields to CreateTemplateRequest schema

In the `CreateTemplateRequest` schema (components → schemas → CreateTemplateRequest → properties), after the `settings` property, add:

```json
        "translations": {
          "type": "object",
          "additionalProperties": {
            "type": "object",
            "additionalProperties": true
          },
          "description": "Per-locale translation key-value maps"
        },
        "default_language": {
          "type": "string",
          "nullable": true,
          "description": "Override the workspace default language for this template",
          "maxLength": 10
        },
```

### Step 3: Add fields to UpdateTemplateRequest schema

Same as CreateTemplateRequest — add `translations` and `default_language` after `settings`.

### Step 4: Add translations to CompileTemplateRequest schema

In the `CompileTemplateRequest` schema, after the `channel` property, add:

```json
        "translations": {
          "type": "object",
          "additionalProperties": true,
          "description": "Merged translations map for a specific locale, used by the Liquid t filter during compilation"
        },
```

### Step 5: Verify JSON is valid

Run: `cd /var/www/forks/notifuse-docs && python3 -c "import json; json.load(open('openapi.json'))" 2>&1 || echo "JSON invalid"`

### Step 6: Commit

```bash
cd /var/www/forks/notifuse-docs
git add openapi.json
git commit -m "docs: add translations fields to OpenAPI template schemas"
```

---

## Task 9: Update openapi.json — Workspace Translations endpoints

Add the three workspace translation API endpoints and their schemas.

**Files:**
- Modify: `openapi.json`

### Step 1: Add WorkspaceTranslation schema

In the `components.schemas` section, add a new `WorkspaceTranslation` schema:

```json
    "WorkspaceTranslation": {
      "type": "object",
      "properties": {
        "locale": {
          "type": "string",
          "description": "Locale code (e.g., \"en\", \"fr\", \"pt-BR\")",
          "example": "en",
          "maxLength": 10
        },
        "content": {
          "type": "object",
          "additionalProperties": true,
          "description": "Nested key-value translation map. Keys use dot-separated paths. Values are strings, optionally containing {{ placeholder }} syntax.",
          "example": {
            "common": {
              "greeting": "Hello",
              "footer": "Unsubscribe from our emails"
            }
          }
        },
        "created_at": {
          "type": "string",
          "format": "date-time",
          "description": "When the translation was created"
        },
        "updated_at": {
          "type": "string",
          "format": "date-time",
          "description": "When the translation was last updated"
        }
      },
      "required": ["locale", "content"]
    },
    "UpsertWorkspaceTranslationRequest": {
      "type": "object",
      "required": ["workspace_id", "locale", "content"],
      "properties": {
        "workspace_id": {
          "type": "string",
          "description": "The ID of the workspace",
          "example": "ws_1234567890"
        },
        "locale": {
          "type": "string",
          "description": "Locale code",
          "example": "fr",
          "maxLength": 10
        },
        "content": {
          "type": "object",
          "additionalProperties": true,
          "description": "Nested key-value translation map",
          "example": {
            "common": {
              "greeting": "Bonjour",
              "footer": "Se désabonner de nos emails"
            }
          }
        }
      }
    },
    "DeleteWorkspaceTranslationRequest": {
      "type": "object",
      "required": ["workspace_id", "locale"],
      "properties": {
        "workspace_id": {
          "type": "string",
          "description": "The ID of the workspace",
          "example": "ws_1234567890"
        },
        "locale": {
          "type": "string",
          "description": "Locale code to delete",
          "example": "fr"
        }
      }
    },
```

### Step 2: Add workspace_translations.list path

In the `paths` section, add:

```json
    "/api/workspace_translations.list": {
      "get": {
        "summary": "List workspace translations",
        "description": "Retrieves all workspace-level translations. Returns one entry per locale with its nested key-value content.",
        "operationId": "listWorkspaceTranslations",
        "security": [{ "BearerAuth": [] }],
        "parameters": [
          {
            "name": "workspace_id",
            "in": "query",
            "required": true,
            "schema": { "type": "string" },
            "description": "The ID of the workspace",
            "example": "ws_1234567890"
          }
        ],
        "responses": {
          "200": {
            "description": "List of workspace translations retrieved successfully",
            "content": {
              "application/json": {
                "schema": {
                  "type": "object",
                  "properties": {
                    "translations": {
                      "type": "array",
                      "items": { "$ref": "#/components/schemas/WorkspaceTranslation" }
                    }
                  }
                }
              }
            }
          },
          "400": {
            "description": "Bad request - validation failed",
            "content": {
              "application/json": {
                "schema": { "$ref": "#/components/schemas/ErrorResponse" }
              }
            }
          },
          "401": {
            "description": "Unauthorized",
            "content": {
              "application/json": {
                "schema": { "$ref": "#/components/schemas/ErrorResponse" }
              }
            }
          }
        }
      }
    },
```

### Step 3: Add workspace_translations.upsert path

```json
    "/api/workspace_translations.upsert": {
      "post": {
        "summary": "Create or update workspace translation",
        "description": "Creates or updates translations for a specific locale at the workspace level. If translations for the locale already exist, they are replaced. Workspace translations are shared across all templates and resolved when a template uses {{ \"key\" | t }} and the key is not found in the template's own translations.",
        "operationId": "upsertWorkspaceTranslation",
        "security": [{ "BearerAuth": [] }],
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": { "$ref": "#/components/schemas/UpsertWorkspaceTranslationRequest" }
            }
          }
        },
        "responses": {
          "200": {
            "description": "Translation upserted successfully",
            "content": {
              "application/json": {
                "schema": {
                  "type": "object",
                  "properties": {
                    "success": { "type": "boolean", "example": true }
                  }
                }
              }
            }
          },
          "400": {
            "description": "Bad request - validation failed",
            "content": {
              "application/json": {
                "schema": { "$ref": "#/components/schemas/ErrorResponse" }
              }
            }
          },
          "401": {
            "description": "Unauthorized",
            "content": {
              "application/json": {
                "schema": { "$ref": "#/components/schemas/ErrorResponse" }
              }
            }
          }
        }
      }
    },
```

### Step 4: Add workspace_translations.delete path

```json
    "/api/workspace_translations.delete": {
      "post": {
        "summary": "Delete workspace translation",
        "description": "Deletes all translations for a specific locale at the workspace level.",
        "operationId": "deleteWorkspaceTranslation",
        "security": [{ "BearerAuth": [] }],
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": { "$ref": "#/components/schemas/DeleteWorkspaceTranslationRequest" }
            }
          }
        },
        "responses": {
          "200": {
            "description": "Translation deleted successfully",
            "content": {
              "application/json": {
                "schema": {
                  "type": "object",
                  "properties": {
                    "success": { "type": "boolean", "example": true }
                  }
                }
              }
            }
          },
          "400": {
            "description": "Bad request - validation failed",
            "content": {
              "application/json": {
                "schema": { "$ref": "#/components/schemas/ErrorResponse" }
              }
            }
          },
          "401": {
            "description": "Unauthorized",
            "content": {
              "application/json": {
                "schema": { "$ref": "#/components/schemas/ErrorResponse" }
              }
            }
          }
        }
      }
    },
```

### Step 5: Verify JSON is valid

Run: `cd /var/www/forks/notifuse-docs && python3 -c "import json; json.load(open('openapi.json'))" 2>&1 || echo "JSON invalid"`

### Step 6: Commit

```bash
cd /var/www/forks/notifuse-docs
git add openapi.json
git commit -m "docs: add workspace translations API to OpenAPI spec"
```

---

## Summary

| Task | File(s) | What changes |
|------|---------|-------------|
| 1 | `features/template-translations.mdx` (new) | Full i18n guide: t filter, keys, locale resolution, workspace translations, import/export, best practices |
| 2 | `features/templates.mdx` | Brief Translations section with `t` filter example + link |
| 3 | `features/workspaces.mdx` | Language Settings section (default language, supported languages) |
| 4 | `features/contacts.mdx` | Expand `language` field description with i18n link |
| 5 | `features/transactional-api.mdx` | Multi-Language Support note under Key Features |
| 6 | `features/broadcast-campaigns.mdx` | Multi-Language Support section before Best Practices |
| 7 | `docs.json` | Add page to Features nav + Workspace Translations API group |
| 8 | `openapi.json` | Add translations/default_language to Template schemas |
| 9 | `openapi.json` | Add workspace_translations endpoints + schemas |

Tasks 1-6 are MDX page changes (Task 1 is the big one, 2-6 are small additions).
Task 7 is navigation config.
Tasks 8-9 are OpenAPI spec updates.

All tasks are independent except: Task 7 depends on Task 1 (page must exist before adding to nav), and Task 9 depends on Task 8 (both modify the same file sequentially).
