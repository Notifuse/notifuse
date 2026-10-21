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
 * HTML that shows `text` as it is: what would read as markup is escaped.
 */
export function textToHtml(text: string): string {
  const container = parseHtmlInert('')
  container.textContent = text
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

// What DOMPurify removes before reading a link's scheme. A browser ignores some
// of these around or inside a scheme; refusing on all of them costs nothing.
// eslint-disable-next-line no-control-regex -- control characters are the point
const URL_WHITESPACE = /[\u0000-\u0020\u00A0\u1680\u180E\u2000-\u2029\u205F\u3000]/g
const SCRIPT_OR_DATA_URL = /^(?:\w+script|data):/i

/**
 * Whether a saved link target can be put in an `href` in the console: anything
 * but a script (`javascript:`, `vbscript:`…) or `data:` URL, the rule
 * `sanitizeHtml` applies to links in content. Web, mail, phone and app links,
 * relative ones and Liquid (`{{ url }}`, `https://{{ host }}/x`) all pass.
 */
export function isSafeHref(href: string | null | undefined): boolean {
  if (!href) return false
  return !SCRIPT_OR_DATA_URL.test(href.replace(URL_WHITESPACE, ''))
}
