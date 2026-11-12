import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { resolveTransactionalPeriod } from './period'

const dayCount = (startDay: string, endDay: string) =>
  Math.round((Date.parse(`${endDay}T00:00:00Z`) - Date.parse(`${startDay}T00:00:00Z`)) / 86_400_000) + 1

describe('resolveTransactionalPeriod', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    // 12 November 2026, 00:30 in Paris (UTC+1), still 11 November 18:30 in New York (UTC−5)
    vi.setSystemTime(new Date('2026-11-11T23:30:00Z'))
  })
  afterEach(() => vi.useRealTimers())

  it.each([
    ['7d', 7, '2026-11-06'],
    ['30d', 30, '2026-10-14'],
    ['60d', 60, '2026-09-14']
  ] as const)('covers %s up to today in Paris', (period, days, startDay) => {
    const { current } = resolveTransactionalPeriod(period, 'Europe/Paris')
    expect(current.startDay).toBe(startDay)
    expect(current.endDay).toBe('2026-11-12')
    expect(dayCount(current.startDay, current.endDay)).toBe(days)
    expect(current.endUtc).toBe('2026-11-12T22:59:59.999Z')
  })

  it('takes today in the workspace timezone, behind UTC', () => {
    const { current } = resolveTransactionalPeriod('7d', 'America/New_York')
    expect(current.startDay).toBe('2026-11-05')
    expect(current.endDay).toBe('2026-11-11')
    expect(current.startUtc).toBe('2026-11-05T05:00:00.000Z')
    expect(current.endUtc).toBe('2026-11-12T04:59:59.999Z')
  })

  it('compares with as many days just before, without overlap', () => {
    const { current, previous } = resolveTransactionalPeriod('7d', 'Europe/Paris')
    expect(previous.startDay).toBe('2026-10-30')
    expect(previous.endDay).toBe('2026-11-05')
    expect(Date.parse(previous.endUtc)).toBeLessThan(Date.parse(current.startUtc))
    expect(Date.parse(current.startUtc) - Date.parse(previous.endUtc)).toBe(1)
  })

  it('keeps every day across the end of summer time', () => {
    // Paris leaves summer time on 25 October 2026.
    const { current, previous } = resolveTransactionalPeriod('30d', 'Europe/Paris')
    expect(dayCount(previous.startDay, previous.endDay)).toBe(30)
    expect(current.startUtc).toBe('2026-10-13T22:00:00.000Z')
    expect(previous.startUtc).toBe('2026-09-13T22:00:00.000Z')
  })

  it('reads an empty timezone as UTC', () => {
    const { current } = resolveTransactionalPeriod('7d', '')
    expect(current.endDay).toBe('2026-11-11')
    expect(current.endUtc).toBe('2026-11-11T23:59:59.999Z')
  })
})
