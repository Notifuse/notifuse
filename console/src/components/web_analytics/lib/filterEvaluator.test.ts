import { describe, expect, it } from 'vitest'
import { WebFilter } from '../../../services/api/web_analytics'
import {
  compileRuleRegex,
  conditionResults,
  evaluateCondition,
  evaluateConditions,
  fieldsRead,
  firstFailingCondition,
  isUntestedCondition,
  readRuleRegex,
  ruleOutcome,
  simulateOperations,
  testAllFilters,
  TestValues,
  toTestValues
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

describe('toTestValues', () => {
  it('trims each value and leaves out the blank ones', () => {
    expect(toTestValues({ utm_source: ' newsletter ', utm_medium: '   ', utm_term: '', utm_id: undefined })).toEqual({
      utm_source: 'newsletter'
    })
    expect(toTestValues(undefined)).toEqual({})
  })
})

describe('fieldsRead', () => {
  it('lists each field once, in condition order', () => {
    expect(
      fieldsRead([
        { field: 'utm_medium', operator: 'equals', value: 'cpc' },
        { field: 'utm_source', operator: 'contains', value: 'goo' },
        { field: 'utm_medium', operator: 'not_equals', value: 'email' }
      ])
    ).toEqual(['utm_medium', 'utm_source'])
  })
})

describe('conditionResults and firstFailingCondition', () => {
  const conditions: WebFilter['conditions'] = [
    { field: 'utm_source', operator: 'equals', value: 'google' },
    { field: 'utm_medium', operator: 'equals', value: 'cpc' },
    { field: 'utm_term', operator: 'is_not_empty' }
  ]

  it('reports each condition and the first one the visit fails', () => {
    const values = { utm_source: 'google', utm_medium: 'email' }
    expect(conditionResults(conditions, values)).toEqual([true, false, false])
    expect(firstFailingCondition(conditions, values)).toBe(1)
  })

  it('answers null when the rule matches, or has no condition', () => {
    expect(firstFailingCondition(conditions, { utm_source: 'google', utm_medium: 'cpc', utm_term: 'x' })).toBeNull()
    expect(firstFailingCondition([], {})).toBeNull()
  })
})

describe('compileRuleRegex', () => {
  it('runs the inline flags RE2 accepts and JavaScript does not', () => {
    expect(evaluateCondition({ field: 'utm_source', operator: 'regex', value: '(?i)^google$' }, { utm_source: 'Google' })).toBe(
      true
    )
    expect(compileRuleRegex('(?i)^google$')?.flags).toBe('i')
  })

  it('answers null for a pattern JavaScript cannot compile', () => {
    expect(compileRuleRegex('([')).toBeNull()
  })

  it('runs a Go named group as a JavaScript one', () => {
    expect(compileRuleRegex('^(?P<host>[a-z]+)\\.com$')?.source).toBe('^(?<host>[a-z]+)\\.com$')
    expect(
      evaluateCondition({ field: 'referrer_domain', operator: 'regex', value: '^(?P<host>[a-z]+)\\.com$' }, { referrer_domain: 'bing.com' })
    ).toBe(true)
  })

  it('reads a ] first in a class as RE2 does, a plain ]', () => {
    expect(evaluateCondition({ field: 'utm_term', operator: 'regex', value: '^[]a]+$' }, { utm_term: 'a]a' })).toBe(true)
  })
})

describe('readRuleRegex', () => {
  it('says which RE2 syntax the browser cannot run', () => {
    for (const pattern of ['(?U)a+', 'google\\z', '\\Qa.b\\E', '^[[:alpha:]]+$', '\\p{Greek}', '\\x{41}']) {
      expect(readRuleRegex(pattern).kind).toBe('untestable')
    }
    expect(readRuleRegex('(a)\\1').kind).toBe('refused')
    expect(readRuleRegex('a\\\\1').kind).toBe('ok')
  })
})

describe('isUntestedCondition', () => {
  const condition = { field: 'utm_source', operator: 'regex' as const, value: 'google\\z' }

  const cpc = { field: 'utm_medium', operator: 'equals' as const, value: 'cpc' }

  it('skips a regex only RE2 reads rather than failing the condition', () => {
    expect(isUntestedCondition(condition, { utm_source: 'google' })).toBe(true)
    expect(evaluateCondition(condition, { utm_source: 'bing' })).toBe(true)
  })

  it('leaves the rule untested when nothing else fails, and no match when something does', () => {
    expect(ruleOutcome([condition, cpc], { utm_source: 'x', utm_medium: 'cpc' })).toBe('untested')
    expect(evaluateConditions([condition, cpc], { utm_source: 'x', utm_medium: 'cpc' })).toBe(false)
    expect(ruleOutcome([condition, cpc], { utm_source: 'x', utm_medium: 'email' })).toBe('no_match')
    expect(ruleOutcome([cpc], { utm_medium: 'cpc' })).toBe('matches')
  })

  it('writes nothing for an untested rule', () => {
    const operations = [{ dimension: 'channel', action: 'set_value' as const, value: 'Search' }]
    const [row] = testAllFilters([rule({ conditions: [condition], operations })], { utm_source: 'google' })
    expect(row.outcome).toBe('untested')
    expect(row.matches).toBe(false)
    expect(row.operationResults).toEqual([{ dimension: 'channel', action: 'set_value', resultValue: null }])
  })

  it('still says an empty value never matches it, as the engine does', () => {
    expect(isUntestedCondition(condition, {})).toBe(false)
    expect(evaluateCondition(condition, {})).toBe(false)
  })
})
