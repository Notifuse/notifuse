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
 * PostgreSQL EXTRACT(ISODOW) numbering, which the day_of_week dimension is
 * built on: Monday is 1 and Sunday is 7.
 */
export const DAYS_OF_WEEK: Record<number, string> = {
  1: 'Monday',
  2: 'Tuesday',
  3: 'Wednesday',
  4: 'Thursday',
  5: 'Friday',
  6: 'Saturday',
  7: 'Sunday'
}

export const DAY_OF_WEEK_SHORT: Record<number, string> = {
  1: 'Mon',
  2: 'Tue',
  3: 'Wed',
  4: 'Thu',
  5: 'Fri',
  6: 'Sat',
  7: 'Sun'
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
