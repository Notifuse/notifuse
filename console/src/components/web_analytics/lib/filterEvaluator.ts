import {
  WebFilter,
  WebFilterAction,
  WebFilterCondition,
  WebFilterOperation
} from '../../../services/api/web_analytics'

/**
 * Client-side twin of EvaluateWebFilterCondition in
 * internal/domain/web_analytics_filters.go, used only by the rule tester.
 *
 * The engine short-circuits on a missing or empty field: nothing matches an
 * absent value except is_empty. That asymmetry is what makes not_equals report
 * false on an empty input rather than "everything differs from nothing", and
 * the tester is worthless if it does not reproduce it exactly.
 */

export interface SimulatedOperation {
  dimension: string
  action: WebFilterAction
  resultValue: string | null
}

export interface FilterTestResult {
  matches: boolean
  operationResults: SimulatedOperation[]
}

export interface FilterTestRow extends FilterTestResult {
  filter: WebFilter
}

export type TestValues = Record<string, string | null>

export function evaluateCondition(condition: WebFilterCondition, testValues: TestValues): boolean {
  const testValue = testValues[condition.field] ?? ''
  const conditionValue = condition.value ?? ''

  if (testValue === '') return condition.operator === 'is_empty'

  switch (condition.operator) {
    case 'equals':
      return testValue === conditionValue
    case 'not_equals':
      return testValue !== conditionValue
    case 'contains':
      return testValue.includes(conditionValue)
    case 'not_contains':
      return !testValue.includes(conditionValue)
    case 'is_empty':
      return false
    case 'is_not_empty':
      return true
    case 'regex':
      try {
        return new RegExp(conditionValue).test(testValue)
      } catch {
        return false
      }
    default:
      return false
  }
}

/** All conditions must match; a rule without conditions always matches. */
export function evaluateConditions(
  conditions: WebFilterCondition[],
  testValues: TestValues
): boolean {
  if (conditions.length === 0) return true
  return conditions.every((condition) => evaluateCondition(condition, testValues))
}

export function simulateOperations(
  operations: WebFilterOperation[],
  matches: boolean
): SimulatedOperation[] {
  return operations.map((operation) => {
    let resultValue: string | null = null

    if (matches) {
      switch (operation.action) {
        case 'set_value':
          resultValue = operation.value ?? null
          break
        case 'unset_value':
          resultValue = null
          break
        case 'set_default_value':
          // The tester has no session to read, so it assumes the dimension is
          // still empty and the default therefore applies.
          resultValue = operation.value ?? null
          break
      }
    }

    return {
      dimension: operation.dimension,
      action: operation.action,
      resultValue
    }
  })
}

export function testFilter(filter: WebFilter, testValues: TestValues): FilterTestResult {
  const matches = evaluateConditions(filter.conditions, testValues)
  const operationResults = simulateOperations(filter.operations, matches)
  return { matches, operationResults }
}

/** Every rule against one synthetic session, highest priority first. */
export function testAllFilters(filters: WebFilter[], testValues: TestValues): FilterTestRow[] {
  return filters
    .map((filter) => ({
      filter,
      ...testFilter(filter, testValues)
    }))
    .sort((a, b) => b.filter.priority - a.filter.priority)
}
