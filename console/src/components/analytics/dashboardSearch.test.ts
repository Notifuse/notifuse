import { describe, expect, it } from 'vitest'
import { validateDashboardSearch, dashboardPeriod, dashboardMetric } from './dashboardSearch'

describe('validateDashboardSearch', () => {
  it('keeps the non-default period and metric', () => {
    expect(validateDashboardSearch({ period: '7d', metric: 'opens' })).toEqual({
      period: '7d',
      metric: 'opens'
    })
    expect(validateDashboardSearch({ period: '90d', metric: 'clicks' })).toEqual({ period: '90d', metric: 'clicks' })
  })

  it('writes the defaults as undefined', () => {
    expect(validateDashboardSearch({ period: '30d', metric: 'sent' })).toEqual({
      period: undefined,
      metric: undefined
    })
  })

  it('drops unknown values, arrays and non-strings', () => {
    expect(validateDashboardSearch({ period: '14d', metric: 'bounces' })).toEqual({
      period: undefined,
      metric: undefined
    })
    expect(validateDashboardSearch({ period: ['7d'], metric: 7 })).toEqual({
      period: undefined,
      metric: undefined
    })
    expect(validateDashboardSearch({})).toEqual({
      period: undefined,
      metric: undefined
    })
  })

  it('resolves the defaults for the page', () => {
    expect(dashboardPeriod({})).toBe('30d')
    expect(dashboardPeriod({ period: '7d' })).toBe('7d')
    expect(dashboardMetric({})).toBe('sent')
    expect(dashboardMetric({ metric: 'clicks' })).toBe('clicks')
  })
})
