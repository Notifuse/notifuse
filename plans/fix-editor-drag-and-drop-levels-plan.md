# Pick the drop level in the structure tree from the pointer

Branch `fix/editor-drag-and-drop-levels`, from `fix/editor-drag-and-drop` (PR #510, 70cc34dc). It is rebased onto
`dev` once #510 merges, and the PR goes to `dev`. Console only: no table, no migration, no environment variable,
no new user-facing string.

Follows `plans/fix-editor-drag-and-drop-plan.md`. Pierre asked to handle its remaining limits before the
saved-blocks library work.

## Problem

#510 fixed the audited bugs and kept rc-tree's drop model: rc-tree proposes a node and a side from the pointer, and
`resolveDropTarget` tries candidates from the deepest level up and takes the first valid one. The plan of #510 lists
the limits that come from this model:

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
6. **Expanded state desync**: after a Preview and Edit round trip, rc-tree can show a container open that
   `expandedKeys` says is closed. A lifted drop there gets no line.

Out of scope:
- Group widths, which go to their own thread (gomjml and the editor's `width: 100%` default).
- The AI assistant's tools.

## Model

The tree draws its own line and computes the place itself, from what the user sees: the rows on screen and the
pointer. Dragging stays rc-tree's: draggable rows, the dragging highlight, auto-expand on hover, drag end.

### Gap

The pointer is over a row R, or above the first row, or below the last one. Rows are the visible
`.ant-tree-treenode` elements of the body tree, in order.

- In the top half of R, the gap is between the row above R and R.
- In the bottom half of R, the gap is between R and the row below it.

A gap is `(upper, lower)`, two block ids, and either may be null. A gap with no upper row (above the Body row) has
no place.

### Places in a gap

The function is pure, in `utils/treeDrop.ts`:

```ts
export interface DropGap { upperId: string | null; lowerId: string | null }
export interface DropPlace { parentId: string; slot: number; depth: number }
export function placesInGap(tree: EmailBlock, gap: DropGap): DropPlace[]
```

`depth` counts levels from the Body, which is 0; a section in the body is 1. `slot` is the insertion index in the
parent's current children. With U the upper block and L the lower one, the places run from deepest to shallowest:

1. **Inside U, when U is a container.**
   - If L is a child of U, U is shown open and the gap is before its first child: `(U, 0)`.
   - Otherwise U is collapsed or empty: `(U, U.children.length)`.

   The depth is `depth(U) + 1`.
2. **After U and each ancestor whose shown content ends at U.** For A = U, then its parent, and so on, while A is
   not an ancestor of L and A has a parent P: `(P, index of A in P + 1)`, at `depth(A)`.

Because U and L come from the screen, a collapsed container, a hidden child, or a desync between `expandedKeys` and
the screen all give the gap the user sees (limits 5 and 6).

### Choosing the place

```ts
export function resolveDrop(
  tree: EmailBlock,
  dragId: string,
  gap: DropGap,
  pointerDepth: number
): (DropTarget & { depth: number }) | null
```

1. Keep the places whose parent accepts the dragged block (`canAcceptChild`) and is not inside it.
2. Take the one whose depth is closest to `pointerDepth`; on a tie, take the deeper one.
3. Settle the slot into a final index, as `settle` does in #510: subtract 1 when the block is before the slot in the
   same parent.
4. Return null when the chosen place is the block's current place, or when no place is kept.

`pointerDepth = depth(drag) + round((pointerX − dragStartX) / indent)`. Moving the pointer straight down keeps the
dragged block's level; each indent to the left or right moves one level out or in. `indent` is the width of
`.ant-tree-indent-unit`, or 24 when it measures 0 (jsdom).

Effect on the limits:
- **Section just after a wrapper**, gap `(TW, S2)`, pointer straight down (depth 1). The places are after TW in CW,
  after CW in SW, after SW in W (depth 2) and after W in the body (depth 1). Only the last two accept a section, and
  depth 1 wins: the section lands after the wrapper. One indent to the right puts it in the wrapper (limit 1).
- **Section below an empty wrapper**, gap `(EW, S6)`. The places are inside EW (depth 2) and after EW (depth 1):
  after it by default, inside with one indent to the right (limit 2).
- **Text below an empty column**, gap `(EC, C5)`. Inside EC (depth 3) is the only place that accepts a text, so the
  natural gesture still fills it. The same holds for a column below an empty section and a social element below an
  empty social block.
- **Raw HTML between sections**, gap `(T3 last in S3, R)`, pointer at depth 1. The places are after T3 (3), after C3
  in S3 (2) and after S3 in the body (1). All three accept raw HTML, and the body wins.

### Line

The tree draws its own line, a `div` with `data-testid="tree-drop-line"` that is `aria-hidden`, absolutely
positioned in the body tree's wrapper:
- top: the upper row's bottom, in wrapper coordinates (scroll included);
- left: the content left of the upper row, plus `(depth − depth(U)) × indent`;
- right: 0.

antd's indicator never shows: `allowDrop` returns whether the tree's own place exists, but rc-tree renders its
indicator only where it computes one, so the wrapper hides `.ant-tree-drop-indicator` with a Tailwind arbitrary
variant. The line is recomputed on every `dragover`, so a refused position removes it (limit 4), and it is drawn at
the chosen depth (limit 3).

### Drop

The wrapper handles the native `drop` in the capture phase:
- with a place, it calls `preventDefault`, then `onMoveBlock`, and opens the parent when it is collapsed or empty;
- it clears the line, as do `dragleave` out of the wrapper and rc-tree's `onDragEnd`.

rc-tree's `onDrop` is no longer passed, so its own refusals no longer cancel a valid drop. One example is a column
dragged left over its own last text to leave its group.

`allowDrop` returns `currentPlace !== null`, read from a ref that the capture-phase `dragover` and `dragenter`
handlers update before rc-tree's node handlers run. This keeps rc-tree's auto-expand on hover, which only runs
where `allowDrop` is true.

## Changes

### `console/src/components/email_builder/utils/treeDrop.ts`
- Replace `resolveDropTarget` and `DropSide` with `placesInGap`, `resolveDrop` and `depthOf`. Keep `locate`,
  `contains` and `settle`.

### `console/src/components/email_builder/panels/TreePanel.tsx`
- `TreeNodeTitle` root span: add `data-block-id={block.id}`, next to `data-draggable`.
- Remove `handleDrop` and the resolver-based `allowDrop`.
- `onDragStart` (rc-tree prop): record the drag id, the start x and the dragged block's depth in a ref.
- Wrapper (`relative`):
  - `onDragOverCapture` and `onDragEnterCapture` compute the gap from the row rects, then the pointer depth, then
    `resolveDrop`; they store the result in the ref and the line position in state, and call `preventDefault` when
    there is a place;
  - `onDropCapture` performs the move;
  - `onDragLeave` (target leaving the wrapper) clears the line.
- `onDragEnd` (rc-tree prop) clears.
- Render the line; hide `.ant-tree-drop-indicator` under the wrapper.
- Keep `handleDragStartCapture` (Body row).

### `CHANGELOG.md` (43.2, unreleased)
#510's entries are unreleased, so this folds into its Improvement bullet rather than adding a Fix:
"…; while dragging, moving the pointer left or right picks the level where the block goes, and the line shows that
level."

## Tests (written first, red on #510's head)

### `utils/treeDrop.test.ts` (rewritten for the new API, same audit and empty-container fixtures)

**placesInGap**
1. `(T3, T4)` gives only `(C3, 3, depth 3)`.
2. `(T4 last in S3, R)` gives `(C3, 4, 3)`, `(S3, 1, 2)` and `(B, 4, 1)`.
3. `(S3 open, C3)` gives only `(S3, 0, 2)`.
4. `(C3 collapsed, R)`, with lower R, gives `(C3, 4, 3)`, `(S3, 1, 2)` and `(B, 4, 1)`.
5. `(EW, S6)` gives `(EW, 0, 2)` and `(B, i + 1, 1)`.
6. A null upper gives no place; a null lower (after the last row L) gives `(B, len, 1)`.

**resolveDrop**
1. Moves down in the same parent land where the line is: D in `(T3, T4)` at depth 3 gives `{C3, 2}`.
2. A section in `(TW, S2)`: at depth 1, after W; at depth 2, `{W, 1}`.
3. A section in `(EW, S6)`: at depth 1, after EW; at depth 2, `{EW, 0}`.
4. A text in `(EC, C5)` at depth 3 gives `{EC, 0}`, and also at depth 1 (nearest accepting place).
5. Raw HTML in `(T4, R)`: at depth 1, `{B, 4}`; at depth 3, `{C3, 4}`.
6. A tie between depths 3 and 1 at pointer depth 2 takes depth 3.
7. Refused: a text in `(SE1, SE2)`; a text after the last row L; a section in `(T3, T4)` — all null.
8. No-op: SP in `(D, SP)` gives null; S3 in `(T4, R)` at depth 1 gives null (its own place).
9. Never into itself: S1 in a gap inside its own subtree gives null. On the malformed tree, S1 in `(WX, …)` gives
   null.
10. G2 (a column of group G in S2) dragged over its own last text TG2, gap `(TG2, S3)`, at pointer depth 2 (one
    indent left): after G in S2, `{S2, 1}`; at depth 3: after G2 in G is its own place, null.

### `panels/TreePanel.test.tsx` (real TreePanel, jsdom, `box()` rect stubs and a stubbed indent)
1. Dragging D over the bottom half of T3 shows the line at T3's bottom with D's indentation; the drop calls
   `onMoveBlock('D', 'C3', 2)`.
2. A section over the bottom half of TW, straight down: the line is at body indentation and the drop moves it after
   W. The same with the pointer one indent to the right: wrapper indentation, `{W, 1}`.
3. Moving from an accepted half to a refused half of the same row removes the line (limit 4).
4. A collapsed C3 receives a text and is opened; an empty container receives a block and is opened.
5. The last visible child hides a raw block; the gap after it is computed from the rows, not the data (limit 5).
6. Rows rendered open while `expandedKeys` says closed: the lifted drop still has a line (limit 6).
7. antd's `.ant-tree-drop-indicator` never shows; `allowDrop` follows the tree's own place.
8. Kept from #510: Body dragstart cancelled, `data-draggable`, `aria-label`.

### `EmailBuilder.test.tsx`
No change: `onMoveBlock` keeps its contract.

### `console/e2e/features/editor-drag-and-drop.spec.ts` (Playwright, API mocked, run locally)
- `fixtures/tree-drag.ts`: the pointer goes straight down from the grab point, plus `dx`, instead of to the target's
  label. `lineShown` reads `tree-drop-line`, and a `lineLeft` helper is added.
- Kept cases: unchanged expectations, except that the wrapper and group empty-container cases need `dx = +24`.
- New cases:
  - a section just after a wrapper: dx 0 goes after the wrapper, dx +24 inside it;
  - a section below an empty wrapper: dx 0 goes after it, dx +24 inside it;
  - raw HTML dropped between sections at body level;
  - the line's left matches the target level's row content left, within 2 px;
  - the stale line on the same row is gone.

## Known, not handled
- Native autoscroll during a drag in a long tree is not covered by the e2e.
- The Body row keeps antd's grab cursor.
- Keyboard moves: there are none today.

## Verification

From `console/`:
1. While developing: `npx vitest run src/components/email_builder/utils/treeDrop.test.ts
   src/components/email_builder/panels/TreePanel.test.tsx`.
2. Once: `npx tsc -b`, `npx vitest run`, `npx eslint .`, `npm run build`, and `npm run lingui:extract` (expect line
   references only).
3. Against the dev server: `npx playwright test e2e/features/editor-drag-and-drop.spec.ts
   e2e/features/templates.spec.ts`.
4. No Go change: `make test-unit` once.
