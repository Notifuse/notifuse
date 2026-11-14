# Web analytics on the Nuit claire components — plan

## Problem

The web analytics screens predate the Nuit claire redesign (#530 theme, #538 dashboard). They took its colours for free, because the `@theme` block of `index.css` remaps every `gray-*` class to the token values, but never moved onto the shared components of `components/layout/` (`Panel`, `KpiStrip`) nor onto the themed antd `Table`. Each widget still draws its own card, KPI cells and table, so next to the main dashboard (`pages/AnalyticsPage.tsx`) they read as another product:

| | Nuit claire (`Panel`, `KpiStrip`, antd `Table`) | Web analytics today |
|---|---|---|
| Card background | `bg-surface` (white) + `shadow-card` | none: the `bg-canvas` page (`#f3f4f7`) shows through, no shadow (`DimensionTableWidget`, `TrafficHeatmapWidget`, the chart block of `DashboardTab`, `GoalCard`) |
| Radius | `rounded-lg` (8px) | `rounded-md` (6px), `rounded-lg` on the heatmap only |
| Card header | 40px (`h-10`), `pl-3.5 pr-3`, `border-line-soft` divider, title `text-base` semibold `ink` | `px-4 pt-4 pb-4` (≈51px), no divider, `gray-900` |
| Card body | `px-3.5 py-3` | `px-4 pb-4` / `pb-6` |
| KPI cell | `px-3.5 py-2`, value `text-kpi` (17px), 1px `line-soft` separators, active = `bg-primary-50` + 2px `primary-500` bar, hover `bg-fill` | `p-4`, value `text-xl` (20px, Tailwind's default, not a token) `gray-800`, `gray-200` borders, active = underline only, hover `gray-50` |
| Change vs previous period | `sub` line, `text-xs`, `success-fg` / `danger-fg` | `Delta`: inline `style`, `#10b981` / `#f97316` (orange); the chart tooltip uses `#10b981` / `#ef4444` |
| Space between cards | `gap-3` (12px) everywhere | `mt-6` (24px) between rows, `gap-x-4` / `gap-4` (16px), filter bar `mb-4`, alert `mb-4` |
| Table header | ≈30px, `bg-subtle`, 11.5px weight 500 `ink-3` (`antd-overrides.css`) | 46px (`h-[46px]`), no background, 12px `font-normal` `gray-500` |
| Table rows | 12.5px `ink`, `line-soft` rules, `bg-subtle` hover | 36px (`h-9`), 12px `gray-700`, `gray-50` rules, `gray-50/60` hover |
| Tabs inside a card | antd `Tabs` (13px, gutter 18px, `ink-3` → `ink`, `primary-500` ink bar) or `Segmented` in the `Panel` `extra` | `WidgetTabs`: 12px, `gap-4`, `gray-500` → `gray-900` |
| Charts (ECharts) | `colors.ink2` / `ink3` / `line` / `gray[100]`, labels 10.5px, legend 11.5px, height 250 | hard-coded `#6b7280`, `#374151`, `#e5e7eb`, `#f3f4f6` (Tailwind's former grays), labels 10px, tooltip 12px, height 220 |
| Text colours | `ink` … `ink-4` by role (`console/CLAUDE.md`) | `gray-*` in 29 files; `gray-800`/`gray-700` are not exactly `ink`/`ink-2` |

## Decisions already taken (Pierre, 2026-11-14)

- Plan first, implementation after approval.
- The change vs the previous period takes the console tokens: `success-fg` for an improvement, `danger-fg` for a deterioration, as on the main dashboard's KPIs. The orange (`#f97316`) goes, and so does `#10b981`.

## Decisions proposed (to confirm before implementing)

- **P1 — tabs inside a card: keep `WidgetTabs`, restyled to the antd `Tabs` tokens** (13px, gutter 18px → `gap-4.5`, `py-2`, `ink-3` → `ink`, `primary-500` bar, rule `line-soft`, inline padding `px-3.5` like the panel). Its doc comment still holds: antd `Tabs` owns the pane below it, and here the widget renders its own table. Rejected: `Segmented` in the `Panel` `extra`, which does not fit five UTM tabs in a half-width card.
- **P2 — the dashboard's metric strip becomes a standalone `KpiStrip`, the chart a `Panel` below it**, as on `AnalyticsPage` (`DashboardKpis` then `OriginVolumeChart`). The panel's title is the selected metric's label and its `extra` holds the `GranularitySelector`, which today floats over the chart (`absolute right-4 top-4`).
- **P3 — the dimension tables stay hand-built** (`<table>` in `DimensionTableWidget`), restyled to the antd `Table` values. An antd `Table` would fight the proportional background bar, the TimeScore heat dot and the click-to-filter row; the values to copy are in `antdTheme.ts` (`Table`) and `antd-overrides.css`.
- **P4 — `MetricChart` height 250 on the dashboard** (from 220), the main dashboard's `CHART_HEIGHT`.
- **P5 — a guard test** keeps web analytics on the tokens once migrated (below, step 7).

## Changes

Mapping applied everywhere in `components/web_analytics/**` and `pages/WebAnalytics*.tsx`: text `gray-900`/`gray-800` → `ink`, `gray-700`/`gray-600` → `ink-2`, `gray-500` → `ink-3`, `gray-400`/`gray-300` → `ink-4`; borders `gray-200` → `line`, `gray-100`/`gray-50` → `line-soft`; backgrounds `gray-50` → `subtle`, `gray-100` → `fill`, `bg-white` → `bg-surface`; `text-[10px]` → `text-overline`; `text-xl` on a KPI → `text-kpi`; `rounded-md border border-gray-200` card shells → `Panel`. antd components lose the Tailwind classes put on them (`<Table className="border border-gray-200 rounded-md">`): the `Panel` around them draws the border, per `console/CLAUDE.md`.

### Step 1 — shared pieces

| File | Change | Test file and cases |
|---|---|---|
| `components/web_analytics/Delta.tsx` | Colour from classes, not `style`: `text-success-fg` when better, `text-danger-fg` when worse (`invertTrend` unchanged: the arrow follows the sign, the colour follows good/bad). Size by token (`text-xs` default, `text-2xs` small) instead of the `size` px prop. Export `deltaTone(change, invertTrend): KpiTone` for `KpiStrip`'s `sub`. | `Delta.test.tsx` (new): renders nothing at 0, `undefined`, `NaN`; up arrow + `text-success-fg` for +12; down arrow + `text-danger-fg` for −12; with `invertTrend`, −12 is `text-success-fg` with a down arrow; `deltaTone` returns `positive` / `negative` / `neutral` for those cases. |
| `components/web_analytics/lib/types.ts` | Remove `POSITIVE_COLOR` / `NEGATIVE_COLOR`; any remaining reader uses `colors.tones.success.fg` / `colors.tones.danger.fg`. | `theme/noHardcodedColors.test.ts`: existing case for `lib/types.ts` still passes. |
| `components/web_analytics/WidgetTabs.tsx` | P1: `gap-4.5 px-3.5`, `border-line-soft`, buttons `py-2 text-base`, `text-ink-3 hover:text-ink-2`, active `text-ink border-primary-500`. Doc comment updated. | `WidgetTabs.test.tsx` (new): renders nothing with one tab; clicking a tab calls `onChange` with its key; the active tab carries `aria-selected="true"` (added, with `role="tablist"` / `role="tab"`). |
| `components/web_analytics/MetricSummary.tsx` | Becomes a thin adapter over `KpiStrip`: one `KpiItem` per metric with `onClick` / `active`, `label` = label + info tooltip icon, `value` = `formatValue(...)`, `sub` = formatted change and `tone` = `deltaTone(...)` when comparing; `loading` passed through. The 2/4-column grid goes (`KpiStrip` wraps by width). | `MetricSummary.test.tsx` (new): one `data-kpi-cell` per metric; clicking a cell calls `onSelect` with its key; the selected cell has `aria-pressed="true"`; with comparison, a rise in sessions shows a `text-success-fg` sub and a rise in bounce rate a `text-danger-fg` sub; without comparison no sub; loading shows the skeleton. |
| `components/web_analytics/MetricChart.tsx` | Every hex → `colors.*` (axis `colors.line`, labels `colors.ink3`, split lines `colors.gray[100]`, tooltip text `colors.ink`/`ink3`, tooltip change `colors.tones.success.fg` / `danger.fg`); axis labels 10.5px, legend 11.5px as `OriginVolumeChart`; skeleton `bg-fill`. | `MetricChart.test.tsx`: add — the option's axis label colour is `colors.ink3` and size 10.5; the tooltip change of an improvement carries `colors.tones.success.fg`, of a deterioration `colors.tones.danger.fg`; existing cases unchanged. |

### Step 2 — widgets as `Panel`s

| File | Change | Test file and cases |
|---|---|---|
| `components/web_analytics/DimensionTableWidget.tsx` | Card shell → `Panel` (`title` = title + info tooltip, `extra` = expand button as antd `Button type="text" size="small"`, `flush`); `WidgetTabs` first child of the body. Table per P3: header row `bg-subtle text-2xs font-medium text-ink-3`, cells `py-1.75`, first column `px-3.5`, others `px-2`; rows `text-sm text-ink border-b border-line-soft hover:bg-subtle`; loading bars `bg-fill`; empty state inside the panel body. The drawer view (`ExpandedRows`) reuses the same table, so it aligns too. | `DimensionTableWidget.test.tsx` (new, `useWebAnalytics` and the query mocked): renders inside a `section` with `data-panel-head` holding the title; switching tab queries the tab's dimension; clicking a column header sorts desc then asc; clicking a row calls `toggleFilter` with `equals` and the row's value; the empty text shows with no rows; the expand button opens the drawer, absent for the map tab; with comparison a cell shows a `Delta`. |
| `components/web_analytics/TrafficHeatmapWidget.tsx` | Shell → `Panel`; hex → tokens (`colors.line`, `colors.ink2`, `colors.ink3`, ramp floor `colors.fill`); labels 10.5px. | `TrafficHeatmapWidget.test.tsx` (new): title in the panel head; the option's axis label colour is `colors.ink3`; clicking a cell applies the day/hour filter (current behaviour). |
| `components/web_analytics/CountryMapView.tsx` | `areaColor` → `colors.subtle`, `borderColor` → `colors.line`, label 10.5px. Already in `MIGRATED`. | `theme/noHardcodedColors.test.ts` (step 7) covers it; no behaviour change. |
| `components/web_analytics/LiveButton.tsx` | `border-line bg-surface text-ink-2`, value `text-ink`, dot in the success token instead of `green-400/500`. | `LiveButton.test.tsx` (new): shows the live count; links to the live page. |

### Step 3 — Dashboard tab and page layout

| File | Change | Test file and cases |
|---|---|---|
| `components/web_analytics/tabs/DashboardTab.tsx` | Root `flex flex-col gap-3`; alert without `mb-4`; P2: `MetricSummary` (standalone strip) then a `Panel` titled with the selected metric, `extra` = `GranularitySelector`, body = `MetricChart` at `height={250}`; the four rows of widgets in `grid grid-cols-1 gap-3 md:grid-cols-2`. | `tabs/DashboardTab.test.tsx` (new, queries mocked): a `data-kpi-strip` with four cells; clicking Bounce rate retitles the chart panel and feeds `bounce_rate` to the series; the granularity selector sits in the chart panel's head; a series error shows the alert. |
| `pages/WebAnalyticsPage.tsx` | The body fragment becomes a `flex flex-col gap-3` column: no `mb-2` / `mb-4` on the dimension and filter bars. | `pages/WebAnalyticsPage.test.tsx` (new, tabs mocked): the dashboard tab shows the live button in the header; Explore puts the period controls in the header; other tabs put them beside the filter bar. |
| `pages/WebAnalyticsLivePage.tsx` | `LiveSessionMap` inside a `Panel flush`; the two grids `gap-3` without `mt-4`. | `pages/WebAnalyticsLivePage.test.tsx` (new): map panel and the five dimension widgets render. |

### Step 4 — Goals

| File | Change | Test file and cases |
|---|---|---|
| `components/web_analytics/goals/GoalCard.tsx` | `Panel` (`title` = goal name, `extra` = the eye button); the four tiles → `KpiStrip embedded` with `onClick` / `active` and the change as `sub` + `tone`; `MiniChart` in the body. Values go from centered `text-base` to the strip's left-aligned `text-kpi`. | `goals/GoalCard.test.tsx` (new): title and four cells; clicking Conversion rate selects it and feeds the mini chart; the eye button calls `onOpenDashboard`; currency 0 shows "-". |
| `components/web_analytics/goals/GoalDashboardDrawer.tsx` | Same layout as the dashboard tab (strip + chart `Panel`), grids `gap-3` without `mt-4`. | `goals/GoalDashboardDrawer.test.tsx` (new): strip and chart panel render for the goal; switching metric retitles the chart panel. |
| `components/web_analytics/goals/MiniChart.tsx` | Colours from `colors.*` if any hex; no layout change. | Covered by `GoalCard.test.tsx`. |
| `components/web_analytics/tabs/GoalsTab.tsx` | Grids `gap-3`; loading and empty shells → `Panel`. | `tabs/GoalsTab.test.tsx` (new): one card per goal; the empty state when there is none. |

### Step 5 — Explore

| File | Change | Test file and cases |
|---|---|---|
| `components/web_analytics/explore/ExploreSummary.tsx` | → `KpiStrip` (not clickable); "Best TimeScore" as a cell with `hint` (its current tooltip); the change as `sub` + `tone`. | `explore/ExploreSummary.test.tsx` (new): one cell per measure plus Best TimeScore; its hint shows on hover; "—" with no rows; the change's tone follows `invertTrend`. |
| `components/web_analytics/explore/ExploreTable.tsx` | The antd `Table` in a `Panel flush`, its `className` and the wrapping `rounded-md` div removed. | `explore/ExploreTable.test.tsx` (new): rows render; sorting by a measure; a comparison cell shows a `Delta`. |
| `components/web_analytics/explore/DimensionSelector.tsx`, `BreakdownModal.tsx` | Mapping above only (borders and text tokens). | Existing behaviour; `DimensionSelector.test.tsx` (new): adding and removing a dimension calls `onChange`. |

### Step 6 — Filters, Annotations, install overlay

| File | Change | Test file and cases |
|---|---|---|
| `components/web_analytics/filters/FilterTable.tsx` | The intro box (`rounded-lg border bg-white p-4`) → `Panel`; the antd `Table` in a `Panel flush` without its `className`. | `filters/FilterTable.test.tsx` (new): rows, the edit and delete actions, the backfill status. |
| `components/web_analytics/annotations/AnnotationTable.tsx` | The three shells (`rounded-lg bg-white p-6`, `… border p-4`, `… shadow-sm`) → `Panel`s; the table `flush`. | `tabs/AnnotationsTab.test.tsx`: existing cases still pass; add — the table renders inside a panel. |
| `components/web_analytics/InstallOverlay.tsx` | `bg-surface border-line rounded-lg shadow-popup`, title `text-subtitle`, overlay `bg-surface/60`. | `InstallOverlay.test.tsx`: existing cases still pass. |
| `components/web_analytics/FilterBuilder.tsx`, `toolbar.tsx`, `SdkVersionWarning.tsx`, the remaining files of the 29 | Mapping above only. | `FilterBuilder.test.tsx`, `toolbar.test.tsx`: existing cases still pass. |

### Step 7 — guard

`theme/noHardcodedColors.test.ts`, new `describe('web analytics on the tokens')`: for every non-test `.ts`/`.tsx` under `components/web_analytics/` and `pages/WebAnalytics*.tsx`, no `(text|bg|border)-gray-\d`, no `bg-white`, no `text-xl`, no `#[0-9a-f]{6}` / `#[0-9a-f]{3}\b` colour literal. A failing file is named in the message. (`worldGeo.json` is not scanned: not `.ts`.)

## Out of scope

- The AI assistant panel (`WebAnalyticsAIAssistant.tsx`): a floating chat, its own layout.
- Behaviour: no query, filter or URL changes. A refactor that changes what a widget fetches is a bug in this work.

## CHANGELOG

One **Improvement** line in the `44.0` WIP section, after the console feature bullets: "The web analytics pages take the console's new look: white cards, the same KPI strip as the dashboard, tighter spacing and tables like the rest of the console. A change against the previous period is red when it is worse, instead of orange."

## Translations

No new string expected (the chart panel's title reuses the metric labels). Run `npm run lingui:extract` and confirm the `.po` files do not change; if they do, translate the new strings in every locale.

## Verification

1. `cd console && npx vitest run src/components/web_analytics src/pages src/theme src/components/layout`
2. `cd console && npx tsc --noEmit -p tsconfig.app.json && npm run lint`
3. `cd console && npm test` (full suite; mind the CI timing note: a test above ~1.9s locally can time out in CI).
4. In a browser, on the regenerated demo, before/after screenshots side by side of: the main dashboard, the web analytics dashboard, Explore, Goals (card + drawer), Live, Filters, Annotations. The Chrome extension could not attach to `https://notifusedev.com:5173` on 2026-11-14 (self-signed certificate); accept the certificate first or use headless Chrome.
5. `/code-review` on the final diff before any commit.
