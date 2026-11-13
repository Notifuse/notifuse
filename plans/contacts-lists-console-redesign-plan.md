# Plan — console redesign: Contacts and lists (spec 4, console part)

## Need

Spec 4 « Contacts et listes » (reviewed, decisions Q2 and Q7–Q12 taken by Pierre on 12 Nov 2026,
all as the spec recommends, Gravatar kept on the contact sheet). The server part is merged in `dev`
(#529: `contacts.list?search`, `lists.list?with_counts`, `timeline.list?entity_types` with permission
filtering, `webAnalytics.contactVisits` and `webAnalytics.contactSessionPages`). The theme (#530) and
the shared layout components (#531: `PageHeader`, `RailLayout`/`RailSection`/`RailItem`, `Panel`,
`KpiStrip`, `Pill`, `Mono`, `KeyValueList`, `CountTabs`, `TableToolbar`, `FullDrawer`, `MetaLine`,
`format.ts`, `useDrawerNavKeys`) are merged too: this PR reuses them and redefines none.

This PR is the console part only: Contacts, Lists and Segments on one `rail` page, the Lists page
removed, the contact sheet as a full-width drawer driven by the URL, with J/K navigation, inline
profile editing, KPIs, activity filters and a Web visits tab. No Go file, no SQL, no environment
variable.

## Scope decisions (from the spec, restated so the review can check them)

- Q7 list counts shown in the rail (`with_counts`), Q8 « 2 of 50+ » with filters, Q9 members with
  `lists.read` only land on Contacts with the Lists section only, Q10 server filtering already in
  #529, Q11 unified search + activity filters + « Remove from list » delivered, Q12 a row click opens
  the sheet (checkbox alone selects).
- Q2: the sheet keeps the Gravatar photo (`d=mp`, 40 px) with initials as fallback; table rows show
  initials only.
- Not delivered (spec « hors périmètre »): 30-day variation and sparkline under list KPIs, MRR KPI,
  Sort button, import history, copy emails, GDPR export, workspace comparison of contact KPIs,
  subscription source, page titles and « Identified by » in web visits.

## Shared component changes (minimal, additive, reported to the other threads)

`components/layout/DrawerControls.tsx`, interface `DrawerNav`:

- `totalLabel?: string` — text shown in place of the formatted `total` (« 50+ »). Shown only when
  `position` is set; `total` stays required for the position to show (unchanged rule), so a caller
  passing `totalLabel` also passes `total`.
- `buttons?: boolean` (default `true`) — `false` hides Previous/Next and the position, keeping only
  `context`. The spec's case « contact opened by a link, outside the loaded rows » shows the view name,
  copy link and close only.

Both are additive: no existing caller changes behaviour. Tests in `FullDrawer.test.tsx`.

The shared tooltips stay « Previous (K) » / « Next (J) » (the spec's « Previous contact (K) » would
need a per-caller label; not worth a shared change).

## Files

### Router and menu

`src/router.tsx`

- `ContactsSearch` gains `segment_id?: string`, `search?: string`, `contact?: string`.
- `validateSearch`: every string param coerced through a `normalizeString` helper (the default
  parser `JSON.parse`s values: `?search=123` would be a number, which the page would `.trim()`).
  `limit`: non-numeric or < 1 → 50 (default), > 10 000 → 10 000. `list_id` and `segment_id` both present →
  `segment_id` dropped (list wins).
- `beforeLoad` on the contacts route: `segments` with exactly one value and no `segment_id` and no
  `list_id` → `throw redirect({ ..., search: { ...rest, segment_id: that, segments: undefined },
  replace: true })`. With a `list_id` the single segment stays a chip (the spec only redirects the
  « Tous » case implicitly; a list + one segment is a filter, not a view).
- Contacts route `staticData: { layout: 'rail' }`.
- `/lists` route: `beforeLoad` → `throw redirect({ to: '/console/workspace/$workspaceId/contacts',
  params, replace: true })`, `component` removed, `staticData: { layout: 'rail' }`.

`src/layouts/app-shell/menuItems.ts`: entry `lists` removed (and its icon import if unused);
`contacts` shown when `hasAccess('contacts') || hasAccess('lists')`; `selectedMenuKey` maps `/lists`
to `contacts`. Tests in `menuItems.test.ts` (and `AppSidebar.test.tsx` / `WorkspaceLayout.test.tsx`
if they assert the Lists entry).

### Page and its pieces

`src/pages/ContactsPage.tsx` (rewritten, the orchestrator):

- reads the search, derives `view` (`all` | `list` | `segment`), `tableSearch` = search without
  `contact`;
- `RailLayout` with `ContactsRail` as `rail`, main area = header for the view, list KPIs/settings bar
  or segment « Defined by » bar, then a flush `Panel` with `TableToolbar` and the table;
- permission gates per the spec's « États et droits » table, including `lists.read` only (first list
  opened by default, header and KPIs shown, table area « You don't have access to contacts »), and
  `list_id` without `lists.read` / `segment_id` without `segments.read` falling back to All;
- unknown `list_id` / `segment_id` (lists/segments loaded and id absent) → « This list no longer
  exists » / « This segment no longer exists » with a link to All contacts;
- mounts `ContactDetailsDrawer` once, open when `search.contact` is set;
- keeps: bulk selection + `BulkActionsBar` + `BulkActionProgressModal` + `useBulkContactAction`, the
  beforeunload guard, row menu Edit / Delete (`ContactUpsertDrawer`, `DeleteContactModal`), the
  optimistic delete (now on `['contacts', workspaceId, tableSearch]`), the selection reset on
  `filtersKey` (computed from `tableSearch` minus `limit`), column visibility in localStorage.
- view switch from the rail: `navigate` (push) with `list_id`/`segment_id` set and `limit`,
  `contact_list_status`, `segments` and `contact` emptied; `search` and the Filter chips are kept
  (spec: « en gardant les autres filtres »).
- search box: local state, debounced 300 ms, then `navigate({ replace: true })` with `search` and
  `limit` reset; « Load 50 more » `navigate({ replace: true })` with `limit + 50` while `next_cursor`
  and `rows < 10 000`; at 10 000 the message replaces the button.
- header per view (spec table): All = Import (`ImportContactsButton` semantics via
  `openImportDrawer`), Export, « New contact » (`ContactUpsertDrawer`), `…` menu with « Bulk update
  (CSV) » (`BulkUpdateDrawer` in controlled use, see below); List = title/description, « Import to
  list », Export, `…` « Delete list »; Segment = « Recompute » (Popconfirm, `segments.rebuild`),
  « Edit segment » (`UpsertSegmentButton` from `button_upsert.tsx` with the segment), `…` « Delete
  segment » (Popconfirm, `deleteSegment`).
- the Refresh button invalidates `contacts`, `lists`, `lists-with-counts`, `total-contacts`,
  `segments`, `list-stats` keys.

`src/components/contacts/useContactsTableQuery.ts` (new): `useContactsTableQuery(workspaceId,
tableSearch, { enabled })` — the current paging loop (100 per call up to `limit`, cap 10 000), key
`['contacts', workspaceId, tableSearch]`, `keepPreviousData`, `staleTime 5 s`, sends `search` and
`segments = segment_id ? [segment_id] : segments`, `with_contact_lists: true`. Exports
`toTableSearch(search)` (drops `contact`) and `contactsListParams(tableSearch)` (pure, for tests and
for the export modal).

`src/components/contacts/ContactsRail.tsx` (new): « All contacts » `RailItem` (`Users`,
`contacts.count`, hidden without `contacts.read`); « Lists » `RailSection` (`+` → `ListDrawer`
create, `lists.write`), one `RailItem` per list sorted by name, count = `active_count` from
`lists.list?with_counts=true` under key `['lists-with-counts', workspaceId]`, `staleTime 60 s`, an
empty count cell while loading; « Create your first list » row when there is none; « Segments »
`RailSection` (`+` → segment drawer create, `segments.write`), one item per segment sorted by name
with a colour dot, `users_count`, or a spinning `Loader2` with the progress tooltip when `building`
(`taskApi.findBySegmentId`, 15 s, as `SegmentsFilter` does today); Skeleton rows while loading;
section hidden without its read right. Items are `RailItem` buttons calling `onSelect(view)`.
The `lists-with-counts` key is invalidated only after list create/update/delete, an import to a
list, and the page Refresh (spec).

`src/components/contacts/SegmentProgress.tsx` (new, moved from `SegmentsFilter.tsx`): the progress
tooltip content and the hook `useSegmentBuildTask(workspaceId, segment)`; reused by the rail and the
segment header.

`src/components/lists/ListHeader.tsx` (new, replaces `ListStats.tsx`): `KpiStrip` (Active, Pending,
Unsubscribed, Bounced, Complained from `lists.stats`, refetched every 60 s, key
`['list-stats', workspaceId, listId]`), share of the five's total under Unsubscribed, Bounced,
Complained (`percent`, « — » when the total is 0); each cell `onClick` → same view with
`contact_list_status`, `active` when it matches; then the settings bar: `Pill` Public (tooltip
« Visible in the notification center ») or Private; `Pill` Double opt-in + « Template » + template
name (opens `TemplatePreviewDrawer`) + pencil (`CreateTemplateDrawer`, `canManageSavedBlocks`,
`templates.write`); « ID » + `Mono copy`; « Edit list » (`ListDrawer` edit; disabled with the current
tooltip without `lists.write`). Exports `useListStats` so the page reuses the counts for
`CountTabs` and the sheet's total.

`src/components/lists/DeleteListModal.tsx` (new, from `ListsPage.tsx`): ID-typing confirmation,
409 refusal with `listInUseError` / `listInUseMessage` (moved verbatim, same catalogue strings),
`onDeleted` callback (the page goes back to All and invalidates `lists`, `lists-with-counts`).

`src/components/lists/ListDrawer.tsx`: `CreateListDrawer` gains a controlled mode
(`open?`, `onClose?`): when `open` is provided no trigger button renders. On success it also
invalidates `['lists-with-counts', workspaceId]`.

`src/components/segment/segmentSummary.ts` (new): pure `segmentSummary(tree, i18n)` →
`{ parts: string[], joiner: 'and' | 'or', more: number }` with up to three first-level leaf
descriptions « field operator value » (field label from `TableSchemas` through `i18n._`), a node it
cannot describe (branch, custom events goal, timeline) → « 1 condition »; and
`components/contacts/SegmentDefinedBy.tsx` rendering the chips, the joiner, « +N », and « Updated »
+ relative `db_updated_at` in the segment timezone.

`src/components/contacts/ContactsTable.tsx` (new, columns moved out of the page): Contact column
(initials on `primary-50`, name or first+last, email in `ink-3`, fixed left); « Status in list » in
the list view (Pill + « since » relative date) or « Lists » elsewhere (Pills per list, tooltip as
today), hidden without `lists.read`; Segments (dots + tooltip « Matched on »), hidden without
`segments.read`; the selectable columns as today; « Added » (relative, full date in tooltip); row
`…` menu Edit / Delete. Row click → `onOpen(email)` unless the target is in the selection column or
the actions cell, or outside the row (portal). Checkbox selection only, disabled without
`contacts.write`.

`src/components/contacts/ExportContactsModal.tsx`: sends `search`, `segments` (from
`segment_id` or `segments`) and `full_name`; shows them in the active filters list.

`src/services/api/contacts.ts`: `ListContactsRequest.search`, `contactsApi.getByEmail`.
`src/services/api/list.ts`: `with_counts`, `List.active_count?`.
`src/services/api/contact_timeline.ts`: `entity_types?: string[]` sent comma-separated.
`src/services/api/web_analytics.ts`: `contactVisits`, `contactSessionPages` and their types (from
`internal/domain/web_analytics_contact_visits.go`).

### Contact sheet

`src/components/contacts/ContactDetailsDrawer.tsx` (rewritten): `FullDrawer` open when an email is
given, `backLabel` « Contacts », `nav` from `contactNavigation`. Opening pushes a history entry
(browser Back closes the sheet); J/K replace `contact`; the close and back buttons navigate (push)
with `contact` removed.
Body `ContactDetailsBody key={email}`. Props: `workspace`, `email`, `tableSearch`, `view`, `lists`,
`segments`, `permissions`, `onClose`, `onNavigate(email)`, `onLoadMore()`.

`src/components/contacts/contactNavigation.ts` (new, pure): `contactNavigation({ rows, email,
nextCursor, maxReached, exactTotal })` → `{ position?, total?, totalLabel?, prev?, next?,
needsMore }`: position from the loaded rows; first → no prev; last with `nextCursor` and not
`maxReached` → `needsMore`; last otherwise → no next; out of rows → no position (buttons hidden).
`total` = `exactTotal` when known, else rows length with `totalLabel` « N+ » when `nextCursor`.
And `exactViewTotal({ view, hasFilters, contactsCount, listStats, status, segment })`: All without
filter → `contacts.count`; list without filter → the tab's count or the sum of the five for All;
segment without filter → `users_count`; with a search or a chip → undefined.

J at the last loaded row with `needsMore`: the drawer calls `onLoadMore()` (limit + 50, replace) and
remembers `pendingNext = email`; when the rows grow past the current index it calls
`onNavigate(rows[index + 1])`. Meanwhile `context` shows « Loading… » and Next is disabled.

`ContactDetailsBody` loads the contact by `contacts.getByEmail` (key `['contact', workspaceId,
email]`); 404 → « Contact not found » in the drawer with Close; edits write through `contacts.upsert`
then `setQueryData` on that key and on the row in every `['contacts', workspaceId, …]` cache entry,
and refetch the All timeline.

Header (`surface`): Gravatar 40 px (initials fallback until it loads or on error), name (or email)
in `text-title`, email `Mono copy`, `MetaLine` with job title, country (+ state) and « Added on »
date; right: Refresh (refetch contact, timeline, messages, visits) and `…` « Delete contact »
(`DeleteContactModal`, `contacts.write`, closes the sheet and removes the row optimistically).

`src/components/contacts/ContactKpis.tsx` (new): five cells, hidden cells removed: Emails received
(`analytics.query` on `message_history`, measures `count_sent_emails`, `count_opened`,
`count_opens_tracked`, `count_clicked`, `count_clicks_tracked`, filter `contact_email equals email`;
second query adds `sent_at inDateRange [now−30d, now]` ISO instants, no time dimension), Open rate,
Click rate (« — » with a zero denominator), Last activity (first entry of the unfiltered timeline
query, relative time + short label), Site visits (sessions + median duration from the
`contactVisits` summary). Hidden without `message_history.read` (three first), without web
analytics enabled or `web_analytics.read` (last).

Body composition: main column with tabs (Activity, Messages N, Web visits N) and a 320 px aside on
`subtle` (`w-aside` token if it exists, otherwise `w-[320px]`), stacked under 1 200 px
(`min-[1200px]:` grid). Aside: Profile, Custom fields, Tracking consent, System, Lists, Segments.

`src/components/contacts/ContactProfileFields.tsx` (new): `KeyValueList` rows of
`InlineEditableField hideLabel`; filled first, empty ones behind « N empty fields » (plural);
custom fields with labels from `computeCustomFieldLabel`, labelled-but-empty custom fields first in
the folded group, « Manage » link to Settings custom fields; read-only without `contacts.write`
(`disabled`); timezone value shows « Europe/Paris · 14:32 ».

`src/components/contacts/InlineEditableField.tsx`: `hideLabel?: boolean`; with it, display and edit
modes render the value only (no grid, no label).

`src/components/contacts/ContactListsCard.tsx` (new): title + count, « Subscribe to list » (existing
modal, `lists.write`), one row per subscription (name, status `Pill` with the spec's tones,
« since » + date), `…` « Change status… » (existing modal) and « Remove from list »
(`contactListApi.removeContact` with Popconfirm), both under `lists.write` and `contacts.write`.
Segments card: dots with colour and « Matched on » tooltip in the workspace timezone.

`src/components/timeline/ContactTimeline.tsx`: unchanged rendering; the activity tab wrapper
`src/components/contacts/ContactActivityTab.tsx` (new) holds the `Segmented` (All, Emails, Lists &
segments, Profile, Events, Web), the timezone label on the right, and a `useInfiniteQuery` keyed
`['contact_timeline', workspaceId, email, types]` (a type change restarts without cursor). Emails
filter hidden without `message_history.read`, Web without web analytics or `web_analytics.read`.
Empty filtered result → « No activity of this kind ».

Messages tab: `MessageHistoryTable` with a `useInfiniteQuery` (same as today's accumulation); tab
count = `analytics.query` measure `count` on `message_history` filtered by the email.

`src/components/contacts/ContactWebVisits.tsx` (new): summary (first identified visit with source
label, utm_campaign and device; goals reached up to five « Upgrade · 1 · 290 »; top three pages),
`Segmented` All / With a goal / From an email with summary counts, sessions table 20 per page with
« Load more » (`useInfiniteQuery`, key `['contact-visits', ws, email, filter]`), expandable journey
(`contactSessionPages`, key per session, loaded on expand) with goals inserted at their time, footer
« Exited on » + « Open in web analytics » (the web analytics route's default tab — the spec's
`overview` is named `dashboard` in the router — last 30 days, `filters` with the contact email
encoded as the web analytics page does). Source label/icon from a
pure `sourceLabel(session, i18n)` in `src/components/contacts/webVisitSource.ts`. Device icon map.
Empty states per the spec.

### Removed

`src/pages/ListsPage.tsx`, `src/pages/ListsPage.test.tsx` (its 409 tests move to
`DeleteListModal.test.tsx`), `src/components/lists/ListStats.tsx`,
`src/components/contacts/SegmentsFilter.tsx` (progress moved to `SegmentProgress.tsx`).
`pages.smoke.test.tsx`: `ListsPage` import and case removed; the Contacts case renders All and a
list view.

### e2e

`e2e/features/lists.spec.ts`, `contacts.spec.ts`, `segments.spec.ts` rewritten for the rail (create,
edit, delete a list from Contacts; open a sheet, J and K; segment from the rail). The e2e suite is
not in CI; it is run locally if the dev server can start here, otherwise reported as not run.

### i18n and changelog (last, after rebasing on `dev`)

`npm run lingui:extract`, the new strings translated in every catalogue (`catalogues.test.ts`
requires them). CHANGELOG under the unreleased version: one short line (Lists joins Contacts; the
contact sheet opens full screen with navigation, inline editing and a Web visits tab).

## Tests (TDD: written before the code)

| Implementation file | Test file | Cases |
| --- | --- | --- |
| `layout/DrawerControls.tsx` | `layout/FullDrawer.test.tsx` | `totalLabel` replaces the total; `buttons: false` hides Previous/Next and the position, keeps context |
| `router.tsx` | `src/__tests__/contactsRoutes.test.tsx` (new; the spec named `webAnalyticsRoutes.test.tsx`, a separate file keeps concerns apart) | `/lists` redirects to `/contacts`; one `segments` value → `segment_id`; two kept; `limit` defaults and clamps; `list_id` + `segment_id` → list only; numeric `search` stays a string |
| `menuItems.ts` | `menuItems.test.ts` | no Lists entry; Contacts with `lists.read` only; `/lists` selects `contacts` |
| `useContactsTableQuery.ts` | `useContactsTableQuery.test.tsx` | key without `contact`; `segment_id` sent as `segments`; `search` sent; page and sheet share one fetch |
| `ContactsPage.tsx` | `ContactsPage.test.tsx` | All / list / segment from the URL; rail choice replaces the other view and resets limit, status, segments, contact; search debounced and sent; opening the sheet does not refetch `contacts.list` and keeps the selection; row click opens (replaces the old row-toggle test), checkbox selects; row menu Edit/Delete; Bulk update in the `…` menu; « Load 50 more »; 10 000 message; actions hidden without rights; `lists.read` only; unknown list / segment messages; empty states |
| `ContactsRail.tsx` | `ContactsRail.test.tsx` | counts shown; a bulk action's `lists` invalidation does not refetch `lists-with-counts`; building segment shows spinner, progress, 15 s refetch; sections hidden without read; `+` hidden without write; « Create your first list » |
| `ListHeader.tsx` | `ListHeader.test.tsx` | shares; zero total → « — »; KPI clicks set the status (Bounced, Complained included); Public / Private; template hidden without double opt-in; pencil hidden without `templates.write`; Edit list disabled without `lists.write` |
| `DeleteListModal.tsx` | `DeleteListModal.test.tsx` | ID confirmation; the 409 refusal cases from `ListsPage.test.tsx` |
| `ListDrawer.tsx` | `ListDrawer.test.tsx` | opens from the prop without a trigger; create; edit; close; invalidates `lists-with-counts` |
| `segmentSummary.ts` | `segmentSummary.test.ts` | one condition; three with « and »; « or »; five → three + « +2 »; unknown node → « 1 condition »; labels through `i18n._` |
| `ContactDetailsDrawer.tsx` | `ContactDetailsDrawer.test.tsx` | J/K and disabled bounds; J at the last row with `next_cursor` loads more then moves; J after « Load more activity » shows only the new contact's activity; `ann@x.com` vs `joann@x.com` opens and saves the right one (getByEmail); 404 → « Contact not found »; total per view and « 50+ » with a filter; navigation uses replace |
| `contactNavigation.ts` | `contactNavigation.test.ts` | every row of the spec's navigation table and of the total list |
| `ContactKpis.tsx` | `ContactKpis.test.tsx` | « — » without tracking; 30 days in `inDateRange` ISO; Last activity from the unfiltered query; cells hidden without rights or web analytics |
| `InlineEditableField.tsx` | `InlineEditableField.test.tsx` | label absent with `hideLabel`, present without |
| `ContactProfileFields.tsx` | `ContactProfileFields.test.tsx` | filled vs « N empty fields »; labelled empty custom fields first; read-only without write; timezone local time |
| `ContactListsCard.tsx` | `ContactListsCard.test.tsx` | status pills; Remove from list calls `removeContact`; actions hidden without both rights |
| `ContactActivityTab.tsx` | `ContactActivityTab.test.tsx` | each filter sends its `entity_types`; Emails and Web hidden without rights; filter change restarts without cursor; empty state |
| `ContactWebVisits.tsx`, `webVisitSource.ts` | `ContactWebVisits.test.tsx`, `webVisitSource.test.ts` | summary; source label per `channel_group` and unknown; tablet and other device; expand loads pages once; goal inserted at its time; web analytics link (tab, period, filter); empty states |
| `ExportContactsModal.tsx` | `ExportContactsModal.test.tsx` | export sends `search`, `segments` from `segment_id`, `full_name` |
| `services/api/*` | covered above, plus `web_analytics.test.ts` for the two new calls' URLs | — |
| `pages.smoke.test.tsx` | same | Contacts in All and list views |

## Verification

```bash
cd console && npx vitest run src/pages/ContactsPage.test.tsx src/components/contacts src/components/lists src/components/segment src/components/timeline src/components/layout src/layouts src/__tests__ src/services
cd console && npx tsc -b && npx eslint . && npm run build
cd console && npx vitest run   # full suite once before review
cd console && npx playwright test e2e/features/contacts.spec.ts e2e/features/lists.spec.ts e2e/features/segments.spec.ts --trace off   # if the dev server can run here
```

Screenshots of every touched screen at 1440 and 1280 px (All, list, segment, sheet Activity,
Messages, Web visits), with the dev server on port 19173 against mocked API responses, stopped after.

## Plan review (Opus, effort xhigh) — findings integrated

The plan above was reviewed against `dev` 9f07fc6 and the merged server code. The confirmed findings
amend it as follows; where this section and the text above differ, this section wins.

1. **Legacy segments chip.** Several `segments` show as one removable chip « In any of: A, B »
   (names from `segments.list`); removing it clears every `segments` value at once (removing one of
   two would trigger the single-segment redirect). `list_id`, `contact_list_status` and `segments`
   leave the « Filter by… » fields. Tests in `ContactsPage.test.tsx`.
2. **Controlled drawers.** `BulkUpdateDrawer` gains `open?`/`onClose?` (no trigger when controlled),
   test « opens from the prop without a trigger » in `BulkUpdateDrawer.test.tsx`. The rail's « New
   segment » and the header's « Edit segment » use the exported `DrawerSegment` from
   `button_upsert.tsx`, test in `button_upsert.test.tsx`.
3. **No key shared between `useQuery` and `useInfiniteQuery`.** « Last activity » and the Web visits
   counts read `data.pages[0]` of the same infinite queries the tabs use (same key and options): the
   `contactVisits` summary only comes with the first page.
4. **Opening the sheet does not refetch.** The drawer calls `useContactsTableQuery` unconditionally at
   its top, with the page's `enabled`; the test advances time past the 5 s `staleTime`.
5. **J at the last row.** `pendingNext` resolves once the query is neither placeholder nor fetching,
   looking the email up again in the new rows (they can shift); it is cleared on error, on K and on
   close; with no new row Next is disabled. `onLoadMore` and `onNavigate` use functional
   `search: (prev) => …` updates.
6. **Import refresh.** `ContactsCsvUploadProvider`'s success also invalidates
   `['lists-with-counts', ws]`, test in its test file.
7. **Delete from the sheet** goes through the page's delete (`onDelete(email)`): the sheet closes, the
   row disappears and the selection drops the email.
8. **Gates.** Messages tab hidden without `message_history.read`; Web visits tab without
   `workspace.settings.web_analytics?.enabled` or `web_analytics.read`; Lists card without
   `lists.read`, Segments card without `segments.read`; Export hidden without `contacts.read`;
   neither `contacts.read` nor `lists.read` → « You don't have access to contacts ». All tested.
9. **Rights load first.** The fallbacks (`list_id` without `lists.read`, `segment_id` without
   `segments.read`, `lists.read` only → first list) are derived during render, never navigated; while
   `useWorkspacePermissions` loads the page shows a skeleton. The `total-contacts`, `segments` and
   `lists` queries are `enabled` on their read right.
10. **States.** Ten Skeleton rows on first load; « Could not load » + Retry per block (table, list
    KPIs, sheet); « Computing this segment… » while building, and the table refetches when the
    segment turns active; empty texts « No contacts yet », « No contacts in this list yet », « No
    contacts match this segment », « No contacts match your filters » + Clear filters. Each tested.
11. **Segment labels** go through a `msg` descriptor map (TableSchemas titles are plain strings). A
    contacts leaf with one filter is described; several filters, or any other node, → « 1 condition »;
    a contact_lists leaf → « In list X » / « Not in list X » with the list name.
12. **Existing tests updated:** `src/__tests__/routeLayout.test.ts` (Contacts and Lists declare
    `rail`), `e2e/protected-pages.spec.ts` (header and empty text), and the e2e fixtures gain mocks for
    `contacts.getByEmail`, `timeline.list`, `webAnalytics.contactVisits` and `analytics.query`.
13. **More test files:** `ContactsTable.test.tsx` (row click vs checkbox, actions cell and portal,
    Status in list, columns hidden without rights, and the null/absent `contact_lists` regression
    tests moved from `ContactsPage.test.tsx`), `SegmentProgress.test.tsx`, `SegmentDefinedBy.test.tsx`,
    the Messages tab (count and gate), `BulkUpdateDrawer.test.tsx`, `button_upsert.test.tsx`,
    `ContactsCsvUploadProvider.test.tsx`.
14. **Docs PR** on `Notifuse/docs` from `dev` (branch `docs/contacts-lists`): `features/lists.mdx`
    (lists live in Contacts), the rights section of `features/contacts.mdx`, a paragraph on the Web
    visits tab in `web-analytics/identifying-visitors.mdx`.
15. **Segments key** `['segments', ws, { with_count: true }]` (Automations fills `['segments', ws]`
    without counts); prefix invalidations still match.
16. `contactsApi.getByEmail` already exists; only `search` is new. `UpsertContactOperation` gains the
    stored `contact`, used for `setQueryData`. Edits invalidate the `['contact_timeline', ws, email]`
    prefix. The aside uses `w-aside`.
17. **Deviations to confirm with Pierre:** tooltips « Previous (K) »; J shows « Loading… » in the
    context; list + one segment stays a chip; controls hidden (not disabled) without `contacts.write`,
    as the spec says.
18. Screenshots at **1100, 1280 and 1440** px (1100 shows the stacked aside).
19. The opt-in template name query is gated on `templates.read`, falling back to the template ID.
20. After deleting a list or a segment the page goes to All with `replace` and invalidates `contacts`.
    The export pages with its own params (`with_contact_lists: false`, no `limit`). The Messages
    tab's « Journey » column and « Open in Logs » depend on the Logs spec: out of scope.

Code review: Fable, effort high, on the last commit (Opus, effort high, only for a simple delta).
