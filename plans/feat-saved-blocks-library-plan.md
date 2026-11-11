# Saved blocks library (lot 1): plan

Revision 3.1, 11 November 2026. Revision 1 was reviewed by Opus xhigh (Fable was rate limited):
`reviews/feat-saved-blocks-library-opus-plan-1.md`, 9 blocking findings (B1 to B9), taken into revision 2. Its delta
review, `reviews/feat-saved-blocks-library-opus-plan-2.md`, confirmed B1 to B9 fixed and found 3 more (N1 to N3),
taken into revision 3 with some of its non-blocking points. Revision 3's delta review,
`reviews/feat-saved-blocks-library-opus-plan-3.md`, confirmed N1 to N3 fixed and found one more (R1), taken into 3.1.

Study: `etudes/custom-blocks-ux-bibliotheque.md` (P1, P3). Mockups 10 to 12, 15, 16 and 19, 21 to 23 in
`maquettes/custom-blocks/`. Feasibility of the canvas drop: `etudes/drag-and-drop/bibliotheque-vers-canevas/`
(throwaway prototype; its canvas scenarios passed in Chromium with real mouse gestures; `prototype.patch` holds
`canvasDrop.ts`, `LibraryDnd.tsx`, `LibraryPanel.tsx`).

## Scope

Decided by Pierre:

- Lot 1 is P1 (a permanent library tab in the left panel, with an "Insert" button) plus P3 (a safer, clearer save
  dialog) plus dropping a saved block **on the canvas**. Dropping it **in the tree** is not in lot 1 ("Canevas
  seulement", 11 November).
- The library ships only if the canvas drop works; the prototype showed it does.

Depends on #505 (save dialog fixes), #510 and #514 (tree drag-and-drop) being merged into `dev`. The branch is
`feat/saved-blocks-library`, created from `dev` after those merges, with `git fetch origin dev` just before.

Not in scope: managing the library from the editor (rename, replace, duplicate, delete: lot 2, P4), the enriched "+"
popover (lot 3, P2), dropping in the tree, translating block type labels (the tree shows "Section", "Column" in every
language today; the library reuses `getLabel()` as is), any backend, SQL, migration or environment variable change.

### Product choices taken by default (study §5), to confirm

1. Names: the switch reads `Structure | Library (n)`, the list "Saved blocks", the tiles "Basic blocks".
2. The editor opens on **Structure**; the last tab used is not remembered. After an "Insert" or a drop the panel stays
   on Library (mockup 23 shows the tree because it illustrates a drop in the tree, which is out of scope).
3. "Insert" rules: see C.
4. Refused spots on the canvas, reconciling mockup 22 and the prototype's recommendation: **between sections, at body
   level** (mockup 22's case) a content block is refused, with no line and a badge giving the reason. **Inside a
   section** (under the shorter of two columns, on the section's padding) it snaps to the nearest column of that
   section that takes it, never outside the hovered section.
5. After an "Insert" or a drop, a message "Block inserted" (with the name as a parameter) and an "Undo" link that
   undoes only that insertion (see D, B2).

## Design

### A. Left panel: `Structure | Library` switch

- `EmailBuilder.tsx` replaces the untranslated "Content structure" title (study problem 6) with an antd `Segmented`
  switch, sticky at the top of the panel's scroll area. `leftTab` state: `'structure' | 'library'`, default
  `'structure'`. The Library label carries a badge with `savedBlocks.length` (grey at 0).
- `TreePanel` is unchanged and stays mounted only on the Structure tab, as today.

### B. Library panel (`panels/LibraryPanel.tsx`, new)

- Search by name, case and accent insensitive (`normalize('NFD')`, combining marks removed).
- "Saved blocks" with an `All / Sections / Content` filter (English msgids, translated): Sections are `mj-section` and
  `mj-wrapper`, Content everything else. Sorted by `updated`, falling back to `created`, newest first. Count shown next
  to the heading.
- One card per saved block (`ui/SavedBlockCard.tsx`, new): a thumbnail, the name, the type label and the modified
  date, formatted with `Intl.DateTimeFormat(i18n.locale, { dateStyle: 'medium' })` (B9: the console's dayjs loads no
  locale, so the template list shows English dates in every language). The card body is the drag handle (dnd-kit
  listeners on a `div`, not a `role="button"` around the "Insert" button). "Insert" and the hint "or drag the block
  into the email" are hidden with `opacity-0` and shown on hover and on `:focus-within`, never with `display: none` or
  `visibility: hidden`, so the button stays in the tab order (N3). "Insert" is disabled, with the refusal reason as its tooltip,
  when C refuses. A card can be highlighted with a "New" tag (see E).
- Thumbnail (`ui/BlockThumb.tsx`, new): renders the block with `EmailBlockClass.renderEmailBlock`, no-op handlers,
  the tree's attribute defaults (memoized on the tree), 600 px wide, scaled down to fit, `pointer-events: none` and
  `inert` through the spread `MjRawBlock.tsx` already uses (React 18 drops a boolean `inert`). Rendered once the card
  is near the viewport (`IntersectionObserver`, a fixed-height placeholder before); when `IntersectionObserver` does
  not exist (jsdom), it renders at once. The thumbnail is memoized on the block and on the head's attribute block, not
  on the whole tree, so typing in the email does not re-render the library. Its rendered blocks keep their
  `data-block-id`: every lookup by block id (hit testing, toolbar, e2e locators) is scoped to the canvas root.
- "Basic blocks": tiles for the block types `AddBlockPopover` offers in a body or a column, each insertable and
  draggable like a card. A tile goes through `handleAddBlock` with `{ wrapSections: false }` (B5, see D "Drop"), and a
  Section tile placed in a wrapper gets its default column too.
- Empty state (mockup 12): explains how to save a block, with a "Save the selected block" button shown only when a
  block under `mj-body`, other than the body, is selected and `onSaveBlock` exists. It opens the same dialog as the
  toolbar (B4, see E).

### Shared placement rule (`utils/libraryPlacement.ts`, new)

`takesFromLibrary(container, block)`: the container's `canAcceptChild(block.type)`, **and**, when the container is a
section or a group, the block is a column or a group (N1). It is a filter on `canAcceptChild`, never an override: a
group's `canAcceptChild` takes only columns (`MjGroupBlock.tsx:235-237`), so a group never takes a group (R1, from
`reviews/feat-saved-blocks-library-opus-plan-3.md`). The editor lets a section hold raw HTML and Liquid, which render as cells in its
row of columns (`MjSectionBlock.tsx:359-385`); the library never puts them there. C and D both use this rule, and the
Liquid check (`liquidPlacementError`) comes after it.

### C. "Insert" target (`utils/insertTarget.ts`, new)

`resolveInsertTarget(tree, selectedId, block: Pick<EmailBlock, 'type' | 'children'>)` returns
`{ parentId, index }` or `{ refusal: 'nesting' | 'liquid', reason }`. Pure. It takes the block, not only its type,
because `liquidPlacementError` also refuses a block that holds a Liquid block (B3).

1. A selection outside `mj-body` (in the head), or none, counts as no selection: the body takes the block at its end
   when it accepts the type.
2. With a selection S, from N = S upward:
   - **after N in its parent**, when the parent takes the block under the shared rule (N2: this single condition
     replaces revision 2's "stacking parent or column type", which could be read two ways);
   - else, for N = S only, **at the end of S**, when S takes the block under the shared rule;
   - else N climbs to its parent, up to the body.
3. The first level found decides. If it fails `liquidPlacementError`, the result is the refusal `'liquid'` there: no
   climbing past it, no other level (B3).
4. No level: refusal `'nesting'`. The reason comes from the types that accept the block (B7): "Select a column" for
   content blocks, "Select a section" for columns and groups, "Select a social block" for social elements, and "Not
   allowed here" otherwise (sections, wrappers, raw HTML and Liquid always find the body). The Liquid reason is "Not
   allowed inside a conditional block". The drag badge in D uses its own wording, since nothing is selected during a
   drag: "Goes inside a column", "Goes inside a section", "Goes inside a social block", "Not allowed here", "Not
   allowed inside a conditional block".

This keeps the study's rules (a section after the section holding the selection; a content block after the selected
block in its column or at the end of the selected column) and keeps raw HTML and Liquid out of the row of columns:
with a section selected they go after the section.

### D. Drop on the canvas

Built on the prototype, cleaned up:

- **Gesture**: `@dnd-kit/core` (already a dependency) `DndContext` in `EmailBuilder`, around the left panel and the
  canvas. `PointerSensor` with a 4 px activation distance, so a click on "Insert" never starts a drag. Escape cancels
  (dnd-kit's own handling; both drawers set `keyboard={false}`, so Escape does not close them). `DragOverlay` shows a
  small copy of the card. dnd-kit's screen-reader instructions and announcements are passed translated through its
  `accessibility` prop. Native HTML5 drag-and-drop is not used: an iframe swallows it, Tiptap would insert text on
  drop, and rc-tree's native drag stays independent.
- **Canvas lookup (B8)**: `EditPanel` exposes its canvas root and its scroll viewport through refs handed to the
  drag layer of the same `EmailBuilder`. No `document.querySelector`: the template drawer keeps the main editor
  mounted and hidden while the translations editor is open, and a document-level lookup would find the hidden canvas.
  Hit testing uses `document.elementsFromPoint` filtered to elements inside that root.
- **Target** (`utils/canvasDrop.ts`, new, pure): `resolveCanvasDrop({ tree, block, hoveredId, x, y, rectOf })`.
  - From the deepest rendered block under the pointer up to the body, the **first** level that takes the block under
    the shared rule decides: inside it at the slot nearest the pointer (vertical for stacked children, horizontal for
    columns in a section or group), or beside the hovered child, before or after by its half. Near the edge of a
    container whose parent also takes the block (10 px), the drop goes beside it. With the shared rule, raw HTML or
    Liquid over a section's padding or under its shorter column climbs to the body and goes before or after the
    section, never among its columns (N1).
  - If that level fails `liquidPlacementError`: `{ drop: null, refusal: 'liquid' }`, no climbing (B3).
  - No level, pointer inside a section: snap to the nearest column of that section that takes the block (choice 4).
    Pointer at body level: `{ drop: null, refusal: 'nesting', reason }` with the reasons of C.
  - The area of the canvas viewport below the email counts as the end of the body.
  - Returns the parent, the index and the indicator geometry (a line, or a box for an empty container).
- **Recompute (B6)**: the target is recomputed on every animation frame while a drag runs (the canvas may scroll with
  the pointer still), and once more at release from the last pointer position. The drop uses that last result.
- **Indicator** (`panels/CanvasDropIndicator.tsx`, new): portaled to `document.body`, fixed positioning. A #7763F1
  line across the target with an "Insert here" label, or a dashed box for an empty container; for a refusal, no line
  and a badge by the pointer with the reason. It carries `data-parent-id` and `data-index` for the e2e. Cleared on
  leave, cancel and drop.
- **Iframes**: while a drag runs, `EmailBuilder` sets `data-library-dragging` on its root, and CSS in `editor.css`
  gives every iframe inside `pointer-events: none`. Raw HTML blocks already have `inert` and an overlay; this third
  guard covers any future iframe.
- **Auto-scroll**: in the same animation-frame loop, the canvas viewport scrolls while the pointer is within 48 px of
  its visible top or bottom edge, speed proportional to the depth into that band. dnd-kit's own auto-scroll is
  disabled.
- **Drop and selection (B1)**: a saved block goes through `handleAddSavedBlock(parentId, block, index)`, a tile
  through `handleAddBlock(parentId, type, index, { wrapSections: false })`. Changes to both handlers:
  - they return the new block's id, or `null` when they refused (Liquid) or failed; the success message shows only
    for an id;
  - they select the new block through `handleSelectBlock`, so a controlled selection (the template drawer passes
    `selectedBlockId` and `onSelectBlock`, and `EmailBuilder.tsx:132` lets it win) follows; today they set only the
    editor's own state, which loses to the drawer's;
  - `handleAddBlock`'s "first wrapper wraps every section and goes first" branch (`EmailBuilder.tsx:375`) runs only
    when `wrapSections` is not `false`, so the tree's "+" keeps it and the library places an empty wrapper where the
    line shows (B5).
  Every library call passes an explicit index (`handleAddSavedBlock` turns `undefined` into 0).
- **Message and Undo (B2)**: after an insertion, `message.success` with an "Undo" link. The editor records the
  history index the insertion produced. The link calls the latest undo through a ref (not the render's closure), and
  acts only while the history index still equals the recorded one; the message closes on the next history change.
  After that Undo, the selection is cleared through `handleSelectBlock(null)`, so the drawer does not keep the id of a
  removed block.

### E. Save dialog (P3), `ui/SaveBlockModal.tsx` (new, extracted from `BlockToolbar`)

B4: the dialog moves out of `BlockToolbar`, which `EditPanel` mounts only for a selected block on the canvas.
`SaveBlockModal` is controlled; `EmailBuilder` owns `saveDialogBlockId` and an `openSaveDialog(blockId)` callback
given to `EditPanel` (for the toolbar's save button) and to `LibraryPanel` (for the empty state). #505's behaviour
stays: always starts as a new block, "replace" lists only blocks of the same type, success only after the API
answers, the dialog stays open on failure, a double Enter starts one save. Changes:

- Title and toolbar tooltip: "Save to library". Ok button: "Save" or "Replace".
- A thumbnail of the block (`BlockThumb`, 470 px wide) and its type label instead of `mj-section`.
- Suggested name (`utils/suggestBlockName.ts`, new): type label, then " · " and the first words of the first text or
  button content found depth first (HTML and Liquid tags stripped, whitespace collapsed, at most 32 characters with
  an ellipsis); else the first image's `alt`; else the type label alone.
- A choice "New block / Replace an existing block (n)", n counting blocks of the same type, disabled at 0.
- An info line: the block is available in every template of the workspace, and templates that already contain it do
  not change.
- `SaveBlockHandler` resolves with the saved block's id (`Promise<string>`): `CreateTemplateDrawer.handleSaveBlock`
  returns `response.block.id` on create and the id on update. `TemplateTranslationsTab` passes the same handler to its
  own editor. `EmailBuilder` gets the id directly from the dialog it owns: on a **create**, it switches the left panel
  to Library, clears the search and the filter, and highlights that card with "New" until the next tab change
  (mockup 16). A replace shows the success message only, without "New".

### F. Canvas height (existing defect)

`EditPanel` sets `height: calc(100vh - 58px)` on the canvas container (shipped in v42.0). It is 65 px taller than the
space left in the drawer, so the bottom of a long email cannot be reached by scrolling, clicking or dragging. Replace
it with the flex height of its parent (`h-full min-h-0`), and check the drawer and the translations editor both still
fill the screen.

### G. Strings, changelog, docs

- All new strings through `useLingui` `t` or `<Trans>`, with English msgids; `npm run lingui:extract`; translations
  filled for every locale in `lingui.config.ts` (ca, de, es, fr, it, ja, pt-BR), as #505 did.
- `CHANGELOG.md`, under `## [44.0] - unreleased`:
  - one Feature bullet: the library tab with previews and search, "Insert", dragging a saved block or a basic block
    onto the email;
  - #505's unreleased bullet about the save dialog is rewritten to describe the dialog as it ships (it names "Update
    existing block", which this lot renames), instead of adding a line;
  - one Fix bullet for the canvas height (the defect shipped in 42.0).
- A docs PR (`docs/saved-blocks-library`, from `dev` of Notifuse/docs) describing the library; it also corrects
  `features/admins-api-keys.mdx`, which says editing a saved block changes every template.

## Files and tests

| Implementation | Test file | Test cases |
|---|---|---|
| `utils/insertTarget.ts` | `utils/insertTarget.test.ts` | section with a text selected goes after its section; text with a text selected goes after it in its column; text with a column selected goes at the column's end; column with a column selected goes after it; raw HTML with a section selected goes after the section, not among its columns; saved column with a text selected goes after that text's column; saved group with a text selected goes after that text's column; saved group with a text in a group's column selected goes after the group in its section, not inside the group; text with nothing selected is refused with "Select a column"; saved column with nothing selected is refused with "Select a section"; section with nothing selected goes at the body's end; a selection in the head counts as none; Liquid with a block inside a conditional section selected is refused with `liquid` and does not climb; a saved section holding a Liquid block with a block in a conditional wrapper selected is refused with `liquid`; a selected block inside a wrapper puts a section after the inner section |
| `utils/canvasDrop.ts` | `utils/canvasDrop.test.ts` (rects from a layout stub, as `TreePanel.test.tsx` does) | section over a section's top half and bottom half; section over the area below the email goes last; section in an empty wrapper (box); section between two sections of a wrapper; text between two texts of a column, at top, at bottom; text in an empty column (box); column beside columns (horizontal line); near a column's stacked edge goes beside it; raw HTML near a column's edge does not land between columns; raw HTML and Liquid over a section's padding go before or after the section; raw HTML under the shorter of two columns goes after the section; raw HTML over a group stays out of the group's row; text between two sections is refused with the reason; column over the body is refused with "Goes inside a section"; group over a group's column goes beside the group in its section, not inside it; text under the shorter of two columns snaps into that section's column; snapping never leaves the hovered section; Liquid over a conditional block is refused with `liquid`; a section holding Liquid over a conditional wrapper is refused; unknown hovered id gives null |
| `utils/suggestBlockName.ts` | `utils/suggestBlockName.test.ts` | section with a heading text; text with Liquid and HTML stripped; long text cut at 32 characters with an ellipsis; image with alt; block with no text gives the type label |
| `ui/BlockThumb.tsx` | `ui/BlockThumb.test.tsx` | renders the block's content; carries `inert` and is not focusable; renders a placeholder until visible (IntersectionObserver stub); renders at once without IntersectionObserver |
| `utils/libraryPlacement.ts` | `utils/libraryPlacement.test.ts` | a section takes a column and a group, not raw HTML or Liquid; a group takes a column, not a group, raw HTML or Liquid; a column takes a text; the body takes raw HTML |
| `ui/SavedBlockCard.tsx` | `ui/SavedBlockCard.test.tsx` | shows name, type label and date; the date follows the `fr` locale; "Insert" calls back with the block; disabled "Insert" shows the refusal reason; "Insert" is hidden with opacity, not `display` or `visibility`; "New" tag when highlighted |
| `panels/LibraryPanel.tsx` | `panels/LibraryPanel.test.tsx` | lists newest first; search ignores case and accents; filters Sections and Content; count; empty state without a selection has no save button; empty state with a selected block opens the save dialog for it; no save button with the body selected; tiles insert a basic block |
| `panels/CanvasDropIndicator.tsx` | `panels/CanvasDropIndicator.test.tsx` | line with label and data attributes; box for an empty container; refusal badge with the reason; rendered in `document.body` |
| `dnd/LibraryDnd.tsx`, `dnd/useCanvasDropTarget.ts` | `dnd/LibraryDnd.test.tsx` | drag start shows the overlay; a move over a block sets the indicator; drop calls the insert handler with parent and index; the rects change after the last move and the drop uses the new ones; drop on a refused spot inserts nothing; Escape and leaving the canvas clear the indicator; the root carries `data-library-dragging` during the drag only; auto-scroll moves the viewport near its edge (fake rAF); with two builders mounted, the drop resolves in the builder that owns the drag; accessibility strings are the translated ones |
| `ui/SaveBlockModal.tsx` | `ui/SaveBlockModal.test.tsx` (#505's dialog tests moved from `BlockToolbar.test.tsx`) | #505's cases kept (new by default, same-type replace list, success after the API, stays open on failure, single save on double Enter); title, thumbnail and type label; suggested name; replace count and disabled at 0; resolves the saved id |
| `ui/BlockToolbar.tsx` | `ui/BlockToolbar.test.tsx` | save button calls `openSaveDialog` with its block id; clone and delete kept |
| `EmailBuilder.tsx` | `EmailBuilder.test.tsx` | switch shows Structure by default and the Library count; "Insert" with a controlled selection calls `onSelectBlock` with the new id; two "Insert" in a row keep their order, with the selection held by a stateful wrapper like the drawer's; after the message's Undo the selection is cleared; a refused insert shows no success message; a Wrapper tile between sections inserts an empty wrapper at that index and the sections keep their ids; tree "+" still wraps the sections into the first wrapper; after an edit then an insertion, the message's Undo keeps the edit and Redo restores the insertion; after an insertion then an edit, the message's Undo changes nothing; a create switches to Library, clears search and filter, and highlights the card; a replace does not highlight |
| `panels/EditPanel.tsx` | `panels/EditPanel.test.tsx` (new) | canvas container uses the parent's height, not `100vh`; the toolbar's save button calls `openSaveDialog`; canvas root and viewport refs are set |
| `templates/CreateTemplateDrawer.tsx` | `templates/CreateTemplateDrawer.test.tsx` | `handleSaveBlock` resolves with the created block's id, and with the id on update |
| `templates/TemplateTranslationsTab.tsx` | existing tests | still saves blocks with the new handler type |
| e2e | `e2e/features/editor-library.spec.ts` (new), `e2e/features/saved-blocks.spec.ts` (updated) | ported from the prototype's spec with real mouse gestures, checking the tree sent to the API and the indicator's `data-parent-id`/`data-index`: section between sections, before the first, after the last, below the email; section in an empty wrapper; text between blocks and in an empty column; column beside columns with widths rebalanced; over a raw HTML block, also with `inert` and the overlay removed (the prototype's variants); Liquid refused over a conditional block; long email: hold in the scroll band until the canvas scrolls, release without moving, the inserted index is the one shown just before release; Escape and leaving insert nothing; one undo; with a text selected, "Insert" then the inserted block is the selected one; keyboard Tab reaches a card's "Insert" and Enter inserts; drop in the translations editor while the main editor is mounted; save dialog then library highlight (mocks: `templateBlocks.create` answers `{ block: { id, ... } }` and the refreshed workspace includes it); bottom of a long email reachable |

## Order of work

1. F (canvas height), test first.
2. C and `suggestBlockName`, pure, test first.
3. E (dialog extraction, handler id), then A and B with the "Insert" button and the handler changes (B1, B5, B2).
4. D (canvas drop), unit tests first, then the e2e.
5. Strings, changelog, docs PR.

Quality steps as usual: revision 2 gets a delta review of the plan, then TDD, an independent code review, a delta
review after any later commit.

## Test commands

From `console/`:

```
npx tsc -b
npx eslint .
npx eslint src/components/email_builder src/components/templates e2e --max-warnings 0
npx vitest run src/components/email_builder src/components/templates
npx vitest run
npm run build
npm run lingui:extract
```

`npm run lint` is not used: it passes `--max-warnings 0` over the whole console, which fails on `dev` today because of
unrelated warnings (the CI job says so in `.github/workflows/console.yml`). The touched folders are held to zero.

Then the e2e with Playwright against the mocked API: `editor-library.spec.ts`, `saved-blocks.spec.ts`,
`editor-drag-and-drop.spec.ts`, `templates.spec.ts`, and, because F changes `EditPanel` for every editor,
`content-rendering.spec.ts` and `raw-block-preview.spec.ts`. The dev server is stopped by PID afterwards and its port
checked free. No backend layer is touched, so no `make test-*` target is needed.
