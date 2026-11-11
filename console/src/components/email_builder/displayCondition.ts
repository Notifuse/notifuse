import type {
  DisplayCondition,
  DisplayConditionOperator,
  DisplayConditionRule,
  EmailBlock,
  MJMLComponentType
} from './types'
import { EmailBlockClass } from './EmailBlockClass'
import { COMMON_TEMPLATE_VARIABLES } from './templateVariables'

// The editor's evaluation of display conditions. Sending evaluates the Liquid that
// pkg/notifuse_mjml/display_condition.go generates, with liquidgo; this file mirrors
// liquidgo's reading of that Liquid, and both are checked against the same cases in
// pkg/notifuse_mjml/testdata/display_conditions.json.

export const DISPLAY_CONDITION_OPERATORS: readonly DisplayConditionOperator[] = [
  'is_true',
  'is_blank',
  'is_present',
  'is_absent',
  'eq',
  'neq',
  'contains',
  'not_contains',
  'gt',
  'gte',
  'lt',
  'lte'
]

export const MAX_DISPLAY_CONDITION_RULES = 20
export const MAX_DISPLAY_CONDITION_VARIABLE_LENGTH = 200
export const MAX_DISPLAY_CONDITION_VALUE_LENGTH = 200 // Unicode code points

// Blocks a condition may hide: hiding a column or a group would leave a hole in a
// layout of explicit widths.
export const CONDITION_ELIGIBLE_TYPES: readonly MJMLComponentType[] = [
  'mj-wrapper',
  'mj-section',
  'mj-text',
  'mj-image',
  'mj-button',
  'mj-divider',
  'mj-spacer',
  'mj-social',
  'mj-raw'
]

const VALUELESS_OPERATORS: ReadonlySet<string> = new Set([
  'is_true',
  'is_blank',
  'is_present',
  'is_absent'
])
const ORDER_OPERATORS: ReadonlySet<string> = new Set(['gt', 'gte', 'lt', 'lte'])
const KNOWN_OPERATORS: ReadonlySet<string> = new Set(DISPLAY_CONDITION_OPERATORS)
const RESERVED_FIRST_SEGMENTS: ReadonlySet<string> = new Set([
  'true',
  'false',
  'nil',
  'null',
  'empty',
  'blank',
  'and',
  'or',
  'contains'
])

const VARIABLE_REGEX = /^[A-Za-z_][A-Za-z0-9_]*(\.[A-Za-z0-9_]+)*$/
const NUMBER_REGEX = /^[+-]?(\d+(\.\d*)?|\.\d+)([eE][+-]?\d+)?$/
const EQ_NUMBER_REGEX = /^-?\d+(\.\d+)?$/
// Numbers of at most 15 integer digits, which liquidgo and float64 read alike
const MAX_CONDITION_NUMBER = 1e15

export function operatorTakesValue(operator: string): boolean {
  return KNOWN_OPERATORS.has(operator) && !VALUELESS_OPERATORS.has(operator)
}

export function isOrderOperator(operator: string): boolean {
  return ORDER_OPERATORS.has(operator)
}

// Go's unicode.IsSpace, which liquidgo's strip and blank use (JavaScript's \s differs).
const GO_SPACE: ReadonlySet<string> = new Set([
  '\t',
  '\n',
  '\v',
  '\f',
  '\r',
  ' ',
  '\u0085',
  ' ',
  ' ',
  ' ',
  ' ',
  ' ',
  ' ',
  ' ',
  ' ',
  ' ',
  ' ',
  ' ',
  ' ',
  ' ',
  ' ',
  ' ',
  ' ',
  ' ',
  '　'
])

export function trimGoSpace(s: string): string {
  let start = 0
  let end = s.length
  while (start < end && GO_SPACE.has(s[start])) start++
  while (end > start && GO_SPACE.has(s[end - 1])) end--
  return s.slice(start, end)
}

// formatGoFloat writes n as Go's strconv.FormatFloat(n, 'f', -1, 64) and liquidgo do:
// the shortest digits, never an exponent.
export function formatGoFloat(n: number): string {
  if (n === 0) return '0'
  const [mantissa, exponentText] = Math.abs(n).toExponential().split('e')
  const digits = mantissa.replace('.', '')
  const exponent = parseInt(exponentText, 10)
  let integer: string
  let fraction: string
  if (exponent >= 0) {
    const padded = digits.padEnd(exponent + 1, '0')
    integer = padded.slice(0, exponent + 1)
    fraction = padded.slice(exponent + 1)
  } else {
    integer = '0'
    fraction = '0'.repeat(-exponent - 1) + digits
  }
  return (n < 0 ? '-' : '') + integer + (fraction ? '.' + fraction : '')
}

interface ConditionNumber {
  value: number
  text: string
}

// conditionNumber parses an order operator's value, normalised as the Liquid writes it.
function conditionNumber(value: string): ConditionNumber | null {
  const trimmed = trimGoSpace(value)
  if (!NUMBER_REGEX.test(trimmed)) return null
  const n = Number(trimmed)
  if (!Number.isFinite(n) || Math.abs(n) >= MAX_CONDITION_NUMBER) return null
  const normalised = n === 0 ? 0 : n
  return { value: normalised, text: formatGoFloat(normalised) }
}

// eqNumber is the number an eq/neq/contains value also matches: "100" matches 100.
function eqNumber(value: string): ConditionNumber | null {
  return EQ_NUMBER_REGEX.test(value) ? conditionNumber(value) : null
}

// liquidgo reads these segments as lookup commands, which resolveVariable does not
// mirror: refused on both sides so the editor and the send never disagree.
const UNSUPPORTED_SEGMENTS = new Set(['empty', 'to_liquid'])

// usesUnsupportedLookup reports a path with an .empty or .to_liquid segment.
export function usesUnsupportedLookup(variable: string): boolean {
  return variable
    .split('.')
    .slice(1)
    .some((segment) => UNSUPPORTED_SEGMENTS.has(segment))
}

export function isValidConditionVariable(variable: unknown): boolean {
  if (
    typeof variable !== 'string' ||
    variable.length > MAX_DISPLAY_CONDITION_VARIABLE_LENGTH ||
    !VARIABLE_REGEX.test(variable)
  ) {
    return false
  }
  const first = variable.split('.')[0]
  return (
    !RESERVED_FIRST_SEGMENTS.has(first) &&
    !first.startsWith('__dc') &&
    !usesUnsupportedLookup(variable)
  )
}

export type DisplayConditionRuleError =
  | 'invalid_variable'
  | 'unsupported_lookup'
  | 'unknown_operator'
  | 'value_not_allowed'
  | 'value_required'
  | 'value_too_long'
  | 'value_has_liquid'
  | 'value_not_number'

export type DisplayConditionError =
  | { kind: 'invalid_match' }
  | { kind: 'no_rules' }
  | { kind: 'too_many_rules' }
  | { kind: 'rule'; index: number; error: DisplayConditionRuleError }

// validateRule mirrors DisplayConditionRule.validate in Go: the first reason the rule
// cannot be saved, or null.
// A rule from the AI assistant or an imported file may have any shape: anything
// malformed is an error, never an exception.
export function validateRule(rule: DisplayConditionRule): DisplayConditionRuleError | null {
  if (!isPlainObject(rule) || !isValidConditionVariable(rule.variable)) {
    return isPlainObject(rule) &&
      typeof rule.variable === 'string' &&
      usesUnsupportedLookup(rule.variable)
      ? 'unsupported_lookup'
      : 'invalid_variable'
  }
  if (VALUELESS_OPERATORS.has(rule.operator)) {
    return rule.value === undefined || rule.value === null ? null : 'value_not_allowed'
  }
  if (!KNOWN_OPERATORS.has(rule.operator)) return 'unknown_operator'
  if (typeof rule.value !== 'string' || trimGoSpace(rule.value) === '') {
    return 'value_required'
  }
  if ([...rule.value].length > MAX_DISPLAY_CONDITION_VALUE_LENGTH) return 'value_too_long'
  if (['{{', '}}', '{%', '%}'].some((delimiter) => rule.value!.includes(delimiter))) {
    return 'value_has_liquid'
  }
  if (ORDER_OPERATORS.has(rule.operator) && conditionNumber(rule.value) === null) {
    return 'value_not_number'
  }
  return null
}

// validateCondition mirrors DisplayCondition.Validate in Go.
export function validateCondition(condition: DisplayCondition): DisplayConditionError | null {
  if (!isPlainObject(condition)) return { kind: 'invalid_match' }
  if (condition.match !== 'all' && condition.match !== 'any') return { kind: 'invalid_match' }
  if (!Array.isArray(condition.rules) || condition.rules.length === 0) return { kind: 'no_rules' }
  if (condition.rules.length > MAX_DISPLAY_CONDITION_RULES) return { kind: 'too_many_rules' }
  for (let index = 0; index < condition.rules.length; index++) {
    const error = validateRule(condition.rules[index])
    if (error) return { kind: 'rule', index, error }
  }
  return null
}

type PlainObject = Record<string, unknown>

function isPlainObject(x: unknown): x is PlainObject {
  return typeof x === 'object' && x !== null && !Array.isArray(x)
}

function hasOwn(object: PlainObject, key: string): boolean {
  return Object.prototype.hasOwnProperty.call(object, key)
}

// resolveVariable reads a dotted path from the data as liquidgo does: own keys of
// objects, indexes and size/first/last of lists, size of text and of objects without
// a size key. Anything else is absent (undefined).
export function resolveVariable(data: unknown, variable: string): unknown {
  const segments = variable.split('.')
  if (!isPlainObject(data) || !hasOwn(data, segments[0])) return undefined
  let current: unknown = data[segments[0]]
  for (const segment of segments.slice(1)) {
    if (Array.isArray(current)) {
      if (/^\d+$/.test(segment)) current = current[parseInt(segment, 10)]
      else if (segment === 'size') current = current.length
      else if (segment === 'first') current = current[0]
      else if (segment === 'last') current = current[current.length - 1]
      else return undefined
    } else if (isPlainObject(current)) {
      if (hasOwn(current, segment)) current = current[segment]
      else if (segment === 'size') current = Object.keys(current).length
      else return undefined
    } else if (typeof current === 'string' && segment === 'size') {
      current = [...current].length
    } else {
      return undefined
    }
  }
  return current
}

function isBlank(x: unknown): boolean {
  return x === undefined || x === null || (typeof x === 'string' && trimGoSpace(x) === '')
}

function isEmpty(x: unknown): boolean {
  return (
    x === undefined ||
    x === null ||
    x === '' ||
    (Array.isArray(x) && x.length === 0) ||
    (isPlainObject(x) && Object.keys(x).length === 0)
  )
}

type LiquidOperand = string | number | boolean

// Liquid's ==, for the operands a condition writes
function liquidEquals(x: unknown, y: LiquidOperand): boolean {
  if (typeof y === 'boolean') return x === y
  if (typeof y === 'number') return typeof x === 'number' && x === y
  return typeof x === 'string' && x === y
}

// Liquid's contains: a substring of text, an element of a list, a key of an object
function liquidContains(x: unknown, y: string | ConditionNumber): boolean {
  const text = typeof y === 'string' ? y : y.text
  if (typeof x === 'string') return x.includes(text)
  if (Array.isArray(x)) return x.some((e) => liquidEquals(e, typeof y === 'string' ? y : y.value))
  if (isPlainObject(x)) return hasOwn(x, text)
  return false
}

// numericValue is the number the generated guard reads from x: a number, or a text of
// decimal digits with an optional sign and one point, at most 15 integer digits.
function numericValue(x: unknown): number | null {
  if (typeof x === 'number') return Math.abs(x) < MAX_CONDITION_NUMBER ? x : null
  if (typeof x !== 'string') return null
  const s = trimGoSpace(x)
  const sign = s.slice(0, 1)
  const body = sign === '-' || sign === '+' ? s.slice(1) : s
  if (!/^[0-9]/.test(body)) return null
  const rest = body.replace(/[0-9]/g, '')
  if (rest !== '' && rest !== '.') return null
  if (body.split('.')[0].length > 15) return null
  const n = Number(body)
  return sign === '-' ? -n : n
}

export function evaluateRule(rule: DisplayConditionRule, data: unknown): boolean {
  if (!isPlainObject(rule) || !isValidConditionVariable(rule.variable)) return false
  const x = resolveVariable(data, rule.variable)
  switch (rule.operator) {
    case 'is_true':
      return x === true || x === 'true'
    case 'is_blank':
      return isBlank(x) || x === false || isEmpty(x)
    case 'is_present':
      return !isBlank(x) && !isEmpty(x)
    case 'is_absent':
      return isBlank(x) || isEmpty(x)
  }
  const value = rule.value
  if (typeof value !== 'string' || trimGoSpace(value) === '') return false
  if (ORDER_OPERATORS.has(rule.operator)) {
    const literal = conditionNumber(value)
    const n = numericValue(x)
    if (literal === null || n === null) return false
    switch (rule.operator) {
      case 'gt':
        return n > literal.value
      case 'gte':
        return n >= literal.value
      case 'lt':
        return n < literal.value
      default:
        return n <= literal.value
    }
  }
  const number = eqNumber(value)
  switch (rule.operator) {
    case 'eq':
    case 'neq': {
      const alternatives: LiquidOperand[] = [value]
      if (number) alternatives.push(number.value)
      if (value === 'true' || value === 'false') alternatives.push(value === 'true')
      const equal = alternatives.some((alternative) => liquidEquals(x, alternative))
      return rule.operator === 'eq' ? equal : !equal
    }
    case 'contains':
    case 'not_contains': {
      const contained = liquidContains(x, value) || (number !== null && liquidContains(x, number))
      return rule.operator === 'contains' ? contained : !contained
    }
  }
  return false
}

// evaluateDisplayCondition reports whether the block shows with data, as a send does.
// No condition, or one without rules, shows the block; an unknown match reads as "all".
export function evaluateDisplayCondition(
  condition: DisplayCondition | undefined | null,
  data: unknown
): boolean {
  if (!isPlainObject(condition) || !Array.isArray(condition.rules) || condition.rules.length === 0)
    return true
  if (condition.match === 'any') return condition.rules.some((rule) => evaluateRule(rule, data))
  return condition.rules.every((rule) => evaluateRule(rule, data))
}

// isConditionEligible reports whether block may carry a condition in tree: an eligible
// type, outside mj-head (whose mj-raw and mj-attributes defaults are not shown blocks).
export function isConditionEligible(tree: EmailBlock, block: EmailBlock): boolean {
  return (
    CONDITION_ELIGIBLE_TYPES.includes(block.type) &&
    !EmailBlockClass.isChildOf(tree, block.id, 'mj-head')
  )
}

// holdsLiquidBlock reports an mj-liquid under block: hiding the block would drop it,
// and it can hold one half of a tag pair whose other half stays (ValidateDisplayConditions
// in Go refuses the condition).
export function holdsLiquidBlock(block: EmailBlock): boolean {
  return ((block.children as EmailBlock[] | undefined) ?? []).some(
    (child) => !!child && (child.type === 'mj-liquid' || holdsLiquidBlock(child))
  )
}

// conditionedContainer is the block with a display condition among blockId and its
// ancestors, the one an mj-liquid placed in blockId would sit under, or null.
export function conditionedContainer(tree: EmailBlock, blockId: string): EmailBlock | null {
  const path = (block: EmailBlock): EmailBlock[] | null => {
    if (block.id === blockId) return [block]
    for (const child of (block.children as EmailBlock[] | undefined) ?? []) {
      const found = child ? path(child) : null
      if (found) return [block, ...found]
    }
    return null
  }
  return (path(tree) ?? []).reverse().find((block) => !!block.display_condition) ?? null
}

// liquidPlacementError is why block cannot go under parentId: it is or holds an
// mj-liquid and parentId sits in a block with a display condition, which the server
// would then refuse (see holdsLiquidBlock). Null when it can.
export function liquidPlacementError(
  tree: EmailBlock,
  parentId: string,
  block: Pick<EmailBlock, 'type' | 'children'>
): string | null {
  if (block.type !== 'mj-liquid' && !holdsLiquidBlock(block as EmailBlock)) return null
  const container = conditionedContainer(tree, parentId)
  return container
    ? `block ${container.id} (${container.type}) has a display condition and cannot hold an mj-liquid block`
    : null
}

// conditionErrorsInTree lists every condition the server would refuse in tree, with
// the block it sits on.
export function conditionErrorsInTree(tree: EmailBlock): string[] {
  const errors: string[] = []
  const walk = (block: EmailBlock, inHead: boolean) => {
    const head = inHead || block.type === 'mj-head'
    if (block.display_condition) {
      if (head || !CONDITION_ELIGIBLE_TYPES.includes(block.type)) {
        errors.push(`Block ${block.id} (${block.type}) cannot have a display condition`)
      } else if (holdsLiquidBlock(block)) {
        errors.push(
          `Block ${block.id} (${block.type}) holds an mj-liquid block and cannot have a display condition`
        )
      } else {
        const error = validateCondition(block.display_condition)
        if (error) errors.push(`Block ${block.id}: ${describeConditionError(error)}`)
      }
    }
    for (const child of (block.children as EmailBlock[] | undefined) ?? []) {
      if (child) walk(child, head)
    }
  }
  walk(tree, false)
  return errors
}

// describeConditionError is the English text of a validation error, for the AI
// assistant; the editor shows its own translated messages.
export function describeConditionError(error: DisplayConditionError): string {
  switch (error.kind) {
    case 'invalid_match':
      return 'match must be "all" or "any"'
    case 'no_rules':
      return 'a display condition needs at least one rule'
    case 'too_many_rules':
      return `a display condition has at most ${MAX_DISPLAY_CONDITION_RULES} rules`
    case 'rule': {
      const messages: Record<DisplayConditionRuleError, string> = {
        invalid_variable: 'invalid variable path',
        unsupported_lookup: 'the .empty and .to_liquid lookups are not supported, use is_blank',
        unknown_operator: 'unknown operator',
        value_not_allowed: 'this operator takes no value',
        value_required: 'this operator needs a value',
        value_too_long: `the value is longer than ${MAX_DISPLAY_CONDITION_VALUE_LENGTH} characters`,
        value_has_liquid: 'the value cannot contain Liquid delimiters',
        value_not_number:
          'this operator needs a number of at most 15 digits before the decimal point'
      }
      return `rule ${error.index + 1}: ${messages[error.error]}`
    }
  }
}

// countHiddenBlocks counts the blocks data hides; a hidden block's children are not
// counted again.
export function countHiddenBlocks(tree: EmailBlock, data: unknown): number {
  if (tree.display_condition && !evaluateDisplayCondition(tree.display_condition, data)) return 1
  let count = 0
  for (const child of (tree.children as EmailBlock[] | undefined) ?? [])
    count += countHiddenBlocks(child, data)
  return count
}

export interface VariableSuggestion {
  variable: string
  sample?: string
}

const MAX_SUGGESTIONS = 200
const MAX_SUGGESTION_DEPTH = 6

function sampleOf(value: unknown): string | undefined {
  if (value === null || value === undefined) return undefined
  if (typeof value === 'string') return value.length > 40 ? value.slice(0, 40) + '…' : value
  if (typeof value === 'number' || typeof value === 'boolean') return String(value)
  if (Array.isArray(value)) return `[${value.length}]`
  return undefined
}

// collectVariableSuggestions lists the variable paths of the test data, with a sample
// value, followed by the variables every send fills in.
export function collectVariableSuggestions(testData: unknown): VariableSuggestion[] {
  const suggestions: VariableSuggestion[] = []
  const seen = new Set<string>()
  const add = (variable: string, sample?: string) => {
    if (
      seen.has(variable) ||
      suggestions.length >= MAX_SUGGESTIONS ||
      !isValidConditionVariable(variable)
    )
      return
    seen.add(variable)
    suggestions.push({ variable, sample })
  }
  const walk = (value: unknown, path: string, depth: number) => {
    if (depth > MAX_SUGGESTION_DEPTH) return
    if (isPlainObject(value)) {
      for (const [key, child] of Object.entries(value)) {
        const childPath = path ? `${path}.${key}` : key
        if (!isPlainObject(child)) add(childPath, sampleOf(child))
        walk(child, childPath, depth + 1)
      }
    } else if (Array.isArray(value) && path) {
      add(`${path}.size`, String(value.length))
      if (value.length > 0) {
        if (!isPlainObject(value[0])) add(`${path}.first`, sampleOf(value[0]))
        walk(value[0], `${path}.first`, depth + 1)
      }
    }
  }
  walk(testData, '', 0)
  for (const variable of COMMON_TEMPLATE_VARIABLES) {
    if (variable.startsWith('contact.')) add(variable)
  }
  return suggestions
}

// displayConditionUpdateError reports why the AI assistant's update of a block's
// display_condition cannot be applied, or null: removing one (null) is always fine.
export function displayConditionUpdateError(
  tree: EmailBlock,
  blockId: string,
  updates: { display_condition?: DisplayCondition | null }
): string | null {
  if (
    !('display_condition' in updates) ||
    updates.display_condition === null ||
    updates.display_condition === undefined
  ) {
    return null
  }
  const block = EmailBlockClass.findBlockById(tree, blockId)
  if (!block) return null
  if (!isConditionEligible(tree, block)) {
    return `block ${block.id} (${block.type}) cannot have a display condition`
  }
  if (holdsLiquidBlock(block)) {
    return `block ${block.id} (${block.type}) holds an mj-liquid block and cannot have a display condition`
  }
  const error = validateCondition(updates.display_condition)
  return error ? describeConditionError(error) : null
}
