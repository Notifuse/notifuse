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
`X-Message-ID` header as `message_id`, and the address it was sent to as `recipient`. The CHANGELOG
says so.

### Consequences

- Events about someone an email was copied to no longer mark that email bounced, complained or
  delivered, nor bounce that address. Same rule as unsubscribes.
- A late bounce or complaint for an erased contact's email is ignored: its address was redacted.
- An event that names no email is not retried. Providers would keep retrying mail of other senders
  that never matches, and batched providers would re-store the matched events of the same delivery.
  The history row is written before the send, or milliseconds after it for queued sends, while provider
  events come seconds later. A queued email whose history row could not be written has its events
  ignored too, a hard bounce included, so its address is not suppressed by it; the email is already
  missing from statistics.
- SES reports a bounced recipient as the delivery report's Final-Recipient when there is one, which for
  a forward or an alias is the address it led to. Such a bounce names another address than the email's,
  so it is ignored: the contact's own mailbox may still accept the email.

## Design

### D1. One lookup per delivery, before anything is stored

`domain.MessageHistoryRepository.GetRecipients(ctx, workspaceID, ids) (map[string]string, error)`:
`SELECT id, contact_email FROM message_history WHERE id = ANY($1) AND contact_email <> 'DELETED_EMAIL'`.
An erased contact's emails name no one, so they are left out. No ids, no query.

`ProcessWebhook` calls a new `eventsOfSentEmails` right after parsing. It collects the message IDs
(deduplicated), calls `GetRecipients` once, keeps an event when its address is non-empty, its ID is
found and `sameAddress(recipient, event.RecipientEmail)` (D3), and rewrites the kept event's address to
the stored one. It logs one Info line per delivery when it leaves events out: workspace, integration,
how many, the first ignored message ID (cut to 255 bytes on a character boundary, since it comes from
the request), no address. A lookup error is returned before anything is stored. When no event is left, `ProcessWebhook` returns nil.

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
spelling of a domain, and SparkPost's sandbox mode sends to the address with its sink domain appended
(`sparkPostSinkSuffix`). `sameAddress(stored, reported)`, surrounding spaces trimmed, is true when the
two are equal case aside; otherwise the sink domain is removed from `reported`, which is then compared
by `addressForm` with `stored` and with `stored` as SES is sent it (its domain converted to Punycode as
stored, uncomposed and in its letter case). An encoded local part is compared decoded, since Base64 is
case-sensitive. `addressForm` is the local part
decoded when encoded and lowercased, the domain as DNS looks it up (`idna.Lookup`: Punycode, case and
Unicode composition aside), or lowercased as is when DNS refuses it. A domain longer than DNS holds,
253 characters (`maxDomainLength`), is refused before the lookup, whose Punycode encoding takes time
growing with the square of a label's length; so is a reported domain longer than both forms of the
stored one by more than 8 characters DNS ignores (`maxIgnoredDomainCharacters`), since one delivery can
report thousands of addresses for one email. Decoding an encoded local part takes time growing with its
length too, so an address of more than 2048 bytes (`maxAddressLength`, beyond any spelling of an address
SMTP carries) has no form.
A value with no `@`, or an empty local part or domain, has no form.

### D4. One SES event per listed recipient

An SES notification lists every recipient of the email it reports on, an address the email was copied
to among them, and the parser used only the first. It now gives one event per listed recipient (a
bounce keeps each recipient's own diagnostic), so the email's own recipient is found whatever its place
in the list. An address listed twice, case aside, gets one event, so a bounce counts once. At most
50 recipients are read, the most an SES email can have, and the events share one copy of the payload. A
notification listing none, or an empty address, gives no event for it. The other providers already
send one event per recipient. The address reported is kept as is, so a forward's or an alias's address
is not taken for the email's own (see Consequences).

## Implementation files and their tests

| Implementation file | Test file | Cases |
|---|---|---|
| `internal/domain/message_history.go` (interface) and the regenerated `internal/domain/mocks/mock_message_history_repository.go` | `internal/repository/message_history_postgre_test.go`, `TestMessageHistoryRepository_GetRecipients`; `internal/service/inbound_webhook_event_service_test.go`, through the mock | an interface method only: its implementation's cases are in the repository row below, and every service case that reaches the lookup expects it on the mock |
| `internal/domain/inbound_webhook_event.go` (`CountConsecutiveSoftBounces` doc comment only) | `internal/repository/inbound_webhook_event_postgres_test.go`, `TestCountConsecutiveSoftBounces_OnlyCountsBouncesOfEmailsSentToTheAddress`; `tests/integration/email_event_message_match_test.go` | the rule it documents: the join is in the query; stored rows naming no email, or another address's, are not counted |
| `internal/repository/message_history_postgre.go` (`GetRecipients`) | `internal/repository/message_history_postgre_test.go`, `TestMessageHistoryRepository_GetRecipients` | found rows mapped, ids passed as one array with the redacted address; no ids, no connection; connection, query, scan and row errors returned |
| `internal/repository/inbound_webhook_event_postgres.go` (`countConsecutiveSoftBouncesSQL`) | `internal/repository/inbound_webhook_event_postgres_test.go`, `TestCountConsecutiveSoftBounces_OnlyCountsBouncesOfEmailsSentToTheAddress`; the no-prior-delivery regex follows the join | the join is in the query |
| `internal/service/inbound_webhook_event_service.go` (`ProcessWebhook`, `eventsOfSentEmails`) | `internal/service/inbound_webhook_event_service_test.go`, `TestProcessWebhook_ActsOnlyOnEmailsSentToTheEventAddress` | hard bounce of an unknown email; of another address's email; soft bounce of an unknown email not counted; event carrying only the provider's ID; no ID or no address, no lookup; address in another case kept under the stored address, for a hard and a soft bounce; complaint and delivery of another address's email; SendGrid batch: one lookup with deduplicated IDs, only the matching event acted on; SES bounce, complaint and delivery listing a copied address first: only the recipient's event stored and applied; SES bounce reporting the address an email sent to its recipient alone was forwarded to: ignored; SES complaint giving the stored international domain in Punycode: kept under the stored address; Mailjet unsubscribe of an unknown email; lookup error returned, nothing stored; the Info log line's fields; a long ignored ID logged cut to its first 255 bytes, and cut on a character boundary |
| same file (`sameAddress`, `addressForm`) | same file, `TestSameAddress` | same address; case; Punycode reported for a Unicode domain and the reverse; Unicode domain in capitals; non-ASCII local part as SES is sent it, in capitals, reported lowercase, reported encoded in lowercase; another non-ASCII local part; another domain; another local part; no address; no domain; a decomposed domain reported composed, in Punycode, and as SES is sent it; a domain DNS refuses, in another case, with an encoded local part, with another local part; `straße.de` and `strasse.de` apart; the SparkPost sandbox form, and another address in it; spaces around the reported address; a character DNS ignores; a stored domain longer than DNS holds not looked up; as many characters DNS ignores as room is left for, and one more, not looked up; a local part encoded in two words; an address longer than any spelling of one not decoded; an international domain in Punycode as the SparkPost sandbox sends it; a local part as SES is sent it with another letter case, which decodes to another; `TestSameAddress_ALongAddressIsComparedQuickly`: a very long domain, a very long encoded local part, and many domains as long as DNS holds against a short one, each compared in under a second; `TestAddressForm_ADomainLongerThanDNSHoldsIsNotLookedUp`: given room for it, a domain longer than DNS holds is still not looked up |
| same file (`processSESWebhook`) | same file, `TestProcessSESWebhook` | a bounce, a complaint and a delivery listing two recipients give two events in order, each with its address, the Notifuse ID and, for a bounce, its own diagnostic; an address listed twice gives one event, the first; 60 listed recipients give 50 events sharing the payload; none listed gives no event; a forwarded address is kept as reported, for an email sent to one address or to several; an empty address gives no event |
| | same file, existing tests | each test that reaches storage expects `GetRecipients`; "events without message ID" now stores nothing; `TestProcessWebhook_MailjetUnsubscribe` adds an unknown email and a copied address, and its race cases cover an email gone, readdressed or erased by the time the opt-out is recorded |
| `internal/service/sparkpost_service.go` (`sparkPostSinkSuffix`) | `internal/service/sparkpost_service_test.go`, `TestSparkPostService_SendEmail` "Sandbox mode" | the email goes to the address on the sink domain |
| full stack | `tests/integration/email_event_message_match_test.go`, `TestEmailEventMessageMatch` (SES and SparkPost) | hard bounce for a contact with no email; naming another contact's email; naming an erased contact's email; address upper-cased: bounced and stored under the contact's address; soft bounces of unknown emails never reach the threshold; rows stored earlier that name no email, or another contact's, are not counted (each kind alone would reach the threshold); genuine soft bounces reach it; complaint and delivery with another address change nothing; delivery with the right address marks the email delivered; SES bounce listing a copied contact first bounces the recipient only; SES bounce reporting the address the contact's email was forwarded to changes nothing; SES bounce giving a contact's international domain in Punycode bounces the contact; SparkPost batch with a mixed-case contact; SparkPost bounce of a sandbox email, for the address on the sink domain, bounces the contact |
| | `tests/integration/bounce_test.go` | every posted bounce names an email sent to the contact; the delivery-reset case dates its bounces around a delivered email; `sesBounceOf` builds a bounce for any recipients and destination |

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
INTEGRATION_TESTS=true go test -race -tags integration,licdev -timeout 20m ./tests/integration/ \
  -run 'TestSESBounceHandling|TestEmailEventMessageMatch|TestWebhookSubscriptionStatusE2E|TestAutomationComplaint|TestMessageStatusRecordsTheListRow|TestMailjetUnsubscribeWebhook' -v
make test-integration
```
