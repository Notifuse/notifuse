import { describe, expect, it } from 'vitest'
import { visitFromLink } from './visitFromLink'

describe('visitFromLink', () => {
  it('fills the Pages and UTM fields as the SDK and the server would', () => {
    expect(
      visitFromLink('https://Shop.Example.com/iphone/?utm_source=google&utm_medium=cpc&fbclid=x&GCLID=y')
    ).toEqual({
      // The address as the browser holds it: lowercase host.
      landing_page: 'https://shop.example.com/iphone/?utm_source=google&utm_medium=cpc&fbclid=x&GCLID=y',
      landing_domain: 'shop.example.com',
      landing_path: '/iphone/',
      path: '/iphone/',
      utm_source: 'google',
      utm_medium: 'cpc',
      utm_campaign: '',
      utm_term: '',
      utm_content: '',
      // gclid comes before fbclid in the list, whatever the order in the address.
      utm_id: 'y',
      utm_id_from: 'gclid'
    })
  })

  it('matches a click id whatever its case and reports the canonical spelling', () => {
    const visit = visitFromLink('https://example.com/?sccid=abc')
    expect(visit?.utm_id).toBe('abc')
    expect(visit?.utm_id_from).toBe('ScCid')
  })

  it('lets the list order decide between two click ids', () => {
    expect(visitFromLink('https://example.com/?qclid=1&msclkid=2')?.utm_id_from).toBe('msclkid')
  })

  it('leaves the click id empty when there is none, and ignores a utm_id parameter', () => {
    const visit = visitFromLink('https://example.com/landing?utm_id=campaign-7')
    expect(visit?.utm_id).toBe('')
    expect(visit?.utm_id_from).toBe('')
  })

  it('reads UTM keys case-sensitively, as URLSearchParams.get does', () => {
    expect(visitFromLink('https://example.com/?UTM_SOURCE=google')?.utm_source).toBe('')
  })

  it('reads an empty path as /', () => {
    const visit = visitFromLink('https://example.com')
    expect(visit?.landing_path).toBe('/')
    expect(visit?.path).toBe('/')
  })

  it('decodes the path as the server does', () => {
    expect(visitFromLink('https://example.com/caf%C3%A9')?.landing_path).toBe('/café')
  })

  it('leaves the landing domain and path empty when the server cannot parse the address', () => {
    // Go's url.Parse refuses a malformed escape in the path or the fragment, and webURLParts then
    // stores both empty; the UTMs come from the browser, which reads them all the same.
    for (const link of ['https://example.com/100%/off?utm_source=x', 'https://example.com/a%2?utm_source=x', 'https://example.com/a?utm_source=x#100%']) {
      const visit = visitFromLink(link)
      expect(visit).toMatchObject({ landing_domain: '', landing_path: '', path: '', utm_source: 'x' })
      expect(visit?.landing_page).toBe(link)
    }
    // The query is not checked.
    expect(visitFromLink('https://example.com/a?off=100%')?.landing_path).toBe('/a')
  })

  it('accepts a link without its scheme', () => {
    const visit = visitFromLink('example.com/pricing?utm_medium=email')
    expect(visit?.landing_page).toBe('https://example.com/pricing?utm_medium=email')
    expect(visit?.landing_domain).toBe('example.com')
    expect(visit?.utm_medium).toBe('email')
  })

  it('answers null for what is not a web address', () => {
    expect(visitFromLink('not a link')).toBeNull()
    expect(visitFromLink('   ')).toBeNull()
    expect(visitFromLink('ftp://example.com/file')).toBeNull()
  })
})
