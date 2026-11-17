import type { DimensionFilter, FieldType, FilterOperator } from '../../services/api/segment'
import { isFilterRowComplete } from './tree_completeness'

/**
 * What a filter can compare and what value each comparison takes, for the editor's filter rows.
 * Mirrors what the query builder compiles (internal/service/query_builder.go): an operator offered
 * here for a type must compile for it.
 */

/** The value a filter row asks for once its operator is chosen. */
export type FilterValueKind =
  | 'none' // is set, is not set
  | 'text' // one string
  | 'texts' // several strings, matched with OR
  | 'number' // one number
  | 'date' // one instant
  | 'dateRange' // two instants
  | 'days' // a day count

/** The type a JSON value is read as, i.e. the `field_type` a JSON filter is saved with. */
export type JsonValueType = 'string' | 'number' | 'time'

export const OPERATORS_BY_FIELD_TYPE: Record<'string' | 'number' | 'time', FilterOperator[]> = {
  string: ['is_set', 'is_not_set', 'equals', 'not_equals', 'contains', 'not_contains'],
  number: ['is_set', 'is_not_set', 'equals', 'not_equals', 'gt', 'lt', 'gte', 'lte'],
  time: [
    'is_set',
    'is_not_set',
    'before_date',
    'after_date',
    'in_date_range',
    'not_in_date_range',
    'in_the_last_days',
    'not_in_the_last_days'
  ]
}

// A value read out of a JSON column. Dates offer no range: the JSON branch of the query builder
// takes exactly one value. They offer no greater / less than either: those saved number_values,
// which a date filter cannot carry, so the server refused every one of them.
export const OPERATORS_BY_JSON_VALUE_TYPE: Record<JsonValueType, FilterOperator[]> = {
  string: ['is_set', 'is_not_set', 'equals', 'not_equals', 'contains', 'not_contains', 'in_array'],
  number: ['is_set', 'is_not_set', 'equals', 'not_equals', 'gt', 'lt', 'gte', 'lte'],
  time: [
    'is_set',
    'is_not_set',
    'equals',
    'not_equals',
    'before_date',
    'after_date',
    'in_the_last_days',
    'not_in_the_last_days'
  ]
}

/** Every operator a saved filter may carry and still be shown, offered or not. */
export const KNOWN_OPERATORS: FilterOperator[] = [
  'is_set',
  'is_not_set',
  'equals',
  'not_equals',
  'contains',
  'not_contains',
  'starts_with',
  'ends_with',
  'gt',
  'gte',
  'lt',
  'lte',
  'in_date_range',
  'not_in_date_range',
  'before_date',
  'after_date',
  'in_the_last_days',
  'not_in_the_last_days',
  'in_array'
]

/** The JSON value type a filter on a JSON column reads: `json` (API only) reads as text. */
export const jsonValueType = (fieldType: FieldType | undefined): JsonValueType =>
  fieldType === 'number' || fieldType === 'time' ? fieldType : 'string'

/**
 * The operators a row offers. A saved operator outside the list (a JSON date « greater than » built
 * through the API) is kept at the end, so opening the row never rewrites it.
 */
export function operatorsFor(
  fieldType: FieldType | undefined,
  isJsonField: boolean,
  current?: string
): FilterOperator[] {
  const list = isJsonField
    ? OPERATORS_BY_JSON_VALUE_TYPE[jsonValueType(fieldType)]
    : OPERATORS_BY_FIELD_TYPE[fieldType === 'number' || fieldType === 'time' ? fieldType : 'string']
  if (current && !list.includes(current as FilterOperator) && KNOWN_OPERATORS.includes(current as FilterOperator)) {
    return [...list, current as FilterOperator]
  }
  return list
}

/** The value an operator takes for a field of the given type. */
export function valueKindOf(operator: string | undefined, fieldType: FieldType | undefined): FilterValueKind {
  switch (operator) {
    case undefined:
    case 'is_set':
    case 'is_not_set':
      return 'none'
    case 'contains':
    case 'not_contains':
      return 'texts'
    case 'in_array':
    case 'starts_with':
    case 'ends_with':
      return 'text'
    case 'before_date':
    case 'after_date':
      return 'date'
    case 'in_date_range':
    case 'not_in_date_range':
      return 'dateRange'
    case 'in_the_last_days':
    case 'not_in_the_last_days':
      return 'days'
    default:
      // equals, not_equals, gt, lt, gte, lte: the field's own type
      if (fieldType === 'number') return 'number'
      if (fieldType === 'time') return 'date'
      return 'text'
  }
}

/** Whether an operator is one the editor knows how to show. */
export const isKnownOperator = (operator: string | undefined): boolean =>
  !!operator && KNOWN_OPERATORS.includes(operator as FilterOperator)

/** The filter with its values dropped, for an operator or type change. */
export const withoutValues = (filter: DimensionFilter): DimensionFilter => {
  const next = { ...filter }
  delete next.string_values
  delete next.number_values
  return next
}

/** The `field_type` a filter on a schema field is saved with. */
export const fieldTypeOfSchemaField = (type: string | undefined): FieldType => {
  if (type === 'number' || type === 'boolean') return 'number'
  if (type === 'time') return 'time'
  // A JSON column is read as text until its value type says otherwise.
  return 'string'
}

/** Keys filtered by more than one row: the server would AND them, which is never what was meant. */
export const duplicatePropertyKeys = (filters: DimensionFilter[] | undefined): string[] => {
  const seen = new Set<string>()
  const duplicates = new Set<string>()
  for (const filter of filters ?? []) {
    const key = filter.field_name?.trim()
    if (!key) continue
    if (seen.has(key)) duplicates.add(key)
    seen.add(key)
  }
  return [...duplicates]
}

/** Whether every property row may be confirmed: complete, and each key filtered once. */
export const arePropertyFiltersValid = (filters: DimensionFilter[] | undefined): boolean =>
  (filters ?? []).every((filter) => isFilterRowComplete(filter, false)) &&
  duplicatePropertyKeys(filters).length === 0
