import dayjs from '../../lib/dayjs'
import { resolveRange } from '../web_analytics/lib/dates'
import type { ResolvedRange } from '../web_analytics/lib/types'

/**
 * The one "last 30 days" of the automations screens: the 30 days that end today, today included,
 * in the workspace's timezone. An automation activated this morning shows this morning's journeys.
 * The list's open rate and the editor's step results read the same window.
 */
export function last30Days(timezone: string): ResolvedRange {
  const today = dayjs().tz(timezone)
  return resolveRange({ start: today.subtract(29, 'day'), end: today }, timezone)
}
