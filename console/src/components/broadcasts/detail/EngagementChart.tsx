import { useMemo, useState } from 'react'
import ReactECharts from 'echarts-for-react'
import { Segmented, Skeleton } from 'antd'
import { useQuery } from '@tanstack/react-query'
import { useLingui } from '@lingui/react/macro'
import dayjs from '../../../lib/dayjs'
import type { Broadcast } from '../../../services/api/broadcast'
import { Panel } from '../../layout'
import { engagementQuery, ENGAGEMENT_COLUMNS, type EngagementGranularity, type EngagementKind } from '../broadcastAnalytics'
import { runBroadcastQuery } from '../broadcastQueryClient'
import { timeBucketColumn } from '../../web_analytics/lib/dates'
import { bucketKey, fillBuckets } from './fillBuckets'
import { CouldntLoad } from '../CouldntLoad'
import { buildChartOption, type ChartMarker } from './engagementChartOption'

export type ChartPeriod = '48h' | '7d' | '30d'

const PERIODS: Record<ChartPeriod, { hours: number; granularity: EngagementGranularity }> = {
  '48h': { hours: 48, granularity: 'hour' },
  '7d': { hours: 7 * 24, granularity: 'hour' },
  '30d': { hours: 30 * 24, granularity: 'day' }
}

export interface EngagementChartProps {
  workspaceId: string
  broadcast: Broadcast
  timezone: string
  live: boolean
}

/** F7 and F8: first opens and first clicks over 48 h, 7 d or 30 d from the start. */
export function EngagementChart({ workspaceId, broadcast, timezone, live }: EngagementChartProps) {
  const { t } = useLingui()
  const [period, setPeriod] = useState<ChartPeriod>('48h')
  const { hours, granularity } = PERIODS[period]
  const start = useMemo(
    () => (broadcast.started_at ? dayjs(broadcast.started_at).tz(timezone).startOf('hour') : null),
    [broadcast.started_at, timezone]
  )
  const end = start ? start.add(hours, 'hour') : null

  const read = (kind: EngagementKind) => ({
    queryKey: ['broadcast-hourly', workspaceId, broadcast.id, kind, period],
    queryFn: () =>
      runBroadcastQuery(
        engagementQuery({ broadcastId: broadcast.id, kind, granularity, start: start!, end: end!, timezone }),
        workspaceId
      ),
    enabled: start !== null,
    refetchInterval: live ? 30_000 : (false as const)
  })
  const opensQuery = useQuery(read('opens'))
  const clicksQuery = useQuery(read('clicks'))

  const option = useMemo(() => {
    if (!start || !end) return null
    const fill = (kind: EngagementKind, rows: Record<string, unknown>[]) =>
      fillBuckets({
        start,
        end,
        granularity,
        timezone,
        rows,
        column: timeBucketColumn(ENGAGEMENT_COLUMNS[kind].dimension, granularity),
        measure: ENGAGEMENT_COLUMNS[kind].measure
      })
    const markers: ChartMarker[] = []
    if (broadcast.test_sent_at) markers.push({ key: bucketKey(broadcast.test_sent_at, granularity, timezone), label: t`Test sent` })
    if (broadcast.winner_sent_at)
      markers.push({ key: bucketKey(broadcast.winner_sent_at, granularity, timezone), label: t`Winner sent` })
    return buildChartOption({
      opens: fill('opens', opensQuery.data?.data ?? []),
      clicks: fill('clicks', clicksQuery.data?.data ?? []),
      granularity,
      markers,
      labels: { opens: t`First opens`, clicks: t`First clicks` }
    })
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [opensQuery.data, clicksQuery.data, period, timezone, broadcast.test_sent_at, broadcast.winner_sent_at, start?.valueOf()])

  if (!start) return null

  return (
    <Panel
      title={t`Opens and clicks over time`}
      extra={
        <Segmented<ChartPeriod>
          size="small"
          value={period}
          onChange={setPeriod}
          options={[
            { value: '48h', label: t`48 h` },
            { value: '7d', label: t`7 d` },
            { value: '30d', label: t`30 d` }
          ]}
        />
      }
    >
      <p data-unreset className="m-0 mb-2 text-xs text-ink-3">
        {t`First opens counts the messages opened or clicked, once each; first clicks, the messages clicked once each.`}
      </p>
      {opensQuery.isError || clicksQuery.isError ? (
        <CouldntLoad />
      ) : opensQuery.isLoading || clicksQuery.isLoading || !option ? (
        <Skeleton active paragraph={{ rows: 4 }} />
      ) : (
        <ReactECharts option={option} style={{ height: 220 }} opts={{ renderer: 'svg' }} notMerge />
      )}
    </Panel>
  )
}
