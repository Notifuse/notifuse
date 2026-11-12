import { describe, it, expect } from 'vitest'
import dayjs from '../../lib/dayjs'
import { broadcastVisits, kpiTraffic } from './attribution'
import type { CampaignDayRow } from './broadcastAnalytics'
import type { Broadcast } from '../../services/api/broadcast'

const now = dayjs('2026-11-12T12:00:00Z')

const bc = (over: Partial<Broadcast>): Broadcast =>
  ({ id: 'b', status: 'processed', utm_parameters: { campaign: 'news' }, ...over }) as unknown as Broadcast

const row = (campaign: string, day: string, sessions: number, conversions = 0, value = 0): CampaignDayRow => ({
  campaign,
  day,
  sessions,
  conversions,
  value
})

describe('broadcastVisits (list column)', () => {
  const rows = [
    row('news', '2026-10-31', 50),
    row('news', '2026-11-01', 10),
    row('news', '2026-11-05', 5),
    row('news', '2026-11-09', 7),
    row('other', '2026-11-01', 99)
  ]

  it('sums the sessions of its campaign inside its attribution window only', () => {
    // Started 1 Nov, completed 1 Nov: the window runs 1 Nov to 8 Nov.
    const b = bc({ started_at: '2026-11-01T09:00:00Z', completed_at: '2026-11-01T10:00:00Z' })
    expect(broadcastVisits(b, rows, 'UTC', now)).toBe(15)
  })

  it('is null without a campaign', () => {
    const b = bc({ started_at: '2026-11-01T09:00:00Z', utm_parameters: { campaign: '' } })
    expect(broadcastVisits(b, rows, 'UTC', now)).toBeNull()
    expect(broadcastVisits(bc({ started_at: '2026-11-01T09:00:00Z', utm_parameters: undefined }), rows, 'UTC', now)).toBeNull()
  })

  it('is null for a send older than 90 days, or one that has not started', () => {
    expect(broadcastVisits(bc({ started_at: '2026-07-01T09:00:00Z' }), rows, 'UTC', now)).toBeNull()
    expect(broadcastVisits(bc({ started_at: undefined }), rows, 'UTC', now)).toBeNull()
  })

  it('is 0 when its campaign had no visit', () => {
    expect(broadcastVisits(bc({ started_at: '2026-11-01T09:00:00Z' }), [], 'UTC', now)).toBe(0)
  })
})

describe('kpiTraffic (Website traffic KPI)', () => {
  const period = { startDay: '2026-10-13', endDay: '2026-11-11' }

  it('counts a day once for two broadcasts of one campaign', () => {
    const broadcasts = [
      bc({ id: 'a', started_at: '2026-11-01T09:00:00Z', completed_at: '2026-11-01T10:00:00Z' }),
      bc({ id: 'b', started_at: '2026-11-03T09:00:00Z', completed_at: '2026-11-03T10:00:00Z' })
    ]
    const rows = [row('news', '2026-11-04', 10, 2, 30)]
    expect(kpiTraffic(broadcasts, rows, period, 'UTC', now)).toEqual({ sessions: 10, conversions: 2, value: 30 })
  })

  it('drops the days outside every window and outside the period', () => {
    const broadcasts = [bc({ started_at: '2026-11-01T09:00:00Z', completed_at: '2026-11-01T10:00:00Z' })]
    const rows = [
      row('news', '2026-10-30', 100), // before the window
      row('news', '2026-11-02', 4),
      row('news', '2026-11-12', 100), // after the period (today)
      row('other', '2026-11-02', 100) // another campaign
    ]
    expect(kpiTraffic(broadcasts, rows, period, 'UTC', now).sessions).toBe(4)
  })

  it('counts nothing in the period for a broadcast started today', () => {
    const broadcasts = [bc({ started_at: '2026-11-12T08:00:00Z' })]
    expect(kpiTraffic(broadcasts, [row('news', '2026-11-12', 9)], period, 'UTC', now).sessions).toBe(0)
  })

  it('ignores broadcasts without a campaign', () => {
    const broadcasts = [bc({ started_at: '2026-11-01T09:00:00Z', utm_parameters: { campaign: '' } })]
    expect(kpiTraffic(broadcasts, [row('', '2026-11-02', 9)], period, 'UTC', now).sessions).toBe(0)
  })
})
