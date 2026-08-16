import { describe, expect, it } from 'vitest'
import { defaultParseSearch, defaultStringifySearch } from '@tanstack/router-core'
import dayjs from '../../../lib/dayjs'
import {
  BROADCAST_CONVERSION_WINDOW_DAYS,
  BROADCAST_EXPLORE_DIMENSIONS,
  BROADCAST_MIN_SESSIONS,
  WEB_ANALYTICS_TAB_ROUTE,
  buildBroadcastAnalyticsLinks
} from './broadcastLinks'

const NOW = dayjs('2026-08-20T12:00:00Z')

function build(overrides: Partial<Parameters<typeof buildBroadcastAnalyticsLinks>[0]> = {}) {
  return buildBroadcastAnalyticsLinks({
    workspaceId: 'ws1',
    campaign: 'weekly_newsletter_4',
    startedAt: '2026-08-10T09:30:00Z',
    completedAt: '2026-08-10T10:15:00Z',
    timezone: 'UTC',
    now: NOW,
    ...overrides
  })
}

describe('link targets', () => {
  it('sends traffic to explore and conversions to goals', () => {
    const { traffic, conversions } = build()
    expect(traffic.to).toBe(WEB_ANALYTICS_TAB_ROUTE)
    expect(conversions.to).toBe(WEB_ANALYTICS_TAB_ROUTE)
    expect(traffic.params).toEqual({ workspaceId: 'ws1', tab: 'explore' })
    expect(conversions.params).toEqual({ workspaceId: 'ws1', tab: 'goals' })
  })

  it('scopes both reports to the same campaign', () => {
    const { traffic, conversions } = build()
    expect(traffic.search.filters).toBe(conversions.search.filters)
    // The param is a JSON *string*; an object here is the double-encoding bug.
    expect(typeof traffic.search.filters).toBe('string')
    expect(JSON.parse(traffic.search.filters as string)).toEqual([
      { dimension: 'utm_campaign', operator: 'equals', values: ['weekly_newsletter_4'] }
    ])
  })

  it('gives explore a drill order and a reachable session floor', () => {
    const { traffic } = build()
    expect(traffic.search.dimensions).toBe(BROADCAST_EXPLORE_DIMENSIONS)
    expect(traffic.search.minSessions).toBe(BROADCAST_MIN_SESSIONS)
    expect(traffic.search.comparison).toBe('none')
  })

  it('omits the explore-only params from the goals link', () => {
    const { conversions } = build()
    expect(conversions.search).not.toHaveProperty('dimensions')
    expect(conversions.search).not.toHaveProperty('minSessions')
    expect(conversions.search.comparison).toBe('none')
  })
})

describe('variation scope', () => {
  it('filters on the campaign alone when no variation is given', () => {
    const { traffic } = build()
    expect(JSON.parse(traffic.search.filters as string)).toHaveLength(1)
  })

  it('narrows to one variation by its utm_content', () => {
    const { traffic, conversions } = build({ content: 'newsletter-weekly-v2' })
    for (const target of [traffic, conversions]) {
      expect(JSON.parse(target.search.filters as string)).toEqual([
        { dimension: 'utm_campaign', operator: 'equals', values: ['weekly_newsletter_4'] },
        { dimension: 'utm_content', operator: 'equals', values: ['newsletter-weekly-v2'] }
      ])
    }
  })

  it('leaves the window and drill order untouched by the variation filter', () => {
    const whole = build()
    const variation = build({ content: 'newsletter-weekly-v2' })
    expect(variation.traffic.search.customStart).toBe(whole.traffic.search.customStart)
    expect(variation.traffic.search.customEnd).toBe(whole.traffic.search.customEnd)
    expect(variation.traffic.search.dimensions).toBe(whole.traffic.search.dimensions)
  })

  it('ignores an empty variation rather than filtering on an empty content', () => {
    const { traffic } = build({ content: '' })
    expect(JSON.parse(traffic.search.filters as string)).toHaveLength(1)
  })
})

describe('send window', () => {
  it('always carries both custom bounds', () => {
    for (const target of Object.values(build())) {
      expect(target.search.period).toBe('custom')
      expect(target.search.customStart).toBeTruthy()
      expect(target.search.customEnd).toBeTruthy()
    }
  })

  it('runs from the send day to the end of the conversion window', () => {
    const { traffic } = build()
    expect(traffic.search.customStart).toBe('2026-08-10')
    expect(traffic.search.customEnd).toBe(
      dayjs('2026-08-10T10:15:00Z').add(BROADCAST_CONVERSION_WINDOW_DAYS, 'day').format('YYYY-MM-DD')
    )
  })

  it('measures the window from now while the broadcast is still sending', () => {
    const { traffic } = build({ completedAt: null })
    // now + 7 days is in the future, so the range is capped at today.
    expect(traffic.search.customEnd).toBe('2026-08-20')
  })

  it('never asks for a future day', () => {
    const { traffic } = build({ completedAt: '2026-08-19T08:00:00Z' })
    expect(traffic.search.customEnd).toBe('2026-08-20')
  })

  it('keeps a same-day send from ending before it started', () => {
    const { traffic } = build({
      startedAt: '2026-08-20T09:00:00Z',
      completedAt: '2026-08-20T09:05:00Z'
    })
    expect(traffic.search.customStart).toBe('2026-08-20')
    expect(traffic.search.customEnd).toBe('2026-08-20')
  })

  it('reads the send day in the workspace timezone, not UTC', () => {
    // 22:30 UTC is already the 11th in Auckland.
    const { traffic } = build({
      startedAt: '2026-08-10T22:30:00Z',
      timezone: 'Pacific/Auckland'
    })
    expect(traffic.search.customStart).toBe('2026-08-11')
  })
})

describe('campaign encoding', () => {
  it('carries a campaign that needs escaping through untouched', () => {
    const campaign = 'Été "50%" sale & more'
    const { traffic } = build({ campaign })
    expect(JSON.parse(traffic.search.filters as string)[0].values).toEqual([campaign])
  })

  it('survives the router round-trip the console reads it back through', () => {
    // The trap this pins: TanStack re-stringifies a search value that parses as
    // JSON, so `filters` is stored double-encoded. Building the query string by
    // hand instead produces a value that parses back as an array and is then
    // discarded, leaving an unfiltered report and no error.
    const { traffic } = build()
    const parsed = defaultParseSearch(defaultStringifySearch(traffic.search)) as Record<
      string,
      unknown
    >
    expect(parsed.filters).toBe(traffic.search.filters)
    expect(JSON.parse(parsed.filters as string)).toEqual([
      { dimension: 'utm_campaign', operator: 'equals', values: ['weekly_newsletter_4'] }
    ])
  })
})
