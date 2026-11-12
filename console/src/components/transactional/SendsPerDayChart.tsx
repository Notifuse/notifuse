import { useMemo } from 'react'
import ReactECharts from 'echarts-for-react'
import { Empty, Segmented, Skeleton } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { Panel, useNumberFormat } from '../layout'
import { colors } from '../../theme/tokens'
import { LoadError } from './LoadError'
import type { DailyPoint } from './transactionalStats'
import type { TransactionalPeriod } from './transactionalSearch'
import { formatSeriesDay } from './transactionalTime'

interface SendsPerDayChartProps {
  points?: DailyPoint[]
  loading: boolean
  error?: unknown
  onRetry: () => void
  /** SMTP reports no delivery nor bounce: the bars are Sent and Failed. */
  smtp: boolean
  /** The workspace's today, YYYY-MM-DD: its bar is partial. */
  today: string
  period: TransactionalPeriod
  onPeriodChange: (period: TransactionalPeriod) => void
}

export const SENDS_CHART_HEIGHT = 210

interface TooltipParam {
  dataIndex: number
}

const escapeHtml = (text: string) => text.replace(/[&<>"']/g, (char) => `&#${char.charCodeAt(0)};`)

/** The notification's sends per day, delivered against bounced or failed. */
export function SendsPerDayChart({
  points,
  loading,
  error,
  onRetry,
  smtp,
  today,
  period,
  onPeriodChange
}: SendsPerDayChartProps) {
  const { t, i18n } = useLingui()
  const { int, compact } = useNumberFormat()

  const labels = {
    sent: t`Sent`,
    delivered: t`Delivered`,
    bounced: t`Bounced`,
    failed: t`Failed`,
    bouncedOrFailed: t`Bounced or failed`,
    today: t`Today, so far`
  }

  const empty = !points || points.every((point) => point.sent === 0 && point.failed === 0)

  const option = useMemo(() => {
    if (!points) return null
    const axis = points.map((point) => formatSeriesDay(point.day, i18n.locale))
    const stack = (name: string, data: number[], color: string, top: boolean) => ({
      name,
      type: 'bar',
      stack: 'sends',
      data,
      barMaxWidth: 14,
      color,
      itemStyle: top ? { borderRadius: [2, 2, 0, 0] } : undefined
    })
    const series = smtp
      ? [
          stack(
            labels.sent,
            points.map((point) => point.sent),
            colors.primary[400],
            false
          ),
          stack(
            labels.failed,
            points.map((point) => point.failed),
            colors.status.warning,
            true
          )
        ]
      : [
          stack(
            labels.delivered,
            points.map((point) => point.delivered),
            colors.primary[400],
            false
          ),
          stack(
            labels.bouncedOrFailed,
            points.map((point) => point.bounced + point.failed),
            colors.status.warning,
            true
          )
        ]
    return {
      animation: false,
      grid: { left: 40, right: 8, top: 28, bottom: 22 },
      legend: {
        top: 0,
        left: 0,
        itemWidth: 9,
        itemHeight: 9,
        icon: 'roundRect',
        textStyle: { fontSize: 11.5, color: colors.ink2 }
      },
      tooltip: {
        trigger: 'axis',
        confine: true,
        axisPointer: { type: 'shadow' },
        formatter: (params: TooltipParam[]) => {
          if (!Array.isArray(params) || params.length === 0) return ''
          const point = points[params[0].dataIndex]
          if (!point) return ''
          const rows: [string, number][] = smtp
            ? [
                [labels.sent, point.sent],
                [labels.failed, point.failed]
              ]
            : [
                [labels.sent, point.sent],
                [labels.delivered, point.delivered],
                [labels.bounced, point.bounced],
                [labels.failed, point.failed]
              ]
          const head = escapeHtml(formatSeriesDay(point.day, i18n.locale))
          const note = point.day === today ? `<div style="color:${colors.ink3}">${escapeHtml(labels.today)}</div>` : ''
          return `<div style="font-weight:600;margin-bottom:4px">${head}</div>${rows
            .map(([name, value]) => `<div>${escapeHtml(name)}: <b>${int(value)}</b></div>`)
            .join('')}${note}`
        }
      },
      xAxis: {
        type: 'category',
        data: axis,
        axisTick: { show: false },
        axisLine: { lineStyle: { color: colors.line } },
        axisLabel: { fontSize: 10.5, color: colors.ink3, hideOverlap: true }
      },
      yAxis: {
        type: 'value',
        minInterval: 1,
        splitLine: { lineStyle: { color: colors.gray[100] } },
        axisLabel: { fontSize: 10.5, color: colors.ink3, formatter: (value: number) => compact(value) }
      },
      series
    }
    // the labels follow the language, which re-renders the sheet anyway
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [points, smtp, today, i18n.locale, int, compact])

  let body
  if (error) body = <LoadError onRetry={onRetry} error={error} />
  else if (loading || !option)
    body = (
      <div data-testid="sends-chart-loading" className="h-[210px]">
        <Skeleton active title={false} paragraph={{ rows: 6 }} />
      </div>
    )
  else if (empty)
    body = (
      <div className="flex h-[210px] items-center justify-center">
        <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={t`No sends in this period`} />
      </div>
    )
  else
    body = <ReactECharts option={option} notMerge style={{ height: SENDS_CHART_HEIGHT }} opts={{ renderer: 'svg' }} />

  return (
    <Panel
      title={t`Sends per day`}
      extra={
        <Segmented<TransactionalPeriod>
          size="small"
          value={period}
          onChange={onPeriodChange}
          options={[
            { label: t`7d`, value: '7d' },
            { label: t`30d`, value: '30d' },
            { label: t`60d`, value: '60d' }
          ]}
        />
      }
    >
      {body}
    </Panel>
  )
}
