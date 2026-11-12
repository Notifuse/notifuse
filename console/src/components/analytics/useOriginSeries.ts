import { keepPreviousData, useQueries, type UseQueryResult } from '@tanstack/react-query'
import type { AnalyticsResponse } from '../../services/api/analytics'
import type { ResolvedRange } from '../web_analytics/lib/types'
import { DASHBOARD_REFRESH_MS, dashboardAnalytics } from './dashboardClient'
import {
  MEASURES,
  MESSAGE_ORIGINS,
  parseSeries,
  sumSeries,
  type MessageOrigin,
  type MessageTotals,
  type SeriesPoint
} from './dashboardKpis'

interface UseOriginSeriesOptions {
  workspaceId: string
  timezone: string
  range: ResolvedRange
  enabled: boolean
}

export interface OriginSeries {
  series?: Record<MessageOrigin, SeriesPoint[]>
  totals?: Record<MessageOrigin, MessageTotals>
  isLoading: boolean
  /** A request failed with nothing loaded before; a failed refresh keeps the last figures. */
  isError: boolean
  error?: unknown
  refetch: () => void
}

/**
 * Combined inside useQueries, which runs it again only when a result changes: the series keep
 * their identity across renders, so the chart is not rebuilt on every tick of the page.
 */
function combine(results: UseQueryResult<AnalyticsResponse>[]): OriginSeries {
  const failed = results.find((result) => result.isError && !result.data)
  const isLoading = results.some((result) => result.isLoading)
  const isError = Boolean(failed)
  const refetch = () => {
    for (const result of results) void result.refetch()
  }

  if (isLoading || isError || results.some((result) => !result.data)) {
    return { isLoading, isError, error: failed?.error, refetch }
  }

  const series = {} as Record<MessageOrigin, SeriesPoint[]>
  const totals = {} as Record<MessageOrigin, MessageTotals>
  MESSAGE_ORIGINS.forEach((origin, index) => {
    series[origin] = parseSeries(results[index].data?.data)
    totals[origin] = sumSeries(series[origin])
  })
  return { series, totals, isLoading, isError, refetch }
}

/**
 * Emails per day for each origin, one request per origin: with a dimension next to the time
 * dimension, the engine's gap filler keeps one row per day and two origins of the same day would
 * overwrite each other. The totals of the period are the sums of the three series.
 */
export function useOriginSeries({ workspaceId, timezone, range, enabled }: UseOriginSeriesOptions): OriginSeries {
  return useQueries({
    queries: MESSAGE_ORIGINS.map((origin) => ({
      queryKey: ['dashboard', 'series', workspaceId, timezone, range.startDay, range.endDay, origin],
      queryFn: () =>
        dashboardAnalytics.query(
          {
            schema: 'message_history',
            measures: Object.values(MEASURES),
            dimensions: [],
            timezone,
            timeDimensions: [
              {
                dimension: 'created_at',
                granularity: 'day',
                dateRange: [range.startDay, range.endDay]
              }
            ],
            filters: [
              { member: 'origin', operator: 'equals', values: [origin] },
              { member: 'channel', operator: 'equals', values: ['email'] }
            ]
          },
          workspaceId
        ),
      enabled,
      refetchInterval: DASHBOARD_REFRESH_MS,
      placeholderData: keepPreviousData
    })),
    combine
  })
}
