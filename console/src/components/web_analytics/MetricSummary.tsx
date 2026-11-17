import { Tooltip } from 'antd'
import type { ReactNode } from 'react'
import { InfoCircleOutlined } from '@ant-design/icons'
import { KpiStrip, type KpiItem } from '../layout'
import { Delta } from './Delta'
import { deltaTone } from './lib/delta'
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
  /** Inside a card: the strip draws neither border nor shadow. */
  embedded?: boolean
  /** A line under a value that has no change to show ("21.4% of search-paid"), keyed by metric. */
  subs?: Record<string, ReactNode>
}

/**
 * The KPI strip is also the chart's metric selector: the numbers are the thing
 * an operator scans first, so making them the control removes a separate
 * dropdown that would only ever repeat them.
 */
export function MetricSummary(props: MetricSummaryProps) {
  const items: KpiItem[] = props.metrics.map((metric) => {
    const totals = props.totals[metric.key]
    const tooltip = props.tooltips?.[metric.key]
    const change = props.showComparison ? totals?.changePercent : undefined
    const tone = deltaTone(change, metric.invertTrend)

    return {
      key: metric.key,
      label: (
        <span className="inline-flex items-center gap-1">
          {props.labels[metric.key] ?? metric.label}
          {tooltip ? (
            <Tooltip title={tooltip}>
              <InfoCircleOutlined className="text-overline" />
            </Tooltip>
          ) : null}
        </span>
      ),
      // No value is no amount, as on the goal cards: a currency 0 reads "-".
      value:
        metric.format === 'currency' && (totals?.current ?? 0) === 0
          ? '-'
          : formatValue(totals?.current ?? 0, metric.format, props.currency),
      // Nothing to print for a zero or missing change: no empty sub line under the value.
      sub:
        tone === 'neutral' ? (
          props.subs?.[metric.key]
        ) : (
          <Delta change={change} invertTrend={metric.invertTrend} decimals={1} />
        ),
      tone,
      onClick: metric.selectable === false ? undefined : () => props.onSelect(metric.key),
      active: metric.selectable === false ? undefined : props.selected === metric.key
    }
  })

  return <KpiStrip items={items} loading={props.loading} embedded={props.embedded} />
}
