# Fix: the raw HTML block's canvas preview loses its fonts and the bottom of its content

Created 2026-10-23 on `dev` @ `b3c0541` (after #472). `VERSION` is `42.0`, unreleased.
Branch `fix/raw-block-canvas-preview`, one PR to `dev`. Console only: no Go file, table or environment variable changes.
Amended after Gemini's counter-review (v2; raw answer `reviews/fix-raw-block-canvas-preview-gemini-brut-plan.md` in the project files, each remark checked in `reviews/fix-raw-block-canvas-preview-gemini-verif-plan.md`): the height is measured on a wrapper around the content (C2), the late-font path is exercised in the browser test, and the font listener is removed from the document it was added to.
Gemini's second counter-review (`reviews/fix-raw-block-canvas-preview-gemini-brut-plan-v2.md`, checked in `reviews/fix-raw-block-canvas-preview-gemini-verif-plan-v2.md`) found the changes sound; its three remarks are applied as they stand (v3): the wrapper must be the body's only node, its `display` is `!important`, and the test font is resolved from the package that installs it.

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
- Only hrefs that `safeExternalUrl` accepts and that hold no Liquid (`isLiquidExpression`), the same test `ImportFontInput` applies before showing the href as a link: an absolute http(s) URL on another origin. A Liquid href has no address until the email is sent; a `javascript:`, `data:` or relative href is not a stylesheet the canvas should fetch. The normalised URL is written with `&` and `"` escaped as `&amp;` and `&quot;` (new `escapeHtmlAttribute`, local to the file): a Google Fonts query such as `?family=A&display=swap` keeps its `&`, and a `&copy=` in a query is not read as `©`.
- The `mj-style` content moves out of the base `<style>` into a second `<style>` element of its own, after the base one, as in the email: an `@import` at the top of the first `mj-style` now applies, as it does once sent. Order of the rules is unchanged (custom after base), so what overrides what stays as it is.

### C2. Height kept in step with the content (`MjRawBlock.tsx`, `SandboxedIframe`)

In the existing `useEffect` (keyed on `fullHtmlContent`), keeping `adjustHeight` and its 50 ms debounce as they are:
- A `ResizeObserver`, created in the console's window, observes the `iframe` element. Its callback re-measures only when the frame's width differs from the last width it measured (`entry.contentRect.width`); its first callback, on `observe`, measures. Height-only changes are ignored on purpose: the canvas sets that height itself, and content sized in `vh` or `%` of the frame's viewport would otherwise grow the frame on every measurement, without end.
- On each `load` of the frame (each new `srcdoc` document), a `loadingdone` listener on that document's `fonts` re-measures, then the height is measured as before. The listener on the previous document is removed. `fonts` is optional-chained: a browser without `FontFaceSet` keeps today's behaviour.
- `iframe.onload = …` becomes `addEventListener('load', …)`, removed in the cleanup. The handler also runs once when the effect starts, so a frame that loaded before the effect ran still gets its listener and a measurement; this replaces the 200 ms fallback timer, which only re-measured.
- Cleanup: `observer.disconnect()`, the `load` listener and the current `loadingdone` listener removed, the debounce timer cleared.

- The fonts listener is kept with the `FontFaceSet` it was added to (a local variable of the effect), and removed from that one on the next `load` and in the cleanup, not from whatever `iframe.contentDocument` holds by then.

The measurement no longer reads the body, whose height follows the frame's own viewport when the content's CSS says `html, body { height: 100% }`, a common email reset: then every measurement read the frame's current height, so the frame could never shrink and grew 10 px with each one. The raw HTML goes into a wrapper, `<div data-raw-content style="display: flow-root !important">` (`!important` so an `mj-style` rule such as `body > div { display: flex !important }` cannot change it), the only node in `body` (the document ends `</div></body></html>`, so the parser adds no whitespace after it). The height is the wrapper's top in the document (`wrapper.getBoundingClientRect().top - documentElement.getBoundingClientRect().top`), plus `max(wrapper.scrollHeight, wrapper.offsetHeight)` (`scrollHeight` keeps content positioned past the wrapper), plus the body's computed bottom padding, then, as today, `+ 10` and at least 100 px. For the default body padding this equals today's figure for content that does not follow the viewport. `flow-root` keeps floats and the content's outer margins inside the wrapper; the body's 10 px padding already kept those margins from collapsing through it, so nothing moves. If the wrapper is not the body's only node, text included (a stray `</div>` in the raw HTML closed it early and what follows sits in the body), the measurement falls back to today's `max(body.scrollHeight, body.offsetHeight, body.clientHeight)`.

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
5. **Height follows the width.** (jsdom has no layout and does not load `srcdoc`: the frame keeps its about:blank document, which has no wrapper, so these tests go through the body fallback; the wrapper measurement is proved in the browser test.) `ResizeObserver` replaced for the test by a class that records its callback, target and `disconnect` calls; fake timers; the frame document's `body.scrollHeight` stubbed. First callback at width 600 with `scrollHeight` 80 → frame height `100px` (minimum); `scrollHeight` 300, callback at width 600 → height unchanged; callback at width 300 → `310px`. Unmount → `disconnect` called.
6. **Height follows late fonts.** The frame document's `fonts` stubbed as an `EventTarget`; a `load` event on the frame; `scrollHeight` raised to 400; `loadingdone` dispatched → `410px`. A second document (new `fonts` stub) and `load`: a `loadingdone` on the first document no longer measures. After unmount, `loadingdone` on the current one changes nothing. Timers advanced inside `act`.
7. **Content in a wrapper.** The `srcdoc`, parsed with `DOMParser`, has a `body` whose only child is `div[data-raw-content]` holding the block's HTML.

If jsdom replaces the frame's document when it processes `srcdoc`, tests 5 and 6 stub the document the frame holds after its `load` event.

### Browser, `console/e2e/features/content-rendering.spec.ts` (Playwright, Chromium, API mocked)

Describe `Template content in the email editor` gains one test, `raw HTML block loads the template's fonts and shows all its content`:
- Template tree with an `mj-head` holding an `mj-font` (`Probe Sans`, `https://fonts.example.com/probe.css`) and, in the body's column, a raw block of several paragraphs in `font-family: 'Probe Sans'`. `https://fonts.example.com/**` routed: requests recorded.
- The stylesheet's `@font-face` for `Probe Sans` points at `https://fonts.example.com/probe.woff2`, answered after 1 s with KaTeX's `KaTeX_Main-Regular.woff2` (resolved with `createRequire` from `@ant-design/x-markdown`'s `package.json`, the package that installs KaTeX), with `size-adjust: 300%` so the font's arrival makes the text taller. Fonts: both the stylesheet and the font file are requested, and once the font has arrived the frame's `document.fonts.check('16px "Probe Sans"')` is true and the frame has grown to cover its document (`frame.clientHeight >= wrapper bottom`, polled).
- Width: the column element narrowed to 200 px with a style change → `expect.poll` until the frame is taller than before and again covers its content; column restored → the frame returns to its height before narrowing.
- The template's `mj-style` holds `html, body { height: 100% }`: the frame still returns to its smaller height when the column is restored (fails with a body measurement).
- A second raw block holds `<p>Inside</p></div><p>Escaped</p>`: its frame still covers the escaped paragraph (the fallback measurement).

## Not in scope

- The other canvas blocks (`mj-text`, `mj-button`…) do not load `mj-font` either; they render in the console's page, where loading a template's stylesheet would apply it to the console. Separate change if wanted.
- Content sized in `vh` units still follows the frame's viewport, so each width change or font load can add the 10 px margin; `%` heights on `html` and `body` no longer do.
- `</style>` in an `mj-style` is written as is, as the email does (see the verification file, remark 3).
- Lazy images inside the raw HTML that load after `load` are not re-measured; rare in an email.

## Test step

From `console/`: `npx tsc -b`, `npx vitest run`, `npx eslint .`, `npm run build` (the CI steps), then `npx playwright test e2e/features/content-rendering.spec.ts e2e/features/templates.spec.ts` against the dev server. No Go file changes, so no Go test target applies.
