import type { AnalyticsQuery } from '../../services/api/analytics'

const HOUR = 60 * 60 * 1000
const DAY = 24 * HOUR

/**
 * The 30 days the views column counts, in ISO instants: from 30 days ago rounded down to the
 * hour, to now rounded up to the hour, so the query key holds still within an hour.
 */
export function viewsWindow(now: Date = new Date()): [string, string] {
  const end = Math.ceil(now.getTime() / HOUR) * HOUR
  const start = Math.floor((now.getTime() - 30 * DAY) / HOUR) * HOUR
  return [new Date(start).toISOString(), new Date(end).toISOString()]
}

/**
 * Page views of some post paths over a window. `inDateRange` on `entered_at` is the filter that
 * lets the database skip the partitions of `web_pages` outside the window.
 */
export function postViewsQuery(paths: string[], window: [string, string]): AnalyticsQuery {
  return {
    schema: 'web_pages',
    measures: ['page_count'],
    dimensions: ['page_path'],
    filters: [
      { member: 'page_path', operator: 'in', values: paths },
      { member: 'entered_at', operator: 'inDateRange', values: window }
    ],
    limit: paths.length
  }
}

export function viewsByPath(rows: Array<Record<string, unknown>>): Map<string, number> {
  const views = new Map<string, number>()
  for (const row of rows) {
    const path = row.page_path
    const count = Number(row.page_count)
    if (typeof path === 'string' && Number.isFinite(count)) views.set(path, count)
  }
  return views
}
