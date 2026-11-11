import React, { useMemo, useState } from 'react'
import { useLingui } from '@lingui/react/macro'
import { AutoComplete, Button, Input, Modal, Select, Space, Tag, Tooltip, Typography } from 'antd'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import {
  faCircleCheck,
  faCircleXmark,
  faCode,
  faPlus,
  faTriangleExclamation,
  faXmark
} from '@fortawesome/free-solid-svg-icons'
import { faPenToSquare, faTrashCan } from '@fortawesome/free-regular-svg-icons'
import type {
  DisplayCondition,
  DisplayConditionOperator,
  DisplayConditionRule,
  EmailBlock
} from '../types'
import { EmailBlockClass } from '../EmailBlockClass'
import {
  DISPLAY_CONDITION_OPERATORS,
  MAX_DISPLAY_CONDITION_RULES,
  collectVariableSuggestions,
  evaluateDisplayCondition,
  evaluateRule,
  operatorTakesValue,
  resolveVariable,
  trimGoSpace,
  validateRule,
  type DisplayConditionRuleError
} from '../displayCondition'
import { CONDITION_MONO, useOperatorLabels } from './displayConditionLabels'

const { Text } = Typography

const LINE_COLOR = '#03A9F4'
const SEPARATOR = '#CFD8DC'
const ANDOR_BOX: React.CSSProperties = {
  position: 'absolute',
  left: -55,
  top: 16,
  height: 24,
  lineHeight: '24px',
  backgroundColor: '#fafafa',
  border: `1px solid ${SEPARATOR}`,
  borderRadius: 3,
  fontSize: 12,
  fontWeight: 600,
  textAlign: 'center',
  width: 42
}

export const ConditionResult: React.FC<{ shown: boolean; label: string }> = ({ shown, label }) => (
  <span className={`inline-flex items-center gap-1 ${shown ? 'text-green-700' : 'text-gray-500'}`}>
    <FontAwesomeIcon icon={shown ? faCircleCheck : faCircleXmark} />
    {label}
  </span>
)

const VariableTag: React.FC = () => {
  const { t } = useLingui()
  return (
    <Tag variant="filled" color="geekblue" style={{ margin: 0 }}>
      <FontAwesomeIcon icon={faCode} style={{ marginRight: 8 }} />
      {t`Variable`}
    </Tag>
  )
}

interface DisplayConditionModalProps {
  block: EmailBlock
  initial?: DisplayCondition
  testData?: Record<string, unknown>
  onCancel: () => void
  onApply: (condition: DisplayCondition | undefined) => void
}

export const DisplayConditionModal: React.FC<DisplayConditionModalProps> = ({
  block,
  initial,
  testData,
  onCancel,
  onApply
}) => {
  const { t } = useLingui()
  const operatorLabels = useOperatorLabels()
  const [condition, setCondition] = useState<DisplayCondition>(
    () => initial ?? { match: 'all', rules: [] }
  )
  // The rule being edited and its draft; a new rule is added only on Confirm
  const [editing, setEditing] = useState<{ index: number; rule: DisplayConditionRule } | null>(null)
  const suggestions = useMemo(() => collectVariableSuggestions(testData), [testData])

  const ruleErrorText = (error: DisplayConditionRuleError): string => {
    switch (error) {
      case 'invalid_variable':
        return t`Enter a variable path such as order.total: letters, digits and underscores, separated by dots.`
      case 'unsupported_lookup':
        return t`A condition cannot use .empty or .to_liquid: use the operator "is false or empty".`
      case 'value_required':
        return t`Enter a value.`
      case 'value_too_long':
        return t`The value is too long.`
      case 'value_has_liquid': {
        const delimiters = '{{ }} {% %}'
        return t`The value cannot contain the Liquid delimiters ${delimiters}.`
      }
      case 'value_not_number':
        return t`Enter a number, with at most 15 digits before the decimal point.`
      default:
        return t`This rule is not valid.`
    }
  }

  const andOr = condition.match === 'all' ? t`and` : t`or`
  const shown = evaluateDisplayCondition(condition, testData)
  // the rule as Confirm saves it, so the error shown is the one Confirm would meet
  const draft: DisplayConditionRule | null = editing
    ? {
        ...editing.rule,
        variable: editing.rule.variable.trim(),
        ...(editing.rule.value !== undefined ? { value: trimGoSpace(editing.rule.value) } : {})
      }
    : null
  const draftError = draft ? validateRule(draft) : null

  const removeRule = (index: number) =>
    setCondition({ ...condition, rules: condition.rules.filter((_, j) => j !== index) })

  const confirmEdit = () => {
    if (!editing || !draft || draftError) return
    const rules = [...condition.rules]
    rules[editing.index] = draft
    setCondition({ ...condition, rules })
    setEditing(null)
  }

  const setDraft = (rule: DisplayConditionRule) => editing && setEditing({ ...editing, rule })

  const readRow = (rule: DisplayConditionRule, index: number) => {
    const holds = evaluateRule(rule, testData)
    const missing = resolveVariable(testData, rule.variable) === undefined
    return (
      <div className="py-4" data-testid={`condition-rule-${index}`}>
        <div className="float-right flex items-center gap-3">
          <Tooltip title={t`Result with the test data`}>
            <span className="text-xs">
              <ConditionResult shown={holds} label={holds ? t`true` : t`false`} />
            </span>
          </Tooltip>
          <Space.Compact>
            <Button
              size="small"
              aria-label={t`Delete rule`}
              disabled={editing !== null}
              onClick={() => removeRule(index)}
            >
              <FontAwesomeIcon icon={faTrashCan} />
            </Button>
            <Button
              size="small"
              aria-label={t`Edit rule`}
              disabled={editing !== null}
              onClick={() => setEditing({ index, rule })}
            >
              <FontAwesomeIcon icon={faPenToSquare} />
            </Button>
          </Space.Compact>
        </div>
        <Space style={{ alignItems: 'center' }} wrap>
          <VariableTag />
          <b style={{ fontFamily: CONDITION_MONO, fontSize: 13 }}>{rule.variable}</b>
          <span className="opacity-60">{operatorLabels[rule.operator] ?? rule.operator}</span>
          {operatorTakesValue(rule.operator) && (
            <Tag variant="filled" color="blue" style={{ margin: 0 }}>
              {rule.value}
            </Tag>
          )}
        </Space>
        {missing && (
          <div className="text-[11px] text-amber-600 mt-2 flex items-center gap-1">
            <FontAwesomeIcon icon={faTriangleExclamation} />
            {t`Not in the test data: the rule is evaluated as if the variable were empty.`}
          </div>
        )}
      </div>
    )
  }

  const editRow = (rule: DisplayConditionRule, index: number) => (
    <div className="py-4" data-testid={`condition-rule-${index}`}>
      <div className="float-right flex items-center gap-2">
        <Button
          size="small"
          type="text"
          aria-label={t`Cancel the rule changes`}
          onClick={() => setEditing(null)}
        >
          <FontAwesomeIcon icon={faXmark} />
        </Button>
        <Button size="small" type="primary" disabled={draftError !== null} onClick={confirmEdit}>
          {t`Confirm`}
        </Button>
      </div>
      <Space style={{ alignItems: 'center' }} wrap>
        <VariableTag />
        <AutoComplete
          size="small"
          aria-label={t`Variable`}
          value={rule.variable}
          onChange={(variable: string) => setDraft({ ...rule, variable })}
          placeholder={t`e.g. order.has_image`}
          style={{ width: 210 }}
          popupMatchSelectWidth={340}
          options={suggestions.map((s) => ({
            value: s.variable,
            label: (
              <span className="flex items-center justify-between gap-3">
                <span style={{ fontFamily: CONDITION_MONO }} className="text-xs">
                  {s.variable}
                </span>
                <Text type="secondary" className="text-xs truncate max-w-[120px]">
                  {s.sample}
                </Text>
              </span>
            )
          }))}
          filterOption={(input, option) => String(option?.value ?? '').includes(input)}
        />
        <Select
          size="small"
          aria-label={t`Operator`}
          style={{ width: 210 }}
          value={rule.operator}
          onChange={(operator: DisplayConditionOperator) =>
            setDraft(
              operatorTakesValue(operator)
                ? { variable: rule.variable, operator, value: rule.value ?? '' }
                : { variable: rule.variable, operator }
            )
          }
          options={DISPLAY_CONDITION_OPERATORS.map((op) => ({
            value: op,
            label: operatorLabels[op]
          }))}
        />
        {operatorTakesValue(rule.operator) && (
          <Input
            size="small"
            aria-label={t`Value`}
            style={{ width: 140 }}
            value={rule.value ?? ''}
            onChange={(e) => setDraft({ ...rule, value: e.target.value })}
            onPressEnter={confirmEdit}
          />
        )}
      </Space>
      {draftError && rule.variable !== '' && (
        <div className="text-[11px] text-red-600 mt-2">{ruleErrorText(draftError)}</div>
      )}
    </div>
  )

  const rows =
    editing && editing.index === condition.rules.length
      ? [...condition.rules, editing.rule]
      : condition.rules

  return (
    <Modal
      open
      width={860}
      onCancel={onCancel}
      title={
        <div className="flex items-center gap-2">
          <span>{t`Display condition`}</span>
          <Tag bordered={false} className="!ml-1 !font-normal">
            {EmailBlockClass.from(block).getLabel()}
          </Tag>
        </div>
      }
      footer={[
        <Button key="cancel" onClick={onCancel}>
          {t`Cancel`}
        </Button>,
        <Button
          key="apply"
          type="primary"
          disabled={editing !== null || condition.rules.length === 0}
          onClick={() => onApply(condition)}
        >
          {t`Apply`}
        </Button>
      ]}
    >
      <Text type="secondary" className="text-xs block mb-4">
        {t`The block and everything in it are sent only to recipients for whom the condition holds. Variables are those of the email's data: contact fields, data sent to the API, data feeds.`}
      </Text>
      <div style={{ borderBottom: `1px solid ${LINE_COLOR}`, paddingBottom: '1rem' }}>
        <Select
          size="small"
          aria-label={t`Match`}
          className="mr-2"
          style={{ width: 130 }}
          value={condition.match}
          onChange={(match: 'all' | 'any') => setCondition({ ...condition, match })}
          options={[
            { value: 'all', label: t`ALL` },
            { value: 'any', label: t`AT LEAST ONE` }
          ]}
        />{' '}
        <span className="opacity-60">{t`of the following conditions match:`}</span>
      </div>
      {rows.map((rule, index) => (
        <div
          key={index}
          style={{
            marginLeft: 60,
            position: 'relative',
            borderBottom: '1px solid rgba(0, 0, 100, 0.1)'
          }}
        >
          <div
            style={{
              position: 'absolute',
              left: -35,
              top: 0,
              bottom: 0,
              borderRight: `1px solid ${SEPARATOR}`
            }}
          />
          {index !== 0 && <div style={ANDOR_BOX}>{andOr}</div>}
          {editing?.index === index ? editRow(editing.rule, index) : readRow(rule, index)}
        </div>
      ))}
      <div style={{ marginLeft: 60, position: 'relative' }}>
        <div
          style={{
            position: 'absolute',
            left: -35,
            top: 0,
            bottom: 25,
            borderRight: `1px solid ${SEPARATOR}`
          }}
        />
        {rows.length > 0 && <div style={ANDOR_BOX}>{andOr}</div>}
        <div className="py-4">
          <Button
            size="small"
            type="primary"
            ghost={rows.length > 0}
            disabled={editing !== null || condition.rules.length >= MAX_DISPLAY_CONDITION_RULES}
            onClick={() =>
              setEditing({
                index: condition.rules.length,
                rule: { variable: '', operator: 'is_true' }
              })
            }
          >
            <FontAwesomeIcon icon={faPlus} /> {t`Condition`}
          </Button>
        </div>
      </div>
      <div className="mt-2 text-xs text-slate-600">
        {t`With the test data:`}{' '}
        <ConditionResult shown={shown} label={shown ? t`block shown` : t`block hidden`} />
      </div>
    </Modal>
  )
}
