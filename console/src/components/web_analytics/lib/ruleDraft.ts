import type {
  WebFilter,
  WebFilterCondition,
  WebFilterOperation
} from '../../../services/api/web_analytics'
import { isValuelessRuleOperator } from './filterCatalog'
import { readRuleRegex } from './filterEvaluator'

/** Everything the rule editor owns; ids, order and timestamps stay with the caller. */
export interface FilterDraft {
  name: string
  priority: number
  tags: string[]
  enabled: boolean
  conditions: WebFilterCondition[]
  operations: WebFilterOperation[]
}

/** What the editor starts from: the rule as saved, or a new rule's defaults. */
export function draftOf(filter?: WebFilter): FilterDraft {
  if (filter) {
    return {
      name: filter.name,
      priority: filter.priority,
      tags: filter.tags ?? [],
      enabled: filter.enabled,
      conditions: filter.conditions ?? [],
      operations: filter.operations ?? []
    }
  }
  return {
    name: '',
    priority: 500,
    tags: [],
    enabled: true,
    conditions: [{ field: 'utm_source', operator: 'equals', value: '' }],
    operations: [{ dimension: 'channel', action: 'set_value', value: '' }]
  }
}

/**
 * A draft as the engine reads it: a trimmed name, no tags as [], a cleared priority as null, and
 * no value where the operator or action takes none (the condition rows clear it to undefined, the
 * operation rows to '').
 */
function normalised(draft: Partial<FilterDraft>) {
  return {
    name: (draft.name ?? '').trim(),
    priority: draft.priority ?? null,
    tags: draft.tags ?? [],
    enabled: Boolean(draft.enabled),
    conditions: (draft.conditions ?? []).map((condition) => ({
      field: condition?.field ?? '',
      operator: condition?.operator ?? '',
      value: isValuelessRuleOperator(condition?.operator) ? '' : (condition?.value ?? '')
    })),
    operations: (draft.operations ?? []).map((operation) => ({
      dimension: operation?.dimension ?? '',
      action: operation?.action ?? '',
      value: operation?.action === 'unset_value' ? '' : (operation?.value ?? '')
    }))
  }
}

/** Whether two drafts would save the same rule. */
export function sameRuleDraft(a: Partial<FilterDraft>, b: Partial<FilterDraft>): boolean {
  return JSON.stringify(normalised(a)) === JSON.stringify(normalised(b))
}

export type ConditionsProblem = 'missing_value'
export type OperationsProblem = 'none' | 'missing_value'

/**
 * What keeps a draft from being saved, shared by the validators and the Checks card. A rule may
 * have no condition (it then always matches, as the server allows); it needs one operation.
 */
export function ruleDraftProblems(draft: Partial<FilterDraft>): {
  conditions: ConditionsProblem | null
  operations: OperationsProblem | null
} {
  const conditions = draft.conditions ?? []
  const operations = draft.operations ?? []
  const conditionMissing = conditions.some(
    (condition) => condition && !isValuelessRuleOperator(condition.operator) && !condition.value?.trim()
  )
  const operationMissing = operations.some(
    (operation) =>
      operation &&
      (operation.action === 'set_value' || operation.action === 'set_default_value') &&
      !operation.value?.trim()
  )
  return {
    conditions: conditionMissing ? 'missing_value' : null,
    operations: operations.length === 0 ? 'none' : operationMissing ? 'missing_value' : null
  }
}

/**
 * - `refused`: a lookaround or a backreference, which Go's RE2 does not support, so the server
 *   refuses the rule.
 * - `invalid`: JavaScript cannot compile the pattern; the server may refuse it too.
 * - `untestable`: RE2 syntax JavaScript cannot express; the server checks it on save.
 */
export type RegexProblemKind = 'refused' | 'invalid' | 'untestable'

export interface RegexProblem {
  /** The condition's position in the rule. */
  index: number
  kind: RegexProblemKind
}

const SEVERITY: RegexProblemKind[] = ['refused', 'invalid', 'untestable']

/**
 * The regex conditions the server would refuse, may refuse, or the browser cannot check, the most
 * serious first. Checked in the browser as a warning: the server compiles with RE2 on save, and the
 * backfill runs the same patterns in PostgreSQL, so neither dialect is JavaScript's.
 */
export function regexProblems(conditions: WebFilterCondition[]): RegexProblem[] {
  const problems: RegexProblem[] = []
  conditions.forEach((condition, index) => {
    if (condition?.operator !== 'regex' || !condition.value) return
    const { kind } = readRuleRegex(condition.value)
    if (kind !== 'ok') problems.push({ index, kind })
  })
  return problems.sort((a, b) => SEVERITY.indexOf(a.kind) - SEVERITY.indexOf(b.kind) || a.index - b.index)
}
