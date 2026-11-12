import { describe, it, expect, vi, afterEach } from 'vitest'
import { last30Days } from './last30Days'

afterEach(() => vi.useRealTimers())

describe('last30Days', () => {
  it('covers the 30 whole local days that end yesterday', () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-11-12T10:00:00Z'))
    const range = last30Days('Europe/Paris')
    expect(range.startDay).toBe('2026-10-13')
    expect(range.endDay).toBe('2026-11-11')
    // Paris is UTC+2 until 25 October, UTC+1 after.
    expect(range.startUtc).toBe('2026-10-12T22:00:00.000Z')
    expect(range.endUtc).toBe('2026-11-11T22:59:59.999Z')
  })
})
