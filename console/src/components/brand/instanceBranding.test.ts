import { describe, it, expect, afterEach } from 'vitest'
import {
  getInstanceBranding,
  getBrandName,
  brandInitial,
  DEFAULT_INSTANCE_BRANDING
} from './instanceBranding'
import { useBrandName } from '../../hooks/useBrandName'

const acme: InstanceBrandingGlobal = {
  custom: true,
  name: 'Acme',
  logo_url: 'https://cdn.example.com/l.png',
  icon_url: '',
  hide_help_menu: true,
  splash_logo_src: 'https://cdn.example.com/l.png',
  favicon_src: '/console/neutral-icon.png',
  revision: 'abc'
}

afterEach(() => {
  delete window.INSTANCE_BRANDING
})

describe('getInstanceBranding', () => {
  it('falls back to Notifuse when config.js carries no brand', () => {
    expect(getInstanceBranding()).toEqual(DEFAULT_INSTANCE_BRANDING)
    expect(getBrandName()).toBe('Notifuse')
    expect(useBrandName()).toBe('Notifuse')
  })

  it('returns what the server resolved', () => {
    window.INSTANCE_BRANDING = acme
    expect(getInstanceBranding()).toEqual(acme)
    expect(getBrandName()).toBe('Acme')
    expect(useBrandName()).toBe('Acme')
  })

  it('fills fields an older server did not send', () => {
    window.INSTANCE_BRANDING = { name: 'Acme', custom: true } as InstanceBrandingGlobal
    expect(getInstanceBranding().hide_help_menu).toBe(false)
    expect(getInstanceBranding().favicon_src).toBe(DEFAULT_INSTANCE_BRANDING.favicon_src)
  })

  it('never prints an empty name', () => {
    window.INSTANCE_BRANDING = { ...acme, name: '' }
    expect(getBrandName()).toBe('Notifuse')
  })
})

describe('brandInitial', () => {
  it('takes the first grapheme, upper-cased', () => {
    expect(brandInitial('acme')).toBe('A')
    expect(brandInitial('  émile')).toBe('É')
    expect(brandInitial('👩‍💻 Labs')).toBe('👩‍💻')
  })

  it('answers "?" for an empty name', () => {
    expect(brandInitial('')).toBe('?')
    expect(brandInitial('   ')).toBe('?')
  })

  it('keeps a whole code point without Intl.Segmenter', () => {
    expect(brandInitial('acme', null)).toBe('A')
    expect(brandInitial('😀 Labs', null)).toBe('😀')
    // Without grapheme segmentation, a composed emoji loses its joined part: accepted.
    expect(brandInitial('👩‍💻 Labs', null)).toBe('👩')
  })
})
