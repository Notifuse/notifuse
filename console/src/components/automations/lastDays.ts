import dayjs from '../../lib/dayjs'
import { resolveRange } from '../web_analytics/lib/dates'
import type { ResolvedRange } from '../web_analytics/lib/types'

/**
 * The `days` days that end today, today included, in the workspace's timezone. An automation
 * activated this morning shows this morning's journeys.
 */
export function lastDays(timezone: string, days: number): ResolvedRange {
  const today = dayjs().tz(timezone)
  return resolveRange({ start: today.subtract(days - 1, 'day'), end: today }, timezone)
}

/**
 * The one "last 30 days" of the automations screens. The list's open rate and the editor's step
 * results read the same window.
 */
export function last30Days(timezone: string): ResolvedRange {
  return lastDays(timezone, 30)
}
