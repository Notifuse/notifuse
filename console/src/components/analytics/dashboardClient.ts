import { AnalyticsService, type AnalyticsQuery, type AnalyticsResponse } from '../../services/api/analytics'

let client: AnalyticsService | undefined

/**
 * The dashboard's own analytics client. The shared singleton runs one request at a time, which
 * would make each card wait for the others; the cache stays under the 60 s refresh.
 *
 * Created on the first query rather than at import: the router imports this page, and the API
 * client imports the router, so a module that starts from the API client reaches this file before
 * AnalyticsService is defined.
 */
export const dashboardAnalytics = {
  query(query: AnalyticsQuery, workspaceId: string): Promise<AnalyticsResponse> {
    client ??= AnalyticsService.create({ maxConcurrency: 4, cacheTTL: 30_000 })
    return client.query(query, workspaceId)
  }
}

/** Every block refreshes on this interval. */
export const DASHBOARD_REFRESH_MS = 60_000
