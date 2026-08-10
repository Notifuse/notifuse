import { describe, expect, it } from 'vitest'
import { weekdayLabel } from './dictionaries'

describe('weekdayLabel', () => {
  it('maps ISODOW numbering, where Monday is 1 and Sunday is 7', () => {
    // The day_of_week dimension is PostgreSQL EXTRACT(ISODOW). Reading it as
    // the 0-6 numbering used by JavaScript's getDay would name every day one
    // off, so the reference week has to start on a Monday.
    expect(weekdayLabel(1, 'en')).toBe('Monday')
    expect(weekdayLabel(7, 'en')).toBe('Sunday')
  })

  it('translates without going through the message catalogue', () => {
    expect(weekdayLabel(1, 'fr')).toBe('lundi')
    expect(weekdayLabel(7, 'ja')).toBe('日曜日')
    expect(weekdayLabel(1, 'pt-BR')).toBe('segunda-feira')
  })

  it('returns the short form on request', () => {
    expect(weekdayLabel(1, 'en', 'short')).toBe('Mon')
    expect(weekdayLabel(7, 'en', 'short')).toBe('Sun')
  })

  it('rejects anything outside ISODOW rather than naming the wrong day', () => {
    // 0 is the trap: it is a valid day in JavaScript's numbering and a
    // perfectly ordinary array index, but it is not a day the dimension can
    // produce. Returning undefined lets the caller show the raw value.
    expect(weekdayLabel(0, 'en')).toBeUndefined()
    expect(weekdayLabel(8, 'en')).toBeUndefined()
    expect(weekdayLabel(-1, 'en')).toBeUndefined()
    expect(weekdayLabel(1.5, 'en')).toBeUndefined()
    expect(weekdayLabel(NaN, 'en')).toBeUndefined()
  })

  it('falls back to English when the locale has not loaded yet', () => {
    // Intl throws a RangeError on an empty tag, and the locale is empty until
    // the first catalogue is activated.
    expect(() => weekdayLabel(1, '')).not.toThrow()
    expect(weekdayLabel(1, '')).toBe('Monday')
  })

  it('does not shift a day when the runtime sits west of UTC', () => {
    // The reference dates are built with Date.UTC, so they must be formatted
    // in UTC too. Formatting in a negative-offset zone would land on the
    // previous day and name every weekday one early.
    const previous = process.env.TZ
    try {
      process.env.TZ = 'America/Los_Angeles'
      expect(weekdayLabel(1, 'en')).toBe('Monday')
    } finally {
      process.env.TZ = previous
    }
  })
})
