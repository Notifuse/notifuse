# Console transactional redesign (Nuit claire, screens 91 and 92) — plan

Spec: « Spec 7 · Transactionnel » (Claude Docs, rev. 15, decisions of 12 Nov 2026: actions without the right
shown disabled with a tooltip; this screen's period includes today). Server parts already in `dev` (#524:
`messages.list` filter `transactional_notification_id`, `last_sent_at` measure). This PR is console only: no
SQL, no migration, no environment variable, no backend file.

Reused from `dev` (#531): `PageHeader`, `Panel`, `KpiStrip`, `Pill`, `RateBar`, `FullDrawer` (+ `DrawerNav`,
J/K keys), `KeyValueList`, `TableToolbar` (+ `TableToolbar.Search`), `Mono`, `useNumberFormat`,
`lib/copyToClipboard.ts`. Not redefined.

Coordination with the threads working in parallel:

- `components/templates/compileTemplatePreview.ts` comes from the Newsletters thread's own small PR
  (`feat/compile-template-preview`: `buildTemplatePreviewRequest`, `compileTemplatePreview(input)` rejecting
  with `TemplatePreviewError`). This PR rebases on it once merged. Fallback, as spec 7 says: if that PR is not
  in `dev` when this one is otherwise ready, this PR delivers the file itself with that exact signature and
  its tests, and the Newsletters thread is told.
- `lib/messageStatus.ts` is delivered here and reused by the Logs thread (`messageStatus(message) → { key, tone }`).
- `services/api/messages_history.ts` gains the `transactional_notification_id` parameter, and
  `MessageHistoryTab` gains it in `filterKeys` and in the second filter group (« Transactional notification
  ID »), as spec 7 says, so links from this screen arrive filtered today. Two small lines in a file the Logs
  thread rewrites; it is told so it keeps them.
- Message detail link: `/logs?tab=messages&id={messageId}&message={messageId}` (Logs thread's contract).

## URL

`validateTransactionalSearch(search) → { q?: string; period?: '7d' | '60d'; notification?: string }` in
`components/transactional/transactionalSearch.ts`, exported and tested; `30d` (default), unknown values,
arrays and non-strings → `undefined`; `q` trimmed, empty dropped. `router.tsx` imports it on the
`/transactional-notifications` route (one line plus the import). Search writes `q` with `replace` 250 ms after
the last keystroke; `period` and `notification` write with `push`. Query keys never contain `notification`.

The search box keeps a local mirror of what is typed and writes the trimmed `q` 250 ms later, ignoring the
echo of its own navigation (`TemplatesPage.tsx:66-115` pattern), so trailing spaces are not eaten while typing.

## Overlays

`ApiCommandModal`, `SendTemplateModal`, `TemplatePreviewDrawer` (controlled) and the edit drawer are mounted
only while open (`{open && …}`), so each opening appends a fresh portal at the end of `body`: one opened from
the sheet always lies after the `FullDrawer` in the DOM, renders above it, and `useDrawerNavKeys`' topmost
rule turns J/K off while it is open.

## Rights while loading

While `useWorkspacePermissions` loads, KPIs, statistic cells and actions render as skeletons and no analytics
query starts; no « without the right » state is shown before the rights are known. Rights are read strictly
(`.read` / `.write`), never through the sidebar's read-or-write test.

## Analytics client and relative times

The page's analytics queries use their own `AnalyticsService.create({ maxConcurrency: 4, cacheTTL: 30_000 })`.
Relative times use `Intl.RelativeTimeFormat(locale, { numeric: 'auto', style: 'short' })` under a day,
« Yesterday, {time} » through Lingui with `Intl.DateTimeFormat` for the time, then a short date and time;
re-rendered on `useMinuteTick()`. No dayjs locale is loaded in the console, so dayjs is not used for text.
Responses are read defensively (`?? []`, numeric conversion).

## Period — `components/transactional/period.ts`

`resolveTransactionalPeriod(period, tz)` → `{ days, current: ResolvedRange, previous: ResolvedRange }`:
`computeDateRange('custom', tz, { start: today − (N − 1), end: today })` then `resolveRange`;
previous = `computeComparisonRange(range, 'previous_period')` (nullable type, guarded). `DATE_PRESETS` is not
touched.

## Queries

As the spec's table (T1 list, T2 stats per notification over the window with `transactional_notification_id
in [sorted ids]` and `created_at inDateRange [startUtc,endUtc]`, T3 previous sent/delivered, T4 `last_sent_at`
over 60 days, T5 `templates.get` per distinct template id via `useQueries`, F1 `transactional.get`, F3 daily
series of one notification with `dateRange [startDay,endDay]`, F4 `messages.list` limit 10 refetched every
30 s while visible, F5 template compile through `compileTemplatePreview`). Measures M = `count_sent,
count_delivered, count_failed, count_bounced, count_opened, count_opens_tracked, count_clicked,
count_clicks_tracked`. Every analytics query passes `timezone`. Without `message_history.read`, T2, T3, T4,
F3 and F4 are not requested.

## Files

| Implementation | Test | Cases |
|---|---|---|
| `components/transactional/transactionalSearch.ts` (new) | `transactionalSearch.test.ts` | `30d`/unknown → undefined; `7d`,`60d` kept; `q` trimmed, empty dropped; array/number → string or dropped |
| `components/transactional/period.ts` (new) | `period.test.ts` | 7/30/60 days in Paris (+) and New York (−), today included; `startUtc`/`endUtc`; previous same length, no overlap; DST change inside window |
| `components/transactional/transactionalStats.ts` (new, pure) | `transactionalStats.test.ts` | row lookup with missing row → zeros; totals = sum of rows of the listed notifications; rates null on zero denominator; failed share on sent + failed; delivered delta in points; bounce warning ≥ 2 %; opens tone `partial` when tracked < sent; « Not tracked » when 0; peak failure day from F3 rows |
| `components/transactional/senderInfo.ts` (new, pure) | `senderInfo.test.ts` | template's default-language `sender_id` found among provider senders; else provider default; none → null; without template → provider default flagged `(default)`; reply-to |
| `lib/messageStatus.ts` (new) | `lib/messageStatus.test.ts` | each status incl. Unsubscribed; order when several dates set; no delivery date → Sent |
| `lib/transactionalApiSnippets.ts` (new, generators moved out of `ApiCommandModal`) | `transactionalApiSnippets.test.ts` | each generator contains the notification id, workspace id and `window.API_ENDPOINT`; output byte-identical to the modal's before the move (snapshot captured from the current modal first) |
| `components/transactional/ApiCommandModal.tsx` | `ApiCommandModal.test.tsx` (new) | six tabs with `SMTP_BRIDGE_ENABLED`, five without; copy uses shared generator |
| `components/transactional/UpsertTransactionalNotificationDrawer.tsx` | existing test, extended | current cases unchanged; `open` prop opens with form filled, `onClose` called; F1 key invalidated after save |
| `components/templates/TemplatePreviewDrawer.tsx` | existing test, extended | controlled `open`/`onClose`; child mode unchanged |
| `components/transactional/SendsPerDayChart.tsx` (new) | `SendsPerDayChart.test.tsx` (echarts mocked) | delivered + bounced/failed stacked; SMTP → sent + failed; server keys read as the first 10 chars, no tz shift; today's bar tooltip « Today, so far »; « No sends in this period » |
| `components/transactional/RecentSends.tsx` (new) | `RecentSends.test.tsx` | columns; pill from `messageStatus`; row link to the message; 30 s refetch interval; no `message_history.read` sentence |
| `components/transactional/ApiCallPanel.tsx` (new) | `ApiCallPanel.test.tsx` | cURL/TypeScript/Python segments; Copy copies the shown code; « All languages » opens the modal |
| `components/transactional/TransactionalDetail.tsx` (new) | `TransactionalDetail.test.tsx` | loads by `transactional.get`, list row as `placeholderData`; not found; J/K `replace` and disabled outside the list; J/K ignored while a real antd `Modal` is open over the sheet; body keyed by id; KPI cells link (router `Link` in the value, no toggle button) to logs with the right flag (`is_failed=false` for Sent); SMTP: Delivered and Bounced « — », not links; no `message_history.read`: no KPIs, no chart, F3/F4 not requested, View in logs disabled; failure peak; sender cases; tracking settings three modes, empty mode = inherit; empty `utm_content` shows template id; integration gone → « Managed by an integration »; delete closes the sheet |
| `components/transactional/PreviewPanel.tsx` (new) | `PreviewPanel.test.tsx` | compiles through `compileTemplatePreview` with the template's test data; subject + preview line; no `templates.read` → « You need access to templates »; 404 → « Template not found »; deleted template → warning pill; « Open » opens the preview drawer |
| `components/transactional/TransactionalTable.tsx` (new) | `TransactionalTable.test.tsx` | row click opens with `push`; Enter on a focused row opens; action buttons do not open the sheet; disabled actions carry their tooltip; footer « {n} notifications » / « {n} of {total} notifications » |
| `pages/TransactionalNotificationsPage.tsx` (rewritten) | `TransactionalNotificationsPage.test.tsx` (rewritten) | permission cases now assert disabled buttons with tooltip; drawer mock with `open`; router mock with `useSearch`/`useNavigate`; search by name and id; « No notifications match » + « Clear search »; KPIs = sum of rows; SMTP → « — »; « Not tracked »; 97 % and 2 % thresholds; Last sent and its « — » (missing row, null); deleted and missing template; no-provider alert; empty page without header button; T1 error → `Result` with Retry; T2 error → « — »; no `message_history.read` → no analytics query; skeletons while rights load |
| `components/messages/MessageHistoryTab.tsx` | `MessageHistoryTab.test.tsx` (extended) | `transactional_notification_id` read from the URL, shown as a filter, removable |
| `services/api/messages_history.ts` | `messages_history` test (new or extended) | `transactional_notification_id` appended to the query string |
| `router.tsx` | via `transactionalSearch.test.ts` | — |
| `src/__tests__/pages.smoke.test.tsx` | existing | mock targets `../services/api/transactional_notifications` |
| `e2e/fixtures/auth.ts` | — | `transactional.get` returns the notification of the requested `id`; `analytics.query` answers per request (measures as keys, `created_at_day` with a time dimension) and `messages.list` returns `{ messages, has_more }` (shared with the dashboard PR, whichever lands first) |
| `e2e/features/transactional.spec.ts` | e2e | `cardAction` (`.ant-card-head` + FontAwesome icons) replaced by the row « … » menu and row buttons in the five edit tests and the API-command test; « New notification »; open the sheet by click and by URL (second notification); API command opened from a row, closed, then « All languages » from the sheet shows the modal above it and J does nothing |
| `e2e/protected-pages.spec.ts` | e2e | the button looked for becomes « New notification » |

Removed: the page's internal `TransactionalNotificationCard` and `TemplatePreview`.

Relative times follow « Analytics client and relative times » above (`Intl`, no dayjs locale); if the Logs
thread's `formatRelativeDate` lands in `components/layout/format.ts` first, it is reused instead.

## Documentation (separate PR to `dev` on Notifuse/docs, branch `docs/console-transactional`)

- `features/transactional-api.mdx`: the screenshot under the title is replaced by the new list, and a short
  « Monitoring in the console » section describes the list, the detail view and the API call in three languages.
- `features/logs.mdx`: the filter list gains « Transactional notification: filter by the notification that sent
  the message ».

## CHANGELOG

Under the unreleased 44.0, one Feature line: « The transactional page shows each notification's open and
click rates and its last send, and opens a detail view with sends per day, the latest sends and the API call
in cURL, TypeScript and Python. » (the server line for the filter is already there, #524).

## Translations

`npm run lingui:extract` after rebase on `dev`, every new string translated in all catalogues.

## Verification

1. `cd console && npx vitest run src/components/transactional src/lib src/pages/TransactionalNotificationsPage.test.tsx src/components/templates`
2. `npx tsc -b`, `npm run lint`, full `npx vitest run` once at the end, `npm run build`.
3. `npx playwright test e2e/features/transactional.spec.ts e2e/protected-pages.spec.ts --trace off`.
4. `EXPLAIN (ANALYZE, BUFFERS)` of T2, T4 and F4 on a local PostgreSQL 16 with a synthetic `message_history`
   of 5 million messages, one notification with 1 million and one with 10 (no demo database is reachable from
   here); SQL taken from the analytics builder; timings in the PR, and an index proposed to Pierre (not made)
   if one exceeds 2 s.
5. Screenshots of the list and the sheet at 1440 and 1280 px (dev server on port 19173, killed after).

## Spec review (Opus, effort xhigh) — integrated

Confirmed and integrated: Logs filter lines kept in this PR (#12), overlays mounted only while open (#13),
relative times with `Intl` (#14), shaped e2e mocks and the rewritten e2e tests (#15), docs PR and EXPLAIN step
(#16), missing test cases incl. a `TransactionalTable` test and a real modal for J/K (#17), fallback for
`compileTemplatePreview` (#18), KPI links as `Link` (#19), search mirror (#20), rights loading state (#21),
nullable comparison range (#22), own analytics client (#6).
