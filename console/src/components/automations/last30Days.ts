import { computeDateRange, resolveRange } from '../web_analytics/lib/dates'
import type { ResolvedRange } from '../web_analytics/lib/types'

/**
 * The one "last 30 days" of the automations screens: the 30 whole days that end yesterday, in the
 * workspace's timezone. The list's open rate and the editor's step results read the same window.
 */
export function last30Days(timezone: string): ResolvedRange {
  return resolveRange(computeDateRange('previous_30_days', timezone), timezone)
}
