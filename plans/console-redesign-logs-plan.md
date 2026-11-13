# Console redesign — Logs screens (spec 8)

Source spec: "Spec 8 · Logs" (Nuit claire redesign, screens 101 messages, 102 message detail, 103 incoming
webhooks, 104 audit log). This plan implements its console part.

## What already shipped on `dev`, and what is still open

- #531: layout components (`PageHeader`, `Panel`, `Pill`, `Mono`, `KeyValueList`, `TableToolbar`,
  `SidePanel`, `useDrawerNavKeys`, `components/layout/format.ts`, `lib/copyToClipboard.ts`).
- #543 (spec 7): `lib/messageStatus.ts`, the `transactional_notification_id` filter in the current
  messages tab and in `services/api/messages_history.ts`; links from the transactional screen to
  `logs?tab=messages&transactional_notification_id=…` and `…&id=…&message=…`.
- #546 (spec 6): `automation_id` in `MessageListParams` (console) and the Automations page link
  `logs?automation_id=…`.
- Server defects thread: `webhookSubscriptions.deliveries` reads `status`, `inboundWebhookEvents.list` accepts
  `auth_email` and `before_user_created`. `webhookSubscriptionApi.getDeliveries` already takes `status`, and
  `openapi/paths/webhook-subscriptions.yaml` documents it. The CHANGELOG already has the four `Fix` lines of
  the spec (deliveries status, Auth email / User created, webhooks tab in a link, View more on the failed
  messages card).
- Dashboard: the failed messages card is `components/analytics/RecentFailures.tsx` (`FailedMessagesTable.tsx`
  no longer exists); it already writes `is_failed: true`.
- **Open**: #535 (server part 5): `origin`, `created_after`, `created_before` on `messages.list`, the new
  `messages.count`, `source` on `inboundWebhookEvents.list`. Draft, waiting for review.

## Decisions taken while reading the code

1. **Two console PRs instead of one** (declared difference). The audit log and the outgoing webhooks need
   nothing from #535; messages, the message detail and incoming webhooks do.
   - **PR L1 (now)**: page shell (header, tabs by right, first permitted tab, tab change keeps only `tab`),
     `parseLogsSearch` in the route, the menu entry, `period.ts`, `FilterChip`, `labels.ts`, the
     Outgoing webhooks tab, the Audit log tab with its phrases and the Go test, the e2e subtitle,
     translations, the first half of the CHANGELOG line, and the docs of the audit log.
   - **PR L2 (after #535 is on `dev`)**: Messages tab, message detail, Incoming webhooks tab, the API
     additions, `/api/messages.count` in the permissions catalogue, the `RecentFailures` link, the e2e
     routes of `messages.count`, removal of `MessageHistoryTab` and `components/webhooks/*`, the rest of the
     CHANGELOG line and of the docs, and the `EXPLAIN (ANALYZE, BUFFERS)` measure of `messages.count` (#535
     leaves it to the console PR) that sets `COUNT_MAX_PERIOD`.
   - In L1 the Messages and Incoming webhooks tabs keep today's components under the new labels. The old
     messages tab gains `automation_id` in its `filterKeys` (one line), so the Automations page link filters
     until L2 replaces the tab.
2. **Menu entry**: it lives in `layouts/app-shell/menuItems.ts` (lines 140 and 344), not
   `WorkspaceLayout.tsx:467`. `BuildMenuItemsOptions` gains an optional `canOpenLogs`; `WorkspaceLayout`
   computes it as `message_history.read || webhook_events.read || webhook_subscriptions.read ||
   useAuditLogAccess(workspaceId).canRead` (read, as the spec says: a write-only right would open a 403). Without
   the option the entry keeps today's rule, so existing calls and tests are unchanged. The landing rule
   `ENTRIES_AFTER_DASHBOARD` keeps `message_history` (known, not handled: a member with only another logs
   right does not land on Logs).
3. **Tabs by right**: `message_history.read`, `webhook_events.read`, `webhook_subscriptions.read`,
   `useAuditLogAccess().canRead`. The rights come from `useWorkspacePermissions` (`permissions`, `loading`).
   While either loads: a `Skeleton` and no URL write. With none of the four: `Result` 403 "You don't have
   access to logs".
   - **Missing `tab`**: the first permitted tab shows, and the URL is left as is, so a link without `tab`
     (Automations, broadcast link stats, automation node panel) keeps its filters.
   - **Unpermitted or unknown `tab`**: replaced (`replace`) by `{ tab: first }` only, so an audit `id` never
     becomes a message search.
   - **Tab change**: `push` of `{ tab }` only.
   - antd `Tabs` with `destroyOnHidden`: a tab left and reopened remounts from the URL, so the old tabs of L1
     never show a filter the URL no longer has.
4. **URL**: `components/logs/logsSearch.ts` holds `LogsSearch` (every key of the four tabs, each optional),
   the closed lists (tabs, periods per tab, origins, channels, sources, event types including `reply` and
   `auto_reply`, delivery statuses, outcomes, scopes, actor types) and `parseLogsSearch`, built on
   `searchString` (`lib/searchParams.ts`). `period` is checked against the list of the URL's tab (messages and
   incoming: `1h` to `30d` and `all`; audit: `24h` to `90d`, `all`, `custom`). The route's `validateSearch`
   calls it. L1 writes every key of the spec now, so L2 does not touch the router. `LOGS_TAB` gains `audit`;
   `notificationRules.ts`' `logsSearch()` writes `tab: LOGS_TAB.messages` (one line), since a typed `tab`
   refuses the `string` it infers today.
5. **Period**: `components/logs/period.ts` — `periodBounds(period, now, custom?)` gives `from` (rounded down
   to the minute) and, for `custom`, `to`; `defaultMessagesPeriod(search)` and `defaultInboundPeriod(search)`
   (`all` with an indexed entity filter, `24h` otherwise), `defaultAuditPeriod(search)` (`all` with
   `target_id`, `7d` otherwise); `COUNT_MAX_PERIOD` (L2). **`now` is state** of each tab
   (`useState(() => Date.now())`), reset on a filter change and on Refresh: a re-render after a minute
   boundary never changes the query key nor drops the loaded pages.
6. **Time column**: `components/logs/logTime.ts` — `formatLogTime(date, timezone, now)` ("14:34:02"
   today, "Yesterday 18:40", "12 Nov, 18:40"), with the console's dayjs locale (#545). New code; the date
   formats of `MessageHistoryTable.tsx` and `utils/formatters.tsx` are left to the dayjs thread.
7. **`lib/format.ts`** of the spec is `components/layout/format.ts` (`useNumberFormat().int`).
8. **Local-state details** (outgoing and incoming webhooks) pass `copyLink={false}` to `SidePanel`: their
   detail has no URL, so a copied link would not reopen it.
9. **Outgoing webhooks "Open subscription"**: `settings/webhooks?webhook={id}` (spec 9 is on `dev`,
   `parseSettingsSearch` reads `webhook`).
10. **Audit actor list**: today's `workspaces.members` query of `useAuditLogAccess` (key
   `['workspace-members', workspaceId]`); for a root, the tab runs the same query itself in workspace view.
11. **Audit phrases**: `components/logs/auditPhrases.ts` holds `AUDIT_PHRASES`, one
   `msg({ message: '…' })` per action of `AuditActionCatalogue()`, one per line. The render is a separate
   `components/logs/AuditPhrase.tsx` (`Trans` from `@lingui/react` with the descriptor,
   `values={{ target }}`, `components={{ 0: <strong /> }}`, as `TriggerText.tsx` does), so the `.ts` file
   has no JSX and the `.tsx` exports only a component. The Go test's key pattern is
   `[A-Za-z]+(?:\.[A-Za-z]+)+` (declared difference: the spec's `[A-Za-z]+\.[A-Za-z]+` misses
   `user.oidc.callback` and `user.oidc.exchange`).
12. **Licence notice**: `LicenceGateNotice` stays above the audit panel (today in `AuditLogsTab`, removed).
13. **Strings**: Lingui everywhere; `.po` extraction and the translations last in each PR, after the
   rebase on `dev`. `AuditLogsSettings.tsx`' "under Logs → Audit logs" follows the new tab name.

## Files and tests (TDD: tests first)

### PR L1

| File | Change | Test file and cases |
|---|---|---|
| `components/logs/logsSearch.ts` (new), `router.tsx` | `LogsSearch`, lists, `parseLogsSearch`; the `logs` route's `validateSearch` | `logsSearch.test.ts`: `true` from a link kept; `"true"` → `true`; `"false"` → `false`; other `is_*` value dropped; numeric id → string; array → first element; whitespace-only dropped; unknown `tab`, `origin`, `channel`, `source`, `event_type`, `status`, `outcome`, `scope`, `actor_type` dropped; `period` checked per tab (`tab=messages&period=90d` and `custom` dropped, `tab=audit&period=1h` dropped); `reply` accepted as event type; empty URL valid; old `is_sent` dropped. `src/__tests__/logsRoute.test.ts`: the route's `validateSearch` is `parseLogsSearch` (model: `broadcastsRoute.test.ts`) |
| `components/logs/period.ts` (new) | bounds, defaults | `period.test.ts`: each period's `from`; `all` → no bound; `custom` → `from` and `to`; rounding to the minute; message default `all` with each indexed filter, `24h` with `list_id` or `external_id` alone; inbound default; audit default `all` with `target_id` |
| `components/logs/logTime.ts` (new) | Time column format | `logTime.test.ts`: today, yesterday, older, in the workspace timezone across midnight |
| `components/logs/FilterChip.tsx` (new) | chip and dashed "+ Filter" variant | `FilterChip.test.tsx`: label and value; clear cross calls `onClear` and does not open; opens with Enter |
| `components/logs/labels.ts` (new) | `msg` tables: delivery statuses, audit categories, outcomes, actor types (L2: sources, event types) | covered by the tab tests |
| `components/logs/logsUrl.ts` | `LOGS_TAB.audit` | `logsUrl.test.ts` passes |
| `components/transactional/notificationRules.ts` | `tab: LOGS_TAB.messages` (one line, declared) | type check; its tests pass |
| `pages/LogsPage.tsx` | header, tabs by right, skeleton while rights load, first permitted tab, 403 result, URL rules of decision 3, `destroyOnHidden` | `LogsPage.test.tsx` (**rewritten**; removed cases: "Message History" / "Audit logs" labels, Refresh and Export in the tab bar, tab change keeping `is_failed`): each right alone shows its tab only; `?automation_id=a` opens Messages with the URL untouched; `?tab=audit&id=x` without audit access → `{ tab: 'messages' }` only; `tab=audit&id=x` kept while the audit access loads then opened; tab change pushes `{ tab }` only; a tab left and reopened remounts; no right → 403 result |
| `layouts/app-shell/menuItems.ts`, `layouts/WorkspaceLayout.tsx` | optional `canOpenLogs` | `menuItems.test.ts`: `canOpenLogs` true shows Logs without `message_history`, false hides it with it; the existing case at line 121 unchanged (no option). `WorkspaceLayout.test.tsx`: `useAuditLogAccess` mocked; audit read alone shows Logs; webhook read alone shows Logs; a write-only member and no right hide it |
| `components/messages/MessageHistoryTab.tsx` | `automation_id` in `filterKeys` (one line, removed with the file in L2) | `MessageHistoryTab.test.tsx`: `automation_id` from the URL is sent |
| `components/logs/OutgoingWebhooksTab.tsx` (new) | toolbar (Subscription, Status, Refresh), table, Load more by offset without duplicates, footer, `SidePanel` detail with J/K, `copyLink={false}` | `OutgoingWebhooksTab.test.tsx`: `status` and `subscription_id` sent; filters written through the router with `tab`, as a push; `subscription_id` from the URL pre-filters; Retrying vs Failed pill; response code tone and error tooltip; subscription id shown when not in the list; Load more skips an id already shown; detail (ids, Next attempt only for Retrying and Pending, error alert, response body, payload, Open subscription link, disabled for a deleted subscription); "No webhook deliveries yet" with "Add a webhook"; "No results match" with "Clear filters"; error result with Retry |
| `components/logs/auditPhrases.ts`, `components/logs/AuditPhrase.tsx` (new) | `AUDIT_PHRASES`; render | `AuditPhrase.test.tsx`: with target name; with id only (`Mono`); without target ("(unknown)"); unknown action as its route name in `Mono`. `auditPhrases.test.ts`: one entry passed through the plugin transform with `isProduction: false` keeps `<0>{target}</0>` in its message |
| `internal/domain/audit_actions_test.go` | phrase test | the test itself: every catalogue action has a phrase (`user.oidc.callback` and `user.oidc.exchange` named in a case), no phrase for an unknown action, `<0>{target}</0>` only with a `TargetType`, every line of the block matches the pattern |
| `components/logs/AuditLogTab.tsx` (new) | `LicenceGateNotice`, search (IP / email / id), Period (with Custom range in the workspace timezone), Actor, Resource, "+ Filter" (Action, Outcome, Actor email, Target ID, IP), root `Segmented`, Export, Refresh, table (Workspace column in deployment view), Load more, footer with retention | `AuditLogTab.test.tsx`: licence notice shown; search routes an IPv4, an IPv6, an email, an id; actor + target + IP combined; Resource writes `categories`; Custom range bounds in the workspace timezone; Actor chip (member by email, pending invitation absent, API keys, by type, "Me" without `workspace.read`, root runs the members query); Segmented for root only; deployment view shows the Workspace column and hides the members and the retention; filters pushed, search typing replaced; `id` opens the detail; Load more, and a filter change restarting from an empty list; a re-render a minute later keeps the loaded rows, Refresh moves `from`; Export CSV and NDJSON with the URL filters, `message.error` on failure; the four empty states; retention footer for 0, n, absent, owner link |
| `components/logs/AuditEntryDetail.tsx` (new) | `SidePanel` of an entry, J/K | `AuditEntryDetail.test.tsx`: content, Changes table, Metadata, Raw entry, Copy link with `id` and `scope`, J and K, entry not found |
| `components/settings/audit_logs/AuditLogsSettings.tsx` | "under Logs → Audit log" | its test passes |
| Removed | `components/logs/AuditLogsTab.tsx` and test, `components/logs/AuditLogActions.tsx`, `components/settings/audit_logs/AuditFilterBuilder.tsx`, `AuditLogTable.tsx`, `AuditLogDetailsDrawer.tsx`, `actionLabels.ts` and their tests, `components/webhooks/OutgoingWebhooksTab.tsx` and test | — |
| `e2e/protected-pages.spec.ts` | new subtitle (the messages tab still shows today's empty text in L1) | the test itself |
| `CHANGELOG.md` | first half of the Feature line: tabs by right, the audit log as sentences with its filters in the link, the outgoing webhooks in a side panel | — |
| `src/i18n/locales/*.po` | extraction and translations | — |
| Docs (`Notifuse/docs`, PR to `dev`) | `features/audit-logs.mdx` (search and chips, filters in the link, sentences), `assets/screenshots/audit_logs.png` | — |

### PR L2

| File | Change | Test file and cases |
|---|---|---|
| `services/api/messages_history.ts` | `MessageHistory` gains `automation_id`, `transactional_notification_id`, `opens_tracked`, `clicks_tracked`, `attachments`; `ChannelOptions` gains `subject`, `subject_preview`; params `origin`, `created_after`, `created_before`; `countMessages` | `messages_history.test.ts`: query string of each new param; `countMessages` URL |
| `services/api/inbound_webhook_event.ts` | `source`; `reply`, `auto_reply` in `EmailEventType`; `transactional_id`, `broadcast_id` params removed | `inbound_webhook_event.test.ts`: query string |
| `services/api/permissions.ts` | `/api/messages.count` under Message History read | `permissions.test.ts`: listed under its right |
| `components/logs/MessagesTab.tsx` (new) | toolbar, counters, table, footer | `MessagesTab.test.tsx`: search mode switches to Email on "@", writes in `replace`, incomplete email neither written nor sent; counter sets its `is_*` and removes the others, `false` included; active counter; M2 gets the filters minus `is_*`, one key across counter clicks; no number on `all` without an indexed filter nor with `external_id` alone; `COUNT_MAX_PERIOD` at `7d` and `30d`; count error "—"; SMTP rule on the Delivered counter; Journey tones and the SMTP tooltip by origin; ids in `Mono` without broadcasts, automations or lists read; Clicked link offered only with a Broadcast; columns taken once from the old key; "Contact erased"; Load more; empty states |
| `components/logs/MessageDetail.tsx` (new) | `SidePanel`, Events / Sent content / Data | `MessageDetail.test.tsx`: loaded by D1; J and K; not found; Events order, no `sent_at` row, titles and tones; deleted template; template unreadable; contact erased; Open contact disabled without `contacts.read`; Sent by links; forced subject with "Set at send time"; From address; Tracking "On (default)"; HTML / Text / MJML tabs; render failure alert; Data with "No data" |
| `components/logs/InboundWebhooksTab.tsx` (new) | toolbar, table, `SidePanel` detail (`copyLink={false}`) | `InboundWebhooksTab.test.tsx`: malformed email not sent; Source and Type sent; period sends `timestamp_after`; linked message only for the four delivery types and with `message_history.read`; detail with J/K and raw payload; "No webhook events received yet" with its link |
| `components/analytics/RecentFailures.tsx` | "View in logs" adds `period: 'all'` (one line, declared) | `RecentFailures.test.tsx`: link search has `is_failed` and `period` |
| `components/logs/logsUrl.ts`, `logsUrl.test.ts` | `buildLogsTabUrl` removed with its test (kept: `LOGS_TAB`) | — |
| `e2e/protected-pages.spec.ts`, `e2e/fixtures/auth.ts` | "Nothing in the last 24 hours"; `messages.count` route | the test itself |
| Removed | `components/messages/MessageHistoryTab.tsx` and test, `components/webhooks/*` and tests | — |
| `CHANGELOG.md` | the rest of the spec's Feature line, without the API sentence that #535 already wrote | — |
| `src/i18n/locales/*.po` | extraction and translations | — |
| Docs (`Notifuse/docs`, PR to `dev`) | `features/logs.mdx`, `logs_webhooks.png`, a new messages screenshot | — |

## Verification

```bash
cd console
npx vitest run <touched test files>
npx vitest run
npx tsc --noEmit -p tsconfig.app.json
npm run lint
npm run lingui:extract
npm run build
npx playwright test -c playwright.local.config.ts e2e/protected-pages.spec.ts --trace off
cd .. && make test-domain   # audit phrases test (L1)
```

Screenshots at 1440 and 1280 of each tab and detail.

## Spec review (Opus, effort xhigh) and what the plan takes from it

Reviewed against the spec, #535 and the code. Taken into the plan:

- B1: the Go key pattern misses `user.oidc.callback` and `user.oidc.exchange` → decision 11, test case.
- B2: a typed `tab` refuses `notificationRules.ts`' inferred `string` (reproduced with tsc) → decision 4.
- B3: links without `tab` (Automations, link stats, node panel) must keep their filters; an unpermitted tab
  is replaced alone → decision 3, `LogsPage` cases.
- B4: `LogsPage.test.tsx` exists → marked rewritten, removed cases listed.
- B5: `WorkspaceLayout.test.tsx` has no query client and `menuItems.test.ts:121` pins today's rule → optional
  `canOpenLogs`, `useAuditLogAccess` mocked.
- B6: L1 needs its `.po` row.
- B7: `LicenceGateNotice` → decision 12, test case.
- B8: the menu follows read rights → decision 2.
- B9: `now` of the period bound is state → decision 5, test cases.
- B10: the docs PR rows.
- B11: antd keeps hidden panes mounted → `destroyOnHidden`, remount case.
- N12 (`automation_id` in the old tab), N13 (test gaps), N14 (`AuditPhrase.tsx`, `isProduction: false`),
  N15 (`logsUrl.test.ts`, settings wording), N16 (CHANGELOG half in L1), N17 (lists in `logsSearch.ts`)
  taken.

Known, not handled here: `useWorkspacePermissions` does not reset `loading` when the workspace changes
(review N15); the Logs page reads it as it is.
