import type { ContactTimelineEntry } from '../../services/api/contact_timeline'
import type { Workspace } from '../../services/api/types'
import { safeExternalUrl } from '../../utils/safeHtml'

/**
 * A value of a timeline entry's changes. The web navigation rows are written by the analytics
 * projection, not by a database trigger, but they carry the same {field: {new: value}} envelope:
 * segment conditions on contact_timeline read changes->'<key>'->>'new', so any other shape would be
 * unreachable from a segment.
 */
export function changeNew<T>(entry: ContactTimelineEntry, key: string): T | undefined {
  const change = entry.changes?.[key]
  if (typeof change === 'object' && change !== null && 'new' in change) {
    return (change as { new: T }).new
  }
  return undefined
}

/** The value a change replaced, when the entry recorded one. */
export function changeOld<T>(entry: ContactTimelineEntry, key: string): T | undefined {
  const change = entry.changes?.[key]
  if (typeof change === 'object' && change !== null && 'old' in change) {
    return (change as { old: T }).old
  }
  return undefined
}

// Whether a hostname is one of the web analytics allowed domains, read the way
// the server reads them for incoming visits: case and spaces ignored, an IPv6
// address compared without its brackets, a numeric port dropped (`[::1]:8080`
// is `::1`), and `*.example.com` covering example.com and its subdomains but
// never a bare TLD.
const isTrackedHost = (hostname: string, allowedDomains: string[]): boolean => {
  const host = hostname.toLowerCase().replace(/^\[(.*)\]$/, '$1')
  return allowedDomains.some((entry) => {
    const trimmed = entry.trim().toLowerCase()
    const withPort = /^\[(.*)\]:\d+$/.exec(trimmed) ?? /^([^:]*):\d+$/.exec(trimmed)
    const allowed = withPort ? withPort[1] : trimmed
    if (!allowed) return false
    if (allowed.startsWith('*.')) {
      const wild = allowed.slice(2)
      return wild.includes('.') && (host === wild || host.endsWith(`.${wild}`))
    }
    return host === allowed
  })
}

// The workspace website, whose address the settings accept with or without
// its scheme (`example.com`)
const parseWebsiteURL = (websiteURL?: string): URL | null => {
  const address = websiteURL?.trim()
  if (!address) return null
  try {
    return new URL(/^https?:\/\//i.test(address) ? address : `https://${address}`)
  } catch {
    return null
  }
}

// Whether a hostname is the workspace website's, with or without `www.`
const isWebsiteHost = (hostname: string, website: URL | null): boolean => {
  if (!website) return false
  const host = website.hostname
  return hostname === host || hostname === `www.${host}` || `www.${hostname}` === host
}

/**
 * Rebuilds the address a pageview refers to. web_pages stores paths only, so the host comes from
 * the visit's landing domain, which the projection carries alongside the path.
 *
 * The workspace's own website URL is the fallback, for rows projected before the domain was
 * recorded. It is only a fallback: a workspace can track a site that is not the one configured in
 * its settings, and guessing wrong there sends someone to a page that was never visited.
 *
 * For the same reason the address must stay on that site. A path is appended to the site's origin
 * rather than resolved against it, so `//x/y` is a page of the site as it is for the browser that
 * reported it; anything else the SDK may have been given, a full URL included, is resolved and kept
 * only if it lands there.
 *
 * The domain is reported by the visitor's browser too, so it is trusted only as far as the
 * workspace vouches for it: one of its web analytics allowed domains, the only ones a workspace that
 * lists any receives visits from, or, with none listed, the host of its website, `www.` or not. A
 * domain taken off the list takes the previews of its earlier visits with it.
 */
export function webPageURL(entry: ContactTimelineEntry, workspace?: Workspace): string | null {
  const path = changeNew<string>(entry, 'path')
  if (!path) return null

  // An IPv6 address is recorded without the brackets a URL needs
  const domain = changeNew<string>(entry, 'landing_domain')?.replace(/^([^[].*:.*)$/, '[$1]')
  const website = parseWebsiteURL(workspace?.settings?.website_url)
  const origin = domain ? `https://${domain}` : website?.href
  if (!origin) return null

  try {
    const site = new URL(origin)
    const trackedDomains = workspace?.settings?.web_analytics?.allowed_domains ?? []
    const vouched =
      trackedDomains.length > 0
        ? isTrackedHost(site.hostname, trackedDomains)
        : isWebsiteHost(site.hostname, website)
    if (domain && !vouched) return null
    const page = path.startsWith('/') ? new URL(site.origin + path) : new URL(path, site)
    if (page.origin !== site.origin) return null
    return safeExternalUrl(page.toString())
  } catch {
    return null
  }
}
