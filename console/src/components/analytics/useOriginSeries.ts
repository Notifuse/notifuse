import { keepPreviousData, useQueries } from '@tanstack/react-query'
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
  isError: boolean
  refetch: () => void
}

/**
 * Emails per day for each origin, one request per origin: with a dimension next to the time
 * dimension, the engine's gap filler keeps one row per day and two origins of the same day would
 * overwrite each other. The totals of the period are the sums of the three series.
 */
export function useOriginSeries({ workspaceId, timezone, range, enabled }: UseOriginSeriesOptions): OriginSeries {
  const results = useQueries({
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
              { dimension: 'created_at', granularity: 'day', dateRange: [range.startDay, range.endDay] }
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
    }))
  })

  const isLoading = results.some((result) => result.isLoading)
  const isError = results.some((result) => result.isError)
  const refetch = () => {
    for (const result of results) void result.refetch()
  }

  if (isLoading || isError || results.some((result) => !result.data)) {
    return { isLoading, isError, refetch }
  }

  const series = {} as Record<MessageOrigin, SeriesPoint[]>
  const totals = {} as Record<MessageOrigin, MessageTotals>
  MESSAGE_ORIGINS.forEach((origin, index) => {
    series[origin] = parseSeries(results[index].data?.data)
    totals[origin] = sumSeries(series[origin])
  })
  return { series, totals, isLoading, isError, refetch }
}
