# Plan — console redesign, screens: Broadcasts (spec 5)

## Need

Rebuild the Broadcasts screens of the console on the Nuit claire components: a table with
status tabs and 30-day results (screen 61), a full-width detail page with the funnel, opens
and clicks over time, A/B and clicked links (62), and a one-screen composer that replaces the
4-step drawer and the send-or-schedule modal (63).

Spec (reviewed, decisions Q13 to Q18 taken by Pierre): "Spec 5 · Newsletters",
https://claude.ai/artifact/EZHC62qkEieXqGrtMifNkv. This plan does not repeat the spec; it
says how the code is cut, what the code on `dev` changes in the spec, and which test covers
which rule. Where this plan is silent, the spec applies.

Server parts are merged on `dev`: `status_counts`, `started_after` and
`broadcasts.estimateAudience` (#532), `opened_at`/`clicked_at` time dimensions (#532),
`origin` dimension (#524), latest template version from `broadcasts.get` (#521),
`lists.list?with_counts` (#529). Shared components are merged (#531): `PageHeader`, `Panel`,
`KpiStrip` (with `connector` and `aside`), `Pill`, `RateBar`, `FullDrawer` (+ `DrawerNav`,
`useDrawerNavKeys`), `KeyValueList`, `CountTabs`, `TableToolbar`, `SaveBar`, `Mono`,
`MetaLine`, `format.ts`. This plan defines no shared component and does not change them.

No table, column, index, migration or environment variable. No server change.

## Three PRs, all from `dev` to `dev` (the third in the docs repo, see the review corrections)

1. **`feat/compile-template-preview`** (small, first): extracts
   `components/templates/compileTemplatePreview.ts` from `TemplatePreviewDrawer`, which the
   Transactional thread (spec 7) also needs. Console only.
2. **`feat/console-broadcasts-screens`**: everything else in this plan. Rebased on `dev` once
   PR 1 is merged. `.po` catalogues and the CHANGELOG line last, after the final rebase.

## Facts from the code that change or settle the spec

1. **Analytics client cache.** `analyticsService` (singleton) runs one request at a time and
   caches every answer 30 s by its JSON. The 30 s refetch of live rows would get the cached
   answer. The broadcasts screens use their own client,
   `AnalyticsService.create({ maxConcurrency: 4, cacheTTL: 0 })`, in
   `components/broadcasts/broadcastAnalytics.ts`; TanStack Query is the only cache.
2. **Range bounds.** `inDateRange` binds the raw strings (`sql_builder.go:412-419`; only partition
   pruning parses them, `:93-107`), and a bound without zone is read in the database session's
   zone. Every bound is therefore an ISO instant with `Z`: `resolveRange().startUtc/endUtc`, or
   `dayjs.tz(…).toISOString()`; `broadcastAnalytics.test` asserts it on every bound. Time buckets of `opened_at`/`clicked_at` come back as local
   time with a `Z` (`DATE_TRUNC(… AT TIME ZONE tz)` read back as UTC,
   `sql_builder.go:319-325, 602-604`), as the spec says: `fillBuckets` builds its keys the same
   way and never converts the server value.
3. **utm_campaign today** is overwritten by the name on every name change while the drawer is
   open (`UpsertBroadcastDrawer.tsx:141-153`). The spec's rule ("follows the name until edited;
   a reopened broadcast whose stored value differs from the transformed name is considered
   edited") is new behaviour and is implemented as written. The transformation is kept:
   lower case, non-word characters to `_`, spaces to `_`, `_` runs collapsed.
4. **Variation payload today** is the raw form value. `buildBroadcastPayload` sends each
   variation as `{ variation_name, template_id, sender_id? }` only, per the spec; the server
   ignores the joined `template` and `metrics` anyway (`broadcast.go` `BroadcastVariation`).
   A joined `template` sent back would be stored in the `test_settings` JSONB
   (`broadcast.go:109-123`), which is why only the three keys go. `metadata` is never sent (the
   server keeps the stored value). A new broadcast's single variation gets
   `variation_name: 'A'`; the A/B ones `'A'…'E'` from their index; stored names are kept.
5. **Default sample.** Today 50 %; the spec's slider has no default stated. 50 % stays when
   the A/B switch is first turned on.
6. **Lists with counts.** The console's `listsApi.list` has no `with_counts` yet (spec 4 adds
   it in a parallel PR). This PR adds the optional `with_counts` parameter and
   `active_count?: number` to `List` in `services/api/list.ts` with a test of the exact URL;
   the Contacts PR will meet the same lines and the second to merge resolves the conflict.
   Query key `['lists', ws, 'with-counts']`, so it never shares data with the `['lists', ws]`
   key (no counts) other screens use.
7. **Task lookup.** `taskApi.findByBroadcastId` reads the last 100 tasks and returns `null` on
   error. Kept, as the spec says (`['task', ws, id]`, the current key).
8. **Smoke test mock** targets `../services/api/broadcasts`, which does not exist: retargeted to
   `../services/api/broadcast` (`broadcastApi`), as the spec says.
9. **e2e fixture**: `broadcasts.get`, `analytics.query`, `broadcasts.estimateAudience` answers
   are made request-aware in `e2e/fixtures/auth.ts`, as the spec says. Also: the mock workspace
   gets email tracking on (`mock-data.ts:9-21`, else the A/B switch is disabled), the list answer
   gets `status_counts`, lists get `active_count`, and `messages.broadcastStats`,
   `broadcastLinkStats`, `broadcastVariationStats` answer real shapes instead of `{}`. The payload
   checks of `broadcasts.spec.ts:355-418` are kept, on the last `update`. The empty state renders
   a heading (`protected-pages.spec.ts:79` looks a heading up).
10. **Create ignores `schedule`** (`CreateBroadcastRequest` → `ScheduleSettings{}`,
   `broadcast.go:549`), and `update` accepts `draft`, `scheduled` and `paused`
   (`broadcast.go:711-716`) with a `schedule` key that overwrites the stored one (`:739-741`).
11. **Save responses carry no templates** (`broadcast_handler.go:240-242, 297-299`); only `get`
   joins them.
12. **Server limits `validateDraft` must mirror**: the name length is counted in bytes
   (`broadcast.go:377`); feed header name and value are required (`data_feed.go:19-26`); a feed
   URL needs a host and no query nor fragment (`data_feed_url_validation.go:29-47`).
13. **Permissions load**: `useWorkspacePermissions` starts `permissions=null, loading=true`
   (`AuthContext.tsx:164-165`). Every rights-based redirect waits for `!loading`.
14. **Second wrong smoke mock**: `pages.smoke.test.tsx:194` mocks `../services/api/message_history`
   (the module is `messages_history`); retargeted too.

## Spec review corrections (Opus xhigh, 12/11)

- **Failed save stops the send flow.** `flush()` returns `{ ok: boolean }`; Send test, Send now and
  Schedule stop (message "Save failed: …") when it is not ok. Test: refused save, then Send now →
  `schedule` never called.
- **No autosave after the send flow starts.** Starting Send now / Schedule sets `sending`: inputs
  disabled, debounce and queued save cancelled; on success the composer closes (detail opens). The
  `schedule` key goes in an `update` only while the last server answer says `draft`. On unmount,
  pending timers are cleared. Test: a change typed during `schedule` → no `update` afterwards.
- **Status gate.** `?compose={id}` of a broadcast whose loaded status is neither `draft` nor
  `scheduled` is replaced by `?broadcast={id}`; same without `broadcasts.write` once permissions are
  loaded; `?compose=new` without write is dropped. Tests: paused, processing, no write, loading then
  write granted (kept).
- **Schedule choice of a new draft.** After `create`, an `update` is queued when the values carry a
  schedule choice (Schedule + date/time/timezone). `fromBroadcast` reads the choice back as
  `schedule` when `schedule.scheduled_date` is set, else `now`. Test: Schedule picked, then the list
  → create followed by an update carrying `schedule.is_scheduled=false` and the date.
- **Cache writes keep templates.** After `create`/`update`, `setQueryData(['broadcast', ws, id])`
  writes the answer with each variation's `template` carried over from the previous cache value
  when the `template_id` is unchanged; the composer reads template data from C3 anyway.
- **SaveBar** (shared, minimal change, reported in the PR): optional `saveLabel`, `saveDisabled`
  with `saveDisabledReason` tooltip, and `blockNavigation` (default `true`). The composer of a
  scheduled broadcast uses it with `blockNavigation={false}` and its own blocker, so one prompt only.
  Existing `SaveBar.test.tsx` passes unchanged, plus cases for the three props.
- **Composer blocker**: `ignoreBlocker: true` on every navigation the composer makes itself
  (`new` → `{id}`, confirmed close, close after Send now/Schedule, open detail after a refusal).
- **Pristine new composer** (nothing typed, nothing created) closes without a prompt; the e2e case
  of `broadcasts.spec.ts:339-353` stays.
- **List rows refresh**: L1 refetches every 30 s while a loaded row is live (`testing`,
  `processing`, `winner_selected`, or followed with verdict `sending`), so pills, "Test ends in" and
  L2a/L2b membership follow. Test: rows with and without a live one (interval set / not set).
- **Body keys**: the composer key changes on any `compose` value other than `new` and the id this
  body created. The detail drawer keeps the last id while it closes (no `key={undefined}` remount
  during the animation). After Delete from the detail, the drawer closes and
  `removeQueries(['broadcast', ws, id])` runs before invalidating the list.
- **Remount test** rerenders the page with a changed search value (`useSearch` mock returning the
  new `compose`) and asserts the typed name is kept and `get` not called.
- **Tasks for followed rows**: one shared query `['broadcast-tasks', ws]` (`tasks.list` limit 100),
  refetched every 30 s while a followed row is live; each row picks its task. The detail keeps
  `['task', ws, id]`.
- **Rows deduped by id** after flattening pages. The "wait for the next row" J effect is cancelled
  on K, close and fetch error.
- **C3 key** reuses `['template', ws, id]` with the same `{ template }` shape as
  `TransactionalNotificationsPage.tsx:77-78`.
- **Links panel**: hidden when the broadcast has no clicked link (or `no_tracking`, with its
  sentence); a variant without clicks shows the empty message (old "empty" case).
- **statusBadge line 85-89** ("keeps naming the phase" of `winner_selected`) becomes an assertion on
  the tooltip, since the pill reads "Sending".
- **`progressStatsFrom(stats, enqueuedCount?)`** returns `failedCount` (= `total_failed`) too;
  without `enqueuedCount` it returns the queue fields and `remaining: 0`.
- **Data feeds**: `GlobalFeedSettings` and `RecipientFeedSettings` get the tooltip "Save the
  broadcast first" on their disabled test buttons (both files edited, their tests extended).
- **Docs PR (third)**: `docs` repo, branch `docs/broadcasts-screens` from `dev`:
  `openapi.json` copied from notifuse `openapi.json`, and `features/broadcast-campaigns.mdx`
  (one-screen composer, estimate, summary, unsubscribe warning; detail with funnel, chart, links;
  steps at lines 344-350 rewritten). Screenshots in the mdx are replaced by the captures.

## Code layout (console/src)

```
components/templates/compileTemplatePreview.ts            (PR 1)
components/broadcasts/
  broadcastStatus.ts        status → pill tone/label/tooltip, deliveryVerdict, getRemainingTestTime,
                            isFollowedRow, isLiveRow, progressStatsFrom(stats, enqueued)
  broadcastAnalytics.ts     dedicated AnalyticsService + pure query builders L2, L3, L4/L5,
                            L7, F6, F7/F8, and row readers (sumByBroadcast, visitsByCampaignDay)
  attribution.ts            sumVisitsInWindow, kpiTrafficFromRows (pure, L3 and L7 sums)
  useBroadcastsList.ts      L1 (useInfiniteQuery), status counts, flattening
  useBroadcastPageStats.ts  L2a, L2b, L2c, L3 for the loaded rows
  useAudienceEstimates.ts   L8, at most 4 in flight
  useBroadcastActions.ts    pause, resume, cancel, retry failed, select winner, delete, with
                            the spec's invalidations and messages
  DeleteBroadcastModal.tsx  the current type-the-ID modal, moved
  BroadcastKpis.tsx         L4 to L7 KpiStrip
  BroadcastsTable.tsx       table rows, cells, row menu
  detail/
    BroadcastDetail.tsx     FullDrawer body: load by id, header, actions, layout
    BroadcastFunnel.tsx     KpiStrip connector + aside, clickable cells
    EngagementChart.tsx     echarts bars + line, markers, Segmented 48 h / 7 d / 30 d
    fillBuckets.ts          bucket keys and zero fill (pure)
    AbTestPanel.tsx
    LinkStatsPanel.tsx      replaces BroadcastLinkStats
    BroadcastAside.tsx      Preview, Audience, Web tracking, Sending task, History
    TemplatePreviewFrame.tsx  compiled HTML in a scaled iframe (detail + composer)
  compose/
    composeValues.ts        ComposeValues type, fromBroadcast(), newDraft(), campaignFromName()
    validateDraft.ts
    buildBroadcastPayload.ts
    useAutosave.ts
    BroadcastCompose.tsx    FullDrawer body: top bar, layout, send flow
    AudienceCard.tsx, ContentCard.tsx, SendingCard.tsx, TrackingCard.tsx, AdvancedRow.tsx
    PreviewPanel.tsx
    SendSummary.tsx         checklist (pure `summaryChecks()` + view)
lib/unsubscribeLink.ts
components/web_analytics/lib/broadcastLinks.ts  + broadcastAttributionWindow()
pages/BroadcastsPage.tsx    rewritten: header, KPIs, toolbar, table, the two URL drawers
router.tsx                  BroadcastsSearch + broadcast, compose
services/api/broadcast.ts   with_status_counts, started_after, status_counts, estimateAudience
services/api/list.ts        with_counts, active_count
services/api/permissions.ts estimateAudience in the broadcasts read entry
```

Deleted: `UpsertBroadcastDrawer.tsx` (+ test), `SendOrScheduleModal.tsx`,
`BroadcastStats.tsx` (+ test), `BroadcastLinkStats.tsx` (+ test),
`pages/BroadcastsPage.statusBadge.test.tsx`, `DataFeedSettings.tsx`. Every case of the deleted
tests is reassigned below. `HeadersEditor`, `GlobalFeedSettings`, `RecipientFeedSettings`,
`VariationSenderSelect`, `ChangeVariationSenderModal`, `SendingProgress`,
`BroadcastAnalyticsLinks`, `variationMetrics`, `variationSender` stay.

### Drawers driven by the URL

`BroadcastsPage` reads `broadcast` and `compose` from the route search. The list query key is
`['broadcasts', ws, statusGroup, q]` only. The detail drawer is `open={!!broadcast}` and its
body is `<BroadcastDetail key={broadcast} … />`. The composer holds its body key in page
state, set when `compose` goes from absent to present (`compose` value, or `'new'`), so the
`new` → `{id}` URL change after `create` does not remount it. Closing removes the parameter
(`navigate({ search: prev => ({ ...prev, broadcast: undefined }) })`). J/K: `replace`
navigation over the flattened loaded rows; at the last loaded row J calls `fetchNextPage()`
when `hasNextPage`, then moves on once the page has arrived (an effect waiting for the next
row); a draft reached by J/K opens in the detail drawer. If the open id is not in the loaded
rows, `nav` has no `onPrev/onNext` and no position.

When both parameters are present, `compose` wins and `broadcast` is removed (`replace`).

### Statuses, followed rows and verdicts (`broadcastStatus.ts`)

- `FOLLOWED_STATUSES = testing, test_completed, processing, winner_selected, paused, failed`;
  `isFollowedRow(b, now)` adds `processed` with `completed_at` less than 3 days before `now`.
  A `processed` row without `completed_at` is followed (it may still be draining).
- `progressStatsFrom(statsResult, enqueuedCount)` reproduces `BroadcastStats`'s
  `ProgressStats` (`inFlight` = pending + processing + paused + failed_retrying,
  `failedTerminal`, `remaining`); `deliveryVerdict` moves unchanged.
- `statusPill(b, { verdict?, l2Failed? })` returns `{ tone, label, pulse, tooltip }` per the
  spec's table; tooltips are the current ones of `StatusBadge`, kept word for word where the
  status still exists (`processing` tooltip, `winner_selected`, …).
- `getRemainingTestTime(b, testResults, now)` returns the duration only (`dayjs.to(…, true)`);
  the sentences "Test ends in {duration}" and "{duration} remaining" are Lingui messages.

### KPIs (L4 to L7) and website visits (L3)

Built in `broadcastAnalytics.ts` and `attribution.ts` as pure functions, consumed by
`BroadcastKpis` and `useBroadcastPageStats`. Shown only when the conditions of the spec hold:
`message_history.read` for the strip, `web_analytics.read` and web analytics enabled
(same test as `BroadcastAnalyticsLinks`: read it and reuse its condition helper, extracting it
if it is inline) for the traffic cell and the visits column.

`broadcastAttributionWindow(broadcast, timezone, now)` returns `{ start, end }` local days
(Dayjs), extracted from `buildBroadcastAnalyticsLinks`, which now calls it. Null when the
broadcast has no `started_at`.

### Detail page

As the spec: header (meta line, actions, `Alert` for `error_message`, `SendingProgress` under
the header), funnel, chart, A/B panel, links panel, aside. F1 `broadcasts.get` (key
`['broadcast', ws, id]`, `with_templates: true`), 404 → "Broadcast not found". Refetch every
30 s while the spec's live conditions hold (F1, F2), else F2 `staleTime` 5 min.

Chart: echarts (already a dependency, as the dashboard) — bars `count_opened` per bucket,
line `count_clicked`, vertical dashed `markLine`s at `test_sent_at` and `winner_sent_at`
(converted to the bucket key of their local hour/day). `fillBuckets(start, end, granularity,
timezone, rows, column)` returns `[{ key, value }]` from the first to the last bucket of the
window, bounded by now.

### Composer

State: one `ComposeValues` object in React state (not an antd `Form` at the top: the
autosave needs every change as a value, and the cards are small). Cards use antd inputs,
controlled. `validateDraft(values, ctx)` and `buildBroadcastPayload(values, ctx)` are pure.

Autosave (`useAutosave`): debounced 1 s after the last valid change, one request at a time,
a change during a request queues one more at its end; `create` on the first valid state of a
new broadcast, then `update`. `flush()` returns a promise that resolves once nothing is
pending (used by Send test, Send now, Schedule, close). `setQueryData(['broadcast', ws,
id], response.broadcast)` after each answer; `['broadcasts', ws]` invalidated once at close.
Scheduled broadcast: no autosave, `SaveBar` (the shared one) with `dirty` from a comparison of
`buildBroadcastPayload` output with the one of the loaded broadcast. A server refusal shows
"Not saved" with its message and a Retry link; a refusal that means the broadcast left
draft/scheduled (re-read of F1 shows another status) offers to open its detail page.

Closing with unsaved or incomplete changes asks for confirmation (modal with the reason).
Browser navigation away is guarded by `useBlocker` while there are unsaved changes
(`shouldBlockFn` blocks when `compose` leaves the URL or the path changes, not on the
`new` → `{id}` replace done by the composer itself, which passes `ignoreBlocker: true`).

Send now / Schedule: `flush()`, then `broadcasts.schedule` as the spec; on success
`compose` → `broadcast={id}` (replace) and invalidations of the spec. Send now opens a
confirmation modal first.

Preview: `compileTemplatePreview` with the variant shown; Desktop iframe 600 px scaled to the
column width, Mobile `PhonePreview` (default export `IphoneEmailPreview` of
`components/templates/PhonePreview`). Unsubscribe check from `lib/unsubscribeLink.ts` on the
loaded template (C3 `templates.get`, `refetchOnWindowFocus: true`).

## Files, tests and cases

TDD: each test file is written before its implementation file; the page and drawer tests are
written before the page rewrite. Vitest + React Testing Library, mocks of `services/api/*`.

### PR 1

| File | Test | Cases |
|---|---|---|
| `components/templates/compileTemplatePreview.ts` (new): `buildTemplatePreviewRequest({ workspace, email, testData })` → `{ request }` or `{ error: 'missing_content' \| 'invalid_tree' }`; `compileTemplatePreview(input)` → `templatesApi.compile(request)`, rejects with `TemplatePreviewError` (`code`) when no request can be built | `compileTemplatePreview.test.ts` | code mode sends `mjml_source`, no tree; visual mode with an object tree and with a JSON string tree; unparsable string → `invalid_tree`; missing content in either mode → `missing_content`; subject, subject_preview, custom text only when not blank, test_data default `{}`, tracking settings from the workspace; compile called with the built request; rejection with the code |
| `components/templates/TemplatePreviewDrawer.tsx` | `TemplatePreviewDrawer.test.tsx` passes unchanged | uses `buildTemplatePreviewRequest`, same request as before (existing tests) |

Verification PR 1: `npx vitest run src/components/templates`, `npx tsc -b`, `npm run lint`,
`npm run build`.

### PR 2

| File | Test | Cases |
|---|---|---|
| `router.tsx` (exports `validateBroadcastsSearch`) | `src/__tests__/broadcastsRoute.test.ts` (new) | `broadcast` and `compose` normalised like `status`/`q` (trim, empty dropped, array → first); unknown keys dropped |
| `services/api/broadcast.ts` | `services/api/broadcast.test.ts` | `list` sends `with_status_counts` and `started_after` only when set; `estimateAudience` POSTs `{ workspace_id, audience }` to `/api/broadcasts.estimateAudience` |
| `services/api/list.ts` | `services/api/list.test.ts` (new or existing) | `with_counts=true` in the URL only when asked |
| `services/api/permissions.ts` | `permissions.test.ts` | broadcasts read lists `/api/broadcasts.estimateAudience` |
| `lib/unsubscribeLink.ts` | `unsubscribeLink.test.ts` | code mode; visual mode (tree object and string); the three variables; Liquid filter `{{ unsubscribe_url \| escape }}`; translation without link reported by language; default reported as `default`; no false positive on `unsubscribe_url_text` nor on text outside `{{ }}` |
| `components/web_analytics/lib/broadcastLinks.ts` | `broadcastLinks.test.ts` | existing cases unchanged; window alone: completed, not completed, capped at today, never before start |
| `components/broadcasts/broadcastStatus.ts` | `broadcastStatus.test.ts` | every case of `BroadcastsPage.statusBadge.test.tsx` (incl. its line 34, processed while draining) on the new pills; each status of the spec table; `{n} failed` from L2 on a non-followed row; followed row without queue → fallback; `isFollowedRow` for processed 2 and 4 days old and without `completed_at`; duration without hard-coded text |
| `components/broadcasts/broadcastAnalytics.ts` | `broadcastAnalytics.test.ts` | exact query objects of L2, L3, L4/L5, L7, F6, F7/F8 (measures, dimensions, filters, timezone, no `dateRange`); readers turn rows into maps |
| `components/broadcasts/attribution.ts` | `attribution.test.ts` | sum limited to the window; empty campaign → null; row older than 90 days → null; KPI: day counted once for two broadcasts of one campaign; days outside every window dropped; broadcast started today → nothing in the period; no campaign → 0 without call (hook test) |
| `components/broadcasts/useBroadcastPageStats.ts` | `useBroadcastPageStats.test.ts` | only L2a refetches every 30 s; a row that finishes moves to L2b; L2c only for followed rows; no L3 without web analytics permission |
| `components/broadcasts/useAudienceEstimates.ts` | `useAudienceEstimates.test.ts` | ten drafts → never more than 4 in flight; same audience → one call; no call without `lists.read` |
| `components/broadcasts/BroadcastKpis.tsx` | `BroadcastKpis.test.tsx` | rates and point deltas with tone; zero denominator → "—"; traffic cell absent without web analytics; hidden without `message_history.read` |
| `components/broadcasts/useBroadcastActions.ts`, `DeleteBroadcastModal.tsx` | covered by page and detail tests | — |
| `pages/BroadcastsPage.tsx`, `BroadcastsTable.tsx`, `useBroadcastsList.ts` | `pages/BroadcastsPage.test.tsx` (new) | tab counts, missing status 0, Failed tab hidden at 0 unless active; search written to `q` with replace after 300 ms; opening the detail does not call `broadcasts.list` again; draft row click opens `compose`; "Load 25 more"; empty states (none, no match + Clear filters); followed row: Pause and Cancel by verdict, Retry by `failedTerminal`; old row: neither, "Retry failed…" opens the detail; delete by typing the ID; rights (no `message_history.read`: no KPIs and "—"; no `web_analytics.read`: no visits column; no `broadcasts.write`: actions disabled, `?compose=new` dropped); no marketing provider → Alert |
| `components/broadcasts/detail/fillBuckets.ts` | `fillBuckets.test.ts` | keys "local hour + Z" in Europe/Paris and America/New_York; server value never converted; empty buckets 0; window bounded by now; day granularity |
| `components/broadcasts/detail/BroadcastDetail.tsx` | `BroadcastDetail.test.tsx` | loads by id (not from the list); 404 → not found; J and K with replace; arrows disabled outside the list; draft reached by J opens in detail; state reset on change (key); F1 refetched while sending; Refresh stats invalidates F1 to F5; header meta per status; actions per status and rights; Alert with task error |
| `components/broadcasts/detail/BroadcastFunnel.tsx` | `BroadcastFunnel.test.tsx` | values and rates; cells navigate to logs with the right filter; SMTP: Delivered, Bounces, Complaints "—" not clickable with tooltip; failed share on sent + failed; visits and conversions only with web analytics |
| `components/broadcasts/detail/EngagementChart.tsx` | `EngagementChart.test.tsx` | hidden without `started_at`; F7/F8 query per period; markers when the dates exist (echarts option built by a pure `buildChartOption`, asserted) |
| `components/broadcasts/detail/AbTestPanel.tsx` | `AbTestPanel.test.tsx` | letters, winner and recommended pills; From: deleted, default, chosen on the broadcast, Reply-To; "Pick as winner" per `canSelectWinner`, confirmation; header sentence (manual, auto, sent to remaining, test ends in) |
| `components/broadcasts/detail/LinkStatsPanel.tsx` | `LinkStatsPanel.test.tsx` | every case of `BroadcastLinkStats.test.tsx` (one row per link, caveats behind the icon, widest bar for top link, "—" without recipients, URL never a link, copy, empty, error, See who clicked with and without `template_id`) + 200 note, 10 rows then Show all, hidden with `no_tracking` with its sentence |
| `components/broadcasts/detail/BroadcastAside.tsx` | `BroadcastAside.test.tsx` | Sending task rows with `error_message` and `failed_count`; History only present dates; Web tracking empty utm_content text; Audience segments None |
| `components/broadcasts/ChangeVariationSenderModal.tsx` | existing test | both keys invalidated |
| `compose/validateDraft.ts` | `validateDraft.test.ts` | each reason of the spec list; complete draft → none |
| `compose/buildBroadcastPayload.ts` | `buildBroadcastPayload.test.ts` | cases of `UpsertBroadcastDrawer.test.tsx` lines 122-184 (feeds), 193-265 (schedule: none for a scheduled broadcast, `is_scheduled:false` + choice for a draft, create unscheduled), 445-535 (sender sent, none when kept or cleared, one per A/B variation); variations carry only three keys |
| `compose/composeValues.ts` | `composeValues.test.ts` | `fromBroadcast` round trip; utm_campaign considered edited when stored ≠ transformed name; new draft name "Broadcast {date}" and defaults (utm_source from site TLD, medium email, 11:00, timezone broadcast → workspace) |
| `compose/useAutosave.ts` | `useAutosave.test.ts` | no request before the list; none while invalid, "Incomplete" reason exposed; one at a time with one queued; `setQueryData` after each answer, list not invalidated; `flush()` waits; error state |
| `compose/ContentCard.tsx`, `TrackingCard.tsx`, `AudienceCard.tsx`, `SendingCard.tsx` | one test each | cases of `UpsertBroadcastDrawer.test.tsx` 277-397 (tracking default, `no_tracking`, stored value, auto winner off and locked, unknown-consent warning and none) and 404-412 (unsubscribed help) and 445-535 (sender rule, deleted sender refused); A/B switch disabled without workspace tracking; 5 variants max; template already used refused; time "9:5" → "09:05"; past date refused |
| `compose/SendSummary.tsx` | `SendSummary.test.tsx` | each check and level; error count disables the main button tooltip |
| `compose/BroadcastCompose.tsx` | `BroadcastCompose.test.tsx` | create then URL replace without remount (typed value kept); scheduled: only Save changes and no `schedule` key; Send test waits for flush, disabled before create and without `transactional.write`; utm_campaign follows the name until typed; Send now: flush then schedule then detail; server refusal shown; `?compose=new` without write dropped |
| `src/__tests__/pages.smoke.test.tsx` | itself | both mocks retargeted (`broadcast`, `messages_history`); page renders "Broadcasts" and a row |
| `components/layout/SaveBar.tsx` | `SaveBar.test.tsx` | see "Cases added by the spec review" |
| `e2e/fixtures/auth.ts`, `e2e/features/broadcasts.spec.ts`, `e2e/protected-pages.spec.ts` | e2e | create to saved draft; Incomplete with an empty variant; open a detail by URL then J to the next with the right name; status shown; empty state texts |

### Cases added by the spec review

- `broadcastStatus.test.ts` (from `BroadcastStats.test.tsx:64, 77`): queue mapped to `inFlight` and
  `failedTerminal`; both undefined when the queue is unreadable; `failedCount` from `total_failed`.
- `BroadcastFunnel.test.tsx` (from `BroadcastStats.test.tsx:51, 100, 121, 139`): failed count and
  share over sent + failed; rates over tracked emails with the "of {n} tracked" hint; no hint when
  every email was tracked; "—" when nothing was tracked.
- `validateDraft.test.ts`: multibyte name over 255 bytes; feed header without name or value; feed
  URL without host, with a query, with a fragment.
- `useAutosave.test.ts`: list invalidated once, at close only; `flush()` returns `{ ok: false }`
  after a refusal; nothing saved after `freeze()`; create followed by an update carrying the
  schedule choice; cache write keeps joined templates.
- `useAudienceEstimates.test.ts`: segments without `segments.read` → no call.
- `BroadcastCompose.test.tsx`: without `templates.read` the template picker is disabled with
  "You need access to templates"; without `lists.read`/`segments.read` selectors show the stored
  value only and no estimate; templates limited to `category="marketing"`; status gate cases;
  pristine close without prompt; scheduled composer uses SaveBar "Save changes" disabled while
  incomplete.
- `BroadcastsPage.test.tsx`: L1 error → `Result` with Retry (403 included); L2–L8 error →
  "—" with "Couldn't load"; Unsub. warning at 0.5 %, Bounces at 2 %; RateBar scales 0.7 and 0.25;
  L1 refetch interval only with a live row.
- `BroadcastDetail.test.tsx`: draft or scheduled → "Stats appear once the broadcast starts
  sending"; `no_tracking` → opens/clicks "—" with "Not tracked", chart and links hidden; no
  `message_history.read` → no funnel, chart, links; delete closes before the list refetch.
- `BroadcastAside.test.tsx`: without A/B, Preview block shows From, Reply-To, subject and
  `subject_preview`.
- `fillBuckets.test.ts`: a daylight-saving change day in Europe/Paris (23 and 25 hour days).
- `SaveBar.test.tsx`: `saveLabel`, `saveDisabled` with its tooltip, `blockNavigation={false}`.
- `GlobalFeedSettings`/`RecipientFeedSettings` tests: disabled test button tooltip.
- `buildBroadcastPayload.test.ts`: no `metadata` key.

## Captures

Playwright against `npm run dev` on port 19173 with the e2e fixture backend, 1440 and 1280 px,
deviceScaleFactor 2: list, list with a followed row, detail (A/B, sent), detail of a draft,
composer (new, A/B on, incomplete), composer of a scheduled broadcast. Saved under
`/mnt/project-files/captures/fil-5-6/`. The dev server is stopped afterwards.

## CHANGELOG

Under the unreleased 44.0, the existing broadcasts feature line written by #532 is extended for
the screens (no new Fix: #521's fix line already covers the template version). One sentence,
customer-facing.

## Verification (run before review, again after fixes)

```
cd console
npx vitest run src/components/broadcasts src/components/templates src/pages/BroadcastsPage.test.tsx src/lib/unsubscribeLink.test.ts src/components/web_analytics/lib src/__tests__ src/services/api
npx tsc -b
npm run lint
npm run build
npm test -- --run          # full suite once before review
npx playwright test e2e/features/broadcasts.spec.ts e2e/protected-pages.spec.ts
npm run lingui:extract     # last, after rebase
```
No backend file changes: the Go suites are not run for these PRs.
