# Console redesign — Settings screens (spec 9)

Source spec: "Spec 9 · Paramètres" (Nuit claire redesign, screens 111 to 115). This plan implements it in
the console. The server side of the spec (`name` in `workspaces.members`) already shipped in #527, so this PR
touches `console/` only, plus a CHANGELOG line.

Base: `dev` after #530 (theme) and #531 (sidebar without top bar, shared layout components in
`console/src/components/layout`). Every shared component is reused, none redefined.

## Scope

- Left rail grouped into Workspace, Sending, Content, Security, with lucide icons; `/settings` and an unknown
  section open General.
- General: settings in rows (`SettingsRow`) inside four panels; logo behind a "Replace" popover; read-only
  view in the same rows; skeleton while ownership is unknown.
- Team & API keys: two flush panels (members and invitations, API keys) with member names, access summary
  by resource, row menus.
- Integrations: one card per email provider (`EmailProviderCard`, `IntegrationCard`, `providerDetails`), a
  stacked `KeyValueList`, a grid for the other integrations, catalogue buttons hidden for non-owners.
- Custom fields: a table of named slots with the free-slot count.
- Webhooks: a table with the 7-day success rate and last success; detail in a `SidePanel` driven by
  `?webhook={id}`.
- Other sections (SMTP bridge, Blog, Web analytics, Audit log, Licence, Danger zone): wrapped in `Panel`s,
  content unchanged.

Out of scope (spec "Non livré"): last activity, key prefix, "created by", custom field fill rate, DNS
verified state, logo upload.

## Decisions taken while reading the code

1. **Form.Item bottom margin**: set through the theme by `SettingsRows` (see review item 10).
2. **Relative dates**: `relativeDate` / `useRelativeDate` in `components/layout/format.ts`, on `Intl` (review
   item 11).
3. **Initials avatar.** The sidebar's account menu has an initials rule (`layouts/app-shell`). The member
   column reuses that function if it is exported, otherwise it is moved to `components/layout/initials.ts`
   with its test and the account menu imports it from there (minimal shared change, signalled).
4. **`canManageWebhooks`** is computed by the page like the other `canManage*` flags and passed to
   `WebhooksSettings`, then to `WebhookDetail`.
5. **Members fetch** stays as it is (`fetchMembers` state in the page; S1 of the spec). A
   `membersLoaded` flag distinguishes "not answered yet" from "answered": General and Team render skeletons
   until it is set; on error the page sets `membersError` (Team shows a `Result` with Retry, other sections
   behave as for a member without write rights).
6. **Webhook queries.** W1 (`webhookSubscriptions.list`) moves to `useQuery` with key
   `['webhook-subscriptions', workspaceId]` — the key `OutgoingWebhooksTab` already uses for the same call,
   so both share the cache. W2 key `['webhook-success-7d', workspaceId, from]`, W3
   `['webhook-stats', workspaceId, id, range]`, W4 `['webhook-subscription', workspaceId, id]` with the W1
   row as `placeholderData`. Every write invalidates W1, W2 and W4.
7. **W2 measure.** The spec asks to time W2 on 500 000 deliveries. Done once on a local database seeded with
   synthetic deliveries; the timing goes in the PR description. Above one second, W2 switches to one query
   per subscription (spec rule).
8. **`validateSearch` for `settings/$section`.** Exported `parseSettingsSearch(search)` returns
   `{ webhook?: string }` (number or string coerced to a string, anything else dropped); tested in
   `router.test.ts` (new file, the Content PR adds its own cases).
9. **Strings.** All new strings through Lingui; `.po` extraction and the 7 translations last, after the
   rebase on `dev`, as the coordinator asked.

## Files and tests (TDD: tests first)

| File | Change | Test file and cases |
|---|---|---|
| `components/layout/format.ts` | `relativeDate`, `useRelativeDate` | `format.test.ts`: under 7 days relative in English and French; 7 days or more as a date; future date; ISO string |
| `components/layout/KeyValueList.tsx` | `layout?: 'inline' \| 'stacked'`, `columns?: 1 \| 2 \| 3` | `KeyValueList.test.tsx`: stacked in 1, 2, 3 columns (grid class), inline unchanged by default, empty value dash in stacked |
| `components/settings/SettingsSidebar.tsx` | `SETTINGS_SECTIONS` new order, `SETTINGS_GROUPS`, `RailSection` per group, lucide icons, sentence-case labels | `SettingsSidebar.test.tsx`: every key of `SETTINGS_SECTIONS` in exactly one group; Security absent for a non-root member; Audit log and Danger zone for owner only, Licence for root only; active entry; group titles rendered |
| `pages/WorkspaceSettingsPage.tsx` | default `general`, `max-w-[900px]`, skeletons, `canManageWebhooks`, members error | `WorkspaceSettingsPage.test.tsx` (new): unknown section → `general`; skeleton without workspace; General skeleton while S1 pending, never the read-only view first; `canManageWebhooks` true for owner, true for `webhook_subscriptions.write`, false otherwise |
| `router.tsx` | `/settings` → `general`; `validateSearch` on `settings/$section` | `router.test.ts` (new): `parseSettingsSearch` with a string, a number, a boolean, an empty string, absent |
| `components/settings/SettingsRow.tsx` (new) | label, help, field, `htmlFor`, `action` | `SettingsRow.test.tsx`: label bound to the field via `htmlFor`; help rendered; container-query classes (`@max-[720px]:grid-cols-1`); `data-settings-field` present |
| `components/settings/GeneralSettings.tsx` | four panels of rows; logo popover with `logo_url` always mounted; read-only rows | `GeneralSettings.test.tsx`: current cases on new labels (save, Discard, emptied domain sent as `''`, default language replaced); saving without opening Replace keeps `logo_url`; Remove empties the logo; empty name shows the error under the field; read-only view has no input nor save bar and lists the languages; consent help in the info tooltip |
| `components/settings/LogoInput.tsx` | rendered in the popover, logic kept | `LogoInput.test.tsx` (new): detect from website calls `detectFavicon`; error without website |
| `components/settings/permissionLabels.ts` (new), `accessSummary.ts` (new) | shared resource labels; `summarizeAccess` | `accessSummary.test.ts`: owner, invitation, full access with and without opt-in permission, no access, partial with "+n", resource order (opt-in last), audit-logs-only, "read only" without write and absent with a write, count kept for the tooltip |
| `components/settings/PermissionsMatrix.tsx` | reads `permissionLabels.ts` | `PermissionsMatrix` existing tests (via `EditPermissionsDrawer.test.tsx`) |
| `components/settings/WorkspaceMembers.tsx` | two panels, Member column with name, menus, row key `invitation_id ?? user_id` | `WorkspaceMembers.test.tsx`: current cases on new labels (Invite, New key): invite, create key, edit permissions, resend, delete, plan limit; members and keys in the right table; name else email; "(you)"; invitation sent date and expiry; Platform admin without joined date; two invitations without key clash; no menu for a non-owner nor on an owner row; error `Result` with Retry |
| `components/settings/integrations/providerDetails.ts` (new) | `providerDetails(integration): DetailRow[]` | `providerDetails.test.ts`: SES, SparkPost, Postmark, Mailgun, Mailjet, SMTP (Basic, OAuth2 Microsoft and Google), SendGrid… every provider the current `renderProviderSpecificDetails` handles; marketing rate limit per hour and per day; `credential_hints` shown as `••••xxxx`; nothing for a secret without hint |
| `components/settings/integrations/IntegrationCard.tsx`, `EmailProviderCard.tsx` (new) | card frame; email card | `EmailProviderCard.test.tsx`: header with roles and ID tooltip; webhook pill (registered, not registered, inactive, checking, unavailable) and endpoint list on hover; `webhookStatus.error` alert; menu by role, Mailjet without "Use for marketing", confirmations; senders footer with names and default; read-only without buttons |
| `components/settings/Integrations.tsx` | role summary, two blocks, other-integrations grid, catalogue buttons for owner only | `Integrations.test.tsx`: current cases on new labels; "Marketing: not set"; other-integrations grid with Supabase hook by hook; no email provider `Empty`; other block hidden without integrations; catalogue without buttons for a non-owner |
| `components/settings/CustomFieldsConfiguration.tsx` | table, free slots, texts | `CustomFieldsConfiguration.test.tsx`: current cases (Name a free slot); `setCustomFieldLabels` called; order of `ALL_CUSTOM_FIELDS`; free slots per type; all named; none named; disabled menu without right |
| `components/settings/WebhooksSettings.tsx` | table, W1/W2 in `useQuery`, panel via `webhook` | `WebhooksSettings.test.tsx`: rate and tones (≥99 success, 90–99 warning, <90 danger), "—" without delivery, "No successful delivery", write buttons disabled without right, W1 loading skeleton, W1 403 `Result`, other error with Retry, panel opened from the URL |
| `components/settings/WebhookCard.tsx` → `WebhookDetail.tsx` | panel body loaded by W4, stats in `useQuery` | `WebhookDetail.test.tsx` (renamed): W4 with the W1 row as placeholder; 404 closes the panel with the info message; secret for owner only; test hidden for Zapier; deliveries link with `subscription_id`; confirmations; W1, W2, W4 invalidated after a write |
| `SMTPBridgeSettings.tsx`, `BlogSettings.tsx`, `WebAnalyticsSettings.tsx`, `audit_logs/AuditLogsSettings.tsx`, `audit_logs/AuditLogRetentionCard.tsx`, `LicenseSettings.tsx` | in `Panel`s | existing tests, updated where they read "Save Changes" |
| `components/settings/DeleteWorkspace.tsx` | `Panel` with `danger-200` border | `DeleteWorkspace.test.tsx` (new): title, confirmation, delete call |
| `services/api/workspace.ts` | `name?: string` on `WorkspaceMember` | type check |
| `e2e/protected-pages.spec.ts`, `e2e/features/workspace-settings.spec.ts` | new rail, labels, default section | the tests themselves |
| `src/i18n/locales/*.po` | extraction then translations in the 7 catalogues | `catalogues.test.ts` |
| `CHANGELOG.md` | Feature line of the spec (44.0) | — |

## Verification

1. `cd console && npx vitest run <touched files>` while working, then `npx vitest run` once.
2. `npm run lint`, `npx tsc -b`, `npm run build`.
3. `npm run lingui:extract`, translations, `npx vitest run src/i18n`.
4. `npm run test:e2e -- --trace off e2e/protected-pages.spec.ts e2e/features/workspace-settings.spec.ts`.
5. Screenshots of the five sections at 1440 and 1280 px, dev server on port 19173, stopped afterwards.

## Spec review (Opus, effort xhigh) and what the plan takes from it

Each finding was checked in the code; the plan changes as follows.

1. **Docs PR** (separate, `Notifuse/docs` from `dev`): the spec's pages plus `integrations/zapier.mdx:40`,
   `features/email-tracking.mdx:19-20`, `features/templates.mdx:69-70`, `features/workspaces.mdx:20-22`,
   `blog/setup.mdx:19`; screenshots of Team, Integrations, Webhooks.
2. **CHANGELOG**: `workspaces.members` `name` is already in the 44.0 entry of #527; the screen line drops it.
3. **Save bar texts** are already those of spec 2 on `dev` (`SaveBar.tsx`); not presented as a change.
4. **Skeletons**: `membersLoaded` is set at the first answer and never reset by a refetch (a General save
   refreshes the workspaces, which refetches the members; General must not unmount). It resets only when
   the workspace changes; a stale response (other workspace) is ignored. Every section whose view depends on
   the owner or a right shows the skeleton until then (General, Integrations, Custom fields, Webhooks, Blog,
   Web analytics, Audit log, Danger zone).
5. **W1 cache shape**: the cache under `['webhook-subscriptions', ws]` holds the raw `{ subscriptions }`
   response, as the Logs tab writes it; Settings unwraps with `select`. W4's placeholder is
   `{ subscription: row }`.
6. **Logo**: one always-mounted `Form.Item name="logo_url"` with the URL rule, whose child is a `LogoField`
   (value/onChange): preview, "Replace" popover (URL input and "Detect from website"), "Remove". Its onChange
   goes through the form, so it marks the form dirty and the rule's error shows under the row. Tests: Detect
   raises the save bar; an invalid URL shows the error.
7. **Avatars**: `initialsFor` is imported from `layouts/app-shell/gravatar.ts`; the logo fallback reuses
   `WorkspaceBadge`, exported from `WorkspaceSwitcher.tsx` (shared change, signalled).
8. **Tokens**: no `danger-200` nor `surface-2`. `Panel` gains `tone?: 'danger'` (border `border-danger-fg/25`,
   with a Panel test); `surface-2` is `bg-subtle`.
9. **Sticky SaveBar**: `Panel` clips its content, so the bar is the last child of the section, outside the
   panels, in General, Blog, Web analytics and Retention. `@container` sits on `SettingsRows` (rows) and in
   `KeyValueList` (stacked).
10. **Form item margin**: through the theme, not CSS: `SettingsRows` wraps its rows in a `ConfigProvider` with
    `components.Form.itemMarginBottom = 0` (pattern of `TriggerConfigForm.tsx`). No rule in
    `antd-overrides.css`.
11. **Relative dates**: one helper for both plans, `relativeDate(locale, date, now?)` / `useRelativeDate()` in
    `components/layout/format.ts`, built on `Intl` with the console locale (dayjs has no locale set).
12. **Catalogue for non-owners**: buttons hidden and the row itself no longer clickable (no onClick, no
    pointer); tested by a row click.
13. **providerDetails**: `DetailRow` gains `tone?: 'warning'`; labels and values are translated (`msg`/`t`),
    including today's untranslated Yes/No/Enabled/Disabled/Basic/emails per min; SendGrid shows what it has.
14. **Declared differences**: "No access" loses its hover (spec); the webhook-registration error keeps its
    `error` Alert, shown only when webhooks are registered as today; platform admin = owner row whose email
    is a `ROOT_EMAIL` (the server marks nothing else; a root user who is a real owner reads the same, which
    is true), tested; API-key confirmation becomes one translated message; row confirmations use
    `modal.confirm` (a Popconfirm in a dropdown unmounts with the menu).
15. **Regression tests kept**: the access-count cases move to the summary (count in the details popover);
    `WebhooksSettings.test.tsx` keeps its drawer cases (empty custom filter sent, list/segment ids echoed,
    no `enabled`) with the edit opened from the detail panel; `WebhookDetail.test.tsx` keeps the auto-disabled
    reason and count; `__tests__/PermissionsMatrix.test.tsx` runs unchanged.
16. **e2e**: `protected-pages.spec.ts:171` text; `workspace-settings.spec.ts` lines 47, 52-59, 84, 93, 112,
    132, 143, 175, 184, 220, 229, 247, 317, 323, 326, 355, 361, 372. Playwright runs its own dev server on 5173
    inside this sandbox, stopped afterwards.
17. **Spec items added to the work**: W2 with `keepPreviousData`, a minute-rounded `from` and a Refresh button,
    W2 error "—" with "Couldn't load"; J/K through `SidePanel` `nav`; section titles and subtitles; SMTP bridge
    `Descriptions` → `KeyValueList`; Zapier "Disconnect"; "Available integrations"; Reply forwarding row;
    senders "+ Add"; invitation expired; empty API keys; key name from its address; row order; logo load
    error; the three `team` defaults (page twice, router). `__tests__/pages.smoke.test.tsx` runs after each
    component change.
