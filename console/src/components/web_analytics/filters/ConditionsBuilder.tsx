import { useCallback, useRef, useState } from 'react'
import { Button, Input, Popconfirm, Select } from 'antd'
import type { RefSelectProps } from 'antd/es/select'
import { DeleteOutlined, PlusOutlined } from '@ant-design/icons'
import { useLingui } from '@lingui/react/macro'
import { WebFilterCondition, WebFilterOperator } from '../../../services/api/web_analytics'
import {
  isValuelessOperator,
  OPERATORS,
  SOURCE_FIELD_OPTIONS,
  useFilterVocabulary
} from '../lib/filterCatalog'

interface ConditionRowProps {
  index: number
  value: WebFilterCondition
  onChange: (condition: WebFilterCondition) => void
  onRemove: () => void
  isOnlyCondition: boolean
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

  const showValueInput = !isValuelessOperator(props.value.operator)

  return (
    <div className="flex flex-col gap-2 md:flex-row md:items-center">
      <div className="flex flex-1 items-center gap-2">
        <span className="w-4 shrink-0 text-xs text-gray-400">{props.index + 1}.</span>
        <Select
          ref={handleSelectRef}
          value={props.value.field}
          onChange={(field) => props.onChange({ ...props.value, field })}
          options={SOURCE_FIELD_OPTIONS}
          placeholder={t`Select field`}
          className="w-full md:!w-[180px] md:shrink-0"
          showSearch
          optionFilterProp="label"
          open={isOpen}
          onOpenChange={setIsOpen}
        />
      </div>
      <Select
        value={props.value.operator}
        onChange={(operator: WebFilterOperator) => {
          if (isValuelessOperator(operator)) {
            props.onChange({ ...props.value, operator, value: undefined })
            return
          }
          props.onChange({ ...props.value, operator })
        }}
        options={OPERATORS.map((operator) => ({
          value: operator,
          label: vocabulary.operators[operator]
        }))}
        className="w-full md:!w-[160px] md:shrink-0"
      />
      {showValueInput ? (
        <Input
          value={props.value.value ?? ''}
          onChange={(event) => props.onChange({ ...props.value, value: event.target.value })}
          placeholder={props.value.operator === 'regex' ? t`Regular expression` : t`Value`}
          className="min-w-0 flex-1"
        />
      ) : (
        <div className="hidden flex-1 md:block" />
      )}
      <Popconfirm
        title={t`Delete this condition?`}
        onConfirm={props.onRemove}
        okText={t`Delete`}
        cancelText={t`Cancel`}
        disabled={props.isOnlyCondition}
      >
        <Button
          type="text"
          icon={<DeleteOutlined />}
          disabled={props.isOnlyCondition}
          className="shrink-0 self-end md:self-auto"
        />
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
    <div>
      <div className="mb-3 text-xs text-gray-500">{t`All conditions must match (AND logic):`}</div>
      <div className="space-y-2">
        {conditions.map((condition, index) => (
          <ConditionRow
            key={index}
            index={index}
            value={condition}
            onChange={(next) =>
              emit(conditions.map((current, position) => (position === index ? next : current)))
            }
            onRemove={() => emit(conditions.filter((_, position) => position !== index))}
            isOnlyCondition={conditions.length === 1}
            autoFocus={focusIndex === index}
            onFocused={() => setFocusIndex(null)}
          />
        ))}
      </div>
      <Button
        type="primary"
        ghost
        block
        size="small"
        icon={<PlusOutlined />}
        onClick={addCondition}
        className="!mt-4"
      >
        {t`Add condition`}
      </Button>
    </div>
  )
}
