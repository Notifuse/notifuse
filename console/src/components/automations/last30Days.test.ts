import { describe, it, expect, vi, afterEach } from 'vitest'
import { last30Days } from './last30Days'

afterEach(() => vi.useRealTimers())

describe('last30Days', () => {
  it('covers the 30 local days that end today, today included', () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-11-12T10:00:00Z'))
    const range = last30Days('Europe/Paris')
    expect(range.startDay).toBe('2026-10-14')
    expect(range.endDay).toBe('2026-11-12')
    // Paris is UTC+2 until 25 October, UTC+1 after.
    expect(range.startUtc).toBe('2026-10-13T22:00:00.000Z')
    expect(range.endUtc).toBe('2026-11-12T22:59:59.999Z')
  })

  it('takes the day of the workspace, not the browser', () => {
    vi.useFakeTimers()
    // 23:30 UTC on the 12th is already the 13th in Tokyo.
    vi.setSystemTime(new Date('2026-11-12T23:30:00Z'))
    expect(last30Days('Asia/Tokyo').endDay).toBe('2026-11-13')
    expect(last30Days('UTC').endDay).toBe('2026-11-12')
  })
})
