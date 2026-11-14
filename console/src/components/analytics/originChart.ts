import type { DashboardMetric } from './dashboardSearch'
import type { MessageOrigin, MessageTotals, SeriesPoint } from './dashboardKpisModel'

const METRIC_FIELD: Record<DashboardMetric, keyof MessageTotals> = {
  sent: 'sent',
  opens: 'opened',
  clicks: 'clicked'
}

/** A peak above this many times the median of the non-zero daily values (all origins pooled) calls for a log axis. */
const LOG_RATIO = 10

export interface OriginChartData {
  days: string[]
  values: Record<MessageOrigin, (number | null)[]>
  log: boolean
  empty: boolean
}

function median(values: number[]): number {
  const sorted = [...values].sort((a, b) => a - b)
  const middle = Math.floor(sorted.length / 2)
  return sorted.length % 2 === 0 ? (sorted[middle - 1] + sorted[middle]) / 2 : sorted[middle]
}

/**
 * The daily series of one metric, one per origin, on the days the server returned (the gap filler gives
 * every day of the range, so the series share them). A single newsletter in the middle of small
 * daily sends would flatten the lines to the floor: past a ratio of ten between the peak and the
 * median of non-zero days, the axis turns logarithmic, where a zero cannot be drawn and is
 * passed as null.
 */
export function buildOriginChart(
  series: Record<MessageOrigin, SeriesPoint[]>,
  metric: DashboardMetric
): OriginChartData {
  const field = METRIC_FIELD[metric]
  const days = [...new Set(Object.values(series).flatMap((points) => points.map((point) => point.day)))].sort()

  const raw = {} as Record<MessageOrigin, number[]>
  for (const origin of Object.keys(series) as MessageOrigin[]) {
    const byDay = new Map(series[origin].map((point) => [point.day, point[field]]))
    raw[origin] = days.map((day) => byDay.get(day) ?? 0)
  }

  const all = Object.values(raw).flat()
  const nonZero = all.filter((value) => value > 0)
  // Empty when no email left: a period with sends but no click still draws its zeros.
  const empty = Object.values(series).every((points) => points.every((point) => point.sent === 0))
  const log = nonZero.length > 0 && Math.max(...nonZero) > LOG_RATIO * median(nonZero)

  const values = {} as Record<MessageOrigin, (number | null)[]>
  for (const origin of Object.keys(raw) as MessageOrigin[]) {
    values[origin] = log ? raw[origin].map((value) => (value > 0 ? value : null)) : raw[origin]
  }
  return { days, values, log, empty }
}
