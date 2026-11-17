# Empty-workspace dashboard: "Get started" checklist and empty states

Mockup: https://claude.ai/artifact/WQHXfwnWzzKrAAHmHTxm8Z (four artboards: today, option A, option A after the first steps, option B).

## Decisions (Pierre, 2026-11-17)

- D1 Option A: a full-width "Get started" checklist at the top of the dashboard; it folds as steps are done and disappears at 6/6.
- D2 Steps tick automatically from the workspace's data; "Hide" hides it per user in the browser; no backend state.
- D3 The KPI strip stays; each tile says when it fills ("After your first send") instead of 0 / "—"; the period switch and date range are hidden until the first email is sent.
- D4 One "Send your first email" step, ticked by any send, offering both New broadcast and New notification.
- Included regardless: the Import contacts button when the only contact is the owner; the contacts KPI subtitle counting today; "No failure" not shown before any send; "Not configured" in ink-3 for contrast.

Questions Q1–Q4 are answered in section 8. Facts were checked against the code on 2026-11-17 (CreateWorkspace root-only and owner contact in list `test`, Import shown only at zero rows, `GetSender` nil without a default sender, `messages.list` order and index, `?compose=new`, contacts KPI window ending yesterday).

No Go change is needed: every step ticks from data the console already has or from an existing cheap endpoint. All work is in `console/src`.

## 1. Current state: files that change

| File | What it does today |
|---|---|
| `console/src/pages/AnalyticsPage.tsx` | The workspace home. Resolves the periods (`:66-70`), gates everything on `message_history.read` (`:73`, `:93-95`; `canReadDashboard` at `layouts/app-shell/menuItems.ts:195-197`). The header holds the covered-days label and the 7/30/90 d `Segmented` (`:97-128`); the loading skeleton reuses it (`:130-147`). `can()` at `:149-150`. Layout: KPIs, then chart + failures + contacts, then sending + upcoming (`:152-188`). |
| `console/src/components/analytics/DashboardKpis.tsx` | Six tiles. Contacts total `:98-104` (key `['contacts', ws, 'dashboard-total']`). Contacts new `:105-122`, filtered on `current.startUtc/endUtc`, so it ends yesterday: the "+0 over 30 d" bug. Contacts sub `:237-243`, sent `:248-257`, delivered `:259-279`, open/click `:196-206` and `:281-282`. Visits when off `:319-334` (`—` plus "Set up web analytics"). |
| `console/src/components/analytics/dashboardPeriods.ts` | `resolveDashboardPeriods` (`:24-36`): N complete days ending yesterday (`web_analytics/lib/dates.ts:24-27`, `:43-46`). |
| `console/src/components/analytics/OriginVolumeChart.tsx` | Empty state = antd `Empty` "No email sent in this period" (`:164-169`); metric `Segmented` at `:175-186`. |
| `console/src/components/analytics/RecentFailures.tsx` | Three `messages.list` queries (`:43-49`); with no rows a green `CircleCheck` and "No failure" (`:109-115`), even before any send. |
| `console/src/components/analytics/NewContactsTable.tsx` | `contacts.list` limit 5 under `['contacts', ws, 'dashboard-latest']` (`:32-41`); Import only when `rows.length === 0` (`:68-79`), so the owner contact hides it. |
| `console/src/components/analytics/SendingCard.tsx` | Provider and sender per role (`:22-74`); "Not configured" in `text-ink-4` (`:58`); falls back to `senders[0]` for display (`:45`). |
| `console/src/components/analytics/UpcomingSends.tsx` | Empty "No broadcast scheduled" plus "New broadcast" to plain `/broadcasts` (`:94-105`). |
| `console/src/components/layout/KpiStrip.tsx` | `KpiItem` (`:7-22`); the value always renders in `text-kpi` (`KpiCell`, ~`:117`). |

Other facts relied on:

- **Owner contact.** `CreateWorkspace` is root-only (`internal/service/workspace_service.go:230-234`); it adds the creator as owner (`:343-361`), upserts a contact with the creator's email (`:371-388`) and subscribes it to list `test` (`:393-418`). So the only pre-existing contact is a root email. The console knows root emails through `window.ROOT_EMAIL` (`internal/http/root_handler.go:189`, `console/src/services/api/auth.ts:49-65`, `parseRootEmails`).
- **Members.** `workspaces.members` is fetched by `useWorkspacePermissions` under `['workspace-members', ws, userId]` (`console/src/contexts/AuthContext.tsx:210-219`), skipped for root users (`:216`, `:244-246`). It returns pending invitations (`workspace_service.go:1422-1453`) and `type: 'user' | 'api_key'` (`console/src/services/api/workspace.ts:429-443`); owners also get virtual root-email owner rows (`:1455-1480`); a member without `workspace.read` gets only their own row (`:1393-1413`); inviting is owner-only (`:853-860`).
- **Sent mail.** `messages.list` with no filter orders by `created_at DESC, id DESC` (`internal/repository/message_history_postgre.go:1007-1014`) on `idx_message_history_created_at_id` (`internal/database/init.go:304`): `limit=1` is one index probe. Template tests and double opt-in confirmations are rows of origin `other`; a broadcast test send is `broadcast` (`internal/domain/analytics.go:273-284`).
- **Templates.** `templates.list` has no count variant and returns full rows (`internal/repository/template_postgres.go:218-262`). TemplatesPage reads it under `['templates', ws, 'email']` (`console/src/pages/TemplatesPage.tsx:131-132`), invalidated through `['templates', ws]` (`:228`, `components/templates/CreateTemplateDrawer.tsx:368,378`).
- **Senders.** `GetSender(id)` returns the sender chosen by id, and falls back to the `is_default` one only when no id is given (`internal/domain/email_provider.go:158-173`); the console's provider drawer always keeps a default sender (`EmailProviderDrawer.tsx:213-231`), so "no default" only happens for providers created through the API.
- **Deep links.** `/broadcasts?compose=new` opens the composer (`console/src/router.tsx:119-140`, `pages/BroadcastsPage.tsx:125-139`). The transactional page has no "new" parameter (`components/transactional/transactionalSearch.ts:23-28`), nor `/templates` (`lib/searchParams.ts:40-49`).
- **CSV import drawer.** Provided by `layouts/WorkspaceLayout.tsx:141`, so it stays open across navigation; on success it invalidates `['contacts', ws]` (`components/contacts/ContactsCsvUploadProvider.tsx:36-45`).
- **Workspace object.** Carries `integrations`, `settings`, `created_at` (`services/api/workspace.ts:296-303`); settings screens call `refreshWorkspaces()` after saving (`pages/WorkspaceSettingsPage.tsx:167,172`).
- **localStorage pattern.** `layouts/app-shell/useSidebarPinned.ts:6-27` (try/catch on read and write).
- **Translations.** Every non-English catalogue must be complete (`console/src/i18n/locales/catalogues.test.ts:125-135`).
- **Changelog.** The dashboard is unreleased (44.0, `CHANGELOG.md:59`): extend its bullet, no Fix bullet.
- **Demo workspace.** SMTP integration with a default sender for both roles (`internal/service/demo_service.go:738-790`), plus contacts, templates and messages: 6/6, so no checklist.

## 2. Data source for each step

| Step | Done when | Source | Already loaded? | Permission |
|---|---|---|---|---|
| 1 Create workspace | always | — | — | — |
| 2 Connect a provider | `workspace.integrations` has a `type === 'email'` integration with an `email_provider` | workspace object | yes | none |
| 3 Choose sender and uses | both `marketing_email_provider_id` and `transactional_email_provider_id` resolve to an existing email integration with an `is_default` sender (see Q2) | workspace object | yes | none |
| 4 Add contacts | latest-5 rows ≥ 2, or exactly 1 row whose lower-cased email is not an owner email | `['contacts', ws, 'dashboard-latest']` (shared with NewContactsTable); the pill count from `['contacts', ws, 'dashboard-total']` | yes | `contacts.read` |
| 5 Pick a template | `templates.length ≥ 1` | `templates.list?channel=email` under `['templates', ws, 'email']` | no; fetched only while the checklist is a candidate (§3) | `templates.read` |
| 6 Send first email | the latest message exists (see Q1) | new query `['dashboard', 'latest-message', ws]` → `listMessages(ws, { limit: 1 })` | new, one index probe; also drives the empty states | `message_history.read` (already required by the page) |
| Track your website (optional) | `settings.web_analytics?.enabled` | workspace object (`DashboardKpis.tsx:88`) | yes | none |
| Invite your team (optional) | any `type === 'api_key'` member; or any member/invitation whose `user_id !== me` and whose email is not a root email; or the viewer is not a root email | `['workspace-members', ws, userId]` (the checklist adds the fetch for root users) | yes, except root users | `workspace.read` |

Owner emails = `parseRootEmails(window.ROOT_EMAIL)` only (only root can create a workspace, so the owner contact is always a root email); case-insensitive. No members request is needed for step 4.

No new endpoint: the two contacts queries and the members list are already in flight, `messages.list` limit 1 is an index probe, and `templates.list` is requested only for candidate workspaces (never sent, or created in the last 30 days), which hold few or no templates. A `templates.count` endpoint is the rejected alternative.

Permissions (no 403): the checklist renders, and its queries run, only for an owner or root (`role === 'owner' || role === 'root'`): integrations and sender roles are owner-only on the server (`CreateIntegration`, `workspace_service.go:2143-2152`; `UpdateWorkspace`, `:472-475`), so a non-owner could not complete steps 2–3. Owners hold every read right, so no step can 403. Each step's button needs its own write right: integrations and web analytics `workspace.write`; import `contacts.write`; templates `templates.write`; New broadcast `broadcasts.write`; New notification `transactional.write`; team `role === 'owner' || role === 'root'`. A step without its right still ticks, with no button.

## 3. Rules (pure model)

- **Next step** = first required step (2→6) not done. One `type="primary"` button on the page: the next step's action (step 6: New broadcast primary, New notification default). Every other action is a default button or link.
- **Progress** = required done / 6; optional steps never count and are never next.
- **Collapse.** From two required steps done (`COLLAPSE_AFTER = 2`), done steps fold into pills saying what was done ("Workspace created", "Amazon SES connected", "Sends as hello@acme.com", "1,240 contacts", "3 templates", "First email sent", "Website tracked", "2 teammates"); "Show done steps" (component state) expands them.
- **Visibility** `shown` only when: the member has the rights in §2; not hidden in localStorage; required steps not 6/6; and either no message yet or `workspace.created_at` less than `GET_STARTED_GRACE_DAYS = 30` days ago (see Q3). So an older workspace that already sends never sees it; a new one keeps it after its first send until 6/6 or Hide.
- **Loading.** No message → the checklist will show: skeleton panel while templates and members load. A message and a recent workspace → render nothing until the data is in, so a 6/6 workspace never flashes.
- **Hide.** Key `notifuse.getStarted.hidden.<userId>.<workspaceId>` = `'true'`; read and write in try/catch; blocked storage still hides for the visit.
- **Send state** (page-level): `sendState(latestAt, periods.current.endUtc, totalsSent)` → `'loading'` | `'never'` (no message) | `'today'` (latest after the period end and no send in the period's series) | `'counted'`. `'today'` exists because the period ends yesterday: right after the first send the chart would otherwise say "No email sent in this period".
- **Contacts KPI fix.** Count new contacts over the N calendar days ending today, workspace time zone: `start = startOf(today − (N−1))`, `end = endOf(today)`, via `resolveRange` (DST-safe) as `periods.recent`, with hint "Added in the last 30 days, today included". Other KPIs keep complete days up to yesterday.

## 4. Components

New, all in `console/src/components/analytics/`:

1. `getStartedModel.ts` (pure): types `StepId`, `StepState { id; required; done; detail? }`, `GetStartedModel { steps; done; total; next?; complete }`; functions `deriveGetStarted(input)`, `emailIntegrations(ws)`, `roleReady(ws, role)`, `ownerEmails(rootEmails, members?)`, `hasOnlyOwnerContact(rows, owners)` (also used by NewContactsTable), `hasTeammate(members, me, rootEmails)`, `getStartedVisibility({ canAct, hidden, model, hasSent, createdAt, now })` → `'hidden' | 'loading' | 'shown'`, `shouldCollapse(model)`; constants `GET_STARTED_GRACE_DAYS`, `COLLAPSE_AFTER`.
2. `dashboardSendState.ts` (pure): `sendStateOf(latest, periodEndUtc, totalsSent)`.
3. `useLatestMessage.ts`: `useQuery(['dashboard', 'latest-message', ws], () => listMessages(ws, { limit: 1 }))`, `enabled: canRead`, `refetchInterval` = `DASHBOARD_REFRESH_MS` while empty, `false` once a message exists; returns `{ latestAt: string | null | undefined, isError }`; an error counts as `'counted'` so the page keeps today's behaviour.
4. `useGetStartedHidden.ts`: `[hidden, hide]`, modelled on `useSidebarPinned.ts`.
5. `dashboardQueries.ts`: query-option factories keeping the shared keys identical — `latestContactsQuery(ws)` (from `NewContactsTable.tsx:32-41`), `contactsTotalQuery(ws, tz)` (from `DashboardKpis.tsx:98-104`), `emailTemplatesQuery(ws)` (= `TemplatesPage.tsx:131-132`), `membersQuery(ws, userId)` (= `AuthContext.tsx:212-215`).
6. `GetStartedChecklist.tsx`: props `workspace`, `permissions`, `role`, `userId`, `latestAt`, `sendState`; runs the gated queries, derives the model, renders a full-width `Panel` "Get started" with "n of 6" (plural macro) and Hide; required steps as numbered rows with their links, optional steps in a subgroup, a blog line linking to `/blog` with `blog.read`. Actions: steps 2/3 `Link` to `settings/$section` `integrations`; step 4 `navigate('/contacts')` then `openDrawer(ws)` from `useContactsCsvUpload`; step 5 `/templates?new=1`; step 6 `/broadcasts?compose=new` and `/transactional-notifications?new=1`; optional `settings/web-analytics`, `settings/team`. Pills with `Pill` (`components/layout/Pill.tsx`, tone `success`).
7. `ChartGhost.tsx`: grey bars (`bg-fill` / `bg-line-soft`, heights from a fixed array via `style.height`, `aria-hidden`) behind a centred message and one default action.

Modified:

- `AnalyticsPage.tsx`: call `useLatestMessage` beside `useOriginSeries` (`:74-79`); compute `sendState` from `series.totals`; hide the covered-days span and period `Segmented` (`:110-125`) while `'never'` or `'loading'`; mount `<GetStartedChecklist>` between `{header}` and `<DashboardKpis>` (`:154-155`) with `role` from `useWorkspacePermissions` and `user.id` from `useAuth()`; pass `sendState` to DashboardKpis, OriginVolumeChart, RecentFailures; `ownerEmails` to NewContactsTable; `marketingReady` (`roleReady(ws, 'marketing')`) and `canConfigure` to UpcomingSends; the chart ghost action = New broadcast (`/broadcasts?compose=new`) when marketing is ready and `broadcasts.write`, else "Connect an email provider" with `workspace.write`, else none.
- `KpiStrip.tsx`: optional `KpiItem.placeholder?: ReactNode`, rendered instead of `value` in `text-sm text-ink-3`.
- `DashboardKpis.tsx`: prop `sendState`; `'never'` → Sent, Delivered, Open, Click show "After your first send" (no 0/—); `'today'` → "From tomorrow" with hint "The dashboard counts complete days; today's emails show tomorrow"; `seriesError` still wins; visits off → "Once your site is tracked" with the "Set up web analytics" link as sub; contacts new uses `periods.recent` (key and filter) plus its hint; total from the factory.
- `dashboardPeriods.ts`: add `recent`.
- `OriginVolumeChart.tsx`: prop `ghost?: { message: string; action?: ReactNode }` → `ChartGhost` instead of skeleton/empty, metric switch hidden. `'never'`: "Your emails will show here, day by day, from your first send." `'today'`: "Your first emails show here tomorrow: the chart counts complete days." (no action).
- `RecentFailures.tsx`: prop `sendState`; queries enabled only for `'counted'` / `'today'`; skeleton while loading; `'never'` → "Nothing has been sent yet. Bounces, complaints and failures will show here." with no tick.
- `NewContactsTable.tsx`: prop `ownerEmails`; `hasOnlyOwnerContact` → "Only you so far" plus Import (with `canImport`); query from `latestContactsQuery`.
- `SendingCard.tsx`: `:58` `text-ink-4` → `text-ink-3`; when neither role resolves, a line "Nothing can be sent until an email provider is connected." plus Configure (with `canConfigure`).
- `UpcomingSends.tsx`: empty "Scheduled broadcasts will show here." plus New broadcast to `/broadcasts?compose=new` (`:99`); without a marketing provider, a note "Sending it needs an email provider" with Configure (with `canConfigure`).
- `CHANGELOG.md`: extend the unreleased dashboard bullet (`:59`).

## 5. Tests, file by file

| Implementation file | Test file | Cases |
|---|---|---|
| `getStartedModel.ts` | `getStartedModel.test.ts` (new) | Step 1 always done. Step 2: none; non-email integration (zapier, llm) doesn't count; email integration counts. Step 3: neither role; one role; both but one id points to a deleted integration; both without an `is_default` sender; both ready. Step 4: 0 rows; 1 root-email row (other case); 1 owner-member row; 1 non-owner row; 2 rows; undefined is not done. Step 5: 0 / 1 / undefined. Step 6: null / ISO string. Website: enabled / disabled / absent. Team: only me; root virtual owner only; pending invitation; api_key; non-root viewer. Next: first undone in order; skips done; undefined when complete; optional never next. Totals: optional excluded; complete at 6/6 with optional undone. Visibility: no write → hidden; missing read → hidden; hidden flag → hidden; complete → hidden; sent and created 31 days ago → hidden; sent and created 2 days ago → shown; never sent and created 2 years ago → shown; `created_at` missing counts as old; loading with no message → loading. Collapse at threshold − 1 and at threshold. |
| `dashboardSendState.ts` | `dashboardSendState.test.ts` (new) | loading (undefined); never (null); today (latest after the period end, series 0); counted (latest after end, series > 0); counted (latest inside the period). |
| `dashboardPeriods.ts` | `dashboardPeriods.test.ts` (extend) | `recent` covers N days ending today in Europe/Paris (fake timers); just after local midnight it moves; DST week; UTC fallback. |
| `useLatestMessage.ts` | `useLatestMessage.test.tsx` (new) | asks `messages.list` limit 1 under its key; no request when disabled; polls each minute while empty, stops once a message exists. |
| `useGetStartedHidden.ts` | `useGetStartedHidden.test.ts` (new) | not hidden by default; hide persists under the user+workspace key; another user or workspace not hidden; `getItem` throwing → not hidden; `setItem` throwing still hides for the visit. |
| `dashboardQueries.ts` | `dashboardQueries.test.ts` (new) | keys exactly `['contacts', ws, 'dashboard-latest']`, `['contacts', ws, 'dashboard-total']`, `['templates', ws, 'email']`, `['workspace-members', ws, userId]`, so invalidations keep working. |
| `GetStartedChecklist.tsx` | `GetStartedChecklist.test.tsx` (new; mocks for `templatesApi`, `contactsApi`, `dashboardClient`, `workspaceService.getMembers`, `useContactsCsvUpload`, router as in `DashboardKpis.test.tsx:18-41`) | New workspace: "Get started", "1 of 6", workspace step ticked, one primary "Connect an email provider" to `/settings/integrations`. Provider without roles: next is the sender step. Owner-only contact: contacts step open; Import navigates to `/contacts` and calls `openDrawer('ws1')`. Step 6 offers New broadcast (`?compose=new`) and New notification. After two steps: pills naming provider, sender and counts; "Show done steps" expands. 6/6: nothing rendered even with optional steps open. Hide removes it and stays hidden on remount. Without `templates.read`: nothing rendered, `templatesApi.list` never called. Without workspace write: nothing, no request. Old workspace that has sent: nothing, neither templates nor members requested. Recent workspace that has sent with steps open: shown. Without `templates.write` no template button; non-owner no team button. Exactly one `.ant-btn-primary`. Blog line links to `/blog`. Root user: members fetched here, root virtual rows ignored. |
| `ChartGhost.tsx` | `ChartGhost.test.tsx` (new) | message and action rendered; bars `aria-hidden`. |
| `KpiStrip.tsx` | `components/layout/KpiStrip.test.tsx` (extend) | the placeholder replaces the value and is not in the `text-kpi` span; loading still shows the skeleton. |
| `DashboardKpis.tsx` | `DashboardKpis.test.tsx` (extend; `recent` in the `periods` fixture `:47-62`) | never: Sent, Delivered, Open, Click read "After your first send", no "0"/"—"; today: "From tomorrow"; series error still offers Retry; contacts new uses `recent` bounds and key; a contact created today counts in the sub; visits off shows the placeholder and the link. |
| `OriginVolumeChart.tsx` | `OriginVolumeChart.test.tsx` (extend) | ghost shows its message and action and hides the metric switch; without a ghost the existing empty state is unchanged (`:191`). |
| `RecentFailures.tsx` | `RecentFailures.test.tsx` (extend) | never: "Nothing has been sent yet", no "No failure", `listMessages` not called; counted with no rows: "No failure"; loading: skeleton, no request. |
| `NewContactsTable.tsx` | `NewContactsTable.test.tsx` (extend) | owner only (root email, other case) → "Only you so far" and Import calling `openDrawer`; one non-owner contact listed; owner only with `canImport=false` → no button. |
| `SendingCard.tsx` | `SendingCard.test.tsx` (extend) | no provider → "Nothing can be sent…" and Configure; "Not configured" has `text-ink-3`; one role set → no "nothing can be sent". |
| `UpcomingSends.tsx` | `UpcomingSends.test.tsx` (extend `:176-190`) | empty state explains, New broadcast to `/broadcasts?compose=new`; without a marketing provider the note and Configure; without `broadcasts.write` no button. |
| `AnalyticsPage.tsx` | `pages/AnalyticsPage.test.tsx` (extend; mock `useLatestMessage` and `GetStartedChecklist`; `user` in the `useAuth` mock `:25-33`) | before the first send no period switch and no `[data-dashboard-days]`; once sent both shown; hidden while loading; the checklist gets role and user and sits above the KPIs; `sendState` reaches KPIs and failures; existing redirect/permission tests (`:169-198`) still pass. |
| smoke | `__tests__/pages.smoke.test.tsx` | AnalyticsPage still renders (add `listMessages` and `templatesApi.list` mocks if missing, `:103`, `:150`). |
| i18n | `i18n/locales/catalogues.test.ts` | passes after extraction and translation of every new msgid in ca, de, es, fr, it, ja, pt-BR (plural macro for "n of 6", contacts, templates). |

## 6. Risks and edge cases

- **Workspaces that already send**: the 30-day grace rule hides it; old never-sent workspaces get it (they are empty).
- **Members without write**: no checklist; read-only empty states have no buttons. **Writer without contacts/templates read**: no checklist, no 403.
- **Demo workspace**: 6/6, hidden; a reset recreates it with a recent `created_at`, harmless at 6/6.
- **SMTP-only workspaces**: step 2 ticked by any email integration; after the first send Delivered shows the existing SMTP sub (`DashboardKpis.tsx:268-269`).
- **Owner-contact detection**: relies on the creator being root (`workspace_service.go:230-234`); if `ROOT_EMAIL` changed, owner-member emails cover it when readable; an owner contact deleted with one real contact left counts as done; case-insensitive.
- **Time zone of today**: `recent` and `'today'` use workspace-tz bounds from `resolveRange`; `useMinuteTick` (`AnalyticsPage.tsx:26-33`) rolls them at local midnight; the 30-day grace is an absolute duration.
- **Default sender**: a provider with senders but none default fails to send (`email_provider.go:158-173`) while SendingCard shows `senders[0]` (`:45`); step 3 requires `is_default`, so the checklist and the card can disagree. Showing "No default sender" in the card is small but out of scope unless decided.
- **Staleness**: the workspace object refreshes through `refreshWorkspaces()`; templates and contacts are invalidated by their screens; the latest message polls each minute until found.
- **Layout shift**: the skeleton appears only when the checklist is certain to show.

## 7. Sequence

1. Pure layer with tests: `getStartedModel.ts`, `dashboardSendState.ts`, `dashboardPeriods.ts` (`recent`).
2. Hooks: `useLatestMessage.ts`, `useGetStartedHidden.ts`, `dashboardQueries.ts`; switch NewContactsTable and DashboardKpis to the factories (no behaviour change).
3. `KpiStrip` placeholder.
4. Empty states and fixes: DashboardKpis (placeholders, contacts window), OriginVolumeChart + ChartGhost, RecentFailures, NewContactsTable, SendingCard, UpcomingSends.
5. `?new=1` on the Templates and Transactional pages (Q4), then `GetStartedChecklist.tsx`.
6. Wire `AnalyticsPage.tsx` (header gating, checklist mount, props).
7. `npm run lingui:extract`, translate the new entries in the seven non-English `.po` files, extend `CHANGELOG.md:59`.
8. Run the tests:
   ```
   cd console
   npx vitest run src/components/analytics src/components/layout/KpiStrip.test.tsx src/pages/AnalyticsPage.test.tsx src/pages/TemplatesPage.test.tsx src/pages/TransactionalNotificationsPage.test.tsx src/lib/searchParams.test.ts src/components/transactional/transactionalSearch.test.ts src/__tests__/pages.smoke.test.tsx src/i18n
   npm test -- --run
   npx tsc --noEmit -p tsconfig.app.json
   npm run lint
   npm run lingui:extract   # no new empty msgstr; catalogues.test.ts confirms
   ```
   No Go file is touched; if Q-answers add a `templates.count` endpoint, add `make test-domain test-service test-http test-repo` and an integration test.

## 8. Answered questions (Pierre, 2026-11-17)

- **Q1** Any message ticks step 6 and brings back the period switch, template tests included (one indexed call).
- **Q2** Step 3 needs both roles (marketing and transactional), each with a default sender.
- **Q3** Grace window: 30 days (`GET_STARTED_GRACE_DAYS = 30`).
- **Q4** Add `?new=1` deep links: `/templates?new=1` opens the "New template" gallery and `/transactional-notifications?new=1` opens the new-notification editor, like `/broadcasts?compose=new`. Files: `console/src/lib/searchParams.ts` (`parseTemplatesSearch`) + `pages/TemplatesPage.tsx`; `components/transactional/transactionalSearch.ts` + `pages/TransactionalNotificationsPage.tsx`; the parameter is dropped from the URL once the drawer opens (closing doesn't reopen it on refresh) and ignored without the matching write right. Tests: `lib/searchParams.test.ts` and `transactionalSearch.test.ts` (parse/normalise `new`), `TemplatesPage.test.tsx` and `TransactionalNotificationsPage.test.tsx` (`?new=1` opens the gallery / editor once, removes the parameter, does nothing without the write right). The checklist's step 5 and step 6 buttons use these links.

## 9. Corrections from the plan review (Opus, 2026-11-17) — these supersede the sections above

1. **Owner-only checklist and Configure actions.** The checklist shows only to owner/root (see §2). The chart ghost's "Connect an email provider" and the new Configure links in SendingCard and UpcomingSends use `role === 'owner' || role === 'root'` (not `workspace.write`, `AnalyticsPage.tsx:176`); the web analytics step's button uses `isOwner || web_analytics.write` (`WorkspaceSettingsPage.tsx:140-143`). Tests: a writer who is not an owner gets no checklist; SendingCard and UpcomingSends show no Configure for that member.
2. **No header flicker.** Hide the period switch and date range only for `'never'`; while `'loading'` keep today's header (the permissions skeleton already renders it, `AnalyticsPage.tsx:130-134`). The AnalyticsPage test "hidden while loading" becomes "shown while loading".
3. **Send state.** Compare dates with `Date.parse` on both sides (server timestamps may carry an offset or nanoseconds). Series still loading → `'loading'`; series error → `'counted'`. Texts drop "first": "Today's emails show here tomorrow: the chart counts complete days." (a quiet workspace sending today also hits `'today'`). New `dashboardSendState.test.ts` cases: series undefined; series error; latest exactly at `endUtc`; latest with a `+01:00` offset.
4. **`?new=1` links.** Use a boolean search value `new: true` (the router writes it `new=true`; a string would be JSON-quoted, see `RecentFailures.tsx:84-89`); `parseTemplatesSearch` and `validateTransactionalSearch` normalise `true`, `'true'`, `1`, `'1'`. Tests assert on the parsed object from the raw router values (number `1`, boolean `true`), not on an href. Act on the parameter only once permissions are loaded (keep it while loading). Transactional: the right is `canCreateTransactionalNotification` (transactional write + templates read, `services/api/permissions.ts:1107-1111`); opening from the URL needs a new `overlay` kind `'create'` rendered with `open`/`onClose`, like the edit overlay (`TransactionalNotificationsPage.tsx:450-459`). Extra tests: permissions loading → parameter kept; templates read missing → editor not opened. The checklist's "New notification" uses `canCreateTransactionalNotification` too.
5. **Import with lists.** Step 4's Import and NewContactsTable's Import pass the workspace's lists to `openDrawer(ws, lists, true)` (as `ContactsViewHeader.tsx:261`, `ImportContactsButton.tsx:36`), from a `['lists', ws]` query gated on `lists.read`. Test: the drawer is opened with the lists.
6. **Members list.** Owner emails come from root emails only (see §2). The team step reads the members list only when it is already available (`useWorkspacePermissions` for non-root users) or with one gated request for root users; while it loads, or without `workspace.read`, the team step shows without a tick; the skeleton never waits on it. Drop the members-union test cases; keep: team step unticked while members load.
7. **Shared query factories.** Put `emailTemplatesQuery` and `membersQuery` under `services/api` (not `components/analytics/`) and use them in every place that writes those keys by hand: `TemplatesPage.tsx:131`, `AutomationsPage.tsx:114`, `AuthContext.tsx:212`, `AuditLogTab.tsx:118`, `hooks/useAuditLogAccess.ts:22`; the contacts factories stay in `components/analytics/dashboardQueries.ts`. The key test then guards those shared factories.
8. **Grace rule.** A recent workspace that already sends but lacks one role or a default sender gets the checklist on the day this ships, by design (it is not set up). The "`created_at` missing" case becomes "invalid date counts as old" (`created_at` is required, `services/api/workspace.ts:302`). To check before release: the cloud workspaces migrated on 2026-11-15 kept their original `created_at`.
9. **RecentFailures and KpiStrip.** RecentFailures branches on `sendState` before `isLoading` (a disabled query has `isLoading === false`, `RecentFailures.tsx:64`). KpiStrip's tooltip wraps the placeholder too (`KpiStrip.tsx:142-143`); test: placeholder with a hint shows its tooltip.
10. **Wording.** SendingCard: "Nothing can be sent until an email provider is chosen for marketing or transactional emails." Pills: teammates and API keys counted separately ("2 teammates", "1 API key"); the sender pill names its role when the two roles' senders differ ("Marketing sends as …").
11. **Test setup.** AnalyticsPage test: add `role` (and `user`) to its `useWorkspacePermissions`/`useAuth` mocks (`pages/AnalyticsPage.test.tsx:28-31`). Smoke test: add a `messages_history` mock (missing today). Every fixture building `DashboardPeriods` gains `recent` (grep for them).
12. **Index claim.** `idx_message_history_created_at_id` exists only in `internal/database/init.go:304` (no migration), so very old workspace databases may lack it; the latest-message query is still a `LIMIT 1` ordered scan and is acceptable.
