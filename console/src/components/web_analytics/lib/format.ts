import { i18n } from '@lingui/core'
import { msg } from '@lingui/core/macro'
import dayjs from '../../../lib/dayjs'
import { DayjsRange } from './dates'
import { Granularity, ValueFormat } from './types'
import { formatDayRange, formatHour, formatMonthDay, formatMonthYear } from '../../../i18n/dateFormats'

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

// Every label here is in the console's language, not the browser's: the active Lingui locale.
const locale = (): string => i18n.locale || 'en'

/**
 * 999 → "999", 1234 → "1.2K", 1234567 → "1.2M" in English; "1,2 k" and "1,2 M" in French. Each
 * language abbreviates its own way (Japanese "1.2万" from ten thousand, German not below a million).
 */
export function formatNumber(value: number): string {
  if (!Number.isFinite(value)) return '0'
  // Rounded before the cutoff, so 999.6 reads "1K" like 1000 does, not "1,000". Halves go away
  // from zero, as Intl rounds them, so -999.5 reads "-1K" like 999.5 reads "1K".
  const rounded = Math.sign(value) * Math.round(Math.abs(value))
  if (Math.abs(rounded) < 1_000) {
    return new Intl.NumberFormat(locale(), { maximumFractionDigits: 0 }).format(value)
  }
  // Grouping forced: compact notation leaves German and Japanese four-digit counts ungrouped
  // ("1234") where the rest of the console writes "1.234".
  return new Intl.NumberFormat(locale(), {
    notation: 'compact',
    maximumFractionDigits: 1,
    useGrouping: true
  }).format(Math.abs(value) < 1_000 ? rounded : value)
}

/** A percentage given in percent: 12.5 → "12.5%" in English, "12,5 %" in French. */
export function formatPercent(value: number, decimals = 1, minimumDecimals = decimals): string {
  return new Intl.NumberFormat(locale(), {
    style: 'percent',
    minimumFractionDigits: minimumDecimals,
    maximumFractionDigits: decimals
  }).format(value / 100)
}

/**
 * Seconds in the words of the console's language: "45 s", "5 min 12 s", "5 min", "1 h 05 min",
 * "2 h". The same messages as a contact's visits (contacts/visits/duration.ts), plus the two
 * round forms.
 */
export function formatDuration(value: number): string {
  const total = Number.isFinite(value) && value > 0 ? Math.round(value) : 0
  if (total < 60) {
    const seconds = total
    return i18n._(msg`${seconds} s`)
  }
  if (total < 3600) {
    const minutes = Math.floor(total / 60)
    const seconds = total % 60
    return seconds > 0 ? i18n._(msg`${minutes} min ${seconds} s`) : i18n._(msg`${minutes} min`)
  }
  // Rounded to the minute from the value itself, not from its rounded seconds.
  const roundedMinutes = Math.round(value / 60)
  const hours = Math.floor(roundedMinutes / 60)
  const rest = roundedMinutes % 60
  if (rest === 0) return i18n._(msg`${hours} h`)
  const mm = String(rest).padStart(2, '0')
  return i18n._(msg`${hours} h ${mm} min`)
}

/**
 * Money, when the workspace has a currency configured. Without one the value
 * is still a quantity worth reading exactly, so it falls back to a plain
 * decimal rather than the abbreviated form used for counts.
 */
export function formatCurrency(value: number, currency?: string): string {
  // The console's language, not the browser's: "1 235" in French, "1,235" in English.
  const language = locale()
  if (!currency) {
    return value.toLocaleString(language, { maximumFractionDigits: value >= 1000 ? 0 : 2 })
  }
  const format = (amount: number) =>
    new Intl.NumberFormat(language, {
      style: 'currency',
      currency,
      currencyDisplay: 'narrowSymbol',
      minimumFractionDigits: 0,
      maximumFractionDigits: amount >= 1000 ? 0 : 2
    }).format(amount)

  const absolute = Math.abs(value)
  // The suffix goes after the whole number, group separators included: French groups with a
  // narrow no-break space, so "2 346 €" becomes "2 346M €", not "2M 346 €".
  const amount = /\d(?:[\d.,\u202f\u00a0]*\d)?/
  if (absolute >= 1_000_000) return format(value / 1_000_000).replace(amount, (n) => `${n}M`)
  if (absolute >= 10_000) return format(value / 1_000).replace(amount, (n) => `${n}K`)
  return format(value)
}

export function formatValue(value: number, format: ValueFormat, currency?: string): string {
  switch (format) {
    case 'duration':
      return formatDuration(value)
    case 'percentage':
      return formatPercent(value)
    case 'currency':
      return formatCurrency(value, currency)
    default:
      return formatNumber(value)
  }
}

/** Shorter than formatValue: chart axes have no room for "5 min 12 s". */
export function formatAxisValue(value: number, format: ValueFormat): string {
  switch (format) {
    case 'duration': {
      if (Math.round(value) < 60) {
        const seconds = Math.round(value)
        return i18n._(msg`${seconds} s`)
      }
      const minutes = new Intl.NumberFormat(locale(), { maximumFractionDigits: 1 }).format(value / 60)
      return i18n._(msg`${minutes} min`)
    }
    case 'percentage':
      return formatPercent(value, 0)
    default:
      return formatNumber(value)
  }
}

/** Percentage change between two totals; 0 when there is nothing to compare. */
export function changePercent(current: number, previous: number): number {
  if (!previous) return 0
  return ((current - previous) / previous) * 100
}

/** Chart series name for a range, e.g. "Dec 21 – 27" or "Nov 28 – Dec 4", in the console language. */
export function formatRangeSeriesName(range: DayjsRange): string {
  return formatDayRange(range.start, range.end)
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
      return formatHour(parsed)
    case 'month':
      return formatMonthYear(parsed)
    case 'year':
      return parsed.format('YYYY')
    default:
      return formatMonthDay(parsed)
  }
}
