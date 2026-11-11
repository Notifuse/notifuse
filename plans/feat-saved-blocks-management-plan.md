# Saved blocks: manage them from the library, and a richer "+" popover

Lots 2 and 3 of the custom blocks study (`/mnt/project-files/etudes/custom-blocks-ux-bibliotheque.md`, P4 and P2).
Builds on the library of #517 (`feat/saved-blocks-library`). This branch, `feat/saved-blocks-management`, starts
from it, and its PR goes to `dev` once #517 is merged. Frontend only: the API already has `templateBlocks.update`
(id, name, block) and `templateBlocks.delete`, both gated on Templates write. No schema, migration or env change.

## Lot 2 — manage a saved block from its card (P4)

### Behaviour

Each saved block card in the Library gets a "More actions" button (`…`, `aria-label="More actions for {name}"`)
opening an antd `Dropdown` menu. It appears only when the editor is given `canManageSavedBlocks` (the drawer
passes the member's Templates write permission); a member without it sees the cards as today.

- **Rename**: turns the card's name into an input, focused and selected. Enter or blur saves; Escape cancels.
  An empty or unchanged name, after trimming, saves nothing. At most 255 characters (`maxLength`), as the API
  checks. While the request runs the input is disabled; on failure the old name stays and the error message
  shows (the handler's own `message.error`).
- **Replace with the selected block**: enabled when the selected block is under the body, is not the body, and
  has the saved block's type; otherwise disabled with the reason in its title ("Select a block of the same
  type"). Confirmation (`Modal.confirm`): "Replace “{name}” with the selected block?" and "Templates that
  already contain this block do not change." OK sends the selected block through the existing
  `onSaveBlock(block, 'update', id)`; the card is then highlighted ("Updated" tag is not added: the highlight
  border only).
- **Duplicate**: `onSaveBlock(saved.block, 'create', t\`${name} (copy)\`)`, cut to 255 characters. The copy is
  highlighted and tagged "New", as after a save from the canvas.
- **Delete**: confirmation (`Modal.confirm`, danger): "Delete “{name}” from the library?" and "It is removed
  for every template of the workspace. Templates that already contain this block do not change." OK calls
  `onDeleteSavedBlock(id)`.

The menu's actions are disabled on a card while one of its requests runs. Errors leave the library unchanged
and are reported by the handlers.

The study's "Insert after the selection" menu item is left out: the card's Insert button already does it.

### Wiring

- `types.ts`: `RenameSavedBlockHandler = (id: string, name: string) => Promise<void>` and
  `DeleteSavedBlockHandler = (id: string) => Promise<void>`.
- `EmailBuilder` props: `canManageSavedBlocks?: boolean` (default false), `onRenameSavedBlock?`,
  `onDeleteSavedBlock?`. The menu shows only when `canManageSavedBlocks` is true; Rename and Delete only when
  their handler is given. EmailBuilder passes to `LibraryPanel` the selected block (for Replace), and
  `onDuplicate` / `onReplace` built on `onSaveBlock`, which set the highlight like `handleBlockSaved`.
- `CreateTemplateDrawer`: `handleRenameSavedBlock` (update with the stored block and the new name; success
  message `Block renamed to "{name}"`) and `handleDeleteSavedBlock` (delete; success `Block "{name}" deleted`).
  Both refresh the saved blocks the way `handleSaveBlock` does (the invalidate + `refreshWorkspaces` part is
  moved into one `refreshSavedBlocks()` used by all three). `canManageSavedBlocks` comes from
  `useWorkspacePermissions(workspace.id).permissions?.templates?.write`.
- `TemplateTranslationsTab`: passes the three props through to its EmailBuilder, like `onSaveBlock`.

## Lot 3 — the "+" popover shows every saved block (P2)

In content mode (`mode === 'content'`; header mode is unchanged):

- The "Saved blocks ({n})" tab is always there, `n` being every saved block of the workspace, also at 0. Empty:
  "No saved blocks yet" and "Save a block from its toolbar, or find your saved blocks in the Library tab."
- The list puts first the saved blocks that can go at this place (`takesFromLibrary(parent, block)`, and no
  `liquidPlacementError(tree, parentId, block)`), then, under a "Not available here" heading, the others,
  greyed, `aria-disabled`, not clickable, with the reason as their title: "Goes inside a column", "Goes inside a
  section", "Goes inside a social block", "Not allowed here" (from `nestingReason`), or "Not allowed inside a
  conditional block". Sorted newest first in each group.
- Each available row shows a small thumbnail (`BlockThumb`, 56 px wide, 32 px high at most) before its name.
- Hovering an available row shows on the right: a `BlockThumb` preview (268 px wide, 220 px high at most), the
  name, the type label, the date, the note "A copy is inserted. Later changes to the saved block do not change
  it.", and the "Add" button.
- The last tab used ("blocks" or "saved") is remembered in `localStorage` under
  `notifuse.addBlockPopover.tab`, with every read and write in try/catch; reading fails → "blocks".
- The popover needs the email for thumbnails and the Liquid check: TreePanel already passes `emailTree`.

The saved block tab of the popover keeps using `onAddSavedBlock`, unchanged.

## Files and tests

| Implementation | Tests | Cases |
|---|---|---|
| `ui/SavedBlockCard.tsx` | `ui/SavedBlockCard.test.tsx` | no menu without `canManage`; menu lists Rename, Replace, Duplicate, Delete; rename: Enter saves trimmed name, Escape and unchanged/empty save nothing, input disabled while pending, old name kept on failure; Replace disabled with reason when no or other-type selection |
| `panels/LibraryPanel.tsx` | `panels/LibraryPanel.test.tsx` | passes selection type to cards; Replace enabled for a same-type selection; actions reach the handlers with the saved block's id |
| `EmailBuilder.tsx` | `EmailBuilder.test.tsx` | duplicate calls `onSaveBlock(block,'create','X (copy)')` and highlights the new id; replace confirms then calls `onSaveBlock(selected,'update',id)`; delete confirms then calls `onDeleteSavedBlock(id)`; cancel calls nothing; no menu when `canManageSavedBlocks` is false |
| `templates/CreateTemplateDrawer.tsx` | `templates/CreateTemplateDrawer.test.tsx` | rename sends update with the stored block and the new name, then refreshes; delete sends delete then refreshes; errors show and reject |
| `templates/TemplateTranslationsTab.tsx` | existing tests | still renders with the new props |
| `ui/AddBlockPopover.tsx` | `ui/AddBlockPopover.test.tsx` (new or extended) | saved tab always present with the total count, also 0; available first, unavailable under "Not available here" with the reason and not clickable; Liquid refusal under a conditional parent; hover shows preview, type and copy note; last tab remembered and storage errors ignored; header mode unchanged |
| e2e | `e2e/features/library-management.spec.ts` (new) | real clicks: rename by Enter (API body: id, new name, stored block), Escape cancels; duplicate (create body "X (copy)", new card tagged New); replace with a selected section (update body has the selected block) and disabled for another type; delete with confirmation (delete body), cancel sends nothing; member without Templates write sees no menu |
| e2e | `e2e/features/add-block-popover.spec.ts` (new) | open "+" in a column: saved tab count, text available, section under "Not available here" with its reason on hover; adding a saved text inserts it there (tree sent on save); tab remembered after closing and reopening |

Translations for every new string in ca, de, es, fr, it, ja, pt-BR. CHANGELOG: lots 2 and 3 are unreleased, so
the 44.0 library Feature bullet gains a sentence (rename, replace, duplicate and delete from the library; the "+"
menu shows every saved block, with why some cannot go there) instead of new bullets. Docs: a follow-up PR on
`Notifuse/docs` from `dev` once docs#35 is merged, extending the "Saved blocks library" section.

## Order of work

1. Tests first for each unit, then code: card menu and rename, then LibraryPanel and EmailBuilder wiring, then
   drawer handlers, then the popover.
2. e2e specs.
3. Strings, CHANGELOG.
4. Code review (Opus, effort high), fixes, delta review.
5. Once #517 is merged: merge `dev`, full checks, PR to `dev`.

## Test commands

From `console/`:

- `npx vitest run src/components/email_builder src/components/templates`
- `npx tsc --noEmit -p tsconfig.app.json`
- `npx eslint src/components/email_builder src/components/templates e2e --max-warnings 0`
- `npx vitest run` (full suite, once before review)
- `npm run build`
- e2e against the dev server: `library-management`, `add-block-popover`, `editor-library`, `saved-blocks`,
  `editor-drag-and-drop`, `templates`.

## Revision 2 — after the plan check (Opus, effort high)

Decisions that replace the text above where they differ:

- **Permission**: `CreateTemplateDrawer` takes `canManageSavedBlocks?: boolean` (default false) instead of calling
  `useWorkspacePermissions` (each mounted drawer would fetch members). `TemplatesPage` passes
  `permissions?.templates?.write`; other entry points pass it when they already hold permissions, else no menu.
- **One library change at a time** across the whole panel (the backend rewrites `settings.template_blocks` without
  a lock): `EmailBuilder` keeps a `libraryBusy` flag; every card's menu and rename are disabled while it is set.
- **Highlight**: `highlight?: 'new' | 'updated'`. Duplicate → `'new'` (tag "New"); Replace and Rename →
  `'updated'` (border, no tag). The changed card is scrolled into view (it moves to the top: the list is sorted by
  modification date). Duplicate does not remount the library when it is already showing (no `libraryKey` bump).
- **Names are limited to 255 bytes** (Go `len`): the rename input counts UTF-8 bytes (antd `count` with a
  `TextEncoder` strategy) and refuses more; the copy name keeps the translated " (copy)" suffix and cuts the
  original name by code point so the whole fits in 255 bytes (`utils/blockName.ts`, `fitUtf8`).
- **Confirmations** use `App.useApp().modal.confirm` with translated OK and Cancel.
- **Rename**: the card stops being draggable while its name is edited; the input is focused in an effect once the
  menu has closed; Enter then blur saves once, Escape then blur saves nothing. Rename sends the block held in
  memory: a block replaced elsewhere since the page loaded comes back (accepted, noted in the PR).
- **The actions row stays visible while the card's menu is open.**
- **Delete**: a not-found answer counts as already deleted, then refresh. Each operation's refresh failure warning
  says what was done ("Block renamed…", "Block deleted…").
- **Popover**: looks the parent up with `EmailBlockClass.findBlockById(emailTree, parentId)`; availability keeps
  today's rule (`getAvailableBlocksForParent(parentType)`, so raw HTML and Liquid stay insertable in a section) plus
  `liquidPlacementError`; hidden block types stay filtered out entirely; the reason shows as visible text under the
  row's name; the lists are computed only while the popover is open; rows grow to fit the thumbnail. The tab is
  remembered in content mode only, read from storage each time a popover opens, and falls back to "blocks" when
  the tab is missing. Empty state: "No saved blocks yet" and "Save a block from its toolbar to reuse it in every
  template."
- **Translations tab**: its props interface gains the three props and passes them on (tested on its EmailBuilder).

Extra tests: menus of other cards disabled while one change runs; duplicate keeps search and filter; replace
highlights without "New"; copy name of a 255-byte multibyte name; rename refuses a too-long CJK name; Enter+blur
once, Escape+blur none; delete not-found; popover tab shared between two mounted "+"; header mode falls back to
"blocks"; reasons as visible text. e2e confirm buttons in French.
