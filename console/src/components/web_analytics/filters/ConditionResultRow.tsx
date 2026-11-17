import { Tooltip } from 'antd'
import { useLingui } from '@lingui/react/macro'
import type { WebFilterCondition } from '../../../services/api/web_analytics'
import { Mono, Pill } from '../../layout'
import { getSourceFieldLabel, useFilterVocabulary } from '../lib/filterCatalog'
import { isUntestedCondition, type RuleOutcome } from '../lib/filterEvaluator'

/** A rule's result for one visit: matches, no match, or a condition the browser cannot test. */
export function RuleOutcomePill({ outcome }: { outcome: RuleOutcome }) {
  const { t } = useLingui()
  if (outcome === 'matches') return <Pill tone="success">{t`Matches`}</Pill>
  if (outcome === 'untested') return <Pill tone="neutral">{t`Can't test in the browser`}</Pill>
  return <Pill tone="danger">{t`No match`}</Pill>
}

export interface ConditionResultRowProps {
  condition: WebFilterCondition
  /** The visit's value for the field the condition reads; '' or absent reads as empty. */
  visitValue?: string | null
  /** Whether the visit meets it; absent for a rule that does not run (disabled). */
  matches?: boolean
  /** The condition that makes the rule fail: the first one the visit does not meet. */
  emphasised?: boolean
}

/** A condition, the visit's value for the field it reads, and whether the visit meets it. */
export function ConditionResultRow({ condition, visitValue: raw, matches, emphasised }: ConditionResultRowProps) {
  const { t } = useLingui()
  const vocabulary = useFilterVocabulary()
  // Named as the message's placeholder: the catalogues read "This visit: {visitValue}".
  const visitValue = raw ?? ''
  // A regex only the server reads is skipped rather than reported as no match.
  const untested = matches !== undefined && isUntestedCondition(condition, { [condition.field]: visitValue })
  return (
    <div
      data-condition-result
      data-first-failing={emphasised ? 'true' : undefined}
      className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-0.5 text-sm"
    >
      {matches === undefined ? null : untested ? (
        <Tooltip title={t`This regular expression uses syntax the browser cannot run. The server checks it on save.`}>
          <span data-untested className="inline-flex">
            <Pill tone="neutral">{t`Not tested`}</Pill>
          </span>
        </Tooltip>
      ) : matches ? (
        <Pill tone="success">{t`Matches`}</Pill>
      ) : (
        <Pill tone="danger">{t`No match`}</Pill>
      )}
      <span className="inline-flex min-w-0 flex-wrap items-center gap-x-1.5 gap-y-0.5">
        <span className="text-ink-2">{getSourceFieldLabel(condition.field)}</span>
        <span className="text-ink-3">{vocabulary.operators[condition.operator]}</span>
        {condition.value ? <Mono>{condition.value}</Mono> : null}
      </span>
      <span className={`text-xs ${emphasised ? 'font-medium text-ink-2' : 'text-ink-3'}`}>
        {visitValue ? t`This visit: ${visitValue}` : t`This visit: empty`}
      </span>
    </div>
  )
}
