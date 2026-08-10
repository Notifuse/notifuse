import { useState } from 'react'
import { Button, Tooltip } from 'antd'
import { EyeOutlined, InfoCircleOutlined } from '@ant-design/icons'
import { useLingui } from '@lingui/react/macro'
import { useWebAnalytics } from '../context'
import { Delta } from '../Delta'
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

  return (
    <div className="overflow-hidden rounded-md border border-gray-200">
      <div className="flex items-center justify-between gap-2 px-4 pb-2 pt-3">
        <span className="truncate font-medium text-gray-800" title={goalName}>
          {goalName}
        </span>
        <Tooltip title={t`View full dashboard`}>
          <Button type="text" size="small" icon={<EyeOutlined />} onClick={onOpenDashboard} />
        </Tooltip>
      </div>

      <div className="grid grid-cols-2 sm:grid-cols-4">
        {TILES.map((tile, index) => {
          const tileTotals = totals[tile.key]
          const isSelected = selected === tile.key
          // Two columns on mobile, four from `sm` up, so the separator a tile
          // needs on its right differs between the two layouts.
          const isRightEdgeMobile = index % 2 === 1
          const isRightEdgeDesktop = index === TILES.length - 1

          return (
            <button
              type="button"
              key={tile.key}
              onClick={() => setSelected(tile.key)}
              className={[
                'cursor-pointer p-2 text-center transition-colors',
                isRightEdgeMobile ? '' : 'border-r border-gray-100',
                isRightEdgeDesktop ? 'sm:border-r-0' : 'sm:border-r sm:border-gray-100',
                isSelected
                  ? 'border-b-2 border-b-[var(--primary)] bg-gray-50/50'
                  : 'border-b border-b-gray-100 hover:bg-gray-50'
              ].join(' ')}
            >
              <div className="mb-1 flex items-center justify-center gap-1 text-xs text-gray-500">
                {labels[tile.key]}
                {tile.key === 'conversion_rate' ? (
                  <Tooltip
                    title={t`Not all visitors come with the same intent. This rate should be interpreted with caution as it includes all traffic regardless of purpose.`}
                  >
                    <InfoCircleOutlined className="cursor-help text-[10px] text-gray-400" />
                  </Tooltip>
                ) : null}
              </div>
              <div className="text-base font-semibold text-gray-800">
                {tile.format === 'currency' && tileTotals.current === 0
                  ? '-'
                  : formatValue(tileTotals.current, tile.format)}
              </div>
              {context.showComparison ? (
                <div className="flex justify-center">
                  <Delta change={tileTotals.changePercent} size={11} />
                </div>
              ) : null}
            </button>
          )
        })}
      </div>

      <div className="px-2 pb-2">
        {selected === 'conversion_rate' ? (
          <ChartPlaceholder text={t`Select another metric to view chart`} />
        ) : series.isLoading ? (
          <div className="animate-pulse rounded bg-gray-100" style={{ height: CHART_HEIGHT }} />
        ) : current.length === 0 ? (
          <ChartPlaceholder text={t`No chart data`} />
        ) : (
          <MiniChart current={current} previous={previous} height={CHART_HEIGHT} />
        )}
      </div>
    </div>
  )
}

function ChartPlaceholder({ text }: { text: string }) {
  return (
    <div
      className="flex items-center justify-center text-xs text-gray-400"
      style={{ height: CHART_HEIGHT }}
    >
      {text}
    </div>
  )
}
