# Fix: translated emails lose the template's sender and reply-to

## Problem

`Template.ResolveEmailContent` (`internal/domain/template.go`) returns a translation's
`EmailTemplate` as stored. The console never writes `sender_id` or `reply_to` into a translation
(`console/src/components/templates/CreateTemplateDrawer.tsx`, the `translations[lang]` payload only
carries subject, preview and content), and the template editor has no per-language sender field.

So for every contact whose language has a translation, every send path resolves an empty
`SenderID` and an empty `ReplyTo`:

- `EmailProvider.GetSender("")` falls back to the integration's **default** sender instead of the
  sender picked on the template;
- the template's reply-to is dropped.

Affected send paths (all go through `ResolveEmailContent`):

| Path | File |
| --- | --- |
| Transactional API, list double opt-in / welcome emails | `internal/service/email_service.go` (`SendEmailForTemplate`) |
| Template test send | `internal/service/transactional_service.go` (`TestTemplate`, reply-to only: sender is chosen in the dialog) |
| Broadcast, direct sender | `internal/service/broadcast/message_sender.go` |
| Broadcast, queue sender | `internal/service/broadcast/queue_message_sender.go` |
| Broadcast, send-to-individual | `internal/service/broadcast_service.go` |
| Automation email node | `internal/service/automation_node_executor.go` |

Only installations licensed for `template_i18n` can author translations, but resolving stored ones
is never gated, so a lapsed licence still sends them.

## Fix

Resolve, do not persist. In `ResolveEmailContent`, when a translation is picked:

- if the translation has no `SenderID`, use the base template's `SenderID`;
- if the translation has no `ReplyTo`, use the base template's `ReplyTo`;
- a value set on the translation itself (possible through the API) still wins;
- return a **shallow copy** of the translation's `EmailTemplate` with those two fields filled, so the
  stored template (which callers may hold, cache or re-save) is never mutated.

Nothing else changes: subject, preview, content, editor mode, MJML source and plain text stay
per-language. When the default content is returned (no language, default language, no translation,
translation without email), the function keeps returning `t.Email` itself.

Consequence, accepted: an empty `sender_id` / `reply_to` on a translation now means "same as the
default content". A translation can no longer mean "integration default sender" or "no reply-to"
while the base template sets one; the console cannot express either anyway.

### Explicit reply-to on a send wins over the template's

`SendEmailForTemplate` (`internal/service/email_service.go`, "optional override for reply to")
replaces an explicit `email_options.reply_to` with the template's reply-to whenever the template has
one. The API documents `reply_to` as "Set custom reply-to email address", and `TestTemplate` already
lets the explicit one win. Today translated sends escape that override only because their reply-to is
empty; inheriting it would extend the override to them. So the same PR makes the template's reply-to
a fallback there too: applied only when the send gives none. Covers transactional sends, emails
relayed over SMTP (whose `Reply-To` header fills `email_options.reply_to`), and list
double opt-in / welcome emails (the latter never set one, so they keep the template's).

Why at resolution rather than on save: it heals templates already stored without a migration, covers
translations written by the API, and keeps one rule in one place for all six send paths.

No SQL change, no environment variable. Version bump 43.0 → 43.1 (minor), CHANGELOG entry under
`## [43.1] - WIP`.

## Files and tests (TDD: tests first)

### `internal/domain/template.go`

`ResolveEmailContent`: copy-and-inherit as described.

Tests — `internal/domain/template_test.go`, new `TestTemplate_ResolveEmailContent_InheritsSenderAndReplyTo`:

1. translation without `sender_id` inherits the base template's `sender_id`;
2. translation without `reply_to` inherits the base template's `reply_to`;
3. translation with its own `sender_id` / `reply_to` keeps them, each field on its own (new `it` fixture, `it-sender`);
4. base template without sender or reply-to: translation stays empty (default sender is then picked
   by `GetSender`, as today);
5. resolving does not mutate the stored translation (`Translations["it"].Email.SenderID` and
   `ReplyTo` still empty afterwards) nor the base template;
6. the returned translation keeps its own subject, preview and editor mode;
7. default-content branches still return the base `*EmailTemplate` pointer (`assert.Same`).

Fixture rule for every test below: `domain.NewEmailSender` always sets `IsDefault: true`, so the
provider gets a default sender A and a second sender B with `IsDefault = false`, the base template
uses B, and the assertion is "From is B". Otherwise `GetSender("")` returns the template's sender
anyway and the test passes before the fix. New subtests use their own template value and a language
that is not already a fallback case (`de` is one in the existing tables), e.g. `it`.

### `internal/service/email_service.go`

Template reply-to applied only when `request.EmailOptions.ReplyTo` is empty.

Tests — `internal/service/email_service_test.go` (`SendEmailForTemplate`):

1. explicit `email_options.reply_to` + template reply-to → the explicit one is sent;
2. no explicit reply-to + template reply-to → the template's is sent;
3. translated contact, translation without sender / reply-to → From is the template's sender B and
   Reply-To the template's;
4. translated contact with an explicit `email_options.reply_to` → the explicit one is sent.

### Send paths (no code change, regression tests)

- `internal/service/broadcast/message_sender_test.go`, `TestSendToRecipient_LanguageResolution`: new
  subtest, translation without sender or reply-to → `req.FromAddress` is B and
  `req.EmailOptions.ReplyTo` the template's reply-to.
- `internal/service/broadcast/queue_message_sender_test.go`: same assertions on the entry built by
  `buildQueueEntry` (`Payload.FromAddress`, `Payload.EmailOptions.ReplyTo`).

### Integration — `tests/integration/template_translation_test.go`

A new `TestTemplateTranslationSenderAndReplyTo`, on a workspace set up with
`SetupWorkspaceWithSMTPProvider`, whose Mailpit SMTP integration gets, through an inline
`IntegrationOption`, a default sender A and a non-default sender B; the template uses B and a
reply-to, and its `fr` translation has neither:

- **Transactional**: send to an `fr` contact → Mailpit `From` is B, the raw message
  (`GetMailpitMessageRaw`) carries the template's `Reply-To`.
- **Transactional, explicit reply-to**: same send with `email_options.reply_to` → that one is sent.
- **Broadcast**: same From / Reply-To checks for an `fr` contact of the broadcast's list.
- **Automation**: same checks for the email node.

### Documentation

- `openapi/components/schemas/template.yaml`, `TemplateTranslation`: an email translation without
  `sender_id` or `reply_to` uses the default content's.
- `openapi/components/schemas/transactional.yaml`, `reply_to`: overrides the template's reply-to.
- `../docs` (separate PR, `docs/` branch to `dev`): `features/templates.mdx` (translations send from
  the template's sender with its reply-to) and `features/transactional-api.mdx` (`reply_to` overrides
  the template's).

## Known, not handled here

- Saving a template from the console rebuilds translations without `sender_id` / `reply_to`, so a
  per-language sender set through the API is wiped by a console save. With this fix the translation
  then inherits the base template's values, which is the console's intent.
- The automation email node's `from_override` / `subject_override` are declared but never read.

## Test commands

```bash
go test -race ./internal/domain -run 'TestTemplate_ResolveEmailContent'
go test -race ./internal/service/broadcast -run 'TestSendToRecipient_LanguageResolution'
go test -race ./internal/service -run 'TestEmailService_SendEmailForTemplate'
make test-domain
make test-service
INTEGRATION_TESTS=true go test -race -tags integration,licdev -timeout 20m ./tests/integration/ -run 'TestTemplateTranslation' -v
make test-unit
```

## Review

A counter-review of the first version of this plan raised six findings. All of them were checked in
the code and folded in above (reply-to override, empty-means-inherit, non-default sender fixtures,
language collision, mandatory queue test, automation coverage).
