# Console redesign, screens 6: Automations (list and editor with stats)

Spec: "Spec 6 · Automatisations" (Claude Doc ea2f8d88-8828-4cd4-8d07-19b6dd6761de, 2026-11-11), screens
71 (list) and 72 (editor with stats). Mockups: `maquettes/identite-visuelle/automatisations-refonte/71-*.png`,
`72-*.png` and `sources/AutomationsRedesign.tsx.txt`.

## Need

`AutomationsPage` shows cards by ten, each with its `stats` counters and a read-only "Flow Stats" drawer
whose "Inflight" figure is wrong (it is `count_entered`, written only on the trigger). Editing happens in
`UpsertAutomationDrawer`, a plain full-screen drawer that closes on save. Spec 6 turns the list into a
table read from exact journey counts and a 30-day open rate, and the editor into one URL-driven
`FullDrawer` with settings and palette on the left, the canvas, the step panel on the right, and a
"With stats" toggle that puts each step's results on its node and on the branch edges.

## Starting point

- Server parts are on dev since #533 (`c8256be`): `automations.list?with_journey_counts=true` answers
  `journey_counts` by id; `GET automations.nodeStats` (`since`/`until` RFC 3339, together or absent)
  answers `{automation_id, since?, until?, nodes: {id: NodeStats}}` with `email` only when the member may
  read message history; `messages.list` takes `automation_id`; the `message_history` analytics schema has
  the `automation_id` dimension; `automations.get` answers 404 (#522). Nothing server-side in this PR.
- Console building blocks from earlier specs: `PageHeader`, `Panel`, `Pill`, `RateBar`, `KeyValueList`,
  `CountTabs`, `TableToolbar` (+ `ToolbarSearch`), `FullDrawer`, `RailSection`, `Mono`, `format.ts`,
  `lib/copyToClipboard.ts`, `components/templates/compileTemplatePreview.ts`,
  `components/web_analytics/lib/dates.ts` `resolveRange`.
- Branch `feat/console-automations-screens` from dev **after #540 (broadcasts) merges**: it brings
  `FullDrawer`'s `barTitle` slot, `components/broadcasts/CouldntLoad.tsx` and `workspaceTimezone`
  (`components/broadcasts/access.ts`), all reused here.
- The Logs thread owns `MessageHistoryTab` (spec 8): it adds the "Automation ID" filter and reads
  `automation_id`/`template_id` from `/logs`. This PR only adds `automation_id` to the API client.
  Until the Logs PR lands, "View messages" opens Logs unfiltered by automation (known, not handled).

## Shared-component changes (minimal, reported in the PR)

1. `components/layout/FullDrawer.tsx`: a `fill?: boolean` prop. With it, the drawer body is a column of
   the viewport's height and the content slot is `flex min-h-0 flex-1` without the page padding, for a
   screen that lays out its own columns (the canvas). Default unchanged.
   Test (`FullDrawer.test.tsx`): with `fill`, the content slot has no padding classes and fills; without,
   unchanged.
2. Nothing else. The editor's top bar is built from `barTitle` and `topActions`.

## Changes

### A. Route and API clients

- `router.tsx`: `AutomationsSearch {status?: 'live'|'paused'|'draft'; q?: string; automation?: string;
  stats?: '1'}` and `validateAutomationsSearch`, normalised like `validateBroadcastsSearch` (first value of
  an array, trimmed strings, unknown status dropped, `stats` kept as `1` when `'1'` or `1`, a numeric `automation` turned into its string, empty `q` dropped).
  Exported for the test.
- `services/api/automation.ts`:
  - `ListAutomationsRequest.with_journey_counts?: boolean` sent as `with_journey_counts=true`;
    `ListAutomationsResponse.journey_counts?: Record<string, JourneyCounts>`;
    `JourneyCounts {active, completed, exited, failed, total}`.
  - `NodeStats`, `NodeEmailStats`, `NodeStatsResponse` mirroring `internal/domain/automation_node_stats.go`;
    `nodeStats({workspace_id, automation_id, since?, until?})` → `GET /api/automations.nodeStats`.
  - `activate`/`pause` typed `Promise<{ success: boolean }>` (what the handler writes).
  - Removed: `AutomationNodeStats`, `GetNodeStatsRequest`, `GetNodeStatsResponse`, `getNodeStats` (the
    analytics-based "Flow Stats"), and the `analyticsService` import if unused.
- `services/api/messages_history.ts`: `automation_id?: string` in the list params, appended only when set.
- `services/api/permissions.ts`: `/api/automations.nodeStats` in the `automations` read catalogue, with the
  note that email figures also need message history read.

### B. Shared automation helpers (pure)

- `components/automations/nodeTypes.ts` (new): one table per node type: lucide icon, tone (whole class
  strings: chip `bg-*`/`text-*` from the Pill tones), and its label as a Lingui `msg`. Tones follow the
  mockup: trigger warning, email primary, add to list success, remove from list danger, webhook neutral,
  delay neutral, filter/list status/A/B test/branch info. Used by the palette, `AddNodeButton`, the node
  cards, the panel header and the list's Steps column, so the four agree. Replaces the hex
  `nodeTypeColors` reads in the files this PR restyles; `nodes/constants.ts` keeps `getNodeDescription`.
- `components/automations/describeTrigger.ts` (new): `EVENT_KIND_OPTIONS` as `msg` descriptors (the
  cascader options of `TriggerConfigForm.tsx:76-112` move here, the form reads them through `i18n._`) and
  `describeTrigger(trigger, {lists, segments, broadcasts?, templates?}, i18n)` returning
  `{text: string, mono?: string} | null` per the spec's table: full messages with their variable
  ("Subscribed to {list}", "Joined segment {segment}", "Clicked {url}", "Clicked {url} in {name}",
  "Event {name}"), the id when the name is unknown, `null` for no trigger ("No trigger configured" is
  rendered by the caller). `mono` carries the URL or event name so the row shows it in `Mono`.
- `components/automations/stepIcons.ts` (new): `orderedSteps(automation)`: breadth-first from
  `root_node_id` over `next_node_id` and every branch target (filter continue/exit, A/B variants, list
  status three targets, branch paths and default), trigger excluded, each node once; nodes unreachable
  from the root appended in their array order. `stepSummary(steps, max = 6)` → `{shown, rest}`.
- `components/automations/journeyRates.ts` (new): `finishedOf(c)`, `completedRate(c)` and `exitedRate(c)`
  over completed + exited + failed, `null` on a zero denominator; `neverRan(automation, counts)` (draft
  with `total = 0`) for the "—" cells.
- `components/automations/branchKeys.ts` (new): `branchKeyOfHandle(nodeType, sourceHandle)` (filter
  `continue`→`yes`, `exit`→`no`; list status and A/B test and branch: the handle id) and
  `branchShare(stats, key, extraKeys)` → `{count, rate}` over the node's completed passes (sum of
  `branches`); the branch default path's edge adds the `default` key.

### C. List (screen 71)

- `pages/AutomationsPage.tsx` rewritten, named and default export kept:
  - URL: `useSearch` for `status`, `q`, `automation`, `stats`; tab and search write with
    `replace: true`; search debounced 300 ms (local input state, like broadcasts).
  - A1 `useQuery(['automations', ws], list({with_journey_counts: true}))`, `staleTime` 30 s, no limit.
  - A3 lists and segments (existing keys), templates as today (no category filter, only with
    templates read: the existing test keeps).
  - Tabs `CountTabs` All/Live/Paused/Drafts counted after the search; search over name and the trigger
    label (`describeTrigger` text + mono), case- and accent-insensitive (`normalize('NFD')` + strip marks).
  - `PageHeader` title, count, "New automation" (`?automation=new`), disabled with the current tooltip
    without write.
  - States: Skeleton (6 rows), `Empty` "No automations yet" with the sentence and "Create your first
    automation", "No automations match" + "Clear filters", `Result` error with "Retry".
  - Renders `AutomationEditor` when `automation` is set (D).
- `components/automations/list/AutomationsTable.tsx` (new): antd `Table` per the spec's columns: Status
  `Pill` (Draft neutral, Live success with pulse dot, Paused warning); Automation (name with tooltip
  "ID {id} · Last activity {relative}", frequency pill "Once"/"Every time", second line zap icon +
  trigger label); Steps (icon chips, "+{n}", tooltip listing each step's description or label); List
  pill or "No list"; In progress; Entered; Completed `RateBar` + tooltip of the three counts; Exited %;
  Failed (danger tone when > 0); Open rate; actions (Pause or Activate icon with the current
  Popconfirms and tooltips, then a "…" menu: Edit, View stats, View messages, Delete with "Pause the
  automation first" for a live one). Row click: `stats=1` for live/paused, none for a draft. The open
  row keeps `is-hover`. Shows 50 rows of the filter, then "Show {n} more" (+50, no call). Footer
  "{n} automations". Toolbar note "Journeys since creation · open rate last 30 days".
- `components/automations/list/useAutomationOpenRates.ts` (new): A2 `analytics.query` on
  `message_history`, measures `count_opened`, `count_opens_tracked`, dimension `automation_id`, filters
  `automation_id in [ids with an email node, sorted]` and `created_at inDateRange [startUtc, endUtc]`
  from `last30Days(tz)` (R1); key
  `['automation-open-rates', ws, ids, startUtc]`, `staleTime` 10 min; disabled without
  `message_history.read` or without such ids. Returns `Map<id, rate|null>`, `loading`, `error`
  (error → "—" with the "Couldn't load" tooltip, `CouldntLoad`).
- `components/automations/list/useAutomationActions.ts` (new): activate (400 shows the server's
  reason), pause, delete, each invalidating `['automations', ws]` and, when cached,
  `['automation', ws, id]`; shared by the table and the editor.

### D. Editor (screen 72)

- `components/automations/AutomationEditor.tsx` (new, replaces `UpsertAutomationDrawer.tsx`):
  - `FullDrawer fill backLabel="Automations"`; `push={false}` has to stay for the condition drawers:
    `FullDrawer` gets no `push` prop, so the editor's own condition drawers keep working only if rc-drawer
    does not push a `FullDrawer` parent; checked in the browser, and if it does, `FullDrawer` takes a
    `push` passthrough (second minimal shared change, reported).
  - Session key fixed at opening (`id` or `new`) in state; E1 `automations.get` (`['automation', ws,
    id]`), 404 → "Automation not found" with a back button; never read from the list.
  - Mounts `AutomationProvider` keyed by the session key. After a create, the provider receives the
    created automation (`onSaved(automation)`), the URL moves from `new` to the id with
    `replace: true, ignoreBlocker: true`; the key does not change, undo history kept.
  - Top bar: `barTitle` = editable name (`Input` borderless, `aria-label` "Automation name"), status
    `Pill`, "Unsaved changes" (warning) / "Saved" (ink-3, after a save of this session). `topActions` =
    Undo/Redo, `Segmented` Edit / With stats (disabled for an unsaved automation; disabled with "No
    journeys yet" for a draft with no journey per E3/A1 counts), period `Segmented` Last 30 days / All
    time when stats are on (local state, default 30 days), Pause or Activate (Activate disabled with
    "Save your changes first" while dirty), Save/Create (primary, ⌘S/Ctrl+S, absent read-only), "…"
    menu (Copy ID, View messages, Delete for draft/paused → closes with `ignoreBlocker`).
  - Read-only for a live automation or without `automations.write`: `ConfigProvider componentDisabled`
    around the left column and the right panel, canvas props (below), no Undo/Redo/Save nor their
    shortcuts. Live: `Alert` over the canvas "This automation is live. Pause it to edit." with "Pause";
    after the pause the editor turns editable without closing (status read from the refetched E1).
    Without write, no banner; `?automation=new` is removed from the URL.
  - Leaving: `useBlocker({shouldBlockFn: ({current, next}) => dirty && (next.pathname !==
    current.pathname || next.search.automation !== current.search.automation), enableBeforeUnload: () =>
    dirty, withResolver: true})` with the current "Unsaved Changes" modal; `FullDrawer onClose` runs the
    same confirmation (back, close, Escape). Toggling `stats` never blocks.
  - Undo shortcuts ignored when the focus is in an input, textarea, select or contenteditable.
  - Three columns: left `w-rail` (`EditorSettings` + `NodePalette` + help line), canvas, right panel
    `w-aside` when a step is selected (overlays the canvas below 1200 px).
- `components/automations/context/AutomationContext.tsx`:
  - `buildAutomationPayload({id, workspaceId, name, listId, exitOnReply, nodes, edges, base})` extracted
    to `utils/buildAutomationPayload.ts` (pure); `save()` uses it.
  - `dirty` = `stableKey(payload without timestamps) !== savedKey`, `savedKey` taken at load and after
    each successful save (keys sorted). Replaces `hasUnsavedChanges` booleans (kept as the context's
    `hasUnsavedChanges` name, now derived).
  - Created automation: the provider holds `saved: Automation | null` state; `isEditing` and the id come
    from it, so the next Save calls `update` with the same id.
  - Success: `setQueryData(['automation', ws, id], res.automation)`, invalidate `['automations', ws]`
    and `['automation-node-stats', ws, id]`, new snapshot, `onSaved(res.automation)`; editor stays open.
  - Failure: keep edits, current messages (`changedDespiteFailure`, paused warning), invalidate
    `['automation', ws, id]` and the list; a 409 shows "This automation was changed elsewhere" with
    "Reload" (confirm if dirty, then refetch E1 and reinitialise the provider from it).
  - `readOnly` and `frequency` / `setFrequency` (writes the trigger node's `config.frequency`) in the
    context value.
- `components/automations/EditorSettings.tsx` (new): List select (current rule: no list → email
  disabled; clearing with email nodes refused as today), Frequency select, Exit on reply switch with the
  two current help texts.
- `components/automations/NodePalette.tsx`: two groups, Actions (Email, Add to list, Remove from list,
  Webhook) and Logic (Delay, Filter, List status, A/B test), chips from `nodeTypes.ts`, drag as today,
  hidden read-only.
- `components/automations/AddNodeButton.tsx`: labels from `nodeTypes.ts` (`msg`), icons from the same
  table; `ADD_NODE_MENU_ITEMS` keeps keys and order for the existing callers.
- `components/automations/config/TriggerConfigForm.tsx`: frequency field removed (moved), event options
  read from `describeTrigger.ts`.
- `components/automations/config/ConditionsField.tsx`: `readOnly` (from antd's disabled context):
  conditions listed, no edit button.
- `components/automations/AutomationFlowEditor.tsx`: `readOnly` from the context →
  `nodesDraggable={false}`, `nodesConnectable={false}`, `deleteKeyCode={null}`, no floating "+", no
  edge buttons, no drop; palette moved out to the left column; restyled background (`bg-subtle`
  dots); right panel in its own column, not absolutely positioned. `Controls` bottom-left, minimap and
  "Reorganize layout" kept.
- `components/automations/nodes/*`: `BaseNode` restyled as a `Panel`-like card: type chip icon, type in
  small caps, title (description, else the computed label), detail line, selected border `primary`;
  the stats footer slot. Each node passes its detail line. No hex colours left in `BaseNode`.
- `components/automations/stats/NodeStatsContext.ts` (new): `{enabled, period, loading, error,
  nodes?: Record<string, NodeStats>, savedNodeIds: Set<string>, canReadHistory}`; provided by the
  editor so node components and edges read stats without putting them in node data (which feeds the
  payload and the undo history).
- `components/automations/stats/NodeStatsFooter.tsx` (new): per the spec's table (trigger entered;
  email sent · opens · clicks, or reached without history; delay waiting; filter checked; branches
  reached; list steps added/removed + errors (warning); webhook calls), then pills failed (danger),
  retrying (warning), in progress (info), skipped (neutral, reasons in tooltip) on email. One-line
  Skeleton while loading; nothing for an unsaved node or when stats are off/errored.
- `components/automations/edges/AutomationEdge.tsx` and `edges/AddNodeEdge.tsx`: a label chip on edges
  out of a multi-exit node: base label (Yes green / No warning, Not in list / Active / Non-active,
  "{name} ({weight} %)", branch path name) translated in the edge from the source node's config, and
  with stats " · {p} %" (tooltip: the count). `none` never labelled. The stub to "+" keeps its label.
- `components/automations/useNodeStats.ts` (new): E3 `['automation-node-stats', ws, id, startUtc|'all']`,
  `since/until` from `last30Days(tz)` (R1) for "Last 30 days", none for "All time";
  enabled for a saved automation when stats are on or a step panel is open; `refetchInterval` 60 s only
  when live and watched; `staleTime` 60 s.
- `components/automations/NodeConfigPanel.tsx`: header (icon chip, type small caps, title, close), the
  current form, then stats block and "Delete step" (danger, not for the trigger nor read-only,
  Popconfirm as the node's current one, calls `removeNode`).
  - `components/automations/stats/EmailStepStats.tsx` (new): "Last 30 days"/"All time" title, four
    `RateBar` rows (Sent full bar with count, Delivered/sent with "—" + SMTP tooltip when the step's
    provider is SMTP (node `integration_id`, else the marketing provider), Opened/opens_tracked,
    Clicked/clicks_tracked), "{failed} failed · {unsubscribed} unsubscribes · {bounced} bounces",
    "{queued} queued", "{not_sent} not sent", "View messages"
    (`/logs?automation_id={id}&template_id={template_id}`). Without history: the sentence.
  - `components/automations/stats/StepStats.tsx` (new): `KeyValueList` Reached, Completed, In
    progress, Waiting (delay), Retrying, Failed; splits as `RateBar` rows for multi-exit nodes.
  - `components/automations/stats/EmailStepPreview.tsx` (new): from/subject and compiled HTML in a
    scaled iframe via `compileTemplatePreview`, "Preview" (`TemplatePreviewDrawer`), "Open template"
    (`/templates?q={name}`, new tab); without templates read "You need access to templates".
  - Unsaved step: "Save to start counting this step"; never-run draft: no stats block.
- Deleted: `AutomationCard.tsx`, `AutomationFlowViewer.tsx` (+test), `nodes/StatNode.tsx` (+test),
  `UpsertAutomationDrawer.tsx` (+test). Their useful cases move as listed in the tests below.

### E. Smoke, e2e, i18n, changelog, docs

- `src/__tests__/pages.smoke.test.tsx`: the automations mock targets `../services/api/automation`
  (`automationApi`), plus analytics/lists/segments mocks; the page renders "Automations" and a mocked row.
- `e2e/fixtures/auth.ts`: `automations.list` answers two automations (one live with an email node, one
  paused) with `journey_counts` on the data fixture, empty otherwise; `automations.get` answers the one
  asked or 404; `automations.nodeStats` answers stats for it; `analytics.query` keeps its default.
- `e2e/features/automations.spec.ts` (new): list rows and numbers; open by URL; With stats shows a
  footer and a split label; a live one opens read-only (banner, no Save); editing a paused one shows
  "Unsaved changes" and closing asks to confirm.
- `e2e/protected-pages.spec.ts`: "Create Automation" → "New automation".
- `.po`: extract, then fill the seven locales (catalogues test), after rebasing on dev.
- `CHANGELOG.md`: extend #533's 44.0 automations Feature line with the console part, short and
  customer-facing; the "flow stats" Fix line of the spec (Inflight) added if absent.
- Docs PR (`docs/automations-screens`, from docs `dev`): `features/automations.mdx` per the spec (list
  capture, Automation-Level Stats, Node-Level Stats, "Flow Stats" → "the stats view", Permissions with
  `automations.nodeStats`), `openapi.json` copied from notifuse.

## Files and tests (TDD: tests first)

| Implementation | Test | Cases |
|---|---|---|
| `router.tsx` `validateAutomationsSearch` | `src/__tests__/automationsRoute.test.ts` (new) | status kept/unknown dropped; `q` trimmed, empty dropped; `automation` kept; `stats` `'1'`/`1` kept, other values dropped; arrays take the first |
| `services/api/automation.ts` | `services/api/automation.test.ts` (new) | list URL with `with_journey_counts=true` and without limit; `nodeStats` URL with and without `since`/`until`; activate/pause POST bodies and typed response |
| `services/api/messages_history.ts` | `messages_history.test.ts` | `automation_id` sent only when set |
| `services/api/permissions.ts` | `permissions.test.ts` | `automations.nodeStats` listed under automations read |
| `components/layout/FullDrawer.tsx` | `FullDrawer.test.tsx` | `fill` content slot without padding; default unchanged |
| `nodeTypes.ts` | `nodeTypes.test.ts` (new) | every `NodeType` has icon, tone and label; palette keys ⊂ table |
| `describeTrigger.ts` | `describeTrigger.test.ts` (new) | contact events; each list action with name and with unknown id; segment joined/left; email events; link clicked plain, in a broadcast, in a template; custom event mono; no trigger → null |
| `stepIcons.ts` | `stepIcons.test.ts` (new) | linear order; filter/A-B/list status/branch targets breadth-first; a node reached twice counted once; trigger excluded; unreachable nodes appended; more than six → `rest` |
| `journeyRates.ts` | `journeyRates.test.ts` (new) | rates over finished journeys; zero denominator → null; never-ran draft |
| `branchKeys.ts` | `branchKeys.test.ts` (new) | filter handles → yes/no; variant and path ids; default path adds `default`; `none` ignored; share over completed passes; no passes → null rate |
| `pages/AutomationsPage.tsx` + `list/*` | `pages/AutomationsPage.test.tsx` (rewritten, keeps the three template cases) | tabs and counts after search; search on name and trigger label, accents, written to `q` after 300 ms; status tab in URL; percentages over finished journeys; "—" for zero denominator and never-ran drafts; failed in danger; open rate only for automations with an email node, "—" + "Couldn't load" on A2 error, no A2 call without history read; Activate and its 400 reason; Pause; Edit/Delete only for draft and paused, Delete disabled live with its reason; row click live → `stats=1`, draft → none; New automation disabled without write; Show {n} more; empty, no match + Clear filters, error + Retry |
| `list/useAutomationOpenRates.ts` | `useAutomationOpenRates.test.tsx` (new) | query shape (dimension, filters, sorted ids, range); disabled without ids / without permission; map of rates, `null` when nothing tracked |
| `AutomationEditor.tsx` | `AutomationEditor.test.tsx` (new) | create then Save calls `update` with the same id, URL `new` → id, provider not remounted (undo kept); Save keeps the editor open and shows "Saved"; Activate disabled while dirty; "Unsaved changes" after an edge change, a name change, a list change; gone after undoing back to the saved state; blocker: `stats` toggle passes, another automation blocks, confirmed close and Delete pass; Escape with changes asks; With stats writes `stats=1` (replace); read-only live (banner, Pause, no Save/Undo, fields disabled) and without write (no banner); `new` without write removed from the URL; undo shortcut ignored in the name input; 409 → Reload; failed save invalidates the automation and list; 404 → "Automation not found" |
| `context/AutomationContext.tsx`, `utils/buildAutomationPayload.ts` | `AutomationContext.test.tsx` (extended), `buildAutomationPayload.test.ts` (new) | payload identical to today's for a fixture flow; key ignores timestamps and key order; after create `isEditing` true with the same id; frequency written to the trigger config |
| `EditorSettings.tsx`, `NodePalette.tsx` | `EditorSettings.test.tsx`, `NodePalette.test.tsx` (new) | frequency to `trigger.frequency`; Exit on reply texts by provider; email disabled without list (tooltip); groups and order; disabled/hidden read-only |
| `config/TriggerConfigForm.tsx` | `TriggerConfigForm.test.tsx` | frequency cases moved to `EditorSettings.test.tsx`; event options still selectable |
| `config/ConditionsField.tsx` | `ConditionsField.test.tsx` | disabled: conditions shown, no edit button |
| `AutomationFlowEditor.tsx` | `AutomationFlowEditor.test.tsx` (new) | read-only: not draggable, not connectable, no delete key, no "+", no drop |
| `nodes/*`, `stats/NodeStatsFooter.tsx` | node tests (updated), `NodeStatsFooter.test.tsx` (new) | each type's footer; failed/retrying/in progress pills; skipped with reasons; list steps errors; email without `email` → reached; unsaved node no footer; loading skeleton (cases from `StatNode.test.tsx` that still apply) |
| `edges/AutomationEdge.tsx`, `edges/AddNodeEdge.tsx` | `AutomationEdge.test.tsx` (new) | filter Yes/No with shares; A/B "{name} ({weight} %)" + share; branch default; `none` never; no passes → no percentage; stats off → base label only |
| `useNodeStats.ts` | `useNodeStats.test.tsx` (new) | `since`/`until` from `resolveRange` for 30 days, none for all time; disabled unsaved / not watched; refetch interval only live and watched |
| `NodeConfigPanel.tsx`, `stats/*` | `NodeConfigPanel.test.tsx` (extended), `EmailStepStats.test.tsx`, `EmailStepPreview.test.tsx` (new) | email: four bars, queued, not sent, View messages link with template_id, SMTP by the step's provider then the workspace's; without history read; unsaved step; trigger without Delete step; read-only without Delete step; preview without templates read |
| deleted files | — | — |
| `pages.smoke.test.tsx` | itself | the page renders a mocked row |
| e2e fixtures/specs | `automations.spec.ts`, `protected-pages.spec.ts` | as in E |

## Plan review and corrections (Opus xhigh, 12/11)

The review found 13 blocking points and 9 others. Each is taken; these rules override the sections
above where they differ, and the code review below overrides them where it says so (R1, R19, R21).

- **R1 (B2) One 30-day window.** `resolveRange` takes a `DayjsRange`:
  `components/automations/last30Days.ts` exports `last30Days(tz) =
  resolveRange(computeDateRange('previous_30_days', tz), tz)`, used by A2 and E3. Test: start and end
  of the window in a non-UTC timezone.
- **R2 (B1) `FullDrawer push`.** Second shared change: `push?: DrawerProps['push']` passed through
  to antd's `Drawer`; the editor passes `false` (rc-drawer moves the parent by the parent's own
  `push`, so the condition, template selector and preview drawers would slide the editor 180 px).
  Test: the prop reaches the Drawer.
- **R3 (B3) Dirty key.** The session id is fixed once (`useState(() => automation?.id ?? uuidv4())`);
  `isEditing` comes from `saved`. `stableKey` drops `status`, `created_at`, `updated_at` at the top
  and on each node, sorts object keys and nodes by id, and drops `undefined` like JSON. `savedKey` is
  the key of the payload actually sent (the handlers echo it). Tests: a new untouched automation is
  clean; pausing in the editor leaves it clean; node order and `undefined` fields do not count.
- **R4 (B4) Read-only is total.** Name rendered as text; nodes get no `onDelete`; no Reorganize;
  `updateNodeConfig`, `setNodes` writers and the settings setters are no-ops; `dirty = !readOnly &&
  …` (ABTestConfigForm's mount effect writes config). Tests for each, and the blocker never fires
  read-only.
- **R5 (B5) Disable forms only.** `ConfigProvider componentDisabled` wraps the config forms and
  `EditorSettings`, not the panel header, stats, preview or menus (it disables antd Buttons through
  portals). `ConditionsField` reads `useAutomation().readOnly` (DisabledContext is not public).
  Test: "View messages" and "Preview" work on a live automation.
- **R6 (B6) Branch nodes.** `nodes/BranchNode.tsx` (one source handle per path, id = path id) is
  registered; `AutomationFlowEditor.test.tsx` takes the viewer's "every NodeType registered" and
  "every edge sourceHandle declared" cases.
- **R7 (B7) Session lifecycle.** E1 follows the created id once known. Skeleton and "Automation not
  found" only on the first load; a later E1 error keeps the provider. 409 Reload: `await refetch()`,
  then remount the provider with key `${session}:${reloadCount}`.
- **R8 (B8) Selection is lifted.** `useAutomationCanvas` reports selection through the context
  (`selectedNodeId`), so the editor knows a step panel is open; `useNodeStats` and the stats context
  live in the editor.
- **R9 (B9) Delete removes.** `removeQueries` for `['automation', ws, id]` and
  `['automation-node-stats', ws, id]`, then invalidate the list, then close.
- **R10 (B10) Plurals.** `plural()` for "{n} automations", "{n} entered", "{n} sent", "{n} waiting",
  "{n} failed", "{n} retrying", "{n} in progress", "{n} skipped", "{n} added", "{n} removed", "{n}
  errors", "{n} calls", "{n} queued", "{n} not sent", "{n} checked", "{n} reached", "Show {n} more";
  a line with several counts is several plurals joined by " · ". Tests with 1 and 2.
- **R11 (B11) E3 error.** `Alert` over the canvas "Couldn't load stats" with "Retry"; no footers.
- **R12 (B12) Menu conditions.** View stats absent for a never-run draft; View messages absent
  without an email node or without `message_history.read`, in the list and the editor menu. Tested.
- **R13 (B13)** `stats` is the number 1 (see A).
- **R14 (N14)** `setFrequency` pushes history like `updateNodeConfig`; the control is an antd
  `Select`.
- **R15 (N15)** Reuse #540's `TemplatePreviewFrame`/`useTemplatePreview` and
  `isSmtpMarketingProvider`; E4 `templates.get` keyed `['template', ws, id]`.
- **R16 (N16)** Layout uses each node's `measured.height` (fallback 86) and the stubs sit below the
  measured bottom; width stays 300.
- **R17 (N17)** Handle and stub colours from `theme/tokens` (`colors.status`, `colors.primary`), no
  hex in the files touched.
- **R18 (N18)** Zoom: a small custom control bottom-left ("−", percentage from `useViewport`, "+",
  fit view) replaces `Controls`; the minimap moves bottom-right.
- **R19 (N19)** `describeTrigger` returns `{message, values}` and the row renders it with Lingui's
  `<Trans id values components>` so the Mono part is a component of the full message; `none`
  branch passes go in the node's tooltip; without lists/segments read the selectors show the saved
  value only.
- **R20 (N20)** The `FullDrawer` sits inside the provider; undo shortcuts are skipped when a drawer
  is on top of the editor (`isTopmost` as in `useDrawerNavKeys`); no E3 for a never-run draft; "No
  journeys yet" from A1.
- **R21 (N21) Added tests.** `AddNodeButton` translated labels; `StepStats`; "Delete step" confirm
  → `removeNode`; Activate/Pause/Delete disabled without write; "Save Anyway" and the empty-name
  modal (moved from `UpsertAutomationDrawer` into the editor); the editor's canvas height; read-only
  cases of R4; node sorting in the key; `messages_history.test.ts` is new. No count in a test name
  without an assertion pinning it.
- **R22** Cascader options are `TriggerConfigForm.tsx:76-118`.

## Known, not handled (from the start)

- "View messages" lands on Logs without the automation filter until the Logs PR reads `automation_id`.
- Query cost, measured (left by #533 for this PR). Local PostgreSQL 16 on port 15433, workspace schema
  from `InitializeWorkspaceDatabase`, one automation of 1,000,000 journeys over a year (trigger, email,
  delay, filter, then one of two emails: 4.96 M executions, 1.98 M messages) beside 29 small ones, plus
  2 M broadcast messages over 60 days; the real repository code run with `auto_explain` (ANALYZE,
  BUFFERS), default `work_mem`:
  - A1 `CountJourneysByStatus` (30 ids): 93 ms. A2 open rates (30 days, 30 ids): 200 ms, through
    `idx_message_history_automation_id`.
  - E3 `nodeStats` "Last 30 days": 1.6 s in all (visits 0.5 s, email 0.49 s, the rest < 0.25 s each).
  - E3 `nodeStats` "All time": 8.2 s in all: visits 4.5 s (parallel seq scan, then the
    `COUNT(DISTINCT contact_automation_id)` sort spills 120 MB per worker to disk), email 2.8 s (2 M
    primary-key lookups in `message_history`), splits 0.55 s, the rest < 0.25 s.
  - The `(automation_id, entered_at)` index of the spec would not help "All time" (the automation is
    the table: a seq scan is the right plan), and "Last 30 days" is already under 2 s. Reported to
    Pierre as a decision: keep it (default period is 30 days, cached 60 s, asked only when watched) or
    a server follow-up that counts visits without the distinct sort.
- The undo history covers nodes and edges only, as today (spec).

## Verification

1. `cd console && npx vitest run <touched tests>` while working; then `npm test` once.
2. `npx tsc -b`, `npx eslint .`, `npm run build`.
3. e2e locally (scratchpad configs, port 19173): `npx playwright test e2e/features/automations.spec.ts
   e2e/protected-pages.spec.ts --trace off`, then the port freed.
4. Captures at 1440 and 1280 into `/mnt/project-files/captures/fil-5-6/automations/`: list, editor
   edit mode, With stats, email step panel, read-only live, empty.
5. Docs: `mint broken-links`.

## Implementation notes

- Rebased on dev after the Templates page (#541): "Open template" in an email step's panel opens
  `/templates?preview=<template id>`, the parameter that page reads, rather than a search by name.
- `.po`: the seven locales filled for every message this PR adds; the licence messages dev leaves
  untranslated outside `fr` are not this PR's.
- Local e2e: `automations.spec.ts` and the automations case of `protected-pages.spec.ts` pass. The
  `LogsPage` case of `protected-pages.spec.ts` fails on dev as well (it expects the description the
  Logs screen had before its redesign); not this PR's screen.

## Code review (Fable high, on 593cc97) and corrections

Blocking, fixed in the following commit:

- **Stale copy on reopening.** E1 kept its cache, so an automation reopened within the cache time was
  copied into the editor from the old read, and the fresh one ignored: saving overwrote a newer
  version. The editor now waits for a read made since it opened (`isFetchedAfterMount` and a
  successful status) before mounting the provider. Test: a cached two-step copy and a one-step
  server copy; the editor shows and saves the server's.
- **Read-only not total.** A field that sets its own `disabled` wins over `ConfigProvider
  componentDisabled`: the template field of the email and link-click trigger forms, the Exit on reply
  switch and the A/B variant removal stayed enabled. Each now takes the editor's `readOnly`. Tests
  with the real template field under a disabled provider, the switch read-only with an inbound
  provider, and the variant buttons.
- **Double create.** ⌘S during a create sent a second `automations.create`. `save()` refuses while a
  save runs (a ref) and the shortcut ignores auto-repeat. Test: two presses and a repeat, one create.
- **R1 changed: "Last 30 days" includes today.** `previous_30_days` ends yesterday, so an automation
  activated today read zeros in its default stats view next to its in-progress pills.
  `last30Days(tz)` is now the 30 days ending today, today included, in the workspace's timezone
  (list open rate and step results alike). Tests in Paris across the DST change and in Tokyo.
- **Test that could not fail.** The read-only drop case read the nodes before React rendered: now in
  `act`, with the positive case beside it.
- **R19 changed.** Passes of a branch that took no path (`none`) are a "No path" row of the step
  panel's splits rather than a tooltip, so the shares add up to 100 %. Test.
- **R5, R21 tests added.** View messages and the preview on a read-only email step; Pause disabled and
  no Delete without write. **R21 "Save Anyway"** stays untested: no validation rule emits a
  `Warning:` today, so the confirm is unreachable; kept for when one does.

Known, not handled (reported, not blocking):

- Edits made while E1 turns live (a failed save's refetch after someone activated the automation)
  become read-only without a word.
- The first load renders the skeleton in one `FullDrawer`, then the editor in another: the drawer may
  slide in twice.
- Activate and Pause are clickable again between the action's answer and the E1 refetch.
- Under 1200 px the "+" buttons paint over the step panel (as before).
- Deleting an edge of an API-made branch node leaves `paths[].next_node_id`.
- "View messages" in the step panel is a plain link (full page load); "This template no longer
  exists" for any failed template read; open rate reads "—" while loading; tooltips say Ctrl on a
  Mac; a few Tailwind margin/width classes on antd `Input` and `Form.Item`; the live-save warning
  path of the context is now unreachable; `markAsChanged` is an empty function; the stats context
  value and the payload key are rebuilt on every render.
