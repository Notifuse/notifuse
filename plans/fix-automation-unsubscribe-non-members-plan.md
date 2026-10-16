# Fix: unsubscribe links that fail for contacts with no row on the list (B1)

Created 2026-10-15. Checklist item B1 in `plans/made-up-tests-audit-checklist-plan.md`. Status: **implemented, on `dev`; docs PR and console label done 2026-10-16** (see "Implementation" at the end).

- Decisions D1–D5 were taken on 2026-10-15, after a review of competitors' 2026 docs.
- An independent review checked every claim and test helper against the code; its corrections are folded in.
- HEAD moved during planning (`c974080a favicon` reshuffled `internal/http/public_handler.go`), so the line numbers below are from HEAD `c974080a`.

## What a recipient sees

An automation with a list emails a contact who has no row on that list. They click the unsubscribe link, or their mail client sends the RFC 8058 one-click POST, and get `500 {"error":"Failed to unsubscribe from lists"}`. The next time the trigger fires, they get the email again. Reproduced on a delivered email: in-body link 500, one-click 500, no row written, second email delivered. The control contact (on the list) was unsubscribed and received nothing more.

## Verified facts

- **F1 — The send gate lets non-members through.** It only skips a marketing or blog email when the contact's row on `Automation.ListID` is unsubscribed, bounced or complained; "not found" means "proceed with sending" (`internal/service/automation_node_executor.go:248-256`, categories at `:185-192`). A skipped contact exits with reason `unsubscribed` (`:277-288`).
- **F2 — The email carries that list's links regardless of membership.** `ListID`/`ListName` come from `Automation.ListID` (`automation_node_executor.go:300-314`), so the email gets `List-Unsubscribe` and `unsubscribe_url` for that list. Automations also add `utm_*` parameters to links (`:502-505`).
- **F3 — The unsubscribe can only update an existing row.** `UnsubscribeFromLists` (`internal/service/list_service.go:521-609`) calls `UpdateContactListStatus`. That is a plain UPDATE returning `&ErrContactListNotFound{}` on 0 rows (`internal/repository/contact_list_postgres.go:233-270`, `:266`).
  - `Error()` has a pointer receiver (`internal/domain/contact_list.go:381`), and nothing wraps the repository, so `errors.As` needs a `*domain.ErrContactListNotFound` target.
  - Both callers turn any error into a 500: `internal/http/public_handler.go:242-244` for `/unsubscribe` and `:319-321` for `/unsubscribe-oneclick`.
- **F4 — So nothing is ever recorded.** The gate never stops the next send. The message-history `unsubscribed` event is also lost, because it is written only after every list succeeded (`list_service.go:591-608`).
- **F5 — Contacts *removed* from the list already work.** The gate reads with `deleted_at IS NULL` (`contact_list_postgres.go:132-157`), so removed contacts receive the email too. Their unsubscribe succeeds because `UpdateContactListStatus` doesn't filter on `deleted_at` and clears it, so the bug is strictly "no row at all".
- **F6 — A safe upsert already exists.** `AddContactToList` is `INSERT … ON CONFLICT (email, list_id) DO UPDATE … WHERE status NOT IN ('bounced','complained')` (`contact_list_postgres.go:25-56`). It never overwrites a bounce or complaint and raises no error when it skips.
- **F7 — Inserting a row born `unsubscribed` emits `list.unsubscribed`, never a subscribe event.**
  - Timeline trigger: `internal/database/init.go:646-653`, same mapping in `internal/migrations/v23.go:57-64`.
  - Outgoing-webhook trigger: `internal/database/schema/webhook_triggers.go:139-147`, installed on existing databases by `v39.go:177`. Its payload carries `previous_status: null`.
  - A concurrent duplicate opt-out becomes a same-status UPDATE, which both triggers ignore.
- **F8 — Nothing stops writing a row for an erased contact.** `contact_lists` has no foreign keys (`init.go:143-151`), and contact deletion hard-deletes the contact's list rows (`internal/service/contact_service.go:211`) before the contact row (`:229`). A naive insert would therefore write an erased person's address back when they click an old link.
- **F9 — Only recipients reach this function.** The only callers are the two public handlers, both passing `hasBearerToken = false` (`public_handler.go:242`, and `:319` with `fromBearerToken := false` at `:317`).
- **F10 — A deleted list fails the same way.** Lists are soft-deleted, and both `GetLists` and `GetListByID` hide them (`internal/repository/list_postgres.go:95`, `:67`). The email node therefore cannot send for a deleted list (`automation_node_executor.go:302-305`), and only emails sent before the deletion carry its links. Their unsubscribe answers "list not found", which becomes a 500 (`list_service.go:574-577`).
- **F11 — The harness doesn't build the notification-center widget**, so its page isn't served. On load, the widget sends `{wid, email, email_hmac, lids: [lid], mid}` (`notification_center/src/App.tsx:195-201`) as a POST to `/unsubscribe` (`src/api/notification_center.ts:156-160`). The e2e test replays exactly that, with values taken from the delivered link.
- **F12 — How tests hid it:**
  - Every test in `tests/integration/automation_e2e_test.go` that reaches an email node subscribes the contact first; `testAutomationContactCreatedTrigger` enrols a non-member but stops at a delay node. None opens the delivered email.
  - `TestEmailNodeExecutor_Execute_MarketingEmail_ContactNotInList` (`internal/service/automation_node_executor_test.go:1594`) asserts the non-member send is queued.
  - `TestListService_UnsubscribeFromLists` (`internal/service/list_service_test.go:1205`) never makes the repository return `ErrContactListNotFound`, and its `error - list not found` subtest (`:1437`) asserts the 500-producing error for a deleted list.
- **F13 — List broadcasts include unsubscribed, bounced and complained rows unless `exclude_unsubscribed` is true.** See `internal/repository/contact_postgres.go:1458-1462` (recipients) and `:1742-1746` (count); these are the only two readers of the flag. A `broadcasts.create` body that omits the key stores `false`: it's a plain `bool` (`internal/domain/broadcast.go:169`) with no default, and only the console sends `true` (`console/src/components/broadcasts/UpsertBroadcastDrawer.tsx:196`). Without D5, B1's fix would add non-members who opt out to every such broadcast.
- **F14 — A segment's `contact_list` condition with no status matches any row** (`internal/service/query_builder.go:1222-1250`). The console requires a status for "is in list" (`console/src/components/segment/form_leaf.tsx:248-266`). For "not in list" it checks only that no row exists.
- **F15 — For SMTP sends, `message_history` is written after the send** (`internal/service/queue/worker.go:470`); the pre-send write at `:421` covers only providers that set their own Message-ID. An unsubscribe that lands before that write updates nothing and raises no error.

## The fix

### Part 1 — an unsubscribe with a valid HMAC never fails

**Change — `internal/service/list_service.go`, `UnsubscribeFromLists`:**
1. **No row on the list.** When `UpdateContactListStatus` returns `*domain.ErrContactListNotFound` (checked with `errors.As`, F3), record the opt-out instead of failing:
   1. `contactRepo.GetContactByEmail(ctx, workspaceID, email)`:
      - **`domain.ErrContactNotFound`** (`internal/repository/contact_postgres.go:160-162`): the contact was erased or never existed. Log at info and continue, writing nothing (F8, D3).
      - **Any other error**: return it. An infrastructure failure must stay a 500 so mail providers retry it, rather than a silent 200.
   2. `contactListRepo.AddContactToList(&domain.ContactList{Email, ListID, Status: ContactListStatusUnsubscribed})`. This reuses the existing upsert (F6), so there is no new SQL and no interface or mock change.
2. **Deleted or unknown list (D4).** Replace `return fmt.Errorf("list not found")` (`:574-577`) with a warning log and `continue`.
3. Keep the loop's error wrapping. The message-history `unsubscribed` event is now recorded for these contacts too (F4).

**Side effects of the inserted row, accepted:**
- **The contact appears on the list as `unsubscribed`.** The list's unsubscribed count includes them, and a `list.unsubscribed` timeline entry and webhook event fire (F7). This is what ActiveCampaign documents for this case (D2).
- **The next marketing email from that automation exits them** with reason `unsubscribed` (F1).
- **Broadcasts:** with D5 (Part 2), an `unsubscribed` row is mailed only by a broadcast that explicitly sets `exclude_unsubscribed: false`, which is a deliberate send to that list's unsubscribed contacts.
- **Segments (F14):**
  - A console-built "is in list X" always carries a status, so the row matches only "with status unsubscribed", which is accurate.
  - The contact leaves "not in list X" segments and `segment.left` fires. That's the right outcome for someone who opted out of that list.
  - Only API-built segments that omit the status would newly include them.
- **Notification center:** a private list now shows as unsubscribed in their preferences, and switching it back on fails (`list_service.go:348-349`), exactly as for any private-list member.
- **Erasure race:** contact deletion removes list rows before the contact row (F8). An unsubscribe landing between those two statements passes the existence check and writes the row back. The window is a few statements; accepted.

### Part 2 — broadcast audiences never mail bounces or complaints, and exclude unsubscribed contacts unless asked (D5)

**Change — `internal/domain/broadcast.go`:** give `CreateBroadcastRequest` an `UnmarshalJSON`. Copy the alias pattern of `UpdateBroadcastRequest.UnmarshalJSON` (`:523`), which records an omitted `audience.exclude_unsubscribed` with the existing `jsonKeyOmitted` (`internal/domain/automation.go:284`, which treats `null` as omitted). Then `Validate` (`:451`, called by `BroadcastService.CreateBroadcast` at `internal/service/broadcast_service.go:98`) sets `ExcludeUnsubscribed = true` when the key was omitted. Go callers building the struct directly keep their value; the demo seeder already sets `true` (`internal/service/demo_service.go:1540`).

**Change — `internal/repository/contact_postgres.go`, `GetContactsForBroadcast` (`:1458`) and `CountContactsForBroadcast` (`:1742`):** always exclude `bounced` and `complained` rows, and exclude `unsubscribed` rows only when `ExcludeUnsubscribed` is true. Broadcasts always have a list (`broadcast.go:399`), so the list branch is the only one to change. Keep the two queries identical so the shown count matches the send.

**Change — API docs:**
- `openapi/components/schemas/broadcast.yaml:207-210` (`AudienceSettings`): add `default: true`, and say that omitting the key excludes unsubscribed contacts, and that bounced and complained contacts are never mailed.
- `:232-236` (`AudienceSettingsPatch`): "send false to mail them too" now covers unsubscribed contacts only.
- Re-bundle `openapi.json` and copy it to `../docs`. Another session has uncommitted changes in `openapi/` and `openapi.json`, so bundle once those have landed, or keep them out of this commit.

## Tests — write first; they must fail before the change

**`tests/integration/automation_unsubscribe_delivered_test.go` (new), `TestAutomationUnsubscribeAsDelivered`.**

Setup:
- Model it on `TestListUnsubscribeHeaders`: SMTP provider to Mailpit, `StartBackgroundWorkers` (it also starts the automation scheduler), and `SetWorkspaceCustomEndpointURL(workspace.ID, serverURL)` so links are absolute.
- One list, and a **marketing** template (the factory default category, `tests/testutil/factory.go:167`) whose body carries `<a href="{{ unsubscribe_url }}">`, injected with `testutil.WithTemplateEmailContent` (`factory.go:970`) as `broadcast_liquid_template_test.go:441` does.
- An automation with that list: a `custom_event` trigger with `frequency: every_time`, then trigger → email node. Activate it.
- A helper that creates a fresh contact, fires the event (`factory.CreateCustomEvent`), and returns the delivered message (`waitForEmailAndGetMessage`).
- **Before any unsubscribe, poll until that message's `message_history` row exists** (F15), via `factory.GetWorkspaceDB` (`factory.go:2143`).
- Every link is read from the delivered email and used unchanged: no `ComputeEmailHMAC`, no path re-joined onto the server URL. Reuse `listUnsubscribeURL` and `requestListUnsubscribeURL` (`tests/integration/list_unsubscribe_headers_test.go:286`, `:306`). Parse body hrefs with `golang.org/x/net/html`, already a direct dependency, and read query parameters rather than comparing whole URLs (F2 `utm_*`).

Subtests:
1. **Non-member, one-click POST from the delivered `List-Unsubscribe` header.** Run it url-encoded and multipart, each with a fresh contact. Expect 200, a `contact_lists` row with status `unsubscribed`, and `message_history.unsubscribed_at` set. Fire the event again: `waitForAutomationStatus` (`automation_e2e_test.go:119`) reports `exited` with exit reason `unsubscribed`. That's a positive check, since `GetContactAutomation` returns the latest enrolment (`factory.go:2561-2563`) and the first one is `completed`.
2. **Non-member, the in-body link replayed as the widget sends it.** Take the `/notification-center` href from the delivered HTML, read `wid`, `email`, `email_hmac`, `lid` and `mid`, then `POST /unsubscribe` with `{wid, email, email_hmac, lids: [lid], mid}` (F11). Expect 200 and status `unsubscribed`.
3. **Member (control).** The same one-click on a subscribed contact gives 200, and the next trigger exits them as `unsubscribed`. This passes before the change too.
4. **Erased contact.** After delivery, `POST /api/contacts.delete` for the contact, then send the one-click POST from the delivered header. Expect 200, and no row in `contact_lists` or `contacts` for that address.
5. **Deleted list.** Give this subtest its own list and automation, and run it last. After delivery, `POST /api/lists.delete {workspace_id, id}`, then send the one-click POST from the delivered header. Expect 200 and no `contact_lists` row created.

Before the change, subtests 1, 2, 4 and 5 fail with 500 and subtest 3 passes.

**`tests/integration/broadcast_audience_statuses_test.go` (new), `TestBroadcastAudienceStatusesAsDelivered` (Part 2).**
- Set up one list with four contacts: `active`, `unsubscribed`, `bounced` and `complained`.
- Create a broadcast through `client.CreateBroadcast` with a raw map whose `audience` **omits** `exclude_unsubscribed`, send it now, and read Mailpit: only the active contact gets the email.
- Do the same with an explicit `exclude_unsubscribed: false`: the active and unsubscribed contacts get it, and the bounced and complained contacts never do.
- The recipient count reported for the broadcast matches the delivered emails.
- Before the change: the first broadcast mails all four contacts, and the second mails the bounced and complained ones.

**`internal/service/list_service_test.go`, `TestListService_UnsubscribeFromLists`.** The mocks are shared and the logger expectations strict, so add the matching `WithField`/`Info`/`Warn` expectations. New subtests:
- **No row, contact exists.** `UpdateContactListStatus` returns `&domain.ErrContactListNotFound{}` and `GetContactByEmail` returns a contact. Expect `AddContactToList` with `{Email, ListID, Status: unsubscribed}`, `SetStatusesIfNotSet` called for the `MessageID`, and a nil error.
- **No row, contact erased.** `GetContactByEmail` returns `domain.ErrContactNotFound`. `AddContactToList` is not called (gomock fails on any unexpected call), and the error is nil.
- **No row, contact lookup fails.** A generic error is returned wrapped, and nothing is inserted.
- **No row, insert fails.** The error is returned wrapped.
- The existing `error - list not found` (`:1437`) becomes **unknown or deleted list is skipped**: `GetLists` doesn't return the ID, no `UpdateContactListStatus` call, nil error.

**`internal/domain/broadcast_test.go`.** Decode raw JSON bodies, not struct literals (the #401 rule), then call `Validate`:
- `audience` without `exclude_unsubscribed` → `true`;
- `"exclude_unsubscribed": null` → `true`;
- explicit `false` → `false`;
- explicit `true` → `true`.

**`internal/repository/contact_postgres_test.go`.**
- The `ExcludeUnsubscribed: false` cases (`:1557`, `:1614`, `:1718`) now expect the bounced/complained filter.
- Add a case asserting that `false` excludes `bounced` and `complained` but not `unsubscribed`, for both the recipients query and the count.

Unchanged and still valid:
- `error - update status failure` uses a generic error, which still fails.
- `TestEmailNodeExecutor_Execute_MarketingEmail_ContactNotInList`: non-members keep receiving by design (D1).

**Existing integration tests that create broadcasts without the key** include `broadcast_pagination_test.go`, `broadcast_ab_failure_test.go`, `broadcast_esp_failures_test.go`, `broadcast_partial_failure_test.go`, `broadcast_race_condition_test.go`, `broadcast_scheduled_direct_test.go` and `broadcast_phase2_pause_resume_test.go`. Run them. Any that relied on mailing unsubscribed contacts must now set `false` explicitly.

## CHANGELOG — `[41.1] - WIP`

`VERSION` is already 41.1, so don't bump it. Another session is editing this file, so add these bullets without reordering their entries. Drafts:

> - **Fix**: Unsubscribing from an automation email no longer fails for contacts who weren't on the automation's list. The unsubscribe link and the one-click unsubscribe answered with an error, and the contact kept receiving the automation's emails; the opt-out is now recorded on the list. Unsubscribing from an email sent before the contact or its list was deleted now succeeds too, without storing the address again.
>
> - **Fix**: A broadcast created through the API without `exclude_unsubscribed` no longer emails the list's unsubscribed contacts: leaving the key out now means exclude, as in the console. Bounced and complained addresses are never emailed, even with `exclude_unsubscribed: false`.

## Decisions (2026-10-15)

Taken after checking competitors' current docs (sources below).

- **D1 — Consent for non-members: keep opt-out, today's behaviour.** Contacts with no row on the automation's list keep receiving its emails (F1). This is the same default as Klaviyo and ActiveCampaign [K1][A1]. An automation meant only for list members excludes the others with node filters: a List Status Branch's "not in list" path (`../docs/features/automations.mdx:154`, `:199`) or trigger conditions. Not adopted: a per-list or per-topic opt-in mode, as in Customer.io or Resend [C1][R1].
- **D2 — Opt-outs stay on list rows; no global "unsubscribe from all".** A non-member's unsubscribe becomes an `unsubscribed` row on that list, which is exactly what ActiveCampaign documents for this case [A1]. Klaviyo, SendGrid and Postmark also record opt-outs without membership [K4][S2][P1]. Not adopted: a contact-level global flag (Customer.io, Resend, Loops, Kit [C1][R2][L2][T2]) and a suppression store separate from lists (SendGrid, Resend Nov 2025 [S1][R3]).
- **D3 — Erasure stays a pure delete.** The fix writes nothing for an erased contact (F8). Not adopted: delete-and-suppress (Customer.io [C3], Mailchimp [M4]). Brevo, Kit and Klaviyo warn about the consequence instead [B4][T4][K6], so Notifuse's docs should too (see Follow-ups).
- **D4 — Deleted or unknown list: answer 200 and write nothing.** No future email can carry a deleted list's links (F10). Gmail asks that unsubscribes be honored and Yahoo asks for it within 2 days [G1][G3]; a 500 only makes the provider retry or count it against the sender.
- **D5 — Broadcast audiences: omitted `exclude_unsubscribed` means exclude, and bounces and complaints are never mailed.** An explicit `false` still mails unsubscribed contacts, e.g. for a legal notice; Customer.io likewise allows reaching unsubscribed people only for messages marked operational [C1]. Every ESP suppresses bounces and complaints, and Postmark won't even let you delete a complaint suppression [P1]. Ships with B1 because the fix would otherwise add non-members who opt out to broadcasts that omit the key (F13).

Sources:
- [A1] https://help.activecampaign.com/hc/en-us/articles/12225946427420
- [B4] https://help.brevo.com/hc/en-us/articles/5313915904914
- [C1] https://docs.customer.io/messaging/channels/subscriptions/overview/
- [C3] https://docs.customer.io/messaging/profiles/manage/deleting-users/
- [G1] https://support.google.com/mail/answer/14229414
- [G3] https://senders.yahooinc.com/best-practices/
- [K1] https://help.klaviyo.com/hc/en-us/articles/360037101072
- [K4] https://developers.klaviyo.com/en/reference/bulk_unsubscribe_profiles
- [K6] https://help.klaviyo.com/hc/en-us/articles/24312135764251
- [L2] https://loops.so/docs/contacts/properties
- [M4] https://mailchimp.com/help/delete-contacts/
- [P1] https://postmarkapp.com/developer/api/suppressions-api
- [R1] https://resend.com/docs/dashboard/topics/introduction
- [R2] https://resend.com/docs/dashboard/contacts/managing-unsubscribe-list
- [R3] https://resend.com/docs/dashboard/contacts/migrating-from-audiences-to-segments
- [S1] https://www.twilio.com/docs/sendgrid/ui/sending-email/index-suppressions
- [S2] https://support.sendgrid.com/hc/en-us/articles/4408817749019
- [T2] https://developers.kit.com/api-reference/subscribers/unsubscribe-subscriber
- [T4] https://help.kit.com/en/articles/2502571-gdpr-faq

## Follow-ups (not in this plan)

- [x] **Docs (D1):** state in `../docs/features/automations.mdx` that an automation's emails also reach contacts who aren't on its list, and that a List Status Branch's "not in list" path is how to leave them out. Today neither the docs nor the console's "List" field say so. Done 2026-10-16 in the docs PR, together with the Part 3 docs and the `openapi.json` copy, which hadn't reached `Notifuse/docs`.
- [x] **Docs (D3):** state in `../docs` that deleting a contact doesn't prevent a later import or API upsert from re-subscribing the address. Done 2026-10-16 in the same docs PR.

## Out of scope (checklist items)

- **B24 — Contacts removed from a list keep receiving its broadcasts.** Removal is a soft delete, and neither the broadcast audience queries (`contact_postgres.go:1440-1462`, `:1730-1746`) nor the segment `contact_list` condition (`query_builder.go:1222-1250`) filters `cl.deleted_at`. It touches the same two queries as Part 2, so tackle it next.
- **B25 — A complaint or hard bounce on an automation email is never recorded on the list.** The automation queue payload has no `ListID` (`automation_node_executor.go:403-413`), so `message_history.list_id` stays NULL (`queue/worker.go:584-585`), and the status trigger acts only when it isn't (`init.go:831-861`).
- An invalid `email_hmac` also answers 500 (`public_handler.go` maps every error to 500); it should be a 4xx.
- `UpdateContactListStatus` overwrites an existing `bounced`/`complained` row with `unsubscribed`; this is pre-existing.
- Deleting a list that an automation uses isn't blocked (checklist B23).
- Test sends inject a fake list (`transactional.testTemplate`, checklist B15).

## Running the tests

- Domain, service and repository: `make test-domain`, `make test-service`, `make test-repo`. Narrower: `go test -race ./internal/service/ -run 'TestListService_UnsubscribeFromLists|TestEmailNodeExecutor'`, `go test -race ./internal/domain/ -run 'Broadcast'`, `go test -race ./internal/repository/ -run 'ContactsForBroadcast'`.
- Integration, touched tests only: start `docker compose -f tests/compose.test.yaml up -d`, then `INTEGRATION_TESTS=true go test -race -tags integration,licdev -run 'TestAutomationUnsubscribeAsDelivered|TestBroadcastAudienceStatusesAsDelivered|TestListUnsubscribeHeaders|TestUnsubscribe_Integration|TestContactDeletionErasure|TestBroadcast' ./tests/integration/ -v`.

## Implementation (2026-10-15, uncommitted)

**Code**
- `internal/service/list_service.go`: `UnsubscribeFromLists` skips an unknown or deleted list with a warning, and on `*domain.ErrContactListNotFound` calls the new `recordUnsubscribeWithoutMembership`. That helper writes nothing for an erased contact (logged without the address), returns any other lookup error, and otherwise inserts the `unsubscribed` row with `AddContactToList`.
- `internal/domain/broadcast.go`: `CreateBroadcastRequest.UnmarshalJSON` records an omitted or null `audience.exclude_unsubscribed`, and `Validate` turns it into `true`.
- `internal/repository/contact_postgres.go`: both audience queries always exclude `bounced` and `complained`. With `exclude_unsubscribed: true` the SQL and argument order are unchanged.
- `openapi/components/schemas/broadcast.yaml`, `openapi.json` re-bundled, and copied to `../docs/openapi.json`. The docs copy matched the committed bundle before the copy, and it ships with the docs PR.
- `CHANGELOG.md`: the two Fix bullets, placed after the List-Unsubscribe bullets.

**Deviation from the plan.** The no-row service cases live in their own function, `TestListService_UnsubscribeFromLists_WithoutARowOnTheList`, with fresh mocks per subtest. On the shared controller of `TestListService_UnsubscribeFromLists`, an expectation a failing subtest never consumed answered the next subtest's call. Run against the unfixed code, only 1 of the 5 failures was reported. The broadcast e2e test also has an explicit `true` case.

**Verified on an isolated stack (HEAD `c974080a`, Postgres and Mailpit of its own), with and without the fix:**
- Unit, without the fix: all 5 new service subtests fail; the omitted and null domain cases fail and the explicit cases pass; the 4 repository cases with `false` fail.
- `TestAutomationUnsubscribeAsDelivered`, without the fix: every non-member, erased-contact and deleted-list unsubscribe answers 500 `{"error":"Failed to unsubscribe from lists"}`, and the member control passes. With the fix, all 6 subtests pass.
- `TestBroadcastAudienceStatusesAsDelivered`, without the fix: omitting the key delivered to all 4 contacts, bounced and complained included, and `false` delivered to all 4. With the fix, all 3 subtests pass.
- Regression, with the fix:
  - The unit suites `./internal/domain/... ./internal/service/... ./internal/repository/... ./internal/http/...` pass.
  - The 12 integration tests on the touched paths pass: API-created broadcasts, `TestListUnsubscribeHeaders`, `TestUnsubscribe_Integration`, `TestContactDeletionErasure`, `TestWebhookSubscriptionStatusE2E`.
  - The 20 broadcast tests listed above pass except `TestBroadcastPartialFailure`, once. That is a race in the test itself (checklist B26), and it passed 3 of 3 times both with and without the fix on re-run.

**Independent review (2026-10-15)**
- **The header link opened in a browser still left non-members subscribed.** A GET on `/unsubscribe-oneclick` redirects to `action=confirm_unsubscribe`. The widget asked for confirmation only when the contact had an active row, so a non-member saw "already unsubscribed" on a public list and no prompt on a private one. Fixed in `notification_center/src/App.tsx`: a contact with no row is asked too, and a private list missing from the preferences is named `List <id>`. This GET flow is new in 41.1 (#450), so it gets no CHANGELOG bullet of its own.
  - Tests: 2 widget tests in `App.test.tsx`, which fail without the change.
  - E2E subtest: "the header link opened in a browser, then confirmed".
- **An erased contact's message was still marked unsubscribed.** Erasure redacts `message_history` rather than deleting it (`internal/repository/message_history_postgre.go:1136`). Marking the message would fire its `email.unsubscribed` webhook and write a timeline row under `DELETED_EMAIL`, so D3's "nothing written" was false. Fixed: `unsubscribeFromList` reports whether it recorded an opt-out, and the message is marked only then. A request naming only a deleted list doesn't mark it either. Unit and e2e tests assert this.
- **Open decision: an opt-out can be recorded on any list ID.** `email_hmac` covers the address, not `lids`. With one valid link, a recipient can create `unsubscribed` rows for themselves on any existing list, private ones included, and preferences then show those lists by name. Console list IDs are derived from list names, so they're guessable. Before B1, that request ended in a 500 and wrote nothing.
- Nits applied: the skipped-list comment no longer claims only a deleted list's email can carry a missing ID (test sends carry `lid=foo`). The broadcast CHANGELOG bullet now says existing broadcasts keep their saved value.
- Re-verified on the isolated stack:
  - Without the fix, 6 of the 7 e2e subtests fail and the member control passes. With it, all 7 pass, as do `TestListUnsubscribeHeaders`, `TestUnsubscribe_Integration` and `TestContactDeletionErasure`.
  - Widget: tests, lint and `tsc -b` are clean.

### Part 3 — private lists, and opt-outs bound to the email (D6, decided 2026-10-15)

Context:
- The notification center lists every list a contact has a row on, private ones included, by name.
- A valid `email_hmac` counts as authenticated in `SubscribeToListsWithResults` (`list_service.go:290-297`), and the private-list refusal applies only to unauthenticated requests (`:345-347`). So anyone holding one of their emails could subscribe their address to any private list by guessing its ID, then receive its broadcasts and read its name in their preferences. Console list IDs are derived from list names.
- That contradicts the docs, where private lists are "only accessible through direct management or API" (`../docs/features/lists.mdx:39-43`). The plan's claim above that switching a private list back on fails was wrong.

**D6 — Pierre chose "leave only" (2026-10-16), shipping with B1.** He first chose "show all, rejoin by email". A second independent review then showed that rejoining needed no email: the confirmation is the same signed `/subscribe` request, so a second click on Subscribe activates the contact. Lists without double opt-in get no confirmation email at all. The review also found that a removed contact could come back by unsubscribing first. The final rule:
- Through a link's signature, a contact can confirm a pending double opt-in the workspace started. They can never join a private list (no row), nor rejoin one they left (`unsubscribed`).
- Preferences show every public-list row, and private-list rows only while active or pending. Removed rows (`deleted_at` set) are never shown, because they keep their last status.
- An unsubscribe from a list the contact has no row on is recorded only for the list the email was sent for. Customer.io and SendGrid scope an unsubscribe to the email's own topic or group [C1][S3].

**Changes**
- `internal/domain/message_history.go`: `MessageHistoryRepository.WasSentForList(ctx, workspaceID, messageID, email, listID) (bool, error)`.
  - Implemented in `internal/repository/message_history_postgre.go` as one `EXISTS` over `message_history`, LEFT JOIN `automations`.
  - It matches `mh.list_id` (broadcasts) or the automation's `list_id` (automation sends record no list, B25).
  - Mock regenerated with mockgen v1.6.0.
- `internal/service/list_service.go`:
  - `unsubscribeFromList`, no-row path: after the contact check, require a message ID for which `WasSentForList` is true. Otherwise log and record nothing (200).
  - `SubscribeToListsWithResults`: without a bearer token, a private list with no row or an `unsubscribed` row answers `list is not public` (400, as for anonymous requests).
- `internal/service/notification_center_service.go`: `visibleContactLists` filters both `contact_lists` and `contact.contact_lists`.
- `notification_center/src/App.tsx`:
  - A private-list row offers Unsubscribe only, so the button turns into a disabled "unsubscribed" after leaving.
  - `confirm_unsubscribe` asks contacts with no row too.
- `openapi/paths/subscribe.yaml` (re-bundled, copied to `../docs`): `/subscribe` adds to public lists only, and an `email_hmac` only confirms a pending double opt-in on a private list.
- `../docs/features/lists.mdx` and `../docs/features/notification-center.mdx`: private lists can only be left from the notification center, and adding someone back is up to the workspace.

**Tests**
- `internal/repository/message_history_postgre_test.go`: the query, its arguments, and the true, false and error results.
- `internal/service/list_service_test.go`:
  - No-row unsubscribe: bound (insert), not bound (nothing, message not marked), no message ID (nothing), lookup error.
  - Subscribe through the signature to a private list:
    - with no row: refused, nothing written;
    - with an unsubscribed row: refused, no write, no email;
    - with a pending row: becomes active.
  - With a bearer token, a private list with no row is still allowed.
- `internal/service/notification_center_service_test.go`: which rows preferences keep, in both fields.
- `notification_center/src/App.test.tsx`:
  - Leaving a private list offers no Subscribe afterwards.
  - The multi-list confirm test now uses public lists, so its Subscribe assertion still holds.
- `tests/integration/automation_unsubscribe_delivered_test.go`: a delivered one-click URL whose `lids` is edited to another list records nothing.
- `tests/integration/notification_center_private_lists_test.go` (new), with the signature taken from a delivered email:
  - Joining a private list is refused.
  - A private list the contact is on is shown and can be left; it then disappears, and rejoining it is refused.
  - A list the contact was removed from isn't shown.

**Accepted**
- `list is not public` still tells a caller that a guessed private list ID exists, as it already did for anonymous requests.
- An unsubscribe on a removed row still revives it as `unsubscribed`, because `UpdateContactListStatus` clears `deleted_at`. It can no longer lead to a rejoin, and it stops mail (B24).
- `WasSentForList` reads the automation's current list, so after an automation is moved to another list, a non-member's opt-out from its older emails records nothing.

Sources: [S3] https://www.twilio.com/docs/sendgrid/ui/sending-email/group-unsubscribes. [C1] is listed above.

**Implemented and verified (2026-10-16).** On the isolated stack:
- Without the fix:
  - All 3 private-list e2e subtests fail. The join answered 200 and made the contact active, the rejoin was accepted, and the left and removed lists were shown.
  - The edited-link opt-out answered 500.
  - 3 widget tests and the preferences unit test fail.
- With the fix:
  - All 11 integration tests pass: both new ones plus the existing subscribe, double opt-in, rate-limit, unsubscribe and erasure tests. They cover `WasSentForList` on real Postgres through the automation join.
  - Widget: 39 tests pass, and lint and `tsc -b` are clean.
- CHANGELOG: a **Security** bullet for the private-list join, including the `/subscribe` change for integrations. The binding of no-row opt-outs is part of B1, which is unreleased, so it gets no bullet of its own.
- Docs edited in `../docs`: `features/lists.mdx` (also its outdated "only active contacts receive broadcasts" note), `features/notification-center.mdx` and `openapi.json`.
- Found along the way: checklist B27, where `pending` rows on lists without double opt-in are still emailed.

**Browser check (2026-10-16).** Pierre's Chrome extension wasn't connected, so this ran in a headless Chrome driven over the DevTools protocol.
- Setup: a live server from the sandbox copy, serving the widget built from it, with real emails in the sandbox Mailpit. Every link was taken from those emails.
- Checked:
  - A non-member's header link opened in the browser shows the confirmation card, and confirming records the opt-out (row checked in the database).
  - A non-member's in-body link unsubscribes on load.
  - A member's header link names the list.
  - The member's preferences show their private list but not the one they left, and don't show the removed list as subscribed.
  - Leaving the private list offers no Subscribe, and after a reload it is gone.
  - The public Newsletter still toggles.
  - No console errors in any scenario.
- Two widget defects found and fixed, then re-checked in the browser:
  - The disabled button after leaving a private list read the raw status "unsubscribed" in English on a French page. It now uses a new `unsubscribedStatus` translation in all 11 languages.
  - A private list the page can't name showed its raw ID ("List liste28c2a59"). The card now goes unnamed: its prompt already says "this list".
  - Widget tests updated: 39 pass, and lint and `tsc -b` are clean.
- The same run reproduced B27 on the public Newsletter.

### Release gate (2026-10-16), B1 only

Pierre asked for checks 1-4 before release. Everything ran on a sandbox holding exactly HEAD `c974080a` plus the B1 files, without the other sessions' uncommitted work, against its own Postgres and Mailpit.

**Changes made during the gate**
- **Unsubscribes keep bounced and complained rows.** Found by an adversarial review: before, a spam report followed by an unsubscribe left the contact `unsubscribed`, so a broadcast set to include unsubscribed contacts mailed them again.
  - Code: `unsubscribeFromList` reads the row first. Unit tests updated and added; the CHANGELOG broadcast bullet says it.
  - The admin API (`ContactListService.UpdateContactListStatus`) still overrides any status.
- **The confirmation card now also asks contacts whose double opt-in is pending.** Found by Gemini: before, the header link opened in a browser showed them nothing. Widget test added.
- **New `TestSignedLinkMembershipStateMatrix`:** 70 cases (public or private list × 7 states × 5 actions), each with its own contact and real emails. It pins every outcome, including bounced and complained rows being kept.
- **New subtest:** `lists.subscribe`, the API, still adds a contact to a private list, even one they left.
- **B26 fixed:** test only, shipping with B1.

**1. Full suites, CI's exact commands**
- **Integration (`-race -tags integration,licdev`):**
  - Run 1, before the changes above: 284 passed, 0 failed.
  - Run 2: only the B26 flake failed.
  - Run 3, on the final tree: 284 passed, 0 failed, 4 skipped (the gated browser and setup tests), and all 288 test functions ran.
- **Go unit (`go test -race` on every package but integration):** all 34 packages pass. B28 failed intermittently in earlier full runs; its path has no B1 change, and its cause wasn't captured (see the checklist).
- **Widget:** `tsc -b`, 40 tests, lint, build.
- **Console:** `tsc -b`, 1594 tests, eslint with 0 errors, build.
- **OpenAPI lint:** valid, with the same 3 warnings as HEAD (in `transactional.yaml`).

**2. State matrix:** the 70 cases above. All pass on the final tree.

**3. Mutation check:** 22 mutations, each breaking one guarantee (17 server, 5 widget). After a green baseline, every one is caught.
- The end-to-end layer alone catches 13 of the 17 server mutations.
- The four it misses are caught by unit tests: the erased-contact lookup (the message binding also covers it end to end), a multi-list request, the count query, and a message sent to someone else.

**4. Cross-model review:** Gemini (`gemini-4-argon`), after the first attempt failed on an invalid API key; findings verified against the code.
- Fixed: the pending-contact card.
- Logged in checklist B30: the read/update race on the guard, and a 500 from a confirmation link to a deleted list.
- Rejected after tracing: the rest.

The same-model adversarial pass found the complaint overwrite (fixed), B29 (an erased contact re-created through a subscribe link) and the other B30 items.

**Not covered:** the widget has no automated browser test (it was checked once in headless Chrome), and the combined tree with the other sessions' uncommitted work wasn't tested.

**Follow-ups found while implementing**
- [x] The console's broadcast switch still reads "Exclude unsubscribed, bounced & complained recipients" (`console/src/components/broadcasts/UpsertBroadcastDrawer.tsx:594`). Switched off, it now still excludes bounced and complained contacts. Reword it to "Exclude unsubscribed recipients" in all 8 catalogs once the console work another session has in progress lands, since extracting strings now would pull in theirs.
  - Done 2026-10-16 on `dev`: the label is now "Exclude unsubscribed recipients", translated in the 7 other catalogs. `lingui:extract` changed only this entry, and the i18n catalogue tests pass.
