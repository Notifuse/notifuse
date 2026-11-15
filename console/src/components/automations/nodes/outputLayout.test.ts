import { describe, it, expect } from 'vitest'
import { outputLeft, outputSpacing } from './outputLayout'

describe('outputLayout', () => {
  it('puts the filter outputs at 30 and 70 %, the others spread from 20 to 80 %', () => {
    expect([outputLeft('filter', 0, 2), outputLeft('filter', 1, 2)]).toEqual([30, 70])
    expect([0, 1, 2].map((i) => outputLeft('list_status_branch', i, 3))).toEqual([20, 50, 80])
    expect([0, 1, 2, 3].map((i) => outputLeft('ab_test', i, 4))).toEqual([20, 40, 60, 80])
  })

  it('gives the space between neighbouring outputs', () => {
    expect(outputSpacing('filter', 2)).toBe(40)
    expect(outputSpacing('list_status_branch', 3)).toBe(30)
    expect(outputSpacing('ab_test', 5)).toBe(15)
    expect(outputSpacing('branch', 1)).toBe(60)
  })
})
