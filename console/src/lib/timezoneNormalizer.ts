/**
 * Timezone Normalizer
 * 
 * Maps deprecated/legacy IANA timezone aliases to their canonical names.
 * 
 * The browser's Intl.DateTimeFormat API may return legacy timezone names
 * (e.g., "Asia/Calcutta") but our backend validates against the official
 * IANA timezone database which uses canonical names (e.g., "Asia/Kolkata").
 * 
 * This utility ensures compatibility by normalizing timezone names before
 * sending them to the backend.
 * 
 * NOTE: The backend now accepts both canonical zones AND aliases (594 total).
 * This normalizer is kept for browser compatibility and to provide canonical
 * names when preferred.
 * 
 * For the complete list of valid timezones, see: ./timezones.ts
 * 
 * Reference: https://en.wikipedia.org/wiki/List_of_tz_database_time_zones
 */

/**
 * Map of deprecated timezone aliases to their canonical IANA names
 */
const TIMEZONE_ALIASES: Record<string, string> = {
  // India - Calcutta was renamed to Kolkata in 2001
  'Asia/Calcutta': 'Asia/Kolkata',
  
  // Ukraine - Kiev updated to Kyiv (Ukrainian transliteration)
  'Europe/Kiev': 'Europe/Kyiv',
  
  // Argentina - Deprecated city-level zones
  'America/Buenos_Aires': 'America/Argentina/Buenos_Aires',
  'America/Catamarca': 'America/Argentina/Catamarca',
  'America/Cordoba': 'America/Argentina/Cordoba',
  'America/Jujuy': 'America/Argentina/Jujuy',
  'America/Mendoza': 'America/Argentina/Mendoza',
  
  // US - Legacy zone abbreviations
  'US/Alaska': 'America/Anchorage',
  'US/Aleutian': 'America/Adak',
  'US/Arizona': 'America/Phoenix',
  'US/Central': 'America/Chicago',
  'US/East-Indiana': 'America/Indiana/Indianapolis',
  'US/Eastern': 'America/New_York',
  'US/Hawaii': 'Pacific/Honolulu',
  'US/Indiana-Starke': 'America/Indiana/Knox',
  'US/Michigan': 'America/Detroit',
  'US/Mountain': 'America/Denver',
  'US/Pacific': 'America/Los_Angeles',
  'US/Samoa': 'Pacific/Pago_Pago',
  
  // Canada
  'Canada/Atlantic': 'America/Halifax',
  'Canada/Central': 'America/Winnipeg',
  'Canada/Eastern': 'America/Toronto',
  'Canada/Mountain': 'America/Edmonton',
  'Canada/Newfoundland': 'America/St_Johns',
  'Canada/Pacific': 'America/Vancouver',
  'Canada/Saskatchewan': 'America/Regina',
  'Canada/Yukon': 'America/Whitehorse',
  
  // Mexico
  'Mexico/BajaNorte': 'America/Tijuana',
  'Mexico/BajaSur': 'America/Mazatlan',
  'Mexico/General': 'America/Mexico_City',
  
  // Brazil
  'Brazil/Acre': 'America/Rio_Branco',
  'Brazil/DeNoronha': 'America/Noronha',
  'Brazil/East': 'America/Sao_Paulo',
  'Brazil/West': 'America/Manaus',
  
  // Chile
  'Chile/Continental': 'America/Santiago',
  'Chile/EasterIsland': 'Pacific/Easter',
  
  // Other deprecated zones
  'GB': 'Europe/London',
  'GB-Eire': 'Europe/London',
  'Eire': 'Europe/Dublin',
  'W-SU': 'Europe/Moscow',
  'NZ': 'Pacific/Auckland',
  'NZ-CHAT': 'Pacific/Chatham',
  'PRC': 'Asia/Shanghai',
  'ROC': 'Asia/Taipei',
  'ROK': 'Asia/Seoul',
  'Singapore': 'Asia/Singapore',
  'Turkey': 'Europe/Istanbul',
  'Japan': 'Asia/Tokyo',
  'Egypt': 'Africa/Cairo',
  'Libya': 'Africa/Tripoli',
  'Iceland': 'Atlantic/Reykjavik',
  'Poland': 'Europe/Warsaw',
  'Portugal': 'Europe/Lisbon',
  'Iran': 'Asia/Tehran',
  'Israel': 'Asia/Jerusalem',
  'Jamaica': 'America/Jamaica',
  'Hongkong': 'Asia/Hong_Kong',
  'Cuba': 'America/Havana',
  
  // UTC variations
  'Etc/GMT': 'UTC',
  'Etc/UTC': 'UTC',
  'Etc/Universal': 'UTC',
  'Etc/Zulu': 'UTC',
  'GMT': 'UTC',
  'GMT0': 'UTC',
  'GMT+0': 'UTC',
  'GMT-0': 'UTC',
  'Greenwich': 'UTC',
  'UCT': 'UTC',
  'Universal': 'UTC',
  'Zulu': 'UTC',
}

/**
 * Normalizes a timezone name from a potentially deprecated alias to its canonical IANA name
 * 
 * @param timezone - The timezone name to normalize (e.g., "Asia/Calcutta")
 * @returns The canonical IANA timezone name (e.g., "Asia/Kolkata")
 * 
 * @example
 * normalizeTimezone("Asia/Calcutta")  // Returns: "Asia/Kolkata"
 * normalizeTimezone("Asia/Kolkata")   // Returns: "Asia/Kolkata" (already canonical)
 * normalizeTimezone("US/Eastern")     // Returns: "America/New_York"
 */
export function normalizeTimezone(timezone: string): string {
  return TIMEZONE_ALIASES[timezone] || timezone
}

/**
 * Gets the browser's timezone and normalizes it to the canonical IANA name
 * 
 * @returns The normalized canonical timezone name
 * 
 * @example
 * // If browser returns "Asia/Calcutta"
 * getBrowserTimezone() // Returns: "Asia/Kolkata"
 */
export function getBrowserTimezone(): string {
  const browserTimezone = Intl.DateTimeFormat().resolvedOptions().timeZone
  return normalizeTimezone(browserTimezone)
}

/**
 * Checks if a timezone name is a deprecated alias
 * 
 * @param timezone - The timezone name to check
 * @returns true if the timezone is a deprecated alias
 * 
 * @example
 * isDeprecatedTimezone("Asia/Calcutta")  // Returns: true
 * isDeprecatedTimezone("Asia/Kolkata")   // Returns: false
 */
export function isDeprecatedTimezone(timezone: string): boolean {
  return timezone in TIMEZONE_ALIASES
}
