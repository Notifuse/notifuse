import { describe, expect, it } from 'vitest'
import dayjs from './dayjs'
import { onZoneClock } from './zoneClock'

describe('onZoneClock', () => {
  it('reads the date on the clock of the zone', () => {
    const { when, zone } = onZoneClock(dayjs('2026-08-12T10:00:00Z'), 'Asia/Tokyo')
    expect(zone).toBe('Asia/Tokyo')
    expect(when.format('HH:mm')).toBe('19:00')
  })

  it('falls back to UTC for a zone the browser does not know', () => {
    const { when, zone } = onZoneClock(dayjs('2026-08-12T10:00:00Z'), 'Mars/Olympus_Mons')
    expect(zone).toBe('UTC')
    expect(when.format('HH:mm')).toBe('10:00')
  })
})
