import type { WebFilter, WebFilterOperation } from '../../../services/api/web_analytics'
import type { FilterDraft } from './ruleDraft'

/** The id the rule being created takes until it is saved. */
export const DRAFT_RULE_ID = '__draft__'

/**
 * The rules as they would be saved: the edited rule replaced in place, or the new one appended,
 * as the rules tab writes them.
 */
export function wouldBeSaved(rules: WebFilter[], draft: FilterDraft, editingId?: string): WebFilter[] {
  if (editingId && rules.some((rule) => rule.id === editingId)) {
    return rules.map((rule) => (rule.id === editingId ? { ...rule, ...draft } : rule))
  }
  const order = rules.reduce((highest, rule) => Math.max(highest, rule.order), 0) + 1
  return [...rules, { id: DRAFT_RULE_ID, order, ...draft }]
}

const writtenDimensions = (rule: Pick<WebFilter, 'operations'>) =>
  new Set((rule.operations ?? []).map((operation) => operation?.dimension).filter(Boolean))

/** Whether the draft has a priority to place it by: a cleared field reads as null. */
export const hasPriority = (draft: Pick<FilterDraft, 'priority'>): boolean =>
  typeof draft.priority === 'number' && Number.isFinite(draft.priority)

/**
 * The rules in the order the engine runs them: highest priority first, equal priorities in list
 * order (a stable sort, as sort.SliceStable in internal/domain/web_analytics_filters.go).
 */
const engineOrder = (rules: WebFilter[]) => [...rules].sort((a, b) => b.priority - a.priority)

export interface RuleNeighbours {
  /** The window around this rule, in the order the engine runs them. */
  rows: WebFilter[]
  /** Every rule sharing a written dimension, this one included, in engine order. */
  all: WebFilter[]
  /** The id of this rule in `rows` and `all`. */
  thisId: string
  /** How many of `all` run before and after this one. */
  above: number
  below: number
}

/**
 * The rules that write one of the dimensions this rule writes, in the order the engine runs them,
 * with `window` rules either side of this one. Null while the draft has no priority: it has no
 * place in the order yet.
 */
export function ruleNeighbours(
  rules: WebFilter[],
  draft: FilterDraft,
  editingId?: string,
  window = 2
): RuleNeighbours | null {
  if (!hasPriority(draft)) return null
  const saved = wouldBeSaved(rules, draft, editingId)
  const thisId = editingId && rules.some((rule) => rule.id === editingId) ? editingId : DRAFT_RULE_ID
  const dimensions = writtenDimensions(draft)
  const all = engineOrder(saved).filter(
    (rule) => rule.id === thisId || [...writtenDimensions(rule)].some((dimension) => dimensions.has(dimension))
  )
  const index = all.findIndex((rule) => rule.id === thisId)
  return {
    rows: all.slice(Math.max(0, index - window), index + window + 1),
    all,
    thisId,
    above: index,
    below: all.length - index - 1
  }
}

type Action = WebFilterOperation['action']

const claimsPriority = (action: Action) => action === 'set_value' || action === 'unset_value'

/**
 * How another rule's operation on a dimension changes what this rule's operation leaves there, when
 * both rules match a session. Derived from EvaluateWebFilters: a set or unset applies unless the
 * dimension was claimed by a strictly higher priority, and claims it; a default writes only when
 * the dimension is still unset or null, and claims nothing.
 *
 * - `before`: the other rule runs first and keeps this one's value from being written.
 * - `after`: the other rule runs later and replaces this one's value.
 * - null: this rule's value stands (an unset blocked by another unset leaves the same null).
 */
function overrideBetween(
  mine: Action,
  theirs: Action,
  runsBefore: boolean,
  samePriority: boolean
): 'before' | 'after' | null {
  if (claimsPriority(mine)) {
    // A set or unset that runs first claims the dimension: this one applies only at the same
    // priority. One that runs later applies only at the same priority (it cannot be higher).
    if (claimsPriority(theirs)) {
      if (mine === 'unset_value' && theirs === 'unset_value') return null
      if (runsBefore) return samePriority ? null : 'before'
      return samePriority ? 'after' : null
    }
    // A default run first is overwritten by this set or unset. Run later, it fills only what this
    // rule left null: an unset.
    return !runsBefore && mine === 'unset_value' ? 'after' : null
  }
  // This rule's default: anything run first that leaves the dimension non-null pre-empts it (a set
  // or another default, not an unset); any set or unset run later replaces it.
  if (runsBefore) return theirs === 'unset_value' ? null : 'before'
  return claimsPriority(theirs) ? 'after' : null
}

export interface RuleOverrides {
  /** Enabled rules that run first and keep this rule's value from being written, in engine order. */
  before: WebFilter[]
  /** Enabled rules that run later and replace this rule's value, in engine order. */
  after: WebFilter[]
  /** The dimensions each group takes, in this rule's operation order. */
  beforeDimensions: string[]
  afterDimensions: string[]
}

/**
 * - `disabled`: the draft is disabled; the engine skips it, so nothing overrides what it never writes.
 * - `no_priority`: the priority is cleared; nothing can be placed until it is set.
 */
export type RuleOverridesResult = { kind: 'disabled' } | { kind: 'no_priority' } | ({ kind: 'runs' } & RuleOverrides)

/**
 * The enabled rules that change what this rule writes when both match a session, split by whether
 * they run before it (and keep its value from being written) or after it (and replace it).
 */
export function ruleOverrides(rules: WebFilter[], draft: FilterDraft, editingId?: string): RuleOverridesResult {
  if (!draft.enabled) return { kind: 'disabled' }
  if (!hasPriority(draft)) return { kind: 'no_priority' }
  const saved = wouldBeSaved(rules, draft, editingId)
  const thisId = editingId && rules.some((rule) => rule.id === editingId) ? editingId : DRAFT_RULE_ID
  const ordered = engineOrder(saved)
  const position = new Map(ordered.map((rule, index) => [rule.id, index]))
  const thisPosition = position.get(thisId) ?? ordered.length
  const groups = { before: new Set<string>(), after: new Set<string>() }
  const dimensions = { before: [] as string[], after: [] as string[] }

  for (const mine of draft.operations ?? []) {
    if (!mine?.dimension || !mine.action) continue
    for (const rule of ordered) {
      if (rule.id === thisId || !rule.enabled) continue
      const runsBefore = (position.get(rule.id) ?? 0) < thisPosition
      for (const theirs of rule.operations ?? []) {
        if (theirs?.dimension !== mine.dimension) continue
        const kind = overrideBetween(mine.action, theirs.action, runsBefore, rule.priority === draft.priority)
        if (!kind) continue
        groups[kind].add(rule.id)
        if (!dimensions[kind].includes(mine.dimension)) dimensions[kind].push(mine.dimension)
      }
    }
  }

  return {
    kind: 'runs',
    before: ordered.filter((rule) => groups.before.has(rule.id)),
    after: ordered.filter((rule) => groups.after.has(rule.id)),
    beforeDimensions: dimensions.before,
    afterDimensions: dimensions.after
  }
}
