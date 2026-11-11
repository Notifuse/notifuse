# Transactional notification create and edit permissions plan

**Status**: plan reviewed
**Date**: 2026-11-11
**Target**: 44.0. On `dev` the WIP section is still `## [43.2] - WIP` (v43.0 is the last release; 43.1 and 43.2 never
shipped and are merged into 44.0 by another PR, which also bumps `VERSION`). This PR touches neither section headings
nor `VERSION`. No migration, no environment variable, no backend change.
**Base**: `origin/dev` at `ffa835f`.

## Goal

On the transactional notifications page, Create and Edit are offered exactly when the member holds what the request the
console sends requires, as #506 did for the test button.

## Current behaviour

- `transactional.create` checks Transactional write (`transactional_service.go:107`), then reads the email template with
  the member's context (`:137`), so it also needs Templates read. The handler answers 201.
- `transactional.update` checks Transactional write (`:193`) and reads the template only when the body carries
  `channels` on a notification no integration manages (`:247-251`). For a managed one, `channels` is refused before any
  template read (`:228-232`).
- `UpsertTransactionalNotificationDrawer` sends `channels` on every edit, except when `integration_id` is truthy
  (`:233`).
- `TransactionalNotificationsPage`:
  - The two Create buttons (header and empty state) are enabled on Transactional write, with the tooltip "You don't
    have write permission for transactional notifications". Without Templates read, the drawer's template picker
    (`TemplateSelectorInput`, `templates.list`) cannot load, so the required template blocks the form and it cannot be
    saved. The API itself answers 403.
  - The card's Edit button shows on Transactional write. Saving a notification not managed by an integration gets a
    403 without Templates read; an integration-managed one saves.
- The permissions matrix (`services/api/permissions.ts`, from the unreleased #506) says the console sends `channels`
  on every edit, which is not true for integration-managed notifications. So does the docs sentence of
  `features/admins-api-keys.mdx:159` (docs `dev`, #28).

## Design

1. `console/src/services/api/permissions.ts`:
   - `canCreateTransactionalNotification(permissions)`: `Boolean(transactional.write && templates.read)`.
   - `canEditTransactionalNotification(permissions, notification: { integration_id?: string | null })`: Transactional
     write, plus Templates read unless `!!notification.integration_id` (an empty string is not managed, as in the
     drawer and the server).
   - Strings, all from #506:
     - Templates read, `transactional.create` and `transactional.update` (one shared msgid): "Read the template of a
       transactional notification when creating it, or when editing it with its channels, which the console sends
       unless an integration manages the notification (also needs Transactional write)".
     - Transactional write, `transactional.update`: "Edit a transactional notification (an edit that sends its
       channels, which the console does unless an integration manages the notification, also needs Templates read)".
2. `TransactionalNotificationsPage.tsx`:
   - The duplicated Create drawer block (header and empty state) becomes one element, disabled unless
     `canCreateTransactionalNotification`. Its tooltip becomes "You need Transactional write and Templates read
     permissions to create notifications".
   - The card's Edit button shows when `canEditTransactionalNotification`, as the card's other actions already hide
     rather than disable.
3. i18n: `npm run lingui:extract` (never `--clean`, which would drop shipped obsolete entries), then translate the new
   strings in the 8 catalogues.
   - The old tooltip shipped in v43.0, so its obsolete entry stays, and so do `#~ "Create a transactional
     notification"` and `#~ "Edit a transactional notification"`.
   - The two #506 msgids being reworded never shipped, so their obsolete entries are deleted by hand in the 8
     catalogues.
4. `CHANGELOG.md`: a `Fix` line in the top WIP section as it stands when implemented, since the Create and Edit
   buttons' check shipped in v43.0. The catalogue wording from #506 never shipped and gets no bullet. Text: "**Fix**:
   Create Notification is now disabled, and Edit hidden, on the transactional notifications page for members without
   Templates read: creating offered a template picker they could not load, and saving an edit was refused. Editing a
   notification managed by an integration stays available with Transactional write alone."
5. Docs, its own PR to docs `dev`: `features/admins-api-keys.mdx:159` says "which the console sends when editing a
   notification that no integration manages".

## Tests

| Implementation | Test file | Cases |
|---|---|---|
| `services/api/permissions.ts` | `services/api/permissions.test.ts` | create: true with both; false without Templates read; false without Transactional write; false for null and undefined. Edit: true with both; false without Templates read; true without Templates read when `integration_id` is set; false without Templates read when `integration_id` is `''`; false without Transactional write even when managed; false for null |
| `pages/TransactionalNotificationsPage.tsx` | `pages/TransactionalNotificationsPage.test.tsx` | Drawer mocked as a button exposing `notification?.id` and `buttonProps.disabled`; the list mock is per test. Create (header, and the empty state with an empty list) disabled with Transactional write and no Templates read, with the new tooltip on hover; enabled with both. Edit hidden without Templates read, shown with both, shown without Templates read for an integration-managed notification |
| Backend requirement mirrored by the console | `tests/integration/transactional_handler_test.go`, the "Template Testing Permissions" `t.Run` gains nested cases reusing its `memberClient` | Transactional write only: create → 403 `templates`/`read`; update with `channels` → 403 `templates`/`read`; update of the description alone → 200; update of a managed notification (`IntegrationID` set via a `TransactionalNotificationOption`) with the console's body (name, description, tracking settings, no `channels`) → 200. With Templates read too: create → 201; update with `channels` → 200 |
| catalogues | `i18n/locales/catalogues.test.ts` | every entry translated; compilation checked by `npm run build` |

## Known, not handled

- Whether creating or editing a notification should need Templates read at all is a permission change, left out.
- Editing a managed notification without Templates read saves, but the drawer's template field shows blank (its
  `templates.list` and `templates.get` answer 403), as the card's template preview already does.
- The drawer could omit `channels` when unchanged, so a description-only edit would need only Transactional write.
- `broadcasts.sendToIndividual` is still not listed under Templates read (#506).

## Test commands

The `tests/compose.test.yaml` stack (Postgres, Mailpit) must be up.

```
INTEGRATION_TESTS=true go test -race -tags integration,licdev -timeout 20m ./tests/integration/ -run TestTransactionalHandler -v
cd console && npm run lingui:extract && npx vitest run && npx tsc -b && npx eslint . && npm run build
```
