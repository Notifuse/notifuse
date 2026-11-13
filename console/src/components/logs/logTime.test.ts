import { describe, expect, it } from 'vitest'
import { i18n } from '@lingui/core'
import { formatLogTime } from './logTime'

const PARIS = 'Europe/Paris'
// 2026-11-12 14:34:02 in Paris (UTC+1).
const NOW = new Date('2026-11-12T13:34:02Z')

describe('formatLogTime', () => {
  it('shows the time alone today', () => {
    expect(formatLogTime(i18n, '2026-11-12T08:05:09Z', PARIS, NOW)).toBe('09:05:09')
  })

  it('says yesterday for the day before', () => {
    expect(formatLogTime(i18n, '2026-11-11T17:40:00Z', PARIS, NOW)).toBe('Yesterday 18:40')
  })

  it('shows the day and month before that', () => {
    expect(formatLogTime(i18n, '2026-11-02T17:40:00Z', PARIS, NOW)).toBe('2 Nov, 18:40')
  })

  it('reads the day in the workspace timezone, not in UTC', () => {
    // 23:30 UTC on the 11th is 00:30 on the 12th in Paris: today there.
    expect(formatLogTime(i18n, '2026-11-11T23:30:00Z', PARIS, NOW)).toBe('00:30:00')
    // And the same instant is yesterday in New York (18:30 on the 11th, now 08:34 on the 12th).
    expect(formatLogTime(i18n, '2026-11-11T23:30:00Z', 'America/New_York', NOW)).toBe(
      'Yesterday 18:30'
    )
  })
})
