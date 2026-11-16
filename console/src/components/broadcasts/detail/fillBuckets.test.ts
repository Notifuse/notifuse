import { describe, it, expect } from 'vitest'
import dayjs from '../../../lib/dayjs'
import { bucketKey, fillBuckets, serverBucketKey } from './fillBuckets'

describe('bucketKey', () => {
  it('writes the local hour with a Z, as the server does', () => {
    expect(bucketKey('2026-11-10T08:30:00Z', 'hour', 'Europe/Paris')).toBe('2026-11-10T09:00:00Z')
    expect(bucketKey('2026-11-10T08:30:00Z', 'hour', 'America/New_York')).toBe('2026-11-10T03:00:00Z')
    expect(bucketKey('2026-11-10T02:30:00Z', 'day', 'America/New_York')).toBe('2026-11-09T00:00:00Z')
  })
})

describe('serverBucketKey', () => {
  it('reads the server value as text, never converting it', () => {
    expect(serverBucketKey('2026-11-10T09:00:00Z', 'hour')).toBe('2026-11-10T09:00:00Z')
    expect(serverBucketKey('2026-11-10T09:00:00.000Z', 'hour')).toBe('2026-11-10T09:00:00Z')
    expect(serverBucketKey('2026-11-10T00:00:00Z', 'day')).toBe('2026-11-10T00:00:00Z')
    expect(serverBucketKey(null, 'hour')).toBeNull()
  })
})

describe('fillBuckets', () => {
  const rows = [
    { opened_at_hour: '2026-11-10T09:00:00Z', count_opened: 5 },
    { opened_at_hour: '2026-11-10T11:00:00Z', count_opened: '2' }
  ]

  it('fills every hour of the window with 0 where the server has nothing (Europe/Paris)', () => {
    const buckets = fillBuckets({
      start: dayjs('2026-11-10T08:00:00Z'),
      end: dayjs('2026-11-10T11:00:00Z'),
      granularity: 'hour',
      timezone: 'Europe/Paris',
      rows,
      column: 'opened_at_hour',
      measure: 'count_opened',
      now: dayjs('2026-11-12T00:00:00Z')
    })
    expect(buckets).toEqual([
      { key: '2026-11-10T09:00:00Z', value: 5 },
      { key: '2026-11-10T10:00:00Z', value: 0 },
      { key: '2026-11-10T11:00:00Z', value: 2 },
      { key: '2026-11-10T12:00:00Z', value: 0 }
    ])
  })

  it('keys the hours of New York in local time', () => {
    const buckets = fillBuckets({
      start: dayjs('2026-11-10T14:00:00Z'),
      end: dayjs('2026-11-10T15:00:00Z'),
      granularity: 'hour',
      timezone: 'America/New_York',
      rows: [{ opened_at_hour: '2026-11-10T09:00:00Z', count_opened: 3 }],
      column: 'opened_at_hour',
      measure: 'count_opened',
      now: dayjs('2026-11-12T00:00:00Z')
    })
    expect(buckets).toEqual([
      { key: '2026-11-10T09:00:00Z', value: 3 },
      { key: '2026-11-10T10:00:00Z', value: 0 }
    ])
  })

  it('spans the whole period, the hours still to come empty rather than at 0', () => {
    const buckets = fillBuckets({
      start: dayjs('2026-11-10T08:00:00Z'),
      end: dayjs('2026-11-12T08:00:00Z'),
      granularity: 'hour',
      timezone: 'UTC',
      rows: [],
      column: 'opened_at_hour',
      measure: 'count_opened',
      now: dayjs('2026-11-10T10:30:00Z')
    })
    expect(buckets).toHaveLength(49)
    expect(buckets[buckets.length - 1].key).toBe('2026-11-12T08:00:00Z')
    // 08:00, 09:00 and the hour under way have their value; 11:00 onwards is to come.
    expect(buckets.slice(0, 4)).toEqual([
      { key: '2026-11-10T08:00:00Z', value: 0 },
      { key: '2026-11-10T09:00:00Z', value: 0 },
      { key: '2026-11-10T10:00:00Z', value: 0 },
      { key: '2026-11-10T11:00:00Z', value: null }
    ])
    expect(buckets.slice(3).every((b) => b.value === null)).toBe(true)
  })

  it('shows 30 days for a broadcast sent 6 days ago, the days to come empty', () => {
    const start = dayjs('2026-11-09T08:00:00Z')
    const buckets = fillBuckets({
      start,
      end: start.add(30 * 24, 'hour'),
      granularity: 'day',
      timezone: 'UTC',
      rows: [{ opened_at_day: '2026-11-10T00:00:00Z', count_opened: 7 }],
      column: 'opened_at_day',
      measure: 'count_opened',
      now: dayjs('2026-11-15T12:00:00Z')
    })
    expect(buckets).toHaveLength(31)
    expect(buckets[1]).toEqual({ key: '2026-11-10T00:00:00Z', value: 7 })
    // Today has its value so far; tomorrow onwards is to come.
    expect(buckets[6]).toEqual({ key: '2026-11-15T00:00:00Z', value: 0 })
    expect(buckets[7]).toEqual({ key: '2026-11-16T00:00:00Z', value: null })
    expect(buckets.filter((b) => b.value !== null)).toHaveLength(7)
  })

  it('fills days', () => {
    const buckets = fillBuckets({
      start: dayjs('2026-11-01T10:00:00Z'),
      end: dayjs('2026-11-03T10:00:00Z'),
      granularity: 'day',
      timezone: 'UTC',
      rows: [{ clicked_at_day: '2026-11-02T00:00:00Z', count_clicked: 4 }],
      column: 'clicked_at_day',
      measure: 'count_clicked',
      now: dayjs('2026-11-12T00:00:00Z')
    })
    expect(buckets).toEqual([
      { key: '2026-11-01T00:00:00Z', value: 0 },
      { key: '2026-11-02T00:00:00Z', value: 4 },
      { key: '2026-11-03T00:00:00Z', value: 0 }
    ])
  })

  // Midnight to the next midnight in Paris, as UTC instants.
  const dayOfHours = (startUtc: string, endUtc: string) =>
    fillBuckets({
      start: dayjs(startUtc),
      end: dayjs(endUtc).subtract(1, 'minute'),
      granularity: 'hour',
      timezone: 'Europe/Paris',
      rows: [],
      column: 'opened_at_hour',
      measure: 'count_opened',
      now: dayjs('2027-01-01T00:00:00Z')
    })

  it('has 23 local hours on the spring change day, without the missing hour', () => {
    const keys = dayOfHours('2026-03-28T23:00:00Z', '2026-03-29T22:00:00Z').map((b) => b.key)
    expect(keys).toHaveLength(23)
    expect(keys).not.toContain('2026-03-29T02:00:00Z')
  })

  it('has one bucket for the repeated hour of the autumn change day', () => {
    const keys = dayOfHours('2026-10-24T22:00:00Z', '2026-10-25T23:00:00Z').map((b) => b.key)
    expect(keys).toHaveLength(24)
    expect(keys.filter((k) => k === '2026-10-25T02:00:00Z')).toHaveLength(1)
  })

  it('steps local days across a change', () => {
    const keys = fillBuckets({
      start: dayjs('2026-10-23T22:00:00Z'),
      end: dayjs('2026-10-27T10:00:00Z'),
      granularity: 'day',
      timezone: 'Europe/Paris',
      rows: [],
      column: 'opened_at_day',
      measure: 'count_opened',
      now: dayjs('2027-01-01T00:00:00Z')
    }).map((b) => b.key)
    expect(keys).toEqual(['2026-10-24T00:00:00Z', '2026-10-25T00:00:00Z', '2026-10-26T00:00:00Z', '2026-10-27T00:00:00Z'])
  })
})
