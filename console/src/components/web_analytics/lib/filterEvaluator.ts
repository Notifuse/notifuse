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

/**
 * A rule against one visit: `untested` when no condition fails but one is a regex only the server
 * reads, so the browser cannot say whether the rule matches.
 */
export type RuleOutcome = 'matches' | 'no_match' | 'untested'

export interface FilterTestResult {
  outcome: RuleOutcome
  /** The rule surely matches (outcome `matches`); an untested rule writes nothing here. */
  matches: boolean
  operationResults: SimulatedOperation[]
}

export interface FilterTestRow extends FilterTestResult {
  filter: WebFilter
}

export type TestValues = Record<string, string | null>

/**
 * How the browser can read a rule's regular expression, which the server compiles with Go's RE2:
 * - `ok`: as `regex`, RE2 syntax translated (leading `(?ims)` flags, `(?P<name>…)` groups).
 * - `refused`: a lookaround or a backreference, which RE2 does not support: the server refuses it.
 * - `untestable`: RE2 syntax JavaScript cannot express (`(?U)`, flags after the start, `\A`, `\z`,
 *   `\Q…\E`, `\p{…}`, `\x{…}`, `[[:alpha:]]`): the server checks it on save, the tester skips it.
 * - `invalid`: JavaScript cannot compile it; the server may refuse it too.
 */
export type RuleRegexReading =
  | { kind: 'ok'; regex: RegExp }
  | { kind: 'refused' }
  | { kind: 'untestable' }
  | { kind: 'invalid' }

type Translation = { kind: 'refused' } | { kind: 'untestable' } | { kind: 'source'; source: string; flags: string }

/** Escapes RE2 reads differently, or JavaScript (without the u flag) reads as a plain letter. */
const UNTESTABLE_ESCAPES = new Set(['A', 'z', 'Q', 'p', 'P', 'C'])

/** RE2 syntax as JavaScript writes it, scanned token by token so an escaped `\\1` is a backslash and a 1. */
function translateRe2(pattern: string): Translation {
  let flags = ''
  let i = 0
  const lead = /^\(\?([a-zA-Z]+)\)/.exec(pattern)
  if (lead) {
    // (?U) makes every quantifier lazy: JavaScript has no such flag.
    if (/[^ims]/.test(lead[1])) return { kind: 'untestable' }
    flags = [...new Set(lead[1])].join('')
    i = lead[0].length
  }
  let source = ''
  let inClass = false
  while (i < pattern.length) {
    const char = pattern[i]
    if (char === '\\') {
      const next = pattern[i + 1] ?? ''
      if (UNTESTABLE_ESCAPES.has(next) || (next === 'x' && pattern[i + 2] === '{')) return { kind: 'untestable' }
      // \1 to \9 alone is a backreference, which RE2 refuses (\8 and \9 as invalid escapes); \1 to \7
      // followed by a digit is an octal code, which both read.
      if (!inClass && /[1-9]/.test(next) && !(/[1-7]/.test(next) && /[0-9]/.test(pattern[i + 2] ?? ''))) {
        return { kind: 'refused' }
      }
      source += char + next
      i += 2
      continue
    }
    if (inClass) {
      if (char === '[' && pattern[i + 1] === ':' && pattern.indexOf(':]', i + 2) !== -1) return { kind: 'untestable' }
      if (char === ']') inClass = false
      source += char
      i += 1
      continue
    }
    if (char === '[') {
      inClass = true
      source += char
      i += 1
      if (pattern[i] === '^') {
        source += '^'
        i += 1
      }
      // A ] first in a class is a plain ] in RE2, and closes an empty class in JavaScript.
      if (pattern[i] === ']') {
        source += '\\]'
        i += 1
      }
      continue
    }
    if (char === '(' && pattern[i + 1] === '?') {
      const rest = pattern.slice(i + 2)
      if (/^<?[=!]/.test(rest)) return { kind: 'refused' }
      const named = /^P<([A-Za-z_][A-Za-z0-9_]*)>/.exec(rest)
      if (named) {
        source += `(?<${named[1]}>`
        i += 2 + named[0].length
        continue
      }
      if (rest.startsWith(':') || /^<[A-Za-z_]/.test(rest)) {
        source += '(?'
        i += 2
        continue
      }
      // (?i) after the start, (?i:…) or (?U): flags JavaScript cannot set on part of a pattern.
      return { kind: 'untestable' }
    }
    source += char
    i += 1
  }
  return { kind: 'source', source, flags }
}

/**
 * A rule's regex as the browser can run it, or why it cannot. A pathological pattern still
 * backtracks in JavaScript (RE2 never does) and can hold the tab on a long value: an accepted risk
 * for a tester.
 */
export function readRuleRegex(pattern: string): RuleRegexReading {
  const translated = translateRe2(pattern)
  if (translated.kind !== 'source') return translated
  try {
    return { kind: 'ok', regex: new RegExp(translated.source, translated.flags) }
  } catch {
    return { kind: 'invalid' }
  }
}

/** A rule's regex as JavaScript can run it; null when it cannot (see readRuleRegex for why). */
export function compileRuleRegex(pattern: string): RegExp | null {
  const reading = readRuleRegex(pattern)
  return reading.kind === 'ok' ? reading.regex : null
}

/**
 * Whether the tester skips this condition on this visit: a regex in syntax only RE2 reads, on a
 * value to test (an empty value never matches a regex, which the browser can say).
 */
export function isUntestedCondition(condition: WebFilterCondition, testValues: TestValues): boolean {
  return (
    condition?.operator === 'regex' &&
    (testValues[condition.field] ?? '') !== '' &&
    readRuleRegex(condition.value ?? '').kind === 'untestable'
  )
}

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
    case 'regex': {
      const reading = readRuleRegex(conditionValue)
      // Syntax only RE2 reads is skipped: it does not fail the rule (see isUntestedCondition).
      if (reading.kind === 'untestable') return true
      return reading.kind === 'ok' ? reading.regex.test(testValue) : false
    }
    default:
      return false
  }
}

/**
 * All conditions must match; a rule without conditions always matches. A failing condition settles
 * it; otherwise a condition the browser cannot test leaves the rule untested.
 */
export function ruleOutcome(conditions: WebFilterCondition[], testValues: TestValues): RuleOutcome {
  if (!conditions.every((condition) => evaluateCondition(condition, testValues))) return 'no_match'
  return conditions.some((condition) => isUntestedCondition(condition, testValues)) ? 'untested' : 'matches'
}

/** Whether the rule surely matches the visit: not when a condition cannot be tested. */
export function evaluateConditions(conditions: WebFilterCondition[], testValues: TestValues): boolean {
  return ruleOutcome(conditions, testValues) === 'matches'
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
  const outcome = ruleOutcome(filter.conditions, testValues)
  const matches = outcome === 'matches'
  const operationResults = simulateOperations(filter.operations, matches)
  return { outcome, matches, operationResults }
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

/** A visit typed into a form: trimmed, with blank fields left out so they read as empty. */
export function toTestValues(values: Record<string, string | null | undefined> | undefined): TestValues {
  const testValues: TestValues = {}
  for (const [field, value] of Object.entries(values ?? {})) {
    const trimmed = typeof value === 'string' ? value.trim() : ''
    if (trimmed) testValues[field] = trimmed
  }
  return testValues
}

/** The fields a rule's conditions read, each once, in condition order. */
export function fieldsRead(conditions: WebFilterCondition[]): string[] {
  const fields: string[] = []
  for (const condition of conditions) {
    if (condition?.field && !fields.includes(condition.field)) fields.push(condition.field)
  }
  return fields
}

/** Whether each condition holds for the visit, in condition order. */
export function conditionResults(conditions: WebFilterCondition[], testValues: TestValues): boolean[] {
  return conditions.map((condition) => evaluateCondition(condition, testValues))
}

/** The index of the first condition the visit fails, null when the rule matches (or has none). */
export function firstFailingCondition(
  conditions: WebFilterCondition[],
  testValues: TestValues
): number | null {
  const index = conditions.findIndex((condition) => !evaluateCondition(condition, testValues))
  return index === -1 ? null : index
}
