import { useMemo, type ReactNode } from 'react'
import ReactECharts from 'echarts-for-react'
import { Empty, Segmented, Skeleton } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { Panel, useNumberFormat } from '../layout'
import { colors, typography } from '../../theme/tokens'
import { BlockError } from './BlockError'
import { ChartGhost } from './ChartGhost'
import { buildOriginChart } from './originChart'
import type { DashboardMetric } from './dashboardSearch'
import type { OriginSeries } from './useOriginSeries'
import type { MessageOrigin } from './dashboardKpisModel'

interface OriginVolumeChartProps {
  data: OriginSeries
  metric: DashboardMetric
  onMetricChange: (metric: DashboardMetric) => void
  /**
   * Shown instead of the chart while there is nothing to draw yet (no email ever sent, or only
   * today's): grey bars behind the message, and the metric switch hidden.
   */
  ghost?: { message: ReactNode; action?: ReactNode }
}

const CHART_HEIGHT = 250

const SERIES_COLOR: Record<MessageOrigin, string> = {
  broadcast: colors.chart.primary,
  transactional: colors.chart.series2,
  automation: colors.chart.series3,
  other: colors.chart.series4
}

interface TooltipParam {
  axisValue: string
  seriesName: string
  value: number | null
  color: string
}

const escapeHtml = (text: string) => text.replace(/[&<>"']/g, (char) => `&#${char.charCodeAt(0)};`)

/** Emails per day by origin: broadcasts as bars, the other origins as lines. */
export function OriginVolumeChart({ data, metric, onMetricChange, ghost }: OriginVolumeChartProps) {
  const { t, i18n } = useLingui()
  const { int, compact } = useNumberFormat()

  const names: Record<MessageOrigin, string> = {
    broadcast: t`Broadcasts`,
    transactional: t`Transactional`,
    automation: t`Automations`,
    other: t`Other`
  }
  const titles: Record<DashboardMetric, string> = {
    sent: t`Emails sent per day`,
    opens: t`Opens per day`,
    clicks: t`Clicks per day`
  }
  // Under the date, saying what each figure counts: opens and clicks are those of the emails sent
  // that day, whenever they happened.
  const ofSentThatDay =
    metric === 'opens'
      ? t`Opens of the emails sent that day`
      : metric === 'clicks'
        ? t`Clicks of the emails sent that day`
        : ''

  const chart = useMemo(() => (data.series ? buildOriginChart(data.series, metric) : null), [data.series, metric])

  const option = useMemo(() => {
    if (!chart) return null
    const dayLabel = new Intl.DateTimeFormat(i18n.locale, {
      day: 'numeric',
      month: 'numeric',
      timeZone: 'UTC'
    })
    const labels = chart.days.map((day) => dayLabel.format(new Date(`${day}T00:00:00Z`)))
    const line = (origin: MessageOrigin) => ({
      name: names[origin],
      type: 'line',
      data: chart.values[origin],
      // On a log axis a zero is null, so a send between two empty days is a lone point: a line
      // alone would draw nothing there.
      symbol: 'circle',
      symbolSize: 4,
      showSymbol: chart.log,
      // Every point, not only those on a labelled day: a narrow chart would hide the lone sends.
      showAllSymbol: true,
      // Straight on a log axis: a smoothed curve overshoots between two points, and toward a day
      // of one email it dipped below 1, where no value can be.
      smooth: !chart.log,
      connectNulls: false,
      color: SERIES_COLOR[origin],
      lineStyle: { width: 1.6 }
    })
    return {
      animation: false,
      grid: { left: 44, right: 10, top: 28, bottom: 22 },
      legend: {
        top: 0,
        left: 0,
        itemWidth: 9,
        itemHeight: 9,
        icon: 'roundRect',
        textStyle: { fontSize: typography.size['2xs'], color: colors.ink2 }
      },
      tooltip: {
        trigger: 'axis',
        confine: true,
        formatter: (params: TooltipParam[]) => {
          if (!Array.isArray(params) || params.length === 0) return ''
          const rows = params.map(
            (param) =>
              `<div><span style="display:inline-block;width:8px;height:8px;border-radius:2px;background:${param.color};margin-right:6px"></span>${escapeHtml(
                param.seriesName
              )}: <b>${int(param.value ?? 0)}</b></div>`
          )
          const note = ofSentThatDay
            ? `<div style="color:${colors.ink3};margin-bottom:4px">${escapeHtml(ofSentThatDay)}</div>`
            : ''
          const date = `<div style="font-weight:600;margin-bottom:${note ? 0 : 4}px">${escapeHtml(params[0].axisValue)}</div>`
          return `${date}${note}${rows.join('')}`
        }
      },
      xAxis: {
        type: 'category',
        data: labels,
        axisTick: { show: false },
        axisLine: { lineStyle: { color: colors.line } },
        axisLabel: { fontSize: typography.size.overline, color: colors.ink3, hideOverlap: true }
      },
      yAxis: {
        type: chart.log ? 'log' : 'value',
        // Nothing below one email on a log axis.
        ...(chart.log ? { min: 1 } : {}),
        minInterval: 1,
        splitLine: { lineStyle: { color: colors.gray[100] } },
        axisLabel: {
          fontSize: typography.size.overline,
          color: colors.ink3,
          formatter: (value: number) => compact(value)
        }
      },
      series: [
        {
          name: names.broadcast,
          type: 'bar',
          data: chart.values.broadcast,
          barMaxWidth: 10,
          color: SERIES_COLOR.broadcast,
          itemStyle: { borderRadius: [2, 2, 0, 0] }
        },
        line('transactional'),
        line('automation'),
        line('other')
      ]
    }
    // names and the tooltip note follow the language, which re-renders the page anyway
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [chart, i18n.locale, int, compact, ofSentThatDay])

  let body
  if (ghost) body = <ChartGhost message={ghost.message} action={ghost.action} />
  else if (data.isError) body = <BlockError onRetry={data.refetch} error={data.error} />
  else if (!chart || !option)
    body = (
      <div data-testid="chart-loading" className="h-[250px]">
        <Skeleton active title={false} paragraph={{ rows: 7 }} />
      </div>
    )
  else if (chart.empty)
    body = (
      <div className="flex h-[250px] items-center justify-center">
        <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={t`No email sent in this period`} />
      </div>
    )
  else body = <ReactECharts option={option} notMerge style={{ height: CHART_HEIGHT }} opts={{ renderer: 'svg' }} />

  return (
    <Panel
      title={titles[metric]}
      extra={
        ghost ? undefined : (
        <Segmented
          size="small"
          value={metric}
          onChange={(value) => onMetricChange(value as DashboardMetric)}
          options={[
            { value: 'sent', label: t`Sent` },
            { value: 'opens', label: t`Opens` },
            { value: 'clicks', label: t`Clicks` }
          ]}
        />
        )
      }
    >
      {body}
    </Panel>
  )
}
