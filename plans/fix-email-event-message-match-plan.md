# Fix: provider events act only on emails Notifuse sent to the address they name

Created 2026-10-21. Target: 42.0 (unreleased). No schema change, no migration.

## Behaviour

`POST /webhooks/email` receives delivery, bounce, complaint and unsubscribe events from the email
providers. Mailjet unsubscribes already checked that the email they name was sent to their address
(`recordUnsubscribes`, since #459). Every event type now follows the same rule.

Now an event goes further only when it names an email of the workspace and that email was sent to the
address the event reports, the letter case and the spelling of an international domain aside (D3).
Everything else is left out before anything is
stored: no `inbound_webhook_events` row (so no timeline entry and no line under Logs → Email Provider
Webhooks), no status update, no bounce, no soft-bounce count.

Kept events take the address stored on their email, which is the contact's own key in `contact_lists`,
so a provider that lowercases addresses (SparkPost documents `rcpt_to` as lowercase) bounces a contact
whose address has capitals.

### Where the Notifuse ID comes from

Every provider integration attaches the Notifuse message ID when sending and the parsers read it back.
When it is missing, a parser falls back to the provider's own ID, which names no Notifuse email, so the
event is now ignored. That only happens for mail Notifuse did not send (another application on the same
provider account, the integration's test email, which writes no message history) or very old messages.
Only the SES parser changes (D4).

| Provider | Sent as | Read from |
|---|---|---|
| SES | `notifuse_message_id` tag, `X-Message-ID` header on raw sends | `mail.tags`, then `mail.headers` |
| Postmark | `Metadata.notifuse_message_id` | `Metadata` |
| Mailgun | `v:notifuse_message_id` | `event-data.user-variables` |
| SparkPost | transmission `metadata.notifuse_message_id` | `rcpt_meta` (SparkPost copies transmission metadata there) |
| Mailjet | `CustomID` | `CustomID` |
| SendGrid | `custom_args.notifuse_message_id` | `notifuse_message_id` |
| SMTP | `X-Message-ID` header | `message_id` of the posted payload |

SMTP has no provider webhook; a relay of the operator's own that posts events must send the email's
`X-Message-ID` header as `message_id`. The CHANGELOG says so.

### Consequences

- Events about someone an email was copied to no longer mark that email bounced, complained or
  delivered, nor bounce that address. Same rule as unsubscribes.
- A late bounce or complaint for an erased contact's email is ignored: its address was redacted.
- An event that names no email is not retried. Providers would keep retrying mail of other senders
  that never matches, and batched providers would re-store the matched events of the same delivery.
  The history row is written before the send, or milliseconds after it for queued sends, while provider
  events come seconds later. The queue worker now writes the row before marking its entry sent (D5),
  so a failed mark no longer leaves a sent email without its row.

## Design

### D1. One lookup per delivery, before anything is stored

`domain.MessageHistoryRepository.GetRecipients(ctx, workspaceID, ids) (map[string]string, error)`:
`SELECT id, contact_email FROM message_history WHERE id = ANY($1) AND contact_email <> 'DELETED_EMAIL'`.
An erased contact's emails name no one, so they are left out. No ids, no query.

`ProcessWebhook` calls a new `eventsOfSentEmails` right after parsing. It collects the message IDs
(deduplicated), calls `GetRecipients` once, keeps an event when its address is non-empty, its ID is
found and `sameAddress(recipient, event.RecipientEmail)` (D3), and rewrites the kept event's address to
the stored one. It logs one Info line per delivery when it leaves events out: workspace, integration,
how many, the first ignored message ID, no address. A lookup error is returned before anything is
stored. When no event is left, `ProcessWebhook` returns nil.

The rest of `ProcessWebhook` is unchanged and only sees kept events. `recordUnsubscribes` keeps its own
lookup, which it needs for the list and for a contact erased in between.

### D2. The soft-bounce count joins the email

`countConsecutiveSoftBouncesSQL` gains
`JOIN message_history mh ON mh.id = e.message_id AND mh.contact_email = e.recipient_email`.
New rows already pass D1, so this only leaves out rows stored before the upgrade that name no email,
or another address's. One-time side effect: a contact with capitals whose earlier soft bounces were
stored under a provider's lowercase spelling starts its count again from zero.

### D3. Comparing the addresses

SES is sent the address as `encodeEmailAddress` writes it, an international domain in Punycode and a
non-ASCII local part RFC 2047-encoded, and reports that form; other providers may report either
spelling of a domain. `sameAddress(stored, reported)` is true when the two are equal case aside, when
`reported` is `stored` as SES is sent it, or when both have the same `addressForm`: the local part
decoded when encoded and lowercased, the domain as DNS looks it up (`idna.Lookup`: Punycode, case and
Unicode composition aside), or lowercased as is when DNS refuses it. A value with no `@`, or an empty
local part or domain, has no form.

### D4. One SES event per listed recipient

An SES notification lists every recipient of the email it reports on, an address the email was copied
to among them, and the parser used only the first. It now gives one event per listed recipient (a
bounce keeps each recipient's own diagnostic), so the email's own recipient is found whatever its place
in the list. An address listed twice, case aside, gets one event, so a bounce counts once. At most
50 recipients are read, the most an SES email can have, and the events share one copy of the payload. A
notification listing none gives no event. The other providers already send one event per recipient.

### D5. The queue worker records the email before marking its entry sent

`EmailQueueWorker.processEntry` upserted the history row after `MarkAsSent`, and returned before it
when the mark failed. The upsert now comes first, right after the provider accepted the email.

## Implementation files and their tests

| Implementation file | Test file | Cases |
|---|---|---|
| `internal/domain/message_history.go` (interface) and the regenerated `internal/domain/mocks/mock_message_history_repository.go` | covered through the repository and service tests | |
| `internal/repository/message_history_postgre.go` (`GetRecipients`) | `internal/repository/message_history_postgre_test.go`, `TestMessageHistoryRepository_GetRecipients` | found rows mapped, ids passed as one array with the redacted address; no ids, no connection; connection, query, scan and row errors returned |
| `internal/repository/inbound_webhook_event_postgres.go` (`countConsecutiveSoftBouncesSQL`) | `internal/repository/inbound_webhook_event_postgres_test.go`, `TestCountConsecutiveSoftBounces_OnlyCountsBouncesOfEmailsSentToTheAddress`; the no-prior-delivery regex follows the join | the join is in the query |
| `internal/service/inbound_webhook_event_service.go` (`ProcessWebhook`, `eventsOfSentEmails`) | `internal/service/inbound_webhook_event_service_test.go`, `TestProcessWebhook_ActsOnlyOnEmailsSentToTheEventAddress` | hard bounce of an unknown email; of another address's email; soft bounce of an unknown email not counted; event carrying only the provider's ID; no ID or no address, no lookup; address in another case kept under the stored address, for a hard and a soft bounce; complaint and delivery of another address's email; SendGrid batch: one lookup with deduplicated IDs, only the matching event acted on; SES bounce, complaint and delivery listing a copied address first: only the recipient's event stored and applied; SES complaint giving the stored international domain in Punycode: kept under the stored address; Mailjet unsubscribe of an unknown email; lookup error returned, nothing stored; the Info log line's fields |
| same file (`sameAddress`, `addressForm`) | same file, `TestSameAddress` | same address; case; Punycode reported for a Unicode domain and the reverse; Unicode domain in capitals; non-ASCII local part as SES is sent it, in capitals, reported lowercase, reported encoded in lowercase; another non-ASCII local part; another domain; another local part; no address; no domain; a decomposed domain reported composed, in Punycode, and as SES is sent it; a domain DNS refuses, in another case; `straße.de` and `strasse.de` apart |
| same file (`processSESWebhook`) | same file, `TestProcessSESWebhook` | a bounce, a complaint and a delivery listing two recipients give two events in order, each with its address, the Notifuse ID and, for a bounce, its own diagnostic; an address listed twice gives one event, the first; 60 listed recipients give 50 events sharing the payload; none listed gives no event |
| `internal/service/queue/worker.go` (`processEntry`) | `internal/service/queue/worker_test.go`, `TestEmailQueueWorker_ProcessEntry_RecordsTheEmailBeforeMarkingTheEntry` | the row is upserted before `MarkAsSent`, and still when the mark fails; the sent callback runs only when the entry is marked |
| | same file, existing tests | each test that reaches storage expects `GetRecipients`; "events without message ID" now stores nothing; `TestProcessWebhook_MailjetUnsubscribe` adds an unknown email and a copied address, and its race cases cover an email gone, readdressed or erased by the time the opt-out is recorded |
| full stack | `tests/integration/email_event_message_match_test.go`, `TestEmailEventMessageMatch` (SES and SparkPost) | hard bounce for a contact with no email; naming another contact's email; naming an erased contact's email; address upper-cased: bounced and stored under the contact's address; soft bounces of unknown emails never reach the threshold; rows stored earlier that name no email, or another contact's, are not counted (either kind would reach the threshold); genuine soft bounces reach it; complaint and delivery with another address change nothing; delivery with the right address marks the email delivered; SES bounce listing a copied contact first bounces the recipient only; SES bounce giving a contact's international domain in Punycode bounces the contact; SparkPost batch with a mixed-case contact |
| | `tests/integration/bounce_test.go` | every posted bounce names an email sent to the contact; the delivery-reset case dates its bounces around a delivered email |

`CHANGELOG.md`: one **Security** bullet under 42.0 with the SMTP relay action in bold.
Docs (`features/logs.mdx`, separate PR to Notifuse/docs `dev`): the Email Provider Webhooks tab lists
events about emails Notifuse sent, for their recipient.

## Test step

```
go build ./...
go vet ./internal/service ./internal/repository ./internal/domain
make test-domain
make test-repo
make test-service
make test-http
go test -race ./internal/service/queue
INTEGRATION_TESTS=true go test -race -tags integration,licdev -timeout 20m ./tests/integration/ \
  -run 'TestSESBounceHandling|TestEmailEventMessageMatch|TestWebhookSubscriptionStatusE2E|TestAutomationComplaint|TestMessageStatusRecordsTheListRow|TestMailjetUnsubscribeWebhook' -v
make test-integration
```
