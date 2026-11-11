# Pick the drop level in the structure tree from the pointer

Branch `fix/editor-drag-and-drop-levels`, from `fix/editor-drag-and-drop` (PR #510, 70cc34dc). It takes `dev` in once
#510 merges, and the PR goes to `dev`. Console only: no table, no migration, no environment variable, no new
user-facing string.

Follows `plans/fix-editor-drag-and-drop-plan.md`. Pierre asked to handle its remaining limits before the
saved-blocks library work.

Revision 2, after the first plan review (`reviews/fix-editor-drag-and-drop-levels-fable-plan-1.md` in the project
files). Changes against revision 1: the gap comes from row middles, the line is cleared on a capture-phase leave,
the tree opens collapsed rows with its own timer and gives rc-tree `allowDrop={() => false}`, an empty container
takes the block unless the pointer moved left, the places stop at the Body, and the test values are corrected.

## Problem

#510 fixed the audited bugs and kept rc-tree's drop model: rc-tree proposes a node and a side from the pointer, and
`resolveDropTarget` tries candidates from the deepest level up and takes the first valid one. The limits that come
from this model (1 to 4 are listed in #510's plan, 5 and 6 were found by its reviews):

1. **The deepest valid level wins.** With no sideways move:
   - a section dropped just after a wrapper lands at the end of the wrapper (`W[SW, S]`), not after it;
   - body-level raw HTML or Liquid dropped between two sections lands at the end of the column above;
   - a column dropped just after a group lands in the group.

   The sideways lift only reaches the outer level after a move of 12, 36 or 60 px to the left, counted from the
   grab point, and only through last children.
2. **The slot right after an empty container cannot be reached** by a block that both the container and its
   parent accept. A section dropped below an empty wrapper always goes into it.
3. **The line is drawn at rc-tree's level, not at the resolver's.** antd hard-codes `dropIndicatorRender`, so when
   the resolver lifts or lowers the level, the line is at the right height but indented several levels off.
4. **rc-tree keeps the previous line** when the pointer moves to a refused half of the same row. The drop then goes
   where that stale line shows.
5. **Hidden last child**: the walk down last children reads the data tree, while rc-tree shows the filtered tree.
   This is latent, since only `mj-title` and `mj-preview` are hidden today.
6. **Expanded state desync**: rc-tree opens the parents of expanded keys on mount (`Tree.js:166`), and the panel is
   unmounted in Preview (`EmailBuilder.tsx:832`). After a Preview and Edit round trip, rc-tree can show a container
   open that `expandedKeys` says is closed. A lifted drop there gets no line.

Out of scope:
- Group widths, which go to their own thread (gomjml and the editor's `width: 100%` default).
- The AI assistant's tools.
- Dragging from the saved-blocks library (lot 1). The API below takes the dragged block's type, with the id
  optional, so that lot does not rework it.

## Model

The tree draws its own line and computes the place itself, from what the user sees: the rows on screen and the
pointer. rc-tree keeps the drag source: draggable rows, the dragging highlight, `onDragStart`, `onDragEnd`. It no
longer decides any drop.

### Gap

Rows are the visible body-tree rows, `[role="treeitem"]` under the wrapper, in document order. (`.ant-tree-treenode`
also matches rc-tree's hidden measurer, which has no block id.) Each row carries its block id in
`data-tree-block-id`, on the title span (not `data-block-id`, which marks the block on the canvas).

- **upper** is the last row whose vertical middle is above the pointer or level with it (the same tie as rc-tree).
  When there is none (the pointer is above the Body row's middle), upper is the Body row.
- **lower** is the row after upper, or null.

This one rule covers the 4 px band antd leaves under each row (`antd/es/tree/style/index.js:123-134`), the space
above the first row, below the last one and the wrapper's padding. The Body row is always the first row, so the top
half of the Body row gives the gap `(B, first child row)` and the place `{B, 0}`, as on #510.

### Places in a gap

Pure, in `utils/treeDrop.ts`:

```ts
export interface DropGap { upperId: string; lowerId: string | null }
export interface DropPlace { parentId: string; slot: number; depth: number; emptyInside: boolean }
export function placesInGap(tree: EmailBlock, gap: DropGap): DropPlace[]
export function depthOf(tree: EmailBlock, id: string): number | null
```

`depth` counts levels from the Body, which is 0; a section in the body is 1. `slot` is the insertion index in the
parent's current children. With U the upper block and L the lower one, the places run from deepest to shallowest:

1. **Inside U, when U is a container.** The depth is `depth(U) + 1`.
   - If L is a child of U, U is shown open and the gap is before L: `(U, index of L in U)`. With a hidden first
     child this is after it, which is where the line is.
   - Otherwise U is collapsed or empty: `(U, U.children.length)`. `emptyInside` is true when U has no children.
2. **After U and each ancestor whose shown content ends at U.** For A = U, then its parent, and so on, while A is
   not the Body and A is not an ancestor of L: `(parent of A, index of A in it + 1)`, at `depth(A)`.

Unknown ids give no place. Because U and L come from the screen, a collapsed container, a hidden child, or a
desync between `expandedKeys` and the screen all give the gap the user sees (limits 5 and 6).

### Choosing the place

```ts
export interface DragSource { id?: string; type: MJMLComponentType; depth: number }
export function resolveDrop(
  tree: EmailBlock,
  drag: DragSource,
  gap: DropGap,
  shift: number
): (DropTarget & { depth: number }) | null
```

`shift = Math.round((pointerX − dragStartX) / indent)`: whole indents moved right (positive) or left (negative)
since the grab. `indent` is the width of the first `.ant-tree-indent-unit` under the wrapper, or 24 when it measures
0 (jsdom). The pointer depth is `drag.depth + shift`.

1. Keep the places whose parent accepts `drag.type` (`canAcceptChild`) and, when `drag.id` is set, is neither the
   dragged block nor inside it.
2. **Empty container.** If a kept place has `emptyInside` and `shift >= 0`, take it. Dropping on an empty
   container fills it, as on #510; one indent to the left puts the block after it instead.
3. Otherwise take the kept place whose depth is closest to the pointer depth; on a tie, the deeper one.
4. Settle the slot into a final index, as `settle` does in #510: subtract 1 when the block is before the slot in the
   same parent.
5. Return null when no place is kept, or when the chosen place is the block's current place.

Why rule 2: from the drag handle there is about 24 px of tree to the right of the pointer (handle centre x 279,
tree right edge 303 in Chromium). A rule that needed one indent to the right to enter an empty wrapper or group
would leave a window of 12 px for users who grab the handle. Room to the left always exists.

Effect on the limits:
- **Section just after a wrapper**, gap `(TW, S2)`, straight down (pointer depth 1). The places are after TW in CW,
  after CW in SW, after SW in W (depth 2) and after W in the body (depth 1). Only the last two accept a section, and
  depth 1 wins: the section lands after the wrapper. One indent to the right puts it at the end of the wrapper
  (limit 1).
- **Section below an empty wrapper**, gap `(EW, S6)`. The places are inside EW (depth 2, empty) and after EW
  (depth 1): inside by default, after it with one indent to the left (limit 2).
- **Text below an empty column**, gap `(EC, C5)`. Inside EC is the only place that accepts a text.
- **Raw HTML between sections**, gap `(T4 last in S3, R)`, pointer depth 1. The places are after T4 (3), after C3
  in S3 (2) and after S3 in the body (1). All three accept raw HTML, and the body wins.

### Behaviour changes for a straight drag (no sideways move)

Measured by the review on the audit fixture and the fixture with empty containers. All follow the line.

| Gesture | #510 | Now |
|---|---|---|
| Section over the last row of a wrapper | `{W, 1}` | after W |
| Column over the last row of a group | `{G, 2}` | after G, `{S2, 1}` |
| Body-level raw or Liquid over the last row of a last column | end of that column | body level, after the section |
| Body-level raw or Liquid over IM1, last row of the first column C1 | `{C1, 3}` | `{S1, 1}`, between the columns |
| S2 over the bottom of TW, the row just above it | `{W, 1}` | nothing: its own place |
| Raw R over the bottom of collapsed C3 | `{S3, 1}` | nothing: its own place |
| Liquid L over the bottom of collapsed C3 | `{S3, 1}` | `{B, 4}`, before R |

Unchanged: empty containers take the block (rule 2), the top half of the Body row gives `{B, 0}`, a collapsed
non-empty container appends (`(U, len)`, the existing e2e case `C3[D,SP,T3,T4,T1]`).

### Line

A `div` with `data-testid="tree-drop-line"`, `aria-hidden`, `pointer-events: none`, absolutely positioned in the
body tree's wrapper (`relative`):
- top: the upper row's bottom, in wrapper coordinates (the rect difference handles the panel's scroll), plus the
  wrapper's `scrollTop`;
- left: the upper row's content left (its `.ant-tree-node-content-wrapper`), in wrapper coordinates, plus the
  wrapper's `scrollLeft`, plus `(depth − depth(U)) × indent`;
- right: 0.

The line state is `{ top, left } | null` and is set only when it changes: every render rebuilds `treeData`, and a
new data identity drops a running expand animation in rc-tree (`NodeList.js:143-147`).

rc-tree renders no indicator at all, since `allowDrop` is always false, so no CSS hiding is needed. The line is
recomputed on every `dragover`, so a refused position removes it (limit 4), and it is drawn at the chosen depth
(limit 3).

### Opening collapsed rows on hover

rc-tree's own timer is armed only on `dragenter` and only where `allowDrop` is true, and is not cancelled when the
pointer moves to a row with no place. With `allowDrop={() => false}` it never arms, and the wrapper runs its own:

- On each `dragover`, the hovered row is the row of the event's target, or none (in the 4 px band under a row,
  that row).
- If it changed, the pending timer is cleared. If the new row is collapsed (not in `expandedKeys`, has children), is
  not the dragged block or inside it, and it or one of its descendants accepts the dragged type, a timer of 800 ms
  calls `onTreeExpand([...expandedKeys, id])`.
- The timer is cleared on leave, drop, `dragend` and unmount.

So a section held over collapsed S3 no longer opens it, and a text coming from above opens a collapsed column.

### Events

The wrapper handles, in the capture phase (it runs before rc-tree's row handlers, which stop propagation):

- `onDragEnterCapture`, `onDragOverCapture`: when a drag from this tree is in progress (the drag ref is set),
  compute the gap, the shift, `resolveDrop`, the line and the hover row; call
  `preventDefault` when there is a place, so the native `drop` fires on the padding too.
- `onDropCapture`: the place is computed again from the drop's coordinates (a row may have opened under the pointer
  since the last `dragover`). With a place: `preventDefault`, `onMoveBlock(id, parentId, index)`, and
  `onTreeExpand` for the parent when it is collapsed or empty. Then clear the line, the timer and the drag ref.
- `onDragLeaveCapture`: clear the line and the timer when the pointer leaves the wrapper: `relatedTarget`
  is outside it, or, when `relatedTarget` is null, the pointer is outside the wrapper's rect.

rc-tree props:
- `onDragStart` records the drag ref: id, type, depth, start x.
- `onDragEnd` clears everything.
- `onDrop` is no longer passed, so rc-tree's own refusals (its descendant check) no longer cancel a valid drop. One
  example is a column dragged left over its own last text to leave its group.
- `allowDrop={() => false}`.

A drag that did not start in this tree (no drag ref) is ignored: no line, no `preventDefault`, no move.

## Changes

### `console/src/components/email_builder/utils/treeDrop.ts`
- Replace `resolveDropTarget` and `DropSide` with `placesInGap`, `resolveDrop` and `depthOf`, and the types
  `DropGap`, `DropPlace`, `DragSource`. Keep `locate`, `contains`, `settle`.

### `console/src/components/email_builder/panels/TreePanel.tsx`
- `TreeNodeTitle` root span: add `data-tree-block-id={block.id}`, next to `data-draggable`.
- Remove `handleDrop` and the resolver-based `allowDrop`.
- Add the drag ref, the line state, the hover timer and the capture handlers above on the body tree's wrapper, which
  becomes `relative`. Render the line.
- Keep `handleDragStartCapture` (Body row).

### `CHANGELOG.md`
#510's entries are unreleased (43.2 WIP, shipping as 44.0), so this folds into its Improvement bullet rather than
adding a Fix. The existing sentence on empty containers stays true thanks to rule 2. Added at its end:
"…; while dragging, moving the pointer left or right picks the level where the block goes, and the line shows that
level." The entries move under 44.0 with #510's when the release section exists on `dev`.

## Tests (written first)

"Red on head" was checked by running the new tests on #510's head (70cc34dc) with only `data-block-id` added to the
rows, so the rows can be found:
- `treeDrop.test.ts` is red as a whole (new API). The values that differ from #510's resolver: resolveDrop 2 at
  shift 0, 3 at shift −1, 5 and 10.
- `TreePanel.test.tsx`: red on head are 1 to 5, the three hover cases of 7 other than "from below", 8, 9, 10, the
  null `relatedTarget` case and 12. The others (6, 7 from below, 11, 13) pin behaviour #510 already has.

### `utils/treeDrop.test.ts` (rewritten for the new API, same audit and empty-container fixtures)

Places are asserted as exact arrays, so their number is pinned.

**depthOf**
1. B 0, S1 1, C1 2, T1 3, TW 4; an unknown id gives null.

**placesInGap**
1. `(T3, T4)` gives `[(C3, 3, 3)]`.
2. `(T4, R)` gives `[(C3, 4, 3), (S3, 1, 2), (B, 4, 1)]`.
3. `(S3 open, C3)` gives `[(S3, 0, 2)]`.
4. `(C3 collapsed, R)` gives `[(C3, 4, 3), (S3, 1, 2), (B, 4, 1)]`.
5. `(EW, S6)` gives `[(EW, 0, 2, empty), (B, index of EW + 1, 1)]`.
6. `(L, null)` gives `[(B, 6, 1)]`: nothing above the Body.
7. `(B, S1)` gives `[(B, 0, 1)]`.
8. A hidden first child RH of C3 (`C3[RH, D, SP, T4]`): `(C3, D)` gives `[(C3, 1, 3)]`, not slot 0, and `(D, SP)`
   gives `[(C3, 2, 3)]` (#510's hidden-first-child case, `('T4', 'C3', 2)`).
9. Unknown upper id: `[]`, no throw.

**resolveDrop**
1. D in `(T3, T4)`, shift 0: `{C3, 2}`.
2. A section in `(TW, S2)`: S1 at shift 0 gives `{B, 1}`, at shift +1 `{W, 1}`; S3 gives `{B, 2}` and `{W, 1}`.
3. A section in `(EW, S6)`: shift 0 gives `{EW, 0}`, shift −1 gives `{B, index of EW + 1}`.
4. A text in `(EC, C5)`: `{EC, 0}` at shifts 0 and −2.
5. Liquid L in `(T4, R)`: shift 0 gives `{B, 4}`; raw R in `(T4, R)` at shift +2 gives `{C3, 4}`, at shift 0 null
   (its own place).
6. Tie: R in `(TG2, S3)` at shift +2, pointer depth 3. The accepting places are at depths 4, 2 and 1 (the group
   refuses raw), so 4 and 2 tie and the deeper wins: `{G2, 1}`.
7. Refused, all null: a text in `(SE1, SE2)`; a text in `(L, null)`; a section in `(T3, T4)`.
8. No-op, null: SP in `(D, SP)`; S3 in `(T4, R)` at shift 0.
9. Never into itself: S1 in a gap inside its own subtree gives null; on the malformed tree, S1 in `(WX, …)` at
   pointer depth 3 and 4 gives null (without the check it would be `{WX, 0}`).
10. G2 over its own last text TG2, gap `(TG2, S3)`: shift −1 gives `{S2, 1}`; shift 0 gives null (its own place).
11. A drag with no id (library shape) and a type: R-type in `(T4, R)` at depth 1 gives `{B, 4}`.
12. Unknown drag id: null, no throw.

### `panels/TreePanel.test.tsx` (real TreePanel, jsdom, layout stubs for rows, content wrappers and the indent unit)

Rows are found by `data-tree-block-id`. Fake timers for the hover cases.

1. D over the bottom half of T3: the line is at T3's bottom with D's indentation; the drop calls
   `onMoveBlock('D', 'C3', 2)`.
2. A section over the bottom half of TW, straight down: body indentation, the drop moves it after W; one indent to
   the right: wrapper indentation, `{W, 1}`.
3. From an accepted half to a refused half of the same row, the second position sent as `dragover` only (as a
   browser does within one row; a `dragenter` resets rc-tree's state): the line is gone and the drop does nothing.
   Head drops at the stale place.
4. The pointer in the band between two rows (below T3's rect, above T4's): the line stays and the drop lands
   between them.
5. A drop on the wrapper's padding under the last row: the block goes at the end of the body.
6. A collapsed C3 receives a text and is opened; an empty container receives a block and is opened.
7. Hover open: a text held over collapsed C3 for 800 ms opens it, from above and from below; a section held over
   S3 does not, with S3 and C3 both left out of `expandedKeys` (with C3 expanded, rc-tree shows S3 open); leaving
   the row before 800 ms cancels it.
8. The last visible child hides a raw block; the gap after it is computed from the rows (limit 5).
9. Mounted with C3 in `expandedKeys` and not S3: the lifted drop still has a line (limit 6).
10. Leaving the wrapper from a row clears the line; crossing to another row does not. With a null `relatedTarget`,
    the pointer's position against the wrapper's rect decides.
11. A drag that did not start in the tree: no line, no move.
12. rc-tree renders no `.ant-tree-drop-indicator`.
13. Kept from #510: Body dragstart cancelled, `data-draggable`, `aria-label`.
14. Added after the code review: a pointer level with a row's middle counts that row as above; the top half of the
    Body row gives `{B, 0}`; a drop right after a row opened under a still pointer uses the rows at the drop.

### `EmailBuilder.test.tsx`
No change: `onMoveBlock` keeps its contract.

### `console/e2e/features/editor-drag-and-drop.spec.ts` (Playwright, API mocked, run locally)
- `fixtures/tree-drag.ts`: the pointer goes straight down from the grab point, plus `dx`, instead of to the target's
  label. `ROW` becomes `[role="treeitem"]`. `lineShown` reads `tree-drop-line`; a `lineLeft` helper is added; a
  `grab` option picks the label or the drag handle. Offsets stay off ±12.
- Kept cases: unchanged expectations. Some gestures change:
  - the drops on the middle of C2 and S3 (including the column-proportions case) move from 0.5 to 0.7 of the row,
    off the exact middle;
  - the sideways offsets of "a section dropped on the top half of the next section" become 0, −24, +24, +48, so
    the pointer stays over the tree now that it starts from the section's label;
  - "the wrapper moves by its handle" now grabs the handle.
  Each case makes one gesture: after a save the test does not drag again.
- New cases:
  - a section just after a wrapper: dx 0 goes after the wrapper, dx +24 inside it;
  - a section on an empty wrapper one indent left goes after it (dx 0 fills it: the kept empty-container case);
  - Liquid dropped below the last text of a section stays at body level, before R (raw R there would be its own
    place);
  - the line's left matches the target level's row content left, within 2 px;
  - the stale line on the same row is gone;
  - a drop in the band between two rows (y = row bottom + 2);
  - grabbed by the handle: S1 into the empty wrapper and C1 into the empty group;
  - leaving the tree to the right clears the line.

## Known, not handled
- From the drag handle the tree ends about 24 px to the right of the pointer (measured in Chromium):
  - a move that needs one indent to the right (a section to the end of a wrapper, a column to the end of a group,
    Liquid to the end of a section) works only for dx 13 to 23;
  - raw HTML or Liquid cannot reach places two levels in: the end of a non-empty column, the end of a section inside
    a wrapper.

  Empty containers are not affected: rule 2 fills them at dx 0. From the label every place is reachable.
- Grabbing S6 (just below the empty wrapper EW) or C6 (just below the empty group EG) on the top half of its own row
  and releasing it nearby moves it into the empty container, by rule 2. #510 does the same.
- Native autoscroll during a drag in a long tree is not covered by the e2e.
- The Body row keeps antd's grab cursor.
- Keyboard moves: there are none today.
- Checked in Chromium only.

## Verification

From `console/`:
1. While developing: `npx vitest run src/components/email_builder/utils/treeDrop.test.ts
   src/components/email_builder/panels/TreePanel.test.tsx`.
2. Once: `npx tsc -b`, `npx vitest run`, `npx eslint .`, `npm run build`, and `npm run lingui:extract` (expect line
   references only).
3. Against the dev server: `npx playwright test e2e/features/editor-drag-and-drop.spec.ts
   e2e/features/templates.spec.ts`.
4. No Go change: `make test-unit` once.
