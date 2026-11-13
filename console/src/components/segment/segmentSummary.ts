import type { I18n, MessageDescriptor } from '@lingui/core'
import { msg } from '@lingui/core/macro'
import type { DimensionFilter, TreeNode } from '../../services/api/segment'
import { ContactsTableSchema } from './table_schemas'

/** The first conditions of a segment's tree, for the "Defined by" bar. */
export interface SegmentSummary {
  /** Up to three described conditions of the tree's first level. */
  conditions: string[]
  /** "and" or "or", translated, from the root node. */
  joiner: string
  /** Conditions of the first level left out of `conditions`. */
  more: number
}

export interface SegmentSummaryOptions {
  /** Name of a list, for a list membership condition. */
  listName?: (listId: string) => string
  /** The workspace's custom field labels. */
  customFieldLabels?: Record<string, string>
}

const MAX_CONDITIONS = 3

// The segment schemas are written in English and not extracted: the standard contact fields
// carry their own catalogue messages here. A custom field shows its workspace label.
const FIELD_LABELS: Record<string, MessageDescriptor> = {
  email: msg`Email`,
  external_id: msg`External ID`,
  first_name: msg`First Name`,
  last_name: msg`Last Name`,
  full_name: msg`Full Name`,
  phone: msg`Phone`,
  country: msg`Country`,
  language: msg`Language`,
  timezone: msg`Timezone`,
  address_line_1: msg`Address Line 1`,
  address_line_2: msg`Address Line 2`,
  postcode: msg`Postcode`,
  state: msg`State`,
  job_title: msg`Job Title`,
  created_at: msg`Created At`,
  updated_at: msg`Updated At`,
  tracking_opens_consent: msg`Opens tracking consent`,
  tracking_clicks_consent: msg`Clicks tracking consent`
}

function fieldLabel(fieldName: string, i18n: I18n, options: SegmentSummaryOptions): string {
  const descriptor = FIELD_LABELS[fieldName]
  if (descriptor) return i18n._(descriptor)
  return options.customFieldLabels?.[fieldName] ?? ContactsTableSchema.fields[fieldName]?.title ?? fieldName
}

function describeFilter(filter: DimensionFilter, i18n: I18n, options: SegmentSummaryOptions): string | null {
  if (filter.field_type === 'json') return null
  const field = fieldLabel(filter.field_name, i18n, options)
  const values = filter.field_type === 'number' ? filter.number_values : filter.string_values
  const value = (values ?? []).join(', ')
  const first = values?.[0] ?? ''
  const second = values?.[1] ?? ''
  switch (filter.operator) {
    case 'is_set':
      return i18n._(msg`${{ field }} is set`)
    case 'is_not_set':
      return i18n._(msg`${{ field }} is not set`)
    case 'equals':
      return i18n._(msg`${{ field }} is ${{ value }}`)
    case 'not_equals':
      return i18n._(msg`${{ field }} is not ${{ value }}`)
    case 'contains':
    case 'in_array':
      return i18n._(msg`${{ field }} contains ${{ value }}`)
    case 'not_contains':
      return i18n._(msg`${{ field }} does not contain ${{ value }}`)
    case 'starts_with':
      return i18n._(msg`${{ field }} starts with ${{ value }}`)
    case 'ends_with':
      return i18n._(msg`${{ field }} ends with ${{ value }}`)
    case 'gt':
      return i18n._(msg`${{ field }} > ${{ value }}`)
    case 'gte':
      return i18n._(msg`${{ field }} ≥ ${{ value }}`)
    case 'lt':
      return i18n._(msg`${{ field }} < ${{ value }}`)
    case 'lte':
      return i18n._(msg`${{ field }} ≤ ${{ value }}`)
    case 'before_date':
      return i18n._(msg`${{ field }} before ${{ value }}`)
    case 'after_date':
      return i18n._(msg`${{ field }} after ${{ value }}`)
    case 'in_date_range':
      return i18n._(msg`${{ field }} between ${{ first }} and ${{ second }}`)
    case 'not_in_date_range':
      return i18n._(msg`${{ field }} not between ${{ first }} and ${{ second }}`)
    case 'in_the_last_days':
      return i18n._(msg`${{ field }} in the last ${{ days: first }} days`)
    case 'not_in_the_last_days':
      return i18n._(msg`${{ field }} not in the last ${{ days: first }} days`)
    default:
      return null
  }
}

function describeNode(node: TreeNode, i18n: I18n, options: SegmentSummaryOptions): string {
  const leaf = node.kind === 'leaf' ? node.leaf : undefined
  let described: string | null = null
  if (leaf?.source === 'contacts' && leaf.contact?.filters.length === 1) {
    described = describeFilter(leaf.contact.filters[0], i18n, options)
  } else if (leaf?.source === 'contact_lists' && leaf.contact_list) {
    const list = options.listName?.(leaf.contact_list.list_id) ?? leaf.contact_list.list_id
    described =
      leaf.contact_list.operator === 'not_in'
        ? i18n._(msg`Not in list ${{ list }}`)
        : i18n._(msg`In list ${{ list }}`)
  }
  return described ?? i18n._(msg`1 condition`)
}

/**
 * Describes the first level of a segment's tree: a leaf root is one condition, a branch root
 * lists its children. A node it cannot describe (a nested branch, an activity, a goal) reads
 * "1 condition".
 */
export function segmentSummary(
  tree: TreeNode,
  i18n: I18n,
  options: SegmentSummaryOptions = {}
): SegmentSummary {
  const nodes = tree.kind === 'branch' && tree.branch ? tree.branch.leaves : [tree]
  const operator = tree.kind === 'branch' && tree.branch?.operator === 'or' ? 'or' : 'and'
  return {
    conditions: nodes.slice(0, MAX_CONDITIONS).map((node) => describeNode(node, i18n, options)),
    joiner: operator === 'or' ? i18n._(msg`or`) : i18n._(msg`and`),
    more: Math.max(0, nodes.length - MAX_CONDITIONS)
  }
}
