import {
  ContactListCondition,
  ContactTimelineCondition,
  CustomEventsGoalCondition,
  DimensionFilter,
  TreeNode,
  TreeNodeLeaf
} from '../../services/api/segment'

// Mirrors the server-side rules that decide whether a tree compiles into SQL: TreeNode.Validate
// in internal/domain/tree.go, plus the exact value counts the query builder requires for date
// operators. A condition open in the form goes through every intermediate state as the user
// fills it in, so the drawer asks this before auto-previewing rather than letting the backend
// reject half-filled conditions.

const FIELD_TYPES = ['string', 'number', 'time', 'json']
const JSON_FIELD_NAMES = [
  'custom_json_1',
  'custom_json_2',
  'custom_json_3',
  'custom_json_4',
  'custom_json_5'
]
const VALUELESS_OPERATORS = ['is_set', 'is_not_set']
const COUNT_OPERATORS = ['at_least', 'at_most', 'exactly']
const CONTACT_LIST_OPERATORS = ['in', 'not_in']
const GOAL_TYPES = ['*', 'purchase', 'subscription', 'lead', 'signup', 'booking', 'trial', 'other']
const AGGREGATE_OPERATORS = ['sum', 'count', 'avg', 'min', 'max']
const GOAL_COMPARISON_OPERATORS = ['gte', 'lte', 'eq', 'between']

const isFilled = (value: string | undefined | null): boolean => !!value && value.length > 0

/** Whether a dimension/property filter carries everything the query builder needs. */
const isFilterComplete = (filter: DimensionFilter | null | undefined): boolean => {
  if (!filter) return false
  if (!filter.field_name || !filter.operator) return false
  if (!FIELD_TYPES.includes(filter.field_type)) return false

  if (filter.json_path?.length) {
    if (!JSON_FIELD_NAMES.includes(filter.field_name)) return false
    if (filter.json_path.some((segment) => !isFilled(segment))) return false
  }

  if (filter.field_type === 'json') {
    if (!JSON_FIELD_NAMES.includes(filter.field_name)) return false
    // A json filter reads a value out of the document, so it needs a path to read — except
    // for existence checks, which apply to the column itself.
    if (!filter.json_path?.length && !VALUELESS_OPERATORS.includes(filter.operator)) return false
  }

  if (VALUELESS_OPERATORS.includes(filter.operator)) return true

  const stringValues = filter.string_values ?? []
  const numberValues = filter.number_values ?? []

  switch (filter.operator) {
    case 'in_array':
      return stringValues.length > 0
    case 'in_date_range':
    case 'not_in_date_range':
      // BETWEEN needs both bounds, and a range picker reports a single side mid-selection.
      return stringValues.length === 2 && stringValues.every(isFilled)
    case 'in_the_last_days':
    case 'not_in_the_last_days':
      return stringValues.length === 1 && isFilled(stringValues[0])
    default:
      break
  }

  if (filter.field_type === 'number') return numberValues.length > 0
  // An empty string cast to a timestamp is a Postgres error, not an empty match.
  if (filter.field_type === 'time') return stringValues.length > 0 && stringValues.every(isFilled)
  return stringValues.length > 0
}

const areFiltersComplete = (filters: DimensionFilter[] | undefined): boolean =>
  (filters ?? []).every(isFilterComplete)

/** Whether a timeframe carries the exact number of dates its operator consumes. */
const isTimeframeComplete = (
  operator: string | undefined,
  values: string[] | undefined
): boolean => {
  const timeframeValues = values ?? []

  switch (operator ?? 'anytime') {
    case 'anytime':
      return true
    case 'in_date_range':
      return timeframeValues.length === 2 && timeframeValues.every(isFilled)
    case 'before_date':
    case 'after_date':
    case 'in_the_last_days':
      return timeframeValues.length === 1 && isFilled(timeframeValues[0])
    default:
      return false
  }
}

const isContactListComplete = (condition: ContactListCondition | undefined): boolean => {
  if (!condition) return false
  if (!CONTACT_LIST_OPERATORS.includes(condition.operator)) return false
  return isFilled(condition.list_id)
}

const isContactTimelineComplete = (condition: ContactTimelineCondition | undefined): boolean => {
  if (!condition) return false
  if (!isFilled(condition.kind)) return false
  if (!COUNT_OPERATORS.includes(condition.count_operator)) return false
  if (typeof condition.count_value !== 'number' || condition.count_value < 0) return false
  if (!isTimeframeComplete(condition.timeframe_operator, condition.timeframe_values)) return false
  return areFiltersComplete(condition.filters)
}

const isCustomEventsGoalComplete = (condition: CustomEventsGoalCondition | undefined): boolean => {
  if (!condition) return false
  if (!GOAL_TYPES.includes(condition.goal_type)) return false
  if (!AGGREGATE_OPERATORS.includes(condition.aggregate_operator)) return false
  if (!GOAL_COMPARISON_OPERATORS.includes(condition.operator)) return false
  if (typeof condition.value !== 'number') return false
  if (condition.operator === 'between' && typeof condition.value_2 !== 'number') return false
  if (!isTimeframeComplete(condition.timeframe_operator, condition.timeframe_values)) return false
  return areFiltersComplete(condition.filters)
}

const isLeafComplete = (leaf: TreeNodeLeaf | undefined): boolean => {
  if (!leaf) return false

  switch (leaf.source) {
    case 'contacts':
      return (leaf.contact?.filters?.length ?? 0) > 0 && areFiltersComplete(leaf.contact?.filters)
    case 'contact_lists':
      return isContactListComplete(leaf.contact_list)
    case 'contact_timeline':
      return isContactTimelineComplete(leaf.contact_timeline)
    case 'custom_events_goals':
      return isCustomEventsGoalComplete(leaf.custom_events_goal)
    default:
      return false
  }
}

/**
 * Whether a tree can be sent to segments.preview without the backend rejecting it. Used to hold
 * back the auto-preview while a condition is half-filled, so the last known count stays on screen
 * instead of being replaced by an error.
 */
export const isTreeQueryable = (node: TreeNode | null | undefined): boolean => {
  if (!node) return false

  if (node.kind === 'branch') {
    if (!node.branch) return false
    if (node.branch.operator !== 'and' && node.branch.operator !== 'or') return false
    if (!node.branch.leaves?.length) return false
    return node.branch.leaves.every(isTreeQueryable)
  }

  if (node.kind === 'leaf') return isLeafComplete(node.leaf)

  return false
}

// Reports whether a tree carries any leaf at all. An editor hands back a branch with zero
// leaves for "nothing configured", which is not the same thing as no conditions: the server
// rejects an empty branch outright (TreeNodeBranch.Validate in internal/domain/tree.go).
export const HasLeaf = (node: TreeNode | null | undefined): boolean => {
  if (!node) return false
  if (node.kind === 'leaf') return Boolean(node.leaf)

  return (node.branch?.leaves ?? []).some((child: TreeNode) => HasLeaf(child))
}

// Reports whether any leaf in the tree reads from the given source, so a caller can show the
// guidance that applies to one source without guessing from the shape of the form.
export const treeUsesSource = (node: TreeNode | null | undefined, source: string): boolean => {
  if (!node) return false
  if (node.kind === 'leaf') return node.leaf?.source === source

  return (node.branch?.leaves ?? []).some((child: TreeNode) => treeUsesSource(child, source))
}

/**
 * Drops everything the server would reject, returning undefined when nothing survives.
 *
 * The tree editor commits a leaf the moment a source is picked, before any filter exists — so
 * a user who opens the editor, picks "Contact property" and then changes their mind leaves a
 * leaf behind that TreeNodeBranch.Validate refuses ("contact condition must have at least one
 * filter"). Left in place it blocks every later save of the automation, including edits made
 * since, and the panel meanwhile counts it as a configured condition.
 *
 * Identity is preserved when nothing is dropped, so a caller can tell a real change from a
 * no-op and avoid writing to its config for nothing.
 */
export const pruneIncompleteConditions = (node: TreeNode | null | undefined): TreeNode | undefined => {
  if (!node) return undefined

  if (node.kind === 'leaf') {
    return isLeafComplete(node.leaf) ? node : undefined
  }

  if (node.kind !== 'branch' || !node.branch) return undefined

  const original = node.branch.leaves ?? []
  const leaves = original
    .map(pruneIncompleteConditions)
    .filter((leaf): leaf is TreeNode => Boolean(leaf))

  if (!leaves.length) return undefined

  const unchanged =
    leaves.length === original.length && leaves.every((leaf, index) => leaf === original[index])

  return unchanged ? node : { ...node, branch: { ...node.branch, leaves } }
}
