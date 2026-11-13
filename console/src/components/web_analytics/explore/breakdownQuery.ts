import { buildWebQuery } from '../lib/query'
import {
  ResolvedRange,
  SESSION_METRIC_KEYS,
  WebDimensionFilter,
  WebMetricFilter
} from '../lib/types'
import type { AnalyticsQuery } from '../../../services/api/analytics'

export interface BreakdownQueryParams {
  dimension: string
  filters: WebDimensionFilter[]
  metricFilters: WebMetricFilter[]
  minSessions: number
  range: ResolvedRange
  timezone: string
}

/**
 * One breakdown column's query. Shared with the hover prefetch on the table so
 * both produce the same cache key and the drawer opens on warm data.
 */
export function buildBreakdownQuery(params: BreakdownQueryParams): AnalyticsQuery {
  return buildWebQuery({
    schema: 'web_sessions',
    measures: SESSION_METRIC_KEYS,
    dimensions: [params.dimension],
    range: params.range,
    filters: params.filters,
    metricFilters: params.metricFilters,
    minSessions: params.minSessions,
    order: { sessions: 'desc' },
    limit: 100,
    timezone: params.timezone
  })
}
