import { useMemo, useState } from 'react'
import ReactECharts from 'echarts-for-react'
import { Segmented, Skeleton } from 'antd'
import { useQuery } from '@tanstack/react-query'
import { useLingui } from '@lingui/react/macro'
import dayjs from '../../../lib/dayjs'
import type { Broadcast } from '../../../services/api/broadcast'
import { HelpTip, Panel, useNumberFormat } from '../../layout'
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

/**
 * F7 and F8: opens and clicks per hour (48 h, 7 d) or per day (30 d) from the start. History keeps
 * a message's first open and first click only, so each message counts once, when it was first opened.
 */
export function EngagementChart({ workspaceId, broadcast, timezone, live }: EngagementChartProps) {
  const { t, i18n } = useLingui()
  const { int } = useNumberFormat()
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
    // While the period is under way, a marker says where it stands: the buckets after it are empty.
    const now = dayjs()
    if (end.isAfter(now)) markers.push({ key: bucketKey(now, granularity, timezone), label: t`Now` })
    if (broadcast.test_sent_at) markers.push({ key: bucketKey(broadcast.test_sent_at, granularity, timezone), label: t`Test sent` })
    if (broadcast.winner_sent_at)
      markers.push({
        key: bucketKey(broadcast.winner_sent_at, granularity, timezone),
        label: t({ message: 'Winner sent', context: 'newsletter A/B' })
      })
    return buildChartOption({
      opens: fill('opens', opensQuery.data?.data ?? []),
      clicks: fill('clicks', clicksQuery.data?.data ?? []),
      granularity,
      markers,
      labels: { opens: t`Opens`, clicks: t`Clicks` },
      locale: i18n.locale,
      formatCount: int,
      periodHours: hours
    })
    // The language is a dependency: the axis, the tooltip and the labels are written in it.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [opensQuery.data, clicksQuery.data, period, timezone, broadcast.test_sent_at, broadcast.winner_sent_at, start?.valueOf(), i18n.locale])

  if (!start) return null

  return (
    <Panel
      title={
        <HelpTip
          title={granularity === 'hour' ? t`Opens and clicks per hour` : t`Opens and clicks per day`}
          label={t`About these numbers`}
        >
          {granularity === 'hour'
            ? t`Each message counts once, in the hour of its first open or click; clicks count each message clicked once.`
            : t`Each message counts once, on the day of its first open or click; clicks count each message clicked once.`}
        </HelpTip>
      }
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
