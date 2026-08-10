import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import dayjs from '../../../lib/dayjs'
import {
  computeComparisonRange,
  computeDateRange,
  determineGranularity,
  determineGranularityForRange,
  formatDateRangeLabel,
  getAvailableGranularities,
  resolveRange,
  timeBucketColumn,
  validateDateRange
} from './dates'

// A timezone well east of UTC, so a bug that ignores it shows up as a whole
// day's worth of offset rather than an hour that could be rounding.
const TZ = 'Asia/Tokyo'

describe('computeDateRange', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    // 2026-03-15 09:30 in Tokyo (2026-03-15 00:30 UTC).
    vi.setSystemTime(new Date('2026-03-15T00:30:00Z'))
  })
  afterEach(() => vi.useRealTimers())

  it('starts "today" at local midnight, not UTC midnight', () => {
    const range = computeDateRange('today', TZ)
    expect(range.start.format('YYYY-MM-DD HH:mm')).toBe('2026-03-15 00:00')
    expect(range.start.toISOString()).toBe('2026-03-14T15:00:00.000Z')
  })

  it('ends the rolling presets yesterday so a full period is never mixed with a partial day', () => {
    const range = computeDateRange('previous_7_days', TZ)
    expect(range.start.format('YYYY-MM-DD')).toBe('2026-03-08')
    expect(range.end.format('YYYY-MM-DD HH:mm')).toBe('2026-03-14 23:59')
  })

  it('covers a whole calendar month for previous_month', () => {
    const range = computeDateRange('previous_month', TZ)
    expect(range.start.format('YYYY-MM-DD')).toBe('2026-02-01')
    expect(range.end.format('YYYY-MM-DD')).toBe('2026-02-28')
  })

  it('widens a custom range to whole local days', () => {
    const range = computeDateRange('custom', TZ, { start: '2026-01-05', end: '2026-01-09' })
    expect(range.start.format('YYYY-MM-DD HH:mm')).toBe('2026-01-05 00:00')
    expect(range.end.format('YYYY-MM-DD HH:mm')).toBe('2026-01-09 23:59')
  })

  it('falls back to a bounded window when the workspace creation date is unknown', () => {
    const range = computeDateRange('all_time', TZ)
    expect(range.start.format('YYYY-MM-DD')).toBe('2024-03-15')
  })

  it('starts all_time at the workspace creation date when known', () => {
    const range = computeDateRange('all_time', TZ, undefined, '2025-11-20T08:00:00Z')
    expect(range.start.format('YYYY-MM-DD')).toBe('2025-11-20')
  })
})

describe('computeComparisonRange', () => {
  // Built in TZ and asserted in TZ: formatting a bare dayjs uses the machine's
  // own zone, which would make these assertions depend on where they run.
  const week = {
    start: dayjs.tz('2026-03-08', TZ).startOf('day'),
    end: dayjs.tz('2026-03-14', TZ).endOf('day')
  }
  const inTz = (value: dayjs.Dayjs | undefined) => value?.tz(TZ).format('YYYY-MM-DD')

  it('abuts the previous period without overlapping it', () => {
    const comparison = computeComparisonRange(week, 'previous_period')
    expect(inTz(comparison?.start)).toBe('2026-03-01')
    expect(inTz(comparison?.end)).toBe('2026-03-07')
    expect(comparison?.end.isBefore(week.start)).toBe(true)
  })

  it('keeps the same calendar dates a year earlier', () => {
    const comparison = computeComparisonRange(week, 'previous_year')
    expect(inTz(comparison?.start)).toBe('2025-03-08')
    expect(inTz(comparison?.end)).toBe('2025-03-14')
  })

  it('has no comparison range when comparison is off', () => {
    const range = { start: dayjs(), end: dayjs() }
    expect(computeComparisonRange(range, 'none')).toBeNull()
  })
})

describe('resolveRange', () => {
  it('converts local day bounds into absolute instants', () => {
    const range = computeDateRange('custom', TZ, { start: '2026-01-05', end: '2026-01-06' })
    const resolved = resolveRange(range, TZ)

    // The engine parses the day strings itself with the query timezone, and
    // compares the instants verbatim; both must describe the same span.
    expect(resolved.startDay).toBe('2026-01-05')
    expect(resolved.endDay).toBe('2026-01-06')
    expect(resolved.startUtc).toBe('2026-01-04T15:00:00.000Z')
    expect(resolved.endUtc).toBe('2026-01-06T14:59:59.999Z')
  })

  it('emits days the time-series gap filler can parse', () => {
    const resolved = resolveRange(computeDateRange('previous_7_days', 'UTC'), 'UTC')
    expect(resolved.startDay).toMatch(/^\d{4}-\d{2}-\d{2}$/)
    expect(resolved.endDay).toMatch(/^\d{4}-\d{2}-\d{2}$/)
  })
})

describe('granularity', () => {
  it('buckets intra-day presets by hour and yearly ones by month', () => {
    expect(determineGranularity('today')).toBe('hour')
    expect(determineGranularity('yesterday')).toBe('hour')
    expect(determineGranularity('previous_28_days')).toBe('day')
    expect(determineGranularity('previous_12_months')).toBe('month')
  })

  it('picks a granularity for custom ranges from their length', () => {
    const twoDays = { start: dayjs('2026-01-01'), end: dayjs('2026-01-03') }
    const twoMonths = { start: dayjs('2026-01-01'), end: dayjs('2026-03-01') }
    const twoYears = { start: dayjs('2024-01-01'), end: dayjs('2026-01-01') }
    expect(determineGranularityForRange(twoDays)).toBe('hour')
    expect(determineGranularityForRange(twoMonths)).toBe('day')
    expect(determineGranularityForRange(twoYears)).toBe('month')
  })

  it('only offers granularities that produce a readable number of buckets', () => {
    expect(getAvailableGranularities(1)).toEqual(['hour'])
    expect(getAvailableGranularities(7)).toEqual(['hour', 'day'])
    expect(getAvailableGranularities(30)).toEqual(['day', 'week'])
    expect(getAvailableGranularities(400)).toEqual(['week', 'month'])
  })
})

describe('validateDateRange', () => {
  it('accepts an ordinary range', () => {
    expect(
      validateDateRange({ start: dayjs().subtract(7, 'day'), end: dayjs().subtract(1, 'day') })
    ).toBeNull()
  })

  it('rejects reversed, oversized and future ranges', () => {
    expect(validateDateRange({ start: dayjs(), end: dayjs().subtract(1, 'day') })).toBe(
      'end_before_start'
    )
    expect(
      validateDateRange({ start: dayjs().subtract(3, 'year'), end: dayjs() })
    ).toBe('too_long')
    expect(
      validateDateRange({ start: dayjs().add(2, 'day'), end: dayjs().add(3, 'day') })
    ).toBe('in_future')
  })
})

describe('formatDateRangeLabel', () => {
  it('drops the parts both bounds share', () => {
    expect(
      formatDateRangeLabel({ start: dayjs('2026-02-03'), end: dayjs('2026-02-03') })
    ).toBe('3 Feb')
    expect(
      formatDateRangeLabel({ start: dayjs('2026-02-03'), end: dayjs('2026-02-09') })
    ).toBe('3 - 9 Feb')
    expect(
      formatDateRangeLabel({ start: dayjs('2026-01-28'), end: dayjs('2026-02-03') })
    ).toBe('28 Jan - 3 Feb')
    expect(
      formatDateRangeLabel({ start: dayjs('2025-01-28'), end: dayjs('2026-02-03') })
    ).toBe('28 Jan 25 - 3 Feb 26')
  })
})

describe('timeBucketColumn', () => {
  it('matches the column the engine returns for a bucketed dimension', () => {
    expect(timeBucketColumn('created_at', 'day')).toBe('created_at_day')
    expect(timeBucketColumn('goal_at', 'hour')).toBe('goal_at_hour')
  })
})
