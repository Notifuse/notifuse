import { describe, expect, it } from 'vitest'
import { WebFilter } from '../../../services/api/web_analytics'
import {
  evaluateCondition,
  evaluateConditions,
  simulateOperations,
  testAllFilters,
  TestValues
} from './filterEvaluator'

const rule = (overrides: Partial<WebFilter>): WebFilter => ({
  id: overrides.id ?? 'rule',
  name: overrides.name ?? 'Rule',
  priority: overrides.priority ?? 500,
  order: overrides.order ?? 0,
  conditions: overrides.conditions ?? [],
  operations: overrides.operations ?? [],
  enabled: overrides.enabled ?? true
})

describe('evaluateCondition', () => {
  // internal/domain/web_analytics_filters.go short-circuits on a missing or
  // empty field, so the tester must agree or it will predict the wrong rule.
  it('matches nothing but is_empty when the field is absent or empty', () => {
    const cases: TestValues[] = [{}, { utm_source: '' }]
    for (const values of cases) {
      expect(evaluateCondition({ field: 'utm_source', operator: 'is_empty' }, values)).toBe(true)
      expect(evaluateCondition({ field: 'utm_source', operator: 'is_not_empty' }, values)).toBe(
        false
      )
      expect(
        evaluateCondition({ field: 'utm_source', operator: 'not_equals', value: 'google' }, values)
      ).toBe(false)
      expect(
        evaluateCondition({ field: 'utm_source', operator: 'not_contains', value: 'goo' }, values)
      ).toBe(false)
      expect(evaluateCondition({ field: 'utm_source', operator: 'equals', value: '' }, values)).toBe(
        false
      )
      expect(
        evaluateCondition({ field: 'utm_source', operator: 'contains', value: '' }, values)
      ).toBe(false)
      expect(evaluateCondition({ field: 'utm_source', operator: 'regex', value: '.*' }, values)).toBe(
        false
      )
    }
  })

  it('compares a present value with every operator', () => {
    const values = { utm_source: 'google' }
    expect(evaluateCondition({ field: 'utm_source', operator: 'equals', value: 'google' }, values)).toBe(true)
    expect(evaluateCondition({ field: 'utm_source', operator: 'equals', value: 'bing' }, values)).toBe(false)
    expect(evaluateCondition({ field: 'utm_source', operator: 'not_equals', value: 'bing' }, values)).toBe(true)
    expect(evaluateCondition({ field: 'utm_source', operator: 'contains', value: 'oog' }, values)).toBe(true)
    expect(evaluateCondition({ field: 'utm_source', operator: 'not_contains', value: 'bing' }, values)).toBe(true)
    expect(evaluateCondition({ field: 'utm_source', operator: 'is_not_empty' }, values)).toBe(true)
    expect(evaluateCondition({ field: 'utm_source', operator: 'is_empty' }, values)).toBe(false)
    expect(evaluateCondition({ field: 'utm_source', operator: 'regex', value: '^goo' }, values)).toBe(true)
  })

  it('treats an unparseable regex as no match instead of throwing', () => {
    expect(
      evaluateCondition({ field: 'utm_source', operator: 'regex', value: '([' }, {
        utm_source: 'google'
      })
    ).toBe(false)
  })
})

describe('evaluateConditions', () => {
  it('requires every condition to match', () => {
    const conditions: WebFilter['conditions'] = [
      { field: 'utm_source', operator: 'equals', value: 'google' },
      { field: 'utm_medium', operator: 'equals', value: 'cpc' }
    ]
    expect(evaluateConditions(conditions, { utm_source: 'google', utm_medium: 'cpc' })).toBe(true)
    expect(evaluateConditions(conditions, { utm_source: 'google', utm_medium: 'organic' })).toBe(
      false
    )
  })

  it('always matches a rule with no conditions, which is how the catch-all works', () => {
    expect(evaluateConditions([], {})).toBe(true)
  })
})

describe('simulateOperations', () => {
  const operations: WebFilter['operations'] = [
    { dimension: 'channel', action: 'set_value', value: 'google-ads' },
    { dimension: 'utm_term', action: 'unset_value' },
    { dimension: 'channel_group', action: 'set_default_value', value: 'search-paid' }
  ]

  it('reports what each operation would write when the rule matches', () => {
    expect(simulateOperations(operations, true)).toEqual([
      { dimension: 'channel', action: 'set_value', resultValue: 'google-ads' },
      { dimension: 'utm_term', action: 'unset_value', resultValue: null },
      { dimension: 'channel_group', action: 'set_default_value', resultValue: 'search-paid' }
    ])
  })

  it('writes nothing when the rule does not match', () => {
    expect(simulateOperations(operations, false).every((result) => result.resultValue === null)).toBe(
      true
    )
  })
})

describe('testAllFilters', () => {
  it('reports every rule, highest priority first, as the engine applies them', () => {
    const results = testAllFilters(
      [
        rule({ id: 'low', priority: 10, conditions: [] }),
        rule({
          id: 'high',
          priority: 900,
          conditions: [{ field: 'utm_id_from', operator: 'equals', value: 'gclid' }]
        })
      ],
      { utm_id_from: 'gclid' }
    )

    expect(results.map((result) => result.filter.id)).toEqual(['high', 'low'])
    expect(results.every((result) => result.matches)).toBe(true)
  })
})
