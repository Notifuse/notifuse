/**
 * The Explore session floor as the URL carries it. An absent `minSessions` means the default, so the
 * param is only written for another value: 1 (no floor) has to stay in the URL, or it reads back as 10.
 *
 * A leaf module: the route's validateSearch imports it, and importing the context from the router
 * would pull the whole dashboard in.
 */
export const DEFAULT_MIN_SESSIONS = 10

/**
 * Reads the param back. TanStack Router parses `?minSessions=1` as the number 1 and `?minSessions="1"`
 * as the string "1", so both are accepted; anything that is not a whole number of at least 1 falls
 * back to the default.
 */
export function parseMinSessions(value: unknown): number | undefined {
  if (typeof value !== 'number' && typeof value !== 'string') return undefined
  if (typeof value === 'string' && value.trim() === '') return undefined
  const parsed = Number(value)
  return Number.isInteger(parsed) && parsed >= 1 ? parsed : undefined
}

/** The value to write to the URL: dropped when it is the default (or not a usable floor). */
export function minSessionsParam(value: number): number | undefined {
  const floor = Math.floor(value)
  if (!Number.isFinite(floor) || floor < 1 || floor === DEFAULT_MIN_SESSIONS) return undefined
  return floor
}
