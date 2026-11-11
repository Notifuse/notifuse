# Plan — notifuse: take the gomjml group-width fix, and make the editor match it

Repository: `Notifuse/notifuse`, branch `fix/editor-group-widths` from `dev`, PR to `dev`.
Context: Notifuse/gomjml#1 (merged, released in gomjml v0.14.0/v0.14.1).
Coordination: the drag-and-drop thread (PR #510 and `fix/editor-drag-and-drop-levels`) does not
touch `mjml-defaults.ts`, `MjSectionBlock.tsx` or `MjGroupBlock.tsx`. Its `utils/columnWidths.ts`
rescales percentage widths of the `mj-column` children of a section or a group and never
changes a group's own width; this plan keeps that rule (it does not write group widths).

## 1. gomjml bump: done elsewhere
Notifuse/notifuse#513 imports `github.com/Notifuse/gomjml v0.14.1` (module renamed, no more
`replace`; v0.14.1 contains the group fix) and adds its CHANGELOG lines under `[44.0] - WIP`.
This PR is console-only and does not depend on #513 to build; its CHANGELOG line goes in the
same `[44.0]` section (whichever merges second resolves the conflict).

## 2. Editor default: no width on new groups
- `console/src/components/email_builder/mjml-defaults.ts`: remove `width: '100%'` from
  `MJ_GROUP_DEFAULTS`, so `createBlock('mj-group')` no longer stores it (MJML has no group
  width default).
- `MjGroupBlock.tsx` settings panel: the Width input placeholder no longer claims `100%`
  (`blockDefaults.width || '100%'`); show the computed share instead (e.g. `50%` when the
  group has one sibling), computed by the helper in §3.
- Starter template `EmailBlockClass.tsx` ~1189 (group alone in its section with `width: '100%'`):
  leave as is (alone, 100% equals its share; output unchanged).
- Tests (vitest): `createBlock('mj-group')` has no `width`; the panel placeholder shows the
  share for 1, 2 and 3 layout siblings.

## 3. Canvas: group width as in the email
Today (`MjSectionBlock.tsx` ~343-371, `MjGroupBlock.tsx` ~288-292):
- the section's default share counts `mj-column` children only, and a width-less group's cell
  gets no width;
- an explicit group width is applied twice: on the section cell and again on the group's own
  flex div (60% of 60% = 36%).
Changes:
- New pure helper (e.g. `utils/sectionChildWidth.ts`): for a section child, its canvas width =
  explicit width if set, else `100 / (number of mj-column + mj-group children)` %.
  Used by the section cell for columns and groups alike, and by the §2 placeholder.
- `MjGroupBlock` flex div: `width: '100%'` (it fills the section cell, which carries the width).
- Tests: helper unit tests (columns only, group + column, group + 2 columns, raw/liquid
  siblings not counted, explicit % and px); render test of a section with a width-less group
  and a column: both cells 50%; with a `60%` group: group cell 60% and group div 100%.

## 4. Existing templates (Pierre chose: leave stored data, CHANGELOG note)
Groups saved with `width="100%"` keep it. Alone in a section: no change. Next to a column:
on the web nothing changes (100% group, column below); in Outlook it becomes one row of
600 + 300px, as MJML renders it (before: two stacked tables). Recommended: leave stored data,
mention it in the CHANGELOG. Alternatives offered: drop `width="100%"` on groups at render
time, or when the template is opened in the editor.

## 5. CHANGELOG (`[44.0]`)
One short **Improvement** line: the editor no longer gives new groups a 100% width, and its
canvas shows a group's width as the email does. Per §4, one sentence for groups saved at 100%
next to a column, if #513's entry does not already say it.

## Out of scope
- gomjml remaining differences listed in `plans/fix-gomjml-group-widths-plan.md`.

## Run
```
cd console && npm test -- email_builder && npx tsc --noEmit && npm run lint
cd console && npm run lingui:extract   # only if a new string is added
```

## Spec review — integrated
- Current behaviours confirmed (mjml-defaults.ts:209-215, EmailBlockClass.tsx:433-456,
  MjSectionBlock.tsx:343-361, MjGroupBlock.tsx:50 and :292).
- **Blocking, needs Pierre's choice**: the editor keeps section column widths summing to 100%
  while ignoring groups (EmailBuilder.tsx:421, :507, :602, :696; EmailBlockClass.tsx:1404-1436;
  `columnWidths.ts` on the drag-and-drop branches counts only mj-column). Adding a group to a
  section never rebalances its columns, so the usual editor result is "group (no width) +
  column 100%": with gomjml v0.14.1 the web wraps and Outlook gets a 900px row. Options:
  (a) count groups in the column rebalancing (columns get 100 − group shares; group add, move,
  delete rebalance), built on `columnWidths.ts` once #510/#514 are in dev; (b) scope it out.
- mj-liquid: the converter writes it verbatim and Liquid runs before gomjml, so gomjml counts
  the columns/groups it produces; the editor cannot know them (loops). Not counting mj-liquid
  stays an editor approximation, listed out of scope; mj-raw not counted is exact.
- Group width from mj-attributes (`<mj-group width="100%"/>`, created by the editor's Default
  attributes) is honoured by gomjml: the canvas helper resolves block width, then
  `attributeDefaults['mj-group'].width`, then the share; the placeholder keeps
  `blockDefaults.width` first. In such templates new groups still get 100% (createBlock copies
  mj-attributes defaults): expected.
- Stored templates, corrected: a group at 100% next to a column at 100% (editor default) gives
  a 1200px Outlook row; two groups at 100% (each editor-added group; `newsletter.json`
  section g87pm7yv5) also 1200px; saved blocks carry groups at 100%. Web unchanged. The
  CHANGELOG note says "a group saved at 100% next to another column or group" and tells users
  to clear the group's Width.
- Commands: `npx tsc -b` (not `--noEmit`, tsconfig has `files: []`), `npx vitest run
  src/components/email_builder` (not `npm test`, which watches).
- Test files: `utils/sectionChildWidth.test.ts`, `blocks/MjSectionBlock.test.tsx`,
  `blocks/MjGroupBlock.test.tsx`, `mjml-defaults` / createBlock test (`not.toHaveProperty('width')`).
  Cases added: width from mj-attributes, group + column 100% (per the choice above), group
  selected inside mj-attributes (no share placeholder), 3 siblings rounded to 33.33%,
  unparseable width falls back to the share.
- Panel: `renderSettingsPanel` gets the tree from SettingsPanel (third argument) and walks it
  to find the parent section.
- Merge after #513 (same 44.0 release), otherwise the canvas share differs from the email.
- Out of scope: columns inside a group look equal on the canvas (30/70 shows 50/50).
- AI system prompt (email-ai-system-prompt.ts:326) "column widths sum to 100% within a
  section" ignores groups: one line to adjust, in scope if (a) is chosen.

## Implementation notes
- Commit 6f395d2 (draft PR #516): §2 and §3 done, AI prompt line adjusted, CHANGELOG Improvement under
  [44.0]. Helper API: `sectionShare(section)`, `sectionChildWidth(child, section, attributeDefaults)`,
  `findParentSection(tree, id)`. A group outside a section (default attributes) keeps the WidthInput
  placeholder ("Enter width").
- After #510/#514 reached dev (merged in): `utils/columnWidths.ts` counts groups in a section. Groups
  take their % width, or 100/(columns+groups) when width-less or px; columns share the rest by ratio
  (equal split when their widths are not all % filling the previous budget). Groups leaving no room
  (e.g. a group saved at 100%) are not counted. Group widths are never written. Call sites in
  EmailBuilder.tsx: add, delete, clone, move across containers, saved block, for mj-group too.
  Tests: columnWidths.test.ts (+9), EmailBuilder.test.tsx (+2, new section SD). Full console suite
  3854 tests, tsc -b, eslint pass.
