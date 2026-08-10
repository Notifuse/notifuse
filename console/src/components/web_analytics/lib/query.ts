import { useQueries, useQuery, UseQueryResult, keepPreviousData } from '@tanstack/react-query'
import {
  AnalyticsQuery,
  AnalyticsResponse,
  AnalyticsService
} from '../../../services/api/analytics'
import { changePercent, toNumber } from './format'
import {
  DimensionFilterOperator,
  DimensionRow,
  Granularity,
  ResolvedRange,
  SCHEMA_TIME_DIMENSION,
  WebDimensionFilter,
  WebMetricFilter,
  WebSchema
} from './types'

/**
 * Web analytics pages fan out into a dozen small widgets at once. The shared
 * analytics singleton runs one request at a time, which would turn a dashboard
 * into a dozen sequential round trips, so this view gets its own client.
 */
export const webAnalyticsClient = AnalyticsService.create({
  maxConcurrency: 4,
  cacheTTL: 60_000
})

/**
 * The live view cannot share the client above: a one-minute cache would answer
 * every poll from memory, so the page would claim to refresh every few seconds
 * while showing minute-old numbers. It also fans out wider — a map plus six
 * breakdowns at once — so it gets its own concurrency budget.
 */
export const webAnalyticsLiveClient = AnalyticsService.create({
  maxConcurrency: 8,
  cacheTTL: 5_000
})

export interface WebQueryOptions {
  schema: WebSchema
  measures: string[]
  dimensions?: string[]
  range: ResolvedRange
  /** Set to bucket the range into a time series instead of aggregating it. */
  granularity?: Granularity
  /**
   * Time dimension the range applies to. Defaults to when the row was created,
   * which is what a report about a period means; the live view overrides it
   * with last activity, which is what "still here" means.
   */
  timeDimension?: string
  filters?: WebDimensionFilter[]
  metricFilters?: WebMetricFilter[]
  /** Drops dimension rows below this many sessions (web_sessions only). */
  minSessions?: number
  order?: Record<string, 'asc' | 'desc'>
  limit?: number
  timezone: string
}

type EngineFilter = NonNullable<AnalyticsQuery['filters']>[number]

const OPERATOR_MAP: Record<DimensionFilterOperator, EngineFilter['operator']> = {
  equals: 'equals',
  notEquals: 'notEquals',
  contains: 'contains',
  notContains: 'notContains',
  in: 'in',
  notIn: 'notIn',
  gt: 'gt',
  gte: 'gte',
  lt: 'lt',
  lte: 'lte',
  // Dimensions are stored NOT NULL DEFAULT '', and the one that is not
  // (contact_email) is exposed through COALESCE, so "empty" is always an equality
  // with the empty string rather than a NULL test.
  isEmpty: 'equals',
  isNotEmpty: 'notEquals'
}

function toEngineFilters(filters: WebDimensionFilter[]): NonNullable<AnalyticsQuery['filters']> {
  return filters.map((filter) => {
    const operator = OPERATOR_MAP[filter.operator]
    const values =
      filter.operator === 'isEmpty' || filter.operator === 'isNotEmpty'
        ? ['']
        : filter.values.map((value) => String(value))
    return { member: filter.dimension, operator, values }
  })
}

/**
 * Translates a widget's intent into a cube query.
 *
 * The range travels one of two ways. A bucketed query puts it on the time
 * dimension as local calendar days, which the engine converts using `timezone`
 * and which lets it fill empty buckets. An aggregate query puts it on a plain
 * range filter as absolute instants, because a granularity there would split
 * every breakdown row per day.
 */
export function buildWebQuery(options: WebQueryOptions): AnalyticsQuery {
  const timeDimension = options.timeDimension ?? SCHEMA_TIME_DIMENSION[options.schema]
  const filters = toEngineFilters(options.filters ?? [])

  const query: AnalyticsQuery = {
    schema: options.schema,
    measures: options.measures,
    dimensions: options.dimensions ?? [],
    timezone: options.timezone || undefined
  }

  if (options.granularity) {
    query.timeDimensions = [
      {
        dimension: timeDimension,
        granularity: options.granularity,
        dateRange: [options.range.startDay, options.range.endDay]
      }
    ]
  } else {
    filters.unshift({
      member: timeDimension,
      operator: 'inDateRange',
      values: [options.range.startUtc, options.range.endUtc]
    })
  }

  if (filters.length > 0) query.filters = filters

  const having: NonNullable<AnalyticsQuery['having']> = (options.metricFilters ?? []).map(
    (metricFilter) => ({
      member: metricFilter.metric,
      operator: metricFilter.operator,
      values: metricFilter.values.map((value) => String(value))
    })
  )

  // A threshold only means something once rows are grouped; on a totals query
  // it would filter away the single row it is meant to summarize.
  const grouped = (options.dimensions?.length ?? 0) > 0
  if (grouped && options.minSessions && options.minSessions > 1 && options.schema === 'web_sessions') {
    having.push({ member: 'sessions', operator: 'gte', values: [String(options.minSessions)] })
  }
  if (having.length > 0) query.having = having

  if (options.order) query.order = options.order
  else if (options.measures.length > 0 && grouped) query.order = { [options.measures[0]]: 'desc' }

  if (options.limit) query.limit = options.limit

  return query
}

/**
 * The explore report's best-performing combination.
 *
 * Grouped by every dimension at once — unlike the drill-down, which asks one
 * level at a time — because the winner is a property of the whole report
 * rather than of a level. A single row comes back carrying both the TimeScore
 * and the dimension values that achieved it.
 *
 * `base` is the drill-down's own level options, so the filters, threshold and
 * timezone stay in step with the table below. The two overrides must therefore
 * come after it.
 */
export function buildBestCombinationQuery(
  base: Omit<WebQueryOptions, 'range' | 'dimensions'>,
  dimensions: string[],
  range: ResolvedRange
): AnalyticsQuery {
  return buildWebQuery({
    ...base,
    dimensions,
    // Ordering by sessions would name the busiest combination rather than the
    // best-performing one — a plausible number in the wrong tile.
    order: { median_duration: 'desc' },
    limit: 1,
    range
  })
}

export function useWebQuery(
  workspaceId: string,
  query: AnalyticsQuery | null,
  options?: { enabled?: boolean; refetchInterval?: number; client?: AnalyticsService }
): UseQueryResult<AnalyticsResponse> {
  const client = options?.client ?? webAnalyticsClient
  return useQuery<AnalyticsResponse>({
    queryKey: ['web-analytics', workspaceId, query],
    queryFn: () => client.query(query as AnalyticsQuery, workspaceId),
    enabled: query != null && options?.enabled !== false,
    placeholderData: keepPreviousData,
    refetchInterval: options?.refetchInterval,
    refetchIntervalInBackground: false
  })
}

/**
 * Runs a query over the current range and, when comparing, the same query over
 * the comparison range. The engine has no notion of a comparison period, so
 * the two windows are two requests merged here.
 */
export function useWebComparisonQuery(
  workspaceId: string,
  current: AnalyticsQuery | null,
  previous: AnalyticsQuery | null,
  options?: { enabled?: boolean; refetchInterval?: number; client?: AnalyticsService }
) {
  const enabled = options?.enabled !== false
  const client = options?.client ?? webAnalyticsClient
  const results = useQueries({
    queries: [
      {
        queryKey: ['web-analytics', workspaceId, current],
        queryFn: () => client.query(current as AnalyticsQuery, workspaceId),
        enabled: current != null && enabled,
        placeholderData: keepPreviousData,
        refetchInterval: options?.refetchInterval,
        refetchIntervalInBackground: false
      },
      {
        queryKey: ['web-analytics', workspaceId, previous],
        queryFn: () => client.query(previous as AnalyticsQuery, workspaceId),
        enabled: previous != null && enabled,
        placeholderData: keepPreviousData
      }
    ]
  })

  const [currentResult, previousResult] = results as UseQueryResult<AnalyticsResponse>[]
  return {
    current: currentResult.data,
    previous: previous ? previousResult.data : undefined,
    isLoading: currentResult.isLoading || (previous != null && previousResult.isLoading),
    isFetching: currentResult.isFetching || previousResult.isFetching,
    error: currentResult.error ?? previousResult.error
  }
}

export function readMeasure(row: Record<string, unknown> | undefined, measure: string): number {
  return toNumber(row?.[measure])
}

/** Reads a single-row aggregate, which the engine zero-fills when empty. */
export function readTotals(
  response: AnalyticsResponse | undefined,
  measures: string[]
): Record<string, number> {
  const row = response?.data?.[0]
  return Object.fromEntries(measures.map((measure) => [measure, readMeasure(row, measure)]))
}

/**
 * Joins a breakdown with its comparison period on the dimension value, adding
 * `prev_<measure>` and `<measure>_change` to every row. Rows that only exist
 * in the comparison period are dropped, matching how the period-over-period
 * tables read: they list what happened, then say how that moved.
 */
export function mergeComparisonRows(
  current: Record<string, unknown>[],
  previous: Record<string, unknown>[] | undefined,
  dimension: string,
  measures: string[]
): DimensionRow[] {
  const previousByValue = new Map<string, Record<string, unknown>>()
  for (const row of previous ?? []) {
    previousByValue.set(String(row[dimension] ?? ''), row)
  }

  return current.map((row) => {
    const value = String(row[dimension] ?? '')
    const previousRow = previousByValue.get(value)
    const merged: DimensionRow = { dimension_value: value }
    for (const [key, raw] of Object.entries(row)) {
      merged[key] = typeof raw === 'number' ? raw : String(raw ?? '')
    }
    for (const measure of measures) {
      merged[measure] = readMeasure(row, measure)
      if (previousRow) {
        const previousValue = readMeasure(previousRow, measure)
        merged[`prev_${measure}`] = previousValue
        merged[`${measure}_change`] = changePercent(readMeasure(row, measure), previousValue)
      }
    }
    return merged
  })
}
