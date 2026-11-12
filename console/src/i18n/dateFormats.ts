import type { Dayjs } from 'dayjs'
import { i18n } from '@lingui/core'

/**
 * Short date labels in the console's language, for the places where a fixed dayjs pattern would
 * keep English word order ("mars 5"). Intl knows each language's order and abbreviations
 * ("Feb 3", "3 févr.", "2月3日") and drops what two bounds share in a range ("3–9 févr.").
 */

const formatters = new Map<string, Intl.DateTimeFormat>()

function formatter(options: Intl.DateTimeFormatOptions): Intl.DateTimeFormat {
  const locale = i18n.locale || 'en'
  const key = `${locale}|${JSON.stringify(options)}`
  let cached = formatters.get(key)
  if (!cached) {
    cached = new Intl.DateTimeFormat(locale, { ...options, timeZone: 'UTC' })
    formatters.set(key, cached)
  }
  return cached
}

// Intl formats an instant in a zone, while a dayjs object from .tz() or .utc() carries the clock it
// shows in its own fields. Rebuild that clock as a UTC instant and format it in UTC, so a label
// reads what the dayjs object reads, whatever zone the browser runs in.
function wallClock(at: Dayjs): Date {
  return new Date(Date.UTC(at.year(), at.month(), at.date(), at.hour(), at.minute(), at.second()))
}

// Intl separates some parts with narrow or thin no-break spaces ("9:00 AM", "Feb 3 – 9"). Plain
// spaces read the same in a label and keep the text searchable and comparable.
function plain(text: string): string {
  return text.replace(/[\u202f\u2009\u00a0]/g, ' ')
}

const MONTH_DAY: Intl.DateTimeFormatOptions = { month: 'short', day: 'numeric' }

/** "Feb 3", "3 févr.", "2月3日". */
export function formatMonthDay(at: Dayjs): string {
  return plain(formatter(MONTH_DAY).format(wallClock(at)))
}

/** "9:00 AM", "09:00", "9:00": the short time of each language, as dayjs's `LT` mostly writes it. */
export function formatHour(at: Dayjs): string {
  return plain(formatter({ timeStyle: 'short' }).format(wallClock(at)))
}

/**
 * The hour of the day alone, for the 24 columns of an hour grid: "12:00 AM", "0:00", "15:00". The
 * minutes stay, so a bare "15" never reads as a day of the month. Takes the hour (0 to 23) rather
 * than a date, since the grid has no date.
 */
export function formatHourOfDay(hour: number): string {
  return plain(
    formatter({ hour: 'numeric', minute: '2-digit' }).format(new Date(Date.UTC(2026, 0, 1, hour)))
  )
}

/** "Aug 2026", "août 2026", "2026年8月". The full year, so it never reads as a day of the month. */
export function formatMonthYear(at: Dayjs): string {
  return plain(formatter({ month: 'short', year: 'numeric' }).format(wallClock(at)))
}

/**
 * The days from start to end, each read on its own clock: "Feb 3", "Feb 3 – 9", "Jan 28 – Feb 3",
 * and with the year when the bounds are in different years, "Dec 28, 2025 – Jan 3, 2026". In
 * Japanese, Intl writes ranges with numbers: "02/03～02/09".
 *
 * The bounds are put in order first: some engines throw on a range whose start is after its end,
 * and a custom range read from the URL is not checked before it is labelled.
 */
export function formatDayRange(start: Dayjs, end: Dayjs): string {
  const [first, last] = start.isAfter(end) ? [end, start] : [start, end]
  const from = wallClock(first.startOf('day'))
  const to = wallClock(last.startOf('day'))
  const options = first.year() === last.year() ? MONTH_DAY : { ...MONTH_DAY, year: 'numeric' as const }
  if (from.getTime() === to.getTime()) return plain(formatter(options).format(from))
  return plain(formatter(options).formatRange(from, to))
}
