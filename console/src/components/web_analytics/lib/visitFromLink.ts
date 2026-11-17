import { DEFAULT_AD_CLICK_IDS } from './adClickIds'

/** The UTM parameters the SDK reads from the address, by their exact (case-sensitive) name. */
const UTM_PARAMS = ['utm_source', 'utm_medium', 'utm_campaign', 'utm_term', 'utm_content'] as const

/** A scheme typed before the host ("https://", "http://"). */
const HAS_SCHEME = /^[a-z][a-z0-9+.-]*:\/\//i

/** A % not followed by two hex digits, which Go's url.Parse refuses in the path or the fragment. */
const MALFORMED_ESCAPE = /%(?![0-9a-f]{2})/i

/**
 * Whether the server cannot read the address: Go's url.Parse refuses a malformed escape in the path
 * or the fragment (the query is not checked), and webURLParts then stores an empty landing domain
 * and path.
 */
export function serverRejectsLink(url: URL): boolean {
  return MALFORMED_ESCAPE.test(url.pathname) || MALFORMED_ESCAPE.test(url.hash)
}

/**
 * Go's url.Parse decodes the path. A byte sequence that is not UTF-8 (%E9 alone) is kept as typed,
 * as the server escapes it back.
 */
function decodePath(pathname: string): string {
  try {
    return decodeURIComponent(pathname)
  } catch {
    return pathname
  }
}

/**
 * The Pages and UTM fields a visit landing on `raw` carries, as the SDK and the server fill them:
 * the SDK sends the address as the browser holds it and takes the UTMs and the first ad click id
 * of its default list from it (web_analytics_sdk/src/utils/utm.ts), and the server derives the
 * landing domain and path (internal/service/web_analytics_enrichment.go), both empty when Go cannot
 * parse the address (a malformed escape such as /100%/off). Every field is returned,
 * '' when the address has none, so a previous link leaves nothing behind. A link without a scheme
 * is read as https; null when it is not an http(s) address.
 */
export function visitFromLink(raw: string): Record<string, string> | null {
  const input = raw.trim()
  if (!input) return null
  let url: URL
  try {
    url = new URL(HAS_SCHEME.test(input) ? input : `https://${input}`)
  } catch {
    return null
  }
  if ((url.protocol !== 'http:' && url.protocol !== 'https:') || !url.hostname) return null

  const unreadable = serverRejectsLink(url)
  // The session's path is its landing path, empty as well when the server could not read it.
  const path = unreadable ? '' : decodePath(url.pathname) || '/'
  const visit: Record<string, string> = {
    landing_page: url.href,
    landing_domain: unreadable ? '' : url.hostname.toLowerCase(),
    landing_path: path,
    path
  }
  for (const param of UTM_PARAMS) visit[param] = url.searchParams.get(param) ?? ''

  // Click id keys are matched whatever their case, and reported in the list's spelling; the
  // list's order, not the address's, decides which one wins. A utm_id parameter is ignored, as
  // the SDK ignores it.
  const byLowerKey = new Map<string, string>()
  for (const [key, value] of url.searchParams) {
    const lower = key.toLowerCase()
    if (!byLowerKey.has(lower)) byLowerKey.set(lower, value)
  }
  visit.utm_id = ''
  visit.utm_id_from = ''
  for (const param of DEFAULT_AD_CLICK_IDS) {
    const value = byLowerKey.get(param.toLowerCase())
    if (value) {
      visit.utm_id = value
      visit.utm_id_from = param
      break
    }
  }
  return visit
}
