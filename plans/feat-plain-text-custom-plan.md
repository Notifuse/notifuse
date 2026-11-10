# Plan — custom plain-text version per template and language (issue #443, PR 2)

Base: `dev` at `dfa4a6d` (PR 1, #504, merged: every email carries an automatic text version;
unreleased version **43.2**). Parent plan: `plans/feat-plain-text-alternative-plan.md`, whose
"PR 2 — custom text" section this plan replaces, checked against the code PR 1 shipped.
Revision 2: integrates the independent plan review (see "Review log").

## Need

PR 1 generates the text version from the HTML at every send. Some senders want to write it
themselves (a receipt, a shorter text). The API already has a field for it:
`EmailTemplate.Text` (`internal/domain/template.go:401`, `json:"text,omitempty"`), documented
as "Plain text version of the email" (`openapi/components/schemas/template.yaml`) and stored,
but no send reads it.

## Decisions (taken by Pierre, mock-ups `maquettes/issue-443/05`, `06`, `01`, `04`)

- The "Text version" mode of the editor (PR 1) gets two cards, **Automatic** and **Custom**.
  Custom shows an editable monospace text area with the text as written (Liquid not
  rendered), a "Start again from the automatic version" button, a warning box, and a note
  that each translation has its own text in the Translations tab, "otherwise it receives the
  automatic version" (mock-up 06).
- The preview drawer's Text version tab shows the rendered text with an Automatic / Custom
  tag.

## Scope

One PR to `dev`. **No SQL schema change, no migration, no environment variable**: new JSON
keys only, inside existing JSONB columns (`templates.email.text`, translation
`email.text`, `email_queue.payload.text_content`). Version stays **43.2** while unreleased
(if 43.2 ships first: 43.3, SDK rebuild, OpenAPI bundle). Also aligns
`web_analytics_sdk/package-lock.json` (still `42.3.0`) on `43.2.0`.

## Data and rules

1. **Storage.** `Template.Email.Text` is the default language's custom text;
   `Translations[lang].Email.Text` a language's. nil means Automatic.
2. **No inheritance.** A language without its own custom text gets the automatic version of
   its own HTML, never the default language's custom text. Enabling or resetting a
   translation in the console leaves its `text` unset (Automatic), as mock-up 06 says.
3. **One normaliser**, `domain.NormalizeCustomText(*string) *string`: `\r\n` and bare `\r` →
   `\n`; whitespace-only → nil. Used by `EmailTemplate.Validate` (save, also each translation
   through `validateTranslations`, `template.go:180`), by `authoredTranslation` (so nil, `""`,
   whitespace and line-ending differences never read as "translation touched" for the
   licence gate, `TranslationsWiden`, `template.go:131`; `template_service.go:405`), and at
   render (stored values from before this PR are not normalised until their next save).
4. **Validation at save**: source over 64 KiB → error; Liquid syntax checked by
   `notifuse_mjml.ValidateLiquidSyntax(content string) error` (parse with the same
   environment options as `SecureLiquidEngine`, `liquid_secure.go:96`, after the same
   `cleanLiquidTemplate`, `converter.go:410`, with the same `recover`, a fresh or pooled
   environment); `{% include %}` and `{% render %}` refused (they parse, then write an
   inline "Liquid error" at render). Error text: `invalid email template: text version: …`.
   Validate runs on create/update only (`template_service.go:244`, `:428`, request
   validators `template.go:663`, `:817`), never at send.
5. **Rendering**: same rules as the subject (`renderSubjectField` → `ProcessLiquidTemplate`,
   `template_compilation.go:697`). No HTML escaping. Runtime errors are lax, as for the body.
   Parse errors and the 100 KB input limit return an error; the 10 MB render-length cap
   writes an inline message (`liquid_secure.go:16-24`) and does not.
6. **Fallback to automatic**, never a failed email, when the custom render: returns an
   error, renders blank (e.g. a whole-text `{% if %}` false for this contact), or exceeds
   256 KiB rendered. The compile then sets `text_error` (blank: no error, just automatic) and
   the email gets the automatic text generated in `SendEmail`. One stored value saved before
   this PR (never validated) must not fail every recipient of a broadcast.
7. **Links in a custom text are written as typed**: no click tracking, no UTM.
8. **Warning box by category** (departs from mock-up 06, whose order notification would
   render `Se désabonner : ` empty: `unsubscribe_url` exists only with a list,
   `template.go:1079-1099`, and transactional sends have none,
   `transactional_service.go:726-728`):
   - always: "Links in a custom text are not tracked." and "This text is not updated when
     you change the email.";
   - `marketing`: keep `{{ unsubscribe_url }}`; `opt_in`: keep
     `{{ confirm_subscription_url }}`; others: nothing more.
   - Save-time warning (non-blocking `message.warning`) for the same two cases when a custom
     text (default or a translation) lacks the variable. The category is the form value or
     `forceCategory`.
9. **Seed** ("Start again from the automatic version", and switching to Custom with an empty
   area): compile of the current tree or source with `preserve_liquid: true`, no
   `test_data`, tracking off, `include_text`; the response text fills the area.
   - The converter writes an `href` starting with `{{` or `{%` as a destination, always
     (`plaintext.go:346-376`): after a real send's Liquid pass such an href can only be a
     literal, so sends do not change, and the seed keeps `{{ unsubscribe_url }}` links.
   - **Limit, stated in the UI and the docs**: conditions and loops written between blocks
     (code mode, `mj-liquid`) are not carried over (gomjml drops text between elements; a
     block-level `<` comparison makes the preserve-Liquid compile fail). When the compiled
     MJML returned with the seed holds `{%`, the panel shows a notice asking to check
     conditions and loops. The server also runs the seed text through
     `ValidateLiquidSyntax` and returns `text_error` when a pair was split.
   - Seed failure: Custom stays selected, the area stays as it was (empty if new), the error
     is shown. The area is disabled while a seed runs.

## Implementation, tests first

Each step lists its test file and cases; tests are written and seen failing first.

### 1. Converter — `pkg/plaintext/plaintext.go`

- In `link`: an href whose trimmed value starts with `{{` or `{%` takes the http(s) branch.
- Tests — `plaintext_test.go`: `{{ unsubscribe_url }}` with label → `Unsubscribe
  ({{ unsubscribe_url }})`; `{{url}}` without spaces; `{{ workspace.base_url }}/x`; href equal
  to its label → once; a linked image with a Liquid href → `alt ({{ … }})`;
  `https://e.com/?id={{ contact.id }}` unchanged; `{% if %}` text kept.

### 2. Liquid syntax — `pkg/notifuse_mjml/liquid_secure.go`

- `ValidateLiquidSyntax(content string) error` (rule 4).
- Tests — `liquid_secure_test.go`: valid tags, filters, `{% raw %}` accepted; plain text and
  `{"a":{"b":1}}` accepted; unclosed `{% if %}`, unknown tag rejected; `{% include 'a' %}`
  and `{% render 'a' %}` rejected; an input that only parses once cleaned
  (`{% i&#8203;f x %}a{% endif %}` with a zero-width space, raw: unknown tag) accepted.

### 3. Domain — `internal/domain/template.go`

- `NormalizeCustomText`; `EmailTemplate.Validate` (rules 3, 4); `authoredTranslation` uses
  the normaliser; `ApplyToCompileRequest`: `req.Text = e.Text`.
- Tests — `template_test.go`: 64 KiB accepted, 64 KiB + 1 rejected; nil accepted;
  `{% if x %}Hello` rejected with "text version"; a translation's invalid text rejected with
  its language; `\r\n`, `\r` normalised; whitespace-only → nil; `ApplyToCompileRequest`
  copies `Text` and nil; `TranslationsWiden` true when only a translation's `Text` changes,
  false for nil → `""`, nil → whitespace, `"a\r\nb"` vs `"a\nb"`, `"a\rb"` vs `"a\nb"`.

### 4. Compile — `pkg/notifuse_mjml/template_compilation.go`

- Request `Text *string json:"text,omitempty"` (source); response `TextSource string
  json:"text_source,omitempty"` (`auto` | `custom`, constants) and `TextError string
  json:"text_error,omitempty"`; method `(*CompileTemplateResponse) CustomText() string`
  (rendered text when `TextSource == custom`, else `""`; nil-safe).
- Email success path, after tracking: custom (normalised) non-nil and not web → render
  (raw when `PreserveLiquid` or no data; with `PreserveLiquid` also `ValidateLiquidSyntax`).
  OK and not blank and ≤ 256 KiB → `Text`, `TextSource = custom` (also without
  `IncludeText`). Otherwise rule 6: `TextError` set (except blank), and with `IncludeText`
  the automatic text with `TextSource = auto`. No custom → PR 1 behaviour, plus
  `TextSource = auto` when a text is returned. The seed's automatic text with
  `PreserveLiquid` is also validated (rule 9) and sets `TextError` on a split pair.
- Tests — `template_compilation_plaintext_test.go`: custom rendered with data; rendered
  without `IncludeText`; parse error → `Success`, HTML intact, `TextError`, automatic with
  `IncludeText`, `Text` nil without; blank render → automatic, no error; over the rendered
  cap → automatic and `TextError`; `PreserveLiquid` + custom → raw; `PreserveLiquid`
  automatic keeps `{{ unsubscribe_url }}` links; the four seed rows of the review (code-mode
  `{% for %}` around sections, if/else around sections, visual `mj-liquid` loop, block-level
  `<` comparison) pinned as the documented behaviour; split pair → `TextError`; custom links
  untouched with tracking on; web → no text; HTML byte-identical with and without `Text`
  **with tracking off** (tokens carry the time).
- `CustomText()` tests: nil response, auto, custom, custom with nil `Text`.

### 5. Queue payload — `internal/domain/email_queue.go`

- `EmailQueuePayload.TextContent string json:"text_content,omitempty"`;
  `ToSendEmailProviderRequest` copies it.
- Tests — `email_queue_test.go`: copied; omitted when empty; an old payload without the key
  gives `""`.

### 6. Send sites — custom only; automatic stays generated in `SendEmail`

Each site sets `TextContent = resp.CustomText()` and logs a warning (template id, version,
`TextError`) when `TextError` is set. The broadcast queue sender logs it once per template
version (workspace, template, version), not per recipient; the other sites send one email
per call and log per call.

| Site | Field | Compile |
|---|---|---|
| `broadcast/queue_message_sender.go:371` | payload | direct |
| `automation_node_executor.go:347` | payload | direct |
| `broadcast/message_sender.go:343` | request | direct |
| `broadcast_service.go:1294` `SendToIndividual` | request | mocked service |
| `transactional_service.go:997` `TestTemplate` | request | mocked service |
| `email_service.go:552` `SendEmailForTemplate` (transactional, double opt-in) | request | mocked service |

- Tests in each site's test file. Direct compile: custom → payload/request carries the
  rendered text; automatic → `""`; translation with its own text → that text; translation
  without → `""` while the default has one. Mocked compile: assert the compile **request**
  (`req.Text` = translation's value, nil when it has none while the default has one, as
  `email_service_test.go:1485-1491` does for `SubjectPreviewOverride`), and that a canned
  custom response reaches `TextContent`.
- `queue/worker_test.go`: payload `text_content` reaches `SendEmail` as `TextContent`.

### 7. HTTP / OpenAPI

- `templates.compile` request `text` ("source of a custom text, with Liquid"), response
  `text` description updated ("custom text rendered, or automatic text generated from the
  HTML"), `text_source`, `text_error`; `email.text`: "Custom plain-text version, with Liquid,
  up to 64 KiB. Omit for the automatic version. Links are not tracked." `make openapi-bundle`,
  `make openapi-lint`.
- `template_handler_test.go`: compile with `text` returns `text_source: custom`; invalid
  `text` → success with `text_error`.

### 8. Console

- `services/api/template.ts`: request `text?`, `include_text` not needed (the handler sets
  it); response `text_source?`, `text_error?`.
- `TextVersionPanel.tsx`: props `text`, `errors` (PR 1), plus optional `custom?: { value?:
  string; onChange: (v?: string) => void; onSeed: () => Promise<{ text?: string; error?:
  string; hasLiquidBlocks?: boolean }>; category?: string; isTranslation?: boolean;
  disabled?: boolean }` and `compact?: boolean` (code editor pane). Without `custom`: PR 1
  read-only panel. With it: the two cards; Automatic → PR 1 preview; Custom → textarea that
  never depends on the compile result (editable while the HTML compile is pending or
  failed), warning box by category (rule 8), translation note (default: "Each translation
  has its own text in the Translations tab, otherwise it receives the automatic version";
  translation: "Without its own text, this language receives the automatic version"), seed
  notice and errors (rule 9). Switching to Automatic: `onChange(undefined)` behind a confirm
  when text would be lost. No client-side Liquid check: the save answers 400 with the
  reason, shown by the drawer.
- `EmailBuilder.tsx`: `textVersion` gains `custom?` (same shape) passed to the panel; in
  Custom mode the panel renders even when `compilationResults` is absent
  (`EmailBuilder.tsx:879-882`).
- `MjmlCodeEditor.tsx`: new prop `textVersion?: { custom?: … }`; its text tab
  (`:846-876`) renders `TextVersionPanel` with `compact`, not tied to `compileErrors` in
  Custom mode.
- **Editor compile calls never send `text`** (the editors only show the automatic text);
  only `TemplatePreviewDrawer` does.
- `CreateTemplateDrawer.tsx`: state `emailText` seeded from a non-blank
  `template.email.text` (edit `:473`, duplicate `:505`, latest `:408`), reset in
  `handleClose` (`:537-548`); a change sets `dirtyRef`; sent as `email.text` only when set;
  `onSeed` compiles the current tree or source (rule 9) and reports `lostLiquid` from
  `response.mjml`; save shows rule 8's warning; a 400 naming the text version of the default language
  switches the drawer to the email editor (one of a translation stays on the translations),
  and the error is shown.
- `TemplateTranslationsTab.tsx`: `TranslationEditorState.text?`, not cloned nor reset from
  the default (rule 2); its drawer editors get `custom` with `isTranslation`; the card shows
  "Text version: Custom" when set. `CreateTemplateDrawer` maps it to
  `translations[lang].email.text` (`:729-738`) and seeds it from a non-blank value (`:382`).
- `TemplatePreviewDrawer.tsx`: sends `effectiveEmail?.text` (language-resolved, as the
  subject at `:105`); tag Automatic / Custom from `text_source`; caption for Custom ("Written by hand for this
  template … Its links are not tracked.") replaces PR 1's (`:232`); `text_error` shown above the
  text.
- Lingui on every string, `npm run lingui:extract`, the 8 catalogs translated.
- Tests:
  - `TextVersionPanel.test.tsx`: read-only without `custom`; cards; typing calls `onChange`;
    Custom → Automatic confirm then `onChange(undefined)`; seed fills the area, area disabled
    during the seed, failure keeps the area and shows the error, notice when
    `lostLiquid`; warning box per category (marketing, opt_in, transactional);
    translation note; `disabled`.
  - `EmailBuilder.test.tsx`: Custom textarea rendered while compile errors; Automatic shows
    the automatic text after leaving Custom.
  - `MjmlCodeEditor.test.tsx`: text tab editable with `custom`, read-only without.
  - `CreateTemplateDrawer.test.tsx`: payload has `email.text` only when custom; per-language
    `text`; seeding from an existing template and translation; seed compile called with
    `preserve_liquid: true`, no `test_data`, tracking off; editor compile calls carry no
    `text`; close then create again → no `email.text` (cancel and after a save); warning on
    save for marketing without `unsubscribe_url`, none for transactional.
  - `TemplateTranslationsTab.test.tsx`: enable and reset leave `text` unset; card tag.
  - `TemplatePreviewDrawer.test.tsx`: sends the translation's text; default has a text and
    the translation none → no `text` sent; tag follows `text_source`; `text_error` shown.

### 9. Integration — `tests/integration/plain_text_alternative_e2e_test.go`

- Transactional: custom text with Liquid → raw `text/plain` part (CRLF normalised) equals
  the rendered text; HTML part unchanged and tracked.
- Broadcast to a list with a custom text holding `{{ unsubscribe_url }}` → every recipient's
  text part is the custom text through the queue payload, with a working unsubscribe link.
- `fr` translation with its own text → that text for a `fr` contact; `de` translation
  without → automatic text of the `de` HTML.
- A custom text stored with invalid Liquid directly in the database → sent with the
  automatic text.
- `templates.compile` with a valid `text` (`text_source: custom`) and an invalid one
  (`success`, `text_error`).
- `template_api_test.go`: round-trip of `email.text` and translation texts; update without
  `text` → Automatic; invalid Liquid → 400 naming the text version; 64 KiB + 1 → 400.

### 10. Docs, changelog

- Docs (`../docs`, branch `docs/plain-text-custom` from `dev`, which has the PR 1
  "Plain-text version" section since docs#27): "Write your own" (per language, Liquid, links
  not tracked, unsubscribe link for marketing, seed limit, fallback); `openapi.json`.
- `CHANGELOG.md`, unreleased 43.2: rewrite PR 1's bullet into one coherent feature bullet
  (automatic by default, or your own per template and language in the Text version mode),
  plus a "before you upgrade" note for API users: a text set in `email.text` is now sent,
  validated (Liquid syntax, 64 KiB, 400 otherwise), rendered with Liquid (a literal `{{`
  needs `{% raw %}`), per language, links not tracked.

### 11. Run

`go test -race ./pkg/plaintext/ ./pkg/notifuse_mjml/ ./internal/domain/ ./internal/service/...
./internal/http/`; `make test-integration` (Mailpit + Postgres via
`tests/compose.test.yaml`); `cd console && npx vitest run && npx tsc -b && npx eslint . &&
npm run build`; `make openapi-lint`; then `make test-unit` once.

## Risks

| Risk | Mitigation |
|---|---|
| A custom text goes stale when the HTML changes | the warning box says so; Automatic stays the default |
| Stored API values go live at upgrade | "before you upgrade" note; render errors fall back to automatic |
| Missing unsubscribe link in a marketing custom text | warning box, save-time warning, docs |
| Seed misses conditions and loops between blocks | notice when the MJML holds `{%`, server check of the seed, docs |
| Queue payload size | rendered custom text capped at 256 KiB, above it automatic |
| Licence gate on translations | one normaliser on both sides; widen tests |

## Known, not handled (out of scope)

- Template Import/Export does not carry the custom text.
- Custom-text links are neither tracked nor given UTM parameters.
- No per-send override of the text in `transactional.send`.
- Seeding does not carry conditions and loops written between blocks (stated in UI and docs).
- Liquid parse errors keep liquidgo's message keys (`errors.syntax.unknown_tag`).
- An update that leaves stored, never-validated invalid translation text answers 500 without
  a reason (existing error mapping of `template_handler.go:188`).
- Previews of sent messages render the default language, so their tag follows the default.
- `]` after a URL in the automatic text (PR 1 known item).

## Rules for tests and comments

No count in a test name or comment unless an assertion pins it.

## Review log

Revision 2, independent plan review (`reviews/feat-plain-text-custom-revue-fable-xhigh-plan.md`):

| Finding | Change |
|---|---|
| B1 seed drops conditions and loops between blocks, fails on `<` | limit stated in UI and docs, `{%` notice, server validation of the seed, failure path, pinning tests |
| B2 editor compile with `text` shows custom as automatic | editor compile calls never send `text`; textarea independent of the compile |
| B3 clone contradicts no-inheritance | no clone (mock-up 06) |
| B4 `emailText` never reset | reset in `handleClose`, test |
| B5 unsubscribe advice for transactional | warning box by category |
| B6 line endings one-sided in the licence gate | one normaliser for `Validate`, `authoredTranslation` and render |
| B7 tests passing with the bug | zero-width cleaning case; mocked sites assert `req.Text`; negative preview case; byte-identical only with tracking off |
| B8 false claims | rendered cap 256 KiB; docs base now has the section (docs#27 merged); `MjmlCodeEditor` prop added; no client Liquid check; stale-text sentence added |
| Suggestions taken | blank render → automatic; one warning per batch; coherent changelog bullet and API note; OpenAPI wording and lint; CI steps in Run; converter without options; `CustomText()` method; refuse include/render; recover and fresh env; normalise at render; integration paths; console details |
