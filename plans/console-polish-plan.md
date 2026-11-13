# Console polish: Logs leftovers, blog footer, lint at zero warnings

The points left aside by the Logs L2 PR (#560, "Known, not handled here"), and the 32 lint warnings that
keep `npm run lint` from passing, so that CI can run it. Console only; no API, SQL or version change.

## Changes

1. **Audit log Refresh asks once.** `AuditLogTab.refresh` moves the clock and resets every
   `['audit-logs']` query. When the minute changed, the moved clock gives a new key, which fetches,
   and the reset also refetches the old key still on screen: two requests. Same rule as
   `MessagesTab`: build the params at the new time; when they equal the current ones (all time, a
   custom range, the same minute), reset the current key; otherwise only move the clock.
2. **Blog posts footer spread across the panel.** `Panel` lays its footer's children out with
   `justify-between`; `PostsTable` wraps them in one `div`, so the count and Load more sit together
   on the left. Pass them as a fragment, as `BroadcastsPage` and `AutomationsPage` do.
3. **Translated template error.** `MessageDetail` throws `new Error('Failed to load template')` when
   `templates.get` answers without a template, and the message is shown as the description of the
   "Couldn't load the template" alert. Throw a translated message instead
   (`t\`The server returned no template\``). `MessageHistoryTable` throws the same text under the
   same query key, so it throws the same translated message.
4. **Counters' tooltip follows `COUNT_MAX_PERIOD`.** The tooltip says "7 days" in the text. `period.ts`
   gains `type CountedPeriod = Exclude<LivePeriod, 'all'>` and types `COUNT_MAX_PERIOD` with it
   (`COUNTED_PERIODS` stays `readonly LivePeriod[]`). The tab builds a `Record<CountedPeriod, string>`
   of durations with `t` (1 hour, 24 hours, 7 days, 30 days) and the tooltip reads
   `t\`Counts cover ${duration} at most. …\``, so the text follows the constant.
5. **Accessibility.**
   - `MessageColumnsSelector`: the icon-only button gets `aria-label={t\`Select columns\`}`, the
     text of its tooltip.
   - `LogsSearchBox`: the "Enter a full email address" hint gets an id (`useId`) and the input
     `aria-describedby` it while it shows. The input is the direct child of the "Exact match"
     `Tooltip`, which clones it with its own `aria-describedby` (the tooltip's id while open, else
     undefined) and so overwrites the input's. A small `forwardRef` wrapper between the two joins the
     tooltip's id and the hint's, and passes every other prop and the ref through. The Tooltip stays
     rendered (a conditional one would remount the input and drop its focus).
   - Links with `href="#"`: the three Sent by links of `MessageDetail` (`SourceLink`: newsletter,
     automation, notification) and the audit tab's "Change in Settings" become router `Link`s with
     their real `href`, so they open in a new tab and read as links.
6. **Lint at zero warnings, and CI runs `npm run lint`.**
   - Each `react-refresh/only-export-components` warning is a helper, constant or hook exported
     from a component file. It moves to a sibling module, named after what it holds, and its
     importers (code and tests) import it from there. No behaviour change. Per file:

     | Component file | Moves out | To |
     |---|---|---|
     | `broadcasts/BroadcastKpis.tsx` | `rate` | no importer elsewhere: `export` removed |
     | `broadcasts/BroadcastsTable.tsx` | `shownVariation` | no importer elsewhere: `export` removed |
     | `broadcasts/compose/AdvancedRow.tsx` | `feedState` | `broadcasts/compose/feedState.ts` |
     | `broadcasts/compose/BroadcastCompose.tsx` | `EDITABLE_STATUSES` | no importer elsewhere: `export` removed |
     | `broadcasts/compose/SendSummary.tsx` | `summaryChecks`, `errorCount`, `useCheckText`, with the types `CheckLevel`, `SummaryCheck`, `SummaryInput` | `broadcasts/compose/sendSummaryChecks.tsx` (`useCheckText` holds JSX) |
     | `broadcasts/detail/AbTestPanel.tsx` | `asTemplate` | `broadcasts/detail/asTemplate.ts` |
     | `broadcasts/detail/BroadcastAside.tsx` | `previewVariationOf` | `broadcasts/detail/previewVariation.ts` |
     | `broadcasts/detail/EngagementChart.tsx` | `buildChartOption` (and `PERIODS`, which has no importer elsewhere and stays, unexported, if only the component reads it) | `broadcasts/detail/engagementChartOption.ts` |
     | `broadcasts/detail/TemplatePreviewFrame.tsx` | `useTemplatePreview` | `broadcasts/detail/useTemplatePreview.ts` |
     | `broadcasts/detail/VariationSender.tsx` | `resolveSenderOf`, `senderKey`, `useSenderText` | `broadcasts/detail/senderText.ts` (not `variationSender.ts`: it would differ from `VariationSender.tsx` only by case, and `'./VariationSender'` would resolve to it on a case-insensitive disk) |
     | `contacts/ContactsFilters.tsx` | `useContactFilterFields` (`activeContactFilters` has no importer elsewhere: `export` removed) | `contacts/contactFilterFields.ts` |
     | `contacts/ContactsTable.tsx` | `DEFAULT_VISIBLE_COLUMNS`, `useSelectableColumns` | `contacts/contactColumns.ts` |
     | `contacts/SegmentProgress.tsx` | `useSegmentBuildTask` | no importer elsewhere: `export` removed |
     | `contacts/TrackingConsent.tsx` | `useTrackingConsentOptions` (`useTrackingConsentSourceLabel` has no importer elsewhere: `export` removed) | `contacts/trackingConsentOptions.ts` |
     | `contacts/sheet/ContactDetailsBody.tsx` | `contactKey` | `contacts/sheet/contactKey.ts` |
     | `contacts/visits/ContactWebVisits.tsx` | `webAnalyticsContactSearch` | `contacts/visits/webAnalyticsContactSearch.ts` |
     | `lists/ListHeader.tsx` | `LIST_STATUSES`, `LIST_STAT_KEYS`, `useListStats` | `lists/listStats.ts` |
     | `web_analytics/WebAnalyticsAIAssistant.tsx` | `shouldHideAssistant` | `web_analytics/shouldHideAssistant.ts` |
     | `web_analytics/context.tsx` | `useWebAnalytics` (and the context object it reads, with the `WebAnalyticsContextValue` interface, now exported) | `web_analytics/useWebAnalytics.ts`, context object in `web_analytics/webAnalyticsContext.ts`; every importer of `useWebAnalytics` changes, and the six tests that `vi.mock('./context')` for it mock `useWebAnalytics` instead (toolbar, FilterBuilder, InstallOverlay, WebAnalyticsAIAssistant, AnnotationsTab, installStatus.hook) |
     | `web_analytics/explore/BreakdownDrawer.tsx` | `buildBreakdownQuery` | `web_analytics/explore/breakdownQuery.ts` |
     | `web_analytics/toolbar.tsx` | `usePeriodLabels` | `web_analytics/usePeriodLabels.ts` |

     A re-export from the component file would still trip the rule, so importers change. A helper
     whose own unit tests live in the component's test file keeps them there, importing from the new
     module; no test is dropped. A `vi.mock` of a moved export follows it (`BroadcastCompose.test.tsx`
     mocks `useTemplatePreview` on `TemplatePreviewFrame`: it moves to `../detail/useTemplatePreview`).
   - `web_analytics/TrafficHeatmapWidget.tsx`: the `dayLabels` memo depends on `i18n.locale`, an
     outer-scope value for the rule. Read the locale into a local `const locale = i18n.locale` and
     depend on `locale`. The `hourLabels` memo keeps its disable comment (its callback does not read
     the locale it depends on), with the reason updated.
   - `.github/workflows/console.yml`: the Lint step runs `npm run lint` and its comment goes.

## Tests (TDD: each test written red first, except pure moves, which existing tests cover)

| File | Test file | Cases |
|---|---|---|
| `logs/AuditLogTab.tsx` | `AuditLogTab.test.tsx` | Refresh minutes later: exactly one more request (red first); that the window moves (a different `from`) is pinned by the existing test above it. Guards, green today: Refresh in the same minute, on all time and on a custom range each ask once more. Clock pinned with `vi.spyOn(Date, 'now')` at a minute start; counts asserted once requests have settled |
| `blog/PostsTable.tsx` | `PostsTable.test.tsx` | The footer's count and Load more are direct children of `[data-panel-footer]`; the count span has no `text-sm` |
| `logs/MessageDetail.tsx` | `MessageDetail.test.tsx` | `templates.get` answering without a template: the alert's description is the French text of "The server returned no template" when that translation is loaded (timeout 4000: the query retries once) |
| `messages/MessageHistoryTable.tsx` | `MessageHistoryTable.test.tsx` | — (the message is not shown there; the change is the same translated text) |
| `logs/MessagesTab.tsx`, `logs/period.ts` | `MessagesTab.test.tsx` | Existing tooltip test keeps "Counts cover 7 days at most"; with `COUNT_MAX_PERIOD` mocked to `'24h'` through a getter, on all time, the tooltip reads "Counts cover 24 hours at most" |
| `messages/MessageColumnsSelector.tsx` | `MessageColumnsSelector.test.tsx` (new) | The button is found by role `button`, name "Select columns" |
| `logs/LogsSearchBox.tsx` | `LogsSearchBox.test.tsx` | Incomplete email: the input's `aria-describedby` holds the hint's id (red first), and also the tooltip's id while the tooltip is open; ids read with `getElementById` |
| `logs/MessageDetail.tsx` | `MessageDetail.test.tsx` | The router mock gains a `Link` stub that builds `href` from `to`, `params` and `search` (as in `InboundWebhooksTab.test.tsx`). The newsletter, automation and notification Sent by links have their real `href`; the existing click-and-navigate test becomes an `href` test |
| `logs/AuditLogTab.tsx` | `AuditLogTab.test.tsx` | Same `Link` stub; "Change in Settings" has `href` `…/settings/audit-logs`; the existing click-and-navigate test becomes an `href` test |
| Moved modules (`feedState.ts`, `sendSummaryChecks.tsx`, `asTemplate.ts`, `previewVariation.ts`, `engagementChartOption.ts`, `useTemplatePreview.ts`, `senderText.ts`, `contactFilterFields.ts`, `contactColumns.ts`, `trackingConsentOptions.ts`, `contactKey.ts`, `webAnalyticsContactSearch.ts`, `listStats.ts`, `shouldHideAssistant.ts`, `useWebAnalytics.ts`, `webAnalyticsContext.ts`, `breakdownQuery.ts`, `usePeriodLabels.ts`) | the existing tests that cover each export: `cards.test.tsx`, `SendSummary.test.tsx`, `BroadcastAside.test.tsx`, `BroadcastDetail.test.tsx`, `EngagementChart.test.tsx`, `PreviewPanel` and `BroadcastCompose.test.tsx`, `ContactsTable.test.tsx`, `ContactsPage.test.tsx`, `ContactDetailsBody.test.tsx`, `ContactWebVisits.test.tsx`, `WebAnalyticsAIAssistant.test.tsx`, the web analytics tests mocking `useWebAnalytics`, `ExploreTab` tests | unchanged assertions; imports and `vi.mock` paths follow the move (pure moves: no red step) |
| `web_analytics/TrafficHeatmapWidget.tsx` | — | Lint-only change of a memo's dependency name; covered by `npm run lint` (no test file exists) |

## Verification

New strings ("The server returned no template", the four durations, the new tooltip) are
extracted and translated in the seven other locales before the tests run:
`src/i18n/locales/catalogues.test.ts` fails on an untranslated message. 44.0 is unreleased and its
CHANGELOG already describes the Logs and the blog, so no CHANGELOG line.

```bash
cd console
npm run lingui:extract   # then translate the new entries in ca, de, es, fr, it, ja, pt-BR
npx vitest run src/components/logs src/components/blog src/components/messages \
  src/components/broadcasts src/components/contacts src/components/lists src/components/web_analytics \
  src/i18n src/pages src/lib
npx vitest run --maxWorkers=3
npx tsc -b
npm run lint          # must exit 0
NODE_OPTIONS=--max-old-space-size=6144 npm run build
```
