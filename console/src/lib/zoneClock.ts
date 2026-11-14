import type { Dayjs } from 'dayjs'

/**
 * A date on the clock of a timezone, with the zone it was read in. A zone the browser's ICU does not
 * know makes `tz()` throw (the server validates zones, so it is rare): the date is then read in UTC,
 * and says so.
 */
export function onZoneClock(date: Dayjs, zone: string): { when: Dayjs; zone: string } {
  try {
    return { when: date.tz(zone), zone }
  } catch {
    return { when: date.tz('UTC'), zone: 'UTC' }
  }
}
