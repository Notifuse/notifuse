export type DashboardPeriod = '7d' | '30d' | '90d'
export type DashboardMetric = 'sent' | 'opens' | 'clicks'

/**
 * The dashboard's URL. Defaults (30 days, emails sent) are written as undefined, so a link without
 * `search` stays typed and the URL stays clean.
 */
export interface DashboardSearch {
  period?: Exclude<DashboardPeriod, '30d'>
  metric?: Exclude<DashboardMetric, 'sent'>
}

/**
 * Validator of the workspace index route and of /analytics. The router's default parser turns
 * `?period=7&period=9` into an array and numbers into numbers: anything but a known string goes.
 */
export function validateDashboardSearch(search: Record<string, unknown>): DashboardSearch {
  const period = search.period === '7d' || search.period === '90d' ? search.period : undefined
  const metric = search.metric === 'opens' || search.metric === 'clicks' ? search.metric : undefined
  return { period, metric }
}

export function dashboardPeriod(search: DashboardSearch): DashboardPeriod {
  return search.period ?? '30d'
}

export function dashboardMetric(search: DashboardSearch): DashboardMetric {
  return search.metric ?? 'sent'
}
