import { useMemo, useState, type KeyboardEvent } from 'react'
import { Button, Input, InputNumber, Popover, Select, Space, Tag } from 'antd'
import { PlusCircleOutlined, SearchOutlined } from '@ant-design/icons'
import { useLingui } from '@lingui/react/macro'
import { useWebAnalytics } from './useWebAnalytics'
import {
  DIMENSIONS,
  DimensionInfo,
  dimensionsForSchema,
  getDimensionLabel,
  groupByCategory
} from './lib/dimensions'
import { DIMENSION_VALUE_OPTIONS } from './lib/dictionaries'
import { describeDimensionFilter, describeMetricFilter, useOperatorLabels } from './lib/filterLabels'
import {
  DimensionFilterOperator,
  isValuelessDimensionFilterOperator,
  MetricFilterOperator,
  WebDimensionFilter,
  WebMetricFilter,
  WebSchema
} from './lib/types'

const OPERATORS_BY_TYPE: Record<string, DimensionFilterOperator[]> = {
  string: ['equals', 'notEquals', 'contains', 'notContains', 'isEmpty', 'isNotEmpty', 'in', 'notIn'],
  number: ['equals', 'notEquals', 'gt', 'gte', 'lt', 'lte'],
  boolean: ['equals', 'notEquals']
}

const MULTI_VALUE_OPERATORS: DimensionFilterOperator[] = ['in', 'notIn']

/** Measures a threshold can be set on; session count has its own control. */
const FILTERABLE_METRICS = ['median_duration', 'bounce_rate', 'median_scroll'] as const

function useCategoryLabels(): Record<string, string> {
  const { t } = useLingui()
  return {
    Channel: t`Channel`,
    UTM: t`UTM`,
    Traffic: t`Traffic`,
    Pages: t`Pages`,
    Device: t`Device`,
    Geo: t`Geo`,
    Time: t`Time`,
    Session: t`Session`,
    Goal: t`Goal`,
    User: t`User`,
    Custom: t`Custom dimensions`
  }
}

interface FilterBuilderProps {
  schema?: WebSchema
  /** Explore also filters on aggregated measures (rendered as SQL HAVING). */
  allowMetricFilters?: boolean
}

/** The chip whose filter is being edited, by its position in its list. */
interface EditedChip {
  kind: 'dimension' | 'metric'
  index: number
}

export function FilterBuilder(props: FilterBuilderProps) {
  const { t } = useLingui()
  const context = useWebAnalytics()
  const operatorLabels = useOperatorLabels()
  const [open, setOpen] = useState(false)
  const [edited, setEdited] = useState<EditedChip | null>(null)

  const isEdited = (kind: EditedChip['kind'], index: number) =>
    edited?.kind === kind && edited.index === index
  // A chip opens its filter for editing on a click, or on Enter or Space from the keyboard.
  const chipProps = (kind: EditedChip['kind'], index: number) => ({
    role: 'button',
    tabIndex: 0,
    onKeyDown: (event: KeyboardEvent<HTMLElement>) => {
      if (event.key !== 'Enter' && event.key !== ' ') return
      event.preventDefault()
      setEdited({ kind, index })
    }
  })
  const editPopover = (kind: EditedChip['kind'], index: number) => ({
    open: isEdited(kind, index),
    onOpenChange: (next: boolean) => setEdited(next ? { kind, index } : null),
    trigger: 'click' as const,
    placement: 'bottomLeft' as const,
    destroyOnHidden: true,
    content: (
      <FilterPicker
        schema={props.schema ?? 'web_sessions'}
        allowMetricFilters={props.allowMetricFilters}
        edit={{ kind, index }}
        onClose={() => setEdited(null)}
      />
    )
  })

  const metricLabels: Record<string, string> = {
    median_duration: t`TimeScore`,
    bounce_rate: t`Bounce Rate`,
    median_scroll: t`Median Scroll Depth`
  }

  return (
    <div className="flex flex-wrap items-center gap-2">
      <Popover
        open={open}
        onOpenChange={setOpen}
        trigger="click"
        placement="bottomLeft"
        // Each opening starts on the field list, its search focused.
        destroyOnHidden
        content={
          <FilterPicker
            schema={props.schema ?? 'web_sessions'}
            allowMetricFilters={props.allowMetricFilters}
            onClose={() => setOpen(false)}
          />
        }
      >
        <Button type="link" size="small" icon={<PlusCircleOutlined />}>
          {t`Add filter`}
        </Button>
      </Popover>

      <Space size={[8, 4]} wrap>
        {context.filters.map((filter, index) => (
          <Popover key={`${filter.dimension}-${index}`} {...editPopover('dimension', index)}>
            <Tag
              color="orange"
              closable
              {...chipProps('dimension', index)}
              onClose={(event) => {
                // The close icon sits in the chip: without this the click would also open it.
                event.stopPropagation()
                context.setFilters(context.filters.filter((_, position) => position !== index))
              }}
            >
              {describeDimensionFilter(filter, operatorLabels, context.customDimensionLabels)}
            </Tag>
          </Popover>
        ))}
        {props.allowMetricFilters
          ? context.metricFilters.map((filter, index) => (
              <Popover key={`${filter.metric}-${index}`} {...editPopover('metric', index)}>
                <Tag
                  color="gold"
                  closable
                  {...chipProps('metric', index)}
                  onClose={(event) => {
                    event.stopPropagation()
                    context.setMetricFilters(
                      context.metricFilters.filter((_, position) => position !== index)
                    )
                  }}
                >
                  {describeMetricFilter(filter, operatorLabels, metricLabels)}
                </Tag>
              </Popover>
            ))
          : null}
      </Space>
    </div>
  )
}

/**
 * Two-step picker: choose a field, then its operator and value. Opened from a chip (`edit`), it
 * starts on the second step with that filter, and Apply replaces it in place.
 */
function FilterPicker(props: {
  schema: WebSchema
  allowMetricFilters?: boolean
  edit?: EditedChip
  onClose: () => void
}) {
  const { t } = useLingui()
  const context = useWebAnalytics()
  const operatorLabels = useOperatorLabels()
  const categoryLabels = useCategoryLabels()

  const { edit } = props
  const editedFilter = edit?.kind === 'dimension' ? context.filters[edit.index] : undefined
  const editedMetric = edit?.kind === 'metric' ? context.metricFilters[edit.index] : undefined

  const [search, setSearch] = useState('')
  const [dimension, setDimension] = useState<DimensionInfo | null>(
    editedFilter ? (DIMENSIONS[editedFilter.dimension] ?? null) : null
  )
  const [metric, setMetric] = useState<string | null>(editedMetric?.metric ?? null)
  const [operator, setOperator] = useState<DimensionFilterOperator | MetricFilterOperator>(
    editedFilter?.operator ?? editedMetric?.operator ?? 'equals'
  )
  const [values, setValues] = useState<string[]>(
    editedFilter ? editedFilter.values.map((value) => String(value)) : []
  )
  const [metricValue, setMetricValue] = useState<number>(editedMetric?.values[0] ?? 0)
  // Set by Apply on an empty value, so the field says what is missing rather than nothing happening.
  const [valueMissing, setValueMissing] = useState(false)

  const metricLabels: Record<string, string> = {
    median_duration: t`TimeScore (seconds)`,
    bounce_rate: t`Bounce Rate (%)`,
    median_scroll: t`Median Scroll Depth (%)`
  }

  const used = new Set(context.filters.map((filter) => filter.dimension))
  const usedMetrics = new Set(context.metricFilters.map((filter) => filter.metric))

  const groups = useMemo(() => {
    const term = search.trim().toLowerCase()
    const available = dimensionsForSchema(props.schema).filter(
      (candidate) => !used.has(candidate.name)
    )
    const matching = term
      ? available.filter(
          (candidate) =>
            getDimensionLabel(candidate.name, context.customDimensionLabels)
              .toLowerCase()
              .includes(term) || candidate.category.toLowerCase().includes(term)
        )
      : available
    return groupByCategory(matching)
    // eslint-disable-next-line react-hooks/exhaustive-deps -- `used` is derived from context.filters
  }, [search, props.schema, context.filters, context.customDimensionLabels])

  const reset = () => {
    setDimension(null)
    setMetric(null)
    setValues([])
    setMetricValue(0)
    setOperator('equals')
    setValueMissing(false)
  }

  const applyDimension = () => {
    if (!dimension) return
    const operators = OPERATORS_BY_TYPE[dimension.type] ?? OPERATORS_BY_TYPE.string
    const chosen = operators.includes(operator as DimensionFilterOperator)
      ? (operator as DimensionFilterOperator)
      : 'equals'
    const typed = values.filter((value) => value.trim() !== '')
    if (!isValuelessDimensionFilterOperator(chosen) && typed.length === 0) {
      setValueMissing(true)
      return
    }
    const next: WebDimensionFilter = {
      dimension: dimension.name,
      operator: chosen,
      values: isValuelessDimensionFilterOperator(chosen) ? [] : typed
    }
    context.setFilters(
      editedFilter && edit
        ? context.filters.map((filter, position) => (position === edit.index ? next : filter))
        : [...context.filters, next]
    )
    reset()
    props.onClose()
  }

  const applyMetric = () => {
    if (!metric) return
    const next: WebMetricFilter = {
      metric,
      operator: operator as MetricFilterOperator,
      values: [metricValue]
    }
    context.setMetricFilters(
      editedMetric && edit
        ? context.metricFilters.map((filter, position) => (position === edit.index ? next : filter))
        : [...context.metricFilters, next]
    )
    reset()
    props.onClose()
  }

  const changeValues = (next: string[]) => {
    setValues(next)
    setValueMissing(false)
  }

  if (dimension) {
    const operators = OPERATORS_BY_TYPE[dimension.type] ?? OPERATORS_BY_TYPE.string
    const dictionary = DIMENSION_VALUE_OPTIONS[dimension.name]
    const multiple = MULTI_VALUE_OPERATORS.includes(operator as DimensionFilterOperator)

    return (
      <div className="w-72">
        {/* An edited filter keeps its field: Back would drop it for a list it is not in. */}
        {editedFilter ? null : (
          <Button type="link" size="small" className="!px-0" onClick={reset}>
            {t`Back`}
          </Button>
        )}
        <div className="mb-2 text-sm font-medium">
          {getDimensionLabel(dimension.name, context.customDimensionLabels)}
        </div>
        <div className="mb-2 flex flex-wrap gap-1">
          {operators.map((candidate) => (
            <button
              key={candidate}
              type="button"
              onClick={() => {
                setOperator(candidate)
                setValueMissing(false)
              }}
              className={
                candidate === operator
                  ? 'rounded bg-primary px-2 py-0.5 text-xs text-white'
                  : 'rounded bg-fill px-2 py-0.5 text-xs text-ink-2 hover:bg-line'
              }
            >
              {operatorLabels[candidate]}
            </button>
          ))}
        </div>
        {!isValuelessDimensionFilterOperator(operator) ? (
          dictionary ? (
            <Select
              className="w-full"
              mode={multiple ? 'multiple' : undefined}
              showSearch
              placeholder={t`Value`}
              value={multiple ? values : values[0]}
              options={dictionary.map((value) => ({ value, label: value }))}
              onChange={(value) => changeValues(Array.isArray(value) ? value : [value])}
              status={valueMissing ? 'error' : undefined}
            />
          ) : (
            <Input
              autoFocus
              placeholder={multiple ? t`Comma-separated values` : t`Value`}
              value={values.join(', ')}
              onChange={(event) =>
                changeValues(
                  multiple
                    ? event.target.value.split(',').map((part) => part.trim()).filter(Boolean)
                    : [event.target.value]
                )
              }
              onPressEnter={applyDimension}
              status={valueMissing ? 'error' : undefined}
            />
          )
        ) : null}
        {valueMissing ? (
          <div role="alert" className="mt-1 text-xs text-danger-fg">
            {multiple ? t`Enter at least one value` : t`Enter a value`}
          </div>
        ) : null}
        <Button type="primary" block className="!mt-3" onClick={applyDimension}>
          {t`Apply filter`}
        </Button>
      </div>
    )
  }

  if (metric) {
    return (
      <div className="w-72">
        {editedMetric ? null : (
          <Button type="link" size="small" className="!px-0" onClick={reset}>
            {t`Back`}
          </Button>
        )}
        <div className="mb-2 text-sm font-medium">{metricLabels[metric] ?? metric}</div>
        <div className="mb-2 flex flex-wrap gap-1">
          {(['gt', 'gte', 'lt', 'lte'] as MetricFilterOperator[]).map((candidate) => (
            <button
              key={candidate}
              type="button"
              onClick={() => setOperator(candidate)}
              className={
                candidate === operator
                  ? 'rounded bg-primary px-2 py-0.5 text-xs text-white'
                  : 'rounded bg-fill px-2 py-0.5 text-xs text-ink-2 hover:bg-line'
              }
            >
              {operatorLabels[candidate]}
            </button>
          ))}
        </div>
        <InputNumber
          className="w-full"
          min={0}
          // A rate is a percent, one decimal at most; a duration counts whole seconds
          max={metric === 'median_duration' ? undefined : 100}
          precision={metric === 'median_duration' ? 0 : 1}
          value={metricValue}
          onChange={(value) => setMetricValue(value ?? 0)}
        />
        <Button type="primary" block className="!mt-3" onClick={applyMetric}>
          {t`Apply filter`}
        </Button>
      </div>
    )
  }

  return (
    <div className="w-72">
      <Input
        autoFocus
        prefix={<SearchOutlined />}
        allowClear
        placeholder={t`Search`}
        value={search}
        onChange={(event) => setSearch(event.target.value)}
      />
      <div className="mt-2 max-h-80 overflow-y-auto">
        {groups.map((group) => (
          <div key={group.category} className="mb-2">
            <div className="px-1 py-1 text-overline font-semibold uppercase text-primary">
              {categoryLabels[group.category] ?? group.category}
            </div>
            {group.dimensions.map((candidate) => (
              <button
                key={candidate.name}
                type="button"
                onClick={() => {
                  setDimension(candidate)
                  setOperator((OPERATORS_BY_TYPE[candidate.type] ?? OPERATORS_BY_TYPE.string)[0])
                  setValues([])
                }}
                className="block w-full rounded px-1 py-1 text-left text-sm hover:bg-subtle"
              >
                {getDimensionLabel(candidate.name, context.customDimensionLabels)}
              </button>
            ))}
          </div>
        ))}
        {props.allowMetricFilters ? (
          <div className="mb-2">
            <div className="px-1 py-1 text-overline font-semibold uppercase text-primary">
              {t`Metrics`}
            </div>
            {FILTERABLE_METRICS.filter((candidate) => !usedMetrics.has(candidate)).map(
              (candidate) => (
                <button
                  key={candidate}
                  type="button"
                  onClick={() => {
                    setMetric(candidate)
                    setOperator('gt')
                  }}
                  className="block w-full rounded px-1 py-1 text-left text-sm hover:bg-subtle"
                >
                  {metricLabels[candidate]}
                </button>
              )
            )}
          </div>
        ) : null}
        {groups.length === 0 ? (
          <div className="px-1 py-4 text-center text-xs text-ink-4">{t`No results found`}</div>
        ) : null}
      </div>
    </div>
  )
}
