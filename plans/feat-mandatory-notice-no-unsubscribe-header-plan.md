# Plan — no List-Unsubscribe header on broadcasts that include unsubscribed contacts (issue #484)

## Need

Issue #484: security, outage and legal notices must reach every member of a list, including
those who unsubscribed, and such emails should not carry the one-click unsubscribe header.

Sending to the list's unsubscribed members already exists: a broadcast whose
`audience.exclude_unsubscribed` is `false` mails them (`withBroadcastListFilter`,
`internal/repository/contact_postgres.go`); bounced and complained contacts stay excluded.
What is missing is the header: every broadcast email sent to a list carries
`List-Unsubscribe` and `List-Unsubscribe-Post`.

## Decision (taken by Pierre)

No new field. A broadcast that keeps the list's unsubscribed contacts
(`exclude_unsubscribed = false`) is by definition a notice every member must receive, so its
emails carry neither `List-Unsubscribe` nor `List-Unsubscribe-Post`. A broadcast that excludes
them (the default) keeps both headers, unchanged.

No SQL schema change, no migration, no environment variable.

Out of scope, unchanged: the `unsubscribe_url`, `oneclick_unsubscribe_url` and
`notification_center_url` Liquid variables (the template's footer link is the template's
choice), automation emails, transactional emails, and the one-click endpoint itself.

Consequence to state in the CHANGELOG: draft, scheduled or paused broadcasts that already have
"Exclude unsubscribed recipients" switched off will go out without the header.

## Where the header is decided

Every provider adds both headers only when `EmailOptions.ListUnsubscribeURL` is non-empty
(SMTP, SES, SendGrid, Mailgun x2, Mailjet, Postmark, SparkPost). The URL is copied from
template data into `EmailOptions.ListUnsubscribeURL` in exactly three broadcast places:

1. `internal/service/broadcast/queue_message_sender.go` `buildQueueEntry` (the queue path used
   in production; the URL is stored in the queued payload and sent as is by the worker).
2. `internal/service/broadcast/message_sender.go` `SendToRecipient` (direct path, used when the
   factory is built without the queue).
3. `internal/service/broadcast_service.go` `SendToIndividual` (the console's "send test"
   to one address).

The email queue worker never re-derives the URL (it only forwards `Payload.EmailOptions`), so
an entry enqueued before the upgrade keeps what it was enqueued with.

## Implementation

### Domain — `internal/domain/broadcast.go`

Add one method so the three call sites cannot drift apart:

```go
// SendsListUnsubscribeHeader reports whether the broadcast's emails carry the RFC 8058
// List-Unsubscribe headers. A broadcast that keeps the list's unsubscribed contacts is a
// notice every member must receive (security, outage, legal), so it offers no one-click
// unsubscribe.
func (b *Broadcast) SendsListUnsubscribeHeader() bool {
	return b != nil && b.Audience.ExcludeUnsubscribed
}
```

Test file: `internal/domain/broadcast_test.go`
- `TestBroadcast_SendsListUnsubscribeHeader`: true when `ExcludeUnsubscribed` is true; false
  when false; false on a nil broadcast; false on a zero-value audience (a broadcast loaded without the key reads false,
  which already mails unsubscribed contacts, so no header is the consistent answer).

### Service — the three call sites

Each becomes:

```go
if broadcast.SendsListUnsubscribeHeader() {
	if unsubscribeURL, ok := data["oneclick_unsubscribe_url"].(string); ok && unsubscribeURL != "" {
		...ListUnsubscribeURL = unsubscribeURL
	}
}
```

Template data is not touched, so the footer link and message history stay as they are.

Test files and cases:
- `internal/service/broadcast/queue_message_sender_test.go`
  - existing "extracts List-Unsubscribe URL from data": set `Audience.ExcludeUnsubscribed:
    true` on its broadcast (it relied on the zero value).
  - new "no List-Unsubscribe URL when the broadcast includes unsubscribed contacts":
    `ExcludeUnsubscribed: false`, data carries `oneclick_unsubscribe_url` → entry's
    `ListUnsubscribeURL` is empty, and `entry.Payload.TemplateData["oneclick_unsubscribe_url"]`
    is still set.
  - the existing SendBatch test asserting the URL (around line 648) must have a broadcast with
    `ExcludeUnsubscribed: true`; add the false counterpart through `SendBatch` too, so the path
    production uses is covered end to end at unit level.
- `internal/service/broadcast/message_sender_test.go`
  - new `TestSendToRecipient_ListUnsubscribeHeader`: table of true/false; mock
    `emailService.SendEmail` captures the request; true → URL set, false → empty.
- `internal/service/broadcast_service_test.go`
  - new `TestBroadcastService_SendToIndividual_ListUnsubscribeHeader`: same table, capturing the
    `SendEmailProviderRequest` passed to `emailSvc.SendEmail`.
  - check the existing SendToIndividual tests for an assertion on the URL that relied on the
    zero value, and set `ExcludeUnsubscribed: true` there.

### Integration — real SMTP to Mailpit

- `tests/integration/broadcast_audience_statuses_test.go`: the existing subtests already send
  real broadcasts created through the API. Extend `sendToList` to also return the delivered
  messages' headers per recipient, and assert:
  - `exclude_unsubscribed` omitted and `true`: the active contact's email has
    `List-Unsubscribe` and `List-Unsubscribe-Post`.
  - `exclude_unsubscribed` false: neither the active nor the unsubscribed contact's email has
    either header.
  - Needs a testutil helper listing Mailpit message IDs by subject (`tests/testutil/helpers.go`),
    then `GetMailpitMessage` for headers.
- `tests/integration/list_unsubscribe_headers_test.go`: add a subtest sending through
  `broadcasts.sendToIndividual` with a broadcast whose `ExcludeUnsubscribed` is false and
  asserting the delivered email has no `List-Unsubscribe` header.

### Console — `console/src/components/broadcasts/UpsertBroadcastDrawer.tsx`

Add an `extra` help text under the "Exclude unsubscribed recipients" switch:
"Turn off for notices every member must receive, such as security or legal notices: the
list's unsubscribed contacts receive it too, and its emails carry no List-Unsubscribe header,
so mail clients show no Unsubscribe button. The unsubscribe link in your template is not
affected."
Run `npm run lingui:extract`, fill the translations in every locale `.po`, and
`npm run lingui:compile`.

Test: `console/src/components/broadcasts/UpsertBroadcastDrawer.test.tsx` if it exists (render
and find the help text); otherwise the `catalogues.test.ts` locale check covers the
translations. `npm test`, `npm run lint`/typecheck.

### OpenAPI and docs

- `openapi/components/schemas/broadcast.yaml`: both `exclude_unsubscribed` descriptions say
  that `false` also leaves out the `List-Unsubscribe` headers; rebundle `openapi.json`
  (`make openapi-bundle`) and lint.
- `Notifuse/docs` (own branch `docs/...` from `dev`, own PR to `dev`):
  - `features/lists.mdx` line 29 note.
  - `features/notification-center.mdx` (section on the headers) and `features/templates.mdx`
    line 154 ("adds to every email sent to a list") — add the exception.
  - `openapi.json` mirrored from the notifuse bundle.

### CHANGELOG

`42.3` is not released (latest tag v42.2), so one bullet in `## [42.3]`, no version bump:

- **Improvement**: A broadcast that also sends to the list's unsubscribed contacts, for a
  security, outage or legal notice, no longer carries the List-Unsubscribe header, so mail
  clients no longer show an Unsubscribe button on it. Draft and scheduled broadcasts with
  "Exclude unsubscribed recipients" switched off go out this way too.

## Risks

- SES: without a List-Unsubscribe URL (and no attachment) a broadcast email takes the SESv2
  `Simple` path instead of the raw MIME one (`ses_service.go`). That path is the one every
  transactional email already uses: same configuration set, tenant, `notifuse_message_id` tag
  (what SES webhook matching reads first) and captured Message-ID. No change needed; the
  existing SES tests of the simple path cover it.

- Deliverability: Gmail and Yahoo require one-click unsubscribe on bulk marketing mail. The
  change only applies to broadcasts that explicitly mail unsubscribed contacts, which a
  marketing campaign should not do; the help text says what the switch is for.
- A broadcast created through the API before `exclude_unsubscribed` defaulted to true could
  have been stored with `false`; it already mails unsubscribed contacts and will now also lose
  the header. Covered by the CHANGELOG line.

## Steps

1. Tests first (domain, three service sites, integration), see them fail.
2. Implement the domain method and the three call sites.
3. Console help text, lingui extract/compile, translations.
4. OpenAPI yaml + bundle; docs PR.
5. CHANGELOG bullet.
6. Run: `make test-domain`, `make test-service`, `go test ./internal/service/...` (race),
   the integration tests `TestBroadcastAudienceStatusesAsDelivered` and
   `TestListUnsubscribeHeaders` (`INTEGRATION_TESTS=true go test -race -tags integration,licdev
   ./tests/integration/ -run ...`), `go vet ./...`, golangci-lint if configured,
   `cd console && npm test && npm run lint`, `make openapi-lint`.
