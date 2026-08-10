import dayjs from '../../../lib/dayjs'
import { DayjsRange } from './dates'
import { Granularity, ValueFormat } from './types'

/**
 * Measures come back from the analytics engine as numbers or, for anything
 * built on PostgreSQL numeric, as strings. Everything that reads a measure
 * goes through here.
 */
export function toNumber(value: unknown): number {
  if (typeof value === 'number') return Number.isFinite(value) ? value : 0
  if (typeof value === 'string') {
    const parsed = parseFloat(value)
    return Number.isFinite(parsed) ? parsed : 0
  }
  return 0
}

/** 1234 → "1.2K", 1234567 → "1.2M". */
export function formatNumber(value: number): string {
  if (!Number.isFinite(value)) return '0'
  const absolute = Math.abs(value)
  if (absolute >= 1_000_000) return `${(value / 1_000_000).toFixed(1)}M`
  if (absolute >= 1_000) return `${(value / 1_000).toFixed(1)}K`
  return value.toFixed(0)
}

/** Seconds → "45s", "5m 12s", "2h 30m". */
export function formatDuration(seconds: number): string {
  if (!Number.isFinite(seconds) || seconds <= 0) return '0s'
  if (seconds < 60) return `${Math.round(seconds)}s`
  if (seconds < 3600) {
    const minutes = Math.floor(seconds / 60)
    const remainder = Math.round(seconds % 60)
    return remainder > 0 ? `${minutes}m ${remainder}s` : `${minutes}m`
  }
  const hours = Math.floor(seconds / 3600)
  const minutes = Math.round((seconds % 3600) / 60)
  return minutes > 0 ? `${hours}h ${minutes}m` : `${hours}h`
}

/**
 * Money, when the workspace has a currency configured. Without one the value
 * is still a quantity worth reading exactly, so it falls back to a plain
 * decimal rather than the abbreviated form used for counts.
 */
export function formatCurrency(value: number, currency?: string): string {
  if (!currency) {
    return value.toLocaleString(undefined, { maximumFractionDigits: value >= 1000 ? 0 : 2 })
  }
  const format = (amount: number) =>
    new Intl.NumberFormat(undefined, {
      style: 'currency',
      currency,
      currencyDisplay: 'narrowSymbol',
      minimumFractionDigits: 0,
      maximumFractionDigits: amount >= 1000 ? 0 : 2
    }).format(amount)

  const absolute = Math.abs(value)
  if (absolute >= 1_000_000) return format(value / 1_000_000).replace(/[\d.,]+/, (n) => `${n}M`)
  if (absolute >= 10_000) return format(value / 1_000).replace(/[\d.,]+/, (n) => `${n}K`)
  return format(value)
}

export function formatValue(value: number, format: ValueFormat, currency?: string): string {
  switch (format) {
    case 'duration':
      return formatDuration(value)
    case 'percentage':
      return `${value.toFixed(1)}%`
    case 'currency':
      return formatCurrency(value, currency)
    default:
      return formatNumber(value)
  }
}

/** Shorter than formatValue: chart axes have no room for "5m 12s". */
export function formatAxisValue(value: number, format: ValueFormat): string {
  switch (format) {
    case 'duration':
      return value < 60 ? `${Math.round(value)}s` : `${(value / 60).toFixed(1)}m`
    case 'percentage':
      return `${Math.round(value)}%`
    default:
      return formatNumber(value)
  }
}

/** Percentage change between two totals; 0 when there is nothing to compare. */
export function changePercent(current: number, previous: number): number {
  if (!previous) return 0
  return ((current - previous) / previous) * 100
}

/** Chart series name for a range, e.g. "Dec 21-27" or "Nov 28 - Dec 4". */
export function formatRangeSeriesName(range: DayjsRange): string {
  const { start, end } = range
  if (start.isSame(end, 'month')) return `${start.format('MMM D')}-${end.format('D')}`
  return `${start.format('MMM D')} - ${end.format('MMM D')}`
}

/**
 * X axis tick for a bucket timestamp at the given granularity.
 *
 * Buckets are read as UTC on purpose. The engine truncates in the query's
 * timezone and hands back the resulting wall clock with a "Z" it did not earn,
 * so converting it again — which is what parsing it as an instant does —
 * shifts every label by the gap between the workspace's timezone and the
 * viewer's, labelling a whole day of traffic as the day before.
 */
export function formatXAxisLabel(timestamp: string, granularity: Granularity): string {
  const parsed = dayjs.utc(timestamp)
  if (!parsed.isValid()) return timestamp
  switch (granularity) {
    case 'hour':
      return parsed.format('ha')
    case 'month':
      return parsed.format("MMM 'YY")
    case 'year':
      return parsed.format('YYYY')
    default:
      return parsed.format('MMM D')
  }
}
