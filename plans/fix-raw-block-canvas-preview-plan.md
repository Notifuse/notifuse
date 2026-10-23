# Fix: the raw HTML block's canvas preview loses its fonts and the bottom of its content

Created 2026-10-23 on `dev` @ `b3c0541` (after #472). `VERSION` is `42.0`, unreleased.
Branch `fix/raw-block-canvas-preview`, one PR to `dev`. Console only: no Go file, table or environment variable changes.
Amended after Gemini's counter-review (v2; raw answer `reviews/fix-raw-block-canvas-preview-gemini-brut-plan.md` in the project files, each remark checked in `reviews/fix-raw-block-canvas-preview-gemini-verif-plan.md`): the height is measured on a wrapper around the content (C2), the late-font path is exercised in the browser test, and the font listener is removed from the document it was added to.
Gemini's second counter-review (`reviews/fix-raw-block-canvas-preview-gemini-brut-plan-v2.md`, checked in `reviews/fix-raw-block-canvas-preview-gemini-verif-plan-v2.md`) found the changes sound; its three remarks are applied as they stand (v3): the wrapper must be the body's only node, its `display` is `!important`, and the test font is resolved from the package that installs it.
Amended after the first high review, of `cd93bc9` (v4): the height is measured with the frame collapsed, its parent holding its height, instead of on a wrapper (C2); fonts go through one helper shared with the side panel, which accepts protocol-relative addresses (C1); the frame sends no `Referer` (C1). Gemini's third counter-review, on these changes: `reviews/fix-raw-block-canvas-preview-gemini-brut-plan-v3.md`, checked in `reviews/fix-raw-block-canvas-preview-gemini-verif-plan-v3.md`; adopted: the measurement also reads the root element, for margins that collapse through a body without padding.

## Problem

The email editor's canvas shows an `mj-raw` block in `SandboxedIframe` (`console/src/components/email_builder/blocks/MjRawBlock.tsx`): a `srcdoc` frame, `sandbox="allow-same-origin"`, `inert` since #472, whose height the canvas sets from the frame's `body` measurements.

1. **mj-font fonts never load.** The frame's single `<style>` holds the reset rules, then `@import url('<href>')` for each `mj-font`, then the base rules and the `mj-style` content. CSS ignores an `@import` that follows any other rule, so the browser drops every import. Same for an `@import` at the top of an `mj-style`, which is appended after the base rules. In the sent email both work: gomjml writes each font as a `<link>` plus `@import` before any rule, and all `mj-style` content in a `<style>` element of its own (`gomjml@v0.12.0/mjml/render.go`, `generateCustomStyles`).
2. **The height goes stale.** It is measured on the frame's `load` and once 200 ms after the effect runs. When the column changes width afterwards (column width or column count changed, window or drawer resized), the text rewraps and the frame keeps its old height. Since #472 the frame is inert, so the cut-off bottom can no longer be scrolled into view. Once fix 1 makes fonts load, a font arriving after `load` changes the line heights too.

Checked in Chromium 1194 on a scratch page (sandboxed `srcdoc` frame, `allow-same-origin`, `inert`, a stylesheet whose font file arrives 1.5 s late):
- reset rules then `@import`: the frame's `document.fonts` stays empty and the text keeps the fallback font. `@import` first: the face is registered and loads.
- the frame's `load` fires at 23 ms with the font still `loading`; the font arrives at 1.5 s and the body grows from 141 to 144 px. Font loads do not delay `load`.
- a `ResizeObserver` created by the parent and observing the `iframe` element fires when the column narrows. A `loadingdone` listener added by the parent on the frame's `document.fonts` fires although the frame runs no script.
- narrowing the column from 600 to 300 px takes the body from 94 to 141 px; nothing in the current code measures again.

## Changes

### C1. Fonts and `mj-style` placement (`MjRawBlock.tsx`, `SandboxedIframe`)

- `extractFontImports` becomes `extractFontLinks`: for each `mj-font` with a name and an `href`, a `<link rel="stylesheet" href="…">` in the frame's `<head>`, before any `<style>`, as gomjml writes it. A `<link>` has no ordering rule to break, unlike `@import`.
- Only hrefs that the new `fontStylesheetUrl(href)` in `console/src/utils/safeHtml.ts` accepts. It returns null for Liquid (`isLiquidExpression`), reads a protocol-relative href (`//fonts.googleapis.com/css?family=Lato`, a form older Google Fonts snippets use and the email resolves) as `https:`, then applies `safeExternalUrl`: an absolute http(s) URL on another origin, normalised. `ImportFontInput` uses the same helper to decide whether it shows the href as a link, instead of its own copy of the test. A Liquid href has no address until the email is sent; a `javascript:`, `data:` or relative href is not a stylesheet the canvas should fetch. The normalised URL is written with `&` and `"` escaped as `&amp;` and `&quot;` (new `escapeHtmlAttribute`, local to the file): a Google Fonts query such as `?family=A&display=swap` keeps its `&`, and a `&copy=` in a query is not read as `©`.
- The frame's head starts with `<meta name="referrer" content="no-referrer">`: the `iframe`'s `referrerPolicy="no-referrer"` covers only the frame's own navigation, so the font requests, now that they happen, and the raw HTML's images would otherwise send the console's address to the font or image host.
- The `mj-style` content moves out of the base `<style>` into a second `<style>` element of its own, after the base one, as in the email: an `@import` at the top of the first `mj-style` now applies, as it does once sent. Order of the rules is unchanged (custom after base), so what overrides what stays as it is.

### C2. Height kept in step with the content (`MjRawBlock.tsx`, `SandboxedIframe`)

In the existing `useEffect` (keyed on `fullHtmlContent`), keeping `adjustHeight` and its 50 ms debounce as they are:
- A `ResizeObserver`, created in the console's window, observes the `iframe` element. Its callback re-measures only when the frame's width differs from the last width it measured (`entry.contentRect.width`); its first callback, on `observe`, measures. Height-only changes are ignored on purpose: the canvas sets that height itself, and content sized in `vh` or `%` of the frame's viewport would otherwise grow the frame on every measurement, without end.
- On each `load` of the frame (each new `srcdoc` document), a `loadingdone` listener on that document's `fonts` re-measures, then the height is measured as before. The listener on the previous document is removed. `fonts` is optional-chained: a browser without `FontFaceSet` keeps today's behaviour.
- `iframe.onload = …` becomes `addEventListener('load', …)`, removed in the cleanup. The handler also runs once when the effect starts, so a frame that loaded before the effect ran still gets its listener and a measurement; this replaces the 200 ms fallback timer, which only re-measured.
- Cleanup: `observer.disconnect()`, the `load` listener and the current `loadingdone` listener removed, the debounce timer cleared.

- The fonts listener is kept with the `FontFaceSet` it was added to (a local variable of the effect), and removed from that one on the next `load` and in the cleanup, not from whatever `iframe.contentDocument` holds by then.

The measurement is taken with the frame collapsed. Read at the frame's current height, content sized on the frame's viewport (`html, body { height: 100% }`, a common email reset; `div { height: 100% }`; `min-height: 50vh`) measures that height, so the frame could never shrink and, now that width changes and fonts trigger measurements, would grow on each one. New `measureContentHeight(iframe, body)`: its parent (the block's element) gets `min-height` equal to its current `offsetHeight`, the frame gets `height: 0px; min-height: 0px`, the document is measured: `max` of the body's `scrollHeight`, `offsetHeight` and `clientHeight`, as today, and of the root element's `scrollHeight` and `offsetHeight`, which count the content's margins when an `mj-style` removes the body's padding and they collapse through it (at full height the root element read the frame's height, so it was left out; collapsed it does not), then the three inline values are put back as they were, in a `finally`. All of it runs in one task, so nothing is painted collapsed; the parent keeping its height means nothing around the block moves, and a scrolled canvas keeps its scroll position. Checked in Chromium on a scratch page, a 400 px frame at the bottom of a scrolled container: plain content 62 px either way; `html, body { height: 100% }` 400 px at full height, 52 px collapsed; `div { height: 100% }` 400 → 31 (the overflowing paragraph counted); `min-height: 50vh` 241 → 62; content after a stray `</div>` 62 → 62. The container's `scrollTop` stayed at 1100 with the parent holding its height, and dropped to 700 without it. The frame's `ResizeObserver` reports no change, since the frame's size is the same at the next rendering. This replaces the wrapper of v2, which covered only `%` heights on `html` and `body` and fell back to the body when content escaped it.

Content sized in `vh` is laid out against a zero-height viewport for the measurement, so its frame can end up a little shorter than that content at the frame's final height; what `vh` means in an email depends on the reader's window anyway, and the frame stays the same size on every measurement.

No change to the sandbox or to `inert`.

### C3. `CHANGELOG.md`

Under `[42.0] - WIP`, one line:
`- **Improvement**: In the email editor, a raw HTML block now shows the fonts imported in the template, and its whole content when its column changes width.`

## Tests (written first, each seen failing on `b3c0541`)

### Unit, `console/src/components/email_builder/blocks/canvasRendering.test.tsx` (jsdom)

Describe `editor canvas raw HTML` gains:
1. **Font links before any style.** Tree with an `mj-head` holding two `mj-font` (`https://fonts.googleapis.com/css2?family=Inter&display=swap`, `https://fonts.example.com/b.css`) and the raw block: the frame's `srcdoc` has a `<link rel="stylesheet" href="https://fonts.googleapis.com/css2?family=Inter&amp;display=swap">` and one for `b.css`, both before the first `<style`, and no `@import url(` from them.
2. **Hrefs that are not an external stylesheet are left out.** `{{ font_url }}`, `javascript:alert(1)`, `/fonts/a.css`, the console's own origin, an `mj-font` without a name: no `<link` in the `srcdoc`.
3. **An href cannot leave its attribute.** `https://fonts.example.com/a.css?x="><b id="escaped">`: the `srcdoc` has no `"><b`, and parsing it with `DOMParser` gives one `link` whose `href` is the normalised URL and no `b#escaped`.
4. **`mj-style` in its own element.** An `mj-style` starting with `@import url('https://fonts.example.com/c.css');` then a rule: parsed with `DOMParser`, the last `style` element's text starts with that `@import`, and the base rules (`font-family: Arial`) are in an earlier `style` element.
5. **Height follows the width.** (jsdom has no layout and does not load `srcdoc`: the frame keeps its about:blank document, whose body height the tests stub; the real measurement is proved in the browser test.) `ResizeObserver` replaced for the test by a class that records its callback, target and `disconnect` calls; fake timers; the frame document's `body.scrollHeight` stubbed. First callback at width 600 with `scrollHeight` 80 → frame height `100px` (minimum); `scrollHeight` 300, callback at width 600 → height unchanged; callback at width 300 → `310px`. Unmount → `disconnect` called.
6. **Height follows late fonts.** The frame document's `fonts` stubbed as an `EventTarget`; a `load` event on the frame; `scrollHeight` raised to 400; `loadingdone` dispatched → `410px`. A second document (new `fonts` stub) and `load`: a `loadingdone` on the first document no longer measures. After unmount, `loadingdone` on the current one changes nothing. Timers advanced inside `act`.
7. **Measured collapsed, parent held.** The block element's `offsetHeight` stubbed at 250; the body's `scrollHeight` getter records the frame's `height` and `min-height` and the parent's `min-height` when read: `0px`, `0px`, `250px`. After the measurement the parent's `min-height` is back to empty and the frame's `min-height` to `100px`.
8. **No `Referer`.** The `srcdoc`'s head has `<meta name="referrer" content="no-referrer">` before any `<link>`.

`console/src/utils/safeHtml.test.ts`, `fontStylesheetUrl`: Google Fonts URL kept; `//fonts.googleapis.com/css?family=Lato` → `https://fonts.googleapis.com/css?family=Lato`; `{{ font_url }}`, `https://{{ host }}/a.css`, `{% if x %}…`, `javascript:`, `data:`, `/a.css`, the console's origin, `''`, `undefined` → null.

`console/src/components/email_builder/ui/ImportFontInput.test.tsx`: a font whose href is protocol-relative is shown as a link; a Liquid one as text.

If jsdom replaces the frame's document when it processes `srcdoc`, tests 5 and 6 stub the document the frame holds after its `load` event.

### Browser, `console/e2e/features/content-rendering.spec.ts` (Playwright, Chromium, API mocked)

Describe `Template content in the email editor` gains one test, `raw HTML block loads the template's fonts and shows all its content`:
- Template tree with an `mj-head` holding an `mj-font` (`Probe Sans`, `https://fonts.example.com/probe.css`) and, in the body's column, a raw block of several paragraphs in `font-family: 'Probe Sans'`. `https://fonts.example.com/**` routed: requests recorded.
- The stylesheet's `@font-face` for `Probe Sans` points at `https://fonts.example.com/probe.woff2`, answered when the test releases it, after the frame has loaded, with KaTeX's `KaTeX_Main-Regular.woff2` (resolved with `createRequire` from `@ant-design/x-markdown`'s `package.json`, the package that installs KaTeX), with `size-adjust: 300%` so the font's arrival makes the text taller. Fonts: both the stylesheet and the font file are requested, and once the font has arrived the frame's `document.fonts.check('16px "Probe Sans"')` is true and the frame has grown to cover its document (`frame.clientHeight >= #end bottom`, polled).
- Width: the column element narrowed to 200 px with a style change → `expect.poll` until the frame is taller than before and again covers its content; column restored → the frame returns to its height before narrowing.
- The template's `mj-style` holds `html, body { height: 100% }`: the frame still returns to its smaller height when the column is restored (fails with a body measurement).
- More raw blocks, under the same `html, body { height: 100% }`: one with a `min-height: 50vh` element, one with paragraphs after a stray `</div>`, taller than the 100 px minimum. Each frame covers its content (the `vh` one aside, see above) and is back to its first height after the column is narrowed and restored, where a measurement at full height grows it.
- The font stylesheet request carries no `Referer` header.
- The template's `mj-style` also sets `body { padding: 0 }`, and a raw block's content sits in a `div` with 80 px vertical margins, which then collapse through the body: its frame covers its last line.

## Not in scope

- The other canvas blocks (`mj-text`, `mj-button`…) do not load `mj-font` either; they render in the console's page, where loading a template's stylesheet would apply it to the console. Separate change if wanted.
- `</style>` in an `mj-style` is written as is, as the email does (see the verification file, remark 3).
- Lazy images inside the raw HTML that load after `load` are not re-measured; rare in an email.

## Test step

From `console/`: `npx tsc -b`, `npx vitest run`, `npx eslint .`, `npm run build` (the CI steps), then `npx playwright test e2e/features/content-rendering.spec.ts e2e/features/templates.spec.ts` against the dev server. No Go file changes, so no Go test target applies.
