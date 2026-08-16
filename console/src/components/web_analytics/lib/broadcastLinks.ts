import { Dayjs } from 'dayjs'
import dayjs from '../../../lib/dayjs'

/**
 * Deep links from a broadcast into the web analytics section, scoped to the
 * broadcast's UTM campaign over its send window.
 *
 * The search values are returned as an object and must stay one all the way to
 * the router: TanStack re-stringifies any search value that parses as JSON, so
 * a hand-built `?filters=[{...}]` comes back as a real array, is String()-ed to
 * "[object Object]" by the route's text coercer, and is then dropped by the
 * context's own try/catch — an unfiltered report with no error anywhere.
 */

/** Drill order of the traffic report: which linked page pulled traffic, then who. */
export const BROADCAST_EXPLORE_DIMENSIONS = 'landing_path,device,country'

/**
 * Explore applies a `HAVING sessions >= minSessions` floor that defaults to 10,
 * which blanks a modest broadcast's report entirely. 2 is the lowest value the
 * URL can carry: validateSearch discards anything at or below 1, and the
 * context then falls back to the default of 10.
 */
export const BROADCAST_MIN_SESSIONS = 2

/** Days after the send during which a visit still counts as coming from it. */
export const BROADCAST_CONVERSION_WINDOW_DAYS = 7

export const WEB_ANALYTICS_TAB_ROUTE = '/console/workspace/$workspaceId/web-analytics/$tab'

export interface BroadcastAnalyticsTarget {
  to: typeof WEB_ANALYTICS_TAB_ROUTE
  params: { workspaceId: string; tab: string }
  search: Record<string, string | number>
}

export interface BroadcastAnalyticsLinks {
  traffic: BroadcastAnalyticsTarget
  conversions: BroadcastAnalyticsTarget
}

export interface BroadcastAnalyticsLinksInput {
  workspaceId: string
  campaign: string
  /**
   * Narrows both reports to one A/B variation. A send stamps utm_content with
   * the variation's template id, but only when the broadcast itself sets no
   * utm_content — when it does, every variation ships that one value and the
   * variations cannot be separated. Callers must check that before passing a
   * value here.
   */
  content?: string
  startedAt: string
  completedAt?: string | null
  timezone: string
  /** Injectable so the window is not clock-dependent under test. */
  now?: Dayjs
}

/**
 * Builds both targets. They share a period and a filter so the two reports
 * always describe the same traffic; only the tab and the explore-specific
 * params differ.
 */
export function buildBroadcastAnalyticsLinks(
  input: BroadcastAnalyticsLinksInput
): BroadcastAnalyticsLinks {
  const { workspaceId, campaign, content, startedAt, completedAt, timezone } = input
  const now = (input.now ?? dayjs()).tz(timezone)

  const start = dayjs(startedAt).tz(timezone).startOf('day')
  const lastActivity = completedAt ? dayjs(completedAt).tz(timezone) : now
  const windowEnd = lastActivity.add(BROADCAST_CONVERSION_WINDOW_DAYS, 'day')
  // The conversion window usually runs past today, and a range that ends in the
  // future reads as missing data rather than as a window still open.
  const capped = windowEnd.isAfter(now) ? now : windowEnd
  const end = capped.isBefore(start) ? start : capped

  const dimensionFilters = [{ dimension: 'utm_campaign', operator: 'equals', values: [campaign] }]
  if (content) {
    dimensionFilters.push({ dimension: 'utm_content', operator: 'equals', values: [content] })
  }
  const filters = JSON.stringify(dimensionFilters)

  // Both bounds always travel with period=custom: setPeriod clears them for
  // every other preset, so one without the other silently widens the report.
  const shared = {
    period: 'custom',
    customStart: start.format('YYYY-MM-DD'),
    customEnd: end.format('YYYY-MM-DD'),
    // A one-shot send has no previous period to compare against, and the
    // remembered default would put an empty comparison column on every row.
    comparison: 'none',
    filters
  }

  return {
    traffic: {
      to: WEB_ANALYTICS_TAB_ROUTE,
      params: { workspaceId, tab: 'explore' },
      // Explore without a drill order renders the template picker with the
      // whole toolbar hidden, so the campaign scope would apply invisibly.
      search: {
        ...shared,
        dimensions: BROADCAST_EXPLORE_DIMENSIONS,
        minSessions: BROADCAST_MIN_SESSIONS
      }
    },
    conversions: {
      to: WEB_ANALYTICS_TAB_ROUTE,
      params: { workspaceId, tab: 'goals' },
      // The goals tab honours neither a drill order nor the session floor.
      search: { ...shared }
    }
  }
}
