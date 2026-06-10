# Inbound Reply Detection (stop-on-reply) — Plan

Implements the **inbound ingestion** layer of issue #346: ingest reply emails and
emit an `email.replied` event that automations can trigger on (enabling
stop-on-reply sequences). Scope is intentionally limited to inbound ingestion +
event emission — the automation that reacts to the reply is built by the user
with the existing automation builder (the trigger mechanism already exists).

## Decisions

- **Provider (first cut):** Mailgun Routes inbound parsing. The handler/service
  are structured so other providers (Postmark Inbound, SES Inbound via SNS) slot
  in behind the same endpoint by switching on the integration's provider kind.
- **Reply→contact matching (first cut):** by **sender address**. The reply's
  `sender` (fallback: parsed `from` header) is matched to an existing contact.
  This needs no changes to outbound sending and is sufficient for stop-on-reply
  (we only need to know *that* a known contact replied). `In-Reply-To` /
  `Message-Id` are captured on the event for future per-send attribution.
- **Event surfacing:** replies are stored in the existing `inbound_webhook_events`
  table with `type = "reply"`, and the timeline trigger emits the dedicated
  contact-timeline kind `email.replied`. This reuses the existing inbound
  multi-provider pattern and the existing automation trigger mechanism
  (automation triggers fire on `contact_timeline` inserts whose `kind` matches
  the trigger's event kind).

## Why `email.replied` (not `reply_email`)

Automation triggers compare `contact_timeline.kind` to the configured event kind
literally (`automation_trigger_generator.go` `buildWHENClause`). The working
trigger kinds (`segment.joined`, `list.subscribed`, `custom_event.*`,
`contact.created`) are dotted and match `ValidEventKinds` exactly. Using the
dotted `email.replied` both in `ValidEventKinds` and as the timeline kind keeps
the trigger self-consistent. (Note: the message-history-sourced email kinds use
a different `verb_channel` shape — `open_email`, `click_email` — which does *not*
match the dotted `email.opened` in `ValidEventKinds`; that pre-existing
inconsistency is out of scope here and untouched.)

## Changes

| File | Change |
|------|--------|
| `internal/domain/inbound_webhook_event.go` | New `EmailEventReply` type; `ProcessInboundReply` on the service interface; allow `reply` in the list filter. |
| `internal/domain/automation.go` | Add `email.replied` to `ValidEventKinds`. |
| `internal/database/init.go` | `track_inbound_webhook_event_changes()` emits `email.replied` for `type = 'reply'`. |
| `internal/migrations/v33.go` | Workspace migration re-creating the trigger function on existing installs. |
| `config/config.go` | `VERSION` 32.2 → 33.0. |
| `internal/service/inbound_webhook_event_service.go` | `ProcessInboundReply` + `parseMailgunInboundReply` + `extractEmailAddress`. |
| `internal/http/inbound_webhook_event_handler.go` | Route `POST /webhooks/email/inbound` + `handleIncomingReply` (multipart/urlencoded form parsing). |
| `internal/domain/mocks/mock_inbound_webhook_event_service.go` | Hand-added `ProcessInboundReply` (no mockgen in env). |
| `internal/migrations/manager_test.go` | "up to date" version 32 → 33. |
| `CHANGELOG.md` | 33.0 entry. |

## Tests

- `internal/service/inbound_reply_service_test.go`: known-contact happy path
  (asserts reply event shape + `In-Reply-To` preference), unknown contact
  ignored (no store), `from`-header fallback + lowercasing + `Message-Id`
  fallback, missing sender error, unsupported provider error.
- `internal/http/inbound_webhook_event_handler_test.go`: method-not-allowed,
  missing params, success (form parsed and forwarded), service error.

## Provider setup (Mailgun)

Create a Mailgun Route with action
`forward("https://<host>/webhooks/email/inbound?workspace_id=<ws>&integration_id=<int>")`
(store-and-notify also works). Set the campaign/automation emails' `Reply-To`
(or `From`) to a domain Mailgun receives, so replies land on the Route.

## Out of scope / follow-ups

- **Signature verification:** the existing event webhook handler doesn't verify
  provider signatures; this endpoint matches that for parity. Forged replies can
  at most stop a sequence early. Hardening (Mailgun timestamp/token/signature
  HMAC) is a follow-up.
- **Other providers:** Postmark Inbound, SES Inbound via SNS.
- **Per-send attribution:** match `In-Reply-To`/`References` to a stored
  `Message-Id` (needs persisting the SMTP Message-Id per send).
- **Restrict to recent recipients:** only emit when the contact actually has a
  recent send (reduces noise from unrelated inbound).
- **OpenAPI:** document the new endpoint in `openapi.json` (generated file).
- **Console UI:** the automation builder already lists trigger event kinds from
  `ValidEventKinds`; verify `email.replied` is selectable / labeled there.
