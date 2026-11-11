# Feature: choose the sender at send time (issue #418)

## Goal

An email integration can hold several senders, but only the template picks one
(`email.sender_id`, else the integration's default). Users want to pick the sender:

- on a broadcast, per variation (one dropdown without A/B test, one per variation with it);
- on a transactional API call (`email_options.sender_id`).

Only senders already declared on the integration can be chosen, never a free address (ESPs require
verified senders; same stance as #43, which added `from_name` only). The template's sender stays the
default. UI mock-ups: `maquettes/issue-418/` in the project files.

Out of scope: a sender on the transactional notification itself (the template already does that), the
automation email node's dead `from_override` field, a warning when a sender used by a broadcast is
deleted from its integration.

## Resolution rule

`domain.EmailProvider.ResolveSender(overrideID, templateSenderID string) (*EmailSender, error)`:

- `overrideID != ""`: the sender with that ID, **no fallback**; missing → `ErrSenderNotFound`
  (a typed error naming the ID). Sending from another address than the one chosen is never right.
- otherwise: today's `GetSender(templateSenderID)` (template's sender, else the default).

`from_name` (transactional) keeps applying on top of the resolved sender.

## Storage — no SQL change

- `BroadcastVariation.SenderID string` (`json:"sender_id,omitempty"`), stored in the existing
  `broadcasts.test_settings` JSONB. Broadcasts stored without it keep today's behaviour.
- `EmailOptions.SenderID string` (`json:"sender_id,omitempty"`), request-only. `IsEmpty` /
  `ToChannelOptions` unchanged: message history does not store the From today.
- No migration, no environment variable. No version bump of its own (`dev` already carries 44.0), CHANGELOG `Feature` bullet under `## [44.0] - unreleased`.

## Broadcast

1. **Domain** (`internal/domain/broadcast.go`): `SenderID` on `BroadcastVariation`;
   `(*Broadcast).VariationSenderID(templateID string) string`. `Broadcast.Validate` refuses two
   variations with the same `template_id` and different `sender_id` (the sender is looked up by
   template; the console already forbids duplicate templates, the API did not).
   `UpdateBroadcastRequest` gets a `TestSettingsProvided() bool` accessor.
2. **Validation on save** (`internal/service/broadcast_service.go`): `CreateBroadcast`, and
   `UpdateBroadcast` only when the request carries `test_settings` (a patch renaming a broadcast whose
   stored sender has since gone must still pass). Every variation that sets `sender_id` must name a
   sender of the workspace's marketing integration, else `domain.ValidationError`. Variations without
   one need no provider. A paused broadcast may change sender (needed to recover from the pause in
   step 3); the emails already sent keep theirs.
3. **Validation before sending**, scoped to the variations the phase sends:
   - `ScheduleBroadcast` (send now and scheduled): all variations.
   - `ResumeBroadcast`: only on the phase-1 branches (send now / back to scheduled); a broadcast whose
     recipients are all queued (`CompletedAt != nil`) only un-pauses rows that already carry their
     From, so it is not checked.
   - Orchestrator (`internal/service/broadcast/orchestrator.go`, after `ValidateTemplates`): test phase
     and single send check every variation, winner phase only the winning one. A missing sender
     **pauses** the broadcast with a `PauseReason` naming the variation and the sender, through the
     same code as the recipient-feed pause (extracted into one helper), rather than failing it:
     `failed` is terminal and `BroadcastError.Retryable` is not read in production. The error is a
     typed `SenderUnavailableError` (matching the pause sentinel and `ErrSenderNotFound`), so the
     reason is built from its fields, not from the sentinel's text. The integration is read once per
     task run, so a sender removed during a run is caught at the next run, not the next batch. The
     check is skipped while the broadcast is `paused` (the loop stops it anyway, and a paused run is
     not announced as sending started) or `test_completed` (it waits for a manual winner: pausing it
     on a losing variation would block the winner choice); `SendBatch` still checks the templates of
     any batch it is given. The pause reason names the variation by its name, else by its number
     (the console saves none), else "this broadcast".
4. **Send paths** resolve with `ResolveSender(broadcast.VariationSenderID(template.ID), emailContent.SenderID)`:
   `broadcast/message_sender.go`, `broadcast/queue_message_sender.go` (`buildQueueEntry`),
   `broadcast_service.go` (`SendToIndividual`). In both `SendBatch`, the overrides of the batch's
   templates are checked once **before** the recipient loop and a missing one returns an error wrapping
   the pause sentinel, so no recipient is skipped and lost (a per-recipient `buildQueueEntry` error is
   counted and dropped today). Queued entries keep the From resolved at enqueue, as today.
5. **HTTP** (`internal/http/broadcast_handler.go`): create, update, schedule, resume and
   send-to-individual map `domain.ValidationError` (value type, `errors.As`) and
   `domain.ErrSenderNotFound` to 400 with the message; today every service error there is a 500.

Sender IDs belong to one integration: switching the workspace's marketing integration invalidates the
stored choices. Scheduling then refuses with a clear message and the broadcast stays editable.

## Transactional API

`internal/service/transactional_service.go`, `SendNotification`: when the email channel is requested
and `email_options.sender_id` is set, resolve the transactional provider and check the sender
**before** the contact upsert, and return the error (`sender not found: <id>`, mapped to 400 by the
handler) instead of letting the channel loop swallow it into a 500. This also applies to a replayed
`external_id` (the check runs first) and to the SMTP bridge, which decodes the same request (a
relayed JSON body can carry `email_options.sender_id`); the Supabase callers pass none. `internal/service/email_service.go`, `SendEmailForTemplate`:
`ResolveSender(request.EmailOptions.SenderID, emailContent.SenderID)`. The template test endpoint
keeps its top-level `sender_id`; `sender_id` is added to the send schema's `email_options` only.

## Console

- `console/src/services/api/broadcast.ts`: `sender_id?: string` on `BroadcastVariation`;
  `transactional_notifications.ts`: `sender_id?: string` on `EmailOptions`.
- `console/src/components/broadcasts/UpsertBroadcastDrawer.tsx`: a `VariationSenderSelect` (new file
  `console/src/components/broadcasts/VariationSenderSelect.tsx`) under each template field, options =
  senders of the marketing integration (`currentWorkspace.integrations` +
  `settings.marketing_email_provider_id`), tags "template's" / "default", `allowClear`, placeholder
  "Template's sender (<name>)" read from the selected template. A stored sender no longer on the
  integration shows as "(deleted)" with a field error.
- `console/src/pages/BroadcastsPage.tsx`: the "From:" line shows the variation's sender when set, with
  a "chosen on the broadcast" tag; also reads the marketing provider rather than the template
  category's (a broadcast always sends through the marketing one).
- The drawer gets the workspace as its `workspace` prop; both template fields get a sender field
  registered as a `Form.Item` (`Form.List` items and the non-A/B `variations[0]`), since `onFinish`
  only sends registered fields. Switching A/B off keeps variation 1's sender.
- `console/src/components/broadcasts/ChangeVariationSenderModal.tsx` (new): on a paused broadcast
  not fully queued (no `completed_at`: queued emails keep their sender) whose variation still to
  send (the winner once chosen) lost its sender, a "Change sender" button
  next to Resume opens one sender field per template whose sender is gone (the server checks them
  all on save), labelled by variation name or number, and saves through `broadcasts.update` (the
  edit drawer only opens drafts and scheduled broadcasts). A failed resume shows the server's
  message.
- Lingui: wrap strings, `npm run lingui:extract`, translate every catalogue (ca, de, es, fr, it, ja,
  pt-BR): `catalogues.test.ts` fails on any empty `msgstr`.

## Docs

- `openapi/components/schemas/broadcast.yaml` (`BroadcastVariation.sender_id`, shared by requests and
  responses; note that `test_settings` is replaced whole on update, so a client rebuilding variations
  without `sender_id` goes back to the template's sender),
  `openapi/components/schemas/transactional.yaml` (`email_options.sender_id`), bundle `openapi.json`.
- `../docs` (separate PR, `docs/` branch to `dev`): `features/broadcast-campaigns.mdx`,
  `features/transactional-api.mdx`, and the docs copy of `openapi.json` (only these schema entries).

## Tests (TDD: tests first)

| Implementation | Test file | Cases |
| --- | --- | --- |
| `domain/email_provider.go` `ResolveSender` | `domain/email_provider_test.go` | override found; override missing → `ErrSenderNotFound`, no fallback; no override → template's; no override, template's missing → default; no sender at all → plain error, not `ErrSenderNotFound` |
| `domain/broadcast.go` | `domain/broadcast_test.go` | `VariationSenderID` found / not set / unknown template; same template twice with different senders refused, same sender accepted; `TestSettingsProvided`; JSON round-trip keeps `sender_id`, old JSON without it decodes empty |
| `service/broadcast_service.go` | `service/broadcast_service_sender_test.go` | create/update with known sender OK; unknown sender → `ValidationError`; no marketing provider + sender → error; no sender set → no provider check; update without `test_settings` on a broadcast whose sender left → accepted; schedule refused when a variation's sender left; resume phase 1 refused, resume of a fully queued broadcast accepted; `SendToIndividual` uses the variation's sender |
| `broadcast/orchestrator.go` | `broadcast/orchestrator_sender_test.go` | missing sender → broadcast paused with a reason naming the variation and the sender (not the feed sentinel), nothing sent or queued; winner phase with a losing variation's sender gone → sends; `test_completed` and `paused` with a sender gone → not paused, and a paused run not announced as sending started; pause reason for named, unnamed and single variations |
| `broadcast/message_sender.go`, `queue_message_sender.go` | `broadcast/variation_sender_test.go`, `queue_message_sender_test.go` | variation sender wins over the template's (non-default sender fixture: `NewEmailSender` sets `IsDefault: true`); missing override → batch returns the pause error before any recipient, none counted as failed |
| `service/transactional_service.go`, `email_service.go` | `service/transactional_sender_test.go` | `email_options.sender_id` → From; unknown → error before the contact upsert and before any message, also on a replayed `external_id`; `from_name` still applies on top |
| `http/transactional_handler.go`, `http/broadcast_handler.go` | `http/transactional_handler_test.go`, `http/broadcast_handler_sender_test.go` | unknown sender → 400 on send, create, update, schedule, resume, send-to-individual; other service errors stay 500 |
| console drawer + select + page | `UpsertBroadcastDrawer.test.tsx`, `VariationSenderSelect.test.tsx`, `BroadcastsPage` test if present | options from the marketing integration; clear = template's; payload carries `sender_id`; deleted sender shown in error and still clearable; no template's sender named before a template is chosen; "From:" shows the override |
| change-sender modal | `ChangeVariationSenderModal.test.tsx` | shown only on a paused, not fully queued broadcast whose variation to send has a missing sender, not for a loser's alone; disabled without write permission; asks only for the missing variations, numbered when unnamed; saves the new sender and keeps the others and the A/B settings, without the joined template or a schedule; refuses the deleted sender |
| integration | `tests/integration/broadcast_sender_test.go` (new), `tests/integration/transactional_handler_test.go` | Mailpit From for a broadcast with a variation sender (A/B and single), for a transactional `sender_id`; unknown sender → 400 on create and on send; sender removed from the integration → schedule refused |

## Review

A counter-review of version 1 raised findings, each checked in the code (handlers' 500s, unread
`Retryable`, catalogue test) and folded in above. Decided against finding 13's guard: a paused
broadcast may change sender, otherwise a sender deleted mid-send could never be replaced. The first
code review found no console way back from that pause, a pause reason quoting the feed sentinel, an
A/B test waiting for its winner paused by a losing variation, and a pause timing claim stated per
batch; all four are fixed above.

## Test commands

```bash
go test -race ./internal/domain -run 'ResolveSender|VariationSenderID|Broadcast'
go test -race ./internal/service ./internal/service/broadcast -run 'Sender|Broadcast|Transactional|SendEmailForTemplate'
go test -race ./internal/http -run 'Transactional|Broadcast'
cd console && npm test -- --run src/components/broadcasts && npx eslint src/components/broadcasts src/pages/BroadcastsPage.tsx && npx tsc -b --noEmit
INTEGRATION_TESTS=true go test -race -tags integration,licdev -timeout 20m ./tests/integration/ -run 'SendTimeSender|Transactional' -v
make test-unit
cd console && npm test
```
