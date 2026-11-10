# Fix the "Save Block" dialog of the email editor

Branch `fix/save-block-modal`, from `dev` (c859e1c). PR to `dev`. Console only: no table, no migration,
no environment variable.

## Problem

Saving a block from the canvas toolbar (floppy icon, `console/src/components/email_builder/ui/BlockToolbar.tsx`)
has four defects, all reproduced in the console:

1. **The dialog offers to overwrite an unrelated saved block.** `handleSaveClick` (lines 44-101) looks for a saved
   block "matching" the selected one. A block with `content` matches on content; any other block (section,
   column, wrapper, group, social…) matches when its attributes, minus the four long-hand `paddingTop/Right/Bottom/Left`
   keys, serialise to the same JSON (line 66, lines 76-79). Two white sections are enough: saving a product section
   opens the dialog in "Update existing block" mode with the workspace's saved header section preselected, and one
   click on "Update" replaces the content of that library entry (its name is kept) for the whole workspace.
   Templates that already contain the header are not affected (blocks are inserted as copies), but the next
   insertion of "Header" inserts the product section.
2. **"Update existing block" lists saved blocks of every type** (lines 243-272): a saved section can be replaced
   by a button.
3. **The success message does not reflect the save.** `handleSaveBlock` (lines 103-127) calls `onSave` without
   awaiting it, shows its own `message.success`, and closes the dialog. The real save runs in
   `CreateTemplateDrawer.handleSaveBlock` (lines 554-605), which shows a second success message, or an error
   right after the first success when the API call fails; the dialog is already closed, so the name is lost.
   A failure of `refreshWorkspaces()` after a successful API call is also reported as an error (the success
   toast, then the error's message), which would invite a second save and a duplicate.
4. `savedBlocks?.sort(...)` (line 259) sorts the prop array in place.

## Changes

### `console/src/components/email_builder/types.ts`

Add, next to `SaveOperation`:

```ts
// Saves a block in the workspace library. Resolves once saved; rejects when the save failed,
// after the caller has told the user why.
export type SaveBlockHandler = (block: EmailBlock, operation: SaveOperation, nameOrId: string) => Promise<void>
```

Use it instead of the inline `(block, operation, nameOrId) => void` signature in `EmailBuilder.tsx:31`,
`panels/EditPanel.tsx:18`, `EmailBlockClass.tsx:751`, `blocks/BaseEmailBlock.tsx:15`,
`ui/BlockToolbar.tsx:13` (`onSave`) and `templates/TemplateTranslationsTab.tsx:37`. These only pass the handler
down; no behaviour changes there.

### `console/src/components/email_builder/ui/BlockToolbar.tsx`

- `handleSaveClick`: remove the matching heuristic. The dialog always opens in "Save as new block" mode, with
  the current default name (`section Block`, unchanged in this fix) and no preselected block.
- `sameTypeSavedBlocks`: a new array of the saved blocks whose `block.type` equals the selected block's type,
  sorted newest `created` first (copy, no in-place sort). The "Update existing block" radio is disabled when it
  is empty; the select lists only these blocks.
- `handleSaveBlock`: ignore a call while `saving` (Enter in the name field during a pending save);
  `try { await onSave(...); close and reset } catch { /* the handler already told the user; keep the dialog open
  with what was typed */ } finally { setSaving(false) }`, so no rejection escapes to `onOk` / `onPressEnter`.
  Remove the toolbar's own `message.success` calls and the now unused `App.useApp()`.
- While `saving`, the dialog cannot be dismissed: Cancel button disabled (`cancelButtonProps`), `closable`,
  `mask.closable` and `keyboard` false. Otherwise a cancel during a slow save, then a reopen, lets the first save's
  completion close and reset the second dialog.

### `console/src/components/templates/CreateTemplateDrawer.tsx`

`handleSaveBlock` becomes a `SaveBlockHandler`:

- Only the API call is inside the `try`. On API failure: `message.error(...)` as today, then rethrow so the
  toolbar keeps the dialog open.
- `update` of an id missing from `workspace.settings.template_blocks`: `message.error(t\`Template block not found\`)`
  as today, then throw.
- After a successful call: the success message as today (the only one shown), then
  `queryClient.invalidateQueries` as today and `await refreshWorkspaces()` (awaited, so the saved-blocks list is
  current when the dialog closes). A refresh failure is not rethrown, since the block is saved: it is logged with
  `console.error` and a warning says so, `t\`Block saved. Reload the page to see it in your saved blocks.\``, so
  the stale list does not invite a second save.
- Remove the `delete` branch: `SaveOperation` is `'create' | 'update'`, nothing calls it with `delete`, and the
  typed handler makes that comparison a type error. Deleting from the editor is part of the later library work.

### Translations

`cd console && npm run lingui:extract`: three strings become obsolete (`Block saved as "{0}"`, `Block "{0}" updated`,
`Template block "{0}" deleted successfully`); one new string, the refresh warning, translated in every catalog.

### Version and changelog

`dev` already moved to 43.1 (unreleased, with the web analytics SDK synced) while this branch was open, so no
version change here. `CHANGELOG.md`, under `## [43.1]`:

  `- **Fix**: In the email editor, saving a block now always starts as a new saved block, and "Update existing block" only lists saved blocks of the same type. Before, the dialog could open on updating another saved block that merely looked alike. The confirmation now appears once the block is saved, and the dialog stays open if saving fails.`

## Tests (written first)

### `console/src/components/email_builder/ui/BlockToolbar.test.tsx` (new, Vitest + Testing Library)

Rendered inside antd `App`. Selected block: an `mj-section` with no `content` key and attributes
`{ backgroundColor: '#ffffff', paddingTop: '8px', paddingBottom: '8px' }`. Saved blocks: a section "Header" whose
attributes have the same keys in the same order but `paddingTop: '24px'` (so today's heuristic matches it), a
newer section "Footer" with a grey background, and a button "CTA" (newest).

Harness constraints found in review: `src/__tests__/setup.tsx:123` mocks `getComputedStyle` (always
`display: block`), and antd keeps a closed modal in the DOM, so "open" / "closed" is read from the inline
`style.display` of `.ant-modal-wrap` (`''` open, `'none'` closed), never from `toBeVisible` or the dialog role.
Select options are read from `.ant-select-item-option-content` (rc-select exposes only a few options by role).
Toasts are counted with `document.querySelectorAll('.ant-message-notice')`.

1. `opens in "Save as new block" mode even when a saved section looks alike`: click the save button; the
   "Save as new block" radio is checked, the name field shows `section Block`, no block select.
2. `lists only saved blocks of the selected block's type when updating, newest first`: switch to update, open
   the select; option texts are exactly `Footer (mj-section)`, `Header (mj-section)`, in that order.
3. `disables "Update existing block" when no saved block has the selected block's type`: saved blocks = the
   button only; the update radio is disabled.
4. `does not reorder the savedBlocks prop`: the array passed in keeps its order after opening the dialog and
   the select.
5. `saves a new block and closes once the save resolves`: type a name, click Save; `onSave` called once with
   `(block, 'create', 'Products')`; while the promise is pending the dialog is still open; after it resolves the
   wrap's display is `none`; zero `.ant-message-notice`.
6. `keeps the dialog open with the typed name when the save fails`: `onSave` rejects; the wrap's display is
   still `''`, the name field holds `Products`, the Save button has no `ant-btn-loading` class.
7. `cannot be dismissed while a save is pending`: `onSave` pending; the Cancel button is disabled, the close
   icon absent, and neither Escape nor a click on the mask closes the dialog. The dialog is opened with a plain
   click: hovering the save button shows its tooltip, which in the test environment takes the dialog off the
   Escape stack.
8. `updates the chosen saved block` (characterisation, also passes before the fix): update mode, pick `Footer`,
   click Update; `onSave(block, 'update', 'footer-id')`.
9. `ignores Enter while a save is pending` (guards the new `await`; before the fix a second Enter saved the
   block twice): `onSave` pending; Enter pressed and released twice (the input ignores a second keydown until keyup);
   `onSave` called once.

### `console/src/components/templates/CreateTemplateDrawer.test.tsx` (extended)

The `EmailBuilder` mock records its props (via `vi.hoisted`), so the tests call `onSaveBlock` directly;
`templateBlocksApi` and `useAuth().refreshWorkspaces` are mocks.

`templateBlocksApi` (`../../services/api/template_blocks`) and the `useAuth` mock become hoisted so the tests can
reach `refreshWorkspaces`.

1. `creates a template block, refreshes the workspace and shows one success message` (characterisation: the
   double toast came from the toolbar): API called with `{ workspace_id, name, block }`, refresh awaited,
   one `.ant-message-notice`, the promise resolves.
2. `rejects and shows the API error when creating a template block fails`.
3. `rejects without calling the API when updating a template block the workspace no longer has`.
4. `resolves with a warning when the block is saved but refreshing the workspace fails`: success and warning
   shown, no error message.

### `console/e2e/features/saved-blocks.spec.ts` (new, Playwright, API mocked like the other console e2e specs)

The workspace, overridden through `mockApi(page, 'user.me', …)` as in `content-rendering.spec.ts`, holds a saved
section "Header" whose attributes match the template's section but for long-hand paddings, and a saved button
"CTA". `requestCapture` has no `templateBlocks` pattern and unknown URLs get a silent `{}`, so the spec routes
`templateBlocks.create` and `templateBlocks.update` itself, recording calls, and answers 500 for case 3. CI does not
run Playwright: this is local evidence, run before review.

1. `saves a look-alike section as a new block`: select the section on the canvas, click the save button; the
   dialog is in new mode; Save; the captured request is `templateBlocks.create` with the name and an
   `mj-section` block, no `templateBlocks.update` request, and exactly one success message.
2. `offers only saved sections when updating a section`: update mode lists Header and not CTA.
3. `keeps the dialog open when saving fails`: `templateBlocks.create` answers 500; the error message shows,
   the dialog stays open.

## Known, not handled here

- The docs say editing a block changes every template that embeds it (`docs/features/admins-api-keys.mdx:91` and
  `:145`, `docs/empty.json:146` and `:210`). Blocks are inserted as copies (`EmailBuilder.handleAddSavedBlock` regenerates ids, nothing resolves
  blocks at compile time), so that sentence is wrong. Separate docs PR.
- `invalidateQueries({ queryKey: ['workspace', workspace.id] })` matches no query in the console; harmless, left.
- Default name `section Block`, raw type `mj-section` in the dialog, untranslated "Content structure" title, no
  rename/delete UI, no preview: covered by the saved-blocks library study (lot 1 and later).

## Verification

From `console/`: `npx vitest run src/components/email_builder/ui/BlockToolbar.test.tsx
src/components/templates/CreateTemplateDrawer.test.tsx` while developing, then the CI steps once: `npx tsc -b`,
`npx vitest run`, `npx eslint .`, `npm run build`; then `npx playwright test e2e/features/saved-blocks.spec.ts
e2e/features/templates.spec.ts` against the dev server. No Go or `web_analytics_sdk/` file changes: `make test-unit`
once as a sanity check.

## Plan review

Integrated after review: test assertions that passed without the fix (modal state, fixture paddings, option query),
characterisation labels, toast counts, dismissal during a pending save, explicit `catch`, awaited refresh with a
warning, three obsolete strings, e2e routing, WIP changelog heading, SDK sync through `prebuild`, docs locations.
