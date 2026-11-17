import { useLingui } from '@lingui/react/macro'
import { getDimensionLabel } from './dimensions'
import { formatDuration, formatPercent } from './format'
import { isValuelessDimensionFilterOperator, type WebDimensionFilter, type WebMetricFilter } from './types'

/** The wording of each filter operator, as the toolbar chips write it. */
export function useOperatorLabels(): Record<string, string> {
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

/** "Device equals mobile", "UTM Term is empty": a dimension filter as its chip reads. */
export function describeDimensionFilter(
  filter: WebDimensionFilter,
  operatorLabels: Record<string, string>,
  customLabels?: Record<string, string>
): string {
  const label = getDimensionLabel(filter.dimension, customLabels)
  const operator = operatorLabels[filter.operator] ?? filter.operator
  if (isValuelessDimensionFilterOperator(filter.operator)) return `${label} ${operator}`
  return `${label} ${operator} ${filter.values.join(', ')}`
}

/**
 * The threshold of a metric filter as typed. Ten decimals at most: formatPercent divides by 100
 * and Intl multiplies back, which turns 12.3 into "12.300000000000001%" when twenty are allowed.
 */
export function describeMetricValue(filter: WebMetricFilter): string {
  return filter.metric === 'median_duration'
    ? formatDuration(filter.values[0])
    : formatPercent(filter.values[0], 10, 0)
}

/** "TimeScore >= 30 s": a metric filter as its chip reads. */
export function describeMetricFilter(
  filter: WebMetricFilter,
  operatorLabels: Record<string, string>,
  metricLabels: Record<string, string>
): string {
  return `${metricLabels[filter.metric] ?? filter.metric} ${operatorLabels[filter.operator] ?? filter.operator} ${describeMetricValue(filter)}`
}
