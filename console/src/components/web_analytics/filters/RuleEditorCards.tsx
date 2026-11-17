import { useState } from 'react'
import { Button } from 'antd'
import { FlaskConical } from 'lucide-react'
import { plural } from '@lingui/core/macro'
import { useLingui } from '@lingui/react/macro'
import type { WebFilter, WebFilterCondition, WebFilterOperation } from '../../../services/api/web_analytics'
import { KeyValueList, Mono, Panel, Pill } from '../../layout'
import { SeoChecks, type SeoCheck } from '../../seo/SeoChecks'
import { getDimensionLabel } from '../lib/dimensions'
import { getSourceFieldLabel } from '../lib/filterCatalog'
import {
  conditionResults,
  fieldsRead,
  firstFailingCondition,
  ruleOutcome,
  simulateOperations,
  toTestValues
} from '../lib/filterEvaluator'
import { describeBackfill } from '../lib/backfill'
import { regexProblems, ruleDraftProblems, type FilterDraft } from '../lib/ruleDraft'
import { ruleNeighbours, ruleOverrides } from '../lib/ruleOrder'
import { ConditionResultRow, RuleOutcomePill } from './ConditionResultRow'
import { useBackfillStatus } from './useBackfillStatus'
import { VisitFieldInput } from './VisitFieldInput'

/** "Channel and Channel Group", "Channel or Channel Group", in the console's language. */
function useListFormat() {
  const { i18n } = useLingui()
  return (items: string[], type: 'conjunction' | 'disjunction') =>
    new Intl.ListFormat(i18n.locale, { style: 'long', type }).format(items)
}

const uniqueDimensions = (operations: WebFilterOperation[]) => [
  ...new Set((operations ?? []).map((operation) => operation?.dimension).filter(Boolean))
]

interface RuleContextProps {
  rules: WebFilter[]
  draft: FilterDraft
  editingId?: string
  customDimensionLabels?: Record<string, string>
}

/** The rules that write the same dimensions, in the order the engine runs them. */
export function RuleOrderCard({ rules, draft, editingId, customDimensionLabels }: RuleContextProps) {
  const { t } = useLingui()
  const list = useListFormat()
  const [showAll, setShowAll] = useState(false)
  const order = ruleNeighbours(rules, draft, editingId)
  const dimensions = uniqueDimensions(draft.operations).map((dimension) =>
    getDimensionLabel(dimension, customDimensionLabels)
  )
  const dimensionList = list(dimensions, 'disjunction')
  const extra =
    dimensions.length > 0 ? <span className="text-xs text-ink-3">{t`Rules that also write ${dimensionList}`}</span> : null

  // A cleared priority places the rule nowhere: nothing is sorted until one is typed.
  if (!order) {
    return (
      <Panel title={t`Rule order`} extra={extra}>
        <p className="m-0 text-sm text-ink-3">{t`Set a priority to place this rule among the others.`}</p>
      </Panel>
    )
  }
  const { above, below } = order
  const shown = showAll ? order.all : order.rows

  return (
    <Panel
      title={t`Rule order`}
      extra={extra}
      flush
      footer={
        <>
          <span data-rule-order-counts>
            {above + below === 0
              ? t`No other rule writes these dimensions`
              : plural(above, {
                  one: `# rule above and ${below} below this one`,
                  other: `# rules above and ${below} below this one`
                })}
          </span>
          {order.all.length > order.rows.length ? (
            <Button type="link" size="small" onClick={() => setShowAll((all) => !all)}>
              {showAll ? t`Show fewer rules` : t`Show all rules`}
            </Button>
          ) : null}
        </>
      }
    >
      <div className="overflow-x-auto">
        <table className="w-full border-collapse text-sm">
          <thead>
            <tr className="text-left text-xs text-ink-3">
              <th className="px-3.5 py-2 font-medium">{t`Priority`}</th>
              <th className="px-3.5 py-2 font-medium">{t`Rule`}</th>
              <th className="px-3.5 py-2 font-medium">{t`Reads`}</th>
              <th className="px-3.5 py-2 font-medium">{t`Writes`}</th>
            </tr>
          </thead>
          <tbody>
            {shown.map((rule) => {
              const isThis = rule.id === order.thisId
              const reads = fieldsRead(rule.conditions ?? []).map(getSourceFieldLabel)
              const writes = uniqueDimensions(rule.operations).map((dimension) =>
                getDimensionLabel(dimension, customDimensionLabels)
              )
              return (
                <tr
                  key={rule.id}
                  data-rule-order-row={isThis ? 'this' : rule.id}
                  className={`border-t border-line-soft align-top ${isThis ? 'bg-primary-50' : ''}`}
                >
                  <td className={`px-3.5 py-2 tabular-nums ${isThis ? 'font-semibold text-ink' : 'text-ink-2'}`}>
                    {rule.priority}
                  </td>
                  <td className="px-3.5 py-2">
                    <span className="inline-flex flex-wrap items-center gap-2">
                      <span className={isThis ? 'font-semibold text-ink' : 'text-ink'}>
                        {isThis ? draft.name?.trim() || t`New rule` : rule.name}
                      </span>
                      {isThis ? <Pill tone="primary" dot={false}>{t`This rule`}</Pill> : null}
                      {!rule.enabled ? <Pill tone="neutral" dot={false}>{t`Disabled`}</Pill> : null}
                    </span>
                  </td>
                  <td className="px-3.5 py-2 text-ink-2">{reads.length > 0 ? reads.join(', ') : t`Always matches`}</td>
                  <td className="px-3.5 py-2 text-ink-2">{writes.join(', ')}</td>
                </tr>
              )
            })}
          </tbody>
        </table>
      </div>
    </Panel>
  )
}

export interface RuleChecksProps extends RuleContextProps {
  /** The validators' messages, by problem. */
  messages: { missing_value: string; none: string }
  onShowField: (path: string[]) => void
}

/** What the server will say on save, and what the rule's priority means, as the rule is typed. */
export function RuleChecks({ rules, draft, editingId, customDimensionLabels, messages, onShowField }: RuleChecksProps) {
  const { t } = useLingui()
  const list = useListFormat()
  const conditions = (draft.conditions ?? []) as WebFilterCondition[]
  const problems = ruleDraftProblems(draft)
  const checks: SeoCheck[] = []

  const operationsMissing = problems.operations ? messages[problems.operations] : null
  checks.push({
    key: 'values',
    ok: !problems.conditions && !problems.operations,
    label: problems.conditions
      ? messages[problems.conditions]
      : (operationsMissing ?? t`Every condition and every operation has a value`)
  })

  if (conditions.length === 0) {
    checks.push({
      key: 'always',
      ok: false,
      tone: 'info',
      label: t`Always matches`,
      why: t`With no condition, this rule runs on every session.`
    })
  }

  const regexCount = conditions.filter((condition) => condition?.operator === 'regex').length
  if (regexCount > 0) {
    const [problem] = regexProblems(conditions)
    const position = problem ? problem.index + 1 : 0
    checks.push({
      key: 'regex',
      ok: !problem,
      // Syntax only the server reads is not wrong: worth knowing, not to fix.
      tone: problem?.kind === 'untestable' ? 'info' : undefined,
      label: !problem
        ? plural(regexCount, {
            one: 'The regular expression is valid',
            other: 'The # regular expressions are valid'
          })
        : problem.kind === 'refused'
          ? t`Condition ${position} uses a lookaround or a backreference, which the server refuses`
          : problem.kind === 'untestable'
            ? t`Condition ${position} can't be tested in the browser; the server checks it on save`
            : t`Condition ${position} is not a valid regular expression and may be refused on save`,
      why:
        problem?.kind === 'untestable'
          ? t`It uses syntax only the server's regular expressions read, so the Quick test and the tester skip it.`
          : t`Checked as you type; the server checks again on save.`
    })
  }

  const overrides = ruleOverrides(rules, draft, editingId)
  const dimensionList = (dimensions: string[]) =>
    list(
      dimensions.map((dimension) => getDimensionLabel(dimension, customDimensionLabels)),
      'conjunction'
    )
  if (overrides.kind === 'disabled') {
    checks.push({
      key: 'priority',
      ok: false,
      tone: 'info',
      label: t`This rule is disabled: it does not run, so it writes nothing`,
      why: t`Enable it to see which rules would override it.`
    })
  } else if (overrides.kind === 'no_priority') {
    checks.push({
      key: 'priority',
      ok: false,
      tone: 'info',
      label: t`Set a priority to see which rules override this one`
    })
  } else if (overrides.before.length === 0 && overrides.after.length === 0) {
    checks.push({ key: 'priority', ok: true, label: t`No other rule overrides what this rule writes` })
  } else {
    if (overrides.before.length > 0) {
      const beforeList = dimensionList(overrides.beforeDimensions)
      checks.push({
        key: 'priority-before',
        ok: false,
        tone: 'info',
        label: plural(overrides.before.length, {
          one: `# rule runs first and can keep this rule from writing ${beforeList}`,
          other: `# rules run first and can keep this rule from writing ${beforeList}`
        }),
        why: t`When one of them also matches a session, this rule's value is not written.`
      })
    }
    if (overrides.after.length > 0) {
      const afterList = dimensionList(overrides.afterDimensions)
      checks.push({
        key: 'priority-after',
        ok: false,
        tone: 'info',
        label: plural(overrides.after.length, {
          one: `# rule runs later and can overwrite this rule on ${afterList}`,
          other: `# rules run later and can overwrite this rule on ${afterList}`
        }),
        why: t`When one of them also matches a session, its value replaces this rule's.`
      })
    }
  }

  return <SeoChecks title={t`Checks`} checks={checks} onShowField={onShowField} />
}

export interface QuickTestCardProps {
  conditions: WebFilterCondition[]
  operations: WebFilterOperation[]
  /** The visit typed so far, by field. */
  values: Record<string, string>
  onChange: (values: Record<string, string>) => void
  customDimensionLabels?: Record<string, string>
  onOpenTester: () => void
}

/** The fields this rule reads, for one visit, evaluated in the browser. */
export function QuickTestCard({
  conditions,
  operations,
  values,
  onChange,
  customDimensionLabels,
  onOpenTester
}: QuickTestCardProps) {
  const { t } = useLingui()
  const fields = fieldsRead(conditions)
  const visit = toTestValues(values)
  // An untested rule writes nothing: whether it matches is the server's to say.
  const outcome = ruleOutcome(conditions, visit)
  const matches = outcome === 'matches'
  const outcomes = conditionResults(conditions, visit)
  const failing = firstFailingCondition(conditions, visit)
  const writes = simulateOperations(operations, matches)

  return (
    <Panel
      title={t`Quick test`}
      extra={<RuleOutcomePill outcome={outcome} />}
      footer={
        <>
          <span>{t`Every field of the visit`}</span>
          <Button size="small" icon={<FlaskConical size={14} aria-hidden="true" />} onClick={onOpenTester}>
            {t`Open tester`}
          </Button>
        </>
      }
    >
      <div data-quick-test className="flex flex-col gap-3">
        <p className="m-0 text-xs text-ink-3">{t`The fields this rule reads, for one visit. Nothing is saved or sent.`}</p>
        {fields.length === 0 ? (
          <p className="m-0 text-sm text-ink-3">{t`With no condition, this rule matches every visit.`}</p>
        ) : (
          fields.map((field) => (
            <div key={field} className="flex flex-col gap-1">
              <label htmlFor={`quick-test-${field}`} className="text-sm text-ink">
                {getSourceFieldLabel(field)} <span className="font-mono text-xs text-ink-4">{field}</span>
              </label>
              <VisitFieldInput
                id={`quick-test-${field}`}
                field={field}
                value={values[field] ?? ''}
                placeholder={t`Empty`}
                onChange={(value) => onChange({ ...values, [field]: value })}
              />
            </div>
          ))
        )}
        {conditions.length > 0 ? (
          <div className="flex flex-col gap-1.5 border-t border-line-soft pt-3">
            {conditions.map((condition, index) => (
              <ConditionResultRow
                key={index}
                condition={condition}
                visitValue={visit[condition?.field]}
                matches={outcomes[index]}
                emphasised={failing === index}
              />
            ))}
          </div>
        ) : null}
        {matches && writes.length > 0 ? (
          <div className="flex flex-col gap-1.5 border-t border-line-soft pt-3">
            <span className="text-xs text-ink-3">{t`Values written`}</span>
            <KeyValueList
              labelWidth={120}
              items={writes.map((operation, index) => ({
                key: String(index),
                label: getDimensionLabel(operation.dimension, customDimensionLabels),
                value:
                  operation.resultValue === null ? (
                    <span className="text-ink-4">null</span>
                  ) : (
                    <Mono>{operation.resultValue}</Mono>
                  )
              }))}
            />
          </div>
        ) : null}
      </div>
    </Panel>
  )
}

/** When a saved rule takes effect, and the last backfill that applied rules to past sessions. */
export function WhenItAppliesCard({ workspaceId }: { workspaceId: string }) {
  const { t, i18n } = useLingui()
  const { data: status, isLoading } = useBackfillStatus(workspaceId)
  const backfill = describeBackfill(status)

  let lastBackfill = null
  if (isLoading) {
    lastBackfill = <span className="text-ink-4">…</span>
  } else if (backfill.state === 'never') {
    lastBackfill = <span className="text-ink-2">{i18n._(backfill.label)}</span>
  } else if (backfill.state === 'failed') {
    lastBackfill = (
      <span className="flex flex-col items-start gap-0.5">
        <Pill tone={backfill.tone}>{i18n._(backfill.label)}</Pill>
        <span className="text-xs text-ink-3">{status?.error_message || t`No error message was recorded.`}</span>
      </span>
    )
  } else {
    lastBackfill = (
      <Pill tone={backfill.tone} pulse={backfill.state === 'active'}>
        {i18n._(backfill.label)}
      </Pill>
    )
  }

  return (
    <Panel title={t`When it applies`}>
      <KeyValueList
        labelWidth={110}
        items={[
          { key: 'new', label: t`New sessions`, value: t`From the moment you save` },
          { key: 'past', label: t`Past sessions`, value: t`After a backfill, run from Attribution rules` },
          { key: 'last', label: t`Last backfill`, value: lastBackfill }
        ]}
      />
    </Panel>
  )
}
