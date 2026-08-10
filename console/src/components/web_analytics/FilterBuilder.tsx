import { useMemo, useState } from 'react'
import { Button, Input, InputNumber, Popover, Select, Space, Tag } from 'antd'
import { PlusCircleOutlined, SearchOutlined } from '@ant-design/icons'
import { useLingui } from '@lingui/react/macro'
import { useWebAnalytics } from './context'
import {
  DimensionInfo,
  dimensionsForSchema,
  getDimensionLabel,
  groupByCategory
} from './lib/dimensions'
import { DIMENSION_VALUE_OPTIONS } from './lib/dictionaries'
import { formatDuration } from './lib/format'
import {
  DimensionFilterOperator,
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

const VALUELESS_OPERATORS: DimensionFilterOperator[] = ['isEmpty', 'isNotEmpty']
const MULTI_VALUE_OPERATORS: DimensionFilterOperator[] = ['in', 'notIn']

/** Measures a threshold can be set on; session count has its own control. */
const FILTERABLE_METRICS = ['median_duration', 'bounce_rate', 'median_scroll'] as const

function useOperatorLabels(): Record<string, string> {
  const { t } = useLingui()
  return {
    equals: t`equals`,
    notEquals: t`not equals`,
    contains: t`contains`,
    notContains: t`not contains`,
    isEmpty: t`is empty`,
    isNotEmpty: t`is not empty`,
    in: t`in`,
    notIn: t`not in`,
    gt: '>',
    gte: '>=',
    lt: '<',
    lte: '<='
  }
}

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

export function FilterBuilder(props: FilterBuilderProps) {
  const { t } = useLingui()
  const context = useWebAnalytics()
  const operatorLabels = useOperatorLabels()
  const [open, setOpen] = useState(false)

  const metricLabels: Record<string, string> = {
    median_duration: t`TimeScore`,
    bounce_rate: t`Bounce Rate`,
    median_scroll: t`Median Scroll Depth`
  }

  const describeValues = (filter: WebDimensionFilter) =>
    VALUELESS_OPERATORS.includes(filter.operator) ? '' : filter.values.join(', ')

  const describeMetricValue = (filter: WebMetricFilter) =>
    filter.metric === 'median_duration'
      ? formatDuration(filter.values[0])
      : `${filter.values[0]}%`

  return (
    <div className="flex flex-wrap items-center gap-2">
      <Popover
        open={open}
        onOpenChange={setOpen}
        trigger="click"
        placement="bottomLeft"
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
          <Tag
            key={`${filter.dimension}-${index}`}
            color="orange"
            closable
            onClose={() =>
              context.setFilters(context.filters.filter((_, position) => position !== index))
            }
          >
            {getDimensionLabel(filter.dimension, context.customDimensionLabels)}{' '}
            {operatorLabels[filter.operator]} {describeValues(filter)}
          </Tag>
        ))}
        {props.allowMetricFilters
          ? context.metricFilters.map((filter, index) => (
              <Tag
                key={`${filter.metric}-${index}`}
                color="gold"
                closable
                onClose={() =>
                  context.setMetricFilters(
                    context.metricFilters.filter((_, position) => position !== index)
                  )
                }
              >
                {metricLabels[filter.metric] ?? filter.metric} {operatorLabels[filter.operator]}{' '}
                {describeMetricValue(filter)}
              </Tag>
            ))
          : null}
      </Space>
    </div>
  )
}

/** Two-step picker: choose a field, then its operator and value. */
function FilterPicker(props: {
  schema: WebSchema
  allowMetricFilters?: boolean
  onClose: () => void
}) {
  const { t } = useLingui()
  const context = useWebAnalytics()
  const operatorLabels = useOperatorLabels()
  const categoryLabels = useCategoryLabels()

  const [search, setSearch] = useState('')
  const [dimension, setDimension] = useState<DimensionInfo | null>(null)
  const [metric, setMetric] = useState<string | null>(null)
  const [operator, setOperator] = useState<DimensionFilterOperator | MetricFilterOperator>('equals')
  const [values, setValues] = useState<string[]>([])
  const [metricValue, setMetricValue] = useState<number>(0)

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
  }

  const applyDimension = () => {
    if (!dimension) return
    const operators = OPERATORS_BY_TYPE[dimension.type] ?? OPERATORS_BY_TYPE.string
    const chosen = operators.includes(operator as DimensionFilterOperator)
      ? (operator as DimensionFilterOperator)
      : 'equals'
    if (!VALUELESS_OPERATORS.includes(chosen) && values.length === 0) return
    context.setFilters([
      ...context.filters,
      { dimension: dimension.name, operator: chosen, values }
    ])
    reset()
    props.onClose()
  }

  const applyMetric = () => {
    if (!metric) return
    context.setMetricFilters([
      ...context.metricFilters,
      { metric, operator: operator as MetricFilterOperator, values: [metricValue] }
    ])
    reset()
    props.onClose()
  }

  if (dimension) {
    const operators = OPERATORS_BY_TYPE[dimension.type] ?? OPERATORS_BY_TYPE.string
    const dictionary = DIMENSION_VALUE_OPTIONS[dimension.name]
    const multiple = MULTI_VALUE_OPERATORS.includes(operator as DimensionFilterOperator)

    return (
      <div className="w-72">
        <Button type="link" size="small" className="!px-0" onClick={reset}>
          {t`Back`}
        </Button>
        <div className="mb-2 text-sm font-medium">
          {getDimensionLabel(dimension.name, context.customDimensionLabels)}
        </div>
        <div className="mb-2 flex flex-wrap gap-1">
          {operators.map((candidate) => (
            <button
              key={candidate}
              type="button"
              onClick={() => setOperator(candidate)}
              className={
                candidate === operator
                  ? 'rounded bg-[var(--primary)] px-2 py-0.5 text-xs text-white'
                  : 'rounded bg-gray-100 px-2 py-0.5 text-xs text-gray-700 hover:bg-gray-200'
              }
            >
              {operatorLabels[candidate]}
            </button>
          ))}
        </div>
        {!VALUELESS_OPERATORS.includes(operator as DimensionFilterOperator) ? (
          dictionary ? (
            <Select
              className="w-full"
              mode={multiple ? 'multiple' : undefined}
              showSearch
              placeholder={t`Value`}
              value={multiple ? values : values[0]}
              options={dictionary.map((value) => ({ value, label: value }))}
              onChange={(value) => setValues(Array.isArray(value) ? value : [value])}
            />
          ) : (
            <Input
              autoFocus
              placeholder={multiple ? t`Comma-separated values` : t`Value`}
              value={values.join(', ')}
              onChange={(event) =>
                setValues(
                  multiple
                    ? event.target.value.split(',').map((part) => part.trim()).filter(Boolean)
                    : [event.target.value]
                )
              }
              onPressEnter={applyDimension}
            />
          )
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
        <Button type="link" size="small" className="!px-0" onClick={reset}>
          {t`Back`}
        </Button>
        <div className="mb-2 text-sm font-medium">{metricLabels[metric] ?? metric}</div>
        <div className="mb-2 flex flex-wrap gap-1">
          {(['gt', 'gte', 'lt', 'lte'] as MetricFilterOperator[]).map((candidate) => (
            <button
              key={candidate}
              type="button"
              onClick={() => setOperator(candidate)}
              className={
                candidate === operator
                  ? 'rounded bg-[var(--primary)] px-2 py-0.5 text-xs text-white'
                  : 'rounded bg-gray-100 px-2 py-0.5 text-xs text-gray-700 hover:bg-gray-200'
              }
            >
              {operatorLabels[candidate]}
            </button>
          ))}
        </div>
        <InputNumber
          className="w-full"
          min={0}
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
        prefix={<SearchOutlined />}
        allowClear
        placeholder={t`Search`}
        value={search}
        onChange={(event) => setSearch(event.target.value)}
      />
      <div className="mt-2 max-h-80 overflow-y-auto">
        {groups.map((group) => (
          <div key={group.category} className="mb-2">
            <div className="px-1 py-1 text-[10px] font-semibold uppercase text-[var(--primary)]">
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
                className="block w-full rounded px-1 py-1 text-left text-sm hover:bg-gray-50"
              >
                {getDimensionLabel(candidate.name, context.customDimensionLabels)}
              </button>
            ))}
          </div>
        ))}
        {props.allowMetricFilters ? (
          <div className="mb-2">
            <div className="px-1 py-1 text-[10px] font-semibold uppercase text-[var(--primary)]">
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
                  className="block w-full rounded px-1 py-1 text-left text-sm hover:bg-gray-50"
                >
                  {metricLabels[candidate]}
                </button>
              )
            )}
          </div>
        ) : null}
        {groups.length === 0 ? (
          <div className="px-1 py-4 text-center text-xs text-gray-400">{t`No results found`}</div>
        ) : null}
      </div>
    </div>
  )
}
