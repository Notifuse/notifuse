# Template field without Templates read plan

**Status**: plan reviewed, implemented
**Date**: 2026-11-11
**Target**: 44.0 (`## [44.0] - unreleased` on `dev`). No migration, no environment variable, no backend change.
**Base**: `origin/dev` at `de243ef`.

## Goal

A member without Templates read who edits a transactional notification (since #511, only one an integration manages)
sees which template it uses and why it cannot be shown, instead of a blank field. The console no longer requests
templates that member cannot read.

## Current behaviour

- `TemplateSelectorInput` (`components/templates/TemplateSelectorInput.tsx`) always runs `templates.list` (`:52-68`,
  `enabled: !!workspaceId`) and `templates.get` for its value (`:74-99`), even when `disabled`. Without Templates read
  both answer 403. The error is only logged, so the field stays blank, with no hint.
- `UpsertTransactionalNotificationDrawer` (`:322-327`) renders it, disabled for a managed notification. Saving still
  works: the form keeps `channels.email.template_id` and the drawer omits `channels` for a managed notification.
- The card's `TemplatePreview` (`pages/TransactionalNotificationsPage.tsx:71-95`) runs `templates.get` for every card.
  Without Templates read it answers 403 and the eye button just does not appear.
- All of this shipped in v43.0.

## Design

1. `TemplateSelectorInput`: optional `canReadTemplates?: boolean`, default `true`, so its other callers are unchanged.
   When `false`:
   - neither `templates.list` nor `templates.get` runs (`enabled: !!workspaceId && canReadTemplates`, and an early
     return in the effect, with `canReadTemplates` in its dependencies);
   - the input shows the stored template ID (`canReadTemplates ? selectedTemplate?.name || '' : value || ''`, or the
     placeholder when there is none), is disabled, never calls `onChange`, opens no drawer, and shows no preview
     button. The ID is meaningful: the only managed notifications come from Supabase, whose template IDs read like
     `supabase_magiclink_…`, and the notification carries no template name.
2. `UpsertTransactionalNotificationDrawer`: a required `canReadTemplates: boolean` prop, passed to the selector.
   Without it, the template field's `extra` (not `help`, so validation errors still show) says "You need Templates read
   permission to see this template's name and preview". The page
   passes `!!permissions?.templates?.read` to both drawer uses (Create stays disabled without it, as #511 made it).
3. `TransactionalNotificationsPage`: the card renders `TemplatePreview` only with Templates read, so no `templates.get`
   runs without it.
4. i18n: the new string in the 8 catalogues.
5. `CHANGELOG.md`: folded into #511's `Fix` bullet under 44.0, which already says that editing a notification managed
   by an integration stays available with Transactional write alone. Appended: "Its edit drawer then shows the
   template's ID, and that Templates read is needed to see the template, instead of an empty field, and the page no
   longer requests templates such a member cannot read."

## Tests

| Implementation | Test file | Cases |
|---|---|---|
| `components/templates/TemplateSelectorInput.tsx` | `components/templates/TemplateSelectorInput.test.tsx` | A `QueryClient` per render, `Wrapper` takes `canReadTemplates`. `canReadTemplates={false}` with a value: neither `templates.list` nor `templates.get` is called, the input shows the ID and is disabled, `onChange` is never called; without the prop: list and get are called (existing cases) |
| `components/transactional/UpsertTransactionalNotificationDrawer.tsx` | `components/transactional/UpsertTransactionalNotificationDrawer.test.tsx` | The selector mock exposes `canReadTemplates` and `disabled`. Managed notification with `canReadTemplates={false}`: the selector gets `false`, the hint is shown; the existing "sends no channels" case runs with it; with `canReadTemplates`: no hint (existing cases pass the prop) |
| `pages/TransactionalNotificationsPage.tsx` | `pages/TransactionalNotificationsPage.test.tsx` | `templatesApi.get` cleared in `beforeEach`; not called without Templates read, called with `{workspace_id: 'ws1', id: 'tpl1'}` with it; the drawer mock exposes `canReadTemplates`, checked for Create and for the managed Edit |
| catalogues | `i18n/locales/catalogues.test.ts` | every entry translated; compilation checked by `npm run build` |

## Known, not handled

- The other callers of `TemplateSelectorInput` (broadcasts, lists, automations, segments, for example
  `ListDrawer.tsx`, `UpsertBroadcastDrawer.tsx`) do not check Templates read, so the same blank field appears there for
  a member without it. Passing the new prop from those pages is how to fix them later.

## Test commands

```
cd console && npm run lingui:extract && npx vitest run && npx tsc -b && npx eslint . && npm run build
```
