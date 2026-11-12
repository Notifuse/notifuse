/**
 * Dates of the dashboard cards, with Intl in the console's language: no dayjs locale is loaded in
 * the console, so dayjs would print English month names everywhere.
 */

function dayKey(at: Date, timeZone: string): string {
  return new Intl.DateTimeFormat('en-CA', {
    timeZone,
    year: 'numeric',
    month: '2-digit',
    day: '2-digit'
  }).format(at)
}

/** "14:29" for an event of today, "9 Nov" otherwise; days of the workspace timezone. */
export function formatEventTime(iso: string, timeZone: string, locale: string, now: Date = new Date()): string {
  const at = new Date(iso)
  const zone = timeZone || 'UTC'
  if (dayKey(at, zone) === dayKey(now, zone)) {
    return new Intl.DateTimeFormat(locale, {
      timeZone: zone,
      hour: '2-digit',
      minute: '2-digit',
      hourCycle: 'h23'
    }).format(at)
  }
  return new Intl.DateTimeFormat(locale, {
    timeZone: zone,
    day: 'numeric',
    month: 'short'
  }).format(at)
}

/** "27 Nov, 08:30" in that zone. */
export function formatDayAndTime(at: Date, timeZone: string, locale: string): string {
  return new Intl.DateTimeFormat(locale, {
    timeZone: timeZone || 'UTC',
    day: 'numeric',
    month: 'short',
    hour: '2-digit',
    minute: '2-digit',
    hourCycle: 'h23'
  }).format(at)
}

/** "4 min", "3 h", "2 days": how long ago, without "ago", as the new contacts card shows it. */
export function formatAge(iso: string, now: Date, locale: string): string {
  const minutes = Math.max(1, Math.floor((now.getTime() - new Date(iso).getTime()) / 60_000))
  const [value, unit] =
    minutes < 60
      ? [minutes, 'minute']
      : minutes < 60 * 24
        ? [Math.floor(minutes / 60), 'hour']
        : [Math.floor(minutes / 1440), 'day']
  return new Intl.NumberFormat(locale, {
    style: 'unit',
    unit,
    unitDisplay: unit === 'day' ? 'long' : 'short'
  }).format(value)
}
