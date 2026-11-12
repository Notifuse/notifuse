# Transactional screen polish — plan

Follow-up of #543 (console transactional screen, merged in dev). It treats the items of its
« Known, not treated » list, except the all-digit ID in the logs links (owned by the logs work).
Console only: no server, schema, query or environment change.

## 1. Accessibility

### 1a. Rows: a real link per notification
`TransactionalTable.tsx`
- The notification name becomes a TanStack `<Link to="." search={(prev) => ({ ...prev, notification: id })}>`,
  so each row carries a link with an `href` (keyboard reachable, readable by screen readers, opens in a
  new tab with a modifier click).
- Plain click on the link: `preventDefault()` so the Link does not navigate itself; the click bubbles to
  the row, whose `onClick` opens the sheet once (a new history entry, as today). Modifier click
  (`metaKey`, `ctrlKey`, `shiftKey`, `altKey`; a middle click fires `auxclick`, not `click`, and never
  reaches the row): `stopPropagation()` only, so the
  browser opens the link in a new tab and the row does not open the sheet as well.
- The row loses `tabIndex`, `aria-label` and its Enter handler: the link is the keyboard target (Enter
  on a focused link fires its click). Mouse click anywhere on the row still opens the sheet.
  `group-focus-within` still reveals the row actions when the link is focused.

Tests — `TransactionalTable.test.tsx` (router `Link` mocked to an `<a>` that forwards `onClick` and
renders its search as `href`, as `TransactionalDetail.test.tsx` does):
- each row has a link named after the notification whose href carries `notification={id}`;
- a click on the link opens the sheet once (`onOpen` called once) and its default is prevented;
- a ctrl/meta click on the link does not call `onOpen`;
- the row is no longer focusable (no `tabIndex`); the old « Enter on a row » test is replaced by the link
  test, the « Enter on a button of the row does not open the sheet » assertion is kept for the buttons.
`TransactionalNotificationsPage.test.tsx`: « opens the sheet of a row … by click and with Enter » becomes
« … by click on the row and on its link » (the page mock of `Link` forwards `onClick`).
`e2e/features/transactional.spec.ts`: « views notification details » also opens a sheet through the
name's link (`getByRole('link', { name: 'Password Reset' })`).

### 1b. Reasons of disabled menu entries, visible without hover
`notificationParts.tsx`: new `MenuEntryLabel({ label, reason })` — the label, and when a reason is given,
a second line in `text-xs text-ink-3` with the reason (wrapping, `max-w-[260px]`). No tooltip. An empty
reason (`''`, the template still loading) shows the label alone. Used by the table's menu (replaces the
local `menuLabel`) and by the sheet's menu (1c). Being text inside the `menuitem`, the reason is part of
its accessible name and readable without a mouse.

Tests — `TransactionalTable.test.tsx`: the existing « disables … and says why » and « refuses to delete a
notification an integration manages » assert the reason is inside the disabled `menuitem` with no hover
(`within(entry).getByText(reason)`), and the « keeps the template preview disabled until the template is
read » case asserts no second line. `notificationParts` gets a small test for `MenuEntryLabel`.

### 1c. KPI links named by what they count
`TransactionalDetail.tsx`: `logLink(label, value, filter)` sets `aria-label={t\`${label}: ${value}, view in the message log\`}`
on the link (the visible value stays in the name, WCAG 2.5.3).
Tests — `TransactionalDetail.test.tsx` « links each KPI … »: `getByRole('link', { name: 'Opened: 30.0%, view in the message log' })`.

### 1d. Text alternative of the sends per day chart
`SendsPerDayChart.tsx`: the ECharts element is wrapped in `aria-hidden="true"`, and a `sr-only` `<table>`
(caption « Sends per day », header row Day / Sent / Delivered / Bounced / Failed, or Day / Sent / Failed
with SMTP) lists each day of the series with the same day labels and number formatting, today marked
« Today, so far ». Rendered only when the chart is (not while loading, failed or empty).
Tests — `SendsPerDayChart.test.tsx`: the table has one row per point plus the header, the SMTP variant has
3 columns, the table is absent while loading and when empty, the chart wrapper is `aria-hidden`.

## 2. Sender of the sheet's header, marked when it is only the provider's default
`TransactionalDetail.tsx` + `senderInfo.ts`
- Today « (default) » is appended only when the template is `forbidden`. It becomes: whenever
  `providerDefaultOnly` (the template is not read: forbidden, not found, failed, or the notification has
  no template). While the template loads the sender stays hidden, as today.
- The tooltip of the sender then says why: « The provider's default sender: the template's own sender is
  unknown until the template is read. » (the general tooltip stays for a resolved sender).
- `SenderInfo.providerDefaultOnly` doc comment updated.
Tests — `TransactionalDetail.test.tsx`: « does not mark the provider's default when the template only
failed to load » is inverted (marked, with the tooltip); new cases: template not found (404) marked;
template loading → no sender shown (pins the current behaviour); resolved template → not marked.

## 3. Sheet: reasons of its disabled menu entries, and the « Deleted template » pill
`TransactionalDetail.tsx`
- The sheet's « View in logs » and « Delete » entries use `MenuEntryLabel` with `rules.logs` /
  `rules.remove` (same reasons as the table).
- The header's template meta shows the shared `DeletedTemplatePill` (new in `notificationParts.tsx`,
  the warning pill with the tooltip « This notification still sends with it ») after the name when
  `loadedTemplate.deleted_at` is set. `TemplateLabel` (table) and `PreviewPanel` use the same component.
Tests — `TransactionalDetail.test.tsx`: without transactional write and message history read, the sheet
menu shows « Delete » disabled with « You need Transactional write permission » and « View in logs »
disabled with « You need access to message history »; a managed notification shows the integration
reason; a deleted template shows the pill in the header (`PageHeader` meta) — `PreviewPanel.test.tsx` and
`TransactionalTable.test.tsx` keep their pill assertions.

## 4. Sheet figures of a notification the list does not cover
Today the sheet reads its KPIs from the list's statistics map. A notification outside the list's IDs
(created elsewhere after the list loaded, or opened by link) shows zeros; and when the list's IDs become
empty after a delete, the list's stats query is disabled with `keepPreviousData`, so
`isPlaceholderData` stays true and `sheetGate.loading` never clears.
- `useTransactionalStats.ts`: the current-window query is extracted into an exported
  `useCurrentStats({ workspaceId, ids, period, periods, timezone, enabled })` hook (same key, same query,
  same `{ period, stats }` data) used by `useTransactionalStats`.
- `TransactionalDetail` gets a `listed: boolean` prop: the page's statistics cover this notification.
  Page: `listed = list.isLoading || ids.includes(search.notification)`.
- `DetailContent`, when `!listed`, runs `useCurrentStats` for `[notification.id]` (enabled with
  message history read) and gates its KPIs on it: loading while it loads or shows a previous period
  (`isPlaceholderData`), error when it failed with nothing loaded. When `listed`, nothing changes.
- The page's `sheetGate` keeps its period-switch wait only while the query is enabled
  (`stats.current.isPlaceholderData && ids.length > 0`), so an empty list never leaves it loading.
Tests —
- `TransactionalDetail.test.tsx`: with `listed: false`, the sheet asks `message_history` grouped by
  `transactional_notification_id` filtered on `[id]` over the current window, and shows its figures
  (not zeros); with `listed: true`, no such query; with `listed: false` and the query failing, the KPIs
  say « Couldn't load ».
- `TransactionalNotificationsPage.test.tsx`: the sheet gets `listed: true` for a listed notification and
  `false` for one outside the list; after the list goes from one notification to none (refetch), the
  sheet's `statsGate.loading` is false.
- `useCurrentStats` covered through the two above (the list tests of the page already pin its query).

## 5. Literal « last 60 days », and missing pins
- `TransactionalTable.tsx`: the tooltip uses `LAST_SENT_DAYS` with a plural
  (« No successful send in the last # days »). `LAST_SENT_DAYS` stays in `useTransactionalStats.ts`,
  imported by the table.
  Test: the tooltip text of a dash in « Last sent » is « No successful send in the last 60 days ».
- `refetchOnWindowFocus` pinned: page test — after the figures load, move the clock past the stale time
  and fire `focusManager.setFocused(true)` (after `false`): the current-window and last-sent queries
  are asked again, the previous-window query is not. Detail test — same for the daily series.
- Last day in the daily series key: detail test — rerender with the periods of the next day (same
  `period`, later `endDay`): the daily series is asked again with the new `dateRange`.
- Open button → template preview drawer:
  - `TransactionalDetail.test.tsx`: clicking the preview panel's « Open » calls `onPreview` with the
    notification.
  - `TransactionalNotificationsPage.test.tsx`: the sheet's `onPreview` opens `TemplatePreviewDrawer`
    with the loaded template, also for a notification outside the list (template read under the same key
    by the sheet).
  - e2e `transactional.spec.ts`: from a sheet, « Open » in the preview panel shows the template preview
    drawer above the sheet.

## i18n
`npm run lingui:extract`, then fill every new `msgstr` in ca, de, es, fr, it, ja, pt-BR (and drop the
obsolete « No successful send in the last 60 days » entry the extract marks).

## CHANGELOG
44.0 is unreleased and the transactional screen ships in it: no Fix bullet. One short sentence appended
to its transactional feature bullet: the name of each notification is a link to its detail view,
disabled actions say why in the menu, and the sends per day can be read as a table by screen readers.

## Verification
- `cd console && npx vitest run src/components/transactional src/pages/TransactionalNotificationsPage.test.tsx`
  while working, then `npm test` once.
- `npx tsc -b`, `npx eslint` on the changed files, `npm run build`.
- `npx playwright test e2e/features/transactional.spec.ts e2e/protected-pages.spec.ts --trace off`.

## Spec review (Opus, xhigh) — confirmed points folded in
- 1a: TanStack `Link` runs the user's onClick first and skips its own navigation once `defaultPrevented`
  (`@tanstack/react-router/src/link.tsx` `composeHandlers`), and ignores modified clicks itself: the
  approach holds. The `Link` mocks of the table and page tests must forward `onClick` and resolve a
  `search` function. The « Enter on a button of the row » test becomes a click test (the row has no key
  handler any more), and the row actions' `onKeyDown={stop}` goes. The keyboard path is tested in e2e:
  focus the name's link, Enter opens the sheet, Back closes it (one history entry). It gets its own e2e
  test (the full-width sheet would cover the table in « views notification details »).
- 2: the tooltip names why the sender is the provider's default, per case: no right to templates,
  template not found, template failed to load, no template. Reply-To shows « Unknown » rather than « — »
  when the template exists but is not read.
- 3: the sheet's preview panel already shows the pill: it moves to the header only, so the sheet shows it
  once (`PreviewPanel` and its test lose it; `PreviewPanel` is only used by the sheet).
- 4: diagnosis confirmed (query-core applies `keepPreviousData` to a disabled pending query). The sheet's
  own query always runs as a hook, `enabled: canReadMessages && !listed`; `useCurrentStats` is the single
  source of key and query function (the key equals the list's when the list is exactly `[id]`, and then
  the data is identical). `listed` guards an undefined ID. The `sheetGate` change is dropped (redundant).
  Detail tests default `listed: true`; the unlisted case's `query` mock answers by `dimensions`.
- 5: the « Last sent » tooltip is pinned with `LAST_SENT_DAYS` mocked to 45; the focus tests use a client
  with `refetchOnWindowFocus: false`, as the console's, and reset `focusManager`. The preview e2e clicks
  inside the preview drawer, which fails if the sheet covers it.
- i18n: `src/i18n/locales/catalogues.test.ts` (no empty msgstr) joins the targeted run.

## Code review (Fable, high) of bdbf9e8 — folded in
- Pinned: the sheet's own figures wait for a new period (`isPlaceholderData`), and the sheet waits for
  the list rather than ask its own figures while the list loads.
- The « empty list after a delete » page test now starts from a listed notification.
- `MenuEntryLabel` has its own test (`notificationParts.test.tsx`); `focusManager` is reset after each test.
- « Preview template » disabled because the template failed to load now says so
  (« Couldn't load the template »), so every disabled menu entry gives its reason.

Known, not treated: antd skips disabled menu items in arrow navigation, so a screen reader reads their
reason in browse mode only; the default-sender reason, the deleted template pill and the Send test / Edit
reasons stay hover tooltips; a KPI link without a rate is named « Delivered: —, … ».
