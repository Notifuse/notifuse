import React, { useState } from 'react'
import { useLingui } from '@lingui/react/macro'
import { Button, Popconfirm, Tag, Tooltip, Typography } from 'antd'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import { faPlus } from '@fortawesome/free-solid-svg-icons'
import { faTrashCan } from '@fortawesome/free-regular-svg-icons'
import type { DisplayCondition, EmailBlock } from '../types'
import { evaluateDisplayCondition, operatorTakesValue } from '../displayCondition'
import { ConditionResult, DisplayConditionModal } from './DisplayConditionModal'
import { CONDITION_MONO, useOperatorLabels } from './displayConditionLabels'

const { Text } = Typography

const AND_OR_STYLE: React.CSSProperties = {
  display: 'inline-block',
  height: 20,
  lineHeight: '18px',
  backgroundColor: '#fafafa',
  border: '1px solid #CFD8DC',
  borderRadius: 3,
  fontSize: 12,
  fontWeight: 600,
  textAlign: 'center',
  minWidth: 42,
  padding: '0 6px'
}

// The rules as the segment editor reads its leaves: variable in bold, operator dimmed,
// value in a blue tag.
const ConditionRules: React.FC<{ condition: DisplayCondition }> = ({ condition }) => {
  const { t } = useLingui()
  const operatorLabels = useOperatorLabels()
  return (
    <div className="flex flex-col items-start gap-1.5">
      {condition.rules.map((rule, index) => (
        <React.Fragment key={index}>
          {index > 0 && (
            <span style={AND_OR_STYLE}>{condition.match === 'any' ? t`or` : t`and`}</span>
          )}
          <div className="flex flex-wrap items-center gap-x-1.5 gap-y-1">
            <b style={{ fontFamily: CONDITION_MONO, fontSize: 12 }}>{rule.variable}</b>
            <span className="opacity-60">{operatorLabels[rule.operator] ?? rule.operator}</span>
            {operatorTakesValue(rule.operator) && (
              <Tag variant="filled" color="blue" style={{ margin: 0 }}>
                {rule.value}
              </Tag>
            )}
          </div>
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
  const header = <div className="text-gray-900 text-sm font-bold">{t`Display condition`}</div>
  const shown = evaluateDisplayCondition(condition, testData)

  return (
    <div
      className="px-5 pt-4 pb-4 border-b border-gray-200"
      data-testid="display-condition-summary"
    >
      {condition ? (
        <>
          <div className="flex items-center justify-between">
            {header}
            <div className="flex items-center gap-1">
              <Popconfirm
                placement="left"
                title={t`Remove the display condition?`}
                description={t`The block will be sent to every recipient again.`}
                okText={t`Remove`}
                cancelText={t`Cancel`}
                onConfirm={() => onChange(undefined)}
              >
                <Tooltip title={t`Remove the condition`}>
                  <Button
                    size="small"
                    type="link"
                    aria-label={t`Remove the condition`}
                    icon={<FontAwesomeIcon icon={faTrashCan} />}
                  />
                </Tooltip>
              </Popconfirm>
              <Button size="small" type="primary" ghost onClick={() => setOpen(true)}>
                {t`Edit`}
              </Button>
            </div>
          </div>
          <div className="mt-2 text-xs text-slate-700 bg-[#fafafa] border border-[#CFD8DC] rounded-md px-2.5 py-2">
            <div className="opacity-60 mb-1.5">{t`Show this block if`}</div>
            <ConditionRules condition={condition} />
          </div>
          <div className="mt-1.5 text-xs text-slate-500">
            {t`With the test data:`}{' '}
            <ConditionResult shown={shown} label={shown ? t`shown` : t`hidden`} />
          </div>
        </>
      ) : (
        <>
          {header}
          <div className="flex items-center justify-between mt-2">
            <Text type="secondary" className="text-xs">
              {t`Always shown`}
            </Text>
            <Button
              size="small"
              type="dashed"
              icon={<FontAwesomeIcon icon={faPlus} />}
              onClick={() => setOpen(true)}
            >
              {t`Add a condition`}
            </Button>
          </div>
        </>
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
