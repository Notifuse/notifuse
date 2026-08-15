import { useMemo } from 'react'
import ReactECharts from 'echarts-for-react'
import { Empty } from 'antd'
import { useQuery } from '@tanstack/react-query'
import { useLingui } from '@lingui/react/macro'
import {
  Annotation,
  ANNOTATION_LIST_MAX,
  ANNOTATIONS_QUERY_KEY,
  annotationService
} from '../../services/api/annotation'
import {
  bucketIndexForAnnotation,
  escapeTooltipHTML,
  sanitizeRichLabelText
} from './lib/annotations'
import { formatAxisValue, formatValue, formatXAxisLabel } from './lib/format'
import {
  ChartDataPoint,
  COMPARISON_COLOR,
  Granularity,
  MetricConfig,
  PRIMARY_COLOR,
  ResolvedRange
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
  /**
   * Vertical markers on the current series. Deliberately not drawn on the
   * comparison series: that one covers a different period, so a date line
   * there would mark a moment it does not contain.
   */
  annotations?: Annotation[]
  /**
   * The QUERY timezone the buckets were computed in — the toolbar can override
   * it away from the workspace's. Required whenever `annotations` is non-empty:
   * a bucket timestamp is a wall clock and cannot be placed on a real timeline
   * without the zone it was truncated in.
   */
  timezone?: string
}

/** Stable empty result, so an idle fetch does not re-run the chart's memo. */
const NO_ANNOTATIONS: Annotation[] = []

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

    // Placed by bucket INDEX, never by axis label: at hour granularity
    // `formatXAxisLabel` prints "3am" once per day, and echarts resolves a
    // category name through a hash map where a repeat overwrites its
    // predecessor — so a label would drop every annotation on the last match.
    const marks = (props.annotations ?? [])
      .map((annotation) => ({
        annotation,
        index: bucketIndexForAnnotation(annotation, current, granularity, props.timezone)
      }))
      .filter(
        (mark): mark is { annotation: Annotation; index: number } => mark.index !== null
      )

    // The one grouping both the markLine and the tooltip read. A Record keyed
    // by label would reintroduce the collision above in both.
    const annotationsByIndex = new Map<number, Annotation[]>()
    for (const mark of marks) {
      const existing = annotationsByIndex.get(mark.index)
      if (existing) existing.push(mark.annotation)
      else annotationsByIndex.set(mark.index, [mark.annotation])
    }

    // One vertical per BUCKET rather than per annotation, off the same map the
    // tooltip reads: the fetch asks for up to ANNOTATION_LIST_MAX rows, and a
    // busy workspace would otherwise pile hundreds of dotted lines and titles
    // onto a dozen buckets. Sorted by index so the alternating label side below
    // follows the x-order it exists to separate.
    const collapsed = [...annotationsByIndex.entries()].sort(([a], [b]) => a - b)

    if (collapsed.length > 0) {
      series[0].markLine = {
        silent: true,
        symbol: 'none',
        animation: false,
        data: collapsed.map(([index, group], position) => {
          const [first] = group
          const hidden = group.length - 1
          return {
            xAxis: index,
            name: first.id,
            lineStyle: { color: first.color, type: 'dotted', width: 1 },
            label: {
              show: true,
              // Alternating keeps two annotations a few buckets apart from
              // printing their titles over each other.
              position: position % 2 === 0 ? 'insideEndTop' : 'insideEndBottom',
              // A rich label, so `{`, `}` and `|` in the title are markup here
              // and have to go — a different problem from the HTML escaping the
              // tooltip needs. "+N" stands for the rest of the bucket, which
              // the tooltip lists in full.
              formatter: `{dot|●}{title|${sanitizeRichLabelText(first.title)}${
                hidden > 0 ? ` +${hidden}` : ''
              }}`,
              rich: {
                dot: { color: first.color, fontSize: 12, padding: [0, 4, 0, 0] },
                title: { color: '#6b7280', fontSize: 10 }
              }
            }
          }
        })
      }
    }

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
        // Annotation titles print inside the plot area at its top edge; without
        // the extra headroom they sit on the series.
        top: collapsed.length > 0 ? '18%' : '5%',
        containLabel: true
      },
      legend: { show: false },
      tooltip: {
        trigger: 'axis',
        // An annotation description is free text up to 500 characters, and the
        // tooltip box sizes to max-content: without a width it renders as one
        // unbroken line far wider than the chart and runs off both edges of the
        // viewport. Capping the width is what makes the rows wrap at all;
        // confine then keeps the taller box inside the chart instead of over
        // the page. break-word covers a pasted URL, which has no space to break at.
        confine: true,
        extraCssText: 'max-width:320px;white-space:normal;word-break:break-word;',
        backgroundColor: 'rgba(255, 255, 255, 0.95)',
        borderColor: '#e5e7eb',
        textStyle: { color: '#374151', fontSize: 12 },
        formatter: (
          params: { axisValue: string; value: number; seriesName: string; dataIndex: number }[]
        ) => {
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

          // Titles and descriptions are operator-typed text landing in an HTML
          // string; escape with echarts' own escaper so the tooltip and the
          // library agree on what counts as markup.
          for (const annotation of annotationsByIndex.get(params[0].dataIndex) ?? []) {
            rows.push(
              `<div style="margin-top:4px">${bullet(
                escapeTooltipHTML(annotation.color)
              )}${escapeTooltipHTML(annotation.title)}</div>`
            )
            if (annotation.description) {
              rows.push(
                `<div style="margin-left:14px;color:#6b7280">${escapeTooltipHTML(
                  annotation.description
                )}</div>`
              )
            }
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
  }, [
    current,
    previous,
    granularity,
    metric,
    props.currentLabel,
    props.previousLabel,
    props.currency,
    // Annotations are fetched separately and land after the first paint; both
    // of these have to be here or the chart keeps the memo it built without them.
    props.annotations,
    props.timezone,
    t
  ])

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

/**
 * The annotations covering a chart's range, ready for the `annotations` prop.
 *
 * It lives beside the chart because both call sites already import this module
 * for `MetricChart`, and the fetch has no other consumer.
 *
 * The key carries the range, so the annotations tab invalidating
 * `[ANNOTATIONS_QUERY_KEY, workspaceId]` prefix-matches every range currently
 * cached instead of only the one range it happens to know about.
 */
// eslint-disable-next-line react-refresh/only-export-components -- Hook co-located with the chart it feeds
export function useAnnotations(
  workspaceId: string,
  range: ResolvedRange,
  options?: { enabled?: boolean }
): Annotation[] {
  const enabled = options?.enabled ?? true

  const query = useQuery({
    queryKey: [ANNOTATIONS_QUERY_KEY, workspaceId, range.startUtc, range.endUtc],
    queryFn: () =>
      annotationService.list({
        workspace_id: workspaceId,
        start: range.startUtc,
        end: range.endUtc,
        // Left out, the endpoint falls back to 100 rows applied AFTER ordering
        // by `annotated_at` descending, so a workspace with more than that
        // would lose its oldest marks off the chart with nothing to signal it.
        limit: ANNOTATION_LIST_MAX
      }),
    enabled: enabled && Boolean(workspaceId),
    staleTime: 60_000
  })

  return query.data ?? NO_ANNOTATIONS
}
