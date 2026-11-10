# Test email button permissions plan

**Status**: plan reviewed, implemented
**Date**: 2026-11-10
**Target**: 43.1 (unreleased on `dev`). No migration, no environment variable, no backend change.
**Base**: `origin/dev` at `f7699b8`.

## Goal

The console's "Send Test Email" buttons are enabled exactly when the member holds what
`POST /api/transactional.testTemplate` requires, and their tooltip names those permissions.

## Current behaviour

- `TransactionalNotificationService.TestTemplate` requires `transactional:write`. It then reads the template through
  `TemplateService.GetTemplateByID` with the member's context, which requires `templates:read`. The contact upsert and
  lookup run system-scoped, so `contacts:write` is not needed. cc/bcc need `contacts:read` and a list needs
  `lists:read`. Checked against the live test stack: `transactional` only gives 403 `templates`/`read`;
  `templates:read` + `transactional:write` gives `success: true`; adding a cc without `contacts:read` gives 403.
- `TemplatesPage` and `BroadcastsPage` (each variation) enable the test when `templates:read && contacts:write`, with
  the tooltip "You need read template and write contact permissions to send test emails".
  - A member with `templates:read` and `contacts:write` but no `transactional:write` gets an enabled button whose
    send fails with 403.
  - A member with `templates:read` and `transactional:write` but no `contacts:write` gets a disabled button although
    the send would succeed.
- `TransactionalNotificationsPage` shows the test button when `transactional:write`, without `templates:read`: a member
  without it gets a 403 on send.
- `SendTemplateModal` hides the list field without `lists:read`, but shows CC and BCC to everyone (its
  `withCCAndBCC` prop is never read), so a member without `contacts:read` who adds a cc gets a 403.
- `services/api/permissions.ts` lists `transactional.testTemplate` under `transactional.write`, `contacts.read`
  (cc/bcc) and `lists.read`, but not under `templates.read`. `transactional.create` and `transactional.update` also
  read the template with the member's context, and are not listed under `templates.read` either.

## Design

1. `console/src/services/api/permissions.ts`:
   - Export `canSendTestEmail(permissions: StoredPermissions | null | undefined): boolean`, which returns
     `Boolean(permissions?.templates?.read && permissions?.transactional?.write)`, like the file's other predicates.
   - Add `templates.read` entries for `/api/transactional.testTemplate` ("Read the template a test email is sent
     from (also needs Transactional write)"), `/api/transactional.create` and `/api/transactional.update` ("Pick the
     template a transactional notification sends (also needs Transactional write)").
   - The `transactional.write` lines for `testTemplate`, `create` and `update` say what also needs Templates read.
2. New `console/src/components/templates/SendTestEmailButton.tsx`: the tooltip and icon button now duplicated in
   `TemplatesPage` and `BroadcastsPage`, disabled unless `canSendTestEmail`. Its tooltip says "Send Test Email", or
   "You need Templates read and Transactional write permissions to send test emails". Props: `permissions`,
   `onClick`, optional `size`. Both pages use it.
3. `TransactionalNotificationsPage.tsx`: the card's test button shows when `canSendTestEmail`, so it disappears for a
   member without `templates:read`, as it already does without `transactional:write`.
4. `SendTemplateModal.tsx`: a required `canReadContacts` prop, so a page that omits it fails to type-check. Without it the CC and
   BCC fields are hidden and none are sent. The three pages pass `!!permissions?.contacts?.read`. `withCCAndBCC` is
   left as it is.
5. i18n: extract, translate the new strings in the 8 catalogues. The old tooltip shipped, so its obsolete entry
   stays; the `transactional.write` testTemplate string being reworded was added by the unreleased #503, so its
   obsolete entry is deleted.
6. `CHANGELOG.md`: a `Fix` line under 43.1, since the buttons' checks are released behaviour.
7. Docs repository, its own PR: `features/transactional-api.mdx` and `features/admins-api-keys.mdx` say a test email
   also needs Templates read (and the Templates row of the admins page lists test sends). "Transactional write alone
   is enough to send" stays, since `transactional.send` does not read the template with the member's context.

## Tests

| Implementation | Test file | Cases |
|---|---|---|
| `services/api/permissions.ts` (`canSendTestEmail`) | `services/api/permissions.test.ts` | true with both; false without `templates.read`; false without `transactional.write`; false with only `templates.read` and `contacts.write`; false for null and undefined |
| `services/api/permissions.ts` (catalogue) | `services/api/permissions.test.ts` | `templates.read` lists `/api/transactional.testTemplate`, `/api/transactional.create`, `/api/transactional.update`; existing `PermissionsMatrix` tests still pass |
| `components/templates/SendTestEmailButton.tsx` | `components/templates/SendTestEmailButton.test.tsx` | enabled with `templates:read` + `transactional:write` and no contacts write, click calls `onClick`; disabled with `templates:read` + `contacts:write` and no transactional write, permission tooltip shown on hover; disabled without permissions |
| `pages/TemplatesPage.tsx`, `pages/BroadcastsPage.tsx` | covered by `SendTestEmailButton.test.tsx`; `npx tsc -b` for the wiring | |
| `pages/TransactionalNotificationsPage.tsx` | `pages/TransactionalNotificationsPage.test.tsx` (new) | test button present with `templates:read` + `transactional:write`; absent without `templates:read` |
| `components/templates/SendTemplateModal.tsx` | `components/templates/SendTemplateModal.test.tsx` | CC/BCC fields hidden without `canReadContacts`, shown with it; a cc typed with it is not sent once it is gone |
| Backend requirement mirrored by the console | `tests/integration/transactional_handler_test.go`, new `t.Run` in `TestTransactionalHandler` that keeps the SMTP integration and logs in a second member with `testutil.NewAPIClient` | `transactional:write` only: 403 with `resource` `templates` and `permission` `read`; `templates:read` + `transactional:write`, no contacts: 200 with `success: true`, using the integration's real sender |
| catalogues | `i18n/locales/catalogues.test.ts` | every entry translated; compilation checked by `npm run build` |

## Known, not handled

- Whether a test should need `templates:read` at all (the backend could read the template system-scoped) is a
  permission change, left out.
- The transactional notifications page offers Create and Edit on `transactional:write` alone, while both also need
  `templates:read`; only the catalogue is corrected here.
- `SendTemplateModal`'s `withCCAndBCC` prop is still unused.
- `broadcasts.sendToIndividual` also reads the template with the member's context and is not listed under Templates
  read; no screen calls it.
- The transactional notifications page hides its test button without a hint, as it does Edit and Delete.

## Test commands

The `tests/compose.test.yaml` stack (Postgres, Mailpit) must be up.

```
INTEGRATION_TESTS=true go test -race -tags integration,licdev -timeout 20m ./tests/integration/ -run TestTransactionalHandler -v
cd console && npm run lingui:extract && npx vitest run permissions PermissionsMatrix SendTestEmailButton SendTemplateModal TransactionalNotificationsPage catalogues i18nMacro && npx tsc -b && npx eslint . && npm run build
```
