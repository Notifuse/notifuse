// Value vocabularies for the filter builder's dropdowns. These mirror what the
// SDK actually sends (web_analytics_sdk/src/detection/device.ts), so picking a
// value from a list can never produce a filter that matches nothing.

export const DEVICE_TYPES = ['desktop', 'mobile', 'tablet'] as const

/** ua-parser-js browser "type"; an ordinary browser reports none of these. */
export const BROWSER_TYPES = ['inapp', 'crawler', 'cli', 'email', 'fetcher'] as const

/** Normalized by the SDK, which is why "Mac OS" appears here as "macOS". */
export const OS_TYPES = [
  'Windows',
  'macOS',
  'iOS',
  'iPadOS',
  'Android',
  'Linux',
  'Chrome OS',
  'Unknown'
] as const

/** The browser names ua-parser-js reports most often. Free text is allowed. */
export const BROWSERS = [
  'Chrome',
  'Chrome WebView',
  'Mobile Chrome',
  'Safari',
  'Mobile Safari',
  'Firefox',
  'Mobile Firefox',
  'Edge',
  'Opera',
  'Opera Mobi',
  'Samsung Internet',
  'Brave',
  'Vivaldi',
  'DuckDuckGo',
  'Yandex',
  'UCBrowser',
  'Instagram',
  'Facebook',
  'WebKit',
  'Unknown'
] as const

/**
 * Monday of the reference week. The day_of_week dimension is PostgreSQL
 * EXTRACT(ISODOW), where Monday is 1 and Sunday is 7, so day N is simply the
 * Nth of January 2024 — which happens to start on a Monday.
 */
const ISO_WEEK_START = { year: 2024, monthIndex: 0 }

// Constructing an Intl formatter is not cheap and a table can ask for a label
// per row, so they are built once per locale and width. Bounded by the eight
// supported locales.
const weekdayFormatters = new Map<string, Intl.DateTimeFormat>()

/**
 * Weekday name for an ISODOW number, in the console's active locale.
 *
 * These come from Intl rather than the message catalogue on purpose: weekday
 * names are locale data every runtime already carries, and routing them
 * through translations would ask eight catalogues to restate what the platform
 * knows — and get it wrong in the meantime.
 *
 * Returns undefined for anything outside 1-7, leaving the caller to decide how
 * to render a value the dimension should never have produced.
 */
export function weekdayLabel(
  isoDow: number,
  locale: string,
  width: 'long' | 'short' = 'long'
): string | undefined {
  if (!Number.isInteger(isoDow) || isoDow < 1 || isoDow > 7) return undefined

  // Intl rejects an empty tag, which is what the locale is before the first
  // catalogue finishes loading.
  const tag = locale || 'en'
  const key = `${tag}:${width}`
  let formatter = weekdayFormatters.get(key)
  if (!formatter) {
    formatter = new Intl.DateTimeFormat(tag, { weekday: width, timeZone: 'UTC' })
    weekdayFormatters.set(key, formatter)
  }
  return formatter.format(
    new Date(Date.UTC(ISO_WEEK_START.year, ISO_WEEK_START.monthIndex, isoDow))
  )
}

/** "12a", "1a", … "11p" — the hour labels of the traffic heat map. */
export const HOUR_LABELS = Array.from({ length: 24 }, (_, hour) => {
  if (hour === 0) return '12a'
  if (hour < 12) return `${hour}a`
  if (hour === 12) return '12p'
  return `${hour - 12}p`
})

/** Values a boolean dimension can take; the engine renders them as text. */
export const BOOLEAN_VALUES = ['true', 'false'] as const

/** Dictionary-backed value pickers, by dimension. */
export const DIMENSION_VALUE_OPTIONS: Record<string, readonly string[]> = {
  device: DEVICE_TYPES,
  browser: BROWSERS,
  browser_type: BROWSER_TYPES,
  os: OS_TYPES,
  is_direct: BOOLEAN_VALUES,
  is_weekend: BOOLEAN_VALUES,
  is_landing_page: BOOLEAN_VALUES,
  is_exit_page: BOOLEAN_VALUES
}
