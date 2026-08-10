import { useCallback, useMemo, useRef, useState } from 'react'
import { Button, Input, Popconfirm, Select } from 'antd'
import type { RefSelectProps } from 'antd/es/select'
import { DeleteOutlined, PlusOutlined } from '@ant-design/icons'
import { useLingui } from '@lingui/react/macro'
import { WebFilterAction, WebFilterOperation } from '../../../services/api/web_analytics'
import { buildDimensionOptions, FILTER_ACTIONS, useFilterVocabulary } from '../lib/filterCatalog'

interface ActionOption {
  value: WebFilterAction
  label: string
  description: string
}

interface OperationRowProps {
  index: number
  value: WebFilterOperation
  onChange: (operation: WebFilterOperation) => void
  onRemove: () => void
  isOnlyOperation: boolean
  dimensionOptions: ReturnType<typeof buildDimensionOptions>
  actionOptions: ActionOption[]
  autoFocus?: boolean
  onFocused?: () => void
}

function OperationRow(props: OperationRowProps) {
  const { t } = useLingui()
  const selectRef = useRef<RefSelectProps | null>(null)
  const [isOpen, setIsOpen] = useState(false)
  const hasAutoFocused = useRef(false)

  const { autoFocus, onFocused } = props

  // A ref callback rather than an effect: the row is focused exactly once, when
  // the "Add operation" button mounts it, and never again on later renders.
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

  const showValueInput =
    props.value.action === 'set_value' || props.value.action === 'set_default_value'

  return (
    <div className="flex flex-col gap-2 md:flex-row md:items-center">
      <div className="flex flex-1 items-center gap-2">
        <span className="w-4 shrink-0 text-xs text-gray-400">{props.index + 1}.</span>
        <Select
          ref={handleSelectRef}
          value={props.value.dimension}
          onChange={(dimension) => props.onChange({ ...props.value, dimension })}
          options={props.dimensionOptions}
          placeholder={t`Select dimension`}
          className="w-full md:!w-[180px] md:shrink-0"
          showSearch
          optionFilterProp="label"
          open={isOpen}
          onOpenChange={setIsOpen}
        />
      </div>
      <Select<WebFilterAction, ActionOption>
        value={props.value.action}
        onChange={(action) =>
          props.onChange({
            ...props.value,
            action,
            value: action === 'unset_value' ? '' : props.value.value
          })
        }
        options={props.actionOptions}
        className="w-full md:!w-[160px] md:shrink-0"
        optionRender={(option) => (
          <div>
            <div>{option.label}</div>
            <div className="text-xs text-gray-400">{option.data.description}</div>
          </div>
        )}
      />
      {showValueInput ? (
        <Input
          value={props.value.value ?? ''}
          onChange={(event) => props.onChange({ ...props.value, value: event.target.value })}
          placeholder={t`Value`}
          className="min-w-0 flex-1"
        />
      ) : (
        <div className="hidden flex-1 md:block" />
      )}
      <Popconfirm
        title={t`Delete this operation?`}
        onConfirm={props.onRemove}
        okText={t`Delete`}
        cancelText={t`Cancel`}
        disabled={props.isOnlyOperation}
      >
        <Button
          type="text"
          icon={<DeleteOutlined />}
          disabled={props.isOnlyOperation}
          className="shrink-0 self-end md:self-auto"
        />
      </Popconfirm>
    </div>
  )
}

export interface OperationsBuilderProps {
  /** Injected by the enclosing Form.Item. */
  value?: WebFilterOperation[]
  onChange?: (operations: WebFilterOperation[]) => void
  customDimensionLabels?: Record<string, string>
}

export function OperationsBuilder(props: OperationsBuilderProps) {
  const { t } = useLingui()
  const vocabulary = useFilterVocabulary()
  const operations = props.value ?? []
  const [focusIndex, setFocusIndex] = useState<number | null>(null)

  const { customDimensionLabels } = props
  const dimensionOptions = useMemo(
    () => buildDimensionOptions(customDimensionLabels),
    [customDimensionLabels]
  )

  const actionOptions: ActionOption[] = FILTER_ACTIONS.map((action) => ({
    value: action,
    label: vocabulary.actions[action],
    description: vocabulary.actionDescriptions[action]
  }))

  const emit = (next: WebFilterOperation[]) => props.onChange?.(next)

  const addOperation = () => {
    const newIndex = operations.length
    emit([...operations, { dimension: 'channel', action: 'set_value', value: '' }])
    setFocusIndex(newIndex)
  }

  return (
    <div>
      <div className="mb-3 text-xs text-gray-500">{t`When matched, execute these operations:`}</div>
      <div className="space-y-2">
        {operations.map((operation, index) => (
          <OperationRow
            key={index}
            index={index}
            value={operation}
            onChange={(next) =>
              emit(operations.map((current, position) => (position === index ? next : current)))
            }
            onRemove={() => emit(operations.filter((_, position) => position !== index))}
            isOnlyOperation={operations.length === 1}
            dimensionOptions={dimensionOptions}
            actionOptions={actionOptions}
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
        onClick={addOperation}
        className="!mt-4"
      >
        {t`Add operation`}
      </Button>
    </div>
  )
}
