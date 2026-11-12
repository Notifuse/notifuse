import { describe, it, expect } from 'vitest'
import { i18n } from '@lingui/core'
import type { ContactVisitSession } from '../../../services/api/web_analytics'
import { deviceIcon, visitSource } from './webVisitSource'

const session = (patch: Partial<ContactVisitSession>): ContactVisitSession => ({
  id: 's1',
  session_date: '2026-11-08',
  created_at: '2026-11-08T10:00:00Z',
  duration_ms: 1000,
  pageview_count: 1,
  landing_path: '/',
  exit_path: '/',
  channel: '',
  channel_group: '',
  referrer_domain: '',
  utm_source: '',
  utm_campaign: '',
  device: 'desktop',
  os: '',
  browser: '',
  country: '',
  goal_count: 0,
  goal_value: 0,
  first_goal_name: null,
  ...patch
})

describe('visitSource', () => {
  it.each([
    [{ channel_group: 'email', utm_campaign: 'launch-v30' }, 'mail', 'launch-v30'],
    [{ channel_group: 'email' }, 'mail', 'Email'],
    [{ channel_group: 'search-organic', referrer_domain: 'google.com' }, 'search', 'google.com'],
    [{ channel_group: 'search-organic' }, 'search', 'Organic search'],
    [{ channel_group: 'search-paid' }, 'search', 'Paid search'],
    [{ channel_group: 'social-organic', referrer_domain: 'linkedin.com' }, 'social', 'linkedin.com'],
    [{ channel_group: 'social-organic' }, 'social', 'Organic social'],
    [{ channel_group: 'social-paid' }, 'social', 'Paid social'],
    [{ channel_group: 'display-banner', referrer_domain: 'ads.example' }, 'display', 'Display'],
    [{ channel_group: 'video-paid' }, 'video', 'Paid video'],
    [{ channel_group: 'direct' }, 'direct', 'Direct'],
    [
      { channel_group: 'not-mapped', referrer_domain: 'news.ycombinator.com' },
      'other',
      'news.ycombinator.com'
    ],
    [{ channel_group: 'not-mapped' }, 'other', 'Other'],
    [{ channel_group: 'partners' }, 'other', 'partners']
  ] as const)('reads %o as %s "%s"', (patch, icon, label) => {
    expect(visitSource(session(patch), i18n)).toMatchObject({ icon, label })
  })

  it('describes the channel and the UTM source below', () => {
    expect(
      visitSource(session({ channel_group: 'email', channel: 'Email', utm_source: 'newsletter' }), i18n)
        .detail
    ).toBe('Email · newsletter')
    expect(visitSource(session({ channel_group: 'direct', channel: 'Direct' }), i18n).detail).toBe('Direct')
  })
})

describe('deviceIcon', () => {
  it.each([
    ['desktop', 'desktop'],
    ['mobile', 'mobile'],
    ['tablet', 'tablet'],
    ['tv', 'other'],
    ['', 'other']
  ])('reads %s as %s', (device, icon) => {
    expect(deviceIcon(device)).toBe(icon)
  })
})
