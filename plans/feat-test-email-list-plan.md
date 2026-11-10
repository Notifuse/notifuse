# Test email for a mailing list plan (issue #423)

**Status**: plan reviewed in two rounds, implemented
**Date**: 2026-11-10
**Target**: 43.1. No migration, no environment variable.
**Base**: `origin/dev` at `c859e1c`.

## Goal

A test email sent from the console's "Send Test Email" dialog can be sent for a mailing list. It then carries that
list's real links (`unsubscribe_url`, `oneclick_unsubscribe_url`, `confirm_subscription_url`, `{{ list.name }}`) and
the `List-Unsubscribe` / `List-Unsubscribe-Post` headers, and unsubscribing from it works. Without a list the test is
sent as before, except that its history row is recorded under the contact's lowercase address.

## Current behaviour

- Every console test goes through `SendTemplateModal` → `POST /api/transactional.testTemplate`, the broadcast page
  included. `broadcasts.sendToIndividual` is called by no screen.
- `TransactionalNotificationService.TestTemplate` builds `ContactWithList{ListID: "foo", ListName: "bar"}` and never
  sets `EmailOptions.ListUnsubscribeURL`; the recorded `MessageHistory` has no `ListID`, and its `ContactEmail` is the
  address as typed.
- `domain.BuildTemplateData` only builds list links when `ListID`, `WorkspaceID` and `Contact` are set.
- Every provider sets both headers once `EmailOptions.ListUnsubscribeURL` is set.
- `ListService.unsubscribeFromList` creates an `unsubscribed` row for a recipient with no row on the list only when
  `WasSentForList(messageID, email, listID)` holds, which reads `message_history.list_id` and compares
  `contact_email` exactly against the link's lowercase address.
- A broadcast that keeps the list's unsubscribed contacts (`exclude_unsubscribed = false`) sends the list's links but
  no header (`Broadcast.SendsListUnsubscribeHeader`).

## Design

### API

`POST /api/transactional.testTemplate` takes two optional fields at request level:

- `list_id`: the list the test is sent for. Empty means today's behaviour.
- `omit_list_unsubscribe_header` (default `false`): with a list, keep its links but leave out the header, as a notice
  broadcast does. No effect without a list.

### Backend

1. `internal/domain/transactional.go`: `TestTemplateRequest.ListID` and `OmitListUnsubscribeHeader`; `Validate()`
   refuses a `list_id` over 32 characters or not alphanumeric (the `List.Validate` rules). The
   `TransactionalNotificationService.TestTemplate` interface method gains `listID string, omitListUnsubscribeHeader bool`
   after `language`; its mock is regenerated alone with the `//go:generate` line of that file.
2. `internal/service/transactional_service.go`:
   - `listRepo domain.ListRepository` in the service and its constructor (wired in `internal/app/app.go`).
   - With a list: `lists:read` is required on top of `transactional:write` (the list's name goes into the email);
     the list is read with `GetListByID` before the contact is upserted, so an unknown or deleted list leaves nothing
     behind and the wrapped `ErrListNotFound` is returned; `ContactWithList` gets the list's id and name;
     `EmailOptions.ListUnsubscribeURL` is set from `oneclick_unsubscribe_url`, replacing any URL the caller passed, or
     cleared when the header is omitted; `MessageHistory.ListID` is set.
   - Always: `MessageHistory.ContactEmail` is the contact's address (the fallback contact gets
     `domain.NormalizeEmail(recipientEmail)`), as the real transactional send records it. The provider still gets the
     address as typed.
3. `internal/http/transactional_handler.go`: passes both fields; a wrapped `ErrListNotFound` answers 404 "List not found".

### Console

4. `services/api/template.ts`: `TestTemplateRequest.list_id` and `omit_list_unsubscribe_header`.
   `services/api/transactional_notifications.ts`: `testTemplate` takes an optional last `listOptions` and only sends
   the fields when a list is given.
5. `components/templates/SendTemplateModal.tsx`: optional `defaultListId`, `omitListUnsubscribeHeader` and
   `canReadLists` props, passed by the three pages that already hold the member's permissions (no
   `useWorkspacePermissions` in the modal: the hook has no cache and the broadcast page mounts one modal per card).
   Lists load with the console's `['lists', workspaceId]` query, enabled only when the dialog is open and
   `canReadLists` is true. A "Mailing list" select under the recipient, clearable, with the default list labelled
   "(broadcast audience)"; a preselected list that is not among the workspace's lists selects nothing. The help text
   says what the list adds, that the links of the test really act on the list, and, for a notice broadcast, that
   there is no header.
6. `services/api/broadcast.ts`: `testEmailListProps(broadcast)` returns the audience list and
   `omitListUnsubscribeHeader: !exclude_unsubscribed`; `BroadcastsPage` passes it to the modal. Kept out of the page
   module so it adds no `react-refresh/only-export-components` warning.
7. `services/api/permissions.ts`: `transactional.testTemplate` listed under `lists.read`, and the `transactional.write`
   line says a list also needs Lists read.
8. i18n: the new strings in the 8 catalogues.

### Release and docs

9. `VERSION` 43.1; the web analytics SDK bundle is rebuilt since it embeds the version; two CHANGELOG improvement lines.
10. Docs repository: `features/templates.mdx`, `features/notification-center.mdx` (who carries the header),
    `features/transactional-api.mdx` and `features/admins-api-keys.mdx` (Lists read for a test sent for a list).

## Tests

| Implementation | Test file | Cases |
|---|---|---|
| `internal/domain/transactional.go` | `internal/domain/transactional_test.go` (`TestTestTemplateRequest_Validate`) | valid with a list; `list_id` of 33 characters refused; non-alphanumeric `list_id` refused |
| `internal/service/transactional_service.go` | `internal/service/transactional_service_test.go` (`TestTransactionalNotificationService_TestTemplate_WithList`) | list: compiled data carries the real `list`, `unsubscribe_url` (`lid`, `lname`) and `confirm_subscription_url`, the header URL is the list's one-click URL, history has the list; unknown list: `ErrListNotFound`, nothing sent; no `lists:read`: permission error, nothing sent; no list: no header, no list, lowercase address; caller's `list_unsubscribe_url` replaced; header omitted: list links kept, caller's URL cleared, list recorded; address typed with capitals, contact found and fallback contact: history address equals the link's `email` |
| `internal/http/transactional_handler.go` | `internal/http/transactional_handler_test.go` (`TestTransactionalNotificationHandler_HandleTestTemplate_List`) | both fields passed to the service; defaults without them; wrapped `ErrListNotFound` → 404 |
| Full stack | `tests/integration/list_unsubscribe_headers_test.go` (`test email for a list`) | Mailpit receives the list's name and `lid` in the body, both headers; the one-click POST from the delivered header unsubscribes a recipient typed with capitals who was on no row; no header when omitted; no header without a list; `success` checked in the response |
| `services/api/transactional_notifications.ts` | `services/api/transactional_notifications.test.ts` | fields sent with a list, absent without |
| `components/templates/SendTemplateModal.tsx` | `components/templates/SendTemplateModal.test.tsx` | default list preselected and labelled; notice help text and flag; unknown default selects nothing; cleared list sends none; field hidden and no list request when `canReadLists` is false or absent |
| `services/api/broadcast.ts` | `services/api/broadcast.test.ts` (`testEmailListProps`) | regular broadcast; notice broadcast; audience without a list |
| catalogues | `i18n/locales/catalogues.test.ts` | catalogues complete and compilable |

## Known, not handled

- The `foo`/`bar` placeholder list stays when no list is chosen.
- The broadcast page's test button checks `templates:read` and `contacts:write`, while the endpoint requires
  `transactional:write`.
- A test sent from a broadcast does not apply the broadcast's tracking mode, UTM parameters or broadcast variables.
- `email_options.list_unsubscribe_url` is still accepted as given from API callers without a list.

## Test commands

```
make test-domain && make test-service && make test-http
INTEGRATION_TESTS=true go test -race -tags integration,licdev -timeout 20m ./tests/integration/ -run TestListUnsubscribeHeaders -v
cd console && npx vitest run SendTemplateModal transactional_notifications broadcast catalogues && npx tsc -b && npx eslint .
cd web_analytics_sdk && npm run build && git diff --exit-code -- dist package.json
```
