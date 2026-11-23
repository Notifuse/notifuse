import { describe, expect, it } from 'vitest'
import { fromDays, toDays } from './periods'

describe('retention periods', () => {
  it('shows a period in the largest unit that holds it exactly', () => {
    expect(fromDays(1095)).toEqual({ value: 3, unit: 'years' })
    expect(fromDays(761)).toEqual({ value: 25, unit: 'months' })
    expect(fromDays(183)).toEqual({ value: 6, unit: 'months' })
    expect(fromDays(90)).toEqual({ value: 90, unit: 'days' })
    expect(fromDays(7)).toEqual({ value: 7, unit: 'days' })
  })

  it('converts back to the same number of days', () => {
    for (const days of [7, 30, 90, 180, 183, 365, 761, 1095, 3650]) {
      expect(toDays(fromDays(days))).toBe(days)
    }
    expect(toDays({ value: 1, unit: 'months' })).toBe(30)
    expect(toDays({ value: 2, unit: 'years' })).toBe(730)
  })
})
