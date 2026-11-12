import { AnalyticsService, type AnalyticsQuery } from '../../services/api/analytics'

let client: AnalyticsService | undefined

/**
 * Runs an analytics query of the broadcasts screens. The shared singleton runs one query at a
 * time and caches each answer 30 s by its JSON, so the 30 s refetch of a live row would read its
 * own cached answer: this client runs four at once and caches nothing, TanStack Query being the
 * only cache. Created on first use.
 */
export function runBroadcastQuery(query: AnalyticsQuery, workspaceId: string) {
  client ??= AnalyticsService.create({ maxConcurrency: 4, cacheTTL: 0 })
  return client.query(query, workspaceId)
}
