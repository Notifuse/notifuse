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

export const periodDays = (period: TransactionalPeriod): number => DAYS[period]

/** Today in the workspace timezone, YYYY-MM-DD: the window slides when it changes. */
export const workspaceToday = (timezone: string, now: Date = new Date()): string =>
  dayjs(now)
    .tz(timezone || 'UTC')
    .format('YYYY-MM-DD')

/**
 * N calendar days in the workspace timezone, today included: an outage this morning must show on a
 * monitoring screen. Days serve the daily series (`dateRange`), instants the totals (`inDateRange`).
 */
export function resolveTransactionalPeriod(
  period: TransactionalPeriod,
  timezone: string,
  /** YYYY-MM-DD in the workspace timezone; today when left out. */
  todayDay?: string
): TransactionalPeriods {
  const tz = timezone || 'UTC'
  const days = DAYS[period]
  const today = todayDay ? dayjs.tz(todayDay, tz) : dayjs().tz(tz)
  const range = computeDateRange('custom', tz, {
    start: today.subtract(days - 1, 'day').format('YYYY-MM-DD'),
    end: today.format('YYYY-MM-DD')
  })
  // previous_period never returns null; the type allows it for the 'none' mode.
  const previous = computeComparisonRange(range, 'previous_period') ?? range
  return { days, current: resolveRange(range, tz), previous: resolveRange(previous, tz) }
}
