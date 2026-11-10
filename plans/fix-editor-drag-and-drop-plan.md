# Fix drag-and-drop in the email editor's structure tree

Branch `fix/editor-drag-and-drop`, from `dev` (ff1ed695). PR to `dev`. Console only: no table, no migration, no
environment variable, no new user-facing string.

Audit (46 browser scenarios) and independent second opinion:
`/mnt/project-files/etudes/drag-and-drop/glisser-deposer-editeur-email.md`.

## Problem

Blocks are moved by dragging rows of the body tree (`panels/TreePanel.tsx`, antd `Tree` on `@rc-component/tree`).
The canvas accepts no drop; the head tree is not draggable. Verified defects:

1. **A block moved down inside its own parent lands one slot too far** (unless the target is the last slot).
   `handleDrop` takes the index from the pre-move tree; `EmailBlockClass.moveBlockInTree` removes the block before
   inserting. Dropping a block "after itself" also shifts it one slot.
2. **The drop line shows where the drop is refused.** `allowDrop` returns `true` for every gap; `handleDrop` then
   refuses silently (console warning only). Also the top half of the Body row.
3. **Reordering sections needs a precise sideways move.** Hovering the top of a section puts the gap after the last
   visible row above (often a text three levels down); rc-tree lifts the level only with the cursor moved left by
   12/36/60 px from the grab point, which grabbing near the label start does not allow.
4. **Dropping into an empty container fails with the natural gesture**: text onto an empty column, column onto an
   empty section, social element onto an empty social block (line shown, refused); section onto an empty wrapper
   lands after it; column onto an empty group lands beside it. Same for collapsed containers until auto-expand.
5. **Body and wrapper rows are draggable** (rc-tree ignores per-node `draggable`); the wrapper has no handle.
6. **Column widths**: moving, adding, duplicating or deleting a column resets the container's columns to equal
   widths (30/70 becomes 50/50) written as raw floats (`33.333333333333336%`); reordering columns inside a group
   resets them too.
7. A move that changes nothing pushes an undo step.
8. A drop into a collapsed container leaves it collapsed, so the block disappears from view.
9. Latent: `handleDrop` takes the index from the tree data, which filters `hiddenBlocks`, and inserts into the
   unfiltered children (no effect today: hidden types are head-only).

Group widths are out of this PR (decision pending with Pierre, recommended: separate gomjml fix): gomjml renders a
percentage `mj-group` width at 100% outside Outlook (no CSS rule registered for the group's class, unless a column
elsewhere happens to use the same integer percentage), so giving
groups a width in the editor would make the "group beside columns" case worse. Groups keep today's behaviour.

## Changes

### New `console/src/components/email_builder/utils/treeDrop.ts`

One pure resolver used both to decide whether a line is shown and where the block goes:

```ts
// rc-tree's relative drop position: before the row (first root row only), first child of the row, after the row
// (and its visible subtree)
export type DropSide = -1 | 0 | 1
// index: position in parent.children once the dragged block has been taken out
export interface DropTarget { parentId: string; index: number }
export function resolveDropTarget(
  tree: EmailBlock, dragId: string, dropId: string, side: DropSide, expandedKeys: readonly string[]
): DropTarget | null
```

`accepts(p) = EmailBlockClass.from(p).canAcceptChild(drag.type)`; a parent that is the dragged block or inside it
is never a candidate; `open(x)` = x has children and is in `expandedKeys`. Unknown ids return `null`. rc-tree calls
`allowDrop` before its own self/descendant checks, so drop = drag and drop inside drag must be handled.

Candidates are tried in order; **the first valid candidate decides, with no fall-through**: if it is the block's
current place, the result is `null` (no line, no move).

- `side 0` (first child of the row; rc-tree sends it for an open row, or a closed row with the cursor moved right):
  only `(drop, 0)`. Refusals stay invisible, as today.
- `side -1` (rc-tree gives it only on the first root row): `(parent(drop), index(drop))`, then `(drop, 0)` (top half
  of the Body row becomes "first in body").
- `side 1`:
  1. `drop` is an empty container that accepts: `(drop, 0)`. Filling an empty container wins over "after it".
  2. rc-tree's level: `(parent(drop), index(drop) + 1)`.
  3. Down the open chain, shallowest first: for `x = drop`, then its last child while `open(x)`: a container that
     accepts gives `(x, x.children.length)`. Covers collapsed containers and a level lifted above the body.
  4. Up the last-child chain: while `x` is the last child of `p` and `p` has a parent `pp`: `(pp, index(p) + 1)`,
     then `x = p`. Covers section reordering after a nested last row.

Indices are taken by id from the real `children` (fixes 9). When the dragged block is in the chosen parent before
the raw index, the index is decreased by one (fixes 1).

### `console/src/components/email_builder/panels/TreePanel.tsx`

- `allowDrop` prop: `resolveDropTarget(emailTree, dragNode.key, dropNode.key, dropPosition, expandedKeys) !== null`.
- `handleDrop` (typed rc-tree drop info, no `any`): side = `0` when `!dropToGap`, else the sign of
  `info.dropPosition - lastPosIndex`; resolve; on `null` return; `onMoveBlock(dragKey, target.parentId,
  target.index)`; if the target parent is not in `expandedKeys`, call `onTreeExpand([...expandedKeys,
  target.parentId])` (fixes 8).
- Remove the local `allowDrop`, `findParentId`, `getNodesWithPos` (dead).
- `mj-wrapper` joins `DRAGGABLE_NODES` (handle shown). The handle gets `aria-label` from its existing tooltip text
  (no new string). `TreeNodeTitle`'s root span gets `data-draggable="true|false"`.
- The body `Tree` is wrapped in a `div` whose `onDragStartCapture` finds the row with
  `target.closest('[role="treeitem"]')?.querySelector('[data-draggable]')` and, when it says `false`, calls both
  `preventDefault` and `stopPropagation` (with `preventDefault` alone rc-tree leaves the row in its dragging state).
  No row found (e.g. an event from a popover portal): allowed. The Body row is then not draggable but stays a drop
  target. (`draggable.nodeDraggable` is not used: rc-tree only attaches drop handlers to draggable rows.)

### New `console/src/components/email_builder/utils/columnWidths.ts`

```ts
export function rebalanceAfterAdd(container: EmailBlock, addedId: string): void   // mutates the given node
export function rebalanceAfterRemove(container: EmailBlock): void
```

- Slots: `mj-column` children of a section or a group (as today; groups are not slots in this PR). Other children
  (group, raw, liquid) are untouched. Any other container: no-op.
- After add: the added column's exact share is `100/n`. If every other column has a percentage width and they sum
  to 100 (±0.5), their exact shares are their widths scaled to `100 - 100/n` keeping their ratios (30/70 receiving
  one becomes 20 / 46.67 / 33.33); otherwise every exact share is `100/n` (as today).
- After remove: if every remaining column has a percentage width and they sum to more than 0, their exact shares are
  their widths scaled to 100 keeping ratios; otherwise equal. No remaining column: nothing.
- Rounding: every column but the last in document order gets its exact share rounded to hundredths, computed in
  integer hundredths; the last takes the remainder, so the total is exactly 100. Written without trailing zeros
  (`33.33%`, `50%`). Adding to 50/50 gives 33.33 / 33.33 / 33.34.

### `console/src/components/email_builder/EmailBuilder.tsx`

- `handleMoveBlock`: clamp the position, then return without a history step when the block's parent and index
  would not change. Widths, when the moved block is a column: source ≠ target: `rebalanceAfterRemove(source)` and
  `rebalanceAfterAdd(target, blockId)`; same parent: no change (fixes the group reorder reset).
- `handleAddBlock`, `handleCloneBlock`, `handleDeleteBlock`: the equal-width code becomes the helpers (column in a
  section or a group). The default "Column N" text of an added column is kept.
- `handleAddSavedBlock`: a saved column inserted into a section or a group calls `rebalanceAfterAdd`; its own
  saved width is replaced by its share, like an added column.
- Remove `EmailBlockClass.redistributeGroupColumnWidths` and `redistributeColumnWidthsAfterMove` (no caller left;
  no test touches them).

`moveBlockInTree` keeps its contract (final index): the AI assistant's tools rely on it.

### Version and changelog

`dev` is on 43.2 WIP: no version change. `CHANGELOG.md`, under `## [43.2] - WIP`:

`- **Fix**: In the email editor's structure tree, a block dragged down now lands where the line shows, and the line only appears where the block can go.`

`- **Improvement**: Blocks can now be dragged into empty columns, sections, wrappers, groups and social blocks, wrappers get a drag handle, and a section dragged below nested content finds its place without a sideways move. Moving, adding, duplicating or deleting a column keeps the other columns' proportions when their widths are percentages adding up to 100.`

## Tests (written first)

### `console/src/components/email_builder/utils/treeDrop.test.ts` (new)

Fixture: the audit tree (body: S1[C1[T1,BT1,IM1], C2[T2,SO[SE1,SE2]]], W[SW[CW[TW]]], S2[G[G1[TG1],G2[TG2]]],
S3[C3[D,SP,T3,T4]], R (raw), L (liquid)), all containers expanded, plus variants with empty containers (including a
second, empty social block) and a collapsed C3.

1. moving down in the same parent: D after T3 → `{C3, 2}`; T1 after BT1 → `{C1, 1}`; S1 after R → `{B, 4}`.
2. moving up: T4 after D → `{C3, 1}`; S3 side -1 on B → `{B, 0}`.
3. no-op returns null: SP after D; D after D; SW after SW (lifted from TW: with fall-through it would give `{B, 2}`).
4. cross-parent: T1 after T2 → `{C2, 1}`; T1 side 0 on C2 → `{C2, 0}`; R side 0 on C1 → `{C1, 0}`.
5. refused: T1 side 0 on S1, S2 side 0 on C1, T1 after SE1, T1 after L, C1 after L, T1 side -1 on B → null.
6. section reorder: S1 after TG2, after G2, after G and after S2 → `{B, 2}` (before S3); T1 after lifted G2 →
   `{G2, 1}`.
7. lifted above the body: SW after B → `{B, 6}` (end of body, SW taken out of W).
8. empty containers: T1 after an empty column → inside at 0; a column after an empty section → inside; SE1 after the
   empty social → inside; S1 after an empty wrapper → inside; C1 after an empty group → inside.
9. collapsed container: T1 after collapsed C3 → `{C3, 4}`; R after collapsed C3 → after C3 in S3 (rc-tree level
   first); T1 side 0 on collapsed C3 → `{C3, 0}`.
10. never into itself: C1 after T1 (its own child) → null; W after SW → null.
11. unknown drag or drop id → null, no throw.

### `console/src/components/email_builder/utils/columnWidths.test.ts` (new)

1. add to 30/70 → 20 / 46.67 / 33.33, total exactly 100.
2. remove one from 20/30/50 → the remaining two keep their ratio and total 100 (e.g. remove 50 → 40 / 60).
3. adding a column to 50/50, and equalising three columns with px widths → 33.33 / 33.33 / 33.34 (the last
   column in order takes the remainder).
4. px, `auto` or missing widths among the others, or percentages not summing to 100 → equal percentages; all-zero
   widths on remove → equal percentages, never `NaN%`.
5. a group and raw/liquid children in the section are untouched and not counted.
6. inside a group, columns rebalanced the same way.
7. last remaining column → 100%; no column → unchanged; a block that is not a section or group → unchanged.

### `console/src/components/email_builder/panels/TreePanel.test.tsx` (new)

Real `TreePanel` in jsdom. Drag sequences are `dragStart` / `dragEnter` / `dragOver` / `drop` on the
`[role="treeitem"]` rows, with `clientX`/`clientY` on the events and `getBoundingClientRect` stubbed on the target.

`HTMLElement.prototype.offsetWidth` is stubbed to 24 so rc-tree reads a real indent (jsdom gives 0, which makes
rc-tree ask for "inside" before "after").

1. D dropped on the lower half of T3 calls `onMoveBlock('D', 'C3', 2)` (dev: `('D', 'C3', 3)`).
2. a text dropped on the lower half of a collapsed C3 calls `onMoveBlock(text, 'C3', 4)` (end of C3) and
   `onTreeExpand` with C3 (dev: a line, no move, no expand).
3. a text dragged over the lower half of a social element shows no `.ant-tree-drop-indicator` after `dragOver`
   (dev: the line shows) and the drop does not call `onMoveBlock`.
4. with `hiddenBlocks={['mj-raw']}` and C3 = [raw, D, SP, T3, T4], T4 dropped on the lower half of D calls
   `onMoveBlock('T4', 'C3', 2)` (dev: index 1): the index counts every child, not only the rows shown.
5. the wrapper row has a drag handle (by its aria-label) and `data-draggable="true"`; the Body row has
   `data-draggable="false"`.
6. `dragStart` on the Body row is cancelled (`fireEvent.dragStart` returns false); on a section row it is not.

### `console/src/components/email_builder/EmailBuilder.test.tsx` (extended)

The `TreePanel` mock records its props (via `vi.hoisted`); the tests call `onMoveBlock` / `onAddBlock` /
`onCloneBlock` / `onDeleteBlock` / `onAddSavedBlock` inside `act()` and read the tree through the existing harness.

1. a move to the block's own place adds no undo step (Undo stays disabled).
2. moving the 50% column of a 20/30/50 section to index 2 of a 30/70 section: source 40 / 60 (dev: 50 / 50),
   target 20 / 46.67 / 33.33 (dev: `33.333333333333336%` three times).
3. reordering columns inside a 30/70 group keeps 30/70 (dev: 50 / 50).
4. adding a column to a 30/70 section: 20 / 46.67 / 33.33 and the new column holds the "Column 3" text.
5. duplicating a column of a 30/70 section, and deleting one of 20/30/50: proportions kept.
6. inserting a saved column into a 30/70 section: 20 / 46.67 / 33.33 (dev: widths untouched).
7. (characterisation) `moveBlockInTree(tree, id, parent, n)` still puts the block at final index `n`.

### `console/e2e/features/editor-drag-and-drop.spec.ts` (new, Playwright, API mocked)

Fixture tree as above; each case drags with real mouse gestures, saves the template, and reads the tree sent to
`templates.update`. The drag helper goes to `e2e/fixtures/tree-drag.ts`. CI does not run Playwright: local
evidence, run before review.

1. moving down: D below T3, T1 below BT1, S1 below R land where the line was.
2. moving up and across containers (a sample of the audit's passing cases, as regression guard).
3. refused positions show no line on a freshly entered row and change nothing (text between social elements, text
   below Liquid at body level, section at a text's level).
4. S1 dropped on the top half of S3 lands before S3 with the cursor at 0, -24, -48 and -72 px.
5. empty column, section, social, wrapper and group are filled by the natural gesture; a collapsed column receives
   a text and is expanded after the drop.
6. the wrapper moves by its handle; the Body row cannot be dragged.
7. a drop on the block's own place leaves Undo disabled.
8. a column moved from a 30/70 section into another section keeps proportions on both sides.

## Known, not handled here

- The line's indentation is rc-tree's (antd hard-codes `dropIndicatorRender`); when the resolver changes the level,
  the line is at the right height but its indentation can be several levels off.
- While a container is empty, the slot right after it is unreachable for a block both it and its parent accept
  (e.g. a section right after an empty wrapper, a column after an empty group followed by a column, raw/liquid
  after an empty column): the empty container takes the block. Lifting works when the empty container is the last
  child and the level above its parent does not accept the block (true for columns and sections, not for
  raw/liquid).
- The deepest accepting level wins when no sideways move is made: a section dropped just after a wrapper lands at
  the end of the wrapper, a column just after a group lands in the group, body-level raw/liquid dropped between
  sections lands at the end of the last column above. The sideways lift still reaches the outer level.
- rc-tree keeps the previous line when the cursor moves to a refused half of the same row; the drop then goes where
  that line shows.
- Column widths in px, `auto` or missing, or percentages that do not add up to 100 (on add), are still equalised
  to percentages, as today.
- Group widths: gomjml renders percentage `mj-group` widths at 100% outside Outlook, and its default is 100% where
  MJML shares the width between siblings; the canvas also applies an explicit group width twice
  (`MjSectionBlock.tsx:361-371` on the cell, `MjGroupBlock.tsx:288-291` on the inner div). Separate fix.
- The AI assistant's add, delete and move tools do not rebalance widths.
- Dragging from the saved-blocks library (lot 1) needs its own drag handling; it will reuse `resolveDropTarget`.
- Native autoscroll during a drag in a long tree is not covered by the e2e (synthetic drags do not trigger it).
- The Body row keeps antd's grab cursor.

## Plan review

First review: resolver confirmed against rc-tree (all expected results reproduced); integrated: group widths out of
scope (gomjml renders percentage group widths at 100%), no-fall-through rule and its distinguishing case, row lookup
through `[role="treeitem"]` with both `preventDefault` and `stopPropagation`, jsdom drag tests of `TreePanel`, width
tests that fail on dev, saved-column insertion, sum-to-100 condition and integer rounding, corrected and added
limitations, Fix/Improvement changelog split, rebase on ff1ed695. Second review: the line itself as the red signal for
refused gaps, the jsdom indent stub, the hidden-children case moved to `TreePanel`, the rounding rule, the
raw/liquid lifting limit, wording of the limitations and the changelog.

## Verification

From `console/`: `npx vitest run src/components/email_builder/utils src/components/email_builder/panels/TreePanel.test.tsx
src/components/email_builder/EmailBuilder.test.tsx` while developing, then once: `npx tsc -b`, `npx vitest run`,
`npx eslint .`, `npm run build`, `npm run lingui:extract` (expect no change), and
`npx playwright test e2e/features/editor-drag-and-drop.spec.ts e2e/features/templates.spec.ts` against the dev
server. No Go file changes: `make test-unit` once as a sanity check.
