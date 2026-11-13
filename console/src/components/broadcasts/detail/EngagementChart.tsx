import { useMemo, useState } from 'react'
import ReactECharts from 'echarts-for-react'
import { Segmented, Skeleton } from 'antd'
import { useQuery } from '@tanstack/react-query'
import { useLingui } from '@lingui/react/macro'
import dayjs from '../../../lib/dayjs'
import { colors, typography } from '../../../theme/tokens'
import type { Broadcast } from '../../../services/api/broadcast'
import { Panel } from '../../layout'
import { engagementQuery, ENGAGEMENT_COLUMNS, type EngagementGranularity, type EngagementKind } from '../broadcastAnalytics'
import { runBroadcastQuery } from '../broadcastQueryClient'
import { timeBucketColumn } from '../../web_analytics/lib/dates'
import { bucketKey, fillBuckets, type Bucket } from './fillBuckets'
import { CouldntLoad } from '../CouldntLoad'

export type ChartPeriod = '48h' | '7d' | '30d'

export const PERIODS: Record<ChartPeriod, { hours: number; granularity: EngagementGranularity }> = {
  '48h': { hours: 48, granularity: 'hour' },
  '7d': { hours: 7 * 24, granularity: 'hour' },
  '30d': { hours: 30 * 24, granularity: 'day' }
}

export interface ChartMarker {
  key: string
  label: string
}

/** The echarts option: bars of first opens, a line of first clicks, dashed markers. Pure. */
export function buildChartOption({
  opens,
  clicks,
  granularity,
  markers,
  labels
}: {
  opens: Bucket[]
  clicks: Bucket[]
  granularity: EngagementGranularity
  markers: ChartMarker[]
  labels: { opens: string; clicks: string }
}) {
  const keys = opens.map((b) => b.key)
  // The key is local time written with a Z: its text is read, never converted.
  const axisLabel = (key: string) => (granularity === 'hour' ? `${key.slice(5, 10)} ${key.slice(11, 16)}` : key.slice(5, 10))
  return {
    animation: false,
    textStyle: { fontFamily: typography.sans },
    grid: { left: 40, right: 16, top: 32, bottom: 28 },
    legend: { top: 0, left: 0, itemWidth: 10, itemHeight: 10, data: [labels.opens, labels.clicks] },
    tooltip: { trigger: 'axis' },
    xAxis: {
      type: 'category',
      data: keys,
      axisLabel: { formatter: axisLabel, color: colors.ink3, hideOverlap: true },
      axisLine: { lineStyle: { color: colors.line } },
      axisTick: { show: false }
    },
    yAxis: {
      type: 'value',
      minInterval: 1,
      axisLabel: { color: colors.ink3 },
      splitLine: { lineStyle: { color: colors.lineSoft } }
    },
    series: [
      {
        name: labels.opens,
        type: 'bar',
        data: opens.map((b) => b.value),
        itemStyle: { color: colors.chart.primary, borderRadius: [2, 2, 0, 0] },
        barMaxWidth: 14,
        markLine: {
          symbol: 'none',
          silent: true,
          lineStyle: { type: 'dashed', color: colors.ink3 },
          label: { formatter: '{b}', color: colors.ink2, position: 'insideEndTop' },
          data: markers.filter((m) => keys.includes(m.key)).map((m) => ({ name: m.label, xAxis: m.key }))
        }
      },
      {
        name: labels.clicks,
        type: 'line',
        data: clicks.map((b) => b.value),
        smooth: false,
        symbol: 'none',
        lineStyle: { color: colors.chart.series2, width: 2 },
        itemStyle: { color: colors.chart.series2 }
      }
    ]
  }
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
