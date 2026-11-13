import { describe, expect, it } from 'vitest'
import { formatAge, formatEventTime, formatDayAndTime } from './dashboardFormat'

const now = new Date('2026-11-11T13:40:00Z') // 14:40 in Paris

describe('formatEventTime', () => {
  it('gives the time for today, in the workspace timezone', () => {
    expect(formatEventTime('2026-11-11T13:29:00Z', 'Europe/Paris', 'en-GB', now)).toBe('14:29')
  })

  it('gives the date for another day', () => {
    expect(formatEventTime('2026-11-09T08:00:00Z', 'Europe/Paris', 'en-GB', now)).toBe('9 Nov')
    expect(formatEventTime('2026-11-09T08:00:00Z', 'Europe/Paris', 'fr', now)).toBe('9 nov.')
  })

  it('compares days in the workspace timezone', () => {
    // 23:30 on the 10th in UTC is already the 11th in Paris
    expect(formatEventTime('2026-11-10T23:30:00Z', 'Europe/Paris', 'en-GB', now)).toBe('00:30')
  })
})

describe('formatAge', () => {
  it('says minutes, hours, then days', () => {
    expect(formatAge('2026-11-11T13:36:00Z', now, 'en')).toBe('4 min')
    expect(formatAge('2026-11-11T10:40:00Z', now, 'en')).toBe('3 hr')
    expect(formatAge('2026-11-11T10:40:00Z', now, 'fr').replace(/\s/g, ' ')).toBe('3 h')
    expect(formatAge('2026-11-09T13:40:00Z', now, 'en')).toBe('2 days')
  })

  it('never goes below one minute', () => {
    expect(formatAge('2026-11-11T13:39:50Z', now, 'en')).toBe('1 min')
    expect(formatAge('2026-11-11T13:45:00Z', now, 'en')).toBe('1 min')
  })
})

describe('formatDayAndTime', () => {
  it('gives day, month and time in the zone', () => {
    expect(formatDayAndTime(new Date('2026-11-27T07:30:00Z'), 'Europe/Paris', 'en-GB')).toBe('27 Nov, 08:30')
    expect(formatDayAndTime(new Date('2026-11-27T07:30:00Z'), 'Europe/Paris', 'fr')).toBe('27 nov., 08:30')
  })
})
