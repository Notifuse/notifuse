import { useMemo } from 'react'
import ReactECharts from 'echarts-for-react'
import { Empty } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { formatAxisValue, formatValue, formatXAxisLabel } from './lib/format'
import {
  ChartDataPoint,
  COMPARISON_COLOR,
  Granularity,
  MetricConfig,
  PRIMARY_COLOR
} from './lib/types'

interface MetricChartProps {
  metric: MetricConfig
  current: ChartDataPoint[]
  previous?: ChartDataPoint[]
  granularity: Granularity
  /** Series names, e.g. "Dec 21-27" and "Dec 14-20". */
  currentLabel: string
  previousLabel?: string
  loading?: boolean
  height?: number
  currency?: string
}

export function MetricChart(props: MetricChartProps) {
  const { t } = useLingui()
  const { metric, current, previous = [], granularity, height = 200 } = props

  const option = useMemo(() => {
    const labels = current.map((point) => formatXAxisLabel(point.timestamp, granularity))
    // Long ranges would print a tick per bucket; thin them out instead of
    // letting echarts drop labels at arbitrary positions.
    const labelInterval = labels.length > 14 ? Math.floor(labels.length / 7) - 1 : 0
    const labelRotate = labels.length > 20 ? 45 : 0

    const series: Record<string, unknown>[] = [
      {
        name: props.currentLabel,
        type: 'line',
        smooth: false,
        symbol: 'none',
        data: current.map((point) => point.value),
        lineStyle: { color: PRIMARY_COLOR, width: 2 },
        itemStyle: { color: PRIMARY_COLOR },
        areaStyle: {
          color: {
            type: 'linear',
            x: 0,
            y: 0,
            x2: 0,
            y2: 1,
            colorStops: [
              { offset: 0, color: `${PRIMARY_COLOR}40` },
              { offset: 1, color: `${PRIMARY_COLOR}05` }
            ]
          }
        }
      }
    ]

    if (previous.length > 0) {
      series.push({
        name: props.previousLabel ?? t`Previous period`,
        type: 'line',
        smooth: false,
        symbol: 'none',
        data: previous.map((point) => point.value),
        lineStyle: { color: COMPARISON_COLOR, width: 1 },
        itemStyle: { color: COMPARISON_COLOR }
      })
    }

    return {
      animation: false,
      grid: {
        left: '1%',
        right: '1%',
        bottom: labelRotate > 0 ? '10%' : '5%',
        top: '5%',
        containLabel: true
      },
      legend: { show: false },
      tooltip: {
        trigger: 'axis',
        backgroundColor: 'rgba(255, 255, 255, 0.95)',
        borderColor: '#e5e7eb',
        textStyle: { color: '#374151', fontSize: 12 },
        formatter: (params: { axisValue: string; value: number; seriesName: string }[]) => {
          if (!Array.isArray(params) || params.length === 0) return ''
          const currentValue = Number(params[0].value ?? 0)
          const rows = [`<div style="font-weight:600;margin-bottom:4px">${params[0].axisValue}</div>`]
          const bullet = (color: string) =>
            `<span style="display:inline-block;width:8px;height:8px;border-radius:50%;background:${color};margin-right:6px"></span>`

          rows.push(
            `<div>${bullet(PRIMARY_COLOR)}${params[0].seriesName}: <b>${formatValue(
              currentValue,
              metric.format,
              props.currency
            )}</b></div>`
          )

          if (params.length > 1) {
            const previousValue = Number(params[1].value ?? 0)
            const delta = previousValue !== 0 ? ((currentValue - previousValue) / previousValue) * 100 : 0
            const better = metric.invertTrend ? delta <= 0 : delta >= 0
            const sign = delta >= 0 ? '+' : ''
            rows.push(
              `<div>${bullet(COMPARISON_COLOR)}${params[1].seriesName}: <b>${formatValue(
                previousValue,
                metric.format,
                props.currency
              )}</b>` +
                (delta !== 0
                  ? ` <span style="color:${better ? '#10b981' : '#ef4444'}">${sign}${delta.toFixed(
                      1
                    )}%</span>`
                  : '') +
                `</div>`
            )
          }
          return rows.join('')
        }
      },
      xAxis: {
        type: 'category',
        boundaryGap: false,
        data: labels,
        axisTick: { show: false },
        axisLine: { lineStyle: { color: '#e5e7eb' } },
        axisLabel: {
          color: '#6b7280',
          fontSize: 10,
          interval: labelInterval,
          rotate: labelRotate
        }
      },
      yAxis: {
        type: 'value',
        axisLine: { show: false },
        axisTick: { show: false },
        splitLine: { lineStyle: { color: '#f3f4f6' } },
        axisLabel: {
          color: '#6b7280',
          fontSize: 10,
          formatter: (value: number) => formatAxisValue(value, metric.format)
        }
      },
      series
    }
  }, [current, previous, granularity, metric, props.currentLabel, props.previousLabel, props.currency, t])

  if (props.loading) {
    return <div className="animate-pulse rounded bg-gray-100" style={{ height }} />
  }

  if (current.length === 0) {
    return (
      <div className="flex items-center justify-center" style={{ height }}>
        <Empty description={t`No data for this period`} image={Empty.PRESENTED_IMAGE_SIMPLE} />
      </div>
    )
  }

  return (
    <ReactECharts
      option={option}
      style={{ height }}
      opts={{ renderer: 'svg' }}
      notMerge
    />
  )
}
