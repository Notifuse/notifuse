/**
 * Valid IANA Timezone Identifiers
 * 
 * This list is dynamically loaded from the backend via /config.js
 * The backend serves window.TIMEZONES with all valid timezone identifiers
 * from Go's embedded IANA timezone database.
 * 
 * Source: Backend's internal/domain/timezones.go
 * Endpoint: /config.js (window.TIMEZONES)
 * 
 * This ensures perfect synchronization between frontend and backend
 * without needing to maintain a separate static file.
 */

// Declare global window.TIMEZONES type
declare global {
  interface Window {
    TIMEZONES?: string[]
  }
}

/**
 * Array of all valid IANA timezone identifiers accepted by the backend
 * Loaded from window.TIMEZONES which is served by /config.js
 */
export const VALID_TIMEZONES: readonly string[] = window.TIMEZONES || []

/**
 * Type representing any valid timezone identifier
 */
export type TimezoneIdentifier = string

/**
 * Form options for Ant Design Select component
 */
export const TIMEZONE_OPTIONS = VALID_TIMEZONES.map(tz => ({
  value: tz,
  label: tz,
}))

/**
 * Checks if a timezone string is valid according to the backend
 * 
 * @param timezone - The timezone identifier to validate
 * @returns true if the timezone is valid
 */
export function isValidTimezone(timezone: string): timezone is TimezoneIdentifier {
  return VALID_TIMEZONES.includes(timezone)
}

/**
 * Total number of valid timezones
 */
export const TIMEZONE_COUNT = VALID_TIMEZONES.length

/**
 * Checks if timezones have been loaded from the backend
 * 
 * @returns true if timezones are available
 */
export function areTimezonesLoaded(): boolean {
  return TIMEZONE_COUNT > 0
}
