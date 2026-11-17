import dayjs from '../../lib/dayjs'
import { computeComparisonRange, computeDateRange, resolveRange } from '../web_analytics/lib/dates'
import type { DatePreset, ResolvedRange } from '../web_analytics/lib/types'
import type { DashboardPeriod } from './dashboardSearch'

export interface DashboardPeriods {
  days: number
  /** The web analytics preset of the same period, for the link to its dashboard. */
  preset: DatePreset
  current: ResolvedRange
  previous: ResolvedRange
  /**
   * The same number of calendar days ending today, today included: what counts new contacts, which
   * a period ending yesterday showed as "+0" right after an import.
   */
  recent: ResolvedRange
}

const PERIODS = {
  '7d': { days: 7, preset: 'previous_7_days' },
  '30d': { days: 30, preset: 'previous_30_days' },
  '90d': { days: 90, preset: 'previous_90_days' }
} as const satisfies Record<DashboardPeriod, { days: number; preset: DatePreset }>

/**
 * The period and the one before it, both ending yesterday, as the web analytics computes them: n
 * complete days in the workspace timezone, so the visits figure is the one web analytics shows.
 * Days serve the daily series (`dateRange`), instants the totals (`inDateRange`).
 */
export function resolveDashboardPeriods(period: DashboardPeriod, timezone: string): DashboardPeriods {
  const tz = timezone || 'UTC'
  const { days, preset } = PERIODS[period]
  const range = computeDateRange(preset, tz)
  // Never null for 'previous_period'; the fallback only satisfies the type.
  const previous = computeComparisonRange(range, 'previous_period') ?? range
  const now = dayjs().tz(tz)
  // resolveRange takes the local days and widens them to whole days, DST included.
  const recent = { start: now.subtract(days - 1, 'day').startOf('day'), end: now.endOf('day') }
  return {
    days,
    preset,
    current: resolveRange(range, tz),
    previous: resolveRange(previous, tz),
    recent: resolveRange(recent, tz)
  }
}
