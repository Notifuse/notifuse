import { useMemo } from 'react'
import { keepPreviousData, useQuery } from '@tanstack/react-query'
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
} from './dashboardKpisModel'

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
  /** Some origin still shows the previous period while the new one loads. */
  isPlaceholderData: boolean
  /** A request failed with nothing loaded before; a failed refresh keeps the last figures. */
  isError: boolean
  error?: unknown
  refetch: () => void
}

/** The rows of one origin, from the answer split by origin. */
const rowsOf = (rows: Record<string, unknown>[], origin: MessageOrigin) =>
  rows.filter((row) => row.origin === origin)

/**
 * Emails per day for each origin, in one request split by the `origin` dimension. The server fills
 * the days without data for every origin it saw in the range; an origin with no email in it has no
 * row and counts as zero every day (the chart reads the days of all origins). The totals of the
 * period are the sums of the series.
 */
export function useOriginSeries({ workspaceId, timezone, range, enabled }: UseOriginSeriesOptions): OriginSeries {
  const query = useQuery({
    queryKey: ['dashboard', 'series', workspaceId, timezone, range.startDay, range.endDay],
    queryFn: () =>
      dashboardAnalytics.query(
        {
          schema: 'message_history',
          measures: Object.values(MEASURES),
          dimensions: ['origin'],
          timezone,
          timeDimensions: [
            {
              dimension: 'created_at',
              granularity: 'day',
              dateRange: [range.startDay, range.endDay]
            }
          ],
          filters: [{ member: 'channel', operator: 'equals', values: ['email'] }]
        },
        workspaceId
      ),
    enabled,
    refetchInterval: DASHBOARD_REFRESH_MS,
    placeholderData: keepPreviousData
  })

  const { data, isLoading, isPlaceholderData, isError, error, refetch } = query
  // Built once per answer: the chart is not rebuilt on every tick of the page.
  const parsed = useMemo(() => {
    if (!data) return undefined
    const rows = data.data ?? []
    const series = {} as Record<MessageOrigin, SeriesPoint[]>
    const totals = {} as Record<MessageOrigin, MessageTotals>
    for (const origin of MESSAGE_ORIGINS) {
      series[origin] = parseSeries(rowsOf(rows, origin))
      totals[origin] = sumSeries(series[origin])
    }
    return { series, totals }
  }, [data])

  const failed = isError && !data
  return {
    ...(parsed && !failed ? parsed : {}),
    isLoading,
    isPlaceholderData,
    isError: failed,
    error: failed ? error : undefined,
    refetch: () => void refetch()
  }
}
