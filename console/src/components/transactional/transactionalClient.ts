import { useEffect, useState } from 'react'
import { AnalyticsService, type AnalyticsQuery, type AnalyticsResponse } from '../../services/api/analytics'

let client: AnalyticsService | undefined

/**
 * The page's own analytics client: the shared singleton runs one request at a time. Created on the
 * first query rather than at import, because the API client imports the router, which imports this
 * page: a module that starts from the API client reaches this file before AnalyticsService exists.
 */
export const transactionalAnalytics = {
  query(query: AnalyticsQuery, workspaceId: string): Promise<AnalyticsResponse> {
    client ??= AnalyticsService.create({ maxConcurrency: 4, cacheTTL: 30_000 })
    return client.query(query, workspaceId)
  }
}

/** Changes once a minute, so relative times ("3 min ago") stay true on an open page. */
export function useMinuteTick(): number {
  const [tick, setTick] = useState(() => Math.floor(Date.now() / 60_000))
  useEffect(() => {
    const id = window.setInterval(() => setTick(Math.floor(Date.now() / 60_000)), 60_000)
    return () => window.clearInterval(id)
  }, [])
  return tick
}
