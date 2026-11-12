import { afterAll, afterEach, describe, expect, it } from 'vitest'
import { i18n } from '@lingui/core'
import dayjs from '../lib/dayjs'
import {
  formatDayRange,
  formatHour,
  formatHourOfDay,
  formatMonthDay,
  formatMonthYear
} from './dateFormats'

// West of UTC, so a label formatted on the browser's clock rather than the dayjs object's lands on
// the day before and fails, even on a runner that is itself on UTC. Restored after the file: a
// worker can go on to run other files. The zone is read when Date and Intl run, after the imports
// above.
const runnerTimeZone = process.env.TZ
process.env.TZ = 'America/Los_Angeles'
afterAll(() => {
  if (runnerTimeZone === undefined) delete process.env.TZ
  else process.env.TZ = runnerTimeZone
})

// The helpers read the active Lingui locale. Switch it for one case, then put English back.
function inLocale(locale: string): void {
  i18n.load(locale, {})
  i18n.activate(locale)
}

afterEach(() => {
  inLocale('en')
})

const day = (iso: string) => dayjs.utc(iso)

describe('formatMonthDay', () => {
  it('orders day and month as each language does', () => {
    expect(formatMonthDay(day('2026-02-03T00:00:00Z'))).toBe('Feb 3')
    inLocale('fr')
    expect(formatMonthDay(day('2026-02-03T00:00:00Z'))).toBe('3 févr.')
    inLocale('ja')
    expect(formatMonthDay(day('2026-02-03T00:00:00Z'))).toBe('2月3日')
  })

  it('shows the wall clock of the dayjs object, not the browser zone', () => {
    // 23:30 UTC on the 9th is 08:30 on the 10th in Tokyo.
    expect(formatMonthDay(day('2026-08-09T23:30:00Z'))).toBe('Aug 9')
    expect(formatMonthDay(dayjs('2026-08-09T23:30:00Z').tz('Asia/Tokyo'))).toBe('Aug 10')
    expect(formatMonthDay(dayjs('2026-08-10T02:00:00Z').tz('America/Los_Angeles'))).toBe('Aug 9')
  })
})

describe('formatHour', () => {
  it('writes the hour as each language does', () => {
    expect(formatHour(day('2026-08-09T09:00:00Z'))).toBe('9:00 AM')
    inLocale('fr')
    expect(formatHour(day('2026-08-09T09:00:00Z'))).toBe('09:00')
    inLocale('ja')
    expect(formatHour(day('2026-08-09T09:00:00Z'))).toBe('9:00')
  })

  it('keeps the hour of a converted dayjs object', () => {
    expect(formatHour(dayjs('2026-08-09T09:00:00Z').tz('Asia/Tokyo'))).toBe('6:00 PM')
  })
})

describe('formatHourOfDay', () => {
  it('names each hour of the day as each language does', () => {
    expect(formatHourOfDay(0)).toBe('12:00 AM')
    expect(formatHourOfDay(15)).toBe('3:00 PM')
    inLocale('fr')
    expect(formatHourOfDay(0)).toBe('0:00')
    expect(formatHourOfDay(15)).toBe('15:00')
  })
})

describe('formatMonthYear', () => {
  it('writes month and year as each language does', () => {
    expect(formatMonthYear(day('2026-08-01T00:00:00Z'))).toBe('Aug 2026')
    inLocale('fr')
    expect(formatMonthYear(day('2026-08-01T00:00:00Z'))).toBe('août 2026')
    inLocale('ja')
    expect(formatMonthYear(day('2026-08-01T00:00:00Z'))).toBe('2026年8月')
  })
})

describe('formatDayRange', () => {
  it('gives one day when both bounds fall on it', () => {
    expect(formatDayRange(day('2026-02-03T00:00:00Z'), day('2026-02-03T23:59:59Z'))).toBe('Feb 3')
  })

  it('puts reversed bounds in order', () => {
    expect(formatDayRange(day('2026-02-09T00:00:00Z'), day('2026-02-03T00:00:00Z'))).toBe('Feb 3 – 9')
  })

  it('drops the parts both bounds share', () => {
    expect(formatDayRange(day('2026-02-03T00:00:00Z'), day('2026-02-09T23:59:59Z'))).toBe('Feb 3 – 9')
    expect(formatDayRange(day('2026-01-28T00:00:00Z'), day('2026-02-03T23:59:59Z'))).toBe(
      'Jan 28 – Feb 3'
    )
    inLocale('fr')
    expect(formatDayRange(day('2026-02-03T00:00:00Z'), day('2026-02-09T23:59:59Z'))).toBe('3–9 févr.')
    expect(formatDayRange(day('2026-01-28T00:00:00Z'), day('2026-02-03T23:59:59Z'))).toBe(
      '28 janv. – 3 févr.'
    )
  })

  it('adds the year when the bounds are in different years', () => {
    expect(formatDayRange(day('2025-12-28T00:00:00Z'), day('2026-01-03T23:59:59Z'))).toBe(
      'Dec 28, 2025 – Jan 3, 2026'
    )
  })

  it('writes Japanese ranges with numbers, as Intl does', () => {
    inLocale('ja')
    expect(formatDayRange(day('2026-02-03T00:00:00Z'), day('2026-02-03T23:59:59Z'))).toBe('2月3日')
    expect(formatDayRange(day('2026-02-03T00:00:00Z'), day('2026-02-09T23:59:59Z'))).toBe('02/03～02/09')
  })

  it('reads each bound on its own wall clock', () => {
    // Bounds of a Tokyo week: their UTC instants fall on the day before.
    const start = dayjs.tz('2026-02-03 00:00', 'Asia/Tokyo')
    const end = dayjs.tz('2026-02-09 23:59', 'Asia/Tokyo')
    expect(formatDayRange(start, end)).toBe('Feb 3 – 9')
  })
})
