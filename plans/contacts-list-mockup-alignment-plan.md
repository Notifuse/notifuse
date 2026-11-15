# Plan — contacts list aligned with the Nuit claire mockups (41 to 45)

## Need

The review of the console against the mockups found the contacts page « with deviations » on
screens 41 (all contacts), 42 (an open segment), 44 (selection), « off-topic » on 43 (adding a
filter) and « not done » on 45 (the « … » menu). Pierre asked for every unwanted deviation to be
fixed, screen by screen; the contact sheet is PR #579, this PR is the contacts list. It builds on
#579, which already gives `volcano` / `geekblue` segments their dot in the table (`segmentDotColor`).
Console only: no Go file, no SQL, no environment variable.

Out of scope: the sidebar and the top bar (Pierre redid them since the mockups), the segment form and
its conditions engine and the list form (screen 47), which Pierre said not to touch, and the list
header (screen 46), left to another PR.

Known deviations, because the mockup shows something the server cannot do today. Each one goes in the
PR, and none adds an input or a button that would do nothing:

- « Select all N » and « Export » in the selection bar: bulk actions run in the browser on the
  selected emails, and the export takes the view's filters (and is audited), not a list of emails.
- « Save as segment », the « Recent first » sort and the « Timezone » filter: no API behind them.
- « Import ▾ » with an import history: the audit log records imports per batch, not per file.
- The « Subscriptions » filter group: the list is the view, chosen in the rail (`list_id` is the
  view, and the e2e pins it). The « Custom fields » group: `contacts.list` has no custom-field filter.
- A pill with several values (« Country is France, Belgium »): each filter takes one value.
- The Company, Plan and MRR columns: custom fields the demo does not name.
- « Copy the emails of the view »: left out, Pierre's decision of 15/11. The mockup added it with no
  spec, and the CSV export already does it with an audit trail, which a copy would not leave.

## Changes

1. **Denser rows** (`ContactsTable`): small table size, 24 px avatar, name and email on two tight
   lines (about 44 px a row instead of 60). Lists show the first two memberships and « +N »,
   segments the first one and « +N », sorted by name (the server loads them with no `ORDER BY`). « +N » names the rest in a tooltip and
   in visually hidden text. Long names are truncated with an ellipsis.
2. **Count next to the title** (`ContactsViewHeader`, through `PageHeader`'s `count`): the same
   the view's own total, without the row filters nor the status tab (workspace total, list
   statuses summed, `users_count`): it stays the same while a search, a filter or a tab applies.
   `exactViewTotal` keeps feeding the footer and the sheet. No badge while it is unknown, nor while the segment is being computed (as the rail
   does). The all-contacts title reads « All contacts », as the rail and the mockup.
3. **Open segment** (`ContactsViewHeader`, `SegmentDefinedBy`):
   - the title gets the segment's colour dot;
   - Recompute (with its confirmation) and Edit segment move from the header into the « Defined by »
     bar, on the right, behind segments write; the header keeps Export and « … » (Delete segment);
   - `segmentSummary` describes more conditions instead of « 1 condition », and never a broader one
     than the segment computes:
     - a contact leaf with several filters, joined by « and »;
     - an activity: « Opened an email », « … at least 3 times », « · in the last 30 days » (own
       wording, in the contact's past tense; the form's « Open email » labels are imperative);
       « (filtered) » when it also narrows by template, broadcast, link or properties;
     - a goal: « Purchase: total ≥ 800 », the goal name and the event name in quotes after the type, both when both are set (the server ANDs them)
       (« Any goal » for `*`), « Not (…) » when negated, « (filtered) » with property filters, and
       the timeframe;
     - a list condition with a status: « In list Newsletter (Unsubscribed) »;
     - a nested group: « N conditions »; a JSON filter still reads « 1 condition ».
4. **Adding a filter** (`ContactsFilters`, replaces `components/filters/Filter`): « Filter » opens a
   picker with a search box (focused on open), the fields as buttons grouped Identity (email,
   external ID, first, last and full name, phone), Location (country, language), Tracking (opens and
   clicks consent), and with segments write « Advanced conditions: create a segment », which opens
   the segment drawer. Choosing a field shows its input (`FilterInputs`, prefilled with the active
   value) with Back (focus returns to the list) and Apply (disabled while empty; Enter applies in a
   text input). Applying pushes a history entry through the router, resets `limit` and keeps the
   view, the search and the other filters, as today.
5. **Active filters as pills**: the label is a button that reopens the picker on that field, and a
   separate ✕ button removes it (« Remove filter Country »), replacing the entry. The operator says
   what the server does: « contains » for every field (`ILIKE '%…%'`, country and language included),
   « is » for the two tracking consents (exact match). The pills reuse the translated
   `{field} contains {value}` and `{field} is {value}`. The legacy `segments` pill (« In any of: … »)
   stays, removed as a whole. « Clear » stays, also shown for a search alone.
6. **Selection in the table card** (`BulkActionsBar`): with rows selected, the bar takes the
   toolbar's place inside the card (the list status tabs come back with the toolbar): « 3 selected »
   in a live region, Add to list, Remove from list, Unsubscribe from list, Delete as text buttons with
   their icons, and a ✕ « Clear selection » that hands focus to the search box. The modals, the
   tooltips and the « a bulk action is running » state are kept, without the gradient frame.
   **Fix**: Remove from list and Unsubscribe from list also need lists write, as the server does
   (`contact_list_service.go`); today a member without it launches them and every row fails.
7. **« … » menu** (`ContactsViewHeader`): Bulk update (CSV) with contacts write (its drawer mounted in
   every view), a divider only between two groups, Displayed columns… with contacts read (opens the
   column chooser in a modal: the toolbar icon is hidden during a selection), Manage custom fields for
   an owner or with workspace write (Settings › Custom fields, as the settings page decides). In a
   list or segment view, Delete list / Delete segment stays last. Export gets the tooltip « Exports
   the contacts of this view, filters included ».
8. **Column chooser** (`ContactColumnsSelector`): its checklist is shared by the toolbar popover and
   the modal; the icon button gets a label; it no longer writes `localStorage` itself (the page
   already does, guarded); tokens instead of `gray-*`.
9. **Footer** (`ContactsPage`): « 10 of 1,288 contacts » on the left when the total is known and not
   below the rows shown, « 50+ contacts » when rows remain and the total is unknown, else « 10
   contacts », with « Load 50 more » on the right. The 10,000-row cap message replaces the button, as
   today.

## Files and tests

| File | Change | Test file and cases |
| --- | --- | --- |
| `components/contacts/ContactsTable.tsx` | §1 | `ContactsTable.test.tsx`: 3 lists show 2 and « +1 » naming the third; 2 segments show 1 and « +1 »; small size |
| `components/segment/segmentSummary.ts` | §3 | `segmentSummary.test.ts`: multi-filter leaf; activity counts, timeframe, « (filtered) »; goal with name, negation, between, filters; list status; nested group; JSON still « 1 condition » |
| `components/contacts/SegmentDefinedBy.tsx` | §3 buttons | `SegmentDefinedBy.test.tsx`: buttons with segments write only; Recompute confirms then rebuilds; Edit opens the drawer with the segment |
| `components/contacts/ContactsViewHeader.tsx` | §2, §3, §7 (`role` is a required prop, so the page cannot forget it) | new `ContactsViewHeader.test.tsx`: badge for all, list and segment, none while unknown or building; dot in a segment title; no Recompute in the header; menu items gated by rights (owner without explicit workspace write sees Manage custom fields); Bulk update in a list view; Delete last; Displayed columns opens the chooser |
| `components/contacts/ContactsFilters.tsx` | §4, §5 | new `ContactsFilters.test.tsx`: search narrows the fields; Country then a value then Apply calls `onApply('country', …)`; Apply disabled when empty; Enter applies; Back; reopening a pill prefills; « Email contains acme », « Opens tracking consent is Granted »; ✕ removes; legacy segments pill; Advanced conditions only with segments write |
| `components/filters/Filter.tsx` | removed (only this page used it) | — |
| `components/filters/FilterInputs.tsx` | `onPressEnter` on the text and number inputs | covered by `ContactsFilters.test.tsx` |
| `components/contacts/BulkActionsBar.tsx` | §6 and the fix | new `BulkActionsBar.test.tsx`: count singular/plural; picker then Confirm calls the right handler; Delete needs the typed count; Remove and Unsubscribe disabled without lists write; all disabled while running; ✕ clears |
| `components/contacts/ContactColumnsSelector.tsx` | §8 | `ContactsViewHeader.test.tsx` (modal) and `ContactsPage.test.tsx` (toolbar) |
| `pages/ContactsPage.tsx` | §6, §9, filters through the router | `ContactsPage.test.tsx`: updated cases (« 1 selected », the filter flow); applying pushes with `limit` reset and keeps `list_id` and `search`; ✕ brings back the toolbar and the status tabs; « 50+ contacts », also when a segment count lags behind the rows; the singular; the title badge unchanged by a status tab or a search; the cap message |
| `e2e/features/contacts.spec.ts` | the new filter flow | the two « Filtering & Search » cases |
| `e2e/features/segments.spec.ts` | « Edit segment » moves to the Defined by bar | the comment of `openSegmentEditor` |
| `CHANGELOG.md` | the 44.0 contacts bullet extended; one `Fix` line for §6 | — |
| `i18n/locales/*.po` | new strings in the 7 locales | `catalogues.test.ts` |

## Checks

1. `cd console && npx tsc -b && npm run lint`.
2. The changed test files with `npx vitest run <files>`, then `npx vitest run` once.
3. `npx lingui extract` (without `--clean`), every new string translated in the 7 locales, a second
   extract leaves `src/i18n` unchanged.
4. `npm run build`, the contacts e2e if the environment allows it (the e2e do not run in CI: the PR says whether they ran), then screenshots at 1440 and
   1280 px with the demo data, compared with mockups 41 to 45; the deviations go in the PR.
