import { useMemo, useState } from 'react'
import { Alert, Drawer, Tag } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { useWebAnalytics } from '../context'
import { ColumnConfig, DimensionTabConfig, DimensionTableWidget } from '../DimensionTableWidget'
import { MetricChart, useAnnotations } from '../MetricChart'
import { MetricSummary } from '../MetricSummary'
import { TrafficHeatmapWidget } from '../TrafficHeatmapWidget'
import { CountryFlag, getDeviceIcon } from '../lib/icons'
import { GOAL_BREAKDOWN_MEASURES, GOAL_MEASURES, goalFilters, goalNameFilter } from './queries'
import { timeBucketColumn } from '../lib/dates'
import { changePercent, formatRangeSeriesName, toNumber } from '../lib/format'
import { buildWebQuery, useWebComparisonQuery } from '../lib/query'
import { ChartDataPoint, MetricConfig, MetricTotals, PRIMARY_COLOR } from '../lib/types'

/** The three series a goal has; labels are translated at render time. */
const GOAL_METRICS: MetricConfig[] = [
  { key: 'goals', label: 'Count', format: 'number', color: PRIMARY_COLOR },
  { key: 'sum_goal_value', label: 'Total Value', format: 'currency', color: PRIMARY_COLOR },
  { key: 'median_goal_value', label: 'Median Value', format: 'currency', color: PRIMARY_COLOR }
]

interface GoalDashboardDrawerProps {
  open: boolean
  goalName: string
  onClose: () => void
  /** Lets the grid drop the drawer once its closing animation has finished. */
  afterOpenChange?: (open: boolean) => void
}

/**
 * A full dashboard for one goal: the same KPI strip, chart and breakdowns as
 * the main tab, every one of them filtered down to conversions of that goal.
 */
export function GoalDashboardDrawer(props: GoalDashboardDrawerProps) {
  const { t } = useLingui()
  const context = useWebAnalytics()
  const [selected, setSelected] = useState('goals')

  const labels: Record<string, string> = {
    goals: t`Count`,
    sum_goal_value: t`Total Value`,
    median_goal_value: t`Median Value`
  }

  const filters = useMemo(
    () => goalFilters(context.filters, props.goalName),
    [context.filters, props.goalName]
  )

  const base = {
    schema: 'web_goals' as const,
    measures: GOAL_MEASURES,
    dimensions: [],
    filters,
    granularity: context.granularity,
    timezone: context.timezone
  }

  const series = useWebComparisonQuery(
    context.workspaceId,
    buildWebQuery({ ...base, range: context.resolved }),
    context.showComparison && context.resolvedCompare
      ? buildWebQuery({ ...base, range: context.resolvedCompare })
      : null,
    { enabled: props.open }
  )

  const currentRows = series.current?.data
  const previousRows = series.previous?.data

  const totals: Record<string, MetricTotals> = useMemo(() => {
    return Object.fromEntries(
      GOAL_MEASURES.map((measure) => {
        const current = periodTotal(currentRows, measure)
        const previous = periodTotal(previousRows, measure)
        return [measure, { current, previous, changePercent: changePercent(current, previous) }]
      })
    )
  }, [currentRows, previousRows])

  const bucketColumn = timeBucketColumn('goal_at', context.granularity)
  const toPoints = (rows: Record<string, unknown>[] | undefined): ChartDataPoint[] =>
    (rows ?? []).map((row) => ({
      timestamp: String(row[bucketColumn] ?? ''),
      value: toNumber(row[selected])
    }))

  const metric = GOAL_METRICS.find((candidate) => candidate.key === selected) ?? GOAL_METRICS[0]
  const chartMetric: MetricConfig = { ...metric, label: labels[metric.key] ?? metric.label }

  const annotations = useAnnotations(context.workspaceId, context.resolved, {
    enabled: props.open
  })

  const columns: ColumnConfig[] = [
    { key: 'goals', label: t`Count`, format: 'number' },
    { key: 'sum_goal_value', label: t`Value`, format: 'currency' }
  ]

  // Every breakdown reads the goal table rather than the session one, so a row
  // counts conversions of this goal instead of the sessions behind them.
  const breakdownTab = (
    tab: Pick<DimensionTabConfig, 'key' | 'label' | 'dimensionLabel' | 'dimension'> &
      Partial<DimensionTabConfig>
  ): DimensionTabConfig => ({
    schema: 'web_goals',
    measures: GOAL_BREAKDOWN_MEASURES,
    order: { goals: 'desc' },
    ...tab,
    filters: [goalNameFilter(props.goalName), ...(tab.filters ?? [])]
  })

  return (
    <Drawer
      open={props.open}
      onClose={props.onClose}
      afterOpenChange={props.afterOpenChange}
      placement="right"
      size="100%"
      styles={{ body: { background: 'var(--background)' } }}
      title={
        <div className="flex items-center gap-2">
          <Tag color="green" className="m-0">
            {props.goalName}
          </Tag>
          <span className="font-normal text-gray-500">{t`Goal dashboard`}</span>
        </div>
      }
    >
      <div className={series.isFetching ? 'opacity-75 transition-opacity' : ''}>
        {series.error ? (
          <Alert type="error" showIcon title={t`Could not load this goal`} className="mb-4" />
        ) : null}

        <div className="overflow-hidden rounded-md border border-gray-200">
          <MetricSummary
            metrics={GOAL_METRICS}
            totals={series.isLoading ? {} : totals}
            selected={selected}
            onSelect={setSelected}
            showComparison={context.showComparison}
            loading={series.isLoading}
            labels={labels}
          />
          <div className="pb-3 pl-2 pr-4 pt-4">
            <MetricChart
              metric={chartMetric}
              current={toPoints(currentRows)}
              previous={context.showComparison ? toPoints(previousRows) : []}
              granularity={context.granularity}
              currentLabel={formatRangeSeriesName(context.range)}
              previousLabel={
                context.compareRange ? formatRangeSeriesName(context.compareRange) : undefined
              }
              loading={series.isLoading}
              height={200}
              annotations={annotations}
              timezone={context.timezone}
            />
          </div>
        </div>

        <div className="mt-4 grid grid-cols-1 gap-4 md:grid-cols-2">
          <DimensionTableWidget
            title={t`Top Sources`}
            columns={columns}
            emptyText={t`No source data`}
            tabs={[
              breakdownTab({
                key: 'referrers',
                label: t`Referrers`,
                dimensionLabel: t`Referrer domain`,
                dimension: 'referrer_domain'
              }),
              breakdownTab({
                key: 'channels',
                label: t`Channels`,
                dimensionLabel: t`Channel`,
                dimension: 'channel'
              }),
              breakdownTab({
                key: 'channel_groups',
                label: t`Channel groups`,
                dimensionLabel: t`Channel group`,
                dimension: 'channel_group'
              })
            ]}
          />
          <DimensionTableWidget
            title={t`Top Campaigns`}
            columns={columns}
            emptyText={t`No campaign data`}
            tabs={(
              [
                ['campaign', t`Campaigns`, 'utm_campaign'],
                ['source', t`Sources`, 'utm_source'],
                ['medium', t`Mediums`, 'utm_medium']
              ] as const
            ).map(([key, label, dimension]) =>
              breakdownTab({
                key,
                label,
                dimensionLabel: label,
                dimension,
                // Most conversions carry no campaign at all; without this the
                // table would be one giant "(empty)" row on every tab.
                filters: [{ dimension, operator: 'isNotEmpty', values: [] }]
              })
            )}
          />
        </div>

        <div className="mt-4 grid grid-cols-1 gap-4 md:grid-cols-2">
          <DimensionTableWidget
            title={t`Countries`}
            columns={columns}
            emptyText={t`No country data`}
            iconPrefix={(value, tabKey) => (tabKey === 'list' ? <CountryFlag iso2={value} /> : null)}
            tabs={[
              breakdownTab({
                key: 'map',
                label: t`Map`,
                dimensionLabel: t`Country`,
                dimension: 'country',
                type: 'country_map',
                limit: 100
              }),
              breakdownTab({
                key: 'list',
                label: t`List`,
                dimensionLabel: t`Country`,
                dimension: 'country'
              })
            ]}
          />
          <DimensionTableWidget
            title={t`Devices`}
            columns={columns}
            emptyText={t`No device data`}
            iconPrefix={(value, tabKey) => getDeviceIcon(value, tabKey)}
            tabs={[
              breakdownTab({
                key: 'devices',
                label: t`Devices`,
                dimensionLabel: t`Device`,
                dimension: 'device'
              }),
              breakdownTab({
                key: 'browsers',
                label: t`Browsers`,
                dimensionLabel: t`Browser`,
                dimension: 'browser'
              }),
              breakdownTab({ key: 'os', label: t`OS`, dimensionLabel: t`OS`, dimension: 'os' })
            ]}
          />
        </div>

        <div className="mt-4 grid grid-cols-1 gap-4 md:grid-cols-2">
          <TrafficHeatmapWidget
            title={t`Goals by day and hour`}
            schema="web_goals"
            emptyText={t`No goal data`}
            extraFilters={[goalNameFilter(props.goalName)]}
            tabs={[{ key: 'goals', label: t`Goals`, measure: 'goals', format: 'number' }]}
          />
        </div>
      </div>
    </Drawer>
  )
}

/**
 * Period total for a measure, read off the bucketed series.
 *
 * Counts and sums add up. A median does not: the engine returns one median per
 * bucket with no way to recombine them, so the KPI shows the mean of the
 * buckets that actually held a conversion. Buckets the gap filler zero-filled
 * have no median at all, and averaging those in would only dilute the figure.
 */
function periodTotal(rows: Record<string, unknown>[] | undefined, measure: string): number {
  const all = rows ?? []
  if (measure !== 'median_goal_value') {
    return all.reduce((sum, row) => sum + toNumber(row[measure]), 0)
  }
  const observed = all.filter((row) => toNumber(row.goals) > 0)
  if (observed.length === 0) return 0
  return observed.reduce((sum, row) => sum + toNumber(row[measure]), 0) / observed.length
}
