# Template i18n External Docs Update — Design Document

**Related**: `docs/plans/2026-02-24-template-i18n-design.md` (feature design), `docs/plans/2026-02-24-template-i18n-docs.md` (internal docs plan)
**Repo**: https://github.com/Notifuse/docs (cloned to `/var/www/forks/notifuse-docs`)
**Framework**: Mintlify (MDX pages, `docs.json` nav, `openapi.json` API spec)
**Date**: 2026-02-24

## Goal

Update the public docs site (docs.notifuse.com) to document the template i18n feature (v28.0). Users need to understand how to use the `t` filter, manage translations, and configure language settings.

## Scope

### New page

**`features/template-translations.mdx`** — Dedicated guide for template internationalization.

Sections:
1. **Overview** — What it does and why (auto-select email content based on contact language)
2. **The `t` Filter** — Liquid syntax with examples: simple key lookup, placeholder interpolation, usage in subject lines
3. **Translation Keys** — Dot-separated key paths, nested JSON structure, example per-locale JSON
4. **Per-Template Translations** — Translations panel in the template editor, managing keys and values per locale
5. **Workspace Translations** — Shared translation catalog available to all templates, API-managed, acts as fallback
6. **Locale Resolution** — Fallback chain: contact.language exact → base language → template default → workspace default
7. **Import/Export** — JSON format per locale, upsert semantics for import, one file per locale on export
8. **Best Practices** — Start with default language, use workspace translations for repeated strings (footers, CTAs), keep key naming consistent

### Existing page updates

| Page | What to add |
|------|-------------|
| `features/templates.mdx` | New "Translations" section after Liquid Syntax: brief `t` filter example + link to `features/template-translations` |
| `features/workspaces.mdx` | New "Language Settings" section: default language, supported languages, how they affect template rendering |
| `features/contacts.mdx` | Expand `language` field row description to mention it drives automatic locale resolution for translated templates |
| `features/transactional-api.mdx` | Add "Multi-Language Support" note: language is auto-resolved from `contact.language`, no API changes needed |
| `features/broadcast-campaigns.mdx` | Add "Multi-Language Support" note: per-contact language resolution happens automatically in the batch loop |

### Navigation & API spec

| File | What to change |
|------|----------------|
| `docs.json` | Add `features/template-translations` to Features group (after `features/templates`). Add "Workspace Translations" group to API Reference tab with list/upsert/delete endpoints. |
| `openapi.json` | Add 3 workspace_translations endpoints (GET list, POST upsert, POST delete). Update Template schema + CreateTemplateRequest + UpdateTemplateRequest with `translations` and `default_language` fields. Update CompileTemplateRequest with `translations` field. |

## Design Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Dedicated page vs. expand templates.mdx | Dedicated page | Templates page is already 133 lines; i18n is a self-contained feature with enough depth for its own page |
| Where to place in nav | After "Templates" in Features | Natural reading order — learn about templates first, then translations |
| openapi.json updates | Yes | Keeps API Reference tab current; workspace_translations endpoints need to be discoverable |
| Existing page updates | Brief mentions + links | Avoids duplicating content; each page acknowledges the feature exists and links to the dedicated guide |
