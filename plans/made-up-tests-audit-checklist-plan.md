# Made-up tests audit — bug checklist and testing recommendations

Created 2026-10-15. Status: fixes in progress. Tackle one item at a time; tick it with the commit or PR that fixes it.

## Context

Issue #450 shipped because no test used the email Notifuse actually sent. `TestListUnsubscribeHeaders` fetched the message from Mailpit only to pull the header, then built its own request: it kept the path (`baseURL + u.Path`), sent only the url-encoded POST it expected, and used `sendToIndividual` instead of the queue. The unit test `rejects non-POST methods` asserted the bug itself (`GET → 405 {"error":"Method not allowed"}`). The in-body links were never read from an email, and the widget tests mocked its URL parser.

A test is "made up" when it fabricates what the code emits instead of consuming the real output. Such a test agrees with the code by construction, so the defect lives exactly in the part it skipped. The audit sent real emails through each send path on isolated Postgres + Mailpit stacks, read them from Mailpit, and used every link and header unchanged, the way a mail client, a person, a scanner or a browser would.

Verification levels used below:

- **Reproduced**: re-run in this session against a delivered email or the real code path.
- **Code-confirmed**: every step read in the code; the effect was observed by the audit with a delivered email.
- **Audit-only**: reported by an audit agent from a delivered email, not re-run. Re-verify before fixing.

## How to work each item

1. Write the "Test first" test. It must use the real artifact (delivered email, real handler, real SDK output), and it must fail for the reported reason.
2. Fix, see it pass, run the layer tests (see "Running the tests").
3. CHANGELOG: the top section is the unreleased 42.0 (`- WIP`; B24 bumped `VERSION` to 42.0 with its v42 migration), so don't bump `VERSION` again. A schema change goes into the unreleased v42. A defect in released behaviour gets a short **Fix** bullet, or an **Improvement** when it only shows in an edge case; a rough edge that worked as designed is an **Improvement**. Keep entries to 2-4 sentences.
4. Tick the box and note the commit.

## High

- [x] **B1 — Automation emails to contacts outside the automation's list can't be unsubscribed, and keep coming** — implemented 2026-10-15, committed in 44880ee7; see "Implementation" in the plan.
  - Impact: the in-body unsubscribe and the RFC 8058 one-click both answer 500, and the next automation email is delivered anyway. That breaks the Gmail/Yahoo bulk-sender requirement for these sends.
  - Cause: `internal/service/automation_node_executor.go:254-256` sends when the contact has no row on `Automation.ListID` ("Not found … proceed with sending"), and `:301-314` still puts that list's unsubscribe links in the email. `UpdateContactListStatus` only UPDATEs and returns `ErrContactListNotFound` on 0 rows (`internal/repository/contact_list_postgres.go:266`). `internal/http/public_handler.go:242-244` and `:319-321` turn that into a 500 (line numbers as of HEAD `c974080a`). No row is ever written, so the send gate never stops the next email.
  - Hidden by: `tests/integration/automation_e2e_test.go` subscribes every contact before triggering and never opens the delivered email. `TestEmailNodeExecutor_Execute_MarketingEmail_ContactNotInList` (`internal/service/automation_node_executor_test.go:1594`) asserts the non-member send is queued. `TestListService_UnsubscribeFromLists` (`internal/service/list_service_test.go:1205`) mocks `UpdateContactListStatus` as succeeding.
  - Verified: Reproduced. Member: unsubscribe 200, next email not sent. Non-member: 500 on both paths, no row, next email delivered.
  - Plan: `plans/fix-automation-unsubscribe-non-members-plan.md` (decisions D1–D5 taken 2026-10-15, independently reviewed). It also ships D5: a broadcast created through the API without `exclude_unsubscribed` now excludes unsubscribed contacts, and bounced or complained addresses are never mailed. Non-members keep receiving by design: opt-out, and an automation leaves them out with a List Status Branch node. Either way, an unsubscribe carrying a valid `email_hmac` must succeed, so the fix records an `unsubscribed` row when none exists.
  - Test first: new `tests/integration/automation_unsubscribe_delivered_test.go`. Set up a custom-event automation with a list, one member and one non-member, and read both emails from Mailpit. POST the delivered `List-Unsubscribe` URL (url-encoded and multipart) and open the in-body link: expect 200 and `unsubscribed`. Fire the trigger again: expect no email for either contact. Unit: a `list_service_test.go` case where the repository returns `ErrContactListNotFound`.

- [x] **B2 — Links whose URL contains an apostrophe are dead (tracking on) or misdirected (tracking off)** — Fixed 2026-10-16 in PR #452.
  - Impact, tracking on: the click answers 400 "Invalid token".
  - Impact, tracking off: the link goes to a different page, `…/blog/what?utm_content=…'s-new`. This hits every workspace, because the queue path always sets `utm_content` (`internal/service/email_service.go:413-416`), so links are always rewritten.
  - The same split breaks personalised links per recipient when Liquid puts contact data containing `'` (O'Brien) into a URL. It also splices the web-analytics `nf_id` mid-URL. Visual-editor links are affected because their `&#39;` is decoded back to `'` first.
  - Cause: `pkg/notifuse_mjml/template_compilation.go:993`. `hrefRegex` captures `[^"']+`, so the URL ends at the first quote of either kind, even inside a double-quoted href. `:940`/`:957` decode `&#39;` into a raw `'` beforehand.
  - Hidden by: `TestTrackLinks` (`pkg/notifuse_mjml/template_compilation_test.go:17`) uses hand-written HTML with substring checks and no quotes in URLs. `tests/integration/link_click_mailpit_e2e_test.go:146-168` extracts hrefs with a regex that silently skips malformed ones, then re-joins the token onto `baseURL` instead of following the href.
  - Verified: Reproduced on a real queued broadcast. `…/r/<token>'s-new` → 400; the control link → 303 to the right page. The tracking-off splice was reproduced on `TrackLinks`.
  - Test first: extend `link_click_mailpit_e2e_test.go` with authored links containing `'`: mj-text, mj-button, a visual-editor link, and `{{ contact.last_name }}` = O'Brien. Parse the delivered hrefs with `golang.org/x/net/html`, GET them unchanged, and assert `Location` equals the authored URL plus UTM. Add `TestTrackLinks` cases built from real gomjml output: `'` inside `"…"` and `"` inside `'…'`.
  - Fix direction: match each quote style separately (`href="([^"]*)"` or `href='([^']*)'`), or rewrite with an HTML tokenizer. Never decode `&quot;`/`&#39;` into a raw quote inside an attribute that quote delimits. Escape when writing back. This also fixes B17.

- [x] **B3 — A goal with a non-string property or a string `value` loses the rest of the session's web analytics** — Fixed 2026-10-16, see PR.
  - Impact: from that goal on, every beat of the tab's session is rejected with 400 and dropped: pageviews, durations and later goals. It is silent, because `trackGoal` doesn't throw. It hits snippet and tag-manager users calling from plain JS, e.g. `properties: {order_id: 12345}` or `value: '49.90'`.
  - Cause: `internal/domain/web_analytics.go:387-388` (`Value float64`, `Properties map[string]string`) fail `json.Decode` (`internal/http/web_analytics_handler.go:194`). That happens before `dropInvalidActions` (`web_analytics.go:610`), whose own comment forbids exactly this outcome.
  - SDK side: it passes `value`/`properties` through unchecked (`web_analytics_sdk/src/sdk.ts:963-979`, `src/core/session-state.ts`). It treats any 4xx as permanent (`src/transport/sender.ts:92`) and re-sends the cumulative `actions[]` on every beat.
  - Hidden by: the Go tests hand-build beats with typed `map[string]string` (`tests/integration/web_analytics_track_test.go:188`, `tests/integration/web_analytics_bridge_test.go:101`). The malformed-action table (`internal/domain/web_analytics_test.go:216-262`) has wrong values, never wrong JSON types. The SDK Playwright suite posts to a fake collector (`web_analytics_sdk/tests/e2e/helpers/collector-server.ts`). The real-SDK tests never run in CI (R-C1).
  - Verified: Reproduced at decode. Numeric property, boolean property and string value all fail; a string property decodes. The audit ran the real `na.js` in Chrome: statuses `[200, 200, 400, 400]`, and the later goal was never stored.
  - Test first: a domain decode test with raw JSON beats of the wrong types, asserting the beat is accepted, the bad goal is dropped or coerced, and the other actions are kept. Plus a `BROWSER_E2E` test calling `trackGoal` with a numeric property, followed by a clean goal.
  - Fix direction, server: decode actions one by one (`[]json.RawMessage`) so one bad entry is dropped, or coerce numbers/booleans in `properties` and numeric strings in `value`.
  - Fix direction, SDK: validate or coerce in `trackGoal`, as `setDimension` already does (`src/core/session.ts:414`). Add a note in `../docs` goals page.

- [x] **B24 — Contacts removed from a list keep receiving its broadcasts** — fixed 2026-10-16 on branch `claude/made-up-tests-audit-high-ciyb05-b24`, see its PR.
  - Impact: the API's "remove from list" (`contactLists.removeContact`) and the automation "Remove from List" node both soft-delete the row, and the broadcast audience ignores that. A removed contact keeps getting every broadcast to the list, even with `exclude_unsubscribed: true`. Segments built on "is in list X with status active" also keep matching removed contacts.
  - Cause: `GetContactsForBroadcast` and `CountContactsForBroadcast` (`internal/repository/contact_postgres.go:1440-1462`, `:1730-1746`) and the segment `contact_list` condition (`internal/service/query_builder.go:1222-1250`) never filter `cl.deleted_at`. Other membership queries in the same repository do (`contact_postgres.go:297-305`, `:428`).
  - Verified: Reproduced. `removeContact` → 200; then a broadcast to the list with `exclude_unsubscribed: true` reported `recipient_count: 2`, and both the active and the removed contact received the real email in Mailpit. Probe: `/private/tmp/claude-501/-Users-pierre-Sites-notifuse3-code-notifuse/dea8a25b-947f-46a9-ad32-e4e84bbe0b2b/scratchpad/probe/src/tests/integration/zz_verify_removed_contact_broadcast_test.go`.
  - Test first: remove a contact through the API, send a broadcast to the list, and read Mailpit: the removed contact gets nothing and the count excludes them. Add a segment case: "is in list X with status active" no longer matches after removal.
  - Fix direction: add `cl.deleted_at IS NULL` to both audience queries and to the segment condition. Segment membership must be recomputed for affected segments. This touches the same two queries as B1's Part 2, so do it right after B1.
  - Plan: `plans/fix-removed-contacts-list-membership-plan.md` (2026-10-15, independently reviewed, implemented 2026-10-16). The scope is wider than above:
    - automation trigger conditions, filter nodes, the API's `contact_lists`, Liquid and the notification center are affected too;
    - a re-add writes no event, so the query fix alone would keep re-added contacts out of their segments;
    - a v42 migration patches the stored SQL. Decided 2026-10-16: a re-add records what a subscribe does (P1), and B24 ships in the current unreleased version, which becomes 42.0 (P2).

- [x] **B25 — A complaint or hard bounce on an automation email is never recorded on the list** — Fixed 2026-10-16, see PR. Hard bounces were already recorded on every list by `MarkEmailsAsBounced` (`internal/service/inbound_webhook_event_service.go`); complaints were the gap. Test: `tests/integration/automation_complaint_list_status_test.go`.
  - Impact: someone who reports an automation email as spam stays `active` on the automation's list. The automation's marketing and blog emails keep reaching them, and so do the list's broadcasts, which damages the sender's reputation. (The audit also listed hard bounces; the tick note above explains why they were already handled.)
  - Cause: the automation queue payload has no `ListID` (`internal/service/automation_node_executor.go:403-413`), so `message_history.list_id` stays NULL (`internal/service/queue/worker.go:584-585`). The trigger that turns complaints and bounces into list statuses acts only when `list_id` is set (`internal/database/init.go:831-861`), and only hard bounces have another path (`MarkEmailsAsBounced`).
  - Verified: Code-confirmed.
  - Test first: an automation email to a list member, then a complaint on that message through a provider webhook payload; expect `contact_lists.status = complained` and the next trigger exiting the contact.
  - Fix direction: set `Payload.ListID` on automation queue entries, the same field broadcasts set.
  - Also fixes a B1 edge case. B1 records a non-member's opt-out only for the list its email was sent for (`MessageHistoryRepository.WasSentForList`). Automation sends store no `list_id`, so that check reads the automation's current list. If someone moves an automation to another list, opt-outs from its earlier emails record nothing. With `list_id` stored, keep the automation join only as a fallback for older messages.
  - Left open, needs a decision — **fixed 2026-10-16 by branch `claude/made-up-tests-audit-high-ciyb05-b25-nonmembers`, inside v42**: a contact with no active row on the automation's list (a non-member, or one removed from it) keeps receiving the automation's emails after a complaint or a hard bounce. The trigger and `MarkEmailsAsBounced` only update existing rows, and the send gate ignores removed rows (`GetContactListByIDs` filters `deleted_at`). Reproduced in review for complaints: a non-member's left no row, a removed contact's marked the removed row `complained`, and in both cases the next trigger delivered a second email. Recording a `complained` or `bounced` row, as D2 does for unsubscribes, would close it; removed rows overlap B24. The fix:
    - The complaint and bounce status (`apply_message_status_to_contact_lists` in `internal/database/schema/message_history_status_trigger.go`, installed by `init.go` and v42) still acts only on the `list_id` stored on the message; emails sent before automations stored theirs record no list. A contact with no row on that list now gets a `complained` or `bounced` row, as D2 does for unsubscribes; a removed row takes the status and stays removed. Nothing is inserted for an address that is no longer a contact or on a deleted list.
    - The email node's gate reads the row with `GetContactListIncludingRemoved`, so a removed row's `unsubscribed`, `bounced` or `complained` status still stops the email.
    - Review follow-ups: `unsubscribeFromList` writes through `MarkContactListUnsubscribed`, one statement that keeps a bounce or complaint and never clears `deleted_at`, so an unsubscribe neither restores a removed contact nor races a complaint (`UpdateContactListStatus` still writes `deleted_at = NULL`, but its only production caller, `contactLists.updateStatus`, answers "contact not in list" for a removed row and writes nothing). The subscribe flow reads with `GetContactListIncludingRemoved`: a removed active or pending row is a new subscription, a removed unsubscribed row goes through double opt-in, a removed bounced or complained row is refused with no confirmation email. `MarkEmailsAsBounced` no longer skips removed rows. The inserts of `apply_message_status_to_contact_lists` use `ON CONFLICT DO UPDATE` under its update's guard, so a row committed concurrently still takes the status.
    - `track_message_history_changes` now sets the status itself, before it writes the message's timeline entries, and a new workspace has no `message_history_status_trigger`. That trigger fired after the timeline one, so a complaint or a hard bounce locked the contact's `contact_segment_queue` row, through the message's timeline entry, before its `contact_lists` row, the reverse of every `contact_lists` write. With an import of the same contact in flight, PostgreSQL aborted one of the two: reproduced in review on an existing row, and the insert for a contact with no row widened it. A first version renamed the trigger so that it fired first, but v42's `DROP TRIGGER` would have held an ACCESS EXCLUSIVE lock on `message_history` for the whole migration, so v42 only replaces function bodies: an upgraded workspace keeps the old trigger, and the body its function has had since v17, which only updates an existing row and finds the status already set. Giving that function the shared one's insert, as a later version did, deadlocked again in review: firing after the timeline entries, it waited for a subscribe of an address that became a contact while the complaint ran, and the subscribe waited for the queue row. `TestMessageStatusRecordsTheListRow`'s subtest "a complaint racing the creation and the subscribe of its address" deadlocks every time on PostgreSQL 17 and 18 when that insert is put back. On an upgraded workspace the old function can still update, after the queue row, a row committed for such an address in between, so a third write of that row in flight can still deadlock with it, as before 42. And when a contact and its row on the list are created in one transaction while a complaint on that address is held on the queue row, a new workspace keeps the row as created, where an upgraded one, like code before 42, records the complaint on it: applying the status again after the timeline entries would bring the deadlock back. The only known way to get there is a send still in flight when its contact is erased, then the contact created again. `list.complained` or `list.bounced` is now written before the `email.*` entry. The conditions of an automation triggered by `email.complained` or `email.bounced` now see the status the complaint or bounce set; in exchange, a timeline condition of an automation triggered by `list.complained` or `list.bounced` no longer counts the `email.*` entry of the same complaint or bounce. The order the timeline shows does not change, since it sorts by `created_at`, which the two entries take from different clocks, and neither does the order of webhook events. An update of several messages in one statement, as SendGrid and SparkPost batches make, can still deadlock with an import of several of their contacts.
    - A write to a row that stays removed records no timeline row and sends no webhook event (`track_contact_list_changes`, `webhook_contact_lists_trigger`): a removed contact is not on the list, and the status their row keeps for the email gate would otherwise reach webhook consumers and list.* automations for a list they left. Decided by Pierre on 2026-10-17 (the alternative was to keep the events and document them).
    - Tests: `tests/integration/automation_complaint_non_member_test.go`. `tests/integration/signed_link_membership_matrix_test.go` expected an unsubscribe to put a removed contact back on the list, and now also starts from removed rows in every status.
  - Found in review 2026-10-17, left out of #453:
    - With the list stored, a hard bounce or complaint on an automation email also reaches a removed contact's row through the trigger, which has no `deleted_at` filter: the row is marked, a `list.complained` or `list.bounced` event is recorded for a list they had left, and a later re-add to the list does nothing, since `AddContactToList` leaves bounced and complained rows alone. B24 (#455) changes none of this; the B25 non-members PR (#456) keeps the status on the removed row, where the email gate reads it, and stops it from recording any list event.
    - Mailjet's `unsub` event is recorded as a complaint (Mailjet parser in `internal/service/inbound_webhook_event_service.go`), so a Mailjet unsubscribe now marks the contact `complained` on the automation's list, as it already did on a broadcast's list. Decided 2026-10-17: a separate PR records it as an unsubscribe. **Fixed 2026-10-18 by branch `claude/made-up-tests-audit-high-ciyb05-mailjet-unsub`**: recorded on the email's list as its unsubscribe link records an opt-out; those recorded as complaints before stay complaints (no data migration). Test: `tests/integration/mailjet_unsubscribe_webhook_test.go`. Same branch, 2026-10-20: the console labels the new `unsubscribe` type in the webhook event log (with its filter) and on the contact timeline, and the Mailjet webhook status reports the `unsub` webhook as an `unsubscribe` endpoint, also for webhooks registered before (no re-registration). Tests: `console/src/components/webhooks/InboundWebhookEventsTab.test.tsx`, `console/src/components/timeline/ContactTimeline.test.tsx`, `internal/service/mailjet_service_register_test.go`, `internal/service/mailjet_webhook_status_test.go`. After review, same day: the email is marked unsubscribed also when it was sent for no list, such as a transactional one (most Mailjet mail), or its list was deleted since, with no list touched; an unsubscribe of another address than the email's recipient, or of an erased contact, still records nothing. An automation email queued before 42.0, stored without its list, is unsubscribed from its automation's current list, read with the SQL of `WasSentForList`. That is not the email's list when the automation was moved to another list since: its own unsubscribe link names its list, and `WasSentForList` only checks that list, it never picks one. Kept anyway: without it, none of these emails would reach their list, which is worse in the usual case of an automation that kept its list. The list their link names can't be read back, since from 20.0 to 41.0 no automation email stored its list: 20.0 sent them directly with template data holding no list, 21.0-41.0 queued them with no template data (`automation_node_executor.go` at `v20.0` and `v41.0`, `queue/worker.go` at `v41.0`). An email whose contact is erased while its unsubscribe is handled is not marked: for unsubscribes only, `SetStatusesIfNotSet` skips a message whose address was redacted, which would otherwise send `email.unsubscribed` with `DELETED_EMAIL`. Complaints and bounces are still recorded on such a message. The guard also changes the email's own link, with no race: an erased address that became a contact again and is back on the list, unsubscribing with the link of an email sent before the erasure, is unsubscribed on its new row, and the old email is no longer marked (it was, with `email.unsubscribed` sent with `DELETED_EMAIL`, a timeline entry and one more unsubscribe in the broadcast's count). Tests: `internal/service/inbound_webhook_event_service_test.go` (`TestProcessWebhook_MailjetUnsubscribe`), `internal/repository/message_history_postgre_test.go` (`TestMessageHistoryRepository_GetRecipientAndList`, `TestMessageHistoryRepository_SetStatusesIfNotSet`), `tests/integration/mailjet_unsubscribe_webhook_test.go` (also an automation moved to another list or deleted since its email, and the link of an email sent before its contact was erased).
    - Pre-existing, already open as B23: deleting a list an automation uses makes its email node fail for every contact that reaches it, waiting or newly enrolled. Fixed 2026-10-18 on branch `claude/made-up-tests-audit-high-ciyb05-list-delete-guard` (see B23): the deletion is refused while a live automation uses the list, and activation refuses a deleted list.

## Medium

- [ ] **B4 — Open redirect on every Notifuse host, including customers' branded tracking domains**
  - Impact: `GET /visit?url=https://anything` without `mid`/`wid` answers 303 to that URL. Spammers can send phishing through `track.customer.com`, which can get the customer's tracking domain blocklisted (DBL/URIBL) and hurt the deliverability of all their emails. `/r/` tokens are forgeable too: the key is a constant in the public source.
  - Cause: `internal/http/email_handler.go:163-166` redirects unconditionally when `mid` or `wid` is missing. `pkg/crypto/crypto.go:20` `trackingObfuscationKey` is hard-coded, and its comment says it is "NOT for security".
  - Hidden by: `tests/integration/email_handler_test.go:89` ("redirect without tracking parameters") asserts the open redirect as expected behaviour.
  - Verified: Code-confirmed. The redirect is unconditional.
  - Decision: links in emails already delivered must keep working. Options: redirect only to URLs found in that message's rendered links, sign the target with the workspace secret in new links and accept unsigned legacy links only with `mid`/`wid` that resolve to a real message, or show an interstitial for unverifiable targets.
  - Test first: `/visit` without IDs and a forged `/r/` token must not redirect to an arbitrary host. Real `/r/` links taken from a delivered email still redirect.

- [ ] **B5 — Someone invited with capitals can't sign in with lowercase**
  - Impact: the account keeps the owner's spelling (`Jane.Doe@Example.com`). Signing in as `jane.doe@example.com` answers "user does not exist". The first session lasts 30 days, so this surfaces a month later.
  - Cause: `AcceptInvitation` matches users case-insensitively but creates the user with the invitation's exact casing (`internal/service/workspace_service.go:1597-1616`). `GetUserByEmail` is exact on purpose (`internal/repository/user_postgres.go:72-82`). The console sends the email as typed (`console/src/pages/SignInPage.tsx:219`).
  - Hidden by: invitation tests take the token from the development-only `inviteMember` response, use generated lowercase emails, and never sign the invitee in again.
  - Verified: Code-confirmed: nothing on the path normalises. The audit reproduced it with the token from the delivered email.
  - Decision: lowercase invited addresses at invite time (the exact-match comment is about root sign-in), and decide what to do with existing mixed-case users.
  - Test first, with the real mailer (R-H3): invite `Mixed.Case+tag@Example.COM`, accept from the delivered link, sign in lowercase, and verify with the code read from the delivered email.

- [ ] **B6 — A failed invitation email is reported as sent**
  - Impact: with SMTP down or rejecting, the API answers 200 "Invitation sent" and the console shows "Invitation sent to X" or "Invitation resent". Nothing is delivered, and in production the token isn't returned, so the owner can't share the link by hand.
  - Cause: `internal/service/workspace_service.go:952-960` logs the error and continues.
  - Hidden by: the harness installs `ConsoleMailer` (`Environment: "development"`), which never fails.
  - Verified: Code-confirmed. The audit reproduced it with SMTP down.
  - Test first, with the real mailer pointed at a closed port: `inviteMember` and resend must report the failure.
  - Fix direction: return the failure, or `email_sent: false` plus the link for the owner to copy, and show it in the console.

- [ ] **B7 — System emails put names into HTML unescaped**
  - Impact, invitations: on multi-tenant installs a workspace owner can rename the workspace to `<a href=//ev.il>Verify</a>` (it fits the length limit). That link then appears in invitations sent from the instance's own sender to any address.
  - Impact, circuit-breaker alert: it loses `<sender@domain>` from the SMTP error, which is the address the owner must fix, and mangles broadcast names containing `<`, `&` or `"`.
  - Cause: `pkg/mailer/mailer.go:85-103` (invitation) and `:255-262` (alert) interpolate with `fmt.Sprintf`. The file contains no escaping call.
  - Hidden by: the mailer unit tests use a nil-client mailer and only check log lines. No integration test ever receives a system email.
  - Verified: Code-confirmed. The audit saw both in delivered emails.
  - Test first: in `pkg/mailer/mailer_test.go`, build the real message (not the nil client) and parse its HTML. Integration: hostile workspace and broadcast names produce escaped text in the delivered HTML, with no injected element.
  - Fix direction: `html.EscapeString` every interpolated value in HTML bodies, or move to `html/template`.

- [ ] **B8 — Transactional and automation messages are recorded as template version 0**
  - Impact, timeline: it shows the raw template ID, a stray "0" and no preview button.
  - Impact, Messages: it shows "v0" and previews version 0, which means "latest", so it shows today's template rather than the one sent. Automation rows also lack data and list.
  - Cause: `internal/service/email_service.go:606-619` builds the history row without `TemplateVersion`. The automation queue entry (`internal/service/automation_node_executor.go` ~390) omits the version, data and list that broadcasts set (`internal/service/broadcast/queue_message_sender.go:413`).
  - Display side: the timeline joins templates on version (`internal/repository/contact_timeline_postgre.go:132`), and `console/src/components/timeline/ContactTimeline.tsx:599` renders `{template_version && …}`, which prints `0`.
  - Hidden by: `internal/service/queue/worker_test.go:1130,1153` hand-build `TemplateVersion: 1`, a shape only broadcasts produce. The console tests use `template_version: 1` (`MessageHistoryTable.test.tsx:29`, `MessageHistoryTab.test.tsx:17`), and `ContactTimeline.test.tsx` has no email entry.
  - Verified: Code-confirmed. The audit fed the real `timeline.list` entry to the real component.
  - Test first: integration, `transactional.send` and an automation email, then read `message_history` and `timeline.list`: the version equals the template's and `template_name` is set. A console test fed that real JSON, plus a legacy version-0 row that renders without a stray "0".
  - Fix direction: record the version actually rendered on every path. Existing rows stay 0, so the console must show 0 as an unknown version.

- [ ] **B9 — A per-send `reply_to` is ignored when the template has one**
  - Impact: `transactional.send` with `reply_to` delivers the template's Reply-To, while the console's test send delivers the caller's. The console's sent-message view shows the caller's value, which never left the server. The docs present `reply_to` as per-send (`../docs/features/transactional-api.mdx:127`).
  - Cause: `internal/service/email_service.go:645-647` lets the template overwrite the caller's value, and `:603` captures channel options before that. `internal/service/transactional_service.go:985-986` (test send) does the opposite.
  - Hidden by: `internal/service/email_service_test.go:1102,1260` compute the expected value from `emailTemplate.Email.ReplyTo`. No test sets both, and no integration test sends `reply_to`.
  - Verified: Code-confirmed. The audit read the delivered headers.
  - Test first: integration `transactional.send` with both set: the delivered `Reply-To` is the caller's, and `message_history.channel_options` matches what was delivered.
  - Fix direction: the caller's value wins and the template's is the default. Store channel options after resolution.

- [ ] **B10 — Translations only match the exact language code**
  - Impact: a contact stored as `fr-FR` receives the default-language email. The notification center saves the primary subtag (`pt`), so a `pt-BR` translation never matches.
  - Cause: an exact map lookup, `t.Translations[contactLanguage]` (`internal/domain/template.go:211`, and `:260` for web). That file has uncommitted edits from another session, so recheck the line numbers. The contact language is stored verbatim (`internal/domain/contact.go:35`). The widget saves `navigator.language.split('-')[0]` (`notification_center/src/App.tsx:233`). A fallback already exists in `webSupportedLanguage` (`internal/service/web_analytics_service.go:351`): full tag, then primary subtag.
  - Hidden by: `tests/integration/template_translation_test.go:133` uses exactly `"fr"`, and the widget tests run under jsdom's `en-US`.
  - Verified: the lookup and widget code are confirmed. The delivered results are Audit-only.
  - Test first: integration, a template with `fr` and `pt-BR` translations; contacts `fr-FR` and `pt` must receive French and Portuguese.
  - Fix direction: one shared resolver (exact tag, then primary subtag, then a translation sharing the primary subtag, then the default), used by email and web rendering.

## Low

- [ ] **B11 — The expiry text contradicts the code.** The sign-in email says 10 minutes (`pkg/mailer/translations.go:113` and locales); the code uses 15 (`internal/service/user_service.go:135`). The invitation says 7 days (`translations.go:125` and locales); the code uses 15 (`internal/service/workspace_service.go:920`). Code-confirmed. Fix: derive the text from the same constants. Test: a real-message mailer test asserting the delivered text matches the constant.
- [ ] **B12 — System emails put text/plain last.** `pkg/mailer/mailer.go:118-119`, `:194-195` and `:272-273` set the HTML body, then add text/plain as the alternative. RFC 2046 §5.1.4 makes the last part the preferred one. Code-confirmed. Fix: plain first, HTML last. Test: assert the delivered MIME structure.
- [ ] **B13 — The notification center only loads when the browser sends a Referer.** Its bundle is served from `/assets/*`, which reaches the widget only through the `Referer contains /notification-center` rule (`internal/http/root_handler.go:103`). Without a Referer the asset 307s to `/console` and the page stays on its spinner, e.g. under `Referrer-Policy: no-referrer` or with privacy extensions. Reproduced. Fix: build the widget with Vite `base: '/notification-center/'` and drop the Referer rule. Test: fetch the delivered page's assets without a Referer.
- [ ] **B14 — HEAD on notification-center pages answers 405** (`internal/http/root_handler.go:224`). The #450 HEAD subtest asserts the 303 but never follows it. Code-confirmed. Fix: accept HEAD. Test: follow the redirect with HEAD.
- [ ] **B15 — Test sends don't send what real sends do.** `transactional.testTemplate` injects a fake list (`internal/service/transactional_service.go:904-905`, `ListID: "foo"`, `ListName: "bar"`), so the test email shows "bar" and unsubscribe/confirm links that answer 500. `broadcasts.sendToIndividual` sets `ListName: ""` (`internal/service/broadcast_service.go:1259`) and skips the default `utm_content` the queue adds (`email_service.go:413-416`). Three integration files use it as a stand-in for real broadcasts. Code-confirmed. Fix: use real list data and share the queue path's link settings.
- [ ] **B16 — The Message-ID comes from the host name.** SMTP sends set only `X-Message-ID` (`internal/service/smtp_service.go:614`), so go-mail generates `Message-ID` from `os.Hostname()`. Under Docker that's the container ID: not an FQDN (spam filters penalise it, e.g. Rspamd `MID_RHS_NOT_FQDN`), and it leaks the host name. Audit-only; check the system mailer too. Fix: set `Message-ID: <messageID@sender-domain>`.
- [x] **B17 — A `"` in a visual-editor href silently truncates the URL.** Fixed 2026-10-16 in PR #452 (with B2). `template_compilation.go:956` decodes `&quot;` into a raw `"` inside a double-quoted attribute, so `…/s?q="exact"` is delivered as `…/s?q=`. Audit-only. Fixed by the B2 rewrite; add the case to B2's tests.
- [ ] **B18 — A code-mode href with surrounding whitespace is missing from per-link stats.** The redirect works, but `sanitizeClickedURL` (`internal/service/email_service.go:261`) fails to parse it, so the click never reaches per-link stats. Audit-only. Fix: trim before parsing, or at rewrite time.
- [ ] **B19 — The blog `base_url` keeps a trailing slash.** A Custom Endpoint or Website URL saved as `https://blog.example.com/` makes every theme's canonical, `og:url` and nav link `https://blog.example.com//slug`, which is answered with 307 (`internal/domain/blog.go:1113-1119`). Reproduced. Fix: `strings.TrimRight(…, "/")`, as `ResolveEndpoint` and `BuildWorkspaceTemplateVars` do. Test: `internal/domain/blog_test.go` cases with trailing slashes, and a rendered preset theme's canonical.
- [ ] **B20 — Tracking endpoints accept any method.** A HEAD from a scanner or link checker on `/r/` records a click (`internal/http/email_handler.go:268-366` has no method check). Code-confirmed. Fix: record clicks and opens on GET only.
- [ ] **B21 — Sign-in leaks internal errors.** When the magic-code email fails, `user.signin` returns `err.Error()` in its 500 (`internal/http/user_handler.go:122`), e.g. `dial tcp …: connection refused`, to anonymous callers. Code-confirmed. Fix: return a generic message and log the detail.
- [ ] **B22 — Message and timeline lookups are case-sensitive.** Contacts are lowercased on save, but `messages.list?contact_email=` and `timeline.list?email=` match the filter as given, so `Mixed.Case@Example.com` finds nothing. Audit-only. Fix: lowercase the filter.
- [x] **B23 — Deleting a list an automation uses breaks the automation.** **Fixed 2026-10-18 on branch `claude/made-up-tests-audit-high-ciyb05-list-delete-guard`: the deletion is refused while a live automation uses the list, and activation refuses a deleted list. Test: `tests/integration/list_delete_automation_guard_test.go`.** `ListService.DeleteList` has no in-use check, and `GetListByID` hides deleted lists, so the automation's email node then fails on every run with "failed to get list" (`internal/service/automation_node_executor.go:302-305`). Code-confirmed. Fix: refuse the deletion while an automation references the list (or detach it with a warning). Test: delete an automation's list through the API, then fire its trigger.
- [x] **B26 — `TestBroadcastPartialFailure` is flaky (test race, found 2026-10-15 while verifying B1).** **Fixed 2026-10-16 in 44880ee7, with B1.** The fix covers all four reads in the file that came right after a queue settle; the helper `broadcastStatsOnceRecorded` waits up to 10s for the expected counts. Proof: with a 500ms sleep added between `MarkAsSent` and the history write in a sandbox worker, the old test failed every time and the fixed one passed. It had failed again in the second full run before the fix. On success the queue worker deletes the queue row (`MarkAsSent`, `internal/service/queue/worker.go:460`) before it writes message history (`:468`). The test waits for "SMTP accepted 12 and the queue is empty", then reads `GetBroadcastStats` once (`tests/integration/broadcast_partial_failure_test.go:291-299`), so it can see 11 sent and 1 failed. Seen once in a 20-test run on HEAD plus the B1 fix; 3 of 3 passes with and without the fix on re-run. Fix: put the stats check inside the `Eventually`. The ordering also means a crash between the two writes leaves a delivered message recorded as failed.
- [ ] **B27 — A "pending" contact is still emailed when the list has no double opt-in (found 2026-10-15 while implementing D6).** Re-subscribing an `unsubscribed` contact forces double opt-in, so the row becomes `pending` (`internal/service/list_service.go`, `forceDoubleOptin`). Nothing waits for the confirmation, though:
  - The broadcast audience queries drop `pending` only when the list itself has double opt-in (`l.is_double_optin = false OR cl.status <> 'pending'` in `internal/repository/contact_postgres.go`).
  - The automation send gate never skips `pending` (`internal/service/automation_node_executor.go`, `case unsubscribed, bounced, complained`).
  - The confirmation email goes out only when the list has a double opt-in template, so on a plain list the contact can stay `pending` for good while still being emailed.
  - This undercuts the "forced DOI for compliance" on every public list without double opt-in. D6 ("leave only", 2026-10-16) keeps private lists out of it, since a contact can no longer rejoin one.
  - The confirmation can't really be required anyway: the confirmation link and the page's Subscribe button send the same signed `/subscribe` request, so a second click confirms.
  - Reproduced 2026-10-16 in a real (headless) Chrome against the live server, on a public list without double opt-in. Unsubscribing, then clicking Subscribe, left the row `pending` and sent no email, yet the page said "subscribed". A later click on Subscribe made the row active without any email.
  - Fix direction: never mail `pending` rows (both audience queries and the gate). Send the confirmation with a default template when the list has none, and give the confirmation link its own token.
- [ ] **B28 — `TestSettingsHandler_TestSMTP_ConnectionFails` fails intermittently in full unit runs, cause not captured (found 2026-10-16 while verifying B1).** The unit test (`internal/http/settings_handler_test.go:729`) dials `invalid-host.example.com:587` for real and expects 400.
  - **Failures:** 3 of 9 full multi-package runs with B1 applied, each after about 0.65s; the one full run without B1 passed.
  - **Passes:** 35 of 35 runs of the test or the HTTP package alone, including under load (0.00s). The last 4 full runs passed in a row, so the failure output was never captured; earlier runs had filtered it away.
  - **DNS ruled out:** the name gets no answer, so the dial fails fast and the handler's only error path answers 400. The DNS theory doesn't explain a failure, so something else in a full run is involved, a `-race` report for example.
  - **Not B1:** its path (settings handler, setup service, SMTP dial) has no B1 change.
  - **Next:** loop full runs, keeping the whole output, until it fails, then fix the cause. Also stop the test resolving a public name (inject a failing dialer).
- [ ] **B29 — An erased contact can be re-created through a subscribe link (found 2026-10-16, adversarial review of B1).** For a request signed with `email_hmac`, `SubscribeToListsWithResults` calls `UpsertContact` before it checks the list (`internal/service/list_service.go`, the `canUpsert` block). An old double opt-in confirmation link (widget `action=confirm`) therefore stores the erased address again, even when the request then fails with "list is not public". Code-confirmed, not reproduced. Fix: don't upsert for a signed request whose contact no longer exists, and test it with a confirmation link taken from a real email sent before the erasure.
- [ ] **B30 — Minor items from the same review.** All code-confirmed and low impact; none is caused by B1.
  - **`mid` isn't tied to the address.** When the contact has a row on the list, an unsubscribe marks whatever `mid` the request carries, firing that message's `email.unsubscribed` webhook. Message IDs are UUIDs, so this needs one the person was sent or was forwarded.
  - **JSON key casing.** `encoding/json` matches keys case-insensitively, but the presence check in `CreateBroadcastRequest`/`UpdateBroadcastRequest.UnmarshalJSON` is exact. `"EXCLUDE_UNSUBSCRIBED": false` decodes as false, is then treated as omitted, and becomes true. This errs on the safe side.
  - **Private pending rows.** One shows a disabled button with the raw, untranslated status "pending", so the contact can't leave it from the page.
  - **Race on a private list.** A double opt-in confirmation that races a removal can bring the row back: the upsert clears `deleted_at`.
  - **Partial writes.** A multi-list `/subscribe` writes the earlier lists before refusing a later private one.
  - **Removed complained rows** — fixed by branch `claude/made-up-tests-audit-high-ciyb05-b25-nonmembers` (`MarkContactListUnsubscribed`). One that receives an unsubscribe comes back as `unsubscribed`. Live rows keep the complaint since B1, but the lookup B1 uses ignores removed rows.
  - **Complaint race** — fixed by the same branch, with the guarded UPDATE below. B1's guard reads the row, then updates it in a separate statement, so a complaint saved between the two is still overwritten; a later complaint wins anyway. Fix all three overwrite cases with one guarded UPDATE (`AND status NOT IN ('bounced','complained')`) in a dedicated repository method; `UpdateContactListStatus` is shared with the admin API, which must keep overriding.
  - **Deleted list on a confirmation link.** `/subscribe` on a deleted list answers 500 even with a valid signature: `handleSubscribe` maps only "list is not public" to 400, so `ErrListNotFound` falls through. This bites a double opt-in confirmation clicked after the list was deleted. Found by Gemini.
  - **A removed row's bounce or complaint can't be cleared.** `contactLists.updateStatus` refuses a removed row, `AddContactToList` and the bulk add keep a bounced or complained status, and the subscribe flow refuses it. Since `MarkEmailsAsBounced` reaches removed rows, a false-positive hard bounce leaves every list the contact was removed from bounced for good, so a later add-to-list node or import into one of them silently leaves them removed and bounced. Decide whether `updateStatus` may act on a removed row; reading through `GetContactListIncludingRemoved` alone would also re-add the contact, since the repository write clears `deleted_at`.
  - **Unsubscribe racing a subscribe and a complaint.** When a row is committed between `unsubscribeFromList`'s first update and its read, and a complaint lands after the read but before the second update, the complaint is kept but the request reports nothing recorded: the message gets no `unsubscribed_at`, so no `email.unsubscribed` webhook and no `email.unsubscribed` timeline row, which automations triggered by it would start from. Statistics, that webhook and those automations only.
  - **A non-member's opt-out can restore a removal.** The insert for a contact with no row goes through `AddContactToList`, whose conflict update clears `deleted_at`, so a row added and then removed between the read and the insert comes back as a live `unsubscribed` row.
  - **Segments see non-members' statuses.** A `complained` or `bounced` row recorded for a non-member, like an `unsubscribed` one, matches the list's "in list" conditions that filter on no status, so the contact joins those segments and can enter automations triggered by them. Documented in the 42.0 changelog; decide whether "in list" should mean a mailable status.
  - **Repeated soft bounces skip non-members.** The soft-bounce threshold marks the address through `MarkEmailsAsBounced`, which only updates rows that exist, and sets no `bounced_at` on the message, so a non-member with no row keeps receiving the automation's emails.
  - **Erased contact's page.** When an erased contact opens the in-body unsubscribe link, the POST answers 200, but the page then loads `/preferences`, gets 404, and replaces the success message with an error.

## Decisions (not bugs)

- [ ] **D1 — Double opt-in confirms on page load.** `action=confirm` POSTs `/subscribe` as soon as the page loads, like `action=unsubscribe` does by design. A JS-executing scanner opening the confirmation email therefore confirms the subscription, which is what double opt-in exists to prevent. Decide whether confirmation needs a click.

Not bugs: `user.signin` returning the magic code whenever `ENVIRONMENT` is not `production` is by design (demo and dev).

### Decided 2026-10-15 (after a review of competitors' 2026 docs; sources in the B1 plan)

- [ ] **N1 — Docs: automation emails reach non-members.** Automations stay opt-out, so `../docs/features/automations.mdx` should say that contacts who aren't on the automation's list receive its emails, and that a List Status Branch's "not in list" path leaves them out.
- [ ] **N2 — Docs: deleting a contact doesn't stop a re-import.** Erasure stays a pure delete, so `../docs` must say that a later import or API upsert re-subscribes the address.
- Not adopting:
  - a per-list opt-in mode for automations (node filters cover it);
  - a global "unsubscribe from all" (opt-outs stay per list);
  - delete-and-suppress on erasure.

## Unverified leads (from the first audit wave, stopped before reporting)

These were probed but never reported or checked. Run each probe before treating it as a bug.

- [ ] **L1 — Blog feed `If-Modified-Since` may never match.** `updated_at` has microsecond precision but `Last-Modified` has second precision, so an echoed header may never yield 304. Probe: `/private/tmp/claude-501/-Users-pierre-Sites-notifuse3-code-notifuse/dea8a25b-947f-46a9-ad32-e4e84bbe0b2b/scratchpad/public/src/internal/http/zz_probe_feed_ims_test.go`.
- [ ] **L2 — `identify()` signatures in uppercase hex may be rejected.** Probe: `/private/tmp/claude-501/-Users-pierre-Sites-notifuse3-code-notifuse/dea8a25b-947f-46a9-ad32-e4e84bbe0b2b/scratchpad/identity/src/internal/domain/zz_probe_identify_hex_case_test.go`.
- [ ] **L3 — Outgoing webhook signatures, checked against the official Standard Webhooks library.** Probe: `/private/tmp/claude-501/-Users-pierre-Sites-notifuse3-code-notifuse/dea8a25b-947f-46a9-ad32-e4e84bbe0b2b/scratchpad/identity/src/internal/service/zz_probe_stdwebhooks_test.go`.
- [ ] **L4 — Contact request decoders: whitespace handling in decoded fields.** Probe: `/private/tmp/claude-501/-Users-pierre-Sites-notifuse3-code-notifuse/dea8a25b-947f-46a9-ad32-e4e84bbe0b2b/scratchpad/api/src/internal/domain/zz_probe_contact_decoders_test.go`.

## Recommendations — testing system and process

The rule: a test must use the artifact the code really produced, the way its real consumer uses it. Anything else is made up, and can only agree with the code.

### Harness and infrastructure

- [ ] **R-H1 — Absolute links by default.** The workspace factory sets the Custom Endpoint URL to the test server, so every delivered link is usable as is. Today the harness's `APIEndpoint: ""` makes links relative, which forced tests to rebuild them.
- [ ] **R-H2 — One delivered-email helper in `tests/testutil`.** It waits for the message and exposes:
  - hrefs parsed with `golang.org/x/net/html` and entity-decoded as a mail client does;
  - headers and MIME parts;
  - `Click`/`Open`, which request a link unchanged with a browser User-Agent; redirects are asserted, not silently followed.

  Migrate the existing Mailpit tests to it and retire the ad-hoc href regexes.
- [ ] **R-H3 — The real system mailer in the harness.** Add an option that installs `SMTPMailer` pointed at Mailpit, so invitation and sign-in tests read the actual email. Keep `Login()`'s side channel for tests that only need a session.
- [ ] **R-H4 — Configurable test infrastructure.** Read the Mailpit API URL and SMTP port from env, as `TEST_DB_PORT` already is, and query Mailpit by recipient instead of clearing it globally. Several stacks can then run in parallel (the audit had to rewrite hard-coded ports in copies), and tests stop deleting each other's mail.
- [ ] **R-H5 — Production path by default.** Start the background workers and send broadcasts through the queue. Keep `sendToIndividual` for its own tests, since it differs (B15).
- [ ] **R-H6 — Realistic fixtures.** Contacts with plus-addressing, mixed case, apostrophes, accents/IDN, `& < > "` in names and a regional language (`fr-FR`, `pt-BR`). Links with `'`, `#fragment`, an existing query, percent-encoding and Liquid. Use them in every send-path test, not only `contact-<uuid>@example.com`.

### Writing tests

- [ ] **R-W1 — No server helper as input builder or oracle in integration tests.** That means no `ComputeEmailHMAC`, `EncryptTrackingToken` or `BuildTemplateData`, and no delivered path re-joined onto `GetURL()`. Inputs come from delivered emails or the public API. Rewrite:
  - `unsubscribe_api_test.go`
  - `preferences_api_test.go`
  - `subscribe_status_test.go` (double opt-in)
  - `link_click_stats_test.go`
  - the tracking parts of `email_handler_test.go`
  - `link_click_mailpit_e2e_test.go`
  - `web_analytics_email_identify_test.go`
  - the link checks in `broadcast_liquid_template_test.go`
- [ ] **R-W2 — Exercise every real consumer of an artifact:**
  - mail clients: GET/HEAD the header URL, and the one-click POST in both RFC 8058 encodings;
  - people: open the link in a browser, follow the redirects, load the page's assets with and without a Referer;
  - scanners: GET/HEAD every link and run the page's JS without interaction;
  - API clients: raw JSON as the docs describe.
- [ ] **R-W3 — Assert outcomes, not presence.** Check the DB state after the click, that `Location` equals the authored URL, and that the header is usable as delivered. Retire `Contains(html, "/t/")` and "href is not empty" style assertions.
- [ ] **R-W4 — Derive contract cases from the contract.** Tests of RFC-, provider- or browser-facing behaviour cite their source (RFC section, provider doc). A test asserting a rejection (405, 400) on an externally used endpoint must justify it against that source. The pre-#450 `rejects non-POST methods` and `email_handler_test.go:89` both pinned a bug as expected behaviour.
- [ ] **R-W5 — Wire-shaped inputs for every decoder.** Use raw JSON with wrong types, missing fields and extra fields. This extends the #401 raw-JSON rule to beats, webhooks and public endpoints (B3).
- [ ] **R-W6 — Producer-to-consumer golden fixtures.** The producer's test writes its real output to a checked-in fixture; the consumer's test reads it. Pairs:
  - delivered notification-center URLs (Go) → the widget parser test;
  - SDK beats → the Go decode test;
  - Go handler JSON, empty and full → console mocks.

  Drift then fails CI on one side.
- [ ] **R-W7 — Mocks built from real responses.** When a console or widget test mocks the API, derive the mock from a recorded real response, including `null` for empty Go slices, not from the TypeScript type.

### CI

- [ ] **R-C1 — Run the real-browser tests.** Add a CI job with Chrome and `BROWSER_E2E=true` for `TestWebAnalyticsBrowserEndToEnd` and `TestWebAnalyticsBrowserIdentify`, which never run today, and add a `trackGoal` with properties to them (B3).
- [ ] **R-C2 — Guard the bans.** A CI grep that fails when `tests/integration` calls `ComputeEmailHMAC(` or `EncryptTrackingToken(`, or concatenates `GetURL()` with a path taken from an email. Allowlist tests that call fixed API routes.
- [ ] **R-C3 — Fail on skipped gated suites.** Report skipped integration tests, and fail a job whose gated suite (e.g. `BROWSER_E2E`) skipped where it was meant to run.

### Process

- [ ] **R-P1 — Bug fixes start with a red, as-delivered test.** Reproduce with the real artifact before touching the code, then fix. Add this rule to the Testing section of `CLAUDE.md`.
- [ ] **R-P2 — A review question for anything that leaves Notifuse.** For every PR that emits an email link, header, webhook, snippet or public response, ask: "which test consumes the real output, the way its real consumer does?"
- [ ] **R-P3 — A new public artifact ships with its contract test.** Any new link, header or endpoint used outside Notifuse comes with a delivered-artifact test and the list of its real consumers.
- [ ] **R-P4 — Mutation testing comes after contract cases.** It hardens whatever the tests assert, including a wrong expectation: it would have defended the 405. Use it once the contract-derived cases exist, not instead of them.

## Running the tests

For each item, run the layers it touches:

- Go layers: `make test-domain`, `make test-service`, `make test-repo`, `make test-http` (or `make test-unit`)
- Packages under `pkg/`: `go test -race ./pkg/notifuse_mjml/ ./pkg/mailer/ ./pkg/crypto/`
- Integration, only the touched tests: start `docker compose -f tests/compose.test.yaml up -d`, then `INTEGRATION_TESTS=true go test -race -tags integration,licdev -run 'TestName' ./tests/integration/ -v`. Without `INTEGRATION_TESTS=true` every test skips and still prints `ok`.
- Real-browser tests: `BROWSER_E2E=true` (needs Chrome)
- Console: `cd console && npm test`
- Notification center: `cd notification_center && npx vitest run`
- Web analytics SDK: `cd web_analytics_sdk && npx vitest run`

## Audit artifacts

These live in the session scratchpad under `/private/tmp`, which may be cleared on reboot. Copy anything worth keeping.

- Isolated stack script (own Postgres + Mailpit per slot, ports rewritten in a HEAD copy): `/private/tmp/claude-501/-Users-pierre-Sites-notifuse3-code-notifuse/dea8a25b-947f-46a9-ad32-e4e84bbe0b2b/scratchpad/isolated_stack.sh`
- B1 probe: `/private/tmp/claude-501/-Users-pierre-Sites-notifuse3-code-notifuse/dea8a25b-947f-46a9-ad32-e4e84bbe0b2b/scratchpad/nclinks/src/tests/integration/zz_audit_nclinks_test.go`, and the widget parser run on delivered URLs, `/private/tmp/claude-501/-Users-pierre-Sites-notifuse3-code-notifuse/dea8a25b-947f-46a9-ad32-e4e84bbe0b2b/scratchpad/nclinks/src/notification_center/src/zz_audit_parse.test.ts`
- B2 probes: `/private/tmp/claude-501/-Users-pierre-Sites-notifuse3-code-notifuse/dea8a25b-947f-46a9-ad32-e4e84bbe0b2b/scratchpad/probe/src/tests/integration/zz_verify_apostrophe_test.go`, `/private/tmp/claude-501/-Users-pierre-Sites-notifuse3-code-notifuse/dea8a25b-947f-46a9-ad32-e4e84bbe0b2b/scratchpad/probe/src/pkg/notifuse_mjml/zz_verify_trackoff_test.go`, `/private/tmp/claude-501/-Users-pierre-Sites-notifuse3-code-notifuse/dea8a25b-947f-46a9-ad32-e4e84bbe0b2b/scratchpad/tracking/src/tests/integration/zz_audit_tracking_test.go`, `/private/tmp/claude-501/-Users-pierre-Sites-notifuse3-code-notifuse/dea8a25b-947f-46a9-ad32-e4e84bbe0b2b/scratchpad/tracking/src/tests/integration/zz_audit_tracking2_test.go`
- B3 probes: `/private/tmp/claude-501/-Users-pierre-Sites-notifuse3-code-notifuse/dea8a25b-947f-46a9-ad32-e4e84bbe0b2b/scratchpad/probe/src/internal/domain/zz_verify_goal_types_test.go`, real Chrome: `/private/tmp/claude-501/-Users-pierre-Sites-notifuse3-code-notifuse/dea8a25b-947f-46a9-ad32-e4e84bbe0b2b/scratchpad/pairs/src/tests/integration/zz_probe_goal_props_test.go`
- System emails (B5-B7, B11, B12, B21), with a production-mode harness: `/private/tmp/claude-501/-Users-pierre-Sites-notifuse3-code-notifuse/dea8a25b-947f-46a9-ad32-e4e84bbe0b2b/scratchpad/sysmail/src/tests/integration/zz_probe_sysmail_test.go`, `/private/tmp/claude-501/-Users-pierre-Sites-notifuse3-code-notifuse/dea8a25b-947f-46a9-ad32-e4e84bbe0b2b/scratchpad/sysmail/src/tests/testutil/zz_probe_env.go`
- Delivered content (B8-B10, B22): `/private/tmp/claude-501/-Users-pierre-Sites-notifuse3-code-notifuse/dea8a25b-947f-46a9-ad32-e4e84bbe0b2b/scratchpad/content/src/tests/integration/zz_probe_delivered_content_test.go`, `/private/tmp/claude-501/-Users-pierre-Sites-notifuse3-code-notifuse/dea8a25b-947f-46a9-ad32-e4e84bbe0b2b/scratchpad/content/src/tests/integration/zz_probe_replyto_test.go`, `/private/tmp/claude-501/-Users-pierre-Sites-notifuse3-code-notifuse/dea8a25b-947f-46a9-ad32-e4e84bbe0b2b/scratchpad/content/src/tests/integration/zz_probe_case_test.go`, `/private/tmp/claude-501/-Users-pierre-Sites-notifuse3-code-notifuse/dea8a25b-947f-46a9-ad32-e4e84bbe0b2b/scratchpad/content/src/console/src/components/timeline/zz_probe_message_entry.test.tsx`
- B19 probe: `/private/tmp/claude-501/-Users-pierre-Sites-notifuse3-code-notifuse/dea8a25b-947f-46a9-ad32-e4e84bbe0b2b/scratchpad/src/internal/domain/zz_probe_blog_base_url_test.go`
