import DOMPurify from 'dompurify'

// An instance of its own, so that the configuration below applies to every
// call here, isSafeHref's included, and to nothing else on the page that uses
// DOMPurify.
const purify = DOMPurify(window)
purify.setConfig({
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
})

/**
 * HTML for display in the console page itself, such as a template block on the
 * editor canvas or the content an editor loads.
 *
 * Event handlers, `<script>`, frames and `javascript:`, `vbscript:` and `data:`
 * links are removed, and so are `<style>`, which would restyle the console
 * around it, form controls, image maps, whose areas are links the canvas would
 * follow, audio and video, which would play in it, and the email builder's own
 * `data-block-id`, which locates blocks on the canvas. Formatting,
 * inline styles, classes, `data-*` attributes, link targets, links that open
 * another app (`webcal:`, `whatsapp:`…) and Liquid (`{{ url }}` in an `href`
 * included) are kept. An editor saves what it loaded, so anything dropped here
 * that an email may legitimately carry would be lost on the next edit.
 */
export function sanitizeHtml(html: string): string {
  if (!html) return ''
  return purify.sanitize(html)
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
