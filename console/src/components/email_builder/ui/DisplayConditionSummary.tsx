import React, { useState } from 'react'
import { useLingui } from '@lingui/react/macro'
import { Button, Popconfirm, Tooltip } from 'antd'
import type { DisplayCondition, EmailBlock } from '../types'
import { evaluateDisplayCondition, holdsLiquidBlock, operatorTakesValue } from '../displayCondition'
import { ConditionResult, DisplayConditionModal } from './DisplayConditionModal'
import { useOperatorLabels } from './displayConditionLabels'
import { Pencil, Plus, Trash2 } from 'lucide-react'

// The rules as a sentence, as the segment editor reads its conditions: the variable and the
// value as tokens, the operator in words, an AND / OR pill between rules.
const ConditionRules: React.FC<{ condition: DisplayCondition }> = ({ condition }) => {
  const { t } = useLingui()
  const operatorLabels = useOperatorLabels()
  return (
    <div className="mt-1.5 flex flex-wrap items-center gap-x-1.5 gap-y-1 text-xs leading-6">
      <span className="text-ink-2">{t`Shown if`}</span>
      {condition.rules.map((rule, index) => (
        <React.Fragment key={index}>
          {index > 0 && (
            <span className="rounded-full border border-primary-300 bg-surface px-1.5 text-overline font-semibold tracking-wider text-primary-700 uppercase">
              {condition.match === 'any' ? t`or` : t`and`}
            </span>
          )}
          <span className="rounded-sm bg-info-bg px-1.5 font-mono text-2xs text-info-fg">{rule.variable}</span>
          <span className="text-ink-2">{operatorLabels[rule.operator] ?? rule.operator}</span>
          {operatorTakesValue(rule.operator) && (
            <span className="rounded-sm bg-primary-100 px-1.5 font-medium text-primary-700">{rule.value}</span>
          )}
        </React.Fragment>
      ))}
    </div>
  )
}

interface DisplayConditionSummaryProps {
  block: EmailBlock
  testData?: Record<string, unknown>
  onChange: (condition: DisplayCondition | undefined) => void
}

// The block's display condition at the top of its settings: a summary, its result with
// the test data, and the modal that edits it.
export const DisplayConditionSummary: React.FC<DisplayConditionSummaryProps> = ({
  block,
  testData,
  onChange
}) => {
  const { t } = useLingui()
  const [open, setOpen] = useState(false)
  const condition = block.display_condition
  const header = (
    <span className="text-overline font-semibold tracking-wider text-ink-3 uppercase">{t`Display condition`}</span>
  )
  const shown = evaluateDisplayCondition(condition, testData)
  // the server refuses a condition on a block holding an mj-liquid (see holdsLiquidBlock)
  const liquidInside = holdsLiquidBlock(block)
  const liquidMessage = t`This block holds a Liquid block, which a condition could cut off from the rest of the email. Move the Liquid block out of it first.`

  return (
    <div className="border-b border-line bg-surface px-3.5 py-2.5" data-testid="display-condition-summary">
      {condition ? (
        <>
          <div className="flex items-center gap-2">
            {header}
            <span className="ml-auto flex items-center gap-1">
              <Tooltip title={t`With the test data`}>
                <span>
                  <ConditionResult shown={shown} label={shown ? t`shown` : t`hidden`} />
                </span>
              </Tooltip>
              <Button size="small" type="text" aria-label={t`Edit`} icon={<Pencil size={13} aria-hidden="true" />} onClick={() => setOpen(true)} />
              <Popconfirm
                placement="left"
                title={t`Remove the display condition?`}
                description={t`The block will be sent to every recipient again.`}
                okText={t`Remove`}
                cancelText={t`Cancel`}
                onConfirm={() => onChange(undefined)}
              >
                <Button size="small" type="text" aria-label={t`Remove the condition`} icon={<Trash2 size={13} aria-hidden="true" />} />
              </Popconfirm>
            </span>
          </div>
          <ConditionRules condition={condition} />
          {liquidInside && (
            <div className="mt-1.5 rounded-md bg-danger-bg px-2.5 py-1.5 text-xs text-danger-fg" data-testid="display-condition-liquid">
              {liquidMessage}
            </div>
          )}
        </>
      ) : (
        <div className="flex items-center gap-2">
          {header}
          <span className="ml-auto flex items-center gap-2">
            <span className="text-xs text-ink-4">{t`Always shown`}</span>
            <Tooltip title={liquidInside ? liquidMessage : undefined}>
              <Button
                size="small"
                type="primary"
                ghost
                disabled={liquidInside}
                aria-label={t`Add a condition`}
                icon={<Plus size={13} aria-hidden="true" />}
                onClick={() => setOpen(true)}
              >
                {t`Condition`}
              </Button>
            </Tooltip>
          </span>
        </div>
      )}
      {open && (
        <DisplayConditionModal
          block={block}
          initial={condition}
          testData={testData}
          onCancel={() => setOpen(false)}
          onApply={(next) => {
            onChange(next)
            setOpen(false)
          }}
        />
      )}
    </div>
  )
}
