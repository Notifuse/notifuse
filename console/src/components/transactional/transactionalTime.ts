/**
 * Times of the transactional screens, with Intl in the console's language: no dayjs locale is
 * loaded in the console, so dayjs would print English everywhere.
 */

function dayKey(at: Date, timeZone: string): string {
  return new Intl.DateTimeFormat('en-CA', { timeZone, year: 'numeric', month: '2-digit', day: '2-digit' }).format(at)
}

function zone(timeZone: string): string {
  return timeZone || 'UTC'
}

/** "18:12" in that zone. */
export function formatTime(at: Date, timeZone: string, locale: string, seconds = false): string {
  return new Intl.DateTimeFormat(locale, {
    timeZone: zone(timeZone),
    hour: '2-digit',
    minute: '2-digit',
    ...(seconds ? { second: '2-digit' } : {}),
    hourCycle: 'h23'
  }).format(at)
}

/** "9 Nov, 18:12". */
export function formatShortDateTime(at: Date, timeZone: string, locale: string): string {
  return new Intl.DateTimeFormat(locale, {
    timeZone: zone(timeZone),
    day: 'numeric',
    month: 'short',
    hour: '2-digit',
    minute: '2-digit',
    hourCycle: 'h23'
  }).format(at)
}

/** "12 Nov 2026, 18:12:05": the full instant, for a tooltip. */
export function formatFullDateTime(iso: string | undefined, timeZone: string, locale: string): string {
  const at = new Date(iso ?? '')
  // A missing or malformed date reads as a dash rather than breaking the page.
  if (Number.isNaN(at.getTime())) return '—'
  return new Intl.DateTimeFormat(locale, { timeZone: zone(timeZone), dateStyle: 'medium', timeStyle: 'medium' }).format(
    at
  )
}

/** "9 Nov", the day of a series key (YYYY-MM-DD), without any timezone shift. */
export function formatSeriesDay(day: string, locale: string): string {
  return new Intl.DateTimeFormat(locale, { timeZone: 'UTC', day: 'numeric', month: 'short' }).format(
    new Date(`${day}T00:00:00Z`)
  )
}

export function isToday(iso: string, timeZone: string, now: Date): boolean {
  return dayKey(new Date(iso), zone(timeZone)) === dayKey(now, zone(timeZone))
}

export function todayKey(timeZone: string, now: Date): string {
  return dayKey(now, zone(timeZone))
}

/**
 * When a notification last sent: "3 min ago" or "2 hr ago" under a day, then "Yesterday, 18:12"
 * (the label comes from Lingui), then "9 Nov, 18:12".
 */
export function formatLastSent(
  iso: string,
  now: Date,
  timeZone: string,
  locale: string,
  yesterday: (time: string) => string
): string {
  const at = new Date(iso)
  if (Number.isNaN(at.getTime())) return '—'
  const minutes = Math.max(0, Math.floor((now.getTime() - at.getTime()) / 60_000))
  const relative = new Intl.RelativeTimeFormat(locale, { numeric: 'auto', style: 'short' })
  // « now » under a minute, rather than « this minute ».
  if (minutes === 0) return relative.format(0, 'second')
  if (minutes < 60) return relative.format(-minutes, 'minute')
  if (minutes < 24 * 60) return relative.format(-Math.floor(minutes / 60), 'hour')
  const dayBefore = new Date(now.getTime() - 86_400_000)
  if (dayKey(at, zone(timeZone)) === dayKey(dayBefore, zone(timeZone))) {
    return yesterday(formatTime(at, timeZone, locale))
  }
  return formatShortDateTime(at, timeZone, locale)
}

/** "14:29:03" for a send of today, "9 Nov, 14:29" otherwise. */
export function formatSendTime(iso: string, now: Date, timeZone: string, locale: string): string {
  const at = new Date(iso)
  if (Number.isNaN(at.getTime())) return '—'
  return isToday(iso, timeZone, now)
    ? formatTime(at, timeZone, locale, true)
    : formatShortDateTime(at, timeZone, locale)
}
