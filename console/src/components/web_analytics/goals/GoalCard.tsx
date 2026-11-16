import { useState } from 'react'
import { Button, Tooltip } from 'antd'
import { EyeOutlined, InfoCircleOutlined } from '@ant-design/icons'
import { useLingui } from '@lingui/react/macro'
import { KpiStrip, Panel, type KpiItem } from '../../layout'
import { useWebAnalytics } from '../useWebAnalytics'
import { Delta } from '../Delta'
import { deltaTone } from '../lib/delta'
import { MiniChart } from './MiniChart'
import { GOAL_MEASURES, goalFilters } from './queries'
import { timeBucketColumn } from '../lib/dates'
import { formatValue, toNumber } from '../lib/format'
import { buildWebQuery, useWebComparisonQuery } from '../lib/query'
import { ChartDataPoint, MetricTotals, ValueFormat } from '../lib/types'

/** The four figures a card puts on a goal, in reading order. */
export type GoalMetricKey = 'goals' | 'conversion_rate' | 'sum_goal_value' | 'median_goal_value'

const TILES: { key: GoalMetricKey; format: ValueFormat }[] = [
  { key: 'goals', format: 'number' },
  { key: 'conversion_rate', format: 'percentage' },
  { key: 'sum_goal_value', format: 'currency' },
  { key: 'median_goal_value', format: 'currency' }
]

const CHART_HEIGHT = 80

interface GoalCardProps {
  goalName: string
  totals: Record<GoalMetricKey, MetricTotals>
  onOpenDashboard: () => void
}

/**
 * One goal, as a KPI strip over a sparkline. The tiles double as the
 * sparkline's metric selector, so a card answers "how much, and which way" in
 * a single glance without opening anything.
 */
export function GoalCard({ goalName, totals, onOpenDashboard }: GoalCardProps) {
  const { t } = useLingui()
  const context = useWebAnalytics()
  const [selected, setSelected] = useState<GoalMetricKey>('goals')

  const labels: Record<GoalMetricKey, string> = {
    goals: t`Count`,
    conversion_rate: t`Conv. Rate`,
    sum_goal_value: t`Value`,
    median_goal_value: t`Median`
  }

  // Every card runs its own series query instead of the grid running one query
  // grouped by goal_name: the engine's gap filler keys rows by time bucket
  // alone, so a grouped time series comes back collapsed to one arbitrary goal
  // per bucket. Filtering to a single goal also gets the empty buckets
  // zero-filled, which is what makes the sparklines comparable.
  const base = {
    schema: 'web_goals' as const,
    measures: GOAL_MEASURES,
    dimensions: [],
    filters: goalFilters(context.filters, goalName),
    granularity: context.granularity,
    timezone: context.timezone
  }

  const series = useWebComparisonQuery(
    context.workspaceId,
    buildWebQuery({ ...base, range: context.resolved }),
    context.showComparison && context.resolvedCompare
      ? buildWebQuery({ ...base, range: context.resolvedCompare })
      : null,
    // The rate is derived from a total the series cannot express, so its tile
    // has no chart to feed and the request would be wasted.
    { enabled: selected !== 'conversion_rate' }
  )

  const bucketColumn = timeBucketColumn('goal_at', context.granularity)
  const toPoints = (rows: Record<string, unknown>[] | undefined): ChartDataPoint[] =>
    (rows ?? []).map((row) => ({
      timestamp: String(row[bucketColumn] ?? ''),
      value: toNumber(row[selected])
    }))

  const current = toPoints(series.current?.data)
  const previous = context.showComparison ? toPoints(series.previous?.data) : []

  const items: KpiItem[] = TILES.map((tile) => {
    const tileTotals = totals[tile.key]
    const change = context.showComparison ? tileTotals.changePercent : undefined
    const tone = deltaTone(change)
    return {
      key: tile.key,
      label: (
        <span className="inline-flex items-center gap-1">
          {labels[tile.key]}
          {tile.key === 'conversion_rate' ? (
            <Tooltip
              title={t`Not all visitors come with the same intent. This rate should be interpreted with caution as it includes all traffic regardless of purpose.`}
            >
              <InfoCircleOutlined className="cursor-help text-overline text-ink-4" />
            </Tooltip>
          ) : null}
        </span>
      ),
      value:
        tile.format === 'currency' && tileTotals.current === 0
          ? '-'
          : formatValue(tileTotals.current, tile.format),
      // Nothing to print for a zero change: no empty line under the value.
      sub: tone === 'neutral' ? undefined : <Delta change={change} small />,
      tone,
      onClick: () => setSelected(tile.key),
      active: selected === tile.key
    }
  })

  return (
    <Panel
      title={<span title={goalName}>{goalName}</span>}
      extra={
        <Tooltip title={t`View full dashboard`}>
          <Button
            type="text"
            size="small"
            aria-label={t`View full dashboard`}
            icon={<EyeOutlined />}
            onClick={onOpenDashboard}
          />
        </Tooltip>
      }
      flush
    >
      <div className="border-b border-line-soft">
        <KpiStrip items={items} embedded />
      </div>

      <div className="p-2">
        {selected === 'conversion_rate' ? (
          <ChartPlaceholder text={t`Select another metric to view chart`} />
        ) : series.isLoading ? (
          <div className="animate-pulse rounded bg-fill" style={{ height: CHART_HEIGHT }} />
        ) : current.length === 0 ? (
          <ChartPlaceholder text={t`No chart data`} />
        ) : (
          <MiniChart current={current} previous={previous} height={CHART_HEIGHT} />
        )}
      </div>
    </Panel>
  )
}

function ChartPlaceholder({ text }: { text: string }) {
  return (
    <div
      className="flex items-center justify-center text-xs text-ink-4"
      style={{ height: CHART_HEIGHT }}
    >
      {text}
    </div>
  )
}
