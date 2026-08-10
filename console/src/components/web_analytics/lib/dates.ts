import { Dayjs } from 'dayjs'
import dayjs from '../../../lib/dayjs'
import { ComparisonMode, DatePreset, Granularity, ResolvedRange } from './types'

export interface DayjsRange {
  start: Dayjs
  end: Dayjs
}

/**
 * Resolves a preset to concrete bounds in the workspace timezone.
 *
 * Note that the "previous N days" presets end yesterday rather than today, so
 * a period never mixes a complete history with today's partial one.
 */
export function computeDateRange(
  preset: DatePreset,
  timezone: string,
  customRange?: { start: string; end: string },
  firstSessionAt?: string
): DayjsRange {
  const now = dayjs().tz(timezone)
  const lastNDays = (days: number): DayjsRange => ({
    start: now.subtract(days, 'day').startOf('day'),
    end: now.subtract(1, 'day').endOf('day')
  })

  switch (preset) {
    case 'today':
      return { start: now.startOf('day'), end: now }
    case 'yesterday':
      return {
        start: now.subtract(1, 'day').startOf('day'),
        end: now.subtract(1, 'day').endOf('day')
      }
    case 'previous_7_days':
      return lastNDays(7)
    case 'previous_14_days':
      return lastNDays(14)
    case 'previous_28_days':
      return lastNDays(28)
    case 'previous_30_days':
      return lastNDays(30)
    case 'previous_90_days':
      return lastNDays(90)
    case 'previous_91_days':
      return lastNDays(91)
    case 'this_week':
      return { start: now.startOf('week'), end: now }
    case 'previous_week':
      return {
        start: now.subtract(1, 'week').startOf('week'),
        end: now.subtract(1, 'week').endOf('week')
      }
    case 'this_month':
      return { start: now.startOf('month'), end: now }
    case 'previous_month':
      return {
        start: now.subtract(1, 'month').startOf('month'),
        end: now.subtract(1, 'month').endOf('month')
      }
    case 'this_quarter':
      return { start: now.startOf('quarter'), end: now }
    case 'previous_quarter':
      return {
        start: now.subtract(1, 'quarter').startOf('quarter'),
        end: now.subtract(1, 'quarter').endOf('quarter')
      }
    case 'this_year':
      return { start: now.startOf('year'), end: now }
    case 'previous_year':
      return {
        start: now.subtract(1, 'year').startOf('year'),
        end: now.subtract(1, 'year').endOf('year')
      }
    case 'previous_12_months':
      return { start: now.subtract(12, 'month').startOf('month'), end: now }
    case 'all_time':
      // Web analytics has no data before the workspace existed; falling back
      // two years keeps the range bounded when the date is unknown, which
      // matters because every extra month is another partition to scan.
      return {
        start: firstSessionAt
          ? dayjs(firstSessionAt).tz(timezone).startOf('day')
          : now.subtract(2, 'year').startOf('day'),
        end: now
      }
    case 'custom':
      if (!customRange) return lastNDays(7)
      return {
        start: dayjs.tz(customRange.start, timezone).startOf('day'),
        end: dayjs.tz(customRange.end, timezone).endOf('day')
      }
    default:
      return lastNDays(7)
  }
}

/**
 * Converts bounds to the two forms the analytics engine needs. Days are
 * widened to whole local days so a range filter and a bucketed time series
 * cover exactly the same span.
 */
export function resolveRange(range: DayjsRange, timezone: string): ResolvedRange {
  const startDay = range.start.format('YYYY-MM-DD')
  const endDay = range.end.format('YYYY-MM-DD')
  return {
    startDay,
    endDay,
    startUtc: dayjs.tz(startDay, timezone).startOf('day').toISOString(),
    endUtc: dayjs.tz(endDay, timezone).endOf('day').toISOString()
  }
}

/**
 * The range a comparison is measured against. `previous_period` shifts back by
 * the period's own length so the two windows abut without overlapping;
 * `previous_year` keeps the same calendar dates a year earlier.
 */
export function computeComparisonRange(range: DayjsRange, mode: ComparisonMode): DayjsRange | null {
  if (mode === 'none') return null
  if (mode === 'previous_year') {
    return { start: range.start.subtract(1, 'year'), end: range.end.subtract(1, 'year') }
  }
  const days = range.end.diff(range.start, 'day')
  return {
    start: range.start.subtract(days + 1, 'day'),
    end: range.start.subtract(1, 'day').endOf('day')
  }
}

const PRESET_GRANULARITY: Record<DatePreset, Granularity> = {
  today: 'hour',
  yesterday: 'hour',
  previous_7_days: 'day',
  previous_14_days: 'day',
  previous_28_days: 'day',
  previous_30_days: 'day',
  previous_90_days: 'day',
  previous_91_days: 'day',
  this_week: 'day',
  previous_week: 'day',
  this_month: 'day',
  previous_month: 'day',
  this_quarter: 'day',
  previous_quarter: 'day',
  this_year: 'month',
  previous_year: 'month',
  previous_12_months: 'month',
  all_time: 'month',
  custom: 'day'
}

export function determineGranularity(preset: DatePreset): Granularity {
  return PRESET_GRANULARITY[preset] ?? 'day'
}

export function determineGranularityForRange(range: DayjsRange): Granularity {
  const days = range.end.diff(range.start, 'day')
  if (days <= 2) return 'hour'
  if (days <= 120) return 'day'
  return 'month'
}

/** Granularities that produce a readable number of buckets for a range. */
export function getAvailableGranularities(days: number): Granularity[] {
  if (days <= 2) return ['hour']
  if (days <= 7) return ['hour', 'day']
  if (days <= 30) return ['day', 'week']
  if (days <= 120) return ['day', 'week', 'month']
  return ['week', 'month']
}

/** Compact label for a custom range, e.g. "3 - 9 Feb" or "28 Jan 24 - 3 Feb 25". */
export function formatDateRangeLabel(range: DayjsRange): string {
  const { start, end } = range
  if (start.isSame(end, 'day')) return start.format('D MMM')
  if (start.year() === end.year()) {
    if (start.month() === end.month()) return `${start.format('D')} - ${end.format('D MMM')}`
    return `${start.format('D MMM')} - ${end.format('D MMM')}`
  }
  return `${start.format('D MMM YY')} - ${end.format('D MMM YY')}`
}

export type DateRangeProblem = 'end_before_start' | 'too_long' | 'in_future'

/** Returns the first problem with a user-picked range, or null when valid. */
export function validateDateRange(range: DayjsRange): DateRangeProblem | null {
  if (range.end.isBefore(range.start)) return 'end_before_start'
  if (range.end.diff(range.start, 'day') > 730) return 'too_long'
  if (range.start.isAfter(dayjs())) return 'in_future'
  return null
}

/**
 * Column the engine returns for a bucketed time dimension: it suffixes the
 * dimension with the granularity (`created_at_day`).
 */
export function timeBucketColumn(dimension: string, granularity: Granularity): string {
  return `${dimension}_${granularity}`
}
