import { describe, expect, it } from 'vitest'
import {
  deliveredChange,
  lastSentByNotification,
  parseDaily,
  peakFailureDay,
  statsByNotification,
  statsOf,
  statsRates,
  sumStats,
  emptyStats
} from './transactionalStats'

const row = (id: string, values: Record<string, number | string>) => ({ transactional_notification_id: id, ...values })

describe('statsByNotification', () => {
  const map = statsByNotification([
    row('a', { count_sent: '100', count_delivered: 98, count_failed: 2, count_opened: 40, count_opens_tracked: 100 }),
    row('b', { count_sent: 50, count_delivered: 50, count_bounced: 1 })
  ])

  it('reads each row, numbers sent as strings included', () => {
    expect(statsOf(map, 'a')).toMatchObject({ sent: 100, delivered: 98, failed: 2, opened: 40, opensTracked: 100 })
  })

  it('reads a notification without a row as zeros', () => {
    expect(statsOf(map, 'missing')).toEqual(emptyStats())
    expect(statsOf(undefined, 'a')).toEqual(emptyStats())
  })

  it('sums the rows of the listed notifications only', () => {
    expect(sumStats(map, ['a', 'b', 'missing'])).toMatchObject({ sent: 150, delivered: 148, failed: 2, bounced: 1 })
    expect(sumStats(map, ['b']).sent).toBe(50)
  })
})

describe('statsRates', () => {
  it('has no rate without a denominator', () => {
    const rates = statsRates(emptyStats())
    expect(rates.delivered).toBeNull()
    expect(rates.bounced).toBeNull()
    expect(rates.failedShare).toBeNull()
    expect(rates.opens.rate).toBeNull()
    expect(rates.opens.notTracked).toBe(false)
  })

  it('counts failures over every attempt', () => {
    expect(statsRates({ ...emptyStats(), sent: 98, failed: 2 }).failedShare).toBe(0.02)
  })

  it('warns under 97 % delivered and from 2 % bounces', () => {
    expect(statsRates({ ...emptyStats(), sent: 100, delivered: 97 }).deliveredTone).toBe('neutral')
    expect(statsRates({ ...emptyStats(), sent: 100, delivered: 96 }).deliveredTone).toBe('warning')
    expect(statsRates({ ...emptyStats(), sent: 100, bounced: 1 }).bouncedTone).toBe('neutral')
    expect(statsRates({ ...emptyStats(), sent: 100, bounced: 2 }).bouncedTone).toBe('warning')
  })

  it('computes opens and clicks on the tracked emails, partial when fewer were tracked than sent', () => {
    const rates = statsRates({ ...emptyStats(), sent: 100, opened: 30, opensTracked: 60, clicked: 0, clicksTracked: 100 })
    expect(rates.opens).toEqual({ rate: 0.5, notTracked: false, partial: true, tracked: 60 })
    expect(rates.clicks).toEqual({ rate: 0, notTracked: false, partial: false, tracked: 100 })
  })

  it('says not tracked when emails left but none was tracked', () => {
    const rates = statsRates({ ...emptyStats(), sent: 10 })
    expect(rates.opens.notTracked).toBe(true)
    expect(rates.clicks.notTracked).toBe(true)
  })
})

describe('deliveredChange', () => {
  it('gives the change in points and its tone', () => {
    const change = deliveredChange({ sent: 100, delivered: 99 }, { sent: 100, delivered: 97 })
    expect(change?.points).toBeCloseTo(2)
    expect(change?.tone).toBe('positive')
    expect(deliveredChange({ sent: 100, delivered: 95 }, { sent: 100, delivered: 97 })?.tone).toBe('negative')
    expect(deliveredChange({ sent: 1000, delivered: 970 }, { sent: 1000, delivered: 970 })?.tone).toBe('neutral')
  })

  it('has no change without a previous period or a send', () => {
    expect(deliveredChange({ sent: 100, delivered: 99 }, undefined)).toBeNull()
    expect(deliveredChange({ sent: 100, delivered: 99 }, { sent: 0, delivered: 0 })).toBeNull()
    expect(deliveredChange({ sent: 0, delivered: 0 }, { sent: 10, delivered: 10 })).toBeNull()
  })
})

describe('lastSentByNotification', () => {
  it('keeps the instant, null when every send failed', () => {
    const map = lastSentByNotification([
      row('a', { last_sent_at: '2026-11-12T09:00:00Z' }),
      { transactional_notification_id: 'b', last_sent_at: null }
    ])
    expect(map.get('a')).toBe('2026-11-12T09:00:00Z')
    expect(map.get('b')).toBeNull()
    expect(map.has('c')).toBe(false)
  })
})

describe('daily series', () => {
  const points = parseDaily([
    { created_at_day: '2026-11-10T00:00:00Z', count_sent: 10, count_failed: 1 },
    { created_at_day: '2026-11-11T00:00:00Z', count_sent: 12, count_failed: 4, count_bounced: 1 },
    { created_at_day: '2026-11-12T00:00:00Z', count_sent: 3, count_failed: 4 }
  ])

  it('reads the day as the server wrote it', () => {
    expect(points.map((point) => point.day)).toEqual(['2026-11-10', '2026-11-11', '2026-11-12'])
    expect(points[1]).toEqual({ day: '2026-11-11', sent: 12, delivered: 0, bounced: 1, failed: 4 })
  })

  it('finds the first day with the most failures', () => {
    expect(peakFailureDay(points)).toBe('2026-11-11')
    expect(peakFailureDay(parseDaily([{ created_at_day: '2026-11-10T00:00:00Z', count_sent: 4 }]))).toBeNull()
  })
})
