import { Skeleton, Tooltip } from 'antd'
import { InfoCircleOutlined } from '@ant-design/icons'
import { Delta } from './Delta'
import { formatValue } from './lib/format'
import { MetricConfig, MetricTotals } from './lib/types'

interface MetricSummaryProps {
  metrics: MetricConfig[]
  totals: Record<string, MetricTotals>
  /** Which metric the chart below is currently plotting. */
  selected: string
  onSelect: (metric: string) => void
  showComparison: boolean
  loading?: boolean
  currency?: string
  /** Translated labels and tooltips, keyed by metric. */
  labels: Record<string, string>
  tooltips?: Record<string, string>
}

/**
 * The KPI strip is also the chart's metric selector: the numbers are the thing
 * an operator scans first, so making them the control removes a separate
 * dropdown that would only ever repeat them.
 */
export function MetricSummary(props: MetricSummaryProps) {
  const columns = props.metrics.length

  return (
    // Literal class strings so Tailwind's scanner sees them.
    <div className={columns === 3 ? 'grid grid-cols-3' : 'grid grid-cols-2 md:grid-cols-4'}>
      {props.metrics.map((metric, index) => {
        const totals = props.totals[metric.key]
        const isSelected = props.selected === metric.key
        const tooltip = props.tooltips?.[metric.key]

        return (
          <button
            type="button"
            key={metric.key}
            onClick={() => props.onSelect(metric.key)}
            className={[
              'cursor-pointer p-4 text-left transition-colors',
              index < columns - 1 ? 'border-r border-gray-200' : '',
              isSelected
                ? 'border-b-2 border-b-[var(--primary)]'
                : 'border-b border-b-gray-200 hover:bg-gray-50'
            ].join(' ')}
          >
            <div className="mb-1 flex items-center gap-1 text-xs text-gray-500">
              {props.labels[metric.key] ?? metric.label}
              {tooltip ? (
                <Tooltip title={tooltip}>
                  <InfoCircleOutlined className="text-[10px]" />
                </Tooltip>
              ) : null}
            </div>
            {props.loading && !totals ? (
              <Skeleton active paragraph={false} title={{ width: '60%' }} />
            ) : (
              <div className="flex items-baseline gap-2">
                <span className="text-xl font-semibold text-gray-800">
                  {formatValue(totals?.current ?? 0, metric.format, props.currency)}
                </span>
                {props.showComparison ? (
                  <Delta
                    change={totals?.changePercent}
                    invertTrend={metric.invertTrend}
                    decimals={1}
                  />
                ) : null}
              </div>
            )}
          </button>
        )
      })}
    </div>
  )
}
