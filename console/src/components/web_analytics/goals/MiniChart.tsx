import { useMemo } from 'react'
import ReactECharts from 'echarts-for-react'
import { ChartDataPoint, COMPARISON_COLOR, PRIMARY_COLOR } from '../lib/types'

interface MiniChartProps {
  current: ChartDataPoint[]
  /** Comparison period, drawn as a thin grey line when the page compares. */
  previous?: ChartDataPoint[]
  height?: number
}

/**
 * The sparkline on a goal card: no axes, no tooltip, no legend.
 *
 * The KPI tiles above it already carry the exact figures, so the chart only has
 * to answer "which way is this going" while the eye is scanning a grid of
 * cards. Anything readable enough to invite a hover belongs in the drawer,
 * where the full chart lives.
 */
export function MiniChart({ current, previous = [], height = 80 }: MiniChartProps) {
  const option = useMemo(() => {
    const series: Record<string, unknown>[] = [
      {
        type: 'line',
        smooth: true,
        symbol: 'none',
        data: current.map((point) => point.value),
        lineStyle: { color: PRIMARY_COLOR, width: 2 },
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
        type: 'line',
        smooth: true,
        symbol: 'none',
        data: previous.map((point) => point.value),
        lineStyle: { color: COMPARISON_COLOR, width: 1 }
      })
    }

    return {
      animation: false,
      grid: { left: 0, right: 0, top: 5, bottom: 5 },
      xAxis: {
        type: 'category',
        show: false,
        boundaryGap: false,
        data: current.map((point) => point.timestamp)
      },
      yAxis: { type: 'value', show: false },
      tooltip: { show: false },
      series
    }
  }, [current, previous])

  return <ReactECharts option={option} style={{ height }} opts={{ renderer: 'svg' }} notMerge />
}
