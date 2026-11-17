import { describe, expect, it } from 'vitest'
import { describeDimensionFilter, describeMetricFilter } from './filterLabels'

const OPERATORS = { equals: 'equals', isEmpty: 'is empty', in: 'in', gte: '>=' }

describe('describeDimensionFilter', () => {
  it('writes the dimension, the operator and the value', () => {
    expect(describeDimensionFilter({ dimension: 'device', operator: 'equals', values: ['mobile'] }, OPERATORS)).toBe(
      'Device equals mobile'
    )
  })

  it('writes no value after an operator that takes none', () => {
    expect(describeDimensionFilter({ dimension: 'utm_term', operator: 'isEmpty', values: [] }, OPERATORS)).toBe(
      'UTM Term is empty'
    )
  })

  it('joins the values of an in filter', () => {
    expect(
      describeDimensionFilter({ dimension: 'device', operator: 'in', values: ['mobile', 'tablet'] }, OPERATORS)
    ).toBe('Device in mobile, tablet')
  })

  it('names a custom dimension by the workspace label', () => {
    expect(
      describeDimensionFilter({ dimension: 'custom_1', operator: 'equals', values: ['iPhone'] }, OPERATORS, {
        custom_1: 'Product line'
      })
    ).toBe('Product line equals iPhone')
  })
})

describe('describeMetricFilter', () => {
  it('writes a TimeScore threshold as a duration', () => {
    expect(
      describeMetricFilter({ metric: 'median_duration', operator: 'gte', values: [90] }, OPERATORS, {
        median_duration: 'TimeScore'
      })
    ).toBe('TimeScore >= 1 min 30 s')
  })
})
