# Plan — send a plain-text alternative with every email (issue #443)

Base: `dev` at `f075a7f` (version 43.0). Revision 2: integrates an independent spec review;
see "Review log" at the end.

## Need

Issue #443: Notifuse sends HTML-only emails. `smtp_service.go:622` calls
`msg.SetBodyString(mail.TypeTextHTML, request.Content)` and nothing else, and every other
provider only fills its HTML field. Emails should be `multipart/alternative` with a
`text/plain` part. Spam filters score HTML-only mail (SpamAssassin `MIME_HTML_ONLY`), and
text-only readers, watches and some screen readers show the text part.

## Decisions (taken by Pierre)

- The text version is reached from step **"2. Template"** of the template editor, as a third
  mode of the editor toolbar next to Edit and Preview (variant A, mock-ups 05 and 06). The
  settings-step section (old mock-ups 02/03) and the preview-switcher icon (07) are dropped.
- The template preview drawer gets a "Text version" tab (mock-up 01).

## Scope and delivery

Two PRs to `dev`, same unreleased version **43.1** (minor bump: no schema change).

| PR | Content |
|---|---|
| **PR 1 — automatic text** | HTML-to-text converter; every send carries a text part generated from its final HTML; 7 providers; `templates.compile` returns the text; editor "Text version" mode (read-only) in visual and code mode; preview-drawer tab; docs. |
| **PR 2 — custom text** | Per-template, per-language hand-written text with Liquid, stored in the existing `email.text`; Automatic/Custom choice and editing in the "Text version" mode; carried through every send path. |

PR 2 depends on PR 1. Each PR follows: tests first, then code, all tests green (unit +
integration), then the high review.

**No SQL schema change, no migration, no environment variable.** PR 1 stores nothing new.
PR 2 only adds JSON keys inside existing JSONB columns: `templates.email.text` (the Go field
`EmailTemplate.Text` already exists, unused) and `email_queue.payload.text_content`.

## Where the text is produced (PR 1)

Every send goes through `EmailService.SendEmail` (`internal/service/email_service.go:178`)
just before the provider is chosen: queue worker (`queue/worker.go:790`), broadcast direct
sender (`broadcast/message_sender.go:428`), send-to-individual (`broadcast_service.go:1335`),
transactional test (`transactional_service.go:1027`), `SendEmailForTemplate`
(`email_service.go:693`, used by transactional sends and double opt-in), and the provider
test (`email_service.go:167`).

So: `SendEmailProviderRequest` gets `TextContent string`; `SendEmail` sets
`TextContent = plaintext.FromHTML(request.Content)` when `strings.TrimSpace(TextContent)` is
empty. Providers send a text part only when `TextContent != ""`. The conversion runs **once
per email**, there: send-time compiles never convert (see step 5, `IncludeText`).

Consequences, deliberate:

- **Generated from the final HTML**, after `TrackLinks`. Text links are exactly the HTML
  links: tracked `/r/…` form when click tracking applies to the contact, direct (with UTM)
  otherwise. Per-contact tracking consent (43.0) holds without extra code, and a click from
  the text part is recorded.
- **Nothing stored**: the queue payload is unchanged in PR 1; entries enqueued before the
  upgrade get a text part when the worker sends them.
- The open pixel is an image: a text-only reader does not load it (same as a client that
  blocks images today).
- The footer unsubscribe link is in the text; `List-Unsubscribe` headers are unchanged.
- If conversion yields nothing (empty or unreadable HTML), the email goes out HTML-only, as
  today.

## PR 1 — implementation, test file and test cases for each file

TDD order: for each step, write the listed tests, see them fail, implement, see them pass.

### 1. Converter — `pkg/plaintext/plaintext.go` (new)

`func FromHTML(html string) string`, built on `golang.org/x/net/html` (already in `go.mod`;
no new dependency). Validated by a prototype
on a real compiled MJML template. Rules:

- skip `head`, `title`, `style`, `script`, `noscript`, `iframe`, `template`, `svg`, `math`,
  `object`, `input`, `select`, `textarea`, comments (incl. Outlook conditional comments), and
  elements hidden for every client: `style` with `display:none` or `visibility:hidden`, or the
  `hidden` attribute (MJML's preheader uses `display:none`). **Not** `mso-hide:all`: it hides in
  Outlook only, and bulletproof-button fallbacks carry it on the real link;
- drop invisible padding characters (U+200B, U+200C, U+034F, U+00AD, U+FEFF), turn U+00A0
  into a space, collapse whitespace; entities are decoded by the parser;
- block elements (`p, div, table, tr, td, th, h1–h6, ul, ol, li, blockquote, section, center, pre`)
  start and end lines; `p`, headings, `table`, `tr` end with a blank line; never more than one
  blank line in a row; lines trimmed; output ends with one `\n`;
- `br` → line break; `hr` → a line of `-` between blank lines;
- `a`: `label (href)`; label empty → `href`; label equal to href, or label that is itself an
  absolute URL (a tracked link whose text is the destination) → `href` only. **A URL is always
  followed by whitespace or end of line**: the converter inserts a space after a link when the
  next output would otherwise touch it (adjacent anchors such as `mj-navbar` would otherwise
  glue `https://e.com/n1Nav2` and break tracked `/r/<token>` links). `mailto:` → the address
  without its `?query`, or `label (address)` when they differ; `tel:` same; any other non-http(s)
  scheme (`sms:`, custom) → label only; `#…`, empty or `javascript:` href → label only; an
  image inside a link contributes its `alt` as label, and a linked image without `alt` gives
  the bare `href`;
- `img` outside a link → its `alt`, nothing when empty;
- `ul > li` → `- item`; `ol > li` → `1. item` numbered; nested lists indented two spaces;
- `blockquote` → each line prefixed `> `;
- `pre` → whitespace preserved;
- never panics; output is valid UTF-8 (invalid input bytes replaced);
- **nothing but whitespace → `""`** (image-only HTML must not yield `"\n"`).

Line endings stay `\n`; the transports already emit CRLF (go-mail, `quotedprintable.Writer`).
No hard wrapping (quoted-printable handles long lines).

**Tests — `pkg/plaintext/plaintext_test.go`**, table-driven, one case per rule above:
empty and whitespace-only input → `""`; image-only HTML without alt → `""`; each skipped
element (`noscript`, `iframe`, `svg`…) contributes nothing; `display:none`, `visibility:hidden`
and `hidden` skipped; **`<a style="mso-hide:all" href=…>Buy</a>` keeps `Buy (url)`**; hidden
preheader with `&zwnj;&#847;&nbsp;` padding skipped; **three adjacent anchors (`mj-navbar`
output) → every URL followed by whitespace**; `mailto:a@b.c?subject=x` → `a@b.c`; `sms:` → label; two paragraphs → one blank line between; three
consecutive empty blocks → still one blank line; `br`; headings; link label+href; label ==
href; URL label with different (tracked) href → href only; mailto same/different label; tel;
`#`, empty and `javascript:` hrefs; linked image with alt; linked image without alt → href;
standalone image with and without alt; `ul`; `ol` numbering; nested list indentation;
blockquote; pre preserves spaces and newlines; table with two cells → two lines; `&amp;` →
`&`; NBSP → space; zero-width chars removed; Outlook conditional comment content skipped;
invalid UTF-8 input → valid UTF-8 output.
Plus `FuzzFromHTML` (properties: no panic, valid UTF-8, a marker the fuzzer places inside
`<script>` never appears in the output; `&lt;b&gt;` legitimately yields `<b>`, so "no `<`" is
not a property) and `BenchmarkFromHTML` on a ~100 KB fixture with real `http` links
(`pkg/plaintext/testdata/newsletter-links.html`).

### 2. Domain — `internal/domain/email_provider.go`

Add `TextContent string` to `SendEmailProviderRequest`, with a comment: optional; empty means
"derive from Content". `Validate` unchanged (text is optional).

**Tests — `internal/domain/email_provider_test.go`**: `Validate` accepts an empty
`TextContent` and a non-empty one (no new error paths).

### 3. Choke point — `internal/service/email_service.go` `SendEmail`

After `Validate`, before choosing the provider:
`if request.TextContent == "" { request.TextContent = plaintext.FromHTML(request.Content) }`.

**Tests — `internal/service/email_service_test.go`** (provider mock captures the request):
empty `TextContent` → provider receives the converted text of `Content`; whitespace-only
`TextContent` → converted text too; non-empty
`TextContent` → passed through untouched (PR 2 relies on it); `Content` that converts to
nothing → provider receives `TextContent == ""`; demo mode still returns before anything.

### 4. Providers — one test file each, written first

Shared case list per provider: (a) with `TextContent` → text part sent with the exact
expected shape; (b) with `TextContent == ""` → request byte-identical to today (HTML only);
(c) with attachments (where the provider has a separate attachment path) → text still sent.

| Provider | File | Change | Test file and specific assertions |
|---|---|---|---|
| SMTP | `smtp_service.go:623` | `msg.SetBodyString(mail.TypeTextPlain, text)` then `msg.AddAlternativeString(mail.TypeTextHTML, html)` (both exist in go-mail 0.7.2; HTML last = preferred); text empty → current single call | `smtp_service_test.go` via `mockSMTPServer`: parse captured DATA with `net/mail` + `mime/multipart`: top `multipart/alternative`, part 1 `text/plain; charset=UTF-8`, part 2 `text/html`; with inline image + attachment: `mixed > related > alternative{text, html}` + embed + attachment, Content-ID kept; text empty → no `multipart/alternative` |
| SES simple | `ses_service.go:1361` | `Body.Text` next to `Body.Html` when text non-empty | `ses_service_test.go`: captured `SendEmailInput.Content.Simple.Body.Text.Data` equals text, charset UTF-8; nil when empty |
| SES raw MIME | `ses_service.go:1518–1640` | new `writeBodyPart(w)`: when text non-empty writes `multipart/alternative{ text/plain (QP), html-or-related }`; inline images stay in `related` with the HTML: `mixed{ alternative{ text, related{ html, inline… } }, attachments… }`; text empty → today's tree exactly. This path carries most marketing sends (taken whenever `List-Unsubscribe` is set; notice broadcasts that keep unsubscribed contacts have no header, `internal/domain/broadcast.go:331-333`, and take the simple path) | `ses_service_test.go`: parse raw bytes with `mime/multipart` recursively and assert the full tree for: List-Unsubscribe only; inline only; inline + attachment; attachment only; each with and without text. Text part `Content-Transfer-Encoding: quoted-printable`, decodes to the input. Update `TestSendEmail_InlineAttachment_MultipartRelatedAndContentID` and `TestSendEmail_VerifyMIMEStructureWithAttachments` assertions only where the tree legitimately changes |
| SendGrid | `sendgrid_service.go:331` | prepend `{Type:"text/plain"}` when non-empty (SendGrid requires `text/plain` before `text/html`) | `sendgrid_service_test.go` (HTTP client mock): JSON `content[0].type == "text/plain"`, `content[1].type == "text/html"`; one element when empty |
| Mailgun | `mailgun_service.go:782` (form) and `:860` (multipart) | `text` field in both paths when non-empty | `mailgun_service_test.go`: decoded form has `text`; multipart body has a `text` field; absent when empty |
| Postmark | `postmark_service.go:552` | `"TextBody"` when non-empty | `postmark_service_test.go`: JSON `TextBody`; key absent when empty |
| Mailjet | `mailjet_service.go:546` | fill the existing `TextPart` (`omitempty`) | `mailjet_service_test.go`: JSON `Messages[0].TextPart`; absent when empty |
| SparkPost | `sparkpost_service.go:816` | `Text string json:"text,omitempty"` in `Content` | `sparkpost_service_test.go`: JSON `content.text`; absent when empty |

### 5. Compile response — `pkg/notifuse_mjml/template_compilation.go`

`CompileTemplateRequest` gets `IncludeText bool json:"-"`; `CompileTemplateResponse` gets
`Text *string json:"text,omitempty"`. When `IncludeText` is true, the channel is **not
`"web"`** (no send path sets `Channel`, so the test is `!= "web"`, never `== "email"`) and
compilation succeeded: `Text = plaintext.FromHTML(trackedHTML)` (the same HTML a send uses,
so the preview shows what goes out). Otherwise `Text` is nil. Only the preview path sets
`IncludeText`: `TemplateHandler.handleCompile` sets it on the decoded request before calling
the service. Send-time compiles leave it false, so a broadcast converts once (in `SendEmail`),
not twice. (`pkg/notifuse_mjml` importing `pkg/plaintext` creates no cycle.)

**Tests — `pkg/notifuse_mjml/template_compilation_test.go`**: `IncludeText` false → `Text ==
nil` (send path); `IncludeText` with empty `Channel` → `Text` set; visual-mode compile returns
`Text` with the rendered greeting and the button `label (url)`; code-mode compile returns
`Text`; tracking enabled → every URL in `Text` is an `href` of the HTML (extract both sets and
compare); tracking off → direct links; `Channel == "web"` → nil; MJML error → nil; hidden
preheader not in `Text`. Golden test with **tracking off** (tracked tokens embed `time.Now()`):
`pkg/notifuse_mjml/testdata/plain-text-golden.json` (new folder; a tree with heading, text,
button, navbar, list, social and footer blocks and real `http` links — the console's
`newsletter.json` only has `href="#"`) compiled with fixed data → `plain-text-golden.txt`.
`BenchmarkCompileTemplate` with and without `IncludeText`.

`internal/http/template_handler.go`: set `req.IncludeText = true` in `handleCompile`.
**Test — `internal/http/template_handler_test.go`**: the service receives `IncludeText ==
true`; `/api/templates.compile` JSON contains a non-empty `text` for an email compile; a
client sending `"include_text": false` cannot switch it off (field is `json:"-"`).

`openapi/components/schemas/template.yaml`: document `text` on the compile response, then
`make openapi-bundle` (`openapi.json` is committed).

### 6. Console — "Text version" mode (read-only in PR 1)

- `console/src/services/api/template.ts`: `CompileTemplateResponse.text?: string`.
- New `console/src/components/email_builder/panels/TextVersionPanel.tsx`: left panel in the
  frame of "Content structure" (`w-80 bg-gray-50 border-r`), explanation text; centre: the
  text on a white page over the checkered canvas, monospace (mock-up 05, without the
  Automatic/Custom choice in PR 1), "Preview with test data" caption.
- `EmailBuilder.tsx`: optional prop `textVersion?: { enabled: true }` (PR 2 extends it). When
  present, the toolbar `Segmented` gets a third option "Text version"; selecting it calls
  `onCompile` like Preview does and renders `TextVersionPanel` with `result.text`. `onCompile`'s
  return type gains `text?: string`. `viewMode` state (`:106`) and `forcedViewMode` types
  widened to include `'text'`. While touching the toolbar, wrap its hard-coded labels in `t`
  (`'Edit'`, `'Preview'`, the Undo/Redo titles at `:816`/`:824`), so "Version texte" does not sit
  next to English labels. Other untranslated strings of the builder stay out of scope.
- `MjmlCodeEditor.tsx`: a "Text version" tab next to "Generated HTML" (`:693`) showing the
  compiled text; `onCompile` return gains `text`.
- `CreateTemplateDrawer.tsx` (`:1065-1069`, `:1131-1135`) and `TemplateTranslationsTab.tsx`
  (`:142`, `:171-175`): their four hand-built compile results pass `text: response.text`; both
  pass `textVersion={{ enabled: true }}`.
- `TemplatePreviewDrawer.tsx`: "Text version" tab between "HTML Preview" and "MJML Source",
  rendered like `MJMLPreview` (mock-up 01), with the one-line caption about tracked links.
- `npm run lingui:extract`; translate new strings in the 8 `.po` files.

**Tests (Vitest)**:
- `email_builder/EmailBuilder.test.tsx`: no third option without `textVersion`; with it,
  selecting "Text version" calls `onCompile` (the builder compiles on mode change and on an
  effect, `:208-229`, so the test asserts "called" and the text shown, not a call count);
  Undo/Redo hidden in that mode; compile error → error state, no stale text.
- `email_builder/MjmlCodeEditor.test.tsx` (new; none exists today; mock `@monaco-editor/react`):
  tab present, shows `text`.
- `templates/CreateTemplateDrawer.test.tsx` and `templates/TemplateTranslationsTab.test.tsx`:
  the compile results handed to the builder carry `text` from the API response.
- `email_builder/panels/TextVersionPanel.test.tsx`: renders lines verbatim (whitespace kept),
  empty text → "No text version: this email will be sent as HTML only".
- `templates/TemplatePreviewDrawer.test.tsx`: tab shows `text` from the compile response; no
  tab when the response has no text.
- `src/__tests__/i18nMacro.test.ts` and `src/i18n/locales/catalogues.test.ts` stay green.

### 7. Integration — `tests/integration/plain_text_alternative_e2e_test.go` (new, Mailpit)

Use `GetMailpitMessageRaw` and parse MIME ourselves: Mailpit's `Text` field may be derived by
Mailpit from the HTML, so it proves nothing.
- Transactional send through a Mailpit SMTP integration → raw message is
  `multipart/alternative`, text part contains the Liquid-rendered greeting and the button
  URL, HTML part unchanged.
- Broadcast to one contact through the queue worker → same, plus `List-Unsubscribe` still set.
- Contact who refused click tracking → text part links are direct (no `/r/`).
- In every case above: the set of URLs in the text part equals the set of `href`s in the HTML
  part (minus `#`, `javascript:` and non-http ones), which catches glued URLs end to end.
- `POST /api/templates.compile` through the real server returns `text`.

### 8. Docs, changelog, version

- `config/config.go`: `VERSION = "43.1"`, then `make sdk-build` on Node 24 and commit
  `web_analytics_sdk/dist/` (the bundle embeds VERSION and CI diffs it byte for byte,
  `Makefile:68-72`); `CHANGELOG.md`: `## [43.1] - WIP`, one feature line:
  "Every email now includes a plain-text version, generated from its HTML, for mail clients
  that show text only and for better deliverability. Its links follow the same tracking as
  the HTML. Preview it in the template editor's new Text version mode."
- Docs PR on `Notifuse/docs` (to `dev`): `features/templates.mdx` "Plain-text version"
  section; `openapi.json` compile response `text`.

### 9. Run (PR 1)

`make test-pkg`, `make test-domain`, `make test-service`,
`go test -race ./internal/service/queue` (not in `test-service`), `make test-http`,
`cd console && npm test`, `make test-integration` (Mailpit + Postgres via
`tests/compose.test.yaml`), then `make test-unit` once.

## PR 2 — custom text

### Data and rules

- `EmailTemplate.Text *string` (exists, `json:"text,omitempty"`, and **already writable and
  documented in the API** as "Plain text version of the email",
  `openapi/components/schemas/template.yaml:148-151`, but ignored by every send) is the custom
  text of the default language. Values already stored through the API start being sent at
  upgrade: the changelog says so; `Translations[lang].Email.Text` the custom text of a language. nil or
  whitespace-only = Automatic. A language without its own custom text gets the automatic
  version of its own HTML (it does not inherit the default language's custom text, which is
  in another language).
- Liquid rendered with the same data as the body, via the same function as the subject
  (`renderSubjectField` → `ProcessLiquidTemplate`, no HTML escaping).
- **Liquid syntax is validated on the server at save**: `EmailTemplate.Validate` parses `Text`
  with a new `notifuse_mjml.ValidateLiquidSyntax(content string) error` using the same parse
  options as `liquid_secure.go:96` (the console's `validateLiquidTags` only balances braces,
  and nothing compiles at save). Translations go through the same `Validate`.
- **At send, a Liquid render error does not fail the email**: the text falls back to the
  automatic version (warning logged with template id and version). One bad text must not fail
  every recipient of a broadcast (`queue_message_sender.go:239-249` skips a recipient whose
  entry fails to build). Values stored before the upgrade are only validated at their next
  save, which is why the fallback is needed.
- Normalisation at save: line endings `\r\n` and bare `\r` → `\n`; whitespace-only → nil.
  `authoredTranslation` also maps a blank `Text` to nil, so `""` vs nil never reads as
  "translation touched" for the licence gate (`template.go:131-163`,
  `template_service.go:403-407`).
- Links in a custom text are **not** rewritten: no click tracking, no UTM. Stated in the UI
  (mock-up 06). Keeping `{{ unsubscribe_url }}` is the author's job; the UI reminds it and the
  save shows a warning (not an error) when a marketing template's custom text lacks it, and
  likewise for `{{ confirm_subscription_url }}` in an opt-in template.
- Size cap: 64 KiB per text (`EmailTemplate.Validate`).

### Files, tests first

1. **`internal/domain/template.go`**
   - `Validate`: `Text` over 64 KiB → error; Liquid syntax error → error naming the text
     version; normalisation above. `ApplyToCompileRequest`: `req.Text = e.Text`.
   - `authoredTranslation`: blank `Text` → nil (licence gate); `Text` stays authored.
   - Tests — `template_test.go`: 64 KiB + 1 rejected, 64 KiB accepted, nil accepted;
     `{% if x %}Hello` rejected; `\r\n` normalised; whitespace-only → nil;
     `ApplyToCompileRequest` copies `Text` (and nil); `TranslationsWiden` true when only a
     translation's `Text` changed, **false** when it goes from nil to `""`.
   - Tests — `pkg/notifuse_mjml/liquid_secure_test.go`: `ValidateLiquidSyntax` accepts valid
     tags and filters, rejects unclosed blocks.
2. **`pkg/notifuse_mjml/template_compilation.go`**: `CompileTemplateRequest.Text *string`;
   response `TextSource string json:"text_source,omitempty"` (`"auto"` | `"custom"`). Custom
   non-blank → render Liquid (always, also when `IncludeText` is false: sends need it),
   `Text` = rendered, `TextSource = "custom"`. Render error → no compile failure: with
   `IncludeText`, `Text` = automatic and `TextSource = "auto"`; without it, `Text` stays nil and
   `SendEmail` converts. Blank custom → as PR 1. `PreserveLiquid` → raw text.
   - Tests: custom rendered with data; Liquid render error → `Success=true`, HTML intact,
     `TextSource "auto"`; blank custom → auto; `PreserveLiquid` → raw; custom links untouched
     with tracking enabled; web channel → no text; custom with `IncludeText` false → still
     rendered.
3. **`internal/domain/email_queue.go`**: `EmailQueuePayload.TextContent string
   json:"text_content,omitempty"`; `ToSendEmailProviderRequest` copies it.
   - Tests — `email_queue_test.go`: copied; JSON omits it when empty; an old payload without
     the key unmarshals with `""`.
4. **Six send sites** — set the text only when custom, so automatic text stays generated at
   send time from the final HTML:
   `if resp.TextSource == "custom" && resp.Text != nil { …TextContent = *resp.Text }`
   in `broadcast/queue_message_sender.go` (payload), `automation_node_executor.go` (payload),
   `broadcast/message_sender.go`, `broadcast_service.go` `SendToIndividual`,
   `transactional_service.go` `TestTemplate`, `email_service.go` `SendEmailForTemplate`.
   A small helper `domain.CustomTextOf(resp) string` keeps the sites identical; it has its
   own tests in `internal/domain` (nil response, auto, custom, nil `Text`).
   - Tests in each site's existing test file: custom template → request/payload carries the
     rendered text; automatic template → `TextContent == ""`; translation with its own text
     → that text; translation without → `""`.
5. **`queue/worker.go`**: no code change; test in `worker_test.go` that a payload with
   `text_content` reaches `SendEmail` with it.
6. **Console**
   - `template.ts`: `CompileTemplateRequest.text?: string`, response `text_source?`.
   - `TextVersionPanel`: Automatic / Custom radio cards (mock-ups 05/06); Custom → editable
     monospace textarea, `validateLiquidTags` on change (the server is the real check),
     "Start again from the automatic version", warnings about unsubscribe link and
     translations; Automatic → read-only preview.
   - **"Start again from the automatic version" must not seed rendered, tracked text** (it would
     store the test contact's data and preview tracking tokens): it runs a dedicated compile
     with `preserve_liquid: true` and tracking off, so the seed keeps `{{ }}` and direct URLs.
   - `EmailBuilder` / `MjmlCodeEditor`: `textVersion` prop gains `value?: string` and
     `onChange(value?: string)`; switching to Automatic calls `onChange(undefined)`.
   - `CreateTemplateDrawer`: state `emailText`, sent as `email.text` only when custom; compile
     calls send `text`. `TemplateTranslationsTab`: `TranslationEditorState.text?`, sent per
     language. `TemplatePreviewDrawer`: sends
     `effectiveEmail?.text` (the language-resolved content, as the subject already does,
     `TemplatePreviewDrawer.tsx:67-71`, `:101-102`), shows the Automatic/Custom tag from
     `text_source` (mock-ups 01/04).
   - Tests: radio switch calls `onChange`; restart button calls compile with
     `preserve_liquid: true` and tracking off and fills the textarea with its text; a server
     validation error on save is shown on the Text version mode; submit payload has
     `email.text` only when custom, and per-language `text`; state seeding from an existing
     template and translation (`CreateTemplateDrawer.tsx:375-392`,
     `TemplateTranslationsTab.tsx:86-104`, `:253-262`); preview drawer sends the translation's
     text and its tag follows `text_source`.
7. **Integration** — extend `plain_text_alternative_e2e_test.go`: custom text with Liquid →
   raw text part equals the rendered text; `fr` translation with its own text → that text for
   a `fr` contact; `de` translation without → automatic text of the `de` HTML; template API
   round-trip (`template_api_test.go`): create/update/get keeps `email.text` and translation
   texts, update without `text` returns to Automatic.
8. **Docs / changelog**: fold into the 43.1 feature line ("…or write your own, per language,
   in the same mode"), plus one line for API users: "If you set `email.text` through the API,
   that text is now sent as the plain-text version." Docs section and `openapi`
   (`email.text` description, compile `text_source`), `make openapi-bundle`.
9. **Run**: same commands as PR 1.

## Risks

| Risk | Mitigation |
|---|---|
| Poor text on unusual HTML (raw blocks, code mode, product tables) | rule-per-case unit tests, golden newsletter, fuzz; empty result → HTML-only |
| SES raw MIME tree (marketing path) | full-tree assertions for every attachment/inline combination, with and without text |
| SendGrid content order | exact-order assertion |
| CPU per send in broadcasts | one conversion per email (send time only; `IncludeText` keeps send-time compiles free of it); `BenchmarkFromHTML` and `BenchmarkCompileTemplate` committed; the review measured about 1.5 ms per conversion against 8.3 ms per compile on `newsletter.json` |
| Existing tests that pin MIME/body shape | update only where the tree legitimately changes; "text empty" cases prove the old shape is intact |
| Provider-side link tracking (SendGrid, Mailgun, SparkPost, Postmark) rewrites text links too when enabled at the provider | same behaviour as for HTML; documented |

## Known, not handled (out of scope)

- Import/Export (JSON/MJML) does not carry the custom text.
- Custom-text links are neither tracked nor given UTM parameters.
- No workspace switch to turn the text part off (no request for it; best practice is to always send it).

## Rules for tests and comments

No count in a test name or comment unless an assertion pins it (repository rules): no
"7 providers" or "six send sites" in test names or code comments.

## Review log

Revision 2 (2026-11-10), independent spec review. The five blocking findings
and the suggestions were checked against the code and integrated:

| Finding | Change in this plan |
|---|---|
| B1 URLs glued by adjacent anchors (`mj-navbar`) | link format `label (url)`, guaranteed whitespace after a URL; navbar case; URL-set equality in compile and integration tests |
| B2 `mso-hide:all` treated as hidden | hidden = `display:none`, `visibility:hidden`, `hidden`; bulletproof-button case |
| B3 restart seeds rendered, tracked text | dedicated compile with `preserve_liquid: true` and tracking off |
| B4 preview drawer would send the default language's text | `effectiveEmail?.text` |
| B5 no real Liquid check at save; stored API values go live | server-side `ValidateLiquidSyntax` in `Validate`; send-time fallback to automatic; changelog line for API users |
| S1 double conversion per email | `IncludeText` (`json:"-"`) set by the compile handler only; benchmarks committed |
| S2 incomplete skip list, fuzz property | skip list extended; fuzz properties redefined |
| S3 blank must be `""` | `TrimSpace` in `SendEmail`; image-only case |
| S4 link details, golden fixture | mailto query, non-http schemes, linked image without alt; dedicated golden with real links, tracking off |
| S5 `Channel` is empty on send paths | rule is `!= "web"`; empty-Channel test |
| S6 nil vs `""` breaks the licence gate | blank → nil in `authoredTranslation` and at save |
| S7 version bump needs SDK rebuild and OpenAPI bundle | `make sdk-build` + commit `dist/`; `make openapi-bundle` |
| S8 missing tests | compile-result objects, `CustomTextOf`, `templates.compile` integration, PR 2 state seeding |
| S9 EmailBuilder details | no call-count assertion; `viewMode` typing; toolbar strings named precisely |
| S10 wrong references | line numbers, SES wording, `catalogues.test.ts` path, queue tests command, Monaco mock |
| S11 line endings | normalised at save |
| S12 opt-in confirmation link | same warning as unsubscribe |
| S13 counts in names | rule added above |

## Implementation notes (PR 1)

- The visual editor has no navbar block, so the golden fixture builds its navigation row
  as adjacent anchors in an `mj-text`, which is what the navbar compiles to.
- Punctuation right after a link (`.,;:!?)]` and ASCII quotes) stays attached when a
  space or the end of the text follows: `label (url).`, not `label (url) .`. Autolinkers
  leave it out of the link. Typographic closing quotes are kept apart, since autolinkers
  take them into the link.
- A link with a relative `href` keeps its label only: there is no base to resolve it
  against. Tracked links are absolute whenever `API_ENDPOINT` or a custom endpoint is set.
