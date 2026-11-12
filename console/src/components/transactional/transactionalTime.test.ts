import { describe, expect, it } from 'vitest'
import { formatLastSent, formatSendTime, formatSeriesDay, isToday } from './transactionalTime'

// 12 November 2026, 10:00 in Paris
const now = new Date('2026-11-12T09:00:00Z')
const yesterday = (time: string) => `Yesterday, ${time}`
const ago = (minutes: number) => new Date(now.getTime() - minutes * 60_000).toISOString()
const spaces = (text: string) => text.replace(/\s/g, ' ')

describe('formatLastSent', () => {
  it('counts minutes, then hours, under a day', () => {
    expect(formatLastSent(ago(3), now, 'Europe/Paris', 'en', yesterday)).toBe('3 min. ago')
    expect(formatLastSent(ago(0), now, 'Europe/Paris', 'en', yesterday)).toBe('this minute')
    expect(formatLastSent(ago(185), now, 'Europe/Paris', 'en', yesterday)).toBe('3 hr. ago')
  })

  it('says yesterday with the time in the workspace timezone', () => {
    // 11 November, 08:12 in Paris
    expect(formatLastSent('2026-11-11T07:12:00Z', now, 'Europe/Paris', 'en', yesterday)).toBe('Yesterday, 08:12')
  })

  it('gives the date and time further back', () => {
    expect(formatLastSent('2026-11-09T17:12:00Z', now, 'Europe/Paris', 'en', yesterday)).toBe('Nov 9, 18:12')
    expect(spaces(formatLastSent('2026-11-09T17:12:00Z', now, 'Europe/Paris', 'fr', yesterday))).toBe('9 nov., 18:12')
  })

  it('writes relative times in the console language', () => {
    expect(spaces(formatLastSent(ago(3), now, 'Europe/Paris', 'fr', yesterday))).toBe('il y a 3 min')
  })
})

describe('formatSendTime', () => {
  it('shows seconds for a send of today, the date otherwise', () => {
    expect(formatSendTime('2026-11-12T08:29:03Z', now, 'Europe/Paris', 'en')).toBe('09:29:03')
    expect(formatSendTime('2026-11-11T22:30:00Z', now, 'Europe/Paris', 'en')).toBe('Nov 11, 23:30')
    expect(formatSendTime('2026-11-11T22:30:00Z', now, 'America/New_York', 'en')).toBe('Nov 11, 17:30')
  })
})

describe('isToday', () => {
  it('reads the day in the workspace timezone', () => {
    expect(isToday('2026-11-11T23:30:00Z', 'Europe/Paris', now)).toBe(true)
    expect(isToday('2026-11-11T23:30:00Z', 'UTC', now)).toBe(false)
  })
})

describe('formatSeriesDay', () => {
  it('reads a series key without shifting it', () => {
    expect(formatSeriesDay('2026-11-09', 'en')).toBe('Nov 9')
  })
})
