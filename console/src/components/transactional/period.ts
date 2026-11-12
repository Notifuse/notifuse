import dayjs from '../../lib/dayjs'
import { computeComparisonRange, computeDateRange, resolveRange } from '../web_analytics/lib/dates'
import type { ResolvedRange } from '../web_analytics/lib/types'
import type { TransactionalPeriod } from './transactionalSearch'

export interface TransactionalPeriods {
  days: number
  current: ResolvedRange
  /** The same number of days just before. */
  previous: ResolvedRange
}

const DAYS: Record<TransactionalPeriod, number> = { '7d': 7, '30d': 30, '60d': 60 }

/**
 * N calendar days in the workspace timezone, today included: an outage this morning must show on a
 * monitoring screen. Days serve the daily series (`dateRange`), instants the totals (`inDateRange`).
 */
export function resolveTransactionalPeriod(period: TransactionalPeriod, timezone: string): TransactionalPeriods {
  const tz = timezone || 'UTC'
  const days = DAYS[period]
  const today = dayjs().tz(tz)
  const range = computeDateRange('custom', tz, {
    start: today.subtract(days - 1, 'day').format('YYYY-MM-DD'),
    end: today.format('YYYY-MM-DD')
  })
  // previous_period never returns null; the type allows it for the 'none' mode.
  const previous = computeComparisonRange(range, 'previous_period') ?? range
  return { days, current: resolveRange(range, tz), previous: resolveRange(previous, tz) }
}
