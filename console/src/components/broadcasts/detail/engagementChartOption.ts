import { colors, typography } from '../../../theme/tokens'
import { type EngagementGranularity } from '../broadcastAnalytics'
import { type Bucket } from './fillBuckets'

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
  const axisLabel = (key: string) =>
    granularity === 'hour' ? `${key.slice(5, 10)} ${key.slice(11, 16)}` : key.slice(5, 10)
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
          data: markers
            .filter((m) => keys.includes(m.key))
            .map((m) => ({ name: m.label, xAxis: m.key }))
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
