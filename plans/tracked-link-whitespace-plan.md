# Tracked links written with surrounding whitespace — plan

## Goal

An href written with spaces around it, or with a tab or line break inside it, is a link browsers open: the WHATWG URL parser trims leading and trailing C0 controls and spaces and removes tab, CR and LF before reading it. The send path does not read it that way, so these links get their UTM parameters in the wrong place, get none at all, or, for a `mailto:`, get wrapped in a tracking redirect they must never have. The goal is for `TrackLinks` to read every href the way the recipient's browser will, before deciding anything about it.

Found by the second code review of PR #488 (2026-11-05). It is older than that PR and independent of it.

## Current behaviour (facts, measured on `dev` with a probe calling `TrackLinks`, 2026-11-05)

| href as written | UTM only (`UTMContent: "tpl"`) | click tracking on, minted destination |
|---|---|---|
| `https://shop.com/sale ` (trailing space) | `https://shop.com/sale ?utm_content=tpl` | `https://shop.com/sale ?utm_content=tpl` |
| ` https://shop.com/sale` (leading space) | unchanged, no UTM | ` https://shop.com/sale`, no UTM |
| `https://shop.com/sa\nle` (line break inside) | unchanged, no UTM | `https://shop.com/sa\nle`, no UTM |
| `\thttps://shop.com/x?a=1` (leading tab) | unchanged, no UTM | `\thttps://shop.com/x?a=1`, no UTM |
| ` mailto:hi@shop.com` (leading space) | unchanged | **wrapped in `/r/`**: ` mailto:hi@shop.com` |

Why each happens (facts, from the code):
- `TrackLinks` (`pkg/notifuse_mjml/template_compilation.go`, the `replaceQuotedValues` callback) passes the decoded href straight to `isNonTrackableURL`, `applyUTMParameters` and `applyIdentityParam`.
- `url.Parse` accepts a trailing space (path `"/sale "`), so `applyUTMParameters` tags the link and `appendToRawQuery` writes `?utm_content=` after the space.
- `url.Parse` refuses a leading space (`first path segment in URL cannot contain colon`) and control characters, so both helpers return the link untouched.
- `isNonTrackableURL` tests `strings.HasPrefix(lower, "mailto:")` on the raw value, so a leading space defeats it and the `mailto:` is wrapped in a redirect.

Consequence of the trailing-space case (per the WHATWG URL spec, not yet observed in a browser): a space inside a path is percent-encoded, so the recipient lands on `/sale%20?utm_content=tpl` instead of `/sale`, often a 404. On the send path `utm_content` defaults to the template ID, so **every such link is affected on every send**, whether click tracking is on or off. The only exception is a notification whose tracking is set to disabled, which leaves links untouched.

Verified before implementing (2026-11-05):
- gomjml keeps the whitespace of an href as written, in `mj-text` links and in `mj-button` (probe through `CompileTemplate`), so the whitespace does reach `TrackLinks` and the e2e test below exercises the fix.
- A verified click answers `Location: tok.URL` through `http.Redirect`, which only percent-encodes non-ASCII bytes: the space reaches the browser as written.
- ` javascript:x` (leading space) is wrapped in `/r/` today and answered 400 on click (`blockedSchemes`). After the fix it is recognised as non-trackable and left as written, like `javascript:x` written without the space today.

## Decisions (validated by Pierre on 2026-11-05: "verifie le plan et go"; delivery as a separate PR after #488)

- **D1 — Preprocess once, at the top of the `TrackLinks` callback.** Right after `decodeHrefCharacterReferences`, apply `trackingtoken.Preprocess` (the same function the click route and recorder use). Every later step (non-trackable check, UTM, identity token, minting) reads the preprocessed URL. This fixes all five rows at their common root, instead of patching each helper.
- **D2 — A link left unchanged keeps its bytes.** When the preprocessed link turns out non-trackable (`mailto:`, `tel:`, `#…`, Liquid, empty), the callback still returns `rawValue` as today: the author's href, spaces included, which mail clients and browsers already handle. Only links Notifuse rewrites are written back in their preprocessed form.
- **D3 — Accept one key split for links already sent.** New sends record clicks under `https://shop.com/sale?utm_content=…` instead of `https://shop.com/sale ?utm_content=…`. Emails already delivered keep their old key, as PR #488 guarantees. A broadcast still sending, or paused, across the upgrade shows such a link on two rows of its per-link stats. The alternative, keeping the broken form, would keep sending recipients to `/sale%20`.
- **D4 — Changelog: a separate `Fix` bullet in `## [42.3] - WIP`**, since it is a different symptom from the `50%off` bullet (links landing on the wrong page, a `mailto:` tracked).
- **D5 — Delivery:** a separate PR, stacked on PR #488's branch (it needs #488's `## [42.3] - WIP` section), retargeted to `main` once #488 is merged.

Not in scope: hrefs Notifuse builds itself (unsubscribe, notification center), which never hold whitespace; the console editors, which are not where the whitespace comes from in the reported cases.

## Implementation steps

### Step 1 — Preprocess the href in `TrackLinks`

File: `pkg/notifuse_mjml/template_compilation.go`. In the `replaceQuotedValues` callback of `TrackLinks`, set `originalURL := trackingtoken.Preprocess(decodeHrefCharacterReferences(rawValue))`, with a comment saying why (browsers read the href that way; the redirect route and click recorder already do). Keep the `return rawValue` for non-trackable links (D2). No change to `applyUTMParameters`, `applyIdentityParam` or `isNonTrackableURL`: they now receive the preprocessed URL.

Test file: `pkg/notifuse_mjml/template_compilation_whitespace_test.go` (new; it reuses the token regexp of `template_compilation_signing_test.go`), `TestTrackLinks_ReadsHrefsAsBrowsersDo`, table-driven, with UTM-only settings and asserting the output HTML exactly:
- `https://shop.com/sale ` → `href="https://shop.com/sale?utm_content=tpl"`;
- ` https://shop.com/sale` → `href="https://shop.com/sale?utm_content=tpl"`;
- `https://shop.com/sa\nle` and `\thttps://shop.com/x?a=1` → line break and tab removed, UTM appended;
- ` mailto:hi@shop.com` and ` tel:+33…` → returned byte-for-byte, never wrapped (asserted with tracking on too: no `/r/` in the output);
- ` #top` → unchanged;
- `https://shop.com/{{ contact.x }} ` → unchanged (Liquid guard still wins);
- a link with no whitespace → byte-identical to today's output (regression guard).

Second test, same file, `TestTrackLinks_MintsThePreprocessedDestination`: tracking on, decode the minted `/r/` token with `trackingtoken.Parse` and assert `Token.URL` is `https://shop.com/sale?utm_content=tpl` for the trailing- and leading-space hrefs; with `IdentifyToken` and `IdentifyAllowedHosts: ["shop.com"]`, a leading-space href gains `nf_id`.

### Step 2 — End-to-end with a real email

File: `tests/integration/link_click_mailpit_e2e_test.go`, in `TestLinkClickMailpitE2E_URLsWithQuotesOrCharacterReferences` (code-mode send):
- add `<a href="https://example.com/trailing-space ">code text trailing space</a>` and `<a href=" https://example.com/leading-space">code text leading space</a>` to the MJML, with authored values `https://example.com/trailing-space` and `https://example.com/leading-space`; the existing loop then asserts each delivered href is a `/r/` link on the server and redirects to the authored URL plus `utm_content`;
- add `<a href=" mailto:hello@example.com">code text mailto</a>` and assert, from the delivered HTML read through `deliveredLinkHref`, that its href does not contain `/r/`.

The click is read from the real email in Mailpit and requested unchanged, as the test already does.

### Step 3 — Changelog

`CHANGELOG.md`, `## [42.3] - WIP`: one `Fix` bullet, 2-3 sentences: links written with a space before or after them, or a line break inside, now land on the right page with their UTM parameters and identity token; a `mailto:` or `tel:` link written with a leading space is no longer wrapped in a tracking redirect.

### Step 4 — Run the tests

- `go test ./pkg/notifuse_mjml ./pkg/trackingtoken ./internal/service ./internal/http` (or `make test-unit`).
- Mutation check: remove the `Preprocess` call and confirm both new `TrackLinks` tests and the e2e cases fail.
- Integration, environment up (`docker compose -f tests/compose.test.yaml up -d`): `INTEGRATION_TESTS=true go test -tags integration,licdev ./tests/integration -run '^(TestLinkClickMailpitE2E|TestLinkClickMailpitE2E_URLsWithQuotesOrCharacterReferences|TestEmailHandler_TrackedLinkPreservesUTM|TestTrackedLinksAreSignedOnEverySendPath|TestWebAnalyticsEmailIdentify|TestLinkClickStats)$' -count=1 -v`.
