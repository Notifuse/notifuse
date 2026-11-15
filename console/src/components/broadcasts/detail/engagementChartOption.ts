import { colors, typography } from '../../../theme/tokens'
import { type EngagementGranularity } from '../broadcastAnalytics'
import { type Bucket } from './fillBuckets'

export interface ChartMarker {
  key: string
  label: string
}

// The key is local time written with a Z (fillBuckets.ts): formatting it in UTC reads its text as
// is, where any other zone would shift it by that zone's offset.
const AS_WRITTEN = 'UTC'

/** The hour of a bucket key, read from its text. */
const hourOf = (key: string) => Number(key.slice(11, 13))

const FORMATS = {
  hour: { hour: 'numeric' },
  day: { day: 'numeric', month: 'short' },
  hourHeading: { weekday: 'short', day: 'numeric', month: 'short', hour: 'numeric' },
  dayHeading: { weekday: 'short', day: 'numeric', month: 'short' }
} satisfies Record<string, Intl.DateTimeFormatOptions>

// ECharts formats every label on each layout and the tooltip on each pointer move: each formatter
// is built once per language.
const formatters = new Map<string, Intl.DateTimeFormat>()
function format(locale: string, kind: keyof typeof FORMATS, key: string): string {
  const id = `${locale}|${kind}`
  let formatter = formatters.get(id)
  if (!formatter) {
    formatter = new Intl.DateTimeFormat(locale, { ...FORMATS[kind], timeZone: AS_WRITTEN })
    formatters.set(id, formatter)
  }
  return formatter.format(new Date(key))
}

/**
 * The label of an axis key: the hour alone ("15 h", "3 PM"), the day at midnight ("11 nov.");
 * a day bucket is its day.
 */
export function axisLabel(key: string, granularity: EngagementGranularity, locale: string): string {
  if (granularity === 'hour' && hourOf(key) !== 0) {
    // Some languages write the hour on two digits ("06 h"): the axis reads "6 h".
    return format(locale, 'hour', key).replace(/^0(\d)/, '$1')
  }
  return format(locale, 'day', key)
}

/** The heading of the tooltip of a bucket: its weekday, day and hour, or its day. */
export function tooltipHeading(key: string, granularity: EngagementGranularity, locale: string): string {
  return format(locale, granularity === 'hour' ? 'hourHeading' : 'dayHeading', key)
}

/**
 * The hours between two labels of an hourly axis: a divisor of 24, so labels fall on the same
 * hours every day, and midnight, which names the day, is always one of them. Over 48 h, every 6
 * hours; over a week, only the days.
 */
export function hourStep(periodHours: number): number {
  return periodHours <= 72 ? 6 : 24
}

/**
 * Category indexes that get a label: on an hourly axis those on the step of the period (not of the
 * buckets so far: a recent broadcast has few, and its week view would label like its 48 h one);
 * on a daily axis, all.
 */
export function labelledIndex(
  keys: string[],
  granularity: EngagementGranularity,
  periodHours: number
): (index: number) => boolean {
  if (granularity !== 'hour') return () => true
  const step = hourStep(periodHours)
  const onStep = (index: number) => hourOf(keys[index]) % step === 0
  // A broadcast started minutes ago may have no hour on the step yet: its first hour then. Only
  // then, so it never crowds out the midnight that names the first day.
  const none = !keys.some((_, index) => onStep(index))
  return (index) => onStep(index) || (none && index === 0)
}

// ECharts inserts the tooltip as HTML: a translated series name is text.
const escapeHtml = (text: string) =>
  text.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;')

interface TooltipParam {
  axisValue?: string
  marker?: string
  seriesName?: string
  value?: unknown
}

/** The echarts option: bars of opens, a line of clicks, dashed markers. Pure. */
export function buildChartOption({
  opens,
  clicks,
  granularity,
  markers,
  labels,
  locale,
  formatCount,
  periodHours
}: {
  opens: Bucket[]
  clicks: Bucket[]
  granularity: EngagementGranularity
  markers: ChartMarker[]
  labels: { opens: string; clicks: string }
  locale: string
  formatCount: (n: number) => string
  /** The hours the selected period spans (48, 168, 720), whatever is filled so far. */
  periodHours: number
}) {
  const keys = opens.map((b) => b.key)
  const shown = markers.filter((m) => keys.includes(m.key))
  // Two markers each get their own line above the plot, so their labels never overlap. A marker
  // near an edge is aligned inwards, so its label stays inside the chart.
  const markLines = shown.map((m, index) => {
    const position = keys.indexOf(m.key) / Math.max(keys.length - 1, 1)
    const align = position < 0.25 ? 'left' : position > 0.75 ? 'right' : 'center'
    return {
      name: m.label,
      xAxis: m.key,
      label: { align, offset: [0, -14 * (shown.length - 1 - index)] }
    }
  })
  const markerRows = shown.length
  return {
    animation: false,
    textStyle: { fontFamily: typography.sans },
    grid: { left: 40, right: 16, top: 32 + 14 * markerRows, bottom: 28 },
    legend: {
      top: 0,
      right: 0,
      itemWidth: 10,
      itemHeight: 10,
      icon: 'roundRect',
      textStyle: { color: colors.ink2 },
      data: [labels.opens, labels.clicks]
    },
    tooltip: {
      trigger: 'axis',
      confine: true,
      formatter: (params: TooltipParam | TooltipParam[]) => {
        const list = Array.isArray(params) ? params : [params]
        const key = list[0]?.axisValue
        const heading = key ? tooltipHeading(key, granularity, locale) : ''
        // A bucket still to come has no value: a dash, not a 0.
        const count = (value: unknown) =>
          typeof value === 'number' && Number.isFinite(value) ? formatCount(value) : '—'
        const lines = list.map((p) => `${p.marker ?? ''}${escapeHtml(p.seriesName ?? '')} <b>${count(p.value)}</b>`)
        return [heading, ...lines].join('<br/>')
      }
    },
    xAxis: {
      type: 'category',
      data: keys,
      axisLabel: {
        formatter: (key: string) => axisLabel(key, granularity, locale),
        interval: labelledIndex(keys, granularity, periodHours),
        color: colors.ink3,
        hideOverlap: true
      },
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
          label: { formatter: '{b}', color: colors.ink2, position: 'end' },
          data: markLines
        }
      },
      {
        name: labels.clicks,
        type: 'line',
        data: clicks.map((b) => b.value),
        // Smooth, but never below zero between two empty hours.
        smooth: true,
        smoothMonotone: 'x',
        symbol: 'none',
        lineStyle: { color: colors.chart.series2, width: 2 },
        itemStyle: { color: colors.chart.series2 }
      }
    ]
  }
}
