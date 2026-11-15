# Automation editor on the Nuit claire mockup (screen 72) — plan

## Problem

Mockup 72 (`suite-maquettes/3-maquettes/identite-visuelle/automatisations-refonte/72-editeur-avec-statistiques.png`, source `console/src/identity/AutomationsRedesign.tsx` in `maquette-console-v2.patch`) shows the automation editor with stats on. The console editor on `dev` (`c77e084e`, checked live on Welcome Series of the demo workspace at 1440×900) already has every element the mockup draws in the step panel (template, preview, provider, 30-day results, "View messages", delete), but places and sizes them so that the screen reads differently:

| Area | Mockup 72 | Console on `dev` |
|---|---|---|
| Opening zoom | 100 %, trigger at the top, nodes readable | `fitView` with `maxZoom: 0.9`, `padding: 0.3`: 52 % on Welcome Series, nodes unreadable |
| Minimap | none | bottom-right, covers the "Win-back Offer" node |
| Branch outputs | "Oui · 63 %" / "Non · 37 %" labels clear of everything | the "+" of the free "Not in list" output of List status sits on the "Not in list" and "Active · 82 %" labels |
| Settings (left column) | compact "label: value ▾" rows under a "Settings" heading | a vertical form, each label above its field |
| Palette | flat rows, icon and name | bordered cards |
| Step panel title | "EMAIL · Bienvenue dans la newsletter" | "EMAIL · Email" when the step has no description |
| Email step panel order | template, compact preview, sending provider, 30-day results, fixed "Delete step" footer | description, template, integration, sender/subject, a ~420px preview, results below the fold, delete at the end of the scroll |

## Decisions (Pierre, 2026-11-15)

- D1. The editor opens at 100 % with the trigger centred at the top, for an existing automation as for a new one. The "Fit to screen" button stays.
- D2. The minimap is removed.
- D3. Left column: settings as compact rows and the palette as flat rows. Same fields, same tooltip, same disabled rules.
- D4. Email step panel: the Description field stays first (it exists for every step type and fields are never removed). After it, the mockup's order: template, compact preview, provider, results, then a fixed footer with "Delete step". The title shows the step's name.
- D5. The "+" that covers the branch labels is fixed in this lot.
- D6. No "⋯" menu on the nodes: the mockup draws it without actions, and deleting already has the trash button beside the selected node and the panel footer.
- D7. A deliberate departure from the mockup: the "Edit / With stats" and "Last 30 days / All time" Segmented controls leave the header. A "Stats" switch and, when it is on, a "7 d / 30 d / All time" period sit together at the top right of the canvas. The header keeps the actions on the automation only. "Edit" wrongly suggested the stats view could not be edited; showing the figures is a display setting, like the zoom.
- D8. The period goes into the URL (`period=7d|all`, 30 days without the parameter), as `stats=1` already does. Switching stats off clears it (back to 30 days).
- D9. (Replaced by D11.) Connected branch labels were first kept at the middle of their curve.
- D11. Outputs of a multi-output step (filter, A/B test, list status, branch): their names form the last row of the card, each centred above its output point (the in-card Yes/No row, list status row and A/B variant chips go). With stats on, each link carries only its share, placed on the link just under its own output ("G′"), not halfway, so a link crossing under a neighbour's output cannot be read as the neighbour's share; no label without stats. A free output's stub has no label, and its "+" steps outward beside its point. Chosen by Pierre on 2026-11-15 from mockups A–H and two console captures (G at the middle of the link was misread).
- D10. The top of the canvas is one row: Reorganize on the left, the banner in the middle (it shrinks), Stats and the period on the right.

## Intended gaps (not done, on purpose)

- The node "⋯" menu (D6).
- The header keeps its existing controls the mockup does not draw: Activate, the "⋯" menu and the copy-link button. They are features, not decoration. The mockup's "Édition / Avec statistiques" control is replaced by D7.
- The Description field in the step panel (D4); the mockup has none.
- The help lines under the template and integration fields stay, in an info icon beside each label (decided after the first pass: the panel kept its results out of view).
- Sidebar and top bar: merged into the sidebar on purpose, not counted.
- Mockup content (Acme, French template names, 1 284 entered) is fake data; the demo shows its own.

## Changes

### 1. Open at 100 %, trigger at the top (D1)

`console/src/components/automations/AutomationFlowEditor.tsx`, new `console/src/components/automations/utils/anchorViewport.ts`

- Drop `fitView={isEditing}` / `fitViewOptions` from `<ReactFlow>`.
- A pure helper `anchorOnTrigger(trigger, wrapperWidth, zoom)` returns `{ x: wrapperWidth / 2 - (trigger.x + width / 2) * zoom, y: TOP_GAP - trigger.y * zoom, zoom }`, `width` from `trigger.measured.width`.
- One effect replaces the new-automation placement (`!isEditing && nodes.length === 1`, :365-376) for both new and existing automations. It runs once `useNodesInitialized()` is true (so `measured.width` is real, not the 300 fallback) and the wrapper has a size, calls `setViewport(anchorOnTrigger(trigger, width, 1))`, and sets the once-only guard only when `setViewport` resolves `true`: it resolves `false` and does nothing while ReactFlow's pan-zoom is not ready (`@xyflow/react` 12.10.1), and a guard set before that would leave the view at `defaultViewport` for good.
- Auto-reorganize (after adding a filter, an A/B test or a list status, :196-209) no longer calls `fitView`, which zoomed back out to ~50 % after every branch added. `layoutNodes` moves every step back to its own grid (`utils/layoutNodes.ts`), so instead of keeping the old view, which could leave the flow off screen, it re-anchors on the trigger at the current zoom with the same helper. The manual "Reorganize layout" button and "Fit to screen" keep `fitView`, being explicit requests to see everything.

### 2. No minimap (D2)

Same file: remove `<MiniMap>` and its import.

### 3. Output names in the card, shares on the links (D5, D11)

`nodes/OutputNames.tsx` (new), `nodes/BaseNode.tsx` (an `outputs` slot after the stats footer), `nodes/FilterNode.tsx`, `nodes/ListStatusBranchNode.tsx`, `nodes/ABTestNode.tsx`, `nodes/BranchNode.tsx`, `edges/AutomationEdge.tsx`, `edges/AddNodeEdge.tsx`, `utils/freeOutputPlus.ts` (new), `AutomationFlowEditor.tsx`

- `OutputNames` renders each output's name (from `edgeBaseLabel`, in its tone's text colour) at its handle's `left`, as the card's last row.
- `AutomationEdge`, for a link out of a multi-output step: the label is the share alone (`branchShare`, journey count in the tooltip), placed `SHARE_DOWN` under the output and `SHARE_ALONG` of the way across; no label without stats or without a share. Other links unchanged.
- `AddNodeEdge` draws no label for such an output. `freeOutputPlus` moves its "+" just under the point and outward (left of the middle to the left, right to the right).

### 4. Settings as compact rows (D3)

`console/src/components/automations/EditorSettings.tsx`

- A "Settings" heading styled like the palette's group headings (`text-xs font-medium tracking-wide text-ink-3 uppercase`).
- Each setting on one row: the label on the left at a fixed width, the control on the right, through the antd `Form` in horizontal layout (`colon={false}`, fixed `labelCol` width), so the `Form.Item`s stay. List and Frequency use the antd `Select` with `variant="borderless"` (antd 6.6.0 has it; themed, no Tailwind on the component), so they read "Newsletter ▾". Exit on reply keeps its label, info tooltip and `Switch` on the same row as today.
- Unchanged: the `htmlFor` / `id` pairs, `allowClear` on the list, the two frequency options, the inbound-integration rule and the read-only rule on the switch.

### 5. Flat palette rows (D3)

`console/src/components/automations/NodePalette.tsx`

- Items lose their border and card background: `flex items-center gap-2 rounded-md px-2 py-1.5 text-sm text-ink`, `hover:bg-subtle` when enabled. Groups, drag data, the disabled Email item and its tooltip are unchanged.

### 6. Step panel title (D4)

`console/src/components/automations/NodeConfigPanel.tsx`

- Title: the step's description, else for an email step the template's name, else the type label (today: description, else type label, :186).
- The name comes from `useAutomation().templates`, as the email node on the canvas already reads it (`nodes/EmailNode.tsx:15-18`): the page loads every email template once (`pages/AutomationsPage.tsx:109-116`), so no new query or hook. Without Templates read the list is empty and the title falls back to the type label.

### 7. Email step panel order and footer (D4)

`NodeConfigPanel.tsx`, `config/EmailConfigForm.tsx`, `stats/EmailStepPreview.tsx`, `console/src/components/broadcasts/detail/TemplatePreviewFrame.tsx`

- `EmailConfigForm` takes an optional `preview` node, rendered between the template field and the integration field. `NodeConfigPanel` passes `EmailStepPreview` there instead of its own section under the form.
- Today the preview stays usable read-only because it sits outside the panel's `ConfigProvider componentDisabled={readOnly}` (`NodeConfigPanel.tsx:262-293`). Inside the form it is wrapped in `ConfigProvider componentDisabled={false}` (the inner value wins in antd 6.6), so Preview and Open template stay usable read-only.
- `TemplatePreviewFrame` takes an optional `frameHeight` (default `FRAME_HEIGHT`, 900). It sets both the iframe and the box, so a shorter value crops the email to its top part rather than shrinking it: a thumbnail of the header and first lines, as the mockup draws. The step panel passes a height that gives ~200px in the 320px panel (today ~429px). The broadcast screens (`BroadcastAside.tsx`, `PreviewPanel.tsx`) keep the default.
- The integration select's default option names the provider it falls back to: "Use workspace default · {name}" (the marketing provider's integration name), plain "Use workspace default" when none is set.
- The panel becomes three parts: header, scrolling body (description, form with the preview, results), and a fixed footer with "Delete step" when the step can be deleted. Every step type gets the footer, not only email.

### 8. Stats switch and period on the canvas (D7, D8)

`AutomationEditor.tsx`, `AutomationFlowEditor.tsx`, new `stats/StatsControl.tsx`, `last30Days.ts`, `stats/nodeStatsState.ts`, `useNodeStats.ts`, `NodeConfigPanel.tsx`, `pages/AutomationsPage.tsx`, `console/src/router.tsx`

- `StatsControl`: a "Stats" label with an antd `Switch` (`aria-label` "Show stats"), and when on a `Segmented` "7 d / 30 d / All time" (`aria-label` "Period"). Disabled with the existing tooltip (`statsDisabledReason`: "Save the automation first" / "No journeys yet") in the same cases as today's Segmented, the `Switch` wrapped in a `<span>` so the tooltip shows on a disabled control (as the Segmented is today, `AutomationEditor.tsx:480-481`).
- Placement: the canvas's top row becomes one `<Panel position="top-left">` spanning the canvas width: "Reorganize layout" on the left, the live / stats-error banner in the middle (shrinking, `min-w-0`, at most `max-w-xl`), `StatsControl` on the right. A separate top-right Panel would overlap the 576px banner, which a live automation always shows, in the ~900px canvas left at 1280–1440 by the rail and the step panel. `AutomationFlowEditor` takes the control as a `statsControl` prop, as it already takes `overlay`.
- The two Segmented controls leave `topActions` in the header.
- `StatsPeriod` becomes `'last7' | 'last30' | 'all'`. `last30Days.ts` (renamed `lastDays.ts` after the third review) gains `lastDays(timezone, n)` (the n days ending today, today included, in the workspace's timezone) and keeps `last30Days` as `lastDays(tz, 30)`, so its importers (`list/useAutomationOpenRates.ts`, its test, `useNodeStats.test.tsx`) are untouched. `useNodeStats` resolves 7 or 30 days, or no bounds for all time; the query key already carries the start (`useNodeStats.ts:24`).
- The step panel's results heading follows the period: "Last 7 days", "Last 30 days", "All time".
- URL: `AutomationsSearch` gains `period?: '7d' | 'all'`; `validateAutomationsSearch` drops any other value (30 days is the absence of the parameter). As for `stats`, the editor does not touch the router: `AutomationEditor` takes `period` and `onPeriodChange` props, replacing its `useState` (:226), and `AutomationsPage.tsx` reads and writes the search. `period` is cleared everywhere `stats` is today (:55, :156, :157, :294, :299): opening another automation, a new one, closing, and switching stats off.
- Switching stats off clears the period: with the period control hidden, a 7-day period kept in the URL would still drive the step panel's results (stats are also read when a step is selected, `AutomationEditor.tsx:243`) with no visible way to change it.

### 9. Results rows as the mockup draws them (Pierre, after the first pass)

`stats/EmailStepStats.tsx`: each row is its name, a bar across the room left, then its value in bold; Sent shows its count with a full bar, Delivered keeps "—" and its SMTP tooltip, a rate without a base reads "—" with an empty bar. `RateBar` (shared with the tables) is unchanged.

### 10. A selected step stays in view when its panel opens (found at 1280)

`AutomationFlowEditor.tsx`, `utils/anchorViewport.ts` (`revealDelta`): opening the step panel narrows the canvas, and at 100 % the selected step could end up behind the panel. On selection, the canvas pans across just enough to show the step and its delete button (`REVEAL_MARGIN`); dragging the selected step never pans.

## Tests

| File | Cases |
|---|---|
| `utils/anchorViewport.test.ts` (new) | the trigger is centred horizontally and `TOP_GAP` from the top at zoom 1; at zoom 0.5 the same anchor holds; `revealDelta`: in view stays, hidden on the right pans left, past the left edge pans right, wider than the canvas lines up left, zoom counted |
| `stats/EmailStepStats.test.tsx` | name, bar and value per row, Sent as a count with a full bar, bar widths follow the rates, an empty bar and "—" without a base |
| `AutomationFlowEditor.test.tsx` | an existing automation's viewport is set once at zoom 1 from the helper (spy on `setViewport` resolving `true`, `useNodesInitialized` mocked true, wrapper size stubbed); a `false` result is retried on the next render; a later node change does not set it again; auto-reorganize re-anchors at the current zoom and does not call `fitView`; the Reorganize button and "Fit to screen" still call `fitView`; `statsControl` renders in the top row with the overlay. The minimap: `ReactFlow` is stubbed in this file, so no case is added; its removal is checked in the captures |
| `nodes/OutputNames.test.tsx` (new) | one name per output at its handle's position (filter 30/70, list status 20/50/80, A/B and branch spread), named and toned as the links were |
| `nodes/FilterNode.test.tsx`, `nodes/TriggerNode.test.tsx` neighbours | the card's names row replaces the old Yes/No row |
| `edges/AutomationEdge.test.tsx` | a link out of a filter, list status or A/B test shows its share alone with stats on, under its output, and nothing without stats; a single-output link is unchanged |
| `utils/freeOutputPlus.test.ts` (new) | left of the middle steps left, right steps right, the middle stays; always just under the point |
| `EditorSettings.test.tsx` | the "Settings" heading names its region; the list and frequency selects keep their labels (`getByLabelText`); the `.ant-form-item` case (:101-106) kept, the items are still there; the switch stays disabled without an inbound integration and read-only |
| `EditorSettings.test.tsx` (palette cases) | an enabled item is draggable, the disabled Email item is not and keeps its tooltip; the items carry no `border` class |
| `NodeConfigPanel.test.tsx` | the `EmailConfigForm` stub (:32) renders its `preview` prop, so the existing preview cases (:306-322) keep passing; title = description when set; title = template name from the context's `templates` for an email step without description; title = type label otherwise and without the template in the list; "Delete step" is in the footer, outside the scrolling body, for an email and a delay step; no footer read-only or for the trigger; the results heading reads "Last 7 days" / "Last 30 days" / "All time" with the period; one case with the real `EmailStepPreview` (template query mocked) read-only: the Preview button is enabled |
| `config/EmailConfigForm.test.tsx` | the `preview` node renders between the two fields; the default option reads "Use workspace default · {name}" with a marketing provider, plain without |
| `broadcasts/detail/TemplatePreviewFrame.test.tsx` | `frameHeight={420}` gives a 210px box with the 300px clientWidth stub; the default stays 450px |
| `stats/StatsControl.test.tsx` (new) | the switch reflects and toggles stats; the period appears only when on, with three options, and reports the chosen one; disabled with its tooltip text for an unsaved automation and for one without journeys |
| `AutomationEditor.test.tsx` | the `AutomationFlowEditor` stub (:58-80) renders `statsControl` as well as `overlay`, so the stats cases (:224, :321, :329, :335) find the switch; the header has no "View" / "Period" Segmented any more; the switch calls `onToggleStats`; a period calls `onPeriodChange`; the `period` prop drives the stats read; the stats error banner still shows |
| `pages/AutomationsPage.test.tsx` | 7 d writes `period=7d`, All time `period=all`, 30 d removes it; `period=7d` in the URL reaches the editor; switching stats off, closing, and opening another automation clear `period` |
| `last30Days.test.ts` | `lastDays(tz, 7)` spans 7 days ending today in the timezone; `last30Days` unchanged (existing cases) |
| `useNodeStats.test.tsx` | 7 days sends `since`/`until` 6 days back; all time sends neither (existing case) |
| `__tests__/automationsRoute.test.ts` | `period=7d` and `period=all` kept, `period=30d` / garbage dropped, alongside the existing `stats` cases |
| `e2e/features/automations.spec.ts` | the existing stats case opens with `stats=1` and needs no change; add: the editor opens at 100 % (`await expect(zoomLabel).toHaveText('100%')`, the English `percent` output) |

## i18n and changelog

- New strings (expected): "Settings" if not already in the catalogue, "Use workspace default · {provider}", "Stats", "Show stats", "7 d", "30 d", "Last 7 days". "Edit" / "With stats" / "View" stay in the catalogue only if used elsewhere (extraction without `--clean` keeps obsolete entries: check they are marked obsolete, not removed by hand). `npm run lingui:extract` (no `--clean`), then ca, de, es, fr, it, ja, pt-BR; French with the typographic apostrophe.
- CHANGELOG: the 44.0 section is unreleased. If it already has an automations editor bullet, fold this into it; otherwise one short Improvement line (the editor opens at 100 %, the stats switch and a 7-day, 30-day or all-time period sit on the canvas, a compact left column, the step panel shows the template's name and its results without scrolling).

## Plan review

Reviewed once (Opus, medium effort) on 2026-11-15. Corrections taken: the URL is written by `AutomationsPage`, not the editor; the title reads the context's templates instead of a new query; `setViewport` guarded on its result after the nodes are measured; auto-reorganize re-anchors instead of keeping a stale view; branch labels pinned under their handle instead of a longer stub; the stats control shares the top row with the banner; `last30Days` kept for its importers; the test stubs that would hide the new props; `frameHeight` crops.

## Code review (medium, 2026-11-15, uncommitted tree)

- Fixed: the opening placement waited on `useNodesInitialized()`, which never turns true in edit mode (the "+" placeholders are rebuilt unmeasured), so the view stayed at `defaultViewport`; it now waits for `onInit` and the trigger's measured width. The e2e case opens a paused automation and checks the trigger is centred and near the top, which the earlier "100%" assertion could not catch.
- Fixed: the automatic re-layout re-centres across only, keeping the author's zoom and height; the opening placement stops retrying once the author pans or zooms.
- Fixed: one `outputLeft` for the handles, the names, the canvas hook's "+" positions; `STUB_GAP` shared from `nodes/outputLayout.ts`.
- Kept: "Reorganize layout" and "Fit to screen" still fit the whole flow (D1: explicit requests to see everything). The free-output map rebuilt on node changes costs no more than the placeholders already rebuilt there.

### 11. The selected step's border, as the mockup draws it (Pierre)

`nodes/BaseNode.tsx`, `styles/reactflow.css`: a selected step gets the primary border and a soft 3px halo (`ring-3 ring-primary-500/16`, the mockup's `0 0 0 3px` at 16 %) instead of a drop shadow. A pre-Nuit claire rule in `reactflow.css` forced antd's old blue `#1890ff` border and a 2px ring on every selected node over BaseNode's classes; it goes, and a selected link's stroke takes the primary token. Test: `nodes/BaseNode.test.tsx` (selected: border, ring width and colour; unselected: plain border, no ring).

## Second code review (medium)

- Fixed: the share of a link out of a multi-output step had dragged the link's hidden insert/delete buttons under the step with it; the buttons are back halfway along the link, only the share sits under its output.
- Fixed: `period` in the URL is read only with `stats=1`.
- Fixed: the free output's "+" is placed by the canvas hook (`freeOutputPlus(step, left, defaultHeight)`), one width fallback; the bar of a results row guards NaN; a stale comment goes.
- Decided by Pierre on mockups: A/B variant names on one line and their weight on the next (A1), names kept rather than A/B/C pills (N); a free "No" stub drawn in the amber of its name and share (B1).
- Kept: "Reorganize layout" fits the whole flow (D1).

## Third code review (medium)

- Decided by Pierre: after an automatic re-layout the view no longer moves, except to bring the selected step (the one just added) back into view when the layout moved it out, across and up/down (`revealDeltaY` beside `revealDelta`); the re-centring on the trigger goes.
- Fixed: one pair of helpers maps the URL's period to the editor's (`stats/periodParam.ts`); `last30Days.ts` is renamed `lastDays.ts`, its general helper being `lastDays(timezone, n)`.
- Kept, already decided: Reorganize fits the whole flow (D1); the red point under an amber "No" (B1); names everywhere, truncated with a tooltip past 3 variants (N). Kept as is: the first paint before the opening placement; the results bar written out in `EmailStepStats` (its Sent row is a count, not a rate, which `RateBar` does not draw).

## Fourth code review (medium)

- Fixed: a live automation's banner covered the top of the trigger placed 48px down; the opening placement now puts the trigger under the banner (`[data-canvas-overlay]` bottom + 16px). The e2e case of the live automation checks the trigger sits under the banner.
- Fixed: a drag that starts on an unselected step selects it, opens its panel, and the reveal could pan the canvas under the pointer; no reveal while a step is dragged.
- Fixed: the opening placement is tried again every 100ms, 10 times, when the canvas has no width yet or ReactFlow cannot move the view, instead of only on the next edit.
- Fixed: the mini bar is one component, `layout/FillBar`, used by `RateBar` and the result rows (console/CLAUDE.md: repeated recipes are components); the stub edge draws no label any more (dead since outputs are named in the card); `periodParam.ts` is the single source of the URL's periods, the router reads them through `readPeriodParam`.
- Kept, already decided: Reorganize fits the whole flow (D1), the red point under the amber "No" (B1). Kept as on dev: an API-made branch's free paths get no "+".

## Fifth code review (high)

- Fixed: a JS comment left as JSX text after the fourth review's refactor drew "// Buttons, visible on hover" in the middle of every editable link; a test checks an editable link writes no text.
- Fixed: a flow made through the API without a trigger opens on its topmost step (it stayed at the default viewport).
- Fixed: a retry of the opening placement checks again, before it runs, that the author has not moved the view and the editor is still open.
- Fixed: a link's share is placed on the curve React Flow draws (`edges/pointDownLink.ts`), not on a straight line beside it, for links going far across.
- Fixed: a free output's "+" steps aside at most 40 % of the way to its neighbour (4+ A/B variants); long translated setting labels wrap (`labelWrap`); the card sizes (300 × 86) come from `nodes/outputLayout.ts`; the stub edge's unused `sourceType`/`sourceConfig` go; "With stats" comments follow the switch.
- Fixed while re-formatting: Prettier had rewritten the router's import block in the console's single-quote style; `router.tsx` keeps its own style and only the five lines this change needs.
- Kept: the red point under the amber "No" (B1).

## Last requests (Pierre, after the fifth review)

- More room between SETTINGS, ACTIONS and LOGIC in the left column (`gap-7`, 28px).
- Layout: each level starts under the tallest step of the level above (measured height), plus 110px under a branching step and 70px under a single-output step, instead of a fixed 200px grid (`utils/layoutNodes.ts`, `layoutNodes.test.ts`). Steps of one level stay 80px apart.
- Demo data (`internal/service/demo_automations.go`): siblings at ±190px from the spine instead of ±150px (they touched), rows under a branching step 60px lower per branch above; `TestDemoAutomations_StepsDoNotTouch` pins both. Existing demo workspaces keep their stored positions until a demo reset.
- Removed: the Preview and Open template buttons of the email step panel, the icon of the list status "Branch Logic" alert, and the panel's "Delete step" footer (a step is deleted from its card's trash button or the Delete key).

## Verification

1. `cd console && npx vitest run src/components/automations src/components/broadcasts/detail src/pages/AutomationsPage.test.tsx src/__tests__/automationsRoute.test.ts src/i18n`
2. `npx tsc -b` and `npx eslint` on the touched files; Prettier (`--single-quote --no-semi --print-width 120 --trailing-comma none`) on the touched files only.
3. `npx playwright test e2e/features/automations.spec.ts`
4. Once: the full `npx vitest run`.
5. Captures at 1440 and 1280 on the demo (Welcome Series, stats on, Welcome Email selected; a filter with one free output; a read-only live automation with its banner and the stats control), side by side with mockup 72: no minimap, labels clear of the "+", and the list of remaining gaps for the PR.
