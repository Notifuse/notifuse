import { describe, expect, it } from 'vitest'
import type { ContactTimelineEntry } from '../../services/api/contact_timeline'
import type { Workspace } from '../../services/api/types'
import { changeNew, changeOld, webPageURL } from './webPageUrl'

// The {field: {new: value}} envelope the analytics projection writes, which segment conditions read.
const webChange = (value: unknown) => ({ new: value })

const pageEntry: ContactTimelineEntry = {
  id: '22222222-2222-2222-2222-222222222222',
  email: 'reader@example.com',
  operation: 'insert',
  entity_type: 'web_page',
  kind: 'web.pageview',
  entity_id: 'sess-1:7:2',
  changes: { path: webChange('/docs/api'), duration_ms: webChange(62000) },
  created_at: '2026-08-12T10:01:00Z',
  db_created_at: '2026-08-12T10:01:00Z'
}

// The path and the domain are whatever the visitor's browser reported.
const pageAt = (path: string, landingDomain?: string): ContactTimelineEntry => ({
  ...pageEntry,
  changes: {
    ...pageEntry.changes,
    path: webChange(path),
    ...(landingDomain ? { landing_domain: webChange(landingDomain) } : {})
  }
})

// A workspace that lists the domains it tracks only receives visits from them, so a domain off
// that list was never one of its pages.
const tracking = (allowedDomains: string[], websiteURL?: string) =>
  ({
    settings: { website_url: websiteURL, web_analytics: { enabled: true, allowed_domains: allowedDomains } }
  }) as unknown as Workspace

const website = (url: string) => ({ settings: { website_url: url } }) as unknown as Workspace

describe('changeNew and changeOld', () => {
  it('read the envelope, and nothing from a value outside it', () => {
    const entry = { ...pageEntry, changes: { status: { old: 'pending', new: 'active' }, flat: 'x' } }
    expect(changeNew(entry, 'status')).toBe('active')
    expect(changeOld(entry, 'status')).toBe('pending')
    expect(changeNew(entry, 'flat')).toBeUndefined()
    expect(changeOld(entry, 'missing')).toBeUndefined()
  })
})

describe('webPageURL', () => {
  it('builds the page from the visit domain', () => {
    expect(webPageURL(pageAt('/docs/api', 'www.apple.com'), tracking(['www.apple.com']))).toBe(
      'https://www.apple.com/docs/api'
    )
  })

  it('falls back to the workspace website for rows projected without a domain', () => {
    // The domain was added to the projection after these rows were written, and a finished visit
    // is never re-projected.
    expect(webPageURL(pageEntry, website('https://shop.example.com'))).toBe('https://shop.example.com/docs/api')
    expect(webPageURL(pageEntry, website('shop.example.com'))).toBe('https://shop.example.com/docs/api')
    expect(webPageURL(pageEntry, tracking(['www.apple.com'], 'https://shop.example.com'))).toBe(
      'https://shop.example.com/docs/api'
    )
  })

  it('gives nothing when there is no domain to be had', () => {
    // Guessing a host would send someone to a page that was never visited.
    expect(webPageURL(pageEntry)).toBeNull()
    expect(webPageURL(pageAt('/pricing', 'www.apple.com'))).toBeNull()
  })

  it('keeps a path that starts with two slashes, or a full URL, on the visited site', () => {
    const workspace = tracking(['www.apple.com'])
    expect(webPageURL(pageAt('//other.example/x', 'www.apple.com'), workspace)).toBe(
      'https://www.apple.com//other.example/x'
    )
    expect(webPageURL(pageAt('https://www.apple.com/pricing?a=1', 'www.apple.com'), workspace)).toBe(
      'https://www.apple.com/pricing?a=1'
    )
  })

  it.each([
    'javascript:window.__probe=1',
    ' javascript:window.__probe=1',
    'data:text/html,<p>x</p>',
    'https://other.example/x',
    '\\\\other.example/x',
    ' //other.example/x'
  ])('gives nothing for a path that leaves the visited site: %j', (path) => {
    expect(webPageURL(pageAt(path, 'www.apple.com'), tracking(['www.apple.com']))).toBeNull()
  })

  it('gives nothing for a page on the console itself', () => {
    expect(webPageURL(pageAt('/console/workspace'), website(window.location.origin))).toBeNull()
  })

  it.each([
    [['www.apple.com'], 'www.apple.com'],
    [[' WWW.Apple.com '], 'www.apple.com'],
    [['www.apple.com:443'], 'www.apple.com'],
    [['*.apple.com'], 'www.apple.com'],
    [['*.apple.com'], 'apple.com'],
    [['other.example', 'www.apple.com'], 'www.apple.com'],
    // An IPv6 address is recorded without its brackets
    [['::1'], '::1'],
    [['[::1]:8443'], '::1'],
    [['10.0.0.5'], '10.0.0.5']
  ])('builds a page of a domain the workspace tracks: %j, %s', (allowedDomains, domain) => {
    const host = domain.includes(':') ? `[${domain}]` : domain
    expect(webPageURL(pageAt('/pricing', domain), tracking(allowedDomains))).toBe(`https://${host}/pricing`)
  })

  // With no list, the visitor's domain is vouched for by nothing but the workspace website
  it.each([
    ['https://WWW.apple.com/', 'www.apple.com'],
    ['https://apple.com', 'www.apple.com'],
    ['https://www.apple.com', 'apple.com'],
    ['apple.com', 'apple.com'],
    ['www.apple.com', 'apple.com']
  ])('builds a page of the workspace website when the workspace lists no domain: %s, %s', (url, domain) => {
    expect(webPageURL(pageAt('/pricing', domain), tracking([], url))).toBe(`https://${domain}/pricing`)
  })

  it.each(['other.example', 'shop.apple.com', 'localhost', 'localhost.', '127.0.0.1', '::1', 'intranet', 'nas.local'])(
    'gives nothing for any other domain when the workspace lists none: %s',
    (domain) => {
      expect(webPageURL(pageAt('/pricing', domain), tracking([], 'https://www.apple.com'))).toBeNull()
    }
  )

  it.each([
    [['www.apple.com'], 'other.example'],
    [['www.apple.com'], 'apple.com'],
    [['*.apple.com'], 'notapple.com'],
    [['*.com'], 'other.com'],
    // The server never matches a bracketed entry without a port either
    [['[::1]'], '::1']
  ])('gives nothing for a domain the workspace does not track: %j, %s', (allowed, domain) => {
    expect(webPageURL(pageAt('/pricing', domain), tracking(allowed))).toBeNull()
  })
})
