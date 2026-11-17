import { useCallback, useRef, useState } from 'react'
import { Button, ConfigProvider, Form, Popconfirm, Select } from 'antd'
import type { RefSelectProps } from 'antd/es/select'
import { DeleteOutlined, PlusOutlined } from '@ant-design/icons'
import { useLingui } from '@lingui/react/macro'
import { RuleValueInput } from './RuleValueInput'
import { WebFilterCondition, WebFilterOperator } from '../../../services/api/web_analytics'
import {
  isValuelessRuleOperator,
  OPERATORS,
  SOURCE_FIELD_OPTIONS,
  useFilterVocabulary
} from '../lib/filterCatalog'
import { typography } from '../../../theme/tokens'

const MONO_THEME = { token: { fontFamily: typography.mono } }

interface ConditionRowProps {
  index: number
  value: WebFilterCondition
  onChange: (condition: WebFilterCondition) => void
  onRemove: () => void
  autoFocus?: boolean
  onFocused?: () => void
}

function ConditionRow(props: ConditionRowProps) {
  const { t } = useLingui()
  const vocabulary = useFilterVocabulary()
  const selectRef = useRef<RefSelectProps | null>(null)
  const [isOpen, setIsOpen] = useState(false)
  const hasAutoFocused = useRef(false)

  const { autoFocus, onFocused } = props

  // A ref callback rather than an effect: the row is focused exactly once, when
  // the "Add condition" button mounts it, and never again on later renders.
  const handleSelectRef = useCallback(
    (node: RefSelectProps | null) => {
      selectRef.current = node
      if (autoFocus && node && !hasAutoFocused.current) {
        hasAutoFocused.current = true
        // Defer past the mount so the dropdown anchors to a laid-out element.
        setTimeout(() => {
          selectRef.current?.focus()
          setIsOpen(true)
          onFocused?.()
        }, 0)
      }
    },
    [autoFocus, onFocused]
  )

  const showValueInput = !isValuelessRuleOperator(props.value.operator)
  // The list's error, once shown, outlines the value that causes it, not the whole row.
  const { errors } = Form.Item.useStatus()
  const valueMissing = errors.length > 0 && !props.value.value?.trim()

  // One line when the card is wide enough (field, operator, value, delete); on a narrow card the
  // value goes under them.
  return (
    <div
      data-condition-row
      className="flex flex-wrap items-center gap-2 border-t border-line-soft py-2.5 first:border-t-0 first:pt-0"
    >
      <span className="w-4 shrink-0 text-xs text-ink-4">{props.index + 1}.</span>
      <div className="min-w-0 flex-1">
        <Select
          ref={handleSelectRef}
          value={props.value.field}
          onChange={(field) => props.onChange({ ...props.value, field })}
          options={SOURCE_FIELD_OPTIONS}
          placeholder={t`Select field`}
          aria-label={t`Field`}
          style={{ width: '100%' }}
          showSearch
          optionFilterProp="label"
          open={isOpen}
          onOpenChange={setIsOpen}
        />
      </div>
      <div className="w-[140px] shrink-0">
        <Select
          value={props.value.operator}
          onChange={(operator: WebFilterOperator) => {
            if (isValuelessRuleOperator(operator)) {
              props.onChange({ ...props.value, operator, value: undefined })
              return
            }
            props.onChange({ ...props.value, operator })
          }}
          options={OPERATORS.map((operator) => ({
            value: operator,
            label: vocabulary.operators[operator]
          }))}
          aria-label={t`Operator`}
          style={{ width: '100%' }}
        />
      </div>
      {showValueInput ? (
        <div className="order-last basis-full pl-6 @min-[480px]:order-none @min-[480px]:basis-0 @min-[480px]:flex-1 @min-[480px]:pl-0">
          {/* A pattern reads in the code typeface. */}
          <ConfigProvider theme={props.value.operator === 'regex' ? MONO_THEME : undefined}>
            <RuleValueInput
              // A pattern is typed whatever the field holds.
              field={props.value.operator === 'regex' ? '' : props.value.field}
              value={props.value.value ?? ''}
              onChange={(value) => props.onChange({ ...props.value, value })}
              placeholder={props.value.operator === 'regex' ? t`Regular expression` : t`Value`}
              status={valueMissing ? 'error' : undefined}
            />
          </ConfigProvider>
        </div>
      ) : null}
      <Popconfirm
        title={t`Delete this condition?`}
        onConfirm={props.onRemove}
        okText={t`Delete`}
        okButtonProps={{ danger: true }}
        cancelText={t`Cancel`}
      >
        <Button type="text" aria-label={t`Delete condition`} icon={<DeleteOutlined />} />
      </Popconfirm>
    </div>
  )
}

export interface ConditionsBuilderProps {
  /** Injected by the enclosing Form.Item. */
  value?: WebFilterCondition[]
  onChange?: (conditions: WebFilterCondition[]) => void
}

export function ConditionsBuilder(props: ConditionsBuilderProps) {
  const { t } = useLingui()
  const conditions = props.value ?? []
  const [focusIndex, setFocusIndex] = useState<number | null>(null)

  const emit = (next: WebFilterCondition[]) => props.onChange?.(next)

  const addCondition = () => {
    const newIndex = conditions.length
    emit([...conditions, { field: 'utm_source', operator: 'equals', value: '' }])
    setFocusIndex(newIndex)
  }

  return (
    <div className="@container">
      {conditions.length === 0 ? (
        // The server runs a rule without conditions on every session; the seeded catch-alls are.
        <p data-always-matches className="m-0 text-sm text-ink-3">
          {t`Always matches: with no condition, this rule runs on every session.`}
        </p>
      ) : null}
      <div>
        {conditions.map((condition, index) => (
          <ConditionRow
            key={index}
            index={index}
            value={condition}
            onChange={(next) =>
              emit(conditions.map((current, position) => (position === index ? next : current)))
            }
            onRemove={() => emit(conditions.filter((_, position) => position !== index))}
            autoFocus={focusIndex === index}
            onFocused={() => setFocusIndex(null)}
          />
        ))}
      </div>
      <div className="mt-3">
        <Button type="dashed" block size="small" icon={<PlusOutlined />} onClick={addCondition}>
          {t`Add condition`}
        </Button>
      </div>
    </div>
  )
}
