import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { resolveDashboardPeriods } from './dashboardPeriods'

describe('resolveDashboardPeriods', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    // 11 November 2026, 10:00 in Paris (UTC+1)
    vi.setSystemTime(new Date('2026-11-11T09:00:00Z'))
  })
  afterEach(() => vi.useRealTimers())

  it('ends 30 days yesterday and compares with the 30 days before', () => {
    const periods = resolveDashboardPeriods('30d', 'Europe/Paris')
    expect(periods.days).toBe(30)
    expect(periods.preset).toBe('previous_30_days')
    expect(periods.current.startDay).toBe('2026-10-12')
    expect(periods.current.endDay).toBe('2026-11-10')
    expect(periods.previous.startDay).toBe('2026-09-12')
    expect(periods.previous.endDay).toBe('2026-10-11')
    // Paris is UTC+2 until 25 October, UTC+1 after
    expect(periods.current.startUtc).toBe('2026-10-11T22:00:00.000Z')
    expect(periods.current.endUtc).toBe('2026-11-10T22:59:59.999Z')
    expect(periods.previous.startUtc).toBe('2026-09-11T22:00:00.000Z')
    expect(periods.previous.endUtc).toBe('2026-10-11T21:59:59.999Z')
  })

  it('keeps every day across the end of summer time', () => {
    const { current } = resolveDashboardPeriods('30d', 'Europe/Paris')
    const start = Date.parse(`${current.startDay}T00:00:00Z`)
    const end = Date.parse(`${current.endDay}T00:00:00Z`)
    expect((end - start) / 86_400_000 + 1).toBe(30)
  })

  it('resolves 7 and 90 days', () => {
    const week = resolveDashboardPeriods('7d', 'Europe/Paris')
    expect([week.current.startDay, week.current.endDay]).toEqual(['2026-11-04', '2026-11-10'])
    expect([week.previous.startDay, week.previous.endDay]).toEqual(['2026-10-28', '2026-11-03'])
    const quarter = resolveDashboardPeriods('90d', 'Europe/Paris')
    expect(quarter.days).toBe(90)
    expect([quarter.current.startDay, quarter.current.endDay]).toEqual(['2026-08-13', '2026-11-10'])
  })

  it('falls back to UTC without a timezone', () => {
    const { current } = resolveDashboardPeriods('7d', '')
    expect(current.startUtc).toBe('2026-11-04T00:00:00.000Z')
    expect(current.endUtc).toBe('2026-11-10T23:59:59.999Z')
  })

  describe('recent', () => {
    it('covers the same number of days ending today, in the workspace time zone', () => {
      const { recent } = resolveDashboardPeriods('30d', 'Europe/Paris')
      expect([recent.startDay, recent.endDay]).toEqual(['2026-10-13', '2026-11-11'])
      expect(recent.startUtc).toBe('2026-10-12T22:00:00.000Z')
      expect(recent.endUtc).toBe('2026-11-11T22:59:59.999Z')
      const week = resolveDashboardPeriods('7d', 'Europe/Paris').recent
      expect([week.startDay, week.endDay]).toEqual(['2026-11-05', '2026-11-11'])
    })

    it('moves to the next day just after local midnight', () => {
      // 00:05 on 12 November in Paris, still 11 November in UTC
      vi.setSystemTime(new Date('2026-11-11T23:05:00Z'))
      const { recent } = resolveDashboardPeriods('7d', 'Europe/Paris')
      expect([recent.startDay, recent.endDay]).toEqual(['2026-11-06', '2026-11-12'])
      expect(recent.endUtc).toBe('2026-11-12T22:59:59.999Z')
    })

    it('keeps whole local days across the end of summer time', () => {
      // 25 October is 25 hours long in Paris
      vi.setSystemTime(new Date('2026-10-27T09:00:00Z'))
      const { recent } = resolveDashboardPeriods('7d', 'Europe/Paris')
      expect([recent.startDay, recent.endDay]).toEqual(['2026-10-21', '2026-10-27'])
      expect(recent.startUtc).toBe('2026-10-20T22:00:00.000Z')
      expect(recent.endUtc).toBe('2026-10-27T22:59:59.999Z')
    })

    it('falls back to UTC without a timezone', () => {
      const { recent } = resolveDashboardPeriods('7d', '')
      expect(recent.startUtc).toBe('2026-11-05T00:00:00.000Z')
      expect(recent.endUtc).toBe('2026-11-11T23:59:59.999Z')
    })
  })
})
