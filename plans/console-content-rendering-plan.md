# Console content rendering

Created 2026-10-21, for 42.0. Console only: no backend change, no migration, no new user-facing string.

Template, blog theme and web analytics content is written by workspace members or reported by a site's visitors, and shown to other members in the console. This change makes the console display it without running any script it carries, and makes the links and frames built from it point only where they should.

## Changes

1. **`console/src/utils/safeHtml.ts`** (new), four helpers:
   - `sanitizeHtml(html)`: DOMPurify with its default HTML profile, plus `target` and links to other apps (`webcal:`, `whatsapp:`…, while `javascript:`, `vbscript:` and `data:` stay refused), minus `<style>` and form controls. Keeps formatting, inline styles, classes, `data-*` and Liquid, `{{ url }}` in an `href` included. An editor saves what it loaded, so nothing an email may legitimately carry is dropped.
   - `parseHtmlInert(html)`: a `<div>` of `document.implementation.createHTMLDocument('')`, for code that reads or rewrites markup. Nothing in that document is fetched or run.
   - `safeExternalUrl(url)`: an absolute http(s) URL on another origin than the console's, or `null`.
   - `isSafeHref(href)`: a web, mail, phone or relative link, resolved as the browser would.
   - `dompurify` becomes a direct dependency of the console. It was already installed, at the same version, as a dependency of `@ant-design/x-markdown`.
2. **Email editor canvas.** `MjTextBlock` and `MjButtonBlock` render `sanitizeHtml` of their content, memoised, and only while the block is not open in its editor. A link click in a button's text selects the block, as it already did in a text block, instead of being followed. `MjImageBlock` and `MjSocialElementBlock` keep a link target on the canvas only if `isSafeHref` accepts it. The saved content is never rewritten.
3. **Block editors.** `TiptapRichEditor` and `TiptapInlineEditor` load `sanitizeHtml` of their content, in their text-only fallbacks too. The initial content is computed once, when the editor is created, rather than on every render.
4. **Markup helpers.** The tiptap `shared/utils.ts` helpers and `NotifuseEditor.addSyntaxHighlightingToHTML` parse with `parseHtmlInert`. Every node they create (span, text node, fragment) comes from that document, not from the page's.
5. **Previews.** The visual editor's email preview frame gets `sandbox="allow-popups allow-popups-to-escape-sandbox"`: no script, links still open in a new tab. The blog theme preview keeps `allow-scripts` and drops `allow-same-origin`.
6. **Contact timeline, View website.** `webPageURL` appends a path that starts with `/` to the visit's origin, resolves any other value against it, and keeps the result only if it is on that origin and passes `safeExternalUrl`. When the workspace lists web analytics allowed domains, the visit's domain must be one of them, matched as the server matches incoming visits (case, spaces and a port ignored, `*.example.com` for the domain and its subdomains, never a bare TLD). The workspace website, a fallback its members set, is not held to the list. `WebPageDrawer` applies `safeExternalUrl` again before linking or framing.
7. **Editor side panel.** `StringPopoverInput` and `ImportFontInput` show a value as a link only if `isSafeHref` accepts it, and as plain text otherwise.
8. **e2e fixtures.** `auth.ts` and `public-pages.spec.ts` mock `**/config.js*`: `index.html` loads `/config.js?_t=…`, which the old pattern missed, so every spec landed on the setup page.

## Tests

| Implementation | Test file | Cases |
|---|---|---|
| `utils/safeHtml.ts` | `utils/safeHtml.test.ts` (new) | `sanitizeHtml`: event handlers, `<script>`, `<iframe>`, `<style>`, `javascript:` (mixed case, leading space, entity), `vbscript:` and `data:` links removed; form controls removed with their text kept; styles, classes, `data-*`, `target`, `rel`, Liquid in text and `href`, `mailto:`, `webcal:`, `whatsapp:`, `tg:` and `sms:` links kept; empty input. `parseHtmlInert`: own document, no `defaultView`, markup intact. `safeExternalUrl` and `isSafeHref`: tables of accepted and refused values. |
| `blocks/MjTextBlock.tsx`, `MjButtonBlock.tsx`, `MjImageBlock.tsx`, `MjSocialElementBlock.tsx` | `blocks/canvasRendering.test.tsx` (new) | text and button canvas HTML carries no handler or `javascript:` link, keeps Liquid and styles, block content unchanged, `onUpdateBlock` not called; a click inside a link of either canvas is prevented and selects the block; image and social links drop a `javascript:` target, keep `https://` and Liquid. |
| `ui/tiptap/TiptapRichEditor.tsx`, `TiptapInlineEditor.tsx` | `ui/tiptap/TiptapEditors.test.tsx` (new) | the loaded editor DOM carries no handler attribute and no `javascript:` link, for rich content and for wrapped and block button content; formatting, Liquid and a `webcal:` link kept; `onChange` not called on load; content given after loading is cleaned the same way and not written back. |
| `ui/tiptap/shared/utils.ts` | `ui/tiptap/shared/utils.test.ts` (new) | each helper returns the same HTML as before, never calls `document.createElement`, `createDocumentFragment` or `createTextNode`, and inserts nodes only into a document of its own (the helpers that rewrite markup are seen inserting); attributes still copied onto converted spans. |
| `blog_editor/NotifuseEditor.tsx` | covered by `parseHtmlInert`'s tests | its input is the editor's own serialised output. |
| `panels/Preview.tsx` | `panels/Preview.test.tsx` (new) | the frame's `sandbox` value. |
| `blog/ThemePreview.tsx` | `blog/ThemePreview.test.tsx` (new) | the frame's `sandbox` value. |
| `timeline/ContactTimeline.tsx` | `timeline/ContactTimeline.test.tsx` (new cases) | `//x/y` stays on the visited site; a full URL on that site is kept; `javascript:`, `data:`, another host, `\\host` and ` //host` give no View website button; nor does a page on the console's origin. With allowed domains set: a listed domain (exact, any case and spacing, with a port, wildcard, wildcard apex, one of several) is offered; an unlisted one, the apex of a listed subdomain, a lookalike suffix and a TLD wildcard are not; the workspace website fallback still is. |
| `timeline/WebPageDrawer.tsx` | `timeline/WebPageDrawer.test.tsx` (new) | an https page is framed and linked; `javascript:`, `data:` and the console's origin are neither. |
| `ui/StringPopoverInput.tsx`, `ui/ImportFontInput.tsx` | `ui/StringPopoverInput.test.tsx`, `ui/ImportFontInput.test.tsx` (new) | an https value is a link; a `javascript:` or `data:` value is text. |
| all of the above, in a real browser | `e2e/features/content-rendering.spec.ts` (new, Playwright) | content carrying scripts and handlers, in a template (canvas, both block editors, side panel, email preview), a blog theme and a contact's page views. Each snippet records itself if it runs; none may. jsdom fires no image error and ignores `sandbox`, so this is where the behaviour itself is checked. |

## Checks

From `console/`: `npx tsc -b`, `npx vitest run`, `npx eslint .`, `npm run build`, then `npx playwright test e2e/features/content-rendering.spec.ts e2e/features/templates.spec.ts e2e/features/contacts.spec.ts e2e/features/blog.spec.ts` against the dev server.
