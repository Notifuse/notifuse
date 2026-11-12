# Template fields of lists, broadcasts, automations and segments without Templates read plan

**Status**: plan reviewed, implemented
**Date**: 2026-11-12
**Target**: 44.0 (`## [44.0] - unreleased` on `dev`). No migration, no environment variable, no backend change, no
docs change (no API changes; `../docs/features/admins-api-keys.mdx:159` already lists what needs Templates read).
**Base**: `origin/dev` at `65813b8`.

## Goal

A member without Templates read sees, in every template field and template label of the lists, broadcasts, automations
and segments screens, the stored template's ID and why it cannot be shown or chosen, instead of a blank field. These
screens no longer request templates that member cannot read. #520 did this for the transactional notification drawer
only.

## Current behaviour

- `TemplateSelectorInput` takes `canReadTemplates?: boolean` (default `true`, #520). Only
  `UpsertTransactionalNotificationDrawer` passes it. When `false`, the field shows the stored ID, is disabled, requests
  nothing, and opens no drawer. Nothing in the field itself says why: the transactional drawer adds an `extra`.
- No create or update path of lists, broadcasts, automations or segments checks Templates read (`list_service.go:53`,
  `:153` check Lists write, `segment_service.go:68` Segments, `automation_service.go` Automations,
  `CreateBroadcast`/`UpdateBroadcast` Broadcasts write and senders). So a member without Templates read can save these
  with a template already set, but cannot choose one: the list request answers 403. Some reads do go through the
  template service, see the broadcast cards below.
- The callers that pass nothing, so the field stays blank and the requests answer 403:
  - Lists: `ListDrawer.tsx:255` (double opt-in template), in `CreateListDrawer`, rendered 3 times by `ListsPage.tsx`
    (`:242`, `:297`, `:376`). `ListsPage`'s card `TemplatePreviewButton` (`:62-110`) runs `templates.get` for each list
    with a double opt-in template; without Templates read it stays a "preview" link that does nothing.
  - Broadcasts: `UpsertBroadcastDrawer.tsx:833` (each A/B variation) and `:912` (single variation), rendered 3 times by
    `BroadcastsPage.tsx` (`:621`, `:1640`, `:1779`). Beside each, `VariationSenderSelect` runs `templates.list`
    (category marketing) to tag the template's sender; `ChangeVariationSenderModal` (`BroadcastsPage.tsx:686`) uses it
    too. The page lists broadcasts `with_templates: true` (`:1375`); without Templates read `ListBroadcasts` drops the
    template silently (`broadcast_service.go:290-295`, through `template_service.go:274`), so each variation of a card
    reads "Untitled" (`:856`) and its preview button is disabled (`:1039-1056`).
  - Automations: `TriggerConfigForm.tsx:405` ("Clicked a link" trigger template filter) and `EmailConfigForm.tsx:70`
    (email node), both rendered by `NodeConfigPanel`, inside `AutomationProvider` (`UpsertAutomationDrawer.tsx:309`),
    inside `UpsertAutomationDrawer`, rendered 3 times by `AutomationsPage.tsx` (`:168`, `:193`, `:241`).
    `AutomationsPage` runs `templates.list` (`:70-74`) for the canvas, passed as `templates`; `EmailNode.tsx:47` shows
    the name, else "Template set".
  - Segments: `form_leaf.tsx:424` (`LeafActionForm`, email timeline filter), rendered by `TreeNodeInput`
    (`segment/input.tsx:535-550`), whose own `templates.list` (`:112-116`) names the template in a leaf's summary
    (`:798-806`, already falling back to the ID). `TreeNodeInput` is rendered by `ButtonUpsertSegment`
    (`segment/button_upsert.tsx:766`), from `SegmentsFilter` (`:248`) and its `SegmentButton` (`:19-37`, `:131`), from
    `ContactsPage.tsx:1115`; by automations' `ConditionsField.tsx:140` (Trigger and Filter nodes, inside
    `AutomationProvider`); and by `DebugSegmentPage` (`:70-75`), which passes no `workspaceId` and so requests nothing.
- Shipped in v43.0: the lists field and card, the broadcast fields and cards, the email node, the segment email
  filter. Not shipped (unreleased 44.0): the "Clicked a link" trigger's template filter (#507) and
  `VariationSenderSelect`/`ChangeVariationSenderModal` (#509).

## Design

Reading the permissions inside the selector is not simpler: `useWorkspacePermissions` (`AuthContext.tsx:160-200`) has
no cache, runs `getMembers` per instance, and starts at `null`. So the pages, which already hold the permissions, pass
them down as props, as #520 did.

1. `TemplateSelectorInput`: without Templates read, the input gets a lock suffix (`LockOutlined` in a `Tooltip`, with the
   same text as `aria-label`): "You need Templates read permission to see this template's name and preview" when there
   is a value (the string #520 added) and "You need Templates read permission to choose a template" when there is none
   (new string). The suffix is a plain span in antd 6.6's affix wrapper and the disabled styles set no
   `pointer-events: none`, so hovering it opens the tooltip. Clearing stays unavailable without Templates read: the
   field is disabled, as #520 made it. The transactional drawer keeps its `extra`, so its hint shows twice (text and
   tooltip), which is acceptable.
2. Entry points take a **required** `canReadTemplates: boolean`, so a new use cannot forget it; the components between
   them and the selector take it **optional, default `true`**, as the selector does:
   - Lists: `CreateListDrawer` (required) passes it to its selector. `ListsPage` passes
     `!!permissions?.templates?.read` to its 3 uses. Without it, the card shows the double opt-in template's ID (as
     `Typography.Text code`) instead of `TemplatePreviewButton`, so no `templates.get` runs.
   - Broadcasts: `UpsertBroadcastDrawer` (required) passes it to both selectors and both `VariationSenderSelect`.
     `VariationSenderSelect` (optional) skips its `templates.list` without it (no "template's" tag then).
     `ChangeVariationSenderModal` (required, rendered by the page) passes it on. `BroadcastsPage` passes
     `!!permissions?.templates?.read` to the 3 drawers and to the modal. `BroadcastCard`, which has `permissions`, shows
     `variation.template_id` instead of "Untitled" for a variation without template when Templates read is missing.
   - Automations: `UpsertAutomationDrawer` (required) passes it to `AutomationProvider` (required), which adds
     `canReadTemplates` to `AutomationContextType`, to the context value and to its `useMemo` dependencies
     (`AutomationContext.tsx:396-429`). `NodeConfigPanel` passes it from the context to `TriggerConfigForm` and
     `EmailConfigForm` (optional), which pass it to their selector. `ConditionsField` passes it from the context to
     `TreeNodeInput`. `EmailNode` shows the template ID instead of "Template set" without it. `AutomationsPage` passes
     `!!permissions?.templates?.read` to the 3 drawers and adds it to its `templates.list` `enabled`.
   - Segments: `SegmentsFilter`, `SegmentButton` and `ButtonUpsertSegment` (and its inner drawer) take it (required)
     and pass it to `TreeNodeInput`. `TreeNodeInput` (optional) adds it to its `templates.list` `enabled` (the summary
     then shows the ID, as it already does for an unknown template) and passes it through `LeafFormProps` to
     `LeafActionForm`'s selector. `ContactsPage` passes `!!permissions?.templates?.read`. `DebugSegmentPage` keeps the
     default.
3. i18n: the new string in the 8 catalogues.
4. `CHANGELOG.md`: a new `Fix` line under 44.0, right after #520's bullet, naming only what shipped: "A member without
   Templates read now sees the template's ID, with a lock saying that Templates read is needed, instead of an empty
   field or "Untitled", in a list's double opt-in template, a broadcast's templates, an automation's email nodes and a
   segment's email conditions, and these screens no longer request templates such a member cannot read." The unreleased
   trigger filter and sender select get no line.

## Tests

| Implementation | Test file | Cases |
|---|---|---|
| `components/templates/TemplateSelectorInput.tsx` | `components/templates/TemplateSelectorInput.test.tsx` | without Templates read: with a value, the lock (by its label) shows and hovering it shows the name-and-preview tooltip; without a value, the choose hint; with it: no lock (existing cases) |
| `components/lists/ListDrawer.tsx` | `components/lists/ListDrawer.test.tsx` (new) | editing a list with double opt-in and `canReadTemplates={false}`: the selector shows the ID, is disabled, no `templates.list`/`templates.get`; with `true`: `templates.list` runs |
| `pages/ListsPage.tsx` | `pages/ListsPage.test.tsx` | without Templates read: the card shows the template ID, no `templates.get`, the drawers get `false`; with it: `templates.get` runs, `true` |
| `components/broadcasts/UpsertBroadcastDrawer.tsx` | `components/broadcasts/UpsertBroadcastDrawer.test.tsx` | the selector mock exposes `canReadTemplates`: `false` reaches it for a single and an A/B variation, and `templatesApi.list` (the real `VariationSenderSelect`) is not called; existing renders pass `true` |
| `components/broadcasts/VariationSenderSelect.tsx` | `components/broadcasts/VariationSenderSelect.test.tsx` | `canReadTemplates={false}`: no `templates.list`, the senders still listed, no "template's" tag |
| `components/broadcasts/ChangeVariationSenderModal.tsx` | `components/broadcasts/ChangeVariationSenderModal.test.tsx` | `canReadTemplates={false}`: no `templates.list`; existing renders pass `true` |
| `pages/BroadcastsPage.tsx` | `pages/BroadcastsPage.templatesRead.test.tsx` (new, drawer and modal mocked) | without Templates read: the drawer and the modal get `false`, a variation without template shows its ID; with it: `true`, and "Untitled" stays for a missing template |
| `components/automations/context/*` | `components/automations/context/AutomationContext.test.tsx` | the context exposes the provider's `canReadTemplates`; existing renders pass it |
| `components/automations/NodeConfigPanel.tsx` | `components/automations/NodeConfigPanel.test.tsx` | the stubs expose the prop: with `false` in the context, the trigger and email forms get `false` |
| `components/automations/config/TriggerConfigForm.tsx` | `components/automations/config/TriggerConfigForm.test.tsx` | link_clicked trigger: the mocked selector gets the prop |
| `components/automations/config/EmailConfigForm.tsx` | `components/automations/config/EmailConfigForm.test.tsx` (new) | the selector gets `canReadTemplates`, default `true` |
| `components/automations/config/ConditionsField.tsx` | `components/automations/config/ConditionsField.test.tsx` | with `false` in the context, the mocked `TreeNodeInput` gets `false` |
| `components/automations/nodes/EmailNode.tsx` | `components/automations/nodes/EmailNode.test.tsx` | without Templates read: the ID, not "Template set"; with it: the name, else "Template set" |
| `components/automations/UpsertAutomationDrawer.tsx` | `components/automations/UpsertAutomationDrawer.test.tsx` (new, provider mocked) | the drawer passes `canReadTemplates` to `AutomationProvider` |
| `pages/AutomationsPage.tsx` | `pages/AutomationsPage.test.tsx` | the permission mock becomes settable; the existing case gets `templates: { read: true }`; without it: no `templates.list`, the drawers get `false`; with it: `templates.list` runs, `true` |
| `components/segment/input.tsx`, `form_leaf.tsx` | `components/segment/form_leaf_templates_read.test.tsx` (new) | `TreeNodeInput` with `canReadTemplates={false}` and an email leaf with a template: no `templates.list`, the summary shows the ID; editing it, the selector shows the ID, disabled, no request; without the prop: `templates.list` runs |
| `components/segment/button_upsert.tsx` | `components/segment/button_upsert_preview.test.tsx` | renders pass the prop; the drawer passes it to `TreeNodeInput` |
| `components/contacts/SegmentsFilter.tsx` | `components/contacts/SegmentsFilter.test.tsx` (new, `ButtonUpsertSegment` mocked) | both the create button and a segment's edit button get the prop |
| `pages/ContactsPage.tsx` | `pages/ContactsPage.test.tsx` | the permission mock becomes settable; `SegmentsFilter` (mocked) gets `false` and `true` |
| catalogues | `i18n/locales/catalogues.test.ts` | every entry translated; compilation checked by `npm run build` |

`npx tsc -b` checks the tests too, so every existing render of a component whose prop becomes required is updated.

## Known, not handled

- Other template reads such a member still triggers on a click: `MessageHistoryTable`'s template preview, the blog
  post drawer, the email builder preview.
- `TemplateSelector` (`components/templates/TemplateSelector.tsx`) is exported and used nowhere.
- A member without Templates read still cannot choose a template for a new list, broadcast or email node, or clear a
  template filter: the lock says why.

## Test commands

```
cd console && npm run lingui:extract && npx vitest run && npx tsc -b && npx eslint . && npm run build
```
