import type { CSSProperties } from 'react'
import DOMPurify, { type Config } from 'dompurify'

const CONFIG: Config = {
  ADD_ATTR: ['target'],
  ALLOW_UNKNOWN_PROTOCOLS: true,
  FORBID_TAGS: [
    'style',
    'form',
    'input',
    'button',
    'select',
    'option',
    'textarea',
    'map',
    'area',
    'audio',
    'video',
    'source',
    'track'
  ],
  FORBID_ATTR: ['data-block-id']
}

// Instances of their own, so that each configuration applies to every call
// through it, isSafeHref's included, and to nothing else on the page that uses
// DOMPurify.
const purify = DOMPurify(window)
purify.setConfig(CONFIG)

// For editors, an attribute value that reads as the end of a comment or of a
// CDATA section (`-->`, `]>`) is kept: DOMPurify drops it because markup
// carrying it can parse differently once inserted elsewhere, while an editor
// parses what it loads in a document that runs nothing and builds its own DOM
// from the result, node by node. As DOMPurify requires for that, the content is
// HTML only, without SVG or MathML, which the editors have no place for anyway.
const purifyForEditor = DOMPurify(window)
purifyForEditor.setConfig({
  ...CONFIG,
  USE_PROFILES: { html: true },
  SAFE_FOR_XML: false
})

/**
 * HTML for display in the console page itself, such as a template block on the
 * editor canvas. Shown in an element styled with `containedContentStyle`.
 *
 * Event handlers, `<script>`, frames and `javascript:`, `vbscript:` and `data:`
 * links are removed, and so are `<style>`, which would restyle the console
 * around it, form controls, image maps, whose areas are links the canvas would
 * follow, audio and video, which would play in it, and the email builder's own
 * `data-block-id`, which locates blocks on the canvas. Formatting,
 * inline styles, classes, `data-*` attributes, link targets, links that open
 * another app (`webcal:`, `whatsapp:`…) and Liquid (`{{ url }}` in an `href`
 * included) are kept.
 */
export function sanitizeHtml(html: string): string {
  if (!html) return ''
  return purify.sanitize(html)
}

/**
 * HTML for a block editor to load: what `sanitizeHtml` keeps, and also
 * attribute values it drops because they contain `-->` or `]>`, such as a link
 * to `https://example.com/?q=a]>b`. An editor saves what it loaded, so anything
 * dropped here that an email may legitimately carry would be lost on the next
 * edit. SVG and MathML are left out, along with what they contain.
 */
export function sanitizeEditorHtml(html: string): string {
  if (!html) return ''
  return purifyForEditor.sanitize(html)
}

/**
 * Style for the element that shows content in the console page, on the canvas
 * or in an editor. Paint containment keeps what the content draws, fixed
 * positioning included, on that element rather than over the console around
 * it, and below the toolbars stacked above a block, whatever its `z-index`.
 * What the content draws just past the element's edges, such as a shadow or a
 * list marker, is still shown up to 25px out, into the padding an email gives
 * its text by default, where the browser supports it.
 */
export const containedContentStyle: CSSProperties = {
  contain: 'paint',
  overflowClipMargin: '25px'
}

let inertDocument: Document | null = null

/**
 * Parses HTML in a document that has no browsing context, for code that reads
 * or rewrites markup without rendering it.
 *
 * Nothing in such a document is fetched or run, unlike an element created with
 * `document.createElement`, which belongs to the page even while detached. Any
 * node added to the result must come from `container.ownerDocument`: a node of
 * the page's document, a fragment included, takes what is moved into it back
 * into the page. The document is created once; every call gets a container of
 * its own, which is never attached to it.
 */
export function parseHtmlInert(html: string): HTMLDivElement {
  inertDocument ??= document.implementation.createHTMLDocument('')
  const container = inertDocument.createElement('div')
  container.innerHTML = html
  return container
}

/**
 * The text of some HTML, as HTML that shows only that text: the markup is
 * dropped, and whatever in the text reads as markup is escaped.
 */
export function textOnlyHtml(html: string): string {
  const container = parseHtmlInert(html)
  container.textContent = container.textContent ?? ''
  return container.innerHTML
}

/**
 * A URL the console may open in a new tab or show in a frame: an absolute
 * http(s) URL on another origin than the console's. Returns it normalised, or
 * null.
 */
export function safeExternalUrl(url: string | null | undefined): string | null {
  if (!url) return null
  try {
    const parsed = new URL(url)
    if (parsed.protocol !== 'http:' && parsed.protocol !== 'https:') return null
    if (parsed.origin === window.location.origin) return null
    return parsed.toString()
  } catch {
    return null
  }
}

const OPENABLE_PROTOCOLS = new Set(['http:', 'https:', 'mailto:', 'tel:'])

/**
 * Whether the console offers a URL as a link to follow, as the editor's side
 * panel does for a link target: an absolute web, mail or phone address. Any
 * other scheme would hand the click to an app on the member's computer.
 */
export function isOpenableUrl(url: string | null | undefined): boolean {
  if (!url) return false
  try {
    return OPENABLE_PROTOCOLS.has(new URL(url).protocol)
  } catch {
    return false
  }
}

/**
 * Whether a saved link target can be put in an `href` in the console: the same
 * verdict `sanitizeHtml` gives a link in content, so anything but a script
 * (`javascript:`, `vbscript:`…) or `data:` URL. Web, mail, phone and app links,
 * relative ones and Liquid (`{{ url }}`, `https://{{ host }}/x`) all pass.
 */
export function isSafeHref(href: string | null | undefined): boolean {
  if (!href) return false
  return purify.isValidAttribute('a', 'href', href)
}
