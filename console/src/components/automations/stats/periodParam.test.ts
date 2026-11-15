import { describe, it, expect } from 'vitest'
import { paramFromPeriod, periodFromParam, readPeriodParam } from './periodParam'
import type { StatsPeriod } from './nodeStatsState'

describe('periodParam', () => {
  it('reads 7 days, all time, and 30 days without a parameter', () => {
    expect(periodFromParam('7d')).toBe('last7')
    expect(periodFromParam('all')).toBe('all')
    expect(periodFromParam(undefined)).toBe('last30')
  })

  it('writes each period back to what it was read from', () => {
    for (const period of ['last7', 'last30', 'all'] as StatsPeriod[]) {
      expect(periodFromParam(paramFromPeriod(period))).toBe(period)
    }
    expect(paramFromPeriod('last30')).toBeUndefined()
  })

  it('reads only the periods the URL writes', () => {
    expect(readPeriodParam('7d')).toBe('7d')
    expect(readPeriodParam('all')).toBe('all')
    expect(readPeriodParam('30d')).toBeUndefined()
    expect(readPeriodParam(7)).toBeUndefined()
  })
})
