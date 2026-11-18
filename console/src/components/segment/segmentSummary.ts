import type { I18n, MessageDescriptor } from '@lingui/core'
import { msg } from '@lingui/core/macro'
import type {
  ContactTimelineCondition,
  CustomEventsGoalCondition,
  DimensionFilter,
  TreeNode
} from '../../services/api/segment'
import { LIST_STATUS_LABEL } from '../contacts/contactDisplay'
import { AGGREGATE_LABELS, fieldLabel as contactFieldLabel, GOAL_LABELS } from './segmentLabels'

/** The first conditions of a segment's tree, for the "Defined by" bar. */
export interface SegmentSummary {
  /** Up to three described conditions of the tree's first level. */
  conditions: string[]
  /** "and" or "or", translated, from the root node. */
  joiner: string
  /** Conditions of the first level left out of `conditions`. */
  more: number
  /** Those left out, described, for a tooltip on « +N ». */
  hidden: string[]
}

export interface SegmentSummaryOptions {
  /** Name of a list, for a list membership condition. */
  listName?: (listId: string) => string
  /** The workspace's custom field labels. */
  customFieldLabels?: Record<string, string>
}

const MAX_CONDITIONS = 3

/** A contact field's name: the workspace's label, then the catalogue's, then the raw key. */
function fieldLabel(fieldName: string, i18n: I18n, options: SegmentSummaryOptions): string {
  return contactFieldLabel('contacts', fieldName, i18n, options.customFieldLabels)
}

/**
 * An ISO date as a date in the console language; any other value as it is. The form stores the
 * midnight picked in the browser's timezone as an instant, so an instant reads in that timezone;
 * a bare date (YYYY-MM-DD) reads as itself.
 */
function formatDate(value: string, i18n: I18n): string {
  if (!/^\d{4}-\d{2}-\d{2}/.test(value)) return value
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  const bareDate = value.length === 10
  return new Intl.DateTimeFormat(i18n.locale, { dateStyle: 'medium', ...(bareDate ? { timeZone: 'UTC' } : {}) }).format(
    date
  )
}

const DATE_OPERATORS = new Set(['before_date', 'after_date', 'in_date_range', 'not_in_date_range'])

function describeFilter(filter: DimensionFilter, i18n: I18n, options: SegmentSummaryOptions): string | null {
  // A filter on a key inside a JSON field narrows more than its column: not described. The form
  // saves it with the type of the key (string, number, time) and a path, not as « json ».
  if (filter.field_type === 'json' || filter.json_path?.length) return null
  const field = fieldLabel(filter.field_name, i18n, options)
  const raw = filter.field_type === 'number' ? filter.number_values : filter.string_values
  const values = DATE_OPERATORS.has(filter.operator) ? raw?.map((v) => formatDate(String(v), i18n)) : raw
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

// What each activity a timeline condition counts reads as, in the past tense of the contact.
const TIMELINE_ACTIONS: Record<string, MessageDescriptor> = {
  // Written when the email is sent, not when it is delivered.
  'email.sent': msg`Was sent an email`,
  'email.opened': msg`Opened an email`,
  'email.clicked': msg`Clicked in an email`,
  'email.bounced': msg`Had an email bounce`,
  'email.complained': msg`Marked an email as spam`,
  'email.unsubscribed': msg`Unsubscribed from an email`,
  'web.pageview': msg`Viewed a page`,
  'web.session': msg`Visited the website`
}


/** « in the last 30 days », « before 2026-01-01 »…, or nothing for any time. */
function describeTimeframe(operator: string | undefined, values: string[] | undefined, i18n: I18n): string | null {
  const days = values?.[0] ?? ''
  const first = formatDate(values?.[0] ?? '', i18n)
  const second = formatDate(values?.[1] ?? '', i18n)
  switch (operator) {
    case 'in_the_last_days':
      return i18n._(msg`in the last ${{ days }} days`)
    case 'before_date':
      return i18n._(msg`before ${{ date: first }}`)
    case 'after_date':
      return i18n._(msg`after ${{ date: first }}`)
    case 'in_date_range':
      return i18n._(msg`between ${{ first }} and ${{ second }}`)
    default:
      return null
  }
}

const withTimeframe = (condition: string, timeframe: string | null) =>
  timeframe ? `${condition} · ${timeframe}` : condition

function describeTimeline(condition: ContactTimelineCondition, i18n: I18n): string | null {
  const descriptor = TIMELINE_ACTIONS[condition.kind]
  // An activity the console does not name reads « 1 condition », not its raw kind.
  if (!descriptor) return null
  const action = i18n._(descriptor)
  const count = condition.count_value
  let counted: string
  if (condition.count_operator === 'at_most') counted = i18n._(msg`${{ action }} at most ${{ count }} times`)
  else if (condition.count_operator === 'exactly') counted = i18n._(msg`${{ action }} exactly ${{ count }} times`)
  else if (count === 1) counted = action
  else counted = i18n._(msg`${{ action }} at least ${{ count }} times`)
  // Narrowed to a template, a broadcast, a link or event properties: said, not detailed.
  const narrowed =
    !!condition.template_id || !!condition.broadcast_id || !!condition.link_url || !!condition.filters?.length
  if (narrowed) counted = i18n._(msg`${{ condition: counted }} (filtered)`)
  return withTimeframe(counted, describeTimeframe(condition.timeframe_operator, condition.timeframe_values, i18n))
}

function describeGoal(condition: CustomEventsGoalCondition, i18n: I18n): string {
  const goal = GOAL_LABELS[condition.goal_type]
  const type = goal ? i18n._(goal) : condition.goal_type
  // The type, the goal name and the event name all narrow the events: each one set is shown,
  // the event name said to be one (a bare “add_to_cart” read as a raw value).
  const goalName = condition.goal_name ? i18n._(msg`“${{ name: condition.goal_name }}”`) : null
  const eventName = condition.event_name
    ? goalName || condition.goal_type !== '*'
      ? i18n._(msg`event “${{ name: condition.event_name }}”`)
      : i18n._(msg`Event “${{ name: condition.event_name }}”`)
    : null
  const names = [goalName, eventName].filter((name): name is string => !!name)
  const subject = condition.goal_type === '*' && names.length > 0 ? names.join(' ') : [type, ...names].join(' ')
  const aggregateLabel = AGGREGATE_LABELS[condition.aggregate_operator]
  const aggregate = aggregateLabel ? i18n._(aggregateLabel) : condition.aggregate_operator
  const value = condition.value
  let compared: string
  switch (condition.operator) {
    case 'lte':
      compared = i18n._(msg`${{ subject }}: ${{ aggregate }} ≤ ${{ value }}`)
      break
    case 'eq':
      compared = i18n._(msg`${{ subject }}: ${{ aggregate }} = ${{ value }}`)
      break
    case 'between':
      compared = i18n._(
        msg`${{ subject }}: ${{ aggregate }} between ${{ value }} and ${{ second: condition.value_2 ?? '' }}`
      )
      break
    default:
      compared = i18n._(msg`${{ subject }}: ${{ aggregate }} ≥ ${{ value }}`)
  }
  if (condition.filters?.length) compared = i18n._(msg`${{ condition: compared }} (filtered)`)
  if (condition.negate) compared = i18n._(msg`Not (${{ condition: compared }})`)
  return withTimeframe(compared, describeTimeframe(condition.timeframe_operator, condition.timeframe_values, i18n))
}

/** The conditions under a node, however deeply grouped. */
function countConditions(node: TreeNode): number {
  if (node.kind === 'branch') return (node.branch?.leaves ?? []).reduce((sum, child) => sum + countConditions(child), 0)
  return 1
}

function describeNode(node: TreeNode, i18n: I18n, options: SegmentSummaryOptions): string {
  if (node.kind === 'branch') {
    const count = countConditions(node)
    return count === 1 ? i18n._(msg`1 condition`) : i18n._(msg`${{ count }} conditions`)
  }
  const leaf = node.leaf
  let described: string | null = null
  if (leaf?.source === 'contacts' && leaf.contact && leaf.contact.filters.length > 0) {
    const parts = leaf.contact.filters.map((filter) => describeFilter(filter, i18n, options))
    if (parts.every((part) => part !== null)) described = parts.join(` ${i18n._(msg`and`)} `)
  } else if (leaf?.source === 'contact_lists' && leaf.contact_list) {
    const list = options.listName?.(leaf.contact_list.list_id) ?? leaf.contact_list.list_id
    described =
      leaf.contact_list.operator === 'not_in' ? i18n._(msg`Not in list ${{ list }}`) : i18n._(msg`In list ${{ list }}`)
    const status = leaf.contact_list.status
    if (status) {
      const label = LIST_STATUS_LABEL[status] ? i18n._(LIST_STATUS_LABEL[status]) : status
      described = `${described} (${label})`
    }
  } else if (leaf?.source === 'contact_timeline' && leaf.contact_timeline) {
    described = describeTimeline(leaf.contact_timeline, i18n)
  } else if (leaf?.source === 'custom_events_goals' && leaf.custom_events_goal) {
    described = describeGoal(leaf.custom_events_goal, i18n)
  }
  return described ?? i18n._(msg`1 condition`)
}

/**
 * Describes the first level of a segment's tree: a leaf root is one condition, a branch root
 * lists its children. A nested group reads "N conditions"; a leaf it cannot describe (a JSON
 * filter, an unknown source) reads "1 condition".
 */
export function segmentSummary(tree: TreeNode, i18n: I18n, options: SegmentSummaryOptions = {}): SegmentSummary {
  const nodes = tree.kind === 'branch' && tree.branch ? tree.branch.leaves : [tree]
  const operator = tree.kind === 'branch' && tree.branch?.operator === 'or' ? 'or' : 'and'
  return {
    conditions: nodes.slice(0, MAX_CONDITIONS).map((node) => describeNode(node, i18n, options)),
    joiner: operator === 'or' ? i18n._(msg`or`) : i18n._(msg`and`),
    more: Math.max(0, nodes.length - MAX_CONDITIONS),
    hidden: nodes.slice(MAX_CONDITIONS).map((node) => describeNode(node, i18n, options))
  }
}
