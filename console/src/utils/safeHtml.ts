import type { CSSProperties } from 'react'
import DOMPurify, { type Config } from 'dompurify'
import { hasLiquidOutput, isLiquidExpression } from './liquid'

// The schemes of the links the console offers to follow: web, mail and phone.
// Any other would hand the member's click to an app on their computer.
const OPENABLE_SCHEMES = ['http', 'https', 'mailto', 'tel']

const CONFIG: Config = {
  ADD_ATTR: ['target'],
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

// The prefix DOMPurify gives ids and names (SANITIZE_NAMED_PROPS), which it
// leaves off one that already has it
const ID_PREFIX = 'user-content-'
const prefixId = (id: string) => (id.startsWith(ID_PREFIX) ? id : ID_PREFIX + id)

// Attributes that refer to elements by id, as a list of ids
const ID_REFERENCES = new Set([
  'for',
  'headers',
  'aria-activedescendant',
  'aria-controls',
  'aria-describedby',
  'aria-details',
  'aria-errormessage',
  'aria-flowto',
  'aria-labelledby',
  'aria-owns'
])
// Attributes that may refer to an SVG paint server, clip, mask, filter or
// marker by id, as `url(#id)`
const URL_REFERENCES = new Set([
  'style',
  'fill',
  'stroke',
  'clip-path',
  'mask',
  'filter',
  'marker-start',
  'marker-mid',
  'marker-end'
])

// Audio and video give way to what they hold, the fallback a mail client that
// cannot play them shows, which DOMPurify would otherwise drop with them: it is
// moved out before DOMPurify removes the element, and sanitized where it lands.
const unwrapMedia = (node: Node, data: { tagName: string }) => {
  const parent = node.parentNode
  if ((data.tagName !== 'audio' && data.tagName !== 'video') || !parent) return
  for (let child = node.lastChild; child; child = node.lastChild) {
    parent.insertBefore(child, node.nextSibling)
  }
}

// Instances of their own, so that each configuration applies to every call
// through it, isSafeHref's included, and to nothing else on the page that uses
// DOMPurify.
const purify = DOMPurify(window)
// Ids and names get a prefix, and so does what refers to them, so that content
// shown in the page never names one of its globals (`window.monaco`) or another
// of its elements; nor does it change the order in which the keyboard moves
// through the page (`tabindex`). A link or image URL is kept only with one of
// the openable schemes, or with none, as a relative URL or Liquid (`{{ url }}`)
// has: DOMPurify's own test, with its list of schemes narrowed and digits read
// as part of a scheme (`ts3server:`), as browsers read them. That test reads a
// `srcset` as one URL, the first of its candidates, so an image shows its `src`.
purify.setConfig({
  ...CONFIG,
  SANITIZE_NAMED_PROPS: true,
  FORBID_ATTR: [...(CONFIG.FORBID_ATTR ?? []), 'tabindex', 'srcset'],
  ALLOWED_URI_REGEXP: new RegExp(
    `^(?:(?:${OPENABLE_SCHEMES.join('|')}):|[^a-z]|[a-z0-9+.\\-]+(?:[^a-z0-9+.\\-:]|$))`,
    'i'
  )
})
purify.addHook('uponSanitizeElement', unwrapMedia)
purify.addHook('uponSanitizeAttribute', (_node, data) => {
  const { attrName, attrValue } = data
  if (ID_REFERENCES.has(attrName)) {
    data.attrValue = attrValue.split(/\s+/).filter(Boolean).map(prefixId).join(' ')
  } else if ((attrName === 'href' || attrName === 'xlink:href') && /^#./.test(attrValue)) {
    data.attrValue = '#' + prefixId(attrValue.slice(1))
  } else if (URL_REFERENCES.has(attrName)) {
    data.attrValue = attrValue.replace(
      /url\((\s*['"]?)#([^'")\s]+)/gi,
      (_match, opening: string, id: string) => `url(${opening}#${prefixId(id)}`
    )
  }
})

// For editors, an attribute value that reads as the end of a comment or of a
// CDATA section (`-->`, `]>`) is kept: DOMPurify drops it because markup
// carrying it can parse differently once inserted elsewhere, while an editor
// parses what it loads in a document that runs nothing and builds its own DOM
// from the result, node by node. As DOMPurify requires for that, the content is
// HTML only, without SVG or MathML, which the editors have no place for anyway.
const purifyForEditor = DOMPurify(window)
purifyForEditor.setConfig({
  ...CONFIG,
  ALLOW_UNKNOWN_PROTOCOLS: true,
  USE_PROFILES: { html: true },
  SAFE_FOR_XML: false
})
purifyForEditor.addHook('uponSanitizeElement', unwrapMedia)

/**
 * HTML for display in the console page itself, such as a template block on the
 * editor canvas. Shown in an element styled with `containedContentStyle`.
 *
 * Event handlers, `<script>` and frames are removed, and so are `<style>`,
 * which would restyle the console around it, form controls, image maps, whose
 * areas are links the canvas would follow, audio and video, which would play in
 * it and give way to the fallback content they hold, and the email builder's
 * own `data-block-id`, which locates blocks on the canvas. A link or image
 * keeps its URL only if it is a web, mail or phone address, a relative URL or
 * Liquid, or, for an image, a `data:` URL: a link that opens another app
 * (`webcal:`, `whatsapp:`…) is shown without its target, and an image without
 * its `srcset`. Ids and names, and what refers to them (`for`, `headers`,
 * `aria-labelledby`…, `#id` links, SVG's `url(#id)`), are prefixed with
 * `user-content-`, and `tabindex` is dropped. Formatting, inline styles,
 * classes, `data-*` attributes, link targets and Liquid (`{{ url }}` in an
 * `href` included) are kept.
 */
export function sanitizeHtml(html: string): string {
  if (!html) return ''
  return purify.sanitize(html)
}

/**
 * HTML for a block editor to load: what `sanitizeHtml` keeps, with ids, names
 * and what refers to them as written, `tabindex`, `srcset`, links that open
 * another app (anything but a script or a `data:` URL), and also attribute
 * values `sanitizeHtml` drops because they contain `-->` or `]>`, such as a
 * link to `https://example.com/?q=a]>b`. An editor saves what it loaded, so
 * anything changed or dropped here that an email may legitimately carry would
 * be lost on the next edit. Still dropped: an id or a name that is also a
 * property of the document or of a form, such as `title` or `links`, so that it
 * cannot stand in for that property, and SVG and MathML, along with what they
 * contain.
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

/**
 * The web address an `mj-font` names for its stylesheet: an absolute http(s) URL,
 * a protocol-relative one (`//fonts.googleapis.com/…`) read as https. Liquid has
 * no address until the email is sent, so it gives null, unless `liquidTags` asks
 * to read an address that holds Liquid tags (`{% if %}`) as it is written.
 */
export function fontStylesheetAddress(
  href: string | null | undefined,
  { liquidTags = false }: { liquidTags?: boolean } = {}
): URL | null {
  // Only ASCII whitespace, as a link's address is trimmed
  const url = href?.replace(/^[\t\n\f\r ]+|[\t\n\f\r ]+$/g, '')
  if (!url || hasLiquidOutput(url) || (!liquidTags && isLiquidExpression(url))) return null
  try {
    const parsed = new URL(url.startsWith('//') ? `https:${url}` : url)
    return parsed.protocol === 'http:' || parsed.protocol === 'https:' ? parsed : null
  } catch {
    return null
  }
}

/**
 * The stylesheet an `mj-font` imports, as the console may link it or load it: its
 * address (see `fontStylesheetAddress`) on another origin than the console's,
 * normalised.
 */
export function fontStylesheetUrl(href: string | null | undefined): string | null {
  const address = fontStylesheetAddress(href)
  return address ? safeExternalUrl(address.toString()) : null
}

/**
 * Whether the console offers a URL as a link to follow, as the editor's side
 * panel does for a link target: an absolute web, mail or phone address.
 */
export function isOpenableUrl(url: string | null | undefined): boolean {
  if (!url) return false
  try {
    return OPENABLE_SCHEMES.includes(new URL(url).protocol.slice(0, -1))
  } catch {
    return false
  }
}

/**
 * Whether a saved link target can be put in an `href` on the canvas: the same
 * verdict `sanitizeHtml` gives a link in content. Web, mail and phone
 * addresses, relative URLs and Liquid (`{{ url }}`, `https://{{ host }}/x`)
 * pass; scripts, `data:` URLs and links that open another app do not.
 */
export function isSafeHref(href: string | null | undefined): boolean {
  if (!href) return false
  return purify.isValidAttribute('a', 'href', href)
}
