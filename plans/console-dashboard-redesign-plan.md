# Console dashboard redesign (Nuit claire, screen 131) — plan

Spec: « Spec 3 · Tableau de bord » (Claude Docs, decisions of 12 Nov 2026). Server parts already in `dev`
(#524: `origin` dimension of `message_history`, partial failure indexes in v44). This PR is console only:
no SQL, no migration, no environment variable, no backend file.

Shared layout components come from #531 (`PageHeader`, `Panel`, `KpiStrip`, `Pill`, `useNumberFormat`); they
are reused, not redefined. Page padding comes from `WorkspaceLayout` (`px-5 pt-4 pb-6`); the page root is
`flex flex-col gap-3`. Headings or lists styled inside a flex row carry `data-unreset`.

## Decisions already taken (Pierre, 12 Nov)

- Periods 7 d / 30 d / 90 d end yesterday (`previous_N_days` presets of `web_analytics/lib/dates.ts`), and the
  previous period abuts them (`computeComparisonRange(range, 'previous_period')`).
- The page follows the workspace timezone (`workspace.settings.timezone`, `UTC` when absent); the timezone
  selector and the 14 d period go away.
- The Sending card keeps provider and default sender of each use, without status nor rate gauge.
- Links into screens not yet redesigned use today's URLs and switch at rebase when their PR lands:
  contact sheet `?contact={email}` (spec 4, confirmed by the Contacts thread; meanwhile `?email={email}`),
  broadcast detail `?broadcast={id}` (spec 5, confirmed by the Newsletters thread; `BroadcastsSearch` is
  `{ status?, q? }` today and a typed `Link` with `broadcast` does not compile, so rows link to
  `/broadcasts?status=scheduled` until that PR lands, then switch at rebase), message detail `/logs?tab=messages&id={id}&message={id}` (spec 8, confirmed by the
  Logs thread; `id` filters today's Logs, `message` opens the panel once spec 8 lands).

## URL

`validateDashboardSearch(search) → { period?: '7d' | '90d'; metric?: 'opens' | 'clicks' }` in
`components/analytics/dashboardSearch.ts`. `30d` and `sent` are the defaults and are written as `undefined`,
so a link without `search` stays typed and the URL stays clean. An unknown value, an array or a non-string
gives `undefined`. `router.tsx` imports it on the workspace index route and on `/analytics` (two lines each,
nothing else in the router changes: other threads edit that file in parallel). The page reads with
`useSearch({ strict: false })` and writes with `useNavigate()({ search: (prev) => …, replace: false })`.

## Periods — `components/analytics/dashboardPeriods.ts`

`resolveDashboardPeriods(period: '7d'|'30d'|'90d', timezone: string)` returns
`{ days, preset, current: ResolvedRange, previous: ResolvedRange }` from `computeDateRange(preset, tz)` with
`preset` a typed map `{ '7d': 'previous_7_days', … } satisfies Record<…, DatePreset>`,
`computeComparisonRange(range, 'previous_period')` (non-null for this mode, guarded) and `resolveRange`. Empty
timezone → `UTC`.

## Queries (all through `analyticsService.query`, `timezone` = workspace timezone)

| Code | Query | Key | Refresh | Condition |
|---|---|---|---|---|
| Q1 ×3 | `message_history`, measures `count_sent, count_delivered, count_bounced, count_complained, count_opened, count_clicked, count_opens_tracked, count_clicks_tracked`, `timeDimensions [{created_at, day, [startDay,endDay]}]`, filters `origin equals <o>`, `channel equals email` | `['dashboard','series',ws,tz,startDay,endDay,origin]` | 60 s, `placeholderData: keepPreviousData` | `message_history.read` |
| Q2 | same schema, measures opened, clicked, opens_tracked, clicks_tracked, filters `created_at inDateRange [prev.startUtc, prev.endUtc]`, `channel equals email` | `['dashboard','previous-rates',ws,tz,prevStartUtc,prevEndUtc]` | `staleTime` 1 h | idem |
| Q3 | `contacts`, `count`, no filter | `['dashboard','contacts-total',ws]` | 60 s | `contacts.read` |
| Q4 | `contacts`, `count`, `created_at inDateRange [startUtc,endUtc]` | `['dashboard','contacts-new',ws,tz,startUtc,endUtc]` | 60 s | idem |
| Q5 ×2 | `web_sessions`, `sessions`, `created_at inDateRange` (the schema's time dimension, `SCHEMA_TIME_DIMENSION`) current, then previous, built with `buildWebQuery` so the figure equals the web analytics KPI | `['dashboard','visits',ws,tz,startUtc,endUtc]` | current 60 s, previous `staleTime` 1 h | `web_analytics.read` and `workspace.settings.web_analytics` set |
| F | `messages.list` ×3 (`is_failed`, `is_bounced`, `is_complained`, `limit 5`) | `['dashboard','failures',ws,kind]` | 60 s | `message_history.read` |
| C | `contacts.list` `limit 5` | `['dashboard','new-contacts',ws]` | 60 s | `contacts.read` |
| U | `broadcasts.list` `statuses: ['scheduled'], limit: 100` | `['dashboard','upcoming',ws]` | 60 s | `broadcasts.read` |
| L | `lists.list` | `['lists', ws]` (existing key) | default | `lists.read`, only when U has rows |

All dashboard analytics queries go through their own client, `AnalyticsService.create({ maxConcurrency: 4,
cacheTTL: 30_000 })` (the shared singleton runs one request at a time, so blocks would wait for each other;
the cache stays under the 60 s refresh). Bounds are recomputed from `useMinuteTick()` so the query keys move
at midnight. Every response is read defensively (`data ?? []`, numbers through `toNumber`).

One request per origin (never `origin` as a dimension with a time dimension): the gap filler keeps one row per
day (`fillTimeSeriesGaps`). Current totals are the sum of the three series, computed in the page. Switching
Sent / Opens / Clicks re-renders, never re-queries.

## KPIs — `components/analytics/dashboardKpis.ts` (pure)

`computeDashboardKpis({ series, previousRates, contactsTotal, contactsNew, visits, previousVisits })` returns
plain numbers and tones; formatting stays in the component (`useNumberFormat`).

- Contacts: Q3; sub `+Q4 over {n} d` (plural), `positive` when Q4 > 0 else `neutral`.
- Emails sent: Σ `count_sent`; sub `{part} broadcasts` with part = broadcast sent / total; `neutral`.
- Delivered: delivered / sent; sub `Bounces {b} · Complaints {c}` on sent; `warning` when bounces > 2 % or
  complaints > 0.1 %; permanent hint « Counted from your providers' delivery webhooks »; delivered = 0 with
  sent > 0 → value « — », sub « No delivery events ».
- Open rate: opened / opens_tracked; delta in points vs Q2 rate; `positive` > +0.05 pt, `negative` < −0.05 pt;
  hint = partial tracking (`isPartiallyTracked`, « of {tracked} tracked », from `EmailMetricsChart`) and
  « Recent emails may still be opened ».
- Click rate: same with clicks.
- Website visits: Q5 `compact`; relative delta vs previous; tones taken from the value rounded to one decimal
  of a percent (`positive` > 0, `negative` < 0, `neutral` at 0.0 or without previous), so « +0.0 % » is never
  green.
- « — » for a rate with denominator 0 and for a delta whose previous period has no denominator.

## Components (`components/analytics/`)

- `DashboardKpis.tsx`: builds the `KpiStrip` items from the hook data and rights. Contacts KPI is a link to
  Contacts; Website visits links to `/web-analytics/dashboard?period=previous_{N}_days&comparison=previous_period`.
  `KpiStrip`'s `onClick` makes a toggle button (`aria-pressed`), wrong for a link: the cell value is wrapped in
  a router `Link` inside `label`/`value` instead, so `KpiStrip` is not modified. Without web analytics
  settings: `—` and sub link « Set up web analytics » (to `/settings/web-analytics`) for a `workspace` writer,
  hidden otherwise. Without `contacts`: no Contacts KPI; without `web_analytics`: no visits KPI.
- `OriginVolumeChart.tsx`: `Panel` « Emails sent per day », `extra` = `Segmented` Sent / Opens / Clicks.
  ECharts: broadcasts as bars (`colors.chart.primary`), transactional and automations as lines
  (`series2`, `series3`), legend top-left, axis tooltip with the three values (Opens/Clicks add « of emails
  sent that day »). Log axis when max > 10 × median of non-zero daily values (zeros → `null`), linear otherwise;
  axis labels `compact`. Empty period (all zero) → antd `Empty` « No email sent in this period ».
  The chart helpers (`buildSeries`, `useLogScale`) live in `originChart.ts` (pure, tested).
- `RecentFailures.tsx`: three lists merged (`mergeFailures` in `recentFailures.ts`, pure): event time
  = `complained_at` / `bounced_at` / `failed_at` per list, dedup by message id with priority Complaint >
  Bounce > Failed, sorted desc, 5 rows. Row: time (HH:mm today, else short date, both with
  `Intl.DateTimeFormat` in the console locale and the workspace tz); recipient
  and reason under it (`status_info`, else « Marked as spam » / « Bounced » / « Failed »); `Pill` Complaint
  `danger`, Bounce `warning`, Failed `danger`. `extra` « View in logs » → Logs `is_failed=true` (as today).
  No failure → « No failure » with `CircleCheck` in `text-success-fg`.
- `NewContactsTable.tsx` (rewritten): 5 rows, age (`4 min`, `3 h`, `2 d`: `Intl.NumberFormat` unit style
  `short`, the mockup's form; `Intl.RelativeTimeFormat` would add « ago »), email, name (first + last, else
  nothing). `extra` « All contacts ». Row click → contact sheet. Empty → `Empty` « No contact yet » with an
  « Import contacts » button that opens the import drawer (`useContactsCsvUpload().openDrawer`, provided by
  `WorkspaceLayout`), shown with `contacts.write` only.
- `SendingCard.tsx`: rows Marketing and Transactional: provider name (`emailProviders` by kind) and default
  sender (`is_default`, else first). No provider → « Not configured » and, for a `workspace` writer, link
  « Configure » to `/settings/integrations`.
- `UpcomingSends.tsx`: scheduled broadcasts sorted by `scheduleInstant(schedule, workspaceTz)`
  (`upcomingSends.ts`, pure: date + time read in `schedule.timezone`, in UTC when it is empty as the server
  does (`domain/broadcast.go:237-241`), or in the workspace tz when `use_recipient_timezone`), first 5: when
  (`Intl.DateTimeFormat(locale, { timeZone, day, month: 'short', hour, minute })`, no dayjs locale being loaded,
  then the tz abbreviation from `timeZoneName: 'short'` when it differs from the workspace's, or « local time »
  for recipient timezone), name, list name (with `lists`, else « 1 list »), « A/B »
  when `test_settings.enabled`. Footer « +{n} more » (plural) from `total_count − 5` linking to
  `/broadcasts?status=scheduled`. `extra` « Broadcasts ». Empty → « No broadcast scheduled » and, with
  `broadcasts.write`, a « New broadcast » button to `/broadcasts`. Row → `/broadcasts?status=scheduled` until `?broadcast={id}` exists (see Decisions).
- Every block: `Skeleton` while loading; on error « Could not load » + « Retry » (`refetch`), error logged
  with `console.warn` (the e2e protected-pages test fails on any `console.error`); no global message. Tables: antd `Table size="small" showHeader={false}` in
  `Panel flush`, clickable rows (`cursor-pointer`), keyboard: rows get `tabIndex=0` and Enter opens.

## Page — `pages/AnalyticsPage.tsx`

`PageHeader` « Dashboard » with `Segmented` 7 d / 30 d / 90 d. Permissions from `useWorkspacePermissions`;
while they load, KPI strip and panels show skeletons. Rights are read strictly (`.read`): `message_history`
write is unenforced and granted to every member saved by the console (`UNENFORCED_PERMISSIONS`), so the
sidebar's read-or-write `hasAccess` shows Dashboard to members who cannot read messages. Without
`message_history.read`, the page redirects (`<Navigate replace>`) to the first sidebar entry the member can
reach other than the dashboard, or Settings when none: `firstAccessiblePage(hasAccess, workspaceId)` added to
`layouts/app-shell/menuItems.ts` (shared file, small addition, flagged in the PR), which skips the
`analytics` entry and follows `buildMenuItems`' order, so the redirect can never point at itself. Layout: KPI strip; grid
`grid-cols-1 min-[1200px]:grid-cols-[minmax(0,1fr)_var(--spacing-aside)]` (or the token width the spec 2
detail composition uses) `gap-3 items-start`; main column = chart then
`grid grid-cols-1 min-[1000px]:grid-cols-2 gap-3` with failures and new contacts; side = Sending then Upcoming.

## Removed

`AnalyticsDashboard.tsx`, `ChartVisualization.tsx`, `EmailMetricsChart.tsx` (+ test), `FailedMessagesTable.tsx`
(+ test), `index.ts` (nothing else imports these; `tsc -b` checks). `TrackedTooltipTitle` stays (used elsewhere,
checked with grep before deleting anything).

## Tests (TDD, written first)

| Implementation | Test | Cases |
|---|---|---|
| `components/analytics/dashboardSearch.ts` | `dashboardSearch.test.ts` | `7d`/`90d` kept; `30d`, unknown, array, number → undefined; `opens`/`clicks` kept; `sent`/unknown → undefined |
| `dashboardPeriods.ts` | `dashboardPeriods.test.ts` | fake time 11 Nov 2026 10:00 Paris: 30 d = 12 Oct → 10 Nov, previous 12 Sep → 11 Oct, days and UTC instants; 7 d and 90 d lengths; DST end (25 Oct) inside the window: no day lost (30 distinct days); empty tz → UTC |
| `dashboardKpis.ts` | `dashboardKpis.test.ts` | rate with zero denominator → null; delta in points and tones at ±0.05; broadcasts share; bounce and complaint tones at 2 % / 0.1 %; « No delivery events »; visits delta with empty previous → null; partial tracking flag and tracked count; totals sum three series |
| `originChart.ts` | `originChart.test.ts` | log scale above ratio 10 with zeros → null; linear otherwise; Opens/Clicks pick the right measure; all-zero → empty |
| `recentFailures.ts` | `recentFailures.test.ts` | merge sorted by event time; ≤ 5; duplicate message keeps Complaint over Bounce over Failed; fallback reason without `status_info` |
| `upcomingSends.ts` | `upcomingSends.test.ts` | sort by instant in each schedule's tz (a 09:00 New York send after a 10:00 Paris one on the same day); recipient timezone read in workspace tz; tz abbreviation only when different |
| `DashboardKpis.tsx` | `DashboardKpis.test.tsx` | no `contacts` → contacts query not called and KPI absent; web analytics unset → setup link for workspace writer, KPI hidden otherwise; partial-tracking hint text; visits link carries the period; query key carries bounds and tz |
| `OriginVolumeChart.tsx` | `OriginVolumeChart.test.tsx` (echarts-for-react mocked to capture `option`) | three queries each with its `origin` filter and `channel = email`; metric switch does not query again; empty period → Empty |
| `RecentFailures.tsx` | `RecentFailures.test.tsx` | three `listMessages` calls with the right flag and limit 5; rows and pills; « No failure »; error → « Could not load » + Retry refetches; row link targets the message |
| `NewContactsTable.tsx` | `NewContactsTable.test.tsx` | full name; no name; row click navigates to the contact; empty state |
| `SendingCard.tsx` | `SendingCard.test.tsx` | provider and default sender; « Not configured »; « Configure » only for workspace writer |
| `UpcomingSends.tsx` | `UpcomingSends.test.tsx` | sorted rows; « +n more » from `total_count`; « 1 list » without `lists`; empty with and without `broadcasts.write` |
| `layouts/app-shell/menuItems.ts` (`firstAccessiblePage`) | `menuItems.test.ts` (extended) | first reachable non-dashboard entry follows `buildMenuItems` order for several permission sets; `{ message_history: { read: false, write: true } }` never returns the dashboard; none → settings |
| `pages/AnalyticsPage.tsx` | `pages/AnalyticsPage.test.tsx` (new) | 30 d by default; `?period=7d` read; changing period writes the URL; no `message_history.read` (incl. write-only) → redirect to first accessible page; skeletons while permissions load |
| `src/__tests__/pages.smoke.test.tsx` | existing | page still renders with mocked data (mocks updated) |
| `e2e/protected-pages.spec.ts` | e2e | `Total Contacts`, `Email Metrics`, `Recent New Contacts` assertions replaced by the new titles |
| `e2e/fixtures/auth.ts` | e2e | `analytics.query` answers per request (rows keyed by the requested measures, plus `created_at_day` when a time dimension is asked); `messages.list` returns `{ messages: [], has_more: false }` |

## CHANGELOG

Under the unreleased 44.0, one Feature line (customer-facing, short): « The dashboard shows website visits, how
rates moved since the previous period, emails sent each day by origin (broadcasts, transactional,
automations) and your upcoming broadcasts. » The server line for `origin` is already there (#524).

## Translations

`npm run lingui:extract` after rebase on `dev`, then the new strings translated in every locale catalogue
(`catalogues.test.ts` requires them), plurals included.

## Verification

1. `cd console && npx vitest run src/components/analytics src/pages/AnalyticsPage.test.tsx src/layouts/app-shell`
2. `npx tsc -b`, `npm run lint`, `npx vitest run` (full suite once at the end), `npm run build`.
3. `npx playwright test e2e/protected-pages.spec.ts` and the sidebar tests that start on the dashboard
   (`contacts`, `broadcasts`, `lists`, `templates`, `blog`, `segments`, `transactional` specs) `--trace off`.
4. Manual checks on mocked data in the browser: member without `message_history.read` (redirect, incl.
   write-only), member without `contacts`, workspace without web analytics, empty workspace; KPI numbers
   cross-checked against the old chart on the same period before it is deleted.
5. `EXPLAIN (ANALYZE, BUFFERS)` of the three daily series and the three failure lists on a local PostgreSQL 16
   with a synthetic `message_history` of 5 million rows (no demo database is reachable from here), SQL taken from
   the analytics builder; timings noted in the PR.
6. Screenshots of the dashboard at 1440 and 1280 px (dev server on port 19173 with API mocks, killed after),
   compared to capture 131.

## Spec review (Opus, effort xhigh) — integrated

Confirmed and integrated: redirect loop for write-only `message_history` members (#1), `?broadcast` link not
typed yet (#2), `web_sessions` time column is `created_at` (#3), e2e assertions and mock shapes (#4), dates
through `Intl` (#5), own analytics client (#6), minute tick for midnight (#7), empty schedule timezone = UTC
(#8), spec manual checks (#9), visits tone from the rounded value (#10), import drawer (#11), typed presets and
nullable comparison range (#22).
