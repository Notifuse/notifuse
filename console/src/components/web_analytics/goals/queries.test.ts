import { describe, expect, it } from 'vitest'
import { conversionRate, goalFilters } from './queries'

describe('conversionRate', () => {
  it('is 0 without sessions', () => {
    expect(conversionRate(3, 0)).toBe(0)
  })

  it('divides the conversions by the sessions, in percent', () => {
    expect(conversionRate(3, 200)).toBe(1.5)
  })

  it('can pass 100%, a goal being counted each time it is reached', () => {
    expect(conversionRate(250, 200)).toBe(125)
  })
})

describe('goalFilters', () => {
  it('replaces a page filter on the goal name by the goal viewed, and keeps the others', () => {
    expect(
      goalFilters(
        [
          { dimension: 'goal_name', operator: 'equals', values: ['purchase'] },
          { dimension: 'device', operator: 'equals', values: ['mobile'] }
        ],
        'signup'
      )
    ).toEqual([
      { dimension: 'device', operator: 'equals', values: ['mobile'] },
      { dimension: 'goal_name', operator: 'equals', values: ['signup'] }
    ])
  })
})
