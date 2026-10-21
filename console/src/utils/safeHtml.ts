import DOMPurify from 'dompurify'

/**
 * HTML for display in the console page itself, such as a template block on the
 * editor canvas or the content an editor loads.
 *
 * Event handlers, `<script>`, frames and `javascript:`, `vbscript:` and `data:`
 * links are removed, and so are `<style>`, which would restyle the console
 * around it, and form controls, which have no place in an email. Formatting,
 * inline styles, classes, `data-*` attributes, link targets, links that open
 * another app (`webcal:`, `whatsapp:`…) and Liquid (`{{ url }}` in an `href`
 * included) are kept. An editor saves what it loaded, so anything dropped here
 * that an email may legitimately carry would be lost on the next edit.
 */
export function sanitizeHtml(html: string): string {
  if (!html) return ''
  return DOMPurify.sanitize(html, {
    ADD_ATTR: ['target'],
    ALLOW_UNKNOWN_PROTOCOLS: true,
    FORBID_TAGS: ['style', 'form', 'input', 'button', 'select', 'option', 'textarea']
  })
}

/**
 * Parses HTML in a document that has no browsing context, for code that reads
 * or rewrites markup without rendering it.
 *
 * Nothing in such a document is fetched or run, unlike an element created with
 * `document.createElement`, which belongs to the page even while detached. Any
 * node added to the result must come from `container.ownerDocument`: a node of
 * the page's document, a fragment included, takes what is moved into it back
 * into the page.
 */
export function parseHtmlInert(html: string): HTMLDivElement {
  const doc = document.implementation.createHTMLDocument('')
  const container = doc.createElement('div')
  container.innerHTML = html
  return container
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

const SAFE_HREF_PROTOCOLS = new Set(['http:', 'https:', 'mailto:', 'tel:'])

/**
 * Whether a saved link target can be put in an `href` in the console: a web,
 * mail or phone link, or a relative one, which covers anchors and Liquid
 * (`{{ url }}`).
 *
 * The value is resolved the way the browser would resolve it, so case,
 * whitespace and control characters around or inside the scheme change nothing.
 */
export function isSafeHref(href: string | null | undefined): boolean {
  if (!href) return false
  try {
    return SAFE_HREF_PROTOCOLS.has(new URL(href, 'https://relative.invalid/').protocol)
  } catch {
    return false
  }
}
