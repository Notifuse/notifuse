import { buildWebQuery } from '../lib/query'
import {
  Granularity,
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

export interface BreakdownSeriesQueryParams {
  filters: WebDimensionFilter[]
  range: ResolvedRange
  timezone: string
  granularity: Granularity
}

/**
 * The broken-down row over the period, bucketed by the page's granularity. Neither the metric
 * filters nor the session threshold: they select rows of a breakdown, and on a series they would
 * delete the days that fall under them.
 */
export function buildBreakdownSeriesQuery(params: BreakdownSeriesQueryParams): AnalyticsQuery {
  return buildWebQuery({
    schema: 'web_sessions',
    measures: SESSION_METRIC_KEYS,
    dimensions: [],
    range: params.range,
    filters: params.filters,
    granularity: params.granularity,
    timezone: params.timezone
  })
}
